package channel

import (
	"fmt"

	"github.com/linrong1982/Sunshine/internal/channel/store"
)

// SetChannelTitle 设置 channel 的显示标题。标题为空时报错，
// 清除标题请用 ClearChannelTitle。
func SetChannelTitle(opts SetChannelTitleOptions) (ChannelEvent, error) {
	if opts.Title == "" {
		return ChannelEvent{}, fmt.Errorf("channel 标题不能为空（清除标题请用 ClearChannelTitle）")
	}
	ref, err := resolveRef(opts.ChannelAddress, false)
	if err != nil {
		return ChannelEvent{}, err
	}
	partial := withExtra(newAppendable(EventKindChannel, opts.By, opts.Origin, opts.Meta), map[string]any{
		"action": "title",
		"title":  opts.Title,
	})
	ev, err := store.AppendEvent(ref, partial)
	if err != nil {
		return ChannelEvent{}, err
	}
	return fromStoreEvent(ev)
}

// ClearChannelTitle 清除 channel 的显示标题（追加 title 为 null 的事件）。
func ClearChannelTitle(opts ClearChannelTitleOptions) (ChannelEvent, error) {
	ref, err := resolveRef(opts.ChannelAddress, false)
	if err != nil {
		return ChannelEvent{}, err
	}
	partial := withExtra(newAppendable(EventKindChannel, opts.By, opts.Origin, opts.Meta), map[string]any{
		"action": "title",
		"title":  nil,
	})
	ev, err := store.AppendEvent(ref, partial)
	if err != nil {
		return ChannelEvent{}, err
	}
	return fromStoreEvent(ev)
}
