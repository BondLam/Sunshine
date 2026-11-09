# 添加项目本地约定

很多时候用户并不需要改 Sunshine 机制；他们需要本地 AI 理解团队的约定。这时优先使用 `.sunshine/spec/` 或项目本地 skill，而不是编辑 `sunshine-meta`。

## 东西放哪里

| 内容类型 | 位置 |
| --- | --- |
| 代码必须遵守的规则 | `.sunshine/spec/<layer>/` |
| 跨层思考方法 | `.sunshine/spec/guides/` |
| 项目专属流程的 AI 能力 | 平台本地 skill |
| 一次性任务材料 | `.sunshine/tasks/<task>/` |
| 会话摘要 | `.sunshine/workspace/<developer>/journal-N.md` |

## 创建项目本地 Skill

如果用户想让 AI 知道“这个项目如何定制 Sunshine”，创建一个本地 skill：

```text
.claude/skills/sunshine-local/
└── SKILL.md
```

示例：

```md
---
name: sunshine-local
description: "Project-local Sunshine customizations for this repository. Use when changing this project's Sunshine workflow, hooks, local agents, or team-specific conventions."
---

# Sunshine Local

## Local Scope

This skill documents this repository's Sunshine customizations only.

## Custom Workflow Rules

- ...

## Local Hook Changes

- ...

## Local Agent Changes

- ...
```

对于多平台项目，在其他平台 skill 目录里放置等价版本，或对支持共享层的平台使用 `.agents/skills/`。

## 写入 `.sunshine/spec/`

如果内容是编码约定，把它写进规范。例如：

```text
.sunshine/spec/backend/error-handling.md
.sunshine/spec/frontend/components.md
.sunshine/spec/guides/cross-platform-thinking-guide.md
```

写完后，更新对应的 `index.md`，以便 AI 能从入口找到新规则。

## 让当前任务使用新约定

写好规范后，把它加入当前任务上下文：

```bash
sunshine task add-context <task> implement ".sunshine/spec/backend/error-handling.md" "Error handling conventions"
sunshine task add-context <task> check ".sunshine/spec/backend/error-handling.md" "Review error handling"
```

## 不要把项目私有规则存进 `sunshine-meta`

`sunshine-meta` 是用于理解 Sunshine 架构和本地定制入口的公开 skill。把项目私有内容放进：

- `.sunshine/spec/`
- 项目本地 skill
- 当前任务
- 工作区日志

这样可以防止未来对 Sunshine 内置 `sunshine-meta` 的更新覆盖团队自己的约定。
