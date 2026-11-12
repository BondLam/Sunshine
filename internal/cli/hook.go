package cli

import (
	"fmt"
	"os"

	"github.com/linrong1982/Sunshine/internal/hook"
	"github.com/spf13/cobra"
)

// hookCmd 是 `sunshine hook <事件>`：各平台 hook 配置里调用的入口，
// 替代 Trellis 的全部 Python hook 脚本。
//
// 注意：这里故意只注册一个 hook 命令、由第一个参数分发事件，
// 而不是给每个事件各注册一个 cobra 子命令。hook 有一条铁律——
// 任何情况下都不得以非零码退出（未知事件、无参数、stdin 不是
// JSON、缺少字段等都必须退出 0，否则会阻断 agent 会话）；
// cobra 对未知子命令会直接返回错误并让 Execute 以退出码 1 结束，
// 单命令分发才能把“未知事件”也收敛到 stderr 中文提示 + 返回 nil。
var hookCmd = &cobra.Command{
	Use:   "hook <事件>",
	Short: "供各平台 hook 调用的子命令（替代 Python hooks）",
	Long: `供各平台 hook 配置调用的子命令，替代 Python hook 脚本。

事件：
  session-start      会话开始：注入结构化上下文（workflow 概览、开发者身份、
                     git 状态、当前任务、项目规范）
  workflow-state     每轮提示词面包屑：只从 workflow.md 的
                     [workflow-state:STATUS] 标签块取内容
  subagent-context   子代理上下文注入：给 implement/check/research 子代理
                     注入任务上下文（prototype 只记启动事件，不注入）
  subagent-stop      子代理结束：记一条 subagent_stopped 事件，供 serve
                     推导智能体团运行状态（SubagentStop）
  shell-session      shell 执行前：待执行命令调用 sunshine task 命令时，
                     写一张 30 秒票据把会话身份桥接给子进程
  statusline         Claude Code 状态栏：输出 1-2 行状态

各事件都从 stdin 读 hook JSON。无 .sunshine/ 时静默退出 0；任何失败
都静默退出 0，绝不阻断 agent 会话。`,
	Example: `  echo '{"session_id":"abc","cwd":"."}' | sunshine hook session-start
  echo '{"prompt":"继续","cwd":"."}' | sunshine hook workflow-state`,
	Args: cobra.ArbitraryArgs,
	Run: func(cmd *cobra.Command, args []string) {
		if len(args) == 0 {
			fmt.Fprintln(os.Stderr, "用法：sunshine hook <事件>，事件为 session-start、workflow-state、subagent-context、subagent-stop、shell-session、statusline 之一")
			return
		}
		opts := hook.Options{Version: version, Stdout: cmd.OutOrStdout(), Stderr: cmd.ErrOrStderr()}
		// 所有 Run* 函数失败时都返回 nil；这里同样不返回错误，
		// 保证 hook 永远以退出码 0 结束。
		switch args[0] {
		case "session-start":
			_ = hook.RunSessionStart(os.Stdin, opts)
		case "workflow-state":
			_ = hook.RunWorkflowState(os.Stdin, opts)
		case "subagent-context":
			_ = hook.RunSubagentContext(os.Stdin, opts)
		case "subagent-stop":
			_ = hook.RunSubagentStop(os.Stdin, opts)
		case "shell-session":
			_ = hook.RunShellSession(os.Stdin, opts)
		case "statusline":
			_ = hook.RunStatusline(os.Stdin, opts)
		default:
			fmt.Fprintf(os.Stderr, "未知 hook 事件 %q，可用事件为 session-start、workflow-state、subagent-context、subagent-stop、shell-session、statusline\n", args[0])
		}
	},
}

func init() {
	rootCmd.AddCommand(hookCmd)
}
