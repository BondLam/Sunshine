# 平台文件地图

本页按平台列出用户项目中常见的 Sunshine 文件位置。实际项目中是否存在某个平台目录，取决于用户运行过哪些 `sunshine init --<platform>` 命令。

> Go 版现状：已接 22 个平台（状态列标“已接”）。

## 矩阵

| 平台 | CLI flag | 主目录 | Skill 目录 | Agent 目录 | Hook/扩展 | 状态 |
| --- | --- | --- | --- | --- | --- | --- |
| Claude Code | `--claude` | `.claude/` | `.claude/skills/` | `.claude/agents/` | `.claude/settings.json`（注册 `sunshine hook` 调用） | 已接 |
| Cursor | `--cursor` | `.cursor/` | `.cursor/skills/` | `.cursor/agents/` | `.cursor/hooks.json`（注册 `sunshine hook` 调用） | 已接 |
| OpenCode | `--opencode` | `.opencode/` | `.opencode/skills/` | `.opencode/agents/` | `.opencode/plugins/` | 已接 |
| Codex | `--codex` | `.codex/` | `.agents/skills/` | `.codex/agents/` | `.codex/hooks.json`（注册 `sunshine hook` 调用） | 已接 |
| Kilo | `--kilo` | `.kilocode/` | `.kilocode/skills/` | 通常无 | `.kilocode/workflows/` | 已接 |
| Kiro | `--kiro` | `.kiro/` | `.kiro/skills/` | `.kiro/agents/` | `.kiro/hooks/` | 已接 |
| Gemini CLI | `--gemini` | `.gemini/` | `.agents/skills/` | `.gemini/agents/` | `.gemini/settings.json` | 已接 |
| Antigravity | `--antigravity` | `.agent/` | `.agent/skills/` | 通常无 | `.agent/workflows/` | 已接 |
| Devin | `--devin` | `.devin/` | `.devin/skills/` | 通常无 | `.devin/workflows/` | 已接 |
| Qoder | `--qoder` | `.qoder/` | `.qoder/skills/` | `.qoder/agents/` | `.qoder/settings.json` | 已接 |
| CodeBuddy | `--codebuddy` | `.codebuddy/` | `.codebuddy/skills/` | `.codebuddy/agents/` | `.codebuddy/settings.json` | 已接 |
| GitHub Copilot | `--copilot` | `.github/` | `.github/skills/` | `.github/agents/` | `.github/copilot/hooks.json` + `.github/hooks/sunshine.json` | 已接 |
| Factory Droid | `--droid` | `.factory/` | `.factory/skills/` | `.factory/droids/` | `.factory/settings.json` | 已接 |
| DeepSeek Harness | `--dsh` | `.dsh/` | `.agents/skills/`（共享）+ `.dsh/skills/`（入口 skill） | 无（工作流 skill 内联运行 implement/check） | 无（class-2 拉取式；无项目 hook/设置） | 已接 |
| Pi Agent | `--pi` | `.pi/` | `.agents/skills/` | `.pi/agents/` | `.pi/extensions/sunshine/`（原生 `sunshine_subagent` 工具）+ `.pi/settings.json` | 已接 |
| Trae IDE | `--trae` | `.trae/` | `.trae/skills/` | `.trae/agents/` | `.trae/hooks.json` | 已接 |
| Reasonix | `--reasonix` | `.reasonix/` | `.reasonix/skills/` | 无——子 Agent 是带 `runAs: subagent` frontmatter 的 skill | 无 | 已接 |
| ZCode | `--zcode` | `.zcode/` | `.zcode/skills/` | `.zcode/agents/` | `.zcode/hooks/` + `.zcode/config.json`（SessionStart + UserPromptSubmit + PreToolUse Agent/Task）；子 Agent 使用 hook 注入的上下文 | 已接 |
| Grok Build | `--grok` | `.grok/` | `.grok/skills/` | `.grok/agents/` | 拉取式前奏（无 hook；扁平的 `.grok/commands/sunshine-*.md`） | 已接 |
| Kimi Code | `--kimi` | `.kimi-code/` | `.agents/skills/`（共享）+ `.kimi-code/skills/` | `.kimi-code/agents/`（自定义子 Agent；相同的提示词也作为 skill 发布） | 无（拉取式前奏；无项目 hook/设置） | 已接 |
| Snow CLI | `--snow` | `.snow/` | `.snow/skills/` | `.snow/agents/`（自动发现；主要路径） | class-1：自动注入 + 项目 agent + `beforeSubAgentStart`（`.snow/hooks/` 的 `session`/`user`/`subagent` 模式 -> `additionalContext` JSON）；无旧版子 Agent JSON；命令 `.snow/commands/sunshine-*.json` | 已接 |

