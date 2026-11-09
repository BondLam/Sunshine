package task

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func sessionOpts(title string) AddSessionOptions {
	return AddSessionOptions{
		Title:    title,
		Commit:   "-",
		Summary:  "测试摘要",
		NoCommit: true,
	}
}

func journalFiles(t *testing.T, devDir string) []string {
	t.Helper()
	entries, err := os.ReadDir(devDir)
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), "journal-") {
			names = append(names, e.Name())
		}
	}
	return names
}

func TestAddSessionBasic(t *testing.T) {
	root := newTestRoot(t)
	r, _, _ := newTestRunner(t, root)
	devDir := filepath.Join(root, DirSunshine, "workspace", "tester")

	if code := r.AddSession(sessionOpts("第一次会话")); code != 0 {
		t.Fatalf("add-session failed: %d", code)
	}
	journal := filepath.Join(devDir, "journal-1.md")
	data, err := os.ReadFile(journal)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "## 会话 1：第一次会话") {
		t.Errorf("journal should contain session heading, got:\n%s", data)
	}
	if !strings.Contains(string(data), "<!-- sunshine-session: v=2 fp=") {
		t.Errorf("journal should contain v2 fingerprint marker, got:\n%s", data)
	}
	index, err := os.ReadFile(filepath.Join(devDir, "index.md"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(index), "**总会话数**：1") {
		t.Errorf("index should record 1 session, got:\n%s", index)
	}
	if !strings.Contains(string(index), "| 1 | 2026-11-09 | 第一次会话 |") {
		t.Errorf("index history should have session row, got:\n%s", index)
	}
}

func TestAddSessionIdempotent(t *testing.T) {
	root := newTestRoot(t)
	r, _, _ := newTestRunner(t, root)
	devDir := filepath.Join(root, DirSunshine, "workspace", "tester")

	opts := sessionOpts("幂等会话")
	opts.IdempotencyKey = "key-1"
	if code := r.AddSession(opts); code != 0 {
		t.Fatalf("first add-session failed: %d", code)
	}
	if code := r.AddSession(opts); code != 0 {
		t.Fatalf("second add-session (same key) failed: %d", code)
	}
	journal, _ := os.ReadFile(filepath.Join(devDir, "journal-1.md"))
	if n := strings.Count(string(journal), "## 会话 1：幂等会话"); n != 1 {
		t.Errorf("session heading appears %d times, want 1", n)
	}
	// 完全相同的命令（同 key）第三次重跑：仍不追加第二条。
	if code := r.AddSession(opts); code != 0 {
		t.Fatalf("third add-session (same key) failed: %d", code)
	}
	journal, _ = os.ReadFile(filepath.Join(devDir, "journal-1.md"))
	if n := strings.Count(string(journal), "## 会话 1：幂等会话"); n != 1 {
		t.Errorf("after third replay, session heading appears %d times, want 1", n)
	}
	index, _ := os.ReadFile(filepath.Join(devDir, "index.md"))
	if !strings.Contains(string(index), "**总会话数**：1") {
		t.Errorf("index should still record 1 session after replay, got:\n%s", index)
	}
}

func TestAddSessionBadCommitWritesNothing(t *testing.T) {
	root := newTestRoot(t)
	r, _, _ := newTestRunner(t, root)
	devDir := filepath.Join(root, DirSunshine, "workspace", "tester")

	opts := sessionOpts("坏提交")
	opts.Commit = "deadbee"
	if code := r.AddSession(opts); code == 0 {
		t.Fatalf("add-session with unresolvable commit should fail")
	}
	if _, err := os.Stat(filepath.Join(devDir, "journal-1.md")); !os.IsNotExist(err) {
		t.Errorf("journal-1.md should not exist after failed add-session")
	}
	if _, err := os.Stat(filepath.Join(devDir, "index.md")); !os.IsNotExist(err) {
		t.Errorf("index.md should not exist after failed add-session")
	}
}

func TestAddSessionBadIdempotencyKey(t *testing.T) {
	root := newTestRoot(t)
	r, _, _ := newTestRunner(t, root)
	opts := sessionOpts("坏 key")
	opts.IdempotencyKey = "bad key with spaces!"
	if code := r.AddSession(opts); code == 0 {
		t.Errorf("bad idempotency key should fail")
	}
}

func TestJournalRotation(t *testing.T) {
	root := newTestRoot(t)
	// max_journal_lines: 10 行，迫使轮换。
	cfgPath := filepath.Join(root, DirSunshine, "config.yaml")
	if err := os.WriteFile(cfgPath, []byte("max_journal_lines: 10\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	r, _, _ := newTestRunner(t, root)
	devDir := filepath.Join(root, DirSunshine, "workspace", "tester")

	for i := 1; i <= 3; i++ {
		opts := sessionOpts("会话 " + string(rune('0'+i)))
		opts.IdempotencyKey = "rot-" + string(rune('0'+i))
		if code := r.AddSession(opts); code != 0 {
			t.Fatalf("add-session %d failed: %d", i, code)
		}
	}
	files := journalFiles(t, devDir)
	if len(files) < 2 {
		t.Fatalf("expected journal rotation into multiple files, got %v", files)
	}
	var found int
	for _, name := range files {
		data, _ := os.ReadFile(filepath.Join(devDir, name))
		found += strings.Count(string(data), "## 会话 ")
	}
	if found != 3 {
		t.Errorf("expected 3 sessions across journal files, found %d in %v", found, files)
	}
	index, _ := os.ReadFile(filepath.Join(devDir, "index.md"))
	if !strings.Contains(string(index), "**总会话数**：3") {
		t.Errorf("index should record 3 sessions, got:\n%s", index)
	}
}

func TestAddSessionCommitSubjectOverride(t *testing.T) {
	root := newTestRoot(t)
	r, _, _ := newTestRunner(t, root)
	devDir := filepath.Join(root, DirSunshine, "workspace", "tester")

	opts := sessionOpts("带提交主题")
	opts.Commit = "abc1234"
	opts.CommitSubjects = []string{"abc1234=修复了退款校验"}
	if code := r.AddSession(opts); code != 0 {
		t.Fatalf("add-session with --commit-subject failed: %d", code)
	}
	journal, _ := os.ReadFile(filepath.Join(devDir, "journal-1.md"))
	if !strings.Contains(string(journal), "`abc1234`") || !strings.Contains(string(journal), "修复了退款校验") {
		t.Errorf("journal should contain commit evidence, got:\n%s", journal)
	}
	// --commit-subject 与 --commit 不对应时失败。
	opts2 := sessionOpts("错配主题")
	opts2.Commit = "abc1234"
	opts2.CommitSubjects = []string{"def5678=另一提交"}
	if code := r.AddSession(opts2); code == 0 {
		t.Errorf("mismatched --commit-subject should fail")
	}
}
