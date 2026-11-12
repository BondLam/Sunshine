package cli

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/AlecAivazis/survey/v2"
	"github.com/BondLam/Sunshine/internal/platforms"
	"github.com/BondLam/Sunshine/internal/projecttype"
	"github.com/BondLam/Sunshine/internal/registry"
	"github.com/BondLam/Sunshine/internal/scaffold"
	"github.com/spf13/cobra"
	"golang.org/x/term"
)

var (
	initUser           string
	initYes            bool
	initForce          bool
	initClaude         bool
	initCursor         bool
	initCodex          bool
	initKiro           bool
	initGemini         bool
	initQoder          bool
	initPi             bool
	initZcode          bool
	initTrae           bool
	initOmp            bool
	initSnow           bool
	initOpencode       bool
	initDsh            bool
	initReasonix       bool
	initGrok           bool
	initKimi           bool
	initKilo           bool
	initAntigravity    bool
	initDevin          bool
	initWindsurf       bool
	initCodebuddy      bool
	initCopilot        bool
	initDroid          bool
	initWithStatusline bool
	initType           string
)

// platformIDs 是 init 支持的全部 22 个平台，按安装顺序排列
// （与 internal/platforms 注册表顺序一致）。
var platformIDs = []string{
	"claude-code", "cursor", "codex",
	"kiro", "gemini", "qoder",
	"pi", "zcode", "trae", "omp", "snow",
	"opencode", "dsh", "reasonix", "grok", "kimi",
	"kilo", "antigravity", "devin",
	"codebuddy", "copilot", "droid",
}

// platformDisplayNames 是平台 ID 到中文展示名的映射。
var platformDisplayNames = map[string]string{
	"claude-code": "Claude Code",
	"cursor":      "Cursor",
	"codex":       "Codex",
	"kiro":        "Kiro Code",
	"gemini":      "Gemini CLI",
	"qoder":       "Qoder",
	"pi":          "Pi Agent",
	"zcode":       "ZCode",
	"trae":        "Trae",
	"omp":         "Oh My Pi",
	"snow":        "Snow CLI",
	"opencode":    "OpenCode",
	"dsh":         "DeepSeek Harness (dsh)",
	"reasonix":    "Reasonix",
	"grok":        "Grok Build",
	"kimi":        "Kimi Code",
	"kilo":        "Kilo CLI",
	"antigravity": "Antigravity",
	"devin":       "Devin",
	"codebuddy":   "CodeBuddy",
	"copilot":     "GitHub Copilot",
	"droid":       "Factory Droid",
}

