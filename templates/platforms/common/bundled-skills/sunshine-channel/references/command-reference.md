# 命令参考

`sunshine channel` 子命令的权威当前命令参考，已对照 `packages/cli/src/commands/channel/` 中的源码验证（`index.ts` 的 Commander 接线和每个子命令处理函数）。

除非另有说明，每个子命令都接受 `--scope <project|global>`；`project` 是默认值，按当前 cwd 所在的项目 bucket 解析。

## 顶层

```
sunshine channel <subcommand>
```

> 多 Agent 协作运行时——通过共享事件日志派生 / 协调 / 中断 worker agent。

---

## 创建 / 列出

### `create <name>`

```bash
sunshine channel create <name>
  [--scope project|global]                # 默认：project
  [--type chat|forum]                     # 默认：chat
  [--task <path>]                         # 关联的 Sunshine 任务目录
  [--project <slug>]
  [--labels a,b,c]
  [--description <text>]                  # 稳定的 channel 描述
  [--context-file <abs-path>] ...         # 可重复
  [--context-raw  <text>]      ...        # 可重复
  [--linked-context-file <abs-path>]      # [已废弃的别名]
  [--linked-context-raw  <text>]          # [已废弃的别名]
  [--cwd <path>]                          # 记录在 create 事件中
  [--by <agent>]                          # 默认：main
  [--force]                               # 覆盖已有的 channel
  [--ephemeral]                           # 在默认列表中隐藏，可清理
```

行为：
- 追加一个 `create` 事件；`type` 不可变（创建后无法在 forum↔chat 之间变更）。
- `--ephemeral` channel 默认在 `channel list` 中隐藏，并且是 `channel prune --ephemeral` 的清扫目标。
- `--linked-context-*` 会折叠进 `--context-*`；使用时会发出废弃提示。

### `list`

```bash
sunshine channel list
  [--scope project|global]
  [--json]
  [--project <slug>]                      # 对 task 字段做子串匹配
  [--all]                                 # 包含 ephemeral（后缀 '*'）
  [--all-projects]                        # 扫描每个项目 bucket
```

行为：
- 默认 scope：当前 cwd 所在的项目。`--all-projects` 扫描每个 bucket。
- 美化模式打印 `NAME WORKERS EVENTS LAST KIND TYPE TASK`，按最近程度排序，页脚注明隐藏的 ephemeral 数量。
- `--json` 切换为 JSON 数组。

---

## 聊天消息

### `send <name> [text]`

```bash
sunshine channel send <name> [text]
  --as <agent>                            # 必填——作者
  [--scope project|global]
  [--to <agents,csv>]                     # 默认：广播
  [--stdin | --text-file <path>]          # 正文来自 stdin 或文件
  [--delivery-mode appendOnly|requireKnownWorker|requireRunningWorker]
```

行为：
- 正文优先级：位置参数 `[text]` → `--stdin` → `--text-file`。
- `--to` 只有一个条目时存为字符串，多个存为数组；省略表示广播。
- `--delivery-mode` 选择定向投递校验：
  - `appendOnly`（接近默认——只记录），
  - `requireKnownWorker`（指名目标必须有过 `spawned` 事件），
  - `requireRunningWorker`（worker 当前必须存活）。
- 把追加的事件作为一行 JSON 打印到 stdout。

