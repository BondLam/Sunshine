// Package cli 的 sunshine channel 命令：多 Agent 协同（阶段 6 C 组）。
//
// 本文件：channel 根命令与 create / send / wait / interrupt。
// spawn / run 见 channel_spawn.go；rm / prune / list / messages / kill /
// context / title / __supervisor / __parse-trace 见 channel_manage.go。
// forum 相关子命令（post / forum / thread / rename）由 E 组在
// channel_forum.go 里挂到包级变量 channelCmd 上。
package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

// channelCmd 是 `sunshine channel` 根命令（包级变量，供 E 组的 forum
// 子命令文件挂接：channelCmd.AddCommand(...)）。
var channelCmd *cobra.Command

// RegisterChannelCommand 把 `sunshine channel` 接到根命令上。
// 每次调用都新建一棵命令树（flags 不跨调用残留，测试可反复建根命令）。
func RegisterChannelCommand(root *cobra.Command) {
	channelCmd = buildChannelCmd()
	root.AddCommand(channelCmd)
}

// buildChannelCmd 新建 `sunshine channel` 命令树。
func buildChannelCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "channel",
		Short: "多 Agent 协同：通过共享事件日志派生、协调与打断 worker",
		Long: `多 Agent 协同：通过共享事件日志派生、协调与打断 worker。

channel 是多个 worker（claude / codex）协作的会话：create 建会话，
spawn 注册 worker，send 发消息，wait 等事件，interrupt 打断回合，
run 一次性跑完一个任务。

存储根为 ~/.sunshine/channels/（可用 SUNSHINE_CHANNEL_ROOT 覆盖）。

示例：
  sunshine channel create review --type chat --task .sunshine/tasks/11-09-xxx
  sunshine channel spawn review --agent implement --provider claude --as impl
  sunshine channel send review --to impl "开始实现任务"
  sunshine channel wait review --from impl --kind done --timeout 5m
  sunshine channel run --agent implement --provider claude --message "修这个 bug"`,
	}
	cmd.AddCommand(
		newChannelCreateCmd(),
		newChannelSendCmd(),
		newChannelWaitCmd(),
		newChannelInterruptCmd(),
		newChannelSpawnCmd(),
		newChannelRunCmd(),
		newChannelRmCmd(),
		newChannelPruneCmd(),
		newChannelListCmd(),
		newChannelMessagesCmd(),
		newChannelKillCmd(),
		newChannelContextCmd(),
		newChannelTitleCmd(),
		newChannelSupervisorCmd(),
		newChannelParseTraceCmd(),
	)
	return cmd
}

// ---------------------------------------------------------------------------
// 共享 helpers
// ---------------------------------------------------------------------------

// channelStdin 是正文 --stdin 的来源（默认 os.Stdin；测试可替换）。
var channelStdin io.Reader = os.Stdin

