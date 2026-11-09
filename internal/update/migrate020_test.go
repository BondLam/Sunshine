package update

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/linrong1982/Sunshine/internal/migrate"
	"github.com/linrong1982/Sunshine/internal/platforms"
	"github.com/linrong1982/Sunshine/internal/scaffold"
)

// ---------- 0.2.0 内嵌清单的 CLI 级回归 ----------
//
// 夹具模拟一个 0.1.0 时代的项目：旧版命令形式的文件还在，
// skill-first 之后的 skill 文件还没装。旧版文件内容是本测试定义的
// “旧版模板内容”——与 internal/migrate/manifests/0.2.0.json 里
// safe-file-delete 的 allowed_hashes 对应；改动其中任何一处内容，
// 须同步更新清单里的哈希（否则 safe-file-delete 不命中，测试会失败）。
const (
	oldClaudeBeforeDev = "# 开发前检查（旧版命令）\n\n0.1.0 时代的命令形式：开发前按这份清单逐项检查，通过后再动手。\n0.2.0 起改为自动触发的 skill（sunshine-before-dev），命令形式不再保留。\n"
	oldClaudeCheck     = "# 检查（旧版命令）\n\n0.1.0 时代的命令形式：按规范逐项检查本次改动。\n0.2.0 起改为自动触发的 skill（sunshine-check），命令形式不再保留。\n"
	oldAgentsBeforeDev = "# 开发前检查（旧版 skill）\n\n0.1.0 时代的 skill 形式（未加 sunshine- 前缀）。\n0.2.0 起改名 sunshine-before-dev，避免与用户自己的 skill 重名。\n"
	oldAgentsCheck     = "# 检查（旧版 skill）\n\n0.1.0 时代的 skill 形式（未加 sunshine- 前缀）。\n0.2.0 起改名 sunshine-check，避免与用户自己的 skill 重名。\n"
	oldOnboard         = "# 迎新（旧版命令）\n\n0.1.0 时代的命令形式：为新成员介绍项目规范与工作流。\n0.2.0 起由初始化流程与 skill 承接，命令形式退役。\n"
	oldParallel        = "# 并行开发（旧版命令）\n\n0.1.0 时代的命令形式：多任务并行时的分工与同步办法。\n0.2.0 起由 skill 承接，命令形式退役。\n"
	oldAgentsOnboard   = "# 迎新（旧版 skill）\n\n0.1.0 时代的 skill 形式：为新成员介绍项目规范与工作流。\n0.2.0 起退役。\n"
	// 改动过的两处：先按原内容记哈希，再追加本地定制行。
	modifiedParallel    = oldParallel + "\n<!-- 本地定制：并行任务上限改为 3。 -->\n"
	modifiedCursorCheck = oldClaudeCheck + "\n<!-- 本地定制：检查时跳过文档类改动。 -->\n"
)

// rename020Targets 是 0.2.0 清单 6 条改名的目标路径。
var rename020Targets = []string{
	".claude/skills/sunshine-before-dev/SKILL.md",
	".claude/skills/sunshine-check/SKILL.md",
	".cursor/skills/sunshine-before-dev/SKILL.md",
	".cursor/skills/sunshine-check/SKILL.md",
	".agents/skills/sunshine-before-dev/SKILL.md",
	".agents/skills/sunshine-check/SKILL.md",
}

// rename020Sources 是 0.2.0 清单 6 条改名的源路径。
var rename020Sources = []string{
	".claude/commands/sunshine/before-dev.md",
	".claude/commands/sunshine/check.md",
	".cursor/commands/sunshine-before-dev.md",
	".cursor/commands/sunshine-check.md",
	".agents/skills/before-dev/SKILL.md",
	".agents/skills/check/SKILL.md",
}

