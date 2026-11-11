# 开发工作流

---

## 核心原则

1. **先规划再写代码**——动手之前先想清楚要做什么
2. **规范靠注入、不靠记忆**——指南通过 hook/skill 注入，而不是凭记忆回忆
3. **一切落盘**——研究、决策、教训都写进文件；对话会被压缩，文件不会
4. **增量开发**——一次只做一个任务
5. **沉淀所学**——每个任务结束后复盘，把新知识写回规范

---

## Sunshine 系统

### 开发者身份

首次使用时初始化你的身份：

```bash
sunshine init -u <你的名字>
```

生成 `.sunshine/.developer`（已加入 gitignore）+ `.sunshine/workspace/<你的名字>/`。

### 规范系统

`.sunshine/spec/` 按包与分层组织编码规范。

- `.sunshine/spec/<包名>/<分层>/index.md`——入口，含**开发前检查清单** + **质量检查**。真正的指南在它指向的 `.md` 文件里。
- `.sunshine/spec/guides/index.md`——跨包的思考指南。

```bash
sunshine task context --mode packages   # 列出包 / 分层
```

**何时更新规范**：发现了新模式/新约定 · 修 Bug 时要把防重犯的办法固化下来 · 有了新的技术决策。

### 任务系统

每个任务在 `.sunshine/tasks/{MM-DD-名称}/` 下有自己的目录，内含 `task.json`、`prd.md`、可选的 `design.md`、可选的 `implement.md`、可选的 `research/`，以及给支持子代理的平台用的上下文清单（`implement.jsonl`、`check.jsonl`）。

```bash
# 任务生命周期
sunshine task create "<标题>" [--slug <名称>] [--parent <父目录>]
sunshine task start <名称>          # 设为当前任务（有会话身份时按会话隔离）
sunshine task current --source      # 显示当前任务及来源
sunshine task finish                # 清空当前任务（触发 after_finish hook）
sunshine task archive <名称>        # 移入 archive/{年-月}/
sunshine task list [--mine] [--status <状态>]
sunshine task list-archive

# 代码规范上下文（通过 JSONL 注入 implement/check 子代理）。
# `implement.jsonl` / `check.jsonl` 在 `task create` 时为支持子代理的
# 平台预置（空）；AI 在规划期间整理出真正的规范与研究条目。清单仍为空时
# `validate` 失败、`start` 拒绝——子代理会在零规范上下文的情况下运行。
# 确实有意如此时传 `start --allow-empty-context`。
sunshine task add-context <名称> <操作> <文件> <原因>
sunshine task list-context <名称> [操作]
sunshine task validate <名称>

# 任务元数据
sunshine task set-branch <名称> <分支>
sunshine task set-base-branch <名称> <分支>    # PR 目标分支
sunshine task set-scope <名称> <范围>

# 层级（父/子任务）
sunshine task add-subtask <父任务> <子任务>
sunshine task remove-subtask <父任务> <子任务>

# 创建 PR
sunshine task create-pr [名称] [--dry-run]
```

> 运行 `sunshine task --help` 查看权威的最新命令列表。

**当前任务机制**：`task create` 创建任务目录，并在会话身份可用时自动设置按会话隔离的当前任务指针，让规划面包屑立即生效。`task start` 写同一个指针（已设置时幂等），并把 `task.json.status` 从 `planning` 翻转为 `in_progress`。状态存在 `.sunshine/.runtime/sessions/` 下。如果 hook 输入、`SUNSHINE_CONTEXT_ID` 或平台原生的会话环境变量都提供不了上下文键，就没有当前任务，`task start` 会报会话身份提示并失败。`task finish` 删除当前会话文件（状态不变）。`task archive <任务>` 写 `status=completed`，把目录移入 `archive/`，并删除仍指向已归档任务的运行时会话文件。

### 工作区系统

在 `.sunshine/workspace/<开发者>/` 下记录每一次 AI 会话，用于跨会话追踪。

- `journal-N.md`——会话日志。**每个文件最多 2000 行**；超限时自动新建 `journal-(N+1).md`。
- `index.md`——个人索引（会话总数、最近活跃时间）。

```bash
sunshine task add-session --title "标题" --commit "提交哈希" --summary "摘要"
```

### 上下文命令

```bash
sunshine task context                            # 完整会话运行时信息
sunshine task context --mode packages            # 可用的包 + 规范分层
sunshine task context --mode phase --step <X.Y>  # 某工作流步骤的详细指南
```

---

