// Package config 实现 .sunshine/config.yaml 的完整字段读写。
//
// 字段以 Trellis .trellis/config.yaml 原文为准（Trellis 0.6.17，
// commit f089cb3），字段名按 Sunshine 改名：trellis → sunshine，
// prompt_injection.skip_keyword 的 Go 版默认值为 "no-sunshine"。
//
// 行为约定：
//
//   - 缺失文件时 Load 返回全默认值，不报错；损坏文件（YAML 解析失败、
//     字段类型错误）时返回错误，绝不静默吞掉。
//   - 未知字段会被忽略（不启用严格模式）：registry.spec、update.skip
//     等字段由阶段 4 接入，旧文件里的这些字段不应导致读取失败。
//   - Marshal / Write 只用于读取后生成新文件：yaml.v3  marshal 会丢弃
//     原文件的注释与排版，因此绝不能用它原地改写用户的 config.yaml。
//     用户文件的注释版以 templates/sunshine/config.yaml 为准。
//   - codex.dispatch_mode 在 Load 时规范化（见 NormalizeDispatchMode）。
package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// 默认值（与 Trellis .trellis/config.yaml 及 internal/hook 的
// LoadConfig 保持一致）。
const (
	DefaultSessionCommitMessage = "chore: record journal"
	DefaultMaxJournalLines      = 2000
	DefaultIdleTimeout          = "5m"
	DefaultMaxLiveWorkers       = 6
	DefaultMaxFileBytes         = 32768
	DefaultMaxArtifactBytes     = 65536
	DefaultMaxTotalBytes        = 131072
	DefaultSkipKeyword          = "no-sunshine"
	DefaultTasksDir             = ".sunshine/tasks"
	DefaultSpecDir              = ".sunshine/spec"
)

// Bool 是接受 Trellis 文档所列写法的布尔值：true / false / yes / no /
// 1 / 0 / on / off（大小写不敏感）。yaml.v3 本身只把 true/false 与
// yes/no/on/off 解析为布尔，裸 1/0 会是整数，因此这里单独处理。
type Bool bool

// UnmarshalYAML 接受布尔、整数 0/1 与上述字符串写法。
func (b *Bool) UnmarshalYAML(value *yaml.Node) error {
	var v any
	if err := value.Decode(&v); err != nil {
		return err
	}
	switch t := v.(type) {
	case bool:
		*b = Bool(t)
		return nil
	case int:
		switch t {
		case 0:
			*b = false
			return nil
		case 1:
			*b = true
			return nil
		}
		return fmt.Errorf("session_auto_commit 的值 %d 非法，只接受 true/false/yes/no/1/0/on/off", t)
	case string:
		switch strings.ToLower(strings.TrimSpace(t)) {
		case "true", "yes", "on", "1":
			*b = true
			return nil
		case "false", "no", "off", "0":
			*b = false
			return nil
		}
		return fmt.Errorf("session_auto_commit 的值 %q 非法，只接受 true/false/yes/no/1/0/on/off", t)
	default:
		return fmt.Errorf("session_auto_commit 的值类型 %T 非法，只接受 true/false/yes/no/1/0/on/off", v)
	}
}

// MarshalYAML 写回为标准布尔。
func (b Bool) MarshalYAML() (any, error) { return bool(b), nil }

// Package 是 monorepo 的一个包声明（config.yaml 的 packages: 一节）。
type Package struct {
	// Path 是包相对项目根的路径。
	Path string `yaml:"path"`
	// Type 是包类型，如 submodule（可选）。
	Type string `yaml:"type,omitempty"`
	// Git 为真时表示该包是独立仓库（polyrepo / meta-repo 布局）。
	Git bool `yaml:"git,omitempty"`
}

// Hooks 是任务生命周期 hook：每个事件对应一组 shell 命令。命令收到
// TASK_JSON_PATH 环境变量，指向当前任务的 task.json；hook 失败只
// 告警，不阻塞主任务操作。
//
// 事件名以 Trellis config.yaml 原文为准：after_create、after_start、
// after_finish、after_archive（PLAN.md 阶段 3 一节曾误写作
// start / finish / archive，见本包测试）。
type Hooks struct {
	AfterCreate  []string `yaml:"after_create"`
	AfterStart   []string `yaml:"after_start"`
	AfterFinish  []string `yaml:"after_finish"`
	AfterArchive []string `yaml:"after_archive"`
}

