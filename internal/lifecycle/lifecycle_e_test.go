package lifecycle

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/BondLam/Sunshine/templates"
)

// writeTestFile 把 content 写到 dir 下的相对路径 rel（自动建目录）。
func writeTestFile(t *testing.T, dir, rel, content string) error {
	t.Helper()
	p := filepath.Join(dir, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return err
	}
	return os.WriteFile(p, []byte(content), 0o644)
}

// E 路清理器测试：codebuddy/droid 的 settings.json（嵌套 hooks 形）、
// copilot 的两份 hooks.json（扁平形）、copilot-instructions.md（托管块形）。
func TestScrubEPlatformSettings(t *testing.T) {
	for _, rel := range []string{"platforms/codebuddy/settings.json", "platforms/droid/settings.json"} {
		raw, err := templates.Platforms.ReadFile(rel)
		if err != nil {
			t.Fatal(err)
		}
		result := scrubHooksJSON(string(raw), "nested")
		if !result.FullyEmpty {
			t.Errorf("%s 清理后应判空（只有 hooks 键），实际：\n%s", rel, result.Content)
		}
		if strings.Contains(result.Content, "sunshine hook") {
			t.Errorf("%s 清理后不应残留 sunshine hook 命令", rel)
		}
	}
}

func TestScrubCopilotHooksJSON(t *testing.T) {
	raw, err := templates.Platforms.ReadFile("platforms/copilot/hooks.json")
	if err != nil {
		t.Fatal(err)
	}
	// 两份文件内容相同，清理器同一份逻辑覆盖。
	for _, rel := range []string{".github/copilot/hooks.json", ".github/hooks/sunshine.json"} {
		result := scrubHooksJSON(string(raw), "flat")
		if !result.FullyEmpty {
			t.Errorf("%s 清理后应判空（只有 hooks 键），实际：\n%s", rel, result.Content)
		}
		if strings.Contains(result.Content, "sunshine hook") {
			t.Errorf("%s 清理后不应残留 sunshine hook 命令", rel)
		}
	}
	// 用户自有 hook 条目保留（bash/powershell 双键形也不误删）。
	withUser := `{"hooks": {"SessionStart": [{"type": "command", "command": "echo hi", "timeout": 5}], "userPromptSubmitted": [{"type": "command", "bash": "SUNSHINE_HOOK_PLATFORM=copilot sunshine hook workflow-state", "powershell": "SUNSHINE_HOOK_PLATFORM=copilot sunshine hook workflow-state", "timeoutSec": 15}]}}`
	result := scrubHooksJSON(withUser, "flat")
	if result.FullyEmpty {
		t.Error("有用户 hook 条目时不应判空")
	}
	if !strings.Contains(result.Content, "echo hi") {
		t.Errorf("用户 hook 条目应保留，实际：\n%s", result.Content)
	}
	if strings.Contains(result.Content, "sunshine hook") {
		t.Error("Sunshine hook 条目应已被剥离")
	}
}

func TestScrubCopilotInstructions(t *testing.T) {
	// 块外用户内容保留，只剥离托管块。
	raw, err := templates.Platforms.ReadFile("platforms/copilot/copilot-instructions.md")
	if err != nil {
		t.Fatal(err)
	}
	mixed := "# 我的团队说明\n\n按本仓库规范评审。\n\n" + string(raw)
	result := scrubCopilotInstructions(mixed)
	if result.FullyEmpty {
		t.Error("块外有用户内容时不应判空")
	}
	if !strings.Contains(result.Content, "我的团队说明") {
		t.Errorf("用户内容应保留，实际：\n%s", result.Content)
	}
	if strings.Contains(result.Content, "SUNSHINE:COPILOT-GUIDANCE") {
		t.Error("托管块应已被剥离")
	}
	// 只有托管块时整文件删。
	only := scrubCopilotInstructions(string(raw))
	if !only.FullyEmpty {
		t.Errorf("只有托管块时应判空，实际：%q", only.Content)
	}
	// 无托管块时原样不动。
	plain := "# 用户自有说明\n"
	unchanged := scrubCopilotInstructions(plain)
	if unchanged.Content != plain || unchanged.FullyEmpty {
		t.Errorf("无托管块时应原样不动，实际 %+v", unchanged)
	}
}

// E 路结构化文件登记覆盖三家：BuildRemovalPlan 应把三家托管文件
// 认作结构化文件（清理后判空 → 删除），而不是直接删除含用户字段的文件。
func TestRemovalPlanCoversEPlatforms(t *testing.T) {
	dir := t.TempDir()
	// 直接写三家模板文件并记入清单，模拟已安装项目。
	for _, rel := range []string{
		".codebuddy/settings.json",
		".factory/settings.json",
		".github/copilot/hooks.json",
		".github/hooks/sunshine.json",
		".github/copilot-instructions.md",
	} {
		src := rel
		switch rel {
		case ".codebuddy/settings.json":
			src = "platforms/codebuddy/settings.json"
		case ".factory/settings.json":
			src = "platforms/droid/settings.json"
		case ".github/copilot/hooks.json", ".github/hooks/sunshine.json":
			src = "platforms/copilot/hooks.json"
		case ".github/copilot-instructions.md":
			src = "platforms/copilot/copilot-instructions.md"
		}
		raw, err := templates.Platforms.ReadFile(src)
		if err != nil {
			t.Fatal(err)
		}
		if err := writeTestFile(t, dir, rel, string(raw)); err != nil {
			t.Fatal(err)
		}
	}
	hashes := map[string]string{
		".codebuddy/settings.json":        "x",
		".factory/settings.json":          "x",
		".github/copilot/hooks.json":      "x",
		".github/hooks/sunshine.json":     "x",
		".github/copilot-instructions.md": "x",
	}
	plan, err := BuildRemovalPlan(dir, hashes, false)
	if err != nil {
		t.Fatalf("BuildRemovalPlan 失败：%v", err)
	}
	if len(plan.Deletions) != 5 {
		t.Errorf("三家 5 个托管文件应全部进入删除计划，实际删除 %d、修改 %d", len(plan.Deletions), len(plan.Modifications))
		for _, d := range plan.Deletions {
			t.Logf("  删除：%s", d.PosixPath)
		}
		for _, m := range plan.Modifications {
			t.Logf("  修改：%s", m.PosixPath)
		}
	}
}
