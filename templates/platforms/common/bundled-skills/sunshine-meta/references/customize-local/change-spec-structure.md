# 修改本地规范结构

当用户想改变 AI 遵循的工程约定、添加新的规范层，或调整 monorepo 包映射时，编辑 `.sunshine/spec/` 和 `.sunshine/config.yaml`。

## 先读这些文件

1. `.sunshine/config.yaml`
2. `.sunshine/spec/`
3. `.sunshine/workflow.md` 的规划产物指导和 Phase 3.3
4. 当前任务的 `implement.jsonl` / `check.jsonl`

## 常见需求

| 需求 | 编辑位置 |
| --- | --- |
| 添加后端/前端/文档/测试规范层 | `.sunshine/spec/<layer>/` 或 `.sunshine/spec/<package>/<layer>/` |
| 添加共享思考指南 | `.sunshine/spec/guides/` |
| 调整 monorepo 包 | `.sunshine/config.yaml` 中的 `packages` |
| 修改默认包 | `.sunshine/config.yaml` 中的 `default_package` |
| 控制规范扫描范围 | `.sunshine/config.yaml` 中的 `spec_scope` |
| 让任务读新的规范 | 任务的 `implement.jsonl` / `check.jsonl` |

## 添加规范层

单仓库示例：

```text
.sunshine/spec/security/
├── index.md
└── auth.md
```

Monorepo 示例：

```text
.sunshine/spec/webapp/security/
├── index.md
└── auth.md
```

`index.md` 应当包括：

- 该层适用于哪些代码。
- Pre-Development Checklist。
- Quality Check。
- 指向具体指南文件的链接。

## 更新上下文

添加规范并不意味着每个任务都会自动读它。当前任务必须在 JSONL 中引用它：

```bash
sunshine task add-context <task> implement ".sunshine/spec/webapp/security/index.md" "Security conventions"
sunshine task add-context <task> check ".sunshine/spec/webapp/security/index.md" "Security review rules"
```

## 修改 Monorepo 包

`.sunshine/config.yaml` 示例：

```yaml
packages:
  webapp:
    path: apps/web
  api:
    path: apps/api
default_package: webapp
```

编辑后运行：

```bash
sunshine task context
```

用该输出确认 AI 能看到正确的包和规范层。

## 注意事项

- 规范是用户项目约定，可以按项目需要修改。
- 不要把临时任务信息放进规范；临时信息放进任务。
- 不要把长期约定只放在 agent 或命令里；把它们保存在规范中。
- 修改规范结构后，检查已有任务 JSONL 文件是否仍指向存在的文件。
