package task

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// writeLinesFile 在 root 下写一份 n 行的文件（每行可单独断言）。
func writeLinesFile(t *testing.T, root, rel string, n int) {
	t.Helper()
	var b strings.Builder
	for i := 1; i <= n; i++ {
		b.WriteString("第" + strconv.Itoa(i) + "行\n")
	}
	if err := os.MkdirAll(filepath.Dir(filepath.Join(root, rel)), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, rel), []byte(b.String()), 0o644); err != nil {
		t.Fatal(err)
	}
}

// TestAddContext_WithLines：--lines 写入条目，行范围随条目落盘。
func TestAddContext_WithLines(t *testing.T) {
	root := newTestRoot(t)
	r, _, _ := newTestRunner(t, root)
	name := createTask(t, r, "行范围测试")
	writeLinesFile(t, root, ".sunshine/spec/backend.md", 200)
	if code := r.AddContext(name, "implement", ".sunshine/spec/backend.md", "后端规范：接口定义", "120-180"); code != 0 {
		t.Fatalf("AddContext 带 lines 失败，退出码 %d", code)
	}
	data, err := os.ReadFile(filepath.Join(root, DirSunshine, DirTasks, name, "implement.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), `"lines":"120-180"`) {
		t.Errorf("implement.jsonl 应含 lines 字段，得到：%s", string(data))
	}
}

// TestAddContext_LinesInvalid：lines 格式非法拒绝写入。
func TestAddContext_LinesInvalid(t *testing.T) {
	root := newTestRoot(t)
	r, _, _ := newTestRunner(t, root)
	name := createTask(t, r, "行范围测试")
	writeLinesFile(t, root, ".sunshine/spec/backend.md", 200)
	for _, bad := range []string{"abc", "180-120", "0-10", "120"} {
		if code := r.AddContext(name, "implement", ".sunshine/spec/backend.md", "后端规范：接口定义", bad); code == 0 {
			t.Errorf("AddContext lines=%q 应拒绝，实际通过", bad)
		}
	}
}

// TestAddContext_LinesOnDirectory：目录型条目不支持 lines。
func TestAddContext_LinesOnDirectory(t *testing.T) {
	root := newTestRoot(t)
	r, _, _ := newTestRunner(t, root)
	name := createTask(t, r, "行范围测试")
	if err := os.MkdirAll(filepath.Join(root, "doc"), 0o755); err != nil {
		t.Fatal(err)
	}
	if code := r.AddContext(name, "implement", "doc", "文档目录", "1-10"); code == 0 {
		t.Errorf("目录型条目带 lines 应拒绝，实际通过")
	}
}

// TestValidate_Lines：格式非法与起始行越界是硬错误；结束行越界只警告。
func TestValidate_Lines(t *testing.T) {
	root := newTestRoot(t)
	r, out, _ := newTestRunner(t, root)
	name := createTask(t, r, "行范围测试")
	taskDir := filepath.Join(root, DirSunshine, DirTasks, name)
	writeLinesFile(t, root, ".sunshine/spec/backend.md", 10)

	writeJSONL := func(content string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(taskDir, "implement.jsonl"), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(taskDir, "check.jsonl"), []byte("{\"file\": \".sunshine/spec/backend.md\", \"reason\": \"检查\"}\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	// 格式非法 → 错误。
	writeJSONL("{\"file\": \".sunshine/spec/backend.md\", \"reason\": \"申请页\", \"lines\": \"abc\"}\n")
	out.Reset()
	if code := r.Validate(name); code == 0 {
		t.Errorf("lines 格式非法时 Validate 应返回非 0")
	}
	if !strings.Contains(out.String(), "lines 格式非法") {
		t.Errorf("Validate 应报 lines 格式非法，得到：%s", out.String())
	}

	// 起始行越界 → 错误。
	writeJSONL("{\"file\": \".sunshine/spec/backend.md\", \"reason\": \"申请页\", \"lines\": \"20-30\"}\n")
	out.Reset()
	if code := r.Validate(name); code == 0 {
		t.Errorf("lines 起始行越界时 Validate 应返回非 0")
	}
	if !strings.Contains(out.String(), "超出文件总行数") {
		t.Errorf("Validate 应报起始行越界，得到：%s", out.String())
	}

	// 结束行越界 → 只警告，校验通过。
	writeJSONL("{\"file\": \".sunshine/spec/backend.md\", \"reason\": \"申请页\", \"lines\": \"8-99\"}\n")
	out.Reset()
	if code := r.Validate(name); code != 0 {
		t.Errorf("lines 结束行越界只应警告，Validate 返回 %d：%s", code, out.String())
	}
	if !strings.Contains(out.String(), "截到文件末尾") {
		t.Errorf("Validate 应提示结束行截到末尾，得到：%s", out.String())
	}

	// 合法范围 → 通过。
	writeJSONL("{\"file\": \".sunshine/spec/backend.md\", \"reason\": \"申请页\", \"lines\": \"3-5\"}\n")
	out.Reset()
	if code := r.Validate(name); code != 0 {
		t.Errorf("合法 lines 应校验通过，Validate 返回 %d：%s", code, out.String())
	}
}

// TestListContext_ShowsLines：list-context 显示行范围。
func TestListContext_ShowsLines(t *testing.T) {
	root := newTestRoot(t)
	r, out, _ := newTestRunner(t, root)
	name := createTask(t, r, "行范围测试")
	writeLinesFile(t, root, ".sunshine/spec/backend.md", 200)
	if code := r.AddContext(name, "implement", ".sunshine/spec/backend.md", "后端规范：接口定义", "120-180"); code != 0 {
		t.Fatalf("AddContext 失败，退出码 %d", code)
	}
	out.Reset()
	if code := r.ListContext(name); code != 0 {
		t.Fatalf("ListContext 失败，退出码 %d", code)
	}
	if !strings.Contains(out.String(), "lines 120-180") {
		t.Errorf("list-context 应显示 lines 120-180，得到：%s", out.String())
	}
}
