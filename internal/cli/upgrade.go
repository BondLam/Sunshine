package cli

import (
	"context"
	"errors"

	"github.com/linrong1982/Sunshine/internal/update"
	"github.com/linrong1982/Sunshine/internal/upgrade"
	"github.com/spf13/cobra"
)

// 注意：版本与仓库地址用 A 组在 root.go 提供的 ldflags 变量与
// 导出访问函数（Version() / Repo()）；本文件不另起一套变量。
// upgrade 与 update 的最新版检查共用这两个访问函数。

var (
	upgradeTag    string
	upgradeDryRun bool
)

// wireLatestVersionFunc 把 GitHub Releases 最新版查询接给
// update.LatestVersionFunc：update 的最新版检查与 upgrade 共用
// 同一函数与同一仓库变量。未配置或离线时查询返回错误，
// update 侧只警告不中断（update.go 已有该逻辑）。
func wireLatestVersionFunc() {
	update.LatestVersionFunc = func() (string, error) {
		client := &upgrade.Client{Repo: Repo()}
		return client.LatestVersion(context.Background())
	}
}

// RegisterUpgradeCommand 注册 sunshine upgrade 命令。
func RegisterUpgradeCommand(root *cobra.Command) {
	wireLatestVersionFunc()
	cmd := &cobra.Command{
		Use:   "upgrade",
		Short: "升级 Sunshine CLI 自身到最新版",
		Long: `升级 Sunshine CLI 自身到最新版（查 GitHub Releases → 下载 → 校验 → 替换）。

与 sunshine update 的区别：
  - sunshine upgrade：升级 CLI 自身（换掉 sunshine 这个二进制）；
  - sunshine update：更新项目内的模板文件（.sunshine/ 与各平台配置）。

流程：查最新版 → 已是最新则提示退出 → 按当前系统与架构选资产 →
下载 → 按 checksums.txt 校验 sha256 → 备份当前二进制（同目录 .bak，
权限保留）→ 替换 → 运行新版 --version 校验 → 失败回滚备份并报错。

不传 --tag 时按当前版本推断通道：含 -beta 走 beta，含 -rc 走 rc，
否则走 latest。仓库地址未配置或离线时报错说明，不静默跳过。

示例：
  sunshine upgrade
  sunshine upgrade --tag beta
  sunshine upgrade --tag 0.2.0
  sunshine upgrade --dry-run`,
		RunE: func(cmd *cobra.Command, args []string) error {
			_, err := upgrade.Run(cmd.Context(), upgrade.Options{
				CurrentVersion: version,
				Tag:            upgradeTag,
				DryRun:         upgradeDryRun,
				Repo:           Repo(),
				Out:            cmd.OutOrStdout(),
			})
			if errors.Is(err, upgrade.ErrAlreadyLatest) {
				return nil
			}
			return err
		},
	}
	cmd.Flags().StringVar(&upgradeTag, "tag", "", "目标版本：通道（latest/beta/rc）或具体版本号（如 0.2.0）；不传时按当前版本推断")
	cmd.Flags().BoolVar(&upgradeDryRun, "dry-run", false, "只打印升级计划，不写盘")
	root.AddCommand(cmd)
}
