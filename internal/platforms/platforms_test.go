package platforms

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

func TestRegistry(t *testing.T) {
	reg := Registry()
	if len(reg) != 22 {
		t.Fatalf("注册表应有 22 个平台，实际 %d", len(reg))
	}
	// 只比核心登记字段（布局字段另由 TestInstall* 覆盖行为）；
	// Platform 含 slice 字段，不能直接用 != 比较。
	type core struct {
		Name, Flag, ConfigDir, CmdRefPrefix, executorAI, userActionLabel string
		DefaultChecked, AgentCapable, HasHooks, SupportsAgentSkills      bool
	}
	want := map[string]core{
		"claude-code": {"Claude Code", "claude", ".claude", "/sunshine:", "Bash scripts or Task calls", "Slash commands", true, true, true, false},
		"cursor":      {"Cursor", "cursor", ".cursor", "/sunshine-", "Bash scripts or Task calls", "Slash commands", true, true, true, false},
		"codex":       {"Codex", "codex", ".codex", "$", "Bash scripts or tool calls", "Skills", false, true, false, true},
		"kiro":        {"Kiro Code", "kiro", ".kiro/skills", "$", "Bash scripts or tool calls", "Skills", false, true, true, false},
		"gemini":      {"Gemini CLI", "gemini", ".gemini", "/sunshine:", "Bash scripts or tool calls", "Slash commands", false, true, true, true},
		"qoder":       {"Qoder", "qoder", ".qoder", "$", "Bash scripts or tool calls", "Skills", false, true, true, false},
		"pi":          {"Pi Agent", "pi", ".pi", "/sunshine-", "Bash scripts or tool calls", "Slash commands", false, true, true, true},
		"zcode":       {"ZCode", "zcode", ".zcode", "/sunshine:", "Bash scripts or Agent calls", "Skills", false, true, true, false},
		"trae":        {"Trae", "trae", ".trae", "/sunshine-", "Bash scripts or tool calls", "Commands", false, true, true, false},
		"omp":         {"Oh My Pi", "omp", ".omp", "/sunshine:", "Bash scripts or Task calls", "Slash commands", false, true, true, false},
		"snow":        {"Snow CLI", "snow", ".snow/skills", "/sunshine-", "Bash scripts or Agent calls", "Skills", false, true, true, false},
		"opencode":    {"OpenCode", "opencode", ".opencode", "/sunshine:", "Bash scripts or Task calls", "Slash commands", false, true, false, false},
		"dsh":         {"DeepSeek Harness", "dsh", ".dsh", "sunshine-", "Bash scripts or tool calls", "Skills", false, true, false, true},
		"reasonix":    {"Reasonix", "reasonix", ".reasonix", "/skill sunshine-", "Bash scripts or tool calls", "Skills", false, true, false, false},
		"grok":        {"Grok Build", "grok", ".grok", "/sunshine-", "Bash scripts or Agent calls", "Skills", false, true, false, false},
		"kimi":        {"Kimi Code", "kimi", ".kimi-code", "/skill:sunshine-", "Bash scripts or Agent calls", "Slash commands", false, true, false, true},
		"kilo":        {"Kilo CLI", "kilo", ".kilocode", "/sunshine:", "Bash scripts or file reads", "Workflows", false, false, false, false},
		"antigravity": {"Antigravity", "antigravity", ".agent/workflows", "/", "Bash scripts or file reads", "Workflows", false, false, false, false},
		"devin":       {"Devin", "devin", ".devin/workflows", "/sunshine-", "Bash scripts or file reads", "Workflows", false, false, false, false},
		"codebuddy":   {"CodeBuddy", "codebuddy", ".codebuddy", "/sunshine:", "Bash scripts or Task calls", "Slash commands", false, true, true, false},
		"copilot":     {"GitHub Copilot", "copilot", ".github/copilot", "/", "Bash scripts or tool calls", "Prompts", false, true, true, false},
		"droid":       {"Factory Droid", "droid", ".factory", "/sunshine-", "Bash scripts or Task calls", "Slash commands", false, true, true, false},
	}
	for _, p := range reg {
		w, ok := want[p.ID]
		if !ok {
			t.Errorf("注册表出现未知平台 %q", p.ID)
			continue
		}
		got := core{p.Name, p.Flag, p.ConfigDir, p.CmdRefPrefix, p.executorAI, p.userActionLabel, p.DefaultChecked, p.AgentCapable, p.HasHooks, p.SupportsAgentSkills}
		if got != w {
			t.Errorf("平台 %s 的注册信息不对：%+v，期望 %+v", p.ID, got, w)
		}
		if p.SkillsDir == "" {
			t.Errorf("平台 %s 未登记 SkillsDir", p.ID)
		}
		if p.CmdForm == "" {
			t.Errorf("平台 %s 未登记 CmdForm", p.ID)
		}
	}
	if got, ok := ByID("cursor"); !ok || got.Name != "Cursor" {
		t.Errorf("ByID(cursor) = %+v, %v", got, ok)
	}
	if _, ok := ByID("opencode"); !ok {
		t.Error("ByID(opencode) 应找到（滚动轨道已接）")
	}
	for _, id := range []string{"codebuddy", "copilot", "droid"} {
		if _, ok := ByID(id); !ok {
			t.Errorf("ByID(%s) 应找到（E 路已接）", id)
		}
	}
}

