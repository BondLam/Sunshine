package lifecycle

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// 托管移除计划（对照 utils/managed-removal.ts）。
//
// 清单是所有权边界：只删清单列出的托管内容，清单外的文件永不碰。
// 结构化配置文件（settings.json / hooks.json / config.toml）
// 先过清理器，只剥离 Sunshine 字段、保留用户邻居；清理后
// 只剩空壳的才删文件。

// PlannedDeletion 是一条计划删除（文件缺失时只记账不删）。
type PlannedDeletion struct {
	PosixPath string
	AbsPath   string
	Missing   bool
}

// PlannedModification 是一条计划修改（混合文件清理后的新内容）。
type PlannedModification struct {
	PosixPath string
	AbsPath   string
	Reason    string
	Result    ScrubResult
}

// RemovalPlan 是托管移除计划。
type RemovalPlan struct {
	Deletions         []PlannedDeletion
	Modifications     []PlannedModification
	RemoveSunshineDir bool
	AllPosixPaths     []string
}

// structuredFileSpec 是结构化文件的清理规格。
type structuredFileSpec struct {
	posixPath string
	reason    string
	scrub     func(content string) ScrubResult
}

// structuredFileSpecs 是 Go 版的结构化文件登记（22 个已接平台
// 实际写出的结构化文件；新平台接入时在此补登记）。
func structuredFileSpecs() map[string]structuredFileSpec {
	specs := []structuredFileSpec{
		{
			posixPath: ".claude/settings.json",
			reason:    "剥离 Sunshine hooks，保留用户字段",
			scrub:     func(content string) ScrubResult { return scrubHooksJSON(content, "nested") },
		},
		{
			posixPath: ".codex/hooks.json",
			reason:    "剥离 Sunshine hooks，保留用户字段",
			scrub:     func(content string) ScrubResult { return scrubHooksJSON(content, "nested") },
		},
		{
			posixPath: ".cursor/hooks.json",
			reason:    "剥离 Sunshine hooks，保留用户字段",
			scrub:     func(content string) ScrubResult { return scrubHooksJSON(content, "flat") },
		},
		{
			posixPath: ".codex/config.toml",
			reason:    "移除 Sunshine 的 project_doc_fallback_filenames 与说明",
			scrub:     scrubCodexConfigToml,
		},
		{
			posixPath: ".gemini/settings.json",
			reason:    "剥离 Sunshine hooks，保留用户字段",
			scrub:     func(content string) ScrubResult { return scrubHooksJSON(content, "nested") },
		},
		{
			posixPath: ".qoder/settings.json",
			reason:    "剥离 Sunshine hooks，保留用户字段",
			scrub:     func(content string) ScrubResult { return scrubHooksJSON(content, "nested") },
		},
		{
			posixPath: ".trae/hooks.json",
			reason:    "剥离 Sunshine hooks，保留用户字段",
			scrub:     func(content string) ScrubResult { return scrubHooksJSON(content, "nested") },
		},
		{
			posixPath: ".zcode/config.json",
			reason:    "剥离 Sunshine hooks，保留用户字段",
			scrub:     scrubZcodeConfigJSON,
		},
		{
			posixPath: ".snow/hooks/onSessionStart.json",
			reason:    "剥离 Sunshine hooks，保留用户字段",
			scrub:     scrubSnowHookJSON,
		},
		{
			posixPath: ".snow/hooks/onUserMessage.json",
			reason:    "剥离 Sunshine hooks，保留用户字段",
			scrub:     scrubSnowHookJSON,
		},
		{
			posixPath: ".snow/hooks/beforeSubAgentStart.json",
			reason:    "剥离 Sunshine hooks，保留用户字段",
			scrub:     scrubSnowHookJSON,
		},
		{
			posixPath: ".opencode/package.json",
			reason:    "移除 Sunshine 的 @opencode-ai/plugin 依赖，保留用户字段",
			scrub:     scrubOpencodePackageJSON,
		},
		{
			posixPath: ".pi/settings.json",
			reason:    "移除 Sunshine 的扩展与提示词注册，保留用户字段",
			scrub:     scrubPiSettingsJSON,
		},
		{
			posixPath: ".codebuddy/settings.json",
			reason:    "剥离 Sunshine hooks，保留用户字段",
			scrub:     func(content string) ScrubResult { return scrubHooksJSON(content, "nested") },
		},
		{
			posixPath: ".factory/settings.json",
			reason:    "剥离 Sunshine hooks，保留用户字段",
			scrub:     func(content string) ScrubResult { return scrubHooksJSON(content, "nested") },
		},
		{
			posixPath: ".github/copilot/hooks.json",
			reason:    "剥离 Sunshine hooks，保留用户字段",
			scrub:     func(content string) ScrubResult { return scrubHooksJSON(content, "flat") },
		},
		{
			posixPath: ".github/hooks/sunshine.json",
			reason:    "剥离 Sunshine hooks，保留用户字段",
			scrub:     func(content string) ScrubResult { return scrubHooksJSON(content, "flat") },
		},
		{
			posixPath: ".github/copilot-instructions.md",
			reason:    "剥离 Sunshine 托管块，保留块外用户内容",
			scrub:     scrubCopilotInstructions,
		},
	}
	out := make(map[string]structuredFileSpec, len(specs))
	for _, s := range specs {
		out[s.posixPath] = s
	}
	return out
}

