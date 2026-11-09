// Package cli 的 channel 管理类命令（阶段 6 C 组）：
// rm / prune / list / messages / kill / context / title /
// __supervisor / __parse-trace。
package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"
)

// ---------------------------------------------------------------------------
// rm
// ---------------------------------------------------------------------------

func newChannelRmCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "rm <name>",
		Short: "杀掉全部 worker 并删除整个 channel 目录",
		Long: `杀掉全部 worker 并删除整个 channel 目录。

示例：
  sunshine channel rm review`,
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
			if err := channelBackendOf().RemoveChannel(context.Background(), channelRefOf(name, scope)); err != nil {
				return err
			}
			fmt.Printf("已删除 channel '%s'\n", name)
			return nil
		},
	}
	addScopeFlag(cmd)
	return cmd
}

// ---------------------------------------------------------------------------
// prune
// ---------------------------------------------------------------------------

func newChannelPruneCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "prune",
		Short: "按条件批量清理 channel（默认只演练，不真删）",
		Long: `按条件批量清理 channel。

默认是演练（dry-run）：只列出将删除的 channel，不真删；
传 --yes 才真删。--all、--empty、--idle、--ephemeral 四个条件
互斥，只能选其一；不选任一条件时无事可做。
仍有存活 worker 的 channel 永远跳过；--keep 里的名字永远保留。

示例：
  sunshine channel prune --empty
  sunshine channel prune --empty --yes
  sunshine channel prune --ephemeral --yes
  sunshine channel prune --all --keep review --yes
  sunshine channel prune --idle 7d --yes`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			all, _ := cmd.Flags().GetBool("all")
			empty, _ := cmd.Flags().GetBool("empty")
			idleStr, _ := cmd.Flags().GetString("idle")
			ephemeral, _ := cmd.Flags().GetBool("ephemeral")
			yes, _ := cmd.Flags().GetBool("yes")
			dryRun, _ := cmd.Flags().GetBool("dry-run")
			keepFlag, _ := cmd.Flags().GetString("keep")
			scope, _ := cmd.Flags().GetString("scope")
			scope, err := parseChannelScopeFlag(scope)
			if err != nil {
				return err
			}
			// 四个条件互斥（与上游 channelPrune 一致）。
			modes := []string{}
			if ephemeral {
				modes = append(modes, "--ephemeral")
			}
			if all {
				modes = append(modes, "--all")
			}
			if empty {
				modes = append(modes, "--empty")
			}
			idle, err := parseDurationFlag(idleStr)
			if err != nil {
				return err
			}
			if idle > 0 {
				modes = append(modes, "--idle")
			}
			if len(modes) > 1 {
				return fmt.Errorf("prune 条件互斥：%s，只能选其一", strings.Join(modes, " / "))
			}
			if len(modes) == 0 {
				return fmt.Errorf("须指定一个清理条件：--all、--empty、--idle <时长> 或 --ephemeral")
			}
			backend := channelBackendOf()
			ctx := context.Background()
			candidates, err := backend.ListPruneCandidates(ctx, PruneOptions{
				Scope: scope, All: all, Empty: empty, Idle: idle,
				Ephemeral: ephemeral, Keep: parseCSV(keepFlag),
			})
			if err != nil {
				return err
			}
			if len(candidates) == 0 {
				fmt.Println("（无可清理的 channel）")
				return nil
			}
			for _, c := range candidates {
				last := "-"
				if c.LastTs != "" {
					last = strings.Replace(c.LastTs[:min(len(c.LastTs), 19)], "T", " ", 1)
				}
				fmt.Printf("  %-24s  %s  (%s)\n", c.Name, last, c.Reason)
			}
			if dryRun && !yes {
				fmt.Printf("\n（演练）将删除 %d 个 channel\n", len(candidates))
				return nil
			}
			if !yes {
				fmt.Printf("\n拒绝删除 %d 个 channel（未传 --yes）。重新运行并加上 --yes，或用 --dry-run 预览。\n", len(candidates))
				return nil
			}
			removed := 0
			for _, c := range candidates {
				ref := ChannelRef{Name: c.Name, Scope: scope, Project: c.Project}
				if err := backend.RemoveChannel(ctx, ref); err != nil {
					fmt.Fprintf(os.Stderr, "  删除 %s 失败：%v\n", c.Name, err)
					continue
				}
				removed++
			}
			fmt.Printf("\n已删除 %d 个 channel\n", removed)
			return nil
		},
	}
	addScopeFlag(cmd)
	cmd.Flags().Bool("all", false, "删除全部 channel（仍有存活 worker 的与 --keep 的除外）")
	cmd.Flags().Bool("empty", false, "删除无活动的 channel（只有 create 事件）")
	cmd.Flags().String("idle", "", "删除最后事件早于该时长的 channel，如 1h、7d")
	cmd.Flags().Bool("ephemeral", false, "只删除创建时标记为临时（--ephemeral）的 channel")
	cmd.Flags().Bool("yes", false, "真删（默认只演练）")
	cmd.Flags().Bool("dry-run", true, "只列出将删除的 channel，不真删")
	cmd.Flags().String("keep", "", "逗号分隔的 channel 名，永远保留")
	return cmd
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// ---------------------------------------------------------------------------
// list
// ---------------------------------------------------------------------------

func newChannelListCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "list",
		Short: "列出 channel（含 worker 与活动摘要）",
		Long: `列出 channel（含 worker 与活动摘要）。

默认只列当前项目的 channel，且隐藏临时 channel；
--all 显示临时 channel，--all-projects 扫描全部项目桶。
按最近活动倒序排列。

示例：
  sunshine channel list
  sunshine channel list --all
  sunshine channel list --json
  sunshine channel list --scope global`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			scope, _ := cmd.Flags().GetString("scope")
			scope, err := parseChannelScopeFlag(scope)
			if err != nil {
				return err
			}
			asJSON, _ := cmd.Flags().GetBool("json")
			project, _ := cmd.Flags().GetString("project")
			all, _ := cmd.Flags().GetBool("all")
			allProjects, _ := cmd.Flags().GetBool("all-projects")
			rows, err := channelBackendOf().ListChannels(context.Background(), ListChannelsOptions{
				Scope: scope, Project: project, All: all, AllProjects: allProjects,
			})
			if err != nil {
				return err
			}
			if asJSON {
				if rows == nil {
					rows = []ChannelSummary{}
				}
				return printChannelJSON(rows)
			}
			if len(rows) == 0 {
				fmt.Println("（无匹配的 channel）")
				return nil
			}
			// 临时 channel 计数（后端已按 --all 过滤；这里只作提示用）。
			fmt.Printf("%-24s  %-9s  %-6s  %-19s  %-9s  %-7s  %s\n", "名称", "工作者", "事件", "最近活动", "最近事件", "类型", "任务")
			for _, r := range rows {
				name := r.Name
				if r.Ephemeral {
					name += " *"
				}
				workers := "-"
				if r.WorkersTotal > 0 {
					workers = fmt.Sprintf("%d/%d", r.WorkersAlive, r.WorkersTotal)
				}
				last := "-"
				if r.LastEventTs != "" {
					last = strings.Replace(r.LastEventTs[:min(len(r.LastEventTs), 19)], "T", " ", 1)
				}
				kind := r.LastEventKind
				if kind == "" {
					kind = "-"
				}
				task := r.Task
				if task == "" {
					task = r.Description
				}
				if task == "" {
					task = "-"
				}
				fmt.Printf("%-24s  %-9s  %-6d  %-19s  %-9s  %-7s  %s\n",
					truncateChannelCell(name, 24), workers, r.TotalEvents, last, kind, r.Type, task)
			}
			ephemeralHidden := 0
			for _, r := range rows {
				if r.Ephemeral {
					ephemeralHidden++
				}
			}
			_ = ephemeralHidden
			return nil
		},
	}
	addScopeFlag(cmd)
	cmd.Flags().Bool("json", false, "输出机器可读的 JSON")
	cmd.Flags().String("project", "", "只列 task 字段包含该子串的 channel")
	cmd.Flags().Bool("all", false, "含临时 channel（默认隐藏）")
	cmd.Flags().Bool("all-projects", false, "扫描全部项目桶（默认只列当前项目）")
	return cmd
}

