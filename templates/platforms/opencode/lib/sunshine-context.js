/**
 * Sunshine 上下文管理器
 *
 * 为 OpenCode 插件提供文件读取、JSONL 解析与上下文构建能力的工具类。
 */

import { existsSync, readFileSync, appendFileSync, readdirSync, realpathSync, statSync } from "fs"
import { isAbsolute, join, relative } from "path"
import { createHash } from "crypto"
import { Buffer, isUtf8 } from "buffer"
import process from "process"

// 调试日志
const DEBUG_LOG = "/tmp/sunshine-plugin-debug.log"

function debugLog(prefix, ...args) {
  const timestamp = new Date().toISOString()
  const msg = `[${timestamp}] [${prefix}] ${args.map(a => typeof a === "object" ? JSON.stringify(a) : a).join(" ")}\n`
  try {
    appendFileSync(DEBUG_LOG, msg)
  } catch {
    // 忽略
  }
}

function stringValue(value) {
  return typeof value === "string" && value.trim() ? value.trim() : null
}

function sanitizeKey(raw) {
  const safe = raw.trim().replace(/[^A-Za-z0-9._-]+/g, "_").replace(/^[._-]+|[._-]+$/g, "")
  return safe ? safe.slice(0, 160) : ""
}

function hashValue(raw) {
  return createHash("sha256").update(raw).digest("hex").slice(0, 24)
}

function lookupString(data, keys) {
  if (!data || typeof data !== "object") return null
  for (const key of keys) {
    const value = stringValue(data[key])
    if (value) return value
  }
  for (const nestedKey of ["input", "properties", "event", "hook_input", "hookInput"]) {
    const nested = data[nestedKey]
    if (nested && typeof nested === "object") {
      const value = lookupString(nested, keys)
      if (value) return value
    }
  }
  return null
}

function buildContextKey(platformName, kind, value) {
  if (kind === "transcript") {
    return `${platformName}_transcript_${hashValue(value)}`
  }
  const safeValue = sanitizeKey(value)
  return safeValue ? `${platformName}_${safeValue}` : `${platformName}_${hashValue(value)}`
}

// 精确匹配 `sunshine-implement`、`sunshine-check`、`sunshine-research`。
// 供 chat.message 插件在 Sunshine 子代理轮次内跳过注入。
const SUNSHINE_SUBAGENT_RE = /^sunshine-(implement|check|research)$/

/**
 * 当 OpenCode `chat.message` 输入代表一个 Sunshine 子代理轮次时返回 true。
 * Task 工具派生带自定义 agent 的子会话时，OpenCode 会设置 `input.agent`
 *（见 `packages/opencode/src/tool/task.ts`）。
 */
export function isSunshineSubagent(input) {
  if (!input || typeof input !== "object") return false
  const agent = typeof input.agent === "string" ? input.agent.trim() : ""
  return SUNSHINE_SUBAGENT_RE.test(agent)
}

// ============================================================
// 上下文注入上限（issue #441）
 //
// 提示文案与行为与 Go 版 `sunshine hook subagent-context` 及 Pi 平台扩展
// 逐字节一致。改一处文案，另一处也要同步改。
// ============================================================

const DEFAULT_CONTEXT_INJECTION_LIMITS = {
  max_file_bytes: 32768,
  max_artifact_bytes: 65536,
  max_total_bytes: 131072,
}

/**
 * 把 `buf` 截断到至多 `cap` 字节，且不切断 UTF-8 多字节序列。
 * `cap <= 0` 表示不限。
 */
