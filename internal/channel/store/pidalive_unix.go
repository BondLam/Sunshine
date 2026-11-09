//go:build !windows

package store

import (
	"errors"
	"syscall"
)

// pidAlive 判断 pid 是否存活：kill(pid, 0) 成功或报 EPERM 都算存活，
// 仅 ESRCH（无此进程）算已死。
func pidAlive(pid int) bool {
	err := syscall.Kill(pid, 0)
	if err == nil {
		return true
	}
	return errors.Is(err, syscall.EPERM)
}
