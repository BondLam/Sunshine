package task

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/BondLam/Sunshine/internal/hook"
)

// Runner 是 task 命令的执行器：持有项目根与输出 writer，便于测试注入。
type Runner struct {
	Root string
	Out  io.Writer
	Err  io.Writer
	// Now 可注入时间（测试用）；为空时用 time.Now。
	Now func() time.Time
}

func NewRunner(root string, out, err io.Writer) *Runner {
	if out == nil {
		out = os.Stdout
	}
	if err == nil {
		err = os.Stderr
	}
	return &Runner{Root: root, Out: out, Err: err}
}

func (r *Runner) out() io.Writer {
	if r.Out != nil {
		return r.Out
	}
	return os.Stdout
}

func (r *Runner) errOut() io.Writer {
	if r.Err != nil {
		return r.Err
	}
	return os.Stderr
}

func (r *Runner) now() time.Time {
	if r.Now != nil {
		return r.Now()
	}
	return time.Now()
}

func (r *Runner) today() string { return r.now().Format("2006-01-02") }

// requireRoot 校验项目根已初始化。
func (r *Runner) requireRoot() bool {
	if r.Root == "" {
		fmt.Fprintln(r.errOut(), "错误：当前目录未初始化（找不到 .sunshine/），请先运行 sunshine init。")
		return false
	}
	return true
}

func (r *Runner) errf(format string, args ...any) {
	fmt.Fprintf(r.errOut(), "错误："+format+"\n", args...)
}

func (r *Runner) warnf(format string, args ...any) {
	fmt.Fprintf(r.errOut(), "警告："+format+"\n", args...)
}

// resolveOrError 解析任务目录参数，失败时打印原因并返回 ""。
func (r *Runner) resolveOrError(target string) string {
	dir, err := ResolveTaskDir(target, r.Root)
	if err != nil {
		r.errf("%v", err)
		return ""
	}
	if st, err := os.Stat(dir); err != nil || !st.IsDir() {
		r.errf("任务不存在：%s", target)
		r.errf("提示：用任务名（如 my-task）或完整路径（如 .sunshine/tasks/11-09-my-task）")
		return ""
	}
	return dir
}

// ---------------------------------------------------------------------------
// create
// ---------------------------------------------------------------------------

// CreateOptions 是 create 的参数。
type CreateOptions struct {
	Title       string
	Slug        string
	Assignee    string
	Priority    string
	Description string
	Parent      string
	Package     string
	BaseBranch  string
	Meta        []string
	NoStart     bool
	Force       bool
}

func parseMetaPairs(pairs []string) (map[string]string, error) {
	meta := map[string]string{}
	for _, pair := range pairs {
		key, value, ok := strings.Cut(pair, "=")
		if !ok || key == "" {
			return nil, fmt.Errorf("--meta 格式错误 %q，应为 key=value", pair)
		}
		meta[key] = value
	}
	return meta, nil
}

