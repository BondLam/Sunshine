package update

import (
	"bytes"
	"strings"
	"testing"

	"github.com/linrong1982/Sunshine/internal/platforms"
	"github.com/linrong1982/Sunshine/internal/scaffold"
)

// E 路：copilot-instructions.md 的 update 托管块合并。
// 项目已装 copilot，用户在块外写了自有内容；CollectTemplateFiles
// 收集的期望内容应保留用户文字、只更新托管块，不直接覆盖。
func TestCollectTemplateFilesMergesCopilotInstructions(t *testing.T) {
	dir := setupProject(t)
	files, err := platforms.Install(dir, "copilot", platforms.InstallOptions{})
	if err != nil {
		t.Fatalf("安装 copilot 失败：%v", err)
	}
	if err := scaffold.RecordHashes(dir, files); err != nil {
		t.Fatalf("记录哈希失败：%v", err)
	}
	writeFile(t, dir, ".github/copilot-instructions.md",
		"# 我的团队说明\n\n按本仓库规范评审。\n")
	var out bytes.Buffer
	collected, _, err := CollectTemplateFiles(&out, dir, LoadHashes(dir), CollectOptions{})
	if err != nil {
		t.Fatalf("CollectTemplateFiles 失败：%v", err)
	}
	got, ok := collected[CopilotInstructionsPath]
	if !ok {
		t.Fatal("模板集合应包含 .github/copilot-instructions.md")
	}
	if !strings.Contains(got, "我的团队说明") {
		t.Errorf("合并后应保留用户块外内容，实际：\n%s", got)
	}
	if !strings.Contains(got, CopilotBlockStart) || !strings.Contains(got, CopilotBlockEnd) {
		t.Errorf("合并后应含 Sunshine 托管块，实际：\n%s", got)
	}
}

// 未追踪的 copilot-instructions.md（清单无记录、磁盘已有、无托管块）：
// 期望内容是“已有内容 + 模板块”的合并结果时，AnalyzeChanges 应判为
// 自动更新（isSafeUntrackedCopilotInstructionsMerge 路径）。
func TestAnalyzeChangesCopilotUntrackedMerge(t *testing.T) {
	dir := setupProject(t)
	files, err := platforms.Install(dir, "copilot", platforms.InstallOptions{})
	if err != nil {
		t.Fatalf("安装 copilot 失败：%v", err)
	}
	if err := scaffold.RecordHashes(dir, files); err != nil {
		t.Fatalf("记录哈希失败：%v", err)
	}
	writeFile(t, dir, ".github/copilot-instructions.md",
		"# 我的团队说明\n\n按本仓库规范评审。\n")
	hashes := LoadHashes(dir)
	delete(hashes, CopilotInstructionsPath)
	var out bytes.Buffer
	collected, _, err := CollectTemplateFiles(&out, dir, hashes, CollectOptions{})
	if err != nil {
		t.Fatalf("CollectTemplateFiles 失败：%v", err)
	}
	changes := AnalyzeChanges(dir, hashes, collected, collected[CopilotInstructionsPath])
	found := false
	for _, f := range changes.AutoUpdateFiles {
		if f.RelativePath == CopilotInstructionsPath {
			found = true
		}
	}
	if !found {
		t.Errorf("未追踪的可安全合并文件应进自动更新，自动更新：%v，待确认：%v",
			pathsOf(changes.AutoUpdateFiles), pathsOf(changes.ChangedFiles))
	}
}

func pathsOf(files []FileChange) []string {
	var out []string
	for _, f := range files {
		out = append(out, f.RelativePath)
	}
	return out
}
