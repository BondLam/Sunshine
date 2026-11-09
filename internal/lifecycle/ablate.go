package lifecycle

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// sunshine ablate / restore：临时移除项目全部 Sunshine 表面（可逆）
// 与精确恢复。对照 packages/cli/src/commands/ablate.ts。
//
//   - 恢复事务在项目外（SUNSHINE_ABLATION_STATE_ROOT，未设时默认
//     ~/.sunshine/ablations/v1），含 state.json + backup/；
//   - 流程 stage → apply → verify → applied，失败回滚；
//   - 已 ablate 或中断的项目先提示 sunshine restore；
//   - 非交互时拒绝交互确认；
//   - 只动托管领地（isManagedDirectoryTerritory /
//     collectPrunableDirectories / buildAblationEntries）。

// AblateOptions 是 ablate 的选项。
type AblateOptions struct {
	Yes     bool
	DryRun  bool
	Confirm func(message string) (bool, error)
}

// RestoreOptions 是 restore 的选项。
type RestoreOptions struct {
	Yes     bool
	DryRun  bool
	Confirm func(message string) (bool, error)
}

func isSunshinePath(relativePath string) bool {
	return relativePath == SunshineDir || strings.HasPrefix(relativePath, SunshineDir+"/")
}

func backupPathFor(relativePath string) string {
	return "backup/" + relativePath
}

func relativeFromRoot(projectRoot, absPath string) string {
	rel, err := filepath.Rel(projectRoot, absPath)
	if err != nil {
		return absPath
	}
	return filepath.ToSlash(rel)
}

// isManagedDirectoryTerritory 判断一个相对目录是否属于托管领地。
// .sunshine/ 自身不算（它整体另有条目）；其余只要与任一托管目录
// （除 .sunshine/ 外）有包含关系就算。
func isManagedDirectoryTerritory(relativeDir string) bool {
	if isSunshinePath(relativeDir) {
		return false
	}
	for _, managedDir := range AllManagedDirs {
		if managedDir == SunshineDir {
			continue
		}
		if relativeDir == managedDir ||
			strings.HasPrefix(relativeDir, managedDir+"/") ||
			strings.HasPrefix(managedDir, relativeDir+"/") {
			return true
		}
	}
	return false
}

// collectPrunableDirectories 预测删除全部计划条目后会变空的目录。
func collectPrunableDirectories(projectRoot string, plan RemovalPlan) []string {
	deleted := map[string]bool{}
	for _, entry := range plan.Deletions {
		if entry.Missing || isSunshinePath(entry.PosixPath) {
			continue
		}
		abs, err := filepath.Abs(entry.AbsPath)
		if err != nil {
			continue
		}
		deleted[abs] = true
	}
	candidates := map[string]bool{}
	for deletedPath := range deleted {
		current := filepath.Dir(deletedPath)
		for current != projectRoot && strings.HasPrefix(current, projectRoot+string(filepath.Separator)) {
			relative := relativeFromRoot(projectRoot, current)
			if !isManagedDirectoryTerritory(relative) {
				break
			}
			candidates[current] = true
			current = filepath.Dir(current)
		}
	}
	memo := map[string]bool{}
	var willBeEmpty func(directory string) bool
	willBeEmpty = func(directory string) bool {
		if cached, ok := memo[directory]; ok {
			return cached
		}
		stat, err := lstatIfPresent(directory)
		if err != nil || stat == nil || !stat.IsDir() || stat.Mode()&os.ModeSymlink != 0 {
			memo[directory] = false
			return false
		}
		entries, err := os.ReadDir(directory)
		if err != nil {
			memo[directory] = false
			return false
		}
		empty := true
		for _, entry := range entries {
			child := filepath.Join(directory, entry.Name())
			childAbs, err := filepath.Abs(child)
			if err == nil && deleted[childAbs] {
				continue
			}
			childStat, err := lstatIfPresent(child)
			if err != nil || childStat == nil || !childStat.IsDir() || childStat.Mode()&os.ModeSymlink != 0 || !candidates[child] || !willBeEmpty(child) {
				empty = false
				break
			}
		}
		memo[directory] = empty
		return empty
	}
	var result []string
	for directory := range candidates {
		if willBeEmpty(directory) {
			result = append(result, relativeFromRoot(projectRoot, directory))
		}
	}
	sort.Slice(result, func(i, j int) bool {
		return strings.Count(result[i], "/") > strings.Count(result[j], "/")
	})
	return result
}