func TestDefaultSelected(t *testing.T) {
	got := DefaultSelected()
	if len(got) != 2 || got[0] != "claude-code" || got[1] != "cursor" {
		t.Errorf("DefaultSelected() = %v，期望 [claude-code cursor]", got)
	}
}

func TestCodexHooksHint(t *testing.T) {
	hint := CodexHooksHint()
	for _, sub := range []string{"features.hooks = true", "codex_hooks = true", "/hooks", "~/.codex/config.toml"} {
		if !strings.Contains(hint, sub) {
			t.Errorf("CodexHooksHint 缺少 %q：%s", sub, hint)
		}
	}
}

func TestResolvePlaceholders(t *testing.T) {
	claude, _ := ByID("claude-code")
	cursor, _ := ByID("cursor")
	codex, _ := ByID("codex")

	src := "用 {{CMD_REF:finish-work}}，平台 {{CLI_FLAG}}，{{EXECUTOR_AI}}，{{USER_ACTION_LABEL}}。"
	if got := resolvePlaceholders(src, claude); !strings.Contains(got, "/sunshine:finish-work") || !strings.Contains(got, "claude") {
		t.Errorf("Claude 解析不对：%s", got)
	}
	if got := resolvePlaceholders(src, cursor); !strings.Contains(got, "/sunshine-finish-work") || !strings.Contains(got, "cursor") {
		t.Errorf("Cursor 解析不对：%s", got)
	}
	if got := resolvePlaceholders(src, codex); !strings.Contains(got, "$finish-work") || !strings.Contains(got, "codex") {
		t.Errorf("Codex 解析不对：%s", got)
	}
	// 中性解析：CMD_REF 与平台无关。
	if got := resolvePlaceholdersNeutral(src, codex); !strings.Contains(got, "`finish-work` (Sunshine command)") {
		t.Errorf("中性解析不对：%s", got)
	}
	if got := resolvePlaceholdersNeutral(src, claude); !strings.Contains(got, "`finish-work` (Sunshine command)") {
		t.Errorf("中性解析（Claude 上下文）不对：%s", got)
	}
	// 条件块：Codex 的 HAS_HOOKS 为假。
	cond := "A{{#HAS_HOOKS}}有钩子{{/HAS_HOOKS}}{{^HAS_HOOKS}}无钩子{{/HAS_HOOKS}}B"
	if got := resolvePlaceholders(cond, claude); got != "A有钩子B" {
		t.Errorf("Claude 条件块不对：%q", got)
	}
	if got := resolvePlaceholders(cond, codex); got != "A无钩子B" {
		t.Errorf("Codex 条件块不对：%q", got)
	}
	// 条件块删除后多余空行被压成两个换行。
	blank := "A\n\n\n\n{{#HAS_HOOKS}}x{{/HAS_HOOKS}}\n\nB"
	if got := resolvePlaceholders(blank, codex); strings.Contains(got, "\n\n\n") {
		t.Errorf("多余空行未压缩：%q", got)
	}
}

