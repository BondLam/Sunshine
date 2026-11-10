---
description: "Sunshine Copilot 提示词：创建新的 Copilot 提示词"
---

# 创建新的 Copilot 提示词

在 `.github/prompts/` 里新建一个 Copilot 斜杠命令提示词文件。

## 用法

```
/create-command <command-name> <description>
```

示例：

```
/create-command review-pr Check PR code changes against project guidelines
```

## 执行步骤

### 1. 解析输入

从用户输入提取：
- 命令名：必须是 kebab-case（例如 `review-pr`）
- 描述：一句话说明用途

### 2. 分析需求

给命令意图分类：
- 初始化
- 开发前检查
- 质量检查
- 会话记录
- 生成 / 自动化

### 3. 生成提示词内容

创建简洁、可执行的 Markdown 内容。

简单提示词形状：

```markdown
一条清晰的指令，附期望输出。
```

复杂提示词形状：

```markdown
# 提示词标题

简短用途。

## 步骤

### 1. 第一步
具体行动。

### 2. 第二步
具体行动。

## 输出

期望输出格式。
```

### 4. 创建提示词文件

写文件：

- `.github/prompts/<command-name>.prompt.md`

如果文件已存在，先比较内容，只有用户要求覆盖时才更新。

### 5. 确认结果

输出：

```
[OK] 已创建 Copilot 提示词：/<command-name>

文件路径：
- .github/prompts/<command-name>.prompt.md

用法：
/<command-name>

描述：
<description>
```

## 内容质量指南

好的提示词特征：
1. 清晰简洁
2. 无需额外解读即可执行
3. 范围恰当
4. 需要时定义期望输出

避免：
1. 意图含糊（例如 “optimize code”）
2. 指令过长、目标混杂
3. 无理由地重复已有提示词的行为

## 命名约定

| 提示词类型 | 前缀 | 示例 |
|------------|--------|---------|
| 会话开始 | `start` | `start` |
| 开发前 | `before-` | `before-dev` |
| 检查 | `check-` | `check` |
| 记录 | `record-` | `record-session` |
| 生成 | `generate-` | `generate-api-doc` |
| 更新 | `update-` | `update-changelog` |
| 其他 | 动词开头 | `review-code`、`sync-data` |
