// Codex `app-server` 适配器（JSON-RPC 2.0 over stdio）。
//
// 线路形态（真实数据，见上游研究探针 research/probes/codex/*.jsonl）：
//
//	三类入站消息：
//	  1. 对出站请求的响应：{ id, result } | { id, error }，无 method
//	  2. 服务端发来的请求：{ method, id, params }——必须回复
//	  3. 通知：{ method, params }——即发即弃
//
//	出站顺序：initialize → thread/start → turn/start
//
//	翻译成的事件：
//	  thread/start 的 result.thread.id → 持久化 threadId + sessionId（同一 UUIDv7）
//	  item/started commandExecution    → progress(tool=shell, cmd, status=inProgress)
//	  item/started mcpToolCall         → progress(kind=mcp, server, tool, args_summary)
//	  item/started dynamicToolCall     → progress(kind=dynamic, namespace, tool, args)
//	  item/started webSearch           → progress(kind=web_search, query)
//	  item/started fileChange          → progress(kind=file_change)
//	  item/completed agentMessage      → message(text, phase)
//	  item/agentMessage/delta          → progress(kind, stream_id, text_delta)
//	  item/completed commandExecution  → 可选 progress(status, exitCode)
//	  item/started collabAgentToolCall → error（多 agent 递归被 channel 阻断）
//	  turn/completed                   → done
//	  turn/aborted                     → error(reason=aborted)
//	  warning                          → progress(kind=warning, message)
//	  mcpServer/elicitation/request    → 自动回复 { action: accept, content: {} }
//
//	静默跳过的通知：remoteControl/status/changed、mcpServer/startupStatus/updated、
//	mcpServer/oauthLoginCompleted、account/rateLimits/updated、thread/status/changed、
//	thread/started、turn/started、serverRequest/resolved、ItemGuardianApprovalReview*。
//
// 与上游的一处有意偏离（见包文档）：thread/tokenUsage/updated 通知
// 上游跳过；本包按任务要求提取 token 用量，转为 progress 事件
// （detail.kind 为 token_usage，含 total/last 用量）。
package adapters

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/BondLam/Sunshine/internal/channel/store"
)

// CodexCtx 是 codex 适配器的每 worker 可变上下文：
// 跟踪待处理出站请求 id、item 流元数据与回合终态。
type CodexCtx struct {
	// Pending 是出站请求 id → 标签（initialize / thread/start / turn/start / other）。
	Pending map[int]string
	// Items 是 codex item id → 流元数据，用于分类交错到达的 delta。
	Items map[string]codexItemMeta
	// FinalMessageSeen 表示本回合已发出最终可见答案。
	FinalMessageSeen bool
	// PendingDone 表示 turn/completed 先于最终 agentMessage 到达，done 待补。
	PendingDone bool
	// TerminalErrorSeen 防止同一失败被 Codex 报告两次时重复发 error。
	TerminalErrorSeen bool
	// ThreadID 是最近一次 thread/start 拿到的线程 id（即会话 id）。
	ThreadID string
	// NextID 是出站请求 id 的单调分配器。
	NextID int
}

// NewCodexCtx 生成一份新的 codex 解析上下文。
func NewCodexCtx() *CodexCtx {
	return &CodexCtx{
		Pending: make(map[int]string),
		Items:   make(map[string]codexItemMeta),
		NextID:  1,
	}
}

type codexItemMeta struct {
	Type  string
	Phase string
}

type codexInbound struct {
	JSONRPC string         `json:"jsonrpc"`
	ID      *int           `json:"id"`
	Method  string         `json:"method"`
	Params  map[string]any `json:"params"`
	Result  any            `json:"result"`
	Error   *codexRPCError `json:"error"`
}

type codexRPCError struct {
	Code    *int   `json:"code"`
	Message string `json:"message"`
}

// skipMethods 是静默跳过的通知方法集合。
var skipMethods = map[string]bool{
	"remoteControl/status/changed":         true,
	"mcpServer/startupStatus/updated":      true,
	"mcpServer/oauthLoginCompleted":        true,
	"account/rateLimits/updated":           true,
	"thread/status/changed":                true,
	"thread/started":                       true,
	"turn/started":                         true,
	"serverRequest/resolved":               true,
	"itemGuardianApprovalReview/started":   true,
	"itemGuardianApprovalReview/completed": true,
}