func TestFindLeftoverPlaceholder(t *testing.T) {
	// 文档措辞中提到的非占位符 token（Trellis 原文如此）不应误报。
	doc := "任何 `{{platform_name}}`、`{{python_cmd}}` 等 token 都会按平台替换。"
	if got := findLeftoverPlaceholder(doc); got != "" {
		t.Errorf("文档提及不应视为残留占位符，实际 %q", got)
	}
	for _, s := range []string{"{{CMD_REF:finish-work}}", "{{CLI_FLAG}}", "{{PYTHON_CMD}}", "{{#HAS_HOOKS}}", "{{/HAS_HOOKS}}", "{{EXECUTOR_AI}}", "{{USER_ACTION_LABEL}}"} {
		if got := findLeftoverPlaceholder("x" + s + "y"); got != s {
			t.Errorf("应检出残留占位符 %q，实际 %q", s, got)
		}
	}
}

func TestWrapSkillFrontmatter(t *testing.T) {
	out, err := wrapSkillFrontmatter("sunshine-check", "正文")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(out, "---\nname: sunshine-check\ndescription: \"") {
		t.Errorf("frontmatter 格式不对：\n%s", out)
	}
	if !strings.Contains(out, "全面质量核查") {
		t.Errorf("description 未译成中文：\n%s", out)
	}
	if _, err := wrapSkillFrontmatter("sunshine-unknown", "正文"); err == nil {
		t.Error("未登记的 skill 应报错")
	}
}

// TestInstallBundledSkills 覆盖内置多文件 skill 的写出（B 组目录已合入）。
func TestInstallBundledSkills(t *testing.T) {
	if !BundledSkillsPresent() {
		t.Skip("bundled-skills 目录尚未合入，跳过")
	}
	for _, tc := range []struct {
		platform string
		root     string
	}{
		{"claude-code", ".claude/skills"},
		{"cursor", ".cursor/skills"},
		{"codex", ".agents/skills"},
	} {
		dir := t.TempDir()
		files, err := Install(dir, tc.platform, InstallOptions{})
		if err != nil {
			t.Fatalf("安装 %s 失败：%v", tc.platform, err)
		}
		for _, base := range []string{"channel", "meta", "spec-bootstrap", "session-insight"} {
			skillPath := tc.root + "/sunshine-" + base + "/SKILL.md"
			if !containsAll(files, skillPath) {
				t.Errorf("%s 未写出 %s（返回 %v）", tc.platform, skillPath, files)
				continue
			}
			data, err := os.ReadFile(filepath.Join(dir, filepath.FromSlash(skillPath)))
			if err != nil {
				t.Fatal(err)
			}
			if !strings.HasPrefix(string(data), "---\nname: sunshine-"+base+"\n") {
				t.Errorf("%s 的 %s frontmatter 未改名：\n%.200s", tc.platform, skillPath, data)
			}
		}
		// references 应整目录复制（以 meta 为例，B 组共 38 个文件）。
		tree := readTree(t, dir)
		metaRefs := 0
		for rel := range tree {
			if strings.HasPrefix(rel, tc.root+"/sunshine-meta/references/") {
				metaRefs++
			}
		}
		if metaRefs == 0 {
			t.Errorf("%s 的 sunshine-meta references 未写出", tc.platform)
		}
		assertClean(t, tree)
	}
}

