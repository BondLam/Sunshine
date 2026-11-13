package channel

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/linrong007/Sunshine/internal/channel/store"
)

// 本文件是 API 层的共享类型：forum 与 core API 共用。
//
// 除 ChannelEvent 外的类型一律是 store 子包同名类型的别名
// （零转换成本；store 是唯一的事实来源）：ChannelRef、
// ChannelMetadata、ContextEntry、WorkerState、WorkerRegistry、
// DeliveryMode、InboxPolicy 等。
//
// ChannelEvent 是 API 层的富事件结构：基础字段 + 各 kind 的
// 具名字段，未知字段保留在 Extra 里；与 store.ChannelEvent 之间
// 用 JSON 往返转换（见 forum_store.go 的 fromStoreEvent），
// 专有字段在两边都不丢失。

// 类型别名：与 store 子包同一类型。
type (
	// ChannelRef 定位一个 channel：名字、作用域、项目桶与磁盘目录。
	ChannelRef = store.ChannelRef
	// ChannelScope 是 channel 的作用域：project 或 global。
	ChannelScope = store.ChannelScope
	// ChannelType 是 channel 的结构类型：chat 或 forum。
	ChannelType = store.ChannelType
	// ChannelMetadata 是从事件流投影出的 channel 元数据。
	ChannelMetadata = store.ChannelMetadata
	// ContextEntry 是一条上下文条目：file（绝对路径）或 raw（文本）。
	ContextEntry = store.ContextEntry
	// EventOrigin 是事件来源：cli、api 或 worker。
	EventOrigin = store.EventOrigin
	// InboxPolicy 是 worker 的收件箱策略。
	InboxPolicy = store.InboxPolicy
	// DeliveryMode 是定向消息的投递校验模式。
	DeliveryMode = store.DeliveryMode
	// WorkerLifecycle 是 worker 的进程生命周期。
	WorkerLifecycle = store.WorkerLifecycle
	// WorkerActivity 是 worker 的回合活动：idle 或 mid-turn。
	WorkerActivity = store.WorkerActivity
	// WorkerState 是从事件日志投影出的单个 worker 状态。
	WorkerState = store.WorkerState
	// WorkerRegistry 是 channel 的 worker 注册表投影。
	WorkerRegistry = store.WorkerRegistry
	// InterruptReason 是中断原因。
	InterruptReason = store.InterruptReason
	// InterruptMethod 是中断的执行方式。
	InterruptMethod = store.InterruptMethod
	// InterruptOutcome 是中断尝试的结果。
	InterruptOutcome = store.InterruptOutcome
	// UndeliverableReason 是消息无法投递的原因。
	UndeliverableReason = store.UndeliverableReason
	// ThreadAction 是 forum thread 的动作。
	ThreadAction = store.ThreadAction
	// ContextTarget 是上下文条目的归属层级。
	ContextTarget = store.ContextTarget
	// ContextMutationAction 是上下文变更动作。
	ContextMutationAction = store.ContextMutationAction
	// ChannelCursor 是跨 channel 监听的每 channel 续跑游标。
	ChannelCursor = store.ChannelCursor
	// Pagination 是读事件的游标分页参数；AfterSeq 与 BeforeSeq 不可同传。
	Pagination = store.ReadChannelEventsPagination
	// EventKind 是 channel 事件的种类，共 21 种。
	EventKind = store.ChannelEventKind
	// UndeliverableTarget 是投递分类里一个失败的目标。
	UndeliverableTarget = store.UndeliverableTarget
)

