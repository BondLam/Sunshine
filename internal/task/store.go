package task

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/BondLam/Sunshine/internal/hook"
)

const (
	// DirSunshine 是项目内 Sunshine 目录名。
	DirSunshine = ".sunshine"
	// DirTasks 是任务目录名。
	DirTasks = "tasks"
	// DirArchive 是归档目录名。
	DirArchive = "archive"
	// FileTaskJSON 是任务记录文件名。
	FileTaskJSON = "task.json"
	// FileCurrentTask 是当前任务指针文件名（内容为任务路径）。
	FileCurrentTask = ".current-task"
	// DeveloperHint 是“未设置开发者”报错后附的提示。
	DeveloperHint = "  也可以设置 SUNSHINE_DEVELOPER=<名字> 环境变量。\n  链接的 git worktree 会继承主 checkout 的 .sunshine/.developer——请在主 checkout 运行 sunshine init 设置开发者身份。"
)

// FindRoot 从 start 出发向上找含 .sunshine/ 的目录，找不到返回 ""。
func FindRoot(start string) string { return hook.FindSunshineRoot(start) }

// TasksDir 返回任务目录绝对路径（读 hook.LoadConfig 的 tasks_dir）。
func TasksDir(root string) string {
	return hook.LoadConfig(root).TasksDirAbs(root)
}

// Developer 返回当前开发者名（环境变量 → .sunshine/.developer）。
func Developer(root string) string { return hook.GetDeveloper(root) }

// LoadConfig 读 .sunshine/config.yaml（hook 的迷你解析器）。
func LoadConfig(root string) hook.Config { return hook.LoadConfig(root) }

// writeFileAtomic 原子写文件：同目录临时文件 + 重命名。
func writeFileAtomic(path string, data []byte, perm os.FileMode) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, ".sunshine-tmp-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Chmod(tmpName, perm); err != nil {
		return err
	}
	return os.Rename(tmpName, path)
}

// ---------------------------------------------------------------------------
// task.json 读写
// ---------------------------------------------------------------------------

// LoadRecord 读并严格校验某任务目录的 task.json。
func LoadRecord(taskDir string) (*TaskRecord, error) {
	data, err := os.ReadFile(filepath.Join(taskDir, FileTaskJSON))
	if err != nil {
		return nil, fmt.Errorf("读取 %s 失败：%w", filepath.Join(taskDir, FileTaskJSON), err)
	}
	rec, err := ParseRecord(data)
	if err != nil {
		return nil, err
	}
	return rec, nil
}

// WriteRecord 写 task.json：canonical 字段在前（FieldOrder），磁盘已有
// 的未知字段按原顺序合并保留；磁盘文件损坏（非对象）时拒绝覆盖。
func WriteRecord(taskDir string, rec *TaskRecord) error {
	if _, err := ParseRecord(mustMarshal(rec)); err != nil {
		return err
	}
	path := filepath.Join(taskDir, FileTaskJSON)
	if data, err := os.ReadFile(path); err == nil {
		existing, err := ParseRecord(data)
		if err != nil {
			// 磁盘文件不可解析：拒绝覆盖，避免丢可恢复的本地数据。
			// 但若只是未知字段校验问题，ParseRecord 会报错；这里按
			// records.ts 的语义：非对象/损坏都拒绝。
			return fmt.Errorf("拒绝覆盖损坏的 %s：%v", path, err)
		}
		merged := &TaskRecord{}
		*merged = *rec
		merged.Unknown = existing.Unknown
		rec = merged
	} else if !os.IsNotExist(err) {
		return err
	}
	out, err := MarshalRecord(rec)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(taskDir, 0o755); err != nil {
		return err
	}
	return writeFileAtomic(path, out, 0o644)
}

func mustMarshal(rec *TaskRecord) []byte {
	out, err := MarshalRecord(rec)
	if err != nil {
		return []byte("{}")
	}
	return out
}

// rawObject 是 task.json 的原始有序对象，供“只改一个字段”的命令用：
// Python 的 set-*/rename/archive 等命令都是读 dict、改一键、原样写回，
// 不按 24 字段 schema 重校验，因此这里也不做严格校验，只保序、
// 保未知字段。
type rawObject struct {
	keys []string
	vals map[string]json.RawMessage
}

