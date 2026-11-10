# 内置 Skill

“内置 skill”是指随 Sunshine Go 二进制内嵌发布的多文件内置 skill（`go:embed` 打进二进制，随二进制分发，没有 npm 包、没有运行时下载）。与 marketplace skill（用户自行安装到自己的 `.claude/skills/` 或其他平台 skill 根目录）不同，内置 skill 由 `sunshine init` 自动写入每个已接平台的 skill 根目录，并由 `sunshine update` 保持同步。它们是 Sunshine 自身的一部分，不是第三方内容。

一个内置 skill 是 `templates/platforms/common/bundled-skills/<skill>/` 下的一个目录，其中已有自己的 `SKILL.md`（带 YAML frontmatter）及可选的 `references/`、asset 或其他支撑文件。Sunshine 把整个目录树按原样复制到每个平台的 skill 根目录，因此 references 保持可懒加载，而不会被压平成一个超大的 `SKILL.md`。

## 哪些算内置（与相邻概念的区别）

| 源码路径 | 类型 | 如何发布 |
| --- | --- | --- |
| `templates/platforms/common/bundled-skills/<name>/` | 内置 skill（多文件） | 整个目录复制到每个平台 skill 根目录 |
| `templates/platforms/common/skills/<name>.md` | 单文件工作流 skill | 包上 frontmatter，写为 `<root>/<name>/SKILL.md` |
| `templates/platforms/common/commands/<name>.md` | Slash 命令 / 提示词 | 写入每个平台的命令目录（`.claude/commands/sunshine/`、`.cursor/commands/sunshine-*.md`、Codex 转为 `.agents/skills/sunshine-*/SKILL.md` 等） |
| `templates/platforms/<platform>/skills/` | 平台专属 skill | 只写入该平台的目录 |
| `.claude/skills/<my-skill>/` 等之下的用户 skill | Marketplace 或用户自写 | Sunshine 完全不管 |

Sunshine CLI 绝不触碰任何不是由其自身模板加载器产生的内容。用户手工放进平台 skill 根目录的任何东西都会原样保留。

## 当前内置 Skill

该集合在运行时通过列出内嵌模板里 `platforms/common/bundled-skills/` 下的目录来发现：

| Skill | 用途 |
| --- | --- |
| `sunshine-meta` | 本 skill。向在用户项目内工作的 AI 解释本地 Sunshine 架构和定制入口。 |
| `sunshine-session-insight` | 包装 `sunshine mem` CLI，让 AI 知道何时及如何回溯过去的 Claude Code / Codex 会话日志。 |
| `sunshine-spec-bootstrap` | 平台中立的工作流，用于基于真实代码库创建或刷新 `.sunshine/spec/`。 |
| `sunshine-channel` | 能力 skill，教 AI 何时为多 Agent 协作、forum/thread 持久看板和分发者等待模式使用 `sunshine channel`。 |

列表在运行时发现，所以在 `bundled-skills/` 下新增一个目录，就是注册新 skill 所需的唯一一步（见下文“添加新的内置 Skill”）。

## 各平台内置 Skill 的落点

Go 版目前已接 22 个平台（Claude Code、Cursor、Codex、Kiro Code、Gemini CLI、Qoder、Pi Agent、ZCode、Trae、Oh My Pi、Snow CLI、OpenCode、DeepSeek Harness、Reasonix、Grok Build、Kimi Code、Kilo CLI、Antigravity、Devin、CodeBuddy、GitHub Copilot、Factory Droid）。已接平台都会收到完整的内置 skill 集合：

