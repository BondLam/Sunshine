package hook

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// makeProject 在 t.TempDir() 下建一个最小 Sunshine 项目：
// .sunshine/config.yaml、workflow.md（含 [workflow-state:*] 标签块）、
// .developer、一个带 task.json/prd.md/jsonl 的任务、一条会话指针。
// 返回项目根目录与任务引用。
func makeProject(t *testing.T) (root, taskRef string) {
	t.Helper()
	root = t.TempDir()
	// 真实 Claude Code 环境会设 CLAUDE_PROJECT_DIR，平台检测据此
	// 得到 claude，会话键为 claude_<session_id>，与夹具文件名一致。
	t.Setenv("CLAUDE_PROJECT_DIR", root)
	sun := filepath.Join(root, ".sunshine")
	mustWrite(t, filepath.Join(sun, "config.yaml"), "version: 0.1.0\nmax_journal_lines: 2000\ntasks_dir: .sunshine/tasks\n")
	mustWrite(t, filepath.Join(sun, "workflow.md"), `# 工作流

## Phase Index

- 规划、实现、检查三段式。

[workflow-state:planning]
按规划推进：先写 prd.md，再评审。
[/workflow-state:planning]

[workflow-state:in_progress]
按 implement.md 执行，每轮只做一个任务。
[/workflow-state:in_progress]

[workflow-state:no_task]
无当前任务：先判断本轮是否值得建任务。
[/workflow-state:no_task]

## Phase 1: Plan

规划阶段正文。
`)
	mustWrite(t, filepath.Join(sun, ".developer"), "小林\n")
	mustWrite(t, filepath.Join(sun, "spec", "backend", "index.md"), "# 后端规范\n")
	mustWrite(t, filepath.Join(sun, "spec", "backend", "api.md"), "# API 规范\n")
	taskRef = ".sunshine/tasks/11-09-demo"
	taskDir := filepath.Join(root, ".sunshine", "tasks", "11-09-demo")
	mustWrite(t, filepath.Join(taskDir, "task.json"), `{"id": "11-09-demo", "title": "演示任务", "status": "planning", "priority": "P1"}`+"\n")
	mustWrite(t, filepath.Join(taskDir, "prd.md"), "# 需求\n做一个演示功能。\n")
	mustWrite(t, filepath.Join(taskDir, "design.md"), "# 设计\n分两层实现。\n")
	mustWrite(t, filepath.Join(taskDir, "implement.jsonl"), "{\"file\": \".sunshine/spec/backend/api.md\", \"reason\": \"接口规范\"}\n")
	mustWrite(t, filepath.Join(taskDir, "check.jsonl"), "{\"file\": \".sunshine/spec/backend/api.md\", \"reason\": \"检查规范\"}\n")
	// 会话指针：claude_<session_id> -> 当前任务。
	mustWrite(t, filepath.Join(sun, ".runtime", "sessions", "claude_sess-1.json"),
		`{"current_task": ".sunshine/tasks/11-09-demo", "platform": "claude"}`+"\n")
	// Codex 原生 SubagentStart 路径固定用 platform=codex 解析，
	// 会话键为 codex_<session_id>，另备一份同内容指针。
	mustWrite(t, filepath.Join(sun, ".runtime", "sessions", "codex_sess-1.json"),
		`{"current_task": ".sunshine/tasks/11-09-demo", "platform": "codex"}`+"\n")
	return root, taskRef
}