// WorkerGuard 是 channel worker 的 OOM / 空闲防护阈值。
type WorkerGuard struct {
	// IdleTimeout 是 worker 持续空闲多久后被清理，接受 "5m"、"30s"
	// 这类 Go duration 写法；"0" 表示禁用空闲清理。
	IdleTimeout string `yaml:"idle_timeout"`
	// MaxLiveWorkers 是存活 worker 预算；0 表示禁用 spawn 时预算检查。
	MaxLiveWorkers int `yaml:"max_live_workers"`
	// TrustedContextDirs 把 --file/--jsonl/--agent 的 containment
	// 检查扩展到 worker cwd 之外（.sunshine/tasks 或
	// .sunshine/workspace 是外部目录的符号链接时用得上）。
	TrustedContextDirs []string `yaml:"trusted_context_dirs"`
	// AutoTrustSunshineSymlinks 为真（默认）时，.sunshine/tasks /
	// .sunshine/workspace 本身是顶层符号链接时会被窄幅自动信任。
	// 对应 Trellis 的 auto_trust_trellis_symlinks。
	AutoTrustSunshineSymlinks bool `yaml:"auto_trust_sunshine_symlinks"`
}

// Channel 是 channel 相关的配置。
type Channel struct {
	WorkerGuard WorkerGuard `yaml:"worker_guard"`
}

// Codex 是 Codex 专属的分发行为配置，其他平台忽略。
type Codex struct {
	// DispatchMode 见 NormalizeDispatchMode。
	DispatchMode string `yaml:"dispatch_mode"`
}

// ContextInjection 是子代理上下文注入的字节上限；0 表示禁用对应上限。
type ContextInjection struct {
	// MaxFileBytes 是每个 implement.jsonl / check.jsonl 引用文件的上限。
	MaxFileBytes int `yaml:"max_file_bytes"`
	// MaxArtifactBytes 是每个任务产物（prd.md / design.md / implement.md）的上限。
	MaxArtifactBytes int `yaml:"max_artifact_bytes"`
	// MaxTotalBytes 是整个注入负载的上限；溢出后剩余文件降级为索引行。
	MaxTotalBytes int `yaml:"max_total_bytes"`
}

// PromptInjection 是每轮提示词注入的配置。
type PromptInjection struct {
	// SkipKeyword 是跳过 <workflow-state> 面包屑的逃生关键词；用户
	// 提示词以独立单词形式包含它（大小写不敏感、按词边界匹配）时，
	// 该轮跳过面包屑。"" 表示禁用该逃生口。
	SkipKeyword string `yaml:"skip_keyword"`
}

// Update 是 `sunshine update` 的行为配置（阶段 4 补建模）。
type Update struct {
	// Skip 是 update 跳过的相对路径列表（文件或目录前缀，如
	// ".claude/commands"）。命中 Skip 的模板文件不参与本次更新；
	// 破坏性版本 + `sunshine update --migrate` 时可绕过
	// （见 loadUpdateSkipPaths 的上游实现与 internal/update）。
	Skip []string `yaml:"skip,omitempty"`
}

// Config 是 .sunshine/config.yaml 的完整结构。
type Config struct {
	// SessionCommitMessage 是自动提交日志 / 索引改动时用的提交消息。
	SessionCommitMessage string `yaml:"session_commit_message"`
	// MaxJournalLines 是单个日志文件轮换前的最大行数。
	MaxJournalLines int `yaml:"max_journal_lines"`
	// SessionAutoCommit 为真（默认）时，脚本在 add-session / task
	// archive 后自动暂存并提交日志与任务改动；为假时只写文件不碰 git。
	SessionAutoCommit Bool `yaml:"session_auto_commit"`
	// Hooks 是任务生命周期 hook。
	Hooks Hooks `yaml:"hooks"`
	// Packages 是 monorepo 包声明；为空表示单仓。
	Packages map[string]Package `yaml:"packages"`
	// DefaultPackage 是未指定 --package 时的默认包。
	DefaultPackage string `yaml:"default_package"`
	// Channel 是 channel 相关的配置。
	Channel Channel `yaml:"channel"`
	// Codex 是 Codex 专属配置。
	Codex Codex `yaml:"codex"`
	// ContextInjection 是子代理上下文注入上限。
	ContextInjection ContextInjection `yaml:"context_injection"`
	// PromptInjection 是每轮提示词注入配置。
	PromptInjection PromptInjection `yaml:"prompt_injection"`
	// Update 是 `sunshine update` 的行为配置（阶段 4）。
	Update Update `yaml:"update,omitempty"`
	// Registry 是模板市场来源（阶段 4，B 组）：安装 .sunshine/spec
	// 所用的 registry 来源与模板，sunshine update 靠它持续刷新。
	// 指针 + omitempty：未配置时不出现在 Marshal 输出里；
	// 行级读写见 LoadSpecRegistryConfig / WriteSpecRegistryConfig。
	Registry *RegistryConfig `yaml:"registry,omitempty"`

	// 以下为 Sunshine 扩展字段（阶段 1 沿用，非 Trellis 原文所有）：
	// Version 是项目内 Sunshine 版本标记（由 sunshine update 维护，
	// 与 .sunshine/.version 文件含义相同）。
	Version string `yaml:"version"`
	// TasksDir 是任务默认存放目录（相对项目根目录）。
	TasksDir string `yaml:"tasks_dir"`
	// SpecDir 是规范目录（相对项目根目录）。
	SpecDir string `yaml:"spec_dir"`
}