// parseCSV 把逗号分隔的字符串拆成数组（去空、去首尾空白；空串返回 nil）。
func parseCSV(s string) []string {
	if strings.TrimSpace(s) == "" {
		return nil
	}
	parts := strings.Split(s, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

// parseChannelScopeFlag 校验 --scope（空串表示未指定，由后端按默认解析）。
func parseChannelScopeFlag(s string) (string, error) {
	switch s {
	case "", ScopeProject, ScopeGlobal:
		return s, nil
	default:
		return "", fmt.Errorf("作用域须为 project 或 global，当前是 %q", s)
	}
}

// parseChannelTypeFlag 校验 --type（空串默认 chat；
// 上游旧日志里的 thread/threads 不归一化为 forum，传了就报错）。
func parseChannelTypeFlag(s string) (string, error) {
	switch s {
	case "", ChannelTypeChat:
		return ChannelTypeChat, nil
	case ChannelTypeForum:
		return ChannelTypeForum, nil
	default:
		return "", fmt.Errorf("类型须为 chat 或 forum，当前是 %q", s)
	}
}

// validateChannelName 校验 channel 名：与上游 assertSafeName 一致，
// 只允许字母、数字、.、_ 和 -（名字会成为文件系统路径段，
// 中文名等不在此集合里的字符一律拒绝；与 B 组 store 的规则相同）。
func validateChannelName(name string) error {
	if name == "" {
		return fmt.Errorf("channel 名不能为空")
	}
	if name == "." || name == ".." {
		return fmt.Errorf("channel 名 %q 不合法：只允许字母、数字、.、_ 和 -", name)
	}
	for _, r := range name {
		ok := r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '.' || r == '_' || r == '-'
		if !ok {
			return fmt.Errorf("channel 名 %q 不合法：只允许字母、数字、.、_ 和 -", name)
		}
	}
	return nil
}

// channelRefOf 由名字与 --scope 拼 ChannelRef（项目桶由后端解析）。
func channelRefOf(name, scope string) ChannelRef {
	return ChannelRef{Name: name, Scope: scope}
}

// resolveSender 解析发送者身份：--by 优先，--as 为兼容写法，默认 main。
// 上游 send/wait/interrupt/context/title 用 --as，PLAN.md 用 --by；
// 两者都接受，--by 优先。
func resolveSender(cmd *cobra.Command) string {
	if by, _ := cmd.Flags().GetString("by"); cmd.Flags().Changed("by") && by != "" {
		return by
	}
	if as, _ := cmd.Flags().GetString("as"); cmd.Flags().Changed("as") && as != "" {
		return as
	}
	if by, _ := cmd.Flags().GetString("by"); by != "" {
		return by
	}
	if as, _ := cmd.Flags().GetString("as"); as != "" {
		return as
	}
	return "main"
}

// addSenderFlags 给命令加 --by（主）与 --as（兼容旧写法，隐藏）。
func addSenderFlags(cmd *cobra.Command, def string) {
	cmd.Flags().String("by", def, "发送者身份（默认 main）")
	cmd.Flags().String("as", "", "发送者身份（兼容旧写法，等同 --by）")
	_ = cmd.Flags().MarkHidden("as")
}

// addScopeFlag 给命令加 --scope。
func addScopeFlag(cmd *cobra.Command) {
	cmd.Flags().String("scope", "", "channel 作用域：project（当前项目）或 global（全局）")
}

// parseDurationFlag 解析时长：Go 的 time.ParseDuration（30s/2m/1h）；
// 另兼容上游的纯数字写法（"30" 视为 30 秒）。
func parseDurationFlag(s string) (time.Duration, error) {
	if strings.TrimSpace(s) == "" {
		return 0, nil
	}
	if d, err := time.ParseDuration(strings.TrimSpace(s)); err == nil {
		return d, nil
	}
	// 上游支持天（7d）；Go 的 time.ParseDuration 没有 d 单位，单独处理。
	if trimmed := strings.TrimSpace(s); strings.HasSuffix(trimmed, "d") {
		if n, err := strconv.Atoi(strings.TrimSuffix(trimmed, "d")); err == nil && n >= 0 {
			return time.Duration(n) * 24 * time.Hour, nil
		}
	}
	if n, err := strconv.Atoi(strings.TrimSpace(s)); err == nil && n >= 0 {
		return time.Duration(n) * time.Second, nil
	}
	return 0, fmt.Errorf("时长 %q 不合法：用 30s、2m、1h、7d 这样的写法（纯数字视为秒）", s)
}

// parseDurationFlagDefault 同 parseDurationFlag，空串时返回默认值。
func parseDurationFlagDefault(s string, def time.Duration) (time.Duration, error) {
	if strings.TrimSpace(s) == "" {
		return def, nil
	}
	return parseDurationFlag(s)
}

// resolveChannelTextBody 解析正文：位置参数 > --text-file > --stdin；
// required 时缺正文或空白正文都报错（与上游 text-body.ts 一致）。
func resolveChannelTextBody(cmd *cobra.Command, args []string, required bool, missingMsg, emptyMsg string) (string, error) {
	var raw string
	var ok bool
	if len(args) > 0 && strings.TrimSpace(args[0]) != "" {
		raw, ok = args[0], true
	} else if file, _ := cmd.Flags().GetString("text-file"); file != "" {
		b, err := os.ReadFile(file)
		if err != nil {
			return "", fmt.Errorf("读正文文件 %s 失败：%v", file, err)
		}
		raw, ok = string(b), true
	} else if stdin, _ := cmd.Flags().GetBool("stdin"); stdin {
		b, err := io.ReadAll(channelStdin)
		if err != nil {
			return "", fmt.Errorf("读标准输入失败：%v", err)
		}
		raw, ok = string(b), true
	}
	if !ok {
		if required {
			return "", fmt.Errorf("%s", missingMsg)
		}
		return "", nil
	}
	text := strings.TrimRight(raw, " \t\r\n")
	if text == "" {
		return "", fmt.Errorf("%s", emptyMsg)
	}
	return text, nil
}

// buildContextEntries 由 --file/--raw 组装上下文条目；
// file 条目须为绝对路径（与上游 ContextEntry 要求一致）。
func buildContextEntries(files, raws []string) ([]ChannelContextEntry, error) {
	var out []ChannelContextEntry
	for _, f := range files {
		if !strings.HasPrefix(f, "/") && len(f) < 2 {
			return nil, fmt.Errorf("上下文文件须为绝对路径：%s", f)
		}
		if !isAbsPath(f) {
			return nil, fmt.Errorf("上下文文件须为绝对路径：%s", f)
		}
		out = append(out, ChannelContextEntry{Type: "file", Path: f})
	}
	for _, r := range raws {
		if strings.TrimSpace(r) == "" {
			continue
		}
		out = append(out, ChannelContextEntry{Type: "raw", Text: r})
	}
	return out, nil
}

// isAbsPath 判断绝对路径（兼容 Unix 与 Windows 盘符写法）。
func isAbsPath(p string) bool {
	if strings.HasPrefix(p, "/") {
		return true
	}
	if len(p) >= 3 && p[1] == ':' && (p[2] == '/' || p[2] == '\\') {
		return true
	}
	return false
}

// validateKindList 校验 --kind 的 kind 名。
func validateKindList(kinds []string) error {
	for _, k := range kinds {
		found := false
		for _, known := range ChannelEventKinds {
			if k == known {
				found = true
				break
			}
		}
		if !found {
			return fmt.Errorf("未知事件 kind %q，可选：%s", k, strings.Join(ChannelEventKinds, "、"))
		}
	}
	return nil
}

// validateThreadAction 校验 thread action。
func validateThreadAction(action string) error {
	if action == "" {
		return nil
	}
	for _, a := range ThreadActions {
		if action == a {
			return nil
		}
	}
	return fmt.Errorf("未知 thread 动作 %q，可选：%s", action, strings.Join(ThreadActions, "、"))
}

// printChannelEventJSON 把事件打印为一行 JSON（上游 send/interrupt/
// context/title/wait 的输出格式）。
func printChannelEventJSON(ev ChannelEvent) error {
	out, err := json.Marshal(ev)
	if err != nil {
		return fmt.Errorf("序列化事件失败：%v", err)
	}
	fmt.Println(string(out))
	return nil
}

// printChannelEventJSONIndent 同上，缩进两格（list --json 用）。
func printChannelJSON(v any) error {
	out, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return fmt.Errorf("序列化输出失败：%v", err)
	}
	fmt.Println(string(out))
	return nil
}

// channelBackendOf 取本次调用的后端。
func channelBackendOf() channelBackend { return newChannelBackend() }

// interruptibleContext 返回一个收到 SIGINT 即取消的 context。
func interruptibleContext() (context.Context, context.CancelFunc) {
	return signal.NotifyContext(context.Background(), os.Interrupt)
}

// ---------------------------------------------------------------------------
// create
// ---------------------------------------------------------------------------

func newChannelCreateCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "create <name>",
		Short: "新建一个 channel（协作会话）",
		Long: `新建一个 channel（协作会话）。

类型：chat（时间线式，默认）或 forum（主题区）。
channel 名只允许字母、数字、.、_ 和 -（会成为文件系统路径段）。
临时 channel（--ephemeral）默认不列入 channel list，
可用 channel prune --ephemeral 清理。

示例：
  sunshine channel create review --type chat --task .sunshine/tasks/11-09-xxx
  sunshine channel create req-pool --type forum --ephemeral
  sunshine channel create review --context-file /绝对路径/需求.md --context-raw "只改退款逻辑"`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			name := args[0]
			if err := validateChannelName(name); err != nil {
				return err
			}
			scope, _ := cmd.Flags().GetString("scope")
			scope, err := parseChannelScopeFlag(scope)
			if err != nil {
				return err
			}
			typeFlag, _ := cmd.Flags().GetString("type")
			channelType, err := parseChannelTypeFlag(typeFlag)
			if err != nil {
				return err
			}
			labels, _ := cmd.Flags().GetString("labels")
			contextFiles, _ := cmd.Flags().GetStringArray("context-file")
			contextRaws, _ := cmd.Flags().GetStringArray("context-raw")
			linkedFiles, _ := cmd.Flags().GetStringArray("linked-context-file")
			linkedRaws, _ := cmd.Flags().GetStringArray("linked-context-raw")
			entries, err := buildContextEntries(
				append(append([]string{}, contextFiles...), linkedFiles...),
				append(append([]string{}, contextRaws...), linkedRaws...),
			)
			if err != nil {
				return err
			}
			task, _ := cmd.Flags().GetString("task")
			project, _ := cmd.Flags().GetString("project")
			description, _ := cmd.Flags().GetString("description")
			cwd, _ := cmd.Flags().GetString("cwd")
			by, _ := cmd.Flags().GetString("by")
			if by == "" {
				by = "main"
			}
			force, _ := cmd.Flags().GetBool("force")
			ephemeral, _ := cmd.Flags().GetBool("ephemeral")

			ctx := context.Background()
			ref, _, err := channelBackendOf().CreateChannel(ctx, CreateChannelOptions{
				Name: name, Scope: scope, Type: channelType,
				Task: task, Project: project, Labels: parseCSV(labels),
				Description: description, Cwd: cwd, By: by,
				Force: force, Ephemeral: ephemeral, Context: entries,
			})
			if err != nil {
				return err
			}
			fmt.Printf("已创建 channel '%s'（%s）位于 %s\n", name, channelType, ref.Dir)
			if ephemeral {
				fmt.Fprintln(os.Stderr, "临时 channel 默认不列入 channel list；用 channel list --all 查看，或用 channel prune --ephemeral 清理")
			}
			return nil
		},
	}
	addScopeFlag(cmd)
	cmd.Flags().String("type", "chat", "channel 类型：chat（时间线式）或 forum（主题区）")
	cmd.Flags().String("task", "", "关联的任务目录（如 .sunshine/tasks/11-09-xxx）")
	cmd.Flags().String("project", "", "项目标识")
	cmd.Flags().String("labels", "", "逗号分隔的标签")
	cmd.Flags().String("description", "", "channel 描述（稳定描述）")
	cmd.Flags().StringArray("context-file", nil, "上下文文件绝对路径（可重复）")
	cmd.Flags().StringArray("context-raw", nil, "上下文文本（可重复）")
	cmd.Flags().StringArray("linked-context-file", nil, "上下文文件绝对路径（旧写法，等同 --context-file，可重复）")
	cmd.Flags().StringArray("linked-context-raw", nil, "上下文文本（旧写法，等同 --context-raw，可重复）")
	_ = cmd.Flags().MarkHidden("linked-context-file")
	_ = cmd.Flags().MarkHidden("linked-context-raw")
	cmd.Flags().String("cwd", "", "记入创建事件的工作目录")
	cmd.Flags().String("by", "main", "创建者身份")
	cmd.Flags().Bool("force", false, "同名 channel 已存在时覆盖")
	cmd.Flags().Bool("ephemeral", false, "标记为临时 channel：默认不列入 channel list，可由 channel prune --ephemeral 清理")
	return cmd
}