> **注意：** `send` **没有** `--tag` flag，也**没有** `--kind` flag。见下文 [`tag-vs-kind`](#tag-vs-kind事件形态实际是如何控制的)。

### `messages <name>`

```bash
sunshine channel messages <name>
  [--scope project|global]
  [--raw]                                 # 每行一个 JSON 事件
  [--follow]                              # 流式输出新事件
  [--last <N>]                            # 最后 N 个匹配事件
  [--since <seq>]                         # seq > N
  [--kind <kind>]                         # CHANNEL_EVENT_KINDS 之一
  [--from <csv>]                          # 作者过滤
  [--to <target>]                         # 路由目标过滤
  [--thread <key>]                        # 仅 forum
  [--action <thread-action>]              # 仅 forum
  [--no-progress]                         # 隐藏 progress 事件
```

行为：
- 自动检测 forum 频道：无过滤条件时渲染 thread 看板而不是事件流。`--thread` / `--action` 仅 forum 可用，对 chat 频道会报错。
- `--kind` 会对照 `CHANNEL_EVENT_KINDS` 校验（单个值，不是 CSV——CSV 是 `wait` 那边的用法）。

### `wait <name>`

```bash
sunshine channel wait <name>
  --as <agent>                            # 必填——用于过滤上下文的 self
  [--scope project|global]
  [--timeout <Ns|Nm|Nh|Nms>]              # 由 parseDuration 解析
  [--from <a,b>]                          # 作者 CSV
  [--kind <k1,k2>]                        # CSV，或语义
  [--thread <key>]                        # forum 过滤
  [--action <thread-action>]              # forum 过滤
  [--to <target>]                         # 默认：自己的 agent（广播 + 发给我的）
  [--include-progress]                    # 同时在 progress 事件到来时唤醒
  [--all]                                 # 要求每个 --from 都匹配
```

行为：
- 把匹配的事件作为 JSON 流式输出，每行一个。
- 默认的 `--to` 过滤器是调用者自己的 agent（广播事件仍然匹配——广播 + 明确发给我的）。
- `--all` 要求传入 `--from`，并阻塞直到每个列出的 agent 都产生了匹配事件。
- **超时退出 124**，并在 `--all` 起作用时向 stderr 打印 `timeout: still waiting on ...`。

---

## tag-vs-kind——事件形态实际是如何控制的

v0.6.0 的 channel CLI 中任何地方都**没有 `--tag` flag**；`--kind` 不是任何 `--tag` flag 的旧版别名。

当前源码中的具体模型：

- `--kind` 是唯一的事件类型过滤器，并且受限于 sunshine 发出的白名单（`packages/core/src/channel/internal/store/events.ts` 中的 `CHANNEL_EVENT_KINDS`）：
  - `create`、`join`、`leave`、`message`、`thread`、`context`、`channel`、`spawned`、`killed`、`respawned`、`progress`、`done`、`error`、`waiting`、`awake`、`undeliverable`、`interrupt_requested`、`turn_started`、`turn_finished`、`interrupted`、`supervisor_warning`
  - 传入其他任何值都会抛出 `Invalid --kind '<x>'. Must be one of: …`。
- `--kind` 位于 `wait`（CSV，或语义）和 `messages`（单个值）上。`send` 和 `run` 无法发出自定义 kind——每次 `send` 写入的都是 `message` 事件。
- 轮次中途的 worker 中止**不是**一个 tag。它是专用的 `channel interrupt` 命令，会追加一对 `interrupt_requested` / `interrupted` 事件，并在 provider 层面中断 worker。

分发者等待 worker 时的实用规则：

- 用 `--kind done,turn_finished` 表示“worker 完成了一轮”——这些是 supervisor 自动触发的系统事件。不要依赖 worker LLM 记得发出任何自定义信号。
- 只有在确实想要轮次中途中止行为时，才使用 `sunshine channel interrupt`（该命令）。
- **不要**把用户侧 tag 当作完成信号来发明。没有 `--tag` 过滤器；worker 把自定义字符串写进最终消息里，只是 `message` 事件里的一段文本，`wait` 无法匹配它。

长正文始终走 stdin 或文件：

```bash
sunshine channel send T --as A --stdin < /tmp/message.md
sunshine channel send T --as A --text-file /tmp/message.md
```

---

## 中断

### `interrupt <name> [text]`

```bash
sunshine channel interrupt <name> [text]
  --as <agent>                            # 必填——调用者
  --to <agent>                            # 必填——目标 worker
  [--scope project|global]
  [--stdin | --text-file <path>]
```

行为：
- 追加一个 `interrupt` 事件，`reason: "user"`，附带替换指令正文；supervisor 在支持的地方执行 provider 级中断（Claude 的 `/interrupt`、Codex 的轮次取消）。
- 把追加的事件 JSON 打印到 stdout。

---

## Worker

### `spawn <name>`

```bash
sunshine channel spawn <name>
  [--scope project|global]
  [--agent <agent-name>]                  # 加载 .sunshine/agents/<name>.md
  [--provider claude|codex]               # 覆盖 agent 文件
  [--as <worker-name>]                    # 默认：agent 名
  [--cwd <path>]
  [--model <id>]
  [--resume <id>]                         # 会话/thread id 恢复
  [--timeout <Ns|Nm|Nh>]                  # 超过该时长后自动 kill
  [--warn-before <Ns|Nm|Nh>]              # supervisor_warning 的提前时间
                                          # 默认 5m，0ms 表示禁用
  [--file <path>] ...                     # glob，可重复；注入内容
  [--jsonl <path>] ...                    # Sunshine 清单，可重复
  [--by <agent>]                          # spawn 事件作者
                                          # 默认：SUNSHINE_CHANNEL_AS 环境变量或 'main'
  [--inbox-policy explicitOnly|broadcastAndExplicit]
                                          # 默认 explicitOnly
  [--idle-timeout <Ns|Nm|Nh>]             # OOM 防护的空闲 TTL
                                          # 默认 5m，0 表示禁用
  [--max-live-workers <n>]                # spawn 时的存活 worker 预算
                                          # 默认 6，0 表示禁用
```

行为：
- provider 会对照 adapter 注册表校验（`packages/cli/src/commands/channel/adapters/`）；当前支持：`claude`、`codex`。
- Worker 在首次收到 `send --to <worker>` 之前一直处于 inbox 空闲状态。
- 记录一个 `spawned` 事件，包含 `pid`、provider、`agent`、`files`、`manifests`。
- OOM 防护优先级：CLI flag → 环境变量（`SUNSHINE_CHANNEL_WORKER_IDLE_TIMEOUT`、`SUNSHINE_CHANNEL_MAX_LIVE_WORKERS`）→ `.sunshine/config.yaml#channel.worker_guard` → 内置默认值。

### `run [name]`

```bash
sunshine channel run [name?]
  [--agent <name>]
  [--provider claude|codex]
  [--as <worker-name>]
  [--cwd <path>]
  [--model <id>]
  [--file <path>] ...                     # 可重复，glob
  [--jsonl <path>] ...                    # 可重复
  [--message <text> | --message-file <path> | --stdin]
  [--timeout <Ns|Nm|Nh>]                  # 默认 5m
```

行为：
- 一次性运行。如果省略 `name`，自动生成 `run-<hex>`。
- 创建一个临时 channel（`createMode=run`）、派生单个 worker、发送提示词、等待 `done`、把最终助手文本打印到 stdout，成功后删除该 channel。失败时保留 channel 供检查，退出码为 1。

> `run` **没有** `--tag` flag。完成是通过 supervisor 发出的 `done` 事件检测的。

### `kill <name>`

```bash
sunshine channel kill <name>
  --as <agent>                            # 必填——worker 的 agent 名
  [--scope project|global]
  [--force]                               # 立即 SIGKILL
```

行为：
- 默认路径：SIGTERM → 8 秒宽限 → SIGKILL 升级；当需要 SIGKILL 时，CLI 会写入一个 `killed` 事件，使日志保持真实。
- 清理 `pid`、`worker-pid`、`config`、`spawnlock` 附属文件；保留 `log`、`session-id`、`thread-id` 用于取证 / 恢复。

### `rm <name>`

```bash
sunshine channel rm <name>
  [--scope project|global]
```

行为：
- 先杀掉所有存活 worker，再删除整个 channel 目录。
- 打印 `Removed channel '<name>'`。

### `prune`

```bash
sunshine channel prune
  [--scope project|global]                # 省略：扫描每个项目
  [--all | --empty | --idle <Ns|Nm|Nh|Nd> | --ephemeral]   # 互斥
  [--yes]                                 # 真正删除（默认：演练）
  [--dry-run]                             # 默认 true；与默认值重复
  [--keep <names,csv>]                    # 排除列表
```

行为：
- 过滤 flag 互斥——否则报错。
- 默认是演练；`--yes` 切换为真正删除。
- 不带 `--scope` 时扫描**每个**项目 bucket（有意的全仓库清理）；带 `--scope project|global` 时只限于该 bucket。
- 无论用什么过滤器，有存活 worker 的 channel 始终跳过。
- 输出：每个候选一行 `name  last-ts  (reason)`，最后加一个汇总。

---

## Forum 频道

### `post <name> <action>`

```bash
sunshine channel post <name> <action>
  --as <agent>                            # 必填
  [--scope project|global]
  [--thread <key>]                        # 除 action=opened 外必填
  [--title <text>]
  [--text <text> | --stdin | --text-file <path>]
  [--description <text>]                  # 稳定的 thread 描述
  [--status <status>]
  [--labels a,b]                          # 替换 thread 的标签
  [--assignees a,b]                       # 替换 assignees
  [--summary <text>]
  [--context-file <abs-path>] ...
  [--context-raw  <text>]      ...
  [--linked-context-file <abs-path>]      # [已废弃的别名]
  [--linked-context-raw  <text>]          # [已废弃的别名]
```

行为：
- `<action>` 在 CLI 表面上是自由形式的；约定值包括 `opened`、`comment`、`status`、`labels`、`assignees`、`summary`、`processed`。
- `action=rename` 会被拒绝——请改用 `thread rename`。
- `--labels` / `--assignees` 是替换语义，不是追加。
- 输出：追加的事件 JSON 到 stdout。

### `forum <name>`

```bash
sunshine channel forum <name>
  [--scope project|global]
  [--status <status>]
  [--raw]
```

行为：
- 列出 thread（归约后的状态）。`--status` 按当前 thread 状态过滤。`--raw` 每行打印一个 thread 的 JSON。

### `thread <name> <thread>` / `thread rename`

```bash
sunshine channel thread <name> <thread-key>
  [--scope project|global]
  [--raw]

sunshine channel thread rename <name> <old-thread> <new-thread>
  --as <agent>                            # 必填
  [--scope project|global]
```

行为：
- `thread <name> <key>` 展示一个 thread 的时间线：标题行 `<thread> [<status>] <title>`，然后是 description / labels / assignees / summary / 时间线各行。`--raw` 切换为原始事件。
- `thread rename` 是唯一的变更操作；`post --action rename` 会被拒绝。

---

## Context / Title

### `context add` / `context delete` / `context list`

```bash
sunshine channel context add <name>
  [--as <agent>]                          # 默认：main
  [--scope project|global]
  [--thread <key>]                        # thread 级而不是频道级
  [--file <abs-path>] ...                 # 可重复
  [--raw <text>]      ...                 # 可重复
                                          # --file 或 --raw 至少其一

sunshine channel context delete <name>
  [--as <agent>]                          # 默认：main
  [--scope project|global]
  [--thread <key>]
  [--file <abs-path>] ...
  [--raw <text>]      ...

sunshine channel context list <name>
  [--scope project|global]
  [--thread <key>]
  [--raw]                                 # 每行一个 JSON 条目
```

行为：
- `add` / `delete` 追加一个 `context` 事件并打印该事件 JSON。
- `list` 投影当前的 context 条目；美化输出是 `file <path>` / `raw <截断文本>` 各行，空时显示 `(no context)`。

### `title set <name>` / `title clear <name>`

```bash
sunshine channel title set <name>
  --title <text>                          # 必填
  [--as <agent>]                          # 默认：main
  [--scope project|global]

sunshine channel title clear <name>
  [--as <agent>]                          # 默认：main
  [--scope project|global]
```

行为：
- 追加一个 `title` 事件，把稳定的展示标题投影到 channel 上。输出：事件 JSON。

---

## 隐藏 / 内部

| 命令 | 用途 |
|---|---|
| `channel __supervisor <channel> <worker> <config>` | 由 `spawn` 调用的 fork 入口点。不要直接调用。 |
| `channel __parse-trace <adapter> <file>` | 开发辅助工具——把一段录制的 stream-json / 线路 trace 通过匹配的 adapter 重放，并打印生成的 channel 事件。adapter 会对照 provider 注册表校验。 |

---

## 事件模型

`CHANNEL_EVENT_KINDS`（由 `parseChannelKind` 强制执行的白名单）：

`create`、`join`、`leave`、`message`、`thread`、`context`、`channel`、`spawned`、`killed`、`respawned`、`progress`、`done`、`error`、`waiting`、`awake`、`undeliverable`、`interrupt_requested`、`turn_started`、`turn_finished`、`interrupted`、`supervisor_warning`。

`MEANINGFUL_EVENT_KINDS`（未明确传入 `--kind` 时，`wait` / `messages` 使用的默认可见子集）：

`create`、`join`、`leave`、`message`、`thread`、`context`、`channel`、`spawned`、`killed`、`respawned`、`done`、`error`。

非 meaningful 的 kind（例如 `progress`、`waiting`、`awake`、`supervisor_warning`、`turn_*` / `interrupt*` 集合）仍会流经 store；通过 `--kind` 或 `--include-progress` 选择加入。

Forum 频道是事件溯源的；使用 CLI reducer（`forum`、`thread`、`context list`）做状态投影。

---

## 输出约定

- **变更操作**（`send`、`interrupt`、`post`、`context add/delete`、`title set/clear`、`thread rename`）把追加的事件作为一行 JSON 打印到 **stdout**。
- **流式读取**（`wait`、`messages --follow`）每行打印一个 JSON 事件到 stdout。
- **美化读取**（`list`、`messages`、`forum`、`thread`、`context list`）打印带颜色、带填充的表格 / 时间线。
- **`run`** 只把最终助手文本打印到 stdout（以便调用者管道处理）；诊断说明走 stderr。
- **错误**经由 `chalk.red("Error:")` 输出到 stderr，退出码为 `1`。
- **`wait` 超时**专门退出 **124**。
