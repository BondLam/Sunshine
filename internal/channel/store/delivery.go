package store

import (
	"fmt"
)

// DeliveryMode 是定向 sendMessage 的投递校验模式。
type DeliveryMode string

const (
	// DeliveryModeAppendOnly 默认：只追加，永不报错，保留 pre-spawn 积压投递。
	DeliveryModeAppendOnly DeliveryMode = "appendOnly"
	// DeliveryModeRequireKnownWorker 目标从未出现 → 追加 undeliverable{reason: worker-unknown}。
	DeliveryModeRequireKnownWorker DeliveryMode = "requireKnownWorker"
	// DeliveryModeRequireRunningWorker 未知或终态 → undeliverable{reason: worker-unknown|worker-terminal}。
	DeliveryModeRequireRunningWorker DeliveryMode = "requireRunningWorker"
)

// DeliveryModes 是全部合法的投递模式。
var DeliveryModes = map[DeliveryMode]bool{
	DeliveryModeAppendOnly:           true,
	DeliveryModeRequireKnownWorker:   true,
	DeliveryModeRequireRunningWorker: true,
}

// ParseDeliveryMode 校验投递模式。
func ParseDeliveryMode(v string) (DeliveryMode, error) {
	if !DeliveryModes[DeliveryMode(v)] {
		return "", fmt.Errorf("无效的投递模式 %q，必须是 appendOnly、requireKnownWorker 或 requireRunningWorker", v)
	}
	return DeliveryMode(v), nil
}

// UndeliverableTarget 是一个无法投递的目标 worker 及其原因。
type UndeliverableTarget struct {
	TargetWorker string
	Reason       UndeliverableReason
}

// ClassifyDelivery 是纯函数：按持久 worker 注册表判断哪些定向目标
// 无法投递。appendOnly 恒返回空；广播（无目标）永不产生 undeliverable。
func ClassifyDelivery(registry WorkerRegistry, targets []string, mode DeliveryMode) []UndeliverableTarget {
	if mode == DeliveryModeAppendOnly || len(targets) == 0 {
		return nil
	}
	var failed []UndeliverableTarget
	for _, target := range targets {
		worker := registry.FindWorker(target)
		if worker == nil {
			failed = append(failed, UndeliverableTarget{TargetWorker: target, Reason: UndeliverableWorkerUnknown})
			continue
		}
		if mode == DeliveryModeRequireRunningWorker && worker.Terminal {
			failed = append(failed, UndeliverableTarget{TargetWorker: target, Reason: UndeliverableWorkerTerminal})
		}
	}
	return failed
}

// AppendMessage 追加一条 message 事件，并在严格投递模式下为每个
// 失败目标追加 undeliverable 事件。消息本身先落盘，再追加
// undeliverable 事件，用户意图永不丢失。广播消息（无 to）永不产生
// undeliverable。返回消息事件与追加的 undeliverable 事件。
//
// undeliverable 事件的 idempotencyKey 由消息的 key 派生：
// "<key>:undeliverable:<targetWorker>"（消息无 key 时不设），
// 保证消息重放时 undeliverable 不会重复追加。
func AppendMessage(ref ChannelRef, partial AppendableEvent, mode DeliveryMode) (ChannelEvent, []ChannelEvent, error) {
	if partial.Kind == "" {
		partial.Kind = KindMessage
	}
	if partial.Kind != KindMessage {
		return ChannelEvent{}, nil, fmt.Errorf("AppendMessage 只接受 kind:message，收到 %q", string(partial.Kind))
	}
	msg, err := AppendEvent(ref, partial)
	if err != nil {
		return ChannelEvent{}, nil, err
	}
	if mode == DeliveryModeAppendOnly || len(msg.To) == 0 {
		return msg, nil, nil
	}
	events, err := ReadChannelEvents(ref, nil)
	if err != nil {
		return msg, nil, err
	}
	registry := ReduceWorkerRegistry(events, &ref)
	failures := ClassifyDelivery(registry, msg.To, mode)
	// 已存在的幂等键集合：消息重放时派生的 undeliverable 键已存在，
	// 不再重复追加、也不计入本次返回。
	existingKeys := map[string]bool{}
	for _, ev := range events {
		if ev.IdempotencyKey != "" {
			existingKeys[ev.IdempotencyKey] = true
		}
	}
	var undeliverable []ChannelEvent
	for _, failure := range failures {
		und := AppendableEvent{
			Kind:   KindUndeliverable,
			By:     partial.By,
			Origin: partial.Origin,
			Meta:   partial.Meta,
			Extra: map[string]any{
				"targetWorker": failure.TargetWorker,
				"messageSeq":   msg.Seq,
				"reason":       string(failure.Reason),
			},
		}
		if partial.IdempotencyKey != "" {
			und.IdempotencyKey = partial.IdempotencyKey + ":undeliverable:" + failure.TargetWorker
			if existingKeys[und.IdempotencyKey] {
				continue
			}
		}
		ev, err := AppendEvent(ref, und)
		if err != nil {
			return msg, undeliverable, err
		}
		undeliverable = append(undeliverable, ev)
	}
	return msg, undeliverable, nil
}
