package update

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/linrong007/Sunshine/internal/migrate"
	"github.com/linrong007/Sunshine/internal/scaffold"
)

func TestRunDryRunDoesNotWrite(t *testing.T) {
	dir := setupProject(t)
	orig := readFile(t, dir, ".sunshine/workflow.md")
	writeFile(t, dir, ".sunshine/agents/check.md", "用户改过的检查\n")
	// 清单里把 check.md 的哈希记为改动前（即已改动），
	// workflow.md 保持未改动。
	var out bytes.Buffer
	summary, err := Run(dir, Options{CLIVersion: "0.1.0", DryRun: true, Out: &out})
	if err != nil {
		t.Fatalf("dry-run 失败：%v", err)
	}
	if summary.Added != 0 || summary.AutoUpdated != 0 || summary.Updated != 0 {
		t.Errorf("dry-run 不应有写盘计数：%+v", summary)
	}
	if got := readFile(t, dir, ".sunshine/agents/check.md"); got != "用户改过的检查\n" {
		t.Error("dry-run 不应改动文件")
	}
	if got := readFile(t, dir, ".sunshine/workflow.md"); got != orig {
		t.Error("dry-run 不应改动未改动文件")
	}
	if _, err := os.Stat(filepath.Join(dir, ".sunshine", ".backups")); !os.IsNotExist(err) {
		t.Error("dry-run 不应生成备份目录")
	}
	if !strings.Contains(out.String(), "你已修改") || !strings.Contains(out.String(), "试运行") {
		t.Errorf("dry-run 应打印计划，实际：\n%s", out.String())
	}
}

func TestRunForceOverwritesAndBacksUp(t *testing.T) {
	dir := setupProject(t)
	origCheck := readFile(t, dir, ".sunshine/agents/check.md")
	writeFile(t, dir, ".sunshine/agents/check.md", "用户改过的检查\n")
	var out bytes.Buffer
	summary, err := Run(dir, Options{CLIVersion: "0.2.0", Force: true, Prompter: fakePrompter{proceed: true}, Out: &out})
	if err != nil {
		t.Fatalf("Run 失败：%v", err)
	}
	if summary.Updated != 1 {
		t.Errorf("应覆盖 1 个文件，实际 %+v", summary)
	}
	if got := readFile(t, dir, ".sunshine/agents/check.md"); got != origCheck {
		t.Error("覆盖后内容应恢复为模板")
	}
	if summary.BackupDir == "" {
		t.Fatal("应生成备份目录")
	}
	backupCheck := readFile(t, summary.BackupDir, ".sunshine/agents/check.md")
	if backupCheck != "用户改过的检查\n" {
		t.Errorf("备份应含被改文件旧内容，实际 %q", backupCheck)
	}
	if got := GetInstalledVersion(dir); got != "0.2.0" {
		t.Errorf(".version 应更新为 0.2.0，实际 %s", got)
	}
	// 哈希已更新：再跑一次应无事可做。
	var out2 bytes.Buffer
	summary2, err := Run(dir, Options{CLIVersion: "0.2.0", Force: true, Out: &out2})
	if err != nil {
		t.Fatal(err)
	}
	if !summary2.UpToDate {
		t.Errorf("第二次应无事可做，实际 %+v", summary2)
	}
	if !strings.Contains(out2.String(), "已是最新") {
		t.Errorf("第二次应提示已是最新：\n%s", out2.String())
	}
}

