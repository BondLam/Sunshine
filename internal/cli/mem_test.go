package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/spf13/cobra"
)

// testMemRoot 建一个只含 mem 命令的测试根命令。
func testMemRoot() *cobra.Command {
	root := &cobra.Command{
		Use:           "sunshine",
		SilenceErrors: true,
		SilenceUsage:  true,
	}
	RegisterMemCommand(root)
	return root
}

// memFixture 是 mem CLI 测试用的 Claude 会话夹具。
type memFixture struct {
	home    string
	proj    string
	session string
}

// setupMemFixture 在临时 HOME 里造一份 Claude 会话存储：
// 一个会话、两组用户/助手轮次（含“记忆检索”关键词）、一行坏行、
// 一条未知事件类型、一轮压缩摘要；再把工作目录切到该项目。
func setupMemFixture(t *testing.T) memFixture {
	t.Helper()
	home := t.TempDir()
	proj := t.TempDir()
	t.Setenv("HOME", home)
	t.Chdir(proj)

	sessionID := "aaaaaaaaaaaa"
	dir := filepath.Join(home, ".claude", "projects", "myproj")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	lines := []string{
		`{"type":"user","message":{"role":"user","content":"请帮我实现记忆检索功能"},"cwd":"` + proj + `","timestamp":"2026-11-09T10:00:00Z"}`,
		`{"type":"assistant","message":{"role":"assistant","content":[{"type":"text","text":"好的，我来帮你实现记忆检索。"}]},"timestamp":"2026-11-09T10:01:00Z"}`,
		`{"type":"user","message":{"role":"user","content":"再加一个会话列表"},"timestamp":"2026-11-09T10:02:00Z"}`,
		`{"type":"assistant","message":{"role":"assistant","content":[{"type":"text","text":"会话列表已加上。"}]},"timestamp":"2026-11-09T10:03:00Z"}`,
		`{"type":"user","isCompactSummary":true,"message":{"content":"此前讨论了部署方案。"},"timestamp":"2026-11-09T10:04:00Z"}`,
		`{"type":"telemetry","foo":1}`,
		`{"type":"user","broken`,
	}
	path := filepath.Join(dir, sessionID+".jsonl")
	if err := os.WriteFile(path, []byte(strings.Join(lines, "\n")+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	// 固定文件改动时间，让 updated 稳定落在 2026-11-09。
	mtime := time.Date(2026, 11, 9, 12, 0, 0, 0, time.UTC)
	if err := os.Chtimes(path, mtime, mtime); err != nil {
		t.Fatal(err)
	}
	index := `{"entries":[{"id":"` + sessionID + `","cwd":"` + proj + `","created":"2026-11-09T10:00:00Z","title":"记忆检索会话"}]}`
	if err := os.WriteFile(filepath.Join(dir, "sessions-index.json"), []byte(index), 0o644); err != nil {
		t.Fatal(err)
	}
	return memFixture{home: home, proj: proj, session: sessionID}
}

// runMemArgs 执行一次 mem 命令并返回标准输出；出错时返回错误。
func runMemArgs(t *testing.T, args ...string) (string, error) {
	t.Helper()
	root := testMemRoot()
	root.SetArgs(args)
	return captureStdout(t, func() error {
		return root.Execute()
	})
}

func TestMemListDefaultScope(t *testing.T) {
	fx := setupMemFixture(t)
	out, err := runMemArgs(t, "mem", "list")
	if err != nil {
		t.Fatalf("mem list 不应报错：%v", err)
	}
	if !strings.Contains(out, "范围：项目=") {
		t.Errorf("默认作用域应显示当前项目，实际输出：\n%s", out)
	}
	if !strings.Contains(out, "aaaaaaaaaaaa") || !strings.Contains(out, "记忆检索会话") {
		t.Errorf("列表应含会话 id 与标题，实际输出：\n%s", out)
	}
	if !strings.Contains(out, "[claude") || !strings.Contains(out, "共 1 个会话") {
		t.Errorf("列表应含平台与计数，实际输出：\n%s", out)
	}
	_ = fx
}

func TestMemListGlobalAndCwdScope(t *testing.T) {
	fx := setupMemFixture(t)
	out, err := runMemArgs(t, "mem", "list", "--global")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "范围：全部项目") {
		t.Errorf("--global 应显示全部项目，实际输出：\n%s", out)
	}
	// 切到别的目录：默认作用域下应为空；显式 --cwd 能找回。
	other := t.TempDir()
	t.Chdir(other)
	out, err = runMemArgs(t, "mem", "list")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "（暂无会话）") {
		t.Errorf("离开项目后默认作用域应为空，实际输出：\n%s", out)
	}
	out, err = runMemArgs(t, "mem", "list", "--cwd", fx.proj)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "aaaaaaaaaaaa") {
		t.Errorf("--cwd 应找回项目会话，实际输出：\n%s", out)
	}
}