var initCmd = &cobra.Command{
	Use:   "init",
	Short: "初始化当前项目，生成 .sunshine/ 工程骨架",
	Long: `在当前目录生成 .sunshine/ 工程骨架，包括：
  - config.yaml：项目配置
  - workflow.md：标准工作流
  - spec/：团队规范模板（中文）
  - AGENTS.md：给 AI 助手的工作说明
  - tasks/、workspace/：任务与协作区

并按选择的平台安装对应的 AI 编程助手配置
（22 个平台：Claude Code、Cursor、Codex、Kiro、Gemini、Qoder、
Pi、ZCode、Trae、Oh My Pi、Snow、OpenCode、DeepSeek Harness、
Reasonix、Grok、Kimi、Kilo、Antigravity、Devin、CodeBuddy、
GitHub Copilot、Factory Droid）。

重复执行 init 时进入增量模式：只追加新选的平台，
不动已有平台文件与 .sunshine/ 骨架。

--registry 从模板市场安装规范模板：
  - 有 index.json 的来源是模板市场，须用 --template 指定条目 id；
  - 无 index.json 的来源走直接下载，整个 registry 内容装进 .sunshine/spec；
  - 装完后来源与模板会记进 .sunshine/config.yaml 的 registry.spec，
    sunshine update 按它持续刷新。
  - 交互模式（不带 -y）且未传 --template 时会弹出规范模板选单：
    从零开始，或从模板市场里选一个（也可输入自定义来源）。

示例：
  sunshine init -u 张三 -y
  sunshine init --codex
  sunshine init --claude --with-statusline
  sunshine init --registry gh:owner/repo --template nextjs-fullstack -y
  sunshine init --registry /path/to/spec-repo -y

项目类型检测：init 会检测项目类型（读 package.json 依赖与常见前后端
配置文件），只生成对应的 spec 模板；检测不到时交互问一句；--type 可显式
指定（合法值：前端、后端、全栈、产品设计）；-y 或非交互终端时检测不到默认全栈；
--registry 写 spec 时不检测、不提问。产品设计不参与检测，只能手动选：
选它只生成通用规范（system.md）与 doc/ 设计文档模板（需求列表、概要设计、原型），
不生成前后端规范（技术栈待定，以后定了再 init -f 补）。`,
	RunE: func(cmd *cobra.Command, args []string) error {
		printInitBanner()
		cwd, err := os.Getwd()
		if err != nil {
			return err
		}
		initialized := dirExists(filepath.Join(cwd, ".sunshine"))

		selected, withStatusline, err := resolvePlatformSelection(cmd)
		if err != nil {
			return err
		}
		if !initialized && len(selected) == 0 {
			fmt.Println("未选择任何平台，初始化已取消。")
			return nil
		}

		// 交互模式且未传 --template 时先出规范模板选单（对齐 Trellis 的
		// "Select a spec template"）：选单结果写回 initRegistry /
		// initTemplate，后续流程与 flag 驱动完全一致。-y、非交互终端、
		// 已传 --template 时不走选单。
		var target *registry.Target
		var strategy registry.TemplateStrategy
		if initTemplate == "" && !initYes && stdinIsTTY() {
			var picked string
			initRegistry, picked, target, err = resolveInteractiveTemplate(initRegistry)
			if err != nil {
				return err
			}
			initTemplate = picked
		}

		// --registry 相关 flag 校验与目标解析（在 scaffold.Init 之前，
		// 以便按目标是否写 spec 决定是否跳过默认 spec 模板）。
		if initRegistry != "" || initTemplate != "" {
			if initTemplate != "" && initRegistry == "" {
				if target != nil {
					target.Close()
				}
				return fmt.Errorf("--template 需要配合 --registry 使用")
			}
			strategy, err = resolveRegistryStrategy()
			if err != nil {
				if target != nil {
					target.Close()
				}
				return err
			}
			if target == nil {
				target, err = registry.ResolveTarget(initRegistry, initTemplate)
				if err != nil {
					return err
				}
			}
			// 探测资源（临时克隆）由安装步骤接管关闭；任何提前返回
			// 路径都由这层 defer 兜底（安装后再次 Close 是安全的）。
			defer target.Close()
		}
		skipSpec := target != nil && target.WritesSpec()

		switch {
		case !initialized:
			user := resolveUser(initUser)
			specType, typeSource, err := resolveInitProjectType(cwd, skipSpec)
			if err != nil {
				return err
			}
			if err := scaffold.InitWithOptions(cwd, user, scaffold.InitOptions{SkipSpec: skipSpec, SpecType: specType, Version: version}); err != nil {
				return err
			}
			installed, err := installPlatforms(cwd, selected, withStatusline, false)
			if err != nil {
				return err
			}
			if target != nil {
				installRegistryTemplates(cwd, target, strategy)
			}
			fmt.Printf("初始化完成，开发者：%s\n", user)
			if !skipSpec {
				fmt.Printf("项目类型：%s（%s）\n", specType.DisplayName(), typeSource)
			}
			printPlatformSummary(installed, withStatusline)
			fmt.Println("下一步：")
			fmt.Println("  1. 在 agent 界面里打开项目，直接用自然语言提第一个需求；")
			if skipSpec {
				fmt.Println("  2. 规范已从模板市场装进 .sunshine/spec/，工作流按 .sunshine/workflow.md 自动推进，一般不用改。")
			} else {
				fmt.Println(specNextStepLine(specType))
			}
			return nil
		case initForce:
			user := resolveUser(initUser)
			specType, typeSource, err := resolveInitProjectType(cwd, skipSpec)
			if err != nil {
				return err
			}
			if err := scaffold.InitWithOptions(cwd, user, scaffold.InitOptions{Force: true, SkipSpec: skipSpec, SpecType: specType, Version: version}); err != nil {
				return err
			}
			installed, err := installPlatforms(cwd, selected, withStatusline, true)
			if err != nil {
				return err
			}
			if target != nil {
				installRegistryTemplates(cwd, target, strategy)
			}
			fmt.Printf("重新初始化完成，开发者：%s\n", user)
			if !skipSpec {
				fmt.Printf("项目类型：%s（%s）\n", specType.DisplayName(), typeSource)
			}
			printPlatformSummary(installed, withStatusline)
			return nil
		default:
			// 增量模式：跳过 scaffold.Init，只安装新选的平台与 registry 模板。
			toInstall, skipped := filterUninstalled(cwd, selected)
			for _, id := range skipped {
				fmt.Printf("  ○ %s 已安装，跳过\n", platformDisplayNames[id])
			}
			if len(toInstall) == 0 && target == nil {
				fmt.Println("所选平台均已安装，没有需要安装的新平台。")
				return nil
			}
			if len(toInstall) > 0 {
				installed, err := installPlatforms(cwd, toInstall, withStatusline, false)
				if err != nil {
					return err
				}
				printPlatformSummary(installed, withStatusline)
			}
			if target != nil {
				installRegistryTemplates(cwd, target, strategy)
			}
			return nil
		}
	},
}

