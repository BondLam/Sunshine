// Package cli 的 channel 命令测试（阶段 6 C 组）。
//
// 用 fake 后端 + stub WorkerRuntime 验证 CLI 全套；
// fake 后端把事件以 JSONL 落到temp SUNSHINE_CHANNEL_ROOT 下
// （布局与 PLAN.md 存储一节一致：<root>/<projectKey>/<channel>/events.jsonl），
// 但它只是测试夹具——生产后端由 B 组合并时接线。
package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/spf13/cobra"
)

// ---------------------------------------------------------------------------
// 测试根命令与执行 helper
// ---------------------------------------------------------------------------

func testChannelRoot() *cobra.Command {
	root := &cobra.Command{
		Use:           "sunshine",
		SilenceErrors: true,
		SilenceUsage:  true,
	}
	RegisterChannelCommand(root)
	return root
}

// runChannelCmd 执行一次 channel 命令并返回标准输出。
func runChannelCmd(t *testing.T, args ...string) (string, error) {
	t.Helper()
	root := testChannelRoot()
	root.SetArgs(args)
	return captureStdout(t, func() error { return root.Execute() })
}

// channelTestEnv 是每个测试的 fake 环境。
type channelTestEnv struct {
	fake *fakeChannelBackend
	stub *stubWorkerRuntime
	root string
	proj string
}

// setupChannelTest 搭一个 fake 后端 + stub runtime 的测试环境：
// temp SUNSHINE_CHANNEL_ROOT、temp工作目录、后端与 runtime 注入。
func setupChannelTest(t *testing.T) *channelTestEnv {
	t.Helper()
	root := t.TempDir()
	proj := t.TempDir()
	t.Setenv("SUNSHINE_CHANNEL_ROOT", root)
	t.Chdir(proj)
	fake := newFakeChannelBackend(root)
	oldBackend := newChannelBackend
	newChannelBackend = func() channelBackend { return fake }
	t.Cleanup(func() { newChannelBackend = oldBackend })
	stub := &stubWorkerRuntime{interruptResult: true}
	oldRuntime := channelWorkerRuntime
	channelWorkerRuntime = stub
	t.Cleanup(func() { channelWorkerRuntime = oldRuntime })
	return &channelTestEnv{fake: fake, stub: stub, root: root, proj: proj}
}

// setChannelStdin temp替换 channel 的 stdin 来源。
func setChannelStdin(t *testing.T, s string) {
	t.Helper()
	old := channelStdin
	channelStdin = strings.NewReader(s)
	t.Cleanup(func() { channelStdin = old })
}

// ---------------------------------------------------------------------------
// fake 后端：内存事件存储 + JSONL 落盘（仅测试用）
// ---------------------------------------------------------------------------

type fakeWorkerState struct {
	id          string
	provider    string
	agent       string
	lifecycle   string // starting|running|done|error|killed|crashed
	activity    string
	inboxPolicy string
	startedBy   string
	pid         int
	live        bool
}

type fakeChannelState struct {
	ref         ChannelRef
	events      []ChannelEvent
	workers     map[string]*fakeWorkerState
	ctxEntries  []ChannelContextEntry
	threadCtx   map[string][]ChannelContextEntry
	title       string
	ephemeral   bool
	chType      string
	description string
	task        string
}

type fakeChannelBackend struct {
	mu       sync.Mutex
	root     string
	channels map[string]*fakeChannelState
}

func newFakeChannelBackend(root string) *fakeChannelBackend {
	return &fakeChannelBackend{root: root, channels: map[string]*fakeChannelState{}}
}

// fakeProjectKey 按 Claude Code 约定由 cwd 生成项目桶名。
func fakeProjectKey(cwd string) string {
	var b strings.Builder
	for _, r := range cwd {
		switch {
		case r == '/' || r == '\\' || r == '_':
			b.WriteByte('-')
		case r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '-' || r == '.':
			b.WriteRune(r)
		default:
			b.WriteByte('-')
		}
	}
	if b.Len() == 0 {
		return "-"
	}
	return b.String()
}

func (f *fakeChannelBackend) projectKey(scope, cwdOverride, projectOverride string) string {
	if scope == ScopeGlobal {
		return "_global"
	}
	if projectOverride != "" {
		return projectOverride
	}
	cwd := cwdOverride
	if cwd == "" {
		cwd, _ = os.Getwd()
	}
	return fakeProjectKey(cwd)
}

func (f *fakeChannelBackend) key(project, name string) string { return project + "\x00" + name }

// appendEventLocked 在锁内追加事件：分配 seq、写时间戳、落盘 JSONL。
func (f *fakeChannelBackend) appendEventLocked(st *fakeChannelState, ev ChannelEvent) ChannelEvent {
	ev["seq"] = int64(len(st.events) + 1)
	ev["ts"] = time.Now().UTC().Format(time.RFC3339Nano)
	ev["origin"] = "cli"
	st.events = append(st.events, ev)
	dir := st.ref.Dir
	_ = os.MkdirAll(dir, 0o755)
	line, _ := json.Marshal(ev)
	_ = os.WriteFile(filepath.Join(dir, "events.jsonl"),
		append(readFakeFile(filepath.Join(dir, "events.jsonl")), line...), 0o644)
	return ev
}

func readFakeFile(path string) []byte {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	if len(b) > 0 && b[len(b)-1] != '\n' {
		b = append(b, '\n')
	}
	return b
}

// resolveLocked 按名字与 scope 找 channel（锁内调用）。
func (f *fakeChannelBackend) resolveLocked(name, scope string) (*fakeChannelState, error) {
	// 先在当前项目桶与全局桶里找（scope 为空时两边都认）。
	candidates := []string{}
	if scope == ScopeGlobal {
		candidates = []string{"_global"}
	} else if scope == ScopeProject {
		cwd, _ := os.Getwd()
		candidates = []string{fakeProjectKey(cwd)}
	} else {
		cwd, _ := os.Getwd()
		candidates = []string{fakeProjectKey(cwd), "_global"}
	}
	for _, project := range candidates {
		if st, ok := f.channels[f.key(project, name)]; ok {
			return st, nil
		}
	}
	return nil, fmt.Errorf("channel '%s' 不存在", name)
}

func (f *fakeChannelBackend) CreateChannel(_ context.Context, opts CreateChannelOptions) (ChannelRef, ChannelEvent, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	project := f.projectKey(opts.Scope, opts.Cwd, opts.Project)
	k := f.key(project, opts.Name)
	if _, ok := f.channels[k]; ok && !opts.Force {
		return ChannelRef{}, nil, fmt.Errorf("channel '%s' 已存在（用 --force 覆盖）", opts.Name)
	}
	scope := opts.Scope
	if scope == "" {
		scope = ScopeProject
	}
	chType := opts.Type
	if chType == "" {
		chType = ChannelTypeChat
	}
	cwd := opts.Cwd
	if cwd == "" {
		cwd, _ = os.Getwd()
	}
	ref := ChannelRef{Name: opts.Name, Scope: scope, Project: project, Dir: filepath.Join(f.root, project, opts.Name)}
	st := &fakeChannelState{
		ref: ref, workers: map[string]*fakeWorkerState{},
		threadCtx: map[string][]ChannelContextEntry{},
		ephemeral: opts.Ephemeral, chType: chType,
		description: opts.Description, task: opts.Task,
	}
	f.channels[k] = st
	ev := ChannelEvent{
		"kind": "create", "by": opts.By, "type": chType,
		"task": opts.Task, "description": opts.Description,
		"labels": opts.Labels, "cwd": cwd, "ephemeral": opts.Ephemeral,
	}
	if len(opts.Context) > 0 {
		ev["context"] = opts.Context
	}
	if opts.Origin != "" {
		ev["meta"] = map[string]any{"sunshine": map[string]any{"createMode": opts.Origin}}
	}
	return ref, f.appendEventLocked(st, ev), nil
}

