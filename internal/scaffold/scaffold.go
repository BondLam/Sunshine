// Package scaffold 实现 sunshine init：生成 .sunshine/ 工程骨架。
package scaffold

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/linrong1982/Sunshine/internal/projecttype"
	"github.com/linrong1982/Sunshine/templates"
)

// RecordHashes 把 files（相对项目根的路径，如 ".claude/settings.json"）的
// SHA-256 合并进 .sunshine/.template-hashes.json，保留已有条目。
// 清单格式与 Init 写入的一致：{"__version": 2, "files": {...}}。
// 清单不存在时新建一份；任一文件不存在或清单损坏时返回错误。
func RecordHashes(cwd string, files []string) error {
	manifestPath := filepath.Join(cwd, ".sunshine", ".template-hashes.json")
	hashes := make(map[string]string)
	if data, err := os.ReadFile(manifestPath); err == nil {
		var manifest struct {
			Version int               `json:"__version"`
			Files   map[string]string `json:"files"`
		}
		if err := json.Unmarshal(data, &manifest); err != nil {
			return fmt.Errorf("读取模板哈希清单失败：%w", err)
		}
		for k, v := range manifest.Files {
			hashes[k] = v
		}
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("读取模板哈希清单失败：%w", err)
	}
	for _, rel := range files {
		data, err := os.ReadFile(filepath.Join(cwd, filepath.FromSlash(rel)))
		if err != nil {
			return fmt.Errorf("计算 %s 的哈希失败：%w", rel, err)
		}
		sum := sha256.Sum256(data)
		hashes[rel] = hex.EncodeToString(sum[:])
	}
	manifest, err := json.MarshalIndent(map[string]any{
		"__version": 2,
		"files":     hashes,
	}, "", "  ")
	if err != nil {
		return err
	}
	if err := os.WriteFile(manifestPath, append(manifest, '\n'), 0o644); err != nil {
		return fmt.Errorf("写入模板哈希清单失败：%w", err)
	}
	return nil
}

var hashExcluded = map[string]bool{
	".template-hashes.json": true,
	".version":              true,
	".developer":            true,
}

// InitOptions 是 InitWithOptions 的参数。
type InitOptions struct {
	// Force 为真时覆盖已有的骨架。
	Force bool
	// SkipSpec 为真时不写默认 spec 模板（--registry 将从模板市场装 spec，
	// 装完再由调用方记入哈希清单；对应上游 init 的 skipSpecTemplates）。
	SkipSpec bool
	// SpecType 是本次 init 的项目类型：Frontend 只写前端 spec，
	// Backend 只写后端 spec，Fullstack 或 Unknown（零值）三份全写；
	// Design 与 Fullstack 一样三份全写，另写 doc/ 设计文档模板；
	// system.md 与类型无关，永远写。
	SpecType projecttype.Type
	// Version 为空时 .version 沿用模板文件内容；非空时写入该值
	//（CLI 的 init 传 cli.version，保证 .version 与 --version 同源，
	// 不另留一份版本常量）。
	Version string
}

// Init 在 cwd 下生成 .sunshine/ 工程骨架。force 为真时覆盖已有的骨架。
func Init(cwd, user string, force bool) error {
	return InitWithOptions(cwd, user, InitOptions{Force: force})
}

