# 本地规范系统

`.sunshine/spec/` 是用户项目专属的工程规范库。Sunshine 的目的不是让 AI 背诵约定；而是在正确的时间注入相关规范，或要求 AI 阅读它们。

## 目录模型

常见的单仓库结构：

```text
.sunshine/spec/
├── backend/
│   ├── index.md
│   └── ...
├── frontend/
│   ├── index.md
│   └── ...
└── guides/
    ├── index.md
    └── ...
```

常见的 monorepo 结构：

```text
.sunshine/spec/
├── webapp/
│   ├── backend/
│   │   ├── index.md
│   │   └── ...
│   └── unit-test/
│       ├── index.md
│       └── ...
├── api/
│   └── docs/
│       ├── index.md
│       └── ...
└── guides/
    ├── index.md
    └── ...
```

`index.md` 是每一层的入口。它应当列出 Pre-Development Checklist 和 Quality Check。具体指南位于同一目录的其他 Markdown 文件中。

## 包配置

`.sunshine/config.yaml` 可以声明包：

```yaml
packages:
  webapp:
    path: apps/webapp
  api:
    path: apps/api
default_package: webapp
```

AI 可以运行：

```bash
sunshine task context
```

该命令列出当前项目的包和规范层。配置上下文 JSONL 时，以该输出为准。

## 规范如何进入任务

在任务进入实现之前，如果任务需要的规范或研究上下文超出任务产物本身，规划阶段可能把相关规范写入 `implement.jsonl` / `check.jsonl`：

```jsonl
{"file": ".sunshine/spec/webapp/backend/index.md", "reason": "Webapp backend conventions"}
{"file": ".sunshine/spec/webapp/unit-test/conventions.md", "reason": "Test expectations"}
```

子 Agent 或平台前奏读取这些 JSONL 文件并加载引用的规范。在不支持子 Agent 的平台上，AI 应当按工作流直接阅读相关规范。

## 规范应当包含什么

规范应当包含项目的可执行工程约定，而不是通用最佳实践：

- 文件应当放在哪里。
- 错误处理应当如何表达。
- API、hook 和命令的输入/输出契约。
- 禁止的模式。
- 需要测试的情形。
- 项目专属的陷阱及避免方法。

当 AI 在实现或调试期间学到新规则时，应当更新 `.sunshine/spec/`，而不只在聊天里总结它。

## 本地定制点

| 需求 | 编辑位置 |
| --- | --- |
| 添加新的规范层 | `.sunshine/spec/<package>/<layer>/index.md` 及对应指南文件。 |
| 修改 monorepo 规范映射 | `.sunshine/config.yaml` 中的 `packages` / `default_package` / `spec_scope`。 |
| 修改实现前 AI 阅读哪些规范 | 任务的 `implement.jsonl`。 |
| 修改检查期间 AI 阅读哪些规范 | 任务的 `check.jsonl`。 |
| 修改何时应当更新规范 | `.sunshine/workflow.md` 中的 Phase 3.3 和 `sunshine-update-spec` skill。 |

## 边界

`.sunshine/spec/` 是用户的项目规范，不是 Sunshine 内置模板的永久副本。AI 应当鼓励用户按实际项目代码更新它，而不是把 Sunshine 默认模板当作不可变的文档。