func (f *fakeChannelBackend) SendMessage(_ context.Context, opts SendMessageOptions) (ChannelEvent, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	st, err := f.resolveLocked(opts.Channel, opts.Scope)
	if err != nil {
		return nil, err
	}
	ev := ChannelEvent{"kind": "message", "by": opts.By, "text": opts.Text}
	if len(opts.To) == 1 {
		ev["to"] = opts.To[0]
	} else if len(opts.To) > 1 {
		ev["to"] = opts.To
	}
	ev = f.appendEventLocked(st, ev)
	if opts.DeliveryMode == "requireKnownWorker" || opts.DeliveryMode == "requireRunningWorker" {
		for _, target := range opts.To {
			w, known := st.workers[target]
			reason := ""
			if !known {
				reason = "worker-unknown"
			} else if opts.DeliveryMode == "requireRunningWorker" && !w.live {
				reason = "worker-terminal"
			}
			if reason != "" {
				f.appendEventLocked(st, ChannelEvent{
					"kind": "undeliverable", "by": "cli",
					"to": target, "reason": reason,
				})
			}
		}
	}
	return ev, nil
}

func fakeReadFilter(ev ChannelEvent, opts ReadEventsOptions) bool {
	if opts.Kind != "" && ev.Kind() != opts.Kind {
		return false
	}
	if len(opts.From) > 0 {
		matched := false
		for _, from := range opts.From {
			if ev.By() == from {
				matched = true
				break
			}
		}
		if !matched {
			return false
		}
	}
	if opts.Thread != "" {
		if ev.Kind() != "thread" || evString(ev, "thread") != opts.Thread {
			return false
		}
	}
	if opts.Action != "" {
		if ev.Kind() != "thread" || evString(ev, "action") != opts.Action {
			return false
		}
	}
	if !opts.IncludeProgress && ev.Kind() == "progress" {
		return false
	}
	if !opts.IncludeNonMeaning && !meaningfulEventKinds[ev.Kind()] {
		return false
	}
	if opts.To != "" {
		targets := evToList(ev)
		if len(targets) > 0 {
			matched := false
			for _, t := range targets {
				if t == opts.To {
					matched = true
					break
				}
			}
			if !matched {
				return false
			}
		}
	}
	return true
}

func (f *fakeChannelBackend) ReadEvents(_ context.Context, ref ChannelRef, opts ReadEventsOptions) ([]ChannelEvent, error) {
	if opts.AfterSeq != nil && opts.BeforeSeq != nil {
		return nil, fmt.Errorf("afterSeq 与 beforeSeq 不可同传")
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	st, err := f.resolveLocked(ref.Name, ref.Scope)
	if err != nil {
		return nil, err
	}
	out := []ChannelEvent{}
	for _, ev := range st.events {
		if opts.AfterSeq != nil && ev.Seq() <= *opts.AfterSeq {
			continue
		}
		if opts.BeforeSeq != nil && ev.Seq() >= *opts.BeforeSeq {
			continue
		}
		if !fakeReadFilter(ev, opts) {
			continue
		}
		out = append(out, ev)
	}
	limit := opts.Limit
	if limit > 0 && len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}

func (f *fakeChannelBackend) WatchEvents(ctx context.Context, ref ChannelRef, filter WaitFilter) (<-chan ChannelEvent, error) {
	f.mu.Lock()
	st, err := f.resolveLocked(ref.Name, ref.Scope)
	if err != nil {
		f.mu.Unlock()
		return nil, err
	}
	startSeq := int64(len(st.events))
	f.mu.Unlock()
	out := make(chan ChannelEvent, 16)
	go func() {
		defer close(out)
		last := startSeq
		ticker := time.NewTicker(15 * time.Millisecond)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				f.mu.Lock()
				st, err := f.resolveLocked(ref.Name, ref.Scope)
				if err != nil {
					f.mu.Unlock()
					return
				}
				var fresh []ChannelEvent
				for _, ev := range st.events {
					if ev.Seq() > last && channelEventMatchesWait(ev, filter) {
						fresh = append(fresh, ev)
						last = ev.Seq()
					} else if ev.Seq() > last {
						last = ev.Seq()
					}
				}
				f.mu.Unlock()
				for _, ev := range fresh {
					select {
					case out <- ev:
					case <-ctx.Done():
						return
					}
				}
			}
		}
	}()
	return out, nil
}

func (f *fakeChannelBackend) RequestInterrupt(_ context.Context, opts InterruptOptions) (ChannelEvent, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	st, err := f.resolveLocked(opts.Channel, opts.Scope)
	if err != nil {
		return nil, err
	}
	return f.appendEventLocked(st, ChannelEvent{
		"kind": "interrupt_requested", "by": opts.By,
		"worker": opts.Worker, "message": opts.Message, "reason": opts.Reason,
	}), nil
}

