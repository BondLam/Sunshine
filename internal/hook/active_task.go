package hook

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

// ActiveTask 是解析出的当前任务状态，对应 active_task.ActiveTask。
type ActiveTask struct {
	// TaskPath 是任务引用（如 .sunshine/tasks/11-09-xxx），为空表示无任务。
	TaskPath string
	// SourceType 是 "session" / "session-fallback" / "none"。
	SourceType string
	// ContextKey 是会话键（如 claude_abc123），可能为空。
	ContextKey string
	// Stale 表示指针存在但任务目录已不存在。
	Stale bool
}

// Source 是人类可读的来源标签：session:<key> / session-fallback:<key> / none。
func (a ActiveTask) Source() string {
	switch a.SourceType {
	case "session":
		if a.ContextKey != "" {
			return "session:" + a.ContextKey
		}
	case "session-fallback":
		if a.ContextKey != "" {
			return "session-fallback:" + a.ContextKey
		}
	}
	return a.SourceType
}

var (
	sessionKeys      = []string{"session_id", "sessionId", "sessionID"}
	conversationKeys = []string{"conversation_id", "conversationId", "conversationID"}
	transcriptKeys   = []string{"transcript_path", "transcriptPath", "transcript"}
	nestedKeys       = []string{"input", "properties", "event", "hook_input", "hookInput"}
)

// envSessionKeys 是经核实的平台会话环境变量表（active_task.py 的
// _ENV_SESSION_KEYS，含注释里的核实状态；未核实的平台故意不在表里）。
var envSessionKeys = []struct {
	platform string
	keys     []string
}{
	{"dsh", []string{"DSH_SESSION_ID"}},
	{"claude", []string{"CLAUDE_CODE_SESSION_ID"}},
	{"codex", []string{"CODEX_THREAD_ID"}},
	{"gemini", []string{"GEMINI_SESSION_ID"}},
	{"qoder", []string{"QODER_SESSION_ID"}},
	{"kiro", []string{"KIRO_SESSION_ID"}},
	{"copilot", []string{"COPILOT_SESSION_ID", "COPILOT_SESSIONID"}},
	{"zcode", []string{"CLAUDE_CODE_SESSION_ID", "CLAUDE_SESSION_ID"}},
	{"snow", []string{"SNOW_SESSION_ID"}},
}

var envConversationKeys = []struct {
	platform string
	keys     []string
}{
	{"cursor", []string{"CURSOR_CONVERSATION_ID", "CURSOR_CONVERSATIONID"}},
}

var envTranscriptKeys = []struct {
	platform string
	keys     []string
}{
	{"cursor", []string{"CURSOR_TRANSCRIPT_PATH"}},
	{"gemini", []string{"GEMINI_TRANSCRIPT_PATH"}},
	{"droid", []string{"FACTORY_TRANSCRIPT_PATH", "DROID_TRANSCRIPT_PATH"}},
	{"qoder", []string{"QODER_TRANSCRIPT_PATH"}},
	{"codebuddy", []string{"CODEBUDDY_TRANSCRIPT_PATH"}},
}

var contextKeyPlatformAliases = map[string]string{
	"zcode":   "claude",
	"factory": "droid",
}

var envPlatformAliases = map[string]string{
	"claude-code":    "claude",
	"factory":        "droid",
	"factory-ai":     "droid",
	"github-copilot": "copilot",
}

var sanitizeRe = regexp.MustCompile(`[^A-Za-z0-9._-]+`)

func sanitizeKey(raw string) string {
	safe := sanitizeRe.ReplaceAllString(strings.TrimSpace(raw), "_")
	safe = strings.Trim(safe, "._-")
	if len(safe) > 160 {
		safe = safe[:160]
	}
	return safe
}

func hashValue(raw string) string {
	sum := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(sum[:])[:24]
}

func contextKey(platform, kind, value string) string {
	if alias, ok := contextKeyPlatformAliases[platform]; ok {
		platform = alias
	}
	if kind == "transcript" {
		return platform + "_transcript_" + hashValue(value)
	}
	if safe := sanitizeKey(value); safe != "" {
		return platform + "_" + safe
	}
	return platform + "_" + hashValue(value)
}

