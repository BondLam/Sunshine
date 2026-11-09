# Channel 驱动的子代理派发工作流

---

## 核心原则

1. **先规划再写代码**——实现之前先定好任务、规划产物与验收标准。
2. **主会话负责协调**——主会话澄清需求、规划任务、派发工作线程（worker）、更新规范、提交并收尾。
3. **实现与检查跑在 channel 工作线程里**——默认用 `sunshine channel spawn` 派发 implement/check 工作线程，而不是宿主原生的子代理。
4. **上下文显式传递**——工作线程上下文顺序是 `jsonl 条目 -> prd.md -> design.md -> implement.md`。
5. **结果可审计**——用 `sunshine channel messages --raw` 看工作线程事件；美化输出只是操作员看板，可能会截断进度。
6. **决策落盘**——需求、研究、计划与复核结论都属于任务文件。

---

## Sunshine 系统

### 开发者身份

首次使用时初始化你的身份：

```bash
sunshine init -u <你的名字>
```

### 规范系统

`.sunshine/spec/` 存放项目工程指南。写代码之前，先加载与任务相关的包/分层规范：

```bash
sunshine task context --mode packages
```

### 任务系统

每个任务在 `.sunshine/tasks/{MM-DD-名称}/` 下有自己的目录，内含 `task.json`、`prd.md`、可选的 `design.md`、可选的 `implement.md`、可选的 `research/`，以及 `implement.jsonl` / `check.jsonl`。

常用命令：

```bash
sunshine task create "<标题>" [--slug <名称>] [--parent <父目录>]
sunshine task start <名称>
sunshine task current --source
sunshine task finish
sunshine task archive <名称>
sunshine task validate <名称>
```

### Channel 系统

Channel 是工作线程协作与事件审计层。临时实现/检查 channel 用 `--ephemeral`。持久讨论板用 `--type forum`；`thread` 是 forum 里的一项。

稳定的工作线程代号：

- `implement`——实现工作线程
- `check`——默认检查工作线程
- `check-cc`——Claude 检查工作线程
- `check-cx`——Codex 检查工作线程

---

<!--
  WORKFLOW-STATE 面包屑契约

  [workflow-state:STATUS] 块是每轮提示词注入的唯一来源。
  不要删除标签或改动标签语法。正文可以改；解析器不应该改。
-->

## 阶段索引

```
第 1 阶段：规划   -> 分类、征得创建任务的同意，然后写规划产物
第 2 阶段：执行   -> 通过 sunshine channel 工作线程实现/检查
第 3 阶段：收尾   -> 核查、更新规范、提交、收尾
```

### 需求分流

- 简单对话或小任务：只问这一轮是否要创建一个 Sunshine 任务。用户说不，就跳过本会话的 Sunshine。
- 复杂任务：问是否可以创建一个 Sunshine 任务并进入规划。用户说不，就不要大范围直接实现。
- 用户同意创建任务，不等于同意开始实现。实现要等产物复核通过并运行 `task start` 之后。

### 规划产物

- `prd.md`——需求、约束与验收标准。
- `design.md`——复杂任务的技术设计。
- `implement.md`——复杂任务的执行计划、验证命令、复核关卡与回滚点。
- `implement.jsonl` / `check.jsonl`——工作线程上下文清单。把规范与研究文件放这里，不放代码文件。

轻量任务可以只有 PRD。复杂任务在 `task start` 之前必须有 `prd.md`、`design.md` 与 `implement.md`。

### 父 / 子任务树

一个请求包含多个可独立验证的交付物时，用父任务。子任务拥有可独立规划、实现、检查、归档的交付物。父/子结构不是依赖系统；依赖必须写进子任务的 `prd.md` / `implement.md`。

[workflow-state:no_task]
无当前任务。先对本轮请求分类，征得创建任务的同意，再创建任何 Sunshine 任务。
简单对话 / 小任务：只问这一轮是否要创建一个 Sunshine 任务。用户说不，就跳过本会话的 Sunshine。
复杂任务：问用户是否可以创建一个 Sunshine 任务并进入规划阶段。用户说不，就先解释、澄清范围，或建议拆成更小的任务。
[/workflow-state:no_task]

### 第 1 阶段：规划

- 1.0 创建任务 `[required · once]`
- 1.1 需求探索 `[required · repeatable]`
- 1.2 研究 `[optional · repeatable]`
- 1.3 配置上下文 `[required · once]`（仅支持子代理派发的平台）
- 1.4 激活任务 `[required · once]`
- 1.5 完成标准

