package cli

import (
	"os"

	"github.com/linrong1982/Sunshine/internal/lifecycle"
	"github.com/spf13/cobra"
)

// RegisterRestoreCommand 把 `sunshine restore` 接到根命令上。
// 由总协调人统一接线（不要改 root.go，也不要用 init() 自注册）。
func RegisterRestoreCommand(root *cobra.Command) {
	var yes, dryRun bool
	cmd := &cobra.Command{
		Use:   "restore",
		Short: "精确恢复 ablate 保存的 Sunshine 状态",
		Long: `精确恢复 ablate 保存的 Sunshine 状态：校验事务版本与创建时间，
按事务 state.json 恢复全部条目；恢复成功后删除外部事务目录。

切除期间被改动的托管路径会拒绝写入（恢复前先做冲突预检）。
--dry-run 只做冲突预检，不恢复、不删事务。

示例：
  sunshine restore --dry-run
  sunshine restore -y`,
		RunE: func(cmd *cobra.Command, args []string) error {
			cwd, err := os.Getwd()
			if err != nil {
				return err
			}
			return lifecycle.Restore(cwd, lifecycle.RestoreOptions{
				Yes:     yes,
				DryRun:  dryRun,
				Confirm: promptConfirm,
			})
		},
	}
	cmd.Flags().BoolVarP(&yes, "yes", "y", false, "跳过确认，直接恢复")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "只预检恢复冲突，不修改任何文件")
	root.AddCommand(cmd)
}
