// Claude `--input-format stream-json --output-format stream-json` 适配器。
//
// 真实行形态（见上游研究探针
// research/probes/claude/list-files.jsonl）：
//   - system.subtype=hook_started / hook_response → 跳过（hook 生命周期）
//   - system.subtype=init                        → 持久化 session_id，不发事件
//   - assistant.message.content[]               → 每块：text → message 事件，
//     tool_use → progress 事件，thinking → 跳过
//   - user.message.content[]                    → tool_result → progress 摘要事件
//     （上游跳过；本包按任务要求提取，见包文档）
//   - rate_limit_event                          → 跳过
//   - result                                    → done（成功）或 error
//   - control_response                          → 跳过（中断控制请求的确认）
//
// token 用量：assistant 行的 message.usage 与 result 行的 usage
// 一并带进事件 payload（上游不提取；本包按任务要求提取）。
package adapters

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/linrong007/Sunshine/internal/channel/store"
)

// InlineSystemPromptMaxChars 是系统提示词内联在 argv 里的长度上限：
// 小于等于该长度走 `--append-system-prompt`，超长走
// `--append-system-prompt-file`（Windows CreateProcess 命令行上限
// 32767 字、Linux 单个参数上限 128KiB，超长内联会 spawn 失败）。
const InlineSystemPromptMaxChars = 8000

// ShouldUseSystemPromptFile 判断系统提示词是否大到必须走文件
// 而不是 argv 内联（非空且超长时为真）。
func ShouldUseSystemPromptFile(systemPrompt string) bool {
	return strings.TrimSpace(systemPrompt) != "" && len(systemPrompt) > InlineSystemPromptMaxChars
}

type claudeRawMsg struct {
	Type         string `json:"type"`
	Subtype      string `json:"subtype"`
	SessionID    string `json:"session_id"`
	Message      *claudeMessage
	Result       string         `json:"result"`
	IsError      bool           `json:"is_error"`
	DurationMs   *int64         `json:"duration_ms"`
	TotalCostUSD *float64       `json:"total_cost_usd"`
	NumTurns     *int           `json:"num_turns"`
	Usage        map[string]any `json:"usage"`
}

type claudeMessage struct {
	Role    string         `json:"role"`
	Model   string         `json:"model"`
	Content []claudeBlock  `json:"content"`
	Usage   map[string]any `json:"usage"`
}

type claudeBlock struct {
	Type      string `json:"type"`
	Text      string `json:"text"`
	Thinking  string `json:"thinking"`
	Name      string `json:"name"`
	ID        string `json:"id"`
	Input     any    `json:"input"`
	ToolUseID string `json:"tool_use_id"`
	Content   any    `json:"content"`
	IsError   *bool  `json:"is_error"`
}

// ParseClaudeLine 解析一行 Claude stream-json stdout，
// 返回要追加的频道事件与副作用。纯函数：相同输入总产生相同输出。
func ParseClaudeLine(line string) ParseResult {
	trimmed := strings.TrimSpace(line)
	if trimmed == "" {
		return ParseResult{}
	}
	var msg claudeRawMsg
	if err := json.Unmarshal([]byte(trimmed), &msg); err != nil {
		return ParseResult{Events: []AdapterEvent{{
			Kind: store.KindError,
			Payload: map[string]any{
				"message":     "解析 Claude 输出行失败",
				"raw_excerpt": firstRunes(trimmed, 200),
			},
		}}}
	}
	switch msg.Type {
	case "system":
		return handleClaudeSystem(&msg)
	case "assistant":
		return handleClaudeAssistant(&msg)
	case "user":
		return handleClaudeUser(&msg)
	case "rate_limit_event":
		return ParseResult{}
	case "result":
		return handleClaudeResult(&msg)
	case "control_response":
		// 对出站 control_request（如中断）的确认，静默消费。
		return ParseResult{}
	default:
		return ParseResult{}
	}
}

