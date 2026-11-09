package adapters

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/linrong1982/Sunshine/internal/mem"
)

func codexFixture(t *testing.T, home string) string {
	t.Helper()
	dir := filepath.Join(home, ".codex", "sessions", "2026-11-08")
	file := filepath.Join(dir, "rollout-2026-11-08T10-00-00-abc.jsonl")
	writeFixture(t, file, `{"timestamp":"2026-11-08T10:00:00.000Z","type":"session_meta","payload":{"type":"session","id":"codex-1","cwd":"/tmp/proj"}}`+"\n"+
		`{"timestamp":"2026-11-08T10:00:01.000Z","type":"response_item","payload":{"type":"message","role":"user","content":[{"type":"input_text","text":"帮我重构"}]}}`+"\n"+
		`{"timestamp":"2026-11-08T10:00:02.000Z","type":"response_item","payload":{"type":"message","role":"assistant","content":[{"type":"output_text","text":"已重构完成"}]}}`+"\n"+
		`{"timestamp":"2026-11-08T10:00:03.000Z","type":"response_item","payload":{"type":"agent_message","content":[{"type":"input_text","text":"Message Type: FINAL_ANSWER\nTask name: t\nSender: a\nPayload: 子代理完成的结果"}]}}`+"\n"+
		`{"timestamp":"2026-11-08T10:00:04.000Z","type":"response_item","payload":{"type":"agent_message","content":[{"type":"input_text","text":"Message Type: MESSAGE\nTask name: t\nSender: a\nPayload:"},{"type":"encrypted_content","text":"xxx"}]}}`+"\n"+
		`{"timestamp":"2026-11-08T10:00:05.000Z","type":"event_msg","payload":{"type":"agent_message","content":[{"type":"input_text","text":"Message Type: FINAL_ANSWER\nTask name: t\nSender: a\nPayload: 重复的事件消息，不应出现两次"}]}}`+"\n"+
		`{"timestamp":"2026-11-08T10:00:06.000Z","type":"something_new","payload":{"foo":1}}`+"\n"+
		`坏行`+"\n"+
		`{"timestamp":"2026-11-08T10:00:07.000Z","type":"compacted","payload":{"replacement_history":[{"type":"message","role":"user","content":[{"type":"input_text","text":"帮我重构"}]},{"type":"message","role":"user","content":[{"type":"input_text","text":"更早的问题"}]}]}}`+"\n")
	return file
}

func TestCodexSessions(t *testing.T) {
	home := newTempHome(t)
	file := codexFixture(t, home)
	sessions, warnings, err := CodexAdapter{}.Sessions(context.Background(), home)
	if err != nil {
		t.Fatalf("Sessions 返回错误：%v", err)
	}
	if len(warnings) != 0 {
		t.Errorf("正常夹具不应有警告，实际：%v", warnings)
	}
	if len(sessions) != 1 {
		t.Fatalf("会话数 = %d，想要 1", len(sessions))
	}
	s := sessions[0]
	if s.ID != "codex-1" {
		t.Errorf("id = %q，想要 codex-1（取首个事件 payload）", s.ID)
	}
	if s.CWD != "/tmp/proj" {
		t.Errorf("cwd = %q", s.CWD)
	}
	if s.Created != "2026-11-08T10:00:00.000Z" {
		t.Errorf("created = %q，想要首个事件时间戳", s.Created)
	}
	if s.FilePath != file {
		t.Errorf("filePath = %q", s.FilePath)
	}
}

func TestCodexSessionsFilenameFallback(t *testing.T) {
	home := newTempHome(t)
	dir := filepath.Join(home, ".codex", "sessions")
	file := filepath.Join(dir, "rollout-2026-11-07T09-30-00-xyz.jsonl")
	writeFixture(t, file, `{"type":"other"}`+"\n")
	sessions, _, err := CodexAdapter{}.Sessions(context.Background(), home)
	if err != nil || len(sessions) != 1 {
		t.Fatalf("Sessions 失败：%v，%d", err, len(sessions))
	}
	if sessions[0].ID != "xyz" {
		t.Errorf("无 payload id 时应取文件名 id，实际 %q", sessions[0].ID)
	}
	if sessions[0].Created != "2026-11-07T09:30:00.000Z" {
		t.Errorf("无时间戳时应取文件名时间，实际 %q", sessions[0].Created)
	}
}

