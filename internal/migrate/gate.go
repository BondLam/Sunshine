package migrate

import "fmt"

// ShouldHardStop 判断 update 是否应硬停：区间内有破坏性待迁移项，
// 且用户没有传 --migrate。硬停可防止版本号先被抬上去、旧路径却留在
// 原地造成的半迁移状态。
//
//   - migrate：用户是否传了 --migrate；
//   - dryRun：--dry-run 只预览，不硬停；
//   - isUpgrade：CLI 版高于项目版（降级不硬停）；
//   - knownVersion：项目版已知（读不到 .version 时不硬停）；
//   - meta：区间清单的汇总元数据（Breaking + RecommendMigrate 同时为真才硬停）。
func ShouldHardStop(classified ClassifiedMigrations, meta MigrationMetadata, migrate, dryRun, isUpgrade, knownVersion bool) bool {
	if classified.PendingCount() == 0 {
		return false
	}
	if migrate || dryRun || !isUpgrade || !knownVersion {
		return false
	}
	return meta.Breaking && meta.RecommendMigrate
}

// HardStopMessage 返回硬停时展示给用户的提示（中文），指引用户重跑
// 带 --migrate 的更新。
func HardStopMessage(fromVersion, toVersion string) string {
	return fmt.Sprintf(
		"✖ 需要迁移：%s → %s 之间有破坏性变更，必须用 --migrate 执行迁移。\n\n  请运行：sunshine update --migrate\n\n  不带 --migrate 时，重命名/搬移的文件不会被处理，项目会留下新旧并存的过期路径。\n  用 --dry-run 可先预览 --migrate 将要做什么。",
		fromVersion, toVersion,
	)
}

// MigrateHint 返回非破坏性待迁移项的轻提示（中文）：无 --migrate 时
// 告诉用户可以用 --migrate 应用迁移；无待迁移项时返回 ""。
func MigrateHint(classified ClassifiedMigrations) string {
	auto, confirm := len(classified.Auto), len(classified.Confirm)
	if auto == 0 && confirm == 0 {
		return ""
	}
	hint := "提示：用 --migrate 可应用迁移（已改动文件会逐个确认）。"
	if confirm > 0 {
		hint += "用 --migrate -f 可全部强制执行，或用 --migrate -s 跳过已改动文件。"
	}
	return hint
}
