package hook

import (
	"os"
	"strconv"
	"strings"
	"testing"
	"time"
)

func readActivity(t *testing.T, root string) []ActivityEvent {
	t.Helper()
	events, err := ReadActivityEvents(root, 0)
	if err != nil {
		t.Fatalf("读 activity.jsonl 失败：%v", err)
	}
	return events
}

func activityKinds(t *testing.T, root string) []string {
	t.Helper()
	var kinds []string
	for _, ev := range readActivity(t, root) {
		kinds = append(kinds, ev.Kind+"|"+ev.Agent+"|"+ev.Task)
	}
	return kinds
}

func TestActivityAppendAndRead(t *testing.T) {
	root := t.TempDir()
	RecordSubagentStart(root, "implement", "11-09-demo", "sess-1")
	RecordSubagentStop(root, "implement", "11-09-demo", "sess-1")
	events := readActivity(t, root)
	if len(events) != 2 {
		t.Fatalf("应有 2 条事件，实际 %d", len(events))
	}
	if events[0].Kind != ActivityKindStarted || events[0].Agent != "implement" ||
		events[0].Task != "11-09-demo" || events[0].Source != ActivitySourceHook {
		t.Errorf("start 事件字段不对：%+v", events[0])
	}
	if events[1].Kind != ActivityKindStopped {
		t.Errorf("第二条应为 stop，实际 %+v", events[1])
	}
	if _, err := time.Parse(time.RFC3339, events[0].TS); err != nil {
		t.Errorf("ts 应为 RFC3339，实际 %q", events[0].TS)
	}
	// 坏行跳过，不报错。
	path := ActivityFilePath(root)
	f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		t.Fatal(err)
	}
	_, _ = f.WriteString("{ not json\n\n")
	_ = f.Close()
	events = readActivity(t, root)
	if len(events) != 2 {
		t.Errorf("坏行应跳过，仍应有 2 条事件，实际 %d", len(events))
	}
}

func TestActivityTruncate(t *testing.T) {
	root := t.TempDir()
	for i := 0; i < MaxActivityLines+5; i++ {
		RecordSubagentStart(root, "implement", "11-09-demo", "sess-"+strconv.Itoa(i))
	}
	events := readActivity(t, root)
	if len(events) != MaxActivityLines {
		t.Errorf("超过上限应截断到 %d 行，实际 %d", MaxActivityLines, len(events))
	}
	if events[len(events)-1].SessionID != "sess-1004" {
		t.Errorf("应保留最近的行，末行 session 应为 sess-1004，实际 %+v", events[len(events)-1])
	}
}

func TestUnendedStart(t *testing.T) {
	events := []ActivityEvent{
		{Kind: ActivityKindStarted, Agent: "implement", Task: "task-a"},
		{Kind: ActivityKindStopped, Agent: "implement", Task: "task-a"},
		{Kind: ActivityKindStarted, Agent: "implement", Task: "task-b"},
		{Kind: ActivityKindStarted, Agent: "check", Task: "task-b"},
		{Kind: ActivityKindStopped, Agent: "check", Task: ""},
	}
	if st := UnendedStart(events, "implement", ""); st == nil || st.Task != "task-b" {
		t.Errorf("implement 最近未结束 start 应为 task-b，实际 %+v", st)
	}
	if st := UnendedStart(events, "check", ""); st != nil {
		t.Errorf("check 已被空 task 的 stop 配对，应无未结束 start，实际 %+v", st)
	}
	if st := UnendedStart(events, "research", ""); st != nil {
		t.Errorf("research 无事件，应为 nil，实际 %+v", st)
	}
	// stop 的 task 与 start 不同时不配对。
	events2 := []ActivityEvent{
		{Kind: ActivityKindStarted, Agent: "implement", Task: "task-a"},
		{Kind: ActivityKindStopped, Agent: "implement", Task: "task-b"},
	}
	if st := UnendedStart(events2, "implement", ""); st == nil || st.Task != "task-a" {
		t.Errorf("不同任务的 stop 不应配对，task-a 的 start 应仍未结束，实际 %+v", st)
	}
}

// --- Start 事件写盘 ---

