---
name: mem-recall
description: 通过 `sunshine mem` CLI 跨 Claude Code、Codex、Devin CLI、Grok、OpenCode、Pi 与 ZCode 搜索并回想过去的 AI 对话。用户想记住、查找或翻看以前 AI 会话里讨论过的任何内容时使用——跨平台、跨项目、跨时间。触发语例如“我之前跟 Claude/Codex 讨论过 X”、“上次怎么处理 Y”、“翻一下历史对话”、“我们当时怎么决定 X 的”、“为什么我们选了 X 而不是 Y”、“find what I said about Z”、“what did I discuss last week”、“the rationale for choosing X”、“find the brainstorm where we picked Z over alternatives”。即使用户没说“历史”或“回想”，只要提到过去 AI 对话里的内容就应触发本 skill。本工具直接从各平台的本地存储读取会话，不上传任何内容。
---

# Mem Recall（记忆回想）

面向 Claude Code、Codex CLI、Devin CLI、Grok、OpenCode、Pi 与 ZCode 的跨平台对话记忆。`sunshine mem` 命令读取各平台的本地会话存储，清理对话内容（去掉系统提示词、工具噪音、hook 注入，压缩摘要按正确方式处理），并提供一个聚焦的 5 个子命令 CLI，供回想工作流使用。Devin CLI 用 `--platform devin` 读取 `~/.local/share/devin/cli/sessions.db`——不是 `sunshine init --devin`（Desktop/Cascade），也不是 Factory Droid。

## 前置条件

已安装 Sunshine CLI：

```bash
sunshine --version
```

Sunshine 是 Go 单二进制发布的 CLI（无 npm 包、无 Python 运行时）：从发布渠道取得对应平台的二进制，放到 PATH 里即可。`sunshine mem` 随 CLI 一起提供，无需额外安装。用 `sunshine mem --help` 确认命令可用。

当前版本已支持 Cognition Devin CLI（`--platform devin`，`~/.local/share/devin/cli/sessions.db`）。这不是 `sunshine init --devin`（Desktop/Cascade），也不是 Factory Droid。Grok 已支持，且压缩前的对话轮次会保留；OpenCode 已恢复，经 SQLite 只读读取。

OpenCode 经零依赖 SQLite 读取器恢复。`--platform opencode` 读取 `~/.local/share/opencode/opencode.db`。

## 何时使用本 skill

用户问到以下任何一类问题时，主动使用：

- “我之前跟 Codex/Claude 讨论过 X，你了解下”
- “上次我们怎么解决 Y 的？”
- “翻一下历史看看”
- “我之前在 sunshine 项目里聊过哪些 plugin 设计？”
- “find sessions about memory architecture”
- “what did I tell another AI about this last week”

**设计决策类**（这类问题优先用 `--phase brainstorm`，见下文）：

- “我们当时怎么决定 X 的？”
- “之前讨论过 X 的 trade-off”
- “为什么我们选了 X 而不是 Y”
- “what was the design discussion about Y”
- “the rationale for choosing X”
- “find the brainstorm where we picked Z over alternatives”

不要多猜。只要用户的意图是“把过去的对话当作上下文来用”，就是本 skill 的服务范围。跨平台、跨项目的回想没有替代工具——`git log` 只能看到提交，你自己的记忆里也没有你在另一个 CLI 里说过什么的记录。

## 回想工作流（记住这个模式）

回想是两步钻取，不是单次查询。第一步锁定会话，第二步取实际内容。

```
第一步（发现）：sunshine mem search "<主题>" [--cwd <项目>] [--since <日期>]
第二步（钻取）：sunshine mem context <会话 id> --grep <主题> --turns 3 --around 1
```

如果用户没说清楚是哪个项目，先跑 `sunshine mem projects`，列出最近活跃的项目 cwd，再选最可能的那一个。

## 命令

### `sunshine mem projects`——列出活跃项目的 cwd

用户说“这个项目”却没指明是哪个时，先用它。显示所有平台上不同的 cwd，按最近活跃时间排序，并给出每个平台的会话数。这是 AI 路由的入口。

```bash
sunshine mem projects --since 2026-04-20 --limit 10
```

输出：

```
2026-05-04 03:42  sessions= 1 (claude:1)  ~/workspace/nb_project/mem-poc
2026-05-04 01:00  sessions=114 (claude:51 codex:63)  ~/workspace/.../Sunshine
...
```

