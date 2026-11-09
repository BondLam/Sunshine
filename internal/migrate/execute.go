package migrate

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// ExecuteOptions 是 ExecuteMigrations 的选项。
type ExecuteOptions struct {
	// Force 为真时，已改动文件不经确认直接执行（须已做完整备份）。
	Force bool
	// SkipAll 为真时，已改动文件一律跳过，不确认。
	SkipAll bool
	// Prompter 是交互确认函数；为空时用默认的 PromptMigrationAction。
	Prompter Prompter
	// Templates 是当前版本模板内容（相对项目根的 POSIX 路径 → 内容），
	// 用于 rename-dir 目标已存在时的方向判定。
	Templates map[string]string
}

// ExecuteMigrations 执行已分类的迁移：
//  1. auto 项按 SortMigrationsForExecution 的顺序直接执行；
//  2. confirm 项按 Force / SkipAll / 交互确认决定执行或跳过，
//     选“备份后执行”时先写一份 .backup；
//  3. conflict 项不执行，计入结果的 Conflicts；skip 项计入 Skipped。
//
// 哈希清单在执行结束时统一保存一次（rename 更新键名，delete 删键，
// rename-dir 批量改前缀）。单个迁移项失败时记入结果的 Errors 并继续
// 执行其余项，调用结束时同时返回 error。
func ExecuteMigrations(classified ClassifiedMigrations, cwd string, opts ExecuteOptions) (MigrationResult, error) {
	result := MigrationResult{Conflicts: len(classified.Conflict)}
	prompter := opts.Prompter
	if prompter == nil {
		prompter = promptMigrationActionImpl
	}
	hashes, err := LoadHashes(cwd)
	if err != nil {
		return result, err
	}
	hashesChanged := false

	for _, item := range SortMigrationsForExecution(classified.Auto) {
		kind, err := executeAutoItem(cwd, item, hashes, opts.Templates)
		if err != nil {
			result.Errors = append(result.Errors, fmt.Sprintf("%s：%v", describeItem(item), err))
			continue
		}
		hashesChanged = true
		switch kind {
		case countRenamed:
			result.Renamed++
		case countDeleted:
			result.Deleted++
		}
	}

	for _, item := range classified.Confirm {
		action := ActionRename
		switch {
		case opts.Force:
			action = ActionRename
		case opts.SkipAll:
			action = ActionSkip
		default:
			chosen, err := prompter(item)
			if err != nil {
				result.Errors = append(result.Errors, fmt.Sprintf("%s：确认失败，已跳过（%v）", describeItem(item), err))
				action = ActionSkip
			} else {
				action = chosen
			}
		}
		if action == ActionSkip || action == ActionViewDiff {
			result.Skipped++
			continue
		}
		kind, err := executeConfirmedItem(cwd, item, hashes, action == ActionBackupRename, opts.Templates)
		if err != nil {
			result.Errors = append(result.Errors, fmt.Sprintf("%s：%v", describeItem(item), err))
			continue
		}
		hashesChanged = true
		switch kind {
		case countRenamed:
			result.Renamed++
		case countDeleted:
			result.Deleted++
		}
	}

	result.Skipped += len(classified.Skip)
	if hashesChanged {
		if err := SaveHashes(cwd, hashes); err != nil {
			result.Errors = append(result.Errors, fmt.Sprintf("保存模板哈希清单失败：%v", err))
		}
	}
	if len(result.Errors) > 0 {
		return result, fmt.Errorf("迁移执行中有 %d 项失败：%s", len(result.Errors), strings.Join(result.Errors, "；"))
	}
	return result, nil
}

// countKind 是单条迁移项的执行结果计数种类。
type countKind string

const (
	countRenamed countKind = "renamed"
	countDeleted countKind = "deleted"
)

