package registry

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/BondLam/Sunshine/internal/config"
)

// Target 是 --registry 解析后的安装目标：模式、模板条目与探测结果。
// 由 ResolveTarget 产生，供 CLI 在 scaffold.Init 之前决定是否跳过
// 默认 spec 模板（WritesSpec），再交给 Install 执行安装。
type Target struct {
	// Source 是解析后的 registry 来源。
	Source *Source
	// Mode 是 marketplace（有 index.json）或 direct（无 index.json）。
	Mode string
	// Template 是 marketplace 模式下选中的条目；direct 模式为 nil。
	Template *SpecTemplate
	// TemplateID 是 --template 传入的 id（direct 模式为空）。
	TemplateID string
	// Probe 是探测结果；git 后端时保留临时克隆，Install 会接管并关闭，
	// 未调用 Install 时调用方需 Close。
	Probe *ProbeResult
}

// WritesSpec 报告本次安装是否会写 .sunshine/spec（direct 模式一定会；
// marketplace 模式仅当选中模板是 spec 类型时）。
func (t *Target) WritesSpec() bool {
	if t == nil {
		return false
	}
	if t.Mode == "direct" {
		return true
	}
	return t.Template != nil && t.Template.Type == "spec"
}

// Close 释放 Target 保留的探测资源；Install 后调用也安全。
func (t *Target) Close() {
	if t == nil {
		return
	}
	if t.Probe != nil {
		t.Probe.Close()
	}
}

// ResolveTarget 解析 --registry 来源并探测 index.json，确定安装模式与模板。
// marketplace 模式必须用 --template 指定条目（对应上游 -y 模式的行为：
// 多模板市场无法自动选择）；workflow 类型模板会被拒绝，提示改用
// sunshine workflow --template。探测到瞬时失败（非 404）时直接报错，
// 不误判为直接下载模式。
func ResolveTarget(source, templateID string) (*Target, error) {
	src, err := ParseRegistrySource(source)
	if err != nil {
		return nil, err
	}
	probe := ProbeRegistryIndex(context.Background(), src)
	if probe.Err != nil {
		probe.Close()
		return nil, probe.Err
	}
	return ResolveTargetWithProbe(src, probe, templateID)
}

// ResolveTargetWithProbe 用已探测到的 index 解析安装目标，复用调用方
// 传入的 probe（交互选模板流程先探测再选，避免对同一来源探测两次）。
// probe 须是已成功探测的结果（Err 为 nil）；出错时关闭 probe，
// 成功时 Target 接管 probe。
func ResolveTargetWithProbe(src *Source, probe *ProbeResult, templateID string) (*Target, error) {
	if probe.IsNotFound {
		if templateID != "" {
			probe.Close()
			return nil, NewError(ErrKindNotFound, "该 registry 没有 index.json，不能用 --template；去掉 --template 走直接下载模式")
		}
		return &Target{Source: src, Mode: "direct", Probe: probe}, nil
	}
	if templateID == "" {
		ids := TemplateIDs(probe.Templates)
		probe.Close()
		return nil, fmt.Errorf("该 registry 是模板市场，有多个模板，请用 --template <id> 指定一个，可选：%s", strings.Join(ids, "、"))
	}
	t := FindTemplate(probe.Templates, templateID)
	if t == nil {
		ids := TemplateIDs(probe.Templates)
		probe.Close()
		return nil, NewError(ErrKindNotFound, "模板 %q 不存在，可选：%s", templateID, strings.Join(ids, "、"))
	}
	if t.Type == "workflow" {
		probe.Close()
		return nil, fmt.Errorf("工作流模板请用 sunshine workflow --template %s 安装，不要用 --registry", t.ID)
	}
	return &Target{Source: src, Mode: "marketplace", Template: t, TemplateID: templateID, Probe: probe}, nil
}

// InstallOptions 是 Install 的参数。
type InstallOptions struct {
	// Source 是 --registry 的原始输入。
	Source string
	// Template 是 --template 的 id，direct 模式为空。
	Template string
	// Strategy 是冲突策略（skip / overwrite / append）。
	Strategy TemplateStrategy
	// Probe 是可选的已探测结果（一般来自 ResolveTarget）；
	// 传入后 Install 接管并在返回前关闭。
	Probe *ProbeResult
}

