package hook

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// RunStatusline 实现 sunshine hook statusline，对应
// templates/claude/hooks/statusline.py：读 Claude Code 会话 JSON，
// 输出 1–2 行状态：
//
//	有当前任务：[P1] 任务标题 (状态) [+ 来源]，再加信息行；
//	无任务：只输出信息行。
//
// 信息行：模型 · ctx% · 分支 · 耗时 · 开发者 · 任务数 · 限流。
// COLUMNS 太窄放不下信息行时，限流段另起一行。
//
// 与 Python 的一点偏差：Python 在无 .trellis/ 时仍输出信息行；
// Go 版按本阶段任务要求，无 .sunshine/ 时静默退出 0（与其他
// hook 子命令一致，测试也按此验收）。
func RunStatusline(r io.Reader, opts Options) error {
	input := ReadInput(r)
	root := FindSunshineRoot("")
	if root == "" {
		return nil
	}
	sep := " \x1b[90m·\x1b[0m "

	var task *statuslineTask
	dev := ""
	taskCount := 0
	if root != "" {
		task = currentTaskForStatusline(root, input)
		dev = GetDeveloper(root)
		taskCount = CountActiveTasks(root, LoadConfig(root).TasksDirAbs(root))
	}

	model := "?"
	if m, ok := input["model"].(map[string]any); ok {
		if s, ok := m["display_name"].(string); ok && s != "" {
			model = s
		}
	}
	ctxPct := 0
	ctxSize := 0
	if cw, ok := input["context_window"].(map[string]any); ok {
		ctxPct = int(numberField(cw, "used_percentage"))
		ctxSize = int(numberField(cw, "context_window_size"))
	}
	durationMs := 0
	if cost, ok := input["cost"].(map[string]any); ok {
		durationMs = int(numberField(cost, "total_duration_ms"))
	}
	branch := RunGit(root, "branch", "--show-current")

	modelLabel := model
	if !regexp.MustCompile(`\d+[KMG]\b`).MatchString(model) {
		// 避免 "Opus 4.6 (1M context) (1M)" 这种重复标注。
		modelLabel = fmt.Sprintf("%s (%s)", model, formatCtxSize(ctxSize))
	}
	var ctxColor string
	switch {
	case ctxPct >= 90:
		ctxColor = "\x1b[31m"
	case ctxPct >= 70:
		ctxColor = "\x1b[33m"
	default:
		ctxColor = "\x1b[32m"
	}
	parts := []string{
		modelLabel,
		fmt.Sprintf("ctx %s%d%%\x1b[0m", ctxColor, ctxPct),
	}
	if branch != "" {
		parts = append(parts, fmt.Sprintf("\x1b[35m%s\x1b[0m", branch))
	}
	parts = append(parts, formatDuration(durationMs))
	if dev != "" {
		parts = append(parts, fmt.Sprintf("\x1b[32m%s\x1b[0m", dev))
	}
	if taskCount > 0 {
		parts = append(parts, fmt.Sprintf("%d task(s)", taskCount))
	}
	now := time.Now().Unix()
	var rateParts []string
	if rl, ok := input["rate_limits"].(map[string]any); ok {
		for _, kv := range []struct{ label, key string }{{"5h", "five_hour"}, {"7d", "seven_day"}} {
			if w, ok := rl[kv.key].(map[string]any); ok {
				if part := rateLimitPart(kv.label, w, now); part != "" {
					rateParts = append(rateParts, part)
				}
			}
		}
	}
	infoLine := strings.Join(append(append([]string{}, parts...), rateParts...), sep)

	var lines []string
	if task != nil {
		sourceTag := task.Source
		if strings.HasPrefix(sourceTag, "session:") {
			sourceTag = "session"
		}
		suffix := ""
		if sourceTag != "" {
			suffix = fmt.Sprintf(" \x1b[90m[%s]\x1b[0m", sourceTag)
		}
		lines = append(lines, fmt.Sprintf("\x1b[36m[%s]\x1b[0m %s \x1b[33m(%s)\x1b[0m%s", task.Priority, task.Title, task.Status, suffix))
	}
	// Claude Code 的状态栏高度只数 "\n" 个数，视觉折行会错位；
	// 宿主给了终端宽度且信息行会溢出时，把限流段显式换行另起一行。
	if width := terminalWidth(); width != nil && len(rateParts) > 0 && VisibleLen(infoLine) > *width {
		lines = append(lines, strings.Join(parts, sep))
		lines = append(lines, strings.Join(rateParts, sep))
	} else {
		lines = append(lines, infoLine)
	}
	fmt.Fprint(opts.out(), strings.Join(lines, "\n")+"\n")
	return nil
}

