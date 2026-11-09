# 本地 Sunshine 架构总览

`sunshine-meta` 面向已经运行过 `sunshine init` 的用户项目。用户机器上通常只有 Sunshine 二进制（Go 单文件发布，无 npm 包、无 Python 运行时）和项目内生成的 Sunshine 文件；可能没有 Sunshine CLI 源码。

因此，当 AI 使用本 skill 时，默认的定制目标是用户项目内的本地文件：

- `.sunshine/`：工作流、任务、规范、记忆和运行时状态。
- 平台目录：`.claude/`、`.cursor/`、`.codex/`（Go 版已接的三家；其余平台在滚动轨道待扩展）。
- 共享 skill 层：`.agents/skills/`（Codex 与后续支持 Agent Skills 的平台共用）。

不要默认引导用户 fork Sunshine CLI 仓库。只有当用户明确说想改 Sunshine 上游源码、发布新版本或贡献 PR 时，才把上游源码当作操作目标。

## 本地系统模型

Sunshine 在用户项目内提供三层：

1. **工作流层**：`.sunshine/workflow.md` 定义阶段、路由、下一步操作和提示块。
2. **持久化层**：`.sunshine/tasks/`、`.sunshine/spec/` 和 `.sunshine/workspace/` 存储任务、规范和会话记忆。
3. **平台集成层**：平台目录中的 hook、设置、agent、skill、命令、提示词和工作流，把 Sunshine 工作流连接到不同的 AI 工具。

三层都位于用户项目内，所以 AI 可以直接读取和修改它们。

## 核心路径

| 路径 | 用途 |
| --- | --- |
| `.sunshine/workflow.md` | 工作流阶段、skill 路由和 workflow-state 提示块。 |
| `.sunshine/config.yaml` | 项目配置、任务生命周期 hook、monorepo 包配置和日志配置。 |
| `.sunshine/spec/` | 用户项目专属的编码约定和思考指南。 |
| `.sunshine/tasks/` | 每个任务的 PRD、技术笔记、研究文件和 JSONL 上下文。 |
| `.sunshine/workspace/` | 各开发者的日志和跨会话记忆。 |
| `.sunshine/.runtime/` | 会话级运行时状态，例如当前任务指针。 |
| `.sunshine/.template-hashes.json` | Sunshine 管理文件的模板哈希，供 update 判断本地文件是否被用户修改过。 |

任务、hook 与上下文注入的逻辑不在项目里：它们由 Sunshine 二进制内的 `sunshine task` / `sunshine hook` 子命令实现。项目内可改的是工作流、配置、任务、规范与平台注册文件。

## AI 定制原则

1. **先找本地事实来源**：不要凭记忆编辑。先读 `.sunshine/workflow.md`、`.sunshine/config.yaml`、相关平台目录和相关任务文件。
2. **编辑用户项目，而不是二进制安装位置**：修改项目内生成的文件，而不是 Sunshine 二进制所在的目录。
3. **保持平台文件与 `.sunshine/` 对齐**：如果工作流路由变了，也要检查平台 skill 或命令是否仍描述同一流程。
4. **把项目专属规则放进 `.sunshine/spec/` 或本地 skill**：不要把团队约定放进 `sunshine-meta`。
5. **保留用户改动**：如果文件已被本地修改，从当前内容出发工作，而不是用默认模板覆盖它。

## 如何使用本目录

- 要了解 init 之后存在哪些文件，读 `generated-files.md`。
- 要修改阶段、路由或下一步操作，读 `workflow.md`。
- 要修改任务模型、JSONL 上下文或当前任务行为，读 `task-system.md`。
- 要修改编码约定注入，读 `spec-system.md`。
- 要了解日志和跨会话记忆，读 `workspace-memory.md`。
- 要修改 hook 或子 Agent 上下文加载，读 `context-injection.md`。