// Create 创建新任务（对应 task_store.cmd_create）。
func (r *Runner) Create(opts CreateOptions) int {
	if !r.requireRoot() {
		return 1
	}
	title := strings.TrimSpace(opts.Title)
	if title == "" {
		r.errf("标题必填")
		fmt.Fprintln(r.errOut(), "请传入非空 <标题>（纯空白不算）：空标题的任务会在归档时被拒绝。")
		return 1
	}
	description := strings.TrimSpace(opts.Description)
	if description == "" {
		r.errf("--description 必填")
		fmt.Fprintln(r.errOut(), "请传入 --description \"<该任务交付什么>\"（纯空白不算）：空描述的任务会在归档时被拒绝。")
		return 1
	}
	meta, err := parseMetaPairs(opts.Meta)
	if err != nil {
		r.errf("%v", err)
		return 1
	}
	// --package：单仓忽略、多仓校验、未传则推断 default_package。
	cfg := LoadConfig(r.Root)
	packageName := ""
	if !cfg.IsMonorepo() {
		if opts.Package != "" {
			r.warnf("单仓项目忽略 --package")
		}
	} else if opts.Package != "" {
		if !cfg.Packages[opts.Package] {
			var available []string
			for name := range cfg.Packages {
				available = append(available, name)
			}
			sort.Strings(available)
			r.errf("未知包 %q，可用：%s", opts.Package, strings.Join(available, ", "))
			return 1
		}
		packageName = opts.Package
	} else if cfg.DefaultPackage != "" {
		if cfg.Packages[cfg.DefaultPackage] {
			packageName = cfg.DefaultPackage
		} else {
			r.warnf("config.yaml 的 default_package %q 不在 packages 里，已忽略", cfg.DefaultPackage)
		}
	}
	// assignee 默认为当前开发者。
	assignee := opts.Assignee
	if assignee == "" {
		assignee = Developer(r.Root)
		if assignee == "" {
			r.errf("未设置开发者。请先运行 sunshine init，或用 --assignee 指定负责人。")
			fmt.Fprintln(r.errOut(), DeveloperHint)
			return 1
		}
	}
	creator := Developer(r.Root)
	if creator == "" {
		creator = assignee
	}
	slug := opts.Slug
	if slug == "" {
		slug = Slugify(title)
	}
	if slug == "" {
		r.errf("无法从标题生成 slug")
		return 1
	}
	if opts.Slug != "" && (strings.ContainsAny(slug, "/\\") || strings.Contains(slug, "..")) {
		r.errf("--slug 必须是纯名字，不能含路径分隔符或 ..：%s", slug)
		return 1
	}
	tasksDir := EnsureTasksDir(r.Root)
	datePrefix := r.now().Format("01-02")
	if opts.Slug != "" {
		if m := regexp.MustCompile(`^(\d{2})-(\d{2})-(.+)$`).FindStringSubmatch(slug); m != nil {
			month, day := m[1], m[2]
			mi, di := atoi2(month), atoi2(day)
			if mi >= 1 && mi <= 12 && di >= 1 && di <= 31 {
				prefix := month + "-" + day
				if prefix == datePrefix {
					slug = m[3]
					r.warnf("--slug 不应带 MM-DD 前缀，已规范化为 %q", slug)
				} else {
					r.errf("--slug 以日期前缀（%s-）开头，但 create 一律用今天的日期（%s）。", prefix, datePrefix)
					fmt.Fprintf(r.errOut(), "请只传 slug 本体，例如 --slug %s\n", m[3])
					return 1
				}
			}
		}
	}
	// --parent 先解析：链不上的父任务是失败请求，不是警告。
	var parentDir string
	var parentName string
	if opts.Parent != "" {
		parentDir = r.resolveOrError(opts.Parent)
		if parentDir == "" {
			r.errf("父任务无法解析：%s", opts.Parent)
			fmt.Fprintln(r.errOut(), "未创建任务。请传入已存在的任务目录。")
			return 1
		}
		if _, err := os.Stat(filepath.Join(parentDir, FileTaskJSON)); err != nil {
			r.errf("父任务的 task.json 不存在：%s", opts.Parent)
			fmt.Fprintln(r.errOut(), "未创建任务。请传入已存在的任务目录。")
			return 1
		}
		parentName = filepath.Base(parentDir)
	}
	dirName := datePrefix + "-" + slug
	taskDir := filepath.Join(tasksDir, dirName)
	if archived := FindArchivedTaskByDirName(tasksDir, dirName); archived != "" {
		r.errf("任务已归档：%s", dirName)
		fmt.Fprintf(r.errOut(), "归档位置：%s\n请换一个 slug 再创建。\n", RepoRelative(r.Root, archived))
		return 1
	}
	createdDir := false
	if st, err := os.Stat(taskDir); err == nil && st.IsDir() {
		if !opts.Force {
			r.errf("任务已存在：%s", dirName)
			fmt.Fprintf(r.errOut(), "已有任务在：%s\n请换一个 --slug，或传 --force 覆盖其 task.json。\n", RepoRelative(r.Root, taskDir))
			return 1
		}
		r.warnf("--force：覆盖已有任务 %s 的 task.json", dirName)
	} else {
		if err := os.MkdirAll(taskDir, 0o755); err != nil {
			r.errf("创建任务目录失败：%v", err)
			return 1
		}
		createdDir = true
	}
	// base_branch：显式 > 本地默认分支 > 当前检出分支（无 remote 时告警）。
	baseBranch := opts.BaseBranch
	if baseBranch == "" {
		if resolved := DefaultBranchLocal(r.Root); resolved != "" {
			baseBranch = resolved
		} else {
			current := CurrentBranch(r.Root)
			if current == "" {
				current = "main"
			}
			baseBranch = current
			r.warnf("无法解析仓库默认分支（未配 remote 或离线），base_branch 暂记为当前检出分支 %q，可用 --base-branch 覆盖。", baseBranch)
		}
	}
	rec := EmptyRecord()
	rec.CreatedAt = r.today()
	rec.ID = slug
	rec.Name = slug
	rec.Title = title
	rec.Description = description
	rec.Priority = opts.Priority
	if rec.Priority == "" {
		rec.Priority = "P2"
	}
	rec.Creator = creator
	rec.Assignee = assignee
	if packageName != "" {
		rec.Package = strptr(packageName)
	}
	rec.BaseBranch = strptr(baseBranch)
	if parentName != "" {
		rec.Parent = strptr(parentName)
	}
	if len(meta) > 0 {
		keys := make([]string, 0, len(meta))
		for k := range meta {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		var buf strings.Builder
		buf.WriteString("{")
		for i, k := range keys {
			if i > 0 {
				buf.WriteString(", ")
			}
			kb, _ := encodeJSONValue(k)
			vb, _ := encodeJSONValue(meta[k])
			buf.Write(kb)
			buf.WriteString(": ")
			buf.Write(vb)
		}
		buf.WriteString("}")
		rec.Meta = json.RawMessage(buf.String())
	}
	if err := WriteRecord(taskDir, rec); err != nil {
		r.errf("写入 task.json 失败：%v", err)
		fmt.Fprintf(r.errOut(), "未创建任务：%s\n", dirName)
		if createdDir {
			_ = os.Remove(taskDir)
		}
		return 1
	}
	prdPath := filepath.Join(taskDir, "prd.md")
	if _, err := os.Stat(prdPath); os.IsNotExist(err) {
		_ = writeFileAtomic(prdPath, []byte(DefaultPRDContent(title, description)), 0o644)
	}
	createdJSONL := false
	if HasSubagentPlatform(r.Root) {
		for _, name := range []string{"implement.jsonl", "check.jsonl"} {
			p := filepath.Join(taskDir, name)
			if _, err := os.Stat(p); os.IsNotExist(err) {
				_ = writeFileAtomic(p, []byte{}, 0o644)
			}
		}
		createdJSONL = true
	}
	// 双向父子链接。
	if parentDir != "" {
		if err := updateTaskJSON(parentDir, func(obj *rawObject) error {
			children := obj.ensureStringList("children")
			found := false
			for _, c := range children {
				if c == dirName {
					found = true
				}
			}
			if !found {
				children = append(children, dirName)
				obj.setStringList("children", children)
			}
			return nil
		}); err != nil {
			r.errf("写入父任务 task.json 失败：%v", err)
			fmt.Fprintf(r.errOut(), "任务已建在 %s，但未链接到 %s，请用 sunshine task add-subtask %s %s 补链。\n",
				RepoRelative(r.Root, taskDir), parentName, parentName, dirName)
			return 1
		}
		fmt.Fprintf(r.errOut(), "已链接为 %s 的子任务\n", parentName)
	}
	// 自动激活（除非 --no-start）：无会话身份时静默跳过（与 Python 一致）。
	if opts.NoStart {
		fmt.Fprintln(r.errOut(), "已跳过会话激活（--no-start）；就绪后运行 sunshine task start。")
	} else if key := hook.ResolveContextKey(nil, ""); key != "" {
		if active, err := setActiveTaskWithKey(r.Root, RepoRelative(r.Root, taskDir), key); err != nil {
			r.warnf("会话激活失败：%v", err)
		} else {
			fmt.Fprintf(r.errOut(), "已为本会话激活任务：%s\n来源：%s\n", active.TaskPath, active.Source())
		}
	}
	fmt.Fprintf(r.errOut(), "已创建任务：%s\n\n下一步：\n", dirName)
	fmt.Fprintln(r.errOut(), "  - 在 prd.md 里写清需求与验收标准")
	fmt.Fprintln(r.errOut(), "  - 轻量任务：只有 PRD 也可以")
	fmt.Fprintln(r.errOut(), "  - 复杂任务：在 sunshine task start 前补充 design.md 与 implement.md")
	if createdJSONL {
		fmt.Fprintln(r.errOut(), "  - 整理 implement.jsonl / check.jsonl（已建空文件），供子代理在 start 前读规范：")
		fmt.Fprintln(r.errOut(), "      每行一个 JSON 对象 {\"file\": \"<路径>\", \"reason\": \"<原因>\"}，只放规范/调研文档")
		fmt.Fprintln(r.errOut(), "      可用 sunshine task context --mode packages 列出可用规范")
	}
	fmt.Fprintln(r.errOut(), "  - 用 /sunshine:continue 或阶段上下文决定下一步")
	fmt.Fprintln(r.errOut())
	fmt.Fprintln(r.out(), DirSunshine+"/"+DirTasks+"/"+dirName)
	RunTaskHooks("after_create", filepath.Join(taskDir, FileTaskJSON), r.Root, fileWriter(r.errOut()))
	return 0
}

func atoi2(s string) int {
	n := 0
	for _, c := range s {
		n = n*10 + int(c-'0')
	}
	return n
}

// setActiveTaskWithKey 用指定会话键写当前任务指针（create 的激活路径）。
func setActiveTaskWithKey(root, taskRef, contextKey string) (ActiveTask, error) {
	if resolved := hook.ResolveTaskRef(taskRef, root); resolved == "" || !isDir(resolved) {
		return ActiveTask{}, fmt.Errorf("任务不存在：%s", taskRef)
	}
	canonical := taskRef
	if rel, err := filepath.Rel(root, hook.ResolveTaskRef(taskRef, root)); err == nil && !strings.HasPrefix(rel, "..") {
		canonical = filepath.ToSlash(rel)
	}
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
	if err := writeCurrentTaskFile(root, canonical); err != nil {
		return ActiveTask{}, fmt.Errorf("写入 .current-task 失败：%w", err)
	}
	return ActiveTask{TaskPath: canonical, SourceType: "session", ContextKey: contextKey}, nil
}

// fileWriter 把 io.Writer 转成 *os.File（RunTaskHooks 的签名用 *os.File；
// 非文件 writer 时回退到 os.Stderr——hook 输出本就是尽力而为的提示）。
func fileWriter(w io.Writer) *os.File {
	if f, ok := w.(*os.File); ok {
		return f
	}
	return os.Stderr
}

// ---------------------------------------------------------------------------
// start / current / finish
// ---------------------------------------------------------------------------

// recordStartState 把刚启动的任务翻为 in_progress 并记录分支（对应
// task.py._record_start_state；task.json 损坏时只警告，不阻断指针）。
func (r *Runner) recordStartState(taskJSONPath, label string) {
	obj, reason, err := readRawObject(taskJSONPath)
	if err != nil {
		problem, hint := describeReadFailure(taskJSONPath, reason)
		fmt.Fprintf(r.errOut(), "警告：%s；task.json 未更新。\n%s\n", problem, hint)
		return
	}
	var applied []string
	if status, _ := obj.getString("status"); status == "planning" {
		obj.set("status", "in_progress")
		applied = append(applied, fmt.Sprintf("✓ 状态：planning → in_progress%s", label))
	}
	var baseConflict string
	if branch, ok := obj.getStringOrNull("branch"); !ok || branch == "" {
		if current := CurrentBranch(r.Root); current != "" {
			obj.set("branch", current)
			applied = append(applied, fmt.Sprintf("✓ 已记录分支：%s%s", current, label))
			if base, _ := obj.getStringOrNull("base_branch"); base == current {
				baseConflict = current
			}
		} else {
			fmt.Fprintln(r.errOut(), "提示：无检出分支（detached HEAD 或非 git 仓库），未记录任务分支。")
		}
	}
	if len(applied) == 0 {
		return
	}
	if err := writeRawObject(taskJSONPath, obj); err != nil {
		fmt.Fprintf(r.errOut(), "警告：写入 %s 失败，状态与分支未变。\n", taskJSONPath)
		return
	}
	for _, line := range applied {
		fmt.Fprintln(r.errOut(), line)
	}
	if baseConflict != "" {
		fmt.Fprintf(r.errOut(), "警告：%q 也是该任务的 base_branch，PR 不能指向自身分支，归档时会拒绝。\n", baseConflict)
		fmt.Fprintf(r.errOut(), "切出分支后运行：sunshine task set-branch <任务> <功能分支>\n")
	}
}

// printStartBreadcrumb 在 task start 成功后输出 in_progress 面包屑。
// workflow.md 承诺“命令成功后，面包屑自动切换到 [workflow-state:in_progress]”，
// 但切换只靠 UserPromptSubmit hook——同一轮内（批准 → start → 实现）没有新
// 的用户消息，hook 不再触发，派发指令就递不到模型面前，模型直接走 inline
// 写代码。这里由 start 命令自己把新状态的块打出来，与 hook 注入的
// <workflow-state> 块格式一致，派发指令在动手写代码之前就进上下文。
func (r *Runner) printStartBreadcrumb(taskDir string) {
	cfg := hook.LoadConfig(r.Root)
	platform := hook.DetectPlatform(hook.HookInput{})
	if platform == "" && os.Getenv("CODEX_THREAD_ID") != "" {
		// CODEX_THREAD_ID 是已核实的 Codex 会话变量（hook 的 envSessionKeys），
		// DetectPlatform 的环境变量表里没有它；只有 Codex 才需要按
		// dispatch_mode 区分 inline/派发块。
		platform = "codex"
	}
	templates := hook.LoadBreadcrumbs(r.Root)
	key := hook.ResolveBreadcrumbKey("in_progress", platform, cfg)
	fmt.Fprintln(r.errOut(), hook.BuildBreadcrumb(r.startTaskDisplayName(taskDir), "in_progress", templates, key))
}

// startTaskDisplayName 取面包屑标题里的任务名：与 hook 的
// GetActiveTaskForWorkflow 一致，优先用 task.json 的 id 字段，缺失时用目录名。
func (r *Runner) startTaskDisplayName(taskDir string) string {
	name := filepath.Base(taskDir)
	if obj, _, err := readRawObject(filepath.Join(taskDir, FileTaskJSON)); err == nil {
		if id := obj.getStringField("id"); strings.TrimSpace(id) != "" {
			name = id
		}
	}
	return name
}

// Start 设为当前任务（对应 task.py cmd_start）。
func (r *Runner) Start(dir string, allowEmptyContext bool) int {
	if !r.requireRoot() {
		return 1
	}
	if dir == "" {
		r.errf("需要任务目录或任务名")
		return 1
	}
	fullPath := r.resolveOrError(dir)
	if fullPath == "" {
		return 1
	}
	// 上下文清单门禁（#573）：已建的 implement/check.jsonl 为空时拒绝，
	// 除非 --allow-empty-context；文件不存在不设防。
	if !allowEmptyContext {
		var empty []string
		for _, name := range []string{"implement.jsonl", "check.jsonl"} {
			if CuratedEntryCount(filepath.Join(fullPath, name)) == 0 {
				empty = append(empty, name)
			}
		}
		if len(empty) > 0 {
			if len(empty) == 1 {
				r.errf("%s 没有已整理条目", empty[0])
			} else {
				r.errf("%s 没有已整理条目", strings.Join(empty, " 和 "))
			}
			fmt.Fprintln(r.errOut(), "子代理（implement/check）将得不到任何规范上下文。")
			fmt.Fprintf(r.errOut(), "  整理：sunshine task add-context %s implement <路径> \"<原因>\"\n", dir)
			fmt.Fprintf(r.errOut(), "  检查：sunshine task validate %s\n", dir)
			fmt.Fprintln(r.errOut(), "  确实要空着：重新运行 start 并加 --allow-empty-context")
			return 1
		}
	}
	taskRel := RepoRelative(r.Root, fullPath)
	taskJSONPath := filepath.Join(fullPath, FileTaskJSON)
	if key := ResolveContextKey(r.Root, "start", dir); key == "" {
		// 降级模式：无会话身份，不写 session 文件；仍翻状态、记分支。
		fmt.Fprintln(r.errOut(), "ℹ 会话身份不可用；本次不持久化当前任务指针（降级模式），AI 仍可按对话上下文继续。")
		fmt.Fprintln(r.errOut(), "提示：在能提供会话身份的 AI 会话内运行，或先设置 SUNSHINE_CONTEXT_ID 再执行 sunshine task start。")
		if _, err := os.Stat(taskJSONPath); err == nil {
			r.recordStartState(taskJSONPath, "（降级）")
			RunTaskHooks("after_start", taskJSONPath, r.Root, fileWriter(r.errOut()))
		}
		// PLAN 要求 start 写 .current-task：降级模式也写该文件指针，
		// 以便 sunshine task current 在纯 CLI 下可见。
		_ = writeCurrentTaskFile(r.Root, taskRel)
		r.printStartBreadcrumb(fullPath)
		return 0
	}
	active, err := SetActiveTask(r.Root, taskRel)
	if err != nil {
		r.errf("设置当前任务失败：%v", err)
		return 1
	}
	fmt.Fprintf(r.errOut(), "✓ 当前任务已设为：%s\n来源：%s\n", active.TaskPath, active.Source())
	if _, err := os.Stat(taskJSONPath); err == nil {
		r.recordStartState(taskJSONPath, "")
	}
	fmt.Fprintln(r.errOut())
	fmt.Fprintln(r.errOut(), "hook 现在会从该任务的 jsonl 文件注入上下文。")
	r.printStartBreadcrumb(fullPath)
	RunTaskHooks("after_start", taskJSONPath, r.Root, fileWriter(r.errOut()))
	return 0
}

// Finish 清除当前任务（对应 task.py cmd_finish）。
func (r *Runner) Finish() int {
	if !r.requireRoot() {
		return 1
	}
	active := ClearActiveTask(r.Root, "finish", "")
	if active.TaskPath == "" {
		fmt.Fprintln(r.errOut(), "当前没有设置任务")
		return 0
	}
	fmt.Fprintf(r.errOut(), "✓ 已清除当前任务（原为：%s）\n来源：%s\n", active.TaskPath, active.Source())
	taskJSONPath := filepath.Join(r.Root, active.TaskPath, FileTaskJSON)
	if _, err := os.Stat(taskJSONPath); err == nil {
		RunTaskHooks("after_finish", taskJSONPath, r.Root, fileWriter(r.errOut()))
	}
	return 0
}

// Current 显示当前任务（对应 task.py cmd_current）。
func (r *Runner) Current(showSource, asJSON bool) int {
	if !r.requireRoot() {
		return 1
	}
	active := ResolveActiveTask(r.Root, "current", "")
	if asJSON {
		var taskObj map[string]any
		var readErr map[string]any
		if active.TaskPath != "" {
			taskJSONPath := filepath.Join(r.Root, active.TaskPath, FileTaskJSON)
			obj, reason, err := readRawObject(taskJSONPath)
			data := map[string]any{}
			if err != nil {
				problem, hint := describeReadFailure(taskJSONPath, reason)
				readErr = map[string]any{
					"file":    taskJSONPath,
					"reason":  reason,
					"message": problem + "。" + hint,
				}
			} else {
				for _, k := range obj.keys {
					var v any
					_ = json.Unmarshal(obj.vals[k], &v)
					data[k] = v
				}
			}
			id, _ := data["id"].(string)
			if id == "" {
				id, _ = data["name"].(string)
			}
			var idVal any
			if id != "" {
				idVal = id
			}
			children := []any{}
			if c, ok := data["children"].([]any); ok {
				children = c
			}
			taskObj = map[string]any{
				"dir":         active.TaskPath,
				"id":          idVal,
				"title":       data["title"],
				"status":      data["status"],
				"parent":      data["parent"],
				"children":    children,
				"branch":      data["branch"],
				"base_branch": data["base_branch"],
			}
		}
		payload := map[string]any{
			"current_task": taskObj,
			"source":       active.Source(),
			"stale":        active.Stale,
		}
		if readErr != nil {
			payload["error"] = readErr
		}
		printJSON(r.out(), payload)
		if active.TaskPath != "" {
			return 0
		}
		return 1
	}
	if showSource {
		if active.TaskPath == "" {
			fmt.Fprintln(r.out(), "当前任务：(无)")
		} else {
			fmt.Fprintf(r.out(), "当前任务：%s\n", active.TaskPath)
		}
		fmt.Fprintf(r.out(), "来源：%s\n", active.Source())
		if active.Stale {
			fmt.Fprintln(r.out(), "状态：指针过期")
		}
		if active.TaskPath != "" {
			return 0
		}
		return 1
	}
	if active.TaskPath != "" {
		fmt.Fprintln(r.out(), active.TaskPath)
		return 0
	}
	fmt.Fprintln(r.out(), "当前任务：(无)")
	return 1
}

func printJSON(w io.Writer, v any) {
	var buf strings.Builder
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	_ = enc.Encode(v)
	fmt.Fprint(w, buf.String())
}

// ---------------------------------------------------------------------------
// list / list-archive
// ---------------------------------------------------------------------------

func displayStatus(t *TaskInfo, allStatuses map[string]string) string {
	if t.Status == "planning" && len(t.Children) > 0 {
		for _, c := range t.Children {
			if st, ok := allStatuses[c]; ok && st != "planning" {
				return "active"
			}
		}
	}
	return t.Status
}

// List 列出活动任务（对应 task.py cmd_list）。
func (r *Runner) List(mine bool, status string, asJSON bool) int {
	if !r.requireRoot() {
		return 1
	}
	tasksDir := TasksDir(r.Root)
	developer := Developer(r.Root)
	all := IterActiveTasks(tasksDir, fileWriter(r.errOut()))
	byName := map[string]*TaskInfo{}
	for _, t := range all {
		byName[t.DirName] = t
	}
	allStatuses := AllStatuses(all)
	current := ResolveActiveTask(r.Root, "current", "")
	if asJSON {
		if mine && developer == "" {
			fmt.Fprintln(r.errOut(), `{"error": "未设置开发者", "hint": "请先运行 sunshine init，或设置 SUNSHINE_DEVELOPER"}`)
			return 1
		}
		type item struct {
			Dir           string   `json:"dir"`
			ID            string   `json:"id"`
			Title         string   `json:"title"`
			Status        string   `json:"status"`
			DisplayStatus string   `json:"display_status"`
			Priority      string   `json:"priority"`
			Assignee      *string  `json:"assignee"`
			Parent        *string  `json:"parent"`
			Children      []string `json:"children"`
			Package       *string  `json:"package"`
		}
		var items []item
		names := make([]string, 0, len(byName))
		for name := range byName {
			names = append(names, name)
		}
		sort.Strings(names)
		for _, name := range names {
			t := byName[name]
			assignee := t.Assignee
			if mine && (orDash(assignee) != developer) {
				continue
			}
			if status != "" && t.Status != status {
				continue
			}
			id := t.Name
			if id == "" {
				id = name
			}
			var assigneePtr *string
			if assignee != "" {
				assigneePtr = &assignee
			}
			var parentPtr *string
			if t.Parent != "" {
				parentPtr = &t.Parent
			}
			var pkgPtr *string
			if t.Package != "" {
				pkgPtr = &t.Package
			}
			children := t.Children
			if children == nil {
				children = []string{}
			}
			items = append(items, item{
				Dir: DirSunshine + "/" + DirTasks + "/" + name, ID: id, Title: t.Title,
				Status: t.Status, DisplayStatus: displayStatus(t, allStatuses),
				Priority: t.Priority, Assignee: assigneePtr, Parent: parentPtr,
				Children: children, Package: pkgPtr,
			})
		}
		if items == nil {
			items = []item{}
		}
		printJSON(r.out(), map[string]any{"tasks": items})
		return 0
	}
	if mine {
		if developer == "" {
			r.errf("未设置开发者。请先运行 sunshine init。")
			fmt.Fprintln(r.errOut(), DeveloperHint)
			return 1
		}
		fmt.Fprintf(r.out(), "我的任务（负责人：%s）：\n\n", developer)
	} else {
		fmt.Fprintln(r.out(), "全部活动任务：")
		fmt.Fprintln(r.out())
	}
	count := 0
	var printTask func(name string, indent int)
	printTask = func(name string, indent int) {
		t := byName[name]
		if mine && orDash(t.Assignee) != developer {
			return
		}
		if status != "" && t.Status != status {
			return
		}
		rel := DirSunshine + "/" + DirTasks + "/" + name
		marker := ""
		if rel == current.TaskPath {
			marker = " <- 当前任务"
		}
		progress := ChildrenProgress(t.Children, allStatuses)
		statusLabel := displayStatus(t, allStatuses)
		pkgTag := ""
		if t.Package != "" {
			pkgTag = " @" + t.Package
		}
		prefix := strings.Repeat("  ", indent) + "  - "
		if mine {
			fmt.Fprintf(r.out(), "%s%s/ (%s)%s%s%s\n", prefix, name, statusLabel, pkgTag, progress, marker)
		} else {
			fmt.Fprintf(r.out(), "%s%s/ (%s)%s%s [%s]%s\n", prefix, name, statusLabel, pkgTag, progress, orDash(t.Assignee), marker)
		}
		count++
		for _, child := range t.Children {
			if _, ok := byName[child]; ok {
				printTask(child, indent+1)
			}
		}
	}
	names := make([]string, 0, len(byName))
	for name := range byName {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		parent := byName[name].Parent
		if parent == "" {
			printTask(name, 0)
			continue
		}
		if _, ok := byName[parent]; !ok {
			printTask(name, 0)
		}
	}
	if count == 0 {
		if mine {
			fmt.Fprintln(r.out(), "  （没有分配给你的任务）")
		} else {
			fmt.Fprintln(r.out(), "  （没有活动任务）")
		}
	}
	fmt.Fprintln(r.out())
	fmt.Fprintf(r.out(), "总计：%d 个任务\n", count)
	return 0
}

func orDash(s string) string {
	if s == "" {
		return "-"
	}
	return s
}

// ListArchive 列出已归档任务（对应 task.py cmd_list_archive）。
func (r *Runner) ListArchive(month string) int {
	if !r.requireRoot() {
		return 1
	}
	tasksDir := TasksDir(r.Root)
	archiveDir := filepath.Join(tasksDir, DirArchive)
	fmt.Fprintln(r.out(), "已归档任务：")
	fmt.Fprintln(r.out())
	if month != "" {
		monthDir := filepath.Join(archiveDir, month)
		if st, err := os.Stat(monthDir); err == nil && st.IsDir() {
			fmt.Fprintf(r.out(), "[%s]\n", month)
			entries, _ := os.ReadDir(monthDir)
			for _, e := range entries {
				if e.IsDir() {
					fmt.Fprintf(r.out(), "  - %s/\n", e.Name())
				}
			}
		} else {
			fmt.Fprintf(r.out(), "  该月无归档：%s\n", month)
		}
		return 0
	}
	entries, _ := os.ReadDir(archiveDir)
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		sub, _ := os.ReadDir(filepath.Join(archiveDir, e.Name()))
		var names []string
		for _, d := range sub {
			if d.IsDir() {
				names = append(names, d.Name())
			}
		}
		fmt.Fprintf(r.out(), "[%s] - %d 个任务\n", e.Name(), len(names))
		for _, name := range names {
			fmt.Fprintf(r.out(), "  - %s/\n", name)
		}
	}
	return 0
}

