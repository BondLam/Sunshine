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
下载地址：https://github.com/BondLam/Sunshine/releases/download/v0.2.1/sunshine_0.2.1_linux_amd64.tar.gz
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

This section was verified with **v0.3.0** (the current Release; `sunshine version` prints `Sunshine 版本 0.3.0`). The demo directories all live under `/tmp/scenes/` (`s1-new`, `s2-old`, `s3-cross`, `s4-refactor`, `s5-bug`, `s6-design`) — delete them and nothing in any real project is affected. The `11-10` prefix in task directory names is the month-day of the day the commands were run (2026-11-10); section 12.6 was run on 2026-11-11, so its task directories start with `11-11`. Command outputs are real excerpts from runs made for this tutorial (`……` marks omitted lines, not rewritten ones); prompts shown to the agent are labeled as example prompts, not records of a real session. Each of the six scenarios includes an example session with Claude Code (the conversation is an example; the commands and outputs are real values from runs) — you talk, answer, and confirm in the conversation; the commands are the agent's job, running under the hood. The CLI itself is Chinese-only, so commands and sample outputs below are kept in the original Chinese; the explanations around them are in English.

### 12.1 Starting a new project from zero: a duty-roster tool

An empty directory, nothing in it — the cleanest starting point. Create an empty directory, put it in git, and leave the rest to Claude Code: `init`, creating the task, lining up context, starting it — all of it runs underneath, and you type no commands. An empty directory has no detectable type, so it defaults to full-stack and all three specs (system.md, frontend.md, backend.md) are generated — a new project starts with a default set of specs, and the team adjusts them later, which beats starting with nothing.

Open the project in Claude Code and state the first requirement in plain language (same for Codex and Cursor):

Example prompt (an example, not a record of a real session):

> I'm starting a duty-roster tool from scratch: generate this week's roster from a CSV, first column date, second column the person on duty. Please follow the Sunshine workflow: ask me first to pin down the missing product logic (date formats, output form) and technical decisions (language, how to run it); once confirmed, create the first task, write the PRD, then implement.

Here is how the session continues (the conversation is an example; the commands and outputs are real values from runs):

> **Claude Code:** Three quick questions first: 1. What date format is in the CSV — `11-10`, or `2026-11-10`? 2. Where should the roster go — printed to the terminal, or written to a file? 3. What language, and how do I run it?
>
> **You:** Dates are month-day, like `11-10`; printing to the terminal is fine; Go, a command-line tool, run with `go run ./cmd/duty 值班表.csv`.
>
> **Claude Code:** Got it. Here's the plan: create the task "值班表小工具第一版" and write a PRD that says generate this week's roster from the CSV (first column date, second column person on duty) and print it to the terminal; acceptance is running `go run ./cmd/duty 值班表.csv` and seeing lines like `11-10：张三`. Once you confirm, I'll create the task, write the PRD, and start implementing. OK?
>
> **You:** OK, go ahead.
>
> **Claude Code:** The task is created, the context is lined up, the task is started. Now I'll implement it: a small tool that reads the CSV and prints the roster. Let me show you the run.

Here is the real run result:

```text
11-10：张三
11-11：李四
11-12：王五
```

> **Claude Code:** Done — the task is archived:

```text
[2026-11] - 1 个任务
  - 11-10-值班表小工具第一版/
```

After the first task, the project has three things: code that runs, an archived task record, and specs generated from the default templates. From the second task on, the agent has something to work from.

### 12.2 Adopting an existing project: land the conventions in spec first

The repo exists, but the conventions live mostly in people's heads — the most common adoption case. The demo is a refund API that has run for two years (`express` is the only dependency in `package.json`, one file `src/server.js`), with the team's conventions written in `README.md` and nowhere else:

```markdown
# refund-api

团队约定（一直靠人记，没落进任何文档）：
- 接口统一返回 { code, data }，code 是大写下划线错误码。
- 所有写操作必须记审计日志。
- 端口固定 3000，改端口要全组同步。
```

(In English: "Team conventions (always remembered by people, never written into any document): APIs return { code, data } with code an UPPER_SNAKE error code. Every write must write an audit log. The port is fixed at 3000; changing it needs the whole team to sync.")

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

> **Claude Code:** I can see all three conventions: APIs return `{ code, data }`, every write logs an audit trail, and the port is fixed at 3000. Quick check on where they go — the first two into `backend.md`, but the port: `system.md` or `backend.md`?
>
> **You:** The first two into `backend.md`. The port is a team-wide rule — put it in `system.md`.
>
> **Claude Code:** Got it. Here's the plan: create the task "把现有约定落进规范", write a PRD that says land the three remembered conventions from the README into spec; acceptance is the audit-log convention appearing once in backend.md. Once you confirm, I'll create the task, write the PRD, and write the conventions in. OK?
>
> **You:** OK, go ahead.
>
> **Claude Code:** The task is created and the conventions are written in. Here is what actually landed:

```markdown
## 退款接口（团队已有约定）

- 接口统一返回 `{ code, data }`，code 是大写下划线错误码。
- 所有写操作必须记审计日志。
```

```markdown
## 团队约定

- 服务端口固定 3000，改端口要全组同步。
```

> **Claude Code:** Let me verify what landed for you: the audit-log convention appears once in backend.md. The task ran its normal course and is archived.

From now on the conventions no longer live only in senior engineers' heads: the agent on a new task reads the spec, and the conventions survive people leaving. The acceptance bar for adopting an existing project is one line — **after `init`, `git status` shows no modified old files, and spec has one more convention that used to live only in memory**.

### 12.3 Delivering a product feature: one task across product, API, data, and UI

When one task spans four layers, the usual failure is changing one layer and missing another — Sunshine's answer is one task whose context list names the files in all four layers at once, so acceptance checks against the same list. The demo is a refund app (`src/Refund.vue` is the UI, `src/server.js` the API, `data/refunds.json` the data), and the task is making the refund reason mandatory.

State the requirement to Claude Code (same for Codex and Cursor):

Example prompt (an example, not a record of a real session):

> I need to deliver a product feature: make the refund reason mandatory, one task across the product, API, data, and UI layers (the UI is src/Refund.vue, the API is src/server.js, the data is data/refunds.json). Please follow the Sunshine workflow: ask me first to pin down the unclear validation rules (the reason options, the error code when the field is missing) and the order to change the four layers; once confirmed, create the task, write the PRD, then implement.

Here is how the session continues (the conversation is an example; the commands and outputs are real values from runs):

> **Claude Code:** Three quick questions first: 1. What are the reason options in the dropdown? 2. What does the backend return when `reason` is missing — 400 plus which error code? 3. In what order do the four layers change?
>
> **You:** Three reasons: quality issue, wrong size, other — picking "other" shows a text box; missing `reason` returns 400 with `REASON_REQUIRED`; API first, then UI, data fields follow.
>
> **Claude Code:** Got it. Here's the plan: create the task "退款原因必填校验（全链路）", write a PRD that says change all four layers together; line up the context by layer — `src/Refund.vue`, `src/server.js`, `data/refunds.json` into `implement`, `.sunshine/spec/backend.md` into `check`; the change order goes through a channel. Once you confirm, I'll create the task, line up the context, and start it. OK?
>
> **You:** OK, go ahead.
>
> **Claude Code:** The task is created and the context is lined up by layer: the UI, API, and data files for implementation, the backend API contract for acceptance. The four layers change API first, then UI, and I put the order in the channel — two messages: API first — POST /refunds with a missing reason returns 400 with `REASON_REQUIRED`; once the API is done, the UI — a three-option reason dropdown, with "other" showing a text box.
>
> **Claude Code:** The channel is bound to the task, so the task path shows in the messages; these two are numbered coordination messages, not casual notes. Waiting for a worker's reply and getting a timeout is normal when no worker answers — this section only coordinates as far as sending the messages, and spawning a real worker needs a logged-in claude / codex on this machine (section 10). At the end the workflow goes back to native and the task is archived as usual.

The acceptance bar for a cross-layer task is in the same list: the files in all four layers done, and the one spec for acceptance verified line by line — whichever layer is missed shows up on the list.

### 12.4 Refactoring a legacy module: pin the behavior first, then change the code

The code runs, but it is no longer safe to change — the first step of a refactor is not to change anything, but to pin down what the code currently does. The demo module is a price function with all the discount rules hard-coded in `if`s:

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

> I need to refactor a legacy module: the price function Price, with all the discount rules hard-coded in `if`s — the code runs, but it is no longer safe to change. Please follow the Sunshine workflow (the TDD template this time): ask me first to confirm the behavior to pin down (the expected prices for each quantity tier and VIP) and the boundaries of the refactor; once confirmed, create the task, write the PRD, then follow TDD: write characterization tests to pin the current behavior first, then refactor.

Here is how the session continues (the conversation is an example; the commands and outputs are real values from runs):

> **Claude Code:** Two quick questions first: 1. Which cases should I pin down — the expected prices for each quantity tier, VIP and non-VIP? 2. Where does the refactor stop — only pull the discount rules out, with `Price`'s signature and return value unchanged, right?
>
> **You:** Pin three cases: 1 item non-VIP 100, 10 items non-VIP 900, 10 items VIP 855; only pull the discount rules out, `Price`'s signature stays, behavior must not change.
>
> **Claude Code:** Got it. Here's the plan: switch to the TDD template first, create the task "重构价格计算"; write characterization tests to pin those three cases (all green is the baseline), then pull the discount rules into constants and two small functions with `Price` only composing them, and run the same tests again — still all green. Once you confirm, I'll start. OK?
>
> **You:** OK, go ahead.
>
> **Claude Code:** The workflow is on the TDD template, the task is created and started. Writing the characterization tests to pin the three cases first:

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