func handleClaudeSystem(msg *claudeRawMsg) ParseResult {
	if msg.Subtype == "init" && msg.SessionID != "" {
		return ParseResult{Side: &SideEffect{PersistSessionID: msg.SessionID}}
	}
	return ParseResult{}
}

func handleClaudeAssistant(msg *claudeRawMsg) ParseResult {
	if msg.Message == nil {
		return ParseResult{}
	}
	var events []AdapterEvent
	for _, b := range msg.Message.Content {
		switch b.Type {
		case "text":
			if b.Text != "" {
				payload := map[string]any{"text": b.Text}
				if usage := claudeUsage(msg); usage != nil {
					payload["usage"] = usage
				}
				events = append(events, AdapterEvent{Kind: store.KindMessage, Payload: payload})
			}
		case "tool_use":
			detail := map[string]any{
				"tool":          b.Name,
				"input_summary": summarizeInput(b.Input, 120),
			}
			if isMCPToolName(b.Name) {
				detail["kind"] = "mcp"
				parts := strings.Split(b.Name, "__")
				if len(parts) >= 3 {
					detail["server"] = parts[1]
					detail["tool_name"] = strings.Join(parts[2:], "__")
				}
			}
			events = append(events, AdapterEvent{
				Kind:    store.KindProgress,
				Payload: map[string]any{"detail": detail},
			})
		case "thinking":
			// 跳过（只在详细模式才需要）。
		default:
			// 未知块类型，静默跳过。
		}
	}
	return ParseResult{Events: events}
}

// handleClaudeUser 处理 user 行：只提取 tool_result 为 progress 摘要事件。
// 上游直接跳过整行（嫌噪音大）；本包按任务要求提取 tool_use/tool_result，
// 内容截断为 200 字摘要，不把完整输出写进事件日志。
func handleClaudeUser(msg *claudeRawMsg) ParseResult {
	if msg.Message == nil {
		return ParseResult{}
	}
	var events []AdapterEvent
	for _, b := range msg.Message.Content {
		if b.Type != "tool_result" {
			continue
		}
		detail := map[string]any{
			"tool_result":    true,
			"tool_use_id":    b.ToolUseID,
			"output_summary": summarizeInput(b.Content, 200),
		}
		if b.IsError != nil {
			detail["is_error"] = *b.IsError
		}
		events = append(events, AdapterEvent{
			Kind:    store.KindProgress,
			Payload: map[string]any{"detail": detail},
		})
	}
	return ParseResult{Events: events}
}

func handleClaudeResult(msg *claudeRawMsg) ParseResult {
	if msg.IsError {
		payload := map[string]any{
			"message": firstNonEmpty(msg.Result, "Claude 报告 is_error"),
		}
		if msg.DurationMs != nil {
			payload["duration_ms"] = *msg.DurationMs
		}
		return ParseResult{Events: []AdapterEvent{{Kind: store.KindError, Payload: payload}}}
	}
	// 注意：不把 msg.Result 复制进 done.text。Claude 已把最终文本
	// 作为 assistant 文本块发出（已转为 kind:message 事件），
	// 再复制一次会让消费方把答案渲染两遍。
	payload := map[string]any{}
	if msg.DurationMs != nil {
		payload["duration_ms"] = *msg.DurationMs
	}
	if msg.TotalCostUSD != nil {
		payload["total_cost_usd"] = *msg.TotalCostUSD
	}
	if msg.NumTurns != nil {
		payload["num_turns"] = *msg.NumTurns
	}
	if msg.Usage != nil {
		payload["usage"] = msg.Usage
	}
	return ParseResult{Events: []AdapterEvent{{Kind: store.KindDone, Payload: payload}}}
}

// claudeUsage 取 assistant 行的 token 用量（message.usage 优先，
// 顶层 usage 兜底）；无用量时返回 nil。
func claudeUsage(msg *claudeRawMsg) map[string]any {
	if msg.Message != nil && len(msg.Message.Usage) > 0 {
		return msg.Message.Usage
	}
	if len(msg.Usage) > 0 {
		return msg.Usage
	}
	return nil
}

