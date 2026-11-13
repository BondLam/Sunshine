# Sunshine tutorial: init once, then work in the agent UI

The main flow in one line: **run `sunshine init` once in the terminal, then do everything else by talking to the agent in its UI (Claude Code first, Codex / Cursor the same idea) in plain language — the agent drives the work following `.sunshine/workflow.md` and calls `sunshine` commands under the hood; you review the diff, accept, and the work is committed.**

That is the whole point of this tutorial: the terminal is not where you spend your day. You type one setup command, then you talk. The `sunshine task …` / `sunshine channel …` commands still exist, but they are the agent's tools, not your daily interface — they appear in this tutorial only in [section 4](#4-what-happened-under-the-hood) (what the agent actually ran, with real outputs), in [section 12](#12-six-scenarios-from-day-one-to-daily-work) (one walkthrough per scenario), and in the [appendix](#appendix-command-reference) (command reference for when you step in manually).

The demo project is the same throughout: **refund reason required** — add a mandatory reason field to the refund flow. The example is small; the workflow is real.

Conventions:

- Dialogue with the agent is shown as **example prompts**, labeled as such. They are illustrations of what to say, not records of a real session.
- Every command output shown is a real excerpt from a run made for this tutorial (`……` marks omitted lines, not rewritten ones). The `11-10` prefix in task directory names is the month-day of the day the commands were run (today, 2026-11-10).
- This tutorial was verified with v0.2.1 (the current Release). The two places that show a different version say so explicitly.
- **The CLI itself is Chinese-only.** Commands and sample output below are kept in the original Chinese so you can copy them verbatim; the explanations around them are in English.
- The demo directories all live under `/tmp/rewrite-en/` (`demo`, `demo2`, `demo3`, `registry-src`, `channels`, `home`) — delete them and nothing in any real project is affected.

## 1. Run once in the terminal: init

