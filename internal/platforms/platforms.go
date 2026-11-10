// Package platforms 提供 Sunshine 的平台注册表与三平台适配器
// （Claude Code、Cursor、Codex），以及 init 接入所需的 Install 入口。
//
// 行为对照 Trellis 源码（packages/cli/src/configurators/{shared,claude,
// cursor,codex}.ts、types/ai-tools.ts）按需求清单重新实现，不复制源码。
// 与 Trellis 的关键区别：hook 不再是 Python 脚本，全部改由
// `sunshine hook <事件>` 子命令实现，命令前加
// `SUNSHINE_HOOK_PLATFORM=<平台>` 前缀（C 组接线要求：hook 二进制
// 靠该环境变量识别调用方平台；不加也能工作，但 Codex 的
// `<codex-mode>` 横幅与 bootstrap 提示不会出现，shell 票据的
// platform 字段为空）。因此模板里的 hook 命令字符串一律写作
// `SUNSHINE_HOOK_PLATFORM=<平台> sunshine hook <事件>`，
// 不写 .py 文件、不写 python 命令。
// 占位符替换表与源码核对结论见同目录 NOTES.md。
package platforms

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/linrong1982/Sunshine/templates"
)

// Platform 描述一个受支持的 AI 编程平台。
// 字段取值以 Trellis types/ai-tools.ts 的 AI_TOOLS 登记为准，
// 其中 Name 一律用短名（Codex 在 Trellis 登记里带有一段
// “.agents/skills/” 说明性后缀，见 NOTES.md）。
//
// 布局字段（滚动轨道重构后新增）：Install 的写出形态全部由
// 注册表驱动，buildFiles 只剩一个通用引擎 + Codex 两个特例
// （agent model 保留、.codex/skills/ 空目录保证）。
// 字段含义：
//   - SkillsDir：5 个自动触发 skill + 4 个 bundled skill 的根目录，
//     为空表示该平台不写 skill（如无此形态的平台不填）。
//   - AutoSkillsNeutral：自动触发 skill 是否用中性解析写共享目录。
//   - CmdForm：命令形态（见 CmdForm 常量）；CmdDir 为命令落盘目录，
//     CmdPrefix 为扁平/workflow 形态的文件名前缀，CmdAll 为真时
//     保留 start（否则只写 continue、finish-work），CmdNeutral
//     为真时命令转 skill 用中性解析（仅 codex）。
//   - CmdJSONDir：非空时另写一份命令 JSON（仅 snow）。
//   - AgentFiles：子代理定义（原样写出；Prelude 为真时对
//     implement/check 注入 pull-based prelude）。
//   - AgentSkillCopies：与 AgentFiles 同源的 skill 副本（仅 kimi）。
//   - ExtraFiles：平台专属配置/扩展等原样写出的文件。
//   - ExtraManagedPaths：configDir 之外的托管路径
//     （供 lifecycle/update 划定清理范围）。
type Platform struct {
	ID                  string // "claude-code" | "cursor" | "codex" | …
	Name                string // "Claude Code" | "Cursor" | "Codex" | …
	Flag                string // "claude" | "cursor" | "codex" | …
	ConfigDir           string // 平台配置目录：".claude" | ".cursor" | ".codex" | …
	DefaultChecked      bool   // claude/cursor 为 true，其余为 false
	CmdRefPrefix        string // "/sunshine:" | "/sunshine-" | "$" | …
	AgentCapable        bool
	HasHooks            bool
	SupportsAgentSkills bool
	ExtraManagedPaths   []string

	SkillsDir         string
	AutoSkillsNeutral bool
	CmdForm           string
	CmdDir            string
	CmdPrefix         string
	CmdAll            bool
	CmdNeutral        bool
	CmdJSONDir        string
	AgentFiles        []PlatformFile
	AgentSkillCopies  []PlatformFile
	ExtraFiles        []PlatformFile
	ExcludeCheckSkill bool // 为真时不写 common 的 check 自动触发 skill（仅 reasonix：check 被子代理版顶替）

	// 以下两项是 Trellis templateContext 的其余字段，模板内目前
	// 没有占位符引用它们，但占位符解析器按 shared.ts 实现它们，
	// 以便后续模板直接使用。
	executorAI      string
	userActionLabel string
}

// PlatformFile 是一条“模板源路径 → 项目内目标路径”的原样写出登记。
// Src 为 templates.Platforms 内的相对路径，Dst 为项目根相对路径。
// Prelude 为真时只对 implement/check agent 在 frontmatter 之后
// 注入 pull-based prelude（grok、kimi；见 injectPullBasedPrelude）。
type PlatformFile struct {
	Src     string
	Dst     string
	Prelude bool
}

// 命令形态（Platform.CmdForm 的取值）。
const (
	// CmdFormSubdir：子目录 + 裸名，如 .claude/commands/sunshine/<名>.md。
	CmdFormSubdir = "subdir"
	// CmdFormFlat：扁平 sunshine-<名>.md，如 .cursor/commands/。
	CmdFormFlat = "flat"
	// CmdFormTOML：TOML 包装，如 .gemini/commands/sunshine/<名>.toml。
	CmdFormTOML = "toml"
	// CmdFormFrontmatter：YAML frontmatter 包装的扁平命令（qoder、trae）。
	CmdFormFrontmatter = "frontmatter"
	// CmdFormOMP：OMP frontmatter（只有 description，finish-work 加
	// argument-hint，正文去 H1 行）。
	CmdFormOMP = "omp"
	// CmdFormSkill：命令转 skill 写 SKILL.md（codex、kiro、dsh、kimi、
	// reasonix、snow）。
	CmdFormSkill = "skill"
	// CmdFormWorkflow：workflow 文件，无 frontmatter（kilo、antigravity、
	// devin）。
	CmdFormWorkflow = "workflow"
	// CmdFormPrompt：Copilot 提示词文件，无 frontmatter、裸名
	// （.github/prompts/<名>.prompt.md，不带 sunshine- 前缀）。
	CmdFormPrompt = "prompt"
)

// mdAgents 返回某平台三个 md agent 定义的登记
// （src：templates/platforms/<tmplDir>/agents/；dst：<agentsDir>/）。
func mdAgents(tmplDir, agentsDir string) []PlatformFile {
	var out []PlatformFile
	for _, name := range agentNames {
		out = append(out, PlatformFile{
			Src: "platforms/" + tmplDir + "/agents/sunshine-" + name + ".md",
			Dst: agentsDir + "/sunshine-" + name + ".md",
		})
	}
	return out
}

// pullAgents 返回 grok/kimi 的 agent 登记：implement/check 带 prelude。
func pullAgents(tmplDir, agentsDir string) []PlatformFile {
	files := mdAgents(tmplDir, agentsDir)
	for i, f := range files {
		if f.Dst == agentsDir+"/sunshine-implement.md" || f.Dst == agentsDir+"/sunshine-check.md" {
			files[i].Prelude = true
		}
	}
	return files
}

// agentSkillCopies 返回 kimi 的 agent skill 副本登记：同一份 agent
// 内容写到 <skillsDir>/sunshine-<名>/SKILL.md，与 agents/ 下的
// 定义逐字节相同（含 prelude）。
func agentSkillCopies(tmplDir, skillsDir string) []PlatformFile {
	var out []PlatformFile
	for _, name := range agentNames {
		out = append(out, PlatformFile{
			Src:     "platforms/" + tmplDir + "/agents/sunshine-" + name + ".md",
			Dst:     skillsDir + "/sunshine-" + name + "/SKILL.md",
			Prelude: name == "implement" || name == "check",
		})
	}
	return out
}

