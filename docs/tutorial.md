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
curl -fsSL https://raw.githubusercontent.com/BondLam/Sunshine/main/install.sh | sh
```

Windows 请到 [Releases](https://github.com/BondLam/Sunshine/releases) 页下载对应 zip，解压后把 `sunshine.exe` 放进 PATH；已有 Go 的机器也可以 `go install github.com/BondLam/Sunshine/cmd/sunshine@latest`。

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
下载地址：https://github.com/BondLam/Sunshine/releases/download/v0.2.1/sunshine_0.2.1_linux_amd64.tar.gz
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

> **Claude Code：** 先问一个决定怎么做的问题：商品数据放哪——先写死在代码里，还是读文件、接数据库？
>
> **你：** 先写死在代码里，三件商品够演示就行。
>
> **Claude Code：** 好，剩下的也是一次一个问：列表显示哪些字段、价格用什么单位——问完再汇总。明白了，规划是这样：建任务「商品列表」，PRD 里写首页展示商品列表（名称、价格、库存），三件商品先写死在代码里；验收就是服务跑起来，`curl /api/products` 能看到三件商品。确认后我就建任务、写 PRD、开始实现，没问题吗？
>
> **你：** 没问题，开始吧。
>
> **Claude Code：** 任务建好了，上下文配好了，任务启动了。我现在开始实现：写个小服务，商品列表走 `/api/products`，首页也一样能点着看，跑给你看。

跑给你看的结果是真实的：

```sh
curl -s localhost:18080/api/products
# [{"id":1,"name":"无线鼠标","price":89,"stock":120},{"id":2,"name":"机械键盘","price":299,"stock":45},{"id":3,"name":"27寸显示器","price":1299,"stock":20}]
```

浏览器打开首页，三件商品也都在：无线鼠标 89 元、机械键盘 299 元、27寸显示器 1299 元。

> **Claude Code：** 做完了，任务归档了：

```text
[2026-11] - 1 个任务
  - 11-13-商品列表/
```

接着做第二个需求：

> **你：** 接着做第二个需求：加入购物车。
>
> **Claude Code：** 先问一个问题：购物车放哪——先放内存里，还是写文件、接数据库？
>
> **你：** 先放内存，演示够用就行。
>
> **Claude Code：** 好，再问加购后返回什么、总价怎么算，问完汇总。明白了：建任务「加入购物车」，点“加入购物车”把商品放进购物车，`GET /api/cart` 看件数和总价；验收就是加两件商品，总价 388。确认后我就建任务、开始实现，没问题吗？
>
> **你：** 没问题，开始吧。
>
> **Claude Code：** 任务建好了，上下文配好了，任务启动了。我现在开始实现：加 `POST /api/cart` 加购、`GET /api/cart` 看购物车，列表页每件商品后面加个“加入购物车”按钮，跑给你看。

跑给你看的结果是真实的：先加一件无线鼠标、再加一件机械键盘，购物车里是这两件，总价 388：

```sh
curl -s localhost:18080/api/cart
# {"items":[{"id":1,"name":"无线鼠标","price":89,"qty":1},{"id":2,"name":"机械键盘","price":299,"qty":1}],"total":388}
```

> **Claude Code：** 做完了，任务归档了：

```text
[2026-11] - 2 个任务
  - 11-13-加入购物车/
  - 11-13-商品列表/
```

两个需求走完，项目里就有了：能跑的商城服务、两条归档的任务记录、按默认模板生成的规范。

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

> **Claude Code：** 三条约定我都看到了：接口统一返回 `{ code, data }`、价格统一用“分”存库、端口固定 3000。确认一下分法：前两条进 `backend.md`，端口那条呢——放 `system.md` 还是 `backend.md`？
>
> **你：** 前两条进 `backend.md`；端口是全组约定，放 `system.md`。
>
> **Claude Code：** 好，规划是这样：建任务「把现有约定落进规范」，PRD 里写把 README 里三条靠人记的约定写进 spec；验收就是三条约定都写进 spec 了。确认后我就建任务、写 PRD、把约定写进去，没问题吗？
>
> **你：** 没问题，开始吧。
>
> **Claude Code：** 任务建好了，约定写进去了，下面是真实写进去的内容：

```markdown
## 商品与订单接口（团队已有约定）

- 接口统一返回 `{ code, data }`，code 是大写下划线错误码。
- 价格统一用“分”存库，展示时除以 100。
```

```markdown
## 团队约定

