package registry

import (
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/transport"
	"github.com/go-git/go-git/v5/plumbing/transport/ssh"
	"github.com/skeema/knownhosts"
	cryptossh "golang.org/x/crypto/ssh"
)

// cloneTimeout 是单次克隆的超时，对应上游 TIMEOUTS.DOWNLOAD_MS。
const cloneTimeout = 30 * time.Second

// gitCheckout 是一次 registry 克隆的临时目录，用完需 Close。
type gitCheckout struct {
	dir string
}

// Close 删除临时克隆目录（尽力而为）。
func (c *gitCheckout) Close() {
	if c == nil {
		return
	}
	_ = os.RemoveAll(c.dir)
}

// sshAuth 按“ssh-agent → ~/.ssh 默认私钥”的顺序找 SSH 凭据，
// 找不到时返回 auth 错误（上游 shell 出 git CLI，凭据由用户环境提供）。
func sshAuth() (transport.AuthMethod, error) {
	if auth, err := ssh.NewSSHAgentAuth("git"); err == nil {
		return withHostKeyCallback(auth), nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, NewError(ErrKindAuth, "SSH registry 需要认证，但无法定位用户主目录")
	}
	for _, name := range []string{"id_ed25519", "id_rsa", "id_ecdsa", "id_dsa"} {
		keyPath := filepath.Join(home, ".ssh", name)
		if _, err := os.Stat(keyPath); err != nil {
			continue
		}
		auth, err := ssh.NewPublicKeysFromFile("git", keyPath, "")
		if err != nil {
			// 私钥存在但读不出来（多半是带口令），直接说明，不再试下一个。
			return nil, NewError(ErrKindAuth, "SSH 私钥 %s 无法使用（可能带口令）：%v", keyPath, err)
		}
		return withHostKeyCallback(auth), nil
	}
	return nil, NewError(ErrKindAuth, "SSH registry 需要认证，但未找到可用的 SSH 凭据（ssh-agent 或 ~/.ssh/id_ed25519 等私钥）")
}

// sshKnownHostsPath 返回 known_hosts 文件位置：优先 $SSH_KNOWN_HOSTS，
// 否则 ~/.ssh/known_hosts。
func sshKnownHostsPath() string {
	if p := os.Getenv("SSH_KNOWN_HOSTS"); p != "" {
		return p
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".ssh", "known_hosts")
}

// sshHostKeyCallback 返回主机密钥校验回调（accept-new 语义）：
// 已知主机严格校验（密钥变更拒绝）；未知主机接受并追加记入
// known_hosts，与 git 的 StrictHostKeyChecking=accept-new 一致。
// go-git 默认的回调在没有任何 known_hosts 文件时直接报错
// （cannot create known hosts callback），本回调避免该问题。
func sshHostKeyCallback() cryptossh.HostKeyCallback {
	return func(hostname string, remote net.Addr, key cryptossh.PublicKey) error {
		path := sshKnownHostsPath()
		if path != "" {
			if _, err := os.Stat(path); err == nil {
				if khcb, err := knownhosts.New(path); err == nil {
					if err := khcb.HostKeyCallback()(hostname, remote, key); err != nil {
						if !knownhosts.IsHostUnknown(err) {
							return err
						}
					} else {
						return nil
					}
				}
			}
		}
		// 未知主机：接受并记入 known_hosts。
		if path == "" {
			return fmt.Errorf("无法记录 %s 的主机密钥：找不到用户主目录", hostname)
		}
		if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
			return fmt.Errorf("无法创建 %s：%w", filepath.Dir(path), err)
		}
		f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
		if err != nil {
			return fmt.Errorf("无法写入 %s：%w", path, err)
		}
		defer f.Close()
		if err := knownhosts.WriteKnownHost(f, hostname, remote, key); err != nil {
			return err
		}
		fmt.Printf("提示：已将 %s 的主机密钥记入 %s。\n", hostname, path)
		return nil
	}
}

// withHostKeyCallback 给 SSH 认证方法装上 accept-new 主机密钥回调。
func withHostKeyCallback(auth transport.AuthMethod) transport.AuthMethod {
	cb := sshHostKeyCallback()
	switch a := auth.(type) {
	case *ssh.PublicKeys:
		a.HostKeyCallback = cb
	case *ssh.PublicKeysCallback:
		a.HostKeyCallback = cb
	}
	return auth
}