<!--
  WORKFLOW-STATE 面包屑契约（编辑下面的标签块之前先读这段）

  下面 ## 阶段索引一节里内嵌的 [workflow-state:STATUS] 块，是每个受支持
  的 AI 平台的 UserPromptSubmit hook 每轮读取的 `<workflow-state>` 面包屑
  的唯一真实来源。`sunshine hook workflow-state`（Go 实现）只解析它们，
  脚本里没有内置的兜底文案。

  STATUS 字符集：[A-Za-z0-9_-]+。hook 找不到标签时会退化为一句通用的
  “请参考 workflow.md 查看当前步骤。”——特意写得显眼，好让用户发现
  workflow.md 坏了并去修复。

  不变量（test/regression.test.ts）：
    每个标记为 `[required · once]` 的 workflow walkthrough 步骤，在其
    所在阶段的 [workflow-state:*] 块里都必须有一句对应的执行提示。
    面包屑是唯一的每轮通道；强制步骤在这里没被提到，AI 就会悄悄跳过
    （第 1 阶段规划关卡跳过、第 3.4 步提交跳过，都是由此产生的）。

  标签 ↔ 阶段对应关系：
    [workflow-state:no_task]      → 无当前任务；第 1 阶段之前
    [workflow-state:task_error]   → 当前任务记录不可读；先修复再继续
    [workflow-state:planning]     → 全部第 1 阶段（status='planning'）
    [workflow-state:planning-inline] → 第 1 阶段的 Codex inline 变体
    [workflow-state:in_progress]  → 第 2 阶段 + 第 3.2-3.4 阶段
                                    （从 task start 到 task archive，
                                    status 一直是 'in_progress'）
    [workflow-state:in_progress-inline] → 第 2/3 阶段的 Codex inline 变体
    [workflow-state:completed]    → 目前是死代码：archive 命令在同一
                                    次调用里既翻转 status 又移动目录，
                                    解析器随即丢失指针（保留该块，供
                                    未来显式的 in_progress→completed
                                    状态迁移使用）

  编辑检查清单：
    - 改动 [workflow-state:STATUS] 块时，同步检查对应阶段的
      `[required · once]` walkthrough 步骤是否一致
    - 编辑后运行 `sunshine update`，把新内容推送到下游用户项目
      （块级托管替换）
    - 完整运行时契约：
      .sunshine/spec/cli/backend/workflow-state-contract.md
-->

## 阶段索引

```
第 1 阶段：规划   → 分类、征得创建任务的同意，然后写规划产物
第 2 阶段：执行   → 任务状态变为 in_progress 后才能动手实现
第 3 阶段：收尾   → 核查、更新规范、提交、收尾
```

### 需求分流

- 简单对话或小任务：只问这一轮是否要创建一个 Sunshine 任务。用户说不，就跳过本会话的 Sunshine。
- 复杂任务：问是否可以创建一个 Sunshine 任务并进入规划。用户说不，就不要大范围直接写代码；先解释、澄清范围，或建议拆成更小的任务。
- 用户同意创建任务，不等于同意开始实现。规划仍然要先做。

### 规划产物

- `prd.md`——需求、约束与验收标准。不要在这里写技术设计或执行检查清单。
- `design.md`——复杂任务的技术设计：边界、契约、数据流、权衡、兼容性、上线/回滚形态。
- `implement.md`——复杂任务的执行计划：有序检查清单、验证命令、复核关卡与回滚点。
- `implement.jsonl` / `check.jsonl`——子代理上下文的规范与研究清单。它们不能替代 `implement.md`。
- 轻量任务可以只有 PRD。复杂任务在 `task start` 之前必须有 `prd.md`、`design.md` 和 `implement.md`。

### 父 / 子任务树

一个用户请求包含多个可独立验证的交付物时，用父任务。父任务拥有原始需求集合、任务地图、跨子任务的验收标准与最终集成复核；除非父任务自身也有直接工作，否则它通常不是实现目标。

可独立规划、实现、检查、归档的交付物，用子任务。父/子结构不是依赖系统：如果一个子任务必须等另一个，先把顺序写进子任务的 `prd.md` / `implement.md`，并保持每个子任务的验收标准可测试。

用 `sunshine task create "<标题>" --slug <名称> --parent <父目录>` 创建新子任务。用 `sunshine task add-subtask <父任务> <子任务>` 关联已有任务，用 `sunshine task remove-subtask <父任务> <子任务>` 解除误关联。

<!-- 每轮面包屑：无当前任务时显示（第 1 阶段之前） -->

[workflow-state:no_task]
无当前任务。先对本轮请求分类，征得创建任务的同意，再创建任何 Sunshine 任务。
简单对话 / 小任务：只问这一轮是否要创建一个 Sunshine 任务。用户说不，就跳过本会话的 Sunshine。
复杂任务：问用户是否可以创建一个 Sunshine 任务并进入规划阶段。用户说不，就先解释、澄清范围，或建议拆成更小的任务。
[/workflow-state:no_task]

<!-- 每轮面包屑：当前任务记录不可读时显示。 -->

[workflow-state:task_error]
当前任务记录不可读。不要创建或激活另一个任务。
检查上面给出的任务目录，修复它的 task.json。它必须是一个有效的 JSON 对象，且 status 非空。
保留已有的任务字段与产物。如果无法安全地确定正确状态，重建记录前先问用户。
[/workflow-state:task_error]

### 第 1 阶段：规划
- 1.0 创建任务 `[required · once]`（征得创建任务的同意后才能创建）
- 1.1 需求探索 `[required · repeatable]`（写 `prd.md`；复杂任务还要 `design.md` + `implement.md`；先定设计先行还是直接开发）
- 1.2 任务拆解 `[optional · once]`（多个可独立验证的交付物时，拆成父任务加子任务）
- 1.3 研究 `[optional · repeatable]`
- 1.4 配置上下文 `[required · once]`——Claude Code、Cursor、OpenCode、Codex、Kiro、Gemini、Qoder、CodeBuddy、Copilot、Droid、Pi、Oh My Pi、ZCode、Snow、Reasonix、Grok、Kimi Code（仅支持子代理派发的平台；inline 平台跳过）
- 1.5 激活任务 `[required · once]`（需求评审，然后 `task start`；status → in_progress）
- 1.6 完成标准

