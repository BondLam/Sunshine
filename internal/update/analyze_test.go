package update

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestAnalyzeChangesTriState(t *testing.T) {
	dir := setupProject(t)
	// 模板集合：取真实模板中的三个文件。
	templates := map[string]string{
		".sunshine/workflow.md":                  readFile(t, dir, ".sunshine/workflow.md"),
		".sunshine/agents/check.md":              readFile(t, dir, ".sunshine/agents/check.md"),
		".claude/skills/sunshine-check/SKILL.md": readFile(t, dir, ".claude/skills/sunshine-check/SKILL.md"),
		".sunshine/agents/plan.md":               readFile(t, dir, ".sunshine/agents/plan.md"),
	}
	hashes := LoadHashes(dir)

	// 1. 未改动：全部 unchanged。
	changes := AnalyzeChanges(dir, hashes, templates, "")
	if len(changes.UnchangedFiles) != 4 || len(changes.AutoUpdateFiles) != 0 || len(changes.ChangedFiles) != 0 {
		t.Fatalf("未改动时应全部 unchanged，实际 %+v", changes)
	}

	// 2. 用户已改动 check.md（不碰清单）→ changedFiles。
	writeFile(t, dir, ".sunshine/agents/check.md", "用户改过的检查\n")
	changes = AnalyzeChanges(dir, hashes, templates, "")
	if len(changes.ChangedFiles) != 1 || changes.ChangedFiles[0].RelativePath != ".sunshine/agents/check.md" {
		t.Fatalf("改动后应进 changedFiles，实际 %+v", changes)
	}

	// 3. 模板已更新、用户未改动：把 plan.md 的清单哈希记为当前内容，
	// 再把模板换成新内容 → autoUpdateFiles。
	planContent := "新版计划\n"
	templates[".sunshine/agents/plan.md"] = planContent
	if err := UpdateHashes(dir, map[string]string{".sunshine/agents/plan.md": readFile(t, dir, ".sunshine/agents/plan.md")}); err != nil {
		t.Fatal(err)
	}
	hashes = LoadHashes(dir)
	changes = AnalyzeChanges(dir, hashes, templates, "")
	found := false
	for _, f := range changes.AutoUpdateFiles {
		if f.RelativePath == ".sunshine/agents/plan.md" {
			found = true
		}
	}
	if !found {
		t.Fatalf("模板更新且用户未改动时应进 autoUpdateFiles，实际 %+v", changes)
	}

	// 4. 新增文件（清单无记录、磁盘不存在）→ newFiles。
	templates[".sunshine/agents/new-one.md"] = "新 agent\n"
	changes = AnalyzeChanges(dir, hashes, templates, "")
	found = false
	for _, f := range changes.NewFiles {
		if f.RelativePath == ".sunshine/agents/new-one.md" {
			found = true
		}
	}
	if !found {
		t.Fatalf("新增文件应进 newFiles，实际 %+v", changes)
	}

	// 5. 用户已删除（清单有记录、磁盘不存在）→ userDeletedFiles。
	if err := os.Remove(filepath.Join(dir, ".claude", "skills", "sunshine-check", "SKILL.md")); err != nil {
		t.Fatal(err)
	}
	changes = AnalyzeChanges(dir, hashes, templates, "")
	found = false
	for _, f := range changes.UserDeletedFiles {
		if f.RelativePath == ".claude/skills/sunshine-check/SKILL.md" {
			found = true
		}
	}
	if !found {
		t.Fatalf("用户删除后应进 userDeletedFiles，实际 %+v", changes)
	}
}

func TestCollectUnchangedFileHashRepairs(t *testing.T) {
	dir := setupProject(t)
	templates := map[string]string{
		".sunshine/workflow.md": readFile(t, dir, ".sunshine/workflow.md"),
	}
	hashes := LoadHashes(dir)
	// 清单条目写错 → 修复。
	hashes[".sunshine/workflow.md"] = "错误的哈希"
	changes := AnalyzeChanges(dir, hashes, templates, "")
	repairs := CollectUnchangedFileHashRepairs(changes, hashes)
	if _, ok := repairs[".sunshine/workflow.md"]; !ok {
		t.Errorf("错误哈希应被修复，实际 %v", repairs)
	}
	// 清单缺失条目 → 补记（.gitignore 这类被排除的除外）。
	delete(hashes, ".sunshine/workflow.md")
	repairs = CollectUnchangedFileHashRepairs(changes, hashes)
	if _, ok := repairs[".sunshine/workflow.md"]; !ok {
		t.Errorf("缺失条目应补记，实际 %v", repairs)
	}
}

func TestPreserveExistingClaudeStatusLine(t *testing.T) {
	dir := setupProject(t)
	fresh := readFile(t, dir, ".claude/settings.json")
	if strings.Contains(fresh, "statusLine") {
		t.Fatal("模板 settings.json 本不应含 statusLine")
	}
	// 情形一：磁盘有 statusLine、新模板没有 → 带进新模板（追加到末尾）。
	existing := "{\n  \"env\": {\n    \"MY_VAR\": \"1\"\n  },\n  \"statusLine\": {\n    \"type\": \"command\",\n    \"command\": \"my-statusline\"\n  }\n}\n"
	writeFile(t, dir, ".claude/settings.json", existing)
	templates := map[string]string{ClaudeSettingsPath: fresh}
	PreserveExistingClaudeStatusLine(dir, templates)
	got := templates[ClaudeSettingsPath]
	if !strings.Contains(got, "my-statusline") {
		t.Errorf("已有 statusLine 应保留：\n%s", got)
	}
	if !strings.Contains(got, "SUNSHINE_HOOK_PLATFORM=claude sunshine hook session-start") {
		t.Errorf("模板 hooks 不应丢失：\n%s", got)
	}
	if !strings.HasSuffix(got, "}\n") {
		t.Errorf("结果应以 }\\n 结尾：\n%s", got)
	}
	// 情形二：磁盘没有 statusLine → 不动。
	writeFile(t, dir, ".claude/settings.json", fresh)
	templates = map[string]string{ClaudeSettingsPath: fresh}
	PreserveExistingClaudeStatusLine(dir, templates)
	if templates[ClaudeSettingsPath] != fresh {
		t.Error("无 statusLine 时不应改动模板")
	}
	// 情形三：新模板已有 statusLine → 不动（模板的胜出）。
	writeFile(t, dir, ".claude/settings.json", existing)
	withStatus := "{\n  \"statusLine\": {\n    \"type\": \"command\",\n    \"command\": \"template-statusline\"\n  }\n}\n"
	templates = map[string]string{ClaudeSettingsPath: withStatus}
	PreserveExistingClaudeStatusLine(dir, templates)
	if templates[ClaudeSettingsPath] != withStatus {
		t.Error("新模板已有 statusLine 时不应覆盖")
	}
	// 情形四：磁盘 JSON 非法 → 不动（走正常冲突流程）。
	writeFile(t, dir, ".claude/settings.json", "{ not json")
	templates = map[string]string{ClaudeSettingsPath: fresh}
	PreserveExistingClaudeStatusLine(dir, templates)
	if templates[ClaudeSettingsPath] != fresh {
		t.Error("磁盘 JSON 非法时不应改动模板")
	}
}
