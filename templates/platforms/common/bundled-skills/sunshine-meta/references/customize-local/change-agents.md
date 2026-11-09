# 修改本地 Agent

当用户想改变 `sunshine-research`、`sunshine-implement` 或 `sunshine-check` 的行为时，编辑用户项目里的平台 agent 文件。

> Go 版目前只接 Claude Code、Cursor、Codex 三家；下表其余平台的路径是目标形态，待滚动轨道扩展。以用户项目里的实际路径为准。

## 先读这些文件

1. 目标平台 agent 目录
2. `.sunshine/workflow.md` 的 Phase 2 / research 路由
3. 当前任务的 `prd.md`
4. 当前任务的 `implement.jsonl` / `check.jsonl`
5. 相关的 hook 或 agent 前奏

## 常见路径

| 平台 | 路径 |
| --- | --- |
| Claude Code | `.claude/agents/sunshine-*.md` |
| Cursor | `.cursor/agents/sunshine-*.md` |
| OpenCode | `.opencode/agents/sunshine-*.md` |
| Codex | `.codex/agents/sunshine-*.toml` |
| Kiro | `.kiro/agents/sunshine-*.json` |
| Gemini CLI | `.gemini/agents/sunshine-*.md` |
| Qoder | `.qoder/agents/sunshine-*.md` |
| CodeBuddy | `.codebuddy/agents/sunshine-*.md` |
| Factory Droid | `.factory/droids/sunshine-*.md` |
| Pi Agent | `.pi/agents/sunshine-*.md` |
| Reasonix | `.reasonix/skills/sunshine-*/SKILL.md`（subagent frontmatter） |
| ZCode | `.zcode/agents/sunshine-*.md` |

以用户项目里的实际路径为准。

## 常见需求

| 需求 | 编辑哪个 agent |
| --- | --- |
| Research 必须写文件，而不只在聊天里回复 | `sunshine-research` |
| 实现前必须先读某些本地规范 | `sunshine-implement` + `implement.jsonl` 配置规则 |
| 检查期间必须运行特定命令 | `sunshine-check` |
| Agent 不得修改某些目录 | 对应 agent 的写边界说明 |
| Agent 输出格式必须固定 | 对应 agent 的最终/汇报说明 |

## 修改原则

1. **保持角色边界**：research 调查并持久化；implement 编写实现；check 评审并修复。
2. **不要把项目规范硬编码进 agent**：长期规范属于 `.sunshine/spec/`；agent 负责阅读它们。
3. **让阅读顺序明确**：当前任务 -> PRD -> 信息 -> JSONL -> 规范/研究。
4. **让写边界明确**：哪些目录可以写，哪些不可以。
5. **跨平台同步**：当用户配置了多个平台时，决定是只改当前平台，还是改全部平台的 agent。

## Agent 拉取型平台

如果 agent 文件里含有“启动后读取任务/上下文”的前奏，编辑时不要删除这些步骤。否则 agent 将只凭聊天上下文工作，绕过 Sunshine 的核心机制。

## Hook 推送型平台

如果上下文由 hook 注入，agent 文件仍应保留责任边界。不要因为 hook 注入了上下文，就从 agent 里删除 PRD/规范要求。
