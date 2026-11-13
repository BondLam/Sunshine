package adapters

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/linrong007/Sunshine/internal/mem"
)

func piFixture(t *testing.T, home string) string {
	t.Helper()
	dir := filepath.Join(home, ".pi", "agent", "sessions", "--tmp-proj--")
	file := filepath.Join(dir, "20261108_120000_pi-1.jsonl")
	writeFixture(t, file, `{"type":"session","id":"pi-1","cwd":"/tmp/proj","timestamp":"2026-11-08T10:00:00.000Z"}`+"\n"+
		`{"type":"session_info","id":"e-info","name":"Pi 会话标题"}`+"\n"+
		`{"type":"message","id":"e1","message":{"role":"user","content":"第一问","timestamp":"2026-11-08T10:00:01.000Z"}}`+"\n"+
		`{"type":"message","id":"e2","parentId":"e1","message":{"role":"assistant","content":"回答","timestamp":"2026-11-08T10:00:02.000Z"}}`+"\n"+
		`{"type":"compaction","id":"e3","parentId":"e2","summary":"压缩摘要"}`+"\n"+
		`{"type":"branch_summary","id":"e4","parentId":"e3","summary":"分支摘要"}`+"\n"+
		`{"type":"custom_message","id":"e5","parentId":"e4","content":"自定义消息"}`+"\n"+
		`{"type":"message","id":"e6","parentId":"e5","message":{"role":"compactionSummary","summary":"压缩摘要2"}}`+"\n"+
		`{"type":"message","id":"e-fork","parentId":"e1","message":{"role":"user","content":"分叉内容","timestamp":"2026-11-08T10:00:03.000Z"}}`+"\n"+
		`{"type":"message","id":"e7","parentId":"e6","message":{"role":"assistant","content":[{"type":"text","text":"最后回答"},{"type":"thinking","thinking":"思考不入对话"}]}}`+"\n")
	return file
}

func TestPiSessions(t *testing.T) {
	home := newTempHome(t)
	file := piFixture(t, home)
	sessions, warnings, err := PiAdapter{}.Sessions(context.Background(), home)
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
	if s.ID != "pi-1" || s.Title != "Pi 会话标题" || s.CWD != "/tmp/proj" {
		t.Errorf("会话 = %+v", s)
	}
	if s.Created != "2026-11-08T10:00:00.000Z" {
		t.Errorf("created = %q，想要 header 时间戳", s.Created)
	}
	if s.Updated != "2026-11-08T10:00:03.000Z" {
		t.Errorf("updated = %q，想要最晚消息时间戳（含分叉条目）", s.Updated)
	}
	if s.FilePath != file {
		t.Errorf("filePath = %q", s.FilePath)
	}
}

func TestPiDialogue(t *testing.T) {
	home := newTempHome(t)
	file := piFixture(t, home)
	ctx := context.Background()
	sessions, _, err := PiAdapter{}.Sessions(ctx, home)
	if err != nil || len(sessions) != 1 {
		t.Fatalf("建夹具会话失败：%v，%d", err, len(sessions))
	}
	before := hashFile(t, file)
	turns, warnings, err := PiAdapter{}.Dialogue(ctx, sessions[0])
	if err != nil {
		t.Fatalf("Dialogue 返回错误：%v", err)
	}
	if len(warnings) != 0 {
		t.Errorf("正常夹具对话不应有警告，实际：%v", warnings)
	}
	if before != hashFile(t, file) {
		t.Error("只读验证：Pi JSONL 文件实测前后哈希变化")
	}
	// 期望：沿活动分支 e1→e7；分叉条目 e-fork 不在路径上；
	// session_info 条目不入对话；thinking 块丢弃。
	if len(turns) != 7 {
		t.Fatalf("轮次数 = %d，想要 7：%+v", len(turns), turns)
	}
	want := []struct {
		role, text, kind string
	}{
		{"user", "第一问", ""},
		{"assistant", "回答", ""},
		{"user", "", "marker"},
		{"user", "[branch summary]\n分支摘要", ""},
		{"user", "自定义消息", ""},
		{"user", "", "marker"},
		{"assistant", "最后回答", ""},
	}
	for i, w := range want {
		if turns[i].Role != w.role {
			t.Errorf("第 %d 轮 role = %q，想要 %q", i+1, turns[i].Role, w.role)
		}
		if w.text != "" && turns[i].Text != w.text {
			t.Errorf("第 %d 轮 text = %q，想要 %q", i+1, turns[i].Text, w.text)
		}
		if turns[i].Kind != w.kind {
			t.Errorf("第 %d 轮 kind = %q，想要 %q", i+1, turns[i].Kind, w.kind)
		}
	}
	if !contains(turns[2].Text, "压缩摘要") || !contains(turns[5].Text, "压缩摘要2") {
		t.Errorf("压缩 marker 应含摘要，实际 %q / %q", turns[2].Text, turns[5].Text)
	}
	for _, turn := range turns {
		if contains(turn.Text, "分叉内容") {
			t.Errorf("分叉条目不应出现在活动分支对话里：%q", turn.Text)
		}
	}
}

