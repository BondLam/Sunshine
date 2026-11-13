package channel

import (
	"context"
	"testing"

	"github.com/linrong007/Sunshine/internal/channel/store"
)

func TestSpawnSendReadInterruptDoneLoop(t *testing.T) {
	testSetup(t)
	ctx := t.Context()
	testCreate(t, "demo")
	runtime := NewStubRuntime()

	// spawn：空闲等待第一条 send --to 才工作，不立即派活。
	state := testSpawn(t, ctx, "demo", "w1", runtime)
	if state.Lifecycle != WorkerLifecycleRunning || state.Activity != WorkerActivityIdle {
		t.Fatalf("spawn 后应为 running/idle，得到 %s/%s", state.Lifecycle, state.Activity)
	}
	if state.Terminal || state.PendingMessageCount != 0 {
		t.Fatalf("spawn 后应为非终态且无待处理消息：%+v", state)
	}
	if state.InboxPolicy != InboxPolicyExplicitOnly {
		t.Errorf("默认收件箱策略应为 explicitOnly，得到 %q", state.InboxPolicy)
	}
	if state.Provider != "claude" {
		t.Errorf("provider 应为 claude，得到 %q", state.Provider)
	}
	if state.Agent != "" {
		t.Errorf("未传 agent 时应为空，得到 %q", state.Agent)
	}
	events, _ := ReadEvents(ReadEventsOptions{ChannelAddress: testAddr("demo")})
	if len(events) != 2 {
		t.Fatalf("spawn 后应只有 create + spawned 两条事件，得到 %d", len(events))
	}
	if runtime.StartCount() != 1 {
		t.Fatalf("runtime.Start 应被调 1 次，实际 %d", runtime.StartCount())
	}
	spec, err := runtime.LastStart()
	if err != nil {
		t.Fatalf("取 Start 参数失败：%v", err)
	}
	if spec.WorkerID != "w1" || spec.SystemPrompt != "你是测试 worker。" {
		t.Errorf("Start 收到的 spec 不正确：%+v", spec)
	}

	// send --to：worker 收到一条待处理消息。
	msg, err := SendMessage(SendMessageOptions{
		ChannelAddress: testAddr("demo"), By: "main", Text: "实现登录", To: []string{"w1"},
	})
	if err != nil {
		t.Fatalf("发送失败：%v", err)
	}
	workers, err := ListWorkers(ListWorkersOptions{ChannelAddress: testAddr("demo")})
	if err != nil {
		t.Fatalf("列 worker 失败：%v", err)
	}
	if len(workers) != 1 || workers[0].PendingMessageCount != 1 {
		t.Fatalf("send 后待处理消息应为 1，得到 %+v", workers)
	}
	inbox, err := ReadWorkerInbox(ReadWorkerInboxOptions{
		ChannelAddress: testAddr("demo"), WorkerID: "w1",
	})
	if err != nil {
		t.Fatalf("读收件箱失败：%v", err)
	}
	if len(inbox) != 1 || inbox[0].Event.Text != "实现登录" || inbox[0].Seq != msg.Seq {
		t.Fatalf("收件箱内容不正确：%+v", inbox)
	}

	// worker 开始回合：turn_started 消费掉该消息。
	appendStoreEvent(t, "demo", store.AppendableEvent{
		Kind: store.KindTurnStarted, By: "w1",
		Extra: map[string]any{"worker": "w1", "turnId": "turn-1", "inputSeq": msg.Seq},
	})
	workers, _ = ListWorkers(ListWorkersOptions{ChannelAddress: testAddr("demo")})
	if workers[0].Activity != WorkerActivityMidTurn || workers[0].PendingMessageCount != 0 {
		t.Errorf("turn_started 后应为 mid-turn 且无待处理消息：%+v", workers[0])
	}
	if workers[0].ActiveTurnID != "turn-1" {
		t.Errorf("activeTurnId 应为 turn-1，得到 %q", workers[0].ActiveTurnID)
	}

	// 中断当前回合：runtime 被调一次，delivery 为 interrupted-current-turn。
	result, err := InterruptWorker(ctx, InterruptWorkerOptions{
		ChannelAddress: testAddr("demo"), By: "main", WorkerID: "w1",
		Reason: InterruptReasonUser, Message: "改一下需求",
	}, runtime)
	if err != nil {
		t.Fatalf("中断失败：%v", err)
	}
	if !result.Interrupted || result.Delivery != InterruptDeliveryInterruptedCurrentTurn {
		t.Errorf("中断结果不正确：%+v", result)
	}
	if result.Event.Kind != EventKindInterrupted || result.Event.Method != "provider" || result.Event.Outcome != "interrupted" {
		t.Errorf("interrupted 事件不正确：%+v", result.Event)
	}
	if runtime.InterruptCount() != 1 {
		t.Errorf("runtime.Interrupt 应被调 1 次，实际 %d", runtime.InterruptCount())
	}
	workers, _ = ListWorkers(ListWorkersOptions{ChannelAddress: testAddr("demo")})
	if workers[0].Activity != WorkerActivityIdle {
		t.Errorf("中断后应回到 idle，得到 %s", workers[0].Activity)
	}

	// worker 完成：合成 done 事件使其终态。
	appendStoreEvent(t, "demo", store.AppendableEvent{
		Kind: store.KindDone, By: "supervisor:w1",
		Extra: map[string]any{"worker": "w1", "synthesized": true, "exit_code": 0},
	})
	workers, _ = ListWorkers(ListWorkersOptions{ChannelAddress: testAddr("demo")})
	if len(workers) != 0 {
		t.Errorf("默认列表不应含终态 worker，得到 %+v", workers)
	}
	workers, _ = ListWorkers(ListWorkersOptions{
		ChannelAddress: testAddr("demo"), IncludeTerminal: true,
	})
	if len(workers) != 1 || workers[0].Lifecycle != WorkerLifecycleDone || !workers[0].Terminal {
		t.Fatalf("终态列表不正确：%+v", workers)
	}
}

