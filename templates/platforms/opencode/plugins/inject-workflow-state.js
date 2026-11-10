/* global process */
/**
 * Sunshine 工作流状态注入插件
 *
 * OpenCode 上的每轮 UserPromptSubmit 等价物。
 *
 * 每次向模型发请求时，通过 `experimental.chat.messages.transform` 把一条
 * 简短的 <workflow-state> 面包屑注入最新用户消息的内存副本。存储的历史
 * 记录与 TUI 不受影响（issue #553）。面包屑文本只从项目 workflow.md 的
 * [workflow-state:STATUS] 标签块取——workflow.md 是唯一事实来源。本插件
 * 没有兜底表：workflow.md 缺失或标签缺失时，面包屑退化为通用的
 * "Refer to workflow.md for current step." 行（与 Go 版 `sunshine hook
 * workflow-state` 的兜底行逐字一致），让用户看到并修复损坏的状态，
 * 而不是被插件静默掩盖。
 *
 * 以下情况静默跳过：
 *   - 没有 .sunshine/ 目录
 *   - 会话运行时上下文中没有当前任务
 *   - task.json 损坏或缺少 status
 */

import { existsSync, readFileSync } from "fs"
import { join } from "path"
import {
  MESSAGES_TRANSFORM_HOOK,
  latestUserPromptText,
  platformInputFromMessages,
  prependEphemeralText,
} from "../lib/context-visibility.js"
import { SunshineContext, debugLog, isSunshineSubagent } from "../lib/sunshine-context.js"

// STATUS 取值支持字母、数字、下划线、连字符
//（因此 "in-review" / "blocked-by-team" 与 "in_progress" 都能用）。
const TAG_RE = /\[workflow-state:([A-Za-z0-9_-]+)\]\s*\n([\s\S]*?)\n\s*\[\/workflow-state:\1\]/g

// 每轮面包屑的逃生口（issue #427）。与 Go 版 `sunshine hook workflow-state`
// 的跳过关键词解析（PromptHasSkipKeyword）保持一致。
const DEFAULT_PROMPT_INJECTION_SKIP_KEYWORD = "no-sunshine"

function stripInlineComment(value) {
  let inQuote = null
  for (let idx = 0; idx < value.length; idx++) {
    const ch = value[idx]
    if (inQuote) {
      if (ch === inQuote) inQuote = null
      continue
    }
    if (ch === '"' || ch === "'") {
      inQuote = ch
      continue
    }
    if (ch === "#" && (idx === 0 || /\s/.test(value[idx - 1]))) return value.slice(0, idx)
  }
  return value
}

function unquoteYaml(s) {
  if (s.length >= 2 && s[0] === s[s.length - 1] && (s[0] === '"' || s[0] === "'")) return s.slice(1, -1)
  return s
}

/**
 * 只解析 `.sunshine/config.yaml` 的 `prompt_injection:` 段的行级解析器。
 * 不是通用 YAML 解析器——只对该段保持与 Go 版 hook 一致的语义
 *（缺 key 保留默认值；非字符串值保留默认值）。
 */