// ---------------------------------------------------------------------------
// set-* / set-meta
// ---------------------------------------------------------------------------

func (r *Runner) setField(dir, field, value string, okMsg string) int {
	if !r.requireRoot() {
		return 1
	}
	fullPath := r.resolveOrError(dir)
	if fullPath == "" {
		return 1
	}
	taskJSONPath := filepath.Join(fullPath, FileTaskJSON)
	if _, err := os.Stat(taskJSONPath); err != nil {
		r.errf("task.json 不存在：%s", fullPath)
		return 1
	}
	if err := updateTaskJSON(fullPath, func(obj *rawObject) error {
		obj.set(field, value)
		return nil
	}); err != nil {
		r.errf("%v", err)
		return 1
	}
	fmt.Fprintln(r.errOut(), okMsg)
	return 0
}

// SetBranch 设置任务分支。
func (r *Runner) SetBranch(dir, branch string) int {
	if branch == "" {
		r.errf("缺少参数")
		fmt.Fprintln(r.errOut(), "用法：sunshine task set-branch <任务目录> <分支名>")
		return 1
	}
	return r.setField(dir, "branch", branch, "✓ 分支已设为："+branch)
}

// SetBaseBranch 设置 PR 目标分支。
func (r *Runner) SetBaseBranch(dir, baseBranch string) int {
	if baseBranch == "" {
		r.errf("缺少参数")
		fmt.Fprintln(r.errOut(), "用法：sunshine task set-base-branch <任务目录> <目标分支>")
		fmt.Fprintln(r.errOut(), "示例：sunshine task set-base-branch <任务目录> develop")
		fmt.Fprintln(r.errOut())
		fmt.Fprintln(r.errOut(), "设置 PR 的目标分支（功能分支将要合并进去的分支）。")
		return 1
	}
	code := r.setField(dir, "base_branch", baseBranch, "✓ 目标分支已设为："+baseBranch)
	if code == 0 {
		fmt.Fprintf(r.errOut(), "  PR 将指向：%s\n", baseBranch)
	}
	return code
}

