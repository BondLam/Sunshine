//go:build windows

package cli

import (
	"os"
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

// signalProcess 向进程发信号：Windows 没有 SIGTERM/SIGKILL 的语义区分，
// 一律用 TerminateProcess 强制结束（sig 参数仅为与 Unix 版统一签名）。
func signalProcess(pid int, sig syscall.Signal) error {
	p, err := os.FindProcess(pid)
	if err != nil {
		return err
	}
	return p.Kill()
}
