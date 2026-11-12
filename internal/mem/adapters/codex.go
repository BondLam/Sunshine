package adapters

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/BondLam/Sunshine/internal/mem"
)

// Codex 会话布局：~/.codex/sessions/**/<rollout-<时间戳>-<id>.jsonl。
// 元数据取首个事件的 payload；文件名时间戳是 created 的后备。
//
// 压缩：顶层 compacted 事件标注 Codex 截断自身上下文的位置。
// 其 payload.replacement_history 不是摘要，而是逐字保留下来的对话
// 切片——已在池子里的轮次按出现次数去重，不重复加入；只在保留历史
// 里出现的轮次恢复到池子最前面，并补一条边界 marker。
//
// 多智能体：response_item / agent_message 事件带
// “Message Type: NEW_TASK | MESSAGE | FINAL_ANSWER”信封，只有
// FINAL_ANSWER 是明文；其余是 encrypted_content，读不回来，
// 以警告说明而不是假装读到。event_msg / agent_message 是紧随其后
// 的 response_item / message 的逐字节重复，始终排除。

var rolloutNamePattern = regexp.MustCompile(`^rollout-(\d{4}-\d{2}-\d{2}T\d{2}-\d{2}-\d{2})-(.+)$`)

func codexSessions(ctx context.Context, home string) ([]mem.MemSessionInfo, []string, error) {
	root := CodexSessionsDir(home)
	if !dirExists(root) {
		return nil, []string{fmt.Sprintf("未找到 Codex 会话存储（%s），已跳过", root)}, nil
	}
	var out []mem.MemSessionInfo
	for _, file := range WalkDir(root) {
		if ctx.Err() != nil {
			return out, nil, ctx.Err()
		}
		if !strings.HasSuffix(file, ".jsonl") {
			continue
		}
		base := strings.TrimSuffix(filepath.Base(file), ".jsonl")
		var tsFromName string
		idFromName := base
		if m := rolloutNamePattern.FindStringSubmatch(base); m != nil {
			ts := m[1]
			// 2026-11-09T10-00-00 → 2026-11-09T10:00:00Z
			iso := ts[:11] + strings.ReplaceAll(ts[11:], "-", ":") + "Z"
			if parsed, err := time.Parse("2006-01-02T15:04:05Z", iso); err == nil {
				tsFromName = formatISOTime(parsed)
			}
			idFromName = m[2]
		}
		first, _ := ReadJSONLFirst(file)
		meta := objMap(first, "payload")
		id := objStr(meta, "id")
		if id == "" {
			id = idFromName
		}
		cwd := objStr(meta, "cwd")
		created := ""
		if first != nil {
			created = objStr(first, "timestamp")
		}
		if created == "" {
			created = tsFromName
		}
		updated := ""
		if info, err := os.Stat(file); err == nil {
			updated = formatISOTime(info.ModTime())
		}
		out = append(out, mem.MemSessionInfo{
			Platform: mem.MemSourceCodex,
			ID:       id,
			CWD:      cwd,
			Created:  created,
			Updated:  updated,
			FilePath: file,
		})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Updated != out[j].Updated {
			return out[i].Updated > out[j].Updated
		}
		return out[i].ID < out[j].ID
	})
	return out, nil, nil
}

func codexDialogue(ctx context.Context, s mem.MemSessionInfo) ([]mem.DialogueTurn, []string, error) {
	turns, _, warnings, err := codexCollect(ctx, s)
	return turns, warnings, err
}

