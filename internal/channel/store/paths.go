package store

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// bucketMarker 是项目桶目录里的标记文件：有它才算一个项目桶。
const bucketMarker = ".bucket"

// ChannelRoot 返回 channel 存储根：环境变量 SUNSHINE_CHANNEL_ROOT
// 优先，否则为 ~/.sunshine/channels/。
func ChannelRoot() string {
	if env := strings.TrimSpace(os.Getenv("SUNSHINE_CHANNEL_ROOT")); env != "" {
		abs, err := filepath.Abs(env)
		if err == nil {
			return abs
		}
		return env
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return filepath.Join(".sunshine", "channels")
	}
	return filepath.Join(home, ".sunshine", "channels")
}

// ProjectKey 由绝对 cwd 按 Claude Code 约定生成：/、\、_ → -，
// 其余非法字符（字母、数字、.、- 之外）→ -。
func ProjectKey(cwd string) string {
	abs, err := filepath.Abs(cwd)
	if err != nil {
		abs = cwd
	}
	replaced := strings.NewReplacer("/", "-", "\\", "-", "_", "-").Replace(abs)
	return illegalProjectKeyChars.ReplaceAllString(replaced, "-")
}

var illegalProjectKeyChars = regexp.MustCompile(`[^A-Za-z0-9.-]`)

// CurrentProjectKey 返回当前调用的项目桶：环境变量
// SUNSHINE_CHANNEL_PROJECT 优先，否则由当前工作目录派生。
func CurrentProjectKey() string {
	if env := strings.TrimSpace(os.Getenv("SUNSHINE_CHANNEL_PROJECT")); env != "" {
		return env
	}
	cwd, err := os.Getwd()
	if err != nil {
		return ""
	}
	return ProjectKey(cwd)
}

// ProjectDir 返回某项目桶的目录。
func ProjectDir(project string) string {
	if project == "" {
		project = CurrentProjectKey()
	}
	return filepath.Join(ChannelRoot(), project)
}

// safeSegmentRe 是 channel / worker 名作为路径段时允许的字符。
var safeSegmentRe = regexp.MustCompile(`^[A-Za-z0-9._-]+$`)

// IsSafeName 判断名字是否可用作文件系统路径段。
func IsSafeName(name string) bool {
	return name != "." && name != ".." && safeSegmentRe.MatchString(name)
}

// AssertSafeName 拒绝会逃出桶目录的名字（如 ../../x）。
func AssertSafeName(name string, kind string) error {
	if kind == "" {
		kind = "channel"
	}
	if !IsSafeName(name) {
		return fmt.Errorf("无效的 %s 名 %q，只允许字母、数字、.、_ 和 -", kind, name)
	}
	return nil
}

// ChannelDir 返回 channel 目录（不保证存在）。
func ChannelDir(name string, project string) (string, error) {
	if err := AssertSafeName(name, "channel"); err != nil {
		return "", err
	}
	return filepath.Join(ProjectDir(project), name), nil
}

// EventsPath 返回 channel 的 events.jsonl 路径。
func EventsPath(name string, project string) (string, error) {
	dir, err := ChannelDir(name, project)
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "events.jsonl"), nil
}

// SeqSidecarPath 返回 channel 的 .seq sidecar 路径。
func SeqSidecarPath(name string, project string) (string, error) {
	dir, err := ChannelDir(name, project)
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, ".seq"), nil
}

// LockPath 返回 channel 锁文件路径：<channelDir>/<name>.lock。
func LockPath(name string, project string) (string, error) {
	dir, err := ChannelDir(name, project)
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, name+".lock"), nil
}

// WorkerFile 返回 channel 目录下某 worker 的 <worker>.<suffix> 文件路径。
func WorkerFile(name string, worker string, suffix string, project string) (string, error) {
	if err := AssertSafeName(worker, "worker"); err != nil {
		return "", err
	}
	dir, err := ChannelDir(name, project)
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, worker+"."+suffix), nil
}

// WorkerLockPath 返回某 worker 的 spawn 锁路径：<worker>.spawnlock。
func WorkerLockPath(name string, worker string, project string) (string, error) {
	return WorkerFile(name, worker, "spawnlock", project)
}

// EnsureChannelDir 创建 channel 目录（权限 0700），同时确保项目桶的
// .bucket 标记文件存在（桶发现依赖它），返回目录路径。
func EnsureChannelDir(name string, project string) (string, error) {
	dir, err := ChannelDir(name, project)
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", fmt.Errorf("创建 channel 目录失败：%w", err)
	}
	if err := EnsureBucketMarker(project); err != nil {
		return "", err
	}
	return dir, nil
}

// EnsureBucketMarker 创建项目桶目录并写入 .bucket 标记文件。
func EnsureBucketMarker(project string) error {
	dir := ProjectDir(project)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("创建项目桶失败：%w", err)
	}
	marker := filepath.Join(dir, bucketMarker)
	if _, err := os.Stat(marker); os.IsNotExist(err) {
		if err := os.WriteFile(marker, nil, 0o644); err != nil {
			return fmt.Errorf("写入桶标记失败：%w", err)
		}
	}
	return nil
}