func (f *fakeChannelBackend) InterruptWorker(ctx context.Context, opts InterruptOptions, _ WorkerRuntime) (InterruptResult, error) {
	ev, err := f.RequestInterrupt(ctx, opts)
	if err != nil {
		return InterruptResult{}, err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	st, err := f.resolveLocked(opts.Channel, opts.Scope)
	if err != nil {
		return InterruptResult{}, err
	}
	w, ok := st.workers[opts.Worker]
	interrupted := ok && w.live
	delivery := "appended"
	if interrupted {
		delivery = "delivered"
	}
	return InterruptResult{Event: ev, Interrupted: interrupted, Delivery: delivery}, nil
}

func (f *fakeChannelBackend) SpawnWorker(_ context.Context, opts SpawnWorkerOptions, runtime WorkerRuntime) (SpawnResult, error) {
	f.mu.Lock()
	st, err := f.resolveLocked(opts.Channel, opts.Scope)
	if err != nil {
		f.mu.Unlock()
		return SpawnResult{}, err
	}
	if w, ok := st.workers[opts.As]; ok && w.live {
		f.mu.Unlock()
		return SpawnResult{}, fmt.Errorf("worker '%s' 已在 channel '%s' 中运行", opts.As, opts.Channel)
	}
	// 存活上限检查（跨同项目桶的所有 channel）。
	if opts.MaxLiveWorkers > 0 {
		live := 0
		for _, other := range f.channels {
			if other.ref.Project != st.ref.Project {
				continue
			}
			for _, w := range other.workers {
				if w.live {
					live++
				}
			}
		}
		if live >= opts.MaxLiveWorkers {
			f.mu.Unlock()
			return SpawnResult{}, fmt.Errorf("存活 worker 已达上限（%d/%d），先等空闲 worker 回收或调大 --max-live-workers", live, opts.MaxLiveWorkers)
		}
	}
	f.mu.Unlock()
	pid := 0
	if runtime != nil {
		res, err := runtime.Start(context.Background(), WorkerStartOptions{
			Channel: opts.Channel, Worker: opts.As, Provider: opts.Provider,
			Cwd: opts.Cwd, SystemPrompt: opts.SystemPrompt, Model: opts.Model,
			Resume: opts.Resume, Sandbox: opts.Sandbox,
			Timeout: opts.Timeout, WarnBefore: opts.WarnBefore, IdleTimeout: opts.IdleTimeout,
		})
		if err != nil {
			return SpawnResult{}, fmt.Errorf("拉起 worker 失败：%v", err)
		}
		pid = res.PID
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	st, err = f.resolveLocked(opts.Channel, opts.Scope)
	if err != nil {
		return SpawnResult{}, err
	}
	st.workers[opts.As] = &fakeWorkerState{
		id: opts.As, provider: opts.Provider, agent: opts.Agent,
		lifecycle: "running", activity: "idle",
		inboxPolicy: opts.InboxPolicy, startedBy: opts.By, pid: pid, live: true,
	}
	ev := ChannelEvent{
		"kind": "spawned", "by": opts.By, "worker": opts.As,
		"provider": opts.Provider, "agent": opts.Agent, "pid": pid,
	}
	if len(opts.Files) > 0 {
		ev["files"] = opts.Files
	}
	if len(opts.JSONLManifests) > 0 {
		ev["manifests"] = opts.JSONLManifests
	}
	f.appendEventLocked(st, ev)
	return SpawnResult{PID: pid, Log: filepath.Join(st.ref.Dir, opts.As+".log"), Worker: opts.As}, nil
}

func (f *fakeChannelBackend) ListChannels(_ context.Context, opts ListChannelsOptions) ([]ChannelSummary, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	cwd, _ := os.Getwd()
	currentProject := fakeProjectKey(cwd)
	rows := []ChannelSummary{}
	for _, st := range f.channels {
		if opts.Scope == ScopeGlobal && st.ref.Project != "_global" {
			continue
		}
		if opts.Scope == ScopeProject && st.ref.Project != currentProject {
			continue
		}
		if !opts.AllProjects && opts.Scope == "" && st.ref.Project != currentProject && st.ref.Project != "_global" {
			// 未指定 scope 且未要求全部项目时，只看当前项目桶与全局桶。
			if st.ref.Project != currentProject {
				continue
			}
		}
		if opts.Project != "" && !strings.Contains(st.task, opts.Project) {
			continue
		}
		if st.ephemeral && !opts.All {
			continue
		}
		alive, total := 0, 0
		for _, w := range st.workers {
			total++
			if w.live {
				alive++
			}
		}
		var last ChannelEvent
		if len(st.events) > 0 {
			last = st.events[len(st.events)-1]
		}
		desc := st.title
		if desc == "" {
			desc = st.description
		}
		summary := ChannelSummary{
			Name: st.ref.Name, Project: st.ref.Project, Type: st.chType,
			Description: desc, Task: st.task, WorkersAlive: alive,
			WorkersTotal: total, TotalEvents: len(st.events),
			Ephemeral: st.ephemeral,
		}
		if last != nil {
			summary.LastEventTs = last["ts"].(string)
			summary.LastEventKind = last.Kind()
		}
		if len(st.events) > 0 {
			summary.CreatedAt = st.events[0]["ts"].(string)
		}
		rows = append(rows, summary)
	}
	// 最近活动倒序。
	for i := 0; i < len(rows); i++ {
		for j := i + 1; j < len(rows); j++ {
			if rows[j].LastEventTs > rows[i].LastEventTs {
				rows[i], rows[j] = rows[j], rows[i]
			}
		}
	}
	return rows, nil
}

func (f *fakeChannelBackend) ListWorkers(_ context.Context, ref ChannelRef) ([]ChannelWorkerState, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	st, err := f.resolveLocked(ref.Name, ref.Scope)
	if err != nil {
		return nil, err
	}
	out := []ChannelWorkerState{}
	for _, w := range st.workers {
		out = append(out, ChannelWorkerState{
			WorkerID: w.id, Channel: ref.Name, Agent: w.agent,
			Provider: w.provider, Lifecycle: w.lifecycle, Activity: w.activity,
			StartedBy: w.startedBy, InboxPolicy: w.inboxPolicy,
		})
	}
	return out, nil
}

func (f *fakeChannelBackend) ListPruneCandidates(_ context.Context, opts PruneOptions) ([]PruneCandidate, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	cwd, _ := os.Getwd()
	currentProject := fakeProjectKey(cwd)
	out := []PruneCandidate{}
	for _, st := range f.channels {
		switch opts.Scope {
		case ScopeGlobal:
			if st.ref.Project != "_global" {
				continue
			}
		case ScopeProject:
			if st.ref.Project != currentProject {
				continue
			}
		}
		keep := false
		for _, k := range opts.Keep {
			if k == st.ref.Name {
				keep = true
			}
		}
		if keep {
			continue
		}
		live := false
		for _, w := range st.workers {
			if w.live {
				live = true
			}
		}
		if live {
			continue
		}
		var lastTs string
		if len(st.events) > 0 {
			lastTs, _ = st.events[len(st.events)-1]["ts"].(string)
		}
		var reason string
		switch {
		case opts.Ephemeral:
			if st.ephemeral {
				reason = "ephemeral"
			}
		case opts.All:
			reason = "all"
		case opts.Empty:
			if len(st.events) <= 1 {
				reason = "empty"
			}
		case opts.Idle > 0:
			if lastTs != "" {
				if ts, err := time.Parse(time.RFC3339Nano, lastTs); err == nil {
					if time.Since(ts) >= opts.Idle {
						reason = fmt.Sprintf("idle %dm", int(time.Since(ts).Minutes()))
					}
				}
			}
		}
		if reason != "" {
			out = append(out, PruneCandidate{Name: st.ref.Name, Project: st.ref.Project, Reason: reason, LastTs: lastTs})
		}
	}
	return out, nil
}

func (f *fakeChannelBackend) RemoveChannel(_ context.Context, ref ChannelRef) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	st, err := f.resolveLocked(ref.Name, ref.Scope)
	if err != nil {
		// prune 按 project 精确删：按 ref.Project 兜底。
		if ref.Project != "" {
			if st2, ok := f.channels[f.key(ref.Project, ref.Name)]; ok {
				st = st2
			} else {
				return err
			}
		} else {
			return err
		}
	}
	for _, w := range st.workers {
		w.live = false
		w.lifecycle = "killed"
	}
	_ = os.RemoveAll(st.ref.Dir)
	delete(f.channels, f.key(st.ref.Project, st.ref.Name))
	return nil
}

func (f *fakeChannelBackend) KillWorker(_ context.Context, ref ChannelRef, worker string, force bool) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	st, err := f.resolveLocked(ref.Name, ref.Scope)
	if err != nil {
		return err
	}
	w, ok := st.workers[worker]
	if !ok {
		return fmt.Errorf("worker '%s' 未在 channel '%s' 中运行", worker, ref.Name)
	}
	signal := "SIGTERM"
	if force {
		signal = "SIGKILL"
	}
	if !w.live {
		f.appendEventLocked(st, ChannelEvent{
			"kind": "error", "by": "cli:kill", "worker": worker,
			"message": "supervisor 已丢失",
		})
		return nil
	}
	w.live = false
	w.lifecycle = "killed"
	f.appendEventLocked(st, ChannelEvent{
		"kind": "killed", "by": "cli:kill", "worker": worker,
		"reason": "explicit-kill", "signal": signal,
	})
	return nil
}

