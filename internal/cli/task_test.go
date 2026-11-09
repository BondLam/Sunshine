package cli

import (
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

// TestTaskSubcommands 注册了全部 17 个 task.py 子命令 + context + add-session。
func TestTaskSubcommands(t *testing.T) {
	want := []string{
		"create", "add-context", "validate", "list-context", "start", "current",
		"finish", "set-branch", "set-base-branch", "set-scope", "set-meta",
		"rename", "archive", "list", "add-subtask", "remove-subtask",
		"list-archive", "context", "add-session",
	}
	byName := map[string]*cobra.Command{}
	for _, cmd := range taskCmd.Commands() {
		byName[cmd.Name()] = cmd
	}
	for _, name := range want {
		if byName[name] == nil {
			t.Errorf("sunshine task 缺少子命令 %q", name)
		}
	}
	if len(byName) != len(want) {
		t.Errorf("sunshine task 有 %d 个子命令，预期 %d 个", len(byName), len(want))
	}
	for _, cmd := range taskCmd.Commands() {
		if cmd.Short == "" {
			t.Errorf("sunshine task %s 缺少中文简介", cmd.Name())
		}
	}
	if !strings.Contains(taskCmd.Short, "任务") {
		t.Errorf("task 命令简介应为中文，实际 %q", taskCmd.Short)
	}
}

func TestTaskCommandFlags(t *testing.T) {
	cases := map[string][]string{
		"create":      {"slug", "assignee", "priority", "description", "parent", "package", "base-branch", "meta", "no-start", "force"},
		"start":       {"allow-empty-context"},
		"current":     {"source", "json"},
		"list":        {"mine", "status", "json"},
		"rename":      {"dry-run"},
		"archive":     {"no-commit", "skip-branch-validation"},
		"context":     {"mode", "step", "platform", "json"},
		"add-session": {"title", "commit", "summary", "branch", "package", "commit-subject", "change", "test", "next-step", "idempotency-key", "no-commit", "stdin", "content-file"},
	}
	byName := map[string]*cobra.Command{}
	for _, cmd := range taskCmd.Commands() {
		byName[cmd.Name()] = cmd
	}
	for name, flags := range cases {
		cmd := byName[name]
		if cmd == nil {
			t.Errorf("sunshine task 缺少子命令 %q，无法检查其选项", name)
			continue
		}
		for _, flag := range flags {
			if cmd.Flags().Lookup(flag) == nil {
				t.Errorf("sunshine task %s 缺少 --%s 选项", name, flag)
			}
		}
	}
	// 简写对照：create -s/-a/-p/-d，list -m/-s，current/context -j，context -m。
	shorthands := []struct {
		cmd, flag, want string
	}{
		{"create", "slug", "s"}, {"create", "assignee", "a"},
		{"create", "priority", "p"}, {"create", "description", "d"},
		{"list", "mine", "m"}, {"list", "status", "s"},
		{"current", "json", "j"}, {"context", "json", "j"}, {"context", "mode", "m"},
	}
	for _, c := range shorthands {
		cmd := byName[c.cmd]
		if cmd == nil {
			continue
		}
		f := cmd.Flags().Lookup(c.flag)
		if f == nil {
			continue
		}
		if f.Shorthand != c.want {
			t.Errorf("sunshine task %s --%s 的简写应为 %q，实际 %q", c.cmd, c.flag, c.want, f.Shorthand)
		}
	}
}
