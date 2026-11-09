package lifecycle

import (
	"regexp"
	"strings"

	"github.com/linrong1982/Sunshine/templates"
)

// 结构化配置文件的清理器（对照 utils/uninstall-scrubbers.ts）。
//
// 每个清理器接收文件内容，返回清理后的内容与“清理后是否只剩
// 空壳”（fullyEmpty 为真时调用方直接删文件）。
//
// Go 版只覆盖三平台实际写出的结构化文件：
//   - .claude/settings.json、.codex/hooks.json（嵌套 hooks 形）
//   - .cursor/hooks.json（扁平 hooks 形）
//   - .codex/config.toml
// 上游其余平台的清理器（opencode package.json、pi settings.json、
// copilot instructions 托管块、根 AGENTS.md 托管块）在 Go 版没有
// 对应文件——Go 版 init 不写这些文件，清单里出现即视为孤儿键剪掉。
// 滚动轨道接入新平台时在此补对应的清理器。

// ScrubResult 是清理器的返回：清理后的内容与是否已空。
type ScrubResult struct {
	Content    string
	FullyEmpty bool
}

// isSunshineHookCommand 判断一条 hook 命令是否为 Sunshine 托管的。
// Go 版模板里的 hook 命令形如
// `SUNSHINE_HOOK_PLATFORM=<平台> sunshine hook <事件>`，
// 不再指向某个脚本文件，因此按“命令含 sunshine hook”判定；
// 上游按“命令末 token 是被删清单路径”判定，
// 是同一条“只认自家托管命令”的规则在两种 hook 形态下的写法。
func isSunshineHookCommand(command string) bool {
	return strings.Contains(command, "sunshine hook")
}

// entryCommand 从一条 hook 条目里读出 command（或 Copilot 式的
// bash / powershell）字符串；不是对象或没有命令时返回 ""。
func entryCommand(entry any) string {
	obj, ok := entry.(*orderedObject)
	if !ok {
		return ""
	}
	for _, key := range []string{"command", "bash", "powershell"} {
		if v, ok := obj.get(key); ok {
			if s, ok := v.(string); ok {
				return s
			}
		}
	}
	return ""
}

// scrubHooksJSON 清理 hooks 形 JSON 文件。
//
// mode = "nested" → hooks.{事件}.[ {matcher?, hooks: [ {command,...} ]} ]
// （.claude/settings.json、.codex/hooks.json）；
// mode = "flat" → hooks.{事件}.[ {command,...} ]
// （.cursor/hooks.json）。
//
// 删每条 Sunshine 托管的 hook 条目，再自底向上清理空容器
// （matcher 块、事件数组、hooks 对象）。Sunshine 的 Claude
// statusLine 命令同样删掉；hooks 之外的键（env、model、
// permissions 等）一律保留。JSON 非法时原样不动（调用方跳过）。
func scrubHooksJSON(content, mode string) ScrubResult {
	root, err := parseOrderedJSON([]byte(content))
	if err != nil {
		return ScrubResult{Content: content, FullyEmpty: false}
	}

	// statusLine：命令是 Sunshine statusline 时删掉。
	if v, ok := root.get("statusLine"); ok {
		if obj, ok := v.(*orderedObject); ok {
			if cmd := entryCommand(obj); cmd != "" && strings.Contains(cmd, "sunshine hook statusline") {
				root.delete("statusLine")
			}
		}
	}

	hooksVal, ok := root.get("hooks")
	if !ok {
		// 无 hooks 块：只有整个文件已无其他键时才算空。
		out, _ := root.marshalIndent()
		return ScrubResult{Content: string(out) + "\n", FullyEmpty: len(root.keys) == 0}
	}
	hooksObj, ok := hooksVal.(*orderedObject)
	if !ok {
		return ScrubResult{Content: content, FullyEmpty: false}
	}

	for _, eventName := range append([]string(nil), hooksObj.keys...) {
		eventVal, _ := hooksObj.get(eventName)
		eventArr, ok := eventVal.([]any)
		if !ok {
			continue
		}
		var filtered []any
		for _, entry := range eventArr {
			if mode == "flat" {
				if cmd := entryCommand(entry); cmd != "" && isSunshineHookCommand(cmd) {
					continue
				}
				filtered = append(filtered, entry)
				continue
			}
			// 嵌套形：条目是 { matcher?, hooks: [...] }。
			block, ok := entry.(*orderedObject)
			if !ok {
				filtered = append(filtered, entry)
				continue
			}
			innerVal, ok := block.get("hooks")
			if !ok {
				filtered = append(filtered, entry)
				continue
			}
			innerArr, ok := innerVal.([]any)
			if !ok {
				filtered = append(filtered, entry)
				continue
			}
			var filteredInner []any
			for _, sub := range innerArr {
				if cmd := entryCommand(sub); cmd != "" && isSunshineHookCommand(cmd) {
					continue
				}
				filteredInner = append(filteredInner, sub)
			}
			if len(filteredInner) == 0 {
				continue // 整个 matcher 块已空，删块。
			}
			rebuilt := &orderedObject{vals: make(map[string]any)}
			for _, k := range block.keys {
				if k == "hooks" {
					rebuilt.set(k, filteredInner)
				} else {
					v, _ := block.get(k)
					rebuilt.set(k, v)
				}
			}
			filtered = append(filtered, rebuilt)
		}
		if len(filtered) == 0 {
			hooksObj.delete(eventName)
		} else {
			hooksObj.set(eventName, filtered)
		}
	}

	if len(hooksObj.keys) == 0 {
		root.delete("hooks")
	}
	out, err := root.marshalIndent()
	if err != nil {
		return ScrubResult{Content: content, FullyEmpty: false}
	}
	return ScrubResult{Content: string(out) + "\n", FullyEmpty: len(root.keys) == 0}
}

