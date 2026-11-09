# Skill、命令、提示词与工作流

Skill 和命令是用户与 Sunshine 交互的文本入口点。不同平台使用不同的名字，但核心目的相同：当用户表达某种意图时，告诉 AI 如何进入 Sunshine 流程。

## 概念区别

| 类型 | 触发方式 | 最适合 |
| --- | --- | --- |
| skill | AI 自动匹配或用户显式提及 | 长期能力、工作流规则、修改指南。 |
| command | 用户显式调用 | 清晰的操作入口点，例如 continue 和 finish-work。 |
| prompt | 用户显式调用或平台选择 | 类似命令，但采用平台提示词格式。 |
| workflow | 用户显式选择或平台自动匹配 | 在没有子 Agent/hook 时引导主会话。 |

Sunshine 工作流 skill 通常共享同一套语义集合：brainstorm、before-dev、check、update-spec、break-loop。`sunshine-meta` 这类多文件内置 skill 使用分层 references。

## 常见路径

> Go 版目前只接 Claude Code、Cursor、Codex 三家；下表其余平台的入口是目标形态，待滚动轨道扩展。

| 平台 | 常见入口 |
| --- | --- |
| Claude Code | `.claude/skills/`、`.claude/commands/` |
| Cursor | `.cursor/skills/`、`.cursor/commands/` |
| OpenCode | `.opencode/skills/`、`.opencode/commands/` |
| Codex | `.agents/skills/`、`.codex/skills/` |
| Kilo | `.kilocode/skills/`、`.kilocode/workflows/` |
| Kiro | `.kiro/skills/` |
| Gemini CLI | `.agents/skills/`、`.gemini/commands/` |
| Antigravity | `.agent/skills/`、`.agent/workflows/` |
| Devin | `.devin/skills/`、`.devin/workflows/` |
| Qoder | `.qoder/skills/`、`.qoder/commands/` |
| CodeBuddy | `.codebuddy/skills/`、`.codebuddy/commands/` |
| GitHub Copilot | `.github/skills/`、`.github/prompts/` |
| Factory Droid | `.factory/skills/`、`.factory/commands/` |
| Pi Agent | `.agents/skills/` |
| Reasonix | `.reasonix/skills/` |
| ZCode | `.zcode/skills/`、`.zcode/commands/` |
| Kimi Code | `.agents/skills/`、`.kimi-code/skills/`（命令作为 `/skill:sunshine-*` skill 交付） |

在用户项目中，以 init 实际生成的文件为准。

## Skill 结构

常见的 skill 是一个目录：

```text
sunshine-meta/
├── SKILL.md
└── references/
```

`SKILL.md` 应当告诉 AI：

- 何时使用本 skill。
- 当前任务先读哪个参考。
- 什么不要做。

references 承载更长的说明，使入口文件不必包含一切。

## 命令/提示词/工作流结构

命令、提示词和工作流通常是单个文件。其内容应当包括：

- 何时使用它。
- 读哪些 `.sunshine/` 文件。
- 运行哪些 `sunshine` 子命令。
- 完成后如何汇报。

它们不应存储任务状态；任务状态属于 `.sunshine/tasks/` 和 `.sunshine/.runtime/`。

## 本地修改场景

| 用户需求 | 编辑位置 |
| --- | --- |
| 修改 AI 自动触发规则 | 对应 skill 的 frontmatter description。 |
| 修改用户命令行为 | 对应的命令/提示词/工作流文件。 |
| 添加项目本地 skill | 平台 skill 目录，或共享的 `.agents/skills/`。 |
| 让多个平台共享同一能力 | 在每个平台 skill 目录写等价 skill，或在支持的平台上使用 `.agents/skills/` 共享层。 |
| 修改 finish/continue 入口点 | 平台命令/提示词/工作流。 |

## 修改原则

1. **入口文件保持简短；长内容放 references**。这对 `sunshine-meta` 这类多文件 skill 尤其重要。
2. **触发描述要具体**。太宽泛的描述可能误触发；太窄的可能不触发。
3. **跨平台保持相同语义一致**。文件格式可以不同，但行为描述应当匹配。
4. **把项目专属能力放进本地 skill**。不要把团队私有的流程放进公开的 `sunshine-meta`。

如果用户只想让本地 AI 多知道一条项目规则，通常是创建一个项目本地 skill 或更新 `.sunshine/spec/`，而不是修改 Sunshine 内置的工作流 skill。
