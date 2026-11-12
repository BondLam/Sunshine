// Package lifecycle 实现 Sunshine 的项目生命周期命令：uninstall、ablate、
// restore 与 platforms。
//
// 行为对照上游源码（packages/cli/src/commands/uninstall.ts、
// ablate.ts、utils/ablation-store.ts、utils/managed-removal.ts、
// utils/manifest-prune.ts、utils/uninstall-scrubbers.ts、
// configurators/index.ts 的 getConfiguredPlatforms）按需求清单
// 第 2 模块重新实现，不复制源码。
//
// 与上游的关键区别（逐条见 PLAN.md 阶段 4 实施记录 D 组）：
//   - 清单格式沿用 Go 版约定：{"__version": 2, "files": {...}}
//     （上游是 "hashes" 键；Go 版 scaffold / workflow 已定为 "files"）。
//   - 22 个平台已接（claude-code、cursor、codex + 滚动轨道 19 家），
//     结构化清理（scrubber）覆盖各家实际写出的共享配置文件。
//   - Hook 不再是 Python 脚本：settings/hooks.json 里的托管命令是
//     `SUNSHINE_HOOK_PLATFORM=<平台> sunshine hook <事件>`，
//     scrubber 按“命令含 sunshine hook”判定托管条目，而不是
//     按脚本路径匹配。
//   - 根目录 AGENTS.md 不是 Go 版 init 写出的文件（Go 版只写
//     .sunshine/AGENTS.md），因此清单里的根 AGENTS.md 一律视为
//     孤儿键剪掉，不做托管块判定。
//   - 环境变量名前缀统一为 SUNSHINE_：SUNSHINE_ALLOW_HOMEDIR、
//     SUNSHINE_ALLOW_DIRTY_UNINSTALL、SUNSHINE_ABLATION_STATE_ROOT。
package lifecycle

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/BondLam/Sunshine/internal/platforms"
	"golang.org/x/term"
)

// Version 是写入切除事务 state.json 的 Sunshine 版本，由调用方
// （internal/cli）传入，避免本包反向依赖 cli 包。
// CLI 命令把 root.go 的 version 常量传进来。

// SunshineDir 是工程骨架目录名。
const SunshineDir = ".sunshine"

// ManifestName 是模板哈希清单的文件名。
const ManifestName = ".template-hashes.json"

// ManifestVersion 是清单的 schema 版本（与上游 __version: 2 一致）。
const ManifestVersion = 2

// AllManagedDirs 是 Sunshine 管理的全部目录（含 .sunshine/ 本身）。
// 对照上游 ALL_MANAGED_DIRS（[工程目录, ...各平台托管目录]）；
// 共享 .agents/skills/ 由 codex、gemini、dsh、pi、kimi 五家写入。
// Copilot 的 .github/copilot-instructions.md 是托管文件（非目录），
// 为让覆盖测试认领它也列入本表；空目录清理时只删空目录，
// .github 本身不在此列（用户自有工作流等文件不受影响）。
var AllManagedDirs = []string{
	SunshineDir,
	".claude",
	".cursor",
	".codex",
	".agents/skills",
	".kiro",
	".gemini",
	".qoder",
	".pi",
	".zcode",
	".zcode/cli/agents",
	".trae",
	".omp",
	".snow",
	".opencode",
	".dsh",
	".reasonix",
	".grok",
	".kimi-code",
	".kilocode",
	".agent",
	".devin",
	".codebuddy",
	".factory",
	".github/copilot",
	".github/agents",
	".github/hooks",
	".github/prompts",
	".github/skills",
	".github/copilot-instructions.md",
}

// stdinIsTTY 判断标准输入是否为交互终端。
func stdinIsTTY() bool {
	return term.IsTerminal(int(os.Stdin.Fd()))
}

// isInteractiveTerminal 是交互确认前的 TTY 判定（默认 stdinIsTTY；
// 测试可替换为替身，与 cli 包的 promptConfirm 变量同一模式）。
var isInteractiveTerminal = stdinIsTTY

// ---------------------------------------------------------------------------
// 主目录守卫（对照 utils/cwd-guard.ts）
// ---------------------------------------------------------------------------

// IsHomeDir 判断 cwd 是否恰好是用户主目录（解析符号链接后比较，
// 只拦精确相等，子目录不拦）。查不到时返回 false，不让一次
// 安全检查崩掉整个命令。
func IsHomeDir(cwd string) bool {
	home, err := os.UserHomeDir()
	if err != nil {
		return false
	}
	realCwd, err := filepath.EvalSymlinks(cwd)
	if err != nil {
		realCwd = cwd
	}
	realHome, err := filepath.EvalSymlinks(home)
	if err != nil {
		realHome = home
	}
	return realCwd == realHome
}

// HomedirBypassEnabled 报告是否设置了主目录绕过环境变量。
func HomedirBypassEnabled() bool {
	return os.Getenv("SUNSHINE_ALLOW_HOMEDIR") == "1"
}

