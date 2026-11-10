package cli

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/linrong1982/Sunshine/internal/projecttype"
)

// stubProjectTypePrompt 用测试替身替换类型提问，返回恢复函数。
func stubProjectTypePrompt(t *testing.T, fn func() (projecttype.Type, error)) {
	t.Helper()
	old := promptProjectType
	promptProjectType = fn
	t.Cleanup(func() { promptProjectType = old })
}

func TestResolveProjectTypePriority(t *testing.T) {
	cases := []struct {
		name       string
		in         projectTypeInput
		prompt     func() (projecttype.Type, error)
		wantType   projecttype.Type
		wantSource string
		wantErr    bool
	}{
		{
			name:       "--type 覆盖检测结果",
			in:         projectTypeInput{flag: "后端", detected: projecttype.Frontend, isTTY: true},
			wantType:   projecttype.Backend,
			wantSource: "--type 指定",
		},
		{
			name:       "--type 英文值",
			in:         projectTypeInput{flag: "frontend", detected: projecttype.Backend, isTTY: true},
			wantType:   projecttype.Frontend,
			wantSource: "--type 指定",
		},
		{
			name:    "--type 非法值报错",
			in:      projectTypeInput{flag: "移动端", detected: projecttype.Frontend, isTTY: true},
			wantErr: true,
		},
		{
			name: "registry 写 spec 时不检测不提问",
			in:   projectTypeInput{skipSpec: true, detected: projecttype.Unknown, isTTY: true},
			prompt: func() (projecttype.Type, error) {
				t.Error("registry 时不应提问")
				return projecttype.Frontend, nil
			},
			wantType:   projecttype.Fullstack,
			wantSource: "默认",
		},
		{
			name: "检测到前端直接采用",
			in:   projectTypeInput{detected: projecttype.Frontend, isTTY: true},
			prompt: func() (projecttype.Type, error) {
				t.Error("检测到类型时不应提问")
				return projecttype.Backend, nil
			},
			wantType:   projecttype.Frontend,
			wantSource: "自动检测",
		},
		{
			name:       "检测到后端直接采用",
			in:         projectTypeInput{detected: projecttype.Backend, isTTY: true},
			wantType:   projecttype.Backend,
			wantSource: "自动检测",
		},
		{
			name:       "检测到全栈直接采用",
			in:         projectTypeInput{detected: projecttype.Fullstack, isTTY: true},
			wantType:   projecttype.Fullstack,
			wantSource: "自动检测",
		},
		{
			name:       "-y + unknown 默认全栈",
			in:         projectTypeInput{yes: true, detected: projecttype.Unknown, isTTY: true},
			prompt:     func() (projecttype.Type, error) { t.Error("-y 时不应提问"); return projecttype.Frontend, nil },
			wantType:   projecttype.Fullstack,
			wantSource: "默认",
		},
		{
			name:       "非 TTY + unknown 默认全栈",
			in:         projectTypeInput{detected: projecttype.Unknown, isTTY: false},
			prompt:     func() (projecttype.Type, error) { t.Error("非 TTY 时不应提问"); return projecttype.Frontend, nil },
			wantType:   projecttype.Fullstack,
			wantSource: "默认",
		},
		{
			name:       "TTY + unknown 提问选前端",
			in:         projectTypeInput{detected: projecttype.Unknown, isTTY: true},
			prompt:     func() (projecttype.Type, error) { return projecttype.Frontend, nil },
			wantType:   projecttype.Frontend,
			wantSource: "手动选择",
		},
		{
			name:       "提问出错默认全栈",
			in:         projectTypeInput{detected: projecttype.Unknown, isTTY: true},
			prompt:     func() (projecttype.Type, error) { return projecttype.Fullstack, errors.New("EOF") },
			wantType:   projecttype.Fullstack,
			wantSource: "默认",
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if c.prompt != nil {
				stubProjectTypePrompt(t, c.prompt)
			} else {
				stubProjectTypePrompt(t, func() (projecttype.Type, error) {
					t.Error("不应提问")
					return projecttype.Fullstack, nil
				})
			}
			got, source, err := resolveProjectType(c.in)
			if c.wantErr {
				if err == nil {
					t.Fatal("非法 --type 应返回错误")
				}
				if !strings.Contains(err.Error(), "无效的项目类型") {
					t.Errorf("错误信息应含“无效的项目类型”，实际：%v", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("resolveProjectType 失败：%v", err)
			}
			if got != c.wantType {
				t.Errorf("类型 = %v，想得到 %v", got, c.wantType)
			}
			if source != c.wantSource {
				t.Errorf("来源 = %q，想得到 %q", source, c.wantSource)
			}
		})
	}
}

func TestSpecNextStepLine(t *testing.T) {
	cases := []struct {
		specType projecttype.Type
		want     string
	}{
		{projecttype.Frontend, "已生成 system.md、frontend.md"},
		{projecttype.Backend, "已生成 system.md、backend.md"},
		{projecttype.Fullstack, "已生成 system.md、frontend.md、backend.md"},
		{projecttype.Unknown, "已生成 system.md、frontend.md、backend.md"},
	}
	for _, c := range cases {
		got := specNextStepLine(c.specType)
		if !strings.Contains(got, c.want) {
			t.Errorf("specNextStepLine(%v) = %q，应含 %q", c.specType, got, c.want)
		}
		if !strings.HasPrefix(got, "  2. 规范已按默认模板生成在 .sunshine/spec/") {
			t.Errorf("specNextStepLine(%v) 前缀不对：%q", c.specType, got)
		}
	}
}

// ---- init 端到端：临时目录实跑已构建的二进制 ----

var (
	testBinaryOnce sync.Once
	testBinaryPath string
	testBinaryErr  error
)

// buildTestBinary 构建一次 sunshine 二进制（供端到端测试复用）。
func buildTestBinary(t *testing.T) string {
	t.Helper()
	testBinaryOnce.Do(func() {
		goBin, err := exec.LookPath("go")
		if err != nil {
			goBin = filepath.Join(os.Getenv("HOME"), "go", "bin", "go")
		}
		wd, err := os.Getwd()
		if err != nil {
			testBinaryErr = err
			return
		}
		repoRoot := filepath.Dir(filepath.Dir(wd))
		dir, err := os.MkdirTemp("", "sunshine-test-bin-*")
		if err != nil {
			testBinaryErr = err
			return
		}
		testBinaryPath = filepath.Join(dir, "sunshine")
		cmd := exec.Command(goBin, "build", "-o", testBinaryPath, "./cmd/sunshine")
		cmd.Dir = repoRoot
		cmd.Env = append(os.Environ(), "PATH="+os.Getenv("PATH")+":"+filepath.Join(os.Getenv("HOME"), "go", "bin"))
		var stderr bytes.Buffer
		cmd.Stderr = &stderr
		if err := cmd.Run(); err != nil {
			testBinaryErr = errors.New("构建测试二进制失败：" + err.Error() + "：" + stderr.String())
			return
		}
	})
	if testBinaryErr != nil {
		t.Fatal(testBinaryErr)
	}
	return testBinaryPath
}

// runInitBinary 在 dir 里跑一次 sunshine init，返回输出与退出码。
func runInitBinary(t *testing.T, dir string, args ...string) (string, int) {
	t.Helper()
	bin := buildTestBinary(t)
	cmd := exec.Command(bin, append([]string{"init"}, args...)...)
	cmd.Dir = dir
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out
	err := cmd.Run()
	code := 0
	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			code = exitErr.ExitCode()
		} else {
			t.Fatalf("运行 init 失败：%v", err)
		}
	}
	return out.String(), code
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