// TestHookCommandPlatformPrefix：三份 hook 配置里的每条 hook 命令
// 都必须带 SUNSHINE_HOOK_PLATFORM=<平台> 前缀（C 组接线要求），
// 且不得出现无前缀的 sunshine hook 命令。
func TestHookCommandPlatformPrefix(t *testing.T) {
	cases := []struct {
		platform string
		files    map[string]string
	}{
		{"claude", map[string]string{".claude/settings.json": ""}},
		{"cursor", map[string]string{".cursor/hooks.json": ""}},
		{"codex", map[string]string{".codex/hooks.json": ""}},
	}
	for _, tc := range cases {
		dir := t.TempDir()
		id := map[string]string{"claude": "claude-code", "cursor": "cursor", "codex": "codex"}[tc.platform]
		opts := InstallOptions{}
		if tc.platform == "claude" {
			opts.WithStatusline = true // statusLine 也应带前缀
		}
		if _, err := Install(dir, id, opts); err != nil {
			t.Fatal(err)
		}
		for rel := range tc.files {
			data, err := os.ReadFile(filepath.Join(dir, filepath.FromSlash(rel)))
			if err != nil {
				t.Fatal(err)
			}
			content := string(data)
			want := "SUNSHINE_HOOK_PLATFORM=" + tc.platform + " sunshine hook "
			if n := strings.Count(content, want); n == 0 {
				t.Errorf("%s 的 %s 缺少带前缀的 hook 命令", tc.platform, rel)
			}
			// 逐条检查 "sunshine hook" 出现处：前面必须是对应前缀。
			for _, idx := range regexp.MustCompile(`sunshine hook [a-z-]+`).FindAllStringIndex(content, -1) {
				start := idx[0]
				prefix := "SUNSHINE_HOOK_PLATFORM=" + tc.platform + " "
				if start < len(prefix) || content[start-len(prefix):start] != prefix {
					t.Errorf("%s 的 %s 存在无前缀的 hook 命令：%.60s", tc.platform, rel, content[start:])
				}
			}
		}
	}
}

func TestInstallUnknownPlatform(t *testing.T) {
	if _, err := Install(t.TempDir(), "not-a-platform", InstallOptions{}); err == nil {
		t.Error("未知平台应报错")
	}
}

// readTree 返回 dir 下全部文件的相对路径（正斜杠）与内容。
func readTree(t *testing.T, dir string) map[string]string {
	t.Helper()
	out := map[string]string{}
	err := filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(dir, path)
		if err != nil {
			return err
		}
		out[filepath.ToSlash(rel)] = string(data)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return out
}

// assertNoTrellisOrPlaceholder 断言生成文件无 trellis 残留、无残留占位符、无 .py。
func assertClean(t *testing.T, files map[string]string) {
	t.Helper()
	for rel, content := range files {
		if strings.Contains(strings.ToLower(content), "trellis") {
			t.Errorf("%s 残留 trellis 字样", rel)
		}
		if leftover := findLeftoverPlaceholder(content); leftover != "" {
			t.Errorf("%s 残留占位符 %s", rel, leftover)
		}
		if strings.HasSuffix(rel, ".py") {
			t.Errorf("不应写出 .py 文件：%s", rel)
		}
		// Python 调用检查只针对 A 组自己的模板：B 组的 bundled-skills
		// references 里仍有 python3 脚本调用原文（B 组翻译口径，见 NOTES.md），
		// 不归 A 组改，此处不误报。
		isBundled := strings.Contains(rel, "/sunshine-channel/") || strings.Contains(rel, "/sunshine-meta/") ||
			strings.Contains(rel, "/sunshine-spec-bootstrap/") || strings.Contains(rel, "/sunshine-session-insight/")
		if !isBundled && (strings.Contains(content, "python3") || strings.Contains(content, "python ")) {
			t.Errorf("%s 残留 python 命令", rel)
		}
	}
}

func containsAll(list []string, want ...string) bool {
	set := map[string]bool{}
	for _, v := range list {
		set[v] = true
	}
	for _, w := range want {
		if !set[w] {
			return false
		}
	}
	return true
}

