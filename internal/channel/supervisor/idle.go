package supervisor

import (
	"fmt"
	"io"
	"time"
)

// IdleTimerHandle 是 supervisor 侧空闲超时定时器的句柄。
type IdleTimerHandle struct {
	// Reset 因 worker 刚结束一回合而重启定时器。
	Reset func()
	// Pause 因 worker 进入回合而暂停定时器（回合中永不杀）。
	Pause func()
	// Cancel 彻底取消（supervisor 退出时）。
	Cancel func()
}

// ScheduleIdleTimer 安排一个自重置的空闲定时器。
// idleTimeoutMs <= 0 时短路为无操作句柄（空闲清理已禁用）。
//
// 行为（与上游 supervisor/idle.ts 一致）：
//   - spawned 后立即启动；
//   - turn_finished / interrupted 后重置（worker 回到空闲）；
//   - turn_started 后暂停（worker 在回合中）；
//   - 到时调用 shutdown.Request("SIGTERM", "idle-timeout")。
func ScheduleIdleTimer(idleTimeoutMs int64, shutdown *ShutdownController, isChildExited func() bool, log io.Writer) IdleTimerHandle {
	noop := IdleTimerHandle{
		Reset:  func() {},
		Pause:  func() {},
		Cancel: func() {},
	}
	if idleTimeoutMs <= 0 {
		return noop
	}
	var timer *time.Timer
	cancelled := false
	stop := func() {
		if timer != nil {
			timer.Stop()
			timer = nil
		}
	}
	var start func()
	start = func() {
		if cancelled {
			return
		}
		stop()
		d := time.Duration(idleTimeoutMs) * time.Millisecond
		timer = time.AfterFunc(d, func() {
			if cancelled {
				return
			}
			if shutdown.IsShuttingDown() || isChildExited() {
				return
			}
			if log != nil {
				_, _ = fmt.Fprintf(log, "[supervisor] 空闲超时 %dms 已到，请求关停\n", idleTimeoutMs)
			}
			_ = shutdown.Request(ReasonIdleTimeout)
		})
	}
	start()
	return IdleTimerHandle{
		Reset: start,
		Pause: stop,
		Cancel: func() {
			cancelled = true
			stop()
		},
	}
}