// registry 是平台注册表，唯一事实源；后续平台按同一结构扩展。
// 顺序：claude-code、cursor、codex（阶段 2），随后是滚动轨道
// 批次的 19 个平台（含 E 路的 codebuddy、copilot、droid）。
var registry = []Platform{
	{
		ID: "claude-code", Name: "Claude Code", Flag: "claude", ConfigDir: ".claude",
		DefaultChecked: true, CmdRefPrefix: "/sunshine:",
		AgentCapable: true, HasHooks: true, SupportsAgentSkills: false,
		SkillsDir: ".claude/skills",
		CmdForm:   CmdFormSubdir, CmdDir: ".claude/commands/sunshine",
		AgentFiles: mdAgents("claude", ".claude/agents"),
		ExtraFiles: []PlatformFile{{Src: "platforms/claude/settings.json", Dst: ".claude/settings.json"}},
		executorAI: "Bash scripts or Task calls", userActionLabel: "Slash commands",
	},
	{
		ID: "cursor", Name: "Cursor", Flag: "cursor", ConfigDir: ".cursor",
		DefaultChecked: true, CmdRefPrefix: "/sunshine-",
		AgentCapable: true, HasHooks: true, SupportsAgentSkills: false,
		SkillsDir: ".cursor/skills",
		CmdForm:   CmdFormFlat, CmdDir: ".cursor/commands",
		AgentFiles: mdAgents("cursor", ".cursor/agents"),
		ExtraFiles: []PlatformFile{{Src: "platforms/cursor/hooks.json", Dst: ".cursor/hooks.json"}},
		executorAI: "Bash scripts or Task calls", userActionLabel: "Slash commands",
	},
	{
		ID: "codex", Name: "Codex", Flag: "codex", ConfigDir: ".codex",
		DefaultChecked: false, CmdRefPrefix: "$",
		AgentCapable: true, HasHooks: false, SupportsAgentSkills: true,
		SkillsDir: ".agents/skills", AutoSkillsNeutral: true,
		CmdForm: CmdFormSkill, CmdDir: ".agents/skills", CmdAll: true, CmdNeutral: true,
		AgentFiles: []PlatformFile{
			{Src: "platforms/codex/agents/sunshine-implement.toml", Dst: ".codex/agents/sunshine-implement.toml"},
			{Src: "platforms/codex/agents/sunshine-check.toml", Dst: ".codex/agents/sunshine-check.toml"},
			{Src: "platforms/codex/agents/sunshine-research.toml", Dst: ".codex/agents/sunshine-research.toml"},
		},
		ExtraFiles: []PlatformFile{
			{Src: "platforms/codex/hooks.json", Dst: ".codex/hooks.json"},
			{Src: "platforms/codex/config.toml", Dst: ".codex/config.toml"},
		},
		executorAI: "Bash scripts or tool calls", userActionLabel: "Skills",
	},
	{
		ID: "kiro", Name: "Kiro Code", Flag: "kiro", ConfigDir: ".kiro/skills",
		DefaultChecked: false, CmdRefPrefix: "$",
		AgentCapable: true, HasHooks: true, SupportsAgentSkills: false,
		ExtraManagedPaths: []string{".kiro/agents", ".kiro/hooks"},
		SkillsDir:         ".kiro/skills",
		CmdForm:           CmdFormSkill, CmdDir: ".kiro/skills",
		AgentFiles: []PlatformFile{
			{Src: "platforms/kiro/agents/sunshine.json", Dst: ".kiro/agents/sunshine.json"},
			{Src: "platforms/kiro/agents/sunshine-implement.json", Dst: ".kiro/agents/sunshine-implement.json"},
			{Src: "platforms/kiro/agents/sunshine-check.json", Dst: ".kiro/agents/sunshine-check.json"},
			{Src: "platforms/kiro/agents/sunshine-research.json", Dst: ".kiro/agents/sunshine-research.json"},
		},
		ExtraFiles: []PlatformFile{
			{Src: "platforms/kiro/hooks/sunshine-workflow-state.kiro.hook", Dst: ".kiro/hooks/sunshine-workflow-state.kiro.hook"},
		},
		executorAI: "Bash scripts or tool calls", userActionLabel: "Skills",
	},
	{
		ID: "gemini", Name: "Gemini CLI", Flag: "gemini", ConfigDir: ".gemini",
		DefaultChecked: false, CmdRefPrefix: "/sunshine:",
		AgentCapable: true, HasHooks: true, SupportsAgentSkills: true,
		SkillsDir: ".agents/skills", AutoSkillsNeutral: true,
		CmdForm: CmdFormTOML, CmdDir: ".gemini/commands/sunshine",
		AgentFiles: mdAgents("gemini", ".gemini/agents"),
		ExtraFiles: []PlatformFile{{Src: "platforms/gemini/settings.json", Dst: ".gemini/settings.json"}},
		executorAI: "Bash scripts or tool calls", userActionLabel: "Slash commands",
	},
	{
		ID: "qoder", Name: "Qoder", Flag: "qoder", ConfigDir: ".qoder",
		DefaultChecked: false, CmdRefPrefix: "$",
		AgentCapable: true, HasHooks: true, SupportsAgentSkills: false,
		SkillsDir: ".qoder/skills",
		CmdForm:   CmdFormFrontmatter, CmdDir: ".qoder/commands",
		AgentFiles: mdAgents("qoder", ".qoder/agents"),
		ExtraFiles: []PlatformFile{{Src: "platforms/qoder/settings.json", Dst: ".qoder/settings.json"}},
		executorAI: "Bash scripts or tool calls", userActionLabel: "Skills",
	},
	{
		ID: "pi", Name: "Pi Agent", Flag: "pi", ConfigDir: ".pi",
		DefaultChecked: false, CmdRefPrefix: "/sunshine-",
		AgentCapable: true, HasHooks: true, SupportsAgentSkills: true,
		SkillsDir: ".agents/skills", AutoSkillsNeutral: true,
		CmdForm: CmdFormFlat, CmdDir: ".pi/prompts", CmdAll: true,
		AgentFiles: mdAgents("pi", ".pi/agents"),
		ExtraFiles: []PlatformFile{
			{Src: "platforms/pi/settings.json", Dst: ".pi/settings.json"},
			{Src: "platforms/pi/extensions/sunshine/index.ts", Dst: ".pi/extensions/sunshine/index.ts"},
		},
		executorAI: "Bash scripts or tool calls", userActionLabel: "Slash commands",
	},
	{
		ID: "zcode", Name: "ZCode", Flag: "zcode", ConfigDir: ".zcode",
		DefaultChecked: false, CmdRefPrefix: "/sunshine:",
		AgentCapable: true, HasHooks: true, SupportsAgentSkills: false,
		ExtraManagedPaths: []string{".zcode/cli/agents", ".zcode/agents", ".zcode/commands", ".zcode/skills", ".zcode/hooks"},
		SkillsDir:         ".zcode/skills",
		CmdForm:           CmdFormSubdir, CmdDir: ".zcode/commands/sunshine",
		AgentFiles: mdAgents("zcode", ".zcode/agents"),
		ExtraFiles: []PlatformFile{{Src: "platforms/zcode/config.json", Dst: ".zcode/config.json"}},
		executorAI: "Bash scripts or Agent calls", userActionLabel: "Skills",
	},
	{
		ID: "trae", Name: "Trae", Flag: "trae", ConfigDir: ".trae",
		DefaultChecked: false, CmdRefPrefix: "/sunshine-",
		AgentCapable: true, HasHooks: true, SupportsAgentSkills: false,
		SkillsDir: ".trae/skills",
		CmdForm:   CmdFormFrontmatter, CmdDir: ".trae/commands",
		AgentFiles: mdAgents("trae", ".trae/agents"),
		ExtraFiles: []PlatformFile{{Src: "platforms/trae/hooks.json", Dst: ".trae/hooks.json"}},
		executorAI: "Bash scripts or tool calls", userActionLabel: "Commands",
	},
	{
		ID: "omp", Name: "Oh My Pi", Flag: "omp", ConfigDir: ".omp",
		DefaultChecked: false, CmdRefPrefix: "/sunshine:",
		AgentCapable: true, HasHooks: true, SupportsAgentSkills: false,
		SkillsDir: ".omp/skills",
		CmdForm:   CmdFormOMP, CmdDir: ".omp/commands",
		AgentFiles: mdAgents("omp", ".omp/agents"),
		ExtraFiles: []PlatformFile{
			{Src: "platforms/omp/extensions/sunshine/index.ts", Dst: ".omp/extensions/sunshine/index.ts"},
		},
		executorAI: "Bash scripts or Task calls", userActionLabel: "Slash commands",
	},
	{
		ID: "snow", Name: "Snow CLI", Flag: "snow", ConfigDir: ".snow/skills",
		DefaultChecked: false, CmdRefPrefix: "/sunshine-",
		AgentCapable: true, HasHooks: true, SupportsAgentSkills: false,
		ExtraManagedPaths: []string{".snow/commands", ".snow/agents", ".snow/hooks", ".snow/SNOW.md"},
		SkillsDir:         ".snow/skills",
		CmdForm:           CmdFormSkill, CmdDir: ".snow/skills", CmdJSONDir: ".snow/commands",
		AgentFiles: mdAgents("snow", ".snow/agents"),
		ExtraFiles: []PlatformFile{
			{Src: "platforms/snow/hooks/beforeSubAgentStart.json", Dst: ".snow/hooks/beforeSubAgentStart.json"},
			{Src: "platforms/snow/hooks/onSessionStart.json", Dst: ".snow/hooks/onSessionStart.json"},
			{Src: "platforms/snow/hooks/onUserMessage.json", Dst: ".snow/hooks/onUserMessage.json"},
			{Src: "platforms/snow/SNOW.md", Dst: ".snow/SNOW.md"},
		},
		executorAI: "Bash scripts or Agent calls", userActionLabel: "Skills",
	},
	{
		ID: "opencode", Name: "OpenCode", Flag: "opencode", ConfigDir: ".opencode",
		DefaultChecked: false, CmdRefPrefix: "/sunshine:",
		AgentCapable: true, HasHooks: false, SupportsAgentSkills: false,
		SkillsDir: ".opencode/skills",
		CmdForm:   CmdFormSubdir, CmdDir: ".opencode/commands/sunshine", CmdAll: true,
		AgentFiles: mdAgents("opencode", ".opencode/agents"),
		ExtraFiles: []PlatformFile{
			{Src: "platforms/opencode/lib/context-visibility.js", Dst: ".opencode/lib/context-visibility.js"},
			{Src: "platforms/opencode/lib/session-utils.js", Dst: ".opencode/lib/session-utils.js"},
			{Src: "platforms/opencode/lib/sunshine-context.js", Dst: ".opencode/lib/sunshine-context.js"},
			{Src: "platforms/opencode/plugins/session-start.js", Dst: ".opencode/plugins/session-start.js"},
			{Src: "platforms/opencode/plugins/inject-workflow-state.js", Dst: ".opencode/plugins/inject-workflow-state.js"},
			{Src: "platforms/opencode/plugins/inject-subagent-context.js", Dst: ".opencode/plugins/inject-subagent-context.js"},
			{Src: "platforms/opencode/package.json", Dst: ".opencode/package.json"},
		},
		executorAI: "Bash scripts or Task calls", userActionLabel: "Slash commands",
	},
	{
		ID: "dsh", Name: "DeepSeek Harness", Flag: "dsh", ConfigDir: ".dsh",
		DefaultChecked: false, CmdRefPrefix: "sunshine-",
		AgentCapable: true, HasHooks: false, SupportsAgentSkills: true,
		SkillsDir: ".agents/skills", AutoSkillsNeutral: true,
		CmdForm: CmdFormSkill, CmdDir: ".dsh/skills", CmdAll: true,
		ExtraFiles: []PlatformFile{{Src: "platforms/dsh/DSH.md", Dst: ".dsh/DSH.md"}},
		executorAI: "Bash scripts or tool calls", userActionLabel: "Skills",
	},
	{
		ID: "reasonix", Name: "Reasonix", Flag: "reasonix", ConfigDir: ".reasonix",
		DefaultChecked: false, CmdRefPrefix: "/skill sunshine-",
		AgentCapable: true, HasHooks: false, SupportsAgentSkills: false,
		SkillsDir: ".reasonix/skills", ExcludeCheckSkill: true,
		CmdForm: CmdFormSkill, CmdDir: ".reasonix/skills", CmdAll: true,
		AgentFiles: []PlatformFile{
			{Src: "platforms/reasonix/agents/sunshine-implement.md", Dst: ".reasonix/skills/sunshine-implement/SKILL.md"},
			{Src: "platforms/reasonix/agents/sunshine-check.md", Dst: ".reasonix/skills/sunshine-check/SKILL.md"},
		},
		executorAI: "Bash scripts or tool calls", userActionLabel: "Skills",
	},
	{
		ID: "grok", Name: "Grok Build", Flag: "grok", ConfigDir: ".grok",
		DefaultChecked: false, CmdRefPrefix: "/sunshine-",
		AgentCapable: true, HasHooks: false, SupportsAgentSkills: false,
		ExtraManagedPaths: []string{".grok/skills", ".grok/commands", ".grok/agents"},
		SkillsDir:         ".grok/skills",
		CmdForm:           CmdFormFlat, CmdDir: ".grok/commands", CmdAll: true,
		AgentFiles: pullAgents("grok", ".grok/agents"),
		executorAI: "Bash scripts or Agent calls", userActionLabel: "Skills",
	},
	{
		ID: "kimi", Name: "Kimi Code", Flag: "kimi", ConfigDir: ".kimi-code",
		DefaultChecked: false, CmdRefPrefix: "/skill:sunshine-",
		AgentCapable: true, HasHooks: false, SupportsAgentSkills: true,
		SkillsDir: ".agents/skills", AutoSkillsNeutral: true,
		CmdForm: CmdFormSkill, CmdDir: ".kimi-code/skills", CmdAll: true,
		AgentFiles:       pullAgents("kimi", ".kimi-code/agents"),
		AgentSkillCopies: agentSkillCopies("kimi", ".kimi-code/skills"),
		executorAI:       "Bash scripts or Agent calls", userActionLabel: "Slash commands",
	},
	{
		ID: "kilo", Name: "Kilo CLI", Flag: "kilo", ConfigDir: ".kilocode",
		DefaultChecked: false, CmdRefPrefix: "/sunshine:",
		AgentCapable: false, HasHooks: false, SupportsAgentSkills: false,
		SkillsDir: ".kilocode/skills",
		CmdForm:   CmdFormWorkflow, CmdDir: ".kilocode/workflows", CmdAll: true,
		executorAI: "Bash scripts or file reads", userActionLabel: "Workflows",
	},
	{
		ID: "antigravity", Name: "Antigravity", Flag: "antigravity", ConfigDir: ".agent/workflows",
		DefaultChecked: false, CmdRefPrefix: "/",
		AgentCapable: false, HasHooks: false, SupportsAgentSkills: false,
		ExtraManagedPaths: []string{".agent/skills"},
		SkillsDir:         ".agent/skills",
		CmdForm:           CmdFormWorkflow, CmdDir: ".agent/workflows", CmdAll: true,
		executorAI: "Bash scripts or file reads", userActionLabel: "Workflows",
	},
	{
		ID: "devin", Name: "Devin", Flag: "devin", ConfigDir: ".devin/workflows",
		DefaultChecked: false, CmdRefPrefix: "/sunshine-",
		AgentCapable: false, HasHooks: false, SupportsAgentSkills: false,
		ExtraManagedPaths: []string{".devin/skills"},
		SkillsDir:         ".devin/skills",
		CmdForm:           CmdFormWorkflow, CmdDir: ".devin/workflows", CmdPrefix: "sunshine-", CmdAll: true,
		executorAI: "Bash scripts or file reads", userActionLabel: "Workflows",
	},
	{
		ID: "codebuddy", Name: "CodeBuddy", Flag: "codebuddy", ConfigDir: ".codebuddy",
		DefaultChecked: false, CmdRefPrefix: "/sunshine:",
		AgentCapable: true, HasHooks: true, SupportsAgentSkills: false,
		SkillsDir: ".codebuddy/skills",
		CmdForm:   CmdFormSubdir, CmdDir: ".codebuddy/commands/sunshine",
		AgentFiles: mdAgents("codebuddy", ".codebuddy/agents"),
		ExtraFiles: []PlatformFile{{Src: "platforms/codebuddy/settings.json", Dst: ".codebuddy/settings.json"}},
		executorAI: "Bash scripts or Task calls", userActionLabel: "Slash commands",
	},
	{
		ID: "copilot", Name: "GitHub Copilot", Flag: "copilot", ConfigDir: ".github/copilot",
		DefaultChecked: false, CmdRefPrefix: "/",
		AgentCapable: true, HasHooks: true, SupportsAgentSkills: false,
		ExtraManagedPaths: []string{".github/agents", ".github/copilot-instructions.md", ".github/hooks", ".github/prompts", ".github/skills"},
		SkillsDir:         ".github/skills",
		CmdForm:           CmdFormPrompt, CmdDir: ".github/prompts",
		AgentFiles: []PlatformFile{
			{Src: "platforms/copilot/agents/sunshine-implement.agent.md", Dst: ".github/agents/sunshine-implement.agent.md"},
			{Src: "platforms/copilot/agents/sunshine-check.agent.md", Dst: ".github/agents/sunshine-check.agent.md"},
			{Src: "platforms/copilot/agents/sunshine-research.agent.md", Dst: ".github/agents/sunshine-research.agent.md"},
		},
		ExtraFiles: []PlatformFile{
			{Src: "platforms/copilot/copilot-instructions.md", Dst: ".github/copilot-instructions.md"},
			{Src: "platforms/copilot/hooks.json", Dst: ".github/copilot/hooks.json"},
			{Src: "platforms/copilot/hooks.json", Dst: ".github/hooks/sunshine.json"},
		},
		executorAI: "Bash scripts or tool calls", userActionLabel: "Prompts",
	},
	{
		ID: "droid", Name: "Factory Droid", Flag: "droid", ConfigDir: ".factory",
		DefaultChecked: false, CmdRefPrefix: "/sunshine-",
		AgentCapable: true, HasHooks: true, SupportsAgentSkills: false,
		SkillsDir: ".factory/skills",
		CmdForm:   CmdFormSubdir, CmdDir: ".factory/commands/sunshine",
		AgentFiles: []PlatformFile{
			{Src: "platforms/droid/droids/sunshine-implement.md", Dst: ".factory/droids/sunshine-implement.md"},
			{Src: "platforms/droid/droids/sunshine-check.md", Dst: ".factory/droids/sunshine-check.md"},
			{Src: "platforms/droid/droids/sunshine-research.md", Dst: ".factory/droids/sunshine-research.md"},
		},
		ExtraFiles: []PlatformFile{{Src: "platforms/droid/settings.json", Dst: ".factory/settings.json"}},
		executorAI: "Bash scripts or Task calls", userActionLabel: "Slash commands",
	},
}

