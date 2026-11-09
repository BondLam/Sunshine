package registry

import (
	"context"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// TemplateStrategy 是模板安装的冲突策略，对应上游 TemplateStrategy。
type TemplateStrategy string

const (
	// StrategySkip 已存在时跳过，不下载。
	StrategySkip TemplateStrategy = "skip"
	// StrategyOverwrite 已存在时先下载到临时目录再整体替换。
	StrategyOverwrite TemplateStrategy = "overwrite"
	// StrategyAppend 已存在时只补缺失文件。
	StrategyAppend TemplateStrategy = "append"
)

// ParseStrategy 解析策略字符串，非法时返回错误。
func ParseStrategy(s string) (TemplateStrategy, error) {
	switch TemplateStrategy(strings.ToLower(strings.TrimSpace(s))) {
	case StrategySkip:
		return StrategySkip, nil
	case StrategyOverwrite:
		return StrategyOverwrite, nil
	case StrategyAppend:
		return StrategyAppend, nil
	default:
		return "", fmt.Errorf("未知的模板策略 %q，只接受 skip / overwrite / append", s)
	}
}

// skipGitDir 是 WalkDir 回调的 .git 排除：克隆的 .git 目录绝不能
// 跟着模板内容装进项目（上游 giget 的 tarball 本来就不含 .git）。
func skipGitDir(d fs.DirEntry) bool {
	return d.IsDir() && d.Name() == ".git"
}

// copyMissing 只复制目标不存在的文件（append 策略用）。
func copyMissing(srcDir, destDir string) error {
	return filepath.WalkDir(srcDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if skipGitDir(d) {
			return filepath.SkipDir
		}
		rel, err := filepath.Rel(srcDir, path)
		if err != nil {
			return err
		}
		dest := filepath.Join(destDir, rel)
		if d.IsDir() {
			return os.MkdirAll(dest, 0o755)
		}
		if _, err := os.Stat(dest); err == nil {
			return nil
		}
		return copyFile(path, dest)
	})
}

// copyAll 复制全部文件，覆盖已存在的文件。
func copyAll(srcDir, destDir string) error {
	return filepath.WalkDir(srcDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if skipGitDir(d) {
			return filepath.SkipDir
		}
		rel, err := filepath.Rel(srcDir, path)
		if err != nil {
			return err
		}
		dest := filepath.Join(destDir, rel)
		if d.IsDir() {
			return os.MkdirAll(dest, 0o755)
		}
		return copyFile(path, dest)
	})
}

// copyFile 复制单个文件（创建父目录，保留权限位）。
func copyFile(src, dest string) error {
	data, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		return err
	}
	st, err := os.Stat(src)
	mode := os.FileMode(0o644)
	if err == nil {
		mode = st.Mode().Perm()
	}
	return os.WriteFile(dest, data, mode)
}

// copyWithStrategy 按策略把 srcDir 的内容装进 destDir。
// 返回 false 表示 skip 策略下目标已存在、未安装。
func copyWithStrategy(srcDir, destDir string, strategy TemplateStrategy) (bool, error) {
	st, err := os.Stat(destDir)
	exists := err == nil && st.IsDir()
	if strategy == StrategySkip && exists {
		return false, nil
	}
	switch strategy {
	case StrategyOverwrite:
		if exists {
			// 先把新内容备到临时目录，成功后再替换旧目录，避免
			// 复制中途失败毁掉用户现有内容（对应上游 overwrite 的
			// temp-dir swap；跨文件系统时不用 rename，用复制）。
			tmp, err := os.MkdirTemp("", "sunshine-template-")
			if err != nil {
				return false, fmt.Errorf("创建临时目录失败：%w", err)
			}
			defer os.RemoveAll(tmp)
			if err := copyAll(srcDir, tmp); err != nil {
				return false, err
			}
			if err := os.RemoveAll(destDir); err != nil {
				return false, fmt.Errorf("清理旧目录失败：%w", err)
			}
			if err := copyAll(tmp, destDir); err != nil {
				return false, err
			}
			return true, nil
		}
		return true, copyAll(srcDir, destDir)
	case StrategyAppend:
		if exists {
			return true, copyMissing(srcDir, destDir)
		}
		return true, copyAll(srcDir, destDir)
	default: // skip（目标不存在时直接装）
		return true, copyAll(srcDir, destDir)
	}
}

// copyWithStrategyRoot 是 copyWithStrategy 的项目根版本（full 类型用）：
// 目标就是项目根，永远不删除目标目录；skip 按 append 处理
// （只补缺失文件），overwrite 直接覆盖同名文件。
// 上游对 full 类型未做此保护，Go 版为安全起见特殊处理。
func copyWithStrategyRoot(srcDir, destDir string, strategy TemplateStrategy) (bool, error) {
	switch strategy {
	case StrategyOverwrite:
		return true, copyAll(srcDir, destDir)
	default:
		return true, copyMissing(srcDir, destDir)
	}
}

