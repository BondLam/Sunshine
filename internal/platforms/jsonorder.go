// ordered JSON 解析：用于 .claude/settings.json 的 statusLine 注入
// （injectStatusLine）与 update 的 statusLine 保留。
//
// Trellis 的 settings.json 处理（update.ts preserveExistingClaudeStatusLine、
// claude.ts injectStatusLine）是“解析 → 末尾赋值 → stringify(null, 2)”：
// 已有键的顺序与值保持不变，新键追加到对象末尾。Go 的 encoding/json
// 对 map 按键排序重排，会打乱用户原有顺序，因此这里用一棵保序的
// JSON 树实现解析与序列化，只用标准库。
//
// 注意：阶段 2 的 mergeSettingsJSON（保留全部顶层键、env/hooks 补写）
// 已在阶段 4 删除——那不是 Trellis 的行为；settings.json 走普通模板
// 文件流程，仅 statusLine 由 update 保留（见 NOTES.md 第 3 节）。
package platforms

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
)

// orderedObject 是保序的 JSON 对象：keys 记录首次出现的顺序。
type orderedObject struct {
	keys []string
	vals map[string]any
}

// set 设置键值；新键追加到末尾，已有键只换值、位置不动。
func (o *orderedObject) set(key string, value any) {
	if _, ok := o.vals[key]; !ok {
		o.keys = append(o.keys, key)
	}
	o.vals[key] = value
}

// get 返回键值与是否存在。
func (o *orderedObject) get(key string) (any, bool) {
	v, ok := o.vals[key]
	return v, ok
}

// parseOrderedJSON 解析 JSON 文本；顶层必须是对象。
// 数字用 json.Number 保留原始字面量，避免 30 被重写为 30.0 之类。
func parseOrderedJSON(data []byte) (*orderedObject, error) {
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.UseNumber()
	v, err := parseJSONValue(dec)
	if err != nil {
		return nil, err
	}
	obj, ok := v.(*orderedObject)
	if !ok {
		return nil, fmt.Errorf("settings.json 顶层不是 JSON 对象")
	}
	return obj, nil
}

func parseJSONValue(dec *json.Decoder) (any, error) {
	tok, err := dec.Token()
	if err != nil {
		return nil, err
	}
	switch t := tok.(type) {
	case json.Delim:
		switch t {
		case '{':
			obj := &orderedObject{vals: make(map[string]any)}
			for dec.More() {
				keyTok, err := dec.Token()
				if err != nil {
					return nil, err
				}
				key, ok := keyTok.(string)
				if !ok {
					return nil, fmt.Errorf("JSON 对象键不是字符串")
				}
				v, err := parseJSONValue(dec)
				if err != nil {
					return nil, err
				}
				obj.set(key, v)
			}
			if _, err := dec.Token(); err != nil {
				return nil, err
			}
			return obj, nil
		case '[':
			var arr []any
			for dec.More() {
				v, err := parseJSONValue(dec)
				if err != nil {
					return nil, err
				}
				arr = append(arr, v)
			}
			if _, err := dec.Token(); err != nil {
				return nil, err
			}
			return arr, nil
		default:
			return nil, fmt.Errorf("意外的 JSON 分隔符 %v", t)
		}
	default:
		return t, nil
	}
}

// marshalIndent 按 2 空格缩进序列化（不含末尾换行，由调用方补）。
// 字符串转义与 encoding/json 一致，但不转义 HTML 字符
// （<、>、& 保持原样，与 Trellis JSON.stringify 的行为一致）。
func (o *orderedObject) marshalIndent() ([]byte, error) {
	var buf bytes.Buffer
	if err := writeJSONValue(&buf, o, 0); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func writeJSONValue(buf *bytes.Buffer, v any, indent int) error {
	switch t := v.(type) {
	case *orderedObject:
		if len(t.keys) == 0 {
			buf.WriteString("{}")
			return nil
		}
		buf.WriteString("{\n")
		for i, k := range t.keys {
			buf.WriteString(strings.Repeat(" ", indent+2))
			if err := writeJSONString(buf, k); err != nil {
				return err
			}
			buf.WriteString(": ")
			if err := writeJSONValue(buf, t.vals[k], indent+2); err != nil {
				return err
			}
			if i < len(t.keys)-1 {
				buf.WriteString(",")
			}
			buf.WriteString("\n")
		}
		buf.WriteString(strings.Repeat(" ", indent))
		buf.WriteString("}")
		return nil
	case []any:
		if len(t) == 0 {
			buf.WriteString("[]")
			return nil
		}
		buf.WriteString("[\n")
		for i, e := range t {
			buf.WriteString(strings.Repeat(" ", indent+2))
			if err := writeJSONValue(buf, e, indent+2); err != nil {
				return err
			}
			if i < len(t)-1 {
				buf.WriteString(",")
			}
			buf.WriteString("\n")
		}
		buf.WriteString(strings.Repeat(" ", indent))
		buf.WriteString("]")
		return nil
	case string:
		return writeJSONString(buf, t)
	case json.Number:
		buf.WriteString(t.String())
		return nil
	case bool:
		if t {
			buf.WriteString("true")
		} else {
			buf.WriteString("false")
		}
		return nil
	case nil:
		buf.WriteString("null")
		return nil
	default:
		// float64 等其他标量（不应出现，防御性分支）
		raw, err := json.Marshal(t)
		if err != nil {
			return err
		}
		buf.Write(raw)
		return nil
	}
}

func writeJSONString(buf *bytes.Buffer, s string) error {
	enc := json.NewEncoder(buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(s); err != nil {
		return err
	}
	// Encoder.Encode 追加了换行，去掉。
	buf.Truncate(buf.Len() - 1)
	return nil
}