// 作用域与类型常量（与 store 子包同值）。
const (
	// ScopeProject 是项目作用域。
	ScopeProject = store.ScopeProject
	// ScopeGlobal 是全局作用域。
	ScopeGlobal = store.ScopeGlobal
	// GlobalProjectKey 是全局桶的固定 project key。
	GlobalProjectKey = store.GlobalProjectKey
	// ChannelTypeChat 是时间线式 channel。
	ChannelTypeChat = store.ChannelTypeChat
	// ChannelTypeForum 是主题区 channel。
	ChannelTypeForum = store.ChannelTypeForum
	// OriginCLI 表示事件来自命令行。
	OriginCLI = store.OriginCLI
	// OriginAPI 表示事件来自 API 调用。
	OriginAPI = store.OriginAPI
	// OriginWorker 表示事件来自 worker。
	OriginWorker = store.OriginWorker
	// InboxPolicyExplicitOnly 是默认策略：只消费 to 指向自己的消息。
	InboxPolicyExplicitOnly = store.InboxPolicyExplicitOnly
	// InboxPolicyBroadcastAndExplicit 除定向消息外还消费无 to 的广播消息。
	InboxPolicyBroadcastAndExplicit = store.InboxPolicyBroadcastAndExplicit
	// DefaultInboxPolicy 是默认收件箱策略。
	DefaultInboxPolicy = store.DefaultInboxPolicy
	// DeliveryModeAppendOnly 是默认模式：只追加，永不报错。
	DeliveryModeAppendOnly = store.DeliveryModeAppendOnly
	// DeliveryModeRequireKnownWorker 对从未出现过的目标追加 undeliverable 事件。
	DeliveryModeRequireKnownWorker = store.DeliveryModeRequireKnownWorker
	// DeliveryModeRequireRunningWorker 对未知或终态目标追加 undeliverable 事件。
	DeliveryModeRequireRunningWorker = store.DeliveryModeRequireRunningWorker
	// WorkerLifecycleStarting 表示 worker 正在启动。
	WorkerLifecycleStarting = store.WorkerLifecycleStarting
	// WorkerLifecycleRunning 表示 worker 运行中。
	WorkerLifecycleRunning = store.WorkerLifecycleRunning
	// WorkerLifecycleDone 表示 worker 正常完成。
	WorkerLifecycleDone = store.WorkerLifecycleDone
	// WorkerLifecycleError 表示 worker 出错结束。
	WorkerLifecycleError = store.WorkerLifecycleError
	// WorkerLifecycleKilled 表示 worker 被杀掉。
	WorkerLifecycleKilled = store.WorkerLifecycleKilled
	// WorkerLifecycleCrashed 表示 worker 崩溃。
	WorkerLifecycleCrashed = store.WorkerLifecycleCrashed
	// WorkerActivityIdle 表示 worker 空闲。
	WorkerActivityIdle = store.WorkerActivityIdle
	// WorkerActivityMidTurn 表示 worker 正在执行一个回合。
	WorkerActivityMidTurn = store.WorkerActivityMidTurn
	// InterruptReasonUser 表示用户发起的中断。
	InterruptReasonUser = store.InterruptReasonUser
	// InterruptReasonSystem 表示系统发起的中断。
	InterruptReasonSystem = store.InterruptReasonSystem
	// InterruptReasonTimeout 表示超时引起的中断。
	InterruptReasonTimeout = store.InterruptReasonTimeout
	// InterruptReasonSuperseded 表示被更新指令取代的中断。
	InterruptReasonSuperseded = store.InterruptReasonSuperseded
	// InterruptMethodProvider 表示由 provider 中断。
	InterruptMethodProvider = store.InterruptMethodProvider
	// InterruptMethodStdin 表示经 stdin 中断。
	InterruptMethodStdin = store.InterruptMethodStdin
	// InterruptMethodSignal 表示经信号中断。
	InterruptMethodSignal = store.InterruptMethodSignal
	// InterruptMethodNone 表示未尝试中断（无可用 runtime）。
	InterruptMethodNone = store.InterruptMethodNone
	// InterruptOutcomeInterrupted 表示已中断当前回合。
	InterruptOutcomeInterrupted = store.InterruptOutcomeInterrupted
	// InterruptOutcomeQueued 表示中断已排队。
	InterruptOutcomeQueued = store.InterruptOutcomeQueued
	// InterruptOutcomeUnsupported 表示 runtime 不支持中断。
	InterruptOutcomeUnsupported = store.InterruptOutcomeUnsupported
	// InterruptOutcomeNoActiveTurn 表示当时无进行中的回合。
	InterruptOutcomeNoActiveTurn = store.InterruptOutcomeNoActiveTurn
	// InterruptOutcomeFailed 表示中断尝试失败。
	InterruptOutcomeFailed = store.InterruptOutcomeFailed
	// UndeliverableWorkerUnknown 表示目标 worker 从未出现。
	UndeliverableWorkerUnknown = store.UndeliverableWorkerUnknown
	// UndeliverableWorkerTerminal 表示目标 worker 已终态。
	UndeliverableWorkerTerminal = store.UndeliverableWorkerTerminal
	// ThreadActionOpened 表示开贴。
	ThreadActionOpened = store.ThreadActionOpened
	// ThreadActionComment 表示评论。
	ThreadActionComment = store.ThreadActionComment
	// ThreadActionStatus 表示改状态。
	ThreadActionStatus = store.ThreadActionStatus
	// ThreadActionLabels 表示改标签。
	ThreadActionLabels = store.ThreadActionLabels
	// ThreadActionAssignees 表示改负责人。
	ThreadActionAssignees = store.ThreadActionAssignees
	// ThreadActionSummary 表示写摘要。
	ThreadActionSummary = store.ThreadActionSummary
	// ThreadActionProcessed 表示标记已处理。
	ThreadActionProcessed = store.ThreadActionProcessed
	// ThreadActionRename 表示重命名。
	ThreadActionRename = store.ThreadActionRename
	// ContextTargetChannel 表示 channel 级上下文。
	ContextTargetChannel = store.ContextTargetChannel
	// ContextTargetThread 表示 thread 级上下文。
	ContextTargetThread = store.ContextTargetThread
	// ContextMutationAdd 表示新增上下文条目。
	ContextMutationAdd = store.ContextMutationAdd
	// ContextMutationDelete 表示删除上下文条目。
	ContextMutationDelete = store.ContextMutationDelete
	// DefaultCursorPageSize 是带游标时默认的页大小。
	DefaultCursorPageSize = store.DefaultPageSize
)