- 服务端口固定 3000，改端口要全组同步。
```

> **Claude Code：** 上面两段就是写进去的，任务照常走完、归档了：

```text
[2026-11] - 1 个任务
  - 11-13-把现有约定落进规范/
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
> **Claude Code：** 好，剩下的也是一次一个问：地址是只一个文本框还是分省市区、四层按什么顺序改——问完再汇总。明白了，规划是这样：建任务「下单必填收货地址」，PRD 里写四层一起改——结算页加地址输入框、POST /orders 缺地址返回 400（ADDRESS_REQUIRED）、订单记录多 address 字段；上下文清单里放接口契约（backend.md），实现时 agent 自己读代码文件；改动顺序先 API 再 UI，走 channel 说清楚。确认后我就建任务、配上下文、启动，没问题吗？
>
> **你：** 没问题，开始吧。
>
> **Claude Code：** 任务建好了，上下文配好了。四层按先 API 再 UI 的顺序改，顺序我发到 channel 里说清楚，两条消息：先改 API——POST /orders 缺 address 返回 400，错误码 ADDRESS_REQUIRED；API 改完再改 UI——结算页加收货地址输入框，缺了不让提交。
>
> **Claude Code：** channel 和任务绑在一起，消息里能看到任务路径；这两条是带序号的协调消息，不是随手一句话。没有 worker 回复时等一会儿就超时，这是正常的——本节的协调只做到发消息这一步，拉起真实 worker 的前提是本机有已登录的 claude / codex（第 10 节讲过）。收尾时工作流切回 native，任务照常归档。

跑给你看的结果是真实的：缺地址下单返回 400，带地址下单成功，订单里有地址：

```sh
curl -s -X POST localhost:3000/api/orders -H 'Content-Type: application/json' -d '{"items":[{"id":1,"qty":1}]}' -w "\nHTTP %{http_code}\n"
# {"code":"ADDRESS_REQUIRED","data":null}
# HTTP 400

curl -s -X POST localhost:3000/api/orders -H 'Content-Type: application/json' -d '{"items":[{"id":1,"qty":1}],"address":"北京市朝阳区建国路 1 号"}' -w "\nHTTP %{http_code}\n"
# {"code":"OK","data":{"id":1,"items":[{"id":1,"qty":1}],"address":"北京市朝阳区建国路 1 号","status":"待付款"}}
# HTTP 200
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
> **Claude Code：** 明白了，规划是这样：先切到 TDD 模板，建任务「重构价格计算」；先写特征测试把这三档钉住（全绿是基线），再把折扣规则抽成常量与两个小函数，`Price` 只做组合，改完同一套测试再跑一次还是全绿。确认后我就开始，没问题吗？
>
> **你：** 没问题，开始吧。
>
> **Claude Code：** 工作流切到 TDD 模板了，任务建好了，启动了。先写特征测试把三档钉住：

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

跑给你看，全绿（`ok shop 0.003s`）——测试全绿是基线，这一步没改任何业务代码，只证明了“现在的行为就是这三行用例”。接下来才重构：折扣规则抽成常量与两个小函数，`Price` 只做组合：

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

重构后同一套测试再跑一次，还是全绿（`ok shop 0.003s`）——行为没变，改动才算成立。TDD 工作流里的红灯是给新行为的，纯重构的顺序是反过来的：**先写特征测试钉住行为（绿灯），再重构（还是绿灯）**。收尾时把新约定写进 spec，以后改规则先改用例表：

```markdown
## 价格计算

- 折扣规则集中在 calc.go 的常量与 applyBulk / applyVIP，Price 只做组合；改规则先改 calc_test.go 的用例表。
```

任务照常归档了，工作流切回 native：

```text
[2026-11] - 1 个任务
  - 11-13-重构价格计算/