| 平台 | 内置 skill 根目录 |
| --- | --- |
| Claude Code | `.claude/skills/<skill>/` |
| Cursor | `.cursor/skills/<skill>/` |
| Codex | `.agents/skills/<skill>/` |
| Kiro Code | `.kiro/skills/<skill>/` |
| Gemini CLI | `.agents/skills/<skill>/` |
| Qoder | `.qoder/skills/<skill>/` |
| Pi Agent | `.agents/skills/<skill>/` |
| ZCode | `.zcode/skills/<skill>/` |
| Trae | `.trae/skills/<skill>/` |
| Oh My Pi | `.omp/skills/<skill>/` |
| Snow CLI | `.snow/skills/<skill>/` |
| OpenCode | `.opencode/skills/<skill>/` |
| DeepSeek Harness | `.agents/skills/<skill>/` |
| Reasonix | `.reasonix/skills/<skill>/` |
| Grok Build | `.grok/skills/<skill>/` |
| Kimi Code | `.agents/skills/<skill>/` |
| Kilo CLI | `.kilocode/skills/<skill>/` |
| Antigravity | `.agent/skills/<skill>/` |
| Devin | `.devin/skills/<skill>/` |

Codex、Gemini CLI、Pi Agent、DeepSeek Harness、Kimi Code 五家共享 `.agents/skills/` 根目录。多个平台共同写入同一文件时，内容必须字节相同（这五家用中性解析，见 `internal/platforms` 的占位符处理）。

一份描述，两个消费者：

1. `sunshine init` → 平台安装器 → 把内嵌模板里的 bundled-skills 写到磁盘。Claude Code、Codex 各自处理少量 Map 无法表达的工作（可选的 `--with-statusline` flag、有意留空的 `.codex/skills/` 目录、一次性控制台提示）——它们都不重述文件列表。
2. `sunshine update`（阶段 4）→ 同一套内嵌模板，用于检测漂移并填充 `.sunshine/.template-hashes.json`。

因为两个消费者读取同一份描述，init 和 update 不可能对内置 skill 产生哪些文件意见不一。

## 分发接线（代码路径）

把内置 skill 自动分发到平台 skill 根目录的机制位于 Go 二进制内：

1. `templates/templates.go`
   - `//go:embed all:platforms` 把 `templates/platforms/` 整个打进二进制。
   - `templates.Platforms` 是内嵌模板文件系统。

2. `internal/platforms/`
   - `BundledSkillsPresent()` 检查内嵌模板里是否已有 bundled-skills。
   - 安装时遍历 `platforms/common/bundled-skills/` 下的每个 skill 目录，对每个文件做占位符解析，再写到 `<skillsRoot>/<skill>/<relativePath>` 之下。

每个已接平台都从同一个内嵌模板源拿到内置 skill——要么直接读，要么经平台安装器做同样的展开。

## 添加新的内置 Skill

形态和分发接线已经是通用的，所以添加 skill 只需要改文件并验证分发。

1. **创建目录树。**

   ```
   templates/platforms/common/bundled-skills/<my-skill>/
     SKILL.md                     # YAML frontmatter + 正文
     references/                  # 可选
       <topic>.md
     assets/                      # 可选（任何可按 utf-8 读取的内容）
   ```

2. **写一个有效的 `SKILL.md` 头。** frontmatter 至少必须包含：

   ```yaml
   ---
   name: <my-skill>
   description: "When the AI should reach for this skill. Triggering phrases go here."
   ---
   ```

   `description` 是每个平台自动触发机制匹配的内容，所以它应当描述用户意图触发条件，而不是 skill 的内部机制。

3. **在适当的地方使用占位符。** 内置 skill 内容会经过平台占位符解析。支持的 token 只有：`CMD_REF:name`、`CLI_FLAG`、`PYTHON_CMD`（Go 版解析为 `sunshine`，无 Python 运行时）、`EXECUTOR_AI`、`USER_ACTION_LABEL`，以及条件块 `AGENT_CAPABLE` / `HAS_HOOKS`（含 `^` 取反形式）。其他双花括号字样（例如文档措辞里的 `platform_name`、`python_cmd`）不是可解析 token，会原样保留。（注：此处 token 名特意不写双花括号——写成字面形式会在渲染时被解析器替换、把本句掏空。）

4. **不需要分发接线。** 内嵌模板的目录枚举会自动发现新目录，所以所有已接平台都会在下一次 `sunshine init` 或 `sunshine update` 时收到它。

