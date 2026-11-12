package channel

import (
	"context"
	"strings"
	"time"

	"github.com/BondLam/Sunshine/internal/channel/store"
)

// WatchEvents 流式监听某 channel 的新事件。默认只推监听开始后
// 的新事件；FromStart 或 SinceSeq 可回读历史。
//
// 调用返回前先同步快照最后 seq（无 FromStart/SinceSeq 时以它为
// 续跑点），避免监听 goroutine 启动前的竞态导致丢事件。
// 上下文取消时关闭返回的两个 channel。
func WatchEvents(ctx context.Context, addr ChannelAddress, opts WatchEventsOptions) (<-chan ChannelEvent, <-chan error) {
	out := make(chan ChannelEvent)
	errCh := make(chan error, 1)
	fail := func(err error) (<-chan ChannelEvent, <-chan error) {
		errCh <- err
		close(out)
		close(errCh)
		return out, errCh
	}
	ref, err := resolveRef(addr, false)
	if err != nil {
		return fail(err)
	}
	storeOpts := store.WatchOptions{
		FromStart:    opts.FromStart,
		Filter:       opts.Filter,
		PollInterval: opts.PollInterval,
	}
	switch {
	case opts.SinceSeq != nil:
		storeOpts.HasSinceSeq = true
		storeOpts.SinceSeq = *opts.SinceSeq
	case !opts.FromStart:
		// 同步快照最后 seq 作为续跑点。
		events, err := store.ReadChannelEvents(ref, nil)
		if err != nil {
			return fail(err)
		}
		if len(events) > 0 {
			storeOpts.HasSinceSeq = true
			storeOpts.SinceSeq = events[len(events)-1].Seq
		}
	}
	go func() {
		defer close(out)
		defer close(errCh)
		evCh, watchErrCh := store.WatchChannelEvents(ctx, ref, storeOpts)
		for {
			select {
			case <-ctx.Done():
				return
			case err, ok := <-watchErrCh:
				if !ok {
					watchErrCh = nil
					continue
				}
				if err != nil {
					select {
					case errCh <- err:
					case <-ctx.Done():
					}
					return
				}
			case ev, ok := <-evCh:
				if !ok {
					return
				}
				converted, err := fromStoreEvent(ev)
				if err != nil {
					select {
					case errCh <- err:
					case <-ctx.Done():
					}
					return
				}
				select {
				case out <- converted:
				case <-ctx.Done():
					return
				}
			}
		}
	}()
	return out, errCh
}

// WatchChannels 跨 channel 监听：监听某项目桶（或 global 桶）内
// 全部 channel 的事件，合并成一条流；监听开始后新建的 channel
// 会被动态发现。每个事件附带全量游标快照；投递为至少一次。
// 上下文取消时关闭返回的两个 channel。
func WatchChannels(ctx context.Context, opts WatchChannelsOptions) (<-chan CrossChannelEvent, <-chan error) {
	out := make(chan CrossChannelEvent)
	errCh := make(chan error, 1)
	go func() {
		defer close(out)
		defer close(errCh)
		projectKey := opts.ProjectKey
		if projectKey == "" && opts.CWD != "" {
			projectKey = store.ProjectKey(opts.CWD)
		}
		input := store.WatchChannelsInput{
			ProjectKey:           projectKey,
			Global:               opts.Global,
			Filter:               opts.Filter,
			Cursor:               opts.Cursor,
			FromStartNewChannels: opts.FromStartNewChannels,
		}
		if opts.DiscoveryIntervalMs > 0 {
			input.DiscoveryInterval = time.Duration(opts.DiscoveryIntervalMs) * time.Millisecond
		}
		evCh, watchErrCh := store.WatchChannels(ctx, input)
		for {
			select {
			case <-ctx.Done():
				return
			case err, ok := <-watchErrCh:
				if !ok {
					watchErrCh = nil
					continue
				}
				if err != nil {
					select {
					case errCh <- err:
					case <-ctx.Done():
					}
					return
				}
			case ev, ok := <-evCh:
				if !ok {
					return
				}
				converted, err := fromStoreEvent(ev.Event)
				if err != nil {
					select {
					case errCh <- err:
					case <-ctx.Done():
					}
					return
				}
				select {
				case out <- CrossChannelEvent{Channel: ev.Channel, Event: converted, Cursor: ev.Cursor}:
				case <-ctx.Done():
					return
				}
			}
		}
	}()
	return out, errCh
}

