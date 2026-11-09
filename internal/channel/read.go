package channel

import (
	"fmt"

	"github.com/linrong1982/Sunshine/internal/channel/store"
)

// ReadEvents 读 channel 事件；分页参数透传给 store
// （afterSeq 与 beforeSeq 不可同传；带游标时默认页大小 200）。
// 不传任何分页参数时返回全部事件。
func ReadEvents(opts ReadEventsOptions) ([]ChannelEvent, error) {
	ref, err := resolveRef(opts.ChannelAddress, false)
	if err != nil {
		return nil, err
	}
	var pagination *store.ReadChannelEventsPagination
	if opts.AfterSeq != nil || opts.BeforeSeq != nil || opts.Limit != nil {
		pagination = &store.ReadChannelEventsPagination{
			AfterSeq:  opts.AfterSeq,
			BeforeSeq: opts.BeforeSeq,
			Limit:     opts.Limit,
		}
	}
	events, err := store.ReadChannelEvents(ref, pagination)
	if err != nil {
		return nil, err
	}
	return toAPIEvents(events)
}

// ReadChannelMetadata 从事件流投影 channel 元数据。
func ReadChannelMetadata(addr ChannelAddress) (ChannelMetadata, error) {
	ref, err := resolveRef(addr, false)
	if err != nil {
		return ChannelMetadata{}, err
	}
	events, err := readAllEvents(ref)
	if err != nil {
		return ChannelMetadata{}, err
	}
	return store.ReduceChannelMetadata(events), nil
}

// ReadWorkerInbox 读某 worker 收件箱里的消息。
//
// 先由持久 worker 注册表找 worker：不存在时报错
// WORKER_INBOX_WORKER_NOT_FOUND；已终态且未传 IncludeTerminal 时
// 报错 WORKER_INBOX_WORKER_TERMINAL。只返回符合该 worker 收件箱
// 策略的 message 事件（explicitOnly 只收定向自己的，
// broadcastAndExplicit 另收广播；自己发的消息永不进收件箱）。
func ReadWorkerInbox(opts ReadWorkerInboxOptions) ([]WorkerInboxMessage, error) {
	ref, err := resolveRef(opts.ChannelAddress, false)
	if err != nil {
		return nil, err
	}
	if opts.Limit != nil && *opts.Limit < 0 {
		return nil, fmt.Errorf("readWorkerInbox：limit 须为非负整数")
	}
	if opts.Limit != nil && *opts.Limit == 0 {
		return []WorkerInboxMessage{}, nil
	}
	registry, events, err := readRegistry(ref)
	if err != nil {
		return nil, err
	}
	worker := registry.FindWorker(opts.WorkerID)
	if worker == nil {
		return nil, &WorkerInboxError{Code: WorkerInboxWorkerNotFound, Channel: opts.Channel, WorkerID: opts.WorkerID}
	}
	if worker.Terminal && !opts.IncludeTerminal {
		return nil, &WorkerInboxError{Code: WorkerInboxWorkerTerminal, Channel: opts.Channel, WorkerID: opts.WorkerID}
	}
	afterSeq := int64(0)
	if opts.AfterSeq != nil {
		afterSeq = *opts.AfterSeq
	}
	out := []WorkerInboxMessage{}
	for _, ev := range events {
		if ev.Seq <= afterSeq {
			continue
		}
		if !store.MatchesInboxPolicy(ev, opts.WorkerID, worker.InboxPolicy) {
			continue
		}
		converted, err := fromStoreEvent(ev)
		if err != nil {
			return nil, err
		}
		out = append(out, WorkerInboxMessage{
			WorkerID: opts.WorkerID,
			Event:    converted,
			Seq:      ev.Seq,
			Cursor:   ev.Seq,
		})
		if opts.Limit != nil && len(out) >= *opts.Limit {
			break
		}
	}
	return out, nil
}
