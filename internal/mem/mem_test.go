package mem

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// fakeAdapter 是测试用的内存适配器。
type fakeAdapter struct {
	kind     MemSourceKind
	sessions []MemSessionInfo
	dialogue map[string][]DialogueTurn
	err      error
}

func (f *fakeAdapter) Kind() MemSourceKind { return f.kind }

func (f *fakeAdapter) Sessions(_ context.Context, _ string) ([]MemSessionInfo, []string, error) {
	if f.err != nil {
		return nil, nil, f.err
	}
	return f.sessions, nil, nil
}

func (f *fakeAdapter) Dialogue(_ context.Context, s MemSessionInfo) ([]DialogueTurn, []string, error) {
	if f.err != nil {
		return nil, nil, f.err
	}
	return f.dialogue[s.ID], nil, nil
}

func (f *fakeAdapter) Collect(_ context.Context, s MemSessionInfo) ([]DialogueTurn, []MemEvent, []string, error) {
	if f.err != nil {
		return nil, nil, nil, f.err
	}
	return f.dialogue[s.ID], nil, nil, nil
}

func testSession(platform MemSourceKind, id, cwd, created, updated string) MemSessionInfo {
	return MemSessionInfo{
		Platform: platform,
		ID:       id,
		Title:    "标题-" + id,
		CWD:      cwd,
		Created:  created,
		Updated:  updated,
		FilePath: "/fake/" + id + ".jsonl",
	}
}

func TestBuildFilterDefaults(t *testing.T) {
	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	f, err := BuildFilter(FilterOptions{})
	if err != nil {
		t.Fatalf("BuildFilter 默认值不应报错：%v", err)
	}
	if f.Platform != SourceAll {
		t.Errorf("默认平台应为 all，实际 %q", f.Platform)
	}
	if f.Cwd != wd {
		t.Errorf("默认作用域应为当前工作目录 %q，实际 %q", wd, f.Cwd)
	}
	if f.Global {
		t.Error("默认不应为全局")
	}
	if f.Limit != 50 {
		t.Errorf("默认 limit 应为 50，实际 %d", f.Limit)
	}
	if f.HasSince || f.HasUntil {
		t.Error("默认不应有时间边界")
	}
	if f.Phase != PhaseAll {
		t.Errorf("默认阶段应为 all，实际 %q", f.Phase)
	}
}

func TestBuildFilterGlobal(t *testing.T) {
	f, err := BuildFilter(FilterOptions{Global: true, Cwd: "/somewhere"})
	if err != nil {
		t.Fatal(err)
	}
	if !f.Global || f.Cwd != "" {
		t.Errorf("--global 时 Cwd 应为空，实际 Global=%v Cwd=%q", f.Global, f.Cwd)
	}
}

func TestBuildFilterExplicitCwd(t *testing.T) {
	dir := t.TempDir()
	f, err := BuildFilter(FilterOptions{Cwd: dir})
	if err != nil {
		t.Fatal(err)
	}
	if f.Cwd != dir {
		t.Errorf("--cwd 应为 %q，实际 %q", dir, f.Cwd)
	}
}

func TestBuildFilterBadPlatform(t *testing.T) {
	if _, err := BuildFilter(FilterOptions{Platform: "foo"}); err == nil {
		t.Error("非法 --platform 应报错")
	} else if !strings.Contains(err.Error(), "未知平台") {
		t.Errorf("报错应说明未知平台，实际：%v", err)
	}
	for _, p := range []string{"claude", "codex", "devin", "grok", "opencode", "pi", "zcode", "all"} {
		if _, err := BuildFilter(FilterOptions{Platform: p, Global: true}); err != nil {
			t.Errorf("合法平台 %q 不应报错：%v", p, err)
		}
	}
}

