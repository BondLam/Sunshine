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
type Platform struct {
	ID                  string // "claude-code" | "cursor" | "codex"
	Name                string // "Claude Code" | "Cursor" | "Codex"
	Flag                string // "claude" | "cursor" | "codex"
	ConfigDir           string // 平台配置目录：".claude" | ".cursor" | ".codex"
	DefaultChecked      bool   // claude/cursor 为 true，codex 为 false
	CmdRefPrefix        string // "/sunshine:" | "/sunshine-" | "$"
	AgentCapable        bool   // 三家均为 true
	HasHooks            bool   // claude/cursor 为 true，codex 为 false
	SupportsAgentSkills bool   // 仅 codex 为 true

	// 以下两项是 Trellis templateContext 的其余字段，模板内目前
	// 没有占位符引用它们，但占位符解析器按 shared.ts 实现它们，
	// 以便后续模板直接使用。
	executorAI      string
	userActionLabel string
}

// registry 是平台注册表，唯一事实源；后续平台按同一结构扩展。
var registry = []Platform{
	{
		ID: "claude-code", Name: "Claude Code", Flag: "claude", ConfigDir: ".claude",
		DefaultChecked: true, CmdRefPrefix: "/sunshine:",
		AgentCapable: true, HasHooks: true, SupportsAgentSkills: false,
		executorAI: "Bash scripts or Task calls", userActionLabel: "Slash commands",
	},
	{
		ID: "cursor", Name: "Cursor", Flag: "cursor", ConfigDir: ".cursor",
		DefaultChecked: true, CmdRefPrefix: "/sunshine-",
		AgentCapable: true, HasHooks: true, SupportsAgentSkills: false,
		executorAI: "Bash scripts or Task calls", userActionLabel: "Slash commands",
	},
	{
		ID: "codex", Name: "Codex", Flag: "codex", ConfigDir: ".codex",
		DefaultChecked: false, CmdRefPrefix: "$",
		AgentCapable: true, HasHooks: false, SupportsAgentSkills: true,
		executorAI: "Bash scripts or tool calls", userActionLabel: "Skills",
	},
}

// Registry 返回全部已注册平台（按 claude-code、cursor、codex 顺序）。
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

// skillsRoot 返回平台的 skill 根目录（相对项目根，正斜杠）。
func skillsRoot(p Platform) string {
	switch p.ID {
	case "claude-code":
		return ".claude/skills"
	case "cursor":
		return ".cursor/skills"
	case "codex":
		return ".agents/skills"
	default:
		return ""
	}
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
// 所有模板内容先经占位符解析；解析后仍残留 {{...}} 占位符时报错，
// 避免把未解析的模板写到用户项目里。
func buildFiles(cwd string, p Platform, opts InstallOptions) (map[string]string, error) {
	files := make(map[string]string)
	root := skillsRoot(p)

	// 5 个自动触发 skill：Codex 写到共享 .agents/skills/，用中性解析
	// 保证多平台写同一目录时字节一致；Claude/Cursor 用平台解析。
	for _, name := range autoSkillNames {
		raw, err := templates.Platforms.ReadFile("platforms/common/skills/" + name + ".md")
		if err != nil {
			return nil, fmt.Errorf("读取 skill 模板 %s 失败：%w", name, err)
		}
		var resolved string
		if p.ID == "codex" {
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

	// 4 个内置多文件 skill：整目录复制，目录存在时才写出。
	if BundledSkillsPresent() {
		for _, base := range bundledSkillNames {
			if err := collectBundledSkill(files, root, base, p); err != nil {
				return nil, err
			}
		}
	}

	// 命令：agentCapable 且 hasHooks 的平台（Claude、Cursor）由
	// SessionStart hook 自动注入工作流概览，start 命令多余，不生成
	// （shared.ts filterCommands）；Codex 无 hook，start 保留并转成 skill。
	if p.ID == "codex" {
		for _, name := range commandNames {
			raw, err := templates.Platforms.ReadFile("platforms/common/commands/" + name + ".md")
			if err != nil {
				return nil, fmt.Errorf("读取命令模板 %s 失败：%w", name, err)
			}
			content, err := wrapSkillFrontmatter("sunshine-"+name, resolvePlaceholdersNeutral(string(raw), p))
			if err != nil {
				return nil, err
			}
			files[".agents/skills/sunshine-"+name+"/SKILL.md"] = content
		}
	} else {
		for _, name := range []string{"continue", "finish-work"} {
			raw, err := templates.Platforms.ReadFile("platforms/common/commands/" + name + ".md")
			if err != nil {
				return nil, fmt.Errorf("读取命令模板 %s 失败：%w", name, err)
			}
			resolved := resolvePlaceholders(string(raw), p)
			switch p.ID {
			case "claude-code":
				files[".claude/commands/sunshine/"+name+".md"] = resolved
			case "cursor":
				files[".cursor/commands/sunshine-"+name+".md"] = resolved
			}
		}
	}

	// 子代理定义：各平台源模板内容不同（frontmatter 描述不同），直接取
	// 平台专属模板原样写出；源模板不含占位符，不做解析（与 Trellis
	// walkClaudeTemplateDir / listMdAgents / getAllAgents 一致）。
	switch p.ID {
	case "claude-code":
		for _, name := range agentNames {
			raw, err := templates.Platforms.ReadFile("platforms/claude/agents/sunshine-" + name + ".md")
			if err != nil {
				return nil, fmt.Errorf("读取 Claude agent 模板 %s 失败：%w", name, err)
			}
			files[".claude/agents/sunshine-"+name+".md"] = string(raw)
		}
	case "cursor":
		for _, name := range agentNames {
			raw, err := templates.Platforms.ReadFile("platforms/cursor/agents/sunshine-" + name + ".md")
			if err != nil {
				return nil, fmt.Errorf("读取 Cursor agent 模板 %s 失败：%w", name, err)
			}
			files[".cursor/agents/sunshine-"+name+".md"] = string(raw)
		}
	case "codex":
		for _, name := range agentNames {
			raw, err := templates.Platforms.ReadFile("platforms/codex/agents/sunshine-" + name + ".toml")
			if err != nil {
				return nil, fmt.Errorf("读取 Codex agent 模板 %s 失败：%w", name, err)
			}
			files[".codex/agents/sunshine-"+name+".toml"] = string(raw)
		}
	}

	// 平台专属配置。
	switch p.ID {
	case "claude-code":
		raw, err := templates.Platforms.ReadFile("platforms/claude/settings.json")
		if err != nil {
			return nil, fmt.Errorf("读取 Claude settings 模板失败：%w", err)
		}
		content := string(raw)
		if opts.WithStatusline {
			content, err = injectStatusLine(content)
			if err != nil {
				return nil, fmt.Errorf("注入 statusLine 失败：%w", err)
			}
		}
		files[".claude/settings.json"] = content
	case "cursor":
		raw, err := templates.Platforms.ReadFile("platforms/cursor/hooks.json")
		if err != nil {
			return nil, fmt.Errorf("读取 Cursor hooks 模板失败：%w", err)
		}
		files[".cursor/hooks.json"] = string(raw)
	case "codex":
		raw, err := templates.Platforms.ReadFile("platforms/codex/hooks.json")
		if err != nil {
			return nil, fmt.Errorf("读取 Codex hooks 模板失败：%w", err)
		}
		files[".codex/hooks.json"] = string(raw)
		raw, err = templates.Platforms.ReadFile("platforms/codex/config.toml")
		if err != nil {
			return nil, fmt.Errorf("读取 Codex config 模板失败：%w", err)
		}
		files[".codex/config.toml"] = string(raw)
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