// ParseCodexLine 解析一行 Codex app-server stdout，返回频道事件与副作用。
// ctx 跟踪待处理出站 id；调用方须在每 worker 的整个生命周期内复用同一 ctx。
func ParseCodexLine(line string, ctx *CodexCtx) ParseResult {
	trimmed := strings.TrimSpace(line)
	if trimmed == "" {
		return ParseResult{}
	}
	if ctx == nil {
		ctx = NewCodexCtx()
	}
	var msg codexInbound
	if err := json.Unmarshal([]byte(trimmed), &msg); err != nil {
		return ParseResult{Events: []AdapterEvent{{
			Kind: store.KindError,
			Payload: map[string]any{
				"message":     "解析 Codex 输出行失败",
				"raw_excerpt": firstRunes(trimmed, 200),
			},
		}}}
	}
	// (1) 服务端发来的请求：有 method 又有 id。
	if msg.Method != "" && msg.ID != nil {
		return handleCodexServerRequest(&msg)
	}
	// (2) 对出站请求的响应：有 id、无 method。
	if msg.ID != nil && msg.Method == "" {
		return handleCodexResponse(&msg, ctx)
	}
	// (3) 通知。
	if msg.Method != "" {
		return handleCodexNotification(&msg, ctx)
	}
	return ParseResult{}
}

func handleCodexServerRequest(msg *codexInbound) ParseResult {
	var events []AdapterEvent
	result := any(map[string]any{"action": "decline"})
	if msg.Method == "mcpServer/elicitation/request" {
		// MVP：自动放行 MCP 工具调用。channel worker 的派生本身
		// 已由执行 `channel spawn` 的人信任，权限边界在 spawn 调用处，
		// 不在每次 MCP 调用上。
		result = map[string]any{"action": "accept", "content": map[string]any{}}
		params := msg.Params
		meta, _ := params["_meta"].(map[string]any)
		events = append(events, AdapterEvent{
			Kind: store.KindProgress,
			Payload: map[string]any{"detail": map[string]any{
				"kind":             "mcp_elicitation_auto_accept",
				"server":           params["serverName"],
				"tool_description": meta["tool_description"],
			}},
		})
	} else {
		// 未知的服务端请求：拒绝并记一条 error 事件。
		events = append(events, AdapterEvent{
			Kind: store.KindError,
			Payload: map[string]any{
				"message":    fmt.Sprintf("未知的服务端请求：%s", msg.Method),
				"request_id": *msg.ID,
			},
		})
	}
	reply, _ := json.Marshal(map[string]any{"jsonrpc": "2.0", "id": *msg.ID, "result": result})
	return ParseResult{
		Events: events,
		Side:   &SideEffect{Reply: []string{string(reply) + "\n"}},
	}
}

func handleCodexResponse(msg *codexInbound, ctx *CodexCtx) ParseResult {
	id := *msg.ID
	label := ctx.Pending[id]
	delete(ctx.Pending, id)
	var events []AdapterEvent
	side := &SideEffect{Resolved: []ResolvedRPC{{ID: id, Result: msg.Result, Error: msg.Error}}}
	if msg.Error != nil {
		events = append(events, AdapterEvent{
			Kind: store.KindError,
			Payload: map[string]any{
				"message": fmt.Sprintf("%s 出站请求 RPC 出错（id=%d）：%s", labelOrUnknown(label), id, msg.Error.Message),
				"code":    msg.Error.Code,
			},
		})
		return ParseResult{Events: events, Side: side}
	}
	if label == "thread/start" {
		if resultMap, ok := msg.Result.(map[string]any); ok {
			if thread, ok := resultMap["thread"].(map[string]any); ok {
				threadID, _ := thread["id"].(string)
				if threadID == "" {
					threadID, _ = thread["sessionId"].(string)
				}
				if threadID != "" {
					ctx.ThreadID = threadID
					side.PersistThreadID = threadID
					// codex 的 thread id 与 session id 是同一 UUIDv7，
					// 消费方按同一 id 处理。
					side.PersistSessionID = threadID
				}
			}
		}
	}
	return ParseResult{Events: events, Side: side}
}

