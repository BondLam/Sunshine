# Worker 与 Agent 卡片

当一个对等 Agent 应当独立执行并通过 channel 事件日志汇报回来时，使用 worker。worker 是挂在一个 channel 上的已注册子进程（claude 或 codex）；supervisor 把 inbox 消息转发给它，并把它的输出翻译回 channel 事件。

## Spawn

```bash
sunshine channel create impl-task --by dispatcher --cwd /path/to/repo
sunshine channel spawn impl-task --provider codex --as codex-impl --timeout 30m

echo "Implement the schema for table X per .sunshine/.../prd.md" \
  | sunshine channel send impl-task --as dispatcher --to codex-impl --stdin

sunshine channel wait impl-task --as dispatcher --from codex-impl --kind done --timeout 30m
```

`spawn` 会 fork 一个 `channel __supervisor` worker，它发出 `spawned`、流式输出 `progress`，最后应当以 `done`、`error` 或 `killed` 收尾。在收到 `send --to <worker>`（或设置了 `--inbox-policy broadcastAndExplicit` 时的广播）唤醒之前，worker 一直处于 inbox 空闲状态。

关键的 `spawn` flag：

- `--agent <name>`——加载 `.sunshine/agents/<name>.md`（provider/model/as/系统提示词默认值）。
- `--provider <claude|codex>`——覆盖 agent 卡片；对照 adapter 注册表校验。
- `--as <name>`——channel worker 代号；默认为 agent 名。
- `--cwd <path>`——worker 工作目录（同时也是 `--file`/`--jsonl` 的 jail 根目录）。
- `--model <id>`——模型覆盖。
- `--resume <id>`——恢复已有的 claude session / codex thread。
- `--timeout <duration>`——`30s` / `2m` / `1h` 后自动 kill。
- `--warn-before <duration>`——supervisor_warning 的提前时间（默认 `5m`；`0ms` 表示禁用）。
- `--file <path>`（可重复，支持 glob）——把文件内容注入系统提示词。
- `--jsonl <path>`（可重复）——Sunshine jsonl 清单（每行 `{file, reason}`）。
- `--by <agent>`——`spawned` 事件的作者（默认为 `$SUNSHINE_CHANNEL_AS` 或 `main`）。
- `--inbox-policy <explicitOnly|broadcastAndExplicit>`——默认 `explicitOnly`。
- `--idle-timeout <duration>`——OOM 防护的空闲 TTL（默认 `5m`；`0` 表示禁用）。
- `--max-live-workers <n>`——spawn 时的存活 worker 预算（默认 `6`；`0` 表示禁用）。

成功事件 `spawned` 会记录 `pid`、provider、`agent`、注入的 `files` 和解析后的 `manifests`，以便后续旁观者审计 context。

## Agent 卡片

`--agent <name>` 解析为 `.sunshine/agents/<name>.md`。卡片名必须匹配 `[A-Za-z0-9._-]+`。默认的 Sunshine 安装自带两张卡片：

- `.sunshine/agents/check.md`——代码质量评审员。
- `.sunshine/agents/implement.md`——实现运行的编码 worker。

```yaml
---
name: check
description: Code quality check expert.
provider: claude
---
```

frontmatter 字段填充 `spawn` 的默认值（provider、model、`as`）；markdown 正文成为 worker 的系统提示词角色。卡片**不会**自动附加任务文件——context 必须在每次 spawn 时显式注入（见下文）。

派生具名 agent 之前，务必先检查项目卡片：

```bash
ls .sunshine/agents
sed -n '1,100p' .sunshine/agents/check.md
```

## Context 注入

两个 flag 把内容注入 worker 系统提示词中 `# CONTEXT FILES` 块的下方，由 `context-loader` 组装：

- `--file <path>`——可重复，支持 glob（`*`、`**`）。每个匹配的文件都会被读取并拼接。
- `--jsonl <path>`——可重复的 Sunshine 清单，每行是 `{"file":"<path>","reason":"<why>"}`。reason 会作为标题注释保留在每个文件内容上方。

加载器强制执行的限制：

- 每个文件 1 MB 硬上限（超限 → 报错）。
- 每个文件 200 KB 警告到 stderr。
- 组装后 context 总计 500 KB 警告到 stderr。
- 路径穿越 jail：所有解析后的路径必须留在 `--cwd` 之下。

