package task

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/linrong1982/Sunshine/internal/hook"
)

// Context 实现 sunshine task context，对应 get_context.py（common/
// git_context.main）：--mode default/record/packages/phase，文本与
// JSON 两种输出。
func (r *Runner) Context(mode, step, platform string, asJSON bool) int {
	if !r.requireRoot() {
		return 1
	}
	switch mode {
	case "default", "":
		if asJSON {
			printJSON(r.out(), r.contextJSON())
			return 0
		}
		fmt.Fprint(r.out(), r.contextText())
		return 0
	case "record":
		if asJSON {
			printJSON(r.out(), r.contextRecordJSON())
			return 0
		}
		fmt.Fprint(r.out(), r.contextRecordText())
		return 0
	case "packages":
		if asJSON {
			printJSON(r.out(), r.contextPackagesJSON())
			return 0
		}
		fmt.Fprint(r.out(), r.contextPackagesText())
		return 0
	case "phase":
		content := ""
		if step != "" {
			content = getWorkflowStep(r.Root, step)
			if strings.TrimSpace(content) == "" {
				fmt.Fprintf(r.errOut(), "步骤不存在：%s\n", step)
				return 2
			}
		} else {
			content = getPhaseIndex(r.Root)
			if strings.TrimSpace(content) == "" {
				fmt.Fprintln(r.errOut(), "workflow.md 里找不到 Phase Index")
				return 2
			}
		}
		if platform != "" {
			content = filterPlatform(content, platform)
		}
		fmt.Fprint(r.out(), content)
		return 0
	default:
		r.errf("未知模式：%s（可用：default、record、packages、phase）", mode)
		return 1
	}
}

// ---------------------------------------------------------------------------
// git 信息
// ---------------------------------------------------------------------------

type gitInfo struct {
	IsRepo             bool          `json:"isRepo"`
	Branch             string        `json:"branch"`
	IsClean            bool          `json:"isClean"`
	UncommittedChanges int           `json:"uncommittedChanges"`
	RecentCommits      []commitEntry `json:"recentCommits"`
	StatusShort        []string      `json:"-"`
}

type commitEntry struct {
	Hash    string `json:"hash"`
	Message string `json:"message"`
}

func collectRootGitInfo(root string) gitInfo {
	if !IsGitWorktree(root) {
		return gitInfo{IsRepo: false, RecentCommits: []commitEntry{}}
	}
	_, branchOut, _ := runGit(root, 15*time.Second, "branch", "--show-current")
	branch := strings.TrimSpace(branchOut)
	if branch == "" {
		branch = "unknown"
	}
	_, statusOut, _ := runGit(root, 15*time.Second, "status", "--porcelain")
	statusLines := []string{}
	for _, line := range strings.Split(statusOut, "\n") {
		if strings.TrimSpace(line) != "" {
			statusLines = append(statusLines, line)
		}
	}
	_, shortOut, _ := runGit(root, 15*time.Second, "status", "--short")
	var short []string
	for _, line := range strings.Split(shortOut, "\n") {
		if line != "" {
			short = append(short, line)
		}
	}
	_, logOut, _ := runGit(root, 15*time.Second, "log", "--oneline", "-5")
	return gitInfo{
		IsRepo: true, Branch: branch,
		IsClean:            len(statusLines) == 0,
		UncommittedChanges: len(statusLines),
		RecentCommits:      parseRecentCommits(logOut),
		StatusShort:        short,
	}
}

func parseRecentCommits(logOutput string) []commitEntry {
	var out []commitEntry
	for _, line := range strings.Split(logOutput, "\n") {
		if strings.TrimSpace(line) == "" {
			continue
		}
		parts := strings.SplitN(line, " ", 2)
		if len(parts) == 2 {
			out = append(out, commitEntry{Hash: parts[0], Message: parts[1]})
		} else {
			out = append(out, commitEntry{Hash: parts[0]})
		}
	}
	if out == nil {
		out = []commitEntry{}
	}
	return out
}

func (r *Runner) activeJournalInfo() (rel string, lines int) {
	dev := Developer(r.Root)
	if dev == "" {
		return "", 0
	}
	journal := hookActiveJournalFile(r.Root)
	if journal == "" {
		return "", 0
	}
	return DirSunshine + "/workspace/" + dev + "/" + filepath.Base(journal), countLines(journal)
}