// Registry 返回全部已注册平台（按注册表顺序：claude-code、cursor、codex，
// 随后是滚动轨道的 19 个平台）。
func Registry() []Platform {
	out := make([]Platform, len(registry))
	copy(out, registry)
	return out
}

// ByID 按平台 ID 查找平台。
func ByID(id string) (Platform, bool) {
	for _, p := range registry {
		if p.ID == id {
			return p, true
		}
	}
	return Platform{}, false
}

// DefaultSelected 返回默认勾选的平台 ID（Claude Code 与 Cursor）。
func DefaultSelected() []string {
	var ids []string
	for _, p := range registry {
		if p.DefaultChecked {
			ids = append(ids, p.ID)
		}
	}
	return ids
}

// CodexHooksHint 返回 init 完成后打印的 Codex hooks 启用提示（中文）。
// Codex 的 hooks.json 只有在用户级配置开启 hooks 后才生效，
// 且 Codex 0.129+ 会对每个 hook 做一次 /hooks 审批。
func CodexHooksHint() string {
	return "提示：Codex hooks 须在 ~/.codex/config.toml 中设置 features.hooks = true" +
		"（Codex 0.129+；旧版为 codex_hooks = true），并在 Codex 里运行一次 /hooks 审批，" +
		"否则 .codex/hooks.json 不生效。"
}

