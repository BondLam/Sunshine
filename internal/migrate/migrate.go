// Package migrate 实现 sunshine update 的迁移系统。
//
// 迁移系统由三部分组成：
//
//   - 引擎：按项目版→CLI 版区间筛选适用清单（Registry），按文件状态把
//     迁移项分类（ClassifyMigrations），按安全顺序执行（ExecuteMigrations），
//     以及无需 --migrate 也能自动执行的 safe-file-delete 清理。
//   - 清单格式：MigrationManifest / MigrationItem / ConfigSectionAdded，
//     与上游清单格式一致的 JSON（字段名见各结构体的 json 标签）。
//   - 清单集：Sunshine 自己的版本线起步，第一份真实清单是 0.2.0
//     （internal/migrate/manifests/0.2.0.json，蓝本 Trellis
//     0.5.0-beta.0 的 Skill-first 迁移）；后续每次模板结构变更，由该
//     版本补一份 manifests/<版本号>.json，格式见
//     internal/migrate/manifests/README.md 的“新增清单”一节。
//
// 执行规则（供 update 调用）：只有 --migrate 才执行 rename / rename-dir /
// delete 类迁移；safe-file-delete 在内容哈希命中 allowed_hashes 时无需
// --migrate 也自动执行；无 --migrate 且有破坏性待迁移项时，update 应硬停
// 并提示 `sunshine update --migrate`（判定见 ShouldHardStop）。
package migrate

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// MigrationItem 是一条迁移动作。
type MigrationItem struct {
	// Type 是迁移类型：rename | rename-dir | delete | safe-file-delete。
	Type string `json:"type"`
	// From 是源路径（相对项目根）。
	From string `json:"from"`
	// To 是目标路径（rename / rename-dir 用，相对项目根）。
	To string `json:"to,omitempty"`
	// Description 说明这条迁移做什么。
	Description string `json:"description,omitempty"`
	// Reason 说明为何要向用户确认这条迁移（版本相关的特殊背景，1-2 句）。
	Reason string `json:"reason,omitempty"`
	// AllowedHashes 是 safe-file-delete 的已知模板内容哈希：只有文件内容
	// 哈希命中其一时才删除。
	AllowedHashes []string `json:"allowed_hashes,omitempty"`
}

// MigrationTypeRename 等是 MigrationItem.Type 的取值。
const (
	MigrationTypeRename         = "rename"
	MigrationTypeRenameDir      = "rename-dir"
	MigrationTypeDelete         = "delete"
	MigrationTypeSafeFileDelete = "safe-file-delete"
)

// Validate 检查迁移项的基本合法性：类型已知，且 rename / rename-dir 有目标路径。
func (m MigrationItem) Validate() error {
	switch m.Type {
	case MigrationTypeRename, MigrationTypeRenameDir:
		if m.To == "" {
			return fmt.Errorf("迁移项 %s 缺少目标路径 to", m.From)
		}
	case MigrationTypeDelete, MigrationTypeSafeFileDelete:
	default:
		return fmt.Errorf("未知迁移类型 %q（from=%s）", m.Type, m.From)
	}
	if m.From == "" {
		return fmt.Errorf("迁移项缺少源路径 from")
	}
	return nil
}

// ConfigSectionAdded 是某版本新引入的顶层配置段落。
//
// update 用它把新段落追加到用户已有的配置文件（如 .sunshine/config.yaml），
// 而不覆盖用户的其他定制。追加前先看文件是否已含 Sentinel（含则视为已有，
// 不追加）；段落正文从新版模板里按 SectionHeading 提取。
type ConfigSectionAdded struct {
	// File 是目标文件（相对项目根，如 .sunshine/config.yaml）。
	File string `json:"file"`
	// Sentinel 是判定段落是否已存在的子串（取稳定的新顶层键，如 "codex:"）。
	Sentinel string `json:"sentinel"`
	// SectionHeading 是新版模板分隔块里的 `# <标题>` 文本。
	SectionHeading string `json:"sectionHeading"`
}