func handleCodexNotification(msg *codexInbound, ctx *CodexCtx) ParseResult {
	method := msg.Method
	if skipMethods[method] {
		return ParseResult{}
	}
	switch method {
	case "item/started":
		return handleCodexItemStarted(msg, ctx)
	case "item/completed":
		return handleCodexItemCompleted(msg, ctx)
	case "item/agentMessage/delta":
		return handleCodexAgentMessageDelta(msg, ctx)
	case "turn/completed":
		return handleCodexTurnCompleted(msg, ctx)
	case "error":
		return handleCodexErrorNotification(msg, ctx)
	case "turn/aborted":
		return ParseResult{Events: []AdapterEvent{{
			Kind:    store.KindError,
			Payload: map[string]any{"message": "回合已中止", "reason": "aborted"},
		}}}
	case "warning":
		message, _ := msg.Params["message"].(string)
		if message == "" {
			message = "<无内容>"
		}
		return progressEvent(map[string]any{"kind": "warning", "message": message})
	case "thread/tokenUsage/updated":
		// 上游跳过本通知；本包按任务要求提取 token 用量。
		detail := map[string]any{"kind": "token_usage"}
		if usage, ok := msg.Params["tokenUsage"]; ok {
			detail["usage"] = usage
		}
		return progressEvent(detail)
	case "mcp/toolCall/progress":
		message, _ := msg.Params["message"].(string)
		return progressEvent(map[string]any{"kind": "mcp_progress", "text_delta": message})
	default:
		return ParseResult{}
	}
}

func handleCodexTurnCompleted(msg *codexInbound, ctx *CodexCtx) ParseResult {
	turn, _ := msg.Params["turn"].(map[string]any)
	status, _ := turn["status"].(string)
	if status == "failed" {
		ctx.PendingDone = false
		if ctx.TerminalErrorSeen {
			return ParseResult{}
		}
		ctx.TerminalErrorSeen = true
		return ParseResult{Events: []AdapterEvent{{
			Kind:    store.KindError,
			Payload: map[string]any{"message": codexErrorMessage(turn["error"], "Codex 回合失败")},
		}}}
	}
	if ctx.TerminalErrorSeen {
		return ParseResult{}
	}
	if ctx.FinalMessageSeen {
		ctx.PendingDone = false
		return ParseResult{Events: []AdapterEvent{{Kind: store.KindDone, Payload: map[string]any{}}}}
	}
	ctx.PendingDone = true
	return ParseResult{}
}

func handleCodexErrorNotification(msg *codexInbound, ctx *CodexCtx) ParseResult {
	params := msg.Params
	message := codexErrorMessage(params["error"], "Codex app-server 出错")
	if willRetry, _ := params["willRetry"].(bool); willRetry {
		return progressEvent(map[string]any{"kind": "warning", "message": message})
	}
	if ctx.TerminalErrorSeen {
		return ParseResult{}
	}
	ctx.TerminalErrorSeen = true
	ctx.PendingDone = false
	return ParseResult{Events: []AdapterEvent{{
		Kind:    store.KindError,
		Payload: map[string]any{"message": message},
	}}}
}

func handleCodexItemStarted(msg *codexInbound, ctx *CodexCtx) ParseResult {
	item, ok := msg.Params["item"].(map[string]any)
	if !ok {
		return ParseResult{}
	}
	rememberCodexItem(ctx, item)
	itemType, _ := item["type"].(string)
	switch itemType {
	case "commandExecution":
		return progressEvent(map[string]any{
			"tool":   "shell",
			"cmd":    summarizeInput(item["command"], 120),
			"status": item["status"],
		})
	case "mcpToolCall":
		return progressEvent(map[string]any{
			"kind":         "mcp",
			"server":       item["server"],
			"tool_name":    item["tool"],
			"args_summary": summarizeInput(item["arguments"], 120),
		})
	case "dynamicToolCall":
		return progressEvent(map[string]any{
			"kind":         "dynamic_tool",
			"namespace":    item["namespace"],
			"tool_name":    item["tool"],
			"args_summary": summarizeInput(item["arguments"], 120),
		})
	case "webSearch":
		action, _ := item["action"].(map[string]any)
		return progressEvent(map[string]any{"kind": "web_search", "query": action["query"]})
	case "fileChange":
		return progressEvent(map[string]any{"kind": "file_change", "status": item["status"]})
	case "imageView":
		return progressEvent(map[string]any{"kind": "image_view", "path": item["path"]})
	case "collabAgentToolCall":
		return ParseResult{Events: []AdapterEvent{{
			Kind: store.KindError,
			Payload: map[string]any{
				"message":             "Worker 试图派生 codex 子代理（collabAgentToolCall），channel 已阻断该行为",
				"recommendation":      "thread/start 须设 features.multi_agent=false 以防递归",
				"receiver_thread_ids": item["receiverThreadIds"],
			},
		}}}
	case "agentMessage", "userMessage", "reasoning", "plan", "hookPrompt",
		"contextCompaction", "enteredReviewMode", "exitedReviewMode":
		return ParseResult{}
	default:
		// 未知 item 类型，静默跳过。
		return ParseResult{}
	}
}