func contextEntriesEqual(a, b ChannelContextEntry) bool {
	return a.Type == b.Type && a.Path == b.Path && a.Text == b.Text
}

func (f *fakeChannelBackend) AddChannelContext(_ context.Context, ref ChannelRef, by string, entries []ChannelContextEntry) (ChannelEvent, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	st, err := f.resolveLocked(ref.Name, ref.Scope)
	if err != nil {
		return nil, err
	}
	for _, e := range entries {
		found := false
		for _, cur := range st.ctxEntries {
			if contextEntriesEqual(cur, e) {
				found = true
			}
		}
		if !found {
			st.ctxEntries = append(st.ctxEntries, e)
		}
	}
	return f.appendEventLocked(st, ChannelEvent{"kind": "context", "by": by, "context": entries}), nil
}

func (f *fakeChannelBackend) DeleteChannelContext(_ context.Context, ref ChannelRef, by string, entries []ChannelContextEntry) (ChannelEvent, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	st, err := f.resolveLocked(ref.Name, ref.Scope)
	if err != nil {
		return nil, err
	}
	kept := st.ctxEntries[:0]
	for _, cur := range st.ctxEntries {
		drop := false
		for _, e := range entries {
			if contextEntriesEqual(cur, e) {
				drop = true
			}
		}
		if !drop {
			kept = append(kept, cur)
		}
	}
	st.ctxEntries = kept
	return f.appendEventLocked(st, ChannelEvent{"kind": "context", "by": by, "context": entries}), nil
}

func (f *fakeChannelBackend) ListChannelContext(_ context.Context, ref ChannelRef) ([]ChannelContextEntry, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	st, err := f.resolveLocked(ref.Name, ref.Scope)
	if err != nil {
		return nil, err
	}
	out := append([]ChannelContextEntry{}, st.ctxEntries...)
	return out, nil
}

func (f *fakeChannelBackend) AddThreadContext(_ context.Context, ref ChannelRef, thread string, by string, entries []ChannelContextEntry) (ChannelEvent, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	st, err := f.resolveLocked(ref.Name, ref.Scope)
	if err != nil {
		return nil, err
	}
	cur := st.threadCtx[thread]
	for _, e := range entries {
		found := false
		for _, c := range cur {
			if contextEntriesEqual(c, e) {
				found = true
			}
		}
		if !found {
			cur = append(cur, e)
		}
	}
	st.threadCtx[thread] = cur
	return f.appendEventLocked(st, ChannelEvent{"kind": "context", "by": by, "thread": thread, "context": entries}), nil
}

func (f *fakeChannelBackend) DeleteThreadContext(_ context.Context, ref ChannelRef, thread string, by string, entries []ChannelContextEntry) (ChannelEvent, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	st, err := f.resolveLocked(ref.Name, ref.Scope)
	if err != nil {
		return nil, err
	}
	kept := []ChannelContextEntry{}
	for _, cur := range st.threadCtx[thread] {
		drop := false
		for _, e := range entries {
			if contextEntriesEqual(cur, e) {
				drop = true
			}
		}
		if !drop {
			kept = append(kept, cur)
		}
	}
	st.threadCtx[thread] = kept
	return f.appendEventLocked(st, ChannelEvent{"kind": "context", "by": by, "thread": thread, "context": entries}), nil
}

func (f *fakeChannelBackend) ListThreadContext(_ context.Context, ref ChannelRef, thread string) ([]ChannelContextEntry, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	st, err := f.resolveLocked(ref.Name, ref.Scope)
	if err != nil {
		return nil, err
	}
	return append([]ChannelContextEntry{}, st.threadCtx[thread]...), nil
}

func (f *fakeChannelBackend) SetChannelTitle(_ context.Context, ref ChannelRef, by string, title string) (ChannelEvent, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	st, err := f.resolveLocked(ref.Name, ref.Scope)
	if err != nil {
		return nil, err
	}
	st.title = title
	return f.appendEventLocked(st, ChannelEvent{"kind": "channel", "by": by, "title": title}), nil
}

func (f *fakeChannelBackend) ClearChannelTitle(_ context.Context, ref ChannelRef, by string) (ChannelEvent, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	st, err := f.resolveLocked(ref.Name, ref.Scope)
	if err != nil {
		return nil, err
	}
	st.title = ""
	return f.appendEventLocked(st, ChannelEvent{"kind": "channel", "by": by, "title": ""}), nil
}

// SimulateWorkerReply 模拟一个 worker：等一条发给 worker 的消息，
// 然后回一条消息并记 done（供 run / wait 测试用）。
func (f *fakeChannelBackend) SimulateWorkerReply(channel, worker, reply string) {
	go func() {
		deadline := time.Now().Add(10 * time.Second)
		for time.Now().Before(deadline) {
			f.mu.Lock()
			st, err := f.resolveLocked(channel, "")
			var found bool
			if err == nil {
				for _, ev := range st.events {
					if ev.Kind() == "message" {
						for _, t := range evToList(ev) {
							if t == worker {
								found = true
							}
						}
					}
				}
			}
			if found {
				f.appendEventLocked(st, ChannelEvent{"kind": "message", "by": worker, "text": reply})
				f.appendEventLocked(st, ChannelEvent{"kind": "done", "by": worker})
				if w, ok := st.workers[worker]; ok {
					w.lifecycle = "done"
					w.activity = "idle"
				}
				f.mu.Unlock()
				return
			}
			f.mu.Unlock()
			time.Sleep(10 * time.Millisecond)
		}
	}()
}

// ---------------------------------------------------------------------------
// stub WorkerRuntime
// ---------------------------------------------------------------------------

type stubWorkerRuntime struct {
	mu              sync.Mutex
	started         []WorkerStartOptions
	onStart         func(WorkerStartOptions)
	interruptResult bool
}

func (s *stubWorkerRuntime) Start(_ context.Context, opts WorkerStartOptions) (WorkerStartResult, error) {
	s.mu.Lock()
	s.started = append(s.started, opts)
	onStart := s.onStart
	s.mu.Unlock()
	if onStart != nil {
		onStart(opts)
	}
	return WorkerStartResult{WorkerID: opts.Worker, Provider: opts.Provider, PID: 4242, StartedAt: time.Now().UTC().Format(time.RFC3339)}, nil
}

func (s *stubWorkerRuntime) Interrupt(_ context.Context, _ string) (bool, error) {
	return s.interruptResult, nil
}

func (s *stubWorkerRuntime) Stop(_ context.Context, _ string) error { return nil }

// ---------------------------------------------------------------------------
// 测试
// ---------------------------------------------------------------------------

