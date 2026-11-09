package hook

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// RunShellSession 实现 sunshine hook shell-session，对应
// shared-hooks/inject-shell-session-context.py（beforeShellExecution /
// PreToolUse / BeforeTool）。
//
// 待执行命令调用任务命令（Go 版：`sunshine task start/current/finish`，
// 阶段 3 实现；兼容旧的 `task.py start/current/finish` 写法）时，
// 在 .sunshine/.runtime/shell-tickets/ 写一张 TTL 30 秒的票据，
// 把会话身份桥接给子进程；shell 执行类事件返回 {"permission":"allow"}。
// 不调用任务命令、或无会话身份、或无 .sunshine/ 时静默退出 0，不输出。
func RunShellSession(r io.Reader, opts Options) error {
	if HooksDisabled() {
		return nil
	}
	input := ReadInput(r)
	command, response := pendingShellCommand(input)
	subcommands := extractTaskSubcommands(command)
	if len(subcommands) == 0 {
		return nil
	}
	root := ResolveProjectRootMulti(input)
	if root == "" {
		return nil
	}
	if !hasContextIdentity(input) {
		return nil
	}
	platform := DetectPlatform(input)
	contextKey := ResolveContextKey(input, platform)
	if contextKey == "" {
		return nil
	}
	if err := writeTicket(root, input, contextKey, command, platform, subcommands); err != nil {
		return nil
	}
	if response != nil {
		WriteJSON(opts.out(), response)
	}
	return nil
}

// ShellTicketTTLSeconds 是票据有效期（秒），与 Python 的 TICKET_TTL_SECONDS 一致。
const ShellTicketTTLSeconds = 30

// contextIdentityKeys 是判定“有会话身份”的顶层字段（Python 的
// CONTEXT_IDENTITY_KEYS 原样）。
var contextIdentityKeys = []string{
	"session_id", "sessionId", "sessionID",
	"conversation_id", "conversationId", "conversationID",
	"transcript_path", "transcriptPath", "transcript",
}

func hasContextIdentity(input HookInput) bool {
	for _, k := range contextIdentityKeys {
		if s, ok := input[k].(string); ok && strings.TrimSpace(s) != "" {
			return true
		}
	}
	return false
}

// pendingShellCommand 从两种 hook 载荷形状里读待执行命令：
//   - shell 执行事件（Cursor beforeShellExecution）：命令在顶层，
//     宿主希望收到明确的放行答复；
//   - 工具调用事件（PreToolUse / BeforeTool）：命令在 tool_input 下，
//     宿主读的是另一套 schema，这里不作答。
//
// 两种形状都没有时返回 ("", nil)，调用方直接无操作。
func pendingShellCommand(input HookInput) (string, map[string]any) {
	if cmd := GetString(input, "command"); cmd != "" {
		return cmd, map[string]any{"permission": "allow"}
	}
	for _, key := range []string{"tool_input", "toolInput"} {
		if ti, ok := input[key].(map[string]any); ok {
			if cmd := GetString(ti, "command"); cmd != "" {
				return cmd, nil
			}
		}
	}
	return "", nil
}

// extractTaskSubcommands 从命令里找出任务子命令。Go 版命令形如
// `sunshine task start <任务>` / `sunshine task current` /
// `sunshine task finish`；旧的 `task.py start/...` 写法也接受
// （过渡期兼容）。start 后紧跟的 token 是任务引用。
// 分词是 POSIX shell 风格（单/双引号与反斜杠转义），
// 分词失败返回空（Python 的 shlex.split 出错时同样返回空）。
func extractTaskSubcommands(command string) []TicketCommand {
	tokens, err := shlexSplit(command)
	if err != nil {
		return nil
	}
	var out []TicketCommand
	for i := 0; i+1 < len(tokens); i++ {
		isSunshineTask := tokens[i] == "sunshine" && i+2 < len(tokens) && tokens[i+1] == "task"
		isTaskPy := filepath.Base(strings.Trim(tokens[i], `"'`)) == "task.py"
		var name string
		var next int
		switch {
		case isSunshineTask:
			name = tokens[i+2]
			next = i + 2
		case isTaskPy:
			name = tokens[i+1]
			next = i + 1
		default:
			continue
		}
		if name != "start" && name != "current" && name != "finish" {
			continue
		}
		item := TicketCommand{Name: name}
		if name == "start" && next+1 < len(tokens) {
			item.TaskRef = tokens[next+1]
		}
		out = append(out, item)
	}
	return out
}

