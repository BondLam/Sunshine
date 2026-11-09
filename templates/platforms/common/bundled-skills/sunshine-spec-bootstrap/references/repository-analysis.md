# 仓库分析

目标是在写规则之前发现项目的真实架构。不要从通用规范模板出发去填空。从代码出发，再让规范结构跟随代码。

## 分析顺序

1. 阅读现有的 `.sunshine/spec/` 树，记下哪些文件是模板、已过期或已经项目专属。
2. 检查包清单、构建脚本、工作区配置和顶层文档，识别包和运行时层。
3. 用 GitNexus 分析执行流、模块簇、依赖中心和影响敏感区域。
4. 用 ABCoder 或语言原生工具获取精确的签名、类型、类边界和实现示例。
5. 在把任何发现变成规范规则之前，直接阅读有代表性的源码和测试文件。

## 要捕获的内容

| 领域 | 问题 |
|------|-----------|
| 包边界 | 每个包拥有什么？哪些 import 跨越了边界？ |
| 运行时层 | 哪些代码是 CLI、后端、前端、worker、共享库、仅测试或工具？ |
| 核心抽象 | 哪些类型、服务、store、命令、路由或 adapter 定义了系统形态？ |
| 数据流 | 用户输入从哪里进入、如何校验、状态持久化在哪里？ |
| 错误处理 | 失败如何表示、记录、呈现和测试？ |
| 配置 | 默认值、环境配置、生成文件和模板存放在哪里？ |
| 测试 | 哪些测试风格是新工作的可信示例？ |

## GitNexus 用法

先宽泛、再检查具体符号：

```text
gitnexus_query({query: "CLI command execution flow"})
gitnexus_query({query: "template generation and migration"})
gitnexus_context({name: "SymbolName"})
gitnexus_cypher({query: "MATCH (n)-[r]->(m) RETURN n.name, type(r), m.name LIMIT 30"})
```

用 GitNexus 的结果找到重要文件和流程。在检查过相关源码文件之前，不要把图输出当作最终权威来引用。

## ABCoder 用法

当规范需要精确的代码形态时使用 ABCoder：

```text
list_repos()
get_repo_structure({repo_name: "package-name"})
get_file_structure({repo_name: "package-name", file_path: "src/example.ts"})
get_ast_node({repo_name: "package-name", node_ids: [{mod_path: "...", pkg_path: "...", name: "SymbolName"}]})
```

ABCoder 在记录构造函数模式、函数签名、类型契约和引用链时最有价值。

## 分析笔记

分析时做简短笔记。笔记应当包括：

- 包或层的名字。
- 定义本地模式的文件。
- 规范应当传授的规则。
- 在旧代码、注释、测试或迁移路径中发现的反模式。
- 应当创建、删除、重命名或合并的规范文件。