func TestChannelCreateSendListMessages(t *testing.T) {
	env := setupChannelTest(t)
	_ = env
	out, err := runChannelCmd(t, "channel", "create", "review", "--type", "chat", "--task", "11-09-xxx")
	if err != nil {
		t.Fatalf("create 失败：%v", err)
	}
	if !strings.Contains(out, "已创建 channel 'review'") {
		t.Fatalf("create 输出不对：%q", out)
	}
	// 事件落盘到temp SUNSHINE_CHANNEL_ROOT。
	matches, _ := filepath.Glob(filepath.Join(env.root, "*", "review", "events.jsonl"))
	if len(matches) != 1 {
		t.Fatalf("events.jsonl 未落到 SUNSHINE_CHANNEL_ROOT 下：%v", matches)
	}
	out, err = runChannelCmd(t, "channel", "send", "review", "--by", "main", "开始实现任务")
	if err != nil {
		t.Fatalf("send 失败：%v", err)
	}
	var ev ChannelEvent
	if err := json.Unmarshal([]byte(strings.TrimSpace(out)), &ev); err != nil {
		t.Fatalf("send 输出不是 JSON：%q：%v", out, err)
	}
	if ev.Kind() != "message" || ev.Text() != "开始实现任务" {
		t.Fatalf("send 事件不对：%v", ev)
	}
	out, err = runChannelCmd(t, "channel", "list")
	if err != nil {
		t.Fatalf("list 失败：%v", err)
	}
	if !strings.Contains(out, "review") {
		t.Fatalf("list 未列出 channel：%q", out)
	}
	out, err = runChannelCmd(t, "channel", "messages", "review")
	if err != nil {
		t.Fatalf("messages 失败：%v", err)
	}
	if !strings.Contains(out, "开始实现任务") || !strings.Contains(out, "[message]") {
		t.Fatalf("messages 输出不对：%q", out)
	}
	// temp channel 默认隐藏，--all 显示。
	if _, err := runChannelCmd(t, "channel", "create", "temp", "--ephemeral"); err != nil {
		t.Fatalf("create temp失败：%v", err)
	}
	out, _ = runChannelCmd(t, "channel", "list")
	if strings.Contains(out, "temp") {
		t.Fatalf("list 默认应隐藏temp channel：%q", out)
	}
	out, _ = runChannelCmd(t, "channel", "list", "--all")
	if !strings.Contains(out, "temp") {
		t.Fatalf("list --all 应显示temp channel：%q", out)
	}
	// list --json。
	out, err = runChannelCmd(t, "channel", "list", "--json", "--all")
	if err != nil {
		t.Fatalf("list --json 失败：%v", err)
	}
	var rows []ChannelSummary
	if err := json.Unmarshal([]byte(out), &rows); err != nil {
		t.Fatalf("list --json 不是 JSON：%q：%v", out, err)
	}
	if len(rows) != 2 {
		t.Fatalf("list --json 应有 2 行，实际 %d", len(rows))
	}
}

// TestChannelNameValidation：channel 名只允许字母、数字、.、_ 和 -，
// 与上游 assertSafeName / B 组 store 一致（中文名不可用）。
func TestChannelNameValidation(t *testing.T) {
	setupChannelTest(t)
	for _, bad := range []string{"评审", "a b", "a/b", "..", "a..b/c"} {
		if _, err := runChannelCmd(t, "channel", "create", bad); err == nil {
			t.Fatalf("channel 名 %q 应被拒绝", bad)
		} else if !strings.Contains(err.Error(), "只允许字母、数字") {
			t.Fatalf("channel 名 %q 报错不对：%v", bad, err)
		}
	}
	if _, err := runChannelCmd(t, "channel", "create", "review-1"); err != nil {
		t.Fatalf("合法名 review-1 应通过：%v", err)
	}
}

func TestChannelCreateDuplicateAndForce(t *testing.T) {
	setupChannelTest(t)
	if _, err := runChannelCmd(t, "channel", "create", "review"); err != nil {
		t.Fatalf("首次 create 失败：%v", err)
	}
	if _, err := runChannelCmd(t, "channel", "create", "review"); err == nil {
		t.Fatalf("重复 create 未加 --force 应报错")
	} else if !strings.Contains(err.Error(), "已存在") {
		t.Fatalf("重复 create 报错不对：%v", err)
	}
	if _, err := runChannelCmd(t, "channel", "create", "review", "--force"); err != nil {
		t.Fatalf("create --force 失败：%v", err)
	}
}

func TestChannelSendValidation(t *testing.T) {
	setupChannelTest(t)
	if _, err := runChannelCmd(t, "channel", "create", "review"); err != nil {
		t.Fatalf("create 失败：%v", err)
	}
	if _, err := runChannelCmd(t, "channel", "send", "review", "--by", "main"); err == nil {
		t.Fatalf("无正文的 send 应报错")
	} else if !strings.Contains(err.Error(), "未提供正文") {
		t.Fatalf("无正文报错不对：%v", err)
	}
	if _, err := runChannelCmd(t, "channel", "send", "review", "--by", "main", "--delivery-mode", "nope", "你好"); err == nil {
		t.Fatalf("非法 delivery-mode 应报错")
	}
	if _, err := runChannelCmd(t, "channel", "send", "review", "--by", "main", "--scope", "nope", "你好"); err == nil {
		t.Fatalf("非法 scope 应报错")
	}
	if _, err := runChannelCmd(t, "channel", "send", "不存在", "--by", "main", "你好"); err == nil {
		t.Fatalf("不存在的 channel 的 send 应报错")
	}
	// --stdin 与 --text-file。
	setChannelStdin(t, "stdin 正文\n")
	out, err := runChannelCmd(t, "channel", "send", "review", "--by", "main", "--stdin")
	if err != nil {
		t.Fatalf("send --stdin 失败：%v", err)
	}
	if !strings.Contains(out, "stdin 正文") {
		t.Fatalf("send --stdin 正文不对：%q", out)
	}
	tmp := filepath.Join(t.TempDir(), "正文.txt")
	if err := os.WriteFile(tmp, []byte("文件正文"), 0o644); err != nil {
		t.Fatal(err)
	}
	out, err = runChannelCmd(t, "channel", "send", "review", "--by", "main", "--text-file", tmp)
	if err != nil {
		t.Fatalf("send --text-file 失败：%v", err)
	}
	if !strings.Contains(out, "文件正文") {
		t.Fatalf("send --text-file 正文不对：%q", out)
	}
}

func TestChannelWaitMatchAndTimeout(t *testing.T) {
	env := setupChannelTest(t)
	if _, err := runChannelCmd(t, "channel", "create", "review"); err != nil {
		t.Fatalf("create 失败：%v", err)
	}
	// 100ms 后 worker 发消息；wait 应匹配到。
	// 直接调 fake 后端发消息（不用 runChannelCmd，避免并发 captureStdout 抢 os.Stdout）。
	go func() {
		time.Sleep(100 * time.Millisecond)
		_, _ = env.fake.SendMessage(context.Background(), SendMessageOptions{Channel: "review", By: "impl", Text: "做完了"})
	}()
	out, err := runChannelCmd(t, "channel", "wait", "review", "--from", "impl", "--timeout", "5s")
	if err != nil {
		t.Fatalf("wait 匹配失败：%v", err)
	}
	if !strings.Contains(out, "做完了") {
		t.Fatalf("wait 输出应含匹配事件：%q", out)
	}
	// 超时：100ms 内无匹配事件。
	if _, err := runChannelCmd(t, "channel", "wait", "review", "--from", "没人", "--timeout", "100ms"); err == nil {
		t.Fatalf("wait 超时应报错")
	} else if !strings.Contains(err.Error(), "超时") {
		t.Fatalf("wait 超时报错不对：%v", err)
	}
	// 旧事件不重放：wait 之前已有的 done 不应立刻唤醒。
	if _, err := runChannelCmd(t, "channel", "send", "review", "--by", "impl", "旧消息"); err != nil {
		t.Fatalf("send 失败：%v", err)
	}
	if _, err := runChannelCmd(t, "channel", "wait", "review", "--from", "impl", "--timeout", "100ms"); err == nil {
		t.Fatalf("wait 应从当前时刻起看事件，旧事件不应唤醒")
	}
}