<!-- 每轮面包屑：在整个第 1 阶段显示（status='planning'） -->

[workflow-state:planning]
加载 `sunshine-brainstorm`；留在规划阶段。
轻量任务：`prd.md` 可能就够了。复杂任务：写完 `prd.md`、`design.md` 和 `implement.md`；`task start` 之前先请用户复核。
多交付物范围：考虑父任务加多个可独立验证的子任务；依赖必须写进子任务产物，不能只靠树的位置暗示。
子代理模式：开始前把 `implement.jsonl` 和 `check.jsonl` 整理成规范/研究清单。
[/workflow-state:planning]

<!-- 每轮面包屑：codex.dispatch_mode=inline 时在整个第 1 阶段显示。
     Codex 专用的可选替代方案，对应 [workflow-state:planning]。主代理
     在第 2 阶段直接改代码，因此跳过 jsonl 整理——
     inline 工作流改用 `sunshine-before-dev` 加载上下文，而不是向子代理注入 JSONL。 -->

[workflow-state:planning-inline]
本分支仅 Codex 且 dispatch_mode=inline 时可用；Claude Code、Cursor 等支持子代理的平台一律走 [workflow-state:planning]，不得用本分支。
加载 `sunshine-brainstorm`；留在规划阶段。
轻量任务：`prd.md` 可能就够了。复杂任务：写完 `prd.md`、`design.md` 和 `implement.md`；`task start` 之前先请用户复核。
多交付物范围：考虑父任务加多个可独立验证的子任务；依赖必须写进子任务产物，不能只靠树的位置暗示。
Inline 模式：跳过 jsonl 整理；第 2 阶段通过 `sunshine-before-dev` 读产物/规范。
[/workflow-state:planning-inline]

### 第 2 阶段：执行
- 2.1 实现 `[required · repeatable]`
- 2.2 质量检查 `[required · repeatable]`
- 2.3 回滚 `[on demand]`

<!-- 每轮面包屑：status='in_progress' 时显示。
     范围：全部第 2 阶段 + 第 3.2-3.4 阶段（从 task start 到 task archive，
     status 一直是 'in_progress'；只有 archive 会翻转它）。因此正文
     必须覆盖从实现到提交的每一个必做步骤，包括第 3.3 步规范更新与
     第 3.4 步提交。 -->

子代理派发协议适用于所有平台与所有子代理，包括原生的 Codex `SubagentStart` 上下文注入（带子代理侧拉取兜底）、二类 Gemini/Qoder/Copilot/Reasonix/Trae/Grok/Kimi Code、hook 支撑的 ZCode/Snow，以及 `sunshine-research`：每次派发提示词都以 `Active task: <task current 给出的任务路径>` 开头，再写角色专属指令。在 Grok Build 上用 `spawn_subagent`，`subagent_type` 设为 Sunshine 子代理名（例如 `sunshine-implement`）。在 Kimi Code 上派发内置的 `coder` / `explore` 子代理，并附上对应的 `.kimi-code/skills/sunshine-<角色>/SKILL.md` 指令。

[workflow-state:in_progress]
工具：`sunshine-implement` / `sunshine-research` 只是子代理类型（Task/Agent 工具，不是 Skill；没有同名的 skill）。`sunshine-update-spec` 是 skill。`sunshine-check` 两者都是；改完代码后核查时优先用 Agent 形态。
流程：`sunshine-implement` -> `sunshine-check` -> `sunshine-update-spec` -> 提交（第 3.4 步）-> `/sunshine:finish-work`。
主会话默认：派发 implement/check 子代理。不得以任务小、改动少为由改走 inline；inline 分支只属于 Codex（见 [workflow-state:in_progress-inline]）。子代理自豁免：如果自己已经是 `sunshine-implement`，不要再派生 `sunshine-implement` 或 `sunshine-check`；如果自己已经是 `sunshine-check`，不要再派生 `sunshine-check` 或 `sunshine-implement`。派发只由主会话做。
派发提示词以 `Active task: <task current 给出的任务路径>` 开头。读上下文的顺序：jsonl 条目 -> `prd.md` -> `design.md（如有）` -> `implement.md（如有）`。
[/workflow-state:in_progress]

<!-- 每轮面包屑：codex.dispatch_mode=inline 且 status='in_progress' 时显示。
     Codex 专用的可选替代方案，对应 [workflow-state:in_progress]。
     主会话直接改代码，不派发子代理。 -->

[workflow-state:in_progress-inline]
本分支仅 Codex 且 dispatch_mode=inline 时可用；Claude Code、Cursor 等支持子代理的平台一律走 [workflow-state:in_progress] 派发 implement/check 子代理，不得以任务小、改动少为由改走 inline。
流程：`sunshine-before-dev` -> 改代码 -> `sunshine-check` -> 验证 -> `sunshine-update-spec` -> 提交（第 3.4 步）-> `/sunshine:finish-work`。
Inline 模式下不要派发 implement/check 子代理。
读上下文的顺序：`prd.md` -> `design.md（如有）` -> `implement.md（如有）`，再加上 skill 加载的相关规范/研究。
[/workflow-state:in_progress-inline]

### 第 3 阶段：收尾
- 3.2 调试复盘 `[on demand]`
- 3.3 规范更新 `[required · once]`
- 3.4 提交改动 `[required · once]`
- 3.5 收尾提醒

