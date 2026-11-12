package lifecycle

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/BondLam/Sunshine/templates"
)

func TestScrubHooksJSONNested(t *testing.T) {
	raw, err := templates.Platforms.ReadFile("platforms/claude/settings.json")
	if err != nil {
		t.Fatal(err)
	}
	// 用户自加一条 hook 与一个顶层键，另加一条 Sunshine statusline。
	root, err := parseOrderedJSON(raw)
	if err != nil {
		t.Fatal(err)
	}
	root.set("model", "opus")
	root.set("statusLine", func() any {
		obj := &orderedObject{vals: make(map[string]any)}
		obj.set("type", "command")
		obj.set("command", "SUNSHINE_HOOK_PLATFORM=claude sunshine hook statusline")
		return obj
	}())
	hooksVal, _ := root.get("hooks")
	hooksObj := hooksVal.(*orderedObject)
	sessionStart, _ := hooksObj.get("SessionStart")
	entries := sessionStart.([]any)
	userBlock := &orderedObject{vals: make(map[string]any)}
	userBlock.set("matcher", "startup")
	inner := &orderedObject{vals: make(map[string]any)}
	inner.set("command", "echo user-hook")
	inner.set("timeout", "5")
	inner.set("type", "command")
	userBlock.set("hooks", []any{inner})
	hooksObj.set("SessionStart", append(entries, userBlock))
	merged, err := root.marshalIndent()
	if err != nil {
		t.Fatal(err)
	}

	result := scrubHooksJSON(string(merged)+"\n", "nested")
	if result.FullyEmpty {
		t.Error("settings.json 还有 env/model 等键，不应判空")
	}
	after, err := parseOrderedJSON([]byte(result.Content))
	if err != nil {
		t.Fatalf("清理后内容应仍是合法 JSON：%v\n%s", err, result.Content)
	}
	if strings.Contains(result.Content, "sunshine hook") {
		t.Error("清理后内容里不应再有 Sunshine hook 命令")
	}
	if _, ok := after.get("statusLine"); ok {
		t.Error("Sunshine 的 statusLine 应已被删掉")
	}
	if v, ok := after.get("model"); !ok || v != "opus" {
		t.Errorf("用户顶层键 model 应保留，实际 %v", v)
	}
	if _, ok := after.get("env"); !ok {
		t.Error("env 应保留")
	}
	// 用户自加的 hook 条目（echo user-hook）应在 hooks 里保留——
	// 但本例里 SessionStart 只剩用户条目，hooks 键应还在。
	hooksAfter, ok := after.get("hooks")
	if !ok {
		t.Fatal("用户 hook 条目还在时 hooks 键不应被删")
	}
	hooksAfterObj := hooksAfter.(*orderedObject)
	sessionAfter, ok := hooksAfterObj.get("SessionStart")
	if !ok {
		t.Fatal("SessionStart 事件应保留（有用户条目）")
	}
	found := false
	for _, entry := range sessionAfter.([]any) {
		block, ok := entry.(*orderedObject)
		if !ok {
			continue
		}
		innerVal, _ := block.get("hooks")
		for _, sub := range innerVal.([]any) {
			if entryCommand(sub) == "echo user-hook" {
				found = true
			}
		}
	}
	if !found {
		t.Error("用户自加的 hook 条目应保留")
	}
	// 模板原样清理：只剩 env + enabledPlugins，不判空。
	plain := scrubHooksJSON(string(raw), "nested")
	if plain.FullyEmpty {
		t.Error("模板 settings.json 清理后还有 env/enabledPlugins，不应判空")
	}
	plainObj, err := parseOrderedJSON([]byte(plain.Content))
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := plainObj.get("hooks"); ok {
		t.Error("模板 settings.json 的 hooks 应已被剥离")
	}
}