// 21 种事件 kind（与 store 子包同值）。
const (
	// EventKindCreate 是建 channel 事件。
	EventKindCreate = store.KindCreate
	// EventKindJoin 是加入事件。
	EventKindJoin = store.KindJoin
	// EventKindLeave 是离开事件。
	EventKindLeave = store.KindLeave
	// EventKindMessage 是消息事件。
	EventKindMessage = store.KindMessage
	// EventKindThread 是 thread 事件。
	EventKindThread = store.KindThread
	// EventKindContext 是上下文变更事件。
	EventKindContext = store.KindContext
	// EventKindChannel 是 channel 元数据事件。
	EventKindChannel = store.KindChannel
	// EventKindSpawned 是 worker 注册事件。
	EventKindSpawned = store.KindSpawned
	// EventKindKilled 是 worker 被杀事件。
	EventKindKilled = store.KindKilled
	// EventKindRespawned 是 worker 重生事件。
	EventKindRespawned = store.KindRespawned
	// EventKindProgress 是进度事件。
	EventKindProgress = store.KindProgress
	// EventKindDone 是完成事件。
	EventKindDone = store.KindDone
	// EventKindError 是出错事件。
	EventKindError = store.KindError
	// EventKindWaiting 是等待事件。
	EventKindWaiting = store.KindWaiting
	// EventKindAwake 是唤醒事件。
	EventKindAwake = store.KindAwake
	// EventKindUndeliverable 是无法投递事件。
	EventKindUndeliverable = store.KindUndeliverable
	// EventKindInterruptRequested 是中断请求事件。
	EventKindInterruptRequested = store.KindInterruptRequested
	// EventKindTurnStarted 是回合开始事件。
	EventKindTurnStarted = store.KindTurnStarted
	// EventKindTurnFinished 是回合结束事件。
	EventKindTurnFinished = store.KindTurnFinished
	// EventKindInterrupted 是已中断事件。
	EventKindInterrupted = store.KindInterrupted
	// EventKindSupervisorWarning 是 supervisor 预警事件。
	EventKindSupervisorWarning = store.KindSupervisorWarning
)

