package store

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// testRoot 为每个测试建隔离的存储根，并返回固定项目桶的 ref。
func testRoot(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	t.Setenv("SUNSHINE_CHANNEL_ROOT", root)
	t.Setenv("SUNSHINE_CHANNEL_PROJECT", "")
	return root
}

func testRef(name string) ChannelRef {
	return ChannelRef{Name: name, Scope: ScopeProject, Project: "testproj", Dir: filepath.Join("testproj", name)}
}

func appendMsg(t *testing.T, ref ChannelRef, by string, extra map[string]any) ChannelEvent {
	t.Helper()
	ev, err := AppendEvent(ref, AppendableEvent{Kind: KindMessage, By: by, Extra: extra})
	if err != nil {
		t.Fatalf("追加事件失败：%v", err)
	}
	return ev
}

func intPtr(v int64) *int64 { return &v }
func limitPtr(v int) *int   { return &v }

func TestProjectKey(t *testing.T) {
	cases := map[string]string{
		"/home/lin/my_proj": "-home-lin-my-proj",
		"/tmp/项目-x":         "-tmp----x",
		"/home/lin/a.b-c_d": "-home-lin-a.b-c-d",
		"/home/lin/a_b/c":   "-home-lin-a-b-c",
	}
	for in, want := range cases {
		if got := ProjectKey(in); got != want {
			t.Errorf("ProjectKey(%q) = %q，想要 %q", in, got, want)
		}
	}
}

func TestResolveChannel(t *testing.T) {
	testRoot(t)
	ref, err := ResolveChannel("demo", ResolveChannelOptions{Scope: ScopeGlobal})
	if err != nil {
		t.Fatalf("ResolveChannel 失败：%v", err)
	}
	if ref.Project != GlobalProjectKey || ref.Scope != ScopeGlobal {
		t.Errorf("global 解析错误：%+v", ref)
	}
	ref, err = ResolveChannel("demo", ResolveChannelOptions{Scope: ScopeProject, Cwd: "/home/lin/my_proj"})
	if err != nil {
		t.Fatalf("ResolveChannel 失败：%v", err)
	}
	if ref.Project != "-home-lin-my-proj" {
		t.Errorf("project 解析错误：%q", ref.Project)
	}
	if _, err := ResolveChannel("../escape", ResolveChannelOptions{}); err == nil {
		t.Error("非法 channel 名应报错")
	}
}

func TestResolveExistingChannelRef(t *testing.T) {
	root := testRoot(t)
	// 当前项目桶与 global 桶各建一个同名 channel。
	current := CurrentProjectKey()
	for _, project := range []string{current, GlobalProjectKey} {
		ref := ChannelRef{Name: "dup", Scope: ScopeProject, Project: project}
		if _, err := AppendEvent(ref, AppendableEvent{Kind: KindCreate, By: "main"}); err != nil {
			t.Fatalf("建 channel 失败：%v", err)
		}
	}
	_ = root
	if _, err := ResolveExistingChannelRef("dup", ResolveChannelOptions{}); err == nil {
		t.Error("global 与项目同时存在时应报错要求明确 scope")
	}
	ref, err := ResolveExistingChannelRef("dup", ResolveChannelOptions{Scope: ScopeGlobal})
	if err != nil {
		t.Fatalf("global 解析失败：%v", err)
	}
	if ref.Project != GlobalProjectKey {
		t.Errorf("应为 global 桶，得到 %q", ref.Project)
	}
	if _, err := ResolveExistingChannelRef("不存在", ResolveChannelOptions{}); err == nil {
		t.Error("不存在的 channel 应报错")
	}
}

func TestAppendAndRead(t *testing.T) {
	testRoot(t)
	ref := testRef("chat1")
	ev1, err := AppendEvent(ref, AppendableEvent{Kind: KindCreate, By: "main", Extra: map[string]any{"type": "chat"}})
	if err != nil {
		t.Fatalf("追加失败：%v", err)
	}
	if ev1.Seq != 1 {
		t.Errorf("首条 seq 应为 1，得到 %d", ev1.Seq)
	}
	ev2 := appendMsg(t, ref, "main", map[string]any{"text": "你好"})
	if ev2.Seq != 2 {
		t.Errorf("第二条 seq 应为 2，得到 %d", ev2.Seq)
	}
	events, err := ReadChannelEvents(ref, nil)
	if err != nil || len(events) != 2 {
		t.Fatalf("读事件失败：%v，条数 %d", err, len(events))
	}
	if events[1].StrField("text") != "你好" {
		t.Errorf("Extra 字段丢失：%v", events[1].Extra)
	}
	// sidecar 与 JSONL 一致。
	sidecar, _ := SeqSidecarPath(ref.Name, ref.Project)
	data, _ := os.ReadFile(sidecar)
	if strings.TrimSpace(string(data)) != "2" {
		t.Errorf("sidecar 应为 2，得到 %q", string(data))
	}
	// To 字段：字符串与数组两种写法都能读。
	ev3, err := AppendEvent(ref, AppendableEvent{Kind: KindMessage, By: "main", To: []string{"w1", "w2"}, Extra: map[string]any{"text": "群发"}})
	if err != nil {
		t.Fatalf("追加失败：%v", err)
	}
	if len(ev3.To) != 2 {
		t.Errorf("To 应有 2 个目标，得到 %v", ev3.To)
	}
	raw, _ := os.ReadFile(filepath.Join(ChannelRoot(), "testproj", "chat1", "events.jsonl"))
	lines := strings.Split(strings.TrimSpace(string(raw)), "\n")
	if len(lines) != 3 {
		t.Fatalf("应有 3 行，得到 %d", len(lines))
	}
	if !strings.Contains(lines[2], `"to":["w1","w2"]`) {
		t.Errorf("多目标应写成数组：%s", lines[2])
	}
}

