package serve

import (
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/linrong1982/Sunshine/internal/channel/store"
	"github.com/linrong1982/Sunshine/internal/task"
	"github.com/linrong1982/Sunshine/internal/workflow"
)

// testProject 是 serve 测试用的临时项目夹。
type testProject struct {
	root     string
	channels string
	server   *Server
	router   http.Handler
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func writeTask(t *testing.T, dir string, mutate func(*task.TaskRecord)) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	rec := task.EmptyRecord()
	rec.ID = filepath.Base(dir)
	rec.Name = filepath.Base(dir)
	rec.Title = "任务 " + filepath.Base(dir)
	rec.Description = "测试任务"
	rec.Creator = "tester"
	rec.Assignee = "tester"
	if mutate != nil {
		mutate(rec)
	}
	if err := task.WriteRecord(dir, rec); err != nil {
		t.Fatalf("写 task.json 失败：%v", err)
	}
}

func writeChannelEvents(t *testing.T, dir string, events []store.ChannelEvent) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	var b strings.Builder
	for _, ev := range events {
		data, err := json.Marshal(ev)
		if err != nil {
			t.Fatal(err)
		}
		b.Write(data)
		b.WriteString("\n")
	}
	writeFile(t, filepath.Join(dir, "events.jsonl"), b.String())
}

func event(seq int64, ts, kind, by string, extra map[string]any) store.ChannelEvent {
	return store.ChannelEvent{
		Seq: seq, Ts: ts, Kind: store.ChannelEventKind(kind), By: by, Extra: extra,
	}
}

// newTestProject 建一个带任务、归档、channel、journal、doc/ 的临时项目。
// workflow 模板用 native 原文（哈希命中）；repo 传 "" 使最新版检查
// 不联网、直接返回 ""。
func newTestProject(t *testing.T) *testProject {
	t.Helper()
	root := t.TempDir()
	channels := t.TempDir()
	t.Setenv("SUNSHINE_CHANNEL_ROOT", channels)

	writeFile(t, filepath.Join(root, ".sunshine", "config.yaml"),
		"session_commit_message: \"chore: record journal\"\n"+
			"max_journal_lines: 2000\n"+
			"channel:\n  worker_guard:\n    idle_timeout: \"5m\"\n    max_live_workers: 6\n"+
			"tasks_dir: \".sunshine/tasks\"\n"+
			"spec_dir: \".sunshine/spec\"\n")
	writeFile(t, filepath.Join(root, ".sunshine", ".version"), "0.3.11\n")
	writeFile(t, filepath.Join(root, ".sunshine", ".developer"), "tester\n")
	native, err := workflow.BundledContent("native")
	if err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(root, ".sunshine", "workflow.md"), native)

	tasksDir := filepath.Join(root, ".sunshine", "tasks")
	writeTask(t, filepath.Join(tasksDir, "11-12-parent"), func(r *task.TaskRecord) {
		r.Title = "父任务"
		r.Children = []string{"11-12-child"}
	})
	writeFile(t, filepath.Join(tasksDir, "11-12-parent", "prd.md"), "# 父任务\n\n## 目标\n\n测试。\n")
	writeFile(t, filepath.Join(tasksDir, "11-12-parent", "implement.jsonl"),
		"{\"file\": \"a.go\", \"type\": \"code\", \"reason\": \"实现\"}\n")
	writeFile(t, filepath.Join(tasksDir, "11-12-parent", "check.jsonl"),
		"{\"file\": \"a_test.go\", \"type\": \"test\", \"reason\": \"检查\"}\n")
	writeTask(t, filepath.Join(tasksDir, "11-12-child"), func(r *task.TaskRecord) {
		r.Title = "子任务"
		r.Status = "in_progress"
		r.Parent = strptr("11-12-parent")
	})
	writeFile(t, filepath.Join(tasksDir, "11-12-child", "prd.md"), "# 子任务\n")
	writeTask(t, filepath.Join(tasksDir, "archive", "2026-10", "10-01-old"), func(r *task.TaskRecord) {
		r.Title = "旧任务"
		r.Status = "completed"
	})
	writeFile(t, filepath.Join(root, ".sunshine", ".current-task"), ".sunshine/tasks/11-12-child\n")
	writeFile(t, filepath.Join(root, ".sunshine", "workspace", "tester", "journal-1.md"),
		"# 会话记录\n\n- 做了测试任务。\n")
	writeFile(t, filepath.Join(root, "doc", "design", "需求列表.md"), "# 需求列表\n\n- 需求一\n- 需求二\n")
	writeFile(t, filepath.Join(root, "doc", "design", "概要设计.md"), "# 概要设计\n\n## 模块\n\n- 模块 A\n")
	writeFile(t, filepath.Join(root, "doc", "prototype", "原型.html"), "<html><body>原型</body></html>\n")

	// channel：dev-channel 带完整事件流，live-channel 只有一个运行中 worker。
	projectKey := store.ProjectKey(root)
	devDir := filepath.Join(channels, projectKey, "dev-channel")
	writeChannelEvents(t, devDir, []store.ChannelEvent{
		event(1, "2026-11-12T09:00:00+08:00", "create", "main", map[string]any{
			"type": "chat", "task": "11-12-child", "description": "开发频道", "ephemeral": false,
		}),
		event(2, "2026-11-12T09:01:00+08:00", "spawned", "supervisor", map[string]any{
			"as": "w1", "agent": "implement", "provider": "claude",
		}),
		event(3, "2026-11-12T09:02:00+08:00", "progress", "w1", map[string]any{
			"worker": "w1", "text": "进度 50%",
		}),
		event(4, "2026-11-12T09:03:00+08:00", "turn_started", "w1", map[string]any{
			"worker": "w1", "turnId": "t-1",
		}),
		event(5, "2026-11-12T09:04:00+08:00", "done", "w1", map[string]any{
			"worker": "w1", "synthesized": true, "exit_code": 0,
		}),
		event(6, "2026-11-12T09:05:00+08:00", "thread", "main", map[string]any{
			"thread": "t1", "action": "opened", "title": "主题一",
		}),
		event(7, "2026-11-12T09:06:00+08:00", "thread", "w1", map[string]any{
			"thread": "t1", "action": "comment", "text": "跟进",
		}),
	})
	writeFile(t, filepath.Join(devDir, "w1.log"), "第一行\n第二行\n第三行\n")
	writeChannelEvents(t, filepath.Join(channels, projectKey, "live-channel"), []store.ChannelEvent{
		event(1, "2026-11-12T09:00:00+08:00", "create", "main", map[string]any{
			"type": "chat", "task": "11-12-parent", "description": "在线频道",
		}),
		event(2, "2026-11-12T09:01:00+08:00", "spawned", "supervisor", map[string]any{
			"as": "w2", "agent": "check", "provider": "claude",
		}),
	})

	srv := NewServer(root, 3, "test", "")
	return &testProject{root: root, channels: channels, server: srv, router: srv.Router()}
}

