---
name: sunshine-meta
description: "理解并定制用户项目内的本地 Sunshine 架构。当修改 .sunshine 及平台 hook、设置、agent、skill、命令、提示词、工作流、channel 运行时（sunshine channel）、.sunshine/agents/ 下的内置运行时 agent、可选工作流模板、基于 registry 的规范刷新、由 sunshine init 生成的跨会话记忆（sunshine mem），或面向 AI 的内置 skill（sunshine-channel、sunshine-session-insight、sunshine-spec-bootstrap）及内置 skill 自动分发流程时使用。"
---

# Sunshine Meta

本 skill 面向已在项目里运行过 `sunshine init` 的本地 Sunshine 用户。读完它之后，AI 应当理解该用户项目内的 Sunshine 架构、运行模型和定制入口，然后按用户要求修改生成的 `.sunshine/` 和平台目录文件。

Sunshine 是 Go 单二进制发布的 CLI（无 npm 包、无 Python 运行时），工作流 / 持久化 / 平台模型之上新增了三个架构面。第一，多 Agent 协作运行时：`sunshine channel` 通过项目范围的 JSONL 事件日志（位于 `~/.sunshine/channels/<project>/<channel>/events.jsonl`）协调多个 AI worker 进程，带 worker OOM 防护、forum/thread 频道、持久幂等键，以及内置的 `.sunshine/agents/{architect,check,implement,plan,research}.md` 运行时定义。第二，跨会话记忆：`sunshine mem list | search | context | extract | projects` 读取磁盘上已有的原始 Claude Code 与 Codex 会话 JSONL，按 `--phase brainstorm|implement|all` 切片，永不上传任何内容。第三，任务子命令：`sunshine task`（创建、启动、完成、归档、上下文、会话记录）替代了旧的本地脚本。把这些与各平台集成文件一起当作一等定制面。

> 注意：三个架构面（`sunshine task`、`sunshine mem`、`sunshine channel`）均已实现，可直接使用。

默认的操作范围是用户项目内的本地文件：

- `.sunshine/`：工作流、配置、任务、规范、工作区、内置运行时 agent 和运行时状态。
- 平台目录：`.claude/`、`.cursor/`、`.codex/` 等 22 个已接平台的目录（见 `references/platform-files/platform-map.md`）。
- 共享 skill 层：`.agents/skills/`（Codex 与后续支持 Agent Skills 的平台共用）。
- 项目树之外的用户自有 channel store：`~/.sunshine/channels/<project>/<channel>/events.jsonl`。
- 可通过 `sunshine mem` 查询的原始平台对话日志：`~/.claude/projects/` 和 `~/.codex/sessions/`。

不要假设用户拥有 Sunshine 源码仓库。不要默认修改 Sunshine 二进制的安装位置——Sunshine 是单二进制发布的 CLI，每个版本一个二进制、一个 git tag。

## 使用方法

1. 先读 `references/local-architecture/overview.md`，建立本地 Sunshine 系统模型。
2. 如果请求涉及某个具体的 AI 工具，读 `references/platform-files/platform-map.md` 和相关的平台文件说明。
3. 如果请求涉及多 Agent 分发或 channel worker，读 `references/local-architecture/multi-agent-channel.md` 和内置的 `.sunshine/agents/` 文件。
4. 如果用户想改变行为，读 `references/customize-local/overview.md`，再打开具体的定制主题。
5. 编辑之前，先读用户项目里的实际文件，并把本地内容当作权威。

## 参考

### 本地架构