[workflow-state:planning]
加载 `sunshine-brainstorm`；留在规划阶段。
轻量任务：`prd.md` 可能就够了。复杂任务：写完 `prd.md`、`design.md` 和 `implement.md`；`task start` 之前先请用户复核。
多交付物范围：考虑父任务加多个可独立验证的子任务；依赖必须写进子任务产物，不能只靠树的位置暗示。
Channel 工作线程模式：开始前把 `implement.jsonl` 和 `check.jsonl` 整理成规范/研究清单。
[/workflow-state:planning]

[workflow-state:planning-inline]
加载 `sunshine-brainstorm`；留在规划阶段。
轻量任务：`prd.md` 可能就够了。复杂任务：写完 `prd.md`、`design.md` 和 `implement.md`；`task start` 之前先请用户复核。
多交付物范围：考虑父任务加多个可独立验证的子任务；依赖必须写进子任务产物，不能只靠树的位置暗示。
Inline 模式：跳过 jsonl 整理；第 2 阶段通过 `sunshine-before-dev` 读产物/规范。
[/workflow-state:planning-inline]

### 第 2 阶段：执行

- 2.1 实现 `[required · repeatable]`
- 2.2 质量检查 `[required · repeatable]`
- 2.3 回滚 `[on demand]`

Channel 驱动的子代理派发是本工作流的默认执行模型。主会话用 `sunshine channel create`、`sunshine channel spawn`、`sunshine channel send` 与 `sunshine channel wait` 协调工作线程。只有用户明确要求原生派发，或必须用宿主专属能力时，才回退到宿主原生子代理。

[workflow-state:in_progress]
流程：channel 驱动的 `implement` 工作线程 -> channel 驱动的 `check` 工作线程 -> `sunshine-update-spec` -> 提交（第 3.4 步）-> `/sunshine:finish-work`。
主会话默认：用 `sunshine channel spawn` 配合 `.sunshine/agents/implement.md` 与 `.sunshine/agents/check.md`；除非明确要求或宿主专属工具必需，否则不要用原生的 Claude Task / Codex sub_agent。
工作线程上下文顺序：jsonl 条目 -> `prd.md` -> `design.md（如有）` -> `implement.md（如有）`。用 `implement`、`check`、`check-cx`、`check-cc` 这类稳定的工作线程代号；需要精确结果时用 `sunshine channel messages --raw` 读结果。
[/workflow-state:in_progress]

[workflow-state:in_progress-inline]
流程：`sunshine-before-dev` -> 改代码 -> channel 驱动的 `check` 工作线程 -> 验证 -> `sunshine-update-spec` -> 提交（第 3.4 步）-> `/sunshine:finish-work`。
只有用户要求 inline，或改动太小不值得派工作线程时，才允许 inline 实现。改完代码后，优先用 `sunshine channel spawn --agent check` 做独立复核。
改代码前读上下文：`prd.md` -> `design.md（如有）` -> `implement.md（如有）`，再加上 skill 加载的相关规范/研究。
[/workflow-state:in_progress-inline]

### 第 3 阶段：收尾

- 3.2 调试复盘 `[on demand]`
- 3.3 规范更新 `[required · once]`
- 3.4 提交改动 `[required · once]`
- 3.5 收尾提醒

> 注意：步骤 3.1 已并入 2.2（最后一轮的全范围检查）与 3.4（提交前导说明）。编号保持不变，以免破坏外部引用。

[workflow-state:completed]
代码已提交。运行 `/sunshine:finish-work`；如果工作区还脏，先回第 3.4 步。
[/workflow-state:completed]

---

## 规则

1. 先确认当前在哪个阶段，再从该阶段的下一步继续。
2. 每个阶段内按顺序执行步骤；`[required]` 步骤不能跳过。
3. 第 2 阶段默认用 channel 工作线程。除非用户要求 inline，或任务小到够用，否则不要在主会话里直接做大的改动。
4. 工作线程简报必须写明当前任务、目标、可编辑范围、验证命令与禁止事项。
5. `sunshine channel messages --raw` 是精确的审计路径；美化输出只用于快速看状态。
6. 工作线程完成后，主会话整合结果，并在需要时跑检查工作线程。最终判断权在主会话。

### 当前任务路由

[Claude Code, Cursor, OpenCode, codex-sub-agent, Kiro, Gemini, Qoder, CodeBuddy, Copilot, Droid, Pi]