func TestRunConflictActions(t *testing.T) {
	// 覆盖 / 跳过 / 写 .new 各走一遍。
	for _, tc := range []struct {
		name   string
		action ConflictAction
		check  func(t *testing.T, dir string)
	}{
		{"覆盖", ActionOverwrite, func(t *testing.T, dir string) {
			if got := readFile(t, dir, ".sunshine/agents/check.md"); !strings.Contains(got, "检查") || got == "用户改过的检查\n" {
				t.Errorf("覆盖后应为模板内容，实际 %q", got)
			}
		}},
		{"跳过", ActionSkip, func(t *testing.T, dir string) {
			if got := readFile(t, dir, ".sunshine/agents/check.md"); got != "用户改过的检查\n" {
				t.Errorf("跳过后应保留用户内容，实际 %q", got)
			}
		}},
		{"写副本", ActionCreateNew, func(t *testing.T, dir string) {
			if got := readFile(t, dir, ".sunshine/agents/check.md"); got != "用户改过的检查\n" {
				t.Errorf("写副本后原文件应保留，实际 %q", got)
			}
			if _, err := os.Stat(filepath.Join(dir, ".sunshine", "agents", "check.md.new")); err != nil {
				t.Error("应生成 check.md.new")
			}
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := setupProject(t)
			writeFile(t, dir, ".sunshine/agents/check.md", "用户改过的检查\n")
			summary, err := Run(dir, Options{
				CLIVersion: "0.1.0",
				Prompter:   fakePrompter{proceed: true, actions: map[string]ConflictAction{".sunshine/agents/check.md": tc.action}},
				Out:        &bytes.Buffer{},
			})
			if err != nil {
				t.Fatal(err)
			}
			tc.check(t, dir)
			_ = summary
		})
	}
	// 非交互（Prompter 为 nil）有冲突时默认 Skip。
	dir := setupProject(t)
	writeFile(t, dir, ".sunshine/agents/check.md", "用户改过的检查\n")
	summary, err := Run(dir, Options{CLIVersion: "0.1.0", Out: &bytes.Buffer{}})
	if err != nil {
		t.Fatal(err)
	}
	if summary.Skipped != 1 {
		t.Errorf("非交互应默认跳过 1 个，实际 %+v", summary)
	}
	if got := readFile(t, dir, ".sunshine/agents/check.md"); got != "用户改过的检查\n" {
		t.Error("非交互默认 Skip 后应保留用户内容")
	}
	// 批量 skip-all：两个冲突文件一次跳过。
	dir = setupProject(t)
	writeFile(t, dir, ".sunshine/agents/check.md", "改一\n")
	writeFile(t, dir, ".sunshine/agents/plan.md", "改二\n")
	summary, err = Run(dir, Options{
		CLIVersion: "0.1.0",
		Prompter: fakePrompter{proceed: true, actions: map[string]ConflictAction{
			".sunshine/agents/check.md": ActionSkipAll,
		}},
		Out: &bytes.Buffer{},
	})
	if err != nil {
		t.Fatal(err)
	}
	if summary.Skipped != 2 {
		t.Errorf("skip-all 应跳过 2 个，实际 %+v", summary)
	}
}

func TestRunPreservesUserConfigAndCustomFiles(t *testing.T) {
	dir := setupProject(t)
	// registry.spec 存在但刷新未接入（替身 nil）时应警告、不中断。
	oldRefresh := RegistryRefresh
	RegistryRefresh = nil
	defer func() { RegistryRefresh = oldRefresh }()
	// 用户自加字段、注释、registry.spec 与自加命令/skill/agent。
	cfgPath := filepath.Join(dir, ".sunshine", "config.yaml")
	cfg, _ := os.ReadFile(cfgPath)
	custom := string(cfg) + "\n# 我的自定义注释\nmy_custom_field: hello\n\nregistry:\n  spec:\n    source: gh:owner/repo\n    template: my-spec\n"
	if err := os.WriteFile(cfgPath, []byte(custom), 0o644); err != nil {
		t.Fatal(err)
	}
	writeFile(t, dir, ".claude/commands/sunshine/mycmd.md", "我的命令\n")
	writeFile(t, dir, ".claude/skills/sunshine-custom/SKILL.md", "我的 skill\n")
	writeFile(t, dir, ".claude/agents/sunshine-custom.md", "我的 agent\n")
	// 用户自加的 hook 文件（不在模板集合里）。
	writeFile(t, dir, ".claude/hooks.json", "{\"hooks\": {}}\n")

	var out bytes.Buffer
	summary, err := Run(dir, Options{CLIVersion: "0.2.0", Force: true, Prompter: fakePrompter{proceed: true}, Out: &out})
	if err != nil {
		t.Fatalf("Run 失败：%v", err)
	}
	_ = summary
	after := readFile(t, dir, ".sunshine/config.yaml")
	for _, want := range []string{"my_custom_field: hello", "# 我的自定义注释", "registry:", "source: gh:owner/repo", "template: my-spec"} {
		if !strings.Contains(after, want) {
			t.Errorf("update 后 config.yaml 丢失 %q：\n%s", want, after)
		}
	}
	if !strings.Contains(out.String(), "模板市场刷新尚未接入") {
		t.Errorf("registry.spec 存在但刷新未接入时应警告：\n%s", out.String())
	}
	for _, rel := range []string{".claude/commands/sunshine/mycmd.md", ".claude/skills/sunshine-custom/SKILL.md",
		".claude/agents/sunshine-custom.md", ".claude/hooks.json"} {
		if _, err := os.Stat(filepath.Join(dir, filepath.FromSlash(rel))); err != nil {
			t.Errorf("用户自加文件 %s 不应被覆盖或删除", rel)
		}
	}
	// 用户自加文件不应进哈希清单（只追踪模板写出的文件）。
	hashes := LoadHashes(dir)
	for k := range hashes {
		if strings.Contains(k, "mycmd") || strings.Contains(k, "sunshine-custom") {
			t.Errorf("用户自加文件 %s 不应进哈希清单", k)
		}
	}
}

