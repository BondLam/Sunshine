package cli

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/linrong007/Sunshine/internal/lifecycle"
	"github.com/linrong007/Sunshine/internal/platforms"
	"github.com/linrong007/Sunshine/templates"
	"github.com/spf13/cobra"
)

// testLifecycleRoot 建一个只含四个生命周期命令的测试根命令。
func testLifecycleRoot() *cobra.Command {
	root := &cobra.Command{
		Use:           "sunshine",
		SilenceErrors: true,
		SilenceUsage:  true,
	}
	RegisterUninstallCommand(root)
	RegisterAblateCommand(root)
	RegisterRestoreCommand(root)
	RegisterPlatformsCommand(root)
	return root
}

// captureStdout 在 fn 执行期间截获 os.Stdout，返回输出。
func captureStdout(t *testing.T, fn func() error) (string, error) {
	t.Helper()
	old := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stdout = w
	runErr := fn()
	_ = w.Close()
	os.Stdout = old
	var buf bytes.Buffer
	if _, err := io.Copy(&buf, r); err != nil {
		t.Fatal(err)
	}
	return buf.String(), runErr
}

// setupLifecycleProject 在临时目录里搭建一个真实的已初始化项目
// （内嵌模板骨架 + 三平台安装 + 哈希登记），并把工作目录切过去
// （测试结束恢复）。骨架搭建不用 scaffold 包：scaffold 在阶段 4
// 接了 internal/registry（B 组施工中），测试直接依赖它会在
// 他组包编译失败时连带失败；逻辑见 lifecycle 测试里的同名 helper。
func setupLifecycleProject(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	writeLifecycleSkeleton(t, dir)
	for _, id := range []string{"claude-code", "cursor", "codex"} {
		files, err := platforms.Install(dir, id, platforms.InstallOptions{})
		if err != nil {
			t.Fatalf("安装平台 %s 失败：%v", id, err)
		}
		recordLifecycleHashes(t, dir, files)
	}
	t.Chdir(dir)
	return dir
}

var lifecycleHashExcluded = map[string]bool{
	".template-hashes.json": true,
	".version":              true,
	".developer":            true,
}

func writeLifecycleSkeleton(t *testing.T, dir string) {
	t.Helper()
	target := filepath.Join(dir, ".sunshine")
	hashes := map[string]string{}
	err := fs.WalkDir(templates.Sunshine, "sunshine", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		rel := strings.TrimPrefix(path, "sunshine/")
		data, err := templates.Sunshine.ReadFile(path)
		if err != nil {
			return err
		}
		dest := filepath.Join(target, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(dest, data, 0o644); err != nil {
			return err
		}
		if !lifecycleHashExcluded[rel] {
			sum := sha256.Sum256(data)
			hashes[filepath.ToSlash(filepath.Join(".sunshine", rel))] = hex.EncodeToString(sum[:])
		}
		return nil
	})
	if err != nil {
		t.Fatalf("写骨架失败：%v", err)
	}
	nativeWorkflow, err := templates.Workflows.ReadFile("workflows/native/workflow.md")
	if err != nil {
		t.Fatalf("读原生工作流模板失败：%v", err)
	}
	if err := os.WriteFile(filepath.Join(target, "workflow.md"), nativeWorkflow, 0o644); err != nil {
		t.Fatalf("写 workflow.md 失败：%v", err)
	}
	sum := sha256.Sum256(nativeWorkflow)
	hashes[".sunshine/workflow.md"] = hex.EncodeToString(sum[:])
	if err := os.WriteFile(filepath.Join(target, ".developer"), []byte("tester\n"), 0o644); err != nil {
		t.Fatalf("写开发者身份失败：%v", err)
	}
	if err := lifecycle.SaveHashes(dir, hashes); err != nil {
		t.Fatalf("写清单失败：%v", err)
	}
}

func recordLifecycleHashes(t *testing.T, dir string, files []string) {
	t.Helper()
	hashes, err := lifecycle.LoadHashes(dir)
	if err != nil {
		t.Fatalf("读清单失败：%v", err)
	}
	for _, rel := range files {
		data, err := os.ReadFile(filepath.Join(dir, filepath.FromSlash(rel)))
		if err != nil {
			t.Fatalf("计算 %s 的哈希失败：%v", rel, err)
		}
		sum := sha256.Sum256(data)
		hashes[rel] = hex.EncodeToString(sum[:])
	}
	if err := lifecycle.SaveHashes(dir, hashes); err != nil {
		t.Fatalf("写清单失败：%v", err)
	}
}

func TestLifecycleCommandsRegistered(t *testing.T) {
	root := testLifecycleRoot()
	var names []string
	for _, cmd := range root.Commands() {
		names = append(names, cmd.Name())
	}
	for _, want := range []string{"uninstall", "ablate", "restore", "platforms"} {
		found := false
		for _, name := range names {
			if name == want {
				found = true
			}
		}
		if !found {
			t.Errorf("根命令下缺少 %s，实际 %v", want, names)
		}
	}
}

