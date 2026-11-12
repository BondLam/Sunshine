package supervisor

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/BondLam/Sunshine/internal/config"
)

// autoTrustEntries 是可自动信任的顶层 .sunshine/* 条目：
// 用户把 .sunshine/tasks / .sunshine/workspace 持久化为指向外部目录的
// 符号链接时，其下的合法上下文文件会被 cwd jail 拒绝；
// 本模块窄幅自动信任这两个顶层符号链接的目标。
var autoTrustEntries = []string{"tasks", "workspace"}

// ResolveTrustedRoots 解析上下文/agent  containment 检查的
// 额外信任真实路径根（cwd 之外）：
//   - .sunshine/config.yaml 的 channel.worker_guard.trusted_context_dirs；
//   - .sunshine/tasks / .sunshine/workspace 本身是顶层符号链接时的窄幅自动信任
//     （auto_trust_sunshine_symlinks，默认开）。
//
// 注意：Go 版 config 把这两个键放在 worker_guard 下（阶段 3 已建模），
// 上游是放在 channel: 直属下；本函数按 Go 版 config 读取。
func ResolveTrustedRoots(cwd string) []string {
	var roots []string
	autoTrust := true
	cfg, err := config.LoadProject(cwd)
	if err != nil {
		fmt.Fprintf(os.Stderr, "[channel] 读项目配置失败，信任目录只用符号链接自动信任：%v\n", err)
	} else {
		for _, entry := range cfg.Channel.WorkerGuard.TrustedContextDirs {
			resolved := entry
			if !filepath.IsAbs(resolved) {
				resolved = filepath.Join(cwd, entry)
			}
			real, err := filepath.EvalSymlinks(resolved)
			if err != nil {
				fmt.Fprintf(os.Stderr, "[channel] channel.trusted_context_dirs：条目不存在或无效，已跳过：%s\n", entry)
				continue
			}
			roots = append(roots, real)
		}
		autoTrust = cfg.Channel.WorkerGuard.AutoTrustSunshineSymlinks
	}
	if autoTrust {
		roots = append(roots, autoTrustSymlinkRoots(cwd)...)
	}
	// 去重保序。
	seen := map[string]bool{}
	var out []string
	for _, r := range roots {
		if !seen[r] {
			seen[r] = true
			out = append(out, r)
		}
	}
	return out
}

func autoTrustSymlinkRoots(cwd string) []string {
	var roots []string
	for _, name := range autoTrustEntries {
		entryPath := filepath.Join(cwd, ".sunshine", name)
		info, err := os.Lstat(entryPath)
		if err != nil || info.Mode()&os.ModeSymlink == 0 {
			continue
		}
		real, err := filepath.EvalSymlinks(entryPath)
		if err != nil {
			continue
		}
		roots = append(roots, real)
	}
	return roots
}
