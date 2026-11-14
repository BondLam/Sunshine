# Sunshine 教程：在 agent 界面里工作，终端只跑一次 init

主线一句话：**终端只跑一次 `sunshine init`，然后进 agent 界面（Claude Code 为主）用自然语言提需求，agent 按 `.sunshine/workflow.md` 干活，人检查 diff、验收、提交。**

约定：

- 与 agent 的对话示例都标明是“示例提示词”，不是真实会话记录；展示的命令输出是真实跑出来的摘录，`……` 表示省略的行，不是改写。
- 演示项目不变：**退款原因必填校验**——给退款流程加一个必填的原因字段。例子很小，流程是真的。
- 任务目录名里的 `11-10` 是实跑当天的月-日（2026-11-10），你跑的时候会是当天的日期。
- 版本号随你安装的 CLI 版本而变；本教程用当前 Release **v0.2.1** 验证（`sunshine version` 打印 `Sunshine 版本 0.2.1`）。第 11 节“有新版”的输出来自本地构建的 0.1.0 二进制，仅版本号不同，形态一致，那里会注明。
- 演示用的目录都在 `/tmp/rewrite-zh/demo/` 下面（`demo`、`demo2`、`registry-src`、`channels`、`home`），删掉即可，不影响任何真实项目。

终端命令在本教程里只出现在三处：第 4 节“底下发生了什么”（agent 实际调的命令 + 真实输出）、第 12 节“六个场景”（每个场景的实录）和末尾的附录“命令速查”。其余章节讲的是人在 agent 界面里做什么、agent 按什么干活。

## 1. 终端只跑一次：init

先装好 CLI（安装方法也在 [README.cn.md](../README.cn.md)）：

```sh
curl -fsSL https://raw.githubusercontent.com/linrong007/Sunshine/main/install.sh | sh
```