func TestRequestInterruptDoesNotTouchRuntime(t *testing.T) {
	testSetup(t)
	ctx := t.Context()
	testCreate(t, "demo")
	runtime := NewStubRuntime()
	testSpawn(t, ctx, "demo", "w1", runtime)

	ev, err := RequestInterrupt(InterruptWorkerOptions{
		ChannelAddress: testAddr("demo"), By: "main", WorkerID: "w1",
		Reason: InterruptReasonUser, Message: "停一下",
	})
	if err != nil {
		t.Fatalf("requestInterrupt 失败：%v", err)
	}
	if ev.Kind != EventKindInterruptRequested || ev.Worker != "w1" {
		t.Errorf("interrupt_requested 事件不正确：%+v", ev)
	}
	if runtime.InterruptCount() != 0 {
		t.Errorf("requestInterrupt 不应碰 runtime，实际调了 %d 次", runtime.InterruptCount())
	}
	// 持久化意图不改 worker 状态。
	workers, _ := ListWorkers(ListWorkersOptions{ChannelAddress: testAddr("demo")})
	if workers[0].Activity != WorkerActivityIdle || workers[0].Lifecycle != WorkerLifecycleRunning {
		t.Errorf("requestInterrupt 不应改变 worker 状态：%+v", workers[0])
	}
}

func TestInterruptWorkerUnknownAndTerminal(t *testing.T) {
	testSetup(t)
	ctx := t.Context()
	testCreate(t, "demo")
	runtime := NewStubRuntime()
	testSpawn(t, ctx, "demo", "w1", runtime)

	// 未知 worker：只追加 interrupt_requested，不碰 runtime。
	result, err := InterruptWorker(ctx, InterruptWorkerOptions{
		ChannelAddress: testAddr("demo"), By: "main", WorkerID: "ghost",
	}, runtime)
	if err != nil {
		t.Fatalf("中断未知 worker 失败：%v", err)
	}
	if result.Delivery != InterruptDeliveryWorkerUnknown || result.Interrupted {
		t.Errorf("未知 worker 的中断结果不正确：%+v", result)
	}
	if result.Event.Kind != EventKindInterruptRequested {
		t.Errorf("未知 worker 应只追加 interrupt_requested，得到 %s", result.Event.Kind)
	}
	if runtime.InterruptCount() != 0 {
		t.Errorf("未知 worker 不应调 runtime")
	}

	// 终态 worker：同样只追加 interrupt_requested。
	appendStoreEvent(t, "demo", store.AppendableEvent{
		Kind: store.KindKilled, By: "supervisor:w1",
		Extra: map[string]any{"worker": "w1", "reason": "explicit-kill"},
	})
	result, err = InterruptWorker(ctx, InterruptWorkerOptions{
		ChannelAddress: testAddr("demo"), By: "main", WorkerID: "w1",
	}, runtime)
	if err != nil {
		t.Fatalf("中断终态 worker 失败：%v", err)
	}
	if result.Delivery != InterruptDeliveryWorkerTerminal || result.Interrupted {
		t.Errorf("终态 worker 的中断结果不正确：%+v", result)
	}
	if runtime.InterruptCount() != 0 {
		t.Errorf("终态 worker 不应调 runtime")
	}

	// 无 runtime 时仍追加 interrupted 事件（method none / outcome unsupported）。
	result, err = InterruptWorker(ctx, InterruptWorkerOptions{
		ChannelAddress: testAddr("demo"), By: "main", WorkerID: "w1",
	}, nil)
	if err != nil {
		t.Fatalf("无 runtime 中断终态 worker 失败：%v", err)
	}
	if result.Delivery != InterruptDeliveryWorkerTerminal {
		t.Errorf("无 runtime 时终态 worker 仍应返回 worker-terminal，得到 %+v", result)
	}
}

