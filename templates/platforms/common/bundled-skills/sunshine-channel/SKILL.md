---
name: sunshine-channel
description: 使用 Sunshine channel 进行实时多 Agent 协作、派生 worker、跨 Agent 评审、进度检查、forum 频道和 channel 日志调试。
---

# sunshine-channel

`sunshine channel` 是本地多 Agent 协作运行时。当多个 Agent 需要通过持久事件日志对话、当 worker 需要作为对等进程被派生、当运行中的 worker 需要中断/调试、或者反馈需要记录在持久的 `--type forum` 频道上时，就用它。

典型的用户信号：“和 codex/claude 讨论”、“brainstorm with another agent”、“spawn an implement/check worker”、“let agent review”、“open an issue board / changelog forum”、“look at this thread”、“channel is stuck / no output”、“progress was truncated”、“how do I write that channel command”。

本 skill 是一个索引。只加载当前任务对应的参考文件——不要全部预加载。

## 首选命令

```bash
sunshine --version
sunshine channel --help
sunshine channel list --all
sunshine channel list --scope global --all
```

如果用户点名了某个 channel 或 thread，先检查它再问背景：

```bash
sunshine channel forum <board> --scope global
sunshine channel thread <board> <thread> --scope global
sunshine channel context list <board> --scope global --thread <thread>
```

## 按用户意图路由

| 用户意图 | 阅读 |
|---|---|
| “和 codex/claude 讨论一下”、“brainstorm with another agent” | `references/workflows.md` |
| “派一个 implement/check agent”、“让 agent review”、“spawn a worker” | `references/workflows.md`，然后是 `references/workers.md` |
| “开 issue 区 / topic 群 / changelog / board”、“make a forum” | `references/forum.md` |
| “看看这个 thread / linked context”、“inspect a thread” | `references/forum.md` |
| “channel 卡住了 / 没输出 / progress 被截断”、“worker stalled” | `references/progress-debugging.md` |
| “具体命令怎么写”、“what flags does X take” | `references/command-reference.md` |

## 核心规则

- 新的 forum 频道使用 `--type forum`。一个 `thread` 是 forum 频道内的一个条目。
- 使用 `--context-file` / `--context-raw` 以及 `sunshine channel context add/delete/list`。`--linked-context-*` 是已废弃的术语。
- 长消息使用 `--stdin` 或 `--text-file`。不要把长篇中英混杂文本放在位置 shell 参数里。
- 美化后的 `messages` 输出是面向操作员的仪表盘，可能会截断 progress。请用 `--raw` 做审计。
- `--as` 是发言者或 worker 的代号，具体取决于命令。多个 Agent 或会话参与时，使用明确、稳定的名字。
- `--scope project`（默认）操作当前 cwd 所在项目的 bucket；`--scope global` 操作共享的 `__global__` bucket。要有意识地选择 scope——不传 `--scope global` 时，global 看板在项目列表里是看不见的。
- 头脑风暴要做多轮压力测试。一轮回答加一轮确认是评审，不是头脑风暴。
- **分发者等待模式**：使用 `--kind done` / `--kind turn_finished`（sunshine 发出的系统事件），不要用用户 `--tag` 作为完成信号。CLI 帮助把 `phase_done` / `question` 列为 `--tag` 示例，但只有 `interrupt` 是有硬编码 sunshine 行为的保留 tag，其余都是不透明的用户标签。依赖 worker 去执行 `send --tag <my_signal>` 并不可靠——LLM worker 常常把 tag 字符串写进正文，而不是真正执行 CLI 命令。见 `references/command-reference.md` 的“tag vs kind”。
- Forum 频道是事件溯源的。不要先解析 `events.jsonl`；使用 `forum`、`thread`、`messages --thread` 和 `context list`。
- `@mindfoldhq/sunshine-core` 拥有可复用的 channel/thread 状态、事件追加、seq 分配、context/title 投影、reducer 和任务辅助函数。CLI 拥有 flag、终端渲染、提示、worker 生命周期和进程退出。

## 参考文件

- `references/workflows.md` — 经典协作模式 A–F（对等头脑风暴、派生评审、分发并等待、forum issue 捕获、中断并重定向、一次性运行）。
- `references/forum.md` — forum 频道、context、title、重命名、changelog forum、thread 过滤。
- `references/workers.md` — spawn、agent 卡片、context 注入（`--file` / `--jsonl`）、中断、kill 语义。
- `references/progress-debugging.md` — progress/raw 检查、停滞 worker 诊断、OOM 防护、退出码。
- `references/command-reference.md` — 当前 CLI 命令参考（每个子命令、每个 flag、输出约定、scope/type 模型）。

## 不适用场景

- 一次静态评审，一个 markdown 文件加提示词就够了。
- 用自记录代替正常的工具调用。
- 长期记忆检索。使用持久 forum 频道记录可执行的 issue，使用 `sunshine mem`（`sunshine-session-insight` skill）做会话/历史搜索。