func TestInstallClaude(t *testing.T) {
	dir := t.TempDir()
	files, err := Install(dir, "claude-code", InstallOptions{})
	if err != nil {
		t.Fatal(err)
	}
	want := []string{
		".claude/skills/sunshine-before-dev/SKILL.md",
		".claude/skills/sunshine-brainstorm/SKILL.md",
		".claude/skills/sunshine-break-loop/SKILL.md",
		".claude/skills/sunshine-check/SKILL.md",
		".claude/skills/sunshine-update-spec/SKILL.md",
		".claude/commands/sunshine/continue.md",
		".claude/commands/sunshine/finish-work.md",
		".claude/agents/sunshine-implement.md",
		".claude/agents/sunshine-check.md",
		".claude/agents/sunshine-research.md",
		".claude/settings.json",
	}
	if !containsAll(files, want...) {
		t.Errorf("Claude 返回路径缺失：返回 %v，期望包含 %v", files, want)
	}
	for _, p := range files {
		if strings.Contains(p, "start") {
			t.Errorf("Claude 不应生成 start 相关文件：%s", p)
		}
	}
	tree := readTree(t, dir)
	assertClean(t, tree)
	settings := tree[".claude/settings.json"]
	for _, sub := range []string{"SUNSHINE_HOOK_PLATFORM=claude sunshine hook session-start", "SUNSHINE_HOOK_PLATFORM=claude sunshine hook subagent-context", "SUNSHINE_HOOK_PLATFORM=claude sunshine hook workflow-state"} {
		if !strings.Contains(settings, sub) {
			t.Errorf("settings.json 缺少 %q", sub)
		}
	}
	if strings.Contains(settings, "statusLine") {
		t.Error("默认不应注入 statusLine")
	}
	skill := tree[".claude/skills/sunshine-check/SKILL.md"]
	if !strings.HasPrefix(skill, "---\nname: sunshine-check\n") {
		t.Errorf("skill frontmatter 不对：\n%.200s", skill)
	}
	if !strings.Contains(tree[".claude/commands/sunshine/continue.md"], "sunshine task context") {
		t.Error("continue 命令未译成 sunshine task 调用")
	}
}

func TestInstallClaudeWithStatusline(t *testing.T) {
	dir := t.TempDir()
	if _, err := Install(dir, "claude-code", InstallOptions{WithStatusline: true}); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(dir, ".claude", "settings.json"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), `"SUNSHINE_HOOK_PLATFORM=claude sunshine hook statusline"`) {
		t.Errorf("WithStatusline 未注入 statusLine：\n%s", data)
	}
}

func TestInstallCursor(t *testing.T) {
	dir := t.TempDir()
	files, err := Install(dir, "cursor", InstallOptions{})
	if err != nil {
		t.Fatal(err)
	}
	want := []string{
		".cursor/skills/sunshine-before-dev/SKILL.md",
		".cursor/skills/sunshine-brainstorm/SKILL.md",
		".cursor/skills/sunshine-break-loop/SKILL.md",
		".cursor/skills/sunshine-check/SKILL.md",
		".cursor/skills/sunshine-update-spec/SKILL.md",
		".cursor/commands/sunshine-continue.md",
		".cursor/commands/sunshine-finish-work.md",
		".cursor/agents/sunshine-implement.md",
		".cursor/agents/sunshine-check.md",
		".cursor/agents/sunshine-research.md",
		".cursor/hooks.json",
	}
	if !containsAll(files, want...) {
		t.Errorf("Cursor 返回路径缺失：返回 %v，期望包含 %v", files, want)
	}
	tree := readTree(t, dir)
	assertClean(t, tree)
	hooks := tree[".cursor/hooks.json"]
	for _, sub := range []string{"SUNSHINE_HOOK_PLATFORM=cursor sunshine hook session-start", "SUNSHINE_HOOK_PLATFORM=cursor sunshine hook subagent-context", "SUNSHINE_HOOK_PLATFORM=cursor sunshine hook shell-session"} {
		if !strings.Contains(hooks, sub) {
			t.Errorf("cursor hooks.json 缺少 %q", sub)
		}
	}
	// Cursor 命令用扁平命名与 /sunshine- 前缀。
	cmd := tree[".cursor/commands/sunshine-continue.md"]
	if !strings.Contains(cmd, "--platform cursor") {
		t.Errorf("Cursor continue 命令的 CLI_FLAG 未解析为 cursor：\n%.300s", cmd)
	}
	// WithStatusline 对 Cursor 无效。
	if _, err := Install(dir, "cursor", InstallOptions{WithStatusline: true, Force: true}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, ".cursor", "settings.json")); !os.IsNotExist(err) {
		t.Error("Cursor 不应写 settings.json")
	}
}

