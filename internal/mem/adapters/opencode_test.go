package adapters

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/linrong007/Sunshine/internal/mem"
)

func opencodeFixtureDB(t *testing.T, path string) {
	t.Helper()
	db := openFixtureDB(t, path)
	defer db.Close()
	execFixtureSQL(t, db,
		`CREATE TABLE session (id TEXT, parent_id TEXT, title TEXT, directory TEXT, time_created INTEGER, time_updated INTEGER)`,
		`CREATE TABLE message (id TEXT, session_id TEXT, time_created INTEGER, data TEXT)`,
		`CREATE TABLE part (message_id TEXT, session_id TEXT, time_created INTEGER, data TEXT)`,
		`INSERT INTO session VALUES ('oc-1', '', 'OC 会话', '/tmp/proj', 1762600000000, 1762603600000)`,
		`INSERT INTO session VALUES ('oc-child', 'oc-1', '子会话', '/tmp/proj', 1762600100000, 1762600200000)`,
		`INSERT INTO message VALUES ('m1', 'oc-1', 1762600000000, '{"role":"user"}')`,
		`INSERT INTO message VALUES ('m2', 'oc-1', 1762600100000, '{"role":"assistant"}')`,
		`INSERT INTO message VALUES ('m3', 'oc-1', 1762600200000, '{"role":"assistant"}')`,
		`INSERT INTO part VALUES ('m1', 'oc-1', 1762600000000, '{"type":"text","text":"你好"}')`,
		`INSERT INTO part VALUES ('m2', 'oc-1', 1762600100000, '{"type":"text","text":"你好呀"}')`,
		`INSERT INTO part VALUES ('m2', 'oc-1', 1762600101000, '{"type":"tool","text":"工具调用不是对话"}')`,
		`INSERT INTO part VALUES ('m2', 'oc-1', 1762600102000, '{"type":"compaction","replace":true,"summaryMessageId":"m3"}')`,
		`INSERT INTO part VALUES ('m3', 'oc-1', 1762600200000, '{"type":"text","text":"摘要文本"}')`,
		`INSERT INTO part VALUES ('m3', 'oc-1', 1762600201000, '{"type":"compaction","tail_start_id":"m1"}')`,
	)
}

func TestOpencodeSessionsAndDialogue(t *testing.T) {
	home := newTempHome(t)
	xdg := filepath.Join(home, "xdg")
	t.Setenv("XDG_DATA_HOME", xdg)
	t.Setenv("OPENCODE_DB", "")
	dbPath := filepath.Join(xdg, "opencode", "opencode.db")
	opencodeFixtureDB(t, dbPath)
	ctx := context.Background()

	before := hashFile(t, dbPath)
	sessions, warnings, err := OpencodeAdapter{}.Sessions(ctx, home)
	if err != nil {
		t.Fatalf("Sessions 返回错误：%v", err)
	}
	if len(warnings) != 0 {
		t.Errorf("正常夹具不应有警告，实际：%v", warnings)
	}
	if len(sessions) != 2 {
		t.Fatalf("会话数 = %d，想要 2", len(sessions))
	}
	var oc1 map[string]string
	var oc1Session *mem.MemSessionInfo
	for i := range sessions {
		s := sessions[i]
		if s.ID == "oc-1" {
			oc1 = map[string]string{"title": s.Title, "cwd": s.CWD, "created": s.Created, "updated": s.Updated, "file": s.FilePath, "parent": s.ParentID}
			oc1Session = &sessions[i]
		}
		if s.ID == "oc-child" && s.ParentID != "oc-1" {
			t.Errorf("子会话 parent_id = %q，想要 oc-1", s.ParentID)
		}
	}
	if oc1 == nil {
		t.Fatal("未列出 oc-1")
	}
	if oc1["title"] != "OC 会话" || oc1["cwd"] != "/tmp/proj" || oc1["parent"] != "" {
		t.Errorf("oc-1 = %+v", oc1)
	}
	if oc1["created"] != "2025-11-08T11:06:40.000Z" || oc1["updated"] != "2025-11-08T12:06:40.000Z" {
		t.Errorf("oc-1 created/updated = %q/%q（毫秒时间戳应转 ISO）", oc1["created"], oc1["updated"])
	}
	if oc1["file"] != dbPath {
		t.Errorf("filePath = %q", oc1["file"])
	}

	turns, warnings, err := OpencodeAdapter{}.Dialogue(ctx, *oc1Session)
	if err != nil {
		t.Fatalf("Dialogue 返回错误：%v", err)
	}
	if len(warnings) != 0 {
		t.Errorf("正常夹具对话不应有警告，实际：%v", warnings)
	}
	if before != hashFile(t, dbPath) {
		t.Error("只读验证：OpenCode 会话库实测前后哈希变化")
	}
	// 期望：用户、助手、压缩 marker（m3 由 marker part 与 compaction part 双重标注）。
	if len(turns) != 3 {
		t.Fatalf("轮次数 = %d，想要 3：%+v", len(turns), turns)
	}
	if turns[0].Role != "user" || turns[0].Text != "你好" {
		t.Errorf("第 1 轮 = %+v", turns[0])
	}
	if turns[1].Role != "assistant" || turns[1].Text != "你好呀" {
		t.Errorf("第 2 轮 = %+v（tool part 应丢弃）", turns[1])
	}
	if turns[2].Kind != "marker" || !contains(turns[2].Text, "摘要文本") {
		t.Errorf("第 3 轮应为压缩 marker 且含摘要，实际 %+v", turns[2])
	}
}

