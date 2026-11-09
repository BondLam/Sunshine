package update

// ConflictAction 是单个冲突文件的处理动作。
type ConflictAction string

const (
	// ActionOverwrite 用新版覆盖。
	ActionOverwrite ConflictAction = "overwrite"
	// ActionSkip 保留当前版本。
	ActionSkip ConflictAction = "skip"
	// ActionCreateNew 把新版存为 .new 副本。
	ActionCreateNew ConflictAction = "create-new"
	// ActionOverwriteAll 对剩余全部文件覆盖。
	ActionOverwriteAll ConflictAction = "overwrite-all"
	// ActionSkipAll 对剩余全部文件跳过。
	ActionSkipAll ConflictAction = "skip-all"
	// ActionCreateNewAll 对剩余全部文件写 .new 副本。
	ActionCreateNewAll ConflictAction = "create-new-all"
)

// Prompter 是 update 的交互接口（CLI 层用 survey 实现；
// 为 nil 时视为非交互环境：统一确认直接通过，
// 冲突文件默认 Skip）。
type Prompter interface {
	// ConfirmProceed 在写盘前做统一确认。
	ConfirmProceed() (bool, error)
	// ResolveConflict 对单个冲突文件提问，返回六种动作之一。
	ResolveConflict(path string) (ConflictAction, error)
}

// resolveConflict 解析单个冲突文件的动作（上游 promptConflictResolution）：
// 已有批量动作时直接用；--force → 覆盖；--skip-all → 跳过；
// --create-new → 写 .new；否则交互提问（批量项会设置 applyToAll）；
// 无 Prompter（非交互）时默认 Skip。
func resolveConflict(file FileChange, opts Options, applyToAll *ConflictAction) (ConflictAction, error) {
	if applyToAll != nil && *applyToAll != "" {
		return *applyToAll, nil
	}
	if opts.Force {
		return ActionOverwrite, nil
	}
	if opts.SkipAll {
		return ActionSkip, nil
	}
	if opts.CreateNew {
		return ActionCreateNew, nil
	}
	if opts.Prompter == nil {
		return ActionSkip, nil
	}
	action, err := opts.Prompter.ResolveConflict(file.RelativePath)
	if err != nil {
		return "", err
	}
	switch action {
	case ActionOverwriteAll:
		*applyToAll = ActionOverwrite
		return ActionOverwrite, nil
	case ActionSkipAll:
		*applyToAll = ActionSkip
		return ActionSkip, nil
	case ActionCreateNewAll:
		*applyToAll = ActionCreateNew
		return ActionCreateNew, nil
	case ActionOverwrite, ActionSkip, ActionCreateNew:
		return action, nil
	default:
		return ActionSkip, nil
	}
}
