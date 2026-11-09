package hook

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// Config 是 .sunshine/config.yaml 里 hook 需要的配置子集。
// 解析用本包的迷你 YAML 解析器（Trellis 的 trellis_config.py 同款
// 子集思路），不引入 yaml 依赖：hook 必须零依赖、单二进制可用。
type Config struct {
	// Packages 是 monorepo 包声明（config.yaml 的 packages: 一节），
	// 键为包名；为空表示单仓。
	Packages map[string]bool
	// DefaultPackage 是默认包名。
	DefaultPackage string
	// SpecScope 是 session.spec_scope：nil 表示全量扫描；
	// "active_task" 或包名列表。
	SpecScope any
	// SkipKeyword 是 prompt_injection.skip_keyword，默认 "no-sunshine"。
	SkipKeyword string
	// CodexDispatchMode 是 codex.dispatch_mode，见 ResolveCodexDispatchMode。
	CodexDispatchMode string
	// ContextInjection 是子代理上下文注入的字节上限。
	MaxFileBytes     int
	MaxArtifactBytes int
	MaxTotalBytes    int
	// TasksDir 是任务目录（相对项目根目录），默认 .sunshine/tasks。
	TasksDir string
	// MaxJournalLines 是开发日志单文件最大行数，默认 2000。
	MaxJournalLines int
}

// DefaultLimits 是 context_injection 的默认值，对应
// inject-subagent-context.py 的 DEFAULT_LIMITS。
const (
	DefaultMaxFileBytes     = 32768
	DefaultMaxArtifactBytes = 65536
	DefaultMaxTotalBytes    = 131072
)

// LoadConfig 读 .sunshine/config.yaml，缺失或损坏时返回全默认值。
func LoadConfig(repoRoot string) Config {
	cfg := Config{
		Packages:          map[string]bool{},
		SkipKeyword:       "no-sunshine",
		CodexDispatchMode: "auto",
		MaxFileBytes:      DefaultMaxFileBytes,
		MaxArtifactBytes:  DefaultMaxArtifactBytes,
		MaxTotalBytes:     DefaultMaxTotalBytes,
		TasksDir:          ".sunshine/tasks",
		MaxJournalLines:   2000,
	}
	raw, err := os.ReadFile(filepath.Join(repoRoot, ".sunshine", "config.yaml"))
	if err != nil {
		return cfg
	}
	parsed := parseSimpleYAML(string(raw))

	if pkgs, ok := parsed["packages"].(map[string]any); ok {
		for name, v := range pkgs {
			if _, isMap := v.(map[string]any); isMap {
				cfg.Packages[name] = true
			}
		}
	}
	if v, ok := parsed["default_package"].(string); ok && v != "" {
		cfg.DefaultPackage = v
	}
	if session, ok := parsed["session"].(map[string]any); ok {
		if scope, ok := session["spec_scope"]; ok {
			cfg.SpecScope = scope
		}
	}
	if pi, ok := parsed["prompt_injection"].(map[string]any); ok {
		if kw, ok := pi["skip_keyword"].(string); ok {
			cfg.SkipKeyword = kw
		}
	}
	if codex, ok := parsed["codex"].(map[string]any); ok {
		if m, ok := codex["dispatch_mode"].(string); ok && m != "" {
			cfg.CodexDispatchMode = m
		}
	}
	if ci, ok := parsed["context_injection"].(map[string]any); ok {
		cfg.MaxFileBytes = yamlInt(ci, "max_file_bytes", DefaultMaxFileBytes)
		cfg.MaxArtifactBytes = yamlInt(ci, "max_artifact_bytes", DefaultMaxArtifactBytes)
		cfg.MaxTotalBytes = yamlInt(ci, "max_total_bytes", DefaultMaxTotalBytes)
	}
	if v, ok := parsed["tasks_dir"].(string); ok && v != "" {
		cfg.TasksDir = v
	}
	if _, ok := parsed["max_journal_lines"]; ok {
		cfg.MaxJournalLines = yamlInt(parsed, "max_journal_lines", 2000)
	}
	return cfg
}

func yamlInt(m map[string]any, key string, def int) int {
	raw, ok := m[key]
	if !ok {
		return def
	}
	switch v := raw.(type) {
	case int:
		if v < 0 {
			return def
		}
		return v
	case string:
		if n, err := strconv.Atoi(strings.TrimSpace(v)); err == nil && n >= 0 {
			return n
		}
	}
	return def
}

// IsMonorepo 对应 common.config.is_monorepo：配了 packages 即 monorepo。
func (c Config) IsMonorepo() bool { return len(c.Packages) > 0 }

// TasksDirAbs 返回任务目录绝对路径。
func (c Config) TasksDirAbs(repoRoot string) string {
	if filepath.IsAbs(c.TasksDir) {
		return c.TasksDir
	}
	return filepath.Join(repoRoot, filepath.FromSlash(c.TasksDir))
}

