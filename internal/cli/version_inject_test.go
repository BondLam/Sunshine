package cli

import "testing"

// TestVersionDefaults：开发构建（无 ldflags 注入）时版本与仓库地址取默认值。
// ldflags 注入本身由发布构建验证（goreleaser build --snapshot 后
// sunshine --version 打印注入值），这里只锁定默认值不被意外改掉。
func TestVersionDefaults(t *testing.T) {
	if got := Version(); got != "0.1.0" {
		t.Errorf("Version() 默认值不对：%q，想要 %q", got, "0.1.0")
	}
	if got := Repo(); got != "github.com/BondLam/Sunshine" {
		t.Errorf("Repo() 默认值不对：%q，想要 %q", got, "github.com/BondLam/Sunshine")
	}
}
