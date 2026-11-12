package cli

import (
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/BondLam/Sunshine/internal/task"
	"github.com/spf13/cobra"
)

// taskCmd 是 `sunshine task <子命令>`：任务系统（对应 Trellis 的 task.py）。
var taskCmd = &cobra.Command{
	Use:   "task",
	Short: "任务管理：创建、启动、完成、归档任务",
	Long: `任务管理：创建、启动、完成、归档任务（对应 Trellis 的 task.py）。

任务目录：.sunshine/tasks/<MM-DD-slug>/，归档到
.sunshine/tasks/archive/<YYYY-MM>/。当前任务记录在
.sunshine/.current-task，同时写进会话文件
.sunshine/.runtime/sessions/<context_key>.json。`,
	Example: `  sunshine task create "给退款原因加必填校验" --description "…" 
  sunshine task list
  sunshine task start 11-09-退款原因必填
  sunshine task finish
  sunshine task context --json`,
}

func newTaskRunner(cmd *cobra.Command) *task.Runner {
	cwd, err := os.Getwd()
	if err != nil {
		fmt.Fprintf(cmd.ErrOrStderr(), "错误：无法获取当前目录：%v\n", err)
		os.Exit(1)
	}
	return task.NewRunner(cwd, cmd.OutOrStdout(), cmd.ErrOrStderr())
}

// runTaskCode 以 Runner 的返回码退出进程。
func runTaskCode(cmd *cobra.Command, fn func(r *task.Runner) int) {
	os.Exit(fn(newTaskRunner(cmd)))
}

func init() {
	rootCmd.AddCommand(taskCmd)
	taskCmd.AddCommand(taskCreateCmd)
	taskCmd.AddCommand(taskAddContextCmd)
	taskCmd.AddCommand(taskValidateCmd)
	taskCmd.AddCommand(taskListContextCmd)
	taskCmd.AddCommand(taskStartCmd)
	taskCmd.AddCommand(taskCurrentCmd)
	taskCmd.AddCommand(taskFinishCmd)
	taskCmd.AddCommand(taskSetBranchCmd)
	taskCmd.AddCommand(taskSetBaseBranchCmd)
	taskCmd.AddCommand(taskSetScopeCmd)
	taskCmd.AddCommand(taskSetMetaCmd)
	taskCmd.AddCommand(taskRenameCmd)
	taskCmd.AddCommand(taskArchiveCmd)
	taskCmd.AddCommand(taskListCmd)
	taskCmd.AddCommand(taskAddSubtaskCmd)
	taskCmd.AddCommand(taskRemoveSubtaskCmd)
	taskCmd.AddCommand(taskListArchiveCmd)
	taskCmd.AddCommand(taskContextCmd)
	taskCmd.AddCommand(taskAddSessionCmd)
}

var taskCreateCmd = &cobra.Command{
	Use:   "create <标题>",
	Short: "创建新任务",
	Long: `创建新任务：在 .sunshine/tasks/<MM-DD-slug>/ 下写 task.json 与 prd.md，
默认自动激活（写 .current-task 与会话文件）。

--slug 为空时按标题生成；--description 必填；--meta 可重复传 key=value。
同名任务已存在时拒绝（--force 覆盖）；同名已归档时拒绝。`,
	Example: `  sunshine task create "给退款原因加必填校验" --description "退款时原因字段必填" --priority P1
  sunshine task create "接入新支付渠道" -d "支持微信支付" --slug xin-zhifu --package api`,
	Args: cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		slug, _ := cmd.Flags().GetString("slug")
		assignee, _ := cmd.Flags().GetString("assignee")
		priority, _ := cmd.Flags().GetString("priority")
		description, _ := cmd.Flags().GetString("description")
		parent, _ := cmd.Flags().GetString("parent")
		pkg, _ := cmd.Flags().GetString("package")
		baseBranch, _ := cmd.Flags().GetString("base-branch")
		meta, _ := cmd.Flags().GetStringArray("meta")
		noStart, _ := cmd.Flags().GetBool("no-start")
		force, _ := cmd.Flags().GetBool("force")
		runTaskCode(cmd, func(r *task.Runner) int {
			return r.Create(task.CreateOptions{
				Title: args[0], Slug: slug, Assignee: assignee,
				Priority: priority, Description: description,
				Parent: parent, Package: pkg, BaseBranch: baseBranch,
				Meta: meta, NoStart: noStart, Force: force,
			})
		})
	},
}

