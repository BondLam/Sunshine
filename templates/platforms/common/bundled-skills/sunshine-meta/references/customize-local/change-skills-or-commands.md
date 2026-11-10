# 修改本地 Skill、命令、提示词与工作流

当用户想改变 AI 入口点、自动触发规则或显式命令行为时，编辑本地平台目录里的 skill、命令、提示词或工作流。

编辑之前，先给即将触碰的 skill 分类：

- **内置上游 skill**——`sunshine-meta`、`sunshine-spec-bootstrap`、`sunshine-session-insight`、`sunshine-channel`。事实来源在 Sunshine CLI 仓库的 `templates/platforms/common/bundled-skills/<name>/` 之下；Go 二进制用 `go:embed` 内嵌该目录，在 `sunshine init` / `sunshine update` 时由 `internal/platforms` 自动分发到每个已接平台的 skill 根目录。这里的本地编辑由 `.sunshine/.template-hashes.json` 追踪，下次更新时会被标记。
- **项目本地 skill**——`.{platform}/skills/` 之下的其他任何内容。属于用户；`sunshine update` 不刷新。

本文件其余部分用“skill”指本地文件；两种情况的覆盖与冲突规则不同。

## 先读这些文件

1. `.sunshine/workflow.md`
2. 目标平台的 skill/命令/提示词/工作流目录
3. 相关的 agent 或 hook 文件
4. 项目规则是否已存在于 `.sunshine/spec/`
5. `.sunshine/.template-hashes.json`——确认即将编辑的 skill 是上游所有（有条目）还是项目本地（无条目）

## 选择哪种入口类型

| 目标 | 建议 |
| --- | --- |
| AI 应当自动知道某项能力 | 添加或修改 skill。 |
| 用户想用命令手动触发 | 添加或修改命令/提示词/工作流。 |
| 团队项目约定 | 优先用 `.sunshine/spec/` 或项目本地 skill——绝不要用内置 skill 目录。 |
| 为用户自己的项目微调某个内置 skill（`sunshine-meta` 等） | 创建一个项目本地的同类 skill（名字不同）来覆盖意图，或编辑 `.sunshine/spec/`。内置 skill 目录内的编辑只存活到下一次 `sunshine update`，每次都需要做“keep”选择。 |
| 把改动贡献回上游 | 编辑 Sunshine CLI 仓库里的 `templates/platforms/common/bundled-skills/<name>/`，而不是已部署的副本。 |
| 修改 Sunshine 流程语义 | 同步 `.sunshine/workflow.md`。 |

## 修改 Skill

一个 skill 通常是：

```text
<skill-name>/
├── SKILL.md
└── references/
```

`SKILL.md` 应当简短，负责触发/路由。长内容放进 `references/`，让 AI 按需阅读。

frontmatter 的 description 应当写明何时使用该 skill。例如：

```yaml
description: "Use when customizing this project's deployment workflow and release checklist."
```

不要写“helpful project skill”这类含糊描述；它们可能错误触发。

### 内置 vs. 项目本地

同一种目录形态对应两种截然不同的归属模型：

| 方面 | 内置（`sunshine-meta`、`sunshine-spec-bootstrap`、`sunshine-session-insight`、`sunshine-channel`） | 项目本地 |
| --- | --- | --- |
| 事实来源 | Sunshine CLI 仓库里的 `templates/platforms/common/bundled-skills/<name>/` | 用户项目内部 |
| 分发 | 在 `sunshine init` / `sunshine update` 时由 `internal/platforms`（内嵌模板 `templates.Platforms`）自动分发到每个已接平台 skill 根目录 | 由用户（或其他 skill）创建，永不移动 |
| 哈希追踪 | 每个文件都记录在 `.sunshine/.template-hashes.json` 中；更新时出现冲突提示 | 不追踪 |
| 本地编辑 | 允许，但下次更新时会被标记为“modified by user” | 自由编辑 |
| 正确的定制方式 | 新增一个名字*不同*的项目本地 skill 来补充（或取代）内置 skill | 直接编辑文件 |

