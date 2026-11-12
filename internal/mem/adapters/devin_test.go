package adapters

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/BondLam/Sunshine/internal/mem"
)

func devinFixtureDB(t *testing.T, path string) {
	t.Helper()
	db := openFixtureDB(t, path)
	defer db.Close()
	execFixtureSQL(t, db,
		`CREATE TABLE sessions (id TEXT, working_directory TEXT, created_at INTEGER, last_activity_at INTEGER, title TEXT, main_chain_id INTEGER, hidden INTEGER)`,
		`CREATE TABLE message_nodes (session_id TEXT, node_id INTEGER, parent_node_id INTEGER, chat_message TEXT, created_at INTEGER)`,
		`INSERT INTO sessions VALUES ('dev-1', '/tmp/proj', 1762600000, 1762603600, 'Devin 会话', 3, 0)`,
		`INSERT INTO sessions VALUES ('dev-hidden', '/tmp/proj', 1762600000, 1762603600, '隐藏会话', NULL, 1)`,
		`INSERT INTO message_nodes VALUES ('dev-1', 1, NULL, '{"role":"user","content":"你好","metadata":{"is_user_input":true}}', 1762600000)`,
		`INSERT INTO message_nodes VALUES ('dev-1', 2, 1, '{"role":"assistant","content":"你好，我来帮忙"}', 1762600100)`,
		`INSERT INTO message_nodes VALUES ('dev-1', 3, 2, '{"role":"system","content":"摘要文本","metadata":{"extensions":{"devin-rs/summary":{}}}}', 1762600200)`,
		`INSERT INTO message_nodes VALUES ('dev-1', 4, 1, '{"role":"user","content":"分叉内容","metadata":{"is_user_input":true}}', 1762600300)`,
		`INSERT INTO message_nodes VALUES ('dev-1', 5, 2, '{"role":"tool","content":"工具输出"}', 1762600400)`,
		`INSERT INTO message_nodes VALUES ('dev-1', 6, 2, '{"role":"user","content":"非用户输入","metadata":{"is_user_input":false}}', 1762600500)`,
	)
}

func TestDevinSessionsAndDialogue(t *testing.T) {
	home := newTempHome(t)
	xdg := filepath.Join(home, "xdg")
	t.Setenv("XDG_DATA_HOME", xdg)
	t.Setenv("DEVIN_DB_PATH", "")
	dbPath := filepath.Join(xdg, "devin", "cli", "sessions.db")
	devinFixtureDB(t, dbPath)
	ctx := context.Background()

	before := hashFile(t, dbPath)
	sessions, warnings, err := DevinAdapter{}.Sessions(ctx, home)
	if err != nil {
		t.Fatalf("Sessions 返回错误：%v", err)
	}
	if len(warnings) != 0 {
		t.Errorf("正常夹具不应有警告，实际：%v", warnings)
	}
	if len(sessions) != 1 {
		t.Fatalf("会话数 = %d，想要 1（隐藏会话应排除）", len(sessions))
	}
	s := sessions[0]
	if s.ID != "dev-1" || s.Title != "Devin 会话" || s.CWD != "/tmp/proj" {
		t.Errorf("会话 = %+v", s)
	}
	if s.Created != "2025-11-08T11:06:40.000Z" {
		t.Errorf("created = %q（Unix 秒应转 ISO）", s.Created)
	}
	if s.Updated != "2025-11-08T12:06:40.000Z" {
		t.Errorf("updated = %q（应取 last_activity_at）", s.Updated)
	}
	if s.FilePath != dbPath {
		t.Errorf("filePath = %q", s.FilePath)
	}

	turns, warnings, err := DevinAdapter{}.Dialogue(ctx, s)
	if err != nil {
		t.Fatalf("Dialogue 返回错误：%v", err)
	}
	if len(warnings) != 0 {
		t.Errorf("正常夹具对话不应有警告，实际：%v", warnings)
	}
	if before != hashFile(t, dbPath) {
		t.Error("只读验证：Devin 会话库实测前后哈希变化")
	}
	// 期望：主链 1→2→3；分叉节点 4、工具节点 5、非用户输入节点 6 都排除。
	if len(turns) != 3 {
		t.Fatalf("轮次数 = %d，想要 3：%+v", len(turns), turns)
	}
	if turns[0].Role != "user" || turns[0].Text != "你好" {
		t.Errorf("第 1 轮 = %+v", turns[0])
	}
	if turns[1].Role != "assistant" || turns[1].Text != "你好，我来帮忙" {
		t.Errorf("第 2 轮 = %+v", turns[1])
	}
	if turns[2].Kind != "marker" || !contains(turns[2].Text, "摘要文本") {
		t.Errorf("第 3 轮应为压缩 marker 且含摘要，实际 %+v", turns[2])
	}
}