- `references/local-architecture/overview.md`：分层的本地 Sunshine 架构（工作流 / 持久化 / 平台 / channel 运行时）与定制原则。
- `references/local-architecture/generated-files.md`：`sunshine init` 生成的文件及其定制边界，包括 `.sunshine/agents/`。
- `references/local-architecture/workflow.md`：阶段、路由、workflow-state 块，以及 `.sunshine/workflow.md` 中可选的工作流模板（`native`、`tdd`、`channel-driven-subagent-dispatch`、marketplace）。
- `references/local-architecture/task-system.md`：任务目录、当前任务、JSONL 上下文、父/子任务树和任务运行时。
- `references/local-architecture/spec-system.md`：`.sunshine/spec/` 如何组织、注入，以及如何从 `registry.spec` 源刷新。
- `references/local-architecture/workspace-memory.md`：`.sunshine/workspace/` 日志，以及 `sunshine mem` 跨会话回忆。
- `references/local-architecture/context-injection.md`：Hook、子 Agent 前奏和 channel 运行时 worker inbox 路由。
- `references/local-architecture/multi-agent-channel.md`：`sunshine channel` 子命令、项目范围的事件 store、forum/thread 频道、worker OOM 防护、持久幂等和内置的 `.sunshine/agents/` 运行时 agent。
- `references/local-architecture/bundled-skills.md`：自动分发的内置 skill（`sunshine-meta`、`sunshine-spec-bootstrap`、`sunshine-session-insight`、`sunshine-channel`），以及 Go 二进制如何把内嵌模板里的它们发往每个平台 skill 根目录。

### 平台文件

- `references/platform-files/overview.md`：共享的 `.sunshine/` 文件与平台目录的关系，以及四种平台集成模式（hook 驱动、agent 前奏、主会话工作流、channel 运行时）。
- `references/platform-files/platform-map.md`：各平台的目录与 skill、agent、hook、扩展路径。
- `references/platform-files/hooks-and-settings.md`：设置/配置文件、hook、插件和扩展如何连接到 Sunshine；覆盖 `channel.worker_guard.*` 和 `codex.dispatch_mode`。
- `references/platform-files/agents.md`：各平台的 `sunshine-research` / `sunshine-implement` / `sunshine-check` 子 Agent 文件，以及 channel 运行时用的内置 `.sunshine/agents/{check,implement}.md`。
- `references/platform-files/skills-and-commands.md`：skill、命令、提示词和工作流的区别，以及如何修改它们。

### 本地定制

- `references/customize-local/overview.md`：为用户的请求选择正确的本地定制入口。
- `references/customize-local/change-workflow.md`：修改阶段、路由、下一步操作、workflow-state 和选定的工作流模板。
- `references/customize-local/change-task-lifecycle.md`：修改任务创建、状态、归档行为、父/子链接、归档 slug 冲突处理和生命周期 hook。
- `references/customize-local/change-context-loading.md`：修改任务、规范、日志、hook 上下文、channel inbox 消息和 `sunshine mem` 回忆的加载方式。
- `references/customize-local/change-hooks.md`：修改平台 hook、设置、任务生命周期 hook（`hooks.after_*`）和 shell 会话桥。
- `references/customize-local/change-agents.md`：修改平台子 Agent、内置 channel 运行时 agent 和 Codex `dispatch_mode` 开关下的 research、implement、check agent 行为。
- `references/customize-local/change-skills-or-commands.md`：添加或修改本地 skill、命令、提示词和工作流；覆盖上游内置 skill 的自动分发。
- `references/customize-local/change-spec-structure.md`：调整 `.sunshine/spec/` 下的项目规范结构，包括基于 registry 的源。
- `references/customize-local/add-project-local-conventions.md`：把团队规则放进项目本地规范或本地 skill。

## 当前规则