5. **发布前验证分发路径**。跳过其中任何一步，历史上都曾导致功能被记录为内置，而已发布的二进制里却缺了文件：

   - 源码文件存在于将被打 tag 的分支上。
   - `go build ./...` 通过，且 `go:embed` 包含新目录（`go test ./...` 覆盖安装路径）。
   - 在全新的临时项目里，`sunshine init` 写入 `.claude/skills/<skill>/SKILL.md`、`.cursor/skills/<skill>/SKILL.md`、`.agents/skills/<skill>/SKILL.md`。
   - `.sunshine/.template-hashes.json` 列出生成的文件。
   - `sunshine update` 落地（阶段 4）后，在该临时项目里用它确认“已是最新”。

6. **如果 skill 是在其他项目会升级进入的版本中添加的，添加一条迁移 manifest 条目。** 没有显式 manifest 条目时，文件仍会通过 `sunshine update` 的标准“缺失文件”分支落盘，但 manifest 会让这一变化在 changelog 中可见。

## 本地覆盖内置 Skill

没有正式的“项目本地 skill”机制（例如 `.sunshine/skills/`）。内置 skill 是按平台扎根的，所以任何覆盖也是按平台扎根的。

受支持的模式依赖 `sunshine update` 中已有的模板哈希差异：

1. 直接编辑本地文件。例如：`.claude/skills/sunshine-meta/SKILL.md`。
2. 该文件的哈希现在与 `.sunshine/.template-hashes.json` 中的条目不一致。
3. 下一次 `sunshine update` 检测到用户修改并保持文件不动（Sunshine 未经显式 `--force` 绝不覆盖用户修改的文件）。

注意事项：

- 覆盖只适用于你编辑的那个平台的目录。要在例如 Claude Code 和 Codex 上覆盖同一个 skill，必须同时编辑 `.claude/skills/<name>/` 和 `.agents/skills/<name>/`。
- 未来的 `sunshine update --force` 会覆盖本地编辑。把覆盖纳入版本控制，以便需要时重新应用。
- 以不同文件夹名安装在同一平台 skill 根目录下的 marketplace skill（例如 `.claude/skills/my-custom-meta/`）不受 Sunshine 触碰，当目标是添加行为而不是修改内置 skill 时，它是更干净的选择。
- 团队私有约定属于 `.sunshine/spec/` 或独立的 marketplace 风格本地 skill，而不是对 `sunshine-meta` 本身的修改。见 `customize-local/add-project-local-conventions.md`。

## 从项目中删除内置 Skill

没有针对单个项目的内置 skill 退出开关。两种选择：

1. **删除每个平台 skill 根目录下的目录。** `sunshine update` 会发现文件缺失，对照 `.sunshine/.template-hashes.json`，并把删除当作任何其他用户修改一样对待——除非传入 `--force`，否则不会悄悄重新创建该目录。

2. **固定一个未发布该 skill 的 Sunshine 版本。** 内置 skill 集合在构建时确定，所以安装旧版二进制是永久排除当前版本所带 skill 的唯一方法。

第三种选择——全局禁用全部内置 skill——不受支持。分发是无条件的：平台安装器不接受“跳过内置 skill”的参数。添加一个开关意味着要改全部 19 个已接平台的安装签名。

## 运行规则

- 把 `templates/platforms/common/bundled-skills/` 当作内置 skill 存在的唯一事实来源。不要手工维护按平台的 skill 列表。
- 不要在内置 `SKILL.md` 内添加平台专属逻辑。如果某行为是平台专属的，把它放进 `templates/platforms/<platform>/` 下的平台专属模板。
- 不要把内置 skill 耦合到某个具体的 CLI 子命令（例如 `sunshine mem`），而不把该依赖在 skill 的 description 和 references 中表面化——旧版本的用户可能没有该命令。
- 不要在内置 skill 里存储项目私有内容。内置 skill 是公开的，会发给每个用户；项目规则属于 `.sunshine/spec/` 或本地 skill。