func TestInstallCodex(t *testing.T) {
	dir := t.TempDir()
	files, err := Install(dir, "codex", InstallOptions{})
	if err != nil {
		t.Fatal(err)
	}
	want := []string{
		".agents/skills/sunshine-before-dev/SKILL.md",
		".agents/skills/sunshine-brainstorm/SKILL.md",
		".agents/skills/sunshine-break-loop/SKILL.md",
		".agents/skills/sunshine-check/SKILL.md",
		".agents/skills/sunshine-update-spec/SKILL.md",
		".agents/skills/sunshine-start/SKILL.md",
		".agents/skills/sunshine-continue/SKILL.md",
		".agents/skills/sunshine-finish-work/SKILL.md",
		".codex/agents/sunshine-implement.toml",
		".codex/agents/sunshine-check.toml",
		".codex/agents/sunshine-research.toml",
		".codex/hooks.json",
		".codex/config.toml",
	}
	if !containsAll(files, want...) {
		t.Errorf("Codex 返回路径缺失：返回 %v，期望包含 %v", files, want)
	}
	tree := readTree(t, dir)
	assertClean(t, tree)
	// .codex/skills/ 空目录存在。
	if st, err := os.Stat(filepath.Join(dir, ".codex", "skills")); err != nil || !st.IsDir() {
		t.Error(".codex/skills/ 空目录不存在")
	}
	// hooks.json：无 -X utf8，matcher 已改名。
	hooks := tree[".codex/hooks.json"]
	if strings.Contains(hooks, "-X utf8") {
		t.Error("Codex hooks.json 不应含 -X utf8")
	}
	if !strings.Contains(hooks, "sunshine-implement|sunshine-check|sunshine-research") {
		t.Error("Codex hooks.json 的 SubagentStart matcher 未改名")
	}
	// SessionStart 已接线（2026-11-09 用户确认，Trellis 未注册属上游遗漏）。
	if !strings.Contains(hooks, "SessionStart") || !strings.Contains(hooks, "SUNSHINE_HOOK_PLATFORM=codex sunshine hook session-start") {
		t.Error("Codex hooks.json 未注册 SessionStart")
	}
	// config.toml：保留 [agents] max_depth = 1 与 multi_agent_v2 说明。
	cfg := tree[".codex/config.toml"]
	if !strings.Contains(cfg, "max_depth = 1") || !strings.Contains(cfg, "multi_agent_v2") {
		t.Error("Codex config.toml 缺少 max_depth / multi_agent_v2 说明")
	}
	// 共享 skill 用中性解析：多平台写同一目录字节一致。
	skill := tree[".agents/skills/sunshine-update-spec/SKILL.md"]
	if !strings.Contains(skill, "`break-loop` (Sunshine command)") {
		t.Errorf("Codex 共享 skill 未用中性 CMD_REF：\n%.500s", skill)
	}
	// agent toml 保留 marker-gated pull fallback 说明。
	if !strings.Contains(tree[".codex/agents/sunshine-implement.toml"], "<!-- sunshine-hook-injected -->") {
		t.Error("Codex agent 缺少 sunshine-hook-injected 标记说明")
	}
}