- 规划或需求不清 -> `sunshine-brainstorm`。
- `in_progress` 的实现 -> `sunshine channel spawn --agent implement`。
- `in_progress` 的质量检查 -> `sunshine channel spawn --agent check`。
- 反复调试 -> `sunshine-break-loop`；规范更新 -> `sunshine-update-spec`。

[/Claude Code, Cursor, OpenCode, codex-sub-agent, Kiro, Gemini, Qoder, CodeBuddy, Copilot, Droid, Pi]

[codex-inline, Kilo, Antigravity, Devin]

- 规划或需求不清 -> `sunshine-brainstorm`。
- 改代码前 -> `sunshine-before-dev`；改完后 -> 优先用 channel 驱动的 `check` 工作线程。
- 反复调试 -> `sunshine-break-loop`；规范更新 -> `sunshine-update-spec`。

[/codex-inline, Kilo, Antigravity, Devin]

---

## 第 1 阶段：规划

目标：澄清需求、征得创建任务的同意，并产出实现前必须复核的规划产物。

#### 1.0 创建任务 `[required · once]`

征得创建任务的同意后才能创建任务目录：

```bash
sunshine task create "<任务标题>" --slug <名称>
```

这里只运行 `create`，不要顺手运行 `start`。`start` 会把 status 切到 `in_progress`，面包屑随即进入执行阶段。

#### 1.1 需求探索 `[required · repeatable]`

加载 `sunshine-brainstorm`，把用户需求写进 `prd.md`。复杂任务还要 `design.md` 与 `implement.md`。

要求：

- 一次只问一个问题。
- 能调研的优先调研，而不是问用户。
- 需求变化时立即更新任务产物。
- 把大范围工作拆成父任务 + 子任务。
- `prd.md` 只聚焦需求与验收标准，不写实现检查清单。

#### 1.2 研究 `[optional · repeatable]`

需要研究时，把结果写到 `{TASK_DIR}/research/`。研究文件必须能被后续工作线程使用。

#### 1.3 配置上下文 `[required · once]`

整理工作线程上下文清单：

- `implement.jsonl`——实现工作线程需要的规范与研究。
- `check.jsonl`——检查工作线程需要的质量规范、测试规范与研究。

不要把代码文件放进 jsonl。工作线程在执行期间自己读代码。

#### 1.4 激活任务 `[required · once]`

产物复核通过后，启动任务：

```bash
sunshine task start <任务目录>
```

#### 1.5 完成标准

| 条件 | 是否必需 |
| --- | :---: |
| `prd.md` 存在 | 是 |
| 用户确认任务可以进入实现 | 是 |
| 已运行 `task start` | 是 |
| 复杂任务有 `design.md` | 是 |
| 复杂任务有 `implement.md` | 是 |
| `implement.jsonl` 与 `check.jsonl` 各至少含一条真实整理条目（预置行不算） | 是 |

运行时消费者出于兼容会容忍缺失或只有预置行的清单，但那不是规划就绪状态。

---

## 第 2 阶段：执行

目标：主会话通过 channel 工作线程，把复核通过的规划产物变成通过检查的代码。

#### 2.1 实现 `[required · repeatable]`

[Claude Code, Cursor, OpenCode, codex-sub-agent, Kiro, Gemini, Qoder, CodeBuddy, Copilot, Droid, Pi]

用 channel 驱动的 implement 派发：

```bash
TASK=.sunshine/tasks/<当前任务>
sunshine channel create impl-<主题> --task "$TASK" --by main --ephemeral
sunshine channel spawn impl-<主题> \
  --agent implement \
  --as implement \
  --jsonl "$TASK/implement.jsonl" \
  --file "$TASK/prd.md" \
  --file "$TASK/design.md" \
  --file "$TASK/implement.md" \
  --cwd "$PWD" \
  --timeout 60m
sunshine channel send impl-<主题> --as main --to implement --text-file /tmp/implement-brief.md
sunshine channel wait impl-<主题> --as main --kind done --from implement --timeout 60m
sunshine channel messages impl-<主题> --raw --from implement --last 20
```

文件不存在时，省略对应的 `design.md` 或 `implement.md` 的 `--file`。简报必须写明工作线程目标、禁止事项、验证命令与预期的完成摘要。

只有用户明确要求，或必须用宿主专属能力时，才允许回退到原生子代理。

[/Claude Code, Cursor, OpenCode, codex-sub-agent, Kiro, Gemini, Qoder, CodeBuddy, Copilot, Droid, Pi]

[codex-inline, Kilo, Antigravity, Devin]

