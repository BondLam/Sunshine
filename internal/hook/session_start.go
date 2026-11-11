package hook

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

// RunSessionStart 实现 sunshine hook session-start，对应
// shared-hooks/session-start.py：读 stdin 的 hook JSON，注入结构化
// 上下文（workflow 概览、开发者身份、git 状态、当前任务、项目规范），
// 输出 additionalContext JSON。无 .sunshine/ 时静默退出 0。
// 任何失败都静默返回 nil，绝不阻断 agent 会话。
func RunSessionStart(r io.Reader, opts Options) error {
	if HooksDisabled() {
		return nil
	}
	input := ReadInput(r)
	root := ResolveProjectRoot(input)
	if root == "" {
		return nil
	}
	platform := DetectPlatform(input)
	contextKey := ResolveContextKey(input, platform)
	persistContextKeyForBash(contextKey)

	cfg := LoadConfig(root)
	active := ResolveActiveTask(root, input, platform, ResolveOptions{AllowEnvironmentContext: true})
	taskPkg := taskPackage(root, active)
	allowedPkgs := resolveSpecScope(cfg, taskPkg)
	specIndexPaths := collectSpecIndexPaths(root, allowedPkgs)

	var out strings.Builder
	out.WriteString("<session-context>\nSunshine compact SessionStart context. Use it to orient the session; load details on demand.\n</session-context>\n\n")
	out.WriteString(buildFirstReplyNotice(resolveUpdateHint(root, contextKey, opts.Version)))
	out.WriteString("\n\n")

	if warn := checkLegacySpec(root, cfg); warn != "" {
		out.WriteString("<migration-warning>\n" + warn + "\n</migration-warning>\n\n")
	}

	out.WriteString("<current-state>\n")
	out.WriteString(buildCompactCurrentState(root, input, cfg, active, specIndexPaths))
	out.WriteString("\n</current-state>\n\n")

	out.WriteString("<sunshine-workflow>\n")
	out.WriteString(buildWorkflowOverview(filepath.Join(root, ".sunshine", "workflow.md")))
	out.WriteString("\n</sunshine-workflow>\n\n")

	out.WriteString("<guidelines>\n")
	out.WriteString("Task context order for implementation/check: jsonl entries -> `prd.md` -> `design.md if present` -> `implement.md if present`. Missing optional artifacts are skipped for lightweight tasks.\n\n")
	if len(specIndexPaths) > 0 {
		out.WriteString("## Available indexes (read on demand)\n")
		for _, p := range specIndexPaths {
			out.WriteString("- " + p + "\n")
		}
		out.WriteString("\n")
	}
	out.WriteString("</guidelines>\n\n")

	out.WriteString("<task-status>\n" + taskStatusBlock(root, input, active) + "\n</task-status>\n\n")
	out.WriteString("<ready>\nContext loaded. Follow <task-status>. Load workflow/spec/task details only when needed.\n</ready>")

	contextText := out.String()

	// Kiro 把 hook 的 stdout 直接加进会话上下文，不用 JSON 信封。
	if platform == "kiro" {
		fmt.Fprint(opts.out(), contextText)
		return nil
	}
	result := map[string]any{
		"hookSpecificOutput": map[string]any{
			"hookEventName":     "SessionStart",
			"additionalContext": contextText,
		},
	}
	// ZCode 会同时读 hookSpecificOutput.additionalContext 与顶层
	// additional_context 且不去重，两处都发会让上下文重复出现，
	// 因此只对非 ZCode 平台发顶层键。
	if platform != "zcode" {
		result["additional_context"] = contextText
	}
	WriteJSON(opts.out(), result)
	return nil
}

const firstReplyNoticeHead = `<first-reply-notice>
On the first visible assistant reply in this session, briefly acknowledge that Sunshine SessionStart context loaded.`

const firstReplyNoticeTail = `Choose the acknowledgment language in this order:
1. Use the language of the user's current request (the user message that triggered this reply).
2. If that request has no clear natural language, use an explicitly established project communication language.
3. If neither provides a language, output the language-neutral fallback exactly: ` + "`Sunshine SessionStart ✓`" + `.
Continue directly with the user's request after the acknowledgment.
The acknowledgment must not alter the language used for the remainder of the response.
This notice is one-shot: do not repeat it after the first visible assistant reply in this session.
</first-reply-notice>`

