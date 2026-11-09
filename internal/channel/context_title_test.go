package channel

import (
	"testing"
)

func TestChannelContext(t *testing.T) {
	testSetup(t)
	testCreate(t, "demo")

	// 校验：相对路径的 file 条目应报错；空 raw 应报错；空列表应报错。
	if _, err := AddChannelContext(ContextMutationOptions{
		ChannelAddress: testAddr("demo"), By: "main",
		Context: []ContextEntry{{Type: "file", Path: "relative/path.md"}},
	}); err == nil {
		t.Errorf("相对路径的 file 条目应报错")
	}
	if _, err := AddChannelContext(ContextMutationOptions{
		ChannelAddress: testAddr("demo"), By: "main",
		Context: []ContextEntry{{Type: "raw", Text: "   "}},
	}); err == nil {
		t.Errorf("空 raw 文本应报错")
	}
	if _, err := AddChannelContext(ContextMutationOptions{
		ChannelAddress: testAddr("demo"), By: "main",
		Context: []ContextEntry{},
	}); err == nil {
		t.Errorf("空 context 应报错")
	}

	// 增：两条；查：两条；删一条；查：一条；删不存在的条目不报错且不变。
	entries := []ContextEntry{
		{Type: "file", Path: "/tmp/spec.md"},
		{Type: "raw", Text: "注意点"},
	}
	ev, err := AddChannelContext(ContextMutationOptions{
		ChannelAddress: testAddr("demo"), By: "main", Context: entries,
	})
	if err != nil {
		t.Fatalf("加上下文失败：%v", err)
	}
	if ev.Kind != EventKindContext || ev.Target != "channel" || ev.Action != "add" {
		t.Errorf("context 事件不正确：%+v", ev)
	}
	listed, err := ListChannelContext(testAddr("demo"))
	if err != nil {
		t.Fatalf("列上下文失败：%v", err)
	}
	if len(listed) != 2 {
		t.Fatalf("应列出 2 条上下文，得到 %+v", listed)
	}
	if _, err := DeleteChannelContext(ContextMutationOptions{
		ChannelAddress: testAddr("demo"), By: "main",
		Context: []ContextEntry{{Type: "raw", Text: "注意点"}},
	}); err != nil {
		t.Fatalf("删上下文失败：%v", err)
	}
	listed, _ = ListChannelContext(testAddr("demo"))
	if len(listed) != 1 || listed[0].Path != "/tmp/spec.md" {
		t.Errorf("删除后应剩 1 条 file 条目，得到 %+v", listed)
	}
	if _, err := DeleteChannelContext(ContextMutationOptions{
		ChannelAddress: testAddr("demo"), By: "main",
		Context: []ContextEntry{{Type: "raw", Text: "不存在"}},
	}); err != nil {
		t.Fatalf("删不存在的条目不应报错：%v", err)
	}
	listed, _ = ListChannelContext(testAddr("demo"))
	if len(listed) != 1 {
		t.Errorf("删不存在的条目不应改变列表，得到 %+v", listed)
	}
	// BuildContextEntries 辅助函数。
	built, err := BuildContextEntries([]string{"/tmp/a.md"}, []string{"文本"})
	if err != nil || len(built) != 2 {
		t.Errorf("BuildContextEntries 不正确：%v %+v", err, built)
	}
	if _, err := BuildContextEntries([]string{"rel.md"}, nil); err == nil {
		t.Errorf("BuildContextEntries 相对路径应报错")
	}
}