// HomedirGuardMessage 返回主目录守卫触发时的中文说明。
func HomedirGuardMessage(command string) string {
	return fmt.Sprintf(
		"拒绝在用户主目录运行 sunshine %s。\n\n"+
			"Sunshine 管理 .claude/、.codex/ 等平台配置目录，在主目录下这些目录还包含\n"+
			"各 CLI 的运行数据（聊天记录、会话 JSONL、缓存）。在此运行可能误删这些数据。\n\n"+
			"请改在项目目录运行。如确实要在主目录运行，请设置 SUNSHINE_ALLOW_HOMEDIR=1。",
		command)
}

// AssertNotHomedir 在 cwd 是主目录且未绕过时返回守卫错误。
func AssertNotHomedir(cwd, command string) error {
	if IsHomeDir(cwd) && !HomedirBypassEnabled() {
		return errors.New(HomedirGuardMessage(command))
	}
	return nil
}

// RefuseNonInteractivePrompt 返回非交互终端下拒绝确认的错误
// （对照 ablate.ts 的 refuseNonInteractivePrompt）。
func RefuseNonInteractivePrompt(command string) error {
	return fmt.Errorf("拒绝在非交互终端里确认 %s。请传 --yes/-y 确认，或用 --dry-run 预览。", command)
}

// ---------------------------------------------------------------------------
// 模板哈希清单（对照 utils/template-hash.ts；键名按 Go 版约定为 files）
// ---------------------------------------------------------------------------

// ManifestPath 返回项目内清单文件的绝对路径。
func ManifestPath(cwd string) string {
	return filepath.Join(cwd, SunshineDir, ManifestName)
}

// ManifestExists 报告清单文件是否存在。
func ManifestExists(cwd string) bool {
	_, err := os.Stat(ManifestPath(cwd))
	return err == nil
}

// LoadHashes 读清单的 files 表。清单不存在时返回空表；
// 清单损坏（JSON 非法或结构不对）时返回错误——损坏的清单
// 不能悄悄当成空清单，否则 uninstall 会误判为“无清单”。
func LoadHashes(cwd string) (map[string]string, error) {
	data, err := os.ReadFile(ManifestPath(cwd))
	if err != nil {
		if os.IsNotExist(err) {
			return map[string]string{}, nil
		}
		return nil, fmt.Errorf("读取模板哈希清单失败：%w", err)
	}
	var manifest struct {
		Version int               `json:"__version"`
		Files   map[string]string `json:"files"`
	}
	if err := json.Unmarshal(data, &manifest); err != nil {
		return nil, fmt.Errorf("读取模板哈希清单失败：%w", err)
	}
	if manifest.Version != ManifestVersion {
		return nil, fmt.Errorf("模板哈希清单版本不受支持（__version=%d，需要 %d）", manifest.Version, ManifestVersion)
	}
	if manifest.Files == nil {
		return map[string]string{}, nil
	}
	return manifest.Files, nil
}

// SaveHashes 把 files 表写回清单（保留 __version: 2）。
func SaveHashes(cwd string, hashes map[string]string) error {
	manifest, err := json.MarshalIndent(map[string]any{
		"__version": ManifestVersion,
		"files":     hashes,
	}, "", "  ")
	if err != nil {
		return err
	}
	if err := os.WriteFile(ManifestPath(cwd), append(manifest, '\n'), 0o644); err != nil {
		return fmt.Errorf("写入模板哈希清单失败：%w", err)
	}
	return nil
}

// ---------------------------------------------------------------------------
// 托管路径校验（对照 utils/managed-removal.ts）
// ---------------------------------------------------------------------------

// ValidateManagedRelativePath 校验清单键：拒绝空、NUL、反斜杠、
// 绝对路径与含空段 / "." / ".." 的路径。
func ValidateManagedRelativePath(posixPath string) error {
	if posixPath == "" || strings.Contains(posixPath, "\x00") ||
		strings.Contains(posixPath, "\\") || strings.HasPrefix(posixPath, "/") ||
		(len(posixPath) >= 2 && posixPath[1] == ':') {
		return fmt.Errorf("无效的托管清单路径：%q", posixPath)
	}
	for _, segment := range strings.Split(posixPath, "/") {
		if segment == "" || segment == "." || segment == ".." {
			return fmt.Errorf("无效的托管清单路径：%q", posixPath)
		}
	}
	return nil
}

// withinProject 报告 candidate 是否在 root 内（含等于 root）。
func withinProject(root, candidate string) bool {
	rel, err := filepath.Rel(root, candidate)
	if err != nil {
		return false
	}
	return rel == "." || (rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) && !filepath.IsAbs(rel))
}

