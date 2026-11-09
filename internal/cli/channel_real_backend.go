// Package cli 的真实 channel 后端：把 channelBackend 接口接到
// B 组 internal/channel API 包与 A 组 internal/channel/store 上。
//
// 接线说明（阶段 6 C 组，B/D 组落地后接线）：
//   - create/send/read/watch/workers/interrupt/context/title 直接调
//     B 组 API（函数名与 PLAN.md 规格一致）；
//   - list/rm/kill/prune 是上游 CLI 层（rm.ts/prune.ts/list.ts/kill.ts）
//     的行为，B 组 API 未提供，CLI 层用 store 原语组合实现；
//   - spawn 默认（未注入 runtime）走 D 组 supervisor.SpawnWorker
//     （守卫 → 写配置 → 派生 supervisor 子进程，与上游 spawn.ts 一致）；
//     注入 runtime 时走 B 组 SpawnWorker（测试与 API 消费方用）。
package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/linrong1982/Sunshine/internal/channel"
	"github.com/linrong1982/Sunshine/internal/channel/store"
	"github.com/linrong1982/Sunshine/internal/channel/supervisor"
)

// channelSupervisorBinary 是 spawn 派生 supervisor 子进程用的二进制路径；
// 为空时用当前进程的可执行文件（即 sunshine 自身）。
// 测试可指向模拟脚本。
var channelSupervisorBinary string

// realChannelBackend 是 channelBackend 的真实实现。
type realChannelBackend struct{}

// toChannelAddress 把 CLI 的 ChannelRef 转成 B 组寻址参数。
func toChannelAddress(ref ChannelRef) channel.ChannelAddress {
	return channel.ChannelAddress{
		Channel:    ref.Name,
		Scope:      ref.Scope,
		ProjectKey: ref.Project,
	}
}

// fromAPIEvent 把 B 组事件转成 CLI 的 map 事件（JSON 往返，不丢字段）。
func fromAPIEvent(ev channel.ChannelEvent) (ChannelEvent, error) {
	data, err := json.Marshal(ev)
	if err != nil {
		return nil, fmt.Errorf("序列化事件失败：%v", err)
	}
	var out ChannelEvent
	if err := json.Unmarshal(data, &out); err != nil {
		return nil, fmt.Errorf("解析事件失败：%v", err)
	}
	return out, nil
}