function truncateUtf8(buf, cap) {
  if (cap <= 0 || buf.length <= cap) return buf
  let i = cap
  // 沿后继字节（10xxxxxx）回退，找到起始字节。
  while (i > 0 && (buf[i - 1] & 0xc0) === 0x80) i--
  if (i === 0) return Buffer.alloc(0)
  const lead = buf[i - 1]
  if (lead & 0x80) {
    let seqLen = 1
    if ((lead & 0xe0) === 0xc0) seqLen = 2
    else if ((lead & 0xf0) === 0xe0) seqLen = 3
    else if ((lead & 0xf8) === 0xf0) seqLen = 4
    // 完整序列放不下时，起始字节也一并去掉。
    if (i - 1 + seqLen > cap) i--
  }
  return buf.subarray(0, i)
}

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
    if (ch === "#" && (idx === 0 || /\s/.test(value[idx - 1]))) {
      return value.slice(0, idx)
    }
  }
  return value
}

function unquoteYaml(s) {
  if (s.length >= 2 && s[0] === s[s.length - 1] && (s[0] === '"' || s[0] === "'")) {
    return s.slice(1, -1)
  }
  return s
}

/**
 * 只解析 `.sunshine/config.yaml` 的 `context_injection:` 段的行级解析器。
 * 不是通用 YAML 解析器——只对该段保持与 Go 版 hook 一致的语义
 *（缺 key 保留默认值；无效/负数值对该 key 回退到默认值并记一条 debugLog）。
 */