// BuildRemovalPlan 按清单建移除计划。strict 为真时（ablate 用）
// 校验每条清单路径不逃出项目、结构化文件必须是常规文件，
// 且清理必须确实移掉点东西（否则报错，拒绝在无法证明时动手）；
// 为假时（uninstall 用）沿用上游的宽松语义。
func BuildRemovalPlan(cwd string, hashes map[string]string, strict bool) (RemovalPlan, error) {
	structured := structuredFileSpecs()
	allPosixPaths := make([]string, 0, len(hashes))
	for k := range hashes {
		allPosixPaths = append(allPosixPaths, k)
	}
	sort.Strings(allPosixPaths)

	var deletions []PlannedDeletion
	var modifications []PlannedModification
	for _, posixPath := range allPosixPaths {
		var absPath string
		var stat os.FileInfo
		if strict {
			p, err := AssertSafeManagedPath(cwd, posixPath)
			if err != nil {
				return RemovalPlan{}, err
			}
			absPath = p
			stat, err = lstatIfPresent(absPath)
			if err != nil {
				return RemovalPlan{}, err
			}
		} else {
			absPath = filepath.Join(cwd, filepath.FromSlash(posixPath))
		}
		spec, isStructured := structured[posixPath]
		if !isStructured {
			missing := false
			if strict {
				missing = stat == nil
			} else {
				_, err := os.Stat(absPath)
				missing = os.IsNotExist(err)
			}
			deletions = append(deletions, PlannedDeletion{PosixPath: posixPath, AbsPath: absPath, Missing: missing})
			continue
		}

		if strict {
			if stat == nil || stat.Mode()&os.ModeSymlink != 0 || !stat.Mode().IsRegular() {
				deletions = append(deletions, PlannedDeletion{PosixPath: posixPath, AbsPath: absPath, Missing: stat == nil})
				continue
			}
		} else if _, err := os.Stat(absPath); os.IsNotExist(err) {
			deletions = append(deletions, PlannedDeletion{PosixPath: posixPath, AbsPath: absPath, Missing: true})
			continue
		}

		content, err := os.ReadFile(absPath)
		if err != nil {
			return RemovalPlan{}, fmt.Errorf("读取 %s 失败：%w", posixPath, err)
		}
		result := spec.scrub(string(content))
		if strict && !result.FullyEmpty && result.Content == string(content) {
			return RemovalPlan{}, fmt.Errorf("无法证明已从结构化文件中移除 Sunshine 内容：%s", posixPath)
		}
		if result.FullyEmpty {
			deletions = append(deletions, PlannedDeletion{PosixPath: posixPath, AbsPath: absPath, Missing: false})
		} else {
			modifications = append(modifications, PlannedModification{
				PosixPath: posixPath, AbsPath: absPath, Reason: spec.reason, Result: result,
			})
		}
	}
	return RemovalPlan{
		Deletions:         deletions,
		Modifications:     modifications,
		RemoveSunshineDir: true,
		AllPosixPaths:     allPosixPaths,
	}, nil
}

// RemovalSummary 是执行计划后的计数。
type RemovalSummary struct {
	DeletedFiles  int
	ModifiedFiles int
	DeletedDirs   int
}