func TestScrubHooksJSONFlat(t *testing.T) {
	raw, err := templates.Platforms.ReadFile("platforms/cursor/hooks.json")
	if err != nil {
		t.Fatal(err)
	}
	// Cursor hooks.json 除 hooks 外还有 version 键，
	// 清理后只剩 {"version": 1} 的壳，不判空（上游同理：
	// fullyEmpty 只看顶层键是否全空）。
	result := scrubHooksJSON(string(raw), "flat")
	if result.FullyEmpty {
		t.Error("Cursor hooks.json 清理后还有 version 键，不应判空")
	}
	if !strings.Contains(result.Content, `"version": 1`) || strings.Contains(result.Content, "sunshine hook") {
		t.Errorf("清理后应只剩 version 键，实际：\n%s", result.Content)
	}
	// 只有 hooks 键时清理后判空。
	onlyHooks := `{"hooks": {"sessionStart": [{"command": "SUNSHINE_HOOK_PLATFORM=cursor sunshine hook session-start", "timeout": 30}]}}`
	onlyResult := scrubHooksJSON(onlyHooks, "flat")
	if !onlyResult.FullyEmpty || strings.TrimSpace(onlyResult.Content) != "{}" {
		t.Errorf("只有 hooks 键时清理后应判空为 {}，实际：%q", onlyResult.Content)
	}
	// 用户条目保留。
	root, err := parseOrderedJSON(raw)
	if err != nil {
		t.Fatal(err)
	}
	hooksVal, _ := root.get("hooks")
	hooksObj := hooksVal.(*orderedObject)
	hooksObj.set("sessionStart", []any{
		func() any {
			o := &orderedObject{vals: make(map[string]any)}
			o.set("command", "echo user-hook")
			o.set("timeout", "5")
			return o
		}(),
	})
	merged, err := root.marshalIndent()
	if err != nil {
		t.Fatal(err)
	}
	result2 := scrubHooksJSON(string(merged)+"\n", "flat")
	if result2.FullyEmpty {
		t.Error("有用户 hook 条目时不应判空")
	}
	if !strings.Contains(result2.Content, "echo user-hook") {
		t.Errorf("用户 hook 条目应保留，实际：\n%s", result2.Content)
	}
	if strings.Contains(result2.Content, "sunshine hook") {
		t.Error("Sunshine hook 条目应已被剥离")
	}
	// 非法 JSON 原样不动。
	bad := scrubHooksJSON("{not json", "flat")
	if bad.Content != "{not json" || bad.FullyEmpty {
		t.Errorf("非法 JSON 应原样不动，实际 %+v", bad)
	}
}

func TestScrubCodexConfigToml(t *testing.T) {
	raw, err := templates.Platforms.ReadFile("platforms/codex/config.toml")
	if err != nil {
		t.Fatal(err)
	}
	result := scrubCodexConfigToml(string(raw))
	if !result.FullyEmpty {
		t.Errorf("模板 config.toml 清理后应判空，实际：\n%s", result.Content)
	}
	if result.Content != "" {
		t.Errorf("判空时内容应为空，实际 %q", result.Content)
	}
	// 用户自有行保留。
	withUser := string(raw) + "\nmy_option = \"keep me\"\n"
	result2 := scrubCodexConfigToml(withUser)
	if result2.FullyEmpty {
		t.Error("有用户自有行时不应判空")
	}
	if !strings.Contains(result2.Content, `my_option = "keep me"`) {
		t.Errorf("用户行应保留，实际：\n%s", result2.Content)
	}
	if strings.Contains(result2.Content, "project_doc_fallback_filenames") {
		t.Error("Sunshine 的 project_doc_fallback_filenames 行应被删掉")
	}
	// [agents] 表里有用户键时保留表头、只删 max_depth。
	table := "[agents]\nmax_depth = 1\nmodel = \"gpt-5\"\n"
	result3 := scrubCodexConfigToml(table)
	if !strings.Contains(result3.Content, "[agents]") || !strings.Contains(result3.Content, `model = "gpt-5"`) {
		t.Errorf("[agents] 表有用户键时应保留表头与用户键，实际：\n%s", result3.Content)
	}
	if strings.Contains(result3.Content, "max_depth") {
		t.Error("max_depth = 1 应被删掉")
	}
	// [agents] 表只有 max_depth 时整表去掉。
	result4 := scrubCodexConfigToml("[agents]\nmax_depth = 1\n")
	if result4.Content != "" || !result4.FullyEmpty {
		t.Errorf("[agents] 表只有 Sunshine 键时应整表去掉，实际 %q", result4.Content)
	}
}

