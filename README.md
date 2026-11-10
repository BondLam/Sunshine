# Sunshine

把 Trellis 的全部功能用 Go 重新实现，CLI 界面与生成的规范文件全部中文。

> 英文版：[README.en.md](README.en.md) · 完整教程：[docs/tutorial.md](docs/tutorial.md)（[English](docs/tutorial.en.md)）

## 安装

macOS / Linux 一行装好（自动识别系统与架构，校验 sha256）：

```sh
curl -fsSL https://raw.githubusercontent.com/linrong1982/Sunshine/main/install.sh | sh
```

装指定版本：

```sh
curl -fsSL https://raw.githubusercontent.com/linrong1982/Sunshine/main/install.sh | sh -s -- v0.2.0
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
sunshine init -u 你的名字 -y   # 生成中文 .sunshine/ 骨架（规范、任务、工作流）
sunshine --help                 # 看全部命令
```

`init` 默认安装 Claude Code 与 Cursor 两套平台配置；只想装 Codex 时用 `sunshine init --codex -y`，已初始化的项目里再跑 `init --codex -y` 是增量追加，不动已有文件。`sunshine platforms` 可查看当前项目已配置的平台。

## 核心命令一览

| 命令 | 一句话 | 最常用示例 |
| --- | --- | --- |
| `init` | 初始化项目，生成中文 `.sunshine/` 骨架与平台配置 | `sunshine init -u 你的名字 -y` |
| `task` | 任务管理：创建、启动、完成、归档 | `sunshine task create "给退款原因加必填校验" --description "退款时原因字段必填"` |
| `workflow` | 查看或切换 `.sunshine/workflow.md` 的工作流模板 | `sunshine workflow --list` |
| `update` | 把项目内的 Sunshine 模板文件同步到当前 CLI 版本 | `sunshine update --dry-run` |
| `init --registry` | 从模板市场装规范模板（装完由 `update` 持续刷新） | `sunshine init --registry gh:owner/repo --template demo-spec -y` |
| `mem` | 跨平台检索本机 AI 会话历史（只读） | `sunshine mem search "退款"` |
| `channel` | 多 Agent 协同：通过共享事件日志派生、协调 worker | `sunshine channel create review --type chat` |
| `upgrade` | 升级 Sunshine CLI 自身（查 GitHub Releases） | `sunshine upgrade --dry-run` |

`update` 与 `upgrade` 的区别：`update` 更新项目里的模板文件，`upgrade` 换掉 `sunshine` 这个二进制。`task create` 须带 `--description`；`task start` 前须先用 `task add-context` 整理上下文清单，或加 `--allow-empty-context`。

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

`sunshine init --registry <来源> --template <模板 id>` 可从模板市场安装规范模板，来源支持 `gh:owner/repo`、完整 git URL、HTTP(S) URL、`git@` 形式或本地 git 仓库路径；装完后来源记进 `config.yaml` 的 `registry.spec`，`sunshine update` 会持续刷新。模板市场仓库的标准结构是 `marketplace/index.json` + `marketplace/specs/`。详见[教程第 5 步](docs/tutorial.md#5-update-与-registry模板市场)。

## 更多

- 完整教程（一条主线走到底）：[docs/tutorial.md](docs/tutorial.md)（[English](docs/tutorial.en.md)）
- 完整规划与各阶段实施记录：[PLAN.md](PLAN.md)
- 发布与下载：[Releases](https://github.com/linrong1982/Sunshine/releases)
