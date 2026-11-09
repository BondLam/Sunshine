package lifecycle

import (
	"fmt"
	"os"
	"path/filepath"
)

// sunshine uninstall：删除项目内全部 Sunshine 文件。
//
// 对照 packages/cli/src/commands/uninstall.ts：
//   - 要求清单齐全：无 .sunshine/ 时直接提示未安装；
//     有 .sunshine/ 但清单缺失或为空时拒绝（无法确定删哪些平台文件）；
//   - 只删清单列出的托管内容，清单外的文件永不删；
//   - 先 pruneOrphanManifestKeys 自愈被污染的清单；
//   - 未提交的用户数据（spec/tasks/workspace）先警告；
//     非交互 --yes 且有未提交数据时拒绝，
//     SUNSHINE_ALLOW_DIRTY_UNINSTALL=1 仅作显式绕过；
//   - $HOME 守卫；--dry-run 预览；删后整个 .sunshine/ 目录一并移除。

// UninstallOptions 是 uninstall 的选项。
// Confirm 由调用方传入（CLI 层用 survey 实现；测试传替身）。
// Confirm 为 nil 且需要交互确认时视为拒绝确认。
type UninstallOptions struct {
	Yes     bool
	DryRun  bool
	Confirm func(message string) (bool, error)
}

// Uninstall 执行卸载流程。
func Uninstall(cwd string, opts UninstallOptions) error {
	if err := AssertNotHomedir(cwd, "uninstall"); err != nil {
		return err
	}
	sunshineDir := filepath.Join(cwd, SunshineDir)
	if st, err := os.Stat(sunshineDir); err != nil || !st.IsDir() {
		fmt.Println("当前项目未安装 Sunshine（未找到 .sunshine/ 目录）。")
		return nil
	}
	if !ManifestExists(cwd) {
		return fmt.Errorf("已找到 .sunshine/ 目录，但清单缺失——无法确定应删除哪些平台文件。如有需要可手动删除 .sunshine/")
	}
	hashes, err := LoadHashes(cwd)
	if err != nil {
		return err
	}
	if len(hashes) == 0 {
		return fmt.Errorf("已找到 .sunshine/ 目录，但清单为空——无法确定应删除哪些平台文件。如有需要可手动删除 .sunshine/")
	}

	configured := ConfiguredPlatformIDs(hashes)
	pruned, err := PruneOrphanManifestKeys(cwd, configured, hashes, !opts.DryRun)
	if err != nil {
		return fmt.Errorf("清理清单孤儿条目失败：%w", err)
	}
	if len(pruned.Pruned) > 0 {
		fmt.Printf("已清理 %d 条孤儿清单条目（用户自有文件，Sunshine 未写过）。\n", len(pruned.Pruned))
	}
	plan, err := BuildRemovalPlan(cwd, pruned.Hashes, false)
	if err != nil {
		return err
	}
	renderUninstallPlan(cwd, plan)

	uncommitted := CollectUncommittedSunshineData(cwd)
	if len(uncommitted) > 0 {
		fmt.Fprintf(os.Stderr, "\n⚠ %d 个未提交文件在 .sunshine/ 下（spec/tasks/workspace），卸载将永久删除且无备份：\n", len(uncommitted))
		for _, p := range uncommitted[:min(20, len(uncommitted))] {
			fmt.Fprintf(os.Stderr, "    %s\n", p)
		}
		if len(uncommitted) > 20 {
			fmt.Fprintf(os.Stderr, "    ……以及另外 %d 个\n", len(uncommitted)-20)
		}
		fmt.Fprintln(os.Stderr, "如需保留，请先提交或暂存。")
	}

	if opts.DryRun {
		fmt.Println("试运行——未修改任何文件。")
		return nil
	}
	if len(uncommitted) > 0 && opts.Yes && !DirtyUninstallBypassEnabled() {
		return fmt.Errorf("拒绝在 --yes 下卸载：.sunshine/ 下有未提交的用户数据（spec/tasks/workspace）。请先提交或暂存，或不带 --yes 交互确认，或设置 SUNSHINE_ALLOW_DIRTY_UNINSTALL=1 绕过")
	}
	if !opts.Yes {
		if !isInteractiveTerminal() {
			return RefuseNonInteractivePrompt("uninstall")
		}
		ok, err := confirmOrFalse(opts.Confirm, "继续卸载？")
		if err != nil {
			return fmt.Errorf("确认卸载失败：%w", err)
		}
		if !ok {
			fmt.Println("已取消卸载，未修改任何文件。")
			return nil
		}
	}

	summary := ExecuteRemovalPlan(cwd, plan)
	fmt.Printf("\n已卸载 Sunshine：删除 %d 个文件，修改 %d 个文件，移除 %d 个目录。\n",
		summary.DeletedFiles, summary.ModifiedFiles, summary.DeletedDirs)
	return nil
}

// renderUninstallPlan 打印两栏卸载计划。
func renderUninstallPlan(cwd string, plan RemovalPlan) {
	fmt.Println("\nSunshine 卸载计划")
	fmt.Println()
	var deletePaths []string
	var missing int
	for _, d := range plan.Deletions {
		if d.Missing {
			missing++
			continue
		}
		deletePaths = append(deletePaths, d.PosixPath)
	}
	fmt.Printf("将删除（%d 项）：\n", len(deletePaths)+1)
	for _, p := range deletePaths {
		fmt.Printf("  - %s\n", p)
	}
	if plan.RemoveSunshineDir {
		if st, err := os.Stat(filepath.Join(cwd, SunshineDir)); err == nil && st.IsDir() {
			fmt.Printf("  - %s/  （整个目录——包括规范、任务 PRD、日志与记忆）\n", SunshineDir)
		}
	}
	if len(plan.Modifications) > 0 {
		fmt.Printf("\n将修改（%d 个文件）：\n", len(plan.Modifications))
		for _, m := range plan.Modifications {
			fmt.Printf("  ~ %s  （%s）\n", m.PosixPath, m.Reason)
		}
	}
	if missing > 0 {
		fmt.Printf("\n（%d 条清单记录在磁盘上已不存在，跳过。）\n", missing)
	}
	fmt.Println()
}

// confirmOrFalse 在 Confirm 为 nil 时返回 false（拒绝确认）。
func confirmOrFalse(confirm func(string) (bool, error), message string) (bool, error) {
	if confirm == nil {
		return false, nil
	}
	return confirm(message)
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
