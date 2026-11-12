package platforms

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// 滚动轨道 16 平台的安装测试：每平台在临时目录实跑 Install，
// 断言生成文件集与各路集成规格 §3 的总数一致，并抽查关键内容。
// 文件数来自各规格的最终生成文件完整清单（A1/B/A3/C 路）。
func TestInstallNewPlatforms(t *testing.T) {
	wantCounts := map[string]int{
		"kiro": 51, "gemini": 50, "qoder": 50,
		"pi": 52, "zcode": 50, "trae": 50, "omp": 50, "snow": 55,
		"opencode": 57, "dsh": 47, "reasonix": 47, "grok": 50, "kimi": 54,
		"kilo": 46, "antigravity": 46, "devin": 46,
	}
	for _, id := range []string{
		"kiro", "gemini", "qoder", "pi", "zcode", "trae", "omp", "snow",
		"opencode", "dsh", "reasonix", "grok", "kimi", "kilo", "antigravity", "devin",
	} {
		t.Run(id, func(t *testing.T) {
			dir := t.TempDir()
			written, err := Install(dir, id, InstallOptions{})
			if err != nil {
				t.Fatalf("Install(%s) 失败：%v", id, err)
			}
			tree := readTree(t, dir)
			if len(tree) != wantCounts[id] {
				t.Errorf("Install(%s) 写出 %d 个文件，期望 %d 个", id, len(tree), wantCounts[id])
				for rel := range tree {
					t.Logf("  文件：%s", rel)
				}
			}
			if len(written) != len(tree) {
				t.Errorf("Install(%s) 返回 %d 个写出路径，磁盘有 %d 个文件", id, len(written), len(tree))
			}
			for _, rel := range written {
				if _, ok := tree[rel]; !ok {
					t.Errorf("Install(%s) 返回的路径 %s 不在磁盘上", id, rel)
				}
			}
			// 生成文件不得残留 trellis 字样与已知占位符
			// （opencode 的 JS 模板含旧版只读兼容读点，见 B 规格 §2.3，
			// 属有意保留，不在此处断言）。
			if id != "opencode" {
				for rel, content := range tree {
					if strings.Contains(strings.ToLower(content), "trellis") {
						t.Errorf("%s 的 %s 含 trellis 残留", id, rel)
					}
					if leftover := findLeftoverPlaceholder(content); leftover != "" {
						t.Errorf("%s 的 %s 残留占位符 %s", id, rel, leftover)
					}
				}
			}
		})
	}
}

