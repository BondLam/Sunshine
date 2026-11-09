# 平台适配核对笔记（A 组，阶段 2）

对照源码：`~/workspace/research/trellis-source/packages/cli/src/`（commit `f089cb3`，
`@mindfoldhq/trellis` 0.6.17），只读核对，未修改该目录任何文件。
行为按需求清单（`~/workspace/your_files/trellis-feature-requirements.md` 第 5、11 模块）
重新实现，不复制 Trellis 源码。

## 1. 占位符替换表

源头：`configurators/shared.ts` 的 `resolvePlaceholders` /
`resolvePlaceholdersNeutral`；`types/ai-tools.ts` 的 `templateContext`。

| 占位符 | Claude Code | Cursor | Codex（平台解析） | Codex（中性解析，`.agents/skills/`） |
|---|---|---|---|---|
| `{{CMD_REF:name}}` | `/sunshine:name` | `/sunshine-name` | `$name` | `` `name` (Sunshine command) `` |
| `{{CLI_FLAG}}` | `claude` | `cursor` | `codex` | `codex` |
| `{{EXECUTOR_AI}}` | `Bash scripts or Task calls` | `Bash scripts or Task calls` | `Bash scripts or tool calls` | 同左（Codex 上下文） |
| `{{USER_ACTION_LABEL}}` | `Slash commands` | `Slash commands` | `Skills` | 同左（Codex 上下文） |
| `{{#AGENT_CAPABLE}}…{{/AGENT_CAPABLE}}` | 保留内容 | 保留内容 | 保留内容 | 保留内容（三家均为 true） |
| `{{^AGENT_CAPABLE}}…{{/AGENT_CAPABLE}}` | 删除 | 删除 | 删除 | 删除 |
| `{{#HAS_HOOKS}}…{{/HAS_HOOKS}}` | 保留内容 | 保留内容 | 删除 | 删除（Codex 为 false） |
| `{{^HAS_HOOKS}}…{{/HAS_HOOKS}}` | 删除 | 删除 | 保留内容 | 保留内容 |
| `{{PYTHON_CMD}}` | 见下 | 见下 | 见下 | 见下 |

核对结论：

- **本次范围内的模板实际用到的占位符只有 `{{CMD_REF:name}}`、`{{CLI_FLAG}}`**
  （`common/skills/update-spec.md` 用了 6 处 `{{CMD_REF:…}}`；
  `common/commands/{continue,start}.md` 各 1 处 `{{CLI_FLAG}}`）。
  `{{EXECUTOR_AI}}`、`{{USER_ACTION_LABEL}}` 与条件块在本次 3 平台的
  模板中均未出现；解析器仍按 `shared.ts` 完整实现，以备后续模板使用。
- **条件块解析后多余空行**：与 `shared.ts` 一致，3 个以上连续换行压成两个。
- **`{{PYTHON_CMD}}` 不再是占位符**：Go 版无 Python 运行时（PLAN.md 关键决策 1），
  模板翻译时已把脚本调用改写为 `sunshine` 子命令（见第 4 节映射表），
  内嵌模板中不再含有 `{{PYTHON_CMD}}`。解析器里把它映射为 `sunshine`
  只是兜底（例如 B 组 bundled-skills 文件中万一残留），规范模板不得依赖它。
  `Install` 在写出前校验：解析后仍残留任何 `{{…}}` 占位符即报错。
- **Trellis 原文的中性形式是 `` `name` (Trellis command) ``**；按改名规则，
  Go 版为 `` `name` (Sunshine command) ``。

## 2. Hook 事件与 Go 子命令的对应关系

源头：`templates/shared-hooks/index.ts` 的 `SHARED_HOOKS_BY_PLATFORM`、
各平台 `settings.json` / `hooks.json`、各 configurator。

| 平台 | Trellis hook 脚本（SHARED_HOOKS_BY_PLATFORM 分配） | 配置中的事件 | Go 版命令 |
|---|---|---|---|
| Claude | `session-start.py`、`inject-workflow-state.py`、`inject-subagent-context.py` | SessionStart（startup/clear/compact） | `SUNSHINE_HOOK_PLATFORM=claude sunshine hook session-start` |
| Claude | 同上 | PreToolUse（Task/Agent） | `SUNSHINE_HOOK_PLATFORM=claude sunshine hook subagent-context` |
| Claude | 同上 | UserPromptSubmit | `SUNSHINE_HOOK_PLATFORM=claude sunshine hook workflow-state` |
| Cursor | `session-start.py`、`inject-shell-session-context.py`、`inject-subagent-context.py` | sessionStart | `SUNSHINE_HOOK_PLATFORM=cursor sunshine hook session-start` |
| Cursor | 同上 | preToolUse（Task\|Subagent） | `SUNSHINE_HOOK_PLATFORM=cursor sunshine hook subagent-context` |
| Cursor | 同上 | beforeShellExecution | `SUNSHINE_HOOK_PLATFORM=cursor sunshine hook shell-session` |
| Codex | `inject-workflow-state.py`、`inject-subagent-context.py`（共享） | UserPromptSubmit | `SUNSHINE_HOOK_PLATFORM=codex sunshine hook workflow-state` |
| Codex | 同上 | SubagentStart（matcher `^(?:sunshine-implement\|sunshine-check\|sunshine-research)$`） | `SUNSHINE_HOOK_PLATFORM=codex sunshine hook subagent-context` |