var (
	reCodexFallbackFilenames = regexp.MustCompile(`^\s*project_doc_fallback_filenames\s*=`)
	reCodexAgentsHeader      = regexp.MustCompile(`^\s*\[agents\]\s*$`)
	reCodexTableHeader       = regexp.MustCompile(`^\s*\[\[?[^\]]+\]\]?\s*$`)
	reCodexMaxDepth          = regexp.MustCompile(`^\s*max_depth\s*=\s*1\s*(?:#.*)?$`)
)

// codexTemplateCommentMarkers 返回当前内嵌 Codex config 模板里的
// 注释行内容集合（去掉 "#" 与空白后的文本）。对照上游
// scrubCodexConfigToml 从 getCodexConfigTemplate() 取当前模板
// 注释的做法：模板变了，清理器认的注释也跟着变。
func codexTemplateCommentMarkers() map[string]bool {
	markers := map[string]bool{}
	raw, err := templates.Platforms.ReadFile("platforms/codex/config.toml")
	if err != nil {
		return markers
	}
	for _, line := range strings.Split(string(raw), "\n") {
		trimmed := strings.TrimSpace(line)
		if !strings.HasPrefix(trimmed, "#") {
			continue
		}
		inner := strings.TrimSpace(strings.TrimLeft(trimmed, "#"))
		inner = strings.TrimSpace(inner)
		if inner != "" {
			markers[inner] = true
		}
	}
	return markers
}

// isSunshineCodexCommentLine 判断一行注释是否属于 Sunshine 模板：
// 内容（去掉 "#" 与空白后）命中当前模板注释集合，或是空 "#" 行。
func isSunshineCodexCommentLine(line string, markers map[string]bool) bool {
	trimmed := strings.TrimSpace(line)
	if !strings.HasPrefix(trimmed, "#") {
		return false
	}
	inner := strings.TrimSpace(strings.TrimLeft(trimmed, "#"))
	if inner == "" {
		return true
	}
	return markers[inner]
}

// scrubCodexConfigToml 清理 .codex/config.toml（行级删除）：
//   - `project_doc_fallback_filenames = ...` 行；
//   - Sunshine 模板的注释行；
//   - `[agents]` 表里 Sunshine 的 `max_depth = 1`（表内还有用户键
//     时保留表头，否则整表去掉）；
//   - 删除后多余的空行压成一行，尾部空行去掉。
//
// 用户自加的非模板行原样保留；清理后无非空白字符即 fullyEmpty。
func scrubCodexConfigToml(content string) ScrubResult {
	markers := codexTemplateCommentMarkers()
	var kept []string
	for _, line := range strings.Split(content, "\n") {
		if reCodexFallbackFilenames.MatchString(line) {
			continue
		}
		if isSunshineCodexCommentLine(line, markers) {
			continue
		}
		kept = append(kept, line)
	}

	// 去掉 [agents] 表里的 max_depth = 1，表内还有用户内容时保留表头。
	var out []string
	for i := 0; i < len(kept); i++ {
		line := kept[i]
		if !reCodexAgentsHeader.MatchString(line) {
			out = append(out, line)
			continue
		}
		var tableLines []string
		j := i + 1
		for j < len(kept) && !reCodexTableHeader.MatchString(kept[j]) {
			if !reCodexMaxDepth.MatchString(kept[j]) {
				tableLines = append(tableLines, kept[j])
			}
			j++
		}
		hasUserContent := false
		for _, candidate := range tableLines {
			trimmed := strings.TrimSpace(candidate)
			if trimmed != "" && !strings.HasPrefix(trimmed, "#") {
				hasUserContent = true
				break
			}
		}
		if hasUserContent {
			out = append(out, line)
			out = append(out, tableLines...)
		} else {
			for _, candidate := range tableLines {
				if strings.TrimSpace(candidate) != "" {
					out = append(out, candidate)
				}
			}
		}
		i = j - 1
	}

	// 压掉删除产生的连续空行，去尾部空行。
	var collapsed []string
	prevBlank := true // 行首视为空白起手
	for _, line := range out {
		blank := strings.TrimSpace(line) == ""
		if blank && prevBlank {
			continue
		}
		collapsed = append(collapsed, line)
		prevBlank = blank
	}
	for len(collapsed) > 0 && strings.TrimSpace(collapsed[len(collapsed)-1]) == "" {
		collapsed = collapsed[:len(collapsed)-1]
	}
	result := ""
	if len(collapsed) > 0 {
		result = strings.Join(collapsed, "\n") + "\n"
	}
	return ScrubResult{Content: result, FullyEmpty: strings.TrimSpace(result) == ""}
}
