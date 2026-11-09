# 修改本地 Hook

Hook 是把平台连接到 Sunshine 的自动化层。当用户想改变“上下文何时注入”、“shell 命令如何继承会话”或“agent 启动前读哪些文件”时，hook 通常是编辑点。

Go 版没有本地 hook 脚本：全部 hook 行为都在 Sunshine 二进制内，由 `sunshine hook <事件>` 子命令实现（`session-start` / `workflow-state` / `subagent-context` / `shell-session` / `statusline`）。项目内能改的是平台设置文件里的 hook **注册**（调哪个子命令、传什么环境变量），以及 hook 所读的 `.sunshine/workflow.md` 与任务文件；hook 行为本身不在项目内改。

## 先读这些文件

1. 目标平台的设置/配置，例如 `.claude/settings.json`、`.codex/hooks.json`、`.cursor/hooks.json`（Go 版目前只接 Claude Code、Cursor、Codex 三家）
2. `.sunshine/workflow.md`
3. `sunshine hook --help`（列出全部 hook 子命令）

## 常见 Hook 类型

| Hook（`sunshine hook` 子命令） | 用途 |
| --- | --- |
| session-start | 在会话开始、清空或压缩时注入 Sunshine 总览。 |
| workflow-state | 在每次用户输入时注入状态提示。 |
| subagent-context | 在 agent 启动前注入 PRD/规范/研究。 |
| shell-session | 让 shell 里的 `sunshine task` 命令看到同一会话身份。 |
| statusline | 状态栏信息（仅 Claude Code，`--with-statusline` 时安装）。 |

## 修改步骤

1. 在设置/配置中找到 hook 注册（命令形如 `SUNSHINE_HOOK_PLATFORM=<平台> sunshine hook <事件>`）。
2. 确认 `sunshine` 二进制在 PATH 里，且项目有 `.sunshine/`（无 `.sunshine/` 时 hook 静默退出 0，不注入也不报错）。
3. 手工复现：用模拟的 stdin JSON 运行 `sunshine hook <事件>`，确认输入、输出与注入内容。
4. 改注册或改 hook 所读的 `.sunshine/workflow.md` / 任务文件；hook 行为本身不在项目内改。
5. 如果 hook 依赖工作流内容，同步 `.sunshine/workflow.md`。

## 示例：修改新会话注入内容

先找到 session-start hook 注册：

```text
.claude/settings.json        # SessionStart 事件 → sunshine hook session-start
```

注入内容的组装在 Go 二进制内；想让新会话看到更多/更少内容，改 `.sunshine/workflow.md` 的总览说明或任务上下文，而不是在设置里硬编码内容。

## 示例：Agent 没读 JSONL

先确认：

```bash
sunshine task current --source
sunshine task validate <task>
```

如果任务和 JSONL 都正确，判断平台使用 hook 推送还是 agent 拉取。hook 推送时检查 `sunshine hook subagent-context` 的注册与输出；agent 拉取时编辑 agent 文件。

## 注意事项

- 设置负责注册，hook 子命令负责行为；两者一起检查。
- 不同平台支持不同的 hook 事件。不要直接照搬另一个平台的设置。
- Hook 应当读取项目本地的 `.sunshine/`；不应依赖 Sunshine 上游源码路径。
- Hook 失败时静默退出 0，绝不阻断 agent 会话；排查时手工运行 `sunshine hook <事件>` 看输出，而不是等它在会话里报错。