// EventKinds 是全部 21 种事件 kind。
var EventKinds = []EventKind{
	EventKindCreate, EventKindJoin, EventKindLeave, EventKindMessage,
	EventKindThread, EventKindContext, EventKindChannel, EventKindSpawned,
	EventKindKilled, EventKindRespawned, EventKindProgress, EventKindDone,
	EventKindError, EventKindWaiting, EventKindAwake, EventKindUndeliverable,
	EventKindInterruptRequested, EventKindTurnStarted, EventKindTurnFinished,
	EventKindInterrupted, EventKindSupervisorWarning,
}

// IsValidEventKind 判断 kind 是否合法。
func IsValidEventKind(kind string) bool {
	return store.ChannelEventKinds[store.ChannelEventKind(kind)]
}

// IsTerminalLifecycle 判断生命周期是否为终态。
func IsTerminalLifecycle(lifecycle WorkerLifecycle) bool {
	return store.IsTerminalLifecycle(lifecycle)
}

// CrossChannelEvent 是跨 channel 监听吐出的一条事件，附带全量游标快照。
type CrossChannelEvent struct {
	Channel ChannelRef
	Event   ChannelEvent
	Cursor  ChannelCursor
}

// ChannelCursorKey 是游标的键：scope/project/name。
func ChannelCursorKey(ref ChannelRef) string {
	return store.ChannelCursorKey(ref)
}

// ContextEntryKey 是上下文条目的去重键：file 按绝对路径，raw 按全文。
func ContextEntryKey(entry ContextEntry) string {
	return store.ContextEntryKey(entry)
}

// BuildContextEntries 由绝对路径文件列表与 raw 文本列表拼出上下文条目；
// 两者都为空时返回 nil。
func BuildContextEntries(files []string, raws []string) ([]ContextEntry, error) {
	return store.BuildContextEntries(files, raws)
}

// ValidateContextEntry 校验单条上下文条目。
func ValidateContextEntry(entry ContextEntry) error {
	switch entry.Type {
	case "file":
		if entry.Path == "" {
			return fmt.Errorf("file 类型上下文条目缺少 path")
		}
		if !strings.HasPrefix(entry.Path, "/") {
			return fmt.Errorf("上下文文件须为绝对路径：%s", entry.Path)
		}
	case "raw":
		if strings.TrimSpace(entry.Text) == "" {
			return fmt.Errorf("raw 类型上下文条目文本不能为空")
		}
	default:
		return fmt.Errorf("上下文条目类型 %q 无效，可选值：file、raw", entry.Type)
	}
	return nil
}

// ValidateContextEntries 校验一批上下文条目（至少一条）。
func ValidateContextEntries(entries []ContextEntry) error {
	if len(entries) == 0 {
		return fmt.Errorf("context 至少须包含一条条目")
	}
	for _, entry := range entries {
		if err := ValidateContextEntry(entry); err != nil {
			return err
		}
	}
	return nil
}