func TestInterruptWorkerNoRuntimeIdle(t *testing.T) {
	testSetup(t)
	ctx := t.Context()
	testCreate(t, "demo")
	runtime := NewStubRuntime()
	testSpawn(t, ctx, "demo", "w1", runtime)

	result, err := InterruptWorker(ctx, InterruptWorkerOptions{
		ChannelAddress: testAddr("demo"), By: "main", WorkerID: "w1",
	}, nil)
	if err != nil {
		t.Fatalf("中断失败：%v", err)
	}
	if result.Interrupted || result.Delivery != InterruptDeliveryNoActiveTurn {
		t.Errorf("空闲 worker 无 runtime 中断结果不正确：%+v", result)
	}
	if result.Event.Kind != EventKindInterrupted || result.Event.Method != "none" || result.Event.Outcome != "unsupported" {
		t.Errorf("interrupted 事件不正确：%+v", result.Event)
	}
}

func TestInterruptWorkerRuntimeFailure(t *testing.T) {
	testSetup(t)
	ctx := t.Context()
	testCreate(t, "demo")
	runtime := NewStubRuntime()
	runtime.FailInterrupt = errTestInterrupt
	testSpawn(t, ctx, "demo", "w1", runtime)

	result, err := InterruptWorker(ctx, InterruptWorkerOptions{
		ChannelAddress: testAddr("demo"), By: "main", WorkerID: "w1",
	}, runtime)
	if err != nil {
		t.Fatalf("中断失败：%v", err)
	}
	if result.Interrupted {
		t.Errorf("runtime 失败时 interrupted 应为 false")
	}
	if result.Event.Outcome != "failed" || result.Event.Message != errTestInterrupt.Error() {
		t.Errorf("失败的 interrupted 事件不正确：%+v", result.Event)
	}
}

func TestSpawnValidation(t *testing.T) {
	testSetup(t)
	ctx := context.Background()
	testCreate(t, "demo")
	runtime := NewStubRuntime()
	base := SpawnWorkerOptions{
		ChannelAddress: testAddr("demo"), By: "main", WorkerID: "w1",
		Provider: "claude", SystemPrompt: "提示词",
	}
	// 非法 worker 名。
	bad := base
	bad.WorkerID = "../x"
	if _, err := SpawnWorker(ctx, bad, runtime); err == nil {
		t.Errorf("非法 worker 名应报错")
	}
	// 非法 provider。
	bad = base
	bad.Provider = "gpt"
	if _, err := SpawnWorker(ctx, bad, runtime); err == nil {
		t.Errorf("非法 provider 应报错")
	}
	// 缺 systemPrompt。
	bad = base
	bad.SystemPrompt = ""
	if _, err := SpawnWorker(ctx, bad, runtime); err == nil {
		t.Errorf("缺 systemPrompt 应报错")
	}
	// 非法收件箱策略。
	bad = base
	bad.InboxPolicy = "all"
	if _, err := SpawnWorker(ctx, bad, runtime); err == nil {
		t.Errorf("非法收件箱策略应报错")
	}
	// 无 runtime。
	if _, err := SpawnWorker(ctx, base, nil); err == nil {
		t.Errorf("无 runtime 应报错")
	}
	// runtime 启动失败。
	failing := NewStubRuntime()
	failing.FailStart = errTestSpawn
	if _, err := SpawnWorker(ctx, base, failing); err == nil {
		t.Errorf("runtime 启动失败应报错")
	}
	// 不存在的 channel。
	if _, err := SpawnWorker(ctx, SpawnWorkerOptions{
		ChannelAddress: testAddr("nope"), By: "main", WorkerID: "w1",
		Provider: "claude", SystemPrompt: "提示词",
	}, runtime); err == nil {
		t.Errorf("不存在的 channel 应报错")
	}
	// broadcastAndExplicit 策略能透传。
	state, err := SpawnWorker(ctx, SpawnWorkerOptions{
		ChannelAddress: testAddr("demo"), By: "main", WorkerID: "w2",
		Provider: "codex", Agent: "implement", SystemPrompt: "提示词",
		InboxPolicy: "broadcastAndExplicit",
	}, runtime)
	if err != nil {
		t.Fatalf("spawn w2 失败：%v", err)
	}
	if state.InboxPolicy != InboxPolicyBroadcastAndExplicit || state.Provider != "codex" || state.Agent != "implement" {
		t.Errorf("w2 状态不正确：%+v", state)
	}
}

