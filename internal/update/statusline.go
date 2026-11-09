package update

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// ClaudeSettingsPath 是 Claude Code 设置文件的相对路径。
const ClaudeSettingsPath = ".claude/settings.json"

// PreserveExistingClaudeStatusLine 把磁盘上已有的 statusLine 带进
// 新模板内容（上游同名函数）：仅当新模板有 settings.json、
// 磁盘文件存在且含有 statusLine、而新模板没有 statusLine 时才处理；
// 新模板已有 statusLine 时不动（模板的胜出）；
// 磁盘 JSON 非法时不动（走正常冲突流程）。
// templates 的值会被原地更新。
func PreserveExistingClaudeStatusLine(cwd string, templates map[string]string) {
	newContent, ok := templates[ClaudeSettingsPath]
	if !ok {
		return
	}
	data, err := os.ReadFile(filepath.Join(cwd, filepath.FromSlash(ClaudeSettingsPath)))
	if err != nil {
		return
	}
	var existing map[string]json.RawMessage
	if err := json.Unmarshal(data, &existing); err != nil {
		return
	}
	rawStatusLine, ok := existing["statusLine"]
	if !ok {
		return
	}
	var fresh map[string]json.RawMessage
	if err := json.Unmarshal([]byte(newContent), &fresh); err != nil {
		return
	}
	if _, ok := fresh["statusLine"]; ok {
		return
	}
	// 把已有的 statusLine 追加到新模板对象末尾（2 空格缩进 + 末尾换行，
	// 与上游 JSON.stringify(newSettings, null, 2) + "\n" 一致）。
	// 用保序方式重建：先写新模板的全部键，再追加 statusLine。
	// 为保持键顺序与原始模板一致，这里做行级拼接而不是重排：
	// 去掉新内容末尾的 "}\n"，在最后一个键后加逗号并插入 statusLine。
	trimmed := strings.TrimRight(newContent, " \t\r\n")
	if !strings.HasSuffix(trimmed, "}") {
		return
	}
	body := strings.TrimSuffix(trimmed, "}")
	body = strings.TrimRight(body, " \t\r\n")
	var buf bytes.Buffer
	buf.WriteString(body)
	if strings.TrimSpace(body) != "{" {
		buf.WriteString(",")
	}
	buf.WriteString("\n  \"statusLine\": ")
	indented, err := indentJSONValue(rawStatusLine, 2)
	if err != nil {
		return
	}
	buf.Write(indented)
	buf.WriteString("\n}\n")
	templates[ClaudeSettingsPath] = buf.String()
}

// indentJSONValue 把一段 JSON 值按 2 空格缩进重排（顶层相对缩进 2）。
// statusLine 的值是对象：把它展开为多行、每行前加 2 空格。
func indentJSONValue(raw json.RawMessage, baseIndent int) ([]byte, error) {
	var v any
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	if err := dec.Decode(&v); err != nil {
		return nil, fmt.Errorf("解析 statusLine 失败：%w", err)
	}
	out, err := json.MarshalIndent(v, strings.Repeat(" ", baseIndent), "  ")
	if err != nil {
		return nil, err
	}
	// MarshalIndent 的第一行没有前缀缩进；调用方已写了键与冒号，
	// 这里返回从 "{" 开始的全文即可（后续行自带缩进）。
	return out, nil
}