// InstallOptions 控制 Install 的安装行为。
type InstallOptions struct {
	WithStatusline bool // 仅 claude-code 有效：为 settings.json 注入 statusLine 段
	Force          bool // 为真时覆盖已有模板文件；为假时已存在的普通模板文件跳过
}

// skill 模板的源文件名（不含 sunshine- 前缀）。
var autoSkillNames = []string{"before-dev", "brainstorm", "break-loop", "check", "update-spec"}

// command 模板的源文件名。
var commandNames = []string{"continue", "finish-work", "start"}

// agent 定义名（不含 sunshine- 前缀）。
var agentNames = []string{"implement", "check", "research"}

// 内置多文件 skill 的目录名（不含 sunshine- 前缀）。
// 实际文件由 B 组写入 templates/platforms/common/bundled-skills/；
// 该目录不存在时 Install 跳过这一步（见 BundledSkillsPresent）。
var bundledSkillNames = []string{"channel", "meta", "spec-bootstrap", "session-insight"}

// skillsRoot 返回平台的 skill 根目录（相对项目根，正斜杠），
// 取自注册表的 SkillsDir 布局字段。
func skillsRoot(p Platform) string {
	return p.SkillsDir
}

// ZcodeSetupHint 返回 init 完成后打印的 ZCode trellis-bridge 提示
// （A3 规格 §5.2 的双语文案）：只提示用户手动安装，不代装。
func ZcodeSetupHint() string {
	return "ℹ️ ZCode: if project Hooks are disabled, install trellis-bridge, then start a new session.\n" +
		"ZCode：若项目 Hooks 被禁用，请安装 trellis-bridge，然后新建会话。\n" +
		"请手动在 ZCode 插件市场中添加 https://github.com/CNHLAIA/ZCode-Trellis-Plugin.git，并手动安装 ZCode 补丁插件 trellis-bridge"
}

// commandDescriptions 是命令的中文描述（A3 规格 §6.2 的统一值，
// qoder/trae/omp/snow 四家与 COMMAND_DESCRIPTIONS 译文共用；
// A1 的 qoder 译文与 A3 §6.1 的 snow 译文与之不同时以此处为准）。
var commandDescriptions = map[string]string{
	"start":       "初始化 Sunshine 开发会话。",
	"continue":    "在正确的阶段恢复当前任务。",
	"finish-work": "收尾当前会话：质量关卡、提交提醒、归档、日志。",
}

// wrapWithCommandFrontmatter 给命令包 YAML frontmatter（qoder、trae；
// shared.ts wrapWithCommandFrontmatter）：name + JSON 双引号 description。
func wrapWithCommandFrontmatter(name, content string) (string, error) {
	desc, ok := commandDescriptions[strings.TrimPrefix(name, "sunshine-")]
	if !ok {
		return "", fmt.Errorf("命令 %q 没有登记描述，请在 commandDescriptions 中补充", name)
	}
	quoted, err := json.Marshal(desc)
	if err != nil {
		return "", err
	}
	return "---\nname: " + name + "\ndescription: " + string(quoted) + "\n---\n\n" + content, nil
}

var reOmpH1 = regexp.MustCompile(`^# [^\n]+\n\n`)

// wrapWithOmpFrontmatter 给命令包 OMP frontmatter（shared.ts
// wrapWithOmpFrontmatter）：只有 description；finish-work 加
// argument-hint "[task-name]"；去掉正文开头的 H1 行（由 frontmatter 替代）。
func wrapWithOmpFrontmatter(name, content string) (string, error) {
	desc, ok := commandDescriptions[strings.TrimPrefix(name, "sunshine-")]
	if !ok {
		return "", fmt.Errorf("命令 %q 没有登记描述，请在 commandDescriptions 中补充", name)
	}
	quoted, err := json.Marshal(desc)
	if err != nil {
		return "", err
	}
	out := "---\ndescription: " + string(quoted) + "\n"
	if strings.TrimPrefix(name, "sunshine-") == "finish-work" {
		out += "argument-hint: \"[task-name]\"\n"
	}
	return out + "---\n\n" + reOmpH1.ReplaceAllString(content, ""), nil
}