如果目标是“让我的项目 AI 在讨论 release notes 时表现不同”，答案几乎总是项目本地 skill，而不是对 `sunshine-meta/` 动手术。

## 修改命令/提示词/工作流

显式入口点应当说明：

- 用户如何触发它。
- 读哪些 `.sunshine/` 文件。
- 运行哪些 `sunshine` 子命令。
- 完成后如何汇报。

如果命令只是重复工作流规则，优先让它引用/阅读 `.sunshine/workflow.md`，而不是维护流程的第二份副本。

## 常见路径

> Go 版目前已接 22 个平台（Claude Code、Cursor、Codex、Kiro Code、Gemini CLI、Qoder、Pi Agent、ZCode、Trae、Oh My Pi、Snow CLI、OpenCode、DeepSeek Harness、Reasonix、Grok Build、Kimi Code、Kilo CLI、Antigravity、Devin、CodeBuddy、GitHub Copilot、Factory Droid）；下表三家平台的入口目录。

| 平台 | 入口目录 |
| --- | --- |
| Claude Code | `.claude/skills/`、`.claude/commands/` |
| Cursor | `.cursor/skills/`、`.cursor/commands/` |
| OpenCode | `.opencode/skills/`、`.opencode/commands/` |
| Codex | `.agents/skills/`、`.codex/skills/` |
| Gemini CLI | `.agents/skills/`、`.gemini/commands/` |
| Kiro | `.kiro/skills/` |
| Qoder | `.qoder/skills/`、`.qoder/commands/` |
| CodeBuddy | `.codebuddy/skills/`、`.codebuddy/commands/` |
| GitHub Copilot | `.github/skills/`、`.github/prompts/` |
| Factory Droid | `.factory/skills/`、`.factory/commands/` |
| Pi Agent | `.agents/skills/` |
| Reasonix | `.reasonix/skills/`（无独立 commands 目录；slash 命令内置于平台） |
| ZCode | `.zcode/skills/`、`.zcode/commands/` |
| Kilo / Antigravity / Devin | workflows + skills |

上面每个目录都是四个内置 skill 的部署目标。每个已接平台在 `sunshine init` 时收到完整副本，在 `sunshine update` 时刷新；无需手工接线。

## 添加项目本地 Skill

如果用户想记录团队私有的定制，创建一个项目本地 skill——绝不要把项目私有内容放进内置 skill 目录，因为 `sunshine update` 会覆盖它。

```text
.claude/skills/project-sunshine-local/
└── SKILL.md
```

对于多平台项目，在每个平台 skill 目录里添加等价版本，或在支持共享层的平台（Codex）上使用 `.agents/skills/`。

选一个**不**与内置集合冲突的名字：

- `sunshine-meta`
- `sunshine-spec-bootstrap`
- `sunshine-session-insight`
- `sunshine-channel`

重用名字会导致内嵌模板分发在下次更新时覆盖项目本地副本。常见约定是加上项目名前缀：`acme-sunshine-deploy`、`acme-sunshine-onboarding`。

## 注意事项

- 不要把每个平台的语法混进同一个文件。
- 不要只改一个平台的入口点，却声称所有平台都支持。
- 不要把长期工程约定藏进命令里；写进 `.sunshine/spec/`。
- 不要手工编辑任何 `.{platform}/skills/` 目录下的 `sunshine-meta/`、`sunshine-spec-bootstrap`、`sunshine-session-insight` 或 `sunshine-channel/` 内的文件并期望改动留存——它们是内置的，由 `sunshine update` 刷新。要么贡献到上游，要么添加一个补充它们的项目本地 skill。
- 在 `sunshine update` 报告某个内置 skill 文件“modified by you”冲突后，只有当你接受手工维护这一分歧时才选择 **keep**；否则接受覆盖，并把意图以项目本地 skill 的形式重新应用。
