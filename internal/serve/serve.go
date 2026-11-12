// Package serve 实现 sunshine serve 只读观测后台（Go + Gin，嵌进二进制）。
//
// 只读：全部 GET，无任何写端点；不写盘（Gin 日志除外）；只绑本机地址。
// 数据一律复用内部包（task / channel store / mem / update / upgrade /
// config / lifecycle / workflow）拿结构化结果，不解析 CLI 纯文本输出。
package serve

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/BondLam/Sunshine/internal/channel/store"
	"github.com/BondLam/Sunshine/internal/config"
	"github.com/BondLam/Sunshine/internal/hook"
	"github.com/BondLam/Sunshine/internal/lifecycle"
	"github.com/BondLam/Sunshine/internal/task"
	"github.com/BondLam/Sunshine/internal/update"
	"github.com/BondLam/Sunshine/internal/upgrade"
	"github.com/BondLam/Sunshine/internal/workflow"
)

// templateCacheTTL 是模板集合与最新版检查的缓存有效期。
const templateCacheTTL = 10 * time.Minute

// latestVersionCacheTTL 是最新版检查的缓存有效期。
const latestVersionCacheTTL = 10 * time.Minute

// Server 是只读观测后台的 HTTP 服务。
type Server struct {
	root         string
	projectKey   string
	pollInterval int
	// cliVersion / repo 由 CLI 层传入（serve 不能 import internal/cli，
	// 否则与 internal/cli/serve.go 循环导入）。
	cliVersion string
	repo       string

	mu               sync.Mutex
	templates        map[string]string
	templatesAt      time.Time
	latestVersion    string
	latestVersionAt  time.Time
	latestVersionSet bool

	activity *activityTracker
}

// NewServer 建一个后台服务实例。root 须为项目根（含 .sunshine/）。
func NewServer(root string, pollInterval int, cliVersion, repo string) *Server {
	return &Server{
		root:         root,
		projectKey:   store.ProjectKey(root),
		pollInterval: pollInterval,
		cliVersion:   cliVersion,
		repo:         repo,
		activity:     newActivityTracker(root),
	}
}

// Root 返回服务对应的项目根。
func (s *Server) Root() string { return s.root }

// PollInterval 返回轮询秒数。
func (s *Server) PollInterval() int { return s.pollInterval }

// ProjectKey 返回当前项目的 channel 分桶 key。
func (s *Server) ProjectKey() string { return s.projectKey }

// Router 建 Gin 路由（ReleaseMode，不信任任何代理）。
func (s *Server) Router() *gin.Engine {
	gin.SetMode(gin.ReleaseMode)
	r := gin.New()
	r.Use(gin.Logger())
	r.Use(gin.Recovery())
	_ = r.SetTrustedProxies(nil)

	r.GET("/", s.handleIndex)
	r.GET("/static/*filepath", s.handleStatic)

	api := r.Group("/api")
	api.GET("/status", s.handleStatus)
	api.GET("/tasks", s.handleTasks)
	api.GET("/tasks/current", s.handleCurrentTask)
	api.GET("/tasks/archive", s.handleArchive)
	api.GET("/tasks/:id", s.handleTaskDetail)
	api.GET("/tasks/:id/commits", s.handleTaskCommits)
	api.GET("/channels", s.handleChannels)
	api.GET("/channels/:name/events", s.handleChannelEvents)
	api.GET("/channels/:name/workers", s.handleChannelWorkers)
	api.GET("/channels/:name/workers/:worker/log", s.handleWorkerLog)
	api.GET("/channels/:name/threads", s.handleChannelThreads)
	api.GET("/channels/:name/threads/:thread", s.handleChannelThread)
	api.GET("/mem/sessions", s.handleMemSessions)
	api.GET("/mem/projects", s.handleMemProjects)
	api.GET("/health", s.handleHealth)
	api.GET("/version", s.handleVersion)
	api.GET("/design", s.handleDesign)
	api.GET("/journal", s.handleJournal)
	api.GET("/activity", s.handleActivity)
	return r
}

// Run 在 addr:port 上启动服务。端口被占时直接报错，不自动跳端口。
func (s *Server) Run(addr string, port int) error {
	listener, err := net.Listen("tcp", net.JoinHostPort(addr, fmt.Sprintf("%d", port)))
	if err != nil {
		return fmt.Errorf("端口 %d 已被占用：%v，请换一个端口（--port）后重试", port, err)
	}
	srv := &http.Server{Handler: s.Router()}
	return srv.Serve(listener)
}

