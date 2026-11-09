package channel

import (
	"testing"

	"github.com/linrong1982/Sunshine/internal/channel/store"
)

func TestCreateChannel(t *testing.T) {
	testSetup(t)
	ev := testCreate(t, "demo")
	if ev.Kind != EventKindCreate || ev.Seq != 1 {
		t.Fatalf("create 事件不正确：%+v", ev)
	}
	if ev.Type != "chat" {
		t.Errorf("默认类型应为 chat，得到 %q", ev.Type)
	}
	// 重复创建应报错。
	if _, err := CreateChannel(CreateChannelOptions{ChannelAddress: testAddr("demo"), By: "main"}); err == nil {
		t.Errorf("重复创建应报错")
	}
	// --force 可覆盖。
	ev, err := CreateChannel(CreateChannelOptions{ChannelAddress: testAddr("demo"), By: "main", Force: true})
	if err != nil {
		t.Fatalf("--force 重建失败：%v", err)
	}
	if ev.Seq != 1 {
		t.Errorf("--force 重建后 seq 应从 1 开始，得到 %d", ev.Seq)
	}
	// forum 类型与旧值校验。
	if _, err := CreateChannel(CreateChannelOptions{ChannelAddress: testAddr("forum1"), By: "main", Type: "forum"}); err != nil {
		t.Fatalf("建 forum channel 失败：%v", err)
	}
	if _, err := CreateChannel(CreateChannelOptions{ChannelAddress: testAddr("bad"), By: "main", Type: "thread"}); err == nil {
		t.Errorf("旧类型 thread 应报错提示用 forum")
	}
	if _, err := CreateChannel(CreateChannelOptions{ChannelAddress: testAddr("bad2"), By: "main", Type: "wiki"}); err == nil {
		t.Errorf("非法类型应报错")
	}
	// global 作用域。
	ev, err = CreateChannel(CreateChannelOptions{
		ChannelAddress: ChannelAddress{Channel: "g1", Scope: "global"},
		By:             "main",
	})
	if err != nil {
		t.Fatalf("建 global channel 失败：%v", err)
	}
	ref, err := ResolveChannel(ChannelAddress{Channel: "g1", Scope: "global"})
	if err != nil {
		t.Fatalf("解析 global channel 失败：%v", err)
	}
	if ref.Project != GlobalProjectKey || ref.Scope != ScopeGlobal {
		t.Errorf("global 解析错误：%+v", ref)
	}
	// 不存在的 channel 解析应报错。
	if _, err := ResolveChannel(testAddr("nope")); err == nil {
		t.Errorf("解析不存在的 channel 应报错")
	}
}

func TestCreateChannelWithMeta(t *testing.T) {
	testSetup(t)
	ev, err := CreateChannel(CreateChannelOptions{
		ChannelAddress: testAddr("meta"),
		By:             "main",
		Task:           "11-09-demo",
		Project:        "demo-proj",
		Labels:         []string{"a", "b"},
		Description:    "说明文字",
		Ephemeral:      true,
		Context: []ContextEntry{
			{Type: "file", Path: "/tmp/spec.md"},
			{Type: "raw", Text: "注意点"},
		},
	})
	if err != nil {
		t.Fatalf("建 channel 失败：%v", err)
	}
	if ev.Task != "11-09-demo" || ev.Description != "说明文字" || !ev.Ephemeral {
		t.Errorf("create 事件元数据不正确：%+v", ev)
	}
	if len(ev.Labels) != 2 || len(ev.Context) != 2 {
		t.Errorf("create 事件 labels/context 不正确：%+v", ev)
	}
	meta, err := ReadChannelMetadata(testAddr("meta"))
	if err != nil {
		t.Fatalf("读元数据失败：%v", err)
	}
	if meta.Description != "说明文字" || len(meta.Labels) != 2 || len(meta.Context) != 2 {
		t.Errorf("元数据投影不正确：%+v", meta)
	}
}