var taskAddContextCmd = &cobra.Command{
	Use:   "add-context <任务> <文件> <路径> [原因]",
	Short: "给任务的上下文清单加一条记录",
	Long: `给任务的上下文清单（implement.jsonl / check.jsonl）加一条记录。

<文件> 可简写 implement 或 check（自动补 .jsonl）；<路径> 为目录时
记为目录型条目；<路径> 不存在时拒绝；已存在相同路径的条目时警告
并返回 0。`,
	Example: `  sunshine task add-context 11-09-退款原因必填 implement src/Refund.vue
  sunshine task add-context 11-09-退款原因必填 check docs/api.md "字段定义"`,
	Args: cobra.RangeArgs(3, 4),
	Run: func(cmd *cobra.Command, args []string) {
		reason := ""
		if len(args) == 4 {
			reason = args[3]
		}
		runTaskCode(cmd, func(r *task.Runner) int {
			return r.AddContext(args[0], args[1], args[2], reason)
		})
	},
}

var taskValidateCmd = &cobra.Command{
	Use:   "validate <任务>",
	Short: "校验任务的上下文清单",
	Long: `校验任务的 implement.jsonl / check.jsonl：条目是否指向真实存在
的文件/目录、JSON 是否有效、是否超出注入上限。

占位行（_example）是硬错误；代码文件在规范目录、docs/ 与任务自身
目录之外时只警告；0 条已整理条目时该文件计 1 个错误。`,
	Example: `  sunshine task validate 11-09-退款原因必填`,
	Args:    cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		runTaskCode(cmd, func(r *task.Runner) int { return r.Validate(args[0]) })
	},
}

var taskListContextCmd = &cobra.Command{
	Use:     "list-context <任务>",
	Short:   "列出任务的上下文清单",
	Long:    `列出任务 implement.jsonl / check.jsonl 里带 file 的已整理条目，目录型条目标注 [目录]。`,
	Example: `  sunshine task list-context 11-09-退款原因必填`,
	Args:    cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		runTaskCode(cmd, func(r *task.Runner) int { return r.ListContext(args[0]) })
	},
}

var taskStartCmd = &cobra.Command{
	Use:   "start <任务>",
	Short: "开始一个任务",
	Long: `开始一个任务：状态 planning → in_progress，记当前检出分支，
写 .current-task 与会话文件（.sunshine/.runtime/sessions/<context_key>.json）。

上下文清单门禁：implement.jsonl / check.jsonl 已存在但 0 条已整理
条目时拒绝，除非传 --allow-empty-context；文件不存在时不设防。`,
	Example: `  sunshine task start 11-09-退款原因必填`,
	Args:    cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		allowEmpty, _ := cmd.Flags().GetBool("allow-empty-context")
		runTaskCode(cmd, func(r *task.Runner) int { return r.Start(args[0], allowEmpty) })
	},
}

var taskCurrentCmd = &cobra.Command{
	Use:   "current",
	Short: "显示当前任务",
	Long: `显示当前任务（状态、分支、父子关系）。

--source 显示任务来源（会话文件 / .current-task / 唯一会话回退）；
--json 输出 JSON（含 current_task、source、stale；读失败时加 error）。`,
	Example: `  sunshine task current
  sunshine task current --json`,
	Args: cobra.NoArgs,
	Run: func(cmd *cobra.Command, args []string) {
		source, _ := cmd.Flags().GetBool("source")
		asJSON, _ := cmd.Flags().GetBool("json")
		runTaskCode(cmd, func(r *task.Runner) int { return r.Current(source, asJSON) })
	},
}

var taskFinishCmd = &cobra.Command{
	Use:   "finish",
	Short: "完成当前任务",
	Long: `完成当前任务：清除当前任务指针（删会话文件、清空 .current-task），
并跑 after_finish hooks。任务本身的状态不由 finish 改，归档时才置 completed。`,
	Example: `  sunshine task finish`,
	Args:    cobra.NoArgs,
	Run: func(cmd *cobra.Command, args []string) {
		runTaskCode(cmd, func(r *task.Runner) int { return r.Finish() })
	},
}

var taskSetBranchCmd = &cobra.Command{
	Use:     "set-branch <任务> <分支>",
	Short:   "设置任务的分支",
	Long:    `只改 task.json 的 branch 一键（原子写回，不重校验 24 字段）。`,
	Example: `  sunshine task set-branch 11-09-退款原因必填 feature/refund-reason`,
	Args:    cobra.ExactArgs(2),
	Run: func(cmd *cobra.Command, args []string) {
		runTaskCode(cmd, func(r *task.Runner) int { return r.SetBranch(args[0], args[1]) })
	},
}

