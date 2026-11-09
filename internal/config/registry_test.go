package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeConfigYAML(t *testing.T, root, content string) {
	t.Helper()
	dir := filepath.Join(root, ".sunshine")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "config.yaml"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func readConfigYAML(t *testing.T, root string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(root, ".sunshine", "config.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func TestLoadSpecRegistryConfig(t *testing.T) {
	root := t.TempDir()
	// 无文件 → nil。
	if got, err := LoadSpecRegistryConfig(root); err != nil || got != nil {
		t.Errorf("无文件应返回 nil，得到 %+v, %v", got, err)
	}
	// 无 registry 一节 → nil。
	writeConfigYAML(t, root, "version: 0.1.0\nsession_commit_message: \"x\"\n")
	if got, _ := LoadSpecRegistryConfig(root); got != nil {
		t.Errorf("无 registry 应返回 nil，得到 %+v", got)
	}
	// 带 template。
	writeConfigYAML(t, root, `# 注释
version: 0.1.0

registry:
  spec:
    source: gh:owner/repo
    template: demo-spec

channel:
  worker_guard:
    idle_timeout: 5m
`)
	got, err := LoadSpecRegistryConfig(root)
	if err != nil {
		t.Fatalf("读取失败：%v", err)
	}
	if got == nil || got.Source != "gh:owner/repo" || got.Template != "demo-spec" {
		t.Errorf("得到 %+v", got)
	}
	// 引号标量应去引号；直连型无 template。
	writeConfigYAML(t, root, "registry:\n  spec:\n    source: 'gh:owner/repo'\n")
	got, _ = LoadSpecRegistryConfig(root)
	if got == nil || got.Source != "gh:owner/repo" || got.Template != "" {
		t.Errorf("得到 %+v", got)
	}
	// registry 一节后还有其他顶层字段时仍能读到。
	writeConfigYAML(t, root, "registry:\n  spec:\n    source: gh:owner/repo\n    template: demo-spec\nupdate:\n  skip:\n    - .claude/commands\n")
	got, _ = LoadSpecRegistryConfig(root)
	if got == nil || got.Template != "demo-spec" {
		t.Errorf("得到 %+v", got)
	}
}

func TestStripYamlScalar(t *testing.T) {
	cases := []struct{ in, want string }{
		{"gh:owner/repo", "gh:owner/repo"},
		{"  'gh:owner/repo'  ", "gh:owner/repo"},
		{`"gh:owner/repo"`, "gh:owner/repo"},
		{"''", ""},
	}
	for _, c := range cases {
		if got := stripYamlScalar(c.in); got != c.want {
			t.Errorf("stripYamlScalar(%q) = %q，想得到 %q", c.in, got, c.want)
		}
	}
}

func TestWriteSpecRegistryConfigAppend(t *testing.T) {
	root := t.TempDir()
	// 文件不存在 → 不做任何事（上游行为：由 init 先生成 config.yaml）。
	if err := WriteSpecRegistryConfig(root, SpecRegistryConfig{Source: "gh:o/r"}); err != nil {
		t.Fatalf("不应报错：%v", err)
	}
	if _, err := os.Stat(filepath.Join(root, ".sunshine", "config.yaml")); !os.IsNotExist(err) {
		t.Error("文件不存在时不应创建")
	}

	// 追加一节：保留原有注释与字段。
	writeConfigYAML(t, root, "# 我的配置\nversion: 0.1.0\nsession_commit_message: \"chore: x\"\n")
	if err := WriteSpecRegistryConfig(root, SpecRegistryConfig{Source: "gh:owner/repo", Template: "demo-spec"}); err != nil {
		t.Fatalf("写入失败：%v", err)
	}
	content := readConfigYAML(t, root)
	for _, want := range []string{
		"# 我的配置",
		`session_commit_message: "chore: x"`,
		"registry:",
		"  spec:",
		"    source: gh:owner/repo",
		"    template: demo-spec",
		"sunshine update 会按这里持续刷新",
	} {
		if !strings.Contains(content, want) {
			t.Errorf("写入后缺少 %q：\n%s", want, content)
		}
	}
	if strings.Contains(content, "trellis") || strings.Contains(content, "Trellis") {
		t.Error("生成的注释不应含 trellis 字样")
	}
	// 读回一致。
	got, _ := LoadSpecRegistryConfig(root)
	if got == nil || got.Source != "gh:owner/repo" || got.Template != "demo-spec" {
		t.Errorf("读回 %+v", got)
	}
}

func TestWriteSpecRegistryConfigReplace(t *testing.T) {
	root := t.TempDir()
	writeConfigYAML(t, root, `# 头注释
version: 0.1.0

registry:
  spec:
    source: gh:old/repo
    template: old-template

# 尾注释
channel:
  worker_guard:
    idle_timeout: 5m
`)
	// 替换 source 与 template，保留其他内容。
	if err := WriteSpecRegistryConfig(root, SpecRegistryConfig{Source: "gh:new/repo", Template: "new-template"}); err != nil {
		t.Fatalf("写入失败：%v", err)
	}
	content := readConfigYAML(t, root)
	for _, want := range []string{"# 头注释", "# 尾注释", "idle_timeout: 5m", "source: gh:new/repo", "template: new-template"} {
		if !strings.Contains(content, want) {
			t.Errorf("缺少 %q：\n%s", want, content)
		}
	}
	if strings.Contains(content, "gh:old/repo") || strings.Contains(content, "old-template") {
		t.Errorf("旧值应被替换：\n%s", content)
	}
	// 去掉 template：旧 template 行应消失，source 保留。
	if err := WriteSpecRegistryConfig(root, SpecRegistryConfig{Source: "gh:new/repo"}); err != nil {
		t.Fatalf("写入失败：%v", err)
	}
	content = readConfigYAML(t, root)
	if strings.Contains(content, "template:") {
		t.Errorf("去掉 template 后不应残留 template 行：\n%s", content)
	}
	if !strings.Contains(content, "source: gh:new/repo") {
		t.Errorf("source 应保留：\n%s", content)
	}
	// registry 一节存在但无 spec 子节时补写。
	writeConfigYAML(t, root, "registry:\n  other: 1\nversion: 0.1.0\n")
	if err := WriteSpecRegistryConfig(root, SpecRegistryConfig{Source: "gh:o/r", Template: "t"}); err != nil {
		t.Fatalf("写入失败：%v", err)
	}
	got, _ := LoadSpecRegistryConfig(root)
	if got == nil || got.Source != "gh:o/r" || got.Template != "t" {
		t.Errorf("读回 %+v", got)
	}
}

func TestConfigRegistryFieldRoundTrip(t *testing.T) {
	// Config 结构读写 registry.spec（yaml.v3 路径）。
	cfg, err := LoadBytes([]byte("version: 0.1.0\nregistry:\n  spec:\n    source: gh:o/r\n    template: demo-spec\n"))
	if err != nil {
		t.Fatalf("LoadBytes 失败：%v", err)
	}
	if cfg.Registry == nil || cfg.Registry.Spec.Source != "gh:o/r" || cfg.Registry.Spec.Template != "demo-spec" {
		t.Errorf("Registry = %+v", cfg.Registry)
	}
	// 未配 registry 时 Marshal 不应输出空 registry 一节。
	plain, err := Default().Marshal()
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(plain), "registry:") {
		t.Errorf("默认配置不应输出 registry：\n%s", plain)
	}
}