核对结论：

- Go 版**不写任何 `.py` 文件**，也不写 `python`/`python3` 命令；hook 配置里
  的命令字符串一律是 `SUNSHINE_HOOK_PLATFORM=<平台> sunshine hook <事件>`
  （C 组接线要求：`sunshine hook` 靠 `SUNSHINE_HOOK_PLATFORM` 环境变量
  识别调用方平台，Go 二进制无法像 Python 那样从 argv[0] 所在目录推断平台；
  不加前缀时 hook 仍能工作，但 Codex 的 `<codex-mode>` 横幅与 bootstrap
  提示不会出现，shell 票据的 platform 字段为空）。事件名固定为
  `session-start`、`workflow-state`、`subagent-context`、`shell-session`、`statusline`。
  各平台前缀：Claude `SUNSHINE_HOOK_PLATFORM=claude`、Cursor
  `SUNSHINE_HOOK_PLATFORM=cursor`、Codex `SUNSHINE_HOOK_PLATFORM=codex`。
  `--with-statusline` 注入的 `statusLine` 命令同样带 `claude` 前缀
  （与 hooks 段同一机制，C 组补充要求只点名了 hooks 段与两份 hooks.json，
  statusLine 一并加前缀是 A 组为一致性做的决定，见汇报）。
- Claude 的 `--with-statusline` 才注入 `statusLine` 段；默认不装
  （与 `claude.ts` 的 opt-in 逻辑一致）。
- Codex `hooks.json` 不写 `-X utf8`（Go 天生 UTF-8）；SubagentStart 的
  matcher 已由源码的 `^(?:trellis-implement|trellis-check|trellis-research)$`
  改为 `^(?:sunshine-implement|sunshine-check|sunshine-research)$`，
  实测确认（见 `TestInstallCodex` 与 `TestHookCommandPlatformPrefix`）。
- **Codex 自有的 `templates/codex/hooks/session-start.py`（552 行）已接线（2026-11-09 用户确认）**：
  它输出 Codex hook 协议 JSON（`hookSpecificOutput.hookEventName = "SessionStart"`，
  含中文任务名/路径的 UTF-8 处理、任务上下文拼装）。Trellis 的 `templates/codex/hooks.json`
  没有注册 SessionStart 事件（只有 UserPromptSubmit 与 SubagentStart），属上游遗漏；
  Codex 官方支持 SessionStart 事件（startup/resume/clear/compact）。Go 版在
  `.codex/hooks.json` 补上 SessionStart 条目 → `SUNSHINE_HOOK_PLATFORM=codex sunshine hook session-start`
  （timeout 30，不设 matcher，全来源触发）；Go 不写 `.py` 文件，`RunSessionStart`
  本就输出同一协议 JSON，无需改 hook 实现。
- **跨组契约**：agent 模板中引用的 hook 注入标记已改名为
  `<!-- sunshine-hook-injected -->`，`sunshine hook subagent-context` /
  `session-start` 的实现（C 组）注入上下文时必须输出**这个**标记，
  否则 agent 的“标记存在 → 直接开工 / 标记缺失 → 走兜底”分支会误判。
  同理，`Full hook output saved to: <path>` 是 hook 与 agent 约定的字面量，
  翻译时原样保留，hook 实现输出该提示时措辞不得改动。

## 3. settings.json 合并规则核对结果

源头：`commands/update.ts`（`preserveExistingClaudeStatusLine`，约 706–740 行）、
`configurators/claude.ts`（`injectStatusLine`）、`utils/file-writer.ts`（写出模式）。

**核对发现（与 PLAN.md / 任务描述的出入，见第 5 节第 1 条）**：
Trellis `update.ts` 里**没有**通用的 settings.json 合并逻辑。它对
settings.json 的唯一特殊处理是 `preserveExistingClaudeStatusLine`：
仅当磁盘已有 `statusLine`、而新模板没有 `statusLine` 时，才把已有的
`statusLine` 带进新模板；其余所有顶层键不做合并——settings.json 走
与其他模板文件相同的哈希比对/冲突流程（未改动则覆盖、改动过则提示冲突）。

