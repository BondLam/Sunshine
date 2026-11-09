// Package cli 的 sunshine channel forum 相关命令：post / forum / thread / rename。
//
// 挂接方式：子命令挂到 C 组在 channel.go 里定义的包级 channelCmd 变量上，
// 由 root.go 的 init 在 RegisterChannelCommand 之后调用
// RegisterChannelForumCommand() 完成挂接；本文件不自建 channel 父命令。
package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/linrong1982/Sunshine/internal/channel"
	"github.com/spf13/cobra"
)

// RegisterChannelForumCommand 把 forum 子命令（post、forum、thread、
// thread rename）挂到包级 channelCmd 上。
// 须在 RegisterChannelCommand 之后调用（root.go 的 init 里顺序调用）；
// channelCmd 尚未建立时不做任何事（避免测试里单独建根命令时 panic）。
func RegisterChannelForumCommand() {
	if channelCmd == nil {
		return
	}
	// 幂等：已挂接过则不再重复挂（重复挂接会让 help 里出现两遍）。
	for _, c := range channelCmd.Commands() {
		if c.Name() == "post" {
			return
		}
	}
	channelCmd.AddCommand(newChannelPostCommand())
	channelCmd.AddCommand(newChannelForumCommand())
	channelCmd.AddCommand(newChannelThreadCommand())
}

// channelForumStore 返回 forum 命令用的存储（存储根由 SUNSHINE_CHANNEL_ROOT 决定）。
func channelForumStore() channel.ForumStore {
	return channel.DefaultStore()
}

func printEventJSON(ev channel.ChannelEvent) error {
	data, err := json.Marshal(ev)
	if err != nil {
		return fmt.Errorf("输出事件失败：%w", err)
	}
	fmt.Println(string(data))
	return nil
}

// resolveTextBody 解析 post 的正文来源：--text > --text-file > --stdin；
// 未提供返回空串；提供了但内容为空（去尾部空白后）则报错。
func resolveTextBody(text, textFile string, stdin bool) (string, error) {
	var raw string
	switch {
	case text != "":
		raw = text
	case textFile != "":
		data, err := os.ReadFile(textFile)
		if err != nil {
			return "", fmt.Errorf("读正文文件失败：%w", err)
		}
		raw = string(data)
	case stdin:
		data, err := io.ReadAll(os.Stdin)
		if err != nil {
			return "", fmt.Errorf("读标准输入失败：%w", err)
		}
		raw = string(data)
	default:
		return "", nil
	}
	trimmed := strings.TrimRight(raw, " \t\r\n")
	if trimmed == "" {
		return "", fmt.Errorf("正文为空（用 --text、--stdin 或 --text-file 提供）")
	}
	return trimmed, nil
}

// parseScopeFlag 校验 --scope。
func parseScopeFlag(v string) (string, error) {
	return channel.ParseScope(v)
}