func TestBuildFilterDates(t *testing.T) {
	f, err := BuildFilter(FilterOptions{Since: "2026-04-01", Until: "2026-04-30", Global: true})
	if err != nil {
		t.Fatal(err)
	}
	if !f.HasSince || !f.HasUntil {
		t.Fatal("since/until 标记应为真")
	}
	if got := f.Since.Format(time.RFC3339); got != "2026-04-01T00:00:00Z" {
		t.Errorf("since 应为当天 00:00:00Z，实际 %s", got)
	}
	if got := f.Until.Format("2006-01-02 15:04:05.000"); got != "2026-04-30 23:59:59.999" {
		t.Errorf("until 应取当天 23:59:59.999，实际 %s", got)
	}
	if _, err := BuildFilter(FilterOptions{Since: "2026-13-01"}); err == nil {
		t.Error("非法 --since 应报错")
	}
	if _, err := BuildFilter(FilterOptions{Since: "04/01/2026"}); err == nil {
		t.Error("非 YYYY-MM-DD 的 --since 应报错")
	}
	if _, err := BuildFilter(FilterOptions{Until: "不是日期"}); err == nil {
		t.Error("非法 --until 应报错")
	}
}

func TestParsePhase(t *testing.T) {
	for _, tc := range []struct {
		raw  string
		want MemPhase
	}{
		{"", PhaseAll},
		{"all", PhaseAll},
		{"brainstorm", PhaseBrainstorm},
		{"implement", PhaseImplement},
	} {
		got, err := ParsePhase(tc.raw)
		if err != nil || got != tc.want {
			t.Errorf("ParsePhase(%q) = %q, %v；期望 %q", tc.raw, got, err, tc.want)
		}
	}
	if _, err := ParsePhase("foo"); err == nil {
		t.Error("非法 --phase 应报错")
	}
}

func TestSameProject(t *testing.T) {
	cases := []struct {
		session, target string
		want            bool
	}{
		{"/proj", "/proj", true},
		{"/proj/sub/deep", "/proj", true},
		{"/proj", "", true},  // 不限定
		{"", "/proj", false}, // 会话目录未知
		{"/other", "/proj", false},
		{"/proj2", "/proj", false}, // 前缀相同但不是子目录
	}
	for _, c := range cases {
		if got := SameProject(c.session, c.target); got != c.want {
			t.Errorf("SameProject(%q, %q) = %v；期望 %v", c.session, c.target, got, c.want)
		}
	}
}

func TestInRangeOverlap(t *testing.T) {
	f, _ := BuildFilter(FilterOptions{Since: "2026-04-10", Until: "2026-04-20", Global: true})
	cases := []struct {
		start, end string
		want       bool
	}{
		{"2026-04-12T00:00:00Z", "2026-04-13T00:00:00Z", true},
		{"2026-04-01T00:00:00Z", "2026-04-11T00:00:00Z", true},  // 跨天会话：结束在窗口内
		{"2026-04-01T00:00:00Z", "2026-04-05T00:00:00Z", false}, // 全程在窗口前
		{"2026-04-25T00:00:00Z", "2026-04-26T00:00:00Z", false}, // 全程在窗口后
		{"", "", true},
		{"not-a-date", "", true}, // 无法解析则放行
	}
	for _, c := range cases {
		if got := InRangeOverlap(c.start, c.end, f); got != c.want {
			t.Errorf("InRangeOverlap(%q, %q) = %v；期望 %v", c.start, c.end, got, c.want)
		}
	}
}

func TestResolveFilter(t *testing.T) {
	f := ResolveFilter(MemFilter{})
	if f.Platform != SourceAll || f.Limit != 50 || f.Phase != PhaseAll {
		t.Errorf("ResolveFilter 默认值错误：%+v", f)
	}
}

