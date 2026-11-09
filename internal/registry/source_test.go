package registry

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestNormalizeRegistrySource(t *testing.T) {
	cases := []struct{ in, want string }{
		{"https://github.com/user/repo", "gh:user/repo"},
		{"https://github.com/user/repo/tree/main/specs/foo", "gh:user/repo/specs/foo#main"},
		{"https://github.com/user/repo/tree/dev", "gh:user/repo#dev"},
		{"https://github.com/user/repo.git", "gh:user/repo"},
		{"https://gitlab.com/org/repo", "gitlab:org/repo"},
		{"https://bitbucket.org/org/repo/", "bitbucket:org/repo"},
		{"gh:user/repo", "gh:user/repo"},
		{"git@github.com:user/repo", "git@github.com:user/repo"},
		{"https://git.corp.com/org/repo", "https://git.corp.com/org/repo"},
	}
	for _, c := range cases {
		if got := NormalizeRegistrySource(c.in); got != c.want {
			t.Errorf("NormalizeRegistrySource(%q) = %q，想得到 %q", c.in, got, c.want)
		}
	}
}

func TestParseRegistrySource(t *testing.T) {
	cases := []struct {
		in   string
		want Source
	}{
		{
			"gh:owner/repo",
			Source{Provider: "gh", Repo: "owner/repo", Ref: "main",
				RawBaseURL:  "https://raw.githubusercontent.com/owner/repo/main",
				GigetSource: "gh:owner/repo", GitURL: "https://github.com/owner/repo.git",
				Kind: SourceKindPrefixed},
		},
		{
			"gh:owner/repo/marketplace#dev",
			Source{Provider: "gh", Repo: "owner/repo", Subdir: "marketplace", Ref: "dev",
				RawBaseURL:  "https://raw.githubusercontent.com/owner/repo/dev/marketplace",
				GigetSource: "gh:owner/repo/marketplace#dev", GitURL: "https://github.com/owner/repo.git",
				Kind: SourceKindPrefixed},
		},
		{
			"https://github.com/owner/repo/tree/main/specs/foo",
			Source{Provider: "gh", Repo: "owner/repo", Subdir: "specs/foo", Ref: "main",
				RawBaseURL:  "https://raw.githubusercontent.com/owner/repo/main/specs/foo",
				GigetSource: "gh:owner/repo/specs/foo#main", GitURL: "https://github.com/owner/repo.git",
				Kind: SourceKindHTTPS},
		},
		{
			"git@github.com:owner/repo.git",
			Source{Provider: "gh", Repo: "owner/repo", Ref: "main",
				RawBaseURL:  "https://raw.githubusercontent.com/owner/repo/main",
				GigetSource: "gh:owner/repo", GitURL: "git@github.com:owner/repo.git",
				PreferGit: true, Kind: SourceKindSSH},
		},
		{
			"ssh://git@github.com:2222/owner/repo",
			Source{Provider: "gh", Repo: "owner/repo", Ref: "main",
				RawBaseURL:  "https://raw.githubusercontent.com/owner/repo/main",
				GigetSource: "gh:owner/repo", GitURL: "ssh://git@github.com:2222/owner/repo.git",
				PreferGit: true, Kind: SourceKindSSH},
		},
		{
			"git@git.corp.com:owner/repo",
			Source{Provider: "gitlab", Repo: "owner/repo", Ref: "main",
				RawBaseURL:  "https://git.corp.com/owner/repo/-/raw/main",
				GigetSource: "gitlab:owner/repo", GitURL: "git@git.corp.com:owner/repo.git",
				Host: "git.corp.com", PreferGit: true, Kind: SourceKindSSH},
		},
		{
			"https://git.corp.com/owner/repo",
			Source{Provider: "gitlab", Repo: "owner/repo", Ref: "main",
				RawBaseURL:  "https://git.corp.com/owner/repo/-/raw/main",
				GigetSource: "gitlab:owner/repo", GitURL: "https://git.corp.com/owner/repo.git",
				Host: "git.corp.com", PreferGit: true, Kind: SourceKindHTTPS},
		},
		{
			"gitlab:owner/repo#v1.0",
			Source{Provider: "gitlab", Repo: "owner/repo", Ref: "v1.0",
				RawBaseURL:  "https://gitlab.com/owner/repo/-/raw/v1.0",
				GigetSource: "gitlab:owner/repo#v1.0", GitURL: "https://gitlab.com/owner/repo.git",
				Kind: SourceKindPrefixed},
		},
	}
	for _, c := range cases {
		got, err := ParseRegistrySource(c.in)
		if err != nil {
			t.Errorf("ParseRegistrySource(%q) 失败：%v", c.in, err)
			continue
		}
		want := c.want
		want.Original = c.in
		if !reflect.DeepEqual(*got, want) {
			t.Errorf("ParseRegistrySource(%q) =\n%+v\n想得到\n%+v", c.in, *got, want)
		}
	}
}

func TestParseRegistrySourceErrors(t *testing.T) {
	for _, in := range []string{
		"",
		"not-a-source",
		"gh:onlyone",
		"unknown:owner/repo",
		"gh:",
	} {
		if _, err := ParseRegistrySource(in); err == nil {
			t.Errorf("ParseRegistrySource(%q) 应报错", in)
		}
	}
}