func newChannelPostCommand() *cobra.Command {
	var (
		as, scope, thread, title, text, textFile, description, status string
		labels, assignees, summary                                    string
		stdin                                                         bool
		contextFiles, contextRaws                                     []string
		linkedContextFiles, linkedContextRaws                         []string
	)
	cmd := &cobra.Command{
		Use:   "post <频道> <动作>",
		Short: "向 forum 频道追加结构化 thread 事件",
		Long: `向 forum 频道追加一个结构化 thread 事件。

动作（opened 外均须带 --thread）：
  opened     开新主题（--thread 不填时自动生成 thread-<时间戳>）
  comment    评论（正文用 --text、--stdin 或 --text-file 提供）
  status     改状态（--status）
  labels     替换标签（--labels，逗号分隔）
  assignees  替换负责人（--assignees，逗号分隔）
  summary    写摘要（--summary）
  processed  标记已处理（可带 --status 指定最终状态，默认 processed）

改名不用 post：用 sunshine channel thread rename <频道> <旧名> <新名>。`,
		Example: `  sunshine channel post board opened --as main --thread t1 --title "第一主题"
  sunshine channel post board comment --as alice --thread t1 --text "收到，正在看"
  sunshine channel post board status --as main --thread t1 --status in-progress`,
		Args: cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			name, action := args[0], args[1]
			if as == "" {
				return fmt.Errorf("post 命令须指定 --as（发帖方）")
			}
			parsed, err := channel.ParseThreadAction(action)
			if err != nil {
				return err
			}
			if parsed == "rename" {
				return fmt.Errorf("请用 sunshine channel thread rename <频道> <旧名> <新名>，不要用 post rename")
			}
			scope, err := parseScopeFlag(scope)
			if err != nil {
				return err
			}
			body, err := resolveTextBody(text, textFile, stdin)
			if err != nil {
				return err
			}
			context, err := channel.BuildContextEntries(
				append(append([]string{}, contextFiles...), linkedContextFiles...),
				append(append([]string{}, contextRaws...), linkedContextRaws...),
			)
			if err != nil {
				return err
			}
			ev, err := channel.PostThread(channelForumStore(), channel.PostThreadOptions{
				Channel: name, Scope: scope, By: as, Action: parsed, Thread: thread,
				Title: title, Text: body, Description: description, Status: status,
				Labels: channel.ParseCSV(labels), Assignees: channel.ParseCSV(assignees),
				Summary: summary, Context: context, Origin: "cli",
			})
			if err != nil {
				return err
			}
			return printEventJSON(ev)
		},
	}
	f := cmd.Flags()
	f.StringVar(&as, "as", "", "发帖方（必填）")
	f.StringVar(&scope, "scope", "", "频道作用域：project 或 global（不填自动解析）")
	f.StringVar(&thread, "thread", "", "thread 标识（opened 外必填）")
	f.StringVar(&title, "title", "", "主题标题")
	f.StringVar(&text, "text", "", "事件正文")
	f.BoolVar(&stdin, "stdin", false, "从标准输入读事件正文")
	f.StringVar(&textFile, "text-file", "", "从文件读事件正文")
	f.StringVar(&description, "description", "", "主题描述")
	f.StringVar(&status, "status", "", "主题状态")
	f.StringVar(&labels, "labels", "", "替换主题标签（逗号分隔）")
	f.StringVar(&assignees, "assignees", "", "替换负责人（逗号分隔）")
	f.StringVar(&summary, "summary", "", "主题摘要")
	f.StringArrayVar(&contextFiles, "context-file", nil, "附加的上下文文件（绝对路径，可重复）")
	f.StringArrayVar(&contextRaws, "context-raw", nil, "附加的上下文原始文本（可重复）")
	f.StringArrayVar(&linkedContextFiles, "linked-context-file", nil, "旧别名，等同 --context-file（可重复）")
	f.StringArrayVar(&linkedContextRaws, "linked-context-raw", nil, "旧别名，等同 --context-raw（可重复）")
	return cmd
}

func newChannelForumCommand() *cobra.Command {
	var scope, status string
	var raw bool
	cmd := &cobra.Command{
		Use:   "forum <频道>",
		Short: "列出 forum 频道的 thread",
		Long: `列出 forum 频道的全部 thread（按更新时间倒序）。

--status 按状态过滤；--raw 输出每条 thread 的原始 JSON（每行一条）。`,
		Example: `  sunshine channel forum board
  sunshine channel forum board --status open
  sunshine channel forum board --raw`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			scope, err := parseScopeFlag(scope)
			if err != nil {
				return err
			}
			states, err := channel.ListForumThreads(channelForumStore(), channel.ListForumThreadsOptions{
				Channel: args[0], Scope: scope,
			})
			if err != nil {
				return err
			}
			if status != "" {
				filtered := states[:0]
				for _, st := range states {
					if st.Status == status {
						filtered = append(filtered, st)
					}
				}
				states = filtered
			}
			if raw {
				for _, st := range states {
					data, err := json.Marshal(st)
					if err != nil {
						return fmt.Errorf("输出 thread 失败：%w", err)
					}
					fmt.Println(string(data))
				}
				return nil
			}
			for _, line := range channel.FormatThreadBoard(states) {
				fmt.Println(line)
			}
			return nil
		},
	}
	f := cmd.Flags()
	f.StringVar(&scope, "scope", "", "频道作用域：project 或 global（不填自动解析）")
	f.StringVar(&status, "status", "", "按状态过滤")
	f.BoolVar(&raw, "raw", false, "输出原始 thread JSON（每行一条）")
	return cmd
}

