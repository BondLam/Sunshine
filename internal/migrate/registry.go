package migrate

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// Registry 是迁移清单集：按版本号索引的 MigrationManifest 集合。
//
// Sunshine 自己的清单从空开始（NewRegistry）：当前版本尚无历史迁移，
// 后续每次模板结构变更由该版本补一份清单。清单目录见 LoadManifestDir。
type Registry struct {
	manifests map[string]MigrationManifest
}

// NewRegistry 返回空的清单集。
func NewRegistry() *Registry {
	return &Registry{manifests: map[string]MigrationManifest{}}
}

// Add 登记一份清单（同版本后者覆盖前者）。清单无版本号时返回错误。
func (r *Registry) Add(manifest MigrationManifest) error {
	if manifest.Version == "" {
		return fmt.Errorf("迁移清单缺少 version 字段")
	}
	if r.manifests == nil {
		r.manifests = map[string]MigrationManifest{}
	}
	r.manifests[manifest.Version] = manifest
	return nil
}

// LoadManifestDir 从目录读取全部 *.json 清单文件。
// 单个文件损坏或缺少 version 时跳过该文件并记入 warnings（不中断加载）；
// 目录不存在时返回空清单集（不报错）。
func LoadManifestDir(dir string) (*Registry, []error) {
	return LoadManifestFS(os.DirFS(dir), ".")
}

// LoadManifestFS 从任意 fs.FS 读取全部 *.json 清单文件（LoadManifestDir
// 与内嵌清单共用同一份逻辑）。单个文件损坏或缺少 version 时跳过该文件
// 并记入 warnings（不中断加载）；目录不存在时返回空清单集（不报错）。
func LoadManifestFS(fsys fs.FS, dir string) (*Registry, []error) {
	registry := NewRegistry()
	entries, err := fs.ReadDir(fsys, dir)
	if err != nil {
		if os.IsNotExist(err) {
			return registry, nil
		}
		return registry, []error{fmt.Errorf("读取迁移清单目录失败：%w", err)}
	}
	var warnings []error
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		path := filepath.Join(dir, entry.Name())
		data, err := fs.ReadFile(fsys, path)
		if err != nil {
			warnings = append(warnings, fmt.Errorf("读取迁移清单 %s 失败：%w", entry.Name(), err))
			continue
		}
		var manifest MigrationManifest
		if err := json.Unmarshal(data, &manifest); err != nil {
			warnings = append(warnings, fmt.Errorf("解析迁移清单 %s 失败：%w", entry.Name(), err))
			continue
		}
		if manifest.Version == "" {
			warnings = append(warnings, fmt.Errorf("迁移清单 %s 缺少 version 字段，已跳过", entry.Name()))
			continue
		}
		registry.manifests[manifest.Version] = manifest
	}
	return registry, warnings
}

// Versions 返回全部清单版本号（按版本号升序）。
func (r *Registry) Versions() []string {
	versions := make([]string, 0, len(r.manifests))
	for v := range r.manifests {
		versions = append(versions, v)
	}
	sort.Slice(versions, func(i, j int) bool { return CompareVersions(versions[i], versions[j]) < 0 })
	return versions
}

// applicableVersions 返回区间 (from, to] 内的清单版本号（升序）。
func (r *Registry) applicableVersions(fromVersion, toVersion string) []string {
	var applicable []string
	for _, v := range r.Versions() {
		if CompareVersions(v, fromVersion) > 0 && CompareVersions(v, toVersion) <= 0 {
			applicable = append(applicable, v)
		}
	}
	return applicable
}

// MigrationsForVersion 返回从 fromVersion 升级到 toVersion 需执行的全部
// 迁移项（区间 (from, to]，按版本号升序）。
func (r *Registry) MigrationsForVersion(fromVersion, toVersion string) []MigrationItem {
	var items []MigrationItem
	for _, v := range r.applicableVersions(fromVersion, toVersion) {
		items = append(items, r.manifests[v].Migrations...)
	}
	return items
}

// AllMigrations 返回全部清单的迁移项（按版本号升序），用于孤儿迁移检测
// 与 safe-file-delete 收集（哈希匹配本身就是安全网，与版本无关）。
func (r *Registry) AllMigrations() []MigrationItem {
	var items []MigrationItem
	for _, v := range r.Versions() {
		items = append(items, r.manifests[v].Migrations...)
	}
	return items
}

// ConfigSectionsAddedBetween 返回区间 (from, to] 内各清单新引入的配置段落。
func (r *Registry) ConfigSectionsAddedBetween(fromVersion, toVersion string) []ConfigSectionAdded {
	var sections []ConfigSectionAdded
	for _, v := range r.applicableVersions(fromVersion, toVersion) {
		sections = append(sections, r.manifests[v].ConfigSectionsAdded...)
	}
	return sections
}

// Metadata 返回区间 (from, to] 内各清单的汇总元数据。
func (r *Registry) Metadata(fromVersion, toVersion string) MigrationMetadata {
	meta := MigrationMetadata{}
	for _, v := range r.applicableVersions(fromVersion, toVersion) {
		manifest := r.manifests[v]
		if manifest.Changelog != "" {
			meta.Changelog = append(meta.Changelog, "v"+v+": "+manifest.Changelog)
		}
		if manifest.Breaking {
			meta.Breaking = true
		}
		if manifest.RecommendMigrate {
			meta.RecommendMigrate = true
		}
		if manifest.MigrationGuide != "" {
			meta.Guides = append(meta.Guides, MigrationGuide{
				Version:        v,
				Guide:          manifest.MigrationGuide,
				AIInstructions: manifest.AIInstructions,
			})
		}
	}
	return meta
}

// HasPendingMigrations 判断区间内是否有待执行的迁移项。
func (r *Registry) HasPendingMigrations(fromVersion, toVersion string) bool {
	return len(r.MigrationsForVersion(fromVersion, toVersion)) > 0
}

// FindOrphanMigrations 找出孤儿迁移：源存在且目标不存在的 rename /
// rename-dir 项，且不在 pending 之内。用于处理“版本号已更新但迁移没
// 执行完”的半迁移状态。
func FindOrphanMigrations(cwd string, all, pending []MigrationItem) []MigrationItem {
	inPending := make(map[string]bool, len(pending))
	for _, m := range pending {
		inPending[m.From+"\x00"+m.To] = true
	}
	var orphans []MigrationItem
	for _, item := range all {
		if item.Type != MigrationTypeRename && item.Type != MigrationTypeRenameDir {
			continue
		}
		if item.From == "" || item.To == "" {
			continue
		}
		if inPending[item.From+"\x00"+item.To] {
			continue
		}
		if _, err := os.Stat(filepath.Join(cwd, filepath.FromSlash(item.From))); err != nil {
			continue
		}
		if _, err := os.Stat(filepath.Join(cwd, filepath.FromSlash(item.To))); err == nil {
			continue
		}
		orphans = append(orphans, item)
	}
	return orphans
}
