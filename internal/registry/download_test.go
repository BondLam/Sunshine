package registry

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// runGit 在 dir 里跑 git 命令（测试夹具用，要求环境有 git）。
func runGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(),
		"GIT_AUTHOR_NAME=test", "GIT_AUTHOR_EMAIL=test@example.com",
		"GIT_COMMITTER_NAME=test", "GIT_COMMITTER_EMAIL=test@example.com")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v 失败：%v\n%s", args, err, out)
	}
}

// initGitRepo 在 dir 建一个 git 仓库并提交 files（相对路径 → 内容）。
func initGitRepo(t *testing.T, dir string, files map[string]string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	runGit(t, dir, "init", "-b", "main")
	for rel, content := range files {
		p := filepath.Join(dir, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	runGit(t, dir, "add", "-A")
	runGit(t, dir, "commit", "-m", "init")
}

// marketplaceIndex 是夹具市场索引：1 spec + 1 skill + 1 workflow。
const marketplaceIndex = `{"version": 1, "templates": [
	{"id": "demo-spec", "type": "spec", "name": "演示规范", "description": "演示用 spec 模板", "path": "specs/demo-spec"},
	{"id": "demo-skill", "type": "skill", "name": "演示 Skill", "description": "演示用 skill", "path": "skills/demo-skill"},
	{"id": "native", "type": "workflow", "name": "原生工作流", "description": "演示用工作流", "path": "workflows/native/workflow.md"}
]}`

// marketplaceFiles 是夹具市场仓库的文件（仓库根布局）。
func marketplaceFiles() map[string]string {
	return map[string]string{
		"index.json":                   marketplaceIndex,
		"specs/demo-spec/backend.md":   "# 后端规范\n",
		"specs/demo-spec/frontend.md":  "# 前端规范\n",
		"skills/demo-skill/SKILL.md":   "# 演示 Skill\n",
		"workflows/native/workflow.md": "# 工作流\n",
		"README.md":                    "# 演示市场\n",
	}
}

// readFile 读文件内容（测试辅助）。
func readFile(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("读 %s 失败：%v", path, err)
	}
	return string(data)
}

func TestProbeAndInstallMarketplaceLocal(t *testing.T) {
	repo := filepath.Join(t.TempDir(), "market")
	initGitRepo(t, repo, marketplaceFiles())
	proj := t.TempDir()

	src, err := ParseRegistrySource(repo)
	if err != nil {
		t.Fatalf("解析本地来源失败：%v", err)
	}
	probe := ProbeRegistryIndex(context.Background(), src)
	defer probe.Close()
	if probe.Err != nil {
		t.Fatalf("探测失败：%v", probe.Err)
	}
	if probe.IsNotFound {
		t.Fatal("应探测到 index.json")
	}
	if len(probe.Templates) != 3 {
		t.Fatalf("模板数 = %d，想得到 3", len(probe.Templates))
	}

	// --template 指定 spec 条目。
	result, err := Install(proj, InstallOptions{Source: repo, Template: "demo-spec", Strategy: StrategySkip, Probe: probe})
	if err != nil {
		t.Fatalf("Install 失败：%v", err)
	}
	if result.Mode != "marketplace" || result.Skipped {
		t.Fatalf("result = %+v", result)
	}
	if readFile(t, filepath.Join(proj, ".sunshine", "spec", "backend.md")) != "# 后端规范\n" {
		t.Error("spec 文件内容不对")
	}
	if readFile(t, filepath.Join(proj, ".sunshine", "spec", "frontend.md")) != "# 前端规范\n" {
		t.Error("spec 文件内容不对")
	}
	if len(result.Files) != 2 {
		t.Errorf("Files = %v，想得到 2 个", result.Files)
	}
	if result.SpecConfig.Source != repo || result.SpecConfig.Template != "demo-spec" {
		t.Errorf("SpecConfig = %+v", result.SpecConfig)
	}

	// 再次安装（skip 策略）应跳过。
	probe2 := ProbeRegistryIndex(context.Background(), src)
	defer probe2.Close()
	result2, err := Install(proj, InstallOptions{Source: repo, Template: "demo-spec", Strategy: StrategySkip, Probe: probe2})
	if err != nil {
		t.Fatalf("重复 Install 失败：%v", err)
	}
	if !result2.Skipped {
		t.Error("重复安装应跳过")
	}

	// skill 条目装进 .agents/skills/<id>/。
	result3, err := Install(proj, InstallOptions{Source: repo, Template: "demo-skill", Strategy: StrategySkip})
	if err != nil {
		t.Fatalf("安装 skill 失败：%v", err)
	}
	if readFile(t, filepath.Join(proj, ".agents", "skills", "demo-skill", "SKILL.md")) != "# 演示 Skill\n" {
		t.Error("skill 文件内容不对")
	}
	if result3.DestRel != ".agents/skills/demo-skill" {
		t.Errorf("DestRel = %q", result3.DestRel)
	}

	// workflow 条目应拒绝，提示改用 workflow 命令。
	if _, err := Install(proj, InstallOptions{Source: repo, Template: "native", Strategy: StrategySkip}); err == nil {
		t.Error("workflow 模板应拒绝安装")
	} else if !strings.Contains(err.Error(), "sunshine workflow --template") {
		t.Errorf("错误信息应提示 workflow 命令：%v", err)
	}

	// 不存在的模板 id 应报错并列出可选 id。
	if _, err := Install(proj, InstallOptions{Source: repo, Template: "no-such", Strategy: StrategySkip}); err == nil {
		t.Error("不存在的模板应报错")
	} else if !strings.Contains(err.Error(), "demo-spec") {
		t.Errorf("错误信息应列出可选模板：%v", err)
	}
}

func TestResolveTargetMarketplaceRequiresTemplate(t *testing.T) {
	repo := filepath.Join(t.TempDir(), "market")
	initGitRepo(t, repo, marketplaceFiles())
	if _, err := ResolveTarget(repo, ""); err == nil {
		t.Error("marketplace 模式不指定 --template 应报错")
	} else if !strings.Contains(err.Error(), "--template") {
		t.Errorf("错误信息应提示 --template：%v", err)
	}
	target, err := ResolveTarget(repo, "demo-spec")
	if err != nil {
		t.Fatalf("ResolveTarget 失败：%v", err)
	}
	defer target.Close()
	if target.Mode != "marketplace" || !target.WritesSpec() {
		t.Errorf("target = %+v", target)
	}
	skillTarget, err := ResolveTarget(repo, "demo-skill")
	if err != nil {
		t.Fatalf("ResolveTarget(skill) 失败：%v", err)
	}
	defer skillTarget.Close()
	if skillTarget.WritesSpec() {
		t.Error("skill 模板不应写 spec")
	}
}

func TestResolveTargetWithProbe(t *testing.T) {
	repo := filepath.Join(t.TempDir(), "market")
	initGitRepo(t, repo, marketplaceFiles())
	src, err := ParseRegistrySource(repo)
	if err != nil {
		t.Fatalf("解析本地来源失败：%v", err)
	}

	// 复用一次探测解析 spec 模板（交互选模板流程的路径）。
	probe := ProbeRegistryIndex(context.Background(), src)
	if probe.Err != nil {
		t.Fatalf("探测失败：%v", probe.Err)
	}
	target, err := ResolveTargetWithProbe(src, probe, "demo-spec")
	if err != nil {
		t.Fatalf("ResolveTargetWithProbe 失败：%v", err)
	}
	defer target.Close()
	if target.Mode != "marketplace" || target.Template == nil || target.Template.ID != "demo-spec" {
		t.Errorf("target = %+v", target)
	}
	if target.Probe != probe {
		t.Error("Target 应接管传入的 probe（不应二次探测）")
	}

	// 不指定模板应报错（与 ResolveTarget 一致）。
	probe2 := ProbeRegistryIndex(context.Background(), src)
	if probe2.Err != nil {
		t.Fatalf("探测失败：%v", probe2.Err)
	}
	if _, err := ResolveTargetWithProbe(src, probe2, ""); err == nil {
		t.Error("marketplace 模式不指定模板应报错")
	}

	// 不存在的模板应报错。
	probe3 := ProbeRegistryIndex(context.Background(), src)
	if probe3.Err != nil {
		t.Fatalf("探测失败：%v", probe3.Err)
	}
	if _, err := ResolveTargetWithProbe(src, probe3, "nope"); err == nil {
		t.Error("不存在的模板应报错")
	}

	// workflow 型模板应拒绝。
	probe4 := ProbeRegistryIndex(context.Background(), src)
	if probe4.Err != nil {
		t.Fatalf("探测失败：%v", probe4.Err)
	}
	if _, err := ResolveTargetWithProbe(src, probe4, "native"); err == nil {
		t.Error("workflow 型模板应拒绝")
	}
}

func TestInstallDirectLocal(t *testing.T) {
	// 直连型：仓库根就是 spec 目录，无 index.json。
	repo := filepath.Join(t.TempDir(), "specrepo")
	initGitRepo(t, repo, map[string]string{
		"backend.md": "# 后端规范\n",
		"guide/a.md": "# 指南\n",
	})
	proj := t.TempDir()
	result, err := Install(proj, InstallOptions{Source: repo, Strategy: StrategySkip})
	if err != nil {
		t.Fatalf("Install 失败：%v", err)
	}
	if result.Mode != "direct" {
		t.Fatalf("Mode = %q，想得到 direct", result.Mode)
	}
	if readFile(t, filepath.Join(proj, ".sunshine", "spec", "backend.md")) != "# 后端规范\n" {
		t.Error("直连安装内容不对")
	}
	if readFile(t, filepath.Join(proj, ".sunshine", "spec", "guide", "a.md")) != "# 指南\n" {
		t.Error("直连安装子目录内容不对")
	}
	if result.SpecConfig.Template != "" {
		t.Errorf("直连模式不应记 template：%+v", result.SpecConfig)
	}
	if len(result.Files) != 2 {
		t.Errorf("Files = %v", result.Files)
	}
}

func TestInstallDirectMarketplaceSpecs(t *testing.T) {
	// 直连型：仓库根含 marketplace/specs/（无 index.json），
	// 整个 marketplace/specs/ 当一个模板。
	repo := filepath.Join(t.TempDir(), "mono")
	initGitRepo(t, repo, map[string]string{
		"marketplace/specs/a.md": "# A\n",
		"marketplace/specs/b.md": "# B\n",
		"other.txt":              "不应装进 spec\n",
	})
	proj := t.TempDir()
	result, err := Install(proj, InstallOptions{Source: repo, Strategy: StrategySkip})
	if err != nil {
		t.Fatalf("Install 失败：%v", err)
	}
	if result.Mode != "direct" {
		t.Fatalf("Mode = %q", result.Mode)
	}
	if readFile(t, filepath.Join(proj, ".sunshine", "spec", "a.md")) != "# A\n" {
		t.Error("marketplace/specs 内容不对")
	}
	if _, err := os.Stat(filepath.Join(proj, ".sunshine", "spec", "other.txt")); !os.IsNotExist(err) {
		t.Error("仓库根的其他文件不应装进 spec")
	}
}

func TestInstallSubdirSparseCheckout(t *testing.T) {
	// 带 subdir 的 registry：marketplace/ 在仓库子目录，模板 path
	// 按 registry 根（marketplace/）解析，稀疏检出只取该目录。
	repo := filepath.Join(t.TempDir(), "mono")
	initGitRepo(t, repo, map[string]string{
		"marketplace/index.json":                   marketplaceIndex,
		"marketplace/specs/demo-spec/a.md":         "# A\n",
		"marketplace/skills/demo-skill/SKILL.md":   "# 演示 Skill\n",
		"marketplace/workflows/native/workflow.md": "# 工作流\n",
		"unrelated/big.bin":                        "不应检出\n",
	})
	proj := t.TempDir()
	src := repo + "/marketplace"
	// 用 gh 风格的子目录写法同样应解析出 subdir（本地路径加子目录）。
	localSrc, err := ParseRegistrySource(src)
	if err != nil {
		t.Fatalf("解析带子目录的本地来源失败：%v", err)
	}
	if localSrc.Subdir != "marketplace" {
		t.Fatalf("Subdir = %q，想得到 marketplace", localSrc.Subdir)
	}
	_ = localSrc
	if _, err := Install(proj, InstallOptions{Source: src, Template: "demo-spec", Strategy: StrategySkip}); err != nil {
		t.Fatalf("Install 失败：%v", err)
	}
	if readFile(t, filepath.Join(proj, ".sunshine", "spec", "a.md")) != "# A\n" {
		t.Error("子目录 registry 的 spec 内容不对")
	}
}

func TestInstallStrategies(t *testing.T) {
	repo := filepath.Join(t.TempDir(), "market")
	initGitRepo(t, repo, marketplaceFiles())
	proj := t.TempDir()
	if _, err := Install(proj, InstallOptions{Source: repo, Template: "demo-spec", Strategy: StrategySkip}); err != nil {
		t.Fatalf("首次安装失败：%v", err)
	}
	// 用户改动一个文件。
	if err := os.WriteFile(filepath.Join(proj, ".sunshine", "spec", "backend.md"), []byte("# 用户改动\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	// skip：改动保留。
	if _, err := Install(proj, InstallOptions{Source: repo, Template: "demo-spec", Strategy: StrategySkip}); err != nil {
		t.Fatalf("skip 安装失败：%v", err)
	}
	if readFile(t, filepath.Join(proj, ".sunshine", "spec", "backend.md")) != "# 用户改动\n" {
		t.Error("skip 策略应保留用户改动")
	}
	// overwrite：恢复模板内容。
	if _, err := Install(proj, InstallOptions{Source: repo, Template: "demo-spec", Strategy: StrategyOverwrite}); err != nil {
		t.Fatalf("overwrite 安装失败：%v", err)
	}
	if readFile(t, filepath.Join(proj, ".sunshine", "spec", "backend.md")) != "# 后端规范\n" {
		t.Error("overwrite 策略应恢复模板内容")
	}
	// append：只补缺失文件，已存在的不动。
	if err := os.WriteFile(filepath.Join(proj, ".sunshine", "spec", "backend.md"), []byte("# 用户改动2\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(proj, ".sunshine", "spec", "frontend.md")); err != nil {
		t.Fatal(err)
	}
	if _, err := Install(proj, InstallOptions{Source: repo, Template: "demo-spec", Strategy: StrategyAppend}); err != nil {
		t.Fatalf("append 安装失败：%v", err)
	}
	if readFile(t, filepath.Join(proj, ".sunshine", "spec", "backend.md")) != "# 用户改动2\n" {
		t.Error("append 策略不应覆盖已存在文件")
	}
	if readFile(t, filepath.Join(proj, ".sunshine", "spec", "frontend.md")) != "# 前端规范\n" {
		t.Error("append 策略应补回缺失文件")
	}
}

func TestRefreshSpecTemplates(t *testing.T) {
	repo := filepath.Join(t.TempDir(), "market")
	initGitRepo(t, repo, marketplaceFiles())
	// marketplace 型刷新。
	files, err := RefreshSpecTemplates(repo, "demo-spec")
	if err != nil {
		t.Fatalf("刷新失败：%v", err)
	}
	if files[".sunshine/spec/backend.md"] != "# 后端规范\n" {
		t.Errorf("刷新结果不对：%v", files)
	}
	// 缺 template 时报错（不静默返回空集合）。
	if _, err := RefreshSpecTemplates(repo, ""); err == nil {
		t.Error("marketplace 型缺 template 应报错")
	}
	// 不存在的模板报错。
	if _, err := RefreshSpecTemplates(repo, "no-such"); err == nil {
		t.Error("不存在的模板应报错")
	}
	// 直连型刷新。
	direct := filepath.Join(t.TempDir(), "specrepo")
	initGitRepo(t, direct, map[string]string{"x.md": "# X\n"})
	files, err = RefreshSpecTemplates(direct, "")
	if err != nil {
		t.Fatalf("直连刷新失败：%v", err)
	}
	if files[".sunshine/spec/x.md"] != "# X\n" {
		t.Errorf("直连刷新结果不对：%v", files)
	}
	// Refresher 实现 update.RegistryRefresher 的方法签名。
	var _ = Refresher{}.RefreshSpecTemplates
}

func TestCollectRegistrySpecTemplates(t *testing.T) {
	repo := filepath.Join(t.TempDir(), "market")
	initGitRepo(t, repo, marketplaceFiles())
	proj := t.TempDir()
	if err := os.MkdirAll(filepath.Join(proj, ".sunshine"), 0o755); err != nil {
		t.Fatal(err)
	}
	configYAML := "version: 0.1.0\n\nregistry:\n  spec:\n    source: " + repo + "\n    template: demo-spec\n"
	if err := os.WriteFile(filepath.Join(proj, ".sunshine", "config.yaml"), []byte(configYAML), 0o644); err != nil {
		t.Fatal(err)
	}
	files := CollectRegistrySpecTemplates(proj)
	if files[".sunshine/spec/backend.md"] != "# 后端规范\n" {
		t.Errorf("Collect 结果不对：%v", files)
	}
	// 未配 registry.spec → 空集合。
	proj2 := t.TempDir()
	if err := os.MkdirAll(filepath.Join(proj2, ".sunshine"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(proj2, ".sunshine", "config.yaml"), []byte("version: 0.1.0\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := CollectRegistrySpecTemplates(proj2); len(got) != 0 {
		t.Errorf("未配 registry.spec 应返回空集合，得到 %v", got)
	}
	// marketplace 型缺 template → 空集合（只警告）。
	proj3 := t.TempDir()
	if err := os.MkdirAll(filepath.Join(proj3, ".sunshine"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(proj3, ".sunshine", "config.yaml"), []byte("registry:\n  spec:\n    source: "+repo+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := CollectRegistrySpecTemplates(proj3); len(got) != 0 {
		t.Errorf("缺 template 应返回空集合，得到 %v", got)
	}
}

func TestInstallInvalidSource(t *testing.T) {
	proj := t.TempDir()
	if _, err := Install(proj, InstallOptions{Source: "not-a-source", Strategy: StrategySkip}); err == nil {
		t.Error("无效来源应报错")
	}
	// 本地路径不是 git 仓库。
	if _, err := Install(proj, InstallOptions{Source: t.TempDir(), Strategy: StrategySkip}); err == nil {
		t.Error("非 git 仓库的本地路径应报错")
	} else if re := AsRegistryError(err); re == nil || re.Kind != ErrKindNotFound {
		t.Errorf("应为 not-found 错误，得到 %v", err)
	}
}