var taskSetBaseBranchCmd = &cobra.Command{
	Use:     "set-base-branch <任务> <分支>",
	Short:   "设置任务的基准分支",
	Long:    `只改 task.json 的 base_branch 一键（原子写回，不重校验 24 字段）。`,
	Example: `  sunshine task set-base-branch 11-09-退款原因必填 main`,
	Args:    cobra.ExactArgs(2),
	Run: func(cmd *cobra.Command, args []string) {
		runTaskCode(cmd, func(r *task.Runner) int { return r.SetBaseBranch(args[0], args[1]) })
	},
}

var taskSetScopeCmd = &cobra.Command{
	Use:     "set-scope <任务> <范围>",
	Short:   "设置任务的范围",
	Long:    `只改 task.json 的 scope 一键（原子写回，不重校验 24 字段）。`,
	Example: `  sunshine task set-scope 11-09-退款原因必填 "只改 Refund.vue 三处"`,
	Args:    cobra.ExactArgs(2),
	Run: func(cmd *cobra.Command, args []string) {
		runTaskCode(cmd, func(r *task.Runner) int { return r.SetScope(args[0], args[1]) })
	},
}

var taskSetMetaCmd = &cobra.Command{
	Use:   "set-meta <任务> <键> <值>",
	Short: "设置任务的一条元数据",
	Long: `只改 task.json 的 meta.<键> 一键（原子写回，不重校验 24 字段；
meta 非对象时重建为 {}，值存字符串）。`,
	Example: `  sunshine task set-meta 11-09-退款原因必填 reviewer zhangsan`,
	Args:    cobra.ExactArgs(3),
	Run: func(cmd *cobra.Command, args []string) {
		runTaskCode(cmd, func(r *task.Runner) int { return r.SetMeta(args[0], args[1], args[2]) })
	},
}

var taskRenameCmd = &cobra.Command{
	Use:   "rename <任务> <新 slug>",
	Short: "重命名任务",
	Long: `重命名活动任务：改目录名、task.json 的 id/name、本任务 jsonl 里的
旧相对路径引用、其他任务的 parent/children/subtasks 反向引用，
会话指针改指新路径。

保留原创建日期前缀；--dry-run 只输出改动计划不执行。`,
	Example: `  sunshine task rename 11-09-旧名字 11-09-新名字 --dry-run
  sunshine task rename 11-09-旧名字 11-09-新名字`,
	Args: cobra.ExactArgs(2),
	Run: func(cmd *cobra.Command, args []string) {
		dryRun, _ := cmd.Flags().GetBool("dry-run")
		runTaskCode(cmd, func(r *task.Runner) int { return r.Rename(args[0], args[1], dryRun) })
	},
}

var taskArchiveCmd = &cobra.Command{
	Use:   "archive <任务>",
	Short: "归档任务",
	Long: `归档任务：状态置 completed、completedAt 记当天，移入
.sunshine/tasks/archive/<YYYY-MM>/，默认自动提交
（chore(task): archive <名>）。

分支校验：branch 已不存在只警告；branch == base_branch 拒绝；
无 branch 但有 base_branch 且仓库有 remote 时拒绝
（--skip-branch-validation 跳过）。归档父任务时清除子女的 parent。`,
	Example: `  sunshine task archive 11-09-退款原因必填
  sunshine task archive 11-09-退款原因必填 --no-commit`,
	Args: cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		noCommit, _ := cmd.Flags().GetBool("no-commit")
		skipValidation, _ := cmd.Flags().GetBool("skip-branch-validation")
		runTaskCode(cmd, func(r *task.Runner) int { return r.Archive(args[0], noCommit, skipValidation) })
	},
}

var taskListCmd = &cobra.Command{
	Use:   "list",
	Short: "列出活动任务",
	Long: `按父子层级列出活动任务。

--mine 只列当前开发者的任务；--status 按状态过滤；--json 输出 JSON。
planning 父任务有非 planning 子女时显示 active；子女进度显示 [n/m done]。`,
	Example: `  sunshine task list
  sunshine task list --mine
  sunshine task list --status in_progress --json`,
	Args: cobra.NoArgs,
	Run: func(cmd *cobra.Command, args []string) {
		mine, _ := cmd.Flags().GetBool("mine")
		status, _ := cmd.Flags().GetString("status")
		asJSON, _ := cmd.Flags().GetBool("json")
		runTaskCode(cmd, func(r *task.Runner) int { return r.List(mine, status, asJSON) })
	},
}

