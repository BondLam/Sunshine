package workflow

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func mustResolve(t *testing.T, id string) Resolved {
	t.Helper()
	r, err := Resolve(id, "")
	if err != nil {
		t.Fatalf("解析 %q 失败：%v", id, err)
	}
	return r
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("读 %s 失败：%v", path, err)
	}
	return string(data)
}

func TestApplyMissingWritesAndRegistersHash(t *testing.T) {
	cwd := t.TempDir()
	if err := os.MkdirAll(filepath.Join(cwd, ".sunshine"), 0o755); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	if err := Apply(cwd, mustResolve(t, "native"), ApplyOptions{Out: &out}); err != nil {
		t.Fatalf("Apply 失败：%v", err)
	}
	if got := readFile(t, filepath.Join(cwd, ".sunshine", "workflow.md")); got != mustResolve(t, "native").Content {
		t.Error("缺失时应写入 native 模板内容")
	}
	// native 应登记哈希。
	hashes, err := loadHashes(cwd)
	if err != nil {
		t.Fatal(err)
	}
	if hashes[WorkflowRelPath] == "" {
		t.Error("native 写入后应登记哈希条目")
	}
}

func TestApplyIdenticalRefreshesHashOnly(t *testing.T) {
	cwd := t.TempDir()
	tmpl := mustResolve(t, "native")
	writeWorkflow(t, cwd, tmpl.Content)
	// 清单里先放一个错误的哈希：内容相同应刷新为正确值。
	writeManifest(t, cwd, map[string]string{WorkflowRelPath: hashOf("# 错的\n")})
	var out bytes.Buffer
	if err := Apply(cwd, tmpl, ApplyOptions{Out: &out}); err != nil {
		t.Fatalf("Apply 失败：%v", err)
	}
	hashes, _ := loadHashes(cwd)
	if hashes[WorkflowRelPath] != hashOf(tmpl.Content) {
		t.Error("内容相同时应刷新哈希条目")
	}
}

func TestApplyPristineOverwrites(t *testing.T) {
	cwd := t.TempDir()
	current := "# 旧的原生内容\n"
	writeWorkflow(t, cwd, current)
	writeManifest(t, cwd, map[string]string{WorkflowRelPath: hashOf(current)})
	var out bytes.Buffer
	if err := Apply(cwd, mustResolve(t, "tdd"), ApplyOptions{Out: &out}); err != nil {
		t.Fatalf("Apply 失败：%v", err)
	}
	if got := readFile(t, filepath.Join(cwd, ".sunshine", "workflow.md")); got != mustResolve(t, "tdd").Content {
		t.Error("未改动时应直接覆盖为 tdd")
	}
	// 非 native 应删除哈希条目。
	hashes, _ := loadHashes(cwd)
	if _, ok := hashes[WorkflowRelPath]; ok {
		t.Error("切换到 tdd 后应删除 workflow.md 的哈希条目")
	}
}

func TestApplyModifiedNonInteractiveErrors(t *testing.T) {
	cwd := t.TempDir()
	writeWorkflow(t, cwd, "# 我改过的\n")
	writeManifest(t, cwd, map[string]string{WorkflowRelPath: hashOf("# 原始\n")})
	var out bytes.Buffer
	err := Apply(cwd, mustResolve(t, "tdd"), ApplyOptions{Out: &out, Interactive: false})
	if err == nil {
		t.Fatal("已改动且非交互、无 --force 时应报错")
	}
	if !strings.Contains(err.Error(), "--force") || !strings.Contains(err.Error(), "--create-new") {
		t.Errorf("报错应提示 --force 与 --create-new，实际：%v", err)
	}
	if got := readFile(t, filepath.Join(cwd, ".sunshine", "workflow.md")); got != "# 我改过的\n" {
		t.Error("报错时不应改动现有文件")
	}
}

func TestApplyModifiedForceOverwrites(t *testing.T) {
	cwd := t.TempDir()
	writeWorkflow(t, cwd, "# 我改过的\n")
	writeManifest(t, cwd, map[string]string{WorkflowRelPath: hashOf("# 原始\n")})
	var out bytes.Buffer
	if err := Apply(cwd, mustResolve(t, "tdd"), ApplyOptions{Out: &out, Force: true}); err != nil {
		t.Fatalf("Apply 失败：%v", err)
	}
	if got := readFile(t, filepath.Join(cwd, ".sunshine", "workflow.md")); got != mustResolve(t, "tdd").Content {
		t.Error("--force 时应覆盖")
	}
}