func TestCodexDialogue(t *testing.T) {
	home := newTempHome(t)
	file := codexFixture(t, home)
	ctx := context.Background()
	sessions, _, err := CodexAdapter{}.Sessions(ctx, home)
	if err != nil || len(sessions) != 1 {
		t.Fatalf("建夹具会话失败：%v，%d", err, len(sessions))
	}
	before := hashFile(t, file)
	turns, warnings, err := CodexAdapter{}.Dialogue(ctx, sessions[0])
	if err != nil {
		t.Fatalf("Dialogue 返回错误：%v", err)
	}
	if before != hashFile(t, file) {
		t.Error("只读验证：Codex JSONL 文件实测前后哈希变化")
	}
	// 期望顺序：恢复的“更早的问题”插到最前，已在池子里的“帮我重构”
	// 抵扣一次不重复，助手回答、FINAL_ANSWER、压缩 marker；
	// 加密信封只警告不入轮，event_msg 重复排除，未知事件与坏行容忍。
	if len(turns) != 5 {
		t.Fatalf("轮次数 = %d，想要 5：%+v", len(turns), turns)
	}
	want := []struct{ role, text string }{
		{"user", "更早的问题"},
		{"user", "帮我重构"},
		{"assistant", "已重构完成"},
		{"assistant", "子代理完成的结果"},
		{"user", ""},
	}
	for i, w := range want {
		if turns[i].Role != w.role {
			t.Errorf("第 %d 轮 role = %q，想要 %q", i+1, turns[i].Role, w.role)
		}
		if w.text != "" && turns[i].Text != w.text {
			t.Errorf("第 %d 轮 text = %q，想要 %q", i+1, turns[i].Text, w.text)
		}
	}
	if turns[4].Kind != "marker" {
		t.Errorf("第 5 轮应为压缩 marker，实际 %+v", turns[4])
	}
	for _, text := range []string{"重复的事件消息"} {
		for _, turn := range turns {
			if contains(turn.Text, text) {
				t.Errorf("event_msg 重复内容不应出现：%q", turn.Text)
			}
		}
	}
	mustContainWarning(t, warnings, "加密存储")
	mustContainWarning(t, warnings, "跳过 1 行")
}

func TestCommandFromCodexArguments(t *testing.T) {
	cases := []struct {
		name string
		in   any
		want string
	}{
		{"原始字符串", "ls -la", "ls -la"},
		{"JSON cmd", `{"cmd": "echo hi"}`, "echo hi"},
		{"JSON command", `{"command": "echo hi"}`, "echo hi"},
		{"JSON argv", `{"argv": ["git", "status"]}`, "git status"},
		{"原始对象", map[string]any{"command": "pwd"}, "pwd"},
		{"空", nil, ""},
	}
	for _, tc := range cases {
		if got := commandFromCodexArguments(tc.in); got != tc.want {
			t.Errorf("%s：= %q，想要 %q", tc.name, got, tc.want)
		}
	}
}

func codexCollectFixture(t *testing.T, home string) mem.MemSessionInfo {
	t.Helper()
	dir := filepath.Join(home, ".codex", "sessions", "2026-11-08")
	file := filepath.Join(dir, "rollout-2026-11-08T10-00-00-phase.jsonl")
	writeFixture(t, file, `{"timestamp":"2026-11-08T10:00:00.000Z","type":"session_meta","payload":{"type":"session","id":"codex-phase","cwd":"/tmp/proj"}}`+"\n"+
		`{"timestamp":"2026-11-08T10:00:01.000Z","type":"response_item","payload":{"type":"message","role":"user","content":[{"type":"input_text","text":"先想想做什么"}]}}`+"\n"+
		`{"timestamp":"2026-11-08T10:00:02.000Z","type":"response_item","payload":{"type":"function_call","name":"exec_command","arguments":"{\"command\":\"sunshine task create --slug demo\"}"}}`+"\n"+
		`{"timestamp":"2026-11-08T10:00:03.000Z","type":"response_item","payload":{"type":"message","role":"assistant","content":[{"type":"output_text","text":"方案草稿"}]}}`+"\n"+
		`{"timestamp":"2026-11-08T10:00:04.000Z","type":"response_item","payload":{"type":"message","role":"user","content":[{"type":"input_text","text":"方案定了"}]}}`+"\n"+
		`{"timestamp":"2026-11-08T10:00:05.000Z","type":"response_item","payload":{"type":"function_call","name":"shell","arguments":"{\"argv\":[\"sunshine\",\"task\",\"start\",\".sunshine/tasks/11-09-demo/\"]}"}}`+"\n"+
		`{"timestamp":"2026-11-08T10:00:06.000Z","type":"response_item","payload":{"type":"message","role":"assistant","content":[{"type":"output_text","text":"写代码中"}]}}`+"\n"+
		`{"timestamp":"2026-11-08T10:00:07.000Z","type":"response_item","payload":{"type":"message","role":"user","content":[{"type":"input_text","text":"收尾"}]}}`+"\n")
	return mem.MemSessionInfo{Platform: mem.MemSourceCodex, ID: "codex-phase", CWD: "/tmp/proj", FilePath: file}
}