// ---------------------------------------------------------------------------
// send
// ---------------------------------------------------------------------------

func newChannelSendCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "send <name> [正文]",
		Short: "向 channel 发一条消息",
		Long: `向 channel 发一条消息。

正文来源（按优先级）：位置参数 > --text-file > --stdin。
--to 不传为广播；传 worker 名（逗号分隔可多发）为定向消息。
--delivery-mode 校验定向投递：appendOnly（默认，只追加）、
requireKnownWorker（目标从未出现则追加 undeliverable）、
requireRunningWorker（目标未知或已终态则追加 undeliverable）。

示例：
  sunshine channel send review --by main --to impl "开始实现任务"
  sunshine channel send review --by main --stdin < 需求.txt
  sunshine channel send review --by main --text-file 需求.txt --delivery-mode requireRunningWorker`,
		Args: cobra.MaximumNArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			name := args[0]
			if err := validateChannelName(name); err != nil {
				return err
			}
			textArgs := []string{}
			if len(args) > 1 {
				textArgs = args[1:]
			}
			text, err := resolveChannelTextBody(cmd, textArgs, true,
				"未提供正文（用位置参数、--stdin 或 --text-file 提供）", "消息正文为空")
			if err != nil {
				return err
			}
			toFlag, _ := cmd.Flags().GetString("to")
			deliveryMode, _ := cmd.Flags().GetString("delivery-mode")
			if deliveryMode != "" {
				matched := false
				for _, m := range DeliveryModes {
					if deliveryMode == m {
						matched = true
						break
					}
				}
				if !matched {
					return fmt.Errorf("投递模式须为 %s，当前是 %q", strings.Join(DeliveryModes, "、"), deliveryMode)
				}
			}
			scope, _ := cmd.Flags().GetString("scope")
			scope, err = parseChannelScopeFlag(scope)
			if err != nil {
				return err
			}
			by := resolveSender(cmd)
			ctx := context.Background()
			ev, err := channelBackendOf().SendMessage(ctx, SendMessageOptions{
				Channel: name, Scope: scope, By: by, Text: text,
				To: parseCSV(toFlag), DeliveryMode: deliveryMode,
			})
			if err != nil {
				return err
			}
			return printChannelEventJSON(ev)
		},
	}
	addScopeFlag(cmd)
	addSenderFlags(cmd, "main")
	cmd.Flags().String("to", "", "定向目标 worker（逗号分隔；不传为广播）")
	cmd.Flags().Bool("stdin", false, "从标准输入读消息正文")
	cmd.Flags().String("text-file", "", "从文件读消息正文")
	cmd.Flags().String("delivery-mode", "", "定向投递校验：appendOnly（默认）、requireKnownWorker、requireRunningWorker")
	return cmd
}

