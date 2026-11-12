package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

// findServeCommand 取根命令下注册的 serve 子命令。
func findServeCommand(t *testing.T) *cobra.Command {
	t.Helper()
	for _, cmd := range rootCmd.Commands() {
		if cmd.Name() == "serve" {
			return cmd
		}
	}
	t.Fatal("根命令下未注册 serve 子命令")
	return nil
}

func TestServeCommandRegisteredChinese(t *testing.T) {
	cmd := findServeCommand(t)
	if !strings.Contains(cmd.Short, "只读") {
		t.Errorf("serve Short 应为中文只读说明，得到 %q", cmd.Short)
	}
	for _, name := range []string{"port", "interval", "addr"} {
		if cmd.Flags().Lookup(name) == nil {
			t.Errorf("serve 缺 --%s 选项", name)
		}
	}
	if got := cmd.Flags().Lookup("port").DefValue; got != "17890" {
		t.Errorf("--port 默认值 = %s，想要 17890", got)
	}
	if got := cmd.Flags().Lookup("interval").DefValue; got != "3" {
		t.Errorf("--interval 默认值 = %s，想要 3", got)
	}
	if got := cmd.Flags().Lookup("addr").DefValue; got != "127.0.0.1" {
		t.Errorf("--addr 默认值 = %s，想要 127.0.0.1", got)
	}
}

func TestServeRejectsNonProjectDir(t *testing.T) {
	cmd := findServeCommand(t)
	empty := t.TempDir()
	t.Chdir(empty)
	if err := cmd.RunE(cmd, nil); err == nil {
		t.Fatal("非项目目录应拒绝启动")
	} else if !strings.Contains(err.Error(), ".sunshine") {
		t.Errorf("报错应提示未找到 .sunshine/，得到：%v", err)
	}
}

func TestServeRejectsNonLocalAddr(t *testing.T) {
	cmd := findServeCommand(t)
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, ".sunshine"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Chdir(dir)
	if err := cmd.Flags().Set("addr", "0.0.0.0"); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = cmd.Flags().Set("addr", "127.0.0.1") }()
	if err := cmd.RunE(cmd, nil); err == nil {
		t.Fatal("--addr 0.0.0.0 应拒绝启动")
	} else if !strings.Contains(err.Error(), "127.0.0.1") {
		t.Errorf("报错应提示只支持本机地址，得到：%v", err)
	}
}

func TestServeRejectsBadInterval(t *testing.T) {
	cmd := findServeCommand(t)
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, ".sunshine"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Chdir(dir)
	if err := cmd.Flags().Set("interval", "0"); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = cmd.Flags().Set("interval", "3") }()
	if err := cmd.RunE(cmd, nil); err == nil {
		t.Fatal("--interval 0 应拒绝启动")
	} else if !strings.Contains(err.Error(), "1–30") {
		t.Errorf("报错应提示 1–30 范围，得到：%v", err)
	}
}