```

### 12.5 修复反复出现的 bug：先找上次的会话

同类问题多次出现时，最贵的是每次都从零查起。这一节的 bug 是真实发生过的：商城项目派 codex worker 连不上，报 401——11 月 9 日出现过一次，11 月 10 日凌晨又出现一次，今天再跑还是 401。

在 Claude Code 里提需求（Codex、Cursor 同理）：

示例提示词（示例，不是真实会话记录）：

> 我要修复一个反复出现的 bug：商城项目派 codex worker 连不上，报 401——11 月 9 日出现过一次，11 月 10 日凌晨又出现一次，今天再跑还是 401。请按 sunshine 工作流走：先用 mem 找上次相关的会话，再向我提问，确认这次是不是同一类问题、复现条件是什么；确认后，帮我创建任务、写好 PRD，修完把教训写进 spec。

下面是接下来的会话示意（对话是示例；命令与输出是真实实跑值）：

> **Claude Code：** 我先用 mem 找了上次相关的会话，找到了 11 月 10 日 05:05 的一次派发，发给 worker 的消息里就写了 401。先确认一个问题：这次是不是同一类问题——商城项目派 codex worker，连不上报 401？
>
> **你：** 是同一类问题，今天再跑还是 401。
>
> **Claude Code：** 好，再问一个：复现条件是什么，在本机跑还是线上？
>
> **你：** 就在本机复现，codex 没登录。
>
> **Claude Code：** 明白了，规划是这样：先在本机复现一次当时的派发，确认还是 401；再建任务，描述里写清会话编号，修完把教训写进 spec。确认后我就开始，没问题吗？
>
> **你：** 没问题，开始吧。
>
> **Claude Code：** 我先在本机复现一次当时的派发：建了个 channel，给 worker 发了那句话，拉起 worker——worker 拉起来了，但随即在日志里报了错（本机 codex 没登录，第 10 节讲过的前提）：

```text
failed to connect to websocket: HTTP error: 401 Unauthorized, url: wss://api.openai.com/v1/responses
```

> **Claude Code：** worker 没回任何话，但会话已经落盘了。mem 找得到这次派发：11 月 10 日 05:05 的那次，会话编号 01a1c3d8；搜 401 命中 1 次，就是发给 worker 的那句话——“线上 codex worker 又连不上了，之前报过 401，帮我查一下这次的原因”。

一个实测得出的边界要记住：**`mem` 搜的是对话轮次（用户/助手）里的文字，不搜错误事件**。11 月 10 日凌晨那个会话（`01a1c376`，`/tmp/h-test/chanproj`）的 worker 同样报了 401，但它的 401 只在错误事件里，对话里没有，所以搜 `401` 搜不到它——要找的是当时发给 worker 的那句话。这次能搜到，是因为发给 worker 的消息里就写了 401。

找到上次的会话、确认是同一类问题之后，再建任务——描述里写清会话编号（01a1c3d8），agent 不用重新问一遍“上次是怎么回事”。任务做完，教训要写进 spec，否则下次还是一样——这是“反复出现”的真正解法：

```markdown
## 多 Agent 派发

- 派发 codex worker 前先确认本机 codex 已登录；spawn 失败时先看 worker.log（401 即未登录），不要反复重派。
```

任务照常归档了：

```text
[2026-11] - 1 个任务
  - 11-13-修复-codex-worker-401/
