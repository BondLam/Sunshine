# Sunshine tutorial: from project setup to multi-agent collaboration, one storyline end to end

This tutorial follows one demo project through the whole Sunshine workflow: **init a project → write specs (three spec files) → split work with task → execute and switch workflows → update and registry (template marketplace) → review sessions with mem → multi-agent collaboration with channel → upgrade**.

Conventions:

- The demo project is "refund reason required": add a mandatory reason field to a refund flow. The example is small; the workflow is real.
- Every command in this tutorial was actually run in a temporary directory, and the sample outputs are excerpts of real output (`……` marks omitted lines, not rewritten ones).
- The `11-09` prefix in task directory names is the month-day of the day you run it.
- Version numbers in the output depend on the CLI version you installed. This tutorial was verified with v0.2.0 (the current Release); the few places that show a different version say so explicitly.
- **The CLI itself is Chinese-only.** Commands and sample output below are kept in the original Chinese so you can copy them verbatim; the explanations around them are in English.

First confirm the installation (see [README.en.md](../README.en.md) for how to install):

```sh
sunshine version
# Sunshine 版本 0.2.0

sunshine --help
# Sunshine 把规范、任务与项目记忆持久化进仓库，
# 让 AI 编程助手按团队的工程标准工作，而不是每次从零猜测。
# ……
# 可用命令：
#   ablate      临时移除项目全部 Sunshine 表面（可逆）
#   channel     多 Agent 协同：通过共享事件日志派生、协调与打断 worker
#   help        显示命令帮助信息
#   hook        供各平台 hook 调用的子命令（替代 Python hooks）
#   init        初始化当前项目，生成 .sunshine/ 工程骨架
#   mem         跨平台检索 AI 会话历史
#   platforms   显示当前项目已配置的 AI 平台
#   restore     精确恢复 ablate 保存的 Sunshine 状态
#   task        任务管理：创建、启动、完成、归档任务
#   uninstall   删除项目内全部 Sunshine 文件
#   update      更新项目内的 Sunshine 模板文件
#   upgrade     升级 Sunshine CLI 自身到最新版
#   version     显示 Sunshine 版本
#   workflow    查看或切换 .sunshine/workflow.md 的工作流模板
```

## 1. init a project

Create a demo project directory and initialize it:

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
已安装平台：
  - Claude Code
  - Cursor
下一步：
  1. 编辑 .sunshine/spec/，填入团队已有的技术规范与业务术语；
  2. 按 .sunshine/workflow.md 开始第一个任务。
```

(`-u` is the developer name; `-y` skips all prompts and uses the defaults. In English: "Configuring Claude Code… / Configuring Cursor… / Initialization complete, developer: 演示者 / Installed platforms: Claude Code, Cursor / Next steps: 1. Edit .sunshine/spec/ and fill in the team's existing technical specs and business terms; 2. Start the first task following .sunshine/workflow.md.")

By default `init` installs Claude Code and Cursor. To install only Codex, use `sunshine init --codex -y`; running `init --codex -y` again in an already initialized project only adds the new platform and leaves existing files untouched (verified: `.codex/` appears and `platforms` then lists all three).

`init` generated 115 files. The skeleton:

```sh
ls .sunshine/spec/
# backend.md  frontend.md  system.md

