package update

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/linrong1982/Sunshine/internal/platforms"
)

// ConfiguredPlatforms 按哈希清单判断已配置的平台（上游
// getConfiguredPlatforms）：清单里有该平台配置目录下任一模板条目
// 的，即为已配置。目录不存在本身不算配置——
// 预装的原生平台目录没有 Sunshine 模板条目时不算。
func ConfiguredPlatforms(cwd string, hashes map[string]string) []string {
	var configured []string
	for _, p := range platforms.Registry() {
		files, err := platforms.RenderTemplates(cwd, p.ID, platforms.InstallOptions{})
		if err != nil {
			continue
		}
		for key := range files {
			if (key == p.ConfigDir || strings.HasPrefix(key, p.ConfigDir+"/")) &&
				hashes[key] != "" {
				configured = append(configured, p.ID)
				break
			}
		}
	}
	// devin 的 Windsurf 遗留兼容（C 规格 §5.2）：.windsurf/workflows/
	// 下有 sunshine- 前缀的被追踪模板时，同样判定 devin 已配置。
	found := false
	for _, id := range configured {
		if id == "devin" {
			found = true
		}
	}
	if !found {
		for key := range hashes {
			if strings.HasPrefix(key, ".windsurf/workflows/sunshine-") {
				configured = append(configured, "devin")
				break
			}
		}
	}
	return configured
}

// PruneResult 是清单孤儿键清理的结果。
type PruneResult struct {
	// Pruned 是被删除的清单键（POSIX 相对路径）。
	Pruned []string
	// Hashes 是清理后的清单。
	Hashes map[string]string
}

// PruneOrphanManifestKeys 清理清单里的孤儿键
// （上游 pruneOrphanManifestKeys，自愈被污染的清单）：
//   - .sunshine/ 下的条目一律保留（update 靠它们检测用户改动；
//     uninstall 整个删除 .sunshine/，清单精度不影响数据安全）；
//   - 根级 AGENTS.md 仅当文件缺失、或仍带 Sunshine 托管块标记时保留，
//     否则视为旧版误追踪的用户自有文件，删键保文件；
//   - 迁移清单 from/to 引用的路径保留（待执行的迁移需要它们）；
//   - 其余条目只在“当前已配置平台的模板集合”里有时保留，
//     否则删除（例如旧版 init 误哈希的 .codex/sessions/*、
//     用户自加的 .codex/skills/<custom>/）。
//
// persist 为真时把清理后的清单写回磁盘（dry-run 时传 false，
// 只计算不写盘——与上游“dry-run 也写盘”不同，见 PLAN.md 记录）。
func PruneOrphanManifestKeys(cwd string, configured []string, hashes map[string]string, migrationPaths map[string]bool, persist bool) (PruneResult, error) {
	known := map[string]bool{}
	for _, id := range configured {
		files, err := platforms.RenderTemplates(cwd, id, platforms.InstallOptions{})
		if err != nil {
			continue
		}
		for key := range files {
			known[ToPosix(key)] = true
		}
	}
	for p := range migrationPaths {
		known[ToPosix(p)] = true
	}
	kept := map[string]string{}
	var pruned []string
	for rawKey, value := range hashes {
		key := ToPosix(rawKey)
		if key == ".sunshine" || strings.HasPrefix(key, ".sunshine/") {
			kept[key] = value
			continue
		}
		if key == "AGENTS.md" {
			if shouldKeepAgentsMd(cwd) {
				kept[key] = value
			} else {
				pruned = append(pruned, key)
			}
			continue
		}
		if known[key] {
			kept[key] = value
			continue
		}
		// devin 的 Windsurf 遗留键：devin 已配置时不剪
		// （见 lifecycle 的同款保留逻辑）。
		if strings.HasPrefix(key, ".windsurf/workflows/sunshine-") {
			for _, id := range configured {
				if id == "devin" {
					kept[key] = value
					break
				}
			}
			if _, ok := kept[key]; ok {
				continue
			}
		}
		pruned = append(pruned, key)
	}
	if persist && len(pruned) > 0 {
		if err := SaveHashes(cwd, kept); err != nil {
			return PruneResult{}, err
		}
	}
	return PruneResult{Pruned: pruned, Hashes: kept}, nil
}

// shouldKeepAgentsMd 判断根级 AGENTS.md 的清单条目是否保留
// （上游 shouldKeepAgentsMd）：文件缺失时保留；
// 文件仍带 Sunshine 托管块标记时保留；否则删键保文件。
func shouldKeepAgentsMd(cwd string) bool {
	data, err := os.ReadFile(filepath.Join(cwd, "AGENTS.md"))
	if err != nil {
		return true
	}
	content := string(data)
	return strings.Contains(content, SunshineBlockStart) &&
		strings.Contains(content, SunshineBlockEnd)
}