func readRawObject(path string) (*rawObject, string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, "missing", err
		}
		return nil, "unreadable", err
	}
	if len(bytes.TrimSpace(data)) == 0 {
		return nil, "empty", fmt.Errorf("文件为空")
	}
	var probe any
	dec := json.NewDecoder(bytes.NewReader(data))
	if err := dec.Decode(&probe); err != nil {
		return nil, "invalid", err
	}
	obj, ok := probe.(map[string]any)
	if !ok {
		return nil, "not-object", fmt.Errorf("顶层不是 JSON 对象")
	}
	if len(obj) == 0 {
		return nil, "empty", fmt.Errorf("空 JSON 对象")
	}
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, "invalid", err
	}
	return &rawObject{keys: orderedTopLevelKeys(data), vals: raw}, "", nil
}

// describeReadFailure 返回读失败的原因说明与修复提示（对应 io.py）。
func describeReadFailure(path, reason string) (string, string) {
	switch reason {
	case "missing":
		return path + "：文件不存在", "请传入已存在的任务目录，或先创建任务。"
	case "unreadable":
		return path + "：无法读取（权限或 I/O 错误）", "请检查文件与目录权限后重试。"
	case "invalid":
		return path + "：不是合法 JSON", "请修复 JSON 语法后重试。"
	case "not-object":
		return path + "：顶层不是 JSON 对象", "请把文件恢复为 JSON 对象（{ ... }）。"
	case "empty":
		return path + "：空 JSON 对象", "请恢复任务字段（或重新创建任务）。"
	default:
		return path + "：无法加载", "请检查文件后重试。"
	}
}

func (o *rawObject) getString(field string) (string, bool) {
	raw, ok := o.vals[field]
	if !ok {
		return "", false
	}
	var s string
	if err := json.Unmarshal(raw, &s); err != nil {
		return "", false
	}
	return s, true
}

func (o *rawObject) getStringOrNull(field string) (string, bool) {
	raw, ok := o.vals[field]
	if !ok {
		return "", false
	}
	if string(bytes.TrimSpace(raw)) == "null" {
		return "", true
	}
	var s string
	if err := json.Unmarshal(raw, &s); err != nil {
		return "", false
	}
	return s, true
}

func (o *rawObject) getStringList(field string) []string {
	raw, ok := o.vals[field]
	if !ok {
		return nil
	}
	var arr []string
	if err := json.Unmarshal(raw, &arr); err != nil {
		return nil
	}
	return arr
}

func (o *rawObject) set(field string, value any) {
	encoded, err := encodeJSONValue(value)
	if err != nil {
		return
	}
	if _, ok := o.vals[field]; !ok {
		o.keys = append(o.keys, field)
	}
	o.vals[field] = encoded
}

func (o *rawObject) setNull(field string) {
	if _, ok := o.vals[field]; !ok {
		o.keys = append(o.keys, field)
	}
	o.vals[field] = json.RawMessage("null")
}

func (o *rawObject) setStringList(field string, values []string) {
	if values == nil {
		values = []string{}
	}
	o.set(field, values)
}

// ensureStringList 保证 children/subtasks 字段是字符串数组（不是就丢弃
// 重建为空数组），对应 task_store._ensure_children_list。
func (o *rawObject) ensureStringList(field string) []string {
	raw, ok := o.vals[field]
	if !ok {
		o.setStringList(field, []string{})
		return []string{}
	}
	var arr []string
	if err := json.Unmarshal(raw, &arr); err != nil || arr == nil {
		o.setStringList(field, []string{})
		return []string{}
	}
	return arr
}

func (o *rawObject) marshal() ([]byte, error) {
	var buf bytes.Buffer
	buf.WriteString("{\n")
	for i, key := range o.keys {
		kb, _ := encodeJSONValue(key)
		buf.WriteString("  ")
		buf.Write(kb)
		buf.WriteString(": ")
		buf.Write(compactJSON(o.vals[key]))
		if i < len(o.keys)-1 {
			buf.WriteString(",")
		}
		buf.WriteString("\n")
	}
	buf.WriteString("}\n")
	return buf.Bytes(), nil
}

