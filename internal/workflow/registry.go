package workflow

import (
	"fmt"
	"regexp"
	"strings"
)

// RegistrySource 是解析后的模板市场来源，对应 Trellis
// template-fetcher.ts 的 parseRegistrySource。
//
// 支持的来源形式（与上游一致）：
//   - giget 前缀：gh:org/repo、github:org/repo、gitlab:org/repo、bitbucket:org/repo，
//     可带 /子目录 与 #分支（默认 main）；
//   - 公开 HTTPS：https://github.com/org/repo、…/tree/<分支>/<子目录>；
//   - 公开 SSH：git@github.com:org/repo、ssh://git@host[:端口]/org/repo；
//   - 自托管 HTTPS/SSH：https://git.corp.com/org/repo、git@git.corp.com:org/repo、
//     GitLab 浏览地址 https://host/org/repo/-/tree/<分支>/<路径>、带端口的 HTTPS。
//
// 不支持本地路径：上游 parseRegistrySource 同样不支持（无 file:// 或
// 本地目录分支），传本地路径会报“无效的市场来源”。
type RegistrySource struct {
	Provider   string // gh | github | gitlab | bitbucket
	Repo       string // org/repo
	Subdir     string // 仓库内子目录（可空）
	Ref        string // Git 分支/标签（默认 main）
	RawBaseURL string // index.json 与模板文件的 HTTP 直链根地址
	GitURL     string // git clone 用地址
	PreferGit  bool   // SSH 来源或自托管 host 时为真：走 git 而非 HTTP
	Host       string // 自托管 host（公开域名时为空）
	Kind       string // prefixed | https | ssh
}

// rawURLPatterns 是各供应商的 HTTP 直链模板（对应上游 RAW_URL_PATTERNS）。
var rawURLPatterns = map[string]string{
	"gh":        "https://raw.githubusercontent.com/{repo}/{ref}/{subdir}",
	"github":    "https://raw.githubusercontent.com/{repo}/{ref}/{subdir}",
	"gitlab":    "https://gitlab.com/{repo}/-/raw/{ref}/{subdir}",
	"bitbucket": "https://bitbucket.org/{repo}/raw/{ref}/{subdir}",
}

// SupportedProviders 是支持的 giget 前缀。
func SupportedProviders() []string {
	return []string{"gh", "github", "gitlab", "bitbucket"}
}

var (
	scpSSHRe      = regexp.MustCompile(`^git@([^:]+):(.+?)(?:\.git)?/?$`)
	protocolSSHRe = regexp.MustCompile(`^ssh://git@([^/:]+)(?::(\d+))?/(.+?)(?:\.git)?/?$`)
	httpsRe       = regexp.MustCompile(`^https?://([^/]+)/(.+?)(?:\.git)?/?$`)
	gitlabTreeRe  = regexp.MustCompile(`^([^/]+/[^/]+)(?:/-)?/tree/([^/]+)(?:/(.+?))?$`)
	githubTreeRe  = regexp.MustCompile(`^([^/]+/[^/]+)/tree/([^/]+)(?:/(.+?))?$`)
)

var knownPublicDomains = map[string]bool{
	"github.com":    true,
	"gitlab.com":    true,
	"bitbucket.org": true,
}

var publicDomainToPrefix = map[string]string{
	"github.com":    "gh",
	"gitlab.com":    "gitlab",
	"bitbucket.org": "bitbucket",
}

// normalizeRegistrySource 把公开 HTTPS 地址转成 giget 形式
// （例如 https://github.com/user/repo → gh:user/repo，
// https://github.com/user/repo/tree/branch/path → gh:user/repo/path#branch）。
// 非公开 HTTPS 地址原样返回。
func normalizeRegistrySource(source string) string {
	for _, rule := range []struct {
		re     *regexp.Regexp
		prefix string
	}{
		{regexp.MustCompile(`^https?://github\.com/`), "gh:"},
		{regexp.MustCompile(`^https?://gitlab\.com/`), "gitlab:"},
		{regexp.MustCompile(`^https?://bitbucket\.org/`), "bitbucket:"},
	} {
		if !rule.re.MatchString(source) {
			continue
		}
		rest := rule.re.ReplaceAllString(source, "")
		rest = strings.TrimSuffix(rest, ".git")
		rest = strings.TrimSuffix(rest, "/")
		if m := githubTreeRe.FindStringSubmatch(rest); m != nil {
			out := rule.prefix + m[1]
			if m[3] != "" {
				out += "/" + m[3]
			}
			return out + "#" + m[2]
		}
		return rule.prefix + rest
	}
	return source
}

func buildRegistryGitURL(provider, host, repo, kind, sshHost, sshPort, sshStyle string) string {
	if kind == "ssh" {
		if sshStyle == "ssh-url" {
			port := ""
			if sshPort != "" {
				port = ":" + sshPort
			}
			return "ssh://git@" + sshHost + port + "/" + repo + ".git"
		}
		return "git@" + sshHost + ":" + repo + ".git"
	}
	if host != "" {
		return "https://" + host + "/" + repo + ".git"
	}
	switch provider {
	case "gh", "github":
		return "https://github.com/" + repo + ".git"
	case "gitlab":
		return "https://gitlab.com/" + repo + ".git"
	case "bitbucket":
		return "https://bitbucket.org/" + repo + ".git"
	default:
		return repo
	}
}

