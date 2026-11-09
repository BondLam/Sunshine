# Init 后生成的本地文件

`sunshine init` 把 Sunshine 运行时写入用户项目。之后，`sunshine update` 尝试更新 Sunshine 管理的模板文件，但它用 `.sunshine/.template-hashes.json` 判断哪些文件已被用户修改过。

本页只描述用户项目内可见且可编辑的文件。

## `.sunshine/`

```text
.sunshine/
├── workflow.md
├── config.yaml
├── .developer
├── .version
├── .template-hashes.json
├── .runtime/
├── agents/
├── spec/
├── tasks/
└── workspace/
```

| 路径 | 通常可编辑？ | 说明 |
| --- | --- | --- |
| `.sunshine/workflow.md` | 是 | 本地工作流文档和 AI 路由规则。 |
| `.sunshine/config.yaml` | 是 | 项目配置、hook、包、日志行数限制及相关设置。 |
| `.sunshine/spec/` | 是 | 项目规范，预期由用户和 AI 定期更新。 |
| `.sunshine/tasks/` | 是 | 任务材料和研究产物，由任务工作流维护。 |
| `.sunshine/workspace/` | 是 | 会话记录，通常由 `sunshine task add-session` 写入。 |
| `.sunshine/agents/` | 是 | channel 运行时 agent 定义（architect、check、implement、plan、research），`sunshine channel spawn --agent <name>` 消费。 |
| `.sunshine/.runtime/` | 否 | 运行时状态，通常由 hook/命令自动写入。 |
| `.sunshine/.developer` | 谨慎 | 当前开发者身份。 |
| `.sunshine/.version` | 否 | update/迁移逻辑使用的 Sunshine 版本记录。 |
| `.sunshine/.template-hashes.json` | 否 | 模板哈希记录。不要在这里手写业务规则。 |

## 平台目录

Go 版目前只接 Claude Code、Cursor、Codex 三家；其余平台在滚动轨道待扩展。不同平台生成不同目录。常见类别：

| 类别 | 示例路径 | 用途 |
| --- | --- | --- |
| hooks | `.claude/settings.json` 注册的 `sunshine hook` 调用、`.cursor/hooks.json`、`.codex/hooks.json` | 注入会话上下文、workflow-state 和子 Agent 上下文。 |
| settings | `.claude/settings.json`、`.codex/hooks.json`、`.codex/config.toml`、`.cursor/hooks.json` | 告诉平台何时运行 hook。 |
| agents | `.claude/agents/`、`.cursor/agents/`、`.codex/agents/` | 定义 `sunshine-research`、`sunshine-implement` 和 `sunshine-check` 这类 agent。 |
| skills | `.claude/skills/`、`.cursor/skills/`、`.agents/skills/`（Codex 共用） | 自动触发或可由 AI 阅读的 skill。 |
| commands/prompts/workflows | `.claude/commands/sunshine/`、`.cursor/commands/sunshine-*.md`、`.agents/skills/sunshine-*/SKILL.md`（Codex 的命令转 skill） | 用户显式调用的命令或工作流入口点。 |

修改平台目录时，也要确认 `.sunshine/workflow.md` 是否仍描述同一流程。

## 模板哈希的含义

`.sunshine/.template-hashes.json` 记录 Sunshine 上次写入模板文件时的内容哈希。`sunshine update` 用它区分三种情况：

| 情况 | 更新行为 |
| --- | --- |
| 文件未被用户修改 | 可以自动更新。 |
| 文件已被用户修改 | 提示用户选择覆盖、保留，或生成 `.new`。 |
| 文件已不再是当前模板 | 按迁移规则删除、重命名或保留。 |

当 AI 定制本地 Sunshine 文件时，无需手工维护哈希。Sunshine update 把结果识别为“modified by the user”是正常的。

## 本地定制边界

默认可编辑：

- `.sunshine/workflow.md`
- `.sunshine/config.yaml`
- `.sunshine/spec/**`
- `.sunshine/agents/**`
- 平台 hook 注册、设置、agent、skill、命令、提示词和工作流

默认不要编辑：

- Sunshine 二进制的安装位置
- Sunshine GitHub 仓库源码
- `.sunshine/.runtime/**` 下的具体状态文件
- `.sunshine/.template-hashes.json` 内的哈希内容

只有当用户明确想向上游贡献时，才切换到 Sunshine CLI 源码视角。
