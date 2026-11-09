package adapters

import (
	"encoding/json"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
)

// 本文件按上游 internal/paths.ts 实现各平台存储路径解析。
// home 由调用方传入（测试用临时 HOME 夹具）；环境变量每次调用时读取，
// 以便调用方（或测试）改动后立即生效。

// ExpandHome 展开路径开头的 ~（~/ 或单独的 ~）。
func ExpandHome(home, p string) string {
	if p == "~" {
		return home
	}
	if strings.HasPrefix(p, "~/") {
		return filepath.Join(home, p[2:])
	}
	if strings.HasPrefix(p, `~\`) {
		return filepath.Join(home, p[2:])
	}
	return p
}

// ClaudeProjectsDir 返回 ~/.claude/projects。
func ClaudeProjectsDir(home string) string {
	return filepath.Join(home, ".claude", "projects")
}

// CodexSessionsDir 返回 ~/.codex/sessions。
func CodexSessionsDir(home string) string {
	return filepath.Join(home, ".codex", "sessions")
}

// GrokSessionsDir 返回 ~/.grok/sessions。
func GrokSessionsDir(home string) string {
	return filepath.Join(home, ".grok", "sessions")
}

// ZcodeDBPath 返回 ~/.zcode/cli/db/db.sqlite。
func ZcodeDBPath(home string) string {
	return filepath.Join(home, ".zcode", "cli", "db", "db.sqlite")
}

// dataHomeDir 返回 $XDG_DATA_HOME（非空时）否则 ~/.local/share。
func dataHomeDir(home string) string {
	if xdg := strings.TrimSpace(os.Getenv("XDG_DATA_HOME")); xdg != "" {
		return xdg
	}
	return filepath.Join(home, ".local", "share")
}

// DevinDataDir 返回 Devin CLI 数据目录：
// Unix 为 $XDG_DATA_HOME/devin/cli（否则 ~/.local/share/devin/cli），
// Windows 为 %APPDATA%\devin\cli（未设时为 ~/AppData/Roaming/devin/cli）。
func DevinDataDir(home string) string {
	if runtime.GOOS == "windows" {
		appData := strings.TrimSpace(os.Getenv("APPDATA"))
		base := appData
		if base == "" {
			base = filepath.Join(home, "AppData", "Roaming")
		}
		return filepath.Join(base, "devin", "cli")
	}
	return filepath.Join(dataHomeDir(home), "devin", "cli")
}

// DevinDBPath 解析 Devin CLI 会话库路径。DEVIN_DB_PATH 规则：
// 绝对路径直接用、~/ 展开、相对名拼到数据目录、:memory: 表示无文件。
// 第二个返回值为 false 时表示无文件可读。
func DevinDBPath(home string) (string, bool) {
	override := strings.TrimSpace(os.Getenv("DEVIN_DB_PATH"))
	if override != "" {
		if override == ":memory:" {
			return "", false
		}
		expanded := ExpandHome(home, override)
		if filepath.IsAbs(expanded) {
			return expanded, true
		}
		return filepath.Join(DevinDataDir(home), expanded), true
	}
	return filepath.Join(DevinDataDir(home), "sessions.db"), true
}

// OpencodeDataDir 返回 OpenCode 数据目录：$XDG_DATA_HOME/opencode，
// 否则 ~/.local/share/opencode。XDG 规则全平台一致，无 Windows/macOS 分支。
func OpencodeDataDir(home string) string {
	return filepath.Join(dataHomeDir(home), "opencode")
}

// OpencodeDBPath 解析 OpenCode 会话库路径。规则与上游一致：
//  1. OPENCODE_DB：绝对路径直接用、相对名拼到数据目录、:memory: 表示无文件；
//  2. <数据目录>/opencode.db；
//  3. <数据目录>/opencode-<channel>.db：取最近写入的一份。
//
// 第二个返回值为 false 时表示本机无此库。
func OpencodeDBPath(home string) (string, bool) {
	dir := OpencodeDataDir(home)
	override := strings.TrimSpace(os.Getenv("OPENCODE_DB"))
	if override != "" {
		if override == ":memory:" {
			return "", false
		}
		expanded := ExpandHome(home, override)
		if filepath.IsAbs(expanded) {
			return expanded, true
		}
		return filepath.Join(dir, expanded), true
	}
	primary := filepath.Join(dir, "opencode.db")
	if fileExists(primary) {
		return primary, true
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return "", false
	}
	var newest string
	var newestMtime int64 = -1
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if !strings.HasPrefix(name, "opencode-") || !strings.HasSuffix(name, ".db") {
			continue
		}
		full := filepath.Join(dir, name)
		info, err := os.Stat(full)
		if err != nil {
			continue
		}
		if mtime := info.ModTime().UnixMilli(); newest == "" || mtime > newestMtime {
			newest, newestMtime = full, mtime
		}
	}
	if newest == "" {
		return "", false
	}
	return newest, true
}

// PiAgentDir 返回 Pi Agent 目录：PI_CODING_AGENT_DIR 环境变量，否则 ~/.pi/agent。
func PiAgentDir(home string) string {
	if dir := strings.TrimSpace(os.Getenv("PI_CODING_AGENT_DIR")); dir != "" {
		return ExpandHome(home, dir)
	}
	return filepath.Join(home, ".pi", "agent")
}

// PiSessionsDir 返回 Pi 默认会话目录：PI_CODING_AGENT_SESSION_DIR，
// 否则 <PiAgentDir>/sessions。
func PiSessionsDir(home string) string {
	if dir := strings.TrimSpace(os.Getenv("PI_CODING_AGENT_SESSION_DIR")); dir != "" {
		return ExpandHome(home, dir)
	}
	return filepath.Join(PiAgentDir(home), "sessions")
}

// readPiSettingsSessionDir 读 Pi settings.json 里的 sessionDir；
// 相对路径按 settings 文件所在目录解析。读不到或无此字段时返回 ""。
func readPiSettingsSessionDir(home, settingsFile string) string {
	data, err := os.ReadFile(settingsFile)
	if err != nil {
		return ""
	}
	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil || raw == nil {
		return ""
	}
	dir, _ := raw["sessionDir"].(string)
	if strings.TrimSpace(dir) == "" {
		return ""
	}
	expanded := ExpandHome(home, dir)
	if filepath.IsAbs(expanded) {
		return expanded
	}
	return filepath.Join(filepath.Dir(settingsFile), expanded)
}

// PiSessionRoots 返回可见的 Pi 会话根目录（去重）：默认会话目录、环境变量
// 指定目录、全局 settings.json 的 sessionDir，以及项目 .pi/settings.json
// 的 sessionDir（cwd 非空时）。
func PiSessionRoots(home, cwd string) []string {
	roots := []string{filepath.Join(PiAgentDir(home), "sessions"), PiSessionsDir(home)}
	if dir := readPiSettingsSessionDir(home, filepath.Join(PiAgentDir(home), "settings.json")); dir != "" {
		roots = append(roots, dir)
	}
	if cwd != "" {
		if dir := readPiSettingsSessionDir(home, filepath.Join(cwd, ".pi", "settings.json")); dir != "" {
			roots = append(roots, dir)
		}
	}
	seen := make(map[string]bool, len(roots))
	var out []string
	for _, root := range roots {
		normalized := filepath.Clean(root)
		if seen[normalized] {
			continue
		}
		seen[normalized] = true
		out = append(out, root)
	}
	return out
}

// ClaudeProjectDirFromCwd 把 cwd 转成 Claude 的项目目录名：
// 每个路径分隔符（/ 与 \）、盘符冒号（:）、下划线、点都换成 -。
func ClaudeProjectDirFromCwd(home, cwd string) string {
	replacer := strings.NewReplacer("/", "-", `\`, "-", ":", "-", "_", "-", ".", "-")
	return filepath.Join(ClaudeProjectsDir(home), replacer.Replace(cwd))
}

// GrokProjectDirFromCwd 把 cwd 做 URL 编码后拼成 Grok 项目目录，
// 如 /Users/x/proj → %2FUsers%2Fx%2Fproj。
func GrokProjectDirFromCwd(home, cwd string) string {
	return filepath.Join(GrokSessionsDir(home), url.PathEscape(filepath.Clean(cwd)))
}

// GrokCwdFromProjectDir 是 GrokProjectDirFromCwd 的逆操作；
// 名字不是合法百分号编码时返回 ""。
func GrokCwdFromProjectDir(dirName string) string {
	decoded, err := url.PathUnescape(dirName)
	if err != nil {
		return ""
	}
	return decoded
}

// PiProjectDirFromCwd 把 cwd 编码成 Pi 的项目目录：
// --<去开头的分隔符、分隔符与冒号换成短横的绝对路径>--。
func PiProjectDirFromCwd(home, cwd string) string {
	resolved := filepath.Clean(cwd)
	trimmed := strings.TrimPrefix(resolved, string(filepath.Separator))
	trimmed = strings.TrimPrefix(trimmed, `\`)
	replacer := strings.NewReplacer("/", "-", `\`, "-", ":", "-")
	safe := "--" + replacer.Replace(trimmed) + "--"
	return filepath.Join(PiAgentDir(home), "sessions", safe)
}

// WalkDir 递归列出 root 下的全部文件路径（排序后返回，
// 便于测试稳定）。根目录缺失或不可读时返回空列表。
func WalkDir(root string) []string {
	var out []string
	entries, err := os.ReadDir(root)
	if err != nil {
		return out
	}
	for _, entry := range entries {
		full := filepath.Join(root, entry.Name())
		if entry.IsDir() {
			out = append(out, WalkDir(full)...)
		} else if entry.Type().IsRegular() {
			out = append(out, full)
		}
	}
	sort.Strings(out)
	return out
}

func fileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.Mode().IsRegular()
}

func dirExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}
