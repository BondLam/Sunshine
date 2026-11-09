//go:build !windows

package task

import (
	"context"
	"os/exec"
	"syscall"
)

// newHookCmd 构造一条 hook 命令：Unix 上跑 sh -c，并用 Setsid 让 hook
// 脱离父进程进程组（父进程退出或超时杀组时不连带杀 hook 的子进程树）。
func newHookCmd(ctx context.Context, cmdStr string) *exec.Cmd {
	cmd := exec.CommandContext(ctx, "sh", "-c", cmdStr)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	return cmd
}
