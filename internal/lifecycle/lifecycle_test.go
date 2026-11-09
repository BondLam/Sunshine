package lifecycle

import (
	"crypto/sha256"
	"encoding/hex"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"

	"github.com/linrong1982/Sunshine/internal/platforms"
	"github.com/linrong1982/Sunshine/templates"
)

// setupProject 在临时目录里搭建一个真实的已初始化项目：
// 按内嵌模板写 .sunshine/ 骨架（与 scaffold.Init 相同的写出与
// 哈希规则），再装三平台并登记哈希，返回项目根目录。
// 注意：这里不用 scaffold 包——scaffold 在阶段 4 接了
// internal/registry（B 组施工中），测试直接依赖它会在
// 他组包编译失败时连带失败；骨架逻辑很小，在测试里自备一份。
func setupProject(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	writeSunshineSkeleton(t, dir, "tester")
	for _, id := range []string{"claude-code", "cursor", "codex"} {
		files, err := platforms.Install(dir, id, platforms.InstallOptions{})
		if err != nil {
			t.Fatalf("安装平台 %s 失败：%v", id, err)
		}
		recordHashes(t, dir, files)
	}
	return dir
}

// hashExcluded 是骨架里不进哈希清单的文件（与 scaffold 一致）。
var hashExcluded = map[string]bool{
	".template-hashes.json": true,
	".version":              true,
	".developer":            true,
}

// writeSunshineSkeleton 按内嵌模板写 .sunshine/ 骨架并生成清单。
func writeSunshineSkeleton(t *testing.T, dir, user string) {
	t.Helper()
	target := filepath.Join(dir, ".sunshine")
	hashes := map[string]string{}
	err := fs.WalkDir(templates.Sunshine, "sunshine", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		rel := strings.TrimPrefix(path, "sunshine/")
		data, err := templates.Sunshine.ReadFile(path)
		if err != nil {
			return err
		}
		dest := filepath.Join(target, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(dest, data, 0o644); err != nil {
			return err
		}
		if !hashExcluded[rel] {
			sum := sha256.Sum256(data)
			hashes[filepath.ToSlash(filepath.Join(".sunshine", rel))] = hex.EncodeToString(sum[:])
		}
		return nil
	})
	if err != nil {
		t.Fatalf("写骨架失败：%v", err)
	}
	nativeWorkflow, err := templates.Workflows.ReadFile("workflows/native/workflow.md")
	if err != nil {
		t.Fatalf("读原生工作流模板失败：%v", err)
	}
	if err := os.WriteFile(filepath.Join(target, "workflow.md"), nativeWorkflow, 0o644); err != nil {
		t.Fatalf("写 workflow.md 失败：%v", err)
	}
	sum := sha256.Sum256(nativeWorkflow)
	hashes[".sunshine/workflow.md"] = hex.EncodeToString(sum[:])
	if err := os.WriteFile(filepath.Join(target, ".developer"), []byte(user+"\n"), 0o644); err != nil {
		t.Fatalf("写开发者身份失败：%v", err)
	}
	if err := SaveHashes(dir, hashes); err != nil {
		t.Fatalf("写清单失败：%v", err)
	}
}

// recordHashes 把 files 的 SHA-256 并入清单（与 scaffold.RecordHashes 同规则）。
func recordHashes(t *testing.T, dir string, files []string) {
	t.Helper()
	hashes, err := LoadHashes(dir)
	if err != nil {
		t.Fatalf("读清单失败：%v", err)
	}
	for _, rel := range files {
		data, err := os.ReadFile(filepath.Join(dir, filepath.FromSlash(rel)))
		if err != nil {
			t.Fatalf("计算 %s 的哈希失败：%v", rel, err)
		}
		sum := sha256.Sum256(data)
		hashes[rel] = hex.EncodeToString(sum[:])
	}
	if err := SaveHashes(dir, hashes); err != nil {
		t.Fatalf("写清单失败：%v", err)
	}
}

// snapshotEntry 是快照里单个文件的记录。
type snapshotEntry struct {
	SHA256 string
	Mode   uint32
	Target string // 符号链接的目标，非链接为空
}