// detectContextPlatform 对应 _detect_platform：显式 platform 参数优先，
// 其次 input 里的平台字段，再看 cursor_version，都没有则为 "session"。
func detectContextPlatform(input HookInput, platform string) string {
	if platform != "" {
		return sanitizeKey(platform)
	}
	if input != nil {
		for _, k := range []string{"_trellis_platform", "trellis_platform", "platform", "source"} {
			if v := GetString(input, k); v != "" {
				if s := sanitizeKey(v); s != "" {
					return s
				}
				return "session"
			}
		}
		if _, ok := input["cursor_version"].(string); ok {
			return "cursor"
		}
	}
	return "session"
}

// ResolveContextKey 解析稳定的会话键，对应 resolve_context_key。
// 顺序：SUNSHINE_CONTEXT_ID（兼容旧名 TRELLIS_CONTEXT_ID）显式覆盖 →
// hook JSON 的 session/conversation/transcript id → 平台环境变量。
//
// 偏差：Python 最后还会查 shell 票据（_lookup_shell_ticket_context_key），
// 但那一步要求调用者就是 task.py 本身（argv[0] 名为 task.py 且参数
// 匹配），在 hook 进程里恒为假；Go 版 hook 因此不查票据，票据只由
// 阶段 3 的 sunshine task 命令消费。
func ResolveContextKey(input HookInput, platform string) string {
	return resolveContextKey(input, platform, true)
}

func resolveContextKey(input HookInput, platform string, allowEnv bool) string {
	if allowEnv {
		if override := strings.TrimSpace(os.Getenv("SUNSHINE_CONTEXT_ID")); override != "" {
			if s := sanitizeKey(override); s != "" {
				return s
			}
			return hashValue(override)
		}
		if override := strings.TrimSpace(os.Getenv("TRELLIS_CONTEXT_ID")); override != "" {
			if s := sanitizeKey(override); s != "" {
				return s
			}
			return hashValue(override)
		}
	}
	platformName := ""
	if input != nil || platform != "" {
		platformName = detectContextPlatform(input, platform)
	}
	if input != nil {
		if v := GetNested(input, sessionKeys...); v != "" {
			return contextKey(platformName, "session", v)
		}
		if v := GetNested(input, conversationKeys...); v != "" {
			return contextKey(platformName, "conversation", v)
		}
		if v := GetNested(input, transcriptKeys...); v != "" {
			return contextKey(platformName, "transcript", v)
		}
	}
	if allowEnv {
		if key := lookupEnvContextKey(platformName); key != "" {
			return key
		}
	}
	return ""
}

func lookupEnvContextKey(platformName string) string {
	envPlatform := ""
	if platformName != "" && platformName != "session" {
		envPlatform = platformName
		if alias, ok := envPlatformAliases[envPlatform]; ok {
			envPlatform = alias
		}
	}
	// session 类
	for _, entry := range envSessionKeys {
		if envPlatform != "" && entry.platform != envPlatform {
			continue
		}
		for _, k := range entry.keys {
			if v := strings.TrimSpace(os.Getenv(k)); v != "" {
				return contextKey(entry.platform, "session", v)
			}
		}
	}
	for _, entry := range envConversationKeys {
		if envPlatform != "" && entry.platform != envPlatform {
			continue
		}
		for _, k := range entry.keys {
			if v := strings.TrimSpace(os.Getenv(k)); v != "" {
				return contextKey(entry.platform, "conversation", v)
			}
		}
	}
	for _, entry := range envTranscriptKeys {
		if envPlatform != "" && entry.platform != envPlatform {
			continue
		}
		for _, k := range entry.keys {
			if v := strings.TrimSpace(os.Getenv(k)); v != "" {
				return contextKey(entry.platform, "transcript", v)
			}
		}
	}
	return ""
}

