package adapters

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/BondLam/Sunshine/internal/mem"
)

func TestRegistryAll(t *testing.T) {
	all := All()
	want := []mem.MemSourceKind{
		mem.MemSourceClaude,
		mem.MemSourceCodex,
		mem.MemSourceDevin,
		mem.MemSourceGrok,
		mem.MemSourceOpencode,
		mem.MemSourcePi,
		mem.MemSourceZcode,
	}
	if len(all) != len(want) {
		t.Fatalf("适配器数量 = %d，想要 %d", len(all), len(want))
	}
	for i, adapter := range all {
		if adapter.Kind() != want[i] {
			t.Errorf("第 %d 个适配器 = %q，想要 %q", i, adapter.Kind(), want[i])
		}
	}
}

func TestSessionsMissingStorage(t *testing.T) {
	home := newTempHome(t)
	ctx := context.Background()
	for _, adapter := range All() {
		sessions, warnings, err := adapter.Sessions(ctx, home)
		if err != nil {
			t.Errorf("%s：存储缺失时不应返回错误，实际：%v", adapter.Kind(), err)
		}
		if len(sessions) != 0 {
			t.Errorf("%s：存储缺失时应返回空列表，实际 %d 条", adapter.Kind(), len(sessions))
		}
		if len(warnings) == 0 {
			t.Errorf("%s：存储缺失时应给出警告", adapter.Kind())
		}
	}
}

func TestDialogueMissingFile(t *testing.T) {
	home := newTempHome(t)
	ctx := context.Background()
	for _, adapter := range All() {
		turns, warnings, err := adapter.Dialogue(ctx, mem.MemSessionInfo{
			Platform: adapter.Kind(),
			ID:       "不存在的会话",
			FilePath: filepath.Join(home, "不存在", "nope.jsonl"),
		})
		if err != nil {
			t.Errorf("%s：文件缺失时不应返回错误，实际：%v", adapter.Kind(), err)
		}
		if len(turns) != 0 {
			t.Errorf("%s：文件缺失时应返回空对话，实际 %d 轮", adapter.Kind(), len(turns))
		}
		if len(warnings) == 0 {
			t.Errorf("%s：文件缺失时应给出警告", adapter.Kind())
		}
	}
}

func TestReadJSONLBadLines(t *testing.T) {
	home := newTempHome(t)
	path := filepath.Join(home, "bad.jsonl")
	writeFixture(t, path, "{\"a\": 1}\n不是 JSON\n{\"b\": 2}\n{坏行\n\n{\"c\": 3}")
	var seen []string
	warnings := ReadJSONL(path, func(obj map[string]any) bool {
		if v, ok := obj["a"]; ok {
			seen = append(seen, "a")
			_ = v
		}
		if _, ok := obj["b"]; ok {
			seen = append(seen, "b")
		}
		if _, ok := obj["c"]; ok {
			seen = append(seen, "c")
		}
		return true
	})
	if len(seen) != 3 {
		t.Errorf("可解析行 = %v，想要 [a b c]", seen)
	}
	if len(warnings) == 0 {
		t.Error("坏行应产生警告")
	}
	mustContainWarning(t, warnings, "跳过 2 行")
}

func TestReadJSONLFirst(t *testing.T) {
	home := newTempHome(t)
	path := filepath.Join(home, "first.jsonl")
	writeFixture(t, path, "坏行\n{\"id\": \"第一行\"}\n{\"id\": \"第二行\"}\n")
	first, warnings := ReadJSONLFirst(path)
	if first == nil || first["id"] != "第一行" {
		t.Errorf("首行 = %v，想要 id=第一行", first)
	}
	if len(warnings) == 0 {
		t.Error("坏行应产生警告")
	}
}

func TestReadJSONLMissingFile(t *testing.T) {
	warnings := ReadJSONL("/不存在的目录/不存在.jsonl", func(obj map[string]any) bool { return true })
	if len(warnings) == 0 {
		t.Error("打不开的文件应产生警告")
	}
}

func TestStripInjectionTags(t *testing.T) {
	got := stripInjectionTags("前面<system-reminder>注入内容</system-reminder>后面<workflow-state>状态</workflow-state>结尾")
	if got != "前面后面结尾" {
		t.Errorf("剥离结果 = %q，想要 %q", got, "前面后面结尾")
	}
	if got := stripInjectionTags("正常文本\n\n\n\n下一段"); got != "正常文本\n\n下一段" {
		t.Errorf("换行压缩结果 = %q", got)
	}
}

func TestIsBootstrapTurn(t *testing.T) {
	if !isBootstrapTurn("# AGENTS.md instructions for xxx", 100) {
		t.Error("AGENTS.md 前言应判定为启动注入")
	}
	long := make([]byte, 5000)
	for i := range long {
		long[i] = 'x'
	}
	if !isBootstrapTurn("<INSTRUCTIONS>"+string(long), 5000) {
		t.Error("超长 INSTRUCTIONS 块应判定为启动注入")
	}
	if isBootstrapTurn("普通对话", 100) {
		t.Error("普通对话不应判定为启动注入")
	}
}

func TestCompactionBoundaryTurn(t *testing.T) {
	turn := compactionBoundaryTurn("此处上下文已压缩", "摘要文本")
	if turn.Kind != "marker" {
		t.Errorf("marker 轮次 kind = %q，想要 marker", turn.Kind)
	}
	if turn.Role != "user" {
		t.Errorf("marker 轮次 role = %q，想要 user", turn.Role)
	}
	if !contains(turn.Text, CompactBoundaryPrefix) {
		t.Errorf("marker 文本应以 %q 开头，实际 %q", CompactBoundaryPrefix, turn.Text)
	}
	if !contains(turn.Text, "摘要文本") {
		t.Errorf("marker 文本应包含摘要，实际 %q", turn.Text)
	}
}

