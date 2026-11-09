// Package mem 提供跨平台会话检索的统一模型与汇总逻辑。
//
// 只读解析各平台会话库（Claude / Codex 的 JSONL / SQLite 等），统一为
// session / turn 模型；关键词匹配，无向量检索；不读 channel 事件。
package mem

import (
	"context"
	"time"
)

// MemSourceKind 是会话来源平台。
type MemSourceKind string

// 7 个会话来源平台（与上游 MEM_SOURCE_KINDS 一致）。
const (
	MemSourceClaude   MemSourceKind = "claude"
	MemSourceCodex    MemSourceKind = "codex"
	MemSourceDevin    MemSourceKind = "devin"
	MemSourceGrok     MemSourceKind = "grok"
	MemSourceOpencode MemSourceKind = "opencode"
	MemSourcePi       MemSourceKind = "pi"
	MemSourceZcode    MemSourceKind = "zcode"
)

// MemSourceKinds 是全部 7 个平台，按固定顺序排列
// （projects 输出的 by_platform 计数按此顺序呈现）。
var MemSourceKinds = []MemSourceKind{
	MemSourceClaude,
	MemSourceCodex,
	MemSourceDevin,
	MemSourceGrok,
	MemSourceOpencode,
	MemSourcePi,
	MemSourceZcode,
}

// IsValidMemSourceKind 判断平台名是否合法。
func IsValidMemSourceKind(kind string) bool {
	for _, k := range MemSourceKinds {
		if string(k) == kind {
			return true
		}
	}
	return false
}

// MemSourceFilter 是平台过滤值：单个平台或全部。
type MemSourceFilter string

// SourceAll 表示不过滤平台。
const SourceAll MemSourceFilter = "all"

// IsAll 判断是否不过滤平台。
func (f MemSourceFilter) IsAll() bool {
	return f == "" || f == SourceAll
}

// MemPhase 是工作阶段过滤值。
type MemPhase string

// brainstorm / implement / all 三种阶段。
const (
	PhaseBrainstorm MemPhase = "brainstorm"
	PhaseImplement  MemPhase = "implement"
	PhaseAll        MemPhase = "all"
)

// 对话轮次的角色取值（Role 字段为 string，用这两个常量比较）。
const (
	RoleUser      = "user"
	RoleAssistant = "assistant"
)

// DialogueTurn 是一轮清理后的对话。
type DialogueTurn struct {
	// Role 为 user 或 assistant。
	Role string `json:"role"`
	Text string `json:"text"`
	// Kind 为 "marker" 时标注压缩边界：它留在轮次池里，供 extract /
	// context 展示模型上下文被截断的位置；search 计分时跳过，
	// 避免与平台摘要重复计分。
	Kind string `json:"kind,omitempty"`
}

// IsMarker 判断该轮次是否为压缩边界标注。
func (t DialogueTurn) IsMarker() bool {
	return t.Kind == "marker"
}

// SearchExcerpt 是一条段落对齐的命中摘录。
type SearchExcerpt struct {
	Role    string `json:"role"`
	Snippet string `json:"snippet"`
}

// SearchHit 是单个会话的检索命中：命中次数与段落对齐的摘录。
type SearchHit struct {
	// Count 是全部命中 token 数。
	Count int `json:"count"`
	// UserCount 是用户轮次里的命中数。
	UserCount int `json:"userCount"`
	// AsstCount 是助手轮次里的命中数。
	AsstCount int `json:"asstCount"`
	// TotalTurns 是已清理对话规模（相关度分母，不含 marker 轮次）。
	TotalTurns int `json:"totalTurns"`
	// Excerpts 是段落对齐的摘录。
	Excerpts []SearchExcerpt `json:"excerpts"`
}

// MemFilter 是跨平台的会话筛选条件。Since / Until 用 time.Time，
// HasSince / HasUntil 标记是否真正传了该边界（零值 time.Time 不代表未传）。
type MemFilter struct {
	Platform MemSourceFilter
	Since    time.Time
	HasSince bool
	Until    time.Time
	HasUntil bool
	// Cwd 为空表示全局检索；非空时只取该目录及其子目录下的会话。
	Cwd    string
	Global bool
	Limit  int
	// Grep 是关键词过滤（多 token 为 AND）；search 时放检索关键词。
	Grep string
	// Phase 只对 extract 生效。
	Phase MemPhase
	// Events 是 extract 阶段切片用的事件通道：适配器 Collect 从
	// 工具调用里抽出的 `sunshine task create|start` 事件
	//（TurnIndex 指向清理后对话里的下标）。为空表示无任务边界，
	// 与 SliceMemPhase 的无边界语义一致。轮次文本里的命令只是
	// 普通文本，绝不作为事件来源。
	Events []MemEvent
	// PlatformKind 是 extract 阶段切片用的平台：只有 OpenCode
	// 不支持阶段切片（降级返回全文）。为空时按支持切片的平台处理。
	PlatformKind MemSourceKind
	// Turns / Around / MaxChars 只对 context 生效。
	Turns    int
	Around   int
	MaxChars int
	// IncludeChildren 只对 search / context 生效：沿 OpenCode 的
	// parent_id 链把子会话并入父会话。
	IncludeChildren bool
	// JSON 表示输出机器可读的 JSON。
	JSON bool
	// SessionID 是 context 的目标会话 id：include-children 时用它在
	// Sessions 表里定位父会话，再沿 parent_id 链找子会话。
	SessionID string
	// Sessions 是子会话查找用的会话表（调用方传入；include-children
	// 时据此建 parent_id 索引）。
	Sessions []MemSessionInfo
	// LoadTurns 加载一个子会话的对话轮次；为 nil 时不合并子会话
	//（search 的子会话加载走 SearchSessions 的 load 参数，不走这里）。
	LoadTurns func(MemSessionInfo) ([]DialogueTurn, error)
}

