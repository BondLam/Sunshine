package store

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"
)

// TestConcurrentAppendGoroutines 多 goroutine 同时写同一 channel：
// seq 连续无丢失、无撕裂行。
func TestConcurrentAppendGoroutines(t *testing.T) {
	testRoot(t)
	ref := testRef("concurrent")
	const goroutines = 20
	const perGoroutine = 25
	var wg sync.WaitGroup
	errs := make(chan error, goroutines*perGoroutine)
	for g := 0; g < goroutines; g++ {
		wg.Add(1)
		go func(g int) {
			defer wg.Done()
			for i := 0; i < perGoroutine; i++ {
				_, err := AppendEvent(ref, AppendableEvent{
					Kind:  KindMessage,
					By:    fmt.Sprintf("worker-%d", g),
					Extra: map[string]any{"text": fmt.Sprintf("g%d-%d", g, i)},
				})
				if err != nil {
					errs <- err
				}
			}
		}(g)
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Fatalf("并发追加失败：%v", err)
	}
	events, err := ReadChannelEvents(ref, nil)
	if err != nil {
		t.Fatal(err)
	}
	total := goroutines * perGoroutine
	if len(events) != total {
		t.Fatalf("应有 %d 条事件，得到 %d（有丢失）", total, len(events))
	}
	seqs := make([]int64, 0, total)
	for _, ev := range events {
		seqs = append(seqs, ev.Seq)
	}
	sort.Slice(seqs, func(i, j int) bool { return seqs[i] < seqs[j] })
	for i, seq := range seqs {
		if seq != int64(i+1) {
			t.Fatalf("seq 不连续：第 %d 条为 %d", i+1, seq)
		}
	}
	// sidecar 与尾部一致。
	last, err := ReadLastSeq(ref)
	if err != nil || last != int64(total) {
		t.Fatalf("最后 seq 应为 %d，得到 %d（err=%v）", total, last, err)
	}
}