## 能力分组

### Sunshine 子 Agent 支持

这些平台通常有 `sunshine-research`、`sunshine-implement` 和 `sunshine-check` 文件：

- Claude Code
- Cursor
- OpenCode
- Codex
- Kiro
- Gemini CLI
- Qoder
- CodeBuddy
- GitHub Copilot
- Factory Droid
- Pi Agent
- Trae IDE
- Reasonix（作为带 `runAs: subagent` 的 skill 在 `.reasonix/skills/` 下交付，而不是独立的 `agents/` 目录）
- ZCode
- Grok Build（`.grok/agents/`；通过 `spawn_subagent` 分发并指定 `subagent_type`）
- Kimi Code（`.kimi-code/agents/`；相同的提示词也作为 skill 在 `.kimi-code/skills/` 下交付）
- Snow CLI（`.snow/agents/`；自动发现的项目 agent + class-1 hook）

修改实现/检查/研究行为时，先找对应的平台 agent 文件。

### 原生 Sunshine 子 Agent 工具

有些平台暴露宿主运行时理解的一等工具。模型像调用任何其他工具一样调用它，宿主渲染进度卡片、对照 `.<platform>/agents/` 校验 agent 名，并强制执行分发模式。

- Pi Agent——`sunshine_subagent` 工具，定义在 `.pi/extensions/sunshine/index.ts`。支持 `single` / `parallel` / `chain` 分发模式，并发出实时的 `sunshine-subagent-progress` 事件。Pi 已接。

修改这些平台上的子 Agent 分发行为时，编辑扩展文件，**而不是** agent markdown——agent markdown 定义职责，但宿主扩展拥有分发、校验和进度渲染。

### 主会话工作流平台

这些平台更多依靠工作流/skill 引导主会话：

- Kilo
- Antigravity
- Devin

修改行为时，先检查工作流和 skill。不要假设存在 Sunshine 子 Agent。

### 共享 `.agents/skills/`

Codex、Gemini CLI、Pi Agent、Kimi Code 和 DeepSeek Harness 写入共享的 `.agents/skills/` 层。一些支持 agentskills.io 的工具也可以读取该目录。如果用户希望多个兼容工具共享一个 skill，优先考虑 `.agents/skills/`，但不要假设每个平台都读取它。ZCode 把 Sunshine 管理的 skill 保留在 `.zcode/skills/` 下。

## 修改平台文件时的决策规则

1. 用户指定了平台：只修改该平台目录，除非共享工作流/规范文件也必须改。
2. 用户说“所有平台都应当这样”：逐个平台同步等价入口点；不要只改一个目录。
3. 用户只说“我的 AI”：检查项目中实际存在的配置目录，推断当前 AI 平台。
4. 用户想要项目规则：优先使用 `.sunshine/spec/` 或项目本地 skill。
5. 用户想要 Sunshine 行为：编辑 `.sunshine/workflow.md` 加平台 hook/agent/skill/命令。

## 路径不一致时

平台生态在变化，用户项目可能已有定制。如果本表与本地文件不一致，以用户项目中实际的设置/配置为准：

- 检查设置注册的 hook。
- 检查命令/提示词/工作流指向的 `sunshine` 子命令。
- 按 agent 文件中当前写明的读取规则判断行为。

不要因为某个自定义文件不在本路径表中就删除它。

### `.omp/`——Oh My Pi (OMP)

由扩展支撑的平台。OMP 原生 provider 自动发现所有子目录。

```text
.omp/
├── commands/          # Slash 命令（扁平 .md）
├── skills/            # 自动触发的 skill（每目录一个 SKILL.md）
├── agents/            # Agent 定义（.md）
└── extensions/
    └── sunshine/
        └── index.ts   # Sunshine 扩展（上下文注入）
```

无 `settings.json`——OMP 自动扫描 `.omp/` 子目录。
无项目 hook——与 hook 等价的行为由 `sunshine hook` 子命令实现（Go 二进制内，无需 Python 运行时）；OMP 已接，见本页开头的平台状态。