对任务目录派生一个 check agent 的示例：

```bash
TASK=.sunshine/tasks/05-13-example
sunshine channel spawn cr-example --agent check --provider codex --as check-cx \
  --file "$TASK/prd.md" \
  --file "$TASK/design.md" \
  --file "$TASK/implement.md" \
  --jsonl "$TASK/check.jsonl" \
  --cwd "$PWD" --timeout 30m
```

`spawned` 事件同时记录字面 `files` 数组和由 `--jsonl` 展开的 `manifests`，因此审计轨迹能捕获 worker 实际看到的内容。

## 名字与路由

`--as` 有两层含义：

- `send` / `wait` / `interrupt`：发言者身份（生成事件的作者）。
- `spawn`：其他 Agent 用 `--to` 寻址的 worker 代号。

当多个 worker 或 provider 参与同一个 channel 时，使用明确的名字：

```bash
sunshine channel spawn cr-feature --agent check --as check-claude
sunshine channel spawn cr-feature --agent check --provider codex --as check-cx

sunshine channel wait cr-feature --as main \
  --from check-claude,check-cx --kind done --all --timeout 15m
```

`--all` 要求传入 `--from`，并阻塞直到每个列出的 worker 都产生了匹配事件；超时时退出码为 **124**，并向 stderr 打印 `timeout: still waiting on ...`。

## 软中断——`interrupt`

`channel interrupt` 是协作式重定向：它追加一个 `interrupt` 事件（reason 为 `"user"`），并在 adapter 支持的地方发出 provider 级别的轮次中断，附带替换指令。当 worker 应当丢弃当前轮次、立即按新输入行动、同时又不丢失会话时，使用它。

```bash
echo "Stop refactoring the parser — switch to fixing the failing test in src/foo.ts" \
  | sunshine channel interrupt impl-task --as dispatcher --to codex-impl --stdin
```

Flag：

- `--as <agent>` **（必填）**——调用者身份。
- `--to <agent>` **（必填）**——目标 worker。
- `--scope <project|global>`——channel scope。
- `--stdin` / `--text-file <path>` / `[text]`——替换指令正文。

追加的事件具有 `kind: "interrupt"`——下游的 `wait` / `messages` 过滤器可以用 `--kind interrupt` 订阅重定向（例如记录路由变更，或把其他 worker 的工作交给协调者的纠正来把关）。

对于应当等到 worker 下一轮次再处理的低优先级提示，改为发送一条普通的带 tag 消息：

```bash
echo "Check this when you reach the next turn." \
  | sunshine channel send impl-task --as dispatcher --to codex-impl \
      --stdin --tag question
```

## 硬中断——`kill` + `--resume`

当 worker 必须**立刻**停止时（例如失控循环、已在执行的错误指令，或 adapter 不执行 `interrupt`），使用 `kill`。supervisor 按 SIGTERM → 8 秒宽限 → SIGKILL 逐级升级；当需要 SIGKILL 时，CLI 会写入一个 `killed` 事件，使事件日志保持真实。

```bash
sunshine channel kill impl-task --as codex-impl
sunshine channel spawn impl-task --as codex-impl --provider codex \
  --resume "$(cat ~/.sunshine/channels/<bucket>/impl-task/worker.session-id)"

echo "STOP — new instructions: ..." \
  | sunshine channel send impl-task --as dispatcher --to codex-impl --stdin
```

`kill` 的 flag：

- `--as <agent>` **（必填）**——指明 worker（位置参数 `<name>` 是 channel）。
- `--scope <project|global>`。
- `--force`——立即 SIGKILL（同时杀掉内部 worker pid）。

副作用：清理 `pid`、`worker-pid`、`config`、`spawnlock` 附属文件；保留 `log`、`session-id`、`thread-id` 用于取证和恢复。

当 `interrupt` 无法收敛时，kill + `--resume` 是有保障的重定向路径。

## Worker OOM 防护

OOM 防护防止孤儿/空闲 worker 堆积并耗尽主机资源。它在每次 `spawn` 时运行，并按每个项目 bucket 强制执行两条策略：