func writeRawObject(path string, obj *rawObject) error {
	data, err := obj.marshal()
	if err != nil {
		return err
	}
	return writeFileAtomic(path, data, 0o644)
}

// updateTaskJSON 读任务的 task.json 原始对象、改动、原子写回。
// 文件不存在或损坏时返回错误（调用方按 Python 的 _report_read_failure
// 打印原因）。
func updateTaskJSON(taskDir string, mutate func(obj *rawObject) error) error {
	path := filepath.Join(taskDir, FileTaskJSON)
	obj, reason, err := readRawObject(path)
	if err != nil {
		problem, hint := describeReadFailure(path, reason)
		return fmt.Errorf("%s\n%s", problem, hint)
	}
	if err := mutate(obj); err != nil {
		return err
	}
	if err := writeRawObject(path, obj); err != nil {
		return fmt.Errorf("写入 %s 失败（原文件未动）：%w", path, err)
	}
	return nil
}

// ---------------------------------------------------------------------------
// 任务目录解析与遍历
// ---------------------------------------------------------------------------

// TaskInfo 是加载后的任务摘要（对应 common/tasks.TaskInfo）。
type TaskInfo struct {
	DirName  string
	Dir      string
	Title    string
	Name     string
	Status   string
	Assignee string
	Priority string
	Children []string
	Parent   string
	Package  string
	Raw      *rawObject
}

// loadTaskInfo 读一个任务目录；无 task.json 不是任务（返回 nil）；
// task.json 损坏则返回错误（调用方在 stderr 告警后跳过）。
func loadTaskInfo(dir string) (*TaskInfo, error) {
	path := filepath.Join(dir, FileTaskJSON)
	obj, reason, err := readRawObject(path)
	if err != nil {
		if reason == "missing" {
			return nil, nil
		}
		problem, hint := describeReadFailure(path, reason)
		return nil, fmt.Errorf("[警告] 跳过任务 %q：%s\n       %s", filepath.Base(dir), problem, hint)
	}
	title, _ := obj.getString("title")
	if title == "" {
		title, _ = obj.getString("name")
	}
	if title == "" {
		title = "unknown"
	}
	status, _ := obj.getString("status")
	if status == "" {
		status = "unknown"
	}
	assignee, _ := obj.getString("assignee")
	priority, _ := obj.getString("priority")
	if priority == "" {
		priority = "P2"
	}
	parent, _ := obj.getStringOrNull("parent")
	pkg, _ := obj.getStringOrNull("package")
	name, _ := obj.getString("name")
	if name == "" {
		name, _ = obj.getString("id")
	}
	if name == "" {
		name = filepath.Base(dir)
	}
	return &TaskInfo{
		DirName:  filepath.Base(dir),
		Dir:      dir,
		Title:    title,
		Name:     name,
		Status:   status,
		Assignee: assignee,
		Priority: priority,
		Children: obj.getStringList("children"),
		Parent:   parent,
		Package:  pkg,
		Raw:      obj,
	}, nil
}

// IterActiveTasks 列出任务目录下全部有效任务（按目录名排序，跳过
// archive；损坏的 task.json 在 stderr 告警后跳过）。
func IterActiveTasks(tasksDir string, errOut io.Writer) []*TaskInfo {
	entries, err := os.ReadDir(tasksDir)
	if err != nil {
		return nil
	}
	var out []*TaskInfo
	for _, e := range entries {
		if !e.IsDir() || e.Name() == DirArchive {
			continue
		}
		info, err := loadTaskInfo(filepath.Join(tasksDir, e.Name()))
		if err != nil {
			if errOut != nil {
				fmt.Fprintln(errOut, err.Error())
			}
			continue
		}
		if info != nil {
			out = append(out, info)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].DirName < out[j].DirName })
	return out
}

// AllStatuses 返回 {目录名: 状态}。
func AllStatuses(tasks []*TaskInfo) map[string]string {
	m := map[string]string{}
	for _, t := range tasks {
		m[t.DirName] = t.Status
	}
	return m
}

// ChildrenProgress 返回 " [2/3 已完成]" 形式的子女进度；已归档（不在
// 活动集里）的子女计为已完成。
func ChildrenProgress(children []string, allStatuses map[string]string) string {
	if len(children) == 0 {
		return ""
	}
	done := 0
	for _, c := range children {
		if st, ok := allStatuses[c]; !ok || st == "completed" || st == "done" {
			done++
		}
	}
	return fmt.Sprintf(" [%d/%d 已完成]", done, len(children))
}