// ChannelEvent 是一条 channel 事件：基础字段 + 各 kind 的具名字段。
// 未知字段保留在 Extra 里往返，不丢失；Extra 同时镜像全部已知字段，
// 供只读 Extra 的投影（如 threadstate 的 contextTarget）使用。
type ChannelEvent struct {
	Seq            int64          `json:"seq"`
	TS             string         `json:"ts"`
	Kind           EventKind      `json:"kind"`
	By             string         `json:"by"`
	IdempotencyKey string         `json:"idempotencyKey,omitempty"`
	To             []string       `json:"-"`
	Origin         EventOrigin    `json:"origin,omitempty"`
	Meta           map[string]any `json:"meta,omitempty"`

	// create 事件字段。
	CWD         string   `json:"cwd,omitempty"`
	Task        string   `json:"task,omitempty"`
	Type        string   `json:"type,omitempty"`
	Description string   `json:"description,omitempty"`
	Labels      []string `json:"labels,omitempty"`
	Ephemeral   bool     `json:"ephemeral,omitempty"`

	// message / thread 事件字段。
	Text          string         `json:"text,omitempty"`
	Thread        string         `json:"thread,omitempty"`
	NewThread     string         `json:"newThread,omitempty"`
	Action        string         `json:"action,omitempty"`
	Title         *string        `json:"title,omitempty"`
	TitleNull     bool           `json:"-"`
	Status        string         `json:"status,omitempty"`
	Assignees     []string       `json:"assignees,omitempty"`
	Summary       string         `json:"summary,omitempty"`
	Context       []ContextEntry `json:"context,omitempty"`
	LinkedContext []ContextEntry `json:"linkedContext,omitempty"`
	Target        string         `json:"target,omitempty"`

	// spawned / killed / done / error / turn / interrupt 事件字段。
	As            string      `json:"as,omitempty"`
	Worker        string      `json:"worker,omitempty"`
	Provider      string      `json:"provider,omitempty"`
	PID           int         `json:"pid,omitempty"`
	Agent         string      `json:"agent,omitempty"`
	InboxPolicy   InboxPolicy `json:"inboxPolicy,omitempty"`
	Reason        string      `json:"reason,omitempty"`
	Signal        string      `json:"signal,omitempty"`
	Message       string      `json:"message,omitempty"`
	Method        string      `json:"method,omitempty"`
	Outcome       string      `json:"outcome,omitempty"`
	TurnID        string      `json:"turnId,omitempty"`
	InputSeq      int64       `json:"inputSeq,omitempty"`
	TargetWorker  string      `json:"targetWorker,omitempty"`
	MessageSeq    int64       `json:"messageSeq,omitempty"`
	Synthesized   bool        `json:"synthesized,omitempty"`
	ExitCode      *int        `json:"exit_code,omitempty"`
	ExitSignal    string      `json:"exit_signal,omitempty"`
	DurationMS    int64       `json:"duration_ms,omitempty"`
	TimeoutMS     int64       `json:"timeout_ms,omitempty"`
	IdleTimeoutMS int64       `json:"idle_timeout_ms,omitempty"`
	RemainingMS   int64       `json:"remaining_ms,omitempty"`
	Files         []string    `json:"files,omitempty"`
	Manifests     []string    `json:"manifests,omitempty"`

	// 全部字段（含已知字段）的镜像与未知字段。
	Extra map[string]any `json:"-"`
}

// knownEventKeys 是 ChannelEvent 的已知 JSON 字段名集合。
var knownEventKeys = map[string]bool{
	"seq": true, "ts": true, "kind": true, "by": true,
	"idempotencyKey": true, "to": true, "origin": true, "meta": true,
	"cwd": true, "task": true, "type": true, "description": true,
	"labels": true, "ephemeral": true, "text": true, "thread": true,
	"newThread": true, "action": true, "title": true, "status": true,
	"assignees": true, "summary": true, "context": true,
	"linkedContext": true, "target": true, "as": true, "worker": true,
	"provider": true, "pid": true, "agent": true, "inboxPolicy": true,
	"reason": true, "signal": true, "message": true, "method": true,
	"outcome": true, "turnId": true, "inputSeq": true, "targetWorker": true,
	"messageSeq": true, "synthesized": true, "exit_code": true,
	"exit_signal": true, "duration_ms": true, "timeout_ms": true,
	"idle_timeout_ms": true, "remaining_ms": true, "files": true,
	"manifests": true,
}