func handleCodexItemCompleted(msg *codexInbound, ctx *CodexCtx) ParseResult {
	item, ok := msg.Params["item"].(map[string]any)
	if !ok {
		return ParseResult{}
	}
	rememberCodexItem(ctx, item)
	itemType, _ := item["type"].(string)
	switch itemType {
	case "agentMessage":
		text, _ := item["text"].(string)
		if text == "" {
			return ParseResult{}
		}
		phase, _ := item["phase"].(string)
		// Codex 的 commentary agentMessage 是回合内的旁白/思考，
		// 真正面向用户的答案是 final_answer（或无 phase 的 agentMessage）。
		// commentary 映射到 progress，保持 `kind:message` 一回合一条答案，
		// 让 `--no-progress` / `wait --kind message` 的行为符合预期。
		if phase == "commentary" {
			// commentary 单回合可达多 KB，截断后写事件，避免 events.jsonl 膨胀。
			return progressEvent(map[string]any{
				"kind":       "commentary",
				"text_delta": summarizeInput(text, 4000),
			})
		}
		ctx.FinalMessageSeen = true
		events := []AdapterEvent{{Kind: store.KindMessage, Payload: map[string]any{"text": text}}}
		if ctx.PendingDone {
			ctx.PendingDone = false
			events = append(events, AdapterEvent{Kind: store.KindDone, Payload: map[string]any{}})
		}
		return ParseResult{Events: events}
	case "commandExecution":
		if exitCode, ok := toFloat(item["exitCode"]); ok && exitCode != 0 {
			return progressEvent(map[string]any{
				"tool":        "shell",
				"status":      "failed",
				"exit_code":   exitCode,
				"duration_ms": item["durationMs"],
			})
		}
		return ParseResult{}
	case "mcpToolCall":
		if item["error"] != nil {
			return progressEvent(map[string]any{
				"kind":        "mcp",
				"status":      "failed",
				"server":      item["server"],
				"tool_name":   item["tool"],
				"error":       summarizeInput(item["error"], 120),
				"duration_ms": item["durationMs"],
			})
		}
		return ParseResult{}
	default:
		return ParseResult{}
	}
}

func handleCodexAgentMessageDelta(msg *codexInbound, ctx *CodexCtx) ParseResult {
	params := msg.Params
	delta, _ := params["delta"].(string)
	if delta == "" {
		delta, _ = params["text"].(string)
	}
	if delta == "" {
		return ParseResult{}
	}
	var meta *codexItemMeta
	if itemID, ok := params["itemId"].(string); ok {
		if m, found := ctx.Items[itemID]; found {
			cp := m
			meta = &cp
		}
	}
	if item, ok := params["item"].(map[string]any); ok {
		rememberCodexItem(ctx, item)
		m := codexItemMetaOf(item)
		meta = &m
	}
	kind := classifyCodexDelta(meta)
	detail := map[string]any{"kind": kind, "text_delta": delta}
	if itemID, ok := params["itemId"].(string); ok {
		detail["stream_id"] = itemID
	}
	if meta != nil && meta.Phase != "" {
		detail["phase"] = meta.Phase
	}
	return progressEvent(detail)
}

func rememberCodexItem(ctx *CodexCtx, item map[string]any) {
	id, _ := item["id"].(string)
	if id == "" {
		return
	}
	ctx.Items[id] = codexItemMetaOf(item)
}