func strptr(s string) *string { return &s }

func (p *testProject) get(t *testing.T, path string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, path, nil)
	w := httptest.NewRecorder()
	p.router.ServeHTTP(w, req)
	return w
}

func decodeJSON(t *testing.T, w *httptest.ResponseRecorder, v any) {
	t.Helper()
	if err := json.Unmarshal(w.Body.Bytes(), v); err != nil {
		t.Fatalf("解析 JSON 失败：%v，body=%s", err, w.Body.String())
	}
}

func TestIndexAndStatic(t *testing.T) {
	p := newTestProject(t)
	w := p.get(t, "/")
	if w.Code != http.StatusOK {
		t.Fatalf("GET / 状态码 = %d", w.Code)
	}
	indexBody := w.Body.String()
	if !strings.Contains(indexBody, "<html") {
		t.Fatalf("首页不是 HTML：%q", indexBody[:200])
	}
	// 回归 v0.4.0：script 顺序决定 serveApp 能否注册。
	// Alpine 在自身脚本执行完后经 queueMicrotask 立即启动并派发 alpine:init；
	// app.js 必须排在 alpine.min.js 前面执行，否则组件注册不上、
	// 浏览器控制台报 serveApp is not defined、页面全是 —。
	// v0.4.2 改向：hero 画布已去掉，hero.js 不再加载。
	posApp := strings.Index(indexBody, "/static/app.js")
	posAlpine := strings.Index(indexBody, "/static/alpine.min.js")
	if posApp < 0 || posAlpine < 0 {
		t.Fatalf("首页缺 script：app=%d alpine=%d", posApp, posAlpine)
	}
	if !(posApp < posAlpine) {
		t.Fatalf("script 顺序错：app.js 必须在 alpine.min.js 之前（app=%d alpine=%d）", posApp, posAlpine)
	}
	if strings.Contains(indexBody, "/static/hero.js") || strings.Contains(indexBody, "hero-canvas") {
		t.Fatalf("首页仍引用已去掉的 hero 画布")
	}
	if !strings.Contains(indexBody, "cdn.tailwindcss.com") {
		t.Fatalf("首页缺少 Tailwind Play CDN")
	}
	for _, path := range []string{"/static/app.js", "/static/style.css", "/static/alpine.min.js"} {
		w := p.get(t, path)
		if w.Code != http.StatusOK {
			t.Fatalf("GET %s 状态码 = %d", path, w.Code)
		}
	}
}

