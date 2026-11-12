package task

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/BondLam/Sunshine/internal/hook"
)

// ActiveTask 是解析出的当前任务（对应 active_task.ActiveTask）。
type ActiveTask struct {
	// TaskPath 是任务引用（如 .sunshine/tasks/11-09-xxx），为空表示无任务。
	TaskPath string
	// SourceType 是 "session" / "session-fallback" / "file" / "none"。
	SourceType string
	// ContextKey 是会话键（如 claude_abc123），可能为空。
	ContextKey string
	// Stale 表示指针存在但任务目录已不存在。
	Stale bool
}

// Source 返回人类可读的来源标签。
func (a ActiveTask) Source() string {
	switch a.SourceType {
	case "session", "session-fallback":
		if a.ContextKey != "" {
			return a.SourceType + ":" + a.ContextKey
		}
	case "file":
		return "file:.sunshine/.current-task"
	}
	return a.SourceType
}

func sessionsDir(root string) string {
	return filepath.Join(root, DirSunshine, ".runtime", "sessions")
}

func sessionFilePath(root, contextKey string) string {
	return filepath.Join(sessionsDir(root), contextKey+".json")
}

func currentTaskFilePath(root string) string {
	return filepath.Join(root, DirSunshine, FileCurrentTask)
}

// readCurrentTaskFile 读 .current-task 文件里的任务引用。
func readCurrentTaskFile(root string) string {
	data, err := os.ReadFile(currentTaskFilePath(root))
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(data))
}

// writeCurrentTaskFile 写 .current-task 文件（内容为任务路径）。
func writeCurrentTaskFile(root, ref string) error {
	return writeFileAtomic(currentTaskFilePath(root), []byte(ref+"\n"), 0o644)
}

// removeCurrentTaskFile 删除 .current-task 文件（不存在不报错）。
func removeCurrentTaskFile(root string) {
	_ = os.Remove(currentTaskFilePath(root))
}

// ---------------------------------------------------------------------------
// context_key 解析
//
// 决策（源码不清处以此为准）：task 命令没有 hook JSON 输入，因此
// context_key 的来源链是 hook.active_task.resolve_context_key 去掉
// hook 输入后的部分——
//   1. SUNSHINE_CONTEXT_ID / TRELLIS_CONTEXT_ID 环境变量显式覆盖；
//   2. 平台会话环境变量（hook.ResolveContextKey(nil, "") 覆盖这两步，
//      含 CLAUDE_CODE_SESSION_ID、CURSOR_CONVERSATION_ID 等）；
//   3. shell 票据（本文件 lookupShellTicketContextKey 实现，见下）。
// 会话文件写到 .sunshine/.runtime/sessions/<context_key>.json，
// 内容为 {"current_task": "<引用>", ...}（与 hook 的 activeFromSessionFile
// 读取格式一致）。
// ---------------------------------------------------------------------------

var sanitizeRe = regexp.MustCompile(`[^A-Za-z0-9._-]+`)

func sanitizeKey(raw string) string {
	safe := sanitizeRe.ReplaceAllString(strings.TrimSpace(raw), "_")
	safe = strings.Trim(safe, "._-")
	if len(safe) > 160 {
		safe = safe[:160]
	}
	return safe
}

// ResolveContextKey 解析当前命令的会话键：环境变量优先，其次 shell 票据。
// subcommand 是当前 task 子命令（start/current/finish），taskRef 仅
// start 时用于票据的 task_ref 匹配。
func ResolveContextKey(root, subcommand, taskRef string) string {
	if key := hook.ResolveContextKey(nil, ""); key != "" {
		return key
	}
	return lookupShellTicketContextKey(root, subcommand, taskRef)
}

// ---------------------------------------------------------------------------
// shell 票据消费（internal/hook/NOTES.md 的四条接受条件，与
// active_task._matching_ticket_context_key 一致，argv[0] 的 task.py
// 判定改为“当前正在执行的 sunshine task 子命令”）：
//
//  1. 票据新鲜（expires_at_epoch 未过；无 created_at_epoch 时只看
//     expires_at_epoch，两者皆无视为新鲜；有 created_at 时须在 30 秒内）；
//  2. cwd 落在当前仓库内（为空时放行）；
//  3. 正在执行的命令与票据 subcommands 匹配（start 还须 task_ref 匹配）；
//  4. 恰好只有一个新鲜票据的 context_key 匹配——两个窗口并发时宁可都降级。
//
// 票据目录：.sunshine/.runtime/shell-tickets/ 与旧目录
// .sunshine/.runtime/cursor-shell/（只读不写）。
// ---------------------------------------------------------------------------

