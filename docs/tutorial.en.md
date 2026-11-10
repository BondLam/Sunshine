# Sunshine tutorial: init once, then work in the agent UI

The main flow in one line: **run `sunshine init` once in the terminal, then do everything else by talking to the agent in its UI (Claude Code first, Codex / Cursor the same idea) in plain language — the agent drives the work following `.sunshine/workflow.md` and calls `sunshine` commands under the hood; you review the diff, accept, and the work is committed.**

That is the whole point of this tutorial: the terminal is not where you spend your day. You type one setup command, then you talk. The `sunshine task …` / `sunshine channel …` commands still exist, but they are the agent's tools, not your daily interface — they appear in this tutorial only in [section 4](#4-what-happened-under-the-hood) (what the agent actually ran, with real outputs), in [section 12](#12-wrap-up-manual-intervention-cheat-sheet) (when you step in manually), and in [section 13](#13-five-scenarios-from-day-one-to-daily-work) (one walkthrough per scenario).

The demo project is the same throughout: **refund reason required** — add a mandatory reason field to the refund flow. The example is small; the workflow is real.

Conventions:

- Dialogue with the agent is shown as **example prompts**, labeled as such. They are illustrations of what to say, not records of a real session.
- Every command output shown is a real excerpt from a run made for this tutorial (`……` marks omitted lines, not rewritten ones). The `11-10` prefix in task directory names is the month-day of the day the commands were run (today, 2026-11-10).
- This tutorial was verified with v0.2.1 (the current Release). The two places that show a different version say so explicitly.
- **The CLI itself is Chinese-only.** Commands and sample output below are kept in the original Chinese so you can copy them verbatim; the explanations around them are in English.
- The demo directories all live under `/tmp/rewrite-en/` (`demo`, `demo2`, `demo3`, `registry-src`, `channels`, `home`) — delete them and nothing in any real project is affected.

## 1. Run once in the terminal: init