// setupMigration020Project 建一个 0.1.0 项目夹具：真实 init（三平台）
// + 植入旧版文件（改名源与待清理文件）+ 删掉改名目标
// （模拟 skill-first 之前的布局：命令在、skill 不在）。
func setupMigration020Project(t *testing.T) string {
	t.Helper()
	dir := setupProject(t)
	files, err := platforms.Install(dir, "codex", platforms.InstallOptions{})
	if err != nil {
		t.Fatalf("安装 codex 失败：%v", err)
	}
	if err := scaffold.RecordHashes(dir, files); err != nil {
		t.Fatalf("记录 codex 哈希失败：%v", err)
	}
	planted := map[string]string{
		".claude/commands/sunshine/before-dev.md": oldClaudeBeforeDev,
		".claude/commands/sunshine/check.md":      oldClaudeCheck,
		".cursor/commands/sunshine-before-dev.md": oldClaudeBeforeDev,
		".cursor/commands/sunshine-check.md":      oldClaudeCheck,
		".agents/skills/before-dev/SKILL.md":      oldAgentsBeforeDev,
		".agents/skills/check/SKILL.md":           oldAgentsCheck,
		".claude/commands/sunshine/onboard.md":    oldOnboard,
		".claude/commands/sunshine/parallel.md":   oldParallel,
		".cursor/commands/sunshine-onboard.md":    oldOnboard,
		".agents/skills/onboard/SKILL.md":         oldAgentsOnboard,
	}
	hashes := LoadHashes(dir)
	for rel, content := range planted {
		writeFile(t, dir, rel, content)
		hashes[rel] = ComputeHashString(content)
	}
	// 两处本地改动：哈希仍记原内容，文件内容已改。
	writeFile(t, dir, ".claude/commands/sunshine/parallel.md", modifiedParallel)
	writeFile(t, dir, ".cursor/commands/sunshine-check.md", modifiedCursorCheck)
	// 删掉改名目标（文件与哈希条目一起删），否则它们会被当成
	// “用户已删除”而不写新版模板。
	for _, target := range rename020Targets {
		if err := os.Remove(filepath.Join(dir, filepath.FromSlash(target))); err != nil {
			t.Fatalf("删除改名目标 %s 失败：%v", target, err)
		}
		delete(hashes, target)
	}
	if err := SaveHashes(dir, hashes); err != nil {
		t.Fatalf("保存哈希清单失败：%v", err)
	}
	return dir
}

func backupDirs(t *testing.T, dir string) []string {
	t.Helper()
	entries, err := os.ReadDir(filepath.Join(dir, ".sunshine", ".backups"))
	if err != nil {
		return nil
	}
	var out []string
	for _, e := range entries {
		if e.IsDir() {
			out = append(out, filepath.Join(dir, ".sunshine", ".backups", e.Name()))
		}
	}
	return out
}

func TestRunMigration020Gate(t *testing.T) {
	dir := setupMigration020Project(t)
	if got := GetInstalledVersion(dir); got != "0.1.0" {
		t.Fatalf("夹具项目版应为 0.1.0，得到 %s", got)
	}
	// 无 --migrate 时破坏性迁移硬停，什么都不改。
	var out bytes.Buffer
	if _, err := Run(dir, Options{CLIVersion: "0.2.0", Force: true, Prompter: fakePrompter{proceed: true}, Out: &out}); err == nil {
		t.Error("无 --migrate 时应硬停")
	} else if !strings.Contains(out.String(), "sunshine update --migrate") {
		t.Errorf("硬停提示应指引 --migrate，实际输出：\n%s", out.String())
	}
	if got := GetInstalledVersion(dir); got != "0.1.0" {
		t.Errorf("硬停后项目版应仍为 0.1.0，得到 %s", got)
	}
	if len(backupDirs(t, dir)) != 0 {
		t.Error("硬停后不应创建备份")
	}
	if _, err := os.Stat(filepath.Join(dir, ".claude", "commands", "sunshine", "before-dev.md")); err != nil {
		t.Error("硬停后旧版文件应原样保留")
	}
}

func TestRunMigration020DryRun(t *testing.T) {
	dir := setupMigration020Project(t)
	var out bytes.Buffer
	if _, err := Run(dir, Options{CLIVersion: "0.2.0", DryRun: true, Force: true, Out: &out}); err != nil {
		t.Fatalf("dry-run 不应硬停：%v", err)
	}
	if len(backupDirs(t, dir)) != 0 {
		t.Error("dry-run 不应创建备份")
	}
	if got := GetInstalledVersion(dir); got != "0.1.0" {
		t.Errorf("dry-run 后项目版应仍为 0.1.0，得到 %s", got)
	}
	if _, err := os.Stat(filepath.Join(dir, ".claude", "commands", "sunshine", "before-dev.md")); err != nil {
		t.Error("dry-run 后旧版文件应原样保留")
	}
	// 预览应列出迁移计划与过期文件清理计划。
	if !strings.Contains(out.String(), "自动迁移") {
		t.Errorf("dry-run 应列出自动迁移计划，实际输出：\n%s", out.String())
	}
	if !strings.Contains(out.String(), "过期文件清理") {
		t.Errorf("dry-run 应列出过期文件清理计划，实际输出：\n%s", out.String())
	}
}

