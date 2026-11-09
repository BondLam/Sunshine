package update

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/linrong1982/Sunshine/internal/platforms"
	"github.com/linrong1982/Sunshine/internal/scaffold"
)

// setupProject 用 scaffold + platforms 建一个已初始化的临时项目
// （模拟 sunshine init -y 的结果），返回项目根目录。
func setupProject(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	if err := scaffold.Init(dir, "测试", false); err != nil {
		t.Fatalf("Init 失败：%v", err)
	}
	for _, id := range []string{"claude-code", "cursor"} {
		files, err := platforms.Install(dir, id, platforms.InstallOptions{})
		if err != nil {
			t.Fatalf("安装 %s 失败：%v", id, err)
		}
		if err := scaffold.RecordHashes(dir, files); err != nil {
			t.Fatalf("记录哈希失败：%v", err)
		}
	}
	return dir
}

func readFile(t *testing.T, dir, rel string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(dir, filepath.FromSlash(rel)))
	if err != nil {
		t.Fatalf("读取 %s 失败：%v", rel, err)
	}
	return string(data)
}

func writeFile(t *testing.T, dir, rel, content string) {
	t.Helper()
	p := filepath.Join(dir, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

// fakePrompter 是测试用的交互替身。
type fakePrompter struct {
	actions map[string]ConflictAction
	proceed bool
}

func (f fakePrompter) ConfirmProceed() (bool, error) { return f.proceed, nil }
func (f fakePrompter) ResolveConflict(path string) (ConflictAction, error) {
	if a, ok := f.actions[path]; ok {
		return a, nil
	}
	return ActionSkip, nil
}

func TestCompareVersions(t *testing.T) {
	cases := []struct {
		a, b string
		want int
	}{
		{"0.1.0", "0.1.0", 0},
		{"0.2.0", "0.1.0", 1},
		{"0.1.0", "0.2.0", -1},
		{"0.3.0-beta.1", "0.3.0", -1},
		{"0.3.0", "0.3.0-beta.1", 1},
		{"0.3.0-alpha", "0.3.0-beta", -1},
		{"0.3.0-beta.1", "0.3.0-beta.2", -1},
		{"0.3.0-beta.16", "0.3.0-rc.0", -1},
		{"1.0.0-alpha-1", "1.0.0-alpha-1", 0},
		{"unknown", "0.1.0", -1},
		{"0.1.0", "unknown", 1},
	}
	for _, c := range cases {
		if got := CompareVersions(c.a, c.b); got != c.want {
			t.Errorf("CompareVersions(%q, %q) = %d，期望 %d", c.a, c.b, got, c.want)
		}
	}
}

func TestLoadSaveHashes(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, ".sunshine"), 0o755); err != nil {
		t.Fatal(err)
	}
	// 缺失时返回空。
	if got := LoadHashes(dir); len(got) != 0 {
		t.Errorf("缺失时 LoadHashes = %v", got)
	}
	// Sunshine 的 files 键。
	if err := os.WriteFile(filepath.Join(dir, ".sunshine", ".template-hashes.json"),
		[]byte("{\"__version\": 2, \"files\": {\".claude/settings.json\": \"abc\"}}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := LoadHashes(dir); got[".claude/settings.json"] != "abc" {
		t.Errorf("files 键读取失败：%v", got)
	}
	// 上游的 hashes 键兼容读取。
	if err := os.WriteFile(filepath.Join(dir, ".sunshine", ".template-hashes.json"),
		[]byte("{\"__version\": 2, \"hashes\": {\".claude/settings.json\": \"def\"}}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := LoadHashes(dir); got[".claude/settings.json"] != "def" {
		t.Errorf("hashes 键兼容读取失败：%v", got)
	}
	// 损坏 JSON / 旧格式 → 空。
	for _, bad := range []string{"{ not json", "{\"files\": {\"a\": \"b\"}}", "{\"__version\": 1, \"files\": {}}"} {
		if err := os.WriteFile(filepath.Join(dir, ".sunshine", ".template-hashes.json"), []byte(bad), 0o644); err != nil {
			t.Fatal(err)
		}
		if got := LoadHashes(dir); len(got) != 0 {
			t.Errorf("损坏/旧格式应返回空，实际 %v（输入 %s）", got, bad)
		}
	}
	// 保存用 files 键写出，往返一致。
	hashes := map[string]string{".sunshine/workflow.md": "x", ".claude/settings.json": "y"}
	if err := SaveHashes(dir, hashes); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(filepath.Join(dir, ".sunshine", ".template-hashes.json"))
	if !strings.Contains(string(data), `"files"`) || strings.Contains(string(data), `"hashes"`) {
		t.Errorf("保存应用 files 键：\n%s", data)
	}
	if got := LoadHashes(dir); len(got) != 2 {
		t.Errorf("往返后 = %v", got)
	}
}

func TestShouldExcludeFromHash(t *testing.T) {
	for _, rel := range []string{".sunshine/spec/system.md", ".sunshine/tasks/README.md",
		".sunshine/workspace/index.md", ".sunshine/.version", ".sunshine/.developer",
		".sunshine/.current-task", ".sunshine/.backups/2026-01-01T00-00-00/x"} {
		if !ShouldExcludeFromHash(rel) {
			t.Errorf("%s 应排除", rel)
		}
	}
	for _, rel := range []string{".sunshine/workflow.md", ".sunshine/config.yaml",
		".sunshine/.gitignore", ".sunshine/agents/check.md", ".claude/settings.json"} {
		if ShouldExcludeFromHash(rel) {
			t.Errorf("%s 不应排除", rel)
		}
	}
}