// SetScope 设置 PR 标题的 scope。
func (r *Runner) SetScope(dir, scope string) int {
	if scope == "" {
		r.errf("缺少参数")
		fmt.Fprintln(r.errOut(), "用法：sunshine task set-scope <任务目录> <scope>")
		return 1
	}
	return r.setField(dir, "scope", scope, "✓ Scope 已设为："+scope)
}

// SetMeta 覆盖指定元数据键（对应 task_store.cmd_set_meta）。
func (r *Runner) SetMeta(dir, key, value string) int {
	if !r.requireRoot() {
		return 1
	}
	if key == "" {
		r.errf("缺少参数")
		fmt.Fprintln(r.errOut(), "用法：sunshine task set-meta <任务目录> <键> <值>")
		return 1
	}
	fullPath := r.resolveOrError(dir)
	if fullPath == "" {
		return 1
	}
	taskJSONPath := filepath.Join(fullPath, FileTaskJSON)
	if _, err := os.Stat(taskJSONPath); err != nil {
		r.errf("task.json 不存在：%s", fullPath)
		return 1
	}
	if err := updateTaskJSON(fullPath, func(obj *rawObject) error {
		var meta map[string]any
		if raw, ok := obj.vals["meta"]; ok {
			_ = json.Unmarshal(raw, &meta)
		}
		if meta == nil {
			meta = map[string]any{}
		}
		meta[key] = value
		obj.set("meta", meta)
		return nil
	}); err != nil {
		r.errf("%v", err)
		return 1
	}
	fmt.Fprintf(r.errOut(), "✓ 元数据已设置：%s = %s\n", key, value)
	return 0
}

