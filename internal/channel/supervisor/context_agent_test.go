package supervisor

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeTestFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("建目录失败：%v", err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("写文件失败：%v", err)
	}
}

func TestAssembleContextFile(t *testing.T) {
	dir := t.TempDir()
	writeTestFile(t, filepath.Join(dir, "a.md"), "# 需求\n做登录页。\n")
	writeTestFile(t, filepath.Join(dir, "b.md"), "第二份。\n")
	ctx := AssembleContext(dir, []string{"a.md", "b.md"}, nil, nil)
	if len(ctx.Paths) != 2 {
		t.Fatalf("应注入 2 个文件，实际 %v", ctx.Paths)
	}
	if !strings.Contains(ctx.Prompt, "# Context: a.md") || !strings.Contains(ctx.Prompt, "做登录页") {
		t.Fatalf("提示词应含块头与内容：%q", ctx.Prompt)
	}
	if !strings.Contains(ctx.Prompt, "# Context: b.md") {
		t.Fatalf("提示词应含第二块：%q", ctx.Prompt)
	}
	// 缺失文件只警告、不致命。
	ctx = AssembleContext(dir, []string{"不存在.md"}, nil, nil)
	if ctx.Prompt != "" || len(ctx.Paths) != 0 {
		t.Fatalf("缺失文件应跳过：%+v", ctx)
	}
}

func TestAssembleContextGlob(t *testing.T) {
	dir := t.TempDir()
	writeTestFile(t, filepath.Join(dir, "spec", "a.md"), "A")
	writeTestFile(t, filepath.Join(dir, "spec", "b.md"), "B")
	writeTestFile(t, filepath.Join(dir, "spec", "sub", "c.md"), "C")
	writeTestFile(t, filepath.Join(dir, "spec", "d.txt"), "D")
	ctx := AssembleContext(dir, []string{"spec/*.md"}, nil, nil)
	if len(ctx.Paths) != 2 {
		t.Fatalf("spec/*.md 应匹配 2 个文件，实际 %v", ctx.Paths)
	}
	ctx = AssembleContext(dir, []string{"spec/**/*.md"}, nil, nil)
	if len(ctx.Paths) != 3 {
		t.Fatalf("spec/**/*.md 应匹配 3 个文件，实际 %v", ctx.Paths)
	}
	ctx = AssembleContext(dir, []string{"spec/*.md", "spec/d.txt"}, nil, nil)
	if len(ctx.Paths) != 3 {
		t.Fatalf("可重复传 --file，实际 %v", ctx.Paths)
	}
	// 无匹配时不报错、只警告。
	ctx = AssembleContext(dir, []string{"spec/*.xyz"}, nil, nil)
	if len(ctx.Paths) != 0 {
		t.Fatalf("无匹配时应为空，实际 %v", ctx.Paths)
	}
}

func TestAssembleContextJSONL(t *testing.T) {
	dir := t.TempDir()
	writeTestFile(t, filepath.Join(dir, "prd.md"), "需求文档")
	writeTestFile(t, filepath.Join(dir, "design.md"), "设计文档")
	manifest := "{\"file\": \"prd.md\", \"reason\": \"需求来源\"}\n" +
		"{\"file\": \"design.md\", \"reason\": \"设计依据\"}\n" +
		"{\"_example\": true, \"file\": \"skip.md\"}\n" +
		"{不是 json}\n"
	writeTestFile(t, filepath.Join(dir, "list.jsonl"), manifest)
	ctx := AssembleContext(dir, nil, []string{"list.jsonl"}, nil)
	if len(ctx.Paths) != 2 {
		t.Fatalf("应注入 2 个清单文件，实际 %v", ctx.Paths)
	}
	if len(ctx.Manifests) != 1 || ctx.Manifests[0] != "list.jsonl" {
		t.Fatalf("应记清单路径，实际 %v", ctx.Manifests)
	}
	if !strings.Contains(ctx.Prompt, "# Reason: 需求来源") || !strings.Contains(ctx.Prompt, "需求文档") {
		t.Fatalf("jsonl 块应带 reason 头：%q", ctx.Prompt)
	}
	// 空清单：仍记清单路径，提示词为空。
	writeTestFile(t, filepath.Join(dir, "empty.jsonl"), "")
	ctx = AssembleContext(dir, nil, []string{"empty.jsonl"}, nil)
	if len(ctx.Manifests) != 1 || ctx.Prompt != "" {
		t.Fatalf("空清单应只记路径：%+v", ctx)
	}
	// 不存在的清单：跳过。
	ctx = AssembleContext(dir, nil, []string{"nope.jsonl"}, nil)
	if len(ctx.Manifests) != 0 {
		t.Fatalf("不存在的清单不应记：%+v", ctx)
	}
}