Go 版 `Install` 实现的合并是任务与 PLAN.md 指定的 Install 行为
（比 Trellis 的 update 流程更进一步），规则如下：

- 已有顶层键一律保留（顺序与值都不动），包括用户自设键与已有的 `statusLine`；
- 模板有、已有缺失的顶层键按模板顺序追加到末尾；
- `env`：两边都是对象时按变量名补写缺失项，已有变量的值不动；
- `hooks`：两边都是对象时按事件名补写：缺失的事件整体追加，已有的事件
  把模板里尚未出现的条目（按内容判重）追加到末尾；
- `statusLine`：已有则保留；缺失且 `--with-statusline` 时追加模板的
  `statusLine`（`sunshine hook statusline`）；缺失且未传该 flag 时不写。
  这与 `update.ts` 的 statusLine 保留分支逐条对应；
- 已有 settings.json 解析失败（非法 JSON）时无法合并：`Force` 为假保留原文件
  跳过，为真用模板内容覆盖；
- 键顺序用保序 JSON 解析/序列化实现（`jsonorder.go`），新键追加末尾，
  与 Trellis“解析 → 末尾赋值 → stringify(null, 2)”的字节行为一致；
  字符串不转义 HTML 字符（与 `JSON.stringify` 一致）。

**阶段 4 定稿（2026-11-09，A 组）**：上面这份合并已删除。`Install` 的
settings.json 改回 Trellis 行为——与普通模板文件完全相同（不存在写、
相同跳过、不同时 Force 为假跳过、为真覆盖）；`mergeSettingsJSON` /
`mergeEnvObject` / `mergeHooksObject` / `canonicalJSON` 已删除，
`jsonorder.go` 只剩保序解析/序列化（供 `injectStatusLine` 与 update 的
statusLine 保留用）。`sunshine update` 里 settings.json 走正常哈希冲突
流程，写盘前仅 `statusLine` 字段按 `preserveExistingClaudeStatusLine`
保留用户现有值（见 `internal/update`）。

## 4. Python 脚本调用的 Go 映射表（翻译时改写）

Go 版无 `.sunshine/scripts/`（PLAN.md 关键决策 1：hooks 不用 Python；
任务系统在阶段 3 才实现）。模板翻译时，源码中的 Python 脚本调用
按以下映射改写为 `sunshine` 子命令；其中 `task create/archive/start/current`
有 PLAN.md 明确依据（`sunshine task start/current/finish`），
`task context` 与 `task add-session` 是暂定映射，待阶段 3 任务系统定稿时确认：

| Trellis 原文 | Sunshine 模板 |
|---|---|
| `{{PYTHON_CMD}} ./.trellis/scripts/task.py create …` | `sunshine task create …` |
| `` `task.py start` ``（含 `task.py archive`、`task.py current --source`） | `` `sunshine task start` `` / `sunshine task archive …` / `sunshine task current --source` |
| `{{PYTHON_CMD}} ./.trellis/scripts/get_context.py [--mode …]`、`python3 ./.trellis/scripts/get_context.py …` | `sunshine task context [--mode …]` |
| `{{PYTHON_CMD}} ./.trellis/scripts/add_session.py …` | `sunshine task add-session …` |
| `get_context.py`（正文提及） | `sunshine task context` |
| `add_session.py` / `task.py archive`（正文提及） | `sunshine task add-session` / `sunshine task archive` |
| `.trellis/scripts/`（agent 禁写列表等） | 按改名规则译为 `.sunshine/scripts/`（字面改名；该目录在 Go 版尚不存在） |

## 5. 与 PLAN.md / 需求清单的偏差（以源码为准的差异，逐条）

1. **settings.json 合并**：PLAN.md 阶段 2 与任务描述称“合并规则以 Trellis
   update.ts 的 settings 合并逻辑为准”；实际核查 `update.ts` 只有
   `preserveExistingClaudeStatusLine`（statusLine 单键保留），不存在
   hooks/env 合并。Go 版 Install 仍按任务/PLAN 指定的合并实现（见第 3 节），
   但此处明确记录：该合并**不是** Trellis update.ts 的行为，阶段 4 不可
   把它当作“与 Trellis 一致”来复用。