// shlexSplit 是 POSIX shell 风格的分词（单引号原样、双引号内
// 允许转义、反斜杠转义下一字符、空白分隔）。引号未闭合返回错误。
func shlexSplit(s string) ([]string, error) {
	var tokens []string
	var cur strings.Builder
	var quote byte
	escaped := false
	hasToken := false
	for i := 0; i < len(s); i++ {
		c := s[i]
		if escaped {
			cur.WriteByte(c)
			escaped = false
			hasToken = true
			continue
		}
		if quote == '\'' {
			if c == '\'' {
				quote = 0
			} else {
				cur.WriteByte(c)
			}
			hasToken = true
			continue
		}
		if quote == '"' {
			switch c {
			case '"':
				quote = 0
			case '\\':
				escaped = true
			default:
				cur.WriteByte(c)
			}
			hasToken = true
			continue
		}
		switch c {
		case '\\':
			escaped = true
		case '\'', '"':
			quote = c
			hasToken = true
		case ' ', '\t', '\n', '\r':
			if hasToken {
				tokens = append(tokens, cur.String())
				cur.Reset()
				hasToken = false
			}
		default:
			cur.WriteByte(c)
			hasToken = true
		}
	}
	if escaped || quote != 0 {
		return nil, fmt.Errorf("引号未闭合")
	}
	if hasToken {
		tokens = append(tokens, cur.String())
	}
	return tokens, nil
}

// writeTicket 写一张 shell 票据。文件名取
// `<毫秒时间戳>-<sha256(context_key\0command\0now) 前 16 位 hex>.json`，
// 与 Python 的命名规则一致；写前清理已过期的票据。
func writeTicket(root string, input HookInput, contextKey, command, platform string, subcommands []TicketCommand) error {
	now := float64(time.Now().UnixNano()) / 1e9
	ticketDir := filepath.Join(root, ".sunshine", ".runtime", "shell-tickets")
	if err := os.MkdirAll(ticketDir, 0o755); err != nil {
		return err
	}
	cleanupExpiredTickets(ticketDir, now)
	digest := sha256.Sum256([]byte(contextKey + "\x00" + command + "\x00" + fmt.Sprintf("%v", now)))
	name := fmt.Sprintf("%d-%s.json", time.Now().UnixMilli(), hex.EncodeToString(digest[:])[:16])
	ticket := ShellTicket{
		Platform:       platform,
		ContextKey:     contextKey,
		ConversationID: GetString(input, "conversation_id"),
		SessionID:      GetString(input, "session_id"),
		GenerationID:   GetString(input, "generation_id"),
		CWD:            root,
		HostCWD:        GetString(input, "cwd"),
		Command:        command,
		Subcommands:    subcommands,
		CreatedAtEpoch: now,
		ExpiresAtEpoch: now + ShellTicketTTLSeconds,
	}
	data, err := json.MarshalIndent(ticket, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(ticketDir, name), append(data, '\n'), 0o644)
}

// cleanupExpiredTickets 删掉票据目录里已过期的票据；
// 损坏的票据文件跳过（不删，留给消费方按“不新鲜”丢弃）。
func cleanupExpiredTickets(ticketDir string, now float64) {
	entries, err := os.ReadDir(ticketDir)
	if err != nil {
		return
	}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		path := filepath.Join(ticketDir, e.Name())
		data := ReadJSONFile(path)
		if data == nil {
			continue
		}
		if exp, ok := data["expires_at_epoch"].(float64); ok && exp < now {
			_ = os.Remove(path)
		}
	}
}