func TestOpencodeChannelFallback(t *testing.T) {
	home := newTempHome(t)
	xdg := filepath.Join(home, "xdg")
	t.Setenv("XDG_DATA_HOME", xdg)
	t.Setenv("OPENCODE_DB", "")
	// 只有 channel 库，没有主库。
	dbPath := filepath.Join(xdg, "opencode", "opencode-beta.db")
	opencodeFixtureDB(t, dbPath)
	sessions, _, err := OpencodeAdapter{}.Sessions(context.Background(), home)
	if err != nil || len(sessions) != 2 {
		t.Errorf("无主库时应取 channel 库，实际 %d 条，err=%v", len(sessions), err)
	}
}

func TestOpencodeSchemaUnsupported(t *testing.T) {
	home := newTempHome(t)
	dbPath := filepath.Join(home, "bad.db")
	db := openFixtureDB(t, dbPath)
	defer db.Close()
	execFixtureSQL(t, db, `CREATE TABLE session (id TEXT)`)
	ctx := context.Background()
	// 直接用 Dialogue 测结构校验：message 表缺失应警告并返回空对话，不报错。
	turns, warnings, err := OpencodeAdapter{}.Dialogue(ctx, mem.MemSessionInfo{
		Platform: mem.MemSourceOpencode,
		ID:       "x",
		FilePath: dbPath,
	})
	if err != nil {
		t.Fatalf("不应返回错误：%v", err)
	}
	if len(turns) != 0 {
		t.Errorf("结构不支持时应返回空对话，实际 %d 轮", len(turns))
	}
	mustContainWarning(t, warnings, "不支持的 OpenCode 会话库结构")
}

func TestOpencodeCollect_NoEvents(t *testing.T) {
	home := newTempHome(t)
	xdg := filepath.Join(home, "xdg")
	t.Setenv("XDG_DATA_HOME", xdg)
	t.Setenv("OPENCODE_DB", "")
	dbPath := filepath.Join(xdg, "opencode", "opencode.db")
	db := openFixtureDB(t, dbPath)
	defer db.Close()
	execFixtureSQL(t, db,
		`CREATE TABLE session (id TEXT, parent_id TEXT, title TEXT, directory TEXT, time_created INTEGER, time_updated INTEGER)`,
		`CREATE TABLE message (id TEXT, session_id TEXT, time_created INTEGER, data TEXT)`,
		`CREATE TABLE part (message_id TEXT, session_id TEXT, time_created INTEGER, data TEXT)`,
		`INSERT INTO session VALUES ('oc-phase', '', 'OC 阶段会话', '/tmp/proj', 1762600000000, 1762603600000)`,
		`INSERT INTO message VALUES ('m1', 'oc-phase', 1762600000000, '{"role":"user"}')`,
		`INSERT INTO message VALUES ('m2', 'oc-phase', 1762600100000, '{"role":"assistant"}')`,
		`INSERT INTO message VALUES ('m3', 'oc-phase', 1762600200000, '{"role":"user"}')`,
		`INSERT INTO part VALUES ('m1', 'oc-phase', 1762600000000, '{"type":"text","text":"先想想做什么"}')`,
		`INSERT INTO part VALUES ('m2', 'oc-phase', 1762600100000, '{"type":"text","text":"方案草稿"}')`,
		`INSERT INTO part VALUES ('m2', 'oc-phase', 1762600101000, '{"type":"tool","tool":"Bash","state":{"input":{"command":"sunshine task create --slug demo"}}}')`,
		`INSERT INTO part VALUES ('m3', 'oc-phase', 1762600200000, '{"type":"text","text":"方案定了"}')`,
	)
	ctx := context.Background()
	s := mem.MemSessionInfo{Platform: mem.MemSourceOpencode, ID: "oc-phase", CWD: "/tmp/proj", FilePath: dbPath}

	dTurns, _, err := OpencodeAdapter{}.Dialogue(ctx, s)
	if err != nil {
		t.Fatalf("Dialogue 返回错误：%v", err)
	}
	turns, events, _, err := OpencodeAdapter{}.Collect(ctx, s)
	if err != nil {
		t.Fatalf("Collect 返回错误：%v", err)
	}
	assertSameTurns(t, turns, dTurns)
	assertNoCommandInTurnText(t, turns)
	if len(turns) != 3 {
		t.Fatalf("轮次数 = %d，想要 3：%+v", len(turns), turns)
	}
	// OpenCode 不抽事件：即使 tool part 里有任务命令，事件也恒为空。
	if len(events) != 0 {
		t.Errorf("OpenCode 事件应为空，实际：%+v", events)
	}
	// 降级：--phase brainstorm 也返回全文 + 警告。
	got, _, warnings := mem.SliceMemPhase(turns, events, mem.PhaseBrainstorm, mem.MemSourceOpencode)
	if len(got) != 3 {
		t.Errorf("OpenCode 降级时应返回全文 3 轮，实际 %d 轮", len(got))
	}
	mustContainWarning(t, warnings, "暂不支持")
}
