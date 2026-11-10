/* global process */
import { existsSync, readFileSync, readdirSync, statSync } from "fs"
import { join } from "path"
import { execFileSync } from "child_process"
import { debugLog } from "./sunshine-context.js"

const FIRST_REPLY_NOTICE = `<first-reply-notice>
在本会话第一次可见的助手回复中，简要确认 Sunshine SessionStart 上下文已加载。
按以下顺序选择确认所用的语言：
1. 使用用户当前请求的语言（触发本次回复的那条用户消息的语言）。
2. 如果该请求没有明确的自然语言，使用项目已明确约定的沟通语言。
3. 如果两者都没有，严格按以下语言中立兜底输出：\`Sunshine SessionStart ✓\`。
确认后直接继续处理用户的请求。
确认不得改变本次回复其余部分的语言。
本提示只生效一次：在本会话第一次可见的助手回复之后不要重复。
</first-reply-notice>`

function hasCuratedJsonlEntry(jsonlPath) {
  try {
    const content = readFileSync(jsonlPath, "utf-8")
    for (const rawLine of content.split(/\r?\n/)) {
      const line = rawLine.trim()
      if (!line) continue
      try {
        const row = JSON.parse(line)
        if (row && typeof row === "object" && typeof row.file === "string" && row.file) {
          return true
        }
      } catch {
        // 忽略格式错误的行
      }
    }
  } catch {
    return false
  }
  return false
}

function getTaskStatus(ctx, platformInput = null) {
  const active = ctx.getActiveTask(platformInput)
  const taskRef = active.taskPath
  if (!taskRef) {
    return (
      "状态：无当前任务\n" +
      "下一步：创建任何 Sunshine 任务之前，先对本轮做分类。" +
      "简单对话/小任务只需问本轮是否要创建 Sunshine 任务；" +
      "复杂任务要问是否允许创建任务并做规划。"
    )
  }

  const taskDir = ctx.resolveTaskDir(taskRef)

  if (active.stale || !taskDir || !existsSync(taskDir)) {
    return `状态：过期指针\n任务：${taskRef}\n下一步：任务目录不存在。请运行：sunshine task finish`
  }

  let taskData = {}
  const taskJsonPath = join(taskDir, "task.json")
  if (existsSync(taskJsonPath)) {
    try {
      taskData = JSON.parse(readFileSync(taskJsonPath, "utf-8"))
    } catch {
      // 忽略解析错误
    }
  }

  const taskTitle = taskData.title || taskRef
  const taskStatus = taskData.status || "unknown"

  if (taskStatus === "completed") {
    return `状态：已完成\n任务：${taskTitle}\n下一步：运行 /sunshine:finish-work。如果工作区有未提交改动，先回到阶段 3.4。`
  }

  const hasPrd = existsSync(join(taskDir, "prd.md"))
  const hasDesign = existsSync(join(taskDir, "design.md"))
  const hasImplementPlan = existsSync(join(taskDir, "implement.md"))
  const artifactNames = ["prd.md", "design.md", "implement.md", "implement.jsonl", "check.jsonl"]
  const present = artifactNames.filter(name => existsSync(join(taskDir, name)))
  if (existsSync(join(taskDir, "research"))) present.push("research/")
  const presentLine = present.length > 0 ? present.join(", ") : "(none)"
  const implementJsonl = join(taskDir, "implement.jsonl")
  const checkJsonl = join(taskDir, "check.jsonl")
  const jsonlReady =
    (!existsSync(implementJsonl) || hasCuratedJsonlEntry(implementJsonl)) &&
    (!existsSync(checkJsonl) || hasCuratedJsonlEntry(checkJsonl))

  if (taskStatus === "planning" && !hasPrd) {
    return `状态：规划中\n任务：${taskTitle}\n已有：${presentLine}\n下一步：加载 sunshine-brainstorm 并写 prd.md。留在规划阶段。`
  }

  if (taskStatus === "planning") {
    const missingComplex = []
    if (!hasDesign) missingComplex.push("design.md")
    if (!hasImplementPlan) missingComplex.push("implement.md")
    const nextBits = []
    if (missingComplex.length > 0) {
      nextBits.push(
        `轻量任务可以只凭 PRD 申请开始评审；复杂任务开始前必须补上 ${missingComplex.join("、")}`,
      )
    } else {
      nextBits.push("规划产物已齐全；在运行 `sunshine task start` 之前请用户评审")
    }
    if (!jsonlReady) {
      nextBits.push("子代理模式开始前先整理好 `implement.jsonl` 与 `check.jsonl`")
    }
    return `状态：规划中\n任务：${taskTitle}\n已有：${presentLine}\n下一步：${nextBits.join("；")}。用户确认开始之前不要进入实现。`
  }

  return (
    `状态：${String(taskStatus).toUpperCase()}\n任务：${taskTitle}\n` +
    `已有：${presentLine}\n` +
    "下一步：按匹配的每轮 workflow-state 执行。" +
    "实现/检查的上下文顺序是 jsonl 条目 -> `prd.md` -> `design.md（如果有）` -> `implement.md（如果有）`。"
  )
}

