package channel

import "testing"

// 纯函数测试：只给事件切片，不碰文件。

func TestBuildThreadAliasResolverChains(t *testing.T) {
	events := []ChannelEvent{
		{Seq: 1, Kind: "thread", By: "main", Action: "opened", Thread: "a"},
		{Seq: 2, Kind: "thread", By: "main", Action: "rename", Thread: "a", NewThread: "b"},
		{Seq: 3, Kind: "thread", By: "main", Action: "rename", Thread: "b", NewThread: "c"},
	}
	r := BuildThreadAliasResolver(events)
	for _, key := range []string{"a", "b", "c"} {
		if got := r.Resolve(key); got != "c" {
			t.Errorf("改名链 %q 应解析到 c，得到 %q", key, got)
		}
	}
	if got := r.Resolve("unknown"); got != "unknown" {
		t.Errorf("未知键应返回原键，得到 %q", got)
	}
	aliases := r.AliasesFor("c")
	if len(aliases) != 2 || aliases[0] != "a" || aliases[1] != "b" {
		t.Errorf("c 的别名应为 [a b]，得到 %v", aliases)
	}
	if got := r.AliasesFor("a"); len(got) != 0 {
		t.Errorf("a 不是当前键，别名应为空，得到 %v", got)
	}
}

func TestReduceThreadsContextAndSorting(t *testing.T) {
	events := []ChannelEvent{
		{Seq: 1, Kind: "thread", By: "main", Action: "opened", Thread: "t1", Title: strptr("甲"), TS: "2026-11-09T10:00:00Z"},
		{Seq: 2, Kind: "context", By: "main", Action: "add", Thread: "t1", Target: "thread",
			Context: []ContextEntry{{Type: "file", Path: "/tmp/spec.md"}}, TS: "2026-11-09T10:01:00Z"},
		{Seq: 3, Kind: "thread", By: "main", Action: "opened", Thread: "t2", Title: strptr("乙"), TS: "2026-11-09T10:02:00Z"},
		{Seq: 4, Kind: "context", By: "main", Action: "delete", Thread: "t1", Target: "thread",
			Context: []ContextEntry{{Type: "file", Path: "/tmp/spec.md"}}, TS: "2026-11-09T10:03:00Z"},
	}
	states := ReduceThreads(events)
	if len(states) != 2 {
		t.Fatalf("期望 2 个 thread，得到 %d", len(states))
	}
	// 按更新时间倒序：t1 最后更新在 10:03，排第一；t2 排第二。
	if states[0].ID != "t1" || states[1].ID != "t2" {
		t.Fatalf("排序不对：%v", states)
	}
	// t1 的上下文被删后应为空。
	if len(states[0].Context) != 0 {
		t.Errorf("t1 上下文删除后应为空，得到 %v", states[0].Context)
	}
	if states[0].UpdatedAt != "2026-11-09T10:03:00Z" {
		t.Errorf("t1 更新时间不对：%q", states[0].UpdatedAt)
	}
}

func TestReduceThreadsOpenedDefaults(t *testing.T) {
	// opened 不带 status 时默认 open；processed 不带 status 时置 processed；
	// status 动作不带 status 时保持原状。
	events := []ChannelEvent{
		{Seq: 1, Kind: "thread", By: "main", Action: "opened", Thread: "t1", TS: "2026-11-09T10:00:00Z"},
		{Seq: 2, Kind: "thread", By: "main", Action: "status", Thread: "t1", TS: "2026-11-09T10:01:00Z"},
		{Seq: 3, Kind: "thread", By: "main", Action: "processed", Thread: "t1", TS: "2026-11-09T10:02:00Z"},
	}
	states := ReduceThreads(events)
	if len(states) != 1 {
		t.Fatalf("期望 1 个 thread，得到 %d", len(states))
	}
	if states[0].Status != "processed" || !states[0].Processed {
		t.Fatalf("processed 后状态应为 processed：%+v", states[0])
	}
	if states[0].Labels == nil || states[0].Assignees == nil {
		t.Fatalf("标签/负责人应为非空数组：%+v", states[0])
	}
}

func TestCollectThreadTimelineIncludesContext(t *testing.T) {
	events := []ChannelEvent{
		{Seq: 1, Kind: "thread", By: "main", Action: "opened", Thread: "t1"},
		{Seq: 2, Kind: "thread", By: "main", Action: "opened", Thread: "t2"},
		{Seq: 3, Kind: "context", By: "main", Action: "add", Thread: "t1", Target: "thread",
			Context: []ContextEntry{{Type: "raw", Text: "背景"}}},
		{Seq: 4, Kind: "context", By: "main", Action: "add", Target: "channel",
			Context: []ContextEntry{{Type: "raw", Text: "频道级"}}},
		{Seq: 5, Kind: "thread", By: "main", Action: "comment", Thread: "t1"},
	}
	timeline := CollectThreadTimeline(events, "t1")
	if len(timeline) != 3 {
		t.Fatalf("t1 时间线应为 3 条（opened/context/comment），得到 %d", len(timeline))
	}
	if timeline[0].Seq != 1 || timeline[1].Seq != 3 || timeline[2].Seq != 5 {
		t.Fatalf("时间线 seq 应为 1、3、5，得到 %v", timeline)
	}
}
