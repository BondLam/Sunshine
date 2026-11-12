package cli

import (
	"errors"
	"fmt"

	"github.com/BondLam/Sunshine/internal/config"
	"github.com/BondLam/Sunshine/internal/registry"
	"github.com/BondLam/Sunshine/internal/scaffold"
	"github.com/spf13/cobra"
)

var (
	initRegistry  string
	initTemplate  string
	initOverwrite bool
	initAppend    bool
)

// registerRegistryFlags 注册 init 的 --registry 相关 flag。
// 由总协调人接线：init.go 的 init() 里调用本函数（root.go 不动）。
func registerRegistryFlags(cmd *cobra.Command) {
	cmd.Flags().StringVar(&initRegistry, "registry", "", "模板市场来源：gh:owner/repo、完整 git URL、HTTP(S) URL、git@ 形式或本地 git 仓库路径")
	cmd.Flags().StringVar(&initTemplate, "template", "", "模板市场条目 id（配合 --registry 使用）")
	cmd.Flags().BoolVar(&initOverwrite, "overwrite", false, "目标已存在时覆盖（仅配合 --template 使用）")
	cmd.Flags().BoolVar(&initAppend, "append", false, "目标已存在时只补缺失文件（仅配合 --template 使用）")
}

// resolveRegistryStrategy 把 --overwrite/--append 转成模板策略，
// 并校验它们仅配合 --template 使用（对应上游 init.ts 的语义）。
func resolveRegistryStrategy() (registry.TemplateStrategy, error) {
	if initOverwrite && initAppend {
		return "", errors.New("--overwrite 与 --append 不能同时使用")
	}
	if (initOverwrite || initAppend) && initTemplate == "" {
		return "", errors.New("--overwrite/--append 仅配合 --template 使用")
	}
	switch {
	case initOverwrite:
		return registry.StrategyOverwrite, nil
	case initAppend:
		return registry.StrategyAppend, nil
	default:
		return registry.StrategySkip, nil
	}
}

// installRegistryTemplates 执行 registry 安装：记哈希、持久化 registry.spec。
// 安装失败时回退到内置空白 spec 模板（仅当目标会写 spec 时），不中断 init。
// target 的探测资源（临时克隆）由本次安装接管并关闭。
func installRegistryTemplates(cwd string, target *registry.Target, strategy registry.TemplateStrategy) {
	result, err := scaffold.InstallRegistry(cwd, registry.InstallOptions{
		Source:   initRegistry,
		Template: initTemplate,
		Strategy: strategy,
		Probe:    target.Probe,
	})
	if err != nil {
		fmt.Printf("警告：模板市场安装失败：%v\n", err)
		if target.WritesSpec() {
			fmt.Println("改用内置空白规范模板继续初始化。")
			files, werr := scaffold.WriteDefaultSpec(cwd)
			if werr != nil {
				fmt.Printf("警告：写入内置空白规范模板失败：%v\n", werr)
			} else if err := scaffold.RecordHashes(cwd, files); err != nil {
				fmt.Printf("警告：记录模板哈希失败：%v\n", err)
			}
		}
		retry := fmt.Sprintf("sunshine init --registry %s", initRegistry)
		if initTemplate != "" {
			retry += " --template " + initTemplate
		}
		fmt.Printf("稍后可重试：%s\n", retry)
		return
	}
	if result.Skipped {
		fmt.Printf("模板已存在，已跳过安装（%s）。\n", result.DestRel)
		return
	}
	if result.Mode == "marketplace" {
		fmt.Printf("已安装模板 %q（%s），共 %d 个文件。\n", result.Template.ID, result.DestRel, len(result.Files))
	} else {
		fmt.Printf("已从模板市场直接安装规范模板，共 %d 个文件。\n", len(result.Files))
	}
	if len(result.Files) > 0 {
		if err := scaffold.RecordHashes(cwd, result.Files); err != nil {
			fmt.Printf("警告：记录模板哈希失败：%v\n", err)
		}
	}
	if err := config.WriteSpecRegistryConfig(cwd, result.SpecConfig); err != nil {
		fmt.Printf("警告：写入 registry.spec 失败：%v\n", err)
		return
	}
	if result.SpecConfig.Template != "" {
		fmt.Printf("已记录 registry.spec：%s（模板 %s），sunshine update 会持续刷新。\n",
			result.SpecConfig.Source, result.SpecConfig.Template)
	} else {
		fmt.Printf("已记录 registry.spec：%s，sunshine update 会持续刷新。\n", result.SpecConfig.Source)
	}
}