// ---------------------------------------------------------------------------
// 共用小工具
// ---------------------------------------------------------------------------

func errorJSON(c *gin.Context, code int, format string, args ...any) {
	c.JSON(code, gin.H{"error": fmt.Sprintf(format, args...)})
}

func strSlice(in []string) []string {
	if in == nil {
		return []string{}
	}
	return in
}

func strPtrValue(p *string) any {
	if p == nil {
		return nil
	}
	return *p
}

// displayStatus 是 lifecycle.go:689 的 displayStatus 规则的复制
// （未导出）：planning 父任务已有非 planning 子女时显示 active。
// 来源：internal/task/lifecycle.go displayStatus。
func displayStatus(t *task.TaskInfo, allStatuses map[string]string) string {
	if t.Status == "planning" && len(t.Children) > 0 {
		for _, c := range t.Children {
			if st, ok := allStatuses[c]; ok && st != "planning" {
				return "active"
			}
		}
	}
	return t.Status
}

// childrenProgressCounts 按 ChildrenProgress 口径计数：归档子女
// （不在活动集里）与 completed/done 子女都算完成。
// 与 task.ChildrenProgress 的计数逻辑一致，只返回数字不拼字符串。
func childrenProgressCounts(children []string, allStatuses map[string]string) (done, total int) {
	total = len(children)
	for _, c := range children {
		if st, ok := allStatuses[c]; !ok || st == "completed" || st == "done" {
			done++
		}
	}
	return done, total
}

// taskListDTO 把 TaskInfo 转成 /api/tasks 的行。
func taskListDTO(t *task.TaskInfo, allStatuses map[string]string) map[string]any {
	done, total := childrenProgressCounts(t.Children, allStatuses)
	return map[string]any{
		"dir":            t.DirName,
		"id":             t.Name,
		"title":          t.Title,
		"status":         t.Status,
		"display_status": displayStatus(t, allStatuses),
		"priority":       t.Priority,
		"assignee":       t.Assignee,
		"parent":         t.Parent,
		"children":       strSlice(t.Children),
		"package":        t.Package,
		"progress_done":  done,
		"progress_total": total,
	}
}

// tasksDir 返回任务目录绝对路径。
func (s *Server) tasksDir() string {
	return task.TasksDir(s.root)
}

// activeTasks 返回活动任务与状态表。
func (s *Server) activeTasks() ([]*task.TaskInfo, map[string]string) {
	tasks := task.IterActiveTasks(s.tasksDir(), io.Discard)
	return tasks, task.AllStatuses(tasks)
}

// channelRef 由 channel 名解析当前项目的 channel 引用。
func (s *Server) channelRef(name string) (store.ChannelRef, error) {
	if err := store.AssertSafeName(name, "channel"); err != nil {
		return store.ChannelRef{}, err
	}
	return store.ChannelRef{Name: name, Project: s.projectKey}, nil
}

// readChannelEvents 读某 channel 的全部事件；事件文件不存在时返回 404 错误。
func (s *Server) readChannelEvents(name string) ([]store.ChannelEvent, error) {
	ref, err := s.channelRef(name)
	if err != nil {
		return nil, err
	}
	path, err := store.EventsPath(name, s.projectKey)
	if err != nil {
		return nil, err
	}
	if _, err := os.Stat(path); err != nil {
		return nil, errChannelNotFound
	}
	events, err := store.ReadChannelEvents(ref, nil)
	if err != nil {
		return nil, err
	}
	return events, nil
}

var errChannelNotFound = fmt.Errorf("channel 不存在")

// ---------------------------------------------------------------------------
// 版本与 workflow 模板名
// ---------------------------------------------------------------------------

// versions 返回项目版、CLI 版、最新版三方。最新版走 upgrade.Client
// 查 GitHub Releases，缓存 10 分钟；离线失败时返回 ""，不报错。
func (s *Server) versions() (projectVersion, cliVersion, latestVersion string) {
	projectVersion = update.GetInstalledVersion(s.root)
	cliVersion = s.cliVersion

	s.mu.Lock()
	if s.latestVersionSet && time.Since(s.latestVersionAt) < latestVersionCacheTTL {
		latestVersion = s.latestVersion
		s.mu.Unlock()
		return projectVersion, cliVersion, latestVersion
	}
	s.mu.Unlock()

	latest := ""
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	if v, err := (&upgrade.Client{Repo: s.repo}).LatestVersion(ctx); err == nil {
		latest = v
	}

	s.mu.Lock()
	s.latestVersion = latest
	s.latestVersionAt = time.Now()
	s.latestVersionSet = true
	s.mu.Unlock()
	return projectVersion, cliVersion, latest
}

