package migrate

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// SortMigrationsForExecution 按安全执行顺序排序迁移项：
//   - rename-dir 排在 rename / delete 之前（先搬目录）；
//   - rename-dir 之间按源路径深度从深到浅（先搬嵌套的子目录）；
//   - 其余保持原有相对顺序（稳定排序）。
func SortMigrationsForExecution(migrations []MigrationItem) []MigrationItem {
	sorted := make([]MigrationItem, len(migrations))
	copy(sorted, migrations)
	sort.SliceStable(sorted, func(i, j int) bool {
		a, b := sorted[i], sorted[j]
		if a.Type == MigrationTypeRenameDir && b.Type == MigrationTypeRenameDir {
			return pathDepth(a.From) > pathDepth(b.From)
		}
		if a.Type == MigrationTypeRenameDir && b.Type != MigrationTypeRenameDir {
			return true
		}
		if a.Type != MigrationTypeRenameDir && b.Type == MigrationTypeRenameDir {
			return false
		}
		return false
	})
	return sorted
}

func pathDepth(path string) int {
	return len(strings.Split(filepath.ToSlash(strings.Trim(path, "/")), "/"))
}

// IsFileSafeToReplace 判断目标文件是否可安全覆盖：不存在即可；
// 存在时只有内容与当前模板完全一致才安全。
func IsFileSafeToReplace(cwd, rel string, templates map[string]string) bool {
	full := filepath.Join(cwd, filepath.FromSlash(rel))
	data, err := os.ReadFile(full)
	if err != nil {
		return os.IsNotExist(err)
	}
	template, ok := templates[filepath.ToSlash(rel)]
	if !ok {
		return false
	}
	return string(data) == template
}

// IsDirectorySafeToReplace 判断目录是否只含未改动的模板文件（可安全删除
// 后再搬入源目录）：不存在或为空即安全；每个文件要么内容与当前模板一致，
// 要么在哈希清单里有记录且未被改动。
func IsDirectorySafeToReplace(cwd, dirRel string, hashes map[string]string, templates map[string]string) bool {
	full := filepath.Join(cwd, filepath.FromSlash(dirRel))
	files, err := collectFiles(full)
	if err != nil {
		return os.IsNotExist(err)
	}
	for _, file := range files {
		rel, err := filepath.Rel(cwd, file)
		if err != nil {
			return false
		}
		rel = filepath.ToSlash(rel)
		if template, ok := templates[rel]; ok {
			data, err := os.ReadFile(file)
			if err == nil && string(data) == template {
				continue
			}
		}
		if _, ok := hashes[rel]; ok && !IsTemplateModified(cwd, rel, hashes) {
			continue
		}
		return false
	}
	return true
}

// DirMatchesCurrentTemplates 判断目录下每个文件的内容是否都与当前模板
// 字节一致（且至少有一个文件）。用于 rename-dir 目标已存在时的方向判定：
// 目标已是新版标准内容时，不得用旧源目录覆盖它。
func DirMatchesCurrentTemplates(cwd, dirRel string, templates map[string]string) bool {
	full := filepath.Join(cwd, filepath.FromSlash(dirRel))
	files, err := collectFiles(full)
	if err != nil || len(files) == 0 {
		return false
	}
	for _, file := range files {
		rel, err := filepath.Rel(cwd, file)
		if err != nil {
			return false
		}
		template, ok := templates[filepath.ToSlash(rel)]
		if !ok {
			return false
		}
		data, err := os.ReadFile(file)
		if err != nil || string(data) != template {
			return false
		}
	}
	return true
}

// collectFiles 递归收集目录下全部文件（返回绝对路径）。
func collectFiles(dir string) ([]string, error) {
	var files []string
	err := filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() {
			files = append(files, path)
		}
		return nil
	})
	return files, err
}

// ClassifyMigrations 按文件状态把迁移项分类。
//
//   - safe-file-delete 不走这里（由 CollectSafeFileDeletes 单独处理）；
//   - 受保护路径的源文件永不迁移；delete 类也不得写向受保护路径
//     （rename / rename-dir 允许移入受保护路径）；
//   - rename：目标已存在时，目标内容是模板则自动迁移，否则冲突；
//     目标不存在时，未改动则自动迁移，已改动则需确认；
//   - rename-dir：目标已存在时，目标只含未改动模板文件则自动迁移，
//     否则冲突；目标不存在时，源目录有哈希清单记录则自动迁移，
//     否则跳过（很可能是用户自己的同名目录，不得悄悄搬走）；
//   - delete：未改动则自动删除，已改动则需确认；
//   - 源不存在、类型非法或缺目标路径的项一律跳过。
func ClassifyMigrations(migrations []MigrationItem, cwd string, hashes map[string]string, templates map[string]string) ClassifiedMigrations {
	result := ClassifiedMigrations{}
	for _, item := range migrations {
		if item.Type == MigrationTypeSafeFileDelete {
			continue
		}
		if IsProtectedPath(item.From) {
			result.Skip = append(result.Skip, item)
			continue
		}
		if item.To != "" && IsProtectedPath(item.To) &&
			item.Type != MigrationTypeRename && item.Type != MigrationTypeRenameDir {
			result.Skip = append(result.Skip, item)
			continue
		}
		if err := item.Validate(); err != nil {
			result.Skip = append(result.Skip, item)
			continue
		}
		oldPath := filepath.Join(cwd, filepath.FromSlash(item.From))
		if _, err := os.Stat(oldPath); err != nil {
			result.Skip = append(result.Skip, item)
			continue
		}
		switch item.Type {
		case MigrationTypeRename:
			newPath := filepath.Join(cwd, filepath.FromSlash(item.To))
			if _, err := os.Stat(newPath); err == nil {
				if IsFileSafeToReplace(cwd, item.To, templates) {
					result.Auto = append(result.Auto, item)
				} else {
					result.Conflict = append(result.Conflict, item)
				}
			} else if IsTemplateModified(cwd, item.From, hashes) {
				result.Confirm = append(result.Confirm, item)
			} else {
				result.Auto = append(result.Auto, item)
			}
		case MigrationTypeRenameDir:
			newPath := filepath.Join(cwd, filepath.FromSlash(item.To))
			if _, err := os.Stat(newPath); err == nil {
				if IsDirectorySafeToReplace(cwd, item.To, hashes, templates) {
					result.Auto = append(result.Auto, item)
				} else {
					result.Conflict = append(result.Conflict, item)
				}
			} else if DirHasManifestEntries(item.From, hashes) {
				result.Auto = append(result.Auto, item)
			} else {
				result.Skip = append(result.Skip, item)
			}
		case MigrationTypeDelete:
			if IsTemplateModified(cwd, item.From, hashes) {
				result.Confirm = append(result.Confirm, item)
			} else {
				result.Auto = append(result.Auto, item)
			}
		}
	}
	return result
}