// buildSnowCommandJSON 生成 snow 的命令 JSON（snow.ts
// buildSnowCommandJson）：type prompt、location project，
// 2 空格缩进 + 末尾换行；不用 HTML 转义（与 JSON.stringify 一致）。
func buildSnowCommandJSON(name, content string) (string, error) {
	desc, ok := commandDescriptions[name]
	if !ok {
		return "", fmt.Errorf("命令 %q 没有登记描述，请在 commandDescriptions 中补充", name)
	}
	obj := struct {
		Type        string `json:"type"`
		Description string `json:"description"`
		Command     string `json:"command"`
		Location    string `json:"location"`
	}{Type: "prompt", Description: desc, Command: content, Location: "project"}
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	if err := enc.Encode(obj); err != nil {
		return "", err
	}
	return buf.String(), nil
}

// wrapGeminiCommandTOML 生成 gemini 的命令 TOML（gemini.ts）：
// description = "Sunshine: <name>"，prompt 为三引号包裹的命令全文。
func wrapGeminiCommandTOML(name, content string) string {
	return "description = \"Sunshine: " + name + "\"\n\nprompt = \"\"\"\n" + content + "\n\"\"\"\n"
}

// pullPrelude 返回 pull-based prelude 的中文全文（B 规格 §8；
// shared.ts buildPullBasedPrelude）。{jsonl} 处按 agent 取
// implement.jsonl 或 check.jsonl；末尾保留 "---" 分隔行。
// 只由 buildFiles 在 grok/kimi 的 implement/check 上注入；
// gemini/qoder/pi/trae 的 prelude 已 baked 进模板文件，不走这里。
func pullPrelude(jsonl string) string {
	return "## 必须先加载 Sunshine 上下文\n\n" +
		"本平台不会通过 hook 自动注入任务上下文。做其他任何事之前，你必须先自行加载上下文。\n\n" +
		"### 第 1 步：找到当前任务路径\n\n" +
		"按顺序尝试——找到任务路径就停下：\n\n" +
		"1. **看派发提示**：如果你收到的派发提示第一行是 `Active task: <path>`（例如 `Active task: .sunshine/tasks/04-17-foo`），就用这个路径。主会话必须在 class-2 平台上写上这一行。\n" +
		"2. **运行** `sunshine task current --source`，读 `Current task:` 这一行。\n" +
		"3. **两处都失败**（派发提示里没有 `Active task:` 行，`sunshine task current` 也没有返回任务），问用户要做哪个任务；不要猜。\n\n" +
		"### 第 2 步：从解析出的路径加载任务上下文\n\n" +
		"1. 读 `<task-path>/" + jsonl + "`——与本 agent 相关的规范/调研文件 JSONL 清单。\n" +
		"2. 对 JSONL 里的每一条，Read 它的 `file` 路径——这些是你必须遵循的规范与调研笔记。**跳过没有 `\"file\"` 字段的行**（例如旧版 `sunshine task create` 留下的 `{\"_example\": \"...\"}` 占位行）。\n" +
		"3. 读任务的 `prd.md`（需求），然后读 `design.md`（如果有，技术设计），再读 `implement.md`（如果有，执行计划）。\n\n" +
		"如果 `" + jsonl + "` 里没有整理好的条目（为空、只有占位行，或文件缺失），兜底做法：读任务产物，用 `sunshine task context --mode packages` 列出可用规范，自己挑与任务领域匹配的规范。不要因为 jsonl 缺失就卡住——轻量任务可能只有 PRD，复杂任务还可能有 `design.md` 与 `implement.md`。\n\n" +
		"如果解析出的任务路径下没有 `prd.md`，问用户要做什么；没有上下文不要继续。\n\n" +
		"---"
}

// injectPullBasedPrelude 在 agent 内容的 YAML frontmatter 之后、
// 正文之前插入 prelude（shared.ts injectPullBasedPreludeMarkdown；
// 无 frontmatter 时直接放文首）。prelude 与正文之间空一行，
// 与 pi/trae 模板里 baked 的前置段排版一致。
func injectPullBasedPrelude(content, jsonl string) string {
	prelude := pullPrelude(jsonl)
	if strings.HasPrefix(content, "---\n") {
		if idx := strings.Index(content[4:], "\n---\n"); idx >= 0 {
			fmEnd := 4 + idx + len("\n---\n")
			body := strings.TrimLeft(content[fmEnd:], "\n")
			return content[:fmEnd] + "\n" + prelude + "\n\n" + body
		}
	}
	return prelude + "\n\n" + content
}

// commandNamesFor 返回某平台要写的命令名：CmdAll 为真时三个全写
// （start 保留：无 hook 注入的平台靠 start 作入口兜底；pi 的
// resolvePiCommands 兜底结果同样是三个全写），否则只写
// continue、finish-work（agentCapable && hasHooks 的平台由
// SessionStart hook 注入工作流概览，start 被 filterCommands 滤掉）。
func commandNamesFor(p Platform) []string {
	if p.CmdAll {
		return commandNames
	}
	return []string{"continue", "finish-work"}
}

// BundledSkillsPresent 报告内嵌模板里是否已有翻译好的 bundled-skills
// 目录（B 组负责写入）。为假时 Install 只写出 5 个自动触发 skill，
// 不报错；B 组目录合入后再验证 bundled 部分。
func BundledSkillsPresent() bool {
	entries, err := fs.ReadDir(templates.Platforms, "platforms/common/bundled-skills")
	return err == nil && len(entries) > 0
}

// Install 把单个平台的全部模板文件写入 cwd 下的项目根，返回实际写入的
// 文件相对路径列表（相对项目根，正斜杠，供 .template-hashes.json 使用）。
// 未写入的文件（Force 为假时已存在、合并后内容无变化等）不在列表中。
//
// 写出规则（核对 Trellis configurators/shared.ts、claude/cursor/codex.ts）：
//   - 普通模板文件：不存在时写入；已存在且内容相同视为已就位；已存在
//     且内容不同时，Force 为假跳过、为真覆盖。
//   - .claude/settings.json：与普通模板文件相同，不做合并（阶段 4 定稿，
//     见 NOTES.md 第 3 节与 PLAN.md 阶段 4 实施记录）：已有文件内容不同
//     时 Force 为假跳过、为真覆盖；用户已有的 statusLine 只在
//     `sunshine update` 里按 preserveExistingClaudeStatusLine 保留，
//     Install 不负责。
//   - .codex/agents/sunshine-*.toml：Force 覆盖时保留用户自设的
//     model / model_reasoning_effort（按 codex.ts 的逐行匹配逻辑）。
//   - Codex 安装时确保 .codex/skills/ 空目录存在（无文件可列入返回）。
//   - WithStatusline 仅对 claude-code 生效，其他平台忽略。
func Install(cwd, platformID string, opts InstallOptions) ([]string, error) {
	p, ok := ByID(platformID)
	if !ok {
		return nil, fmt.Errorf("未知平台：%s", platformID)
	}

	files, err := buildFiles(cwd, p, opts)
	if err != nil {
		return nil, err
	}
	// settings.json 单独走合并流程，不进通用写出表。
	settingsContent, hasSettings := files[".claude/settings.json"]
	delete(files, ".claude/settings.json")

	var written []string
	for rel, content := range files {
		ok, err := writeTemplateFile(cwd, rel, content, opts.Force)
		if err != nil {
			return written, err
		}
		if ok {
			written = append(written, rel)
		}
	}

	if p.ID == "claude-code" && hasSettings {
		ok, err := installSettings(cwd, settingsContent, opts.Force)
		if err != nil {
			return written, err
		}
		if ok {
			written = append(written, ".claude/settings.json")
		}
	}

	if p.ID == "codex" {
		// Trellis codex.ts 的残留行为：.codex/skills/ 是用户自有 skill 的
		// 约定位置，模板不向其中写文件，但要保证空目录存在。
		if err := os.MkdirAll(filepath.Join(cwd, ".codex", "skills"), 0o755); err != nil {
			return written, fmt.Errorf("创建 .codex/skills/ 失败：%w", err)
		}
	}

	sort.Strings(written)
	return written, nil
}

