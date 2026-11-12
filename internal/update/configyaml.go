package update

import (
	"regexp"
	"strings"

	"github.com/BondLam/Sunshine/internal/config"
)

// PreserveExistingRegistryConfig 把用户已有的 registry.spec 追加到
// 模板 config.yaml 末尾（上游同名函数）：这样用户唯一的改动是
// registry.spec 时，模板（含 registry 段）与磁盘内容一致，
// 不会误报为“你已修改”的冲突；registry.spec 段本身的注释写明
// 它由 update 持续刷新、本地编辑走正常冲突流程。
// 用户没有 registry.spec 时原样返回模板。
func PreserveExistingRegistryConfig(cwd, template string) string {
	// registry.spec 的行级读写归 B 组（internal/config），
	// 这里只读它来做追加；读取失败时当作无 registry 处理。
	registry, _ := config.LoadSpecRegistryConfig(cwd)
	if registry == nil {
		return template
	}
	var b strings.Builder
	b.WriteString(strings.TrimRight(template, " \t\r\n"))
	b.WriteString("\n\n")
	b.WriteString("#-------------------------------------------------------------------------------\n")
	b.WriteString("# Registry\n")
	b.WriteString("#-------------------------------------------------------------------------------\n")
	b.WriteString("\n")
	b.WriteString("# Source used to install .sunshine/spec. sunshine update refreshes this\n")
	b.WriteString("# hash-tracked spec template while preserving local edits through the\n")
	b.WriteString("# normal update conflict flow.\n")
	b.WriteString("registry:\n")
	b.WriteString("  spec:\n")
	b.WriteString("    source: " + registry.Source + "\n")
	if registry.Template != "" {
		b.WriteString("    template: " + registry.Template + "\n")
	}
	return b.String()
}

// ConfigSection 是 config.yaml 模板里的一个段落（以 `#---...---`
// 分隔线 + `# 标题` 开头，直到下一分隔线或文件尾）。
type ConfigSection struct {
	// Heading 是段落标题（如“会话记录”）。
	Heading string
	// Text 是段落全文（含开头分隔线块，不含末尾空行）。
	Text string
	// Keys 是段落内未注释的顶层键（如 session_commit_message）。
	Keys []string
}

var (
	reConfigSeparator = regexp.MustCompile(`^#-{3,}\s*$`)
	reConfigTopKey    = regexp.MustCompile(`^([A-Za-z0-9_-]+):`)
)

// ExtractConfigSections 把模板按段落切分（上游 extractConfigSection
// 的泛化版：一次取出全部段落，供补齐与迁移段落追加共用）。
func ExtractConfigSections(template string) []ConfigSection {
	lines := strings.Split(template, "\n")
	isSeparator := func(line string) bool {
		return reConfigSeparator.MatchString(strings.TrimRight(line, " \t\r"))
	}
	var sections []ConfigSection
	for i := 0; i < len(lines); i++ {
		if !isSeparator(lines[i]) {
			continue
		}
		if i+2 >= len(lines) {
			continue
		}
		headingLine, closing := lines[i+1], lines[i+2]
		if !strings.HasPrefix(headingLine, "# ") || !isSeparator(closing) {
			continue
		}
		heading := strings.TrimSpace(strings.TrimPrefix(headingLine, "# "))
		end := len(lines)
		for j := i + 3; j < len(lines); j++ {
			if isSeparator(lines[j]) {
				end = j
				break
			}
		}
		text := strings.Join(lines[i:end], "\n")
		text = strings.TrimRight(text, " \t\r\n")
		var keys []string
		for _, line := range lines[i+3 : end] {
			trimmed := strings.TrimRight(line, " \t\r")
			if trimmed == "" || strings.HasPrefix(trimmed, "#") ||
				strings.HasPrefix(trimmed, " ") || strings.HasPrefix(trimmed, "\t") {
				continue
			}
			if m := reConfigTopKey.FindStringSubmatch(trimmed); m != nil {
				keys = append(keys, m[1])
			}
		}
		sections = append(sections, ConfigSection{Heading: heading, Text: text, Keys: keys})
		// 下一循环从 end 处继续找分隔线（end 本身就是分隔线）。
		i = end - 1
	}
	return sections
}

// sectionPresent 判断用户文件里是否已有该段落：标题行在、
// 或段落内任一顶层键在，即算已有（避免重复追加；
// 用户手写 registry 块没有 "# Registry" 标题时，
// `registry:` 键本身就是哨兵）。
func sectionPresent(userContent string, section ConfigSection) bool {
	if strings.Contains(userContent, "# "+section.Heading) {
		return true
	}
	for _, line := range strings.Split(userContent, "\n") {
		trimmed := strings.TrimRight(line, " \t\r")
		if trimmed == "" || strings.HasPrefix(trimmed, "#") ||
			strings.HasPrefix(trimmed, " ") || strings.HasPrefix(trimmed, "\t") {
			continue
		}
		for _, key := range section.Keys {
			if strings.HasPrefix(trimmed, key+":") {
				return true
			}
		}
	}
	return false
}

// AppendMissingConfigSections 把模板里用户文件缺失的段落追加到用户内容末尾
// （上游 applyConfigSectionsAdded 的通用版，只加不改）：
// 用户已有的行（字段、注释、自定义段落）一行不动；
// 缺失的模板段落按原文追加。返回新内容与追加的段落标题。
// 模板里没有段落、或用户文件已齐全时原样返回。
func AppendMissingConfigSections(userContent, template string) (string, []string) {
	var appended []string
	result := userContent
	for _, section := range ExtractConfigSections(template) {
		if sectionPresent(result, section) {
			continue
		}
		// 与上游一致：文件以换行结尾时加一个空行再接段落，
		// 否则先补两个换行。
		if strings.HasSuffix(result, "\n") {
			result += "\n" + section.Text + "\n"
		} else {
			result += "\n\n" + section.Text + "\n"
		}
		appended = append(appended, section.Heading)
	}
	return result, appended
}
