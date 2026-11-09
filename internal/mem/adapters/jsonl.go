package adapters

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
)

// ReadJSONL 逐行解析 JSONL 文件，对每个解析出的对象调用 onLine。
// onLine 返回 false 时停止读取。坏行（空行除外：非 { 开头、JSON 解析失败）
// 跳过并汇总为一条警告返回；文件不存在或打不开时返回一条警告，不报错。
// 调用方把返回的警告并入自己的警告列表。
func ReadJSONL(path string, onLine func(obj map[string]any) bool) []string {
	f, err := os.Open(path)
	if err != nil {
		return []string{fmt.Sprintf("无法打开会话文件（%s）：%v，已跳过", path, err)}
	}
	defer f.Close()

	badLines := 0
	reader := bufio.NewReaderSize(f, 256*1024)
	for {
		line, err := reader.ReadString('\n')
		if line != "" {
			trimmed := strings.TrimRight(line, "\r\n")
			if strings.TrimSpace(trimmed) != "" {
				if !strings.HasPrefix(strings.TrimLeft(trimmed, " \t"), "{") {
					badLines++
				} else {
					var obj map[string]any
					if decErr := json.Unmarshal([]byte(trimmed), &obj); decErr != nil || obj == nil {
						badLines++
					} else if !onLine(obj) {
						break
					}
				}
			}
		}
		if err == io.EOF {
			break
		}
		if err != nil {
			badLines++
			break
		}
	}
	if badLines > 0 {
		return []string{fmt.Sprintf("文件 %s：跳过 %d 行无法解析的内容", path, badLines)}
	}
	return nil
}

// ReadJSONLFirst 读出文件中第一个可解析的 JSON 对象；没有时返回 nil。
func ReadJSONLFirst(path string) (map[string]any, []string) {
	var first map[string]any
	warnings := ReadJSONL(path, func(obj map[string]any) bool {
		first = obj
		return false
	})
	return first, warnings
}

// FindInJSONL 扫描文件，返回前 maxLines 行内第一个满足 predicate 的对象。
func FindInJSONL(path string, maxLines int, predicate func(obj map[string]any) bool) (map[string]any, []string) {
	var found map[string]any
	count := 0
	warnings := ReadJSONL(path, func(obj map[string]any) bool {
		count++
		if predicate(obj) {
			found = obj
			return false
		}
		return count < maxLines
	})
	return found, warnings
}

// ReadJSONFile 读出整个 JSON 文件并解析；失败时返回 nil。
func ReadJSONFile(path string) map[string]any {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	var obj map[string]any
	if err := json.Unmarshal(data, &obj); err != nil || obj == nil {
		return nil
	}
	return obj
}

// objStr 取对象的字符串字段；不是字符串时返回 ""。
func objStr(obj map[string]any, key string) string {
	if v, ok := obj[key]; ok {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return ""
}

// objBool 取对象的布尔字段。
func objBool(obj map[string]any, key string) bool {
	if v, ok := obj[key]; ok {
		if b, ok := v.(bool); ok {
			return b
		}
	}
	return false
}

// objMap 取对象的对象字段。
func objMap(obj map[string]any, key string) map[string]any {
	if v, ok := obj[key]; ok {
		if m, ok := v.(map[string]any); ok {
			return m
		}
	}
	return nil
}

// objList 取对象的数组字段。
func objList(obj map[string]any, key string) []any {
	if v, ok := obj[key]; ok {
		if l, ok := v.([]any); ok {
			return l
		}
	}
	return nil
}

// objNum 取对象的数字字段。
func objNum(obj map[string]any, key string) (float64, bool) {
	if v, ok := obj[key]; ok {
		if n, ok := v.(float64); ok {
			return n, true
		}
	}
	return 0, false
}
