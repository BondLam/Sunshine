---
description: "Sunshine Copilot 提示词：开始会话"
---

# 开始会话

初始化一个由 Sunshine 管理的开发会话。本平台没有会话启动 hook，因此按以下步骤手动加载等价上下文（每一步对应 hook 本会注入的一个部分）。

---

## 步骤 1：当前状态
身份、git 状态、当前任务、进行中的任务、日志位置。

```bash
sunshine task context
```

如果输出里有一行以 `Sunshine update available:` 开头，汇总会话上下文时请原样复制整行。不要缩短操作类命令提示。

## 步骤 2：工作流概览
阶段索引 + skill 路由表 + 不可跳过规则。

```bash
sunshine task context --mode phase
```

完整指南见 `.sunshine/workflow.md`（按需阅读）。

## 步骤 3：规范索引
发现包与规范层，然后阅读每个相关索引文件。

```bash
sunshine task context --mode packages
cat .sunshine/spec/guides/index.md
cat .sunshine/spec/<package>/<layer>/index.md   # 每个相关层都要读
```

索引文件列出真正开始写代码时要读的具体规范文档。

## 步骤 4：决定下一步
从步骤 1 已知当前任务与状态。查看任务目录：

- **当前任务状态为 `planning` 且没有 `prd.md`** → 阶段 1.1。加载 `sunshine-brainstorm` skill。
- **当前任务状态为 `planning` 且已有 `prd.md`** → 停留在阶段 1。轻量任务可以只有 PRD；复杂任务需要 `design.md` + `implement.md`。在 `sunshine task start` 之前先加载相关阶段 1 步骤的详情。
- **当前任务状态为 `in_progress`** → 阶段 2 的步骤 2.1。加载该步骤的详情：
  ```bash
  sunshine task context --mode phase --step 2.1 --platform copilot
  ```
- **没有当前任务** → 先分类。简单对话/小任务，只问本轮是否应该创建一个 Sunshine 任务。复杂工作，问是否可以创建一个 Sunshine 任务并进入规划。如果用户说不，本会话跳过 Sunshine。

---

## Skill 路由（速查）

| 用户意图 | Skill |
|---|---|
| 新功能 / 需求不明确 | `sunshine-brainstorm` |
| 准备写代码 | `sunshine-before-dev` |
| 写完代码 / 质量检查 | `sunshine-check` |
| 卡住了 / 同一个 bug 修了多次 | `sunshine-break-loop` |
| 学到了值得沉淀的东西 | `sunshine-update-spec` |

完整规则与反自我合理化表见 `.sunshine/workflow.md`。