// ---------------------------------------------------------------------------
// add-subtask / remove-subtask
// ---------------------------------------------------------------------------

func (r *Runner) linkSubtask(parentArg, childArg string, link bool) int {
	if !r.requireRoot() {
		return 1
	}
	parentDir := r.resolveOrError(parentArg)
	if parentDir == "" {
		return 1
	}
	childDir := r.resolveOrError(childArg)
	if childDir == "" {
		return 1
	}
	parentJSON := filepath.Join(parentDir, FileTaskJSON)
	childJSON := filepath.Join(childDir, FileTaskJSON)
	if _, err := os.Stat(parentJSON); err != nil {
		r.errf("父任务的 task.json 不存在：%s", parentArg)
		return 1
	}
	if _, err := os.Stat(childJSON); err != nil {
		r.errf("子任务的 task.json 不存在：%s", childArg)
		return 1
	}
	parentObj, parentReason, err := readRawObject(parentJSON)
	if err != nil {
		problem, hint := describeReadFailure(parentJSON, parentReason)
		r.errf("%s\n%s", problem, hint)
		return 1
	}
	childObj, childReason, err := readRawObject(childJSON)
	if err != nil {
		problem, hint := describeReadFailure(childJSON, childReason)
		r.errf("%s\n%s", problem, hint)
		return 1
	}
	parentName := filepath.Base(parentDir)
	childName := filepath.Base(childDir)
	if link {
		if existing, _ := childObj.getStringOrNull("parent"); existing != "" {
			r.errf("子任务已有父任务：%s", existing)
			return 1
		}
		children := parentObj.ensureStringList("children")
		found := false
		for _, c := range children {
			if c == childName {
				found = true
			}
		}
		if !found {
			children = append(children, childName)
			parentObj.setStringList("children", children)
		}
		childObj.set("parent", parentName)
		if err := writeRawObject(parentJSON, parentObj); err != nil {
			r.errf("写入父任务 task.json 失败：%v", err)
			fmt.Fprintln(r.errOut(), "未写入任何链接。")
			return 1
		}
		if err := writeRawObject(childJSON, childObj); err != nil {
			r.errf("写入子任务 task.json 失败：%v", err)
			fmt.Fprintf(r.errOut(), "链接写了一半：%s 已列出 %q，但子任务未记录父任务。\n", parentJSON, childName)
			return 1
		}
		fmt.Fprintf(r.errOut(), "已关联：%s -> %s\n", childName, parentName)
		return 0
	}
	children := parentObj.ensureStringList("children")
	kept := children[:0]
	for _, c := range children {
		if c != childName {
			kept = append(kept, c)
		}
	}
	parentObj.setStringList("children", kept)
	childObj.setNull("parent")
	if err := writeRawObject(parentJSON, parentObj); err != nil {
		r.errf("写入父任务 task.json 失败：%v", err)
		fmt.Fprintln(r.errOut(), "未解除任何链接。")
		return 1
	}
	if err := writeRawObject(childJSON, childObj); err != nil {
		r.errf("写入子任务 task.json 失败：%v", err)
		fmt.Fprintf(r.errOut(), "解除写了一半：%s 已不再列出 %q，但子任务仍记录着父任务。\n", parentJSON, childName)
		return 1
	}
	fmt.Fprintf(r.errOut(), "已解除关联：%s（原父任务 %s）\n", childName, parentName)
	return 0
}

// AddSubtask 链接子任务。
func (r *Runner) AddSubtask(parent, child string) int { return r.linkSubtask(parent, child, true) }

// RemoveSubtask 解除子任务链接。
func (r *Runner) RemoveSubtask(parent, child string) int {
	return r.linkSubtask(parent, child, false)
}

// ---------------------------------------------------------------------------
// add-context / validate / list-context
// ---------------------------------------------------------------------------

// AddContext 向 implement/check.jsonl 加一条上下文（对应 task_context.cmd_add_context）。
func (r *Runner) AddContext(dir, file, path, reason string) int {
	if !r.requireRoot() {
		return 1
	}
	fullPath := r.resolveOrError(dir)
	if fullPath == "" {
		return 1
	}
	if reason == "" {
		reason = "手动添加"
	}
	jsonlName := file
	if strings.ContainsAny(jsonlName, "/\\") || jsonlName == "." || jsonlName == ".." {
		r.errf("上下文文件必须是纯文件名（如 implement、check）：%s", jsonlName)
		return 1
	}
	if !strings.HasSuffix(jsonlName, ".jsonl") {
		jsonlName += ".jsonl"
	}
	jsonlPath := filepath.Join(fullPath, jsonlName)
	fullTarget := filepath.Join(r.Root, filepath.FromSlash(path))
	entryType := "file"
	targetPath := path
	if st, err := os.Stat(fullTarget); err == nil && st.IsDir() {
		entryType = "directory"
		if !strings.HasSuffix(targetPath, "/") {
			targetPath += "/"
		}
	} else if err != nil || !fileExists(fullTarget) {
		r.errf("路径不存在：%s", path)
		return 1
	}
	if data, err := os.ReadFile(jsonlPath); err == nil {
		if strings.Contains(string(data), "\""+targetPath+"\"") {
			r.warnf("该路径已有条目：%s", targetPath)
			return 0
		}
	}
	entry := jsonlEntry{File: targetPath, Reason: reason}
	if entryType == "directory" {
		entry.Type = "directory"
	}
	line, _ := json.Marshal(entry)
	f, err := os.OpenFile(jsonlPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		r.errf("写入 %s 失败：%v", jsonlPath, err)
		return 1
	}
	defer f.Close()
	if _, err := f.Write(append(line, '\n')); err != nil {
		r.errf("写入 %s 失败：%v", jsonlPath, err)
		return 1
	}
	if entryType == "directory" {
		fmt.Fprintf(r.errOut(), "已添加目录：%s\n", targetPath)
	} else {
		fmt.Fprintf(r.errOut(), "已添加文件：%s\n", targetPath)
	}
	return 0
}

func fileExists(path string) bool {
	st, err := os.Stat(path)
	return err == nil && !st.IsDir()
}

// Validate 校验任务的 jsonl 上下文文件（对应 task_context.cmd_validate）。
func (r *Runner) Validate(dir string) int {
	if !r.requireRoot() {
		return 1
	}
	fullPath := r.resolveOrError(dir)
	if fullPath == "" {
		return 1
	}
	fmt.Fprintln(r.out(), "=== 校验上下文文件 ===")
	fmt.Fprintf(r.out(), "目标目录：%s\n\n", fullPath)
	taskJSONPath := filepath.Join(fullPath, FileTaskJSON)
	if obj, _, err := readRawObject(taskJSONPath); err == nil {
		if branch, _ := obj.getStringOrNull("branch"); branch != "" && !BranchExistsLocal(r.Root, branch) {
			fmt.Fprintf(r.out(), "警告：记录的分支 %q 已不存在（可能已合并删除）。\n\n", branch)
		}
	}
	totalErrors := 0
	for _, name := range []string{"implement.jsonl", "check.jsonl"} {
		totalErrors += r.validateJSONL(filepath.Join(fullPath, name), fullPath)
	}
	fmt.Fprintln(r.out())
	if totalErrors == 0 {
		fmt.Fprintln(r.out(), "✓ 全部校验通过")
		return 0
	}
	fmt.Fprintf(r.out(), "✗ 校验失败（%d 个错误）\n", totalErrors)
	return 1
}