### `sunshine mem search <关键词>`——查找候选会话

对清理后的对话做多 token AND 搜索。返回按相关度排序的会话，每个匹配附一段摘录。默认只搜当前工作目录；用 `--global` 可跨所有项目搜索。

```bash
sunshine mem search "sunshine memory" --cwd ~/workspace/.../Sunshine --since 2026-04-13
```

每个会话的输出：

```
[claude  ] 2026-04-20 11:39  4cda3c7f-8f9  ~/.../Sunshine  score=6.027  hits=169 (u=27,a=142)  turns=37
    [user] 是我们的用户想要搞类似记忆系统的东西…
    [assistant] ## Memory plugin 调研结论…
```

**分数这样读**：`(3 × 用户命中数 + 助手命中数) / 总轮次数`。分数越高，说明主题越集中，而且是用户自己提起的。用户轮次的命中按 ×3 加权，因为用户的措辞是最强的主题信号——AI 的展开会把同一个词反复说，拉高原始计数。

**摘录是按段落对齐的块**，不是按字符截取的窗口。摘录尊重 Markdown / 代码块边界，并优先选择能明显包含全部查询 token 的块。当查询的多个 token 在一轮长对话里离得很远时，摘录会退而锚定**最罕见**的那个 token（区分度更高）。

### `sunshine mem context <会话 id>`——深入一个会话

`search` 挑出候选会话后，用它取具体的命中轮次及周围上下文。按 token 预算裁剪，可直接供 AI 使用。

```bash
sunshine mem context 4cda3c7f --grep memory --turns 3 --around 1
# 前 3 个命中轮次 + 每个命中轮次前后各 1 轮，总计 ≤6000 字符

sunshine mem context 4cda3c7f --turns 5 --around 0
# 不加 grep：返回前 5 轮（可以看看会话是怎么开场的）
```

默认预算 6000 字符（约 1500 token）；每轮上限是预算的一半。用 `--max-chars N` 调整。

### `sunshine mem extract <会话 id>`——导出清理后的对话

导出平台清理后的完整对话。用于长篇检查，不适合预算紧张的回想场景。

```bash
sunshine mem extract 4cda3c7f --grep memory   # 只保留匹配关键词的轮次
sunshine mem extract 4cda3c7f --json          # 结构化输出
```

### `sunshine mem extract --phase brainstorm`——只取讨论部分

一次 Sunshine 会话常常以头脑风暴开场（用户边想边说，AI 给出选项，某些备选被否决、某些决定被做出），用户运行 `sunshine task start` 之后才进入实现工作。`--phase` 标志沿着这条边界切片清理后的对话，这样你能找回**讨论**而不被实现噪音淹没——反之亦然。

**边界**：头脑风暴窗口是同一会话里 `sunshine task create ...` 与之匹配的 `sunshine task start ...` Bash 调用之间的全部内容。多任务会话会产生多个窗口。

**三个取值**：

| `--phase` | 得到的内容 |
|-----------|--------------|
| `all`（默认） | 完整清理后的对话——与没有这个标志时一样 |
| `brainstorm` | 仅 `[create, start)` 窗口——讨论 / 决策 |
| `implement` | 每个头脑风暴窗口之外的全部内容——实际工作本身 |

**何时优先用 `--phase brainstorm`**：设计决策类问题（“为什么我们当时选了 X”、“the trade-off discussion about Y”、“what alternatives did we reject”）在头脑风暴窗口内的信号密度远高于整个会话。直接用 `extract --phase brainstorm`，而不要用 `context --grep`——后者可能落在实现阶段中间，那里主题只是被顺带提到，并不是做决定的地方。

**示例**：

```bash
# 单个会话，只取头脑风暴部分
sunshine mem extract 4cda3c7f --phase brainstorm

# 多任务会话——输出按任务分隔：
#   --- task: my-feature ---
#   ## Human ...
#   --- task: another-task ---
#   ## Human ...
sunshine mem extract 4cda3c7f --phase brainstorm

# 只在头脑风暴窗口内过滤（--phase 先执行，再执行 --grep）
sunshine mem extract 4cda3c7f --phase brainstorm --grep "trade-off"

# 结构化输出：给下游脚本 / AI 提供窗口元数据
sunshine mem extract 4cda3c7f --phase brainstorm --json
# JSON 新增：
#   "phase": "brainstorm"
#   "windows": [{ "label": "my-feature", "startTurn": 1, "endTurn": 14 }, ...]
#   "groups":  [{ "label": "my-feature", "turns": [...] }, ...]
#   "turns":   [...]   // 扁平拼接，保持向后兼容

# 反过来：只要实现工作
sunshine mem extract 4cda3c7f --phase implement
```