// ParseRegistrySource 解析 -m/--marketplace 的来源字符串。
// 传本地路径（./x、/abs/path、~/x 等）时报错：上游不支持本地路径。
func ParseRegistrySource(source string) (RegistrySource, error) {
	if strings.HasPrefix(source, "./") || strings.HasPrefix(source, "../") ||
		strings.HasPrefix(source, "/") || strings.HasPrefix(source, "~/") ||
		strings.HasPrefix(source, "file://") {
		return RegistrySource{}, fmt.Errorf("无效的市场来源 %q：本地路径不是有效的模板市场来源，请用 gh:user/repo 或 https://github.com/user/repo 这类 Git 仓库地址", source)
	}

	var host string
	var normalizedInput string
	kind := "prefixed"
	var sshHost, sshPort, sshStyle string

	// SSH：git@host:org/repo[.git] 或 ssh://git@host[:port]/org/repo[.git]
	if m := scpSSHRe.FindStringSubmatch(source); m != nil {
		kind = "ssh"
		sshStyle = "scp"
		sshHost = m[1]
		sshPath := m[2]
		if prefix, ok := publicDomainToPrefix[sshHost]; ok {
			normalizedInput = prefix + ":" + sshPath
		} else {
			host = sshHost
			normalizedInput = "gitlab:" + sshPath
		}
	} else if m := protocolSSHRe.FindStringSubmatch(source); m != nil {
		kind = "ssh"
		sshStyle = "ssh-url"
		sshHost = m[1]
		sshPort = m[2]
		sshPath := m[3]
		if prefix, ok := publicDomainToPrefix[sshHost]; ok {
			normalizedInput = prefix + ":" + sshPath
		} else {
			host = sshHost
			normalizedInput = "gitlab:" + sshPath
		}
	}

	// 未知域名的 HTTPS 地址（非 github.com/gitlab.com/bitbucket.org）
	if normalizedInput == "" {
		if m := httpsRe.FindStringSubmatch(source); m != nil {
			kind = "https"
			domain := m[1]
			if !knownPublicDomains[domain] {
				host = domain
				pathPart := m[2]
				if tm := gitlabTreeRe.FindStringSubmatch(pathPart); tm != nil {
					normalizedInput = "gitlab:" + tm[1]
					if tm[3] != "" {
						normalizedInput += "/" + tm[3]
					}
					normalizedInput += "#" + tm[2]
				} else {
					normalizedInput = "gitlab:" + pathPart
				}
			}
		}
	}

	normalized := normalizedInput
	if normalized == "" {
		normalized = normalizeRegistrySource(source)
	}

	colon := strings.Index(normalized, ":")
	if colon == -1 {
		return RegistrySource{}, fmt.Errorf("无效的市场来源 %q：期望格式为 gh:user/repo/path", source)
	}
	provider := normalized[:colon]
	rest := normalized[colon+1:]

	pattern, ok := rawURLPatterns[provider]
	if !ok {
		return RegistrySource{}, fmt.Errorf("不支持的供应商 %q，支持：%s", provider, strings.Join(SupportedProviders(), "、"))
	}

	// 解析 rest：user/repo/subdir#ref
	ref := "main"
	pathPart := rest
	if i := strings.Index(rest, "#"); i != -1 {
		pathPart = rest[:i]
		ref = rest[i+1:]
	}
	segments := []string{}
	for _, seg := range strings.Split(pathPart, "/") {
		if seg != "" {
			segments = append(segments, seg)
		}
	}
	if len(segments) < 2 {
		return RegistrySource{}, fmt.Errorf("无效的市场来源 %q：至少需要 user/repo", normalized)
	}
	repo := segments[0] + "/" + segments[1]
	subdir := strings.Join(segments[2:], "/")

	rawBaseURL := strings.ReplaceAll(pattern, "{repo}", repo)
	rawBaseURL = strings.ReplaceAll(rawBaseURL, "{ref}", ref)
	rawBaseURL = strings.ReplaceAll(rawBaseURL, "{subdir}", subdir)
	// 自托管 host 替换 raw 地址里的公开域名。
	if host != "" && provider == "gitlab" {
		rawBaseURL = strings.Replace(rawBaseURL, "https://gitlab.com", "https://"+host, 1)
	}
	// gh 供应商的自托管 host 不适用；上游只在 gitlab 时替换。
	rawBaseURL = strings.TrimSuffix(rawBaseURL, "/")

	gitURL := buildRegistryGitURL(provider, host, repo, kind, sshHost, sshPort, sshStyle)
	preferGit := kind == "ssh" || host != ""

	return RegistrySource{
		Provider:   provider,
		Repo:       repo,
		Subdir:     subdir,
		Ref:        ref,
		RawBaseURL: rawBaseURL,
		GitURL:     gitURL,
		PreferGit:  preferGit,
		Host:       host,
		Kind:       kind,
	}, nil
}