func TestDevinDBPathForms(t *testing.T) {
	home := newTempHome(t)
	ctx := context.Background()
	absPath := filepath.Join(home, "custom-devin.db")
	devinFixtureDB(t, absPath)

	t.Run("绝对路径", func(t *testing.T) {
		t.Setenv("DEVIN_DB_PATH", absPath)
		sessions, _, err := DevinAdapter{}.Sessions(ctx, home)
		if err != nil || len(sessions) != 1 || sessions[0].ID != "dev-1" {
			t.Errorf("绝对路径应读到会话，实际 %d 条，err=%v", len(sessions), err)
		}
	})
	t.Run("~/ 展开", func(t *testing.T) {
		rel := filepath.Join(home, "tilde.db")
		devinFixtureDB(t, rel)
		t.Setenv("DEVIN_DB_PATH", "~/tilde.db")
		sessions, _, err := DevinAdapter{}.Sessions(ctx, home)
		if err != nil || len(sessions) != 1 {
			t.Errorf("~/ 应展开到 HOME，实际 %d 条，err=%v", len(sessions), err)
		}
	})
	t.Run("相对名拼数据目录", func(t *testing.T) {
		xdg := filepath.Join(home, "xdg2")
		t.Setenv("XDG_DATA_HOME", xdg)
		devinFixtureDB(t, filepath.Join(xdg, "devin", "cli", "mine.db"))
		t.Setenv("DEVIN_DB_PATH", "mine.db")
		sessions, _, err := DevinAdapter{}.Sessions(ctx, home)
		if err != nil || len(sessions) != 1 {
			t.Errorf("相对名应拼到数据目录，实际 %d 条，err=%v", len(sessions), err)
		}
	})
	t.Run(":memory: 无文件", func(t *testing.T) {
		t.Setenv("DEVIN_DB_PATH", ":memory:")
		sessions, warnings, err := DevinAdapter{}.Sessions(ctx, home)
		if err != nil {
			t.Fatalf("不应返回错误：%v", err)
		}
		if len(sessions) != 0 || len(warnings) == 0 {
			t.Errorf(":memory: 应返回空列表与警告，实际 %d 条、%d 条警告", len(sessions), len(warnings))
		}
	})
}

func TestDevinMissingMainChain(t *testing.T) {
	home := newTempHome(t)
	dbPath := filepath.Join(home, "devin.db")
	db := openFixtureDB(t, dbPath)
	defer db.Close()
	execFixtureSQL(t, db,
		`CREATE TABLE sessions (id TEXT, working_directory TEXT, created_at INTEGER, last_activity_at INTEGER, title TEXT, main_chain_id INTEGER, hidden INTEGER)`,
		`CREATE TABLE message_nodes (session_id TEXT, node_id INTEGER, parent_node_id INTEGER, chat_message TEXT, created_at INTEGER)`,
		`INSERT INTO sessions VALUES ('dev-nochain', '/tmp/proj', 1762600000, 1762603600, '无主链', NULL, 0)`,
		`INSERT INTO message_nodes VALUES ('dev-nochain', 1, NULL, '{"role":"user","content":"你好","metadata":{"is_user_input":true}}', 1762600000)`,
	)
	ctx := context.Background()
	// 直接用 Dialogue 测缺主链：不猜 max(node_id)，返回空对话与警告。
	turns, warnings, err := DevinAdapter{}.Dialogue(ctx, mem.MemSessionInfo{
		Platform: mem.MemSourceDevin,
		ID:       "dev-nochain",
		FilePath: dbPath,
	})
	if err != nil {
		t.Fatalf("不应返回错误：%v", err)
	}
	if len(turns) != 0 {
		t.Errorf("缺主链时应返回空对话，实际 %d 轮", len(turns))
	}
	mustContainWarning(t, warnings, "main_chain_id")
}