func buildFirstReplyNotice(updateHint string) string {
	if updateHint == "" {
		return firstReplyNoticeHead + "\n" + firstReplyNoticeTail
	}
	return firstReplyNoticeHead + "\nAlso relay this Sunshine maintenance notice on its own line in that same reply: " + updateHint + "\n" + firstReplyNoticeTail
}

// persistContextKeyForBash 把会话身份桥接给后续的 Bash 命令：
// Claude Code 的 SessionStart hook 可以往 CLAUDE_ENV_FILE 追加 export，
// 同会话的 Bash 工具随后就能读到。文件是用户自己的（conda、代理…），
// 无条件追加会无限增长，因此当最后一个 TRELLIS…/SUNSHINE_CONTEXT_ID
// 导出已是同一值时跳过（last wins，只看最后一次赋值才正确）。
func persistContextKeyForBash(contextKey string) {
	if contextKey == "" {
		return
	}
	envFile := os.Getenv("CLAUDE_ENV_FILE")
	if envFile == "" {
		return
	}
	exportLine := "export SUNSHINE_CONTEXT_ID=" + shellQuote(contextKey)
	if lastContextKeyExport(envFile) == exportLine {
		return
	}
	f, err := os.OpenFile(envFile, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return
	}
	defer f.Close()
	_, _ = f.WriteString(exportLine + "\n")
}

func lastContextKeyExport(envFile string) string {
	b, err := os.ReadFile(envFile)
	if err != nil {
		return ""
	}
	last := ""
	for _, line := range strings.Split(string(b), "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), "export SUNSHINE_CONTEXT_ID=") {
			last = strings.TrimSpace(line)
		}
	}
	return last
}

func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

// taskPackage 读当前任务 task.json 的 package 字段。
func taskPackage(root string, active ActiveTask) string {
	if active.TaskPath == "" {
		return ""
	}
	dir := ResolveTaskRef(active.TaskPath, root)
	if dir == "" {
		return ""
	}
	data := ReadJSONFile(filepath.Join(dir, "task.json"))
	if data == nil {
		return ""
	}
	pkg, _ := data["package"].(string)
	return pkg
}

// resolveSpecScope 解析哪些包的规范要注入，对应 _resolve_spec_scope。
// 返回 nil 表示全量扫描；非 nil 为允许的包名集合。
func resolveSpecScope(cfg Config, taskPkg string) map[string]bool {
	if !cfg.IsMonorepo() || cfg.SpecScope == nil {
		return nil
	}
	switch scope := cfg.SpecScope.(type) {
	case string:
		if scope == "active_task" {
			if taskPkg != "" && cfg.Packages[taskPkg] {
				return map[string]bool{taskPkg: true}
			}
			if cfg.DefaultPackage != "" && cfg.Packages[cfg.DefaultPackage] {
				return map[string]bool{cfg.DefaultPackage: true}
			}
		}
		return nil
	case []any:
		valid := map[string]bool{}
		for _, e := range scope {
			if name, ok := e.(string); ok {
				if cfg.Packages[name] {
					valid[name] = true
				} else {
					fmt.Fprintf(os.Stderr, "Warning: spec_scope contains unknown package: %s, ignoring\n", name)
				}
			}
		}
		if len(valid) > 0 {
			if taskPkg != "" && !valid[taskPkg] {
				fmt.Fprintf(os.Stderr, "Warning: active task package '%s' is out of configured spec_scope\n", taskPkg)
			}
			return valid
		}
		fmt.Fprintln(os.Stderr, "Warning: all spec_scope entries invalid, falling back to task/default/full")
		if taskPkg != "" && cfg.Packages[taskPkg] {
			return map[string]bool{taskPkg: true}
		}
		if cfg.DefaultPackage != "" && cfg.Packages[cfg.DefaultPackage] {
			return map[string]bool{cfg.DefaultPackage: true}
		}
		return nil
	}
	return nil
}

