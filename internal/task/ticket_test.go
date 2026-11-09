package task

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func writeTicket(t *testing.T, root, name string, ticket map[string]any) {
	t.Helper()
	dir := filepath.Join(root, DirSunshine, ".runtime", "shell-tickets")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	data, err := json.Marshal(ticket)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, name), data, 0o644); err != nil {
		t.Fatal(err)
	}
}

func freshTicket(root, key, subcommand, taskRef string) map[string]any {
	now := float64(time.Now().UnixNano()) / 1e9
	subs := []any{map[string]any{"name": subcommand}}
	if taskRef != "" {
		subs[0].(map[string]any)["task_ref"] = taskRef
	}
	return map[string]any{
		"context_key":      key,
		"cwd":              root,
		"created_at_epoch": now,
		"expires_at_epoch": now + 30,
		"subcommands":      subs,
	}
}

func TestTicketAccepted(t *testing.T) {
	root := newTestRoot(t)
	ticket := freshTicket(root, "claude_test_123", "start", ".sunshine/tasks/11-09-退款原因必填")
	writeTicket(t, root, "1-abc.json", ticket)
	// 任务目录不存在时 task_ref 按规范化名字比较，仍应匹配。
	if got := lookupShellTicketContextKey(root, "start", ".sunshine/tasks/11-09-退款原因必填"); got != "claude_test_123" {
		t.Errorf("fresh matching ticket = %q, want claude_test_123", got)
	}
	// current 需要票据的 subcommands 里含 current（与 start 互不通用）。
	writeTicket(t, root, "2-cur.json", freshTicket(root, "claude_cur_456", "current", ""))
	if got := lookupShellTicketContextKey(root, "current", ""); got != "claude_cur_456" {
		t.Errorf("current should match current ticket, got %q", got)
	}
}

func TestTicketExpired(t *testing.T) {
	root := newTestRoot(t)
	now := float64(time.Now().UnixNano()) / 1e9
	ticket := freshTicket(root, "claude_old", "current", "")
	ticket["created_at_epoch"] = now - 120
	ticket["expires_at_epoch"] = now - 60
	writeTicket(t, root, "1-old.json", ticket)
	if got := lookupShellTicketContextKey(root, "current", ""); got != "" {
		t.Errorf("expired ticket should not match, got %q", got)
	}
	if _, err := os.Stat(filepath.Join(root, DirSunshine, ".runtime", "shell-tickets", "1-old.json")); !os.IsNotExist(err) {
		t.Errorf("expired ticket file should be deleted")
	}
}

func TestTicketWrongCWD(t *testing.T) {
	root := newTestRoot(t)
	ticket := freshTicket(root, "claude_other", "current", "")
	ticket["cwd"] = "/some/other/repo"
	writeTicket(t, root, "1-other.json", ticket)
	if got := lookupShellTicketContextKey(root, "current", ""); got != "" {
		t.Errorf("ticket with cwd outside repo should not match, got %q", got)
	}
}

func TestTicketCommandMismatch(t *testing.T) {
	root := newTestRoot(t)
	// 票据只含 finish，start 不应匹配。
	ticket := freshTicket(root, "claude_finish_only", "finish", "")
	writeTicket(t, root, "1-f.json", ticket)
	if got := lookupShellTicketContextKey(root, "start", ".sunshine/tasks/11-09-x"); got != "" {
		t.Errorf("start should not match finish-only ticket, got %q", got)
	}
	if got := lookupShellTicketContextKey(root, "finish", ""); got != "claude_finish_only" {
		t.Errorf("finish should match, got %q", got)
	}
	// start 的 task_ref 不匹配。
	ticket2 := freshTicket(root, "claude_start_other", "start", ".sunshine/tasks/11-09-另一个任务")
	writeTicket(t, root, "2-s.json", ticket2)
	if got := lookupShellTicketContextKey(root, "start", ".sunshine/tasks/11-09-退款原因必填"); got != "" {
		t.Errorf("start with different task_ref should not match, got %q", got)
	}
}

func TestTicketTwoKeysAmbiguous(t *testing.T) {
	root := newTestRoot(t)
	writeTicket(t, root, "1-a.json", freshTicket(root, "key_a", "current", ""))
	writeTicket(t, root, "2-b.json", freshTicket(root, "key_b", "current", ""))
	if got := lookupShellTicketContextKey(root, "current", ""); got != "" {
		t.Errorf("two matching tickets with different keys should degrade to empty, got %q", got)
	}
}

func TestTicketOldCursorShellDirRead(t *testing.T) {
	root := newTestRoot(t)
	dir := filepath.Join(root, DirSunshine, ".runtime", "cursor-shell")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	data, err := json.Marshal(freshTicket(root, "cursor_old_key", "current", ""))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "1-old.json"), data, 0o644); err != nil {
		t.Fatal(err)
	}
	if got := lookupShellTicketContextKey(root, "current", ""); got != "cursor_old_key" {
		t.Errorf("old cursor-shell dir should still be read, got %q", got)
	}
}

func TestTicketNoTimestampsIsFresh(t *testing.T) {
	root := newTestRoot(t)
	ticket := map[string]any{
		"context_key": "no_ts_key",
		"cwd":         root,
		"subcommands": []any{map[string]any{"name": "current"}},
	}
	writeTicket(t, root, "1-nts.json", ticket)
	if got := lookupShellTicketContextKey(root, "current", ""); got != "no_ts_key" {
		t.Errorf("ticket without timestamps should be treated as fresh, got %q", got)
	}
}