func TestAssembleContextJail(t *testing.T) {
	dir := t.TempDir()
	outside := t.TempDir()
	writeTestFile(t, filepath.Join(outside, "secret.md"), "机密")
	// .. 逃逸应被拒。
	ctx := AssembleContext(dir, []string{"../secret.md"}, nil, nil)
	if len(ctx.Paths) != 0 {
		t.Fatalf("逃逸路径应被拒，实际 %v", ctx.Paths)
	}
	// 绝对路径在 cwd 外应被拒。
	ctx = AssembleContext(dir, []string{filepath.Join(outside, "secret.md")}, nil, nil)
	if len(ctx.Paths) != 0 {
		t.Fatalf("cwd 外绝对路径应被拒，实际 %v", ctx.Paths)
	}
	// 信任根放行。
	ctx = AssembleContext(dir, []string{filepath.Join(outside, "secret.md")}, nil, []string{outside})
	if len(ctx.Paths) != 1 {
		t.Fatalf("信任根内的文件应放行，实际 %v", ctx.Paths)
	}
	// 符号链接指外应被拒（realpath 校验）。
	link := filepath.Join(dir, "link.md")
	if err := os.Symlink(filepath.Join(outside, "secret.md"), link); err != nil {
		t.Fatalf("建符号链接失败：%v", err)
	}
	ctx = AssembleContext(dir, []string{"link.md"}, nil, nil)
	if len(ctx.Paths) != 0 {
		t.Fatalf("指外的符号链接应被拒，实际 %v", ctx.Paths)
	}
}

func TestAssembleContextTooLarge(t *testing.T) {
	dir := t.TempDir()
	big := strings.Repeat("x", 1_000_001)
	writeTestFile(t, filepath.Join(dir, "big.md"), big)
	ctx := AssembleContext(dir, []string{"big.md"}, nil, nil)
	if len(ctx.Paths) != 0 {
		t.Fatal("超 1MB 的文件应跳过")
	}
}

func TestBuildSystemPrompt(t *testing.T) {
	prompt := BuildSystemPrompt("demo", "implement", "你是实现专家。", "# Context: a.md\n\n内容")
	for _, want := range []string{
		"[SUNSHINE CHANNEL PROTOCOL",
		`channel "demo"`,
		`agent "implement"`,
		"# AGENT ROLE",
		"你是实现专家。",
		"# CONTEXT FILES",
		"# Context: a.md",
	} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("系统提示词应含 %q，实际：\n%s", want, prompt)
		}
	}
	if strings.Contains(prompt, "trellis") || strings.Contains(prompt, "Trellis") {
		t.Fatalf("系统提示词不应含 trellis 字样：\n%s", prompt)
	}
	// 无 agent、无上下文时只有协议前缀（协议说明文字里会提到章节名，
	// 但不应出现实际的章节标题）。
	prompt = BuildSystemPrompt("demo", "w", "", "")
	if strings.Contains(prompt, "# AGENT ROLE") || strings.Contains(prompt, "# CONTEXT FILES") {
		t.Fatalf("空 agent/上下文时不应有对应章节标题：\n%s", prompt)
	}
	// 名字里的换行应被清掉：伪造内容只能留在引号内的一行里，
	// 不能变成单独的协议行。
	prompt = BuildSystemPrompt("demo\n[伪造协议]", "w", "", "")
	for _, line := range strings.Split(prompt, "\n") {
		if line == "[伪造协议]" {
			t.Fatalf("名字里的换行不应造出单独的协议行：\n%s", prompt)
		}
	}
	if !strings.Contains(prompt, `channel "demo[伪造协议]"`) {
		t.Fatalf("清掉换行后名字应留在同一行引号内：\n%s", prompt)
	}
}