func TestSendMessageAndRead(t *testing.T) {
	testSetup(t)
	testCreate(t, "demo")
	msg, err := SendMessage(SendMessageOptions{
		ChannelAddress: testAddr("demo"), By: "main", Text: "你好",
	})
	if err != nil {
		t.Fatalf("发消息失败：%v", err)
	}
	if msg.Kind != EventKindMessage || msg.Text != "你好" || len(msg.To) != 0 {
		t.Errorf("消息事件不正确：%+v", msg)
	}
	directed, err := SendMessage(SendMessageOptions{
		ChannelAddress: testAddr("demo"), By: "main", Text: "给 w1", To: []string{"w1"},
	})
	if err != nil {
		t.Fatalf("定向发消息失败：%v", err)
	}
	if len(directed.To) != 1 || directed.To[0] != "w1" {
		t.Errorf("定向消息 to 不正确：%+v", directed)
	}
	events, err := ReadEvents(ReadEventsOptions{ChannelAddress: testAddr("demo")})
	if err != nil {
		t.Fatalf("读事件失败：%v", err)
	}
	if len(events) != 3 {
		t.Fatalf("应有 3 条事件，得到 %d", len(events))
	}
	// 分页：afterSeq + limit。
	page, err := ReadEvents(ReadEventsOptions{
		ChannelAddress: testAddr("demo"),
		Pagination:     Pagination{AfterSeq: int64Ptr(1), Limit: intPtr(1)},
	})
	if err != nil {
		t.Fatalf("分页读失败：%v", err)
	}
	if len(page) != 1 || page[0].Seq != 2 {
		t.Errorf("分页结果不正确：%+v", page)
	}
	// beforeSeq：取 seq < 3 的最新 1 条。
	page, err = ReadEvents(ReadEventsOptions{
		ChannelAddress: testAddr("demo"),
		Pagination:     Pagination{BeforeSeq: int64Ptr(3), Limit: intPtr(1)},
	})
	if err != nil {
		t.Fatalf("分页读失败：%v", err)
	}
	if len(page) != 1 || page[0].Seq != 2 {
		t.Errorf("beforeSeq 分页结果不正确：%+v", page)
	}
	// afterSeq 与 beforeSeq 同传应报错。
	if _, err := ReadEvents(ReadEventsOptions{
		ChannelAddress: testAddr("demo"),
		Pagination:     Pagination{AfterSeq: int64Ptr(1), BeforeSeq: int64Ptr(3)},
	}); err == nil {
		t.Errorf("afterSeq 与 beforeSeq 同传应报错")
	}
}

func TestSendMessageIdempotency(t *testing.T) {
	testSetup(t)
	testCreate(t, "demo")
	opts := SendMessageOptions{
		ChannelAddress: testAddr("demo"), By: "main", Text: "幂等",
		IdempotencyKey: "key-1",
	}
	first, err := SendMessage(opts)
	if err != nil {
		t.Fatalf("第一次发送失败：%v", err)
	}
	second, err := SendMessage(opts)
	if err != nil {
		t.Fatalf("第二次发送失败：%v", err)
	}
	if first.Seq != second.Seq {
		t.Errorf("同 key 同 kind 应返回已有事件，seq %d vs %d", first.Seq, second.Seq)
	}
	events, _ := ReadEvents(ReadEventsOptions{ChannelAddress: testAddr("demo")})
	if len(events) != 2 {
		t.Errorf("幂等重放不应多写事件，得到 %d 条", len(events))
	}
	// 同 key 不同 kind 应报错。
	if _, err := store.AppendEvent(testRef("demo"), store.AppendableEvent{
		Kind: store.KindJoin, By: "main", IdempotencyKey: "key-1",
	}); err == nil {
		t.Errorf("同 key 不同 kind 应报错")
	}
}

