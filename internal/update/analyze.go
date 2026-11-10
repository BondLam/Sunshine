package update

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
)

// FileChange 是一条待处理的文件变更。
type FileChange struct {
	// Path 是磁盘绝对路径。
	Path string
	// RelativePath 是相对项目根的 POSIX 路径。
	RelativePath string
	// NewContent 是模板的新内容。
	NewContent string
	// Status 是 new | unchanged | changed（见 ChangeAnalysis）。
	Status string
}

// ChangeAnalysis 是变更分析的结果（上游同名结构）。
type ChangeAnalysis struct {
	// NewFiles 是模板新增的文件（清单无记录、磁盘不存在）。
	NewFiles []FileChange
	// UnchangedFiles 是与模板字节一致的文件。
	UnchangedFiles []FileChange
	// AutoUpdateFiles 是模板已更新、用户未改动的文件（自动更新）。
	AutoUpdateFiles []FileChange
	// ChangedFiles 是用户已改动的文件（需逐个交互确认）。
	ChangedFiles []FileChange
	// UserDeletedFiles 是清单有记录、但用户已删除的文件（尊重删除，不恢复）。
	UserDeletedFiles []FileChange
	// ProtectedPaths 是受保护的用户数据路径。
	ProtectedPaths []string
}

// AnalyzeChanges 按 .template-hashes.json 做哈希三态判定
// （上游 analyzeChanges）：
//   - 磁盘不存在 + 清单有记录 → userDeletedFiles（尊重删除）；
//   - 磁盘不存在 + 清单无记录 → newFiles；
//   - 磁盘内容与模板一致 → unchangedFiles；
//   - 磁盘内容与模板不同 + 当前哈希等于清单哈希（或旧版未追踪的原版
//     模板、或可安全合并的 Copilot 文件）→ autoUpdateFiles；
//   - 其余（哈希不同或无清单记录）→ changedFiles，需交互确认。
//
// copilotTemplate 是 Copilot instructions 的模板内容（取自本次收集的
// 模板集合里合并后的 .github/copilot-instructions.md；Copilot 未配置
// 时传 ""；isSafeUntrackedCopilotInstructionsMerge 只在模板集合
// 包含该文件时才可能命中）。
func AnalyzeChanges(cwd string, hashes map[string]string, templates map[string]string, copilotTemplate string) ChangeAnalysis {
	result := ChangeAnalysis{ProtectedPaths: append([]string(nil), ProtectedPaths...)}
	keys := make([]string, 0, len(templates))
	for k := range templates {
		keys = append(keys, k)
	}
	// 排序后处理：输出稳定，测试可断言。
	sort.Strings(keys)
	for _, rel := range keys {
		newContent := templates[rel]
		fullPath := filepath.Join(cwd, filepath.FromSlash(rel))
		change := FileChange{Path: fullPath, RelativePath: rel, NewContent: newContent, Status: "new"}
		data, err := os.ReadFile(fullPath)
		if err != nil {
			if !os.IsNotExist(err) {
				continue
			}
			if _, ok := hashes[ToPosix(rel)]; ok {
				result.UserDeletedFiles = append(result.UserDeletedFiles, change)
			} else {
				result.NewFiles = append(result.NewFiles, change)
			}
			continue
		}
		existingContent := string(data)
		if existingContent == newContent {
			change.Status = "unchanged"
			result.UnchangedFiles = append(result.UnchangedFiles, change)
			continue
		}
		storedHash, hasStored := hashes[ToPosix(rel)]
		currentHash := ComputeHashString(existingContent)
		if (hasStored && storedHash == currentHash) ||
			(!hasStored && IsKnownUntrackedTemplate(rel, existingContent)) ||
			(!hasStored && IsSafeUntrackedCopilotInstructionsMerge(rel, existingContent, newContent, copilotTemplate)) {
			change.Status = "changed"
			result.AutoUpdateFiles = append(result.AutoUpdateFiles, change)
		} else {
			change.Status = "changed"
			result.ChangedFiles = append(result.ChangedFiles, change)
		}
	}
	return result
}

// CollectUnchangedFileHashRepairs 收集“文件与模板字节一致、但清单条目
// 错误或缺失”的修复项（上游同名函数）：这类文件落在 unchanged 桶，
// 写盘阶段不会碰它们，清单错误会永远残留；这里直接记下模板哈希修复。
// 排除清单本就不追踪的路径（ShouldExcludeFromHash）；
// AGENTS.md、config.yaml 这类混合归属文件允许与清单哈希不同，
// 不在此修复（它们的定制内容本就与模板不同）。
func CollectUnchangedFileHashRepairs(changes ChangeAnalysis, hashes map[string]string) map[string]string {
	files := map[string]string{}
	for _, file := range changes.UnchangedFiles {
		key := ToPosix(file.RelativePath)
		recorded, ok := hashes[key]
		if !ok {
			if !ShouldExcludeFromHash(key) {
				files[key] = file.NewContent
			}
			continue
		}
		if recorded != ComputeHashString(file.NewContent) {
			files[key] = file.NewContent
		}
	}
	return files
}

// PrintChangeSummary 把变更摘要打印到 w（上游 printChangeSummary，
// 去掉颜色码：CLI 输出保持纯文本，便于管道与测试断言）。
func PrintChangeSummary(w io.Writer, cwd string, changes ChangeAnalysis) {
	fmt.Fprint(w, "\n正在扫描变更……\n")
	if len(changes.NewFiles) > 0 {
		fmt.Fprintln(w, "  新增文件（将添加）：")
		for _, f := range changes.NewFiles {
			fmt.Fprintf(w, "    + %s\n", f.RelativePath)
		}
		fmt.Fprintln(w)
	}
	if len(changes.AutoUpdateFiles) > 0 {
		fmt.Fprintln(w, "  模板已更新（将自动更新）：")
		for _, f := range changes.AutoUpdateFiles {
			fmt.Fprintf(w, "    ↑ %s\n", f.RelativePath)
		}
		fmt.Fprintln(w)
	}
	if len(changes.UnchangedFiles) > 0 {
		fmt.Fprintln(w, "  未变更文件（将跳过）：")
		for _, f := range changes.UnchangedFiles[:min(5, len(changes.UnchangedFiles))] {
			fmt.Fprintf(w, "    ○ %s\n", f.RelativePath)
		}
		if len(changes.UnchangedFiles) > 5 {
			fmt.Fprintf(w, "    ……另有 %d 个\n", len(changes.UnchangedFiles)-5)
		}
		fmt.Fprintln(w)
	}
	if len(changes.ChangedFiles) > 0 {
		fmt.Fprintln(w, "  你已修改（需要你决定）：")
		for _, f := range changes.ChangedFiles {
			fmt.Fprintf(w, "    ? %s\n", f.RelativePath)
		}
		fmt.Fprintln(w)
	}
	if len(changes.UserDeletedFiles) > 0 {
		fmt.Fprintln(w, "  你已删除（保持删除，不恢复）：")
		for _, f := range changes.UserDeletedFiles {
			fmt.Fprintf(w, "    ✕ %s\n", f.RelativePath)
		}
		fmt.Fprintln(w)
	}
	var existingProtected []string
	for _, p := range changes.ProtectedPaths {
		if _, err := os.Stat(filepath.Join(cwd, filepath.FromSlash(p))); err == nil {
			existingProtected = append(existingProtected, p)
		}
	}
	if len(existingProtected) > 0 {
		fmt.Fprintln(w, "  用户数据（保持不动）：")
		for _, p := range existingProtected {
			fmt.Fprintf(w, "    ○ %s/\n", p)
		}
		fmt.Fprintln(w)
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
