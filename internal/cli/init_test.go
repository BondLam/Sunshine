package cli

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestExplicitPlatformIDs(t *testing.T) {
	cases := []struct {
		flags map[string]bool
		want  []string
	}{
		{map[string]bool{}, nil},
		{map[string]bool{"claude-code": true}, []string{"claude-code"}},
		{map[string]bool{"codex": true}, []string{"codex"}},
		{map[string]bool{"claude-code": true, "cursor": true, "codex": true}, []string{"claude-code", "cursor", "codex"}},
		{map[string]bool{"cursor": true, "codex": true}, []string{"cursor", "codex"}},
		{map[string]bool{"devin": true}, []string{"devin"}},
		{map[string]bool{"kiro": true, "gemini": true, "devin": true}, []string{"kiro", "gemini", "devin"}},
	}
	for _, c := range cases {
		got := explicitPlatformIDs(c.flags)
		if !reflect.DeepEqual(got, c.want) {
			t.Errorf("explicitPlatformIDs(%v) = %v，想得到 %v", c.flags, got, c.want)
		}
	}
}

// stubPrompts 用测试替身替换交互提问，返回恢复函数。
func stubPrompts(t *testing.T, platformsFn func([]string) ([]string, error), statuslineFn func() (bool, error)) {
	t.Helper()
	oldPlatforms, oldStatusline := promptPlatforms, promptStatusline
	promptPlatforms, promptStatusline = platformsFn, statuslineFn
	t.Cleanup(func() {
		promptPlatforms, promptStatusline = oldPlatforms, oldStatusline
	})
}

func TestResolveSelectionExplicitWinsOverYes(t *testing.T) {
	// 显式 flag 优先于 -y，且 -y 下不追问 statusline。
	stubPrompts(t,
		func([]string) ([]string, error) { t.Error("显式选择时不应弹出多选"); return nil, nil },
		func() (bool, error) { t.Error("-y 时不应追问 statusline"); return false, nil },
	)
	selected, withStatusline, err := resolveSelection(platformSelectionInput{
		explicit: []string{"codex"},
		yes:      true,
		isTTY:    true,
	})
	if err != nil {
		t.Fatalf("resolveSelection 失败：%v", err)
	}
	if !reflect.DeepEqual(selected, []string{"codex"}) {
		t.Errorf("selected = %v，想得到 [codex]", selected)
	}
	if withStatusline {
		t.Error("未选 Claude 时 withStatusline 应为 false")
	}
}

func TestResolveSelectionYesUsesDefaults(t *testing.T) {
	stubPrompts(t,
		func([]string) ([]string, error) { t.Error("-y 时不应弹出多选"); return nil, nil },
		func() (bool, error) { t.Error("-y 时不应追问 statusline"); return false, nil },
	)
	selected, withStatusline, err := resolveSelection(platformSelectionInput{
		yes:   true,
		isTTY: true,
	})
	if err != nil {
		t.Fatalf("resolveSelection 失败：%v", err)
	}
	if !reflect.DeepEqual(selected, []string{"claude-code", "cursor"}) {
		t.Errorf("selected = %v，想得到 [claude-code cursor]", selected)
	}
	if withStatusline {
		t.Error("-y 时 withStatusline 应为 false")
	}
}

func TestResolveSelectionNonTTYFallsBackToDefaults(t *testing.T) {
	stubPrompts(t,
		func([]string) ([]string, error) { t.Error("非 TTY 时不应弹出多选"); return nil, nil },
		func() (bool, error) { t.Error("非 TTY 时不应追问 statusline"); return false, nil },
	)
	selected, withStatusline, err := resolveSelection(platformSelectionInput{
		isTTY: false,
	})
	if err != nil {
		t.Fatalf("resolveSelection 失败：%v", err)
	}
	if !reflect.DeepEqual(selected, []string{"claude-code", "cursor"}) {
		t.Errorf("selected = %v，想得到 [claude-code cursor]", selected)
	}
	if withStatusline {
		t.Error("非 TTY 时 withStatusline 应为 false")
	}
}