func mustWrite(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func runWithInput(t *testing.T, run func(in *strings.Reader, opts Options) error, input string) (string, string) {
	t.Helper()
	var out, errBuf bytes.Buffer
	err := run(strings.NewReader(input), Options{Version: "0.1.0", Stdout: &out, Stderr: &errBuf})
	if err != nil {
		t.Fatalf("Run 返回了错误（hook 绝不能返回错误）：%v", err)
	}
	return out.String(), errBuf.String()
}

func mustJSON(t *testing.T, s string) map[string]any {
	t.Helper()
	var data map[string]any
	if err := json.Unmarshal([]byte(s), &data); err != nil {
		t.Fatalf("输出不是合法 JSON：%v\n输出：%s", err, s)
	}
	return data
}

func additionalContext(t *testing.T, s string) string {
	t.Helper()
	data := mustJSON(t, s)
	hs, ok := data["hookSpecificOutput"].(map[string]any)
	if !ok {
		t.Fatalf("输出缺少 hookSpecificOutput：%s", s)
	}
	ctx, _ := hs["additionalContext"].(string)
	return ctx
}

// --- session-start ---

func TestSessionStart_WithSunshine(t *testing.T) {
	root, _ := makeProject(t)
	out, _ := runWithInput(t, func(in *strings.Reader, o Options) error { return RunSessionStart(in, o) },
		`{"session_id":"sess-1","cwd":`+strconv_quot(root)+`}`)
	ctx := additionalContext(t, out)
	for _, want := range []string{
		"Developer: 小林",
		"Current task: .sunshine/tasks/11-09-demo; status=planning.",
		"演示任务",
		"Status: PLANNING",
		"Development Workflow - Session Summary",
		".sunshine/spec/backend/index.md",
		"Sunshine SessionStart",
	} {
		if !strings.Contains(ctx, want) {
			t.Errorf("session-start 上下文缺少 %q\n全文：\n%s", want, ctx)
		}
	}
	// 顶层 additional_context 与 hookSpecificOutput 应一致（非 ZCode 平台）。
	data := mustJSON(t, out)
	if data["additional_context"] != ctx {
		t.Errorf("顶层 additional_context 与 hookSpecificOutput 不一致")
	}
}

func TestSessionStart_NoSunshine(t *testing.T) {
	empty := t.TempDir()
	// 进程 cwd 也参与项目根查找（回退）；切到空目录，避免祖先目录
	// 的残留 .sunshine/（如 ~/.sunshine/ablations 默认事务根）干扰。
	t.Chdir(empty)
	out, _ := runWithInput(t, func(in *strings.Reader, o Options) error { return RunSessionStart(in, o) },
		`{"session_id":"sess-1","cwd":`+strconv_quot(empty)+`}`)
	if out != "" {
		t.Errorf("无 .sunshine/ 时应静默无输出，实际：%s", out)
	}
}

func TestSessionStart_BadJSON(t *testing.T) {
	root, _ := makeProject(t)
	t.Chdir(root)
	// stdin 不是 JSON：按空输入处理，cwd 回退到进程 cwd，仍输出上下文。
	out, _ := runWithInput(t, func(in *strings.Reader, o Options) error { return RunSessionStart(in, o) }, `这不是 JSON`)
	if !strings.Contains(out, "Developer: 小林") {
		t.Errorf("stdin 非 JSON 时仍应在项目内输出上下文，实际：%s", out)
	}
}

func TestSessionStart_DisabledByEnv(t *testing.T) {
	root, _ := makeProject(t)
	t.Setenv("SUNSHINE_HOOKS", "0")
	out, _ := runWithInput(t, func(in *strings.Reader, o Options) error { return RunSessionStart(in, o) },
		`{"session_id":"sess-1","cwd":`+strconv_quot(root)+`}`)
	if out != "" {
		t.Errorf("SUNSHINE_HOOKS=0 时应无输出，实际：%s", out)
	}
}

func TestSessionStart_NoTask(t *testing.T) {
	root, _ := makeProject(t)
	out, _ := runWithInput(t, func(in *strings.Reader, o Options) error { return RunSessionStart(in, o) },
		`{"session_id":"nobody","cwd":`+strconv_quot(root)+`}`)
	ctx := additionalContext(t, out)
	if !strings.Contains(ctx, "Status: NO ACTIVE TASK") {
		t.Errorf("无任务时应输出 NO ACTIVE TASK，实际：\n%s", ctx)
	}
}

// --- workflow-state ---

func TestWorkflowState_WithTask(t *testing.T) {
	root, _ := makeProject(t)
	out, _ := runWithInput(t, func(in *strings.Reader, o Options) error { return RunWorkflowState(in, o) },
		`{"session_id":"sess-1","cwd":`+strconv_quot(root)+`,"prompt":"继续开发"}`)
	ctx := additionalContext(t, out)
	if !strings.Contains(ctx, "Task: 11-09-demo (planning)") {
		t.Errorf("面包屑标题不对：%s", ctx)
	}
	if !strings.Contains(ctx, "按规划推进：先写 prd.md，再评审。") {
		t.Errorf("面包屑正文应取自 workflow.md 的标签块：%s", ctx)
	}
}

func TestWorkflowState_NoSunshine(t *testing.T) {
	empty := t.TempDir()
	out, _ := runWithInput(t, func(in *strings.Reader, o Options) error { return RunWorkflowState(in, o) },
		`{"cwd":`+strconv_quot(empty)+`,"prompt":"hi"}`)
	if out != "" {
		t.Errorf("无 .sunshine/ 时应静默无输出，实际：%s", out)
	}
}

func TestWorkflowState_NoTaskCodexBootstrap(t *testing.T) {
	root, _ := makeProject(t)
	t.Setenv("SUNSHINE_HOOK_PLATFORM", "codex")
	out, _ := runWithInput(t, func(in *strings.Reader, o Options) error { return RunWorkflowState(in, o) },
		`{"session_id":"nobody","cwd":`+strconv_quot(root)+`,"prompt":"hi"}`)
	ctx := additionalContext(t, out)
	for _, want := range []string{"<sunshine-bootstrap>", "sunshine-start", "<codex-mode>auto:", "Status: no_task", "无当前任务：先判断本轮是否值得建任务。"} {
		if !strings.Contains(ctx, want) {
			t.Errorf("Codex 无任务面包屑缺少 %q\n全文：\n%s", want, ctx)
		}
	}
}

func TestWorkflowState_TaskError(t *testing.T) {
	root, _ := makeProject(t)
	// 指针指向的任务目录存在但 task.json 缺失 → task_error 面包屑。
	taskDir := filepath.Join(root, ".sunshine", "tasks", "11-09-demo")
	if err := os.Remove(filepath.Join(taskDir, "task.json")); err != nil {
		t.Fatal(err)
	}
	out, _ := runWithInput(t, func(in *strings.Reader, o Options) error { return RunWorkflowState(in, o) },
		`{"session_id":"sess-1","cwd":`+strconv_quot(root)+`,"prompt":"hi"}`)
	ctx := additionalContext(t, out)
	if !strings.Contains(ctx, "Task: 11-09-demo (task_error)") {
		t.Errorf("task.json 缺失时应输出 task_error 面包屑，实际：%s", ctx)
	}
}

func TestWorkflowState_SkipKeyword(t *testing.T) {
	root, _ := makeProject(t)
	out, _ := runWithInput(t, func(in *strings.Reader, o Options) error { return RunWorkflowState(in, o) },
		`{"session_id":"sess-1","cwd":`+strconv_quot(root)+`,"prompt":"这次 no-sunshine，别注入"}`)
	if out != "" {
		t.Errorf("提示词含 skip_keyword 时应无输出，实际：%s", out)
	}
}

func TestWorkflowState_MissingTagFallsBackToGeneric(t *testing.T) {
	root, _ := makeProject(t)
	// 把任务状态改成 workflow.md 里没有标签的 completed。
	taskDir := filepath.Join(root, ".sunshine", "tasks", "11-09-demo")
	mustWrite(t, filepath.Join(taskDir, "task.json"), `{"id": "11-09-demo", "title": "演示任务", "status": "completed"}`+"\n")
	out, _ := runWithInput(t, func(in *strings.Reader, o Options) error { return RunWorkflowState(in, o) },
		`{"session_id":"sess-1","cwd":`+strconv_quot(root)+`,"prompt":"hi"}`)
	ctx := additionalContext(t, out)
	if !strings.Contains(ctx, "Refer to workflow.md for current step.") {
		t.Errorf("标签缺失时应退化为通用提示行，实际：%s", ctx)
	}
}

func TestPromptHasSkipKeyword(t *testing.T) {
	cases := []struct {
		prompt  string
		keyword string
		want    bool
	}{
		{"这次 no-sunshine，别注入", "no-sunshine", true},
		{"NO-SUNSHINE", "no-sunshine", true},
		{"no-sunshinex 不算", "no-sunshine", false},
		{"xno-sunshine 不算", "no-sunshine", false},
		{"foo-no-sunshine 不算", "no-sunshine", false},
		{"（no-sunshine）标点边界算", "no-sunshine", true},
		{"什么都没有", "no-sunshine", false},
		{"no-sunshine", "", false},
	}
	for _, c := range cases {
		if got := PromptHasSkipKeyword(c.prompt, c.keyword); got != c.want {
			t.Errorf("PromptHasSkipKeyword(%q, %q) = %v，想要 %v", c.prompt, c.keyword, got, c.want)
		}
	}
}

// --- subagent-context ---

func TestSubagentContext_Implement(t *testing.T) {
	root, _ := makeProject(t)
	out, _ := runWithInput(t, func(in *strings.Reader, o Options) error { return RunSubagentContext(in, o) },
		`{"session_id":"sess-1","cwd":`+strconv_quot(root)+`,"tool_name":"Task","tool_input":{"subagent_type":"sunshine-implement","prompt":"实现演示功能"}}`)
	data := mustJSON(t, out)
	hs := data["hookSpecificOutput"].(map[string]any)
	updated := hs["updatedInput"].(map[string]any)
	prompt, _ := updated["prompt"].(string)
	for _, want := range []string{
		"# Implement Agent Task",
		"做一个演示功能。",
		"# API 规范",
		"实现演示功能",
	} {
		if !strings.Contains(prompt, want) {
			t.Errorf("implement 提示词缺少 %q\n全文：\n%s", want, prompt)
		}
	}
	if data["permission"] != "allow" {
		t.Errorf("非 ZCode 平台应同时返回顶层 permission=allow")
	}
}

func TestSubagentContext_ResearchNeedsNoTask(t *testing.T) {
	root, _ := makeProject(t)
	out, _ := runWithInput(t, func(in *strings.Reader, o Options) error { return RunSubagentContext(in, o) },
		`{"session_id":"nobody","cwd":`+strconv_quot(root)+`,"tool_name":"Task","tool_input":{"subagent_type":"sunshine-research","prompt":"找找后端规范在哪"}}`)
	data := mustJSON(t, out)
	hs := data["hookSpecificOutput"].(map[string]any)
	updated := hs["updatedInput"].(map[string]any)
	prompt, _ := updated["prompt"].(string)
	if !strings.Contains(prompt, "# Research Agent Task") || !strings.Contains(prompt, ".sunshine/spec/") {
		t.Errorf("research 不依赖当前任务，应输出规范目录结构，实际：\n%s", prompt)
	}
}

func TestSubagentContext_NoSunshine(t *testing.T) {
	empty := t.TempDir()
	out, _ := runWithInput(t, func(in *strings.Reader, o Options) error { return RunSubagentContext(in, o) },
		`{"cwd":`+strconv_quot(empty)+`,"tool_name":"Task","tool_input":{"subagent_type":"sunshine-implement","prompt":"做点事"}}`)
	if out != "" {
		t.Errorf("无 .sunshine/ 时应静默无输出，实际：%s", out)
	}
}

func TestSubagentContext_UnknownAgent(t *testing.T) {
	root, _ := makeProject(t)
	out, _ := runWithInput(t, func(in *strings.Reader, o Options) error { return RunSubagentContext(in, o) },
		`{"session_id":"sess-1","cwd":`+strconv_quot(root)+`,"tool_name":"Task","tool_input":{"subagent_type":"sunshine-plan","prompt":"规划一下"}}`)
	if out != "" {
		t.Errorf("未知子代理应无输出，实际：%s", out)
	}
}

func TestSubagentContext_CodexSubagentStart(t *testing.T) {
	root, _ := makeProject(t)
	out, _ := runWithInput(t, func(in *strings.Reader, o Options) error { return RunSubagentContext(in, o) },
		`{"hook_event_name":"SubagentStart","agent_type":"sunshine-implement","session_id":"sess-1","cwd":`+strconv_quot(root)+`}`)
	ctx := additionalContext(t, out)
	for _, want := range []string{"Sunshine Native Implement Subagent", "Active task: .sunshine/tasks/11-09-demo", "做一个演示功能。"} {
		if !strings.Contains(ctx, want) {
			t.Errorf("Codex SubagentStart 上下文缺少 %q\n全文：\n%s", want, ctx)
		}
	}
	data := mustJSON(t, out)
	if data["hookSpecificOutput"].(map[string]any)["hookEventName"] != "SubagentStart" {
		t.Errorf("Codex 分支的 hookEventName 应为 SubagentStart")
	}
}

func TestSubagentContext_TrellisAliasAccepted(t *testing.T) {
	root, _ := makeProject(t)
	out, _ := runWithInput(t, func(in *strings.Reader, o Options) error { return RunSubagentContext(in, o) },
		`{"session_id":"sess-1","cwd":`+strconv_quot(root)+`,"tool_name":"Task","tool_input":{"subagent_type":"trellis-check","prompt":"检查代码"}}`)
	if !strings.Contains(out, "# Check Agent Task") {
		t.Errorf("trellis- 旧名应兼容接受，实际：%s", out)
	}
}

func TestTruncateUTF8KeepsCharactersWhole(t *testing.T) {
	// "演示任务" 每个汉字 3 字节：cap=5 时只能保留 1 个汉字（3 字节），
	// 不能切出半个汉字。
	got := truncateUTF8([]byte("演示任务"), 5)
	if string(got) != "演" {
		t.Errorf("truncateUTF8 应不切断多字节字符，得到 %q", string(got))
	}
	if got := truncateUTF8([]byte("abc"), 0); string(got) != "abc" {
		t.Errorf("cap<=0 表示不限，得到 %q", string(got))
	}
}

// --- shell-session ---

func TestShellSession_WritesTicket(t *testing.T) {
	root, _ := makeProject(t)
	out, _ := runWithInput(t, func(in *strings.Reader, o Options) error { return RunShellSession(in, o) },
		`{"command":"sunshine task start 11-09-demo","session_id":"sess-1","cwd":`+strconv_quot(root)+`}`)
	if strings.TrimSpace(out) != `{"permission":"allow"}` {
		t.Errorf("shell 执行类事件应返回 {\"permission\":\"allow\"}，实际：%s", out)
	}
	dir := filepath.Join(root, ".sunshine", ".runtime", "shell-tickets")
	entries, err := os.ReadDir(dir)
	if err != nil || len(entries) != 1 {
		t.Fatalf("应写一张票据，目录 %s 内容：%v, err=%v", dir, entries, err)
	}
	data := mustJSON(t, string(mustRead(t, filepath.Join(dir, entries[0].Name()))))
	if data["context_key"] != "claude_sess-1" {
		t.Errorf("票据 context_key 应为 claude_sess-1，实际 %v", data["context_key"])
	}
	if data["cwd"] != root {
		t.Errorf("票据 cwd 应为项目根目录，实际 %v", data["cwd"])
	}
	subs, _ := data["subcommands"].([]any)
	if len(subs) != 1 {
		t.Fatalf("票据 subcommands 应有 1 条，实际 %v", data["subcommands"])
	}
	sub := subs[0].(map[string]any)
	if sub["name"] != "start" || sub["task_ref"] != "11-09-demo" {
		t.Errorf("票据 subcommands 不对：%v", sub)
	}
	created, _ := data["created_at_epoch"].(float64)
	expires, _ := data["expires_at_epoch"].(float64)
	if expires-created != 30 {
		t.Errorf("票据 TTL 应为 30 秒，实际 %v", expires-created)
	}
}

func TestShellSession_ToolInputShapeWritesTicketWithoutResponse(t *testing.T) {
	root, _ := makeProject(t)
	out, _ := runWithInput(t, func(in *strings.Reader, o Options) error { return RunShellSession(in, o) },
		`{"session_id":"sess-1","cwd":`+strconv_quot(root)+`,"tool_input":{"command":"sunshine task current"}}`)
	if out != "" {
		t.Errorf("工具调用形状不应输出放行答复，实际：%s", out)
	}
	dir := filepath.Join(root, ".sunshine", ".runtime", "shell-tickets")
	entries, err := os.ReadDir(dir)
	if err != nil || len(entries) != 1 {
		t.Fatalf("工具调用形状也应写票据，err=%v entries=%v", err, entries)
	}
}

func TestShellSession_NonTaskCommand(t *testing.T) {
	root, _ := makeProject(t)
	out, _ := runWithInput(t, func(in *strings.Reader, o Options) error { return RunShellSession(in, o) },
		`{"command":"ls -la","session_id":"sess-1","cwd":`+strconv_quot(root)+`}`)
	if out != "" {
		t.Errorf("非任务命令应无输出，实际：%s", out)
	}
	if _, err := os.Stat(filepath.Join(root, ".sunshine", ".runtime", "shell-tickets")); !os.IsNotExist(err) {
		t.Errorf("非任务命令不应写票据")
	}
}

func TestShellSession_NoSunshine(t *testing.T) {
	empty := t.TempDir()
	// 同 TestSessionStart_NoSunshine：进程 cwd 也参与项目根查找。
	t.Chdir(empty)
	out, _ := runWithInput(t, func(in *strings.Reader, o Options) error { return RunShellSession(in, o) },
		`{"command":"sunshine task start x","session_id":"sess-1","cwd":`+strconv_quot(empty)+`}`)
	if out != "" {
		t.Errorf("无 .sunshine/ 时应静默无输出，实际：%s", out)
	}
}

func TestShellSession_LegacyTaskPyAccepted(t *testing.T) {
	root, _ := makeProject(t)
	out, _ := runWithInput(t, func(in *strings.Reader, o Options) error { return RunShellSession(in, o) },
		`{"command":"python3 ./.sunshine/scripts/task.py finish","session_id":"sess-1","cwd":`+strconv_quot(root)+`}`)
	if strings.TrimSpace(out) != `{"permission":"allow"}` {
		t.Errorf("旧 task.py 写法应兼容并放行，实际：%s", out)
	}
}

// --- statusline ---

func TestStatusline_WithTask(t *testing.T) {
	root, _ := makeProject(t)
	t.Chdir(root)
	out, _ := runWithInput(t, func(in *strings.Reader, o Options) error { return RunStatusline(in, o) },
		`{"session_id":"sess-1","model":{"display_name":"Sonnet"},"context_window":{"used_percentage":42,"context_window_size":200000},"cost":{"total_duration_ms":65000}}`)
	lines := strings.Split(strings.TrimRight(out, "\n"), "\n")
	if len(lines) != 2 {
		t.Fatalf("有任务时应输出 2 行，实际 %d 行：%s", len(lines), out)
	}
	if !strings.Contains(lines[0], "[P1]") || !strings.Contains(lines[0], "演示任务") || !strings.Contains(lines[0], "(planning)") {
		t.Errorf("任务行不对：%s", lines[0])
	}
	for _, want := range []string{"Sonnet (200K)", "ctx", "42%", "1m", "小林", "1 task(s)"} {
		if !strings.Contains(lines[1], want) {
			t.Errorf("信息行缺少 %q：%s", want, lines[1])
		}
	}
}

func TestStatusline_NoSunshine(t *testing.T) {
	empty := t.TempDir()
	t.Chdir(empty)
	out, _ := runWithInput(t, func(in *strings.Reader, o Options) error { return RunStatusline(in, o) },
		`{"model":{"display_name":"Sonnet"},"context_window":{"used_percentage":10}}`)
	if out != "" {
		t.Errorf("无 .sunshine/ 时应静默无输出，实际：%s", out)
	}
}

func TestStatusline_BadJSON(t *testing.T) {
	root, _ := makeProject(t)
	t.Chdir(root)
	out, _ := runWithInput(t, func(in *strings.Reader, o Options) error { return RunStatusline(in, o) }, `不是 JSON`)
	if !strings.Contains(out, "? (0)") || !strings.Contains(out, "ctx") {
		t.Errorf("stdin 非 JSON 时仍应在项目内输出信息行（模型未知），实际：%s", out)
	}
}

func TestStatusline_RateLimitsSplitOnNarrowTerminal(t *testing.T) {
	root, _ := makeProject(t)
	t.Chdir(root)
	t.Setenv("COLUMNS", "20")
	out, _ := runWithInput(t, func(in *strings.Reader, o Options) error { return RunStatusline(in, o) },
		`{"session_id":"sess-1","model":{"display_name":"Sonnet"},"context_window":{"used_percentage":42,"context_window_size":200000},"cost":{"total_duration_ms":1000},"rate_limits":{"five_hour":{"used_percentage":55,"resets_at":1999999999}}}`)
	lines := strings.Split(strings.TrimRight(out, "\n"), "\n")
	if len(lines) != 3 {
		t.Fatalf("窄终端时限流段应另起一行，共 3 行，实际 %d 行：%q", len(lines), out)
	}
	if !strings.Contains(lines[2], "5h 55%") {
		t.Errorf("第三行应为限流段，实际：%s", lines[2])
	}
}

// --- config 解析 ---

func TestLoadConfig(t *testing.T) {
	root, _ := makeProject(t)
	mustWrite(t, filepath.Join(root, ".sunshine", "config.yaml"), `packages:
  api:
    path: packages/api
  web:
    path: packages/web
default_package: api
session:
  spec_scope:
    - api
    - unknown-pkg
prompt_injection:
  skip_keyword: "skip-me"
codex:
  dispatch_mode: inline
context_injection:
  max_file_bytes: 100
  max_artifact_bytes: 200
  max_total_bytes: 300
max_journal_lines: 500
`)
	cfg := LoadConfig(root)
	if !cfg.IsMonorepo() || !cfg.Packages["api"] || !cfg.Packages["web"] {
		t.Errorf("packages 解析不对：%v", cfg.Packages)
	}
	if cfg.DefaultPackage != "api" || cfg.SkipKeyword != "skip-me" {
		t.Errorf("标量解析不对：default=%q skip=%q", cfg.DefaultPackage, cfg.SkipKeyword)
	}
	if cfg.ResolveCodexDispatchMode() != "inline" {
		t.Errorf("dispatch_mode 应为 inline，实际 %q", cfg.ResolveCodexDispatchMode())
	}
	if cfg.MaxFileBytes != 100 || cfg.MaxArtifactBytes != 200 || cfg.MaxTotalBytes != 300 || cfg.MaxJournalLines != 500 {
		t.Errorf("数值解析不对：%+v", cfg)
	}
	scope, ok := cfg.SpecScope.([]any)
	if !ok || len(scope) != 2 {
		t.Errorf("spec_scope 列表解析不对：%v", cfg.SpecScope)
	}
}

func mustRead(t *testing.T, path string) []byte {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func strconv_quot(s string) string {
	b, _ := json.Marshal(s)
	return string(b)
}