func TestThreadContext(t *testing.T) {
	testSetup(t)
	// forum channel + 一个 thread（直接追加 thread 事件模拟开贴）。
	if _, err := CreateChannel(CreateChannelOptions{
		ChannelAddress: testAddr("forum"), By: "main", Type: "forum",
	}); err != nil {
		t.Fatalf("建 forum channel 失败：%v", err)
	}
	appendStoreEvent(t, "forum", storeAppendableThread("t1"))

	// 未知 thread 加上下文应报错；列未知 thread 返回空列表。
	if _, err := AddThreadContext(ThreadContextMutationOptions{
		ContextMutationOptions: ContextMutationOptions{
			ChannelAddress: testAddr("forum"), By: "main",
			Context: []ContextEntry{{Type: "raw", Text: "x"}},
		},
		Thread: "nope",
	}); err == nil {
		t.Errorf("未知 thread 加上下文应报错")
	}
	listed, err := ListThreadContext(ThreadContextListOptions{
		ChannelAddress: testAddr("forum"), Thread: "nope",
	})
	if err != nil {
		t.Fatalf("列未知 thread 上下文失败：%v", err)
	}
	if len(listed) != 0 {
		t.Errorf("未知 thread 应返回空列表，得到 %+v", listed)
	}

	// 增删查闭环。
	if _, err := AddThreadContext(ThreadContextMutationOptions{
		ContextMutationOptions: ContextMutationOptions{
			ChannelAddress: testAddr("forum"), By: "main",
			Context: []ContextEntry{
				{Type: "file", Path: "/tmp/t1.md"},
				{Type: "raw", Text: "背景"},
			},
		},
		Thread: "t1",
	}); err != nil {
		t.Fatalf("加 thread 上下文失败：%v", err)
	}
	listed, err = ListThreadContext(ThreadContextListOptions{
		ChannelAddress: testAddr("forum"), Thread: "t1",
	})
	if err != nil {
		t.Fatalf("列 thread 上下文失败：%v", err)
	}
	if len(listed) != 2 {
		t.Fatalf("应列出 2 条 thread 上下文，得到 %+v", listed)
	}
	if _, err := DeleteThreadContext(ThreadContextMutationOptions{
		ContextMutationOptions: ContextMutationOptions{
			ChannelAddress: testAddr("forum"), By: "main",
			Context: []ContextEntry{{Type: "raw", Text: "背景"}},
		},
		Thread: "t1",
	}); err != nil {
		t.Fatalf("删 thread 上下文失败：%v", err)
	}
	listed, _ = ListThreadContext(ThreadContextListOptions{
		ChannelAddress: testAddr("forum"), Thread: "t1",
	})
	if len(listed) != 1 || listed[0].Path != "/tmp/t1.md" {
		t.Errorf("删除后应剩 1 条，得到 %+v", listed)
	}
	// channel 级上下文与 thread 级互不干扰。
	channelCtx, _ := ListChannelContext(testAddr("forum"))
	if len(channelCtx) != 0 {
		t.Errorf("forum channel 级上下文应为空，得到 %+v", channelCtx)
	}
}

func TestChannelTitle(t *testing.T) {
	testSetup(t)
	testCreate(t, "demo")

	// 空标题应报错。
	if _, err := SetChannelTitle(SetChannelTitleOptions{
		ChannelAddress: testAddr("demo"), By: "main", Title: "",
	}); err == nil {
		t.Errorf("空标题应报错")
	}
	ev, err := SetChannelTitle(SetChannelTitleOptions{
		ChannelAddress: testAddr("demo"), By: "main", Title: "登录重构",
	})
	if err != nil {
		t.Fatalf("设标题失败：%v", err)
	}
	if ev.Kind != EventKindChannel || ev.Action != "title" || ev.Title == nil || *ev.Title != "登录重构" {
		t.Errorf("标题事件不正确：%+v", ev)
	}
	meta, err := ReadChannelMetadata(testAddr("demo"))
	if err != nil {
		t.Fatalf("读元数据失败：%v", err)
	}
	if meta.Title != "登录重构" {
		t.Errorf("标题投影不正确：%+v", meta)
	}
	// 清除标题后投影为空。
	if _, err := ClearChannelTitle(ClearChannelTitleOptions{
		ChannelAddress: testAddr("demo"), By: "main",
	}); err != nil {
		t.Fatalf("清除标题失败：%v", err)
	}
	meta, _ = ReadChannelMetadata(testAddr("demo"))
	if meta.Title != "" {
		t.Errorf("清除后标题应为空，得到 %q", meta.Title)
	}
}

func TestChannelEventJSON(t *testing.T) {
	// to 接受字符串或数组；title 区分 null 与缺失。
	var ev ChannelEvent
	if err := jsonUnmarshal([]byte(`{"seq":1,"ts":"2026-11-09T00:00:00Z","kind":"message","by":"main","to":"w1","text":"hi"}`), &ev); err != nil {
		t.Fatalf("解析失败：%v", err)
	}
	if len(ev.To) != 1 || ev.To[0] != "w1" {
		t.Errorf("字符串 to 应归一化为数组，得到 %+v", ev.To)
	}
	var ev2 ChannelEvent
	if err := jsonUnmarshal([]byte(`{"seq":2,"ts":"2026-11-09T00:00:00Z","kind":"channel","by":"main","action":"title","title":null}`), &ev2); err != nil {
		t.Fatalf("解析失败：%v", err)
	}
	if !ev2.TitleNull || ev2.Title != nil {
		t.Errorf("title null 应置 TitleNull，得到 %+v", ev2)
	}
	data, err := jsonMarshal(ev2)
	if err != nil {
		t.Fatalf("编码失败：%v", err)
	}
	if !contains(string(data), `"title":null`) {
		t.Errorf("TitleNull 编码后应含 \"title\":null，得到 %s", string(data))
	}
	// 未知字段往返不丢失。
	var ev3 ChannelEvent
	if err := jsonUnmarshal([]byte(`{"seq":3,"ts":"t","kind":"message","by":"m","text":"x","custom":42}`), &ev3); err != nil {
		t.Fatalf("解析失败：%v", err)
	}
	if ev3.Extra["custom"] != float64(42) {
		t.Errorf("未知字段应进 Extra，得到 %+v", ev3.Extra)
	}
	data, _ = jsonMarshal(ev3)
	if !contains(string(data), `"custom":42`) {
		t.Errorf("未知字段编码后应保留，得到 %s", string(data))
	}
}