var taskAddSubtaskCmd = &cobra.Command{
	Use:   "add-subtask <父任务> <子任务>",
	Short: "给任务加一个子任务",
	Long: `给父任务加一个子任务：双向维护父任务的 children 与子任务的 parent。
子任务已有父任务时拒绝；两边写失败都报半写状态。`,
	Example: `  sunshine task add-subtask 11-09-退款原因必填 11-09-前端校验`,
	Args:    cobra.ExactArgs(2),
	Run: func(cmd *cobra.Command, args []string) {
		runTaskCode(cmd, func(r *task.Runner) int { return r.AddSubtask(args[0], args[1]) })
	},
}

var taskRemoveSubtaskCmd = &cobra.Command{
	Use:   "remove-subtask <父任务> <子任务>",
	Short: "解除任务的父子链接",
	Long: `解除父子链接：同时清父任务的 children 与子任务的 parent。
两边写失败都报半写状态。`,
	Example: `  sunshine task remove-subtask 11-09-退款原因必填 11-09-前端校验`,
	Args:    cobra.ExactArgs(2),
	Run: func(cmd *cobra.Command, args []string) {
		runTaskCode(cmd, func(r *task.Runner) int { return r.RemoveSubtask(args[0], args[1]) })
	},
}

var taskListArchiveCmd = &cobra.Command{
	Use:   "list-archive [YYYY-MM]",
	Short: "列出已归档任务",
	Long:  `列出已归档任务；传 YYYY-MM 时只列该月归档的任务。`,
	Example: `  sunshine task list-archive
  sunshine task list-archive 2026-11`,
	Args: cobra.MaximumNArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		month := ""
		if len(args) == 1 {
			month = args[0]
		}
		runTaskCode(cmd, func(r *task.Runner) int { return r.ListArchive(month) })
	},
}

var taskContextCmd = &cobra.Command{
	Use:   "context",
	Short: "输出当前工作上下文（供 AI 助手）",
	Long: `输出当前工作上下文：开发者、Git 状态、最近提交、当前任务、
活动任务树、我的任务、日志文件、包、路径（对应 get_context.py）。

--mode 可选 default / record / packages / phase（默认 default）；
phase 模式用 --step 取 workflow.md 的某一步，--platform 按平台
标记过滤步骤内容；--json 输出 JSON。`,
	Example: `  sunshine task context
  sunshine task context --json
  sunshine task context --mode phase --step 3.1`,
	Args: cobra.NoArgs,
	Run: func(cmd *cobra.Command, args []string) {
		mode, _ := cmd.Flags().GetString("mode")
		step, _ := cmd.Flags().GetString("step")
		platform, _ := cmd.Flags().GetString("platform")
		asJSON, _ := cmd.Flags().GetBool("json")
		runTaskCode(cmd, func(r *task.Runner) int { return r.Context(mode, step, platform, asJSON) })
	},
}

var taskAddSessionCmd = &cobra.Command{
	Use:   "add-session",
	Short: "把一次会话记录进开发者日志",
	Long: `把一次会话记录进开发者日志（对应 add_session.py）：
往 .sunshine/workspace/<开发者>/journal-N.md 追加一条会话记录，
更新 index.md 的自动区块，默认自动提交。

每条记录带指纹标记（<!-- sunshine-session: v=2 fp=… -->），
重跑同一命令会从断点续做，不会追加第二条；传 --idempotency-key
时已提交的记录直接返回 0。

--commit 的 OID 先在本地 git 对象库解析：解析失败则不写任何文件，
或用 --commit-subject <oid>=<主题> 显式给主题。`,
	Example: `  sunshine task add-session --title "退款原因必填" --commit abc1234 --summary "完成了 Refund.vue 三处修改"
  sunshine task add-session --title "规划会话" --commit - --summary "只做了方案设计"`,
	Args: cobra.NoArgs,
	Run: func(cmd *cobra.Command, args []string) {
		title, _ := cmd.Flags().GetString("title")
		commit, _ := cmd.Flags().GetString("commit")
		summary, _ := cmd.Flags().GetString("summary")
		branch, _ := cmd.Flags().GetString("branch")
		pkg, _ := cmd.Flags().GetString("package")
		subjects, _ := cmd.Flags().GetStringArray("commit-subject")
		changes, _ := cmd.Flags().GetStringArray("change")
		tests, _ := cmd.Flags().GetStringArray("test")
		nextSteps, _ := cmd.Flags().GetStringArray("next-step")
		idemKey, _ := cmd.Flags().GetString("idempotency-key")
		noCommit, _ := cmd.Flags().GetBool("no-commit")
		useStdin, _ := cmd.Flags().GetBool("stdin")
		contentFile, _ := cmd.Flags().GetString("content-file")
		if strings.TrimSpace(title) == "" {
			fmt.Fprintln(cmd.ErrOrStderr(), "错误：--title 必填")
			os.Exit(1)
		}
		extra := ""
		if contentFile != "" {
			if data, err := os.ReadFile(contentFile); err == nil {
				extra = string(data)
			}
		} else if useStdin {
			if data, err := io.ReadAll(os.Stdin); err == nil {
				extra = string(data)
			}
		}
		runTaskCode(cmd, func(r *task.Runner) int {
			return r.AddSession(task.AddSessionOptions{
				Title: title, Commit: commit, Summary: summary,
				Branch: branch, Package: pkg, CommitSubjects: subjects,
				Changes: changes, Tests: tests, NextSteps: nextSteps,
				ExtraContent: extra, IdempotencyKey: idemKey, NoCommit: noCommit,
			})
		})
	},
}