**平台支持**：

| 平台 | `--phase brainstorm` / `implement` |
|----------|------------------------------------|
| Claude | 原生支持——在原始 JSONL 的 `tool_use` Bash 块上检测边界 |
| Codex | 原生支持——在 `function_call`（`exec_command`）事件上检测边界 |
| Devin | 原生支持——沿 `main_chain_id` 遍历，在 `exec` tool_calls 上检测边界 |
| Pi | 原生支持——在活跃分支的会话条目上检测边界 |
| OpenCode | 降级——返回完整对话 + `opencode-phase-unsupported` 警告 |

**边界情况均已优雅处理**：

- 找到 `create` 但后面没有 `start`（任务还在做）→ 窗口一直开到会话结束。
- 找到 `start` 但前面没有 `create`（任务在更早的会话里创建）→ 头脑风暴窗口为 `[0, start)`。
- 两者都没找到 → 返回完整对话 + stderr 警告。

### `sunshine mem list`——枚举会话

主要用于浏览 / 调试。默认按项目过滤；用 `--global` 扩大范围。

```bash
sunshine mem list --since 2026-04-27
```

OpenCode 的子会话会显示 `↳ child of <父会话 id>` 标注。

## Flags 参考

```
--platform claude|codex|devin|grok|opencode|pi|zcode|all   默认 all
--since YYYY-MM-DD                     下限（含当天）
--until YYYY-MM-DD                     上限（含当天）
--global                               包含所有项目（默认：只搜当前 cwd）
--cwd <路径>                           覆盖项目 cwd
--limit N                              输出上限（默认 50）
--grep KW                              extract / context：按关键词过滤轮次
--turns N                              context：取前 N 个命中轮次（默认 3）
--around N                             context：每个命中轮次前后取几轮（默认 1）
--max-chars N                          context：字符预算（默认 6000）
--phase brainstorm|implement|all       extract：按 [sunshine task create, start) 切片（默认 all；Claude、Codex、Devin 与 Pi 支持）
--include-children                     search / context：把 OpenCode 子 Agent 会话并入父会话
--json                                 输出 JSON
--help, -h                             显示帮助
```

以 `sunshine mem help` 为准，看规范的 flag 参考。

## 数据来源（按平台）

本工具直接读取以下位置。不跑守护进程，不建索引，不上传。

| 平台 | 存储位置 | 说明 |
|---|---|---|
| **Claude Code** | `~/.claude/projects/<编码后的 cwd>/*.jsonl` | 每个会话一个 JSONL；cwd 路径编码在目录名里（`/` 与 `_` → `-`） |
| **Codex** | `~/.codex/sessions/YYYY/MM/DD/rollout-*.jsonl` | 每个会话一个 JSONL；cwd 在首个事件的 `session_meta` 载荷里 |
| **Devin CLI** | `~/.local/share/devin/cli/sessions.db` | Cognition Devin CLI（WAL SQLite）。`--platform devin`。不是 `sunshine init --devin`（Desktop/Cascade），也不是 Factory Droid |
| **Grok** | `~/.grok/sessions/<url 编码后的 cwd>/<会话 id>/chat_history.jsonl` | cwd 经 URL 编码后作为目录名；`session_search.sqlite` 只是索引，不读 |
| **Pi** | 默认 `~/.pi/agent/sessions/`；可用环境变量覆盖；全局 `~/.pi/agent/settings.json`；项目内 `.pi/settings.json` | 每个会话一个 JSONL；相对 `sessionDir` 值从设置文件所在目录解析。项目本地设置按当前 cwd 或 `--cwd` 做项目范围发现，不做无限制的 `--global` 扫描。只提取活跃 `id` / `parentId` 分支 |
| **OpenCode** | `~/.local/share/opencode/opencode.db` | SQLite 的 `session` / `message` / `part` 表 |

