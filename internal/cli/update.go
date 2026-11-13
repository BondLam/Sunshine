package cli

import (
	"fmt"
	"os"

	"github.com/AlecAivazis/survey/v2"
	"github.com/linrong007/Sunshine/internal/update"
	"github.com/spf13/cobra"
)

var (
	updateDryRun         bool
	updateForce          bool
	updateSkipAll        bool
	updateCreateNew      bool
	updateAllowDowngrade bool
	updateMigrate        bool
)

// surveyPrompter 用 survey 实现 update.Prompter。
// 非交互环境（stdin 不是终端）时不提问：
// 统一确认直接通过，冲突文件默认跳过（安全）。
type surveyPrompter struct{}

func (surveyPrompter) ConfirmProceed() (bool, error) {
	if !stdinIsTTY() {
		fmt.Println("非交互环境，跳过统一确认（冲突文件默认跳过）。")
		return true, nil
	}
	var proceed bool
	err := survey.AskOne(&survey.Confirm{
		Message: "确认更新吗？",
		Default: true,
	}, &proceed)
	return proceed, err
}

func (surveyPrompter) ResolveConflict(path string) (update.ConflictAction, error) {
	if !stdinIsTTY() {
		return update.ActionSkip, nil
	}
	options := []string{
		"覆盖——用新版替换（你的修改会丢失）",
		"跳过——保留当前版本",
		"创建副本——新版存为 .new 文件",
		"全部覆盖——剩余文件都用新版替换",
		"全部跳过——剩余文件都保留当前版本",
		"全部创建副本——剩余文件的新版都存为 .new",
	}
	var choice int
	err := survey.AskOne(&survey.Select{
		Message: fmt.Sprintf("%s 已被你修改，如何处理？", path),
		Options: options,
		Default: options[1],
	}, &choice)
	if err != nil {
		return "", err
	}
	switch choice {
	case 0:
		return update.ActionOverwrite, nil
	case 1:
		return update.ActionSkip, nil
	case 2:
		return update.ActionCreateNew, nil
	case 3:
		return update.ActionOverwriteAll, nil
	case 4:
		return update.ActionSkipAll, nil
	default:
		return update.ActionCreateNewAll, nil
	}
}

// RegisterUpdateCommand 注册 sunshine update 命令。
// 由总协调人统一接线（不要改 internal/cli/root.go）。
func RegisterUpdateCommand(root *cobra.Command) {
	cmd := &cobra.Command{
		Use:   "update",
		Short: "更新项目内的 Sunshine 模板文件",
		Long: `把项目内的 Sunshine 模板文件同步到当前 CLI 版本。

按 .sunshine/.template-hashes.json 区分三态：
  - 未改动：自动更新；
  - 你已改动：逐个确认（覆盖 / 跳过 / 创建 .new 副本，默认跳过）；
  - 新增：直接添加；你已删除的文件保持删除，不恢复。

更新前在 .sunshine/.backups/<时间戳>/ 做完整备份；
成功后更新 .sunshine/.version。

示例：
  sunshine update --dry-run
  sunshine update --force
  sunshine update --migrate`,
		RunE: func(cmd *cobra.Command, args []string) error {
			cwd, err := os.Getwd()
			if err != nil {
				return err
			}
			_, err = update.Run(cwd, update.Options{
				CLIVersion:     version,
				DryRun:         updateDryRun,
				Force:          updateForce,
				SkipAll:        updateSkipAll,
				CreateNew:      updateCreateNew,
				AllowDowngrade: updateAllowDowngrade,
				Migrate:        updateMigrate,
				Prompter:       surveyPrompter{},
			})
			return err
		},
	}
	cmd.Flags().BoolVar(&updateDryRun, "dry-run", false, "只打印更新计划，不写盘")
	cmd.Flags().BoolVarP(&updateForce, "force", "f", false, "冲突文件一律覆盖")
	cmd.Flags().BoolVarP(&updateSkipAll, "skip-all", "s", false, "冲突文件一律跳过")
	cmd.Flags().BoolVarP(&updateCreateNew, "create-new", "n", false, "冲突文件的新版一律写为 .new 副本")
	cmd.Flags().BoolVar(&updateAllowDowngrade, "allow-downgrade", false, "允许 CLI 比项目旧时仍执行（降级）")
	cmd.Flags().BoolVar(&updateMigrate, "migrate", false, "执行改名/删除类迁移")
	root.AddCommand(cmd)
}
