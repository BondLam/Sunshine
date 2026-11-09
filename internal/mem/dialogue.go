// 对话清理：注入标签剥离、启动注入轮次识别、压缩边界 marker。
//
// 行为对照上游 dialogue.ts 重新实现，不复制源码：清理管线决定了
// 普通子串匹配做相关度排序是否可行——不去掉平台注入标签，
// 每个检索命中都会被注入内容主导。

package mem

import (
	"regexp"
	"strings"
)

// CompactBoundaryPrefix 是每个压缩边界 marker 轮次文本的固定开头，
// 供调用方（与测试）识别边界，无需依赖细节措辞。
const CompactBoundaryPrefix = "[compaction boundary]"

// injectionTags 是各平台注入的标签名：清理时整段去掉
// （含标签属性，大小写不敏感）。
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

var agentsPreambleStart = regexp.MustCompile(`(?m)^# AGENTS\.md instructions for`)

// StripInjectionTags 去掉文本里全部注入标签 `<tag>...</tag>` 整段内容
// 与开头的 AGENTS.md 前言，再把 3 个以上连续换行压成一段空行，
// 并裁剪首尾空白。
func StripInjectionTags(text string) string {
	out := text
	for _, pattern := range injectionTagPatterns {
		out = pattern.ReplaceAllString(out, "")
	}
	out = stripAgentsPreamble(out)
	out = regexp.MustCompile(`\n{3,}`).ReplaceAllString(out, "\n\n")
	return strings.TrimSpace(out)
}

// stripAgentsPreamble 去掉“# AGENTS.md instructions for …”前言段。
// Go 的 regexp 不支持前瞻，手动找段落边界：前言后第一个
// “空行 + 大写字母或中文”处结束；找不到则删到文末。
func stripAgentsPreamble(text string) string {
	loc := agentsPreambleStart.FindStringIndex(text)
	if loc == nil {
		return text
	}
	idx := loc[0]
	rest := text[idx:]
	// 前言起始行本身也要去掉：从前缀末尾开始找边界。
	after := rest[len("# AGENTS.md instructions for"):]
	for i := 0; i+2 < len(after); i++ {
		if after[i] == '\n' && after[i+1] == '\n' {
			r := []rune(after[i+2:])
			if len(r) > 0 {
				c := r[0]
				if (c >= 'A' && c <= 'Z') || (c >= 0x4E00 && c <= 0x9FA5) {
					return text[:idx] + after[i+2:]
				}
			}
		}
	}
	return text[:idx]
}

// IsBootstrapTurn 判断清理后的轮次是否为平台启动注入
// （AGENTS.md 前言、纯 INSTRUCTIONS 大块），是则整轮丢弃。
// 按清理后的文本判断，但长度阈值按原始长度（字符数）算。
func IsBootstrapTurn(cleaned string, originalLength int) bool {
	if strings.HasPrefix(cleaned, "# AGENTS.md instructions for") {
		return true
	}
	if originalLength > 4000 && strings.HasPrefix(strings.ToUpper(cleaned), "<INSTRUCTIONS>") {
		return true
	}
	return false
}

// CompactionBoundaryTurn 生成压缩边界 marker 轮次：role 为 user，
// kind 为 "marker"。detail 说明边界前轮次的去向，summary 是平台
// 自身的摘要或保留历史文本（可为空），记录模型当时认为重要的内容，
// 但永不计入对话轮次。
func CompactionBoundaryTurn(detail, summary string) DialogueTurn {
	body := strings.TrimSpace(summary)
	text := CompactBoundaryPrefix + " " + detail
	if body != "" {
		text += "\n\n" + body
	}
	return DialogueTurn{Role: RoleUser, Text: text, Kind: "marker"}
}

// TurnKey 是对话轮次的去重键：role 加清理后的精确文本。
func TurnKey(turn DialogueTurn) string {
	return turn.Role + "\x00" + turn.Text
}

// CleanTurns 清理一串对话轮次：去掉系统提示词、工具噪音与 hook 注入，
// 丢弃启动注入轮次与清理后为空的轮次。
//
// 压缩摘要按 marker 处理：kind 为 "marker" 的轮次原样留在池子里，
// 供 extract / context 展示模型上下文被截断的位置；search 计分时
// 由检索侧跳过。清理是幂等的：对已清理过的轮次再跑一次结果不变。
func CleanTurns(turns []DialogueTurn) []DialogueTurn {
	out := make([]DialogueTurn, 0, len(turns))
	for _, t := range turns {
		if t.IsMarker() {
			out = append(out, t)
			continue
		}
		cleaned := StripInjectionTags(t.Text)
		if cleaned == "" {
			continue
		}
		if IsBootstrapTurn(cleaned, len([]rune(t.Text))) {
			continue
		}
		t.Text = cleaned
		out = append(out, t)
	}
	return out
}
