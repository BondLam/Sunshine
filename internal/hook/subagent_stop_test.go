package hook

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func runStop(t *testing.T, input string) {
	t.Helper()
	_, _ = runWithInput(t, func(in *strings.Reader, o Options) error { return RunSubagentStop(in, o) }, input)
}

func TestSubagentStop_AgentTypeDirect(t *testing.T) {
	root, _ := makeProject(t)
	RecordSubagentStart(root, "implement", "11-09-demo", "sess-1")
	runStop(t, `{"hook_event_name":"SubagentStop","agent_type":"sunshine-implement","agent_id":"agt-1","session_id":"sess-1","cwd":`+strconv_quot(root)+`}`)
	kinds := activityKinds(t, root)
	if len(kinds) != 2 || kinds[1] != "subagent_stopped|implement|11-09-demo" {
		t.Errorf("agent_type 直连应记一条 stop 事件，实际 %v", kinds)
	}
}

func TestSubagentStop_TranscriptFallback(t *testing.T) {
	root, _ := makeProject(t)
	// 旧版宿主：payload 无 agent_type，transcript 里有角色头与任务引用。
	tr := filepath.Join(t.TempDir(), "transcript.jsonl")
	mustWrite(t, tr, "{\"type\":\"user\",\"message\":{\"role\":\"user\",\"content\":\"<!-- sunshine-hook-injected -->\\n# Check Agent Task\\n\\nYou are the Check Agent.\\n\\n=== .sunshine/tasks/11-09-demo/prd.md (Requirements) ===\\n做一个演示功能。\"}}\n")
	runStop(t, `{"hook_event_name":"SubagentStop","agent_transcript_path":`+strconv_quot(tr)+`,"session_id":"sess-1","cwd":`+strconv_quot(root)+`}`)
	kinds := activityKinds(t, root)
	if len(kinds) != 1 || kinds[0] != "subagent_stopped|check|11-09-demo" {
		t.Errorf("transcript 退路应从角色头与任务引用取 agent/task，实际 %v", kinds)
	}
}

func TestSubagentStop_FallbackLatestUnended(t *testing.T) {
	root, _ := makeProject(t)
	RecordSubagentStart(root, "research", "11-09-demo", "sess-1")
	// 无 transcript、无任务信息：按最近未结束 start 匹配。
	runStop(t, `{"hook_event_name":"SubagentStop","agent_type":"sunshine-research","session_id":"sess-1","cwd":`+strconv_quot(root)+`}`)
	kinds := activityKinds(t, root)
	if len(kinds) != 2 || kinds[1] != "subagent_stopped|research|11-09-demo" {
		t.Errorf("退路应按最近未结束 start 取任务，实际 %v", kinds)
	}
}

func TestSubagentStop_UnknownAgentIgnored(t *testing.T) {
	root, _ := makeProject(t)
	RecordSubagentStart(root, "implement", "11-09-demo", "sess-1")
	// Claude 内部 agent（如 prompt suggestions）的 stop：agent_type
	// 不是 sunshine 子代理，应忽略，不写盘。
	runStop(t, `{"hook_event_name":"SubagentStop","agent_type":"general-purpose","session_id":"sess-1","cwd":`+strconv_quot(root)+`}`)
	kinds := activityKinds(t, root)
	if len(kinds) != 1 {
		t.Errorf("未知 agent 的 stop 不应写盘，实际 %v", kinds)
	}
}

func TestSubagentStop_NoSunshine(t *testing.T) {
	empty := t.TempDir()
	runStop(t, `{"hook_event_name":"SubagentStop","agent_type":"sunshine-implement","cwd":`+strconv_quot(empty)+`}`)
	if _, err := os.Stat(ActivityFilePath(empty)); !os.IsNotExist(err) {
		t.Errorf("无 .sunshine/ 时不应写盘")
	}
}

func TestSubagentStop_Disabled(t *testing.T) {
	root, _ := makeProject(t)
	t.Setenv("SUNSHINE_HOOKS", "0")
	runStop(t, `{"hook_event_name":"SubagentStop","agent_type":"sunshine-implement","session_id":"sess-1","cwd":`+strconv_quot(root)+`}`)
	if _, err := os.Stat(ActivityFilePath(root)); !os.IsNotExist(err) {
		t.Errorf("HooksDisabled 时不应写盘")
	}
}

func TestSubagentStop_CursorPayload(t *testing.T) {
	root, _ := makeProject(t)
	RecordSubagentStart(root, "implement", "11-09-demo", "sess-1")
	// Cursor 的 subagentStop：camelCase 事件名、subagent_type 字段、
	// workspace_roots 定位项目。
	runStop(t, `{"hook_event_name":"subagentStop","subagent_type":"sunshine-implement","workspace_roots":[`+strconv_quot(root)+`]}`)
	kinds := activityKinds(t, root)
	if len(kinds) != 2 || kinds[1] != "subagent_stopped|implement|11-09-demo" {
		t.Errorf("Cursor payload 应同样记 stop 事件，实际 %v", kinds)
	}
}