sunshine platforms
# 已配置的平台：
#   Claude Code (claude-code) — .claude
#   Cursor (cursor) — .cursor
```

- `.sunshine/spec/`: three spec templates (general principles, backend, frontend) — step 2 fills in your own conventions.
- `.sunshine/workflow.md`: the full development workflow (714 lines) that the AI assistant follows to drive tasks.
- `.sunshine/config.yaml`: project config with Chinese comments (key fields: see [README.en.md](../README.en.md#configuration)).
- `.claude/` and `.cursor/`: skills, commands, sub-agents, and hook configs for the two platforms.

## 2. Write the specs (three files)

Specs are the team's engineering standards: write them once, and every later task follows them automatically. Add one convention of the demo project to each of the three files:

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

(In English: the system spec gains "Demo project convention: the refund reason is required — submitting a refund with an empty reason is rejected"; the backend spec gains "`POST /refunds`: `reason` is required, missing it returns 400 with error code `REASON_REQUIRED`"; the frontend spec gains "The reason input is a dropdown (quality issue / wrong size / other); choosing 'other' reveals a text box.")

Specs are user data: `sunshine update` never touches `.sunshine/spec/` (in step 5's scan output it appears under "用户数据（保持不动）", "user data (kept untouched)"). Specs can also be maintained autonomously by the AI assistant in the background, with a human only reviewing the result — that is what `system.md` states at the top.

## 3. Split work with task

### Create a task

`--description` is required (omitting it fails immediately with `错误：--description 必填`, "error: --description is required"):

```sh
sunshine task create "给退款原因加必填校验" --description "退款时原因字段必填" --priority P1
```

Real output:

```text
警告：无法解析仓库默认分支（未配 remote 或离线），base_branch 暂记为当前检出分支 "master"，可用 --base-branch 覆盖。
已创建任务：11-09-给退款原因加必填校验

下一步：
  - 在 prd.md 里写清需求与验收标准
  - 轻量任务：只有 PRD 也可以
  - 复杂任务：在 sunshine task start 前补充 design.md 与 implement.md
  - 整理 implement.jsonl / check.jsonl（已建空文件），供子代理在 start 前读规范：
      每行一个 JSON 对象 {"file": "<路径>", "reason": "<原因>"}，只放规范/调研文档
      可用 sunshine task context --mode packages 列出可用规范
  - 用 /sunshine:continue 或阶段上下文决定下一步

.sunshine/tasks/11-09-给退款原因加必填校验
```

(The warning says the default branch could not be resolved — no remote or offline — so `base_branch` is recorded as the current checkout branch `master` for now, overridable with `--base-branch`. The "next steps" say: write requirements and acceptance criteria in prd.md; a light task can have only a PRD; a complex task should add design.md and implement.md before `task start`; organize implement.jsonl / check.jsonl — one JSON object per line, `{"file": "<path>", "reason": "<reason>"}`, spec/research docs only.)

Task directories are `.sunshine/tasks/<MM-DD-slug>/`, and `task.json` inside has 24 fields in a fixed order with fixed defaults:

```json
{
  "id": "给退款原因加必填校验",
  "name": "给退款原因加必填校验",
  "title": "给退款原因加必填校验",
  "description": "退款时原因字段必填",
  "status": "planning",
  "priority": "P1",
  "creator": "演示者",
  "assignee": "演示者",
  "createdAt": "2026-11-09",
  "completedAt": null,
  "branch": null,
  "base_branch": "master",
  "subtasks": [],
  "children": [],
  "parent": null,
  "relatedFiles": [],
  "notes": "",
  "meta": {}
}
```

(Only the key fields are shown above; `dev_type`, `scope`, `package`, `worktree_path`, `commit`, `pr_url`, and similar are `null` when empty.)

Note: `create` tries to auto-activate the task by default, but in a plain terminal there is no session identity, so activation is silently skipped — after `create`, `task current` is empty (verified). That is why the next step is an explicit `start`.

### Organize the context list

Prepare a real file first, then register it in the context list (`implement` = files to read while implementing; `check` = files to check against while verifying):

```sh
mkdir -p src
cat > src/Refund.vue <<'EOF'
<template><select v-model="reason"></select></template>
EOF

sunshine task add-context 11-09-给退款原因加必填校验 implement src/Refund.vue "实现退款原因必填校验"
# 已添加文件：src/Refund.vue

sunshine task add-context 11-09-给退款原因加必填校验 check .sunshine/spec/backend.md "退款接口契约"
# 已添加文件：.sunshine/spec/backend.md

sunshine task list-context 11-09-给退款原因加必填校验
# === 上下文文件 ===
#
# [implement.jsonl]
#   1. src/Refund.vue
#      → 实现退款原因必填校验
#
# [check.jsonl]
#   1. .sunshine/spec/backend.md
#      → 退款接口契约