// assertInitSpecFiles 校验 init 后 spec 目录的文件集与哈希清单。
func assertInitSpecFiles(t *testing.T, dir string, wantFrontend, wantBackend bool) {
	t.Helper()
	for _, name := range []string{"system.md", "frontend.md", "backend.md"} {
		_, err := os.Stat(filepath.Join(dir, ".sunshine/spec", name))
		want := true
		if name == "frontend.md" {
			want = wantFrontend
		}
		if name == "backend.md" {
			want = wantBackend
		}
		if want && err != nil {
			t.Errorf("缺少 .sunshine/spec/%s：%v", name, err)
		}
		if !want && err == nil {
			t.Errorf("不应生成 .sunshine/spec/%s", name)
		}
	}
	data, err := os.ReadFile(filepath.Join(dir, ".sunshine/.template-hashes.json"))
	if err != nil {
		t.Fatalf("读取哈希清单失败：%v", err)
	}
	var manifest struct {
		Files map[string]string `json:"files"`
	}
	if err := json.Unmarshal(data, &manifest); err != nil {
		t.Fatalf("解析哈希清单失败：%v", err)
	}
	for _, tc := range []struct {
		key  string
		want bool
	}{
		{".sunshine/spec/system.md", true},
		{".sunshine/spec/frontend.md", wantFrontend},
		{".sunshine/spec/backend.md", wantBackend},
	} {
		_, ok := manifest.Files[tc.key]
		if tc.want && !ok {
			t.Errorf("哈希清单缺少 %s", tc.key)
		}
		if !tc.want && ok {
			t.Errorf("哈希清单不应含 %s", tc.key)
		}
	}
}

