package workflow

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// writeManifest 在临时项目里写一份 .template-hashes.json。
func writeManifest(t *testing.T, cwd string, files map[string]string) {
	t.Helper()
	manifest, err := json.MarshalIndent(map[string]any{
		"__version": 2,
		"files":     files,
	}, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(cwd, ".sunshine"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(cwd, ".sunshine", ".template-hashes.json"), append(manifest, '\n'), 0o644); err != nil {
		t.Fatal(err)
	}
}

func hashOf(s string) string {
	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:])
}

func writeWorkflow(t *testing.T, cwd, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Join(cwd, ".sunshine"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(cwd, ".sunshine", "workflow.md"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestClassifyFiveStates(t *testing.T) {
	newContent := "# 新模板\n"

	// 缺失
	cwd := t.TempDir()
	if err := os.MkdirAll(filepath.Join(cwd, ".sunshine"), 0o755); err != nil {
		t.Fatal(err)
	}
	class, err := Classify(cwd, newContent)
	if err != nil || class != ClassMissing {
		t.Errorf("无文件时应为 ClassMissing，实际 %v, %v", class, err)
	}

	// 内容相同
	writeWorkflow(t, cwd, newContent)
	class, _ = Classify(cwd, newContent)
	if class != ClassIdentical {
		t.Errorf("内容相同时应为 ClassIdentical，实际 %v", class)
	}

	// 未改动（哈希一致）
	current := "# 当前工作流\n"
	writeWorkflow(t, cwd, current)
	writeManifest(t, cwd, map[string]string{WorkflowRelPath: hashOf(current)})
	class, _ = Classify(cwd, newContent)
	if class != ClassPristine {
		t.Errorf("哈希一致时应为 ClassPristine，实际 %v", class)
	}

	// 已改动（哈希有记录但不匹配）
	writeManifest(t, cwd, map[string]string{WorkflowRelPath: hashOf("# 旧内容\n")})
	class, _ = Classify(cwd, newContent)
	if class != ClassModified {
		t.Errorf("哈希不匹配时应为 ClassModified，实际 %v", class)
	}

	// 清单无记录（与上游一致：按已改动走交互确认，不自动写 .new）
	writeManifest(t, cwd, map[string]string{".sunshine/config.yaml": hashOf("x")})
	class, _ = Classify(cwd, newContent)
	if class != ClassModified {
		t.Errorf("清单无记录时应为 ClassModified，实际 %v", class)
	}

	// 清单不存在也一样
	cwd2 := t.TempDir()
	writeWorkflow(t, cwd2, current)
	class, _ = Classify(cwd2, newContent)
	if class != ClassModified {
		t.Errorf("清单不存在时应为 ClassModified，实际 %v", class)
	}
}

func TestApplyHashContract(t *testing.T) {
	cwd := t.TempDir()
	writeWorkflow(t, cwd, "# 原生\n")
	writeManifest(t, cwd, map[string]string{
		WorkflowRelPath:         hashOf("# 旧\n"),
		".sunshine/config.yaml": hashOf("c"),
		".claude/settings.json": hashOf("s"),
	})

	// native：刷新 workflow.md 的哈希条目，其余条目保留。
	if err := ApplyHashContract(cwd, "native"); err != nil {
		t.Fatalf("native 哈希登记失败：%v", err)
	}
	hashes, err := loadHashes(cwd)
	if err != nil {
		t.Fatal(err)
	}
	if hashes[WorkflowRelPath] != hashOf("# 原生\n") {
		t.Errorf("native 应刷新哈希条目，实际 %q", hashes[WorkflowRelPath])
	}
	if hashes[".sunshine/config.yaml"] != hashOf("c") || hashes[".claude/settings.json"] != hashOf("s") {
		t.Error("native 登记不应动其他条目")
	}

	// 非 native：删除 workflow.md 的条目，其余条目保留。
	if err := ApplyHashContract(cwd, "tdd"); err != nil {
		t.Fatalf("tdd 哈希登记失败：%v", err)
	}
	hashes, _ = loadHashes(cwd)
	if _, ok := hashes[WorkflowRelPath]; ok {
		t.Error("非 native 模板应删除 workflow.md 的哈希条目")
	}
	if hashes[".sunshine/config.yaml"] != hashOf("c") {
		t.Error("非 native 登记不应动其他条目")
	}
}

func TestRemoveHashEntryMissingManifest(t *testing.T) {
	cwd := t.TempDir()
	// 清单不存在时应静默成功（不新建空清单）。
	if err := removeHashEntry(cwd, WorkflowRelPath); err != nil {
		t.Errorf("清单不存在时删除应成功：%v", err)
	}
	if _, err := os.Stat(filepath.Join(cwd, ".sunshine", ".template-hashes.json")); !os.IsNotExist(err) {
		t.Error("清单不存在时删除不应新建清单")
	}
}