// ResolveOptions 控制 ResolveActiveTask 的回退行为。
type ResolveOptions struct {
	// AllowSingleSessionFallback：唯一会话文件回退（子代理等拿不到父
	// 身份的调用方才开；Codex 原生 SubagentStart 必须关）。
	AllowSingleSessionFallback bool
	// AllowEnvironmentContext：是否允许环境变量参与解析（Codex 原生
	// SubagentStart 必须关，避免借到别的窗口的任务）。
	AllowEnvironmentContext bool
}

// ResolveActiveTask 解析当前任务，对应 resolve_active_task：只读
// .sunshine/.runtime/sessions/<context_key>.json 里的 current_task。
// 指针存在但目录不存在时返回 Stale=true 的任务；无键或无指针时，
// 视 AllowSingleSessionFallback 决定是否用唯一会话文件回退，
// 否则返回无任务。
func ResolveActiveTask(repoRoot string, input HookInput, platform string, opts ResolveOptions) ActiveTask {
	allowEnv := opts.AllowEnvironmentContext
	// 调用方须显式传 AllowEnvironmentContext：除 Codex 原生
	// SubagentStart（必须关）外一律传 true；零值即关。
	contextKey := resolveContextKey(input, platform, allowEnv)
	if contextKey != "" {
		if active := activeFromSessionFile(repoRoot, contextKey, "session"); active != nil {
			return *active
		}
	}
	if opts.AllowSingleSessionFallback {
		if fb := singleSessionFallback(repoRoot); fb != nil {
			return *fb
		}
	}
	return ActiveTask{SourceType: "none", ContextKey: contextKey}
}

func activeFromSessionFile(repoRoot, contextKey, sourceType string) *ActiveTask {
	path := filepath.Join(repoRoot, ".sunshine", ".runtime", "sessions", contextKey+".json")
	data := ReadJSONFile(path)
	if data == nil {
		return nil
	}
	ref, _ := data["current_task"].(string)
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return nil
	}
	return activeFromRef(ref, repoRoot, sourceType, contextKey)
}

func activeFromRef(ref, repoRoot, sourceType, contextKey string) *ActiveTask {
	if strings.TrimSpace(ref) == "" {
		return nil
	}
	resolved := ResolveTaskRef(ref, repoRoot)
	stale := resolved == "" || !isDir(resolved)
	return &ActiveTask{TaskPath: ref, SourceType: sourceType, ContextKey: contextKey, Stale: stale}
}

func singleSessionFallback(repoRoot string) *ActiveTask {
	dir := filepath.Join(repoRoot, ".sunshine", ".runtime", "sessions")
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	var files []string
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".json") {
			files = append(files, e.Name())
		}
	}
	if len(files) != 1 {
		return nil
	}
	sort.Strings(files)
	data := ReadJSONFile(filepath.Join(dir, files[0]))
	if data == nil {
		return nil
	}
	ref, _ := data["current_task"].(string)
	if strings.TrimSpace(ref) == "" {
		return nil
	}
	key := strings.TrimSuffix(files[0], ".json")
	return activeFromRef(ref, repoRoot, "session-fallback", key)
}

func isDir(path string) bool {
	st, err := os.Stat(path)
	return err == nil && st.IsDir()
}

// ShellTicket 是 shell 票据的文件格式（见 shell_session.go）。
type ShellTicket struct {
	Platform       string          `json:"platform"`
	ContextKey     string          `json:"context_key"`
	ConversationID string          `json:"conversation_id,omitempty"`
	SessionID      string          `json:"session_id,omitempty"`
	GenerationID   string          `json:"generation_id,omitempty"`
	CWD            string          `json:"cwd"`
	HostCWD        string          `json:"host_cwd,omitempty"`
	Command        string          `json:"command"`
	Subcommands    []TicketCommand `json:"subcommands"`
	CreatedAtEpoch float64         `json:"created_at_epoch"`
	ExpiresAtEpoch float64         `json:"expires_at_epoch"`
}

// TicketCommand 是票据里的一条任务子命令。
type TicketCommand struct {
	Name    string `json:"name"`
	TaskRef string `json:"task_ref,omitempty"`
}
