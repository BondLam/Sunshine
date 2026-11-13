package channel

import (
	"fmt"

	"github.com/linrong007/Sunshine/internal/channel/store"
)

// appendContextEvent 追加一条 context 事件。
func appendContextEvent(ref ChannelRef, by string, origin EventOrigin, meta map[string]any, action, target string, context []ContextEntry, thread string) (ChannelEvent, error) {
	if err := ValidateContextEntries(context); err != nil {
		return ChannelEvent{}, err
	}
	extra := map[string]any{
		"target":  target,
		"action":  action,
		"context": context,
	}
	if thread != "" {
		extra["thread"] = thread
	}
	partial := withExtra(newAppendable(EventKindContext, by, origin, meta), extra)
	ev, err := store.AppendEvent(ref, partial)
	if err != nil {
		return ChannelEvent{}, err
	}
	return fromStoreEvent(ev)
}

// AddChannelContext 给 channel 加上下文条目。
func AddChannelContext(opts ContextMutationOptions) (ChannelEvent, error) {
	ref, err := resolveRef(opts.ChannelAddress, false)
	if err != nil {
		return ChannelEvent{}, err
	}
	return appendContextEvent(ref, opts.By, opts.Origin, opts.Meta, "add", "channel", opts.Context, "")
}

// DeleteChannelContext 删 channel 的上下文条目。
func DeleteChannelContext(opts ContextMutationOptions) (ChannelEvent, error) {
	ref, err := resolveRef(opts.ChannelAddress, false)
	if err != nil {
		return ChannelEvent{}, err
	}
	return appendContextEvent(ref, opts.By, opts.Origin, opts.Meta, "delete", "channel", opts.Context, "")
}

// ListChannelContext 列出 channel 当前的上下文条目（事件投影）。
func ListChannelContext(addr ChannelAddress) ([]ContextEntry, error) {
	meta, err := ReadChannelMetadata(addr)
	if err != nil {
		return nil, err
	}
	if meta.Context == nil {
		return []ContextEntry{}, nil
	}
	return meta.Context, nil
}

// AddThreadContext 给 forum 的 thread 加上下文条目；thread 不存在时报错。
func AddThreadContext(opts ThreadContextMutationOptions) (ChannelEvent, error) {
	ref, err := resolveRef(opts.ChannelAddress, false)
	if err != nil {
		return ChannelEvent{}, err
	}
	thread, err := store.NormalizeThreadKey(opts.Thread)
	if err != nil {
		return ChannelEvent{}, err
	}
	if err := assertKnownThread(ref, thread, opts.Channel); err != nil {
		return ChannelEvent{}, err
	}
	return appendContextEvent(ref, opts.By, opts.Origin, opts.Meta, "add", "thread", opts.Context, thread)
}

// DeleteThreadContext 删 forum thread 的上下文条目；thread 不存在时报错。
func DeleteThreadContext(opts ThreadContextMutationOptions) (ChannelEvent, error) {
	ref, err := resolveRef(opts.ChannelAddress, false)
	if err != nil {
		return ChannelEvent{}, err
	}
	thread, err := store.NormalizeThreadKey(opts.Thread)
	if err != nil {
		return ChannelEvent{}, err
	}
	if err := assertKnownThread(ref, thread, opts.Channel); err != nil {
		return ChannelEvent{}, err
	}
	return appendContextEvent(ref, opts.By, opts.Origin, opts.Meta, "delete", "thread", opts.Context, thread)
}

// ListThreadContext 列出 forum thread 当前的上下文条目（事件投影，
// 别名解析到当前键）；thread 不存在时返回空列表。
func ListThreadContext(opts ThreadContextListOptions) ([]ContextEntry, error) {
	ref, err := resolveRef(opts.ChannelAddress, false)
	if err != nil {
		return nil, err
	}
	key, err := store.NormalizeThreadKey(opts.Thread)
	if err != nil {
		return nil, err
	}
	events, err := readAllEvents(ref)
	if err != nil {
		return nil, err
	}
	states := store.ReduceThreads(events)
	resolver := store.BuildThreadAliasResolver(events)
	current := resolver.Resolve(key)
	for _, state := range states {
		if state.Thread == current {
			if state.Context == nil {
				return []ContextEntry{}, nil
			}
			return state.Context, nil
		}
	}
	return []ContextEntry{}, nil
}

// assertKnownThread 校验 thread 是否存在（按当前键或任一别名）。
func assertKnownThread(ref ChannelRef, thread, channel string) error {
	events, err := readAllEvents(ref)
	if err != nil {
		return err
	}
	states := store.ReduceThreads(events)
	resolver := store.BuildThreadAliasResolver(events)
	for _, state := range states {
		if state.Thread == thread {
			return nil
		}
		for _, alias := range resolver.AliasesFor(state.Thread) {
			if alias == thread {
				return nil
			}
		}
	}
	return fmt.Errorf("在频道 %q 中找不到 thread %q", channel, thread)
}
