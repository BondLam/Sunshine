# 修改本地任务生命周期

任务生命周期包括创建、启动、上下文配置、完成、归档、父/子任务和生命周期 hook。默认的定制目标是 `.sunshine/tasks/` 和 `.sunshine/config.yaml`。任务生命周期本身由 Go 二进制内的 `sunshine task` 子命令实现，项目里没有本地脚本可改；定制入口是 `config.yaml` 的 `hooks` 与 `.sunshine/workflow.md`。

## 先读这些文件

1. `.sunshine/workflow.md`
2. `.sunshine/config.yaml`
3. 当前任务的 `.sunshine/tasks/<task>/task.json`

任务命令本身（`sunshine task create/start/finish/archive` 等）是 Go 二进制内的实现，不在项目里；`sunshine task --help` 列出全部子命令。

## 常见需求与编辑点

| 需求 | 编辑点 |
| --- | --- |
| 任务创建后自动同步外部系统 | `.sunshine/config.yaml` 中的 `hooks.after_create`。 |
| 任务启动后自动更新状态 | `.sunshine/config.yaml` 中的 `hooks.after_start`。 |
| 任务完成后运行脚本 | `.sunshine/config.yaml` 中的 `hooks.after_finish`。 |
| 归档后清理外部资源 | `.sunshine/config.yaml` 中的 `hooks.after_archive`。 |
| 修改默认任务字段 | 不在项目内改——字段结构由 `sunshine task` 实现决定；项目本地字段放 `task.json` 的 `meta`。 |
| 修改任务解析/搜索 | 不在项目内改——`sunshine task list` / `validate` 的行为在 Go 二进制内。 |
| 修改当前任务行为 | `.sunshine/workflow.md` 中的当前任务描述，以及平台 hook 或 shell 会话桥（见下文）。 |

## lifecycle hook

`.sunshine/config.yaml` 支持：

```yaml
hooks:
  after_create:
    - "bash .sunshine/hooks/my_sync.sh create"
  after_start:
    - "bash .sunshine/hooks/my_sync.sh start"
  after_finish:
    - "bash .sunshine/hooks/my_sync.sh finish"
  after_archive:
    - "bash .sunshine/hooks/my_sync.sh archive"
```

`.sunshine/hooks/` 是放用户自己的 hook 脚本的约定位置（不在模板哈希管理内，不会被 `sunshine update` 覆盖）。hook 命令可以是任意 shell 命令，不限于某个脚本目录。

Hook 命令会收到 `TASK_JSON_PATH` 环境变量，指向当前任务的 `task.json`。Hook 失败通常应当警告，但不应阻塞主任务操作。

## 修改任务字段

如果用户想添加项目本地字段，优先把它们放在 `task.json` 的 `meta` 之下，以免破坏 `sunshine task` 对标准字段的假设。

示例：

```json
"meta": {
  "linearIssue": "ENG-123",
  "risk": "high"
}
```

如果标准字段确实需要改，检查每个读取 `task.json` 的调用方（`sunshine task` 子命令与平台 hook）；字段结构本身由 Sunshine CLI 决定，不在项目内改。

## 修改当前任务

当前任务是存储在 `.sunshine/.runtime/sessions/` 中的会话级状态。不要退回到全局 `.current-task` 模型。如果用户想改变当前任务行为，调整：

- 平台 hook 或 shell 会话桥（`sunshine hook shell-session` 写会话票据，`sunshine task start/current/finish` 消费票据）
- `.sunshine/workflow.md` 中的当前任务描述

当前任务指针的写入逻辑在 Go 二进制内，不在项目内改。

### `sunshine task create` 会设置当前任务指针

`sunshine task create` 在写出新任务目录后，会尽力设置当前任务指针。其行为：

- 当调用 shell 带有会话身份（`SUNSHINE_CONTEXT_ID` 环境变量，或 hook 能识别的任何平台专属会话环境变量）时，位于 `.sunshine/.runtime/sessions/<context_key>.json` 的每会话指针会被重写，指向新任务。任务的 `status=planning` 和 `[workflow-state:planning]` 会在下一个 `UserPromptSubmit` 时立即触发。
- 当无法获得会话身份（在 AI 会话之外的原始 CLI 调用，或不向 shell 传递身份的平台）时，任务目录仍然会被创建，`status=planning` 仍然会被写入，但当前任务指针保持不变。用户回到 AI 会话后，可以用 `sunshine task start <dir>` 再挂接该任务。

这使得 `[workflow-state:planning]` 成为 `sunshine task create` 之后头脑风暴和 JSONL 策划工作期间的实时面包屑。此前版本的行为会让面包屑一直卡在 `no_task`，直到 `sunshine task start`，规划块实际上是死文本。

任务创建路径在 Go 二进制内实现，无法像改脚本那样 fork 出新路径；需要额外的创建后动作时，用 `config.yaml` 的 `hooks.after_create`，而不是绕开 `sunshine task create`。完整的状态写入约定见 `.sunshine/workflow.md`。

## 修改步骤

1. 用 `sunshine task current --source` 确认当前任务。
2. 阅读当前任务的 `task.json`，确认状态和字段。
3. 配置需求先编辑 `.sunshine/config.yaml`。
4. 命令行为需求属于 Sunshine CLI 本身，不在项目内改；先确认是不是配置或工作流能覆盖，不能覆盖再向 Sunshine 提需求。
5. 如果 AI 流程变了，同步 `.sunshine/workflow.md`。

## 不要做

- 不要直接编辑 `.sunshine/.runtime/sessions/` 来“修复”业务状态。
- 不要把项目私有字段硬编码进命令行为；优先使用 `meta`。
- 不要默认要求用户 fork Sunshine CLI。
