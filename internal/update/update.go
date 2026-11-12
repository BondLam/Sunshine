package update

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/linrong1982/Sunshine/internal/migrate"
	"github.com/linrong1982/Sunshine/internal/platforms"
)

// LatestVersionFunc 是“最新版”检查（上游 getLatestNpmVersion 的对应实现：
// Go 无 npm，对应为 GitHub Releases 检查；仓库地址待阶段 7 定模块路径后
// 填入）。为 nil 表示未配置；返回的 error 表示离线或检查失败。
// 两种情况都只警告、不中断 update（项目版↔CLI 版比对才是硬门）。
var LatestVersionFunc func() (string, error)

// Options 是 Run 的选项。
type Options struct {
	// CLIVersion 是当前 CLI 内嵌版本（由 CLI 层传入）。
	CLIVersion string
	// DryRun 只打印计划，不写盘。
	DryRun bool
	// Force 冲突文件一律覆盖。
	Force bool
	// SkipAll 冲突文件一律跳过。
	SkipAll bool
	// CreateNew 冲突文件一律写 .new 副本。
	CreateNew bool
	// AllowDowngrade 允许 CLI 比项目旧时仍执行（降级）。
	AllowDowngrade bool
	// Migrate 执行 rename/delete 类迁移。
	Migrate bool
	// Prompter 是交互接口；为 nil 时非交互（冲突默认 Skip）。
	Prompter Prompter
	// Out 是输出目标；为 nil 时用 os.Stdout。
	Out io.Writer
}

// Summary 是 Run 的结果汇总。
type Summary struct {
	ProjectVersion string
	CLIVersion     string
	Added          int
	AutoUpdated    int
	Updated        int
	Skipped        int
	CreatedNew     int
	SafeDeleted    int
	SectionsAdded  int
	BackupDir      string
	UpToDate       bool
}

func (o Options) out() io.Writer {
	if o.Out != nil {
		return o.Out
	}
	return os.Stdout
}

