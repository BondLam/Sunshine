package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/linrong1982/Sunshine/internal/platforms"
	"github.com/linrong1982/Sunshine/internal/scaffold"
	"github.com/spf13/cobra"
)

func TestRegisterUpdateCommand(t *testing.T) {
	root := &cobra.Command{Use: "sunshine"}
	RegisterUpdateCommand(root)
	found := false
	for _, c := range root.Commands() {
		if c.Name() == "update" {
			found = true
			for _, flag := range []string{"dry-run", "force", "skip-all", "create-new", "allow-downgrade", "migrate"} {
				if c.Flags().Lookup(flag) == nil {
					t.Errorf("update 缺少 --%s", flag)
				}
			}
		}
	}
	if !found {
		t.Fatal("RegisterUpdateCommand 未注册 update 命令")
	}

	// 在临时项目里跑 --dry-run（非交互 stdin 应默认跳过冲突）。
	dir := t.TempDir()
	if err := scaffold.Init(dir, "测试", false); err != nil {
		t.Fatal(err)
	}
	files, err := platforms.Install(dir, "claude-code", platforms.InstallOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if err := scaffold.RecordHashes(dir, files); err != nil {
		t.Fatal(err)
	}
	checkPath := filepath.Join(dir, ".sunshine", "agents", "check.md")
	if err := os.WriteFile(checkPath, []byte("用户改过的检查\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	oldWd, _ := os.Getwd()
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.Chdir(oldWd) }()
	// stdin 设为 /dev/null，模拟非交互。
	oldStdin := os.Stdin
	devNull, err := os.Open(os.DevNull)
	if err != nil {
		t.Fatal(err)
	}
	os.Stdin = devNull
	defer func() { os.Stdin = oldStdin; _ = devNull.Close() }()

	// 重置包级 flag（避免与其他测试互相污染）。
	updateDryRun, updateForce, updateSkipAll, updateCreateNew, updateAllowDowngrade, updateMigrate = false, false, false, false, false, false
	root.SetArgs([]string{"update", "--dry-run"})
	var buf bytes.Buffer
	root.SetOut(&buf)
	root.SetErr(&buf)
	if err := root.Execute(); err != nil {
		t.Fatalf("update --dry-run 失败：%v", err)
	}
	if got := readUpdateTestFile(t, checkPath); got != "用户改过的检查\n" {
		t.Error("dry-run 不应改动文件")
	}
	// 未初始化的目录应报错。
	empty := t.TempDir()
	if err := os.Chdir(empty); err != nil {
		t.Fatal(err)
	}
	root2 := &cobra.Command{Use: "sunshine", SilenceErrors: true, SilenceUsage: true}
	RegisterUpdateCommand(root2)
	updateDryRun, updateForce, updateSkipAll, updateCreateNew, updateAllowDowngrade, updateMigrate = false, false, false, false, false, false
	root2.SetArgs([]string{"update"})
	if err := root2.Execute(); err == nil {
		t.Error("未初始化的目录应报错")
	} else if !strings.Contains(err.Error(), "尚未初始化") {
		t.Errorf("错误信息应提示尚未初始化，实际 %v", err)
	}
}

func readUpdateTestFile(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}
