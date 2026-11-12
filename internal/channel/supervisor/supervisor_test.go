package supervisor

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/BondLam/Sunshine/internal/channel/adapters"
	"github.com/BondLam/Sunshine/internal/channel/store"
)

// testSetup 建一个隔离的 channel 存储根与项目桶，返回频道引用。
func testSetup(t *testing.T, channel string) store.ChannelRef {
	t.Helper()
	root := t.TempDir()
	t.Setenv("SUNSHINE_CHANNEL_ROOT", root)
	t.Setenv("SUNSHINE_CHANNEL_PROJECT", "test-proj")
	ref := store.ChannelRef{Name: channel, Scope: store.ScopeProject, Project: "test-proj"}
	if _, err := store.AppendEvent(ref, store.AppendableEvent{
		Kind: store.KindCreate, By: "main",
		Extra: map[string]any{"type": "chat"},
	}); err != nil {
		t.Fatalf("建测试频道失败：%v", err)
	}
	return ref
}

func readEvents(t *testing.T, ref store.ChannelRef) []store.ChannelEvent {
	t.Helper()
	events, err := store.ReadChannelEvents(ref, nil)
	if err != nil {
		t.Fatalf("读事件失败：%v", err)
	}
	return events
}

func waitForEvent(t *testing.T, ref store.ChannelRef, kind store.ChannelEventKind, timeout time.Duration) store.ChannelEvent {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		for _, ev := range readEvents(t, ref) {
			if ev.Kind == kind {
				return ev
			}
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("在 %v 内未等到 %s 事件，已有事件：%v", timeout, kind, eventKinds(readEvents(t, ref)))
	return store.ChannelEvent{}
}

func waitForKindCount(t *testing.T, ref store.ChannelRef, kind store.ChannelEventKind, n int, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		count := 0
		for _, ev := range readEvents(t, ref) {
			if ev.Kind == kind {
				count++
			}
		}
		if count >= n {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("在 %v 内未等到 %d 个 %s 事件，已有：%v", timeout, n, kind, eventKinds(readEvents(t, ref)))
}

func eventKinds(events []store.ChannelEvent) []string {
	out := make([]string, len(events))
	for i, ev := range events {
		out[i] = string(ev.Kind)
	}
	return out
}

func claudeTurns() [][]string {
	return [][]string{{
		`{"type":"system","subtype":"init","session_id":"stub-session-1"}`,
		`{"type":"assistant","message":{"role":"assistant","content":[{"type":"text","text":"收到，正在做。"}]}}`,
		`{"type":"result","subtype":"success","is_error":false,"duration_ms":12,"num_turns":1,"result":"收到，正在做。"}`,
	}}
}

func stubFactory(turns [][]string) RuntimeFactory {
	return func(provider adapters.Provider, sink adapters.EventSink) (adapters.WorkerRuntime, error) {
		return adapters.NewStubWorkerRuntime(provider, sink, turns)
	}
}

// TestSupervisorClosedLoop 验证完整闭环：
// spawn → send --to → worker 工作 → done，含 turn_started/turn_finished
// 与 session 身份桥接（session-id 文件）。
func TestSupervisorClosedLoop(t *testing.T) {
	ref := testSetup(t, "demo")
	workdir := t.TempDir()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	errCh := make(chan error, 1)
	go func() {
		errCh <- RunSupervisor(ctx, ref, "implement", SupervisorConfig{
			Provider:      "claude",
			Cwd:           workdir,
			SpawnedBy:     "main",
			IdleTimeoutMs: 0,
		}, WithRuntimeFactory(stubFactory(claudeTurns())))
	}()

	spawned := waitForEvent(t, ref, store.KindSpawned, 5*time.Second)
	if spawned.StrField("as") != "implement" || spawned.StrField("provider") != "claude" {
		t.Fatalf("spawned 事件字段不对：%+v", spawned.Extra)
	}
	if spawned.StrField("inboxPolicy") != "explicitOnly" {
		t.Fatalf("spawned 应记默认 inboxPolicy explicitOnly：%+v", spawned.Extra)
	}

	// 发一条定向消息（相当于 send --to implement）。
	if _, err := store.AppendEvent(ref, store.AppendableEvent{
		Kind: store.KindMessage, By: "main", To: []string{"implement"},
		Extra: map[string]any{"text": "实现登录页"},
	}); err != nil {
		t.Fatalf("发消息失败：%v", err)
	}
	// worker 的回答与 done。turn_finished 由 supervisorSink 在 done
	// 落盘后追加，两者之间有窗口：只等 done 就读事件，可能在 done 已见、
	// turn_finished 未落盘时误判（CI 曾因此抖动失败），所以两个都等到再断言。
	done := waitForEvent(t, ref, store.KindDone, 5*time.Second)
	_ = done
	waitForEvent(t, ref, store.KindTurnFinished, 5*time.Second)
	events := readEvents(t, ref)
	var sawText, sawTurnStarted, sawTurnFinished bool
	for _, ev := range events {
		if ev.Kind == store.KindMessage && ev.By == "implement" && strings.Contains(ev.StrField("text"), "正在做") {
			sawText = true
		}
		if ev.Kind == store.KindTurnStarted {
			sawTurnStarted = true
		}
		if ev.Kind == store.KindTurnFinished && ev.StrField("outcome") == "done" {
			sawTurnFinished = true
		}
	}
	if !sawText {
		t.Fatalf("未见 worker 的 message 事件：%v", eventKinds(events))
	}
	if !sawTurnStarted || !sawTurnFinished {
		t.Fatalf("应有 turn_started 与 turn_finished(done)：%v", eventKinds(events))
	}
	// session 身份桥接：init 行的 session_id 写进了 worker 文件。
	sessionID, err := ReadSessionID(ref, "implement")
	if err != nil {
		t.Fatalf("读 session-id 失败：%v", err)
	}
	if sessionID != "stub-session-1" {
		t.Fatalf("session-id 文件应为 stub-session-1，实际 %q", sessionID)
	}
	// 广播消息不应投递给 explicitOnly 的 worker：再发一条广播，
	// 等 300ms 后确认事件数不再增加（无新 turn_started）。
	before := len(readEvents(t, ref))
	if _, err := store.AppendEvent(ref, store.AppendableEvent{
		Kind: store.KindMessage, By: "main",
		Extra: map[string]any{"text": "广播"},
	}); err != nil {
		t.Fatalf("发广播失败：%v", err)
	}
	time.Sleep(300 * time.Millisecond)
	after := readEvents(t, ref)
	turnStarted := 0
	for _, ev := range after {
		if ev.Kind == store.KindTurnStarted {
			turnStarted++
		}
	}
	if turnStarted != 1 {
		t.Fatalf("广播不应触发新回合，turn_started 应仍为 1，实际 %d（事件 %d → %d）", turnStarted, before, len(after))
	}

	// 取消上下文 → supervisor 关停 worker，写 killed 事件后退出。
	cancel()
	select {
	case err := <-errCh:
		if err != nil {
			t.Fatalf("RunSupervisor 应正常返回，实际 %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("取消后 supervisor 在 5 秒内未退出")
	}
	killed := waitForEvent(t, ref, store.KindKilled, 5*time.Second)
	if killed.StrField("reason") != "explicit-kill" {
		t.Fatalf("killed 原因应为 explicit-kill，实际 %q", killed.StrField("reason"))
	}
	// 临时文件已清理，log 与 session-id 保留。
	for _, suffix := range []string{"pid", "worker-pid", "config", "reservation"} {
		p, _ := store.WorkerFile(ref.Name, "implement", suffix, ref.Project)
		if _, err := os.Stat(p); !os.IsNotExist(err) {
			t.Fatalf("退出后 %s 应已删除", suffix)
		}
	}
	sessionID, _ = ReadSessionID(ref, "implement")
	if sessionID != "stub-session-1" {
		t.Fatalf("退出后 session-id 应保留，实际 %q", sessionID)
	}
}

// TestSupervisorBroadcastPolicy 验证 broadcastAndExplicit 能消费广播。
func TestSupervisorBroadcastPolicy(t *testing.T) {
	ref := testSetup(t, "bc")
	workdir := t.TempDir()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	errCh := make(chan error, 1)
	go func() {
		errCh <- RunSupervisor(ctx, ref, "check", SupervisorConfig{
			Provider:    "claude",
			Cwd:         workdir,
			InboxPolicy: "broadcastAndExplicit",
		}, WithRuntimeFactory(stubFactory(claudeTurns())))
	}()
	waitForEvent(t, ref, store.KindSpawned, 5*time.Second)
	if _, err := store.AppendEvent(ref, store.AppendableEvent{
		Kind: store.KindMessage, By: "main",
		Extra: map[string]any{"text": "广播任务"},
	}); err != nil {
		t.Fatalf("发广播失败：%v", err)
	}
	waitForEvent(t, ref, store.KindDone, 5*time.Second)
	cancel()
	select {
	case err := <-errCh:
		if err != nil {
			t.Fatalf("RunSupervisor 应正常返回，实际 %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("取消后 supervisor 在 5 秒内未退出")
	}
}

// TestSupervisorIdleTimeout 验证空闲超时回收：
// worker 空闲超过 TTL 后 supervisor 自行关停，killed 原因 idle-timeout。
func TestSupervisorIdleTimeout(t *testing.T) {
	ref := testSetup(t, "idle")
	workdir := t.TempDir()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	errCh := make(chan error, 1)
	go func() {
		errCh <- RunSupervisor(ctx, ref, "implement", SupervisorConfig{
			Provider:      "claude",
			Cwd:           workdir,
			IdleTimeoutMs: 300,
		}, WithRuntimeFactory(stubFactory(claudeTurns())))
	}()
	waitForEvent(t, ref, store.KindSpawned, 5*time.Second)
	killed := waitForEvent(t, ref, store.KindKilled, 5*time.Second)
	if killed.StrField("reason") != "idle-timeout" {
		t.Fatalf("killed 原因应为 idle-timeout，实际 %q", killed.StrField("reason"))
	}
	select {
	case err := <-errCh:
		if err != nil {
			t.Fatalf("RunSupervisor 应正常返回，实际 %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("空闲超时后 supervisor 在 5 秒内未退出")
	}
}

// TestSupervisorWarnBefore 验证 warn-before 预警：
// 超时前发一条 supervisor_warning，超时到达后发 killed(timeout)。
func TestSupervisorWarnBefore(t *testing.T) {
	ref := testSetup(t, "warn")
	workdir := t.TempDir()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	warnBefore := int64(800)
	errCh := make(chan error, 1)
	go func() {
		errCh <- RunSupervisor(ctx, ref, "implement", SupervisorConfig{
			Provider:     "claude",
			Cwd:          workdir,
			TimeoutMs:    1500,
			WarnBeforeMs: &warnBefore,
		}, WithRuntimeFactory(stubFactory(claudeTurns())))
	}()
	waitForEvent(t, ref, store.KindSpawned, 5*time.Second)
	warning := waitForEvent(t, ref, store.KindSupervisorWarning, 5*time.Second)
	if warning.StrField("reason") != "approaching_timeout" {
		t.Fatalf("预警原因应为 approaching_timeout，实际 %q", warning.StrField("reason"))
	}
	if ms, ok := warning.NumField("remaining_ms"); !ok || ms != 800 {
		t.Fatalf("预警 remaining_ms 应为 800，实际 %v", warning.Extra["remaining_ms"])
	}
	killed := waitForEvent(t, ref, store.KindKilled, 5*time.Second)
	if killed.StrField("reason") != "timeout" {
		t.Fatalf("killed 原因应为 timeout，实际 %q", killed.StrField("reason"))
	}
	select {
	case err := <-errCh:
		if err != nil {
			t.Fatalf("RunSupervisor 应正常返回，实际 %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("超时后 supervisor 在 5 秒内未退出")
	}
}

// TestSupervisorWarnBeforeDisabled 验证 warnBefore 显式 0 时禁用预警。
func TestSupervisorWarnBeforeDisabled(t *testing.T) {
	ref := testSetup(t, "warn0")
	workdir := t.TempDir()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	zero := int64(0)
	errCh := make(chan error, 1)
	go func() {
		errCh <- RunSupervisor(ctx, ref, "implement", SupervisorConfig{
			Provider:     "claude",
			Cwd:          workdir,
			TimeoutMs:    800,
			WarnBeforeMs: &zero,
		}, WithRuntimeFactory(stubFactory(claudeTurns())))
	}()
	waitForEvent(t, ref, store.KindSpawned, 5*time.Second)
	waitForEvent(t, ref, store.KindKilled, 5*time.Second)
	for _, ev := range readEvents(t, ref) {
		if ev.Kind == store.KindSupervisorWarning {
			t.Fatal("warnBefore=0 时不应发 supervisor_warning")
		}
	}
	select {
	case err := <-errCh:
		if err != nil {
			t.Fatalf("RunSupervisor 应正常返回，实际 %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("超时后 supervisor 在 5 秒内未退出")
	}
}

// TestSupervisorInterruptIdle 验证空闲时中断：
// interrupt_requested → interrupted 事件（outcome no-active-turn），
// stub 收到中断文本并开始新回合（发 done）。
func TestSupervisorInterruptIdle(t *testing.T) {
	ref := testSetup(t, "intr")
	workdir := t.TempDir()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var stub *adapters.StubWorkerRuntime
	factory := func(provider adapters.Provider, sink adapters.EventSink) (adapters.WorkerRuntime, error) {
		s, err := adapters.NewStubWorkerRuntime(provider, sink, claudeTurns())
		if err != nil {
			return nil, err
		}
		stub = s
		return s, nil
	}
	errCh := make(chan error, 1)
	go func() {
		errCh <- RunSupervisor(ctx, ref, "implement", SupervisorConfig{
			Provider: "claude",
			Cwd:      workdir,
		}, WithRuntimeFactory(factory))
	}()
	waitForEvent(t, ref, store.KindSpawned, 5*time.Second)
	if _, err := store.AppendEvent(ref, store.AppendableEvent{
		Kind: store.KindInterruptRequested, By: "main",
		Extra: map[string]any{"worker": "implement", "message": "停下，改做 B"},
	}); err != nil {
		t.Fatalf("发中断请求失败：%v", err)
	}
	interrupted := waitForEvent(t, ref, store.KindInterrupted, 5*time.Second)
	if interrupted.StrField("outcome") != "no-active-turn" {
		t.Fatalf("空闲中断 outcome 应为 no-active-turn，实际 %q", interrupted.StrField("outcome"))
	}
	if interrupted.StrField("method") != "stdin" || interrupted.StrField("reason") != "user" {
		t.Fatalf("interrupted 字段不对：%+v", interrupted.Extra)
	}
	// 中断本身开启新回合：stub 收到中断文本并播放一回合，产生 done。
	waitForEvent(t, ref, store.KindDone, 5*time.Second)
	if stub == nil {
		t.Fatal("stub 未创建")
	}
	found := false
	for _, s := range stub.Interrupts() {
		if s == "停下，改做 B" {
			found = true
		}
	}
	if !found {
		t.Fatalf("stub 应收到中断文本，实际 %v", stub.Interrupts())
	}
	// 发给其他 worker 的中断不应被本 worker 消费。
	before := len(readEvents(t, ref))
	if _, err := store.AppendEvent(ref, store.AppendableEvent{
		Kind: store.KindInterruptRequested, By: "main",
		Extra: map[string]any{"worker": "other", "message": "别理我"},
	}); err != nil {
		t.Fatalf("发中断请求失败：%v", err)
	}
	time.Sleep(300 * time.Millisecond)
	after := readEvents(t, ref)
	interruptedCount := 0
	for _, ev := range after {
		if ev.Kind == store.KindInterrupted {
			interruptedCount++
		}
	}
	if interruptedCount != 1 || len(after) != before+1 {
		t.Fatalf("他人中断不应被消费：interrupted %d 个，事件 %d → %d", interruptedCount, before, len(after))
	}
	cancel()
	select {
	case err := <-errCh:
		if err != nil {
			t.Fatalf("RunSupervisor 应正常返回，实际 %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("取消后 supervisor 在 5 秒内未退出")
	}
}

// TestHandleInboxEventInterruptAbortsTurn 验证回合中中断的单元行为：
// 有进行中回合时，中断先以 aborted 收尾该回合，再发 interrupted（outcome interrupted）。
func TestHandleInboxEventInterruptAbortsTurn(t *testing.T) {
	ref := testSetup(t, "intr2")
	tracker := NewTurnTracker(TurnTrackerHooks{})
	shutdown := NewShutdownController(ref, "implement", nil, nil, 0, 0)
	sink := &recordingSinkForTest{}
	stub, err := adapters.NewStubWorkerRuntime(adapters.ProviderClaude, sink, claudeTurns())
	if err != nil {
		t.Fatalf("建 stub 失败：%v", err)
	}
	if _, err := stub.Start(context.Background(), adapters.StartOptions{Provider: adapters.ProviderClaude, Worker: "implement"}); err != nil {
		t.Fatalf("stub Start 失败：%v", err)
	}
	// 先模拟一条消息开启回合。
	msg := store.ChannelEvent{Seq: 5, Kind: store.KindMessage, By: "main", To: []string{"implement"}}
	msg.Extra = map[string]any{"text": "做 A"}
	delivered, done := handleInboxEvent(context.Background(), ref, "implement", store.DefaultInboxPolicy, stub, tracker, msg)
	if !delivered || done {
		t.Fatalf("消息应投递成功：delivered=%v done=%v", delivered, done)
	}
	if _, ok := tracker.Current(); !ok {
		t.Fatal("消息投递后应有进行中回合")
	}
	// 再发中断：应 abort 回合。
	interrupt := store.ChannelEvent{Seq: 6, Kind: store.KindInterruptRequested, By: "main"}
	interrupt.Extra = map[string]any{"worker": "implement", "message": "改做 B"}
	delivered, done = handleInboxEvent(context.Background(), ref, "implement", store.DefaultInboxPolicy, stub, tracker, interrupt)
	if !delivered || done {
		t.Fatalf("中断应投递成功：delivered=%v done=%v", delivered, done)
	}
	events := readEvents(t, ref)
	var sawAborted, sawInterrupted bool
	for _, ev := range events {
		if ev.Kind == store.KindTurnFinished && ev.StrField("outcome") == "aborted" {
			sawAborted = true
		}
		if ev.Kind == store.KindInterrupted && ev.StrField("outcome") == "interrupted" {
			sawInterrupted = true
		}
	}
	if !sawAborted || !sawInterrupted {
		t.Fatalf("应有 aborted 的 turn_finished 与 interrupted(interrupted)：%v", eventKinds(events))
	}
	_ = shutdown
}

// TestSupervisorSynthesizedDone 验证冷退出兜底：
// worker 进程直接退出且适配器未发终态事件时，supervisor 合成 done（exit 0）。
// TestSupervisorSigtermGracefulShutdown 验证进程入口的信号接管：
// RunSupervisorFromConfigFile 收到 SIGTERM 时应走优雅关停
// （killed explicit-kill 事件 + 临时文件清理），而不是直接被杀掉。
// 注意：signal.NotifyContext 会拦截发给测试进程的 SIGTERM，
// 因此这里直接向自己发信号是安全的。
func TestSupervisorSigtermGracefulShutdown(t *testing.T) {
	ref := testSetup(t, "sigterm")
	workdir := t.TempDir()
	cfgPath, err := WriteSupervisorConfig("sigterm", "implement", SupervisorConfig{
		Provider:      "claude",
		Cwd:           workdir,
		SpawnedBy:     "main",
		IdleTimeoutMs: 0,
	}, "test-proj")
	if err != nil {
		t.Fatalf("写配置失败：%v", err)
	}
	errCh := make(chan error, 1)
	go func() {
		errCh <- RunSupervisorFromConfigFile(context.Background(), "sigterm", "implement", cfgPath,
			WithRuntimeFactory(stubFactory(claudeTurns())))
	}()
	waitForEvent(t, ref, store.KindSpawned, 5*time.Second)
	// 向测试进程自己发中断信号（被 NotifyContext 拦截，转成 ctx 取消）。
	// 用 os.Interrupt 而不用 syscall.Kill 发 SIGTERM：二者走同一条
	// NotifyContext 路径，且 os.Interrupt 在 Windows 上也能编译运行。
	self, err := os.FindProcess(os.Getpid())
	if err != nil {
		t.Fatalf("取自身进程失败：%v", err)
	}
	if err := self.Signal(os.Interrupt); err != nil {
		t.Fatalf("发中断信号失败：%v", err)
	}
	select {
	case err := <-errCh:
		if err != nil {
			t.Fatalf("SIGTERM 后 supervisor 应正常返回，实际 %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("SIGTERM 后 supervisor 5 秒内未返回")
	}
	events := readEvents(t, ref)
	found := false
	for _, ev := range events {
		if ev.Kind == store.KindKilled && ev.StrField("reason") == "explicit-kill" {
			found = true
		}
	}
	if !found {
		t.Fatalf("SIGTERM 后应有 killed explicit-kill 事件：%v", eventKinds(events))
	}
	pidPath, _ := store.WorkerFile("sigterm", "implement", "pid", "test-proj")
	if _, err := os.Stat(pidPath); !os.IsNotExist(err) {
		t.Fatal("关停后 pid 文件应已删除")
	}
}

func TestSupervisorSynthesizedDone(t *testing.T) {
	ref := testSetup(t, "synth")
	// 建一个无终态、无关停原因的 shutdown 控制器，调 FinalizeOnExit(0)。
	shutdown := NewShutdownController(ref, "implement", nil, nil, 0, 0)
	if err := shutdown.FinalizeOnExit(0, ""); err != nil {
		t.Fatalf("FinalizeOnExit 失败：%v", err)
	}
	events := readEvents(t, ref)
	found := false
	for _, ev := range events {
		if ev.Kind == store.KindDone && ev.BoolField("synthesized") {
			found = true
			if code, ok := ev.NumField("exit_code"); !ok || code != 0 {
				t.Fatalf("合成 done 应带 exit_code=0，实际 %v", ev.Extra)
			}
		}
	}
	if !found {
		t.Fatalf("冷退出 0 应合成 done 事件：%v", eventKinds(events))
	}
	// 非 0 退出合成 error。
	ref2 := testSetup(t, "synth2")
	shutdown2 := NewShutdownController(ref2, "implement", nil, nil, 0, 0)
	if err := shutdown2.FinalizeOnExit(3, ""); err != nil {
		t.Fatalf("FinalizeOnExit 失败：%v", err)
	}
	events = readEvents(t, ref2)
	found = false
	for _, ev := range events {
		if ev.Kind == store.KindError && ev.BoolField("synthesized") {
			found = true
		}
	}
	if !found {
		t.Fatalf("冷退出非 0 应合成 error 事件：%v", eventKinds(events))
	}
}

// TestSupervisorSpawnFailure 验证拉起失败：
// runtime.Start 报错时写 error 事件、不写 spawned，RunSupervisor 返回错误。
func TestSupervisorSpawnFailure(t *testing.T) {
	ref := testSetup(t, "spawnfail")
	workdir := t.TempDir()
	// Start 恒失败的工厂。
	factory := func(provider adapters.Provider, sink adapters.EventSink) (adapters.WorkerRuntime, error) {
		return &failingRuntime{}, nil
	}
	err := RunSupervisor(context.Background(), ref, "implement", SupervisorConfig{
		Provider: "claude",
		Cwd:      workdir,
	}, WithRuntimeFactory(factory))
	if err == nil {
		t.Fatal("拉起失败时 RunSupervisor 应返回错误")
	}
	events := readEvents(t, ref)
	for _, ev := range events {
		if ev.Kind == store.KindSpawned {
			t.Fatal("拉起失败时不应写 spawned 事件")
		}
	}
	found := false
	for _, ev := range events {
		if ev.Kind == store.KindError && ev.By == "supervisor:implement" {
			found = true
		}
	}
	if !found {
		t.Fatalf("拉起失败时应写 supervisor 的 error 事件：%v", eventKinds(events))
	}
}

// failingRuntime 是 Start 恒失败的 runtime。
type failingRuntime struct{}

func (failingRuntime) Start(ctx context.Context, opts adapters.StartOptions) (adapters.StartResult, error) {
	return adapters.StartResult{}, fmt.Errorf("找不到 claude 二进制")
}
func (failingRuntime) Send(ctx context.Context, text string) error      { return nil }
func (failingRuntime) Interrupt(ctx context.Context, text string) error { return nil }
func (failingRuntime) Stop(ctx context.Context) error                   { return nil }
func (failingRuntime) Wait() error                                      { return nil }
func (failingRuntime) Ready() bool                                      { return false }
func (failingRuntime) ExitState() (int, string, bool)                   { return -1, "", false }

// recordingSinkForTest 是测试用的简易 sink（事件直接丢弃）。
type recordingSinkForTest struct{}

func (recordingSinkForTest) Emit(kind store.ChannelEventKind, by string, payload map[string]any) error {
	return nil
}
func (recordingSinkForTest) PersistSessionID(id string) error { return nil }
func (recordingSinkForTest) PersistThreadID(id string) error  { return nil }

// TestSupervisorSessionBridge 验证会话 id 读写桥接。
func TestSupervisorSessionBridge(t *testing.T) {
	ref := testSetup(t, "sess")
	if id, _ := ReadSessionID(ref, "implement"); id != "" {
		t.Fatalf("未写过时 session-id 应为空，实际 %q", id)
	}
	if err := WriteSessionID(ref, "implement", "sess-abc"); err != nil {
		t.Fatalf("写 session-id 失败：%v", err)
	}
	if id, _ := ReadSessionID(ref, "implement"); id != "sess-abc" {
		t.Fatalf("session-id 应为 sess-abc，实际 %q", id)
	}
	if err := WriteThreadID(ref, "check", "thread-xyz"); err != nil {
		t.Fatalf("写 thread-id 失败：%v", err)
	}
	if id, _ := ReadThreadID(ref, "check"); id != "thread-xyz" {
		t.Fatalf("thread-id 应为 thread-xyz，实际 %q", id)
	}
}