func TestInitEndToEndProjectType(t *testing.T) {
	// 前端项目：vite.config.ts → 只剩 system.md + frontend.md。
	t.Run("前端检测", func(t *testing.T) {
		dir := t.TempDir()
		writeFile(t, filepath.Join(dir, "vite.config.ts"), "export default {}\n")
		out, code := runInitBinary(t, dir, "-y", "-u", "测试")
		if code != 0 {
			t.Fatalf("init 退出码 %d，输出：%s", code, out)
		}
		assertInitSpecFiles(t, dir, true, false)
		if !strings.Contains(out, "项目类型：前端（自动检测）") {
			t.Errorf("输出应含“项目类型：前端（自动检测）”，实际：\n%s", out)
		}
		if !strings.Contains(out, "已生成 system.md、frontend.md") {
			t.Errorf("“下一步”第 2 行应列出已生成的 spec 文件，实际：\n%s", out)
		}
	})

	// 后端项目：go.mod → 只剩 system.md + backend.md。
	t.Run("后端检测", func(t *testing.T) {
		dir := t.TempDir()
		writeFile(t, filepath.Join(dir, "go.mod"), "module example.com/demo\n\ngo 1.27\n")
		out, code := runInitBinary(t, dir, "-y", "-u", "测试")
		if code != 0 {
			t.Fatalf("init 退出码 %d，输出：%s", code, out)
		}
		assertInitSpecFiles(t, dir, false, true)
		if !strings.Contains(out, "项目类型：后端（自动检测）") {
			t.Errorf("输出应含“项目类型：后端（自动检测）”，实际：\n%s", out)
		}
		if !strings.Contains(out, "已生成 system.md、backend.md") {
			t.Errorf("“下一步”第 2 行应列出已生成的 spec 文件，实际：\n%s", out)
		}
	})

	// 全栈项目：react + express 的 package.json → 三份都在。
	t.Run("全栈检测", func(t *testing.T) {
		dir := t.TempDir()
		writeFile(t, filepath.Join(dir, "package.json"), `{"name":"demo","dependencies":{"react":"^19.0.0","express":"^4.0.0"}}`+"\n")
		out, code := runInitBinary(t, dir, "-y", "-u", "测试")
		if code != 0 {
			t.Fatalf("init 退出码 %d，输出：%s", code, out)
		}
		assertInitSpecFiles(t, dir, true, true)
		if !strings.Contains(out, "项目类型：全栈（自动检测）") {
			t.Errorf("输出应含“项目类型：全栈（自动检测）”，实际：\n%s", out)
		}
		if !strings.Contains(out, "已生成 system.md、frontend.md、backend.md") {
			t.Errorf("“下一步”第 2 行应列出已生成的 spec 文件，实际：\n%s", out)
		}
	})

	// --type 后端覆盖前端检测结果。
	t.Run("--type 覆盖检测", func(t *testing.T) {
		dir := t.TempDir()
		writeFile(t, filepath.Join(dir, "vite.config.ts"), "export default {}\n")
		out, code := runInitBinary(t, dir, "-y", "-u", "测试", "--type", "后端")
		if code != 0 {
			t.Fatalf("init 退出码 %d，输出：%s", code, out)
		}
		assertInitSpecFiles(t, dir, false, true)
		if !strings.Contains(out, "项目类型：后端（--type 指定）") {
			t.Errorf("输出应含“项目类型：后端（--type 指定）”，实际：\n%s", out)
		}
	})

	// --type 非法值报错退出码非零。
	t.Run("--type 非法值", func(t *testing.T) {
		dir := t.TempDir()
		out, code := runInitBinary(t, dir, "-y", "-u", "测试", "--type", "移动端")
		if code == 0 {
			t.Errorf("--type 非法值应退出码非零，输出：\n%s", out)
		}
		if !strings.Contains(out, "无效的项目类型") {
			t.Errorf("输出应含“无效的项目类型”，实际：\n%s", out)
		}
		if _, err := os.Stat(filepath.Join(dir, ".sunshine")); !os.IsNotExist(err) {
			t.Error("--type 非法时不应生成 .sunshine/")
		}
	})
}