func TestInboxPolicies(t *testing.T) {
	testSetup(t)
	ctx := t.Context()
	testCreate(t, "demo")
	runtime := NewStubRuntime()
	testSpawn(t, ctx, "demo", "w1", runtime)
	if _, err := SpawnWorker(ctx, SpawnWorkerOptions{
		ChannelAddress: testAddr("demo"), By: "main", WorkerID: "w2",
		Provider: "claude", SystemPrompt: "提示词", InboxPolicy: "broadcastAndExplicit",
	}, runtime); err != nil {
		t.Fatalf("spawn w2 失败：%v", err)
	}
	// 广播：explicitOnly 的 w1 收不到，broadcastAndExplicit 的 w2 收到。
	if _, err := SendMessage(SendMessageOptions{
		ChannelAddress: testAddr("demo"), By: "main", Text: "广播",
	}); err != nil {
		t.Fatalf("广播失败：%v", err)
	}
	inbox1, _ := ReadWorkerInbox(ReadWorkerInboxOptions{ChannelAddress: testAddr("demo"), WorkerID: "w1"})
	inbox2, _ := ReadWorkerInbox(ReadWorkerInboxOptions{ChannelAddress: testAddr("demo"), WorkerID: "w2"})
	if len(inbox1) != 0 {
		t.Errorf("explicitOnly 不应收到广播，得到 %d 条", len(inbox1))
	}
	if len(inbox2) != 1 {
		t.Errorf("broadcastAndExplicit 应收到广播，得到 %d 条", len(inbox2))
	}
	// worker 永不消费自己发的消息。
	if _, err := SendMessage(SendMessageOptions{
		ChannelAddress: testAddr("demo"), By: "w2", Text: "自己发的", To: []string{"w2"},
	}); err != nil {
		t.Fatalf("发送失败：%v", err)
	}
	inbox2, _ = ReadWorkerInbox(ReadWorkerInboxOptions{ChannelAddress: testAddr("demo"), WorkerID: "w2"})
	if len(inbox2) != 1 {
		t.Errorf("自己发的消息不应进收件箱，得到 %d 条", len(inbox2))
	}
	// 读不存在 worker 的收件箱应报错；读终态 worker 的收件箱应报错。
	if _, err := ReadWorkerInbox(ReadWorkerInboxOptions{ChannelAddress: testAddr("demo"), WorkerID: "ghost"}); err == nil {
		t.Errorf("读不存在 worker 的收件箱应报错")
	}
	appendStoreEvent(t, "demo", store.AppendableEvent{
		Kind: store.KindKilled, By: "supervisor:w1",
		Extra: map[string]any{"worker": "w1", "reason": "explicit-kill"},
	})
	if _, err := ReadWorkerInbox(ReadWorkerInboxOptions{ChannelAddress: testAddr("demo"), WorkerID: "w1"}); err == nil {
		t.Errorf("读终态 worker 的收件箱应报错")
	}
	inbox1, err := ReadWorkerInbox(ReadWorkerInboxOptions{
		ChannelAddress: testAddr("demo"), WorkerID: "w1", IncludeTerminal: true,
	})
	if err != nil {
		t.Fatalf("IncludeTerminal 读终态收件箱失败：%v", err)
	}
	if len(inbox1) != 0 {
		t.Errorf("终态 worker 的待处理消息应为 0，得到 %d 条", len(inbox1))
	}
}