// ShellTicketTTLSeconds 是票据有效期（秒）。
const ShellTicketTTLSeconds = 30

func ticketDirs(root string) []string {
	runtime := filepath.Join(root, DirSunshine, ".runtime")
	return []string{
		filepath.Join(runtime, "shell-tickets"),
		filepath.Join(runtime, "cursor-shell"),
	}
}

func ticketIsFresh(ticket map[string]any, path string, now float64) bool {
	if exp, ok := ticket["expires_at_epoch"].(float64); ok && exp < now {
		_ = os.Remove(path)
		return false
	}
	if created, ok := ticket["created_at_epoch"].(float64); ok {
		if now-created <= ShellTicketTTLSeconds {
			return true
		}
		_ = os.Remove(path)
		return false
	}
	return true
}

func ticketCWDMatchesRepo(ticket map[string]any, root string) bool {
	cwd, _ := ticket["cwd"].(string)
	cwd = strings.TrimSpace(cwd)
	if cwd == "" {
		return true
	}
	resolved, err := filepath.EvalSymlinks(cwd)
	if err != nil {
		resolved = filepath.Clean(cwd)
	}
	rootReal, err := filepath.EvalSymlinks(root)
	if err != nil {
		rootReal = filepath.Clean(root)
	}
	rel, err := filepath.Rel(rootReal, resolved)
	return err == nil && (rel == "." || !strings.HasPrefix(rel, ".."+string(filepath.Separator)) && rel != "..")
}

// taskRefsMatch 比较两个任务引用是否指向同一任务目录。
func taskRefsMatch(left, right, root string) bool {
	if left == "" || right == "" {
		return false
	}
	lp := hook.ResolveTaskRef(left, root)
	rp := hook.ResolveTaskRef(right, root)
	if lp != "" && rp != "" {
		return lp == rp
	}
	return hook.NormalizeTaskRef(left) == hook.NormalizeTaskRef(right)
}

func ticketMatchesCommand(ticket map[string]any, subcommand, taskRef, root string) bool {
	if subcommand != "start" && subcommand != "current" && subcommand != "finish" {
		return false
	}
	subs, ok := ticket["subcommands"].([]any)
	if !ok {
		return false
	}
	for _, item := range subs {
		m, ok := item.(map[string]any)
		if !ok {
			continue
		}
		name, _ := m["name"].(string)
		if name != subcommand {
			continue
		}
		if subcommand != "start" {
			return true
		}
		ref, _ := m["task_ref"].(string)
		if taskRefsMatch(ref, taskRef, root) {
			return true
		}
	}
	return false
}

func matchingTicketContextKey(path, root, subcommand, taskRef string, now float64) string {
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	var ticket map[string]any
	if err := json.Unmarshal(data, &ticket); err != nil || ticket == nil {
		return ""
	}
	if !ticketIsFresh(ticket, path, now) {
		return ""
	}
	if !ticketCWDMatchesRepo(ticket, root) {
		return ""
	}
	if !ticketMatchesCommand(ticket, subcommand, taskRef, root) {
		return ""
	}
	key, _ := ticket["context_key"].(string)
	return strings.TrimSpace(key)
}

// lookupShellTicketContextKey 在两个票据目录里找唯一匹配的 context_key。
func lookupShellTicketContextKey(root, subcommand, taskRef string) string {
	now := float64(time.Now().UnixNano()) / 1e9
	candidates := map[string]bool{}
	for _, dir := range ticketDirs(root) {
		entries, err := os.ReadDir(dir)
		if err != nil {
			continue
		}
		for _, e := range entries {
			if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
				continue
			}
			if key := matchingTicketContextKey(filepath.Join(dir, e.Name()), root, subcommand, taskRef, now); key != "" {
				candidates[key] = true
			}
		}
	}
	if len(candidates) == 1 {
		for key := range candidates {
			return key
		}
	}
	return ""
}

// ---------------------------------------------------------------------------
// 会话文件与当前任务解析
// ---------------------------------------------------------------------------

