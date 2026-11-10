# 本地定制总览

本目录面向在用户项目里本地工作的 AI，该项目已安装 Sunshine 二进制且已运行过 `sunshine init`。Sunshine 是 Go 单二进制发布的 CLI，没有 npm 包、没有本地 Python 运行时：任务与 hook 逻辑都在二进制内（`sunshine task` / `sunshine hook` 子命令）。AI 应当修改项目内生成的 `.sunshine/` 和平台目录，而不是 Sunshine CLI 的上游源码。

## 先确定用户到底想改什么

| 用户表述 | 先读 |
| --- | --- |
| “改 Sunshine 流程 / 阶段 / 下一步提示” | `change-workflow.md` |
| “改任务创建、状态、归档或 hook” | `change-task-lifecycle.md` |
| “AI 没读上下文 / 改注入的内容” | `change-context-loading.md` |
| “某个平台 hook 行为不符合预期” | `change-hooks.md` |
| “改 implement/check/research agent 的行为” | `change-agents.md` |
| “加一个 skill/命令/工作流/提示词” | `change-skills-or-commands.md` |
| “调整项目规范结构” | `change-spec-structure.md` |
| “加团队约定和本地笔记” | `add-project-local-conventions.md` |

## 通用操作顺序

1. **确认平台和目录**：检查存在哪些目录，例如 `.claude/`、`.cursor/`、`.codex/`（Go 版目前已接 22 个平台）。
2. **确认当前活跃任务**：运行 `sunshine task current --source`。
3. **阅读本地事实来源**：优先读 `.sunshine/workflow.md`、`.sunshine/config.yaml` 和相关平台文件。
4. **窄幅修改**：只编辑与用户请求相关的文件。
5. **同步语义**：如果共享流程变了，检查平台入口是否也需要改；如果平台入口变了，检查 `.sunshine/workflow.md` 是否仍然一致。

## 本地文件优先级

| 层 | 文件 |
| --- | --- |
| 工作流 | `.sunshine/workflow.md` |
| 项目配置 | `.sunshine/config.yaml` |
| 任务材料 | `.sunshine/tasks/<task>/` |
| 项目规范 | `.sunshine/spec/` |
| CLI 子命令 | `sunshine task` / `sunshine hook`（Go 二进制内实现，不在项目内改） |
| 平台集成 | 22 个已接平台的目录 |
| 共享 skill | `.agents/skills/` |

## 默认不要做的事

- 不要编辑 Sunshine 二进制的安装位置。
- 不要假设用户拥有 Sunshine GitHub 仓库。
- 不要用默认模板覆盖用户已修改的本地文件。
- 不要把团队项目规则放进公开的 `sunshine-meta`；项目规则属于 `.sunshine/spec/` 或本地 skill。

## 何时检查上游源码

只有当用户明确表达以下目标之一时，才切换到上游源码视角：

- “我想给 Sunshine 提 PR”
- “我想改 Sunshine 二进制的发布内容”
- “我想 fork Sunshine”
- “我想改 `sunshine init/update` 的生成逻辑”

否则，默认修改用户项目内的本地 Sunshine 文件。