func TestRunPreservesStatusLine(t *testing.T) {
	dir := setupProject(t)
	// 用户装了 statusline（init --with-statusline 的结果）：
	// settings.json 在清单哈希里含 statusLine。
	settings := readFile(t, dir, ".claude/settings.json")
	withStatus := strings.TrimSuffix(settings, "}\n") + ",\n  \"statusLine\": {\n    \"type\": \"command\",\n    \"command\": \"my-statusline\"\n  }\n}\n"
	writeFile(t, dir, ".claude/settings.json", withStatus)
	// 把清单哈希记为“当前内容”（模拟 init 后状态），再改 hooks 触发冲突。
	if err := UpdateHashes(dir, map[string]string{ClaudeSettingsPath: withStatus}); err != nil {
		t.Fatal(err)
	}
	// 模拟模板更新：改动 hooks 段使磁盘内容与模板不同且哈希不匹配 → 冲突；
	// 覆盖后 statusLine 必须保留。
	// 直接改一个模板键的值，制造冲突。
	conflicted := strings.Replace(withStatus, "SUNSHINE_HOOK_PLATFORM=claude sunshine hook workflow-state",
		"SUNSHINE_HOOK_PLATFORM=claude sunshine hook workflow-state-v2", 1)
	if conflicted == withStatus {
		t.Fatal("测试前置：未能制造冲突内容")
	}
	writeFile(t, dir, ".claude/settings.json", conflicted)
	summary, err := Run(dir, Options{CLIVersion: "0.2.0", Force: true, Prompter: fakePrompter{proceed: true}, Out: &bytes.Buffer{}})
	if err != nil {
		t.Fatal(err)
	}
	if summary.Updated == 0 && summary.AutoUpdated == 0 {
		t.Errorf("settings.json 应被处理，实际 %+v", summary)
	}
	after := readFile(t, dir, ".claude/settings.json")
	if !strings.Contains(after, "my-statusline") {
		t.Errorf("覆盖后用户 statusLine 应保留：\n%s", after)
	}
	if !strings.Contains(after, "SUNSHINE_HOOK_PLATFORM=claude sunshine hook workflow-state\"") {
		t.Errorf("覆盖后应恢复模板 hooks：\n%s", after)
	}
}

