/* global process */
/**
 * Sunshine 上下文注入插件
 *
 * Task 工具以支持的子代理类型调用时注入上下文。
 * 使用 OpenCode 的 tool.execute.before hook。
 */

import { existsSync, readdirSync } from "fs"
import { join } from "path"
import {
  SunshineContext,
  debugLog,
  readContextInjectionLimits,
  ContextBudget,
  materializeArtifact,
} from "../lib/sunshine-context.js"

// 支持的子代理类型
const AGENTS_ALL = ["implement", "check", "research"]
const AGENTS_REQUIRE_TASK = ["implement", "check"]

// 匹配派发提示第一个非空行上的 `Active task: <path>`。与 workflow.md 的
// [workflow-state:in_progress] 面包屑约定一致，便于多窗口用户区分目标任务。
const ACTIVE_TASK_HINT_RE = /^\s*Active task:\s*(\S+)\s*$/m

function extractActiveTaskHint(prompt) {
  if (typeof prompt !== "string" || !prompt) return null
  const match = prompt.match(ACTIVE_TASK_HINT_RE)
  return match ? match[1].trim() : null
}

/**
 * 取 implement agent 的上下文。`taskDir` 可以是相对路径
 *（`.sunshine/tasks/foo`）或绝对路径，都经 `ctx.resolveTaskDir` 解析。
 *
 * 读取顺序（与 Go 版 `sunshine hook subagent-context` 的 implement 上下文一致）：
 *   1. implement.jsonl 里的全部文件（规范/调研清单）
 *   2. prd.md（需求）
 *   3. design.md（如果有，技术设计）
 *   4. implement.md（如果有，执行计划）
 * 所有块共用一个总预算（issue #441）。
 */
function getImplementContext(ctx, taskDir) {
  const parts = []
  const taskDirFull = ctx.resolveTaskDir(taskDir)
  if (!taskDirFull) return ""

  const limits = readContextInjectionLimits(ctx.directory)
  const budget = new ContextBudget(limits.max_total_bytes)

  // 1. 读 implement.jsonl
  const jsonlPath = join(taskDirFull, "implement.jsonl")
  const blocks = ctx.readJsonlWithFiles(jsonlPath, limits, budget)
  if (blocks.length > 0) {
    parts.push(ctx.buildContextFromEntries(blocks))
  }

  // 2. 需求文档
  const prdBlock = materializeArtifact(
    ctx.directory,
    `${taskDir}/prd.md`,
    `${taskDir}/prd.md (Requirements)`,
    "Requirements document",
    limits,
    budget,
  )
  if (prdBlock) parts.push(prdBlock)

  // 3. 复杂任务的技术设计
  const designBlock = materializeArtifact(
    ctx.directory,
    `${taskDir}/design.md`,
    `${taskDir}/design.md (Technical Design)`,
    "Technical design document",
    limits,
    budget,
  )
  if (designBlock) parts.push(designBlock)

  // 4. 复杂任务的执行计划
  const implementPlanBlock = materializeArtifact(
    ctx.directory,
    `${taskDir}/implement.md`,
    `${taskDir}/implement.md (Execution Plan)`,
    "Execution plan document",
    limits,
    budget,
  )
  if (implementPlanBlock) parts.push(implementPlanBlock)

  return parts.join("\n\n")
}

/**
 * 取 check agent 的上下文。`taskDir` 可以是相对路径或绝对路径。
 * 读取顺序与共用预算与 implement 上下文相同。
 */
function getCheckContext(ctx, taskDir) {
  const parts = []
  const taskDirFull = ctx.resolveTaskDir(taskDir)
  if (!taskDirFull) return ""

  const limits = readContextInjectionLimits(ctx.directory)
  const budget = new ContextBudget(limits.max_total_bytes)

  const jsonlPath = join(taskDirFull, "check.jsonl")
  const blocks = ctx.readJsonlWithFiles(jsonlPath, limits, budget)
  if (blocks.length > 0) {
    parts.push(ctx.buildContextFromEntries(blocks))
  }

  const prdBlock = materializeArtifact(
    ctx.directory,
    `${taskDir}/prd.md`,
    `${taskDir}/prd.md (Requirements)`,
    "Requirements document",
    limits,
    budget,
  )
  if (prdBlock) parts.push(prdBlock)

  const designBlock = materializeArtifact(
    ctx.directory,
    `${taskDir}/design.md`,
    `${taskDir}/design.md (Technical Design)`,
    "Technical design document",
    limits,
    budget,
  )
  if (designBlock) parts.push(designBlock)

  const implementPlanBlock = materializeArtifact(
    ctx.directory,
    `${taskDir}/implement.md`,
    `${taskDir}/implement.md (Execution Plan)`,
    "Execution plan document",
    limits,
    budget,
  )
  if (implementPlanBlock) parts.push(implementPlanBlock)

  return parts.join("\n\n")
}

