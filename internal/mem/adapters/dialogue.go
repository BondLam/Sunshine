package adapters

import (
	"math"
	"regexp"
	"strings"
	"time"

	"github.com/linrong1982/Sunshine/internal/mem"
)

// 对话清理：注入标签剥离、启动注入轮次识别、压缩边界 marker。
// 与上游 dialogue.ts 行为一致。

// injectionTags 是各平台注入的标签名，清理时整段去掉。
var injectionTags = []string{
	"system-reminder",
	"task-status",
	"ready",
	"current-state",
	"workflow",
	"workflow-state",
	"guidelines",
	"instructions",
	"command-name",
	"command-message",
	"command-args",
	"local-command-stdout",
	"local-command-stderr",
	"permissions instructions",
	"collaboration_mode",
	"environment_context",
	"auto_compact_summary",
	"user_instructions",
}

var injectionTagPatterns = func() []*regexp.Regexp {
	patterns := make([]*regexp.Regexp, 0, len(injectionTags))
	for _, tag := range injectionTags {
		escaped := regexp.QuoteMeta(tag)
		patterns = append(patterns, regexp.MustCompile(`(?is)<`+escaped+`[^>]*>.*?</`+escaped+`>`))
	}
	return patterns
}()

// stripInjectionTags 去掉注入标签整段内容与 AGENTS.md 前言，
// 再把 3 个以上连续换行压成一段空行并裁剪首尾空白。
func stripInjectionTags(text string) string {
	out := text
	for _, pattern := range injectionTagPatterns {
		out = pattern.ReplaceAllString(out, "")
	}
	out = stripAgentsPreamble(out)
	out = regexp.MustCompile(`\n{3,}`).ReplaceAllString(out, "\n\n")
	return strings.TrimSpace(out)
}

// stripAgentsPreamble 去掉开头的“# AGENTS.md instructions for …”前言段。
// Go 的 regexp 不支持前瞻，手动找段落边界：前言后第一个“空行 + 大写字母
// 或中文”处结束；找不到则删到文末。
func stripAgentsPreamble(text string) string {
	const prefix = "# AGENTS.md instructions for"
	idx := strings.Index(text, prefix)
	if idx == -1 {
		return text
	}
	rest := text[idx+len(prefix):]
	// 找 \n\n 后跟大写字母或中文的位置。
	for i := 0; i+2 < len(rest); i++ {
		if rest[i] == '\n' && rest[i+1] == '\n' {
			r := []rune(rest[i+2:])
			if len(r) > 0 {
				c := r[0]
				if (c >= 'A' && c <= 'Z') || (c >= 0x4E00 && c <= 0x9FFF) {
					return text[:idx] + rest[i+2:]
				}
			}
		}
	}
	return text[:idx]
}

// isBootstrapTurn 判断清理后的轮次是否为平台启动注入（AGENTS.md 前言、
// 纯 INSTRUCTIONS 大块），是则整轮丢弃。按清理后的文本判断，
// 但长度阈值按原始长度算。
func isBootstrapTurn(cleaned string, originalLength int) bool {
	if strings.HasPrefix(cleaned, "# AGENTS.md instructions for") {
		return true
	}
	if originalLength > 4000 && strings.HasPrefix(strings.ToUpper(cleaned), "<INSTRUCTIONS>") {
		return true
	}
	return false
}

// CompactBoundaryPrefix 是每个压缩边界 marker 文本的固定开头，
// 供调用方（与测试）识别边界，无需依赖细节措辞。
const CompactBoundaryPrefix = "[compaction boundary]"

// compactionBoundaryTurn 生成压缩边界 marker 轮次：role 为 user，
// kind 为 "marker"，detail 说明边界前轮次的去向，summary 是平台
// 自身的摘要或保留历史文本（可为空）。
func compactionBoundaryTurn(detail, summary string) mem.DialogueTurn {
	body := strings.TrimSpace(summary)
	text := CompactBoundaryPrefix + " " + detail
	if body != "" {
		text += "\n\n" + body
	}
	return mem.DialogueTurn{Role: "user", Text: text, Kind: "marker"}
}

// turnKey 是对话轮次的去重键：role 加清理后的精确文本。
func turnKey(role, text string) string {
	return role + "\x00" + text
}

// formatISOTime 把时间转成与 JS toISOString 相同的格式
// （UTC、毫秒、末尾 Z，如 2026-11-09T10:00:00.000Z）。
func formatISOTime(t time.Time) string {
	return t.UTC().Format("2006-01-02T15:04:05.000Z")
}

// toISOUnixEpoch 把 Unix 秒或毫秒时间戳转成 ISO 字符串；
// 非法值（<=0 或超范围）返回 ""。
func toISOUnixEpoch(epoch float64) string {
	if epoch <= 0 || math.IsNaN(epoch) || math.IsInf(epoch, 0) {
		return ""
	}
	ms := epoch
	if ms < 1e12 {
		ms *= 1000
	}
	if ms > 8.64e15 {
		return ""
	}
	return formatISOTime(time.UnixMilli(int64(ms)))
}

// toISOEpochMs 把 Unix 毫秒时间戳转成 ISO 字符串；非法值返回 ""。
func toISOEpochMs(epochMs float64) string {
	if epochMs <= 0 || math.IsNaN(epochMs) || math.IsInf(epochMs, 0) {
		return ""
	}
	if epochMs > 8.64e15 {
		return ""
	}
	return formatISOTime(time.UnixMilli(int64(epochMs)))
}
