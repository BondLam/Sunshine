package workflow

import (
	"strings"
	"testing"
)

func TestParseRegistrySourceGigetPrefixes(t *testing.T) {
	cases := []struct {
		source   string
		provider string
		repo     string
		subdir   string
		ref      string
		rawBase  string
	}{
		{"gh:user/repo", "gh", "user/repo", "", "main", "https://raw.githubusercontent.com/user/repo/main"},
		{"gh:user/repo/sub/dir#dev", "gh", "user/repo", "sub/dir", "dev", "https://raw.githubusercontent.com/user/repo/dev/sub/dir"},
		{"github:user/repo#v1", "github", "user/repo", "", "v1", "https://raw.githubusercontent.com/user/repo/v1"},
		{"gitlab:user/repo", "gitlab", "user/repo", "", "main", "https://gitlab.com/user/repo/-/raw/main"},
		{"bitbucket:user/repo", "bitbucket", "user/repo", "", "main", "https://bitbucket.org/user/repo/raw/main"},
	}
	for _, c := range cases {
		r, err := ParseRegistrySource(c.source)
		if err != nil {
			t.Errorf("解析 %q 失败：%v", c.source, err)
			continue
		}
		if r.Provider != c.provider || r.Repo != c.repo || r.Subdir != c.subdir || r.Ref != c.ref {
			t.Errorf("解析 %q 得 %+v，不符 %+v", c.source, r, c)
		}
		if r.RawBaseURL != c.rawBase {
			t.Errorf("解析 %q 的 rawBaseURL 应为 %q，实际 %q", c.source, c.rawBase, r.RawBaseURL)
		}
		if r.PreferGit {
			t.Errorf("解析 %q 不应走 git（preferGit 应为假）", c.source)
		}
	}
}

func TestParseRegistrySourceHTTPS(t *testing.T) {
	r, err := ParseRegistrySource("https://github.com/user/repo")
	if err != nil {
		t.Fatalf("解析失败：%v", err)
	}
	if r.Provider != "gh" || r.Repo != "user/repo" || r.Ref != "main" {
		t.Errorf("解析结果不符：%+v", r)
	}
	if r.RawBaseURL != "https://raw.githubusercontent.com/user/repo/main" {
		t.Errorf("rawBaseURL 不符：%q", r.RawBaseURL)
	}

	r, err = ParseRegistrySource("https://github.com/user/repo/tree/dev/marketplace")
	if err != nil {
		t.Fatalf("解析 tree 地址失败：%v", err)
	}
	if r.Repo != "user/repo" || r.Subdir != "marketplace" || r.Ref != "dev" {
		t.Errorf("tree 地址解析不符：%+v", r)
	}
}

func TestParseRegistrySourceSSH(t *testing.T) {
	r, err := ParseRegistrySource("git@github.com:user/repo")
	if err != nil {
		t.Fatalf("解析失败：%v", err)
	}
	if r.Provider != "gh" || !r.PreferGit || r.Kind != "ssh" {
		t.Errorf("SSH 解析不符：%+v", r)
	}
	if r.GitURL != "git@github.com:user/repo.git" {
		t.Errorf("gitURL 不符：%q", r.GitURL)
	}

	r, err = ParseRegistrySource("ssh://git@github.com:2222/user/repo.git")
	if err != nil {
		t.Fatalf("解析 ssh:// 失败：%v", err)
	}
	if !r.PreferGit || r.GitURL != "ssh://git@github.com:2222/user/repo.git" {
		t.Errorf("ssh:// 解析不符：%+v", r)
	}
}

func TestParseRegistrySourceSelfHosted(t *testing.T) {
	r, err := ParseRegistrySource("https://git.corp.com/org/repo")
	if err != nil {
		t.Fatalf("解析失败：%v", err)
	}
	if r.Provider != "gitlab" || r.Host != "git.corp.com" || !r.PreferGit {
		t.Errorf("自托管解析不符：%+v", r)
	}
	if !strings.HasPrefix(r.RawBaseURL, "https://git.corp.com/org/repo/-/raw/main") {
		t.Errorf("自托管 rawBaseURL 不符：%q", r.RawBaseURL)
	}

	r, err = ParseRegistrySource("git@git.corp.com:org/repo")
	if err != nil {
		t.Fatalf("解析自托管 SSH 失败：%v", err)
	}
	if r.Host != "git.corp.com" || !r.PreferGit {
		t.Errorf("自托管 SSH 解析不符：%+v", r)
	}
}

func TestParseRegistrySourceErrors(t *testing.T) {
	for _, bad := range []string{
		"./local/marketplace",
		"../marketplace",
		"/abs/marketplace",
		"~/marketplace",
		"file:///tmp/marketplace",
		"unknown:user/repo",
		"gh:onlyone",
		"just-a-word",
	} {
		if _, err := ParseRegistrySource(bad); err == nil {
			t.Errorf("解析 %q 应报错", bad)
		}
	}
}