func addAblationEntry(entries []AblationEntry, seen map[string]bool, relativePath string, pre, expectedAblated Fingerprint) []AblationEntry {
	if seen[relativePath] {
		return entries
	}
	seen[relativePath] = true
	entry := AblationEntry{RelativePath: relativePath, Pre: pre, ExpectedAblated: expectedAblated}
	if pre.Kind != "absent" {
		entry.BackupPath = backupPathFor(relativePath)
	}
	return append(entries, entry)
}

// buildAblationEntries 由移除计划建事务条目：
// 清单删除项（.sunshine/ 除外）+ 混合文件清理项 +
// 待剪空目录 + 整个 .sunshine/ 目录。
func buildAblationEntries(projectRoot string, plan RemovalPlan, prunableDirectories []string) ([]AblationEntry, error) {
	var entries []AblationEntry
	seen := map[string]bool{}
	for _, deletion := range plan.Deletions {
		if isSunshinePath(deletion.PosixPath) {
			continue
		}
		pre, err := FingerprintPath(deletion.AbsPath)
		if err != nil {
			return nil, err
		}
		if pre.Kind == "directory" {
			return nil, fmt.Errorf("清单条目是目录而非托管文件：%s", deletion.PosixPath)
		}
		entries = addAblationEntry(entries, seen, deletion.PosixPath, pre, Fingerprint{Kind: "absent"})
	}
	for _, modification := range plan.Modifications {
		pre, err := FingerprintPath(modification.AbsPath)
		if err != nil {
			return nil, err
		}
		if pre.Kind != "file" {
			return nil, fmt.Errorf("混合托管路径不是常规文件：%s", modification.PosixPath)
		}
		entries = addAblationEntry(entries, seen, modification.PosixPath, pre,
			ExpectedFileFingerprint([]byte(modification.Result.Content), pre.Mode))
	}
	for _, relativeDir := range prunableDirectories {
		pre, err := FingerprintPath(filepath.Join(projectRoot, filepath.FromSlash(relativeDir)))
		if err != nil {
			return nil, err
		}
		entries = addAblationEntry(entries, seen, relativeDir, pre, Fingerprint{Kind: "absent"})
	}
	sunshinePath := filepath.Join(projectRoot, SunshineDir)
	pre, err := FingerprintPath(sunshinePath)
	if err != nil {
		return nil, err
	}
	entries = addAblationEntry(entries, seen, SunshineDir, pre, Fingerprint{Kind: "absent"})
	return entries, nil
}

// applyAblationPlan 执行切除：先写清理后的混合文件，
// 再删清单文件，最后整个 .sunshine/ 目录与待剪空目录。
func applyAblationPlan(projectRoot string, plan RemovalPlan, prunableDirectories []string, tx *LoadedAblationTransaction) error {
	preByPath := map[string]Fingerprint{}
	for _, entry := range tx.State.Entries {
		preByPath[entry.RelativePath] = entry.Pre
	}
	for _, modification := range plan.Modifications {
		if err := writeFileAtomic(modification.AbsPath, []byte(modification.Result.Content), 0o644); err != nil {
			return err
		}
		if pre, ok := preByPath[modification.PosixPath]; ok && pre.Kind == "file" {
			_ = os.Chmod(modification.AbsPath, os.FileMode(pre.Mode))
		}
	}
	for _, deletion := range plan.Deletions {
		if deletion.Missing || isSunshinePath(deletion.PosixPath) {
			continue
		}
		stat, err := os.Lstat(deletion.AbsPath)
		if err != nil {
			return err
		}
		if stat.IsDir() && stat.Mode()&os.ModeSymlink == 0 {
			return fmt.Errorf("拒绝删除托管目录：%s", deletion.PosixPath)
		}
		if err := os.Remove(deletion.AbsPath); err != nil {
			return err
		}
	}
	sunshinePath := filepath.Join(projectRoot, SunshineDir)
	if stat, err := lstatIfPresent(sunshinePath); err != nil {
		return err
	} else if stat != nil {
		if stat.Mode()&os.ModeSymlink != 0 {
			if err := os.Remove(sunshinePath); err != nil {
				return err
			}
		} else if err := os.RemoveAll(sunshinePath); err != nil {
			return err
		}
	}
	for _, relativeDir := range prunableDirectories {
		directory := filepath.Join(projectRoot, filepath.FromSlash(relativeDir))
		entries, err := os.ReadDir(directory)
		if err == nil && len(entries) == 0 {
			_ = os.Remove(directory)
		}
	}
	return nil
}