/**
 * 取收尾阶段的上下文（PR 前的最终检查）
 */
function getFinishContext(ctx, taskDir) {
  // 收尾复用 check 上下文（同一 JSONL 来源）
  return getCheckContext(ctx, taskDir)
}


/**
 * 取 research agent 的上下文
 */
function getResearchContext(ctx) {
  const parts = []

  // 动态项目结构（扫描实际的 spec 目录）
  const specPath = ".sunshine/spec"
  const specFull = join(ctx.directory, specPath)

  const structureLines = [`## 项目规范目录结构\n\n\`\`\`\n${specPath}/`]
  if (existsSync(specFull)) {
    try {
      const entries = readdirSync(specFull, { withFileTypes: true })
        .filter(d => d.isDirectory() && !d.name.startsWith("."))
        .sort((a, b) => a.name.localeCompare(b.name))

      for (const entry of entries) {
        const entryPath = join(specFull, entry.name)
        if (existsSync(join(entryPath, "index.md"))) {
          structureLines.push(`├── ${entry.name}/`)
        } else {
          try {
            const nested = readdirSync(entryPath, { withFileTypes: true })
              .filter(d => d.isDirectory() && existsSync(join(entryPath, d.name, "index.md")))
              .sort((a, b) => a.name.localeCompare(b.name))
            if (nested.length > 0) {
              structureLines.push(`├── ${entry.name}/`)
              for (const n of nested) {
                structureLines.push(`│   ├── ${n.name}/`)
              }
            }
          } catch {
            // 忽略嵌套读取错误
          }
        }
      }
    } catch {
      // 忽略读取错误
    }
  }
  structureLines.push("```")

  parts.push(structureLines.join("\n") + `

## 检索提示

- 规范文件：\`.sunshine/spec/**/*.md\`
- 已知问题：\`.sunshine/big-question/\`
- 代码检索：使用 Glob 与 Grep 工具
- 技术方案：使用 mcp__exa__web_search_exa 或 mcp__exa__get_code_context_exa`)

  return parts.join("\n\n")
}

/**
 * 用上下文构建增强提示词
 */
function buildPrompt(agentType, originalPrompt, context, isFinish = false) {
  const templates = {
    implement: `<!-- sunshine-hook-injected -->
# Implement Agent 任务

你是多代理流水线中的 Implement Agent。

## 你的上下文

${context}

---

## 你的任务

${originalPrompt}

---

## 工作流

1. **理解规范**——开发规范已全部注入上方
2. **理解任务产物**——阅读需求、技术设计（如果有）与执行计划（如果有）
3. **实现功能**——按规范与任务产物实现
4. **自查**——保证代码质量

## 重要约束

- 不要执行 git commit
- 遵循上方注入的全部开发规范
- 完成后汇报修改/新建的文件清单`,

    check: isFinish ? `<!-- sunshine-hook-injected -->
# Finish Agent 任务

你正在做创建 PR 之前的最终检查。

## 你的上下文

${context}

---

## 你的任务

${originalPrompt}

---

## 工作流

1. **复查改动**——运行 \`git diff --name-only\` 看全部改动文件
2. **核对任务产物**——检查 prd.md，以及存在时的 design.md / implement.md
3. **规范同步**——分析改动是否引入了新的模式、契约或约定
   - 发现新模式/约定时：读目标规范文件 → 更新它 → 需要时更新 index.md
   - 基础设施/跨层改动时：按 update-spec.md 的 7 节强制模板处理
   - 纯代码修复、无新模式时：跳过这一步
4. **运行最终检查**——执行 lint 与 typecheck
5. **确认就绪**——确认代码已可提 PR

## 重要约束

- 发现规范缺口时可以更新规范文件（以 update-spec.md 为指南）
- 编辑前必须先读目标规范文件（避免与已有内容重复）
- 琐碎改动（错别字、格式、明显修复）不要更新规范
- 发现关键代码问题时要清楚汇报（修规范，不修代码）
- 核对 prd.md 中的全部验收标准都已满足
- 存在 design.md 与 implement.md 时核对其中的约束` :
      `<!-- sunshine-hook-injected -->
# Check Agent 任务

你是多代理流水线中的 Check Agent。

## 你的上下文

${context}

---

## 你的任务

${originalPrompt}

---

## 工作流

1. **取改动**——运行 \`git diff --name-only\` 与 \`git diff\`
2. **对照规范检查**——逐项检查
3. **自行修复**——直接修复问题，不只汇报
4. **运行验证**——运行 lint 与 typecheck

## 重要约束

- 自己修复问题，不只汇报
- 必须执行完整检查清单`,

    research: `<!-- sunshine-hook-injected -->
# Research Agent 任务

你是多代理流水线中的 Research Agent。

## 核心原则

**你只做一件事：查找并解释信息。**

## 项目信息

${context}

---

## 你的任务

${originalPrompt}

---

## 工作流

1. **理解查询**——确定检索类型与范围
2. **规划检索**——列出检索步骤
3. **执行检索**——并行运行多个检索
4. **整理结果**——输出结构化报告

## 严格边界

**只允许**：描述存在什么、在哪里、如何工作

**禁止**：提改进建议、批评实现、修改文件`
  }

  return templates[agentType] || originalPrompt
}

