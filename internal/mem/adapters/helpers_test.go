package adapters

import (
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/linrong007/Sunshine/internal/mem"
	_ "modernc.org/sqlite"
)

// newTempHome 建一个临时 HOME 夹具目录。
func newTempHome(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	return home
}

// writeFixture 写一个夹具文件（父目录自动创建）。
func writeFixture(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("创建夹具目录失败：%v", err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("写夹具文件失败：%v", err)
	}
}

// hashFile 算一个文件的 SHA-256。
func hashFile(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("读文件算哈希失败（%s）：%v", path, err)
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

// hashFiles 算一组文件的 SHA-256（路径 → 哈希）。
func hashFiles(t *testing.T, paths []string) map[string]string {
	t.Helper()
	out := make(map[string]string, len(paths))
	for _, p := range paths {
		out[p] = hashFile(t, p)
	}
	return out
}

// assertHashesEqual 断言两组文件哈希一致（只读验证）。
func assertHashesEqual(t *testing.T, before, after map[string]string) {
	t.Helper()
	for path, beforeHash := range before {
		afterHash, ok := after[path]
		if !ok {
			t.Errorf("只读验证：文件 %s 在实测后消失", path)
			continue
		}
		if beforeHash != afterHash {
			t.Errorf("只读验证：文件 %s 实测前后哈希变化（%s → %s），mem 写了会话存储", path, beforeHash, afterHash)
		}
	}
}

// openFixtureDB 以读写方式打开一个夹具 SQLite 库（测试建库用）。
func openFixtureDB(t *testing.T, path string) *sql.DB {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("创建夹具目录失败：%v", err)
	}
	db, err := sql.Open("sqlite", "file:"+path+"?mode=rwc")
	if err != nil {
		t.Fatalf("打开夹具库失败：%v", err)
	}
	return db
}

// execFixtureSQL 在夹具库上执行一批语句，失败即失败测试。
func execFixtureSQL(t *testing.T, db *sql.DB, stmts ...string) {
	t.Helper()
	for _, stmt := range stmts {
		if _, err := db.Exec(stmt); err != nil {
			t.Fatalf("执行夹具 SQL 失败（%s）：%v", stmt, err)
		}
	}
}

// mustContain 断言警告列表里至少有一条包含子串。
func mustContainWarning(t *testing.T, warnings []string, substr string) {
	t.Helper()
	for _, w := range warnings {
		if strings.Contains(w, substr) {
			return
		}
	}
	t.Errorf("警告列表缺少包含 %q 的警告，实际：%v", substr, warnings)
}

func contains(s, substr string) bool {
	return strings.Contains(s, substr)
}

// assertSameTurns 断言 Collect 返回的轮次与 Dialogue 完全一致
// （同一份遍历、同一次清理，不应有任何出入）。
func assertSameTurns(t *testing.T, collectTurns, dialogueTurns []mem.DialogueTurn) {
	t.Helper()
	if len(collectTurns) != len(dialogueTurns) {
		t.Fatalf("Collect 轮次数 = %d，Dialogue 轮次数 = %d：%+v / %+v",
			len(collectTurns), len(dialogueTurns), collectTurns, dialogueTurns)
	}
	for i := range dialogueTurns {
		if collectTurns[i] != dialogueTurns[i] {
			t.Errorf("第 %d 轮不一致：Collect=%+v，Dialogue=%+v", i, collectTurns[i], dialogueTurns[i])
		}
	}
}

// assertNoCommandInTurnText 断言没有任何一轮的文本里出现任务命令：
// 命令只应藏在工具调用块里，轮次文本里没有——这正是事件通道要补的缺口。
func assertNoCommandInTurnText(t *testing.T, turns []mem.DialogueTurn) {
	t.Helper()
	for i, turn := range turns {
		if strings.Contains(turn.Text, "sunshine task") {
			t.Errorf("第 %d 轮文本里不应出现任务命令：%q", i, turn.Text)
		}
	}
}

// assertEvent 断言一个事件的全部字段。
func assertEvent(t *testing.T, ev mem.MemEvent, action string, turnIndex int, slug, taskDir, timestamp string) {
	t.Helper()
	if ev.Action != action || ev.TurnIndex != turnIndex || ev.Slug != slug || ev.TaskDir != taskDir || ev.Timestamp != timestamp {
		t.Errorf("事件应为 {%s %d slug=%q taskDir=%q ts=%q}，实际：%+v",
			action, turnIndex, slug, taskDir, timestamp, ev)
	}
}