func TestPiCustomSessionDir(t *testing.T) {
	home := newTempHome(t)
	customDir := filepath.Join(home, "custom-pi-sessions")
	file := filepath.Join(customDir, "20261108_120000_pi-9.jsonl")
	writeFixture(t, file, `{"type":"session","id":"pi-9","cwd":"/tmp/other","timestamp":"2026-11-08T10:00:00.000Z"}`+"\n"+
		`{"type":"message","id":"e1","message":{"role":"user","content":"自定义目录会话"}}`+"\n")
	t.Setenv("PI_CODING_AGENT_SESSION_DIR", customDir)
	sessions, _, err := PiAdapter{}.Sessions(context.Background(), home)
	if err != nil || len(sessions) != 1 {
		t.Fatalf("环境变量指定目录应被读取，实际 %d 条，err=%v", len(sessions), err)
	}
	if sessions[0].ID != "pi-9" {
		t.Errorf("id = %q", sessions[0].ID)
	}
}

func TestPiIDFromFile(t *testing.T) {
	// 上游按首个下划线切分：时间戳本身不含下划线时取到 id。
	if got := piIDFromFile("/x/20261108120000_abc-1.jsonl"); got != "abc-1" {
		t.Errorf("= %q，想要 abc-1", got)
	}
	if got := piIDFromFile("/x/plain.jsonl"); got != "plain" {
		t.Errorf("无下划线时应取整个文件名，实际 %q", got)
	}
}

func piCollectFixture(t *testing.T, home string) mem.MemSessionInfo {
	t.Helper()
	dir := filepath.Join(home, ".pi", "agent", "sessions", "--tmp-proj--")
	file := filepath.Join(dir, "20261108_120000_pi-phase.jsonl")
	writeFixture(t, file, `{"type":"session","id":"pi-phase","cwd":"/tmp/proj","timestamp":"2026-11-08T10:00:00.000Z"}`+"\n"+
		`{"type":"message","id":"e1","message":{"role":"user","content":"先想想做什么","timestamp":"2026-11-08T10:00:01.000Z"}}`+"\n"+
		`{"type":"message","id":"e2","parentId":"e1","timestamp":"2026-11-08T10:01:00.000Z","message":{"role":"assistant","content":[{"type":"text","text":"方案草稿"},{"type":"toolCall","name":"Bash","arguments":{"command":"sunshine task create --slug demo"}}],"timestamp":"2026-11-08T10:01:00.000Z"}}`+"\n"+
		`{"type":"message","id":"e3","parentId":"e2","message":{"role":"user","content":"方案定了","timestamp":"2026-11-08T10:02:00.000Z"}}`+"\n"+
		`{"type":"message","id":"e4","parentId":"e3","timestamp":"2026-11-08T10:03:00.000Z","message":{"role":"bashExecution","command":"sunshine task start .sunshine/tasks/11-09-demo/"},"timestamp":"2026-11-08T10:03:00.000Z"}`+"\n"+
		`{"type":"message","id":"e5","parentId":"e4","message":{"role":"assistant","content":"写代码中","timestamp":"2026-11-08T10:04:00.000Z"}}`+"\n"+
		`{"type":"message","id":"e6","parentId":"e5","message":{"role":"user","content":"收尾","timestamp":"2026-11-08T10:05:00.000Z"}}`+"\n")
	return mem.MemSessionInfo{Platform: mem.MemSourcePi, ID: "pi-phase", CWD: "/tmp/proj", FilePath: file}
}

func TestPiCollect(t *testing.T) {
	home := newTempHome(t)
	s := piCollectFixture(t, home)
	ctx := context.Background()

	dTurns, _, err := PiAdapter{}.Dialogue(ctx, s)
	if err != nil {
		t.Fatalf("Dialogue 返回错误：%v", err)
	}
	turns, events, _, err := PiAdapter{}.Collect(ctx, s)
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
	assertEvent(t, events[0], "create", 1, "demo", "", "2026-11-08T10:01:00.000Z")
	// bashExecution 条目自身不产生轮次，start 事件下标是其前的轮次数。
	assertEvent(t, events[1], "start", 3, "", ".sunshine/tasks/11-09-demo/", "2026-11-08T10:03:00.000Z")

	brain, _, _ := mem.SliceMemPhase(turns, events, mem.PhaseBrainstorm, mem.MemSourcePi)
	if len(brain) != 2 || brain[0].Text != "方案草稿" || brain[1].Text != "方案定了" {
		t.Errorf("brainstorm 应只出窗口内 2 轮，实际：%v", brain)
	}
	impl, _, _ := mem.SliceMemPhase(turns, events, mem.PhaseImplement, mem.MemSourcePi)
	if len(impl) != 3 || impl[0].Text != "先想想做什么" || impl[2].Text != "收尾" {
		t.Errorf("implement 应只出窗口外 3 轮，实际：%v", impl)
	}
}