// truncateChannelCell 截断表格单元格（超长加 …）。
func truncateChannelCell(s string, w int) string {
	if len([]rune(s)) <= w {
		return s
	}
	return string([]rune(s)[:w-1]) + "…"
}

// ---------------------------------------------------------------------------
// messages
// ---------------------------------------------------------------------------

func newChannelMessagesCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "messages <name>",
		Short: "查看 channel 里的消息与事件",
		Long: `查看 channel 里的消息与事件。

--thread / --action 只对 forum channel 有效（chat channel 传了会报错）。
--follow 持续输出新到的事件，直到 Ctrl-C。
--raw 按行打印原始事件 JSON。

示例：
  sunshine channel messages review
  sunshine channel messages review --last 20 --raw
  sunshine channel messages review --from impl --kind done
  sunshine channel messages req-pool --thread 需求-1
  sunshine channel messages review --follow`,
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
			raw, _ := cmd.Flags().GetBool("raw")
			follow, _ := cmd.Flags().GetBool("follow")
			last, _ := cmd.Flags().GetInt("last")
			since, _ := cmd.Flags().GetInt64("since")
			kind, _ := cmd.Flags().GetString("kind")
			fromFlag, _ := cmd.Flags().GetString("from")
			to, _ := cmd.Flags().GetString("to")
			thread, _ := cmd.Flags().GetString("thread")
			thread = strings.TrimSpace(thread)
			action, _ := cmd.Flags().GetString("action")
			noProgress, _ := cmd.Flags().GetBool("no-progress")
			if kind != "" {
				if err := validateKindList([]string{kind}); err != nil {
					return err
				}
			}
			if err := validateThreadAction(action); err != nil {
				return err
			}
			backend := channelBackendOf()
			ctx := context.Background()
			ref := channelRefOf(name, scope)
			// chat channel 不接受 --thread/--action（上游 messages.ts 的校验）：
			// 类型以首个 create 事件的 type 为准；须用不过滤的读拿到
			// create 事件（带 --thread 过滤的读会把 create 事件滤掉）。
			if thread != "" || action != "" {
				allEvents, err := backend.ReadEvents(ctx, ref, ReadEventsOptions{
					IncludeProgress: true, IncludeNonMeaning: true,
				})
				if err != nil {
					return err
				}
				for _, ev := range allEvents {
					if ev.Kind() == "create" {
						if evString(ev, "type") == ChannelTypeChat {
							return fmt.Errorf("channel '%s' 是 chat 类型，--thread/--action 只适用于 forum channel", name)
						}
						break
					}
				}
			}
			opts := ReadEventsOptions{
				Kind: kind, From: parseCSV(fromFlag), To: to,
				Thread: thread, Action: action,
				IncludeProgress: !noProgress, IncludeNonMeaning: true,
			}
			if cmd.Flags().Changed("since") {
				v := since
				opts.AfterSeq = &v
			}
			events, err := backend.ReadEvents(ctx, ref, opts)
			if err != nil {
				return err
			}
			if last > 0 && len(events) > last {
				events = events[len(events)-last:]
			}
			for _, ev := range events {
				printChannelEventLine(ev, raw)
			}
			if !follow {
				return nil
			}
			// 跟随新事件：从已有最大 seq 之后开始。
			var afterSeq int64
			for _, ev := range events {
				if ev.Seq() > afterSeq {
					afterSeq = ev.Seq()
				}
			}
			sigCtx, stop := interruptibleContext()
			defer stop()
			filter := WaitFilter{
				From: parseCSV(fromFlag), To: to, Thread: thread, Action: action,
				IncludeProgress: !noProgress,
			}
			if kind != "" {
				filter.Kinds = []string{kind}
			}
			ch, err := backend.WatchEvents(sigCtx, ref, filter)
			if err != nil {
				return err
			}
			_ = afterSeq // 后端 WatchEvents 默认从当前时刻起推送（from-now）。
			for ev := range ch {
				printChannelEventLine(ev, raw)
			}
			return nil
		},
	}
	addScopeFlag(cmd)
	cmd.Flags().Bool("raw", false, "按行打印原始事件 JSON")
	cmd.Flags().Bool("follow", false, "持续输出新到的事件（Ctrl-C 停止）")
	cmd.Flags().Int("last", 0, "只显示最后 N 条匹配事件")
	cmd.Flags().Int64("since", 0, "只显示 seq 大于 N 的事件")
	cmd.Flags().String("kind", "", "按事件 kind 过滤，如 message、done、killed")
	cmd.Flags().String("from", "", "按作者过滤（逗号分隔）")
	cmd.Flags().String("to", "", "按路由目标过滤")
	cmd.Flags().String("thread", "", "按 thread 过滤（仅 forum channel）")
	cmd.Flags().String("action", "", "按 thread 动作过滤（仅 forum channel）")
	cmd.Flags().Bool("no-progress", false, "隐藏 progress 事件")
	return cmd
}