// classifyGitError 把 go-git / 网络错误映射到 registry 错误分类，
// 对应上游 classifyGitError。
func classifyGitError(err error, stage string, src *Source) *Error {
	if err == nil {
		return nil
	}
	if re := AsRegistryError(err); re != nil {
		return re
	}
	if errors.Is(err, transport.ErrAuthenticationRequired) ||
		errors.Is(err, transport.ErrAuthorizationFailed) {
		return NewError(ErrKindAuth, "registry 认证失败，请检查 %s 的本地 Git 凭据", src.GitURL)
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return NewError(ErrKindNetwork, "访问 registry 超时：%s", src.GitURL)
	}
	text := strings.ToLower(err.Error())
	switch {
	case strings.Contains(text, "repository does not exist"),
		strings.Contains(text, "repository not found"),
		strings.Contains(text, "does not appear to be a git repository"):
		if src.Kind == SourceKindLocal {
			return NewError(ErrKindNotFound, "本地路径 %s 不是有效的 git 仓库", src.GitURL)
		}
		return NewError(ErrKindNotFound, "registry 仓库不存在或无法访问：%s", src.GitURL)
	case strings.Contains(text, "could not resolve host"),
		strings.Contains(text, "no such host"),
		strings.Contains(text, "failed to connect"),
		strings.Contains(text, "network is unreachable"),
		strings.Contains(text, "connection refused"),
		strings.Contains(text, "connection reset"),
		strings.Contains(text, "broken pipe"),
		strings.Contains(text, "timed out"),
		strings.Contains(text, "i/o timeout"):
		// 注意：握手失败（handshake failed）常由连接被重置/拒绝引起，
		// 属于网络问题，不归认证；真正的认证失败另有明确文案，
		// 见下一分支。
		return NewError(ErrKindNetwork, "无法连接 registry %s，请检查网络与 Git 远端访问", src.GitURL)
	case stage != "clone" && (strings.Contains(text, "couldn't find remote ref") ||
		strings.Contains(text, "reference not found") ||
		strings.Contains(text, "unknown revision") ||
		strings.Contains(text, "reference is not a tree") ||
		strings.Contains(text, "invalid reference")):
		return NewError(ErrKindRefNotFound, "registry 分支 %q 在 %s 中不存在", src.Ref, src.GitURL)
	case strings.Contains(text, "authentication failed"),
		strings.Contains(text, "permission denied"),
		strings.Contains(text, "access denied"),
		strings.Contains(text, "could not read from remote repository"),
		strings.Contains(text, "no supported authentication methods"),
		strings.Contains(text, "unable to authenticate"):
		return NewError(ErrKindAuth, "registry 认证失败，请检查 %s 的本地 Git 凭据", src.GitURL)
	default:
		return NewError(ErrKindUnknown, "Git registry 操作失败：%v", err)
	}
}

// sparseCheckoutDirs 把 subdir 转成 go-git 稀疏检出的目录列表。
// subdir 为空时返回 nil（完整检出）。
func sparseCheckoutDirs(subdir string) []string {
	subdir = strings.Trim(strings.TrimSpace(subdir), "/")
	if subdir == "" {
		return nil
	}
	return []string{subdir}
}

// applySparseCheckout 在已克隆的仓库上做稀疏检出（只检出 subdir）。
// 按 HEAD 的提交哈希检出：不能依赖分支名（Validate 会把空 Branch
// 默认成 master，而仓库默认分支可能是 main）。
func applySparseCheckout(repo *git.Repository, subdir string) error {
	dirs := sparseCheckoutDirs(subdir)
	if len(dirs) == 0 {
		return nil
	}
	head, err := repo.Head()
	if err != nil {
		return err
	}
	w, err := repo.Worktree()
	if err != nil {
		return err
	}
	return w.Checkout(&git.CheckoutOptions{Hash: head.Hash(), SparseCheckoutDirectories: dirs})
}

// cloneAttempt 描述一次克隆尝试：ref 形态（分支 / 标签 / 不指定）。
type cloneAttempt struct {
	refName plumbing.ReferenceName
	depth   int
}