func TestCodexCollect(t *testing.T) {
	home := newTempHome(t)
	s := codexCollectFixture(t, home)
	ctx := context.Background()

	dTurns, _, err := CodexAdapter{}.Dialogue(ctx, s)
	if err != nil {
		t.Fatalf("Dialogue 返回错误：%v", err)
	}
	turns, events, _, err := CodexAdapter{}.Collect(ctx, s)
	if err != nil {
		t.Fatalf("Collect 返回错误：%v", err)
	}
	assertSameTurns(t, turns, dTurns)
	assertNoCommandInTurnText(t, turns)
	if len(turns) != 5 {
		t.Fatalf("轮次数 = %d，想要 5：%+v", len(turns), turns)
	}
	if len(events) != 2 {
		t.Fatalf("事件数 = %d，想要 2：%+v", len(events), events)
	}
	assertEvent(t, events[0], "create", 1, "demo", "", "2026-11-08T10:00:02.000Z")
	assertEvent(t, events[1], "start", 3, "", ".sunshine/tasks/11-09-demo/", "2026-11-08T10:00:05.000Z")

	brain, _, _ := mem.SliceMemPhase(turns, events, mem.PhaseBrainstorm, mem.MemSourceCodex)
	if len(brain) != 2 || brain[0].Text != "方案草稿" || brain[1].Text != "方案定了" {
		t.Errorf("brainstorm 应只出窗口内 2 轮，实际：%v", brain)
	}
	impl, _, _ := mem.SliceMemPhase(turns, events, mem.PhaseImplement, mem.MemSourceCodex)
	if len(impl) != 3 || impl[0].Text != "先想想做什么" || impl[2].Text != "收尾" {
		t.Errorf("implement 应只出窗口外 3 轮，实际：%v", impl)
	}
}

// 压缩恢复出更早轮次时，压缩前已记录的事件下标应随之平移。
func TestCodexCollect_CompactionShiftsEvents(t *testing.T) {
	home := newTempHome(t)
	dir := filepath.Join(home, ".codex", "sessions", "2026-11-08")
	file := filepath.Join(dir, "rollout-2026-11-08T10-00-00-shift.jsonl")
	writeFixture(t, file, `{"timestamp":"2026-11-08T10:00:00.000Z","type":"session_meta","payload":{"type":"session","id":"codex-shift","cwd":"/tmp/proj"}}`+"\n"+
		`{"timestamp":"2026-11-08T10:00:01.000Z","type":"response_item","payload":{"type":"message","role":"user","content":[{"type":"input_text","text":"第一问"}]}}`+"\n"+
		`{"timestamp":"2026-11-08T10:00:02.000Z","type":"response_item","payload":{"type":"function_call","name":"exec_command","arguments":"{\"command\":\"sunshine task create --slug demo\"}"}}`+"\n"+
		`{"timestamp":"2026-11-08T10:00:03.000Z","type":"response_item","payload":{"type":"message","role":"assistant","content":[{"type":"output_text","text":"回答"}]}}`+"\n"+
		`{"timestamp":"2026-11-08T10:00:04.000Z","type":"compacted","payload":{"replacement_history":[{"type":"message","role":"user","content":[{"type":"input_text","text":"更早的问题"}]},{"type":"message","role":"user","content":[{"type":"input_text","text":"第一问"}]}]}}`+"\n"+
		`{"timestamp":"2026-11-08T10:00:05.000Z","type":"response_item","payload":{"type":"message","role":"user","content":[{"type":"input_text","text":"第二问"}]}}`+"\n"+
		`{"timestamp":"2026-11-08T10:00:06.000Z","type":"response_item","payload":{"type":"function_call","name":"exec_command","arguments":"{\"command\":\"sunshine task start .sunshine/tasks/11-09-demo/\"}"}}`+"\n"+
		`{"timestamp":"2026-11-08T10:00:07.000Z","type":"response_item","payload":{"type":"message","role":"assistant","content":[{"type":"output_text","text":"动手"}]}}`+"\n")
	s := mem.MemSessionInfo{Platform: mem.MemSourceCodex, ID: "codex-shift", CWD: "/tmp/proj", FilePath: file}
	ctx := context.Background()

	turns, events, warnings, err := CodexAdapter{}.Collect(ctx, s)
	if err != nil {
		t.Fatalf("Collect 返回错误：%v", err)
	}
	if len(turns) != 6 {
		t.Fatalf("轮次数 = %d，想要 6：%+v", len(turns), turns)
	}
	if turns[0].Text != "更早的问题" || turns[3].Kind != "marker" {
		t.Errorf("恢复轮次应插到最前、压缩 marker 应在第 4 轮，实际：%+v", turns)
	}
	if len(events) != 2 {
		t.Fatalf("事件数 = %d，想要 2：%+v", len(events), events)
	}
	// create 原在下标 1，恢复 1 轮后应平移到 2。
	assertEvent(t, events[0], "create", 2, "demo", "", "2026-11-08T10:00:02.000Z")
	assertEvent(t, events[1], "start", 5, "", ".sunshine/tasks/11-09-demo/", "2026-11-08T10:00:06.000Z")
	mustContainWarning(t, warnings, "保留历史恢复了 1 轮")
}
