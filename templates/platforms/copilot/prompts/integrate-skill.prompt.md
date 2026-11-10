---
description: "Sunshine Copilot 提示词：把 Claude Skill 集成进项目规范"
---

# 把 Claude Skill 集成进项目规范

把一个 Claude 全局 skill 适配并集成到项目的开发规范里（不是直接集成进项目代码）。

## 用法

```
/ <skill-name>
```

**示例**：
```
/ frontend-design
/ mcp-builder
```

## 核心原则

> [!] **重要**：skill 集成的目标是更新**开发规范**，不是直接生成项目代码。
>
> - 规范内容 -> 写到 `.sunshine/spec/{target}/doc.md`
> - 代码示例 -> 放到 `.sunshine/spec/{target}/examples/skills/<skill-name>/`
> - 示例文件 -> 用 `.template` 后缀（例如 `component.tsx.template`），避免 IDE 报错
>
> 其中 `{target}` 是 `frontend` 或 `backend`，由 skill 类型决定。

## 执行步骤

### 1. 阅读 skill 内容

```bash
openskills read <skill-name>
```

如果 skill 不存在，提示用户检查可用 skill：
```bash
# 可用 skill 列在 AGENTS.md 的 <available_skills> 下
```

### 2. 确定集成目标

按 skill 类型确定要更新哪些规范：

| Skill 类别 | 集成目标 |
|----------------|-------------------|
| UI/前端（`frontend-design`、`web-artifacts-builder`） | `.sunshine/spec/frontend/` |
| 后端/API（`mcp-builder`） | `.sunshine/spec/backend/` |
| 文档（`doc-coauthoring`、`docx`、`pdf`） | `.sunshine/` 或新建专属规范 |
| 测试（`webapp-testing`） | `.sunshine/spec/frontend/`（E2E） |

### 3. 分析 skill 内容

从 skill 中提取：
- **核心概念**：skill 如何工作、关键概念
- **最佳实践**：推荐做法
- **代码模式**：可复用的代码模板
- **注意事项**：常见问题与解决方法

### 4. 执行集成

#### 4.1 更新规范文档

在对应的 `doc.md` 里新增一节：

```markdown
@@@section:skill-<skill-name>
## # <Skill Name> 集成指南

### 概览
[skill 的核心功能与使用场景]

### 项目适配
[在本项目中如何使用该 skill]

### 使用步骤
1. [步骤 1]
2. [步骤 2]

### 注意事项
- [项目专属约束]
- [与默认行为的差异]

### 参考示例
见 `examples/skills/<skill-name>/`

@@@/section:skill-<skill-name>
```

#### 4.2 创建示例目录（如果有代码示例）

```bash
# 目录结构（{target} = frontend 或 backend）
.sunshine/spec/{target}/
|-- doc.md                      # 添加 skill 相关章节
|-- index.md                    # 更新索引
+-- examples/
    +-- skills/
        +-- <skill-name>/
            |-- README.md               # 示例说明
            |-- example-1.ts.template   # 代码示例（用 .template 后缀）
            +-- example-2.tsx.template
```

**文件命名约定**：
- 代码文件：`<name>.<ext>.template`（例如 `component.tsx.template`）
- 配置文件：`<name>.config.template`（例如 `tailwind.config.template`）
- 文档：`README.md`（正常后缀）

#### 4.3 更新索引文件

在 `index.md` 的快速导航表里加一行：

```markdown
| <Skill 相关任务> | <章节名> | `skill-<skill-name>` |
```

### 5. 生成集成报告

---

## Skill 集成报告：`<skill-name>`

### # 概览
- **Skill 描述**：[功能描述]
- **集成目标**：`.sunshine/spec/{target}/`

### # 技术栈兼容性

| Skill 要求 | 项目现状 | 兼容性 |
|-------------------|----------------|---------------|
| [技术 1] | [项目技术] | [OK]/[!]/[X] |

### # 集成位置

| 类型 | 路径 |
|------|------|
| 规范文档 | `.sunshine/spec/{target}/doc.md`（章节：`skill-<name>`） |
| 代码示例 | `.sunshine/spec/{target}/examples/skills/<name>/` |
| 索引更新 | `.sunshine/spec/{target}/index.md` |

> `{target}` = `frontend` 或 `backend`

### # 依赖（如需要）

```bash
# 按包管理器调整，安装所需依赖
npm install <package>
# 或
pnpm add <package>
# 或
yarn add <package>
```

### [OK] 已完成的改动

- [ ] 在 `doc.md` 里加了 `@@@section:skill-<name>` 章节
- [ ] 在 `index.md` 里加了索引条目
- [ ] 在 `examples/skills/<name>/` 里创建了示例文件
- [ ] 示例文件用了 `.template` 后缀

### # 相关规范

- [已有的相关章节 ID]

---

## 6. 可选：创建使用命令

如果这个 skill 经常用，创建一个快捷命令：

```bash
/ use-<skill-name> Use <skill-name> skill following project guidelines
```

## 常见 skill 集成参考

| Skill | 集成目标 | 示例目录 |
|-------|-------------------|-------------------|
| `frontend-design` | `frontend` | `examples/skills/frontend-design/` |
| `mcp-builder` | `backend` | `examples/skills/mcp-builder/` |
| `webapp-testing` | `frontend` | `examples/skills/webapp-testing/` |
| `doc-coauthoring` | `.sunshine/` | N/A（仅文档工作流） |

## 示例：集成 `mcp-builder` skill

### 目录结构

```
.sunshine/spec/backend/
|-- doc.md                           # 添加 MCP 章节
|-- index.md                         # 更新索引
+-- examples/
    +-- skills/
        +-- mcp-builder/
            |-- README.md
            |-- server.ts.template
            |-- tools.ts.template
            +-- types.ts.template
```

### doc.md 里新增的章节

```markdown
@@@section:skill-mcp-builder
## # MCP Server 开发指南

### 概览
用 MCP（Model Context Protocol）创建可供 LLM 调用的工具服务。

### 项目适配
- 把服务放在专属目录
- 遵循项目现有的 TypeScript 与类型定义约定
- 使用项目的日志系统

### 参考示例
见 `examples/skills/mcp-builder/`

@@@/section:skill-mcp-builder
```