func readSessionFile(path string) map[string]any {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	var m map[string]any
	if err := json.Unmarshal(data, &m); err != nil || m == nil {
		return nil
	}
	return m
}

func writeSessionFile(path string, data map[string]any) error {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	if err := enc.Encode(data); err != nil {
		return err
	}
	return writeFileAtomic(path, buf.Bytes(), 0o644)
}

func activeFromRef(ref, root, sourceType, contextKey string) *ActiveTask {
	if strings.TrimSpace(ref) == "" {
		return nil
	}
	resolved := hook.ResolveTaskRef(ref, root)
	stale := resolved == "" || !isDir(resolved)
	return &ActiveTask{TaskPath: ref, SourceType: sourceType, ContextKey: contextKey, Stale: stale}
}

func isDir(path string) bool {
	st, err := os.Stat(path)
	return err == nil && st.IsDir()
}

func activeFromSessionFile(root, contextKey, sourceType string) *ActiveTask {
	data := readSessionFile(sessionFilePath(root, contextKey))
	if data == nil {
		return nil
	}
	ref, _ := data["current_task"].(string)
	if strings.TrimSpace(ref) == "" {
		return nil
	}
	return activeFromRef(ref, root, sourceType, contextKey)
}

func singleSessionFallback(root string) *ActiveTask {
	entries, err := os.ReadDir(sessionsDir(root))
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
	data := readSessionFile(filepath.Join(sessionsDir(root), files[0]))
	if data == nil {
		return nil
	}
	ref, _ := data["current_task"].(string)
	if strings.TrimSpace(ref) == "" {
		return nil
	}
	key := strings.TrimSuffix(files[0], ".json")
	return activeFromRef(ref, root, "session-fallback", key)
}

// ResolveActiveTask 解析当前任务。顺序：会话键（环境变量/票据）对应的
// session 文件 → .sunshine/.current-task 文件 → 唯一会话文件回退。
// 与 Trellis 的差异见包说明：Trellis 只认 session 文件，Go 版另写
// .current-task 作为纯 CLI 下的可见指针（PLAN 要求 start 写它），
// hook 不读该文件，因此纯 CLI start 的任务不会被 session-start 注入；
// 经 shell 票据桥接的 agent 内 start 会同时写 session 文件，不受影响。
func ResolveActiveTask(root, subcommand, taskRef string) ActiveTask {
	contextKey := ResolveContextKey(root, subcommand, taskRef)
	if contextKey != "" {
		if active := activeFromSessionFile(root, contextKey, "session"); active != nil {
			return *active
		}
	}
	if ref := readCurrentTaskFile(root); ref != "" {
		if active := activeFromRef(ref, root, "file", contextKey); active != nil {
			return *active
		}
	}
	if fb := singleSessionFallback(root); fb != nil {
		return *fb
	}
	return ActiveTask{SourceType: "none", ContextKey: contextKey}
}

// SetActiveTask 写当前任务指针：有会话键时写 session 文件，同时写
// .sunshine/.current-task。无会话键时只写 .current-task（纯 CLI 降级，
// 对应 task.py 的 degraded mode——Python 此时不写任何指针，Go 版按
// PLAN 要求仍写 .current-task 以便 sunshine task current 可见）。
// 返回写好的 ActiveTask（无键时 SourceType 为 "file"）。
func SetActiveTask(root, taskRef string) (ActiveTask, error) {
	canonical := taskRef
	if resolved := hook.ResolveTaskRef(taskRef, root); resolved == "" || !isDir(resolved) {
		return ActiveTask{}, fmt.Errorf("任务不存在：%s", taskRef)
	}
	if rel, err := filepath.Rel(root, hook.ResolveTaskRef(taskRef, root)); err == nil && !strings.HasPrefix(rel, "..") {
		canonical = filepath.ToSlash(rel)
	}
	contextKey := ResolveContextKey(root, "start", taskRef)
	if contextKey != "" {
		path := sessionFilePath(root, contextKey)
		data := readSessionFile(path)
		if data == nil {
			data = map[string]any{}
		}
		data["current_task"] = canonical
		if _, ok := data["platform"]; !ok {
			data["platform"] = platformOfKey(contextKey)
		}
		data["last_seen_at"] = time.Now().UTC().Format("2006-01-02T15:04:05Z")
		if _, ok := data["current_run"]; !ok {
			data["current_run"] = nil
		}
		if err := writeSessionFile(path, data); err != nil {
			return ActiveTask{}, fmt.Errorf("写入会话文件失败：%w", err)
		}
	}
	if err := writeCurrentTaskFile(root, canonical); err != nil {
		return ActiveTask{}, fmt.Errorf("写入 .current-task 失败：%w", err)
	}
	sourceType := "file"
	if contextKey != "" {
		sourceType = "session"
	}
	return ActiveTask{TaskPath: canonical, SourceType: sourceType, ContextKey: contextKey}, nil
}

