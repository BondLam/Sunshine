package cli

import (
	"errors"
	"fmt"
	"path/filepath"
	"testing"

	"github.com/AlecAivazis/survey/v2/terminal"
	"github.com/BondLam/Sunshine/internal/projecttype"
)

// TestIsInitCancelled 覆盖取消的两种来源：Ctrl+C 中断与选“退出”。
func TestIsInitCancelled(t *testing.T) {
	if !isInitCancelled(terminal.InterruptErr) {
		t.Error("Ctrl+C 中断应判为取消")
	}
	if !isInitCancelled(errInitCancelled) {
		t.Error("选“退出”应判为取消")
	}
	wrapped := fmt.Errorf("选择平台失败：%w", terminal.InterruptErr)
	if !isInitCancelled(wrapped) {
		t.Error("包在 %w 链里的中断应判为取消")
	}
	if isInitCancelled(errors.New("EOF")) {
		t.Error("EOF 不应判为取消")
	}
	if isInitCancelled(nil) {
		t.Error("nil 不应判为取消")
	}
}

// TestHandleInitErr 取消转干净退出（nil），其余错误原样返回。
func TestHandleInitErr(t *testing.T) {
	if err := handleInitErr(errInitCancelled); err != nil {
		t.Errorf("取消应返回 nil，实际：%v", err)
	}
	if err := handleInitErr(terminal.InterruptErr); err != nil {
		t.Errorf("Ctrl+C 应返回 nil，实际：%v", err)
	}
	other := errors.New("boom")
	if err := handleInitErr(other); err != other {
		t.Errorf("其他错误应原样返回，实际：%v", err)
	}
}

// TestParseProjectTypeChoice 五个选项一一映射，选“退出”返回取消。
func TestParseProjectTypeChoice(t *testing.T) {
	cases := []struct {
		label  string
		want   projecttype.Type
		cancel bool
	}{
		{"前端项目", projecttype.Frontend, false},
		{"后端项目", projecttype.Backend, false},
		{"前后端全栈", projecttype.Fullstack, false},
		{"产品设计", projecttype.Design, false},
		{quitOptionLabel, projecttype.Unknown, true},
	}
	for _, c := range cases {
		got, err := parseProjectTypeChoice(c.label)
		if c.cancel {
			if !errors.Is(err, errInitCancelled) {
				t.Errorf("%q 应返回取消，实际：%v", c.label, err)
			}
			continue
		}
		if err != nil || got != c.want {
			t.Errorf("%q = (%v, %v)，想得到 (%v, nil)", c.label, got, err, c.want)
		}
	}
}

// TestResolveProjectTypeCancel 提问处 Ctrl+C 应返回取消，不再静默默认全栈。
func TestResolveProjectTypeCancel(t *testing.T) {
	for _, promptErr := range []error{errInitCancelled, terminal.InterruptErr} {
		stubProjectTypePrompt(t, func() (projecttype.Type, error) {
			return projecttype.Fullstack, promptErr
		})
		_, _, err := resolveProjectType(projectTypeInput{detected: projecttype.Unknown, isTTY: true})
		if !isInitCancelled(err) {
			t.Fatalf("提问错误 %v 应返回取消，实际：%v", promptErr, err)
		}
	}
}

// TestResolveSelectionCancel 平台多选 Ctrl+C 应返回取消，不再追问状态栏。
func TestResolveSelectionCancel(t *testing.T) {
	oldPlatforms, oldStatusline := promptPlatforms, promptStatusline
	t.Cleanup(func() { promptPlatforms, promptStatusline = oldPlatforms, oldStatusline })
	promptPlatforms = func([]string) ([]string, error) { return nil, terminal.InterruptErr }
	promptStatusline = func() (bool, error) {
		t.Error("取消后不应追问状态栏")
		return false, nil
	}
	_, _, err := resolveSelection(platformSelectionInput{isTTY: true})
	if !isInitCancelled(err) {
		t.Fatalf("平台多选 Ctrl+C 应返回取消，实际：%v", err)
	}
}

// TestResolveInteractiveTemplateQuit 第一级选单选“退出”应返回取消。
func TestResolveInteractiveTemplateQuit(t *testing.T) {
	setFakeTemplatePrompts(t, []string{quitOptionLabel}, nil, nil)

	_, _, _, err := resolveInteractiveTemplate("")
	if !errors.Is(err, errInitCancelled) {
		t.Fatalf("选“退出”应返回取消哨兵，实际：%v", err)
	}
}

// TestResolveInteractiveTemplateQuitMarketplace 市场模板选单选“退出”应返回取消。
func TestResolveInteractiveTemplateQuitMarketplace(t *testing.T) {
	repo := filepath.Join(t.TempDir(), "market")
	initTestGitRepo(t, repo, testMarketFiles())
	setFakeTemplatePrompts(t, []string{quitOptionLabel}, nil, nil)

	_, _, _, err := resolveInteractiveTemplate(repo)
	if !errors.Is(err, errInitCancelled) {
		t.Fatalf("市场选单选“退出”应返回取消哨兵，实际：%v", err)
	}
}

// TestResolveInteractiveTemplateSelectInterrupt 选单 Ctrl+C 应判为取消。
func TestResolveInteractiveTemplateSelectInterrupt(t *testing.T) {
	setFakeTemplatePrompts(t, nil, nil, terminal.InterruptErr)

	_, _, _, err := resolveInteractiveTemplate("")
	if !isInitCancelled(err) {
		t.Fatalf("选单 Ctrl+C 应判为取消，实际：%v", err)
	}
}