// ExecuteRemovalPlan 执行移除计划（uninstall 语义）：
// 改混合文件、删清单文件（单个删失败时尽力而为、不中断）、
// 整个 .sunshine/ 目录一并移除、清理变空的托管目录。
// 对照上游 executeManagedRemovalPlan。
func ExecuteRemovalPlan(cwd string, plan RemovalPlan) RemovalSummary {
	var summary RemovalSummary
	for _, m := range plan.Modifications {
		if err := os.WriteFile(m.AbsPath, []byte(m.Result.Content), 0o644); err == nil {
			summary.ModifiedFiles++
		}
	}
	deletedDirCandidates := map[string]bool{}
	for _, d := range plan.Deletions {
		if d.Missing {
			continue
		}
		if err := os.Remove(d.AbsPath); err == nil {
			summary.DeletedFiles++
			deletedDirCandidates[filepath.Dir(d.PosixPath)] = true
		}
	}
	if plan.RemoveSunshineDir {
		sunshineDir := filepath.Join(cwd, SunshineDir)
		if st, err := lstatIfPresent(sunshineDir); err == nil && st != nil {
			if err := os.RemoveAll(sunshineDir); err == nil {
				summary.DeletedDirs++
			}
		}
	}
	for dirPosix := range deletedDirCandidates {
		if dirPosix == "." || dirPosix == "" {
			continue
		}
		cleanupEmptyDirs(cwd, dirPosix)
	}
	// 变空的托管目录本身也移除（最深的先），再沿父级上行清理。
	sortedManaged := append([]string(nil), AllManagedDirs...)
	sort.Slice(sortedManaged, func(i, j int) bool {
		return strings.Count(sortedManaged[i], "/") > strings.Count(sortedManaged[j], "/")
	})
	for _, managedDir := range sortedManaged {
		if managedDir == SunshineDir {
			continue
		}
		abs := filepath.Join(cwd, filepath.FromSlash(managedDir))
		st, err := os.Stat(abs)
		if err != nil || !st.IsDir() {
			continue
		}
		entries, err := os.ReadDir(abs)
		if err != nil || len(entries) != 0 {
			continue
		}
		if err := os.Remove(abs); err != nil {
			continue
		}
		summary.DeletedDirs++
		parentPosix := parentDirPosix(managedDir)
		for parentPosix != "." && parentPosix != "" {
			parentAbs := filepath.Join(cwd, filepath.FromSlash(parentPosix))
			pst, err := os.Stat(parentAbs)
			if err != nil || !pst.IsDir() {
				break
			}
			pentries, err := os.ReadDir(parentAbs)
			if err != nil || len(pentries) != 0 {
				break
			}
			if err := os.Remove(parentAbs); err != nil {
				break
			}
			summary.DeletedDirs++
			parentPosix = parentDirPosix(parentPosix)
		}
	}
	return summary
}

func parentDirPosix(posixPath string) string {
	if idx := strings.LastIndex(posixPath, "/"); idx >= 0 {
		return posixPath[:idx]
	}
	return "."
}

// isManagedPath 报告 dirPath 是否在任一托管目录下（含等于）。
func isManagedPath(dirPath string) bool {
	normalized := strings.ReplaceAll(dirPath, "\\", "/")
	for _, d := range AllManagedDirs {
		if normalized == d || strings.HasPrefix(normalized, d+"/") {
			return true
		}
	}
	return false
}

// isManagedRootDir 报告 dirName 是否是托管根目录本身（不删）。
func isManagedRootDir(dirName string) bool {
	for _, d := range AllManagedDirs {
		if d == dirName {
			return true
		}
	}
	return false
}

// cleanupEmptyDirs 删掉变空的托管子目录（不上删托管根目录），
// 再沿父级上行清理。对照上游 cleanupEmptyDirs；清理是尽力
// 而为，权限或竞态失败时只留目录、不报错。
func cleanupEmptyDirs(cwd, dirPath string) {
	dirPosix := strings.ReplaceAll(dirPath, "\\", "/")
	segments := strings.Split(dirPosix, "/")
	for _, segment := range segments {
		if segment == "" || segment == "." || segment == ".." {
			return
		}
	}
	if strings.HasPrefix(dirPosix, "/") || !isManagedPath(dirPosix) || isManagedRootDir(dirPosix) {
		return
	}
	canonicalCwd, err := filepath.EvalSymlinks(cwd)
	if err != nil {
		canonicalCwd = cwd
	}
	fullPath := filepath.Join(canonicalCwd, filepath.FromSlash(dirPosix))
	rel, err := filepath.Rel(canonicalCwd, fullPath)
	if err != nil || rel == "" || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || filepath.IsAbs(rel) {
		return
	}
	if _, err := os.Stat(fullPath); err != nil {
		return
	}
	canonicalFullPath, err := filepath.EvalSymlinks(fullPath)
	if err != nil {
		return
	}
	canonicalRel, err := filepath.Rel(canonicalCwd, canonicalFullPath)
	if err != nil || canonicalRel == "" || canonicalRel == ".." ||
		strings.HasPrefix(canonicalRel, ".."+string(filepath.Separator)) || filepath.IsAbs(canonicalRel) {
		return
	}
	stat, err := os.Lstat(fullPath)
	if err != nil || !stat.IsDir() || stat.Mode()&os.ModeSymlink != 0 {
		return
	}
	entries, err := os.ReadDir(fullPath)
	if err != nil || len(entries) != 0 {
		return
	}
	if err := os.Remove(fullPath); err != nil {
		return
	}
	parent := parentDirPosix(dirPosix)
	if parent != "." && parent != dirPosix && !isManagedRootDir(parent) {
		cleanupEmptyDirs(cwd, parent)
	}
}
