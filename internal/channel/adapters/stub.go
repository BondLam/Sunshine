// StubWorkerRuntime 是测试用的 worker runtime：
// 实现完整的 WorkerRuntime 接口（Start / Send / Interrupt / Stop /
// Wait / Ready / ExitState），用脚本化的输出序列模拟 claude / codex
// 的 JSONL 输出流，供 B/C 组与总协调人的闭环测试复用，
// 不拉起任何真实 CLI 子进程。
//
// 行为：
//   - Start 后保持空闲（像真实 worker 等第一条 send --to 才工作），不自动输出。
//   - 每次 Send 播放下一回合的脚本行（回合按传入顺序消费，
//     耗尽后重复最后一回合），行先过真实适配器解析再经 sink 发出，
//     因此能验证适配器解析与 supervisor 收尾的全链路。
//   - Interrupt 记录调用并取消正在播放的回合，随后播放下一回合脚本，
//     模拟 worker 按替换指令工作、最终发出 done（真实 worker 收到中断
//     帧后也会继续工作直至回合结束）。
//   - Stop 取消播放并关闭 done；Wait 等 done 关闭。
package adapters

import (
	"context"
	"fmt"
	"sync"
	"time"
)

// StubWorkerRuntime 是脚本化的测试 worker。
type StubWorkerRuntime struct {
	provider Provider
	adapter  Adapter
	sink     EventSink

	mu         sync.Mutex
	turns      [][]string
	turnIndex  int
	stepDelay  time.Duration
	started    bool
	stopped    bool
	sends      []string
	interrupts []string
	exitCode   int
	done       chan struct{}
	doneClosed bool
	playCancel context.CancelFunc
	actx       any
	worker     string
}

// NewStubWorkerRuntime 建一个 stub runtime。
// turns 是每回合的脚本行（每行一条 JSONL）；provider 决定用哪个
// 真实适配器解析这些行；sink 接收解析出的事件。
// 例：NewStubWorkerRuntime(ProviderClaude, sink, [][]string{{init 行， 文本行， result 行}})。
func NewStubWorkerRuntime(provider Provider, sink EventSink, turns [][]string) (*StubWorkerRuntime, error) {
	a, err := GetAdapter(provider)
	if err != nil {
		return nil, err
	}
	return &StubWorkerRuntime{
		provider:  provider,
		adapter:   a,
		sink:      sink,
		turns:     turns,
		stepDelay: 5 * time.Millisecond,
		exitCode:  0,
		done:      make(chan struct{}),
	}, nil
}

// SetStepDelay 设脚本行之间的播放间隔（默认 5ms；测试可改为 0 加速）。
func (s *StubWorkerRuntime) SetStepDelay(d time.Duration) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.stepDelay = d
}

// SetExitCode 设 Stop 后 ExitState 报告的退出码（默认 0）。
func (s *StubWorkerRuntime) SetExitCode(code int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.exitCode = code
}

// Sends 返回已收到的 Send 文本（按序）。
func (s *StubWorkerRuntime) Sends() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]string, len(s.sends))
	copy(out, s.sends)
	return out
}

// Interrupts 返回已收到的 Interrupt 文本（按序）。
func (s *StubWorkerRuntime) Interrupts() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]string, len(s.interrupts))
	copy(out, s.interrupts)
	return out
}

// Started 判断是否已 Start。
func (s *StubWorkerRuntime) Started() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.started
}

// Stopped 判断是否已 Stop。
func (s *StubWorkerRuntime) Stopped() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.stopped
}

// Start 标记启动并初始化适配器上下文；不播放任何脚本行。
func (s *StubWorkerRuntime) Start(ctx context.Context, opts StartOptions) (StartResult, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.started {
		return StartResult{}, fmt.Errorf("stub worker 已启动")
	}
	s.started = true
	s.worker = opts.Worker
	s.actx = s.adapter.CreateCtx()
	return StartResult{Provider: s.provider, PID: 0, StartedAt: time.Now()}, nil
}

