# Sunshine on Snow CLI

Snow 是一类 Sunshine 宿主：自动注入上下文 + 项目 agent 自动发现 +
`beforeSubAgentStart` 提示增强。

| 能力 | 状态 |
| ------------------------------------------------- | ---------------------------------------------------------------------------- |
| Skills（`.snow/skills/sunshine-*/SKILL.md`） | 可用 |
| 提示命令（`.snow/commands/sunshine-*.json`） | 可用（`/sunshine-continue`、`/sunshine-finish-work`、……） |
| 上下文 hook（`.snow/hooks/`） | 通过 stdout JSON 注入模型上下文 |
| 项目 agent（`.snow/agents/*.md`） | Snow 自动发现（`#sunshine-implement`、……） |
| `beforeSubAgentStart` | 向子代理提示注入当前任务面包屑 |
| `sunshine-start` | 可选——会话 hook 已替代旧的手动流程 |

## 快速上手

```bash
sunshine init --snow -u your-name
snow
```

在 Snow 里：

1. 在项目根目录打开会话——`onSessionStart` 自动注入 Sunshine 上下文。
2. 派发 implement/check/research（`.snow/agents/` 下的项目 agent）。提示首行优先写：

```text
Active task: .sunshine/tasks/<id>
```

3. 可选：`/sunshine-continue` / `/sunshine-finish-work`，或对 `sunshine-*` skill 用 `skill-execute`。
4. 调试注入：设置 `SNOW_DEBUG_HOOKS=1`，检查 `.snow/log/hooks-inject.txt`。

## Agent

Snow 从 `.snow/agents/**/*.md` 加载项目 agent（优先于 `~/.snow/sub-agents.json`）。
主要路径是项目 agent 自动发现——不需要手动合并。不用旧版子代理 JSON 合并文件。

上下文加载是一类 hook 注入（`beforeSubAgentStart` / 会话 / 用户）。Agent **不带**二类 pull 前置段。Hook 已经注入上下文；agent 仍会为求准确重读任务产物。

## 工具名（Snow 原生）

- `filesystem-read` / `filesystem-create` / `filesystem-replaceedit` / `filesystem-edit`
- `terminal-execute`
- `ace-search` / `codebase-search`
- `todo-manage` / `notebook-manage`
- `skill-execute`
- `websearch-search` / `websearch-fetch`（research）
- `ide-get_diagnostics`

## Hook 协议

会话 / 用户 / 子代理 hook 输出：

```json
{ "additionalContext": "...", "display": "..." }
```

- 退出 0 + JSON → 注入（前置）；界面气泡保留用户原输入
- `onUserMessage` 退出 1 → 替换（Sunshine 不用）
- 非 JSON stdout → 忽略

Hook 模式（同一命令，不同深度）：

| Hook | 对应 Sunshine 命令 | 内容 |
| --------------------- | ---------- | ---------------------------------------------------------------- |
| `onSessionStart`      | `sunshine hook session-start` | 完整：当前任务、任务产物、prd 摘要、工作流 |
| `onUserMessage`       | `sunshine hook workflow-state` | 精简面包屑：当前任务与产物概况 |
| `beforeSubAgentStart` | `sunshine hook subagent-context` | 完整 + 按 agent 类型裁剪（implement/check/research） |

## 会话身份（多会话）

Snow 向 hook 命令、`terminal-execute`、bash 模式与子代理子进程注入这些环境变量：

| 变量 | 示例 | 用途 |
| ------------------- | ------------------- | ----------------------------------------- |
| `SNOW_SESSION_ID`    | `c2343752-...`      | Snow 原生会话 uuid |
| `SUNSHINE_CONTEXT_ID` | `snow-c2343752-...` | 首选的 Sunshine 当前任务上下文键 |
| `SNOW_CWD`           | 项目根目录 | Hook / 工具的工作目录 |
| `SNOW_PLATFORM`      | `snow`              | 平台标记 |

说明：

- 已设置 `SUNSHINE_CONTEXT_ID` 时优先使用（显式覆盖）。
- 否则 Sunshine 用 `SNOW_SESSION_ID` 解析当前任务（平台 `snow`）。
- Hook stdin 里也可能同时带有 `sessionId` / `session_id` 两个键。