- `.sunshine/workflow.md` 是本地工作流的事实来源；其初始内容在 `sunshine init` 时从某个工作流模板（内置 `native`、`tdd`、`channel-driven-subagent-dispatch` 或 marketplace 模板）中选定，可通过 `sunshine workflow --template <id>` 重新选择。当前模板引用的 `.sunshine/agents/<name>.md` 文件缺失时，会触发一条非阻塞的 stderr 警告，指向 `sunshine update`。
- `.sunshine/config.yaml` 是项目级 Sunshine 配置入口。它承载任务生命周期 hook（`hooks.after_create` / `after_start` / `after_finish` / `after_archive`）、日志形态（`session_commit_message` / `max_journal_lines` / `session_auto_commit`）、channel worker 防护（`channel.worker_guard.idle_timeout` / `max_live_workers`）、Codex 分发模式（`codex.dispatch_mode: inline | sub-agent`），以及规范 registry 块（`registry.spec.source` + `registry.spec.template`）。
- `.sunshine/spec/` 存放用户项目专属的编码约定和设计约束。设置了 `registry.spec` 时，文件由 `sunshine update` 刷新；本地编辑会在 `.sunshine/.template-hashes.json` 中以“modified by user”冲突呈现。
- `.sunshine/tasks/` 存放任务 PRD、设计笔记、实现计划、研究文件和 JSONL 上下文。任务组成父/子树：`sunshine task create --parent <slug>`、`sunshine task add-subtask <parent> <child>`、`sunshine task remove-subtask <parent> <child>` 和 `sunshine task list-context <task>`。`sunshine task create` 会拒绝已存在于 `.sunshine/tasks/archive/**` 中的 slug。
- `.sunshine/workspace/` 存放**有意写下的**开发者日志。原始跨会话对话**不**存放在这里——它在磁盘上位于 `~/.claude/projects/` 和 `~/.codex/sessions/` 之下，通过 `sunshine mem search|extract|context` 恢复。内置的 `sunshine-session-insight` skill 讲授何时调用 `mem`。
- `.sunshine/agents/{architect,check,implement,plan,research}.md` 是内置的、平台无关的 channel 运行时 agent 定义，由 `sunshine channel spawn --agent <name>` 加载。可编辑；`sunshine update` 会补齐缺失的文件。编辑各平台的 `sunshine-implement.md` / `sunshine-check.md` **不会**改变 channel 运行时 worker 的行为。
- `~/.sunshine/channels/<project>/<channel>/events.jsonl` 是每个项目每个 channel 的 channel 运行时事件日志。用户自有、文件锁定的序号分配、持久的 `idempotencyKey` 支持；永不在 `.sunshine/` 之下。
- 内置多文件 skill（`sunshine-meta`、`sunshine-spec-bootstrap`、`sunshine-session-insight`、`sunshine-channel`）由 Go 二进制内嵌的 `templates/platforms/common/bundled-skills/` 经 `internal/platforms` 自动分发到每个已接平台的 skill 根目录。在上游的 `templates/platforms/common/bundled-skills/` 下新增一个目录，就会在下一次 `sunshine update` 时把它发往每个平台。
- 平台设置/配置文件决定哪些 hook、agent、skill、命令、提示词和工作流真正运行。
- `.sunshine/.template-hashes.json` 和 `.sunshine/.runtime/` 是管理/运行时状态文件。编辑它们之前先确认必要性。

## 不要做

- 不要把 Sunshine 上游源码当作本地定制的默认目标。
- 不要修改 Sunshine 二进制的安装位置来实现项目需求。
- 不要用默认模板覆盖用户已修改的本地文件；先检查 `.sunshine/.template-hashes.json`，优先使用 `.new` 附属文件而不是破坏性覆盖。
- 不要把团队私有的项目规则放进任何公开的内置 skill（`sunshine-meta`、`sunshine-spec-bootstrap`、`sunshine-session-insight`、`sunshine-channel`）；把项目规则放进 `.sunshine/spec/`、项目本地 skill、当前任务或工作区日志——`sunshine update` 会覆盖内置 skill 目录内的任何内容。
- 不要手工编辑 `~/.sunshine/channels/<project>/<channel>/events.jsonl`；序号在文件锁下分配，可重放的安全写入要走 `sunshine channel` CLI。
- 当目标是改变 channel 运行时 worker 行为时，不要编辑 `.claude/agents/sunshine-implement.md`（或任何其他各平台子 Agent 文件）——改编辑 `.sunshine/agents/<name>.md`。
- 不要把已删除或从未发布的机制描述为当前的 Sunshine 行为；在断言某个开关存在之前，先对照本地的 `.sunshine/config.yaml` 和已安装 CLI 的 `sunshine --help` 核对。
