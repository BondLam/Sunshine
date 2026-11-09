// Package hook 实现 sunshine hook <事件> 子命令的业务逻辑。
//
// 它替代 Trellis 的全部 Python hook 脚本（shared-hooks 的 4 个脚本、
// Claude 的 statusline.py、Codex 自有的 session-start.py），行为逐条
// 对齐这些脚本，但按 Go 重新实现，不复制源码、不依赖 Python 运行时。
//
// 铁律：hook 命令在任何情况下都不得以非零码退出，也绝不能阻断 agent
// 会话。本包所有 Run* 函数失败时都静默返回 nil（无输出或只输出规定
// 格式），调用方（internal/cli/hook.go）同样用 Run 而不用 RunE。
package hook

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

// HookInput 是 hook stdin JSON 的通用表示。
type HookInput map[string]any

// Options 是各 Run* 函数的运行选项。
type Options struct {
	// Version 是当前 sunshine 二进制的版本号，用于 session-start 的更新提示。
	Version string
	// Stdout/Stderr 仅供测试注入；为空时用 os.Stdout/os.Stderr。
	Stdout io.Writer
	Stderr io.Writer
}

func (o Options) out() io.Writer {
	if o.Stdout != nil {
		return o.Stdout
	}
	return os.Stdout
}

func (o Options) errOut() io.Writer {
	if o.Stderr != nil {
		return o.Stderr
	}
	return os.Stderr
}

// ReadInput 从 r 读入全部 stdin 并解析为 HookInput。不是 JSON、不是
// 对象、读失败时都返回空 map，不报错（对应各 Python 脚本的容错分支）。
func ReadInput(r io.Reader) HookInput {
	if r == nil {
		return HookInput{}
	}
	raw, err := io.ReadAll(r)
	if err != nil || len(bytes.TrimSpace(raw)) == 0 {
		return HookInput{}
	}
	var data map[string]any
	if err := json.Unmarshal(raw, &data); err != nil || data == nil {
		return HookInput{}
	}
	return data
}

// ReadInputTimeout 与 ReadInput 相同，但只等 timeout：某些 hook 宿主
// （如 Kiro IDE 的 runCommand）会让 stdin 一直开着不发数据，Python 版
// inject-workflow-state.py 用 0.2 秒线程超时防挂死，这里同样处理。
func ReadInputTimeout(r io.Reader, timeout time.Duration) HookInput {
	if r == nil {
		return HookInput{}
	}
	type result struct {
		raw []byte
		err error
	}
	ch := make(chan result, 1)
	go func() {
		raw, err := io.ReadAll(r)
		ch <- result{raw, err}
	}()
	select {
	case res := <-ch:
		if res.err != nil || len(bytes.TrimSpace(res.raw)) == 0 {
			return HookInput{}
		}
		var data map[string]any
		if err := json.Unmarshal(res.raw, &data); err != nil || data == nil {
			return HookInput{}
		}
		return data
	case <-time.After(timeout):
		return HookInput{}
	}
}

// WriteJSON 把 v 以 JSON 形式写到 w，不转义 HTML（中文与 < > 原样输出，
// 与 Python json.dumps(ensure_ascii=False) 一致），末尾加换行。
func WriteJSON(w io.Writer, v any) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	_ = enc.Encode(v)
	_, _ = w.Write(buf.Bytes())
}

// GetString 从 input 取字符串字段，容忍 snake_case / camelCase 两种写法。
// 找不到或不是字符串时返回 ""。
func GetString(input HookInput, keys ...string) string {
	for _, k := range keys {
		if s, ok := input[k].(string); ok {
			if t := strings.TrimSpace(s); t != "" {
				return t
			}
		}
	}
	return ""
}