// FindTaskByName 按名找任务：精确匹配优先，其次 "-<名>" 后缀唯一匹配；
// 后缀多解时报错（对应 task_utils.find_task_by_name）。
func FindTaskByName(name, tasksDir string) (string, error) {
	if name == "" || strings.ContainsAny(name, "/\\") || name == "." || name == ".." {
		return "", fmt.Errorf("无效的任务名：%s", name)
	}
	entries, err := os.ReadDir(tasksDir)
	if err != nil {
		return "", nil
	}
	exact := filepath.Join(tasksDir, name)
	if st, err := os.Stat(exact); err == nil && st.IsDir() {
		return exact, nil
	}
	var matches []string
	for _, e := range entries {
		if e.IsDir() && strings.HasSuffix(e.Name(), "-"+name) {
			matches = append(matches, e.Name())
		}
	}
	sort.Strings(matches)
	if len(matches) == 1 {
		return filepath.Join(tasksDir, matches[0]), nil
	}
	if len(matches) > 0 {
		var lines []string
		for _, m := range matches {
			lines = append(lines, "  - "+m)
		}
		return "", fmt.Errorf("任务名 %q 有多解：\n%s\n请传入完整任务目录名。", name, strings.Join(lines, "\n"))
	}
	return "", nil
}

// IsWithinTasksDir 判断某任务目录是否直接位于 tasks/ 下（归档任务不在内）。
func IsWithinTasksDir(taskDir, tasksDir string) bool {
	resolved, err := filepath.EvalSymlinks(taskDir)
	if err != nil {
		resolved = filepath.Clean(taskDir)
	}
	tasksResolved, err := filepath.EvalSymlinks(tasksDir)
	if err != nil {
		tasksResolved = filepath.Clean(tasksDir)
	}
	if filepath.Dir(resolved) != tasksResolved {
		return false
	}
	return filepath.Base(resolved) != DirArchive
}

// ResolveTaskDir 解析任务目录参数（绝对路径 / 相对路径 / 任务名），
// 收容在 tasks/ 之内；归档任务也算（对应 task_utils.resolve_task_dir）。
func ResolveTaskDir(target, root string) (string, error) {
	if target == "" {
		return "", fmt.Errorf("需要任务目录参数")
	}
	tasksDir := TasksDir(root)
	normalized := strings.ReplaceAll(target, "\\", "/")
	for strings.HasPrefix(normalized, "./") {
		normalized = normalized[2:]
	}
	var candidate string
	switch {
	case filepath.IsAbs(target):
		candidate = target
	case strings.Contains(normalized, "/") || strings.HasPrefix(normalized, DirSunshine):
		candidate = filepath.Join(root, filepath.FromSlash(normalized))
	default:
		found, err := FindTaskByName(target, tasksDir)
		if err != nil {
			return "", err
		}
		if found == "" {
			return "", fmt.Errorf("无法在 %s 下解析任务 %q", tasksDir, target)
		}
		candidate = found
	}
	resolved, err := filepath.EvalSymlinks(candidate)
	if err != nil {
		resolved = filepath.Clean(candidate)
	}
	tasksLexical := tasksDir
	tasksResolved, err := filepath.EvalSymlinks(tasksLexical)
	if err != nil {
		tasksResolved = filepath.Clean(tasksLexical)
	}
	if resolved == tasksResolved {
		return "", fmt.Errorf("拒绝使用 %q：%s 是任务目录本身，不是任务", target, tasksResolved)
	}
	rel, err := filepath.Rel(tasksResolved, resolved)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("拒绝使用 %q：%s 在 %s 之外", target, resolved, tasksResolved)
	}
	// 经由符号链接的 .sunshine 也拼回仓库内写法（#567）。
	return filepath.Join(tasksLexical, filepath.FromSlash(rel)), nil
}

// ArchiveDestinationFor 返回任务归档目标：tasks/archive/YYYY-MM/<名>。
func ArchiveDestinationFor(taskDir string) string {
	ym := time.Now().Format("2006-01")
	return filepath.Join(filepath.Dir(taskDir), DirArchive, ym, filepath.Base(taskDir))
}