// collectSpecIndexPaths 收集规范索引路径，对应 _collect_spec_index_paths。
func collectSpecIndexPaths(root string, allowedPkgs map[string]bool) []string {
	sunshineDir := filepath.Join(root, ".sunshine")
	var paths []string
	if isFile(filepath.Join(sunshineDir, "spec", "guides", "index.md")) {
		paths = append(paths, ".sunshine/spec/guides/index.md")
	}
	specDir := filepath.Join(sunshineDir, "spec")
	entries, err := os.ReadDir(specDir)
	if err != nil {
		return paths
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Name() < entries[j].Name() })
	for _, sub := range entries {
		if !sub.IsDir() || strings.HasPrefix(sub.Name(), ".") || sub.Name() == "guides" {
			continue
		}
		if isFile(filepath.Join(specDir, sub.Name(), "index.md")) {
			paths = append(paths, ".sunshine/spec/"+sub.Name()+"/index.md")
			continue
		}
		if allowedPkgs != nil && !allowedPkgs[sub.Name()] {
			continue
		}
		nested, err := os.ReadDir(filepath.Join(specDir, sub.Name()))
		if err != nil {
			continue
		}
		sort.Slice(nested, func(i, j int) bool { return nested[i].Name() < nested[j].Name() })
		for _, n := range nested {
			if !n.IsDir() {
				continue
			}
			if isFile(filepath.Join(specDir, sub.Name(), n.Name(), "index.md")) {
				paths = append(paths, ".sunshine/spec/"+sub.Name()+"/"+n.Name()+"/index.md")
			}
		}
	}
	return paths
}

func isFile(path string) bool {
	st, err := os.Stat(path)
	return err == nil && !st.IsDir()
}

// checkLegacySpec 检测 monorepo 下的旧式扁平规范结构，对应 _check_legacy_spec。
func checkLegacySpec(root string, cfg Config) string {
	if !cfg.IsMonorepo() || len(cfg.Packages) == 0 {
		return ""
	}
	specDir := filepath.Join(root, ".sunshine", "spec")
	if !isDir(specDir) {
		return ""
	}
	hasLegacy := false
	for _, name := range []string{"backend", "frontend"} {
		if isDir(filepath.Join(specDir, name)) && isFile(filepath.Join(specDir, name, "index.md")) {
			hasLegacy = true
			break
		}
	}
	if !hasLegacy {
		return ""
	}
	var missing []string
	for name := range cfg.Packages {
		if !isDir(filepath.Join(specDir, name)) {
			missing = append(missing, name)
		}
	}
	sort.Strings(missing)
	if len(missing) == 0 {
		return ""
	}
	pkgs := make([]string, 0, len(cfg.Packages))
	for name := range cfg.Packages {
		pkgs = append(pkgs, name)
	}
	sort.Strings(pkgs)
	if len(missing) == len(pkgs) {
		return fmt.Sprintf("[!] Legacy spec structure detected: found `spec/backend/` or `spec/frontend/` but no package-scoped `spec/<package>/` directories.\nMonorepo packages: %s\nPlease reorganize: `spec/backend/` -> `spec/<package>/backend/`", strings.Join(pkgs, ", "))
	}
	return fmt.Sprintf("[!] Partial spec migration detected: packages %s still missing `spec/<pkg>/` directory.\nPlease complete migration for all packages.", strings.Join(missing, ", "))
}

// buildCompactCurrentState 生成 <current-state> 的紧凑状态行。
func buildCompactCurrentState(root string, input HookInput, cfg Config, active ActiveTask, specIndexPaths []string) string {
	var lines []string
	dev := GetDeveloper(root)
	if dev == "" {
		dev = "(not initialized)"
	}
	lines = append(lines, "Developer: "+dev)
	lines = append(lines, FormatGitState(root))
	if active.TaskPath != "" {
		taskDir := ResolveTaskRef(active.TaskPath, root)
		status := "unknown"
		if taskDir != "" {
			if data := ReadJSONFile(filepath.Join(taskDir, "task.json")); data != nil {
				if s, ok := data["status"].(string); ok && s != "" {
					status = s
				}
			}
		}
		rel := active.TaskPath
		if taskDir != "" {
			rel = RepoRelative(root, taskDir)
		}
		lines = append(lines, fmt.Sprintf("Current task: %s; status=%s.", rel, status))
	} else {
		lines = append(lines, "Current task: none.")
	}
	lines = append(lines, fmt.Sprintf("Active tasks: %d total. Use `sunshine task list --mine` only if needed.", CountActiveTasks(root, cfg.TasksDirAbs(root))))
	if journal := ActiveJournalFile(root); journal != "" {
		lines = append(lines, fmt.Sprintf("Journal: %s, %d / %d lines.", RepoRelative(root, journal), CountLines(journal), cfg.MaxJournalLines))
	}
	if len(specIndexPaths) > 0 {
		lines = append(lines, fmt.Sprintf("Spec indexes: %d available.", len(specIndexPaths)))
	}
	return strings.Join(lines, "\n")
}