// MemSessionInfo 是跨平台统一的会话元数据。
// JSON 字段名（platform、parent_id、filePath）与上游保持一致。
type MemSessionInfo struct {
	Platform MemSourceKind `json:"platform"`
	ID       string        `json:"id"`
	Title    string        `json:"title,omitempty"`
	CWD      string        `json:"cwd,omitempty"`
	Created  string        `json:"created,omitempty"`
	Updated  string        `json:"updated,omitempty"`
	FilePath string        `json:"filePath"`
	// ParentID 仅 OpenCode：子代理链的父会话 id。
	ParentID string `json:"parent_id,omitempty"`
}

// MemProjectSummary 是单个项目的会话汇总。
type MemProjectSummary struct {
	Cwd        string                `json:"cwd"`
	LastActive string                `json:"last_active"`
	Sessions   int                   `json:"sessions"`
	ByPlatform map[MemSourceKind]int `json:"by_platform"`
}

// Adapter 是单个平台的会话适配器（B 组按此接口实现，C 组按此消费）。
// 只读：绝不写会话存储。
type Adapter interface {
	Kind() MemSourceKind
	// Sessions 列出该平台的全部会话。存储缺失是正常空结果：返回空列表与
	// 一条警告，不返回错误。第二个返回值是警告列表，不是错误。
	Sessions(ctx context.Context, home string) ([]MemSessionInfo, []string, error)
	// Dialogue 读出一次会话清理后的对话轮次。警告含义同 Sessions。
	Dialogue(ctx context.Context, s MemSessionInfo) ([]DialogueTurn, []string, error)
	// Collect 一次遍历同时产出清理后的对话轮次与任务命令事件
	// （工具调用里的 `sunshine task create|start`，见 MemEvent）：
	// 返回的 turns 与 Dialogue 完全一致；events 的 TurnIndex 是该调用
	// 在已清理轮次里的下标；不支持事件抽取的平台返回空事件列表。
	// 警告含义同 Sessions。
	Collect(ctx context.Context, s MemSessionInfo) ([]DialogueTurn, []MemEvent, []string, error)
}

// ContextTurn 是 context 选中的一轮：它在清理后对话里的下标、角色、
// 文本，以及是否为 grep 命中轮次。
type ContextTurn struct {
	Idx   int    `json:"idx"`
	Role  string `json:"role"`
	Text  string `json:"text"`
	IsHit bool   `json:"is_hit"`
}

// BrainstormWindow 是一段头脑风暴窗口：[StartTurn, EndTurn)，
// 左闭右开；Label 是配对的任务 slug，无 slug 时为 window-N。
type BrainstormWindow struct {
	Label     string `json:"label"`
	StartTurn int    `json:"startTurn"`
	EndTurn   int    `json:"endTurn"`
}

// ParsedTaskCommand 是从一条 shell 调用字符串里解析出的一次任务命令
// （Go 版命令名：sunshine task create | sunshine task start）。
type ParsedTaskCommand struct {
	// Action 为 "create" 或 "start"。
	Action string
	// Slug 是 create 的 --slug（或 -s）值。
	Slug string
	// TitleArg 是 create 的第一个非 flag 参数（任务标题）。
	TitleArg string
	// TaskDir 是 start 的第一个非 flag 参数（任务目录）。
	TaskDir string
}

// MemEvent 是对话里一次任务命令事件：从工具调用里抽出的
// `sunshine task create|start`（对齐上游 TaskPyEvent）。
// TurnIndex 是该调用发生时在已清理 DialogueTurn[] 里的下标；
// Timestamp 是该调用所在事件的时间戳（平台不提供时为空）。
// Slug 只在 create 事件上有值，TaskDir 只在 start 事件上有值。
type MemEvent struct {
	// Action 为 "create" 或 "start"。
	Action string `json:"action"`
	// Timestamp 是调用时间（ISO 字符串；无时间戳的平台为空）。
	Timestamp string `json:"timestamp"`
	// TurnIndex 是该命令所在轮次在清理后对话里的下标。
	TurnIndex int `json:"turnIndex"`
	// Slug 是 create 的 --slug 值（start 事件为空）。
	Slug string `json:"slug,omitempty"`
	// TaskDir 是 start 的任务目录参数（create 事件为空）。
	TaskDir string `json:"taskDir,omitempty"`
}

// SearchMatch 是一次带会话身份的检索匹配：会话、相关度分、命中详情，
// 以及 include-children 时并入该会话的子会话个数。
type SearchMatch struct {
	Session           MemSessionInfo
	Score             float64
	Hit               SearchHit
	DescendantsMerged int
}
