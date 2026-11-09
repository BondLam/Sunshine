package scaffold

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestInitWithOptionsSkipSpec(t *testing.T) {
	dir := t.TempDir()
	if err := InitWithOptions(dir, "tester", InitOptions{SkipSpec: true}); err != nil {
		t.Fatalf("InitWithOptions 失败：%v", err)
	}
	// 骨架其他文件应存在。
	if _, err := os.Stat(filepath.Join(dir, ".sunshine", "config.yaml")); err != nil {
		t.Errorf("config.yaml 应存在：%v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, ".sunshine", "workflow.md")); err != nil {
		t.Errorf("workflow.md 应存在：%v", err)
	}
	// spec 模板应被跳过（目录不存在或为空）。
	specDir := filepath.Join(dir, ".sunshine", "spec")
	entries, err := os.ReadDir(specDir)
	if err == nil && len(entries) != 0 {
		t.Errorf("SkipSpec 时 spec 目录应为空，得到 %v", entries)
	}
	// 哈希清单里不应有 spec 条目。
	data, err := os.ReadFile(filepath.Join(dir, ".sunshine", ".template-hashes.json"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), ".sunshine/spec/") {
		t.Error("哈希清单不应含 spec 条目")
	}
}

func TestInitDefaultWritesSpec(t *testing.T) {
	dir := t.TempDir()
	if err := Init(dir, "tester", false); err != nil {
		t.Fatalf("Init 失败：%v", err)
	}
	entries, err := os.ReadDir(filepath.Join(dir, ".sunshine", "spec"))
	if err != nil || len(entries) == 0 {
		t.Errorf("默认 Init 应写 spec 模板：%v, %v", entries, err)
	}
}

func TestWriteDefaultSpec(t *testing.T) {
	dir := t.TempDir()
	if err := InitWithOptions(dir, "tester", InitOptions{SkipSpec: true}); err != nil {
		t.Fatalf("InitWithOptions 失败：%v", err)
	}
	files, err := WriteDefaultSpec(dir)
	if err != nil {
		t.Fatalf("WriteDefaultSpec 失败：%v", err)
	}
	if len(files) == 0 {
		t.Fatal("应写出 spec 文件")
	}
	for _, f := range files {
		if _, err := os.Stat(filepath.Join(dir, filepath.FromSlash(f))); err != nil {
			t.Errorf("文件 %s 不存在：%v", f, err)
		}
		if !strings.Contains(f, ".sunshine/spec/") {
			t.Errorf("文件路径 %q 应在 .sunshine/spec/ 下", f)
		}
	}
}