// printChannelEventLine 打印一行事件：--raw 为原始 JSON，
// 否则为人类可读格式（[seq] 时间 [kind] by=作者 [to=目标] 正文…）。
func printChannelEventLine(ev ChannelEvent, raw bool) {
	if raw {
		out, err := json.Marshal(ev)
		if err != nil {
			fmt.Fprintf(os.Stderr, "警告：序列化事件失败：%v\n", err)
			return
		}
		fmt.Println(string(out))
		return
	}
	ts := evString(ev, "ts")
	if len(ts) >= 19 {
		ts = ts[11:19]
	}
	line := fmt.Sprintf("[%d] %s [%s] by=%s", ev.Seq(), ts, ev.Kind(), ev.By())
	if targets := evToList(ev); len(targets) > 0 {
		line += " to=" + strings.Join(targets, ",")
	}
	switch ev.Kind() {
	case "create":
		if cwd := evString(ev, "cwd"); cwd != "" {
			line += " cwd=" + cwd
		}
		if task := evString(ev, "task"); task != "" {
			line += " 任务=" + task
		}
	case "spawned":
		line += fmt.Sprintf(" worker=%s provider=%s", evString(ev, "worker"), evString(ev, "provider"))
		if agent := evString(ev, "agent"); agent != "" {
			line += " agent=" + agent
		}
	case "killed":
		line += fmt.Sprintf(" 原因=%s 信号=%s", evString(ev, "reason"), evString(ev, "signal"))
	case "thread":
		line += fmt.Sprintf(" %s %s", evString(ev, "action"), evString(ev, "thread"))
	case "done":
		if d := evInt(ev, "duration_ms"); d != 0 {
			line += fmt.Sprintf(" 用时=%dms", d)
		}
	case "error":
		if msg := evString(ev, "message"); msg != "" {
			line += " " + msg
		}
	case "supervisor_warning":
		line += fmt.Sprintf(" worker=%s 原因=%s", evString(ev, "worker"), evString(ev, "reason"))
	}
	fmt.Println(line)
	if desc := evString(ev, "description"); desc != "" && ev.Kind() == "create" {
		fmt.Println("         描述：" + desc)
	}
	if text := ev.Text(); text != "" && (ev.Kind() == "message" || ev.Kind() == "thread" || ev.Kind() == "error") {
		fmt.Println("         " + strings.ReplaceAll(text, "\n", "\n         "))
	}
	if ev.Kind() == "create" || ev.Kind() == "thread" {
		if ctxs, ok := ev["context"].([]any); ok {
			for _, c := range ctxs {
				if m, ok := c.(map[string]any); ok {
					if m["type"] == "file" {
						fmt.Printf("         上下文：file %s\n", m["path"])
					} else if m["type"] == "raw" {
						fmt.Printf("         上下文：raw %s\n", summarizeChannelText(fmt.Sprint(m["text"]), 100))
					}
				}
			}
		}
	}
}

// summarizeChannelText 把文本压成一行并截断。
func summarizeChannelText(text string, limit int) string {
	one := strings.Join(strings.Fields(text), " ")
	if len([]rune(one)) > limit {
		return string([]rune(one)[:limit]) + "…"
	}
	return one
}

// ---------------------------------------------------------------------------
// kill
// ---------------------------------------------------------------------------

func newChannelKillCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "kill <name>",
		Short: "停止 channel 里的一个 worker",
		Long: `停止 channel 里的一个 worker（默认发 SIGTERM 优雅停止；
--force 直接 SIGKILL，跳过优雅停机）。

示例：
  sunshine channel kill review --as impl
  sunshine channel kill review --as impl --force`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			name := args[0]
			if err := validateChannelName(name); err != nil {
				return err
			}
			worker, _ := cmd.Flags().GetString("as")
			if strings.TrimSpace(worker) == "" {
				return fmt.Errorf("须指定 --as <worker>（要停止的 worker 名）")
			}
			scope, _ := cmd.Flags().GetString("scope")
			scope, err := parseChannelScopeFlag(scope)
			if err != nil {
				return err
			}
			force, _ := cmd.Flags().GetBool("force")
			if err := channelBackendOf().KillWorker(context.Background(), channelRefOf(name, scope), worker, force); err != nil {
				return err
			}
			fmt.Printf("已停止 worker '%s'（channel '%s'）\n", worker, name)
			return nil
		},
	}
	addScopeFlag(cmd)
	cmd.Flags().String("as", "", "要停止的 worker 名（必填）")
	cmd.Flags().Bool("force", false, "跳过优雅停止，直接 SIGKILL")
	return cmd
}

// ---------------------------------------------------------------------------
// context
// ---------------------------------------------------------------------------

func newChannelContextCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "context",
		Short: "管理 channel / thread 级上下文条目",
		Long: `管理 channel / thread 级上下文条目。

子命令：add（加条目）、delete（删条目）、list（列出当前条目）。
--thread 指定 thread 时操作 thread 级上下文，否则操作 channel 级。

示例：
  sunshine channel context add review --file /绝对路径/需求.md --raw "只改退款逻辑"
  sunshine channel context add req-pool --thread 需求-1 --raw "补充验收标准"
  sunshine channel context list review
  sunshine channel context delete review --raw "只改退款逻辑"`,
	}
	add := &cobra.Command{
		Use:   "add <name>",
		Short: "添加上下文条目",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runChannelContextMutate(cmd, args[0], true)
		},
	}
	del := &cobra.Command{
		Use:   "delete <name>",
		Short: "删除上下文条目",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runChannelContextMutate(cmd, args[0], false)
		},
	}
	for _, sub := range []*cobra.Command{add, del} {
		addScopeFlag(sub)
		addSenderFlags(sub, "main")
		sub.Flags().String("thread", "", "操作 thread 级上下文（不传为 channel 级）")
		sub.Flags().StringArray("file", nil, "上下文文件绝对路径（可重复）")
		sub.Flags().StringArray("raw", nil, "上下文文本（可重复）")
	}
	list := &cobra.Command{
		Use:   "list <name>",
		Short: "列出当前上下文条目",
		Args:  cobra.ExactArgs(1),
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
			thread, _ := cmd.Flags().GetString("thread")
			thread = strings.TrimSpace(thread)
			raw, _ := cmd.Flags().GetBool("raw")
			backend := channelBackendOf()
			ctx := context.Background()
			ref := channelRefOf(name, scope)
			var entries []ChannelContextEntry
			if thread != "" {
				entries, err = backend.ListThreadContext(ctx, ref, thread)
			} else {
				entries, err = backend.ListChannelContext(ctx, ref)
			}
			if err != nil {
				return err
			}
			if raw {
				for _, e := range entries {
					out, err := json.Marshal(e)
					if err != nil {
						return err
					}
					fmt.Println(string(out))
				}
				return nil
			}
			if len(entries) == 0 {
				fmt.Println("（无上下文）")
				return nil
			}
			for _, e := range entries {
				if e.Type == "file" {
					fmt.Printf("file %s\n", e.Path)
				} else {
					fmt.Printf("raw  %s\n", summarizeChannelText(e.Text, 200))
				}
			}
			return nil
		},
	}
	addScopeFlag(list)
	list.Flags().String("thread", "", "列出 thread 级上下文（不传为 channel 级）")
	list.Flags().Bool("raw", false, "每行打印一条上下文条目的 JSON")
	cmd.AddCommand(add, del, list)
	return cmd
}