// TestMultiProcessAppend 多进程同时写同一 channel：测试二进制
// 以 SUNSHINE_STORE_APPEND_WORKER=1 重跑自身，子进程只追加事件。
func TestMultiProcessAppend(t *testing.T) {
	if os.Getenv("SUNSHINE_STORE_APPEND_WORKER") == "1" {
		// 子进程模式：追加 25 条事件后直接返回。
		ref := testRef("multiproc")
		for i := 0; i < 25; i++ {
			if _, err := AppendEvent(ref, AppendableEvent{
				Kind:  KindMessage,
				By:    os.Getenv("SUNSHINE_STORE_WORKER_ID"),
				Extra: map[string]any{"text": fmt.Sprintf("第 %d 条", i+1)},
			}); err != nil {
				t.Fatalf("子进程追加失败：%v", err)
			}
		}
		return
	}
	root := testRoot(t)
	const processes = 4
	var wg sync.WaitGroup
	errs := make(chan error, processes)
	for p := 0; p < processes; p++ {
		wg.Add(1)
		go func(p int) {
			defer wg.Done()
			cmd := exec.Command(os.Args[0], "-test.run", "^TestMultiProcessAppend$", "-test.count=1")
			cmd.Env = append(os.Environ(),
				"SUNSHINE_CHANNEL_ROOT="+root,
				"SUNSHINE_CHANNEL_PROJECT=",
				"SUNSHINE_STORE_APPEND_WORKER=1",
				fmt.Sprintf("SUNSHINE_STORE_WORKER_ID=proc-%d", p),
			)
			out, err := cmd.CombinedOutput()
			if err != nil {
				errs <- fmt.Errorf("子进程 %d 失败：%v\n%s", p, err, out)
			}
		}(p)
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Fatal(err)
	}
	ref := testRef("multiproc")
	events, err := ReadChannelEvents(ref, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != processes*25 {
		t.Fatalf("多进程应共写 %d 条，得到 %d", processes*25, len(events))
	}
	seqs := make([]int64, 0, len(events))
	for _, ev := range events {
		seqs = append(seqs, ev.Seq)
	}
	sort.Slice(seqs, func(i, j int) bool { return seqs[i] < seqs[j] })
	for i, seq := range seqs {
		if seq != int64(i+1) {
			t.Fatalf("多进程 seq 不连续：第 %d 条为 %d", i+1, seq)
		}
	}
	// 无撕裂行：每行都能解析（ReadChannelEvents 已跳过坏行，
	// 这里直接数文件行数应与事件数一致）。
	jsonl, _ := EventsPath(ref.Name, ref.Project)
	data, _ := os.ReadFile(jsonl)
	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	if len(lines) != processes*25 {
		t.Fatalf("文件行数应为 %d，得到 %d（有撕裂行）", processes*25, len(lines))
	}
}

func TestWatchChannelEvents(t *testing.T) {
	testRoot(t)
	ref := testRef("watch")
	// 先写一条历史事件：默认只推新事件，不应收到它。
	appendMsg(t, ref, "main", map[string]any{"text": "历史"})
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	out, errCh := WatchChannelEvents(ctx, ref, WatchOptions{})
	select {
	case err := <-errCh:
		t.Fatalf("监听出错：%v", err)
	case <-time.After(300 * time.Millisecond):
	}
	// 追加两条新事件，应陆续收到。
	appendMsg(t, ref, "main", map[string]any{"text": "新一"})
	appendMsg(t, ref, "main", map[string]any{"text": "新二"})
	received := []ChannelEvent{}
	timeout := time.After(3 * time.Second)
	for len(received) < 2 {
		select {
		case ev, ok := <-out:
			if !ok {
				t.Fatalf("监听 channel 提前关闭，已收到 %d 条", len(received))
			}
			received = append(received, ev)
		case err := <-errCh:
			t.Fatalf("监听出错：%v", err)
		case <-timeout:
			t.Fatalf("超时，只收到 %d 条新事件", len(received))
		}
	}
	if received[0].StrField("text") != "新一" || received[1].StrField("text") != "新二" {
		t.Errorf("收到事件内容错误：%v", received)
	}
	// SinceSeq 回读：应收到 seq > 1 的全部事件。
	ctx2, cancel2 := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel2()
	out2, _ := WatchChannelEvents(ctx2, ref, WatchOptions{HasSinceSeq: true, SinceSeq: 1})
	received = nil
	timeout = time.After(3 * time.Second)
	for len(received) < 2 {
		select {
		case ev, ok := <-out2:
			if !ok {
				t.Fatalf("回读监听提前关闭")
			}
			received = append(received, ev)
		case <-timeout:
			t.Fatalf("回读超时，只收到 %d 条", len(received))
		}
	}
	if received[0].Seq != 2 || received[1].Seq != 3 {
		t.Errorf("SinceSeq 回读 seq 错误：%d, %d", received[0].Seq, received[1].Seq)
	}
}

func TestWatchChannels(t *testing.T) {
	testRoot(t)
	project := "testproj"
	refA := ChannelRef{Name: "chan-a", Scope: ScopeProject, Project: project}
	refB := ChannelRef{Name: "chan-b", Scope: ScopeProject, Project: project}
	appendMsg(t, refA, "main", map[string]any{"text": "A 历史"})
	appendMsg(t, refB, "main", map[string]any{"text": "B 历史"})
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	out, errCh := WatchChannels(ctx, WatchChannelsInput{
		ProjectKey:           project,
		FromStartNewChannels: true,
		DiscoveryInterval:    100 * time.Millisecond,
	})
	// 初始扫描读积压：先收到两条历史事件。
	received := map[string]ChannelEvent{}
	timeout := time.After(5 * time.Second)
	for len(received) < 2 {
		select {
		case cross, ok := <-out:
			if !ok {
				t.Fatalf("跨 channel 监听提前关闭")
			}
			received[cross.Channel.Name] = cross.Event
		case err := <-errCh:
			t.Fatalf("监听出错：%v", err)
		case <-timeout:
			t.Fatalf("初始积压超时，只收到 %v", received)
		}
	}
	if received["chan-a"].StrField("text") != "A 历史" || received["chan-b"].StrField("text") != "B 历史" {
		t.Errorf("初始积压内容错误：%v", received)
	}
	appendMsg(t, refA, "main", map[string]any{"text": "A 新"})
	appendMsg(t, refB, "main", map[string]any{"text": "B 新"})
	received = map[string]ChannelEvent{}
	timeout = time.After(5 * time.Second)
	for len(received) < 2 {
		select {
		case cross, ok := <-out:
			if !ok {
				t.Fatalf("跨 channel 监听提前关闭")
			}
			received[cross.Channel.Name] = cross.Event
			// 游标快照应含该 channel 的最新 seq。
			key := ChannelCursorKey(cross.Channel)
			if cross.Cursor[key] != cross.Event.Seq {
				t.Errorf("游标快照错误：%v", cross.Cursor)
			}
		case err := <-errCh:
			t.Fatalf("监听出错：%v", err)
		case <-timeout:
			t.Fatalf("超时，只收到 %v", received)
		}
	}
	if received["chan-a"].StrField("text") != "A 新" || received["chan-b"].StrField("text") != "B 新" {
		t.Errorf("跨 channel 事件内容错误：%v", received)
	}
	// 监听开始后新建的 channel 应被动态发现。
	refC := ChannelRef{Name: "chan-c", Scope: ScopeProject, Project: project}
	appendMsg(t, refC, "main", map[string]any{"text": "C 新"})
	select {
	case cross, ok := <-out:
		if !ok {
			t.Fatal("监听提前关闭")
		}
		if cross.Channel.Name != "chan-c" || cross.Event.StrField("text") != "C 新" {
			t.Errorf("新发现 channel 事件错误：%+v", cross)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("超时未收到新建 channel 的事件")
	}
}
