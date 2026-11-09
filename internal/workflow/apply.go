package workflow

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// OverwriteAction 是已改动 workflow.md 时用户在交互确认里的选择
// （对应上游 workflow.ts 的 confirmOverwriteInteractively 三项）。
type OverwriteAction int

const (
	// ActionOverwrite 覆盖本地修改。
	ActionOverwrite OverwriteAction = iota
	// ActionCreateNew 写 .sunshine/workflow.md.new，保留现有文件。
	ActionCreateNew
	// ActionSkip 不做任何改动。
	ActionSkip
)

// ApplyOptions 是 Apply 的选项（与 cobra 解耦，便于测试）。
type ApplyOptions struct {
	// Force 为真时跳过确认直接覆盖（-f/--force）。
	Force bool
	// CreateNew 为真时只写 .sunshine/workflow.md.new（-n/--create-new）。
	CreateNew bool
	// Interactive 为真表示 stdin 是交互终端（可弹确认）。
	Interactive bool
	// Confirm 在需要交互确认时调用；为 nil 时视为无法确认。
	Confirm func() (OverwriteAction, error)
	// Out 接收中文提示输出；为 nil 时用 os.Stdout。
	Out io.Writer
}

// Apply 把解析出的模板内容写盘，覆盖决策按现有 workflow.md 的
// 哈希状态（Classify 的四态，与上游 classifyExistingWorkflow 一致）：
//
//   - 缺失 / 未改动（哈希一致）→ 直接覆盖；
//   - 内容已与模板相同 → 只刷新哈希条目；
//   - 已改动（哈希有记录但不匹配，或清单无记录）→ 交互确认
//     （覆盖 / 写 .new / 跳过），非交互且无 --force 时报错；
//   - --create-new → 一律写 .new，哈希清单不动。
//
// 哈希登记按 ApplyHashContract：native 刷新条目，非 native 删除条目。
func Apply(cwd string, tmpl Resolved, opts ApplyOptions) error {
	out := opts.Out
	if out == nil {
		out = os.Stdout
	}
	workflowPath := filepath.Join(cwd, WorkflowRelPath)
	newPath := workflowPath + ".new"

	if err := os.MkdirAll(filepath.Dir(workflowPath), 0o755); err != nil {
		return fmt.Errorf("创建 .sunshine/ 目录失败：%w", err)
	}

	writeNew := func() error {
		if err := os.WriteFile(newPath, []byte(tmpl.Content), 0o644); err != nil {
			return fmt.Errorf("写入 %s 失败：%w", WorkflowRelPath+".new", err)
		}
		fmt.Fprintf(out, "  + 已写入 %s（workflow.md 未改动）\n", WorkflowRelPath+".new")
		return nil
	}
	writeMain := func() error {
		if err := os.WriteFile(workflowPath, []byte(tmpl.Content), 0o644); err != nil {
			return fmt.Errorf("写入 %s 失败：%w", WorkflowRelPath, err)
		}
		fmt.Fprintf(out, "  ✓ 已把 %s 换成 %q\n", WorkflowRelPath, tmpl.ID)
		return ApplyHashContract(cwd, tmpl.ID)
	}

	// --create-new 一律写 .new 副本，不看磁盘状态、不动哈希清单。
	if opts.CreateNew {
		return writeNew()
	}

	class, err := Classify(cwd, tmpl.Content)
	if err != nil {
		return err
	}

	switch class {
	case ClassIdentical:
		fmt.Fprintf(out, "  ○ %s 已是 %q，无需更换（刷新哈希条目）\n", WorkflowRelPath, tmpl.ID)
		return ApplyHashContract(cwd, tmpl.ID)
	case ClassMissing, ClassPristine:
		return writeMain()
	case ClassModified:
		if opts.Force {
			return writeMain()
		}
		if !opts.Interactive || opts.Confirm == nil {
			return fmt.Errorf("%s 已有本地修改，请加 --force 覆盖，或用 --create-new 写 %s", WorkflowRelPath, WorkflowRelPath+".new")
		}
		action, err := opts.Confirm()
		if err != nil {
			return fmt.Errorf("确认覆盖失败：%w", err)
		}
		switch action {
		case ActionSkip:
			fmt.Fprintln(out, "  ○ 已跳过，未做任何改动")
			return nil
		case ActionCreateNew:
			return writeNew()
		default:
			return writeMain()
		}
	default:
		return fmt.Errorf("未知的 workflow 状态：%d", class)
	}
}

// FormatMissingAgentsWarning 生成模板引用的 agent 缺失时的警告文案
// （对应上游 workflow.ts 的 warnAboutMissingAgents；只警告、不报错）。
func FormatMissingAgentsWarning(missing []string) string {
	var b strings.Builder
	b.WriteString(fmt.Sprintf("\n⚠ 所选工作流引用了 .sunshine/agents/{%s}.md，但这些文件不在磁盘上。\n", strings.Join(missing, ",")))
	b.WriteString("  请运行 `sunshine update` 补装内置的 agent 定义，或在 .sunshine/agents/ 下创建它们。\n")
	return b.String()
}