// taskStatusBlock 生成 <task-status> 内容，对应 _get_task_status。
func taskStatusBlock(root string, input HookInput, active ActiveTask) string {
	if active.TaskPath == "" {
		return "Status: NO ACTIVE TASK\nNext-Action: Classify the current turn before creating any Sunshine task. Simple conversation / small task asks only whether this turn should create a Sunshine task. Complex task asks whether task creation and planning are allowed."
	}
	taskRef := active.TaskPath
	taskDir := ResolveTaskRef(taskRef, root)
	if active.Stale || taskDir == "" || !isDir(taskDir) {
		return fmt.Sprintf("Status: STALE POINTER\nTask: %s\nNext-Action: Run `sunshine task finish` to clear the stale pointer, then ask the user what to work on next.", taskRef)
	}
	taskData := ReadJSONFile(filepath.Join(taskDir, "task.json"))
	title := taskRef
	status := "unknown"
	if taskData != nil {
		if t, ok := taskData["title"].(string); ok && t != "" {
			title = t
		}
		if s, ok := taskData["status"].(string); ok && s != "" {
			status = s
		}
	}
	artifacts := []string{"prd.md", "design.md", "implement.md", "implement.jsonl", "check.jsonl"}
	var present []string
	for _, name := range artifacts {
		if isFile(filepath.Join(taskDir, name)) {
			present = append(present, name)
		}
	}
	if isDir(filepath.Join(taskDir, "research")) {
		present = append(present, "research/")
	}
	presentLine := "(none)"
	if len(present) > 0 {
		presentLine = strings.Join(present, ", ")
	}
	if status == "completed" {
		return fmt.Sprintf("Status: COMPLETED\nTask: %s\nPresent: %s\nNext-Action: Run `/sunshine:finish-work`. If the working tree is dirty, return to Phase 3.4 first.", title, presentLine)
	}
	hasPRD := isFile(filepath.Join(taskDir, "prd.md"))
	hasDesign := isFile(filepath.Join(taskDir, "design.md"))
	hasImplementPlan := isFile(filepath.Join(taskDir, "implement.md"))
	jsonlReady := (!isFile(filepath.Join(taskDir, "implement.jsonl")) || HasCuratedJSONLEntry(filepath.Join(taskDir, "implement.jsonl"))) &&
		(!isFile(filepath.Join(taskDir, "check.jsonl")) || HasCuratedJSONLEntry(filepath.Join(taskDir, "check.jsonl")))
	if status == "planning" && !hasPRD {
		return fmt.Sprintf("Status: PLANNING\nTask: %s\nPresent: %s\nNext-Action: Load `sunshine-brainstorm` and write `prd.md`. Stay in planning.", title, presentLine)
	}
	if status == "planning" {
		var missing []string
		if !hasDesign {
			missing = append(missing, "design.md")
		}
		if !hasImplementPlan {
			missing = append(missing, "implement.md")
		}
		var bits []string
		if len(missing) > 0 {
			bits = append(bits, "Lightweight task can request start review with PRD-only; complex task must add "+strings.Join(missing, ", ")+" before start")
		} else {
			bits = append(bits, "Planning artifacts are present; ask for review before `sunshine task start`")
		}
		if !jsonlReady {
			bits = append(bits, "curate `implement.jsonl` and `check.jsonl` before sub-agent mode start")
		}
		return fmt.Sprintf("Status: PLANNING\nTask: %s\nPresent: %s\nNext-Action: %s. Do not enter implementation until the user confirms start.", title, presentLine, strings.Join(bits, "; "))
	}
	return fmt.Sprintf("Status: %s\nTask: %s\nPresent: %s\nNext-Action: Follow the matching per-turn workflow-state. Implementation/check context order is jsonl entries -> `prd.md` -> `design.md if present` -> `implement.md if present`.", strings.ToUpper(status), title, presentLine)
}

