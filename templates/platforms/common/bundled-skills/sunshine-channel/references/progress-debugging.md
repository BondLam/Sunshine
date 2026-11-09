# Progress 与调试

美化输出是给操作员看的。原始输出是审计日志。子命令（`forum`、`thread`、`messages`、`context`）是审计*接口*——在手动 grep `events.jsonl` 之前，先用它们。

## 美化输出 vs `--raw`

`sunshine channel messages <channel>` 渲染一个紧凑、人类可读的视图：时间戳、身份、kind 和简短正文。它是为浏览 channel 的操作员准备的，不是为诊断准备的。

美化输出可能而且将会截断：

- 长 progress 增量（`text_delta`、部分工具参数）
- 工具名和命令行
- 多行状态字段和结构化 `detail` 块
- 超出列宽的 forum thread 标题

当某些地方看起来“不对劲”——worker 看似卡住、progress 行在词中间断掉、action 字段显示 `...`——就切换到 `--raw`。Raw 模式每行输出一个 JSON 事件，与它在 `events.jsonl` 里的样子完全一致，所以什么都不会丢。

```bash
# 美化输出（操作员视图）
sunshine channel messages <channel> --kind done --last 10
sunshine channel messages <channel> --kind error --last 10

# Raw（诊断视图）——每行一个 JSON
sunshine channel messages <channel> --raw --kind progress --last 20
sunshine channel messages <channel> --raw --last 50
```

经验法则：永远不要根据一条被截断的 progress 行去诊断 worker。

### 重建流式文本

要重建模型在一轮中实际流式输出的内容，把 progress 事件里的 `detail.text_delta` 拼接起来：

```bash
sunshine channel messages <channel> --raw --kind progress --last 80 \
  | python3 -c 'import json,sys; [print((json.loads(l).get("detail") or {}).get("text_delta",""), end="") for l in sys.stdin if l.strip()]'
```

## 停滞 Worker 诊断

症状：`sunshine channel list` 显示 worker 正在运行，但 `messages` 里没有新事件，`wait` 也一直超时。

排查顺序：

1. **定位 channel 文件。** 如果不确定 channel 属于哪个 bucket，使用 `list --all --all-projects`。

   ```bash
   sunshine channel list --all --all-projects
   CHAN=~/.sunshine/channels/<bucket>/<channel>
   ```

2. **确认 supervisor 和 worker 的 PID 是否存活。**

   ```bash
   cat "$CHAN/<worker>.pid"            # supervisor PID
   cat "$CHAN/<worker>.worker-pid"     # 实际 CLI 子进程 PID
   ps -p "$(cat "$CHAN/<worker>.pid")"
   ps -p "$(cat "$CHAN/<worker>.worker-pid")"
   ```

   如果 supervisor PID 已消失，但 channel 仍把该 worker 列出来，你就遇到了幽灵条目——用 `sunshine channel kill <name> --as <worker> --force` 清理它。

3. **查看 worker 日志的尾部。** 这是查看 provider / MCP / 工具启动输出的标准位置，这些输出永远不会出现在 channel 上。

   ```bash
   tail -f "$CHAN/<worker>.log"
   ```

4. **检查最后的原始事件。** 一个发过 `progress` 但没有 `message`/`done` 的 worker，通常正在流式输出中，或被某个工具调用阻塞：

   ```bash
   sunshine channel messages <channel> --raw --last 50
   ```

常见的“活着但沉默”的原因：

- 首个 token 之前的 provider 冷启动（耗时较长，但最终会动）。
- 启动期间某个阻塞的 MCP 服务器——在 worker 日志里可见。
- Worker 在等待某个子进程挂起的工具结果。
- 提示词巨大 / 模型被限流；在 worker 日志里检查 provider 侧的错误。

## Progress 事件解读

一个 `progress` 事件代表一块进行中的工作。它的形状随 `action` 字段而变化，但起关键作用的字段始终在 `detail` 之下：

- `detail.text_delta`——增量模型输出（跨事件拼接可重建流式回复）。
- `detail.tool_name`、`detail.tool_input`——即将运行或正在运行的工具调用。
- `detail.status`——长时操作使用的短字符串（`starting`、`running`、`flushing`、`done`）。
- `detail.action`——语义标签（例如 thread 心跳的 `status`）。

Progress 事件按设计就是**嘈杂的**。除非传入 `--include-progress`，`wait` 会忽略它们。当你确实想看它们时，优先使用：

```bash
sunshine channel messages <channel> --raw --kind progress --last 80
```

一个以稳定节奏发出 progress、却从不以 `done`/`error`/`message` 收尾的流，是挂起工具调用的典型形态——去检查 worker 日志里的子进程。

## Wait 语义（快速参考）