// workflowTemplateName 用 sha256 比对项目 workflow.md 与三个内嵌模板
// （原始字节，不 TrimSpace、不换行归一化）：命中返回模板名，未命中
// 返回“已修改”；比对机制不可用时返回 ""（调用方不展示该字段）。
func (s *Server) workflowTemplateName() string {
	data, err := os.ReadFile(filepath.Join(s.root, task.DirSunshine, "workflow.md"))
	if err != nil {
		return ""
	}
	sum := sha256.Sum256(data)
	hexSum := hex.EncodeToString(sum[:])
	checked := false
	for _, id := range []string{"native", "tdd", "channel-driven-subagent-dispatch"} {
		content, err := workflow.BundledContent(id)
		if err != nil {
			continue
		}
		checked = true
		tmplSum := sha256.Sum256([]byte(content))
		if hex.EncodeToString(tmplSum[:]) == hexSum {
			return id
		}
	}
	if !checked {
		return ""
	}
	return "已修改"
}

// cachedTemplates 返回模板集合（CollectTemplateFiles 可能触发
// registry 刷新，进程内缓存 10 分钟，不每轮 poll 都调）。
func (s *Server) cachedTemplates(hashes map[string]string) (map[string]string, error) {
	s.mu.Lock()
	if s.templates != nil && time.Since(s.templatesAt) < templateCacheTTL {
		tmpl := s.templates
		s.mu.Unlock()
		return tmpl, nil
	}
	s.mu.Unlock()

	templates, _, err := update.CollectTemplateFiles(io.Discard, s.root, hashes, update.CollectOptions{})
	if err != nil {
		return nil, err
	}
	s.mu.Lock()
	s.templates = templates
	s.templatesAt = time.Now()
	s.mu.Unlock()
	return templates, nil
}

// configSummary 返回 config.yaml 的 5 个已启用键摘要。
func (s *Server) configSummary() map[string]any {
	cfg, err := config.LoadProject(s.root)
	if err != nil || cfg == nil {
		cfg = config.Default()
	}
	return map[string]any{
		"session_commit_message":                cfg.SessionCommitMessage,
		"max_journal_lines":                     cfg.MaxJournalLines,
		"channel.worker_guard.idle_timeout":     cfg.Channel.WorkerGuard.IdleTimeout,
		"channel.worker_guard.max_live_workers": cfg.Channel.WorkerGuard.MaxLiveWorkers,
		"tasks_dir":                             cfg.TasksDir,
		"spec_dir":                              cfg.SpecDir,
	}
}

// platforms 返回已装平台清单。
func (s *Server) platforms() []lifecycle.PlatformInfo {
	platforms, err := lifecycle.ConfiguredPlatforms(s.root)
	if err != nil {
		return []lifecycle.PlatformInfo{}
	}
	if platforms == nil {
		return []lifecycle.PlatformInfo{}
	}
	return platforms
}

// developer 返回当前开发者名。
func (s *Server) developer() string {
	return hook.GetDeveloper(s.root)
}

// currentTaskObject 组装当前任务对象（与 task current --json 同款字段）。
// 无当前任务时返回 nil。
func (s *Server) currentTaskObject() (map[string]any, bool, string, error) {
	active := task.ResolveActiveTask(s.root, "current", "")
	if active.TaskPath == "" {
		return nil, active.Stale, active.Source(), nil
	}
	dir := hook.ResolveTaskRef(active.TaskPath, s.root)
	if dir == "" {
		dir = filepath.Join(s.root, active.TaskPath)
	}
	rec, err := task.LoadRecord(dir)
	if err != nil {
		// 指针指向的任务读不到：仍返回目录与来源，标记过期。
		return map[string]any{
			"dir":         active.TaskPath,
			"id":          filepath.Base(active.TaskPath),
			"title":       nil,
			"status":      nil,
			"parent":      nil,
			"children":    []string{},
			"branch":      nil,
			"base_branch": nil,
		}, true, active.Source(), nil
	}
	id := rec.ID
	if id == "" {
		id = rec.Name
	}
	return map[string]any{
		"dir":         active.TaskPath,
		"id":          id,
		"title":       rec.Title,
		"status":      rec.Status,
		"parent":      strPtrValue(rec.Parent),
		"children":    strSlice(rec.Children),
		"branch":      strPtrValue(rec.Branch),
		"base_branch": strPtrValue(rec.BaseBranch),
	}, active.Stale, active.Source(), nil
}