// cloneRegistry 用 go-git 克隆 registry 到临时目录。
// 策略：先浅克隆指定分支（depth 1、单分支）；失败则试标签；
// 再失败则完整克隆后按 ref 解析提交检出。subdir 非空时做稀疏检出。
func cloneRegistry(ctx context.Context, src *Source) (*gitCheckout, error) {
	ctx, cancel := context.WithTimeout(ctx, cloneTimeout)
	defer cancel()

	var auth transport.AuthMethod
	if src.Kind == SourceKindSSH {
		var err error
		auth, err = sshAuth()
		if err != nil {
			return nil, err
		}
	}

	dir, err := os.MkdirTemp("", "sunshine-registry-")
	if err != nil {
		return nil, fmt.Errorf("创建临时目录失败：%w", err)
	}
	cleanup := func() { _ = os.RemoveAll(dir) }

	branchRef := plumbing.ReferenceName("")
	tagRef := plumbing.ReferenceName("")
	if src.Ref != "" {
		branchRef = plumbing.NewBranchReferenceName(src.Ref)
		tagRef = plumbing.NewTagReferenceName(src.Ref)
	}

	// 尝试 1：浅克隆指定分支（或默认分支）。
	if repo, err := cloneOnce(ctx, dir, src, auth, cloneAttempt{refName: branchRef, depth: 1}); err == nil {
		if err := applySparseCheckout(repo, src.Subdir); err != nil {
			cleanup()
			return nil, classifyGitError(err, "checkout", src)
		}
		return &gitCheckout{dir: dir}, nil
	} else if re := classifyGitError(err, "clone", src); re.Kind == ErrKindAuth || re.Kind == ErrKindNotFound || re.Kind == ErrKindNetwork {
		// 认证 / 仓库不存在 / 网络问题重试无意义，直接返回。
		cleanup()
		return nil, re
	} else {
		_ = os.RemoveAll(dir)
	}

	// 尝试 2：ref 可能是标签，浅克隆指定标签。
	if tagRef != "" {
		if repo, err := cloneOnce(ctx, dir, src, auth, cloneAttempt{refName: tagRef, depth: 1}); err == nil {
			if err := applySparseCheckout(repo, src.Subdir); err != nil {
				cleanup()
				return nil, classifyGitError(err, "checkout", src)
			}
			return &gitCheckout{dir: dir}, nil
		} else {
			_ = os.RemoveAll(dir)
		}
	}

	// 尝试 3：完整克隆默认分支，再按 ref 解析提交后检出。
	repo, err := cloneOnce(ctx, dir, src, auth, cloneAttempt{depth: 0})
	if err != nil {
		cleanup()
		return nil, classifyGitError(err, "clone", src)
	}
	if src.Ref != "" {
		hash, err := repo.ResolveRevision(plumbing.Revision(src.Ref))
		if err != nil {
			cleanup()
			return nil, NewError(ErrKindRefNotFound, "registry 引用 %q 在 %s 中不存在", src.Ref, src.GitURL)
		}
		w, err := repo.Worktree()
		if err != nil {
			cleanup()
			return nil, classifyGitError(err, "checkout", src)
		}
		if err := w.Checkout(&git.CheckoutOptions{Hash: *hash}); err != nil {
			cleanup()
			return nil, classifyGitError(err, "checkout", src)
		}
	}
	if err := applySparseCheckout(repo, src.Subdir); err != nil {
		cleanup()
		return nil, classifyGitError(err, "checkout", src)
	}
	return &gitCheckout{dir: dir}, nil
}

// cloneOnce 执行一次 go-git 克隆。
func cloneOnce(ctx context.Context, dir string, src *Source, auth transport.AuthMethod, attempt cloneAttempt) (*git.Repository, error) {
	opts := &git.CloneOptions{
		URL:           src.GitURL,
		Auth:          auth,
		Depth:         attempt.depth,
		SingleBranch:  attempt.refName != "",
		ReferenceName: attempt.refName,
		Tags:          git.NoTags,
		NoCheckout:    sparseCheckoutDirs(src.Subdir) != nil,
	}
	if attempt.refName == "" {
		opts.ReferenceName = plumbing.HEAD
	}
	return git.PlainCloneContext(ctx, dir, false, opts)
}

// registryRoot 返回克隆内 registry 根目录（subdir 限定，含越界检查），
// 对应上游 getGitRegistryRoot / resolveInsideRegistryRoot。
func registryRoot(checkoutDir string, src *Source) (string, error) {
	root := filepath.Join(checkoutDir, filepath.FromSlash(src.Subdir))
	absRoot, err := filepath.Abs(root)
	if err != nil {
		return "", err
	}
	absBase, err := filepath.Abs(checkoutDir)
	if err != nil {
		return "", err
	}
	rel, err := filepath.Rel(absBase, absRoot)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", NewError(ErrKindPathNotFound, "registry 路径 %q 超出仓库范围", src.Subdir)
	}
	st, err := os.Stat(absRoot)
	if err != nil || !st.IsDir() {
		display := src.Subdir
		if display == "" {
			display = "."
		}
		return "", NewError(ErrKindPathNotFound, "registry 路径 %q 在 %s#%s 中不存在",
			display, src.GitURL, src.Ref)
	}
	return absRoot, nil
}

// resolveInsideRoot 把 relativePath 解析到 root 内，越界时报错。
func resolveInsideRoot(root, relativePath, label string) (string, error) {
	absRoot, err := filepath.Abs(root)
	if err != nil {
		return "", err
	}
	joined := filepath.Join(absRoot, filepath.FromSlash(relativePath))
	abs, err := filepath.Abs(joined)
	if err != nil {
		return "", err
	}
	rel, err := filepath.Rel(absRoot, abs)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", NewError(ErrKindPathNotFound, "%s 路径 %q 必须在 registry 仓库内", label, relativePath)
	}
	return abs, nil
}