// EnsureTasksDir 确保 tasks/ 与 tasks/archive/ 存在。
func EnsureTasksDir(root string) string {
	tasksDir := TasksDir(root)
	_ = os.MkdirAll(filepath.Join(tasksDir, DirArchive), 0o755)
	return tasksDir
}

// FindArchivedTaskByDirName 在 archive/ 各月目录下找同名已归档任务。
func FindArchivedTaskByDirName(tasksDir, dirName string) string {
	archiveDir := filepath.Join(tasksDir, DirArchive)
	months, err := os.ReadDir(archiveDir)
	if err != nil {
		return ""
	}
	for _, m := range months {
		if !m.IsDir() {
			continue
		}
		candidate := filepath.Join(archiveDir, m.Name(), dirName)
		if st, err := os.Stat(candidate); err == nil && st.IsDir() {
			return candidate
		}
	}
	return ""
}

// RepoRelative 返回相对项目根的斜杠路径，超出时返回原路径。
func RepoRelative(root, path string) string { return hook.RepoRelative(root, path) }

// Today 返回当天日期（YYYY-MM-DD，本地时间）。
func Today() string { return time.Now().Format("2006-01-02") }

// DatePrefix 返回当天 MM-DD 前缀。
func DatePrefix() string { return time.Now().Format("01-02") }

// ---------------------------------------------------------------------------
// git
// ---------------------------------------------------------------------------

// runGit 在 root 下运行 git（-c i18n.logOutputEncoding=UTF-8），返回
// （退出码，stdout，stderr）。超时默认 15 秒。
func runGit(root string, timeout time.Duration, args ...string) (int, string, string) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	full := append([]string{"-c", "i18n.logOutputEncoding=UTF-8"}, args...)
	cmd := exec.CommandContext(ctx, "git", full...)
	cmd.Dir = root
	var outBuf, errBuf bytes.Buffer
	cmd.Stdout = &outBuf
	cmd.Stderr = &errBuf
	err := cmd.Run()
	code := 0
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			code = exitErr.ExitCode()
		} else {
			return 1, "", err.Error()
		}
	}
	return code, outBuf.String(), errBuf.String()
}

// runGitRetryIndexLock 运行 git，遇 .git/index.lock 占用时按 0.5s、1s
// 退避重试（对应 git.run_git_retry_index_lock）。
func runGitRetryIndexLock(root string, args ...string) (int, string, string) {
	code, out, errStr := runGit(root, 15*time.Second, args...)
	for attempt, backoff := range []time.Duration{500 * time.Millisecond, time.Second} {
		if code == 0 || !strings.Contains(strings.ToLower(errStr), "index.lock") {
			break
		}
		_ = attempt
		time.Sleep(backoff)
		code, out, errStr = runGit(root, 15*time.Second, args...)
	}
	return code, out, errStr
}

// CurrentBranch 返回当前检出分支，detached / 非 git 仓库时返回 ""。
func CurrentBranch(root string) string {
	code, out, _ := runGit(root, 15*time.Second, "branch", "--show-current")
	if code != 0 {
		return ""
	}
	return strings.TrimSpace(out)
}

// DefaultBranchLocal 本地解析默认分支：origin/HEAD 符号引用 →
// main/master；不做 git remote show 的联网探测（add_session 的
// _default_branch_local 同款本地策略；task.py create 的
// resolve_default_branch 会联网，Go 版为避免命令行挂起不采用）。
func DefaultBranchLocal(root string) string {
	code, out, _ := runGit(root, 15*time.Second, "symbolic-ref", "--quiet", "refs/remotes/origin/HEAD")
	if code == 0 && strings.TrimSpace(out) != "" {
		parts := strings.Split(strings.TrimSpace(out), "/")
		return parts[len(parts)-1]
	}
	for _, cand := range []string{"main", "master"} {
		if BranchExistsLocal(root, cand) {
			return cand
		}
	}
	return ""
}

// HasRemote 判断仓库是否配了 remote。
func HasRemote(root string) bool {
	code, out, _ := runGit(root, 15*time.Second, "remote")
	return code == 0 && strings.TrimSpace(out) != ""
}

