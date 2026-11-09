# 修改本地工作流

当用户想改变 Sunshine 的阶段、下一步操作提示、是否创建任务、是否使用子 Agent，或何时检查/收尾时，先编辑 `.sunshine/workflow.md`。

## 先读这些文件

1. `.sunshine/workflow.md`
2. 当前平台的入口文件，例如 skill/命令/提示词/工作流
3. 当前任务的 `task.json` 和 `prd.md`

## 常见需求与编辑点

| 需求 | 编辑点 |
| --- | --- |
| 修改阶段名或阶段顺序 | `Phase Index` 和对应的 Phase 章节。 |
| 修改无任务时是否创建任务 | `[workflow-state:no_task]` 状态块。 |
| 修改规划期间的下一步 | Phase 1 和 `[workflow-state:planning]`。 |
| 修改 in_progress 期间是否要求 agent | Phase 2 和 `[workflow-state:in_progress]`。 |
| 修改完成后的收尾 | Phase 3 和 `[workflow-state:completed]`。 |
| 修改某种用户意图触发哪个 skill | `Skill Routing` 表。 |

## 修改步骤

1. 在 `.sunshine/workflow.md` 中找到相关章节。
2. 修改规则时，保持明确的触发条件和下一步操作。
3. 如果添加或重命名 skill/agent，同步平台目录中对应的文件。
4. workflow-state 的修改只需编辑 `.sunshine/workflow.md` 中的 `[workflow-state:STATUS]` 块。hook 只是解析器——它读取块里的任何内容。保持开标签和闭标签的 STATUS 字符串相同（`[workflow-state:foo]…[/workflow-state:foo]`）；STATUS 不匹配的对会被悄悄丢弃。
5. 让 AI 重新阅读 `.sunshine/workflow.md`；不要继续使用旧对话里的规则。

## 示例：放宽任务创建要求

要改变何时可以跳过任务创建，通常编辑 `[workflow-state:no_task]`：

```md
[workflow-state:no_task]
Task is not required when the answer is a one-reply explanation, no files are changed, and no research is needed.
[/workflow-state:no_task]
```

如果正式的 Phase 1 流程也需要改，同步 Phase 1 章节。

## 示例：某个平台不使用子 Agent

如果用户只想让某个平台避免使用子 Agent，先确认该平台在工作流中是否有独立分组。然后修改该平台分组的 Phase 2 路由，而不是删除所有平台的 `sunshine-implement` / `sunshine-check` 说明。

## `/sunshine:continue` 路由表

`/sunshine:continue` 通过决定接下来加载哪个阶段步骤来恢复任务。决策结合 `task.json.status` 与任务目录内产物的存在情况。该映射固定在命令本身；添加自定义状态的 fork 必须同时扩展 workflow.md 标签块和本表。

| `status` | 产物状态 | 恢复到 |
| --- | --- | --- |
| `planning` | 缺少 `prd.md` | Phase 1.1（加载 `sunshine-brainstorm`） |
| `planning` | 轻量任务且 `prd.md` 已完成 | 请求启动评审，然后运行 `sunshine task start` |
| `planning` | 复杂任务缺少 `design.md` 或 `implement.md` | 补齐缺失的规划产物 |
| `planning` | 复杂任务已有 `prd.md`、`design.md` 和 `implement.md` | 请求启动评审，然后运行 `sunshine task start` |
| `in_progress` | 对话历史中没有实现 | Phase 2.1（`sunshine-implement`） |
| `in_progress` | 实现已完成，未运行 `sunshine-check` | Phase 2.2（`sunshine-check`） |
| `in_progress` | 检查已通过 | Phase 3.3（规范更新）→ 3.4（提交） |
| `completed` | 任务仍在活跃树中 | Phase 3.5（运行 `/sunshine:finish-work` 归档） |

添加自定义状态（例如 `in-review`）时，在 `.sunshine/workflow.md` 中添加 `[workflow-state:in-review]` 块作为每轮面包屑，并且扩展本路由表——通常是编辑 `/sunshine:continue` 命令文件（`.{platform}/commands/sunshine/continue.md` 或等价文件），添加一行来决定从哪里恢复。没有路由条目，`/sunshine:continue` 会落入默认分支，用户到不了你预期的步骤。

## 注意事项

`.sunshine/workflow.md` 是本地项目工作流，不是不可变的模板。用户可以按团队习惯调整它。编辑之后，平台入口文件可能仍含有旧描述，也要检查它们。
