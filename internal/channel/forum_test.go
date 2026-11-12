package channel

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/BondLam/Sunshine/internal/channel/store"
)

// forumFixture 在临时存储根里建一个 forum 频道（与一个 chat 频道），
// 工作目录切到临时项目目录（project 作用域解析用）。
type forumFixture struct {
	store   ForumStore
	root    string
	proj    string
	project string
}

func setupForumFixture(t *testing.T) forumFixture {
	t.Helper()
	root := t.TempDir()
	proj := t.TempDir()
	t.Setenv("SUNSHINE_CHANNEL_ROOT", root)
	t.Chdir(proj)
	fs := NewFileStore()
	project := store.ProjectKey(proj)
	seedChannel(t, fs, "board", project, "forum")
	seedChannel(t, fs, "talk", project, "chat")
	return forumFixture{store: fs, root: root, proj: proj, project: project}
}

// seedChannel 直接追加一条 create 事件建频道（channel create 是 B/C 组命令，
// 本组测试只用存储原语造数据）。
func seedChannel(t *testing.T, store ForumStore, name, project, typ string) {
	t.Helper()
	_, err := store.AppendEvent(name, project, map[string]any{
		"kind": "create",
		"by":   "main",
		"type": typ,
	})
	if err != nil {
		t.Fatalf("建频道 %s 失败：%v", name, err)
	}
}

func strptr(s string) *string { return &s }

func TestPostThreadFullFlow(t *testing.T) {
	fx := setupForumFixture(t)
	s := fx.store

	ev, err := PostThread(s, PostThreadOptions{
		Channel: "board", Scope: "project", By: "main", Action: "opened",
		Thread: "t1", Title: "第一主题", Description: "主题描述",
		Labels: []string{"bug"}, Assignees: []string{"alice"}, Origin: "cli",
	})
	if err != nil {
		t.Fatalf("opened 失败：%v", err)
	}
	if ev.Kind != "thread" || ev.Action != "opened" || ev.Thread != "t1" || ev.Seq != 2 {
		t.Fatalf("opened 事件不对：%+v", ev)
	}

	steps := []struct {
		action string
		opts   PostThreadOptions
	}{
		{"comment", PostThreadOptions{Channel: "board", Scope: "project", By: "alice", Action: "comment", Thread: "t1", Text: "收到，正在看"}},
		{"status", PostThreadOptions{Channel: "board", Scope: "project", By: "main", Action: "status", Thread: "t1", Status: "in-progress"}},
		{"labels", PostThreadOptions{Channel: "board", Scope: "project", By: "main", Action: "labels", Thread: "t1", Labels: []string{"bug", "urgent"}}},
		{"assignees", PostThreadOptions{Channel: "board", Scope: "project", By: "main", Action: "assignees", Thread: "t1", Assignees: []string{"alice", "bob"}}},
		{"summary", PostThreadOptions{Channel: "board", Scope: "project", By: "main", Action: "summary", Thread: "t1", Summary: "根因是空指针"}},
		{"processed", PostThreadOptions{Channel: "board", Scope: "project", By: "main", Action: "processed", Thread: "t1"}},
	}
	for _, step := range steps {
		if _, err := PostThread(s, step.opts); err != nil {
			t.Fatalf("%s 失败：%v", step.action, err)
		}
	}

	states, err := ListForumThreads(s, ListForumThreadsOptions{Channel: "board", Scope: "project"})
	if err != nil {
		t.Fatalf("列出 thread 失败：%v", err)
	}
	if len(states) != 1 {
		t.Fatalf("期望 1 个 thread，得到 %d", len(states))
	}
	st := states[0]
	if st.ID != "t1" || st.Title != "第一主题" || st.Description != "主题描述" {
		t.Fatalf("标题/描述不对：%+v", st)
	}
	if st.Status != "processed" || !st.Processed {
		t.Fatalf("processed 后状态应为 processed：%+v", st)
	}
	if got := strings.Join(st.Labels, ","); got != "bug,urgent" {
		t.Fatalf("标签不对：%q", got)
	}
	if got := strings.Join(st.Assignees, ","); got != "alice,bob" {
		t.Fatalf("负责人不对：%q", got)
	}
	if st.Summary != "根因是空指针" {
		t.Fatalf("摘要不对：%q", st.Summary)
	}
	if st.Comments != 1 {
		t.Fatalf("评论数应为 1，得到 %d", st.Comments)
	}
	if st.CreatedAt == "" || st.UpdatedAt == "" {
		t.Fatalf("创建/更新时间不应为空：%+v", st)
	}

	// 时间线按 seq 升序：create 事件不属于 thread，thread 事件共 7 条。
	timeline, err := ShowThread(s, ShowThreadOptions{Channel: "board", Scope: "project", Thread: "t1"})
	if err != nil {
		t.Fatalf("查看时间线失败：%v", err)
	}
	if len(timeline) != 7 {
		t.Fatalf("时间线应为 7 条，得到 %d", len(timeline))
	}
	for i := 1; i < len(timeline); i++ {
		if timeline[i].Seq <= timeline[i-1].Seq {
			t.Fatalf("时间线未按 seq 升序：第 %d 条 seq=%d，前一条 seq=%d", i, timeline[i].Seq, timeline[i-1].Seq)
		}
	}
}