// executeAutoItem 执行一条自动迁移项，并同步更新内存中的哈希表。
// 返回该项应计入的种类：rename-dir 目标已是新版标准内容时删源目录，计 deleted。
func executeAutoItem(cwd string, item MigrationItem, hashes map[string]string, templates map[string]string) (countKind, error) {
	switch item.Type {
	case MigrationTypeRename:
		oldPath := filepath.Join(cwd, filepath.FromSlash(item.From))
		newPath := filepath.Join(cwd, filepath.FromSlash(item.To))
		if err := os.MkdirAll(filepath.Dir(newPath), 0o755); err != nil {
			return "", err
		}
		// 目标已存在且内容是模板：分类时已判定安全，先删目标再搬。
		if _, err := os.Stat(newPath); err == nil {
			if err := os.Remove(newPath); err != nil {
				return "", err
			}
		}
		if err := os.Rename(oldPath, newPath); err != nil {
			return "", err
		}
		renameHash(hashes, item.From, item.To)
		if strings.HasSuffix(item.To, ".sh") || strings.HasSuffix(item.To, ".py") {
			_ = os.Chmod(newPath, 0o755)
		}
		CleanupEmptyDirs(cwd, filepath.Dir(item.From))
		return countRenamed, nil
	case MigrationTypeRenameDir:
		deleted, err := executeRenameDir(cwd, item, hashes, templates)
		if err != nil {
			return "", err
		}
		if deleted {
			return countDeleted, nil
		}
		return countRenamed, nil
	case MigrationTypeDelete:
		if err := os.Remove(filepath.Join(cwd, filepath.FromSlash(item.From))); err != nil {
			return "", err
		}
		delete(hashes, filepath.ToSlash(item.From))
		CleanupEmptyDirs(cwd, filepath.Dir(item.From))
		return countDeleted, nil
	default:
		return "", fmt.Errorf("不支持自动执行的迁移类型 %q", item.Type)
	}
}

// executeConfirmedItem 执行一条经确认的迁移项；backup 为真时先写 .backup。
func executeConfirmedItem(cwd string, item MigrationItem, hashes map[string]string, backup bool, templates map[string]string) (countKind, error) {
	switch item.Type {
	case MigrationTypeRename:
		oldPath := filepath.Join(cwd, filepath.FromSlash(item.From))
		newPath := filepath.Join(cwd, filepath.FromSlash(item.To))
		if err := os.MkdirAll(filepath.Dir(newPath), 0o755); err != nil {
			return "", err
		}
		if backup {
			if err := copyFile(oldPath, newPath+".backup"); err != nil {
				return "", fmt.Errorf("写备份失败：%w", err)
			}
		}
		if err := os.Rename(oldPath, newPath); err != nil {
			return "", err
		}
		renameHash(hashes, item.From, item.To)
		if strings.HasSuffix(item.To, ".sh") || strings.HasSuffix(item.To, ".py") {
			_ = os.Chmod(newPath, 0o755)
		}
		CleanupEmptyDirs(cwd, filepath.Dir(item.From))
		return countRenamed, nil
	case MigrationTypeRenameDir:
		if backup {
			// 目录备份：把源目录整份复制到目标路径加 .backup 后缀。
			if err := copyDir(filepath.Join(cwd, filepath.FromSlash(item.From)), filepath.Join(cwd, filepath.FromSlash(item.To))+".backup"); err != nil {
				return "", fmt.Errorf("写备份失败：%w", err)
			}
		}
		deleted, err := executeRenameDir(cwd, item, hashes, templates)
		if err != nil {
			return "", err
		}
		if deleted {
			return countDeleted, nil
		}
		return countRenamed, nil
	case MigrationTypeDelete:
		filePath := filepath.Join(cwd, filepath.FromSlash(item.From))
		if backup {
			if err := copyFile(filePath, filePath+".backup"); err != nil {
				return "", fmt.Errorf("写备份失败：%w", err)
			}
		}
		if err := os.Remove(filePath); err != nil {
			return "", err
		}
		delete(hashes, filepath.ToSlash(item.From))
		CleanupEmptyDirs(cwd, filepath.Dir(item.From))
		return countDeleted, nil
	default:
		return "", fmt.Errorf("不支持的迁移类型 %q", item.Type)
	}
}

