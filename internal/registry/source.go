// Package registry 实现模板市场与 --registry：解析 registry 来源、
// 探测 index.json、下载模板（marketplace 模式与直接下载模式），
// 以及供 sunshine update 调用的 registry.spec 持续刷新接口。
//
// 对照上游 Trellis（0.6.17，commit f089cb3）：逻辑来自
// packages/cli/src/utils/template-fetcher.ts（1396 行）、
// packages/cli/src/utils/registry-config.ts 与
// packages/cli/src/commands/init.ts 的 --registry 部分。
// 需求清单出处称 packages/cli/src/utils/registry.ts，实查该文件
// 不存在，以实查为准。
//
// 与上游的差异（Go 版）：
//   - 下载不用 Node 的 giget，改用 go-git（纯 Go，单二进制无外部依赖）
//     做 git clone（浅克隆 + 稀疏检出）；index.json 探测用 net/http。
//   - 上游 downloadTemplateById 只支持 spec 类型（MVP）；Go 版按
//     INSTALL_PATHS 映射同时支持 spec / skill / command / full，
//     workflow 类型仍拒绝，提示用 sunshine workflow --template 安装。
//   - 直接下载模式：上游把 registry 根原样装进 spec 目录；Go 版若
//     registry 根下存在 marketplace/specs/，则以该目录为模板
//     （对应官方模板仓库结构 marketplace/index.json +
//     marketplace/specs/），否则装 registry 根。
//   - 本地路径（已存在的目录或 file:// URL）作为补充来源：按本地
//     git 仓库处理，上游未提供该用法。
package registry

import (
	"errors"
	"fmt"
)

// Backend 是 registry 访问的后端：http（匿名拉取 index.json）
// 或 git（go-git 克隆）。
type Backend string

const (
	// BackendHTTP 用 net/http 探测 index.json。
	BackendHTTP Backend = "http"
	// BackendGit 用 go-git 克隆整个 registry 后本地读取。
	BackendGit Backend = "git"
)

// ErrorKind 是 registry 错误的分类，对应上游 RegistryErrorKind。
type ErrorKind string

const (
	ErrKindAuth           ErrorKind = "auth"
	ErrKindGitUnavailable ErrorKind = "git-unavailable"
	ErrKindInvalidJSON    ErrorKind = "invalid-json"
	ErrKindNetwork        ErrorKind = "network"
	ErrKindNotFound       ErrorKind = "not-found"
	ErrKindPathNotFound   ErrorKind = "path-not-found"
	ErrKindRefNotFound    ErrorKind = "ref-not-found"
	ErrKindUnknown        ErrorKind = "unknown"
)

// Error 是带分类的 registry 错误。
type Error struct {
	Kind ErrorKind
	msg  string
}

// NewError 构造一个 registry 错误。
func NewError(kind ErrorKind, format string, args ...any) *Error {
	return &Error{Kind: kind, msg: fmt.Sprintf(format, args...)}
}

func (e *Error) Error() string { return e.msg }

// AsRegistryError 从 err 链里取出 *Error（没有则返回 nil）。
func AsRegistryError(err error) *Error {
	var re *Error
	if errors.As(err, &re) {
		return re
	}
	return nil
}

// SourceKind 是 registry 来源的输入形态，对应上游 RegistrySourceKind
// （Go 版补充 local：本地 git 仓库路径）。
type SourceKind string

const (
	// SourceKindPrefixed 是 gh:owner/repo 这类带前缀的写法。
	SourceKindPrefixed SourceKind = "prefixed"
	// SourceKindHTTPS 是 https:// 形式的 URL。
	SourceKindHTTPS SourceKind = "https"
	// SourceKindSSH 是 git@host:owner/repo 或 ssh:// 形式。
	SourceKindSSH SourceKind = "ssh"
	// SourceKindLocal 是本地 git 仓库路径（或 file:// URL），Go 版补充。
	SourceKindLocal SourceKind = "local"
)

// Source 是解析后的 registry 来源，对应上游 RegistrySource。
type Source struct {
	// Provider 是原始前缀，如 gh、gitlab、bitbucket；本地来源为 local。
	Provider string
	// Repo 是仓库路径，如 myorg/myrepo；本地来源为目录名。
	Repo string
	// Subdir 是仓库内的子目录，如 marketplace 或 specs/my-template。
	Subdir string
	// Ref 是 git 分支 / 标签 / 提交，默认 main；本地来源为空表示默认分支。
	Ref string
	// RawBaseURL 是拉取原始文件的基础 URL，如 index.json 的目录；
	// 本地来源为空。
	RawBaseURL string
	// GigetSource 是规范化后的来源字符串（上游 giget 格式），用于展示与持久化。
	GigetSource string
	// Host 是自建实例的主机名（如 git.company.com），公开服务商为空。
	Host string
	// GitURL 是 go-git 克隆用的远端地址。
	GitURL string
	// PreferGit 为真时优先走 git 克隆而不是匿名 HTTP（SSH 来源或自建实例）。
	PreferGit bool
	// Kind 是输入形态。
	Kind SourceKind
	// Original 是用户原始输入，持久化 registry.spec.source 时用它
	//（与上游 registrySourceForConfig 一致，便于 update 时原样重解析）。
	Original string
}
