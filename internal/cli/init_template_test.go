package cli

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/BondLam/Sunshine/internal/registry"
)

// testMarketIndex 是夹具市场索引：1 spec + 1 skill + 1 workflow
// （与 internal/registry/download_test.go 的 marketplaceIndex 同形）。
const testMarketIndex = `{"version": 1, "templates": [
	{"id": "demo-spec", "type": "spec", "name": "演示规范", "description": "演示用 spec 模板", "path": "specs/demo-spec"},
	{"id": "demo-skill", "type": "skill", "name": "演示 Skill", "description": "演示用 skill", "path": "skills/demo-skill"},
	{"id": "native", "type": "workflow", "name": "原生工作流", "description": "演示用工作流", "path": "workflows/native/workflow.md"}
]}`

// testMarketFiles 是夹具市场仓库的文件（仓库根布局）。
func testMarketFiles() map[string]string {
	return map[string]string{
		"index.json":                  testMarketIndex,
		"specs/demo-spec/backend.md":  "# 后端规范\n",
		"specs/demo-spec/frontend.md": "# 前端规范\n",
		"skills/demo-skill/SKILL.md":  "# 演示 Skill\n",
		"README.md":                   "# 演示市场\n",
	}
}

// runTestGit 在 dir 里跑 git 命令（测试夹具用，要求环境有 git）。
func runTestGit(t *testing.T, dir string, args ...string) {
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

// initTestGitRepo 在 dir 建一个 git 仓库并提交 files（相对路径 → 内容）。
func initTestGitRepo(t *testing.T, dir string, files map[string]string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	runTestGit(t, dir, "init", "-b", "main")
	for rel, content := range files {
		p := filepath.Join(dir, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	runTestGit(t, dir, "add", "-A")
	runTestGit(t, dir, "commit", "-m", "init")
}

// setFakeTemplatePrompts 用脚本答案替换选模板的两个提问函数，测试结束恢复。
// selects 按提问顺序给出每次选单的答案（答案必须在当次选项里）；
// inputs 按顺序给出每次自定义来源输入的答案。selectErr 非空时选单提问
// 一律返回该错误。
func setFakeTemplatePrompts(t *testing.T, selects []string, inputs []string, selectErr error) {
	t.Helper()
	origSelect := promptSpecTemplate
	origInput := promptCustomRegistrySource
	t.Cleanup(func() {
		promptSpecTemplate = origSelect
		promptCustomRegistrySource = origInput
	})
	si, ii := 0, 0
	promptSpecTemplate = func(options []string, def string) (string, error) {
		if selectErr != nil {
			return "", selectErr
		}
		if si >= len(selects) {
			t.Fatalf("选单提问次数超出脚本：options=%v", options)
		}
		want := selects[si]
		si++
		for _, o := range options {
			if o == want {
				return want, nil
			}
		}
		t.Fatalf("选单选项 %v 里没有脚本答案 %q", options, want)
		return "", nil
	}
	promptCustomRegistrySource = func() (string, error) {
		if ii >= len(inputs) {
			t.Fatalf("自定义来源提问次数超出脚本")
		}
		v := inputs[ii]
		ii++
		return v, nil
	}
}

func TestSpecTemplateChoices(t *testing.T) {
	templates := []registry.SpecTemplate{
		{ID: "a-spec", Type: "spec", Name: "甲规范"},
		{ID: "b-spec", Type: "spec", Name: ""},
		{ID: "c-skill", Type: "skill", Name: "某 Skill"},
		{ID: "d-workflow", Type: "workflow", Name: "某工作流"},
	}
	choices := specTemplateChoices(templates)
	if len(choices) != 2 {
		t.Fatalf("choices = %v，想得到 2 个（只留 spec 型）", choices)
	}
	if choices[0].label != "a-spec（甲规范）" || choices[0].id != "a-spec" {
		t.Errorf("choices[0] = %+v", choices[0])
	}
	if choices[1].label != "b-spec" || choices[1].id != "b-spec" {
		t.Errorf("无 name 时标签应只写 id，choices[1] = %+v", choices[1])
	}
	if got := templateChoiceID(choices, "a-spec（甲规范）"); got != "a-spec" {
		t.Errorf("templateChoiceID = %q，想得到 a-spec", got)
	}
	if got := templateChoiceID(choices, "不存在"); got != "" {
		t.Errorf("templateChoiceID（不存在的标签）= %q，想得到空", got)
	}
	if got := specTemplateChoices(nil); len(got) != 0 {
		t.Errorf("空输入应得空选单，got = %v", got)
	}
}

func TestResolveInteractiveTemplate_Marketplace(t *testing.T) {
	repo := filepath.Join(t.TempDir(), "market")
	initTestGitRepo(t, repo, testMarketFiles())
	setFakeTemplatePrompts(t, []string{"demo-spec（演示规范）"}, nil, nil)

	source, templateID, target, err := resolveInteractiveTemplate(repo)
	if err != nil {
		t.Fatalf("resolveInteractiveTemplate 失败：%v", err)
	}
	if target == nil {
		t.Fatal("应返回 Target")
	}
	defer target.Close()
	if source != repo || templateID != "demo-spec" {
		t.Errorf("source = %q，templateID = %q", source, templateID)
	}
	if target.Mode != "marketplace" || target.Template == nil || target.Template.ID != "demo-spec" {
		t.Errorf("target = %+v", target)
	}
	if !target.WritesSpec() {
		t.Error("spec 型模板应 WritesSpec")
	}
}

func TestResolveInteractiveTemplate_Direct(t *testing.T) {
	repo := filepath.Join(t.TempDir(), "direct")
	initTestGitRepo(t, repo, map[string]string{"system.md": "# 规范\n"})
	setFakeTemplatePrompts(t, nil, nil, nil)

	source, templateID, target, err := resolveInteractiveTemplate(repo)
	if err != nil {
		t.Fatalf("resolveInteractiveTemplate 失败：%v", err)
	}
	if target == nil {
		t.Fatal("直接下载模式应返回 Target")
	}
	defer target.Close()
	if source != repo || templateID != "" {
		t.Errorf("source = %q，templateID = %q", source, templateID)
	}
	if target.Mode != "direct" || !target.WritesSpec() {
		t.Errorf("target = %+v", target)
	}
}

func TestResolveInteractiveTemplate_Blank(t *testing.T) {
	setFakeTemplatePrompts(t, []string{blankTemplateLabel}, nil, nil)

	source, templateID, target, err := resolveInteractiveTemplate("")
	if err != nil {
		t.Fatalf("resolveInteractiveTemplate 失败：%v", err)
	}
	if source != "" || templateID != "" || target != nil {
		t.Errorf("选空白应三者皆空，source = %q，templateID = %q，target = %v", source, templateID, target)
	}
}

func TestResolveInteractiveTemplate_CustomSource(t *testing.T) {
	repo := filepath.Join(t.TempDir(), "market")
	initTestGitRepo(t, repo, testMarketFiles())
	setFakeTemplatePrompts(t,
		[]string{customTemplateLabel, "demo-spec（演示规范）"},
		[]string{repo}, nil)

	source, templateID, target, err := resolveInteractiveTemplate("")
	if err != nil {
		t.Fatalf("resolveInteractiveTemplate 失败：%v", err)
	}
	if target == nil {
		t.Fatal("应返回 Target")
	}
	defer target.Close()
	if source != repo || templateID != "demo-spec" {
		t.Errorf("source = %q，templateID = %q", source, templateID)
	}
	if target.Mode != "marketplace" {
		t.Errorf("target = %+v", target)
	}
}

func TestResolveInteractiveTemplate_CustomEmptyBackToPicker(t *testing.T) {
	setFakeTemplatePrompts(t,
		[]string{customTemplateLabel, blankTemplateLabel},
		[]string{""}, nil)

	source, templateID, target, err := resolveInteractiveTemplate("")
	if err != nil {
		t.Fatalf("resolveInteractiveTemplate 失败：%v", err)
	}
	if source != "" || templateID != "" || target != nil {
		t.Errorf("空输入回选单再选空白应三者皆空，source = %q，templateID = %q，target = %v", source, templateID, target)
	}
}

func TestResolveInteractiveTemplate_CustomInvalidSourceBackToPicker(t *testing.T) {
	setFakeTemplatePrompts(t,
		[]string{customTemplateLabel, blankTemplateLabel},
		[]string{"::::bad"}, nil)

	source, templateID, target, err := resolveInteractiveTemplate("")
	if err != nil {
		t.Fatalf("无效来源应回选单而不是报错，err = %v", err)
	}
	if source != "" || templateID != "" || target != nil {
		t.Errorf("无效来源回选单再选空白应三者皆空，source = %q，templateID = %q，target = %v", source, templateID, target)
	}
}

func TestResolveInteractiveTemplate_NoSpecTemplates(t *testing.T) {
	repo := filepath.Join(t.TempDir(), "market")
	initTestGitRepo(t, repo, map[string]string{
		"index.json": `{"version": 1, "templates": [
			{"id": "only-skill", "type": "skill", "name": "只有 Skill", "path": "skills/only-skill"}
		]}`,
		"skills/only-skill/SKILL.md": "# Skill\n",
	})
	setFakeTemplatePrompts(t, nil, nil, nil)

	source, templateID, target, err := resolveInteractiveTemplate(repo)
	if err != nil {
		t.Fatalf("resolveInteractiveTemplate 失败：%v", err)
	}
	if source != "" || templateID != "" || target != nil {
		t.Errorf("市场无 spec 模板应改用空白，source = %q，templateID = %q，target = %v", source, templateID, target)
	}
}

func TestResolveInteractiveTemplate_FlagSourceProbeError(t *testing.T) {
	// 本地路径存在但不是 git 仓库：解析通过，探测克隆失败；
	// --registry 显式给的来源应直接报错（与上游一致），不回选单。
	dir := filepath.Join(t.TempDir(), "not-a-repo")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "x.md"), []byte("# x\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	setFakeTemplatePrompts(t, nil, nil, nil)

	_, _, _, err := resolveInteractiveTemplate(dir)
	if err == nil {
		t.Fatal("--registry 来源探测失败应返回错误")
	}
}

func TestResolveInteractiveTemplate_FlagSourceInvalid(t *testing.T) {
	setFakeTemplatePrompts(t, nil, nil, nil)

	_, _, _, err := resolveInteractiveTemplate("::::bad")
	if err == nil {
		t.Fatal("--registry 来源格式无效应返回错误")
	}
}

func TestResolveInteractiveTemplate_SelectError(t *testing.T) {
	setFakeTemplatePrompts(t, nil, nil, errors.New("EOF"))

	_, _, _, err := resolveInteractiveTemplate("")
	if err == nil {
		t.Fatal("提问出错应向上传")
	}
}
