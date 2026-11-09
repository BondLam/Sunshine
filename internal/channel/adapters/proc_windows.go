//go:build windows

package adapters

import "os"

// signalTerm 向 worker 进程发“优雅停止”请求：Windows 没有 SIGTERM，
// 用 os.Interrupt（向进程组发 Ctrl+C 事件）近似；失败时调用方仍会
// 在宽限后走 signalKill。
func signalTerm(p *os.Process) error {
	return p.Signal(os.Interrupt)
}

// signalKill 强制结束 worker 进程（TerminateProcess）。
func signalKill(p *os.Process) error {
	return p.Kill()
}