Windows 请到 [Releases](https://github.com/linrong007/Sunshine/releases) 页下载对应 zip，解压后把 `sunshine.exe` 放进 PATH；已有 Go 的机器也可以 `go install github.com/linrong007/Sunshine/cmd/sunshine@latest`。

确认版本，然后初始化你的项目——这是你在终端里唯一必须跑的一步：

```sh
cd 你的项目
sunshine init -u 你的名字 -y
```

真实输出：

```text
正在配置 Claude Code……
正在配置 Cursor……
初始化完成，开发者：演示者
项目类型：全栈（默认）
已安装平台：
  - Claude Code
  - Cursor
下一步：
  1. 在 agent 界面里打开项目，直接用自然语言提第一个需求；
  2. 规范已按默认模板生成在 .sunshine/spec/（已生成 system.md、frontend.md、backend.md；有团队规范时再补充），工作流按 .sunshine/workflow.md 自动推进，一般不用改。
```

演示目录是空目录，检测不到类型，`-y` 跳过提问后默认按全栈处理，所以上面输出是 `项目类型：全栈（默认）`。init 默认装 Claude Code 与 Cursor，生成中文 `.sunshine/` 骨架与两个平台的配置。

装完这一步，终端的任务就结束了——进 agent 界面提第一个需求（第 2 节）。init 的全部操作（交互提问、常用选项、三种模式、项目类型检测、装了哪些文件）见末尾[附录：命令速查](#附录命令速查)。

## 2. 进 agent 界面：用自然语言提需求

在 Claude Code 里打开这个项目（Codex、Cursor 同理：打开项目，进它的 agent 界面），直接用自然语言说你要什么。不用先学命令，不用先建任务——建任务是 agent 的活。

示例提示词（示例，不是真实会话记录）：

> 给退款流程加一个必填的原因字段：用户提交退款时原因不能为空，否则拒绝提交。原因做成下拉选择（质量问题 / 尺寸不符 / 其他），选“其他”时出现文本框；后端 `POST /refunds` 缺 `reason` 返回 400，错误码 `REASON_REQUIRED`。请按 sunshine 工作流走：先向我提问，把不清楚的产品逻辑和技术细节问清楚；确认后，创建任务、写好 PRD，再动手实现。

注意：提示词里要写明“按 sunshine 工作流走：先提问、确认后再建任务写 PRD”。只给一句裸需求是不够的——实测里 agent 会直接动手写代码，跳过建任务和需求确认。

设计人员和开发人员不是两套工作流，是一套工作流的两个阶段：前者做 1.1 问清需求、产需求列表与原型、1.5 评审、1.2 拆解，后者做第 2 阶段实现与检查、第 3 阶段回写与提交；共用同一份 workflow.md、任务系统与 PRD，交接靠需求列表与任务结构。所以提示词不用点名角色——点名工作流和模式就够了，角色跟着阶段走：任务在设计阶段，agent 就是设计人员；进了开发，就是开发人员。模式没写清，agent 会在 1.1 先问一句。唯一要明确说的是范围边界：只做设计不写代码，或设计已有了只做开发。

agent 收到后会先读 `.sunshine/AGENTS.md`、`.sunshine/workflow.md` 和 `.sunshine/spec/`，再按工作流推进；有不清楚的地方会在界面里反问你，而不是猜。你只需要在对话里回答、确认、验收。

如果需求是在已有项目上改，同样一句话即可，例如：

示例提示词（示例，不是真实会话记录）：

> 退款页再加一个返回按钮，点一下回到订单列表。

## 3. agent 按 workflow.md 干活

agent 按 `.sunshine/workflow.md` 的三个阶段推进（下面是 workflow.md 里的实际阶段名与步骤号，对得上你项目里的文件）：

### 第 1 阶段：规划

- **1.1 需求探索**：先定设计先行还是直接开发，再写 `prd.md`（需求、约束与验收标准）。轻量任务可以只有 PRD；复杂任务还要 `design.md`（技术设计）与 `implement.md`（执行计划），并且在 `task start` 之前先请你复核。设计先行时，设计文档落盘在项目 `doc/`，不在 `.sunshine`。
- **1.2 任务拆解**（可选）：一个任务包含多个可独立验证的交付物时，拆成父任务加子任务；本演示任务只有一个交付物，不走这一步。
- **1.3 研究**（可选）：需要调研时先查资料、看代码，再写设计；本演示任务简单，不走这一步。
- **1.4 配置上下文**：把实现与验收要读的规范、调研文档、设计文档整理进 `implement.jsonl` / `check.jsonl`（每行一个 `{"file": "<路径>", "reason": "<原因>"}`），供 implement/check 子代理读。
- **1.5 激活任务**：需求评审通过后 `task start`，任务进入 `in_progress`。

`task create` 生成的 `prd.md` 只是一个骨架，agent 会在对话里把它填实——下面是刚创建时真实的骨架内容：

```markdown
# 给退款原因加必填校验

## 目标

退款时原因字段必填

## 需求

- 待定

## 验收标准

- [ ] 待定

## 说明

- prd.md 只写需求、约束与验收标准。
- 轻量任务可以只有 PRD。
- 复杂任务在 `sunshine task start` 前补充 design.md（技术设计）与 implement.md（执行计划）。
```

### 第 2 阶段：执行

- **2.1 实现**：主会话默认派发 `sunshine-implement` 子代理写代码（inline 模式仅 Codex 下，主会话直接改代码，不派发子代理）。
- **2.2 质量检查**：派发 `sunshine-check` 子代理按 `check.jsonl` 里的规范与验收标准检查；反复调试同一问题时走 `sunshine-break-loop` 复盘。

### 第 3 阶段：收尾

- **3.3 规范更新**：加载 `sunshine-update-spec` skill，复核本任务是否产生了值得记录的新知识（新发现的模式或约定、踩过的坑、新的技术决策），相应更新 `.sunshine/spec/`。即使结论是“没有要更新的”，也要走一遍判断。
- **3.4 提交改动**：agent 起草提交计划（一个连贯改动单元一个提交），只展示一次、请你一次性确认后提交。
- **`/sunshine:finish-work`**：收尾命令。归档当前任务并记录会话日志。这里不做代码提交——提交在 3.4 已经完成，在调用本命令之前。

整个过程里人只在三处出现：提需求（第 2 节）、复核规划与提交计划（agent 在界面里问你时）、检查验收（第 5 节）。

## 4. 底下发生了什么

上面第 3 节的每一步，agent 在底下调的都是 `sunshine` 命令。下面是演示项目里真实跑出来的命令与输出——你在终端看到的是同一批命令，平时不用自己敲。

### 建任务、整理上下文、启动

```sh
sunshine task create "给退款原因加必填校验" --description "退款时原因字段必填" --priority P1
```

真实输出：

```text
警告：无法解析仓库默认分支（未配 remote 或离线），base_branch 暂记为当前检出分支 "master"，可用 --base-branch 覆盖。
已创建任务：11-10-给退款原因加必填校验

下一步：
  - 在 prd.md 里写清需求与验收标准
  - 轻量任务：只有 PRD 也可以
  - 复杂任务：在 sunshine task start 前补充 design.md 与 implement.md
  - 整理 implement.jsonl / check.jsonl（已建空文件），供子代理在 start 前读规范：
      每行一个 JSON 对象 {"file": "<路径>", "reason": "<原因>"}，只放规范/调研文档
      可用 sunshine task context --mode packages 列出可用规范
  - 用 /sunshine:continue 或阶段上下文决定下一步

.sunshine/tasks/11-10-给退款原因加必填校验
```

任务目录是 `.sunshine/tasks/<月-日-slug>/`，里面的 `task.json` 有 24 个字段（顺序与默认值固定，归档后 `status` 为 `completed`、`completedAt` 填上日期）：

```json
{
  "id": "给退款原因加必填校验",
  "name": "给退款原因加必填校验",
  "title": "给退款原因加必填校验",
  "description": "退款时原因字段必填",
  "status": "completed",
  "dev_type": null,
  "scope": null,
  "package": null,
  "priority": "P1",
  "creator": "演示者",
  "assignee": "演示者",
  "createdAt": "2026-11-10",
  "completedAt": "2026-11-10",
  "branch": "master",
  "base_branch": "master",
  "worktree_path": null,
  "commit": null,
  "pr_url": null,
  "subtasks": [],
  "children": [],
  "parent": null,
  "relatedFiles": [],
  "notes": "",
  "meta": {}
}
```

`--description` 必填，不带会直接报错（下面是另一个任务的实测）：

```text
错误：--description 必填
请传入 --description "<该任务交付什么>"（纯空白不算）：空描述的任务会在归档时被拒绝。
```

整理上下文清单（`implement` 是实现时要读的，`check` 是验收时要对照的）：

```sh
sunshine task add-context 11-10-给退款原因加必填校验 implement src/Refund.vue "实现退款原因必填校验"
# 已添加文件：src/Refund.vue

sunshine task add-context 11-10-给退款原因加必填校验 check .sunshine/spec/backend.md "退款接口契约"
# 已添加文件：.sunshine/spec/backend.md

sunshine task list-context 11-10-给退款原因加必填校验
# === 上下文文件 ===
#
# [implement.jsonl]
#   1. src/Refund.vue
#      → 实现退款原因必填校验
#
# [check.jsonl]
#   1. .sunshine/spec/backend.md
#      → 退款接口契约

sunshine task validate 11-10-给退款原因加必填校验
# === 校验上下文文件 ===
# 目标目录：/tmp/rewrite-zh/demo/demo/.sunshine/tasks/11-10-给退款原因加必填校验
#
#   implement.jsonl：✓（1 条）
#   check.jsonl：✓（1 条）
#
# ✓ 全部校验通过
```

如果上下文清单还是空的，`start` 会直接拒绝，并告诉你怎么办；确实要空着跑时加 `--allow-empty-context`。

启动任务：

```sh
sunshine task start 11-10-给退款原因加必填校验
```

真实输出：

```text
ℹ 会话身份不可用；本次不持久化当前任务指针（降级模式），AI 仍可按对话上下文继续。
提示：在能提供会话身份的 AI 会话内运行，或先设置 SUNSHINE_CONTEXT_ID 再执行 sunshine task start。
✓ 状态：planning → in_progress（降级）
✓ 已记录分支：master（降级）
警告："master" 也是该任务的 base_branch，PR 不能指向自身分支，归档时会拒绝。
切出分支后运行：sunshine task set-branch <任务> <功能分支>
```

“降级模式”指的是不写 AI 会话文件（`.sunshine/.runtime/sessions/`）；`.sunshine/.current-task` 照常写，所以 `task current` 与 `task context` 仍能读到当前任务。在 Claude Code / Cursor 的 AI 会话里运行则两者都写。

查当前任务与全部活动任务：

```sh
sunshine task current
# .sunshine/tasks/11-10-给退款原因加必填校验

sunshine task list
# 全部活动任务：
#
#   - 11-10-给退款原因加必填校验/ (in_progress) [演示者] <- 当前任务
#
# 总计：1 个任务
```

`task context` 输出当前会话的完整运行时信息（下面是摘录）：

```text
========================================
会话上下文
========================================

## 开发者
名字：演示者

## Git 状态
分支：master
工作区：4 处未提交改动

## 当前任务
路径：.sunshine/tasks/11-10-给退款原因加必填校验
来源：file:.sunshine/.current-task
名字：给退款原因加必填校验
状态：in_progress
创建：2026-11-10
描述：退款时原因字段必填
……
```

### 收尾：记会话、清指针、归档

做完任务，agent 会把这次会话记进开发者日志（以后 `mem` 回顾时用得上，第 9 节）：

```sh
sunshine task add-session --title "退款原因必填校验实现" --summary "加了原因下拉框与必填校验"
```

真实输出（摘录）：

```text
警告：task.json 的分支 "master" 在本地与 origin/master 都不存在，改用当前分支 "master"。
========================================
添加会话
========================================

会话：1
标题：退款原因必填校验实现
提交：-

[OK] 会话已追加到 journal-1.md
……
[OK] 会话 1 添加成功！
========================================

更新的文件：
  - journal-1.md
  - index.md

[OK] 已自动提交：chore: record journal
```

记录写进 `.sunshine/workspace/<你的名字>/journal-1.md`，`index.md` 的自动区块同步更新。

`/sunshine:finish-work` 底下先清当前任务指针，再归档。`finish` 只清指针，不改任务状态；状态在归档时才置 `completed`：

```sh
sunshine task finish
# ✓ 已清除当前任务（原为：.sunshine/tasks/11-10-给退款原因加必填校验）
# 来源：file:.sunshine/.current-task
```

归档时演示项目会被拒绝一次，是真实行为，不用慌——任务一直待在 `master` 分支上，而 `master` 也是 `base_branch`，PR 不能指向自身分支：

```sh
sunshine task archive 11-10-给退款原因加必填校验
```

```text
错误：拒绝归档 "11-10-给退款原因加必填校验"：branch 与 base_branch 都是 "master"，PR 不能指向自身分支，这份元数据无法描述已合并的工作。
请修复其中一个字段：
  sunshine task set-branch 11-10-给退款原因加必填校验 <功能分支>
  sunshine task set-base-branch 11-10-给退款原因加必填校验 <目标分支>
  sunshine task archive 11-10-给退款原因加必填校验 --skip-branch-validation   # 仅当该任务从未走 PR
未归档：.sunshine/tasks/11-10-给退款原因加必填校验 未动。
```

演示项目从未走 PR，用 `--skip-branch-validation` 跳过校验（正式项目请先 `set-branch` 切出功能分支）：

```sh
sunshine task archive 11-10-给退款原因加必填校验 --skip-branch-validation
# 已归档：11-10-给退款原因加必填校验 -> archive/2026-11/
# [OK] 已自动提交：chore(task): archive 11-10-给退款原因加必填校验

sunshine task list-archive
# 已归档任务：
#
# [2026-11] - 1 个任务
#   - 11-10-给退款原因加必填校验/
```

归档路径是 `.sunshine/tasks/archive/<年-月>/`，默认自动提交（`session_auto_commit` 为 `true` 时）。

## 5. 人检查验收

agent 说做完了，你在三个地方验收：

1. **看 diff**：`git status` / `git diff` 看改了哪些文件，确认没有动不该动的地方（例如只改退款逻辑，不动其他接口）。
2. **对照验收标准**：打开 `.sunshine/tasks/<任务>/prd.md`，逐条过验收标准——原因下拉框三项齐不齐、选“其他”出不出文本框、缺 `reason` 是不是返回 400 与 `REASON_REQUIRED`。
3. **确认提交**：3.4 的提交计划是 agent 起草、你一次性确认后才提交的；提交记录里应有工作提交在前、归档与日志的账本类提交在后。

验收不过直接在 agent 界面里说，例如：

示例提示词（示例，不是真实会话记录）：

> 选“其他”时没有出现文本框，修一下再按验收标准自查一遍。

中途想看进行到哪了，不用进终端翻文件，在 agent 界面里问一句就行；实在要在终端看，`sunshine task current` 与 `sunshine task list`（输出见第 4 节）就是状态来源。

## 6. 规范这样维护

spec 是团队的工程标准，写一次，后续所有任务自动遵守。演示项目的三条约定就是 spec 的样子（第 1 节 init 后，往三份规范里各加一条）：

```sh
cat >> .sunshine/spec/system.md <<'EOF'

## 演示项目约定

- 退款原因必填：用户提交退款时原因字段不能为空，否则拒绝提交。
EOF

cat >> .sunshine/spec/backend.md <<'EOF'

## 退款接口

- `POST /refunds`：`reason` 必填，缺失返回 400，错误码 `REASON_REQUIRED`。
EOF

cat >> .sunshine/spec/frontend.md <<'EOF'

## 退款表单

- 原因输入框为下拉选择（质量问题 / 尺寸不符 / 其他），选“其他”时出现文本框。
EOF
```

规范的维护分两种，都是 agent 界面里触发：

- **任务结束时自动复盘**：第 3 节的 3.3 步，agent 每个任务收尾都会判断有没有新知识值得写回 spec，有就更新，没有也走一遍判断。
- **你随时提一句**：在 agent 界面里说“把这条约定写进 spec”，agent 会改 `.sunshine/spec/` 下对应的文件，你审阅 diff 即可；你也可以直接改 spec 文件，效果一样。

`sunshine update` 永远不动 `.sunshine/spec/`——第 8 节的扫描输出里它列在“用户数据（保持不动）”下面，和 tasks、workspace、开发者身份在一起。

## 7. 工作流切换

默认工作流是 native（第 3 节讲的就是它）。想换一种，在 agent 界面里说一句“切到 TDD 工作流”，agent 在底下跑的就是：

```sh
sunshine workflow --list
#
# 可用的工作流模板：
#
#   native（内置） — 原生 Sunshine 工作流
#     默认的 Sunshine 规划 / 执行 / 收尾工作流，含原生子代理与 inline 平台分支
#   tdd（内置） — TDD 工作流
#     用一次一个红灯 / 绿灯 / 重构行为切片驱动第 2 阶段的 Sunshine 工作流变体
#   channel-driven-subagent-dispatch（内置） — Channel 驱动的子代理派发
#     主会话通过 sunshine channel 协调 implement/check 工作线程的工作流变体

sunshine workflow --template tdd
#   ✓ 已把 .sunshine/workflow.md 换成 "tdd"
```

切回 native 时会先拦你一下——因为 `workflow.md` 已经被换成 tdd，与哈希清单里的记录不一致，CLI 认为它“已有本地修改”：

```sh
sunshine workflow --template native
# 错误：.sunshine/workflow.md 已有本地修改，请加 --force 覆盖，或用 --create-new 写 .sunshine/workflow.md.new

sunshine workflow --template native --force
#   ✓ 已把 .sunshine/workflow.md 换成 "native"

sunshine workflow --template native --create-new
#   + 已写入 .sunshine/workflow.md.new（workflow.md 未改动）
```

规则是：未改动直接覆盖；已改动或来源未知时交互确认（覆盖 / 写 `.new` / 跳过），非交互终端下用 `--force` 覆盖或 `--create-new` 写副本。`--create-new` 只写 `.sunshine/workflow.md.new`，不动正在用的 `workflow.md`。

本教程到这一步结束时 `workflow.md` 是 tdd——第 8 节的 `update --dry-run` 会把它列为“你已修改”，正好演示三态检测。

## 8. update 与模板市场

### 先看 `update --dry-run`

`update` 是把项目内的 Sunshine 模板文件同步到当前 CLI 版本。`--dry-run` 只打印计划，不写盘，是最安全的用法；平时由 agent 在需要时跑，你手动想看也可以直接跑。下面是第 7 节切到 tdd 之后、在演示项目里跑的真实输出（摘录）：

```sh
sunshine update --dry-run
```

```text
Sunshine 更新
══════════════
项目版本：0.2.1
CLI 版本：0.2.1
最新版：0.2.1


正在扫描变更……
  未变更文件（将跳过）：
    ○ .claude/agents/sunshine-check.md
    ○ .claude/agents/sunshine-implement.md
    ○ .claude/agents/sunshine-research.md
    ○ .claude/commands/sunshine/continue.md
    ○ .claude/commands/sunshine/finish-work.md
    ……另有 101 个

  你已修改（需要你决定）：
    ? .sunshine/workflow.md

  用户数据（保持不动）：
    ○ .sunshine/workspace/
    ○ .sunshine/tasks/
    ○ .sunshine/spec/
    ○ .sunshine/.developer/
    ○ .sunshine/.current-task/

[试运行] 未做任何修改。
```

三态一目了然：未改动的模板文件自动更新、你改过的逐个确认（覆盖 / 跳过 / 写 `.new` 副本，默认跳过）、用户数据（spec、tasks、workspace、开发者身份）永远不动。项目与 CLI 都是最新时，末尾是 `✓ 已是最新，无需更新。`。真正执行时去掉 `--dry-run` 即可，更新前会在 `.sunshine/.backups/<时间戳>/` 做完整备份，成功后更新 `.sunshine/.version`。

### 从模板市场装规范

新项目想从市场装一份规范模板起步，在终端里跑一次 `init --registry` 即可（装完由 `update` 持续刷新）。先建一个最小的本地市场仓库（结构就是官方模板仓库的标准结构 `marketplace/index.json` + `marketplace/specs/`）：

```sh
mkdir -p registry-src/marketplace/specs/demo-spec
cat > registry-src/marketplace/index.json <<'EOF'
{
  "version": 1,
  "templates": [
    {
      "id": "demo-spec",
      "type": "spec",
      "name": "演示规范",
      "description": "演示用规范模板",
      "path": "specs/demo-spec"
    }
  ]
}
EOF
cat > registry-src/marketplace/specs/demo-spec/demo.md <<'EOF'
# 演示规范

本文件来自本地模板市场 demo-spec。

## 约定

- 演示项目的所有接口返回统一错误码。
EOF
cd registry-src && git init && git add -A && git commit -m "demo registry" && cd ..
```

两个容易踩的点（都是实测踩出来的）：

1. 本地路径要指向 `marketplace` 子目录本身（`…/registry-src/marketplace`），而不是仓库根——指向仓库根时 CLI 找不到 `index.json`，会报 `该 registry 没有 index.json，不能用 --template`。
2. `index.json` 里的 `path` 是相对于 `index.json` 所在目录的，写 `specs/demo-spec`，不是 `marketplace/specs/demo-spec`。

然后在新项目里从市场安装：

```sh
mkdir demo2 && cd demo2 && git init
sunshine init -u 演示者 -y --registry /tmp/rewrite-zh/demo/registry-src/marketplace --template demo-spec
```

真实输出：

```text
正在配置 Claude Code……
正在配置 Cursor……
已安装模板 "demo-spec"（.sunshine/spec），共 1 个文件。
已记录 registry.spec：/tmp/rewrite-zh/demo/registry-src/marketplace（模板 demo-spec），sunshine update 会持续刷新。
初始化完成，开发者：演示者
已安装平台：
  - Claude Code
  - Cursor
下一步：
  1. 在 agent 界面里打开项目，直接用自然语言提第一个需求；
  2. 规范已从模板市场装进 .sunshine/spec/，工作流按 .sunshine/workflow.md 自动推进，一般不用改。
```

模板内容装进了 `.sunshine/spec/demo.md`，来源记进了 `config.yaml`：

```yaml
# 受哈希追踪的 spec 模板，本地改动走正常的 update 冲突流程。
registry:
  spec:
    source: /tmp/rewrite-zh/demo/registry-src/marketplace
    template: demo-spec
```

远程市场同理，把本地路径换成 `gh:owner/repo`、完整 git URL、HTTP(S) URL 或 `git@` 地址即可。上面是 `-y` 加 `--template` 的写法；交互模式下不传 `--template` 时 init 会列出该市场的 spec 模板让你现选，不用事先知道模板 id（市场里没有 index.json 时直接下载整个来源，不出选单）。

### 市场模板更新后，`update` 会持续刷新

改一下市场里的模板并提交，再跑 `update --dry-run`：

```text
正在扫描变更……
  模板已更新（将自动更新）：
    ↑ .sunshine/spec/demo.md

  未变更文件（将跳过）：
    ○ .claude/agents/sunshine-check.md
    ……
  你已修改（需要你决定）：
    ? .sunshine/config.yaml

  用户数据（保持不动）：
    ○ .sunshine/workspace/
    ○ .sunshine/tasks/
    ○ .sunshine/spec/
    ○ .sunshine/.developer/

[试运行] 未做任何修改。
```

`demo.md` 因为没动过本地版本，会被自动更新；如果你在本地改过它，同样走“你已修改”的确认流程。`config.yaml` 列为“你已修改”是因为 `registry.spec` 是装模板后写进去的——这是预期行为。

## 9. mem：回头看会话

`mem` 只读解析本机各平台的会话记录，不写会话存储。想回头看之前在 agent 界面里聊过什么，在 agent 界面里说一句“帮我找找上次聊退款的那次会话”，agent 在底下跑的就是 `mem search`；你也可以在终端直接跑：

```sh
sunshine mem list              # 列出本项目的会话
sunshine mem search "退款"      # 按关键词找会话
sunshine mem context 11111111  # 看某次会话的上下文（编号可只写前几位）
sunshine mem extract 11111111  # 导出某次会话的完整对话
sunshine mem projects          # 列出有会话的项目
```

下面用一个最小的会话文件演示输出形态。**这是手工夹具，不是真实会话记录**：把 `HOME` 指向一个临时目录，按真实会话的目录结构（`~/.claude/projects/<项目目录>/<会话 id>.jsonl`）手写了一个 4 轮的演示会话（两轮用户提问、两轮助手回复，内容就是本教程的退款任务），再跑上面的命令：

```sh
export HOME=/tmp/rewrite-zh/demo/home   # 演示用，不动你真实的 HOME
mkdir -p "$HOME/.claude/projects/-tmp-rewrite-zh-demo-demo"
# 写入演示会话 11111111-2222-3333-4444-555555555555.jsonl（4 轮）
```

```sh
sunshine mem list
# 范围：项目=/tmp/rewrite-zh/demo/demo  平台=all
# [claude  ] 2026-11-10 14:14  11111111-222  /tmp/rewrite-zh/demo/demo
#
# 共 1 个会话

sunshine mem search "退款"
# 范围：项目=/tmp/rewrite-zh/demo/demo  平台=all  关键词="退款"
#
# [claude  ] 2026-11-10 14:14  11111111-222  /tmp/rewrite-zh/demo/demo  相关度=0.500  命中=2（用户=2，助手=0）  轮次=4
#     [用户] 给退款原因加必填校验，原因字段不能为空
#     [用户] 退款表单的原因下拉框要包含质量问题、尺寸不符、其他三项。
#
# 共 1 个会话

sunshine mem context 11111111
# # 上下文：[claude] 11111111-2222-3333-4444-555555555555
# # 工作目录：/tmp/rewrite-zh/demo/demo
# # 未指定关键词 — 显示前 3 轮，共 4 轮
# # 显示轮次：4  已用预算：111/6000 字符
#
# ## 第 0 轮（用户） ← 命中
#
# 给退款原因加必填校验，原因字段不能为空
# ……
# ## 第 3 轮（助手）
#
# 已加上，原因选“其他”时会出现文本框。

sunshine mem projects
# 活跃项目
# 2026-11-10 14:14  会话数=  1 (claude:1)  /tmp/rewrite-zh/demo/demo
#
# 共 1 个项目
```

输出开头常有几行 `警告：未找到 Codex 会话存储（…），已跳过` 之类的提示——那只是说某个平台在本机没有会话记录，跳过而已，不是错误。`--global` 可检索全部项目，`--json` 输出机器可读的 JSON，`--platform claude` 只查指定平台。

## 10. channel：多 Agent 协同

channel 是多个 worker（claude / codex）协作的会话：`create` 建会话，`spawn` 注册 worker，`send` 发消息，`wait` 等事件。主会话在 agent 界面里通过 `sunshine-channel` skill 协调 worker；你也可以在终端手动搭一个 channel 看它怎么工作。存储根默认是 `~/.sunshine/channels/`，可用 `SUNSHINE_CHANNEL_ROOT` 覆盖——本教程用临时目录演示，不动你真实的 channel 存储：

```sh
export SUNSHINE_CHANNEL_ROOT=/tmp/rewrite-zh/demo/channels
```

### 建会话、发消息、看事件

```sh
sunshine channel create review --type chat
# 已创建 channel 'review'（chat）位于 /tmp/rewrite-zh/demo/channels/-tmp-rewrite-zh-demo-demo/review

sunshine channel send review --to impl "开始实现退款原因必填校验"
# {"by":"main","kind":"message","origin":"cli","seq":2,"text":"开始实现退款原因必填校验","to":"impl","ts":"2026-11-10T01:14:40.734680463Z"}

sunshine channel messages review
# [1] 01:14:40 [create] by=main cwd=/tmp/rewrite-zh/demo/demo
# [2] 01:14:40 [message] by=main to=impl
#          开始实现退款原因必填校验

sunshine channel list
# 名称                        工作者        事件      最近活动                 最近事件       类型       任务
# review                    -          2       -                    message    chat     -
```

`send` 输出的是一条 JSON 事件（`seq` 是事件序号）；`messages` 是人类可读的时间线。上面事件里的时间是 UTC（`ts` 末尾的 `Z`）。`--task` 可以在建会话时绑一个任务：`sunshine channel create review --type chat --task .sunshine/tasks/11-10-给退款原因加必填校验`。

### 标题与上下文

```sh
sunshine channel title set review --title "退款原因必填校验"
# {"action":"title","by":"main","kind":"channel","origin":"cli","seq":3,"title":"退款原因必填校验","ts":"2026-11-10T01:14:40.761628344Z"}

sunshine channel context add review --raw "只改退款逻辑，不动其他接口"
# {"action":"add","by":"main","context":[{"text":"只改退款逻辑，不动其他接口","type":"raw"}],"kind":"context","origin":"cli","seq":4,"target":"channel","ts":"2026-11-10T01:14:40.770642875Z"}

sunshine channel context list review
# raw  只改退款逻辑，不动其他接口
```

### 等事件：超时是正常结果

```sh
sunshine channel wait review --from impl --kind done --timeout 3s
# 错误：超时：等待 channel 'review' 超时（3s），仍在等待：impl（上游退出码 124）
```

没有 worker 回复时 `wait` 会按 `--timeout` 超时并报错退出——这就是它的设计用途：脚本里用它等某个 worker 的 `done` 事件。

### forum 频道：主题式讨论

`forum` 类型的频道用主题（thread）组织讨论，`chat` 频道不能用 forum 命令（实测会报错：`错误：频道 "review" 的类型是 chat，post 操作只支持 forum 类型的频道`）：

```sh
sunshine channel create board --type forum
# 已创建 channel 'board'（forum）位于 /tmp/rewrite-zh/demo/channels/-tmp-rewrite-zh-demo-demo/board

sunshine channel post board opened --as main --thread t1 --title "退款原因实现方案" --description "原因下拉框：质量问题 / 尺寸不符 / 其他"
# {"action":"opened","by":"main","description":"原因下拉框：质量问题 / 尺寸不符 / 其他","kind":"thread","origin":"cli","seq":2,"thread":"t1","title":"退款原因实现方案","ts":"2026-11-10T01:14:48.316444887Z"}

sunshine channel post board comment --as alice --thread t1 --text "收到，正在看"
# {"action":"comment","by":"alice","kind":"thread","origin":"cli","seq":3,"text":"收到，正在看","thread":"t1","ts":"2026-11-10T01:14:48.324314798Z"}

sunshine channel forum board
# 主题  状态  标题
# t1 [open] 退款原因实现方案

sunshine channel thread board t1
# t1 [open] 退款原因实现方案
# 描述：原因下拉框：质量问题 / 尺寸不符 / 其他
#   2026-11-10 01:14:48 opened 发帖人=main
#   2026-11-10 01:14:48 comment 发帖人=alice 收到，正在看
```

### spawn：需要已登录的真实 worker

`spawn` 是 channel 里唯一需要真实环境的一步：它会拉起本机的 `claude` 或 `codex` 进程。前提是对应 CLI 已安装**并已登录**。沙箱里没有这个条件，实测结果如下，如实记录：

```sh
sunshine channel spawn review --agent implement --provider claude --as impl
# {
#   "pid": 8799,
#   "log": "/tmp/rewrite-zh/demo/channels/-tmp-rewrite-zh-demo-demo/review/impl.log",
#   "worker": "impl"
# }
```

命令本身返回了 pid 与日志路径，但随即在事件日志里记了一条失败：

```sh
sunshine channel messages review
# ……
# [5] 01:14:48 [error] by=supervisor:impl worker 拉起失败：拉起 claude worker 失败：exec: "claude": executable file not found in $PATH
```

换 codex 试（沙箱里有 codex 二进制，但没登录），进程拉起来了，随后在连接时报 401（worker 日志里的原行）：

```text
ERROR codex_api::endpoint::responses_websocket: failed to connect to websocket: HTTP error: 401 Unauthorized, url: wss://api.openai.com/v1/responses
```

所以：**有 claude / codex 且已登录的机器上，`spawn` 之后 `send` 发过去的消息会被 worker 收到并执行；没有登录的 worker 时，`spawn` 只会留下一条 `error` 事件，不会假装成功。** 用 `channel run` 一次性跑完一个任务的前提也一样。

### 停 worker、删 channel

```sh
sunshine channel kill review --as impl-codex
# 已停止 worker 'impl-codex'（channel 'review'）

sunshine channel rm review
# 已删除 channel 'review'
```

`kill` 用 `--as <worker 名>` 指定对象；对未在运行的 worker 会报错（实测：`错误：worker 'impl' 未在 channel 'review' 中运行`）。`rm` 杀掉全部 worker 并删除整个 channel 目录。`prune` 可按条件批量清理（默认只演练，不真删）：`sunshine channel prune --empty`，演示里没有可清理的 channel，输出是 `（无可清理的 channel）`。

## 11. upgrade：升级 CLI 自身

`upgrade` 升级的是 CLI 自身（`update` 升级的是项目里的模板文件，两回事）。先用 `--dry-run` 看计划，不写盘：

已是最新版时（当前 Release 装的 v0.2.1，用 ldflags 注入 `0.2.1` 的二进制跑）：

```sh
sunshine upgrade --dry-run
# 已是最新版本（当前 0.2.1，最新 0.2.1），无需升级。
```

有新版时（下面这行来自默认构建的 0.1.0 二进制——未注入版本号时默认就是 0.1.0，仅版本号不同，形态一致；备份路径是该二进制所在的目录）：

```text
正在升级 Sunshine：0.1.0 → 0.2.1（通道 latest）
资产：sunshine_0.2.1_linux_amd64.tar.gz
下载地址：https://github.com/linrong007/Sunshine/releases/download/v0.2.1/sunshine_0.2.1_linux_amd64.tar.gz
备份：将把当前二进制备份为 /tmp/rewrite-zh/sunshine.bak
[试运行] 未做任何修改。
```

真正升级时去掉 `--dry-run`：查最新版 → 按当前系统与架构选资产 → 下载 → 按 `checksums.txt` 校验 sha256 → 备份当前二进制（同目录 `.bak`，权限保留）→ 替换 → 运行新版 `--version` 校验，失败回滚备份并报错（流程见 `sunshine upgrade --help`）。也可以指定版本或通道：`sunshine upgrade --tag 0.2.0`、`sunshine upgrade --tag beta`。

## 12. 六个场景：从第一天到日常

前面 11 节讲的是主线流程；这一节按六个真实场景走一遍，每个场景都是一个完整的端到端实录：

| 场景 | 适用时机 | 关键点 |
| --- | --- | --- |
| 1. 从零开始新项目 | 新建产品、服务、包或内部工具 | 空目录 init，第一任务从零长出来 |
| 2. 接入存量项目 | 仓库已存在，但规范主要靠人记忆 | init 不动现有代码，先把约定落进 spec |
| 3. 交付产品功能 | 一个任务横跨产品、API、数据和 UI | 一个任务 + 跨层上下文 + channel 协调 |
| 4. 重构老模块 | 代码能跑，但已经难以安全修改 | 先用测试钉住行为，再重构，测试全程绿灯 |
| 5. 修复反复出现的 bug | 同类问题多次出现 | mem 先找上次的会话，再建任务、把教训写进 spec |
| 6. 产品设计先行 | 先做产品设计，再进开发 | 设计文档落盘 `doc/`，拆成子任务后开发一个个接手 |

### 12.1 从零开始新项目：web 商城

空目录，什么都没有——这是最干净的起点。先建个空目录、进 git，剩下的交给 Claude Code：init、建任务、配上下文、启动，全是它在底下跑，你不用敲命令。空目录检测不到类型，默认按全栈处理，三份规范（system.md、frontend.md、backend.md）全生成——新项目先有一套默认规范，后面再按团队习惯改，比一开始就空着强。

在 Claude Code 里打开项目，用自然语言提第一个需求（Codex、Cursor 同理）：

示例提示词（示例，不是真实会话记录）：

> 我正在从零开始做一个 web 商城：先做商品列表，首页展示商品名称、价格、库存。请按 sunshine 工作流走：先向我提问，把不清楚的产品逻辑和技术决策问清楚；确认后，帮我创建第一个任务、写好 PRD，再动手实现。

下面是接下来的会话示意（对话是示例；命令与输出是真实实跑值）：

> **Claude Code：** 先问一个决定怎么做的问题：技术选型怎么定——Go 单文件、标准库 `net/http`，还是上框架、接数据库？
>
> **你：** Go 单文件，标准库就够了，module 叫 `webstore`，不接数据库。
>
> **Claude Code：** 好，再问一个：商品数据放哪——先写死在代码里，还是读文件？
>
> **你：** 先写死在代码里，三件商品够演示就行：无线鼠标 89 元、库存 120；机械键盘 299 元、库存 45；27 寸显示器 1299 元、库存 20。
>
> **Claude Code：** 最后一个：列表显示哪些字段？价格用什么单位？
>
> **你：** 名称、价格、库存三样，价格用元。
>
> **Claude Code：** 好，问完了，给你一份最终规划总结，确认后我再动手：
>
> **最终规划总结**
>
> - 目标：从零做 web 商城，Go 单文件，端口 18080；先做商品列表。
> - 范围内：商品列表（列表页 + `GET /api/products`）；三件商品写死在代码里。
> - 范围外：登录、支付、下单、订单、持久化、样式精修；加入购物车是下一个需求，不在本任务。
> - 关键决策（均已确认）：Go 标准库单文件、无第三方依赖；商品数据写死在代码里；端口 18080。
> - 风险/延期项：重启丢数据（接受，演示够用）；无鉴权、无持久化（后续再议）。
> - 产物状态：规划任务 PRD + spec 三件套（system/frontend/backend）+ 首个实现任务「商品列表」的 PRD；零产品代码（`.go` 文件 0 个），未 `task start`。
>
> 批准这份规划后，我就创建规划任务、写 PRD 和三份 spec，仍不碰产品代码、不启动实现任务。没问题吗？
>
> **你：** 批准。
>
> **Claude Code：** 规划产出已落盘：spec 三件套和「商品列表」的 PRD 都写好了，零产品代码，实现任务已创建但未启动。交付清单给你确认一下，批准后我再启动实现。
>
> **你：** 批准。
>
> **Claude Code：** 启动任务，派 `sunshine-implement` 子代理实现。实现完成，派 `sunshine-check` 子代理核查：`go vet`、`go build` 全绿，没有需要修的问题。跑给你看：

```sh
curl -s localhost:18080/api/products
# [{"id":1,"name":"无线鼠标","price":89,"stock":120},{"id":2,"name":"机械键盘","price":299,"stock":45},{"id":3,"name":"27寸显示器","price":1299,"stock":20}]
```

浏览器打开首页，三件商品也都在：无线鼠标 89 元、机械键盘 299 元、27寸显示器 1299 元。

> **Claude Code：** 3.3 回写 spec：无更新——实现完全按规划时写入 spec 的决策走，没有新模式、新坑。提交计划如下，回复“行”就执行：
>
> ```text
> 建议提交（按顺序）：
>   1. chore: 初始化 Sunshine 工程骨架（全栈）
>   2. feat: 商品列表页 + GET /api/products
>      - go.mod、main.go、两个任务目录
> 未识别的脏文件（不在任何提交里——请确认包含还是排除）：
>   - 无
> ```
>
> **你：** 行。
>
> **Claude Code：** 提交完了：`98d1e5d chore: 初始化 Sunshine 工程骨架（全栈）`、`acaa3e3 feat: 商品列表页 + GET /api/products`。剩下的就是收尾：运行 `/finish-work` 归档任务、记录会话日志，要现在执行吗？
>
> **你：** 可以。

归档时先被拒了一次：当前分支 `main` 也是 `base_branch`，PR 不能指向自身分支。切出 `feat/product-list`、给两个任务都 `set-branch` 后重新归档，通过。`finish-work` 归档了「商品列表」和规划任务，记了会话日志：

```sh
git log --oneline
# eb7adc7 chore: record journal
# e961e60 chore(task): archive 11-13-webstore
# 71f3de9 chore(task): archive 11-13-product-list
# acaa3e3 feat: 商品列表页 + GET /api/products
# 98d1e5d chore: 初始化 Sunshine 工程骨架（全栈）
```

> **Claude Code：** 做完了，任务归档了：

```text
[2026-11] - 2 个任务
  - 11-13-product-list/
  - 11-13-webstore/
```

接着做第二个需求：

> **你：** 接着做第二个需求：加入购物车。
>
> **Claude Code：** 先问一个问题：购物车放哪——先放内存里，还是写文件、接数据库？
>
> **你：** 先放内存，演示够用就行。
>
> **Claude Code：** 再问一个：加购后返回什么、总价怎么算？
>
> **你：** `POST /api/cart/add` 加购，`GET /api/cart` 看件数和总价；先加一件无线鼠标、再加一件机械键盘，总价 388。不存在的 id 返回 404，重复加购数量累加。
>
> **Claude Code：** 好，规划总结：目标是加入购物车；范围内是 `POST /api/cart/add`、`GET /api/cart`、列表页每件商品一个“加入购物车”按钮；范围外是不改 `GET /api/products`、不做持久化；关键决策是内存购物车、重复加购数量累加、不存在的 id 返回 404。产物是「加入购物车」的 PRD，零产品代码，未启动。批准后我写 PRD，批准交付后启动实现，没问题吗？
>
> **你：** 批准。
>
> **Claude Code：** PRD 已落盘，零产品代码，未启动。确认一下，批准后我启动实现。
>
> **你：** 批准。
>
> **Claude Code：** 启动任务，派 `sunshine-implement` 子代理实现；实现完成，派 `sunshine-check` 子代理核查，全绿。跑给你看：

```sh
curl -s -X POST localhost:18080/api/cart/add -H 'Content-Type: application/json' -d '{"id":1}'
# {"status":"ok"}
curl -s -X POST localhost:18080/api/cart/add -H 'Content-Type: application/json' -d '{"id":2}'
# {"status":"ok"}
curl -s localhost:18080/api/cart
# {"items":[{"id":1,"name":"无线鼠标","price":89,"qty":1},{"id":2,"name":"机械键盘","price":299,"qty":1}],"total":388}
curl -s -o /dev/null -w "%{http_code}" -X POST localhost:18080/api/cart/add -H 'Content-Type: application/json' -d '{"id":999}'
# 404
```

`GET /api/products` 还是三件商品不变，首页每件商品后面都有一个“加入购物车”按钮。

> **Claude Code：** 3.3 回写 spec：无更新。提交计划只有一条，回复“行”就执行：
>
> ```text
> 建议提交（按顺序）：
>   1. feat: 加入购物车 + GET /api/cart
>      - main.go、11-13-add-to-cart 任务目录
> 未识别的脏文件：无
> ```
>
> **你：** 行。
>
> **Claude Code：** 提交完了：`ba61066 feat: 加入购物车 + GET /api/cart`。运行 `/finish-work` 归档、记日志，要现在执行吗？
>
> **你：** 可以。

`finish-work` 归档了「加入购物车」，记了会话日志：

```sh
git log --oneline
# 2d87dc7 chore: record journal
# 01d6eba chore(task): archive 11-13-add-to-cart
# ba61066 feat: 加入购物车 + GET /api/cart
# eb7adc7 chore: record journal
# e961e60 chore(task): archive 11-13-webstore
# 71f3de9 chore(task): archive 11-13-product-list
# acaa3e3 feat: 商品列表页 + GET /api/products
# 98d1e5d chore: 初始化 Sunshine 工程骨架（全栈）
```

> **Claude Code：** 做完了，任务归档了：

```text
[2026-11] - 3 个任务
  - 11-13-add-to-cart/
  - 11-13-product-list/
  - 11-13-webstore/
```

两个需求走完，项目里就有了：能跑的商城服务、三条归档的任务记录、按默认模板生成的规范。

这里踩了一个真实的坑，值得记下来：加入购物车是后来才提的需求，规划任务（`11-13-webstore`）在第一个任务做完时就已经归档了，已归档的父任务下建不了子任务，所以「加入购物车」只能建成独立任务（`parent: null`），PRD 里写明与已归档父任务互相指向。按工作流 1.2，拆解只做一次——如果一开始就知道有两个需求，规划时就该把两个子任务都建好，父任务等全部子任务归档后再归档；已归档的任务不 reopen，要返工就新建任务走完整流程。

### 12.2 接入存量项目：先把约定落进 spec

仓库已存在，但规范主要靠人记——这是最常见的接入场景。演示用的存量项目是一个跑了两年的 web 商城（`package.json` 里只有 `express`；代码分了三处：`src/server.js` 是入口，`src/routes/` 里是商品和订单接口，`src/db.js` 管数据读写），团队约定写在 `README.md` 里，从没落进任何规范文件：

```markdown
# webstore

团队约定（一直靠人记，没落进任何文档）：
- 接口统一返回 { code, data }，code 是大写下划线错误码。
- 价格统一用“分”存库，展示时除以 100。
- 端口固定 3000，改端口要全组同步。
```

直接在这个目录里 init——init 只加文件，不改现有代码。Claude Code 在底下跑 init，认出这是后端项目（只有 `express`），只生成 `system.md` 与 `backend.md`；现有文件原样不动，git 里只有新增的 Sunshine 目录：

```text
?? .claude/
?? .cursor/
?? .sunshine/
```

接入存量项目的第一任务不是写功能，是把靠人记的约定落进 spec——否则 agent 每次都要重新猜。在 Claude Code 里说一句就行（Codex、Cursor 同理）：

示例提示词（示例，不是真实会话记录）：

> 我要把一个存量项目接进 sunshine：仓库已存在，但规范主要靠人记，README 里有三条团队约定。请按 sunshine 工作流走：先向我提问，确认这三条约定和要落进哪些 spec 文件；确认后，帮我创建任务、写好 PRD，再把约定写进 .sunshine/spec/，以后所有任务都按它来。

下面是接下来的会话示意（对话是示例；命令与输出是真实实跑值）：

> **Claude Code：** 先问一个问题：三条约定目前写在哪份文档里了？
>
> **你：** 哪份都没写，只在口头上；代码里能看到对应的做法。
>
> **Claude Code：** 再问一个：三条约定的准确表述是什么？
>
> **你：** 接口统一返回 `{code, data}`；价格统一用“分”存库，展示除以 100；端口固定 3000。
>
> **Claude Code：** 再问一个：这次只写规范、不改产品代码，对吗？核查发现代码和约定对不上怎么办？
>
> **你：** 对，零产品代码改动；对不上的先汇报，不擅自改代码。
>
> **Claude Code：** 再问一个：三条约定分别写进哪个 spec 文件？
>
> **你：** 接口格式、价格用“分”进 `backend.md`；端口 3000 进 `system.md`。
>
> **Claude Code：** 最后一个：怎么算完成？
>
> **你：** 三条约定原文进 spec；check 对照代码逐条验证属实；提交、归档、记日志。
>
> **Claude Code：** 好，问完了，给你一份最终规划总结，确认后我再动手：
>
> **最终规划总结**
>
> - 目标：把商城已有的三条口头约定写进 `.sunshine/spec/`，让后续任务自动遵守；不改产品代码。
> - 范围内：约定 1、2 进 `backend.md`，约定 3 进 `system.md`；以代码现状为准（商品/订单接口、`src/db.js`、`src/server.js`）。
> - 范围外：不改任何产品代码（`src/`、`package.json` 均不动）；不改约定本身，只落盘；不做设计先行（无 `doc/`，走直接开发）。
> - 关键决策（均已确认）：轻量任务，只有 PRD，不建 design.md/implement.md；不拆父子任务；端口约定放 `system.md`（总原则）而非 `backend.md`。
> - 风险/延期项：当前分支 `main` 就是 `base_branch`，归档时会拒绝，归档前需切功能分支 + `set-branch`。
> - 产物状态：`prd.md` 已写好（含上述问答记录）；`implement.jsonl`/`check.jsonl` 各 2 条真实条目，`task validate` 全部通过；spec 三行待 `task start` 后由 implement 写入。
>
> 批准这份规划后，我先写 PRD；写 spec 本身就是这个任务的实现，等批准交付、task start 之后再由 implement 写入。没问题吗？
>
> **你：** 批准。
>
> **Claude Code：** PRD 已落盘，零产品代码，未启动。确认一下，批准后我启动任务、写 spec、再对照代码核查。
>
> **你：** 批准。
>
> **Claude Code：** 启动任务，派 `sunshine-implement` 子代理把三条约定写进 spec；实现完成，派 `sunshine-check` 子代理对照代码逐条验证。下面是真实写进去的内容（行号已核对）：

```markdown
## backend.md
- 接口统一返回 `{code, data}`。
- 价格统一用“分”存库，展示时除以 100。
```

```markdown
## system.md
- 服务端口固定 3000。
```

check 的核查结果，全部属实，无问题、无修复：约定 1，`grep "res.json" src/` 共 5 处，全部是 `{ code: 0, data: ... }` 或 `{ code: 404, data: null }`；约定 2，`db.js` 里三件商品的价格是 8900/29900/129900（分），`products.js` 两处 `price: p.price / 100`，订单 `totalPrice` 是分价相乘；约定 3，`server.js` 里 `const PORT = 3000;`、`app.listen(PORT, ...)`。`node --check` 四个文件全部通过，产品代码零改动（`git diff --stat` 为空）。还实跑验证了一次：服务日志是“商城服务运行在端口 3000”，`GET /products` 返回三件商品（价格 89/299/1299），`POST /orders {"productId":2,"quantity":3}` 返回 `{"code":0,"data":{"id":1,"productId":2,"quantity":3,"totalPrice":89700}}`——29900×3，分价运算一致。

> **Claude Code：** 3.3 回写 spec：spec 就是本任务的交付物，没有超出三条约定的新知识，无新增规范更新。提交计划如下，回复“行”就执行：
>
> ```text
> 建议提交（按顺序）：
>   1. docs: 把三条口头约定写进规范
>      - .claude/（sunshine init：Claude Code 平台配置）
>      - .cursor/（sunshine init：Cursor 平台配置）
>      - .sunshine/（工程骨架 + 任务产物 + 三条约定进 spec）
> 未识别的脏文件（不在任何提交里——请确认包含还是排除）：
>   - 无
> ```
>
> **你：** 行。
>
> **Claude Code：** 提交完了：`0476a72 docs: 把三条口头约定写进规范`（122 个文件），工作区干净。运行 `/finish-work` 归档、记日志，要现在执行吗？
>
> **你：** 可以。

归档时先被拒了一次：当前分支 `main` 也是 `base_branch`，PR 不能指向自身分支。切出 `docs/old-shop-conventions`、给任务 `set-branch` 后重新归档，通过。`finish-work` 归档了任务，记了会话日志：

```sh
git log --oneline
# f0c95ed chore: record journal
# b8fcd1a chore(task): archive 11-13-old-shop-conventions
# 0476a72 docs: 把三条口头约定写进规范
# 998efb7 chore: 初始提交：Express 商城（商品/订单接口）
```

注意顺序细节：工作提交 `0476a72` 做在 `main` 上，功能分支是归档前才切的，所以 `main` 上只有前两条，归档与日志两条提交只在 `docs/old-shop-conventions` 上。

> **Claude Code：** 上面两段就是写进去的，任务照常走完、归档了：

```text
[2026-11] - 1 个任务
  - 11-13-old-shop-conventions/
```

从这时起，约定不再只活在老员工的记忆里：新任务的 agent 会读 spec，老员工离职也不带走规范。存量项目接入的验收标准就一条——**init 之后 `git status` 里没有被改过的旧文件，spec 里多了一条以前只靠人记的约定**。

### 12.3 交付产品功能：一个任务横跨产品、API、数据和 UI

一个任务横跨四层时，最容易出的错是只改了一层、漏了另一层——Sunshine 的解法是：一个任务，四层要改什么写进同一份 PRD，改动顺序走 channel 说清楚，验收对照同一份接口契约。演示项目是那个跑了两年的 web 商城，任务是下单必填收货地址：结算页（UI）加地址输入框、下单接口（API）缺地址返回 400、订单记录（数据）多一个 address 字段。

在 Claude Code 里提需求（Codex、Cursor 同理）：

示例提示词（示例，不是真实会话记录）：

> 我要交付一个产品功能：下单必填收货地址，一个任务横跨产品、API、数据和 UI 四层（UI 是 public/checkout.html，API 是 src/routes/orders.js，数据是 src/db.js）。请按 sunshine 工作流走：先向我提问，把不清楚的校验规则（比如缺地址时的错误码）和四层的改动顺序问清楚；确认后，帮我创建任务、写好 PRD，再动手实现。

下面是接下来的会话示意（对话是示例；命令与输出是真实实跑值）：

> **Claude Code：** 先问一个决定怎么做的问题：缺地址时后端返回什么——400 加一个错误码，还是 200 带错误信息？
>
> **你：** 400，错误码 ADDRESS_REQUIRED。
>
> **Claude Code：** 再问一个：地址是只一个文本框还是分省市区？
>
> **你：** 只一个文本框，`name="address"`。
>
> **Claude Code：** 再问一个：四层按什么顺序改？
>
> **你：** 先 API 再 UI，顺序发到 channel 里说清楚。
>
> **Claude Code：** 再问一个：订单记录里的 address 存什么？
>
> **你：** 存下单时传的原样字符串。
>
> **Claude Code：** 最后一个：怎么算完成？
>
> **你：** 两档 curl（缺地址 400、带地址 200 含 address）+ 结算页有 `name="address"` 输入框；任务照常归档。
>
> **Claude Code：** 好，问完了，给你一份最终规划总结，确认后我再动手：
>
> **最终规划总结**
>
> - 目标：下单必填收货地址，一个任务横跨产品、API、数据和 UI 四层。
> - 范围内：结算页加 address 输入框；`POST /orders` 缺地址返回 400（ADDRESS_REQUIRED）；订单记录多 address 字段；PRD 规则。
> - 范围外：商品接口与价格计算、端口 3000、优惠券、登录/鉴权、设计先行。
> - 关键决策（均已确认）：缺地址 400 + ADDRESS_REQUIRED（非 200 带错误信息）；单文本框 `name="address"`；先 API 再 UI，顺序走 channel；address 存原样字符串。
> - 风险/延期项：无 worker 回复时 channel wait 会超时（正常，本节协调只做到发消息；拉起真实 worker 需本机已登录 claude/codex）；空/空白地址边界问答未定，由实现按“必填”语义处理。
> - 产物状态：轻量任务只有 `prd.md`；`implement.jsonl`/`check.jsonl` 各 1 条（只放 `.sunshine/spec/backend.md` 契约，不放代码文件），`task validate` 全部通过；零产品代码，未 `task start`。
>
> 批准这份规划后，我写 PRD 和上下文清单，仍不碰产品代码、不启动。没问题吗？
>
> **你：** 批准。
>
> **Claude Code：** PRD 和上下文清单已落盘，零产品代码，未启动。确认一下，批准后我启动任务、开始实现。
>
> **你：** 批准。
>
> **Claude Code：** 启动任务，派 `sunshine-implement` 子代理实现。四层按先 API 再 UI 的顺序改，顺序我发到 channel 里说清楚，两条消息：

```sh
sunshine channel create s3-cross --type chat --task .sunshine/tasks/11-13-下单必填收货地址 --force
# 已创建 channel 's3-cross'（chat）位于 /home/hatch/.sunshine/channels/-tmp-scenes-s3-cross/s3-cross
sunshine channel send s3-cross --by main "1. 先改 API：POST /orders 缺 address 返回 400，错误码 ADDRESS_REQUIRED"
sunshine channel send s3-cross --by main "2. API 改完再改 UI：结算页加收货地址输入框，缺了不让提交"
sunshine channel messages s3-cross --raw
# {"by":"main","cwd":"/tmp/scenes/s3-cross","kind":"create","origin":"cli","scope":"project","seq":1,"task":".sunshine/tasks/11-13-下单必填收货地址","ts":"2026-11-13T04:02:50.625514611Z","type":"chat"}
# {"by":"main","kind":"message","origin":"cli","seq":2,"text":"1. 先改 API：POST /orders 缺 address 返回 400，错误码 ADDRESS_REQUIRED","ts":"2026-11-13T04:02:50.639038496Z"}
# {"by":"main","kind":"message","origin":"cli","seq":3,"text":"2. API 改完再改 UI：结算页加收货地址输入框，缺了不让提交","ts":"2026-11-13T04:02:50.650859117Z"}
```

channel 和任务绑在一起，消息里能看到任务路径；这两条是带序号的协调消息，不是随手一句话。等 worker 回复等了 10 秒就超时了，这是正常的——本节的协调只做到发消息这一步，拉起真实 worker 的前提是本机有已登录的 claude / codex（第 10 节讲过）：

```sh
time sunshine channel wait s3-cross --from impl --kind done --timeout 10s
# 错误：超时：等待 channel 's3-cross' 超时（10s），仍在等待：impl（上游退出码 124）
# EXIT:1
# real  0m10.013s
```

实现完成，派 `sunshine-check` 子代理核查。跑给你看的结果是真实的：缺地址下单返回 400，带地址下单成功，订单里有地址：

```sh
curl -s -X POST localhost:3000/api/orders -H 'Content-Type: application/json' -d '{"items":[{"id":1,"qty":1}]}' -w "\nHTTP %{http_code}\n"
# {"code":"ADDRESS_REQUIRED","data":null}
# HTTP 400

curl -s -X POST localhost:3000/api/orders -H 'Content-Type: application/json' -d '{"items":[{"id":1,"qty":1}],"address":"北京市朝阳区建国路 1 号"}' -w "\nHTTP %{http_code}\n"
# {"code":"OK","data":{"id":1,"items":[{"id":1,"qty":1}],"address":"北京市朝阳区建国路 1 号","status":"待付款"}}
# HTTP 200
```

另外核对过：`GET /api/products` 返回的价格还是 8900/29900/129900（分）；结算页里有 `name="address"` 的输入框；空串、空白地址同样返回 400 `ADDRESS_REQUIRED`——问答里没单独问，实现按“必填”语义处理，写进了 spec。

> **Claude Code：** 3.3 回写 spec：`backend.md` 新增一节“下单必填收货地址”——`POST /api/orders` 缺 `address`（缺失、空串、空白）返回 400，错误码 `ADDRESS_REQUIRED`，`data` 为 `null`；订单记录含 `address` 字段，存下单时传的原样字符串。提交计划如下，回复“行”就执行：
>
> ```text
> 建议提交（按顺序）：
>   1. 下单必填收货地址：结算页加地址输入框，缺地址返回 400（ADDRESS_REQUIRED）
>      - public/checkout.html
>      - src/routes/orders.js
>      - .sunshine/spec/backend.md
> 未识别的脏文件（不在任何提交里——请确认包含还是排除）：
>   - .sunshine/.current-task
>   - .sunshine/tasks/11-13-下单必填收货地址/
> ```
>
> **你：** 行。
>
> **Claude Code：** 提交完了：`da27e80 下单必填收货地址：结算页加地址输入框，缺地址返回 400（ADDRESS_REQUIRED）`（3 个文件）。运行 `/finish-work` 归档、记日志，要现在执行吗？
>
> **你：** 可以。

归档前先切了功能分支（`main` 也是 `base_branch`，直接归档会被拒）：`git checkout -b feature/address-required` + `task set-branch`。`finish-work` 归档了任务，记了会话日志：

```sh
git log --oneline
# c229570 chore: record journal
# c956c40 chore(task): archive 11-13-下单必填收货地址
# da27e80 下单必填收货地址：结算页加地址输入框，缺地址返回 400（ADDRESS_REQUIRED）
# ed97a6e 12.2 商城 baseline：Express 商城 + 约定进 spec
```

结算页里也有地址输入框了，任务照常走完、归档了：

```text
[2026-11] - 1 个任务
  - 11-13-下单必填收货地址/
```

跨层任务的验收标准也在清单里：四层全改完，验收的那一份规范逐条过——哪层漏了，清单上看得见。

### 12.4 重构老模块：先钉住行为，再动代码

代码能跑，但已经难以安全修改——重构的第一步不是动手改，而是把现有行为钉住。演示用的老模块是商城里的价格计算函数，购物车和订单总价都走它，折扣规则全写死在 `if` 里：

```go
// 老模块：能跑，但折扣规则全写死在 if 里，改一次怕一次。
func Price(qty int, vip bool) int {
	total := qty * 100
	if qty >= 10 {
		total = total * 9 / 10
	}
	if vip {
		total = total * 95 / 100
	}
	return total
}
```

在 Claude Code 里提需求（Codex、Cursor 同理）：

示例提示词（示例，不是真实会话记录）：

> 我要重构一个老模块：商城里算价格的函数 Price，折扣规则全写死在 if 里，购物车和订单总价都走它，代码能跑但已经难以安全修改。请按 sunshine 工作流走（这次用 TDD 模板）：先向我提问，确认要钉住的行为（比如各档数量和 VIP 的预期价格）和重构边界；确认后，帮我创建任务、写好 PRD，再按 TDD 走：先写特征测试钉住现有行为，再重构。

下面是接下来的会话示意（对话是示例；命令与输出是真实实跑值）：

> **Claude Code：** 先问一个问题：要钉住的行为是哪几档——数量和 VIP 的预期价格分别是多少？
>
> **你：** 钉三档：1 件非 VIP 100，10 件非 VIP 900，10 件 VIP 855。
>
> **Claude Code：** 好，再问一个：重构边界到哪——只抽折扣规则，`Price` 的签名和返回值不动，对吗？
>
> **你：** 对，只抽折扣规则，签名不动，行为不能变。
>
> **Claude Code：** 好，问完了，给你一份最终规划总结，确认后我再动手：
>
> **最终规划总结**
>
> - 目标：把 `Price` 里写死在 `if` 里的折扣规则抽出来，改规则只改一处；行为一分不能变。
> - 范围内：`calc.go` 折扣规则抽成常量与 `applyBulk` / `applyVIP`，`Price` 只做组合；新增 `calc_test.go` 特征测试钉三档；购物车（`CartTotal`）与订单总价（`OrderTotal`）继续走 `Price`，不改调用方。
> - 范围外：不改签名/返回值；不改折扣数值与叠加顺序（先批量、后 VIP）；不加新折扣档、不做性能优化。
> - 关键决策（均已确认）：先写特征测试钉住现有行为（全绿是基线），再重构；重构后同一套测试再跑一次还是全绿。
> - 风险/延期项：无。整数除法顺序与原实现一致（先乘后除），三档覆盖满件与 VIP 叠加。
> - 产物状态：`prd.md` 已写；`implement.jsonl`/`check.jsonl` 各 1 条（`.sunshine/spec/backend.md`），`validate` 全过；轻量任务，无 `design.md`/`implement.md`。
>
> 批准这份规划后，我写 PRD 和上下文清单，仍不碰产品代码、不启动。没问题吗？
>
> **你：** 批准。
>
> **Claude Code：** PRD 和上下文清单已落盘，零产品代码，未启动。确认一下，批准后我切 TDD 模板、启动任务、开始实现。
>
> **你：** 批准。
>
> **Claude Code：** 工作流切到 TDD 模板了，任务建好了，启动了。先写特征测试把三档钉住：

```sh
sunshine workflow --template tdd
# ✓ 已把 .sunshine/workflow.md 换成 "tdd"
sunshine task create "重构价格计算" --description "把 Price 的折扣规则从 if 里抽成常量与两个小函数，签名与行为不变；购物车和订单总价继续走 Price。" --base-branch main
# 已为本会话激活任务：.sunshine/tasks/11-13-重构价格计算
sunshine task set-branch 11-13-重构价格计算 refactor-price
# ✓ 分支已设为：refactor-price
sunshine task start 11-13-重构价格计算
# ✓ 状态：planning → in_progress
```

然后写特征测试（characterization test）——不是测“想要的行为”，是把现有行为原样钉住：

```go
func TestPrice(t *testing.T) {
	cases := []struct {
		qty  int
		vip  bool
		want int
	}{
		{1, false, 100},
		{10, false, 900},
		{10, true, 855},
	}
	for _, c := range cases {
		if got := Price(c.qty, c.vip); got != c.want {
			t.Errorf("Price(%d, %v) = %d, want %d", c.qty, c.vip, got, c.want)
		}
	}
}
```

跑给你看，全绿——测试全绿是基线，这一步没改任何业务代码，只证明了“现在的行为就是这三行用例”。实跑第一遍就是绿灯：老代码本身就输出这三档值，特征测试只能钉住现有行为，不可能红；不要写跑红：

```sh
go test ./...
# ok  	shop	0.003s
```

接下来才重构：折扣规则抽成常量与两个小函数，`Price` 只做组合：

```go
// 折扣规则集中在这里改；Price 只做组合。
const (
	unitPrice = 100 // 单价（分）
	bulkQty   = 10  // 满多少件打折
	bulkRate  = 90  // 批量折扣：九折
	vipRate   = 95  // 会员折扣：九五折
)

func Price(qty int, vip bool) int {
	total := qty * unitPrice
	total = applyBulk(total, qty)
	total = applyVIP(total, vip)
	return total
}
```

重构后同一套测试再跑一次，还是全绿——行为没变，改动才算成立。TDD 工作流里的红灯是给新行为的，纯重构的顺序是反过来的：**先写特征测试钉住行为（绿灯），再重构（还是绿灯）**：

```sh
go test ./...
# ok  	shop	0.011s
go test -count=1 ./...
# ok  	shop	0.003s
go test -count=1 -v ./...
# === RUN   TestPrice
# --- PASS: TestPrice (0.00s)
# PASS
# ok  	shop	0.004s
```

（重构后第一遍是 `0.011s`，含编译；`-count=1` 重跑是 `0.003s`，与重构前一致。）派 `sunshine-check` 子代理核查：`func Price(qty int, vip bool) int` 签名未变，`cart.go` 与 `order.go` 仍走 `Price`，`go vet` 退出 0，`go fmt` 干净，无问题可修。

> **Claude Code：** 3.3 回写 spec：把新约定写进 `backend.md`，以后改规则先改用例表：

```markdown
## 价格计算

- 折扣规则集中在 calc.go 的常量与 applyBulk / applyVIP，Price 只做组合；改规则先改 calc_test.go 的用例表。
```

> **Claude Code：** 提交计划如下，回复“行”就执行：
>
> ```text
> 建议提交（按顺序）：
>   1. refactor: 把 Price 的折扣规则抽成常量与小函数
>      - calc.go
>      - calc_test.go
>      - .sunshine/spec/backend.md
> 未识别的脏文件（不在任何提交里——请确认包含还是排除）：
>   - .sunshine/workflow.md（TDD 模板切换，收尾切回 native 后与基线一致）
>   - .sunshine/.template-hashes.json（模板切换的附带记录，同上）
>   - .sunshine/tasks/11-13-重构价格计算/（任务产物，由归档提交带走）
>   - .sunshine/.current-task（当前任务指针，归档时清理）
> ```
>
> **你：** 行。
>
> **Claude Code：** 提交完了：`f73b1e3 refactor: 把 Price 的折扣规则抽成常量与小函数`（3 个文件，+46/−5）。运行 `/finish-work` 归档、记日志，要现在执行吗？
>
> **你：** 可以。

`finish-work` 归档了任务，记了会话日志：

```sh
git log --oneline -5
# 2dc676c chore: record journal
# 50c33dc chore(task): archive 11-13-重构价格计算
# f73b1e3 refactor: 把 Price 的折扣规则抽成常量与小函数
# e293a24 chore: sunshine init
# 3fb2275 chore: 初始化商城价格计算场景
```

任务照常归档了：

```text
[2026-11] - 1 个任务
  - 11-13-重构价格计算/
```

收尾时工作流切回 native——直接切会报错（`workflow.md` 已有本地修改），要加 `--force`：

```sh
sunshine workflow --template native
# 错误：.sunshine/workflow.md 已有本地修改，请加 --force 覆盖，或用 --create-new 写 .sunshine/workflow.md.new
sunshine workflow --template native --force
#   ✓ 已把 .sunshine/workflow.md 换成 "native"
```

切回后 `workflow.md` 与 `.template-hashes.json` 均恢复到基线。

### 12.5 修复反复出现的 bug：先找上次的会话

同类问题多次出现时，最贵的是每次都从零查起。这一节的 bug 是真实发生过的：商城项目派 codex worker 连不上，报 401——11 月 9 日出现过一次，11 月 10 日凌晨又出现一次，今天再跑还是 401。

在 Claude Code 里提需求（Codex、Cursor 同理）：

示例提示词（示例，不是真实会话记录）：

> 我要修复一个反复出现的 bug：商城项目派 codex worker 连不上，报 401——11 月 9 日出现过一次，11 月 10 日凌晨又出现一次，今天再跑还是 401。请按 sunshine 工作流走：先用 mem 找上次相关的会话，再向我提问，确认这次是不是同一类问题、复现条件是什么；确认后，帮我创建任务、写好 PRD，修完把教训写进 spec。

下面是接下来的会话示意（对话是示例；命令与输出是真实实跑值）：

> **Claude Code：** 我先用 mem 找了上次相关的会话：

```sh
sunshine mem search 401 --global
# 范围：全部项目  平台=all  关键词="401"
#
# [codex   ] 2026-11-10 05:05  01a1c3d8-457  /tmp/scenes/s5-bug  相关度=1.000  命中=1（用户=1，助手=0）  轮次=1
#     [用户] 线上 codex worker 又连不上了，之前报过 401，帮我查一下这次的原因
#
# 共 1 个会话
```

> **Claude Code：** 找到了 11 月 10 日 05:05 的一次派发，发给 worker 的消息里就写了 401。先确认一个问题：这次是不是同一类问题——商城项目派 codex worker，连不上报 401？
>
> **你：** 是同一类问题，今天再跑还是 401。
>
> **Claude Code：** 好，再问一个：复现条件是什么，在本机跑还是线上？
>
> **你：** 就在本机复现，codex 没登录。
>
> **Claude Code：** 好，问完了，给你一份最终规划总结，确认后我再动手：
>
> **最终规划总结**
>
> - 目标：复现 401、定位根因、教训写进 system.md。
> - 范围内：本机建 channel 发原话拉起 codex worker、确认 worker.log 里的 401、根因确认、写教训。
> - 范围外：不改商城业务代码、不代用户登录 codex、不修 codex/channel 本身、不处理 401 以外的派发失败。
> - 关键决策（均已确认）：复现只验证到 401 出现为止，登录问题留给用户本人处理。
> - 风险/延期项：无（401 已是第三次出现，根因指向未登录）。
> - 产物状态：PRD + 上下文清单（system.md、backend.md），零产品代码。
>
> 批准这份规划后，我写 PRD（描述里写清会话编号 01a1c3d8）和上下文清单，仍不碰产品代码、不启动。没问题吗？
>
> **你：** 批准。
>
> **Claude Code：** PRD 和上下文清单已落盘（`implement.jsonl`/`check.jsonl` 各 2 条，`task validate` 全部通过），零产品代码，未启动。确认一下，批准后我启动任务、开始复现。
>
> **你：** 批准。
>
> **Claude Code：** 启动任务，派 `sunshine-implement` 子代理复现。我先在本机复现一次当时的派发：建了个 channel，给 worker 发了那句话，拉起 worker——worker 拉起来了，但随即在日志里报了错（本机 codex 没登录，第 10 节讲过的前提；`ls ~/.codex/auth.json` → `No such file or directory`）：

```sh
sunshine channel create codex-401 --type chat --task .sunshine/tasks/11-13-codex-worker-401
# 已创建 channel 'codex-401'（chat）位于 /home/hatch/.sunshine/channels/-tmp-s5-repro/codex-401
sunshine channel spawn codex-401 --provider codex --as worker
# {"pid": 7235, "log": "/home/hatch/.sunshine/channels/-tmp-s5-repro/codex-401/worker.log", "worker": "worker"}
sunshine channel send codex-401 --to worker "线上 codex worker 又连不上了，之前报过 401，帮我查一下这次的原因"
# {"by":"main","kind":"message","origin":"cli","seq":3,"text":"线上 codex worker 又连不上了，之前报过 401，帮我查一下这次的原因","to":"worker","ts":"2026-11-13T04:01:49.517881231Z"}
```

worker.log 里的 401 行（逐字，首现 2026-11-13T04:01:49.049897Z），共出现 7 次：

```text
failed to connect to websocket: HTTP error: 401 Unauthorized, url: wss://api.openai.com/v1/responses
```

根因行（逐字）：

```text
unexpected status 401 Unauthorized: Missing bearer or basic authentication in header, url: wss://api.openai.com/v1/responses
```

worker 随后自行重试 `Reconnecting... 2/5` 至 `5/5`——这正是“不要反复重派”的实证。等 worker 回复等到的是一个终态错误事件（seq 16），随后把 worker 停掉：

```sh
sunshine channel wait codex-401 --from worker --kind done,error --timeout 2m
# {"by":"worker","kind":"error","message":"unexpected status 401 Unauthorized: Missing bearer or basic authentication in header, url: https://api.openai.com/v1/responses, cf-ray: a59ae10cfee192d0-DFW, request id: req_856bb946182d43f9a9620b912c13e4cc","origin":"worker","seq":16,"ts":"2026-11-13T04:02:06.752406346Z"}
sunshine channel kill codex-401 --as worker
```

（注意：`kill codex-401 worker` 位置参数写法会报错 `accepts 1 arg(s), received 2`，须用 `--as worker`。）

> **Claude Code：** 派 `sunshine-check` 子代理核查，全绿，无需修复：401 日志行 7 次精确命中；根因是 codex 未登录（`~/.codex/auth.json` 仍不存在）；教训位置在 system.md:17；业务代码未动（`git diff --stat -- server.js package.json` 为空）；mem 搜 401 现在是 2 个会话——新增本次复现的 `01a1d311-99c`，原来的 `01a1c3d8-457` 还在。教训已写进 spec：

```markdown
## 多 Agent 派发

- 派发 codex worker 前，先确认本机 codex 已登录（`~/.codex/auth.json` 存在；未登录时 worker 拿不到 bearer token，启动即 401）。
- spawn 失败时先看 worker.log，不要反复重派：`failed to connect to websocket: HTTP error: 401 Unauthorized, url: wss://api.openai.com/v1/responses` 即未登录（根因：`Missing bearer or basic authentication in header`），重派只会重复报 401。
- 复现记录：2026-11-13，商城项目派 codex worker 连不上报 401（此前 11 月 9 日、11 月 10 日凌晨已出现两次）；mem 搜 401 命中会话 01a1c3d8（2026-11-10 05:05）。
```

一个实测得出的边界要记住：**`mem` 搜的是对话轮次（用户/助手）里的文字，不搜错误事件**。11 月 10 日凌晨那个会话（`01a1c376`，`/tmp/h-test/chanproj`）的 worker 同样报了 401，但它的 401 只在错误事件里，对话里没有，所以搜 `401` 搜不到它——要找的是当时发给 worker 的那句话。这次能搜到，是因为发给 worker 的消息里就写了 401。直接 grep 会话文件证实过：`01a1c376` 的 401 只出现在 `event_msg/task_complete` 的 error 字段里，而 `01a1c3d8` 有用户原话。11 月 9 日那次也一样：`01a1bfa4-3c6` 的会话文件里确有同一 401 错误行，`01a1bfa3` 里没有。

找到上次的会话、确认是同一类问题之后，再建任务——描述里写清会话编号（01a1c3d8），agent 不用重新问一遍“上次是怎么回事”。任务做完，教训要写进 spec，否则下次还是一样——这是“反复出现”的真正解法。

> **Claude Code：** 3.3 回写 spec：除本教训外无其他新知识，backend.md 无需改动。提交计划只有一条，回复“行”就执行：
>
> ```text
> 建议提交（按顺序）：
>   1. docs: codex worker 401 教训写进 system.md（多 Agent 派发）
>      - .sunshine/spec/system.md
> 未识别的脏文件（不在任何提交里——请确认包含还是排除）：
>   - .sunshine/.current-task（任务会话指针，归档时自动清理）
>   - .sunshine/tasks/11-13-codex-worker-401/（任务目录，由 finish-work 的归档提交管理）
> ```
>
> **你：** 行。
>
> **Claude Code：** 提交完了：`9a4b2bd docs: codex worker 401 教训写进 system.md（多 Agent 派发）`（1 个文件，6 行新增）。只改了 spec 文件，所以前缀用 `docs:` 而非 `fix:`。运行 `/finish-work` 归档、记日志，要现在执行吗？
>
> **你：** 可以。

归档前先切了功能分支（`master` 也是 `base_branch`，直接归档会被拒）：`git checkout -b fix/codex-worker-401` + `task set-branch`。`finish-work` 归档了任务，记了会话日志：

```sh
git log --oneline
# c29bd5e chore: record journal
# 38775f6 chore(task): archive 11-13-codex-worker-401
# 9a4b2bd docs: codex worker 401 教训写进 system.md（多 Agent 派发）
# bfe6581 chore: sunshine init（后端）
# 6ed4c18 chore: 商城后端初始代码
```

任务照常归档了：

```text
[2026-11] - 1 个任务
  - 11-13-codex-worker-401/
```

下次再报 401，agent 会先读 spec 查登录状态，而不是重新派一个 worker 去撞同一堵墙。

### 12.6 产品设计先行：需求 → 设计文档 → 拆解 → 开发

这个场景从零开始：一个全新的 web 商城项目，走设计先行。跟 12.1 的区别是模式不同——12.1 是新项目直接开发，这个场景是新项目先设计再开发：需求问清楚、设计文档落盘、评审通过、拆解，最后才逐个动手写代码。设计人员和开发人员走同一条工作流，但进场、退场的时间不一样——下面每段开头都标了谁在场。

**[设计人员进场]**

在 Claude Code 里提需求（Codex、Cursor 同理）：

示例提示词（示例，不是真实会话记录）：

> 我想从零做一个带优惠券的 web 商城，走设计先行。请按 sunshine 工作流走。
>
> 设计阶段按需求循环，一个需求一个需求来：每条需求先向我提问，问清这条的产品逻辑（一次只问一个问题，附上你的建议与取舍），答案立即记进 prd.md，这条写进 doc/design/需求列表.md 的 R-xxx 行，再按需求列表派 prototype 子代理做/改这条需求对应的原型页。原型是多页面 HTML 演示版，带 mock 数据，不用问真实还是演示；设计阶段不问技术栈。
>
> 循环结束后按需生成交付客户的文档（概要设计是可能之一），然后给我最终规划总结请确认——这时需求列表、概要设计、原型页都已经在 doc/ 里了。评审通过后，按需求拆成子任务再动手实现。
>
> 商城要有商品列表、加入购物车、下单结算，下单时能用优惠券抵扣，还要有一个优惠券列表页能看到每张券的门槛、面额、状态和有效期。

下面是接下来的会话示意（对话是示例；命令与输出是真实实跑值）：

设计先行不是先把所有需求问完再出整包设计，而是按需求循环：每条需求一轮（问答 → 需求列表落盘 → 派 prototype 子代理做原型），循环到设计人员与用户都认为当前批次够了才停，再进评审与拆解。范围不先定死——循环从开场提示词已有的需求起步（起始批次，不是定死的范围）；共用约束与共用数据在首次需要时定；范围外边做边定。

#### 第 1 轮：R-001 商品列表

> **Claude Code：** 商品数据用什么？A 三件商品——无线鼠标 8900 分/库存 120、机械键盘 29900 分/库存 45、27 寸显示器 129900 分/库存 20；B 再加商品。
>
> **你：** A，按给定数据。

> **Claude Code：** 价格、接口、端口怎么定？A 价格用“分”存、接口统一返回 `{code, data}`、端口 3000；B 各自再定。
>
> **你：** A，这三项是共用约束，这轮定下。

需求列表落盘：45 行，新增 R-001 行（商品列表页展示全部商品：名称、价格、库存；GET /api/products 返回三件商品）。

prototype 派发：输入三样——需求列表（当前状态）+ spec 前端样式约束（`.sunshine/spec/frontend.md`）+ 已有原型目录 `doc/prototype/`。子代理汇报：新增 1 页。商品列表页.html 72 行；mock.js 8 行（3 条商品）；index.html 38 行；模板占位页 示例页.html 已删。

#### 第 2 轮：R-002 加入购物车

> **Claude Code：** 购物车放哪里？A 放浏览器 localStorage，不设后端接口；B 后端存购物车。
>
> **你：** A，localStorage。

需求列表落盘：46 行，新增 R-002 行（商品列表页点“加入购物车”，购物车页展示已加商品、数量、合计；购物车放浏览器 localStorage，不设后端接口）。

prototype 派发：同上（需求列表当前状态 + 前端样式约束 + 已有原型目录）。子代理汇报：新增 1 页。购物车页.html 145 行；index.html 39 行；mock.js 未改动。购物车 localStorage key 为 `sunshine_cart`；演示默认 1 键盘 + 2 鼠标，合计 47700 分（node 验算：29900 + 8900 × 2 = 47700）。

#### 第 3 轮：R-003 下单结算

> **Claude Code：** 收货地址必填吗？缺地址怎么办？A 必填，缺地址返回 400 ADDRESS_REQUIRED；B 选填。
>
> **你：** A，必填。

需求列表落盘：47 行，新增 R-003 行（结算页填收货地址提交订单；收货地址必填，缺地址返回 400 ADDRESS_REQUIRED；带地址下单成功）。

prototype 派发：同上。子代理汇报：新增 1 页。结算页.html 132 行；index.html 40 行；mock.js 未改动。地址为空提示“请填写收货地址”，带地址提交显示“下单成功，订单状态：待付款”。券码区未加、只在源码注释预留 R-004 位置。

#### 第 4 轮：R-004 下单用优惠券

> **Claude Code：** 券校验顺序与错误码怎么定？A 不存在 → 已使用 → 已过期 → 不满门槛，错误码 COUPON_INVALID / COUPON_USED / COUPON_EXPIRED / COUPON_THRESHOLD；B 只分能用/不能用。
>
> **你：** A，按四档顺序。

> **Claude Code：** 券码怎么提交、谁校验？A 结算页输入券码，随订单提交、后端校验抵扣；B 先领券再下单。
>
> **你：** A，输入券码随订单提交。

券数据三张在本轮首次需要时定下：MAN100JIAN10（满 10000 分减 1000 分，未使用，2026-12-31）、MAN200JIAN30（满 20000 分减 3000 分，已使用）、NEW5（新人 5 元券，已过期，2026-10-01）。

需求列表落盘：48 行，新增 R-004 行（结算页输入券码，满 100 元减 10 元；券须未使用且未过期、且满足门槛，否则下单失败并提示原因）。

prototype 派发：同上，明确与 R-003 共用结算页（加券码区，不重建）。子代理汇报：新增 0 页。结算页.html 215 行（券码输入框第 49 行、门槛提示“满 100 元减 10 元”第 50 行、校验逻辑第 121–189 行）；mock.js 17 行（商品 3 条 + 券 3 条）；index.html 未改。node 六种情况验算全过（1 键盘实付 28900、2 鼠标实付 16800、1 鼠标不满门槛）。

#### 第 5 轮：R-005 优惠券列表

> **Claude Code：** 券列表页要展示哪些字段？A 券名、门槛、面额、状态、有效期；B 只显示券名和面额。
>
> **你：** A，五项全展示。

需求列表落盘：49 行，新增 R-005 行（优惠券页展示全部券：券名、门槛、面额、状态、有效期；GET /api/coupons 返回三条券）。

prototype 派发：同上。子代理汇报：新增 1 页。优惠券页.html 86 行；index.html 41 行；mock.js 未改动。最终 4 页内容页 + 导航入口。

#### 收尾问答

> **Claude Code：** 还有哪些明确不做？A 不做支付、登录、券的发放/领取/核销后台、过期自动清理、数据库、缓存；B 都做。
>
> **你：** A，这些都不做，记进 prd.md“范围外”。

双方确认当前批次够了，循环结束，进评审与拆解。

八个维度（用户与场景、功能边界、数据、性能、安全、兼容性、验收、范围外）到这里全覆盖了：性能是单机演示、无特殊并发要求，安全是券码只按精确匹配校验，兼容性是价格分存、接口 `{code, data}`、端口 3000——这三项在问答与需求列表的非功能需求里已确认，不再单独提问。没有阻塞性开放问题，可以进入设计。

概要设计在循环结束后写——开场提示词点名要这份文档（按需生成，不是一定要出的清单项）。设计包最终落盘在项目 `doc/`，不在 `.sunshine`：

```text
doc/design/需求列表.md        # 49 行：R-001～R-005（第 23–27 行）、待确认事项（第 39–44 行，空）、变更记录（第 45–49 行）
doc/design/概要设计.md        # 78 行：目标与范围、模块划分、关键流程、接口（第 38–40 行）、数据、技术选型（第 55–64 行，[待定] 第 60–63 行）、验收标准、风险
doc/prototype/index.html       # 41 行：导航入口，列 4 页与对应需求
doc/prototype/mock.js          # 17 行：共用 mock，商品 3 条、券 3 条
doc/prototype/商品列表页.html  # 72 行：R-001
doc/prototype/购物车页.html    # 145 行：R-002
doc/prototype/结算页.html      # 215 行：R-003 + R-004（券码输入框第 49 行、门槛提示“满 100 元减 10 元”第 50 行、校验逻辑第 121–189 行）
doc/prototype/优惠券页.html    # 86 行：R-005
```

5 条需求对应 4 页原型——R-003 下单结算与 R-004 下单用优惠券共用结算页（R-004 是结算页里的券码输入区），其余一需求一页。原型是每轮派 `sunshine-prototype` 子代理做的，输入三样（需求列表当前状态 + spec 前端样式约束 + 已有原型目录），汇报只写页数、mock 数、路径；init 自带的模板 `doc/prototype/示例页.html` 是占位示例，不是真实页面，第 1 轮已删掉，`index.html` 按实际页面重写过。

需求列表按需求分条，每条写清对应系统原型里哪一页（真实文件内容，节选）：

```text
| R-001 | 商品列表 | 商品列表页展示全部商品：名称、价格（元）、库存；GET /api/products 返回三件商品 | P0 | GET /api/products 返回三件商品：无线鼠标 8900 分 / 库存 120、机械键盘 29900 分 / 库存 45、27寸显示器 129900 分 / 库存 20 |
| R-002 | 加入购物车 | 商品列表页点“加入购物车”，购物车页展示已加商品、数量、合计；购物车放浏览器 localStorage，不设后端接口 | P0 | 购物车页能展示已加商品与合计；刷新后购物车还在 |
| R-003 | 下单结算 | 结算页填收货地址提交订单；收货地址必填，缺地址返回 400 ADDRESS_REQUIRED；带地址下单成功 | P0 | 缺地址下单返回 400 ADDRESS_REQUIRED；带地址下单成功，返回订单 id、items、address、status“待付款” |
| R-004 | 下单用优惠券 | 结算页输入券码，满 100 元减 10 元；券须未使用且未过期、且满足门槛，否则下单失败并提示原因 | P0 | 券码 NOPE 下单返回 400 COUPON_INVALID；MAN200JIAN30 返回 400 COUPON_USED；NEW5 返回 400 COUPON_EXPIRED；1 件机械键盘用 MAN100JIAN10 实付 28900 分；2 件无线鼠标用 MAN100JIAN10 实付 16800 分；1 件无线鼠标用 MAN100JIAN10 返回 400 COUPON_THRESHOLD |
| R-005 | 优惠券列表 | 优惠券页展示全部券：券名、门槛、面额、状态、有效期；GET /api/coupons 返回三条券 | P0 | GET /api/coupons 返回三条券：MAN100JIAN10 满100减10 未使用、MAN200JIAN30 满200减30 已使用、NEW5 新人5元券 已过期 |
```

概要设计的接口行（真实内容）：

```text
| GET /api/products | 无 | { code, data: products }，三件商品 | 返回全部商品，不分页 |
| POST /api/orders | { items, address, couponCode } | { code, data: order }，实付写在 data.total | 缺地址返回 400 ADDRESS_REQUIRED；券校验顺序：不存在 → 已使用 → 已过期 → 不满门槛；失败返回 400 + 错误码 COUPON_INVALID / COUPON_USED / COUPON_EXPIRED / COUPON_THRESHOLD |
| GET /api/coupons | 无 | { code, data: coupons }，三条券 | 返回全部券，不分页 |
```

技术选型一节（概要设计第 55–64 行）只写约束与待定项，不强制拍板：约束是价格分存、接口 `{code, data}`、端口 3000、单机演示无特殊并发要求；语言、框架、数据库、缓存四项标 `[待定]`（第 60–63 行），留给后端基础子任务的 1.1 问答。这是 v0.5.5 的规则——后端基础工作量够大就单独立任务，选型项以 `[待定]` 标在它的 prd 里，它的 1.1 读到 `[待定]` 先问答。

---

**[设计评审：设计人员 + 开发人员都在场]**

设计包做完，先评审再拆解。评审就是 1.5 的最终规划总结——整包规划产物列出来请你确认，确认后才 `task start`：

> **Claude Code：** 最终规划总结请确认：目标是从零做带优惠券的 web 商城，设计先行；范围是 R-001～R-005 五条全做；范围外是支付、登录、发放/领取/核销后台、过期自动清理、数据库、缓存；关键决策是选型留给后端基础子任务、购物车放 localStorage、地址必填、券校验四档顺序；产物是 prd.md、需求列表 49 行、概要设计 78 行、原型 4 页 + mock，6 个子任务已建好并通过 validate，顺序是后端基础 → 商品列表 → 加入购物车 → 下单结算 → 下单用优惠券 → 优惠券列表。确认后我就逐个 start 开发，没问题吗？
>
> **你：** 没问题，开始吧。

---

**[拆解：1.2 只做一次]**

拆解只做一次，评审通过后一次建好全部 6 个子任务；拆完不自动开发，子任务建出来都是 planning。父任务只 create 不 start（自身无实现工作），拆解依据是需求列表（主线与契约）与 `doc/` 里实际有的文档：

```sh
sunshine task list
# 全部活动任务：
#
#   - 11-13-商城/ (planning) [0/6 已完成] [lin]
#     - 11-13-后端基础/ (planning) [lin]
#     - 11-13-商品列表/ (planning) [lin]
#     - 11-13-加入购物车/ (planning) [lin]
#     - 11-13-下单结算/ (planning) [lin]
#     - 11-13-下单用优惠券/ (planning) [lin]
#     - 11-13-优惠券列表/ (planning) [lin]
#
# 总计：7 个任务
```

规划产物提交 `b4bf106`（doc/ + 7 个任务文件）——设计包与任务结构先落一笔，再进开发。

后端基础子任务排在最前面，它的 1.1 读到 `[待定]` 先问答，一次一个：

> **Claude Code：** 语言选哪个？A Node.js；B Go。
>
> **你：** A，Node.js，商城演示轻量，前后端同语言。
>
> **Claude Code：** 框架？A Express；B 原生 http。
>
> **你：** A，Express。
>
> **Claude Code：** 数据库？A 不用数据库用内存；B SQLite。
>
> **你：** A，内存，数据量小，三件商品三张券，演示不需要持久化。
>
> **Claude Code：** 缓存？A 不用；B 加一层内存缓存。
>
> **你：** A，不用，内存数据本身就是最快的那层。

四项结论写进该子任务的 prd.md 与 design.md，`[待定]` 清零；选型定了以后不再改，后续子任务沿用。各需求子任务的 1.1 只简短确认范围（范围即各自 prd.md 里那条需求 + 验收标准），不重做全套 brainstorm，直接进 1.4。

每个子任务的 implement.jsonl / check.jsonl 只放规范、研究、doc/ 设计产物，明确不放代码文件（子代理在实现期间自己读，不用预先登记）；start 前两份 jsonl 各须至少一条真实条目，`task validate` 全过才 start。依赖写进 prd：后端基础归档后需求子任务再启动；R-004 依赖 R-003 的订单接口；R-005 依赖 R-004 的 coupons 数据。

---

**[开发进场]**

开发人员进场时，先用下面这段开场白请用户确认。

它先把子任务清单和第一个子任务的上下文列出来，确认后再逐个做。

> 设计评审通过了，子任务已经拆好。现在按 sunshine 工作流开始开发：先把父任务的子任务清单和第一个子任务的上下文列出来给我确认，确认后从第一个子任务开始，一个一个做；每个子任务先启动，做完检查、回写规范、提交、归档，再启动下一个；全部子任务归档后，父任务做最终集成复核再归档。

确认后，开发就按下面每个子任务的实录逐个走；中途关掉重开，跑 /sunshine:continue，它按 task.json 的状态分流续接。

逐个激活：一个子任务完整走完实现 → 检查 → 回写规范 → 提交 → 归档，后下一个才启动。下面是每个子任务的实录（对话是示例；命令与输出是真实实跑值）。

### 后端基础：技术选型与脚手架

先做后端基础子任务。它的 implement.jsonl 有 3 条（概要设计接口与数据、后端规范、本任务 design.md），check.jsonl 有 2 条（概要设计验收标准、本任务 prd.md），`task validate` 全过才 start。2.1 实现：`package.json`（单行 JSON，Express ^4.21.2，start 为 `node server.js`）、`server.js` 18 行（端口 3000，GET / 健康 JSON，托管 public/）、`src/db.js` 21 行（products / coupons / orders / nextOrderId=1）、根 `.gitignore`（node_modules/）、package-lock.json 自动生成；`npm install` 装上 express 4.21.2，共 68 个包。

两处推断（任务数据未给）：MAN200JIAN30 有效期填 2026-12-31（与 MAN100JIAN10 同期）；NEW5 门槛 0、面额 500 分。

2.2 检查：

```sh
curl -s -i http://localhost:3000/
# HTTP/1.1 200 OK
# {"code":0,"data":{"status":"ok"}}（健康 JSON）
curl -s http://localhost:3000/api/products
# Cannot GET /api/products（预期：路由尚未挂载）
```

3.3 回写规范：走了一遍判断，结论“没有要更新的”（选型与接口已记在任务 prd/design 与概要设计里，团队 spec 无新增约定）。工作提交 `d4845de` → 归档提交 `bfe26d0` → 日志提交 `0d2d697`（会话 1）。顺带一提：`.gitignore`（内容仅 `node_modules/`）是这次新建的——init 没生成仓库根 `.gitignore`，否则 `node_modules/` 会进工作区。

### 商品列表

implement.jsonl 有 3 条（概要设计接口 GET /api/products、商品列表页原型、本任务 prd.md），check.jsonl 有 2 条（概要设计验收标准、本任务 prd.md），`task validate` 全过。2.1 实现：`src/routes/products.js` 11 行（GET /api/products）、`server.js` 22 行（+5，挂载路由）、`public/index.html` 103 行（fetch /api/products、购物车写 localStorage、Loading 与错误态）。

2.2 检查：

```sh
curl -s http://localhost:3000/api/products
# {"code":0,"data":[{"id":1,"name":"无线鼠标","price":8900,"stock":120},{"id":2,"name":"机械键盘","price":29900,"stock":45},{"id":3,"name":"27寸显示器","price":129900,"stock":20}]}
curl -s -o /dev/null -w "%{http_code} %{size_download} bytes" http://localhost:3000/index.html
# 200, 4143 bytes
curl -s -o /dev/null -w "%{http_code}" http://localhost:3000/
# 200
```

3.3 回写规范：走了一遍判断，结论“没有要更新的”（接口遵循已有后端规范，无新模式、新决策）。工作提交 `3977045` → 归档提交 `b6fdb15` → 日志提交 `a6faa74`（会话 2）。

### 加入购物车

implement.jsonl 有 2 条（购物车页原型、本任务 prd.md），check.jsonl 有 2 条（购物车页原型、本任务 prd.md），`task validate` 全过。2.1 实现：`public/cart.html` 156 行（6680 字节），读 localStorage `sunshine_cart`、调 /api/products 解析、数量可改合计实时更新、刷新后还在、空态、“去结算”跳 checkout.html；生产版不预置演示数据（原型首次打开无数据时塞 1 键盘 + 2 鼠标），无数据即空态；另加下架商品自动剔除、越界数量夹紧。

2.2 检查：购物车没有后端接口，加购交互本身是浏览器 localStorage 行为，curl 点不了按钮；本次以页面可达 + 关键逻辑 grep + node 按 db.js 数据验算合计为准：

```sh
curl -s -o /dev/null -w "%{http_code} %{size_download} bytes" http://localhost:3000/cart.html
# 200, 6680 bytes（页面含 localStorage 4 处、fetch 1 处）
node -e "console.log('合计 = ' + (29900 + 8900 * 2) + ' 分 = ' + (29900 + 8900 * 2) / 100 + ' 元')"
# 合计 = 47700 分 = 477 元（1 件机械键盘 + 2 件无线鼠标）
```

3.3 回写规范：走了一遍判断，结论“没有要更新的”。工作提交 `1853417` → 归档提交 `faac8ce` → 日志提交 `ace811d`（会话 3）。

### 下单结算

implement.jsonl 有 3 条（概要设计接口 POST /api/orders、结算页原型、本任务 prd.md），check.jsonl 有 2 条（概要设计验收标准、本任务 prd.md），`task validate` 全过。2.1 实现：`src/routes/orders.js` 40 行（地址校验第 13 行，缺 / 空地址返回 400 ADDRESS_REQUIRED；id 取 `db.nextOrderId++`；total 按 price × qty 求和；status“待付款”；couponCode 只接收、不校验、不抵扣）、`server.js` 29 行（补了原先没有的 `express.json()` + 挂载路由）、`public/checkout.html` 176 行（地址输入框第 48 行，无券码区、只留 `<!-- R-004 券码输入区（预留位置） -->` 注释；下单成功后清空购物车）。

2.2 检查：

```sh
curl -s -X POST http://localhost:3000/api/orders -H "Content-Type: application/json" -d '{"items":[{"id":2,"qty":1}]}'
# {"code":"ADDRESS_REQUIRED","data":null}，HTTP 400（缺地址）
curl -s -X POST http://localhost:3000/api/orders -H "Content-Type: application/json" -d '{"items":[{"id":2,"qty":1}],"address":""}'
# {"code":"ADDRESS_REQUIRED","data":null}，HTTP 400（空地址也算缺地址）
curl -s -X POST http://localhost:3000/api/orders -H "Content-Type: application/json" -d '{"items":[{"id":2,"qty":1}],"address":"测试地址"}'
# {"code":0,"data":{"id":1,"items":[{"id":2,"qty":1}],"address":"测试地址","couponCode":null,"total":29900,"status":"待付款"}}，HTTP 200
curl -s -o /dev/null -w "%{http_code} %{size_download} bytes" http://localhost:3000/checkout.html
# 200, 7241 bytes
```

3.3 回写规范：走了一遍判断，结论“没有要更新的”（ADDRESS_REQUIRED 是本任务的合同，已在 design.md 与概要设计接口行里，无需进通用规范）。工作提交 `0d56a42` → 归档提交 `cc5b816` → 日志提交 `9f84869`（会话 4）。顺带一提：db.js 的 nextOrderId 按值导出，orders.js 用 `db.nextOrderId++` 直接改导出对象属性，跨请求可递增。

### 下单用优惠券

implement.jsonl 有 3 条（概要设计接口 POST /api/orders 券校验四档、结算页原型券码区、本任务 prd.md），check.jsonl 有 2 条（概要设计验收标准、本任务 prd.md），`task validate` 全过。2.1 实现：`src/routes/orders.js` 64 行（`checkCoupon` 在第 8 行，调用点第 42 行；顺序：不存在 → COUPON_INVALID、status 已使用 → COUPON_USED、status 已过期或 expires 早于当天 → COUPON_EXPIRED、小计不满 threshold → COUPON_THRESHOLD；全部通过按 discount 抵扣；couponCode 为空 / 未传时不校验、不抵扣；订单对象新增 couponCode 字段）、`public/checkout.html` 200 行（券码输入框第 49 行、门槛提示“满 100 元减 10 元”第 50 行）。coupons 内存表三条券已在后端基础的 db.js 里，无需再加。

2.2 检查，六档全跑（地址均为 `测试地址`）：

```sh
curl -s -X POST http://localhost:3000/api/orders -H "Content-Type: application/json" -d '{"items":[{"id":2,"qty":1}],"address":"测试地址","couponCode":"NOPE"}'
# {"code":"COUPON_INVALID","data":null}，HTTP 400（券码不存在）
curl -s -X POST http://localhost:3000/api/orders -H "Content-Type: application/json" -d '{"items":[{"id":2,"qty":1}],"address":"测试地址","couponCode":"MAN200JIAN30"}'
# {"code":"COUPON_USED","data":null}，HTTP 400（已使用）
curl -s -X POST http://localhost:3000/api/orders -H "Content-Type: application/json" -d '{"items":[{"id":2,"qty":1}],"address":"测试地址","couponCode":"NEW5"}'
# {"code":"COUPON_EXPIRED","data":null}，HTTP 400（已过期）
curl -s -X POST http://localhost:3000/api/orders -H "Content-Type: application/json" -d '{"items":[{"id":2,"qty":1}],"address":"测试地址","couponCode":"MAN100JIAN10"}'
# {"code":0,"data":{"id":1,"items":[{"id":2,"qty":1}],"address":"测试地址","couponCode":"MAN100JIAN10","total":28900,"status":"待付款"}}，HTTP 200（1 件键盘实付 28900）
curl -s -X POST http://localhost:3000/api/orders -H "Content-Type: application/json" -d '{"items":[{"id":1,"qty":2}],"address":"测试地址","couponCode":"MAN100JIAN10"}'
# {"code":0,"data":{"id":2,"items":[{"id":1,"qty":2}],"address":"测试地址","couponCode":"MAN100JIAN10","total":16800,"status":"待付款"}}，HTTP 200（2 件鼠标实付 16800）
curl -s -X POST http://localhost:3000/api/orders -H "Content-Type: application/json" -d '{"items":[{"id":1,"qty":1}],"address":"测试地址","couponCode":"MAN100JIAN10"}'
# {"code":"COUPON_THRESHOLD","data":null}，HTTP 400（1 件鼠标不满门槛）
```

3.3 回写规范：走了一遍判断，结论“没有要更新的”（校验顺序是业务规则，已在 design.md 与概要设计接口行里；backend.md 的补行留给变更演示）。工作提交 `7901f3e` → 归档提交 `cd2a51e` → 日志提交 `87afc67`（会话 5）。订单 id 每次重启服务后从 1 开始计（内存数据，重启即清空），属演示预期。

### 优惠券列表

implement.jsonl 有 3 条（概要设计接口 GET /api/coupons、优惠券页原型、本任务 prd.md），check.jsonl 有 2 条（概要设计验收标准、本任务 prd.md），`task validate` 全过。2.1 实现：`src/routes/coupons.js` 11 行（GET /api/coupons）、`server.js` 33 行（+4，挂载）、`public/coupons.html` 94 行（fetch /api/coupons，五字段，门槛 / 面额分转元，0 门槛显示“无门槛”，券名行附带券码，非“未使用”置灰标注“（不可用）”）。

2.2 检查：

```sh
curl -s http://localhost:3000/api/coupons
# {"code":0,"data":[{"code":"MAN100JIAN10","name":"满100减10","threshold":10000,"discount":1000,"status":"未使用","expires":"2026-12-31"},{"code":"MAN200JIAN30","name":"满200减30","threshold":20000,"discount":3000,"status":"已使用","expires":"2026-12-31"},{"code":"NEW5","name":"新人5元券","threshold":0,"discount":500,"status":"已过期","expires":"2026-10-01"}]}
curl -s -o /dev/null -w "%{http_code} %{size_download} bytes" http://localhost:3000/coupons.html
# 200, 3916 bytes
```

最后一轮 2.2 是全范围回归：GET /api/products 返回三件商品、POST /api/orders 六档（缺地址 400、1 件键盘 + MAN100JIAN10 实付 28900、2 件鼠标 + MAN100JIAN10 实付 16800、1 件鼠标 400 COUPON_THRESHOLD）、`/`、/index.html、/cart.html、/checkout.html、/coupons.html 全部 HTTP 200。

3.3 回写规范：走了一遍判断，结论“没有要更新的”。工作提交 `8c03060` → 归档提交 `8c4e04f` → 日志提交 `f37743d`（会话 6）。

---

**[父任务最终集成复核]**

六个子任务全部归档后，父任务 `[6/6 已完成]`，做最终集成复核再归档。复核读验收标准 + 各子任务检查结果 + 接口约定，未重读整个原型；fresh 服务启动后一次跑完：

```sh
curl -s http://localhost:3000/api/products
# {"code":0,"data":[{"id":1,"name":"无线鼠标","price":8900,"stock":120},{"id":2,"name":"机械键盘","price":29900,"stock":45},{"id":3,"name":"27寸显示器","price":129900,"stock":20}]}
curl -s http://localhost:3000/api/coupons
# 三条券（threshold 10000/20000/0，与变更前一致）
curl -s -X POST http://localhost:3000/api/orders -H "Content-Type: application/json" -d '{"items":[{"id":2,"qty":1}]}'
# {"code":"ADDRESS_REQUIRED","data":null}，HTTP 400
curl -s -X POST http://localhost:3000/api/orders -H "Content-Type: application/json" -d '{"items":[{"id":2,"qty":1}],"address":"测试地址","couponCode":"MAN100JIAN10"}'
# {"code":0,"data":{"id":1,"items":[{"id":2,"qty":1}],"address":"测试地址","couponCode":"MAN100JIAN10","total":28900,"status":"待付款"}}，HTTP 200
curl -s -X POST http://localhost:3000/api/orders -H "Content-Type: application/json" -d '{"items":[{"id":1,"qty":2}],"address":"测试地址","couponCode":"MAN100JIAN10"}'
# total 16800，HTTP 200
curl -s -X POST http://localhost:3000/api/orders -H "Content-Type: application/json" -d '{"items":[{"id":1,"qty":1}],"address":"测试地址","couponCode":"MAN100JIAN10"}'
# {"code":"COUPON_THRESHOLD","data":null}，HTTP 400
# 四页面均 HTTP 200：index.html、cart.html、checkout.html、coupons.html
```

父任务自身无代码提交（代码都在子任务工作提交里）；归档前先 `task set-branch 11-13-商城 feature/11-13-商城`。finish-work 步骤 1（`task context --mode record`）查到除父任务外无其他已完成未归档任务，“一并归档”问题 N=0，无需提问。归档提交 `13c9f37` → 日志提交 `90531a6`（会话 7；`--commit` 记的是规划产物提交 `b4bf106`，因父任务无独立工作提交，摘要里已写明这一点）。归档后 `task list` 活动任务 0 个，`task list-archive` 显示 2026-11 共 7 个任务。

---

**[变更演示：设计人员返场]**

父任务归档之后，用户改主意了：优惠券满减门槛从 100 元改成 200 元。设计先行怎么做变更？设计先改 `doc/` 与受影响原型页，父任务 prd 记 `[变更]`，1.5 只审变更；已归档新建返工不 reopen、进行中暂停回炉、没做的直接改。

先建变更任务。按 1.2 的做法想挂在父任务下面，但父任务已经归档了：

```sh
sunshine task create "满减门槛改成 200 元" --slug 满减门槛改成-200 --parent 11-13-商城 --no-start
# 错误：无法在 /tmp/scenes/s6-loop/.sunshine/tasks 下解析任务 "11-13-商城"
# 错误：父任务无法解析：11-13-商城
# 未创建任务。请传入已存在的任务目录。
```

已归档父任务下建不了子任务——这是 1.2 的含义：拆解只做一次，要么规划时把子任务都建好，要么父任务等全部子任务归档后再归档；返工任务挂不上已归档的父任务，只能独立建。改成独立任务，PRD 记 `[变更 2026-11-13]`：

```sh
sunshine task create "满减门槛改成 200 元" -d "优惠券满减门槛 100 元改为 200 元" --slug 满减门槛改成-200 --no-start
# 任务已创建：11-13-满减门槛改成-200
```

注意一个口径：`[变更 2026-11-13]` 不记进已归档父任务的 prd.md——归档的是历史，不 reopen；`[变更]` 只记在变更任务自己的 prd.md 与设计文档的变更记录里。已归档父任务 prd 至今仍写“满 100 元减 10 元”“2 件无线鼠标用 MAN100JIAN10 实付 16800 分”，是归档前的历史状态，不是遗漏。

设计返场改六处：

| # | 文件 | 行号 | 改动 |
| --- | --- | --- | --- |
| 1 | `doc/design/需求列表.md` | 26（R-004 行） | “满 100 元减 10 元”→“满 200 元减 10 元”；验收“2 件无线鼠标用 MAN100JIAN10 实付 16800 分”→“返回 400 COUPON_THRESHOLD” |
| 2 | `doc/design/需求列表.md` | 49（变更记录行） | 新增行：`2026-11-13 ｜ MAN100JIAN10 门槛从 10000 分改为 20000 分（满 100 元减 10 元改为满 200 元减 10 元）；券名“满100减10”未改 ｜ 门槛调整` |
| 3 | `doc/design/概要设计.md` | 70（验收标准 R-004 行） | 同第 1 处（验收同步改） |
| 4 | `doc/design/概要设计.md` | 78（风险行） | 改为“券名‘满100减10’与门槛 20000 分（200 元）不一致，名不副实（2026-11-13 由 10000 改为 20000，未改名）” |
| 5 | `doc/prototype/结算页.html` | 50（门槛提示行） | “满 100 元减 10 元”→“满 200 元减 10 元（券码 MAN100JIAN10，满 20000 分减 1000 分）” |
| 6 | `doc/prototype/mock.js` | 14 | MAN100JIAN10 `threshold: 10000`→`threshold: 20000`（原型共用 mock 同步；优惠券页按 mock 渲染，自动同步） |

改完后 grep 核对：旧值“10000”只剩变更记录与风险行里的历史记录；“16800”已无残留；“满100减10”只剩券名本身（有意不改名）。

变更任务的 1.5 只评审变更的部分（prd.md + 上述设计改动），确认后 `task start`。implement.jsonl / check.jsonl 各 3 / 2 条（需求列表 R-004 与变更记录、概要设计验收与风险、本任务 prd.md 含 `[变更]`），`task validate` 全过。implement 改三处：

| # | 文件 | 行号 | 改动 |
| --- | --- | --- | --- |
| 1 | `src/db.js` | 11 | MAN100JIAN10 `threshold: 10000`→`threshold: 20000` |
| 2 | `.sunshine/spec/backend.md` | 16（新增行） | `优惠券 MAN100JIAN10 门槛 20000 分（2026-11-13 由 10000 改为 20000；券名“满100减10”未改，名不副实）。` |
| 3 | `public/checkout.html` | 50、179 | 门槛提示“满 100 元减 10 元”→“满 200 元减 10 元”（第 50 行）；`COUPON_THRESHOLD` 文案“未满门槛，满 100 元减 10 元”→“未满门槛，满 200 元减 10 元”（第 179 行）；其它逻辑未动 |

变更后 curl 验证（地址均为 `测试地址`）：

```sh
curl -s -X POST http://localhost:3000/api/orders -H "Content-Type: application/json" -d '{"items":[{"id":1,"qty":2}],"address":"测试地址","couponCode":"MAN100JIAN10"}'
# {"code":"COUPON_THRESHOLD","data":null}，HTTP 400（2 件鼠标原来实付 16800，现在不满 200 元门槛）
curl -s -X POST http://localhost:3000/api/orders -H "Content-Type: application/json" -d '{"items":[{"id":2,"qty":1}],"address":"测试地址","couponCode":"MAN100JIAN10"}'
# {"code":0,"data":{"id":1,"items":[{"id":2,"qty":1}],"address":"测试地址","couponCode":"MAN100JIAN10","total":28900,"status":"待付款"}}，HTTP 200（1 件键盘仍实付 28900）
curl -s http://localhost:3000/api/coupons
# MAN100JIAN10 {"code":"MAN100JIAN10","name":"满100减10","threshold":20000,"discount":1000,"status":"未使用","expires":"2026-12-31"}（名仍“满100减10”，threshold 已是 20000）
```

工作提交 `3f80cf9`（7 个文件，+10/−9，设计返场 + 实现合一笔）→ 归档提交 `261bc6c` → 日志提交 `28eb3c1`（会话 8）。归档后 `task list-archive` 显示 2026-11 共 8 个任务。

---

最终 git log（旧 → 新，25 条提交；每个任务都是“工作提交 → 归档提交 → 日志提交”顺序）：

```text
fd8a2a1 init: 产品设计类型初始化（sunshine v0.5.6，新工作流：按需求循环）
b4bf106 规划产物：需求列表、概要设计、原型 4 页 + mock、7 个任务（设计先行，按需求循环）
d4845de 后端基础：Node.js/Express 脚手架与内存数据（选型：Node.js/Express/内存/无缓存）
bfe26d0 chore(task): archive 11-13-后端基础
0d2d697 chore: record journal
3977045 商品列表：GET /api/products 返回三件商品，生产版商品列表页调真实接口
b6fdb15 chore(task): archive 11-13-商品列表
a6faa74 chore: record journal
1853417 加入购物车：生产版购物车页，localStorage 购物车、数量可改合计实时更新
faac8ce chore(task): archive 11-13-加入购物车
ace811d chore: record journal
0d56a42 下单结算：POST /api/orders 地址必填校验，生产版结算页
cc5b816 chore(task): archive 11-13-下单结算
9f84869 chore: record journal
7901f3e 下单用优惠券：券校验四档 + 抵扣，结算页加券码输入区
cd2a51e chore(task): archive 11-13-下单用优惠券
87afc67 chore: record journal
8c03060 优惠券列表：GET /api/coupons 返回三条券，生产版优惠券页五字段
8c4e04f chore(task): archive 11-13-优惠券列表
f37743d chore: record journal
13c9f37 chore(task): archive 11-13-商城
90531a6 chore: record journal
3f80cf9 满减门槛改成 200：MAN100JIAN10 门槛 10000→20000 分（设计返场 + 实现）
261bc6c chore(task): archive 11-13-满减门槛改成-200
28eb3c1 chore: record journal
```

这个场景的闭环就是：空目录 `init` → 按需求循环（每条需求一轮：问答 → 需求列表落盘 → 原型该页），循环到双方认为当前批次够了才停 → 概要设计按需写完 → 评审（最终规划总结）→ 拆解一次建好 6 个子任务 → 逐个子任务走完实现、检查、回写规范、提交、归档 → 父任务集成复核后归档 → 变更走设计返场再闭环。设计先行没有强制的交付物清单——概要设计、需求列表、原型都不是硬性要求，只是这个场景里的设计包；设计产出由设计者定，1.5 审当前批次，拆解依据需求列表（主线与契约）与 `doc/` 里实际有的文档，没有设计文档就是纯代码任务。

几个边界，照实说：

- “加入购物车”按钮写 localStorage 的逻辑随 R-001 的商品列表页一起交付，R-002 的交付物是购物车页本身（读 localStorage 渲染与合计）及端到端验算——购物车没有后端接口，curl 点不了按钮，靠页面可达 + 关键逻辑 grep + node 验算合计。
- `task start` 本轮没留下任何会话记录：`.sunshine/.runtime/` 目录根本不存在，8 次 start 都没写会话文件；状态照常翻转，未阻塞。
- 归档与 add-session 的自动提交本轮 8 次全部一次成功——演示仓库事先已设本地 git 身份。
- `add-session` 留下的 `.gitattributes`（journal 的 `merge=union` 规则）8 次 journal 提交都没把它带进去，最终 `git status` 仍剩 `?? .gitattributes` 一条残留。
- `.sunshine/.developer` 没被 gitignore，已随基线提交 `fd8a2a1` 进库；init 也没生成仓库根 `.gitignore`，`node_modules/` 是后端基础子任务新建根 `.gitignore` 后才排除的。
- 两处子代理 exec 调用报 `subprocess.output_drain_timeout`（exitCode 0）：优惠券列表 implement 与变更 implement 各一次；汇报完整，结果已由主会话独立复核通过。
- 原型与生产的已知差异：原型 mock.js 的券只有 code、无 name 字段，页上券名显示为券码；生产 db.js 有 name 字段。生产版购物车页不预置演示数据（原型首次打开无数据时塞 1 键盘 + 2 鼠标），无数据即空态。
- 浏览器真实渲染未验证：沙箱无浏览器，原型页与生产页只做了 curl 取回（HTTP 200 + 关键逻辑 grep），页面长什么样、按钮点按反馈未在真实浏览器里看过。

## 附录：命令速查

主线走完了：项目骨架与规范在 `.sunshine/` 里，任务从创建到归档走了一遍，工作流切过 TDD，模板市场装过规范并验证了持续刷新，会话能回顾，多 Agent 的 channel 能建会话、发消息、开主题，CLI 也能自己升级。

平时你在 agent 界面里工作，下面的命令只在手动干预时用——每一行都是本教程前面实跑过的命令（`uninstall` / `ablate` / `restore` 的一句话说明来自各自的 `--help`，未在本教程里真执行）。agent 在底下调的命令（`task create/start/archive`、`task add-context/validate`、`channel create/send/spawn/wait`、`task add-session`）不在这里——那是 agent 的活，见第 4 节。

### init 参考

#### 交互时会问什么（不带 `-y` 时，按顺序共四问）

1. **选平台**（“请选择要配置的 AI 编程平台”）：22 家多选，默认勾选 Claude Code 与 Cursor；全不选则取消初始化（“未选择任何平台，初始化已取消。”）。
2. **状态栏确认**（只有 Claude Code 入选时才问）：“是否为 Claude Code 安装状态栏……？”默认不装；想直接装就传 `--with-statusline`。
3. **选规范模板**（“请选择规范模板”）：默认“从零开始”，也可以选“自定义”输一个模板市场来源再挑；带 `--registry` 时选单直接列出该市场的 spec 模板，选了就装它、不再生成空白规范。
4. **选项目类型**（只有检测不到时才问）：“检测不到项目类型，请选择”：前端项目 / 后端项目 / 前后端全栈 / 产品设计，默认全栈。选产品设计表示项目从产品设计先行：生成 `system.md`、`frontend.md`、`backend.md` 全套规范，外加 `doc/` 设计文档模板（需求列表、概要设计、原型）。

`-y` 跳过上面全部提问用默认配置；传了对应的 flag（平台 flag、`--type`、`--template`）也会跳过那一问。

项目类型检测读 `package.json` 的依赖与常见前后端配置文件（`vite.config.ts`、`go.mod` 等）判断前端/后端/全栈，只生成对应的规范——前端项目只生成 `system.md` 与 `frontend.md`，后端只生成 `system.md` 与 `backend.md`，全栈三份都有。产品设计不参与检测，只能手动选（交互提问或 `--type 产品设计`）。`--registry` 从模板市场装规范时不检测、不提问。

#### 常用选项

| 选项 | 作用 |
| --- | --- |
| `-u, --user <名字>` | 开发者名字，写进 `.sunshine/.developer`；优先级：`-u` > git user.name > `developer`；任务的创建人与负责人默认就是它 |
| `-y, --yes` | 跳过全部提问：默认平台 Claude Code + Cursor、默认全栈、默认空白规范、不装状态栏 |
| `-f, --force` | 已初始化时覆盖重建骨架与平台文件（见下“三种模式”） |
| `--type <前端/后端/全栈/产品设计>` | 显式指定项目类型，跳过检测与提问 |
| `--registry <来源>` | 模板市场来源：`gh:owner/repo`、完整 git URL、HTTP(S) URL、`git@`、本地 git 仓库路径 |
| `--template <id>` | 模板市场条目 id，须配合 `--registry`；不传时 `-y` 下市场型来源直接报错、交互下出选单 |
| `--overwrite` / `--append` | 目标已存在时覆盖 / 只补缺失文件；仅配合 `--template` 使用 |
| `--with-statusline` | 给 Claude Code 装状态栏；仅 Claude Code 入选时有效 |
| `--<平台>` | 只装指定平台：`--claude` `--cursor` `--codex` `--kiro` `--gemini` `--qoder` `--pi` `--zcode` `--trae` `--omp` `--snow` `--opencode` `--dsh` `--reasonix` `--grok` `--kimi` `--kilo` `--antigravity` `--devin` `--codebuddy` `--copilot` `--droid`（22 个；`--windsurf` 是已废弃的 `--devin` 别名，传了会自动转并警告） |

#### 三种模式

- **全新 init**：目录里没有 `.sunshine/`，按上面流程生成骨架与平台配置。
- **覆盖重建**（`-f`）：已初始化时仍重建骨架与平台文件。
- **增量追加**：已初始化时不带 `-f` 再跑 init，只追加新选的平台（也可以顺带用 `--registry` 装模板），不动已有平台文件与 `.sunshine/` 骨架。例如只想装 Codex 时用 `sunshine init --codex -y`。

注意：init 拒绝在用户主目录执行（设置 `SUNSHINE_ALLOW_HOMEDIR=1` 可绕过）。

#### init 装了什么

init 生成了 115 个文件（不含 `.git`），其中给 agent 读的是这些：

- `.sunshine/workflow.md`：完整版开发工作流（714 行），agent 按它推进任务。
- `.sunshine/AGENTS.md`：开头即“本文件面向在本仓库工作的 AI 编程助手”。
- `.sunshine/spec/`：三份规范模板（`system.md` 总原则、`backend.md` 后端、`frontend.md` 前端）。
- `.sunshine/config.yaml`：项目配置，带中文注释（关键字段见 [README.cn.md](../README.cn.md#配置要点)）。
- `.claude/` 与 `.cursor/`：两个平台的 skill、命令、子代理与 hooks 配置（Codex 同理是 `.codex/`）。

`sunshine platforms` 可查看当前项目已配置的平台：

```text
已配置的平台：
  Claude Code (claude-code) — .claude
  Cursor (cursor) — .cursor
```

### 手动干预速查

| 场景 | 命令 | 说明 |
| --- | --- | --- |
| 看当前任务 / 全部活动任务 | `sunshine task current` / `sunshine task list` | 状态来源就是 `.sunshine/.current-task` 与任务目录 |
| 看会话运行时信息 | `sunshine task context` | 开发者、Git 状态、当前任务、活动任务、日志文件 |
| 看已归档任务 | `sunshine task list-archive` | 按 `<年-月>` 分组 |
| 切工作流 | `sunshine workflow --list` / `sunshine workflow --template tdd` | 已改动时加 `--force` 覆盖或 `--create-new` 写 `.new` |
| 看模板更新计划 | `sunshine update --dry-run` | 三态：自动更新 / 你已修改 / 用户数据不动 |
| 从市场装规范 | `sunshine init --registry <来源> --template <模板 id> -y` | 来源支持 `gh:owner/repo`、git URL、HTTP(S) URL、`git@`、本地路径；交互模式不传 `--template` 时列出市场模板现选 |
| init 全部操作 | `sunshine init --help` | 平台、状态栏、模板选单、项目类型、三种模式，详见本附录“init 参考” |
| 回顾会话 | `sunshine mem search "关键词"` / `sunshine mem list` | 只读；`--global` 查全部项目，`--json` 输出 JSON |
| 看 channel | `sunshine channel list` / `sunshine channel messages <名字>` | 事件时间线；`wait` 等某个 worker 的 `done` 事件 |
| 升级 CLI 自身 | `sunshine upgrade --dry-run` / `sunshine upgrade` | 与 `update`（更新项目模板文件）是两回事 |
| 看已配置平台 | `sunshine platforms` | 按哈希清单判断，避免误判原生平台目录 |
| 临时移除全部 Sunshine 表面（可逆） | `sunshine ablate` / `sunshine restore` | 切除内容存入项目外的恢复事务，`restore` 精确恢复 |
| 删除项目内全部 Sunshine 文件 | `sunshine uninstall` | 只删哈希清单列出的托管内容，再移除整个 `.sunshine/` |
