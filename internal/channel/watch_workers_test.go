package channel

import (
	"context"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/linrong1982/Sunshine/internal/channel/store"
)

func TestWatchEventsLiveAndFromStart(t *testing.T) {
	testSetup(t)
	testCreate(t, "demo")
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()

	// 只推新事件：先监听，再发消息，应收到。
	evCh, errCh := WatchEvents(ctx, testAddr("demo"), WatchEventsOptions{
		PollInterval: 20 * time.Millisecond,
	})
	if _, err := SendMessage(SendMessageOptions{
		ChannelAddress: testAddr("demo"), By: "main", Text: "新消息",
	}); err != nil {
		t.Fatalf("发送失败：%v", err)
	}
	select {
	case ev := <-evCh:
		if ev.Kind != EventKindMessage || ev.Text != "新消息" {
			t.Fatalf("监听到的事件不正确：%+v", ev)
		}
	case err := <-errCh:
		t.Fatalf("监听出错：%v", err)
	case <-ctx.Done():
		t.Fatalf("监听超时未收到新事件")
	}

	// FromStart：回放全部历史事件。
	ctx2, cancel2 := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel2()
	evCh2, _ := WatchEvents(ctx2, testAddr("demo"), WatchEventsOptions{
		FromStart:    true,
		PollInterval: 20 * time.Millisecond,
	})
	var kinds []EventKind
	timeout := time.After(3 * time.Second)
	for len(kinds) < 2 {
		select {
		case ev := <-evCh2:
			kinds = append(kinds, ev.Kind)
		case <-timeout:
			t.Fatalf("FromStart 回放超时，只收到 %+v", kinds)
		}
	}
	if kinds[0] != EventKindCreate || kinds[1] != EventKindMessage {
		t.Errorf("FromStart 回放顺序不正确：%+v", kinds)
	}
}

func TestWatchChannelsDiscoversNewChannel(t *testing.T) {
	testSetup(t)
	testCreate(t, "first")
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()

	evCh, errCh := WatchChannels(ctx, WatchChannelsOptions{
		ProjectKey:           "testproj",
		FromStartNewChannels: true,
		DiscoveryIntervalMs:  50,
	})
	// 新建第二个 channel 并发消息，应被动态发现。
	testCreate(t, "second")
	if _, err := SendMessage(SendMessageOptions{
		ChannelAddress: testAddr("second"), By: "main", Text: "hello",
	}); err != nil {
		t.Fatalf("发送失败：%v", err)
	}
	timeout := time.After(5 * time.Second)
	for {
		select {
		case ev := <-evCh:
			if ev.Channel.Name == "second" && ev.Event.Kind == EventKindMessage {
				if ev.Cursor == nil || ev.Cursor[ChannelCursorKey(ev.Channel)] == 0 {
					t.Errorf("跨 channel 事件应带游标快照：%+v", ev.Cursor)
				}
				return
			}
		case err := <-errCh:
			t.Fatalf("监听出错：%v", err)
		case <-timeout:
			t.Fatalf("跨 channel 监听超时")
		case <-ctx.Done():
			t.Fatalf("监听上下文超时")
		}
	}
}

func TestWatchWorkersSnapshots(t *testing.T) {
	testSetup(t)
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	testCreate(t, "demo")
	runtime := NewStubRuntime()
	testSpawn(t, ctx, "demo", "w1", runtime)

	snapCh, errCh := WatchWorkers(ctx, WatchWorkersOptions{
		ListWorkersOptions: ListWorkersOptions{ChannelAddress: testAddr("demo")},
	})
	select {
	case snap := <-snapCh:
		if len(snap) != 1 || snap[0].WorkerID != "w1" {
			t.Fatalf("首个快照不正确：%+v", snap)
		}
	case err := <-errCh:
		t.Fatalf("监听出错：%v", err)
	case <-time.After(3 * time.Second):
		t.Fatalf("首个快照超时")
	}
	// 杀掉 worker 后应收到空快照。
	appendStoreEvent(t, "demo", store.AppendableEvent{
		Kind: store.KindKilled, By: "supervisor:w1",
		Extra: map[string]any{"worker": "w1", "reason": "explicit-kill"},
	})
	select {
	case snap := <-snapCh:
		if len(snap) != 0 {
			t.Fatalf("终态后快照应为空，得到 %+v", snap)
		}
	case err := <-errCh:
		t.Fatalf("监听出错：%v", err)
	case <-time.After(3 * time.Second):
		t.Fatalf("终态快照超时")
	}
}

func TestWatchWorkerInbox(t *testing.T) {
	testSetup(t)
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	testCreate(t, "demo")
	runtime := NewStubRuntime()
	testSpawn(t, ctx, "demo", "w1", runtime)

	msgCh, errCh := WatchWorkerInbox(ctx, WatchWorkerInboxOptions{
		ChannelAddress: testAddr("demo"), WorkerID: "w1",
	})
	if _, err := SendMessage(SendMessageOptions{
		ChannelAddress: testAddr("demo"), By: "main", Text: "开工", To: []string{"w1"},
	}); err != nil {
		t.Fatalf("发送失败：%v", err)
	}
	select {
	case msg := <-msgCh:
		if msg.Event.Text != "开工" || msg.WorkerID != "w1" {
			t.Fatalf("收件箱消息不正确：%+v", msg)
		}
	case err := <-errCh:
		t.Fatalf("监听出错：%v", err)
	case <-time.After(3 * time.Second):
		t.Fatalf("收件箱监听超时")
	}
	// 终态事件到达后流结束。
	appendStoreEvent(t, "demo", store.AppendableEvent{
		Kind: store.KindKilled, By: "supervisor:w1",
		Extra: map[string]any{"worker": "w1", "reason": "explicit-kill"},
	})
	select {
	case _, ok := <-msgCh:
		if ok {
			t.Fatalf("终态后收件箱流应关闭")
		}
	case <-time.After(3 * time.Second):
		t.Fatalf("终态后收件箱流未关闭")
	}
	// 监听不存在的 worker 应直接报错。
	_, errCh2 := WatchWorkerInbox(ctx, WatchWorkerInboxOptions{
		ChannelAddress: testAddr("demo"), WorkerID: "ghost",
	})
	select {
	case err := <-errCh2:
		if err == nil {
			t.Fatalf("监听不存在的 worker 应报错")
		}
	case <-time.After(3 * time.Second):
		t.Fatalf("监听不存在的 worker 未报错")
	}
}