> 注意：步骤 3.1 已并入 2.2（最后一轮的全范围检查）与 3.4（提交前导说明）。编号保持不变，以免破坏外部引用。

<!-- 每轮面包屑：status='completed' 时显示。
     正常流程里目前是死代码：archive 命令在同一次调用里把 status 写成
     'completed' 并移动任务目录，当前任务解析器随即丢失指针，hook 永远
     不会在已归档任务上触发。保留该块，供未来的状态迁移 redesign 使用
     （例如显式的 in_progress→completed 命令）。通过与在线块相同的
     规范渠道编辑。 -->

[workflow-state:completed]
代码已提交。运行 `/sunshine:finish-work`；如果工作区还脏，先回第 3.4 步。
[/workflow-state:completed]

### 规则

1. 先确认自己在哪个阶段，再从该阶段的下一步继续
2. 每个阶段内按顺序执行步骤；`[required]` 步骤不能跳过
3. 阶段可以回退（例如执行中发现 prd 缺陷 → 回到规划修复，再重新进入执行）
4. 标记 `[once]` 的步骤如果产物已存在就跳过；不要重复执行
5. 产物是否存在决定下一步；缺少 `design.md` / `implement.md` 对轻量任务是正常的，对复杂任务则表示规划未完成。

### 当前任务路由

用户请求落在当前任务的以下意图之一时，先按意图路由，再按需加载详细的阶段步骤。

[Claude Code, Cursor, OpenCode, codex-sub-agent, Kiro, Gemini, Qoder, CodeBuddy, Copilot, Droid, Pi, Oh My Pi, ZCode, Snow, Reasonix, Trae, Grok, Kimi Code]

- 规划或需求不清 -> `sunshine-brainstorm`。
- `in_progress` 的实现/检查 -> 派发 `sunshine-implement` / `sunshine-check`。
- 反复调试 -> `sunshine-break-loop`；规范更新 -> `sunshine-update-spec`。

[/Claude Code, Cursor, OpenCode, codex-sub-agent, Kiro, Gemini, Qoder, CodeBuddy, Copilot, Droid, Pi, Oh My Pi, ZCode, Snow, Reasonix, Trae, Grok, Kimi Code]

[codex-inline, Kilo, Antigravity, Devin, DeepSeek Harness]

- 规划或需求不清 -> `sunshine-brainstorm`。
- 改代码前 -> `sunshine-before-dev`；改完后 -> `sunshine-check`。
- 反复调试 -> `sunshine-break-loop`；规范更新 -> `sunshine-update-spec`。

[/codex-inline, Kilo, Antigravity, Devin, DeepSeek Harness]

### 护栏

- 创建任务的批准不是实现批准；实现要等产物复核通过并运行 `task start` 之后。
- 轻量任务只有 PRD 是可以的；复杂任务需要 `design.md` + `implement.md`。
- 规划必须落盘到任务产物；汇报完成之前必须先跑检查。

### 加载步骤详情

每个步骤执行时，运行以下命令获取详细指南：

```bash
sunshine task context --mode phase --step <步骤>
# 例如：sunshine task context --mode phase --step 1.1
```

---

## 第 1 阶段：规划

目标：给请求分类，在需要任务时征得创建任务的同意，并产出实现前必需的规划产物。

#### 1.0 创建任务 `[required · once]`

征得创建任务的同意后才能创建任务目录。该命令把 status 设为 `planning`、写 `task.json`、创建默认的 `prd.md`，并在会话身份可用时自动指向新任务：

```bash
sunshine task create "<任务标题>" --slug <名称>
```

`--slug` 只是人类可读的名称。**不要**带 `MM-DD-` 日期前缀；`task create` 会自动加上。

任务树先创建父任务，再用 `--parent <父目录>` 创建每个子任务。不要因为子任务存在就启动父任务；启动拥有下一个可独立验证交付物的那个子任务。

命令成功后，每轮面包屑自动切换到 `[workflow-state:planning]`，提示 AI 留在规划阶段。

这里只运行 `create`——不要顺手运行 `start`。`start` 会把 status 翻转为 `in_progress`，在规划产物复核之前就把面包屑切到实现阶段。`start` 留到步骤 1.5。

`sunshine task current --source` 已指向某个任务时跳过本步。

#### 1.1 需求探索 `[required · repeatable]`

加载 `sunshine-brainstorm` skill，按 skill 的指南与用户交互式探索需求。

**先定模式**：这次走设计先行还是直接开发——问用户一句，或看 `doc/` 里是否已有设计文档；开场提示词里已说清的不重复问。
- **设计先行**：需求澄清 → 设计文档落盘 `doc/` → 1.2 任务拆解 → 子任务各自走 1.1→1.5 → 开发。
- **直接开发**：没有设计环节，需求澄清 → 写 `prd.md`（轻量）或 `prd.md` + `design.md` + `implement.md`（复杂）→ 1.4 配上下文 → 1.5 激活 → 开发。

brainstorm skill 会引导你：
- 一次只问一个问题
- 能调研的优先调研，而不是问用户
- 优先给选项，而不是开放式提问
- 每次用户回答后立即更新 `prd.md`
- 范围太大时走 1.2 任务拆解
- `prd.md` 只聚焦需求与验收标准
- 复杂任务在实现开始前产出 `design.md` 与 `implement.md`

