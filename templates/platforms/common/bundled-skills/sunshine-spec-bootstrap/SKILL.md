---
name: sunshine-spec-bootstrap
description: "用平台中立的单 Agent 工作流，引导创建项目专属的 Sunshine 编码规范。适用于创建或刷新 .sunshine/spec 指南、使用 GitNexus、ABCoder 或源码检查分析代码库、按包/层分解规范工作，以及编写有真实代码库支撑、无占位文本的规范文档。"
---

# Sunshine Spec Bootstrap

使用本 skill，基于真实代码库创建或刷新 `.sunshine/spec/` 指南。由一个有能力的 Agent 全权负责完整循环：分析仓库、选择规范边界、编写文档、验证结果。工作流不依赖特定的宿主、CLI 或 Agent 品牌。

## 工作流

1. 确认 Sunshine 已初始化，并检查当前的 `.sunshine/spec/` 树。
2. 用最好的可用工具分析仓库架构：GitNexus、ABCoder、语言工具和直接源码阅读。
3. 只有当包和层的划分真实反映代码库时，才按包和层分解规范工作。
4. 用项目中具体的模式、文件路径、示例和反模式填充或重塑规范文件。
5. 验证最终规范内部一致，且不含模板占位符。

## 参考路由

| 需求 | 阅读 |
|------|------|
| 仓库架构分析 | [references/repository-analysis.md](references/repository-analysis.md) |
| 规范工作分解与任务规划 | [references/spec-task-planning.md](references/spec-task-planning.md) |
| 编写高信号的 Sunshine 规范文件 | [references/spec-writing.md](references/spec-writing.md) |
| GitNexus 与 ABCoder 的 MCP 配置 | [references/mcp-setup.md](references/mcp-setup.md) |

## 运行规则

- 把模板当作起点，而不是契约。当仓库需要时，删除、重命名、拆分或新增规范文件。
- 优先采用有源码支撑的规则，而不是通用建议。每条重要建议都应当指向一个真实文件或重复出现的本地模式。
- 默认保持单负责人执行。可选的辅助 Agent 是实现细节，不是必需品，也不是用户可见的依赖。
- 除非目标项目已经标准化使用某个平台，否则不要写平台专属的指令。
- 不要在 `.sunshine/spec/` 里留下占位文本、空标题或复制来的样板。

## 完成标准

- `.sunshine/spec/` 描述的是项目现在的样子。
- 每个相关包或层都有带真实示例的实用编码指导。
- 不适用的模板章节已被删除。
- `index.md` 文件与最终的规范文件集合一致。
- 任何必需的配置或分析假设，都已记录在相关规范或任务笔记中。
