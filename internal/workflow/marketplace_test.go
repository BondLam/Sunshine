package workflow

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// marketEntries 是模拟的市场 index 条目（与传输层无关）。
var marketEntries = []indexEntry{
	{ID: "custom-flow", Type: "workflow", Name: "Custom Flow", Description: "自定义工作流", Path: "workflows/custom-flow/workflow.md"},
	{ID: "bad-path", Type: "workflow", Name: "Bad Path", Path: "workflows/bad-path/workflow.txt"},
	{ID: "escape", Type: "workflow", Name: "Escape", Path: "../secret.md"},
	{ID: "some-spec", Type: "spec", Name: "Some Spec", Path: "specs/some-spec"},
}

func stubFetchFile(t *testing.T) func(string) (string, error) {
	t.Helper()
	return func(path string) (string, error) {
		if path == "workflows/custom-flow/workflow.md" {
			return "# 自定义工作流\n\n正文。\n", nil
		}
		return "", fmt.Errorf("文件不存在：%s", path)
	}
}

func TestResolveFromEntries(t *testing.T) {
	r, err := resolveFromEntries("custom-flow", marketEntries, stubFetchFile(t))
	if err != nil {
		t.Fatalf("解析失败：%v", err)
	}
	if r.ID != "custom-flow" || r.Name != "Custom Flow" || r.Source != SourceMarketplace {
		t.Errorf("解析结果不符：%+v", r.Template)
	}
	if !strings.Contains(r.Content, "自定义工作流") {
		t.Errorf("模板正文不符：%q", r.Content)
	}
}

func TestResolveFromEntriesErrors(t *testing.T) {
	if _, err := resolveFromEntries("no-such", marketEntries, stubFetchFile(t)); err == nil {
		t.Error("市场里没有的模板应报错")
	} else if !strings.Contains(err.Error(), "custom-flow") {
		t.Errorf("报错应提示可用模板，实际：%v", err)
	}
	if _, err := resolveFromEntries("bad-path", marketEntries, stubFetchFile(t)); err == nil {
		t.Error("path 不以 .md 结尾的模板应报错")
	}
	if _, err := resolveFromEntries("escape", marketEntries, stubFetchFile(t)); err == nil {
		t.Error("path 逃出市场根目录的模板应报错")
	}
	if _, err := resolveFromEntries("some-spec", marketEntries, stubFetchFile(t)); err == nil {
		t.Error("type 不是 workflow 的条目不应被解析为工作流模板")
	}
}

func TestResolveFromMarketplaceBadSource(t *testing.T) {
	if _, err := ResolveFromMarketplace("tdd", "not-a-valid-source"); err == nil {
		t.Error("无效的市场来源应报错")
	}
	if _, err := ResolveFromMarketplace("tdd", "./local/path"); err == nil {
		t.Error("本地路径市场来源应报错")
	}
}

func TestFetchMarketplaceIndexNotFound(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	}))
	defer ts.Close()
	// 直接测 HTTP 探针：404 应报“没有 index.json”，而不是当成网络错误。
	_, errMsg, notFound := probeIndexHTTP(ts.URL + "/index.json")
	if !notFound || errMsg != "" {
		t.Errorf("404 应返回 notFound，实际 errMsg=%q notFound=%v", errMsg, notFound)
	}
}

func TestFetchWorkflowFileHTTPNotFound(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	}))
	defer ts.Close()
	if _, err := fetchWorkflowFileHTTP(ts.URL, "workflows/x/workflow.md"); err == nil {
		t.Error("文件 404 应报错")
	}
}

func TestValidateWorkflowPath(t *testing.T) {
	for _, bad := range []string{"/abs.md", "../escape.md", "a/../../escape.md", `..\escape.md`} {
		if err := validateWorkflowPath(bad); err == nil {
			t.Errorf("路径 %q 应被拒绝", bad)
		}
	}
	if err := validateWorkflowPath("workflows/x/workflow.md"); err != nil {
		t.Errorf("合法路径不应被拒绝：%v", err)
	}
}

// TestResolveFromMarketplaceGit 用本地 git 仓库覆盖 git 克隆分支：
// 建一个含 index.json 的仓库，以 git@ 形式的 SSH 来源解析会走
// PreferGit；这里直接测 cloneRegistry + probeIndexGit 这一层
// （ResolveFromMarketplace 的 git 分支与之共用同一代码）。
func TestProbeIndexGitLocalRepo(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("本机无 git，跳过")
	}
	repo := t.TempDir()
	run := func(args ...string) {
		cmd := exec.Command("git", args...)
		cmd.Dir = repo
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v 失败：%v\n%s", args, err, out)
		}
	}
	run("init", "-q", "-b", "main")
	run("config", "user.email", "test@example.com")
	run("config", "user.name", "test")
	indexJSON := `{"version": 1, "templates": [
		{"id": "git-flow", "type": "workflow", "name": "Git Flow", "path": "workflows/git-flow/workflow.md"}
	]}`
	if err := os.MkdirAll(filepath.Join(repo, "workflows", "git-flow"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(repo, "index.json"), []byte(indexJSON), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(repo, "workflows", "git-flow", "workflow.md"), []byte("# Git 工作流\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	run("add", ".")
	run("commit", "-q", "-m", "init")

	registry := RegistrySource{
		Provider:  "gh",
		Repo:      "org/repo",
		Ref:       "main",
		GitURL:    repo,
		PreferGit: true,
		Kind:      "ssh",
	}
	idx, err := probeIndexGit(registry)
	if err != nil {
		t.Fatalf("probeIndexGit 失败：%v", err)
	}
	if len(idx.Templates) != 1 || idx.Templates[0].ID != "git-flow" {
		t.Errorf("索引内容不符：%+v", idx.Templates)
	}

	dir, cleanup, err := cloneRegistry(registry)
	if err != nil {
		t.Fatalf("cloneRegistry 失败：%v", err)
	}
	defer cleanup()
	data, err := readRegistryFile(dir, "", "workflows/git-flow/workflow.md")
	if err != nil {
		t.Fatalf("读克隆出的模板失败：%v", err)
	}
	if !strings.Contains(string(data), "Git 工作流") {
		t.Errorf("模板内容不符：%q", data)
	}
	if _, err := readRegistryFile(dir, "", "../secret.md"); err == nil {
		t.Error("逃出根目录的路径应被拒绝")
	}
}
