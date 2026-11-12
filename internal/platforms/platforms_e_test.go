package platforms

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// E 路（codebuddy、copilot、droid）安装测试：每平台在临时目录实跑
// Install，断言生成文件集与 E 规格 §3 的总数一致（codebuddy 50、
// copilot 52、droid 50），并抽查关键内容。
// v0.3.11：prototype agent 各 +1（codebuddy/copilot/droid 各一文件）。
func TestInstallEPlatforms(t *testing.T) {
	wantCounts := map[string]int{"codebuddy": 50, "copilot": 52, "droid": 50}
	for _, id := range []string{"codebuddy", "copilot", "droid"} {
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
			for rel, content := range tree {
				if strings.Contains(strings.ToLower(content), "trellis") {
					t.Errorf("%s 的 %s 含 trellis 残留", id, rel)
				}
				if leftover := findLeftoverPlaceholder(content); leftover != "" {
					t.Errorf("%s 的 %s 残留占位符 %s", id, rel, leftover)
				}
			}
		})
	}
}

func TestInstallEPlatformsSpotChecks(t *testing.T) {
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

	t.Run("codebuddy", func(t *testing.T) {
		dir := t.TempDir()
		if _, err := Install(dir, "codebuddy", InstallOptions{}); err != nil {
			t.Fatal(err)
		}
		// 命令：subdir 形态，start 已滤，只剩 continue、finish-work。
		mustContain(t, read(t, dir, ".codebuddy/commands/sunshine/continue.md"), "--platform codebuddy", "codebuddy continue CLI_FLAG")
		mustContain(t, read(t, dir, ".codebuddy/commands/sunshine/finish-work.md"), "/sunshine:finish-work", "codebuddy finish-work 命令引用")
		if _, err := os.Stat(filepath.Join(dir, ".codebuddy", "commands", "sunshine", "start.md")); !os.IsNotExist(err) {
			t.Error("codebuddy 不应生成 start 命令")
		}
		// settings.json：SessionStart 三条 matcher，hook 命令带平台前缀。
		settings := read(t, dir, ".codebuddy/settings.json")
		mustContain(t, settings, "SUNSHINE_HOOK_PLATFORM=codebuddy sunshine hook session-start", "codebuddy settings")
		mustContain(t, settings, "SUNSHINE_HOOK_PLATFORM=codebuddy sunshine hook workflow-state", "codebuddy settings")
		mustNotContain(t, settings, ".py", "codebuddy settings")
		// agents 原样写出，含标记协议。
		mustContain(t, read(t, dir, ".codebuddy/agents/sunshine-implement.md"), "sunshine-hook-injected", "codebuddy implement agent")
	})

	t.Run("droid", func(t *testing.T) {
		dir := t.TempDir()
		if _, err := Install(dir, "droid", InstallOptions{}); err != nil {
			t.Fatal(err)
		}
		// 目录名是 droids/，不是 agents/。
		if _, err := os.Stat(filepath.Join(dir, ".factory", "agents")); !os.IsNotExist(err) {
			t.Error("droid 不应生成 .factory/agents/ 目录")
		}
		mustContain(t, read(t, dir, ".factory/commands/sunshine/continue.md"), "--platform droid", "droid continue CLI_FLAG")
		mustContain(t, read(t, dir, ".factory/commands/sunshine/finish-work.md"), "/sunshine-finish-work", "droid finish-work 命令引用")
		settings := read(t, dir, ".factory/settings.json")
		mustContain(t, settings, "SUNSHINE_HOOK_PLATFORM=droid sunshine hook session-start", "droid settings")
		// implement/check 无递归防护一节（E 规格 §4.2），research 有。
		mustNotContain(t, read(t, dir, ".factory/droids/sunshine-implement.md"), "递归防护", "droid implement")
		mustNotContain(t, read(t, dir, ".factory/droids/sunshine-check.md"), "递归防护", "droid check")
	})

	t.Run("copilot", func(t *testing.T) {
		dir := t.TempDir()
		if _, err := Install(dir, "copilot", InstallOptions{}); err != nil {
			t.Fatal(err)
		}
		// 命令：裸名 .prompt.md，无 frontmatter，start 已滤。
		cont := read(t, dir, ".github/prompts/continue.prompt.md")
		mustContain(t, cont, "--platform copilot", "copilot continue CLI_FLAG")
		mustContain(t, read(t, dir, ".github/prompts/finish-work.prompt.md"), "/finish-work", "copilot finish-work 命令引用")
		mustNotContain(t, cont, "---\nname:", "copilot continue 提示词")
		if _, err := os.Stat(filepath.Join(dir, ".github", "prompts", "start.prompt.md")); !os.IsNotExist(err) {
			t.Error("copilot 不应生成 start 提示词")
		}
		// prompts 携带件不安装：13 个遗留提示词一个都不应出现。
		for _, name := range []string{"start", "before-dev", "brainstorm", "break-loop", "check", "check-cross-layer", "create-command", "integrate-skill", "onboard", "parallel", "record-session", "update-spec"} {
			if _, err := os.Stat(filepath.Join(dir, ".github", "prompts", name+".prompt.md")); !os.IsNotExist(err) {
				t.Errorf("copilot 不应安装携带件 %s.prompt.md", name)
			}
		}
		// hooks.json 写两份，内容逐字节相同；事件键大小写原样保留。
		h1 := read(t, dir, ".github/copilot/hooks.json")
		h2 := read(t, dir, ".github/hooks/sunshine.json")
		if h1 != h2 {
			t.Error("copilot 两份 hooks.json 内容应逐字节相同")
		}
		mustContain(t, h1, `"SessionStart"`, "copilot hooks")
		mustContain(t, h1, `"userPromptSubmitted"`, "copilot hooks")
		mustContain(t, h1, "SUNSHINE_HOOK_PLATFORM=copilot sunshine hook session-start", "copilot hooks")
		mustContain(t, h1, "SUNSHINE_HOOK_PLATFORM=copilot sunshine hook workflow-state", "copilot hooks")
		mustNotContain(t, h1, "subagent-context", "copilot hooks")
		mustNotContain(t, h1, "shell-session", "copilot hooks")
		// agents：.agent.md 后缀，列表式 tools，implement/check 含前置段，research 不含。
		impl := read(t, dir, ".github/agents/sunshine-implement.agent.md")
		mustContain(t, impl, "## 必须先做：自行加载 Sunshine 上下文", "copilot implement 前置段")
		mustContain(t, impl, "- read", "copilot implement tools 列表")
		mustNotContain(t, impl, "tools: Read, Write", "copilot implement tools 列表")
		check := read(t, dir, ".github/agents/sunshine-check.agent.md")
		mustContain(t, check, "## 必须先做：自行加载 Sunshine 上下文", "copilot check 前置段")
		research := read(t, dir, ".github/agents/sunshine-research.agent.md")
		mustNotContain(t, research, "必须先做：自行加载 Sunshine 上下文", "copilot research 不应有前置段")
		// copilot-instructions.md 带 Sunshine 托管块标记。
		instr := read(t, dir, ".github/copilot-instructions.md")
		mustContain(t, instr, "<!-- SUNSHINE:COPILOT-GUIDANCE:START -->", "copilot instructions")
		mustContain(t, instr, "<!-- SUNSHINE:COPILOT-GUIDANCE:END -->", "copilot instructions")
		mustNotContain(t, instr, "TRELLIS:COPILOT-GUIDANCE", "copilot instructions")
	})
}