func TestMemListJSONFieldNames(t *testing.T) {
	setupMemFixture(t)
	out, err := runMemArgs(t, "mem", "list", "--json")
	if err != nil {
		t.Fatal(err)
	}
	var rows []map[string]any
	if err := json.Unmarshal([]byte(out), &rows); err != nil {
		t.Fatalf("--json 应输出合法 JSON 数组，实际输出：\n%s\n错误：%v", out, err)
	}
	if len(rows) != 1 {
		t.Fatalf("期望 1 条会话，实际 %d 条：\n%s", len(rows), out)
	}
	row := rows[0]
	for _, key := range []string{"platform", "id", "title", "cwd", "created", "updated", "filePath"} {
		if _, ok := row[key]; !ok {
			t.Errorf("JSON 缺少字段 %q：\n%s", key, out)
		}
	}
	if row["platform"] != "claude" {
		t.Errorf("platform 应为 claude，实际 %v", row["platform"])
	}
	if _, ok := row["parent_id"]; ok {
		t.Errorf("无父会话时 parent_id 应省略：\n%s", out)
	}
	if !strings.Contains(row["filePath"].(string), "aaaaaaaaaaaa.jsonl") {
		t.Errorf("filePath 应指向会话文件，实际 %v", row["filePath"])
	}
}

func TestMemListSinceUntil(t *testing.T) {
	setupMemFixture(t)
	// 会话在 2026-11-09；窗口取不到它时应为空。
	out, err := runMemArgs(t, "mem", "list", "--since", "2026-11-10")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "（暂无会话）") {
		t.Errorf("--since 在会话之后应为空，实际输出：\n%s", out)
	}
	out, err = runMemArgs(t, "mem", "list", "--until", "2026-11-08")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "（暂无会话）") {
		t.Errorf("--until 在会话之前应为空，实际输出：\n%s", out)
	}
	// 当天窗口应命中（until 含当天全天）。
	out, err = runMemArgs(t, "mem", "list", "--since", "2026-11-09", "--until", "2026-11-09")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "aaaaaaaaaaaa") {
		t.Errorf("当天窗口应命中会话，实际输出：\n%s", out)
	}
}

func TestMemSearch(t *testing.T) {
	fx := setupMemFixture(t)
	out, err := runMemArgs(t, "mem", "search", "记忆检索")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "aaaaaaaaaaaa") || !strings.Contains(out, "命中=") {
		t.Errorf("search 应命中会话并显示命中数，实际输出：\n%s", out)
	}
	if !strings.Contains(out, "记忆检索") {
		t.Errorf("search 应输出命中摘录，实际输出：\n%s", out)
	}
	// 多 token AND：两个词都在同一轮次里才命中。
	out, err = runMemArgs(t, "mem", "search", "记忆 检索")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "aaaaaaaaaaaa") {
		t.Errorf("多 token AND 应命中，实际输出：\n%s", out)
	}
	// 两个词不在同一轮次里 → 无命中。
	out, err = runMemArgs(t, "mem", "search", "记忆 列表")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "（无匹配结果）") {
		t.Errorf("不在同一轮次的两个词应无命中，实际输出：\n%s", out)
	}
	// 无关键词报错。
	if _, err := runMemArgs(t, "mem", "search"); err == nil {
		t.Error("search 不带关键词应报错")
	}
	_ = fx
}