type statuslineTask struct {
	Title    string
	Status   string
	Priority string
	Source   string
}

// currentTaskForStatusline 取当前任务（Claude 平台），对应
// statusline.py 的 _get_current_task_for_input。
func currentTaskForStatusline(root string, input HookInput) *statuslineTask {
	active := ResolveActiveTask(root, input, "claude", ResolveOptions{AllowEnvironmentContext: true})
	if active.TaskPath == "" {
		return nil
	}
	taskDir := ResolveTaskRef(active.TaskPath, root)
	if active.Stale {
		return &statuslineTask{
			Title:    filepath.Base(taskDir),
			Status:   "stale",
			Priority: "P?",
			Source:   active.Source(),
		}
	}
	data := ReadJSONFile(filepath.Join(taskDir, "task.json"))
	if data == nil {
		return nil
	}
	title, _ := data["title"].(string)
	if title == "" {
		title, _ = data["name"].(string)
	}
	if title == "" {
		title = "unknown"
	}
	status, _ := data["status"].(string)
	if status == "" {
		status = "unknown"
	}
	priority, _ := data["priority"].(string)
	if priority == "" {
		priority = "P2"
	}
	return &statuslineTask{Title: title, Status: status, Priority: priority, Source: active.Source()}
}

// numberField 取 JSON 数字字段（数字或数字字符串），取不到返回 0。
func numberField(m map[string]any, key string) float64 {
	switch v := m[key].(type) {
	case float64:
		return v
	case int:
		return float64(v)
	case string:
		if f, err := strconv.ParseFloat(strings.TrimSpace(v), 64); err == nil {
			return f
		}
	}
	return 0
}

func formatCtxSize(size int) string {
	switch {
	case size >= 1000000:
		return fmt.Sprintf("%dM", size/1000000)
	case size >= 1000:
		return fmt.Sprintf("%dK", size/1000)
	default:
		return strconv.Itoa(size)
	}
}

func formatDuration(ms int) string {
	secs := ms / 1000
	hours := secs / 3600
	mins := (secs % 3600) / 60
	if hours > 0 {
		return fmt.Sprintf("%dh%dm", hours, mins)
	}
	return fmt.Sprintf("%dm", mins)
}

func formatRemaining(secs int64) string {
	if secs <= 0 {
		return ""
	}
	days := secs / 86400
	hours := (secs % 86400) / 3600
	mins := (secs % 3600) / 60
	switch {
	case days > 0:
		return fmt.Sprintf("%dd%dh", days, hours)
	case hours > 0:
		return fmt.Sprintf("%dh%dm", hours, mins)
	default:
		return fmt.Sprintf("%dm", mins)
	}
}

// parseResetsAt 解析 resets_at：epoch 秒（数字或数字字符串）
// 或 ISO-8601 时间戳；解析失败返回 0（此时不显示倒计时）。
func parseResetsAt(value any) int64 {
	switch v := value.(type) {
	case float64:
		return int64(v)
	case int:
		return int64(v)
	case string:
		if v == "" {
			return 0
		}
		if f, err := strconv.ParseFloat(v, 64); err == nil {
			return int64(f)
		}
		if t, err := time.Parse(time.RFC3339, strings.Replace(v, "Z", "+00:00", 1)); err == nil {
			return t.Unix()
		}
	}
	return 0
}

func rateLimitPart(label string, window map[string]any, now int64) string {
	pct, ok := window["used_percentage"]
	if !ok {
		return ""
	}
	var pctInt int
	switch v := pct.(type) {
	case float64:
		pctInt = int(v)
	case int:
		pctInt = v
	case string:
		f, err := strconv.ParseFloat(strings.TrimSpace(v), 64)
		if err != nil {
			return ""
		}
		pctInt = int(f)
	default:
		return ""
	}
	part := fmt.Sprintf("%s %d%%", label, pctInt)
	if remaining := formatRemaining(parseResetsAt(window["resets_at"]) - now); remaining != "" {
		part += fmt.Sprintf(" \x1b[90m(reset %s)\x1b[0m", remaining)
	}
	return part
}

// terminalWidth 从 COLUMNS 环境变量取终端宽度（Claude Code
// v2.1.153+ 注入）；缺失或非法返回 nil。
func terminalWidth() *int {
	w, err := strconv.Atoi(os.Getenv("COLUMNS"))
	if err != nil || w <= 0 {
		return nil
	}
	return &w
}
