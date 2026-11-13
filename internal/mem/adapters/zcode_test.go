package adapters

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"testing"

	"github.com/linrong007/Sunshine/internal/mem"
)

// zcodeFixtureWALDB 建一个 WAL 模式的 ZCode 夹具库，并保持写连接打开，
// 使 -wal 文件留有未检查点的数据（模拟用户正在用的会话库）。
// 返回写连接（调用方负责关闭）与库路径。
func zcodeFixtureWALDB(t *testing.T, path string) (*sql.DB, string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("创建夹具目录失败：%v", err)
	}
	db, err := sql.Open("sqlite", "file:"+path+"?mode=rwc&_pragma=journal_mode(WAL)")
	if err != nil {
		t.Fatalf("打开 WAL 夹具库失败：%v", err)
	}
	db.SetMaxOpenConns(1)
	execFixtureSQL(t, db,
		`CREATE TABLE session (id TEXT, title TEXT, directory TEXT, time_created INTEGER, time_updated INTEGER, task_type TEXT)`,
		`CREATE TABLE message (id TEXT, session_id TEXT, time_created INTEGER, data TEXT)`,
		`CREATE TABLE part (message_id TEXT, time_created INTEGER, data TEXT)`,
		`INSERT INTO session VALUES ('z-1', 'ZCode 会话', '/tmp/proj', 1762600000000, 1762603600000, '')`,
		`INSERT INTO session VALUES ('z-child', '子代理会话', '/tmp/proj', 1762600100000, 1762600200000, 'subagent_child')`,
		`INSERT INTO message VALUES ('m1', 'z-1', 1762600000000, '{"role":"user"}')`,
		`INSERT INTO message VALUES ('m2', 'z-1', 1762600100000, '{"role":"assistant"}')`,
		`INSERT INTO message VALUES ('m3', 'z-1', 1762600200000, '{"role":"assistant"}')`,
		`INSERT INTO part VALUES ('m1', 1762600000000, '{"type":"text","text":"你好"}')`,
		`INSERT INTO part VALUES ('m2', 1762600100000, '{"type":"text","text":"你好呀"}')`,
		`INSERT INTO part VALUES ('m3', 1762600200000, '{"type":"text","text":"摘要文本"}')`,
		`INSERT INTO part VALUES ('m3', 1762600201000, '{"type":"compaction","tail_start_id":"m1"}')`,
	)
	// 再写一行已提交但未检查点的数据，确保 -wal 里有内容。
	if _, err := db.Exec(`INSERT INTO part VALUES ('m2', 1762600102000, '{"type":"text","text":"追加的回答"}')`); err != nil {
		t.Fatalf("追加 WAL 数据失败：%v", err)
	}
	return db, path
}