function loadSunshineConfig(directory, contextKey = null) {
  // Sunshine 没有 Python 脚本：monorepo 包信息改由 `sunshine task context
  // --mode packages --json` 提供，其 JSON 字段（mode / packages /
  // specScope / activeTaskPackage / defaultPackage）与原 get_context.py
  // 的输出一致。调用方须保证 sunshine 二进制在 PATH 上。
  if (!existsSync(join(directory, ".sunshine"))) {
    return { isMonorepo: false, packages: {}, specScope: null, activeTaskPackage: null, defaultPackage: null }
  }
  try {
    const output = execFileSync("sunshine", ["task", "context", "--mode", "packages", "--json"], {
      cwd: directory,
      timeout: 5000,
      encoding: "utf-8",
      stdio: ["pipe", "pipe", "pipe"],
      env: {
        ...process.env,
        ...(contextKey ? { SUNSHINE_CONTEXT_ID: contextKey } : {}),
      },
    })
    const data = JSON.parse(output)
    if (data.mode !== "monorepo") {
      return { isMonorepo: false, packages: {}, specScope: null, activeTaskPackage: null, defaultPackage: null }
    }
    const pkgDict = {}
    for (const pkg of (data.packages || [])) {
      pkgDict[pkg.name] = pkg
    }
    return {
      isMonorepo: true,
      packages: pkgDict,
      specScope: data.specScope || null,
      activeTaskPackage: data.activeTaskPackage || null,
      defaultPackage: data.defaultPackage || null,
    }
  } catch (e) {
    debugLog("session", "loadSunshineConfig 出错：", e.message)
    return { isMonorepo: false, packages: {}, specScope: null, activeTaskPackage: null, defaultPackage: null }
  }
}

function checkLegacySpec(directory, config) {
  if (!config.isMonorepo || Object.keys(config.packages).length === 0) {
    return null
  }

  const specDir = join(directory, ".sunshine", "spec")
  if (!existsSync(specDir)) return null

  let hasLegacy = false
  for (const name of ["backend", "frontend"]) {
    if (existsSync(join(specDir, name, "index.md"))) {
      hasLegacy = true
      break
    }
  }
  if (!hasLegacy) return null

  const pkgNames = Object.keys(config.packages).sort()
  const missing = pkgNames.filter(name => !existsSync(join(specDir, name)))

  if (missing.length === 0) return null

  if (missing.length === pkgNames.length) {
    return (
      `[!] 检测到旧版规范结构：发现了 \`spec/backend/\` 或 \`spec/frontend/\`，` +
      `但没有按包划分的 \`spec/<package>/\` 目录。\n` +
      `Monorepo 包：${pkgNames.join("、")}\n` +
      `请重组：\`spec/backend/\` -> \`spec/<package>/backend/\``
    )
  }
  return (
    `[!] 检测到规范迁移未完成：以下包仍缺少 \`spec/<pkg>/\` 目录：${missing.join("、")}。\n` +
    `请为全部包完成迁移。`
  )
}

function resolveSpecScope(config) {
  if (!config.isMonorepo || Object.keys(config.packages).length === 0) {
    return null
  }

  const { specScope, activeTaskPackage, defaultPackage, packages } = config
  if (specScope == null) return null

  if (specScope === "active_task") {
    if (activeTaskPackage && activeTaskPackage in packages) return new Set([activeTaskPackage])
    if (defaultPackage && defaultPackage in packages) return new Set([defaultPackage])
    return null
  }

  if (Array.isArray(specScope)) {
    const valid = new Set()
    for (const entry of specScope) {
      if (entry in packages) {
        valid.add(entry)
      }
    }
    if (valid.size > 0) return valid
    if (activeTaskPackage && activeTaskPackage in packages) return new Set([activeTaskPackage])
    if (defaultPackage && defaultPackage in packages) return new Set([defaultPackage])
    return null
  }

  return null
}