// fromAPIEvents 批量转换。
func fromAPIEvents(evs []channel.ChannelEvent) ([]ChannelEvent, error) {
	out := make([]ChannelEvent, 0, len(evs))
	for _, ev := range evs {
		m, err := fromAPIEvent(ev)
		if err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, nil
}

// toAPIContextEntries 把 CLI 上下文条目转成 B 组条目。
func toAPIContextEntries(entries []ChannelContextEntry) []channel.ContextEntry {
	out := make([]channel.ContextEntry, 0, len(entries))
	for _, e := range entries {
		out = append(out, channel.ContextEntry{Type: e.Type, Path: e.Path, Text: e.Text})
	}
	return out
}

// fromAPIContextEntries 把 B 组上下文条目转回 CLI 条目。
func fromAPIContextEntries(entries []channel.ContextEntry) []ChannelContextEntry {
	out := make([]ChannelContextEntry, 0, len(entries))
	for _, e := range entries {
		out = append(out, ChannelContextEntry{Type: e.Type, Path: e.Path, Text: e.Text})
	}
	return out
}

// resolveRealRef 解析 channel 引用（建 channel 用 forCreate=true）。
func resolveRealRef(ref ChannelRef, forCreate bool) (store.ChannelRef, error) {
	addr := toChannelAddress(ref)
	if forCreate {
		return channel.ResolveChannelForCreate(addr)
	}
	return channel.ResolveChannel(addr)
}

// --- create / send ---

func (realChannelBackend) CreateChannel(_ context.Context, opts CreateChannelOptions) (ChannelRef, ChannelEvent, error) {
	ref, err := resolveRealRef(ChannelRef{Name: opts.Name, Scope: opts.Scope, Project: opts.Project}, true)
	if err != nil {
		return ChannelRef{}, nil, err
	}
	meta := map[string]any(nil)
	if opts.Origin != "" {
		meta = map[string]any{"sunshine": map[string]any{"createMode": opts.Origin}}
	}
	ev, err := channel.CreateChannel(channel.CreateChannelOptions{
		ChannelAddress: channel.ChannelAddress{
			Channel: opts.Name, Scope: opts.Scope,
			ProjectKey: opts.Project, CWD: opts.Cwd,
		},
		By: opts.By, Origin: channel.OriginCLI, Meta: meta,
		Type: opts.Type, Task: opts.Task, Project: opts.Project,
		Labels: opts.Labels, Description: opts.Description,
		Context:   toAPIContextEntries(opts.Context),
		Ephemeral: opts.Ephemeral, Force: opts.Force,
	})
	if err != nil {
		return ChannelRef{}, nil, err
	}
	m, err := fromAPIEvent(ev)
	if err != nil {
		return ChannelRef{}, nil, err
	}
	return ChannelRef{Name: ref.Name, Scope: string(ref.Scope), Project: ref.Project, Dir: ref.Dir}, m, nil
}

func (realChannelBackend) SendMessage(_ context.Context, opts SendMessageOptions) (ChannelEvent, error) {
	ev, err := channel.SendMessage(channel.SendMessageOptions{
		ChannelAddress: channel.ChannelAddress{Channel: opts.Channel, Scope: opts.Scope},
		By:             opts.By, Origin: channel.OriginCLI,
		Text: opts.Text, To: opts.To, DeliveryMode: opts.DeliveryMode,
	})
	if err != nil {
		return nil, err
	}
	return fromAPIEvent(ev)
}

// --- read / watch ---

// filterStoreEvents 用 store 的过滤器筛事件（与 B 组 watch 同款语义）。
func filterStoreEvents(evs []store.ChannelEvent, filter store.ChannelEventFilter) []store.ChannelEvent {
	out := make([]store.ChannelEvent, 0, len(evs))
	for _, ev := range evs {
		if store.MatchesEventFilter(ev, filter) {
			out = append(out, ev)
		}
	}
	return out
}

// storeEventsToMapEvents 把 store 事件转成 CLI 的 map 事件。
func storeEventsToMapEvents(evs []store.ChannelEvent) ([]ChannelEvent, error) {
	out := make([]ChannelEvent, 0, len(evs))
	for _, ev := range evs {
		data, err := json.Marshal(ev)
		if err != nil {
			return nil, err
		}
		var m ChannelEvent
		if err := json.Unmarshal(data, &m); err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, nil
}

func (realChannelBackend) ReadEvents(_ context.Context, ref ChannelRef, opts ReadEventsOptions) ([]ChannelEvent, error) {
	if opts.AfterSeq != nil && opts.BeforeSeq != nil {
		return nil, fmt.Errorf("afterSeq 与 beforeSeq 不可同传")
	}
	storeRef, err := resolveRealRef(ref, false)
	if err != nil {
		return nil, err
	}
	// 不分页读全部事件再过滤（channel 规模小；--last 要取尾部，
	// 分页读头 200 条会丢尾，与上游 readChannelEvents 读全部再筛一致）。
	storeEvents, err := store.ReadChannelEvents(storeRef, nil)
	if err != nil {
		return nil, err
	}
	filter := store.ChannelEventFilter{
		From: opts.From, To: opts.To, Thread: opts.Thread,
		Action:               store.ThreadAction(opts.Action),
		IncludeProgress:      opts.IncludeProgress,
		IncludeNonMeaningful: opts.IncludeNonMeaning,
	}
	if opts.Kind != "" {
		filter.Kind = []store.ChannelEventKind{store.ChannelEventKind(opts.Kind)}
	}
	filtered := filterStoreEvents(storeEvents, filter)
	// seq 范围过滤。
	kept := make([]store.ChannelEvent, 0, len(filtered))
	for _, ev := range filtered {
		if opts.AfterSeq != nil && ev.Seq <= *opts.AfterSeq {
			continue
		}
		if opts.BeforeSeq != nil && ev.Seq >= *opts.BeforeSeq {
			continue
		}
		kept = append(kept, ev)
	}
	if opts.Limit > 0 && len(kept) > opts.Limit {
		kept = kept[:opts.Limit]
	}
	return storeEventsToMapEvents(kept)
}

func (realChannelBackend) WatchEvents(ctx context.Context, ref ChannelRef, filter WaitFilter) (<-chan ChannelEvent, error) {
	// 先同步解析引用，channel 不存在时立刻报错
	//（B 组 WatchEvents 在 goroutine 里解析，失败只进 err 通道，
	// CLI 的 wait 会误报“未收到匹配事件”）。
	if _, err := resolveRealRef(ref, false); err != nil {
		return nil, err
	}
	to := filter.To
	if to == "" {
		to = filter.Self
	}
	storeFilter := store.ChannelEventFilter{
		Self: filter.Self, From: filter.From, To: to,
		Thread: filter.Thread, Action: store.ThreadAction(filter.Action),
		IncludeProgress: filter.IncludeProgress,
	}
	for _, k := range filter.Kinds {
		storeFilter.Kind = append(storeFilter.Kind, store.ChannelEventKind(k))
	}
	evCh, errCh := channel.WatchEvents(ctx, toChannelAddress(ref), channel.WatchEventsOptions{
		Filter: storeFilter, FromStart: false,
	})
	out := make(chan ChannelEvent, 16)
	go func() {
		defer close(out)
		for {
			select {
			case <-ctx.Done():
				return
			case err, ok := <-errCh:
				if ok && err != nil {
					fmt.Fprintf(os.Stderr, "警告：监听 channel 事件失败：%v\n", err)
				}
				return
			case ev, ok := <-evCh:
				if !ok {
					return
				}
				m, err := fromAPIEvent(ev)
				if err != nil {
					continue
				}
				select {
				case out <- m:
				case <-ctx.Done():
					return
				}
			}
		}
	}()
	return out, nil
}

// --- interrupt ---

func (realChannelBackend) RequestInterrupt(_ context.Context, opts InterruptOptions) (ChannelEvent, error) {
	ev, err := channel.RequestInterrupt(channel.InterruptWorkerOptions{
		ChannelAddress: channel.ChannelAddress{Channel: opts.Channel, Scope: opts.Scope},
		By:             opts.By, Origin: channel.OriginCLI,
		WorkerID: opts.Worker, Message: opts.Message, Reason: channel.InterruptReason(opts.Reason),
	})
	if err != nil {
		return nil, err
	}
	return fromAPIEvent(ev)
}

func (realChannelBackend) InterruptWorker(ctx context.Context, opts InterruptOptions, runtime WorkerRuntime) (InterruptResult, error) {
	var bRuntime channel.WorkerRuntime
	if runtime != nil {
		bRuntime = &channelRuntimeAdapter{inner: runtime}
	}
	res, err := channel.InterruptWorker(ctx, channel.InterruptWorkerOptions{
		ChannelAddress: channel.ChannelAddress{Channel: opts.Channel, Scope: opts.Scope},
		By:             opts.By, Origin: channel.OriginCLI,
		WorkerID: opts.Worker, Message: opts.Message, Reason: channel.InterruptReason(opts.Reason),
	}, bRuntime)
	if err != nil {
		return InterruptResult{}, err
	}
	ev, err := fromAPIEvent(res.Event)
	if err != nil {
		return InterruptResult{}, err
	}
	return InterruptResult{Event: ev, Interrupted: res.Interrupted, Delivery: string(res.Delivery)}, nil
}

// channelRuntimeAdapter 把 CLI 的 WorkerRuntime 接口适配成
// B 组 channel.WorkerRuntime（Start/Interrupt/Stop，workerID 入参）。
type channelRuntimeAdapter struct {
	inner     WorkerRuntime
	lastStart WorkerStartResult
}

func (a *channelRuntimeAdapter) Start(ctx context.Context, spec channel.WorkerSpec) (channel.WorkerStart, error) {
	res, err := a.inner.Start(ctx, WorkerStartOptions{
		Channel: spec.Channel.Name, Worker: spec.WorkerID,
		Provider: spec.Provider, Cwd: spec.CWD,
		SystemPrompt: spec.SystemPrompt, Model: spec.Model,
		Resume: spec.Resume, Sandbox: spec.Sandbox,
	})
	if err != nil {
		return channel.WorkerStart{}, err
	}
	a.lastStart = res
	return channel.WorkerStart{
		WorkerID: res.WorkerID, Provider: res.Provider,
		PID: res.PID, StartedAt: res.StartedAt,
	}, nil
}

func (a *channelRuntimeAdapter) Interrupt(ctx context.Context, workerID string) error {
	ok, err := a.inner.Interrupt(ctx, workerID)
	if err != nil {
		return err
	}
	if !ok {
		return fmt.Errorf("worker %s 未响应中断", workerID)
	}
	return nil
}

func (a *channelRuntimeAdapter) Stop(ctx context.Context, workerID string) error {
	return a.inner.Stop(ctx, workerID)
}

// --- spawn ---

func (realChannelBackend) SpawnWorker(ctx context.Context, opts SpawnWorkerOptions, runtime WorkerRuntime) (SpawnResult, error) {
	if runtime == nil {
		// 生产路径：走 D 组 supervisor.SpawnWorker
		//（守卫 → 写配置 → 派生 supervisor 子进程）。
		return spawnViaSupervisor(ctx, opts)
	}
	adapter := &channelRuntimeAdapter{inner: runtime}
	state, err := channel.SpawnWorker(ctx, channel.SpawnWorkerOptions{
		ChannelAddress: channel.ChannelAddress{Channel: opts.Channel, Scope: opts.Scope},
		By:             opts.By, Origin: channel.OriginCLI,
		WorkerID: opts.As, Provider: opts.Provider, Agent: opts.Agent,
		SystemPrompt: opts.SystemPrompt, Model: opts.Model,
		Resume: opts.Resume, Sandbox: opts.Sandbox,
		ContextFiles: opts.ContextFiles, InboxPolicy: opts.InboxPolicy,
	}, adapter)
	if err != nil {
		return SpawnResult{}, err
	}
	logPath := ""
	if ref, rerr := resolveRealRef(ChannelRef{Name: opts.Channel, Scope: opts.Scope}, false); rerr == nil {
		if p, perr := store.WorkerFile(ref.Name, state.WorkerID, "log", ref.Project); perr == nil {
			logPath = p
		}
	}
	return SpawnResult{PID: adapter.lastStart.PID, Log: logPath, Worker: state.WorkerID}, nil
}

// spawnViaSupervisor 用 D 组 supervisor.SpawnWorker 派生 worker。
func spawnViaSupervisor(_ context.Context, opts SpawnWorkerOptions) (SpawnResult, error) {
	warnBeforeMs := opts.WarnBefore.Milliseconds()
	idleTimeoutMs := opts.IdleTimeout.Milliseconds()
	maxLive := opts.MaxLiveWorkers
	res, err := supervisor.SpawnWorker(opts.Channel, supervisor.SpawnOptions{
		Provider: opts.Provider, As: opts.As, Agent: opts.Agent,
		Cwd: opts.Cwd, Model: opts.Model, Resume: opts.Resume,
		Sandbox: opts.Sandbox, TimeoutMs: opts.Timeout.Milliseconds(),
		WarnBeforeMs: &warnBeforeMs,
		Files:        opts.Files, JSONLs: opts.JSONLManifests,
		Scope: opts.Scope, By: opts.By, InboxPolicy: opts.InboxPolicy,
		IdleTimeoutMs: &idleTimeoutMs, MaxLiveWorkers: &maxLive,
		SupervisorBinary: channelSupervisorBinary,
	})
	if err != nil {
		return SpawnResult{}, err
	}
	return SpawnResult{PID: res.PID, Log: res.Log, Worker: res.Worker}, nil
}

// --- list / workers ---

func (realChannelBackend) ListChannels(_ context.Context, opts ListChannelsOptions) ([]ChannelSummary, error) {
	var projects []string
	switch opts.Scope {
	case ScopeGlobal:
		projects = []string{store.GlobalProjectKey}
	case ScopeProject:
		projects = []string{store.CurrentProjectKey()}
	default:
		if opts.AllProjects {
			projects = store.ListProjects()
		} else {
			projects = []string{store.CurrentProjectKey(), store.GlobalProjectKey}
		}
	}
	rows := []ChannelSummary{}
	for _, project := range projects {
		for _, name := range store.ListChannelNamesInProject(project) {
			ref := store.ChannelRef{Name: name, Project: project}
			events, err := store.ReadChannelEvents(ref, nil)
			if err != nil || len(events) == 0 {
				continue
			}
			var first, last store.ChannelEvent
			first = events[0]
			last = events[len(events)-1]
			if opts.Project != "" && !strings.Contains(first.StrField("task"), opts.Project) {
				continue
			}
			ephemeral := first.BoolField("ephemeral")
			if ephemeral && !opts.All {
				continue
			}
			metadata := store.ReduceChannelMetadata(events)
			registry := store.ReduceWorkerRegistry(events, &ref)
			alive, total := 0, 0
			for _, w := range registry.Workers {
				total++
				if !w.Terminal {
					alive++
				}
			}
			desc := metadata.Title
			if desc == "" {
				desc = metadata.Description
			}
			rows = append(rows, ChannelSummary{
				Name: name, Project: project,
				CreatedAt: first.StrField("ts"), Task: first.StrField("task"),
				Type: string(metadata.Type), Description: desc,
				WorkersAlive: alive, WorkersTotal: total,
				LastEventTs: last.StrField("ts"), LastEventKind: string(last.Kind),
				TotalEvents: len(events), Ephemeral: ephemeral,
			})
		}
	}
	sort.Slice(rows, func(i, j int) bool { return rows[i].LastEventTs > rows[j].LastEventTs })
	return rows, nil
}

func (realChannelBackend) ListWorkers(_ context.Context, ref ChannelRef) ([]ChannelWorkerState, error) {
	states, err := channel.ListWorkers(channel.ListWorkersOptions{
		ChannelAddress: toChannelAddress(ref), IncludeTerminal: true,
	})
	if err != nil {
		return nil, err
	}
	out := make([]ChannelWorkerState, 0, len(states))
	for _, s := range states {
		out = append(out, ChannelWorkerState{
			WorkerID: s.WorkerID, Agent: s.Agent, Provider: s.Provider,
			Lifecycle: string(s.Lifecycle), Activity: string(s.Activity),
			SpawnedAt: s.SpawnedAt, UpdatedAt: s.UpdatedAt,
			StartedBy: s.StartedBy, Reason: s.Reason, Error: s.Error,
			IdleSince: s.IdleSince, InboxPolicy: string(s.InboxPolicy),
		})
		if s.Channel != nil {
			out[len(out)-1].Channel = s.Channel.Name
		}
	}
	return out, nil
}

// --- prune / rm / kill ---

// pruneBuckets 按 scope 定 prune 扫描的项目桶（未指定 scope 时全桶，
// 与上游 channelPrune“无 scope 即全仓库清理”一致）。
func pruneBuckets(scope string) []string {
	switch scope {
	case ScopeGlobal:
		return []string{store.GlobalProjectKey}
	case ScopeProject:
		return []string{store.CurrentProjectKey()}
	default:
		return store.ListProjects()
	}
}

// channelHasLiveWorker 按 pid 文件判断 channel 里是否有存活 worker
// （与上游 rm.ts/prune.ts 的 hasLiveWorker 一致）。
func channelHasLiveWorker(dir string) bool {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return false
	}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".pid") {
			continue
		}
		data, err := os.ReadFile(filepath.Join(dir, entry.Name()))
		if err != nil {
			continue
		}
		pid, err := strconv.Atoi(strings.TrimSpace(string(data)))
		if err != nil || pid <= 0 {
			continue
		}
		if pidAlive(pid) {
			return true
		}
	}
	return false
}

