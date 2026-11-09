# Hook 实现笔记

## Codex 自有的 session-start.py：只记录，不接线

来源：`packages/cli/src/templates/codex/hooks/session-start.py`（552 行）。

### 它做什么

与共享 `session-start.py`（949 行）是**同构的重复实现**，不是复用：

- 同样的输出骨架：`<session-context>` → first-reply notice → `<current-state>` → `<trellis-workflow>`（Phase Index 摘要）→ `<guidelines>`（规范索引）→ `<task-status>` → `<ready>`。
- 同样的取数逻辑：开发者身份、git 状态、当前任务、任务产物清单、规范索引收集。
- 输出信封是 Codex 专用的：多 `suppressOutput: true` 与 `systemMessage: "Trellis context injected (N chars)"` 两个顶层字段，`hookSpecificOutput.hookEventName` 仍为 `SessionStart`。
- 启动时调 `configure_project_encoding()`（复用 `.trellis/scripts/common` 的 Windows 编码助手），共享版没有这一步。
- `should_skip_injection()` 只看 `TRELLIS_HOOKS` / `TRELLIS_DISABLE_HOOKS` / `CODEX_NON_INTERACTIVE`，没有共享版那 10 个平台的 `*_NON_INTERACTIVE` 列表。
- 项目目录只从 hook JSON 的 `cwd` 取，没有共享版那张 10 项环境变量表（`CLAUDE_PROJECT_DIR` 等）。

### 与共享 session-start.py 的差异（逐项）

| 方面 | 共享版 | Codex 版 |
| --- | --- | --- |
| 更新提示 | 有（`get_update_hint`，会话内一次，`trellis --version` 比对 `.trellis/.version`，marker 节流） | 无，`FIRST_REPLY_NOTICE` 是不带更新行的纯常量 |
| Legacy 规范结构告警 | 有（monorepo 下 `spec/backend|frontend` 扁平结构告警） | 无 |
| 规范作用域过滤 | 有（`session.spec_scope` + 任务包名过滤） | 无，全量收集 |
| 任务状态细节 | 区分 planning 缺 prd / 缺 design+implement / jsonl 未整理；completed 提示 `/trellis:finish-work` | 较粗：无 prd 即 PLANNING；planning 有 design+implement 才提示评审；completed 提示 `task.py archive <dir>` 或开新任务 |
| `stale` 提示 | `python3 ./.trellis/scripts/task.py finish` | 同命令（无 `python3` 前缀差异外基本一致） |
| 输出信封 | `hookSpecificOutput` + 顶层 `additional_context`（ZCode 除外） | 再加 `suppressOutput: true`、`systemMessage` |
| 平台检测 | 环境变量表 + argv[0] 目录名 | 固定 `platform="codex"` 传给 active task 解析器 |

### 为什么不接线

- `.codex/hooks.json` 只注册了两条：`UserPromptSubmit → inject-workflow-state.py`、`SubagentStart`（matcher `^(?:trellis-implement|trellis-check|trellis-research)$`）`→ inject-subagent-context.py`。没有 `SessionStart` 条目。
- `ai-tools.ts` 里 Codex 的 `hasHooks=false`：Codex 拿不到完整的 SessionStart 概览，改由每轮 `workflow-state` 面包屑 + 无任务时的 bootstrap 提示（`read the trellis-start skill once`）补位。
- PLAN.md 阶段 2 已明确：hooks.json 未注册它，init 仍按 Trellis 原样处理，不擅自“修复”接线。本包因此**没有**实现 Codex 版 session-start，也不注册 `sunshine hook session-start` 的 Codex 变体。

### 后续阶段要注意什么