func TestChannelWaitAll(t *testing.T) {
	env := setupChannelTest(t)
	if _, err := runChannelCmd(t, "channel", "create", "review"); err != nil {
		t.Fatalf("create 失败：%v", err)
	}
	go func() {
		time.Sleep(50 * time.Millisecond)
		_, _ = env.fake.SendMessage(context.Background(), SendMessageOptions{Channel: "review", By: "甲", Text: "甲好了"})
		time.Sleep(50 * time.Millisecond)
		_, _ = env.fake.SendMessage(context.Background(), SendMessageOptions{Channel: "review", By: "乙", Text: "乙好了"})
	}()
	out, err := runChannelCmd(t, "channel", "wait", "review", "--from", "甲,乙", "--all", "--timeout", "5s")
	if err != nil {
		t.Fatalf("wait --all 失败：%v", err)
	}
	if !strings.Contains(out, "甲好了") || !strings.Contains(out, "乙好了") {
		t.Fatalf("wait --all 应打印两个匹配事件：%q", out)
	}
	if _, err := runChannelCmd(t, "channel", "wait", "review", "--all", "--timeout", "1s"); err == nil {
		t.Fatalf("--all 不带 --from 应报错")
	}
}

func TestChannelPruneDryRunAndYes(t *testing.T) {
	setupChannelTest(t)
	if _, err := runChannelCmd(t, "channel", "create", "empty-ch"); err != nil {
		t.Fatalf("create 失败：%v", err)
	}
	if _, err := runChannelCmd(t, "channel", "create", "active"); err != nil {
		t.Fatalf("create 失败：%v", err)
	}
	if _, err := runChannelCmd(t, "channel", "send", "active", "--by", "main", "有消息"); err != nil {
		t.Fatalf("send 失败：%v", err)
	}
	if _, err := runChannelCmd(t, "channel", "create", "ephemeral-ch", "--ephemeral"); err != nil {
		t.Fatalf("create 失败：%v", err)
	}
	// --empty 演练：只列出empty-ch，不真删。
	out, err := runChannelCmd(t, "channel", "prune", "--empty")
	if err != nil {
		t.Fatalf("prune 演练失败：%v", err)
	}
	if !strings.Contains(out, "empty-ch") || !strings.Contains(out, "演练") {
		t.Fatalf("prune 演练输出不对：%q", out)
	}
	if strings.Contains(out, "active") {
		t.Fatalf("prune --empty 不应列出active动的 channel：%q", out)
	}
	out, _ = runChannelCmd(t, "channel", "list", "--all")
	if !strings.Contains(out, "empty-ch") {
		t.Fatalf("演练后 channel 应还在：%q", out)
	}
	// 不带 --yes 也不删。
	out, err = runChannelCmd(t, "channel", "prune", "--empty", "--dry-run=false")
	if err != nil {
		t.Fatalf("prune 无 --yes 失败：%v", err)
	}
	if !strings.Contains(out, "拒绝删除") {
		t.Fatalf("无 --yes 应拒绝删除：%q", out)
	}
	out, _ = runChannelCmd(t, "channel", "list", "--all")
	if !strings.Contains(out, "empty-ch") {
		t.Fatalf("无 --yes 时 channel 应还在")
	}
	// --yes 真删：empty-ch与ephemeral-ch都只有 create 事件，都算 empty。
	out, err = runChannelCmd(t, "channel", "prune", "--empty", "--yes")
	if err != nil {
		t.Fatalf("prune --yes 失败：%v", err)
	}
	if !strings.Contains(out, "已删除 2 个 channel") {
		t.Fatalf("prune --yes 输出不对：%q", out)
	}
	out, _ = runChannelCmd(t, "channel", "list", "--all")
	if strings.Contains(out, "empty-ch") || strings.Contains(out, "ephemeral-ch") {
		t.Fatalf("prune --yes 后空 channel 应已删除：%q", out)
	}
	// 再建一个temp channel，--ephemeral 清理它。
	if _, err := runChannelCmd(t, "channel", "create", "ephemeral-2", "--ephemeral"); err != nil {
		t.Fatalf("create 失败：%v", err)
	}
	out, err = runChannelCmd(t, "channel", "prune", "--ephemeral", "--yes")
	if err != nil {
		t.Fatalf("prune --ephemeral 失败：%v", err)
	}
	if !strings.Contains(out, "已删除 1 个") {
		t.Fatalf("prune --ephemeral 输出不对：%q", out)
	}
	// 条件互斥。
	if _, err := runChannelCmd(t, "channel", "prune", "--all", "--empty"); err == nil {
		t.Fatalf("prune 条件互斥应报错")
	} else if !strings.Contains(err.Error(), "互斥") {
		t.Fatalf("互斥报错不对：%v", err)
	}
}

func TestChannelRm(t *testing.T) {
	setupChannelTest(t)
	if _, err := runChannelCmd(t, "channel", "create", "review"); err != nil {
		t.Fatalf("create 失败：%v", err)
	}
	if _, err := runChannelCmd(t, "channel", "send", "review", "--by", "main", "你好"); err != nil {
		t.Fatalf("send 失败：%v", err)
	}
	out, err := runChannelCmd(t, "channel", "rm", "review")
	if err != nil {
		t.Fatalf("rm 失败：%v", err)
	}
	if !strings.Contains(out, "已删除 channel 'review'") {
		t.Fatalf("rm 输出不对：%q", out)
	}
	out, _ = runChannelCmd(t, "channel", "list")
	if strings.Contains(out, "review") {
		t.Fatalf("rm 后 list 不应再有该 channel：%q", out)
	}
	if _, err := runChannelCmd(t, "channel", "messages", "review"); err == nil {
		t.Fatalf("rm 后 messages 应报错")
	}
	if _, err := runChannelCmd(t, "channel", "rm", "review"); err == nil {
		t.Fatalf("重复 rm 应报错")
	}
}