func TestListSessionsFilterSortLimit(t *testing.T) {
	ctx := context.Background()
	adapters := []Adapter{
		&fakeAdapter{
			kind: MemSourceClaude,
			sessions: []MemSessionInfo{
				testSession(MemSourceClaude, "c-old", "/proj", "2026-04-01T00:00:00Z", "2026-04-01T00:00:00Z"),
				testSession(MemSourceClaude, "c-new", "/proj/sub", "2026-04-03T00:00:00Z", "2026-04-03T00:00:00Z"),
				testSession(MemSourceClaude, "c-other", "/other", "2026-04-04T00:00:00Z", "2026-04-04T00:00:00Z"),
			},
		},
		&fakeAdapter{
			kind: MemSourceCodex,
			sessions: []MemSessionInfo{
				testSession(MemSourceCodex, "x-new", "/proj", "2026-04-02T00:00:00Z", "2026-04-02T00:00:00Z"),
			},
		},
	}
	f, err := BuildFilter(FilterOptions{Cwd: "/proj"})
	if err != nil {
		t.Fatal(err)
	}
	rows, warnings, err := ListSessions(ctx, "/home/test", f, adapters)
	if err != nil {
		t.Fatal(err)
	}
	if len(warnings) != 0 {
		t.Errorf("不应有警告：%v", warnings)
	}
	// 作用域 /proj（含子目录）去掉 c-other；按最新时间倒序。
	want := []string{"c-new", "x-new", "c-old"}
	if len(rows) != len(want) {
		t.Fatalf("期望 %d 条，实际 %d 条：%v", len(want), len(rows), rows)
	}
	for i, id := range want {
		if rows[i].ID != id {
			t.Errorf("第 %d 条期望 %s，实际 %s", i, id, rows[i].ID)
		}
	}

	// 平台过滤。
	f.Platform = MemSourceFilter(MemSourceCodex)
	rows, _, err = ListSessions(ctx, "/home/test", f, adapters)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || rows[0].ID != "x-new" {
		t.Errorf("平台过滤后应只剩 x-new，实际 %v", rows)
	}

	// limit 截断。
	f.Platform = SourceAll
	f.Limit = 2
	rows, _, err = ListSessions(ctx, "/home/test", f, adapters)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 2 || rows[0].ID != "c-new" || rows[1].ID != "x-new" {
		t.Errorf("limit=2 应取前两条，实际 %v", rows)
	}

	// 时间窗口：只要 updated 在 04-02 当天。
	f.Limit = 50
	f.Since = time.Date(2026, 4, 2, 0, 0, 0, 0, time.UTC)
	f.HasSince = true
	f.Until = time.Date(2026, 4, 2, 23, 59, 59, 999*int(time.Millisecond), time.UTC)
	f.HasUntil = true
	rows, _, err = ListSessions(ctx, "/home/test", f, adapters)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || rows[0].ID != "x-new" {
		t.Errorf("时间窗口内应只剩 x-new，实际 %v", rows)
	}
}

func TestListSessionsAdapterErrorIsWarning(t *testing.T) {
	ctx := context.Background()
	adapters := []Adapter{
		&fakeAdapter{kind: MemSourceClaude, err: errors.New("存储损坏")},
		&fakeAdapter{
			kind:     MemSourceCodex,
			sessions: []MemSessionInfo{testSession(MemSourceCodex, "x1", "/proj", "2026-04-01T00:00:00Z", "")},
		},
	}
	f, _ := BuildFilter(FilterOptions{Global: true})
	rows, warnings, err := ListSessions(ctx, "/home/test", f, adapters)
	if err != nil {
		t.Fatalf("适配器出错不应让汇总失败：%v", err)
	}
	if len(rows) != 1 || rows[0].ID != "x1" {
		t.Errorf("出错平台应跳过、其余平台继续，实际 %v", rows)
	}
	if len(warnings) != 1 || !strings.Contains(warnings[0], "codex") && !strings.Contains(warnings[0], "claude") {
		t.Errorf("应有一条指向出错平台的警告，实际 %v", warnings)
	}
}