func (r *Runner) validateJSONL(jsonlPath, taskDir string) int {
	name := filepath.Base(jsonlPath)
	data, err := os.ReadFile(jsonlPath)
	if err != nil {
		fmt.Fprintf(r.out(), "  %s：不存在（已跳过）\n", name)
		return 0
	}
	taskRel := RepoRelative(r.Root, taskDir)
	maxFileBytes := LoadConfig(r.Root).MaxFileBytes
	errors := 0
	realEntries := 0
	lines := strings.Split(string(data), "\n")
	for i, line := range lines {
		lineNum := i + 1
		if strings.TrimSpace(line) == "" {
			continue
		}
		var row map[string]any
		if err := json.Unmarshal([]byte(line), &row); err != nil {
			fmt.Fprintf(r.out(), "  %s:%d：JSON 无效\n", name, lineNum)
			errors++
			continue
		}
		if _, ok := row["_example"]; ok {
			fmt.Fprintf(r.out(), "  %s:%d：遗留的 `_example` 占位行——请删除该行，或换成 {\"file\": \"<路径>\", \"reason\": \"<原因>\"}\n", name, lineNum)
			errors++
			continue
		}
		filePath, _ := row["file"].(string)
		if filePath == "" {
			continue
		}
		entryType, _ := row["type"].(string)
		realEntries++
		fullPath := resolveContextEntryPath(filePath, r.Root, taskDir)
		if fullPath == "" {
			fmt.Fprintf(r.out(), "  %s:%d：路径越界：%s\n", name, lineNum, filePath)
			errors++
			continue
		}
		if entryType == "directory" {
			if st, err := os.Stat(fullPath); err != nil || !st.IsDir() {
				fmt.Fprintf(r.out(), "  %s:%d：目录不存在：%s\n", name, lineNum, filePath)
				errors++
			}
			continue
		}
		if !fileExists(fullPath) {
			fmt.Fprintf(r.out(), "  %s:%d：文件不存在：%s\n", name, lineNum, filePath)
			errors++
			continue
		}
		if isCodeFileExt(filePath) && !isExemptFromCodeFileWarning(filePath, taskRel) {
			fmt.Fprintf(r.out(), "  %s:%d：警告：%s 看起来是代码文件——implement/check.jsonl 应引用规范/调研文档，代码由 agent 自行阅读\n", name, lineNum, filePath)
		}
		if maxFileBytes > 0 {
			if st, err := os.Stat(fullPath); err == nil && st.Size() > int64(maxFileBytes) {
				fmt.Fprintf(r.out(), "  %s:%d：警告：%s 有 %d 字节，超过 context_injection.max_file_bytes（%d），注入时会被截断\n", name, lineNum, filePath, st.Size(), maxFileBytes)
			}
		}
	}
	if errors == 0 && realEntries == 0 {
		action := strings.SplitN(name, ".", 2)[0]
		fmt.Fprintf(r.out(), "  %s：✗（0 条已整理条目——子代理将得不到任何规范上下文）\n", name)
		fmt.Fprintf(r.out(), "    整理：sunshine task add-context <任务> %s <路径> \"<原因>\"\n", action)
		fmt.Fprintln(r.out(), "    确实要空着：start 时加 --allow-empty-context")
		return 1
	}
	if errors == 0 {
		fmt.Fprintf(r.out(), "  %s：✓（%d 条）\n", name, realEntries)
	} else {
		fmt.Fprintf(r.out(), "  %s：✗（%d 个错误）\n", name, errors)
	}
	return errors
}

// resolveContextEntryPath 解析 jsonl 条目路径；归档任务里指向旧任务
// 目录的自引用重定向到归档副本（对应 _resolve_context_entry_path）。
func resolveContextEntryPath(filePath, root, taskDir string) string {
	repoPath := filepath.Join(root, filepath.FromSlash(filePath))
	taskRel, err := filepath.Rel(root, taskDir)
	if err != nil {
		return repoPath
	}
	parts := strings.Split(filepath.ToSlash(taskRel), "/")
	if len(parts) != 5 || parts[0] != DirSunshine || parts[1] != DirTasks || parts[2] != DirArchive {
		return repoPath
	}
	yearMonth := parts[3]
	if len(yearMonth) != 7 || yearMonth[4] != '-' {
		return repoPath
	}
	historicalRoot := DirSunshine + "/" + DirTasks + "/" + parts[4]
	posix := strings.ReplaceAll(filePath, "\\", "/")
	var relativeParts []string
	switch {
	case posix == historicalRoot:
		relativeParts = nil
	case strings.HasPrefix(posix, historicalRoot+"/"):
		rel := strings.TrimSuffix(posix[len(historicalRoot)+1:], "/")
		if rel != "" {
			relativeParts = strings.Split(rel, "/")
		}
	default:
		return repoPath
	}
	for _, part := range relativeParts {
		if part == "" || part == "." || part == ".." {
			return ""
		}
	}
	resolved := taskDir
	for _, part := range relativeParts {
		resolved = filepath.Join(resolved, part)
	}
	if abs, err := filepath.EvalSymlinks(resolved); err == nil {
		if rel, err := filepath.Rel(taskDir, abs); err == nil && !strings.HasPrefix(rel, "..") {
			return abs
		}
		return ""
	}
	return resolved
}

// ListContext 列出任务的 jsonl 条目（对应 task_context.cmd_list_context）。
func (r *Runner) ListContext(dir string) int {
	if !r.requireRoot() {
		return 1
	}
	fullPath := r.resolveOrError(dir)
	if fullPath == "" {
		return 1
	}
	fmt.Fprintln(r.out(), "=== 上下文文件 ===")
	fmt.Fprintln(r.out())
	for _, name := range []string{"implement.jsonl", "check.jsonl"} {
		jsonlPath := filepath.Join(fullPath, name)
		data, err := os.ReadFile(jsonlPath)
		if err != nil {
			continue
		}
		fmt.Fprintf(r.out(), "[%s]\n", name)
		count := 0
		curated := false
		for _, line := range strings.Split(string(data), "\n") {
			if strings.TrimSpace(line) == "" {
				continue
			}
			var row map[string]any
			if err := json.Unmarshal([]byte(line), &row); err != nil {
				continue
			}
			filePath, _ := row["file"].(string)
			if filePath == "" {
				continue
			}
			curated = true
			count++
			entryType, _ := row["type"].(string)
			reason, _ := row["reason"].(string)
			if reason == "" {
				reason = "-"
			}
			if entryType == "directory" {
				fmt.Fprintf(r.out(), "  %d. [目录] %s\n", count, filePath)
			} else {
				fmt.Fprintf(r.out(), "  %d. %s\n", count, filePath)
			}
			fmt.Fprintf(r.out(), "     → %s\n", reason)
		}
		if !curated {
			fmt.Fprintln(r.out(), "  （暂无已整理条目）")
		}
		fmt.Fprintln(r.out())
	}
	return 0
}

// ---------------------------------------------------------------------------
// rename
// ---------------------------------------------------------------------------

func splitDatePrefix(dirName string) (prefix, slug string) {
	m := regexp.MustCompile(`^(\d{2})-(\d{2})-(.+)$`).FindStringSubmatch(dirName)
	if m == nil {
		return "", dirName
	}
	month, day := atoi2(m[1]), atoi2(m[2])
	if month < 1 || month > 12 || day < 1 || day > 31 {
		return "", dirName
	}
	return m[1] + "-" + m[2], m[3]
}

func validateRenameSlug(slug, datePrefix string, errOut io.Writer) (string, bool) {
	if slug == "" {
		fmt.Fprintln(errOut, "错误：新 slug 必填")
		return "", false
	}
	if strings.ContainsAny(slug, "/\\") || strings.Contains(slug, "..") {
		fmt.Fprintf(errOut, "错误：<新 slug> 必须是纯名字，不能含路径分隔符或 ..：%s\n", slug)
		return "", false
	}
	prefix, body := splitDatePrefix(slug)
	if prefix == "" {
		return slug, true
	}
	if prefix == datePrefix {
		fmt.Fprintf(errOut, "警告：<新 slug> 不应带 MM-DD 前缀，已规范化为 %q\n", body)
		return body, true
	}
	fmt.Fprintf(errOut, "错误：<新 slug> 以日期前缀（%s-）开头，但 rename 保留任务自己的创建日期（%s）。\n", prefix, datePrefix)
	fmt.Fprintf(errOut, "请只传 slug 本体，例如 %s\n", body)
	return "", false
}

type renamePlan struct {
	oldName, newName string
	oldRel, newRel   string
	identity         [][3]string // field, old, new
	jsonl            []jsonlRewrite
	backrefs         []backrefChange
	reported         [][2]any // path, lineno
	unreadable       []string
}

type jsonlRewrite struct {
	path    string
	linenos []int
	text    string
}

type backrefChange struct {
	path   string
	labels []string
	data   *rawObject
}

