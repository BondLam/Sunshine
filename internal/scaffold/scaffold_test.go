package scaffold

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/BondLam/Sunshine/internal/projecttype"
	"github.com/BondLam/Sunshine/templates"
)

func TestInitSpecTypeSkipsMismatchedSpec(t *testing.T) {
	// SpecType=Frontend：只写 system.md + frontend.md，backend.md 不写，
	// 且哈希清单里没有 backend.md。
	dir := t.TempDir()
	if err := InitWithOptions(dir, "张三", InitOptions{SpecType: projecttype.Frontend}); err != nil {
		t.Fatalf("InitWithOptions Frontend 失败：%v", err)
	}
	assertSpecFiles(t, dir, true, false)
	assertSpecHashes(t, dir, true, false)

	// SpecType=Backend：只写 system.md + backend.md。
	dir = t.TempDir()
	if err := InitWithOptions(dir, "张三", InitOptions{SpecType: projecttype.Backend}); err != nil {
		t.Fatalf("InitWithOptions Backend 失败：%v", err)
	}
	assertSpecFiles(t, dir, false, true)
	assertSpecHashes(t, dir, false, true)

	// SpecType=Fullstack 与零值（Unknown）：三份全写。
	for _, specType := range []projecttype.Type{projecttype.Fullstack, projecttype.Unknown} {
		dir = t.TempDir()
		if err := InitWithOptions(dir, "张三", InitOptions{SpecType: specType}); err != nil {
			t.Fatalf("InitWithOptions %v 失败：%v", specType, err)
		}
		assertSpecFiles(t, dir, true, true)
		assertSpecHashes(t, dir, true, true)
	}
}

