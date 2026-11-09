//go:build !windows

package supervisor

import (
	"os/exec"
	"syscall"
)

// pidAlive 判断 pid 是否存活：kill(pid, 0) 成功或报 EPERM 都算存活，
// 仅 ESRCH（无此进程）算已死。
func pidAlive(pid int) bool {
	if pid <= 0 {
		return false
	}
	err := syscall.Kill(pid, 0)
	return err == nil || err == syscall.EPERM
}

// signalTerm 向进程发 SIGTERM（优雅停止）。
func signalTerm(pid int) error {
	return syscall.Kill(pid, syscall.SIGTERM)
}

// setDetached 让子进程脱离父进程进程组（Setsid）：父进程退出或
// 终端信号不连带影响 supervisor。
func setDetached(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
}