func countLines(path string) int {
	data, err := os.ReadFile(path)
	if err != nil {
		return 0
	}
	if len(data) == 0 {
		return 0
	}
	n := 0
	for _, b := range data {
		if b == '\n' {
			n++
		}
	}
	if data[len(data)-1] != '\n' {
		n++
	}
	return n
}

// hookActiveJournalFile 复用 hook 的 ActiveJournalFile 语义（本包不依赖
// hook 的未导出函数，这里按同一规则实现：取编号最大的 journal-N.md）。
func hookActiveJournalFile(root string) string {
	return hook.ActiveJournalFile(root)
}

// ---------------------------------------------------------------------------
// default 模式
// ---------------------------------------------------------------------------

func (r *Runner) contextJSON() map[string]any {
	dev := Developer(r.Root)
	tasksDir := TasksDir(r.Root)
	journalRel, journalLines := r.activeJournalInfo()
	git := collectRootGitInfo(r.Root)
	tasks := IterActiveTasks(tasksDir, nil)
	var active []map[string]any
	for _, t := range tasks {
		children := t.Children
		if children == nil {
			children = []string{}
		}
		active = append(active, map[string]any{
			"dir": t.DirName, "name": t.Name, "status": t.Status,
			"children": children, "parent": t.Parent,
		})
	}
	if active == nil {
		active = []map[string]any{}
	}
	result := map[string]any{
		"developer": dev,
		"git": map[string]any{
			"isRepo": git.IsRepo, "branch": git.Branch, "isClean": git.IsClean,
			"uncommittedChanges": git.UncommittedChanges, "recentCommits": git.RecentCommits,
		},
		"tasks": map[string]any{
			"active": active, "directory": DirSunshine + "/" + DirTasks,
		},
		"journal": map[string]any{
			"file": journalRel, "lines": journalLines,
			"nearLimit": journalLines > 1800,
		},
	}
	return result
}

