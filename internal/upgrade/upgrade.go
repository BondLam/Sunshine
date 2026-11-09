// Package upgrade 实现 sunshine upgrade（升级 CLI 自身）：
// 查最新版（GitHub Releases API）→ 下载 → 按 checksums.txt 校验
// sha256 → 备份当前二进制 → 替换 → 运行新版 --version 校验 →
// 失败回滚备份。
//
// 与 sunshine update 的区别：upgrade 升级 CLI 自身，update 更新
// 项目内的模板文件。
//
// GitHub Releases 的交互全部走 Client（仓库地址、API 地址与
// HTTP 客户端均可注入），便于用 httptest 做端到端测试；不要把
// api.github.com 写死在不可替换的位置。
package upgrade

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"time"

	"github.com/linrong1982/Sunshine/internal/update"
)

// PlaceholderRepo 是旧占位仓库地址的哨兵值：仓库变量仍为该值时视为
// 未配置（upgrade 报错说明，update 的最新版检查只警告不中断）。
// 真实仓库已定为 github.com/linrong1982/Sunshine（见 internal/cli/root.go）。
const PlaceholderRepo = "github.com/sunshine/sunshine"

// DefaultAPIBase 是 GitHub Releases API 的默认地址。
const DefaultAPIBase = "https://api.github.com"

