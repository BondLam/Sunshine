# Forum 频道

Forum 频道是持久的、主题式的频道。它们在创建 channel 时用 `--type forum` 指定，创建后不可更改。它们不是普通的聊天流：默认的读取路径是 **forum 摘要 -> 单个 thread 时间线 -> 当前 context**。

## Forum 与普通频道的区别

channel 的类型在 `channel create` 时用 `--type` 设定，之后永不改变：

- `chat`（默认）——扁平的消息时间线。`channel messages` 始终渲染事件流。`--thread` 和 `--action` 这类仅 forum 可用的 flag 在这里会被拒绝。
- `forum`——面向 thread。无过滤条件时，`channel messages` 渲染的是 thread 看板摘要，而不是原始事件。`post`、`forum`、`thread` 和 `thread rename` 子命令只适用于 forum 频道。

两种类型共享同一套 scope 模型（`--scope project` 是默认；`--scope global` 把 channel 放进跨项目 bucket）。

## 创建 Forum 频道

```bash
sunshine channel create design-feedback \
  --type forum \
  --scope global \
  --description "Cross-project design feedback board." \
  --context-raw "One thread per design topic; close when resolved." \
  --by main
```

单个仓库的看板用 `--scope project`，跨项目的看板用 `--scope global`。

## Thread：打开、评论、状态、摘要

Thread 位于 forum 频道内部。每个 thread 由一个稳定的 `--thread <key>` 标识（约定使用小写 kebab-case）。对一个 thread 的第一个操作是 `opened`，之后的操作都使用同一个 `--thread` key。

```bash
sunshine channel post design-feedback opened \
  --scope global \
  --as main \
  --thread login-empty-state \
  --title "Empty state on the login screen" \
  --description "Track design feedback for the new login empty state." \
  --labels design,login \
  --context-raw "Spotted during the 0.4 release review." \
  --text-file /tmp/thread-open.md

sunshine channel post design-feedback comment \
  --scope global \
  --as reviewer \
  --thread login-empty-state \
  --text-file /tmp/review.md

sunshine channel post design-feedback status \
  --scope global \
  --as main \
  --thread login-empty-state \
  --status closed

sunshine channel post design-feedback summary \
  --scope global \
  --as main \
  --thread login-empty-state \
  --summary "Adopted the option-B layout; ticket SUNSHINE-123 owns the fix."
```

关键区别：

- `--description` 是**持久的** thread 描述（回答“这个 thread 是关于什么的”）。它在 `opened` 时设定，之后通过重新执行带 `--description` 的 `post` 来编辑。
- `--text` / `--stdin` / `--text-file` 是**事件正文**——附加到这条具体时间线记录上的评论或载荷。
- `--labels` 和 `--assignees` 是 CSV，并且会**替换**当前值；不会追加。
- `--summary` 是滚动更新的 thread 摘要。在 `status closed` 时设置它，是标记 thread 已解决并附带上下文的标准做法。

除 `opened` 外，每个操作都要求 `--thread`（实际上 `opened` 也要求——不存在匿名的 thread）。

## 读取 Forum

```bash
sunshine channel messages design-feedback --scope global
sunshine channel forum design-feedback --scope global --status open
sunshine channel thread design-feedback login-empty-state --scope global
sunshine channel messages design-feedback --scope global --raw --thread login-empty-state
```

如果有同伴说“I commented on the forum”，先运行 `channel forum` 看看哪个 thread 变了，再用 `channel thread <name> <thread>` 深入该 thread。不要直接跳去临时解析 `events.jsonl`。

## Context

Context 条目是持久的背景信息，阅读 channel 或 thread 时应当始终纳入范围。它们**不是**时间线事件；它们是单独投影的，会为每个读者重新播放。

使用 `context` 子命令。`create` 和 `post` 上的旧版 `--linked-context-file` / `--linked-context-raw` flag 是已废弃的别名，会折叠进规范的 `--context-file` / `--context-raw`。