func init() {
	initCmd.Flags().StringVarP(&initUser, "user", "u", "", "开发者名称（默认读取 git 配置，未配置时为 developer）")
	initCmd.Flags().BoolVarP(&initYes, "yes", "y", false, "跳过全部提问，使用默认配置")
	initCmd.Flags().BoolVarP(&initForce, "force", "f", false, "已初始化时仍覆盖重建")
	initCmd.Flags().BoolVar(&initClaude, "claude", false, "安装 Claude Code 平台配置")
	initCmd.Flags().BoolVar(&initCursor, "cursor", false, "安装 Cursor 平台配置")
	initCmd.Flags().BoolVar(&initCodex, "codex", false, "安装 Codex 平台配置")
	initCmd.Flags().BoolVar(&initKiro, "kiro", false, "安装 Kiro Code 平台配置")
	initCmd.Flags().BoolVar(&initGemini, "gemini", false, "安装 Gemini CLI 平台配置")
	initCmd.Flags().BoolVar(&initQoder, "qoder", false, "安装 Qoder 平台配置")
	initCmd.Flags().BoolVar(&initPi, "pi", false, "安装 Pi Agent 平台配置")
	initCmd.Flags().BoolVar(&initZcode, "zcode", false, "安装 ZCode 平台配置")
	initCmd.Flags().BoolVar(&initTrae, "trae", false, "安装 Trae 平台配置")
	initCmd.Flags().BoolVar(&initOmp, "omp", false, "安装 Oh My Pi 平台配置")
	initCmd.Flags().BoolVar(&initSnow, "snow", false, "安装 Snow CLI 平台配置")
	initCmd.Flags().BoolVar(&initOpencode, "opencode", false, "安装 OpenCode 平台配置")
	initCmd.Flags().BoolVar(&initDsh, "dsh", false, "安装 DeepSeek Harness 平台配置")
	initCmd.Flags().BoolVar(&initReasonix, "reasonix", false, "安装 Reasonix 平台配置")
	initCmd.Flags().BoolVar(&initGrok, "grok", false, "安装 Grok Build 平台配置")
	initCmd.Flags().BoolVar(&initKimi, "kimi", false, "安装 Kimi Code 平台配置")
	initCmd.Flags().BoolVar(&initKilo, "kilo", false, "安装 Kilo CLI 平台配置")
	initCmd.Flags().BoolVar(&initAntigravity, "antigravity", false, "安装 Antigravity 平台配置")
	initCmd.Flags().BoolVar(&initDevin, "devin", false, "安装 Devin 平台配置")
	initCmd.Flags().BoolVar(&initWindsurf, "windsurf", false, "已废弃的 --devin 别名（Windsurf 已更名为 Devin）")
	initCmd.Flags().BoolVar(&initCodebuddy, "codebuddy", false, "安装 CodeBuddy 平台配置")
	initCmd.Flags().BoolVar(&initCopilot, "copilot", false, "安装 GitHub Copilot 平台配置")
	initCmd.Flags().BoolVar(&initDroid, "droid", false, "安装 Factory Droid 平台配置")
	initCmd.Flags().BoolVar(&initWithStatusline, "with-statusline", false, "为 Claude Code 安装状态栏（仅 Claude Code 入选时有效）")
	initCmd.Flags().StringVar(&initType, "type", "", "项目类型：前端、后端、全栈、产品设计（默认自动检测，检测不到时交互选择）")
	registerRegistryFlags(initCmd)
	rootCmd.AddCommand(initCmd)
}

