//go:build !windows

package adapters

import (
	"os"
	"syscall"
)

// signalTerm 向 worker 进程发 SIGTERM（优雅停机第一步）。
func signalTerm(p *os.Process) error {
	return p.Signal(syscall.SIGTERM)
}

// signalKill 向 worker 进程发 SIGKILL（强制结束）。
func signalKill(p *os.Process) error {
	return p.Signal(syscall.SIGKILL)
}
