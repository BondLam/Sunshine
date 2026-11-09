# 触发模式

逐字的用户表述，看到它们时 AI 应当想到调用 `sunshine mem`。对照这些校准直觉——如果用户消息命中其中一种模式而你没有调用 `mem`，你很可能错过了一次明显的回忆。

模式按表述背后的*意图*分组，而不是表面措辞。同一种意图会以不同语言和语气出现。

## 过去方案的回忆

用户在问“我们（或我）以前是怎么解决这个的”。过去对话里有答案；代码库只展示结果，不展示推理。

- "How did we solve this last time?"
- "What did we end up doing about X?"
- "We dealt with this once already, didn't we?"
- "上次怎么解的?"
- "之前是怎么搞定 X 的?"
- "我记得以前修过类似的"

调用：`sunshine mem search "<症状关键词>" --global --limit 10`，然后用 `context` 深入看起来最接近的命中。

## 决策取回

用户提到的决定存在于旧对话里，而不是任何已提交的文件中。去头脑风暴窗口里找。

- "What was the decision on X?"
- "Did we decide to use Postgres or SQLite?"
- "The rationale for choosing X over Y was…?"
- "我们当时为啥选了 X 而不是 Y?"
- "关于 X 我们之前是怎么定的?"
- "之前讨论过 X 的方案吗?"

调用：`sunshine mem search "<决策关键词>"` 找到会话，然后 `extract <id> --phase brainstorm` 恢复讨论。

## 跨会话继续

用户间隔一段时间后恢复工作，上下文是隐含的。

- "Where were we?"
- "Continue from last time."
- "Pick up where we left off."
- "继续上次的"
- "我们上次做到哪了"
- "接着昨天那个任务"

调用：`sunshine mem list --task <current-task-dir>` 找到与当前任务关联的最新会话，然后 `extract` 最后一个。

## 熟悉 bug 的调试

当前 bug 感觉像是已经见过的。过去会话里很可能有解决路径。

- "I feel like I've hit this before."
- "Doesn't this look like that bug from last month?"
- "Same kind of timeout I had in X."
- "这个错好像之前见过"
- "这个 bug 是不是上次那个?"
- "怎么又是这个 error?"

调用：`sunshine mem search "<错误信息片段>" --global`。锚定实际错误字符串中简短、有区分度的 token。

## 自我模式发现

用户在问自己是否总在重复同一种错误或决策。

- "Do I always make this mistake?"
- "How often have I run into X?"
- "Is this a recurring thing for me?"
- "我每次都踩这个坑吗?"
- "我老犯这个错?"
- "这类问题之前出现过几次?"

调用：`sunshine mem search "<主题>" --global --limit 50`，浏览列表中的日期 / 项目。也可以 `extract` 其中两三个做对比。

## finish-work 回顾（按需）

用户明确想回顾本任务——不是强制步骤，只在他们要求时。

- "Summarize what we did in this task."
- "What were the key decisions / surprises?"
- "Write up the lessons from this round."
- "总结一下这次的经验"
- "记一下这次踩的坑"
- "复盘下这个任务"

调用：确定当前任务的 session id（从 `.sunshine/.runtime/sessions/*.json` 或 `mem list --task <task-dir>`），然后 `extract <id> --phase brainstorm` 和 `--phase implement`。呈现摘要——尽可能给出具体的文件:行引用。是否同时把摘要写到某处（PRD、规范、笔记文件）由用户决定；提议，不要自动写。

## 反模式：这些情况不要调用 `mem`

- "What does this function do?" → 读文件。
- "Why is this test failing?" → 读测试输出和文件。
- "What's the right pattern for X in our codebase?" → grep / 读规范文件。
- "What's the latest npm version of Y?" → 调用 `npm view`。
- "Fix this bug." → 调试。只有怀疑存在先前上下文时才调用 `mem`；否则它是噪声。

标准不变：一位资深队友在回答之前会不会问“didn't we already talk about this?”。如果会，调用 `mem`。如果不会，就不要。