// snapshot 递归记录 dir 下全部文件（相对路径 → 内容哈希 + 权限）。
func snapshot(t *testing.T, dir string) map[string]snapshotEntry {
	t.Helper()
	out := map[string]snapshotEntry{}
	err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(dir, path)
		if err != nil {
			return err
		}
		info, err := os.Lstat(path)
		if err != nil {
			return err
		}
		entry := snapshotEntry{Mode: uint32(info.Mode().Perm())}
		if info.Mode()&os.ModeSymlink != 0 {
			target, err := os.Readlink(path)
			if err != nil {
				return err
			}
			entry.Target = target
		} else {
			data, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			sum := sha256.Sum256(data)
			entry.SHA256 = hex.EncodeToString(sum[:])
		}
		out[filepath.ToSlash(rel)] = entry
		return nil
	})
	if err != nil {
		t.Fatalf("快照失败：%v", err)
	}
	return out
}

func TestPruneOrphanManifestKeys(t *testing.T) {
	dir := setupProject(t)
	hashes, err := LoadHashes(dir)
	if err != nil {
		t.Fatalf("读清单失败：%v", err)
	}
	// 造三条孤儿键：用户自加的 skill、Codex 会话文件、根 AGENTS.md。
	orphans := []string{
		".claude/skills/custom/SKILL.md",
		".codex/sessions/abc.jsonl",
		"AGENTS.md",
	}
	for _, rel := range orphans {
		abs := filepath.Join(dir, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(abs, []byte("user file\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		hashes[rel] = "deadbeef"
	}
	if err := SaveHashes(dir, hashes); err != nil {
		t.Fatal(err)
	}
	configured := ConfiguredPlatformIDs(hashes)
	result, err := PruneOrphanManifestKeys(dir, configured, hashes, true)
	if err != nil {
		t.Fatalf("剪枝失败：%v", err)
	}
	sort.Strings(result.Pruned)
	if !reflect.DeepEqual(result.Pruned, orphans) {
		t.Errorf("剪掉的键 = %v，想得到 %v", result.Pruned, orphans)
	}
	for _, rel := range orphans {
		if _, ok := result.Hashes[rel]; ok {
			t.Errorf("孤儿键 %s 仍在清单里", rel)
		}
	}
	if _, ok := result.Hashes[".claude/settings.json"]; !ok {
		t.Error("已认领的清单键 .claude/settings.json 不应被剪掉")
	}
	if _, ok := result.Hashes[".sunshine/workflow.md"]; !ok {
		t.Error(".sunshine/ 下的条目应一律保留")
	}
	// persist=true：磁盘上的清单也已更新。
	onDisk, err := LoadHashes(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, rel := range orphans {
		if _, ok := onDisk[rel]; ok {
			t.Errorf("磁盘清单里仍有孤儿键 %s", rel)
		}
	}

	// persist=false：只算不写。拿剪后清单加回一条孤儿键再算。
	keptPlusOrphan := map[string]string{}
	for k, v := range result.Hashes {
		keptPlusOrphan[k] = v
	}
	keptPlusOrphan[".claude/skills/custom/SKILL.md"] = "deadbeef"
	result2, err := PruneOrphanManifestKeys(dir, configured, keptPlusOrphan, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(result2.Pruned) != 1 || result2.Pruned[0] != ".claude/skills/custom/SKILL.md" {
		t.Errorf("persist=false 时剪掉的键 = %v", result2.Pruned)
	}
	onDisk2, _ := LoadHashes(dir)
	if _, ok := onDisk2[".claude/skills/custom/SKILL.md"]; ok {
		t.Error("persist=false 时不应写盘")
	}
}

func TestConfiguredPlatformIDs(t *testing.T) {
	dir := t.TempDir()
	writeSunshineSkeleton(t, dir, "tester")
	hashes, _ := LoadHashes(dir)
	if got := ConfiguredPlatformIDs(hashes); len(got) != 0 {
		t.Errorf("只初始化骨架时已配置平台 = %v，想得到空", got)
	}
	// 只装 Cursor。
	files, err := platforms.Install(dir, "cursor", platforms.InstallOptions{})
	if err != nil {
		t.Fatal(err)
	}
	recordHashes(t, dir, files)
	hashes, _ = LoadHashes(dir)
	got := ConfiguredPlatformIDs(hashes)
	if !reflect.DeepEqual(got, []string{"cursor"}) {
		t.Errorf("只装 Cursor 时已配置平台 = %v，想得到 [cursor]", got)
	}
	// 原生 .claude 目录（清单里无记录）不应误判。
	if err := os.MkdirAll(filepath.Join(dir, ".claude"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, ".claude", "native.md"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := ConfiguredPlatformIDs(hashes); !reflect.DeepEqual(got, []string{"cursor"}) {
		t.Errorf("有原生 .claude 目录时已配置平台 = %v，想得到 [cursor]", got)
	}
}

func TestConfiguredPlatforms(t *testing.T) {
	dir := setupProject(t)
	infos, err := ConfiguredPlatforms(dir)
	if err != nil {
		t.Fatal(err)
	}
	var ids []string
	for _, info := range infos {
		ids = append(ids, info.ID)
	}
	if !reflect.DeepEqual(ids, []string{"claude-code", "cursor", "codex"}) {
		t.Errorf("已配置平台 = %v，想得到 [claude-code cursor codex]", ids)
	}
	if infos[0].DisplayName != "Claude Code" || infos[0].ConfigDir != ".claude" {
		t.Errorf("claude-code 的输出 = %+v", infos[0])
	}
	if infos[2].DisplayName != "Codex" || infos[2].ConfigDir != ".codex" {
		t.Errorf("codex 的输出 = %+v", infos[2])
	}

	empty := t.TempDir()
	infos, err = ConfiguredPlatforms(empty)
	if err != nil {
		t.Fatal(err)
	}
	if len(infos) != 0 {
		t.Errorf("空目录的已配置平台 = %v，想得到空", infos)
	}
}

func TestUninstallDryRun(t *testing.T) {
	dir := setupProject(t)
	before := snapshot(t, dir)
	if err := Uninstall(dir, UninstallOptions{Yes: true, DryRun: true}); err != nil {
		t.Fatalf("dry-run 失败：%v", err)
	}
	after := snapshot(t, dir)
	if !reflect.DeepEqual(before, after) {
		t.Error("dry-run 改动了文件")
	}
}

func TestUninstall(t *testing.T) {
	dir := setupProject(t)
	// 用户自有文件（不在清单里）：卸载后必须还在。
	userSkill := filepath.Join(dir, ".claude", "skills", "custom", "SKILL.md")
	if err := os.MkdirAll(filepath.Dir(userSkill), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(userSkill, []byte("# my skill\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	// 用户往 settings.json 里加了顶层键，往 config.toml 里加了自有行。
	settingsPath := filepath.Join(dir, ".claude", "settings.json")
	settingsData, err := os.ReadFile(settingsPath)
	if err != nil {
		t.Fatal(err)
	}
	settingsObj, err := parseOrderedJSON(settingsData)
	if err != nil {
		t.Fatal(err)
	}
	settingsObj.set("model", "opus")
	out, err := settingsObj.marshalIndent()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(settingsPath, append(out, '\n'), 0o644); err != nil {
		t.Fatal(err)
	}
	configPath := filepath.Join(dir, ".codex", "config.toml")
	f, err := os.OpenFile(configPath, os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.WriteString("\nmy_option = \"keep me\"\n"); err != nil {
		t.Fatal(err)
	}
	_ = f.Close()

	if err := Uninstall(dir, UninstallOptions{Yes: true}); err != nil {
		t.Fatalf("卸载失败：%v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, ".sunshine")); !os.IsNotExist(err) {
		t.Error(".sunshine/ 应已被整个移除")
	}
	if _, err := os.Stat(userSkill); err != nil {
		t.Errorf("用户自有文件被删了：%v", err)
	}
	// settings.json：hooks 被剥离，env 与用户顶层键保留。
	settingsAfter, err := os.ReadFile(settingsPath)
	if err != nil {
		t.Fatalf("settings.json 不应被删：%v", err)
	}
	afterObj, err := parseOrderedJSON(settingsAfter)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := afterObj.get("hooks"); ok {
		t.Error("settings.json 里的 hooks 应已被剥离")
	}
	if v, ok := afterObj.get("model"); !ok || v != "opus" {
		t.Errorf("settings.json 里的用户顶层键 model 应保留，实际 %v", v)
	}
	if _, ok := afterObj.get("env"); !ok {
		t.Error("settings.json 里的 env 应保留")
	}
	// config.toml：只剩用户自有行。
	configAfter, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("config.toml 不应被删（有用户自有行）：%v", err)
	}
	if !strings.Contains(string(configAfter), `my_option = "keep me"`) {
		t.Errorf("config.toml 应保留用户行，实际内容：\n%s", configAfter)
	}
	if strings.Contains(string(configAfter), "project_doc_fallback_filenames") {
		t.Error("config.toml 里的 Sunshine 行应已被清理")
	}
	// 纯托管的 hooks 文件应被删掉；.cursor/hooks.json 有 version 键，
	// 清理后只剩 {"version": 1} 的壳，应保留（上游 scrubHooksJson
	// 的 fullyEmpty 只看顶层键是否全空，同理）。
	if _, err := os.Stat(filepath.Join(dir, ".codex", "hooks.json")); !os.IsNotExist(err) {
		t.Error(".codex/hooks.json 应已被删除")
	}
	cursorHooks, err := os.ReadFile(filepath.Join(dir, ".cursor", "hooks.json"))
	if err != nil {
		t.Fatalf(".cursor/hooks.json 应保留为清理后的壳：%v", err)
	}
	if strings.Contains(string(cursorHooks), "sunshine hook") {
		t.Error(".cursor/hooks.json 里不应再有 Sunshine hook")
	}
	if !strings.Contains(string(cursorHooks), `"version": 1`) {
		t.Errorf(".cursor/hooks.json 应保留 version 键，实际：\n%s", cursorHooks)
	}
	// 共享 skills 目录变空后应被清理。
	if _, err := os.Stat(filepath.Join(dir, ".agents")); !os.IsNotExist(err) {
		t.Error(".agents/ 变空后应被清理")
	}
}

func TestUninstallNoManifest(t *testing.T) {
	dir := setupProject(t)
	if err := os.Remove(filepath.Join(dir, ".sunshine", ".template-hashes.json")); err != nil {
		t.Fatal(err)
	}
	err := Uninstall(dir, UninstallOptions{Yes: true})
	if err == nil {
		t.Fatal("清单缺失时应拒绝卸载")
	}
	if !strings.Contains(err.Error(), "清单") {
		t.Errorf("错误信息应提到清单，实际：%v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, ".sunshine")); err != nil {
		t.Error("拒绝卸载时不应删任何东西")
	}
}

func TestUninstallNotInstalled(t *testing.T) {
	dir := t.TempDir()
	if err := Uninstall(dir, UninstallOptions{Yes: true}); err != nil {
		t.Errorf("未安装时应直接提示、不报错，实际：%v", err)
	}
}

func TestUninstallDirtyYesRefused(t *testing.T) {
	if _, err := lookPathGit(); err != nil {
		t.Skip("无 git，跳过")
	}
	dir := setupProject(t)
	runGit(t, dir, "init", "-q")
	runGit(t, dir, "config", "user.name", "tester")
	runGit(t, dir, "config", "user.email", "tester@example.com")
	specFile := filepath.Join(dir, ".sunshine", "spec", "draft.md")
	if err := os.MkdirAll(filepath.Dir(specFile), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(specFile, []byte("# 未提交的规范\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := CollectUncommittedSunshineData(dir); len(got) == 0 {
		t.Fatal("应检测到未提交的 spec 文件")
	}
	err := Uninstall(dir, UninstallOptions{Yes: true})
	if err == nil || !strings.Contains(err.Error(), "拒绝") {
		t.Errorf("--yes 且有未提交数据时应拒绝，实际 err=%v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, ".sunshine")); err != nil {
		t.Error("拒绝后不应删任何东西")
	}
	// 显式绕过后应能卸载。
	t.Setenv("SUNSHINE_ALLOW_DIRTY_UNINSTALL", "1")
	if err := Uninstall(dir, UninstallOptions{Yes: true}); err != nil {
		t.Errorf("设置绕过后应能卸载，实际：%v", err)
	}
}

func TestUninstallDirtyInteractiveConfirmed(t *testing.T) {
	stubTTY(t, true)
	if _, err := lookPathGit(); err != nil {
		t.Skip("无 git，跳过")
	}
	dir := setupProject(t)
	runGit(t, dir, "init", "-q")
	specFile := filepath.Join(dir, ".sunshine", "spec", "draft.md")
	if err := os.MkdirAll(filepath.Dir(specFile), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(specFile, []byte("# 未提交的规范\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	// 交互确认（非 --yes）时警告后仍可继续。
	if err := Uninstall(dir, UninstallOptions{
		Confirm: func(string) (bool, error) { return true, nil },
	}); err != nil {
		t.Errorf("交互确认后应能卸载，实际：%v", err)
	}
}