// buildFiles 按平台拼出“相对路径 → 渲染后内容”的完整文件表（不含落盘）。
// 注册表驱动的通用引擎：自动触发 skill、bundled skill、命令、
// 子代理定义、平台专属文件五段全部按 Platform 的布局字段走，
// 各平台不再有按 ID 的 switch 分支；仅剩的平台特例是 Codex 的
// agent model 保留（写出前对渲染表做后处理，见下）。
// 所有模板内容先经占位符解析；解析后仍残留 {{...}} 占位符时报错，
// 避免把未解析的模板写到用户项目里。
func buildFiles(cwd string, p Platform, opts InstallOptions) (map[string]string, error) {
	files := make(map[string]string)
	root := skillsRoot(p)

	// 5 个自动触发 skill：写共享目录的平台（codex、gemini、dsh、
	// pi、kimi）用中性解析，保证多平台写同一目录时字节一致；
	// 其余平台用平台解析。reasonix 不写 common 的 check
	// （被子代理版顶替，见 ExcludeCheckSkill）。
	if root != "" {
		for _, name := range autoSkillNames {
			if name == "check" && p.ExcludeCheckSkill {
				continue
			}
			raw, err := templates.Platforms.ReadFile("platforms/common/skills/" + name + ".md")
			if err != nil {
				return nil, fmt.Errorf("读取 skill 模板 %s 失败：%w", name, err)
			}
			var resolved string
			if p.AutoSkillsNeutral {
				resolved = resolvePlaceholdersNeutral(string(raw), p)
			} else {
				resolved = resolvePlaceholders(string(raw), p)
			}
			content, err := wrapSkillFrontmatter("sunshine-"+name, resolved)
			if err != nil {
				return nil, err
			}
			files[root+"/sunshine-"+name+"/SKILL.md"] = content
		}

		// 4 个内置多文件 skill：整目录复制，目录存在时才写出；
		// 一律平台解析，即使写到共享 .agents/skills/ 也是如此
		// （shared.ts resolveBundledSkills）。
		if BundledSkillsPresent() {
			for _, base := range bundledSkillNames {
				if err := collectBundledSkill(files, root, base, p); err != nil {
					return nil, err
				}
			}
		}
	}

	// 命令：按 CmdForm 落盘；CmdAll 为真时三个全写，否则只写
	// continue、finish-work（见 commandNamesFor）。
	if p.CmdForm != "" {
		for _, name := range commandNamesFor(p) {
			raw, err := templates.Platforms.ReadFile("platforms/common/commands/" + name + ".md")
			if err != nil {
				return nil, fmt.Errorf("读取命令模板 %s 失败：%w", name, err)
			}
			var resolved string
			if p.CmdNeutral {
				resolved = resolvePlaceholdersNeutral(string(raw), p)
			} else {
				resolved = resolvePlaceholders(string(raw), p)
			}
			switch p.CmdForm {
			case CmdFormSubdir:
				files[p.CmdDir+"/"+name+".md"] = resolved
			case CmdFormFlat:
				files[p.CmdDir+"/sunshine-"+name+".md"] = resolved
			case CmdFormTOML:
				files[p.CmdDir+"/"+name+".toml"] = wrapGeminiCommandTOML(name, resolved)
			case CmdFormFrontmatter:
				content, err := wrapWithCommandFrontmatter("sunshine-"+name, resolved)
				if err != nil {
					return nil, err
				}
				files[p.CmdDir+"/sunshine-"+name+".md"] = content
			case CmdFormOMP:
				content, err := wrapWithOmpFrontmatter("sunshine-"+name, resolved)
				if err != nil {
					return nil, err
				}
				files[p.CmdDir+"/sunshine-"+name+".md"] = content
			case CmdFormSkill:
				content, err := wrapSkillFrontmatter("sunshine-"+name, resolved)
				if err != nil {
					return nil, err
				}
				files[p.CmdDir+"/sunshine-"+name+"/SKILL.md"] = content
			case CmdFormWorkflow:
				files[p.CmdDir+"/"+p.CmdPrefix+name+".md"] = resolved
			case CmdFormPrompt:
				// Copilot 提示词：裸名 + .prompt.md 后缀，无 frontmatter；
				// 只写 common 命令渲染出的 continue/finish-work，
				// templates/platforms/copilot/prompts/ 下的 13 个携带件
				// 从不进入写出集合（E 规格 §0.1/§9.5）。
				files[p.CmdDir+"/"+name+".prompt.md"] = resolved
			default:
				return nil, fmt.Errorf("平台 %s 的命令形态 %q 未实现", p.ID, p.CmdForm)
			}
			// snow 另写一份命令 JSON（.snow/commands/）。
			if p.CmdJSONDir != "" {
				content, err := buildSnowCommandJSON(name, resolved)
				if err != nil {
					return nil, err
				}
				files[p.CmdJSONDir+"/sunshine-"+name+".json"] = content
			}
		}
	}

	// 子代理定义与 skill 副本：按 AgentFiles / AgentSkillCopies
	// 登记原样写出；Prelude 为真时对 implement/check 注入
	// pull-based prelude（仅 grok、kimi；其余平台的 prelude
	// 已 baked 进模板文件或不需要，见各规格）。
	for _, f := range append(append([]PlatformFile{}, p.AgentFiles...), p.AgentSkillCopies...) {
		raw, err := templates.Platforms.ReadFile(f.Src)
		if err != nil {
			return nil, fmt.Errorf("读取 agent 模板 %s 失败：%w", f.Src, err)
		}
		content := string(raw)
		if f.Prelude {
			jsonl := "implement.jsonl"
			if strings.Contains(f.Dst, "check") {
				jsonl = "check.jsonl"
			}
			content = injectPullBasedPrelude(content, jsonl)
		}
		files[f.Dst] = content
	}

	// 平台专属文件：原样写出（settings/hooks.json/config/
	// 扩展/JS 插件/DSH.md/SNOW.md 等）。
	for _, f := range p.ExtraFiles {
		raw, err := templates.Platforms.ReadFile(f.Src)
		if err != nil {
			return nil, fmt.Errorf("读取平台模板 %s 失败：%w", f.Src, err)
		}
		files[f.Dst] = string(raw)
	}

	// claude 的 --with-statusline：settings.json 注入 statusLine 段。
	if p.ID == "claude-code" && opts.WithStatusline {
		content, ok := files[".claude/settings.json"]
		if !ok {
			return nil, fmt.Errorf("缺少 Claude settings 模板")
		}
		injected, err := injectStatusLine(content)
		if err != nil {
			return nil, fmt.Errorf("注入 statusLine 失败：%w", err)
		}
		files[".claude/settings.json"] = injected
	}

	// Codex agent 的 model 保留：重装时把用户自设的 model /
	// model_reasoning_effort 接回新内容（codex.ts
	// preserveCodexAgentModelKeys，写出前对渲染表做后处理）。
	if p.ID == "codex" {
		preserveCodexAgentModelKeys(cwd, files)
	}

	// 校验：解析后不得残留任何已知占位符（见 findLeftoverPlaceholder）。
	for rel, content := range files {
		if leftover := findLeftoverPlaceholder(content); leftover != "" {
			return nil, fmt.Errorf("模板 %s 解析后残留占位符 %s", rel, leftover)
		}
	}
	return files, nil
}

// collectBundledSkill 把一个内置多文件 skill 的整目录读出并写入文件表。
// srcDir 按 sunshine-<base> 优先、trellis-<base> 兼容的顺序查找
// （B 组写入时目录名以 sunshine- 为准；兼容旧名以防万一）。
// 全部文件都经平台占位符解析（shared.ts resolveBundledSkills），
// SKILL.md 自带 frontmatter，不再另包一层。
func collectBundledSkill(files map[string]string, root, base string, p Platform) error {
	var srcDir string
	for _, cand := range []string{"sunshine-" + base, "trellis-" + base} {
		if st, err := fs.Stat(templates.Platforms, "platforms/common/bundled-skills/"+cand); err == nil && st.IsDir() {
			srcDir = "platforms/common/bundled-skills/" + cand
			break
		}
	}
	if srcDir == "" {
		return nil // 该 skill 尚未合入，跳过
	}
	return fs.WalkDir(templates.Platforms, srcDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		raw, err := templates.Platforms.ReadFile(path)
		if err != nil {
			return err
		}
		rel := strings.TrimPrefix(path, srcDir+"/")
		files[root+"/sunshine-"+base+"/"+rel] = resolvePlaceholders(string(raw), p)
		return nil
	})
}