function collectSpecIndexPaths(directory, allowedPkgs) {
  const specDir = join(directory, ".sunshine", "spec")
  const paths = []

  const guidesIndex = join(specDir, "guides", "index.md")
  if (existsSync(guidesIndex)) {
    paths.push(".sunshine/spec/guides/index.md")
  }

  if (!existsSync(specDir)) return paths

  try {
    const subs = readdirSync(specDir).filter(name => {
      if (name.startsWith(".") || name === "guides") return false
      try {
        return statSync(join(specDir, name)).isDirectory()
      } catch {
        return false
      }
    }).sort()

    for (const sub of subs) {
      const indexFile = join(specDir, sub, "index.md")
      if (existsSync(indexFile)) {
        paths.push(`.sunshine/spec/${sub}/index.md`)
      } else {
        if (allowedPkgs !== null && !allowedPkgs.has(sub)) continue
        try {
          const nested = readdirSync(join(specDir, sub)).filter(name => {
            try {
              return statSync(join(specDir, sub, name)).isDirectory()
            } catch {
              return false
            }
          }).sort()
          for (const layer of nested) {
            const nestedIndex = join(specDir, sub, layer, "index.md")
            if (existsSync(nestedIndex)) {
              paths.push(`.sunshine/spec/${sub}/${layer}/index.md`)
            }
          }
        } catch {
          // 忽略目录读取错误
        }
      }
    }
  } catch {
    // 忽略规范目录读取错误
  }

  return paths
}

function readDeveloper(directory) {
  try {
    const content = readFileSync(join(directory, ".sunshine", ".developer"), "utf-8")
    for (const line of content.split(/\r?\n/)) {
      if (line.startsWith("name=")) return line.slice("name=".length).trim()
    }
  } catch {
    // 忽略缺失的开发者文件
  }
  return "(not initialized)"
}

function runGit(directory, args) {
  try {
    return execFileSync("git", args, {
      cwd: directory,
      timeout: 3000,
      encoding: "utf-8",
      stdio: ["pipe", "pipe", "pipe"],
    }).trim()
  } catch {
    return ""
  }
}

function buildCompactCurrentState(ctx, platformInput, specIndexPaths) {
  const directory = ctx.directory
  const lines = []
  lines.push(`开发者：${readDeveloper(directory)}`)

  const branch = runGit(directory, ["branch", "--show-current"]) || "(detached)"
  const dirtyCount = runGit(directory, ["status", "--porcelain"])
    .split(/\r?\n/)
    .filter(line => line.trim()).length
  lines.push(`Git：分支 ${branch}；${dirtyCount === 0 ? "干净" : `有 ${dirtyCount} 处改动`}。`)

  const active = ctx.getActiveTask(platformInput)
  if (active.taskPath) {
    const taskDir = ctx.resolveTaskDir(active.taskPath)
    let status = "unknown"
    if (taskDir) {
      try {
        const data = JSON.parse(readFileSync(join(taskDir, "task.json"), "utf-8"))
        status = data.status || "unknown"
      } catch {
        // 忽略解析错误
      }
    }
    lines.push(`当前任务：${active.taskPath}；status=${status}。`)
  } else {
    lines.push("当前任务：无。")
  }

  const tasksDir = join(directory, ".sunshine", "tasks")
  if (existsSync(tasksDir)) {
    try {
      const activeTasks = readdirSync(tasksDir, { withFileTypes: true })
        .filter(entry => entry.isDirectory() && entry.name !== "archive" && existsSync(join(tasksDir, entry.name, "task.json")))
      lines.push(`当前任务共 ${activeTasks.length} 个。仅在需要时运行 \`sunshine task list --mine\`。`)
    } catch {
      // 忽略任务列表错误
    }
  }

  const developer = readDeveloper(directory)
  const workspaceDir = join(directory, ".sunshine", "workspace", developer)
  if (developer !== "(not initialized)" && existsSync(workspaceDir)) {
    try {
      const journals = readdirSync(workspaceDir)
        .filter(name => /^journal-\d+\.md$/.test(name))
        .sort((a, b) => Number(a.match(/\d+/)?.[0] || 0) - Number(b.match(/\d+/)?.[0] || 0))
      const journal = journals[journals.length - 1]
      if (journal) {
        const journalPath = join(workspaceDir, journal)
        const lineCount = readFileSync(journalPath, "utf-8").split(/\r?\n/).length
        lines.push(`日志：.sunshine/workspace/${developer}/${journal}，${lineCount} / 2000 行。`)
      }
    } catch {
      // 忽略日志错误
    }
  }

  if (specIndexPaths.length > 0) {
    lines.push(`规范索引：${specIndexPaths.length} 个可用。`)
  }

  return lines.join("\n")
}