// extractRange 取 content 里从 "## startHeader" 起到 "## endHeader"
// 之前（不含）的行；找不到起始标题返回 ""，找不到结束标题取到文末。
func extractRange(content, startHeader, endHeader string) string {
	lines := strings.Split(content, "\n")
	start := -1
	end := len(lines)
	for i, line := range lines {
		if start == -1 && strings.TrimSpace(line) == "## "+startHeader {
			start = i
			continue
		}
		if start != -1 && strings.TrimSpace(line) == "## "+endHeader {
			end = i
			break
		}
	}
	if start == -1 {
		return ""
	}
	return strings.TrimRight(strings.Join(lines[start:end], "\n"), "\n")
}

var (
	workflowTagOpenRe = regexp.MustCompile(`\[workflow-state:([A-Za-z0-9_-]+)\]`)
	htmlCommentRe     = regexp.MustCompile(`(?s)<!--.*?-->`)
	bracketLineRe     = regexp.MustCompile(`^\[[^\]\n]+\]$`)
)

// stripBreadcrumbTagBlocks 去掉 workflow.md 里 [workflow-state:STATUS]
// …[/workflow-state:STATUS] 标签块（它们归 UserPromptSubmit hook 消费，
// session-start 已覆盖完整步骤体，再贴会重复），以及 HTML 注释与
// 孤立的 [xxx] 行。Go 的 regexp（RE2）不支持反向引用与前瞻，
// 因此标签块用手工扫描配对，而不用 Python 的正则。
func stripBreadcrumbTagBlocks(content string) string {
	var out strings.Builder
	rest := content
	for {
		loc := workflowTagOpenRe.FindStringSubmatchIndex(rest)
		if loc == nil {
			out.WriteString(rest)
			break
		}
		after := rest[loc[1]:]
		if !isLoneLineTag(after) {
			// 行内提及不是块：原样保留，继续往后找。
			out.WriteString(rest[:loc[1]])
			rest = after
			continue
		}
		status := rest[loc[2]:loc[3]]
		closeTag := "[/workflow-state:" + status + "]"
		closeIdx := findLineStartCloseTag(after, closeTag)
		if closeIdx == -1 {
			// 独占一行却没有闭标签：原样保留，继续往后找（旧实现到这里
			// 就把剩余全文原样写出、再也不剥任何块）。
			out.WriteString(rest[:loc[1]])
			rest = after
			continue
		}
		out.WriteString(rest[:loc[0]])
		rest = after[closeIdx+len(closeTag):]
	}
	stripped := htmlCommentRe.ReplaceAllString(out.String(), "")
	var kept []string
	for _, line := range strings.Split(stripped, "\n") {
		trimmed := strings.TrimSpace(line)
		if bracketLineRe.MatchString(trimmed) && !strings.HasPrefix(trimmed, "[workflow-state:") && !strings.HasPrefix(trimmed, "[/workflow-state:") {
			continue
		}
		kept = append(kept, line)
	}
	collapsed := regexp.MustCompile(`\n{3,}`).ReplaceAllString(strings.Join(kept, "\n"), "\n\n")
	return strings.TrimSpace(collapsed)
}

// buildWorkflowOverview 只注入精简的 Phase Index 摘要，对应 _build_workflow_overview。
func buildWorkflowOverview(workflowPath string) string {
	content := ReadFile(workflowPath, "")
	if content == "" {
		return "No workflow.md found"
	}
	lines := []string{
		"# Development Workflow - Session Summary",
		"Full guide: .sunshine/workflow.md.",
		"",
	}
	if phases := extractRange(content, "Phase Index", "Phase 1: Plan"); phases != "" {
		lines = append(lines, strings.TrimRight(stripBreadcrumbTagBlocks(phases), "\n"))
	}
	return strings.TrimRight(strings.Join(lines, "\n"), "\n")
}

