package update

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/linrong007/Sunshine/internal/config"
)

func TestPreserveExistingRegistryConfig(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, ".sunshine"), 0o755); err != nil {
		t.Fatal(err)
	}
	template := "version: 0.1.0\n"
	// 无 registry 时原样返回。
	if got := PreserveExistingRegistryConfig(dir, template); got != template {
		t.Errorf("无 registry 时应原样返回，实际 %q", got)
	}
	if err := os.WriteFile(filepath.Join(dir, ".sunshine", "config.yaml"),
		[]byte("registry:\n  spec:\n    source: gh:owner/repo\n    template: my-spec\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	got := PreserveExistingRegistryConfig(dir, template)
	for _, want := range []string{"# Registry", "registry:", "  spec:", "    source: gh:owner/repo", "    template: my-spec", ".sunshine/spec", "sunshine update"} {
		if !strings.Contains(got, want) {
			t.Errorf("保留结果缺少 %q：\n%s", want, got)
		}
	}
	if strings.Contains(got, "trellis") || strings.Contains(got, "Trellis") {
		t.Errorf("保留结果残留 trellis 字样：\n%s", got)
	}
	// 再读一次：保留后的内容仍能被 LoadSpecRegistryConfig 读回
	// （与 B 组 writeSpecRegistryConfig 的字节一致性要求）。
	if err := os.WriteFile(filepath.Join(dir, ".sunshine", "config.yaml"), []byte(got), 0o644); err != nil {
		t.Fatal(err)
	}
	back, err := config.LoadSpecRegistryConfig(dir)
	if err != nil {
		t.Fatal(err)
	}
	if back == nil || back.Source != "gh:owner/repo" || back.Template != "my-spec" {
		t.Fatalf("保留后读回失败：%+v", back)
	}
}

func TestAppendMissingConfigSections(t *testing.T) {
	template := "# 头注释\n\n" +
		"#-------------------------------------------------------------------------------\n" +
		"# 会话记录\n" +
		"#-------------------------------------------------------------------------------\n\n" +
		"session_commit_message: \"chore: record journal\"\n\n" +
		"#-------------------------------------------------------------------------------\n" +
		"# 新段落\n" +
		"#-------------------------------------------------------------------------------\n\n" +
		"new_field: true\n"
	// 用户文件只有旧段落 + 自定义字段与注释 → 补新段落，自定义不动。
	user := "# 头注释\n\n" +
		"#-------------------------------------------------------------------------------\n" +
		"# 会话记录\n" +
		"#-------------------------------------------------------------------------------\n\n" +
		"session_commit_message: \"我的提交消息\"\n" +
		"# 我的注释\n" +
		"my_custom_field: hello\n"
	merged, appended := AppendMissingConfigSections(user, template)
	if len(appended) != 1 || appended[0] != "新段落" {
		t.Fatalf("应追加 1 个段落，实际 %v", appended)
	}
	for _, want := range []string{"我的提交消息", "# 我的注释", "my_custom_field: hello", "new_field: true", "# 新段落"} {
		if !strings.Contains(merged, want) {
			t.Errorf("合并结果缺少 %q：\n%s", want, merged)
		}
	}
	// 幂等：再跑一次不再追加。
	merged2, appended2 := AppendMissingConfigSections(merged, template)
	if len(appended2) != 0 || merged2 != merged {
		t.Errorf("重复追加应是空操作，实际 %v", appended2)
	}
	// 用户文件已齐全 → 原样返回。
	if got, appended := AppendMissingConfigSections(template, template); len(appended) != 0 || got != template {
		t.Errorf("齐全时应原样返回，实际 %v", appended)
	}
	// 用户手写 registry 块（无 "# Registry" 标题）不重复追加。
	templateWithRegistry := template + "\n#-------------------------------------------------------------------------------\n# Registry\n#-------------------------------------------------------------------------------\n\nregistry:\n  spec:\n    source: gh:owner/repo\n"
	userWithRegistry := user + "\nregistry:\n  spec:\n    source: gh:owner/repo\n"
	merged3, appended3 := AppendMissingConfigSections(userWithRegistry, templateWithRegistry)
	if len(appended3) != 1 || appended3[0] != "新段落" {
		t.Fatalf("手写 registry 不应重复追加，实际 %v", appended3)
	}
	if n := strings.Count(merged3, "registry:"); n != 1 {
		t.Errorf("registry 块应恰好 1 个，实际 %d 个：\n%s", n, merged3)
	}
}

func TestExtractConfigSections(t *testing.T) {
	template := "# 头\n\n" +
		"#-------------------------------------------------------------------------------\n" +
		"# 会话记录\n" +
		"#-------------------------------------------------------------------------------\n\n" +
		"# 注释行\n" +
		"session_commit_message: \"x\"\n" +
		"# session_auto_commit: true\n\n" +
		"#-------------------------------------------------------------------------------\n" +
		"# 会话自动提交\n" +
		"#-------------------------------------------------------------------------------\n\n" +
		"# session_auto_commit: true\n"
	sections := ExtractConfigSections(template)
	if len(sections) != 2 {
		t.Fatalf("应切出 2 个段落，实际 %d", len(sections))
	}
	if sections[0].Heading != "会话记录" || len(sections[0].Keys) != 1 || sections[0].Keys[0] != "session_commit_message" {
		t.Errorf("第一段解析错误：%+v", sections[0])
	}
	if sections[1].Heading != "会话自动提交" || len(sections[1].Keys) != 0 {
		t.Errorf("注释掉的键不应计入 Keys：%+v", sections[1])
	}
}