// Send 记录文本，并播放下一回合的脚本行。
func (s *StubWorkerRuntime) Send(ctx context.Context, text string) error {
	s.mu.Lock()
	if !s.started {
		s.mu.Unlock()
		return fmt.Errorf("stub worker 尚未启动")
	}
	if s.stopped {
		s.mu.Unlock()
		return fmt.Errorf("stub worker 已停止")
	}
	s.sends = append(s.sends, text)
	lines, delay := s.nextTurnLocked()
	s.mu.Unlock()
	return s.playTurn(ctx, lines, delay)
}

// Interrupt 记录中断文本，取消正在播放的回合，
// 再播放下一回合脚本（模拟 worker 按替换指令工作）。
func (s *StubWorkerRuntime) Interrupt(ctx context.Context, text string) error {
	s.mu.Lock()
	if !s.started {
		s.mu.Unlock()
		return fmt.Errorf("stub worker 尚未启动")
	}
	s.interrupts = append(s.interrupts, text)
	if s.playCancel != nil {
		s.playCancel()
	}
	lines, delay := s.nextTurnLocked()
	s.mu.Unlock()
	return s.playTurn(ctx, lines, delay)
}

// nextTurnLocked 取下一回合脚本（调用方须持有 s.mu；回合耗尽后重复最后一回合）。
func (s *StubWorkerRuntime) nextTurnLocked() ([]string, time.Duration) {
	idx := s.turnIndex
	if idx >= len(s.turns) {
		idx = len(s.turns) - 1
	}
	if idx < 0 {
		return nil, s.stepDelay
	}
	lines := s.turns[idx]
	s.turnIndex++
	return lines, s.stepDelay
}

// playTurn 按行播放一回合脚本：每行先过真实适配器解析再经 sink 发出。
func (s *StubWorkerRuntime) playTurn(ctx context.Context, lines []string, delay time.Duration) error {
	if len(lines) == 0 {
		return nil
	}
	playCtx, cancel := context.WithCancel(context.Background())
	s.mu.Lock()
	s.playCancel = cancel
	s.mu.Unlock()
	defer func() {
		cancel()
		s.mu.Lock()
		s.playCancel = nil
		s.mu.Unlock()
	}()
	for _, line := range lines {
		select {
		case <-playCtx.Done():
			return nil
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(delay):
		}
		s.mu.Lock()
		actx := s.actx
		s.mu.Unlock()
		result := s.adapter.ParseLine(line, actx)
		s.applyParseResult(result)
	}
	return nil
}

// Stop 取消播放并关闭 done。
func (s *StubWorkerRuntime) Stop(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.stopped {
		return nil
	}
	s.stopped = true
	if s.playCancel != nil {
		s.playCancel()
	}
	if !s.doneClosed {
		s.doneClosed = true
		close(s.done)
	}
	return nil
}

// Wait 等 Stop 关闭 done。
func (s *StubWorkerRuntime) Wait() error {
	<-s.done
	return nil
}

// Ready 恒为真（stub 无需握手）。
func (s *StubWorkerRuntime) Ready() bool { return true }

// ExitState 返回配置的退出码（Stop 后 exited 为真）。
func (s *StubWorkerRuntime) ExitState() (int, string, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.stopped {
		return -1, "", false
	}
	return s.exitCode, "", true
}

func (s *StubWorkerRuntime) applyParseResult(result ParseResult) {
	for _, ev := range result.Events {
		if s.sink == nil {
			continue
		}
		_ = s.sink.Emit(ev.Kind, s.worker, ev.Payload)
	}
	if result.Side == nil || s.sink == nil {
		return
	}
	if result.Side.PersistSessionID != "" {
		_ = s.sink.PersistSessionID(result.Side.PersistSessionID)
	}
	if result.Side.PersistThreadID != "" {
		_ = s.sink.PersistThreadID(result.Side.PersistThreadID)
	}
	// stub 没有真实 stdin：reply 直接丢弃（codex 询问自动应答不影响测试）。
}