```

下次再报 401，agent 会先读 spec 查登录状态，而不是重新派一个 worker 去撞同一堵墙。

### 12.6 产品设计先行：需求 → 设计文档 → 拆解 → 开发

这个场景用跑了两年的 web 商城（12.2 那个）：给它加优惠券功能。设计人员和开发人员走同一条工作流，但进场、退场的时间不一样——下面每段开头都标了谁在场。

****

在 Claude Code 里提需求（Codex、Cursor 同理）：

示例提示词（示例，不是真实会话记录）：

> 我要给商城加优惠券功能。这次先做产品设计——请按 sunshine 工作流走：先向我提问，把不清楚的规则和原型细节问清楚；确认后，帮我创建任务、写好 PRD，再把需求列表（按需求分条，每条对应系统原型里一个页面）、概要设计和一份带 mock 假数据的系统原型落到 doc/ 里；评审通过后，按需求拆成子任务再动手实现。

下面是接下来的会话示意（对话是示例；命令与输出是真实实跑值）：

> **Claude Code：** 先问一个决定后面怎么拆的问题：优惠券是下单用券和券列表两个需求，还是一个？
>
> **你：** 两个：需求一是下单用优惠券，需求二是优惠券列表。

后面也是这样一次一个问：满减规则、券码不对怎么办、过期和已用怎么算——问完规则，再问原型细节（一个需求一页、券列表页显示什么、券码不对返回什么、mock 数据放几条），每次都给选项，你答一个它记一个，答案直接写进需求列表和概要设计。问完再汇总：

> **Claude Code：** 明白了，规划是这样：建任务，PRD 里写两个需求；需求列表按需求分条、每条写清对应系统原型里哪一页，概要设计和系统原型（`doc/prototype/` 里一页一个文件 + `index.html` 导航入口）落到 `doc/` 里；评审通过后按需求拆成两个子任务再动手。确认后我就建任务、写 PRD、落设计文档，没问题吗？
>
> **你：** 没问题，开始吧。

问完后，设计包落盘在项目 `doc/`，不在 `.sunshine`——设计人员先把所有产出物都做完，评审通过后开发才进场：

```text
doc/design/需求列表.md
doc/design/概要设计.md
doc/prototype/index.html
doc/prototype/结算页.html
doc/prototype/优惠券页.html
```

需求列表是按需求分条的（真实文件内容，节选）：需求一“下单用优惠券”对应系统原型结算页——结算页输入券码，满减规则满 100 元减 10 元，券须未使用且未过期，否则提示不可用；需求二“优惠券列表”对应系统原型优惠券页——列表显示券名、门槛、面额、状态、有效期。概要设计的接口行写的是（真实内容）：

```text
| POST /api/orders | body：items + address + couponCode | 订单（含实付 total） | 券不存在/已用/过期/不满门槛返回 400 + 错误码（COUPON_INVALID / COUPON_USED / COUPON_EXPIRED / COUPON_THRESHOLD）；MAN100JIAN10 满 100 元减 10 元（满 10000 分减 1000 分） |
| GET /api/coupons | 无 | 全部券（券名、门槛、面额、状态、有效期） | 列表接口，不分页 |
```

系统原型是 `doc/prototype/` 里的一组页面 + `index.html` 导航入口：顶部是“结算 | 优惠券”的导航，结算页是券码输入框、门槛提示（满 100 元减 10 元）、提交按钮，优惠券页是一张表（券名、门槛、面额、状态、有效期），表里是三条 mock 假数据（满100减10 未使用、满200减30 已使用、新人5元券 已过期），演示时直接点着看。一个原型对应多个需求：需求一和需求二都在这一组页面里，各占一页；拆解也跟着需求走，子任务按需求拆。

设计人员做设计包的顺序也是一步步来的：先把需求问清楚、写成需求列表，再写概要设计（结构、接口、验收），再派发 `sunshine-prototype` 子代理按需求列表一页一页做出原型（带 mock 假数据；派发提示里只给需求列表和概要设计的路径，子代理做完只汇报页数、mock 记录数和文件路径，主会话不直接写原型），最后拼成一份完整的系统原型——原型是设计阶段的终点，不是起点。做原型的时候发现需求有缺口，需求列表跟着改（走变更），但大方向是需求在先、原型在后。

然后是需求评审——设计人员退场前最后一件事：Claude Code 把整包东西列出来请你确认——PRD、设计文档清单、拆解方案；确认后才往下走。

> **Claude Code：** 评审一下：PRD、需求列表、概要设计、系统原型都齐了，拆成两个子任务——需求一是结算页 + POST 接口，需求二是优惠券页 + GET 接口。确认后我就建父任务、一次性拆子任务，没问题吗？
>
> **你：** 没问题，拆吧。

****

开发人员进场时（尤其设计和开发不是同一个人），先在 agent 界面里打开项目，再说一句进场提示词：

示例提示词（示例，不是真实会话记录）：

> 设计评审通过了，子任务已经拆好。现在按 sunshine 工作流开始开发：先把父任务的子任务清单和第一个子任务的上下文列出来给我确认，确认后从第一个子任务开始，一个一个做；每个子任务先启动，做完检查、回写规范、提交、归档，再启动下一个；全部子任务归档后，父任务做最终集成复核再归档。

确认后，开发就按下面“一轮一轮怎么做”的五轮走；中途关掉重开，跑 `/sunshine:continue`，它按 `task.json` 的状态分流续接。

评审通过后拆解只做一次——拆解跟着需求走：一个原型对应多个需求，子任务按需求拆，拆的是整个范围，子任务清单一次定好；拆完不自动开发，两个子任务建出来都是 `planning`。父任务记好子任务映射，开发人员进场时任务已经拆好、上下文已经配好（每个子任务的上下文指向系统原型里自己的那一页）。

一个需求默认是一个子任务（子任务包揽自己那一页 + 它的接口，验收标准在一起，好验收）。但如果一个需求的前后端都很大、或者前后端是不同的人做、或者后端接口要给好几个页面共用，就再拆成前端一个子任务、后端一个子任务——机制上就是子任务再挂子任务（`task create --parent`），父任务的 `children` 里记着两个，前端和后端各是 planning。两个子任务之间的交接物是概要设计里的接口约定：后端按约定实现接口，前端按约定 + 原型里的 mock 数据做页面，前端不用等后端做完；父任务的最终集成复核再把前后端合起来一次验过。

下面是真实跑出来的任务列表（拆解完就是嵌套的）：

```sh
sunshine task list
# 全部活动任务：
#
# - 11-13-商城优惠券/ (planning) [0/2 已完成] [developer]
# - 11-13-下单用优惠券/ (planning) [developer]
# - 11-13-优惠券列表/ (planning) [developer]
#
# 总计：3 个任务
```

**拆解的产出物：磁盘上多了什么**

拆解不是只在对话里说一声，磁盘上多了这些东西（真实目录树，任务目录在归档前后长一个样，只是归档后搬进 `archive/2026-11/`）：

```text
.sunshine/tasks/11-13-商城优惠券/
task.json # 父任务：children 记着两个子任务
prd.md # 父任务的 PRD
implement.jsonl # 空的，父任务不实现
check.jsonl # 空的
.sunshine/tasks/11-13-下单用优惠券/
task.json # 子任务：parent 指回父任务，status 是 planning
prd.md # 子任务的 PRD（骨架，agent 在 1.1 里填实）
implement.jsonl # 实现时读什么：系统原型结算页 + 概要设计
check.jsonl # 检查时对照什么：前端规范 + 后端接口契约
.sunshine/tasks/11-13-优惠券列表/
...（同样四份）
```

需求一的上下文清单（真实内容）——开发人员进场读的就是这两份：

```json
implement.jsonl
{"file":"doc/prototype/结算页.html","reason":"结算页原型：券码输入框"}
{"file":"doc/design/概要设计.md","reason":"接口与流程"}

