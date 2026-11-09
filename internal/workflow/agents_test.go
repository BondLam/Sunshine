package workflow

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestCollectReferencedAgents(t *testing.T) {
	content := "用 `sunshine channel spawn --agent implement` 派发；" +
		"定义见 `.sunshine/agents/implement.md` 与 `.sunshine/agents/check.md`；" +
		"再派一次 `sunshine channel spawn --agent=check --as check-cc`。"
	got := CollectReferencedAgents(content)
	want := []string{"check", "implement"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("应提取 %v，实际 %v", want, got)
	}
	if len(CollectReferencedAgents("没有引用")) != 0 {
		t.Error("无引用时应返回空")
	}
}

func TestCollectMissingAgents(t *testing.T) {
	cwd := t.TempDir()
	agentsDir := filepath.Join(cwd, ".sunshine", "agents")
	if err := os.MkdirAll(agentsDir, 0o755); err != nil {
		t.Fatal(err)
	}
	// implement 以 .md 形式存在；research 以 <名>/AGENT.md 形式存在；check 缺失。
	if err := os.WriteFile(filepath.Join(agentsDir, "implement.md"), []byte("# implement\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(agentsDir, "research"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(agentsDir, "research", "AGENT.md"), []byte("# research\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	content := "spawn --agent implement、spawn --agent check、spawn --agent research"
	got := CollectMissingAgents(cwd, content)
	want := []string{"check"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("缺失的应为 %v，实际 %v", want, got)
	}
}

func TestFormatMissingAgentsWarning(t *testing.T) {
	w := FormatMissingAgentsWarning([]string{"check", "implement"})
	for _, part := range []string{".sunshine/agents/{check,implement}.md", "sunshine update"} {
		if !strings.Contains(w, part) {
			t.Errorf("警告文案应包含 %q，实际：%s", part, w)
		}
	}
}
