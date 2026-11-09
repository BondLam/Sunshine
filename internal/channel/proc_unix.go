//go:build !windows

package channel

import "syscall"

// pidAlive 用 kill(pid, 0) 判断进程是否存活（本包原语义：EPERM 算已死）。
func pidAlive(pid int) bool {
	return syscall.Kill(pid, 0) == nil
}

// signalProcess 向进程发信号。
func signalProcess(pid int, sig syscall.Signal) error {
	return syscall.Kill(pid, sig)
}