func TestPostThreadOpenedAutoKey(t *testing.T) {
	fx := setupForumFixture(t)
	ev, err := PostThread(fx.store, PostThreadOptions{
		Channel: "board", Scope: "project", By: "main", Action: "opened", Title: "自动键",
	})
	if err != nil {
		t.Fatalf("opened 自动键失败：%v", err)
	}
	if !strings.HasPrefix(ev.Thread, "thread-") {
		t.Fatalf("自动生成的 thread 键应以 thread- 开头，得到 %q", ev.Thread)
	}
}

func TestPostThreadErrors(t *testing.T) {
	fx := setupForumFixture(t)
	s := fx.store

	cases := []struct {
		name string
		opts PostThreadOptions
	}{
		{"非法动作", PostThreadOptions{Channel: "board", Scope: "project", By: "main", Action: "rename", Thread: "t1"}},
		{"comment 缺 thread", PostThreadOptions{Channel: "board", Scope: "project", By: "main", Action: "comment", Text: "hi"}},
		{"非法 thread 键", PostThreadOptions{Channel: "board", Scope: "project", By: "main", Action: "opened", Thread: "含 空格"}},
		{"不存在的频道", PostThreadOptions{Channel: "nope", Scope: "project", By: "main", Action: "opened", Thread: "t1"}},
	}
	for _, c := range cases {
		if _, err := PostThread(s, c.opts); err == nil {
			t.Errorf("%s：期望报错，实际成功", c.name)
		}
	}

	// chat 频道上调用 post 应报错说明。
	if _, err := PostThread(s, PostThreadOptions{Channel: "talk", Scope: "project", By: "main", Action: "opened", Thread: "t1"}); err == nil || !strings.Contains(err.Error(), "forum") {
		t.Errorf("chat 频道 post：期望报 forum 类型错误，得到 %v", err)
	}
	// chat 频道上 forum/thread 也应报错。
	if _, err := ListForumThreads(s, ListForumThreadsOptions{Channel: "talk", Scope: "project"}); err == nil || !strings.Contains(err.Error(), "forum") {
		t.Errorf("chat 频道 forum：期望报 forum 类型错误，得到 %v", err)
	}
	if _, err := ShowThread(s, ShowThreadOptions{Channel: "talk", Scope: "project", Thread: "t1"}); err == nil || !strings.Contains(err.Error(), "forum") {
		t.Errorf("chat 频道 thread：期望报 forum 类型错误，得到 %v", err)
	}
	if _, err := RenameThread(s, RenameThreadOptions{Channel: "talk", Scope: "project", By: "main", Thread: "t1", NewThread: "t2"}); err == nil || !strings.Contains(err.Error(), "forum") {
		t.Errorf("chat 频道 rename：期望报 forum 类型错误，得到 %v", err)
	}
}