// ListProjects 列出存储根下全部项目桶（含 _global）。
func ListProjects() []string {
	root := ChannelRoot()
	entries, err := os.ReadDir(root)
	if err != nil {
		return nil
	}
	var out []string
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		dir := filepath.Join(root, entry.Name())
		if _, err := os.Stat(filepath.Join(dir, bucketMarker)); err == nil {
			out = append(out, entry.Name())
			continue
		}
		// 无标记文件的桶：仅当其下已有 channel 目录时才算桶，避免把
		// 杂目录误认进来；_global 桶始终计入。
		if entry.Name() == GlobalProjectKey {
			out = append(out, entry.Name())
		}
	}
	return out
}

// ListChannelNamesInProject 列出某项目桶内含 events.jsonl 的 channel 名。
func ListChannelNamesInProject(project string) []string {
	dir := ProjectDir(project)
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	var out []string
	for _, entry := range entries {
		if !entry.IsDir() || strings.HasPrefix(entry.Name(), ".") {
			continue
		}
		if !IsSafeName(entry.Name()) {
			continue
		}
		if _, err := os.Stat(filepath.Join(dir, entry.Name(), "events.jsonl")); err == nil {
			out = append(out, entry.Name())
		}
	}
	return out
}

// ResolveChannelOptions 是 ResolveChannel / ResolveExistingChannelRef 的选项。
type ResolveChannelOptions struct {
	// Scope 为空时默认 project。
	Scope ChannelScope
	// Cwd 为空时用当前工作目录派生 projectKey。
	Cwd string
}

// ResolveChannel 为新建 channel 解析 ChannelRef：global 用固定
// _global 桶，project 用 Cwd（或当前工作目录）派生项目桶。
func ResolveChannel(name string, opts ResolveChannelOptions) (ChannelRef, error) {
	scope := opts.Scope
	if scope == "" {
		scope = ScopeProject
	}
	if scope != ScopeProject && scope != ScopeGlobal {
		return ChannelRef{}, fmt.Errorf("无效的 --scope %q，必须是 project 或 global", string(scope))
	}
	var project string
	if scope == ScopeGlobal {
		project = GlobalProjectKey
	} else if opts.Cwd != "" {
		project = ProjectKey(opts.Cwd)
	} else {
		project = CurrentProjectKey()
	}
	dir, err := ChannelDir(name, project)
	if err != nil {
		return ChannelRef{}, err
	}
	return ChannelRef{Name: name, Scope: scope, Project: project, Dir: dir}, nil
}

// ResolveExistingChannelRef 解析一个已存在的 channel：指定 scope 时
// 只在该桶里找；未指定时先找当前项目桶，再找 global 桶，最后在其他
// 项目桶里找。两处以上同名时报错，要求明确指定 scope。
func ResolveExistingChannelRef(name string, opts ResolveChannelOptions) (ChannelRef, error) {
	if err := AssertSafeName(name, "channel"); err != nil {
		return ChannelRef{}, err
	}
	projectFor := func(scope ChannelScope) string {
		if scope == ScopeGlobal {
			return GlobalProjectKey
		}
		if opts.Cwd != "" {
			return ProjectKey(opts.Cwd)
		}
		return CurrentProjectKey()
	}
	exists := func(project string) bool {
		p, err := EventsPath(name, project)
		if err != nil {
			return false
		}
		_, err = os.Stat(p)
		return err == nil
	}
	refFor := func(scope ChannelScope, project string) (ChannelRef, error) {
		dir, err := ChannelDir(name, project)
		if err != nil {
			return ChannelRef{}, err
		}
		return ChannelRef{Name: name, Scope: scope, Project: project, Dir: dir}, nil
	}

	if opts.Scope != "" {
		project := projectFor(opts.Scope)
		if !exists(project) {
			return ChannelRef{}, fmt.Errorf("channel %q 在 %s 作用域（%s）下不存在", name, string(opts.Scope), project)
		}
		return refFor(opts.Scope, project)
	}

	current := CurrentProjectKey()
	globalExists := exists(GlobalProjectKey)
	var projectMatches []string
	for _, project := range ListProjects() {
		if project == GlobalProjectKey {
			continue
		}
		if exists(project) {
			projectMatches = append(projectMatches, project)
		}
	}
	if globalExists && len(projectMatches) > 0 {
		return ChannelRef{}, fmt.Errorf("channel %q 同时存在于 global 与项目作用域，请用 --scope global 或 --scope project 明确指定", name)
	}
	if globalExists {
		return refFor(ScopeGlobal, GlobalProjectKey)
	}
	if exists(current) {
		return refFor(ScopeProject, current)
	}
	if len(projectMatches) == 1 {
		return refFor(ScopeProject, projectMatches[0])
	}
	if len(projectMatches) > 1 {
		return ChannelRef{}, fmt.Errorf("channel %q 存在于多个项目桶（%s），请回到所属项目目录或用 --scope 指定", name, strings.Join(projectMatches, "、"))
	}
	return ChannelRef{}, fmt.Errorf("channel %q 在当前项目桶（%s）或任何已知作用域下都不存在", name, current)
}
