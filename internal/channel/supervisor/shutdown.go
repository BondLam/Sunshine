package supervisor

import (
	"fmt"
	"io"
	"sync"

	"github.com/linrong007/Sunshine/internal/channel/store"
)

// ShutdownReason 是 worker 离开的原因（与上游 shutdown.ts 一致）。
type ShutdownReason string

const (
	// ReasonExplicitKill 显式杀掉（kill 命令或外部信号）。
	ReasonExplicitKill ShutdownReason = "explicit-kill"
	// ReasonTimeout 达到 --timeout  lifetime 上限。
	ReasonTimeout ShutdownReason = "timeout"
	// ReasonCrash worker 崩溃（拉起后进程出错）。
	ReasonCrash ShutdownReason = "crash"
	// ReasonIdleTimeout 空闲超时（OOM guard 的 idle 清理）。
	ReasonIdleTimeout ShutdownReason = "idle-timeout"
)

// ShutdownController 是所有“worker 要离开”触发的唯一漏斗
// （显式杀掉、超时、拉起后崩溃、信号、子进程退出），负责：
//   - shutdownReason 标志（幂等，先到先得）
//   - 关停梯队（runtime.Stop：关 stdin → SIGTERM → SIGKILL）
//   - 尾随的 killed 事件追加
//   - terminalEmitted 标志（适配器是否已发 done/error）
//   - finalizeOnExit：worker 退出时若适配器没发终态事件，合成一个兜底
//     done/error（否则 `wait --kind done` 会永远挂起）
type ShutdownController struct {
	mu              sync.Mutex
	reason          ShutdownReason
	hasReason       bool
	terminalEmitted bool
	killedDone      chan struct{}
	killedErr       error

	ref           store.ChannelRef
	worker        string
	log           io.Writer
	stopFn        func() error
	timeoutMs     int64
	idleTimeoutMs int64
}

// NewShutdownController 建一个关停控制器。
// stopFn 是实际关停 worker 的函数（通常为 runtime.Stop）；
// timeoutMs / idleTimeoutMs 只用于记在 killed 事件上。
func NewShutdownController(ref store.ChannelRef, worker string, log io.Writer, stopFn func() error, timeoutMs, idleTimeoutMs int64) *ShutdownController {
	return &ShutdownController{
		ref: ref, worker: worker, log: log, stopFn: stopFn,
		timeoutMs: timeoutMs, idleTimeoutMs: idleTimeoutMs,
		killedDone: make(chan struct{}),
	}
}

// Claim 同步标记关停意图，不启动关停梯队、不追加 killed。
// 供其他代码在第一次 await 之前就让外界看到 isShuttingDown=true。
// 已标记时返回 false。
func (s *ShutdownController) Claim(reason ShutdownReason) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.hasReason {
		return false
	}
	s.reason = reason
	s.hasReason = true
	return true
}

// Request 请求关停：幂等，只有第一次生效。
// 先跑关停梯队，再追加 killed 事件；返回 killed 追加的结果。
// 并发调用者会等第一次的 killed 追加完成。
func (s *ShutdownController) Request(reason ShutdownReason) error {
	s.mu.Lock()
	if s.hasReason && s.killedDone != nil {
		done := s.killedDone
		s.mu.Unlock()
		<-done
		return s.killedErr
	}
	s.reason = reason
	s.hasReason = true
	done := s.killedDone
	stopFn := s.stopFn
	s.mu.Unlock()

	s.logf("[supervisor] 正在关停 worker（原因=%s）\n", reason)
	if stopFn != nil {
		_ = stopFn()
	}
	err := s.appendKilled(reason)
	s.mu.Lock()
	s.killedErr = err
	s.mu.Unlock()
	close(done)
	return err
}

// IsShuttingDown 判断是否已请求关停。
func (s *ShutdownController) IsShuttingDown() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.hasReason
}

// Reason 返回已标记的关停原因（未标记时返回 "" 与 false）。
func (s *ShutdownController) Reason() (ShutdownReason, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.reason, s.hasReason
}

// MarkTerminalEmitted 标记适配器已产生 done/error 事件，
// finalizeOnExit 不再合成兜底事件。须在追加终态事件前同步调用，
// 避免与子进程退出处理竞态出重复事件。
func (s *ShutdownController) MarkTerminalEmitted() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.terminalEmitted = true
}

// HasTerminalEvent 判断是否已有终态事件。
func (s *ShutdownController) HasTerminalEvent() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.terminalEmitted
}

// FinalizeOnExit 在子进程退出时调用：
// 若无终态事件且未请求过关停（冷退出），合成一个兜底 done/error；
// 再等进行中的 killed 追加落盘，避免 killed 事件跑输进程死亡。
func (s *ShutdownController) FinalizeOnExit(code int, signal string) error {
	s.logf("[supervisor] worker 退出 code=%d signal=%s\n", code, signal)
	// 抢占终态槽：先在锁内判定并标记，两个并发的 FinalizeOnExit
	// 不能都通过检查、合成重复的兜底事件。
	s.mu.Lock()
	shouldSynthesize := !s.terminalEmitted && !s.hasReason
	if shouldSynthesize {
		s.terminalEmitted = true
	}
	done := s.killedDone
	hasReason := s.hasReason
	s.mu.Unlock()

	if shouldSynthesize {
		// 只对冷退出（未请求关停）合成终态事件；已请求关停时
		// killed 事件本身就是终态信号，再合成 error 只是重复。
		// by 用 worker 名（不用 supervisor:<name>），
		// 让 `wait --from <worker> --kind done` 能正常唤醒。
		if code == 0 {
			_, _ = appendEvent(s.ref, store.KindDone, s.worker, map[string]any{
				"synthesized": true,
				"exit_code":   code,
			})
		} else {
			_, _ = appendEvent(s.ref, store.KindError, s.worker, map[string]any{
				"message":     fmt.Sprintf("worker 退出时未发终态事件（code=%d，signal=%s）", code, signal),
				"synthesized": true,
				"exit_code":   code,
				"exit_signal": signal,
			})
		}
	}
	// 等进行中的 killed 追加落盘，避免 killed 事件跑输进程死亡。
	// 只有请求过关停时才有进行中的追加；否则 killedDone 永不关闭，不能等。
	if hasReason && done != nil {
		<-done
	}
	return nil
}

// AwaitFinalize 等当前（或最近一次）Request 的 killed 追加完成；
// 从未请求过时直接返回。
func (s *ShutdownController) AwaitFinalize() error {
	s.mu.Lock()
	done := s.killedDone
	hasReason := s.hasReason
	s.mu.Unlock()
	if !hasReason {
		return nil
	}
	<-done
	return s.killedErr
}

func (s *ShutdownController) appendKilled(reason ShutdownReason) error {
	extra := map[string]any{
		"reason": string(reason),
		"signal": "SIGTERM",
	}
	if reason == ReasonTimeout && s.timeoutMs > 0 {
		extra["timeout_ms"] = s.timeoutMs
	}
	if reason == ReasonIdleTimeout && s.idleTimeoutMs > 0 {
		extra["idle_timeout_ms"] = s.idleTimeoutMs
	}
	_, err := appendSupervisorEvent(s.ref, store.KindKilled, s.worker, extra)
	return err
}

func (s *ShutdownController) logf(format string, args ...any) {
	if s.log == nil {
		return
	}
	_, _ = fmt.Fprintf(s.log, format, args...)
}