check.jsonl
{"file":".sunshine/spec/frontend.md","reason":"结算页规范"}
{"file":".sunshine/spec/backend.md","reason":"优惠券接口规范"}
```

原型按页分文件，各子任务直接读自己那页的文件，不用再算行范围；`lines` 只留给其它长单文件（如长规范），用法见 1.4。

子任务的 `prd.md` 是骨架，agent 在子任务自己的 1.1 里填实；复杂任务在 `start` 前还要补 `design.md`（技术设计）和 `implement.md`（执行计划）——这是 `prd.md` 模板里自己写的话。演示里需求写在 `doc/` 设计包里，子任务的上下文指过去就行。

**开发进场：一轮一轮怎么做**

先启动需求一。Claude Code 在底下跑的（你不用敲），只贴关键输出：

```sh
sunshine task start 11-13-下单用优惠券
# ✓ 状态：planning → in_progress（降级）
```

`start` 成功后状态就写进 `task.json`，指针写进 `.sunshine/.current-task`——会话关了重开，状态还在文件里，不在对话里（后面有接续的例子）。

下面是开发时的会话示意（对话是示例；命令与输出是真实实跑值）：

> **Claude Code：** 需求一开始了：主会话派 `sunshine-implement` 按系统原型结算页给 `checkout.html` 加券码输入框，给 `orders.js` 的 POST 加四道校验，改完再派 `sunshine-check` 按 `check.jsonl` 里的规范验收。
>
> **Claude Code：** implement 做完了：结算页多了券码输入框（`name="couponCode"`）；POST 按“不存在→已用→过期→不满门槛”顺序校验，券码不对返回 400，满 100 元减 10 元，1 件机械键盘实付 28900。check 也过了：400 分支四条、券码输入框一处、三条券都在，四档 curl 全对。spec 里我补了优惠券接口和结算页两条约定。提交一次？（一个提交：结算页 + 券接口 + spec 约定）
>
> **你：** 提交吧，归档这个子任务。
>
> **Claude Code：** 好，提交完 `task finish` 清指针、`task archive` 归档。父任务现在 1/2 已完成。

关键输出（真实跑出来的）：grep 三处是 `4`、`1`、`3`（400 分支四条、券码输入框一处、三条券）；curl 四档：券码 NOPE 是 `{"code":"COUPON_INVALID","data":null}`、HTTP 400，1 件机械键盘是 200、订单 id 是 1、实付 28900，1 件无线鼠标是 `{"code":"COUPON_THRESHOLD","data":null}`、HTTP 400，2 件无线鼠标是 200、订单 id 是 2、实付 16800；提交是 `7634b3b 需求一：下单用优惠券，结算页 + 券接口`；归档是 `b67ebfc chore(task): archive 11-13-下单用优惠券`；归档后父任务显示 1/2 已完成。

一个子任务就是这样一轮一轮走的，每轮读什么、产出什么都在文件里，不靠对话记着：

1. **实现（2.1）**：读 `implement.jsonl` 里的文件（系统原型结算页、概要设计）+ 自己的 `prd.md`，派 `sunshine-implement` 改代码。产出：改好的 `checkout.html` 和 `orders.js`（`db.js` 里加了券表）。
2. **检查（2.2）**：读 `check.jsonl` 里的文件（前端规范、后端接口契约），派 `sunshine-check` 验收。产出：grep / curl 的验收结果。
3. **回写规范（3.3）**：`sunshine-update-spec` 把新约定写进 `spec/`（优惠券接口、结算页）。产出：更新后的 `frontend.md`、`backend.md`。
4. **提交（3.4）**：一个提交（代码 + spec 约定）。产出：提交 `7634b3b`。
5. **收尾**：`task finish` 清指针，`task archive` 把任务目录搬进 `archive/2026-11/`、状态改成 `completed`。产出：归档后的任务目录 + 父任务 1/2 已完成。

需求一归档后，需求二才启动——走同样的五轮（对话略，命令与输出是真实实跑值）：

> **Claude Code：** 需求二开始了：派 `sunshine-implement` 按系统原型优惠券页写 `public/优惠券页.html`，加 `GET /api/coupons` 接口，改完派 `sunshine-check` 验收。
>
> **Claude Code：** implement 做完了：优惠券页拉 GET 渲染表格；GET 返回全部三条券。check 也过了：fetch 在、表格在，GET 返回 3 条券。spec 里我补了券列表和 GET 接口两条约定。提交一次？
>
> **你：** 提交吧，归档这个子任务。

关键输出（真实跑出来的）：

```sh
curl -s localhost:3000/api/coupons
# {"code":"OK","data":[{"code":"MAN100JIAN10","name":"满100减10","threshold":10000,"amount":1000,"status":"未使用","expires":"2026-12-31"},{"code":"MAN200JIAN30","name":"满200减30","threshold":20000,"amount":3000,"status":"已使用","expires":"2026-12-31"},{"code":"NEW5","name":"新人5元券","threshold":0,"amount":500,"status":"已过期","expires":"2026-10-01"}]}
```

提交是 `51920ca 需求二：优惠券列表，优惠券页 + GET 接口`；归档是 `e15f11c chore(task): archive 11-13-优惠券列表`；归档后父任务显示 2/2 已完成。

最后父任务做最终集成复核——读概要设计里的验收标准、两个子任务的检查结果和接口约定，不重读整个原型，一次过一遍，再归档：

> **Claude Code：** 集成复核：验收标准都过了——结算页券码输入框在，券校验四档（无效券 400、不满门槛 400、满减两档 200）和 GET 列表三条券都过了；两个子任务的检查结果和接口约定也都对得上。归档父任务？
>
> **你：** 归档吧。

****

```sh
sunshine task list
# 全部活动任务：
#
# （没有活动任务）
#
# 总计：0 个任务

