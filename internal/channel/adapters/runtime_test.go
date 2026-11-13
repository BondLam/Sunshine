package adapters

import (
	"context"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/linrong007/Sunshine/internal/channel/store"
)

// recordingSink 记录 runtime 发出的事件与持久化的 id，供断言。
type recordingSink struct {
	mu        sync.Mutex
	events    []AdapterEvent
	sessionID string
	threadID  string
}

func (s *recordingSink) Emit(kind store.ChannelEventKind, by string, payload map[string]any) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.events = append(s.events, AdapterEvent{Kind: kind, Payload: payload})
	return nil
}

func (s *recordingSink) PersistSessionID(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.sessionID = id
	return nil
}

func (s *recordingSink) PersistThreadID(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.threadID = id
	return nil
}

func (s *recordingSink) kinds() []store.ChannelEventKind {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]store.ChannelEventKind, len(s.events))
	for i, e := range s.events {
		out[i] = e.Kind
	}
	return out
}

func (s *recordingSink) waitKind(t *testing.T, kind store.ChannelEventKind, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		for _, k := range s.kinds() {
			if k == kind {
				return
			}
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("在 %v 内未收到 %s 事件，已有 %v", timeout, kind, s.kinds())
}

// fakeClaudeScript 是一个模拟 claude 二进制的 shell 脚本：
// 启动即输出 init + 文本 + result 三行，随后等 stdin 关闭再退出。
const fakeClaudeScript = `#!/bin/sh
echo '{"type":"system","subtype":"init","session_id":"stub-session-1"}'
echo '{"type":"assistant","message":{"role":"assistant","content":[{"type":"text","text":"收到。"}]}}'
echo '{"type":"result","subtype":"success","is_error":false,"duration_ms":10,"num_turns":1,"result":"收到。"}'
cat > /dev/null
`

func writeFakeBinary(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "fake-claude")
	if err := os.WriteFile(path, []byte(content), 0o755); err != nil {
		t.Fatalf("写模拟二进制失败：%v", err)
	}
	return path
}

func TestProcessRuntimeClaudeFlow(t *testing.T) {
	sink := &recordingSink{}
	rt := NewClaudeWorkerRuntime(sink, nil)
	bin := writeFakeBinary(t, fakeClaudeScript)
	ctx := context.Background()
	res, err := rt.Start(ctx, StartOptions{
		Provider: ProviderClaude, Binary: bin, Worker: "implement",
		Channel: "demo", Cwd: t.TempDir(),
	})
	if err != nil {
		t.Fatalf("Start 失败：%v", err)
	}
	if res.PID <= 0 {
		t.Fatalf("应拿到子进程 pid，实际 %+v", res)
	}
	if !rt.Ready() {
		t.Fatal("claude 拉起后应就绪")
	}
	sink.waitKind(t, store.KindDone, 5*time.Second)
	if sink.sessionID != "stub-session-1" {
		t.Fatalf("应持久化 session_id，实际 %q", sink.sessionID)
	}
	kinds := sink.kinds()
	if len(kinds) != 2 || kinds[0] != store.KindMessage || kinds[1] != store.KindDone {
		t.Fatalf("事件顺序应为 message → done，实际 %v", kinds)
	}
	// Send 不应报错（脚本忽略 stdin）；随后 Stop 应让进程退出。
	if err := rt.Send(ctx, "继续"); err != nil {
		t.Fatalf("Send 失败：%v", err)
	}
	if err := rt.Stop(ctx); err != nil {
		t.Fatalf("Stop 失败：%v", err)
	}
	if err := rt.Wait(); err != nil {
		t.Fatalf("Wait 应返回 nil（脚本正常退出），实际 %v", err)
	}
	code, _, exited := rt.ExitState()
	if !exited || code != 0 {
		t.Fatalf("退出状态应为 code=0，实际 code=%d exited=%v", code, exited)
	}
}

func TestProcessRuntimeMissingBinary(t *testing.T) {
	sink := &recordingSink{}
	rt := NewClaudeWorkerRuntime(sink, nil)
	_, err := rt.Start(context.Background(), StartOptions{
		Provider: ProviderClaude, Binary: "/nonexistent/claude", Worker: "w",
	})
	if err == nil {
		t.Fatal("二进制不存在时 Start 应报错")
	}
}

func TestProcessRuntimeInterrupt(t *testing.T) {
	sink := &recordingSink{}
	rt := NewClaudeWorkerRuntime(sink, nil)
	bin := writeFakeBinary(t, fakeClaudeScript)
	ctx := context.Background()
	if _, err := rt.Start(ctx, StartOptions{Provider: ProviderClaude, Binary: bin, Worker: "w", Cwd: t.TempDir()}); err != nil {
		t.Fatalf("Start 失败：%v", err)
	}
	sink.waitKind(t, store.KindDone, 5*time.Second)
	// 中断应写 control_request + 替换指令进 stdin，不报错。
	if err := rt.Interrupt(ctx, "改做 B"); err != nil {
		t.Fatalf("Interrupt 失败：%v", err)
	}
	if err := rt.Stop(ctx); err != nil {
		t.Fatalf("Stop 失败：%v", err)
	}
	_ = rt.Wait()
}