func TestZcodeSessionsAndDialogueWAL(t *testing.T) {
	home := newTempHome(t)
	dbPath := filepath.Join(home, ".zcode", "cli", "db", "db.sqlite")
	writer, _ := zcodeFixtureWALDB(t, dbPath)
	defer writer.Close()

	walPath := WALPath(dbPath)
	if _, err := os.Stat(walPath); err != nil {
		t.Fatalf("夹具 -wal 文件应存在（写连接保持打开）：%v", err)
	}
	if info, err := os.Stat(walPath); err != nil || info.Size() == 0 {
		t.Fatalf("-wal 文件应非空，实际大小 %d", info.Size())
	}
	ctx := context.Background()
	// 只读验证只比对库文件与 -wal：-shm 是共享内存状态文件，
	// 读连接会改写其中的读标记，不是会话数据，不纳入比对。
	dataFiles := []string{dbPath, walPath}
	before := hashFiles(t, dataFiles)

	sessions, warnings, err := ZcodeAdapter{}.Sessions(ctx, home)
	if err != nil {
		t.Fatalf("Sessions 返回错误：%v", err)
	}
	if len(warnings) != 0 {
		t.Errorf("正常夹具不应有警告，实际：%v", warnings)
	}
	// subagent_child 会话默认排除。
	if len(sessions) != 1 {
		t.Fatalf("会话数 = %d，想要 1（子代理会话应排除）", len(sessions))
	}
	s := sessions[0]
	if s.ID != "z-1" || s.Title != "ZCode 会话" || s.CWD != "/tmp/proj" {
		t.Errorf("会话 = %+v", s)
	}
	if s.Created != "2025-11-08T11:06:40.000Z" || s.Updated != "2025-11-08T12:06:40.000Z" {
		t.Errorf("created/updated = %q/%q", s.Created, s.Updated)
	}

	turns, warnings, err := ZcodeAdapter{}.Dialogue(ctx, s)
	if err != nil {
		t.Fatalf("Dialogue 返回错误：%v", err)
	}
	if len(warnings) != 0 {
		t.Errorf("正常夹具对话不应有警告，实际：%v", warnings)
	}
	after := hashFiles(t, dataFiles)
	assertHashesEqual(t, before, after)
	// 期望：用户、助手（含 -wal 里追加的文本）、压缩 marker。
	if len(turns) != 3 {
		t.Fatalf("轮次数 = %d，想要 3：%+v", len(turns), turns)
	}
	if turns[0].Role != "user" || turns[0].Text != "你好" {
		t.Errorf("第 1 轮 = %+v", turns[0])
	}
	if turns[1].Role != "assistant" || turns[1].Text != "你好呀\n\n追加的回答" {
		t.Errorf("第 2 轮 = %+v（-wal 里追加的文本应被读到）", turns[1])
	}
	if turns[2].Kind != "marker" || !contains(turns[2].Text, "摘要文本") {
		t.Errorf("第 3 轮应为压缩 marker 且含摘要，实际 %+v", turns[2])
	}
}

func TestZcodeMissingDB(t *testing.T) {
	home := newTempHome(t)
	sessions, warnings, err := ZcodeAdapter{}.Sessions(context.Background(), home)
	if err != nil {
		t.Fatalf("不应返回错误：%v", err)
	}
	if len(sessions) != 0 {
		t.Errorf("缺库时应返回空列表，实际 %d 条", len(sessions))
	}
	if len(warnings) == 0 {
		t.Error("缺库时应给出警告")
	}
}

func TestWALPathAndDatabaseFiles(t *testing.T) {
	home := newTempHome(t)
	dbPath := filepath.Join(home, "db.sqlite")
	writeFixture(t, dbPath, "db")
	writeFixture(t, dbPath+"-wal", "wal")
	if got := WALPath(dbPath); got != dbPath+"-wal" {
		t.Errorf("WALPath = %q", got)
	}
	files := DatabaseFiles(dbPath)
	if len(files) != 2 {
		t.Errorf("DatabaseFiles 应识别库文件与 -wal，实际 %v", files)
	}
}