func TestValidateManagedRelativePath(t *testing.T) {
	valid := []string{".claude/settings.json", ".sunshine/workflow.md", "AGENTS.md"}
	for _, p := range valid {
		if err := ValidateManagedRelativePath(p); err != nil {
			t.Errorf("%q 应合法：%v", p, err)
		}
	}
	invalid := []string{"", "/abs/path", "../escape", "a/../b", "a//b", `a\b`, "a/./b", "C:/x"}
	for _, p := range invalid {
		if err := ValidateManagedRelativePath(p); err == nil {
			t.Errorf("%q 应不合法", p)
		}
	}
}

func TestParseAblationStateRejectsUnknown(t *testing.T) {
	// schema 版本不对、能力标识不对、条目为空都应拒绝。
	cases := []string{
		`{"schemaVersion": 2, "status": "applied", "projectRoot": "/tmp/x", "projectKey": "` + strings.Repeat("a", 64) + `", "sunshineVersion": "0.1.0", "capabilities": ["sunshine.full"], "createdAt": "2026-11-09T00:00:00Z", "configuredPlatforms": [], "manifest": {}, "entries": [{"relativePath": ".sunshine", "pre": {"kind": "directory", "sha256": "` + strings.Repeat("b", 64) + `", "mode": 493}, "expectedAblated": {"kind": "absent"}, "backupPath": "backup/.sunshine"}]}`,
		`{"schemaVersion": 1, "status": "applied", "projectRoot": "/tmp/x", "projectKey": "` + strings.Repeat("a", 64) + `", "sunshineVersion": "0.1.0", "capabilities": ["other"], "createdAt": "2026-11-09T00:00:00Z", "configuredPlatforms": [], "manifest": {}, "entries": [{"relativePath": ".sunshine", "pre": {"kind": "absent"}, "expectedAblated": {"kind": "absent"}}]}`,
	}
	for i, raw := range cases {
		if _, err := ParseAblationState([]byte(raw)); err == nil {
			t.Errorf("用例 %d 应被拒绝", i)
		}
	}
	// 正常态应通过。
	good := `{"schemaVersion": 1, "status": "applied", "projectRoot": "/tmp/x", "projectKey": "` + strings.Repeat("a", 64) + `", "sunshineVersion": "0.1.0", "capabilities": ["sunshine.full"], "createdAt": "2026-11-09T00:00:00Z", "configuredPlatforms": ["codex"], "manifest": {}, "entries": [{"relativePath": ".sunshine", "pre": {"kind": "directory", "sha256": "` + strings.Repeat("b", 64) + `", "mode": 493}, "expectedAblated": {"kind": "absent"}, "backupPath": "backup/.sunshine"}]}`
	if _, err := ParseAblationState([]byte(good)); err != nil {
		t.Errorf("正常事务应通过校验：%v", err)
	}
}

func TestHomedirGuard(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Skip("取不到主目录")
	}
	if !IsHomeDir(home) {
		t.Error("主目录本身应被判定为 IsHomeDir")
	}
	if IsHomeDir(filepath.Join(home, "some-subdir")) {
		t.Error("主目录子目录不应被判定为 IsHomeDir")
	}
	if err := AssertNotHomedir(home, "uninstall"); err == nil {
		t.Error("主目录下应触发守卫")
	}
	t.Setenv("SUNSHINE_ALLOW_HOMEDIR", "1")
	if err := AssertNotHomedir(home, "uninstall"); err != nil {
		t.Errorf("设置绕过后不应触发守卫：%v", err)
	}
}