// resolveUser 按“-u 参数 > git user.name > developer”的顺序确定开发者名称。
func resolveUser(flag string) string {
	if strings.TrimSpace(flag) != "" {
		return strings.TrimSpace(flag)
	}
	if out, err := exec.Command("git", "config", "user.name").Output(); err == nil {
		if name := strings.TrimSpace(string(out)); name != "" {
			return name
		}
	}
	return "developer"
}

// dirExists 判断 path 是否为已存在的目录。
func dirExists(path string) bool {
	st, err := os.Stat(path)
	return err == nil && st.IsDir()
}

// platformSelectionInput 是 resolveSelection 的输入（与 cobra 解耦，便于测试）。
type platformSelectionInput struct {
	explicit            []string // 显式传入的平台 flag 对应的平台 ID
	yes                 bool
	withStatusline      bool
	withStatuslineGiven bool // 是否显式传了 --with-statusline
	isTTY               bool
}

// promptPlatforms 弹出平台多选（默认实现用 survey；测试可替换）。
var promptPlatforms = func(defaultIDs []string) ([]string, error) {
	var options []string
	for _, id := range platformIDs {
		options = append(options, platformDisplayNames[id])
	}
	var defaults []string
	for _, id := range defaultIDs {
		defaults = append(defaults, platformDisplayNames[id])
	}
	var chosen []string
	err := survey.AskOne(&survey.MultiSelect{
		Message: "请选择要配置的 AI 编程平台：",
		Options: options,
		Default: defaults,
	}, &chosen)
	if err != nil {
		return nil, err
	}
	var ids []string
	for _, label := range chosen {
		for _, id := range platformIDs {
			if platformDisplayNames[id] == label {
				ids = append(ids, id)
			}
		}
	}
	return ids, nil
}

// promptStatusline 追问是否安装 Claude Code 状态栏（默认实现用 survey；测试可替换）。
var promptStatusline = func() (bool, error) {
	var answer bool
	err := survey.AskOne(&survey.Confirm{
		Message: "是否为 Claude Code 安装状态栏（statusline，显示模型、上下文、分支与限流信息）？",
		Default: false,
	}, &answer)
	return answer, err
}

// promptProjectType 在检测不到项目类型时交互提问（默认实现用 survey；
// 测试可替换）。选项与 projecttype.Parse 的合法值一一对应。
var promptProjectType = func() (projecttype.Type, error) {
	var chosen string
	err := survey.AskOne(&survey.Select{
		Message: "检测不到项目类型，请选择：",
		Options: []string{"前端项目", "后端项目", "前后端全栈", "产品设计"},
		Default: "前后端全栈",
	}, &chosen)
	if err != nil {
		return projecttype.Fullstack, err
	}
	switch chosen {
	case "前端项目":
		return projecttype.Frontend, nil
	case "后端项目":
		return projecttype.Backend, nil
	case "产品设计":
		return projecttype.Design, nil
	default:
		return projecttype.Fullstack, nil
	}
}

// projectTypeInput 是 resolveProjectType 的输入（与 cobra 解耦，便于测试）。
type projectTypeInput struct {
	flag     string           // --type 原值，为空表示未指定
	skipSpec bool             // --registry 写 spec，类型用不上
	yes      bool             // -y：跳过全部提问
	isTTY    bool             // 标准输入是否为交互终端
	detected projecttype.Type // 已检测到的类型（Unknown 表示检测不到）
}

// resolveProjectType 按“--type > 自动检测 > 交互提问 > 默认全栈”的优先级
// 确定本次 init 的项目类型，并返回类型来源
// （"--type 指定" / "自动检测" / "手动选择" / "默认"）。
// --type 非法时直接返回错误；提问出错或 EOF 时静默默认全栈。
func resolveProjectType(in projectTypeInput) (projecttype.Type, string, error) {
	if strings.TrimSpace(in.flag) != "" {
		t, err := projecttype.Parse(strings.TrimSpace(in.flag))
		if err != nil {
			return projecttype.Unknown, "", err
		}
		return t, "--type 指定", nil
	}
	if in.skipSpec {
		return projecttype.Fullstack, "默认", nil
	}
	if in.detected != projecttype.Unknown {
		return in.detected, "自动检测", nil
	}
	if in.yes || !in.isTTY {
		return projecttype.Fullstack, "默认", nil
	}
	t, err := promptProjectType()
	if err != nil {
		return projecttype.Fullstack, "默认", nil
	}
	return t, "手动选择", nil
}