**设计文档落盘**：设计阶段产出的文档（需求列表、概要设计、高保真原型等，按项目与设计者自定，有没有都行）落盘在项目 `doc/`，不在 `.sunshine`；首次落盘时创建 `doc/` 目录。`doc/` 里有什么就用什么，没有原型就按 PRD 或需求列表拆；什么设计文档都没有就是纯代码任务，走直接开发。产品设计文档与任务目录里的技术设计 `design.md` 不是一回事，别混淆。

需求变化时随时回到本步，修订相关产物。

#### 1.2 任务拆解 `[optional · once]`

当一个任务包含多个可独立验证的交付物，或工作量明显超出单次实现时，拆成父任务加子任务。轻量任务、单交付物任务跳过本步。

**输入**：`prd.md` + `doc/` 里的设计文档（有什么读什么）。

**做法**：
- 读设计文档，定子任务清单：每个子任务独立可验证、有明确验收标准。
- 为每个子任务建任务目录：`sunshine task create "<子任务标题>" --description "<一句话摘要>" --parent <父任务>`；已有任务挂成子任务用 `sunshine task add-subtask <父任务> <子任务>`。
- 父任务记好子任务映射、跨子任务验收标准与最终集成复核。
- 子任务随后各自走 1.1（自己的 `prd.md` / `design.md` / `implement.md`）→ 1.4（自己的 jsonl）→ 1.5（自己的 `task start`）→ 第 2 阶段。

**父/子规则**：
- 一个请求包含多个可独立验证的交付物时，用父任务。
- 父任务拥有原始需求、子任务映射、跨子任务验收标准与最终集成复核。
- 子任务拥有可独立规划、实现、检查、归档的实际交付物。
- 父/子结构不是依赖系统。子任务 B 依赖子任务 A 时，把顺序写进子任务 B 的 `prd.md` / `implement.md`。
- 启动拥有下一个交付物的子任务。除非父任务自身有直接实现工作，否则不要启动父任务。

**完成标准**：每个子任务独立可验证；父任务有子任务映射与跨子任务验收标准；子任务都已建好（task.json 的 parent/children 双向一致）。

#### 1.3 研究 `[optional · repeatable]`

研究可以在需求探索期间的任何时间做。不限于本地代码——可以用任何可用工具（MCP 服务器、skill、网页搜索等）查外部信息，包括第三方库文档、行业实践、API 参考等。

[Claude Code, Cursor, OpenCode, codex-sub-agent, Kiro, Gemini, Qoder, CodeBuddy, Copilot, Droid, Pi, Oh My Pi, ZCode, Snow, Reasonix, Trae, Grok, Kimi Code]

派生 research 子代理：

- **子代理类型**：`sunshine-research`
- **任务描述**：Research <具体问题>
- **关键要求**：研究输出必须落盘到 `{TASK_DIR}/research/`

[/Claude Code, Cursor, OpenCode, codex-sub-agent, Kiro, Gemini, Qoder, CodeBuddy, Copilot, Droid, Pi, Oh My Pi, ZCode, Snow, Reasonix, Trae, Grok, Kimi Code]

[codex-inline, Kilo, Antigravity, Devin, DeepSeek Harness]

直接在主会话里做研究，把结论写进 `{TASK_DIR}/research/`。`codex-inline` 是明确把工作留在主会话的模式。

[/codex-inline, Kilo, Antigravity, Devin, DeepSeek Harness]

**研究产物约定**：
- 每个研究主题一个文件（例如 `research/auth-library-comparison.md`）
- 把第三方库用法示例、API 参考、版本约束记进文件
- 记下发现的相关规范文件路径，供后续参考

头脑风暴与研究可以自由交错——暂停下来研究一个技术问题，再回来继续与用户对话。

**关键原则**：研究输出必须写进文件，不能只留在对话里。对话会被压缩，文件不会。

#### 1.4 配置上下文 `[required · once]`

[Claude Code, Cursor, OpenCode, codex-sub-agent, Kiro, Gemini, Qoder, CodeBuddy, Copilot, Droid, Pi, Oh My Pi, ZCode, Snow, Reasonix, Trae, Grok, Kimi Code]

整理 `implement.jsonl` 与 `check.jsonl`，让第 2 阶段的子代理拿到正确的规范/研究上下文。这两个文件在 `task create` 时已预置了一行自描述的 `_example`；你的工作是在这里填入真实条目。

**位置**：`{TASK_DIR}/implement.jsonl` 与 `{TASK_DIR}/check.jsonl`（已存在）。

**格式**：每行一个 JSON 对象——`{"file": "<路径>", "reason": "<原因>"}`。路径相对于仓库根目录。

**放什么**：
- **规范文件**——`.sunshine/spec/<包名>/<分层>/index.md`，以及与本任务相关的具体指南文件（`error-handling.md`、`conventions.md` 等）
- **研究文件**——子代理需要查阅的 `{TASK_DIR}/research/*.md`
- **设计文档**——`doc/` 下与本任务相关的设计产物（需求列表、概要设计、高保真原型等），只放本任务用得上的那几份，不用把整个 `doc/` 列进来

**不要放什么**：
- 代码文件（`src/**`、`packages/**/*.ts` 等）——子代理在实现期间自己读，不用预先登记
- 你将要修改的文件——同理

**两个文件的分工**：
- `implement.jsonl` → implement 子代理写代码需要的规范 + 研究
- `check.jsonl` → check 子代理需要的规范（质量指南、检查约定，研究相同也可放）

