package registry

import (
	"crypto/ed25519"
	"crypto/rand"
	"errors"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"

	cryptossh "golang.org/x/crypto/ssh"
)

func mustTestHostKey(t *testing.T) cryptossh.PublicKey {
	t.Helper()
	_, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	pub, err := cryptossh.NewPublicKey(priv.Public())
	if err != nil {
		t.Fatal(err)
	}
	return pub
}

func TestSSHHostKeyCallbackAcceptNew(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	pub1 := mustTestHostKey(t)
	pub2 := mustTestHostKey(t)
	cb := sshHostKeyCallback()
	remote := &net.TCPAddr{IP: net.ParseIP("140.82.114.4"), Port: 22}

	// 首次：未知主机，接受并记入 known_hosts。
	if err := cb("github.com:22", remote, pub1); err != nil {
		t.Fatalf("未知主机应接受：%v", err)
	}
	khPath := filepath.Join(home, ".ssh", "known_hosts")
	data, err := os.ReadFile(khPath)
	if err != nil {
		t.Fatalf("known_hosts 应被创建：%v", err)
	}
	if !strings.Contains(string(data), "github.com") {
		t.Errorf("known_hosts 应含 github.com：%s", data)
	}
	// 再次：同一密钥，校验通过。
	if err := cb("github.com:22", remote, pub1); err != nil {
		t.Fatalf("已知主机应校验通过：%v", err)
	}
	// 密钥变更：拒绝。
	if err := cb("github.com:22", remote, pub2); err == nil {
		t.Error("主机密钥变更应拒绝")
	}
}

func TestSSHHostKeyCallbackRespectsEnv(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	custom := filepath.Join(home, "my_known_hosts")
	t.Setenv("SSH_KNOWN_HOSTS", custom)
	cb := sshHostKeyCallback()
	remote := &net.TCPAddr{IP: net.ParseIP("10.0.0.1"), Port: 2222}
	if err := cb("git.example.com:2222", remote, mustTestHostKey(t)); err != nil {
		t.Fatalf("未知主机应接受：%v", err)
	}
	if _, err := os.Stat(custom); err != nil {
		t.Errorf("应记入 $SSH_KNOWN_HOSTS 指定的文件：%v", err)
	}
}

func TestClassifyGitError(t *testing.T) {
	src := &Source{GitURL: "git@github.com:o/r.git", Ref: "main", Kind: SourceKindSSH}
	cases := []struct {
		err  string
		kind ErrorKind
	}{
		{"ssh: handshake failed: read: connection reset by peer", ErrKindNetwork},
		{"dial tcp: connection refused", ErrKindNetwork},
		{"authentication failed", ErrKindAuth},
		{"Permission denied (publickey)", ErrKindAuth},
		{"repository not found", ErrKindNotFound},
		{"could not resolve host: github.com", ErrKindNetwork},
		{"something completely different", ErrKindUnknown},
	}
	for _, c := range cases {
		got := classifyGitError(errors.New(c.err), "clone", src)
		if got.Kind != c.kind {
			t.Errorf("classifyGitError(%q) = %q，想得到 %q", c.err, got.Kind, c.kind)
		}
	}
	// 非 clone 阶段的引用错误。
	got := classifyGitError(errors.New("reference not found"), "checkout", src)
	if got.Kind != ErrKindRefNotFound {
		t.Errorf("引用错误应为 ref-not-found，得到 %q", got.Kind)
	}
	// 本地路径不是仓库。
	local := &Source{GitURL: "/tmp/nope", Kind: SourceKindLocal}
	got = classifyGitError(errors.New("repository does not exist"), "clone", local)
	if got.Kind != ErrKindNotFound || !strings.Contains(got.Error(), "不是有效的 git 仓库") {
		t.Errorf("本地非仓库错误不对：%v", got)
	}
}
