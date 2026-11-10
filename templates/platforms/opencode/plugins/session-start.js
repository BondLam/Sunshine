/* global process */
/**
 * Sunshine 会话启动插件
 *
 * 把精简的 SessionStart 上下文注入 OpenCode 发给模型的最新用户消息副本。
 * 使用 `experimental.chat.messages.transform`，因此 TUI / Web / SQLite
 * 历史记录不受影响（issue #553）。
 */

import { SunshineContext, debugLog, isSunshineSubagent } from "../lib/sunshine-context.js"
import {
  MESSAGES_TRANSFORM_HOOK,
  platformInputFromMessages,
  prependEphemeralText,
  transcriptHasAssistantMessage,
} from "../lib/context-visibility.js"
import { buildSessionContext } from "../lib/session-utils.js"

const FIRST_REPLY_NOTICE_RE = /<first-reply-notice>[\s\S]*?<\/first-reply-notice>\s*/g

function stripFirstReplyNotice(context) {
  return context.replace(FIRST_REPLY_NOTICE_RE, "")
}

// OpenCode 1.2.x 要求插件是工厂函数（见 inject-subagent-context.js 的注释）。
export default async ({ directory }) => {
  const ctx = new SunshineContext(directory)
  debugLog("session", "插件已加载，目录：", directory)

  return {
    [MESSAGES_TRANSFORM_HOOK]: async (_input, output) => {
      try {
        const messages = output?.messages
        const platformInput = platformInputFromMessages(messages)
        const agent = platformInput?.agent || "unknown"
        debugLog("session", "messages.transform 已调用，agent：", agent)

        if (isSunshineSubagent(platformInput)) {
          debugLog("session", "跳过 sunshine 子代理轮次：", agent)
          return
        }

        if (
          process.env.SUNSHINE_HOOKS === "0" ||
          process.env.TRELLIS_HOOKS === "0" ||
          process.env.SUNSHINE_DISABLE_HOOKS === "1" ||
          process.env.TRELLIS_DISABLE_HOOKS === "1"
        ) {
          debugLog("session", "跳过——SUNSHINE_HOOKS 已禁用")
          return
        }

        if (process.env.OPENCODE_NON_INTERACTIVE === "1") {
          debugLog("session", "跳过——非交互模式")
          return
        }

        let context = buildSessionContext(ctx, platformInput)
        if (transcriptHasAssistantMessage(messages)) {
          context = stripFirstReplyNotice(context)
        }
        debugLog("session", "上下文已构建，长度：", context.length)
        prependEphemeralText(messages, context)
      } catch (error) {
        debugLog("session", "messages.transform 出错：", error.message, error.stack)
      }
    },
  }
}
