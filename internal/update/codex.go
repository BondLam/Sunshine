package update

import (
	"os"
	"path/filepath"

	"github.com/linrong1982/Sunshine/internal/platforms"
)

// legacyCodexMarkers 是旧版 Codex 安装在清单里的标记文件
// （上游 needsCodexUpgrade 的 legacyMarkers，按 Sunshine 改名）：
// 旧版把 continue / finish-work 以 skill 形式写在共享
// .agents/skills/ 下，那时还没有独立的 .codex/ 配置目录。
var legacyCodexMarkers = []string{
	".agents/skills/sunshine-continue/SKILL.md",
	".agents/skills/sunshine-finish-work/SKILL.md",
}

// NeedsCodexUpgrade 判断是否需要旧版 Codex 升级修复
// （上游 needsCodexUpgrade）：.codex/ 已存在时不需要；
// 清单里有旧版标记、且没有其他已配置平台拥有这些标记路径时需要。
// 注意：本函数只做判断，不执行升级——调用方应在备份/确认后，
// 把 codex 纳入本次模板收集（extraPlatforms），
// .codex/ 文件经正常新文件流程写出（含 SessionStart 接线）。
func NeedsCodexUpgrade(cwd string, hashes map[string]string) bool {
	if _, err := os.Stat(filepath.Join(cwd, ".codex")); err == nil {
		return false
	}
	hasLegacyMarker := false
	for _, marker := range legacyCodexMarkers {
		if _, ok := hashes[marker]; ok {
			hasLegacyMarker = true
			break
		}
	}
	if !hasLegacyMarker {
		return false
	}
	// 当前或未来的非 Codex 平台也可能拥有这些标记路径；
	// 只有确认不是它们时，才触发 Codex 补装。
	for _, id := range ConfiguredPlatforms(cwd, hashes) {
		if id == "codex" {
			continue
		}
		files, err := platforms.RenderTemplates(cwd, id, platforms.InstallOptions{})
		if err != nil {
			continue
		}
		for _, marker := range legacyCodexMarkers {
			if _, ok := files[marker]; ok {
				return false
			}
		}
	}
	return true
}
