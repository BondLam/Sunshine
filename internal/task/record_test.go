package task

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func newTestRunner(t *testing.T, root string) (*Runner, *bytes.Buffer, *bytes.Buffer) {
	t.Helper()
	var out, errBuf bytes.Buffer
	r := NewRunner(root, &out, &errBuf)
	r.Now = func() time.Time { return time.Date(2026, 11, 9, 10, 0, 0, 0, time.UTC) }
	return r, &out, &errBuf
}

func newTestRoot(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, DirSunshine), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("SUNSHINE_DEVELOPER", "tester")
	return root
}

func TestEmptyRecordDefaults(t *testing.T) {
	rec := EmptyRecord()
	if rec.Status != "planning" {
		t.Errorf("status = %q, want planning", rec.Status)
	}
	if rec.Priority != "P2" {
		t.Errorf("priority = %q, want P2", rec.Priority)
	}
	if rec.CreatedAt != time.Now().Format("2006-01-02") {
		t.Errorf("createdAt = %q, want today", rec.CreatedAt)
	}
	if rec.Subtasks == nil || len(rec.Subtasks) != 0 {
		t.Errorf("subtasks = %v, want []", rec.Subtasks)
	}
	if rec.Children == nil || len(rec.Children) != 0 {
		t.Errorf("children = %v, want []", rec.Children)
	}
	if rec.RelatedFiles == nil || len(rec.RelatedFiles) != 0 {
		t.Errorf("relatedFiles = %v, want []", rec.RelatedFiles)
	}
	if string(rec.Meta) != "{}" {
		t.Errorf("meta = %s, want {}", rec.Meta)
	}
	if rec.Description != "" || rec.Scope != nil || rec.Package != nil {
		t.Errorf("nullable fields should be empty/nil: description=%q scope=%v package=%v", rec.Description, rec.Scope, rec.Package)
	}
}

func TestMarshalRecordFieldOrder(t *testing.T) {
	rec := EmptyRecord()
	rec.ID = "x"
	rec.Name = "x"
	rec.Title = "标题"
	rec.Description = "描述"
	data, err := MarshalRecord(rec)
	if err != nil {
		t.Fatal(err)
	}
	text := string(data)
	wantOrder := []string{
		`"id"`, `"name"`, `"title"`, `"description"`, `"status"`, `"dev_type"`,
		`"scope"`, `"package"`, `"priority"`, `"creator"`, `"assignee"`,
		`"createdAt"`, `"completedAt"`, `"branch"`, `"base_branch"`,
		`"worktree_path"`, `"commit"`, `"pr_url"`, `"subtasks"`, `"children"`,
		`"parent"`, `"relatedFiles"`, `"notes"`, `"meta"`,
	}
	prev := -1
	for _, key := range wantOrder {
		idx := strings.Index(text, key)
		if idx < 0 {
			t.Fatalf("missing field %s in %s", key, text)
		}
		if idx <= prev {
			t.Fatalf("field %s out of order in %s", key, text)
		}
		prev = idx
	}
	if !strings.HasSuffix(text, "\n") {
		t.Errorf("marshalled record should end with newline")
	}
	if !strings.Contains(text, `"title": "标题"`) {
		t.Errorf("Chinese text should not be HTML-escaped: %s", text)
	}
}

func TestParseRecordMissingField(t *testing.T) {
	// 缺一个必填字段即报错，不补默认值。
	data := []byte(`{"id":"x","name":"x","title":"t","description":"d","status":"planning","dev_type":null,"scope":null,"package":null,"priority":"P2","creator":"a","assignee":"a","createdAt":"2026-11-09","completedAt":null,"branch":null,"base_branch":null,"worktree_path":null,"commit":null,"pr_url":null,"subtasks":[],"children":[],"parent":null,"relatedFiles":[],"notes":null}`)
	if _, err := ParseRecord(data); err == nil {
		t.Errorf("missing meta should error")
	}
}