// ---------------------------------------------------------------------------
// wait
// ---------------------------------------------------------------------------

// waitTimeoutError 是 wait 超时错误（上游退出码 124；
// cobra RunE 返回错误时进程退出码为 1，见 PLAN.md 阶段 6 C 组记录差异 6）。
type waitTimeoutError struct {
	channel string
	timeout time.Duration
	pending []string
}

func (e *waitTimeoutError) Error() string {
	if len(e.pending) > 0 {
		return fmt.Sprintf("超时：等待 channel '%s' 超时（%s），仍在等待：%s（上游退出码 124）",
			e.channel, e.timeout, strings.Join(e.pending, "、"))
	}
	return fmt.Sprintf("超时：等待 channel '%s' 超时（%s）（上游退出码 124）", e.channel, e.timeout)
}

func newChannelWaitCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "wait <name>",
		Short: "阻塞等待匹配事件或超时",
		Long: `阻塞等待匹配事件或超时。

只看调用之后新到的事件（旧事件不重放）。匹配到第一个事件即打印
该事件 JSON 并退出；--all 时等 --from 里的每个 agent 都各出一个
匹配事件才退出。超时后报错退出。
默认只认有意义的事件（create/message/done/error 等）；progress
事件须加 --include-progress；--kind 可显式指定 kind（含 progress、
supervisor_warning 等非默认 kind）。

示例：
  sunshine channel wait review --from impl --kind done --timeout 5m
  sunshine channel wait review --from impl、checker --all --timeout 10m
  sunshine channel wait req-pool --thread 需求-1 --action comment --timeout 2m`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			name := args[0]
			if err := validateChannelName(name); err != nil {
				return err
			}
			scope, _ := cmd.Flags().GetString("scope")
			scope, err := parseChannelScopeFlag(scope)
			if err != nil {
				return err
			}
			self := resolveSender(cmd)
			fromFlag, _ := cmd.Flags().GetString("from")
			from := parseCSV(fromFlag)
			kindFlag, _ := cmd.Flags().GetString("kind")
			kinds := parseCSV(kindFlag)
			if err := validateKindList(kinds); err != nil {
				return err
			}
			thread, _ := cmd.Flags().GetString("thread")
			thread = strings.TrimSpace(thread)
			action, _ := cmd.Flags().GetString("action")
			if err := validateThreadAction(action); err != nil {
				return err
			}
			to, _ := cmd.Flags().GetString("to")
			includeProgress, _ := cmd.Flags().GetBool("include-progress")
			all, _ := cmd.Flags().GetBool("all")
			if all && len(from) == 0 {
				return fmt.Errorf("--all 须配合 --from <a,b,...> 使用")
			}
			timeoutStr, _ := cmd.Flags().GetString("timeout")
			timeout, err := parseDurationFlag(timeoutStr)
			if err != nil {
				return err
			}
			ctx := context.Background()
			if timeout > 0 {
				var cancel context.CancelFunc
				ctx, cancel = context.WithTimeout(ctx, timeout)
				defer cancel()
			}
			sigCtx, stop := interruptibleContext()
			defer stop()
			ctx = mergeWaitContexts(ctx, sigCtx)

			filter := WaitFilter{
				Self: self, From: from, Kinds: kinds, To: to,
				Thread: thread, Action: action, IncludeProgress: includeProgress,
			}
			ch, err := channelBackendOf().WatchEvents(ctx, channelRefOf(name, scope), filter)
			if err != nil {
				return err
			}
			pending := map[string]bool{}
			for _, f := range from {
				pending[f] = true
			}
			for ev := range ch {
				if err := printChannelEventJSON(ev); err != nil {
					return err
				}
				if !all {
					return nil
				}
				delete(pending, ev.By())
				if len(pending) == 0 {
					return nil
				}
			}
			// 通道关闭且未满足条件：超时或中断。
			if ctx.Err() == context.DeadlineExceeded {
				rest := make([]string, 0, len(pending))
				for p := range pending {
					rest = append(rest, p)
				}
				return &waitTimeoutError{channel: name, timeout: timeout, pending: rest}
			}
			if ctx.Err() == context.Canceled {
				return fmt.Errorf("等待被中断")
			}
			return fmt.Errorf("等待结束，未收到匹配事件")
		},
	}
	addScopeFlag(cmd)
	addSenderFlags(cmd, "main")
	cmd.Flags().String("timeout", "", "最长等待时长，如 30s、2m、1h（不传则一直等到 Ctrl-C）")
	cmd.Flags().String("from", "", "只认这些 agent 发的事件（逗号分隔）")
	cmd.Flags().String("kind", "", "只认这些事件 kind（逗号分隔，如 message、done）")
	cmd.Flags().String("to", "", "只认发给该目标的事件（默认发给自己；无 to 的广播也算匹配）")
	cmd.Flags().String("thread", "", "只认该 thread 的事件")
	cmd.Flags().String("action", "", "只认该 thread 动作的事件：opened、comment、status、labels、assignees、summary、processed")
	cmd.Flags().Bool("include-progress", false, "也认 progress 事件")
	cmd.Flags().Bool("all", false, "等 --from 里的每个 agent 都各出一个匹配事件才退出（默认第一个匹配即退出）")
	return cmd
}