func TestListProbeReconcile(t *testing.T) {
	testSetup(t)
	ctx := t.Context()
	testCreate(t, "demo")
	runtime := NewStubRuntime()
	testSpawn(t, ctx, "demo", "w1", runtime)
	testSpawn(t, ctx, "demo", "w2", runtime)

	// 写 pid 文件：w1 的 supervisor 已死，w2 的活着。
	ref := testRef("demo")
	writePID(t, ref, "w1", "pid", deadPID(t))
	writePID(t, ref, "w1", "worker-pid", deadPID(t))
	writePID(t, ref, "w2", "pid", os.Getpid())
	writePID(t, ref, "w2", "worker-pid", os.Getpid())

	observations, err := ProbeWorkerRuntime(testAddr("demo"))
	if err != nil {
		t.Fatalf("探测失败：%v", err)
	}
	if len(observations) != 2 {
		t.Fatalf("应观测到 2 个 worker，得到 %+v", observations)
	}
	obsByID := map[string]WorkerRuntimeObservation{}
	for _, obs := range observations {
		obsByID[obs.WorkerID] = obs
	}
	if obsByID["w1"].SupervisorAlive == nil || *obsByID["w1"].SupervisorAlive {
		t.Errorf("w1 的 supervisor 应为已死：%+v", obsByID["w1"])
	}
	if obsByID["w2"].SupervisorAlive == nil || !*obsByID["w2"].SupervisorAlive {
		t.Errorf("w2 的 supervisor 应为存活：%+v", obsByID["w2"])
	}
	if obsByID["w2"].Source != "local-pid-files" {
		t.Errorf("观测来源应为 local-pid-files，得到 %q", obsByID["w2"].Source)
	}

	// 默认只提议不写盘：只对 w1 提议一条 error 事件。
	result, err := ReconcileWorkerLiveness(ReconcileWorkerLivenessOptions{
		ChannelAddress: testAddr("demo"),
	})
	if err != nil {
		t.Fatalf("对账失败：%v", err)
	}
	if len(result.ProposedEvents) != 1 || result.ProposedEvents[0].Worker != "w1" {
		t.Fatalf("应只对 w1 提议终态事件，得到 %+v", result.ProposedEvents)
	}
	if len(result.Appended) != 0 {
		t.Errorf("默认不应写盘，得到 %+v", result.Appended)
	}
	events, _ := ReadEvents(ReadEventsOptions{ChannelAddress: testAddr("demo")})
	before := len(events)

	// 追加落盘：w1 变为 error 终态，w2 不受影响。
	result, err = ReconcileWorkerLiveness(ReconcileWorkerLivenessOptions{
		ChannelAddress:       testAddr("demo"),
		AppendTerminalEvents: true,
	})
	if err != nil {
		t.Fatalf("对账追加失败：%v", err)
	}
	if len(result.Appended) != 1 {
		t.Fatalf("应追加 1 条终态事件，得到 %+v", result.Appended)
	}
	events, _ = ReadEvents(ReadEventsOptions{ChannelAddress: testAddr("demo")})
	if len(events) != before+1 {
		t.Errorf("追加后事件数应 +1，之前 %d、之后 %d", before, len(events))
	}
	workers, _ := ListWorkers(ListWorkersOptions{
		ChannelAddress: testAddr("demo"), IncludeTerminal: true,
	})
	byID := map[string]WorkerState{}
	for _, w := range workers {
		byID[w.WorkerID] = w
	}
	if byID["w1"].Lifecycle != WorkerLifecycleError || !byID["w1"].Terminal {
		t.Errorf("w1 对账后应为 error 终态：%+v", byID["w1"])
	}
	if byID["w2"].Terminal {
		t.Errorf("w2 不应变终态：%+v", byID["w2"])
	}
	// 再次对账：w1 已终态，不再提议。
	result, _ = ReconcileWorkerLiveness(ReconcileWorkerLivenessOptions{
		ChannelAddress: testAddr("demo"),
	})
	if len(result.ProposedEvents) != 0 {
		t.Errorf("再次对账不应再提议，得到 %+v", result.ProposedEvents)
	}
}

func writePID(t *testing.T, ref store.ChannelRef, worker, suffix string, pid int) {
	t.Helper()
	path, err := store.WorkerFile(ref.Name, worker, suffix, ref.Project)
	if err != nil {
		t.Fatalf("取 pid 文件路径失败：%v", err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatalf("建目录失败：%v", err)
	}
	if err := os.WriteFile(path, []byte(strconv.Itoa(pid)+"\n"), 0o644); err != nil {
		t.Fatalf("写 pid 文件失败：%v", err)
	}
}
