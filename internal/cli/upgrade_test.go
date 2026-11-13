package cli

import (
	"strings"
	"testing"

	"github.com/linrong007/Sunshine/internal/update"
	"github.com/linrong007/Sunshine/internal/upgrade"
	"github.com/spf13/cobra"
)

func TestRegisterUpgradeCommand(t *testing.T) {
	old := update.LatestVersionFunc
	defer func() { update.LatestVersionFunc = old }()

	root := &cobra.Command{Use: "sunshine", SilenceErrors: true, SilenceUsage: true}
	RegisterUpgradeCommand(root)
	cmd, _, err := root.Find([]string{"upgrade"})
	if err != nil {
		t.Fatalf("找不到 upgrade 命令：%v", err)
	}
	if cmd == nil || cmd.Name() != "upgrade" {
		t.Fatal("upgrade 命令未注册")
	}
	if cmd.Flags().Lookup("tag") == nil || cmd.Flags().Lookup("dry-run") == nil {
		t.Error("upgrade 缺少 --tag 或 --dry-run 选项")
	}
	if !strings.Contains(cmd.Long, "upgrade") || !strings.Contains(cmd.Long, "update") {
		t.Error("upgrade 帮助应写明与 update 的区别")
	}
	if strings.Contains(strings.ToLower(cmd.Long+cmd.Short), "trellis") {
		t.Error("upgrade 帮助残留 trellis 字样")
	}
	// LatestVersionFunc 已接线。“未配置”路径改由占位值显式构造验证
	// （默认仓库已是真实地址 github.com/linrong007/Sunshine，
	// 不再靠默认值是占位值来覆盖该分支；update 侧对此只警告不中断）。
	if update.LatestVersionFunc == nil {
		t.Fatal("LatestVersionFunc 未接线")
	}
	oldRepo := repo
	repo = upgrade.PlaceholderRepo
	_, latestErr := update.LatestVersionFunc()
	repo = oldRepo
	if latestErr == nil || !strings.Contains(latestErr.Error(), "未配置") {
		t.Errorf("占位仓库的最新版检查应报未配置，实际 err=%v", latestErr)
	}
	// 访问函数与版本变量一致。
	if Version() != version {
		t.Errorf("Version() = %q，期望 %q", Version(), version)
	}
	if Repo() == "" {
		t.Error("Repo() 不应为空")
	}
}

func TestUpgradeDryRunUnconfigured(t *testing.T) {
	old := update.LatestVersionFunc
	defer func() { update.LatestVersionFunc = old }()
	// 默认仓库已是真实地址；显式把仓库变量换成占位值再跑，
	// 覆盖“仓库未配置”分支（跑完恢复）。
	oldRepo := repo
	repo = upgrade.PlaceholderRepo
	defer func() { repo = oldRepo }()

	root := &cobra.Command{Use: "sunshine", SilenceErrors: true, SilenceUsage: true}
	RegisterUpgradeCommand(root)
	root.SetArgs([]string{"upgrade", "--dry-run"})
	err := root.Execute()
	if err == nil {
		t.Fatal("仓库未配置时 upgrade --dry-run 应报错")
	}
	if !strings.Contains(err.Error(), "未配置") {
		t.Errorf("报错应说明仓库未配置，实际：%v", err)
	}
}