func TestRenameThreadWithAlias(t *testing.T) {
	fx := setupForumFixture(t)
	s := fx.store

	if _, err := PostThread(s, PostThreadOptions{Channel: "board", Scope: "project", By: "main", Action: "opened", Thread: "old-name", Title: "旧名主题"}); err != nil {
		t.Fatalf("opened 失败：%v", err)
	}
	if _, err := PostThread(s, PostThreadOptions{Channel: "board", Scope: "project", By: "main", Action: "comment", Thread: "old-name", Text: "改名前的评论"}); err != nil {
		t.Fatalf("comment 失败：%v", err)
	}
	if _, err := PostThread(s, PostThreadOptions{Channel: "board", Scope: "project", By: "main", Action: "opened", Thread: "other", Title: "另一主题"}); err != nil {
		t.Fatalf("opened other 失败：%v", err)
	}

	ev, err := RenameThread(s, RenameThreadOptions{Channel: "board", Scope: "project", By: "main", Thread: "old-name", NewThread: "new-name", Origin: "cli"})
	if err != nil {
		t.Fatalf("改名失败：%v", err)
	}
	if ev.Action != "rename" || ev.Thread != "old-name" || ev.NewThread != "new-name" {
		t.Fatalf("改名事件不对：%+v", ev)
	}

	// 列表只剩新名，且别名含旧名。
	states, err := ListForumThreads(s, ListForumThreadsOptions{Channel: "board", Scope: "project"})
	if err != nil {
		t.Fatalf("列出失败：%v", err)
	}
	var renamed *ThreadState
	for i := range states {
		if states[i].ID == "new-name" {
			renamed = &states[i]
		}
		if states[i].ID == "old-name" {
			t.Fatalf("列表里不应再出现旧名 old-name")
		}
	}
	if renamed == nil {
		t.Fatalf("列表里找不到 new-name：%v", states)
	}
	if renamed.Title != "旧名主题" || renamed.Comments != 1 {
		t.Fatalf("改名后状态应继承标题与评论数：%+v", renamed)
	}
	found := false
	for _, a := range renamed.Aliases {
		if a == "old-name" {
			found = true
		}
	}
	if !found {
		t.Fatalf("别名里应含 old-name：%v", renamed.Aliases)
	}

	// 用旧名查时间线仍能解析到新 thread：含改名前评论与改名事件。
	timeline, err := ShowThread(s, ShowThreadOptions{Channel: "board", Scope: "project", Thread: "old-name"})
	if err != nil {
		t.Fatalf("用旧名查时间线失败：%v", err)
	}
	if len(timeline) != 3 {
		t.Fatalf("旧名时间线应为 3 条（opened/comment/rename），得到 %d", len(timeline))
	}
	// 用新名查到同一条时间线。
	timeline2, err := ShowThread(s, ShowThreadOptions{Channel: "board", Scope: "project", Thread: "new-name"})
	if err != nil {
		t.Fatalf("用新名查时间线失败：%v", err)
	}
	if len(timeline2) != len(timeline) {
		t.Fatalf("新名与旧名时间线条数不一致：%d vs %d", len(timeline2), len(timeline))
	}

	// 二次改名走别名链：old-name → newer-name。
	if _, err := RenameThread(s, RenameThreadOptions{Channel: "board", Scope: "project", By: "main", Thread: "old-name", NewThread: "newer-name"}); err != nil {
		t.Fatalf("用旧别名二次改名失败：%v", err)
	}
	states, _ = ListForumThreads(s, ListForumThreadsOptions{Channel: "board", Scope: "project"})
	if len(states) != 2 {
		t.Fatalf("二次改名后应仍为 2 个 thread，得到 %d", len(states))
	}
}

func TestRenameThreadErrors(t *testing.T) {
	fx := setupForumFixture(t)
	s := fx.store
	if _, err := PostThread(s, PostThreadOptions{Channel: "board", Scope: "project", By: "main", Action: "opened", Thread: "a", Title: "A"}); err != nil {
		t.Fatal(err)
	}
	if _, err := PostThread(s, PostThreadOptions{Channel: "board", Scope: "project", By: "main", Action: "opened", Thread: "b", Title: "B"}); err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		name string
		opts RenameThreadOptions
	}{
		{"旧名不存在", RenameThreadOptions{Channel: "board", Scope: "project", By: "main", Thread: "ghost", NewThread: "c"}},
		{"新名已存在", RenameThreadOptions{Channel: "board", Scope: "project", By: "main", Thread: "a", NewThread: "b"}},
		{"新旧同名", RenameThreadOptions{Channel: "board", Scope: "project", By: "main", Thread: "a", NewThread: "a"}},
	}
	for _, c := range cases {
		if _, err := RenameThread(s, c.opts); err == nil {
			t.Errorf("%s：期望报错，实际成功", c.name)
		}
	}
}

