package scaffold

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestInitWithOptionsVersion：传了 Version 时 .version 写该值（与 CLI 版本同源），
// 不传时沿用模板文件内容。
func TestInitWithOptionsVersion(t *testing.T) {
	dir := t.TempDir()
	if err := InitWithOptions(dir, "张三", InitOptions{Version: "9.9.9-test"}); err != nil {
		t.Fatalf("InitWithOptions 失败：%v", err)
	}
	data, err := os.ReadFile(filepath.Join(dir, ".sunshine", ".version"))
	if err != nil {
		t.Fatalf("读取 .version 失败：%v", err)
	}
	if strings.TrimSpace(string(data)) != "9.9.9-test" {
		t.Errorf(".version 内容不对：%q，想要 %q", data, "9.9.9-test\n")
	}

	// 不传 Version 时沿用模板文件内容（现有行为不变）。
	dir2 := t.TempDir()
	if err := Init(dir2, "张三", false); err != nil {
		t.Fatalf("Init 失败：%v", err)
	}
	data, err = os.ReadFile(filepath.Join(dir2, ".sunshine", ".version"))
	if err != nil {
		t.Fatalf("读取 .version 失败：%v", err)
	}
	if strings.TrimSpace(string(data)) == "" {
		t.Error("未传 Version 时 .version 为空")
	}
}