func TestChannelContextAndTitle(t *testing.T) {
	env := setupChannelTest(t)
	if _, err := runChannelCmd(t, "channel", "create", "review"); err != nil {
		t.Fatalf("create 失败：%v", err)
	}
	tmp := filepath.Join(env.proj, "需求.md")
	if err := os.WriteFile(tmp, []byte("需求"), 0o644); err != nil {
		t.Fatal(err)
	}
	out, err := runChannelCmd(t, "channel", "context", "add", "review", "--file", tmp, "--raw", "只改退款逻辑")
	if err != nil {
		t.Fatalf("context add 失败：%v", err)
	}
	if !strings.Contains(out, `"kind":"context"`) {
		t.Fatalf("context add 输出应为 context 事件 JSON：%q", out)
	}
	out, err = runChannelCmd(t, "channel", "context", "list", "review")
	if err != nil {
		t.Fatalf("context list 失败：%v", err)
	}
	if !strings.Contains(out, "file "+tmp) || !strings.Contains(out, "只改退款逻辑") {
		t.Fatalf("context list 输出不对：%q", out)
	}
	// thread 级上下文。
	if _, err := runChannelCmd(t, "channel", "context", "add", "review", "--thread", "需求-1", "--raw", "补充验收"); err != nil {
		t.Fatalf("context add --thread 失败：%v", err)
	}
	out, err = runChannelCmd(t, "channel", "context", "list", "review", "--thread", "需求-1")
	if err != nil {
		t.Fatalf("context list --thread 失败：%v", err)
	}
	if !strings.Contains(out, "补充验收") {
		t.Fatalf("thread 上下文输出不对：%q", out)
	}
	out, err = runChannelCmd(t, "channel", "context", "list", "review")
	if err != nil {
		t.Fatalf("context list 失败：%v", err)
	}
	if strings.Contains(out, "补充验收") {
		t.Fatalf("channel 级 list 不应含 thread 级条目：%q", out)
	}
	// 删除 raw 条目。
	if _, err := runChannelCmd(t, "channel", "context", "delete", "review", "--raw", "只改退款逻辑"); err != nil {
		t.Fatalf("context delete 失败：%v", err)
	}
	out, _ = runChannelCmd(t, "channel", "context", "list", "review")
	if strings.Contains(out, "只改退款逻辑") {
		t.Fatalf("delete 后条目应已删除：%q", out)
	}
	if !strings.Contains(out, "file "+tmp) {
		t.Fatalf("delete 不应误删 file 条目：%q", out)
	}
	// 无条目时报错；相对路径 file 报错。
	if _, err := runChannelCmd(t, "channel", "context", "add", "review"); err == nil {
		t.Fatalf("无条目 add 应报错")
	}
	if _, err := runChannelCmd(t, "channel", "context", "add", "review", "--file", "相对/路径.md"); err == nil {
		t.Fatalf("相对路径 file 应报错")
	}
	// title set / clear。
	out, err = runChannelCmd(t, "channel", "title", "set", "review", "--title", "退款原因重构")
	if err != nil {
		t.Fatalf("title set 失败：%v", err)
	}
	if !strings.Contains(out, "退款原因重构") {
		t.Fatalf("title set 输出不对：%q", out)
	}
	out, err = runChannelCmd(t, "channel", "list", "--json")
	if err != nil {
		t.Fatalf("list --json 失败：%v", err)
	}
	var rows []ChannelSummary
	if err := json.Unmarshal([]byte(out), &rows); err != nil {
		t.Fatalf("list --json 解析失败：%v", err)
	}
	if len(rows) != 1 || rows[0].Description != "退款原因重构" {
		t.Fatalf("title 应反映到 list 的描述里：%v", rows)
	}
	if _, err := runChannelCmd(t, "channel", "title", "clear", "review"); err != nil {
		t.Fatalf("title clear 失败：%v", err)
	}
	out, _ = runChannelCmd(t, "channel", "list", "--json")
	// 用新变量接解析：Unmarshal 会复用已有 slice 元素、缺失字段保留旧值。
	var rowsAfterClear []ChannelSummary
	_ = json.Unmarshal([]byte(out), &rowsAfterClear)
	if len(rowsAfterClear) == 1 && rowsAfterClear[0].Description == "退款原因重构" {
		t.Fatalf("title clear 后描述应已清除")
	}
}

func TestChannelSpawnAndKill(t *testing.T) {
	env := setupChannelTest(t)
	if _, err := runChannelCmd(t, "channel", "create", "review"); err != nil {
		t.Fatalf("create 失败：%v", err)
	}
	// 缺 provider 应报错。
	if _, err := runChannelCmd(t, "channel", "spawn", "review", "--as", "worker1"); err == nil {
		t.Fatalf("缺 provider 的 spawn 应报错")
	} else if !strings.Contains(err.Error(), "--provider") {
		t.Fatalf("缺 provider 报错不对：%v", err)
	}
	// 非法 provider / sandbox 报错。
	if _, err := runChannelCmd(t, "channel", "spawn", "review", "--provider", "nope", "--as", "worker1"); err == nil {
		t.Fatalf("非法 provider 应报错")
	}
	if _, err := runChannelCmd(t, "channel", "spawn", "review", "--provider", "claude", "--as", "worker1", "--sandbox", "read-only"); err == nil {
		t.Fatalf("claude 配 --sandbox 应报错")
	}
	// stub runtime 拉起成功，输出 JSON 含 worker 名。
	out, err := runChannelCmd(t, "channel", "spawn", "review", "--provider", "claude", "--as", "worker1", "--inbox-policy", "broadcastAndExplicit")
	if err != nil {
		t.Fatalf("spawn 失败：%v", err)
	}
	var res SpawnResult
	if err := json.Unmarshal([]byte(strings.TrimSpace(out)), &res); err != nil {
		t.Fatalf("spawn 输出不是 JSON：%q：%v", out, err)
	}
	if res.Worker != "worker1" || res.PID != 4242 {
		t.Fatalf("spawn 结果不对：%+v", res)
	}
	// stub 收到的 Start 参数里 system prompt 含 channel 协议。
	if len(env.stub.started) != 1 {
		t.Fatalf("stub runtime 应被调用 1 次，实际 %d", len(env.stub.started))
	}
	if !strings.Contains(env.stub.started[0].SystemPrompt, "SUNSHINE CHANNEL") {
		t.Fatalf("system prompt 应含 Sunshine channel 协议：%q", env.stub.started[0].SystemPrompt)
	}
	// 重复 spawn 同名 worker 应报错。
	if _, err := runChannelCmd(t, "channel", "spawn", "review", "--provider", "claude", "--as", "worker1"); err == nil {
		t.Fatalf("重复 spawn 同名 worker 应报错")
	}
	// interrupt：打断并发送替换指令。
	out, err = runChannelCmd(t, "channel", "interrupt", "review", "--to", "worker1", "停下，先改别处")
	if err != nil {
		t.Fatalf("interrupt 失败：%v", err)
	}
	if !strings.Contains(out, "interrupt_requested") {
		t.Fatalf("interrupt 输出应为 interrupt_requested 事件：%q", out)
	}
	if _, err := runChannelCmd(t, "channel", "interrupt", "review", "停下"); err == nil {
		t.Fatalf("interrupt 不带 --to 应报错")
	}
	// kill。
	out, err = runChannelCmd(t, "channel", "kill", "review", "--as", "worker1")
	if err != nil {
		t.Fatalf("kill 失败：%v", err)
	}
	if !strings.Contains(out, "已停止 worker 'worker1'") {
		t.Fatalf("kill 输出不对：%q", out)
	}
	out, _ = runChannelCmd(t, "channel", "messages", "review", "--kind", "killed", "--raw")
	if !strings.Contains(out, `"kind":"killed"`) {
		t.Fatalf("kill 后应有 killed 事件：%q", out)
	}
	if _, err := runChannelCmd(t, "channel", "kill", "review", "--as", "不存在"); err == nil {
		t.Fatalf("kill 不存在的 worker 应报错")
	}
}

