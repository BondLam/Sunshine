# `sunshine mem` CLI 参考

五个子命令的完整 flag 参考。把它作为权威来源钉在这里——`sunshine mem help` 在运行时打印相同内容，所以这里任何漂移都是 bug。

## 子命令

| 命令 | 用途 |
| ---------------------- | ---------------------------------------------------------------------------------------------------------------------- |
| `list`                 | 列出会话。未给出子命令时的默认子命令。 |
| `search <keyword>`     | 查找内容匹配关键词的会话。 |
| `context <session-id>` | 深入一个会话：命中最多的前 N 轮 + 周围上下文。搭配 `--grep` 做关键词锚定。 |
| `extract <session-id>` | 转储清理后的对话。结合 `--phase` / `--grep` 切片。 |
| `projects`             | 列出活跃项目的 `cwd` 值及会话数。用它发现要传给其他子命令的 `--cwd`。 |

## Flag（在有意义的地方适用）

| Flag | 子命令 | 含义 |
| --------------------------------------------- | ----------------- | ---------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `--platform claude\|codex\|devin\|grok\|opencode\|pi\|zcode\|all` | 全部 | 默认 `all`。`devin` 是 Cognition Devin CLI（`sessions.db`），不是 `sunshine init --devin`（Desktop 版）。 |
| `--since YYYY-MM-DD`                          | list / search     | 包含在内的下限日期。 |
| `--until YYYY-MM-DD`                          | list / search     | 包含在内的上限日期。 |
| `--global`                                    | list / search     | 包含本机上每个项目的会话。默认是当前项目的 `cwd`。 |
| `--cwd <path>`                                | list / search     | 强制指定某个项目 cwd，而不是从当前位置推断。 |
| `--limit N`                                   | list / search     | 输出行数上限。默认 `50`。 |
| `--grep KW`                                   | extract / context | 按关键词过滤轮次。用空白分隔多 token 时为 AND。 |
| `--phase brainstorm\|implement\|all`          | extract           | 按 Sunshine 任务边界切片会话。`brainstorm` = `[sunshine task create, sunshine task start)`。`implement` = 头脑风暴窗口之外的轮次。默认 `all`。 |
| `--turns N`                                   | context           | 返回的命中轮次数。默认 `3`。 |
| `--around N`                                  | context           | 每个命中轮次包含的周围轮次数。默认 `1`。 |
| `--max-chars N`                               | context           | 总字符预算。默认 `6000`（约 1500 token）。 |
| `--include-children`                          | search / context  | 把 OpenCode 子 Agent 会话合并进父会话。 |
| `--json`                                      | 全部 | 输出机器可解析的 JSON，而不是人类可读的输出。 |

## 常用一行命令

```bash
# 本机上哪些过去会话讨论过 "deadlock"？
sunshine mem search "deadlock" --global --limit 20

# 在某个具体会话内，找出提到 "lock contention" 的前 5 轮
# 及 2 轮周围上下文。
sunshine mem context 5842592d --grep "lock contention" --turns 5 --around 2

# 恢复某个会话的头脑风暴窗口——当继续
# 用户一周前开始的任务时有用。
sunshine mem extract 5842592d --phase brainstorm

# 列出本机上所有有 Sunshine 会话的项目及数量。
sunshine mem projects
```

## 输出形态

- **默认人类输出**（不带 `--json`）：按终端换行，session id 高亮、轮次标记可见。适合直接阅读，但粘贴进 markdown 文件会很乱。
- **`--json`**：稳定的 schema，可安全解析和处理。当把 `mem` 输出通过管道传给后续步骤（例如为 Lessons 章节做摘要）时，优先使用 `--json`。

## 注意点

- **OpenCode adapter 在 `0.6.0-beta.*` 上是 stub。** 当 `--platform` 解析为 OpenCode（或 `all` 且会包含 OpenCode）时，`mem` 打印一行“reader unavailable”提示并继续处理其他平台。在 adapter 发布之前，不要在回复中承诺 OpenCode 覆盖。
- **`--platform devin` 是 Cognition Devin CLI**（`~/.local/share/devin/cli/sessions.db`）。它不是 `sunshine init --devin`（Devin Desktop / Cascade），也不是 Factory Droid。
- **`--phase` 切片依赖会话记录的 bash 调用中出现 `sunshine task create` / `sunshine task start`。** 用户在另一个终端（记录的 AI 循环之外）运行 `sunshine task` 的会话不会有 phase 边界。`--phase all` 是安全的后备。
- **`mem` 直接索引平台 JSONL 文件。** 如果用户清空了 Claude / Codex / Pi 的会话存储，`mem` 无法恢复磁盘上已不存在的内容。
- **`mem` 是只读的。** 不远程同步、不编辑平台 JSONL。基于 `mem` 发现所做的任何写入，都是你自己调用可用编辑工具的后续决定。

## 需要超出本参考的内容时

在用户 shell 里运行 `sunshine mem help`。运行时帮助是权威的，在快速迭代的 beta 发布期间会领先于本参考。