func (realChannelBackend) ListPruneCandidates(_ context.Context, opts PruneOptions) ([]PruneCandidate, error) {
	keep := map[string]bool{}
	for _, k := range opts.Keep {
		keep[k] = true
	}
	out := []PruneCandidate{}
	for _, project := range pruneBuckets(opts.Scope) {
		for _, name := range store.ListChannelNamesInProject(project) {
			if keep[name] {
				continue
			}
			dir, err := store.ChannelDir(name, project)
			if err != nil {
				continue
			}
			if channelHasLiveWorker(dir) {
				continue
			}
			ref := store.ChannelRef{Name: name, Project: project}
			events, err := store.ReadChannelEvents(ref, nil)
			if err != nil {
				events = nil
			}
			var lastTs string
			ephemeral := false
			if len(events) > 0 {
				lastTs = events[len(events)-1].StrField("ts")
				ephemeral = events[0].BoolField("ephemeral")
			}
			var reason string
			switch {
			case opts.Ephemeral:
				if ephemeral {
					reason = "ephemeral"
				}
			case opts.All:
				reason = "all"
			case opts.Empty:
				if len(events) <= 1 {
					reason = "empty"
				}
			case opts.Idle > 0:
				if lastTs != "" {
					if ts, err := time.Parse(time.RFC3339Nano, lastTs); err == nil {
						if age := time.Since(ts); age >= opts.Idle {
							reason = fmt.Sprintf("idle %dm", int(age.Minutes()))
						}
					}
				}
			}
			if reason != "" {
				out = append(out, PruneCandidate{Name: name, Project: project, Reason: reason, LastTs: lastTs})
			}
		}
	}
	return out, nil
}

