// Package task 实现 sunshine task 任务系统：task.json（24 字段）读写、
// 任务生命周期命令（对应 Trellis 的 task.py / add_session.py /
// get_context.py）、工作区 journal 与 shell 票据消费。
//
// 行为对照 Trellis 源码重新实现，不复制源码：
//   - 记录形态与字段顺序见 packages/core/src/task/schema.ts
//     （TASK_RECORD_FIELD_ORDER）与 records.ts（writeTaskRecord 的
//     未知字段合并保留）；
//   - 生命周期见 .trellis/scripts/task.py 与 common/task_store.py、
//     task_context.py、task_utils.py、tasks.py；
//   - 会话记录见 add_session.py；上下文见 get_context.py 与
//     common/session_context.py、git_context.py。
package task

import (
	"bytes"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"
)

// FieldOrder 是 task.json 的 24 个字段的 canonical 顺序，与
// schema.ts 的 TASK_RECORD_FIELD_ORDER 完全一致。Go 的 encoding/json
// 不保序，写盘顺序由本包的手工编码保证。
var FieldOrder = []string{
	"id", "name", "title", "description", "status",
	"dev_type", "scope", "package", "priority", "creator", "assignee",
	"createdAt", "completedAt", "branch", "base_branch", "worktree_path",
	"commit", "pr_url", "subtasks", "children", "parent", "relatedFiles",
	"notes", "meta",
}

var stringFields = map[string]bool{
	"id": true, "name": true, "title": true, "description": true,
	"status": true, "priority": true, "creator": true, "assignee": true,
	"createdAt": true, "notes": true,
}

var nullableStringFields = map[string]bool{
	"dev_type": true, "scope": true, "package": true, "completedAt": true,
	"branch": true, "base_branch": true, "worktree_path": true,
	"commit": true, "pr_url": true, "parent": true,
}

var stringArrayFields = map[string]bool{
	"subtasks": true, "children": true, "relatedFiles": true,
}

func isCanonicalField(name string) bool {
	for _, f := range FieldOrder {
		if f == name {
			return true
		}
	}
	return false
}

// TaskRecord 是 task.json 的 24 字段记录。Meta 以原始 JSON 对象保存，
// 写盘时原样保留键顺序；其余字段按 FieldOrder 写盘。
type TaskRecord struct {
	ID           string
	Name         string
	Title        string
	Description  string
	Status       string
	DevType      *string
	Scope        *string
	Package      *string
	Priority     string
	Creator      string
	Assignee     string
	CreatedAt    string
	CompletedAt  *string
	Branch       *string
	BaseBranch   *string
	Worktree     *string
	Commit       *string
	PRURL        *string
	Subtasks     []string
	Children     []string
	Parent       *string
	RelatedFiles []string
	Notes        string
	// Meta 是原始 JSON 对象文本（已校验为对象），默认 "{}"。
	Meta json.RawMessage
	// Unknown 是磁盘已有 JSON 里的非 canonical 字段（按原顺序），
	// 结构化读取时不返回，写盘时合并保留。
	Unknown []UnknownField
}

// UnknownField 是磁盘 task.json 里手加的未知字段。
type UnknownField struct {
	Key   string
	Value json.RawMessage
}

func strptr(s string) *string { return &s }

// EmptyRecord 返回带默认值的空记录：status "planning"、priority "P2"、
// createdAt 当天日期、subtasks/children/relatedFiles 为 []、meta 为 {}，
// 其余可空字段为 null。
func EmptyRecord() *TaskRecord {
	return &TaskRecord{
		Status:       "planning",
		Priority:     "P2",
		CreatedAt:    time.Now().Format("2006-01-02"),
		Subtasks:     []string{},
		Children:     []string{},
		RelatedFiles: []string{},
		Meta:         json.RawMessage("{}"),
	}
}

// fieldValue 取记录里某字段的写盘值。
func (r *TaskRecord) fieldValue(field string) any {
	switch field {
	case "id":
		return r.ID
	case "name":
		return r.Name
	case "title":
		return r.Title
	case "description":
		return r.Description
	case "status":
		return r.Status
	case "dev_type":
		return r.DevType
	case "scope":
		return r.Scope
	case "package":
		return r.Package
	case "priority":
		return r.Priority
	case "creator":
		return r.Creator
	case "assignee":
		return r.Assignee
	case "createdAt":
		return r.CreatedAt
	case "completedAt":
		return r.CompletedAt
	case "branch":
		return r.Branch
	case "base_branch":
		return r.BaseBranch
	case "worktree_path":
		return r.Worktree
	case "commit":
		return r.Commit
	case "pr_url":
		return r.PRURL
	case "subtasks":
		return r.Subtasks
	case "children":
		return r.Children
	case "parent":
		return r.Parent
	case "relatedFiles":
		return r.RelatedFiles
	case "notes":
		return r.Notes
	case "meta":
		return r.Meta
	}
	return nil
}