Let me show you the run — all green (`ok shop 0.003s`). All green is the baseline — this step changed no business code at all, it only proves "the current behavior is exactly these three cases". Only then does the refactor happen: discount rules become constants and two small functions, and `Price` only composes them:

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

Run the same tests once more after the refactor — still all green (`ok shop 0.002s`). The behavior did not change, so the refactor stands. The red light in the TDD workflow is for new behavior; a pure refactor runs the other way around: **write characterization tests to pin the behavior (green), then refactor (still green)**. Wrap up by writing the new convention into spec, so the next change starts from the test table:

```markdown
## 价格计算

- 折扣规则集中在 calc.go 的常量与 applyBulk / applyVIP，Price 只做组合；改规则先改 calc_test.go 的用例表。
```

(In English: "Price calculation: discount rules live in the constants and applyBulk / applyVIP in calc.go; Price only composes them. Change a rule by changing the case table in calc_test.go first.")

Archive the task as usual and switch the workflow back to native.

### 12.5 Fixing a bug that keeps coming back: find the last session first

When the same class of problem appears again and again, the most expensive part is investigating it from zero every time. The bug in this section really happened: a codex worker failing to connect with a 401 — once on November 9, again in the early hours of November 10, and once more today.

State the requirement to Claude Code (same for Codex and Cursor):

Example prompt (an example, not a record of a real session):

> I need to fix a bug that keeps coming back: a codex worker failing to connect with a 401 — once on November 9, again in the early hours of November 10, and once more today. Please follow the Sunshine workflow: use mem to find the last related session first, then ask me to confirm whether this is the same class of problem and what the reproduction steps are; once confirmed, create the task, write the PRD, and write the lesson into the spec after the fix.

Here is how the session continues (the conversation is an example; the commands and outputs are real values from runs):

> **Claude Code:** I checked `mem` first: the last related session is the dispatch from November 10, 05:05, whose message to the worker already mentions 401. Two quick questions: 1. Is this the same class of problem — a codex worker failing to connect with a 401? 2. How do I reproduce it — on this machine, or online?
>
> **You:** Same class — still 401 today; reproduce it on this machine, codex isn't logged in.
>
> **Claude Code:** Got it. Here's the plan: reproduce the dispatch once on this machine to confirm it's still 401; then create the task with the session id in the description, and write the lesson into spec after the fix. Once you confirm, I'll start. OK?
>
> **You:** OK, go ahead.
>
> **Claude Code:** Reproducing the dispatch once on this machine: a channel is created, the same message goes to the worker, and the worker starts — then fails in its log (codex is not logged in on this machine, the same precondition section 10 states):

```text
unexpected status 401 Unauthorized: Missing bearer <redacted> basic authentication in header, url: https://api.openai.com/v1/responses
```

> **Claude Code:** The worker never answered, but the session is on disk. `mem` finds this dispatch: the one from November 10, 05:05, session 01a1c3d8; searching 401 hits once — the sentence sent to the worker: "线上 codex worker 又连不上了，之前报过 401，帮我查一下这次的原因" (the codex worker online can't connect again — it reported 401 before; help me check the cause this time).

One boundary found by actually running this, worth remembering: **`mem` searches the words in dialogue turns (user / assistant); it does not search error events**. The session from the early hours of November 10 (`01a1c376`, in `/tmp/h-test/chanproj`) had a worker fail with the same 401, but its 401 only appears in an error event, not in any dialogue turn — so searching `401` does not find it. What you search for is the sentence that was sent to the worker at the time. This run is findable because the message sent to the worker already says 401.

Once the last session confirms it is the same class of problem, create the task — with the session id (01a1c3d8) in the description, so the agent does not have to ask "what happened last time" all over again. When the task is done, the lesson goes into spec — otherwise it will happen the same way next time, which is the actual fix for a recurring bug:

```markdown
## 多 Agent 派发

- 派发 codex worker 前先确认本机 codex 已登录；spawn 失败时先看 worker.log（401 即未登录），不要反复重派。
```

(In English: "Multi-agent dispatch: before spawning a codex worker, confirm codex is logged in on this machine; when a spawn fails, read worker.log first (401 means not logged in) instead of spawning again and again.")

Archive the task as usual. Next time a 401 shows up, the agent reads the spec and checks the login first, instead of spawning another worker into the same wall.

### 12.6 Design-first product work: requirements → design documents → split → code

This scenario uses a brand-new project: a team leave-request tool, built from zero. Designers and developers share one workflow, but they enter and exit at different times — each part below says who is on stage.

**[Designer enters]**

State the requirement to Claude Code (same for Codex and Cursor):

Example prompt (an example, not a record of a real session):

> I want to build a brand-new team leave-request tool. Let's do design first — follow the Sunshine workflow: ask me what is unclear about the rules and the prototype details first; once I confirm, create a task, write the PRD, and land the requirements list (one item per requirement, each pointing at its page in the system prototype), the outline design, and one system prototype carrying mock data in doc/; after the review passes, split it into sub-tasks per requirement and then implement.

Here is the session (the conversation is an example; the commands and outputs are real values from runs):

> **Claude Code:** First, how many requirements does this product have (one at a time, with options): 1. Is leave-requesting two requirements — submit and approve — or one? 2. What leave types are there? 3. Is the reason mandatory — block the submit or let it through? 4. What is the longest a single annual-leave request can be, and what happens over the cap?
>
> **You:** Two requirements: requirement one is submitting a leave request, requirement two is approving one. Types are 年假 / 病假 / 事假 (annual / sick / personal leave); the reason is mandatory, block the submit without it; annual leave is at most 10 days per request, over the cap it returns `NEED_APPROVAL` for approval.
>
> **Claude Code:** Now the prototype details: 1. One requirement, one page — requirement one is the request page, requirement two is the approval list page, right? 2. What does the approval list page show? 3. What does the backend return when `reason` is missing? 4. Does the prototype carry mock data?
>
> **You:** Right, requirement one is the request page, requirement two is the approval list page. The list shows applicant, type, start and end dates, days, and status, with a 通过 (approve) and 驳回 (reject) button on each row; a missing `reason` returns 400 with `{"error": "reason 必填"}` (reason required); the prototype carries mock data — three fake rows in the list （张三 pending, 李四 approved, 王五 pending) — so the demo is clickable.
>
> **Claude Code:** Got it. Here's the plan: create the task, write the PRD for both requirements; land the requirements list (one item per requirement, each pointing at its page in the system prototype), the outline design, and one system prototype (two pages plus navigation between them) in `doc/`; after the review passes, split it into two sub-tasks per requirement and then implement. Once you confirm, I'll create the task, write the PRD, and land the design documents. OK?
>
> **You:** OK, go ahead.