// GetNested 在 input 的嵌套对象（input/properties/event/hook_input/
// hookInput）里递归找字符串字段，对应 active_task.py 的 _lookup_string。
func GetNested(input HookInput, keys ...string) string {
	if s := GetString(input, keys...); s != "" {
		return s
	}
	for _, nk := range []string{"input", "properties", "event", "hook_input", "hookInput"} {
		if nested, ok := input[nk].(map[string]any); ok {
			if s := GetNested(nested, keys...); s != "" {
				return s
			}
		}
	}
	return ""
}

// HooksDisabled 对应各 Python 脚本的 should_skip_injection：
// SUNSHINE_HOOKS=0 / SUNSHINE_DISABLE_HOOKS=1，或任一平台的
// <PLATFORM>_NON_INTERACTIVE=1 时跳过注入。保留 TRELLIS_* 旧变量名
// 作兼容（已用 Sunshine 的项目不会再设它们）。
func HooksDisabled() bool {
	if os.Getenv("SUNSHINE_HOOKS") == "0" || os.Getenv("TRELLIS_HOOKS") == "0" {
		return true
	}
	if os.Getenv("SUNSHINE_DISABLE_HOOKS") == "1" || os.Getenv("TRELLIS_DISABLE_HOOKS") == "1" {
		return true
	}
	for _, v := range []string{
		"CLAUDE_NON_INTERACTIVE", "QODER_NON_INTERACTIVE",
		"CODEBUDDY_NON_INTERACTIVE", "FACTORY_NON_INTERACTIVE",
		"CURSOR_NON_INTERACTIVE", "GEMINI_NON_INTERACTIVE",
		"KIRO_NON_INTERACTIVE", "COPILOT_NON_INTERACTIVE",
		"TRAE_NON_INTERACTIVE", "ZCODE_NON_INTERACTIVE",
		"CODEX_NON_INTERACTIVE",
	} {
		if os.Getenv(v) == "1" {
			return true
		}
	}
	return false
}

// projectDirEnvVars 是 session-start.py 查找项目目录的环境变量表
// （宿主注入的项目目录，优先级高于 hook JSON 的 cwd）。
var projectDirEnvVars = []string{
	"CLAUDE_PROJECT_DIR", "QODER_PROJECT_DIR", "CODEBUDDY_PROJECT_DIR",
	"FACTORY_PROJECT_DIR", "CURSOR_PROJECT_DIR", "GEMINI_PROJECT_DIR",
	"KIRO_PROJECT_DIR", "COPILOT_PROJECT_DIR", "TRAE_PROJECT_DIR",
	"ZCODE_PROJECT_DIR",
}

// platformEnvVars 是 _detect_platform 的环境变量表。注意顺序：
// CLAUDE_PROJECT_DIR 是多家宿主共用的兼容别名，必须最后检查，
// 否则 CodeBuddy/ZCode/Trae 都会被误判为 claude，context key 与
// session 文件对不上，每轮都报 no_task。
var platformEnvVars = []struct{ env, platform string }{
	{"ZCODE_PROJECT_DIR", "zcode"},
	{"CURSOR_PROJECT_DIR", "cursor"},
	{"CODEBUDDY_PROJECT_DIR", "codebuddy"},
	{"FACTORY_PROJECT_DIR", "droid"},
	{"GEMINI_PROJECT_DIR", "gemini"},
	{"QODER_PROJECT_DIR", "qoder"},
	{"KIRO_PROJECT_DIR", "kiro"},
	{"COPILOT_PROJECT_DIR", "copilot"},
	{"TRAE_PROJECT_DIR", "trae"},
	{"CLAUDE_PROJECT_DIR", "claude"},
}

