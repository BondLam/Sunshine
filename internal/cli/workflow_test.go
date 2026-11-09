package cli

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/linrong1982/Sunshine/internal/workflow"
)

// resetWorkflowFlags 恢复 workflow 命令的 flag 变量默认值。
func resetWorkflowFlags() {
	workflowTemplate = ""
	workflowMarketplace = ""
	workflowList = false
	workflowForce = false
	workflowCreateNew = false
}

func TestWorkflowCommandFlags(t *testing.T) {
	resetWorkflowFlags()
	for _, want := range []struct{ name, shorthand string }{
		{"template", "t"},
		{"marketplace", "m"},
		{"list", ""},
		{"force", "f"},
		{"create-new", "n"},
	} {
		f := workflowCmd.Flags().Lookup(want.name)
		if f == nil {
			t.Errorf("缺少 --%s 选项", want.name)
			continue
		}
		if f.Shorthand != want.shorthand {
			t.Errorf("--%s 的简写应为 %q，实际 %q", want.name, want.shorthand, f.Shorthand)
		}
	}
}

func TestPrintWorkflowListing(t *testing.T) {
	old := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stdout = w
	printWorkflowListing(workflow.ListBundled())
	_ = w.Close()
	os.Stdout = old
	var buf bytes.Buffer
	if _, err := buf.ReadFrom(r); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	for _, id := range []string{"native", "tdd", "channel-driven-subagent-dispatch"} {
		if !strings.Contains(out, id) {
			t.Errorf("列表应包含 %q，实际输出：%s", id, out)
		}
	}
	if !strings.Contains(out, "内置") {
		t.Errorf("列表应标注内置，实际输出：%s", out)
	}
}

// chdirTempSunshine 建一个含 .sunshine/ 的临时项目并切进去，返回目录；
// 调用方负责用返回的 cleanup 恢复工作目录。
func chdirTempSunshine(t *testing.T) (string, func()) {
	t.Helper()
	cwd := t.TempDir()
	if err := os.MkdirAll(filepath.Join(cwd, ".sunshine"), 0o755); err != nil {
		t.Fatal(err)
	}
	oldWd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(cwd); err != nil {
		t.Fatal(err)
	}
	return cwd, func() { _ = os.Chdir(oldWd) }
}

func sha256Hex(s string) string {
	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:])
}

func TestWorkflowRunESwitchTemplate(t *testing.T) {
	resetWorkflowFlags()
	workflowTemplate = "tdd"
	defer resetWorkflowFlags()
	cwd, cleanup := chdirTempSunshine(t)
	defer cleanup()

	// 现有 workflow.md 与清单哈希一致 → “未改动→直接覆盖”。
	current := "# 简版工作流\n"
	if err := os.WriteFile(filepath.Join(cwd, ".sunshine", "workflow.md"), []byte(current), 0o644); err != nil {
		t.Fatal(err)
	}
	manifest := `{"__version": 2, "files": {".sunshine/workflow.md": "` + sha256Hex(current) + `"}}`
	if err := os.WriteFile(filepath.Join(cwd, ".sunshine", ".template-hashes.json"), []byte(manifest), 0o644); err != nil {
		t.Fatal(err)
	}

	var out bytes.Buffer
	workflowCmd.SetOut(&out)
	if err := workflowCmd.RunE(workflowCmd, nil); err != nil {
		t.Fatalf("切换模板失败：%v", err)
	}
	data, err := os.ReadFile(filepath.Join(cwd, ".sunshine", "workflow.md"))
	if err != nil {
		t.Fatal(err)
	}
	tdd, err := workflow.BundledContent("tdd")
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != tdd {
		t.Error("切换后 workflow.md 应为 tdd 模板内容")
	}
}

func TestWorkflowRunECreateNew(t *testing.T) {
	resetWorkflowFlags()
	workflowTemplate = "native"
	workflowCreateNew = true
	defer resetWorkflowFlags()
	cwd, cleanup := chdirTempSunshine(t)
	defer cleanup()

	current := "# 当前内容\n"
	if err := os.WriteFile(filepath.Join(cwd, ".sunshine", "workflow.md"), []byte(current), 0o644); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	workflowCmd.SetOut(&out)
	if err := workflowCmd.RunE(workflowCmd, nil); err != nil {
		t.Fatalf("-n 切换失败：%v", err)
	}
	data, _ := os.ReadFile(filepath.Join(cwd, ".sunshine", "workflow.md"))
	if string(data) != current {
		t.Error("-n 时不应改动 workflow.md")
	}
	if _, err := os.Stat(filepath.Join(cwd, ".sunshine", "workflow.md.new")); err != nil {
		t.Errorf("-n 时应写 workflow.md.new：%v", err)
	}
}

func TestWorkflowRunENoSunshineDir(t *testing.T) {
	resetWorkflowFlags()
	workflowList = true
	defer resetWorkflowFlags()
	cwd := t.TempDir() // 没有 .sunshine/
	oldWd, _ := os.Getwd()
	defer func() { _ = os.Chdir(oldWd) }()
	_ = os.Chdir(cwd)
	if err := workflowCmd.RunE(workflowCmd, nil); err == nil {
		t.Error("没有 .sunshine/ 时应报错")
	} else if !strings.Contains(err.Error(), "sunshine init") {
		t.Errorf("报错应提示先运行 sunshine init，实际：%v", err)
	}
}

func TestWorkflowRunENoTemplateNonTTY(t *testing.T) {
	resetWorkflowFlags()
	defer resetWorkflowFlags()
	_, cleanup := chdirTempSunshine(t)
	defer cleanup()
	// 未传 --template 且 stdin 不是 TTY（测试进程）时应报错。
	if err := workflowCmd.RunE(workflowCmd, nil); err == nil {
		t.Error("未传 --template 且非交互时应报错")
	} else if !strings.Contains(err.Error(), "--template") {
		t.Errorf("报错应提示 --template，实际：%v", err)
	}
}