// InitWithOptions 在 cwd 下生成 .sunshine/ 工程骨架。
func InitWithOptions(cwd, user string, opts InitOptions) error {
	home, err := os.UserHomeDir()
	if err == nil && cwd == home && os.Getenv("SUNSHINE_ALLOW_HOMEDIR") != "1" {
		return errors.New("不能在用户主目录执行初始化（设置 SUNSHINE_ALLOW_HOMEDIR=1 可绕过）")
	}
	target := filepath.Join(cwd, ".sunshine")
	if st, err := os.Stat(target); err == nil && st.IsDir() {
		if !opts.Force {
			return errors.New("当前目录已经初始化过（.sunshine/ 已存在），如需重新初始化请使用 --force")
		}
		if err := os.RemoveAll(target); err != nil {
			return fmt.Errorf("清理旧的 .sunshine/ 失败：%w", err)
		}
	}
	hashes := make(map[string]string)
	err = fs.WalkDir(templates.Sunshine, "sunshine", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		rel := strings.TrimPrefix(path, "sunshine/")
		if opts.SkipSpec && strings.HasPrefix(rel, "spec/") {
			return nil
		}
		// 按项目类型跳过不匹配的 spec 文件（跳过的文件不写盘，
		// 也不进入哈希清单）；system.md 与类型无关，永远写。
		// 产品设计先行也按全栈写三份规范（另写 doc/ 模板）。
		if rel == "spec/frontend.md" && opts.SpecType == projecttype.Backend {
			return nil
		}
		if rel == "spec/backend.md" && opts.SpecType == projecttype.Frontend {
			return nil
		}
		data, err := templates.Sunshine.ReadFile(path)
		if err != nil {
			return err
		}
		// .version 与 CLI 版本同源：调用方传了 Version 时以它为准，
		// 不沿用模板文件里的旧版本号（模板文件只作未传版本时的 fallback）。
		if rel == ".version" && opts.Version != "" {
			data = []byte(opts.Version + "\n")
		}
		dest := filepath.Join(target, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(dest, data, 0o644); err != nil {
			return err
		}
		if !hashExcluded[rel] {
			sum := sha256.Sum256(data)
			// 清单键统一为相对项目根的 POSIX 路径（与 Trellis 的
			// `.template-hashes.json` 约定一致），不要用相对 .sunshine/ 的路径。
			hashes[filepath.ToSlash(filepath.Join(".sunshine", rel))] = hex.EncodeToString(sum[:])
		}
		return nil
	})
	if err != nil {
		return fmt.Errorf("写入工程骨架失败：%w", err)
	}
	// workflow.md 用完整版 native 模板（与 `sunshine workflow --template native`
	// 同源；2026-11-09 用户确认 init 改用完整版，不再用 28 行简版）。
	nativeWorkflow, err := templates.Workflows.ReadFile("workflows/native/workflow.md")
	if err != nil {
		return fmt.Errorf("读取原生工作流模板失败：%w", err)
	}
	if err := os.WriteFile(filepath.Join(target, "workflow.md"), nativeWorkflow, 0o644); err != nil {
		return fmt.Errorf("写入 workflow.md 失败：%w", err)
	}
	sum := sha256.Sum256(nativeWorkflow)
	hashes[".sunshine/workflow.md"] = hex.EncodeToString(sum[:])
	// 开发者身份文件（不进入哈希清单）。
	if err := os.WriteFile(filepath.Join(target, ".developer"), []byte(user+"\n"), 0o644); err != nil {
		return fmt.Errorf("写入开发者身份失败：%w", err)
	}
	// 模板哈希清单，供后续 update 做漂移检测。
	manifest, err := json.MarshalIndent(map[string]any{
		"__version": 2,
		"files":     hashes,
	}, "", "  ")
	if err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(target, ".template-hashes.json"), append(manifest, '\n'), 0o644); err != nil {
		return fmt.Errorf("写入模板哈希清单失败：%w", err)
	}
	// 产品设计先行：写 doc/ 设计文档模板（不在 .sunshine/ 里，
	// 不进哈希清单；已填过的内容绝不覆盖，-f 重建也不覆盖）。
	if opts.SpecType == projecttype.Design {
		if err := writeDesignTemplates(cwd); err != nil {
			return err
		}
	}
	return nil
}

// designTemplates 是产品设计文档模板：内嵌源路径 → 项目内目标路径。
var designTemplates = []struct{ src, dst string }{
	{"design/requirements.md", "doc/design/需求列表.md"},
	{"design/outline.md", "doc/design/概要设计.md"},
	{"design/prototype.html", "doc/prototype/原型.html"},
}

// writeDesignTemplates 把三份产品设计文档模板写进项目 doc/。
// 只在目标文件不存在时写：设计人员已填的内容绝不覆盖。
func writeDesignTemplates(cwd string) error {
	for _, t := range designTemplates {
		data, err := templates.Design.ReadFile(t.src)
		if err != nil {
			return fmt.Errorf("读取产品设计文档模板 %s 失败：%w", t.src, err)
		}
		dest := filepath.Join(cwd, filepath.FromSlash(t.dst))
		if _, err := os.Stat(dest); err == nil {
			continue
		} else if !os.IsNotExist(err) {
			return fmt.Errorf("检查 %s 失败：%w", t.dst, err)
		}
		if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(dest, data, 0o644); err != nil {
			return fmt.Errorf("写入 %s 失败：%w", t.dst, err)
		}
	}
	return nil
}