// runChannelContextMutate 执行 context add / delete。
func runChannelContextMutate(cmd *cobra.Command, name string, isAdd bool) error {
	if err := validateChannelName(name); err != nil {
		return err
	}
	files, _ := cmd.Flags().GetStringArray("file")
	raws, _ := cmd.Flags().GetStringArray("raw")
	entries, err := buildContextEntries(files, raws)
	if err != nil {
		return err
	}
	if len(entries) == 0 {
		return fmt.Errorf("须至少提供一个 --file <绝对路径> 或 --raw <文本>")
	}
	scope, _ := cmd.Flags().GetString("scope")
	scope, err = parseChannelScopeFlag(scope)
	if err != nil {
		return err
	}
	thread, _ := cmd.Flags().GetString("thread")
	thread = strings.TrimSpace(thread)
	by := resolveSender(cmd)
	backend := channelBackendOf()
	ctx := context.Background()
	ref := channelRefOf(name, scope)
	var ev ChannelEvent
	if thread != "" {
		if isAdd {
			ev, err = backend.AddThreadContext(ctx, ref, thread, by, entries)
		} else {
			ev, err = backend.DeleteThreadContext(ctx, ref, thread, by, entries)
		}
	} else {
		if isAdd {
			ev, err = backend.AddChannelContext(ctx, ref, by, entries)
		} else {
			ev, err = backend.DeleteChannelContext(ctx, ref, by, entries)
		}
	}
	if err != nil {
		return err
	}
	return printChannelEventJSON(ev)
}

// ---------------------------------------------------------------------------
// title
// ---------------------------------------------------------------------------

func newChannelTitleCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "title",
		Short: "设置或清除 channel 显示标题",
		Long: `设置或清除 channel 显示标题。

子命令：set（设置，须传 --title）、clear（清除）。

示例：
  sunshine channel title set review --title "退款原因重构"
  sunshine channel title clear review`,
	}
	set := &cobra.Command{
		Use:   "set <name>",
		Short: "设置 channel 显示标题",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			name := args[0]
			if err := validateChannelName(name); err != nil {
				return err
			}
			title, _ := cmd.Flags().GetString("title")
			if strings.TrimSpace(title) == "" {
				return fmt.Errorf("须指定 --title <标题>")
			}
			scope, _ := cmd.Flags().GetString("scope")
			scope, err := parseChannelScopeFlag(scope)
			if err != nil {
				return err
			}
			ev, err := channelBackendOf().SetChannelTitle(context.Background(),
				channelRefOf(name, scope), resolveSender(cmd), title)
			if err != nil {
				return err
			}
			return printChannelEventJSON(ev)
		},
	}
	clear := &cobra.Command{
		Use:   "clear <name>",
		Short: "清除 channel 显示标题",
		Args:  cobra.ExactArgs(1),
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
			ev, err := channelBackendOf().ClearChannelTitle(context.Background(),
				channelRefOf(name, scope), resolveSender(cmd))
			if err != nil {
				return err
			}
			return printChannelEventJSON(ev)
		},
	}
	for _, sub := range []*cobra.Command{set, clear} {
		addScopeFlag(sub)
		addSenderFlags(sub, "main")
	}
	set.Flags().String("title", "", "显示标题（必填）")
	cmd.AddCommand(set, clear)
	return cmd
}

// ---------------------------------------------------------------------------
// __supervisor / __parse-trace（内部入口，实现在 D 组
// internal/channel/supervisor，经 channel_backend.go 的
// runSupervisorFunc / runParseTraceFunc 接线）
// ---------------------------------------------------------------------------

func newChannelSupervisorCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:    "__supervisor <channel> <worker> <config>",
		Short:  "[内部] supervisor 进程入口，请勿直接调用",
		Hidden: true,
		Args:   cobra.ExactArgs(3),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runSupervisorFunc(context.Background(), args[0], args[1], args[2])
		},
	}
	return cmd
}

func newChannelParseTraceCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:    "__parse-trace <adapter> <file>",
		Short:  "[开发用] 把一段录制的 trace 走适配器解析并打印事件",
		Hidden: true,
		Args:   cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			adapter := args[0]
			if adapter != "claude" && adapter != "codex" {
				return fmt.Errorf("未知适配器 %q（可选：claude、codex）", adapter)
			}
			return runParseTraceFunc(context.Background(), adapter, args[1])
		},
	}
	return cmd
}
