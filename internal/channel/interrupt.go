package channel

import (
	"context"

	"github.com/linrong1982/Sunshine/internal/channel/store"
)

// RequestInterrupt 只追加 interrupt_requested 事件（持久化意图，
// 不碰 runtime）。供 CLI 的 channel interrupt 意图记录与
// supervisor 的纯事件中断使用。
func RequestInterrupt(input InterruptWorkerOptions) (ChannelEvent, error) {
	ref, err := resolveRef(input.ChannelAddress, false)
	if err != nil {
		return ChannelEvent{}, err
	}
	reason, err := parseInterruptReasonOpt(input.Reason)
	if err != nil {
		return ChannelEvent{}, err
	}
	extra := map[string]any{"worker": input.WorkerID}
	if reason != "" {
		extra["reason"] = string(reason)
	}
	if input.Message != "" {
		extra["message"] = input.Message
	}
	partial := withExtra(newAppendable(EventKindInterruptRequested, input.By, input.Origin, input.Meta), extra)
	ev, err := store.AppendEvent(ref, partial)
	if err != nil {
		return ChannelEvent{}, err
	}
	return fromStoreEvent(ev)
}

// InterruptWorker 是编排层中断：先追加 interrupt_requested，
// 再请注入的 runtime 中断 worker，最后追加 interrupted
// （带 runtime 报告的 method / outcome）。Core 绝不直接 import
// CLI 的 provider 适配器，runtime 由调用方注入。
//
// worker 未知或已终态时跳过 runtime 调用（与 interrupted 事件），
// 只返回 interrupt_requested 事件与对应 delivery。
// runtime 为 nil 时仍追加 interrupted 事件，method 为 none、
// outcome 为 unsupported（与上游无 interrupt 实现时的行为一致）。
func InterruptWorker(ctx context.Context, input InterruptWorkerOptions, runtime WorkerRuntime) (InterruptWorkerResult, error) {
	ref, err := resolveRef(input.ChannelAddress, false)
	if err != nil {
		return InterruptWorkerResult{}, err
	}
	reason, err := parseInterruptReasonOpt(input.Reason)
	if err != nil {
		return InterruptWorkerResult{}, err
	}
	registry, _, err := readRegistry(ref)
	if err != nil {
		return InterruptWorkerResult{}, err
	}
	worker := registry.FindWorker(input.WorkerID)

	turnID := ""
	if worker != nil {
		turnID = worker.ActiveTurnID
	}
	requestExtra := map[string]any{"worker": input.WorkerID}
	if turnID != "" {
		requestExtra["turnId"] = turnID
	}
	if reason != "" {
		requestExtra["reason"] = string(reason)
	}
	if input.Message != "" {
		requestExtra["message"] = input.Message
	}
	requestPartial := withExtra(newAppendable(EventKindInterruptRequested, input.By, input.Origin, input.Meta), requestExtra)
	requestEvent, err := store.AppendEvent(ref, requestPartial)
	if err != nil {
		return InterruptWorkerResult{}, err
	}
	requestAPIEvent, err := fromStoreEvent(requestEvent)
	if err != nil {
		return InterruptWorkerResult{}, err
	}
	if worker == nil {
		return InterruptWorkerResult{
			Event:       requestAPIEvent,
			Interrupted: false,
			Delivery:    InterruptDeliveryWorkerUnknown,
		}, nil
	}
	if worker.Terminal {
		return InterruptWorkerResult{
			Event:       requestAPIEvent,
			Interrupted: false,
			Delivery:    InterruptDeliveryWorkerTerminal,
		}, nil
	}

	// 请 runtime 中断；无 runtime 时记为不支持。
	method := InterruptMethodNone
	outcome := InterruptOutcomeUnsupported
	interrupted := false
	var interruptErr error
	if runtime != nil {
		interruptErr = runtime.Interrupt(ctx, input.WorkerID)
		method = InterruptMethodProvider
		if interruptErr == nil {
			outcome = InterruptOutcomeInterrupted
			interrupted = true
		} else {
			outcome = InterruptOutcomeFailed
		}
	}
	interruptedExtra := map[string]any{
		"worker":  input.WorkerID,
		"method":  string(method),
		"outcome": string(outcome),
	}
	if turnID != "" {
		interruptedExtra["turnId"] = turnID
	}
	if reason != "" {
		interruptedExtra["reason"] = string(reason)
	}
	if interruptErr != nil {
		interruptedExtra["message"] = interruptErr.Error()
	} else if input.Message != "" {
		interruptedExtra["message"] = input.Message
	}
	interruptedPartial := withExtra(newAppendable(EventKindInterrupted, input.By, input.Origin, input.Meta), interruptedExtra)
	interruptedEvent, err := store.AppendEvent(ref, interruptedPartial)
	if err != nil {
		return InterruptWorkerResult{}, err
	}
	interruptedAPIEvent, err := fromStoreEvent(interruptedEvent)
	if err != nil {
		return InterruptWorkerResult{}, err
	}
	delivery := InterruptDeliveryNoActiveTurn
	if worker.Activity == WorkerActivityMidTurn {
		delivery = InterruptDeliveryInterruptedCurrentTurn
	}
	return InterruptWorkerResult{
		Event:       interruptedAPIEvent,
		Interrupted: interrupted,
		Delivery:    delivery,
	}, nil
}
