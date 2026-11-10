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

func TestParseRegistrySourceDotGitPaths(t *testing.T) {
	// 显式 .git 规则（与 internal/registry 一致）：自建来源路径以 .git
	// 结尾时，整个路径就是仓库，subdir 为空；不带 .git 时保持现有两段
	// 式规则；giget 前缀与公开域名不受影响，仍按两段式走。
	cases := []struct {
		in      string
		repo    string
		subdir  string
		ref     string
		gitURL  string
		host    string
		kind    string
		rawBase string
	}{
		// 两段式现状不变。
		{"ssh://git@localhost:2222/git/registry", "git/registry", "", "main",
			"ssh://git@localhost:2222/git/registry.git", "localhost", "ssh",
			"https://localhost/git/registry/-/raw/main"},
		{"ssh://git@localhost:2222/git/registry/marketplace", "git/registry", "marketplace", "main",
			"ssh://git@localhost:2222/git/registry.git", "localhost", "ssh",
			"https://localhost/git/registry/-/raw/main/marketplace"},
		// 三段式带 .git：ssh-url / scp / https 三路，含带端口。
		{"ssh://git@localhost:2222/home/git/registry.git", "home/git/registry", "", "main",
			"ssh://git@localhost:2222/home/git/registry.git", "localhost", "ssh",
			"https://localhost/home/git/registry/-/raw/main"},
		{"ssh://git@localhost/home/git/registry.git", "home/git/registry", "", "main",
			"ssh://git@localhost/home/git/registry.git", "localhost", "ssh",
			"https://localhost/home/git/registry/-/raw/main"},
		{"git@localhost:home/git/registry.git", "home/git/registry", "", "main",
			"git@localhost:home/git/registry.git", "localhost", "ssh",
			"https://localhost/home/git/registry/-/raw/main"},
		{"https://localhost:8443/home/git/registry.git", "home/git/registry", "", "main",
			"https://localhost:8443/home/git/registry.git", "localhost:8443", "https",
			"https://localhost:8443/home/git/registry/-/raw/main"},
		// 四段式带 .git。
		{"ssh://git@localhost:2222/a/b/c/d.git", "a/b/c/d", "", "main",
			"ssh://git@localhost:2222/a/b/c/d.git", "localhost", "ssh",
			"https://localhost/a/b/c/d/-/raw/main"},
		// 三段式不带 .git：保持现状（前两段是 repo、余下是 subdir）。
		{"ssh://git@localhost:2222/home/git/registry", "home/git", "registry", "main",
			"ssh://git@localhost:2222/home/git.git", "localhost", "ssh",
			"https://localhost/home/git/-/raw/main/registry"},
		{"git@localhost:home/git/registry/marketplace", "home/git", "registry/marketplace", "main",
			"git@localhost:home/git.git", "localhost", "ssh",
			"https://localhost/home/git/-/raw/main/registry/marketplace"},
		// 带 #ref：.git#ref 也要判对；末尾 / 也要判对。
		{"ssh://git@localhost:2222/home/git/registry.git#dev", "home/git/registry", "", "dev",
			"ssh://git@localhost:2222/home/git/registry.git", "localhost", "ssh",
			"https://localhost/home/git/registry/-/raw/dev"},
		{"git@localhost:home/git/registry.git#dev", "home/git/registry", "", "dev",
			"git@localhost:home/git/registry.git", "localhost", "ssh",
			"https://localhost/home/git/registry/-/raw/dev"},
		{"https://localhost:8443/home/git/registry.git#dev", "home/git/registry", "", "dev",
			"https://localhost:8443/home/git/registry.git", "localhost:8443", "https",
			"https://localhost:8443/home/git/registry/-/raw/dev"},
		{"ssh://git@localhost:2222/home/git/registry.git/", "home/git/registry", "", "main",
			"ssh://git@localhost:2222/home/git/registry.git", "localhost", "ssh",
			"https://localhost/home/git/registry/-/raw/main"},
		// 公开域名不受影响，仍按两段式走。
		{"ssh://git@github.com:2222/home/git/registry.git", "home/git", "registry", "main",
			"ssh://git@github.com:2222/home/git.git", "", "ssh",
			"https://raw.githubusercontent.com/home/git/main/registry"},
	}
	for _, c := range cases {
		r, err := ParseRegistrySource(c.in)
		if err != nil {
			t.Errorf("解析 %q 失败：%v", c.in, err)
			continue
		}
		if r.Repo != c.repo || r.Subdir != c.subdir || r.Ref != c.ref ||
			r.GitURL != c.gitURL || r.Host != c.host || r.Kind != c.kind {
			t.Errorf("解析 %q 得 %+v，想得到 Repo=%q Subdir=%q Ref=%q GitURL=%q Host=%q Kind=%q",
				c.in, r, c.repo, c.subdir, c.ref, c.gitURL, c.host, c.kind)
		}
		if r.RawBaseURL != c.rawBase {
			t.Errorf("解析 %q 的 rawBaseURL = %q，想得到 %q", c.in, r.RawBaseURL, c.rawBase)
		}
		if !r.PreferGit {
			t.Errorf("解析 %q 不应走 HTTP（PreferGit 应为真）", c.in)
		}
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