func TestNewWorkerRuntimeUnknownProvider(t *testing.T) {
	if _, err := NewWorkerRuntime("gemini", &recordingSink{}, nil); err == nil {
		t.Fatal("未知供应商应报错")
	}
	if _, err := GetAdapter("gemini"); err == nil {
		t.Fatal("GetAdapter 未知供应商应报错")
	}
	if !IsProvider("claude") || !IsProvider("codex") || IsProvider("gemini") {
		t.Fatal("IsProvider 判断不对")
	}
}

func TestStubWorkerRuntimeTurns(t *testing.T) {
	sink := &recordingSink{}
	turns := [][]string{
		{
			`{"type":"system","subtype":"init","session_id":"stub-sess-9"}`,
			`{"type":"assistant","message":{"role":"assistant","content":[{"type":"text","text":"第一回合完成。"}]}}`,
			`{"type":"result","subtype":"success","is_error":false,"duration_ms":5,"num_turns":1,"result":"第一回合完成。"}`,
		},
		{
			`{"type":"assistant","message":{"role":"assistant","content":[{"type":"text","text":"第二回合完成。"}]}}`,
			`{"type":"result","subtype":"success","is_error":false,"duration_ms":5,"num_turns":1,"result":"第二回合完成。"}`,
		},
	}
	stub, err := NewStubWorkerRuntime(ProviderClaude, sink, turns)
	if err != nil {
		t.Fatalf("建 stub 失败：%v", err)
	}
	ctx := context.Background()
	if _, err := stub.Start(ctx, StartOptions{Provider: ProviderClaude, Worker: "implement"}); err != nil {
		t.Fatalf("stub Start 失败：%v", err)
	}
	if len(sink.kinds()) != 0 {
		t.Fatalf("Start 后应保持空闲、不输出，实际 %v", sink.kinds())
	}
	if err := stub.Send(ctx, "做 A"); err != nil {
		t.Fatalf("Send 失败：%v", err)
	}
	sink.waitKind(t, store.KindDone, 5*time.Second)
	if sink.sessionID != "stub-sess-9" {
		t.Fatalf("应持久化 session_id，实际 %q", sink.sessionID)
	}
	kinds := sink.kinds()
	if len(kinds) != 2 || kinds[0] != store.KindMessage || kinds[1] != store.KindDone {
		t.Fatalf("第一回合事件应为 message → done，实际 %v", kinds)
	}
	if got := stub.Sends(); len(got) != 1 || got[0] != "做 A" {
		t.Fatalf("应记录 Send 文本，实际 %v", got)
	}
	// 第二回合：播放第二组脚本。
	if err := stub.Send(ctx, "做 B"); err != nil {
		t.Fatalf("第二 Send 失败：%v", err)
	}
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) && len(sink.kinds()) < 4 {
		time.Sleep(10 * time.Millisecond)
	}
	if len(sink.kinds()) != 4 {
		t.Fatalf("两回合应共 4 个事件，实际 %v", sink.kinds())
	}
	// Interrupt 记录调用；Stop 后 Wait 返回，退出码为配置值。
	if err := stub.Interrupt(ctx, "停下，改做 C"); err != nil {
		t.Fatalf("Interrupt 失败：%v", err)
	}
	if got := stub.Interrupts(); len(got) != 1 || got[0] != "停下，改做 C" {
		t.Fatalf("应记录 Interrupt 文本，实际 %v", got)
	}
	stub.SetExitCode(0)
	if err := stub.Stop(ctx); err != nil {
		t.Fatalf("Stop 失败：%v", err)
	}
	if err := stub.Wait(); err != nil {
		t.Fatalf("Wait 失败：%v", err)
	}
	code, _, exited := stub.ExitState()
	if !exited || code != 0 {
		t.Fatalf("退出状态不对：code=%d exited=%v", code, exited)
	}
	if !stub.Stopped() {
		t.Fatal("应标记已停止")
	}
}

func TestStubWorkerRuntimeCodexTurn(t *testing.T) {
	sink := &recordingSink{}
	// codex 回合：先预置 threadId 的响应行，再发最终答案与 turn/completed。
	turns := [][]string{{
		`{"id":2,"result":{"thread":{"id":"` + codexThreadID + `","sessionId":"` + codexThreadID + `"}}}`,
		`{"method":"item/completed","params":{"item":{"type":"agentMessage","id":"m1","text":"查完了。","phase":"final_answer"}}}`,
		`{"method":"turn/completed","params":{"turn":{"id":"t1","status":"completed"}}}`,
	}}
	stub, err := NewStubWorkerRuntime(ProviderCodex, sink, turns)
	if err != nil {
		t.Fatalf("建 stub 失败：%v", err)
	}
	ctx := context.Background()
	if _, err := stub.Start(ctx, StartOptions{Provider: ProviderCodex, Worker: "check"}); err != nil {
		t.Fatalf("stub Start 失败：%v", err)
	}
	// stub 不预置待处理 RPC id：脚本里的 thread/start 响应行
	// 因无 "thread/start" 标签而不持久化 threadId，
	// 但最终答案与 turn/completed 的解析不受影响。
	if err := stub.Send(ctx, "检查"); err != nil {
		t.Fatalf("Send 失败：%v", err)
	}
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) && len(sink.kinds()) < 2 {
		time.Sleep(10 * time.Millisecond)
	}
	kinds := sink.kinds()
	if len(kinds) != 2 || kinds[0] != store.KindMessage || kinds[1] != store.KindDone {
		t.Fatalf("codex 回合应产生 message → done，实际 %v", kinds)
	}
	if err := stub.Stop(ctx); err != nil {
		t.Fatalf("Stop 失败：%v", err)
	}
}