`channel wait` 从 `events.jsonl` 的 EOF 处开始监听，并在以下事件到来时唤醒：

- `message`
- `done`
- `error`
- `killed`
- `progress`（仅带 `--include-progress` 时）

有用的过滤条件：

```bash
sunshine channel wait T --as main --from check --kind done --timeout 15m
sunshine channel wait T --as main --from check,check-cx --kind done --all --timeout 15m
sunshine channel wait T --as worker --tag interrupt --timeout 1h
sunshine channel wait T --as main --thread release-note --action status --timeout 10m
```

退出码：`0` 表示匹配到，`124` 表示超时，`1`/`2` 表示错误。在 `wait --all` 超时时，stderr 会写明仍缺失的 worker。

## 审计 `events.jsonl`——用子命令，别用 `grep`

每个 channel 都把完整历史持久化在 `$CHAN/events.jsonl`。调试时直接 `tail` / `grep` / `jq` 这个文件是很诱人的做法。不要养成这个习惯，forum 频道则**永远**不要这么做。

优先用子命令的原因：

- `messages` 已经用过滤条件（`--kind`、`--from`、`--last`、`--tag`、`--thread`、`--action`）重放了该文件，并通过 `--raw` 给出精确的 JSON。任何你想写的一行命令，`messages` 都已经做到了。
- `wait` 用 EOF 语义消费同一文件——用 `tail -f | jq` 重新实现它，会在负载下丢事件、在轮转下排错顺序。
- `context` 会物化 worker 的 inbox 视图，包括游标状态。手写的过滤器不遵守 `<worker>.inbox-cursor`。

### Forum 频道：永远不要直接解析 `events.jsonl`

Forum 频道把许多逻辑 thread 复用到同一个 `events.jsonl` 上。每个事件都带有 `thread`、`action` 和 tag 字段，forum 子命令知道如何把它们折叠起来。手动解析该文件会导致：

- 把 thread 混在一起，让一个 thread 看起来前后矛盾。
- 错过 thread 生命周期事件（打开 / 状态 / 关闭），而这些事件会改变后续事件的解读方式。
- 忽略 worker 的 inbox 游标，于是你会“看到” worker 已经消费过的事件，并误以为它们还在等待处理。

请改用 forum 感知的视图：

```bash
# 列出 forum 频道内的逻辑 thread
sunshine channel forum list <channel>

# 端到端检查一个 thread
sunshine channel thread show <channel> <thread>

# 重放某个 thread 的消息（支持 --raw、--kind、--last）
sunshine channel messages <channel> --thread <thread> --raw --last 100

# 某个具体 worker 还有什么待处理
sunshine channel context <channel> --as <worker>
```

只有在 CLI 本身值得怀疑时，才直接读取 `events.jsonl`——例如确认某个事件是否真的已持久化，或者在调试 supervisor 时拿它与 `<worker>.inbox-cursor` 做比对。

## 常见故障

| 症状 | 原因 | 修复 |
|---|---|---|
| `sunshine: command not found` | CLI 未安装或不在 PATH | 从 GitHub Releases 下载 Sunshine 二进制并放入 PATH（或从源码 `go build` 后放入 PATH） |
| `wait` 立即退出 | 过滤条件错误或身份冲突 | 使用不同的 `--as`，检查原始消息 |
| zsh 对消息文本报错 | shell 解释了标点符号 | 使用 `--stdin` 或 `--text-file` |
| progress 行被截断 | 美化输出截断 | 使用 `messages --raw --kind progress` |
| worker 从不说话 | provider 启动 / 提示词 / MCP 延迟 | 检查 `<worker>.log`、`ps`、原始事件 |
| 在另一个 cwd 下找不到 channel | 项目 bucket 不匹配 | `cd` 到项目，使用 `--scope global`，或使用 `list --all-projects` |
| 列表里有幽灵 worker | supervisor 死亡时未清理 | `sunshine channel kill <name> --as <worker> --force` |
| forum thread 看起来混乱 | 直接解析了 `events.jsonl` | 使用 `forum`、`thread`、`messages --thread` |

## 存储布局

```text
~/.sunshine/channels/
└── <bucket>/
    └── <channel-name>/
        ├── events.jsonl
        ├── <channel>.lock
        ├── <worker>.log
        ├── <worker>.pid
        ├── <worker>.worker-pid
        ├── <worker>.config
        ├── <worker>.session-id
        ├── <worker>.thread-id
        ├── <worker>.inbox-cursor
        └── <worker>.spawnlock
```

Agent 通常使用 CLI，而不是直接读文件。直接读文件是为 CLI 视图不足时的调试准备的——即便如此，也永远不要对 forum 频道的 `events.jsonl` 这么做。