sunshine task validate 11-09-给退款原因加必填校验
# === 校验上下文文件 ===
# 目标目录：/tmp/sunshine-docs/demo/.sunshine/tasks/11-09-给退款原因加必填校验
#
#   implement.jsonl：✓（1 条）
#   check.jsonl：✓（1 条）
#
# ✓ 全部校验通过
```

("已添加文件" = "file added"; "上下文文件" = "context files"; "校验上下文文件" = "validating context files"; "全部校验通过" = "all checks passed".)

### Start, view, finish

```sh
sunshine task start 11-09-给退款原因加必填校验
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

("Session identity unavailable; the current-task pointer is not persisted this time (degraded mode), the AI can still continue from the conversation context. Tip: run inside an AI session that provides a session identity, or set SUNSHINE_CONTEXT_ID before running sunshine task start. Status: planning → in_progress (degraded). Recorded branch: master (degraded). Warning: 'master' is also the task's base_branch; a PR cannot point at its own branch, archiving will refuse. After checking out a branch, run: sunshine task set-branch <task> <feature branch>.")

"Degraded mode" means the AI session file (`.sunshine/.runtime/sessions/`) is not written; `.sunshine/.current-task` is still written, so `task current` and `task context` can still find the current task. Inside a Claude Code / Cursor AI session both are written.

```sh
sunshine task current
# .sunshine/tasks/11-09-给退款原因加必填校验

sunshine task list
# 全部活动任务：
#
#   - 11-09-给退款原因加必填校验/ (in_progress) [演示者]
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
# 路径：.sunshine/tasks/11-09-给退款原因加必填校验
# 来源：file:.sunshine/.current-task
# 名字：给退款原因加必填校验
# 状态：in_progress
# 创建：2026-11-09
# 描述：退款时原因字段必填
```

("全部活动任务" = "all active tasks"; "总计：1 个任务" = "total: 1 task"; "会话上下文" = "session context"; "开发者" = "developer"; "Git 状态" = "git status"; "当前任务" = "current task"; "来源" = "source".)

If the context list is still empty, `start` refuses outright and tells you what to do (verified with a second task):

```text
错误：implement.jsonl 和 check.jsonl 没有已整理条目
子代理（implement/check）将得不到任何规范上下文。
  整理：sunshine task add-context 11-09-给退款页加返回按钮 implement <路径> "<原因>"
  检查：sunshine task validate 11-09-给退款页加返回按钮
  确实要空着：重新运行 start 并加 --allow-empty-context
```

("Error: implement.jsonl and check.jsonl have no organized entries; the implement/check sub-agents will get no spec context. Organize with… / check with… / if you really want it empty, re-run start with --allow-empty-context.")

If you really want to start empty, add `--allow-empty-context` — verified to proceed (same degraded-mode notice, then `✓ 状态：planning → in_progress（降级）`).

Wrap up a task like this:

```sh
sunshine task finish
# ✓ 已清除当前任务（原为：.sunshine/tasks/11-09-给退款原因加必填校验）
# 来源：file:.sunshine/.current-task
```

("Current task cleared (was: …); source: file:.sunshine/.current-task.") `finish` only clears the current-task pointer; it does not change the task status — the status becomes `completed` at archive time.

### Archive

```sh
sunshine task archive 11-09-给退款原因加必填校验
```

In the demo project this step is refused — that is real behavior, not a mistake:

```text
警告：记录的分支 "master" 已不存在（可能已合并删除）。
错误：拒绝归档 "11-09-给退款原因加必填校验"：branch 与 base_branch 都是 "master"，PR 不能指向自身分支，这份元数据无法描述已合并的工作。
请修复其中一个字段：
  sunshine task set-branch 11-09-给退款原因加必填校验 <功能分支>
  sunshine task set-base-branch 11-09-给退款原因加必填校验 <目标分支>
  sunshine task archive 11-09-给退款原因加必填校验 --skip-branch-validation   # 仅当该任务从未走 PR
未归档：.sunshine/tasks/11-09-给退款原因加必填校验 未动。
```