func TestInstallNewPlatformsSpotChecks(t *testing.T) {
	read := func(t *testing.T, dir, rel string) string {
		t.Helper()
		data, err := os.ReadFile(filepath.Join(dir, filepath.FromSlash(rel)))
		if err != nil {
			t.Fatalf("读取 %s 失败：%v", rel, err)
		}
		return string(data)
	}
	mustContain := func(t *testing.T, content, sub, what string) {
		t.Helper()
		if !strings.Contains(content, sub) {
			t.Errorf("%s 缺少 %q", what, sub)
		}
	}
	mustNotContain := func(t *testing.T, content, sub, what string) {
		t.Helper()
		if strings.Contains(content, sub) {
			t.Errorf("%s 不应含 %q", what, sub)
		}
	}

	t.Run("kiro", func(t *testing.T) {
		dir := t.TempDir()
		if _, err := Install(dir, "kiro", InstallOptions{}); err != nil {
			t.Fatal(err)
		}
		for _, rel := range []string{
			".kiro/skills/sunshine-continue/SKILL.md",
			".kiro/skills/sunshine-update-spec/SKILL.md",
			".kiro/agents/sunshine.json",
			".kiro/agents/sunshine-implement.json",
			".kiro/hooks/sunshine-workflow-state.kiro.hook",
		} {
			content := read(t, dir, rel)
			mustNotContain(t, content, "trellis", rel)
			_ = content
		}
		var main map[string]any
		if err := json.Unmarshal([]byte(read(t, dir, ".kiro/agents/sunshine.json")), &main); err != nil {
			t.Fatalf("sunshine.json 不是合法 JSON：%v", err)
		}
		if main["name"] != "sunshine" {
			t.Errorf("kiro 主 agent name = %v，期望 sunshine", main["name"])
		}
		var hook map[string]any
		if err := json.Unmarshal([]byte(read(t, dir, ".kiro/hooks/sunshine-workflow-state.kiro.hook")), &hook); err != nil {
			t.Fatalf(".kiro.hook 不是合法 JSON：%v", err)
		}
		if hook["name"] != "sunshine-workflow-state" {
			t.Errorf(".kiro.hook name = %v", hook["name"])
		}
		// 命令引用按 kiro 解析为 $ 前缀。
		mustContain(t, read(t, dir, ".kiro/skills/sunshine-update-spec/SKILL.md"), "$update-spec", "kiro update-spec")
		// 无 start skill、无 settings。
		if _, err := os.Stat(filepath.Join(dir, ".kiro", "skills", "sunshine-start")); !os.IsNotExist(err) {
			t.Error("kiro 不应生成 sunshine-start skill")
		}
		if _, err := os.Stat(filepath.Join(dir, ".kiro", "settings.json")); !os.IsNotExist(err) {
			t.Error("kiro 不应生成 settings.json")
		}
	})

	t.Run("gemini", func(t *testing.T) {
		dir := t.TempDir()
		if _, err := Install(dir, "gemini", InstallOptions{}); err != nil {
			t.Fatal(err)
		}
		toml := read(t, dir, ".gemini/commands/sunshine/continue.toml")
		mustContain(t, toml, `description = "Sunshine: continue"`, "gemini continue.toml")
		mustContain(t, toml, "prompt = \"\"\"", "gemini continue.toml")
		settings := read(t, dir, ".gemini/settings.json")
		var obj map[string]any
		if err := json.Unmarshal([]byte(settings), &obj); err != nil {
			t.Fatalf("gemini settings.json 非法：%v", err)
		}
		// 无 .gemini/skills/ 旧路径。
		if _, err := os.Stat(filepath.Join(dir, ".gemini", "skills")); !os.IsNotExist(err) {
			t.Error("gemini 不应生成 .gemini/skills/")
		}
		// implement/check 含 baked prelude，research 不含。
		mustContain(t, read(t, dir, ".gemini/agents/sunshine-implement.md"), "必须先做：自行加载 Sunshine 上下文", "gemini implement prelude")
		mustNotContain(t, read(t, dir, ".gemini/agents/sunshine-research.md"), "必须先做：自行加载 Sunshine 上下文", "gemini research")
		mustNotContain(t, read(t, dir, ".gemini/agents/sunshine-implement.md"), "sunshine-hook-injected", "gemini implement marker")
	})

	t.Run("qoder-trae-omp-command-descriptions", func(t *testing.T) {
		// COMMAND_DESCRIPTIONS 统一为 A3 §6.2 的值。
		cases := []struct{ id, rel, want string }{
			{"qoder", ".qoder/commands/sunshine-continue.md", `description: "在正确的阶段恢复当前任务。"`},
			{"qoder", ".qoder/commands/sunshine-finish-work.md", `description: "收尾当前会话：质量关卡、提交提醒、归档、日志。"`},
			{"trae", ".trae/commands/sunshine-continue.md", `description: "在正确的阶段恢复当前任务。"`},
			{"trae", ".trae/commands/sunshine-finish-work.md", `description: "收尾当前会话：质量关卡、提交提醒、归档、日志。"`},
			{"omp", ".omp/commands/sunshine-continue.md", `description: "在正确的阶段恢复当前任务。"`},
			{"omp", ".omp/commands/sunshine-finish-work.md", `description: "收尾当前会话：质量关卡、提交提醒、归档、日志。"`},
		}
		for _, c := range cases {
			dir := t.TempDir()
			if _, err := Install(dir, c.id, InstallOptions{}); err != nil {
				t.Fatal(err)
			}
			mustContain(t, read(t, dir, c.rel), c.want, c.id+" "+c.rel)
		}
		// omp：finish-work 加 argument-hint，正文去 H1。
		dir := t.TempDir()
		if _, err := Install(dir, "omp", InstallOptions{}); err != nil {
			t.Fatal(err)
		}
		fw := read(t, dir, ".omp/commands/sunshine-finish-work.md")
		mustContain(t, fw, `argument-hint: "[task-name]"`, "omp finish-work")
		mustNotContain(t, fw, "# 收尾工作", "omp finish-work H1")
		mustNotContain(t, read(t, dir, ".omp/commands/sunshine-continue.md"), "argument-hint", "omp continue")
		// qoder settings：SessionStart 恰为 3 条 matcher。
		qdir := t.TempDir()
		if _, err := Install(qdir, "qoder", InstallOptions{}); err != nil {
			t.Fatal(err)
		}
		var q map[string]any
		if err := json.Unmarshal([]byte(read(t, qdir, ".qoder/settings.json")), &q); err != nil {
			t.Fatalf("qoder settings.json 非法：%v", err)
		}
		hooks := q["hooks"].(map[string]any)
		if len(hooks["SessionStart"].([]any)) != 3 {
			t.Errorf("qoder SessionStart 条目数 = %d，期望 3", len(hooks["SessionStart"].([]any)))
		}
	})

	t.Run("pi", func(t *testing.T) {
		dir := t.TempDir()
		if _, err := Install(dir, "pi", InstallOptions{}); err != nil {
			t.Fatal(err)
		}
		// start 兜底保留：三个 prompts 全有。
		for _, name := range []string{"start", "continue", "finish-work"} {
			if _, err := os.Stat(filepath.Join(dir, ".pi", "prompts", "sunshine-"+name+".md")); err != nil {
				t.Errorf("pi 缺少 .pi/prompts/sunshine-%s.md", name)
			}
		}
		mustContain(t, read(t, dir, ".pi/agents/sunshine-implement.md"), "必须先做：自行加载 Sunshine 上下文", "pi implement prelude")
		if _, err := os.Stat(filepath.Join(dir, ".pi", "extensions", "sunshine", "index.ts")); err != nil {
			t.Error("pi 缺少 extensions/sunshine/index.ts")
		}
	})

	t.Run("zcode", func(t *testing.T) {
		dir := t.TempDir()
		if _, err := Install(dir, "zcode", InstallOptions{}); err != nil {
			t.Fatal(err)
		}
		if _, err := os.Stat(filepath.Join(dir, ".zcode", "commands", "sunshine", "continue.md")); err != nil {
			t.Error("zcode 缺少 .zcode/commands/sunshine/continue.md")
		}
		if _, err := os.Stat(filepath.Join(dir, ".zcode", "commands", "sunshine", "start.md")); !os.IsNotExist(err) {
			t.Error("zcode 不应生成 start 命令")
		}
		// 遗留路径只用于 migrate/uninstall，不写文件。
		if _, err := os.Stat(filepath.Join(dir, ".zcode", "cli")); !os.IsNotExist(err) {
			t.Error("zcode 不应写 .zcode/cli/agents")
		}
		var cfg map[string]any
		if err := json.Unmarshal([]byte(read(t, dir, ".zcode/config.json")), &cfg); err != nil {
			t.Fatalf("zcode config.json 非法：%v", err)
		}
	})

	t.Run("snow", func(t *testing.T) {
		dir := t.TempDir()
		if _, err := Install(dir, "snow", InstallOptions{}); err != nil {
			t.Fatal(err)
		}
		for _, name := range []string{"continue", "finish-work"} {
			var cmd map[string]any
			if err := json.Unmarshal([]byte(read(t, dir, ".snow/commands/sunshine-"+name+".json")), &cmd); err != nil {
				t.Fatalf("snow %s.json 非法：%v", name, err)
			}
			if cmd["type"] != "prompt" || cmd["location"] != "project" {
				t.Errorf("snow %s.json type/location 不对：%v", name, cmd)
			}
			if _, err := os.Stat(filepath.Join(dir, ".snow", "skills", "sunshine-"+name, "SKILL.md")); err != nil {
				t.Errorf("snow 缺少 skill 版 %s", name)
			}
		}
		// snow 命令 JSON 的描述同样用 A3 §6.2 统一值。
		mustContain(t, read(t, dir, ".snow/commands/sunshine-continue.json"), "在正确的阶段恢复当前任务。", "snow continue.json")
		mustContain(t, read(t, dir, ".snow/commands/sunshine-finish-work.json"), "收尾当前会话：质量关卡、提交提醒、归档、日志。", "snow finish-work.json")
		// 超时保持毫秒。
		mustContain(t, read(t, dir, ".snow/hooks/onSessionStart.json"), "20000", "snow onSessionStart timeout")
		if _, err := os.Stat(filepath.Join(dir, ".snow", "SNOW.md")); err != nil {
			t.Error("snow 缺少 SNOW.md")
		}
	})

	t.Run("opencode", func(t *testing.T) {
		dir := t.TempDir()
		if _, err := Install(dir, "opencode", InstallOptions{}); err != nil {
			t.Fatal(err)
		}
		if _, err := os.Stat(filepath.Join(dir, ".opencode", "commands", "sunshine", "start.md")); err != nil {
			t.Error("opencode 缺少 fallback 的 start 命令")
		}
		var pkg map[string]any
		if err := json.Unmarshal([]byte(read(t, dir, ".opencode/package.json")), &pkg); err != nil {
			t.Fatalf("opencode package.json 非法：%v", err)
		}
		for _, rel := range []string{
			".opencode/lib/sunshine-context.js",
			".opencode/plugins/session-start.js",
		} {
			if _, err := os.Stat(filepath.Join(dir, filepath.FromSlash(rel))); err != nil {
				t.Errorf("opencode 缺少 %s", rel)
			}
		}
	})

	t.Run("dsh", func(t *testing.T) {
		dir := t.TempDir()
		if _, err := Install(dir, "dsh", InstallOptions{}); err != nil {
			t.Fatal(err)
		}
		if _, err := os.Stat(filepath.Join(dir, ".dsh", "skills", "sunshine-start", "SKILL.md")); err != nil {
			t.Error("dsh 缺少入口 skill sunshine-start")
		}
		if _, err := os.Stat(filepath.Join(dir, ".dsh", "DSH.md")); err != nil {
			t.Error("dsh 缺少 DSH.md")
		}
		// dsh 无 agent 定义。
		if _, err := os.Stat(filepath.Join(dir, ".dsh", "agents")); !os.IsNotExist(err) {
			t.Error("dsh 不应有 agents 目录")
		}
	})

	t.Run("reasonix", func(t *testing.T) {
		dir := t.TempDir()
		if _, err := Install(dir, "reasonix", InstallOptions{}); err != nil {
			t.Fatal(err)
		}
		check := read(t, dir, ".reasonix/skills/sunshine-check/SKILL.md")
		mustContain(t, check, "runAs: subagent", "reasonix check 子代理版")
		if _, err := os.Stat(filepath.Join(dir, ".reasonix", "skills", "sunshine-research")); !os.IsNotExist(err) {
			t.Error("reasonix 不应有 sunshine-research")
		}
		if _, err := os.Stat(filepath.Join(dir, ".reasonix", "skills", "sunshine-prototype")); !os.IsNotExist(err) {
			t.Error("reasonix 不应有 sunshine-prototype（上游只有 implement+check，见 v0.3.11 实施说明）")
		}
		tree := readTree(t, dir)
		dirs := map[string]bool{}
		for rel := range tree {
			if strings.HasPrefix(rel, ".reasonix/skills/sunshine-") {
				dirs[strings.Split(strings.TrimPrefix(rel, ".reasonix/skills/"), "/")[0]] = true
			}
		}
		if len(dirs) != 13 {
			t.Errorf("reasonix skill 目录数 = %d，期望 13（7 工作流 + 4 bundled + 2 子代理）", len(dirs))
		}
	})

	t.Run("grok-kimi-prelude", func(t *testing.T) {
		for _, id := range []string{"grok", "kimi"} {
			dir := t.TempDir()
			if _, err := Install(dir, id, InstallOptions{}); err != nil {
				t.Fatal(err)
			}
			var agentsDir, skillCopy string
			if id == "grok" {
				agentsDir = ".grok/agents"
			} else {
				agentsDir = ".kimi-code/agents"
				skillCopy = ".kimi-code/skills/sunshine-implement/SKILL.md"
			}
			for _, name := range []string{"implement", "check"} {
				content := read(t, dir, agentsDir+"/sunshine-"+name+".md")
				if strings.Count(content, "必须先加载 Sunshine 上下文") != 1 {
					t.Errorf("%s %s 的 prelude 应恰好出现 1 次", id, name)
				}
				mustContain(t, content, "sunshine task current --source", id+" "+name+" prelude")
			}
			research := read(t, dir, agentsDir+"/sunshine-research.md")
			mustNotContain(t, research, "必须先加载 Sunshine 上下文", id+" research")
			if skillCopy != "" {
				agent := read(t, dir, agentsDir+"/sunshine-implement.md")
				copyContent := read(t, dir, skillCopy)
				if agent != copyContent {
					t.Error("kimi agent 定义与 skill 副本内容应逐字节相同")
				}
			}
		}
	})

	t.Run("shared-agents-skills-byte-identical", func(t *testing.T) {
		// 共享 .agents/skills/ 下 5 个自动触发 skill 用中性解析，
		// codex/gemini/dsh/pi/kimi 写出应逐字节一致。
		dir := t.TempDir()
		for _, id := range []string{"codex", "gemini", "dsh", "pi", "kimi"} {
			if _, err := Install(dir, id, InstallOptions{}); err != nil {
				t.Fatal(err)
			}
		}
		// 以 codex 安装后的内容为基准再逐一重装比对：每次重装
		// Force=false 时内容相同会跳过，文件内容即基准内容。
		base := read(t, dir, ".agents/skills/sunshine-update-spec/SKILL.md")
		mustContain(t, base, "`update-spec` (Sunshine command)", "共享 skill 中性解析")
		for _, id := range []string{"gemini", "dsh", "pi", "kimi"} {
			dir2 := t.TempDir()
			if _, err := Install(dir2, id, InstallOptions{}); err != nil {
				t.Fatal(err)
			}
			if got := read(t, dir2, ".agents/skills/sunshine-update-spec/SKILL.md"); got != base {
				t.Errorf("%s 的共享 update-spec 与 codex 不一致", id)
			}
		}
	})

	t.Run("kilo-antigravity-devin", func(t *testing.T) {
		for _, tc := range []struct {
			id, wfDir, prefix string
		}{
			{"kilo", ".kilocode/workflows", ""},
			{"antigravity", ".agent/workflows", ""},
			{"devin", ".devin/workflows", "sunshine-"},
		} {
			dir := t.TempDir()
			if _, err := Install(dir, tc.id, InstallOptions{}); err != nil {
				t.Fatal(err)
			}
			for _, name := range []string{"start", "continue", "finish-work"} {
				rel := tc.wfDir + "/" + tc.prefix + name + ".md"
				content := read(t, dir, rel)
				mustNotContain(t, content, "---\nname:", tc.id+" workflow 不应有 frontmatter")
			}
			tree := readTree(t, dir)
			for rel := range tree {
				if strings.Contains(rel, "/agents/") {
					t.Errorf("%s 不应生成 agent 文件：%s", tc.id, rel)
				}
			}
		}
		// antigravity 的 CMD_REF 渲染为 /name。
		dir := t.TempDir()
		if _, err := Install(dir, "antigravity", InstallOptions{}); err != nil {
			t.Fatal(err)
		}
		mustContain(t, read(t, dir, ".agent/skills/sunshine-update-spec/SKILL.md"), "/update-spec", "antigravity CMD_REF")
		// devin workflow 文件名带 sunshine- 前缀。
		ddir := t.TempDir()
		if _, err := Install(ddir, "devin", InstallOptions{}); err != nil {
			t.Fatal(err)
		}
		if _, err := os.Stat(filepath.Join(ddir, ".devin", "workflows", "sunshine-start.md")); err != nil {
			t.Error("devin 缺少 .devin/workflows/sunshine-start.md")
		}
	})
}