// setFieldValue 按 canonical 校验后的值写记录字段。
func (r *TaskRecord) setFieldValue(field string, value any) {
	switch field {
	case "id":
		r.ID = value.(string)
	case "name":
		r.Name = value.(string)
	case "title":
		r.Title = value.(string)
	case "description":
		r.Description = value.(string)
	case "status":
		r.Status = value.(string)
	case "dev_type":
		r.DevType = value.(*string)
	case "scope":
		r.Scope = value.(*string)
	case "package":
		r.Package = value.(*string)
	case "priority":
		r.Priority = value.(string)
	case "creator":
		r.Creator = value.(string)
	case "assignee":
		r.Assignee = value.(string)
	case "createdAt":
		r.CreatedAt = value.(string)
	case "completedAt":
		r.CompletedAt = value.(*string)
	case "branch":
		r.Branch = value.(*string)
	case "base_branch":
		r.BaseBranch = value.(*string)
	case "worktree_path":
		r.Worktree = value.(*string)
	case "commit":
		r.Commit = value.(*string)
	case "pr_url":
		r.PRURL = value.(*string)
	case "subtasks":
		r.Subtasks = value.([]string)
	case "children":
		r.Children = value.([]string)
	case "parent":
		r.Parent = value.(*string)
	case "relatedFiles":
		r.RelatedFiles = value.([]string)
	case "notes":
		r.Notes = value.(string)
	case "meta":
		r.Meta = value.(json.RawMessage)
	}
}

// ParseRecord 按 schema.ts 校验并解析 task.json 内容：24 个字段全部
// 必填，缺失即报错、不补默认值；四类按集合校验类型。未知字段不进入
// 结构化结果（调用方用 WriteRecord 合并保留）。
func ParseRecord(data []byte) (*TaskRecord, error) {
	var raw map[string]json.RawMessage
	dec := json.NewDecoder(bytes.NewReader(data))
	if err := dec.Decode(&raw); err != nil {
		return nil, fmt.Errorf("task.json 不是合法 JSON：%v", err)
	}
	if raw == nil {
		return nil, fmt.Errorf("task.json 顶层不是 JSON 对象")
	}
	rec := &TaskRecord{}
	for _, field := range FieldOrder {
		value, ok := raw[field]
		if !ok {
			return nil, fmt.Errorf("缺少必填字段 task.%s", field)
		}
		if err := assignRecordField(rec, field, value); err != nil {
			return nil, err
		}
	}
	// 未知字段按磁盘顺序收起来，供 WriteRecord 合并保留。
	for _, key := range orderedTopLevelKeys(data) {
		if !isCanonicalField(key) {
			rec.Unknown = append(rec.Unknown, UnknownField{Key: key, Value: raw[key]})
		}
	}
	return rec, nil
}

func assignRecordField(rec *TaskRecord, field string, raw json.RawMessage) error {
	switch {
	case stringFields[field]:
		var s string
		if err := json.Unmarshal(raw, &s); err != nil {
			return fmt.Errorf("task.%s 必须是字符串", field)
		}
		rec.setFieldValue(field, s)
	case nullableStringFields[field]:
		if string(bytes.TrimSpace(raw)) == "null" {
			rec.setFieldValue(field, (*string)(nil))
			return nil
		}
		var s string
		if err := json.Unmarshal(raw, &s); err != nil {
			return fmt.Errorf("task.%s 必须是字符串或 null", field)
		}
		rec.setFieldValue(field, strptr(s))
	case stringArrayFields[field]:
		var arr []string
		if err := json.Unmarshal(raw, &arr); err != nil || arr == nil {
			return fmt.Errorf("task.%s 必须是字符串数组", field)
		}
		rec.setFieldValue(field, arr)
	case field == "meta":
		var obj map[string]any
		if err := json.Unmarshal(raw, &obj); err != nil || obj == nil {
			return fmt.Errorf("task.meta 必须是 JSON 对象")
		}
		if err := checkJSONValues(obj, "task.meta"); err != nil {
			return err
		}
		rec.setFieldValue(field, json.RawMessage(compactJSON(raw)))
	}
	return nil
}