func TestRunMergesMissingHooksOnConflict(t *testing.T) {
	dir := setupProject(t)
	// 模拟旧版 init 的 settings.json：新模板去掉 SubagentStop 段。
	fresh := readFile(t, dir, ".claude/settings.json")
	var parsed map[string]any
	if err := json.Unmarshal([]byte(fresh), &parsed); err != nil {
		t.Fatal(err)
	}
	delete(parsed["hooks"].(map[string]any), "SubagentStop")
	old, err := json.MarshalIndent(parsed, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	oldStr := string(old) + "\n"
	// 清单哈希记为旧版内容（模拟旧版 init 后的状态）。
	if err := UpdateHashes(dir, map[string]string{ClaudeSettingsPath: oldStr}); err != nil {
		t.Fatal(err)
	}
	// 用户随后自加一个 Stop hook → 磁盘与清单不一致，触发冲突。
	var userParsed map[string]any
	if err := json.Unmarshal([]byte(oldStr), &userParsed); err != nil {
		t.Fatal(err)
	}
	userParsed["hooks"].(map[string]any)["Stop"] = []any{
		map[string]any{"hooks": []any{map[string]any{"command": "my-notify", "timeout": 5, "type": "command"}}},
	}
	user, err := json.MarshalIndent(userParsed, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	writeFile(t, dir, ".claude/settings.json", string(user)+"\n")
	// 冲突覆盖：应补上 SubagentStop，同时保留用户自加 hook。
	var out bytes.Buffer
	summary, err := Run(dir, Options{CLIVersion: "0.2.0", Force: true, Prompter: fakePrompter{proceed: true}, Out: &out})
	if err != nil {
		t.Fatal(err)
	}
	if summary.Updated == 0 {
		t.Errorf("settings.json 应被覆盖处理，实际 %+v", summary)
	}
	after := readFile(t, dir, ".claude/settings.json")
	if !strings.Contains(after, "SUNSHINE_HOOK_PLATFORM=claude sunshine hook subagent-stop") {
		t.Errorf("覆盖后应补上 SubagentStop：\n%s", after)
	}
	if !strings.Contains(after, `"my-notify"`) {
		t.Errorf("覆盖后用户自加 hook 应保留：\n%s", after)
	}
	if !strings.Contains(after, "SUNSHINE_HOOK_PLATFORM=claude sunshine hook session-start") {
		t.Errorf("覆盖后模板 hook 应在：\n%s", after)
	}
}

func TestRunUpdateSkip(t *testing.T) {
	dir := setupProject(t)
	// .claude/commands 加入 update.skip，改动其下文件后 update 不应碰它。
	cfgPath := filepath.Join(dir, ".sunshine", "config.yaml")
	cfg, _ := os.ReadFile(cfgPath)
	if err := os.WriteFile(cfgPath, append(cfg, []byte("\nupdate:\n  skip:\n    - .claude/commands\n")...), 0o644); err != nil {
		t.Fatal(err)
	}
	writeFile(t, dir, ".claude/commands/sunshine/continue.md", "用户改过的命令\n")
	summary, err := Run(dir, Options{CLIVersion: "0.2.0", Force: true, Prompter: fakePrompter{proceed: true}, Out: &bytes.Buffer{}})
	if err != nil {
		t.Fatal(err)
	}
	if got := readFile(t, dir, ".claude/commands/sunshine/continue.md"); got != "用户改过的命令\n" {
		t.Errorf("update.skip 内的文件不应被覆盖，实际 %q", got)
	}
	_ = summary
}

func TestRunUserDeletedRespected(t *testing.T) {
	dir := setupProject(t)
	if err := os.Remove(filepath.Join(dir, ".sunshine", "agents", "plan.md")); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	summary, err := Run(dir, Options{CLIVersion: "0.2.0", Force: true, Prompter: fakePrompter{proceed: true}, Out: &out})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, ".sunshine", "agents", "plan.md")); !os.IsNotExist(err) {
		t.Error("用户删除的文件不应被恢复")
	}
	if !strings.Contains(out.String(), "你已删除") {
		t.Errorf("应提示用户已删除：\n%s", out.String())
	}
	_ = summary
}

func TestRunDowngradeRefused(t *testing.T) {
	dir := setupProject(t)
	if _, err := Run(dir, Options{CLIVersion: "0.0.1", Out: &bytes.Buffer{}}); err == nil {
		t.Error("CLI 比项目旧时应拒绝")
	}
	// --allow-downgrade 后放行。
	summary, err := Run(dir, Options{CLIVersion: "0.0.1", AllowDowngrade: true, Force: true, Out: &bytes.Buffer{}})
	if err != nil {
		t.Fatalf("--allow-downgrade 应放行：%v", err)
	}
	if got := GetInstalledVersion(dir); got != "0.0.1" {
		t.Errorf(".version 应降级为 0.0.1，实际 %s", got)
	}
	_ = summary
}

func TestRunRequiresSunshineDir(t *testing.T) {
	if _, err := Run(t.TempDir(), Options{CLIVersion: "0.1.0", Out: &bytes.Buffer{}}); err == nil {
		t.Error("未初始化的目录应报错")
	}
}

func TestRunCodexUpgrade(t *testing.T) {
	// 旧版布局：只有 .sunshine/ 与 .agents/skills/ 标记，无 .codex/。
	dir := t.TempDir()
	if err := setupLegacyCodexProject(dir); err != nil {
		t.Fatal(err)
	}
	summary, err := Run(dir, Options{CLIVersion: "0.2.0", Force: true, Prompter: fakePrompter{proceed: true}, Out: &bytes.Buffer{}})
	if err != nil {
		t.Fatalf("Run 失败：%v", err)
	}
	hooks := readFile(t, dir, ".codex/hooks.json")
	if !strings.Contains(hooks, "sunshine hook session-start") {
		t.Errorf(".codex/hooks.json 应含 SessionStart 接线：\n%s", hooks)
	}
	if _, err := os.Stat(filepath.Join(dir, ".codex", "config.toml")); err != nil {
		t.Error(".codex/config.toml 应被补建")
	}
	_ = summary
}