func TestTornTailTruncated(t *testing.T) {
	testRoot(t)
	ref := testRef("torn")
	appendMsg(t, ref, "main", map[string]any{"text": "一"})
	appendMsg(t, ref, "main", map[string]any{"text": "二"})
	jsonl, _ := EventsPath(ref.Name, ref.Project)
	f, err := os.OpenFile(jsonl, os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		t.Fatal(err)
	}
	// 模拟追加中途被杀：写半行且无换行。
	if _, err := f.WriteString(`{"seq":99,"ts":"x","kind":"message","by":"main","text":"半行`); err != nil {
		t.Fatal(err)
	}
	f.Close()
	ev := appendMsg(t, ref, "main", map[string]any{"text": "三"})
	if ev.Seq != 3 {
		t.Errorf("撕裂尾部截断后下一条 seq 应为 3，得到 %d", ev.Seq)
	}
	events, err := ReadChannelEvents(ref, nil)
	if err != nil || len(events) != 3 {
		t.Fatalf("应读到 3 条完整事件，得到 %d（err=%v）", len(events), err)
	}
	if events[2].StrField("text") != "三" {
		t.Errorf("最后一条内容错误：%v", events[2].Extra)
	}
}

func TestIdempotency(t *testing.T) {
	testRoot(t)
	ref := testRef("idem")
	ev1, err := AppendEvent(ref, AppendableEvent{Kind: KindMessage, By: "main", IdempotencyKey: "k1", Extra: map[string]any{"text": "首次"}})
	if err != nil {
		t.Fatalf("追加失败：%v", err)
	}
	ev2, err := AppendEvent(ref, AppendableEvent{Kind: KindMessage, By: "main", IdempotencyKey: "k1", Extra: map[string]any{"text": "重放"}})
	if err != nil {
		t.Fatalf("同 key 同 kind 应返回已有事件：%v", err)
	}
	if ev2.Seq != ev1.Seq || ev2.StrField("text") != "首次" {
		t.Errorf("重放应返回已有事件，得到 seq=%d text=%q", ev2.Seq, ev2.StrField("text"))
	}
	if _, err := AppendEvent(ref, AppendableEvent{Kind: KindJoin, By: "main", IdempotencyKey: "k1"}); err == nil {
		t.Error("同 key 不同 kind 应报错")
	}
	events, _ := ReadChannelEvents(ref, nil)
	if len(events) != 1 {
		t.Errorf("幂等重放不应多写事件，得到 %d 条", len(events))
	}
	if _, err := AppendEvent(ref, AppendableEvent{Kind: KindMessage, By: "main", IdempotencyKey: "  "}); err == nil {
		t.Error("空白幂等键应报错")
	}
}

