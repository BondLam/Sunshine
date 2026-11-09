package adapters

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/linrong1982/Sunshine/internal/mem"
)

func claudeFixture(t *testing.T, home string) string {
	t.Helper()
	dir := filepath.Join(home, ".claude", "projects", "-tmp-proj")
	writeFixture(t, filepath.Join(dir, "sessions-index.json"), `{
  "entries": [
    {"id": "abc123", "cwd": "/tmp/proj", "created": "2026-11-08T10:00:00.000Z", "title": "测试会话"}
  ]
}`)
	file := filepath.Join(dir, "abc123.jsonl")
	writeFixture(t, file, `{"type":"user","cwd":"/tmp/proj","timestamp":"2026-11-08T10:00:00.000Z","message":{"role":"user","content":"你好，请帮我写个函数"}}`+"\n"+
		`{"type":"assistant","timestamp":"2026-11-08T10:00:05.000Z","message":{"role":"assistant","content":[{"type":"text","text":"好的，这是代码"},{"type":"tool_use","name":"Bash","input":{"command":"ls"}}]}}`+"\n"+
		`这行不是 JSON`+"\n"+
		`{"type":"telemetry","foo":"bar"}`+"\n"+
		`{"type":"user","timestamp":"2026-11-08T10:01:00.000Z","message":{"role":"user","content":"<system-reminder>注入内容</system-reminder>真实内容"}}`+"\n"+
		`{"type":"user","timestamp":"2026-11-08T10:02:00.000Z","message":{"role":"user","content":"# AGENTS.md instructions for this project"}}`+"\n"+
		`{"type":"user","isCompactSummary":true,"timestamp":"2026-11-08T10:03:00.000Z","message":{"role":"user","content":[{"type":"text","text":"摘要内容"}]}}`+"\n")
	return file
}

func TestClaudeSessions(t *testing.T) {
	home := newTempHome(t)
	file := claudeFixture(t, home)
	ctx := context.Background()
	sessions, warnings, err := ClaudeAdapter{}.Sessions(ctx, home)
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
	if s.ID != "abc123" || s.Title != "测试会话" || s.CWD != "/tmp/proj" {
		t.Errorf("会话 = %+v", s)
	}
	if s.Created != "2026-11-08T10:00:00.000Z" {
		t.Errorf("created = %q", s.Created)
	}
	if s.Updated == "" {
		t.Error("updated 不应为空")
	}
	if s.FilePath != file {
		t.Errorf("filePath = %q，想要 %q", s.FilePath, file)
	}
	if s.Platform != "claude" {
		t.Errorf("platform = %q", s.Platform)
	}
}

func TestClaudeSessionsNoIndex(t *testing.T) {
	home := newTempHome(t)
	dir := filepath.Join(home, ".claude", "projects", "-tmp-proj")
	file := filepath.Join(dir, "noindex.jsonl")
	writeFixture(t, file, `{"type":"user","cwd":"/tmp/other","timestamp":"2026-11-08T09:00:00.000Z","message":{"role":"user","content":"你好"}}`+"\n")
	sessions, _, err := ClaudeAdapter{}.Sessions(context.Background(), home)
	if err != nil {
		t.Fatalf("Sessions 返回错误：%v", err)
	}
	if len(sessions) != 1 {
		t.Fatalf("会话数 = %d，想要 1", len(sessions))
	}
	if sessions[0].CWD != "/tmp/other" || sessions[0].Created != "2026-11-08T09:00:00.000Z" {
		t.Errorf("无索引时应从首个事件取 cwd/created，实际 %+v", sessions[0])
	}
}

func TestClaudeDialogue(t *testing.T) {
	home := newTempHome(t)
	file := claudeFixture(t, home)
	ctx := context.Background()
	sessions, _, err := ClaudeAdapter{}.Sessions(ctx, home)
	if err != nil || len(sessions) != 1 {
		t.Fatalf("建夹具会话失败：%v，%d", err, len(sessions))
	}
	before := hashFile(t, file)
	turns, warnings, err := ClaudeAdapter{}.Dialogue(ctx, sessions[0])
	if err != nil {
		t.Fatalf("Dialogue 返回错误：%v", err)
	}
	after := hashFile(t, file)
	if before != after {
		t.Error("只读验证：Claude JSONL 文件实测前后哈希变化")
	}
	if len(warnings) == 0 {
		t.Error("坏行应产生警告")
	}
	// 期望：用户问候、助手回答（tool_use 已丢）、注入剥离后的用户轮次、
	// 压缩 marker；AGENTS.md 启动注入轮次已丢；telemetry 事件已容忍。
	if len(turns) != 4 {
		t.Fatalf("轮次数 = %d，想要 4：%+v", len(turns), turns)
	}
	if turns[0].Role != "user" || turns[0].Text != "你好，请帮我写个函数" {
		t.Errorf("第 1 轮 = %+v", turns[0])
	}
	if turns[1].Role != "assistant" || turns[1].Text != "好的，这是代码" {
		t.Errorf("第 2 轮 = %+v（tool_use 块应丢弃）", turns[1])
	}
	if turns[2].Role != "user" || turns[2].Text != "真实内容" {
		t.Errorf("第 3 轮 = %+v（注入标签应剥离）", turns[2])
	}
	if turns[3].Kind != "marker" || !contains(turns[3].Text, "摘要内容") {
		t.Errorf("第 4 轮应为压缩 marker 且含摘要，实际 %+v", turns[3])
	}
}