// DetectPlatform 判断当前 hook 宿主平台，对应各 Python 脚本的
// _detect_platform。返回 "" 表示未知。
//
// 偏差：Python 还会从 sys.argv[0] 所在的目录名（.claude/.cursor/…）
// 推断平台；Go 版 argv[0] 是 sunshine 二进制路径，不含这类目录，
// 故该分支无法沿用。补一个显式信号：SUNSHINE_HOOK_PLATFORM
// 环境变量（平台接线方在 hook 命令前设置，如
// `SUNSHINE_HOOK_PLATFORM=codex sunshine hook workflow-state`），
// 优先级高于 cursor_version 与项目目录环境变量。
func DetectPlatform(input HookInput) string {
	if v := strings.TrimSpace(os.Getenv("SUNSHINE_HOOK_PLATFORM")); v != "" {
		return sanitizeKey(v)
	}
	if _, ok := input["cursor_version"].(string); ok {
		return "cursor"
	}
	for _, e := range platformEnvVars {
		if os.Getenv(e.env) != "" {
			return e.platform
		}
	}
	return ""
}

// DetectPlatformForSubagent 是 inject-subagent-context.py 的平台检测：
// SubagentStart 事件一定来自 Codex，其余与 DetectPlatform 相同。
func DetectPlatformForSubagent(input HookInput) string {
	if GetString(input, "hook_event_name", "hookEventName") == "SubagentStart" {
		return "codex"
	}
	return DetectPlatform(input)
}

// FindSunshineRoot 从 start 出发向上查找含 .sunshine/ 的目录，
// 找不到返回 ""。对应 inject-workflow-state.py 的 find_trellis_root。
// start 为 "" 时用当前工作目录。
func FindSunshineRoot(start string) string {
	if start == "" {
		cwd, err := os.Getwd()
		if err != nil {
			return ""
		}
		start = cwd
	}
	abs, err := filepath.Abs(start)
	if err != nil {
		return ""
	}
	cur := abs
	for {
		if st, err := os.Stat(filepath.Join(cur, ".sunshine")); err == nil && st.IsDir() {
			return cur
		}
		parent := filepath.Dir(cur)
		if parent == cur {
			return ""
		}
		cur = parent
	}
}

// ResolveProjectRoot 按 session-start.py 的顺序定位项目目录：先看宿主
// 注入的环境变量，再看 hook JSON 的 cwd，最后用进程 cwd；再向上查找
// .sunshine/。找不到 .sunshine/ 时返回 ""（调用方静默退出 0）。
func ResolveProjectRoot(input HookInput) string {
	for _, v := range projectDirEnvVars {
		if val := os.Getenv(v); val != "" {
			if root := FindSunshineRoot(val); root != "" {
				return root
			}
		}
	}
	if cwd := GetString(input, "cwd"); cwd != "" {
		if root := FindSunshineRoot(cwd); root != "" {
			return root
		}
	}
	return FindSunshineRoot("")
}

// ResolveProjectRootMulti 与 ResolveProjectRoot 相同，但对 shell hook
// 另有要求：某些宿主（如 CodeBuddy IDE）对每个 PreToolUse 事件都
// 发 cwd="/"，只信 payload 会找不到根目录；此时回退到进程 cwd
// （宿主从项目根目录启动 hook，相对命令能解析即证明了这一点）。
func ResolveProjectRootMulti(input HookInput) string {
	candidates := []string{GetString(input, "cwd")}
	if cwd, err := os.Getwd(); err == nil {
		candidates = append(candidates, cwd)
	}
	for _, c := range candidates {
		if c == "" {
			continue
		}
		if root := FindSunshineRoot(c); root != "" {
			return root
		}
	}
	return ""
}

// SunshineDir 返回 root 下的 .sunshine 目录。
func SunshineDir(root string) string { return filepath.Join(root, ".sunshine") }

// ReadFile 读文件，失败返回 fallback（对应 Python 的 read_file）。
func ReadFile(path, fallback string) string {
	b, err := os.ReadFile(path)
	if err != nil {
		return fallback
	}
	return string(b)
}

// ReadJSONFile 读 JSON 对象文件，失败返回 nil。
func ReadJSONFile(path string) map[string]any {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	var data map[string]any
	if err := json.Unmarshal(b, &data); err != nil || data == nil {
		return nil
	}
	return data
}

// RunGit 在 repoRoot 下运行 git（3 秒超时），失败或非零退出返回 ""。
func RunGit(repoRoot string, args ...string) string {
	return runCommandTimeout(repoRoot, 3000, "git", args...)
}