// Rename 重命名任务及全部引用（对应 task_store.cmd_rename）。
func (r *Runner) Rename(name, newSlug string, dryRun bool) int {
	if !r.requireRoot() {
		return 1
	}
	taskDir := r.resolveOrError(name)
	if taskDir == "" {
		return 1
	}
	tasksDir := TasksDir(r.Root)
	if !IsWithinTasksDir(taskDir, tasksDir) {
		r.errf("拒绝重命名 %q：%s 不是 %s 下的活动任务", name, taskDir, tasksDir)
		return 1
	}
	oldName := filepath.Base(taskDir)
	datePrefix, _ := splitDatePrefix(oldName)
	slug, ok := validateRenameSlug(newSlug, datePrefix, r.errOut())
	if !ok {
		return 1
	}
	newName := slug
	if datePrefix != "" {
		newName = datePrefix + "-" + slug
	}
	if newName == oldName {
		r.errf("%q 已是该任务的当前名字", newName)
		return 1
	}
	newDir := filepath.Join(filepath.Dir(taskDir), newName)
	if _, err := os.Stat(newDir); err == nil {
		r.errf("任务已存在：%s", newName)
		fmt.Fprintf(r.errOut(), "已有任务在：%s\n请换一个 slug。\n", RepoRelative(r.Root, newDir))
		return 1
	}
	if archived := FindArchivedTaskByDirName(tasksDir, newName); archived != "" {
		r.errf("任务已归档：%s", newName)
		fmt.Fprintf(r.errOut(), "归档位置：%s\n请换一个 slug。\n", RepoRelative(r.Root, archived))
		return 1
	}
	taskJSONPath := filepath.Join(taskDir, FileTaskJSON)
	if _, err := os.Stat(taskJSONPath); err != nil {
		r.errf("task.json 不存在：%s", taskDir)
		return 1
	}
	taskObj, reason, err := readRawObject(taskJSONPath)
	if err != nil {
		problem, hint := describeReadFailure(taskJSONPath, reason)
		r.errf("%s\n%s", problem, hint)
		fmt.Fprintln(r.errOut(), "未重命名任何内容。")
		return 1
	}
	oldRel := RepoRelative(r.Root, taskDir)
	newRel := RepoRelative(r.Root, newDir)
	plan := &renamePlan{oldName: oldName, newName: newName, oldRel: oldRel, newRel: newRel}
	for _, field := range []string{"id", "name"} {
		if current, _ := taskObj.getString(field); current != slug {
			plan.identity = append(plan.identity, [3]string{field, current, slug})
		}
	}
	// jsonl 里指向旧任务目录的条目。
	needle := oldRel + "/"
	replacement := newRel + "/"
	for _, jsonlName := range []string{"implement.jsonl", "check.jsonl"} {
		p := filepath.Join(taskDir, jsonlName)
		data, err := os.ReadFile(p)
		if err != nil {
			continue
		}
		lines := strings.SplitAfter(string(data), "\n")
		var changed []int
		for i, line := range lines {
			if strings.Contains(line, needle) {
				lines[i] = strings.ReplaceAll(line, needle, replacement)
				changed = append(changed, i+1)
			}
		}
		if len(changed) > 0 {
			plan.jsonl = append(plan.jsonl, jsonlRewrite{path: p, linenos: changed, text: strings.Join(lines, "")})
		}
	}
	// 其他活动任务的反向引用（parent / children / subtasks）。
	entries, _ := os.ReadDir(tasksDir)
	for _, e := range entries {
		if !e.IsDir() || e.Name() == DirArchive || e.Name() == oldName {
			continue
		}
		jsonPath := filepath.Join(tasksDir, e.Name(), FileTaskJSON)
		if _, err := os.Stat(jsonPath); err != nil {
			continue
		}
		other, _, err := readRawObject(jsonPath)
		if err != nil {
			plan.unreadable = append(plan.unreadable, jsonPath)
			continue
		}
		var labels []string
		if parent, _ := other.getStringOrNull("parent"); parent == oldName {
			other.set("parent", newName)
			labels = append(labels, "parent")
		}
		for _, listField := range []string{"children", "subtasks"} {
			values := other.getStringList(listField)
			if values == nil {
				continue
			}
			changed := false
			for i, v := range values {
				if v == oldName {
					values[i] = newName
					labels = append(labels, fmt.Sprintf("%s[%d]", listField, i))
					changed = true
				}
			}
			if changed {
				other.setStringList(listField, values)
			}
		}
		if len(labels) > 0 {
			plan.backrefs = append(plan.backrefs, backrefChange{path: jsonPath, labels: labels, data: other})
		}
	}
	// 其余 .sunshine 下的旧名提及：只报告，不改写。
	rewritten := map[string]bool{}
	for _, j := range plan.jsonl {
		rewritten[j.path] = true
	}
	for _, b := range plan.backrefs {
		rewritten[b.path] = true
	}
	plan.reported = r.findReportedRefs(taskDir, rewritten, oldName)
	// 输出计划（试运行与实跑同一份）。
	fmt.Fprintf(r.out(), "重命名：%s -> %s\n", oldName, newName)
	fmt.Fprintf(r.out(), "  目录：%s -> %s\n", oldRel, newRel)
	for _, id := range plan.identity {
		fmt.Fprintf(r.out(), "  task.json：%s：%s -> %s\n", id[0], id[1], id[2])
	}
	for _, b := range plan.backrefs {
		for _, label := range b.labels {
			fmt.Fprintf(r.out(), "  反向引用：%s：%s：%s -> %s\n", RepoRelative(r.Root, b.path), label, oldName, newName)
		}
	}
	for _, j := range plan.jsonl {
		for _, lineno := range j.linenos {
			fmt.Fprintf(r.out(), "  jsonl：%s:%d：%s/ -> %s/\n", RepoRelative(r.Root, j.path), lineno, oldRel, newRel)
		}
	}
	for _, rep := range plan.reported {
		fmt.Fprintf(r.out(), "  仍引用旧名（未改写）：%s:%v\n", RepoRelative(r.Root, rep[0].(string)), rep[1])
	}
	fmt.Fprintf(r.out(), "  会话：指向 %s 的当前任务指针将改指 %s\n", oldRel, newRel)
	for _, u := range plan.unreadable {
		fmt.Fprintf(r.errOut(), "警告：%s 无法读取，其中对旧名的引用保持原样。\n", RepoRelative(r.Root, u))
	}
	if dryRun {
		fmt.Fprintln(r.errOut(), "试运行：未写入任何内容。")
		return 0
	}
	// 应用：先写原地编辑（可重跑），最后移动目录，再改会话指针。
	if len(plan.identity) > 0 {
		if err := updateTaskJSON(taskDir, func(obj *rawObject) error {
			for _, id := range plan.identity {
				obj.set(id[0], id[2])
			}
			return nil
		}); err != nil {
			r.errf("%v", err)
			fmt.Fprintln(r.errOut(), "未重命名任何内容。")
			return 1
		}
	}
	for _, j := range plan.jsonl {
		if err := writeFileAtomic(j.path, []byte(j.text), 0o644); err != nil {
			r.errf("写入 %s 失败：%v", j.path, err)
			fmt.Fprintf(r.errOut(), "任务仍在 %s，未移动。修复写入失败后重新运行同一条 rename 即可完成。\n", oldRel)
			return 1
		}
	}
	for _, b := range plan.backrefs {
		if err := writeRawObject(b.path, b.data); err != nil {
			r.errf("写入 %s 失败：%v", b.path, err)
			fmt.Fprintf(r.errOut(), "任务仍在 %s，未移动。修复写入失败后重新运行同一条 rename 即可完成。\n", oldRel)
			return 1
		}
	}
	if err := os.Rename(taskDir, newDir); err != nil {
		r.errf("移动 %s -> %s 失败：%v", oldRel, newRel, err)
		fmt.Fprintf(r.errOut(), "任务仍在 %s，未移动。修复写入失败后重新运行同一条 rename 即可完成。\n", oldRel)
		return 1
	}
	RepointTaskInSessions(r.Root, oldRel, newRel)
	fmt.Fprintf(r.errOut(), "已重命名：%s -> %s\n", oldName, newName)
	if len(plan.reported) > 0 {
		fmt.Fprintf(r.errOut(), "%d 处 .sunshine/ 下的其他引用仍用旧名（已在上方列出，未改写）。\n", len(plan.reported))
	}
	return 0
}

