package migrate

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

var separatorLineRe = regexp.MustCompile(`^#-{3,}\s*$`)

func isSeparatorLine(line string) bool {
	return separatorLineRe.MatchString(strings.TrimRight(line, " \t"))
}

// ExtractConfigSection 从模板内容里按 SectionHeading 提取一个配置段落。
//
// 段落格式与模板 config.yaml 的分隔块一致：
// #---...---
// # <标题>
// #---...---
// <段落正文……>
//
// 返回从起始分隔块到下一分隔块（或文件末尾）之间的文本（去掉尾部空行），
// 找不到时返回 ""。
func ExtractConfigSection(template, sectionHeading string) string {
	lines := strings.Split(template, "\n")
	for i := 0; i+2 < len(lines); i++ {
		if !isSeparatorLine(lines[i]) {
			continue
		}
		headingLine := lines[i+1]
		if !strings.HasPrefix(headingLine, "# ") || !isSeparatorLine(lines[i+2]) {
			continue
		}
		if strings.TrimSpace(strings.TrimPrefix(headingLine, "# ")) != sectionHeading {
			continue
		}
		end := len(lines)
		for j := i + 3; j < len(lines); j++ {
			if isSeparatorLine(lines[j]) {
				end = j
				break
			}
		}
		return strings.TrimRight(strings.Join(lines[i:end], "\n"), "\n")
	}
	return ""
}

// ApplyConfigSectionsAdded 把区间内新引入的配置段落追加到用户已有的
// 配置文件。追加是幂等的：按 file+sentinel 去重，文件已含 sentinel
// 时跳过；目标文件不存在、模板里找不到该段落、写盘失败时跳过该条。
// 返回成功追加的段落数。
func ApplyConfigSectionsAdded(entries []ConfigSectionAdded, cwd string, templates map[string]string) int {
	seen := map[string]bool{}
	appended := 0
	for _, entry := range entries {
		dedupeKey := entry.File + "::" + entry.Sentinel
		if seen[dedupeKey] {
			continue
		}
		seen[dedupeKey] = true
		targetPath := filepath.Join(cwd, filepath.FromSlash(entry.File))
		userContent, err := os.ReadFile(targetPath)
		if err != nil {
			continue
		}
		if strings.Contains(string(userContent), entry.Sentinel) {
			continue
		}
		template, ok := templates[filepath.ToSlash(entry.File)]
		if !ok {
			continue
		}
		section := ExtractConfigSection(template, entry.SectionHeading)
		if section == "" {
			continue
		}
		separator := "\n"
		if !strings.HasSuffix(string(userContent), "\n") {
			separator = "\n\n"
		}
		newContent := string(userContent) + separator + section + "\n"
		if err := os.WriteFile(targetPath, []byte(newContent), 0o644); err != nil {
			continue
		}
		appended++
	}
	return appended
}