func TestApplyModifiedInteractiveChoices(t *testing.T) {
	cases := []struct {
		name   string
		action OverwriteAction
		check  func(t *testing.T, cwd string)
	}{
		{"覆盖", ActionOverwrite, func(t *testing.T, cwd string) {
			if got := readFile(t, filepath.Join(cwd, ".sunshine", "workflow.md")); got != mustResolve(t, "tdd").Content {
				t.Error("选覆盖时应写入 tdd")
			}
		}},
		{"写 .new", ActionCreateNew, func(t *testing.T, cwd string) {
			if got := readFile(t, filepath.Join(cwd, ".sunshine", "workflow.md")); got != "# 我改过的\n" {
				t.Error("选写 .new 时不应动 workflow.md")
			}
			if got := readFile(t, filepath.Join(cwd, ".sunshine", "workflow.md.new")); got != mustResolve(t, "tdd").Content {
				t.Error("选写 .new 时应写入 workflow.md.new")
			}
		}},
		{"跳过", ActionSkip, func(t *testing.T, cwd string) {
			if got := readFile(t, filepath.Join(cwd, ".sunshine", "workflow.md")); got != "# 我改过的\n" {
				t.Error("选跳过时不应动 workflow.md")
			}
			if _, err := os.Stat(filepath.Join(cwd, ".sunshine", "workflow.md.new")); !os.IsNotExist(err) {
				t.Error("选跳过时不应写 .new")
			}
		}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			cwd := t.TempDir()
			writeWorkflow(t, cwd, "# 我改过的\n")
			writeManifest(t, cwd, map[string]string{WorkflowRelPath: hashOf("# 原始\n")})
			var out bytes.Buffer
			err := Apply(cwd, mustResolve(t, "tdd"), ApplyOptions{
				Out:         &out,
				Interactive: true,
				Confirm:     func() (OverwriteAction, error) { return c.action, nil },
			})
			if err != nil {
				t.Fatalf("Apply 失败：%v", err)
			}
			c.check(t, cwd)
		})
	}
}

func TestApplyUnknownNeedsConfirm(t *testing.T) {
	cwd := t.TempDir()
	writeWorkflow(t, cwd, "# 来历不明的内容\n")
	// 清单里没有 workflow.md 条目（与上游一致：按已改动走交互确认）。
	writeManifest(t, cwd, map[string]string{".sunshine/config.yaml": hashOf("c")})
	var out bytes.Buffer
	// 非交互且无 --force：报错，不写盘也不写 .new。
	if err := Apply(cwd, mustResolve(t, "tdd"), ApplyOptions{Out: &out}); err == nil {
		t.Fatal("未知来源 + 非交互时应报错")
	}
	if got := readFile(t, filepath.Join(cwd, ".sunshine", "workflow.md")); got != "# 来历不明的内容\n" {
		t.Error("未知来源 + 非交互时不应覆盖 workflow.md")
	}
	if _, err := os.Stat(filepath.Join(cwd, ".sunshine", "workflow.md.new")); !os.IsNotExist(err) {
		t.Error("未知来源 + 非交互时不应写 workflow.md.new")
	}
	// 交互确认选“写 .new”：写 .new，workflow.md 不动，清单不动。
	if err := Apply(cwd, mustResolve(t, "tdd"), ApplyOptions{
		Out:         &out,
		Interactive: true,
		Confirm:     func() (OverwriteAction, error) { return ActionCreateNew, nil },
	}); err != nil {
		t.Fatalf("交互确认写 .new 失败：%v", err)
	}
	if got := readFile(t, filepath.Join(cwd, ".sunshine", "workflow.md.new")); got != mustResolve(t, "tdd").Content {
		t.Error("交互确认选写 .new 时应写 workflow.md.new")
	}
	hashes, _ := loadHashes(cwd)
	if _, ok := hashes[WorkflowRelPath]; ok {
		t.Error("写 .new 不应新增 workflow.md 的哈希条目")
	}
	if hashes[".sunshine/config.yaml"] != hashOf("c") {
		t.Error("写 .new 不应动其他哈希条目")
	}
}

func TestApplyUnknownForceOverwrites(t *testing.T) {
	cwd := t.TempDir()
	writeWorkflow(t, cwd, "# 来历不明的内容\n")
	var out bytes.Buffer
	if err := Apply(cwd, mustResolve(t, "tdd"), ApplyOptions{Out: &out, Force: true}); err != nil {
		t.Fatalf("Apply 失败：%v", err)
	}
	if got := readFile(t, filepath.Join(cwd, ".sunshine", "workflow.md")); got != mustResolve(t, "tdd").Content {
		t.Error("未知来源 + --force 时应覆盖")
	}
}

func TestApplyCreateNewAlwaysWritesNew(t *testing.T) {
	cwd := t.TempDir()
	current := "# 当前内容\n"
	writeWorkflow(t, cwd, current)
	writeManifest(t, cwd, map[string]string{WorkflowRelPath: hashOf(current)})
	var out bytes.Buffer
	if err := Apply(cwd, mustResolve(t, "native"), ApplyOptions{Out: &out, CreateNew: true}); err != nil {
		t.Fatalf("Apply 失败：%v", err)
	}
	if got := readFile(t, filepath.Join(cwd, ".sunshine", "workflow.md")); got != current {
		t.Error("--create-new 时不应动 workflow.md")
	}
	if got := readFile(t, filepath.Join(cwd, ".sunshine", "workflow.md.new")); got != mustResolve(t, "native").Content {
		t.Error("--create-new 时应写 workflow.md.new")
	}
	// --create-new 不动哈希清单。
	hashes, _ := loadHashes(cwd)
	if hashes[WorkflowRelPath] != hashOf(current) {
		t.Error("--create-new 不应改动哈希清单")
	}
}