func TestMemSearchJSON(t *testing.T) {
	setupMemFixture(t)
	out, err := runMemArgs(t, "mem", "search", "记忆检索", "--json")
	if err != nil {
		t.Fatal(err)
	}
	var rows []map[string]any
	if err := json.Unmarshal([]byte(out), &rows); err != nil {
		t.Fatalf("search --json 应输出合法 JSON 数组，实际输出：\n%s\n错误：%v", out, err)
	}
	if len(rows) != 1 {
		t.Fatalf("期望 1 条命中，实际 %d 条：\n%s", len(rows), out)
	}
	row := rows[0]
	for _, key := range []string{"session", "score", "hit_count", "user_count", "asst_count", "total_turns", "descendants_merged", "excerpts"} {
		if _, ok := row[key]; !ok {
			t.Errorf("search JSON 缺少字段 %q：\n%s", key, out)
		}
	}
	if row["hit_count"].(float64) <= 0 {
		t.Errorf("hit_count 应大于 0，实际 %v", row["hit_count"])
	}
}

func TestMemContext(t *testing.T) {
	fx := setupMemFixture(t)
	out, err := runMemArgs(t, "mem", "context", fx.session[:8])
	if err != nil {
		t.Fatalf("context 按 id 前缀查找不应报错：%v", err)
	}
	if !strings.Contains(out, "# 上下文：[claude]") || !strings.Contains(out, "记忆检索功能") {
		t.Errorf("context 应显示会话头与轮次文本，实际输出：\n%s", out)
	}
	out, err = runMemArgs(t, "mem", "context", fx.session, "--grep", "记忆")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "命中轮次=2") {
		t.Errorf("grep 应命中 2 轮，实际输出：\n%s", out)
	}
	if !strings.Contains(out, "← 命中") {
		t.Errorf("命中轮次应打标记，实际输出：\n%s", out)
	}
	// 不存在的会话。
	if _, err := runMemArgs(t, "mem", "context", "zzzzzzzz"); err == nil {
		t.Error("context 不存在的会话应报错")
	} else if !strings.Contains(err.Error(), "未找到会话") {
		t.Errorf("报错应说明未找到会话，实际：%v", err)
	}
	// 不带 id 报错。
	if _, err := runMemArgs(t, "mem", "context"); err == nil {
		t.Error("context 不带会话编号应报错")
	}
}

func TestMemExtract(t *testing.T) {
	fx := setupMemFixture(t)
	out, err := runMemArgs(t, "mem", "extract", fx.session)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "## 用户") || !strings.Contains(out, "## 助手") {
		t.Errorf("extract 应按角色分节输出，实际输出：\n%s", out)
	}
	if !strings.Contains(out, "## 压缩边界") {
		t.Errorf("extract 应展示压缩边界 marker，实际输出：\n%s", out)
	}
	// grep 过滤。
	out, err = runMemArgs(t, "mem", "extract", fx.session, "--grep", "会话列表")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "会话列表") || strings.Contains(out, "记忆检索功能") {
		t.Errorf("extract --grep 应只留命中轮次，实际输出：\n%s", out)
	}
	// 无任务边界时 brainstorm 返回全文、implement 返回空。
	out, err = runMemArgs(t, "mem", "extract", fx.session, "--phase", "brainstorm")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "记忆检索功能") {
		t.Errorf("无任务边界时 brainstorm 应返回全文，实际输出：\n%s", out)
	}
	out, err = runMemArgs(t, "mem", "extract", fx.session, "--phase", "implement")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "轮次：0/") {
		t.Errorf("无任务边界时 implement 应为空，实际输出：\n%s", out)
	}
	// 非法 phase 报错。
	if _, err := runMemArgs(t, "mem", "extract", fx.session, "--phase", "foo"); err == nil {
		t.Error("非法 --phase 应报错")
	}
}

func TestMemExtractJSON(t *testing.T) {
	fx := setupMemFixture(t)
	out, err := runMemArgs(t, "mem", "extract", fx.session, "--json")
	if err != nil {
		t.Fatal(err)
	}
	var doc map[string]any
	if err := json.Unmarshal([]byte(out), &doc); err != nil {
		t.Fatalf("extract --json 应输出合法 JSON，实际输出：\n%s\n错误：%v", out, err)
	}
	for _, key := range []string{"session", "phase", "windows", "total_turns", "groups", "turns"} {
		if _, ok := doc[key]; !ok {
			t.Errorf("extract JSON 缺少字段 %q：\n%s", key, out)
		}
	}
	if doc["phase"] != "all" {
		t.Errorf("默认 phase 应为 all，实际 %v", doc["phase"])
	}
}

