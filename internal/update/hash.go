// Package update 实现 sunshine update：模板同步更新（哈希三态 + 交互确认 +
// 备份 + 迁移 + registry 持续刷新）。
//
// 行为对照 Trellis 源码（packages/cli/src/commands/update.ts、
// utils/template-hash.ts、utils/manifest-prune.ts、utils/registry-config.ts，
// commit f089cb3）按需求清单第 4 模块重新实现，不复制源码；与源码不
// 一致处以源码为准，差异记在 ~/workspace/sunshine/PLAN.md 阶段 4 实施记录。
//
// 流程顺序（按 update() 主流程）：版本比对 → Codex 升级修复 → 完整备份
// （先备份再写盘）→ 清单孤儿键清理 → registry.spec 持续刷新 → 变更分析
// → 交互确认 → 写盘 → 迁移执行 → 更新 .version。
package update

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// manifestFileName 是模板哈希清单文件名。
const manifestFileName = ".template-hashes.json"

// manifestSchemaVersion 是清单 schema 版本（与上游一致）。
const manifestSchemaVersion = 2

// ComputeHash 计算内容的 SHA-256（十六进制）。
//
// 注意与上游的差异：上游 computeHash 先把 CRLF 归一化为 LF 再哈希；
// Sunshine 阶段 1 的 scaffold.Init / RecordHashes 直接对原始字节哈希，
// 清单里的值就是原始字节哈希。update 必须与 init 的写法一致，
// 否则每个文件都会被误判为“用户已改动”。因此这里不对换行符做归一化。
func ComputeHash(content []byte) string {
	sum := sha256.Sum256(content)
	return hex.EncodeToString(sum[:])
}

// ComputeHashString 是 ComputeHash 的字符串版。
func ComputeHashString(content string) string {
	return ComputeHash([]byte(content))
}

// manifestDisk 是清单在磁盘上的形态。Sunshine 阶段 1 起用 "files" 键；
// 上游 v2 用 "hashes" 键。读取时两者都接受（兼容旧清单），
// 写入时一律用 Sunshine 的 "files" 键（与 scaffold 一致）。
type manifestDisk struct {
	Version int               `json:"__version"`
	Files   map[string]string `json:"files,omitempty"`
	Hashes  map[string]string `json:"hashes,omitempty"`
}

// LoadHashes 读 cwd/.sunshine/.template-hashes.json。
// 文件缺失、JSON 损坏、无 __version 或版本不是 2 时返回空清单
// （与上游 loadHashes 一致：旧格式静默丢弃，由后续流程重建；
// 这样损坏的清单不会让 update 误判为“全部未改动”）。
// 键统一归一化为 POSIX 形式。
func LoadHashes(cwd string) map[string]string {
	data, err := os.ReadFile(filepath.Join(cwd, ".sunshine", manifestFileName))
	if err != nil {
		return map[string]string{}
	}
	var m manifestDisk
	if err := json.Unmarshal(data, &m); err != nil {
		return map[string]string{}
	}
	if m.Version != manifestSchemaVersion {
		return map[string]string{}
	}
	out := map[string]string{}
	for k, v := range m.Files {
		out[ToPosix(k)] = v
	}
	for k, v := range m.Hashes {
		if _, ok := out[ToPosix(k)]; !ok {
			out[ToPosix(k)] = v
		}
	}
	return out
}

// SaveHashes 把清单写回磁盘（Sunshine 的 "files" 键格式，键排序后写出）。
func SaveHashes(cwd string, hashes map[string]string) error {
	keys := make([]string, 0, len(hashes))
	for k := range hashes {
		keys = append(keys, ToPosix(k))
	}
	sort.Strings(keys)
	files := make(map[string]string, len(hashes))
	for _, k := range keys {
		files[k] = hashes[k]
	}
	payload, err := json.MarshalIndent(map[string]any{
		"__version": manifestSchemaVersion,
		"files":     files,
	}, "", "  ")
	if err != nil {
		return err
	}
	path := filepath.Join(cwd, ".sunshine", manifestFileName)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("创建清单目录失败：%w", err)
	}
	if err := os.WriteFile(path, append(payload, '\n'), 0o644); err != nil {
		return fmt.Errorf("写入模板哈希清单失败：%w", err)
	}
	return nil
}

// UpdateHashes 把 files（相对项目根路径 → 新内容）的哈希并入清单并保存。
// 相对路径统一为 POSIX 形式。
func UpdateHashes(cwd string, files map[string]string) error {
	hashes := LoadHashes(cwd)
	for rel, content := range files {
		hashes[ToPosix(rel)] = ComputeHashString(content)
	}
	return SaveHashes(cwd, hashes)
}

// RemoveHash 删除清单里某文件的条目（文件被安全删除后调用）。
func RemoveHash(cwd, rel string) error {
	hashes := LoadHashes(cwd)
	delete(hashes, ToPosix(rel))
	return SaveHashes(cwd, hashes)
}

// RenameHash 把清单条目从旧路径移到新路径（迁移改名后调用）。
func RenameHash(cwd, oldPath, newPath string) error {
	hashes := LoadHashes(cwd)
	if v, ok := hashes[ToPosix(oldPath)]; ok {
		delete(hashes, ToPosix(oldPath))
		hashes[ToPosix(newPath)] = v
	}
	return SaveHashes(cwd, hashes)
}

// hashExcluded 是 .sunshine/ 内不进哈希清单的条目（与 scaffold 一致，
// 另加备份目录：备份本身不进哈希清单）。
var hashExcluded = map[string]bool{
	".template-hashes.json": true,
	".version":              true,
	".developer":            true,
}

// ShouldExcludeFromHash 判断相对路径（相对项目根或相对 .sunshine/）
// 是否不进哈希清单。规则与上游 shouldExcludeFromHash 对应，
// 按 Sunshine 的路径改写：.sunshine/spec/、tasks/、workspace/、
// .current-task、.backups/（备份目录）与 .backup- 前缀都不追踪。
func ShouldExcludeFromHash(rel string) bool {
	p := ToPosix(rel)
	base := strings.TrimPrefix(p, ".sunshine/")
	// 注意：.sunshine/.gitignore 不排除——scaffold 会哈希它，
	// 它是 init 模板的一部分，update 需要追踪它。
	switch {
	case hashExcluded[base]:
		return true
	case strings.HasPrefix(base, "spec/") || base == "spec":
		return true
	case strings.HasPrefix(base, "tasks/") || base == "tasks":
		return true
	case strings.HasPrefix(base, "workspace/") || base == "workspace":
		return true
	case base == ".current-task":
		return true
	case strings.HasPrefix(base, ".backups/") || base == ".backups":
		return true
	case strings.Contains(base, ".backup-"):
		return true
	}
	return false
}

// ToPosix 把路径分隔符统一为正斜杠（清单键的格式约定）。
func ToPosix(p string) string {
	return filepath.ToSlash(p)
}
