# Sunshine 教程：在 agent 界面里工作，终端只跑一次 init

主线一句话：**终端只跑一次 `sunshine init`，然后进 agent 界面（Claude Code 为主）用自然语言提需求，agent 按 `.sunshine/workflow.md` 干活，人检查 diff、验收、提交。**

约定：

- 与 agent 的对话示例都标明是“示例提示词”，不是真实会话记录；展示的命令输出是真实跑出来的摘录，`……` 表示省略的行，不是改写。
- 演示项目不变：**退款原因必填校验**——给退款流程加一个必填的原因字段。例子很小，流程是真的。
- 任务目录名里的 `11-10` 是实跑当天的月-日（2026-11-10），你跑的时候会是当天的日期。
- 版本号随你安装的 CLI 版本而变；本教程用当前 Release **v0.2.1** 验证（`sunshine version` 打印 `Sunshine 版本 0.2.1`）。第 11 节“有新版”的输出来自本地构建的 0.1.0 二进制，仅版本号不同，形态一致，那里会注明。
- 演示用的目录都在 `/tmp/rewrite-zh/demo/` 下面（`demo`、`demo2`、`registry-src`、`channels`、`home`），删掉即可，不影响任何真实项目。

终端命令在本教程里只出现在两处：第 4 节“底下发生了什么”（agent 实际调的命令 + 真实输出）和第 12 节“手动干预速查”。其余章节讲的是人在 agent 界面里做什么、agent 按什么干活。

## 1. 终端只跑一次：init

先装好 CLI（安装方法也在 [README.md](../README.md)）：

```sh
curl -fsSL https://raw.githubusercontent.com/linrong1982/Sunshine/main/install.sh | sh
```

