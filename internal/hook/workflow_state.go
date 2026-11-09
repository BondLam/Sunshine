package hook

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
	"unicode"
)

// CodexNoTaskBootstrapNotice 是 Codex 无任务时的 bootstrap 提示：
// Codex 拿不到完整的 SessionStart 概览，这句短提醒让主会话
// 只读一次 sunshine-start skill。Sunshine 版把 skill 名换了过来。
const CodexNoTaskBootstrapNotice = `<sunshine-bootstrap>
If you have not already loaded Sunshine context this session, read the ` + "`sunshine-start`" + ` skill once.
</sunshine-bootstrap>`

// RunWorkflowState 实现 sunshine hook workflow-state，对应
// shared-hooks/inject-workflow-state.py（UserPromptSubmit /
// BeforeAgent，每轮提示词面包屑）。
//
// 内容只从 workflow.md 的 [workflow-state:STATUS] 标签块取；
// workflow.md 缺失或标签缺失时退化为通用提示行。无任务时 Codex
// 侧输出 bootstrap 提示。无 .sunshine/ 时静默退出 0。任务目录的
// task.json 缺失/损坏/无可用状态时输出 task_error 面包屑。
func RunWorkflowState(r io.Reader, opts Options) error {
	if HooksDisabled() {
		return nil
	}
	// 某些宿主不关 stdin：0.2 秒等不到数据就按空输入处理，防挂死。
	data := ReadInputTimeout(r, 200*time.Millisecond)
	cwd := GetString(data, "cwd")
	if cwd == "" {
		if wd, err := os.Getwd(); err == nil {
			cwd = wd
		}
	}
	root := FindSunshineRoot(cwd)
	if root == "" {
		return nil
	}
	cfg := LoadConfig(root)
	if PromptHasSkipKeyword(GetString(data, "prompt"), cfg.SkipKeyword) {
		return nil
	}
	templates := LoadBreadcrumbs(root)
	platform := DetectPlatform(data)
	task := GetActiveTaskForWorkflow(root, data, platform)

	var breadcrumb string
	if task == nil {
		key := ResolveBreadcrumbKey("no_task", platform, cfg)
		breadcrumb = BuildBreadcrumb("", "no_task", templates, key)
	} else {
		key := ResolveBreadcrumbKey(task.Status, platform, cfg)
		breadcrumb = BuildBreadcrumb(task.ID, task.Status, templates, key)
	}
	if platform == "codex" {
		var parts []string
		if task == nil {
			parts = append(parts, CodexNoTaskBootstrapNotice)
		}
		parts = append(parts, CodexModeBanner(cfg))
		parts = append(parts, breadcrumb)
		breadcrumb = strings.Join(parts, "\n\n")
	}
	if platform == "kiro" {
		fmt.Fprint(opts.out(), breadcrumb)
		return nil
	}
	eventName := "UserPromptSubmit"
	if platform == "gemini" {
		// Gemini CLI 0.40.x 把每轮事件改名 BeforeAgent，其 schema
		// 校验会拒绝旧名 UserPromptSubmit。
		eventName = "BeforeAgent"
	}
	WriteJSON(opts.out(), map[string]any{
		"hookSpecificOutput": map[string]any{
			"hookEventName":     eventName,
			"additionalContext": breadcrumb,
		},
	})
	return nil
}

// WorkflowTask 是 workflow-state 解析出的任务三元组。
type WorkflowTask struct {
	ID     string
	Status string
	Source string
}

// GetActiveTaskForWorkflow 解析当前任务，区分三种结果：
// nil = 无任务指针；Status 为 "task_error" = 指针存在但 task.json
// 缺失/损坏/无可用状态；Status 为 "stale_<source_type>" = 指针陈旧。
// 对应 inject-workflow-state.py 的 get_active_task。
func GetActiveTaskForWorkflow(root string, input HookInput, platform string) *WorkflowTask {
	active := ResolveActiveTask(root, input, platform, ResolveOptions{AllowEnvironmentContext: true})
	if active.TaskPath == "" {
		return nil
	}
	taskDir := ResolveTaskRef(active.TaskPath, root)
	name := filepath.Base(taskDir)
	if taskDir == "" {
		name = filepath.Base(strings.TrimSuffix(active.TaskPath, "/"))
	}
	if active.Stale {
		return &WorkflowTask{ID: name, Status: "stale_" + active.SourceType, Source: active.Source()}
	}
	data := ReadJSONFile(filepath.Join(taskDir, "task.json"))
	if data == nil {
		return &WorkflowTask{ID: name, Status: "task_error", Source: active.Source()}
	}
	id, _ := data["id"].(string)
	if id == "" {
		id = name
	}
	status, _ := data["status"].(string)
	if strings.TrimSpace(status) == "" {
		return &WorkflowTask{ID: name, Status: "task_error", Source: active.Source()}
	}
	return &WorkflowTask{ID: id, Status: status, Source: active.Source()}
}

// LoadBreadcrumbs 解析 workflow.md 里的 [workflow-state:STATUS] 标签块，
// 返回 {status: 正文}。workflow.md 是唯一事实来源：没有兜底字典，
// 缺失的标签由 BuildBreadcrumb 退化为通用行。
func LoadBreadcrumbs(root string) map[string]string {
	result := map[string]string{}
	content := ReadFile(filepath.Join(root, ".sunshine", "workflow.md"), "")
	if content == "" {
		return result
	}
	for status, body := range scanWorkflowStateBlocks(content) {
		if strings.TrimSpace(body) != "" {
			result[status] = strings.TrimSpace(body)
		}
	}
	return result
}