("Warning: the recorded branch 'master' no longer exists (may have been merged and deleted). Error: refusing to archive …: branch and base_branch are both 'master'; a PR cannot point at its own branch, this metadata cannot describe merged work. Fix one of the fields: … / … / … # only if the task never went through a PR. Not archived: … untouched.")

The demo task never went through a PR, so skip the check with `--skip-branch-validation` (in a real project, `set-branch` to a feature branch first):

```sh
sunshine task archive 11-09-给退款原因加必填校验 --skip-branch-validation
# 警告：记录的分支 "master" 已不存在（可能已合并删除）。
# 已归档：11-09-给退款原因加必填校验 -> archive/2026-11/
# [OK] 已自动提交：chore(task): archive 11-09-给退款原因加必填校验

sunshine task list-archive
# 已归档任务：
#
# [2026-11] - 1 个任务
#   - 11-09-给退款原因加必填校验/
```

("Archived: … -> archive/2026-11/; auto-committed: chore(task): archive …; archived tasks: [2026-11] - 1 task.") The archive path is `.sunshine/tasks/archive/<YYYY-MM>/`, and archiving auto-commits by default (while `session_auto_commit` is `true`).

### Record the session in the journal

After finishing a task, record the session in the developer journal (useful later when reviewing with `mem`, step 6):

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

("Adding session; session: 1; title: …; commit: -; current journal file: journal-1.md; current lines: 7; lines added: 21; total after append: 28; session appended to journal-1.md; session 1 added successfully; updated files: journal-1.md, index.md; auto-committed: chore: record journal.")

The record goes into `.sunshine/workspace/<your name>/journal-1.md`, and the auto-generated section of `index.md` is updated at the same time.

## 4. Execute and switch workflows

The actual coding happens in an AI session: open this project in Claude Code / Cursor, and the assistant drives the work following `.sunshine/workflow.md` — reading the specs and task context before writing any code. The CLI side only manages task state and workflow templates, which is what step 3 covered.

There are three built-in workflow templates:

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

("Available workflow templates: native (built-in) — the native Sunshine workflow: the default plan / execute / wrap-up workflow with native sub-agents and inline platform branches; tdd (built-in) — TDD workflow: a variant that drives phase 2 with one red / green / refactor behavior slice at a time; channel-driven-subagent-dispatch (built-in) — channel-driven sub-agent dispatch: a variant where the main session coordinates implement/check worker threads through sunshine channel.")

Switch to the TDD workflow:

```sh
sunshine workflow --template tdd
#   ✓ 已把 .sunshine/workflow.md 换成 "tdd"
```

("Switched .sunshine/workflow.md to 'tdd'.") Switching back to native stops you first — because `workflow.md` is now the tdd version, which no longer matches the hash manifest, the CLI treats it as "locally modified":

```sh
sunshine workflow --template native
# 错误：.sunshine/workflow.md 已有本地修改，请加 --force 覆盖，或用 --create-new 写 .sunshine/workflow.md.new

sunshine workflow --template native --force
#   ✓ 已把 .sunshine/workflow.md 换成 "native"

sunshine workflow --template native --create-new
#   + 已写入 .sunshine/workflow.md.new（workflow.md 未改动）
```

("Error: .sunshine/workflow.md has local modifications; add --force to overwrite, or use --create-new to write .sunshine/workflow.md.new. / Switched .sunshine/workflow.md to 'native'. / Wrote .sunshine/workflow.md.new (workflow.md untouched).")

The rule: unmodified files are overwritten directly; modified or unknown-origin files ask interactively (overwrite / write `.new` / skip), and in a non-interactive terminal you use `--force` to overwrite or `--create-new` to write a copy. `--create-new` only writes `.sunshine/workflow.md.new` and leaves the active `workflow.md` alone.

At the end of this step the tutorial project is on tdd — step 5's `update --dry-run` will list it under "你已修改" ("modified by you"), which is exactly the three-state detection in action.

## 5. Update and registry (template marketplace)

### First look with `update --dry-run`

`--dry-run` only prints the plan and writes nothing — the safest way to use it. The first lines are version information (they vary with the CLI version you installed); only the scan part is quoted below:

```sh
sunshine update --dry-run
```

Real output (excerpt):

```text
正在扫描变更……
  未变更文件（将跳过）：
    ○ .claude/agents/sunshine-check.md
    ○ .claude/agents/sunshine-implement.md
    ○ .claude/agents/sunshine-research.md
    ……
  你已修改（需要你决定）：
    ? .sunshine/workflow.md

  用户数据（保持不动）：
    ○ .sunshine/workspace/
    ○ .sunshine/tasks/
    ○ .sunshine/spec/
    ○ .sunshine/.developer/

[试运行] 未做任何修改。
```

("Scanning changes… Unchanged files (will be skipped): … / Modified by you (needs your decision): .sunshine/workflow.md / User data (kept untouched): .sunshine/workspace/, .sunshine/tasks/, .sunshine/spec/, .sunshine/.developer/ / [Dry run] No changes made.")

Three states at a glance: unchanged template files are updated automatically; files you modified ask one by one (overwrite / skip / write a `.new` copy, default skip); user data (specs, tasks, workspace, developer identity) is never touched. When the project and CLI are both on the latest version, the tail reads `✓ 已是最新，无需更新。` ("Already up to date, nothing to update."). To actually run it, drop `--dry-run`; before writing, it takes a full backup under `.sunshine/.backups/<timestamp>/`, and on success it updates `.sunshine/.version`.

### Install specs from a template marketplace

Switch to a fresh project and start from a marketplace template. First build a minimal local marketplace repository (this is the standard layout of an official template repository: `marketplace/index.json` + `marketplace/specs/`):

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

1. A local path must point at the `marketplace` subdirectory itself (`…/registry-src/marketplace`), not the repository root — pointing at the root makes the CLI fail to find `index.json` with `该 registry 没有 index.json，不能用 --template` ("this registry has no index.json, --template cannot be used").
2. The `path` in `index.json` is relative to the directory containing `index.json`, so write `specs/demo-spec`, not `marketplace/specs/demo-spec`.

Then install from the marketplace in the new project:

```sh
mkdir demo2 && cd demo2 && git init
sunshine init -u 演示者 -y --registry /tmp/sunshine-docs/registry-src/marketplace --template demo-spec
```

Real output:

```text
正在配置 Claude Code……
正在配置 Cursor……
已安装模板 "demo-spec"（.sunshine/spec），共 1 个文件。
已记录 registry.spec：/tmp/sunshine-docs/registry-src/marketplace（模板 demo-spec），sunshine update 会持续刷新。
初始化完成，开发者：演示者
已安装平台：
  - Claude Code
  - Cursor
下一步：
  1. 编辑 .sunshine/spec/，填入团队已有的技术规范与业务术语；
  2. 按 .sunshine/workflow.md 开始第一个任务。
```

("Installed template 'demo-spec' (.sunshine/spec), 1 file in total. Recorded registry.spec: /tmp/sunshine-docs/registry-src/marketplace (template demo-spec); sunshine update will keep refreshing it. Initialization complete…")

The template content lands in `.sunshine/spec/demo.md`, and the source is recorded in `config.yaml`:

```yaml
# 受哈希追踪的 spec 模板，本地改动走正常的 update 冲突流程。
registry:
  spec:
    source: /tmp/sunshine-docs/registry-src/marketplace
    template: demo-spec
```

("Spec templates tracked by hash; local edits go through the normal update conflict flow.") A remote marketplace works the same way — replace the local path with `gh:owner/repo`, a full git URL, an HTTP(S) URL, or a `git@` address.

### After the marketplace template changes, `update` keeps refreshing it

Edit the template in the marketplace, commit, and run `update --dry-run` again:

```sh
# In registry-src, append one more convention to demo.md and commit it
sunshine update --dry-run
```

Real output (excerpt):

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

("Template updated (will be updated automatically): .sunshine/spec/demo.md / Unchanged files (will be skipped): … / Modified by you (needs your decision): .sunshine/config.yaml / User data (kept untouched): …")