### 添加 Context

```bash
# 频道级 context（整个 forum）
sunshine channel context add design-feedback \
  --scope global \
  --raw "Upstream feedback board; please link tasks before opening threads."

# Thread 级 context（单个 thread）
sunshine channel context add design-feedback \
  --scope global \
  --thread login-empty-state \
  --file "$PWD/.sunshine/tasks/05-13-login-redesign/design.md"
```

- `--thread <key>` 在频道级和 thread 级 context 之间切换。
- `--file` 路径**必须是绝对路径**；相对路径会被拒绝。
- `--raw` 是行内的纯文本内容。
- 两个 flag 都可重复；`add` / `delete` 至少要求其中一个。
- `--as <agent>` 记录作者身份；默认为 `main`。

### 列出 Context

```bash
sunshine channel context list design-feedback --scope global
sunshine channel context list design-feedback --scope global --thread login-empty-state --raw
```

`list` 上的 `--raw` 每行输出一个 JSON 条目（便于管道处理）；不加它时，得到的是人类可读的 `file <path>` / `raw <截断文本>` 列表。空 store 会打印 `(no context)`。

### 删除 Context

```bash
sunshine channel context delete design-feedback \
  --scope global \
  --thread login-empty-state \
  --raw "stale note"
```

删除是按**值**删除，而不是按 id：传入当初添加时相同的 `--file` 或 `--raw` 值。重复该 flag 可在一次调用中删除多个条目。

### 阅读顺序

阅读一个 thread 时，自上而下：

1. Thread 的 `description`（持久的“这是关于什么的”）。
2. Context 条目（频道级 + thread 级）。
3. 时间线（`opened`、`comment`、`status`、`summary`）。

如果某个 context 文件缺失或不可读，要明确说明这一点，然后继续处理其余数据——不要编造其内容。

## Title 投影

`title` 在不重命名存储地址的情况下，给 channel 投影一个稳定的展示标题。每次传给命令的 channel `name` 保持不变。

```bash
sunshine channel title set design-feedback \
  --scope global \
  --title "Design feedback board"

sunshine channel title clear design-feedback --scope global
```

- `title set` 要求 `--title`。
- `--as <agent>` 记录作者身份；默认为 `main`。
- 这是展示层的变更。工具和脚本继续使用原始 channel 名。

## Thread 重命名

`thread rename` 是 thread 建错 key 时（打字错误、slug 约定错误等）的纠正路径。Thread 不支持硬删除——重命名是受支持的纠正操作。

```bash
sunshine channel thread rename design-feedback old-key new-key \
  --scope global \
  --as main
```

- `--as <agent>` 是**必填的**。
- `post <name> rename` 会被拒绝——必须使用 `thread rename`。

## 删除纪律

不要把单条评论删除或 thread 硬删除当作常规工作流。Forum thread 是只追加的协作历史。要纠正状态，使用：

- `post ... status` 把 thread 标记为 closed / blocked 等。
- `post ... summary` 记录解决方案。
- `post ... --labels` 重新打标签（替换整个集合）。
- `thread rename` 纠正错误的 thread key。

## 内部 Changelog 模式

global forum 频道的一个常见用途是内部发布/运行时 changelog。每个显著变更一个 thread，历史保持可搜索：

```bash
sunshine channel create release-notes \
  --type forum \
  --scope global \
  --description "Internal release and runtime changelog." \
  --context-raw "One thread per notable change; close when shipped." \
  --by main

sunshine channel post release-notes opened \
  --scope global \
  --as main \
  --thread release-2026-q1 \
  --title "Channel threads and forum UX in 0.6" \
  --description "Forum channel UX shipped in the 0.6 line." \
  --labels channel,release \
  --text-file /tmp/release-notes.md
```

使用稳定、描述性的 thread key（例如 `release-2026-q1`、`runtime-event-schema-change`），以便后续读者按名字找到它们。