func TestResolveSelectionInteractiveAsksStatuslineOnlyForClaude(t *testing.T) {
	// 交互选了 Claude：追问 statusline，回答是则装。
	stubPrompts(t,
		func([]string) ([]string, error) { return []string{"claude-code", "codex"}, nil },
		func() (bool, error) { return true, nil },
	)
	selected, withStatusline, err := resolveSelection(platformSelectionInput{isTTY: true})
	if err != nil {
		t.Fatalf("resolveSelection 失败：%v", err)
	}
	if !reflect.DeepEqual(selected, []string{"claude-code", "codex"}) {
		t.Errorf("selected = %v", selected)
	}
	if !withStatusline {
		t.Error("交互确认后 withStatusline 应为 true")
	}

	// 交互只选 Codex：不追问 statusline。
	statuslineAsked := false
	stubPrompts(t,
		func([]string) ([]string, error) { return []string{"codex"}, nil },
		func() (bool, error) { statuslineAsked = true; return false, nil },
	)
	selected, withStatusline, err = resolveSelection(platformSelectionInput{isTTY: true})
	if err != nil {
		t.Fatalf("resolveSelection 失败：%v", err)
	}
	if statuslineAsked {
		t.Error("未选 Claude 时不应追问 statusline")
	}
	if withStatusline {
		t.Error("未选 Claude 时 withStatusline 应为 false")
	}
	if !reflect.DeepEqual(selected, []string{"codex"}) {
		t.Errorf("selected = %v", selected)
	}
}

func TestResolveSelectionStatuslineFlag(t *testing.T) {
	// 已传 --with-statusline：不追问，直接生效。
	stubPrompts(t,
		func([]string) ([]string, error) { t.Error("显式选择时不应弹出多选"); return nil, nil },
		func() (bool, error) { t.Error("已传 --with-statusline 时不应追问"); return false, nil },
	)
	_, withStatusline, err := resolveSelection(platformSelectionInput{
		explicit:            []string{"claude-code"},
		withStatusline:      true,
		withStatuslineGiven: true,
		isTTY:               true,
	})
	if err != nil {
		t.Fatalf("resolveSelection 失败：%v", err)
	}
	if !withStatusline {
		t.Error("--with-statusline 已传时应生效")
	}

	// 已传 --with-statusline 但未选 Claude：忽略，不追问。
	_, withStatusline, err = resolveSelection(platformSelectionInput{
		explicit:            []string{"codex"},
		withStatusline:      true,
		withStatuslineGiven: true,
		isTTY:               true,
	})
	if err != nil {
		t.Fatalf("resolveSelection 失败：%v", err)
	}
	if withStatusline {
		t.Error("未选 Claude 时 --with-statusline 应被忽略")
	}
}

func TestResolveSelectionPromptError(t *testing.T) {
	stubPrompts(t,
		func([]string) ([]string, error) { return nil, errors.New("用户取消") },
		func() (bool, error) { return false, nil },
	)
	_, _, err := resolveSelection(platformSelectionInput{isTTY: true})
	if err == nil {
		t.Error("多选出错时应返回错误")
	}
}

func TestPlatformInstalledAndFilterUninstalled(t *testing.T) {
	dir := t.TempDir()
	if platformInstalled(dir, "claude-code") {
		t.Error("空目录不应判定为已安装 Claude")
	}
	if err := os.MkdirAll(filepath.Join(dir, ".claude"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(dir, ".codex"), 0o755); err != nil {
		t.Fatal(err)
	}
	if !platformInstalled(dir, "claude-code") {
		t.Error(".claude 存在时应判定为已安装")
	}
	if platformInstalled(dir, "cursor") {
		t.Error(".cursor 不存在时不应判定为已安装")
	}
	if platformInstalled(dir, "unknown") {
		t.Error("未知平台不应判定为已安装")
	}
	toInstall, skipped := filterUninstalled(dir, []string{"claude-code", "cursor", "codex"})
	if !reflect.DeepEqual(toInstall, []string{"cursor"}) {
		t.Errorf("toInstall = %v，想得到 [cursor]", toInstall)
	}
	if !reflect.DeepEqual(skipped, []string{"claude-code", "codex"}) {
		t.Errorf("skipped = %v，想得到 [claude-code codex]", skipped)
	}
}
