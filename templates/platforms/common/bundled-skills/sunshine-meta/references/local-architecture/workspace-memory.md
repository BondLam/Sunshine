# 本地工作区记忆系统

`.sunshine/workspace/` 存储跨会话记忆。其目的是让 AI 和人类理解之前在不同窗口、不同日子里发生过什么。

## 目录结构

```text
.sunshine/workspace/
├── index.md
└── <developer>/
    ├── index.md
    ├── journal-1.md
    └── journal-2.md
```

| 文件 | 用途 |
| --- | --- |
| `.sunshine/.developer` | 当前开发者身份。 |
| `.sunshine/workspace/index.md` | 全局工作区总览。 |
| `.sunshine/workspace/<developer>/index.md` | 某位开发者的会话索引。 |
| `.sunshine/workspace/<developer>/journal-N.md` | 会话日志。 |

## 开发者身份

首次运行时执行：

```bash
sunshine init -u <name>
```

这会创建 `.sunshine/.developer` 和对应的工作区目录。AI 不应随意更改开发者身份；如果身份错了，先确认当前项目的使用者是谁。

## 日志

`journal-N.md` 记录每个会话中已完成或部分完成的工作。默认每份日志约 2000 行；之后轮换到下一个文件。

记录会话的常用命令：

```bash
sunshine task add-session \
  --title "Session title" \
  --summary "What changed" \
  --commit "abc1234"
```

没有提交的规划或评审工作，不传 `--commit` 也可以记录。

## 工作区记忆与任务的关系

| 系统 | 存储内容 |
| --- | --- |
| `.sunshine/tasks/` | 某个具体任务的需求、设计、研究和状态。 |
| `.sunshine/workspace/` | 跨任务和会话的工作记录。 |
| `.sunshine/spec/` | 作为长期约定保存的工程知识。 |

如果信息只对当前任务有用，放进任务目录。
如果信息描述当前会话中发生的事，放进工作区日志。
如果信息是以后每次写代码都应当遵守的，放进规范。

## 本地定制点

| 需求 | 编辑位置 |
| --- | --- |
| 修改日志最大行数 | `.sunshine/config.yaml` 中的 `max_journal_lines`。 |
| 修改会话自动提交消息 | `.sunshine/config.yaml` 中的 `session_commit_message`。 |
| 修改会话内容格式 | 不在项目内改——记录格式由 `sunshine task add-session` 实现决定。 |
| 修改工作区在上下文中的展示方式 | 不在项目内改——展示由 `sunshine hook session-start` 决定；如需调整，改 `.sunshine/workflow.md` 的总览说明。 |

## AI 使用规则

AI 不应把工作区当作唯一的事实来源。恢复任务时，先读当前任务，再用工作区做背景。任务完成后，把重要的过程笔记记入工作区；如果出现了长期规则，更新规范。