function readSkipKeyword(directory) {
  const path = join(directory, ".sunshine", "config.yaml")
  if (!existsSync(path)) return DEFAULT_PROMPT_INJECTION_SKIP_KEYWORD
  let text
  try {
    text = readFileSync(path, "utf-8")
  } catch {
    return DEFAULT_PROMPT_INJECTION_SKIP_KEYWORD
  }

  let inSection = false
  let sectionIndent = -1
  for (const rawLine of text.split(/\r?\n/)) {
    const trimmed = rawLine.trim()
    if (!inSection) {
      if (/^prompt_injection\s*:\s*(#.*)?$/.test(trimmed)) {
        inSection = true
        sectionIndent = rawLine.length - rawLine.trimStart().length
      }
      continue
    }
    if (!trimmed || trimmed.startsWith("#")) continue
    const indent = rawLine.length - rawLine.trimStart().length
    if (indent <= sectionIndent) break
    const m = trimmed.match(/^skip_keyword\s*:\s*(.*)$/)
    if (!m) continue
    return unquoteYaml(stripInlineComment(m[1]).trim())
  }
  return DEFAULT_PROMPT_INJECTION_SKIP_KEYWORD
}

/**
 * 在 `text` 中按词边界匹配 `keyword`（大小写不敏感）。连字符算词内字符，
 * 因此 "no-sunshinex" / "xno-sunshine" 不匹配，标点/空白边界才匹配。
 * 空关键词永不匹配。
 */
function promptHasSkipKeyword(text, keyword) {
  if (!keyword || typeof text !== "string") return false
  const escaped = keyword.replace(/[.*+?^${}()|[\]\\]/g, "\\$&")
  const pattern = new RegExp(`(?<![\\w-])${escaped}(?![\\w-])`, "i")
  return pattern.test(text)
}

/**
 * 解析 workflow.md 中的 [workflow-state:STATUS] 块。
 *
 * 返回 {status: body}。workflow.md 是唯一事实来源——这里没有兜底表。
 * 缺失的标签（或缺失/读不到的 workflow.md）在 buildBreadcrumb 里退化为
 * 通用行，让用户看到损坏的状态并修复 workflow.md，而不是被插件静默掩盖。
 */
function loadBreadcrumbs(directory) {
  const workflowPath = join(directory, ".sunshine", "workflow.md")
  if (!existsSync(workflowPath)) return {}
  let content
  try {
    content = readFileSync(workflowPath, "utf-8")
  } catch {
    return {}
  }
  const result = {}
  for (const match of content.matchAll(TAG_RE)) {
    const status = match[1]
    const body = match[2].trim()
    if (body) result[status] = body
  }
  return result
}

/**
 * 从当前任务取 (taskId, status)，无当前任务时返回 null。
 */
function getActiveTask(ctx, platformInput = null) {
  const active = ctx.getActiveTask(platformInput)
  const taskRef = active.taskPath
  if (!taskRef) return null
  const taskDir = ctx.resolveTaskDir(taskRef)
  if (active.stale || !taskDir || !existsSync(taskDir)) {
    return { id: taskRef.split("/").pop(), status: "stale", source: active.source }
  }
  const taskJsonPath = join(taskDir, "task.json")
  if (!existsSync(taskJsonPath)) return null
  try {
    const data = JSON.parse(readFileSync(taskJsonPath, "utf-8"))
    const status = typeof data.status === "string" ? data.status : ""
    if (!status) return null
    const id = data.id || taskRef.split("/").pop()
    return { id, status, source: active.source }
  } catch {
    return null
  }
}

/**
 * 组装 <workflow-state>...</workflow-state> 块。
 * - 已知状态（workflow.md 里有标签）→ 详细正文
 * - 未知状态（无标签，或 workflow.md 缺失）→ 通用
 *   "Refer to workflow.md for current step." 行
 * - no_task 伪状态（id === null）→ 标题省略任务信息
 */
function buildBreadcrumb(id, status, templates) {
  let body = templates[status]
  if (body === undefined) {
    body = "Refer to workflow.md for current step."
  }
  let header = id === null ? `Status: ${status}` : `Task: ${id} (${status})`
  return `<workflow-state>\n${header}\n${body}\n</workflow-state>`
}

// OpenCode 1.2.x 要求插件是工厂函数（见 inject-subagent-context.js 的注释）。
export default async ({ directory }) => {
  const ctx = new SunshineContext(directory)
  debugLog("workflow-state", "插件已加载，目录：", directory)

  return {
      [MESSAGES_TRANSFORM_HOOK]: async (_input, output) => {
        try {
          const messages = output?.messages
          const platformInput = platformInputFromMessages(messages)
          // 跳过 Sunshine 子代理轮次——每轮面包屑只面向主会话；子代理上下文
          // 来自父会话的 tool.execute.before 注入。
          if (isSunshineSubagent(platformInput)) {
            debugLog("workflow-state", "跳过 sunshine 子代理轮次：", platformInput?.agent)
            return
          }
          if (
            process.env.SUNSHINE_HOOKS === "0" ||
            process.env.TRELLIS_HOOKS === "0" ||
            process.env.SUNSHINE_DISABLE_HOOKS === "1" ||
            process.env.TRELLIS_DISABLE_HOOKS === "1"
          ) {
            return
          }
          if (process.env.OPENCODE_NON_INTERACTIVE === "1") {
            return
          }
          if (!ctx.isSunshineProject()) {
            return
          }

          const originalText = latestUserPromptText(messages)

          // 逃生口（issue #427）：用户提示词里以独立单词出现跳过关键词——
          // 仅本轮不注入。
          if (promptHasSkipKeyword(originalText, readSkipKeyword(directory))) {
            debugLog("workflow-state", "跳过本轮：提示词里有跳过关键词")
            return
          }

          const templates = loadBreadcrumbs(directory)
          const task = getActiveTask(ctx, platformInput)
          const breadcrumb = task
            ? buildBreadcrumb(task.id, task.status, templates, task.source)
            : buildBreadcrumb(null, "no_task", templates)

          prependEphemeralText(messages, breadcrumb)
          debugLog(
            "workflow-state",
            "已为任务注入面包屑",
            task ? task.id : "none",
            "状态",
            task ? task.status : "no_task",
          )
        } catch (error) {
          debugLog(
            "workflow-state",
            "messages.transform 出错：",
            error instanceof Error ? error.message : String(error),
          )
        }
      },
  }
}