// v0.4.2 大屏布局（改向后 Tailwind 卡片版）：顶栏 LAST SIGNAL、智能体团、
// 热力格、信号流、底部大指标行；旧 stat-row 已并入底部指标行，不再出现；
// hero 画布已去掉，样式走 Tailwind Play CDN。
func TestOpsDashboardLayout(t *testing.T) {
	p := newTestProject(t)
	w := p.get(t, "/")
	if w.Code != http.StatusOK {
		t.Fatalf("GET / 状态码 = %d", w.Code)
	}
	body := w.Body.String()
	for _, marker := range []string{
		`last-signal`,
		`id="cluster"`,
		`id="heatmap"`,
		`id="heat-canvas"`,
		`id="signal-feed"`,
		`信号流`,
		`metric-row`,
		`id="spark-canvas"`,
		`归档任务`,
		`健康漂移`,
		`cdn.tailwindcss.com`,
		`bg-paper`,
		`panel-magenta`,
		`panel-gold`,
	} {
		if !strings.Contains(body, marker) {
			t.Fatalf("首页缺少大屏标记 %q", marker)
		}
	}
	if strings.Contains(body, `class="stat-row"`) || strings.Contains(body, "stat-card") {
		t.Fatalf("首页仍含已废弃的 stat-row/stat-card（应并入底部大指标行）")
	}
	if strings.Contains(body, "/static/hero.js") || strings.Contains(body, "hero-canvas") || strings.Contains(body, `class="ops-row"`) {
		t.Fatalf("首页仍含已去掉的 hero 画布或旧 ops-row 布局")
	}
	// 热力格与状态灯的补充样式必须存在，否则面板是裸 HTML
	css := p.get(t, "/static/style.css")
	if css.Code != http.StatusOK {
		t.Fatalf("GET /static/style.css 状态码 = %d", css.Code)
	}
	for _, marker := range []string{".heat-labels", ".lamp", "#heat-canvas", ".mono"} {
		if !strings.Contains(css.Body.String(), marker) {
			t.Fatalf("style.css 缺少大屏样式 %q", marker)
		}
	}
	if strings.Contains(css.Body.String(), "text-shadow") || strings.Contains(css.Body.String(), "0 0 8px") || strings.Contains(css.Body.String(), "0 0 12px") {
		t.Fatalf("style.css 仍含霓虹辉光样式")
	}
}

func TestStatus(t *testing.T) {
	p := newTestProject(t)
	w := p.get(t, "/api/status")
	if w.Code != http.StatusOK {
		t.Fatalf("状态码 = %d，body=%s", w.Code, w.Body.String())
	}
	var data map[string]any
	decodeJSON(t, w, &data)
	if data["project_path"] != p.root {
		t.Errorf("project_path = %v，想要 %s", data["project_path"], p.root)
	}
	if data["project_version"] != "0.3.11" {
		t.Errorf("project_version = %v", data["project_version"])
	}
	if data["cli_version"] != "test" {
		t.Errorf("cli_version = %v", data["cli_version"])
	}
	if data["latest_version"] != "" {
		t.Errorf("最新版离线应为空，得到 %v", data["latest_version"])
	}
	if data["developer"] != "tester" {
		t.Errorf("developer = %v", data["developer"])
	}
	if data["poll_interval_sec"] != float64(3) {
		t.Errorf("poll_interval_sec = %v", data["poll_interval_sec"])
	}
	// workflow.md 与 native 模板同源字节：哈希命中。
	if data["workflow_template"] != "native" {
		t.Errorf("workflow_template = %v，想要 native", data["workflow_template"])
	}
	ct, ok := data["current_task"].(map[string]any)
	if !ok {
		t.Fatalf("current_task 不是对象：%v", data["current_task"])
	}
	if ct["title"] != "子任务" {
		t.Errorf("当前任务标题 = %v", ct["title"])
	}
	cfg, ok := data["config_summary"].(map[string]any)
	if !ok {
		t.Fatalf("config_summary 不是对象：%v", data["config_summary"])
	}
	if cfg["tasks_dir"] != ".sunshine/tasks" {
		t.Errorf("config tasks_dir = %v", cfg["tasks_dir"])
	}
	if cfg["channel.worker_guard.max_live_workers"] != float64(6) {
		t.Errorf("max_live_workers = %v", cfg["channel.worker_guard.max_live_workers"])
	}
}

