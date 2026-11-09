package registry

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// indexFetchTimeout 是拉取 index.json 的超时，对应上游 TIMEOUTS.INDEX_FETCH_MS。
const indexFetchTimeout = 5 * time.Second

// SpecTemplate 是 marketplace index.json 里的一条模板，对应上游 SpecTemplate。
type SpecTemplate struct {
	ID          string   `json:"id"`
	Type        string   `json:"type"`
	Name        string   `json:"name"`
	Description string   `json:"description,omitempty"`
	Path        string   `json:"path"`
	Tags        []string `json:"tags,omitempty"`
}

// TemplateIndex 是 marketplace index.json 的结构（version 1）。
type TemplateIndex struct {
	Version   int            `json:"version"`
	Templates []SpecTemplate `json:"templates"`
}

// InstallPaths 是模板类型到安装目录的映射，对应上游 INSTALL_PATHS
// （Go 版把 .trellis 换成 .sunshine）。
//
//	spec → .sunshine/spec
//	skill → .agents/skills
//	command → .claude/commands
//	full → 项目根
var InstallPaths = map[string]string{
	"spec":    ".sunshine/spec",
	"skill":   ".agents/skills",
	"command": ".claude/commands",
	"full":    ".",
}

// InstallPath 返回模板类型对应的安装目录（项目根下的相对路径）。
// 未知类型默认按 spec 处理（与上游 getInstallPath 一致）。
func InstallPath(templateType string) string {
	if p, ok := InstallPaths[templateType]; ok {
		return p
	}
	return InstallPaths["spec"]
}

// ParseTemplateIndex 严格解析 index.json 内容：要求 templates 数组，
// 每条含字符串 id、type、name、path。对应上游 parseTemplateIndex。
func ParseTemplateIndex(raw []byte, sourceLabel string) (*TemplateIndex, error) {
	var parsed struct {
		Version   int `json:"version"`
		Templates []struct {
			ID          string   `json:"id"`
			Type        string   `json:"type"`
			Name        string   `json:"name"`
			Description string   `json:"description"`
			Path        string   `json:"path"`
			Tags        []string `json:"tags"`
		} `json:"templates"`
	}
	dec := json.NewDecoder(strings.NewReader(string(raw)))
	if err := dec.Decode(&parsed); err != nil {
		return nil, NewError(ErrKindInvalidJSON, "%s 不是合法 JSON：%v", sourceLabel, err)
	}
	if parsed.Templates == nil {
		return nil, NewError(ErrKindInvalidJSON, "%s 必须包含 templates 数组", sourceLabel)
	}
	index := &TemplateIndex{Version: parsed.Version}
	if index.Version == 0 {
		index.Version = 1
	}
	for _, item := range parsed.Templates {
		if item.ID == "" || item.Type == "" || item.Name == "" || item.Path == "" {
			return nil, NewError(ErrKindInvalidJSON, "%s 的模板条目必须包含 id、type、name、path 字段", sourceLabel)
		}
		index.Templates = append(index.Templates, SpecTemplate{
			ID:          item.ID,
			Type:        item.Type,
			Name:        item.Name,
			Description: item.Description,
			Path:        item.Path,
			Tags:        item.Tags,
		})
	}
	return index, nil
}

// ProbeResult 是 registry 探测 index.json 的结果，
// 对应上游 RegistryProbeResult。
type ProbeResult struct {
	// Templates 是 index.json 里的模板列表（无 index.json 时为空）。
	Templates []SpecTemplate
	// IsNotFound 为真表示确认无 index.json（404 / 克隆内无该文件）。
	IsNotFound bool
	// Backend 是实际走的后端。
	Backend Backend
	// Err 是非 404 的失败原因（网络、认证、JSON 非法等），成功时为 nil。
	Err *Error

	// checkout 是 git 后端探测时保留的临时克隆，供后续下载复用；
	// 不再需要时必须调用 Close 释放。
	checkout *gitCheckout
}

// Close 释放探测保留的临时克隆目录（HTTP 探测无内容可释放）。
func (p *ProbeResult) Close() {
	if p == nil {
		return
	}
	if p.checkout != nil {
		p.checkout.Close()
		p.checkout = nil
	}
}