func codexItemMetaOf(item map[string]any) codexItemMeta {
	m := codexItemMeta{}
	m.Type, _ = item["type"].(string)
	m.Phase, _ = item["phase"].(string)
	return m
}

func classifyCodexDelta(meta *codexItemMeta) string {
	if meta == nil {
		return "output"
	}
	if meta.Type == "reasoning" {
		return "reasoning"
	}
	if meta.Phase == "commentary" {
		return "commentary"
	}
	return "output"
}

func codexErrorMessage(err any, fallback string) string {
	if s, ok := err.(string); ok && strings.TrimSpace(s) != "" {
		return s
	}
	if m, ok := err.(map[string]any); ok {
		if s, _ := m["message"].(string); strings.TrimSpace(s) != "" {
			return s
		}
	}
	return fallback
}

func progressEvent(detail map[string]any) ParseResult {
	return ParseResult{Events: []AdapterEvent{{
		Kind:    store.KindProgress,
		Payload: map[string]any{"detail": detail},
	}}}
}

func labelOrUnknown(label string) string {
	if label == "" {
		return "<未知>"
	}
	return label
}

func toFloat(v any) (float64, bool) {
	switch n := v.(type) {
	case float64:
		return n, true
	case json.Number:
		f, err := n.Float64()
		return f, err == nil
	case int:
		return float64(n), true
	default:
		return 0, false
	}
}

// EncodeCodexRequest 编码一条 codex JSON-RPC 出站请求，分配 id 并登记待处理标签。
func EncodeCodexRequest(ctx *CodexCtx, method string, params any, label string) (int, string) {
	id := ctx.NextID
	ctx.NextID++
	ctx.Pending[id] = label
	line, _ := json.Marshal(map[string]any{"jsonrpc": "2.0", "id": id, "method": method, "params": params})
	return id, string(line) + "\n"
}

// EncodeCodexUserMessage 把频道消息编码成 turn/start 请求。
// thread/start 尚未完成（无 threadId）时返回错误。
func EncodeCodexUserMessage(ctx *CodexCtx, text string) (string, error) {
	if ctx.ThreadID == "" {
		return "", fmt.Errorf("Codex 适配器：thread/start 尚未完成，无法发送用户消息")
	}
	ctx.FinalMessageSeen = false
	ctx.PendingDone = false
	ctx.TerminalErrorSeen = false
	_, line := EncodeCodexRequest(ctx, "turn/start", map[string]any{
		"threadId": ctx.ThreadID,
		"input":    []any{map[string]any{"type": "text", "text": text}},
	}, "turn/start")
	return line, nil
}

// EncodeCodexInterruptMessage 编码 codex 中断：
// 发一轮带中断标记的新 turn/start，让 worker 丢弃当前工作、
// 改按新指令执行（上游实现即如此）。
func EncodeCodexInterruptMessage(ctx *CodexCtx, text string) (string, error) {
	return EncodeCodexUserMessage(ctx, "[GRID INTERRUPT - drop current work and follow this new instruction]\n"+text)
}

// BuildCodexArgs 生成 `codex app-server` 的参数。
func BuildCodexArgs(model string) []string {
	args := []string{"app-server"}
	if model != "" {
		args = append(args, "-c", fmt.Sprintf("model=%q", model))
	}
	return args
}

// CodexSandboxMode 是 codex 文档化的沙箱模式（见 `codex app-server` thread/start）。
type CodexSandboxMode string

const (
	// CodexSandboxReadOnly 只读沙箱。
	CodexSandboxReadOnly CodexSandboxMode = "read-only"
	// CodexSandboxWorkspaceWrite 工作区可写（默认）。
	CodexSandboxWorkspaceWrite CodexSandboxMode = "workspace-write"
	// CodexSandboxDangerFullAccess 完全访问（危险）。
	CodexSandboxDangerFullAccess CodexSandboxMode = "danger-full-access"
)

// CodexSandboxModes 是全部合法的沙箱模式。
var CodexSandboxModes = []CodexSandboxMode{
	CodexSandboxReadOnly, CodexSandboxWorkspaceWrite, CodexSandboxDangerFullAccess,
}