// TestInitDesignWritesDocTemplates 校验 SpecType=Design：spec 三份全写
// （与全栈相同），doc/ 下写四份设计文档模板；
// 模板不进哈希清单；已填过的文件 -f 重建也不覆盖。
func TestInitDesignWritesDocTemplates(t *testing.T) {
	dir := t.TempDir()
	if err := InitWithOptions(dir, "张三", InitOptions{SpecType: projecttype.Design}); err != nil {
		t.Fatalf("InitWithOptions Design 失败：%v", err)
	}
	assertSpecFiles(t, dir, true, true)
	assertSpecHashes(t, dir, true, true)
	assertDesignTemplates(t, dir)

	// 模板不进哈希清单（doc/ 是设计人员的交付物，不归 update 管）。
	data, err := os.ReadFile(filepath.Join(dir, ".sunshine/.template-hashes.json"))
	if err != nil {
		t.Fatalf("读取哈希清单失败：%v", err)
	}
	for _, key := range []string{"doc/design/需求列表.md", "doc/design/概要设计.md", "doc/prototype/index.html", "doc/prototype/示例页.html"} {
		if strings.Contains(string(data), key) {
			t.Errorf("哈希清单不应含 %s", key)
		}
	}

	// 非 Design 类型不写 doc/。
	dir = t.TempDir()
	if err := InitWithOptions(dir, "张三", InitOptions{SpecType: projecttype.Fullstack}); err != nil {
		t.Fatalf("InitWithOptions Fullstack 失败：%v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "doc")); !os.IsNotExist(err) {
		t.Error("Fullstack 不应生成 doc/")
	}

	// 已填过的文件 -f 重建也不覆盖；缺失的补写。
	dir = t.TempDir()
	if err := InitWithOptions(dir, "张三", InitOptions{SpecType: projecttype.Design}); err != nil {
		t.Fatalf("InitWithOptions Design 失败：%v", err)
	}
	edited := "# 需求列表\n\n已填好的真实需求\n"
	if err := os.WriteFile(filepath.Join(dir, "doc/design/需求列表.md"), []byte(edited), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(dir, "doc/prototype/index.html")); err != nil {
		t.Fatal(err)
	}
	if err := InitWithOptions(dir, "张三", InitOptions{Force: true, SpecType: projecttype.Design}); err != nil {
		t.Fatalf("InitWithOptions Design -f 失败：%v", err)
	}
	kept, err := os.ReadFile(filepath.Join(dir, "doc/design/需求列表.md"))
	if err != nil {
		t.Fatal(err)
	}
	if string(kept) != edited {
		t.Errorf("-f 不应覆盖已填的 需求列表.md，实际：%q", string(kept))
	}
	if _, err := os.Stat(filepath.Join(dir, "doc/prototype/index.html")); err != nil {
		t.Errorf("-f 应补写缺失的 index.html：%v", err)
	}
}

// assertDesignTemplates 校验 doc/ 下四份设计文档模板存在且内容是模板。
func assertDesignTemplates(t *testing.T, dir string) {
	t.Helper()
	for _, tc := range []struct {
		rel    string
		marker string
	}{
		{"doc/design/需求列表.md", "# 需求列表"},
		{"doc/design/概要设计.md", "# 概要设计"},
		{"doc/prototype/index.html", "（请填写产品名称）原型"},
		{"doc/prototype/示例页.html", "（请填写产品名称）原型 · 示例页"},
	} {
		data, err := os.ReadFile(filepath.Join(dir, filepath.FromSlash(tc.rel)))
		if err != nil {
			t.Errorf("缺少 %s：%v", tc.rel, err)
			continue
		}
		if !strings.Contains(string(data), tc.marker) {
			t.Errorf("%s 内容不对，应含 %q", tc.rel, tc.marker)
		}
	}
}

// assertSpecFiles 校验 spec 目录的文件集：system.md 永远存在，
// frontend.md / backend.md 按 wantFrontend / wantBackend 断言存在与否。
func assertSpecFiles(t *testing.T, dir string, wantFrontend, wantBackend bool) {
	t.Helper()
	if _, err := os.Stat(filepath.Join(dir, ".sunshine/spec/system.md")); err != nil {
		t.Errorf("缺少 spec/system.md：%v", err)
	}
	for _, tc := range []struct {
		name string
		want bool
	}{
		{"frontend.md", wantFrontend},
		{"backend.md", wantBackend},
	} {
		_, err := os.Stat(filepath.Join(dir, ".sunshine/spec", tc.name))
		if tc.want && err != nil {
			t.Errorf("缺少 spec/%s：%v", tc.name, err)
		}
		if !tc.want && err == nil {
			t.Errorf("不应生成 spec/%s", tc.name)
		}
	}
}

// assertSpecHashes 校验哈希清单只含实际写出的 spec 文件。
func assertSpecHashes(t *testing.T, dir string, wantFrontend, wantBackend bool) {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(dir, ".sunshine/.template-hashes.json"))
	if err != nil {
		t.Fatalf("读取哈希清单失败：%v", err)
	}
	var manifest struct {
		Files map[string]string `json:"files"`
	}
	if err := json.Unmarshal(data, &manifest); err != nil {
		t.Fatalf("解析哈希清单失败：%v", err)
	}
	if _, ok := manifest.Files[".sunshine/spec/system.md"]; !ok {
		t.Error("哈希清单缺少 .sunshine/spec/system.md")
	}
	for _, tc := range []struct {
		key  string
		want bool
	}{
		{".sunshine/spec/frontend.md", wantFrontend},
		{".sunshine/spec/backend.md", wantBackend},
	} {
		_, ok := manifest.Files[tc.key]
		if tc.want && !ok {
			t.Errorf("哈希清单缺少 %s", tc.key)
		}
		if !tc.want && ok {
			t.Errorf("哈希清单不应含 %s", tc.key)
		}
	}
}

