# 本地多 Agent Channel 运行时

`sunshine channel` 是随 Sunshine CLI 附带的本地多 Agent 协作运行时。它让主 AI 会话派生对等 worker（Claude Code、Codex 或 `.sunshine/agents/` 下的任何 agent 定义），通过事件日志交换持久消息，并协调评审或头脑风暴循环，而无需手工拼接 shell 管道。

本参考说明 channel 如何接入用户项目，以便定制项目的 AI 知道该编辑什么。运行时用法（命令、forum/thread 模式、worker spawn flag）请以内置的 `sunshine-channel` 能力 skill 为准。

> 注意：`sunshine channel` 已实现（阶段 6），可直接使用。

## 本地系统模型

channel 运行时横跨三个本地面：

1. **存储层**，位于用户主目录：持久事件日志和 worker 状态文件。
2. **Agent 定义**，位于项目内的 `.sunshine/agents/`：平台无关的角色卡片，由 `sunshine channel spawn --agent <name>` 消费。
3. **项目配置**，位于 `.sunshine/config.yaml`：worker 防护阈值和其他 channel 旋钮。

## 核心路径

| 路径 | 用途 |
| --- | --- |
| `~/.sunshine/channels/<project>/<channel>/events.jsonl` | 每个 channel 的只追加事件日志。序号锁定、可安全重放。 |
| `~/.sunshine/channels/<project>/<channel>/<channel>.lock` | channel 级写锁。 |
| `~/.sunshine/channels/<project>/<channel>/<worker>.spawnlock` | OOM 防护使用的每 worker spawn 锁。 |
| `~/.sunshine/channels/<project>/<channel>/.seq` | 用于有序事件分配的序号附属文件。 |
| `~/.sunshine/channels/_global/<channel>/...` | 用 `--scope global` 创建的 channel。项目 bucket 被一个共享键替代。 |
| `.sunshine/agents/check.md` | `--agent check` 消费的默认 Check Agent 角色定义。 |
| `.sunshine/agents/implement.md` | `--agent implement` 消费的默认 Implement Agent 角色定义。 |
| `.sunshine/config.yaml`（`channel.*` 块） | worker 防护阈值和 channel 默认值。 |

项目 bucket 名由绝对项目路径派生（斜杠压平、非字母数字字符替换为 `-`），与 Claude Code 的 `~/.claude/projects/<sanitized-cwd>/` 约定一致。用 `SUNSHINE_CHANNEL_ROOT`（根目录）或 `SUNSHINE_CHANNEL_PROJECT`（bucket 名）覆盖，以供测试或沙箱使用。

## 何时使用 Channel 运行时

Channel 比单次 Bash 调用或一次性子 Agent 分发更重。只有至少满足以下条件之一时才使用：

- 工作需要**两个或更多 agent 对话**超过一轮（跨 AI 头脑风暴、对等评审、分发者 + worker）。
- worker 应当作为**对等进程**运行，主会话可以中断它、查看其进度，或异步等待它。
- 对话必须**持久且事后可检查**（forum/thread 频道、issue 看板、决策轨迹）。
- 多个 worker 必须**共享一个事件日志**，以便彼此看到对方的汇报。

在以下情况优先使用更便宜的原语：

- 单次 Bash 命令或单次 Agent 工具调用就够了 -> 直接做。
- 用户只需要针对某个文件的静态评审 -> 读文件并直接回复。
- 需求是“记住我们上周讨论过什么” -> 用 `sunshine mem` 而不是 channel。

## 定制点

| 需求 | 编辑位置 |
| --- | --- |
| 修改默认 channel worker 空闲超时 | `.sunshine/config.yaml` 中的 `channel.worker_guard.idle_timeout`。接受 `5m`、`30s` 等。设为 `0` 可禁用空闲清理。 |
| 修改存活 worker 预算 | `.sunshine/config.yaml` 中的 `channel.worker_guard.max_live_workers`。设为 `0` 可禁用 spawn 时的预算检查。 |
| 按每次 spawn 覆盖 worker 防护 | 在 `sunshine channel spawn` 上传入 `--idle-timeout` / `--max-live-workers`，或在环境中设置 `SUNSHINE_CHANNEL_WORKER_IDLE_TIMEOUT` / `SUNSHINE_CHANNEL_MAX_LIVE_WORKERS`。 |
| 修改默认 Check 或 Implement worker 的行为 | 编辑 `.sunshine/agents/check.md` 或 `.sunshine/agents/implement.md`。这些是平台无关的角色卡片；传入 `--agent check|implement` 时 channel 运行时注入它们。 |
| 添加新的角色卡片 | 把 `<name>.md` 放进 `.sunshine/agents/`。`sunshine channel spawn --agent <name>` 会找到它。 |
| 迁移 channel 存储（CI 沙箱、临时运行） | 设置 `SUNSHINE_CHANNEL_ROOT=/path/to/dir`。channel 事件随之迁移；已有 channel 留在旧根目录。 |
| 切换存储范围 | 在每个 channel 子命令上传入 `--scope project`（默认）或 `--scope global`。bucket 目录会变；其他不变。 |

worker 防护的优先级是：CLI flag > 环境变量 > `.sunshine/config.yaml` > 内置默认值。内置默认值是 `idle_timeout: 5m` 和 `max_live_workers: 6`。

## 与其他本地层的关系

- **工作流层**：使用 channel 分发的工作流（例如 `channel-driven-subagent-dispatch`）指示主 Agent 调用 `sunshine channel spawn --agent check` 或 `--agent implement`，而不是平台子 Agent。如果 `.sunshine/agents/check.md` 或 `implement.md` 缺失，`sunshine workflow --template <id>` 会在安装时打印非阻塞警告。如果误删了它们，用 `sunshine update` 恢复。
- **任务层**：channel worker 不拥有任务状态。监督的主会话通过 worker inbox 传递当前任务路径；worker 从磁盘解析任务产物。
- **规范层**：worker 阅读 `.sunshine/spec/` 的方式与主会话相同。channel 运行时不绕过规范上下文加载。
- **平台集成层**：channel 运行时是平台中立的。它不依赖 `.claude/`、`.codex/` 或任何其他平台目录。规范 provider 输出的 adapter（Claude 的 `stream-json`、Codex 的 `app-server`）位于 Sunshine CLI 二进制内部，而不是项目里。
- **平台子 Agent 文件 vs. channel worker**：编辑 `.claude/agents/sunshine-implement.md`（及其在其他平台 `.X/agents/` 目录下的同类文件）**不会**改变 channel 运行时 worker 的行为——channel worker 加载 `.sunshine/agents/<name>.md`。平台专属 agent 文件用于主 AI 会话的直接子 Agent 分发，而不是 channel 派生的 worker。各平台 agent 面的说明见 `platform-files/agents.md`，以及 `sunshine-meta/SKILL.md` 中固化这一区分的规则。

## 运行时用法

命令语法、forum/thread 模式、worker 代号、进度检查，以及 `--kind done` / `--kind turn_finished` 分发者等待模式，请加载内置的 `sunshine-channel` skill（在 `sunshine init` / `sunshine update` 后自动安装在每个平台的 skills 目录下）。本参考只覆盖本地文件布局和定制旋钮；不重复可能随版本变化的命令语法。
