package update

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/linrong1982/Sunshine/internal/platforms"
	"github.com/linrong1982/Sunshine/internal/scaffold"
)

func TestConfiguredPlatforms(t *testing.T) {
	dir := setupProject(t)
	hashes := LoadHashes(dir)
	got := ConfiguredPlatforms(dir, hashes)
	if len(got) != 2 || got[0] != "claude-code" || got[1] != "cursor" {
		t.Errorf("已配置平台 = %v", got)
	}
	// 删掉 cursor 的清单条目 → 只剩 claude。
	for k := range hashes {
		if strings.HasPrefix(k, ".cursor/") {
			delete(hashes, k)
		}
	}
	got = ConfiguredPlatforms(dir, hashes)
	if len(got) != 1 || got[0] != "claude-code" {
		t.Errorf("删条目后已配置平台 = %v", got)
	}
}

func TestPruneOrphanManifestKeys(t *testing.T) {
	dir := setupProject(t)
	hashes := LoadHashes(dir)
	// 混入孤儿键：旧版误哈希的用户文件 + 用户自加 skill。
	hashes[".codex/sessions/abc.json"] = "deadbeef"
	hashes[".codex/skills/my-custom/SKILL.md"] = "deadbeef"
	hashes[".claude/skills/sunshine-custom/SKILL.md"] = "deadbeef"
	// 正常键：.sunshine 下的与平台模板键。
	configured := ConfiguredPlatforms(dir, hashes)
	prune, err := PruneOrphanManifestKeys(dir, configured, hashes, map[string]bool{}, true)
	if err != nil {
		t.Fatal(err)
	}
	if len(prune.Pruned) != 3 {
		t.Fatalf("应清理 3 条孤儿键，实际 %v", prune.Pruned)
	}
	for _, k := range []string{".sunshine/workflow.md", ".sunshine/config.yaml"} {
		if _, ok := prune.Hashes[k]; !ok {
			t.Errorf(".sunshine/ 条目 %s 应保留", k)
		}
	}
	// 持久化：磁盘清单里孤儿键已消失。
	after := LoadHashes(dir)
	for _, k := range []string{".codex/sessions/abc.json", ".codex/skills/my-custom/SKILL.md"} {
		if _, ok := after[k]; ok {
			t.Errorf("孤儿键 %s 应已从磁盘删除", k)
		}
	}
	// dry-run（persist=false）只计算不写盘。
	hashes2 := LoadHashes(dir)
	hashes2[".codex/sessions/xyz.json"] = "deadbeef"
	prune2, err := PruneOrphanManifestKeys(dir, configured, hashes2, map[string]bool{}, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(prune2.Pruned) != 1 {
		t.Fatalf("dry-run 应算出 1 条，实际 %v", prune2.Pruned)
	}
	if _, ok := LoadHashes(dir)[".codex/sessions/xyz.json"]; ok {
		t.Error("dry-run 不应写盘")
	}
	// 根级 AGENTS.md：无托管块时删键保文件；有块时保留。
	writeFile(t, dir, "AGENTS.md", "# 我的说明\n")
	hashes3 := map[string]string{"AGENTS.md": "x", ".sunshine/workflow.md": "y"}
	prune3, err := PruneOrphanManifestKeys(dir, configured, hashes3, map[string]bool{}, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(prune3.Pruned) != 1 || prune3.Pruned[0] != "AGENTS.md" {
		t.Errorf("无块的 AGENTS.md 应删键，实际 %v", prune3.Pruned)
	}
	if _, err := os.ReadFile(filepath.Join(dir, "AGENTS.md")); err != nil {
		t.Error("删键不应删除用户文件本身")
	}
	writeFile(t, dir, "AGENTS.md", "<!-- SUNSHINE:START -->\n# 块\n<!-- SUNSHINE:END -->\n")
	prune4, err := PruneOrphanManifestKeys(dir, configured, hashes3, map[string]bool{}, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(prune4.Pruned) != 0 {
		t.Errorf("有块的 AGENTS.md 应保留，实际 %v", prune4.Pruned)
	}
	// 迁移引用的路径保留。
	migPaths := map[string]bool{".codex/sessions/abc.json": true}
	prune5, err := PruneOrphanManifestKeys(dir, configured, map[string]string{".codex/sessions/abc.json": "x"}, migPaths, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(prune5.Pruned) != 0 {
		t.Errorf("迁移引用的路径应保留，实际 %v", prune5.Pruned)
	}
}

func TestNeedsCodexUpgrade(t *testing.T) {
	// 情形一：正常三平台项目 → 不需要。
	dir := setupProject(t)
	if NeedsCodexUpgrade(dir, LoadHashes(dir)) {
		t.Error("正常项目不应触发 Codex 升级")
	}
	// 情形二：旧版布局（清单有 .agents/skills 标记、无 .codex/）→ 需要。
	legacy := t.TempDir()
	if err := scaffold.Init(legacy, "测试", false); err != nil {
		t.Fatal(err)
	}
	hashes := map[string]string{
		".agents/skills/sunshine-continue/SKILL.md":    "a",
		".agents/skills/sunshine-finish-work/SKILL.md": "b",
		".sunshine/workflow.md":                        "c",
	}
	if !NeedsCodexUpgrade(legacy, hashes) {
		t.Error("旧版布局应触发 Codex 升级")
	}
	// 情形三：已有 .codex/ → 不需要。
	if err := os.MkdirAll(filepath.Join(legacy, ".codex"), 0o755); err != nil {
		t.Fatal(err)
	}
	if NeedsCodexUpgrade(legacy, hashes) {
		t.Error("已有 .codex/ 时不应触发")
	}
	// 情形四：无旧版标记 → 不需要。
	if err := os.RemoveAll(filepath.Join(legacy, ".codex")); err != nil {
		t.Fatal(err)
	}
	if NeedsCodexUpgrade(legacy, map[string]string{".sunshine/workflow.md": "c"}) {
		t.Error("无旧版标记时不应触发")
	}
	// 情形五：RenderTemplates 可用（codex 模板含标记键）。
	files, err := platforms.RenderTemplates(dir, "codex", platforms.InstallOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := files[".codex/hooks.json"]; !ok {
		t.Error("codex 模板应含 .codex/hooks.json")
	}
	if !strings.Contains(files[".codex/hooks.json"], "sunshine hook session-start") {
		t.Error("codex hooks.json 模板应含 SessionStart 接线")
	}
}
