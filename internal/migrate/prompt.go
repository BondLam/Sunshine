package migrate

import (
	"fmt"
	"strings"

	"github.com/AlecAivazis/survey/v2"
)

// Prompter 是对一条需确认迁移项的交互确认函数。
// ExecuteMigrations 的 Options.Prompter 为空时使用默认实现
// PromptMigrationAction；测试与非交互调用方可传入自己的实现。
type Prompter func(item MigrationItem) (MigrationAction, error)

// promptMigrationActionImpl 是默认的交互确认实现（可用变量替换，便于测试）。
var promptMigrationActionImpl Prompter = PromptMigrationAction

// PromptMigrationAction 弹出交互确认：对一条已被用户改动的迁移项，
// 让用户选择直接执行、备份后执行或跳过。默认选中最安全的“备份后执行”。
// 非交互终端下 survey 会返回错误，调用方应按跳过处理。
func PromptMigrationAction(item MigrationItem) (MigrationAction, error) {
	var headline, renameLabel, backupLabel string
	if item.Type == MigrationTypeRename || item.Type == MigrationTypeRenameDir {
		headline = fmt.Sprintf("%s → %s", item.From, item.To)
		renameLabel = "直接执行——文件未改动，或改动随文件一起搬走也没关系"
		backupLabel = "备份原文件后执行——最安全：先把当前内容写成 .backup，再执行迁移"
	} else {
		headline = fmt.Sprintf("删除 %s", item.From)
		renameLabel = "直接删除——不再需要这个文件（已有替代文件）"
		backupLabel = "备份原文件后执行——最安全：先把当前内容写成 .backup，再删除"
	}
	skipLabel := "跳过——保留旧路径（下次更新还会继续提示，直到手工清理）"
	description := item.Description
	if description == "" {
		description = "清单中未提供说明。"
	}
	var why string
	if item.Reason != "" {
		why = "提示原因：\n" + indentLines(item.Reason)
	} else {
		why = "提示原因：文件内容与模板哈希不一致，通常是本地定制过。如不确定，请选备份后执行。"
	}
	message := strings.Join([]string{
		headline,
		"",
		"说明：" + description,
		why,
		"",
		"请选择：",
	}, "\n")

	options := []string{backupLabel, renameLabel, skipLabel}
	var chosen string
	if err := survey.AskOne(&survey.Select{
		Message: message,
		Options: options,
		Default: backupLabel,
	}, &chosen); err != nil {
		return ActionSkip, err
	}
	switch chosen {
	case renameLabel:
		return ActionRename, nil
	case skipLabel:
		return ActionSkip, nil
	default:
		return ActionBackupRename, nil
	}
}

func indentLines(s string) string {
	lines := strings.Split(s, "\n")
	for i, line := range lines {
		lines[i] = "  " + line
	}
	return strings.Join(lines, "\n")
}