var ansiRe = regexp.MustCompile("\x1b\\[[0-9;]*m")

// VisibleLen 返回去掉 ANSI 转义后的可见长度（statusline 换行判断用）。
func VisibleLen(s string) int { return len([]rune(ansiRe.ReplaceAllString(s, ""))) }

// RepoRelative 返回 path 相对 repoRoot 的斜杠路径，超出时返回原路径。
func RepoRelative(repoRoot, path string) string {
	rel, err := filepath.Rel(repoRoot, path)
	if err != nil || strings.HasPrefix(rel, "..") {
		return filepath.ToSlash(path)
	}
	return filepath.ToSlash(rel)
}

// NormalizeTaskRef 规范化任务引用，对应 active_task.normalize_task_ref：
// 去空白、去 ./ 前缀、tasks/xxx → .sunshine/tasks/xxx，绝对路径原样返回。
func NormalizeTaskRef(ref string) string {
	n := strings.TrimSpace(ref)
	if n == "" {
		return ""
	}
	if filepath.IsAbs(n) {
		return n
	}
	n = strings.ReplaceAll(n, "\\", "/")
	for strings.HasPrefix(n, "./") {
		n = n[2:]
	}
	if strings.HasPrefix(n, "tasks/") {
		return ".sunshine/" + n
	}
	return n
}

// ResolveTaskRef 把任务引用解析为任务目录绝对路径，并做收容检查：
// 引用必须落在仓库内（或经由符号链接的 .sunshine 自身实址内，见
// active_task.py #567），否则返回 ""。对应 resolve_task_ref。
func ResolveTaskRef(ref, repoRoot string) string {
	n := NormalizeTaskRef(ref)
	if n == "" {
		return ""
	}
	var candidate string
	switch {
	case filepath.IsAbs(n):
		candidate = n
	case strings.HasPrefix(n, ".sunshine/"):
		candidate = filepath.Join(repoRoot, filepath.FromSlash(n))
	default:
		candidate = filepath.Join(repoRoot, ".sunshine", "tasks", filepath.FromSlash(n))
	}
	resolved, err := filepath.EvalSymlinks(candidate)
	if err != nil {
		// 目录尚不存在时 EvalSymlinks 失败，退回词法路径做收容检查。
		resolved = filepath.Clean(candidate)
	}
	rootReal, err := filepath.EvalSymlinks(repoRoot)
	if err != nil {
		rootReal = filepath.Clean(repoRoot)
	}
	if contained(rootReal, resolved) {
		return resolved
	}
	if wfReal, err := filepath.EvalSymlinks(filepath.Join(repoRoot, ".sunshine")); err == nil {
		if rel, err := filepath.Rel(wfReal, resolved); err == nil && !strings.HasPrefix(rel, "..") {
			return filepath.Join(repoRoot, ".sunshine", rel)
		}
	}
	return ""
}

// contained 判断 target 是否在 base 之下（含等于）。
func contained(base, target string) bool {
	rel, err := filepath.Rel(base, target)
	return err == nil && (rel == "." || !strings.HasPrefix(rel, ".."+string(filepath.Separator)) && rel != "..")
}

// GetDeveloper 取开发者名，对应 common.paths.get_developer 的前两步：
// SUNSHINE_DEVELOPER 环境变量 → .sunshine/.developer 文件。
// 文件格式兼容两种：name=<名> 行，或首行即名字（Go 版 scaffold 写的
// 就是后者）。找不到返回 ""。
//
// 偏差：Python 还会读 git 主工作区的 .developer（linked worktree 继承）；
// Go 版未实现该分支，worktree 下须自行在 .sunshine/.developer 写名字。
func GetDeveloper(repoRoot string) string {
	if v := strings.TrimSpace(os.Getenv("SUNSHINE_DEVELOPER")); v != "" {
		return v
	}
	if v := strings.TrimSpace(os.Getenv("TRELLIS_DEVELOPER")); v != "" {
		return v
	}
	content := ReadFile(filepath.Join(repoRoot, ".sunshine", ".developer"), "")
	for _, line := range strings.Split(content, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if strings.HasPrefix(line, "name=") {
			if name := strings.TrimSpace(strings.TrimPrefix(line, "name=")); name != "" {
				return name
			}
			continue
		}
		return line
	}
	return ""
}