Once the questions are answered, the design package lands in the project's `doc/`, not `.sunshine` — the designer finishes all deliverables first, and development only starts after the review passes:

```text
doc/design/需求列表.md
doc/design/概要设计.md
doc/prototype/原型.html
```

The requirements list is one item per requirement (real file content, excerpt): requirement one "提交请假申请" (submit a leave request) points at the request page in the system prototype — the type dropdown （年假 / 病假 / 事假） is mandatory, the reason is mandatory, annual leave is at most 10 days per request, over the cap it goes to approval; requirement two "审批请假申请" (approve a leave request) points at the approval list page in the system prototype — the list shows applicant, type, start and end dates, days, and status, with a 通过 (approve) and 驳回 (reject) button on each row. The outline design's interface lines read (real content):

```markdown
- `POST /api/v1/leaves`：缺 `reason` 返回 400，错误体 `{"error": "reason 必填"}`。
- 年假单次最长 10 天，超限返回 `NEED_APPROVAL`。
- `GET /api/v1/leaves`：返回全部记录。
```

(In English: "`POST /api/v1/leaves`: a missing `reason` returns 400 with `{"error": "reason 必填"}`. Annual leave is at most 10 days per request; over the cap it returns `NEED_APPROVAL`. `GET /api/v1/leaves` returns all records.")

The system prototype is one file with two pages plus navigation between them: a "申请 | 审批列表" (Request | Approval list) nav at the top, a request page with a type dropdown (mandatory), start and end dates, a reason text field (mandatory), and a submit button, and an approval list page as a table (applicant, type, start and end dates, days, status) carrying three mock rows （张三 pending, 李四 approved, 王五 pending), each with a 通过 (approve) and 驳回 (reject) button — the demo is clickable. One prototype serves multiple requirements: requirements one and two both live in this one prototype, one page each; and the split follows the requirements — the sub-tasks split per requirement.

The designer builds the design package step by step too: clarify the requirements and write them into the requirements list first, then the outline design (structure, interfaces, acceptance), then dispatch `sunshine-prototype` to build the prototype — one page per requirement in the list, with mock data (the dispatch prompt gives only the requirements list and the outline design; the sub-agent reports back just the page count, the mock record count, and the file path, and the main session never writes the prototype itself) — and the complete system prototype is the end of the design phase, not the start. If building the prototype exposes a gap in the requirements, the requirements list is updated with it (through the change path), but the overall direction stays requirements first, prototype last.

Then comes the requirements review — the designer's last act before exiting: Claude Code lays out the whole package for you to confirm — the PRD, the design document list, and the split plan; nothing moves on until you confirm.

> **Claude Code:** For review: the PRD, the requirements list, the outline design, and the system prototype are all ready, and the split is two sub-tasks — requirement one is the request page plus the POST API, requirement two is the approval list page plus the GET API. Once you confirm, I'll create the parent task and split it in one go. OK?
>
> **You:** OK, split it.

**[Designer exits, developers enter]**

