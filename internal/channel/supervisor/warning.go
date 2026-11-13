package supervisor

import (
	"fmt"
	"io"
	"time"

	"github.com/linrong007/Sunshine/internal/channel/store"
)

// DefaultWarnBeforeMs 是默认的超时前预警提前量：5 分钟（与上游一致）。
const DefaultWarnBeforeMs = int64(5 * 60 * 1000)

// ScheduleTimeoutWarning 安排一次性的超时前预警：
// 在 timeoutMs - warnBeforeMs 时追加一条 supervisor_warning 事件，
// 让观察者（派发方 / channel messages）看到即将到来的杀掉，
// 而不用轮询。事件只是观测用的，永不替代最终的 killed/done/error
// 终态事件。
//
// warnBeforeMs <= 0 时禁用；timeoutMs <= 0 时不安排。
// 预警最多发一次；关停已请求、已有终态事件或子进程已退出时不发。
// 追加失败只记日志，不影响 worker 生命周期。
// 返回取消函数（supervisor 退出时调用）。
func ScheduleTimeoutWarning(ref store.ChannelRef, worker string, timeoutMs, warnBeforeMs int64, shutdown *ShutdownController, isChildExited func() bool, log io.Writer) func() {
	noop := func() {}
	if timeoutMs <= 0 {
		return noop
	}
	if warnBeforeMs <= 0 {
		return noop
	}
	remaining := warnBeforeMs
	if remaining > timeoutMs {
		remaining = timeoutMs
	}
	delay := timeoutMs - remaining
	if delay < 0 {
		delay = 0
	}
	cancelled := false
	emitted := false
	timer := time.AfterFunc(time.Duration(delay)*time.Millisecond, func() {
		if cancelled || emitted {
			return
		}
		if shutdown.IsShuttingDown() || shutdown.HasTerminalEvent() || isChildExited() {
			return
		}
		emitted = true
		_, err := appendEvent(ref, store.KindSupervisorWarning, "supervisor:"+worker, map[string]any{
			"worker":       worker,
			"reason":       "approaching_timeout",
			"timeout_ms":   timeoutMs,
			"remaining_ms": remaining,
		})
		if err != nil && log != nil {
			_, _ = fmt.Fprintf(log, "[supervisor] 预警追加失败：%v\n", err)
		}
	})
	return func() {
		cancelled = true
		timer.Stop()
	}
}