func zcodeCollectFixtureDB(t *testing.T, path string) {
	t.Helper()
	db := openFixtureDB(t, path)
	defer db.Close()
	execFixtureSQL(t, db,
		`CREATE TABLE session (id TEXT, title TEXT, directory TEXT, time_created INTEGER, time_updated INTEGER, task_type TEXT)`,
		`CREATE TABLE message (id TEXT, session_id TEXT, time_created INTEGER, data TEXT)`,
		`CREATE TABLE part (message_id TEXT, time_created INTEGER, data TEXT)`,
		`INSERT INTO session VALUES ('z-phase', 'ZCode 阶段会话', '/tmp/proj', 1762600000000, 1762603600000, '')`,
		`INSERT INTO message VALUES ('m1', 'z-phase', 1762600000000, '{"role":"user"}')`,
		`INSERT INTO message VALUES ('m2', 'z-phase', 1762600100000, '{"role":"assistant"}')`,
		`INSERT INTO message VALUES ('m3', 'z-phase', 1762600200000, '{"role":"user"}')`,
		`INSERT INTO message VALUES ('m4', 'z-phase', 1762600300000, '{"role":"assistant"}')`,
		`INSERT INTO message VALUES ('m5', 'z-phase', 1762600400000, '{"role":"user"}')`,
		`INSERT INTO part VALUES ('m1', 1762600000000, '{"type":"text","text":"先想想做什么"}')`,
		`INSERT INTO part VALUES ('m2', 1762600100000, '{"type":"text","text":"方案草稿"}')`,
		`INSERT INTO part VALUES ('m2', 1762600101000, '{"type":"tool","tool":"Read","state":{"input":{"command":"sunshine task create --slug ignored"}}}')`,
		`INSERT INTO part VALUES ('m2', 1762600102000, '{"type":"tool","tool":"Bash","state":{"input":{"command":"sunshine task create --slug demo"}}}')`,
		`INSERT INTO part VALUES ('m3', 1762600200000, '{"type":"text","text":"方案定了"}')`,
		`INSERT INTO part VALUES ('m4', 1762600300000, '{"type":"text","text":"写代码中"}')`,
		`INSERT INTO part VALUES ('m4', 1762600301000, '{"type":"tool","tool":"Bash","state":{"input":{"command":"sunshine task start .sunshine/tasks/11-09-demo/"}}}')`,
		`INSERT INTO part VALUES ('m4', 1762600302000, '{"type":"tool","tool":"bash","state":{"input":{"command":"sunshine task create --slug late"}}}')`,
		`INSERT INTO part VALUES ('m5', 1762600400000, '{"type":"text","text":"收尾"}')`,
	)
}

func TestZcodeCollect(t *testing.T) {
	home := newTempHome(t)
	dbPath := filepath.Join(home, ".zcode", "cli", "db", "db.sqlite")
	zcodeCollectFixtureDB(t, dbPath)
	ctx := context.Background()

	s := mem.MemSessionInfo{Platform: mem.MemSourceZcode, ID: "z-phase", CWD: "/tmp/proj", FilePath: dbPath}
	dTurns, _, err := ZcodeAdapter{}.Dialogue(ctx, s)
	if err != nil {
		t.Fatalf("Dialogue 返回错误：%v", err)
	}
	turns, events, _, err := ZcodeAdapter{}.Collect(ctx, s)
	if err != nil {
		t.Fatalf("Collect 返回错误：%v", err)
	}
	assertSameTurns(t, turns, dTurns)
	assertNoCommandInTurnText(t, turns)
	if len(turns) != 5 {
		t.Fatalf("轮次数 = %d，想要 5：%+v", len(turns), turns)
	}
	// 非 Bash 工具 part 不应产出事件；小写 bash 应产出事件。
	if len(events) != 3 {
		t.Fatalf("事件数 = %d，想要 3：%+v", len(events), events)
	}
	// ZCode 约定：先入轮次、再记事件，create 下标是入池后的轮次数。
	assertEvent(t, events[0], "create", 2, "demo", "", "2025-11-08T11:08:22.000Z")
	assertEvent(t, events[1], "start", 4, "", ".sunshine/tasks/11-09-demo/", "2025-11-08T11:11:41.000Z")
	assertEvent(t, events[2], "create", 4, "late", "", "2025-11-08T11:11:42.000Z")

	// 窗口：demo [2,4)，late [4,5)；brainstorm 只出窗口内轮次。
	brain, windows, _ := mem.SliceMemPhase(turns, events, mem.PhaseBrainstorm, mem.MemSourceZcode)
	if len(windows) != 2 {
		t.Fatalf("窗口数 = %d，想要 2：%v", len(windows), windows)
	}
	if len(brain) != 3 || brain[0].Text != "方案定了" || brain[1].Text != "写代码中" || brain[2].Text != "收尾" {
		t.Errorf("brainstorm 应只出窗口内 3 轮，实际：%v", brain)
	}
	impl, _, _ := mem.SliceMemPhase(turns, events, mem.PhaseImplement, mem.MemSourceZcode)
	if len(impl) != 2 || impl[0].Text != "先想想做什么" || impl[1].Text != "方案草稿" {
		t.Errorf("implement 应只出窗口外 2 轮，实际：%v", impl)
	}
}