// checkJSONValues 校验 meta 里只有 JSON 值（数字须有限；JSON 文本
// 本身不含 Inf/NaN，这里主要防 map 里混入 Go 侧的非法值）。
func checkJSONValues(obj map[string]any, path string) error {
	for key, v := range obj {
		if err := checkJSONValue(v, path+"."+key); err != nil {
			return err
		}
	}
	return nil
}

func checkJSONValue(v any, path string) error {
	switch val := v.(type) {
	case nil, string, bool:
		return nil
	case float64:
		if val != val || val > 1e308 || val < -1e308 {
			// NaN / Inf 在 JSON 文本里不存在；json.Number 路径下同理。
			return fmt.Errorf("%s 必须是有限 JSON 数字", path)
		}
		return nil
	case json.Number:
		if _, err := val.Float64(); err != nil {
			return fmt.Errorf("%s 必须是有限 JSON 数字", path)
		}
		return nil
	case []any:
		for i, item := range val {
			if err := checkJSONValue(item, fmt.Sprintf("%s[%d]", path, i)); err != nil {
				return err
			}
		}
		return nil
	case map[string]any:
		return checkJSONValues(val, path)
	default:
		return fmt.Errorf("%s 只允许 JSON 值", path)
	}
}

// orderedTopLevelKeys 用 token 流读出顶层 JSON 对象的键顺序。
func orderedTopLevelKeys(data []byte) []string {
	dec := json.NewDecoder(bytes.NewReader(data))
	tok, err := dec.Token()
	if err != nil {
		return nil
	}
	if delim, ok := tok.(json.Delim); !ok || delim != '{' {
		return nil
	}
	var keys []string
	for dec.More() {
		tok, err := dec.Token()
		if err != nil {
			return keys
		}
		key, ok := tok.(string)
		if !ok {
			return keys
		}
		keys = append(keys, key)
		var skip json.RawMessage
		if err := dec.Decode(&skip); err != nil {
			return keys
		}
	}
	return keys
}

// compactJSON 把一段 JSON 压成单行（不转义 HTML，中文原样输出）。
func compactJSON(raw json.RawMessage) []byte {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	var v any
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	if err := dec.Decode(&v); err != nil {
		return bytes.TrimSpace(raw)
	}
	if err := enc.Encode(v); err != nil {
		return bytes.TrimSpace(raw)
	}
	return bytes.TrimSpace(buf.Bytes())
}

// encodeJSONValue 把一个值编码为单行 JSON（不转义 HTML）。
func encodeJSONValue(v any) ([]byte, error) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(v); err != nil {
		return nil, err
	}
	return bytes.TrimSpace(buf.Bytes()), nil
}

// MarshalRecord 按 FieldOrder 编码记录，2 空格缩进，末尾换行；
// Unknown 字段排在 canonical 字段之后、保持原顺序。
func MarshalRecord(rec *TaskRecord) ([]byte, error) {
	var buf bytes.Buffer
	buf.WriteString("{\n")
	type kv struct {
		key string
		val []byte
	}
	var items []kv
	for _, field := range FieldOrder {
		v := rec.fieldValue(field)
		switch field {
		case "subtasks", "children", "relatedFiles":
			if v == nil {
				v = []string{}
			}
			if arr, ok := v.([]string); ok && arr == nil {
				v = []string{}
			}
		case "meta":
			raw, _ := v.(json.RawMessage)
			if len(bytes.TrimSpace(raw)) == 0 {
				raw = json.RawMessage("{}")
			}
			items = append(items, kv{field, compactJSON(raw)})
			continue
		}
		encoded, err := encodeJSONValue(normalizeFieldValue(v))
		if err != nil {
			return nil, fmt.Errorf("编码 task.%s 失败：%w", field, err)
		}
		items = append(items, kv{field, encoded})
	}
	for _, u := range rec.Unknown {
		items = append(items, kv{u.Key, compactJSON(u.Value)})
	}
	for i, item := range items {
		keyJSON, _ := encodeJSONValue(item.key)
		buf.WriteString("  ")
		buf.Write(keyJSON)
		buf.WriteString(": ")
		buf.Write(item.val)
		if i < len(items)-1 {
			buf.WriteString(",")
		}
		buf.WriteString("\n")
	}
	buf.WriteString("}\n")
	return buf.Bytes(), nil
}

// normalizeFieldValue 把 *string 转成 string/nil，供编码用。
func normalizeFieldValue(v any) any {
	if p, ok := v.(*string); ok {
		if p == nil {
			return nil
		}
		return *p
	}
	return v
}