func TestParseRegistrySourceLocal(t *testing.T) {
	dir := t.TempDir()
	repo := filepath.Join(dir, "myrepo")
	if err := os.MkdirAll(repo, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, in := range []string{repo, "file://" + repo} {
		got, err := ParseRegistrySource(in)
		if err != nil {
			t.Errorf("ParseRegistrySource(%q) 失败：%v", in, err)
			continue
		}
		if got.Kind != SourceKindLocal || got.Provider != "local" || !got.PreferGit {
			t.Errorf("ParseRegistrySource(%q) = %+v，期望本地来源", in, got)
		}
		if got.GitURL != repo {
			t.Errorf("GitURL = %q，想得到 %q", got.GitURL, repo)
		}
	}
	// 不存在的本地路径不是本地来源，按普通解析走（应报错）。
	if _, err := ParseRegistrySource(filepath.Join(dir, "不存在")); err == nil {
		t.Error("不存在的本地路径应报错")
	}
	// 以 ./ 开头的相对路径：建一个真实目录再解析。
	workdir := t.TempDir()
	sub := filepath.Join(workdir, "repo")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	oldWd, _ := os.Getwd()
	defer os.Chdir(oldWd)
	if err := os.Chdir(workdir); err != nil {
		t.Fatal(err)
	}
	got, err := ParseRegistrySource("./repo")
	if err != nil {
		t.Fatalf("ParseRegistrySource(./repo) 失败：%v", err)
	}
	if got.Kind != SourceKindLocal {
		t.Errorf("Kind = %q，想得到 local", got.Kind)
	}
}

func TestParseTemplateIndex(t *testing.T) {
	// 已确认的 10 条目索引（Sunshine 命名：bootstarp 错拼已更正为
	// sunshine-spec-bootstrap；mem-recall 于阶段 5 收录）。
	raw := `{"version": 1, "templates": [
		{"id": "electron-fullstack", "type": "spec", "name": "Electron", "path": "specs/electron-fullstack"},
		{"id": "nextjs-fullstack", "type": "spec", "name": "Next.js", "path": "specs/nextjs-fullstack"},
		{"id": "cf-workers-fullstack", "type": "spec", "name": "CF Workers", "path": "specs/cf-workers-fullstack"},
		{"id": "sunshine-meta", "type": "skill", "name": "Sunshine Meta", "path": "skills/sunshine-meta"},
		{"id": "sunshine-spec-bootstrap", "type": "skill", "name": "Spec Bootstrap", "path": "skills/sunshine-spec-bootstrap"},
		{"id": "frontend-fullchain-optimization", "type": "skill", "name": "Frontend", "path": "skills/frontend-fullchain-optimization"},
		{"id": "mem-recall", "type": "skill", "name": "Mem Recall", "path": "skills/mem-recall"},
		{"id": "native", "type": "workflow", "name": "Native", "path": "workflows/native/workflow.md"},
		{"id": "tdd", "type": "workflow", "name": "TDD", "path": "workflows/tdd/workflow.md"},
		{"id": "channel-driven-subagent-dispatch", "type": "workflow", "name": "Dispatch", "path": "workflows/channel-driven-subagent-dispatch/workflow.md"}
	]}`
	index, err := ParseTemplateIndex([]byte(raw), "index.json")
	if err != nil {
		t.Fatalf("ParseTemplateIndex 失败：%v", err)
	}
	if len(index.Templates) != 10 {
		t.Fatalf("模板数 = %d，想得到 10", len(index.Templates))
	}
	for _, id := range []string{"sunshine-spec-bootstrap", "native", "mem-recall"} {
		if FindTemplate(index.Templates, id) == nil {
			t.Errorf("找不到模板 %q", id)
		}
	}
	if FindTemplate(index.Templates, "trellis-spec-bootstarp") != nil {
		t.Error("bootstarp 错拼不应沿用")
	}
	if got := InstallPath("skill"); got != ".agents/skills" {
		t.Errorf("InstallPath(skill) = %q", got)
	}
	if got := InstallPath("spec"); got != ".sunshine/spec" {
		t.Errorf("InstallPath(spec) = %q", got)
	}
	if got := InstallPath("command"); got != ".claude/commands" {
		t.Errorf("InstallPath(command) = %q", got)
	}
	if got := InstallPath("full"); got != "." {
		t.Errorf("InstallPath(full) = %q", got)
	}
	if got := InstallPath("weird"); got != ".sunshine/spec" {
		t.Errorf("InstallPath(未知) = %q，想默认 spec", got)
	}
}

func TestParseTemplateIndexErrors(t *testing.T) {
	for _, raw := range []string{
		`not json`,
		`{"version": 1}`,
		`{"version": 1, "templates": [{"id": "x"}]}`,
		`{"version": 1, "templates": ["x"]}`,
	} {
		if _, err := ParseTemplateIndex([]byte(raw), "index.json"); err == nil {
			t.Errorf("ParseTemplateIndex(%q) 应报错", raw)
		} else if !strings.Contains(err.Error(), "index.json") {
			t.Errorf("错误信息应包含来源名：%v", err)
		}
	}
}