func TestPathsEncoding(t *testing.T) {
	home := "/home/test"
	if got := ClaudeProjectDirFromCwd(home, "/Users/x/.codex/snap_note"); got != filepath.Join(home, ".claude", "projects", "-Users-x--codex-snap-note") {
		t.Errorf("Claude 项目目录 = %q", got)
	}
	if got := GrokProjectDirFromCwd(home, "/Users/x/proj"); got != filepath.Join(home, ".grok", "sessions", "%2FUsers%2Fx%2Fproj") {
		t.Errorf("Grok 项目目录 = %q", got)
	}
	if got := GrokCwdFromProjectDir("%2FUsers%2Fx%2Fproj"); got != "/Users/x/proj" {
		t.Errorf("Grok 目录反解 = %q", got)
	}
	if got := GrokCwdFromProjectDir("%zz"); got != "" {
		t.Errorf("非法百分号编码应返回空，实际 %q", got)
	}
	if got := PiProjectDirFromCwd(home, "/tmp/proj"); got != filepath.Join(home, ".pi", "agent", "sessions", "--tmp-proj--") {
		t.Errorf("Pi 项目目录 = %q", got)
	}
	if got := ExpandHome(home, "~/data/x.db"); got != filepath.Join(home, "data", "x.db") {
		t.Errorf("~ 展开 = %q", got)
	}
}

func TestDevinDBPathResolution(t *testing.T) {
	home := newTempHome(t)
	dataDir := filepath.Join(home, ".local", "share", "devin", "cli")
	customAbs := filepath.Join(home, "custom", "devin.db")
	writeFixture(t, customAbs, "")
	writeFixture(t, filepath.Join(home, "rel.db"), "")

	cases := []struct {
		name   string
		env    string
		want   string
		wantOK bool
	}{
		{"默认", "", filepath.Join(dataDir, "sessions.db"), true},
		{"绝对路径", customAbs, customAbs, true},
		{"~/ 展开", "~/rel.db", filepath.Join(home, "rel.db"), true},
		{"相对名拼数据目录", "mine.db", filepath.Join(dataDir, "mine.db"), true},
		{":memory: 无文件", ":memory:", "", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("DEVIN_DB_PATH", tc.env)
			got, ok := DevinDBPath(home)
			if ok != tc.wantOK || got != tc.want {
				t.Errorf("DevinDBPath = (%q, %v)，想要 (%q, %v)", got, ok, tc.want, tc.wantOK)
			}
		})
	}
}

func TestOpencodeDBPath(t *testing.T) {
	home := newTempHome(t)
	t.Run("XDG 为空时无库", func(t *testing.T) {
		t.Setenv("XDG_DATA_HOME", "")
		t.Setenv("OPENCODE_DB", "")
		if _, ok := OpencodeDBPath(home); ok {
			t.Error("无库时应返回 false")
		}
	})
	t.Run("主库优先", func(t *testing.T) {
		dir := filepath.Join(home, ".local", "share", "opencode")
		writeFixture(t, filepath.Join(dir, "opencode.db"), "")
		writeFixture(t, filepath.Join(dir, "opencode-beta.db"), "")
		t.Setenv("XDG_DATA_HOME", "")
		t.Setenv("OPENCODE_DB", "")
		got, ok := OpencodeDBPath(home)
		if !ok || got != filepath.Join(dir, "opencode.db") {
			t.Errorf("应选中主库，实际 (%q, %v)", got, ok)
		}
	})
	t.Run("无主库时取最近写入的 channel 库", func(t *testing.T) {
		home2 := newTempHome(t)
		dir := filepath.Join(home2, ".local", "share", "opencode")
		oldPath := filepath.Join(dir, "opencode-beta.db")
		newPath := filepath.Join(dir, "opencode-dev.db")
		writeFixture(t, oldPath, "")
		writeFixture(t, newPath, "")
		past := time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)
		if err := os.Chtimes(oldPath, past, past); err != nil {
			t.Fatalf("改旧库时间失败：%v", err)
		}
		t.Setenv("XDG_DATA_HOME", "")
		t.Setenv("OPENCODE_DB", "")
		got, ok := OpencodeDBPath(home2)
		if !ok || got != newPath {
			t.Errorf("应选中最近写入的 channel 库，实际 (%q, %v)", got, ok)
		}
	})
	t.Run("OPENCODE_DB 覆盖", func(t *testing.T) {
		abs := filepath.Join(home, "elsewhere.db")
		writeFixture(t, abs, "")
		t.Setenv("OPENCODE_DB", abs)
		got, ok := OpencodeDBPath(home)
		if !ok || got != abs {
			t.Errorf("绝对路径覆盖应直接用，实际 (%q, %v)", got, ok)
		}
		t.Setenv("OPENCODE_DB", ":memory:")
		if _, ok := OpencodeDBPath(home); ok {
			t.Error(":memory: 应返回 false")
		}
	})
	t.Run("XDG_DATA_HOME 生效", func(t *testing.T) {
		xdg := filepath.Join(home, "xdg-data")
		writeFixture(t, filepath.Join(xdg, "opencode", "opencode.db"), "")
		t.Setenv("XDG_DATA_HOME", xdg)
		t.Setenv("OPENCODE_DB", "")
		got, ok := OpencodeDBPath(home)
		if !ok || got != filepath.Join(xdg, "opencode", "opencode.db") {
			t.Errorf("应走 XDG 目录，实际 (%q, %v)", got, ok)
		}
	})
}
