# 平台文件总览

Sunshine 把同一套本地架构连接到不同的 AI 工具。`.sunshine/` 存储共享运行时；平台目录存储 adapter 文件，定义每个 AI 工具如何进入 Sunshine。

> Go 版目前已接 22 个平台（Claude Code、Cursor、Codex、Kiro Code、Gemini CLI、Qoder、Pi Agent、ZCode、Trae、Oh My Pi、Snow CLI、OpenCode、DeepSeek Harness、Reasonix、Grok Build、Kimi Code、Kilo CLI、Antigravity、Devin、CodeBuddy、GitHub Copilot、Factory Droid）；下文三家平台的目录。

当本地 AI 修改 Sunshine 时，应当先区分两类文件：

- **共享文件**：`.sunshine/workflow.md`、`.sunshine/config.yaml`、`.sunshine/tasks/`、`.sunshine/spec/`、`.sunshine/agents/`。
- **平台文件**：19 个已接平台的目录（见 `platform-map.md`）。

平台文件不存储业务状态。它们让对应的 AI 工具读取 Sunshine 状态、调用 `sunshine` 子命令、加载 Sunshine skill/agent/hook。

## 平台文件类别

| 类别 | 常见路径（已接平台） | 用途 |
| --- | --- | --- |
| settings/config | `.claude/settings.json`、`.codex/hooks.json`、`.cursor/hooks.json` | 注册 hook 或平台行为（调 `sunshine hook <事件>`）。 |
| hooks | 平台设置里注册的 `sunshine hook` 调用 | 在会话开始、用户输入、agent 启动、shell 执行及类似事件时注入上下文。 |
| agents | `.claude/agents/`、`.cursor/agents/`、`.codex/agents/` | 定义 `sunshine-research`、`sunshine-implement` 和 `sunshine-check`。 |
| skills | `.claude/skills/`、`.cursor/skills/`、`.agents/skills/`（Codex 共用） | 自动触发或可按需阅读的能力描述。 |
| commands/prompts/workflows | `.claude/commands/sunshine/`、`.cursor/commands/sunshine-*.md`、`.agents/skills/sunshine-*/SKILL.md`（Codex 的命令转 skill） | 用户显式调用的入口点。 |

## 三种平台集成模式

### 1. Hook 驱动

这些平台可以在特定事件上触发 `sunshine hook` 子命令，并主动向 AI 注入 Sunshine 上下文。

常见能力：

- 会话开始时注入 `.sunshine/` 总览。
- 每次用户轮次的 workflow-state 提示。
- 子 Agent 启动时的 PRD/规范/研究注入。
- 继承会话身份的 shell 命令。

要改变“AI 何时知道什么”，先检查 hook 注册和设置。

### 2. Agent 前奏 / 拉取式

有些平台无法可靠地让 hook 改写子 Agent 提示词，所以 agent 文件本身指示 agent 在启动后读取当前任务、PRD 和 JSONL 上下文。

要改变子 Agent 加载上下文的方式，检查 agent 文件本身。

### 3. 主会话工作流

有些平台没有 Sunshine 子 Agent 或 hook 能力。它们依靠工作流/skill/命令引导主会话 AI 读文件、运行 `sunshine` 子命令、推进任务。

要改变行为，检查平台工作流/skill/命令和 `.sunshine/workflow.md`。

## 本地修改顺序

当用户要求为某个平台定制行为时，AI 应当按以下顺序检查文件：

1. 读 `.sunshine/workflow.md` 确认共享流程。
2. 读目标平台的设置/配置，看注册了哪些 hook/agent/skill/命令。
3. 读目标平台的 agent/skill/命令/hook。
4. 修改最接近用户需求的本地文件。
5. 如果改动影响共享流程，同步 `.sunshine/workflow.md` 或 `.sunshine/spec/`。

不要只改平台文件而忘记共享工作流。不要只改 `.sunshine/workflow.md` 而忘记平台入口可能仍含有旧描述。
