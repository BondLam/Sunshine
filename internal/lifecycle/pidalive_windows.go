//go:build windows

package lifecycle

import "syscall"

// pidAlive 判断 pid 是否存活：Windows 上用 OpenProcess 试开句柄，
// 能打开即存活；打不开即视为已死。仅用于项目锁窃取时的陈旧判断，
// 与 Unix 版 kill(pid, 0) 的语义对应。
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
