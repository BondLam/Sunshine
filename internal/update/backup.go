package update

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// CreateBackupDirPath 生成带时间戳的备份目录路径
// （上游 createBackupDirPath，按任务要求改为项目内
// .sunshine/.backups/<时间戳>/；时间戳格式与上游一致，
// 用连字符代替冒号与小数点，取前 19 位）。
func CreateBackupDirPath(cwd string) string {
	timestamp := time.Now().UTC().Format("2006-01-02T15-04-05")
	return filepath.Join(cwd, ".sunshine", ".backups", timestamp)
}

// CollectAllFiles 递归收集目录下全部文件（不跟随符号链接，
// 避免符号链接指向祖先目录时无限循环；上游 collectAllFiles）。
// 目录不存在时返回空列表。
func CollectAllFiles(dirPath, cwd string) []string {
	if _, err := os.Stat(dirPath); err != nil {
		return nil
	}
	var files []string
	stack := []string{dirPath}
	for len(stack) > 0 {
		current := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		entries, err := os.ReadDir(current)
		if err != nil {
			continue
		}
		for _, entry := range entries {
			fullPath := filepath.Join(current, entry.Name())
			rel, err := filepath.Rel(cwd, fullPath)
			if err != nil {
				continue
			}
			if entry.Type()&fs.ModeSymlink != 0 {
				continue
			}
			if entry.IsDir() {
				if !ShouldExcludeFromBackup(rel) {
					stack = append(stack, fullPath)
				}
				continue
			}
			if entry.Type().IsRegular() {
				files = append(files, fullPath)
			}
		}
	}
	return files
}

func backupFile(cwd, backupDir, rel string) error {
	src := filepath.Join(cwd, filepath.FromSlash(rel))
	if _, err := os.Stat(src); err != nil {
		return nil
	}
	dst := filepath.Join(backupDir, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return fmt.Errorf("创建备份目录失败：%w", err)
	}
	data, err := os.ReadFile(src)
	if err != nil {
		return fmt.Errorf("读取 %s 失败：%w", rel, err)
	}
	if err := os.WriteFile(dst, data, 0o644); err != nil {
		return fmt.Errorf("备份 %s 失败：%w", rel, err)
	}
	return nil
}

// CreateFullBackup 在更新前对全部托管目录做完整快照
// （上游 createFullBackup）：备份 BACKUP_DIRS 下的全部文件
// （shouldExcludeFromBackup 排除用户数据）与 BACKUP_FILES。
// 没有任何文件可备份时返回 ""，不创建备份目录。
// 返回备份目录的绝对路径。
func CreateFullBackup(cwd string) (string, error) {
	backupDir := CreateBackupDirPath(cwd)
	hasFiles := false
	ensureDir := func() error {
		if hasFiles {
			return nil
		}
		if err := os.MkdirAll(backupDir, 0o755); err != nil {
			return fmt.Errorf("创建备份目录失败：%w", err)
		}
		hasFiles = true
		return nil
	}
	for _, dir := range BackupDirs {
		dirPath := filepath.Join(cwd, filepath.FromSlash(dir))
		if _, err := os.Stat(dirPath); err != nil {
			continue
		}
		for _, fullPath := range CollectAllFiles(dirPath, cwd) {
			rel, err := filepath.Rel(cwd, fullPath)
			if err != nil {
				continue
			}
			if ShouldExcludeFromBackup(rel) {
				continue
			}
			if err := ensureDir(); err != nil {
				return "", err
			}
			if err := backupFile(cwd, backupDir, rel); err != nil {
				return "", err
			}
		}
	}
	for _, rel := range BackupFiles {
		fullPath := filepath.Join(cwd, filepath.FromSlash(rel))
		if _, err := os.Stat(fullPath); err != nil {
			continue
		}
		if ShouldExcludeFromBackup(rel) {
			continue
		}
		if err := ensureDir(); err != nil {
			return "", err
		}
		if err := backupFile(cwd, backupDir, rel); err != nil {
			return "", err
		}
	}
	if !hasFiles {
		return "", nil
	}
	return backupDir, nil
}

// backupRel 把备份目录的绝对路径转成相对项目根的显示路径。
func backupRel(cwd, backupDir string) string {
	rel, err := filepath.Rel(cwd, backupDir)
	if err != nil {
		return backupDir
	}
	return strings.ReplaceAll(rel, string(filepath.Separator), "/")
}
