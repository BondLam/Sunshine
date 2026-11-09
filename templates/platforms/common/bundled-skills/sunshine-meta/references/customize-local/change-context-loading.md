# 修改本地上下文加载

上下文加载决定 AI 何时阅读工作流、任务、规范、研究、工作区和 git 状态。当用户说“AI 不知道当前任务”、“agent 没读规范”或“上下文太多/太少”时，读这一页。

## 先读这些文件

1. `.sunshine/workflow.md`
2. `sunshine task context` 的输出（会话上下文：当前开发者、git 状态、当前任务、活跃任务、日志、包）
3. 当前平台的 hook 或 agent 文件
4. 当前任务的 `implement.jsonl` / `check.jsonl`

会话上下文的组装逻辑在 Go 二进制内（`sunshine hook session-start`），不在项目里；项目内能改的是 workflow.md、任务 JSONL 与 agent 文件。

## 上下文来源

| 来源 | 用途 |
| --- | --- |
| `.sunshine/workflow.md` | 工作流和下一步操作提示。 |
| `.sunshine/tasks/<task>/prd.md` | 当前任务需求。 |
| `.sunshine/tasks/<task>/design.md` | 复杂任务的技术设计。 |
| `.sunshine/tasks/<task>/implement.md` | 复杂任务的执行计划。 |
| `.sunshine/tasks/<task>/implement.jsonl` | 实现前要读的规范/研究。 |
| `.sunshine/tasks/<task>/check.jsonl` | 检查期间要读的规范/研究。 |
| `.sunshine/spec/` | 项目规范。 |
| `.sunshine/workspace/` | 会话记录。 |
| git status | 当前工作树变更。 |

## 常见需求与编辑点

| 需求 | 编辑点 |
| --- | --- |
| 在新会话中注入更多/更少信息 | `.sunshine/workflow.md` 的注入说明，或平台的 `session-start` hook 注册（`sunshine hook session-start` 本身在 Go 二进制内，不在项目内改）。 |
| 修改每次用户输入时的提示 | `.sunshine/workflow.md` 中的 `[workflow-state:STATUS]` 块。`sunshine hook workflow-state` 只是解析器，按原文读取该块。 |
| Agent 没读规范 | 任务 JSONL、agent 前奏、`sunshine hook subagent-context` 的注入。 |
| 当前任务丢失了 | 平台会话身份传递与 shell 会话桥（`sunshine hook shell-session`）。 |
| 修改 JSONL 校验规则 | 不在项目内改——校验逻辑在 Go 二进制内（`sunshine task validate`）。 |

## JSONL 规则

`implement.jsonl` / `check.jsonl` 是关键的上下文加载接口：

```jsonl
{"file": ".sunshine/spec/backend/index.md", "reason": "Backend conventions"}
{"file": ".sunshine/tasks/04-28-x/research/api.md", "reason": "API research"}
```

只包含规范/研究文件。不要把将被修改的代码文件放进这些清单；agent 在实现期间会自己读代码文件。

## 修改会话上下文

如果用户希望每个新会话都看到更多项目状态，调整：

- `.sunshine/workflow.md` 里与会话总览相关的说明
- 对应的平台 `session-start` hook 注册（平台设置文件里调 `sunshine hook session-start` 的那一行）

注入内容的组装在 Go 二进制内，不在项目内改。

上下文不能无限制增长。优先注入索引和路径，让 AI 按需读取详细文件。

## 修改子 Agent 上下文

先确定平台使用哪种模式：

- hook 推送：平台 hook（`sunshine hook subagent-context`）在 agent 启动前注入。
- agent 拉取：编辑对应的 `sunshine-implement` / `sunshine-check` agent 文件里的读取步骤。

两种模式下，都要确保 agent 最终读到：

1. 当前任务
2. 对应的 JSONL
3. JSONL 引用的规范/研究
4. `prd.md`
5. 如有 `design.md`
6. 如有 `implement.md`

## 排查顺序

```bash
sunshine task current --source
sunshine task list-context <task>
sunshine task validate <task>
sunshine task context
```

编辑 hook/agent 之前，先确认任务和 JSONL 正确。