// rollbackFailedAblation 回滚一次失败的切除，永不正常返回：
// 回滚成功时重抛原始错误；回滚也不完整时抛包装错误，
// 并把事务状态标为 conflict（preparing/restoring 态除外）。
func rollbackFailedAblation(tx *LoadedAblationTransaction, originalError error, lockHeld bool) error {
	if err := RollbackAblationTransaction(tx, lockHeld); err != nil {
		if tx.State.Status != AblationPreparing && tx.State.Status != AblationRestoring {
			_ = TransitionAblationState(tx, AblationStatusConflict)
		}
		return fmt.Errorf("切除失败，且自动回滚未完成。恢复状态保留在 %s。原始错误：%s。回滚错误：%s",
			tx.Paths.TransactionDir, originalError, err)
	}
	return originalError
}

func renderAblatePlan(plan RemovalPlan, prunableDirectories []string, transactionDir string) {
	deletions := 0
	for _, entry := range plan.Deletions {
		if !entry.Missing && !isSunshinePath(entry.PosixPath) {
			deletions++
		}
	}
	fmt.Println("\nSunshine 完整切除计划")
	fmt.Println()
	fmt.Printf("将移除（%d 个托管条目）：\n", deletions+1)
	for _, entry := range plan.Deletions {
		if entry.Missing || isSunshinePath(entry.PosixPath) {
			continue
		}
		fmt.Printf("  - %s\n", entry.PosixPath)
	}
	fmt.Printf("  - %s/\n", SunshineDir)
	if len(plan.Modifications) > 0 {
		fmt.Printf("\n将清理（%d 个混合文件）：\n", len(plan.Modifications))
		for _, entry := range plan.Modifications {
			fmt.Printf("  ~ %s\n", entry.PosixPath)
		}
	}
	if len(prunableDirectories) > 0 {
		fmt.Printf("\n%d 个变空的托管目录将被剪掉。\n", len(prunableDirectories))
	}
	fmt.Printf("\n恢复事务：%s\n", transactionDir)
	fmt.Println("恢复副本保存了 .sunshine 任务 / 规范 / 工作区的精确字节，其中可能含有用户自写的敏感文本。")
	fmt.Println("该私有事务会保留到验证恢复完成为止。")
	fmt.Println()
}