func TestRunMigration020MigrateForce(t *testing.T) {
	dir := setupMigration020Project(t)
	var out bytes.Buffer
	// 非交互 + --migrate --force：自动项执行，已改动项直接执行（无备份），
	// safe-file-delete 按哈希清理。
	if _, err := Run(dir, Options{CLIVersion: "0.2.0", Migrate: true, Force: true, Out: &out}); err != nil {
		t.Fatalf("--migrate --force 应放行：%v", err)
	}
	// 6 条改名全部执行：源消失，目标为新版模板内容。
	for _, src := range rename020Sources {
		if _, err := os.Stat(filepath.Join(dir, filepath.FromSlash(src))); !os.IsNotExist(err) {
			t.Errorf("改名源应已搬走：%s", src)
		}
	}
	for _, target := range rename020Targets {
		content := readFile(t, dir, target)
		if strings.Contains(content, "旧版命令") || strings.Contains(content, "旧版 skill") {
			t.Errorf("改名目标应为新版模板内容：%s", target)
		}
	}
	if got := readFile(t, dir, ".claude/skills/sunshine-before-dev/SKILL.md"); !strings.Contains(got, "sunshine-before-dev") {
		t.Errorf("改名目标应含新版 skill 名 sunshine-before-dev，实际：\n%s", got)
	}
	// safe-file-delete：3 个未改动的退役文件被删，改动过的保留。
	for _, gone := range []string{
		".claude/commands/sunshine/onboard.md",
		".cursor/commands/sunshine-onboard.md",
		".agents/skills/onboard/SKILL.md",
	} {
		if _, err := os.Stat(filepath.Join(dir, filepath.FromSlash(gone))); !os.IsNotExist(err) {
			t.Errorf("未改动的退役文件应被清理：%s", gone)
		}
	}
	if got := readFile(t, dir, ".claude/commands/sunshine/parallel.md"); got != modifiedParallel {
		t.Errorf("改动过的退役文件应原样保留，实际：\n%s", got)
	}
	// --force 是直接执行，不写 .backup。
	if _, err := os.Stat(filepath.Join(dir, ".cursor", "skills", "sunshine-check", "SKILL.md.backup")); !os.IsNotExist(err) {
		t.Error("--force 直接执行时不应写 .backup")
	}
	// .version 已更新；备份含更新前的内容（可据此回滚）。
	if got := GetInstalledVersion(dir); got != "0.2.0" {
		t.Errorf("更新后项目版应为 0.2.0，得到 %s", got)
	}
	backs := backupDirs(t, dir)
	if len(backs) != 1 {
		t.Fatalf("应恰好创建一个备份目录，得到 %d", len(backs))
	}
	backupCheck, err := os.ReadFile(filepath.Join(backs[0], ".cursor", "commands", "sunshine-check.md"))
	if err != nil {
		t.Fatalf("备份应含改动过的改名源文件：%v", err)
	}
	if string(backupCheck) != modifiedCursorCheck {
		t.Errorf("备份里的改名源应为更新前的内容，实际：\n%s", backupCheck)
	}
	backupParallel, err := os.ReadFile(filepath.Join(backs[0], ".claude", "commands", "sunshine", "parallel.md"))
	if err != nil {
		t.Fatalf("备份应含改动过的退役文件：%v", err)
	}
	if string(backupParallel) != modifiedParallel {
		t.Errorf("备份里的退役文件应为更新前的内容，实际：\n%s", backupParallel)
	}
	// 哈希清单：旧键已改名/删除，新键就位，保留的退役文件键仍在。
	hashes := LoadHashes(dir)
	for _, src := range rename020Sources {
		if _, ok := hashes[src]; ok {
			t.Errorf("哈希清单旧键应已搬走：%s", src)
		}
	}
	for _, target := range rename020Targets {
		if _, ok := hashes[target]; !ok {
			t.Errorf("哈希清单应有新键：%s", target)
		}
	}
	for _, gone := range []string{".claude/commands/sunshine/onboard.md", ".cursor/commands/sunshine-onboard.md", ".agents/skills/onboard/SKILL.md"} {
		if _, ok := hashes[gone]; ok {
			t.Errorf("已清理文件的哈希键应删除：%s", gone)
		}
	}
	if _, ok := hashes[".claude/commands/sunshine/parallel.md"]; !ok {
		t.Error("保留的退役文件哈希键应仍在")
	}
}