// ParseCodexSandboxMode 校验 --sandbox 值；未设置时返回 ""，
// 调用方回落到 workspace-write 默认值。
func ParseCodexSandboxMode(v string) (CodexSandboxMode, error) {
	if v == "" {
		return "", nil
	}
	for _, m := range CodexSandboxModes {
		if CodexSandboxMode(v) == m {
			return m, nil
		}
	}
	return "", fmt.Errorf("无效的 --sandbox %q，应为 read-only、workspace-write、danger-full-access 之一", v)
}

// BuildCodexThreadStartParams 生成 thread/start 的参数：
// 默认审批策略为永不询问、沙箱默认 workspace-write，
// 并关闭 codex 原生多 agent，避免派生的 worker 再递归派生子代理
// （会与 channel 的协作层冲突）。
func BuildCodexThreadStartParams(cwd, systemPrompt string, sandbox CodexSandboxMode) map[string]any {
	if sandbox == "" {
		sandbox = CodexSandboxWorkspaceWrite
	}
	params := map[string]any{
		"cwd":            cwd,
		"approvalPolicy": "never",
		"sandbox":        string(sandbox),
		"config": map[string]any{
			"features": map[string]any{
				"multi_agent": false,
				"multi_agent_v2": map[string]any{
					"enabled": false,
				},
			},
		},
	}
	if strings.TrimSpace(systemPrompt) != "" {
		params["developerInstructions"] = systemPrompt
	}
	return params
}

// codexAdapter 是 Codex 适配器：JSON-RPC 2.0 via `app-server`，需要握手。
type codexAdapter struct{}

func (codexAdapter) Provider() Provider { return ProviderCodex }

func (codexAdapter) BuildArgs(view SupervisorView) []string {
	return BuildCodexArgs(view.Model)
}

func (codexAdapter) CreateCtx() any { return NewCodexCtx() }

// Handshake 执行 initialize → thread/start，并等 threadId 落定（30 秒上限）。
func (codexAdapter) Handshake(ctx context.Context, stdin StdinWriter, c any, view SupervisorView) error {
	cc, ok := c.(*CodexCtx)
	if !ok || cc == nil {
		return fmt.Errorf("Codex 握手：解析上下文类型不对")
	}
	_, initLine := EncodeCodexRequest(cc, "initialize", map[string]any{
		"clientInfo":   map[string]any{"name": "sunshine-channel", "version": "0.1"},
		"capabilities": map[string]any{},
	}, "initialize")
	if err := stdin.WriteLine(initLine); err != nil {
		return fmt.Errorf("Codex 握手：发送 initialize 失败：%w", err)
	}
	// 让 initialize 先落下，再发 thread/start。
	select {
	case <-time.After(150 * time.Millisecond):
	case <-ctx.Done():
		return ctx.Err()
	}
	sandbox, err := ParseCodexSandboxMode(view.Sandbox)
	if err != nil {
		return err
	}
	_, tsLine := EncodeCodexRequest(cc, "thread/start",
		BuildCodexThreadStartParams(view.Cwd, view.SystemPrompt, sandbox), "thread/start")
	if err := stdin.WriteLine(tsLine); err != nil {
		return fmt.Errorf("Codex 握手：发送 thread/start 失败：%w", err)
	}
	deadline := time.Now().Add(30 * time.Second)
	for cc.ThreadID == "" && time.Now().Before(deadline) {
		select {
		case <-time.After(50 * time.Millisecond):
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	if cc.ThreadID == "" {
		return fmt.Errorf("Codex thread/start 在 30 秒内未返回 threadId")
	}
	return nil
}

func (codexAdapter) IsReady(c any) bool {
	cc, ok := c.(*CodexCtx)
	return ok && cc != nil && cc.ThreadID != ""
}

func (codexAdapter) ParseLine(line string, c any) ParseResult {
	cc, _ := c.(*CodexCtx)
	return ParseCodexLine(line, cc)
}

func (codexAdapter) EncodeUserMessage(text string, c any) (string, error) {
	cc, _ := c.(*CodexCtx)
	if cc == nil {
		return "", fmt.Errorf("Codex 适配器：缺少解析上下文")
	}
	return EncodeCodexUserMessage(cc, text)
}

func (codexAdapter) EncodeInterruptMessage(text string, c any) (string, error) {
	cc, _ := c.(*CodexCtx)
	if cc == nil {
		return "", fmt.Errorf("Codex 适配器：缺少解析上下文")
	}
	return EncodeCodexInterruptMessage(cc, text)
}
