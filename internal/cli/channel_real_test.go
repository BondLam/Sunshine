// Package cli 的 channel 真实后端集成测试（阶段 6 C 组）。
//
// 直接用默认真实后端（B 组 API + A 组 store）验证 CLI 全套，
// 存储根为temp SUNSHINE_CHANNEL_ROOT；spawn 用模拟 supervisor
// 脚本（sleep，不拉起真实 claude/codex）验证派生与 kill。
package cli

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/linrong007/Sunshine/internal/channel"
)

// setupRealChannelTest 搭真实后端测试环境：temp存储根与工作目录。
// 不覆盖 newChannelBackend（默认就是真实后端）；
// channelWorkerRuntime 置 nil（生产默认：spawn 走 supervisor），
// channelSupervisorBinary 指向模拟 supervisor 脚本。
func setupRealChannelTest(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	proj := t.TempDir()
	t.Setenv("SUNSHINE_CHANNEL_ROOT", root)
	t.Chdir(proj)
	oldRuntime := channelWorkerRuntime
	channelWorkerRuntime = nil
	t.Cleanup(func() { channelWorkerRuntime = oldRuntime })
	script := filepath.Join(t.TempDir(), "supervisor-stub.sh")
	if err := os.WriteFile(script, []byte("#!/bin/sh\nsleep 30\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	oldBinary := channelSupervisorBinary
	channelSupervisorBinary = script
	t.Cleanup(func() { channelSupervisorBinary = oldBinary })
	return root
}

func TestChannelRealEndToEnd(t *testing.T) {
	setupRealChannelTest(t)
	out, err := runChannelCmd(t, "channel", "create", "review", "--type", "chat", "--task", "11-09-xxx")
	if err != nil {
		t.Fatalf("真实后端 create 失败：%v", err)
	}
	if !strings.Contains(out, "已创建 channel 'review'") {
		t.Fatalf("create 输出不对：%q", out)
	}
	out, err = runChannelCmd(t, "channel", "send", "review", "--by", "main", "开始实现任务")
	if err != nil {
		t.Fatalf("真实后端 send 失败：%v", err)
	}
	var ev ChannelEvent
	if err := json.Unmarshal([]byte(strings.TrimSpace(out)), &ev); err != nil {
		t.Fatalf("send 输出不是 JSON：%q：%v", out, err)
	}
	if ev.Kind() != "message" || ev.Text() != "开始实现任务" {
		t.Fatalf("send 事件不对：%v", ev)
	}
	out, err = runChannelCmd(t, "channel", "list")
	if err != nil {
		t.Fatalf("真实后端 list 失败：%v", err)
	}
	if !strings.Contains(out, "review") {
		t.Fatalf("list 未列出 channel：%q", out)
	}
	out, err = runChannelCmd(t, "channel", "messages", "review")
	if err != nil {
		t.Fatalf("真实后端 messages 失败：%v", err)
	}
	if !strings.Contains(out, "开始实现任务") {
		t.Fatalf("messages 输出不对：%q", out)
	}
	// wait：500ms 后由另一 goroutine 经 B 组 API 发消息。
	go func() {
		time.Sleep(500 * time.Millisecond)
		_, _ = channel.SendMessage(channel.SendMessageOptions{
			ChannelAddress: channel.ChannelAddress{Channel: "review"},
			By:             "impl", Origin: channel.OriginCLI, Text: "做完了",
		})
	}()
	out, err = runChannelCmd(t, "channel", "wait", "review", "--from", "impl", "--timeout", "10s")
	if err != nil {
		t.Fatalf("真实后端 wait 失败：%v", err)
	}
	if !strings.Contains(out, "做完了") {
		t.Fatalf("wait 输出应含匹配事件：%q", out)
	}
	// 超时。
	if _, err := runChannelCmd(t, "channel", "wait", "review", "--from", "没人", "--timeout", "300ms"); err == nil {
		t.Fatalf("真实后端 wait 超时应报错")
	}
	// prune 与 rm。
	if _, err := runChannelCmd(t, "channel", "create", "empty-ch"); err != nil {
		t.Fatalf("create 失败：%v", err)
	}
	out, err = runChannelCmd(t, "channel", "prune", "--empty", "--yes")
	if err != nil {
		t.Fatalf("真实后端 prune 失败：%v", err)
	}
	if !strings.Contains(out, "已删除 1 个 channel") {
		t.Fatalf("prune 输出不对：%q", out)
	}
	out, err = runChannelCmd(t, "channel", "rm", "review")
	if err != nil {
		t.Fatalf("真实后端 rm 失败：%v", err)
	}
	if !strings.Contains(out, "已删除 channel 'review'") {
		t.Fatalf("rm 输出不对：%q", out)
	}
}

func TestChannelRealContextTitle(t *testing.T) {
	setupRealChannelTest(t)
	if _, err := runChannelCmd(t, "channel", "create", "review"); err != nil {
		t.Fatalf("create 失败：%v", err)
	}
	tmp := filepath.Join(t.TempDir(), "需求.md")
	if err := os.WriteFile(tmp, []byte("需求"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := runChannelCmd(t, "channel", "context", "add", "review", "--file", tmp, "--raw", "只改退款逻辑"); err != nil {
		t.Fatalf("真实后端 context add 失败：%v", err)
	}
	out, err := runChannelCmd(t, "channel", "context", "list", "review")
	if err != nil {
		t.Fatalf("context list 失败：%v", err)
	}
	if !strings.Contains(out, "file "+tmp) || !strings.Contains(out, "只改退款逻辑") {
		t.Fatalf("context list 输出不对：%q", out)
	}
	if _, err := runChannelCmd(t, "channel", "context", "delete", "review", "--raw", "只改退款逻辑"); err != nil {
		t.Fatalf("context delete 失败：%v", err)
	}
	out, _ = runChannelCmd(t, "channel", "context", "list", "review")
	if strings.Contains(out, "只改退款逻辑") {
		t.Fatalf("delete 后条目应已删除：%q", out)
	}
	if _, err := runChannelCmd(t, "channel", "title", "set", "review", "--title", "退款原因重构"); err != nil {
		t.Fatalf("title set 失败：%v", err)
	}
	out, err = runChannelCmd(t, "channel", "list", "--json")
	if err != nil {
		t.Fatalf("list --json 失败：%v", err)
	}
	var rows []ChannelSummary
	if err := json.Unmarshal([]byte(out), &rows); err != nil {
		t.Fatalf("list --json 解析失败：%v", err)
	}
	if len(rows) != 1 || rows[0].Description != "退款原因重构" {
		t.Fatalf("title 应反映到 list 描述里：%v", rows)
	}
	if _, err := runChannelCmd(t, "channel", "title", "clear", "review"); err != nil {
		t.Fatalf("title clear 失败：%v", err)
	}
}

func TestChannelRealSpawnKill(t *testing.T) {
	setupRealChannelTest(t)
	if _, err := runChannelCmd(t, "channel", "create", "review"); err != nil {
		t.Fatalf("create 失败：%v", err)
	}
	out, err := runChannelCmd(t, "channel", "spawn", "review", "--provider", "claude", "--as", "worker1")
	if err != nil {
		t.Fatalf("真实后端 spawn 失败：%v", err)
	}
	var res SpawnResult
	if err := json.Unmarshal([]byte(strings.TrimSpace(out)), &res); err != nil {
		t.Fatalf("spawn 输出不是 JSON：%q：%v", out, err)
	}
	if res.Worker != "worker1" || res.PID <= 0 {
		t.Fatalf("spawn 结果不对：%+v", res)
	}
	if _, err := os.Stat(res.Log); err != nil && !os.IsNotExist(err) {
		t.Fatalf("读日志路径失败：%v", err)
	}
	// pid 文件应已写盘（supervisor-stub.sh 的 sleep 进程）。
	pidFile := filepath.Join(filepath.Dir(res.Log), "worker1.pid")
	if _, err := os.Stat(pidFile); err != nil {
		t.Fatalf("spawn 后应有 pid 文件：%v", err)
	}
	out, err = runChannelCmd(t, "channel", "kill", "review", "--as", "worker1")
	if err != nil {
		t.Fatalf("真实后端 kill 失败：%v", err)
	}
	if !strings.Contains(out, "已停止 worker 'worker1'") {
		t.Fatalf("kill 输出不对：%q", out)
	}
	out, err = runChannelCmd(t, "channel", "messages", "review", "--kind", "killed", "--raw")
	if err != nil {
		t.Fatalf("messages 失败：%v", err)
	}
	if !strings.Contains(out, `"kind":"killed"`) {
		t.Fatalf("kill 后应有 killed 事件：%q", out)
	}
}

func TestChannelRealInternalCommands(t *testing.T) {
	setupRealChannelTest(t)
	// __parse-trace 已接 D 组实现：文件不存在时报读文件错。
	if _, err := runChannelCmd(t, "channel", "__parse-trace", "claude", "/不存在/trace.jsonl"); err == nil {
		t.Fatalf("__parse-trace 读不到文件应报错")
	} else if !strings.Contains(err.Error(), "trace") {
		t.Fatalf("__parse-trace 报错不对：%v", err)
	}
	// __supervisor 已接 D 组实现：配置不存在时报读配置错。
	if _, err := runChannelCmd(t, "channel", "__supervisor", "a", "b", "/不存在/配置.json"); err == nil {
		t.Fatalf("__supervisor 读不到配置应报错")
	} else if !strings.Contains(err.Error(), "supervisor 配置") {
		t.Fatalf("__supervisor 报错不对：%v", err)
	}
	_ = context.Background
}