// MigrationManifest 是一份版本迁移清单（manifests/<版本号>.json）。
type MigrationManifest struct {
	// Version 是这份清单对应的目标版本。
	Version string `json:"version"`
	// Description 是本版改动的简述。
	Description string `json:"description,omitempty"`
	// Migrations 是本版的迁移动作列表。
	Migrations []MigrationItem `json:"migrations"`
	// Changelog 是展示给用户的详细变更记录。
	Changelog string `json:"changelog,omitempty"`
	// Breaking 表示本版含破坏性变更。
	Breaking bool `json:"breaking,omitempty"`
	// RecommendMigrate 表示建议用户用 --migrate 执行迁移（破坏性版本常用）。
	RecommendMigrate bool `json:"recommendMigrate,omitempty"`
	// MigrationGuide 是详细的迁移指南（Markdown）。
	MigrationGuide string `json:"migrationGuide,omitempty"`
	// AIInstructions 是给 AI 助手协助迁移的说明。
	AIInstructions string `json:"aiInstructions,omitempty"`
	// ConfigSectionsAdded 是本版新引入的配置段落。
	ConfigSectionsAdded []ConfigSectionAdded `json:"configSectionsAdded,omitempty"`
	// Notes 是其他备注。
	Notes string `json:"notes,omitempty"`
}

// MigrationGuide 是某版本的迁移指南（汇总用）。
type MigrationGuide struct {
	Version        string
	Guide          string
	AIInstructions string
}

// MigrationMetadata 是区间内各清单的汇总元数据。
type MigrationMetadata struct {
	// Changelog 是各版 changelog 的汇总（每条前缀 "v<版本>: "）。
	Changelog []string
	// Breaking 任一适用清单含破坏性变更时为真。
	Breaking bool
	// RecommendMigrate 任一适用清单建议 --migrate 时为真。
	RecommendMigrate bool
	// Guides 是各版的迁移指南。
	Guides []MigrationGuide
}

// Classification 是迁移项的分类结果。
type Classification string

const (
	// ClassificationAuto 未被用户改动，可自动迁移。
	ClassificationAuto Classification = "auto"
	// ClassificationConfirm 已被用户改动，需交互确认。
	ClassificationConfirm Classification = "confirm"
	// ClassificationConflict 新旧文件都存在且新文件含用户内容，需手工处理。
	ClassificationConflict Classification = "conflict"
	// ClassificationSkip 源文件不存在、受保护或不属于模板，无需处理。
	ClassificationSkip Classification = "skip"
)

// ClassifiedMigrations 是 ClassifyMigrations 的分类结果。
type ClassifiedMigrations struct {
	Auto     []MigrationItem
	Confirm  []MigrationItem
	Conflict []MigrationItem
	Skip     []MigrationItem
}

// PendingCount 返回待处理（auto + confirm + conflict）的迁移项数。
func (c ClassifiedMigrations) PendingCount() int {
	return len(c.Auto) + len(c.Confirm) + len(c.Conflict)
}

// Total 返回全部已分类的迁移项数。
func (c ClassifiedMigrations) Total() int {
	return c.PendingCount() + len(c.Skip)
}

// MigrationAction 是用户对一条需确认迁移项的选择。
type MigrationAction string

const (
	// ActionRename 直接执行（重命名/删除）。
	ActionRename MigrationAction = "rename"
	// ActionBackupRename 先写一份 .backup 备份，再执行（默认，最安全）。
	ActionBackupRename MigrationAction = "backup-rename"
	// ActionSkip 跳过这条迁移。
	ActionSkip MigrationAction = "skip"
	// ActionViewDiff 先看差异再决定（交互提示暂不提供该选项，保留给后续扩展）。
	ActionViewDiff MigrationAction = "view-diff"
)

// MigrationResult 是 ExecuteMigrations 的执行结果汇总。
type MigrationResult struct {
	// Renamed 是已重命名的文件/目录数。
	Renamed int
	// Deleted 是已删除的文件/目录数。
	Deleted int
	// Skipped 是跳过的迁移项数（用户选择跳过 + 无需处理）。
	Skipped int
	// Conflicts 是冲突项数（新旧文件都存在，需手工处理）。
	Conflicts int
	// Errors 是执行中单个迁移项的失败原因（有内容时 ExecuteMigrations
	// 同时返回 error；其余迁移项仍会继续执行）。
	Errors []string
}

// ProtectedPaths 是永不迁移、不删除的用户数据路径（相对项目根）。
// 对应上游的 PROTECTED_PATHS，路径换成 Sunshine 的 .sunshine/ 布局。
var ProtectedPaths = []string{
	".sunshine/workspace",
	".sunshine/tasks",
	".sunshine/spec",
	".sunshine/.developer",
	".sunshine/.current-task",
}

// IsProtectedPath 判断路径是否在受保护范围内（相等或在其目录下）。
func IsProtectedPath(path string) bool {
	path = filepath.ToSlash(path)
	for _, protected := range ProtectedPaths {
		if path == protected || strings.HasPrefix(path, protected+"/") {
			return true
		}
	}
	return false
}