// resolveUpdateHint 返回本会话唯一一次的更新提示，对应
// common.session_context.get_update_hint：读 .sunshine/.version 作
// 当前版本，用 `sunshine --version` 的输出作可用的最新版本
// （Python 版跑的是 `trellis --version` 并从其输出里提取版本号；
// Go 版二进制的版本输出就是 "Sunshine 版本 x.y.z"，直接解析），
// 有更新时写一次 .sunshine/.runtime/update-check-<key>.marker 节流。
// 二进制不在 PATH 或版本相等/无法解析时返回 ""。
func resolveUpdateHint(root, contextKey, binaryVersion string) string {
	marker := updateMarkerPath(root, contextKey)
	if _, err := os.Stat(marker); err == nil {
		return ""
	}
	current := strings.TrimSpace(ReadFile(filepath.Join(root, ".sunshine", ".version"), ""))
	if current == "" {
		return ""
	}
	// 当前运行的二进制版本就是“可用版本”的最佳代理（Python 版跑
	// `trellis --version` 从其输出提取，含义相同）；未传入时才去
	// PATH 里找 sunshine 二进制问一次。
	latest := binaryVersion
	if _, ok := parseVersion(latest); !ok {
		latest = availableSunshineVersion()
	}
	if latest == "" {
		return ""
	}
	markUpdateCheckAttempted(marker)
	if compareVersions(current, latest) >= 0 {
		return ""
	}
	return fmt.Sprintf("Sunshine 更新可用：%s -> %s，请运行 sunshine update", current, latest)
}

func updateMarkerPath(root, contextKey string) string {
	key := contextKey
	if key == "" {
		if term := strings.TrimSpace(os.Getenv("TERM_SESSION_ID")); term != "" {
			key = term
		} else {
			key = fmt.Sprintf("ppid-%d", os.Getppid())
		}
	}
	safe := sanitizeKey(key)
	if safe == "" {
		safe = "session"
	}
	return filepath.Join(root, ".sunshine", ".runtime", "update-check-"+safe+".marker")
}

func markUpdateCheckAttempted(marker string) {
	_ = os.MkdirAll(filepath.Dir(marker), 0o755)
	_ = os.WriteFile(marker, []byte(time.Now().Format(time.RFC3339)+"\n"), 0o644)
}

var versionTokenRe = regexp.MustCompile(`\b\d+(?:\.\d+){1,2}(?:-[0-9A-Za-z.-]+)?\b`)

func availableSunshineVersion() string {
	out, ok := runCommandOutput("", 1000, "sunshine", "--version")
	if !ok {
		return ""
	}
	return versionTokenRe.FindString(out)
}

// compareVersions 比较 semver：<0 表示 a<b，0 相等，>0 表示 a>b；
// 无法解析返回 -1 之外的哨兵……调用方只在两者都解析成功时比较，
// 解析失败返回 0（视为无更新，避免误报）。
func compareVersions(a, b string) int {
	pa, okA := parseVersion(a)
	pb, okB := parseVersion(b)
	if !okA || !okB {
		return 0
	}
	for i := 0; i < 3; i++ {
		if pa.nums[i] != pb.nums[i] {
			if pa.nums[i] < pb.nums[i] {
				return -1
			}
			return 1
		}
	}
	// 无预发布号的版本高于有预发布号的同号版本。
	if pa.pre == "" && pb.pre != "" {
		return 1
	}
	if pa.pre != "" && pb.pre == "" {
		return -1
	}
	return strings.Compare(pa.pre, pb.pre)
}

type parsedVersion struct {
	nums [3]int
	pre  string
}

var versionRe = regexp.MustCompile(`^\s*(\d+)(?:\.(\d+))?(?:\.(\d+))?(?:-([0-9A-Za-z.-]+))?\s*$`)

func parseVersion(s string) (parsedVersion, bool) {
	m := versionRe.FindStringSubmatch(s)
	if m == nil {
		return parsedVersion{}, false
	}
	var v parsedVersion
	for i := 0; i < 3; i++ {
		if m[i+1] != "" {
			n, err := strconv.Atoi(m[i+1])
			if err != nil {
				return parsedVersion{}, false
			}
			v.nums[i] = n
		}
	}
	v.pre = m[4]
	return v, true
}