export function buildSessionContext(ctx, platformInput = null) {
  const directory = ctx.directory
  const contextKey = typeof ctx.getContextKey === "function"
    ? ctx.getContextKey(platformInput)
    : null

  const config = loadSunshineConfig(directory, contextKey)
  const allowedPkgs = resolveSpecScope(config)
  const paths = collectSpecIndexPaths(directory, allowedPkgs)

  const parts = []

  parts.push(`<session-context>
Sunshine 精简 SessionStart 上下文。用于定位本会话；细节按需加载。
</session-context>`)
  parts.push(FIRST_REPLY_NOTICE)

  const legacyWarning = checkLegacySpec(directory, config)
  if (legacyWarning) {
    parts.push(`<migration-warning>\n${legacyWarning}\n</migration-warning>`)
  }

  parts.push("<current-state>")
  parts.push(buildCompactCurrentState(ctx, platformInput, paths))
  parts.push("</current-state>")

  const workflowContent = ctx.readProjectFile(".sunshine/workflow.md")
  if (workflowContent) {
    const allLines = workflowContent.split("\n")
    const overviewLines = [
      "# Development Workflow - Session Summary",
      "完整指南：.sunshine/workflow.md。步骤详情：`sunshine task context --mode phase --step <X.Y>`。",
      "",
    ]

    let rangeStart = -1
    let rangeEnd = allLines.length
    for (let i = 0; i < allLines.length; i++) {
      const stripped = allLines[i].trim()
      if (rangeStart === -1 && stripped === "## Phase Index") {
        rangeStart = i
      } else if (rangeStart !== -1 && stripped === "## Phase 1: Plan") {
        rangeEnd = i
        break
      }
    }
    if (rangeStart !== -1) {
      const strippedStateBlocks = allLines
        .slice(rangeStart, rangeEnd)
        .join("\n")
        .replace(/\[workflow-state:([A-Za-z0-9_-]+)\]\s*\n[\s\S]*?\n\s*\[\/workflow-state:\1\]\n?/g, "")
        .replace(/<!--[\s\S]*?-->/g, "")
        .replace(/^\[(?!\/?workflow-state:)\/?[^\]\n]+\]\s*\n?/gm, "")
        .replace(/\n{3,}/g, "\n\n")
      overviewLines.push(strippedStateBlocks.trimEnd())
    }

    parts.push("<sunshine-workflow>")
    parts.push(overviewLines.join("\n").trimEnd())
    parts.push("</sunshine-workflow>")
  }

  parts.push("<guidelines>")
  parts.push(
    "实现/检查的任务上下文顺序：jsonl 条目 -> `prd.md` -> " +
    "`design.md（如果有）` -> `implement.md（如果有）`。轻量任务跳过缺失的可选产物。\n"
  )

  if (paths.length > 0) {
    parts.push("## 可用索引（按需阅读）")
    for (const p of paths) {
      parts.push(`- ${p}`)
    }
    parts.push("")
  }

  parts.push(
    "更多信息：" +
    "`sunshine task context --mode packages`"
  )
  parts.push("</guidelines>")

  const taskStatus = getTaskStatus(ctx, platformInput)
  parts.push(`<task-status>\n${taskStatus}\n</task-status>`)

  parts.push(`<ready>
上下文已加载。按 <task-status> 执行。仅在需要时加载 workflow/规范/任务细节。
</ready>`)

  return parts.join("\n\n")
}

function getSunshineMetadata(metadata) {
  if (!metadata || typeof metadata !== "object") {
    return {}
  }

  // 兼容旧版插件写入的 `trellis` 键（只读，新写入一律用 `sunshine`）。
  const sunshine = metadata.sunshine || metadata.trellis
  if (!sunshine || typeof sunshine !== "object") {
    return {}
  }

  return sunshine
}

function markPartAsSessionStart(part) {
  const metadata = part.metadata && typeof part.metadata === "object"
    ? part.metadata
    : {}
  part.metadata = {
    ...metadata,
    sunshine: {
      ...getSunshineMetadata(metadata),
      sessionStart: true,
    },
  }
}

function hasSessionStartMarker(part) {
  if (!part || part.type !== "text" || typeof part.text !== "string") {
    return false
  }

  return getSunshineMetadata(part.metadata).sessionStart === true
}

export function hasInjectedSunshineContext(messages) {
  if (!Array.isArray(messages)) {
    return false
  }

  return messages.some(message => {
    if (!message?.info || message.info.role !== "user" || !Array.isArray(message.parts)) {
      return false
    }

    return message.parts.some(hasSessionStartMarker)
  })
}

export async function hasPersistedInjectedContext(client, directory, sessionID) {
  try {
    const response = await client.session.messages({
      path: { id: sessionID },
      query: { directory },
      throwOnError: true,
    })
    return hasInjectedSunshineContext(response.data || [])
  } catch (error) {
    debugLog(
      "session",
      "读取会话历史去重失败：",
      error instanceof Error ? error.message : String(error),
    )
    return false
  }
}

export function markContextInjected(part) {
  markPartAsSessionStart(part)
}