// bundled-skills.md 第 90 行回归：该句曾用字面 {{CMD_REF:name}} 等
// 枚举 token 名，渲染时 token 名被替换、两处条件块被掏空。
// 改写后句子不含字面可解析 token，claude（两项 true）与
// antigravity（两项 false）渲染后句子都应完整。
func TestBundledSkillsDocLine90NotSelfDestructing(t *testing.T) {
	for _, id := range []string{"claude-code", "antigravity"} {
		dir := t.TempDir()
		if _, err := Install(dir, id, InstallOptions{}); err != nil {
			t.Fatal(err)
		}
		var rel string
		if id == "claude-code" {
			rel = ".claude/skills/sunshine-meta/references/local-architecture/bundled-skills.md"
		} else {
			rel = ".agent/skills/sunshine-meta/references/local-architecture/bundled-skills.md"
		}
		data, err := os.ReadFile(filepath.Join(dir, filepath.FromSlash(rel)))
		if err != nil {
			t.Fatalf("读取 %s 失败：%v", rel, err)
		}
		content := string(data)
		for _, sub := range []string{
			"支持的 token 只有：`CMD_REF:name`、`CLI_FLAG`、`PYTHON_CMD`",
			"条件块 `AGENT_CAPABLE` / `HAS_HOOKS`",
			"不是可解析 token，会原样保留",
		} {
			if !strings.Contains(content, sub) {
				t.Errorf("%s 渲染后第 90 行句子不完整，缺少 %q", id, sub)
			}
		}
		if leftover := findLeftoverPlaceholder(content); leftover != "" {
			t.Errorf("%s 渲染后残留占位符 %s", id, leftover)
		}
	}
}

func TestZcodeSetupHint(t *testing.T) {
	hint := ZcodeSetupHint()
	for _, sub := range []string{"trellis-bridge", "https://github.com/CNHLAIA/ZCode-Trellis-Plugin.git", "ZCode：若项目 Hooks 被禁用"} {
		if !strings.Contains(hint, sub) {
			t.Errorf("ZcodeSetupHint 缺少 %q", sub)
		}
	}
}
