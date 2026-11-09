package store

import (
	"fmt"
	"path/filepath"
	"strings"
)

// GlobalProjectKey 是 global 桶的固定 projectKey。
const GlobalProjectKey = "_global"

// ChannelScope 是 channel 的作用域：project（按 cwd 分桶）或 global。
type ChannelScope string

const (
	// ScopeProject 按当前项目分桶。
	ScopeProject ChannelScope = "project"
	// ScopeGlobal 使用固定 _global 桶。
	ScopeGlobal ChannelScope = "global"
)

// ChannelType 是 channel 的结构类型：chat（时间线式）与 forum（主题区）。
// 旧日志里可能出现 "thread"/"threads"，读时不归一化为 forum，
// 一律按 chat 投影（见 ReduceChannelMetadata）。
type ChannelType string

const (
	// ChannelTypeChat 时间线式 channel。
	ChannelTypeChat ChannelType = "chat"
	// ChannelTypeForum 主题区 channel。
	ChannelTypeForum ChannelType = "forum"
)

// ChannelTypes 是全部合法的 channel 类型。
var ChannelTypes = map[ChannelType]bool{
	ChannelTypeChat:  true,
	ChannelTypeForum: true,
}

// ThreadAction 是 kind:"thread" 事件的动作。
type ThreadAction string

const (
	ThreadActionOpened    ThreadAction = "opened"
	ThreadActionComment   ThreadAction = "comment"
	ThreadActionStatus    ThreadAction = "status"
	ThreadActionLabels    ThreadAction = "labels"
	ThreadActionAssignees ThreadAction = "assignees"
	ThreadActionSummary   ThreadAction = "summary"
	ThreadActionProcessed ThreadAction = "processed"
	ThreadActionRename    ThreadAction = "rename"
)

// ThreadActions 是全部合法的 thread 动作。
var ThreadActions = map[ThreadAction]bool{
	ThreadActionOpened:    true,
	ThreadActionComment:   true,
	ThreadActionStatus:    true,
	ThreadActionLabels:    true,
	ThreadActionAssignees: true,
	ThreadActionSummary:   true,
	ThreadActionProcessed: true,
	ThreadActionRename:    true,
}

// ContextTarget 是 context 事件的目标层级。
type ContextTarget string

const (
	// ContextTargetChannel channel 级上下文。
	ContextTargetChannel ContextTarget = "channel"
	// ContextTargetThread thread 级上下文。
	ContextTargetThread ContextTarget = "thread"
)

// ContextMutationAction 是 context 事件的变更动作。
type ContextMutationAction string

const (
	// ContextMutationAdd 新增上下文条目。
	ContextMutationAdd ContextMutationAction = "add"
	// ContextMutationDelete 删除上下文条目。
	ContextMutationDelete ContextMutationAction = "delete"
)

// EventOrigin 是事件的来源。
type EventOrigin string

const (
	// OriginCLI 来自命令行。
	OriginCLI EventOrigin = "cli"
	// OriginAPI 来自 API 调用。
	OriginAPI EventOrigin = "api"
	// OriginWorker 来自 worker 进程。
	OriginWorker EventOrigin = "worker"
)

// EventOrigins 是全部合法的事件来源。
var EventOrigins = map[EventOrigin]bool{
	OriginCLI:    true,
	OriginAPI:    true,
	OriginWorker: true,
}

// InboxPolicy 是 worker 的收件箱策略：explicitOnly（默认，仅消费
// to 指向自己的消息）与 broadcastAndExplicit（另消费无 to 的广播）。
// 只对 kind:"message" 事件生效（见 MatchesInboxPolicy）。
type InboxPolicy string

const (
	// InboxPolicyExplicitOnly 仅消费 to 指向自己的消息。
	InboxPolicyExplicitOnly InboxPolicy = "explicitOnly"
	// InboxPolicyBroadcastAndExplicit 另消费无 to 的广播消息。
	InboxPolicyBroadcastAndExplicit InboxPolicy = "broadcastAndExplicit"
)

// DefaultInboxPolicy 是未指定时的默认 inbox 策略。
const DefaultInboxPolicy = InboxPolicyExplicitOnly

// InboxPolicies 是全部合法的 inbox 策略。
var InboxPolicies = map[InboxPolicy]bool{
	InboxPolicyExplicitOnly:         true,
	InboxPolicyBroadcastAndExplicit: true,
}

// ChannelRef 定位一个 channel：名字、作用域、项目桶与磁盘目录。
type ChannelRef struct {
	Name    string
	Scope   ChannelScope
	Project string
	Dir     string
}

// ChannelMetadata 是从事件流投影出的 channel 元数据。
type ChannelMetadata struct {
	Type        ChannelType
	Title       string
	Description string
	Context     []ContextEntry
	Labels      []string
}

// FileContextEntry 是文件型上下文条目：path 须为绝对路径。
type FileContextEntry struct {
	Type string `json:"type"`
	Path string `json:"path"`
}

// RawContextEntry 是原始文本型上下文条目。
type RawContextEntry struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