var knownPlatforms = map[string]bool{
	"claude": true, "codex": true, "cursor": true, "gemini": true,
	"qoder": true, "kiro": true, "copilot": true, "zcode": true,
	"snow": true, "droid": true, "codebuddy": true,
}

func platformOfKey(key string) string {
	if i := strings.Index(key, "_"); i > 0 {
		if prefix := key[:i]; knownPlatforms[prefix] {
			return prefix
		}
	}
	return "session"
}

// ClearActiveTask 清除当前任务：删 session 文件（有键时）与
// .current-task。返回被清除前的任务，无任务时返回 SourceType "none"。
func ClearActiveTask(root, subcommand, taskRef string) ActiveTask {
	previous := ResolveActiveTask(root, subcommand, taskRef)
	if previous.TaskPath == "" {
		return previous
	}
	if previous.ContextKey != "" {
		_ = os.Remove(sessionFilePath(root, previous.ContextKey))
	}
	// .current-task 与指针指向同一任务时一并清除。
	if ref := readCurrentTaskFile(root); ref != "" && taskRefsMatch(ref, previous.TaskPath, root) {
		removeCurrentTaskFile(root)
	} else if previous.SourceType == "file" {
		removeCurrentTaskFile(root)
	}
	return previous
}

// ClearTaskFromSessions 删除所有指向某任务的 session 文件与
// .current-task（archive 用）。返回清除的 session 文件数。
func ClearTaskFromSessions(root, taskPath string) int {
	target := hook.NormalizeTaskRef(taskPath)
	if target == "" {
		return 0
	}
	cleared := 0
	entries, err := os.ReadDir(sessionsDir(root))
	if err == nil {
		for _, e := range entries {
			if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
				continue
			}
			path := filepath.Join(sessionsDir(root), e.Name())
			data := readSessionFile(path)
			if data == nil {
				continue
			}
			current, _ := data["current_task"].(string)
			if current == "" {
				continue
			}
			if taskRefsMatch(hook.NormalizeTaskRef(current), target, root) {
				if err := os.Remove(path); err == nil {
					cleared++
				}
			}
		}
	}
	if ref := readCurrentTaskFile(root); ref != "" && taskRefsMatch(ref, taskPath, root) {
		removeCurrentTaskFile(root)
	}
	return cleared
}

// RepointTaskInSessions 把指向旧任务的指针改指新任务（rename 用）。
// 返回改动的 session 文件数。
func RepointTaskInSessions(root, oldPath, newPath string) int {
	target := hook.NormalizeTaskRef(oldPath)
	replacement := hook.NormalizeTaskRef(newPath)
	if target == "" || replacement == "" {
		return 0
	}
	moved := 0
	entries, err := os.ReadDir(sessionsDir(root))
	if err == nil {
		for _, e := range entries {
			if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
				continue
			}
			path := filepath.Join(sessionsDir(root), e.Name())
			data := readSessionFile(path)
			if data == nil {
				continue
			}
			current, _ := data["current_task"].(string)
			if current == "" {
				continue
			}
			if taskRefsMatch(hook.NormalizeTaskRef(current), target, root) {
				data["current_task"] = replacement
				if err := writeSessionFile(path, data); err == nil {
					moved++
				}
			}
		}
	}
	if ref := readCurrentTaskFile(root); ref != "" && taskRefsMatch(ref, oldPath, root) {
		_ = writeCurrentTaskFile(root, replacement)
	}
	return moved
}

// ticketFingerprintName 供测试构造票据文件名（与 hook 的命名规则一致）。
func ticketFingerprintName(contextKey, command string, now float64) string {
	digest := sha256.Sum256([]byte(contextKey + "\x00" + command + "\x00" + fmt.Sprintf("%v", now)))
	return fmt.Sprintf("%d-%s.json", time.Now().UnixMilli(), hex.EncodeToString(digest[:])[:16])
}
