# Sunshine

A Go reimplementation of all Trellis features. The CLI interface and the generated spec files are all in Chinese.

> 中文版：[README.md](README.md) · Full tutorial: [docs/tutorial.en.md](docs/tutorial.en.md)（[中文](docs/tutorial.md)）

**Note:** Sunshine's CLI itself is Chinese-only — commands, help text, prompts, and output are all in Chinese. This document translates the explanations into English, but commands and sample output are kept in the original Chinese so you can copy them verbatim.

## Installation

macOS / Linux, one line (auto-detects OS and architecture, verifies sha256):

```sh
curl -fsSL https://raw.githubusercontent.com/linrong1982/Sunshine/main/install.sh | sh
```

Install a specific version (v0.3.8 is the current Release):

```sh
curl -fsSL https://raw.githubusercontent.com/linrong1982/Sunshine/main/install.sh | sh -s -- v0.3.8
```

Windows: download the matching zip from [Releases](https://github.com/linrong1982/Sunshine/releases), unzip it, and put `sunshine.exe` on your PATH.

If you already have Go:

```sh
go install github.com/linrong1982/Sunshine/cmd/sunshine@latest
```

To upgrade later:

```sh
sunshine upgrade
```

## Quick start

```sh
cd your-project
sunshine init -u YourName -y   # Generate the Chinese .sunshine/ skeleton (specs, tasks, workflow)
```

That is the only terminal command you need. Then open the project in Claude Code (Codex / Cursor work the same way) and ask for what you want in plain language — the agent drives the work following `.sunshine/workflow.md` and calls `sunshine` commands under the hood. See the [tutorial](docs/tutorial.en.md) for the full storyline.

`init` installs Claude Code and Cursor by default; to install only Codex, use `sunshine init --codex -y`. All init operations (platforms, template picker, three modes) are in the [tutorial appendix](docs/tutorial.en.md#appendix-command-reference).

## How it works: six scenarios

**Run `sunshine init` once in the terminal → ask in plain language inside the agent UI → the agent works through `workflow.md` (creates the task, writes prd/design, dispatches implement/check sub-agents, verifies, writes lessons back into the spec, finishes with `/sunshine:finish-work`) → you review the diff, accept, and commit.** For design-first work, the design documents land in `doc/` first, then split into sub-tasks (scenario 6). The terminal commands are the agent's tools, not your daily interface — an opening prompt names four things: the workflow, ask first, confirm then create the task and write the PRD, then implement.

Section 12 of the tutorial walks through six real scenarios, each one a complete end-to-end run:

| Scenario | When it applies |
| --- | --- |
| [Starting a new project from zero](docs/tutorial.en.md#121-starting-a-new-project-from-zero-a-duty-roster-tool) | A new product, service, package, or internal tool |
| [Adopting an existing project](docs/tutorial.en.md#122-adopting-an-existing-project-land-the-conventions-in-spec-first) | The repo exists, but the conventions live mostly in people's heads |
| [Delivering a product feature](docs/tutorial.en.md#123-delivering-a-product-feature-one-task-across-product-api-data-and-ui) | One task spans product, API, data, and UI |
| [Refactoring a legacy module](docs/tutorial.en.md#124-refactoring-a-legacy-module-pin-the-behavior-first-then-change-the-code) | The code runs, but it is no longer safe to change |
| [Fixing a bug that keeps coming back](docs/tutorial.en.md#125-fixing-a-bug-that-keeps-coming-back-find-the-last-session-first) | The same class of problem appears again and again |
| [Design-first product work](docs/tutorial.en.md#126-design-first-product-work-requirements--design-documents--split--code) | Design comes before development |

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

The `.sunshine/config.yaml` generated by `init` comes with Chinese comments. Most options are commented-out defaults — uncomment and change only what you need; the rest can stay as they are:

- `session_commit_message` (default `"chore: record journal"`): commit message used when journal / index changes are auto-committed.
- `session_auto_commit` (default `true`): auto-commit journal and task changes after `task add-session` / `task archive`.
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

## More

- Full tutorial (one real storyline end to end): [docs/tutorial.en.md](docs/tutorial.en.md)（[中文](docs/tutorial.md)）
- Full plan and per-stage implementation notes: [PLAN.md](PLAN.md)
- Releases and downloads: [Releases](https://github.com/linrong1982/Sunshine/releases)