func (r *Runner) contextText() string {
	var lines []string
	lines = append(lines, "========================================")
	lines = append(lines, "会话上下文")
	lines = append(lines, "========================================")
	lines = append(lines, "")
	dev := Developer(r.Root)
	lines = append(lines, "## 开发者")
	if dev == "" {
		lines = append(lines, "错误：未初始化。请运行：sunshine init <名字>")
		return strings.Join(lines, "\n")
	}
	lines = append(lines, "名字："+dev)
	lines = append(lines, "")
	git := collectRootGitInfo(r.Root)
	lines = appendRootGitContext(lines, git)
	// 当前任务。
	lines = append(lines, "## 当前任务")
	active := ResolveActiveTask(r.Root, "current", "")
	if active.TaskPath != "" {
		lines = append(lines, "路径："+active.TaskPath)
		lines = append(lines, "来源："+active.Source())
		if info, err := loadTaskInfo(filepath.Join(r.Root, active.TaskPath)); err == nil && info != nil {
			lines = append(lines, "名字："+info.Name)
			lines = append(lines, "状态："+info.Status)
			if created, _ := info.Raw.getString("createdAt"); created != "" {
				lines = append(lines, "创建："+created)
			}
			if desc, _ := info.Raw.getString("description"); desc != "" {
				lines = append(lines, "描述："+desc)
			}
		}
		if _, err := os.Stat(filepath.Join(r.Root, active.TaskPath, "prd.md")); err == nil {
			lines = append(lines, "")
			lines = append(lines, "[!] 该任务有 prd.md，详情请读它")
		}
	} else {
		lines = append(lines, "（无）")
	}
	lines = append(lines, "")
	// 活动任务（层级）。
	lines = append(lines, "## 活动任务")
	tasksDir := TasksDir(r.Root)
	all := IterActiveTasks(tasksDir, nil)
	byName := map[string]*TaskInfo{}
	for _, t := range all {
		byName[t.DirName] = t
	}
	allStatuses := AllStatuses(all)
	taskCount := 0
	var printTree func(name string, indent int)
	printTree = func(name string, indent int) {
		t := byName[name]
		progress := ChildrenProgress(t.Children, allStatuses)
		lines = append(lines, fmt.Sprintf("%s- %s/ (%s)%s @%s", strings.Repeat("  ", indent), name, t.Status, progress, orDash(t.Assignee)))
		taskCount++
		for _, child := range t.Children {
			if _, ok := byName[child]; ok {
				printTree(child, indent+1)
			}
		}
	}
	names := make([]string, 0, len(byName))
	for name := range byName {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		if byName[name].Parent == "" {
			printTree(name, 0)
		}
	}
	if taskCount == 0 {
		lines = append(lines, "（没有活动任务）")
	}
	lines = append(lines, fmt.Sprintf("总计：%d 个活动任务", taskCount))
	lines = append(lines, "")
	// 我的任务。
	lines = append(lines, "## 我的任务（分配给我）")
	myCount := 0
	for _, t := range all {
		if t.Assignee == dev && t.Status != "done" {
			progress := ChildrenProgress(t.Children, allStatuses)
			lines = append(lines, fmt.Sprintf("- [%s] %s (%s)%s", t.Priority, t.Title, t.Status, progress))
			myCount++
		}
	}
	if myCount == 0 {
		lines = append(lines, "（没有分配给你的任务）")
	}
	lines = append(lines, "")
	// Journal。
	lines = append(lines, "## 日志文件")
	journalRel, journalLines := r.activeJournalInfo()
	if journalRel != "" {
		lines = append(lines, "当前文件："+journalRel)
		lines = append(lines, fmt.Sprintf("行数：%d / 2000", journalLines))
		if journalLines > 1800 {
			lines = append(lines, "[!] 警告：接近 2000 行上限！")
		}
	} else {
		lines = append(lines, "未找到日志文件")
	}
	lines = append(lines, "")
	// 包。
	if section := r.packagesSection(); section != "" {
		lines = append(lines, section)
		lines = append(lines, "")
	}
	lines = append(lines, "## 路径")
	lines = append(lines, fmt.Sprintf("工作区：%s/workspace/%s/", DirSunshine, dev))
	lines = append(lines, fmt.Sprintf("任务：%s/%s/", DirSunshine, DirTasks))
	lines = append(lines, fmt.Sprintf("规范：%s/spec/", DirSunshine))
	lines = append(lines, "")
	lines = append(lines, "========================================")
	return strings.Join(lines, "\n")
}

func appendRootGitContext(lines []string, git gitInfo) []string {
	lines = append(lines, "## Git 状态")
	if !git.IsRepo {
		lines = append(lines, "根目录不是 Git 仓库。")
		lines = append(lines, "请到下方的包仓库路径下执行 Git 命令。")
	} else {
		lines = append(lines, "分支："+git.Branch)
		if git.IsClean {
			lines = append(lines, "工作区：干净")
		} else {
			lines = append(lines, fmt.Sprintf("工作区：%d 处未提交改动", git.UncommittedChanges))
			lines = append(lines, "")
			lines = append(lines, "改动：")
			lines = append(lines, git.StatusShort...)
		}
	}
	lines = append(lines, "")
	lines = append(lines, "## 最近提交")
	if !git.IsRepo {
		lines = append(lines, "根目录没有 Git 提交历史（不是 Git 仓库）。")
	} else if len(git.RecentCommits) > 0 {
		for _, c := range git.RecentCommits {
			lines = append(lines, c.Hash+" "+c.Message)
		}
	} else {
		lines = append(lines, "（无提交）")
	}
	lines = append(lines, "")
	return lines
}

// ---------------------------------------------------------------------------
// record 模式
// ---------------------------------------------------------------------------