这些清单不能替代 `implement.md`。`implement.md` 是复杂任务的人类可读执行计划；jsonl 文件只列出要注入或加载的上下文文件。

**如何发现相关规范**：

```bash
sunshine task context --mode packages
```

列出每个包及其规范分层与路径。挑与本任务领域匹配的条目。

**如何追加条目**：

直接在编辑器里改 jsonl 文件，或者用：

```bash
sunshine task add-context "$TASK_DIR" implement "<路径>" "<原因>"
sunshine task add-context "$TASK_DIR" check "<路径>" "<原因>"
```

有了真实条目后删掉预置的 `_example` 行（可选——消费者会自动跳过它）。

就绪关卡：`task start` 之前，`implement.jsonl` 与 `check.jsonl` 都必须至少含一条真实的 `{"file": "...", "reason": "..."}` 条目。只剩预置 `_example` 行不算就绪。

两个文件已有真实整理条目时跳过本步。

[/Claude Code, Cursor, OpenCode, codex-sub-agent, Kiro, Gemini, Qoder, CodeBuddy, Copilot, Droid, Pi, Oh My Pi, ZCode, Snow, Reasonix, Trae, Grok, Kimi Code]

[codex-inline, Kilo, Antigravity, Devin, DeepSeek Harness]

跳过本步。第 2 阶段由 `sunshine-before-dev` skill 直接加载上下文。

[/codex-inline, Kilo, Antigravity, Devin, DeepSeek Harness]

#### 1.5 激活任务 `[required · once]`

**需求评审**：激活前把整包规划产物列出来请用户确认——`prd.md`、设计文档清单（`doc/` 里与本任务相关的那几份）、拆解出的子任务（走了 1.2 时）。确认后才往下走。

产物复核通过后，把任务状态翻转为 `in_progress`：

```bash
sunshine task start <任务目录>
```

轻量任务 `prd.md` 可能就够了。复杂任务开始前必须存在 `prd.md`、`design.md` 与 `implement.md` 并已复核。在支持子代理派发的平台上，`implement.jsonl` 与 `check.jsonl` 开始前都必须有真实整理条目。运行时消费者出于兼容会容忍缺失或只有预置行的清单，但那不是规划就绪状态。

父任务不激活（除非自身有直接实现工作）：逐个激活子任务；全部子任务完成后，父任务做最终集成复核。

命令成功后，面包屑自动切换到 `[workflow-state:in_progress]`，随后进入第 2 / 3 阶段。

如果 `task start` 报会话身份错误（hook 输入、`SUNSHINE_CONTEXT_ID` 或平台原生会话环境变量都没提供上下文键），按错误里的提示配好会话身份再重试。

#### 1.6 完成标准

| 条件 | 是否必需 |
|------|:---:|
| `prd.md` 存在 | ✅ |
| 用户确认任务可以进入实现 | ✅ |
| 已运行 `task start`（status = in_progress） | ✅ |
| `research/` 有产物（复杂任务） | 推荐 |
| `design.md` 存在（复杂任务） | ✅ |
| `implement.md` 存在（复杂任务） | ✅ |

[Claude Code, Cursor, OpenCode, codex-sub-agent, Kiro, Gemini, Qoder, CodeBuddy, Copilot, Droid, Pi, Oh My Pi, ZCode, Snow, Reasonix, Trae, Grok, Kimi Code]

| `implement.jsonl` 与 `check.jsonl` 各至少含一条真实整理条目（预置行不算） | ✅ |

[/Claude Code, Cursor, OpenCode, codex-sub-agent, Kiro, Gemini, Qoder, CodeBuddy, Copilot, Droid, Pi, Oh My Pi, ZCode, Snow, Reasonix, Trae, Grok, Kimi Code]

---

## 第 2 阶段：执行

目标：把复核通过的规划产物变成通过质量检查的代码。

#### 2.1 实现 `[required · repeatable]`

[Claude Code, Cursor, OpenCode, codex-sub-agent, CodeBuddy, Droid, Pi, ZCode, Snow, Oh My Pi]

派生 implement 子代理：

- **子代理类型**：`sunshine-implement`
- **任务描述**：按复核通过的任务产物实现，查阅 `{TASK_DIR}/research/` 下的材料；最后运行项目 lint 与类型检查
- **派发提示词守则**：提示词必须以 `Active task: <任务路径>` 开头，然后告诉被派生的子代理：它已经是 `sunshine-implement` 子代理，直接实现，不要再派生 `sunshine-implement` / `sunshine-check`。

平台 hook/插件自动处理：
- 读 `implement.jsonl`，把引用的规范/研究文件注入子代理提示词
- 注入 `prd.md`、有则注入 `design.md`、有则注入 `implement.md`
- Codex 上 `SubagentStart` 提供原生上下文注入；子代理配置保留子代理侧加载作为兜底

[/Claude Code, Cursor, OpenCode, codex-sub-agent, CodeBuddy, Droid, Pi, ZCode, Snow, Oh My Pi]

[Gemini, Qoder, Copilot, Reasonix, Trae, Grok, Kimi Code]

派生 implement 子代理：

- **子代理类型**：`sunshine-implement`
- **任务描述**：按复核通过的任务产物实现，查阅 `{TASK_DIR}/research/` 下的材料；最后运行项目 lint 与类型检查
- **派发提示词守则**：提示词必须以 `Active task: <任务路径>` 开头，然后明确告诉被派生的子代理：它已经是 `sunshine-implement`，直接实现，不要派生另一个 `sunshine-implement` / `sunshine-check`。