func TestParseRecordTypeErrors(t *testing.T) {
	base := map[string]any{
		"id": "x", "name": "x", "title": "t", "description": "d", "status": "planning",
		"dev_type": nil, "scope": nil, "package": nil, "priority": "P2",
		"creator": "a", "assignee": "a", "createdAt": "2026-11-09", "completedAt": nil,
		"branch": nil, "base_branch": nil, "worktree_path": nil, "commit": nil,
		"pr_url": nil, "subtasks": []any{}, "children": []any{}, "parent": nil,
		"relatedFiles": []any{}, "notes": nil, "meta": map[string]any{},
	}
	cases := map[string]any{
		"status":   123,
		"subtasks": "not-a-list",
		"meta":     "not-an-object",
		"priority": true,
	}
	for field, bad := range cases {
		m := map[string]any{}
		for k, v := range base {
			m[k] = v
		}
		m[field] = bad
		data := mustJSON(t, m)
		if _, err := ParseRecord(data); err == nil {
			t.Errorf("field %s = %v should error", field, bad)
		}
	}
}

func TestUnknownFieldsPreservedOnWrite(t *testing.T) {
	root := newTestRoot(t)
	dir := filepath.Join(root, DirSunshine, DirTasks, "11-09-test")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	raw := `{
  "id": "test",
  "name": "test",
  "title": "测试",
  "description": "描述",
  "status": "planning",
  "dev_type": null,
  "scope": null,
  "package": null,
  "priority": "P2",
  "creator": "tester",
  "assignee": "tester",
  "createdAt": "2026-11-09",
  "completedAt": null,
  "branch": null,
  "base_branch": null,
  "worktree_path": null,
  "commit": null,
  "pr_url": null,
  "subtasks": [],
  "children": [],
  "parent": null,
  "relatedFiles": [],
  "notes": null,
  "meta": {},
  "custom_field": "keep-me",
  "another": 42
}
`
	if err := os.WriteFile(filepath.Join(dir, FileTaskJSON), []byte(raw), 0o644); err != nil {
		t.Fatal(err)
	}
	rec, err := LoadRecord(dir)
	if err != nil {
		t.Fatal(err)
	}
	if err := WriteRecord(dir, rec); err != nil {
		t.Fatal(err)
	}
	after, err := os.ReadFile(filepath.Join(dir, FileTaskJSON))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(after), `"custom_field": "keep-me"`) {
		t.Errorf("unknown field custom_field lost: %s", after)
	}
	if !strings.Contains(string(after), `"another": 42`) {
		t.Errorf("unknown field another lost: %s", after)
	}
	// 未知字段在 canonical 字段之后。
	if strings.Index(string(after), `"custom_field"`) < strings.Index(string(after), `"meta"`) {
		t.Errorf("unknown fields should come after canonical fields: %s", after)
	}
}

func TestSlugifyChinese(t *testing.T) {
	if got := Slugify("退款原因必填"); got != "退款原因必填" {
		t.Errorf("Slugify Chinese = %q", got)
	}
	if got := Slugify("Hello World!"); got != "hello-world" {
		t.Errorf("Slugify ASCII = %q", got)
	}
	if got := Slugify("给 退款 原因 加 校验"); got != "给-退款-原因-加-校验" {
		t.Errorf("Slugify spaces = %q", got)
	}
}

func TestValidateTaskDirName(t *testing.T) {
	valid := []string{"11-09-退款原因必填", "01-01-hello-world", "00-bootstrap-guidelines"}
	for _, name := range valid {
		if !ValidateTaskDirName(name) {
			t.Errorf("%q should be valid", name)
		}
	}
	invalid := []string{"13-09-x", "11-32-x", "11-09-", "11-09-a/b", "1109-x", "11-09-x y"}
	for _, name := range invalid {
		if ValidateTaskDirName(name) {
			t.Errorf("%q should be invalid", name)
		}
	}
}

func TestInferPhase(t *testing.T) {
	cases := map[string]string{
		"planning": "plan", "in_progress": "implement", "review": "review",
		"completed": "completed", "done": "completed", "other": "unknown",
	}
	for status, want := range cases {
		if got := InferPhase(status); got != want {
			t.Errorf("InferPhase(%q) = %q, want %q", status, got, want)
		}
	}
}

func mustJSON(t *testing.T, v any) []byte {
	t.Helper()
	data, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return data
}