2. **Codex Name**：`ai-tools.ts` 登记的 Codex 全名是
   `Codex (also writes .agents/skills/ — read by Cursor, Gemini CLI, GitHub Copilot, Amp, Kimi Code)`；
   任务指定 `Name` 为短名 `Codex`，按任务执行。`.agents/skills/` 由 Codex
   写入、供多家读取的事实仍在实现中（skills 写到共享目录）。
3. **Codex hooks 旧版开关**：PLAN.md 写“旧版 `codex_hooks = true`”；
   `codex.ts` 注释更精确：0.129+ 上旧写法是 `features.codex_hooks = true`
   （仍可用但报弃用警告），0.129 之前只接受 `codex_hooks`。
   `CodexHooksHint()` 按 PLAN.md 的措辞输出（与 D 组 init 文案一致）。
4. **命令过滤**：`shared.ts filterCommands` 只在 `agentCapable && hasHooks`
   时滤掉 `start`。Claude/Cursor 满足条件，只写 `continue`、`finish-work`；
   Codex（`hasHooks=false`）保留 `start` 并转成 `.agents/skills/` 下的 skill。
   与任务描述一致，无偏差。
5. **Codex agent 的 pull fallback 不是注入的**：`shared.ts` 的
   `applyPullBasedPreludeToml` 存在，但 `codex.ts collectCodexTemplates`
   **没有调用它**；Codex 三个 `.toml` 的 marker-gated pull fallback
   是模板正文自带的（`<!-- trellis-hook-injected -->` 标记判断 + 兜底步骤），
   翻译时原样保留并改名标记。Claude/Cursor 的 `.md` agent 同样是模板正文
   自带该协议（`claude.ts` / `cursor.ts` 均不注入 prelude）。
6. **Claude/Cursor agent frontmatter 描述不同**：两平台同名 agent 的正文
   完全相同，只有 frontmatter `description` 不同（Claude 是通用描述，
   Cursor 是强调“仅用此 agent、不要用通用 agent”的描述）。翻译时分别保留。
7. **bundled-skills 的占位符解析**：`shared.ts resolveBundledSkills` 用的是
   平台解析 `resolvePlaceholders`（不是中性解析），即使写到共享
   `.agents/skills/` 也是如此；Go 版照此实现（只有 5 个自动触发 skill
   在 Codex 侧用中性解析）。
8. **翻译时调整的 Trellis 仓库专属内容**（非行为差异，仅模板措辞）：
   - `before-dev.md` 的举例包名 `docs-site/`（Trellis 自身文档站）改为通用示例；
   - `break-loop.md` 的“同步模板到 `src/templates/markdown/spec/`”
     （Trellis 仓库自身路径）改为“如项目有从中派生的模板，也一并同步”；
   - `codex/agents/sunshine-implement.toml` 的“修改 `.trellis/scripts/` 时
     保持 `packages/cli/src/templates/trellis/scripts/` 同步”
     （Trellis 仓库自身路径）改为“修改 `.sunshine/` 下的任务脚本时同步更新
     模板中对应内容”。

## 6. 遗留问题

- **bundled-skills 已合入并验证通过**：交付时 B 组已写入
  `templates/platforms/common/bundled-skills/`（4 个 skill、38 个文件，
  目录名均为 `sunshine-<base>`）。`Install` 的写出已实测：
  三平台均写出 4 个 skill 的 `SKILL.md`（frontmatter `name` 已改名）
  与整目录 references；`TestInstallBundledSkills` 覆盖。
  但有一个 B 组翻译口径遗留：bundled references 中仍有约 30 处
  `python3 ./.sunshine/scripts/task.py …` / `get_context.py` /
  `add_session.py` / `init_developer.py` 形式的调用原文（仅改名，未按
  第 4 节映射表改写为 `sunshine task …`），另有 1 处 `python3 -c`
  的 JSONL 打印单行命令与 1 处 “Python hook” 散文提及。
  这些文件属 B 组，A 组未改；`Install` 按 Trellis `resolveBundledSkills`
  行为只做占位符解析、原样写出。建议阶段 3 任务系统定稿时，
  由 B 组或统一个人按第 4 节映射表改写这些调用（`task.py` 的
  `add-context` / `validate` / `list-context` / `add-subtask` /
  `remove-subtask` 等子命令在第 4 节映射表之外，需一并定名）。
- **`sunshine task context` / `sunshine task add-session` 是暂定映射**
  （见第 4 节），待阶段 3 任务系统定稿时确认子命令名；若定名不同，
  需同步修改 `common/skills/` 与 `common/commands/` 下的模板。
- **阶段 4 update 的 settings.json 策略待定**（见第 3 节末尾）。
- **Codex `session-start.py` 未接线**（见第 2 节）：是否补接线需另行决策。