## 清理规则（从原始数据里去掉什么）

本工具只提取真实的人与 AI 对话，并去掉：

- **系统 / 提示词注入**：`<system-reminder>`、`<workflow-state>`、`<INSTRUCTIONS>`、`<environment_context>`、`<permissions instructions>`、`<collaboration_mode>` 等（不区分大小写）
- **Bootstrap 轮次**：Codex 会把 AGENTS.md 前言作为首条用户消息注入——整轮直接丢弃，不只是去掉标签
- **工具调用及其结果**：只保留 `text` 块
压缩前的轮次会**保留**，并标注压缩发生的位置。平台没有以可读方式存储的内容，会明确报告而不是悄悄省略——Codex 会加密 Agent 之间的消息，Grok 把压缩前的轮次以渲染后的 Markdown 保存在 `<会话>/compaction/` 下。

这意味着搜索命中是“实际对话确实讨论过这个”的可靠信号，而不是“关键词出现在某段 hook 注入里”。

## 跨平台子 Agent 语义

| 平台 | 子 Agent 存储方式 | 能找回吗？ |
|---|---|---|
| Claude | 同一个 JSONL——主 Agent 的 `Agent` / `Task` tool_use 记录了提示词；tool_result 里有最终输出。**子 Agent 的内部轮次没有被记录** | 只有提示词 + 最终结果 |
| Codex | 每次 `codex exec` 派生都产生新的 rollout JSONL，没有 `parent_id` 字段 | 按独立会话处理 |
| Devin | `sessions.db` 里的森林（`message_nodes`）；沿 `main_chain_id` / `parent_node_id` 遍历。分叉与回退会被丢弃 | 活跃主链 |
| Pi | 每个会话一个 JSONL；废弃分支会从活跃分支里去掉，但每个废弃分支的 `branch_summary` 条目会保留为一轮摘要 | 活跃分支 + 废弃分支摘要 |
| OpenCode | `session.parent_id` 链接子 Agent 链 | `--include-children` 会把后代并入父会话 |

`--include-children` 只对 OpenCode 搜索有实质影响——其他平台没有原生的 `parent_id` 可供合并。

## 完整示例：“上周我跟人讨论过 Sunshine 的记忆功能吗？”

```bash
# 1. 确认项目名（用户已明确说出时可跳过）
sunshine mem projects --since 2026-04-27
# → 找到 "~/workspace/.../Sunshine"，有 114 个会话

# 2. 查找候选会话
sunshine mem search "memory" \
  --cwd ~/workspace/.../Sunshine \
  --since 2026-04-27

# → 排第一：codex 019dcc75（分数 2.43，Codex memory 子 Agent + Sunshine hook）
#   其次：claude 12d26622（用户访谈“项目记忆 4 形态”）

# 3. 深入最相关的会话
sunshine mem context 12d26622 --grep memory --turns 3 --around 1
# → 返回实际的访谈问题块，列出 4 种记忆原型

# 4. 现在用找回的具体内容回答用户
```

除非用户明确想要完整会话，否则不要为了回想而跑 `extract`——它耗 token 预算，而且很少是必需的。

## 引用找回内容

向用户呈现找回的内容时，引用**会话 id + 实际引用的原句**。不要只说“我记得我们讨论过 X”而不给出依据——用户无法核实，也可能指的是另一场对话。

格式：

```
来自会话 12d26622（claude，2026-04-20）：
> 是我们的用户想要搞类似记忆系统的东西…

你那天提出了四种记忆原型：…
```

## 何时不要用本 skill

- 用户想搜索代码（用 `Grep` / `Read`）
- 用户想看提交历史（用 `git log` / `gh`）
- 用户想在当前项目里搜索文档 / 文件（用 `Read` / `Glob`）

本 skill 专门用于**找回过去 AI 对话的内容**，不是文件内容。

## 性能说明

- 项目范围、3 周的搜索：典型 Mac 上约 0.85 秒
- 全局搜索且无时间过滤：约 3 秒（扫描整机会话库）
- 每次调用都是无状态的——没有缓存，没有守护进程。冷启动与热启动表现相近，因为 macOS / Linux 的页缓存吸收了文件读取
- 交互使用时，优先用 `--cwd` + `--since` 缩小搜索范围