// probeIndexHTTP 用 net/http 拉取 index.json。
func probeIndexHTTP(indexURL string) *ProbeResult {
	ctx, cancel := context.WithTimeout(context.Background(), indexFetchTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, indexURL, nil)
	if err != nil {
		return &ProbeResult{Backend: BackendHTTP, Err: NewError(ErrKindUnknown, "构造 index 请求失败：%v", err)}
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return &ProbeResult{Backend: BackendHTTP, Err: NewError(ErrKindNetwork, "无法访问 registry index：%v", err)}
	}
	defer resp.Body.Close()
	switch resp.StatusCode {
	case http.StatusNotFound:
		return &ProbeResult{Backend: BackendHTTP, IsNotFound: true}
	case http.StatusUnauthorized, http.StatusForbidden:
		return &ProbeResult{Backend: BackendHTTP, Err: NewError(ErrKindAuth,
			"registry index 需要认证（HTTP %d），请改用本地 Git 凭据可访问的 registry 来源", resp.StatusCode)}
	}
	if resp.StatusCode != http.StatusOK {
		return &ProbeResult{Backend: BackendHTTP, Err: NewError(ErrKindNetwork,
			"无法访问 registry index（HTTP %d），请检查网络后重试", resp.StatusCode)}
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 10<<20))
	if err != nil {
		return &ProbeResult{Backend: BackendHTTP, Err: NewError(ErrKindNetwork, "读取 registry index 失败：%v", err)}
	}
	index, err := ParseTemplateIndex(body, "registry index.json")
	if err != nil {
		return &ProbeResult{Backend: BackendHTTP, Err: AsRegistryError(err)}
	}
	return &ProbeResult{Backend: BackendHTTP, Templates: index.Templates}
}

// probeIndexGit 用 go-git 克隆后本地读 index.json。
func probeIndexGit(ctx context.Context, src *Source) *ProbeResult {
	checkout, err := cloneRegistry(ctx, src)
	if err != nil {
		return &ProbeResult{Backend: BackendGit, Err: AsRegistryError(err)}
	}
	root, err := registryRoot(checkout.dir, src)
	if err != nil {
		checkout.Close()
		return &ProbeResult{Backend: BackendGit, Err: AsRegistryError(err)}
	}
	data, err := os.ReadFile(filepath.Join(root, "index.json"))
	if err != nil {
		checkout.Close()
		if os.IsNotExist(err) {
			return &ProbeResult{Backend: BackendGit, IsNotFound: true}
		}
		return &ProbeResult{Backend: BackendGit, Err: NewError(ErrKindUnknown, "读取 registry index.json 失败：%v", err)}
	}
	index, err := ParseTemplateIndex(data, "registry index.json")
	if err != nil {
		checkout.Close()
		return &ProbeResult{Backend: BackendGit, Err: AsRegistryError(err)}
	}
	// 保留克隆供下载复用；调用方用完需 Close。
	return &ProbeResult{Backend: BackendGit, Templates: index.Templates, checkout: checkout}
}

// shouldFallbackToGit 对应上游 shouldFallbackToGit：
// 仅 gitlab 来源、HTTP 探测失败且非 404 时（认证或 JSON 非法）才回退 git。
func shouldFallbackToGit(src *Source, result *ProbeResult) bool {
	if src == nil || src.Provider != "gitlab" {
		return false
	}
	if result.Backend != BackendHTTP || result.IsNotFound {
		return false
	}
	return result.Err != nil && (result.Err.Kind == ErrKindAuth || result.Err.Kind == ErrKindInvalidJSON)
}

// ProbeRegistryIndex 探测 registry 的 index.json，区分“不存在”与“瞬时失败”，
// 对应上游 probeRegistryIndex。
//
//   - 404 / 克隆内无 index.json → {IsNotFound: true}
//   - 其他 HTTP 错误 / 网络超时 / JSON 非法 → {Err: ...}
//   - 200 + 合法 JSON → {Templates: [...]}
//
// preferGit 的来源（SSH、自建实例、本地路径）直接走 git 克隆探测；
// 其余先走 HTTP，gitlab 来源在认证/JSON 失败时回退 git。
// git 探测成功时结果保留临时克隆，调用方必须在用完后调用 Close。
func ProbeRegistryIndex(ctx context.Context, src *Source) *ProbeResult {
	if src.PreferGit {
		return probeIndexGit(ctx, src)
	}
	httpResult := probeIndexHTTP(src.RawBaseURL + "/index.json")
	if shouldFallbackToGit(src, httpResult) {
		return probeIndexGit(ctx, src)
	}
	return httpResult
}

// FindTemplate 按 id 在模板列表里查找，找不到返回 nil。
func FindTemplate(templates []SpecTemplate, id string) *SpecTemplate {
	for i := range templates {
		if templates[i].ID == id {
			return &templates[i]
		}
	}
	return nil
}

// TemplateIDs 返回模板 id 列表（用于报错提示）。
func TemplateIDs(templates []SpecTemplate) []string {
	ids := make([]string, 0, len(templates))
	for _, t := range templates {
		ids = append(ids, t.ID)
	}
	return ids
}