// WatchWorkers 监听某 channel 的 worker 注册表：先吐一次当前快照，
// 之后每当 worker 相关事件落盘就吐一次新快照。
// 调用返回前先同步读出当前注册表与最后 seq，避免监听 goroutine
// 启动前的竞态导致丢事件。上下文取消时关闭返回的两个 channel。
func WatchWorkers(ctx context.Context, opts WatchWorkersOptions) (<-chan []WorkerState, <-chan error) {
	out := make(chan []WorkerState, 16)
	errCh := make(chan error, 1)
	fail := func(err error) (<-chan []WorkerState, <-chan error) {
		errCh <- err
		close(out)
		close(errCh)
		return out, errCh
	}
	ref, err := resolveRef(opts.ChannelAddress, false)
	if err != nil {
		return fail(err)
	}
	registry, events, err := readRegistry(ref)
	if err != nil {
		return fail(err)
	}
	lastSeq := int64(0)
	if len(events) > 0 {
		lastSeq = events[len(events)-1].Seq
	}
	if opts.SinceSeq != nil {
		lastSeq = *opts.SinceSeq
	}
	initial := filterWorkers(registry, opts.IncludeTerminal)
	go func() {
		defer close(out)
		defer close(errCh)
		select {
		case out <- initial:
		case <-ctx.Done():
			return
		}
		storeOpts := store.WatchOptions{
			HasSinceSeq: true,
			SinceSeq:    lastSeq,
			Filter: store.ChannelEventFilter{
				IncludeNonMeaningful: true,
				IncludeProgress:      false,
			},
		}
		evCh, watchErrCh := store.WatchChannelEvents(ctx, ref, storeOpts)
		for {
			select {
			case <-ctx.Done():
				return
			case err, ok := <-watchErrCh:
				if !ok {
					watchErrCh = nil
					continue
				}
				if err != nil {
					select {
					case errCh <- err:
					case <-ctx.Done():
					}
					return
				}
			case ev, ok := <-evCh:
				if !ok {
					return
				}
				events = append(events, ev)
				registry = store.ReduceWorkerRegistry(events, &ref)
				select {
				case out <- filterWorkers(registry, opts.IncludeTerminal):
				case <-ctx.Done():
					return
				}
			}
		}
	}()
	return out, errCh
}

