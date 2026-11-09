package update

import (
	"os"

	"github.com/linrong1982/Sunshine/internal/migrate"
	"golang.org/x/term"
)

// MigrationRegistry 是迁移系统（C 组，internal/migrate）向 update
// 提供的清单集。清单从 Sunshine 自己的版本线起步，第一份真实清单是
// 0.2.0（internal/migrate/manifests/0.2.0.json）；后续每次模板结构
// 变更由该版本补一份 internal/migrate/manifests/<版本号>.json
// （随二进制内嵌发布，加入方式见 manifests/README.md 的“新增清单”一节）。
//
// 接线：默认用内嵌清单集（LoadEmbeddedRegistry）；测试或外部清单可
// 直接给本变量赋值覆盖，例如：
//
//	update.MigrationRegistry, _ = migrate.LoadManifestDir(<清单目录>)
//
// 为 nil 时用内嵌清单集（目前为空），update 跳过全部迁移逻辑。
var MigrationRegistry *migrate.Registry

// MigrationPrompter 是迁移确认的交互实现（C 组默认的
// migrate.PromptMigrationAction，用 survey 提问“直接执行 / 备份后执行 /
// 跳过”，默认备份后执行）。为 nil 时用默认实现（交互环境），
// 非交互环境（opts.Prompter 为 nil）时一律跳过。
// 测试可替换本变量以控制迁移确认。
var MigrationPrompter migrate.Prompter

// migrationRegistry 返回当前清单集（为 nil 时返回内嵌清单集）。
func migrationRegistry() *migrate.Registry {
	if MigrationRegistry != nil {
		return MigrationRegistry
	}
	embedded, _ := migrate.LoadEmbeddedRegistry()
	return embedded
}

// MigrationPaths 返回全部迁移清单项引用的路径集合
// （供清单孤儿键清理保留用；无迁移系统时返回空集合）。
func MigrationPaths() map[string]bool {
	out := map[string]bool{}
	for _, item := range migrationRegistry().AllMigrations() {
		if item.From != "" {
			out[ToPosix(item.From)] = true
		}
		if item.To != "" {
			out[ToPosix(item.To)] = true
		}
	}
	return out
}

// migrationPrompterFor 决定迁移执行用的确认函数：
// 测试替身优先；否则交互环境用 C 组默认的 PromptMigrationAction，
// 非交互环境一律跳过（survey 在非交互终端会报错，
// ExecuteMigrations 会把报错当作跳过并记为错误、让整个 update 失败，
// 所以这里根本不调 survey——与 cli 的 surveyPrompter
// “非交互时不提问、冲突默认 Skip”是同一门禁）。
func migrationPrompterFor(opts Options) migrate.Prompter {
	if MigrationPrompter != nil {
		return MigrationPrompter
	}
	if opts.Prompter == nil || !stdinIsTerminal() {
		return func(migrate.MigrationItem) (migrate.MigrationAction, error) {
			return migrate.ActionSkip, nil
		}
	}
	return migrate.PromptMigrationAction
}

// stdinIsTerminal 用 ioctl 判断标准输入是否为交互终端
// （与 internal/cli 的 stdinIsTTY 同款；/dev/null 虽是字符设备
// 但不是终端，不能只看 ModeCharDevice）。
func stdinIsTerminal() bool {
	return term.IsTerminal(int(os.Stdin.Fd()))
}
