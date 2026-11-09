//go:build windows

package task

import (
	"context"
	"os/exec"
)

// newHookCmd 构造一条 hook 命令：Windows 上没有 sh，用 cmd.exe /c 跑
// （与上游 Windows 路径经 cmd.exe 的做法一致）；Windows 的
// SysProcAttr 没有 Setsid 字段，不设置。
func newHookCmd(ctx context.Context, cmdStr string) *exec.Cmd {
	return exec.CommandContext(ctx, "cmd.exe", "/c", cmdStr)
}