// BranchExistsLocal 判断本地分支引用是否存在。
func BranchExistsLocal(root, branch string) bool {
	if branch == "" {
		return false
	}
	code, _, _ := runGit(root, 15*time.Second, "rev-parse", "--verify", "--quiet", "refs/heads/"+branch)
	return code == 0
}

// BranchRefExists 判断分支在本地或 origin 远端引用是否存在。
func BranchRefExists(root, branch string) bool {
	for _, ref := range []string{"refs/heads/" + branch, "refs/remotes/origin/" + branch} {
		code, _, _ := runGit(root, 15*time.Second, "show-ref", "--verify", "--quiet", ref)
		if code == 0 {
			return true
		}
	}
	return false
}

// IsGitWorktree 判断 root 是否在 git 工作区内。
func IsGitWorktree(root string) bool {
	code, out, _ := runGit(root, 15*time.Second, "rev-parse", "--is-inside-work-tree")
	return code == 0 && strings.TrimSpace(strings.ToLower(out)) == "true"
}

// IsLinkedWorktree 判断是否为链接 worktree（git-dir 与 git-common-dir 不同）。
func IsLinkedWorktree(root string) bool {
	_, gitDir, _ := runGit(root, 15*time.Second, "rev-parse", "--git-dir")
	_, gitCommon, _ := runGit(root, 15*time.Second, "rev-parse", "--git-common-dir")
	if strings.TrimSpace(gitDir) == "" || strings.TrimSpace(gitCommon) == "" {
		return false
	}
	resolve := func(p string) string {
		if !filepath.IsAbs(p) {
			p = filepath.Join(root, p)
		}
		if abs, err := filepath.EvalSymlinks(p); err == nil {
			return abs
		}
		return filepath.Clean(p)
	}
	return resolve(strings.TrimSpace(gitDir)) != resolve(strings.TrimSpace(gitCommon))
}

// ResolveCommitSubject 用本地对象库把 OID 解析为提交主题；
// 失败返回 ""（对应 add_session.resolve_commit_subject，用
// <oid>^{commit} 剥离，防止 hex 形 ref 名解析到 tree/tag）。
func ResolveCommitSubject(root, oid string) string {
	code, out, _ := runGit(root, 15*time.Second, "show", "-s", "--format=%s", oid+"^{commit}", "--")
	if code != 0 {
		return ""
	}
	for _, line := range strings.Split(out, "\n") {
		if s := strings.TrimSpace(line); s != "" {
			if len(s) > 500 {
				return s[:500]
			}
			return s
		}
	}
	return ""
}

// ContentAtHead 返回某路径在 HEAD 提交时的内容，不存在返回 ""。
func ContentAtHead(root, relPath string) (string, bool) {
	code, out, _ := runGit(root, 15*time.Second, "show", "HEAD:./"+relPath)
	if code != 0 {
		return "", false
	}
	return out, true
}

// ---------------------------------------------------------------------------
// 子代理平台探测与默认 PRD
// ---------------------------------------------------------------------------

// subagentConfigDirs 是消费 implement.jsonl / check.jsonl 的平台配置目录
// （与 task_store._SUBAGENT_CONFIG_DIRS 同步；Codex 单独按 dispatch_mode 判断）。
var subagentConfigDirs = []string{
	".claude", ".cursor", ".kiro", ".gemini", ".opencode", ".qoder",
	".codebuddy", ".factory", ".github/copilot", ".pi", ".trae", ".omp",
	".zcode", ".grok", ".kimi-code",
}

// HasSubagentPlatform 判断任一子代理平台是否已配置。
func HasSubagentPlatform(root string) bool {
	for _, d := range subagentConfigDirs {
		if st, err := os.Stat(filepath.Join(root, d)); err == nil && st.IsDir() {
			return true
		}
	}
	if st, err := os.Stat(filepath.Join(root, ".codex")); err == nil && st.IsDir() {
		return LoadConfig(root).ResolveCodexDispatchMode() == "auto"
	}
	return false
}

