# DeepSeek Harness（dsh）上的 Sunshine

dsh 是 **class-2 pull-based** Sunshine 宿主：没有 session-start hook 自动注入
工作流上下文，agent 通过 skill-loader 工具按需加载 Sunshine skill。

| 能力 | 状态 |
| --- | --- |
| Skill（`.agents/skills/sunshine-*/SKILL.md`） | 可用——dsh 原生发现该共享根目录 |
| 入口 skill（`.dsh/skills/sunshine-*/SKILL.md`） | 可用——dsh 自己的项目 skill 根目录（最高优先级） |
| 上下文 hook | 无——pull-based：skill 直接读 `.sunshine/` 下的文件 |
| 子代理 | 未提供——implement/check/research 通过工作流 skill 在主会话内联运行 |

## 快速开始

```bash
sunshine init --dsh -u your-name
dsh web        # 或：dsh --profile headless "start a Sunshine task for ..."
```

在 dsh 里：

1. 在项目根目录打开一个会话，用自然语言描述工作。新任务应让 agent 加载
   `sunshine-start` skill，它从 `.sunshine/` 读取当前任务状态，再分流到
   `sunshine-brainstorm`（需求不清晰）、`sunshine-before-dev`（将要写代码）、
   `sunshine-check`（写完代码）或 `sunshine-update-spec`（学到了值得记录的东西）。
2. 入口 skill 是 `.dsh/skills/` 下的 `sunshine-start` / `sunshine-continue` /
   `sunshine-finish-work`。随时也可以直接按名字让 agent 加载它们。
3. `/sunshine:finish-work` 是其他宿主的斜杠命令写法——dsh 没有斜杠命令面板，
   因此请说“完成 sunshine 任务”，agent 会加载 `sunshine-finish-work`。

## 文件对照

- `.agents/skills/`——自动触发的工作流 skill（`sunshine-before-dev`、
  `sunshine-brainstorm`、`sunshine-check`、`sunshine-break-loop`、
  `sunshine-update-spec`），以及内置的 `sunshine-meta` /
  `sunshine-spec-bootstrap` / `sunshine-session-insight` skill。与 Codex /
  Gemini CLI / Pi / Kimi 写入同一共享根目录的内容逐字节一致。
- `.dsh/skills/`——dsh 私有的入口 skill（`sunshine-start` /
  `sunshine-continue` / `sunshine-finish-work`）。
- `.sunshine/`——规范、任务、工作区记忆，以及 skill 调用的共享命令
  （`sunshine task context`、`sunshine task` 等）。

## 说明

- skill 脚本调用 `sunshine task context` 时传 `--platform dsh`；该值用作
  平台维度的上下文 key。
- 附带的 `minimal` agent 预设只组合 `bash` + `str_replace_editor`；默认预设
  还包括 `web_search`，以及 skill 依赖的文件系统/终端工具。
- dsh 没有项目级子代理定义面，因此 Sunshine 不在这里提供
  `sunshine-implement` / `sunshine-check` / `sunshine-research` agent
  提示词——工作流 skill 在主会话内联运行这些阶段。