sunshine task list-archive
# 已归档任务：
#
# [2026-11] - 3 个任务
# - 11-13-下单用优惠券/
# - 11-13-优惠券列表/
# - 11-13-商城优惠券/
```

设计人员进场时只有需求，退场时留下一份完整的设计包；开发人员进场时任务已经按需求拆好、上下文已经配好，退场时 0 个活动任务、3 个归档。

**关闭重开后怎么接上**

Claude Code 关了重开，会话里的对话没了，但状态都在文件里：`task.json` 的 `status`、`.sunshine/.current-task` 里的指针、`implement.jsonl` / `check.jsonl`、`prd.md`、`spec/`。新会话不用回忆，直接跑 `/sunshine:continue`——它先跑 `sunshine task context` 看当前任务，再按 `status` 分流到对应的步骤（2.1 实现、2.2 检查、3.3 回写规范、3.4 提交）。

下面是真实跑出来的接续（需求一做到一半时取的，只读文件）：

```sh
sunshine task current
#.sunshine/tasks/11-13-下单用优惠券

sunshine task context
# ## 当前任务
# 路径：.sunshine/tasks/11-13-下单用优惠券
# 状态：in_progress
# ……
```

`.sunshine/.current-task` 里就一行，就是任务路径：

```text
.sunshine/tasks/11-13-下单用优惠券
```

接续时的会话示意（对话是示例；命令与输出是真实实跑值）：

> **你：**（第二天，重新打开 Claude Code）继续做商城优惠券。
>
> **Claude Code：** 先看一下当前任务状态：`task context` 说当前任务是需求一，状态 `in_progress`，按上下文清单读系统原型结算页和概要设计，接着从 2.1 做，没问题吗？
>
> **你：** 没问题，继续。

两个注意点，都是真实踩过的：

- `task start` 有守卫：`check.jsonl` 是空的会直接报错（`错误：check.jsonl 没有已整理条目`），先 `add-context` 配好再 `start`，或者加 `--allow-empty-context`。
- 指针在 `.sunshine/.current-task` 里，`task finish` 清掉它，`task archive` 把任务目录搬进归档。归档完的任务不在活动列表里，不会再被续接上——变更走新任务，就是这个道理。

**需求变更：满减门槛从 100 元改成 200 元——设计人员返场**

项目做完归档后，业务方说满减门槛从 100 元改成 200 元。按工作流 1.5 的需求变更规则走：已经归档的三个任务不 reopen，变更走一个新任务；但设计包是设计人员的交付物，改设计包要设计人员先返场——返场不是一句话，是实打实改三处地方、评审只审改动的部分，交接有凭据。

****

> **你：** 需求变了：满减门槛从 100 元改成 200 元。按 sunshine 工作流走：设计人员先返场改设计包，评审通过后再动手。
>
> **Claude Code：** 波及面是需求一一个地方：需求二已归档且不受影响（列表页不拦门槛），不 reopen；需求一也已归档，不 reopen。规划是这样：设计人员先返场，改三处地方——需求列表里需求一那条、概要设计接口行、系统原型结算页里的门槛提示；改完评审只审这三处，评审通过后设计人员退场；再建新任务「满减门槛改成 200」，PRD 里记一笔 `[变更 2026-11-13]`，`db.js` 的门槛改成 20000 分，spec 同步改，curl 验 178 元和 299 元两档。确认后我就开始，没问题吗？
>
> **你：** 没问题，开始吧。

设计人员返场改的是这三处（真实文件内容，改后都是 200 元）：

```text
doc/design/需求列表.md:17
| R-001 | 下单用优惠券 | 结算页 | 结算页输入券码，满减规则：满 200 元减 10 元，订单总额达到 20000 分才减面额；券须未使用且未过期，否则提示不可用 | P0 | 券码 MAN100JIAN10：满 200 元减 10 元；已用/过期/不满门槛下单失败并提示 |

