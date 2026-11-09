package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// 全字段解析：每个字段都显式写出时，解析结果与文件一致。
func TestLoadFullFields(t *testing.T) {
	data := `
session_commit_message: "chore: my commit"
max_journal_lines: 500
session_auto_commit: false
hooks:
  after_create:
    - "echo 'Task created'"
  after_start:
    - "echo 'Task started'"
  after_finish:
    - "echo 'Task finished'"
  after_archive:
    - "echo 'Task archived'"
packages:
  frontend:
    path: packages/frontend
  docs:
    path: docs-site
    type: submodule
  webapp:
    path: ./webapp
    git: true
default_package: frontend
channel:
  worker_guard:
    idle_timeout: 10m
    max_live_workers: 3
    trusted_context_dirs:
      - /work/shared
    auto_trust_sunshine_symlinks: false
codex:
  dispatch_mode: inline
context_injection:
  max_file_bytes: 1000
  max_artifact_bytes: 2000
  max_total_bytes: 3000
prompt_injection:
  skip_keyword: "skip-me"
version: 0.2.0
tasks_dir: custom/tasks
spec_dir: custom/spec
`
	cfg, err := LoadBytes([]byte(data))
	if err != nil {
		t.Fatalf("解析失败：%v", err)
	}
	if cfg.SessionCommitMessage != "chore: my commit" {
		t.Errorf("session_commit_message = %q", cfg.SessionCommitMessage)
	}
	if cfg.MaxJournalLines != 500 {
		t.Errorf("max_journal_lines = %d", cfg.MaxJournalLines)
	}
	if cfg.AutoCommit() {
		t.Error("session_auto_commit 应为 false")
	}
	if got := cfg.Hooks.AfterCreate; len(got) != 1 || got[0] != "echo 'Task created'" {
		t.Errorf("hooks.after_create = %v", got)
	}
	if got := cfg.Hooks.AfterStart; len(got) != 1 || got[0] != "echo 'Task started'" {
		t.Errorf("hooks.after_start = %v", got)
	}
	if got := cfg.Hooks.AfterFinish; len(got) != 1 || got[0] != "echo 'Task finished'" {
		t.Errorf("hooks.after_finish = %v", got)
	}
	if got := cfg.Hooks.AfterArchive; len(got) != 1 || got[0] != "echo 'Task archived'" {
		t.Errorf("hooks.after_archive = %v", got)
	}
	if len(cfg.Packages) != 3 {
		t.Fatalf("packages 数量 = %d", len(cfg.Packages))
	}
	if cfg.Packages["frontend"].Path != "packages/frontend" {
		t.Errorf("packages.frontend.path = %q", cfg.Packages["frontend"].Path)
	}
	if cfg.Packages["docs"].Type != "submodule" {
		t.Errorf("packages.docs.type = %q", cfg.Packages["docs"].Type)
	}
	if !cfg.Packages["webapp"].Git {
		t.Error("packages.webapp.git 应为 true")
	}
	if cfg.DefaultPackage != "frontend" {
		t.Errorf("default_package = %q", cfg.DefaultPackage)
	}
	if cfg.Channel.WorkerGuard.IdleTimeout != "10m" {
		t.Errorf("idle_timeout = %q", cfg.Channel.WorkerGuard.IdleTimeout)
	}
	if cfg.Channel.WorkerGuard.MaxLiveWorkers != 3 {
		t.Errorf("max_live_workers = %d", cfg.Channel.WorkerGuard.MaxLiveWorkers)
	}
	if got := cfg.Channel.WorkerGuard.TrustedContextDirs; len(got) != 1 || got[0] != "/work/shared" {
		t.Errorf("trusted_context_dirs = %v", got)
	}
	if cfg.Channel.WorkerGuard.AutoTrustSunshineSymlinks {
		t.Error("auto_trust_sunshine_symlinks 应为 false")
	}
	if cfg.Codex.DispatchMode != "inline" {
		t.Errorf("dispatch_mode = %q", cfg.Codex.DispatchMode)
	}
	if cfg.ContextInjection.MaxFileBytes != 1000 || cfg.ContextInjection.MaxArtifactBytes != 2000 || cfg.ContextInjection.MaxTotalBytes != 3000 {
		t.Errorf("context_injection = %+v", cfg.ContextInjection)
	}
	if cfg.PromptInjection.SkipKeyword != "skip-me" {
		t.Errorf("skip_keyword = %q", cfg.PromptInjection.SkipKeyword)
	}
	if cfg.Version != "0.2.0" || cfg.TasksDir != "custom/tasks" || cfg.SpecDir != "custom/spec" {
		t.Errorf("扩展字段：version=%q tasks_dir=%q spec_dir=%q", cfg.Version, cfg.TasksDir, cfg.SpecDir)
	}
}

