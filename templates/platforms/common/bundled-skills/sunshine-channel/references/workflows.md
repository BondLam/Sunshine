# 工作流

按意图使用这些模式。多轮工作优先使用持久 channel，一次性问题使用 `channel run`。

## 模式 A：多轮头脑风暴

当用户说“和 codex/claude 讨论一下”、“brainstorm”或“拉一个 agent 进来一起看”时使用。

```bash
sunshine channel create brainstorm-storage-layer --by main \
  --task .sunshine/tasks/05-XX-storage-adapter

sunshine channel spawn brainstorm-storage-layer \
  --agent architect --provider codex \
  --file .sunshine/tasks/05-XX-storage-adapter/prd.md \
  --file .sunshine/tasks/05-XX-storage-adapter/design.md \
  --as cx-arch --timeout 30m

sunshine channel send brainstorm-storage-layer \
  --as main --to cx-arch --text-file /tmp/brainstorm-r1.md

sunshine channel wait brainstorm-storage-layer \
  --as main --kind done --from cx-arch --timeout 10m
```

不要在一轮回答后就停下。读出答案，找出含糊的地方，发送新的探针，重复直到结果可执行。

最低轮次结构：

1. 方向拆分：应当放在已有机制里，还是新机制里？
2. MVP 边界：v1、v2，以及什么会迫使 v2 退回 v1。
3. 数据契约：事件、schema、元数据、状态的唯一事实来源、兼容性。
4. CLI / UX 契约：命令名、flag、错误、默认值、歧义。
5. 跨层风险与测试：共享辅助函数、漂移点、发布阻塞测试。

可选轮次：

- 运维：日志、调试、卡住的 worker、kill/重启、恢复。
- 迁移/发布：破坏性状态、manifest、changelog、文档站。
- 反方评审：让对等 Agent 为当前计划辩护反方。

每个探针都应当要求具体的文件路径、命令、schema、被否决的替代方案和发布阻塞问题。当需要做决策时，拒绝含糊其辞。

## 模式 B：Implement / Check Agent

当用户要求分发实现或评审工作时使用。

```bash
TASK=.sunshine/tasks/05-12-foo
sunshine channel create cr-foo --task "$TASK" --by main

sunshine channel spawn cr-foo \
  --agent check \
  --jsonl "$TASK/check.jsonl" \
  --file "$TASK/prd.md" \
  --file "$TASK/design.md" \
  --file "$TASK/implement.md" \
  --cwd "$PWD" --timeout 15m

sunshine channel send cr-foo --as main --to check --text-file /tmp/cr-brief.md
sunshine channel wait cr-foo --as main --kind done --from check --timeout 15m
sunshine channel messages cr-foo --kind message --from check --tag final_answer
```

实现工作使用 `--agent implement` 并发送实现简报。评审工作要包含确切的 diff 范围、相关规范和已运行过的验证。

## 模式 C：并行评审员

使用一个 channel 和不同的 worker 名字。

```bash
sunshine channel create cr-feature --by main --ephemeral

sunshine channel spawn cr-feature --agent check \
  --jsonl "$TASK/check.jsonl" --file "$TASK/prd.md" --file "$TASK/design.md" \
  --timeout 15m

sunshine channel spawn cr-feature --agent check --provider codex --as check-cx \
  --jsonl "$TASK/check.jsonl" --file "$TASK/prd.md" --file "$TASK/design.md" \
  --timeout 15m

sunshine channel send cr-feature --as main --to check --text-file /tmp/cr-brief.md
sunshine channel send cr-feature --as main --to check-cx --text-file /tmp/cr-brief.md
sunshine channel wait cr-feature --as main --kind done --from check,check-cx --all --timeout 15m
```

`--all` 表示每个列出的 worker 都必须发出匹配事件。

## 模式 D：一次性 Worker

```bash
sunshine channel run --provider codex --message "say hi in 3 words" --timeout 1m
sunshine channel run --agent plan --message-file /tmp/plan-question.md --timeout 10m
```

成功时，`run` 会删除临时 channel。出错/超时/被 kill 时，它会保留 channel 并打印检查路径。

## 模式 E：Forum 频道

用于 issue forum、主题式反馈、发布待办、agent 发现和内部 changelog。完整模型见 `forum.md`。

## 模式 F：接手已有 Thread

如果用户给出了 forum/thread 名，自己恢复上下文：

```bash
sunshine channel forum <board> --scope global
sunshine channel thread <board> <thread> --scope global --raw
sunshine channel context list <board> --scope global --thread <thread>
sunshine channel messages <board> --scope global --raw --thread <thread>
```

输出约束摘要，而不是转录转储：

- 用户层面的问题
- 影响本仓库的 context 文件
- 当前版本与未来版本的需求
- 当前代码/设计是否满足它
- 下一步行动或要追加的评论