// ContextEntry 是上下文条目：file（绝对路径）或 raw（文本）。
type ContextEntry struct {
	Type string `json:"type"`
	Path string `json:"path,omitempty"`
	Text string `json:"text,omitempty"`
}

// ParseChannelScope 校验 scope 字符串；空串返回错误。
func ParseChannelScope(v string) (ChannelScope, error) {
	switch ChannelScope(v) {
	case ScopeProject, ScopeGlobal:
		return ChannelScope(v), nil
	default:
		return "", fmt.Errorf("无效的 --scope %q，必须是 project 或 global", v)
	}
}

// ParseChannelType 校验 channel 类型；"thread"/"threads" 是旧值，
// 新建时拒绝（须用 forum），读旧日志时不归一化（见 ReduceChannelMetadata）。
func ParseChannelType(v string) (ChannelType, error) {
	if v == "" {
		return ChannelTypeChat, nil
	}
	if v == "thread" || v == "threads" {
		return "", fmt.Errorf("无效的 --type %q，请用 --type forum", v)
	}
	if !ChannelTypes[ChannelType(v)] {
		return "", fmt.Errorf("无效的 --type %q，必须是 chat 或 forum", v)
	}
	return ChannelType(v), nil
}

// ParseThreadAction 校验 thread 动作。
func ParseThreadAction(v string) (ThreadAction, error) {
	if !ThreadActions[ThreadAction(v)] {
		return "", fmt.Errorf("无效的 thread 动作 %q", v)
	}
	return ThreadAction(v), nil
}

// ParseEventOrigin 校验事件来源。
func ParseEventOrigin(v string) (EventOrigin, error) {
	if !EventOrigins[EventOrigin(v)] {
		return "", fmt.Errorf("无效的 origin %q，必须是 cli、api 或 worker", v)
	}
	return EventOrigin(v), nil
}

// ParseInboxPolicy 校验 inbox 策略。
func ParseInboxPolicy(v string) (InboxPolicy, error) {
	if !InboxPolicies[InboxPolicy(v)] {
		return "", fmt.Errorf("无效的 inbox 策略 %q，必须是 explicitOnly 或 broadcastAndExplicit", v)
	}
	return InboxPolicy(v), nil
}

// NormalizeThreadKey 校验并规整 thread 键：非空，只含字母、数字、.、_、-。
func NormalizeThreadKey(v string) (string, error) {
	trimmed := strings.TrimSpace(v)
	if trimmed == "" {
		return "", fmt.Errorf("thread 键不能为空")
	}
	for _, r := range trimmed {
		if !(r >= 'A' && r <= 'Z' || r >= 'a' && r <= 'z' || r >= '0' && r <= '9' || r == '.' || r == '_' || r == '-') {
			return "", fmt.Errorf("thread 键只允许字母、数字、.、_ 和 -")
		}
	}
	return trimmed, nil
}

// BuildContextEntries 由绝对路径文件列表与原始文本列表构造上下文条目；
// 两者都为空时返回 nil。文件路径必须为绝对路径，原始文本不能为空。
func BuildContextEntries(files []string, raw []string) ([]ContextEntry, error) {
	var entries []ContextEntry
	for _, file := range files {
		value := strings.TrimSpace(file)
		if !filepath.IsAbs(value) {
			return nil, fmt.Errorf("上下文文件必须是绝对路径：%s", file)
		}
		entries = append(entries, ContextEntry{Type: "file", Path: value})
	}
	for _, text := range raw {
		if strings.TrimSpace(text) == "" {
			return nil, fmt.Errorf("上下文原始文本不能为空")
		}
		entries = append(entries, ContextEntry{Type: "raw", Text: text})
	}
	if len(entries) == 0 {
		return nil, nil
	}
	return entries, nil
}

// ContextEntryKey 是上下文条目的身份键：文件按绝对路径，原始文本按全文。
func ContextEntryKey(entry ContextEntry) string {
	if entry.Type == "file" {
		return "file:" + entry.Path
	}
	return "raw:" + entry.Text
}

// AsStringArray 从任意值里提取字符串数组；非数组返回 nil。
func AsStringArray(value any) []string {
	arr, ok := value.([]any)
	if !ok {
		return nil
	}
	out := make([]string, 0, len(arr))
	for _, item := range arr {
		if s, ok := item.(string); ok {
			out = append(out, s)
		}
	}
	return out
}

// AsContextEntries 从任意值里提取合法的上下文条目；无合法条目返回 nil。
func AsContextEntries(value any) []ContextEntry {
	arr, ok := value.([]any)
	if !ok {
		return nil
	}
	var out []ContextEntry
	for _, item := range arr {
		m, ok := item.(map[string]any)
		if !ok {
			continue
		}
		typ, _ := m["type"].(string)
		switch typ {
		case "file":
			if path, ok := m["path"].(string); ok {
				out = append(out, ContextEntry{Type: "file", Path: path})
			}
		case "raw":
			if text, ok := m["text"].(string); ok {
				out = append(out, ContextEntry{Type: "raw", Text: text})
			}
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}
