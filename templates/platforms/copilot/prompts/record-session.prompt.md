---
description: "Sunshine Copilot 提示词：方法 1：简单参数"
---

[!] **前置条件**：只有在人已经测试并提交代码之后，才能使用本命令。

**不要直接运行 `git commit`**——下面的脚本会自己处理 `.sunshine/` 元数据的提交。你只需要读 git 历史（`git log`、`git status`、`git diff`）并运行 Sunshine 命令。

---

## 记录工作进度

### 步骤 1：获取上下文并检查任务

```bash
sunshine task context --mode record
```

[!] 归档那些工作**实际已完成**的任务——按工作状态判断，不要按 task.json 里的 `status` 字段：
- 代码提交了吗？→ 归档它（不用等 PR）
- 所有验收标准都满足了吗？→ 归档它
- 不要只因为 `status` 还写着 `planning` 或 `in_progress` 就跳过归档

```bash
sunshine task archive <task-name>
```

### 步骤 2：一键添加会话

```bash
# 方法 1：简单参数
sunshine task add-session \
  --title "Session Title" \
  --commit "hash1,hash2" \
  --summary "Brief summary of what was done"

# 方法 2：通过 stdin 传入详细内容
cat << 'EOF' | sunshine task add-session --stdin --title "Title" --commit "hash"
| Feature | Description |
|---------|-------------|
| New API | Added user authentication endpoint |
| Frontend | Updated login form |

**Updated Files**:
- `packages/api/modules/auth/router.ts`
- `apps/web/modules/auth/components/login-form.tsx`
EOF
```

**自动完成**：
- [OK] 把会话追加到 journal-N.md
- [OK] 自动检测行数，超过 2000 行时新建文件
- [OK] 自动检测分支上下文（`--branch` 可覆盖；否则分支取 task.json → 当前 git 分支；缺失的值会优雅地省略）
- [OK] 更新 index.md（总会话数 +1、最近活跃、行数统计、历史）
- [OK] 自动提交 .sunshine/workspace 与 .sunshine/tasks 的改动

---

## 脚本命令参考

| 命令 | 用途 |
|---------|---------|
| `sunshine task context --mode record` | 获取 record-session 的上下文 |
| `sunshine task add-session --title "..." --commit "..."` | **一键添加会话（推荐，分支自动补全）** |
| `sunshine task archive <name>` | 归档已完成的任务（自动提交） |
| `sunshine task list` | 列出进行中的任务 |