// InstallResult 是安装结果。
type InstallResult struct {
	// Mode 是 marketplace 或 direct。
	Mode string
	// Template 是 marketplace 模式下安装的条目；direct 模式为 nil。
	Template *SpecTemplate
	// DestRel 是安装目录（相对项目根的 POSIX 路径）。
	DestRel string
	// Files 是本次安装的文件（相对项目根的 POSIX 路径），供调用方记入哈希清单。
	Files []string
	// Skipped 为真表示目标已存在且策略为 skip，未安装。
	Skipped bool
	// SpecConfig 是待持久化进 config.yaml 的 registry.spec。
	SpecConfig config.SpecRegistryConfig
}

// Install 执行一次 registry 安装：marketplace 模式按 id 装指定条目，
// direct 模式把 registry 内容装进 .sunshine/spec。
// Probe（若传入）由 Install 接管并关闭。
func Install(cwd string, opts InstallOptions) (*InstallResult, error) {
	src, err := ParseRegistrySource(opts.Source)
	if err != nil {
		// 解析失败时调用方传入的 probe 也由 Install 负责关闭，避免泄漏临时克隆。
		if opts.Probe != nil {
			opts.Probe.Close()
		}
		return nil, err
	}
	probe := opts.Probe
	if probe == nil {
		probe = ProbeRegistryIndex(context.Background(), src)
	}
	// 无论成功失败，Install 接管的 probe 都在这里关闭；
	// 调用方传入的 probe 同样由 Install 负责关闭。
	defer probe.Close()
	if probe.Err != nil {
		return nil, probe.Err
	}

	result := &InstallResult{
		SpecConfig: config.SpecRegistryConfig{Source: opts.Source, Template: opts.Template},
	}
	if probe.IsNotFound {
		if opts.Template != "" {
			return nil, NewError(ErrKindNotFound, "该 registry 没有 index.json，不能用 --template；去掉 --template 走直接下载模式")
		}
		result.Mode = "direct"
		result.DestRel = InstallPaths["spec"]
		installed, skipped, files, err := DownloadRegistryDirect(context.Background(), cwd, src, probe, opts.Strategy, "")
		if err != nil {
			return nil, err
		}
		result.Skipped = skipped
		if installed {
			result.Files = files
		}
		return result, nil
	}

	// Marketplace 模式。
	if opts.Template == "" {
		return nil, fmt.Errorf("该 registry 是模板市场，有多个模板，请用 --template <id> 指定一个，可选：%s",
			strings.Join(TemplateIDs(probe.Templates), "、"))
	}
	t := FindTemplate(probe.Templates, opts.Template)
	if t == nil {
		return nil, NewError(ErrKindNotFound, "模板 %q 不存在，可选：%s", opts.Template, strings.Join(TemplateIDs(probe.Templates), "、"))
	}
	if t.Type == "workflow" {
		return nil, fmt.Errorf("工作流模板请用 sunshine workflow --template %s 安装，不要用 --registry", t.ID)
	}
	result.Mode = "marketplace"
	result.Template = t
	result.DestRel = templateDestRel(t)
	installed, skipped, files, err := installTemplate(context.Background(), cwd, src, probe, t, opts.Strategy, "")
	if err != nil {
		return nil, err
	}
	result.Skipped = skipped
	if installed {
		result.Files = files
	}
	return result, nil
}

// EnsureProjectRoot 校验 cwd 不是用户主目录（与 scaffold.Init 的守卫一致），
// 供 CLI 在 --registry 安装前调用。主目录安装需 SUNSHINE_ALLOW_HOMEDIR=1。
func EnsureProjectRoot(cwd string) error {
	home, err := os.UserHomeDir()
	if err == nil && cwd == home && os.Getenv("SUNSHINE_ALLOW_HOMEDIR") != "1" {
		return fmt.Errorf("不能在用户主目录执行初始化（设置 SUNSHINE_ALLOW_HOMEDIR=1 可绕过）")
	}
	return nil
}