// Default 返回全默认值的配置。
func Default() *Config {
	return &Config{
		SessionCommitMessage: DefaultSessionCommitMessage,
		MaxJournalLines:      DefaultMaxJournalLines,
		SessionAutoCommit:    true,
		Channel: Channel{WorkerGuard: WorkerGuard{
			IdleTimeout:               DefaultIdleTimeout,
			MaxLiveWorkers:            DefaultMaxLiveWorkers,
			AutoTrustSunshineSymlinks: true,
		}},
		Codex:            Codex{DispatchMode: "auto"},
		ContextInjection: ContextInjection{MaxFileBytes: DefaultMaxFileBytes, MaxArtifactBytes: DefaultMaxArtifactBytes, MaxTotalBytes: DefaultMaxTotalBytes},
		PromptInjection:  PromptInjection{SkipKeyword: DefaultSkipKeyword},
		TasksDir:         DefaultTasksDir,
		SpecDir:          DefaultSpecDir,
	}
}

// Load 读 path 处的 config.yaml。文件缺失时返回全默认值；文件存在
// 但损坏（YAML 解析失败或字段类型错误）时返回错误。
func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return Default(), nil
		}
		return nil, fmt.Errorf("读取配置文件失败：%w", err)
	}
	return LoadBytes(data)
}

// LoadBytes 解析 config.yaml 内容。解析失败时返回错误。
func LoadBytes(data []byte) (*Config, error) {
	cfg := Default()
	if err := yaml.Unmarshal(data, cfg); err != nil {
		return nil, fmt.Errorf("解析配置文件失败：%w", err)
	}
	cfg.normalize()
	return cfg, nil
}

// LoadProject 读 root/.sunshine/config.yaml。
func LoadProject(root string) (*Config, error) {
	return Load(filepath.Join(root, ".sunshine", "config.yaml"))
}

// SkipPaths 返回 update.skip 跳过列表的副本。
func (c *Config) SkipPaths() []string {
	if c == nil {
		return nil
	}
	out := make([]string, len(c.Update.Skip))
	copy(out, c.Update.Skip)
	return out
}

// LoadUpdateSkipPaths 按行解析 root/.sunshine/config.yaml 的
// update.skip 列表（上游 loadUpdateSkipPaths 的移植，不走 YAML 解析，
// 这样即使文件其余部分损坏也不会丢掉跳过规则；解析失败时返回空列表
// 并打印警告，不报错）。
//
//	config.yaml 写法：
//
//	update:
//	  skip:
//	    - .claude/commands
//	    - .sunshine/agents/check.md
//
// 文件缺失时返回空列表。返回的路径已去掉首尾引号与空白。
func LoadUpdateSkipPaths(root string) []string {
	path := filepath.Join(root, ".sunshine", "config.yaml")
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		fmt.Fprintf(os.Stderr, "警告：读取 %s 失败，update.skip 规则不生效：%v\n", path, err)
		return nil
	}
	var paths []string
	inUpdate := false
	inSkip := false
	for _, line := range strings.Split(string(data), "\n") {
		trimmed := strings.TrimRight(line, " \t\r")
		if trimmed == "update:" {
			inUpdate = true
			inSkip = false
			continue
		}
		if inUpdate && len(trimmed) > 0 && (trimmed[0] == ' ' || trimmed[0] == '\t') &&
			strings.TrimSpace(trimmed) == "skip:" {
			inSkip = true
			continue
		}
		if inSkip {
			if item := strings.TrimSpace(trimmed); strings.HasPrefix(item, "- ") || item == "-" {
				if value := strings.Trim(strings.TrimSpace(strings.TrimPrefix(item, "-")), `'"`); value != "" {
					paths = append(paths, value)
				}
				continue
			}
			if trimmed != "" && !strings.HasPrefix(strings.TrimSpace(trimmed), "#") {
				inSkip = false
				inUpdate = false
			}
		}
		if inUpdate && trimmed != "" && trimmed[0] != ' ' && trimmed[0] != '\t' && !strings.HasPrefix(trimmed, "#") {
			inUpdate = false
			inSkip = false
		}
	}
	return paths
}