func TestWorkflowTemplateModified(t *testing.T) {
	p := newTestProject(t)
	writeFile(t, filepath.Join(p.root, ".sunshine", "workflow.md"), "已改过的 workflow\n")
	w := p.get(t, "/api/status")
	var data map[string]any
	decodeJSON(t, w, &data)
	if data["workflow_template"] != "已修改" {
		t.Errorf("改过 workflow 后 workflow_template = %v，想要 已修改", data["workflow_template"])
	}
}

func TestTasks(t *testing.T) {
	p := newTestProject(t)
	w := p.get(t, "/api/tasks")
	var rows []map[string]any
	decodeJSON(t, w, &rows)
	if len(rows) != 2 {
		t.Fatalf("任务数 = %d，想要 2：%s", len(rows), w.Body.String())
	}
	byDir := map[string]map[string]any{}
	for _, r := range rows {
		byDir[r["dir"].(string)] = r
	}
	parent := byDir["11-12-parent"]
	// planning 父任务已有 in_progress 子女 → display_status 为 active。
	if parent["display_status"] != "active" {
		t.Errorf("父任务 display_status = %v，想要 active", parent["display_status"])
	}
	if parent["progress_done"] != float64(0) || parent["progress_total"] != float64(1) {
		t.Errorf("父任务进度 = %v/%v，想要 0/1", parent["progress_done"], parent["progress_total"])
	}
	child := byDir["11-12-child"]
	if child["progress_done"] != float64(0) || child["progress_total"] != float64(0) {
		t.Errorf("无子女任务进度应为 0/0，得到 %v/%v", child["progress_done"], child["progress_total"])
	}
	if child["parent"] != "11-12-parent" {
		t.Errorf("子任务 parent = %v", child["parent"])
	}
}

func TestCurrentTask(t *testing.T) {
	p := newTestProject(t)
	w := p.get(t, "/api/tasks/current")
	var data map[string]any
	decodeJSON(t, w, &data)
	ct, ok := data["current_task"].(map[string]any)
	if !ok {
		t.Fatalf("current_task 不是对象：%v", data["current_task"])
	}
	if ct["dir"] != ".sunshine/tasks/11-12-child" {
		t.Errorf("dir = %v", ct["dir"])
	}
	if ct["id"] != "11-12-child" || ct["title"] != "子任务" || ct["status"] != "in_progress" {
		t.Errorf("当前任务字段异常：%v", ct)
	}
	if data["source"] != "file:.sunshine/.current-task" {
		t.Errorf("source = %v", data["source"])
	}
	if data["stale"] != false {
		t.Errorf("stale = %v", data["stale"])
	}
}

func TestTaskDetailFieldOrder(t *testing.T) {
	p := newTestProject(t)
	w := p.get(t, "/api/tasks/11-12-parent")
	if w.Code != http.StatusOK {
		t.Fatalf("状态码 = %d，body=%s", w.Code, w.Body.String())
	}
	body := w.Body.String()
	// 24 字段按 FieldOrder 顺序出现。
	fields := []string{`"id"`, `"name"`, `"title"`, `"description"`, `"status"`,
		`"dev_type"`, `"scope"`, `"package"`, `"priority"`, `"creator"`, `"assignee"`,
		`"createdAt"`, `"completedAt"`, `"branch"`, `"base_branch"`, `"worktree_path"`,
		`"commit"`, `"pr_url"`, `"subtasks"`, `"children"`, `"parent"`, `"relatedFiles"`,
		`"notes"`, `"meta"`}
	prev := -1
	for _, f := range fields {
		idx := strings.Index(body, f)
		if idx < 0 {
			t.Fatalf("详情缺字段 %s", f)
		}
		if idx < prev {
			t.Fatalf("字段 %s 顺序错误", f)
		}
		prev = idx
	}
	var data map[string]any
	decodeJSON(t, w, &data)
	if data["prd_md"] == "" || !strings.Contains(data["prd_md"].(string), "父任务") {
		t.Errorf("prd_md 为空或内容不对：%v", data["prd_md"])
	}
	impl, ok := data["implement"].([]any)
	if !ok || len(impl) != 1 {
		t.Fatalf("implement = %v", data["implement"])
	}
	entry := impl[0].(map[string]any)
	if entry["file"] != "a.go" || entry["reason"] != "实现" {
		t.Errorf("implement 条目 = %v", entry)
	}
	check, ok := data["check"].([]any)
	if !ok || len(check) != 1 {
		t.Fatalf("check = %v", data["check"])
	}
	// 父任务 subtasks 为空数组（不是 null）。
	if _, ok := data["subtasks"].([]any); !ok {
		t.Errorf("subtasks 应为数组：%v", data["subtasks"])
	}
}