doc/design/概要设计.md:34
| POST /api/orders | body：items + address + couponCode | 订单（含实付 total） | 券不存在/已用/过期/不满门槛返回 400 + 错误码（COUPON_INVALID / COUPON_USED / COUPON_EXPIRED / COUPON_THRESHOLD）；MAN100JIAN10 满 200 元减 10 元（满 20000 分减 1000 分） |

doc/prototype/结算页.html:27
<p class="hint">满 200 元减 10 元</p>
```

原型那处只是一行门槛提示，小改在主会话里 grep 定向改，不重新派发 `sunshine-prototype`；只有结构性大改才重新派发。

改完是需求评审——这次只审变更的部分，不重审整个设计包：

> **Claude Code：** 评审一下，只审改动的三处：需求列表、概要设计、系统原型里的门槛都从 100 元改成 200 元，其他地方一个字没动；需求二不受影响。确认后设计人员退场，我建新任务、配上下文，开发人员进场，没问题吗？
>
> **你：** 没问题。

**评审时要说什么：波及面和任务列表怎么定**

上面那个例子是项目做完归档后的变更，评审只审改动的三处。开发中途的变更也一样，但评审时要多看一样东西——任务列表怎么调。你在评审时说一句：

示例提示词（示例，不是真实会话记录）：

> 评审这次变更：先列波及面——变更碰到哪几个子任务、每个是已归档、进行中还是还没做；再列任务列表怎么调——哪个新建返工子任务、哪个直接改范围和上下文、哪个摘掉、哪个不动；最后列拆分方案。确认这三样我点头后才动手。

波及面是这样定的：对照两样东西——父任务 `task.json` 里的 `children` 清单（子任务映射）和 `doc/` 设计包（需求列表、概要设计、原型页）。变更碰到哪个子任务的 PRD、设计、接口或原型页，那个子任务就被波及。定下来之后按四类处理：

- **已归档**：不 reopen。新建一个返工子任务挂父任务下，走完整流程，描述里写清原任务与变更原因。
- **进行中**：先停下手头的活，回炉完再继续，状态不用改。
- **还没做（planning）**：直接改，范围用 `task set-scope` 改，上下文用 `task add-context` 加。
- **没被波及**：不停，继续做。
- **作废**：用 `task remove-subtask` 从父任务的 `children` 里摘掉。

拆分还是按需求：返工子任务一个需求一个，前后端都大再拆前端、后端两个。打个比方：需求一已归档、需求二还没做，变更两个都影响——需求一新建返工子任务，需求二直接改内容；如果需求二不用做了，就把它摘掉。调完的清单在评审时给你确认一遍，通过开发才继续。

**交接：开发人员拿到什么**

评审通过，设计人员退场。开发人员进场时拿到的不是一句话，是三样东西——

1. **改好的设计包**：上面三处，`doc/` 里的文件就是合同，开发照着改。
2. **评审确认**：只审变更部分的确认，确认完才往下走。
3. **新任务的上下文清单**：新任务「满减门槛改成 200」的 `implement.jsonl` 和 `check.jsonl`，开发人员进场只读这两份清单里的文件，不用重读整个设计包：

```json
implement.jsonl
{"file":"doc/design/需求列表.md","reason":"变更后的需求一"}
{"file":"doc/design/概要设计.md","reason":"变更后的接口行"}