func newChannelThreadCommand() *cobra.Command {
	var scope string
	var raw bool
	threadCmd := &cobra.Command{
		Use:   "thread <频道> <主题>",
		Short: "查看单个 thread 的时间线",
		Long: `查看单个 thread 的事件时间线（按 seq 升序，含改名前旧名下的事件）。

--raw 输出每条事件的原始 JSON（每行一条）。
重命名 thread 用子命令：sunshine channel thread rename <频道> <旧名> <新名>。`,
		Example: `  sunshine channel thread board t1
  sunshine channel thread board t1 --raw
  sunshine channel thread rename board old-name new-name --as main`,
		Args: cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			scope, err := parseScopeFlag(scope)
			if err != nil {
				return err
			}
			events, err := channel.ShowThread(channelForumStore(), channel.ShowThreadOptions{
				Channel: args[0], Scope: scope, Thread: args[1],
			})
			if err != nil {
				return err
			}
			if raw {
				for _, ev := range events {
					if err := printEventJSON(ev); err != nil {
						return err
					}
				}
				return nil
			}
			if len(events) == 0 {
				return fmt.Errorf("在频道 %q 中找不到主题 %q", args[0], args[1])
			}
			state := channel.ReduceThreads(events)[0]
			fmt.Printf("%s [%s] %s\n", state.ID, state.Status, state.Title)
			if state.Description != "" {
				fmt.Printf("描述：%s\n", state.Description)
			}
			if len(state.Labels) > 0 {
				fmt.Printf("标签：%s\n", strings.Join(state.Labels, ","))
			}
			if len(state.Assignees) > 0 {
				fmt.Printf("负责人：%s\n", strings.Join(state.Assignees, ","))
			}
			if state.Summary != "" {
				fmt.Printf("摘要：%s\n", state.Summary)
			}
			for _, ev := range events {
				fmt.Println("  " + formatTimelineEvent(ev))
			}
			return nil
		},
	}
	f := threadCmd.Flags()
	f.StringVar(&scope, "scope", "", "频道作用域：project 或 global（不填自动解析）")
	f.BoolVar(&raw, "raw", false, "输出原始事件 JSON（每行一条）")

	var renameAs, renameScope string
	renameCmd := &cobra.Command{
		Use:   "rename <频道> <旧主题> <新主题>",
		Short: "重命名 thread",
		Long: `重命名一个 thread：追加一条改名事件，后续投影通过别名解析
把旧名展平到新名（用旧名仍能查到该 thread 的时间线）。

旧名不存在、或新名已是另一条 thread 时拒绝改名。`,
		Example: `  sunshine channel thread rename board old-name new-name --as main`,
		Args:    cobra.ExactArgs(3),
		RunE: func(cmd *cobra.Command, args []string) error {
			if renameAs == "" {
				return fmt.Errorf("rename 命令须指定 --as（操作方）")
			}
			scope, err := parseScopeFlag(renameScope)
			if err != nil {
				return err
			}
			ev, err := channel.RenameThread(channelForumStore(), channel.RenameThreadOptions{
				Channel: args[0], Scope: scope, By: renameAs,
				Thread: args[1], NewThread: args[2], Origin: "cli",
			})
			if err != nil {
				return err
			}
			return printEventJSON(ev)
		},
	}
	rf := renameCmd.Flags()
	rf.StringVar(&renameAs, "as", "", "操作方（必填）")
	rf.StringVar(&renameScope, "scope", "", "频道作用域：project 或 global（不填自动解析）")
	threadCmd.AddCommand(renameCmd)

	return threadCmd
}

// formatTimelineEvent 把一条时间线事件格式化为一行：
// “2026-11-09 10:00:00 comment 发帖人=main 正文”。
func formatTimelineEvent(ev channel.ChannelEvent) string {
	ts := ev.TS
	if len(ts) >= 19 {
		ts = ts[:19]
	}
	ts = strings.Replace(ts, "T", " ", 1)
	if ev.Kind == "thread" {
		action := ev.Action
		if action == "" {
			action = "?"
		}
		text := ""
		if ev.Text != "" {
			text = " " + ev.Text
		}
		return fmt.Sprintf("%s %s 发帖人=%s%s", ts, action, ev.By, text)
	}
	action := ev.Action
	if action == "" {
		action = "?"
	}
	return fmt.Sprintf("%s context-%s 发帖人=%s", ts, action, ev.By)
}
