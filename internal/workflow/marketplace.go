package workflow

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// 超时与上游 template-fetcher.ts 的 TIMEOUTS 一致：
// index 拉取 5 秒，模板文件下载 30 秒。
const (
	indexFetchTimeout = 5 * time.Second
	downloadTimeout   = 30 * time.Second
)

// indexEntry 是模板市场 index.json 里的一条模板记录。
type indexEntry struct {
	ID          string `json:"id"`
	Type        string `json:"type"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Path        string `json:"path"`
}

type templateIndex struct {
	Version   int          `json:"version"`
	Templates []indexEntry `json:"templates"`
}

// fetchMarketplaceIndex 拉取指定来源的 index.json。
// 返回条目列表；失败时返回中文错误说明（调用方把说明当警告展示，
// 仍可用内置模板继续——与上游 listWorkflowTemplates 的降级一致）。
func fetchMarketplaceIndex(source string) ([]indexEntry, string) {
	registry, err := ParseRegistrySource(source)
	if err != nil {
		return nil, err.Error()
	}
	if registry.PreferGit {
		idx, err := probeIndexGit(registry)
		if err != nil {
			return nil, err.Error()
		}
		return idx.Templates, ""
	}
	idx, errMsg, notFound := probeIndexHTTP(registry.RawBaseURL + "/index.json")
	if notFound {
		return nil, "该来源没有 index.json，工作流模板需要 index.json"
	}
	if errMsg != "" {
		return nil, errMsg
	}
	return idx.Templates, ""
}

// probeIndexHTTP 用 HTTP 取 index.json。notFound 表示 404。
func probeIndexHTTP(url string) (*templateIndex, string, bool) {
	ctx, cancel := context.WithTimeout(context.Background(), indexFetchTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Sprintf("无法拉取模板市场索引：%s", err), false
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Sprintf("无法连接模板市场（%s）：%s", url, err), false
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound {
		return nil, "", true
	}
	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		return nil, fmt.Sprintf("模板市场索引需要认证（HTTP %d），请用本地 Git 凭证可访问的市场来源", resp.StatusCode), false
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Sprintf("拉取模板市场索引失败（HTTP %d）：%s", resp.StatusCode, url), false
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 10<<20))
	if err != nil {
		return nil, fmt.Sprintf("读取模板市场索引失败：%s", err), false
	}
	var idx templateIndex
	if err := json.Unmarshal(body, &idx); err != nil {
		return nil, fmt.Sprintf("模板市场索引不是有效的 JSON：%s", err), false
	}
	return &idx, "", false
}

// probeIndexGit 用浅克隆读 index.json（SSH / 自托管来源）。
func probeIndexGit(registry RegistrySource) (*templateIndex, error) {
	dir, cleanup, err := cloneRegistry(registry)
	if err != nil {
		return nil, err
	}
	defer cleanup()
	data, err := readRegistryFile(dir, registry.Subdir, "index.json")
	if err != nil {
		return nil, err
	}
	var idx templateIndex
	if err := json.Unmarshal(data, &idx); err != nil {
		return nil, fmt.Errorf("模板市场索引不是有效的 JSON：%s", err)
	}
	return &idx, nil
}

// cloneRegistry 按上游 fetchWorkflowFileGit 的做法浅克隆市场仓库
// （--filter=blob:none --no-checkout，取 ref 后 detach），返回临时目录。
func cloneRegistry(registry RegistrySource) (string, func(), error) {
	dir, err := os.MkdirTemp("", "sunshine-workflow-")
	if err != nil {
		return "", nil, fmt.Errorf("创建临时目录失败：%w", err)
	}
	cleanup := func() { _ = os.RemoveAll(dir) }
	ctx, cancel := context.WithTimeout(context.Background(), downloadTimeout)
	defer cancel()
	run := func(args ...string) error {
		cmd := exec.CommandContext(ctx, "git", args...)
		cmd.Stdout = nil
		var stderr strings.Builder
		cmd.Stderr = &stderr
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("%s：%s", err, strings.TrimSpace(stderr.String()))
		}
		return nil
	}
	if err := run("clone", "--filter=blob:none", "--no-checkout", registry.GitURL, dir); err != nil {
		cleanup()
		return "", nil, fmt.Errorf("无法克隆市场仓库 %s#%s：%w", registry.GitURL, registry.Ref, err)
	}
	if err := run("-C", dir, "fetch", "--depth", "1", "origin", registry.Ref); err != nil {
		cleanup()
		return "", nil, fmt.Errorf("无法拉取市场分支 %q：%w", registry.Ref, err)
	}
	if err := run("-C", dir, "checkout", "--detach", "FETCH_HEAD"); err != nil {
		cleanup()
		return "", nil, fmt.Errorf("无法检出市场分支 %q：%w", registry.Ref, err)
	}
	return dir, cleanup, nil
}

// readRegistryFile 读克隆出的仓库里 <subdir>/<relPath> 的文件，
// 并校验路径不逃出市场根目录。
func readRegistryFile(cloneDir, subdir, relPath string) ([]byte, error) {
	if err := validateWorkflowPath(relPath); err != nil {
		return nil, err
	}
	root := cloneDir
	if subdir != "" {
		root = filepath.Join(cloneDir, filepath.FromSlash(subdir))
	}
	candidate := filepath.Join(root, filepath.FromSlash(relPath))
	rel, err := filepath.Rel(root, candidate)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || filepath.IsAbs(rel) {
		return nil, fmt.Errorf("工作流模板路径 %q 逃出市场根目录", relPath)
	}
	data, err := os.ReadFile(candidate)
	if err != nil {
		return nil, fmt.Errorf("市场里没有文件 %q", relPath)
	}
	return data, nil
}

// validateWorkflowPath 校验模板路径留在市场根目录内（与上游一致）。
func validateWorkflowPath(relPath string) error {
	normalized := strings.ReplaceAll(relPath, "\\", "/")
	if strings.HasPrefix(normalized, "/") {
		return fmt.Errorf("工作流模板路径 %q 必须留在市场根目录内", relPath)
	}
	for _, part := range strings.Split(normalized, "/") {
		if part == ".." {
			return fmt.Errorf("工作流模板路径 %q 必须留在市场根目录内", relPath)
		}
	}
	return nil
}

// fetchWorkflowFileHTTP 用 HTTP 直链取单个模板文件。
func fetchWorkflowFileHTTP(rawBaseURL, relPath string) (string, error) {
	if err := validateWorkflowPath(relPath); err != nil {
		return "", err
	}
	url := strings.TrimSuffix(rawBaseURL, "/") + "/" + relPath
	ctx, cancel := context.WithTimeout(context.Background(), downloadTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", fmt.Errorf("下载工作流模板失败：%s", err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("下载工作流模板失败：%s", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound {
		return "", fmt.Errorf("工作流模板文件不存在：%s", url)
	}
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("下载工作流模板失败（HTTP %d）：%s", resp.StatusCode, url)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 10<<20))
	if err != nil {
		return "", fmt.Errorf("读取工作流模板失败：%s", err)
	}
	return string(body), nil
}

// resolveFromEntries 在已拉取的 index 条目里找指定工作流模板，
// 校验 path 后用 fetchFile 取单个 .md 文件（与传输层解耦，便于测试）。
func resolveFromEntries(id string, entries []indexEntry, fetchFile func(string) (string, error)) (Resolved, error) {
	var entry *indexEntry
	for i := range entries {
		if entries[i].ID == id && entries[i].Type == "workflow" {
			entry = &entries[i]
			break
		}
	}
	if entry == nil {
		var available []string
		for _, e := range entries {
			if e.Type == "workflow" {
				available = append(available, e.ID)
			}
		}
		hint := ""
		if len(available) > 0 {
			hint = "，可用工作流模板：" + strings.Join(available, "、")
		}
		return Resolved{}, fmt.Errorf("市场索引里没有工作流模板 %q%s", id, hint)
	}
	if !strings.HasSuffix(entry.Path, ".md") {
		return Resolved{}, fmt.Errorf("工作流模板 %q 的路径 %q 无效：必须指向 workflow.md 文件", id, entry.Path)
	}
	if err := validateWorkflowPath(entry.Path); err != nil {
		return Resolved{}, err
	}
	content, err := fetchFile(entry.Path)
	if err != nil {
		return Resolved{}, err
	}
	return Resolved{
		Template: Template{
			ID:          entry.ID,
			Name:        entry.Name,
			Description: entry.Description,
			Path:        entry.Path,
			Source:      SourceMarketplace,
		},
		Content: content,
	}, nil
}

// ResolveFromMarketplace 从指定市场解析模板 id（对应上游
// resolveWorkflowTemplate 的非 native 分支）：先拉 index.json 找
// type=workflow 的条目，再取其 path 指向的单个 .md 文件。
func ResolveFromMarketplace(id, source string) (Resolved, error) {
	registry, err := ParseRegistrySource(source)
	if err != nil {
		return Resolved{}, fmt.Errorf("无效的工作流市场来源 %q：%s", source, err)
	}
	var entries []indexEntry
	if registry.PreferGit {
		idx, err := probeIndexGit(registry)
		if err != nil {
			return Resolved{}, fmt.Errorf("无法拉取工作流模板索引：%s", err)
		}
		entries = idx.Templates
	} else {
		idx, errMsg, notFound := probeIndexHTTP(registry.RawBaseURL + "/index.json")
		if notFound {
			return Resolved{}, fmt.Errorf("无法拉取工作流模板索引：该来源没有 index.json，工作流模板需要 index.json")
		}
		if errMsg != "" {
			return Resolved{}, fmt.Errorf("无法拉取工作流模板索引：%s", errMsg)
		}
		entries = idx.Templates
	}
	if registry.PreferGit {
		dir, cleanup, err := cloneRegistry(registry)
		if err != nil {
			return Resolved{}, fmt.Errorf("下载工作流模板失败：%s", err)
		}
		defer cleanup()
		return resolveFromEntries(id, entries, func(path string) (string, error) {
			data, err := readRegistryFile(dir, registry.Subdir, path)
			if err != nil {
				return "", fmt.Errorf("下载工作流模板失败：%s", err)
			}
			return string(data), nil
		})
	}
	return resolveFromEntries(id, entries, func(path string) (string, error) {
		return fetchWorkflowFileHTTP(registry.RawBaseURL, path)
	})
}
