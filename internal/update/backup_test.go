package update

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestManagedBlock(t *testing.T) {
	template := "前言\n\n<!-- SUNSHINE:START -->\n# 模板块\n<!-- SUNSHINE:END -->\n"
	existing := "用户前言\n\n<!-- SUNSHINE:START -->\n# 旧块\n<!-- SUNSHINE:END -->\n\n用户后记\n"
	// 取块。
	if got := GetManagedBlock(template, SunshineBlockStart, SunshineBlockEnd); !strings.Contains(got, "# 模板块") {
		t.Errorf("GetManagedBlock = %q", got)
	}
	if got := GetManagedBlock("无块", SunshineBlockStart, SunshineBlockEnd); got != "" {
		t.Errorf("无块时应返回空，实际 %q", got)
	}
	// 替换：只换块内，块外保留。
	replaced := ReplaceManagedBlockContent(existing, template, SunshineBlockStart, SunshineBlockEnd)
	if !strings.Contains(replaced, "# 模板块") || !strings.Contains(replaced, "用户前言") || !strings.Contains(replaced, "用户后记") {
		t.Errorf("ReplaceManagedBlockContent 错误：\n%s", replaced)
	}
	if strings.Contains(replaced, "# 旧块") {
		t.Errorf("旧块应被替换掉：\n%s", replaced)
	}
	// 合并：无块时追加到末尾。
	merged := MergeManagedBlockContent("只有用户内容\n", template, SunshineBlockStart, SunshineBlockEnd)
	if !strings.Contains(merged, "只有用户内容") || !strings.Contains(merged, "# 模板块") {
		t.Errorf("MergeManagedBlockContent（追加）错误：\n%s", merged)
	}
	// 合并：模板无块时返回模板全文。
	if got := MergeManagedBlockContent(existing, "模板全文\n", SunshineBlockStart, SunshineBlockEnd); got != "模板全文\n" {
		t.Errorf("模板无块时应返回模板全文，实际 %q", got)
	}
	// BuildManagedBlockTemplate：文件不存在时返回模板全文。
	dir := t.TempDir()
	if got := BuildManagedBlockTemplate(dir, "AGENTS.md", template, SunshineBlockStart, SunshineBlockEnd); got != template {
		t.Errorf("文件不存在时应返回模板全文")
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "AGENTS.md"), []byte(existing), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := BuildManagedBlockTemplate(dir, "AGENTS.md", template, SunshineBlockStart, SunshineBlockEnd); !strings.Contains(got, "用户后记") || !strings.Contains(got, "# 模板块") {
		t.Errorf("BuildManagedBlockTemplate 错误：\n%s", got)
	}
}

func TestIsSafeUntrackedCopilotInstructionsMerge(t *testing.T) {
	copilotTemplate := "用户区\n\n<!-- SUNSHINE:COPILOT-GUIDANCE:START -->\n# 指引\n<!-- SUNSHINE:COPILOT-GUIDANCE:END -->\n"
	existing := "我的说明\n"
	merged := MergeManagedBlockContent(existing, copilotTemplate, CopilotBlockStart, CopilotBlockEnd)
	if !IsSafeUntrackedCopilotInstructionsMerge(CopilotInstructionsPath, existing, merged, copilotTemplate) {
		t.Error("合并结果等于新内容时应判定为可安全合并")
	}
	if IsSafeUntrackedCopilotInstructionsMerge(CopilotInstructionsPath, existing, "别的内容\n", copilotTemplate) {
		t.Error("新内容不是合并结果时不应判定为可安全合并")
	}
	if IsSafeUntrackedCopilotInstructionsMerge("AGENTS.md", existing, merged, copilotTemplate) {
		t.Error("非 Copilot 路径不应判定")
	}
	// 已有内容已有块时不安全。
	withBlock := MergeManagedBlockContent(existing, copilotTemplate, CopilotBlockStart, CopilotBlockEnd)
	if IsSafeUntrackedCopilotInstructionsMerge(CopilotInstructionsPath, withBlock, merged, copilotTemplate) {
		t.Error("已有块时不应判定为可安全合并")
	}
	// Sunshine 暂无旧版未追踪的 AGENTS.md 块哈希。
	if IsKnownUntrackedTemplate("AGENTS.md", "<!-- SUNSHINE:START -->\n# 块\n<!-- SUNSHINE:END -->\n") {
		t.Error("legacy 集合为空时不应命中")
	}
}

func TestShouldExcludeFromBackup(t *testing.T) {
	for _, rel := range []string{".sunshine/workspace/a/journal-1.md", ".sunshine/tasks/t1/prd.md",
		".sunshine/spec/system.md", ".claude/worktrees/x/f", ".sunshine/.backups/2026/x",
		".sunshine/.backup-2026/x", ".claude/node_modules/x"} {
		if !ShouldExcludeFromBackup(rel) {
			t.Errorf("%s 应排除备份", rel)
		}
	}
	for _, rel := range []string{".sunshine/workflow.md", ".claude/settings.json", "AGENTS.md", ".sunshine/config.yaml"} {
		if ShouldExcludeFromBackup(rel) {
			t.Errorf("%s 不应排除备份", rel)
		}
	}
	for _, p := range ProtectedPaths {
		if !IsProtectedPath(p) || !IsProtectedPath(p+"/sub/file.md") {
			t.Errorf("%s 应受保护", p)
		}
	}
	if IsProtectedPath(".sunshine/agents/check.md") {
		t.Error("agents 不受保护")
	}
}

func TestCreateFullBackup(t *testing.T) {
	dir := setupProject(t)
	// 用户数据：不应进备份。
	writeFile(t, dir, ".sunshine/workspace/dev/journal-1.md", "日志\n")
	writeFile(t, dir, ".sunshine/tasks/t1/prd.md", "需求\n")
	writeFile(t, dir, ".sunshine/spec/system.md", "用户规范\n")
	// 被改过的模板文件：应进备份。
	orig := readFile(t, dir, ".sunshine/workflow.md")
	backupDir, err := CreateFullBackup(dir)
	if err != nil {
		t.Fatal(err)
	}
	if backupDir == "" {
		t.Fatal("有文件时应生成备份目录")
	}
	if !strings.HasPrefix(backupDir, filepath.Join(dir, ".sunshine", ".backups")+string(filepath.Separator)) {
		t.Errorf("备份目录应在 .sunshine/.backups/ 下，实际 %s", backupDir)
	}
	if got := readFile(t, backupDir, ".sunshine/workflow.md"); got != orig {
		t.Error("备份里的 workflow.md 与原内容不一致")
	}
	if _, err := os.Stat(filepath.Join(backupDir, ".sunshine", "workspace")); !os.IsNotExist(err) {
		t.Error("备份不应包含 workspace")
	}
	if _, err := os.Stat(filepath.Join(backupDir, ".sunshine", "spec")); !os.IsNotExist(err) {
		t.Error("备份不应包含 spec")
	}
	// 备份本身不进哈希清单。
	if ShouldExcludeFromHash(".sunshine/.backups/2026-01-01T00-00-00/.sunshine/workflow.md") {
		// 路径相对项目根时含 .backups/，应排除。
	} else {
		t.Error("备份目录应排除哈希追踪")
	}
	// 空项目（只有 .sunshine 空目录）不生成备份目录。
	empty := t.TempDir()
	if err := os.MkdirAll(filepath.Join(empty, ".sunshine"), 0o755); err != nil {
		t.Fatal(err)
	}
	if got, err := CreateFullBackup(empty); err != nil || got != "" {
		t.Errorf("无文件时应返回空，实际 %q, %v", got, err)
	}
}