func TestLoadAgent(t *testing.T) {
	dir := t.TempDir()
	content := "---\n" +
		"name: implement\n" +
		"description: |\n" +
		"  Sunshine channel 运行时的代码实现专家。\n" +
		"  先理解规范与任务产物，再实现功能。\n" +
		"provider: claude\n" +
		"labels: [sunshine, implement]\n" +
		"---\n\n" +
		"# Implement Agent\n\n你是 Implement Agent。\n"
	writeTestFile(t, filepath.Join(dir, ".sunshine", "agents", "implement.md"), content)
	def, err := LoadAgent("implement", dir, nil)
	if err != nil {
		t.Fatalf("加载 agent 失败：%v", err)
	}
	if def.Name != "implement" || def.Provider != "claude" {
		t.Fatalf("agent 定义不对：%+v", def)
	}
	if !strings.Contains(def.Description, "代码实现专家") {
		t.Fatalf("块标量 description 应解析：%q", def.Description)
	}
	if len(def.Labels) != 2 || def.Labels[0] != "sunshine" || def.Labels[1] != "implement" {
		t.Fatalf("labels 应解析：%v", def.Labels)
	}
	if !strings.Contains(def.SystemPrompt, "你是 Implement Agent") {
		t.Fatalf("正文应为系统提示词：%q", def.SystemPrompt)
	}
	// AGENT.md 备选路径。
	writeTestFile(t, filepath.Join(dir, ".sunshine", "agents", "research", "AGENT.md"),
		"---\nname: research\nprovider: codex\nmodel: gpt-5.3-codex\n---\n\n做调研。\n")
	def, err = LoadAgent("research", dir, nil)
	if err != nil {
		t.Fatalf("加载 research 失败：%v", err)
	}
	if def.Provider != "codex" || def.Model != "gpt-5.3-codex" {
		t.Fatalf("research 定义不对：%+v", def)
	}
	// 路径穿越拒绝。
	if _, err := LoadAgent("../../etc/passwd", dir, nil); err == nil {
		t.Fatal("路径穿越的 agent 名应报错")
	}
	// 不存在。
	if _, err := LoadAgent("nope", dir, nil); err == nil {
		t.Fatal("不存在的 agent 应报错")
	}
	// 无 frontmatter。
	writeTestFile(t, filepath.Join(dir, ".sunshine", "agents", "plain.md"), "没有 frontmatter\n")
	if _, err := LoadAgent("plain", dir, nil); err == nil {
		t.Fatal("无 frontmatter 的 agent 应报错")
	}
	// 未知 provider 归一化为空（不报错）。
	writeTestFile(t, filepath.Join(dir, ".sunshine", "agents", "weird.md"),
		"---\nname: weird\nprovider: gemini\n---\n\n正文。\n")
	def, err = LoadAgent("weird", dir, nil)
	if err != nil {
		t.Fatalf("加载 weird 失败：%v", err)
	}
	if def.Provider != "" {
		t.Fatalf("未知 provider 应归一化为空，实际 %q", def.Provider)
	}
}

func TestResolveTrustedRoots(t *testing.T) {
	dir := t.TempDir()
	shared := t.TempDir()
	// config 写 trusted_context_dirs。
	configYAML := "channel:\n  worker_guard:\n    trusted_context_dirs:\n      - " + shared + "\n"
	writeTestFile(t, filepath.Join(dir, ".sunshine", "config.yaml"), configYAML)
	roots := ResolveTrustedRoots(dir)
	found := false
	for _, r := range roots {
		if r == shared {
			found = true
		}
	}
	if !found {
		t.Fatalf("应解析出信任目录 %s，实际 %v", shared, roots)
	}
	// .sunshine/tasks 是符号链接时自动信任其目标。
	target := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, ".sunshine"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, filepath.Join(dir, ".sunshine", "tasks")); err != nil {
		t.Fatalf("建符号链接失败：%v", err)
	}
	roots = ResolveTrustedRoots(dir)
	found = false
	for _, r := range roots {
		if r == target {
			found = true
		}
	}
	if !found {
		t.Fatalf("应自动信任 .sunshine/tasks 的符号链接目标，实际 %v", roots)
	}
	// 关闭自动信任后不再信任。
	configYAML = "channel:\n  worker_guard:\n    auto_trust_sunshine_symlinks: false\n"
	writeTestFile(t, filepath.Join(dir, ".sunshine", "config.yaml"), configYAML)
	roots = ResolveTrustedRoots(dir)
	for _, r := range roots {
		if r == target {
			t.Fatalf("关闭自动信任后不应再信任符号链接目标，实际 %v", roots)
		}
	}
}