// copySingleFileWithStrategy 按策略安装单个文件。
func copySingleFileWithStrategy(srcFile, destFile string, strategy TemplateStrategy) (bool, error) {
	if _, err := os.Stat(destFile); err == nil {
		switch strategy {
		case StrategySkip:
			return false, nil
		case StrategyAppend:
			return false, nil
		}
	}
	return true, copyFile(srcFile, destFile)
}

// collectFiles 列出 root 下的全部文件（相对 root 的 POSIX 路径）。
func collectFiles(root string) ([]string, error) {
	var files []string
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if skipGitDir(d) {
			return filepath.SkipDir
		}
		if d.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		files = append(files, filepath.ToSlash(rel))
		return nil
	})
	return files, err
}

// CollectDirectoryFiles 收集 root 下全部文件的内容，键为
// relativeRoot/相对路径（POSIX），对应上游 collectDirectoryFiles。
func CollectDirectoryFiles(root, relativeRoot string) (map[string]string, error) {
	files := make(map[string]string)
	if _, err := os.Stat(root); err != nil {
		return files, nil
	}
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if skipGitDir(d) {
			return filepath.SkipDir
		}
		if d.IsDir() {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		key := filepath.ToSlash(filepath.Join(relativeRoot, rel))
		files[key] = string(data)
		return nil
	})
	return files, err
}

// checkoutForDownload 返回可用的克隆目录：优先复用 probe 保留的
// git 克隆，否则重新克隆一次。返回的 cleanup 必须调用。
func checkoutForDownload(ctx context.Context, src *Source, probe *ProbeResult) (string, func(), error) {
	if probe != nil && probe.checkout != nil {
		return probe.checkout.dir, func() {}, nil
	}
	checkout, err := cloneRegistry(ctx, src)
	if err != nil {
		return "", nil, err
	}
	return checkout.dir, func() { checkout.Close() }, nil
}

// templateDestRel 返回模板条目的安装目录（相对项目根）。
// spec 直接装进 .sunshine/spec；skill 按 id 分目录装进
// .agents/skills/<id>（每个 skill 自成目录，避免多个 skill 的
// SKILL.md 互相覆盖）；command 按源形态决定（见 installTemplate）；
// full 装进项目根。
func templateDestRel(t *SpecTemplate) string {
	switch t.Type {
	case "skill":
		return filepath.ToSlash(filepath.Join(InstallPaths["skill"], t.ID))
	case "command":
		return filepath.ToSlash(filepath.Join(InstallPaths["command"], t.ID))
	case "full":
		return "."
	default: // spec 与未知类型
		return InstallPaths["spec"]
	}
}

// DownloadTemplateByID 按 id 从 registry 安装一条模板，
// 对应上游 downloadTemplateById。destDirOverride 为空时按
// INSTALL_PATHS 决定目标。返回 skipped 表示目标已存在且策略为 skip。
func DownloadTemplateByID(ctx context.Context, cwd string, src *Source, probe *ProbeResult, templateID string, strategy TemplateStrategy, destDirOverride string) (installed bool, skipped bool, err error) {
	var templates []SpecTemplate
	if probe != nil {
		templates = probe.Templates
	} else {
		p := ProbeRegistryIndex(ctx, src)
		defer p.Close()
		if p.Err != nil {
			return false, false, p.Err
		}
		if p.IsNotFound {
			return false, false, NewError(ErrKindNotFound, "registry 没有 index.json，请去掉 --template 走直接下载模式")
		}
		templates = p.Templates
		probe = p
	}
	t := FindTemplate(templates, templateID)
	if t == nil {
		return false, false, NewError(ErrKindNotFound, "模板 %q 不存在，可选：%s", templateID, strings.Join(TemplateIDs(templates), "、"))
	}
	installed, skipped, _, err = installTemplate(ctx, cwd, src, probe, t, strategy, destDirOverride)
	return installed, skipped, err
}