// Run 执行 sunshine update（上游 update() 主流程）：
// 版本比对 → Codex 升级修复 → 完整备份 → 清单孤儿键清理 →
// registry.spec 持续刷新 → 变更分析 → 交互确认 → 写盘 →
// 迁移执行 → 更新 .version。
func Run(cwd string, opts Options) (Summary, error) {
	w := opts.out()
	var summary Summary
	if opts.CLIVersion == "" {
		return summary, errors.New("未传入 CLI 版本（内部错误）")
	}
	summary.CLIVersion = opts.CLIVersion
	if _, err := os.Stat(filepath.Join(cwd, ".sunshine")); err != nil {
		return summary, errors.New("当前目录尚未初始化（缺少 .sunshine/），请先运行 sunshine init")
	}

	fmt.Fprintln(w, "\nSunshine 更新")
	fmt.Fprint(w, "══════════════\n")

	projectVersion := GetInstalledVersion(cwd)
	summary.ProjectVersion = projectVersion
	cliVersion := opts.CLIVersion
	fmt.Fprintf(w, "项目版本：%s\n", projectVersion)
	fmt.Fprintf(w, "CLI 版本：%s\n", cliVersion)
	latest := ""
	if LatestVersionFunc != nil {
		if v, err := LatestVersionFunc(); err == nil {
			latest = v
		} else {
			fmt.Fprintln(w, "最新版：无法获取（离线或检查失败），仅比对项目版与 CLI 版。")
		}
	} else {
		fmt.Fprintln(w, "最新版：未配置检查，仅比对项目版与 CLI 版。")
	}
	if latest != "" {
		fmt.Fprintf(w, "最新版：%s\n", latest)
		if CompareVersions(cliVersion, latest) < 0 {
			fmt.Fprintf(w, "提示：CLI 版本（%s）落后于最新版（%s），请升级 CLI 后再更新项目。\n", cliVersion, latest)
		}
	}
	fmt.Fprintln(w)

	cliVsProject := CompareVersions(cliVersion, projectVersion)
	if cliVsProject < 0 {
		fmt.Fprintf(w, "错误：无法更新：CLI 版本（%s）低于项目版本（%s），这会降级项目！\n", cliVersion, projectVersion)
		if !opts.AllowDowngrade {
			fmt.Fprintln(w, "解决办法：")
			fmt.Fprintln(w, "  1. 升级 CLI 后再更新项目；")
			fmt.Fprintln(w, "  2. 强制降级：sunshine update --allow-downgrade")
			return summary, fmt.Errorf("CLI 版本（%s）低于项目版本（%s），拒绝降级", cliVersion, projectVersion)
		}
		fmt.Fprint(w, "警告：已传 --allow-downgrade，继续降级……\n")
	}

	hashes := LoadHashes(cwd)
	isFirstHashTracking := len(hashes) == 0
	isUnknownVersion := projectVersion == "unknown"
	if isUnknownVersion {
		fmt.Fprintln(w, "警告：未找到版本文件，跳过迁移——请运行 sunshine init 修复。")
		fmt.Fprint(w, "模板更新仍会执行；安全清理仍会执行（哈希校验）。\n")
	}

	// 旧版 Codex 升级检测：必须在清单孤儿键清理之前，
	// 否则清理会误删尚未补装的 Codex 条目。
	codexUpgradeNeeded := NeedsCodexUpgrade(cwd, hashes)
	if codexUpgradeNeeded {
		fmt.Fprintln(w, "检测到旧版 Codex：.agents/skills/ 已追踪但缺少 .codex/，将补建 .codex/。")
	}

	// 清单孤儿键清理（自愈被污染的清单；dry-run 时只计算不写盘）。
	configured := ConfiguredPlatforms(cwd, hashes)
	if codexUpgradeNeeded {
		configured = appendConfigured(configured, "codex")
	}
	prune, err := PruneOrphanManifestKeys(cwd, configured, hashes, MigrationPaths(), !opts.DryRun)
	if err != nil {
		return summary, err
	}
	if len(prune.Pruned) > 0 {
		fmt.Fprintf(w, "已清理 %d 条孤儿清单条目。\n", len(prune.Pruned))
		hashes = prune.Hashes
	}

	// 破坏性版本 + --migrate 时绕过 update.skip。
	registry := migrationRegistry()
	breakingBypass := false
	if opts.Migrate && cliVsProject > 0 && !isUnknownVersion {
		meta := registry.Metadata(projectVersion, cliVersion)
		breakingBypass = meta.Breaking && meta.RecommendMigrate
	}

	// 收集模板（含 Codex 升级修复与 registry.spec 持续刷新）。
	extraPlatforms := []string{}
	if codexUpgradeNeeded {
		extraPlatforms = []string{"codex"}
	}
	templates, skipPaths, err := CollectTemplateFiles(w, cwd, hashes, CollectOptions{
		ExtraPlatforms:   extraPlatforms,
		BypassUpdateSkip: breakingBypass,
	})
	if err != nil {
		return summary, err
	}

	// safe-file-delete 收集（无需 --migrate 也执行；只收集，不执行）。
	templatePaths := map[string]bool{}
	for k := range templates {
		templatePaths[k] = true
	}
	safeDeletes := migrate.CollectSafeFileDeletes(registry.AllMigrations(), cwd, skipPaths, templatePaths, breakingBypass)
	hasSafeDeletes := false
	for _, c := range safeDeletes {
		if c.Action == migrate.SafeDeleteDelete {
			hasSafeDeletes = true
			break
		}
	}

	// 待迁移项（区间筛选）+ 孤儿迁移检测 + 分类。
	var classified migrate.ClassifiedMigrations
	meta := registry.Metadata(projectVersion, cliVersion)
	if !isUnknownVersion {
		pendingMigrations := registry.MigrationsForVersion(projectVersion, cliVersion)
		if orphaned := migrate.FindOrphanMigrations(cwd, registry.AllMigrations(), pendingMigrations); len(orphaned) > 0 {
			fmt.Fprintln(w, "警告：检测到上次更新遗漏的迁移：")
			for _, item := range orphaned {
				fmt.Fprintf(w, "    %s → %s\n", item.From, item.To)
			}
			fmt.Fprintln(w)
			pendingMigrations = append(pendingMigrations, orphaned...)
		}
		classified = migrate.ClassifyMigrations(pendingMigrations, cwd, hashes, templates)
	}
	if classified.Total() > 0 {
		fmt.Fprint(w, "正在分析迁移……\n")
		migrate.PrintMigrationSummary(classified, w)
		// 破坏性硬停：无 --migrate 时拒绝继续（试运行除外）。
		if migrate.ShouldHardStop(classified, meta, opts.Migrate, opts.DryRun, cliVsProject > 0, !isUnknownVersion) {
			fmt.Fprint(w, migrate.HardStopMessage(projectVersion, cliVersion))
			return summary, fmt.Errorf("破坏性变更需要 --migrate（%s → %s）", projectVersion, cliVersion)
		}
		if !opts.Migrate {
			if hint := migrate.MigrateHint(classified); hint != "" {
				fmt.Fprintln(w, hint)
			}
		}
	}
	if len(safeDeletes) > 0 {
		migrate.PrintSafeFileDeleteSummary(safeDeletes, w)
	}

	// 变更分析。
	changes := AnalyzeChanges(cwd, hashes, templates, templates[CopilotInstructionsPath])
	unchangedRepairs := CollectUnchangedFileHashRepairs(changes, hashes)
	PrintChangeSummary(w, cwd, changes)
	if isFirstHashTracking && len(changes.ChangedFiles) > 0 {
		fmt.Fprintln(w, "提示：首次启用哈希追踪的更新。")
		fmt.Fprintln(w, "上面列出的改动文件可能并非你实际改过的；")
		fmt.Fprint(w, "本次更新后，哈希追踪将能准确识别改动。\n")
	}

	isUpgrade := cliVsProject > 0
	isDowngrade := cliVsProject < 0
	isSameVersion := cliVsProject == 0
	hasPendingMigrations := opts.Migrate && classified.PendingCount() > 0

	// 无事可做时仍修复清单、更新版本戳。
	if len(changes.NewFiles) == 0 && len(changes.AutoUpdateFiles) == 0 &&
		len(changes.ChangedFiles) == 0 && !hasPendingMigrations && !hasSafeDeletes {
		if !opts.DryRun && len(unchangedRepairs) > 0 {
			if err := UpdateHashes(cwd, unchangedRepairs); err != nil {
				return summary, err
			}
		}
		summary.UpToDate = true
		if isSameVersion {
			fmt.Fprintln(w, "✓ 已是最新，无需更新。")
		} else {
			if !opts.DryRun {
				if err := WriteVersionFile(cwd, cliVersion); err != nil {
					return summary, err
				}
			}
			if isUpgrade {
				fmt.Fprintf(w, "✓ %s → %s 无需改动文件。\n", projectVersion, cliVersion)
			} else if isDowngrade {
				fmt.Fprintf(w, "✓ %s → %s（降级）无需改动文件。\n", projectVersion, cliVersion)
			}
		}
		return summary, nil
	}

	if isUpgrade {
		fmt.Fprintf(w, "本次将升级：%s → %s\n\n", projectVersion, cliVersion)
	} else if isDowngrade {
		fmt.Fprintf(w, "警告：本次将降级：%s → %s\n\n", projectVersion, cliVersion)
	}
	if cliVsProject > 0 && !isUnknownVersion {
		if meta.Breaking {
			fmt.Fprintln(w, "════════════════════════════════════════════════════════════")
			fmt.Fprintln(w, "⚠️  破坏性变更：请仔细核对上面的变更！")
			if len(meta.Changelog) > 0 {
				fmt.Fprintf(w, "\n%s\n", meta.Changelog[0])
			}
			if meta.RecommendMigrate && !opts.Migrate {
				fmt.Fprintln(w, "\n建议：加 --migrate 完成迁移。")
			}
			fmt.Fprint(w, "════════════════════════════════════════════════════════════\n")
		}
	}

	// 试运行：只打印计划，不写盘。
	if opts.DryRun {
		fmt.Fprintln(w, "[试运行] 未做任何修改。")
		return summary, nil
	}

	// 统一确认（批量 flag 视为已确认；非交互时跳过提问直接继续，
	// 冲突文件默认 Skip）。
	if !opts.Force && !opts.SkipAll && !opts.CreateNew && opts.Prompter != nil {
		proceed, err := opts.Prompter.ConfirmProceed()
		if err != nil {
			return summary, fmt.Errorf("确认更新失败：%w", err)
		}
		if !proceed {
			fmt.Fprintln(w, "更新已取消。")
			return summary, nil
		}
	}

	// 完整备份（先备份再写盘）。
	backupDir, err := CreateFullBackup(cwd)
	if err != nil {
		return summary, err
	}
	if backupDir != "" {
		summary.BackupDir = backupDir
		fmt.Fprintf(w, "\n已创建备份：%s/\n", backupRel(cwd, backupDir))
	}

	// 迁移执行（仅 --migrate；traces 改名 journal 是 Trellis 专项，
	// Sunshine 无此历史，不移植）。
	if opts.Migrate && classified.PendingCount() > 0 {
		result, err := migrate.ExecuteMigrations(classified, cwd, migrate.ExecuteOptions{
			Force:     opts.Force,
			SkipAll:   opts.SkipAll,
			Prompter:  migrationPrompterFor(opts),
			Templates: templates,
		})
		migrate.PrintMigrationResult(result, w)
		if err != nil {
			return summary, fmt.Errorf("执行迁移失败：%w", err)
		}
	}

	// safe-file-delete 执行（备份之后、模板写盘之前）。
	if hasSafeDeletes {
		deleted, errs := migrate.ExecuteSafeFileDeletes(safeDeletes, cwd)
		summary.SafeDeleted = deleted
		if deleted > 0 {
			fmt.Fprintf(w, "\n已清理 %d 个废弃文件。\n", deleted)
		}
		for _, err := range errs {
			fmt.Fprintf(w, "警告：%v\n", err)
		}
	}

	// 写盘：新增、自动更新、冲突逐个确认。
	filesToHash := map[string]string{}
	for k, v := range unchangedRepairs {
		filesToHash[k] = v
	}
	written := map[string]bool{}
	writeFile := func(file FileChange, content string) error {
		if err := os.MkdirAll(filepath.Dir(file.Path), 0o755); err != nil {
			return fmt.Errorf("创建 %s 的目录失败：%w", file.RelativePath, err)
		}
		if err := os.WriteFile(file.Path, []byte(content), 0o644); err != nil {
			return fmt.Errorf("写入 %s 失败：%w", file.RelativePath, err)
		}
		written[file.RelativePath] = true
		return nil
	}
	// config.yaml 写盘时做合并写：用户已有的行（字段、注释、自定义段落）
	// 一行不动，只追加模板里缺失的段落——覆盖绝不丢用户定制。
	mergedContent := func(file FileChange) string {
		if file.RelativePath != configYamlRel {
			return file.NewContent
		}
		existing, err := os.ReadFile(file.Path)
		if err != nil {
			return file.NewContent
		}
		merged, _ := AppendMissingConfigSections(string(existing), file.NewContent)
		return merged
	}

	if len(changes.NewFiles) > 0 {
		fmt.Fprintln(w, "\n正在添加新文件……")
		for _, file := range changes.NewFiles {
			if err := writeFile(file, mergedContent(file)); err != nil {
				return summary, err
			}
			fmt.Fprintf(w, "  + %s\n", file.RelativePath)
			summary.Added++
		}
	}
	if len(changes.AutoUpdateFiles) > 0 {
		fmt.Fprintln(w, "\n正在自动更新模板文件……")
		for _, file := range changes.AutoUpdateFiles {
			if err := writeFile(file, mergedContent(file)); err != nil {
				return summary, err
			}
			fmt.Fprintf(w, "  ↑ %s\n", file.RelativePath)
			summary.AutoUpdated++
		}
	}
	if len(changes.ChangedFiles) > 0 {
		fmt.Fprint(w, "\n--- 逐个确认冲突 ---\n")
		var applyToAll ConflictAction
		for _, file := range changes.ChangedFiles {
			action, err := resolveConflict(file, opts, &applyToAll)
			if err != nil {
				return summary, err
			}
			switch action {
			case ActionOverwrite:
				content := mergedContent(file)
				if file.RelativePath == ClaudeSettingsPath {
					// settings.json 冲突覆盖时做 hook 合并：只补模板里有、
					// 用户文件里缺失的 hook 组（如新增的 SubagentStop），
					// 用户已有配置（自加 hook、env、statusLine 等）一行
					// 不动；合并失败时回落到模板全文（旧行为）。
					if existing, err := os.ReadFile(file.Path); err == nil {
						if merged, err := platforms.MergeMissingSettingsHooks(existing, []byte(content)); err == nil {
							content = string(merged)
						}
					}
				}
				if err := writeFile(file, content); err != nil {
					return summary, err
				}
				fmt.Fprintf(w, "  ✓ 已覆盖：%s\n", file.RelativePath)
				summary.Updated++
			case ActionCreateNew:
				newPath := file.Path + ".new"
				if err := os.MkdirAll(filepath.Dir(newPath), 0o755); err != nil {
					return summary, fmt.Errorf("创建 %s.new 的目录失败：%w", file.RelativePath, err)
				}
				if err := os.WriteFile(newPath, []byte(file.NewContent), 0o644); err != nil {
					return summary, fmt.Errorf("写入 %s.new 失败：%w", file.RelativePath, err)
				}
				fmt.Fprintf(w, "  ✓ 已创建副本：%s.new\n", file.RelativePath)
				summary.CreatedNew++
			default:
				fmt.Fprintf(w, "  ○ 已跳过：%s\n", file.RelativePath)
				summary.Skipped++
			}
		}
	}

	// 跳过的文件也要补齐缺失的模板段落（上游行为：只加不改）。
	// 注意：这里不把文件记为“已写盘”——段落追加不更新哈希，
	// 下次 update 会在 unchanged 桶里自动修复清单
	// （与上游一致，避免静默把用户的跳过收编为新基线）。
	if template, ok := templates[configYamlRel]; ok {
		if data, err := os.ReadFile(filepath.Join(cwd, filepath.FromSlash(configYamlRel))); err == nil {
			if merged, appended := AppendMissingConfigSections(string(data), template); len(appended) > 0 {
				if err := os.WriteFile(filepath.Join(cwd, filepath.FromSlash(configYamlRel)), []byte(merged), 0o644); err == nil {
					for _, h := range appended {
						fmt.Fprintf(w, "  + 已补充配置段落 %q 到 %s\n", h, configYamlRel)
					}
					summary.SectionsAdded += len(appended)
				}
			}
		}
	}
	if cliVsProject > 0 && !isUnknownVersion {
		if entries := registry.ConfigSectionsAddedBetween(projectVersion, cliVersion); len(entries) > 0 {
			added := migrate.ApplyConfigSectionsAdded(entries, cwd, templates)
			if added > 0 {
				fmt.Fprintf(w, "  + 已补充 %d 个迁移配置段落。\n", added)
			}
			summary.SectionsAdded += added
		}
	}

	// 更新 .version。
	if err := WriteVersionFile(cwd, cliVersion); err != nil {
		return summary, err
	}

	// 更新哈希：新增、自动更新、被覆盖的文件按写盘后的实际内容记哈希
	// （config.yaml 记合并后的内容）；跳过与 .new 副本不记。
	for _, file := range append(append(changes.NewFiles, changes.AutoUpdateFiles...), changes.ChangedFiles...) {
		if !written[file.RelativePath] {
			continue
		}
		data, err := os.ReadFile(file.Path)
		if err != nil {
			continue
		}
		filesToHash[file.RelativePath] = string(data)
	}
	if len(filesToHash) > 0 {
		if err := UpdateHashes(cwd, filesToHash); err != nil {
			return summary, err
		}
	}

	// 汇总。
	fmt.Fprint(w, "\n--- 汇总 ---\n")
	if summary.Added > 0 {
		fmt.Fprintf(w, "  新增：%d 个文件\n", summary.Added)
	}
	if summary.AutoUpdated > 0 {
		fmt.Fprintf(w, "  自动更新：%d 个文件\n", summary.AutoUpdated)
	}
	if summary.Updated > 0 {
		fmt.Fprintf(w, "  已覆盖：%d 个文件\n", summary.Updated)
	}
	if summary.Skipped > 0 {
		fmt.Fprintf(w, "  已跳过：%d 个文件\n", summary.Skipped)
	}
	if summary.CreatedNew > 0 {
		fmt.Fprintf(w, "  已创建 .new 副本：%d 个文件\n", summary.CreatedNew)
	}
	if summary.SafeDeleted > 0 {
		fmt.Fprintf(w, "  已清理废弃文件：%d 个\n", summary.SafeDeleted)
	}
	if summary.SectionsAdded > 0 {
		fmt.Fprintf(w, "  已补充配置段落：%d 个\n", summary.SectionsAdded)
	}
	if summary.BackupDir != "" {
		fmt.Fprintf(w, "  备份：%s/\n", backupRel(cwd, summary.BackupDir))
	}
	actionWord := "更新"
	if isDowngrade {
		actionWord = "降级"
	}
	fmt.Fprintf(w, "\n✓ %s完成！（%s → %s）\n", actionWord, projectVersion, cliVersion)
	if summary.CreatedNew > 0 {
		fmt.Fprintln(w, "\n提示：请检查 .new 文件，手工合并需要的改动。")
	}
	return summary, nil
}

func appendConfigured(configured []string, id string) []string {
	for _, c := range configured {
		if c == id {
			return configured
		}
	}
	return append(configured, id)
}
