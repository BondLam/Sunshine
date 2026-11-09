package migrate

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// ---------- 测试辅助 ----------

func writeTestFile(t *testing.T, root, rel, content string) string {
	t.Helper()
	full := filepath.Join(root, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return full
}

func readTestFile(t *testing.T, root, rel string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(rel)))
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func fileExists(t *testing.T, root, rel string) bool {
	t.Helper()
	_, err := os.Stat(filepath.Join(root, filepath.FromSlash(rel)))
	return err == nil
}

// newTestProject 建一个带哈希清单的临时项目目录；hashes 的键是相对项目根
// 的 POSIX 路径，值是 ComputeHash(content)。
func newTestProject(t *testing.T, files map[string]string) string {
	t.Helper()
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, ".sunshine"), 0o755); err != nil {
		t.Fatal(err)
	}
	hashes := map[string]string{}
	for rel, content := range files {
		writeTestFile(t, root, rel, content)
		hashes[rel] = ComputeHash([]byte(content))
	}
	if err := SaveHashes(root, hashes); err != nil {
		t.Fatal(err)
	}
	return root
}

// writeManifests 把自造清单写进临时目录（文件名 <版本>.json）。
func writeManifests(t *testing.T, dir string, manifests ...MigrationManifest) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, m := range manifests {
		data, err := json.MarshalIndent(m, "", "  ")
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, m.Version+".json"), data, 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

