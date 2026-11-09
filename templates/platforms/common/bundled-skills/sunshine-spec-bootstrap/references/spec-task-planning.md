# 规范任务规划

默认使用单个 Agent 作为执行模型。Agent 可以创建 Sunshine 任务以便追溯，但本 skill 不应当要求特定的平台、CLI 或并行 worker 模型。

## 分解

围绕真实的归属边界创建规范工作单元：

- 一个包有自己的约定时，一个包一个单元。
- 同一个包有前端、后端、CLI、worker 或共享库等不同规则时，一层一个单元。
- 某种模式跨越多个包、又不属于某一层时，一份跨领域指南。

避免人为分解。一个小型库通常只需要一次聚焦的规范梳理，而不是几个任务。

## 任务形态

当 Sunshine 任务有用时，写一份简明的 PRD，包含以下章节：

```markdown
# Fill <package-or-layer> Sunshine Specs

## Goal
Write project-specific `.sunshine/spec/` guidance for <scope>.

## Scope
- Spec directory:
- Source directories to inspect:
- Tests to inspect:
- Out of scope:

## Architecture Context
Summarize the concrete findings from repository analysis.

## Files To Create Or Update
- `.sunshine/spec/.../index.md`
- `.sunshine/spec/.../<topic>.md`

## Rules
- Adapt the spec file set to the real codebase.
- Use real source examples with file paths.
- Remove template-only sections that do not apply.
- Do not modify product source code unless the task explicitly asks for it.

## Acceptance Criteria
- [ ] Specs contain concrete examples and anti-patterns from the repository.
- [ ] No placeholder text remains.
- [ ] Index files match the final spec files.
- [ ] Claims are backed by source files, tests, or project docs.
```

## 可选的辅助 Agent

如果宿主支持子 Agent，辅助 Agent 可以检查独立的包或运行验证。它们是可选的。主 Agent 仍然拥有集成和最终质量。

辅助任务必须有明确的归属：

- 只读研究任务可以检查所分配范围需要的任何源码。
- 写任务应当拥有互不相交的规范目录。
- 验证任务应当检查占位符清除、断链和一致性。

不要在本 skill 里编码辅助 Agent 的名字、厂商专属命令或平台专属路由。任务里只写必需的工作和验收标准。