// claudeCollectFixture 造一份含真实工具调用的 Claude 会话：
// 任务命令只藏在 Bash tool_use 块里，轮次文本里没有；
// 另有 flag 值陷阱（--slug= 后面的命令名不算）与散文陷阱。
func claudeCollectFixture(t *testing.T, home string) mem.MemSessionInfo {
	t.Helper()
	dir := filepath.Join(home, ".claude", "projects", "-tmp-proj")
	writeFixture(t, filepath.Join(dir, "sessions-index.json"), `{
  "entries": [
    {"id": "phase-1", "cwd": "/tmp/proj", "created": "2026-11-08T10:00:00.000Z", "title": "阶段切片会话"}
  ]
}`)
	file := filepath.Join(dir, "phase-1.jsonl")
	writeFixture(t, file, `{"type":"user","cwd":"/tmp/proj","timestamp":"2026-11-08T10:00:00.000Z","message":{"role":"user","content":"先想想做什么"}}`+"\n"+
		`{"type":"assistant","timestamp":"2026-11-08T10:01:00.000Z","message":{"role":"assistant","content":[{"type":"text","text":"方案草稿"},{"type":"tool_use","name":"Bash","input":{"command":"sunshine task create --slug demo"}}]}}`+"\n"+
		`{"type":"user","timestamp":"2026-11-08T10:02:00.000Z","message":{"role":"user","content":"方案定了，开始动手"}}`+"\n"+
		`{"type":"assistant","timestamp":"2026-11-08T10:03:00.000Z","message":{"role":"assistant","content":[{"type":"text","text":"写代码中"},{"type":"tool_use","name":"Bash","input":{"command":"echo --slug=sunshine task create"}},{"type":"tool_use","name":"Bash","input":{"command":"sunshine task start .sunshine/tasks/11-09-demo/"}}]}}`+"\n"+
		`{"type":"assistant","timestamp":"2026-11-08T10:04:00.000Z","message":{"role":"assistant","content":[{"type":"text","text":"继续实现功能"},{"type":"tool_use","name":"Bash","input":{"command":"sunshine task create a new thing"}}]}}`+"\n"+
		`{"type":"user","timestamp":"2026-11-08T10:05:00.000Z","message":{"role":"user","content":"收尾"}}`+"\n")
	return mem.MemSessionInfo{Platform: mem.MemSourceClaude, ID: "phase-1", CWD: "/tmp/proj", FilePath: file}
}

func TestClaudeCollect(t *testing.T) {
	home := newTempHome(t)
	s := claudeCollectFixture(t, home)
	ctx := context.Background()

	dTurns, _, err := ClaudeAdapter{}.Dialogue(ctx, s)
	if err != nil {
		t.Fatalf("Dialogue 返回错误：%v", err)
	}
	turns, events, _, err := ClaudeAdapter{}.Collect(ctx, s)
	if err != nil {
		t.Fatalf("Collect 返回错误：%v", err)
	}
	assertSameTurns(t, turns, dTurns)
	assertNoCommandInTurnText(t, turns)
	if len(turns) != 6 {
		t.Fatalf("轮次数 = %d，想要 6：%+v", len(turns), turns)
	}
	// flag 值里的命令名与散文句式都不应产出事件。
	if len(events) != 2 {
		t.Fatalf("事件数 = %d，想要 2：%+v", len(events), events)
	}
	assertEvent(t, events[0], "create", 1, "demo", "", "2026-11-08T10:01:00.000Z")
	assertEvent(t, events[1], "start", 3, "", ".sunshine/tasks/11-09-demo/", "2026-11-08T10:03:00.000Z")

	// 按事件切片：brainstorm 只出窗口 [1,3) 内轮次，implement 只出窗口外轮次。
	brain, _, _ := mem.SliceMemPhase(turns, events, mem.PhaseBrainstorm, mem.MemSourceClaude)
	if len(brain) != 2 || brain[0].Text != "方案草稿" || brain[1].Text != "方案定了，开始动手" {
		t.Errorf("brainstorm 应只出窗口内 2 轮，实际：%v", brain)
	}
	impl, _, _ := mem.SliceMemPhase(turns, events, mem.PhaseImplement, mem.MemSourceClaude)
	if len(impl) != 4 || impl[0].Text != "先想想做什么" || impl[3].Text != "收尾" {
		t.Errorf("implement 应只出窗口外 4 轮，实际：%v", impl)
	}
}