// EncodeClaudeUserMessage 把频道消息编码成 Claude stream-json stdin 行。
func EncodeClaudeUserMessage(text string) string {
	line, _ := json.Marshal(map[string]any{
		"type": "user",
		"message": map[string]any{
			"role":    "user",
			"content": []any{map[string]any{"type": "text", "text": text}},
		},
	})
	return string(line) + "\n"
}

// EncodeClaudeInterruptMessage 编码 Claude 中断：先发供应商级
// control_request 中断帧，再发替换指令（与上游一致，
// 中断是显式命令路径，与频道消息元数据分离）。
func EncodeClaudeInterruptMessage(text string) string {
	var nonce [4]byte
	_, _ = rand.Read(nonce[:])
	reqID := fmt.Sprintf("sunshine-int-%d-%s", time.Now().UnixNano(), hex.EncodeToString(nonce[:]))
	ctrl, _ := json.Marshal(map[string]any{
		"type":       "control_request",
		"request_id": reqID,
		"request":    map[string]any{"subtype": "interrupt"},
	})
	return string(ctrl) + "\n" + strings.TrimRight(EncodeClaudeUserMessage(text), "\n") + "\n"
}

// ClaudeArgs 是 BuildClaudeArgs 的选项。
type ClaudeArgs struct {
	// Resume 是恢复的会话 id（--resume）。
	Resume string
	// Model 是模型覆盖（--model）。
	Model string
	// SystemPrompt 是追加到 Claude 默认系统提示词后的文本。
	SystemPrompt string
	// SystemPromptFile 是系统提示词文件路径，优先于 SystemPrompt。
	SystemPromptFile string
}

// BuildClaudeArgs 生成 `claude -p` 的 stream-json 模式参数。
func BuildClaudeArgs(opts ClaudeArgs) []string {
	args := []string{
		"-p",
		"--input-format", "stream-json",
		"--output-format", "stream-json",
		"--permission-mode", "bypassPermissions",
		"--dangerously-skip-permissions",
		"--verbose",
	}
	if opts.Resume != "" {
		args = append(args, "--resume", opts.Resume)
	}
	if opts.Model != "" {
		args = append(args, "--model", opts.Model)
	}
	if opts.SystemPromptFile != "" {
		args = append(args, "--append-system-prompt-file", opts.SystemPromptFile)
	} else if strings.TrimSpace(opts.SystemPrompt) != "" {
		args = append(args, "--append-system-prompt", opts.SystemPrompt)
	}
	return args
}

// claudeAdapter 是 Claude 适配器：stream-json，无握手，拉起后即就绪。
type claudeAdapter struct{}

func (claudeAdapter) Provider() Provider { return ProviderClaude }

func (claudeAdapter) BuildArgs(view SupervisorView) []string {
	return BuildClaudeArgs(ClaudeArgs{
		Resume:           view.Resume,
		Model:            view.Model,
		SystemPrompt:     view.SystemPrompt,
		SystemPromptFile: view.SystemPromptFile,
	})
}

func (claudeAdapter) CreateCtx() any { return nil }

func (claudeAdapter) Handshake(ctx context.Context, stdin StdinWriter, c any, view SupervisorView) error {
	return nil
}

func (claudeAdapter) IsReady(c any) bool { return true }

func (claudeAdapter) ParseLine(line string, c any) ParseResult {
	return ParseClaudeLine(line)
}

func (claudeAdapter) EncodeUserMessage(text string, c any) (string, error) {
	return EncodeClaudeUserMessage(text), nil
}

func (claudeAdapter) EncodeInterruptMessage(text string, c any) (string, error) {
	return EncodeClaudeInterruptMessage(text), nil
}

func firstRunes(s string, n int) string {
	r := []rune(s)
	if len(r) > n {
		return string(r[:n])
	}
	return s
}

func firstNonEmpty(s, fallback string) string {
	if strings.TrimSpace(s) != "" {
		return s
	}
	return fallback
}