func TestFindSessionByID(t *testing.T) {
	ctx := context.Background()
	adapters := []Adapter{
		&fakeAdapter{
			kind: MemSourceClaude,
			sessions: []MemSessionInfo{
				testSession(MemSourceClaude, "abcdef123456", "/proj", "2026-04-01T00:00:00Z", ""),
				testSession(MemSourceClaude, "abcdef999999", "/proj", "2026-04-02T00:00:00Z", ""),
			},
		},
	}
	// 精确匹配。
	s, _, err := FindSessionByID(ctx, "/home/test", "abcdef999999", MemFilter{}, adapters)
	if err != nil || s.ID != "abcdef999999" {
		t.Errorf("精确查找失败：%v %v", s, err)
	}
	// 前缀匹配（忽略项目作用域）。
	f, _ := BuildFilter(FilterOptions{Cwd: "/nowhere"})
	s, _, err = FindSessionByID(ctx, "/home/test", "abcdef1", f, adapters)
	if err != nil || s.ID != "abcdef123456" {
		t.Errorf("前缀查找失败：%v %v", s, err)
	}
	// 找不到。
	_, _, err = FindSessionByID(ctx, "/home/test", "zzzz", MemFilter{}, adapters)
	var notFound *SessionNotFoundError
	if !errors.As(err, &notFound) {
		t.Errorf("找不到时应返回 *SessionNotFoundError，实际 %T %v", err, err)
	} else if notFound.ID != "zzzz" {
		t.Errorf("错误里的会话 id 应为 zzzz，实际 %q", notFound.ID)
	}
}

func TestLoadDialogueDispatch(t *testing.T) {
	ctx := context.Background()
	want := []DialogueTurn{{Role: RoleUser, Text: "你好"}}
	adapters := []Adapter{
		&fakeAdapter{
			kind:     MemSourceClaude,
			dialogue: map[string][]DialogueTurn{"s1": want},
		},
	}
	turns, _, err := LoadDialogue(ctx, testSession(MemSourceClaude, "s1", "/proj", "", ""), adapters)
	if err != nil {
		t.Fatal(err)
	}
	if len(turns) != 1 || turns[0].Text != "你好" {
		t.Errorf("应按平台分发到 Claude 适配器，实际 %v", turns)
	}
	if _, _, err := LoadDialogue(ctx, testSession(MemSourceCodex, "s1", "/proj", "", ""), adapters); err == nil {
		t.Error("无对应适配器时应报错")
	}
}

func TestLoadCollectDispatch(t *testing.T) {
	ctx := context.Background()
	want := []DialogueTurn{{Role: RoleUser, Text: "你好"}}
	adapters := []Adapter{
		&fakeAdapter{
			kind:     MemSourceClaude,
			dialogue: map[string][]DialogueTurn{"s1": want},
		},
	}
	turns, events, _, err := LoadCollect(ctx, testSession(MemSourceClaude, "s1", "/proj", "", ""), adapters)
	if err != nil {
		t.Fatal(err)
	}
	if len(turns) != 1 || turns[0].Text != "你好" {
		t.Errorf("应按平台分发到 Claude 适配器，实际 %v", turns)
	}
	if len(events) != 0 {
		t.Errorf("测试替身无事件，实际 %v", events)
	}
	if _, _, _, err := LoadCollect(ctx, testSession(MemSourceCodex, "s1", "/proj", "", ""), adapters); err == nil {
		t.Error("无对应适配器时应报错")
	}
}