func TestSubagentContext_WritesStartEvent(t *testing.T) {
	root, _ := makeProject(t)
	out, _ := runWithInput(t, func(in *strings.Reader, o Options) error { return RunSubagentContext(in, o) },
		`{"session_id":"sess-1","cwd":`+strconv_quot(root)+`,"tool_name":"Task","tool_input":{"subagent_type":"sunshine-implement","prompt":"实现演示功能"}}`)
	if !strings.Contains(out, "# Implement Agent Task") {
		t.Fatalf("前置：implement 应注入上下文，实际：%s", out)
	}
	kinds := activityKinds(t, root)
	if len(kinds) != 1 || kinds[0] != "subagent_started|implement|11-09-demo" {
		t.Errorf("注入成功后应记一条 start 事件，实际 %v", kinds)
	}
}

func TestSubagentContext_PrototypeWritesStartOnly(t *testing.T) {
	root, _ := makeProject(t)
	out, _ := runWithInput(t, func(in *strings.Reader, o Options) error { return RunSubagentContext(in, o) },
		`{"session_id":"sess-1","cwd":`+strconv_quot(root)+`,"tool_name":"Task","tool_input":{"subagent_type":"sunshine-prototype","prompt":"按需求列表做原型"}}`)
	if out != "" {
		t.Errorf("prototype 不应注入上下文，应无输出，实际：%s", out)
	}
	kinds := activityKinds(t, root)
	if len(kinds) != 1 || kinds[0] != "subagent_started|prototype|11-09-demo" {
		t.Errorf("prototype 应只记一条 start 事件，实际 %v", kinds)
	}
}

func TestSubagentContext_NoWriteWhenDisabled(t *testing.T) {
	root, _ := makeProject(t)
	t.Setenv("SUNSHINE_HOOKS", "0")
	out, _ := runWithInput(t, func(in *strings.Reader, o Options) error { return RunSubagentContext(in, o) },
		`{"session_id":"sess-1","cwd":`+strconv_quot(root)+`,"tool_name":"Task","tool_input":{"subagent_type":"sunshine-implement","prompt":"实现演示功能"}}`)
	if out != "" {
		t.Errorf("HooksDisabled 时应无输出，实际：%s", out)
	}
	if _, err := os.Stat(ActivityFilePath(root)); !os.IsNotExist(err) {
		t.Errorf("HooksDisabled 时不应写盘")
	}
}

func TestSubagentContext_NoWriteWhenNoSunshine(t *testing.T) {
	empty := t.TempDir()
	out, _ := runWithInput(t, func(in *strings.Reader, o Options) error { return RunSubagentContext(in, o) },
		`{"cwd":`+strconv_quot(empty)+`,"tool_name":"Task","tool_input":{"subagent_type":"sunshine-implement","prompt":"做点事"}}`)
	if out != "" {
		t.Errorf("无 .sunshine/ 时应静默无输出，实际：%s", out)
	}
	if _, err := os.Stat(ActivityFilePath(empty)); !os.IsNotExist(err) {
		t.Errorf("无 .sunshine/ 时不应写盘")
	}
}

func TestSubagentContext_WriteFailureSilent(t *testing.T) {
	root, _ := makeProject(t)
	// activity.jsonl 本身是个目录：写盘失败，但注入输出不受影响。
	dir := ActivityFilePath(root)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	out, _ := runWithInput(t, func(in *strings.Reader, o Options) error { return RunSubagentContext(in, o) },
		`{"session_id":"sess-1","cwd":`+strconv_quot(root)+`,"tool_name":"Task","tool_input":{"subagent_type":"sunshine-implement","prompt":"实现演示功能"}}`)
	if !strings.Contains(out, "# Implement Agent Task") {
		t.Errorf("写盘失败时注入输出仍应正常，实际：%s", out)
	}
}

func TestSubagentContext_CodexWritesStartEvent(t *testing.T) {
	root, _ := makeProject(t)
	out, _ := runWithInput(t, func(in *strings.Reader, o Options) error { return RunSubagentContext(in, o) },
		`{"hook_event_name":"SubagentStart","agent_type":"sunshine-check","session_id":"sess-1","cwd":`+strconv_quot(root)+`}`)
	if !strings.Contains(out, "Sunshine Native Check Subagent") {
		t.Fatalf("前置：Codex SubagentStart 应输出上下文，实际：%s", out)
	}
	kinds := activityKinds(t, root)
	if len(kinds) != 1 || kinds[0] != "subagent_started|check|11-09-demo" {
		t.Errorf("Codex 注入成功后应记一条 start 事件，实际 %v", kinds)
	}
}