拉取式子代理定义自动处理上下文加载要求：
- 用 `task current --source` 解析当前任务，再读 `prd.md`、有则读 `design.md`、有则读 `implement.md`
- 读 `implement.jsonl`，要求子代理在写代码前加载每个引用的规范/研究文件

[/Gemini, Qoder, Copilot, Reasonix, Trae, Grok, Kimi Code]

[Kiro]

派生 implement 子代理：

- **子代理类型**：`sunshine-implement`
- **任务描述**：按复核通过的任务产物实现，查阅 `{TASK_DIR}/research/` 下的材料；最后运行项目 lint 与类型检查
- **派发提示词守则**：告诉被派生的子代理：它已经是 `sunshine-implement` 子代理，直接实现，不要再派生 `sunshine-implement` / `sunshine-check`。

平台前导自动处理上下文加载要求：
- 读 `implement.jsonl`，把引用的规范/研究文件注入子代理提示词
- 注入 `prd.md`、有则注入 `design.md`、有则注入 `implement.md`

[/Kiro]

[codex-inline, Kilo, Antigravity, Devin, DeepSeek Harness]

1. 加载 `sunshine-before-dev` skill 阅读项目指南
2. 读 `{TASK_DIR}/prd.md`，有则读 `design.md`，有则读 `implement.md`
3. 查阅 `{TASK_DIR}/research/` 下的材料
4. 按复核通过的产物实现代码
5. 运行项目 lint 与类型检查

[/codex-inline, Kilo, Antigravity, Devin, DeepSeek Harness]

#### 2.2 质量检查 `[required · repeatable]`

[Claude Code, Cursor, OpenCode, codex-sub-agent, Kiro, Gemini, Qoder, CodeBuddy, Copilot, Droid, Pi, Oh My Pi, ZCode, Snow, Reasonix, Trae, Grok, Kimi Code]

派生 check 子代理：

- **子代理类型**：`sunshine-check`
- **任务描述**：对照规范与任务产物复核全部代码改动；发现的问题直接修复；确保 lint 与类型检查通过
- **派发提示词守则**：提示词必须以 `Active task: <任务路径>` 开头，然后告诉被派生的子代理：它已经是 `sunshine-check` 子代理，直接复核/修复，不要再派生 `sunshine-check` / `sunshine-implement`。

check 子代理的工作：
- 对照规范复核代码改动
- 对照 `prd.md`、有则对照 `design.md`、有则对照 `implement.md` 复核代码改动
- 自动修复发现的问题
- 运行 lint 与类型检查验证

[/Claude Code, Cursor, OpenCode, codex-sub-agent, Kiro, Gemini, Qoder, CodeBuddy, Copilot, Droid, Pi, Oh My Pi, ZCode, Snow, Reasonix, Trae, Grok, Kimi Code]

[codex-inline, Kilo, Antigravity, Devin, DeepSeek Harness]

加载 `sunshine-check` skill，按其指南核查代码：
- 规范合规
- lint / 类型检查 / 测试
- 跨层一致性（改动跨分层时）

发现问题 → 修复 → 再检查，直到全绿。

[/codex-inline, Kilo, Antigravity, Devin, DeepSeek Harness]

**最后一轮（第 3.4 步提交前）**：任务的最后一轮 2.2 必须跑全范围，不能只查最新一轮 implement 的改动。用 `sunshine task context --mode packages` 列出全部受影响的包，再加载每个包的规范索引 Quality Check 一节。这样能发现中途局部 2.2 发现不了的跨层/多包问题。

#### 2.3 回滚 `[on demand]`

- `check` 发现 prd 缺陷 → 回第 1 阶段修 `prd.md`，再重做 2.1
- 实现走偏了 → 回退代码，重做 2.1
- 需要更多研究 → 做研究（同第 1.3 步），把结论写进 `research/`

---

## 第 3 阶段：收尾

目标：确保代码质量、沉淀教训、记录工作。

#### 3.2 调试复盘 `[on demand]`

如果本任务反复调试（同一问题修了多次），加载 `sunshine-break-loop` skill：
- 给根因分类
- 解释为什么之前的修复失败了
- 提出预防办法

目标是沉淀调试教训，让同一类问题不再重现。

#### 3.3 规范更新 `[required · once]`

加载 `sunshine-update-spec` skill，复核本任务是否产生了值得记录的新知识：
- 新发现的模式或约定
- 踩过的坑
- 新的技术决策

相应更新 `.sunshine/spec/` 下的文档。即使结论是“没有要更新的”，也要走一遍判断。

#### 3.4 提交改动 `[required · once]`

**规范同步前导**：起草提交之前先问：本任务修 Bug 或发现的不显而易见的知识，是否应该进 `.sunshine/spec/`，让未来的你（或未来的 AI）不再重蹈覆辙？如果应该，先回第 3.3 步——规范写入属于本次任务的同一批提交，不能事后忘掉。

AI 主导把本任务的代码改动做成批量提交，让 `/finish-work` 之后能干净运行。目标：先产出工作提交，再做账本类提交（归档 + 日志）——绝不交错。

**分步操作**：

1. **查看脏状态**：
   ```bash
   git status --porcelain
   ```
   给每个脏路径做快照。工作区干净时直接跳到 3.5。