func TestMemProjects(t *testing.T) {
	fx := setupMemFixture(t)
	out, err := runMemArgs(t, "mem", "projects")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, fx.proj) || !strings.Contains(out, "会话数=") || !strings.Contains(out, "claude:1") {
		t.Errorf("projects 应列出项目与平台计数，实际输出：\n%s", out)
	}
	out, err = runMemArgs(t, "mem", "projects", "--json")
	if err != nil {
		t.Fatal(err)
	}
	var rows []map[string]any
	if err := json.Unmarshal([]byte(out), &rows); err != nil {
		t.Fatalf("projects --json 应输出合法 JSON 数组，实际输出：\n%s\n错误：%v", out, err)
	}
	if len(rows) != 1 {
		t.Fatalf("期望 1 个项目，实际 %d 个：\n%s", len(rows), out)
	}
	for _, key := range []string{"cwd", "last_active", "sessions", "by_platform"} {
		if _, ok := rows[0][key]; !ok {
			t.Errorf("projects JSON 缺少字段 %q：\n%s", key, out)
		}
	}
}

func TestMemInvalidFlags(t *testing.T) {
	setupMemFixture(t)
	cases := [][]string{
		{"mem", "list", "--platform", "foo"},
		{"mem", "list", "--since", "2026-13-01"},
		{"mem", "list", "--until", "不是日期"},
		{"mem", "nosuchcmd"},
		{"mem", "search"},
		{"mem", "context"},
		{"mem", "extract"},
	}
	for _, args := range cases {
		if _, err := runMemArgs(t, args...); err == nil {
			t.Errorf("%v 应报错", args)
		}
	}
	if _, err := runMemArgs(t, "mem", "list", "--platform", "foo"); !strings.Contains(err.Error(), "未知平台") {
		t.Errorf("非法平台报错应说明未知平台，实际：%v", err)
	}
}

func TestMemHelp(t *testing.T) {
	setupMemFixture(t)
	out, err := runMemArgs(t, "mem", "help")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"sunshine mem", "list", "search <关键词>", "context <会话编号>",
		"extract <会话编号>", "projects", "--platform", "--since", "--until",
		"--global", "--cwd", "--limit", "--grep", "--phase", "--turns",
		"--around", "--max-chars", "--include-children", "--json",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("帮助文本应包含 %q，实际输出：\n%s", want, out)
		}
	}
	if strings.Contains(strings.ToLower(out), "trellis") {
		t.Errorf("帮助文本不应残留 trellis 字样，实际输出：\n%s", out)
	}
}

func TestMemDefaultCommandIsList(t *testing.T) {
	setupMemFixture(t)
	out, err := runMemArgs(t, "mem")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "aaaaaaaaaaaa") {
		t.Errorf("不带子命令时 mem 应默认执行 list，实际输出：\n%s", out)
	}
}