func TestTaskDetailNotFound(t *testing.T) {
	p := newTestProject(t)
	w := p.get(t, "/api/tasks/不存在的任务")
	if w.Code != http.StatusNotFound {
		t.Errorf("不存在的任务状态码 = %d，想要 404", w.Code)
	}
}

func TestArchive(t *testing.T) {
	p := newTestProject(t)
	w := p.get(t, "/api/tasks/archive")
	var data map[string]any
	decodeJSON(t, w, &data)
	months, ok := data["months"].([]any)
	if !ok || len(months) != 1 {
		t.Fatalf("months = %v", data["months"])
	}
	m := months[0].(map[string]any)
	if m["month"] != "2026-10" || m["count"] != float64(1) {
		t.Errorf("月份行 = %v", m)
	}

	w = p.get(t, "/api/tasks/archive?month=2026-10")
	var rows []map[string]any
	decodeJSON(t, w, &rows)
	if len(rows) != 1 || rows[0]["dir"] != "10-01-old" {
		t.Fatalf("归档任务 = %v", rows)
	}

	w = p.get(t, "/api/tasks/archive?month=bad")
	if w.Code != http.StatusBadRequest {
		t.Errorf("非法 month 状态码 = %d，想要 400", w.Code)
	}
}

func TestTaskCommitsEmpty(t *testing.T) {
	p := newTestProject(t)
	w := p.get(t, "/api/tasks/11-12-parent/commits")
	if w.Code != http.StatusOK {
		t.Fatalf("状态码 = %d", w.Code)
	}
	var rows []any
	decodeJSON(t, w, &rows)
	if len(rows) != 0 {
		t.Errorf("无 git 仓库时提交应为空数组，得到 %v", rows)
	}
}

func TestArchivedTaskDetailAndCommits(t *testing.T) {
	p := newTestProject(t)
	w := p.get(t, "/api/tasks/10-01-old")
	if w.Code != http.StatusOK {
		t.Fatalf("归档任务详情状态码 = %d，body=%s", w.Code, w.Body.String())
	}
	var detail map[string]any
	decodeJSON(t, w, &detail)
	if detail["title"] != "旧任务" || detail["status"] != "completed" {
		t.Errorf("归档任务详情 = %v", detail)
	}

	// 归档任务的提交端点同样须能解析（夹具不是 git 仓库，返回空数组）。
	w = p.get(t, "/api/tasks/10-01-old/commits")
	if w.Code != http.StatusOK {
		t.Fatalf("归档任务提交状态码 = %d，body=%s", w.Code, w.Body.String())
	}
	var rows []any
	decodeJSON(t, w, &rows)
	if len(rows) != 0 {
		t.Errorf("无 git 仓库时提交应为空数组，得到 %v", rows)
	}
}

func TestChannels(t *testing.T) {
	p := newTestProject(t)
	w := p.get(t, "/api/channels")
	var rows []map[string]any
	decodeJSON(t, w, &rows)
	if len(rows) != 2 {
		t.Fatalf("channel 数 = %d，想要 2：%s", len(rows), w.Body.String())
	}
	byName := map[string]map[string]any{}
	for _, r := range rows {
		byName[r["name"].(string)] = r
	}
	dev := byName["dev-channel"]
	if dev["totalEvents"] != float64(7) {
		t.Errorf("totalEvents = %v，想要 7", dev["totalEvents"])
	}
	if dev["workersTotal"] != float64(1) || dev["workersAlive"] != float64(0) {
		t.Errorf("worker 数 = %v/%v，想要 alive 0 / total 1", dev["workersAlive"], dev["workersTotal"])
	}
	// 时间取事件的 Ts 字段（StrField("ts") 恒为空，上游 bug）：
	// createdAt 与 lastEventTs 必须非空。
	if dev["createdAt"] == "" {
		t.Errorf("createdAt 为空（Ts 字段未取到）")
	}
	if dev["lastEventTs"] != "2026-11-12T09:06:00+08:00" {
		t.Errorf("lastEventTs = %v", dev["lastEventTs"])
	}
	if dev["lastEventKind"] != "thread" {
		t.Errorf("lastEventKind = %v", dev["lastEventKind"])
	}
	if dev["task"] != "11-12-child" {
		t.Errorf("task = %v", dev["task"])
	}
	live := byName["live-channel"]
	if live["workersAlive"] != float64(1) {
		t.Errorf("live-channel workersAlive = %v，想要 1", live["workersAlive"])
	}
}