func init() {
	taskCreateCmd.Flags().StringP("slug", "s", "", "任务 slug（默认按标题生成）")
	taskCreateCmd.Flags().StringP("assignee", "a", "", "负责人（默认当前开发者）")
	taskCreateCmd.Flags().StringP("priority", "p", "P2", "优先级（P0/P1/P2/P3）")
	taskCreateCmd.Flags().StringP("description", "d", "", "任务描述（必填）")
	taskCreateCmd.Flags().String("parent", "", "父任务名")
	taskCreateCmd.Flags().String("package", "", "所属包（monorepo）")
	taskCreateCmd.Flags().String("base-branch", "", "基准分支（默认仓库默认分支）")
	taskCreateCmd.Flags().StringArray("meta", nil, "元数据 key=value（可重复）")
	taskCreateCmd.Flags().Bool("no-start", false, "创建后不自动激活")
	taskCreateCmd.Flags().Bool("force", false, "同名任务已存在时覆盖")

	taskStartCmd.Flags().Bool("allow-empty-context", false, "上下文清单为空时也允许开始")

	taskCurrentCmd.Flags().Bool("source", false, "显示当前任务的来源")
	taskCurrentCmd.Flags().BoolP("json", "j", false, "以 JSON 输出")

	taskListCmd.Flags().BoolP("mine", "m", false, "只列当前开发者的任务")
	taskListCmd.Flags().StringP("status", "s", "", "按状态过滤")
	taskListCmd.Flags().Bool("json", false, "以 JSON 输出")

	taskRenameCmd.Flags().Bool("dry-run", false, "只输出改动计划，不执行")

	taskArchiveCmd.Flags().Bool("no-commit", false, "归档后不自动提交")
	taskArchiveCmd.Flags().Bool("skip-branch-validation", false, "跳过分支校验")

	taskContextCmd.Flags().StringP("mode", "m", "default", "输出模式：default / record / packages / phase")
	taskContextCmd.Flags().String("step", "", "phase 模式：workflow.md 的步骤编号")
	taskContextCmd.Flags().String("platform", "", "phase 模式：按平台标记过滤")
	taskContextCmd.Flags().BoolP("json", "j", false, "以 JSON 输出")

	taskAddSessionCmd.Flags().String("title", "", "会话标题（必填）")
	taskAddSessionCmd.Flags().String("commit", "-", "逗号分隔的提交 OID，或 - 表示无提交")
	taskAddSessionCmd.Flags().String("summary", "", "会话摘要（默认：未提供会话摘要。）")
	taskAddSessionCmd.Flags().String("branch", "", "分支（默认按任务分支 → 当前分支推断）")
	taskAddSessionCmd.Flags().String("package", "", "所属包（monorepo）")
	taskAddSessionCmd.Flags().StringArray("commit-subject", nil, "提交主题 <oid>=<主题>（可重复）")
	taskAddSessionCmd.Flags().StringArray("change", nil, "主要变更（可重复）")
	taskAddSessionCmd.Flags().StringArray("test", nil, "测试情况（可重复）")
	taskAddSessionCmd.Flags().StringArray("next-step", nil, "下一步（可重复）")
	taskAddSessionCmd.Flags().String("idempotency-key", "", "幂等键（1-64 个 [A-Za-z0-9._-] 字符）")
	taskAddSessionCmd.Flags().Bool("no-commit", false, "不自动提交 journal/index")
	taskAddSessionCmd.Flags().Bool("stdin", false, "从 stdin 读自由内容")
	taskAddSessionCmd.Flags().String("content-file", "", "从文件读自由内容")
}
