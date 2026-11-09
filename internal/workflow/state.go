package workflow

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/linrong1982/Sunshine/internal/scaffold"
)

// WorkflowRelPath 是 .sunshine/workflow.md 相对项目根的路径，
// 也是它在 .template-hashes.json 里的键（与 init 的约定一致）。
const WorkflowRelPath = ".sunshine/workflow.md"

// Classification 是现有 workflow.md 相对新模板内容的状态。
type Classification int

const (
	// ClassMissing 磁盘上没有 workflow.md。
	ClassMissing Classification = iota
	// ClassIdentical 磁盘内容与新模板逐字节相同。
	ClassIdentical
	// ClassPristine 磁盘内容与哈希清单记录一致（未改动）。
	ClassPristine
	// ClassModified 哈希清单有记录但磁盘内容已改动，或清单无记录
	// （无法判断来源）。与上游 classifyExistingWorkflow 一致：上游
	// 只有四态，无记录即按 modified 走交互确认，不自动写 .new。
	ClassModified
)

// loadHashes 读 .sunshine/.template-hashes.json 的 files 表。
// 清单不存在时返回空表；清单损坏时报错。
func loadHashes(cwd string) (map[string]string, error) {
	path := filepath.Join(cwd, ".sunshine", ".template-hashes.json")
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return map[string]string{}, nil
		}
		return nil, fmt.Errorf("读取模板哈希清单失败：%w", err)
	}
	var manifest struct {
		Version int               `json:"__version"`
		Files   map[string]string `json:"files"`
	}
	if err := json.Unmarshal(data, &manifest); err != nil {
		return nil, fmt.Errorf("读取模板哈希清单失败：%w", err)
	}
	if manifest.Files == nil {
		return map[string]string{}, nil
	}
	return manifest.Files, nil
}

// writeHashes 把 files 表写回清单（保留 __version: 2 与其余条目）。
func writeHashes(cwd string, files map[string]string) error {
	path := filepath.Join(cwd, ".sunshine", ".template-hashes.json")
	manifest, err := json.MarshalIndent(map[string]any{
		"__version": 2,
		"files":     files,
	}, "", "  ")
	if err != nil {
		return err
	}
	if err := os.WriteFile(path, append(manifest, '\n'), 0o644); err != nil {
		return fmt.Errorf("写入模板哈希清单失败：%w", err)
	}
	return nil
}

// removeHashEntry 从清单里删除指定键（不存在时不做任何事）。
// 清单本身不存在或损坏时返回错误（损坏不应被悄悄覆盖）。
func removeHashEntry(cwd, rel string) error {
	path := filepath.Join(cwd, ".sunshine", ".template-hashes.json")
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return nil
	}
	hashes, err := loadHashes(cwd)
	if err != nil {
		return err
	}
	if _, ok := hashes[rel]; !ok {
		return nil
	}
	delete(hashes, rel)
	return writeHashes(cwd, hashes)
}

// computeHash 算一段内容的 SHA-256（与模板哈希清单的算法一致）。
func computeHash(content string) string {
	sum := sha256.Sum256([]byte(content))
	return hex.EncodeToString(sum[:])
}

// Classify 判断现有 workflow.md 相对新模板内容的状态。
func Classify(cwd, newContent string) (Classification, error) {
	current, err := os.ReadFile(filepath.Join(cwd, WorkflowRelPath))
	if err != nil {
		if os.IsNotExist(err) {
			return ClassMissing, nil
		}
		return ClassMissing, fmt.Errorf("读取 %s 失败：%w", WorkflowRelPath, err)
	}
	if string(current) == newContent {
		return ClassIdentical, nil
	}
	hashes, err := loadHashes(cwd)
	if err != nil {
		return ClassModified, err
	}
	stored, ok := hashes[WorkflowRelPath]
	if !ok {
		// 清单无记录：按上游当作已改动（走交互确认），不自动写 .new。
		return ClassModified, nil
	}
	if stored == computeHash(string(current)) {
		return ClassPristine, nil
	}
	return ClassModified, nil
}

// ApplyHashContract 按上游 workflow.ts 的哈希契约登记写盘结果，
// 与 init / 平台安装共用同一份 .template-hashes.json：
//
//   - native 模板：刷新 workflow.md 的哈希条目（update 仍视其为模板管理文件）；
//   - 非 native 模板：删除该条目（用户自选内容，update 不得悄悄恢复原生内容）；
//   - --create-new：不调用本函数，清单原样不动。
func ApplyHashContract(cwd, templateID string) error {
	if templateID == NativeID {
		// 复用 scaffold.RecordHashes：同一清单、同一格式、同一键。
		return scaffold.RecordHashes(cwd, []string{WorkflowRelPath})
	}
	return removeHashEntry(cwd, WorkflowRelPath)
}