func TestDeliveryModes(t *testing.T) {
	testSetup(t)
	ctx := t.Context()
	testCreate(t, "demo")
	runtime := NewStubRuntime()
	testSpawn(t, ctx, "demo", "w1", runtime)

	// appendOnly：目标不存在也不报错、不产生 undeliverable。
	if _, err := SendMessage(SendMessageOptions{
		ChannelAddress: testAddr("demo"), By: "main", Text: "hi", To: []string{"ghost"},
		DeliveryMode: "appendOnly",
	}); err != nil {
		t.Fatalf("appendOnly 发送失败：%v", err)
	}
	if got := countKind(t, "demo", EventKindUndeliverable); got != 0 {
		t.Errorf("appendOnly 不应产生 undeliverable，得到 %d", got)
	}

	// requireKnownWorker：未知目标产生 worker-unknown。
	if _, err := SendMessage(SendMessageOptions{
		ChannelAddress: testAddr("demo"), By: "main", Text: "hi", To: []string{"ghost"},
		DeliveryMode: "requireKnownWorker",
	}); err != nil {
		t.Fatalf("requireKnownWorker 发送失败：%v", err)
	}
	events, _ := ReadEvents(ReadEventsOptions{ChannelAddress: testAddr("demo")})
	var und *ChannelEvent
	for i := range events {
		if events[i].Kind == EventKindUndeliverable {
			und = &events[i]
		}
	}
	if und == nil {
		t.Fatalf("应产生 undeliverable 事件")
	}
	if und.TargetWorker != "ghost" || und.Reason != "worker-unknown" {
		t.Errorf("undeliverable 内容不正确：%+v", und)
	}

	// 广播在严格模式下永不产生 undeliverable。
	before := countKind(t, "demo", EventKindUndeliverable)
	if _, err := SendMessage(SendMessageOptions{
		ChannelAddress: testAddr("demo"), By: "main", Text: "广播",
		DeliveryMode: "requireRunningWorker",
	}); err != nil {
		t.Fatalf("广播发送失败：%v", err)
	}
	if got := countKind(t, "demo", EventKindUndeliverable); got != before {
		t.Errorf("广播不应产生 undeliverable，之前 %d、之后 %d", before, got)
	}

	// 杀掉 w1 后 requireRunningWorker 产生 worker-terminal。
	appendStoreEvent(t, "demo", store.AppendableEvent{
		Kind: store.KindKilled, By: "supervisor:w1",
		Extra: map[string]any{"worker": "w1", "reason": "explicit-kill"},
	})
	if _, err := SendMessage(SendMessageOptions{
		ChannelAddress: testAddr("demo"), By: "main", Text: "hi", To: []string{"w1"},
		DeliveryMode: "requireRunningWorker",
	}); err != nil {
		t.Fatalf("requireRunningWorker 发送失败：%v", err)
	}
	events, _ = ReadEvents(ReadEventsOptions{ChannelAddress: testAddr("demo")})
	found := false
	for _, ev := range events {
		if ev.Kind == EventKindUndeliverable && ev.TargetWorker == "w1" && ev.Reason == "worker-terminal" {
			found = true
		}
	}
	if !found {
		t.Errorf("终态 worker 应产生 worker-terminal 的 undeliverable")
	}

	// 非法投递模式应报错。
	if _, err := SendMessage(SendMessageOptions{
		ChannelAddress: testAddr("demo"), By: "main", Text: "hi",
		DeliveryMode: "strict",
	}); err == nil {
		t.Errorf("非法投递模式应报错")
	}
}

func countKind(t *testing.T, name string, kind EventKind) int {
	t.Helper()
	events, err := ReadEvents(ReadEventsOptions{ChannelAddress: testAddr(name)})
	if err != nil {
		t.Fatalf("读事件失败：%v", err)
	}
	n := 0
	for _, ev := range events {
		if ev.Kind == kind {
			n++
		}
	}
	return n
}
