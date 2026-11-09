package migrate

import "embed"

// embeddedManifests 是随二进制发布的迁移清单（manifests/*.json）。
// 第一份真实清单是 0.2.0（蓝本 Trellis 0.5.0-beta.0 的 Skill-first
// 迁移）；后续每次模板结构变更由该版本补一份清单文件，
// 不需要改代码即可随二进制发布。目录里的 README.md 不是清单，
// 加载时会被 LoadManifestFS 跳过（只读 *.json）。
//
//go:embed manifests
var embeddedManifests embed.FS

// LoadEmbeddedRegistry 从二进制内嵌的清单加载清单集。
// 目录不存在或为空时返回空清单集（不报错）。
func LoadEmbeddedRegistry() (*Registry, []error) {
	return LoadManifestFS(embeddedManifests, "manifests")
}