// IsUpdateSkipped 判断路径是否被 update.skip 命中（相等或在其目录下）。
func IsUpdateSkipped(path string, skipPaths []string) bool {
	path = filepath.ToSlash(path)
	for _, skip := range skipPaths {
		skip = filepath.ToSlash(strings.TrimSpace(skip))
		if skip == "" {
			continue
		}
		if path == skip || strings.HasPrefix(path, skip+"/") {
			return true
		}
	}
	return false
}

// ComputeHash 计算内容的 SHA-256（十六进制）。
//
// 注意与上游的差异：上游 computeHash 会先把 CRLF 统一成 LF 再算哈希；
// Sunshine 的 .template-hashes.json 由 scaffold 以原始字节算哈希（见
// internal/scaffold），为与清单里已存的哈希一致，迁移系统用原始字节
// 算哈希，不做换行符归一化。
func ComputeHash(content []byte) string {
	sum := sha256.Sum256(content)
	return hex.EncodeToString(sum[:])
}

// ComputeFileHash 计算文件的 SHA-256（十六进制）；文件不存在时返回错误。
func ComputeFileHash(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	return ComputeHash(data), nil
}

// IsTemplateModified 判断文件是否被用户改动过：与哈希清单里的记录不一致
// 即为已改动；清单里没有记录时按保守策略视为已改动。
func IsTemplateModified(cwd, rel string, hashes map[string]string) bool {
	full := filepath.Join(cwd, filepath.FromSlash(rel))
	if _, err := os.Stat(full); err != nil {
		return false
	}
	stored, ok := hashes[filepath.ToSlash(rel)]
	if !ok || stored == "" {
		return true
	}
	current, err := ComputeFileHash(full)
	if err != nil {
		return true
	}
	return current != stored
}

// hashManifest 是 .sunshine/.template-hashes.json 的磁盘格式
// （{"__version": 2, "files": {...}}，与 scaffold 写入的一致）。
type hashManifest struct {
	Version int               `json:"__version"`
	Files   map[string]string `json:"files"`
}

func hashManifestPath(cwd string) string {
	return filepath.Join(cwd, ".sunshine", ".template-hashes.json")
}

// LoadHashes 读 .sunshine/.template-hashes.json；清单不存在时返回空表。
func LoadHashes(cwd string) (map[string]string, error) {
	data, err := os.ReadFile(hashManifestPath(cwd))
	if err != nil {
		if os.IsNotExist(err) {
			return map[string]string{}, nil
		}
		return nil, fmt.Errorf("读取模板哈希清单失败：%w", err)
	}
	var manifest hashManifest
	if err := json.Unmarshal(data, &manifest); err != nil {
		return nil, fmt.Errorf("解析模板哈希清单失败：%w", err)
	}
	if manifest.Files == nil {
		return map[string]string{}, nil
	}
	return manifest.Files, nil
}

// SaveHashes 写 .sunshine/.template-hashes.json（保留 __version: 2 格式）。
func SaveHashes(cwd string, hashes map[string]string) error {
	if hashes == nil {
		hashes = map[string]string{}
	}
	manifest, err := json.MarshalIndent(hashManifest{Version: 2, Files: hashes}, "", "  ")
	if err != nil {
		return err
	}
	if err := os.WriteFile(hashManifestPath(cwd), append(manifest, '\n'), 0o644); err != nil {
		return fmt.Errorf("写入模板哈希清单失败：%w", err)
	}
	return nil
}

// DirHasManifestEntries 判断哈希清单是否记录了目录下的任何文件，即该目录
// 是否由模板创建（用于 rename-dir 的归属判定：模板没写过的目录不得自动搬走）。
func DirHasManifestEntries(dirRel string, hashes map[string]string) bool {
	dirRel = filepath.ToSlash(dirRel)
	prefix := strings.TrimSuffix(dirRel, "/") + "/"
	for key := range hashes {
		if key == dirRel || strings.HasPrefix(key, prefix) {
			return true
		}
	}
	return false
}

// CleanupEmptyDirs 从 dir 开始向上清理空目录，到项目根为止（不删项目根本身）。
func CleanupEmptyDirs(cwd, dir string) {
	absRoot, err := filepath.Abs(cwd)
	if err != nil {
		return
	}
	absDir, err := filepath.Abs(dir)
	if err != nil {
		return
	}
	for {
		if absDir == absRoot || !strings.HasPrefix(absDir, absRoot+string(os.PathSeparator)) {
			return
		}
		entries, err := os.ReadDir(absDir)
		if err != nil || len(entries) > 0 {
			return
		}
		if err := os.Remove(absDir); err != nil {
			return
		}
		absDir = filepath.Dir(absDir)
	}
}