var workflowTagOpen = regexp.MustCompile(`\[workflow-state:([A-Za-z0-9_-]+)\]`)

// scanWorkflowStateBlocks 手工扫描标签块（RE2 不支持反向引用，
// 不能用 Python 的 `\[...:\1\]` 正则直接配对闭标签）。
func scanWorkflowStateBlocks(content string) map[string]string {
	result := map[string]string{}
	rest := content
	for {
		loc := workflowTagOpen.FindStringSubmatchIndex(rest)
		if loc == nil {
			break
		}
		status := rest[loc[2]:loc[3]]
		// 闭标签须在下一行起始附近：`[...]\n<body>\n\s*[/...]`。
		after := rest[loc[1]:]
		closeTag := "[/workflow-state:" + status + "]"
		closeIdx := strings.Index(after, closeTag)
		if closeIdx == -1 {
			break
		}
		body := after[:closeIdx]
		// body 以换行开头、以换行 + 空白结尾时去掉首尾空白行。
		body = strings.Trim(body, "\n")
		body = strings.TrimRight(body, " \t")
		// 要求闭标签前确有一行结束（与 Python 的 \n\s*[/...] 等价）。
		if idx := strings.LastIndex(body, "\n"); idx != -1 {
			body = body[:idx+1] + strings.TrimSpace(body[idx+1:])
		}
		if _, exists := result[status]; !exists {
			result[status] = strings.TrimSpace(body)
		}
		rest = after[closeIdx+len(closeTag):]
	}
	return result
}

// ResolveBreadcrumbKey 按 Codex dispatch_mode 选面包屑标签键：
// inline 模式用 <status>-inline 标签，auto（默认，sub-agent 是其
// 别名）用普通标签；非法值落到 inline。非 Codex 平台原样返回。
func ResolveBreadcrumbKey(status, platform string, cfg Config) string {
	if platform == "codex" {
		if cfg.ResolveCodexDispatchMode() == "inline" {
			return status + "-inline"
		}
	}
	return status
}

// CodexModeBanner 输出 <codex-mode> 横幅，向 Codex 说明当前分发模式。
func CodexModeBanner(cfg Config) string {
	if cfg.ResolveCodexDispatchMode() == "auto" {
		return "<codex-mode>auto: implement/check work defaults to Sunshine sub-agents; native Codex context injection is preferred and child-side loading is the fallback. The main session still coordinates, clarifies, updates specs, commits, and finishes.</codex-mode>"
	}
	return "<codex-mode>inline: the main session implements/checks directly; do not dispatch implement/check sub-agents.</codex-mode>"
}

// BuildBreadcrumb 组装 <workflow-state> 块：已知状态取 workflow.md
// 的标签正文；未知状态（无标签或 workflow.md 缺失）退化为通用行，
// 让使用者看到并修复 workflow.md，而不是被 hook 静默掩盖。
// taskID 为空时（no_task）标题省略任务信息。
func BuildBreadcrumb(taskID, status string, templates map[string]string, breadcrumbKey string) string {
	body := templates[breadcrumbKey]
	if body == "" && breadcrumbKey != status {
		body = templates[status]
	}
	if body == "" {
		body = "Refer to workflow.md for current step."
	}
	header := "Status: " + status
	if taskID != "" {
		header = fmt.Sprintf("Task: %s (%s)", taskID, status)
	}
	return "<workflow-state>\n" + header + "\n" + body + "\n</workflow-state>"
}

// PromptHasSkipKeyword 判断提示词是否含 skip_keyword（默认 "no-sunshine"）：
// 大小写不敏感、按词边界匹配；连字符算词内字符，
// 因此 "no-sunshinex" / "xno-sunshine" / "foo-no-sunshine" 都不匹配，
// 标点/空白边界才算。空 keyword 永不匹配（关闭逃生口）。
// Go 的 regexp 不支持前瞻/后顾，这里手工检查边界。
func PromptHasSkipKeyword(prompt, keyword string) bool {
	if keyword == "" || prompt == "" {
		return false
	}
	pr := []rune(strings.ToLower(prompt))
	kw := []rune(strings.ToLower(keyword))
	for i := 0; i+len(kw) <= len(pr); {
		idx := -1
		for j := i; j+len(kw) <= len(pr); j++ {
			match := true
			for k := range kw {
				if pr[j+k] != kw[k] {
					match = false
					break
				}
			}
			if match {
				idx = j
				break
			}
		}
		if idx == -1 {
			return false
		}
		if wordBoundary(pr, idx, idx+len(kw)) {
			return true
		}
		i = idx + 1
	}
	return false
}

func isWordChar(r rune) bool {
	return r == '_' || r == '-' || unicode.IsLetter(r) || unicode.IsDigit(r)
}

// wordBoundary 检查 s[start:end] 两侧是否为词边界（两侧字符都不是词字符）。
func wordBoundary(rs []rune, start, end int) bool {
	if start > 0 && isWordChar(rs[start-1]) {
		return false
	}
	if end < len(rs) && isWordChar(rs[end]) {
		return false
	}
	return true
}
