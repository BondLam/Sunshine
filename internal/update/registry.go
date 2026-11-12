package update

import (
	"fmt"
	"io"

	"github.com/BondLam/Sunshine/internal/config"
	"github.com/BondLam/Sunshine/internal/registry"
)

// RegistryRefresher 是模板市场（B 组，internal/registry）向 update
// 提供的 registry.spec 持续刷新接口。
//
// 契约：
//   - 输入是 config.yaml 里 registry.spec 的 source 与 template；
//   - 返回本次 update 要用的 spec 模板集合，键为相对项目根的 POSIX 路径
//     （如 ".sunshine/spec/backend.md"），值为文件内容；
//   - marketplace 型 registry 缺 template、或模板 id 不在 index 里时，
//     返回空集合 + error（update 只警告、不中断）；
//   - 直连型 registry 拉取失败时同样返回 error（update 只警告、不中断）；
//   - 返回的 spec 文件同样受哈希追踪，本地编辑走正常冲突流程。
//
// B 组的实现是 registry.Refresher（见 internal/registry/refresh.go），
// 本包默认就用它；RegistryRefresh 变量保留给测试替换。
type RegistryRefresher interface {
	RefreshSpecTemplates(source, template string) (map[string]string, error)
}

// RegistryRefresh 是当前注册的 registry 刷新实现
// （默认 B 组的 registry.Refresher）。
var RegistryRefresh RegistryRefresher = registry.Refresher{}

// CollectRegistrySpecTemplates 按 registry.spec 重新拉取 spec 模板
// （上游 collectRegistrySpecTemplates 的流程骨架，拉取本身交给
// RegistryRefresh）：无 registry.spec 时返回空集合；
// 未接入刷新实现时打印警告并返回空集合；
// 刷新失败时打印警告并返回空集合（失败只警告不中断）。
func CollectRegistrySpecTemplates(w io.Writer, cwd string) map[string]string {
	cfg, err := config.LoadSpecRegistryConfig(cwd)
	if err != nil {
		fmt.Fprintf(w, "警告：读取 registry.spec 失败：%v\n", err)
		return map[string]string{}
	}
	if cfg == nil {
		return map[string]string{}
	}
	if RegistryRefresh == nil {
		fmt.Fprintln(w, "警告：检测到 registry.spec，但模板市场刷新尚未接入，跳过本次 spec 刷新。")
		return map[string]string{}
	}
	files, err := RegistryRefresh.RefreshSpecTemplates(cfg.Source, cfg.Template)
	if err != nil {
		fmt.Fprintf(w, "警告：registry spec 刷新跳过：%v\n", err)
		return map[string]string{}
	}
	return files
}