func (r *Runner) contextRecordJSON() map[string]any {
	dev := Developer(r.Root)
	tasksDir := TasksDir(r.Root)
	git := collectRootGitInfo(r.Root)
	all := IterActiveTasks(tasksDir, nil)
	allStatuses := AllStatuses(all)
	var myTasks []map[string]any
	for _, t := range all {
		if t.Assignee != dev {
			continue
		}
		done := 0
		for _, c := range t.Children {
			if allStatuses[c] == "completed" || allStatuses[c] == "done" {
				done++
			}
		}
		children := t.Children
		if children == nil {
			children = []string{}
		}
		meta := map[string]any{}
		if raw, ok := t.Raw.vals["meta"]; ok {
			_ = json.Unmarshal(raw, &meta)
		}
		myTasks = append(myTasks, map[string]any{
			"dir": t.DirName, "title": t.Title, "status": t.Status,
			"priority": t.Priority, "children": children,
			"childrenDone": done, "parent": t.Parent, "meta": meta,
		})
	}
	if myTasks == nil {
		myTasks = []map[string]any{}
	}
	var currentInfo map[string]any
	active := ResolveActiveTask(r.Root, "current", "")
	if active.TaskPath != "" {
		if info, err := loadTaskInfo(filepath.Join(r.Root, active.TaskPath)); err == nil && info != nil {
			currentInfo = map[string]any{
				"path": active.TaskPath, "name": info.Name, "status": info.Status,
				"source": active.SourceType, "contextKey": active.ContextKey,
			}
		}
	}
	return map[string]any{
		"developer": dev,
		"git": map[string]any{
			"isRepo": git.IsRepo, "branch": git.Branch, "isClean": git.IsClean,
			"uncommittedChanges": git.UncommittedChanges, "recentCommits": git.RecentCommits,
		},
		"myTasks":     myTasks,
		"currentTask": currentInfo,
	}
}

func (r *Runner) contextRecordText() string {
	var lines []string
	lines = append(lines, "========================================")
	lines = append(lines, "会话上下文（记录模式）")
	lines = append(lines, "========================================")
	lines = append(lines, "")
	dev := Developer(r.Root)
	if dev == "" {
		lines = append(lines, "错误：未初始化。请运行：sunshine init <名字>")
		return strings.Join(lines, "\n")
	}
	lines = append(lines, fmt.Sprintf("## [!!!] 我的活动任务（分配给 %s）", dev))
	lines = append(lines, "[!] 记录本次会话前，先看哪些任务该归档。")
	lines = append(lines, "")
	tasksDir := TasksDir(r.Root)
	allStatuses := AllStatuses(IterActiveTasks(tasksDir, nil))
	myCount := 0
	for _, t := range IterActiveTasks(tasksDir, nil) {
		if t.Assignee == dev {
			progress := ChildrenProgress(t.Children, allStatuses)
			lines = append(lines, fmt.Sprintf("- [%s] %s (%s)%s — %s", t.Priority, t.Title, t.Status, progress, t.DirName))
			myCount++
		}
	}
	if myCount == 0 {
		lines = append(lines, "（没有分配给你的活动任务）")
	}
	lines = append(lines, "")
	git := collectRootGitInfo(r.Root)
	lines = appendRootGitContext(lines, git)
	lines = append(lines, "## 当前任务")
	active := ResolveActiveTask(r.Root, "current", "")
	if active.TaskPath != "" {
		lines = append(lines, "路径："+active.TaskPath)
		lines = append(lines, "来源："+active.Source())
		if info, err := loadTaskInfo(filepath.Join(r.Root, active.TaskPath)); err == nil && info != nil {
			lines = append(lines, "名字："+info.Name)
			lines = append(lines, "状态："+info.Status)
		}
	} else {
		lines = append(lines, "（无）")
	}
	lines = append(lines, "")
	lines = append(lines, "========================================")
	return strings.Join(lines, "\n")
}

// ---------------------------------------------------------------------------
// packages 模式
// ---------------------------------------------------------------------------

type packageInfo struct {
	Name        string   `json:"name"`
	Path        string   `json:"path"`
	Type        string   `json:"type"`
	Default     bool     `json:"default"`
	SpecLayers  []string `json:"specLayers"`
	IsSubmodule bool     `json:"isSubmodule"`
	IsGitRepo   bool     `json:"isGitRepo"`
}

func (r *Runner) packagesInfo() []packageInfo {
	cfg := LoadConfig(r.Root)
	if len(cfg.Packages) == 0 {
		return nil
	}
	specDir := filepath.Join(r.Root, DirSunshine, "spec")
	var out []packageInfo
	names := make([]string, 0, len(cfg.Packages))
	for name := range cfg.Packages {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		out = append(out, packageInfo{
			Name: name, Path: name, Type: "local",
			Default:    name == cfg.DefaultPackage,
			SpecLayers: scanSpecLayers(filepath.Join(specDir, name)),
			IsGitRepo:  isDir(filepath.Join(r.Root, name, ".git")),
		})
	}
	return out
}