When developers enter (especially when design and development are done by different people), open the project in the agent UI first, then say one entry prompt:

Example prompt (an example, not a real conversation record):

> The design review passed and the subtasks are split. Now let's start development following the sunshine workflow: first list the parent task's subtasks and the first subtask's context for me to confirm; after I confirm, start from the first subtask and work through them one by one; start each subtask, then check it, write the specs back, commit, and archive it before starting the next one; once all subtasks are archived, run the final integration review on the parent task and archive it.

After confirmation, development follows the five rounds below; if the session closes and reopens midway, run `/sunshine:continue` — it routes by the `task.json` status.

The split happens once, after the review passes — and it follows the requirements: one prototype serves multiple requirements, so the sub-tasks split per requirement, covering the whole scope, with the sub-task list settled in one pass; the split itself starts nothing, and both sub-tasks are created in `planning`. The parent task records the sub-task map, and developers enter with the work already split and the context already curated (each sub-task's context points at its own page in the system prototype).

One requirement is one sub-task by default (the sub-task owns its page plus its interface, with the acceptance criteria together, so it's easy to verify). But when a requirement's frontend and backend are both large, or done by different people, or the backend API serves several pages, it splits further into a frontend sub-task and a backend sub-task — mechanically, a sub-task with its own sub-tasks (`task create --parent`), the parent's `children` listing both, each in planning. The handoff between the two is the interface contract in the outline design: the backend implements the API per the contract, the frontend builds the page per the contract plus the mock data in the prototype, so the frontend doesn't wait for the backend; the parent's final integration review verifies frontend and backend together.

Here is the real nested output (a separate minimal demo, only to verify that a sub-task can hold sub-tasks):

```sh
sunshine task list
# 全部活动任务：
#
#   - 11-11-接续演示任务/ (in_progress) [0/2 已完成] [演示者] <- 当前任务
#     - 11-11-需求示例-前端/ (planning) [演示者]
#     - 11-11-需求示例-后端/ (planning) [演示者]
#
# 总计：3 个任务
```

**What the split produces: what lands on disk**

The split is not just a line in the conversation — it lands on disk as real files (real directory tree; a task directory looks the same before and after archiving, except archiving moves it under `archive/2026-11/`):

```text
.sunshine/tasks/11-11-请假申请小工具/
  task.json        # parent task: children lists the two sub-tasks
  prd.md           # the parent task's PRD
  implement.jsonl  # empty — the parent implements nothing
  check.jsonl      # empty
.sunshine/tasks/11-11-需求一-提交请假申请/
  task.json        # sub-task: parent points back at the parent, status is planning
  prd.md           # the sub-task's PRD (a skeleton; the agent fills it in at 1.1)
  implement.jsonl  # what to read when implementing: the request page in the system prototype + the outline design
  check.jsonl      # what to check against: the frontend spec + the backend API contract
.sunshine/tasks/11-11-需求二-审批请假申请/
  ...（the same four files）
```

The parent's `task.json` records the sub-task map (real content):

```json
"children": ["11-11-需求一-提交请假申请", "11-11-需求二-审批请假申请"]
```

The sub-task's `task.json` points back at the parent (real content):

```json
"parent": "11-11-请假申请小工具",
"status": "planning"
```

Requirement one's context lists (real content) — this is what developers read on entry:

```json
implement.jsonl
{"file":"doc/prototype/原型.html","reason":"系统原型：申请页"}
{"file":"doc/design/概要设计.md","reason":"概要设计：接口与验收"}

check.jsonl
{"file":".sunshine/spec/frontend.md","reason":"前端规范：申请页"}
{"file":".sunshine/spec/backend.md","reason":"接口契约：POST"}
```

The sub-task's `prd.md` is a skeleton that the agent fills in at the sub-task's own 1.1; complex tasks also add a `design.md` (technical design) and an `implement.md` (execution plan) before `start` — that is what the `prd.md` template itself says. In this demo the requirements live in the `doc/` design package, and the sub-tasks' context lists point at it.

**Developers enter: round by round**

Requirement one starts first. What Claude Code runs underneath (you don't type it), key output only:

```sh
sunshine task start 11-11-需求一-提交请假申请
# ✓ 状态：planning → in_progress（降级）
```

Once `start` succeeds, the status is written into `task.json` and the pointer into `.sunshine/.current-task` — close the session and reopen it, and the state is still in the files, not in the conversation (there is a resumption example below).

Here is the session while it develops (the conversation is an example; the commands and outputs are real values from runs):

> **Claude Code:** Requirement one is starting: the main session dispatches `sunshine-implement` to change `LeaveForm.vue` against the request page in the system prototype and add two validations to the POST in `server.js`, then dispatches `sunshine-check` to verify against the spec in `check.jsonl`.
>
> **Claude Code:** Implement is done: the request page has a three-item type dropdown （年假 / 病假 / 事假） with `required`, the reason is mandatory, and the form blocks the submit without one; a POST with a missing `reason` returns 400, over 10 days returns `NEED_APPROVAL`, a normal submit returns 200. Check passed too: all three options are there, `required` is there, the submit guard is there, a missing reason is 400, 11 days is `NEED_APPROVAL`, 2 days is 200 with `reason` and `days` in the record. I added a leave-form and a leave-API convention to the spec. Commit once? (One commit: request page plus POST API plus spec conventions.)
>
> **You:** Commit it, and archive this sub-task.
>
> **Claude Code:** Done — `task finish` clears the pointer and `task archive` archives it after the commit. The parent is now 1/2 done.

Key output (real runs): the three grep checks read `3`, `2`, `1` (three type options, two `required` spots, one submit guard); the three curl checks: a missing reason is `{"error":"reason 必填"}`, HTTP 400, 11 days is `{"code":"NEED_APPROVAL","data":null}`, 2 days is 200 with record `id` 2 and `days` 2; the commit is `31e6c36 需求一：提交请假申请，申请页 + POST 接口` (requirement one: submit a leave request, request page plus POST API); after the archive the parent shows 1/2 done.

One sub-task goes round by round like this — what each round reads and what it produces is in the files, never in the conversation:

1. **Implement (2.1)**: read the files in `implement.jsonl` (the request page in the system prototype, the outline design) plus your own `prd.md`, and dispatch `sunshine-implement` to change the code. Produces: the changed `LeaveForm.vue` and `server.js`.
2. **Check (2.2)**: read the files in `check.jsonl` (the frontend spec, the backend API contract) and dispatch `sunshine-check` to verify. Produces: the grep / curl verification results.
3. **Write back the spec (3.3)**: `sunshine-update-spec` writes the new conventions into `spec/` (leave form, leave API). Produces: the updated `frontend.md` and `backend.md`.
4. **Commit (3.4)**: one commit (code plus spec conventions). Produces: commit `31e6c36`.
5. **Wrap up**: `task finish` clears the pointer, `task archive` moves the task directory into `archive/2026-11/` and sets the status to `completed`. Produces: the archived task directory, and the parent at 1/2 done.

Only after requirement one is archived does requirement two start — the same five rounds (dialogue skipped; the commands and outputs are real values from runs):

> **Claude Code:** Requirement two is starting: dispatching `sunshine-implement` to change `LeaveList.vue` against the approval list page in the system prototype — the GET API already exists — then `sunshine-check` to verify.
>
> **Claude Code:** Implement is done: the approval list page is a six-column table with a 通过 (approve) and 驳回 (reject) button on each row; GET returns all records. Check passed too: six header columns are there, the approve button is there, GET returns 2 records, and the second one carries `reason` and `days`. I added an approval-list and a GET-API convention to the spec. Commit once?
>
> **You:** Commit it, and archive this sub-task.

Key output (real runs):

```sh
curl -s localhost:3000/api/v1/leaves
# {"code":"OK","data":[{"id":1,"type":"年假","start":"2026-11-10","end":"2026-11-11","reason":"休息","status":"待审批"},{"id":2,"applicant":"演示者","type":"年假","start":"2026-11-10","end":"2026-11-11","days":2,"reason":"休息","status":"待审批"}]}
```

The commit is `b3893e2 需求二：审批请假申请，审批列表页 + GET 接口` (requirement two: approve a leave request, approval list page plus GET API); after the archive the parent shows 2/2 done.

Finally the parent gets its final integration review — checked once against the acceptance criteria in the outline design, the two sub-tasks' check results, and the interface contract, without re-reading the whole prototype — and then it is archived:

> **Claude Code:** Integration review: the acceptance criteria all passed — the request page's three types and mandatory reason, all three API checks (400, `NEED_APPROVAL`, 200) and the GET list, and the records carry `reason` and `days`; the two sub-tasks' check results and the interface contract line up too. Archive the parent?
>
> **You:** Archive it.

**[Developers exit]**

```sh
sunshine task list
# 全部活动任务：
#
#   （没有活动任务）
#
# 总计：0 个任务

sunshine task list-archive
# 已归档任务：
#
# [2026-11] - 3 个任务
#   - 11-11-请假申请小工具/
#   - 11-11-需求一-提交请假申请/
#   - 11-11-需求二-审批请假申请/
```

The designer enters with only a requirement and exits leaving a complete design package; developers enter with the work already split per requirement and the context already curated, and exit with 0 active tasks and 3 archived.

**Picking the work back up after closing and reopening**

Close Claude Code and reopen it, and the conversation is gone — but the state is all in the files: the `status` in `task.json`, the pointer in `.sunshine/.current-task`, the `implement.jsonl` / `check.jsonl`, the `prd.md`, the `spec/`. The new session doesn't have to remember anything; it just runs `/sunshine:continue` — which first runs `sunshine task context` to see the current task, then routes by `status` to the matching step (2.1 implement, 2.2 check, 3.3 write back the spec, 3.4 commit).

Here is a real resumption (a separate minimal demo, only to verify the mechanism; a fresh shell with nothing shared from the previous session, reading files only):

```sh
sunshine task current
# .sunshine/tasks/11-11-接续演示任务

sunshine task context
# ## 当前任务
# 路径：.sunshine/tasks/11-11-接续演示任务
# 来源：file:.sunshine/.current-task
# 名字：接续演示任务
# 状态：in_progress
# 创建：2026-11-11
# 描述：演示关闭重开后怎么接上
#
# [!] 该任务有 prd.md，详情请读它
```

`.sunshine/.current-task` holds a single line — the task path:

```text
.sunshine/tasks/11-11-接续演示任务
```

Here is what resumption looks like in the session (the conversation is an example; the commands and outputs are real values from runs):

> **You:** (Next day, reopening Claude Code) Keep working on the leave-request tool.
>
> **Claude Code:** Let me check the current task first: `task context` says the current task is requirement one, status `in_progress`, implementation not started yet. I'll read the request page in the system prototype and the outline design from the context list and start at 2.1 implementing. OK?
>
> **You:** OK, go on.

Two things worth knowing, both hit for real:

- `task start` has a guard: an empty `check.jsonl` fails outright (`错误：check.jsonl 没有已整理条目`), so curate the context with `add-context` before `start`, or pass `--allow-empty-context`.
- The pointer lives in `.sunshine/.current-task`: `task finish` clears it, and `task archive` moves the task directory into the archive. An archived task is off the active list and can never be resumed — which is why a change goes through a new task.

**Requirement change: the cap moves from 10 days to 5 — the designer returns**

After the project is done and archived, the business asks for the annual-leave cap to move from 10 days to 5 days per request. Follow the requirement-change rule in workflow 1.5: the three archived tasks stay archived — no reopening; the change gets a new task. But the design package is the designer's deliverable, so changing it means the designer returns first — and the return is real work, not one line: three concrete edits, a review of only the changed part, and a handoff with receipts.

**[Designer returns]**

> **You:** The requirement changed: the annual-leave cap moves from 10 days to 5 days per request. Follow the Sunshine workflow: the designer returns to change the design package first, and implementation only starts after the review passes.
>
> **Claude Code:** The blast radius is requirement one only: requirement two is archived and unaffected (the list page doesn't cap the number of days) — no reopening; requirement one is archived too — no reopening either. Here's the plan: the designer returns first and changes three places — the requirement-one line in the requirements list, the interface line in the outline design, and the cap note on the request page in the system prototype; the review then covers only those three places, and the designer exits after it passes; then a new task "年假上限改成 5 天" records `[变更 2026-11-11]` in its PRD, the check in `server.js` moves to 5 days, the spec updates the same way, and curl verifies the 6-day and 5-day cases. Once you confirm, I'll start. OK?
>
> **You:** OK, go ahead.

Here are the three places the designer changed on the return (real file content, all 5 days after the change):

```text
doc/design/需求列表.md:9
- 年假单次最长 5 天，超限提交后返回 NEED_APPROVAL（走审批）。

doc/design/概要设计.md:16
- 年假单次最长 5 天，超限返回 `NEED_APPROVAL`。

doc/prototype/原型.html:12
<p>年假单次最长 5 天，超限走审批。</p>
```

(In English: the requirements list line — "Annual leave is at most 5 days per request; over the cap it returns NEED_APPROVAL for approval." The outline design line — "Annual leave is at most 5 days per request; over the cap it returns `NEED_APPROVAL`." The prototype line — "Annual leave is at most 5 days per request; over the cap it goes to approval.")

The prototype edit is a single line (the cap note); a change this small is made in the main session with a targeted grep, not a fresh `sunshine-prototype` dispatch — only structural changes get re-dispatched.

Then comes the requirements review — this time only the changed part is reviewed, not the whole design package again:

> **Claude Code:** For review, only the three changed places: the cap moved from 10 days to 5 in the requirements list, the outline design, and the system prototype, and nothing else changed a word; requirement two is unaffected. Once you confirm, the designer exits, I'll create the new task and curate its context, and developers enter. OK?
>
> **You:** OK.

**What to say at review: how the blast radius and the task list are decided**

The example above is a change after the project was finished and archived, so the review covers only the three changed places. A mid-development change works the same way, but the review covers one more thing — how the task list is adjusted. Say this at review:

Example prompt (an example, not a real conversation record):

> Review this change: first list the blast radius — which sub-tasks it touches, and whether each is archived, in progress, or not started; then list how the task list changes — which get a new rework sub-task, which get their scope and context edited in place, which get detached, which stay untouched; finally list the split plan. Only start after I confirm all three.

The blast radius is decided against two things — the parent task's `children` list in `task.json` (the sub-task map) and the design package in `doc/` (the requirements list, the outline design, the prototype pages). A sub-task is affected when the change touches its PRD, design, interface, or prototype page. Then handle each by category:

- **Archived**: never reopen. Create a new rework sub-task under the parent, run the full flow, and note the original task and the reason for the change in its description.
- **In progress**: pause the work, rework, then resume; the status stays as is.
- **Not started (`planning`)**: edit it in place — scope with `task set-scope`, context with `task add-context`.
- **Unaffected**: keep going, don't stop.
- **Obsolete**: detach it from the parent's `children` with `task remove-subtask`.

The split still follows requirements: one rework sub-task per requirement, splitting frontend and backend only when both are large. For example: requirement one is archived and requirement two is not started, and the change touches both — requirement one gets a new rework sub-task, requirement two gets its content edited in place; if requirement two is no longer needed, detach it. The adjusted list is confirmed with you at review before development resumes.

**The handoff: what the developer receives**

The review passes, and the designer exits. What developers receive on entry is not one line — it is three things:

1. **The updated design package**: the three places above. The files in `doc/` are the contract; development follows them.
2. **The review confirmation**: the confirmation of only the changed part. Nothing moves on until it is given.
3. **The new task's context lists**: the `implement.jsonl` and `check.jsonl` of the new task "年假上限改成 5 天". Developers entering read only the files on these two lists — they don't re-read the whole design package:

```json
implement.jsonl
{"file":"src/server.js","reason":"后端：上限 10 天改成 5 天"}
{"file":"doc/design/概要设计.md","reason":"概要设计：上限已改成 5 天"}

check.jsonl
{"file":".sunshine/spec/backend.md","reason":"接口契约"}
```

The PRD records it too (real content): `[变更 2026-11-11] 年假单次最长从 10 天改成 5 天（业务方要求）。设计人员已返场：需求列表、概要设计、系统原型都改成 5 天并评审通过。只影响需求一的后端校验：需求二已归档且不受影响，不 reopen；需求一已归档，不 reopen，变更走本任务。` (In English: "[Change 2026-11-11] The annual-leave cap moves from 10 days to 5 days per request (business request). The designer has returned: the requirements list, the outline design, and the system prototype all moved to 5 days and passed review. It affects only requirement one's backend check: requirement two is archived and unaffected — no reopening; requirement one is archived too — no reopening; the change goes through this task.")

**[Designer exits, developers enter]**

Developers enter and implement against the new task's context: the check in `server.js` moves to 5 days, the spec updates the same way, and curl verifies both cases.

Key output (real runs): 6 days returns `{"code":"NEED_APPROVAL","data":null}`, 5 days returns 200 with record `id` 3 and `days` 5; the commit is `add1c30 变更：年假上限改成 5 天（10→5）` (change: annual-leave cap moves to 5 days, 10 → 5); after the archive there are 4 tasks in total. The three old tasks sit untouched in the archive, and the change is its own task with a complete record — next time someone asks why the cap is 5 days, the `[变更 2026-11-11]` line in `prd.md` is the answer.

**The full loop at a glance**

The whole scenario runs as one closed loop, with every entry, exit, and handoff spelled out above:

1. **State the requirement** (you): state what to build in Claude Code.
2. **Clarify the requirement** (the designer enters): the agent asks first, pinning down the rules and the prototype details.
3. **The design package** (the designer): write the requirements list first, then the outline design, then dispatch `sunshine-prototype` to build the system prototype — one page per requirement in the list, with mock data (the main session only takes the report, never writes the prototype itself). The prototype is the end of the design phase. Handoff: the three design documents in `doc/`.
4. **The requirements review** (you confirm): review the whole package (the design package plus the split plan); the designer exits after it passes. Handoff: the approved design package.
5. **The split** (the agent): split the whole scope in one pass, per requirement (one requirement is one sub-task by default; split frontend and backend into two sub-tasks when both are large, with the interface contract in the outline design as the handoff); the split starts nothing. Handoff: the task directories (`task.json` parent/children, `prd.md`, `implement.jsonl`, `check.jsonl`).
6. **Development** (developers enter): activate the sub-tasks one by one, five rounds each (implement → check → write back the spec → commit → archive), each round reading only its own context list. Handoff: the changed code, the updated spec, the commit.
7. **Integration review and archive** (the parent task): once all sub-tasks are archived, the parent reviews once against the acceptance criteria, the two sub-tasks' check results, and the interface contract (without re-reading the whole prototype), then archives. Handoff: the archived task directories.
8. **Requirement change** (the designer returns): change the design package → review only the changed part → handoff (the updated design package, the review confirmation, the new task's context lists, the `[变更]` record in the PRD) → developers enter on a new task. Archived tasks are never reopened.
9. **Picking the work back up**: the state lives entirely in files (`task.json` status, the `.current-task` pointer, the jsonl lists, `prd.md`, `spec/`); a new session runs `/sunshine:continue` and routes by status to the matching step.

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
