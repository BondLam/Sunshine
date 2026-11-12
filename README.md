# Sunshine

**An out-of-the-box engineering framework for AI coding — rebuilt in Go.**

AI writes code fast, but every session it starts from scratch — no memory of your project, your conventions, or your team's requirements. Sunshine persists specs, tasks, and memory into your repo, so any coding agent works to your engineering standards. The CLI and the generated spec files are all in Chinese.

[中文版](README.cn.md) • [Full tutorial](docs/tutorial.en.md) • [Releases](https://github.com/BondLam/Sunshine/releases)

[![Release](https://img.shields.io/github/v/release/BondLam/Sunshine)](https://github.com/BondLam/Sunshine/releases) [![CI](https://github.com/BondLam/Sunshine/actions/workflows/ci.yml/badge.svg)](https://github.com/BondLam/Sunshine/actions/workflows/ci.yml) [![Go](https://img.shields.io/github/go-mod/go-version/BondLam/Sunshine)](https://go.dev/) [![Stars](https://img.shields.io/github/stars/BondLam/Sunshine)](https://github.com/BondLam/Sunshine/stargazers)

**Note:** Sunshine's CLI itself is Chinese-only — commands, help text, prompts, and output are all in Chinese. This document translates the explanations into English, but commands and sample output are kept in the original Chinese so you can copy them verbatim.

## Why Sunshine?

| Capability | What it changes |
| --- | --- |
| **Auto-injected specs** | Write conventions once in `.sunshine/spec/`, then let Sunshine inject the relevant context into each session instead of repeating yourself. |
| **Task-centered workflow** | Keep PRDs, implementation context, review context, and task status in `.sunshine/tasks/` so AI work stays structured. |
| **Project memory** | Journals in `.sunshine/workspace/` preserve what happened last time, so each new session starts with real context. |
| **Team-shared standards** | Specs live in the repo, so one person's hard-won workflow or rule can benefit the whole team. |
| **Multi-platform setup** | Bring the same Sunshine structure to 22 AI coding platforms instead of rebuilding your workflow per tool. |
| **Chinese-first, single binary** | CLI and generated specs are all in Chinese; the CLI is one Go binary with no Node.js or Python runtime to install. |

## Prerequisites

- macOS or Linux (Windows: download the matching zip from [Releases](https://github.com/BondLam/Sunshine/releases), unzip it, and put `sunshine.exe` on your PATH).
- Nothing else — no runtime to install.

## Quick Start

```sh
# 1. Install Sunshine
curl -fsSL https://raw.githubusercontent.com/BondLam/Sunshine/main/install.sh | sh

# 2. Initialize in your repo
sunshine init -u your-name

# 3. Or initialize with only the platforms you use
sunshine init --codex -y -u your-name
```

Install a specific version (v0.5.0 is the current Release):

```sh
curl -fsSL https://raw.githubusercontent.com/BondLam/Sunshine/main/install.sh | sh -s -- v0.5.0
```

If you already have Go:

```sh
go install github.com/BondLam/Sunshine/cmd/sunshine@latest
```

To upgrade later:

```sh
sunshine upgrade
```

See the [full tutorial](docs/tutorial.en.md) for the complete storyline.

## How to Use

The workflow is simple:

1. **Describe what you want** in natural language in the agent UI (Claude Code first, Codex / Cursor the same idea).
2. **Brainstorm** with the AI one question at a time until the PRD is clear, then implementation begins.
3. **Let it run** — the AI calls `sunshine-implement` and auto-checks the result against specs, lint, type-check, and tests.
4. **Type `/sunshine:finish-work`** when the work is done or the session context fills up. Sunshine archives the task and updates journals.

## How It Works

Sunshine runs a 4-phase loop with auto-invoked skills and sub-agents:

1. **Plan** — the agent brainstorms with you one question at a time and writes `prd.md`. Research-heavy items go to a `sunshine-research` sub-agent. The result is curated specs + research files referenced from `implement.jsonl` / `check.jsonl`.
2. **Implement** — a `sunshine-implement` sub-agent writes code from the PRD with the curated context auto-injected, no git commit.
3. **Verify** — a `sunshine-check` sub-agent reviews the diff against specs and runs lint, type-check, and tests, self-fixing where it can.
4. **Finish** — a final check runs, then `sunshine-update-spec` promotes new learnings back into `.sunshine/spec/` so the next session starts smarter.

## Six scenarios

Section 12 of the tutorial walks through six real scenarios, each one a complete end-to-end run:

| Scenario | When it applies |
| --- | --- |
| [Starting a new project from zero](docs/tutorial.en.md#121-starting-a-new-project-from-zero-a-duty-roster-tool) | A new product, service, package, or internal tool |
| [Adopting an existing project](docs/tutorial.en.md#122-adopting-an-existing-project-land-the-conventions-in-spec-first) | The repo exists, but the conventions live mostly in people's heads |
| [Delivering a product feature](docs/tutorial.en.md#123-delivering-a-product-feature-one-task-across-product-api-data-and-ui) | One task spans product, API, data, and UI |
| [Refactoring a legacy module](docs/tutorial.en.md#124-refactoring-a-legacy-module-pin-the-behavior-first-then-change-the-code) | The code runs, but it is no longer safe to change |
| [Fixing a bug that keeps coming back](docs/tutorial.en.md#125-fixing-a-bug-that-keeps-coming-back-find-the-last-session-first) | The same class of problem appears again and again |
| [Design-first product work](docs/tutorial.en.md#126-design-first-product-work-requirements--design-documents--split--code) | Design comes before development |

For design-first work, the design documents land in `doc/` first, then split into sub-tasks (scenario 6). The terminal commands are the agent's tools, not your daily interface — an opening prompt names four things: the workflow, ask first, confirm then create the task and write the PRD, then implement.

## Core commands

The commands the agent calls under the hood are not yours to type; below are the ones you use when you step in manually. For the full reference, see the [tutorial appendix](docs/tutorial.en.md#appendix-command-reference).

| Command | In one line | Most common example |
| --- | --- | --- |
| `init` | Initialize a project: generate the Chinese `.sunshine/` skeleton and platform configs; `--registry` installs specs from a template marketplace | `sunshine init -u YourName -y` |
| `task` | Task management: create, start, finish, archive (the agent calls it under the hood; you check status with `task current` / `task list`) | `sunshine task create "给退款原因加必填校验" --description "退款时原因字段必填"` |
| `workflow` | View or switch the workflow template used by `.sunshine/workflow.md` | `sunshine workflow --list` |
| `update` | Sync the Sunshine template files in the project to the current CLI version | `sunshine update --dry-run` |
| `mem` | Search local AI session history across platforms (read-only) | `sunshine mem search "退款"` |
| `channel` | Multi-agent collaboration: spawn and coordinate workers through a shared event log | `sunshine channel create review --type chat` |
| `upgrade` | Upgrade the Sunshine CLI itself (via GitHub Releases) | `sunshine upgrade --dry-run` |

`update` vs `upgrade`: `update` refreshes the template files inside your project, while `upgrade` replaces the `sunshine` binary itself. The commands the agent runs under the hood are in [section 4 of the tutorial](docs/tutorial.en.md#4-what-happened-under-the-hood).

## Configuration

The `.sunshine/config.yaml` generated by `init` comes with Chinese comments. Most options are commented-out defaults — uncomment and change only what you need:

- `session_commit_message` (default `"chore: record journal"`) and `session_auto_commit` (default `true`): auto-commit journal and task changes after `task add-session` / `task archive`.
- `max_journal_lines` (default `2000`): maximum lines in a single journal file before rotation.
- `hooks.after_create / after_start / after_finish / after_archive`: shell commands run after task lifecycle events (each receives `TASK_JSON_PATH`).
- `channel.worker_guard.idle_timeout` (default `5m`) and `max_live_workers` (default `6`): worker guards for `channel spawn`.
- `codex.dispatch_mode` (default `auto`): Codex sub-agent dispatch behavior (`auto` / `inline`).
- `context_injection` limits: `max_file_bytes` (32768) / `max_artifact_bytes` (65536) / `max_total_bytes` (131072).
- `prompt_injection.skip_keyword` (default `"no-sunshine"`): escape hatch for per-turn prompt injection.
- `registry.spec`: written automatically when a template is installed with `--registry`; `sunshine update` keeps refreshing the marketplace template from it.
- `tasks_dir` / `spec_dir`: task and spec directories (defaults `.sunshine/tasks` / `.sunshine/spec`).

## Template marketplace

`sunshine init --registry <source> --template <template id>` installs spec templates from a template marketplace. Sources can be `gh:owner/repo`, a full git URL, an HTTP(S) URL, a `git@` address, or a local git repository path. After installation the source is recorded in `registry.spec` in `config.yaml`, and `sunshine update` keeps refreshing it. A marketplace repository uses the standard layout `marketplace/index.json` + `marketplace/specs/`. In interactive mode, omitting `--template` lists that marketplace's spec templates for you to pick, so you don't need to know the template id in advance. See [section 8 of the tutorial](docs/tutorial.en.md#8-update-and-the-template-marketplace).

## Resources

| Need | Link |
| --- | --- |
| Install Sunshine in a repo | [Quick Start](#quick-start) |
| See the workflow in practice | [Full tutorial](docs/tutorial.en.md) (six scenarios) |
| Start from spec templates | [Template marketplace](#template-marketplace) |
| Track releases | [Releases](https://github.com/BondLam/Sunshine/releases) |
| Read the full plan | [PLAN.md](PLAN.md) |

## FAQ

**How is Sunshine different from `CLAUDE.md`, `AGENTS.md`, or `.cursorrules`?**

Those files are useful entry points, but they tend to become monolithic. Sunshine adds scoped specs, task PRDs, workflow gates, workspace memory, and platform-aware generated files around them.

**Is Sunshine only for Claude Code?**

No. Sunshine is a project layer that works across 22 coding agents and IDEs.

**Is Sunshine a fork of Trellis?**

No. Sunshine reimplements Trellis's behavior in Go from a feature list; no Trellis source code was copied (Trellis is AGPL-3.0). The workflow, commands, and file layout intentionally match Trellis so skills transfer.

**Why is the CLI Chinese-only?**

The team works in Chinese. Commands, output, and generated specs stay in Chinese so they can be copied verbatim; the documentation is bilingual.

**Do I have to write every spec file manually?**

No. Many teams start by letting AI draft specs from existing code and then tighten the important parts by hand. Sunshine works best when you keep the high-signal rules explicit and versioned.

**Is Sunshine for solo developers or teams?**

Both. Solo developers use it for memory and repeatable workflow. Teams get the larger benefit: shared standards, task boundaries, reviewable context, and platform portability.

**Can I temporarily compare a project with and without Sunshine?**

Yes. `sunshine ablate` temporarily removes all supported project-owned Sunshine surfaces after creating a verified recovery transaction outside the project. Start a fresh agent session for the comparison, then run `sunshine restore` to recover the exact prior state. Use `--dry-run` to preview either operation.

## Community & Resources

- [Full tutorial](docs/tutorial.en.md)
- [GitHub Issues](https://github.com/BondLam/Sunshine/issues)
- [Releases](https://github.com/BondLam/Sunshine/releases)

Built with Go. A Go reimplementation of [Trellis](https://github.com/mindfold-ai/Trellis) — not affiliated with Mindfold.
