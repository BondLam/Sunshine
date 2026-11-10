---
description: "Sunshine Copilot 提示词：before-dev.prompt"
---

开始任务之前，先阅读相关的开发规范。

按以下步骤执行：

1. **阅读当前任务产物**：
   - `prd.md`，了解需求与验收标准
   - `design.md`（如果有），了解技术设计
   - `implement.md`（如果有），了解执行顺序与验证计划

2. **发现包及其规范层**：
   ```bash
   sunshine task context --mode packages
   ```

3. **判断哪些规范适用**，依据：
   - 你要改的是哪个包（例如 `cli/`、`docs-site/`）
   - 工作类型（后端、前端、单测、文档等）
   - 任务产物里引用的规范/调研路径

4. **阅读每个相关模块的规范索引**：
   ```bash
   cat .sunshine/spec/<package>/<layer>/index.md
   ```
   按索引里的 **“开发前检查清单”** 一节执行。

5. **阅读开发前检查清单里列出的、与任务相关的具体规范文件**。索引不是目标——它指向真正的规范文件（例如 `error-handling.md`、`conventions.md`、`mock-strategies.md`）。读这些文件，了解编码标准与模式。

6. **务必阅读共享指南**：
   ```bash
   cat .sunshine/spec/guides/index.md
   ```

7. 理解需要遵循的编码标准与模式，然后继续推进开发计划。

这一步在写任何代码之前都是**强制**的。
