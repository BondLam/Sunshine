package lifecycle

import (
	"github.com/linrong1982/Sunshine/internal/platforms"
)

// sunshine platforms：显示当前项目已配置的 AI 平台。
// 对照 cli/index.ts 的内联 platforms 命令 + getConfiguredPlatforms：
// 按哈希清单判断（清单里有该平台配置目录下条目的即为已配置），
// 输出 id / displayName / configDir；--json 输出 {"platforms": [...]}；
// 未配置任何平台时打印提示。
// displayName 取 internal/platforms 注册表的 Name（Go 版）；
// 上游 Codex 的 displayName 带有一段“.agents/skills/”说明性后缀，
// Go 版注册表已定为短名 "Codex"，此处沿用注册表，不另补后缀。

// PlatformInfo 是 platforms 命令输出的单个平台。
type PlatformInfo struct {
	ID          string `json:"id"`
	DisplayName string `json:"displayName"`
	ConfigDir   string `json:"configDir"`
}

// ConfiguredPlatforms 返回当前项目已配置的平台（按注册表顺序）。
// 清单不存在时返回空表；清单损坏时返回错误。
func ConfiguredPlatforms(cwd string) ([]PlatformInfo, error) {
	hashes, err := LoadHashes(cwd)
	if err != nil {
		return nil, err
	}
	configured := make([]PlatformInfo, 0)
	for _, id := range ConfiguredPlatformIDs(hashes) {
		p, ok := platforms.ByID(id)
		if !ok {
			continue
		}
		configured = append(configured, PlatformInfo{
			ID:          p.ID,
			DisplayName: p.Name,
			ConfigDir:   p.ConfigDir,
		})
	}
	return configured, nil
}