// normalize 对解析结果做合法化：规范化 dispatch_mode、夹取非法负值。
// 注意 yaml.Unmarshal 只覆盖文件中出现的字段，未出现的字段保留
// Default() 的预设值，因此这里只需处理显式写出的非法值。
func (c *Config) normalize() {
	c.Codex.DispatchMode = NormalizeDispatchMode(c.Codex.DispatchMode)
	if c.MaxJournalLines < 0 {
		c.MaxJournalLines = DefaultMaxJournalLines
	}
	if c.ContextInjection.MaxFileBytes < 0 {
		c.ContextInjection.MaxFileBytes = 0
	}
	if c.ContextInjection.MaxArtifactBytes < 0 {
		c.ContextInjection.MaxArtifactBytes = 0
	}
	if c.ContextInjection.MaxTotalBytes < 0 {
		c.ContextInjection.MaxTotalBytes = 0
	}
	if c.Channel.WorkerGuard.MaxLiveWorkers < 0 {
		c.Channel.WorkerGuard.MaxLiveWorkers = 0
	}
}

// NormalizeDispatchMode 把 codex.dispatch_mode 规范化为 "auto" 或
// "inline"：默认 auto；旧值 sub-agent 是 auto 的别名；其他显式值
// （含非法值）一律落到 inline。对应 Trellis config.yaml 的注释与
// internal/hook 的 ResolveCodexDispatchMode。
func NormalizeDispatchMode(mode string) string {
	switch strings.TrimSpace(strings.ToLower(mode)) {
	case "inline":
		return "inline"
	case "auto", "sub-agent", "":
		return "auto"
	default:
		return "inline"
	}
}

// ResolveDispatchMode 返回规范化后的 codex.dispatch_mode。
func (c *Config) ResolveDispatchMode() string {
	return NormalizeDispatchMode(c.Codex.DispatchMode)
}

// AutoCommit 返回 session_auto_commit（默认 true）。
func (c *Config) AutoCommit() bool { return bool(c.SessionAutoCommit) }

// IsMonorepo 对应 Trellis 的 is_monorepo：配了 packages 即 monorepo。
func (c *Config) IsMonorepo() bool { return len(c.Packages) > 0 }

// IdleTimeoutDuration 把 channel.worker_guard.idle_timeout 解析为
// time.Duration（接受 "5m"、"30s"、"0" 等写法）。非法写法返回错误。
func (c *Config) IdleTimeoutDuration() (time.Duration, error) {
	s := strings.TrimSpace(c.Channel.WorkerGuard.IdleTimeout)
	if s == "" {
		return time.ParseDuration(DefaultIdleTimeout)
	}
	if n, err := strconv.Atoi(s); err == nil {
		return time.Duration(n), nil
	}
	return time.ParseDuration(s)
}

// Marshal 把配置序列化为 YAML。注意：注释会丢失（见包文档），
// 只用于生成新文件，不用于原地改写用户文件。
func (c *Config) Marshal() ([]byte, error) {
	return yaml.Marshal(c)
}

// Write 把配置写入 path（生成新文件用，见 Marshal 的注释说明）。
func (c *Config) Write(path string) error {
	data, err := c.Marshal()
	if err != nil {
		return fmt.Errorf("序列化配置文件失败：%w", err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return fmt.Errorf("写入配置文件失败：%w", err)
	}
	return nil
}