// installTemplate 把一条已解析的模板条目装进项目。
// 模板 path 按 registry 根（index.json 所在目录）解析——与上游
// 按仓库根解析不同：上游对带 subdir 的 registry 会取错路径
// （如 gh:mindfold-ai/Trellis/marketplace 的 specs/ 在仓库根不存在，
// 实查 trellis-source 仓库根下无 specs/ 目录），Go 版以 registry 根为准。
// 返回安装的文件（相对项目根的 POSIX 路径）。
func installTemplate(ctx context.Context, cwd string, src *Source, probe *ProbeResult, t *SpecTemplate, strategy TemplateStrategy, destDirOverride string) (installed bool, skipped bool, files []string, err error) {
	if t.Type == "workflow" {
		return false, false, nil, fmt.Errorf("工作流模板请用 sunshine workflow --template %s 安装，不要用 --registry", t.ID)
	}
	destRel := templateDestRel(t)
	if destDirOverride != "" {
		destRel = destDirOverride
	}
	destDir := filepath.Join(cwd, filepath.FromSlash(destRel))

	checkoutDir, cleanup, err := checkoutForDownload(ctx, src, probe)
	if err != nil {
		return false, false, nil, err
	}
	defer cleanup()

	root, err := registryRoot(checkoutDir, src)
	if err != nil {
		return false, false, nil, err
	}
	srcPath, err := resolveInsideRoot(root, t.Path, "模板")
	if err != nil {
		return false, false, nil, err
	}
	st, err := os.Stat(srcPath)
	if err != nil {
		return false, false, nil, NewError(ErrKindPathNotFound, "模板 %q 的路径 %q 在 registry 中不存在", t.ID, t.Path)
	}
	if st.IsDir() {
		var ok bool
		if destRel == "." {
			ok, err = copyWithStrategyRoot(srcPath, destDir, strategy)
		} else {
			ok, err = copyWithStrategy(srcPath, destDir, strategy)
		}
		if err != nil {
			return false, false, nil, err
		}
		if !ok {
			return false, true, nil, nil
		}
		rel, err := collectFiles(srcPath)
		if err != nil {
			return false, false, nil, err
		}
		return true, false, joinRelPaths(destRel, rel), nil
	}
	// 模板 path 指向单个文件（如 command 的 .md）：目标按类型决定。
	destFile := destDir
	destRelFile := destRel
	if t.Type == "command" {
		destFile = filepath.Join(filepath.Dir(destDir), filepath.Base(srcPath))
		destRelFile = filepath.ToSlash(filepath.Join(filepath.Dir(destRel), filepath.Base(srcPath)))
	}
	ok, err := copySingleFileWithStrategy(srcPath, destFile, strategy)
	if err != nil {
		return false, false, nil, err
	}
	if !ok {
		return false, true, nil, nil
	}
	return true, false, []string{destRelFile}, nil
}

// joinRelPaths 把相对源目录的路径拼成相对项目根的路径。
func joinRelPaths(destRel string, rel []string) []string {
	if destRel == "." {
		return rel
	}
	out := make([]string, 0, len(rel))
	for _, r := range rel {
		out = append(out, filepath.ToSlash(filepath.Join(destRel, r)))
	}
	return out
}

// directSourceRoot 返回直接下载模式的源目录：registry 根下若存在
// marketplace/specs/，以它为模板（对应官方模板仓库结构）；
// 否则以 registry 根本身为模板（私有 spec 仓库）。
func directSourceRoot(checkoutDir string, src *Source) (string, error) {
	root, err := registryRoot(checkoutDir, src)
	if err != nil {
		return "", err
	}
	nested := filepath.Join(root, "marketplace", "specs")
	if st, err := os.Stat(nested); err == nil && st.IsDir() {
		return nested, nil
	}
	return root, nil
}

// DownloadRegistryDirect 把 registry 内容直接装进 spec 目录
// （无 index.json 时的直接下载模式），对应上游 downloadRegistryDirect。
// destDirOverride 为空时目标为 .sunshine/spec。
func DownloadRegistryDirect(ctx context.Context, cwd string, src *Source, probe *ProbeResult, strategy TemplateStrategy, destDirOverride string) (installed bool, skipped bool, files []string, err error) {
	destRel := InstallPaths["spec"]
	if destDirOverride != "" {
		destRel = destDirOverride
	}
	destDir := filepath.Join(cwd, filepath.FromSlash(destRel))

	checkoutDir, cleanup, err := checkoutForDownload(ctx, src, probe)
	if err != nil {
		return false, false, nil, err
	}
	defer cleanup()

	srcRoot, err := directSourceRoot(checkoutDir, src)
	if err != nil {
		return false, false, nil, err
	}
	ok, err := copyWithStrategy(srcRoot, destDir, strategy)
	if err != nil {
		return false, false, nil, err
	}
	if !ok {
		return false, true, nil, nil
	}
	rel, err := collectFiles(srcRoot)
	if err != nil {
		return false, false, nil, err
	}
	return true, false, joinRelPaths(destRel, rel), nil
}

// FetchRegistrySpecTemplates 把直连型 registry 的 spec 拉到临时目录，
// 返回 .sunshine/spec/** 下的文件集合（键为相对项目根的 POSIX 路径），
// 对应上游 fetchRegistrySpecTemplates。失败返回错误，不打印。
func FetchRegistrySpecTemplates(ctx context.Context, src *Source) (map[string]string, error) {
	tmpRoot, err := os.MkdirTemp("", "sunshine-registry-spec-")
	if err != nil {
		return nil, fmt.Errorf("创建临时目录失败：%w", err)
	}
	defer os.RemoveAll(tmpRoot)
	_, _, _, err = DownloadRegistryDirect(ctx, tmpRoot, src, nil, StrategyOverwrite, "")
	if err != nil {
		return nil, err
	}
	return CollectDirectoryFiles(filepath.Join(tmpRoot, ".sunshine", "spec"), ".sunshine/spec")
}