// UnmarshalJSON 解析事件：to 接受字符串或数组；title 区分 null 与缺失；
// 全部字段同时镜像进 Extra。
func (e *ChannelEvent) UnmarshalJSON(data []byte) error {
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	decode := func(key string, dst any) {
		if v, ok := raw[key]; ok {
			_ = json.Unmarshal(v, dst)
		}
	}
	decode("seq", &e.Seq)
	decode("ts", &e.TS)
	decode("kind", &e.Kind)
	decode("by", &e.By)
	decode("idempotencyKey", &e.IdempotencyKey)
	decode("origin", &e.Origin)
	decode("meta", &e.Meta)
	decode("cwd", &e.CWD)
	decode("task", &e.Task)
	decode("type", &e.Type)
	decode("description", &e.Description)
	decode("labels", &e.Labels)
	decode("ephemeral", &e.Ephemeral)
	decode("text", &e.Text)
	decode("thread", &e.Thread)
	decode("newThread", &e.NewThread)
	decode("action", &e.Action)
	decode("status", &e.Status)
	decode("assignees", &e.Assignees)
	decode("summary", &e.Summary)
	decode("context", &e.Context)
	decode("linkedContext", &e.LinkedContext)
	decode("target", &e.Target)
	decode("as", &e.As)
	decode("worker", &e.Worker)
	decode("provider", &e.Provider)
	decode("pid", &e.PID)
	decode("agent", &e.Agent)
	decode("inboxPolicy", &e.InboxPolicy)
	decode("reason", &e.Reason)
	decode("signal", &e.Signal)
	decode("message", &e.Message)
	decode("method", &e.Method)
	decode("outcome", &e.Outcome)
	decode("turnId", &e.TurnID)
	decode("inputSeq", &e.InputSeq)
	decode("targetWorker", &e.TargetWorker)
	decode("messageSeq", &e.MessageSeq)
	decode("synthesized", &e.Synthesized)
	decode("exit_code", &e.ExitCode)
	decode("exit_signal", &e.ExitSignal)
	decode("duration_ms", &e.DurationMS)
	decode("timeout_ms", &e.TimeoutMS)
	decode("idle_timeout_ms", &e.IdleTimeoutMS)
	decode("remaining_ms", &e.RemainingMS)
	decode("files", &e.Files)
	decode("manifests", &e.Manifests)
	// to：字符串或数组都归一化为数组。
	if v, ok := raw["to"]; ok {
		var arr []string
		if err := json.Unmarshal(v, &arr); err == nil {
			e.To = arr
		} else {
			var single string
			if err := json.Unmarshal(v, &single); err == nil {
				e.To = []string{single}
			}
		}
	}
	// title：null 表示清除标题，与缺失区分。
	if v, ok := raw["title"]; ok {
		if strings.TrimSpace(string(v)) == "null" {
			e.Title = nil
			e.TitleNull = true
		} else {
			var s string
			if err := json.Unmarshal(v, &s); err == nil {
				e.Title = &s
			}
		}
	}
	// 全部字段镜像进 Extra（已知字段的具名值优先，见 MarshalJSON）。
	for key, v := range raw {
		var value any
		if err := json.Unmarshal(v, &value); err != nil {
			continue
		}
		if e.Extra == nil {
			e.Extra = map[string]any{}
		}
		e.Extra[key] = value
	}
	return nil
}

