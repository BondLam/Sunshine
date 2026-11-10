import { createHash } from "node:crypto"

const PART_ID_PATTERN = /^prt_([0-9a-f]{12})[0-9A-Za-z]{14}$/
const CONTEXT_PART_KINDS = {
  sessionStart: { offset: 2n, slot: "0" },
  workflowState: { offset: 1n, slot: "1" },
}
const MAX_CONTEXT_PART_OFFSET = Object.values(CONTEXT_PART_KINDS).reduce(
  (maximum, definition) => definition.offset > maximum ? definition.offset : maximum,
  0n,
)

/**
 * 返回第一个普通用户撰写的文本 part。
 *
 * Sunshine 插件可能以任意顺序运行，调用方不要把另一个插件插入的
 * 合成上下文 part 误当成用户提示词。
 */
export function findUserTextPart(parts) {
  if (!Array.isArray(parts)) return undefined
  return parts.find(
    part => part?.type === "text" && part.synthetic !== true && part.text !== undefined,
  )
}

function findIdentitySourcePart(parts) {
  return parts
    .filter(
      part =>
        part?.synthetic !== true &&
        typeof part?.id === "string" &&
        typeof part?.sessionID === "string" &&
        typeof part?.messageID === "string" &&
        PART_ID_PATTERN.test(part.id),
    )
    .sort((left, right) => (left.id < right.id ? -1 : left.id > right.id ? 1 : 0))[0]
}

function contextPartId(source, kind) {
  const definition = CONTEXT_PART_KINDS[kind]
  if (!definition) throw new TypeError(`未知的上下文 part 类型：${kind}`)

  const match = PART_ID_PATTERN.exec(source.id)
  if (!match) throw new TypeError(`不支持的 OpenCode part ID：${source.id}`)
  const sourceOrdinal = BigInt(`0x${match[1]}`)
  if (sourceOrdinal <= MAX_CONTEXT_PART_OFFSET) {
    throw new TypeError(`不支持的 OpenCode part ID 序号：${source.id}`)
  }
  const ordinal = sourceOrdinal - definition.offset
  const suffix = createHash("sha256")
    .update(`${source.messageID}\0${kind}`)
    .digest("hex")
    .slice(0, 13)
  return `prt_${ordinal.toString(16).padStart(12, "0")}${definition.slot}${suffix}`
}

/**
 * 把机器生成的上下文作为合成文本 part 持久化到普通用户 part 之前，且
 * 不修改任何已有 part。生成的 ID 在首次请求与存储回放时都排在源用户
 * part 之前，顺序一致。
 */
export function insertSyntheticTextPart(parts, text, kind) {
  if (!Array.isArray(parts)) throw new TypeError("parts 必须是数组")
  if (typeof text !== "string") throw new TypeError("text 必须是字符串")
  if (!CONTEXT_PART_KINDS[kind]) throw new TypeError(`未知的上下文 part 类型：${kind}`)

  const source = findIdentitySourcePart(parts)
  if (!source) throw new TypeError("没有带持久化身份的普通 OpenCode part")

  const id = contextPartId(source, kind)
  if (parts.some(part => part?.id === id)) {
    throw new TypeError(`重复的合成上下文 part：${kind}`)
  }
  const part = {
    id,
    sessionID: source.sessionID,
    messageID: source.messageID,
    type: "text",
    text,
    synthetic: true,
  }
  const insertionIndex = parts.findIndex(existing => typeof existing?.id !== "string" || id < existing.id)
  if (insertionIndex === -1) parts.push(part)
  else parts.splice(insertionIndex, 0, part)
  return part
}

/** OpenCode hook：只改内存中的模型输入，不改存储的历史记录。 */
export const MESSAGES_TRANSFORM_HOOK = "experimental.chat.messages.transform"

/**
 * OpenCode `{info, parts}[]` transcript 中最后一条用户消息的下标。
 * 压缩与提示词都会把这种结构传给 messages.transform。
 */
export function findLatestUserMessageIndex(messages) {
  if (!Array.isArray(messages)) return -1
  for (let i = messages.length - 1; i >= 0; i--) {
    if (messages[i]?.info?.role === "user") return i
  }
  return -1
}

export function platformInputFromMessages(messages) {
  const index = findLatestUserMessageIndex(messages)
  if (index < 0) return null
  const info = messages[index].info
  if (!info || typeof info !== "object") return null
  return {
    sessionID: info.sessionID,
    agent: info.agent,
  }
}

export function latestUserPromptText(messages) {
  const index = findLatestUserMessageIndex(messages)
  if (index < 0) return ""
  const part = findUserTextPart(messages[index].parts)
  return typeof part?.text === "string" ? part.text : ""
}

export function transcriptHasAssistantMessage(messages) {
  if (!Array.isArray(messages)) return false
  return messages.some(message => message?.info?.role === "assistant")
}

/**
 * 克隆最新用户消息，并在前面加一个临时的合成文本 part。原始消息对象
 * 及其 `parts` 数组保持不动，这样 transform 不会泄漏进 OpenCode
 * 存储的历史记录。
 */
export function prependEphemeralText(messages, text) {
  if (!Array.isArray(messages)) return false
  if (typeof text !== "string") return false
  const index = findLatestUserMessageIndex(messages)
  if (index < 0) return false
  const original = messages[index]
  const parts = Array.isArray(original.parts) ? original.parts.slice() : []
  parts.unshift({
    type: "text",
    text,
    synthetic: true,
  })
  messages[index] = {
    ...original,
    parts,
  }
  return true
}