1. 加载 `sunshine-before-dev`。
2. 读 `prd.md`，有则读 `design.md`，有则读 `implement.md`。
3. 读相关研究。
4. 小改动可以 inline 实现；大改动仍应用 channel 工作线程。
5. 实现后进入 channel 驱动的检查。

[/codex-inline, Kilo, Antigravity, Devin]

#### 2.2 质量检查 `[required · repeatable]`

[Claude Code, Cursor, OpenCode, codex-sub-agent, Kiro, Gemini, Qoder, CodeBuddy, Copilot, Droid, Pi]

用 channel 驱动的 check 派发：

```bash
TASK=.sunshine/tasks/<当前任务>
sunshine channel create cr-<主题> --task "$TASK" --by main --ephemeral
sunshine channel spawn cr-<主题> \
  --agent check \
  --as check \
  --jsonl "$TASK/check.jsonl" \
  --file "$TASK/prd.md" \
  --file "$TASK/design.md" \
  --file "$TASK/implement.md" \
  --cwd "$PWD" \
  --timeout 30m
sunshine channel send cr-<主题> --as main --to check --text-file /tmp/check-brief.md
sunshine channel wait cr-<主题> --as main --kind done --from check --timeout 30m
sunshine channel messages cr-<主题> --raw --from check --last 40
```

要做跨供应商的独立复核时，在同一个 channel 里派生 `check-cc` 与 `check-cx`：

```bash
sunshine channel spawn cr-<主题> --agent check --provider claude --as check-cc --cwd "$PWD" --timeout 30m
sunshine channel spawn cr-<主题> --agent check --provider codex --as check-cx --cwd "$PWD" --timeout 30m
sunshine channel send cr-<主题> --as main --to check-cc --text-file /tmp/check-brief.md
sunshine channel send cr-<主题> --as main --to check-cx --text-file /tmp/check-brief.md
sunshine channel wait cr-<主题> --as main --kind done --from check-cc,check-cx --all --timeout 30m
```

检查工作线程应该直接修复明确的问题。主会话读原始事件，做最终判断。

[/Claude Code, Cursor, OpenCode, codex-sub-agent, Kiro, Gemini, Qoder, CodeBuddy, Copilot, Droid, Pi]

[codex-inline, Kilo, Antigravity, Devin]

加载 `sunshine-check` 或用 channel 驱动的检查工作线程。发现问题就修复并再检查，直到全绿。

[/codex-inline, Kilo, Antigravity, Devin]

#### 2.3 回滚 `[on demand]`

- 检查发现 PRD 缺陷 -> 回第 1 阶段修产物，再重新执行。
- 实现工作线程跑偏了 -> 收窄简报、重新派发，或回退那部分工作。
- 需要更多研究 -> 写进 `{TASK_DIR}/research/`，再重新派发。

---

## 第 3 阶段：收尾

目标：核查质量、沉淀教训、提交工作。

#### 3.2 调试复盘 `[on demand]`

同一类问题反复出现时，加载 `sunshine-break-loop`，记录根因与预防办法。

#### 3.3 规范更新 `[required · once]`

加载 `sunshine-update-spec`，判断新模式、踩过的坑或技术决策是否应该写回 `.sunshine/spec/`。

#### 3.4 提交改动 `[required · once]`

**规范同步前导**：起草提交之前先问：本任务修 Bug 或发现的不显而易见的知识，是否应该进 `.sunshine/spec/`，让未来的你（或未来的 AI）不再重蹈覆辙？如果应该，先回第 3.3 步——规范写入属于本次任务的同一批提交，不能事后忘掉。

主会话提交工作改动。提交前，把 AI 改过的文件与未识别的脏文件分开。

```bash
git status --porcelain
git log --oneline -5
```

不要 amend。不要 push。

#### 3.5 收尾提醒

提交后，提醒用户运行 `/sunshine:finish-work`，归档任务并记录会话。

---

## 自定义 Sunshine

本工作流通过 `.sunshine/workflow.md` 自定义。脚本解析标签与标题；它们不保存兜底文案。

### 改动某个步骤

编辑对应第 1 / 2 / 3 阶段步骤的正文。

### 改动每轮提示词文本

编辑对应 `[workflow-state:STATUS]` 块的正文。不要改动标签名或语法。

### 添加自定义状态

新增：

```text
[workflow-state:my-status]
...
[/workflow-state:my-status]
```

生命周期 hook 或脚本必须把 `task.json.status` 写成该值，否则该块永远不会被读取。