`demo.md` is updated automatically because its local copy was never edited; if you had edited it locally, it would go through the same "modified by you" confirmation. `config.yaml` shows as "modified by you" because `registry.spec` was written into it when the template was installed — that is expected.

## 6. Review sessions with mem

`mem` parses the session records of each platform on this machine, read-only — it never writes to session storage. Real usage first: if you have already run Sunshine tasks in Claude Code, just run these in the project:

```sh
sunshine mem list              # List this project's sessions
sunshine mem search "退款"      # Find sessions by keyword
sunshine mem context 11111111  # View one session's context (the id can be abbreviated)
sunshine mem extract 11111111  # Export one session's full conversation
sunshine mem projects          # List projects that have sessions
```

The output shapes below are demonstrated with a minimal session file (a real session has exactly this layout: `~/.claude/projects/<project directory>/<session id>.jsonl`):

```sh
export HOME=/tmp/sunshine-docs/home   # Demo only; your real HOME is untouched
mkdir -p "$HOME/.claude/projects/-tmp-sunshine-docs-demo"
# Write a 4-turn demo session 11111111-2222-3333-4444-555555555555.jsonl
# (two user turns, two assistant turns, about this tutorial's refund task)
```

```sh
sunshine mem list
# 范围：项目=/tmp/sunshine-docs/demo  平台=all
# [claude  ] 2026-11-09 14:13  11111111-222  /tmp/sunshine-docs/demo
#
# 共 1 个会话

sunshine mem search "退款"
# 范围：项目=/tmp/sunshine-docs/demo  平台=all  关键词="退款"
#
# [claude  ] 2026-11-09 14:13  11111111-222  /tmp/sunshine-docs/demo  相关度=0.250  命中=1（用户=1，助手=0）  轮次=4
#     [用户] 给退款原因加必填校验，原因字段不能为空
#
# 共 1 个会话

sunshine mem context 11111111
# # 上下文：[claude] 11111111-2222-3333-4444-555555555555
# # 工作目录：/tmp/sunshine-docs/demo
# # 未指定关键词 — 显示前 3 轮，共 4 轮
# # 显示轮次：4  已用预算：86/6000 字符
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
# 2026-11-09 14:13  会话数=  1 (claude:1)  /tmp/sunshine-docs/demo
#
# 共 1 个项目
```

("范围" = "scope"; "共 1 个会话" = "1 session in total"; "相关度" = "relevance"; "命中" = "hits"; "轮次" = "turns"; "上下文" = "context"; "工作目录" = "working directory"; "未指定关键词 — 显示前 3 轮，共 4 轮" = "no keyword given — showing the first 3 of 4 turns"; "显示轮次" = "turns shown"; "已用预算" = "budget used"; "活跃项目" = "active projects"; "会话数" = "session count".)

The output often starts with a few `警告：未找到 Codex 会话存储（…），已跳过` ("warning: Codex session storage not found (…), skipped") lines — those only mean that platform has no session records on this machine; they are not errors. `--global` searches all projects, `--json` outputs machine-readable JSON, and `--platform claude` restricts the search to one platform.

## 7. Multi-agent collaboration with channel

A channel is a session where several workers (claude / codex) collaborate: `create` opens a session, `spawn` registers a worker, `send` sends a message, `wait` waits for an event. The storage root defaults to `~/.sunshine/channels/` and can be overridden with `SUNSHINE_CHANNEL_ROOT` — this tutorial uses a temporary directory so your real channel storage is untouched:

```sh
export SUNSHINE_CHANNEL_ROOT=/tmp/sunshine-docs/channels
```

### Create a session, send a message, view events

```sh
sunshine channel create review --type chat
# 已创建 channel 'review'（chat）位于 /tmp/sunshine-docs/channels/-tmp-sunshine-docs-demo/review

sunshine channel send review --to impl "开始实现退款原因必填校验"
# {"by":"main","kind":"message","origin":"cli","seq":2,"text":"开始实现退款原因必填校验","to":"impl","ts":"2026-11-09T14:13:05.091268768Z"}

sunshine channel messages review
# [1] 14:13:04 [create] by=main cwd=/tmp/sunshine-docs/demo
# [2] 14:13:05 [message] by=main to=impl
#          开始实现退款原因必填校验

sunshine channel list
# 名称                        工作者        事件      最近活动                 最近事件       类型       任务
# review                    -          1       -                    create     chat     -
```