// resolveInitProjectType 是 init 命令的接线层：只在需要生成默认 spec 时
// 才检测（--type 已指定或 --registry 写 spec 时不检测），再走优先级解析。
func resolveInitProjectType(cwd string, skipSpec bool) (projecttype.Type, string, error) {
	detected := projecttype.Unknown
	if initType == "" && !skipSpec {
		detected = projecttype.Detect(cwd)
	}
	return resolveProjectType(projectTypeInput{
		flag:     initType,
		skipSpec: skipSpec,
		yes:      initYes,
		isTTY:    stdinIsTTY(),
		detected: detected,
	})
}

// specNextStepLine 生成“下一步”第 2 行（默认 spec 情形）：按实际生成的
// spec 文件集措辞；--registry 的“从模板市场装进”写法不在这里。
func specNextStepLine(specType projecttype.Type) string {
	var generated string
	switch specType {
	case projecttype.Frontend:
		generated = "system.md、frontend.md"
	case projecttype.Backend:
		generated = "system.md、backend.md"
	case projecttype.Design:
		return "  2. 规范已按默认模板生成在 .sunshine/spec/（已生成 system.md、frontend.md、backend.md；有团队规范时再补充），产品设计文档模板已生成在 doc/（需求列表、概要设计、原型），工作流按 .sunshine/workflow.md 自动推进，一般不用改。"
	default:
		generated = "system.md、frontend.md、backend.md"
	}
	return fmt.Sprintf("  2. 规范已按默认模板生成在 .sunshine/spec/（已生成 %s；有团队规范时再补充），工作流按 .sunshine/workflow.md 自动推进，一般不用改。", generated)
}

// explicitPlatformIDs 把各平台 flag（以平台 ID 为键）转成平台 ID
// （按 platformIDs 的固定顺序；未传任何 flag 时返回空）。
func explicitPlatformIDs(flags map[string]bool) []string {
	var ids []string
	for _, id := range platformIDs {
		if flags[id] {
			ids = append(ids, id)
		}
	}
	return ids
}

// explicitPlatformFlags 收集当前命令行传入的全部平台 flag。
func explicitPlatformFlags() map[string]bool {
	return map[string]bool{
		"claude-code": initClaude,
		"cursor":      initCursor,
		"codex":       initCodex,
		"kiro":        initKiro,
		"gemini":      initGemini,
		"qoder":       initQoder,
		"pi":          initPi,
		"zcode":       initZcode,
		"trae":        initTrae,
		"omp":         initOmp,
		"snow":        initSnow,
		"opencode":    initOpencode,
		"dsh":         initDsh,
		"reasonix":    initReasonix,
		"grok":        initGrok,
		"kimi":        initKimi,
		"kilo":        initKilo,
		"antigravity": initAntigravity,
		"devin":       initDevin,
		"codebuddy":   initCodebuddy,
		"copilot":     initCopilot,
		"droid":       initDroid,
	}
}

// stdinIsTTY 判断标准输入是否为交互终端（用 term.IsTerminal 做 ioctl 判断，
// /dev/null 虽是字符设备但不是终端，不能只看 ModeCharDevice）。
func stdinIsTTY() bool {
	return term.IsTerminal(int(os.Stdin.Fd()))
}

// resolvePlatformSelection 按 Trellis init.ts 的语义确定平台选择与 statusline：
//
//   - 显式传了任一平台 flag → 恰好安装这些平台；
//   - 无 flag 且 -y → 默认选择（Claude Code + Cursor）；
//   - 无 flag 且交互 → survey 多选，默认勾选 Claude Code 与 Cursor；
//   - stdin 不是 TTY 且无 -y、无 flag → 回退到默认选择并打印中文说明。
//
// statusline：仅 Claude Code 入选时有效；已传 --with-statusline 或 -y 时不追问，
// 否则在交互终端上用 survey Confirm 追问（默认不安装）。
func resolvePlatformSelection(cmd *cobra.Command) (selected []string, withStatusline bool, err error) {
	// --windsurf 是 --devin 的废弃别名：收到时自动转 --devin
	// 并打印中文警告（与 --devin 同时给时只转一次、只警告一次）。
	flags := explicitPlatformFlags()
	if initWindsurf {
		fmt.Println("警告：--windsurf 已废弃（Windsurf 已更名为 Devin），已自动转为 --devin，请改用 --devin。")
		flags["devin"] = true
	}
	in := platformSelectionInput{
		explicit:            explicitPlatformIDs(flags),
		yes:                 initYes,
		withStatusline:      initWithStatusline,
		withStatuslineGiven: cmd.Flags().Changed("with-statusline"),
		isTTY:               stdinIsTTY(),
	}
	return resolveSelection(in)
}