function shellQuote(value) {
  return `'${String(value).replace(/'/g, "'\\''")}'`
}

function powershellQuote(value) {
  return `'${String(value).replace(/'/g, "''")}'`
}

function envValue(env, key) {
  const value = env?.[key]
  return typeof value === "string" && value.trim() ? value.trim() : null
}

function shellBasename(value) {
  return value.replace(/\\/g, "/").split("/").pop()?.toLowerCase() || ""
}

function isWindowsPosixShell(env = process.env) {
  if (envValue(env, "MSYSTEM")) return true
  if (envValue(env, "MINGW_PREFIX")) return true
  if (envValue(env, "OPENCODE_GIT_BASH_PATH")) return true

  const ostype = envValue(env, "OSTYPE")?.toLowerCase() || ""
  if (/(msys|mingw|cygwin)/.test(ostype)) return true

  const shell = shellBasename(envValue(env, "SHELL") || "")
  return /^(bash|sh|zsh)(\.exe)?$/.test(shell)
}

function buildSunshineContextPrefix(contextKey, hostPlatform = process.platform, env = process.env) {
  if (hostPlatform === "win32" && !isWindowsPosixShell(env)) {
    return `$env:SUNSHINE_CONTEXT_ID = ${powershellQuote(contextKey)}; `
  }

  return `export SUNSHINE_CONTEXT_ID=${shellQuote(contextKey)}; `
}

function getBashCommandKey(args) {
  if (!args || typeof args !== "object") return null
  if (typeof args.command === "string") return "command"
  if (typeof args.cmd === "string") return "cmd"
  return null
}

// 检测时同时接受旧的 TRELLIS_CONTEXT_ID 前缀（只读兼容，新写入一律用 SUNSHINE_）。
function commandStartsWithSunshineContext(command) {
  const firstCommand = command.trimStart().split(/[;&|]/, 1)[0].trimStart()
  return (
    /^(?:SUNSHINE|TRELLIS)_CONTEXT_ID\s*=/.test(firstCommand) ||
    /^export\s+(?:SUNSHINE|TRELLIS)_CONTEXT_ID\s*=/.test(firstCommand) ||
    /^env\s+(?:(?:-\S+|[A-Za-z_][A-Za-z0-9_]*=\S*)\s+)*(?:SUNSHINE|TRELLIS)_CONTEXT_ID\s*=/.test(firstCommand) ||
    /^\$env:(?:SUNSHINE|TRELLIS)_CONTEXT_ID\s*=/i.test(firstCommand)
  )
}

/**
 * OpenCode 完全不向 Bash 暴露会话身份——它不在任何进程里设会话环境变量。
 * 插件 hook 能拿到它，因此在执行前把它注入 Bash 命令；这个前缀是 AI 运行
 * 的 `sunshine task` 能看到 OpenCode 会话的唯一通道。
 */
function injectSunshineContextIntoBash(ctx, input, output, hostPlatform, env) {
  const args = output?.args
  const commandKey = getBashCommandKey(args)
  if (!commandKey) return false

  const command = args[commandKey]
  if (!command.trim()) return false
  if (commandStartsWithSunshineContext(command)) return false

  const contextKey = ctx.getContextKey(input)
  if (!contextKey) return false

  args[commandKey] = `${buildSunshineContextPrefix(contextKey, hostPlatform, env)}${command}`
  return true
}