func TestChannelEvents(t *testing.T) {
	p := newTestProject(t)
	w := p.get(t, "/api/channels/dev-channel/events")
	var rows []map[string]any
	decodeJSON(t, w, &rows)
	if len(rows) != 7 {
		t.Fatalf("事件数 = %d，想要 7", len(rows))
	}
	first := rows[0]
	if first["seq"] != float64(1) || first["kind"] != "create" || first["by"] != "main" {
		t.Errorf("首事件 = %v", first)
	}
	if first["ts"] != "2026-11-12T09:00:00+08:00" {
		t.Errorf("首事件 ts = %v", first["ts"])
	}
	raw, ok := first["raw"].(string)
	if !ok || !strings.Contains(raw, `"kind":"create"`) {
		t.Errorf("raw 不是原始 JSON：%v", first["raw"])
	}
	extra, ok := first["extra"].(map[string]any)
	if !ok || extra["task"] != "11-12-child" {
		t.Errorf("extra = %v", first["extra"])
	}

	// since 增量拉取。
	w = p.get(t, "/api/channels/dev-channel/events?since=5")
	decodeJSON(t, w, &rows)
	if len(rows) != 2 {
		t.Fatalf("since=5 事件数 = %d，想要 2", len(rows))
	}
	// kind 过滤：progress 事件绝不能被丢。
	w = p.get(t, "/api/channels/dev-channel/events?kind=progress")
	decodeJSON(t, w, &rows)
	if len(rows) != 1 || rows[0]["kind"] != "progress" {
		t.Fatalf("kind=progress 结果 = %v", rows)
	}
	// from 过滤。
	w = p.get(t, "/api/channels/dev-channel/events?from=w1")
	decodeJSON(t, w, &rows)
	if len(rows) != 4 {
		t.Fatalf("from=w1 事件数 = %d，想要 4", len(rows))
	}
	// last 取尾部。
	w = p.get(t, "/api/channels/dev-channel/events?last=2")
	decodeJSON(t, w, &rows)
	if len(rows) != 2 || rows[0]["seq"] != float64(6) {
		t.Fatalf("last=2 结果 = %v", rows)
	}
	// 非法 kind 报 400。
	w = p.get(t, "/api/channels/dev-channel/events?kind=nope")
	if w.Code != http.StatusBadRequest {
		t.Errorf("非法 kind 状态码 = %d，想要 400", w.Code)
	}
	// 不存在的 channel 报 404。
	w = p.get(t, "/api/channels/nope/events")
	if w.Code != http.StatusNotFound {
		t.Errorf("不存在的 channel 状态码 = %d，想要 404", w.Code)
	}
}

func TestChannelWorkers(t *testing.T) {
	p := newTestProject(t)
	w := p.get(t, "/api/channels/dev-channel/workers")
	var rows []map[string]any
	decodeJSON(t, w, &rows)
	if len(rows) != 1 {
		t.Fatalf("worker 数 = %d，想要 1：%s", len(rows), w.Body.String())
	}
	worker := rows[0]
	if worker["workerId"] != "w1" {
		t.Errorf("workerId = %v", worker["workerId"])
	}
	if worker["lifecycle"] != "done" {
		t.Errorf("lifecycle = %v，想要 done（synthesized done 事件）", worker["lifecycle"])
	}
	if worker["terminal"] != true {
		t.Errorf("terminal = %v", worker["terminal"])
	}
	if worker["agent"] != "implement" || worker["provider"] != "claude" {
		t.Errorf("agent/provider = %v/%v", worker["agent"], worker["provider"])
	}
	if worker["activeTurnId"] != "" {
		t.Errorf("终态 worker 不应有 activeTurnId：%v", worker["activeTurnId"])
	}
	// 无 pid 文件时探活为 null，不编造。
	if worker["supervisorAlive"] != nil || worker["workerAlive"] != nil {
		t.Errorf("无 pid 文件时探活应为 null：%v / %v", worker["supervisorAlive"], worker["workerAlive"])
	}
	// 运行中 worker。
	w = p.get(t, "/api/channels/live-channel/workers")
	decodeJSON(t, w, &rows)
	if len(rows) != 1 || rows[0]["lifecycle"] != "running" {
		t.Fatalf("live-channel worker = %v", rows)
	}
}

