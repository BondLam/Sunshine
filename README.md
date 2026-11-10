# Sunshine

把 Trellis 的全部功能用 Go 重新实现，CLI 界面与生成的规范文件全部中文。

> 英文版：[README.en.md](README.en.md) · 完整教程：[docs/tutorial.md](docs/tutorial.md)（[English](docs/tutorial.en.md)）

主线一句话：**终端只跑一次 `sunshine init`，然后进 agent 界面（Claude Code 为主，Codex/Cursor 同理）用自然语言提需求，agent 按 `.sunshine/workflow.md` 干活，人检查 diff、验收、提交。**

## 安装

macOS / Linux 一行装好（自动识别系统与架构，校验 sha256）：

```sh
curl -fsSL https://raw.githubusercontent.com/linrong1982/Sunshine/main/install.sh | sh
```

装指定版本：

```sh
curl -fsSL https://raw.githubusercontent.com/linrong1982/Sunshine/main/install.sh | sh -s -- v0.2.1
```

Windows 请到 [Releases](https://github.com/linrong1982/Sunshine/releases) 页下载对应 zip，解压后把 `sunshine.exe` 放进 PATH。

已有 Go 的机器也可以：

```sh
go install github.com/linrong1982/Sunshine/cmd/sunshine@latest
```

装好后升级只需：

```sh
sunshine upgrade
```

## 快速上手

```sh
cd 你的项目
sunshine init -u 你的名字 -y   # 生成中文 .sunshine/ 骨架（规范、任务、工作流）与平台配置
```

然后在 Claude Code 里打开这个项目，直接用自然语言提需求，例如“给退款流程加一个必填的原因字段”——agent 会按 `.sunshine/workflow.md` 建任务、写 prd、派 implement/check 子代理、核查、把学到的写回 spec，最后用 `/sunshine:finish-work` 收尾；你检查 diff、验收、提交。

`init` 默认安装 Claude Code 与 Cursor；只想装 Codex 时用 `sunshine init --codex -y`。init 的全部操作（平台、模板选单、三种模式）见[教程附录：命令速查](docs/tutorial.md#附录命令速查)。

## 从 0 到 1：五个场景

日常开发只有四步：init 一次 → 在 agent 界面用自然语言提需求 → agent 按 workflow.md 建任务、写 PRD、派子代理、核查、把教训写回 spec → 你检查 diff、验收、提交。终端命令是 agent 的工具，不是你的日常界面——开场提示词写明四件事：点名工作流、先提问、确认后建任务写 PRD、再动手。

教程第 12 节按五个真实场景走了一遍，每个场景都是完整的端到端实录：

| 场景 | 适用时机 |
| --- | --- |
| [从零开始新项目](docs/tutorial.md#121-从零开始新项目值班表小工具) | 新建产品、服务、包或内部工具 |
| [接入存量项目](docs/tutorial.md#122-接入存量项目先把约定落进-spec) | 仓库已存在，规范主要靠人记忆 |
| [交付产品功能](docs/tutorial.md#123-交付产品功能一个任务横跨产品api数据和-ui) | 一个任务横跨产品、API、数据和 UI |
| [重构老模块](docs/tutorial.md#124-重构老模块先钉住行为再动代码) | 代码能跑，但已经难以安全修改 |
| [修复反复出现的 bug](docs/tutorial.md#125-修复反复出现的-bug先找上次的会话) | 同类问题多次出现 |

## 核心命令一览

agent 在底下调的命令你平时不用敲；下面是你手动干预时用的最常用命令，完整参考见[教程附录：命令速查](docs/tutorial.md#附录命令速查)。

| 命令 | 一句话 | 最常用示例 |
| --- | --- | --- |
| `init` | 初始化项目，生成中文 `.sunshine/` 骨架与平台配置；`--registry` 可从模板市场装规范 | `sunshine init -u 你的名字 -y` |
| `task` | 任务管理：创建、启动、完成、归档（agent 在底下调；人查状态用 `task current` / `task list`） | `sunshine task create "给退款原因加必填校验" --description "退款时原因字段必填"` |
| `workflow` | 查看或切换 `.sunshine/workflow.md` 的工作流模板 | `sunshine workflow --list` |
| `update` | 把项目内模板文件同步到当前 CLI 版本 | `sunshine update --dry-run` |
| `mem` | 跨平台检索本机 AI 会话历史（只读） | `sunshine mem search "退款"` |
| `channel` | 多 Agent 协同：通过共享事件日志派生、协调 worker | `sunshine channel create review --type chat` |
| `upgrade` | 升级 Sunshine CLI 自身（查 GitHub Releases） | `sunshine upgrade --dry-run` |

`update` 与 `upgrade` 的区别：`update` 更新项目里的模板文件，`upgrade` 换掉 `sunshine` 这个二进制。agent 在底下调的命令见[教程第 4 节](docs/tutorial.md#4-底下发生了什么)。

## 配置要点

`init` 生成的 `.sunshine/config.yaml` 带中文注释，关键字段：

- `session_auto_commit`（默认 `true`）：`task add-session` / `task archive` 后自动提交日志与任务改动。
- `max_journal_lines`（默认 `2000`）：单个日志文件轮换前的最大行数。
- `hooks.after_create / after_start / after_finish / after_archive`：任务生命周期事件后运行的 shell 命令。
- `channel.worker_guard.idle_timeout`（默认 `5m`）与 `max_live_workers`（默认 `6`）：`channel spawn` 的 worker 防护。
- `codex.dispatch_mode`（默认 `auto`）：Codex 子代理分发行为。
- `context_injection` 上限：`max_file_bytes`（32768）/ `max_artifact_bytes`（65536）/ `max_total_bytes`（131072）。
- `prompt_injection.skip_keyword`（默认 `"no-sunshine"`）：每轮提示词注入的逃生口。
- `registry.spec`：`--registry` 装模板后自动写入，`sunshine update` 按它持续刷新市场模板。
- `tasks_dir` / `spec_dir`：任务与规范目录（默认 `.sunshine/tasks` / `.sunshine/spec`）。

只改需要改的部分即可，其余保持默认。

## 模板市场

`sunshine init --registry <来源> --template <模板 id>` 可从模板市场安装规范模板，来源支持 `gh:owner/repo`、完整 git URL、HTTP(S) URL、`git@` 形式或本地 git 仓库路径；装完后来源记进 `config.yaml` 的 `registry.spec`，`sunshine update` 会持续刷新。模板市场仓库的标准结构是 `marketplace/index.json` + `marketplace/specs/`。交互模式不传 `--template` 时会列出该市场的 spec 模板让你现选，不用事先知道模板 id。详见[教程第 8 节：update 与模板市场](docs/tutorial.md#8-update-与模板市场)。

## 更多

- 完整教程（一条主线走到底）：[docs/tutorial.md](docs/tutorial.md)（[English](docs/tutorial.en.md)）
- 完整规划与各阶段实施记录：[PLAN.md](PLAN.md)
- 发布与下载：[Releases](https://github.com/linrong1982/Sunshine/releases)
