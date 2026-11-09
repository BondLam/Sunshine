package adapters

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/linrong1982/Sunshine/internal/mem"
)

func grokFixture(t *testing.T, home string) string {
	t.Helper()
	sessionDir := filepath.Join(home, ".grok", "sessions", "%2Ftmp%2Fproj", "sess-1")
	writeFixture(t, filepath.Join(sessionDir, "summary.json"), `{
  "info": {"id": "grok-1", "cwd": "/tmp/proj"},
  "session_summary": "  Grok 摘要  ",
  "created_at": "2026-11-08T10:00:00.000Z",
  "updated_at": "2026-11-08T11:00:00.000Z"
}`)
	// 同目录放一个搜索索引文件：适配器必须不读它。
	writeFixture(t, filepath.Join(sessionDir, "session_search.sqlite"), "不是真正的数据库")
	file := filepath.Join(sessionDir, "chat_history.jsonl")
	writeFixture(t, file, `{"type":"user","content":"第一问"}`+"\n"+
		`{"type":"assistant","content":"回答内容","tool_calls":[{"name":"run_terminal_command","arguments":"{\"command\":\"ls\"}"}]}`+"\n"+
		`{"type":"reasoning","content":"加密思维链"}`+"\n"+
		`{"type":"tool_result","content":"工具结果"}`+"\n"+
		`{"type":"user","synthetic_reason":"reminder","content":"注入提醒，不应出现"}`+"\n"+
		`{"type":"user","synthetic_reason":"compaction_meta","content":"压缩摘要"}`+"\n"+
		`{"type":"system","content":"系统提示词"}`+"\n"+
		`{"type":"assistant","content":[{"type":"text","text":"压缩后的回答"}]}`+"\n"+
		`{"type":"backend_tool_call","content":"后台调用"}`+"\n"+
		`坏行`+"\n")
	return file
}

func TestGrokSessions(t *testing.T) {
	home := newTempHome(t)
	file := grokFixture(t, home)
	sessions, warnings, err := GrokAdapter{}.Sessions(context.Background(), home)
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
	if s.ID != "grok-1" || s.Title != "Grok 摘要" || s.CWD != "/tmp/proj" {
		t.Errorf("会话 = %+v", s)
	}
	if s.Created != "2026-11-08T10:00:00.000Z" || s.Updated != "2026-11-08T11:00:00.000Z" {
		t.Errorf("created/updated = %q/%q", s.Created, s.Updated)
	}
	if s.FilePath != file {
		t.Errorf("filePath = %q", s.FilePath)
	}
}

func TestGrokSessionsCwdFromDir(t *testing.T) {
	home := newTempHome(t)
	sessionDir := filepath.Join(home, ".grok", "sessions", "%2Ftmp%2Fproj2", "sess-9")
	writeFixture(t, filepath.Join(sessionDir, "chat_history.jsonl"), `{"type":"user","content":"你好"}`+"\n")
	sessions, _, err := GrokAdapter{}.Sessions(context.Background(), home)
	if err != nil || len(sessions) != 1 {
		t.Fatalf("Sessions 失败：%v，%d", err, len(sessions))
	}
	if sessions[0].ID != "sess-9" {
		t.Errorf("无 summary 时 id 应取目录名，实际 %q", sessions[0].ID)
	}
	if sessions[0].CWD != "/tmp/proj2" {
		t.Errorf("无 summary 时 cwd 应从目录名反解，实际 %q", sessions[0].CWD)
	}
	if sessions[0].Updated == "" {
		t.Error("无 summary 时 updated 应取文件 mtime")
	}
}