// TestInstallForceSemantics 覆盖普通文件的跳过/覆盖语义。
func TestInstallForceSemantics(t *testing.T) {
	dir := t.TempDir()
	// 先写一个用户已有的 skill 文件。
	custom := filepath.Join(dir, ".claude", "skills", "sunshine-check", "SKILL.md")
	if err := os.MkdirAll(filepath.Dir(custom), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(custom, []byte("用户自定义内容\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	// Force 为假：跳过，不在返回列表中，内容不动。
	files, err := Install(dir, "claude-code", InstallOptions{})
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range files {
		if f == ".claude/skills/sunshine-check/SKILL.md" {
			t.Error("Force 为假时已存在的文件不应在返回列表中")
		}
	}
	data, _ := os.ReadFile(custom)
	if string(data) != "用户自定义内容\n" {
		t.Error("Force 为假时已有文件被改动")
	}
	// Force 为真：覆盖，返回列表包含该路径。
	files, err = Install(dir, "claude-code", InstallOptions{Force: true})
	if err != nil {
		t.Fatal(err)
	}
	if !containsAll(files, ".claude/skills/sunshine-check/SKILL.md") {
		t.Error("Force 为真时覆盖的文件应在返回列表中")
	}
	data, _ = os.ReadFile(custom)
	if strings.Contains(string(data), "用户自定义内容") {
		t.Error("Force 为真时已有文件未被覆盖")
	}
}

// TestSettingsMerge 覆盖 settings.json 的普通模板文件语义（阶段 4 定稿）：
// 已有文件内容不同时 Force 为假跳过、为真覆盖，不再合并顶层键、env、hooks。
func TestSettingsMerge(t *testing.T) {
	dir := t.TempDir()
	settingsPath := filepath.Join(dir, ".claude", "settings.json")
	if err := os.MkdirAll(filepath.Dir(settingsPath), 0o755); err != nil {
		t.Fatal(err)
	}
	existing := `{
  "permissions": {
    "allow": ["Bash(npm run *)"]
  },
  "env": {
    "MY_VAR": "1"
  },
  "statusLine": {
    "type": "command",
    "command": "my-statusline"
  }
}
`
	if err := os.WriteFile(settingsPath, []byte(existing), 0o644); err != nil {
		t.Fatal(err)
	}
	// Force 为假：已有文件原样保留，不在返回列表中。
	files, err := Install(dir, "claude-code", InstallOptions{})
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range files {
		if f == ".claude/settings.json" {
			t.Error("Force 为假时已有 settings.json 不应在返回列表中")
		}
	}
	data, err := os.ReadFile(settingsPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != existing {
		t.Errorf("Force 为假时已有 settings.json 应原样保留，实际：\n%s", data)
	}
	// Force 为真：模板覆盖，返回列表包含该路径。
	files, err = Install(dir, "claude-code", InstallOptions{Force: true})
	if err != nil {
		t.Fatal(err)
	}
	if !containsAll(files, ".claude/settings.json") {
		t.Error("Force 为真时覆盖的 settings.json 应在返回列表中")
	}
	data, _ = os.ReadFile(settingsPath)
	if strings.Contains(string(data), "my-statusline") || strings.Contains(string(data), "MY_VAR") {
		t.Errorf("Force 为真时应以模板覆盖，不保留用户旧键：\n%s", data)
	}
	if !strings.Contains(string(data), "SUNSHINE_HOOK_PLATFORM=claude sunshine hook session-start") {
		t.Errorf("覆盖后应为模板内容：\n%s", data)
	}
}

// TestSettingsMergeWithStatusline：已有 statusLine 时 WithStatusline 不覆盖它
// （Force 为假跳过）；Force 为真时模板 statusLine 覆盖。
func TestSettingsMergeWithStatusline(t *testing.T) {
	dir := t.TempDir()
	settingsPath := filepath.Join(dir, ".claude", "settings.json")
	if err := os.MkdirAll(filepath.Dir(settingsPath), 0o755); err != nil {
		t.Fatal(err)
	}
	existing := "{\n  \"statusLine\": {\n    \"type\": \"command\",\n    \"command\": \"my-statusline\"\n  }\n}\n"
	if err := os.WriteFile(settingsPath, []byte(existing), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Install(dir, "claude-code", InstallOptions{WithStatusline: true}); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(settingsPath)
	if !strings.Contains(string(data), "my-statusline") {
		t.Errorf("Force 为假时已有 statusLine 应保留，实际：\n%s", data)
	}
	if strings.Contains(string(data), "SUNSHINE_HOOK_PLATFORM=claude sunshine hook statusline") {
		t.Errorf("Force 为假时不应注入模板 statusLine：\n%s", data)
	}
	if _, err := Install(dir, "claude-code", InstallOptions{WithStatusline: true, Force: true}); err != nil {
		t.Fatal(err)
	}
	data, _ = os.ReadFile(settingsPath)
	if !strings.Contains(string(data), "SUNSHINE_HOOK_PLATFORM=claude sunshine hook statusline") {
		t.Errorf("Force 为真时应用模板 statusLine：\n%s", data)
	}
}

// TestSettingsMergeInvalidJSON：已有 settings.json 非法时 Force 为假保留、为真覆盖。
func TestSettingsMergeInvalidJSON(t *testing.T) {
	dir := t.TempDir()
	settingsPath := filepath.Join(dir, ".claude", "settings.json")
	if err := os.MkdirAll(filepath.Dir(settingsPath), 0o755); err != nil {
		t.Fatal(err)
	}
	bad := "{ not json"
	if err := os.WriteFile(settingsPath, []byte(bad), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Install(dir, "claude-code", InstallOptions{}); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(settingsPath)
	if string(data) != bad {
		t.Error("非法 JSON 且 Force 为假时应原样保留")
	}
	if _, err := Install(dir, "claude-code", InstallOptions{Force: true}); err != nil {
		t.Fatal(err)
	}
	data, _ = os.ReadFile(settingsPath)
	if !strings.Contains(string(data), "SUNSHINE_HOOK_PLATFORM=claude sunshine hook session-start") {
		t.Error("非法 JSON 且 Force 为真时应用模板覆盖")
	}
}

// TestCodexModelPreservation 覆盖 codex toml 的 model 保留：
// 未注释的 model 行保留；注释提示行不提取；多行字符串内的形似行不提取。
func TestCodexModelPreservation(t *testing.T) {
	dir := t.TempDir()
	agentPath := filepath.Join(dir, ".codex", "agents", "sunshine-implement.toml")
	if err := os.MkdirAll(filepath.Dir(agentPath), 0o755); err != nil {
		t.Fatal(err)
	}
	existing := "name = \"sunshine-implement\"\n" +
		"description = \"旧描述\"\n" +
		"sandbox_mode = \"workspace-write\"\n" +
		"model = \"gpt-5.6-codex\"\n" +
		"model_reasoning_effort = \"high\"\n" +
		"# model = \"should-not-win\"\n" +
		"\n" +
		"developer_instructions = \"\"\"\n" +
		"不要提取这一行：model = \"fake-inside-string\"\n" +
		"\"\"\"\n"
	if err := os.WriteFile(agentPath, []byte(existing), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Install(dir, "codex", InstallOptions{Force: true}); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(agentPath)
	if err != nil {
		t.Fatal(err)
	}
	content := string(data)
	for _, sub := range []string{`model = "gpt-5.6-codex"`, `model_reasoning_effort = "high"`} {
		if !strings.Contains(content, sub) {
			t.Errorf("重装后缺少保留的 %s：\n%s", sub, content)
		}
	}
	if strings.Contains(content, "should-not-win") || strings.Contains(content, "fake-inside-string") {
		t.Errorf("注释行/多行字符串内的 model 行不应被提取：\n%s", content)
	}
	// 保留键插在 sandbox_mode 行之后。
	if strings.Index(content, `model = "gpt-5.6-codex"`) < strings.Index(content, "sandbox_mode") {
		t.Error("保留的 model 键应插在 sandbox_mode 行之后")
	}
	// Force 为假时已有 toml 不动。
	if err := os.WriteFile(agentPath, []byte(existing), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Install(dir, "codex", InstallOptions{}); err != nil {
		t.Fatal(err)
	}
	data, _ = os.ReadFile(agentPath)
	if string(data) != existing {
		t.Error("Force 为假时已有 agent toml 应原样保留")
	}
}

// TestExtractCodexAgentModelKeys 直接覆盖提取函数的边界情况。
func TestExtractCodexAgentModelKeys(t *testing.T) {
	got := extractCodexAgentModelKeys("model = \"a\\\"b\"\nmodel_reasoning_effort = \"low\" # 注释\n")
	if got.model != `a"b` || got.modelReasoningEffort != "low" {
		t.Errorf("提取不对：%+v", got)
	}
	got = extractCodexAgentModelKeys("# model = \"x\"\n")
	if got.model != "" {
		t.Errorf("注释行不应提取：%+v", got)
	}
}
