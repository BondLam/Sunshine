# Sunshine

**开箱即用的 AI 编程工程框架——Go 重构版。**

AI 写代码很快，但每次会话都从零开始——不记得你的项目、你的约定、你团队的需求。Sunshine 把规范、任务和记忆持久化进仓库，让任何编程助手都按团队的工程标准工作。CLI 界面与生成的规范文件全部中文。

[English](README.md) • [完整教程](docs/tutorial.md) • [Releases](https://github.com/linrong1982/Sunshine/releases)

[![Release](https://img.shields.io/github/v/release/linrong1982/Sunshine)](https://github.com/linrong1982/Sunshine/releases) [![CI](https://github.com/linrong1982/Sunshine/actions/workflows/ci.yml/badge.svg)](https://github.com/linrong1982/Sunshine/actions/workflows/ci.yml) [![Go](https://img.shields.io/github/go-mod/go-version/linrong1982/Sunshine)](https://go.dev/) [![Stars](https://img.shields.io/github/stars/linrong1982/Sunshine)](https://github.com/linrong1982/Sunshine/stargazers)

**注意：**命令与示例输出均为中文原文，可直接复制使用。

## 为什么用 Sunshine？

| 能力 | 改变了什么 |
| --- | --- |
| **规范自动注入** | 约定只在 `.sunshine/spec/` 里写一次，之后 Sunshine 在每次会话里注入相关上下文，不用反复重复。 |
| **以任务为中心的工作流** | PRD、实现上下文、检查上下文、任务状态都放在 `.sunshine/tasks/` 里，AI 的工作始终有结构。 |
| **项目记忆** | `.sunshine/workspace/` 里的日志留下了上次发生的事，新会话带着真实上下文开始。 |
| **团队共享标准** | 规范放在仓库里，一个人摸索出的工作流或规则，全团队都能受益。 |
| **多平台配置** | 同一套 Sunshine 结构带到 22 个 AI 编程平台，不用每个工具重建工作流。 |
| **中文优先，单文件** | CLI 与生成的规范全部中文；CLI 是单个 Go 二进制，不用装 Node.js 或 Python 运行时。 |

## 前置条件

- macOS 或 Linux（Windows：到 [Releases](https://github.com/linrong1982/Sunshine/releases) 下载对应的 zip，解压后把 `sunshine.exe` 放进 PATH）。
- 不需要再装任何运行时。

## 快速上手

```sh
# 1. 安装 Sunshine
curl -fsSL https://raw.githubusercontent.com/linrong1982/Sunshine/main/install.sh | sh

# 2. 在你的仓库里初始化
sunshine init -u 你的名字

# 3. 或者只装你用的平台
sunshine init --codex -y -u 你的名字
```

装指定版本（v0.5.0 是当前 Release）：

```sh
curl -fsSL https://raw.githubusercontent.com/linrong1982/Sunshine/main/install.sh | sh -s -- v0.5.0
```

已有 Go 的机器也可以：

```sh
go install github.com/linrong1982/Sunshine/cmd/sunshine@latest
```

装好后升级只需：

```sh
sunshine upgrade
```

完整故事见[完整教程](docs/tutorial.md)。

## 怎么用

流程很简单：

1. **用自然语言说出你要什么**，在 agent 界面里提（Claude Code 为主，Codex / Cursor 同理）。
2. **和 AI 一问一答做头脑风暴**，直到 PRD 清楚了再开始实现。
3. **让它自己跑**——AI 调 `sunshine-implement` 写代码，再按规范、lint、类型检查和测试自动检查结果。
4. **做完或会话上下文快满时输入 `/sunshine:finish-work`**，Sunshine 归档任务并更新日志。

## 工作原理

Sunshine 按四阶段循环跑，skill 与子代理自动触发：

1. **规划**——agent 和你一问一答做头脑风暴，写出 `prd.md`；研究量大的事项交给 `sunshine-research` 子代理。产出是整理好的规范 + 研究文件，由 `implement.jsonl` / `check.jsonl` 引用。
2. **实现**——`sunshine-implement` 子代理按 PRD 写代码，上下文自动注入，不提交。
3. **检查**——`sunshine-check` 子代理按规范检查 diff，跑 lint、类型检查和测试，能自己修的自己修。
4. **收尾**——最后再检查一遍，`sunshine-update-spec` 把新学到的东西写回 `.sunshine/spec/`，下次会话更聪明。

## 六个场景

教程第 12 节走过六个真实场景，每个都是一次完整的端到端跑通：

| 场景 | 适用时 |
| --- | --- |
| [从零开始新项目](docs/tutorial.md#121-从零开始新项目值班表小工具) | 新产品、新服务、新包或内部工具 |
| [接入存量项目](docs/tutorial.md#122-接入存量项目先把约定落进-spec) | 仓库已存在，规范主要靠人记忆 |
| [交付产品功能](docs/tutorial.md#123-交付产品功能一个任务横跨产品api数据和-ui) | 一个任务横跨产品、API、数据和 UI |
| [重构老模块](docs/tutorial.md#124-重构老模块先钉住行为再动代码) | 代码能跑，但已经难以安全修改 |
| [修复反复出现的 bug](docs/tutorial.md#125-修复反复出现的-bug先找上次的会话) | 同类问题多次出现 |
| [产品设计先行](docs/tutorial.md#126-产品设计先行需求--设计文档--拆解--开发) | 先做产品设计，再进开发 |

设计先行时，设计文档先落在 `doc/` 里，再拆解成子任务（场景 6）。终端命令是 agent 的工具，不是你每天的界面——开场提示词点名四件事：工作流、先提问、确认后建任务写 PRD、再动手。

## 核心命令一览

agent 在底下调的命令你平时不用敲；下面是你手动干预时用的最常用命令，完整参考见[教程附录：命令速查](docs/tutorial.md#附录命令速查)。

| 命令 | 一句话 | 最常用例子 |
| --- | --- | --- |
| `init` | 初始化项目：生成中文 `.sunshine/` 骨架与平台配置；`--registry` 从模板市场安装规范 | `sunshine init -u 你的名字 -y` |
| `task` | 任务管理：创建、开始、完成、归档（agent 在底下调；你用 `task current` / `task list` 查状态） | `sunshine task create "给退款原因加必填校验" --description "退款时原因字段必填"` |
| `workflow` | 查看或切换 `.sunshine/workflow.md` 用的工作流模板 | `sunshine workflow --list` |
| `update` | 把项目里的 Sunshine 模板文件同步到当前 CLI 版本 | `sunshine update --dry-run` |
| `mem` | 跨平台搜索本地 AI 会话历史（只读） | `sunshine mem search "退款"` |
| `channel` | 多 agent 协同：通过共享事件日志派生与协调工作线程 | `sunshine channel create review --type chat` |
| `upgrade` | 升级 Sunshine CLI 本身（走 GitHub Releases） | `sunshine upgrade --dry-run` |

`update` 与 `upgrade` 的区别：`update` 更新项目里的模板文件，`upgrade` 换掉 `sunshine` 这个二进制。agent 在底下调的命令见[教程第 4 节](docs/tutorial.md#4-底下发生了什么)。

## 配置要点

`init` 生成的 `.sunshine/config.yaml` 自带中文注释，大多是注释掉的默认值——只改你需要的那几项即可：

- `session_commit_message`（默认 `"chore: record journal"`）与 `session_auto_commit`（默认 `true`）：`task add-session` / `task archive` 后自动提交日志与任务改动。
- `max_journal_lines`（默认 `2000`）：单个日志文件轮转前的最大行数。
- `hooks.after_create / after_start / after_finish / after_archive`：任务生命周期事件后运行的 shell 命令（每个都会收到 `TASK_JSON_PATH`）。
- `channel.worker_guard.idle_timeout`（默认 `5m`）与 `max_live_workers`（默认 `6`）：`channel spawn` 的工作线程守卫。
- `codex.dispatch_mode`（默认 `auto`）：Codex 子代理派发行为（`auto` / `inline`）。
- `context_injection` 上限：`max_file_bytes`（32768）/ `max_artifact_bytes`（65536）/ `max_total_bytes`（131072）。
- `prompt_injection.skip_keyword`（默认 `"no-sunshine"`）：单轮提示词注入的逃生口。
- `registry.spec`：用 `--registry` 装模板时自动写入，`sunshine update` 会持续从它刷新市场模板。
- `tasks_dir` / `spec_dir`：任务与规范目录（默认 `.sunshine/tasks` / `.sunshine/spec`）。

## 模板市场

`sunshine init --registry <来源> --template <模板 id>` 从模板市场安装规范模板。来源可以是 `gh:owner/repo`、完整 git URL、HTTP(S) URL、`git@` 地址或本地 git 仓库路径。装好后来源记在 `config.yaml` 的 `registry.spec` 里，`sunshine update` 会持续刷新。市场仓库用标准结构 `marketplace/index.json` + `marketplace/specs/`。交互模式下不传 `--template` 会列出该市场的规范模板供你选，不用事先知道模板 id。见[教程第 8 节](docs/tutorial.md#8-update-与模板市场)。

## 资源

| 需要 | 链接 |
| --- | --- |
| 在仓库里安装 Sunshine | [快速上手](#快速上手) |
| 看实际用法 | [完整教程](docs/tutorial.md)（六个场景） |
| 从规范模板开始 | [模板市场](#模板市场) |
| 跟踪发布 | [Releases](https://github.com/linrong1982/Sunshine/releases) |
| 完整规划 | [PLAN.md](PLAN.md) |

## 常见问题

**Sunshine 和 `CLAUDE.md`、`AGENTS.md`、`.cursorrules` 有什么区别？**

那些文件是有用的入口，但容易变成大杂烩。Sunshine 在它们之外加了分范围的规范、任务 PRD、工作流关卡、工作区记忆和平台感知的生成文件。

**Sunshine 只支持 Claude Code 吗？**

不是。Sunshine 是项目层，支持 22 个编程 agent 与 IDE。

**Sunshine 是 Trellis 的 fork 吗？**

不是。Sunshine 按功能清单用 Go 重写了 Trellis 的行为，没有复制 Trellis 源码（Trellis 是 AGPL-3.0）。工作流、命令与文件结构有意与 Trellis 对齐，经验可以迁移。

**为什么 CLI 只有中文？**

团队用中文工作。命令、输出与生成的规范保持中文，可以原样复制；文档是中英双语。

**规范文件都要自己手写吗？**

不用。很多团队先让 AI 按现有代码起草规范，再把关键部分手写收紧。Sunshine 在高信号规则明确写进版本控制时效果最好。

**Sunshine 适合个人开发者还是团队？**

都适合。个人用它记记忆、跑固定工作流；团队收益更大：共享标准、任务边界、可检查的上下文、平台可移植。

**能临时对比一个项目用与不用 Sunshine 的区别吗？**

能。`sunshine ablate` 会在项目外建好验证过的恢复事务后，临时移除项目里所有 Sunshine 表面。换一个全新 agent 会话做对比，再跑 `sunshine restore` 恢复到之前的状态。两个操作都可以用 `--dry-run` 先预览。

## 社区与资源

- [完整教程](docs/tutorial.md)
- [GitHub Issues](https://github.com/linrong1982/Sunshine/issues)
- [Releases](https://github.com/linrong1982/Sunshine/releases)

用 Go 构建。Trellis 的 Go 重构版——与 Mindfold 无关。