func TestPagination(t *testing.T) {
	testRoot(t)
	ref := testRef("page")
	for i := 0; i < 250; i++ {
		appendMsg(t, ref, "main", map[string]any{"text": fmt.Sprintf("第 %d 条", i+1)})
	}
	// 默认页大小 200。
	page, err := ReadChannelEvents(ref, &ReadChannelEventsPagination{AfterSeq: intPtr(0)})
	if err != nil || len(page) != 200 {
		t.Fatalf("afterSeq=0 默认应返回 200 条，得到 %d（err=%v）", len(page), err)
	}
	if page[0].Seq != 1 || page[199].Seq != 200 {
		t.Errorf("第一页 seq 范围错误：%d..%d", page[0].Seq, page[199].Seq)
	}
	page, err = ReadChannelEvents(ref, &ReadChannelEventsPagination{AfterSeq: intPtr(200)})
	if err != nil || len(page) != 50 {
		t.Fatalf("afterSeq=200 应返回 50 条，得到 %d（err=%v）", len(page), err)
	}
	// beforeSeq 取最新一页，升序返回。
	page, err = ReadChannelEvents(ref, &ReadChannelEventsPagination{BeforeSeq: intPtr(251)})
	if err != nil || len(page) != 200 {
		t.Fatalf("beforeSeq=251 默认应返回 200 条，得到 %d（err=%v）", len(page), err)
	}
	if page[0].Seq != 51 || page[199].Seq != 250 {
		t.Errorf("beforeSeq 页范围错误：%d..%d", page[0].Seq, page[199].Seq)
	}
	// limit 仅传时返回最新 N 条。
	page, err = ReadChannelEvents(ref, &ReadChannelEventsPagination{Limit: limitPtr(10)})
	if err != nil || len(page) != 10 || page[0].Seq != 241 {
		t.Fatalf("limit=10 应返回 seq 241..250，得到 %d 条，首条 %d（err=%v）", len(page), page[0].Seq, err)
	}
	// afterSeq 与 beforeSeq 不可同传。
	if _, err := ReadChannelEvents(ref, &ReadChannelEventsPagination{AfterSeq: intPtr(1), BeforeSeq: intPtr(10)}); err == nil {
		t.Error("afterSeq 与 beforeSeq 同传应报错")
	}
	if _, err := ReadChannelEvents(ref, &ReadChannelEventsPagination{Limit: limitPtr(-1)}); err == nil {
		t.Error("负 limit 应报错")
	}
	// 坏行跳过。
	jsonl, _ := EventsPath(ref.Name, ref.Project)
	f, err := os.OpenFile(jsonl, os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.WriteString("这不是 JSON\n"); err != nil {
		t.Fatal(err)
	}
	f.Close()
	all, err := ReadChannelEvents(ref, nil)
	if err != nil || len(all) != 250 {
		t.Fatalf("坏行应跳过，仍读到 250 条，得到 %d（err=%v）", len(all), err)
	}
}

func TestReconcileSeq(t *testing.T) {
	testRoot(t)
	ref := testRef("recon")
	for i := 0; i < 5; i++ {
		appendMsg(t, ref, "main", map[string]any{"text": fmt.Sprintf("第 %d 条", i+1)})
	}
	sidecar, _ := SeqSidecarPath(ref.Name, ref.Project)
	// 删 sidecar：按 JSONL 尾部修复。
	if err := os.Remove(sidecar); err != nil {
		t.Fatal(err)
	}
	seq, err := ReconcileSeq(ref)
	if err != nil || seq != 5 {
		t.Fatalf("删 sidecar 后对账应返回 5，得到 %d（err=%v）", seq, err)
	}
	data, _ := os.ReadFile(sidecar)
	if strings.TrimSpace(string(data)) != "5" {
		t.Errorf("sidecar 应被修复为 5，得到 %q", string(data))
	}
	// sidecar 超前：应回退到 JSONL 尾部。
	if err := os.WriteFile(sidecar, []byte("999\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	seq, err = ReconcileSeq(ref)
	if err != nil || seq != 5 {
		t.Fatalf("sidecar 超前时对账应返回 5，得到 %d（err=%v）", seq, err)
	}
	// sidecar 损坏：应修复。
	if err := os.WriteFile(sidecar, []byte("不是数字\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	seq, err = ReconcileSeq(ref)
	if err != nil || seq != 5 {
		t.Fatalf("sidecar 损坏时对账应返回 5，得到 %d（err=%v）", seq, err)
	}
	// 修复后继续追加，seq 不断档。
	ev := appendMsg(t, ref, "main", map[string]any{"text": "第六条"})
	if ev.Seq != 6 {
		t.Errorf("对账后下一条 seq 应为 6，得到 %d", ev.Seq)
	}
}

func TestLockStealAndRelease(t *testing.T) {
	testRoot(t)
	ref := testRef("locktest")
	dir, err := EnsureChannelDir(ref.Name, ref.Project)
	if err != nil {
		t.Fatal(err)
	}
	lockFile := filepath.Join(dir, "chat.lock")
	// 持锁进程已死：写一个已退出的进程 pid，AcquireLock 应窃取。
	deadPid := exitedPid(t)
	if err := os.WriteFile(lockFile, []byte(fmt.Sprintf("%d", deadPid)), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := AcquireLock(lockFile, AcquireOptions{}); err != nil {
		t.Fatalf("死进程的锁应可窃取：%v", err)
	}
	ReleaseLock(lockFile)
	if _, err := os.Stat(lockFile); !os.IsNotExist(err) {
		t.Error("释放后锁文件应被删除")
	}
	// ReleaseLock 不删别人的锁：写 pid 1（存活进程）的锁文件。
	if err := os.WriteFile(lockFile, []byte("1"), 0o644); err != nil {
		t.Fatal(err)
	}
	ReleaseLock(lockFile)
	if _, err := os.Stat(lockFile); err != nil {
		t.Error("ReleaseLock 不应删除他人持有的锁")
	}
	os.Remove(lockFile)
	// 锁被他人持有时超时报错。
	if err := AcquireLock(lockFile, AcquireOptions{}); err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() {
		done <- AcquireLock(lockFile, AcquireOptions{RetryIntervalMs: 10, MaxWaitMs: 100})
	}()
	if err := <-done; err == nil {
		t.Error("锁被占用时应在超时后报错")
		ReleaseLock(lockFile)
	}
	ReleaseLock(lockFile)
}

// exitedPid 返回一个已退出进程的 pid（用于模拟死进程持有的锁）。
func exitedPid(t *testing.T) int {
	t.Helper()
	// 用当前测试进程派生一个立即退出的子进程太重；直接用一个
	// 必然不存在的大 pid：内核不会分配到该值。
	return 999999
}

func TestWorkerRegistry(t *testing.T) {
	testRoot(t)
	ref := testRef("workers")
	events := []AppendableEvent{
		{Kind: KindSpawned, By: "main", Extra: map[string]any{"as": "worker-a", "provider": "claude", "agent": "implement", "inboxPolicy": "broadcastAndExplicit"}},
		{Kind: KindSpawned, By: "main", Extra: map[string]any{"as": "worker-b", "provider": "codex"}},
		{Kind: KindMessage, By: "main", To: []string{"worker-a"}, Extra: map[string]any{"text": "给 A"}},
		{Kind: KindMessage, By: "main", Extra: map[string]any{"text": "广播"}},
		{Kind: KindMessage, By: "worker-a", Extra: map[string]any{"text": "自己发的"}},
		{Kind: KindMessage, By: "main", To: []string{"worker-b"}, Extra: map[string]any{"text": "给 B"}},
	}
	for _, e := range events {
		if _, err := AppendEvent(ref, e); err != nil {
			t.Fatalf("追加失败：%v", err)
		}
	}
	all, _ := ReadChannelEvents(ref, nil)
	reg := ReduceWorkerRegistry(all, &ref)
	if len(reg.Workers) != 2 {
		t.Fatalf("应有 2 个 worker，得到 %d", len(reg.Workers))
	}
	a := reg.FindWorker("worker-a")
	b := reg.FindWorker("worker-b")
	if a == nil || b == nil {
		t.Fatalf("worker 查找失败：a=%v b=%v", a, b)
	}
	if a.Provider != "claude" || a.Agent != "implement" || a.InboxPolicy != InboxPolicyBroadcastAndExplicit {
		t.Errorf("worker-a 字段错误：%+v", a)
	}
	if a.Channel == nil || a.Channel.Name != "workers" {
		t.Error("应贴上 channel 引用")
	}
	// worker-a（broadcastAndExplicit）：定向 1 + 广播 1 = 2；自己发的 1 条不算。
	if a.PendingMessageCount != 2 {
		t.Errorf("worker-a 待处理应为 2，得到 %d", a.PendingMessageCount)
	}
	// worker-b（explicitOnly）：只算定向给自己的 1 条。
	if b.PendingMessageCount != 1 {
		t.Errorf("worker-b 待处理应为 1，得到 %d", b.PendingMessageCount)
	}
	// turn_started 消费 inputSeq=3（给 A 的消息），再 turn_finished。
	if _, err := AppendEvent(ref, AppendableEvent{Kind: KindTurnStarted, By: "worker-a", Extra: map[string]any{"worker": "worker-a", "inputSeq": 3, "turnId": "t1"}}); err != nil {
		t.Fatal(err)
	}
	all, _ = ReadChannelEvents(ref, nil)
	reg = ReduceWorkerRegistry(all, nil)
	a = reg.FindWorker("worker-a")
	if a.Activity != WorkerActivityMidTurn || a.ActiveTurnID != "t1" {
		t.Errorf("turn_started 后应为 mid-turn，得到 %+v", a)
	}
	// 消费到 seq 3：广播（seq 4）仍待处理，定向（seq 3）已消费。
	if a.PendingMessageCount != 1 {
		t.Errorf("消费后 worker-a 待处理应为 1，得到 %d", a.PendingMessageCount)
	}
	if _, err := AppendEvent(ref, AppendableEvent{Kind: KindTurnFinished, By: "worker-a", Extra: map[string]any{"worker": "worker-a", "turnId": "t1"}}); err != nil {
		t.Fatal(err)
	}
	all, _ = ReadChannelEvents(ref, nil)
	reg = ReduceWorkerRegistry(all, nil)
	a = reg.FindWorker("worker-a")
	if a.Activity != WorkerActivityIdle || a.IdleSince == "" {
		t.Errorf("turn_finished 后应为空闲且有 idleSince，得到 %+v", a)
	}
	// 非 synthesized 的 done 不进终态；synthesized 的 done 进终态。
	if _, err := AppendEvent(ref, AppendableEvent{Kind: KindDone, By: "worker-b", Extra: map[string]any{"worker": "worker-b"}}); err != nil {
		t.Fatal(err)
	}
	all, _ = ReadChannelEvents(ref, nil)
	reg = ReduceWorkerRegistry(all, nil)
	if reg.FindWorker("worker-b").Terminal {
		t.Error("非 synthesized 的 done 不应进终态")
	}
	if _, err := AppendEvent(ref, AppendableEvent{Kind: KindDone, By: "worker-b", Extra: map[string]any{"worker": "worker-b", "synthesized": true, "exit_code": 0}}); err != nil {
		t.Fatal(err)
	}
	all, _ = ReadChannelEvents(ref, nil)
	reg = ReduceWorkerRegistry(all, nil)
	b = reg.FindWorker("worker-b")
	if !b.Terminal || b.Lifecycle != WorkerLifecycleDone || b.PendingMessageCount != 0 {
		t.Errorf("synthesized done 应进终态且待处理清零，得到 %+v", b)
	}
	if b.ExitCode == nil || *b.ExitCode != 0 {
		t.Errorf("exit_code 应为 0，得到 %v", b.ExitCode)
	}
	// reason=crash 的 killed → crashed。
	if _, err := AppendEvent(ref, AppendableEvent{Kind: KindKilled, By: "supervisor:worker-a", Extra: map[string]any{"worker": "worker-a", "reason": "crash"}}); err != nil {
		t.Fatal(err)
	}
	all, _ = ReadChannelEvents(ref, nil)
	reg = ReduceWorkerRegistry(all, nil)
	a = reg.FindWorker("worker-a")
	if a.Lifecycle != WorkerLifecycleCrashed || !a.Terminal {
		t.Errorf("crash 原因的 killed 应为 crashed 终态，得到 %+v", a)
	}
	// 普通 message 事件的 by 不应造出幽灵 worker。
	if _, err := AppendEvent(ref, AppendableEvent{Kind: KindMessage, By: "路人", Extra: map[string]any{"text": "hi"}}); err != nil {
		t.Fatal(err)
	}
	all, _ = ReadChannelEvents(ref, nil)
	reg = ReduceWorkerRegistry(all, nil)
	if len(reg.Workers) != 2 {
		t.Errorf("普通消息不应新建 worker，得到 %d 个", len(reg.Workers))
	}
	// ReduceWorkerRegistry 是纯函数：不读 pid 文件与本地游标。
	// 同一份事件两次折叠结果应一致。
	reg2 := ReduceWorkerRegistry(all, nil)
	if len(reg2.Workers) != len(reg.Workers) {
		t.Error("纯函数两次折叠结果不一致")
	}
}

func TestMatchesInboxPolicy(t *testing.T) {
	msg := func(by string, to []string) ChannelEvent {
		return ChannelEvent{Kind: KindMessage, By: by, To: to}
	}
	if !MatchesInboxPolicy(msg("main", []string{"w1"}), "w1", InboxPolicyExplicitOnly) {
		t.Error("explicitOnly 应消费 to 指向自己的消息")
	}
	if MatchesInboxPolicy(msg("main", []string{"w2"}), "w1", InboxPolicyExplicitOnly) {
		t.Error("explicitOnly 不应消费 to 指向别人的消息")
	}
	if MatchesInboxPolicy(msg("main", nil), "w1", InboxPolicyExplicitOnly) {
		t.Error("explicitOnly 不应消费广播")
	}
	if !MatchesInboxPolicy(msg("main", nil), "w1", InboxPolicyBroadcastAndExplicit) {
		t.Error("broadcastAndExplicit 应消费广播")
	}
	if MatchesInboxPolicy(msg("w1", []string{"w1"}), "w1", InboxPolicyBroadcastAndExplicit) {
		t.Error("worker 永不消费自己发的消息")
	}
	if MatchesInboxPolicy(ChannelEvent{Kind: KindProgress, By: "main"}, "w1", InboxPolicyBroadcastAndExplicit) {
		t.Error("inbox 策略只对 kind:message 生效")
	}
}

func TestDeliveryModes(t *testing.T) {
	testRoot(t)
	ref := testRef("delivery")
	// 先 spawn 一个 worker，随后让它终态。
	if _, err := AppendEvent(ref, AppendableEvent{Kind: KindSpawned, By: "main", Extra: map[string]any{"as": "alive"}}); err != nil {
		t.Fatal(err)
	}
	if _, err := AppendEvent(ref, AppendableEvent{Kind: KindSpawned, By: "main", Extra: map[string]any{"as": "dead"}}); err != nil {
		t.Fatal(err)
	}
	if _, err := AppendEvent(ref, AppendableEvent{Kind: KindDone, By: "dead", Extra: map[string]any{"worker": "dead", "synthesized": true}}); err != nil {
		t.Fatal(err)
	}

	// appendOnly：未知目标也不报错、不产生 undeliverable。
	msg, und, err := AppendMessage(ref, AppendableEvent{Kind: KindMessage, By: "main", To: []string{"ghost"}, Extra: map[string]any{"text": "hi"}}, DeliveryModeAppendOnly)
	if err != nil || len(und) != 0 {
		t.Fatalf("appendOnly 不应产生 undeliverable：und=%d err=%v", len(und), err)
	}
	if msg.Seq == 0 {
		t.Error("消息应先落盘")
	}

	// requireKnownWorker：未知目标 → worker-unknown；已知存活目标不产生。
	_, und, err = AppendMessage(ref, AppendableEvent{Kind: KindMessage, By: "main", To: []string{"ghost", "alive"}, Extra: map[string]any{"text": "hi"}}, DeliveryModeRequireKnownWorker)
	if err != nil {
		t.Fatal(err)
	}
	if len(und) != 1 || und[0].StrField("targetWorker") != "ghost" || und[0].StrField("reason") != string(UndeliverableWorkerUnknown) {
		t.Fatalf("requireKnownWorker 应只对 ghost 产生 worker-unknown，得到 %+v", und)
	}
	if und[0].Seq <= msg.Seq {
		t.Error("undeliverable 应在消息之后落盘")
	}

	// requireRunningWorker：未知 → worker-unknown；终态 → worker-terminal。
	_, und, err = AppendMessage(ref, AppendableEvent{Kind: KindMessage, By: "main", To: []string{"ghost", "dead", "alive"}, Extra: map[string]any{"text": "hi"}}, DeliveryModeRequireRunningWorker)
	if err != nil {
		t.Fatal(err)
	}
	if len(und) != 2 {
		t.Fatalf("requireRunningWorker 应产生 2 条 undeliverable，得到 %d", len(und))
	}
	reasons := map[string]string{}
	for _, u := range und {
		reasons[u.StrField("targetWorker")] = u.StrField("reason")
	}
	if reasons["ghost"] != string(UndeliverableWorkerUnknown) || reasons["dead"] != string(UndeliverableWorkerTerminal) {
		t.Errorf("原因错误：%v", reasons)
	}

	// 广播消息永不产生 undeliverable。
	_, und, err = AppendMessage(ref, AppendableEvent{Kind: KindMessage, By: "main", Extra: map[string]any{"text": "广播"}}, DeliveryModeRequireRunningWorker)
	if err != nil || len(und) != 0 {
		t.Fatalf("广播不应产生 undeliverable：und=%d err=%v", len(und), err)
	}

	// 幂等重放：消息带 key 时，undeliverable 的派生 key 也去重。
	_, und, err = AppendMessage(ref, AppendableEvent{Kind: KindMessage, By: "main", IdempotencyKey: "send-1", To: []string{"ghost"}, Extra: map[string]any{"text": "hi"}}, DeliveryModeRequireKnownWorker)
	if err != nil || len(und) != 1 {
		t.Fatalf("首次发送应产生 1 条 undeliverable，得到 %d（err=%v）", len(und), err)
	}
	_, und, err = AppendMessage(ref, AppendableEvent{Kind: KindMessage, By: "main", IdempotencyKey: "send-1", To: []string{"ghost"}, Extra: map[string]any{"text": "hi"}}, DeliveryModeRequireKnownWorker)
	if err != nil {
		t.Fatal(err)
	}
	if len(und) != 0 {
		t.Errorf("幂等重放不应重复产生 undeliverable，得到 %d 条", len(und))
	}
}

func TestReduceChannelMetadata(t *testing.T) {
	testRoot(t)
	ref := testRef("meta")
	events := []AppendableEvent{
		{Kind: KindCreate, By: "main", Extra: map[string]any{
			"type": "forum", "description": "讨论区",
			"labels":  []any{"a", "b"},
			"context": []any{map[string]any{"type": "file", "path": "/tmp/x.md"}},
		}},
		{Kind: KindChannel, By: "main", Extra: map[string]any{"action": "title", "title": "新标题"}},
		{Kind: KindContext, By: "main", Extra: map[string]any{
			"target": "channel", "action": "add",
			"context": []any{map[string]any{"type": "raw", "text": "补充说明"}},
		}},
		{Kind: KindContext, By: "main", Extra: map[string]any{
			"target": "channel", "action": "delete",
			"context": []any{map[string]any{"type": "file", "path": "/tmp/x.md"}},
		}},
	}
	for _, e := range events {
		if _, err := AppendEvent(ref, e); err != nil {
			t.Fatal(err)
		}
	}
	all, _ := ReadChannelEvents(ref, nil)
	meta := ReduceChannelMetadata(all)
	if meta.Type != ChannelTypeForum || meta.Title != "新标题" || meta.Description != "讨论区" {
		t.Errorf("元数据投影错误：%+v", meta)
	}
	if len(meta.Labels) != 2 || len(meta.Context) != 1 || meta.Context[0].Text != "补充说明" {
		t.Errorf("labels/context 投影错误：%+v", meta)
	}
	// title 清除。
	if _, err := AppendEvent(ref, AppendableEvent{Kind: KindChannel, By: "main", Extra: map[string]any{"action": "title", "title": nil}}); err != nil {
		t.Fatal(err)
	}
	all, _ = ReadChannelEvents(ref, nil)
	if meta := ReduceChannelMetadata(all); meta.Title != "" {
		t.Errorf("title 清除后应为空，得到 %q", meta.Title)
	}
	// 旧值 type:"thread" 不归一化为 forum。
	ref2 := testRef("legacy")
	if _, err := AppendEvent(ref2, AppendableEvent{Kind: KindCreate, By: "main", Extra: map[string]any{"type": "thread"}}); err != nil {
		t.Fatal(err)
	}
	all, _ = ReadChannelEvents(ref2, nil)
	if meta := ReduceChannelMetadata(all); meta.Type != ChannelTypeChat {
		t.Errorf("旧 type:thread 应投影为 chat，得到 %q", meta.Type)
	}
}

func TestReduceThreads(t *testing.T) {
	testRoot(t)
	ref := testRef("forum")
	events := []AppendableEvent{
		{Kind: KindThread, By: "alice", Extra: map[string]any{"action": "opened", "thread": "t1", "title": "问题一", "status": "open", "labels": []any{"bug"}}},
		{Kind: KindThread, By: "bob", Extra: map[string]any{"action": "comment", "thread": "t1", "text": "复现了"}},
		{Kind: KindThread, By: "alice", Extra: map[string]any{"action": "status", "thread": "t1", "status": "done"}},
		{Kind: KindThread, By: "alice", Extra: map[string]any{"action": "rename", "thread": "t1", "newThread": "t1-renamed"}},
		{Kind: KindThread, By: "bob", Extra: map[string]any{"action": "comment", "thread": "t1-renamed", "text": "跟进"}},
	}
	for _, e := range events {
		if _, err := AppendEvent(ref, e); err != nil {
			t.Fatal(err)
		}
	}
	all, _ := ReadChannelEvents(ref, nil)
	threads := ReduceThreads(all)
	if len(threads) != 1 {
		t.Fatalf("改名后应只剩 1 个 thread，得到 %d", len(threads))
	}
	th := threads[0]
	if th.Thread != "t1-renamed" || th.Status != "done" || th.Comments != 2 || th.Title != "问题一" {
		t.Errorf("thread 投影错误：%+v", th)
	}
	if len(th.Aliases) != 1 || th.Aliases[0] != "t1" {
		t.Errorf("别名应为 [t1]，得到 %v", th.Aliases)
	}
	// 用旧键也能取到时间线。
	timeline := CollectThreadTimeline(all, "t1")
	if len(timeline) != 5 {
		t.Errorf("旧键时间线应有 5 条，得到 %d", len(timeline))
	}
	resolver := BuildThreadAliasResolver(all)
	if resolver.Resolve("t1") != "t1-renamed" {
		t.Error("别名解析失败")
	}
}

func TestMatchesEventFilter(t *testing.T) {
	ev := func(kind ChannelEventKind, by string) ChannelEvent {
		return ChannelEvent{Kind: kind, By: by, Ts: "2026-11-09T00:00:00Z"}
	}
	// 默认不过滤 progress 与 supervisor_warning。
	if MatchesEventFilter(ev(KindProgress, "main"), ChannelEventFilter{}) {
		t.Error("默认不应匹配 progress")
	}
	if MatchesEventFilter(ev(KindSupervisorWarning, "main"), ChannelEventFilter{}) {
		t.Error("默认不应匹配 supervisor_warning")
	}
	// 显式 kind 绕过 meaningful 过滤。
	if !MatchesEventFilter(ev(KindSupervisorWarning, "main"), ChannelEventFilter{Kind: []ChannelEventKind{KindSupervisorWarning}}) {
		t.Error("显式 kind 应能匹配 supervisor_warning")
	}
	// includeProgress 显式开启。
	if !MatchesEventFilter(ev(KindProgress, "main"), ChannelEventFilter{IncludeProgress: true, IncludeNonMeaningful: true}) {
		t.Error("includeProgress 应匹配 progress")
	}
	// self 排除。
	if MatchesEventFilter(ev(KindMessage, "me"), ChannelEventFilter{Self: "me"}) {
		t.Error("self 应排除自己的事件")
	}
	// from 与 to。
	if !MatchesEventFilter(ChannelEvent{Kind: KindMessage, By: "a", To: []string{"w1"}}, ChannelEventFilter{From: []string{"a"}, To: "w1"}) {
		t.Error("from/to 应匹配")
	}
	if MatchesEventFilter(ChannelEvent{Kind: KindMessage, By: "a", To: []string{"w1"}}, ChannelEventFilter{To: "exclusive", From: []string{"b"}}) {
		t.Error("from 不匹配时不应通过")
	}
	if MatchesEventFilter(ChannelEvent{Kind: KindMessage, By: "a"}, ChannelEventFilter{To: "exclusive"}) {
		t.Error("exclusive 不应匹配广播")
	}
	// thread/action。
	threadEv := ChannelEvent{Kind: KindThread, By: "a", Extra: map[string]any{"thread": "t1", "action": "comment"}}
	if !MatchesEventFilter(threadEv, ChannelEventFilter{Thread: "t1", Action: ThreadActionComment}) {
		t.Error("thread/action 应匹配")
	}
	if MatchesEventFilter(threadEv, ChannelEventFilter{Thread: "t2"}) {
		t.Error("thread 不匹配时不应通过")
	}
}

func TestEventJSONRoundTrip(t *testing.T) {
	ev := ChannelEvent{
		Seq: 7, Ts: "2026-11-09T10:00:00Z", Kind: KindMessage, By: "main",
		To: []string{"w1"}, Origin: OriginCLI,
		Meta:  map[string]any{"k": "v"},
		Extra: map[string]any{"text": "你好", "n": float64(3)},
	}
	data, err := json.Marshal(ev)
	if err != nil {
		t.Fatal(err)
	}
	var back ChannelEvent
	if err := json.Unmarshal(data, &back); err != nil {
		t.Fatal(err)
	}
	if back.Seq != 7 || back.StrField("text") != "你好" || len(back.To) != 1 || back.To[0] != "w1" {
		t.Errorf("往返后字段丢失：%+v", back)
	}
	if back.Meta["k"] != "v" {
		t.Errorf("meta 丢失：%v", back.Meta)
	}
	// 单目标写成字符串、多目标写成数组。
	if !strings.Contains(string(data), `"to":"w1"`) {
		t.Errorf("单目标应写成字符串：%s", string(data))
	}
	// 读旧日志：to 为字符串时也能解析。
	var old ChannelEvent
	if err := json.Unmarshal([]byte(`{"seq":1,"ts":"t","kind":"message","by":"a","to":"w1","text":"hi"}`), &old); err != nil {
		t.Fatal(err)
	}
	if len(old.To) != 1 || old.To[0] != "w1" || old.StrField("text") != "hi" {
		t.Errorf("旧格式 to 字符串解析失败：%+v", old)
	}
}

func TestSortEventsBySeq(t *testing.T) {
	events := []ChannelEvent{{Seq: 3}, {Seq: 1}, {Seq: 2}}
	sortEventsBySeq(events)
	if events[0].Seq != 1 || events[1].Seq != 2 || events[2].Seq != 3 {
		t.Errorf("排序失败：%v", events)
	}
}

func TestParseHelpers(t *testing.T) {
	if _, err := ParseChannelType("thread"); err == nil {
		t.Error("新建时 type:thread 应报错")
	}
	if _, err := ParseChannelType("forum"); err != nil {
		t.Errorf("forum 应合法：%v", err)
	}
	if _, err := ParseInboxPolicy("all"); err == nil {
		t.Error("非法 inbox 策略应报错")
	}
	if _, err := ParseDeliveryMode("strict"); err == nil {
		t.Error("非法投递模式应报错")
	}
	kinds, err := ParseChannelKinds("message, done,message")
	if err != nil || len(kinds) != 2 {
		t.Errorf("kind 列表应去重为 2 个，得到 %v（err=%v）", kinds, err)
	}
	if _, err := ParseChannelKinds("nope"); err == nil {
		t.Error("非法 kind 应报错")
	}
	if _, err := NormalizeThreadKey("有 空格"); err == nil {
		t.Error("含空格的 thread 键应报错")
	}
	if _, err := BuildContextEntries([]string{"相对路径/x.md"}, nil); err == nil {
		t.Error("相对路径上下文文件应报错")
	}
	entries, err := BuildContextEntries([]string{"/tmp/x.md"}, []string{"说明"})
	if err != nil || len(entries) != 2 {
		t.Errorf("上下文条目构造失败：%v %v", entries, err)
	}
	if ContextEntryKey(entries[0]) != "file:/tmp/x.md" {
		t.Errorf("文件条目键错误：%q", ContextEntryKey(entries[0]))
	}
}