2. **从最近历史学习提交风格**（让起草的消息融入既有风格）：
   ```bash
   git log --oneline -5
   ```
   注意前缀约定（`feat:` / `fix:` / `chore:` / `docs:` …）、语言（中文/English）与长度风格。

3. **把脏文件分成两组**：
   - **本会话 AI 改过的**——本会话里你通过 Edit/Write/Bash 工具调用写过/改过的文件。改了什么、为什么改，你都清楚。
   - **未识别的**——本会话你没动过的脏文件（可能是用户手动改的、上一会话留下的 WIP，或不相关的工作）。不要悄悄把它们也提交进去。

4. **起草提交计划**。把 AI 改过的文件按逻辑归组提交（一个连贯改动单元一个提交，不是一个文件一个提交）。每条写：`<提交消息>` + 文件列表。未识别的文件单独列在底部。

5. **提交计划只展示一次，请求一次性确认**。格式：
   ```
   建议提交（按顺序）：
     1. <消息>
        - <文件>
        - <文件>
     2. <消息>
        - <文件>

   未识别的脏文件（不在任何提交里——请确认包含还是排除）：
     - <文件>
     - <文件>

   回复 'ok' / '行' 执行。回复修改意见，或回复 '我自己来' / 'manual' 中止。
   ```

6. **确认后**：按顺序对每批执行 `git add <文件>` + `git commit -m "<消息>"`。不要 amend。不要 push。

7. **被拒绝时**（用户回复“不行”/“我自己来”/“manual”/对计划有任何异议）：停下。不要尝试第二份计划。用户会手动提交；等他们确认后，你直接跳到 3.5。

**规则**：
- 任何地方都不要 `git commit --amend`——三阶段三提交流程（工作提交 → 归档提交 → 日志提交）。
- 本步绝不 push 到远端。
- 用户只想改消息措辞、接受文件分组时，改消息再确认一次——但如果他们拒绝分组，就退出到手动模式。
- 批量计划只提示一次；不要逐个提交提问。

#### 3.5 收尾提醒

做完上面这些后，提醒用户可以运行 `/finish-work` 收尾（归档任务、记录会话）。

---

## 自定义 Sunshine（用于 fork）

本节写给想修改 Sunshine 工作流本身的开发者。所有自定义都通过编辑本文件完成；脚本只是解析器。

### 改动某个步骤的含义

编辑上面第 1 / 2 / 3 阶段里对应步骤的 walkthrough 正文。关键不变量：
- 没有当前任务时必须先分流，征得创建任务的同意，再创建 Sunshine 任务。
- 规划必须区分轻量 PRD-only 任务与复杂任务（复杂任务在开始前需要 `prd.md`、`design.md` 与 `implement.md`）。
- 每条必需的执行路径在 `/sunshine:finish-work` 之前都必须能到达第 3.4 步提交提醒。

所有标签块都在上面的 `## 阶段索引` 一节里，紧跟各阶段摘要之后：

| 范围 | 对应标签 |
|---|---|
| 无当前任务（第 1 阶段之前） | `[workflow-state:no_task]`（阶段索引 ASCII 图之后） |
| 当前任务记录不可读 | `[workflow-state:task_error]`（先修复已有任务再继续） |
| 全部第 1 阶段（任务已创建 → 可进入实现） | `[workflow-state:planning]`（第 1 阶段摘要之后） |
| Codex inline 第 1 阶段 | `[workflow-state:planning-inline]` |
| 第 2 阶段 + 第 3.2–3.4 阶段（实现 + 检查 + 收尾） | `[workflow-state:in_progress]`（第 2 阶段摘要之后） |
| Codex inline 第 2 阶段 + 第 3.2–3.4 阶段 | `[workflow-state:in_progress-inline]` |
| 第 3.5 步之后（已归档） | `[workflow-state:completed]`（第 3 阶段摘要之后；**目前是死代码**） |

### 改动每轮提示词文本

直接编辑对应 `[workflow-state:STATUS]` 块的正文。编辑后，如果你是模板维护者，运行 `sunshine update`；如果是自定义自己的项目，重启 AI 会话——不需要改脚本。

### 添加自定义状态

新增一个块：

```
[workflow-state:my-status]
你的每轮提示词文本
[/workflow-state:my-status]
```

约束：
- STATUS 字符集：`[A-Za-z0-9_-]+`（下划线与连字符均可，例如 `in-review`、`blocked-by-team`）
- 生命周期 hook 必须把 `task.json.status` 写成你的自定义值，否则该标签永远不会被读取
- 生命周期 hook 在 `task.json.hooks.after_*` 里绑定，事件为 `after_create / after_start / after_finish / after_archive` 之一

### 添加生命周期 hook

给 `task.json` 加一个 `hooks` 字段：

```json
{
  "hooks": {
    "after_finish": [
      "your-script-or-command-here"
    ]
  }
}
```

支持的事件：`after_create / after_start / after_finish / after_archive`。注意 `after_finish` ≠ 状态变更（它只清空当前任务指针）；“任务已完成”的通知请用 `after_archive`。

### 完整契约

工作流状态机的运行时契约、全部状态写入方的位置、伪状态（`no_task` / `stale_<source_type>`）、hook 可达性矩阵等深层细节，见：

- `.sunshine/spec/cli/backend/workflow-state-contract.md`——运行时契约 + 写入方表 + 测试不变量
- `sunshine hook workflow-state`——实际解析器（只读 workflow.md，没有内置文本）