// codexCollect 一次遍历同时产出清理后的对话轮次与任务命令事件：
// `function_call` 事件（name 为 exec_command 或 shell）里的 shell 命令
// 藏着 `sunshine task create|start` 调用，事件的 turnIndex 是调用时
// 轮次池的长度。压缩恢复出更早轮次时，已记录事件的下标随之平移，
// 窗口才能继续指向同一段对话。
func codexCollect(ctx context.Context, s mem.MemSessionInfo) ([]mem.DialogueTurn, []mem.MemEvent, []string, error) {
	pool := newCodexTurnPool()
	var events []mem.MemEvent
	var warnings []string
	encryptedInterAgent := 0
	warnings = append(warnings, ReadJSONL(s.FilePath, func(obj map[string]any) bool {
		if ctx.Err() != nil {
			return false
		}
		if objStr(obj, "type") == "compacted" {
			payload := objMap(obj, "payload")
			var recovered []mem.DialogueTurn
			if rh, ok := payload["replacement_history"].([]any); ok {
				recovered = pool.absorbRetainedHistory(rh)
			}
			if len(recovered) > 0 {
				// 恢复的轮次插到池子最前面：压缩前记录的事件下标平移。
				for i := range events {
					events[i].TurnIndex += len(recovered)
				}
				hasAssistant := false
				for _, t := range recovered {
					if t.Role == "assistant" {
						hasAssistant = true
						break
					}
				}
				if !hasAssistant {
					warnings = append(warnings, fmt.Sprintf("会话 %s：从 Codex 保留历史恢复了 %d 轮边界前对话，但其中只有用户消息——边界前的助手回复不在本文件中", s.ID, len(recovered)))
				}
				pool.push(compactionBoundaryTurn(
					fmt.Sprintf("此处上下文已压缩；已从平台保留历史恢复 %d 轮更早的对话", len(recovered)), ""))
			} else {
				pool.push(compactionBoundaryTurn(
					"此处上下文已压缩；平台保留历史未带来上方轮次之外的内容", ""))
			}
			return true
		}
		payload := objMap(obj, "payload")
		if payload == nil {
			return true
		}
		if objStr(payload, "type") == "agent_message" && objStr(obj, "type") == "response_item" {
			envelope := parseCodexAgentEnvelope(objList(payload, "content"))
			if envelope == nil {
				return true
			}
			if envelope.body == "" {
				if envelope.encrypted {
					encryptedInterAgent++
				}
				return true
			}
			cleaned := stripInjectionTags(envelope.body)
			if cleaned == "" {
				return true
			}
			role := "user"
			if envelope.kind == "FINAL_ANSWER" {
				role = "assistant"
			}
			pool.push(mem.DialogueTurn{Role: role, Text: cleaned})
			return true
		}
		if objStr(payload, "type") == "function_call" {
			fnName := objStr(payload, "name")
			if fnName != "exec_command" && fnName != "shell" {
				return true
			}
			cmd := commandFromCodexArguments(payload["arguments"])
			if cmd == "" {
				return true
			}
			events = append(events, mem.EventsFromParsed(
				mem.ParseTaskCommandsAll(cmd), objStr(obj, "timestamp"), len(pool.list))...)
			return true
		}
		if objStr(payload, "type") != "message" {
			// 未知类型一律容忍。
			return true
		}
		role := objStr(payload, "role")
		if role != "user" && role != "assistant" {
			return true
		}
		if turn := buildCodexTurn(role, objList(payload, "content")); turn != nil {
			pool.push(*turn)
		}
		return true
	})...)
	if encryptedInterAgent > 0 {
		warnings = append(warnings, fmt.Sprintf("会话 %s：%d 条智能体间消息由 Codex 加密存储，无法读回——驱动这次多智能体运行的指令无法从 rollout 文件恢复", s.ID, encryptedInterAgent))
	}
	return pool.list, events, warnings, nil
}

// buildCodexTurn 从 message 事件的 content 块里取文本：
// 只取 input_text / output_text 块，注入标签剥离，启动注入轮次丢弃。
func buildCodexTurn(role string, parts []any) *mem.DialogueTurn {
	var collected []string
	totalRaw := 0
	for _, p := range parts {
		part, ok := p.(map[string]any)
		if !ok {
			continue
		}
		typ := objStr(part, "type")
		if typ != "input_text" && typ != "output_text" {
			continue
		}
		text, ok := part["text"].(string)
		if !ok {
			continue
		}
		totalRaw += len(text)
		if cleaned := stripInjectionTags(text); cleaned != "" {
			collected = append(collected, cleaned)
		}
	}
	if len(collected) == 0 {
		return nil
	}
	merged := strings.Join(collected, "\n\n")
	if isBootstrapTurn(merged, totalRaw) {
		return nil
	}
	return &mem.DialogueTurn{Role: role, Text: merged}
}

// codexTurnPool 是带出现次数去重的轮次池：
// “ok”“继续”这类短句在一次会话里出现几十次，每次都是独立一轮，
// 普通集合会误杀；按次数去重后，保留历史里已在池子里的副本恰好
// 抵扣一次，真正的重复轮次得以保留。
type codexTurnPool struct {
	list   []mem.DialogueTurn
	counts map[string]int
}