// killLiveWorkers 结束 channel 目录里 .pid 文件指向的活进程
// （SIGTERM，等 1.5 秒仍活着则 SIGKILL；与上游 rm.ts 一致）。
func killLiveWorkers(dir string) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".pid") {
			continue
		}
		data, err := os.ReadFile(filepath.Join(dir, entry.Name()))
		if err != nil {
			continue
		}
		pid, err := strconv.Atoi(strings.TrimSpace(string(data)))
		if err != nil || pid <= 0 {
			continue
		}
		if !pidAlive(pid) {
			continue
		}
		_ = signalProcess(pid, syscall.SIGTERM)
		deadline := time.Now().Add(1500 * time.Millisecond)
		for time.Now().Before(deadline) {
			if !pidAlive(pid) {
				break
			}
			time.Sleep(50 * time.Millisecond)
		}
		if pidAlive(pid) {
			_ = signalProcess(pid, syscall.SIGKILL)
		}
	}
}

func (realChannelBackend) RemoveChannel(_ context.Context, ref ChannelRef) error {
	storeRef, err := resolveRealRef(ref, false)
	if err != nil {
		// prune 按 project 精确删：ref.Project 兜底。
		if ref.Project != "" {
			storeRef = store.ChannelRef{Name: ref.Name, Project: ref.Project}
			if _, err := store.ChannelDir(ref.Name, ref.Project); err != nil {
				return err
			}
		} else {
			return err
		}
	}
	dir := storeRef.Dir
	if dir == "" {
		d, err := store.ChannelDir(storeRef.Name, storeRef.Project)
		if err != nil {
			return err
		}
		dir = d
	}
	if _, err := os.Stat(dir); err != nil {
		return fmt.Errorf("channel '%s' 不存在", ref.Name)
	}
	killLiveWorkers(dir)
	if err := os.RemoveAll(dir); err != nil {
		return fmt.Errorf("删除 channel 目录失败：%v", err)
	}
	return nil
}

