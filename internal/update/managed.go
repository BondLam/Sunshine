package update

import (
	"os"
	"path/filepath"
	"strings"
)

// GetManagedBlock 取内容中起止标记之间的托管块（含起止标记本身）；
// 任一标记缺失时返回 ""（上游 getManagedBlock）。
func GetManagedBlock(content, startMarker, endMarker string) string {
	start := strings.Index(content, startMarker)
	if start < 0 {
		return ""
	}
	end := strings.Index(content[start:], endMarker)
	if end < 0 {
		return ""
	}
	return content[start : start+end+len(endMarker)]
}

// ReplaceManagedBlockContent 把已有内容里的托管块替换为模板里的托管块；
// 已有内容无块、或模板无块时返回 ""（上游 replaceManagedBlock）。
func ReplaceManagedBlockContent(existingContent, templateContent, startMarker, endMarker string) string {
	existingStart := strings.Index(existingContent, startMarker)
	if existingStart < 0 {
		return ""
	}
	existingEnd := strings.Index(existingContent[existingStart:], endMarker)
	if existingEnd < 0 {
		return ""
	}
	templateBlock := GetManagedBlock(templateContent, startMarker, endMarker)
	if templateBlock == "" {
		return ""
	}
	endPos := existingStart + existingEnd + len(endMarker)
	return existingContent[:existingStart] + templateBlock + existingContent[endPos:]
}

// MergeManagedBlockContent 合并托管块（上游 mergeManagedBlockContent）：
// 已有内容有块时只替换块内部分，块外用户编辑原样保留；
// 已有内容无块时把模板块追加到末尾；模板无块时返回模板全文。
func MergeManagedBlockContent(existingContent, templateContent, startMarker, endMarker string) string {
	if replaced := ReplaceManagedBlockContent(existingContent, templateContent, startMarker, endMarker); replaced != "" {
		return replaced
	}
	templateBlock := GetManagedBlock(templateContent, startMarker, endMarker)
	if templateBlock == "" {
		return templateContent
	}
	return strings.TrimRight(existingContent, " \t\r\n") + "\n\n" + templateBlock + "\n"
}

// BuildManagedBlockTemplate 按托管块合并出某文件的“期望内容”
// （上游 buildManagedBlockTemplate）：文件不存在时返回模板全文；
// 存在时把模板块合并进已有内容，块外编辑保留。
func BuildManagedBlockTemplate(cwd, rel, templateContent, startMarker, endMarker string) string {
	data, err := os.ReadFile(filepath.Join(cwd, filepath.FromSlash(rel)))
	if err != nil {
		return templateContent
	}
	return MergeManagedBlockContent(string(data), templateContent, startMarker, endMarker)
}

// GetSunshineManagedBlock 取内容中的 Sunshine 托管块。
func GetSunshineManagedBlock(content string) string {
	return GetManagedBlock(content, SunshineBlockStart, SunshineBlockEnd)
}

// legacyUntrackedAgentsMdBlockHashes 是旧版未进哈希清单的 AGENTS.md
// 托管块哈希集合（上游 LEGACY_UNTRACKED_AGENTS_MD_BLOCK_HASHES）。
// Sunshine 自阶段 1 起 init 就哈希追踪 AGENTS.md，没有这类旧项目，
// 集合为空——机制保留（文件存在、无哈希记录、块哈希命中时可自动更新），
// 供将来模板演变时填入。
var legacyUntrackedAgentsMdBlockHashes = map[string]bool{}

// IsKnownUntrackedTemplate 判断文件是否为“旧版未追踪的原版模板”
// （上游 isKnownUntrackedTemplate）：仅 AGENTS.md、
// 且其托管块哈希在 legacy 集合里时为真。
func IsKnownUntrackedTemplate(rel, existingContent string) bool {
	if rel != "AGENTS.md" {
		return false
	}
	block := GetSunshineManagedBlock(existingContent)
	if block == "" {
		return false
	}
	return legacyUntrackedAgentsMdBlockHashes[ComputeHashString(block)]
}

// IsSafeUntrackedCopilotInstructionsMerge 判断 Copilot instructions
// 文件是否可安全合并（上游 isSafeUntrackedCopilotInstructionsMerge）：
// 仅当路径是 Copilot 文件、已有内容无托管块、且“已有内容 + 模板块”
// 的合并结果恰好等于新内容时为真。
func IsSafeUntrackedCopilotInstructionsMerge(rel, existingContent, newContent, copilotTemplate string) bool {
	if rel != CopilotInstructionsPath {
		return false
	}
	if GetManagedBlock(existingContent, CopilotBlockStart, CopilotBlockEnd) != "" {
		return false
	}
	return MergeManagedBlockContent(existingContent, copilotTemplate, CopilotBlockStart, CopilotBlockEnd) == newContent
}
