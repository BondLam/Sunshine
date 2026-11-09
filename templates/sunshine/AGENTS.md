# AGENTS.md（Sunshine）

本文件面向在本仓库工作的 AI 编程助手。

## 工作方式

- 先读 `.sunshine/spec/` 与 `.sunshine/workflow.md`，再动手。
- 任务按 `.sunshine/tasks/` 里的拆分一轮一任务推进。
- 交付物写 `doc/`（需求、设计、原型、技术方案）；生产代码写 `src/`。

## 铁律

- 不口头改代码：需求变更先回源到 `.sunshine/` 与 `doc/`，再动手。
- 只改受影响的部分，未受影响的代码与样式不动。
- 关键按钮必须加防抖或提交 Loading 状态。