// executeRenameDir 执行 rename-dir：目标已存在且已是当前模板内容时，
// 直接删除冗余的源目录（不得用旧内容覆盖新版标准内容），返回 deleted=true；
// 目标已存在但只含未改动模板文件时，先删目标再搬；否则直接搬。
// 哈希键批量改前缀。
func executeRenameDir(cwd string, item MigrationItem, hashes map[string]string, templates map[string]string) (deleted bool, err error) {
	oldPath := filepath.Join(cwd, filepath.FromSlash(item.From))
	newPath := filepath.Join(cwd, filepath.FromSlash(item.To))
	oldPrefix := strings.TrimSuffix(filepath.ToSlash(item.From), "/") + "/"
	newPrefix := strings.TrimSuffix(filepath.ToSlash(item.To), "/") + "/"
	if _, err := os.Stat(newPath); err == nil {
		if DirMatchesCurrentTemplates(cwd, item.To, templates) {
			if err := os.RemoveAll(oldPath); err != nil {
				return false, err
			}
			for key := range hashes {
				if key == filepath.ToSlash(item.From) || strings.HasPrefix(key, oldPrefix) {
					delete(hashes, key)
				}
			}
			return true, nil
		}
		if err := os.RemoveAll(newPath); err != nil {
			return false, err
		}
	}
	if err := os.MkdirAll(filepath.Dir(newPath), 0o755); err != nil {
		return false, err
	}
	if err := os.Rename(oldPath, newPath); err != nil {
		return false, err
	}
	for key, value := range hashes {
		if key == filepath.ToSlash(item.From) {
			hashes[filepath.ToSlash(item.To)] = value
			delete(hashes, key)
			continue
		}
		if strings.HasPrefix(key, oldPrefix) {
			hashes[newPrefix+strings.TrimPrefix(key, oldPrefix)] = value
			delete(hashes, key)
		}
	}
	return false, nil
}

func renameHash(hashes map[string]string, from, to string) {
	from = filepath.ToSlash(from)
	to = filepath.ToSlash(to)
	if value, ok := hashes[from]; ok {
		hashes[to] = value
		delete(hashes, from)
	}
}

func copyFile(src, dst string) error {
	data, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	return os.WriteFile(dst, data, 0o644)
}

func copyDir(src, dst string) error {
	return filepath.WalkDir(src, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		target := filepath.Join(dst, rel)
		if d.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		return os.WriteFile(target, data, 0o644)
	})
}

func describeItem(item MigrationItem) string {
	if item.To != "" {
		return item.From + " → " + item.To
	}
	return item.From
}

// PrintMigrationSummary 向 w 打印分类汇总（中文）。
func PrintMigrationSummary(classified ClassifiedMigrations, w io.Writer) {
	if classified.Total() == 0 {
		fmt.Fprintln(w, "  无需迁移。")
		return
	}
	for _, item := range classified.Auto {
		fmt.Fprintf(w, "  ✓ 自动迁移（未改动）：%s\n", describeItem(item))
	}
	for _, item := range classified.Confirm {
		fmt.Fprintf(w, "  ⚠ 需确认（已被修改）：%s\n", describeItem(item))
	}
	for _, item := range classified.Conflict {
		fmt.Fprintf(w, "  ⊘ 冲突（新旧文件都存在）：%s\n", describeItem(item))
	}
	fmt.Fprintln(w, "    → 请手工合并或删除其中一个，再重新运行更新。")
	for _, item := range classified.Skip {
		fmt.Fprintf(w, "  ○ 跳过（不存在、受保护或不属于模板）：%s\n", item.From)
	}
}

// PrintMigrationResult 向 w 打印执行结果汇总（中文）。
func PrintMigrationResult(result MigrationResult, w io.Writer) {
	var parts []string
	if result.Renamed > 0 {
		parts = append(parts, fmt.Sprintf("重命名 %d 个", result.Renamed))
	}
	if result.Deleted > 0 {
		parts = append(parts, fmt.Sprintf("删除 %d 个", result.Deleted))
	}
	if result.Skipped > 0 {
		parts = append(parts, fmt.Sprintf("跳过 %d 个", result.Skipped))
	}
	if result.Conflicts > 0 {
		parts = append(parts, fmt.Sprintf("冲突 %d 个", result.Conflicts))
	}
	if len(parts) > 0 {
		fmt.Fprintf(w, "迁移完成：%s\n", strings.Join(parts, "，"))
	}
}
