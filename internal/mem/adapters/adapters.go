// Package adapters 实现 7 个平台的会话适配器（只读）。
package adapters

import (
	"context"

	"github.com/BondLam/Sunshine/internal/mem"
)

// ClaudeAdapter 读 Claude Code 会话（~/.claude/projects，JSONL）。
type ClaudeAdapter struct{}

// Kind 返回平台标识。
func (ClaudeAdapter) Kind() mem.MemSourceKind { return mem.MemSourceClaude }

// Sessions 列出 Claude 会话。
func (ClaudeAdapter) Sessions(ctx context.Context, home string) ([]mem.MemSessionInfo, []string, error) {
	return claudeSessions(ctx, home)
}

// Dialogue 读出一次 Claude 会话的对话。
func (ClaudeAdapter) Dialogue(ctx context.Context, s mem.MemSessionInfo) ([]mem.DialogueTurn, []string, error) {
	return claudeDialogue(ctx, s)
}

// Collect 一次读出对话轮次与任务命令事件（与 Dialogue 同源）。
func (ClaudeAdapter) Collect(ctx context.Context, s mem.MemSessionInfo) ([]mem.DialogueTurn, []mem.MemEvent, []string, error) {
	return claudeCollect(ctx, s)
}

// CodexAdapter 读 Codex 会话（~/.codex/sessions，JSONL）。
type CodexAdapter struct{}

// Kind 返回平台标识。
func (CodexAdapter) Kind() mem.MemSourceKind { return mem.MemSourceCodex }

// Sessions 列出 Codex 会话。
func (CodexAdapter) Sessions(ctx context.Context, home string) ([]mem.MemSessionInfo, []string, error) {
	return codexSessions(ctx, home)
}

// Dialogue 读出一次 Codex 会话的对话。
func (CodexAdapter) Dialogue(ctx context.Context, s mem.MemSessionInfo) ([]mem.DialogueTurn, []string, error) {
	return codexDialogue(ctx, s)
}

// Collect 一次读出对话轮次与任务命令事件（与 Dialogue 同源）。
func (CodexAdapter) Collect(ctx context.Context, s mem.MemSessionInfo) ([]mem.DialogueTurn, []mem.MemEvent, []string, error) {
	return codexCollect(ctx, s)
}

// DevinAdapter 读 Devin CLI 会话（sessions.db，SQLite 只读）。
type DevinAdapter struct{}

// Kind 返回平台标识。
func (DevinAdapter) Kind() mem.MemSourceKind { return mem.MemSourceDevin }

// Sessions 列出 Devin 会话。
func (DevinAdapter) Sessions(ctx context.Context, home string) ([]mem.MemSessionInfo, []string, error) {
	return devinSessions(ctx, home)
}

// Dialogue 读出一次 Devin 会话的对话。
func (DevinAdapter) Dialogue(ctx context.Context, s mem.MemSessionInfo) ([]mem.DialogueTurn, []string, error) {
	return devinDialogue(ctx, s)
}

// Collect 一次读出对话轮次与任务命令事件（与 Dialogue 同源）。
func (DevinAdapter) Collect(ctx context.Context, s mem.MemSessionInfo) ([]mem.DialogueTurn, []mem.MemEvent, []string, error) {
	return devinCollect(ctx, s)
}

// GrokAdapter 读 Grok CLI 会话（~/.grok/sessions，JSONL）。
type GrokAdapter struct{}

// Kind 返回平台标识。
func (GrokAdapter) Kind() mem.MemSourceKind { return mem.MemSourceGrok }

// Sessions 列出 Grok 会话。
func (GrokAdapter) Sessions(ctx context.Context, home string) ([]mem.MemSessionInfo, []string, error) {
	return grokSessions(ctx, home)
}

// Dialogue 读出一次 Grok 会话的对话。
func (GrokAdapter) Dialogue(ctx context.Context, s mem.MemSessionInfo) ([]mem.DialogueTurn, []string, error) {
	return grokDialogue(ctx, s)
}

// Collect 一次读出对话轮次与任务命令事件（与 Dialogue 同源）。
func (GrokAdapter) Collect(ctx context.Context, s mem.MemSessionInfo) ([]mem.DialogueTurn, []mem.MemEvent, []string, error) {
	return grokCollect(ctx, s)
}

// OpencodeAdapter 读 OpenCode 会话（opencode.db，SQLite 只读）。
type OpencodeAdapter struct{}

// Kind 返回平台标识。
func (OpencodeAdapter) Kind() mem.MemSourceKind { return mem.MemSourceOpencode }

// Sessions 列出 OpenCode 会话。
func (OpencodeAdapter) Sessions(ctx context.Context, home string) ([]mem.MemSessionInfo, []string, error) {
	return opencodeSessions(ctx, home)
}

// Dialogue 读出一次 OpenCode 会话的对话。
func (OpencodeAdapter) Dialogue(ctx context.Context, s mem.MemSessionInfo) ([]mem.DialogueTurn, []string, error) {
	return opencodeDialogue(ctx, s)
}

// Collect 一次读出对话轮次与任务命令事件（与 Dialogue 同源）。
func (OpencodeAdapter) Collect(ctx context.Context, s mem.MemSessionInfo) ([]mem.DialogueTurn, []mem.MemEvent, []string, error) {
	return opencodeCollect(ctx, s)
}

// PiAdapter 读 Pi Agent 会话（~/.pi/agent/sessions，JSONL）。
type PiAdapter struct{}

// Kind 返回平台标识。
func (PiAdapter) Kind() mem.MemSourceKind { return mem.MemSourcePi }

// Sessions 列出 Pi 会话。
func (PiAdapter) Sessions(ctx context.Context, home string) ([]mem.MemSessionInfo, []string, error) {
	return piSessions(ctx, home)
}

// Dialogue 读出一次 Pi 会话的对话。
func (PiAdapter) Dialogue(ctx context.Context, s mem.MemSessionInfo) ([]mem.DialogueTurn, []string, error) {
	return piDialogue(ctx, s)
}

// Collect 一次读出对话轮次与任务命令事件（与 Dialogue 同源）。
func (PiAdapter) Collect(ctx context.Context, s mem.MemSessionInfo) ([]mem.DialogueTurn, []mem.MemEvent, []string, error) {
	return piCollect(ctx, s)
}

// ZcodeAdapter 读 ZCode 会话（~/.zcode/cli/db/db.sqlite，SQLite 只读）。
type ZcodeAdapter struct{}

// Kind 返回平台标识。
func (ZcodeAdapter) Kind() mem.MemSourceKind { return mem.MemSourceZcode }

// Sessions 列出 ZCode 会话。
func (ZcodeAdapter) Sessions(ctx context.Context, home string) ([]mem.MemSessionInfo, []string, error) {
	return zcodeSessions(ctx, home)
}

// Dialogue 读出一次 ZCode 会话的对话。
func (ZcodeAdapter) Dialogue(ctx context.Context, s mem.MemSessionInfo) ([]mem.DialogueTurn, []string, error) {
	return zcodeDialogue(ctx, s)
}

// Collect 一次读出对话轮次与任务命令事件（与 Dialogue 同源）。
func (ZcodeAdapter) Collect(ctx context.Context, s mem.MemSessionInfo) ([]mem.DialogueTurn, []mem.MemEvent, []string, error) {
	return zcodeCollect(ctx, s)
}