// OpenCode 插件工厂：`export default async (input) => hooks`。
// OpenCode 1.2.x 会遍历模块的每个导出并当作函数调用
//（packages/opencode/src/plugin/index.ts——`for ([_, fn] of Object.entries(mod)) await fn(input)`）；
// 之前的 `{ id, server }` 对象形态在 1.2.x 下会报
// `TypeError: fn is not a function`。
export default async ({ directory, platform: hostPlatform = process.platform, env = process.env }) => {
  const ctx = new SunshineContext(directory)
  debugLog("inject", "插件已加载，目录：", directory)

  return {
      "tool.execute.before": async (input, output) => {
        try {
          if (
            process.env.SUNSHINE_HOOKS === "0" ||
            process.env.TRELLIS_HOOKS === "0" ||
            process.env.SUNSHINE_DISABLE_HOOKS === "1" ||
            process.env.TRELLIS_DISABLE_HOOKS === "1"
          ) {
            return
          }
          debugLog("inject", "tool.execute.before 已调用，工具：", input?.tool)

          const toolName = input?.tool?.toLowerCase()
          if (toolName === "bash") {
            if (injectSunshineContextIntoBash(ctx, input, output, hostPlatform, env)) {
              debugLog("inject", "已向 Bash 命令注入 SUNSHINE_CONTEXT_ID")
            }
            return
          }

          if (toolName !== "task") {
            return
          }

          const args = output?.args
          if (!args) return

          const rawSubagentType = args.subagent_type
          // 去掉 v0.5.0-beta.5 agent 改名迁移加上的 "sunshine-" 前缀
          const subagentType = (rawSubagentType || "").replace(/^sunshine-/, "")
          const originalPrompt = args.prompt || ""

          debugLog("inject", "Task 工具已调用，subagent_type：", rawSubagentType)

          if (!AGENTS_ALL.includes(subagentType)) {
            debugLog("inject", "跳过——不支持的 subagent_type")
            return
          }

          // 按以下优先级解析当前任务（只有前面的步骤落空时才走后面的）：
          //   1. 按 input.sessionID 精确查找会话运行时上下文
          //   2. 派发提示里的 `Active task: <path>` 提示
          //      （每次派发可显式覆盖——优先于单会话推断，
          //      便于多窗口用户区分目标任务）
          //   3. 单会话兜底——仅当本地恰好存在 1 个会话运行时文件
          let taskDir = null
          let taskSource = null

          const contextKey = ctx.getContextKey(input)
          if (contextKey) {
            const context = ctx.readContext(contextKey)
            const exactRef = ctx.normalizeTaskRef(context?.current_task || "")
            if (exactRef) {
              taskDir = exactRef
              taskSource = `session:${contextKey}`
            }
          }

          if (!taskDir) {
            const hintRef = extractActiveTaskHint(originalPrompt)
            if (hintRef) {
              const hintNormalized = ctx.normalizeTaskRef(hintRef)
              if (hintNormalized) {
                const hintDir = ctx.resolveTaskDir(hintNormalized)
                if (hintDir && existsSync(hintDir)) {
                  taskDir = hintNormalized
                  taskSource = "prompt-hint"
                  debugLog("inject", "已从 Active task: 提示解析任务：", hintNormalized)
                }
              }
            }
          }

          if (!taskDir) {
            const fallback = ctx._resolveSingleSessionFallback()
            if (fallback?.taskPath) {
              const fallbackDir = ctx.resolveTaskDir(fallback.taskPath)
              if (fallbackDir && existsSync(fallbackDir)) {
                taskDir = fallback.taskPath
                taskSource = fallback.source
                debugLog("inject", "已通过单会话兜底解析任务：", taskDir, "来源：", taskSource)
              }
            }
          }

          // 需要任务目录的 agent
          if (AGENTS_REQUIRE_TASK.includes(subagentType)) {
            // subagentType 上方已去掉 "sunshine-" 前缀
            if (!taskDir) {
              debugLog("inject", "跳过——没有当前任务")
              return
            }
            const taskDirFull = ctx.resolveTaskDir(taskDir)
            if (!taskDirFull || !existsSync(taskDirFull)) {
              debugLog("inject", "跳过——任务目录不存在")
              return
            }
          }

          // 检查 [finish] 标记
          const isFinish = originalPrompt.toLowerCase().includes("[finish]")

          // 按 agent 类型取上下文
          let context = ""
          switch (subagentType) {
            case "implement":
              context = getImplementContext(ctx, taskDir)
              break
            case "check":
              context = isFinish
                ? getFinishContext(ctx, taskDir)
                : getCheckContext(ctx, taskDir)
              break
            case "research":
              context = getResearchContext(ctx, taskDir)
              break
          }

          if (!context) {
            debugLog("inject", "没有可注入的上下文")
            return
          }

          const newPrompt = buildPrompt(subagentType, originalPrompt, context, isFinish)

          // 原地修改 args——整体替换对 task 工具无效，
          // 因为运行时持有同一个 args 对象的本地引用。
          args.prompt = newPrompt

          debugLog("inject", "已为以下 agent 注入上下文", subagentType, "提示词长度：", newPrompt.length)

        } catch (error) {
          debugLog("inject", "tool.execute.before 出错：", error.message, error.stack)
        }
      }
    }
}