// Ablate 执行切除流程。
func Ablate(cwd, version string, opts AblateOptions) error {
	if err := AssertNotHomedir(cwd, "ablate"); err != nil {
		return err
	}
	projectRoot := CanonicalProjectRoot(cwd)
	stateRoot := GetAblationStateRoot()
	if err := AssertExternalStateRoot(projectRoot, stateRoot); err != nil {
		return err
	}
	transactionPaths, err := GetTransactionPaths(projectRoot, stateRoot)
	if err != nil {
		return err
	}
	if st, _ := lstatIfPresent(transactionPaths.TransactionDir); st != nil {
		return fmt.Errorf("该项目已切除或上一次切除被中断，请先运行 sunshine restore")
	}
	sunshinePath := filepath.Join(projectRoot, SunshineDir)
	if st, _ := lstatIfPresent(sunshinePath); st == nil {
		fmt.Println("当前项目未安装 Sunshine。")
		return nil
	}
	hashes, err := LoadHashes(projectRoot)
	if err != nil {
		return err
	}
	if len(hashes) == 0 {
		return fmt.Errorf("Sunshine 清单缺失或不是 v2 版本，请先运行当前版本的 sunshine update 后再切除")
	}
	configured := ConfiguredPlatformIDs(hashes)
	pruned, err := PruneOrphanManifestKeys(projectRoot, configured, hashes, false)
	if err != nil {
		return fmt.Errorf("清理清单孤儿条目失败：%w", err)
	}
	if len(pruned.Pruned) > 0 {
		fmt.Printf("已在内存中清理 %d 条孤儿清单条目。\n", len(pruned.Pruned))
	}
	plan, err := BuildRemovalPlan(projectRoot, pruned.Hashes, true)
	if err != nil {
		return fmt.Errorf("%s 请把托管文件恢复为 Sunshine 生成的形态，或删除其过期的清单条目后重试。", err)
	}
	prunableDirectories := collectPrunableDirectories(projectRoot, plan)
	renderAblatePlan(plan, prunableDirectories, transactionPaths.TransactionDir)
	if opts.DryRun {
		fmt.Println("试运行——未修改任何文件或恢复状态。")
		return nil
	}
	if !opts.Yes {
		if !isInteractiveTerminal() {
			return RefuseNonInteractivePrompt("ablate")
		}
		ok, err := confirmOrFalse(opts.Confirm, "暂时移除项目全部 Sunshine 表面？")
		if err != nil {
			return fmt.Errorf("确认切除失败：%w", err)
		}
		if !ok {
			fmt.Println("已取消切除，未修改任何文件。")
			return nil
		}
	}
	entries, err := buildAblationEntries(projectRoot, plan, prunableDirectories)
	if err != nil {
		return err
	}
	err = WithAblationProjectLock(transactionPaths, func() error {
		if st, _ := lstatIfPresent(transactionPaths.TransactionDir); st != nil {
			return fmt.Errorf("该项目已切除或上一次切除被中断，请先运行 sunshine restore")
		}
		tx, err := StageAblationTransaction(StageAblationInput{
			ProjectRoot:         projectRoot,
			SunshineVersion:     version,
			ConfiguredPlatforms: configured,
			Manifest:            pruned.Hashes,
			Entries:             entries,
		}, true)
		if err != nil {
			return err
		}
		if err := applyAblationPlan(projectRoot, plan, prunableDirectories, tx); err != nil {
			return rollbackFailedAblation(tx, err, true)
		}
		if err := VerifyAblatedState(tx); err != nil {
			return rollbackFailedAblation(tx, err, true)
		}
		return TransitionAblationState(tx, AblationApplied)
	})
	if err != nil {
		return err
	}
	fmt.Println("已完全切除当前项目的 Sunshine。")
	fmt.Println("开始新的 agent 会话后再对比行为。")
	fmt.Println("Git 可能显示 Sunshine 托管文件的删除；全局 CLI、channel 日志与宿主会话记录未被删除。")
	return nil
}

func renderRestorePlan(tx *LoadedAblationTransaction) {
	fmt.Println("\nSunshine 恢复计划")
	fmt.Println()
	fmt.Printf("将从 %s 恢复 %d 个托管路径。\n", tx.Paths.TransactionDir, len(tx.State.Entries))
	fmt.Printf("事务由 Sunshine %s 创建于 %s。\n\n", tx.State.SunshineVersion, tx.State.CreatedAt)
}

// Restore 执行精确恢复流程。
func Restore(cwd string, opts RestoreOptions) error {
	if err := AssertNotHomedir(cwd, "restore"); err != nil {
		return err
	}
	projectRoot := CanonicalProjectRoot(cwd)
	tx, err := LoadAblationTransaction(projectRoot)
	if err != nil {
		return err
	}
	if tx == nil {
		fmt.Println("当前项目没有 Sunshine 切除事务。")
		return nil
	}
	renderRestorePlan(tx)
	if !opts.DryRun && !opts.Yes {
		if !isInteractiveTerminal() {
			return RefuseNonInteractivePrompt("restore")
		}
		ok, err := confirmOrFalse(opts.Confirm, "恢复切除前的 Sunshine 状态（精确恢复）？")
		if err != nil {
			return fmt.Errorf("确认恢复失败：%w", err)
		}
		if !ok {
			fmt.Println("已取消恢复，未修改任何文件。")
			return nil
		}
	}
	if err := RestoreAblationTransaction(tx, opts.DryRun, !opts.DryRun); err != nil {
		return err
	}
	if opts.DryRun {
		fmt.Println("试运行——恢复无冲突，未修改任何文件。")
		return nil
	}
	fmt.Println("已精确恢复项目 Sunshine 状态。")
	fmt.Println("开始新的 agent 会话后继续使用 Sunshine。")
	return nil
}
