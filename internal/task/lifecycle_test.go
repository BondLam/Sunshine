package task

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func createTask(t *testing.T, r *Runner, title string) string {
	t.Helper()
	code := r.Create(CreateOptions{Title: title, Description: "测试任务"})
	if code != 0 {
		t.Fatalf("create %q failed with code %d", title, code)
	}
	return "11-09-" + Slugify(title)
}

func TestLifecycleFullFlow(t *testing.T) {
	root := newTestRoot(t)
	r, out, _ := newTestRunner(t, root)

	name := createTask(t, r, "退款原因必填")
	taskDir := filepath.Join(root, DirSunshine, DirTasks, name)
	data, err := os.ReadFile(filepath.Join(taskDir, FileTaskJSON))
	if err != nil {
		t.Fatal(err)
	}
	var rec map[string]any
	if err := json.Unmarshal(data, &rec); err != nil {
		t.Fatal(err)
	}
	if rec["status"] != "planning" || rec["priority"] != "P2" || rec["id"] != "退款原因必填" {
		t.Errorf("created record: %v", rec)
	}
	if _, err := os.Stat(filepath.Join(taskDir, "prd.md")); err != nil {
		t.Errorf("prd.md missing: %v", err)
	}
	if !strings.Contains(out.String(), ".sunshine/tasks/"+name) {
		t.Errorf("create should print relative path, got: %s", out.String())
	}

	// list
	out.Reset()
	if code := r.List(false, "", false); code != 0 {
		t.Fatalf("list failed: %d", code)
	}
	if !strings.Contains(out.String(), "退款原因必填") {
		t.Errorf("list should contain task, got: %s", out.String())
	}

	// start → current → finish
	out.Reset()
	if code := r.Start(name, false); code != 0 {
		t.Fatalf("start failed: %d", code)
	}
	rec2, err := LoadRecord(taskDir)
	if err != nil {
		t.Fatal(err)
	}
	if rec2.Status != "in_progress" {
		t.Errorf("after start status = %q, want in_progress", rec2.Status)
	}
	currentFile, err := os.ReadFile(filepath.Join(root, DirSunshine, ".current-task"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(currentFile), name) {
		t.Errorf(".current-task = %q, want it to contain %q", currentFile, name)
	}

	out.Reset()
	if code := r.Current(false, false); code != 0 {
		t.Fatalf("current failed: %d", code)
	}
	if !strings.Contains(out.String(), "退款原因必填") {
		t.Errorf("current should print task, got: %s", out.String())
	}

	out.Reset()
	if code := r.Current(false, true); code != 0 {
		t.Fatalf("current --json failed: %d", code)
	}
	var cur map[string]any
	if err := json.Unmarshal(out.Bytes(), &cur); err != nil {
		t.Fatalf("current --json not JSON: %v\n%s", err, out.String())
	}
	if _, ok := cur["current_task"]; !ok {
		t.Errorf("current --json missing current_task: %v", cur)
	}

	if code := r.Finish(); code != 0 {
		t.Fatalf("finish failed: %d", code)
	}
	if data, err := os.ReadFile(filepath.Join(root, DirSunshine, ".current-task")); err == nil && strings.TrimSpace(string(data)) != "" {
		t.Errorf(".current-task should be empty after finish, got %q", data)
	}

	// set-meta / set-branch / set-scope
	if code := r.SetMeta(name, "reviewer", "zhangsan"); code != 0 {
		t.Fatalf("set-meta failed: %d", code)
	}
	rec3, _ := LoadRecord(taskDir)
	meta := map[string]any{}
	_ = json.Unmarshal(rec3.Meta, &meta)
	if meta["reviewer"] != "zhangsan" {
		t.Errorf("meta.reviewer = %v", meta["reviewer"])
	}
	if code := r.SetBranch(name, "feature/refund"); code != 0 {
		t.Fatalf("set-branch failed: %d", code)
	}
	if code := r.SetScope(name, "只改 Refund.vue"); code != 0 {
		t.Fatalf("set-scope failed: %d", code)
	}

	// add-subtask + rename + archive
	child := createTask(t, r, "前端校验")
	if code := r.AddSubtask(name, child); code != 0 {
		t.Fatalf("add-subtask failed: %d", code)
	}
	parentRec, _ := LoadRecord(taskDir)
	if len(parentRec.Children) != 1 || parentRec.Children[0] != child {
		t.Errorf("parent children = %v", parentRec.Children)
	}
	childRec, _ := LoadRecord(filepath.Join(root, DirSunshine, DirTasks, child))
	if childRec.Parent == nil || *childRec.Parent != name {
		t.Errorf("child parent = %v, want %q", childRec.Parent, name)
	}

	newName := "11-09-退款必填"
	if code := r.Rename(name, newName, false); code != 0 {
		t.Fatalf("rename failed: %d", code)
	}
	if _, err := os.Stat(filepath.Join(root, DirSunshine, DirTasks, newName)); err != nil {
		t.Errorf("renamed dir missing: %v", err)
	}
	childRec2, _ := LoadRecord(filepath.Join(root, DirSunshine, DirTasks, child))
	if childRec2.Parent == nil || *childRec2.Parent != newName {
		t.Errorf("child parent after rename = %v, want %q", childRec2.Parent, newName)
	}

	if code := r.Archive(newName, true, true); code != 0 {
		t.Fatalf("archive failed: %d", code)
	}
	archived := filepath.Join(root, DirSunshine, DirTasks, DirArchive, "2026-11", newName)
	if _, err := os.Stat(filepath.Join(archived, FileTaskJSON)); err != nil {
		t.Errorf("archived task.json missing: %v", err)
	}
	archRec, _ := LoadRecord(archived)
	if archRec.Status != "completed" || archRec.CompletedAt == nil || *archRec.CompletedAt != "2026-11-09" {
		t.Errorf("archived record status=%q completedAt=%v", archRec.Status, archRec.CompletedAt)
	}
	// 归档后子女的 parent 被清除。
	childRec3, _ := LoadRecord(filepath.Join(root, DirSunshine, DirTasks, child))
	if childRec3.Parent != nil {
		t.Errorf("child parent after archive = %v, want nil", childRec3.Parent)
	}

	// list-archive：不带月份时也应列出任务名，不只是月标题。
	out.Reset()
	if code := r.ListArchive(""); code != 0 {
		t.Fatalf("list-archive failed: %d", code)
	}
	if !strings.Contains(out.String(), "[2026-11] - 1 个任务") || !strings.Contains(out.String(), newName+"/") {
		t.Errorf("list-archive should list archived task, got: %s", out.String())
	}
	out.Reset()
	if code := r.ListArchive("2026-11"); code != 0 {
		t.Fatalf("list-archive 2026-11 failed: %d", code)
	}
	if !strings.Contains(out.String(), newName+"/") {
		t.Errorf("list-archive 2026-11 should list archived task, got: %s", out.String())
	}
}

func TestCreateValidation(t *testing.T) {
	root := newTestRoot(t)
	r, _, errBuf := newTestRunner(t, root)

	if code := r.Create(CreateOptions{Title: "", Description: "d"}); code == 0 {
		t.Errorf("empty title should fail")
	}
	if code := r.Create(CreateOptions{Title: "t", Description: ""}); code == 0 {
		t.Errorf("empty description should fail")
	}
	if code := r.Create(CreateOptions{Title: "t", Description: "d", Meta: []string{"no-equals"}}); code == 0 {
		t.Errorf("bad --meta should fail")
	}
	if code := r.Create(CreateOptions{Title: "t", Description: "d", Slug: "a/b"}); code == 0 {
		t.Errorf("slug with slash should fail")
	}
	_ = errBuf
}

func TestCreateDuplicateRejected(t *testing.T) {
	root := newTestRoot(t)
	r, _, _ := newTestRunner(t, root)
	createTask(t, r, "重复任务")
	if code := r.Create(CreateOptions{Title: "重复任务", Description: "d"}); code == 0 {
		t.Errorf("duplicate create should fail without --force")
	}
	if code := r.Create(CreateOptions{Title: "重复任务", Description: "d", Force: true}); code != 0 {
		t.Errorf("duplicate create with --force should succeed, got %d", code)
	}
}

func TestRenameDryRun(t *testing.T) {
	root := newTestRoot(t)
	r, out, _ := newTestRunner(t, root)
	name := createTask(t, r, "旧名字")
	out.Reset()
	if code := r.Rename(name, "11-09-新名字", true); code != 0 {
		t.Fatalf("rename --dry-run failed: %d", code)
	}
	if _, err := os.Stat(filepath.Join(root, DirSunshine, DirTasks, name)); err != nil {
		t.Errorf("dry-run should not move the dir: %v", err)
	}
	if !strings.Contains(out.String(), "新名字") {
		t.Errorf("dry-run should print plan, got: %s", out.String())
	}
}

func TestStartEmptyContextGate(t *testing.T) {
	root := newTestRoot(t)
	r, _, _ := newTestRunner(t, root)
	name := createTask(t, r, "有空清单")
	// 子代理平台下才建空 jsonl；这里手动建一个空 implement.jsonl 触发门禁。
	taskDir := filepath.Join(root, DirSunshine, DirTasks, name)
	if err := os.WriteFile(filepath.Join(taskDir, "implement.jsonl"), []byte{}, 0o644); err != nil {
		t.Fatal(err)
	}
	if code := r.Start(name, false); code == 0 {
		t.Errorf("start with empty context should be rejected")
	}
	if code := r.Start(name, true); code != 0 {
		t.Errorf("start with --allow-empty-context should succeed, got %d", code)
	}
}

func TestAddRemoveSubtask(t *testing.T) {
	root := newTestRoot(t)
	r, _, _ := newTestRunner(t, root)
	parent := createTask(t, r, "父任务")
	child := createTask(t, r, "子任务")
	if code := r.AddSubtask(parent, child); code != 0 {
		t.Fatalf("add-subtask failed: %d", code)
	}
	// 子任务已有父任务时再加到另一个父任务应拒绝。
	other := createTask(t, r, "另一父任务")
	if code := r.AddSubtask(other, child); code == 0 {
		t.Errorf("add-subtask to a child that already has a parent should fail")
	}
	if code := r.RemoveSubtask(parent, child); code != 0 {
		t.Fatalf("remove-subtask failed: %d", code)
	}
	childRec, _ := LoadRecord(filepath.Join(root, DirSunshine, DirTasks, child))
	if childRec.Parent != nil {
		t.Errorf("child parent after remove = %v", childRec.Parent)
	}
}

func TestListMine(t *testing.T) {
	root := newTestRoot(t)
	r, out, _ := newTestRunner(t, root)
	createTask(t, r, "我的任务")
	r2, _, _ := newTestRunner(t, root)
	if code := r2.Create(CreateOptions{Title: "别人的任务", Description: "d", Assignee: "other"}); code != 0 {
		t.Fatalf("create for other failed: %d", code)
	}
	out.Reset()
	if code := r.List(true, "", false); code != 0 {
		t.Fatalf("list --mine failed: %d", code)
	}
	if !strings.Contains(out.String(), "我的任务") {
		t.Errorf("list --mine should contain own task, got: %s", out.String())
	}
	if strings.Contains(out.String(), "别人的任务") {
		t.Errorf("list --mine should not contain other's task, got: %s", out.String())
	}
}

func TestContextModes(t *testing.T) {
	root := newTestRoot(t)
	r, out, _ := newTestRunner(t, root)
	createTask(t, r, "上下文任务")
	out.Reset()
	if code := r.Context("default", "", "", false); code != 0 {
		t.Fatalf("context default failed: %d", code)
	}
	if !strings.Contains(out.String(), "tester") || !strings.Contains(out.String(), "上下文任务") {
		t.Errorf("context default should mention developer and task, got: %s", out.String())
	}
	out.Reset()
	if code := r.Context("default", "", "", true); code != 0 {
		t.Fatalf("context --json failed: %d", code)
	}
	var doc map[string]any
	if err := json.Unmarshal(out.Bytes(), &doc); err != nil {
		t.Fatalf("context --json not JSON: %v\n%s", err, out.String())
	}
	if _, ok := doc["developer"]; !ok {
		t.Errorf("context JSON missing developer: %v", doc)
	}
	// phase 模式：步骤不存在时退出 2。
	if code := r.Context("phase", "9.9", "", false); code != 2 {
		t.Errorf("context phase with missing step = %d, want 2", code)
	}
}