// 缺失文件时返回全默认值。
func TestLoadMissingFileReturnsDefaults(t *testing.T) {
	cfg, err := Load(filepath.Join(t.TempDir(), "no-such-dir", "config.yaml"))
	if err != nil {
		t.Fatalf("缺失文件不应报错：%v", err)
	}
	def := Default()
	if cfg.SessionCommitMessage != def.SessionCommitMessage ||
		cfg.MaxJournalLines != def.MaxJournalLines ||
		cfg.AutoCommit() != def.AutoCommit() ||
		cfg.Channel.WorkerGuard.IdleTimeout != def.Channel.WorkerGuard.IdleTimeout ||
		cfg.Channel.WorkerGuard.MaxLiveWorkers != def.Channel.WorkerGuard.MaxLiveWorkers ||
		cfg.Codex.DispatchMode != def.Codex.DispatchMode ||
		cfg.ContextInjection != def.ContextInjection ||
		cfg.PromptInjection.SkipKeyword != def.PromptInjection.SkipKeyword ||
		cfg.TasksDir != def.TasksDir || cfg.SpecDir != def.SpecDir {
		t.Errorf("缺失文件未返回全默认值：%+v", cfg)
	}
	if len(cfg.Packages) != 0 || cfg.DefaultPackage != "" {
		t.Errorf("缺失文件时 packages/default_package 应为空：%v %q", cfg.Packages, cfg.DefaultPackage)
	}
}

