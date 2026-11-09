# MCP 配置

引导 Sunshine 规范时推荐使用 GitNexus 和 ABCoder，因为它们向 Agent 暴露架构和 AST 上下文。它们是工具选择，不是平台要求。通过 Agent 宿主提供的任何 MCP 机制来配置它们。

## GitNexus

GitNexus 基于仓库构建代码知识图谱。用于模块边界、执行流、依赖关系、影响范围和图查询。

### 安装与索引

```bash
# 在仓库根目录运行。
npx gitnexus analyze

# 检查索引状态。
npx gitnexus status

# 代码变更后分析过期时，重新索引。
npx gitnexus analyze
```

索引写入 `.gitnexus/`。只有项目已经在使用 embedding 时才保留 embedding；否则普通索引就足以支撑规范引导。

### MCP 服务器命令

在宿主的 MCP 配置中使用这条服务器命令：

```bash
npx -y gitnexus mcp
```

### 常用工具

| 工具 | 用途 |
|------|---------|
| `gitnexus_query` | 按概念查找执行流和功能区域 |
| `gitnexus_context` | 检查某个符号的调用者、被调用者、引用和流程参与情况 |
| `gitnexus_impact` | 在修改某个符号之前理解影响范围 |
| `gitnexus_detect_changes` | 完成前检查变更的符号和受影响的流程 |
| `gitnexus_cypher` | 运行直接图查询 |
| `gitnexus_list_repos` | 列出已索引的仓库 |

## ABCoder

ABCoder 把代码解析为 UniAST，提供精确的包、文件和节点级结构。用于签名、类型形态、实现、依赖和反向引用。

### 安装

```bash
go install github.com/cloudwego/abcoder@latest
abcoder --help
```

### 解析仓库

```bash
abcoder parse /absolute/path/to/package \
  --lang typescript \
  --name package-name \
  --output ~/abcoder-asts
```

对于 monorepo，每个包用稳定的 `--name` 单独解析，以便任务笔记引用相同的仓库名。

### MCP 服务器命令

在宿主的 MCP 配置中使用这条服务器命令：

```bash
abcoder mcp ~/abcoder-asts
```

### 常用工具

| 工具 | 层级 | 用途 |
|------|-------|---------|
| `list_repos` | 1 | 列出已解析的仓库 |
| `get_repo_structure` | 2 | 检查包和文件 |
| `get_package_structure` | 3 | 检查包内的节点 |
| `get_file_structure` | 3 | 检查文件中的函数、类、类型和签名 |
| `get_ast_node` | 4 | 获取代码、依赖、引用和实现 |

## 验证

配置完成后，从 Agent 宿主验证两个 MCP 服务器都可见。开始规范编写之前，先对每个服务器运行一次简单查询。

```bash
ls .gitnexus/meta.json
ls ~/abcoder-asts/*.json
```
