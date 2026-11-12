package registry

import (
	"context"
	"fmt"
	"os"

	"github.com/BondLam/Sunshine/internal/config"
)

// Refresher 实现 update 包的 RegistryRefresher 接口
// （见 internal/update/registry.go），供总协调人接线：
//
//	update.RegistryRefresh = registry.Refresher{}
//
// 注意：update 包的 registry.go 注释称实现在
// internal/workflow/marketplace.go，实际按阶段 4 分工落在
// internal/registry（本包），以此处为准。
type Refresher struct{}

// RefreshSpecTemplates 按 source 与 template 重新拉取 spec 模板，
// 返回本次 update 要用的 spec 模板集合（键为相对项目根的 POSIX 路径，
// 如 .sunshine/spec/backend.md，值为文件内容）。
// marketplace 型缺 template、模板 id 不在 index 里、拉取失败时
// 返回错误（调用方只警告、不中断）。
func (Refresher) RefreshSpecTemplates(source, template string) (map[string]string, error) {
	return RefreshSpecTemplates(source, template)
}

// RefreshSpecTemplates 是 Refresher 的包级实现（便于直接调用与测试）。
func RefreshSpecTemplates(source, template string) (map[string]string, error) {
	src, err := ParseRegistrySource(source)
	if err != nil {
		return nil, err
	}
	ctx := context.Background()
	probe := ProbeRegistryIndex(ctx, src)
	defer probe.Close()
	if probe.Err != nil {
		return nil, probe.Err
	}
	if len(probe.Templates) > 0 {
		if template == "" {
			return nil, fmt.Errorf("模板市场型 registry 需要 registry.spec.template")
		}
		t := FindTemplate(probe.Templates, template)
		if t == nil {
			return nil, NewError(ErrKindNotFound, "模板 %q 不在 registry index 里", template)
		}
		if t.Type != "spec" {
			return nil, fmt.Errorf("模板 %q 不是 spec 类型，无法作为 registry spec 刷新", template)
		}
		tmpRoot, err := os.MkdirTemp("", "sunshine-registry-template-")
		if err != nil {
			return nil, fmt.Errorf("创建临时目录失败：%w", err)
		}
		defer os.RemoveAll(tmpRoot)
		if _, _, _, err := installTemplate(ctx, tmpRoot, src, probe, t, StrategyOverwrite, ""); err != nil {
			return nil, err
		}
		return CollectDirectoryFiles(tmpRoot+"/.sunshine/spec", ".sunshine/spec")
	}
	if !probe.IsNotFound {
		return nil, fmt.Errorf("无法访问 registry")
	}
	return FetchRegistrySpecTemplates(ctx, src)
}

// CollectRegistrySpecTemplates 按 config.yaml 的 registry.spec 重新拉取
// spec 模板，作为 update 的 spec 模板集合，对应上游 update.ts 的
// collectRegistrySpecTemplates。
//
//   - 未配 registry.spec → 空集合；
//   - marketplace 型缺 template → 跳过并警告；
//   - template 不在 index 里 / 拉取失败 → 只警告，不中断；
//   - 直连型走 FetchRegistrySpecTemplates；
//   - 返回的键均为 .sunshine/spec/**（相对项目根的 POSIX 路径）。
//
// 警告直接打印到标准输出（与上游 console.log 行为一致）。
// 需要错误值的调用方请用 RefreshSpecTemplates / Refresher。
func CollectRegistrySpecTemplates(cwd string) map[string]string {
	empty := make(map[string]string)
	cfg, err := config.LoadSpecRegistryConfig(cwd)
	if err != nil {
		fmt.Printf("警告：读取 registry.spec 失败：%v\n", err)
		return empty
	}
	if cfg == nil {
		return empty
	}
	files, err := RefreshSpecTemplates(cfg.Source, cfg.Template)
	if err != nil {
		fmt.Printf("警告：registry spec 刷新已跳过：%v\n", err)
		return empty
	}
	return files
}