func setupLegacyCodexProject(dir string) error {
	// 用真实 init 建 .sunshine/ 骨架。
	if err := scaffold.Init(dir, "测试", false); err != nil {
		return err
	}
	// 旧版 Codex 技能文件（内容任意）+ 清单标记。
	for _, name := range []string{"continue", "finish-work"} {
		p := filepath.Join(dir, ".agents", "skills", "sunshine-"+name, "SKILL.md")
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(p, []byte("# 旧版 skill\n"), 0o644); err != nil {
			return err
		}
	}
	hashes := LoadHashes(dir)
	hashes[".agents/skills/sunshine-continue/SKILL.md"] = ComputeHashString("# 旧版 skill\n")
	hashes[".agents/skills/sunshine-finish-work/SKILL.md"] = ComputeHashString("# 旧版 skill\n")
	return SaveHashes(dir, hashes)
}

func TestRunMigrationGateAndSafeDelete(t *testing.T) {
	dir := setupProject(t)
	// 废弃文件：内容哈希命中 allowed_hashes → 无 --migrate 也自动删。
	deprecated := ".claude/commands/sunshine/old-cmd.md"
	writeFile(t, dir, deprecated, "旧命令\n")
	deprecatedHash := ComputeHashString("旧命令\n")
	// 真实清单集：0.1.0 份含一条孤儿改名（源存在、目标不存在、
	// 不在 (0.1.0, 0.2.0] 区间内）；0.2.0 份含破坏性改名与 safe-file-delete。
	reg := migrate.NewRegistry()
	if err := reg.Add(migrate.MigrationManifest{
		Version: "0.1.0",
		Migrations: []migrate.MigrationItem{
			{Type: migrate.MigrationTypeRename, From: ".claude/skills/sunshine-brainstorm/SKILL.md", To: ".claude/skills/sunshine-brainstorm-v2/SKILL.md", Description: "孤儿改名"},
		},
	}); err != nil {
		t.Fatal(err)
	}
	if err := reg.Add(migrate.MigrationManifest{
		Version:          "0.2.0",
		Breaking:         true,
		RecommendMigrate: true,
		Changelog:        "旧命令改名。",
		Migrations: []migrate.MigrationItem{
			{Type: migrate.MigrationTypeRename, From: ".claude/skills/sunshine-check/SKILL.md", To: ".claude/skills/sunshine-check-v2/SKILL.md", Description: "改名"},
			{Type: migrate.MigrationTypeSafeFileDelete, From: deprecated, Description: "删除已废弃的旧命令", AllowedHashes: []string{deprecatedHash}},
		},
	}); err != nil {
		t.Fatal(err)
	}
	old := MigrationRegistry
	MigrationRegistry = reg
	defer func() { MigrationRegistry = old }()

	// 无 --migrate 时破坏性迁移硬停。
	if _, err := Run(dir, Options{CLIVersion: "0.2.0", Force: true, Out: &bytes.Buffer{}}); err == nil {
		t.Error("破坏性迁移无 --migrate 时应硬停")
	}
	// --dry-run 时硬停不触发（只预览）。
	if _, err := Run(dir, Options{CLIVersion: "0.2.0", DryRun: true, Force: true, Out: &bytes.Buffer{}}); err != nil {
		t.Errorf("dry-run 不应硬停：%v", err)
	}
	// --migrate 时执行迁移 + safe-file-delete。
	var out bytes.Buffer
	if _, err := Run(dir, Options{CLIVersion: "0.2.0", Migrate: true, Force: true, Out: &out}); err != nil {
		t.Fatalf("--migrate 应放行：%v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, ".claude", "skills", "sunshine-check-v2", "SKILL.md")); err != nil {
		t.Error("改名迁移应执行：新路径文件不存在")
	}
	if _, err := os.Stat(filepath.Join(dir, ".claude", "skills", "sunshine-check", "SKILL.md")); !os.IsNotExist(err) {
		t.Error("改名迁移应执行：旧路径文件应已搬走")
	}
	if _, err := os.Stat(filepath.Join(dir, ".claude", "skills", "sunshine-brainstorm-v2", "SKILL.md")); err != nil {
		t.Error("孤儿迁移应随 --migrate 一并执行")
	}
	if _, err := os.Stat(filepath.Join(dir, filepath.FromSlash(deprecated))); !os.IsNotExist(err) {
		t.Error("safe-file-delete 命中的废弃文件应被删除")
	}
	// 孤儿检测函数本身（真实实现）。
	orphans := migrate.FindOrphanMigrations(dir, reg.AllMigrations(), reg.MigrationsForVersion("0.2.0", "0.2.0"))
	_ = orphans
}

func TestFindOrphanMigrations(t *testing.T) {
	dir := setupProject(t)
	orphanSrc := ".claude/skills/sunshine-brainstorm/SKILL.md"
	orphan := migrate.MigrationItem{Type: migrate.MigrationTypeRename, From: orphanSrc, To: ".claude/skills/sunshine-brainstorm-v2/SKILL.md"}
	orphans := migrate.FindOrphanMigrations(dir, []migrate.MigrationItem{orphan}, nil)
	if len(orphans) != 1 || orphans[0].From != orphanSrc {
		t.Errorf("孤儿迁移检测失败：%v", orphans)
	}
	// 目标已存在时不是孤儿。
	writeFile(t, dir, ".claude/skills/sunshine-brainstorm-v2/SKILL.md", "新版\n")
	if orphans := migrate.FindOrphanMigrations(dir, []migrate.MigrationItem{orphan}, nil); len(orphans) != 0 {
		t.Errorf("目标已存在时不应是孤儿：%v", orphans)
	}
}

func TestRunNoTrellisResidue(t *testing.T) {
	dir := setupProject(t)
	writeFile(t, dir, ".sunshine/agents/check.md", "用户改过的检查\n")
	if _, err := Run(dir, Options{CLIVersion: "0.2.0", Force: true, Prompter: fakePrompter{proceed: true}, Out: &bytes.Buffer{}}); err != nil {
		t.Fatal(err)
	}
	// 生成/更新后的托管文件里不得有 trellis 字样。
	var bad []string
	_ = filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return nil
		}
		rel, _ := filepath.Rel(dir, path)
		if strings.Contains(rel, ".backups") || strings.Contains(rel, ".template-hashes.json") {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return nil
		}
		if strings.Contains(string(data), "trellis") || strings.Contains(string(data), "Trellis") {
			bad = append(bad, rel)
		}
		return nil
	})
	if len(bad) > 0 {
		t.Errorf("更新后文件残留 trellis 字样：%v", bad)
	}
}

