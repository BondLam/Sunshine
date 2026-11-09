package update

import (
	"strings"
)

// 托管块标记（上游 TRELLIS:START/END 按改名规则译为 SUNSHINE:START/END）。
const (
	// SunshineBlockStart 是 Sunshine 托管块的起始标记。
	SunshineBlockStart = "<!-- SUNSHINE:START -->"
	// SunshineBlockEnd 是 Sunshine 托管块的结束标记。
	SunshineBlockEnd = "<!-- SUNSHINE:END -->"
)

// Copilot instructions 文件（上游同名常量；Sunshine 暂无 Copilot 平台，
// 该文件不会由任何平台模板生成，下面的合并函数目前只在“用户已有此文件
// 且 update 模板集合包含它”时才生效——现阶段是休眠路径，
// 实现与测试先行，待 Copilot 平台接入时启用）。
const (
	// CopilotInstructionsPath 是 Copilot instructions 文件的相对路径。
	CopilotInstructionsPath = ".github/copilot-instructions.md"
	// CopilotBlockStart 是 Copilot instructions 文件内托管块的起始标记。
	CopilotBlockStart = "<!-- SUNSHINE:COPILOT-GUIDANCE:START -->"
	// CopilotBlockEnd 是 Copilot instructions 文件内托管块的结束标记。
	CopilotBlockEnd = "<!-- SUNSHINE:COPILOT-GUIDANCE:END -->"
)

// ProtectedPaths 是永不迁移、不删除的用户数据路径（上游 PROTECTED_PATHS，
// 按 Sunshine 目录改名；spec/ 是 init 时生成的用户定制规范，
// update 绝不改动它）。
var ProtectedPaths = []string{
	".sunshine/workspace",
	".sunshine/tasks",
	".sunshine/spec",
	".sunshine/.developer",
	".sunshine/.current-task",
}

// IsProtectedPath 判断路径是否受 PROTECTED_PATHS 保护
// （等于某条保护路径，或位于其下）。
func IsProtectedPath(path string) bool {
	p := ToPosix(path)
	for _, pp := range ProtectedPaths {
		if p == pp || strings.HasPrefix(p, pp+"/") {
			return true
		}
	}
	return false
}

// IsPathSkipped 判断路径是否命中 update.skip 列表
// （等于某条跳过路径，或位于其下）。
func IsPathSkipped(path string, skipPaths []string) bool {
	p := ToPosix(path)
	for _, skip := range skipPaths {
		s := ToPosix(strings.TrimSpace(skip))
		if s == "" {
			continue
		}
		if p == s || strings.HasPrefix(p, s+"/") {
			return true
		}
	}
	return false
}

// BackupDirs 是备份的完整快照目录（上游 BACKUP_DIRS = ALL_MANAGED_DIRS）：
// .sunshine/ 本身加全部已接平台的配置目录（.agents/ 是 Codex 与多平台
// 共用的 skill 目录）。后续平台接入时在这里追加其配置目录。
var BackupDirs = []string{
	".sunshine",
	".claude",
	".cursor",
	".codex",
	".agents",
}

// BackupFiles 是备份包含的根级托管文件（上游 BACKUP_FILES = [AGENTS.md]）。
var BackupFiles = []string{"AGENTS.md"}

// BackupExcludePatterns 是备份排除规则（上游 BACKUP_EXCLUDE_PATTERNS，
// 另加 ".backups/"：备份目录本身不进下一次备份；
// ".backup-" 保留，兼容旧版备份目录名）。
var BackupExcludePatterns = []string{
	".backup-",
	".backups/",
	"/node_modules",
	"/workspace/",
	"/tasks/",
	"/spec/",
	"/backlog/",
	"/agent-traces/",
	"/worktrees/",
	"/worktree/",
}

// ShouldExcludeFromBackup 判断相对路径是否不进备份（上游同名函数）。
func ShouldExcludeFromBackup(rel string) bool {
	normalized := strings.ReplaceAll(ToPosix(rel), "\\", "/")
	for _, pattern := range BackupExcludePatterns {
		if strings.Contains(normalized, pattern) {
			return true
		}
	}
	return false
}
