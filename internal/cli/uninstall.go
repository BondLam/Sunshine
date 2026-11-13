package cli

import (
	"os"

	"github.com/AlecAivazis/survey/v2"
	"github.com/linrong007/Sunshine/internal/lifecycle"
	"github.com/spf13/cobra"
)

// promptConfirm 弹出“继续？[Y/n]”确认（默认 true），
// 供 uninstall / ablate / restore 三个命令共用；
// 测试可替换为替身。
var promptConfirm = func(message string) (bool, error) {
	var proceed bool
	err := survey.AskOne(&survey.Confirm{
		Message: message,
		Default: true,
	}, &proceed)
	return proceed, err
}

// RegisterUninstallCommand 把 `sunshine uninstall` 接到根命令上。
// 由总协调人统一接线（不要改 root.go，也不要用 init() 自注册）。
func RegisterUninstallCommand(root *cobra.Command) {
	var yes, dryRun bool
	cmd := &cobra.Command{
		Use:   "uninstall",
		Short: "删除项目内全部 Sunshine 文件",
		Long: `删除项目内全部 Sunshine 文件：按 .sunshine/.template-hashes.json
列出的托管内容删除平台配置文件（只删清单列出的内容，
清单外的文件永不删），再把整个 .sunshine/ 目录一并移除。

清单缺失或为空时拒绝执行（无法确定删哪些平台文件）。

有未提交的用户数据（.sunshine/spec、tasks、workspace）时先警告；
--yes 下有未提交数据时拒绝，须先提交/暂存，或交互确认，
或设置 SUNSHINE_ALLOW_DIRTY_UNINSTALL=1 显式绕过。

示例：
  sunshine uninstall --dry-run
  sunshine uninstall -y`,
		RunE: func(cmd *cobra.Command, args []string) error {
			cwd, err := os.Getwd()
			if err != nil {
				return err
			}
			return lifecycle.Uninstall(cwd, lifecycle.UninstallOptions{
				Yes:     yes,
				DryRun:  dryRun,
				Confirm: promptConfirm,
			})
		},
	}
	cmd.Flags().BoolVarP(&yes, "yes", "y", false, "跳过确认，直接卸载")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "只打印卸载计划，不修改任何文件")
	root.AddCommand(cmd)
}