func newCodexTurnPool() *codexTurnPool {
	return &codexTurnPool{counts: make(map[string]int)}
}

func (p *codexTurnPool) push(turn mem.DialogueTurn) {
	p.list = append(p.list, turn)
	p.bump(turn)
}

func (p *codexTurnPool) bump(turn mem.DialogueTurn) {
	if turn.Kind == "marker" {
		return
	}
	key := turnKey(turn.Role, turn.Text)
	p.counts[key]++
}

// absorbRetainedHistory 合并压缩保留历史：已在池子里的丢弃，
// 其余插到全部已收集轮次之前（保留历史按时间是本文件内容的
// 前缀）。返回恢复的轮次。
func (p *codexTurnPool) absorbRetainedHistory(items []any) []mem.DialogueTurn {
	consumed := make(map[string]int)
	var recovered []mem.DialogueTurn
	for _, item := range items {
		entry, ok := item.(map[string]any)
		if !ok || objStr(entry, "type") != "message" {
			continue
		}
		role := objStr(entry, "role")
		if role != "user" && role != "assistant" {
			continue
		}
		turn := buildCodexTurn(role, objList(entry, "content"))
		if turn == nil {
			continue
		}
		key := turnKey(turn.Role, turn.Text)
		already := p.counts[key]
		used := consumed[key]
		if used < already {
			consumed[key] = used + 1
			continue
		}
		recovered = append(recovered, *turn)
		consumed[key] = used + 1
	}
	if len(recovered) > 0 {
		p.list = append(recovered, p.list...)
		for _, turn := range recovered {
			p.bump(turn)
		}
	}
	return recovered
}

var agentEnvelopeKindPattern = regexp.MustCompile(`(?m)^Message Type:\s*(\S+)`)

type codexAgentEnvelope struct {
	kind      string
	body      string
	encrypted bool
}

// parseCodexAgentEnvelope 解析 response_item / agent_message 信封：
// input_text 块是信头（Message Type / Task name / Sender / Payload:），
// 明文载荷在 Payload: 之后，加密载荷在 sibling 的 encrypted_content 块里。
func parseCodexAgentEnvelope(parts []any) *codexAgentEnvelope {
	var header strings.Builder
	encrypted := false
	for _, p := range parts {
		part, ok := p.(map[string]any)
		if !ok {
			continue
		}
		if objStr(part, "type") == "encrypted_content" {
			encrypted = true
			continue
		}
		typ := objStr(part, "type")
		if typ != "input_text" && typ != "output_text" {
			continue
		}
		if text, ok := part["text"].(string); ok {
			header.WriteString(text)
		}
	}
	head := header.String()
	if head == "" {
		return nil
	}
	m := agentEnvelopeKindPattern.FindStringSubmatch(head)
	if m == nil {
		return nil
	}
	marker := strings.Index(head, "Payload:")
	body := ""
	if marker != -1 {
		body = strings.TrimSpace(head[marker+len("Payload:"):])
	}
	return &codexAgentEnvelope{kind: m[1], body: body, encrypted: encrypted}
}

// commandFromCodexArguments 从 Codex / Grok 的 function_call arguments 里
// 恢复 shell 命令字符串。各版本编码不一：原始 shell 字符串；
// 含 cmd / command（字符串）或 argv（字符串数组）的 JSON 字符串；
// 同形的原始对象。恢复不到时返回 ""。
func commandFromCodexArguments(argsRaw any) string {
	fromObject := func(obj map[string]any) string {
		if cmd, ok := obj["cmd"].(string); ok && cmd != "" {
			return cmd
		}
		if cmd, ok := obj["command"].(string); ok && cmd != "" {
			return cmd
		}
		if argv, ok := obj["argv"].([]any); ok {
			var parts []string
			for _, a := range argv {
				if s, ok := a.(string); ok {
					parts = append(parts, s)
				}
			}
			if len(parts) > 0 {
				return strings.Join(parts, " ")
			}
		}
		return ""
	}
	switch v := argsRaw.(type) {
	case string:
		var parsed map[string]any
		if err := json.Unmarshal([]byte(v), &parsed); err != nil {
			// 不是 JSON：某些版本直接内联原始 shell 字符串。
			return v
		}
		return fromObject(parsed)
	case map[string]any:
		return fromObject(v)
	}
	return ""
}