// DefaultPRDContent 返回新建任务的默认 prd.md 骨架（中文）。
func DefaultPRDContent(title, description string) string {
	goal := strings.TrimSpace(description)
	if goal == "" {
		goal = "待定。"
	}
	heading := strings.TrimSpace(title)
	if heading == "" {
		heading = "未命名任务"
	}
	return "# " + heading + "\n\n## 目标\n\n" + goal + "\n\n## 需求\n\n- 待定\n\n## 验收标准\n\n- [ ] 待定\n\n## 说明\n\n- prd.md 只写需求、约束与验收标准。\n- 轻量任务可以只有 PRD。\n- 复杂任务在 `sunshine task start` 前补充 design.md（技术设计）与 implement.md（执行计划）。\n"
}

// codeFileExtensions 是 validate 里视作“代码文件”的后缀。
var codeFileExtensions = map[string]bool{
	".ts": true, ".tsx": true, ".js": true, ".jsx": true, ".mjs": true,
	".cjs": true, ".py": true, ".go": true, ".rs": true, ".java": true,
	".rb": true, ".c": true, ".cc": true, ".cpp": true, ".h": true,
}

func isCodeFileExt(path string) bool {
	return codeFileExtensions[strings.ToLower(filepath.Ext(path))]
}

// isExemptFromCodeFileWarning 判断 jsonl 条目是否免于代码文件警告：
// .sunshine/spec/、docs/、docs-site/ 或任务自身目录下的文件免检。
func isExemptFromCodeFileWarning(filePath, taskRel string) bool {
	posix := strings.ReplaceAll(filePath, "\\", "/")
	posix = strings.TrimPrefix(posix, "/")
	for _, prefix := range []string{DirSunshine + "/spec/", "docs/", "docs-site/"} {
		if strings.HasPrefix(posix, prefix) {
			return true
		}
	}
	if taskRel != "" && (posix == taskRel || strings.HasPrefix(posix, taskRel+"/")) {
		return true
	}
	return false
}

// CuratedEntryCount 数 jsonl 里的已整理条目（带 file/path 的 JSON 对象行）。
// 文件不存在返回 -1（对应 Python 的 None：无文件表示不设防，不参与门禁）。
func CuratedEntryCount(jsonlPath string) int {
	data, err := os.ReadFile(jsonlPath)
	if err != nil {
		return -1
	}
	count := 0
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		var row map[string]any
		if err := json.Unmarshal([]byte(line), &row); err != nil {
			continue
		}
		if row["file"] != nil || row["path"] != nil {
			if s, ok := row["file"].(string); ok && s != "" {
				count++
			} else if s, ok := row["path"].(string); ok && s != "" {
				count++
			}
		}
	}
	return count
}

// jsonlEntry 是 implement/check.jsonl 里的一行。
type jsonlEntry struct {
	File   string `json:"file"`
	Type   string `json:"type,omitempty"`
	Reason string `json:"reason,omitempty"`
	// Lines 是可选的行范围（"120-180"，1-based 闭区间）；缺省整文件。
	// 注入时只切该段（见 internal/hook/subagent.go 的 materializeFile）。
	Lines string `json:"lines,omitempty"`
}

// parseLineRange 解析 lines 字段（"120-180"，1-based 闭区间）。
// 空串表示不限；格式非法或 start<1 或 start>end 返回 ok=false。
// 与 internal/hook 的同名函数同形（两处各守一端：注入时降级、校验时报错）。
func parseLineRange(s string) (start, end int, ok bool) {
	if s == "" {
		return 0, 0, false
	}
	parts := strings.Split(s, "-")
	if len(parts) != 2 {
		return 0, 0, false
	}
	start, err1 := strconv.Atoi(strings.TrimSpace(parts[0]))
	end, err2 := strconv.Atoi(strings.TrimSpace(parts[1]))
	if err1 != nil || err2 != nil || start < 1 || start > end {
		return 0, 0, false
	}
	return start, end, true
}

// countFileLines 数文件的总行数（与注入切片同一口径：末尾空行不计）。
func countFileLines(data []byte) int {
	lines := strings.Split(string(data), "\n")
	if n := len(lines); n > 0 && lines[n-1] == "" {
		lines = lines[:n-1]
	}
	return len(lines)
}

var jsonlNameRe = regexp.MustCompile(`^[A-Za-z0-9._-]+$`)

func validJSONLName(name string) bool {
	return jsonlNameRe.MatchString(name) && name != "." && name != ".."
}