Windows 请到 [Releases](https://github.com/linrong1982/Sunshine/releases) 页下载对应 zip，解压后把 `sunshine.exe` 放进 PATH；已有 Go 的机器也可以 `go install github.com/linrong1982/Sunshine/cmd/sunshine@latest`。

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
已安装平台：
  - Claude Code
  - Cursor
下一步：
  1. 编辑 .sunshine/spec/，填入团队已有的技术规范与业务术语；
  2. 按 .sunshine/workflow.md 开始第一个任务。
```

`-u` 是开发者名字，`-y` 跳过全部提问用默认配置。默认装 Claude Code 与 Cursor；只想装 Codex 时用 `sunshine init --codex -y`，已初始化的项目里再跑 `init --codex -y` 是增量追加，不动已有文件。`sunshine platforms` 可查看当前项目已配置的平台：

```text
已配置的平台：
  Claude Code (claude-code) — .claude
  Cursor (cursor) — .cursor
```

init 生成了 115 个文件（不含 `.git`），其中给 agent 读的是这些：

- `.sunshine/workflow.md`：完整版开发工作流（714 行），agent 按它推进任务。
- `.sunshine/AGENTS.md`：开头即“本文件面向在本仓库工作的 AI 编程助手”。
- `.sunshine/spec/`：三份规范模板（`system.md` 总原则、`backend.md` 后端、`frontend.md` 前端）。
- `.sunshine/config.yaml`：项目配置，带中文注释（关键字段见 [README.md](../README.md#配置要点)）。
- `.claude/` 与 `.cursor/`：两个平台的 skill、命令、子代理与 hooks 配置（Codex 同理是 `.codex/`）。

装完这一步，终端的任务就结束了。`sunshine --help` 自己也是这么说的：

```text
先用 sunshine init 初始化当前项目，再让 AI 助手按
.sunshine/workflow.md 推进任务。
```

## 2. 进 agent 界面：用自然语言提需求

在 Claude Code 里打开这个项目（Codex、Cursor 同理：打开项目，进它的 agent 界面），直接用自然语言说你要什么。不用先学命令，不用先建任务——建任务是 agent 的活。

示例提示词（示例，不是真实会话记录）：

> 给退款流程加一个必填的原因字段：用户提交退款时原因不能为空，否则拒绝提交。原因做成下拉选择（质量问题 / 尺寸不符 / 其他），选“其他”时出现文本框；后端 `POST /refunds` 缺 `reason` 返回 400，错误码 `REASON_REQUIRED`。

agent 收到后会先读 `.sunshine/AGENTS.md`、`.sunshine/workflow.md` 和 `.sunshine/spec/`，再按工作流推进；有不清楚的地方会在界面里反问你，而不是猜。你只需要在对话里回答、确认、验收。

如果需求是在已有项目上改，同样一句话即可，例如：

示例提示词（示例，不是真实会话记录）：

> 退款页再加一个返回按钮，点一下回到订单列表。

## 3. agent 按 workflow.md 干活

agent 按 `.sunshine/workflow.md` 的三个阶段推进（下面是 workflow.md 里的实际阶段名与步骤号，对得上你项目里的文件）：

### 第 1 阶段：规划

- **1.1 需求探索**：写 `prd.md`（需求、约束与验收标准）。轻量任务可以只有 PRD；复杂任务还要 `design.md`（技术设计）与 `implement.md`（执行计划），并且在 `task start` 之前先请你复核。
- **1.2 研究**（可选）：需要调研时先查资料、看代码，再写设计；本演示任务简单，不走这一步。
- **1.3 配置上下文**：把实现与验收要读的规范、调研文档整理进 `implement.jsonl` / `check.jsonl`（每行一个 `{"file": "<路径>", "reason": "<原因>"}`），供 implement/check 子代理读。
- **1.4 激活任务**：`task start`，任务进入 `in_progress`。

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

- **2.1 实现**：主会话默认派发 `sunshine-implement` 子代理写代码（inline 模式下主会话直接改代码，不派发子代理）。
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

改动：
?? .claude/
?? .cursor/
?? .sunshine/
?? src/

## 最近提交
（无提交）

## 当前任务
路径：.sunshine/tasks/11-10-给退款原因加必填校验
来源：file:.sunshine/.current-task
名字：给退款原因加必填校验
状态：in_progress
创建：2026-11-10
描述：退款时原因字段必填

[!] 该任务有 prd.md，详情请读它

## 活动任务
- 11-10-给退款原因加必填校验/ (in_progress) @演示者
总计：1 个活动任务

## 我的任务（分配给我）
- [P1] 给退款原因加必填校验 (in_progress)

## 日志文件
未找到日志文件
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

当前日志文件：journal-1.md
当前行数：7
新增行数：21
追加后总计：28

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
  1. 编辑 .sunshine/spec/，填入团队已有的技术规范与业务术语；
  2. 按 .sunshine/workflow.md 开始第一个任务。
```

模板内容装进了 `.sunshine/spec/demo.md`，来源记进了 `config.yaml`：

```yaml
# 受哈希追踪的 spec 模板，本地改动走正常的 update 冲突流程。
registry:
  spec:
    source: /tmp/rewrite-zh/demo/registry-src/marketplace
    template: demo-spec
```

远程市场同理，把本地路径换成 `gh:owner/repo`、完整 git URL、HTTP(S) URL 或 `git@` 地址即可。

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
下载地址：https://github.com/linrong1982/Sunshine/releases/download/v0.2.1/sunshine_0.2.1_linux_amd64.tar.gz
备份：将把当前二进制备份为 /tmp/rewrite-zh/sunshine.bak
[试运行] 未做任何修改。
```

真正升级时去掉 `--dry-run`：查最新版 → 按当前系统与架构选资产 → 下载 → 按 `checksums.txt` 校验 sha256 → 备份当前二进制（同目录 `.bak`，权限保留）→ 替换 → 运行新版 `--version` 校验，失败回滚备份并报错（流程见 `sunshine upgrade --help`）。也可以指定版本或通道：`sunshine upgrade --tag 0.2.0`、`sunshine upgrade --tag beta`。

## 12. 收尾：手动干预速查

主线走完了：项目骨架与规范在 `.sunshine/` 里，任务从创建到归档走了一遍，工作流切过 TDD，模板市场装过规范并验证了持续刷新，会话能回顾，多 Agent 的 channel 能建会话、发消息、开主题，CLI 也能自己升级。

平时你在 agent 界面里工作，下面的命令只在手动干预时用——每一行都是本教程前面实跑过的命令（`uninstall` / `ablate` / `restore` 的一句话说明来自各自的 `--help`，未在本教程里真执行）：

| 场景 | 命令 | 说明 |
| --- | --- | --- |
| 看当前任务 / 全部活动任务 | `sunshine task current` / `sunshine task list` | 状态来源就是 `.sunshine/.current-task` 与任务目录 |
| 看会话运行时信息 | `sunshine task context` | 开发者、Git 状态、当前任务、活动任务、日志文件 |
| 看已归档任务 | `sunshine task list-archive` | 按 `<年-月>` 分组 |
| 切工作流 | `sunshine workflow --list` / `sunshine workflow --template tdd` | 已改动时加 `--force` 覆盖或 `--create-new` 写 `.new` |
| 看模板更新计划 | `sunshine update --dry-run` | 三态：自动更新 / 你已修改 / 用户数据不动 |
| 从市场装规范 | `sunshine init --registry <来源> --template <模板 id> -y` | 来源支持 `gh:owner/repo`、git URL、HTTP(S) URL、`git@`、本地路径 |
| 回顾会话 | `sunshine mem search "关键词"` / `sunshine mem list` | 只读；`--global` 查全部项目，`--json` 输出 JSON |
| 看 channel | `sunshine channel list` / `sunshine channel messages <名字>` | 事件时间线；`wait` 等某个 worker 的 `done` 事件 |
| 升级 CLI 自身 | `sunshine upgrade --dry-run` / `sunshine upgrade` | 与 `update`（更新项目模板文件）是两回事 |
| 看已配置平台 | `sunshine platforms` | 按哈希清单判断，避免误判原生平台目录 |
| 临时移除全部 Sunshine 表面（可逆） | `sunshine ablate` / `sunshine restore` | 切除内容存入项目外的恢复事务，`restore` 精确恢复 |
| 删除项目内全部 Sunshine 文件 | `sunshine uninstall` | 只删哈希清单列出的托管内容，再移除整个 `.sunshine/` |

下一步可以看的：

- [README.md](../README.md)：安装、快速上手、核心命令一览与配置要点。
- [PLAN.md](../PLAN.md)：完整规划与各阶段实施记录，包括本教程没展开的设计决策。
- [Releases](https://github.com/linrong1982/Sunshine/releases)：各版本下载与更新说明。
