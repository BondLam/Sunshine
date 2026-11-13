// Forum 存储适配层：把 Forum API 接到 A 组的 internal/channel/store 上。
//
// B 组的 Store 接口尚未落地；在此之前 Forum 只依赖本文件定义的 ForumStore
// 最小接口，持久化由 fileStoreAdapter 转发给 store 包（JSONL + .seq sidecar、
// 文件锁、幂等去重、撕裂尾部截断均由 A 组实现）。
// 两种事件类型（API 层的 channel.ChannelEvent 与 store.ChannelEvent）
// 之间用 JSON 往返转换：store 事件的专有字段都在 Extra 里，
// API 层事件的未知字段也在 Extra 里，往返不丢失。
package channel

import (
	"encoding/json"
	"fmt"

	"github.com/linrong007/Sunshine/internal/channel/store"
)

// ForumStore 是 Forum API 依赖的最小存储接口。
type ForumStore interface {
	// AppendEvent 追加一个事件（partial 须含 kind、by；其余字段见 forum.go）。
	AppendEvent(channel, project string, partial map[string]any) (ChannelEvent, error)
	// ReadChannelEvents 按 seq 升序读事件；坏行跳过；分页见 Pagination。
	ReadChannelEvents(channel, project string, p Pagination) ([]ChannelEvent, error)
}

// fileStoreAdapter 是 ForumStore 的文件实现（转发给 A 组 store 包）。
type fileStoreAdapter struct{}

// NewFileStore 建一个文件存储适配器。
// 存储根由 SUNSHINE_CHANNEL_ROOT 环境变量决定（A 组 store 包的规则）。
func NewFileStore() *fileStoreAdapter { return &fileStoreAdapter{} }

// DefaultStore 返回默认的文件存储适配器。
func DefaultStore() *fileStoreAdapter { return NewFileStore() }

// AppendEvent 把 partial 拆为 store.AppendableEvent 后追加，
// 返回的 store 事件再转回 API 层事件。
func (s *fileStoreAdapter) AppendEvent(channel, project string, partial map[string]any) (ChannelEvent, error) {
	kind, _ := partial["kind"].(string)
	by, _ := partial["by"].(string)
	appendable := store.AppendableEvent{
		Kind: store.ChannelEventKind(kind),
		By:   by,
	}
	if v, ok := partial["ts"].(string); ok {
		appendable.Ts = v
	}
	if v, ok := partial["idempotencyKey"].(string); ok {
		appendable.IdempotencyKey = v
	}
	if v, ok := partial["origin"].(string); ok {
		appendable.Origin = store.EventOrigin(v)
	}
	if v, ok := partial["meta"].(map[string]any); ok {
		appendable.Meta = v
	}
	if v, ok := partial["to"].([]string); ok {
		appendable.To = v
	}
	extra := make(map[string]any, len(partial))
	for k, v := range partial {
		switch k {
		case "kind", "by", "ts", "idempotencyKey", "origin", "meta", "to":
		default:
			extra[k] = v
		}
	}
	if len(extra) > 0 {
		appendable.Extra = extra
	}
	ev, err := store.AppendEvent(store.ChannelRef{Name: channel, Project: project}, appendable)
	if err != nil {
		return ChannelEvent{}, err
	}
	return fromStoreEvent(ev)
}

// ReadChannelEvents 读事件并转回 API 层事件。
func (s *fileStoreAdapter) ReadChannelEvents(channel, project string, p Pagination) ([]ChannelEvent, error) {
	var pagination *store.ReadChannelEventsPagination
	if p.AfterSeq != nil || p.BeforeSeq != nil || p.Limit != nil {
		pagination = &store.ReadChannelEventsPagination{
			AfterSeq:  p.AfterSeq,
			BeforeSeq: p.BeforeSeq,
			Limit:     p.Limit,
		}
	}
	events, err := store.ReadChannelEvents(
		store.ChannelRef{Name: channel, Project: project}, pagination)
	if err != nil {
		return nil, err
	}
	out := make([]ChannelEvent, 0, len(events))
	for _, ev := range events {
		converted, err := fromStoreEvent(ev)
		if err != nil {
			return nil, err
		}
		out = append(out, converted)
	}
	return out, nil
}

// fromStoreEvent 把 store 事件经 JSON 往返转为 API 层事件。
func fromStoreEvent(ev store.ChannelEvent) (ChannelEvent, error) {
	data, err := json.Marshal(ev)
	if err != nil {
		return ChannelEvent{}, fmt.Errorf("转换事件失败：%w", err)
	}
	var out ChannelEvent
	if err := json.Unmarshal(data, &out); err != nil {
		return ChannelEvent{}, fmt.Errorf("转换事件失败：%w", err)
	}
	return out, nil
}

// resolveRef 已由 B 组 api.go 提供（ResolveChannel）；
// 本文件不再自带解析函数（原 resolveRef 已删，避免重声明）。