function readContextInjectionLimits(repoRoot) {
  const limits = { ...DEFAULT_CONTEXT_INJECTION_LIMITS }
  let text = null
  try {
    text = readFileSync(join(repoRoot, ".sunshine", "config.yaml"), "utf-8")
  } catch {
    return limits
  }
  if (!text) return limits

  let inSection = false
  let sectionIndent = -1
  for (const rawLine of text.split(/\r?\n/)) {
    const trimmed = rawLine.trim()
    if (!inSection) {
      if (/^context_injection\s*:\s*(#.*)?$/.test(trimmed)) {
        inSection = true
        sectionIndent = rawLine.length - rawLine.trimStart().length
      }
      continue
    }
    if (!trimmed || trimmed.startsWith("#")) continue
    const indent = rawLine.length - rawLine.trimStart().length
    if (indent <= sectionIndent) break
    const m = trimmed.match(/^([A-Za-z_][A-Za-z0-9_]*)\s*:\s*(.*)$/)
    if (!m) continue
    const key = m[1]
    if (!(key in limits)) continue
    const raw = unquoteYaml(stripInlineComment(m[2]).trim()).trim()
    if (!/^-?\d+$/.test(raw) || parseInt(raw, 10) < 0) {
      // 无效值/负数 → 保留默认值（Go 版 hook 同样记一条 debugLog 后保留）
      debugLog("context", `invalid context_injection.${key} value: ${raw}; using default ${limits[key]}`)
      continue
    }
    limits[key] = parseInt(raw, 10)
  }
  return limits
}

/** 跟踪已注入子代理上下文的总字节数。 */
class ContextBudget {
  constructor(maxTotalBytes) {
    this.maxTotalBytes = maxTotalBytes
    this.used = 0
  }

  hasRoom(size) {
    if (this.maxTotalBytes <= 0) return true
    return this.used + size <= this.maxTotalBytes
  }

  add(size) {
    this.used += size
  }
}

function truncateNotice(path, cap) {
  return `\n[Sunshine: truncated at ${cap} bytes — read ${path} for the full content]`
}

function isBinaryContent(data) {
  return data.includes(0) || !isUtf8(data)
}

function binaryNotice(path, size, reason) {
  return `[Sunshine: not inlined (binary file) — ${path} (${size} bytes): ${reason}]`
}

function indexNotice(path, size, reason) {
  return `[Sunshine: not inlined (total context limit reached) — ${path} (${size} bytes): ${reason}]`
}

/**
 * 返回内联的 `=== header ===` 块；总上下文预算耗尽时退化为一条索引提示。
 */
function budgetedBlock(budget, header, plainPath, content, reason, sizeForIndex) {
  const block = `=== ${header} ===\n${content}`
  const blockBytes = Buffer.byteLength(block, "utf-8")
  if (!budget.hasRoom(blockBytes)) {
    const notice = indexNotice(plainPath, sizeForIndex, reason)
    budget.add(Buffer.byteLength(notice, "utf-8"))
    return notice
  }
  budget.add(blockBytes)
  return block
}

/** 读文件原始字节，文件不存在时返回 null。 */
function readFileBytes(basePath, filePath) {
  const fullPath = isAbsolute(filePath) ? filePath : join(basePath, filePath)
  try {
    if (!statSync(fullPath).isFile()) return null
  } catch {
    return null
  }
  try {
    return readFileSync(fullPath)
  } catch {
    return null
  }
}

/** 读 JSONL 引用的文件，应用单文件上限，再计入预算。 */
function materializeFile(basePath, filePath, reason, limits, budget) {
  const data = readFileBytes(basePath, filePath)
  if (data === null) return null

  const size = data.length
  if (isBinaryContent(data)) {
    const notice = binaryNotice(filePath, size, reason)
    budget.add(Buffer.byteLength(notice, "utf-8"))
    return notice
  }
  const cap = limits.max_file_bytes
  const truncated = truncateUtf8(data, cap)
  let content = truncated.toString("utf-8")
  if (truncated.length < size) content += truncateNotice(filePath, cap)

  return budgetedBlock(budget, filePath, filePath, content, reason, size)
}

/**
 * 读目录下的全部 .md 文件，单文件上限与总上限与单个 JSONL 条目相同。
 */
function materializeDirectory(basePath, dirPath, reason, limits, budget, maxFiles = 20) {
  const blocks = []
  const fullPath = isAbsolute(dirPath) ? dirPath : join(basePath, dirPath)

  let files
  try {
    if (!statSync(fullPath).isDirectory()) return blocks
    files = readdirSync(fullPath)
      .filter(f => f.endsWith(".md") && statSync(join(fullPath, f)).isFile())
      .sort()
  } catch {
    return blocks
  }

  for (const filename of files.slice(0, maxFiles)) {
    const relativePath = join(dirPath, filename)
    const block = materializeFile(basePath, relativePath, reason, limits, budget)
    if (block) blocks.push(block)
  }
  return blocks
}

/**
 * 读任务产物（prd/design/implement.md），应用单产物上限，再计入预算。
 */
function materializeArtifact(basePath, filePath, headerLabel, reason, limits, budget) {
  const data = readFileBytes(basePath, filePath)
  if (data === null) return null

  const size = data.length
  const cap = limits.max_artifact_bytes
  const truncated = truncateUtf8(data, cap)
  let content = truncated.toString("utf-8")
  if (truncated.length < size) content += truncateNotice(filePath, cap)

  return budgetedBlock(budget, headerLabel, filePath, content, reason, size)
}

/**
 * Sunshine 上下文管理器
 */
export class SunshineContext {
  constructor(directory) {
    this.directory = directory
    debugLog("context", "SunshineContext 已初始化", { directory })
  }

  // ============================================================
  // Sunshine 项目检测
  // ============================================================

  isSunshineProject() {
    return existsSync(join(this.directory, ".sunshine"))
  }

  // OpenCode 不向环境变量导出任何会话身份：1.17.18 二进制与 1.18.13 源码里
  // 没有任何 OPENCODE_* 名字携带它。插件 hook 输入是唯一来源，本插件给
  // Bash 命令加的 `export SUNSHINE_CONTEXT_ID=` 前缀是该身份到达子进程的
  // 唯一途径。这里接受 SUNSHINE_CONTEXT_ID（兼容旧的 TRELLIS_CONTEXT_ID，
  // 只读），以便嵌套的 Sunshine 运行继承父级的 key；没有证据表明某个厂商
  // 会设置平台原生环境变量名之前，不要在旁边再加。
  getContextKey(platformInput = null) {
    const override =
      stringValue(process.env.SUNSHINE_CONTEXT_ID) ||
      stringValue(process.env.TRELLIS_CONTEXT_ID) // 兼容旧版，只读
    if (override) {
      return sanitizeKey(override) || hashValue(override)
    }

    const input = platformInput && typeof platformInput === "object" ? platformInput : null
    if (!input) return null

    const sessionID = lookupString(input, ["session_id", "sessionId", "sessionID"])
    if (sessionID) return buildContextKey("opencode", "session", sessionID)

    const conversationID = lookupString(input, ["conversation_id", "conversationId", "conversationID"])
    if (conversationID) return buildContextKey("opencode", "conversation", conversationID)

    const transcriptPath = lookupString(input, ["transcript_path", "transcriptPath", "transcript"])
    if (transcriptPath) return buildContextKey("opencode", "transcript", transcriptPath)

    return null
  }

  readContext(contextKey) {
    try {
      const contextPath = join(this.directory, ".sunshine", ".runtime", "sessions", `${contextKey}.json`)
      if (!existsSync(contextPath)) return null
      return JSON.parse(readFileSync(contextPath, "utf-8"))
    } catch {
      return null
    }
  }

  /**
   * 从会话运行时上下文取当前任务。
   *
   * 解析顺序（与 `sunshine task` 解析当前任务的顺序一致）：
   *   1. 按输入派生的 context key 查找运行时文件。
   *   2. 查不到且本地恰好存在一个会话运行时文件时，用它
   *      （`_resolveSingleSessionFallback`）。0 个或 ≥2 个时拒绝猜测，
   *      以保持多窗口隔离。
   */
  getActiveTask(platformInput = null) {
    const contextKey = this.getContextKey(platformInput)
    if (contextKey) {
      const context = this.readContext(contextKey)
      const taskRef = this.normalizeTaskRef(context?.current_task || "")
      if (taskRef) {
        const taskDir = this.resolveTaskDir(taskRef)
        return {
          taskPath: taskRef,
          source: `session:${contextKey}`,
          stale: !taskDir || !existsSync(taskDir),
        }
      }
    }

    const fallback = this._resolveSingleSessionFallback()
    if (fallback) {
      return fallback
    }

    return { taskPath: null, source: "none", stale: false }
  }

  /**
   * 与 `sunshine task` 的单会话兜底解析一致。本地恰好存在一个会话运行时
   * 文件时，返回它指向的任务，否则返回 null。
   */
  _resolveSingleSessionFallback() {
    const sessionsDir = join(this.directory, ".sunshine", ".runtime", "sessions")
    if (!existsSync(sessionsDir)) return null

    let files
    try {
      files = readdirSync(sessionsDir)
        .filter(name => name.endsWith(".json"))
        .sort()
    } catch {
      return null
    }
    if (files.length !== 1) return null

    const sessionFile = join(sessionsDir, files[0])
    let context
    try {
      context = JSON.parse(readFileSync(sessionFile, "utf-8"))
    } catch {
      return null
    }
    const taskRef = this.normalizeTaskRef(context?.current_task || "")
    if (!taskRef) return null

    const taskDir = this.resolveTaskDir(taskRef)
    const fallbackKey = files[0].replace(/\.json$/, "")
    return {
      taskPath: taskRef,
      source: `session-fallback:${fallbackKey}`,
      stale: !taskDir || !existsSync(taskDir),
    }
  }

  getCurrentTask(platformInput = null) {
    return this.getActiveTask(platformInput).taskPath
  }

  normalizeTaskRef(taskRef) {
    if (!taskRef) {
      return ""
    }

    if (isAbsolute(taskRef)) {
      return taskRef.trim()
    }

    let normalized = taskRef.trim().replace(/\\/g, "/")
    while (normalized.startsWith("./")) {
      normalized = normalized.slice(2)
    }

    if (normalized.startsWith("tasks/")) {
      return `.sunshine/${normalized}`
    }

    return normalized
  }

  /**
   * `candidate` 落在项目内时返回它，否则返回 null。
   *
   * 任务引用不一定是用户亲手输入的。`sunshine task` 现在拒绝存储离开项目
   * 的引用，但该修复之前写入的会话文件里仍可能有，而 `sunshine update`
   * 不重写会话文件——因此一个有毒指针会在堵住写入口的升级之后继续存在。
   * 两边都解析真实路径，链接到项目外的任务目录也会被拒绝；成功时返回
   * 原始的 `candidate`：调用方按 `this.directory` 拼相对路径，若给它
   * realpath，项目本身位于符号链接后时就会出错。
   */
  containInProject(candidate) {
    try {
      const rel = relative(realpathSync(this.directory), realpathSync(candidate))
      if (rel !== "" && (rel.startsWith("..") || isAbsolute(rel))) {
        return null
      }
      return candidate
    } catch {
      return null
    }
  }

  resolveTaskDir(taskRef) {
    const normalized = this.normalizeTaskRef(taskRef)
    if (!normalized) {
      return null
    }

    const candidate = isAbsolute(normalized)
      ? normalized
      : normalized.startsWith(".sunshine/")
        ? join(this.directory, normalized)
        : join(this.directory, ".sunshine", "tasks", normalized)

    return this.containInProject(candidate)
  }

  // ============================================================
  // 文件读取工具
  // ============================================================

  readFile(filePath) {
    try {
      if (existsSync(filePath)) {
        return readFileSync(filePath, "utf-8")
      }
    } catch {
      // 忽略读取错误
    }
    return null
  }

  readProjectFile(relativePath) {
    return this.readFile(join(this.directory, relativePath))
  }


  // ============================================================
  // JSONL Reading
  // ============================================================

  /**
   * 读 JSONL 文件，把引用的文件/目录物化为上下文块，应用单文件上限与
   * 共用总预算（issue #441）。与 Go 版 `sunshine hook subagent-context`
   * 的 JSONL 物化逻辑一致。支持：
   *   {"file": "path/to/file.md", "reason": "..."}
   *   {"file": "path/to/dir/", "type": "directory", "reason": "..."}
   *
   * 引用的文件缺失时静默跳过（Go 版同样跳过）。
   */
  readJsonlWithFiles(jsonlPath, limits, budget) {
    const blocks = []
    const content = this.readFile(jsonlPath)
    if (!content) return blocks

    for (const line of content.split("\n")) {
      if (!line.trim()) continue
      try {
        const item = JSON.parse(line)
        const file = item.file || item.path
        const entryType = item.type || "file"
        const reason = item.reason || "-"

        if (!file) continue

        if (entryType === "directory") {
          blocks.push(...materializeDirectory(this.directory, file, reason, limits, budget))
        } else {
          const block = materializeFile(this.directory, file, reason, limits, budget)
          if (block) blocks.push(block)
        }
      } catch {
        // 忽略单行的解析错误
      }
    }
    return blocks
  }

  buildContextFromEntries(blocks) {
    return blocks.join("\n\n")
  }
}

// ============================================================
// 上下文收集器（会话去重用）
// ============================================================

class ContextCollector {
  constructor() {
    this.processed = new Set()
  }

  markProcessed(sessionID) {
    this.processed.add(sessionID)
  }

  isProcessed(sessionID) {
    return this.processed.has(sessionID)
  }

  clear(sessionID) {
    this.processed.delete(sessionID)
  }
}

// 单例
export const contextCollector = new ContextCollector()

// 向插件导出 debug log
export { debugLog }

// 上下文注入上限（issue #441）——导出给插件与测试用
export {
  DEFAULT_CONTEXT_INJECTION_LIMITS,
  truncateUtf8,
  readContextInjectionLimits,
  ContextBudget,
  materializeFile,
  materializeDirectory,
  materializeArtifact,
}
