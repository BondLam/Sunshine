package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/AlecAivazis/survey/v2"
	"github.com/linrong1982/Sunshine/internal/workflow"
	"github.com/spf13/cobra"
)

var (
	workflowTemplate    string
	workflowMarketplace string
	workflowList        bool
	workflowForce       bool
	workflowCreateNew   bool
)

// promptWorkflowTemplate 弹出工作流模板单选（默认实现用 survey；测试可替换）。
var promptWorkflowTemplate = func(templates []workflow.Template) (string, error) {
	var options []string
	for _, t := range templates {
		tag := "（内置）"
		if t.Source == workflow.SourceMarketplace {
			tag = "（marketplace）"
		}
		options = append(options, fmt.Sprintf("%s — %s %s", t.ID, t.Name, tag))
	}
	var chosen string
	err := survey.AskOne(&survey.Select{
		Message: "请选择工作流模板：",
		Options: options,
	}, &chosen)
	if err != nil {
		return "", err
	}
	// 选项形如 "<id> — <名称> （内置）"，取 " — " 之前的 id。
	if idx := strings.Index(chosen, " — "); idx != -1 {
		return chosen[:idx], nil
	}
	return chosen, nil
}

// promptOverwriteAction 在现有 workflow.md 已改动时弹出确认
// （默认实现用 survey；测试可替换）。
var promptOverwriteAction = func() (workflow.OverwriteAction, error) {
	var action string
	err := survey.AskOne(&survey.Select{
		Message: "你的 .sunshine/workflow.md 已有本地修改，要怎么处理？",
		Options: []string{
			"覆盖（替换本地修改）",
			"写到 .sunshine/workflow.md.new，保留现有文件",
			"跳过（不做任何改动）",
		},
	}, &action)
	if err != nil {
		return workflow.ActionSkip, err
	}
	switch {
	case strings.HasPrefix(action, "覆盖"):
		return workflow.ActionOverwrite, nil
	case strings.HasPrefix(action, "写到"):
		return workflow.ActionCreateNew, nil
	default:
		return workflow.ActionSkip, nil
	}
}

var workflowCmd = &cobra.Command{
	Use:   "workflow",
	Short: "查看或切换 .sunshine/workflow.md 的工作流模板",
	Long: `查看或切换 .sunshine/workflow.md 使用的工作流模板。

内置模板（离线可用）：
  native                          原生 Sunshine 工作流（默认）
  tdd                             TDD 工作流
  channel-driven-subagent-dispatch  Channel 驱动的子代理派发

覆盖规则按现有 workflow.md 的哈希状态决定：
  未改动 → 直接覆盖；已改动或来源未知 → 交互确认
  （覆盖 / 写 .sunshine/workflow.md.new / 跳过）。

用 -m/--marketplace 可从模板市场拉取其他工作流模板
（来源格式：gh:user/repo、https://github.com/user/repo、
git@github.com:user/repo 等；不支持本地路径）。

示例：
  sunshine workflow --list
  sunshine workflow --template tdd
  sunshine workflow --template tdd --force
  sunshine workflow --template native --create-new
  sunshine workflow --list --marketplace gh:myorg/marketplace`,
	RunE: func(cmd *cobra.Command, args []string) error {
		cwd, err := os.Getwd()
		if err != nil {
			return err
		}
		if !dirExists(filepath.Join(cwd, ".sunshine")) {
			return fmt.Errorf("未找到 .sunshine/ 目录，请先运行 sunshine init")
		}

		// --list：只列模板，不写盘。
		if workflowList {
			templates, errMsg := workflow.ListTemplates(workflowMarketplace)
			printWorkflowListing(templates)
			if errMsg != "" {
				fmt.Fprintf(os.Stderr, "⚠ %s\n", errMsg)
			}
			return nil
		}

		// 确定模板 id：未传 --template 时交互选择。
		templateID := workflowTemplate
		if templateID == "" {
			if !stdinIsTTY() {
				return fmt.Errorf("未指定 --template 且不在交互终端，请传 --template <id> 或在交互终端运行")
			}
			templates, errMsg := workflow.ListTemplates(workflowMarketplace)
			if errMsg != "" {
				fmt.Fprintf(os.Stderr, "⚠ %s\n", errMsg)
			}
			if len(templates) == 0 {
				return fmt.Errorf("没有可用的工作流模板")
			}
			picked, err := promptWorkflowTemplate(templates)
			if err != nil {
				return fmt.Errorf("选择模板失败：%w", err)
			}
			templateID = picked
		}

		tmpl, err := workflow.Resolve(templateID, workflowMarketplace)
		if err != nil {
			return err
		}

		if err := workflow.Apply(cwd, tmpl, workflow.ApplyOptions{
			Force:       workflowForce,
			CreateNew:   workflowCreateNew,
			Interactive: stdinIsTTY(),
			Confirm:     promptOverwriteAction,
			Out:         cmd.OutOrStdout(),
		}); err != nil {
			return err
		}

		// 尽力而为的警告：模板引用的 agent 定义不在磁盘上时提醒用户，
		// 以免之后 channel spawn --agent 在会话中途失败。
		// 只警告，不影响已成功的写入。
		if missing := workflow.CollectMissingAgents(cwd, tmpl.Content); len(missing) > 0 {
			fmt.Fprint(os.Stderr, workflow.FormatMissingAgentsWarning(missing))
		}
		return nil
	},
}

// printWorkflowListing 打印模板列表（中文说明，内置在前）。
func printWorkflowListing(templates []workflow.Template) {
	fmt.Println("\n可用的工作流模板：")
	fmt.Println()
	for _, t := range templates {
		tag := "（内置）"
		if t.Source == workflow.SourceMarketplace {
			tag = "（marketplace）"
		}
		fmt.Printf("  %s%s — %s\n", t.ID, tag, t.Name)
		if t.Description != "" {
			fmt.Printf("    %s\n", t.Description)
		}
	}
	fmt.Println()
}

func init() {
	workflowCmd.Flags().StringVarP(&workflowTemplate, "template", "t", "", "工作流模板 id（native、tdd、channel-driven-subagent-dispatch）")
	workflowCmd.Flags().StringVarP(&workflowMarketplace, "marketplace", "m", "", "模板市场来源（gh:user/repo、https://github.com/user/repo 等）")
	workflowCmd.Flags().BoolVar(&workflowList, "list", false, "列出可用的工作流模板")
	workflowCmd.Flags().BoolVarP(&workflowForce, "force", "f", false, "已有本地修改时直接覆盖，不确认")
	workflowCmd.Flags().BoolVarP(&workflowCreateNew, "create-new", "n", false, "只写 .sunshine/workflow.md.new，不动 workflow.md")
	rootCmd.AddCommand(workflowCmd)
}
