package update

import (
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/linrong007/Sunshine/internal/config"
	"github.com/linrong007/Sunshine/internal/platforms"
	"github.com/linrong007/Sunshine/templates"
)

// configYamlRel 是项目配置文件的相对路径。
const configYamlRel = ".sunshine/config.yaml"

// CollectOptions 控制 CollectTemplateFiles。
type CollectOptions struct {
	// ExtraPlatforms 是除已配置平台外额外收集的平台
	// （Codex 升级修复时传 ["codex"]）。
	ExtraPlatforms []string
	// BypassUpdateSkip 为真时忽略 update.skip
	// （破坏性版本 + --migrate 时的上游行为）。
	BypassUpdateSkip bool
}

// CollectTemplateFiles 收集本次 update 的期望模板集合
// （上游 collectTemplateFiles）：
//   - .sunshine/ 骨架：内嵌模板（config.yaml、workflow.md、agents/、
//     AGENTS.md、.gitignore）；spec/、tasks/、workspace/、.developer、
//     .current-task、.version 是用户数据或版本标记，不进集合；
//   - 已配置平台的全部模板文件（RenderTemplates；
//     Codex 的 model 保留同样生效，避免“只改了 model”被误判为冲突）；
//   - 根级 AGENTS.md：仅当磁盘已存在且带 Sunshine 托管块时纳入，
//     期望内容按托管块合并（Sunshine init 不生成根级 AGENTS.md，
//     从不新建它）；
//   - registry.spec 模板：经 RegistryRefresh 持续刷新后并入
//     （键为 .sunshine/spec/<名>；与内嵌 spec 模板不同，
//     registry 装的 spec 受哈希追踪，本地编辑走正常冲突流程）；
//   - 最后应用 update.skip（除非 BypassUpdateSkip）。
//
// 返回模板集合与本次生效的 skip 列表（供 safe-file-delete 复用）。
func CollectTemplateFiles(w io.Writer, cwd string, hashes map[string]string, opts CollectOptions) (map[string]string, []string, error) {
	files := map[string]string{}

	// .sunshine/ 骨架：内嵌模板。
	err := fs.WalkDir(templates.Sunshine, "sunshine", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		rel := strings.TrimPrefix(path, "sunshine/")
		switch {
		case rel == ".version" || rel == ".developer" || rel == ".current-task" ||
			rel == ".template-hashes.json":
			return nil
		case strings.HasPrefix(rel, "spec/") || rel == "spec" ||
			strings.HasPrefix(rel, "tasks/") || rel == "tasks" ||
			strings.HasPrefix(rel, "workspace/") || rel == "workspace":
			// 受保护的用户数据，不进模板集合。
			return nil
		}
		data, err := templates.Sunshine.ReadFile(path)
		if err != nil {
			return err
		}
		key := ".sunshine/" + rel
		switch rel {
		case "config.yaml":
			files[key] = PreserveExistingRegistryConfig(cwd, string(data))
		case "AGENTS.md":
			files[key] = BuildManagedBlockTemplate(cwd, key, string(data), SunshineBlockStart, SunshineBlockEnd)
		default:
			files[key] = string(data)
		}
		return nil
	})
	if err != nil {
		return nil, nil, fmt.Errorf("收集 .sunshine/ 模板失败：%w", err)
	}
	// workflow.md 用完整版 native 模板（与 init 同源）。
	nativeWorkflow, err := templates.Workflows.ReadFile("workflows/native/workflow.md")
	if err != nil {
		return nil, nil, fmt.Errorf("读取原生工作流模板失败：%w", err)
	}
	files[".sunshine/workflow.md"] = string(nativeWorkflow)

	// 平台模板：只收集已配置平台（+ Codex 升级修复时的 codex）。
	platformSet := map[string]bool{}
	for _, id := range ConfiguredPlatforms(cwd, hashes) {
		platformSet[id] = true
	}
	for _, id := range opts.ExtraPlatforms {
		platformSet[id] = true
	}
	for id := range platformSet {
		rendered, err := platforms.RenderTemplates(cwd, id, platforms.InstallOptions{})
		if err != nil {
			return nil, nil, fmt.Errorf("收集 %s 平台模板失败：%w", id, err)
		}
		for rel, content := range rendered {
			files[rel] = content
		}
	}

	// statusline 保留：新模板不带 statusLine，已有值带进新内容。
	PreserveExistingClaudeStatusLine(cwd, files)

	// Copilot instructions 托管块合并（上游 buildCopilotInstructionsTemplate）：
	// 磁盘已有该文件时，期望内容是“已有内容 + 模板托管块”的合并结果，
	// 不是模板全文直接覆盖；不存在时返回模板全文。
	// E 路起 Copilot 已接入，模板集合里有该文件即走合并。
	if template, ok := files[CopilotInstructionsPath]; ok {
		files[CopilotInstructionsPath] = BuildManagedBlockTemplate(cwd, CopilotInstructionsPath, template, CopilotBlockStart, CopilotBlockEnd)
	}

	// 根级 AGENTS.md：只管理已存在且带 Sunshine 托管块的文件。
	if data, err := os.ReadFile(filepath.Join(cwd, "AGENTS.md")); err == nil {
		if GetSunshineManagedBlock(string(data)) != "" {
			agentsTemplate, err := templates.Sunshine.ReadFile("sunshine/AGENTS.md")
			if err == nil {
				files["AGENTS.md"] = BuildManagedBlockTemplate(cwd, "AGENTS.md", string(agentsTemplate), SunshineBlockStart, SunshineBlockEnd)
			}
		}
	}

	// registry.spec 持续刷新。
	for rel, content := range CollectRegistrySpecTemplates(w, cwd) {
		files[ToPosix(rel)] = content
	}

	// update.skip（破坏性版本 + --migrate 时绕过）。
	skipPaths := config.LoadUpdateSkipPaths(cwd)
	if !opts.BypassUpdateSkip && len(skipPaths) > 0 {
		for rel := range files {
			if IsPathSkipped(rel, skipPaths) {
				delete(files, rel)
			}
		}
	}
	return files, skipPaths, nil
}