// lstatIfPresent 是安全的 lstat：不存在返回 nil，不存在以外的错误照常返回。
func lstatIfPresent(absPath string) (os.FileInfo, error) {
	info, err := os.Lstat(absPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	return info, nil
}

// AssertSafeManagedPath 校验托管相对路径并返回项目内绝对路径。
// 中间是符号链接时要求其实体仍在项目内；末级符号链接视为不透明
// 的托管条目（不解开）。对照上游 assertSafeManagedPath。
func AssertSafeManagedPath(projectRoot, posixPath string) (string, error) {
	if err := ValidateManagedRelativePath(posixPath); err != nil {
		return "", err
	}
	root, err := filepath.Abs(projectRoot)
	if err != nil {
		return "", err
	}
	segments := strings.Split(posixPath, "/")
	current := root
	for i, segment := range segments {
		current = filepath.Join(current, segment)
		if !withinProject(root, current) {
			return "", fmt.Errorf("托管路径超出项目根目录：%s", posixPath)
		}
		stat, err := lstatIfPresent(current)
		if err != nil {
			return "", err
		}
		if stat == nil || i == len(segments)-1 {
			continue
		}
		if stat.Mode()&os.ModeSymlink != 0 {
			real, err := filepath.EvalSymlinks(current)
			if err != nil {
				return "", err
			}
			if !withinProject(root, real) {
				return "", fmt.Errorf("托管路径经符号链接超出项目根目录：%s", posixPath)
			}
		}
		if !stat.IsDir() && stat.Mode()&os.ModeSymlink == 0 {
			return "", fmt.Errorf("托管路径的父级不是目录：%s", posixPath)
		}
	}
	return current, nil
}

// ---------------------------------------------------------------------------
// 已配置平台判定（对照 configurators/index.ts 的 getConfiguredPlatforms）
// ---------------------------------------------------------------------------

// ConfiguredPlatformIDs 按哈希清单判断已配置平台：清单里有该平台
// 配置目录下条目的即为已配置（避免把原生平台目录误判为已配置）。
// 返回顺序与 internal/platforms 的注册表顺序一致。
func ConfiguredPlatformIDs(hashes map[string]string) []string {
	var ids []string
	for _, p := range platforms.Registry() {
		configDir := p.ConfigDir
		for key := range hashes {
			if key == configDir || strings.HasPrefix(key, configDir+"/") {
				ids = append(ids, p.ID)
				break
			}
		}
	}
	// devin 的 Windsurf 遗留兼容（C 规格 §5.2）：.windsurf/workflows/
	// 下有 sunshine- 前缀的被追踪模板时，同样判定 devin 已配置。
	if !containsString(ids, "devin") {
		for key := range hashes {
			if strings.HasPrefix(key, ".windsurf/workflows/sunshine-") {
				ids = append(ids, "devin")
				break
			}
		}
	}
	return ids
}

func containsString(list []string, want string) bool {
	for _, s := range list {
		if s == want {
			return true
		}
	}
	return false
}

// ---------------------------------------------------------------------------
// 未提交的用户数据（对照上游 uninstall.ts 的同名收集函数）
// ---------------------------------------------------------------------------

// CollectUncommittedSunshineData 列出 .sunshine/ 下用户数据子目录
// （spec、tasks、workspace）里未提交（已修改、已暂存、未跟踪）的
// 文件。不是 git 仓库或 git 不可用时返回空表。
func CollectUncommittedSunshineData(cwd string) []string {
	dirs := []string{
		SunshineDir + "/spec",
		SunshineDir + "/tasks",
		SunshineDir + "/workspace",
	}
	out, err := exec.Command("git", append([]string{"-C", cwd, "status", "--porcelain", "--"}, dirs...)...).Output()
	if err != nil {
		return nil
	}
	var result []string
	for _, line := range strings.Split(string(out), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		// 去掉 2 字符状态码；重命名取 " -> " 之后的新路径。
		if len(line) > 3 {
			line = line[3:]
		}
		if idx := strings.LastIndex(line, " -> "); idx >= 0 {
			line = line[idx+4:]
		}
		result = append(result, strings.TrimSpace(line))
	}
	return result
}

// DirtyUninstallBypassEnabled 报告是否显式绕过了“有未提交数据时
// --yes 拒绝卸载”的保护（对照 dirtyUninstallBypassEnabled）。
func DirtyUninstallBypassEnabled() bool {
	return os.Getenv("SUNSHINE_ALLOW_DIRTY_UNINSTALL") == "1"
}

// ---------------------------------------------------------------------------
// 小工具
// ---------------------------------------------------------------------------

// writeFileAtomic 经同目录临时文件 + 改名写盘（避免写一半的文件）。
func writeFileAtomic(absPath string, content []byte, perm os.FileMode) error {
	dir := filepath.Dir(absPath)
	tmp, err := os.CreateTemp(dir, ".sunshine-atomic-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer func() { _ = os.Remove(tmpName) }()
	if _, err := tmp.Write(content); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Chmod(tmpName, perm); err != nil {
		return err
	}
	return os.Rename(tmpName, absPath)
}

// toPosix 把路径分隔符统一为正斜杠（用于清单键与指纹比较）。
func toPosix(p string) string {
	return filepath.ToSlash(p)
}
