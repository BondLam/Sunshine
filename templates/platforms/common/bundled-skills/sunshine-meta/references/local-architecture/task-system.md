# 本地任务系统

Sunshine 任务系统完全存储在用户项目的 `.sunshine/tasks/` 之下。每个任务是一个目录，包含需求、上下文、研究、状态和关系信息。

## 任务目录结构

```text
.sunshine/tasks/
├── 04-28-example-task/
│   ├── task.json
│   ├── prd.md
│   ├── design.md
│   ├── implement.md
│   ├── implement.jsonl
│   ├── check.jsonl
│   └── research/
└── archive/
    └── 2026-04/
```

| 文件 | 用途 |
| --- | --- |
| `task.json` | 任务元数据：状态、负责人、优先级、分支、父/子任务及类似字段。 |
| `prd.md` | 需求、约束和验收标准。轻量任务可以只有 PRD。 |
| `design.md` | 复杂任务的技术设计：边界、契约、数据流、兼容性、权衡。 |
| `implement.md` | 复杂任务的执行计划：有序检查清单、验证命令、评审关卡、回滚点。 |
| `implement.jsonl` | implement agent 必须先读的规范/研究文件列表。 |
| `check.jsonl` | check agent 必须先读的规范/研究文件列表。 |
| `research/` | 研究产物。复杂发现不应只留在聊天里。 |

## `task.json`

`task.json` 记录任务状态和元数据。常见字段：

| 字段 | 含义 |
| --- | --- |
| `id` / `name` / `title` | 任务身份与标题。 |
| `status` | 状态，例如 `planning`、`in_progress`、`review` 或 `completed`。 |
| `priority` | `P0`、`P1`、`P2`、`P3`。 |
| `creator` / `assignee` | 创建者与负责人。 |
| `package` | monorepo 中的目标包；可以为空。 |
| `branch` / `base_branch` | 工作分支与 PR 目标分支。 |
| `children` / `parent` | 父/子任务关系。 |
| `commit` / `pr_url` | 完成后的提交与 PR 信息。 |
| `meta` | 扩展字段。 |

## 父 / 子任务树

父/子任务关系用于工作结构。父任务把相关交付物归组到同一套来源需求之下；它不是依赖调度器，也不能替代子任务自己的规划产物。

当一个请求有多个可独立验证的交付物时，使用父任务。父任务拥有：

- 来源需求和面向用户的范围。
- 子任务及其责任边界的映射。
- 跨子任务的验收标准和最终集成评审。

子任务用于可独立走完规划、实现、检查和归档的交付物。如果一个子任务依赖另一个子任务，把该依赖写进子任务的 `prd.md` / `implement.md`；不要依赖树的位置来暗示顺序。

创建新子任务：

```bash
sunshine task create "<child title>" --description "<one-line summary>" --slug <child-slug> --parent <parent-dir>
```

链接或解除链接已有任务：

```bash
sunshine task add-subtask <parent-dir> <child-dir>
sunshine task remove-subtask <parent-dir> <child-dir>
```

父任务上的 `children` 是历史列表。当子任务归档时，Sunshine 会把该子任务名保留在父任务里，使 `[2/3 done]` 这类进度在已完成的子任务移入 `archive/` 后仍然有意义。

AI 不应把阶段编号当作任务状态。任务进度主要由 `status`、产物存在情况（`prd.md`、可选的 `design.md` / `implement.md`）、是否为子 Agent 模式配置了 JSONL 上下文，以及 `workflow.md` 中的阶段描述决定。

## 当前任务

用户看到的是“当前任务”，但 Sunshine 按会话存储当前任务状态。

```text
.sunshine/.runtime/sessions/<context-key>.json
```

`sunshine task start` 把任务路径写入当前会话的运行时会话文件。`sunshine task current --source` 显示当前任务及其来源。不同的 AI 窗口可以指向不同的任务，互不覆盖。

如果平台或 shell 环境没有稳定的会话身份，`sunshine task start` 可能无法设置当前任务。AI 应当阅读错误、检查平台 hook/会话环境，不要退回到共享的全局指针。

## JSONL 上下文

`implement.jsonl` 和 `check.jsonl` 是子 Agent 先读的上下文清单。它们不能替代 `implement.md`；`implement.md` 是人类可读的执行计划。

格式：

```jsonl
{"file": ".sunshine/spec/backend/index.md", "reason": "Backend conventions"}
{"file": ".sunshine/tasks/04-28-example/research/api.md", "reason": "API research"}
```

规则：

- 包含规范和研究文件。
- 不要包含即将被修改的代码文件。
- 不要把聊天里的临时结论当作唯一上下文。
- 没有 `file` 字段的行会被读者跳过。旧版 `{"_example": ...}` 占位行会被 `sunshine task validate` 拒绝——删除它们。

## 常用命令

```bash
sunshine task create "<title>" --description "<one-line summary>" --slug <slug>
sunshine task start <task>
sunshine task current --source
sunshine task add-context <task> implement <file> <reason>
sunshine task validate <task>
sunshine task finish
sunshine task archive <task>
```

修改任务系统时，AI 应当优先使用 `sunshine task` 子命令来维护结构。只有子命令覆盖不到的需求，才直接编辑 JSON/Markdown。

## 本地定制点

| 需求 | 编辑位置 |
| --- | --- |
| 修改默认任务模板 | 不在项目内改——任务模板由 `sunshine task create` 实现决定；项目本地字段放 `task.json` 的 `meta`。 |
| 修改状态语义 | `.sunshine/workflow.md`、workflow-state hook 逻辑和任务使用约定。 |
| 添加任务生命周期操作 | `.sunshine/config.yaml` 中的 `hooks.after_*`。 |
| 修改上下文规则 | `.sunshine/workflow.md` 中的规划产物指导，以及相关平台 agent/hook 说明。 |
| 修改归档策略 | 不在项目内改——归档路径与策略由 `sunshine task archive` 实现决定。 |

这些都是用户项目内的本地文件。除非用户想向上游贡献，否则不要默认编辑 Sunshine CLI 源码。
