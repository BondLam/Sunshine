# 本地工作流系统

`.sunshine/workflow.md` 是用户项目内 Sunshine 工作流的事实来源。AI 不需要 Sunshine 源码就能理解当前项目应当如何推进任务；这一个文件就够了。

## 文件职责

`.sunshine/workflow.md` 有三项职责：

1. **解释工作流阶段**：Plan、Execute、Finish。
2. **定义 skill 路由**：当用户表达某种意图时，AI 应当使用哪个 skill 或 agent。
3. **提供 workflow-state 提示块**：hook 可以把当前状态的提示块注入对话。

## 当前阶段模型

```text
Phase 1: Plan    -> clarify what to build, produce prd.md and required research
Phase 2: Execute -> implement against the PRD and specs, then check
Phase 3: Finish  -> final verification, preserve lessons, and wrap up
```

每个阶段包含编号步骤，例如 `1.4 Configure context`。这些编号不是 `task.json` 中的运行时字段；它们是供 AI 和人阅读的工作流结构。

## Skill 路由

`workflow.md` 按平台能力区分路由：

- 支持子 Agent 的平台：实现默认分发 `sunshine-implement`，检查默认分发 `sunshine-check`。
- 不支持子 Agent 的平台：主会话先读 `sunshine-before-dev` 这类 skill，再直接执行。

修改本地 AI 行为时，先更新 `workflow.md` 中的路由描述，再检查对应的平台 skill、命令或 agent 文件是否需要保持同步。

## Workflow-State 提示块

`workflow.md` 底部可以包含这样的状态块：

```text
[workflow-state:no_task]
...
[/workflow-state:no_task]
```

Hook 按当前任务状态选择正确的块并注入对话。常见状态包括：

| 状态 | 含义 |
| --- | --- |
| `no_task` | 当前会话没有活跃任务。 |
| `planning` | 任务仍处于需求、研究或上下文配置阶段。 |
| `in_progress` | 任务已进入实现和检查。 |
| `completed` | 任务已完成，等待收尾或归档。 |

如果用户想改变“无任务时是否创建任务”、“何时可以跳过任务创建”或“是否要求子 Agent”这类策略，编辑这些状态块和它们上方的路由表。

## 本地修改模式

常见修改：

| 目标 | 编辑点 |
| --- | --- |
| 添加一个阶段 | 更新 Phase Index、阶段正文、路由和状态块。 |
| 修改任务创建策略 | 更新 `no_task` 状态块和 Phase 1 描述。 |
| 修改默认实现/检查路径 | 更新 Phase 2 和 skill 路由。 |
| 修改收尾流程 | 更新 Phase 3 和 `finish-work` 相关描述。注意当前分工：Phase 3.4 = AI 驱动的代码提交（批量、用户确认），Phase 3.5 = `/finish-work`（归档 + 记录会话）。工作树有未提交改动时，`/finish-work` 拒绝运行。 |
| 修改平台差异 | 更新按平台分组的路由描述。 |

编辑后，让 AI 重新阅读 `.sunshine/workflow.md`；不要假设旧对话里的流程仍然有效。

## 与平台文件的关系

`workflow.md` 是本地工作流的语义中心，但每个平台也可以有自己的入口文件：

- skill，例如 `sunshine-brainstorm` 和 `sunshine-check`。
- 命令/提示词/工作流，例如 continue 和 finish-work。
- hook，例如 session-start 或 workflow-state 注入。

如果只改 `workflow.md`，平台入口文件可能仍含有旧表述。当用户想改变“AI 实际做什么”时，也要检查相关平台目录。
