package hook

import (
	"bytes"
	"context"
	"os/exec"
	"strings"
	"time"
)

// runCommandTimeout 在 dir 下运行命令，timeoutMs 毫秒超时，成功返回
// 去空白后的 stdout，失败/超时/非零退出返回 ""。
func runCommandTimeout(dir string, timeoutMs int, name string, args ...string) string {
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(timeoutMs)*time.Millisecond)
	defer cancel()
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Dir = dir
	var out bytes.Buffer
	cmd.Stdout = &out
	_ = cmd.Run()
	if ctx.Err() != nil {
		return ""
	}
	return strings.TrimSpace(out.String())
}

// runCommandOutput 与 runCommandTimeout 相同，但返回原始 stdout
// （不 TrimSpace）与是否成功，供需要区分“空输出”与“失败”的调用方使用。
func runCommandOutput(dir string, timeoutMs int, name string, args ...string) (string, bool) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(timeoutMs)*time.Millisecond)
	defer cancel()
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Dir = dir
	var out bytes.Buffer
	cmd.Stdout = &out
	err := cmd.Run()
	if ctx.Err() != nil || err != nil {
		return "", false
	}
	return out.String(), true
}
