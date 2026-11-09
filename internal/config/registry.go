package config

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// SpecRegistryConfig 是 config.yaml 的 registry.spec 一节：
// 安装 .sunshine/spec 所用的来源与模板，sunshine update 靠它持续刷新。
// 对应上游 registry-config.ts 的 SpecRegistryConfig。
type SpecRegistryConfig struct {
	// Source 是 --registry 的原始输入（如 gh:owner/repo 或本地路径）。
	Source string `yaml:"source"`
	// Template 是 marketplace 模板 id；直连型 registry 为空。
	Template string `yaml:"template,omitempty"`
}

// RegistryConfig 是 config.yaml 的 registry 一节。
type RegistryConfig struct {
	Spec SpecRegistryConfig `yaml:"spec"`
}

// stripYamlScalar 去掉 YAML 标量值两侧的空白与一对首尾引号，
// 对应上游 registry-config.ts 的 stripYamlScalar。
func stripYamlScalar(value string) string {
	value = strings.TrimSpace(value)
	if len(value) >= 1 && (value[0] == '\'' || value[0] == '"') {
		value = value[1:]
	}
	if len(value) >= 1 && (value[len(value)-1] == '\'' || value[len(value)-1] == '"') {
		value = value[:len(value)-1]
	}
	return value
}

// specRegistryLines 生成 registry.spec 一节的行（行级写回用），
// 对应上游 registry-config.ts 的 specRegistryLines。
func specRegistryLines(cfg SpecRegistryConfig) []string {
	source := filepath.ToSlash(cfg.Source)
	lines := []string{
		"  spec:",
		"    source: " + source,
	}
	if cfg.Template != "" {
		lines = append(lines, "    template: "+cfg.Template)
	}
	return lines
}

var (
	// registrySectionRe 匹配 registry: 一节（(?m) 使 ^/$ 按行生效，
	// 整文匹配与逐行匹配都可用）。
	registrySectionRe = regexp.MustCompile(`(?m)^registry:\s*$`)
	registrySpecRe    = regexp.MustCompile(`^\s{2}spec:\s*$`)
	registrySourceRe  = regexp.MustCompile(`^\s{4}source:\s+(.+)$`)
	registryTmplRe    = regexp.MustCompile(`^\s{4}template:\s+(.+)$`)
)

// LoadSpecRegistryConfig 用行级解析读 root/.sunshine/config.yaml 的
// registry.spec（不丢注释与排版），对应上游 loadSpecRegistryConfig。
// 文件不存在或无 registry.spec 时返回 nil，不报错。
func LoadSpecRegistryConfig(root string) (*SpecRegistryConfig, error) {
	data, err := os.ReadFile(filepath.Join(root, ".sunshine", "config.yaml"))
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("读取配置文件失败：%w", err)
	}
	return parseSpecRegistryConfig(string(data)), nil
}

// parseSpecRegistryConfig 是行级解析的核心（便于测试），
// 状态机与上游 loadSpecRegistryConfig 一致。
func parseSpecRegistryConfig(content string) *SpecRegistryConfig {
	lines := strings.Split(content, "\n")
	inRegistry := false
	inSpec := false
	var source string
	var template string
	for _, line := range lines {
		trimmed := strings.TrimRight(line, " \t\r")
		if registrySectionRe.MatchString(trimmed) {
			inRegistry = true
			inSpec = false
			continue
		}
		if inRegistry && registrySpecRe.MatchString(trimmed) {
			inSpec = true
			continue
		}
		if inRegistry && trimmed != "" && !strings.HasPrefix(trimmed, " ") {
			if source != "" {
				return &SpecRegistryConfig{Source: source, Template: template}
			}
			inRegistry = false
			inSpec = false
			continue
		}
		if inSpec {
			if m := registrySourceRe.FindStringSubmatch(trimmed); m != nil {
				source = stripYamlScalar(m[1])
				continue
			}
			if m := registryTmplRe.FindStringSubmatch(trimmed); m != nil {
				template = stripYamlScalar(m[1])
				continue
			}
			if trimmed != "" && !strings.HasPrefix(trimmed, "    ") {
				if source != "" {
					return &SpecRegistryConfig{Source: source, Template: template}
				}
				inSpec = false
			}
		}
	}
	if source != "" {
		return &SpecRegistryConfig{Source: source, Template: template}
	}
	return nil
}

