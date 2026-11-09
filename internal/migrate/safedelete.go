package migrate

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
)

// SafeFileDeleteAction 是 safe-file-delete 项的分类结果。
type SafeFileDeleteAction string

const (
	// SafeDeleteDelete 内容哈希命中 allowed_hashes，可删除。
	SafeDeleteDelete SafeFileDeleteAction = "delete"
	// SafeDeleteSkipMissing 文件不存在，无需处理。
	SafeDeleteSkipMissing SafeFileDeleteAction = "skip-missing"
	// SafeDeleteSkipModified 内容已被改动（哈希未命中，或清单未给
	// allowed_hashes），保留。
	SafeDeleteSkipModified SafeFileDeleteAction = "skip-modified"
	// SafeDeleteSkipProtected 路径受保护，永不删除。
	SafeDeleteSkipProtected SafeFileDeleteAction = "skip-protected"
	// SafeDeleteSkipUpdateSkip 路径在 update.skip 列表中（破坏性版本
	// 用 bypassUpdateSkip 绕过时除外）。
	SafeDeleteSkipUpdateSkip SafeFileDeleteAction = "skip-update-skip"
)

// ClassifiedSafeFileDelete 是一条已分类的 safe-file-delete 项。
type ClassifiedSafeFileDelete struct {
	Item   MigrationItem
	Action SafeFileDeleteAction
}

// CollectSafeFileDeletes 收集并分类 safe-file-delete 迁移项。
//
// 无需 --migrate 也自动执行的条件：文件存在、路径不受保护、不在
// update.skip（除非 bypassUpdateSkip）、不在当前模板集合中（后一版本
// 恢复了该路径时，以当前模板为准）、内容哈希命中 allowed_hashes。
// 哈希匹配是最后的安全网：用户改过的文件一定留下来。
func CollectSafeFileDeletes(migrations []MigrationItem, cwd string, skipPaths []string, currentTemplatePaths map[string]bool, bypassUpdateSkip bool) []ClassifiedSafeFileDelete {
	var results []ClassifiedSafeFileDelete
	for _, item := range migrations {
		if item.Type != MigrationTypeSafeFileDelete {
			continue
		}
		if currentTemplatePaths[filepath.ToSlash(item.From)] {
			continue
		}
		classified := ClassifiedSafeFileDelete{Item: item}
		fullPath := filepath.Join(cwd, filepath.FromSlash(item.From))
		switch {
		case fileMissing(fullPath):
			classified.Action = SafeDeleteSkipMissing
		case IsProtectedPath(item.From):
			classified.Action = SafeDeleteSkipProtected
		case !bypassUpdateSkip && IsUpdateSkipped(item.From, skipPaths):
			classified.Action = SafeDeleteSkipUpdateSkip
		case len(item.AllowedHashes) == 0:
			// 未给 allowed_hashes 时为安全起见不删。
			classified.Action = SafeDeleteSkipModified
		default:
			hash, err := ComputeFileHash(fullPath)
			if err != nil {
				classified.Action = SafeDeleteSkipMissing
			} else if containsHash(item.AllowedHashes, hash) {
				classified.Action = SafeDeleteDelete
			} else {
				classified.Action = SafeDeleteSkipModified
			}
		}
		results = append(results, classified)
	}
	return results
}

func fileMissing(path string) bool {
	_, err := os.Stat(path)
	return os.IsNotExist(err)
}

func containsHash(hashes []string, target string) bool {
	for _, h := range hashes {
		if h == target {
			return true
		}
	}
	return false
}

// ExecuteSafeFileDeletes 删除全部被分类为 delete 的文件，并清理空目录、
// 从哈希清单里移除对应条目。返回删除数与执行中的错误（单个文件失败
// 不中断其余删除）。
func ExecuteSafeFileDeletes(classified []ClassifiedSafeFileDelete, cwd string) (int, []error) {
	deleted := 0
	var errs []error
	hashes, err := LoadHashes(cwd)
	if err != nil {
		return 0, []error{err}
	}
	hashesChanged := false
	for _, c := range classified {
		if c.Action != SafeDeleteDelete {
			continue
		}
		fullPath := filepath.Join(cwd, filepath.FromSlash(c.Item.From))
		if err := os.Remove(fullPath); err != nil {
			errs = append(errs, fmt.Errorf("删除 %s 失败：%w", c.Item.From, err))
			continue
		}
		delete(hashes, filepath.ToSlash(c.Item.From))
		hashesChanged = true
		CleanupEmptyDirs(cwd, filepath.Dir(c.Item.From))
		deleted++
	}
	if hashesChanged {
		if err := SaveHashes(cwd, hashes); err != nil {
			errs = append(errs, err)
		}
	}
	return deleted, errs
}

// SafeFileDeleteCounts 汇总 safe-file-delete 的分类数量。
func SafeFileDeleteCounts(classified []ClassifiedSafeFileDelete) map[SafeFileDeleteAction]int {
	counts := map[SafeFileDeleteAction]int{}
	for _, c := range classified {
		counts[c.Action]++
	}
	return counts
}

// PrintSafeFileDeleteSummary 向 w 打印 safe-file-delete 汇总（中文）。
// 无待删、无已改动、无 update.skip 跳过时不打印任何内容。
func PrintSafeFileDeleteSummary(classified []ClassifiedSafeFileDelete, w io.Writer) {
	counts := SafeFileDeleteCounts(classified)
	if counts[SafeDeleteDelete] == 0 && counts[SafeDeleteSkipModified] == 0 && counts[SafeDeleteSkipUpdateSkip] == 0 {
		return
	}
	fmt.Fprintln(w, "  过期文件清理：")
	for _, c := range classified {
		switch c.Action {
		case SafeDeleteDelete:
			if c.Item.Description != "" {
				fmt.Fprintf(w, "    ✕ %s（%s）\n", c.Item.From, c.Item.Description)
			} else {
				fmt.Fprintf(w, "    ✕ %s\n", c.Item.From)
			}
		case SafeDeleteSkipModified:
			fmt.Fprintf(w, "    ? %s（已改动，保留）\n", c.Item.From)
		case SafeDeleteSkipUpdateSkip:
			fmt.Fprintf(w, "    ○ %s（在 update.skip 中，跳过）\n", c.Item.From)
		}
	}
	fmt.Fprintln(w)
}