func TestWorkerLog(t *testing.T) {
	p := newTestProject(t)
	w := p.get(t, "/api/channels/dev-channel/workers/w1/log?lines=2")
	var data map[string]any
	decodeJSON(t, w, &data)
	lines, ok := data["lines"].([]any)
	if !ok || len(lines) != 2 || lines[0] != "第二行" || lines[1] != "第三行" {
		t.Fatalf("日志尾部 = %v", data["lines"])
	}
	w = p.get(t, "/api/channels/dev-channel/workers/w1/log")
	decodeJSON(t, w, &data)
	if len(data["lines"].([]any)) != 3 {
		t.Fatalf("默认 100 行应返回全部 3 行：%v", data["lines"])
	}
	w = p.get(t, "/api/channels/dev-channel/workers/nobody/log")
	if w.Code != http.StatusNotFound {
		t.Errorf("不存在的 worker 状态码 = %d，想要 404", w.Code)
	}
}

func TestThreads(t *testing.T) {
	p := newTestProject(t)
	w := p.get(t, "/api/channels/dev-channel/threads")
	var rows []map[string]any
	decodeJSON(t, w, &rows)
	if len(rows) != 1 {
		t.Fatalf("thread 数 = %d，想要 1：%s", len(rows), w.Body.String())
	}
	if rows[0]["thread"] != "t1" || rows[0]["title"] != "主题一" {
		t.Errorf("thread = %v", rows[0])
	}
	w = p.get(t, "/api/channels/dev-channel/threads/t1")
	var data map[string]any
	decodeJSON(t, w, &data)
	events, ok := data["events"].([]any)
	if !ok || len(events) != 2 {
		t.Fatalf("thread 时间线 = %v", data)
	}
	w = p.get(t, "/api/channels/dev-channel/threads/nope")
	if w.Code != http.StatusNotFound {
		t.Errorf("不存在的 thread 状态码 = %d，想要 404", w.Code)
	}
}

func TestMem(t *testing.T) {
	p := newTestProject(t)
	w := p.get(t, "/api/mem/sessions")
	if w.Code != http.StatusOK {
		t.Fatalf("状态码 = %d，body=%s", w.Code, w.Body.String())
	}
	var sessions []any
	decodeJSON(t, w, &sessions)
	if len(sessions) != 0 {
		t.Fatalf("临时项目下不应有会话，得到 %d 条", len(sessions))
	}
	w = p.get(t, "/api/mem/sessions?platform=nope")
	if w.Code != http.StatusBadRequest {
		t.Errorf("非法 platform 状态码 = %d，想要 400", w.Code)
	}
	w = p.get(t, "/api/mem/projects")
	var projects []any
	decodeJSON(t, w, &projects)
	if len(projects) != 0 {
		t.Fatalf("临时项目下项目汇总应为空，得到 %v", projects)
	}
}

func TestHealth(t *testing.T) {
	p := newTestProject(t)
	w := p.get(t, "/api/health")
	if w.Code != http.StatusOK {
		t.Fatalf("状态码 = %d，body=%s", w.Code, w.Body.String())
	}
	var data map[string]any
	decodeJSON(t, w, &data)
	for _, key := range []string{"new_files", "unchanged_files", "auto_update_files",
		"changed_files", "user_deleted_files", "orphan_keys", "pending_migrations", "version"} {
		if _, ok := data[key]; !ok {
			t.Errorf("健康数据缺字段 %s", key)
		}
	}
	version, ok := data["version"].(map[string]any)
	if !ok || version["project_version"] != "0.3.11" || version["cli_version"] != "test" {
		t.Errorf("version = %v", data["version"])
	}
}

func TestVersion(t *testing.T) {
	p := newTestProject(t)
	w := p.get(t, "/api/version")
	var data map[string]any
	decodeJSON(t, w, &data)
	if data["project_version"] != "0.3.11" || data["cli_version"] != "test" {
		t.Errorf("version = %v", data)
	}
}