Install Sunshine first (see [README.en.md](../README.en.md#installation)), then confirm it:

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

Project type is auto-detected at init: `init` reads `package.json` dependencies and common frontend/backend config files (`vite.config.ts`, `go.mod`, etc.) and generates only the matching specs — a frontend project gets `system.md` and `frontend.md`, a backend project gets `system.md` and `backend.md`, a fullstack project gets all three. The demo directory is empty, so nothing is detected; with `-y` skipping the prompt it defaults to fullstack, which is why the output above says `项目类型：全栈（默认）` ("Project type: fullstack (default)"). Without `-y`, an undetected type triggers one question (frontend / backend / fullstack); to specify it explicitly use `--type 前端` (or 后端 / 全栈). With `--registry` (specs from the template marketplace) there is no detection and no question. Without `-y` and without `--template`, init asks one more question after the platform selection — a spec template picker ("请选择规范模板"): "从零开始" (from scratch, the default), or "自定义" (enter a marketplace source and pick from it); with `--registry` the picker lists that marketplace's spec templates, and the picked one replaces the blank specs.

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

From here on, close the terminal. The rest of this tutorial happens in the agent UI.

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

(`system.md` is the spec the agent itself maintains in the background; see [section 6](#6-how-specs-are-maintained`).)

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

**Phase 2 — execution.** The agent dispatches sub-agents: `sunshine-implement` writes the code, then `sunshine-check` verifies it. The platform hooks inject the context automatically — the `implement.jsonl` / `check.jsonl` entries, plus `prd.md`, `design.md`, and `implement.md` if they exist — so each sub-agent starts with the right specs in its prompt. The dispatch prompt always opens with `Active task: <task path>`. The fixed order in `workflow.md` is:

```text
sunshine-implement -> sunshine-check -> sunshine-update-spec -> 提交（第 3.4 步）-> /sunshine:finish-work
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
下载地址：https://github.com/linrong1982/Sunshine/releases/download/v0.2.1/sunshine_0.2.1_linux_amd64.tar.gz
备份：将把当前二进制备份为 /tmp/rewrite-en/sunshine.bak
[试运行] 未做任何修改。
```

("Upgrading Sunshine: 0.1.0 → 0.2.1 (channel latest); asset: `sunshine_0.2.1_linux_amd64.tar.gz`; download URL: …; backup: the current binary will be backed up as `/tmp/rewrite-en/sunshine.bak`; [dry run] no changes made.")

A real upgrade (drop `--dry-run`) follows the same flow: check the latest version → pick the asset for the current OS and architecture → download → verify sha256 against `checksums.txt` → back up the current binary (a `.bak` next to it) → replace → run the new `--version` to verify, rolling back to the backup and reporting an error on failure. You can also pin a version or channel: `sunshine upgrade --tag 0.2.1`, `sunshine upgrade --tag beta`.

## 12. Wrap-up: manual-intervention cheat sheet

That is the whole mainline: you ran `init` once, asked for the refund validation in plain language in the agent UI, the agent planned, implemented, checked, wrote the lessons back into the spec, and finished with `/sunshine:finish-work`; you reviewed the diff and accepted; the task is archived and the session is in the journal.

Everything else in Sunshine is something you reach for when you step in manually. These are the only commands you ever need to type yourself:

| When | Command | What it does |
| --- | --- | --- |
| Where did we get to? | `sunshine task list` / `sunshine task current` / `sunshine task context` | Show active tasks, the current task, and the full session context |
| Check the next step | `sunshine task list-archive` | Show archived tasks by month |
| Switch how the agent works | `sunshine workflow --list` / `sunshine workflow --template tdd` | List or switch workflow templates ([section 7](#7-switching-workflows)) |
| Refresh project templates | `sunshine update --dry-run` (drop `--dry-run` to apply) | Sync template files; user data is never touched ([section 8](#8-update-and-the-template-marketplace)) |
| Install a spec from a marketplace | `sunshine init --registry <source> --template <id>` | Install a spec template, refreshed by `update` afterwards ([section 8](#8-update-and-the-template-marketplace)); in interactive mode, omit `--template` to pick from a list |
| Look back | `sunshine mem search "退款"` / `sunshine mem list` | Search local session history, read-only ([section 9](#9-mem-look-back-at-sessions)) |
| Multi-agent work | `sunshine channel list` / `sunshine channel messages <name>` | Inspect channels and their event timelines ([section 10](#10-channel-multi-agent-collaboration)) |
| Update the CLI itself | `sunshine upgrade --dry-run` (drop `--dry-run` to apply) | Upgrade the `sunshine` binary ([section 11](#11-upgrade-update-the-cli-itself)) |
| See what is configured | `sunshine platforms` | Show the platforms configured for this project |
| Remove Sunshine from a project | `sunshine uninstall` / `sunshine ablate` / `sunshine restore` | Delete all Sunshine files, temporarily remove them (reversible), or restore exactly what `ablate` saved |

Everything not listed here — `task create/start/archive`, `task add-context/validate`, `channel create/send/spawn/wait`, `task add-session` — is the agent's job, driven by `workflow.md`. If you find yourself typing those by hand every day, something is wrong: open the agent UI and let it drive.

## 13. Five scenarios: from day one to daily work

Sections 1–12 cover the main flow; this section walks through five real scenarios, each one a complete end-to-end run:

| Scenario | When it applies | The point |
| --- | --- | --- |
| 1. Starting a new project from zero | A new product, service, package, or internal tool | Empty directory, `init`, first task grows from nothing |
| 2. Adopting an existing project | The repo exists, but the conventions live mostly in people's heads | `init` touches no existing code; land the conventions in spec first |
| 3. Delivering a product feature | One task spans product, API, data, and UI | One task + cross-layer context + channel coordination |
| 4. Refactoring a legacy module | The code runs, but it is no longer safe to change | Pin the behavior with tests first, then refactor, tests green throughout |
| 5. Fixing a bug that keeps coming back | The same class of problem appears again and again | `mem` finds the last session first, then a task, then the lesson goes into spec |

This section was verified with **v0.3.0** (the current Release; `sunshine version` prints `Sunshine 版本 0.3.0`). The demo directories all live under `/tmp/scenes/` (`s1-new`, `s2-old`, `s3-cross`, `s4-refactor`, `s5-bug`) — delete them and nothing in any real project is affected. The `11-10` prefix in task directory names is the month-day of the day the commands were run (2026-11-10). Command outputs are real excerpts from runs made for this tutorial (`……` marks omitted lines, not rewritten ones); prompts shown to the agent are labeled as example prompts, not records of a real session. The CLI itself is Chinese-only, so commands and sample outputs below are kept in the original Chinese; the explanations around them are in English.

### 13.1 Starting a new project from zero: a duty-roster tool

An empty directory, nothing in it — the cleanest starting point. Create the directory, put it in git, then `init`:

```sh
mkdir -p /tmp/scenes/s1-new && cd /tmp/scenes/s1-new && git init
sunshine init -u 演示者 -y
```

Real output:

```text
初始化完成，开发者：演示者
项目类型：全栈（默认）
已安装平台：
  - Claude Code
  - Cursor
下一步：
  1. 在 agent 界面里打开项目，直接用自然语言提第一个需求；
  2. 规范已按默认模板生成在 .sunshine/spec/（已生成 system.md、frontend.md、backend.md；有团队规范时再补充），工作流按 .sunshine/workflow.md 自动推进，一般不用改。
```

(In English: "Initialization complete, developer: 演示者 / Project type: 全栈 (default) / Installed platforms: Claude Code, Cursor / Next steps: 1. Open the project in the agent UI and state your first requirement in plain language; 2. The specs have been generated in .sunshine/spec/ from the default templates (generated: system.md, frontend.md, backend.md; add your team's specs if you have them); the workflow runs automatically following .sunshine/workflow.md and normally needs no changes.")

An empty directory has no detectable type, so `-y` defaults to full-stack and all three specs are generated — a new project starts with a default set of specs, and the team adjusts them later, which beats starting with nothing.

State the first requirement in the agent UI:

Example prompt (an example, not a record of a real session):

> I'm starting a duty-roster tool from scratch: generate this week's roster from a CSV, first column date, second column the person on duty. Please follow the Sunshine workflow: ask me first to pin down the missing product logic (date formats, output form) and technical decisions (language, how to run it); once confirmed, create the first task, write the PRD, then implement.

Under the hood the agent creates the task, lines up context, and starts it (same commands as section 4; only the key outputs are shown here):

```sh
sunshine task create "值班表小工具第一版" --description "按 CSV 生成本周值班表" --priority P2
```

```text
……
  - 用 /sunshine:continue 或阶段上下文决定下一步

.sunshine/tasks/11-10-值班表小工具第一版
```

```sh
sunshine task add-context 11-10-值班表小工具第一版 implement .sunshine/spec/system.md "实现前先读总原则"
# 已添加文件：.sunshine/spec/system.md

sunshine task start 11-10-值班表小工具第一版 --allow-empty-context
# ✓ 状态：planning → in_progress（降级）
```

The implementation is the agent's job — here is the program it actually wrote (a small tool that reads a CSV and prints the roster) and its real run result:

```sh
go mod init duty
go run ./cmd/duty 值班表.csv
```

```text
11-10：张三
11-11：李四
11-12：王五
```

Wrap-up is the same as section 4: `task finish` clears the pointer, `task archive` archives it, `task list-archive` confirms:

```text
[2026-11] - 1 个任务
  - 11-10-值班表小工具第一版/
```

After the first task, the project has three things: code that runs, an archived task record, and specs generated from the default templates. From the second task on, the agent has something to work from.

### 13.2 Adopting an existing project: land the conventions in spec first

The repo exists, but the conventions live mostly in people's heads — the most common adoption case. The demo is a refund API that has run for two years (`express` is the only dependency in `package.json`, one file `src/server.js`), with the team's conventions written in `README.md` and nowhere else:

```markdown
# refund-api

团队约定（一直靠人记，没落进任何文档）：
- 接口统一返回 { code, data }，code 是大写下划线错误码。
- 所有写操作必须记审计日志。
- 端口固定 3000，改端口要全组同步。
```

(In English: "Team conventions (always remembered by people, never written into any document): APIs return { code, data } with code an UPPER_SNAKE error code. Every write must write an audit log. The port is fixed at 3000; changing it needs the whole team to sync.")

Run `init` right in this directory — `init` only adds files, it never changes existing code:

```sh
cd /tmp/scenes/s2-old
sunshine init -u 演示者 -y
```

Real output (key line):

```text
项目类型：后端（自动检测）
```

Detection only looks at the dependencies themselves: this project has only `express`, so it is a backend project, and only `system.md` and `backend.md` are generated (`ls .sunshine/spec/` shows just those two); a pure Express backend is not misjudged as full-stack. Existing files are untouched — `git status` shows only the new Sunshine directories:

```text
?? .claude/
?? .cursor/
?? .sunshine/
```

The first task on an adopted project is not a feature — it is landing the remembered conventions in spec, so the agent stops having to guess them every time. One sentence in the agent UI is enough:

Example prompt (an example, not a record of a real session):

> I'm bringing an existing project into Sunshine: the repo exists, but the conventions live mostly in people's heads, and the README has three team conventions. Please follow the Sunshine workflow: ask me first to confirm the three conventions and which spec files they belong in; once confirmed, create the task, write the PRD, then write the conventions into .sunshine/spec/ so every task from now on follows them.

Under the hood the agent creates the task and writes the conventions into the matching spec files (this is what actually landed):

```sh
sunshine task create "把现有约定落进规范" --description "把 README 里靠人记的三条约定写进 .sunshine/spec" --priority P2
```

```markdown
## 退款接口（团队已有约定）

- 接口统一返回 `{ code, data }`，code 是大写下划线错误码。
- 所有写操作必须记审计日志。
```

```markdown
## 团队约定

- 服务端口固定 3000，改端口要全组同步。
```

The task runs its normal course (`start` → `finish` → `archive --skip-branch-validation`), and what landed is verifiable with `grep`:

```sh
grep -c "审计日志" .sunshine/spec/backend.md
# 1
```

From now on the conventions no longer live only in senior engineers' heads: the agent on a new task reads the spec, and the conventions survive people leaving. The acceptance bar for adopting an existing project is one line — **after `init`, `git status` shows no modified old files, and spec has one more convention that used to live only in memory**.

### 13.3 Delivering a product feature: one task across product, API, data, and UI

When one task spans four layers, the usual failure is changing one layer and missing another — Sunshine's answer is one task whose context list names the files in all four layers at once, so acceptance checks against the same list. The demo is a refund app (`src/Refund.vue` is the UI, `src/server.js` the API, `data/refunds.json` the data), and the task is making the refund reason mandatory.

State the requirement in the agent UI:

Example prompt (an example, not a record of a real session):

> I need to deliver a product feature: make the refund reason mandatory, one task across the product, API, data, and UI layers (the UI is src/Refund.vue, the API is src/server.js, the data is data/refunds.json). Please follow the Sunshine workflow: ask me first to pin down the unclear validation rules (the reason options, the error code when the field is missing) and the order to change the four layers; once confirmed, create the task, write the PRD, then implement.

Under the hood the agent creates the task (same commands as section 4; only the key outputs are shown here):

```sh
sunshine task create "退款原因必填校验（全链路）" --description "产品、API、数据、UI 四层一起改：原因必填" --priority P1
```

Line up the context by layer — the three layers to implement go into `implement`, the spec to check against goes into `check`:

```sh
sunshine task add-context 11-10-退款原因必填校验-全链路 implement src/Refund.vue "UI：原因下拉框与必填校验"
# 已添加文件：src/Refund.vue

sunshine task add-context 11-10-退款原因必填校验-全链路 implement src/server.js "API：缺 reason 返回 400"
# 已添加文件：src/server.js

sunshine task add-context 11-10-退款原因必填校验-全链路 implement data/refunds.json "数据：退款记录加 reason 字段"
# 已添加文件：data/refunds.json

sunshine task add-context 11-10-退款原因必填校验-全链路 check .sunshine/spec/backend.md "接口契约"
# 已添加文件：.sunshine/spec/backend.md

sunshine task list-context 11-10-退款原因必填校验-全链路
# [implement.jsonl]
#   1. src/Refund.vue
#   2. src/server.js
#   3. data/refunds.json
# [check.jsonl]
#   1. .sunshine/spec/backend.md

sunshine task start 11-10-退款原因必填校验-全链路
# ✓ 状态：planning → in_progress（降级）
```

The four layers have to change in order (API first, then UI, data fields following), so say the order in a channel. Switch the workflow to channel-driven dispatch first, then create the channel and send two coordination messages:

```sh
sunshine workflow --template channel-driven-subagent-dispatch
#   ✓ 已把 .sunshine/workflow.md 换成 "channel-driven-subagent-dispatch"

sunshine channel create refund-reason --type chat --task .sunshine/tasks/11-10-退款原因必填校验-全链路
# 已创建 channel 'refund-reason'（chat）位于 /tmp/scenes/s3-cross/.channels/-tmp-scenes-s3-cross/refund-reason

sunshine channel send refund-reason --to impl "先改 API：POST /refunds 缺 reason 返回 400，错误码 REASON_REQUIRED"
# {"by":"main","kind":"message","origin":"cli","seq":2,"text":"先改 API：POST /refunds 缺 reason 返回 400，错误码 REASON_REQUIRED","to":"impl","ts":"2026-11-10T05:04:00.121817199Z"}

sunshine channel send refund-reason --to impl "API 改完再改 UI：原因下拉框三项，选其他出文本框"
# {"by":"main","kind":"message","origin":"cli","seq":3,"text":"API 改完再改 UI：原因下拉框三项，选其他出文本框","to":"impl","ts":"2026-11-10T05:04:00.128921565Z"}

sunshine channel messages refund-reason
# [1] 05:04:00 [create] by=main cwd=/tmp/scenes/s3-cross 任务=.sunshine/tasks/11-10-退款原因必填校验-全链路
# [2] 05:04:00 [message] by=main to=impl
#          先改 API：POST /refunds 缺 reason 返回 400，错误码 REASON_REQUIRED
# [3] 05:04:00 [message] by=main to=impl
#          API 改完再改 UI：原因下拉框三项，选其他出文本框
```

`--task` binds the channel to the task, and the task path shows in `messages`; `send` posts a numbered JSON event, not a casual note. Waiting for the worker's `done` times out — with no worker replying, a timeout is the normal result (section 10), and `spawn` needs a logged-in claude / codex; this section only coordinates as far as sending messages:

```sh
sunshine channel wait refund-reason --from impl --kind done --timeout 2s
# 错误：超时：等待 channel 'refund-reason' 超时（2s），仍在等待：impl（上游退出码 124）
```

Wrap up by switching the workflow back to native (`workflow.md` has changed, so `--force` is needed) and archiving the task as usual:

```sh
sunshine workflow --template native --force
#   ✓ 已把 .sunshine/workflow.md 换成 "native"
```

The acceptance bar for a cross-layer task is in the same list: all four lines in `list-context` done, and the one spec in `check` verified line by line — whichever layer is missed shows up on the list.

### 13.4 Refactoring a legacy module: pin the behavior first, then change the code

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

State the requirement in the agent UI:

Example prompt (an example, not a record of a real session):

> I need to refactor a legacy module: the price function Price, with all the discount rules hard-coded in `if`s — the code runs, but it is no longer safe to change. Please follow the Sunshine workflow (the TDD template this time): ask me first to confirm the behavior to pin down (the expected prices for each quantity tier and VIP) and the boundaries of the refactor; once confirmed, create the task, write the PRD, then follow TDD: write characterization tests to pin the current behavior first, then refactor.

Under the hood the agent switches the workflow, creates the task, and starts it:

```sh
sunshine workflow --template tdd
#   ✓ 已把 .sunshine/workflow.md 换成 "tdd"

sunshine task create "重构价格计算" --description "把折扣规则从 Price 里抽出来，行为不变" --priority P2

sunshine task start 11-10-重构价格计算 --allow-empty-context
# ✓ 状态：planning → in_progress（降级）
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

```sh
go test ./...
# ok  	shop	0.003s
```

All green is the baseline — this step changed no business code at all, it only proves "the current behavior is exactly these three cases". Only then does the refactor happen: discount rules become constants and two small functions, and `Price` only composes them:

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

Run the same tests once more after the refactor:

```sh
go test ./...
# ok  	shop	0.002s
```

Still all green — the behavior did not change, so the refactor stands. The red light in the TDD workflow is for new behavior; a pure refactor runs the other way around: **write characterization tests to pin the behavior (green), then refactor (still green)**. Wrap up by writing the new convention into spec, so the next change starts from the test table:

```markdown
## 价格计算

- 折扣规则集中在 calc.go 的常量与 applyBulk / applyVIP，Price 只做组合；改规则先改 calc_test.go 的用例表。
```

(In English: "Price calculation: discount rules live in the constants and applyBulk / applyVIP in calc.go; Price only composes them. Change a rule by changing the case table in calc_test.go first.")

Archive the task as usual and switch the workflow back to native (`--force`, same reason as in 13.3).

### 13.5 Fixing a bug that keeps coming back: find the last session first

When the same class of problem appears again and again, the most expensive part is investigating it from zero every time. The bug in this section really happened: a codex worker failing to connect with a 401 — once on November 9, again in the early hours of November 10, and once more today.

State the requirement in the agent UI:

Example prompt (an example, not a record of a real session):

> I need to fix a bug that keeps coming back: a codex worker failing to connect with a 401 — once on November 9, again in the early hours of November 10, and once more today. Please follow the Sunshine workflow: use mem to find the last related session first, then ask me to confirm whether this is the same class of problem and what the reproduction steps are; once confirmed, create the task, write the PRD, and write the lesson into the spec after the fix.

Under the hood the agent first reproduces one occurrence as it happened (a real run):

```sh
sunshine channel create codex-401 --type chat
# 已创建 channel 'codex-401'（chat）位于 /tmp/scenes/s5-bug/.channels/-tmp-scenes-s5-bug/codex-401

sunshine channel send codex-401 --to worker "线上 codex worker 又连不上了，之前报过 401，帮我查一下这次的原因"
# {"by":"main","kind":"message","origin":"cli","seq":2,"text":"线上 codex worker 又连不上了，之前报过 401，帮我查一下这次的原因","to":"worker","ts":"2026-11-10T05:04:28.735716296Z"}

sunshine channel spawn codex-401 --agent implement --provider codex --as worker
# {
#   "pid": 4330,
#   "log": "/tmp/scenes/s5-bug/.channels/-tmp-scenes-s5-bug/codex-401/worker.log",
#   "worker": "worker"
# }
```

`--agent` takes a definition that already exists in `.sunshine/agents/` (here `implement`, not a made-up name — a missing one fails outright: `错误：找不到 agent 定义：.sunshine/agents/worker.md`). The worker started, but then failed in its log (codex is not logged in on this machine, the same precondition section 10 states):

```text
unexpected status 401 Unauthorized: Missing bearer <redacted> basic authentication in header, url: https://api.openai.com/v1/responses
```

The worker never answered, but the session is on disk — `mem` can find this dispatch again:

```sh
sunshine mem list --platform codex
# 范围：项目=/tmp/scenes/s5-bug  平台=codex
# [codex   ] 2026-11-10 05:05  01a1c3d8-457  /tmp/scenes/s5-bug
#
# 共 1 个会话

sunshine mem search 401 --platform codex
# 范围：项目=/tmp/scenes/s5-bug  平台=codex  关键词="401"
#
# [codex   ] 2026-11-10 05:05  01a1c3d8-457  /tmp/scenes/s5-bug  相关度=1.000  命中=1（用户=1，助手=0）  轮次=1
#     [用户] 线上 codex worker 又连不上了，之前报过 401，帮我查一下这次的原因
#
# 共 1 个会话

sunshine mem context 01a1c3d8 --platform codex
# # 上下文：[codex] 01a1c3d8-457b-7cc1-9fc3-1573da08c578
# # 工作目录：/tmp/scenes/s5-bug
# # 未指定关键词 — 显示前 1 轮，共 1 轮
# # 显示轮次：1  已用预算：41/6000 字符
# #
# ## 第 0 轮（用户） ← 命中
# #
# 线上 codex worker 又连不上了，之前报过 401，帮我查一下这次的原因
```

One boundary found by actually running this, worth remembering: **`mem` searches the words in dialogue turns (user / assistant); it does not search error events**. The session from the early hours of November 10 (`01a1c376`, in `/tmp/h-test/chanproj`) had a worker fail with the same 401, but its 401 only appears in an error event, not in any dialogue turn — so searching `401` does not find it. What you search for is the sentence that was sent to the worker at the time. This run is findable because the message sent to the worker already says 401.

Once the last session confirms it is the same class of problem, create the task — with the session id in the description, so the agent does not have to ask "what happened last time" all over again:

```sh
sunshine task create "codex worker 401：spawn 前先确认登录状态" --description "codex worker 再次报 401（会话 01a1c3d8）：spawn 前先确认登录状态，失败时看 worker.log" --priority P1
```

When the task is done, the lesson goes into spec — otherwise it will happen the same way next time, which is the actual fix for a recurring bug:

```markdown
## 多 Agent 派发

- 派发 codex worker 前先确认本机 codex 已登录；spawn 失败时先看 worker.log（401 即未登录），不要反复重派。
```

(In English: "Multi-agent dispatch: before spawning a codex worker, confirm codex is logged in on this machine; when a spawn fails, read worker.log first (401 means not logged in) instead of spawning again and again.")

Archive the task as usual. Next time a 401 shows up, the agent reads the spec and checks the login first, instead of spawning another worker into the same wall.

Where to go next:

- [README.en.md](../README.en.md): the doorway — installation, quick start, core commands at a glance, configuration essentials.
- [PLAN.md](../PLAN.md): the full plan and per-stage implementation notes, including the design decisions behind the commands this tutorial did not expand on (`hook`, `platforms`, `uninstall`, `ablate`, `restore`).
- [Releases](https://github.com/linrong1982/Sunshine/releases): downloads and release notes for each version.