// WatchWorkerInbox 监听某 worker 收件箱未来的消息。
//
// 先做前置校验（worker 不存在或已终态时直接报错），再从当前
// 最后 seq（FromStart 时从本代 spawn 起）开始监听；当该 worker
// 的终态事件到达时结束流（不跨终态事件进入同名 worker 的下一代）。
// 校验与续跑点在调用返回前同步算好，避免监听 goroutine 启动前
// 的竞态导致丢事件。上下文取消时关闭返回的两个 channel。
func WatchWorkerInbox(ctx context.Context, opts WatchWorkerInboxOptions) (<-chan WorkerInboxMessage, <-chan error) {
	out := make(chan WorkerInboxMessage)
	errCh := make(chan error, 1)
	fail := func(err error) (<-chan WorkerInboxMessage, <-chan error) {
		errCh <- err
		close(out)
		close(errCh)
		return out, errCh
	}
	ref, err := resolveRef(opts.ChannelAddress, false)
	if err != nil {
		return fail(err)
	}
	registry, events, err := readRegistry(ref)
	if err != nil {
		return fail(err)
	}
	worker := registry.FindWorker(opts.WorkerID)
	if worker == nil {
		return fail(&WorkerInboxError{Code: WorkerInboxWorkerNotFound, Channel: opts.Channel, WorkerID: opts.WorkerID})
	}
	if worker.Terminal {
		return fail(&WorkerInboxError{Code: WorkerInboxWorkerTerminal, Channel: opts.Channel, WorkerID: opts.WorkerID})
	}
	policy := worker.InboxPolicy
	lastSeq := int64(0)
	if len(events) > 0 {
		lastSeq = events[len(events)-1].Seq
	}
	floorSeq := findGenerationFloorSeq(events, opts.WorkerID)
	var sinceSeq int64
	switch {
	case opts.FromStart:
		sinceSeq = floorSeq
	case opts.SinceSeq != nil:
		sinceSeq = *opts.SinceSeq
		if sinceSeq < floorSeq {
			sinceSeq = floorSeq
		}
	default:
		sinceSeq = lastSeq
	}
	go func() {
		defer close(out)
		defer close(errCh)
		storeOpts := store.WatchOptions{
			HasSinceSeq: true,
			SinceSeq:    sinceSeq,
			Filter: store.ChannelEventFilter{
				IncludeNonMeaningful: true,
				IncludeProgress:      false,
			},
		}
		evCh, watchErrCh := store.WatchChannelEvents(ctx, ref, storeOpts)
		for {
			select {
			case <-ctx.Done():
				return
			case err, ok := <-watchErrCh:
				if !ok {
					watchErrCh = nil
					continue
				}
				if err != nil {
					select {
					case errCh <- err:
					case <-ctx.Done():
					}
					return
				}
			case ev, ok := <-evCh:
				if !ok {
					return
				}
				if isTerminalForWorker(ev, opts.WorkerID) {
					return
				}
				if store.MatchesInboxPolicy(ev, opts.WorkerID, policy) {
					converted, err := fromStoreEvent(ev)
					if err != nil {
						select {
						case errCh <- err:
						case <-ctx.Done():
						}
						return
					}
					select {
					case out <- WorkerInboxMessage{WorkerID: opts.WorkerID, Event: converted, Seq: ev.Seq, Cursor: ev.Seq}:
					case <-ctx.Done():
						return
					}
				}
			}
		}
	}()
	return out, errCh
}

// filterWorkers 按 IncludeTerminal 过滤 worker 列表。
func filterWorkers(registry WorkerRegistry, includeTerminal bool) []WorkerState {
	if includeTerminal {
		return registry.Workers
	}
	out := []WorkerState{}
	for _, w := range registry.Workers {
		if !w.Terminal {
			out = append(out, w)
		}
	}
	return out
}

// isTerminalForWorker 判断事件是否为某 worker 的终态事件。
// killed 一律算终态；done/error 只有 supervisor 合成的才算终态
// （普通 done/error 事件不结束监听）。
func isTerminalForWorker(ev store.ChannelEvent, workerID string) bool {
	switch ev.Kind {
	case store.KindKilled:
		return resolveWorkerID(ev) == workerID
	case store.KindDone:
		if !ev.BoolField("synthesized") {
			return false
		}
		return resolveWorkerID(ev) == workerID
	case store.KindError:
		synthesized := ev.BoolField("synthesized")
		fromSupervisor := strings.HasPrefix(ev.By, "supervisor:")
		if !synthesized && !fromSupervisor {
			return false
		}
		return resolveWorkerID(ev) == workerID
	default:
		return false
	}
}

// isSpawnedForWorker 判断事件是否为某 worker 的 spawned 事件。
func isSpawnedForWorker(ev store.ChannelEvent, workerID string) bool {
	return ev.Kind == store.KindSpawned && ev.StrField("as") == workerID
}

// findGenerationFloorSeq 返回本代 worker 的起始 seq 下限：
// 最近一次终态事件的 seq（终态之后、最新一次 spawn 之前）
// 之后的事件才属于本代。
func findGenerationFloorSeq(events []store.ChannelEvent, workerID string) int64 {
	var lastTerminalSeq int64
	var floorSeq int64
	for _, ev := range events {
		if isTerminalForWorker(ev, workerID) {
			lastTerminalSeq = ev.Seq
			continue
		}
		if isSpawnedForWorker(ev, workerID) {
			floorSeq = lastTerminalSeq
		}
	}
	return floorSeq
}

// resolveWorkerID 解析事件指向的 worker id：显式 worker/as 字段优先，
// 否则剥掉 by 的 supervisor: 前缀。
func resolveWorkerID(ev store.ChannelEvent) string {
	if id := ev.StrField("worker"); id != "" {
		return id
	}
	if id := ev.StrField("as"); id != "" {
		return id
	}
	return strings.TrimPrefix(ev.By, "supervisor:")
}