// mergeWaitContexts 把超时 ctx 与信号 ctx 合并：任一取消即取消。
func mergeWaitContexts(timeoutCtx, sigCtx context.Context) context.Context {
	ctx, cancel := context.WithCancel(timeoutCtx)
	go func() {
		select {
		case <-timeoutCtx.Done():
			cancel()
		case <-sigCtx.Done():
			cancel()
		case <-ctx.Done():
		}
	}()
	return ctx
}

// ---------------------------------------------------------------------------
// interrupt
// ---------------------------------------------------------------------------

func newChannelInterruptCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "interrupt <name> [正文]",
		Short: "打断 worker 当前回合并发送替换指令",
		Long: `打断 worker 当前回合并发送替换指令。

正文来源（按优先级）：位置参数 > --text-file > --stdin。
先追加 interrupt_requested 事件（持久化意图），再由后端在有
runtime 时尝试中断 worker。

示例：
  sunshine channel interrupt review --to impl "停下，先改退款原因那一处"
  sunshine channel interrupt review --to impl --text-file 新指令.txt`,
		Args: cobra.MaximumNArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			name := args[0]
			if err := validateChannelName(name); err != nil {
				return err
			}
			to, _ := cmd.Flags().GetString("to")
			if strings.TrimSpace(to) == "" {
				return fmt.Errorf("须指定 --to <worker>（被打断的 worker）")
			}
			textArgs := []string{}
			if len(args) > 1 {
				textArgs = args[1:]
			}
			text, err := resolveChannelTextBody(cmd, textArgs, true,
				"未提供打断指令（用位置参数、--stdin 或 --text-file 提供）", "打断指令为空")
			if err != nil {
				return err
			}
			scope, _ := cmd.Flags().GetString("scope")
			scope, err = parseChannelScopeFlag(scope)
			if err != nil {
				return err
			}
			by := resolveSender(cmd)
			ctx := context.Background()
			res, err := channelBackendOf().InterruptWorker(ctx, InterruptOptions{
				Channel: name, Scope: scope, By: by,
				Worker: strings.TrimSpace(to), Message: text, Reason: "user",
			}, channelWorkerRuntime)
			if err != nil {
				return err
			}
			if err := printChannelEventJSON(res.Event); err != nil {
				return err
			}
			if !res.Interrupted {
				fmt.Fprintln(os.Stderr, "警告：worker 未确认中断（可能已结束或不在运行），interrupt_requested 事件已记录")
			}
			return nil
		},
	}
	addScopeFlag(cmd)
	addSenderFlags(cmd, "main")
	cmd.Flags().String("to", "", "被打断的 worker 名（必填）")
	cmd.Flags().String("text-file", "", "从文件读打断指令")
	cmd.Flags().Bool("stdin", false, "从标准输入读打断指令")
	return cmd
}
