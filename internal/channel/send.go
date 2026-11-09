package channel

import (
	"fmt"
	"strings"

	"github.com/linrong1982/Sunshine/internal/channel/store"
)

// SendMessage 发一条消息。默认 appendOnly：只追加，永不报错，
// 保留 pre-spawn 积压投递。严格模式（requireKnownWorker /
// requireRunningWorker）下消息本身先落盘，再为每个失败目标
// 追加 undeliverable 事件；广播消息永不产生 undeliverable。
func SendMessage(opts SendMessageOptions) (ChannelEvent, error) {
	ref, err := resolveRef(opts.ChannelAddress, false)
	if err != nil {
		return ChannelEvent{}, err
	}
	mode, err := parseDeliveryModeOpt(opts.DeliveryMode)
	if err != nil {
		return ChannelEvent{}, err
	}
	for _, target := range opts.To {
		if strings.TrimSpace(target) == "" {
			return ChannelEvent{}, fmt.Errorf("定向目标 worker 名不能为空")
		}
	}
	partial := newAppendable(EventKindMessage, opts.By, opts.Origin, opts.Meta)
	partial.To = opts.To
	if opts.IdempotencyKey != "" {
		partial.IdempotencyKey = opts.IdempotencyKey
	}
	partial = withExtra(partial, map[string]any{"text": opts.Text})
	msg, _, err := store.AppendMessage(ref, partial, mode)
	if err != nil {
		return ChannelEvent{}, err
	}
	return fromStoreEvent(msg)
}
