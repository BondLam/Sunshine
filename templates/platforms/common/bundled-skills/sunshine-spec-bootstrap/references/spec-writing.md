# 规范编写

Sunshine 规范是写给未来 Agent 的编码指导。它们应当说明如何在这个仓库里工作，而不是通用项目可能如何组织。

## 从证据出发写

每条重要规则都应当有以下支撑之一：

- 一个展示首选模式的源码文件。
- 一个展示预期行为的测试文件。
- 一个定义该约定的项目文档。
- 一个跨多个文件重复出现的模式。

只有当短片段能让规则更清晰时才使用。优先链接文件路径并指明符号或行为。

## 文件结构

保持规范树与项目对齐：

- 保留 `index.md` 作为规范目录的导航文件。
- 当开发者会独立查找某个主题时，拆分主题。
- 当分开的文件会重复同一条规则时，合并主题。
- 删除不适用的模板文件。
- 为模板遗漏的重要本地模式新增文件。

## 内容标准

好的规范章节包括：

- 规则适用的时机。
- 应当遵循的本地模式。
- 证明该模式的源码或测试文件。
- 常见错误或反模式。
- 具体且可靠时的验证命令或检查。

避免：

- 占位文案。
- 通用的框架建议。
- 只在某一个 Agent 宿主上可用的工具说明。
- 大段复制的代码块。
- 基于某个偶然实现细节的规则。

## 示例形态

```markdown
## Command Handlers

Command handlers should keep argument parsing, validation, and side effects separate. The local pattern is:

- Parse CLI flags at the command boundary.
- Convert raw inputs into typed task options before invoking core logic.
- Keep filesystem writes in the command or service layer, not in template helpers.

Reference files:
- `packages/cli/src/commands/example.ts`
- `packages/cli/test/commands/example.test.ts`

Avoid passing raw `process.argv` or unvalidated config objects into shared helpers.
```

## 收尾检查

完成之前：

```bash
grep -R "To be filled\|TODO: fill\|placeholder" .sunshine/spec
```

同时检查链接、索引文件，以及是否还有规范描述的是模板而不是本仓库。
