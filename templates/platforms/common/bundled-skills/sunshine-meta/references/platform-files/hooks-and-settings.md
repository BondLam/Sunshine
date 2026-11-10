# Hook 与设置

Hook/设置是把平台连接到 Sunshine 的入口层。它们决定平台为哪些事件运行哪些 hook 子命令。

> Go 版目前已接 22 个平台（Claude Code、Cursor、Codex、Kiro Code、Gemini CLI、Qoder、Pi Agent、ZCode、Trae、Oh My Pi、Snow CLI、OpenCode、DeepSeek Harness、Reasonix、Grok Build、Kimi Code、Kilo CLI、Antigravity、Devin、CodeBuddy、GitHub Copilot、Factory Droid）；下表三家平台的设置文件。Go 版的 hook 行为全部在二进制内（`sunshine hook <事件>`），项目里没有 hook 脚本文件。

## 设置的职责

设置/配置文件通常注册：

- session-start hook：在新会话开始或上下文重置时注入 Sunshine 总览。
- workflow-state hook：解析 `.sunshine/workflow.md` 中的 `[workflow-state:STATUS]` 块，并在每次用户输入时发出与当前任务 `status` 匹配的正文。只是解析器；脚本不内嵌后备内容。
- sub-agent context hook：在实现/检查/研究 agent 启动时注入任务上下文。
- shell/session 桥：让 shell 命令看到相同的 Sunshine 会话身份。
- 平台插件或扩展入口点。

常见文件：

| 平台 | 设置/配置 |
| --- | --- |
| Claude Code | `.claude/settings.json` |
| Cursor | `.cursor/hooks.json` |
| Codex | `.codex/hooks.json`、`.codex/config.toml` |
| OpenCode | `.opencode/package.json`、`.opencode/plugins/*` |
| Kiro | `.kiro/hooks/` + 平台配置 |
| Gemini CLI | `.gemini/settings.json` |
| Qoder | `.qoder/settings.json` |
| CodeBuddy | `.codebuddy/settings.json` |
| GitHub Copilot | `.github/copilot/hooks.json` |
| Factory Droid | `.factory/settings.json` |
| Pi Agent | `.pi/settings.json`、`.pi/extensions/sunshine/` |
| Trae IDE | `.trae/hooks.json` |

Reasonix 是拉取式平台，其 agent 文件包含启动后读取上下文的前奏说明。ZCode 使用 `.zcode/config.json` 及共享 hook，包括用于子 Agent 提示词注入的 PreToolUse。Kimi Code 同样是拉取式，没有 Sunshine 写入的项目级设置/hook 文件（hook 只存在于用户级 `~/.kimi-code/config.toml`），所以它的 agent 提示词作为 skill 和带相同前奏的 `.kimi-code/agents/` 子 Agent 定义发布。

这些文件是否存在于项目中，取决于用户运行过哪些 `sunshine init --<platform>` flag。

## Hook 子命令

| 子命令 | 用途 |
| --- | --- |
| `sunshine hook session-start` | 生成会话开始上下文。 |
| `sunshine hook workflow-state` | 解析 `.sunshine/workflow.md` 中的 `[workflow-state:STATUS]` 块，发出与当前任务状态匹配的正文。当没有匹配块时回退到 `Refer to workflow.md for current step.`。 |
| `sunshine hook subagent-context` | 向子 Agent 注入 PRD、JSONL 上下文及相关规范/研究。 |
| `sunshine hook shell-session` | 让 shell 命令继承 Sunshine 会话身份。 |
| `sunshine hook statusline` | 状态栏信息（仅 Claude Code，`--with-statusline` 时安装）。 |

不是每个平台都有每个 hook。不要因为某个平台缺少 hook，就从另一个平台复制注册；先确认该平台是否支持对应的事件。

## 本地修改场景

| 用户需求 | 编辑位置 |
| --- | --- |
| AI 在新会话中应当看到更多/更少上下文 | 平台的 `session-start` hook。 |
| 每轮提示策略应当改变 | `.sunshine/workflow.md` 中的 `[workflow-state:STATUS]` 块。hook 按原文解析 workflow.md——无需编辑脚本。 |
| 子 Agent 读不到 PRD/规范 | `sunshine hook subagent-context` 或 agent 前奏。 |
| shell 里的 `sunshine task current` 没有当前任务 | shell/会话桥 hook 或平台环境变量配置。 |
| 禁用某次自动注入 | 设置/配置中对应的 hook 注册。 |

## 修改原则

1. **设置负责接线；hook 子命令定义行为**。如果只改 hook，平台可能永远不会调用它。如果只改设置，行为可能不会变。
2. **先确认平台事件名**。不同平台对 SessionStart、UserPromptSubmit、AgentSpawn、shell 执行及类似事件使用不同的名字。
3. **Hook 读取本地 `.sunshine/`，而不是上游源码**。用户项目中的 `.sunshine/workflow.md` 与任务文件是默认目标。
4. **错误必须可见**。Hook 失败时静默退出 0、绝不阻断会话；排查时手工运行 `sunshine hook <事件>`，而不是等它在会话里报错。

## 排查路径

如果用户说“AI 没读 Sunshine 状态”：

1. 检查平台设置是否注册了该 hook。
2. 检查注册的命令是不是 `sunshine hook <事件>`，且 `sunshine` 在 PATH 里。
3. 手工运行 `sunshine hook <事件>`（用模拟的 stdin JSON）或 `sunshine task current --source` 命令。
4. 检查 `.sunshine/.runtime/sessions/` 中是否存在当前任务状态。
5. 检查平台 shell 是否传递会话身份。