func TestListProjects(t *testing.T) {
	ctx := context.Background()
	adapters := []Adapter{
		&fakeAdapter{
			kind: MemSourceClaude,
			sessions: []MemSessionInfo{
				testSession(MemSourceClaude, "a1", "/projA", "2026-04-01T00:00:00Z", "2026-04-03T00:00:00Z"),
				testSession(MemSourceClaude, "a2", "/projA", "2026-04-01T00:00:00Z", "2026-04-01T00:00:00Z"),
				testSession(MemSourceClaude, "b1", "/projB", "2026-04-02T00:00:00Z", ""),
				testSession(MemSourceClaude, "nocwd", "", "2026-04-05T00:00:00Z", ""),
			},
		},
		&fakeAdapter{
			kind:     MemSourceCodex,
			sessions: []MemSessionInfo{testSession(MemSourceCodex, "a3", "/projA", "2026-04-02T00:00:00Z", "")},
		},
	}
	f, _ := BuildFilter(FilterOptions{Cwd: "/nowhere"})
	rows, _, err := ListProjects(ctx, "/home/test", f, adapters)
	if err != nil {
		t.Fatal(err)
	}
	// projects 始终全局扫描：/nowhere 作用域应被丢掉；无 cwd 的会话不计入。
	if len(rows) != 2 {
		t.Fatalf("期望 2 个项目，实际 %v", rows)
	}
	if rows[0].Cwd != "/projA" || rows[0].Sessions != 3 || rows[0].LastActive != "2026-04-03T00:00:00Z" {
		t.Errorf("首个项目应为 /projA（3 个会话，最近 04-03），实际 %+v", rows[0])
	}
	if rows[0].ByPlatform[MemSourceClaude] != 2 || rows[0].ByPlatform[MemSourceCodex] != 1 {
		t.Errorf("/projA 的平台计数错误：%v", rows[0].ByPlatform)
	}
	if rows[0].ByPlatform[MemSourcePi] != 0 {
		t.Errorf("无会话的平台计数应为 0：%v", rows[0].ByPlatform)
	}
	if rows[1].Cwd != "/projB" || rows[1].Sessions != 1 {
		t.Errorf("第二个项目应为 /projB（1 个会话），实际 %+v", rows[1])
	}
}

func TestMemSessionInfoJSONFieldNames(t *testing.T) {
	s := MemSessionInfo{
		Platform: MemSourceClaude,
		ID:       "abc123",
		Title:    "标题",
		CWD:      "/proj",
		Created:  "2026-04-01T00:00:00Z",
		Updated:  "2026-04-02T00:00:00Z",
		FilePath: "/home/u/.claude/projects/x/abc123.jsonl",
		ParentID: "parent1",
	}
	data, err := json.Marshal(s)
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]any
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"platform", "id", "title", "cwd", "created", "updated", "filePath", "parent_id"} {
		if _, ok := m[key]; !ok {
			t.Errorf("JSON 缺少字段 %q：%s", key, data)
		}
	}
	// 可选字段为空时省略（与上游 JSON.stringify 一致）。
	s2 := MemSessionInfo{Platform: MemSourceCodex, ID: "x", FilePath: "/f"}
	data2, _ := json.Marshal(s2)
	var m2 map[string]any
	_ = json.Unmarshal(data2, &m2)
	for _, key := range []string{"title", "cwd", "created", "updated", "parent_id"} {
		if _, ok := m2[key]; ok {
			t.Errorf("空字段 %q 应省略：%s", key, data2)
		}
	}
}

func TestShortDateShortPath(t *testing.T) {
	if got := ShortDate("2026-04-01T10:20:30Z"); got != "2026-04-01 10:20" {
		t.Errorf("ShortDate 应截成 YYYY-MM-DD HH:MM，实际 %q", got)
	}
	if got := ShortDate(""); strings.TrimSpace(got) != "" || len(got) != 9 {
		t.Errorf("无时间戳时 ShortDate 应返回 9 个空格，实际 %q", got)
	}
	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatal(err)
	}
	if got := ShortPath(filepath.Join(home, "proj")); got != filepath.Join("~", "proj") {
		t.Errorf("主目录应缩写成 ~，实际 %q", got)
	}
	if got := ShortPath(""); got != "（无工作目录）" {
		t.Errorf("空路径应给占位说明，实际 %q", got)
	}
}
