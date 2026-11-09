# Agent

Sunshine agent 文件定义专门角色。用户项目中常见的 Sunshine agent 有：

- `sunshine-research`
- `sunshine-implement`
- `sunshine-check`

文件位置和格式因平台而异，但责任边界应当保持一致。

> Go 版目前只接 Claude Code、Cursor、Codex 三家；下表其余平台的路径是目标形态，待滚动轨道扩展。

## Agent 职责

| Agent | 职责 |
| --- | --- |
| `sunshine-research` | 调查问题，把发现写入当前任务的 `research/`。 |
| `sunshine-implement` | 按 `prd.md`、可选的 `design.md` / `implement.md`、`implement.jsonl` 及相关规范/研究实现。 |
| `sunshine-check` | 评审变更、修复发现的问题，并运行必要的检查。 |

Agent 文件不应变成通用聊天提示词。它们应当定义输入来源、写边界、是否可以改代码，以及如何汇报结果。

## 常见路径

| 平台 | Agent 路径 |
| --- | --- |
| Claude Code | `.claude/agents/sunshine-*.md` |
| Cursor | `.cursor/agents/sunshine-*.md` |
| OpenCode | `.opencode/agents/sunshine-*.md` |
| Codex | `.codex/agents/sunshine-*.toml` |
| Kiro | `.kiro/agents/sunshine-*.json` |
| Gemini CLI | `.gemini/agents/sunshine-*.md` |
| Qoder | `.qoder/agents/sunshine-*.md` |
| CodeBuddy | `.codebuddy/agents/sunshine-*.md` |
| Factory Droid | `.factory/droids/sunshine-*.md` |
| Pi Agent | `.pi/agents/sunshine-*.md` |
| Reasonix | `.reasonix/skills/sunshine-*/SKILL.md`（subagent frontmatter） |
| ZCode | `.zcode/agents/sunshine-*.md` |
| Kimi Code | `.kimi-code/agents/sunshine-*.md`（自定义子 Agent；相同的提示词也作为 `.kimi-code/skills/sunshine-*/SKILL.md` 发布） |

GitHub Copilot 的 agent/提示词支持由 `.github/agents/`、`.github/prompts/` 和 `.github/skills/` 等目录组合提供；检查用户项目中实际生成的文件。

Kilo、Antigravity 和 Devin 这类主会话工作流平台可能没有 Sunshine 子 Agent 文件。它们通常依靠工作流/skill 引导主会话。

## 两种上下文加载模式

### hook 推送

平台 hook 在 agent 启动前注入任务上下文。agent 文件本身可以更聚焦于职责和边界。

常见于支持 agent hook 的平台。

### agent 拉取

agent 文件指示 agent 在启动后读取：

- `sunshine task current --source`
- `implement.jsonl` 或 `check.jsonl`
- JSONL 引用的规范/研究文件
- 当前任务的 `prd.md`
- 如有 `design.md`
- 如有 `implement.md`

该模式适合 hook 无法可靠改写子 Agent 提示词的平台。

## 本地修改场景

| 用户需求 | 编辑位置 |
| --- | --- |
| implement agent 必须遵守额外限制 | 该平台的 `sunshine-implement` agent 文件。 |
| check agent 必须运行项目专属命令 | `sunshine-check` agent 文件，必要时加 `.sunshine/spec/`。 |
| research agent 必须输出固定格式 | `sunshine-research` agent 文件。 |
| Agent 读不到任务上下文 | Agent 前奏或 `inject-subagent-context` hook。 |
| 添加项目专属 agent | 平台 agent 目录 + 相关工作流/命令/skill 入口点。 |

## 修改原则

1. **保持职责单一**。不要把 research、implement、check 的职责混进一个 agent。
2. **明确阅读顺序**。Agent 必须知道从当前任务出发，读 jsonl/规范上下文，再读 `prd.md`、如有 `design.md`、如有 `implement.md`。
3. **明确写边界**。research 通常只写 `research/`；implement 可以写代码；check 可以修复问题。
4. **在多平台项目中保持语义同步**。如果用户同时配置了 Claude、Codex 和 Cursor，决定对一个平台 agent 的修改是否也需要应用到其他平台。

## 不要默认编辑上游模板

本地 AI 默认应当修改用户项目内的平台 agent 文件。只有当用户明确想把改动贡献回 Sunshine 时，才讨论上游模板源码。