// tagRe 是显式 --tag 的校验规则（对应上游 upgrade.ts 的 NPM_TAG_RE）：
// 简单 dist-tag 或版本号，如 latest、beta、rc、0.6.0-beta.8。
var tagRe = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]*$`)

// ErrAlreadyLatest 表示当前已是最新版（不是错误，调用方据此提示退出）。
var ErrAlreadyLatest = errors.New("已是最新版本")

// ResolveTag 解析目标通道（对应上游 resolveUpgradeTag）：
// 显式 --tag 须过校验；不传时按当前版本推断：含 -beta → beta，
// 含 -rc → rc，否则 latest。
func ResolveTag(currentVersion, requestedTag string) (string, error) {
	if requestedTag != "" {
		if !tagRe.MatchString(requestedTag) {
			return "", fmt.Errorf("无效的标签/版本号 %q：请用简单 dist-tag 或版本号，如 latest、beta、rc 或 0.6.0-beta.8", requestedTag)
		}
		return requestedTag, nil
	}
	if strings.Contains(currentVersion, "-beta") {
		return "beta", nil
	}
	if strings.Contains(currentVersion, "-rc") {
		return "rc", nil
	}
	return "latest", nil
}

// BinaryCheckCommand 返回检查 shell 解析到哪个二进制的命令
// （对应上游 binaryCheckCommand；Windows 用 where，Unix 用 which）。
func BinaryCheckCommand(goos string) string {
	if goos == "windows" {
		return "where sunshine"
	}
	return "which sunshine"
}

// Troubleshooting 返回升级失败时的排查文案（对应上游 troubleshooting()）。
// manualURL 是手动下载地址（https://github.com/<owner>/<name>/releases），
// 仓库未配置时为空，此时文案注明暂无下载地址。
func Troubleshooting(manualURL, goos string) string {
	manual := manualURL
	if manual == "" {
		manual = "仓库地址未配置，暂无下载地址"
	}
	return strings.Join([]string{
		"",
		"排查方法：",
		"- 手动下载安装： " + manual,
		"- 检查 shell 解析到哪个 Sunshine 二进制： " + BinaryCheckCommand(goos),
		"- 如果是权限错误，请修复你的安装目录权限；Sunshine 不会用 sudo 升级。",
		"- 如果提示二进制被占用或锁文件已存在，请先关闭正在运行的 sunshine 进程、删除锁文件后重试；Sunshine 不用 --force 强行覆盖。",
	}, "\n")
}

// releaseAsset 是 GitHub Releases API 返回的单个资产。
type releaseAsset struct {
	Name               string `json:"name"`
	BrowserDownloadURL string `json:"browser_download_url"`
}

// release 是 GitHub Releases API 返回的单个发布。
type release struct {
	TagName    string         `json:"tag_name"`
	Draft      bool           `json:"draft"`
	Prerelease bool           `json:"prerelease"`
	Assets     []releaseAsset `json:"assets"`
}

// Client 是 GitHub Releases 客户端：仓库地址、API 地址与 HTTP
// 客户端均可注入（测试用 httptest 替换 API 地址与客户端）。
type Client struct {
	// Repo 是仓库地址，形如 owner/name，也接受 github.com/owner/name
	// 或 https://github.com/owner/name；为空或占位值时视为未配置。
	Repo string
	// APIBase 是 Releases API 的根地址，默认 DefaultAPIBase。
	APIBase string
	// HTTPClient 是发请求的客户端，为 nil 时用带 30 秒超时的默认客户端。
	HTTPClient *http.Client
	// Timeout 是单次请求的超时（仅 HTTPClient 为 nil 时生效）。
	Timeout time.Duration
}

func (c *Client) httpClient() *http.Client {
	if c.HTTPClient != nil {
		return c.HTTPClient
	}
	timeout := c.Timeout
	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	return &http.Client{Timeout: timeout}
}

func (c *Client) apiBase() string {
	if c.APIBase != "" {
		return strings.TrimSuffix(c.APIBase, "/")
	}
	return DefaultAPIBase
}

// ownerName 把 Repo 归一化为 owner/name；未配置时返回错误。
func (c *Client) ownerName() (owner, name string, err error) {
	repo := strings.TrimSpace(c.Repo)
	if repo == "" || repo == PlaceholderRepo {
		return "", "", errors.New("仓库地址未配置（真实仓库待定）：无法检查最新版，请等发布仓库建立后再升级，或手动下载安装")
	}
	repo = strings.TrimPrefix(repo, "https://github.com/")
	repo = strings.TrimPrefix(repo, "http://github.com/")
	repo = strings.TrimPrefix(repo, "github.com/")
	repo = strings.TrimSuffix(repo, "/")
	parts := strings.Split(repo, "/")
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return "", "", fmt.Errorf("仓库地址 %q 格式无效，应为 owner/name（例如 sunshine/sunshine）", c.Repo)
	}
	return parts[0], parts[1], nil
}

// ManualURL 返回手动下载地址（https://github.com/<owner>/<name>/releases）。
// 仓库未配置时返回空字符串。
func (c *Client) ManualURL() string {
	owner, name, err := c.ownerName()
	if err != nil {
		return ""
	}
	return "https://github.com/" + owner + "/" + name + "/releases"
}

func (c *Client) getJSON(ctx context.Context, url string, out any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	resp, err := c.httpClient().Do(req)
	if err != nil {
		return fmt.Errorf("请求 GitHub Releases 失败（可能是离线或网络不通）：%w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound {
		return errReleaseNotFound
	}
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4<<10))
		return fmt.Errorf("GitHub Releases 返回异常状态 %s：%s", resp.Status, strings.TrimSpace(string(body)))
	}
	return json.NewDecoder(resp.Body).Decode(out)
}

var errReleaseNotFound = errors.New("发布不存在")

// ResolveRelease 按通道解析目标发布：
//   - latest：取 /releases/latest（仅正式版）；
//   - beta / rc：取发布列表里第一个含对应 -beta / -rc 后缀的非草稿发布；
//   - 其他（显式版本号）：取 /releases/tags/<tag>（带或不带 v 前缀都试）。
func (c *Client) ResolveRelease(ctx context.Context, tag string) (*release, error) {
	owner, name, err := c.ownerName()
	if err != nil {
		return nil, err
	}
	base := c.apiBase() + "/repos/" + owner + "/" + name
	switch tag {
	case "latest":
		var rel release
		if err := c.getJSON(ctx, base+"/releases/latest", &rel); err != nil {
			return nil, err
		}
		return &rel, nil
	case "beta", "rc":
		var rels []release
		if err := c.getJSON(ctx, base+"/releases?per_page=20", &rels); err != nil {
			return nil, err
		}
		suffix := "-" + tag
		for _, rel := range rels {
			if rel.Draft {
				continue
			}
			if strings.Contains(rel.TagName, suffix) {
				r := rel
				return &r, nil
			}
		}
		return nil, fmt.Errorf("没有找到 %s 通道的发布", tag)
	default:
		candidates := []string{tag}
		if strings.HasPrefix(tag, "v") {
			candidates = append(candidates, strings.TrimPrefix(tag, "v"))
		} else {
			candidates = append(candidates, "v"+tag)
		}
		var lastErr error
		for _, cand := range candidates {
			var rel release
			err := c.getJSON(ctx, base+"/releases/tags/"+url.PathEscape(cand), &rel)
			if err == nil {
				return &rel, nil
			}
			if !errors.Is(err, errReleaseNotFound) {
				return nil, err
			}
			lastErr = err
		}
		return nil, fmt.Errorf("没有找到版本 %q 对应的发布：%w", tag, lastErr)
	}
}

// LatestVersion 返回 latest 通道的最新版本号（去掉 v 前缀，
// 供 update.LatestVersionFunc 与 upgrade 共用）。
func (c *Client) LatestVersion(ctx context.Context) (string, error) {
	rel, err := c.ResolveRelease(ctx, "latest")
	if err != nil {
		return "", err
	}
	return NormalizeVersion(rel.TagName), nil
}

// NormalizeVersion 去掉版本号前的 v 前缀（v0.2.0 → 0.2.0）。
func NormalizeVersion(v string) string {
	return strings.TrimPrefix(strings.TrimSpace(v), "v")
}

// AssetName 返回 goreleaser 默认命名下的资产文件名：
// sunshine_<版本>_<系统>_<架构>.tar.gz（Windows 用 .zip）。
// 版本不带 v 前缀。
func AssetName(version, goos, goarch string) string {
	ext := ".tar.gz"
	if goos == "windows" {
		ext = ".zip"
	}
	return fmt.Sprintf("sunshine_%s_%s_%s%s", NormalizeVersion(version), goos, goarch, ext)
}

// SelectAsset 按当前 GOOS/GOARCH 选资产：先精确匹配 AssetName，
// 找不到时退化为后缀匹配（_<系统>_<架构>.tar.gz / .zip），
// 以兼容发布配置里不同的资产命名。
func SelectAsset(rel *release, version, goos, goarch string) (*releaseAsset, error) {
	if rel == nil {
		return nil, errors.New("发布信息为空（内部错误）")
	}
	want := AssetName(version, goos, goarch)
	for i := range rel.Assets {
		if rel.Assets[i].Name == want {
			return &rel.Assets[i], nil
		}
	}
	ext := ".tar.gz"
	if goos == "windows" {
		ext = ".zip"
	}
	// 两种分隔符都接受：_linux_amd64 与 -linux-amd64。
	for _, suffix := range []string{"_" + goos + "_" + goarch + ext, "-" + goos + "-" + goarch + ext} {
		for i := range rel.Assets {
			if strings.HasSuffix(rel.Assets[i].Name, suffix) {
				return &rel.Assets[i], nil
			}
		}
	}
	var names []string
	for _, a := range rel.Assets {
		names = append(names, a.Name)
	}
	return nil, fmt.Errorf("发布 %s 里没有适用于 %s/%s 的资产（需要 %s；现有资产：%s）", rel.TagName, goos, goarch, want, strings.Join(names, "、"))
}

func (c *Client) download(ctx context.Context, rawURL string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, err
	}
	resp, err := c.httpClient().Do(req)
	if err != nil {
		return nil, fmt.Errorf("下载失败（可能是离线或网络不通）：%w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("下载失败：服务器返回 %s", resp.Status)
	}
	// 上限 256MB，避免异常响应耗尽内存。
	return io.ReadAll(io.LimitReader(resp.Body, 256<<20))
}

// parseChecksums 解析 checksums.txt（每行：<sha256>  <文件名>）。
func parseChecksums(data []byte) (map[string]string, error) {
	out := map[string]string{}
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 2 {
			return nil, fmt.Errorf("checksums.txt 格式无效：%q", line)
		}
		out[fields[1]] = strings.ToLower(fields[0])
	}
	if len(out) == 0 {
		return nil, errors.New("checksums.txt 为空")
	}
	return out, nil
}

// verifyChecksum 按 checksums.txt 校验资产的 sha256。
func verifyChecksum(assetName string, data []byte, sums map[string]string) error {
	want, ok := sums[assetName]
	if !ok {
		return fmt.Errorf("checksums.txt 里没有资产 %s 的校验值，拒绝安装", assetName)
	}
	sum := sha256.Sum256(data)
	if got := hex.EncodeToString(sum[:]); got != want {
		return fmt.Errorf("资产 %s 的 sha256 校验失败（期望 %s，实际 %s），拒绝安装", assetName, want, got)
	}
	return nil
}

// extractBinary 从资产压缩包里取出名为 sunshine（Windows 为
// sunshine.exe）的二进制内容。tar.gz 与 zip 都支持。
func extractBinary(data []byte, goos string) ([]byte, error) {
	want := "sunshine"
	if goos == "windows" {
		want = "sunshine.exe"
	}
	if goos == "windows" {
		return extractZip(data, want)
	}
	return extractTarGz(data, want)
}

func extractTarGz(data []byte, want string) ([]byte, error) {
	gz, err := gzip.NewReader(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("解压资产失败（不是有效的 tar.gz）：%w", err)
	}
	defer gz.Close()
	tr := tar.NewReader(gz)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("解压资产失败：%w", err)
		}
		if hdr.Typeflag != tar.TypeReg {
			continue
		}
		if filepath.Base(hdr.Name) == want {
			return io.ReadAll(io.LimitReader(tr, 256<<20))
		}
	}
	return nil, fmt.Errorf("资产里没有找到二进制 %s", want)
}

func extractZip(data []byte, want string) ([]byte, error) {
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return nil, fmt.Errorf("解压资产失败（不是有效的 zip）：%w", err)
	}
	for _, f := range zr.File {
		if filepath.Base(f.Name) == want {
			rc, err := f.Open()
			if err != nil {
				return nil, err
			}
			defer rc.Close()
			return io.ReadAll(io.LimitReader(rc, 256<<20))
		}
	}
	return nil, fmt.Errorf("资产里没有找到二进制 %s", want)
}

// Options 是 Run 的选项。
type Options struct {
	// CurrentVersion 是当前 CLI 版本。
	CurrentVersion string
	// Tag 是显式 --tag（为空时按当前版本推断通道）。
	Tag string
	// DryRun 只打印计划，不写盘。
	DryRun bool
	// Repo 是仓库地址（owner/name）；为空时用 Client 默认逻辑判定未配置。
	Repo string
	// APIBase 是 Releases API 根地址（测试用 httptest 替换）。
	APIBase string
	// HTTPClient 是发请求的客户端（测试用 httptest 替换）。
	HTTPClient *http.Client
	// ExePath 是要替换的当前二进制路径；为空时用 os.Executable()。
	ExePath string
	// GOOS / GOARCH 覆盖当前平台（测试用；为空时用 runtime 值）。
	GOOS   string
	GOARCH string
	// Out 是输出目标；为 nil 时用 os.Stdout。
	Out io.Writer
}

// Result 是 Run 成功后的结果。
type Result struct {
	// CurrentVersion 是升级前的版本，TargetVersion 是目标版本。
	CurrentVersion string
	TargetVersion  string
	// Tag 是解析出的通道或显式版本。
	Tag string
	// AssetName 是下载的资产名。
	AssetName string
	// BackupPath 是备份路径（.bak）；Windows 交接模式下为空。
	BackupPath string
	// WindowsHandover 为 true 时表示 Windows 交接模式：
	// 新版已写为 .new，须按 HandoverScript 交接后才生效。
	WindowsHandover bool
	// HandoverScript 是 Windows 交接脚本路径。
	HandoverScript string
}

func (o Options) out() io.Writer {
	if o.Out != nil {
		return o.Out
	}
	return os.Stdout
}

func (o Options) goos() string {
	if o.GOOS != "" {
		return o.GOOS
	}
	return runtime.GOOS
}

func (o Options) goarch() string {
	if o.GOARCH != "" {
		return o.GOARCH
	}
	return runtime.GOARCH
}

// Run 执行升级流程：查最新版 → 已是最新则提示退出 →
// 按当前 GOOS/GOARCH 选资产 → 下载 → 校验 sha256 →
// 备份当前二进制（同目录 .bak，权限保留）→ 替换 →
// 运行新版 --version 校验 → 失败回滚备份并报错。
//
// 已是最新时返回 ErrAlreadyLatest（调用方据此打印提示后正常退出）。
func Run(ctx context.Context, opts Options) (*Result, error) {
	w := opts.out()
	if opts.CurrentVersion == "" {
		return nil, errors.New("未传入当前版本（内部错误）")
	}
	goos, goarch := opts.goos(), opts.goarch()
	client := &Client{Repo: opts.Repo, APIBase: opts.APIBase, HTTPClient: opts.HTTPClient}

	tag, err := ResolveTag(opts.CurrentVersion, opts.Tag)
	if err != nil {
		return nil, err
	}

	rel, err := client.ResolveRelease(ctx, tag)
	if err != nil {
		return nil, fmt.Errorf("检查最新版失败：%w%s", err, Troubleshooting(client.ManualURL(), goos))
	}
	target := NormalizeVersion(rel.TagName)
	isExplicitVersion := tag != "latest" && tag != "beta" && tag != "rc"
	if cmp := update.CompareVersions(opts.CurrentVersion, target); cmp == 0 ||
		(cmp > 0 && !isExplicitVersion) {
		fmt.Fprintf(w, "已是最新版本（当前 %s，最新 %s），无需升级。\n", opts.CurrentVersion, target)
		return nil, ErrAlreadyLatest
	}

	asset, err := SelectAsset(rel, target, goos, goarch)
	if err != nil {
		return nil, fmt.Errorf("%w%s", err, Troubleshooting(client.ManualURL(), goos))
	}

	exePath := opts.ExePath
	if exePath == "" {
		exePath, err = os.Executable()
		if err != nil {
			return nil, fmt.Errorf("无法定位当前二进制：%w", err)
		}
	}

	fmt.Fprintf(w, "正在升级 Sunshine：%s → %s（通道 %s）\n", opts.CurrentVersion, target, tag)
	fmt.Fprintf(w, "资产：%s\n", asset.Name)
	if opts.DryRun {
		fmt.Fprintln(w, "下载地址："+asset.BrowserDownloadURL)
		fmt.Fprintf(w, "备份：将把当前二进制备份为 %s.bak\n", exePath)
		fmt.Fprintln(w, "[试运行] 未做任何修改。")
		return &Result{CurrentVersion: opts.CurrentVersion, TargetVersion: target, Tag: tag, AssetName: asset.Name}, nil
	}

	data, err := client.download(ctx, asset.BrowserDownloadURL)
	if err != nil {
		return nil, fmt.Errorf("%w%s", err, Troubleshooting(client.ManualURL(), goos))
	}
	var sums map[string]string
	for _, a := range rel.Assets {
		if a.Name == "checksums.txt" {
			sumData, err := client.download(ctx, a.BrowserDownloadURL)
			if err != nil {
				return nil, fmt.Errorf("下载 checksums.txt 失败：%w%s", err, Troubleshooting(client.ManualURL(), goos))
			}
			sums, err = parseChecksums(sumData)
			if err != nil {
				return nil, fmt.Errorf("解析 checksums.txt 失败：%w", err)
			}
			break
		}
	}
	if sums == nil {
		return nil, fmt.Errorf("发布 %s 里没有 checksums.txt，无法校验，拒绝安装%s", rel.TagName, Troubleshooting(client.ManualURL(), goos))
	}
	if err := verifyChecksum(asset.Name, data, sums); err != nil {
		return nil, fmt.Errorf("%w%s", err, Troubleshooting(client.ManualURL(), goos))
	}
	newBinary, err := extractBinary(data, goos)
	if err != nil {
		return nil, fmt.Errorf("%w%s", err, Troubleshooting(client.ManualURL(), goos))
	}

	// Windows：运行中的二进制不能直接覆盖。v1 只实现交接模式：
	// 新版写为 <exe>.new，并生成交接脚本，标记待验证，不假装已验证。
	if goos == "windows" {
		return runWindowsHandover(w, exePath, newBinary, opts, target, tag, asset.Name, client)
	}

	return runUnixReplace(w, exePath, newBinary, opts, target, tag, asset.Name, client)
}

// runUnixReplace 是 Unix 路径（darwin/linux）：备份 → 替换 →
// 新版 --version 校验 → 失败回滚。
func runUnixReplace(w io.Writer, exePath string, newBinary []byte, opts Options, target, tag, assetName string, client *Client) (*Result, error) {
	goos := opts.goos()
	backupPath := exePath + ".bak"

	info, err := os.Stat(exePath)
	if err != nil {
		return nil, fmt.Errorf("无法读取当前二进制 %s：%w", exePath, err)
	}
	mode := info.Mode().Perm()

	// 备份当前二进制（同目录 .bak，权限保留）。
	// 注意：os.WriteFile 受 umask 影响，写完须再 Chmod 才能精确保留权限。
	old, err := os.ReadFile(exePath)
	if err != nil {
		return nil, fmt.Errorf("备份当前二进制失败：%w", err)
	}
	if err := os.WriteFile(backupPath, old, mode); err != nil {
		return nil, fmt.Errorf("备份当前二进制失败：%w", err)
	}
	if err := os.Chmod(backupPath, mode); err != nil {
		return nil, fmt.Errorf("备份当前二进制失败：%w", err)
	}
	rollback := func() {
		_ = os.WriteFile(exePath, old, mode)
	}

	// 替换：先写临时文件再改名，保证替换是原子的；写完 Chmod
	// 精确保留原权限（WriteFile 受 umask 影响）。
	tmpPath := exePath + ".tmp"
	if err := os.WriteFile(tmpPath, newBinary, mode); err != nil {
		_ = os.Remove(tmpPath)
		return nil, fmt.Errorf("写入新版失败：%w%s", err, Troubleshooting(client.ManualURL(), goos))
	}
	if err := os.Chmod(tmpPath, mode); err != nil {
		_ = os.Remove(tmpPath)
		return nil, fmt.Errorf("写入新版失败：%w%s", err, Troubleshooting(client.ManualURL(), goos))
	}
	if err := os.Rename(tmpPath, exePath); err != nil {
		_ = os.Remove(tmpPath)
		return nil, fmt.Errorf("替换二进制失败：%w%s", err, Troubleshooting(client.ManualURL(), goos))
	}

	// 运行新版 --version 校验；失败则回滚备份并报错。
	if err := verifyNewVersion(exePath, target); err != nil {
		rollback()
		return nil, fmt.Errorf("新版校验失败，已回滚到 %s：%w%s", opts.CurrentVersion, err, Troubleshooting(client.ManualURL(), goos))
	}

	fmt.Fprintf(w, "✓ 升级完成：%s → %s\n", opts.CurrentVersion, target)
	fmt.Fprintf(w, "旧版已备份为 %s\n", backupPath)
	fmt.Fprintln(w, "运行 sunshine --version 确认新版。")
	return &Result{
		CurrentVersion: opts.CurrentVersion,
		TargetVersion:  target,
		Tag:            tag,
		AssetName:      assetName,
		BackupPath:     backupPath,
	}, nil
}

// runWindowsHandover 是 Windows 交接模式（v1，待验证）：
// 新版写为 <exe>.new，生成交接脚本（关闭后由脚本替换并自删），
// 本次不直接替换运行中的二进制。
func runWindowsHandover(w io.Writer, exePath string, newBinary []byte, opts Options, target, tag, assetName string, client *Client) (*Result, error) {
	newPath := exePath + ".new"
	scriptPath := exePath + ".upgrade.bat"
	if err := os.WriteFile(newPath, newBinary, 0o755); err != nil {
		return nil, fmt.Errorf("写入新版失败：%w%s", err, Troubleshooting(client.ManualURL(), opts.goos()))
	}
	// 交接脚本：等待当前进程退出后，用 .new 覆盖原二进制，再自删。
	// 注意：Windows 替换路径尚未在真实 Windows 环境验证。
	script := "@echo off\r\n" +
		"rem Sunshine 升级交接脚本（Windows，待验证）：等待当前进程退出后替换二进制。\r\n" +
		":wait\r\n" +
		"timeout /t 1 /nobreak >nul\r\n" +
		"move /y \"" + newPath + "\" \"" + exePath + "\" >nul 2>&1\r\n" +
		"if exist \"" + newPath + "\" goto wait\r\n" +
		"del \"%~f0\"\r\n"
	if err := os.WriteFile(scriptPath, []byte(script), 0o644); err != nil {
		_ = os.Remove(newPath)
		return nil, fmt.Errorf("写入交接脚本失败：%w", err)
	}
	fmt.Fprintf(w, "新版已准备好（%s → %s），但 Windows 下运行中的二进制不能直接覆盖。\n", opts.CurrentVersion, target)
	fmt.Fprintf(w, "新版已写入 %s\n", newPath)
	fmt.Fprintf(w, "请关闭所有 sunshine 进程后运行交接脚本 %s 完成替换。\n", scriptPath)
	fmt.Fprintln(w, "注意：Windows 替换路径待验证（尚未在真实 Windows 环境验证）。")
	return &Result{
		CurrentVersion:  opts.CurrentVersion,
		TargetVersion:   target,
		Tag:             tag,
		AssetName:       assetName,
		WindowsHandover: true,
		HandoverScript:  scriptPath,
	}, nil
}

// verifyNewVersion 运行新版 --version，要求退出码为 0 且输出含目标版本。
func verifyNewVersion(exePath, target string) error {
	cmd := exec.Command(exePath, "--version")
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("运行新版 --version 失败：%w（输出：%s）", err, strings.TrimSpace(string(out)))
	}
	if !strings.Contains(string(out), target) {
		return fmt.Errorf("新版 --version 输出不含目标版本 %s（输出：%s）", target, strings.TrimSpace(string(out)))
	}
	return nil
}