// WriteSpecRegistryConfig 用行级编辑把 registry.spec 写进
// root/.sunshine/config.yaml：保留用户原有字段、注释与排版，
// 对应上游 writeSpecRegistryConfig。文件不存在时不做任何事
// （由 sunshine init 先生成 config.yaml）。
func WriteSpecRegistryConfig(root string, cfg SpecRegistryConfig) error {
	path := filepath.Join(root, ".sunshine", "config.yaml")
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("读取配置文件失败：%w", err)
	}
	content := string(data)
	var out string
	if registrySectionRe.MatchString(content) {
		out = updateSpecRegistryLines(content, cfg)
	} else {
		out = appendSpecRegistrySection(content, cfg)
	}
	return writeFileAtomic(path, out)
}

// updateSpecRegistryLines 在已有 registry: 一节内替换 / 补写 spec 子节。
func updateSpecRegistryLines(content string, cfg SpecRegistryConfig) string {
	normalizedSource := filepath.ToSlash(cfg.Source)
	specLines := specRegistryLines(cfg)
	lines := strings.Split(content, "\n")
	var output []string
	inRegistry := false
	inSpec := false
	sawSpec := false
	wroteSource := false
	wroteTemplate := false
	flushSpec := func() {
		if inSpec {
			if !wroteSource {
				output = append(output, "    source: "+normalizedSource)
			}
			if cfg.Template != "" && !wroteTemplate {
				output = append(output, "    template: "+cfg.Template)
			}
		} else if !sawSpec {
			output = append(output, specLines...)
		}
	}
	for _, line := range lines {
		trimmed := strings.TrimRight(line, " \t\r")
		if registrySectionRe.MatchString(trimmed) {
			inRegistry = true
			inSpec = false
			sawSpec = false
			wroteSource = false
			wroteTemplate = false
			output = append(output, line)
			continue
		}
		if inRegistry && trimmed != "" && !strings.HasPrefix(trimmed, " ") {
			flushSpec()
			inRegistry = false
			inSpec = false
		}
		if inRegistry && registrySpecRe.MatchString(trimmed) {
			inSpec = true
			sawSpec = true
			wroteSource = false
			wroteTemplate = false
			output = append(output, line)
			continue
		}
		if inSpec && registrySourceRe.MatchString(trimmed) {
			output = append(output, "    source: "+normalizedSource)
			wroteSource = true
			continue
		}
		if inSpec && registryTmplRe.MatchString(trimmed) {
			if cfg.Template != "" {
				output = append(output, "    template: "+cfg.Template)
				wroteTemplate = true
			}
			continue
		}
		if inSpec && trimmed != "" && !strings.HasPrefix(trimmed, "    ") {
			flushSpec()
			inSpec = false
		}
		if inRegistry && trimmed != "" && !strings.HasPrefix(trimmed, " ") {
			inRegistry = false
		}
		output = append(output, line)
	}
	if inRegistry {
		flushSpec()
	}
	return strings.Join(output, "\n")
}

// appendSpecRegistrySection 在文件末尾追加 registry: 一节（中文注释）。
func appendSpecRegistrySection(content string, cfg SpecRegistryConfig) string {
	section := strings.Join([]string{
		"",
		"#-------------------------------------------------------------------------------",
		"# Registry",
		"#-------------------------------------------------------------------------------",
		"",
		"# 安装 .sunshine/spec 所用的来源。sunshine update 会按这里持续刷新",
		"# 受哈希追踪的 spec 模板，本地改动走正常的 update 冲突流程。",
		"registry:",
	}, "\n")
	section += "\n" + strings.Join(specRegistryLines(cfg), "\n") + "\n"
	return strings.TrimRight(content, " \t\r\n") + "\n" + section
}

// writeFileAtomic 经临时文件 + 改名写盘，避免写一半被中断。
func writeFileAtomic(path, content string) error {
	tmp, err := os.CreateTemp(filepath.Dir(path), ".config.yaml.*")
	if err != nil {
		return fmt.Errorf("写入配置文件失败：%w", err)
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)
	if _, err := tmp.WriteString(content); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("写入配置文件失败：%w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("写入配置文件失败：%w", err)
	}
	if err := os.Chmod(tmpName, 0o644); err != nil {
		return fmt.Errorf("写入配置文件失败：%w", err)
	}
	if err := os.Rename(tmpName, path); err != nil {
		return fmt.Errorf("写入配置文件失败：%w", err)
	}
	return nil
}
