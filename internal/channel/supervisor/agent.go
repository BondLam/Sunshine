package supervisor

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// AgentDefinition 是 .sunshine/agents/<name>.md 的 agent 定义：
// YAML frontmatter（--- 围栏之间）+ markdown 正文，正文成为注入
// worker 的系统提示词。
//
//	---
//	name: architect
//	description: 系统架构师……
//	provider: claude       # claude | codex；用作默认 --provider
//	model: claude-opus-4-7 # CLI 专属模型 id；可选
//	labels: [design]       # 可选元数据
//	---
//
//	你是资深系统架构师……
//
// 未知的 frontmatter 字段保留在 Raw 里（供其他层消费），channel 运行时忽略。
type AgentDefinition struct {
	Name         string
	Description  string
	Provider     string // claude | codex；空表示未指定
	Model        string
	Labels       []string
	SystemPrompt string
	Raw          map[string]string
	FilePath     string
}

var safeAgentNameRe = regexp.MustCompile(`^[A-Za-z0-9._-]+$`)

// FindAgentFile 找 agent 定义文件：
// .sunshine/agents/<name>.md，备选 .sunshine/agents/<name>/AGENT.md。
// 找不到返回 ""。名字不安全（..、/ 等路径穿越）时报错。
// 纵深防御：解析出的真实路径须在 agents 根目录内，或在信任根内
// （容纳经信任的 .sunshine 符号链接到达的 agents 目录）。
func FindAgentFile(name, cwd string, trustedRoots []string) (string, error) {
	if !safeAgentNameRe.MatchString(name) {
		return "", fmt.Errorf("agent 名 %q 不合法（只允许字母、数字、.、_、-）", name)
	}
	agentsRoot, err := filepath.Abs(filepath.Join(cwd, ".sunshine", "agents"))
	if err != nil {
		return "", err
	}
	candidates := []string{
		filepath.Join(agentsRoot, name+".md"),
		filepath.Join(agentsRoot, name, "AGENT.md"),
	}
	for _, p := range candidates {
		real := p
		if _, err := os.Stat(p); err == nil {
			if r, err := filepath.EvalSymlinks(p); err == nil {
				real = r
			}
		}
		inAgentsRoot := real == agentsRoot || isUnderRoot(real, agentsRoot)
		inTrustedRoot := false
		for _, root := range trustedRoots {
			if real == root || isUnderRoot(real, root) {
				inTrustedRoot = true
				break
			}
		}
		if !inAgentsRoot && !inTrustedRoot {
			continue
		}
		if _, err := os.Stat(p); err == nil {
			return p, nil
		}
	}
	return "", nil
}

// LoadAgent 加载 agent 定义：找不到或无 frontmatter 时报错。
func LoadAgent(name, cwd string, trustedRoots []string) (*AgentDefinition, error) {
	file, err := FindAgentFile(name, cwd, trustedRoots)
	if err != nil {
		return nil, err
	}
	if file == "" {
		return nil, fmt.Errorf("找不到 agent %q，查找位置：\n  %s\n  %s",
			name,
			filepath.Join(cwd, ".sunshine", "agents", name+".md"),
			filepath.Join(cwd, ".sunshine", "agents", name, "AGENT.md"))
	}
	raw, err := os.ReadFile(file)
	if err != nil {
		return nil, fmt.Errorf("读 agent 定义失败：%w", err)
	}
	frontmatter, body, ok := splitFrontmatter(string(raw))
	if !ok {
		return nil, fmt.Errorf("agent %q（%s）缺少 YAML frontmatter（文件顶部须有 --- ... --- 块）", name, file)
	}
	fm := parseFrontmatter(frontmatter)
	agentName := strings.TrimSpace(fm["name"])
	if agentName == "" {
		agentName = name
	}
	def := &AgentDefinition{
		Name:         agentName,
		Description:  strings.TrimSpace(fm["description"]),
		Provider:     normalizeAgentProvider(fm["provider"]),
		Model:        strings.TrimSpace(fm["model"]),
		SystemPrompt: strings.TrimSpace(body),
		Raw:          fm,
		FilePath:     file,
	}
	if labels := fm["labels"]; labels != "" {
		labels = strings.Trim(labels, "[]")
		for _, part := range strings.Split(labels, ",") {
			if s := strings.TrimSpace(part); s != "" {
				def.Labels = append(def.Labels, s)
			}
		}
	}
	return def, nil
}

func normalizeAgentProvider(v string) string {
	switch strings.TrimSpace(strings.ToLower(v)) {
	case "claude", "codex":
		return strings.TrimSpace(strings.ToLower(v))
	default:
		return ""
	}
}

var frontmatterFenceRe = regexp.MustCompile(`(?s)^---\s*\n(.*?)\n---\s*\n?(.*)$`)

// splitFrontmatter 拆 frontmatter 与正文；无围栏时返回 false。
func splitFrontmatter(raw string) (frontmatter, body string, ok bool) {
	m := frontmatterFenceRe.FindStringSubmatch(raw)
	if m == nil {
		return "", "", false
	}
	return m[1], m[2], true
}

// forbiddenFrontmatterKeys 是拒绝写入的危险 frontmatter 键
// （直接赋值会污染原型链；Go 虽无此风险，仍与上游一致拒绝）。
var forbiddenFrontmatterKeys = map[string]bool{
	"__proto__": true, "prototype": true, "constructor": true,
}

// parseFrontmatter 是极小扁平 YAML 解析器，处理本项目 frontmatter 的方言：
//
//	key: value
//	multiline_key: |
//	  第一行
//	  第二行
//
// 列表/嵌套对象超出方言时以原始字符串形式返回。
func parseFrontmatter(text string) map[string]string {
	out := map[string]string{}
	lines := strings.Split(text, "\n")
	i := 0
	keyRe := regexp.MustCompile(`^([A-Za-z0-9_-]+)\s*:\s*(.*)$`)
	for i < len(lines) {
		line := lines[i]
		if strings.TrimSpace(line) == "" || strings.HasPrefix(strings.TrimSpace(line), "#") {
			i++
			continue
		}
		m := keyRe.FindStringSubmatch(line)
		if m == nil {
			i++
			continue
		}
		key, inline := m[1], m[2]
		if forbiddenFrontmatterKeys[key] {
			fmt.Fprintf(os.Stderr, "[channel agent] 拒绝危险的 frontmatter 键 %q\n", key)
			// 仍要消费掉块标量的续行，避免续行被误解析为新键。
			i++
			if inline == "|" || inline == ">" {
				for i < len(lines) && lines[i] != "" && (lines[i][0] == ' ' || lines[i][0] == '\t') {
					i++
				}
			}
			continue
		}
		if inline == "|" || inline == ">" {
			var block []string
			i++
			for i < len(lines) {
				cont := lines[i]
				if cont != "" && cont[0] != ' ' && cont[0] != '\t' {
					break
				}
				block = append(block, strings.TrimPrefix(cont, "  "))
				i++
			}
			out[key] = strings.TrimSpace(strings.Join(block, "\n"))
		} else {
			out[key] = strings.TrimSpace(inline)
			i++
		}
	}
	return out
}
