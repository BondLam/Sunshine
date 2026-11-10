<!-- SUNSHINE:COPILOT-GUIDANCE:START -->
# Sunshine 评审指引

评审启用了 Sunshine 的仓库时：

- 把 Sunshine 生成的运行时与平台载荷文件当作下游仓库里的复制集成文件。例如 `.sunshine/scripts/**`、`.sunshine/agents/**`、`.sunshine/workflow.md`，以及 `.agents/**`、`.claude/**`、`.codex/**`、`.cursor/**`、`.gemini/**`、`.github/**`、`.opencode/**`、`.qoder/**`、`.codebuddy/**`、`.factory/**`、`.pi/**`、`.reasonix/**`、`.trae/**`、`.zcode/**` 下的平台 Sunshine 文件。
- 除非该 PR 明确改动 Sunshine 集成、被复制的文件本身就是主要对象、文件泄露密钥、存在语法或接线损坏、或者与 PR 目标冲突，否则不要对这些复制文件逐行留风格、措辞或局部重构意见。
- 如果某个 Sunshine 生成的文件需要改行为，留一条交接意见，请上游 Sunshine 修复。写明受影响的路径、期望行为、观察到的证据或复现步骤，以及为什么这个修复属于上游。
- 仓库自有的 `.sunshine/spec/**`、`.sunshine/tasks/**`、应用代码、迁移、测试、文档与非 Sunshine 脚本，照常评审。
- 把同根因的重复发现合并成一条意见；如果已有确定性本地检查覆盖这类问题，就指向它。
<!-- SUNSHINE:COPILOT-GUIDANCE:END -->