func scanSpecLayers(dir string) []string {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return []string{}
	}
	var layers []string
	for _, e := range entries {
		if e.IsDir() && e.Name() != "guides" {
			layers = append(layers, e.Name())
		}
	}
	sort.Strings(layers)
	if layers == nil {
		layers = []string{}
	}
	return layers
}

func (r *Runner) packagesSection() string {
	cfg := LoadConfig(r.Root)
	pkgs := r.packagesInfo()
	var lines []string
	lines = append(lines, "## 包")
	if len(pkgs) == 0 {
		lines = append(lines, "（单仓模式）")
		if layers := scanSpecLayers(filepath.Join(r.Root, DirSunshine, "spec")); len(layers) > 0 {
			lines = append(lines, "规范分层："+strings.Join(layers, ", "))
		}
		return strings.Join(lines, "\n")
	}
	for _, pkg := range pkgs {
		line := "- " + pkg.Name
		if len(pkg.SpecLayers) > 0 {
			line += "  [" + strings.Join(pkg.SpecLayers, ", ") + "]"
		}
		if pkg.IsGitRepo {
			line += "  （git 仓库）"
		}
		if pkg.Default {
			line += "  *"
		}
		lines = append(lines, line)
	}
	if cfg.DefaultPackage != "" {
		lines = append(lines, "默认包："+cfg.DefaultPackage)
	}
	return strings.Join(lines, "\n")
}

func (r *Runner) contextPackagesText() string {
	pkgs := r.packagesInfo()
	var lines []string
	if len(pkgs) == 0 {
		lines = append(lines, "单仓项目（未配置包）")
		lines = append(lines, "")
		if layers := scanSpecLayers(filepath.Join(r.Root, DirSunshine, "spec")); len(layers) > 0 {
			lines = append(lines, "规范分层："+strings.Join(layers, ", "))
		}
		return strings.Join(lines, "\n")
	}
	cfg := LoadConfig(r.Root)
	lines = append(lines, "## 包")
	lines = append(lines, "")
	for _, pkg := range pkgs {
		title := "### " + pkg.Name
		if pkg.Default {
			title += "（默认）"
		}
		if pkg.IsGitRepo {
			title += " [git 仓库]"
		}
		lines = append(lines, title)
		lines = append(lines, "路径："+pkg.Path)
		if len(pkg.SpecLayers) > 0 {
			lines = append(lines, "规范分层："+strings.Join(pkg.SpecLayers, ", "))
			for _, layer := range pkg.SpecLayers {
				lines = append(lines, fmt.Sprintf("  - %s/spec/%s/%s/index.md", DirSunshine, pkg.Name, layer))
			}
		} else {
			lines = append(lines, "规范：未配置")
		}
		lines = append(lines, "")
	}
	if isDir(filepath.Join(r.Root, DirSunshine, "spec", "guides")) {
		lines = append(lines, "### 共享指南（始终包含）")
		lines = append(lines, "路径：.sunshine/spec/guides/index.md")
		lines = append(lines, "")
	}
	_ = cfg
	return strings.Join(lines, "\n")
}

func (r *Runner) contextPackagesJSON() map[string]any {
	pkgs := r.packagesInfo()
	if len(pkgs) == 0 {
		return map[string]any{
			"mode":       "single-repo",
			"specLayers": scanSpecLayers(filepath.Join(r.Root, DirSunshine, "spec")),
		}
	}
	cfg := LoadConfig(r.Root)
	var activeTaskPackage string
	if active := ResolveActiveTask(r.Root, "current", ""); active.TaskPath != "" {
		if info, err := loadTaskInfo(filepath.Join(r.Root, active.TaskPath)); err == nil && info != nil {
			activeTaskPackage = info.Package
		}
	}
	return map[string]any{
		"mode":              "monorepo",
		"packages":          pkgs,
		"defaultPackage":    cfg.DefaultPackage,
		"specScope":         cfg.SpecScope,
		"activeTaskPackage": activeTaskPackage,
	}
}

// ---------------------------------------------------------------------------
// phase 模式（workflow_phase.py）
// ---------------------------------------------------------------------------