func TestInitCreatesSkeleton(t *testing.T) {
	dir := t.TempDir()
	if err := Init(dir, "张三", false); err != nil {
		t.Fatalf("Init 失败：%v", err)
	}
	for _, rel := range []string{
		".sunshine/config.yaml",
		".sunshine/workflow.md",
		".sunshine/AGENTS.md",
		".sunshine/.version",
		".sunshine/.developer",
		".sunshine/.template-hashes.json",
		".sunshine/agents/architect.md",
		".sunshine/agents/check.md",
		".sunshine/agents/implement.md",
		".sunshine/agents/plan.md",
		".sunshine/agents/prototype.md",
		".sunshine/agents/research.md",
		".sunshine/spec/system.md",
		".sunshine/spec/frontend.md",
		".sunshine/spec/backend.md",
		".sunshine/workspace/index.md",
		".sunshine/tasks/README.md",
	} {
		if _, err := os.Stat(filepath.Join(dir, rel)); err != nil {
			t.Errorf("缺少文件 %s：%v", rel, err)
		}
	}
	data, _ := os.ReadFile(filepath.Join(dir, ".sunshine/config.yaml"))
	if !strings.Contains(string(data), "Sunshine 项目配置文件") {
		t.Error("config.yaml 不是中文模板")
	}
	// 完整字段版 config.yaml：已启用字段与注释字段齐全，且为中文注释。
	for _, want := range []string{
		"session_commit_message", "max_journal_lines", "session_auto_commit",
		"after_create", "after_start", "after_finish", "after_archive",
		"packages", "default_package",
		"channel:", "worker_guard", "idle_timeout", "max_live_workers",
		"codex:", "dispatch_mode",
		"context_injection", "max_file_bytes", "max_artifact_bytes", "max_total_bytes",
		"prompt_injection", "skip_keyword", "no-sunshine",
	} {
		if !strings.Contains(string(data), want) {
			t.Errorf("config.yaml 缺少字段 %s", want)
		}
	}
	if strings.Contains(string(data), "trellis") || strings.Contains(string(data), "Trellis") {
		t.Error("config.yaml 残留 trellis 字样")
	}
	// workflow.md 与完整版 native 模板同源（2026-11-09 用户确认 init 改用完整版）。
	native, err := templates.Workflows.ReadFile("workflows/native/workflow.md")
	if err != nil {
		t.Fatalf("读取原生工作流模板失败：%v", err)
	}
	got, err := os.ReadFile(filepath.Join(dir, ".sunshine/workflow.md"))
	if err != nil {
		t.Fatalf("读取 workflow.md 失败：%v", err)
	}
	if !bytes.Equal(got, native) {
		t.Error("init 生成的 workflow.md 与完整版 native 模板不一致")
	}
	// agents 六个定义均为中文。
	for _, name := range []string{"architect", "check", "implement", "plan", "prototype", "research"} {
		agent, err := os.ReadFile(filepath.Join(dir, ".sunshine", "agents", name+".md"))
		if err != nil {
			t.Errorf("缺少 agent 定义 %s：%v", name, err)
			continue
		}
		if strings.Contains(string(agent), "trellis") || strings.Contains(string(agent), "Trellis") {
			t.Errorf("agent 定义 %s 残留 trellis 字样", name)
		}
	}
	dev, _ := os.ReadFile(filepath.Join(dir, ".sunshine/.developer"))
	if strings.TrimSpace(string(dev)) != "张三" {
		t.Errorf(".developer 内容不对：%q", dev)
	}
	// 重复初始化应拒绝。
	if err := Init(dir, "张三", false); err == nil {
		t.Error("重复初始化未拒绝")
	}
	// --force 可覆盖。
	if err := Init(dir, "李四", true); err != nil {
		t.Fatalf("force 初始化失败：%v", err)
	}
	dev, _ = os.ReadFile(filepath.Join(dir, ".sunshine/.developer"))
	if strings.TrimSpace(string(dev)) != "李四" {
		t.Errorf("force 后 .developer 内容不对：%q", dev)
	}
}
