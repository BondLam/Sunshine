// Package adapters 实现 Sunshine channel 的 worker 适配器：
// 把 claude / codex 两种 CLI 子进程的 JSONL 输出流解析成频道事件，
// 并把频道消息编码成写回 worker stdin 的字节。
//
// 行为对照上游源码（packages/cli/src/commands/channel/adapters/，
// commit f089cb3）：claude 用 `--input-format stream-json
// --output-format stream-json` 的 stream-json 协议；codex 用
// `app-server` 的 JSON-RPC 2.0 协议。两种协议的 JSONL 格式、工具名、
// 错误形态均不同，本包按 adapters/ 逐项实现，不混用。
//
// 与上游的两处有意偏离（见 PLAN.md 阶段 6 实施记录 D 组）：
//  1. Claude 的 user tool_result 行：上游直接跳过（嫌噪音大）；
//     本包按任务要求提取为 progress 摘要事件（截断 200 字）。
//  2. Codex 的 thread/tokenUsage/updated 通知：上游跳过；本包提取为
//     progress 事件（detail.kind 为 token_usage），以便上层统计用量。
package adapters

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/linrong1982/Sunshine/internal/channel/store"
)

// Provider 是 worker 适配器的供应商：目前只支持 claude 与 codex。
type Provider string

const (
	// ProviderClaude 是 Claude Code 适配器。
	ProviderClaude Provider = "claude"
	// ProviderCodex 是 Codex 适配器。
	ProviderCodex Provider = "codex"
)

// Providers 是全部已注册供应商（按注册顺序）。
var Providers = []Provider{ProviderClaude, ProviderCodex}

// ParseProvider 校验供应商名。
func ParseProvider(v string) (Provider, error) {
	switch Provider(v) {
	case ProviderClaude, ProviderCodex:
		return Provider(v), nil
	default:
		return "", fmt.Errorf("无效的 --provider %q，应为 claude 或 codex", v)
	}
}

// IsProvider 判断字符串是否为已注册供应商。
func IsProvider(v string) bool {
	_, err := ParseProvider(v)
	return err == nil
}

// AdapterEvent 是适配器从一行 worker 输出里解析出的频道事件。
// 适配器只决定 kind 与 payload，不分配 seq / ts / by——
// 这些由 supervisor 在追加进 events.jsonl 之前补齐。
type AdapterEvent struct {
	Kind    store.ChannelEventKind
	Payload map[string]any
}

// ResolvedRPC 是适配器解析出的已解决出站请求（codex 用）。
type ResolvedRPC struct {
	ID     int
	Result any
	Error  any
}

// SideEffect 是适配器在解析某行时请求的副作用，
// 由 supervisor 在追加事件后执行。
type SideEffect struct {
	// PersistSessionID 是要持久化的 worker 会话 id
	//（claude 的 system init 行、codex 的 thread/start 结果）。
	PersistSessionID string
	// PersistThreadID 是要持久化的 codex 线程 id（与会话 id 相同值）。
	PersistThreadID string
	// Reply 是要写回 worker stdin 的行（已带换行符，如 MCP 询问的自动应答）。
	Reply []string
	// Resolved 是已解决的待处理出站请求。
	Resolved []ResolvedRPC
}

// ParseResult 是一行解析的结果：事件与副作用。
type ParseResult struct {
	Events []AdapterEvent
	Side   *SideEffect
}

// SupervisorView 是 supervisor 传给适配器的启动视图：
// 适配器按需取 model、resume、systemPrompt 等字段。
type SupervisorView struct {
	// Resume 是恢复的会话 id（claude --resume / codex 暂不支持时忽略）。
	Resume string
	// Model 是模型覆盖（如 claude-opus-4-7、gpt-5.3-codex）。
	Model string
	// SystemPrompt 是组合后的 worker 系统提示词。
	SystemPrompt string
	// SystemPromptFile 是系统提示词文件路径（超长时用文件代替 argv 内联）。
	SystemPromptFile string
	// Cwd 是 worker 启动目录。
	Cwd string
	// Sandbox 是 codex 专用的沙箱模式（默认 workspace-write）。
	Sandbox string
}

// StdinWriter 是适配器握手期间向 worker stdin 写行的接口。
type StdinWriter interface {
	WriteLine(line string) error
}

// Adapter 是单个供应商适配器的接口：
// 启动参数、握手、就绪判断、逐行解析、消息编码。
type Adapter interface {
	// Provider 返回供应商名。
	Provider() Provider
	// BuildArgs 生成拉起 worker 子进程的 argv（不含二进制名本身）。
	BuildArgs(view SupervisorView) []string
	// CreateCtx 生成每 worker 一份的可变解析上下文（如 codex 的待处理 RPC 表）。
	CreateCtx() any
	// Handshake 在子进程拉起后、用户消息流动前做一次性握手
	//（codex：initialize + thread/start；claude：无握手，直接返回 nil）。
	Handshake(ctx context.Context, stdin StdinWriter, c any, view SupervisorView) error
	// IsReady 判断适配器是否已能接受用户消息
	//（codex：thread/start 已拿到 threadId；claude：拉起后即就绪）。
	IsReady(c any) bool
	// ParseLine 解析一行 worker stdout，返回频道事件与副作用。纯函数。
	ParseLine(line string, c any) ParseResult
	// EncodeUserMessage 把频道消息编码成写进 worker stdin 的字节。
	EncodeUserMessage(text string, c any) (string, error)
	// EncodeInterruptMessage 编码中断重定向：先发供应商级中断控制帧
	//（如有），再发替换指令。
	EncodeInterruptMessage(text string, c any) (string, error)
}

// registry 是供应商注册表：加新适配器只需在此注册一行。
var registry = map[Provider]Adapter{
	ProviderClaude: claudeAdapter{},
	ProviderCodex:  codexAdapter{},
}

// GetAdapter 按供应商名取适配器；未注册时报错。
func GetAdapter(provider Provider) (Adapter, error) {
	a, ok := registry[provider]
	if !ok {
		return nil, fmt.Errorf("未知的供应商 %q（已注册：claude、codex）", string(provider))
	}
	return a, nil
}

// ListProviders 返回已注册供应商名。
func ListProviders() []Provider {
	out := make([]Provider, len(Providers))
	copy(out, Providers)
	return out
}

// summarizeInput 把工具入参摘要成短字符串（默认上限 120 字），
// 超长截断并加省略号。
func summarizeInput(input any, max int) string {
	if input == nil {
		return ""
	}
	var s string
	switch v := input.(type) {
	case string:
		s = v
	default:
		b, err := json.Marshal(input)
		if err != nil {
			s = fmt.Sprint(input)
		} else {
			s = string(b)
		}
	}
	if max > 0 && len([]rune(s)) > max {
		return string([]rune(s)[:max]) + "…"
	}
	return s
}

// isMCPToolName 判断是否为 MCP 工具名（mcp__<server>__<tool>）。
func isMCPToolName(name string) bool {
	return strings.HasPrefix(name, "mcp__")
}