// MetaMap 把 Meta 解码为 map（读不到时返回空 map）。
func (r *TaskRecord) MetaMap() map[string]any {
	if len(r.Meta) == 0 {
		return map[string]any{}
	}
	var m map[string]any
	if err := json.Unmarshal(r.Meta, &m); err != nil || m == nil {
		return map[string]any{}
	}
	return m
}

// SetMetaValue 返回一份 Meta 里指定键被覆盖为字符串 value 的新记录
// （键按字母序写回——Meta 在 Go 侧无序，写盘时以排序后的稳定顺序呈现）。
func (r *TaskRecord) SetMetaValue(key, value string) {
	m := r.MetaMap()
	m[key] = value
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var buf bytes.Buffer
	buf.WriteString("{")
	for i, k := range keys {
		if i > 0 {
			buf.WriteString(", ")
		}
		kb, _ := encodeJSONValue(k)
		vb, _ := encodeJSONValue(m[k])
		buf.Write(kb)
		buf.WriteString(": ")
		buf.Write(vb)
	}
	buf.WriteString("}")
	r.Meta = buf.Bytes()
}

// NullableString 取可空字符串字段的值（nil 返回 ""）。
func (r *TaskRecord) NullableString(field string) string {
	if p, ok := r.fieldValue(field).(*string); ok && p != nil {
		return *p
	}
	return ""
}

// InferPhase 由 status 推导粗粒度阶段，对应 core 的 phase.ts。
func InferPhase(status string) string {
	switch status {
	case "planning":
		return "plan"
	case "in_progress":
		return "implement"
	case "review":
		return "review"
	case "completed", "done":
		return "completed"
	default:
		return "unknown"
	}
}

// ValidateTaskDirName 校验任务目录名：MM-DD-slug 或 00- 系统任务。
// 与 paths.ts 的正则一致，但 slug 允许中文（Sunshine 的任务名常用中文，
// 如 11-09-退款原因必填；Trellis 的 ASCII-only slugify 在中文标题下
// 会产出空 slug，Go 版放宽到 Unicode 字母/数字/连字符）。
func ValidateTaskDirName(name string) bool {
	if name == "" {
		return false
	}
	for _, r := range name {
		if r == '-' || r == '_' {
			continue
		}
		if r >= '0' && r <= '9' || r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' {
			continue
		}
		// 允许中文等 Unicode 字母与数字。
		if r >= 0x4E00 && r <= 0x9FFF || r >= 0x3400 && r <= 0x4DBF {
			continue
		}
		return false
	}
	parts := strings.SplitN(name, "-", 3)
	if len(parts) < 3 {
		return false
	}
	month, day := parts[0], parts[1]
	if len(month) != 2 {
		return false
	}
	m := 0
	for _, c := range month {
		if c < '0' || c > '9' {
			return false
		}
		m = m*10 + int(c-'0')
	}
	if month == "00" {
		// 00- 系统任务（如 00-bootstrap-guidelines）：第二段是 slug 的
		// 一部分，不按日期校验，只要求剩余部分非空。
		return parts[2] != ""
	}
	if len(day) != 2 {
		return false
	}
	d := 0
	for _, c := range day {
		if c < '0' || c > '9' {
			return false
		}
		d = d*10 + int(c-'0')
	}
	if m < 1 || m > 12 || d < 1 || d > 31 {
		return false
	}
	return parts[2] != "" && !strings.HasSuffix(name, "-")
}

// Slugify 把标题转成 slug：ASCII 小写化，非字母数字变连字符、合并、
// 去首尾连字符；中文等 Unicode 字母/数字保留（Trellis 的 _slugify 只认
// ASCII，中文标题会得到空 slug）。
func Slugify(title string) string {
	lower := strings.ToLower(title)
	var b strings.Builder
	lastDash := true // 开头不留连字符
	for _, r := range lower {
		switch {
		case r >= 'a' && r <= 'z' || r >= '0' && r <= '9':
			b.WriteRune(r)
			lastDash = false
		case r >= 0x4E00 && r <= 0x9FFF || r >= 0x3400 && r <= 0x4DBF:
			b.WriteRune(r)
			lastDash = false
		case r == '-' || r == '_':
			if !lastDash {
				b.WriteRune('-')
				lastDash = true
			}
		default:
			// 空格、标点及其他字符都折成一个连字符。
			if !lastDash {
				b.WriteRune('-')
				lastDash = true
			}
		}
	}
	return strings.Trim(b.String(), "-")
}
