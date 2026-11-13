package hook

import (
	"bytes"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// TestParseLineRange：lines 字段 "120-180"（1-based 闭区间）。
func TestParseLineRange(t *testing.T) {
	cases := []struct {
		in    string
		start int
		end   int
		ok    bool
	}{
		{"120-180", 120, 180, true},
		{"1-1", 1, 1, true},
		{" 120 - 180 ", 120, 180, true},
		{"", 0, 0, false},
		{"180-120", 0, 0, false},
		{"0-10", 0, 0, false},
		{"120", 0, 0, false},
		{"abc", 0, 0, false},
		{"120-180-200", 0, 0, false},
		{"12a-180", 0, 0, false},
	}
	for _, c := range cases {
		start, end, ok := parseLineRange(c.in)
		if ok != c.ok || start != c.start || end != c.end {
			t.Errorf("parseLineRange(%q) = (%d, %d, %v)，想要 (%d, %d, %v)", c.in, start, end, ok, c.start, c.end, c.ok)
		}
	}
}

// TestSliceLines：按行切片（末尾空行不计入总行数）。
func TestSliceLines(t *testing.T) {
	data := []byte("l1\nl2\nl3\nl4\nl5\n")
	content, total, end := sliceLines(data, 2, 4)
	if content != "l2\nl3\nl4" || total != 5 || end != 4 {
		t.Errorf("sliceLines(2-4) = (%q, %d, %d)，想要 (\"l2\\nl3\\nl4\", 5, 4)", content, total, end)
	}
	// end 越界截到文件末尾。
	content, total, end = sliceLines(data, 4, 99)
	if content != "l4\nl5" || total != 5 || end != 5 {
		t.Errorf("sliceLines(4-99) = (%q, %d, %d)，想要 (\"l4\\nl5\", 5, 5)", content, total, end)
	}
	// 单行。
	content, _, end = sliceLines(data, 3, 3)
	if content != "l3" || end != 3 {
		t.Errorf("sliceLines(3-3) = (%q, %d)，想要 (\"l3\", 3)", content, end)
	}
	// start 越界：空内容、末行 0。
	content, total, end = sliceLines(data, 20, 30)
	if content != "" || total != 5 || end != 0 {
		t.Errorf("sliceLines(20-30) = (%q, %d, %d)，想要 (\"\", 5, 0)", content, total, end)
	}
}

// tenLineSpecFile 在规范目录下写一份 10 行的文件，每行可单独断言。
func tenLineSpecFile(t *testing.T, root string) string {
	t.Helper()
	var b strings.Builder
	for i := 1; i <= 10; i++ {
		b.WriteString("第" + strconv.Itoa(i) + "行-规范内容\n")
	}
	rel := ".sunshine/spec/backend.md"
	mustWrite(t, filepath.Join(root, rel), b.String())
	return rel
}

func TestMaterializeFile_Lines(t *testing.T) {
	root, _ := makeProject(t)
	rel := tenLineSpecFile(t, root)
	limits := limitsFromConfig(LoadConfig(root))
	b := &budget{maxTotal: 0}
	block := materializeFile(root, rel, "后端规范：接口定义", "3-5", limits, b)
	for _, want := range []string{"第3行-规范内容", "第4行-规范内容", "第5行-规范内容", "(lines 3-5 of 10)"} {
		if !strings.Contains(block, want) {
			t.Errorf("切片注入缺少 %q\n全文：\n%s", want, block)
		}
	}
	for _, notWant := range []string{"第1行-规范内容", "第2行-规范内容", "第6行-规范内容", "第10行-规范内容"} {
		if strings.Contains(block, notWant) {
			t.Errorf("切片注入不应含 %q\n全文：\n%s", notWant, block)
		}
	}
}

func TestMaterializeFile_LinesEndClamped(t *testing.T) {
	root, _ := makeProject(t)
	rel := tenLineSpecFile(t, root)
	limits := limitsFromConfig(LoadConfig(root))
	b := &budget{maxTotal: 0}
	block := materializeFile(root, rel, "后端规范：接口定义", "8-99", limits, b)
	if !strings.Contains(block, "(lines 8-10 of 10)") {
		t.Errorf("end 越界应截到末尾并注明实际行号，得到：\n%s", block)
	}
	if !strings.Contains(block, "第10行-规范内容") || strings.Contains(block, "第7行-规范内容") {
		t.Errorf("截断后的切片内容不对：\n%s", block)
	}
}

func TestMaterializeFile_LinesStartOutOfRange(t *testing.T) {
	root, _ := makeProject(t)
	rel := tenLineSpecFile(t, root)
	limits := limitsFromConfig(LoadConfig(root))
	b := &budget{maxTotal: 0}
	block := materializeFile(root, rel, "后端规范：接口定义", "20-30", limits, b)
	if !strings.Contains(block, "超出文件总行数") {
		t.Errorf("start 越界应返回越界提示，得到：\n%s", block)
	}
	if strings.Contains(block, "第1行-规范内容") {
		t.Errorf("start 越界不应注入任何行，得到：\n%s", block)
	}
}

func TestMaterializeFile_LinesInvalidFallsBackToWholeFile(t *testing.T) {
	root, _ := makeProject(t)
	rel := tenLineSpecFile(t, root)
	limits := limitsFromConfig(LoadConfig(root))
	b := &budget{maxTotal: 0}
	block := materializeFile(root, rel, "后端规范：接口定义", "abc", limits, b)
	if !strings.Contains(block, "第1行-规范内容") || !strings.Contains(block, "第10行-规范内容") {
		t.Errorf("lines 格式非法应降级为整文件注入，得到：\n%s", block)
	}
	if strings.Contains(block, "(lines ") {
		t.Errorf("降级注入不应带行范围块头，得到：\n%s", block)
	}
}

func TestMaterializeFile_NoLinesInjectsWholeFile(t *testing.T) {
	root, _ := makeProject(t)
	rel := tenLineSpecFile(t, root)
	limits := limitsFromConfig(LoadConfig(root))
	b := &budget{maxTotal: 0}
	block := materializeFile(root, rel, "后端规范：接口定义", "", limits, b)
	if !strings.Contains(block, "第1行-规范内容") || !strings.Contains(block, "第10行-规范内容") {
		t.Errorf("无 lines 应注入整文件，得到：\n%s", block)
	}
	if strings.Contains(block, "(lines ") {
		t.Errorf("无 lines 不应带行范围块头，得到：\n%s", block)
	}
}

// TestReadJSONLEntries_LinesInvalidWarns：注入解析时 lines 格式非法，
// 记 stderr 警告并降级为空（hook 绝不阻断会话）。
func TestReadJSONLEntries_LinesInvalidWarns(t *testing.T) {
	root, _ := makeProject(t)
	tenLineSpecFile(t, root)
	taskDir := ".sunshine/tasks/11-09-demo"
	mustWrite(t, filepath.Join(root, taskDir, "implement.jsonl"),
		"{\"file\": \".sunshine/spec/backend.md\", \"reason\": \"后端规范：接口定义\", \"lines\": \"abc\"}\n")
	var errBuf bytes.Buffer
	entries := readJSONLEntries(root, taskDir+"/implement.jsonl", &errBuf)
	if len(entries) != 1 || entries[0].Lines != "" {
		t.Errorf("非法 lines 应被清空，得到：%v", entries)
	}
	if !strings.Contains(errBuf.String(), "格式非法") {
		t.Errorf("非法 lines 应写 stderr 警告，得到：%q", errBuf.String())
	}
}

// TestSubagentContext_ImplementLines：端到端——implement 子代理的注入
// 提示词里只含原型切片行，不含范围外的行。
func TestSubagentContext_ImplementLines(t *testing.T) {
	root, _ := makeProject(t)
	tenLineSpecFile(t, root)
	taskDir := ".sunshine/tasks/11-09-demo"
	mustWrite(t, filepath.Join(root, taskDir, "implement.jsonl"),
		"{\"file\": \".sunshine/spec/backend.md\", \"reason\": \"后端规范：接口定义\", \"lines\": \"3-5\"}\n")
	out, _ := runWithInput(t, func(in *strings.Reader, o Options) error { return RunSubagentContext(in, o) },
		`{"session_id":"sess-1","cwd":`+strconv_quot(root)+`,"tool_name":"Task","tool_input":{"subagent_type":"sunshine-implement","prompt":"实现演示功能"}}`)
	data := mustJSON(t, out)
	hs := data["hookSpecificOutput"].(map[string]any)
	updated := hs["updatedInput"].(map[string]any)
	prompt, _ := updated["prompt"].(string)
	for _, want := range []string{"第3行-规范内容", "第5行-规范内容", "(lines 3-5 of 10)"} {
		if !strings.Contains(prompt, want) {
			t.Errorf("implement 提示词缺少 %q\n全文：\n%s", want, prompt)
		}
	}
	for _, notWant := range []string{"第1行-规范内容", "第2行-规范内容", "第6行-规范内容"} {
		if strings.Contains(prompt, notWant) {
			t.Errorf("implement 提示词不应含 %q\n全文：\n%s", notWant, prompt)
		}
	}
}
