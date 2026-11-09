package scaffold

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/linrong1982/Sunshine/templates"
)

func TestInitCreatesSkeleton(t *testing.T) {
	dir := t.TempDir()
	if err := Init(dir, "张三", false); err != nil {
		t.Fatalf("Init 失败：%v", err)
	}
	for _, rel := range []string{
		".sunshine/config.yaml",
		".sunshine/workflow.md",
		".sunshine/AGENTS.md",
		".sunshine/.version",
		".sunshine/.developer",
		".sunshine/.template-hashes.json",
		".sunshine/agents/architect.md",
		".sunshine/agents/check.md",
		".sunshine/agents/implement.md",
		".sunshine/agents/plan.md",
		".sunshine/agents/research.md",
		".sunshine/spec/system.md",
		".sunshine/spec/frontend.md",
		".sunshine/spec/backend.md",
		".sunshine/workspace/index.md",
		".sunshine/tasks/README.md",
	} {
		if _, err := os.Stat(filepath.Join(dir, rel)); err != nil {
			t.Errorf("缺少文件 %s：%v", rel, err)
		}
	}
	data, _ := os.ReadFile(filepath.Join(dir, ".sunshine/config.yaml"))
	if !strings.Contains(string(data), "Sunshine 项目配置文件") {
		t.Error("config.yaml 不是中文模板")
	}
	// 完整字段版 config.yaml：已启用字段与注释字段齐全，且为中文注释。
	for _, want := range []string{
		"session_commit_message", "max_journal_lines", "session_auto_commit",
		"after_create", "after_start", "after_finish", "after_archive",
		"packages", "default_package",
		"channel:", "worker_guard", "idle_timeout", "max_live_workers",
		"codex:", "dispatch_mode",
		"context_injection", "max_file_bytes", "max_artifact_bytes", "max_total_bytes",
		"prompt_injection", "skip_keyword", "no-sunshine",
	} {
		if !strings.Contains(string(data), want) {
			t.Errorf("config.yaml 缺少字段 %s", want)
		}
	}
	if strings.Contains(string(data), "trellis") || strings.Contains(string(data), "Trellis") {
		t.Error("config.yaml 残留 trellis 字样")
	}
	// workflow.md 与完整版 native 模板同源（2026-11-09 用户确认 init 改用完整版）。
	native, err := templates.Workflows.ReadFile("workflows/native/workflow.md")
	if err != nil {
		t.Fatalf("读取原生工作流模板失败：%v", err)
	}
	got, err := os.ReadFile(filepath.Join(dir, ".sunshine/workflow.md"))
	if err != nil {
		t.Fatalf("读取 workflow.md 失败：%v", err)
	}
	if !bytes.Equal(got, native) {
		t.Error("init 生成的 workflow.md 与完整版 native 模板不一致")
	}
	// agents 五个定义均为中文。
	for _, name := range []string{"architect", "check", "implement", "plan", "research"} {
		agent, err := os.ReadFile(filepath.Join(dir, ".sunshine", "agents", name+".md"))
		if err != nil {
			t.Errorf("缺少 agent 定义 %s：%v", name, err)
			continue
		}
		if strings.Contains(string(agent), "trellis") || strings.Contains(string(agent), "Trellis") {
			t.Errorf("agent 定义 %s 残留 trellis 字样", name)
		}
	}
	dev, _ := os.ReadFile(filepath.Join(dir, ".sunshine/.developer"))
	if strings.TrimSpace(string(dev)) != "张三" {
		t.Errorf(".developer 内容不对：%q", dev)
	}
	// 重复初始化应拒绝。
	if err := Init(dir, "张三", false); err == nil {
		t.Error("重复初始化未拒绝")
	}
	// --force 可覆盖。
	if err := Init(dir, "李四", true); err != nil {
		t.Fatalf("force 初始化失败：%v", err)
	}
	dev, _ = os.ReadFile(filepath.Join(dir, ".sunshine/.developer"))
	if strings.TrimSpace(string(dev)) != "李四" {
		t.Errorf("force 后 .developer 内容不对：%q", dev)
	}
}