check.jsonl
{"file":".sunshine/spec/backend.md","reason":"优惠券接口规范"}
```

PRD 里还记了一笔（真实内容）：`[变更 2026-11-13] MAN100JIAN10 的满减门槛由 100 元改成 200 元（threshold 10000 分→20000 分）；设计已改三处：需求列表需求一、概要设计接口行、结算页门槛提示；改代码后实测：2 件无线鼠标不满门槛、1 件机械键盘仍可用`

****

开发人员进场，按新任务的上下文动手：`db.js` 的门槛改成 20000 分，spec 同步改，curl 验两档。

关键输出（真实跑出来的）：2 件无线鼠标返回 `{"code":"COUPON_THRESHOLD","data":null}`（改前是 200、实付 16800），1 件机械键盘返回 200、实付 28900；提交是 `06a179c 变更：满减门槛改成 200 元（100→200）`；归档后一共 4 个任务。旧的三个任务原样留在归档里，变更单独一个任务、有完整的记录——下次再有人问“门槛为什么是 200 元”，`prd.md` 里的 `[变更 2026-11-13]` 就是答案。

**全流程闭环一览**

整个场景走下来，是一条闭环，每一步的进出场和交接物都在上面写清了：

1. **需求提出**（你）：在 Claude Code 里提需求，说清要做什么。
2. **需求澄清**（设计人员进场）：agent 先提问，一次只问一个，先问堵住后面工作的（比如需求是几个、怎么拆），再问细节；问之前先对照八个方面看哪个还没问到——谁用、做什么、不做什么、数据、性能、安全、兼容、怎么验收。
3. **设计包**（设计人员）：先写需求列表，再写概要设计，再派发 `sunshine-prototype` 子代理按需求列表一页一页做出系统原型（带 mock 假数据；主会话只收汇报，不直接写原型）——原型是设计阶段的终点。交接物：`doc/` 里的三份设计文档（本例的三份，不是硬性清单——设计产出由设计者定，`doc/` 里有什么、评审就审什么、拆解就依据什么；没有设计文档就是纯代码任务。）。
4. **需求评审**（你确认）：审整包（设计包 + 拆解方案）；确认后设计人员退场。交接物：评审通过的设计包。
5. **拆解**（agent）：一次性拆完整个范围，子任务按需求拆（一个需求默认一个子任务，前后端都大就再拆前端/后端两个子任务，交接物是概要设计里的接口约定）；拆完不自动开发。交接物：任务目录（task.json 的 parent/children、prd.md、implement.jsonl、check.jsonl）。

后端另外算：这个商城的后端已经有了（Express，12.2 那个），不用选型；优惠券的后端改动（券校验、券列表接口）直接写进需求一、需求二的 prd。量大（要选语言、建表、接缓存）才先拆一个“后端基础：技术选型与脚手架”子任务排在最前面，选型项（语言、框架、数据库、缓存）以 `[待定]` 标在它的 prd 里，它的 1.1 先问答定选型再动手，归档后需求子任务再启动。

6. **开发**（开发人员进场）：逐个激活子任务，每个子任务五轮（实现 → 检查 → 回写规范 → 提交 → 归档），每轮读自己的上下文清单。交接物：改好的代码 + 更新后的 spec + 提交。
7. **集成复核与归档**（父任务）：子任务全归档后，父任务读验收标准、两个子任务的检查结果和接口约定一次复核（不重读整个原型），再归档。交接物：归档的任务目录。
8. **需求变更**（设计人员返场）：改设计包 → 只审变更部分 → 交接（改好的设计包 + 评审确认 + 新任务的上下文清单 + PRD 里的 `[变更]` 记录）→ 开发人员进场走新任务。已归档的任务不 reopen。
9. **关闭重开后接上**：状态全在文件里（task.json 的 status、`.current-task` 指针、jsonl、prd.md、spec），新会话跑 `/sunshine:continue`，按 status 分流到对应的步骤。

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