// ResolveCodexDispatchMode 把 codex.dispatch_mode 规范化为 "auto" 或
// "inline"：默认 auto；旧值 sub-agent 是 auto 的别名；其他显式值
// （含非法值）一律落到 inline 且不逐轮告警。对应
// inject-workflow-state.py 的 _resolve_codex_dispatch_mode。
func (c Config) ResolveCodexDispatchMode() string {
	m := strings.TrimSpace(strings.ToLower(c.CodexDispatchMode))
	switch m {
	case "inline":
		return "inline"
	case "auto", "sub-agent", "":
		return "auto"
	default:
		return "inline"
	}
}

// ---------------------------------------------------------------------------
// 迷你 YAML 解析器：支持 key: value 标量、按缩进嵌套的映射、"- " 标量
// 列表、# 注释（行内注释只认空格后的 #，引号内保留）与一层引号。
// 块标量、锚点、流式集合等结构不支持，遇上直接跳过该行——与
// Trellis trellis_config.py 的策略一致：宁可丢值，不猜错值。
// ---------------------------------------------------------------------------

func parseSimpleYAML(content string) map[string]any {
	root := map[string]any{}
	type line struct {
		indent  int
		trimmed string
	}
	var lines []line
	for _, rawLine := range strings.Split(content, "\n") {
		l := strings.TrimRight(rawLine, "\r")
		if strings.TrimSpace(l) == "" || strings.HasPrefix(strings.TrimSpace(l), "#") {
			continue
		}
		indent := len(l) - len(strings.TrimLeft(l, " "))
		lines = append(lines, line{indent: indent, trimmed: strings.TrimSpace(l)})
	}

	type frame struct {
		indent int
		m      map[string]any
	}
	stack := []frame{{indent: -1, m: root}}

	// 正在收集的列表：append 发生在局部切片上，列表结束时才写回映射。
	var pendMap map[string]any
	var pendKey string
	var pendList []any
	var pendIndent int
	flushList := func() {
		if pendMap != nil {
			pendMap[pendKey] = pendList
			pendMap = nil
		}
	}

	for i := 0; i < len(lines); i++ {
		ln := lines[i]
		if strings.HasPrefix(ln.trimmed, "- ") || ln.trimmed == "-" {
			if pendMap != nil && ln.indent >= pendIndent {
				pendList = append(pendList, yamlScalar(strings.TrimSpace(strings.TrimPrefix(ln.trimmed, "-"))))
			}
			continue
		}
		flushList()

		key, value, ok := splitYAMLKeyValue(ln.trimmed)
		if !ok {
			continue
		}
		for len(stack) > 1 && ln.indent <= stack[len(stack)-1].indent {
			stack = stack[:len(stack)-1]
		}
		cur := stack[len(stack)-1].m
		if value != "" {
			cur[key] = yamlScalar(value)
			continue
		}
		// 空值：看下一行决定是嵌套映射还是列表。
		if i+1 < len(lines) && (strings.HasPrefix(lines[i+1].trimmed, "- ") || lines[i+1].trimmed == "-") && lines[i+1].indent > ln.indent {
			pendMap, pendKey, pendList, pendIndent = cur, key, []any{}, lines[i+1].indent
			cur[key] = pendList
			continue
		}
		child := map[string]any{}
		cur[key] = child
		stack = append(stack, frame{indent: ln.indent, m: child})
	}
	flushList()
	return root
}

func splitYAMLKeyValue(line string) (key, value string, ok bool) {
	idx := strings.Index(line, ":")
	if idx <= 0 {
		return "", "", false
	}
	key = strings.TrimSpace(line[:idx])
	if key == "" || strings.ContainsAny(key, " \t") {
		return "", "", false
	}
	rest := strings.TrimSpace(line[idx+1:])
	return key, stripInlineComment(rest), true
}

// stripInlineComment 去掉空格后的 # 注释，引号内的 # 保留。
func stripInlineComment(s string) string {
	var quote byte
	for i := 0; i < len(s); i++ {
		c := s[i]
		if quote != 0 {
			if c == quote {
				quote = 0
			}
			continue
		}
		if c == '"' || c == '\'' {
			quote = c
			continue
		}
		if c == '#' && (i == 0 || s[i-1] == ' ' || s[i-1] == '\t') {
			return strings.TrimSpace(s[:i])
		}
	}
	return strings.TrimSpace(s)
}

func yamlScalar(s string) any {
	s = strings.TrimSpace(s)
	if len(s) >= 2 && s[0] == s[len(s)-1] && (s[0] == '"' || s[0] == '\'') {
		return s[1 : len(s)-1]
	}
	if n, err := strconv.Atoi(s); err == nil {
		return n
	}
	return s
}
