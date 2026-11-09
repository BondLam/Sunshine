package upgrade

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// fakeBinary 生成一个假的 sunshine 二进制（shell 脚本）：
// --version 时打印对应版本；fail 为 true 时 --version 直接退出 1。
func fakeBinary(version string, fail bool) []byte {
	if fail {
		return []byte("#!/bin/sh\nexit 1\n")
	}
	return []byte("#!/bin/sh\nif [ \"$1\" = \"--version\" ]; then echo \"Sunshine 版本 " + version + "\"; fi\n")
}

func makeTarGz(t *testing.T, files map[string][]byte) []byte {
	t.Helper()
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	for name, content := range files {
		if err := tw.WriteHeader(&tar.Header{Name: name, Mode: 0o755, Size: int64(len(content))}); err != nil {
			t.Fatal(err)
		}
		if _, err := tw.Write(content); err != nil {
			t.Fatal(err)
		}
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gz.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func sha256Hex(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

// fakeReleaseServer 是模拟 GitHub Releases API + 资产下载的测试服务。
type fakeReleaseServer struct {
	server     *httptest.Server
	assetBytes []byte // /dl/asset 返回的内容
	sumBytes   []byte // checksums.txt 按此内容算校验值（与 assetBytes 不同即校验失败）
	tag        string // latest 通道返回的 tag
	assetName  string
	// counts 记录各类请求次数。
	assetDownloads int
	latestCalls    int
	listCalls      int
	includeSums    bool
}

func newFakeReleaseServer(t *testing.T, tag string, newVersion string) *fakeReleaseServer {
	t.Helper()
	f := &fakeReleaseServer{tag: tag, includeSums: true}
	asset := makeTarGz(t, map[string][]byte{"sunshine": fakeBinary(newVersion, false)})
	f.assetBytes = asset
	f.sumBytes = asset
	f.assetName = "sunshine_" + newVersion + "_linux_amd64.tar.gz"

	mux := http.NewServeMux()
	releaseJSON := func(w http.ResponseWriter, tag string) {
		assets := fmt.Sprintf(`[{"name":%q,"browser_download_url":%q}`,
			f.assetName, f.server.URL+"/dl/asset")
		if f.includeSums {
			assets += fmt.Sprintf(`,{"name":"checksums.txt","browser_download_url":%q}`, f.server.URL+"/dl/sums")
		}
		assets += "]"
		fmt.Fprintf(w, `{"tag_name":%q,"draft":false,"prerelease":false,"assets":%s}`, tag, assets)
	}
	mux.HandleFunc("/repos/o/r/releases/latest", func(w http.ResponseWriter, r *http.Request) {
		f.latestCalls++
		releaseJSON(w, f.tag)
	})
	mux.HandleFunc("/repos/o/r/releases", func(w http.ResponseWriter, r *http.Request) {
		f.listCalls++
		// beta 列表：第一个是 beta 发布，第二个是正式版。
		fmt.Fprintf(w, `[{"tag_name":"v0.2.0-beta.1","draft":false,"prerelease":true,"assets":[]},{"tag_name":"v0.2.0","draft":false,"prerelease":false,"assets":[]}]`)
	})
	mux.HandleFunc("/repos/o/r/releases/tags/", func(w http.ResponseWriter, r *http.Request) {
		tag := strings.TrimPrefix(r.URL.Path, "/repos/o/r/releases/tags/")
		if tag == "0.2.0-beta.1" || tag == "v0.2.0-beta.1" {
			assets := fmt.Sprintf(`[{"name":%q,"browser_download_url":%q},{"name":"checksums.txt","browser_download_url":%q}]`,
				"sunshine_0.2.0-beta.1_linux_amd64.tar.gz", f.server.URL+"/dl/asset", f.server.URL+"/dl/sums")
			fmt.Fprintf(w, `{"tag_name":"v0.2.0-beta.1","draft":false,"prerelease":true,"assets":%s}`, assets)
			return
		}
		http.NotFound(w, r)
	})
	mux.HandleFunc("/dl/asset", func(w http.ResponseWriter, r *http.Request) {
		f.assetDownloads++
		_, _ = w.Write(f.assetBytes)
	})
	mux.HandleFunc("/dl/sums", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, "%s  %s\n", sha256Hex(f.sumBytes), f.assetName)
	})
	f.server = httptest.NewServer(mux)
	return f
}

func (f *fakeReleaseServer) Close() { f.server.Close() }

// writeFakeExe 在临时目录写一个假的当前二进制，返回路径。
func writeFakeExe(t *testing.T, dir, version string) string {
	t.Helper()
	path := filepath.Join(dir, "sunshine")
	if err := os.WriteFile(path, fakeBinary(version, false), 0o755); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestResolveTag(t *testing.T) {
	cases := []struct {
		current, requested, want string
		wantErr                  bool
	}{
		{"0.1.0", "", "latest", false},
		{"0.2.0-beta.1", "", "beta", false},
		{"0.2.0-rc.1", "", "rc", false},
		{"0.1.0", "beta", "beta", false},
		{"0.1.0", "0.6.0-beta.8", "0.6.0-beta.8", false},
		{"0.1.0", "latest", "latest", false},
		{"0.1.0", "bad tag!", "", true},
		{"0.1.0", "../x", "", true},
		{"0.1.0", "", "latest", false},
	}
	for _, c := range cases {
		got, err := ResolveTag(c.current, c.requested)
		if c.wantErr {
			if err == nil {
				t.Errorf("ResolveTag(%q, %q)：期望报错，实际得到 %q", c.current, c.requested, got)
			}
			continue
		}
		if err != nil {
			t.Errorf("ResolveTag(%q, %q)：意外报错 %v", c.current, c.requested, err)
			continue
		}
		if got != c.want {
			t.Errorf("ResolveTag(%q, %q) = %q，期望 %q", c.current, c.requested, got, c.want)
		}
	}
}

func TestAssetNameAndSelect(t *testing.T) {
	if got := AssetName("v0.2.0", "darwin", "amd64"); got != "sunshine_0.2.0_darwin_amd64.tar.gz" {
		t.Errorf("AssetName darwin = %q", got)
	}
	if got := AssetName("0.2.0", "windows", "amd64"); got != "sunshine_0.2.0_windows_amd64.zip" {
		t.Errorf("AssetName windows = %q", got)
	}
	// 后缀退化匹配：资产名与 goreleaser 默认不同时仍能选中。
	rel := &release{TagName: "v0.2.0", Assets: []releaseAsset{
		{Name: "sunshine-0.2.0-linux-amd64.tar.gz", BrowserDownloadURL: "https://example.invalid/x"},
	}}
	a, err := SelectAsset(rel, "0.2.0", "linux", "amd64")
	if err != nil {
		t.Fatalf("SelectAsset 后缀匹配失败：%v", err)
	}
	if a.Name != "sunshine-0.2.0-linux-amd64.tar.gz" {
		t.Errorf("SelectAsset 选中 %q", a.Name)
	}
	if _, err := SelectAsset(rel, "0.2.0", "darwin", "arm64"); err == nil {
		t.Error("SelectAsset 无适用资产时应报错")
	}
}

func TestTroubleshooting(t *testing.T) {
	text := Troubleshooting("https://github.com/o/r/releases", "linux")
	for _, want := range []string{"手动下载", "https://github.com/o/r/releases", "which sunshine", "sudo", "--force"} {
		if !strings.Contains(text, want) {
			t.Errorf("排查文案缺少 %q", want)
		}
	}
	if got := BinaryCheckCommand("windows"); got != "where sunshine" {
		t.Errorf("Windows 检查命令 = %q", got)
	}
	if strings.Contains(strings.ToLower(text), "trellis") {
		t.Error("排查文案残留 trellis 字样")
	}
}

func TestClientLatestVersion(t *testing.T) {
	f := newFakeReleaseServer(t, "v0.2.0", "0.2.0")
	defer f.Close()
	c := &Client{Repo: "o/r", APIBase: f.server.URL, HTTPClient: f.server.Client()}
	v, err := c.LatestVersion(context.Background())
	if err != nil {
		t.Fatalf("LatestVersion 报错：%v", err)
	}
	if v != "0.2.0" {
		t.Errorf("LatestVersion = %q，期望 0.2.0", v)
	}
	// beta 通道：从列表里挑 -beta 发布。
	rel, err := c.ResolveRelease(context.Background(), "beta")
	if err != nil {
		t.Fatalf("ResolveRelease(beta) 报错：%v", err)
	}
	if rel.TagName != "v0.2.0-beta.1" {
		t.Errorf("beta 发布 = %q", rel.TagName)
	}
	// 显式版本：按 tag 取发布。
	rel, err = c.ResolveRelease(context.Background(), "0.2.0-beta.1")
	if err != nil {
		t.Fatalf("ResolveRelease(0.2.0-beta.1) 报错：%v", err)
	}
	if rel.TagName != "v0.2.0-beta.1" {
		t.Errorf("显式版本发布 = %q", rel.TagName)
	}
}

func TestClientUnconfiguredRepo(t *testing.T) {
	c := &Client{Repo: "", APIBase: "http://127.0.0.1:1"}
	if _, err := c.LatestVersion(context.Background()); err == nil {
		t.Error("仓库未配置时应报错")
	} else if !strings.Contains(err.Error(), "未配置") {
		t.Errorf("未配置报错应说明未配置，实际：%v", err)
	}
	c2 := &Client{Repo: PlaceholderRepo}
	if _, err := c2.LatestVersion(context.Background()); err == nil || !strings.Contains(err.Error(), "未配置") {
		t.Errorf("占位仓库应视为未配置，实际 err=%v", err)
	}
}

func baseOptions(t *testing.T, f *fakeReleaseServer, exePath string) Options {
	t.Helper()
	var buf bytes.Buffer
	return Options{
		CurrentVersion: "0.1.0",
		Repo:           "o/r",
		APIBase:        f.server.URL,
		HTTPClient:     f.server.Client(),
		ExePath:        exePath,
		GOOS:           "linux",
		GOARCH:         "amd64",
		Out:            &buf,
	}
}

func TestRunSuccess(t *testing.T) {
	f := newFakeReleaseServer(t, "v0.2.0", "0.2.0")
	defer f.Close()
	dir := t.TempDir()
	exe := writeFakeExe(t, dir, "0.1.0")
	origMode := func() os.FileMode {
		info, err := os.Stat(exe)
		if err != nil {
			t.Fatal(err)
		}
		return info.Mode().Perm()
	}()
	var out bytes.Buffer
	opts := baseOptions(t, f, exe)
	opts.Out = &out
	res, err := Run(context.Background(), opts)
	if err != nil {
		t.Fatalf("Run 报错：%v", err)
	}
	if res.TargetVersion != "0.2.0" || res.BackupPath != exe+".bak" {
		t.Errorf("结果异常：%+v", res)
	}
	// 新二进制已替换：--version 输出新版。
	got, err := os.ReadFile(exe)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(got, []byte("0.2.0")) {
		t.Error("替换后的二进制不含新版内容")
	}
	// 备份保留旧版，权限与原文件一致。
	bak, err := os.ReadFile(exe + ".bak")
	if err != nil {
		t.Fatalf("备份缺失：%v", err)
	}
	if !bytes.Contains(bak, []byte("0.1.0")) {
		t.Error("备份不是旧版内容")
	}
	if info, _ := os.Stat(exe + ".bak"); info.Mode().Perm() != origMode {
		t.Errorf("备份权限 = %v，期望与原文件一致 %v", info.Mode().Perm(), origMode)
	}
	if info, _ := os.Stat(exe); info.Mode().Perm() != origMode {
		t.Errorf("替换后权限 = %v，期望与原文件一致 %v", info.Mode().Perm(), origMode)
	}
	if !strings.Contains(out.String(), "升级完成") {
		t.Errorf("输出缺少完成提示：%s", out.String())
	}
}

func TestRunChecksumMismatchRollsBack(t *testing.T) {
	f := newFakeReleaseServer(t, "v0.2.0", "0.2.0")
	defer f.Close()
	// checksums 按另一份内容算：校验必失败。
	f.sumBytes = []byte("完全不同的内容")
	dir := t.TempDir()
	exe := writeFakeExe(t, dir, "0.1.0")
	before, _ := os.ReadFile(exe)
	opts := baseOptions(t, f, exe)
	_, err := Run(context.Background(), opts)
	if err == nil {
		t.Fatal("校验失败时应报错")
	}
	if !strings.Contains(err.Error(), "校验失败") {
		t.Errorf("报错应说明校验失败，实际：%v", err)
	}
	after, _ := os.ReadFile(exe)
	if !bytes.Equal(before, after) {
		t.Error("校验失败后原二进制被改动")
	}
	if _, err := os.Stat(exe + ".bak"); !os.IsNotExist(err) {
		t.Error("校验失败不应产生备份（替换尚未开始）")
	}
}

func TestRunAlreadyLatest(t *testing.T) {
	f := newFakeReleaseServer(t, "v0.1.0", "0.1.0")
	defer f.Close()
	dir := t.TempDir()
	exe := writeFakeExe(t, dir, "0.1.0")
	opts := baseOptions(t, f, exe)
	_, err := Run(context.Background(), opts)
	if !strings.Contains(fmt.Sprint(err), "已是最新版本") {
		t.Errorf("已是最新时应返回 ErrAlreadyLatest，实际 err=%v", err)
	}
	if f.assetDownloads != 0 {
		t.Errorf("已是最新时不应下载资产，实际下载 %d 次", f.assetDownloads)
	}
}

func TestRunOffline(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	url := srv.URL
	srv.Close() // 已关闭的地址：模拟离线。
	dir := t.TempDir()
	exe := writeFakeExe(t, dir, "0.1.0")
	opts := Options{
		CurrentVersion: "0.1.0",
		Repo:           "o/r",
		APIBase:        url,
		ExePath:        exe,
		GOOS:           "linux",
		GOARCH:         "amd64",
		Out:            &bytes.Buffer{},
	}
	_, err := Run(context.Background(), opts)
	if err == nil {
		t.Fatal("离线时应报错")
	}
	if !strings.Contains(err.Error(), "离线") && !strings.Contains(err.Error(), "检查最新版失败") {
		t.Errorf("离线报错应说明原因，实际：%v", err)
	}
	after, _ := os.ReadFile(exe)
	if !bytes.Contains(after, []byte("0.1.0")) {
		t.Error("离线时原二进制被改动")
	}
}

func TestRunDryRunWritesNothing(t *testing.T) {
	f := newFakeReleaseServer(t, "v0.2.0", "0.2.0")
	defer f.Close()
	dir := t.TempDir()
	exe := writeFakeExe(t, dir, "0.1.0")
	var out bytes.Buffer
	opts := baseOptions(t, f, exe)
	opts.DryRun = true
	opts.Out = &out
	res, err := Run(context.Background(), opts)
	if err != nil {
		t.Fatalf("试运行报错：%v", err)
	}
	if res.TargetVersion != "0.2.0" {
		t.Errorf("试运行目标版本 = %q", res.TargetVersion)
	}
	if !strings.Contains(out.String(), "[试运行]") {
		t.Errorf("试运行输出缺少标记：%s", out.String())
	}
	if f.assetDownloads != 0 {
		t.Error("试运行不应下载资产")
	}
	if _, err := os.Stat(exe + ".bak"); !os.IsNotExist(err) {
		t.Error("试运行不应产生备份")
	}
	before, _ := os.ReadFile(exe)
	if !bytes.Contains(before, []byte("0.1.0")) {
		t.Error("试运行改动了原二进制")
	}
}

func TestRunVerifyFailureRollsBack(t *testing.T) {
	f := newFakeReleaseServer(t, "v0.2.0", "0.2.0")
	defer f.Close()
	// 新版二进制 --version 失败：替换后校验不过，应回滚。
	f.assetBytes = makeTarGz(t, map[string][]byte{"sunshine": fakeBinary("0.2.0", true)})
	f.sumBytes = f.assetBytes
	f.assetName = "sunshine_0.2.0_linux_amd64.tar.gz"
	dir := t.TempDir()
	exe := writeFakeExe(t, dir, "0.1.0")
	opts := baseOptions(t, f, exe)
	_, err := Run(context.Background(), opts)
	if err == nil {
		t.Fatal("新版校验失败时应报错")
	}
	if !strings.Contains(err.Error(), "回滚") {
		t.Errorf("报错应说明已回滚，实际：%v", err)
	}
	after, _ := os.ReadFile(exe)
	if !bytes.Contains(after, []byte("0.1.0")) {
		t.Error("回滚后原二进制未恢复")
	}
}

func TestRunMissingChecksumsRefuses(t *testing.T) {
	f := newFakeReleaseServer(t, "v0.2.0", "0.2.0")
	defer f.Close()
	f.includeSums = false
	dir := t.TempDir()
	exe := writeFakeExe(t, dir, "0.1.0")
	opts := baseOptions(t, f, exe)
	_, err := Run(context.Background(), opts)
	if err == nil || !strings.Contains(err.Error(), "checksums.txt") {
		t.Errorf("缺 checksums.txt 时应拒绝安装，实际 err=%v", err)
	}
	after, _ := os.ReadFile(exe)
	if !bytes.Contains(after, []byte("0.1.0")) {
		t.Error("拒绝安装后原二进制被改动")
	}
}

func makeZip(t *testing.T, files map[string][]byte) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for name, content := range files {
		w, err := zw.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := w.Write(content); err != nil {
			t.Fatal(err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func TestRunWindowsHandover(t *testing.T) {
	f := newFakeReleaseServer(t, "v0.2.0", "0.2.0")
	defer f.Close()
	// Windows 资产是 zip，内含 sunshine.exe。
	f.assetBytes = makeZip(t, map[string][]byte{"sunshine.exe": fakeBinary("0.2.0", false)})
	f.sumBytes = f.assetBytes
	f.assetName = "sunshine_0.2.0_windows_amd64.zip"
	dir := t.TempDir()
	exe := filepath.Join(dir, "sunshine.exe")
	if err := os.WriteFile(exe, fakeBinary("0.1.0", false), 0o755); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	opts := baseOptions(t, f, exe)
	opts.GOOS = "windows"
	opts.Out = &out
	res, err := Run(context.Background(), opts)
	if err != nil {
		t.Fatalf("Windows 交接报错：%v", err)
	}
	if !res.WindowsHandover || res.HandoverScript != exe+".upgrade.bat" {
		t.Errorf("结果应为交接模式：%+v", res)
	}
	// 原二进制不动，新版写为 .new，交接脚本已生成。
	after, _ := os.ReadFile(exe)
	if !bytes.Contains(after, []byte("0.1.0")) {
		t.Error("交接模式不应直接替换原二进制")
	}
	neu, err := os.ReadFile(exe + ".new")
	if err != nil {
		t.Fatalf(".new 缺失：%v", err)
	}
	if !bytes.Contains(neu, []byte("0.2.0")) {
		t.Error(".new 不是新版内容")
	}
	script, err := os.ReadFile(exe + ".upgrade.bat")
	if err != nil {
		t.Fatalf("交接脚本缺失：%v", err)
	}
	if !bytes.Contains(script, []byte("move /y")) {
		t.Error("交接脚本缺少替换命令")
	}
	if !strings.Contains(out.String(), "待验证") {
		t.Errorf("交接输出应标注待验证：%s", out.String())
	}
}