// 损坏文件时返回错误，不静默吞掉。
func TestLoadCorruptFileReturnsError(t *testing.T) {
	for _, bad := range []string{
		"session_commit_message: [unclosed\n",
		"max_journal_lines: not-a-number\n",
		"session_auto_commit: maybe\n",
		"channel:\n  worker_guard: 42\n",
		"hooks:\n  after_create: not-a-list\n",
	} {
		if _, err := LoadBytes([]byte(bad)); err == nil {
			t.Errorf("损坏内容未报错：%q", bad)
		}
	}
	dir := t.TempDir()
	p := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(p, []byte(":\n - [\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(p); err == nil {
		t.Error("磁盘上的损坏文件未报错")
	}
}

// 部分字段文件：未出现的字段保留默认值（skip_keyword 默认值等）。
func TestLoadPartialFileKeepsDefaults(t *testing.T) {
	cfg, err := LoadBytes([]byte("max_journal_lines: 100\n"))
	if err != nil {
		t.Fatalf("解析失败：%v", err)
	}
	if cfg.MaxJournalLines != 100 {
		t.Errorf("max_journal_lines = %d", cfg.MaxJournalLines)
	}
	if cfg.PromptInjection.SkipKeyword != DefaultSkipKeyword {
		t.Errorf("skip_keyword 默认值 = %q，应为 %q", cfg.PromptInjection.SkipKeyword, DefaultSkipKeyword)
	}
	if cfg.SessionCommitMessage != DefaultSessionCommitMessage {
		t.Errorf("session_commit_message 默认值 = %q", cfg.SessionCommitMessage)
	}
	if cfg.Channel.WorkerGuard.IdleTimeout != DefaultIdleTimeout || cfg.Channel.WorkerGuard.MaxLiveWorkers != DefaultMaxLiveWorkers {
		t.Errorf("worker_guard 默认值不对：%+v", cfg.Channel.WorkerGuard)
	}
	if !cfg.Channel.WorkerGuard.AutoTrustSunshineSymlinks {
		t.Error("auto_trust_sunshine_symlinks 默认应为 true")
	}
	if cfg.ContextInjection.MaxFileBytes != DefaultMaxFileBytes ||
		cfg.ContextInjection.MaxArtifactBytes != DefaultMaxArtifactBytes ||
		cfg.ContextInjection.MaxTotalBytes != DefaultMaxTotalBytes {
		t.Errorf("context_injection 默认值不对：%+v", cfg.ContextInjection)
	}
	if cfg.Codex.DispatchMode != "auto" {
		t.Errorf("dispatch_mode 默认值 = %q", cfg.Codex.DispatchMode)
	}
	if !cfg.AutoCommit() {
		t.Error("session_auto_commit 默认应为 true")
	}
}

// skip_keyword 为空字符串时表示禁用逃生口，应原样保留（不回填默认值）。
func TestLoadEmptySkipKeywordDisables(t *testing.T) {
	cfg, err := LoadBytes([]byte("prompt_injection:\n  skip_keyword: \"\"\n"))
	if err != nil {
		t.Fatalf("解析失败：%v", err)
	}
	if cfg.PromptInjection.SkipKeyword != "" {
		t.Errorf("skip_keyword = %q，应为空", cfg.PromptInjection.SkipKeyword)
	}
}

// codex.dispatch_mode 规范化：auto / sub-agent 别名 / inline / 非法值。
func TestNormalizeDispatchMode(t *testing.T) {
	cases := map[string]string{
		"":            "auto",
		"auto":        "auto",
		"Auto":        "auto",
		" sub-agent ": "auto",
		"sub-agent":   "auto",
		"inline":      "inline",
		"INLINE":      "inline",
		"bogus":       "inline",
		"parallel":    "inline",
	}
	for in, want := range cases {
		if got := NormalizeDispatchMode(in); got != want {
			t.Errorf("NormalizeDispatchMode(%q) = %q，应为 %q", in, got, want)
		}
		cfg, err := LoadBytes([]byte("codex:\n  dispatch_mode: " + in + "\n"))
		if err != nil {
			t.Fatalf("解析 dispatch_mode %q 失败：%v", in, err)
		}
		if cfg.ResolveDispatchMode() != want {
			t.Errorf("Load 后 dispatch_mode(%q) = %q，应为 %q", in, cfg.ResolveDispatchMode(), want)
		}
	}
}

// hooks 事件名以 Trellis config.yaml 原文为准：after_start /
// after_finish / after_archive——PLAN.md 曾误写作 start / finish /
// archive；旧写法不应被解析为 hook。
func TestHooksEventNames(t *testing.T) {
	cfg, err := LoadBytes([]byte("hooks:\n  start:\n    - \"x\"\n  finish:\n    - \"y\"\n  archive:\n    - \"z\"\n"))
	if err != nil {
		t.Fatalf("解析失败：%v", err)
	}
	if len(cfg.Hooks.AfterStart) != 0 || len(cfg.Hooks.AfterFinish) != 0 || len(cfg.Hooks.AfterArchive) != 0 {
		t.Errorf("旧事件名 start/finish/archive 不应被解析：%+v", cfg.Hooks)
	}
}

// session_auto_commit 接受 Trellis 文档所列的全部写法。
func TestSessionAutoCommitForms(t *testing.T) {
	cases := map[string]bool{
		"true": true, "false": false,
		"yes": true, "no": false,
		"on": true, "off": false,
		"1": true, "0": false,
		"YES": true, "Off": false,
	}
	for in, want := range cases {
		cfg, err := LoadBytes([]byte("session_auto_commit: " + in + "\n"))
		if err != nil {
			t.Errorf("解析 session_auto_commit: %s 失败：%v", in, err)
			continue
		}
		if cfg.AutoCommit() != want {
			t.Errorf("session_auto_commit: %s = %v，应为 %v", in, cfg.AutoCommit(), want)
		}
	}
}

// idle_timeout 解析：Go duration 写法与 "0"（禁用）。
func TestIdleTimeoutDuration(t *testing.T) {
	cfg := Default()
	if d, err := cfg.IdleTimeoutDuration(); err != nil || d != 5*time.Minute {
		t.Errorf("默认 idle_timeout 解析 = %v, %v", d, err)
	}
	cfg.Channel.WorkerGuard.IdleTimeout = "0"
	if d, err := cfg.IdleTimeoutDuration(); err != nil || d != 0 {
		t.Errorf("idle_timeout 0 解析 = %v, %v", d, err)
	}
	cfg.Channel.WorkerGuard.IdleTimeout = "nonsense"
	if _, err := cfg.IdleTimeoutDuration(); err == nil {
		t.Error("非法 idle_timeout 未报错")
	}
}

// Marshal / Write 往返：写出的文件能重新读回相同值。
func TestMarshalRoundTrip(t *testing.T) {
	cfg := Default()
	cfg.DefaultPackage = "api"
	cfg.Packages = map[string]Package{"api": {Path: "apps/api"}}
	cfg.Hooks.AfterCreate = []string{"echo hi"}
	p := filepath.Join(t.TempDir(), "config.yaml")
	if err := cfg.Write(p); err != nil {
		t.Fatalf("Write 失败：%v", err)
	}
	back, err := Load(p)
	if err != nil {
		t.Fatalf("读回失败：%v", err)
	}
	if back.DefaultPackage != "api" || back.Packages["api"].Path != "apps/api" {
		t.Errorf("往返后 packages 不一致：%+v", back)
	}
	if len(back.Hooks.AfterCreate) != 1 || back.Hooks.AfterCreate[0] != "echo hi" {
		t.Errorf("往返后 hooks 不一致：%+v", back.Hooks)
	}
	if back.SessionCommitMessage != DefaultSessionCommitMessage || back.PromptInjection.SkipKeyword != DefaultSkipKeyword {
		t.Errorf("往返后默认值丢失：%+v", back)
	}
	// 注释会丢失：Marshal 产物不应含中文注释（行为说明，非缺陷）。
	data, _ := os.ReadFile(p)
	if strings.Contains(string(data), "#") {
		t.Error("Marshal 产物不应含注释（本实现不保留注释）")
	}
}

// LoadProject 读 root/.sunshine/config.yaml。
func TestLoadProject(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, ".sunshine"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, ".sunshine", "config.yaml"), []byte("max_journal_lines: 42\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, err := LoadProject(dir)
	if err != nil {
		t.Fatalf("LoadProject 失败：%v", err)
	}
	if cfg.MaxJournalLines != 42 {
		t.Errorf("max_journal_lines = %d", cfg.MaxJournalLines)
	}
	// 无 .sunshine/config.yaml 时返回默认值。
	cfg, err = LoadProject(t.TempDir())
	if err != nil || cfg.MaxJournalLines != DefaultMaxJournalLines {
		t.Errorf("缺失时 LoadProject = %+v, %v", cfg, err)
	}
}

// update.skip 建模（阶段 4）：YAML 字段解析与行级读取一致；
// 空 update 节不写入 Marshal 产物（omitempty）。
func TestUpdateSkip(t *testing.T) {
	cfg, err := LoadBytes([]byte("update:\n  skip:\n    - .claude/commands\n    - '.sunshine/agents/check.md'\n"))
	if err != nil {
		t.Fatalf("LoadBytes 失败：%v", err)
	}
	if got := cfg.SkipPaths(); len(got) != 2 || got[0] != ".claude/commands" || got[1] != ".sunshine/agents/check.md" {
		t.Errorf("SkipPaths = %v", got)
	}

	// 行级读取：带注释与后续顶层字段时仍能正确收尾。
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, ".sunshine"), 0o755); err != nil {
		t.Fatal(err)
	}
	content := "# 注释\nsession_commit_message: \"x\"\n\nupdate:\n  skip:\n    - .claude/commands\n    - \".cursor/skills\"\n    # 行内注释不算路径\n\nchannel:\n  worker_guard:\n    idle_timeout: 5m\n"
	if err := os.WriteFile(filepath.Join(dir, ".sunshine", "config.yaml"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	got := LoadUpdateSkipPaths(dir)
	if len(got) != 2 || got[0] != ".claude/commands" || got[1] != ".cursor/skills" {
		t.Errorf("LoadUpdateSkipPaths = %v", got)
	}
	// 与 YAML 解析结果一致。
	cfg2, err := LoadProject(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg2.SkipPaths()) != 2 {
		t.Errorf("LoadProject SkipPaths = %v", cfg2.SkipPaths())
	}
	// 缺失文件时返回空，不报错。
	if got := LoadUpdateSkipPaths(t.TempDir()); len(got) != 0 {
		t.Errorf("缺失时 LoadUpdateSkipPaths = %v", got)
	}
	// 默认配置 Marshal 不写出空的 update 节。
	data, err := Default().Marshal()
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), "update:") {
		t.Errorf("默认配置不应写出 update 节：\n%s", data)
	}
}