// resolveSelection 是平台选择的核心逻辑（不依赖 cobra，便于测试）。
func resolveSelection(in platformSelectionInput) ([]string, bool, error) {
	var selected []string
	var err error
	switch {
	case len(in.explicit) > 0:
		selected = in.explicit
	case in.yes:
		selected = platforms.DefaultSelected()
	case !in.isTTY:
		fmt.Println("未检测到交互终端，使用默认平台选择：Claude Code、Cursor。")
		selected = platforms.DefaultSelected()
	default:
		selected, err = promptPlatforms(platforms.DefaultSelected())
		if err != nil {
			return nil, false, fmt.Errorf("选择平台失败：%w", err)
		}
	}

	// --with-statusline 仅 Claude Code 入选时有效。
	hasClaude := false
	for _, id := range selected {
		if id == "claude-code" {
			hasClaude = true
		}
	}
	var withStatusline bool
	switch {
	case !hasClaude:
		if in.withStatuslineGiven && in.withStatusline {
			fmt.Println("提示：--with-statusline 仅在安装 Claude Code 时有效，已忽略。")
		}
		withStatusline = false
	case in.withStatuslineGiven:
		withStatusline = in.withStatusline
	case in.yes || !in.isTTY:
		withStatusline = false
	default:
		answer, err := promptStatusline()
		if err != nil {
			return nil, false, fmt.Errorf("确认状态栏安装失败：%w", err)
		}
		withStatusline = answer
	}
	return selected, withStatusline, nil
}

// platformInstalled 按平台配置目录是否存在判定该平台是否已安装。
// 配置目录取自 internal/platforms 注册表的 ConfigDir 字段（阶段 4 合并：
// 原 platformConfigDirs 映射已删除，唯一来源是注册表）。
func platformInstalled(cwd, platformID string) bool {
	for _, p := range platforms.Registry() {
		if p.ID == platformID {
			return dirExists(filepath.Join(cwd, p.ConfigDir))
		}
	}
	return false
}

// filterUninstalled 把 selected 分成未安装（待安装）与已安装（跳过）两组。
func filterUninstalled(cwd string, selected []string) (toInstall, skipped []string) {
	for _, id := range selected {
		if platformInstalled(cwd, id) {
			skipped = append(skipped, id)
			continue
		}
		toInstall = append(toInstall, id)
	}
	return toInstall, skipped
}

// installPlatforms 逐个安装平台，把写出的文件并入模板哈希清单。
// withStatusline 只对 Claude Code 生效；force 为真时覆盖已有平台文件。
func installPlatforms(cwd string, ids []string, withStatusline bool, force bool) ([]string, error) {
	var installed []string
	var allFiles []string
	for _, id := range ids {
		if _, ok := platforms.ByID(id); !ok {
			return installed, fmt.Errorf("未知平台：%s", id)
		}
		fmt.Printf("正在配置 %s……\n", platformDisplayNames[id])
		files, err := platforms.Install(cwd, id, platforms.InstallOptions{
			WithStatusline: withStatusline && id == "claude-code",
			Force:          force,
		})
		if err != nil {
			return installed, fmt.Errorf("安装 %s 失败：%w", platformDisplayNames[id], err)
		}
		installed = append(installed, id)
		allFiles = append(allFiles, files...)
	}
	if len(allFiles) > 0 {
		if err := scaffold.RecordHashes(cwd, allFiles); err != nil {
			return installed, err
		}
	}
	return installed, nil
}

// printPlatformSummary 打印平台安装小结；新装 Codex 时打印 hooks 启用提示。
func printPlatformSummary(installed []string, withStatusline bool) {
	if len(installed) == 0 {
		return
	}
	fmt.Println("已安装平台：")
	for _, id := range installed {
		fmt.Printf("  - %s\n", platformDisplayNames[id])
		if id == "claude-code" && withStatusline {
			fmt.Println("    ↳ 已安装 Claude Code 状态栏（--with-statusline）")
		}
	}
	for _, id := range installed {
		if id == "codex" {
			fmt.Println(platforms.CodexHooksHint())
		}
		if id == "zcode" {
			fmt.Println(platforms.ZcodeSetupHint())
		}
	}
}