func TestPlatformsCommandJSONMatchesHumanReadable(t *testing.T) {
	setupLifecycleProject(t)
	root := testLifecycleRoot()

	platformsCmd, _, err := root.Find([]string{"platforms"})
	if err != nil {
		t.Fatal(err)
	}
	if err := platformsCmd.Flags().Set("json", "true"); err != nil {
		t.Fatal(err)
	}
	jsonOut, err := captureStdout(t, func() error {
		return platformsCmd.RunE(platformsCmd, nil)
	})
	if err != nil {
		t.Fatalf("platforms --json 失败：%v", err)
	}
	var decoded struct {
		Platforms []lifecycle.PlatformInfo `json:"platforms"`
	}
	if err := json.Unmarshal([]byte(jsonOut), &decoded); err != nil {
		t.Fatalf("platforms --json 输出不是合法 JSON：%v\n%s", err, jsonOut)
	}
	if len(decoded.Platforms) != 3 {
		t.Fatalf("--json 平台数 = %d，想得到 3", len(decoded.Platforms))
	}

	// 人类可读输出应含同一批平台。
	root2 := testLifecycleRoot()
	humanCmd, _, err := root2.Find([]string{"platforms"})
	if err != nil {
		t.Fatal(err)
	}
	humanOut, err := captureStdout(t, func() error {
		return humanCmd.RunE(humanCmd, nil)
	})
	if err != nil {
		t.Fatalf("platforms 失败：%v", err)
	}
	for _, info := range decoded.Platforms {
		if !strings.Contains(humanOut, info.ID) || !strings.Contains(humanOut, info.DisplayName) || !strings.Contains(humanOut, info.ConfigDir) {
			t.Errorf("人类可读输出里缺少平台 %+v，实际：\n%s", info, humanOut)
		}
	}
}

func TestPlatformsCommandNoneConfigured(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	root := testLifecycleRoot()
	platformsCmd, _, err := root.Find([]string{"platforms"})
	if err != nil {
		t.Fatal(err)
	}
	out, err := captureStdout(t, func() error {
		return platformsCmd.RunE(platformsCmd, nil)
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "未配置任何平台") {
		t.Errorf("未配置平台时应打印提示，实际：%s", out)
	}
}

func TestUninstallCommandDryRun(t *testing.T) {
	dir := setupLifecycleProject(t)
	root := testLifecycleRoot()
	uninstallCmd, _, err := root.Find([]string{"uninstall"})
	if err != nil {
		t.Fatal(err)
	}
	if err := uninstallCmd.Flags().Set("dry-run", "true"); err != nil {
		t.Fatal(err)
	}
	if err := uninstallCmd.Flags().Set("yes", "true"); err != nil {
		t.Fatal(err)
	}
	out, err := captureStdout(t, func() error {
		return uninstallCmd.RunE(uninstallCmd, nil)
	})
	if err != nil {
		t.Fatalf("uninstall --dry-run 失败：%v", err)
	}
	if !strings.Contains(out, "试运行") {
		t.Errorf("dry-run 输出应提到试运行，实际：\n%s", out)
	}
	if _, err := os.Stat(filepath.Join(dir, ".sunshine")); err != nil {
		t.Error("dry-run 不应删除 .sunshine/")
	}
}

func TestAblateRestoreCommandsRoundTrip(t *testing.T) {
	dir := setupLifecycleProject(t)
	stateRoot := t.TempDir()
	t.Setenv(lifecycle.AblationStateRootEnv, stateRoot)

	root := testLifecycleRoot()
	ablateCmd, _, err := root.Find([]string{"ablate"})
	if err != nil {
		t.Fatal(err)
	}
	if err := ablateCmd.Flags().Set("yes", "true"); err != nil {
		t.Fatal(err)
	}
	if _, err := captureStdout(t, func() error {
		return ablateCmd.RunE(ablateCmd, nil)
	}); err != nil {
		t.Fatalf("ablate 失败：%v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, ".sunshine")); !os.IsNotExist(err) {
		t.Error("ablate 后 .sunshine/ 应已不存在")
	}

	root2 := testLifecycleRoot()
	restoreCmd, _, err := root2.Find([]string{"restore"})
	if err != nil {
		t.Fatal(err)
	}
	if err := restoreCmd.Flags().Set("yes", "true"); err != nil {
		t.Fatal(err)
	}
	if _, err := captureStdout(t, func() error {
		return restoreCmd.RunE(restoreCmd, nil)
	}); err != nil {
		t.Fatalf("restore 失败：%v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, ".sunshine", "workflow.md")); err != nil {
		t.Errorf("restore 后 workflow.md 应已恢复：%v", err)
	}
}
