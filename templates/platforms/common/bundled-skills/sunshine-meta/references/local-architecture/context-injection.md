# 本地上下文注入系统

Sunshine 上下文注入的目标是让 AI 在正确的时间读正确的文件，而不是依赖模型记忆。在用户项目里，注入由 `sunshine hook` 子命令与平台 hook、agent 和 skill 共同实现（Go 二进制内，无本地脚本）。

## 注入的上下文类型

| 类型 | 来源 | 用途 |
| --- | --- | --- |
| session context | `sunshine task context` | 当前开发者、git 状态、当前任务、活跃任务、日志、包。 |
| workflow context | `.sunshine/workflow.md` | 当前 Sunshine 流程和下一步操作。 |
| spec context | `.sunshine/spec/` + 任务 JSONL | 实现/检查期间必须遵守的规范。 |
| task context | `.sunshine/tasks/<task>/prd.md`、`design.md`、`implement.md`、`research/` | 当前任务的需求、设计、执行计划和研究。 |
| platform context | 平台 hook/设置/agent | 让不同的 AI 工具通过各自机制读取上述文件。 |

## session-start

支持 session-start 的平台会在会话开始、清空、压缩或类似事件时注入 Sunshine 总览。注入内容通常包括：

- 工作流摘要。
- 当前任务状态。
- 活跃任务。
- 规范索引路径。
- 开发者身份和 git 状态。

如果用户觉得新会话里的 AI 不知道当前任务，先检查该平台的 session-start hook 或等价机制是否已安装并运行。

## workflow-state

workflow-state 是在每次用户轮次前后注入的轻量提示。它按当前任务状态，从 `.sunshine/workflow.md` 中选择一个块，例如 `no_task`、`planning`、`in_progress` 或 `completed`。

如果用户想改变“在某个状态下 AI 接下来应当做什么”，先编辑 `.sunshine/workflow.md` 中对应的状态块。

## sub-agent context

implement 和 check agent 需要任务上下文。Sunshine 有两种加载模式：

1. **hook 推送**：平台 hook 在 agent 启动前注入 jsonl 引用的文件，以及 `prd.md`、如有 `design.md`、如有 `implement.md`。
2. **agent 拉取**：agent 定义指示 agent 在启动后读取当前任务、jsonl 上下文和任务产物。

两种模式下，任务目录中的 JSONL 文件都是规范/研究上下文的清单。任务产物按以下顺序单独读取：`prd.md` -> `design.md if present` -> `implement.md if present`。

## JSONL 读取规则

`implement.jsonl` 和 `check.jsonl` 每行包含一个 JSON 对象：

```jsonl
{"file": ".sunshine/spec/backend/index.md", "reason": "Backend rules"}
```

读者应当跳过没有 `file` 字段的行（例如旧版 `_example` 占位符）。配置 JSONL 时，AI 应当只包含规范/研究文件，不要预先注册将被修改的代码文件。

## 当前任务与上下文键

当前任务状态位于 `.sunshine/.runtime/sessions/`，按会话隔离。Hook 尝试从平台事件、环境变量、转录路径或 `SUNSHINE_CONTEXT_ID` 解析上下文键。

如果 shell 命令看不到相同的上下文键，`sunshine task current --source` 可能报告没有当前任务。这种情况下，检查平台是否把会话身份传入 shell，而不是手工写一个全局当前任务文件。

## 本地定制点

| 需求 | 编辑位置 |
| --- | --- |
| 修改 session-start 注入内容 | 平台的 `session-start` hook 注册（调 `sunshine hook session-start` 的那一行），以及 `.sunshine/workflow.md` 的总览说明。 |
| 修改每轮 workflow-state 规则 | `.sunshine/workflow.md` 中的 `[workflow-state:STATUS]` 块。`sunshine hook workflow-state` 按原文解析这些块，不内嵌后备文本。 |
| 修改子 Agent 读取上下文的方式 | 平台 agent 定义、`sunshine hook subagent-context` 或 agent 前奏。 |
| 修改 JSONL 校验/展示 | 不在项目内改——校验逻辑在 Go 二进制内（`sunshine task validate`）。 |
| 修改当前任务解析 | 不在项目内改——解析逻辑在 Go 二进制内（`sunshine hook` 的会话桥接）；检查平台是否把会话身份传入 shell。 |

修改上下文注入时，验证两件事：新会话能看到正确的任务，子 Agent 能看到正确的任务产物/规范/研究。
