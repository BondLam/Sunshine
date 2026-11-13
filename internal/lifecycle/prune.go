package lifecycle

import (
	"os"
	"strings"

	"github.com/linrong007/Sunshine/internal/platforms"
)

// 清单孤儿键自愈（对照 utils/manifest-prune.ts）。
//
// 旧版 init 曾用盲扫把用户自有文件（.codex/sessions/*、
// .claude/projects/*.jsonl、init 前已存在的 AGENTS.md 等）
// 写进清单；uninstall 按清单删文件时会误删用户数据。
// pruneOrphanManifestKeys 删掉“没有当前平台模板认领”的清单键，
// 只保留 Sunshine 实际写出的文件。
//
// 规则：
//   - `.sunshine/` 下的条目一律保留（uninstall 整个目录照删，
//     清单精度不影响数据安全；update 还要靠它们做改动检测）。
//   - 根 AGENTS.md 一律剪掉：Go 版 init 不写根 AGENTS.md，
//     清单里出现就是旧清单污染（上游是“有托管块标记才保留”，
//     Go 版没有根托管块概念，故直接剪）。
//   - 其余条目：只保留“当前已配置平台实际写出的文件”。
//
// “实际写出”的集合取自 internal/platforms.Install 本身：
// 在临时目录里对每个已配置平台跑一次 Install，
// 返回的写出文件表就是该平台认领的键。复用同一份写出逻辑，
// prune 与 init 永远不会对“写了什么”各说各话。
// （A 组的 update 也需要同一份逻辑；总协调人合并时再决定
// 是否抽成公共函数，本包先自备一份，不动 internal/update/。）

// PruneResult 是剪枝结果：被剪的键与剪后的清单。
type PruneResult struct {
	Pruned []string
	Hashes map[string]string
}

// PruneOrphanManifestKeys 剪掉清单里的孤儿键。
// persist 为真且剪掉了条目时把新清单写回磁盘
// （dry-run 时传 false，只算不写）。
func PruneOrphanManifestKeys(cwd string, configuredPlatformIDs []string, hashes map[string]string, persist bool) (PruneResult, error) {
	known, err := knownTemplatePaths(configuredPlatformIDs)
	if err != nil {
		return PruneResult{}, err
	}
	var pruned []string
	kept := make(map[string]string, len(hashes))
	for key, value := range hashes {
		posixKey := strings.ReplaceAll(key, "\\", "/")
		if posixKey == SunshineDir || strings.HasPrefix(posixKey, SunshineDir+"/") {
			kept[posixKey] = value
			continue
		}
		if known[posixKey] {
			kept[posixKey] = value
			continue
		}
		// devin 的 Windsurf 遗留键：.windsurf/workflows/sunshine-* 是
		// devin 已配置的兼容形态，Install 不重写它们，但清单里有
		// 就是 devin 的托管内容，devin 已配置时不剪。
		if devinLegacyKey(posixKey) && containsString(configuredPlatformIDs, "devin") {
			kept[posixKey] = value
			continue
		}
		pruned = append(pruned, posixKey)
	}
	if persist && len(pruned) > 0 {
		if err := SaveHashes(cwd, kept); err != nil {
			return PruneResult{}, err
		}
	}
	return PruneResult{Pruned: pruned, Hashes: kept}, nil
}

// devinLegacyKey 报告清单键是否为 devin 的 Windsurf 遗留键
// （.windsurf/workflows/sunshine-*，C 规格 §5.2）。
func devinLegacyKey(posixKey string) bool {
	return strings.HasPrefix(posixKey, ".windsurf/workflows/sunshine-")
}

// knownTemplatePaths 返回已配置平台实际写出的文件集合
// （相对项目根的 POSIX 路径）。做法是在临时目录里
// 对每个平台跑一次 Install，收集写出表。
// 临时目录在函数返回时删掉；Install 失败时直接报错，
// 调用方应硬停——算不出“写了什么”时宁可不删，不猜。
func knownTemplatePaths(configuredPlatformIDs []string) (map[string]bool, error) {
	known := map[string]bool{}
	if len(configuredPlatformIDs) == 0 {
		return known, nil
	}
	tmp, err := os.MkdirTemp("", "sunshine-prune-*")
	if err != nil {
		return nil, err
	}
	defer func() { _ = os.RemoveAll(tmp) }()
	for _, id := range configuredPlatformIDs {
		if _, ok := platforms.ByID(id); !ok {
			continue
		}
		files, err := platforms.Install(tmp, id, platforms.InstallOptions{})
		if err != nil {
			return nil, err
		}
		for _, f := range files {
			known[f] = true
		}
	}
	return known, nil
}