Install Sunshine first (see [README.md](../README.md#installation)), then confirm it:

```sh
sunshine version
# Sunshine 版本 0.2.1

sunshine --help
# Sunshine 把规范、任务与项目记忆持久化进仓库，
# 让 AI 编程助手按团队的工程标准工作，而不是每次从零猜测。
#
# 先用 sunshine init 初始化当前项目，再让 AI 助手按
# .sunshine/workflow.md 推进任务。
# ……
```

(The help text says, in English: "Sunshine persists specs, tasks, and project memory into the repository, so the AI programming assistant works to the team's engineering standards instead of guessing from scratch each time. First run `sunshine init` to initialize the current project, then let the AI assistant drive tasks following `.sunshine/workflow.md`.")

Now initialize the demo project — the only terminal command you need before switching to the agent UI:

```sh
mkdir demo && cd demo
git init
sunshine init -u 演示者 -y
```

Real output:

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

(`-u` is the developer name; `-y` skips all prompts and uses the defaults. In English: "Configuring Claude Code… / Configuring Cursor… / Initialization complete, developer: 演示者 / Project type: 全栈 (default) / Installed platforms: Claude Code, Cursor / Next steps: 1. Open the project in the agent UI and state your first requirement in plain language; 2. The specs have been generated in .sunshine/spec/ from the default templates (generated: system.md, frontend.md, backend.md; add your team's specs if you have them); the workflow runs automatically following .sunshine/workflow.md and normally needs no changes.")

The demo directory is empty, so nothing is detected; with `-y` skipping the prompt it defaults to fullstack, which is why the output above says `项目类型：全栈（默认）` ("Project type: fullstack (default)"). By default `init` installs Claude Code and Cursor and generates the Chinese `.sunshine/` skeleton with both platforms' configs.

From here on, close the terminal and ask for your first requirement in the agent UI (section 2). All init operations — the interactive questions, the common options, the three modes, project-type detection, and what gets installed — are in the [appendix: command reference](#appendix-command-reference) at the end.

## 2. Open the agent UI: ask in plain language

Open the project in Claude Code (Codex and Cursor work the same way — the agent there reads the same `.sunshine/` files). The agent already knows the project conventions from `AGENTS.md`, `spec/`, and `workflow.md`; you just describe what you want.

> **Example prompt** (an illustration of what to say, not a real session transcript):
>
> Add a required reason field to the refund flow: when a user submits a refund, the reason must not be empty or the submission is rejected. The reason input is a dropdown — quality issue / wrong size / other — and choosing "other" reveals a text box. On the backend, `POST /refunds` requires `reason`; a missing reason returns 400 with error code `REASON_REQUIRED`. Please follow the Sunshine workflow: ask me first to clarify anything unclear about the product logic or technical details; once confirmed, create the task, write the PRD, then implement.

Note: the prompt must name the workflow explicitly — ask first, confirm, then create the task and write the PRD. A bare requirement on its own is not enough: in a real run the agent jumped straight to coding and skipped task creation and requirement confirmation.

The designer and the developer are not two workflows — they are two phases of one workflow: the designer does 1.1 (clarify requirements), the design deliverables, 1.5 (review), and 1.2 (breakdown); the developer does phase 2 (implement and check) and phase 3 (write back and commit). They share one workflow.md, one task system, and one PRD, and the handoff runs through the requirements list and the task structure. So the prompt does not need to name a role — naming the workflow and the mode is enough, and the role follows the phase: while the task is in the design phase, the agent is the designer; once it moves into development, the agent is the developer. If the mode is unclear, the agent asks first in 1.1. The only thing to state explicitly is the scope boundary: design only, no code, or the design is done and only development remains.

You do not need to mention commands. The prompt names the workflow explicitly — ask first, confirm, then create the task and write the PRD — because a bare requirement on its own lets the agent jump straight to coding. Once the flow is named, the agent follows it: if the request is big enough to be a task, `workflow.md` requires it to ask for your agreement before creating one — it will not silently create a task directory. It may also ask clarifying questions first (for example, whether "other" needs a minimum text length), and it will tell you what it is about to do before doing it.

Before the work starts, the demo project's conventions need to live in the specs — either you add them, or you ask the agent to ("Put the refund conventions into the spec files"). Either way they end up as three short additions:

`.sunshine/spec/system.md` — the general principle:

```text
## 演示项目约定

- 退款原因必填：用户提交退款时原因字段不能为空，否则拒绝提交。
```

(`system.md` is the spec the agent itself maintains in the background; see [section 6](#6-how-specs-are-maintained).)

`.sunshine/spec/backend.md` — the API contract:

```text
## 退款接口

- `POST /refunds`：`reason` 必填，缺失返回 400，错误码 `REASON_REQUIRED`。
```

`.sunshine/spec/frontend.md` — the form behavior:

```text
## 退款表单

- 原因输入框为下拉选择（质量问题 / 尺寸不符 / 其他），选“其他”时出现文本框。
```

(In English: system — "Demo project convention: the refund reason is required — submitting a refund with an empty reason is rejected"; backend — "`POST /refunds`: `reason` is required, missing it returns 400 with error code `REASON_REQUIRED`"; frontend — "The reason input is a dropdown (quality issue / wrong size / other); choosing 'other' reveals a text box.")

Once the specs say it, every later task follows it automatically — that is the point of writing specs down instead of repeating the rule in every prompt.

## 3. The agent works through workflow.md

This is what the agent does after you give it the request. You watch it happen in the UI; you do not type any of this.

**Phase 1 — planning.** The agent creates the task directory `.sunshine/tasks/11-10-给退款原因加必填校验/` and writes `prd.md` from a template:

```markdown
# 给退款原因加必填校验

## 目标

退款时原因字段必填

## 需求

- 待定

## 验收标准

- [ ] 待定
```

("Target: the refund reason field is required when refunding.") Then it fills in the real requirements and acceptance criteria. A light task can stop at the PRD; a complex task also gets `design.md` (technical design) and `implement.md` (execution plan) before anything is implemented. It also organizes the context lists `implement.jsonl` / `check.jsonl` — the spec and research files the sub-agents will read. For a complex task it asks you to review the plan before starting implementation; only then does it move the task to `in_progress`.

There are two modes, decided at the start of planning: design-first, where the product design documents (requirements list, outline design, high-fidelity prototype — whatever the project needs, if any) land in the project's `doc/` folder first and the work is then split into sub-tasks, or direct development, where the task goes straight from the PRD to implementation. The design documents live in `doc/`, not in `.sunshine`; `.sunshine` only keeps the index (the task record, `prd.md`, and the `implement.jsonl` / `check.jsonl` lists) pointing at them. Section 12.6 walks the design-first mode end to end.

**Phase 2 — execution.** The agent dispatches sub-agents: `sunshine-implement` writes the code, then `sunshine-check` verifies it. The platform hooks inject the context automatically — the `implement.jsonl` / `check.jsonl` entries, plus `prd.md`, `design.md`, and `implement.md` if they exist — so each sub-agent starts with the right specs in its prompt. The dispatch prompt always opens with `Active task: <task path>`. The fixed order in `workflow.md` is:

```text
sunshine-implement -> sunshine-check -> sunshine-update-spec -> commit (step 3.4) -> /sunshine:finish-work
```

(`sunshine-implement` → `sunshine-check` → `sunshine-update-spec` → commit (step 3.4) → `/sunshine:finish-work`.)

**Phase 3 — wrap-up.** If the same problem needed repeated fixes, the agent loads the `sunshine-break-loop` skill and records the root cause. It then loads `sunshine-update-spec` and asks whether this task produced anything worth writing back into `.sunshine/spec/` — a new pattern, a pitfall, a technical decision — and updates the specs if so, even if the answer is "nothing to update" (it still has to make that judgement). It commits the code changes, and finally runs `/sunshine:finish-work`, which archives the task and records the session in the developer journal.

Your part during all of this: read what the agent says it is doing, answer its questions, and ask it to change course when it goes the wrong way. The commands it runs are listed in the next section — for your understanding, not for you to type.

## 4. What happened under the hood

These are the commands the agent actually ran for the demo task, with real outputs captured by running the equivalent commands. You are reading them to understand the mechanism; in normal use you never type them.

### Creating the task

```sh
sunshine task create "给退款原因加必填校验" --description "退款时原因字段必填" --priority P1
```

Real output:

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

(The warning says the default branch could not be resolved — no remote or offline — so `base_branch` is recorded as the current checkout branch `master` for now, overridable with `--base-branch`. The "next steps" say: write requirements and acceptance criteria in `prd.md`; a light task can have only a PRD; a complex task should add `design.md` and `implement.md` before `task start`; organize `implement.jsonl` / `check.jsonl` — one JSON object per line, `{"file": "<path>", "reason": "<reason>"}`, spec/research docs only.)

`create` prepends the `MM-DD-` date to the task name automatically — you never type the date yourself. `--description` is required; omitting it fails immediately:

```text
错误：--description 必填
请传入 --description "<该任务交付什么>"（纯空白不算）：空描述的任务会在归档时被拒绝。
```

("Error: `--description` is required. Pass `--description "<what this task delivers>"` (blank does not count): a task with an empty description is refused at archive time.")

`create` writes `task.json` with 24 fields in a fixed order (shown here as archived — `status` becomes `completed` and `completedAt` is filled in at archive time):

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

### Organizing the context lists

The agent registers the files the sub-agents need — `implement` for files to read while implementing, `check` for files to check against while verifying:

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
# 目标目录：/tmp/rewrite-en/demo/.sunshine/tasks/11-10-给退款原因加必填校验
#
#   implement.jsonl：✓（1 条）
#   check.jsonl：✓（1 条）
#
# ✓ 全部校验通过
```

("已添加文件" = "file added"; "上下文文件" = "context files"; "校验上下文文件" = "validating context files"; "全部校验通过" = "all checks passed".) The lists are stored as one JSON object per line, for example `{"file":"src/Refund.vue","reason":"实现退款原因必填校验"}`.

If the context lists are still empty, `start` refuses outright and tells the agent what to do:

```text
错误：implement.jsonl 和 check.jsonl 没有已整理条目
子代理（implement/check）将得不到任何规范上下文。
  整理：sunshine task add-context 11-10-给退款页加返回按钮 implement <路径> "<原因>"
  检查：sunshine task validate 11-10-给退款页加返回按钮
  确实要空着：重新运行 start 并加 --allow-empty-context
```

("Error: `implement.jsonl` and `check.jsonl` have no organized entries; the implement/check sub-agents will get no spec context. Organize with… / check with… / if you really want it empty, re-run start with `--allow-empty-context`.")

### Starting, viewing, finishing

```sh
sunshine task start 11-10-给退款原因加必填校验
```

Real output:

```text
ℹ 会话身份不可用；本次不持久化当前任务指针（降级模式），AI 仍可按对话上下文继续。
提示：在能提供会话身份的 AI 会话内运行，或先设置 SUNSHINE_CONTEXT_ID 再执行 sunshine task start。
✓ 状态：planning → in_progress（降级）
✓ 已记录分支：master（降级）
警告："master" 也是该任务的 base_branch，PR 不能指向自身分支，归档时会拒绝。
切出分支后运行：sunshine task set-branch <任务> <功能分支>
```

("Session identity unavailable; the current-task pointer is not persisted this time (degraded mode), the AI can still continue from the conversation context. Tip: run inside an AI session that provides a session identity, or set `SUNSHINE_CONTEXT_ID` before running `sunshine task start`. Status: `planning` → `in_progress` (degraded). Recorded branch: `master` (degraded). Warning: `master` is also the task's `base_branch`; a PR cannot point at its own branch, archiving will refuse. After checking out a branch, run: `sunshine task set-branch <task> <feature branch>`.")

That "degraded mode" is a property of running the command in a plain terminal, where there is no session identity — `workflow.md` itself says `create` points at the new task automatically "when a session identity is available". Inside the agent session the identity exists, so the pointer is persisted normally. Either way, `.sunshine/.current-task` is still written, so the current task can be found:

```sh
sunshine task current
# .sunshine/tasks/11-10-给退款原因加必填校验

sunshine task list
# 全部活动任务：
#
#   - 11-10-给退款原因加必填校验/ (in_progress) [演示者] <- 当前任务
#
# 总计：1 个任务

sunshine task context
# ========================================
# 会话上下文
# ========================================
#
# ## 开发者
# 名字：演示者
#
# ## Git 状态
# 分支：master
# 工作区：4 处未提交改动
# ……
# ## 当前任务
# 路径：.sunshine/tasks/11-10-给退款原因加必填校验
# 来源：file:.sunshine/.current-task
# 名字：给退款原因加必填校验
# 状态：in_progress
# 创建：2026-11-10
# 描述：退款时原因字段必填
# ……
```

("全部活动任务" = "all active tasks"; "总计：1 个任务" = "total: 1 task"; "会话上下文" = "session context"; "开发者" = "developer"; "Git 状态" = "git status"; "当前任务" = "current task"; "来源" = "source".)

### Archiving and recording the session

At the end of the work the agent runs `/sunshine:finish-work`, which archives the task and records the session in the journal. Archiving is strict about branches — in the demo project the task never went through a PR, so the plain archive is refused, and that is real behavior, not a mistake:

```text
警告：记录的分支 "master" 已不存在（可能已合并删除）。
错误：拒绝归档 "11-10-给退款原因加必填校验"：branch 与 base_branch 都是 "master"，PR 不能指向自身分支，这份元数据无法描述已合并的工作。
请修复其中一个字段：
  sunshine task set-branch 11-10-给退款原因加必填校验 <功能分支>
  sunshine task set-base-branch 11-10-给退款原因加必填校验 <目标分支>
  sunshine task archive 11-10-给退款原因加必填校验 --skip-branch-validation   # 仅当该任务从未走 PR
未归档：.sunshine/tasks/11-10-给退款原因加必填校验 未动。
```

("Warning: the recorded branch `master` no longer exists (may have been merged and deleted). Error: refusing to archive …: branch and base_branch are both `master`; a PR cannot point at its own branch, this metadata cannot describe merged work. Fix one of the fields: … / … / … # only if the task never went through a PR. Not archived: … untouched.")

In a real project the agent would `set-branch` to the feature branch first. The demo task never went through a PR, so it skips the check:

```sh
sunshine task archive 11-10-给退款原因加必填校验 --skip-branch-validation
# 警告：记录的分支 "master" 已不存在（可能已合并删除）。
# 已归档：11-10-给退款原因加必填校验 -> archive/2026-11/
# [OK] 已自动提交：chore(task): archive 11-10-给退款原因加必填校验

sunshine task list-archive
# 已归档任务：
#
# [2026-11] - 1 个任务
#   - 11-10-给退款原因加必填校验/
```

("Archived: … -> `archive/2026-11/`; auto-committed: `chore(task): archive …`; archived tasks: `[2026-11]` - 1 task.") The archive path is `.sunshine/tasks/archive/<YYYY-MM>/`, and archiving auto-commits by default (while `session_auto_commit` is `true`).

The session record goes into `.sunshine/workspace/<your name>/journal-1.md`, with the auto-generated section of `index.md` updated at the same time:

```sh
sunshine task add-session --title "退款原因必填校验实现" --summary "加了原因下拉框与必填校验"
```

Real output (excerpt):

```text
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

("Adding session; session: 1; title: …; commit: -; current journal file: `journal-1.md`; current lines: 7; lines added: 21; total after append: 28; session appended to `journal-1.md`; session 1 added successfully; updated files: `journal-1.md`, `index.md`; auto-committed: `chore: record journal`.")

One environment note, recorded as it happened: the auto-commit needs a configured git identity. In the first demo run the sandbox had none, so archiving and `add-session` printed `[警告] 自动提交失败：Author identity unknown` while still writing the files, and re-running the exact same `add-session` command resumed from the commit step instead of appending a second session. Set `git config user.email` / `user.name` once and the `[OK] 已自动提交` lines above are what you get.

## 5. You review and accept

The agent does the work; you decide whether it is done. When it says the task is finished, review it in the UI the same way you would review a colleague's change:

- Read the diff the agent shows you (or run `git diff` yourself) — the refund form change, the backend validation, and nothing else. `workflow.md` has an explicit rule for this: only change the affected parts; untouched code and styles stay untouched.
- Check it against the acceptance criteria in `prd.md` — the criteria you agreed in phase 1 are the definition of done, not the agent's summary.
- Run the thing yourself if it matters: submit a refund with an empty reason and confirm it is rejected; pick "other" and confirm the text box appears.

If something is off, say so in plain language and the agent fixes it — it is still the same task, still the same conversation:

> **Example prompt** (an illustration of what to say, not a real session transcript):
>
> The reason dropdown works, but choosing "other" doesn't show the text box yet. Fix that, and keep the backend validation as it is.

When you accept, the commit has already happened in workflow step 3.4 — the agent batches the code changes into commits before `/sunshine:finish-work` runs, and the finish command itself does no code committing (it only archives the task and records the journal). So "accept" means: the diff is right, the criteria are met, and the commits are already in your branch. Push when you are ready, the same as any other work.

## 6. How specs are maintained

Specs are not a document you write once and forget. `.sunshine/spec/system.md` states the rule at the top:

```text
# 系统规范（自主沉淀总原则）

本文件是 Sunshine 规范自维护的总原则。AI 助手在后台自主、静默地维护 `.sunshine/spec/`，人只负责审阅成果。

## 总原则

- 技术选型、分层架构、数据库规约等确定后，立即写入 `.sunshine/spec/` 对应文件，不等人手动整理。
- 规范一经写入，后续所有任务自动遵守。
- 发现规范与现实不符（代码已改、规范未改），以现实为准并同步修正规范。
```

("System spec — the general principle of autonomous spec maintenance. The AI assistant maintains `.sunshine/spec/` autonomously and silently in the background; a human only reviews the result. / General principles: once a technical choice, layering, or database convention is settled, write it into the matching file under `.sunshine/spec/` immediately, don't wait for manual tidying. Once a spec is written, all later tasks follow it automatically. If a spec disagrees with reality (the code changed, the spec didn't), reality wins and the spec is corrected.")

In practice that means two things:

- **During a task**, the agent writes what it learned back into the specs (workflow step 3.3, `sunshine-update-spec`) — for the demo task, the `REASON_REQUIRED` error code and the "other reveals a text box" rule are exactly the kind of conventions that get written down so the next refund task does not have to rediscover them.
- **Between tasks**, you can also just tell the agent to update a spec ("Record in the backend spec that all list endpoints paginate with cursor-based pagination") and it edits the file; you review the diff like any other change.

Specs are user data: `sunshine update` never touches `.sunshine/spec/` — in the scan output in [section 8](#8-update-and-the-template-marketplace) it appears under "用户数据（保持不动）", "user data (kept untouched)". The one exception is a spec installed from the template marketplace, which is hash-tracked and refreshed by `update` — see [section 8](#8-update-and-the-template-marketplace).

## 7. Switching workflows

There are three built-in workflow templates. You usually switch by asking the agent in the UI ("Switch the workflow to TDD for the next task"), and the agent runs the command; running it yourself in the terminal works the same way:

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
```

("Available workflow templates: `native` (built-in) — the native Sunshine workflow: the default plan / execute / wrap-up workflow with native sub-agents and inline platform branches; `tdd` (built-in) — TDD workflow: a variant that drives phase 2 with one red / green / refactor behavior slice at a time; `channel-driven-subagent-dispatch` (built-in) — channel-driven sub-agent dispatch: a variant where the main session coordinates implement/check worker threads through `sunshine channel`.")

Switch to the TDD workflow:

```sh
sunshine workflow --template tdd
#   ✓ 已把 .sunshine/workflow.md 换成 "tdd"
```

("Switched `.sunshine/workflow.md` to `tdd`.")

Switching back to native stops you first — because `workflow.md` is now the tdd version, which no longer matches the hash manifest, the CLI treats it as "locally modified":

```sh
sunshine workflow --template native
# 错误：.sunshine/workflow.md 已有本地修改，请加 --force 覆盖，或用 --create-new 写 .sunshine/workflow.md.new

sunshine workflow --template native --force
#   ✓ 已把 .sunshine/workflow.md 换成 "native"

sunshine workflow --template native --create-new
#   + 已写入 .sunshine/workflow.md.new（workflow.md 未改动）
```

("Error: `.sunshine/workflow.md` has local modifications; add `--force` to overwrite, or use `--create-new` to write `.sunshine/workflow.md.new`. / Switched `.sunshine/workflow.md` to `native`. / Wrote `.sunshine/workflow.md.new` (`workflow.md` untouched).")

The rule: unmodified files are overwritten directly; modified or unknown-origin files ask interactively (overwrite / write a `.new` copy / skip), and in a non-interactive terminal you use `--force` to overwrite or `--create-new` to write a copy. `--create-new` only writes `.sunshine/workflow.md.new` and leaves the active `workflow.md` alone. The same three-state logic is what `update` uses in the next section — a switched workflow shows up there under "你已修改" ("modified by you").

## 8. update and the template marketplace

### `update`: refresh the template files in the project

When a new Sunshine version ships new templates, you refresh the project with `update`. First look at the plan with `--dry-run` — it prints the plan and writes nothing:

```sh
sunshine update --dry-run
```

Real output (excerpt; run with the v0.2.1 binary, project on 0.2.1):

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
    ……另有 102 个

  用户数据（保持不动）：
    ○ .sunshine/workspace/
    ○ .sunshine/tasks/
    ○ .sunshine/spec/
    ○ .sunshine/.developer/

✓ 已是最新，无需更新。
```

("Scanning changes… Unchanged files (will be skipped): … / User data (kept untouched): `.sunshine/workspace/`, `.sunshine/tasks/`, `.sunshine/spec/`, `.sunshine/.developer/` / Already up to date, nothing to update.")

Three states at a glance: unchanged template files are updated automatically; files you modified ask one by one (overwrite / skip / write a `.new` copy, default skip); user data (specs, tasks, workspace, developer identity) is never touched. Switch the workflow to tdd first ([section 7](#7-switching-workflows)) and the same scan shows the third state in action:

```text
  你已修改（需要你决定）：
    ? .sunshine/workflow.md

  用户数据（保持不动）：
    ○ .sunshine/workspace/
    ○ .sunshine/tasks/
    ○ .sunshine/spec/
    ○ .sunshine/.developer/

[试运行] 未做任何修改。
```

("Modified by you (needs your decision): `.sunshine/workflow.md` / User data (kept untouched): … / [Dry run] No changes made.")

To actually run it, drop `--dry-run`. Before writing, it takes a full backup — verified by running it for real on the tdd-switched project (non-interactive, so the conflict file was skipped by default):

```text
非交互环境，跳过统一确认（冲突文件默认跳过）。

已创建备份：.sunshine/.backups/2026-11-10T01-16-37/

--- 逐个确认冲突 ---
  ○ 已跳过：.sunshine/workflow.md

--- 汇总 ---
  已跳过：1 个文件
  备份：.sunshine/.backups/2026-11-10T01-16-37/

✓ 更新完成！（0.2.1 → 0.2.1）
```

("Non-interactive environment, skipping the unified confirmation (conflicting files are skipped by default). / Backup created: `.sunshine/.backups/2026-11-10T01-16-37/` / … skipped: `.sunshine/workflow.md` / … / Update complete! (0.2.1 → 0.2.1).") On success it updates `.sunshine/.version`.

Who runs `update`? Usually you, in the terminal, when you know a new version is out — or you ask the agent to ("Check whether the project templates need updating") and it runs `update --dry-run` and shows you the plan. Either way, nothing is overwritten silently.

### The template marketplace: install specs from a registry

A template marketplace is a repository of reusable specs, laid out as `marketplace/index.json` + `marketplace/specs/`. You point `init` at one to start a project from a shared spec instead of the built-in templates. Here is a minimal local marketplace, built for this tutorial:

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

Two gotchas, both found by actually running it:

1. A local path must point at the `marketplace` subdirectory itself (`…/registry-src/marketplace`), not the repository root — pointing at the root fails with `错误：该 registry 没有 index.json，不能用 --template；去掉 --template 走直接下载模式` ("this registry has no index.json, `--template` cannot be used; drop `--template` for direct-download mode").
2. The `path` in `index.json` is relative to the directory containing `index.json`, so write `specs/demo-spec`, not `marketplace/specs/demo-spec`.

Then install from the marketplace in a new project:

```sh
mkdir demo3 && cd demo3 && git init
sunshine init -u 演示者 -y --registry /tmp/rewrite-en/registry-src/marketplace --template demo-spec
```

Real output:

```text
正在配置 Claude Code……
正在配置 Cursor……
已安装模板 "demo-spec"（.sunshine/spec），共 1 个文件。
已记录 registry.spec：/tmp/rewrite-en/registry-src/marketplace（模板 demo-spec），sunshine update 会持续刷新。
初始化完成，开发者：演示者
已安装平台：
  - Claude Code
  - Cursor
下一步：
  1. 在 agent 界面里打开项目，直接用自然语言提第一个需求；
  2. 规范已从模板市场装进 .sunshine/spec/，工作流按 .sunshine/workflow.md 自动推进，一般不用改。
```

("Installed template `demo-spec` (`.sunshine/spec`), 1 file in total. Recorded `registry.spec`: `/tmp/rewrite-en/registry-src/marketplace` (template `demo-spec`); `sunshine update` will keep refreshing it. Initialization complete…")

The template content lands in `.sunshine/spec/demo.md`, and the source is recorded in `config.yaml`:

```yaml
# 受哈希追踪的 spec 模板，本地改动走正常的 update 冲突流程。
registry:
  spec:
    source: /tmp/rewrite-en/registry-src/marketplace
    template: demo-spec
```

("Spec templates tracked by hash; local edits go through the normal update conflict flow.") A remote marketplace works the same way — replace the local path with `gh:owner/repo`, a full git URL, an HTTP(S) URL, or a `git@` address. The above uses `-y` with `--template`; in interactive mode, omitting `--template` makes init list that marketplace's spec templates for you to pick, so you don't need to know the template id in advance (a marketplace without `index.json` downloads the whole source directly, with no picker).

After the marketplace template changes, `update` keeps refreshing it. Edit the template in the marketplace, commit, and run `update --dry-run` again:

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

("Template updated (will be updated automatically): `.sunshine/spec/demo.md` / Unchanged files (will be skipped): … / Modified by you (needs your decision): `.sunshine/config.yaml` / User data (kept untouched): …")

`demo.md` is updated automatically because its local copy was never edited; if you had edited it locally, it would go through the same "modified by you" confirmation. `config.yaml` shows as "modified by you" because `registry.spec` was written into it when the template was installed — that is expected.

## 9. mem: look back at sessions

`mem` parses the session records each platform keeps on this machine, read-only — it never writes to session storage. Real usage first: once you have run Sunshine tasks in Claude Code, just ask the agent in the UI ("Find the session where we implemented the refund reason validation") or run these in the project yourself:

```sh
sunshine mem list              # List this project's sessions
sunshine mem search "退款"      # Find sessions by keyword
sunshine mem context 11111111  # View one session's context (the id can be abbreviated)
sunshine mem extract 11111111  # Export one session's full conversation
sunshine mem projects          # List projects that have sessions
```

The outputs below are demonstrated with a hand-made fixture, because the sandbox has no real sessions. The fixture is a real session file in exactly the layout a real session has — `~/.claude/projects/<project directory>/<session id>.jsonl` — four turns (two user, two assistant) about the refund task, written under a temporary `HOME` so your real `HOME` is untouched:

```sh
export HOME=/tmp/rewrite-en/home   # Demo only; your real HOME is untouched
mkdir -p "$HOME/.claude/projects/-tmp-rewrite-en-demo"
# Write a 4-turn demo session 11111111-2222-3333-4444-555555555555.jsonl
# (two user turns, two assistant turns, about this tutorial's refund task)
```

```sh
sunshine mem list
# 范围：项目=/tmp/rewrite-en/demo  平台=all
# [claude  ] 2026-11-10 01:14  11111111-222  /tmp/rewrite-en/demo
#
# 共 1 个会话

sunshine mem search "退款"
# 范围：项目=/tmp/rewrite-en/demo  平台=all  关键词="退款"
#
# [claude  ] 2026-11-10 01:14  11111111-222  /tmp/rewrite-en/demo  相关度=0.250  命中=1（用户=1，助手=0）  轮次=4
#     [用户] 给退款原因加必填校验，原因字段不能为空
#
# 共 1 个会话

sunshine mem context 11111111
# # 上下文：[claude] 11111111-2222-3333-4444-555555555555
# # 工作目录：/tmp/rewrite-en/demo
# # 未指定关键词 — 显示前 3 轮，共 4 轮
# # 显示轮次：4  已用预算：83/6000 字符
#
# ## 第 0 轮（用户） ← 命中
#
# 给退款原因加必填校验，原因字段不能为空
# ……
# ## 第 3 轮（助手）
#
# 已加上，原因选“其他”时会出现文本框。

sunshine mem extract 11111111
# # 会话：[claude] 11111111-2222-3333-4444-555555555555
# # 工作目录：/tmp/rewrite-en/demo
# # 日期：2026-11-10 14:13
# # 阶段：all  轮次：4/4
# ……
# ## 助手
#
# 已加上，原因选“其他”时会出现文本框。

sunshine mem projects
# 活跃项目
# 2026-11-10 01:14  会话数=  1 (claude:1)  /tmp/rewrite-en/demo
#
# 共 1 个项目
```

("范围" = "scope"; "共 1 个会话" = "1 session in total"; "相关度" = "relevance"; "命中" = "hits"; "轮次" = "turns"; "上下文" = "context"; "工作目录" = "working directory"; "未指定关键词 — 显示前 3 轮，共 4 轮" = "no keyword given — showing the first 3 of 4 turns"; "显示轮次" = "turns shown"; "已用预算" = "budget used"; "活跃项目" = "active projects"; "会话数" = "session count".)

Each of these outputs starts with a few `警告：未找到 Codex 会话存储（…），已跳过` ("warning: Codex session storage not found (…), skipped") lines — one per platform with no session records on this machine. Those only mean that platform has no sessions here; they are not errors, and they are omitted from the excerpts above. `--global` searches all projects, `--json` outputs machine-readable JSON, and `--platform claude` restricts the search to one platform.

## 10. channel: multi-agent collaboration

A channel is a session where several workers (claude / codex) collaborate: `create` opens a session, `spawn` registers a worker, `send` sends a message, `wait` waits for an event. In normal use you ask for this in the agent UI — for example:

> **Example prompt** (an illustration of what to say, not a real session transcript):
>
> Open a review channel for the refund task, have the implement worker do the validation, and report back when it is done.

The main session then drives the channel (that is exactly what the `channel-driven-subagent-dispatch` workflow template in [section 7](#7-switching-workflows) does). The commands below are what it runs; you would only type them yourself when debugging a channel. The storage root defaults to `~/.sunshine/channels/` and can be overridden with `SUNSHINE_CHANNEL_ROOT` — this tutorial uses a temporary directory so your real channel storage is untouched:

```sh
export SUNSHINE_CHANNEL_ROOT=/tmp/rewrite-en/channels
```

### Create a session, send a message, view events

```sh
sunshine channel create review --type chat
# 已创建 channel 'review'（chat）位于 /tmp/rewrite-en/channels/-tmp-rewrite-en-demo/review

sunshine channel send review --to impl "开始实现退款原因必填校验"
# {"by":"main","kind":"message","origin":"cli","seq":2,"text":"开始实现退款原因必填校验","to":"impl","ts":"2026-11-10T01:14:48.971360806Z"}

sunshine channel messages review
# [1] 01:14:48 [create] by=main cwd=/tmp/rewrite-en/demo
# [2] 01:14:48 [message] by=main to=impl
#          开始实现退款原因必填校验

sunshine channel list
# 名称                        工作者        事件      最近活动                 最近事件       类型       任务
# review                    -          2       -                    message    chat     -
```

("Created channel `review` (chat) at …") `send` prints one JSON event (`seq` is the event sequence number); `messages` is the human-readable timeline. You can bind a task when creating the session: `sunshine channel create review --type chat --task .sunshine/tasks/11-10-给退款原因加必填校验`.

### Title and context

```sh
sunshine channel title set review --title "退款原因必填校验"
# {"action":"title","by":"main","kind":"channel","origin":"cli","seq":3,"title":"退款原因必填校验","ts":"2026-11-10T01:14:48.996484414Z"}

sunshine channel context add review --raw "只改退款逻辑，不动其他接口"
# {"action":"add","by":"main","context":[{"text":"只改退款逻辑，不动其他接口","type":"raw"}],"kind":"context","origin":"cli","seq":4,"target":"channel","ts":"2026-11-10T01:14:49.005600038Z"}

sunshine channel context list review
# raw  只改退款逻辑，不动其他接口
```

(The raw context entry means "only change the refund logic, don't touch other interfaces".)

### Waiting for an event: a timeout is a normal result

```sh
sunshine channel wait review --from impl --kind done --timeout 3s
# 错误：超时：等待 channel 'review' 超时（3s），仍在等待：impl（上游退出码 124）
```

("Error: timeout: waiting for channel `review` timed out (3s), still waiting: `impl` (upstream exit code 124).") With no worker replying, `wait` times out after `--timeout` and exits with an error — that is what it is for: the agent uses it to wait for a worker's `done` event.

### Forum channels: threaded discussion

A `forum`-type channel organizes discussion into threads (topics). Forum commands do not work on `chat` channels — verified: `错误：频道 "review" 的类型是 chat，post 操作只支持 forum 类型的频道` ("channel `review` is of type chat; `post` only supports forum-type channels"):

```sh
sunshine channel create board --type forum
# 已创建 channel 'board'（forum）位于 /tmp/rewrite-en/channels/-tmp-rewrite-en-demo/board

sunshine channel post board opened --as main --thread t1 --title "退款原因实现方案" --description "原因下拉框：质量问题 / 尺寸不符 / 其他"
# {"action":"opened","by":"main","description":"原因下拉框：质量问题 / 尺寸不符 / 其他","kind":"thread","origin":"cli","seq":2,"thread":"t1","title":"退款原因实现方案","ts":"2026-11-10T01:14:54.822100697Z"}

sunshine channel post board comment --as alice --thread t1 --text "收到，正在看"
# {"action":"comment","by":"alice","kind":"thread","origin":"cli","seq":3,"text":"收到，正在看","thread":"t1","ts":"2026-11-10T01:14:54.830614516Z"}

sunshine channel forum board
# 主题  状态  标题
# t1 [open] 退款原因实现方案

sunshine channel thread board t1
# t1 [open] 退款原因实现方案
# 描述：原因下拉框：质量问题 / 尺寸不符 / 其他
#   2026-11-10 01:14:54 opened 发帖人=main
#   2026-11-10 01:14:54 comment 发帖人=alice 收到，正在看
```

("主题 / 状态 / 标题" = "topic / status / title"; "描述" = "description"; "发帖人" = "posted by"; "收到，正在看" = "got it, looking into it".)

### spawn: requires a real, logged-in worker

`spawn` is the only step in channel that needs a real environment: it starts the local `claude` or `codex` process. The prerequisite is that the corresponding CLI is installed **and logged in**. The sandbox had neither, and the verified results are recorded here as they happened:

```sh
sunshine channel spawn review --agent implement --provider claude --as impl
# {
#   "pid": 8959,
#   "log": "/tmp/rewrite-en/channels/-tmp-rewrite-en-demo/review/impl.log",
#   "worker": "impl"
# }
```

The command itself returned a pid and a log path, but the event log immediately recorded a failure:

```sh
sunshine channel messages review
# ……
# [5] 01:14:54 [error] by=supervisor:impl worker 拉起失败：拉起 claude worker 失败：exec: "claude": executable file not found in $PATH
```

("worker failed to start: failed to start claude worker: `exec: "claude": executable file not found in $PATH`.")

Trying codex instead (the sandbox has a codex binary, but it is not logged in): the process did start, then failed while connecting with a 401, recorded in the worker log:

```text
ERROR codex_api::endpoint::responses_websocket: failed to connect to websocket: HTTP error: 401 Unauthorized, url: wss://api.openai.com/v1/responses
```

So: **on a machine with claude / codex installed and logged in, messages sent with `send` after `spawn` are received and executed by the worker; without a logged-in worker, `spawn` only leaves an `error` event (or a 401 in the worker log) — it never pretends to succeed.** The same prerequisite applies to `channel run`, which builds a temporary channel, runs one task, and prints the final answer.

### Stop a worker, delete a channel

```sh
sunshine channel kill review --as impl-codex
# 已停止 worker 'impl-codex'（channel 'review'）

sunshine channel create tmp --type chat
# 已创建 channel 'tmp'（chat）位于 /tmp/rewrite-en/channels/-tmp-rewrite-en-demo/tmp

sunshine channel rm tmp
# 已删除 channel 'tmp'

sunshine channel prune --empty
# （无可清理的 channel）
```

("Stopped worker `impl-codex` (channel `review`). / Created channel `tmp`… / Deleted channel `tmp`. / (No channels to clean up).") `kill` takes the worker name with `--as <worker>`; `rm` kills all workers and deletes the whole channel directory. `prune` cleans up channels in bulk by condition (dry-run by default, nothing is really deleted).

## 11. upgrade: update the CLI itself

`upgrade` upgrades the CLI itself (`update` upgrades the template files inside a project — two different things). Check the plan first with `--dry-run`; nothing is written. This is a command you run yourself in the terminal — it is about your installation, not about a task, so the agent has no reason to run it.

When already on the latest version (output from a local build with the version injected as 0.2.1 via ldflags, matching the current Release):

```sh
sunshine upgrade --dry-run
# 已是最新版本（当前 0.2.1，最新 0.2.1），无需升级。
```

("Already on the latest version (current 0.2.1, latest 0.2.1), no upgrade needed.")

When a new version exists (output from the default local build, whose version is 0.1.0, against the same latest 0.2.1):

```text
正在升级 Sunshine：0.1.0 → 0.2.1（通道 latest）
资产：sunshine_0.2.1_linux_amd64.tar.gz
下载地址：https://github.com/linrong007/Sunshine/releases/download/v0.2.1/sunshine_0.2.1_linux_amd64.tar.gz
备份：将把当前二进制备份为 /tmp/rewrite-en/sunshine.bak
[试运行] 未做任何修改。
```

("Upgrading Sunshine: 0.1.0 → 0.2.1 (channel latest); asset: `sunshine_0.2.1_linux_amd64.tar.gz`; download URL: …; backup: the current binary will be backed up as `/tmp/rewrite-en/sunshine.bak`; [dry run] no changes made.")

A real upgrade (drop `--dry-run`) follows the same flow: check the latest version → pick the asset for the current OS and architecture → download → verify sha256 against `checksums.txt` → back up the current binary (a `.bak` next to it) → replace → run the new `--version` to verify, rolling back to the backup and reporting an error on failure. You can also pin a version or channel: `sunshine upgrade --tag 0.2.1`, `sunshine upgrade --tag beta`.

## 12. Six scenarios: from day one to daily work

Sections 1–11 cover the main flow; this section walks through six real scenarios, each one a complete end-to-end run:

| Scenario | When it applies | The point |
| --- | --- | --- |
| 1. Starting a new project from zero | A new product, service, package, or internal tool | Empty directory, `init`, first task grows from nothing |
| 2. Adopting an existing project | The repo exists, but the conventions live mostly in people's heads | `init` touches no existing code; land the conventions in spec first |
| 3. Delivering a product feature | One task spans product, API, data, and UI | One task + cross-layer context + channel coordination |
| 4. Refactoring a legacy module | The code runs, but it is no longer safe to change | Pin the behavior with tests first, then refactor, tests green throughout |
| 5. Fixing a bug that keeps coming back | The same class of problem appears again and again | `mem` finds the last session first, then a task, then the lesson goes into spec |
| 6. Design-first product work | Design comes before development | Design documents land in `doc/`; split into sub-tasks, then developers take them one by one |

### 12.1 Starting a new project from zero: a web store

An empty directory, nothing in it — the cleanest starting point. Create an empty directory, put it in git, and leave the rest to Claude Code: `init`, creating the task, lining up context, starting it — all of it runs underneath, and you type no commands. An empty directory has no detectable type, so it defaults to full-stack and all three specs (system.md, frontend.md, backend.md) are generated — a new project starts with a default set of specs, and the team adjusts them later, which beats starting with nothing.

Open the project in Claude Code and state the first requirement in plain language (same for Codex and Cursor):

Example prompt (an example, not a record of a real session):

> I'm starting a web store from scratch: first the product list — the home page shows the product name, price, and stock. Please follow the Sunshine workflow: ask me first to pin down the unclear product logic and technical decisions; once confirmed, create the first task, write the PRD, then implement.

Here is how the session continues (the conversation is an example; the commands and outputs are real values from runs):

> **Claude Code:** One question first — it decides how this is built: which stack — a single Go file on the standard library `net/http`, or a framework, or a database?
>
> **You:** A single Go file, standard library only, module `webstore`, no database.
>
> **Claude Code:** Good, next: where does the product data live — hard-coded for now, or read from a file?
>
> **You:** Hard-code it for now, three products is enough for the demo: 无线鼠标 (wireless mouse) ¥89, stock 120; 机械键盘 (mechanical keyboard) ¥299, stock 45; 27寸显示器 (27" monitor) ¥1299, stock 20.
>
> **Claude Code:** One more: which fields does the list show?
>
> **You:** Name, price, and stock, prices in yuan.
>
> **Claude Code:** Good, that's all the questions. Here's the final planning summary — once you approve it, I'll get moving:
>
> **Final planning summary**
>
> - Goal: build the web store from scratch, a single Go file, port 18080; first the product list.
> - In scope: the product list (list page + `GET /api/products`); the three products hard-coded.
> - Out of scope: login, payment, ordering, orders, persistence, styling polish; add-to-cart is the next requirement, not this task.
> - Key decisions (all confirmed): a single Go file on the standard library, no third-party dependencies; product data hard-coded; port 18080.
> - Risks / deferred items: restarting loses the data (accepted, enough for the demo); no auth, no persistence (to decide later).
> - Artifact status: the planning task's PRD + the three specs (system/frontend/backend) + the PRD of the first implementation task, "商品列表" (Product list); zero product code (0 `.go` files), no `task start` yet.
>
> Once you approve this plan, I'll create the planning task, write the PRD and the three specs — still no product code, and the implementation task stays unstarted. OK?
>
> **You:** Approved.
>
> **Claude Code:** The planning artifacts are on disk: the three specs and the "商品列表" (Product list) PRD are written, zero product code, the implementation task is created but not started. Here's the delivery list for your confirmation — once you approve it, I'll start the implementation.
>
> **You:** Approved.
>
> **Claude Code:** Starting the task, dispatching the `sunshine-implement` subagent. Implementation done, dispatching the `sunshine-check` subagent: `go vet` and `go build` are green, nothing to fix. Let me show you the run:

```sh
curl -s localhost:18080/api/products
# [{"id":1,"name":"无线鼠标","price":89,"stock":120},{"id":2,"name":"机械键盘","price":299,"stock":45},{"id":3,"name":"27寸显示器","price":1299,"stock":20}]
```

Open the home page in a browser and the three products are there too: 无线鼠标 89 元 (wireless mouse, ¥89), 机械键盘 299 元 (mechanical keyboard, ¥299), 27寸显示器 1299 元 (27" monitor, ¥1299).

> **Claude Code:** Step 3.3, spec write-back: no update — the implementation followed exactly the decisions written into the spec during planning, no new patterns, no new pitfalls. Here's the commit plan — reply "OK" and I'll run it:
>
> ```text
> Suggested commits (in order):
>   1. chore: 初始化 Sunshine 工程骨架（全栈）
>   2. feat: 商品列表页 + GET /api/products
>      - go.mod, main.go, the two task directories
> Unrecognized dirty files (in no commit — please confirm include or exclude):
>   - None
> ```
>
> **You:** OK.
>
> **Claude Code:** Commits done: `98d1e5d chore: 初始化 Sunshine 工程骨架（全栈）`, `acaa3e3 feat: 商品列表页 + GET /api/products`. The rest is wrap-up: run `/finish-work` to archive the tasks and record the session log. Shall I run it now?
>
> **You:** Go ahead.

The first archive attempt was refused: the current branch `main` is also the `base_branch`, and a PR cannot point at its own branch. I cut `feat/product-list`, ran `set-branch` for both tasks, and re-archived — through. `finish-work` archived the product-list task and the planning task, and recorded the session log:

```sh
git log --oneline
# eb7adc7 chore: record journal
# e961e60 chore(task): archive 11-13-webstore
# 71f3de9 chore(task): archive 11-13-product-list
# acaa3e3 feat: 商品列表页 + GET /api/products
# 98d1e5d chore: 初始化 Sunshine 工程骨架（全栈）
```

> **Claude Code:** Done — the tasks are archived:

```text
[2026-11] - 2 个任务
  - 11-13-product-list/
  - 11-13-webstore/
```

Then the second requirement:

> **You:** Next, the second requirement: add to cart.
>
> **Claude Code:** One question first: where does the cart live — in memory for now, or a file, or a database?
>
> **You:** In memory, enough for the demo.
>
> **Claude Code:** One more: what does adding return, and how is the total calculated?
>
> **You:** `POST /api/cart/add` to add, `GET /api/cart` for the items and the total; add a wireless mouse first, then a mechanical keyboard, total 388. An unknown id returns 404, adding the same product again accumulates the quantity.
>
> **Claude Code:** Good, here's the plan summary: the goal is add-to-cart; in scope are `POST /api/cart/add`, `GET /api/cart`, and an "加入购物车" (Add to cart) button after each product on the list page; out of scope are changing `GET /api/products` and persistence; the key decisions are an in-memory cart, quantity accumulates on repeat adds, and an unknown id returns 404. The artifact is the "加入购物车" (Add to cart) PRD, zero product code, not started. Once you approve it, I'll write the PRD; once you approve the delivery, I'll start the implementation. OK?
>
> **You:** Approved.
>
> **Claude Code:** The PRD is on disk, zero product code, not started. Confirm, and I'll start the implementation once you approve.
>
> **You:** Approved.
>
> **Claude Code:** Starting the task, dispatching the `sunshine-implement` subagent; implementation done, dispatching the `sunshine-check` subagent — green. Let me show you the run:

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

`GET /api/products` still returns the same three products, and every product on the home page has an "加入购物车" (Add to cart) button after it.

> **Claude Code:** Step 3.3, spec write-back: no update. One commit in the plan — reply "OK" and I'll run it:
>
> ```text
> Suggested commits (in order):
>   1. feat: 加入购物车 + GET /api/cart
>      - main.go, the 11-13-add-to-cart task directory
> Unrecognized dirty files: none
> ```
>
> **You:** OK.
>
> **Claude Code:** Commit done: `ba61066 feat: 加入购物车 + GET /api/cart`. Run `/finish-work` to archive and record the log — shall I run it now?
>
> **You:** Go ahead.

`finish-work` archived "加入购物车" (Add to cart) and recorded the session log:

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

> **Claude Code:** Done — the tasks are archived:

```text
[2026-11] - 3 个任务
  - 11-13-add-to-cart/
  - 11-13-product-list/
  - 11-13-webstore/
```

Two requirements done, and the project now has: a running store service, three archived task records, and the specs from the default templates.

One real pitfall here is worth writing down: add-to-cart was only raised later, and the planning task (`11-13-webstore`) had already been archived when the first task finished, so no subtask can be created under an archived parent — "加入购物车" (Add to cart) had to be created as an independent task (`parent: null`), with its PRD pointing back and forth to the archived parent. Per workflow 1.2, splitting happens exactly once — if both requirements had been known at planning time, both subtasks should have been created then, and the parent archived only after all its subtasks were archived; an archived task never reopens, and rework goes through a new task running the full flow.

### 12.2 Adopting an existing project: land the conventions in spec first

The repo exists, but the conventions live mostly in people's heads — the most common adoption case. The demo is a web store that has run for two years (`express` is the only dependency in `package.json`; the code is split three ways: `src/server.js` is the entry point, `src/routes/` holds the product and order endpoints, `src/db.js` handles data access), with the team's conventions written in `README.md` and nowhere else:

```markdown
# webstore

团队约定（一直靠人记，没落进任何文档）：
- 接口统一返回 { code, data }，code 是大写下划线错误码。
- 价格统一用“分”存库，展示时除以 100。
- 端口固定 3000，改端口要全组同步。
```

(In English: "Team conventions (always remembered by people, never written into any document): APIs return { code, data } with code an UPPER_SNAKE error code. Prices are stored in cents and divided by 100 for display. The port is fixed at 3000; changing it needs the whole team to sync.")

Run `init` right in this directory — `init` only adds files, it never changes existing code. Claude Code runs `init` underneath and recognizes a backend project (only `express`), so only `system.md` and `backend.md` are generated; existing files are untouched, and git shows only the new Sunshine directories:

```text
?? .claude/
?? .cursor/
?? .sunshine/
```

The first task on an adopted project is not a feature — it is landing the remembered conventions in spec, so the agent stops having to guess them every time. One sentence to Claude Code is enough (same for Codex and Cursor):

Example prompt (an example, not a record of a real session):

> I'm bringing an existing project into Sunshine: the repo exists, but the conventions live mostly in people's heads, and the README has three team conventions. Please follow the Sunshine workflow: ask me first to confirm the three conventions and which spec files they belong in; once confirmed, create the task, write the PRD, then write the conventions into .sunshine/spec/ so every task from now on follows them.

Here is how the session continues (the conversation is an example; the commands and outputs are real values from runs):

> **Claude Code:** One question first: where are the three conventions written right now?
>
> **You:** Nowhere — only spoken. The code shows the same practices, though.
>
> **Claude Code:** Next: what exactly do the three conventions say?
>
> **You:** APIs return `{code, data}`; prices are stored in cents and divided by 100 for display; the port is fixed at 3000.
>
> **Claude Code:** Next: this run writes spec only, no product code — right? And if the check finds the code disagreeing with a convention, what do I do?
>
> **You:** Right — zero product-code changes; if something disagrees, report it first, don't change the code on your own.
>
> **Claude Code:** Next: which spec file does each convention go in?
>
> **You:** The API format and the cents rule go into `backend.md`; port 3000 goes into `system.md`.
>
> **Claude Code:** Last one: what counts as done?
>
> **You:** The three conventions land in spec verbatim; check verifies each one against the code; then commit, archive, and record the log.
>
> **Claude Code:** Good, that's all the questions. Here's the final planning summary — once you approve it, I'll get moving:
>
> **Final planning summary**
>
> - Goal: write the store's three spoken conventions into `.sunshine/spec/` so later tasks follow them automatically; no product-code changes.
> - In scope: conventions 1 and 2 into `backend.md`, convention 3 into `system.md`; the code as it stands is the source of truth (product and order endpoints, `src/db.js`, `src/server.js`).
> - Out of scope: no product-code changes (`src/`, `package.json` untouched); don't change the conventions themselves, only record them; no design-first (no `doc/`, straight to development).
> - Key decisions (all confirmed): a light task, PRD only, no design.md/implement.md; no parent/child split; the port rule goes in `system.md` (a global principle), not `backend.md`.
> - Risks / deferred items: the current branch `main` is also the `base_branch`, so archiving will refuse — cut a feature branch and `set-branch` before archiving.
> - Artifact status: `prd.md` is written (with the Q&A record above); `implement.jsonl`/`check.jsonl` have 2 real entries each, `task validate` passes; the three spec lines are written by implement after `task start`.
>
> Once you approve this plan, I'll write the PRD first; writing the spec is itself this task's implementation, so the three lines go in after approval and `task start`. OK?
>
> **You:** Approved.
>
> **Claude Code:** The PRD is on disk, zero product code, not started. Confirm, and I'll start the task, write the spec, and verify it against the code once you approve.
>
> **You:** Approved.
>
> **Claude Code:** Starting the task, dispatching the `sunshine-implement` subagent to write the three conventions into spec; implementation done, dispatching the `sunshine-check` subagent to verify each one against the code. Here is what actually landed (line numbers verified):

```markdown
## backend.md
- 接口统一返回 `{code, data}`。
- 价格统一用“分”存库，展示时除以 100。
```

```markdown
## system.md
- 服务端口固定 3000。
```

The check results, all true, no issues, no fixes: convention 1 — `grep "res.json" src/` finds 5 places, all `{ code: 0, data: ... }` or `{ code: 404, data: null }`; convention 2 — `db.js` holds the three prices as 8900/29900/129900 (cents), `products.js` divides by 100 in two places (`price: p.price / 100`), and the order `totalPrice` multiplies cents; convention 3 — `server.js` has `const PORT = 3000;` and `app.listen(PORT, ...)`. `node --check` passes all four files, and the product code is untouched (`git diff --stat` is empty). A live run confirmed it too: the server log says "商城服务运行在端口 3000" (the store service runs on port 3000), `GET /products` returns the three products (prices 89/299/1299), and `POST /orders {"productId":2,"quantity":3}` returns `{"code":0,"data":{"id":1,"productId":2,"quantity":3,"totalPrice":89700}}` — 29900×3, cents arithmetic as agreed.

> **Claude Code:** Step 3.3, spec write-back: the spec is this task's deliverable, and there is no new knowledge beyond the three conventions — no further spec updates. Here's the commit plan — reply "OK" and I'll run it:
>
> ```text
> Suggested commits (in order):
>   1. docs: 把三条口头约定写进规范
>      - .claude/ (Sunshine init: Claude Code platform config)
>      - .cursor/ (Sunshine init: Cursor platform config)
>      - .sunshine/ (project skeleton + task artifacts + the three conventions in spec)
> Unrecognized dirty files (in no commit — please confirm include or exclude):
>   - None
> ```
>
> **You:** OK.
>
> **Claude Code:** Commit done: `0476a72 docs: 把三条口头约定写进规范` (122 files), working tree clean. Run `/finish-work` to archive and record the log — shall I run it now?
>
> **You:** Go ahead.

The first archive attempt was refused: the current branch `main` is also the `base_branch`, and a PR cannot point at its own branch. I cut `docs/old-shop-conventions`, ran `set-branch` for the task, and re-archived — through. `finish-work` archived the task and recorded the session log:

```sh
git log --oneline
# f0c95ed chore: record journal
# b8fcd1a chore(task): archive 11-13-old-shop-conventions
# 0476a72 docs: 把三条口头约定写进规范
# 998efb7 chore: 初始提交：Express 商城（商品/订单接口）
```

One ordering detail: the work commit `0476a72` was made on `main`, and the feature branch was only cut before archiving, so `main` holds just the first two commits and the archive and journal commits live only on `docs/old-shop-conventions`.

> **Claude Code:** Those two blocks are what landed. The task ran its normal course and is archived:

```text
[2026-11] - 1 个任务
  - 11-13-old-shop-conventions/
```

From now on the conventions no longer live only in senior engineers' heads: the agent on a new task reads the spec, and the conventions survive people leaving. The acceptance bar for adopting an existing project is one line — **after `init`, `git status` shows no modified old files, and spec has one more convention that used to live only in memory**.

### 12.3 Delivering a product feature: one task across product, API, data, and UI

When one task spans four layers, the usual failure is changing one layer and missing another — Sunshine's answer is one task: what changes in all four layers goes into the same PRD, the change order goes through a channel, and acceptance checks against the same API contract. The demo is the same web store that has run for two years, and the task is making the delivery address mandatory at checkout: the checkout page (UI) gets an address field, the order endpoint (API) returns 400 without one, and the order record (data) gains an address field.

State the requirement to Claude Code (same for Codex and Cursor):

Example prompt (an example, not a record of a real session):

> I need to deliver a product feature: the delivery address is mandatory at checkout, one task across the product, API, data, and UI layers (the UI is public/checkout.html, the API is src/routes/orders.js, the data is src/db.js). Please follow the Sunshine workflow: ask me first to pin down the unclear validation rules (the error code when the address is missing) and the order to change the four layers; once confirmed, create the task, write the PRD, then implement.

Here is how the session continues (the conversation is an example; the commands and outputs are real values from runs):

> **Claude Code:** One question first — it decides how this is built: what does the backend return when the address is missing — a 400 with an error code, or a 200 with an error message?
>
> **You:** 400, with the error code ADDRESS_REQUIRED.
>
> **Claude Code:** Next: one text box for the address, or split by province/city/district?
>
> **You:** Just one text box, `name="address"`.
>
> **Claude Code:** Next: in what order do the four layers change?
>
> **You:** API first, then UI — put the order in the channel.
>
> **Claude Code:** Next: what does the address field in the order record store?
>
> **You:** The raw string exactly as submitted.
>
> **Claude Code:** Last one: what counts as done?
>
> **You:** Two curls (400 without an address, 200 with one, the order carrying the address) plus a `name="address"` field on the checkout page; the task is archived as usual.
>
> **Claude Code:** Good, that's all the questions. Here's the final planning summary — once you approve it, I'll get moving:
>
> **Final planning summary**
>
> - Goal: make the delivery address mandatory at checkout, one task across the product, API, data, and UI layers.
> - In scope: an address field on the checkout page; `POST /orders` without an address returns 400 (ADDRESS_REQUIRED); the order record gains an address field; the PRD rule.
> - Out of scope: the product endpoints and price calculation, port 3000, coupons, login/auth, design-first.
> - Key decisions (all confirmed): a missing address returns 400 + ADDRESS_REQUIRED (not a 200 with an error message); a single text box, `name="address"`; API first, then UI, with the order going through the channel; the address is stored as the raw string.
> - Risks / deferred items: if no worker replies, the channel wait will time out (normal — this section only coordinates as far as sending the messages; spawning a real worker needs a logged-in claude/codex on this machine); the empty/blank-address edge was not asked in the Q&A, so the implementation handles it under the "mandatory" reading.
> - Artifact status: a light task, `prd.md` only; `implement.jsonl`/`check.jsonl` have 1 entry each (only the `.sunshine/spec/backend.md` contract, no code files), `task validate` passes; zero product code, no `task start` yet.
>
> Once you approve this plan, I'll write the PRD and the context list — still no product code, no start. OK?
>
> **You:** Approved.
>
> **Claude Code:** The PRD and the context list are on disk, zero product code, not started. Confirm, and I'll start the task and begin implementing once you approve.
>
> **You:** Approved.
>
> **Claude Code:** Starting the task, dispatching the `sunshine-implement` subagent. The four layers change API first, then UI, and I put the order in the channel — two messages:

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

(In English: "1. Change the API first: POST /orders without an address returns 400, error code ADDRESS_REQUIRED"; "2. Once the API is done, change the UI: add an address field to the checkout page, and the order can't be submitted without it.")

The channel is bound to the task, so the task path shows in the messages; these two are numbered coordination messages, not casual notes. Waiting 10 seconds for a worker's reply and getting a timeout is normal — this section only coordinates as far as sending the messages, and spawning a real worker needs a logged-in claude / codex on this machine (section 10):

```sh
time sunshine channel wait s3-cross --from impl --kind done --timeout 10s
# 错误：超时：等待 channel 's3-cross' 超时（10s），仍在等待：impl（上游退出码 124）
# EXIT:1
# real  0m10.013s
```

Implementation done, dispatching the `sunshine-check` subagent. Here is the real run result: ordering without an address returns 400, ordering with one succeeds, and the order carries the address:

```sh
curl -s -X POST localhost:3000/api/orders -H 'Content-Type: application/json' -d '{"items":[{"id":1,"qty":1}]}' -w "\nHTTP %{http_code}\n"
# {"code":"ADDRESS_REQUIRED","data":null}
# HTTP 400

curl -s -X POST localhost:3000/api/orders -H 'Content-Type: application/json' -d '{"items":[{"id":1,"qty":1}],"address":"北京市朝阳区建国路 1 号"}' -w "\nHTTP %{http_code}\n"
# {"code":"OK","data":{"id":1,"items":[{"id":1,"qty":1}],"address":"北京市朝阳区建国路 1 号","status":"待付款"}}
# HTTP 200
```

Also verified: `GET /api/products` still returns the prices 8900/29900/129900 (cents); the checkout page has a `name="address"` field; an empty string or a blank address also returns 400 `ADDRESS_REQUIRED` — the Q&A never asked that one separately, so the implementation handled it under the "mandatory" reading and wrote it into the spec.

> **Claude Code:** Step 3.3, spec write-back: `backend.md` gains a section, "下单必填收货地址" (Delivery address mandatory at checkout) — `POST /api/orders` without an `address` (missing, empty, or blank) returns 400, error code `ADDRESS_REQUIRED`, `data` is `null`; the order record carries an `address` field holding the raw string as submitted. Here's the commit plan — reply "OK" and I'll run it:
>
> ```text
> Suggested commits (in order):
>   1. 下单必填收货地址：结算页加地址输入框，缺地址返回 400（ADDRESS_REQUIRED）
>      - public/checkout.html
>      - src/routes/orders.js
>      - .sunshine/spec/backend.md
> Unrecognized dirty files (in no commit — please confirm include or exclude):
>   - .sunshine/.current-task
>   - .sunshine/tasks/11-13-下单必填收货地址/
> ```
>
> **You:** OK.
>
> **Claude Code:** Commit done: `da27e80 下单必填收货地址：结算页加地址输入框，缺地址返回 400（ADDRESS_REQUIRED）` (3 files). Run `/finish-work` to archive and record the log — shall I run it now?
>
> **You:** Go ahead.

Before archiving I cut a feature branch first (`main` is also the `base_branch`, so archiving directly would be refused): `git checkout -b feature/address-required` + `task set-branch`. `finish-work` archived the task and recorded the session log:

```sh
git log --oneline
# c229570 chore: record journal
# c956c40 chore(task): archive 11-13-下单必填收货地址
# da27e80 下单必填收货地址：结算页加地址输入框，缺地址返回 400（ADDRESS_REQUIRED）
# ed97a6e 12.2 商城 baseline：Express 商城 + 约定进 spec
```

The checkout page has the address field too, and the task ran its normal course and is archived:

```text
[2026-11] - 1 个任务
  - 11-13-下单必填收货地址/
```

The acceptance bar for a cross-layer task is in the same list: the files in all four layers done, and the one spec for acceptance verified line by line — whichever layer is missed shows up on the list.

### 12.4 Refactoring a legacy module: pin the behavior first, then change the code

The code runs, but it is no longer safe to change — the first step of a refactor is not to change anything, but to pin down what the code currently does. The demo module is the store's price function — the cart and order totals both go through it — with all the discount rules hard-coded in `if`s:

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

(In English: "Legacy module: it runs, but the discount rules are hard-coded in ifs — every change is a gamble.")

State the requirement to Claude Code (same for Codex and Cursor):

Example prompt (an example, not a record of a real session):

> I need to refactor a legacy module: the store's price function Price, with all the discount rules hard-coded in `if`s — the cart and order totals go through it, and the code runs, but it is no longer safe to change. Please follow the Sunshine workflow (the TDD template this time): ask me first to confirm the behavior to pin down (the expected prices for each quantity tier and VIP) and the boundaries of the refactor; once confirmed, create the task, write the PRD, then follow TDD: write characterization tests to pin the current behavior first, then refactor.

Here is how the session continues (the conversation is an example; the commands and outputs are real values from runs):

> **Claude Code:** One question first: which cases should I pin down — the expected prices for each quantity tier, VIP and non-VIP?
>
> **You:** Pin three cases: 1 item non-VIP 100, 10 items non-VIP 900, 10 items VIP 855.
>
> **Claude Code:** Good, one more: where does the refactor stop — only pull the discount rules out, with `Price`'s signature and return value unchanged, right?
>
> **You:** Right, only the discount rules; the signature stays, and behavior must not change.
>
> **Claude Code:** Good, that's all the questions. Here's the final planning summary — once you approve it, I'll get moving:
>
> **Final planning summary**
>
> - Goal: pull the discount rules in `Price` out of the `if`s into one place, so changing a rule means changing one place; behavior must not change by a single cent.
> - In scope: the discount rules in `calc.go` become constants plus `applyBulk` / `applyVIP`, with `Price` only composing them; a new `calc_test.go` pins the three cases; the cart (`CartTotal`) and order totals (`OrderTotal`) keep going through `Price`, no caller changes.
> - Out of scope: no signature or return-value changes; no changes to the discount values or the order they stack (bulk first, then VIP); no new discount tiers, no performance work.
> - Key decisions (all confirmed): write characterization tests to pin the current behavior first (all green is the baseline), then refactor; run the same tests after the refactor — still all green.
> - Risks / deferred items: none. The integer division order matches the original implementation (multiply first, then divide), and the three cases cover bulk and the VIP stack.
> - Artifact status: `prd.md` is written; `implement.jsonl`/`check.jsonl` have 1 entry each (`.sunshine/spec/backend.md`), `validate` passes; a light task, no `design.md`/`implement.md`.
>
> Once you approve this plan, I'll write the PRD and the context list — still no product code, no start. OK?
>
> **You:** Approved.
>
> **Claude Code:** The PRD and the context list are on disk, zero product code, not started. Confirm, and I'll switch to the TDD template, start the task, and begin implementing once you approve.
>
> **You:** Approved.
>
> **Claude Code:** The workflow is on the TDD template, the task is created and started. Writing the characterization tests to pin the three cases first:

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

Then write characterization tests — not tests for behavior you want, but tests that pin the existing behavior as it is:

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

Let me show you the run — all green. All green is the baseline — this step changed no business code at all, it only proves "the current behavior is exactly these three cases". The first run is green in the real run: the old code already returns exactly those three values, so characterization tests can only pin the existing behavior — there is no red run to write about:

```sh
go test ./...
# ok  	shop	0.003s
```

Only then does the refactor happen: discount rules become constants and two small functions, and `Price` only composes them:

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

(In English: "Discount rules change here and nowhere else; Price only composes them.")

Run the same tests once more after the refactor — still all green. The behavior did not change, so the refactor stands. The red light in the TDD workflow is for new behavior; a pure refactor runs the other way around: **write characterization tests to pin the behavior (green), then refactor (still green)**:

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

(The first run after the refactor is `0.011s`, which includes compilation; re-running with `-count=1` gives `0.003s`, matching the pre-refactor run.) Dispatching the `sunshine-check` subagent: `func Price(qty int, vip bool) int` is unchanged, `cart.go` and `order.go` still go through `Price`, `go vet` exits 0, `go fmt` is clean — nothing to fix.

> **Claude Code:** Step 3.3, spec write-back: the new convention goes into `backend.md`, so the next rule change starts from the test table:

```markdown
## 价格计算

- 折扣规则集中在 calc.go 的常量与 applyBulk / applyVIP，Price 只做组合；改规则先改 calc_test.go 的用例表。
```

(In English: "Price calculation: discount rules live in the constants and applyBulk / applyVIP in calc.go; Price only composes them. Change a rule by changing the case table in calc_test.go first.")

> **Claude Code:** Here's the commit plan — reply "OK" and I'll run it:
>
> ```text
> Suggested commits (in order):
>   1. refactor: 把 Price 的折扣规则抽成常量与小函数
>      - calc.go
>      - calc_test.go
>      - .sunshine/spec/backend.md
> Unrecognized dirty files (in no commit — please confirm include or exclude):
>   - .sunshine/workflow.md (the TDD template switch — back to baseline once the workflow is switched back to native)
>   - .sunshine/.template-hashes.json (the template switch's side record, same)
>   - .sunshine/tasks/11-13-重构价格计算/ (task artifacts, carried by the archive commit)
>   - .sunshine/.current-task (the current-task pointer, cleaned up by archiving)
> ```
>
> **You:** OK.
>
> **Claude Code:** Commit done: `f73b1e3 refactor: 把 Price 的折扣规则抽成常量与小函数` (3 files, +46/−5). Run `/finish-work` to archive and record the log — shall I run it now?
>
> **You:** Go ahead.

`finish-work` archived the task and recorded the session log:

```sh
git log --oneline -5
# 2dc676c chore: record journal
# 50c33dc chore(task): archive 11-13-重构价格计算
# f73b1e3 refactor: 把 Price 的折扣规则抽成常量与小函数
# e293a24 chore: sunshine init
# 3fb2275 chore: 初始化商城价格计算场景
```

Archive the task as usual:

```text
[2026-11] - 1 个任务
  - 11-13-重构价格计算/
```

Then switch the workflow back to native — switching directly fails (the `workflow.md` has local changes), so add `--force`:

```sh
sunshine workflow --template native
# 错误：.sunshine/workflow.md 已有本地修改，请加 --force 覆盖，或用 --create-new 写 .sunshine/workflow.md.new
sunshine workflow --template native --force
#   ✓ 已把 .sunshine/workflow.md 换成 "native"
```

After switching back, `workflow.md` and `.template-hashes.json` are both back to baseline.

### 12.5 Fixing a bug that keeps coming back: find the last session first

When the same class of problem appears again and again, the most expensive part is investigating it from zero every time. The bug in this section really happened: the store project's codex worker failing to connect with a 401 — once on November 9, again in the early hours of November 10, and once more today.

State the requirement to Claude Code (same for Codex and Cursor):

Example prompt (an example, not a record of a real session):

> I need to fix a bug that keeps coming back: the store project's codex worker failing to connect with a 401 — once on November 9, again in the early hours of November 10, and once more today. Please follow the Sunshine workflow: use mem to find the last related session first, then ask me to confirm whether this is the same class of problem and what the reproduction steps are; once confirmed, create the task, write the PRD, and write the lesson into the spec after the fix.

Here is how the session continues (the conversation is an example; the commands and outputs are real values from runs):

> **Claude Code:** I checked `mem` first:

```sh
sunshine mem search 401 --global
# 范围：全部项目  平台=all  关键词="401"
#
# [codex   ] 2026-11-10 05:05  01a1c3d8-457  /tmp/scenes/s5-bug  相关度=1.000  命中=1（用户=1，助手=0）  轮次=1
#     [用户] 线上 codex worker 又连不上了，之前报过 401，帮我查一下这次的原因
#
# 共 1 个会话
```

> **Claude Code:** The last related session is the dispatch from November 10, 05:05, whose message to the worker already mentions 401. One question first: is this the same class of problem — the store project's codex worker failing to connect with a 401?
>
> **You:** Same class — still 401 today.
>
> **Claude Code:** Good, one more: how do I reproduce it — on this machine, or online?
>
> **You:** Reproduce it on this machine, codex isn't logged in.
>
> **Claude Code:** Good, that's all the questions. Here's the final planning summary — once you approve it, I'll get moving:
>
> **Final planning summary**
>
> - Goal: reproduce the 401, locate the root cause, and write the lesson into system.md.
> - In scope: build a channel on this machine, send the original message, spawn the codex worker, confirm the 401 in worker.log, confirm the root cause, write the lesson.
> - Out of scope: no changes to the store's business code; don't log in to codex for the user; don't fix codex or the channel itself; don't handle spawn failures other than 401.
> - Key decisions (all confirmed): the reproduction only verifies as far as the 401 appearing; the login is left for the user to handle.
> - Risks / deferred items: none (the 401 has already appeared three times, and the root cause points to not being logged in).
> - Artifact status: the PRD plus the context list (system.md, backend.md), zero product code.
>
> Once you approve this plan, I'll write the PRD (with the session id 01a1c3d8 in the description) and the context list — still no product code, no start. OK?
>
> **You:** Approved.
>
> **Claude Code:** The PRD and the context list are on disk (`implement.jsonl`/`check.jsonl` have 2 entries each, `task validate` passes), zero product code, not started. Confirm, and I'll start the task and begin the reproduction once you approve.
>
> **You:** Approved.
>
> **Claude Code:** Starting the task, dispatching the `sunshine-implement` subagent to reproduce. Reproducing the dispatch once on this machine: a channel is created, the same message goes to the worker, and the worker starts — then fails in its log (codex is not logged in on this machine, the same precondition section 10 states; `ls ~/.codex/auth.json` → `No such file or directory`):

```sh
sunshine channel create codex-401 --type chat --task .sunshine/tasks/11-13-codex-worker-401
# 已创建 channel 'codex-401'（chat）位于 /home/hatch/.sunshine/channels/-tmp-s5-repro/codex-401
sunshine channel spawn codex-401 --provider codex --as worker
# {"pid": 7235, "log": "/home/hatch/.sunshine/channels/-tmp-s5-repro/codex-401/worker.log", "worker": "worker"}
sunshine channel send codex-401 --to worker "线上 codex worker 又连不上了，之前报过 401，帮我查一下这次的原因"
# {"by":"main","kind":"message","origin":"cli","seq":3,"text":"线上 codex worker 又连不上了，之前报过 401，帮我查一下这次的原因","to":"worker","ts":"2026-11-13T04:01:49.517881231Z"}
```

The 401 line in worker.log (verbatim, first seen 2026-11-13T04:01:49.049897Z), appearing 7 times:

```text
failed to connect to websocket: HTTP error: 401 Unauthorized, url: wss://api.openai.com/v1/responses
```

And the root-cause line (verbatim):

```text
unexpected status 401 Unauthorized: Missing bearer or basic authentication in header, url: wss://api.openai.com/v1/responses
```

The worker then retried on its own, `Reconnecting... 2/5` through `5/5` — which is exactly the evidence for "don't spawn again and again". Waiting for the worker's reply returned a terminal error event (seq 16), and then the worker was stopped:

```sh
sunshine channel wait codex-401 --from worker --kind done,error --timeout 2m
# {"by":"worker","kind":"error","message":"unexpected status 401 Unauthorized: Missing bearer or basic authentication in header, url: https://api.openai.com/v1/responses, cf-ray: a59ae10cfee192d0-DFW, request id: req_856bb946182d43f9a9620b912c13e4cc","origin":"worker","seq":16,"ts":"2026-11-13T04:02:06.752406346Z"}
sunshine channel kill codex-401 --as worker
```

(Note: the positional form `kill codex-401 worker` errors with `accepts 1 arg(s), received 2` — use `--as worker`.)

> **Claude Code:** Dispatching the `sunshine-check` subagent — all green, nothing to fix: the 401 log line hit 7 times exactly; the root cause is codex not being logged in (`~/.codex/auth.json` still does not exist); the lesson sits at system.md:17; the business code is untouched (`git diff --stat -- server.js package.json` is empty); searching `mem` for 401 now returns 2 sessions — the new one from this reproduction, `01a1d311-99c`, plus the original `01a1c3d8-457`. The lesson is in spec:

```markdown
## 多 Agent 派发

- 派发 codex worker 前，先确认本机 codex 已登录（`~/.codex/auth.json` 存在；未登录时 worker 拿不到 bearer token，启动即 401）。
- spawn 失败时先看 worker.log，不要反复重派：`failed to connect to websocket: HTTP error: 401 Unauthorized, url: wss://api.openai.com/v1/responses` 即未登录（根因：`Missing bearer or basic authentication in header`），重派只会重复报 401。
- 复现记录：2026-11-13，商城项目派 codex worker 连不上报 401（此前 11 月 9 日、11 月 10 日凌晨已出现两次）；mem 搜 401 命中会话 01a1c3d8（2026-11-10 05:05）。
```

(In English: "Multi-agent dispatch: before spawning a codex worker, confirm codex is logged in on this machine (`~/.codex/auth.json` exists; if it is not logged in, the worker gets no bearer token and fails with 401 at startup). When a spawn fails, read worker.log first and don't spawn again and again: the websocket 401 line means not logged in (root cause: `Missing bearer or basic authentication in header`), and re-spawning only repeats the 401. Reproduction record: 2026-11-13, the store project spawned a codex worker that could not connect, 401 (twice before, on November 9 and in the early hours of November 10); searching `mem` for 401 hits session 01a1c3d8 (2026-11-10 05:05).")

One boundary found by actually running this, worth remembering: **`mem` searches the words in dialogue turns (user / assistant); it does not search error events**. The session from the early hours of November 10 (`01a1c376`, in `/tmp/h-test/chanproj`) had a worker fail with the same 401, but its 401 only appears in an error event, not in any dialogue turn — so searching `401` does not find it. What you search for is the sentence that was sent to the worker at the time. This run is findable because the message sent to the worker already says 401. Grep of the session files confirmed it: `01a1c376`'s 401 only appears in the error field of an `event_msg/task_complete`, while `01a1c3d8` has the user's own words. The November 9 case is the same: `01a1bfa4-3c6`'s session file does contain the same 401 error line, `01a1bfa3`'s does not.

Once the last session confirms it is the same class of problem, create the task — with the session id (01a1c3d8) in the description, so the agent does not have to ask "what happened last time" all over again. When the task is done, the lesson goes into spec — otherwise it will happen the same way next time, which is the actual fix for a recurring bug.

> **Claude Code:** Step 3.3, spec write-back: no other new knowledge beyond this lesson, backend.md needs no change. One commit in the plan — reply "OK" and I'll run it:
>
> ```text
> Suggested commits (in order):
>   1. docs: codex worker 401 教训写进 system.md（多 Agent 派发）
>      - .sunshine/spec/system.md
> Unrecognized dirty files (in no commit — please confirm include or exclude):
>   - .sunshine/.current-task (the task-session pointer, cleaned up by archiving)
>   - .sunshine/tasks/11-13-codex-worker-401/ (the task directory, managed by finish-work's archive commit)
> ```
>
> **You:** OK.
>
> **Claude Code:** Commit done: `9a4b2bd docs: codex worker 401 教训写进 system.md（多 Agent 派发）` (1 file, 6 lines added). Only a spec file changed, so the prefix is `docs:`, not `fix:`. Run `/finish-work` to archive and record the log — shall I run it now?
>
> **You:** Go ahead.

Before archiving I cut a feature branch first (`master` is also the `base_branch`, so archiving directly would be refused): `git checkout -b fix/codex-worker-401` + `task set-branch`. `finish-work` archived the task and recorded the session log:

```sh
git log --oneline
# c29bd5e chore: record journal
# 38775f6 chore(task): archive 11-13-codex-worker-401
# 9a4b2bd docs: codex worker 401 教训写进 system.md（多 Agent 派发）
# bfe6581 chore: sunshine init（后端）
# 6ed4c18 chore: 商城后端初始代码
```

Archive the task as usual:

```text
[2026-11] - 1 个任务
  - 11-13-codex-worker-401/
```

Next time a 401 shows up, the agent reads the spec and checks the login first, instead of spawning another worker into the same wall.

### 12.6 Design-first product work: requirements → design documents → split → code

This scenario starts from zero: a brand-new web store project, done design-first. The difference from 12.1 is the mode — 12.1 is a new project built by direct development; this one is a new project designed first and coded second: questions first, design documents on disk, a review that passes, a split, and only then hands on keyboards one sub-task at a time. Designers and developers share one workflow, but they enter and exit at different times — each part below says who is on stage.

**[Designer enters]**

State the requirement to Claude Code (same for Codex and Cursor):

Example prompt (an example, not a record of a real session):

> I want to build a web store with coupons from zero, design-first. Please follow the Sunshine workflow: ask me first to pin down the unclear product logic and technical decisions; once confirmed, create the task, write the PRD, and land the requirements list, the outline design, and the system prototype in doc/; after the review passes, split it into sub-tasks per requirement and then implement. The store needs a product list, adding to cart, checkout, using a coupon at checkout for a discount, and a coupon list page showing each coupon's threshold, amount, status, and expiry. The tech stack isn't decided yet — help me pin down the requirements first.

Here is the session (the conversation is an example; the commands and outputs are real values from runs):

Design-first does not mean asking every question up front and then producing the whole design package in one go. It runs per requirement, one loop at a time: for each requirement, a round of questions, then the requirements list is updated on disk, then a prototype sub-agent builds that requirement's page. The loop stops only when the designer and the user agree the current batch is enough, and only then does it move to review and split. The scope is not fixed in advance — the loop starts from the requirements already in the opening prompt (a starting batch, not a fixed scope); shared constraints and shared data are fixed the first time they are needed; the boundary of what is out of scope is set as the work goes.

#### Round 1: R-001 product list

> **Claude Code:** What product data do we use? A, three products — wireless mouse 8900 fen / stock 120, mechanical keyboard 29900 fen / stock 45, 27-inch monitor 129900 fen / stock 20; B, add more products.
>
> **You:** A, as given.

> **Claude Code:** How do we fix prices, APIs, and the port? A, prices stored in "fen", all APIs return `{code, data}`, port 3000; B, decide each separately later.
>
> **You:** A, these three are shared constraints — fix them in this round.

Requirements list updated on disk: 45 lines, with a new R-001 line (the product list page shows all products: name, price, stock; GET /api/products returns the three products).

Prototype dispatch: three inputs — the requirements list (current state) + the spec's frontend style constraints (`.sunshine/spec/frontend.md`) + the existing prototype directory `doc/prototype/`. The sub-agent reported: 1 new page. 商品列表页.html 72 lines; mock.js 8 lines (3 products); index.html 38 lines; the template placeholder 示例页.html deleted.

#### Round 2: R-002 adding to cart

> **Claude Code:** Where does the cart live? A, in the browser's localStorage, no backend API; B, stored on the backend.
>
> **You:** A, localStorage.

Requirements list updated on disk: 46 lines, with a new R-002 line (click "add to cart" on the product list page; the cart page shows the added products, quantities, and total; the cart lives in the browser's localStorage, no backend API).

Prototype dispatch: same three inputs. The sub-agent reported: 1 new page. 购物车页.html 145 lines; index.html 39 lines; mock.js unchanged. The cart's localStorage key is `sunshine_cart`; the demo pre-fills 1 keyboard + 2 mice, totalling 47700 fen (node check: 29900 + 8900 × 2 = 47700).

#### Round 3: R-003 checkout

> **Claude Code:** Is the shipping address required? What happens without one? A, required — a missing address returns 400 ADDRESS_REQUIRED; B, optional.
>
> **You:** A, required.

Requirements list updated on disk: 47 lines, with a new R-003 line (fill in the shipping address on the checkout page and submit the order; the address is required, a missing address returns 400 ADDRESS_REQUIRED; an order with an address succeeds).

Prototype dispatch: same three inputs. The sub-agent reported: 1 new page. 结算页.html 132 lines; index.html 40 lines; mock.js unchanged. An empty address shows "请填写收货地址"; submitting with an address shows "下单成功，订单状态：待付款". No coupon area yet — only a source comment reserving the R-004 spot.

#### Round 4: R-004 using a coupon at checkout

> **Claude Code:** Coupon validation order and error codes? A, not found → used → expired → below threshold, with COUPON_INVALID / COUPON_USED / COUPON_EXPIRED / COUPON_THRESHOLD; B, only usable versus not usable.
>
> **You:** A, the four-step order.

> **Claude Code:** How is the coupon code submitted, and who validates it? A, type the code on the checkout page and submit it with the order; the backend validates and deducts; B, claim the coupon first, then order.
>
> **You:** A, submit the code with the order.

The three coupons were fixed in this round, the first time they were needed: MAN100JIAN10 (10000 off 1000, unused, 2026-12-31), MAN200JIAN30 (20000 off 3000, used), NEW5 (new-user 5-yuan coupon, expired, 2026-10-01).

Requirements list updated on disk: 48 lines, with a new R-004 line (type the coupon code on the checkout page, 100 off 10; the coupon must be unused and unexpired and meet the threshold, otherwise the order fails with a reason).

Prototype dispatch: same three inputs, explicitly sharing the checkout page with R-003 (add the coupon area, do not rebuild the page). The sub-agent reported: 0 new pages. 结算页.html 215 lines (coupon code input on line 49, the "满 100 元减 10 元" threshold hint on line 50, validation logic on lines 121–189); mock.js 17 lines (3 products + 3 coupons); index.html unchanged. A node check of all six cases passed (1 keyboard pays 28900, 2 mice pay 16800, 1 mouse is below threshold).

#### Round 5: R-005 coupon list

> **Claude Code:** What does the coupon list show? A, coupon name, threshold, amount, status, expiry; B, only name and amount.
>
> **You:** A, all five fields.

Requirements list updated on disk: 49 lines, with a new R-005 line (the coupon page shows all coupons: name, threshold, amount, status, expiry; GET /api/coupons returns the three coupons).

Prototype dispatch: same three inputs. The sub-agent reported: 1 new page. 优惠券页.html 86 lines; index.html 41 lines; mock.js unchanged. Final count: 4 content pages + the navigation entry.

#### Closing questions

> **Claude Code:** What is explicitly out of scope? A, no payments, no login, no coupon issuing / claiming / redemption backend, no automatic expiry cleanup, no database, no cache; B, build all of it.
>
> **You:** A, all of that is out — write it into the prd.md "out of scope" section.

Designer and user agreed the current batch was enough; the loop ends, and the work moves to review and split.

All eight dimensions (users and scenarios, feature boundaries, data, performance, security, compatibility, acceptance, out of scope) are covered by now: performance is a single-machine demo with no special concurrency needs, security is exact-match coupon codes, compatibility is prices in fen, APIs in `{code, data}`, and port 3000 — those three were settled in the answers and in the requirements list's non-functional section, so no separate questions. No blocking open questions, so on to design.

The outline design was written after the loop ended — the opening prompt asked for it by name (generated on demand, not a mandatory checklist item). The final design package lands in the project's `doc/`, not `.sunshine`:

```text
doc/design/需求列表.md        # 49 lines: R-001–R-005 (lines 23–27), open items (lines 39–44, empty), change log (lines 45–49)
doc/design/概要设计.md        # 78 lines: goals and scope, module split, key flows, interfaces (lines 38–40), data, tech selection (lines 55–64, [待定] on lines 60–63), acceptance criteria, risks
doc/prototype/index.html       # 41 lines: navigation entry listing the 4 pages and their requirements
doc/prototype/mock.js          # 17 lines: shared mock, 3 products and 3 coupons
doc/prototype/商品列表页.html  # 72 lines: R-001
doc/prototype/购物车页.html    # 145 lines: R-002
doc/prototype/结算页.html      # 215 lines: R-003 + R-004 (coupon code input on line 49, the "满 100 元减 10 元" threshold hint on line 50, validation logic on lines 121–189)
doc/prototype/优惠券页.html    # 86 lines: R-005
```

Five requirements map to four prototype pages — R-003 checkout and R-004 using a coupon share the checkout page (R-004 is the coupon-code area on that page); every other requirement gets its own page. The prototype was built by dispatching a `sunshine-prototype` sub-agent once per round, with the same three inputs (requirements list as it stood + the spec's frontend style constraints + the existing prototype directory), and each report gave only page count, mock rows, and paths; the template `doc/prototype/示例页.html` that `init` ships is a placeholder, not a real page, so it was deleted in round 1 and `index.html` rewritten for the real pages.

The requirements list is one item per requirement, each pointing at its page in the system prototype (real file content, excerpt):

```text
| R-001 | 商品列表 | 商品列表页展示全部商品：名称、价格（元）、库存；GET /api/products 返回三件商品 | P0 | GET /api/products 返回三件商品：无线鼠标 8900 分 / 库存 120、机械键盘 29900 分 / 库存 45、27寸显示器 129900 分 / 库存 20 |
| R-002 | 加入购物车 | 商品列表页点“加入购物车”，购物车页展示已加商品、数量、合计；购物车放浏览器 localStorage，不设后端接口 | P0 | 购物车页能展示已加商品与合计；刷新后购物车还在 |
| R-003 | 下单结算 | 结算页填收货地址提交订单；收货地址必填，缺地址返回 400 ADDRESS_REQUIRED；带地址下单成功 | P0 | 缺地址下单返回 400 ADDRESS_REQUIRED；带地址下单成功，返回订单 id、items、address、status“待付款” |
| R-004 | 下单用优惠券 | 结算页输入券码，满 100 元减 10 元；券须未使用且未过期、且满足门槛，否则下单失败并提示原因 | P0 | 券码 NOPE 下单返回 400 COUPON_INVALID；MAN200JIAN30 返回 400 COUPON_USED；NEW5 返回 400 COUPON_EXPIRED；1 件机械键盘用 MAN100JIAN10 实付 28900 分；2 件无线鼠标用 MAN100JIAN10 实付 16800 分；1 件无线鼠标用 MAN100JIAN10 返回 400 COUPON_THRESHOLD |
| R-005 | 优惠券列表 | 优惠券页展示全部券：券名、门槛、面额、状态、有效期；GET /api/coupons 返回三条券 | P0 | GET /api/coupons 返回三条券：MAN100JIAN10 满100减10 未使用、MAN200JIAN30 满200减30 已使用、NEW5 新人5元券 已过期 |
```

The outline design's interface lines (real content):

```text
| GET /api/products | 无 | { code, data: products }，三件商品 | 返回全部商品，不分页 |
| POST /api/orders | { items, address, couponCode } | { code, data: order }，实付写在 data.total | 缺地址返回 400 ADDRESS_REQUIRED；券校验顺序：不存在 → 已使用 → 已过期 → 不满门槛；失败返回 400 + 错误码 COUPON_INVALID / COUPON_USED / COUPON_EXPIRED / COUPON_THRESHOLD |
| GET /api/coupons | 无 | { code, data: coupons }，三条券 | 返回全部券，不分页 |
```

The tech-selection section (outline design, lines 55–64) only writes constraints and open items and never forces a decision: constraints are prices in fen, APIs in `{code, data}`, port 3000, and a single-machine demo with no special concurrency needs; language, framework, database, and cache are all marked `[待定]` (lines 60–63), left for the backend-foundation sub-task's 1.1 questions. That is the v0.5.5 rule — when the backend foundation is big enough for its own sub-task, its selection items are marked `[待定]` in its PRD and its 1.1 answers them before any scaffolding.

---

**[Design review: designer and developer both on stage]**

The design package is reviewed before any split. The review is the 1.5 final planning summary — the whole planning package laid out for confirmation, and only then does anything `task start`:

> **Claude Code:** Final planning summary for confirmation: the goal is a coupon-carrying web store built from zero, design-first; the scope is all five requirements R-001–R-005; out of scope are payments, login, coupon issuing / claiming / redemption, automatic expiry cleanup, database, and cache; the key decisions are leaving the stack to the backend-foundation sub-task, keeping the cart in localStorage, requiring the address, and the four-step coupon validation order; the deliverables are prd.md, the requirements list (49 lines), the outline design (78 lines), a 4-page prototype plus mock, and 6 sub-tasks already created and validated, in order backend foundation → product list → adding to cart → checkout → using a coupon → coupon list. Once you confirm, I'll start them one by one. OK?
>
> **You:** OK, go ahead.

---

**[Split: 1.2 runs once]**

The split runs once, right after the review passes, creating all 6 sub-tasks in one pass; it starts nothing, and every sub-task is created in `planning`. The parent task is only created, never started (it has no implementation work of its own), and the split follows the requirements list (the main thread and the contract) and what is actually in `doc/`:

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

The planning products were committed as `b4bf106` (doc/ + the 7 task files) — the design package and the task structure land in one commit before any coding starts.

The backend-foundation sub-task goes first, and its 1.1 answers the `[待定]` items one question at a time:

> **Claude Code:** Which language? A, Node.js; B, Go.
>
> **You:** A, Node.js — the store demo is light, and one language front and back.
>
> **Claude Code:** Which framework? A, Express; B, raw http.
>
> **You:** A, Express.
>
> **Claude Code:** Where does the data live? A, in memory, no database; B, SQLite.
>
> **You:** A, in memory — the data is small, three products and three coupons, and the demo needs no persistence.
>
> **Claude Code:** Cache? A, none; B, add an in-memory cache layer.
>
> **You:** A, none — in-memory data is already the fastest layer there is.

The four answers go into that sub-task's prd.md and design.md, and the `[待定]` items clear; once the stack is fixed it does not change again, and the later sub-tasks reuse it. Each requirement sub-task's own 1.1 only briefly confirms its scope (the one requirement in its prd.md plus its acceptance criteria) and skips the full brainstorm, going straight to 1.4.

Each sub-task's implement.jsonl / check.jsonl lists only specs, research, and doc/ design products — never code files (the sub-agent reads those itself during implementation, so they need no advance registration); before `task start`, each jsonl needs at least one real entry, and `task validate` must pass. Dependencies go in the prd: the requirement sub-tasks start only after the backend foundation is archived; R-004 depends on R-003's order API; R-005 depends on R-004's coupon data.

---

**[Developers enter]**

One sub-task at a time: each runs the full implement → check → write back → commit → archive cycle before the next starts. Here is each sub-task's record (the conversation is an example; the commands and outputs are real values from runs).

### Backend foundation: stack choice and scaffolding

The backend-foundation sub-task goes first. Its implement.jsonl has 3 entries (outline design interfaces and data, the backend spec, this task's design.md), its check.jsonl has 2 (outline design acceptance criteria, this task's prd.md), and `task validate` passes before start. 2.1 implementation: `package.json` (single-line JSON, Express ^4.21.2, start is `node server.js`), `server.js` 18 lines (port 3000, a GET / health JSON, serves public/), `src/db.js` 21 lines (products / coupons / orders / nextOrderId=1), a root `.gitignore` (node_modules/), and package-lock.json generated automatically; `npm install` pulls in express 4.21.2, 68 packages in total.

Two inferences (the task data did not say): MAN200JIAN30's expiry is 2026-12-31 (same as MAN100JIAN10); NEW5's threshold is 0 and its amount 500 fen.

2.2 check:

```sh
curl -s -i http://localhost:3000/
# HTTP/1.1 200 OK
# {"code":0,"data":{"status":"ok"}} (health JSON)
curl -s http://localhost:3000/api/products
# Cannot GET /api/products (expected: no route mounted yet)
```

3.3 write back: the check ran and the answer was "no spec update" (the stack and the interfaces are already recorded in the task prd/design and the outline design; the team spec gains no new convention). Work commit `d4845de` → archive commit `bfe26d0` → journal commit `0d2d697` (session 1). Note: the `.gitignore` (containing only `node_modules/`) was created here — `init` does not generate a repo-root `.gitignore`, or `node_modules/` would land in the working tree.

### Product list

implement.jsonl has 3 entries (outline design interface GET /api/products, the product list prototype page, this task's prd.md), check.jsonl has 2 (outline design acceptance criteria, this task's prd.md), `task validate` passes. 2.1 implementation: `src/routes/products.js` 11 lines (GET /api/products), `server.js` 22 lines (+5, route mounted), `public/index.html` 103 lines (fetches /api/products, writes the cart to localStorage, Loading and error states).

2.2 check:

```sh
curl -s http://localhost:3000/api/products
# {"code":0,"data":[{"id":1,"name":"无线鼠标","price":8900,"stock":120},{"id":2,"name":"机械键盘","price":29900,"stock":45},{"id":3,"name":"27寸显示器","price":129900,"stock":20}]}
curl -s -o /dev/null -w "%{http_code} %{size_download} bytes" http://localhost:3000/index.html
# 200, 4143 bytes
curl -s -o /dev/null -w "%{http_code}" http://localhost:3000/
# 200
```

3.3 write back: the check ran and the answer was "no spec update" (the API follows the existing backend spec; no new pattern, no new decision). Work commit `3977045` → archive commit `b6fdb15` → journal commit `a6faa74` (session 2).

### Adding to cart

implement.jsonl has 2 entries (the cart prototype page, this task's prd.md), check.jsonl has 2 (the cart prototype page, this task's prd.md), `task validate` passes. 2.1 implementation: `public/cart.html` 156 lines (6680 bytes) — reads localStorage `sunshine_cart`, calls /api/products to resolve prices, quantities are editable with the total updating live, the cart survives a refresh, empty state, and a "go to checkout" link to checkout.html; the production page does not pre-fill demo data (the prototype stuffs 1 keyboard + 2 mice when it opens with no data), so no data means the empty state; it also drops products that are no longer listed and clamps out-of-range quantities.

2.2 check: the cart has no backend API — adding to the cart is a browser localStorage action that curl cannot click — so the check is page reachability + key-logic grep + a node calculation against the db.js data:

```sh
curl -s -o /dev/null -w "%{http_code} %{size_download} bytes" http://localhost:3000/cart.html
# 200, 6680 bytes (the page has localStorage in 4 places, fetch in 1)
node -e "console.log('total = ' + (29900 + 8900 * 2) + ' fen = ' + (29900 + 8900 * 2) / 100 + ' yuan')"
# total = 47700 fen = 477 yuan (1 mechanical keyboard + 2 wireless mice)
```

3.3 write back: the check ran and the answer was "no spec update". Work commit `1853417` → archive commit `faac8ce` → journal commit `ace811d` (session 3).

### Checkout

implement.jsonl has 3 entries (outline design interface POST /api/orders, the checkout prototype page, this task's prd.md), check.jsonl has 2 (outline design acceptance criteria, this task's prd.md), `task validate` passes. 2.1 implementation: `src/routes/orders.js` 40 lines (address check on line 13 — a missing or empty address returns 400 ADDRESS_REQUIRED; the id comes from `db.nextOrderId++`; the total is price × qty; status "待付款"; couponCode is accepted but not validated and not deducted), `server.js` 29 lines (adds the `express.json()` the scaffold lacked, plus the route mount), `public/checkout.html` 176 lines (address input on line 48, no coupon area yet — only the `<!-- R-004 券码输入区（预留位置） -->` comment; the cart is cleared after a successful order).

2.2 check:

```sh
curl -s -X POST http://localhost:3000/api/orders -H "Content-Type: application/json" -d '{"items":[{"id":2,"qty":1}]}'
# {"code":"ADDRESS_REQUIRED","data":null}, HTTP 400 (no address)
curl -s -X POST http://localhost:3000/api/orders -H "Content-Type: application/json" -d '{"items":[{"id":2,"qty":1}],"address":""}'
# {"code":"ADDRESS_REQUIRED","data":null}, HTTP 400 (an empty address counts as missing)
curl -s -X POST http://localhost:3000/api/orders -H "Content-Type: application/json" -d '{"items":[{"id":2,"qty":1}],"address":"测试地址"}'
# {"code":0,"data":{"id":1,"items":[{"id":2,"qty":1}],"address":"测试地址","couponCode":null,"total":29900,"status":"待付款"}}, HTTP 200
curl -s -o /dev/null -w "%{http_code} %{size_download} bytes" http://localhost:3000/checkout.html
# 200, 7241 bytes
```

3.3 write back: the check ran and the answer was "no spec update" (ADDRESS_REQUIRED is this task's contract, already in design.md and the outline design's interface line; it does not belong in the shared spec). Work commit `0d56a42` → archive commit `cc5b816` → journal commit `9f84869` (session 4). Note: db.js exports nextOrderId by value, and orders.js mutates the exported object with `db.nextOrderId++`, so the id keeps incrementing across requests.

### Using a coupon at checkout

implement.jsonl has 3 entries (outline design interface POST /api/orders, the four-step coupon validation, the checkout prototype page's coupon area, this task's prd.md), check.jsonl has 2 (outline design acceptance criteria, this task's prd.md), `task validate` passes. 2.1 implementation: `src/routes/orders.js` 64 lines (`checkCoupon` on line 8, called on line 42; the order is not found → COUPON_INVALID, status used → COUPON_USED, status expired or expires before today → COUPON_EXPIRED, subtotal below threshold → COUPON_THRESHOLD; a passing coupon deducts its discount; an empty or missing couponCode skips validation and deducts nothing; the order object gains a couponCode field), `public/checkout.html` 200 lines (coupon code input on line 49, the "满 100 元减 10 元" threshold hint on line 50). The coupons table (three coupons) was already in db.js from the backend foundation, so nothing was added there.

2.2 check, all six cases (the address is `测试地址` throughout):

```sh
curl -s -X POST http://localhost:3000/api/orders -H "Content-Type: application/json" -d '{"items":[{"id":2,"qty":1}],"address":"测试地址","couponCode":"NOPE"}'
# {"code":"COUPON_INVALID","data":null}, HTTP 400 (coupon code does not exist)
curl -s -X POST http://localhost:3000/api/orders -H "Content-Type: application/json" -d '{"items":[{"id":2,"qty":1}],"address":"测试地址","couponCode":"MAN200JIAN30"}'
# {"code":"COUPON_USED","data":null}, HTTP 400 (already used)
curl -s -X POST http://localhost:3000/api/orders -H "Content-Type: application/json" -d '{"items":[{"id":2,"qty":1}],"address":"测试地址","couponCode":"NEW5"}'
# {"code":"COUPON_EXPIRED","data":null}, HTTP 400 (expired)
curl -s -X POST http://localhost:3000/api/orders -H "Content-Type: application/json" -d '{"items":[{"id":2,"qty":1}],"address":"测试地址","couponCode":"MAN100JIAN10"}'
# {"code":0,"data":{"id":1,"items":[{"id":2,"qty":1}],"address":"测试地址","couponCode":"MAN100JIAN10","total":28900,"status":"待付款"}}, HTTP 200 (1 keyboard pays 28900)
curl -s -X POST http://localhost:3000/api/orders -H "Content-Type: application/json" -d '{"items":[{"id":1,"qty":2}],"address":"测试地址","couponCode":"MAN100JIAN10"}'
# {"code":0,"data":{"id":2,"items":[{"id":1,"qty":2}],"address":"测试地址","couponCode":"MAN100JIAN10","total":16800,"status":"待付款"}}, HTTP 200 (2 mice pay 16800)
curl -s -X POST http://localhost:3000/api/orders -H "Content-Type: application/json" -d '{"items":[{"id":1,"qty":1}],"address":"测试地址","couponCode":"MAN100JIAN10"}'
# {"code":"COUPON_THRESHOLD","data":null}, HTTP 400 (1 mouse is below the threshold)
```

3.3 write back: the check ran and the answer was "no spec update" (the validation order is a business rule, already in design.md and the outline design's interface line; the backend.md addition is left for the change demo). Work commit `7901f3e` → archive commit `cd2a51e` → journal commit `87afc67` (session 5). Order ids count from 1 after each service restart (in-memory data, wiped on restart) — expected for the demo.

### Coupon list

implement.jsonl has 3 entries (outline design interface GET /api/coupons, the coupon prototype page, this task's prd.md), check.jsonl has 2 (outline design acceptance criteria, this task's prd.md), `task validate` passes. 2.1 implementation: `src/routes/coupons.js` 11 lines (GET /api/coupons), `server.js` 33 lines (+4, route mounted), `public/coupons.html` 94 lines (fetches /api/coupons, five fields, thresholds and amounts converted from fen to yuan, a 0 threshold shows "无门槛", the coupon name row also carries the code, and anything but "未使用" is greyed out and marked "（不可用）").

2.2 check:

```sh
curl -s http://localhost:3000/api/coupons
# {"code":0,"data":[{"code":"MAN100JIAN10","name":"满100减10","threshold":10000,"discount":1000,"status":"未使用","expires":"2026-12-31"},{"code":"MAN200JIAN30","name":"满200减30","threshold":20000,"discount":3000,"status":"已使用","expires":"2026-12-31"},{"code":"NEW5","name":"新人5元券","threshold":0,"discount":500,"status":"已过期","expires":"2026-10-01"}]}
curl -s -o /dev/null -w "%{http_code} %{size_download} bytes" http://localhost:3000/coupons.html
# 200, 3916 bytes
```

The last round's 2.2 is a full-range regression: GET /api/products returns the three products, the six POST /api/orders cases (missing address 400, 1 keyboard + MAN100JIAN10 pays 28900, 2 mice + MAN100JIAN10 pay 16800, 1 mouse 400 COUPON_THRESHOLD), and `/`, /index.html, /cart.html, /checkout.html, /coupons.html all HTTP 200.

3.3 write back: the check ran and the answer was "no spec update". Work commit `8c03060` → archive commit `8c4e04f` → journal commit `f37743d` (session 6).

---

**[Parent task: final integration review]**

With all six sub-tasks archived, the parent task is `[6/6 已完成]` and gets a final integration review before it is archived. The review reads the acceptance criteria + each sub-task's check results + the interface contracts — it does not re-read the whole prototype; a fresh service start runs everything once:

```sh
curl -s http://localhost:3000/api/products
# {"code":0,"data":[{"id":1,"name":"无线鼠标","price":8900,"stock":120},{"id":2,"name":"机械键盘","price":29900,"stock":45},{"id":3,"name":"27寸显示器","price":129900,"stock":20}]}
curl -s http://localhost:3000/api/coupons
# three coupons (thresholds 10000/20000/0, as before the change)
curl -s -X POST http://localhost:3000/api/orders -H "Content-Type: application/json" -d '{"items":[{"id":2,"qty":1}]}'
# {"code":"ADDRESS_REQUIRED","data":null}, HTTP 400
curl -s -X POST http://localhost:3000/api/orders -H "Content-Type: application/json" -d '{"items":[{"id":2,"qty":1}],"address":"测试地址","couponCode":"MAN100JIAN10"}'
# {"code":0,"data":{"id":1,"items":[{"id":2,"qty":1}],"address":"测试地址","couponCode":"MAN100JIAN10","total":28900,"status":"待付款"}}, HTTP 200
curl -s -X POST http://localhost:3000/api/orders -H "Content-Type: application/json" -d '{"items":[{"id":1,"qty":2}],"address":"测试地址","couponCode":"MAN100JIAN10"}'
# total 16800, HTTP 200
curl -s -X POST http://localhost:3000/api/orders -H "Content-Type: application/json" -d '{"items":[{"id":1,"qty":1}],"address":"测试地址","couponCode":"MAN100JIAN10"}'
# {"code":"COUPON_THRESHOLD","data":null}, HTTP 400
# all four pages HTTP 200: index.html, cart.html, checkout.html, coupons.html
```

The parent task has no code commit of its own (the code is all in the sub-tasks' work commits); before archiving, run `task set-branch 11-13-商城 feature/11-13-商城`. finish-work step 1 (`task context --mode record`) found no other finished-but-unarchived tasks besides the parent, so the "archive together" question has N=0 and needs no asking. Archive commit `13c9f37` → journal commit `90531a6` (session 7; `--commit` records the planning-products commit `b4bf106`, since the parent has no work commit of its own, and the summary says so). After archiving, `task list` shows 0 active tasks and `task list-archive` shows 7 tasks for 2026-11.

---

**[Change demo: the designer returns]**

After the parent task is archived, the user changes their mind: the coupon threshold moves from 100 yuan to 200 yuan. How does a change work under design-first? The designer edits `doc/` and the affected prototype pages first, the parent task's prd records a `[变更]`, and 1.5 reviews only the change; an archived task is never reopened for rework, an in-progress one pauses and goes back, one not started is edited directly.

Create the change task first. Following 1.2, the first instinct is to hang it under the parent task — but the parent is already archived:

```sh
sunshine task create "满减门槛改成 200 元" --slug 满减门槛改成-200 --parent 11-13-商城 --no-start
# 错误：无法在 /tmp/scenes/s6-loop/.sunshine/tasks 下解析任务 "11-13-商城"
# 错误：父任务无法解析：11-13-商城
# 未创建任务。请传入已存在的任务目录。
```

A sub-task cannot be created under an archived parent — that is what 1.2 means by "split once": either create all the sub-tasks during planning, or archive the parent only after all its sub-tasks are archived; a rework task cannot hang off an archived parent, so it is created standalone. Create it as an independent task, with the PRD recording `[变更 2026-11-13]`:

```sh
sunshine task create "满减门槛改成 200 元" -d "优惠券满减门槛 100 元改为 200 元" --slug 满减门槛改成-200 --no-start
# 任务已创建：11-13-满减门槛改成-200
```

One point of wording: `[变更 2026-11-13]` is not written into the archived parent task's prd.md — an archive is history, it is not reopened; the `[变更]` goes only into the change task's own prd.md and the design documents' change log. The archived parent prd still says "满 100 元减 10 元" and "2 件无线鼠标用 MAN100JIAN10 实付 16800 分" — that is its pre-archive state, not an omission.

The designer revisits six places:

| # | File | Line | Change |
| --- | --- | --- | --- |
| 1 | `doc/design/需求列表.md` | 26 (the R-004 line) | "满 100 元减 10 元" → "满 200 元减 10 元"; the acceptance "2 件无线鼠标用 MAN100JIAN10 实付 16800 分" → "返回 400 COUPON_THRESHOLD" |
| 2 | `doc/design/需求列表.md` | 49 (change log line) | New line: `2026-11-13 ｜ MAN100JIAN10 门槛从 10000 分改为 20000 分（满 100 元减 10 元改为满 200 元减 10 元）；券名“满100减10”未改 ｜ 门槛调整` |
| 3 | `doc/design/概要设计.md` | 70 (acceptance criteria, R-004 line) | Same as #1 (acceptance updated in step) |
| 4 | `doc/design/概要设计.md` | 78 (risk line) | Now reads "券名‘满100减10’与门槛 20000 分（200 元）不一致，名不副实（2026-11-13 由 10000 改为 20000，未改名）" |
| 5 | `doc/prototype/结算页.html` | 50 (threshold hint line) | "满 100 元减 10 元" → "满 200 元减 10 元（券码 MAN100JIAN10，满 20000 分减 1000 分）" |
| 6 | `doc/prototype/mock.js` | 14 | MAN100JIAN10 `threshold: 10000` → `threshold: 20000` (the shared prototype mock updates; the coupon page renders from the mock, so it follows automatically) |

A grep afterwards confirms: the old value "10000" remains only in the change log and risk lines, where it is history; "16800" is gone entirely; "满100减10" remains only as the coupon name itself (deliberately not renamed).

The change task's 1.5 reviews only the changed part (prd.md + the design changes above); once confirmed, `task start`. implement.jsonl / check.jsonl have 3 / 2 entries (requirements list R-004 and change log, outline design acceptance and risks, this task's prd.md with `[变更]`), and `task validate` passes. The implementation changes three places:

| # | File | Line | Change |
| --- | --- | --- | --- |
| 1 | `src/db.js` | 11 | MAN100JIAN10 `threshold: 10000` → `threshold: 20000` |
| 2 | `.sunshine/spec/backend.md` | 16 (new line) | `优惠券 MAN100JIAN10 门槛 20000 分（2026-11-13 由 10000 改为 20000；券名“满100减10”未改，名不副实）。` |
| 3 | `public/checkout.html` | 50, 179 | Threshold hint "满 100 元减 10 元" → "满 200 元减 10 元" (line 50); the `COUPON_THRESHOLD` message "未满门槛，满 100 元减 10 元" → "未满门槛，满 200 元减 10 元" (line 179); no other logic touched |

Post-change curl checks (the address is `测试地址` throughout):

```sh
curl -s -X POST http://localhost:3000/api/orders -H "Content-Type: application/json" -d '{"items":[{"id":1,"qty":2}],"address":"测试地址","couponCode":"MAN100JIAN10"}'
# {"code":"COUPON_THRESHOLD","data":null}, HTTP 400 (2 mice used to pay 16800; now they are below the 200-yuan threshold)
curl -s -X POST http://localhost:3000/api/orders -H "Content-Type: application/json" -d '{"items":[{"id":2,"qty":1}],"address":"测试地址","couponCode":"MAN100JIAN10"}'
# {"code":0,"data":{"id":1,"items":[{"id":2,"qty":1}],"address":"测试地址","couponCode":"MAN100JIAN10","total":28900,"status":"待付款"}}, HTTP 200 (1 keyboard still pays 28900)
curl -s http://localhost:3000/api/coupons
# MAN100JIAN10 {"code":"MAN100JIAN10","name":"满100减10","threshold":20000,"discount":1000,"status":"未使用","expires":"2026-12-31"} (the name is still "满100减10", the threshold is now 20000)
```

Work commit `3f80cf9` (7 files, +10/−9, design revisit and implementation in one commit) → archive commit `261bc6c` → journal commit `28eb3c1` (session 8). After archiving, `task list-archive` shows 8 tasks for 2026-11.

---

Final git log (old → new, 25 commits; each task runs "work commit → archive commit → journal commit" in order):

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

The loop in this scenario is: empty directory `init` → per-requirement loop (each requirement gets one round: questions → requirements list updated on disk → that requirement's prototype page), stopping when both sides agree the current batch is enough → the outline design written on demand → review (final planning summary) → one split creating all 6 sub-tasks → each sub-task runs implement, check, write back, commit, archive in turn → the parent task's integration review, then archive → a change goes through the designer revisiting the design and the loop closes again. Design-first has no mandatory deliverables list — the outline design, the requirements list, and the prototype are not hard requirements, they are just this scenario's design package; the designer decides what the design produces, 1.5 reviews the current batch, and the split follows the requirements list (the main thread and the contract) and what is actually in `doc/`; with no design documents it is simply a code-only task.

A few boundaries, stated as they are:

- The "add to cart" button's localStorage logic ships with R-001's product list page; R-002's deliverable is the cart page itself (reading localStorage, rendering, totalling) plus the end-to-end check — the cart has no backend API, curl cannot click a button, so the check is page reachability + key-logic grep + a node calculation of the total.
- `task start` left no session record at all this round: the `.sunshine/.runtime/` directory does not even exist, and none of the 8 starts wrote a session file; the status still flipped as normal, and nothing blocked.
- The automatic commits from archiving and add-session succeeded on the first try all 8 times — the demo repo had a local git identity set up in advance.
- The `.gitattributes` that `add-session` leaves behind (the journal's `merge=union` rule) was not picked up by any of the 8 journal commits, and the final `git status` still shows one leftover line, `?? .gitattributes`.
- `.sunshine/.developer` is not gitignored and entered the repo with the baseline commit `fd8a2a1`; `init` also does not generate a repo-root `.gitignore`, and `node_modules/` was only excluded once the backend-foundation sub-task created a root `.gitignore`.
- Two sub-agent exec calls reported `subprocess.output_drain_timeout` (exitCode 0): one in the coupon-list implement, one in the change implement; both reports were complete, and the results were independently re-checked by the main session.
- Known prototype-versus-production differences: the prototype mock.js gives coupons only a code, no name field, so the page shows the code where the name goes; the production db.js has a name field. The production cart page does not pre-fill demo data (the prototype stuffs 1 keyboard + 2 mice when it opens with no data), so no data means the empty state.
- Real browser rendering was not verified: the sandbox has no browser, so the prototype and production pages were only fetched with curl (HTTP 200 + key-logic grep); what the pages look like and how the buttons feel were never seen in a real browser.

## Appendix: command reference

That is the whole mainline: you ran `init` once, asked for the refund validation in plain language in the agent UI, the agent planned, implemented, checked, wrote the lessons back into the spec, and finished with `/sunshine:finish-work`; you reviewed the diff and accepted; the task is archived and the session is in the journal.

Everything else in Sunshine is something you reach for when you step in manually. These are the only commands you ever need to type yourself — every one of them was run for real earlier in this tutorial (the one-line descriptions of `uninstall` / `ablate` / `restore` come from their own `--help`, not from a run here).

Everything not listed here — `task create/start/archive`, `task add-context/validate`, `channel create/send/spawn/wait`, `task add-session` — is the agent's job, driven by `workflow.md` (see section 4). If you find yourself typing those by hand every day, something is wrong: open the agent UI and let it drive.

### init reference

#### What init asks (without `-y`, four questions in order)

1. **Pick platforms** ("请选择要配置的 AI 编程平台", "Select the AI programming platforms to configure"): a multi-select of all 22 platforms, with Claude Code and Cursor checked by default; selecting none cancels the init ("未选择任何平台，初始化已取消。", "No platform selected, initialization cancelled.").
2. **Statusline confirmation** (only when Claude Code is selected): "是否为 Claude Code 安装状态栏……？" ("Install the statusline for Claude Code…?"), default no; pass `--with-statusline` to install it directly.
3. **Pick a spec template** ("请选择规范模板", "Select a spec template"): "从零开始" (from scratch) by default, or "自定义" (enter a marketplace source and pick from it); with `--registry` the picker lists that marketplace's spec templates, and the picked one is installed instead of the blank specs.
4. **Pick a project type** (only when nothing is detected): "检测不到项目类型，请选择" ("Project type not detected, please choose"): frontend / backend / fullstack / product design, default fullstack. Picking product design means the project starts design-first: it generates the full spec set — `system.md`, `frontend.md`, and `backend.md` — plus the `doc/` design templates (requirements list, outline design, prototype).

`-y` skips all four questions and uses the defaults; passing the matching flag (a platform flag, `--type`, `--template`) skips that question too.

Project-type detection reads `package.json` dependencies and common frontend/backend config files (`vite.config.ts`, `go.mod`, etc.) and generates only the matching specs — a frontend project gets `system.md` and `frontend.md`, a backend project gets `system.md` and `backend.md`, a fullstack project gets all three. Product design is never detected; pick it manually (interactive question or `--type 产品设计`). With `--registry` (specs from the template marketplace) there is no detection and no question.

#### Common options

| Option | What it does |
| --- | --- |
| `-u, --user <name>` | Developer name, written to `.sunshine/.developer`; priority: `-u` > git user.name > `developer`; new tasks default their creator and assignee to it |
| `-y, --yes` | Skip all prompts: default platforms Claude Code + Cursor, default fullstack, default blank specs, no statusline |
| `-f, --force` | Rebuild the skeleton and platform files even when already initialized (see "Three modes" below) |
| `--type <前端/后端/全栈/产品设计>` | Specify the project type explicitly (前端=frontend, 后端=backend, 全栈=fullstack, 产品设计=product design), skipping detection and the question |
| `--registry <source>` | Template marketplace source: `gh:owner/repo`, a full git URL, an HTTP(S) URL, a `git@` address, or a local git repository path |
| `--template <id>` | Marketplace entry id, must be used with `--registry`; when omitted, a marketplace source errors out under `-y` and shows the picker interactively |
| `--overwrite` / `--append` | Overwrite the target if it exists / only fill in missing files; only with `--template` |
| `--with-statusline` | Install the statusline for Claude Code; only effective when Claude Code is selected |
| `--<platform>` | Install only the named platform: `--claude` `--cursor` `--codex` `--kiro` `--gemini` `--qoder` `--pi` `--zcode` `--trae` `--omp` `--snow` `--opencode` `--dsh` `--reasonix` `--grok` `--kimi` `--kilo` `--antigravity` `--devin` `--codebuddy` `--copilot` `--droid` (22 in total; `--windsurf` is a deprecated alias for `--devin` and is converted automatically with a warning) |

#### Three modes

- **Fresh init**: no `.sunshine/` in the directory yet; generates the skeleton and platform configs following the flow above.
- **Rebuild** (`-f`): rebuilds the skeleton and platform files even when already initialized.
- **Incremental**: running `init` again without `-f` in an initialized project only adds the newly selected platforms (and can also install a marketplace template with `--registry`), leaving existing platform files and the `.sunshine/` skeleton untouched. For example, `sunshine init --codex -y` adds only Codex.

Note: `init` refuses to run in your home directory (set `SUNSHINE_ALLOW_HOMEDIR=1` to bypass).

#### What init installs

What `init` just installed — 115 files, in two groups:

- **For the agent to read — `.sunshine/` (17 files):** `workflow.md` (714 lines, the full development workflow the agent follows), `spec/` with three spec templates (`system.md`, `backend.md`, `frontend.md`), `config.yaml` (project config with Chinese comments), `AGENTS.md`, and `agents/` (five agent definitions). `AGENTS.md` opens with:
  ```text
  # AGENTS.md（Sunshine）

  本文件面向在本仓库工作的 AI 编程助手。

  ## 工作方式

  - 先读 `.sunshine/spec/` 与 `.sunshine/workflow.md`，再动手。
  ```
  ("This file is for the AI programming assistant working in this repository. / How to work: first read `.sunshine/spec/` and `.sunshine/workflow.md`, then start.")
- **For the platforms to load — `.claude/` and `.cursor/` (98 files):** skills, commands (`/sunshine:continue`, `/sunshine:finish-work`), sub-agents (`sunshine-implement`, `sunshine-check`, `sunshine-research`), and hook configs. These are what make the agent know Sunshine's workflow at all.

Check what is configured:

```sh
sunshine platforms
# 已配置的平台：
#   Claude Code (claude-code) — .claude
#   Cursor (cursor) — .cursor
```

("Configured platforms: …") By default `init` installs Claude Code and Cursor. To add Codex later, run `init --codex -y` again in the same project — it only adds the new platform and leaves existing files untouched: There are 22 supported platforms in total; install a specific one with `sunshine init --<platform> -y`, e.g. `sunshine init --gemini -y`. With no platform flag, `init` still installs only Claude Code and Cursor.

```text
正在配置 Codex……
已安装平台：
  - Codex
提示：Codex hooks 须在 ~/.codex/config.toml 中设置 features.hooks = true（Codex 0.129+；旧版为 codex_hooks = true），并在 Codex 里运行一次 /hooks 审批，否则 .codex/hooks.json 不生效。
```

("Configuring Codex… / Installed platforms: Codex / Note: Codex hooks require `features.hooks = true` in `~/.codex/config.toml` (Codex 0.129+; older versions use `codex_hooks = true`), plus one `/hooks` approval run inside Codex, otherwise `.codex/hooks.json` does not take effect.")

### Manual-intervention cheat sheet

| When | Command | What it does |
| --- | --- | --- |
| Where did we get to? | `sunshine task list` / `sunshine task current` / `sunshine task context` | Show active tasks, the current task, and the full session context |
| Check the next step | `sunshine task list-archive` | Show archived tasks by month |
| Switch how the agent works | `sunshine workflow --list` / `sunshine workflow --template tdd` | List or switch workflow templates ([section 7](#7-switching-workflows)) |
| Refresh project templates | `sunshine update --dry-run` (drop `--dry-run` to apply) | Sync template files; user data is never touched ([section 8](#8-update-and-the-template-marketplace)) |
| Install a spec from a marketplace | `sunshine init --registry <source> --template <id>` | Install a spec template, refreshed by `update` afterwards ([section 8](#8-update-and-the-template-marketplace)); in interactive mode, omit `--template` to pick from a list |
| All init operations | `sunshine init --help` | Platforms, statusline, template picker, project type, three modes — see the init reference in this appendix |
| Look back | `sunshine mem search "退款"` / `sunshine mem list` | Search local session history, read-only ([section 9](#9-mem-look-back-at-sessions)) |
| Multi-agent work | `sunshine channel list` / `sunshine channel messages <name>` | Inspect channels and their event timelines ([section 10](#10-channel-multi-agent-collaboration)) |
| Update the CLI itself | `sunshine upgrade --dry-run` (drop `--dry-run` to apply) | Upgrade the `sunshine` binary ([section 11](#11-upgrade-update-the-cli-itself)) |
| See what is configured | `sunshine platforms` | Show the platforms configured for this project |
| Remove Sunshine from a project | `sunshine uninstall` / `sunshine ablate` / `sunshine restore` | Delete all Sunshine files, temporarily remove them (reversible), or restore exactly what `ablate` saved |