// setupMemPhaseFixture 在临时 HOME 里造一份 Claude 会话：
// 任务命令只藏在 Bash tool_use 块里，轮次文本里没有；
// 窗口为 [create 所在轮次 1, start 所在轮次 3)。
func setupMemPhaseFixture(t *testing.T) memFixture {
	t.Helper()
	home := t.TempDir()
	proj := t.TempDir()
	t.Setenv("HOME", home)
	t.Chdir(proj)

	sessionID := "bbbbbbbbbbbb"
	dir := filepath.Join(home, ".claude", "projects", "myproj")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	lines := []string{
		`{"type":"user","message":{"role":"user","content":"先想想做什么"},"cwd":"` + proj + `","timestamp":"2026-11-09T10:00:00Z"}`,
		`{"type":"assistant","message":{"role":"assistant","content":[{"type":"text","text":"方案草稿"},{"type":"tool_use","name":"Bash","input":{"command":"sunshine task create --slug demo"}}]},"timestamp":"2026-11-09T10:01:00Z"}`,
		`{"type":"user","message":{"role":"user","content":"方案定了"},"timestamp":"2026-11-09T10:02:00Z"}`,
		`{"type":"assistant","message":{"role":"assistant","content":[{"type":"text","text":"写代码中"},{"type":"tool_use","name":"Bash","input":{"command":"sunshine task start .sunshine/tasks/11-09-demo/"}}]},"timestamp":"2026-11-09T10:03:00Z"}`,
		`{"type":"user","message":{"role":"user","content":"收尾"},"timestamp":"2026-11-09T10:04:00Z"}`,
	}
	path := filepath.Join(dir, sessionID+".jsonl")
	if err := os.WriteFile(path, []byte(strings.Join(lines, "\n")+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	index := `{"entries":[{"id":"` + sessionID + `","cwd":"` + proj + `","created":"2026-11-09T10:00:00Z","title":"阶段切片会话"}]}`
	if err := os.WriteFile(filepath.Join(dir, "sessions-index.json"), []byte(index), 0o644); err != nil {
		t.Fatal(err)
	}
	return memFixture{home: home, proj: proj, session: sessionID}
}

func TestMemExtractPhase(t *testing.T) {
	fx := setupMemPhaseFixture(t)

	// brainstorm 只出窗口内轮次（第 2、3 轮）。
	out, err := runMemArgs(t, "mem", "extract", fx.session, "--phase", "brainstorm")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"方案草稿", "方案定了", "轮次：2/5", "任务：demo"} {
		if !strings.Contains(out, want) {
			t.Errorf("brainstorm 输出应含 %q，实际输出：\n%s", want, out)
		}
	}
	for _, notWant := range []string{"先想想做什么", "写代码中", "收尾"} {
		if strings.Contains(out, notWant) {
			t.Errorf("brainstorm 输出不应含 %q，实际输出：\n%s", notWant, out)
		}
	}

	// implement 只出窗口外轮次。
	out, err = runMemArgs(t, "mem", "extract", fx.session, "--phase", "implement")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"先想想做什么", "写代码中", "收尾", "轮次：3/5"} {
		if !strings.Contains(out, want) {
			t.Errorf("implement 输出应含 %q，实际输出：\n%s", want, out)
		}
	}
	for _, notWant := range []string{"方案草稿", "方案定了"} {
		if strings.Contains(out, notWant) {
			t.Errorf("implement 输出不应含 %q，实际输出：\n%s", notWant, out)
		}
	}

	// all 返回全文。
	out, err = runMemArgs(t, "mem", "extract", fx.session, "--phase", "all")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "轮次：5/5") {
		t.Errorf("all 应返回全文 5 轮，实际输出：\n%s", out)
	}

	// --json：窗口与分组应与上游一致。
	out, err = runMemArgs(t, "mem", "extract", fx.session, "--phase", "brainstorm", "--json")
	if err != nil {
		t.Fatal(err)
	}
	var doc map[string]any
	if err := json.Unmarshal([]byte(out), &doc); err != nil {
		t.Fatalf("extract --json 应输出合法 JSON，实际输出：\n%s\n错误：%v", out, err)
	}
	if _, ok := doc["detected_phase"]; ok {
		t.Errorf("extract JSON 不应再有 detected_phase 字段（单值阶段判定已删除）：\n%s", out)
	}
	windows, _ := doc["windows"].([]any)
	if len(windows) != 1 {
		t.Fatalf("brainstorm JSON 窗口应为 1 个，实际：%v", doc["windows"])
	}
	w0, _ := windows[0].(map[string]any)
	if w0["label"] != "demo" || w0["startTurn"] != float64(1) || w0["endTurn"] != float64(3) {
		t.Errorf("窗口应为 [1,3)/demo，实际：%v", w0)
	}
}

// list / search 忽略 --phase（只 extract 用它）：
// 带 --phase 也应正常列出、正常检索，不报错、不过滤。
func TestMemListSearchIgnorePhase(t *testing.T) {
	fx := setupMemPhaseFixture(t)
	_ = fx
	out, err := runMemArgs(t, "mem", "list", "--phase", "brainstorm")
	if err != nil {
		t.Fatalf("mem list --phase 不应报错：%v", err)
	}
	if !strings.Contains(out, "bbbbbbbbbbbb") || !strings.Contains(out, "共 1 个会话") {
		t.Errorf("list 应忽略 --phase、照常列出会话，实际输出：\n%s", out)
	}
	out, err = runMemArgs(t, "mem", "search", "方案草稿", "--phase", "implement")
	if err != nil {
		t.Fatalf("mem search --phase 不应报错：%v", err)
	}
	if !strings.Contains(out, "bbbbbbbbbbbb") {
		t.Errorf("search 应忽略 --phase、照常命中会话，实际输出：\n%s", out)
	}
}