// fakeRefresher 是测试用的 registry 刷新替身。
type fakeRefresher struct {
	files map[string]string
	err   error
}

func (f fakeRefresher) RefreshSpecTemplates(source, template string) (map[string]string, error) {
	return f.files, f.err
}

// TestCollectRegistrySpecTemplatesRefresh：registry.spec 刷新成功时，
// 刷新出的 spec 文件进入模板集合；刷新失败时只警告、不中断。
func TestCollectRegistrySpecTemplatesRefresh(t *testing.T) {
	dir := setupProject(t)
	cfgPath := filepath.Join(dir, ".sunshine", "config.yaml")
	cfg, _ := os.ReadFile(cfgPath)
	if err := os.WriteFile(cfgPath, append(cfg, []byte("\nregistry:\n  spec:\n    source: gh:owner/repo\n    template: my-spec\n")...), 0o644); err != nil {
		t.Fatal(err)
	}
	oldRefresh := RegistryRefresh
	defer func() { RegistryRefresh = oldRefresh }()

	// 成功：刷新出的 spec 文件在模板集合里。
	RegistryRefresh = fakeRefresher{files: map[string]string{".sunshine/spec/from-registry.md": "市场 spec\n"}}
	var out bytes.Buffer
	templates, _, err := CollectTemplateFiles(&out, dir, LoadHashes(dir), CollectOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if templates[".sunshine/spec/from-registry.md"] != "市场 spec\n" {
		t.Error("刷新出的 spec 文件应进入模板集合")
	}
	// 失败：只警告，集合里没有它。
	RegistryRefresh = fakeRefresher{err: fmt.Errorf("模拟断网")}
	out.Reset()
	templates, _, err = CollectTemplateFiles(&out, dir, LoadHashes(dir), CollectOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := templates[".sunshine/spec/from-registry.md"]; ok {
		t.Error("刷新失败时不应有该文件")
	}
	if !strings.Contains(out.String(), "刷新跳过") {
		t.Errorf("刷新失败时应警告，实际：\n%s", out.String())
	}
	// 默认实现就是 B 组的 registry.Refresher（已接线，非 nil）。
	if oldRefresh == nil {
		t.Error("RegistryRefresh 默认应为 B 组的 registry.Refresher，不应为 nil")
	}
}