func (realChannelBackend) KillWorker(_ context.Context, ref ChannelRef, worker string, force bool) error {
	storeRef, err := resolveRealRef(ref, false)
	if err != nil {
		return err
	}
	// worker 级锁，防 kill ↔ spawn 竞态（与上游 kill.ts 一致）。
	lockPath, err := store.WorkerLockPath(ref.Name, worker, storeRef.Project)
	if err != nil {
		return err
	}
	_, err = store.WithLock(lockPath, func() (bool, error) {
		return true, killWorkerLocked(storeRef, worker, force)
	}, store.AcquireOptions{MaxWaitMs: 10000})
	return err
}

// killWorkerLocked 执行 kill：读 pid 文件，优雅停机（或 --force 直接
// SIGKILL），等退出，追加 killed 事件，清理 worker 临时文件
// （保留 log 与 session-id/thread-id 供复查与续跑，与上游一致）。
func killWorkerLocked(ref store.ChannelRef, worker string, force bool) error {
	pidPath, err := store.WorkerFile(ref.Name, worker, "pid", ref.Project)
	if err != nil {
		return err
	}
	data, err := os.ReadFile(pidPath)
	if err != nil {
		return fmt.Errorf("worker '%s' 未在 channel '%s' 中运行", worker, ref.Name)
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(data)))
	if err != nil || pid <= 0 {
		return fmt.Errorf("worker '%s' 的 pid 文件内容无效", worker)
	}
	appendKillEvent := func(kind, reason, signal, detail string) {
		extra := map[string]any{"worker": worker}
		if reason != "" {
			extra["reason"] = reason
		}
		if signal != "" {
			extra["signal"] = signal
		}
		if detail != "" {
			extra["detail"] = detail
		}
		_, _ = store.AppendEvent(ref, store.AppendableEvent{
			Kind: store.ChannelEventKind(kind), By: "cli:kill", Extra: extra,
		})
	}
	cleanupWorkerFiles := func() {
		for _, suffix := range []string{"pid", "worker-pid", "config", "system-prompt.md", "spawnlock"} {
			if p, err := store.WorkerFile(ref.Name, worker, suffix, ref.Project); err == nil {
				_ = os.Remove(p)
			}
		}
	}
	alive := pidAlive(pid)
	if !alive {
		appendKillEvent("error", "", "", fmt.Sprintf("supervisor 已丢失（pid %d）", pid))
		cleanupWorkerFiles()
		return nil
	}
	signal := syscall.SIGTERM
	signalName := "SIGTERM"
	if force {
		signal = syscall.SIGKILL
		signalName = "SIGKILL"
		// --force 时也杀掉内层 worker，避免孤儿进程。
		if wpidPath, err := store.WorkerFile(ref.Name, worker, "worker-pid", ref.Project); err == nil {
			if wdata, err := os.ReadFile(wpidPath); err == nil {
				if wpid, err := strconv.Atoi(strings.TrimSpace(string(wdata))); err == nil && wpid > 0 {
					if pidAlive(wpid) {
						_ = signalProcess(wpid, syscall.SIGKILL)
					}
				}
			}
		}
		_ = signalProcess(pid, signal)
		appendKillEvent("killed", "explicit-kill", signalName, "")
		cleanupWorkerFiles()
		return nil
	}
	_ = signalProcess(pid, signal)
	deadline := time.Now().Add(8 * time.Second)
	for time.Now().Before(deadline) {
		if !pidAlive(pid) {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	if pidAlive(pid) {
		_ = signalProcess(pid, syscall.SIGKILL)
		appendKillEvent("killed", "explicit-kill", "SIGKILL", "宽限期到，supervisor 被 SIGKILL")
	} else {
		appendKillEvent("killed", "explicit-kill", signalName, "")
	}
	cleanupWorkerFiles()
	return nil
}

// --- context / title ---

func (realChannelBackend) AddChannelContext(_ context.Context, ref ChannelRef, by string, entries []ChannelContextEntry) (ChannelEvent, error) {
	ev, err := channel.AddChannelContext(channel.ContextMutationOptions{
		ChannelAddress: toChannelAddress(ref), By: by, Origin: channel.OriginCLI,
		Context: toAPIContextEntries(entries),
	})
	if err != nil {
		return nil, err
	}
	return fromAPIEvent(ev)
}

func (realChannelBackend) DeleteChannelContext(_ context.Context, ref ChannelRef, by string, entries []ChannelContextEntry) (ChannelEvent, error) {
	ev, err := channel.DeleteChannelContext(channel.ContextMutationOptions{
		ChannelAddress: toChannelAddress(ref), By: by, Origin: channel.OriginCLI,
		Context: toAPIContextEntries(entries),
	})
	if err != nil {
		return nil, err
	}
	return fromAPIEvent(ev)
}

func (realChannelBackend) ListChannelContext(_ context.Context, ref ChannelRef) ([]ChannelContextEntry, error) {
	entries, err := channel.ListChannelContext(toChannelAddress(ref))
	if err != nil {
		return nil, err
	}
	return fromAPIContextEntries(entries), nil
}

func (realChannelBackend) AddThreadContext(_ context.Context, ref ChannelRef, thread string, by string, entries []ChannelContextEntry) (ChannelEvent, error) {
	ev, err := channel.AddThreadContext(channel.ThreadContextMutationOptions{
		ContextMutationOptions: channel.ContextMutationOptions{
			ChannelAddress: toChannelAddress(ref), By: by, Origin: channel.OriginCLI,
			Context: toAPIContextEntries(entries),
		},
		Thread: thread,
	})
	if err != nil {
		return nil, err
	}
	return fromAPIEvent(ev)
}

func (realChannelBackend) DeleteThreadContext(_ context.Context, ref ChannelRef, thread string, by string, entries []ChannelContextEntry) (ChannelEvent, error) {
	ev, err := channel.DeleteThreadContext(channel.ThreadContextMutationOptions{
		ContextMutationOptions: channel.ContextMutationOptions{
			ChannelAddress: toChannelAddress(ref), By: by, Origin: channel.OriginCLI,
			Context: toAPIContextEntries(entries),
		},
		Thread: thread,
	})
	if err != nil {
		return nil, err
	}
	return fromAPIEvent(ev)
}

func (realChannelBackend) ListThreadContext(_ context.Context, ref ChannelRef, thread string) ([]ChannelContextEntry, error) {
	entries, err := channel.ListThreadContext(channel.ThreadContextListOptions{
		ChannelAddress: toChannelAddress(ref), Thread: thread,
	})
	if err != nil {
		return nil, err
	}
	return fromAPIContextEntries(entries), nil
}

func (realChannelBackend) SetChannelTitle(_ context.Context, ref ChannelRef, by string, title string) (ChannelEvent, error) {
	ev, err := channel.SetChannelTitle(channel.SetChannelTitleOptions{
		ChannelAddress: toChannelAddress(ref), By: by, Origin: channel.OriginCLI,
		Title: title,
	})
	if err != nil {
		return nil, err
	}
	return fromAPIEvent(ev)
}

func (realChannelBackend) ClearChannelTitle(_ context.Context, ref ChannelRef, by string) (ChannelEvent, error) {
	ev, err := channel.ClearChannelTitle(channel.ClearChannelTitleOptions{
		ChannelAddress: toChannelAddress(ref), By: by, Origin: channel.OriginCLI,
	})
	if err != nil {
		return nil, err
	}
	return fromAPIEvent(ev)
}
