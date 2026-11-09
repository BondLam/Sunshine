//go:build windows

package supervisor

import (
	"os"
	"os/exec"
	"syscall"
)

// pidAlive 判断进程是否存活：Windows 上用 OpenProcess 试开句柄，
// 能打开即存活；打不开即视为已死。
func pidAlive(pid int) bool {
	if pid <= 0 {
		return false
	}
	h, err := syscall.OpenProcess(syscall.PROCESS_QUERY_INFORMATION, false, uint32(pid))
	if err != nil {
		return false
	}
	syscall.CloseHandle(h)
	return true
}

// signalTerm 向进程发“优雅停止”请求：Windows 没有 SIGTERM，
// 一律用 TerminateProcess 强制结束。
func signalTerm(pid int) error {
	p, err := os.FindProcess(pid)
	if err != nil {
		return err
	}
	return p.Kill()
}

// setDetached 让子进程进入新进程组（CREATE_NEW_PROCESS_GROUP）：
// 与 Unix 的 Setsid 对应——父进程退出或终端 Ctrl+C 不连带影响
// supervisor（新组只收发给本组的控制台事件）。
func setDetached(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{CreationFlags: syscall.CREATE_NEW_PROCESS_GROUP}
}