func froms(items []MigrationItem) []string {
	var out []string
	for _, m := range items {
		out = append(out, m.From)
	}
	return out
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// ---------- 版本比较 ----------

func TestCompareVersions(t *testing.T) {
	cases := []struct {
		a, b string
		want int
	}{
		{"0.1.0", "0.2.0", -1},
		{"0.2.0", "0.2.0", 0},
		{"0.7.0-beta.3", "0.7.0", -1},
		{"0.7.0-beta.3", "0.7.0-beta.10", -1}, // 预发布数字段按数值比
		{"0.7.0-beta.10", "0.7.0-beta.3", 1},
		{"0.2", "0.2.0", 0},
		{"1.0.0", "0.9.9", 1},
		{"v0.2.0", "0.2.0", 0},
		{"xxx", "0.2.0", 0}, // 无法解析视为相等，不误判
	}
	for _, c := range cases {
		got := CompareVersions(c.a, c.b)
		sign := 0
		if got < 0 {
			sign = -1
		} else if got > 0 {
			sign = 1
		}
		if sign != c.want {
			t.Errorf("CompareVersions(%q, %q) = %d，想要符号 %d", c.a, c.b, got, c.want)
		}
	}
}

// ---------- 清单加载与区间筛选 ----------

func TestRegistryStartsEmpty(t *testing.T) {
	r := NewRegistry()
	if len(r.Versions()) != 0 || len(r.AllMigrations()) != 0 {
		t.Errorf("Sunshine 清单集初始应为空，得到版本 %v", r.Versions())
	}
	if r.HasPendingMigrations("0.1.0", "0.2.0") {
		t.Error("空清单集不应有待迁移项")
	}
}

func TestRegistryMigrationsForVersion(t *testing.T) {
	dir := t.TempDir()
	writeManifests(t, dir,
		MigrationManifest{Version: "0.1.0", Migrations: []MigrationItem{{Type: "delete", From: ".sunshine/old-a.md"}}},
		MigrationManifest{Version: "0.2.0", Breaking: true, RecommendMigrate: true, Changelog: "改名",
			Migrations: []MigrationItem{
				{Type: "rename", From: ".sunshine/commands/old.md", To: ".sunshine/commands/new.md"},
				{Type: "safe-file-delete", From: ".sunshine/commands/dead.md", AllowedHashes: []string{"abc"}},
			}},
		MigrationManifest{Version: "0.3.0", Migrations: []MigrationItem{{Type: "delete", From: ".sunshine/old-b.md"}}},
	)
	r, warnings := LoadManifestDir(dir)
	if len(warnings) != 0 {
		t.Fatalf("加载警告：%v", warnings)
	}
	// 区间 (from, to]：含 0.2.0，不含 0.1.0 与 0.3.0。
	got := r.MigrationsForVersion("0.1.0", "0.2.0")
	if !equalStrings(froms(got), []string{".sunshine/commands/old.md", ".sunshine/commands/dead.md"}) {
		t.Errorf("区间筛选错误：%v", froms(got))
	}
	if got := r.MigrationsForVersion("0.2.0", "0.2.0"); len(got) != 0 {
		t.Errorf("同版本不应有迁移项：%v", froms(got))
	}
	meta := r.Metadata("0.1.0", "0.2.0")
	if !meta.Breaking || !meta.RecommendMigrate || len(meta.Changelog) != 1 {
		t.Errorf("汇总元数据错误：%+v", meta)
	}
	meta = r.Metadata("0.2.0", "0.3.0")
	if meta.Breaking || meta.RecommendMigrate {
		t.Errorf("0.3.0 不应是破坏性版本：%+v", meta)
	}
	if !equalStrings(r.Versions(), []string{"0.1.0", "0.2.0", "0.3.0"}) {
		t.Errorf("版本排序错误：%v", r.Versions())
	}
}

func TestLoadManifestDirSkipsInvalid(t *testing.T) {
	dir := t.TempDir()
	writeManifests(t, dir, MigrationManifest{Version: "0.2.0", Migrations: []MigrationItem{{Type: "delete", From: ".sunshine/x.md"}}})
	writeTestFile(t, dir, "bad.json", "{ 不是合法 JSON")
	writeTestFile(t, dir, "no-version.json", `{"migrations": []}`)
	r, warnings := LoadManifestDir(dir)
	if len(warnings) != 2 {
		t.Errorf("应有 2 条加载警告，得到 %d：%v", len(warnings), warnings)
	}
	if !equalStrings(r.Versions(), []string{"0.2.0"}) {
		t.Errorf("损坏清单应跳过，得到版本 %v", r.Versions())
	}
}

func TestLoadManifestDirMissingIsEmpty(t *testing.T) {
	r, warnings := LoadManifestDir(filepath.Join(t.TempDir(), "不存在"))
	if len(warnings) != 0 || len(r.Versions()) != 0 {
		t.Errorf("缺失目录应返回空清单集无警告，得到 %v / %v", r.Versions(), warnings)
	}
}

func TestFindOrphanMigrations(t *testing.T) {
	root := newTestProject(t, map[string]string{".sunshine/commands/old.md": "内容"})
	all := []MigrationItem{
		{Type: "rename", From: ".sunshine/commands/old.md", To: ".sunshine/commands/new.md"},
		{Type: "rename", From: ".sunshine/commands/gone.md", To: ".sunshine/commands/new2.md"},
		{Type: "delete", From: ".sunshine/commands/old.md"},
	}
	orphans := FindOrphanMigrations(root, all, nil)
	if len(orphans) != 1 || orphans[0].From != ".sunshine/commands/old.md" {
		t.Errorf("孤儿检测错误：%v", froms(orphans))
	}
	// 已在 pending 里则不再算孤儿。
	orphans = FindOrphanMigrations(root, all, []MigrationItem{{Type: "rename", From: ".sunshine/commands/old.md", To: ".sunshine/commands/new.md"}})
	if len(orphans) != 0 {
		t.Errorf("已在 pending 里的项不应算孤儿：%v", froms(orphans))
	}
}

// ---------- 排序 ----------

func TestSortMigrationsForExecution(t *testing.T) {
	items := []MigrationItem{
		{Type: "rename", From: ".sunshine/a.md", To: ".sunshine/b.md"},
		{Type: "rename-dir", From: ".sunshine/skills", To: ".sunshine/skills-new"},
		{Type: "delete", From: ".sunshine/c.md"},
		{Type: "rename-dir", From: ".sunshine/skills/nested", To: ".sunshine/skills-new/nested"},
	}
	sorted := SortMigrationsForExecution(items)
	want := []string{".sunshine/skills/nested", ".sunshine/skills", ".sunshine/a.md", ".sunshine/c.md"}
	if !equalStrings(froms(sorted), want) {
		t.Errorf("排序错误：得到 %v，想要 %v", froms(sorted), want)
	}
	if !equalStrings(froms(items), []string{".sunshine/a.md", ".sunshine/skills", ".sunshine/c.md", ".sunshine/skills/nested"}) {
		t.Error("排序不得改动原切片")
	}
}

// ---------- 分类 ----------

func TestClassifyMigrations(t *testing.T) {
	root := newTestProject(t, map[string]string{
		".sunshine/commands/old.md":      "模板内容",
		".sunshine/commands/changed.md":  "模板内容",
		".sunshine/commands/conflict.md": "模板内容",
		".sunshine/commands/target.md":   "用户改过的目标",
		".sunshine/old-dir/a.md":         "目录文件",
		".sunshine/workspace/notes.md":   "用户数据",
		".sunshine/delete-me.md":         "待删",
		".sunshine/delete-changed.md":    "待删改过",
		".sunshine/stale-cmd.md":         "过期命令",
	})
	// plain-dir 是用户自己的同名目录：刻意不进哈希清单，rename-dir 应跳过。
	writeTestFile(t, root, ".sunshine/plain-dir/b.md", "用户目录文件")
	// 改动两个文件（哈希与清单不一致）。
	writeTestFile(t, root, ".sunshine/commands/changed.md", "用户改过")
	writeTestFile(t, root, ".sunshine/delete-changed.md", "用户改过")
	hashes, err := LoadHashes(root)
	if err != nil {
		t.Fatal(err)
	}
	templates := map[string]string{
		".sunshine/commands/target.md": "模板目标内容", // 与磁盘内容不同 → 冲突
	}
	items := []MigrationItem{
		{Type: "rename", From: ".sunshine/commands/old.md", To: ".sunshine/commands/new.md"},
		{Type: "rename", From: ".sunshine/commands/changed.md", To: ".sunshine/commands/changed-new.md"},
		{Type: "rename", From: ".sunshine/commands/conflict.md", To: ".sunshine/commands/target.md"},
		{Type: "rename", From: ".sunshine/commands/missing.md", To: ".sunshine/commands/x.md"},
		{Type: "rename-dir", From: ".sunshine/old-dir", To: ".sunshine/new-dir"},
		{Type: "rename-dir", From: ".sunshine/plain-dir", To: ".sunshine/plain-new"},
		{Type: "rename", From: ".sunshine/workspace/notes.md", To: ".sunshine/workspace/notes-new.md"},
		{Type: "delete", From: ".sunshine/delete-me.md"},
		{Type: "delete", From: ".sunshine/delete-changed.md"},
		{Type: "safe-file-delete", From: ".sunshine/stale-cmd.md", AllowedHashes: []string{ComputeHash([]byte("过期命令"))}},
		{Type: "bogus", From: ".sunshine/commands/old.md"},
	}
	c := ClassifyMigrations(items, root, hashes, templates)
	if !equalStrings(froms(c.Auto), []string{".sunshine/commands/old.md", ".sunshine/old-dir", ".sunshine/delete-me.md"}) {
		t.Errorf("auto 分类错误：%v", froms(c.Auto))
	}
	if !equalStrings(froms(c.Confirm), []string{".sunshine/commands/changed.md", ".sunshine/delete-changed.md"}) {
		t.Errorf("confirm 分类错误：%v", froms(c.Confirm))
	}
	if !equalStrings(froms(c.Conflict), []string{".sunshine/commands/conflict.md"}) {
		t.Errorf("conflict 分类错误：%v", froms(c.Conflict))
	}
	wantSkip := []string{".sunshine/commands/missing.md", ".sunshine/plain-dir", ".sunshine/workspace/notes.md", ".sunshine/commands/old.md"}
	if !equalStrings(froms(c.Skip), wantSkip) {
		t.Errorf("skip 分类错误：得到 %v，想要 %v", froms(c.Skip), wantSkip)
	}
}

func TestIsTemplateModified(t *testing.T) {
	root := newTestProject(t, map[string]string{".sunshine/a.md": "原文"})
	hashes, _ := LoadHashes(root)
	if IsTemplateModified(root, ".sunshine/a.md", hashes) {
		t.Error("内容与哈希一致时不应判为已改动")
	}
	writeTestFile(t, root, ".sunshine/a.md", "改过")
	if !IsTemplateModified(root, ".sunshine/a.md", hashes) {
		t.Error("内容改动后应判为已改动")
	}
	writeTestFile(t, root, ".sunshine/untracked.md", "没进清单的文件")
	if !IsTemplateModified(root, ".sunshine/untracked.md", hashes) {
		t.Error("清单无记录时应按保守策略判为已改动")
	}
	if IsTemplateModified(root, ".sunshine/missing.md", hashes) {
		t.Error("文件不存在时不应判为已改动")
	}
}

// ---------- safe-file-delete ----------

func TestCollectSafeFileDeletes(t *testing.T) {
	staleContent := "过期命令原文"
	root := newTestProject(t, map[string]string{
		".sunshine/commands/stale.md":    staleContent,
		".sunshine/commands/changed.md":  "模板原文",
		".sunshine/workspace/keep.md":    "用户数据",
		".sunshine/commands/skipped.md":  "跳过列表里的文件",
		".sunshine/commands/restored.md": "又恢复成模板了",
		".sunshine/commands/nohash.md":   "没有哈希列表",
	})
	writeTestFile(t, root, ".sunshine/commands/changed.md", "用户改过")
	staleHash := ComputeHash([]byte(staleContent))
	items := []MigrationItem{
		{Type: "safe-file-delete", From: ".sunshine/commands/stale.md", AllowedHashes: []string{staleHash}},
		{Type: "safe-file-delete", From: ".sunshine/commands/changed.md", AllowedHashes: []string{ComputeHash([]byte("模板原文"))}},
		{Type: "safe-file-delete", From: ".sunshine/commands/missing.md", AllowedHashes: []string{"x"}},
		{Type: "safe-file-delete", From: ".sunshine/workspace/keep.md", AllowedHashes: []string{ComputeHash([]byte("用户数据"))}},
		{Type: "safe-file-delete", From: ".sunshine/commands/skipped.md", AllowedHashes: []string{ComputeHash([]byte("跳过列表里的文件"))}},
		{Type: "safe-file-delete", From: ".sunshine/commands/restored.md", AllowedHashes: []string{ComputeHash([]byte("又恢复成模板了"))}},
		{Type: "safe-file-delete", From: ".sunshine/commands/nohash.md"},
		{Type: "rename", From: ".sunshine/commands/stale.md", To: ".sunshine/commands/new.md"},
	}
	currentTemplates := map[string]bool{".sunshine/commands/restored.md": true}
	got := CollectSafeFileDeletes(items, root, []string{".sunshine/commands/skipped.md"}, currentTemplates, false)
	actions := map[string]SafeFileDeleteAction{}
	for _, c := range got {
		actions[c.Item.From] = c.Action
	}
	want := map[string]SafeFileDeleteAction{
		".sunshine/commands/stale.md":   SafeDeleteDelete,
		".sunshine/commands/changed.md": SafeDeleteSkipModified,
		".sunshine/commands/missing.md": SafeDeleteSkipMissing,
		".sunshine/workspace/keep.md":   SafeDeleteSkipProtected,
		".sunshine/commands/skipped.md": SafeDeleteSkipUpdateSkip,
		".sunshine/commands/nohash.md":  SafeDeleteSkipModified,
	}
	if len(got) != len(want) {
		t.Fatalf("应收集 %d 项（含当前模板恢复的 1 项被排除），得到 %d：%v", len(want), len(got), actions)
	}
	for from, wantAction := range want {
		if actions[from] != wantAction {
			t.Errorf("%s：得到 %s，想要 %s", from, actions[from], wantAction)
		}
	}
	// 破坏性版本绕过 update.skip：skipped.md 应变为可删。
	got = CollectSafeFileDeletes(items, root, []string{".sunshine/commands/skipped.md"}, currentTemplates, true)
	for _, c := range got {
		if c.Item.From == ".sunshine/commands/skipped.md" && c.Action != SafeDeleteDelete {
			t.Errorf("绕过 update.skip 后应为 delete，得到 %s", c.Action)
		}
	}
}

func TestExecuteSafeFileDeletes(t *testing.T) {
	root := newTestProject(t, map[string]string{
		".sunshine/commands/stale.md": "过期",
		".sunshine/commands/keep.md":  "保留",
	})
	hashes, _ := LoadHashes(root)
	items := []MigrationItem{
		{Type: "safe-file-delete", From: ".sunshine/commands/stale.md", AllowedHashes: []string{ComputeHash([]byte("过期"))}},
		{Type: "safe-file-delete", From: ".sunshine/commands/keep.md", AllowedHashes: []string{"不匹配"}},
	}
	classified := CollectSafeFileDeletes(items, root, nil, nil, false)
	deleted, errs := ExecuteSafeFileDeletes(classified, root)
	if len(errs) != 0 || deleted != 1 {
		t.Fatalf("应删除 1 个且无错误，得到 deleted=%d errs=%v", deleted, errs)
	}
	if fileExists(t, root, ".sunshine/commands/stale.md") {
		t.Error("过期文件应已被删除")
	}
	if !fileExists(t, root, ".sunshine/commands/keep.md") {
		t.Error("哈希不匹配的文件应保留")
	}
	hashes, _ = LoadHashes(root)
	if _, ok := hashes[".sunshine/commands/stale.md"]; ok {
		t.Error("已删文件的哈希条目应从清单移除")
	}
	if _, ok := hashes[".sunshine/commands/keep.md"]; !ok {
		t.Error("保留文件的哈希条目应还在")
	}
}

// ---------- 执行 ----------

func TestExecuteMigrationsAuto(t *testing.T) {
	root := newTestProject(t, map[string]string{
		".sunshine/commands/old.md": "旧命令",
		".sunshine/old-dir/a.md":    "目录文件",
		".sunshine/delete-me.md":    "待删",
	})
	hashes, _ := LoadHashes(root)
	items := []MigrationItem{
		{Type: "rename", From: ".sunshine/commands/old.md", To: ".sunshine/commands/new.md"},
		{Type: "rename-dir", From: ".sunshine/old-dir", To: ".sunshine/new-dir"},
		{Type: "delete", From: ".sunshine/delete-me.md"},
	}
	classified := ClassifyMigrations(items, root, hashes, nil)
	result, err := ExecuteMigrations(classified, root, ExecuteOptions{})
	if err != nil {
		t.Fatalf("执行失败：%v", err)
	}
	if result.Renamed != 2 || result.Deleted != 1 || result.Skipped != 0 {
		t.Errorf("执行结果错误：%+v", result)
	}
	if !fileExists(t, root, ".sunshine/commands/new.md") || fileExists(t, root, ".sunshine/commands/old.md") {
		t.Error("rename 未生效")
	}
	if !fileExists(t, root, ".sunshine/new-dir/a.md") || fileExists(t, root, ".sunshine/old-dir/a.md") {
		t.Error("rename-dir 未生效")
	}
	if fileExists(t, root, ".sunshine/delete-me.md") {
		t.Error("delete 未生效")
	}
	hashes, _ = LoadHashes(root)
	if _, ok := hashes[".sunshine/commands/new.md"]; !ok {
		t.Error("rename 后哈希键应更新为新路径")
	}
	if _, ok := hashes[".sunshine/new-dir/a.md"]; !ok {
		t.Error("rename-dir 后哈希键应批量改前缀")
	}
	if _, ok := hashes[".sunshine/delete-me.md"]; ok {
		t.Error("delete 后哈希键应移除")
	}
}

func TestExecuteMigrationsConfirmBackupAndForceSkip(t *testing.T) {
	newRoot := func() string {
		return newTestProject(t, map[string]string{".sunshine/commands/changed.md": "模板原文"})
	}
	hashesOf := func(root string) map[string]string {
		h, err := LoadHashes(root)
		if err != nil {
			t.Fatal(err)
		}
		return h
	}
	item := MigrationItem{Type: "rename", From: ".sunshine/commands/changed.md", To: ".sunshine/commands/new.md"}

	// 交互选“备份后执行”：新路径有内容，旧内容另存为 .backup。
	root := newRoot()
	writeTestFile(t, root, ".sunshine/commands/changed.md", "用户改过")
	classified := ClassifyMigrations([]MigrationItem{item}, root, hashesOf(root), nil)
	if len(classified.Confirm) != 1 {
		t.Fatalf("改过的文件应进 confirm，得到 %+v", classified)
	}
	backupPrompter := func(m MigrationItem) (MigrationAction, error) { return ActionBackupRename, nil }
	result, err := ExecuteMigrations(classified, root, ExecuteOptions{Prompter: backupPrompter})
	if err != nil || result.Renamed != 1 {
		t.Fatalf("备份执行失败：result=%+v err=%v", result, err)
	}
	if readTestFile(t, root, ".sunshine/commands/new.md") != "用户改过" {
		t.Error("改动内容应随文件搬走")
	}
	if readTestFile(t, root, ".sunshine/commands/new.md.backup") != "用户改过" {
		t.Error("应写出 .backup 备份")
	}

	// --force：不确认直接执行。
	root = newRoot()
	writeTestFile(t, root, ".sunshine/commands/changed.md", "用户改过")
	classified = ClassifyMigrations([]MigrationItem{item}, root, hashesOf(root), nil)
	result, err = ExecuteMigrations(classified, root, ExecuteOptions{Force: true})
	if err != nil || result.Renamed != 1 || fileExists(t, root, ".sunshine/commands/new.md.backup") {
		t.Errorf("force 应直接执行且不写 .backup：%+v err=%v", result, err)
	}

	// --skip-all：跳过已改动文件。
	root = newRoot()
	writeTestFile(t, root, ".sunshine/commands/changed.md", "用户改过")
	classified = ClassifyMigrations([]MigrationItem{item}, root, hashesOf(root), nil)
	result, err = ExecuteMigrations(classified, root, ExecuteOptions{SkipAll: true})
	if err != nil || result.Renamed != 0 || result.Skipped != 1 {
		t.Errorf("skip-all 应跳过：%+v err=%v", result, err)
	}
	if !fileExists(t, root, ".sunshine/commands/changed.md") {
		t.Error("skip-all 后原文件应保留")
	}

	// 交互确认失败（非交互终端）：按跳过处理并记录错误。
	root = newRoot()
	writeTestFile(t, root, ".sunshine/commands/changed.md", "用户改过")
	classified = ClassifyMigrations([]MigrationItem{item}, root, hashesOf(root), nil)
	failPrompter := func(m MigrationItem) (MigrationAction, error) { return ActionSkip, errPromptEOF }
	result, err = ExecuteMigrations(classified, root, ExecuteOptions{Prompter: failPrompter})
	if err == nil || result.Skipped != 1 || len(result.Errors) != 1 {
		t.Errorf("确认失败应跳过并返回错误：result=%+v err=%v", result, err)
	}
}

var errPromptEOF = errTestPrompt

type testPromptError string

func (e testPromptError) Error() string { return string(e) }

const errTestPrompt = testPromptError("模拟确认失败")

func TestExecuteMigrationsRenameDirTargetIsCurrentTemplate(t *testing.T) {
	root := newTestProject(t, map[string]string{
		".sunshine/skills-old/a.md": "旧内容",
		".sunshine/skills/a.md":     "新版标准内容",
	})
	hashes, _ := LoadHashes(root)
	item := MigrationItem{Type: "rename-dir", From: ".sunshine/skills-old", To: ".sunshine/skills"}
	classified := ClassifyMigrations([]MigrationItem{item}, root, hashes, map[string]string{".sunshine/skills/a.md": "新版标准内容"})
	if len(classified.Auto) != 1 {
		t.Fatalf("目标只含未改动模板文件时应自动迁移，得到 %+v", classified)
	}
	result, err := ExecuteMigrations(classified, root, ExecuteOptions{Templates: map[string]string{".sunshine/skills/a.md": "新版标准内容"}})
	if err != nil {
		t.Fatalf("执行失败：%v", err)
	}
	if result.Deleted != 1 {
		t.Errorf("目标已是新版标准内容时应删源目录计 deleted，得到 %+v", result)
	}
	if readTestFile(t, root, ".sunshine/skills/a.md") != "新版标准内容" {
		t.Error("目标的新版标准内容不得被旧内容覆盖")
	}
	if fileExists(t, root, ".sunshine/skills-old/a.md") {
		t.Error("冗余源目录应被删除")
	}
}

// ---------- 硬停判定 ----------

func TestHardStopGate(t *testing.T) {
	breaking := MigrationMetadata{Breaking: true, RecommendMigrate: true}
	plain := MigrationMetadata{}
	classified := ClassifiedMigrations{Auto: []MigrationItem{{Type: "rename", From: "a", To: "b"}}}
	empty := ClassifiedMigrations{}
	if !ShouldHardStop(classified, breaking, false, false, true, true) {
		t.Error("破坏性待迁移项 + 无 --migrate 时应硬停")
	}
	for name, tc := range map[string]struct {
		meta         MigrationMetadata
		classified   ClassifiedMigrations
		migrate      bool
		dryRun       bool
		isUpgrade    bool
		knownVersion bool
	}{
		"传了 --migrate":  {breaking, classified, true, false, true, true},
		"--dry-run 只预览": {breaking, classified, false, true, true, true},
		"降级不硬停":         {breaking, classified, false, false, false, true},
		"未知项目版不硬停":      {breaking, classified, false, false, true, false},
		"非破坏性版本不硬停":     {plain, classified, false, false, true, true},
		"无待迁移项不硬停":      {breaking, empty, false, false, true, true},
	} {
		if ShouldHardStop(tc.classified, tc.meta, tc.migrate, tc.dryRun, tc.isUpgrade, tc.knownVersion) {
			t.Errorf("%s：不应硬停", name)
		}
	}
	msg := HardStopMessage("0.1.0", "0.2.0")
	if !strings.Contains(msg, "sunshine update --migrate") {
		t.Errorf("硬停提示须包含 sunshine update --migrate，得到：%s", msg)
	}
	if hint := MigrateHint(empty); hint != "" {
		t.Errorf("无待迁移项时不应给提示，得到：%s", hint)
	}
	if hint := MigrateHint(classified); !strings.Contains(hint, "--migrate") {
		t.Errorf("轻提示应提到 --migrate，得到：%s", hint)
	}
}

// ---------- 配置段落追加 ----------

const testConfigTemplate = `# 配置文件
#-------------------------------------------------------------------------------
# 会话记录
#-------------------------------------------------------------------------------

session_commit_message: "chore: record journal"

#-------------------------------------------------------------------------------
# Codex 配置
#-------------------------------------------------------------------------------

# Codex 相关配置。
codex:
  enabled: true

#-------------------------------------------------------------------------------
# 其他配置
#-------------------------------------------------------------------------------

other: 1
`

func TestExtractConfigSection(t *testing.T) {
	section := ExtractConfigSection(testConfigTemplate, "Codex 配置")
	if !strings.Contains(section, "codex:") || !strings.Contains(section, "# Codex 配置") {
		t.Errorf("提取段落错误：%q", section)
	}
	if strings.Contains(section, "其他配置") {
		t.Errorf("段落不应越过下一分隔块：%q", section)
	}
	if got := ExtractConfigSection(testConfigTemplate, "不存在的标题"); got != "" {
		t.Errorf("不存在的标题应返回空，得到 %q", got)
	}
}

func TestApplyConfigSectionsAdded(t *testing.T) {
	root := t.TempDir()
	templates := map[string]string{".sunshine/config.yaml": testConfigTemplate}
	writeTestFile(t, root, ".sunshine/config.yaml", "session_commit_message: \"自定义\"\n")
	entries := []ConfigSectionAdded{
		{File: ".sunshine/config.yaml", Sentinel: "codex:", SectionHeading: "Codex 配置"},
		{File: ".sunshine/config.yaml", Sentinel: "codex:", SectionHeading: "Codex 配置"}, // 重复条目应去重
	}
	appended := ApplyConfigSectionsAdded(entries, root, templates)
	if appended != 1 {
		t.Fatalf("应追加 1 段，得到 %d", appended)
	}
	content := readTestFile(t, root, ".sunshine/config.yaml")
	if !strings.Contains(content, "自定义") || !strings.Contains(content, "codex:") {
		t.Errorf("追加后用户定制应保留、新段落应出现：%q", content)
	}
	// 幂等：再跑一次不追加。
	if appended := ApplyConfigSectionsAdded(entries, root, templates); appended != 0 {
		t.Errorf("已含 sentinel 时应不再追加，得到 %d", appended)
	}
	// 目标文件不存在、模板无此段落时跳过。
	if appended := ApplyConfigSectionsAdded([]ConfigSectionAdded{{File: ".sunshine/missing.yaml", Sentinel: "x:", SectionHeading: "Codex 配置"}}, root, templates); appended != 0 {
		t.Errorf("目标不存在时应跳过，得到 %d", appended)
	}
	if appended := ApplyConfigSectionsAdded([]ConfigSectionAdded{{File: ".sunshine/config.yaml", Sentinel: "nope:", SectionHeading: "不存在的标题"}}, root, templates); appended != 0 {
		t.Errorf("模板无此段落时应跳过，得到 %d", appended)
	}
}

func TestLoadEmbeddedRegistry020(t *testing.T) {
	reg, warnings := LoadEmbeddedRegistry()
	if reg == nil {
		t.Fatal("内嵌清单集不应为 nil")
	}
	if len(warnings) != 0 {
		t.Fatalf("内嵌清单集应无警告，得到 %v", warnings)
	}
	// 第一份真实清单：0.2.0（蓝本 Trellis 0.5.0-beta.0 的 Skill-first 迁移）。
	if got := reg.Versions(); len(got) != 1 || got[0] != "0.2.0" {
		t.Fatalf("内嵌清单集应只有 0.2.0，得到 %v", got)
	}
	items := reg.MigrationsForVersion("0.1.0", "0.2.0")
	var renames, safeDeletes int
	for _, item := range items {
		if strings.Contains(item.From, "trellis") || strings.Contains(item.From, "Trellis") ||
			strings.Contains(item.To, "trellis") || strings.Contains(item.To, "Trellis") {
			t.Errorf("清单路径不得残留 trellis 字样：%s → %s", item.From, item.To)
		}
		switch item.Type {
		case MigrationTypeRename:
			renames++
			if item.To == "" {
				t.Errorf("改名项缺少目标路径：%s", item.From)
			}
		case MigrationTypeSafeFileDelete:
			safeDeletes++
			if len(item.AllowedHashes) == 0 {
				t.Errorf("safe-file-delete 缺少 allowed_hashes：%s", item.From)
			}
		default:
			t.Errorf("0.2.0 清单不应含 %s 类型：%s", item.Type, item.From)
		}
	}
	if renames != 6 {
		t.Errorf("0.2.0 应有 6 条改名，得到 %d", renames)
	}
	if safeDeletes != 4 {
		t.Errorf("0.2.0 应有 4 条 safe-file-delete，得到 %d", safeDeletes)
	}
	meta := reg.Metadata("0.1.0", "0.2.0")
	if !meta.Breaking || !meta.RecommendMigrate {
		t.Errorf("0.2.0 应为破坏性版本且建议 --migrate，得到 Breaking=%v RecommendMigrate=%v", meta.Breaking, meta.RecommendMigrate)
	}
	if !reg.HasPendingMigrations("0.1.0", "0.2.0") {
		t.Error("0.1.0 → 0.2.0 应有待执行迁移")
	}
	if reg.HasPendingMigrations("0.2.0", "0.2.0") {
		t.Error("同版本之间不应有待执行迁移")
	}
}