// findReportedRefs 找 .sunshine 下除任务自身、已改写文件与会话目录外
// 仍以词边界提及旧名的文件位置（只报告，不改写）。
func (r *Runner) findReportedRefs(taskDir string, rewritten map[string]bool, oldName string) [][2]any {
	var hits [][2]any
	sunshineDir := filepath.Join(r.Root, DirSunshine)
	sessionsDir := sessionsDir(r.Root)
	_ = filepath.WalkDir(sunshineDir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			name := d.Name()
			if strings.HasPrefix(name, ".backup-") || name == "__pycache__" {
				return filepath.SkipDir
			}
			return nil
		}
		if rewritten[path] {
			return nil
		}
		if path == taskDir || strings.HasPrefix(path, taskDir+string(filepath.Separator)) {
			return nil
		}
		if filepath.Dir(path) == sessionsDir {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return nil
		}
		for i, line := range strings.Split(string(data), "\n") {
			if containsWordBoundary(line, oldName) {
				hits = append(hits, [2]any{path, i + 1})
			}
		}
		return nil
	})
	return hits
}

// containsWordBoundary 判断行里是否以词边界出现 needle（RE2 不支持
// look-around，这里手工检查两侧字符）。
func containsWordBoundary(line, needle string) bool {
	if needle == "" {
		return false
	}
	isWord := func(c byte) bool {
		return c >= '0' && c <= '9' || c >= 'A' && c <= 'Z' || c >= 'a' && c <= 'z' || c == '_' || c == '-'
	}
	for i := 0; i+len(needle) <= len(line); i++ {
		if line[i:i+len(needle)] != needle {
			continue
		}
		if i > 0 && isWord(line[i-1]) {
			continue
		}
		if i+len(needle) < len(line) && isWord(line[i+len(needle)]) {
			continue
		}
		return true
	}
	return false
}

// ---------------------------------------------------------------------------
// archive
// ---------------------------------------------------------------------------

// validateBranchMetadata 归档前的分支元数据校验（对应 _validate_branch_metadata）。
func (r *Runner) validateBranchMetadata(data *rawObject, taskName string, skip bool) bool {
	branch, _ := data.getStringOrNull("branch")
	baseBranch, _ := data.getStringOrNull("base_branch")
	if branch != "" && !BranchExistsLocal(r.Root, branch) {
		r.warnf("记录的分支 %q 已不存在（可能已合并删除）。", branch)
	}
	if skip {
		return true
	}
	if branch != "" && baseBranch != "" && branch == baseBranch {
		r.errf("拒绝归档 %q：branch 与 base_branch 都是 %q，PR 不能指向自身分支，这份元数据无法描述已合并的工作。", taskName, branch)
		fmt.Fprintln(r.errOut(), "请修复其中一个字段：")
		fmt.Fprintf(r.errOut(), "  sunshine task set-branch %s <功能分支>\n", taskName)
		fmt.Fprintf(r.errOut(), "  sunshine task set-base-branch %s <目标分支>\n", taskName)
		fmt.Fprintf(r.errOut(), "  sunshine task archive %s --skip-branch-validation   # 仅当该任务从未走 PR\n", taskName)
		return false
	}
	if branch == "" && baseBranch != "" && HasRemote(r.Root) {
		r.errf("拒绝归档 %q：未记录 branch，但任务指向 base_branch %q，且仓库配了 remote——它基于哪个分支干活从未写下来。", taskName, baseBranch)
		fmt.Fprintln(r.errOut(), "请修复：")
		fmt.Fprintf(r.errOut(), "  sunshine task set-branch %s <分支>\n", taskName)
		fmt.Fprintf(r.errOut(), "  sunshine task archive %s --skip-branch-validation   # 仅当工作未走自有分支落地\n", taskName)
		return false
	}
	return true
}

// Archive 归档已完成任务（对应 task_store.cmd_archive）。
func (r *Runner) Archive(name string, noCommit, skipBranchValidation bool) int {
	if !r.requireRoot() {
		return 1
	}
	if name == "" {
		r.errf("任务名必填")
		return 1
	}
	tasksDir := TasksDir(r.Root)
	taskDir := r.resolveOrError(name)
	if taskDir == "" {
		fmt.Fprintln(r.errOut(), "活动任务：")
		for _, t := range IterActiveTasks(tasksDir, fileWriter(r.errOut())) {
			fmt.Fprintf(r.errOut(), "  - %s/\n", t.DirName)
		}
		return 1
	}
	if !IsWithinTasksDir(taskDir, tasksDir) {
		r.errf("拒绝归档 %q：%s 不是 %s 下的任务", name, taskDir, tasksDir)
		return 1
	}
	dest := ArchiveDestinationFor(taskDir)
	if _, err := os.Stat(dest); err == nil {
		r.errf("拒绝归档 %q：归档目标已存在：%s", name, RepoRelative(r.Root, dest))
		fmt.Fprintf(r.errOut(), "任务仍在：%s\n请先移走或重命名已归档的同名任务，再重试。\n", RepoRelative(r.Root, taskDir))
		return 1
	}
	dirName := filepath.Base(taskDir)
	taskJSONPath := filepath.Join(taskDir, FileTaskJSON)
	var modifiedChildren []string
	if _, err := os.Stat(taskJSONPath); err == nil {
		data, reason, err := readRawObject(taskJSONPath)
		if err != nil {
			problem, _ := describeReadFailure(taskJSONPath, reason)
			r.warnf("%s；归档时不更新状态/子女关系。", problem)
		} else {
			if !r.validateBranchMetadata(data, name, skipBranchValidation) {
				fmt.Fprintf(r.errOut(), "未归档：%s 未动。\n", RepoRelative(r.Root, taskDir))
				return 1
			}
			children := data.getStringList("children")
			if err := updateTaskJSON(taskDir, func(obj *rawObject) error {
				obj.set("status", "completed")
				obj.set("completedAt", r.today())
				return nil
			}); err != nil {
				r.errf("%v", err)
				fmt.Fprintf(r.errOut(), "未归档：%s 未动。任务仍标进行中，归档会把它从 list 里藏起来。\n", RepoRelative(r.Root, taskDir))
				return 1
			}
			// 父任务归档时清除子女的 parent 指向（失败可回滚）。
			unlinked := map[string]string{}
			for _, childName := range children {
				childDir, err := FindTaskByName(childName, tasksDir)
				if err != nil || childDir == "" {
					continue
				}
				childJSON := filepath.Join(childDir, FileTaskJSON)
				if _, err := os.Stat(childJSON); err != nil {
					continue
				}
				childData, childReason, err := readRawObject(childJSON)
				if err != nil {
					problem, _ := describeReadFailure(childJSON, childReason)
					r.warnf("%s；子任务 %q 保留父引用。", problem, childName)
					continue
				}
				originalParent, _ := childData.getStringOrNull("parent")
				if _, seen := unlinked[childJSON]; !seen {
					unlinked[childJSON] = originalParent
				}
				childData.setNull("parent")
				if err := writeRawObject(childJSON, childData); err != nil {
					// 回滚已解除的链接。
					for jsonPath, orig := range unlinked {
						if d, _, err := readRawObject(jsonPath); err == nil {
							if orig == "" {
								d.setNull("parent")
							} else {
								d.set("parent", orig)
							}
							_ = writeRawObject(jsonPath, d)
						}
					}
					r.errf("写入子任务 %s 的 task.json 失败", childName)
					fmt.Fprintf(r.errOut(), "未归档：%s 已标 completed 但留在原处（子任务指向已归档父任务会成悬空引用）。请修复子任务后重新归档。\n", RepoRelative(r.Root, taskDir))
					return 1
				}
				modifiedChildren = append(modifiedChildren, filepath.Base(childDir))
			}
		}
	}
	// 先清掉指向该任务的会话指针，再移动目录。
	ClearTaskFromSessions(r.Root, RepoRelative(r.Root, taskDir))
	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		r.errf("创建归档目录失败：%v", err)
		return 1
	}
	if _, err := os.Stat(dest); err == nil {
		r.errf("拒绝归档 %q：归档目标已存在：%s", name, RepoRelative(r.Root, dest))
		return 1
	}
	if err := os.Rename(taskDir, dest); err != nil {
		r.errf("移动任务到归档失败：%v", err)
		return 1
	}
	yearMonth := filepath.Base(filepath.Dir(dest))
	fmt.Fprintf(r.errOut(), "已归档：%s -> archive/%s/\n", dirName, yearMonth)
	if !noCommit {
		if !autoCommitArchive(r.Root, dirName, modifiedChildren, fileWriter(r.errOut())) {
			fmt.Fprintln(r.errOut(), "归档已在磁盘上移动，但 git 自动提交未完成。请先处理 git status。")
			return 1
		}
	}
	fmt.Fprintln(r.out(), DirSunshine+"/"+DirTasks+"/"+DirArchive+"/"+yearMonth+"/"+dirName)
	RunTaskHooks("after_archive", filepath.Join(dest, FileTaskJSON), r.Root, fileWriter(r.errOut()))
	return 0
}