func TestRunMigration020MigrateNonInteractive(t *testing.T) {
	// 本测试依赖 go test 的 stdin 不是终端；若有人用终端 stdin 跑测试
	// 则跳过（否则 survey 会等输入而挂住）。
	if stdinIsTerminal() {
		t.Skip("stdin 是终端时跳过非交互测试")
	}
	dir := setupMigration020Project(t)
	var out bytes.Buffer
	// 非交互 + --migrate（无 --force、有 Prompter）：自动项执行，
	// 已改动项直接跳过（不调 survey、不报错），更新照常完成。
	// 回归：修复前这里会在迁移确认处读到 EOF 而让整个 update 失败。
	if _, err := Run(dir, Options{CLIVersion: "0.2.0", Migrate: true, Prompter: fakePrompter{proceed: true}, Out: &out}); err != nil {
		t.Fatalf("非交互 --migrate 应放行：%v\n输出：\n%s", err, out.String())
	}
	// 5 条未改动改名执行；改动过的 cursor-check 被跳过，旧路径保留。
	for _, src := range rename020Sources {
		_, err := os.Stat(filepath.Join(dir, filepath.FromSlash(src)))
		if src == ".cursor/commands/sunshine-check.md" {
			if err != nil {
				t.Error("非交互时改动过的改名源应保留（跳过）")
			}
			continue
		}
		if !os.IsNotExist(err) {
			t.Errorf("改名源应已搬走：%s", src)
		}
	}
	// 被跳过的改名目标仍按新文件写盘（分析时它不在盘上）。
	if got := readFile(t, dir, ".cursor/skills/sunshine-check/SKILL.md"); strings.Contains(got, "旧版命令") {
		t.Error("被跳过的改名目标仍应由模板写盘，不应是旧内容")
	}
	if _, err := os.Stat(filepath.Join(dir, ".claude", "commands", "sunshine", "onboard.md")); !os.IsNotExist(err) {
		t.Error("未改动的退役文件应被清理")
	}
	if got := GetInstalledVersion(dir); got != "0.2.0" {
		t.Errorf("更新后项目版应为 0.2.0，得到 %s", got)
	}
}

func TestRunMigration020MigrateBackupRename(t *testing.T) {
	dir := setupMigration020Project(t)
	// 交互确认替身：已改动项一律“备份后执行”。
	old := MigrationPrompter
	MigrationPrompter = func(item migrate.MigrationItem) (migrate.MigrationAction, error) {
		return migrate.ActionBackupRename, nil
	}
	defer func() { MigrationPrompter = old }()
	var out bytes.Buffer
	if _, err := Run(dir, Options{CLIVersion: "0.2.0", Migrate: true, Prompter: fakePrompter{proceed: true}, Out: &out}); err != nil {
		t.Fatalf("--migrate 应放行：%v", err)
	}
	// 改动过的改名源：先写 .backup 再搬走，备份内容为改动后的旧内容。
	backupPath := ".cursor/skills/sunshine-check/SKILL.md.backup"
	if got := readFile(t, dir, backupPath); got != modifiedCursorCheck {
		t.Errorf(".backup 应保存改动后的旧内容，实际：\n%s", got)
	}
	if _, err := os.Stat(filepath.Join(dir, ".cursor", "commands", "sunshine-check.md")); !os.IsNotExist(err) {
		t.Error("备份后执行的改名源应已搬走")
	}
	if got := GetInstalledVersion(dir); got != "0.2.0" {
		t.Errorf("更新后项目版应为 0.2.0，得到 %s", got)
	}
}