func TestDesign(t *testing.T) {
	p := newTestProject(t)
	w := p.get(t, "/api/design")
	var data map[string]any
	decodeJSON(t, w, &data)
	if data["note"] != "启发式计数，仅供参考" {
		t.Errorf("note = %v", data["note"])
	}
	req, ok := data["requirements"].(map[string]any)
	if !ok || req["exists"] != true || req["lines"] != float64(3) {
		t.Errorf("requirements = %v", data["requirements"])
	}
	proto, ok := data["prototype"].(map[string]any)
	if !ok || proto["exists"] != true {
		t.Errorf("prototype = %v", data["prototype"])
	}
}

func TestJournal(t *testing.T) {
	p := newTestProject(t)
	w := p.get(t, "/api/journal")
	var data map[string]any
	decodeJSON(t, w, &data)
	files, ok := data["files"].([]any)
	if !ok || len(files) != 1 {
		t.Fatalf("journal files = %v", data["files"])
	}
	if !strings.Contains(data["content"].(string), "做了测试任务") {
		t.Errorf("默认应返回最新 journal 文本：%v", data["content"])
	}
	w = p.get(t, "/api/journal?file=journal-1.md")
	decodeJSON(t, w, &data)
	if !strings.Contains(data["content"].(string), "做了测试任务") {
		t.Errorf("按文件取 journal 失败：%v", data["content"])
	}
}

func TestActivityMtime(t *testing.T) {
	p := newTestProject(t)
	// 启动后无变化：活动流里只有 channel 事件，无 mtime 条目。
	w := p.get(t, "/api/activity")
	var data map[string]any
	decodeJSON(t, w, &data)
	entries := data["entries"].([]any)
	for _, e := range entries {
		if e.(map[string]any)["source"] == "mtime" {
			t.Fatalf("启动后尚未变化，不应有 mtime 条目：%v", e)
		}
	}
	if len(entries) == 0 {
		t.Fatalf("channel 事件应进活动流")
	}
	// 改 task.json 的 mtime → 记一条 file_change。
	taskJSON := filepath.Join(p.root, ".sunshine", "tasks", "11-12-child", "task.json")
	now := time.Now().Add(2 * time.Second)
	if err := os.Chtimes(taskJSON, now, now); err != nil {
		t.Fatal(err)
	}
	w = p.get(t, "/api/activity")
	decodeJSON(t, w, &data)
	entries = data["entries"].([]any)
	found := false
	for _, e := range entries {
		m := e.(map[string]any)
		if m["source"] == "mtime" && strings.Contains(m["text"].(string), "task.json 已更新") {
			found = true
		}
	}
	if !found {
		t.Fatalf("改 mtime 后活动流无对应条目：%v", entries)
	}
	// 新任务目录出现 → 记一条。
	writeTask(t, filepath.Join(p.root, ".sunshine", "tasks", "11-12-new"), func(r *task.TaskRecord) {
		r.Title = "新任务"
	})
	w = p.get(t, "/api/activity")
	decodeJSON(t, w, &data)
	entries = data["entries"].([]any)
	found = false
	for _, e := range entries {
		m := e.(map[string]any)
		if m["source"] == "mtime" && strings.Contains(m["text"].(string), "11-12-new") {
			found = true
		}
	}
	if !found {
		t.Fatalf("新任务目录出现后活动流无对应条目：%v", entries)
	}
}

func TestRunPortOccupied(t *testing.T) {
	p := newTestProject(t)
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	port := ln.Addr().(*net.TCPAddr).Port
	err = p.server.Run("127.0.0.1", port)
	if err == nil {
		t.Fatal("端口被占时应报错")
	}
	if !strings.Contains(err.Error(), "已被占用") {
		t.Errorf("报错应提示端口被占，得到：%v", err)
	}
}

func TestRunRealServer(t *testing.T) {
	p := newTestProject(t)
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := ln.Addr().(*net.TCPAddr).Port
	ln.Close()

	done := make(chan error, 1)
	go func() { done <- p.server.Run("127.0.0.1", port) }()
	// 等服务起来。
	var resp *http.Response
	for i := 0; i < 50; i++ {
		time.Sleep(50 * time.Millisecond)
		r, err := http.Get(fmt.Sprintf("http://127.0.0.1:%d/api/version", port))
		if err == nil {
			resp = r
			break
		}
	}
	if resp == nil {
		t.Fatal("服务未在 2.5 秒内起来")
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("真实服务 /api/version 状态码 = %d", resp.StatusCode)
	}
}