- **空闲 TTL**——清扫上次活动早于配置阈值的 worker（默认 `5m`；`0` 表示禁用）。
- **存活 worker 预算**——如果同一项目 bucket 里已有超过 N 个 worker 存活，则拒绝新的 spawn（默认 `6`；`0` 表示禁用）。

优先级（从高到低）：

1. CLI flag：`spawn` 上的 `--idle-timeout`、`--max-live-workers`。
2. 环境变量：`SUNSHINE_CHANNEL_WORKER_IDLE_TIMEOUT`、`SUNSHINE_CHANNEL_MAX_LIVE_WORKERS`。
3. `.sunshine/config.yaml` 中 `channel.worker_guard` 之下的配置。
4. 内置默认值（`5m`、`6`）。

清理通知在 spawn 时写入 stderr，以便操作员看到哪些空闲 worker 被清扫、新的 spawn 为何被拒绝。防护对临时 / `channel run` worker 没有任何区别对待——它们同样受空闲 TTL 和预算约束。

要审计当前状态，通过 `channel list` 的 `WORKERS` 列列出 worker，并检查 `~/.sunshine/channels/<bucket>/<channel>/` 下每个 channel 的 `pid` / `worker-pid` 附属文件。

## Worker Inbox API

inbox 是 worker 被唤醒时面对的 channel 表面。路由由两个旋钮控制：

- **Inbox 策略**（`spawn --inbox-policy`）：
  - `explicitOnly`（默认）——worker 只在 `send --to <worker>` 或 `interrupt --to <worker>` 时唤醒。
  - `broadcastAndExplicit`——广播（不带 `--to` 的 `send`）也会唤醒。
- **投递模式**（`send --delivery-mode`）：
  - `appendOnly`——无论 worker 状态如何都追加事件。
  - `requireKnownWorker`——如果 `--to` 中指名的 worker 从未被 spawn 过，则失败。
  - `requireRunningWorker`——如果指名的 worker 当前未存活，则失败。

更严格的投递模式可以防止调用者期望一个运行中的对等方时消息被悄悄丢失。

与 inbox 相关的子命令：

- `send <channel> [text]`——追加一个 `message` 事件。
  - `--as <agent>` **（必填）**——作者。
  - `--to <agents>`——CSV；一个 → 字符串，多个 → 数组；省略表示广播。
  - `--stdin` / `--text-file <path>` / `[text]`——正文来源。
  - `--delivery-mode <appendOnly|requireKnownWorker|requireRunningWorker>`。
- `interrupt <channel> [text]`——软中断重定向（见上文）。
- `wait <channel>`——阻塞直到匹配的事件到达。
  - `--as <agent>` **（必填）**——用于过滤上下文的 `self`。
  - `--from <agents>`——CSV 作者。
  - `--kind <kind[,kind...]>`——CSV（或语义）；支持 `interrupt`、`done`、`progress` 等。
  - `--to <target>`——默认为自己的 agent（广播 + 明确发给我的）。
  - `--include-progress`——同时在 progress 事件到来时唤醒。
  - `--all`——要求每个 `--from` agent 都匹配（超时 → 退出 **124**）。
  - `--timeout <duration>`——`30s` / `2m` / `1h` / `1000ms`。
- `messages <channel>`——查看 / 过滤 / 跟随事件流。
  - `--follow` 跟随输出，`--kind` / `--from` / `--to` 过滤，`--raw` 输出每行一个 JSON，`--no-progress` 隐藏 progress 噪声。

典型的分发者循环：

```bash
# 1. 唤醒 worker。
echo "Run the failing test and report." \
  | sunshine channel send impl-task --as dispatcher --to codex-impl --stdin \
      --delivery-mode requireRunningWorker

# 2. 阻塞直到它完成。
sunshine channel wait impl-task --as dispatcher \
  --from codex-impl --kind done,error --timeout 30m

# 3. 读取最终答案。
sunshine channel messages impl-task --from codex-impl --last 1 --raw
```

所有发出事件的子命令（`send`、`interrupt`、`post`、`context add` / `delete`、`title set` / `clear`、`thread rename`）都会把追加的事件作为一行 JSON 打印到 stdout，使 inbox 层易于编写脚本调用。