func TestGrokDialogue(t *testing.T) {
	home := newTempHome(t)
	file := grokFixture(t, home)
	ctx := context.Background()
	sessions, _, err := GrokAdapter{}.Sessions(ctx, home)
	if err != nil || len(sessions) != 1 {
		t.Fatalf("建夹具会话失败：%v，%d", err, len(sessions))
	}
	before := hashFile(t, file)
	turns, warnings, err := GrokAdapter{}.Dialogue(ctx, sessions[0])
	if err != nil {
		t.Fatalf("Dialogue 返回错误：%v", err)
	}
	if before != hashFile(t, file) {
		t.Error("只读验证：Grok JSONL 文件实测前后哈希变化")
	}
	// 期望：用户首问、助手回答、压缩 marker、压缩后助手回答；
	// reasoning / tool_result / system / 后台调用 / 注入提醒全部丢弃。
	if len(turns) != 4 {
		t.Fatalf("轮次数 = %d，想要 4：%+v", len(turns), turns)
	}
	if turns[0].Role != "user" || turns[0].Text != "第一问" {
		t.Errorf("第 1 轮 = %+v", turns[0])
	}
	if turns[1].Role != "assistant" || turns[1].Text != "回答内容" {
		t.Errorf("第 2 轮 = %+v", turns[1])
	}
	if turns[2].Kind != "marker" || !contains(turns[2].Text, "压缩摘要") {
		t.Errorf("第 3 轮应为压缩 marker 且含摘要，实际 %+v", turns[2])
	}
	if turns[3].Role != "assistant" || turns[3].Text != "压缩后的回答" {
		t.Errorf("第 4 轮 = %+v", turns[3])
	}
	mustContainWarning(t, warnings, "压缩了 1 次")
	mustContainWarning(t, warnings, "跳过 1 行")
}

func grokCollectFixture(t *testing.T, home string) mem.MemSessionInfo {
	t.Helper()
	sessionDir := filepath.Join(home, ".grok", "sessions", "%2Ftmp%2Fproj", "grok-phase")
	writeFixture(t, filepath.Join(sessionDir, "summary.json"), `{
  "info": {"id": "grok-phase", "cwd": "/tmp/proj"},
  "session_summary": "阶段切片会话",
  "created_at": "2026-11-08T10:00:00.000Z",
  "updated_at": "2026-11-08T11:00:00.000Z"
}`)
	file := filepath.Join(sessionDir, "chat_history.jsonl")
	writeFixture(t, file, `{"type":"user","content":"先想想做什么"}`+"\n"+
		`{"type":"assistant","content":"方案草稿","tool_calls":[{"name":"run_terminal_command","arguments":"{\"command\":\"sunshine task create --slug demo\"}"}]}`+"\n"+
		`{"type":"user","content":"方案定了"}`+"\n"+
		`{"type":"assistant","content":"写代码中","tool_calls":[{"name":"other_tool","arguments":"{\"command\":\"sunshine task create --slug ignored\"}"},{"name":"run_terminal_command","arguments":"sunshine task start .sunshine/tasks/11-09-demo/"}]}`+"\n"+
		`{"type":"user","content":"收尾"}`+"\n")
	return mem.MemSessionInfo{Platform: mem.MemSourceGrok, ID: "grok-phase", CWD: "/tmp/proj", FilePath: file}
}

func TestGrokCollect(t *testing.T) {
	home := newTempHome(t)
	s := grokCollectFixture(t, home)
	ctx := context.Background()

	dTurns, _, err := GrokAdapter{}.Dialogue(ctx, s)
	if err != nil {
		t.Fatalf("Dialogue 返回错误：%v", err)
	}
	turns, events, _, err := GrokAdapter{}.Collect(ctx, s)
	if err != nil {
		t.Fatalf("Collect 返回错误：%v", err)
	}
	assertSameTurns(t, turns, dTurns)
	assertNoCommandInTurnText(t, turns)
	if len(turns) != 5 {
		t.Fatalf("轮次数 = %d，想要 5：%+v", len(turns), turns)
	}
	// 非 run_terminal_command 的工具调用不应产出事件。
	if len(events) != 2 {
		t.Fatalf("事件数 = %d，想要 2：%+v", len(events), events)
	}
	// Grok 事件不带时间戳。
	assertEvent(t, events[0], "create", 1, "demo", "", "")
	assertEvent(t, events[1], "start", 3, "", ".sunshine/tasks/11-09-demo/", "")

	brain, _, _ := mem.SliceMemPhase(turns, events, mem.PhaseBrainstorm, mem.MemSourceGrok)
	if len(brain) != 2 || brain[0].Text != "方案草稿" || brain[1].Text != "方案定了" {
		t.Errorf("brainstorm 应只出窗口内 2 轮，实际：%v", brain)
	}
	impl, _, _ := mem.SliceMemPhase(turns, events, mem.PhaseImplement, mem.MemSourceGrok)
	if len(impl) != 3 || impl[0].Text != "先想想做什么" || impl[2].Text != "收尾" {
		t.Errorf("implement 应只出窗口外 3 轮，实际：%v", impl)
	}
}