// writeTemplateFile 按“普通模板文件”规则写出单个文件：
// 不存在 → 写入；内容相同 → 跳过；内容不同 → Force 为真覆盖、为假跳过。
// 返回值表示本次是否实际写盘。
func writeTemplateFile(cwd, rel, content string, force bool) (bool, error) {
	dest := filepath.Join(cwd, filepath.FromSlash(rel))
	if existing, err := os.ReadFile(dest); err == nil {
		if string(existing) == content {
			return false, nil
		}
		if !force {
			return false, nil
		}
	} else if !os.IsNotExist(err) {
		return false, fmt.Errorf("读取 %s 失败：%w", rel, err)
	}
	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		return false, fmt.Errorf("创建 %s 的目录失败：%w", rel, err)
	}
	if err := os.WriteFile(dest, []byte(content), 0o644); err != nil {
		return false, fmt.Errorf("写入 %s 失败：%w", rel, err)
	}
	return true, nil
}

// RenderTemplates 返回单个平台渲染后的完整文件表（相对项目根的正斜杠
// 路径 → 内容），不写盘。供 `sunshine update` 收集“期望模板集合”用；
// Codex 的 model 保留同样生效（读磁盘上已有的 agent toml）。
// WithStatusline 的含义与 Install 相同；update 固定传 false
// （新模板不带 statusLine，已有 statusLine 由 update 的
// preserveExistingClaudeStatusLine 保留，见 internal/update）。
func RenderTemplates(cwd, platformID string, opts InstallOptions) (map[string]string, error) {
	p, ok := ByID(platformID)
	if !ok {
		return nil, fmt.Errorf("未知平台：%s", platformID)
	}
	return buildFiles(cwd, p, opts)
}

// installSettings 处理 .claude/settings.json 的写出（阶段 4 定稿）：
// 与普通模板文件完全相同——不存在时写入；内容相同跳过；内容不同时
// Force 为假跳过、为真覆盖。阶段 2 的“保留全部顶层键、env/hooks 补写”
// 合并已删除（那不是 Trellis 的行为；见 NOTES.md 第 3 节）。
// statusLine 的保留是 `sunshine update` 的职责，不在这里做。
//
// 返回值表示本次是否实际写盘。
func installSettings(cwd, fresh string, force bool) (bool, error) {
	return writeTemplateFile(cwd, ".claude/settings.json", fresh, force)
}

// injectStatusLine 在 settings.json 模板对象末尾追加 statusLine 段，
// 命令为 `SUNSHINE_HOOK_PLATFORM=claude sunshine hook statusline`
// （Go 实现，不写 .py 文件；前缀与 hooks 段一致，见包注释）。
// 与 Trellis claude.ts injectStatusLine 一致：解析 → 末尾赋值 →
// 2 空格缩进 + 末尾换行；flag 关闭时默认输出保持字节一致。
func injectStatusLine(content string) (string, error) {
	obj, err := parseOrderedJSON([]byte(content))
	if err != nil {
		return "", err
	}
	obj.set("statusLine", map[string]any{
		"type":    "command",
		"command": "SUNSHINE_HOOK_PLATFORM=claude sunshine hook statusline",
	})
	out, err := obj.marshalIndent()
	if err != nil {
		return "", err
	}
	return string(out) + "\n", nil
}

// ---------------------------------------------------------------------------
// 占位符解析（对照 shared.ts resolvePlaceholders /
// resolvePlaceholdersNeutral）
// ---------------------------------------------------------------------------

var (
	rePythonCmd       = regexp.MustCompile(`\{\{PYTHON_CMD\}\}`)
	reCmdRef          = regexp.MustCompile(`\{\{CMD_REF:([\w][\w-]*)\}\}`)
	reExecutorAI      = regexp.MustCompile(`\{\{EXECUTOR_AI\}\}`)
	reUserActionLabel = regexp.MustCompile(`\{\{USER_ACTION_LABEL\}\}`)
	reCLIFlag         = regexp.MustCompile(`\{\{CLI_FLAG\}\}`)
	reBlankLines      = regexp.MustCompile(`\n{3,}`)
	reLeftover        = regexp.MustCompile(`\{\{(?:PYTHON_CMD|CMD_REF:[\w][\w-]*|EXECUTOR_AI|USER_ACTION_LABEL|CLI_FLAG|[#^/](?:AGENT_CAPABLE|HAS_HOOKS))\}\}`)
)

func conditionalRegex(flag, prefix string) *regexp.Regexp {
	// prefix 须已转义（如 `\^`）：Go 的 RE2 会把 `\{\{` 后未转义的 `^`
	// 当作锚点，导致取反条件块永远匹配不到。
	return regexp.MustCompile(`\{\{` + prefix + flag + `\}\}([\s\S]*?)\{\{/` + flag + `\}\}`)
}

// resolvePlaceholders 按平台解析模板占位符（shared.ts 同名函数）：
//   - {{PYTHON_CMD}} → "sunshine"（Go 版无 Python 运行时；规范模板
//     翻译时已把脚本调用改写为 sunshine 子命令，此处只是兜底，
//     不应再有模板依赖它，见 NOTES.md）
//   - {{CMD_REF:name}} → <cmdRefPrefix><name>
//   - {{EXECUTOR_AI}} / {{USER_ACTION_LABEL}} / {{CLI_FLAG}} → 平台上下文值
//   - {{#FLAG}}...{{/FLAG}} 条件包含（FLAG 为真时保留内容）、
//     {{^FLAG}}...{{/FLAG}} 条件取反；支持 AGENT_CAPABLE、HAS_HOOKS
//   - 解析后把条件块删除留下的 3 个以上连续换行压成两个
func resolvePlaceholders(content string, p Platform) string {
	result := rePythonCmd.ReplaceAllString(content, "sunshine")
	result = reCmdRef.ReplaceAllStringFunc(result, func(m string) string {
		name := reCmdRef.FindStringSubmatch(m)[1]
		return p.CmdRefPrefix + name
	})
	result = reExecutorAI.ReplaceAllString(result, p.executorAI)
	result = reUserActionLabel.ReplaceAllString(result, p.userActionLabel)
	result = reCLIFlag.ReplaceAllString(result, p.Flag)
	result = resolveConditionals(result, p)
	result = reBlankLines.ReplaceAllString(result, "\n\n")
	return result
}

// resolvePlaceholdersNeutral 与 resolvePlaceholders 相同，唯独
// {{CMD_REF:name}} 渲染为中性形式 "`name` (Sunshine command)"，
// 使共享 .agents/skills/ 下的自动触发 skill 在任何平台写出时
// 字节一致（shared.ts resolvePlaceholdersNeutral；Trellis 原文为
// "(Trellis command)"，此处按改名规则改为 Sunshine）。
// {{CLI_FLAG}} 等其余占位符仍按平台解析——命令转 skill 的文件
// （start/continue/finish-work）只有 Codex 写到共享目录，
// 不需要跨平台字节一致。
func resolvePlaceholdersNeutral(content string, p Platform) string {
	result := rePythonCmd.ReplaceAllString(content, "sunshine")
	result = reCmdRef.ReplaceAllStringFunc(result, func(m string) string {
		name := reCmdRef.FindStringSubmatch(m)[1]
		return "`" + name + "` (Sunshine command)"
	})
	result = reExecutorAI.ReplaceAllString(result, p.executorAI)
	result = reUserActionLabel.ReplaceAllString(result, p.userActionLabel)
	result = reCLIFlag.ReplaceAllString(result, p.Flag)
	result = resolveConditionals(result, p)
	result = reBlankLines.ReplaceAllString(result, "\n\n")
	return result
}