("Created channel 'review' (chat) at …") `send` prints one JSON event (`seq` is the event sequence number); `messages` is the human-readable timeline. You can bind a task when creating the session: `sunshine channel create review --type chat --task .sunshine/tasks/11-09-给退款原因加必填校验`.

### Title and context

```sh
sunshine channel title set review --title "退款原因必填校验"
# {"action":"title","by":"main","kind":"channel","origin":"cli","seq":3,"title":"退款原因必填校验","ts":"2026-11-09T14:13:14.834590113Z"}

sunshine channel context add review --raw "只改退款逻辑，不动其他接口"
# {"action":"add","by":"main","context":[{"text":"只改退款逻辑，不动其他接口","type":"raw"}],"kind":"context","origin":"cli","seq":6,"target":"channel","ts":"2026-11-09T14:13:56.741845958Z"}

sunshine channel context list review
# raw  只改退款逻辑，不动其他接口
```

(The raw context entry means "only change the refund logic, don't touch other interfaces".)

### Waiting for an event: a timeout is a normal result

```sh
sunshine channel wait review --from impl --kind done --timeout 3s
# 错误：超时：等待 channel 'review' 超时（3s），仍在等待：impl（上游退出码 124）
```

("Error: timeout: waiting for channel 'review' timed out (3s), still waiting: impl (upstream exit code 124).") With no worker replying, `wait` times out after `--timeout` and exits with an error — that is what it is for: scripts use it to wait for a worker's `done` event.

### Forum channels: threaded discussion

A `forum`-type channel organizes discussion into threads (topics). Forum commands do not work on `chat` channels — verified: `频道 "review" 的类型是 chat，forum 操作只支持 forum 类型的频道` ("channel 'review' is of type chat; forum operations only support forum-type channels"):

```sh
sunshine channel create board --type forum
# 已创建 channel 'board'（forum）位于 /tmp/sunshine-docs/channels/-tmp-sunshine-docs-demo/board

sunshine channel post board opened --as main --thread t1 --title "退款原因实现方案" --description "原因下拉框：质量问题 / 尺寸不符 / 其他"
# {"action":"opened","by":"main","description":"原因下拉框：质量问题 / 尺寸不符 / 其他","kind":"thread","origin":"cli","seq":2,"thread":"t1","title":"退款原因实现方案","ts":"2026-11-09T14:13:24.366836885Z"}

sunshine channel post board comment --as alice --thread t1 --text "收到，正在看"
# {"action":"comment","by":"alice","kind":"thread","origin":"cli","seq":3,"text":"收到，正在看","thread":"t1","ts":"2026-11-09T14:13:24.385489679Z"}

sunshine channel forum board
# 主题  状态  标题
# t1 [open] 退款原因实现方案

sunshine channel thread board t1
# t1 [open] 退款原因实现方案
# 描述：原因下拉框：质量问题 / 尺寸不符 / 其他
#   2026-11-09 14:13:24 opened 发帖人=main
#   2026-11-09 14:13:24 comment 发帖人=alice 收到，正在看
```

("主题 / 状态 / 标题" = "topic / status / title"; "描述" = "description"; "发帖人" = "posted by"; "收到，正在看" = "got it, looking into it".)

### spawn: requires a real, logged-in worker

`spawn` is the only step in channel that needs a real environment: it starts the local `claude` or `codex` process. The prerequisite is that the corresponding CLI is installed **and logged in**. The sandbox had neither, and the verified results are recorded here as they happened:

```sh
sunshine channel spawn review --agent implement --provider claude --as impl
# {"pid": 24963, "log": "/tmp/sunshine-docs/channels/-tmp-sunshine-docs-demo/review/impl.log", "worker": "impl"}
```

The command itself returned a pid and a log path, but the event log immediately recorded a failure:

```sh
sunshine channel messages review
# ……
# [4] 14:13:14 [error] by=supervisor:impl worker 拉起失败：拉起 claude worker 失败：exec: "claude": executable file not found in $PATH
```

("worker failed to start: failed to start claude worker: exec: 'claude': executable file not found in $PATH.")

Trying codex instead (the sandbox has a codex binary, but it is not logged in): the process did start, then failed while connecting with a 401:

```text
ERROR codex_api::endpoint::responses_websocket: failed to connect to websocket: HTTP error: 401 Unauthorized, url: wss://api.openai.com/v1/responses
```

So: **on a machine with claude / codex installed and logged in, messages sent with `send` after `spawn` are received and executed by the worker; without a logged-in worker, `spawn` only leaves an `error` event — it never pretends to succeed.** The same prerequisite applies to `channel run`, which builds a temporary channel, runs one task, and prints the final answer.

### Stop a worker, delete a channel

```sh
sunshine channel kill review --as impl-codex
# 已停止 worker 'impl-codex'（channel 'review'）

sunshine channel rm tmp
# 已删除 channel 'tmp'
```

("Stopped worker 'impl-codex' (channel 'review'). / Deleted channel 'tmp'.") `kill` takes the worker name with `--as <worker>`; `rm` kills all workers and deletes the whole channel directory. `prune` cleans up channels in bulk by condition (dry-run by default, nothing is really deleted): `sunshine channel prune --empty`.

## 8. Upgrade

`upgrade` upgrades the CLI itself (`update` upgrades the template files inside a project — two different things). Check the plan first with `--dry-run`; nothing is written:

```sh
sunshine upgrade --dry-run
```

When already on the latest version (v0.2.0 installed from Release):

```text
已是最新版本（当前 0.2.0，最新 0.2.0），无需升级。
```

("Already on the latest version (current 0.2.0, latest 0.2.0), no upgrade needed.")

When a new version exists (the line below comes from a locally built 0.1.0 binary; only the version numbers differ, the shape is the same):

```text
正在升级 Sunshine：0.1.0 → 0.2.0（通道 latest）
资产：sunshine_0.2.0_linux_amd64.tar.gz
下载地址：https://github.com/linrong1982/Sunshine/releases/download/v0.2.0/sunshine_0.2.0_linux_amd64.tar.gz
备份：将把当前二进制备份为 /tmp/sunshine-docs/sunshine.bak
[试运行] 未做任何修改。
```

("Upgrading Sunshine: 0.1.0 → 0.2.0 (channel latest); asset: sunshine_0.2.0_linux_amd64.tar.gz; download URL: …; backup: the current binary will be backed up as /tmp/sunshine-docs/sunshine.bak; [dry run] no changes made.")

A real upgrade (drop `--dry-run`) follows the same flow: check the latest version → pick the asset for the current OS and architecture → download → verify sha256 against `checksums.txt` → back up the current binary (a `.bak` next to it) → replace → run the new `--version` to verify, rolling back to the backup and reporting an error on failure. You can also pin a version or channel: `sunshine upgrade --tag 0.2.0`, `sunshine upgrade --tag beta`.

## Wrap-up

That is the whole mainline: the project skeleton and specs live in `.sunshine/`, a task went from creation to archive, the workflow switched to TDD, a marketplace template was installed and its continuous refresh verified, sessions can be reviewed, a multi-agent channel can open sessions, send messages, and run threads, and the CLI upgrades itself.

The demo directories all live under `/tmp/sunshine-docs/` (`demo`, `demo2`, `registry-src`, `channels`, `home`) — delete them and nothing in any real project is affected.

Where to go next:

- [README.en.md](../README.en.md): core commands at a glance and configuration essentials.
- [PLAN.md](../PLAN.md): the full plan and per-stage implementation notes, including commands this tutorial did not expand on (`uninstall`, `ablate`, `restore`, `platforms`, `hook`) and the design decisions behind them.
- [Releases](https://github.com/linrong1982/Sunshine/releases): downloads and release notes for each version.
