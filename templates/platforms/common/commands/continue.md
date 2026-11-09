# 继续当前任务

继续当前任务——在 `.sunshine/workflow.md` 的正确阶段/步骤处续接。

---

## 步骤 1：加载当前上下文

```bash
sunshine task context
```

确认：当前任务、Git 状态、最近提交。

## 步骤 2：加载阶段索引

```bash
sunshine task context --mode phase
```

展示阶段索引（规划 / 执行 / 收尾）及路由与 skill 映射。

## 步骤 3：判断当前位置

`sunshine task context` 会显示当前任务的 `status` 字段。按 `status` + 产物情况分流。本命令代替用户记忆 Sunshine 流程；它本身不批准进入实现。

- `status=planning`，且没有 `prd.md` → **1.1**（加载 `sunshine-brainstorm`）
- `status=planning`，且只有 `prd.md` → 判断任务是轻量还是复杂。轻量可进入 **1.4** 评审；复杂回到 **1.1** 补充 `design.md` + `implement.md`。
- `status=planning`，复杂产物齐全，但子代理 jsonl 未整理（为空，或只有旧的 `_example` 占位行）→ **1.3**
- `status=planning`，所需产物齐全，且所需 jsonl 已整理或为内联模式 → **1.4**（请用户做开始评审；只有用户确认后才运行 `sunshine task start`）
- `status=in_progress`，实现尚未开始 → **2.1**
- `status=in_progress`，实现已完成、尚未检查 → **2.2**
- `status=in_progress`，检查已通过 → **3.3**（更新规范）→ **3.4**（提交）
- `status=completed`（少见；通常会立即归档）→ 走归档流程

阶段规则（完整细节见 `.sunshine/workflow.md`）：

1. 同一阶段内按顺序执行步骤——`[required]` 步骤不得跳过
2. `[once]` 步骤在所需产出已存在时视为已完成。只有 `prd.md` 时，仅轻量任务足够；复杂任务还需要 `design.md` 和 `implement.md`。
3. 发现问题需要时，可以回到前面的阶段

## 步骤 4：加载具体步骤

确定续接的步骤后：

```bash
sunshine task context --mode phase --step <X.X> --platform {{CLI_FLAG}}
```

按加载的说明执行。每完成一个 `[required]` 步骤，进入下一个。

---

## 参考

完整工作流与各阶段的详细步骤见 `.sunshine/workflow.md`。本命令只是入口，权威指南在那里。