var stepHeadingRe = regexp.MustCompile(`^####\s+(\d+\.\d+)\b.*$`)
var markerRe = regexp.MustCompile(`^\[(/?)([A-Za-z][^\[\]]*)\]\s*$`)
var workflowTagOpenRe = regexp.MustCompile(`\[workflow-state:([A-Za-z0-9_-]+)\]`)

func readWorkflow(root string) (string, error) {
	data, err := os.ReadFile(filepath.Join(root, DirSunshine, "workflow.md"))
	if err != nil {
		return "", err
	}
	return string(data), nil
}

// getPhaseIndex 取 workflow.md 里 Phase Index 一节（到 Phase 1 之前），
// 去掉 [workflow-state:...] 标签块。
func getPhaseIndex(root string) string {
	text, err := readWorkflow(root)
	if err != nil {
		return ""
	}
	lines := strings.Split(text, "\n")
	start, end := -1, len(lines)
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if start == -1 && trimmed == "## Phase Index" {
			start = i
			continue
		}
		if start != -1 && trimmed == "## Phase 1: Plan" {
			end = i
			break
		}
	}
	if start == -1 {
		return ""
	}
	section := strings.Join(lines[start:end], "\n")
	return strings.TrimRight(stripWorkflowStateBlocks(section), "\n") + "\n"
}

// getStep 取 workflow.md 里某 #### X.X 步骤的正文。
func getWorkflowStep(root, stepID string) string {
	text, err := readWorkflow(root)
	if err != nil {
		return ""
	}
	lines := strings.Split(text, "\n")
	start := -1
	for i, line := range lines {
		if m := stepHeadingRe.FindStringSubmatch(line); m != nil && m[1] == stepID {
			start = i
			break
		}
	}
	if start == -1 {
		return ""
	}
	end := len(lines)
	for j := start + 1; j < len(lines); j++ {
		line := lines[j]
		if strings.HasPrefix(line, "#### ") || strings.HasPrefix(line, "## ") || strings.TrimSpace(line) == "---" {
			end = j
			break
		}
	}
	return strings.TrimRight(strings.Join(lines[start:end], "\n"), "\n") + "\n"
}

// stripWorkflowStateBlocks 去掉 [workflow-state:X]…[/workflow-state:X] 标签块。
func stripWorkflowStateBlocks(content string) string {
	var out strings.Builder
	rest := content
	for {
		loc := workflowTagOpenRe.FindStringSubmatchIndex(rest)
		if loc == nil {
			out.WriteString(rest)
			break
		}
		status := rest[loc[2]:loc[3]]
		closeTag := "[/workflow-state:" + status + "]"
		idx := strings.Index(rest[loc[1]:], closeTag)
		if idx == -1 {
			out.WriteString(rest)
			break
		}
		out.WriteString(rest[:loc[0]])
		rest = rest[loc[1]+idx+len(closeTag):]
	}
	return out.String()
}

// filterPlatform 按平台标记过滤内容：只保留包含该平台的 [A, B]…[/A, B] 块。
func filterPlatform(content, platform string) string {
	needle := normalizePlatformName(platform)
	var out []string
	var block []string
	inBlock := false
	keepBlock := false
	flush := func() {
		if inBlock && keepBlock {
			out = append(out, block...)
		}
		block = nil
		inBlock = false
	}
	for _, line := range strings.Split(content, "\n") {
		if m := markerRe.FindStringSubmatch(strings.TrimSpace(line)); m != nil {
			if m[1] == "/" {
				flush()
				continue
			}
			flush()
			inBlock = true
			keepBlock = false
			for _, name := range strings.Split(m[2], ",") {
				if normalizePlatformName(strings.TrimSpace(name)) == needle {
					keepBlock = true
				}
			}
			continue
		}
		if inBlock {
			block = append(block, line)
		} else {
			out = append(out, line)
		}
	}
	flush()
	return strings.Join(out, "\n")
}

func normalizePlatformName(s string) string {
	s = strings.ToLower(s)
	s = strings.ReplaceAll(s, "-", "")
	s = strings.ReplaceAll(s, "_", "")
	s = strings.ReplaceAll(s, " ", "")
	return s
}