func TestReduceChannelMetadataLegacyType(t *testing.T) {
	// 旧日志里的 "thread"/"threads" 类型不归一化为 forum。
	for _, typ := range []string{"thread", "threads", "chat"} {
		events := []ChannelEvent{{Seq: 1, Kind: "create", By: "main", Type: typ}}
		if got := ReduceChannelMetadata(events).Type; got != "chat" {
			t.Errorf("类型 %q 应投影为 chat，得到 %q", typ, got)
		}
	}
	events := []ChannelEvent{{Seq: 1, Kind: "create", By: "main", Type: "forum"}}
	if got := ReduceChannelMetadata(events).Type; got != "forum" {
		t.Errorf("类型 forum 应投影为 forum，得到 %q", got)
	}
	// 无 create 事件时默认 chat。
	if got := ReduceChannelMetadata(nil).Type; got != "chat" {
		t.Errorf("无事件时应默认 chat，得到 %q", got)
	}
}

func TestBuildContextEntriesAndParseCSV(t *testing.T) {
	entries, err := BuildContextEntries([]string{"/tmp/a.md"}, []string{"原始文本"})
	if err != nil || len(entries) != 2 {
		t.Fatalf("上下文条目不对：%v %v", entries, err)
	}
	if _, err := BuildContextEntries([]string{"relative/path.md"}, nil); err == nil {
		t.Errorf("相对路径应报错")
	}
	if _, err := BuildContextEntries(nil, []string{"  "}); err == nil {
		t.Errorf("空原始文本应报错")
	}
	if entries, err := BuildContextEntries(nil, nil); err != nil || entries != nil {
		t.Errorf("两表都空应返回 nil：%v %v", entries, err)
	}
	if got := ParseCSV("a, b,,c "); len(got) != 3 || got[1] != "b" {
		t.Errorf("CSV 解析不对：%v", got)
	}
	if got := ParseCSV(""); got != nil {
		t.Errorf("空 CSV 应返回 nil：%v", got)
	}
}

func TestStorePaginationIdempotencyAndBadLines(t *testing.T) {
	fx := setupForumFixture(t)
	s := fx.store

	// 幂等：同 key 同 kind 返回已有事件，不多写。
	ev1, err := s.AppendEvent("board", fx.project, map[string]any{
		"kind": "thread", "by": "main", "action": "comment", "thread": "t1",
		"idempotencyKey": "k1",
	})
	if err != nil {
		t.Fatalf("首次追加失败：%v", err)
	}
	ev2, err := s.AppendEvent("board", fx.project, map[string]any{
		"kind": "thread", "by": "main", "action": "comment", "thread": "t1",
		"idempotencyKey": "k1",
	})
	if err != nil || ev2.Seq != ev1.Seq {
		t.Fatalf("幂等去重不对：ev2=%+v err=%v", ev2, err)
	}
	// 同 key 不同 kind 报错。
	if _, err := s.AppendEvent("board", fx.project, map[string]any{
		"kind": "message", "by": "main", "idempotencyKey": "k1",
	}); err == nil {
		t.Errorf("同 key 不同 kind 应报错")
	}

	// 坏行跳过。
	path := filepath.Join(fx.root, fx.project, "board", "events.jsonl")
	f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.WriteString("这行不是 JSON\n"); err != nil {
		t.Fatal(err)
	}
	_ = f.Close()

	all, err := s.ReadChannelEvents("board", fx.project, Pagination{})
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != 2 {
		t.Fatalf("坏行跳过后应读到 2 条事件（create + comment），得到 %d", len(all))
	}
	// afterSeq 与 beforeSeq 同传报错。
	after, before := int64(0), int64(99)
	if _, err := s.ReadChannelEvents("board", fx.project, Pagination{AfterSeq: &after, BeforeSeq: &before}); err == nil {
		t.Errorf("afterSeq 与 beforeSeq 同传应报错")
	}
	// limit 只取最新 N 条。
	limit := 1
	page, err := s.ReadChannelEvents("board", fx.project, Pagination{Limit: &limit})
	if err != nil || len(page) != 1 || page[0].Seq != ev1.Seq {
		t.Fatalf("limit 分页不对：%v %v", page, err)
	}
}