func resolveConditionals(content string, p Platform) string {
	flags := map[string]bool{
		"AGENT_CAPABLE": p.AgentCapable,
		"HAS_HOOKS":     p.HasHooks,
	}
	result := content
	for flag, value := range flags {
		pos := conditionalRegex(flag, "#")
		neg := conditionalRegex(flag, `\^`)
		result = pos.ReplaceAllStringFunc(result, func(m string) string {
			if value {
				return pos.FindStringSubmatch(m)[1]
			}
			return ""
		})
		result = neg.ReplaceAllStringFunc(result, func(m string) string {
			if value {
				return ""
			}
			return neg.FindStringSubmatch(m)[1]
		})
	}
	return result
}

// findLeftoverPlaceholder 返回内容中第一个残留的已知占位符，
// 没有则返回 ""。只认解析器支持的占位符名：模板正文里可能出现
// 其他 `{{...}}` 字样（例如 bundled-skills 文档中提到的
// `{{platform_name}}`、`{{python_cmd}}`——Trellis 原文如此，
// shared.ts 的解析器本身也不处理它们），那些是文档措辞，
// 不是待解析的占位符，不应误报。
func findLeftoverPlaceholder(content string) string {
	return reLeftover.FindString(content)
}

// ---------------------------------------------------------------------------
// Skill frontmatter（对照 shared.ts wrapWithSkillFrontmatter）
// ---------------------------------------------------------------------------

// skillDescriptions 是 skill 的自动触发描述（中文译文，源自 shared.ts
// SKILL_DESCRIPTIONS；name 在 frontmatter 里用 sunshine- 前缀）。
var skillDescriptions = map[string]string{
	"start":       "初始化 AI 开发会话：读取工作流指南、开发者身份、Git 状态、当前任务与项目规范（.sunshine/）。对收到的任务分类，分流到头脑风暴、直接修改或任务工作流。适用于开始新的编码会话、恢复工作、开始新任务或重建项目上下文。",
	"continue":    "继续当前任务。读取工作流阶段索引，找到应续接的阶段与步骤，再拉取步骤细节。适用于回到进行中的任务、需要知道下一步做什么时。",
	"finish-work": "收尾当前会话：确认质量门已通过、提醒用户提交、归档已完成任务、把会话进展记入开发者日志。适用于编码结束、准备收尾时。",
	"before-dev":  "在动手实现前，发现并注入 .sunshine/spec/ 中的项目专属编码规范。读取规范索引、开发前检查清单与目标包的共享思考指南。适用于开始新编码任务、写代码之前、切换到不同包，或需要刷新项目约定与标准时。",
	"brainstorm":  "在实现前协作澄清需求。创建任务目录、生成 PRD、一次只问一个高价值问题、调研技术选型、收敛到 MVP 范围。适用于需求不清晰、有多种可行方案，或用户描述新功能、复杂任务时。",
	"check":       "全面质量核查：规范符合性、Lint、类型检查、测试、跨层数据流、代码复用与一致性。适用于代码写完需要质量核查、提交前，或长会话中需要发现上下文漂移时。",
	"break-loop":  "深度 Bug 分析，打破“修了忘、忘了再修”的循环。分析根因类别、修复失败的原因、预防机制，并把知识沉淀进规范。适用于修完 Bug 后防止同类 Bug 再次出现。",
	"update-spec": "把可执行的契约与编码约定沉淀进 .sunshine/spec/ 文档。适用于从调试、实现或讨论中学到值得保留的东西时。",
}

// wrapSkillFrontmatter 给解析后的 skill 内容包上 YAML frontmatter
// （name + description），与 Trellis wrapWithSkillFrontmatter 一致；
// 未登记的 skill 名直接报错，避免静默写出无描述的 SKILL.md。
func wrapSkillFrontmatter(name, content string) (string, error) {
	base := strings.TrimPrefix(name, "sunshine-")
	desc, ok := skillDescriptions[base]
	if !ok {
		return "", fmt.Errorf("skill %q 没有登记描述，请在 skillDescriptions 中补充", base)
	}
	quoted, err := json.Marshal(desc)
	if err != nil {
		return "", err
	}
	return "---\nname: " + name + "\ndescription: " + string(quoted) + "\n---\n\n" + content, nil
}

// ---------------------------------------------------------------------------
// Codex agent model 保留（对照 codex.ts extract/applyCodexAgentModelKeys）
// ---------------------------------------------------------------------------

// codexAgentModelKeys 是用户在 sunshine-*.toml 里自设的顶层 model 配置。
type codexAgentModelKeys struct {
	model                string
	modelReasoningEffort string
}

var (
	reCodexMultilineStart = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_-]*\s*=\s*"""`)
	reCodexModelKV        = regexp.MustCompile(`^(model|model_reasoning_effort)\s*=\s*"((?:[^"\\]|\\.)*)"\s*(?:#.*)?$`)
	reCodexSandboxLine    = regexp.MustCompile(`(?m)^(sandbox_mode\s*=\s*".*"\n)`)
)

// extractCodexAgentModelKeys 从已有的 agent toml 内容中提取未注释的
// model / model_reasoning_effort 顶层键。逐行匹配：注释行（# model = ...）
// 不匹配；多行字符串（"""..."""）内的形似行不提取。
// 与 codex.ts 的实现逐行对应。
func extractCodexAgentModelKeys(existing string) codexAgentModelKeys {
	var result codexAgentModelKeys
	inMultiline := false
	for _, rawLine := range strings.Split(existing, "\n") {
		trimmed := strings.TrimSpace(rawLine)
		if inMultiline {
			if strings.Contains(trimmed, `"""`) {
				inMultiline = false
			}
			continue
		}
		if reCodexMultilineStart.MatchString(trimmed) {
			if strings.Count(trimmed, `"""`) < 2 {
				inMultiline = true
			}
			continue
		}
		m := reCodexModelKV.FindStringSubmatch(trimmed)
		if m == nil {
			continue
		}
		value := strings.ReplaceAll(strings.ReplaceAll(m[2], `\"`, `"`), `\\`, `\`)
		switch m[1] {
		case "model":
			result.model = value
		case "model_reasoning_effort":
			result.modelReasoningEffort = value
		}
	}
	return result
}

func tomlEscape(value string) string {
	value = strings.ReplaceAll(value, `\`, `\\`)
	return strings.ReplaceAll(value, `"`, `\"`)
}

// applyCodexAgentModelKeys 把保留的 model 键插回新内容
// sandbox_mode = "..." 行之后。模板本身只含注释掉的提示行，
// 不会与插入行重复；无保留键时原样返回。
func applyCodexAgentModelKeys(fresh string, preserved codexAgentModelKeys) string {
	var lines []string
	if preserved.model != "" {
		lines = append(lines, `model = "`+tomlEscape(preserved.model)+`"`)
	}
	if preserved.modelReasoningEffort != "" {
		lines = append(lines, `model_reasoning_effort = "`+tomlEscape(preserved.modelReasoningEffort)+`"`)
	}
	if len(lines) == 0 {
		return fresh
	}
	return reCodexSandboxLine.ReplaceAllStringFunc(fresh, func(m string) string {
		return m + strings.Join(lines, "\n") + "\n"
	})
}

func isCodexAgentTomlPath(path string) bool {
	return strings.HasPrefix(path, ".codex/agents/sunshine-") && strings.HasSuffix(path, ".toml")
}

// preserveCodexAgentModelKeys 在写出前把磁盘上已有 agent toml 的
// model 键接回文件表（codex.ts preserveCodexAgentModelKeys）。
// 无对应磁盘文件时跳过。
func preserveCodexAgentModelKeys(cwd string, files map[string]string) {
	for rel, fresh := range files {
		if !isCodexAgentTomlPath(rel) {
			continue
		}
		existing, err := os.ReadFile(filepath.Join(cwd, filepath.FromSlash(rel)))
		if err != nil {
			continue
		}
		preserved := extractCodexAgentModelKeys(string(existing))
		files[rel] = applyCodexAgentModelKeys(fresh, preserved)
	}
}