func TestChannelSpawnWithAgentAndFiles(t *testing.T) {
	env := setupChannelTest(t)
	// 写一个 agent 定义。
	agentDir := filepath.Join(env.proj, ".sunshine", "agents")
	if err := os.MkdirAll(agentDir, 0o755); err != nil {
		t.Fatal(err)
	}
	agentBody := "---\nprovider: claude\nmodel: opus\nname: impl\n---\n\n你是impl，按规范写代码。\n"
	if err := os.WriteFile(filepath.Join(agentDir, "implement.md"), []byte(agentBody), 0o644); err != nil {
		t.Fatal(err)
	}
	ctxFile := filepath.Join(env.proj, "上下文.txt")
	if err := os.WriteFile(ctxFile, []byte("上下文内容"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := runChannelCmd(t, "channel", "create", "review"); err != nil {
		t.Fatalf("create 失败：%v", err)
	}
	out, err := runChannelCmd(t, "channel", "spawn", "review", "--agent", "implement", "--file", ctxFile)
	if err != nil {
		t.Fatalf("spawn --agent 失败：%v", err)
	}
	var spawnRes SpawnResult
	if err := json.Unmarshal([]byte(strings.TrimSpace(out)), &spawnRes); err != nil {
		t.Fatalf("spawn 输出不是 JSON：%q：%v", out, err)
	}
	if spawnRes.Worker != "impl" {
		t.Fatalf("spawn 应以 agent 名为 worker 名：%+v", spawnRes)
	}
	if len(env.stub.started) != 1 {
		t.Fatalf("stub 应被调用 1 次")
	}
	got := env.stub.started[0]
	if got.Provider != "claude" || got.Model != "opus" {
		t.Fatalf("agent 的 provider/model 应作为默认值：%+v", got)
	}
	if !strings.Contains(got.SystemPrompt, "你是impl") || !strings.Contains(got.SystemPrompt, "上下文内容") {
		t.Fatalf("system prompt 应含 agent 正文与上下文文件：%q", got.SystemPrompt)
	}
	// agent 不存在时报错。
	if _, err := runChannelCmd(t, "channel", "spawn", "review", "--agent", "不存在", "--as", "x"); err == nil {
		t.Fatalf("agent 不存在时 spawn 应报错")
	}
}

func TestChannelRunOnce(t *testing.T) {
	env := setupChannelTest(t)
	// stub 启动时模拟 worker：收到 prompt 后回答案并记 done。
	env.stub.onStart = func(opts WorkerStartOptions) {
		env.fake.SimulateWorkerReply("run-xxx", opts.Worker, "答案：已完成")
	}
	// run 的 channel 名固定，便于断言清理。
	out, err := runChannelCmd(t, "channel", "run", "run-xxx", "--provider", "claude", "--as", "worker1", "--message", "请回答", "--timeout", "30s")
	if err != nil {
		t.Fatalf("run 失败：%v", err)
	}
	if !strings.Contains(out, "答案：已完成") {
		t.Fatalf("run 应打印 worker 最终答案：%q", out)
	}
	// 成功后temp channel 已清理。
	out, _ = runChannelCmd(t, "channel", "list", "--all")
	if strings.Contains(out, "run-xxx") {
		t.Fatalf("run 成功后 channel 应已清理：%q", out)
	}
	if _, err := os.Stat(filepath.Join(env.root, "run-xxx")); !os.IsNotExist(err) {
		// 目录可能在别的项目桶下；用 list 为准，此处只作宽松检查。
		_ = err
	}
	// run 无 prompt 应报错。
	if _, err := runChannelCmd(t, "channel", "run", "--provider", "claude", "--as", "worker1"); err == nil {
		t.Fatalf("run 无 prompt 应报错")
	}
}

func TestChannelMessagesFilters(t *testing.T) {
	setupChannelTest(t)
	if _, err := runChannelCmd(t, "channel", "create", "review"); err != nil {
		t.Fatalf("create 失败：%v", err)
	}
	if _, err := runChannelCmd(t, "channel", "send", "review", "--by", "甲", "甲的消息"); err != nil {
		t.Fatalf("send 失败：%v", err)
	}
	if _, err := runChannelCmd(t, "channel", "send", "review", "--by", "乙", "乙的消息"); err != nil {
		t.Fatalf("send 失败：%v", err)
	}
	out, err := runChannelCmd(t, "channel", "messages", "review", "--from", "甲")
	if err != nil {
		t.Fatalf("messages --from 失败：%v", err)
	}
	if !strings.Contains(out, "甲的消息") || strings.Contains(out, "乙的消息") {
		t.Fatalf("messages --from 过滤不对：%q", out)
	}
	out, err = runChannelCmd(t, "channel", "messages", "review", "--last", "1", "--raw")
	if err != nil {
		t.Fatalf("messages --last 失败：%v", err)
	}
	if !strings.Contains(out, "乙的消息") || strings.Contains(out, "甲的消息") {
		t.Fatalf("messages --last 1 应只含最后一条：%q", out)
	}
	// chat channel 的 --thread 应报错。
	if _, err := runChannelCmd(t, "channel", "messages", "review", "--thread", "需求-1"); err == nil {
		t.Fatalf("chat channel 的 --thread 应报错")
	}
	// 非法 kind 报错。
	if _, err := runChannelCmd(t, "channel", "messages", "review", "--kind", "nope"); err == nil {
		t.Fatalf("非法 kind 应报错")
	}
}

func TestChannelHelpChinese(t *testing.T) {
	setupChannelTest(t)
	out, err := runChannelCmd(t, "channel", "--help")
	if err != nil {
		t.Fatalf("channel --help 失败：%v", err)
	}
	for _, want := range []string{"多 Agent 协同", "create", "send", "wait", "spawn", "run", "prune", "context", "title"} {
		if !strings.Contains(out, want) {
			t.Fatalf("channel 帮助缺 %q：\n%s", want, out)
		}
	}
	if strings.Contains(strings.ToLower(out), "trellis") {
		t.Fatalf("channel 帮助不应含 trellis 字样：\n%s", out)
	}
	out, err = runChannelCmd(t, "channel", "create", "--help")
	if err != nil {
		t.Fatalf("create --help 失败：%v", err)
	}
	if !strings.Contains(out, "新建一个 channel") {
		t.Fatalf("create 帮助应为中文：\n%s", out)
	}
	// 内部命令入口存在（已接 D 组实现：配置/文件不存在时报真实错误）。
	if _, err := runChannelCmd(t, "channel", "__supervisor", "a", "b", "/不存在/配置.json"); err == nil {
		t.Fatalf("__supervisor 读不到配置应报错")
	} else if !strings.Contains(err.Error(), "supervisor 配置") {
		t.Fatalf("__supervisor 报错不对：%v", err)
	}
	if _, err := runChannelCmd(t, "channel", "__parse-trace", "nope", "x"); err == nil {
		t.Fatalf("__parse-trace 非法适配器应报错")
	}
	if _, err := runChannelCmd(t, "channel", "__parse-trace", "claude", "/不存在/trace.jsonl"); err == nil {
		t.Fatalf("__parse-trace 读不到文件应报错")
	} else if !strings.Contains(err.Error(), "trace") {
		t.Fatalf("__parse-trace 报错不对：%v", err)
	}
}

func TestParseDurationFlagDays(t *testing.T) {
	cases := map[string]time.Duration{
		"7d":  7 * 24 * time.Hour,
		"1d":  24 * time.Hour,
		"30s": 30 * time.Second,
		"2m":  2 * time.Minute,
		"90":  90 * time.Second,
		"":    0,
	}
	for in, want := range cases {
		got, err := parseDurationFlag(in)
		if err != nil {
			t.Errorf("parseDurationFlag(%q) 报错：%v", in, err)
			continue
		}
		if got != want {
			t.Errorf("parseDurationFlag(%q) = %v，想要 %v", in, got, want)
		}
	}
	if _, err := parseDurationFlag("7x"); err == nil {
		t.Error("parseDurationFlag(\"7x\") 应报错")
	}
}
