---
name: sunshine-session-insight
description: "通过 `sunshine mem` CLI 回溯过去的 AI 对话历史。当用户问 'how did we solve X last time'、'have we discussed this before'、'what was the decision on X'、'remind me what we did in this task'、'上次怎么解的'、'之前讨论过吗'、'想起一段对话'，或在开始与先前工作重叠的头脑风暴、调试熟悉的 bug、跨会话继续任务、做 finish-work 回顾时使用。返回原始的过去对话；由当时的情况决定是更新规范、追加到任务笔记、在回答中直接引用，还是只在心里记下。"
---

# Sunshine Session Insight

本 skill 教 AI **如何调用 `sunshine mem`**——项目的跨会话记忆原料——以及**什么时候调用它是正确的选择**。

它有意是一个**能力型 skill，而不是工作流**。没有固定的输出文件，没有必需的写回步骤，没有“每次 finish-work 后总要运行”的规则。拿到 `mem` 返回的内容后做什么，是对话当下的判断。本 skill 的存在，是为了让 AI 知道这个能力在那里、可以自己决定。

## `sunshine mem` 是什么

一个本地 CLI，为用户过去的 Claude Code、Codex、Devin CLI、Grok、OpenCode、Pi Agent 和 ZCode 对话日志建立索引，可以列出、搜索、按 Sunshine 任务边界切片，并转储清理后的对话。Claude 和 Codex 使用 `~/.claude/projects/` 和 `~/.codex/sessions/`。Devin CLI（Cognition 终端 Agent，不是 `sunshine init --devin` 的 Desktop 版）使用 `~/.local/share/devin/cli/sessions.db`。Grok 使用 `~/.grok/sessions/`。OpenCode 使用 `~/.local/share/opencode/opencode.db`（零依赖 SQLite 读取器）。Pi 使用其默认或环境配置的会话根目录、全局 `~/.pi/agent/settings.json` 以及限定在项目内的 `.pi/settings.json`；相对的 `sessionDir` 值从设置文件所在目录解析。项目本地的 Pi 设置需要通过当前 cwd 或 `--cwd` 做项目范围的查找。ZCode 使用 `~/.zcode/cli/db/db.sqlite`。

`mem` 里的任何内容都不会上传。所有读取都在本地。

## 什么时候调用它

标准是“一位资深队友会不会问‘我们不是已经讨论过这个了吗？’”——那些时刻就是。一些具体模式：

- **头脑风暴重跑风险。** 开始一个触及用户以前涉足领域的新任务，想在重新问用户之前，先检查是否已经做过决定。
- **熟悉 bug 的调试。** 当前 bug 的模式像是用户以前报告/修复过的。拉出相关过去会话可以省下一整轮调试。
- **跨会话继续。** 用户间隔一段时间后恢复工作，说“where were we” / “继续上次的”，但没有具体说明。
- **决策取回。** 用户提到“我们对 X 做的决定”，但该决定存在于旧的头脑风暴里，而不是任何 `prd.md` / `spec/` 中。
- **finish-work 回顾。** 当用户明确要求总结本任务中决定了什么 / 什么地方费劲 / 什么出乎意料时——不是每次 finish-work 的强制步骤。
- **跨过去工作的模式发现。** 用户问“do I keep making the same mistake on X” / “我每次都踩这个坑吗”——跨会话搜索可以回答。

如果这些都不适用，就不要调用 `mem`。它是一个工具，不是仪式。

## 什么时候不要调用它

- 相关上下文已经在当前轮次、`prd.md`、`design.md`、最近的 `git log` 或打开的文件里。`mem` 是为已经超出直接触及范围的东西准备的。
- 用户问的是代码里的事实，而不是过去对话里的事实。`git log -p` / `grep` / 直接读文件更快、更权威。
- 你身处子 Agent（`sunshine-implement` / `sunshine-check`），其分发提示词已经包含了精选的 `implement.jsonl` / `check.jsonl` 上下文。在上面再加 `mem` 通常只会添乱。
- 用户明确说过“don't dig through history, just answer what I asked”。

## 拿到 `mem` 返回内容后做什么

把输出当作**原始素材**，而不是交付物。拿到它之后，根据实时对话决定：

- **在回复中直接引用**——如果某段具体的过去对话回答了用户当前的问题——并引用 session-id / phase 以便用户核实。
- **更新 `<task>/prd.md` 或 `<task>/design.md`**——如果 `mem` 挖出了一个本应写下来却没写的关键决策。先向用户提出拟议的编辑。
- **追加到任务本地笔记文件**（例如 `<task>/notes.md` 或扩展已有的笔记）——如果发现属于当前任务的记录，但不适合放进 PRD。
- **更新 `.sunshine/spec/`**——如果发现是项目级约定或陷阱，能帮助未来的任务。为此运行 `sunshine-update-spec` skill——`session-insight` 到发现为止。
- **只在心里记下**，在接下来的几轮里回答得更好，什么都不写。这对一次性回忆常常是正确的选择。

Sunshine 不规定唯一的去向。把每次回忆都强制塞进固定文件，会让文件长成噪声。让情况决定。

## 如何调用

完整 CLI 参考见 `references/cli-quick-reference.md`。80% 的情况是其中之一：

```bash
# 查找内容提到某个关键词的会话（项目范围是默认；
# 加 --global 可搜索本机上的每个项目）。
sunshine mem search "<keyword>"

# 转储某个会话的对话，可按 phase 或关键词过滤。
sunshine mem extract <session-id> --phase brainstorm
sunshine mem extract <session-id> --grep "<keyword>"

# 深入一个会话：命中最多的前 N 轮 + 周围上下文。
sunshine mem context <session-id> --turns 3 --around 2

# 还不知道 session id 时，先从 list + 过滤开始。
sunshine mem list --cwd <project-path>
sunshine mem projects   # → 列出活跃项目的 cwd，再缩小范围
```

Phase 切片（`--phase brainstorm|implement|all`）在 `sunshine task create` 和 `sunshine task start` 的边界处切分会话。做当前任务的 finish-work 回顾时，`--phase brainstorm` 恢复规划讨论，`--phase implement` 恢复执行循环。默认是 `all`。

## 触发模式

`references/triggering-patterns.md` 列出了更多逐字的用户表述（英文 + 中文），看到它们应当让你想到“调用 `mem`”——训练直觉时随手备查。

## 不在范围内

- `mem` 不编辑代码、不更新文件。任何写回都是你当下的决定。
- `mem` 对平台 JSONL 存储是只读的。不推送、不同步到远程。
- 本 skill 不替代 `sunshine-update-spec`（把发现提升为项目级指导的正确工具）或平台原生的任务 / 规范工作流。