1. **如果将来要接线**，不要直接复用共享 `session-start` 的输出：Codex 需要 `suppressOutput` + `systemMessage` 信封，且不应带更新提示（Codex 版特意没有）；规范作用域与 legacy 告警是否补回需另定。
2. **平台识别**：Python 靠脚本路径里的 `.codex` 认出 Codex；Go 二进制路径没有这个信号。本包用 `SUNSHINE_HOOK_PLATFORM` 环境变量补位（见 `hook.go` 的 `DetectPlatform`），Codex 接线时应在 hook 命令前设置 `SUNSHINE_HOOK_PLATFORM=codex`，否则 `workflow-state` 的 `<codex-mode>` 横幅与 bootstrap 提示不会出现。
3. **Codex hooks 总开关**：用户须在 `~/.codex/config.toml` 设 `features.hooks = true`（Codex 0.129+；旧版 `codex_hooks = true`），并在 Codex 里运行一次 `/hooks` 审批，否则 `hooks.json` 不生效。
4. **子代理名**：Codex 的 SubagentStart matcher 匹配的是 `trellis-implement|trellis-check|trellis-research`；Go 版子代理名为 `sunshine-*`，接线方（阶段 2 平台组）改 matcher 时须同步，否则 `subagent-context` 的 Codex 分支收不到事件。本包同时接受 `trellis-*` 旧名作兼容。

## Shell 票据格式约定（留给阶段 3 的 `sunshine task` 命令）

`sunshine hook shell-session` 在待执行命令调用 `sunshine task start/current/finish`
（兼容旧写法 `task.py start/current/finish`）时写票据；阶段 3 的 task 命令在没有原生
会话环境时用票据恢复会话身份。

- **位置**：`.sunshine/.runtime/shell-tickets/<毫秒时间戳>-<sha256(context_key\0命令\0创建时间) 前 16 位 hex>.json`
- **TTL**：30 秒（`created_at_epoch` / `expires_at_epoch`，Unix 秒，浮点）。
- **文件格式**（JSON，2 空格缩进，UTF-8）：

```json
{
  "platform": "cursor",
  "context_key": "cursor_abc123",
  "conversation_id": "conv-1",
  "session_id": "sess-1",
  "generation_id": "gen-1",
  "cwd": "/path/to/repo",
  "host_cwd": "/path/to/repo",
  "command": "sunshine task start 11-09-demo",
  "subcommands": [
    { "name": "start", "task_ref": "11-09-demo" }
  ],
  "created_at_epoch": 1793000000.123,
  "expires_at_epoch": 1793000030.123
}
```

字段说明（与 Python 的 `inject-shell-session-context.py` 一致，仅路径与命令名换成 Sunshine）：

- `platform`：写票据的宿主平台名（`SUNSHINE_HOOK_PLATFORM` 或环境变量推断；未知时为空字符串）。**仅调试用，消费方不得按它过滤**（Python 曾因此只让 Cursor 的桥接生效）。
- `context_key`：解析出的会话键（如 `claude_<session_id>`），消费方凭它找 `.sunshine/.runtime/sessions/<context_key>.json`。
- `conversation_id` / `session_id` / `generation_id`：hook 输入顶层同名字段原样记录（可能为空），仅调试用。
- `cwd`：解析出的项目根目录（不是 hook 输入的 cwd——某些宿主对每个事件都发 `cwd: "/"`，那个值写进来会在消费方被收容检查丢弃）。
- `host_cwd`：hook 输入的 cwd 原样记录，仅调试用。
- `command`：待执行的完整命令。
- `subcommands`：命令里出现的所有任务子命令；`start` 带 `task_ref`（start 后紧跟的 token），`current` / `finish` 不带。
- 旧目录 `.sunshine/.runtime/cursor-shell/`（Trellis 0.6.13 前的 Cursor 专属名）只读不写：消费方查票据时应同时 glob 该目录，避免跨版本升级中途的命令丢失会话身份。

消费方（阶段 3）接受票据的条件（与 `active_task.py` 的 `_matching_ticket_context_key` 一致）：

1. 票据新鲜（`expires_at_epoch` 未过；无 `created_at_epoch` 时只看 `expires_at_epoch`，两者皆无视为新鲜）；
2. `cwd` 落在当前仓库内；
3. 正在执行的命令与票据 `subcommands` 匹配（`start` 还须 `task_ref` 匹配）；
4. **恰好只有一个**新鲜票据的 context_key 匹配——两个窗口并发时宁可都降级，也不让一方继承另一方的指针。