func devinCollectFixtureDB(t *testing.T, path string) {
	t.Helper()
	db := openFixtureDB(t, path)
	defer db.Close()
	execFixtureSQL(t, db,
		`CREATE TABLE sessions (id TEXT, working_directory TEXT, created_at INTEGER, last_activity_at INTEGER, title TEXT, main_chain_id INTEGER, hidden INTEGER)`,
		`CREATE TABLE message_nodes (session_id TEXT, node_id INTEGER, parent_node_id INTEGER, chat_message TEXT, created_at INTEGER)`,
		`INSERT INTO sessions VALUES ('dev-phase', '/tmp/proj', 1762600000, 1762603600, 'Devin 阶段会话', 5, 0)`,
		`INSERT INTO message_nodes VALUES ('dev-phase', 1, NULL, '{"role":"user","content":"先想想做什么","metadata":{"is_user_input":true}}', 1762600000)`,
		`INSERT INTO message_nodes VALUES ('dev-phase', 2, 1, '{"role":"assistant","content":"方案草稿","tool_calls":[{"name":"exec","arguments":"{\"command\":\"sunshine task create --slug demo\"}"},{"name":"read","arguments":"{\"command\":\"sunshine task create --slug ignored\"}"}]}', 1762600100)`,
		`INSERT INTO message_nodes VALUES ('dev-phase', 3, 2, '{"role":"user","content":"方案定了","metadata":{"is_user_input":true}}', 1762600200)`,
		`INSERT INTO message_nodes VALUES ('dev-phase', 4, 3, '{"role":"assistant","content":"写代码中","tool_calls":[{"name":"exec","arguments":{"command":"sunshine task start .sunshine/tasks/11-09-demo/"}},{"name":"exec","arguments":"{}"}]}', 1762600300)`,
		`INSERT INTO message_nodes VALUES ('dev-phase', 5, 4, '{"role":"user","content":"收尾","metadata":{"is_user_input":true}}', 1762600400)`,
	)
}

func TestDevinCollect(t *testing.T) {
	home := newTempHome(t)
	xdg := filepath.Join(home, "xdg")
	t.Setenv("XDG_DATA_HOME", xdg)
	t.Setenv("DEVIN_DB_PATH", "")
	dbPath := filepath.Join(xdg, "devin", "cli", "sessions.db")
	devinCollectFixtureDB(t, dbPath)
	ctx := context.Background()

	s := mem.MemSessionInfo{Platform: mem.MemSourceDevin, ID: "dev-phase", CWD: "/tmp/proj", FilePath: dbPath}
	dTurns, _, err := DevinAdapter{}.Dialogue(ctx, s)
	if err != nil {
		t.Fatalf("Dialogue 返回错误：%v", err)
	}
	turns, events, _, err := DevinAdapter{}.Collect(ctx, s)
	if err != nil {
		t.Fatalf("Collect 返回错误：%v", err)
	}
	assertSameTurns(t, turns, dTurns)
	assertNoCommandInTurnText(t, turns)
	if len(turns) != 5 {
		t.Fatalf("轮次数 = %d，想要 5：%+v", len(turns), turns)
	}
	// 非 exec 工具调用与无 command 的 exec 调用都不应产出事件。
	if len(events) != 2 {
		t.Fatalf("事件数 = %d，想要 2：%+v", len(events), events)
	}
	assertEvent(t, events[0], "create", 1, "demo", "", "2025-11-08T11:08:20.000Z")
	assertEvent(t, events[1], "start", 3, "", ".sunshine/tasks/11-09-demo/", "2025-11-08T11:11:40.000Z")

	brain, _, _ := mem.SliceMemPhase(turns, events, mem.PhaseBrainstorm, mem.MemSourceDevin)
	if len(brain) != 2 || brain[0].Text != "方案草稿" || brain[1].Text != "方案定了" {
		t.Errorf("brainstorm 应只出窗口内 2 轮，实际：%v", brain)
	}
	impl, _, _ := mem.SliceMemPhase(turns, events, mem.PhaseImplement, mem.MemSourceDevin)
	if len(impl) != 3 || impl[0].Text != "先想想做什么" || impl[2].Text != "收尾" {
		t.Errorf("implement 应只出窗口外 3 轮，实际：%v", impl)
	}
}
