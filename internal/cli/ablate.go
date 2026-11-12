package cli

import (
	"os"

	"github.com/BondLam/Sunshine/internal/lifecycle"
	"github.com/spf13/cobra"
)

// RegisterAblateCommand 把 `sunshine ablate` 接到根命令上。
// 由总协调人统一接线（不要改 root.go，也不要用 init() 自注册）。
func RegisterAblateCommand(root *cobra.Command) {
	var yes, dryRun bool
	cmd := &cobra.Command{
		Use:   "ablate",
		Short: "临时移除项目全部 Sunshine 表面（可逆）",
		Long: `临时移除项目全部 Sunshine 表面（可逆）：把托管内容移出项目，
原内容存入项目外的版本化恢复事务（state.json + backup/），
之后用 sunshine restore 精确恢复。

事务根目录由 SUNSHINE_ABLATION_STATE_ROOT 指定，
未设时默认 ~/.sunshine/ablations/v1；事务根目录必须在项目外。

已切除或上一次切除被中断的项目会先提示运行 sunshine restore。

示例：
  sunshine ablate --dry-run
  sunshine ablate -y`,
		RunE: func(cmd *cobra.Command, args []string) error {
			cwd, err := os.Getwd()
			if err != nil {
				return err
			}
			return lifecycle.Ablate(cwd, version, lifecycle.AblateOptions{
				Yes:     yes,
				DryRun:  dryRun,
				Confirm: promptConfirm,
			})
		},
	}
	cmd.Flags().BoolVarP(&yes, "yes", "y", false, "跳过确认，直接切除")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "只打印切除计划，不修改任何文件或恢复状态")
	root.AddCommand(cmd)
}