// MarshalJSON 把事件写回扁平 JSON：具名字段为准；Extra 只补未知字段；
// TitleNull 时写出 "title": null（清除标题的语义）。
func (e ChannelEvent) MarshalJSON() ([]byte, error) {
	out := map[string]any{}
	for k, v := range e.Extra {
		if !knownEventKeys[k] {
			out[k] = v
		}
	}
	out["seq"] = e.Seq
	out["ts"] = e.TS
	out["kind"] = string(e.Kind)
	out["by"] = e.By
	if e.IdempotencyKey != "" {
		out["idempotencyKey"] = e.IdempotencyKey
	}
	switch len(e.To) {
	case 0:
	case 1:
		out["to"] = e.To[0]
	default:
		out["to"] = e.To
	}
	if e.Origin != "" {
		out["origin"] = string(e.Origin)
	}
	if len(e.Meta) > 0 {
		out["meta"] = e.Meta
	}
	if e.CWD != "" {
		out["cwd"] = e.CWD
	}
	if e.Task != "" {
		out["task"] = e.Task
	}
	if e.Type != "" {
		out["type"] = e.Type
	}
	if e.Description != "" {
		out["description"] = e.Description
	}
	if len(e.Labels) > 0 {
		out["labels"] = e.Labels
	}
	if e.Ephemeral {
		out["ephemeral"] = true
	}
	if e.Text != "" {
		out["text"] = e.Text
	}
	if e.Thread != "" {
		out["thread"] = e.Thread
	}
	if e.NewThread != "" {
		out["newThread"] = e.NewThread
	}
	if e.Action != "" {
		out["action"] = e.Action
	}
	if e.TitleNull {
		out["title"] = nil
	} else if e.Title != nil {
		out["title"] = *e.Title
	}
	if e.Status != "" {
		out["status"] = e.Status
	}
	if len(e.Assignees) > 0 {
		out["assignees"] = e.Assignees
	}
	if e.Summary != "" {
		out["summary"] = e.Summary
	}
	if len(e.Context) > 0 {
		out["context"] = e.Context
	}
	if len(e.LinkedContext) > 0 {
		out["linkedContext"] = e.LinkedContext
	}
	if e.Target != "" {
		out["target"] = e.Target
	}
	if e.As != "" {
		out["as"] = e.As
	}
	if e.Worker != "" {
		out["worker"] = e.Worker
	}
	if e.Provider != "" {
		out["provider"] = e.Provider
	}
	if e.PID != 0 {
		out["pid"] = e.PID
	}
	if e.Agent != "" {
		out["agent"] = e.Agent
	}
	if e.InboxPolicy != "" {
		out["inboxPolicy"] = string(e.InboxPolicy)
	}
	if e.Reason != "" {
		out["reason"] = e.Reason
	}
	if e.Signal != "" {
		out["signal"] = e.Signal
	}
	if e.Message != "" {
		out["message"] = e.Message
	}
	if e.Method != "" {
		out["method"] = e.Method
	}
	if e.Outcome != "" {
		out["outcome"] = e.Outcome
	}
	if e.TurnID != "" {
		out["turnId"] = e.TurnID
	}
	if e.InputSeq != 0 {
		out["inputSeq"] = e.InputSeq
	}
	if e.TargetWorker != "" {
		out["targetWorker"] = e.TargetWorker
	}
	if e.MessageSeq != 0 {
		out["messageSeq"] = e.MessageSeq
	}
	if e.Synthesized {
		out["synthesized"] = true
	}
	if e.ExitCode != nil {
		out["exit_code"] = *e.ExitCode
	}
	if e.ExitSignal != "" {
		out["exit_signal"] = e.ExitSignal
	}
	if e.DurationMS != 0 {
		out["duration_ms"] = e.DurationMS
	}
	if e.TimeoutMS != 0 {
		out["timeout_ms"] = e.TimeoutMS
	}
	if e.IdleTimeoutMS != 0 {
		out["idle_timeout_ms"] = e.IdleTimeoutMS
	}
	if e.RemainingMS != 0 {
		out["remaining_ms"] = e.RemainingMS
	}
	if len(e.Files) > 0 {
		out["files"] = e.Files
	}
	if len(e.Manifests) > 0 {
		out["manifests"] = e.Manifests
	}
	return json.Marshal(out)
}