// CountLines 数文件行数，不存在返回 0。
func CountLines(path string) int {
	b, err := os.ReadFile(path)
	if err != nil {
		return 0
	}
	if len(b) == 0 {
		return 0
	}
	n := bytes.Count(b, []byte("\n"))
	if !bytes.HasSuffix(b, []byte("\n")) {
		n++
	}
	return n
}

// ActiveJournalFile 返回开发者工作区里编号最大的 journal-<N>.md，
// 对应 common.paths.get_active_journal_file（工作区目录为
// .sunshine/workspace/<开发者名>/）。
func ActiveJournalFile(repoRoot string) string {
	dev := GetDeveloper(repoRoot)
	if dev == "" {
		return ""
	}
	dir := filepath.Join(repoRoot, ".sunshine", "workspace", dev)
	entries, err := os.ReadDir(dir)
	if err != nil {
		return ""
	}
	best := ""
	bestNum := 0
	for _, e := range entries {
		if e.IsDir() || !strings.HasPrefix(e.Name(), "journal-") || !strings.HasSuffix(e.Name(), ".md") {
			continue
		}
		var num int
		name := strings.TrimSuffix(e.Name(), ".md")
		if _, err := fmt.Sscanf(name, "journal-%d", &num); err != nil || num <= 0 {
			continue
		}
		if num > bestNum {
			bestNum, best = num, filepath.Join(dir, e.Name())
		}
	}
	return best
}

// CountActiveTasks 数 .sunshine/tasks 下的有效任务数（有合法 task.json
// 的目录，排除 archive），对应 common.tasks.iter_active_tasks 的计数。
func CountActiveTasks(repoRoot string, tasksDir string) int {
	if tasksDir == "" {
		tasksDir = filepath.Join(repoRoot, ".sunshine", "tasks")
	}
	entries, err := os.ReadDir(tasksDir)
	if err != nil {
		return 0
	}
	n := 0
	for _, e := range entries {
		if !e.IsDir() || e.Name() == "archive" {
			continue
		}
		if ReadJSONFile(filepath.Join(tasksDir, e.Name(), "task.json")) != nil {
			n++
		}
	}
	return n
}

// HasCuratedJSONLEntry 判断 jsonl 是否至少有一行含 file 字段。
// 新建的空 jsonl 与只有 {"_example": ...} 占位行的都不算“已整理”。
func HasCuratedJSONLEntry(path string) bool {
	b, err := os.ReadFile(path)
	if err != nil {
		return false
	}
	for _, line := range strings.Split(string(b), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		var row map[string]any
		if err := json.Unmarshal([]byte(line), &row); err != nil {
			continue
		}
		if _, ok := row["file"]; ok {
			return true
		}
	}
	return false
}

// FormatGitState 返回 "Git: branch <分支>; clean/dirty N paths."，
// 对应 session-start.py 的 _format_git_state（git 3 秒超时）。
func FormatGitState(repoRoot string) string {
	branch := RunGit(repoRoot, "branch", "--show-current")
	if branch == "" {
		branch = "(detached)"
	}
	dirty := 0
	if out := RunGit(repoRoot, "status", "--porcelain"); out != "" {
		for _, line := range strings.Split(out, "\n") {
			if strings.TrimSpace(line) != "" {
				dirty++
			}
		}
	}
	dirtyText := "clean"
	if dirty > 0 {
		dirtyText = fmt.Sprintf("dirty %d paths", dirty)
	}
	return fmt.Sprintf("Git: branch %s; %s.", branch, dirtyText)
}
