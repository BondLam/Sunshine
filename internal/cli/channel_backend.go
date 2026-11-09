// Package cli 的 channel 后端接口：CLI 层与 B 组 internal/channel API 包
// 之间的接线面。
//
// 说明（阶段 6 C 组）：
//   - CLI 命令只调 channelBackend 接口（方法名与 PLAN.md 阶段 6 规格一致：
//     CreateChannel、SendMessage、SpawnWorker、InterruptWorker、ListWorkers、
//     context/title 等）；默认实现见 channel_real_backend.go，
//     直接调 B 组 internal/channel API 包，list/rm/kill/prune 用
//     A 组 store 原语组合（上游这四者本就实现在 CLI 层）。
//   - WorkerRuntime：D 组提供 claude/codex 真实实现与测试用 stub；
//     CLI 只持有 channelWorkerRuntime 变量并传给后端 SpawnWorker，
//     绝不直接 spawn 进程。未注入 runtime 时 spawn 走 D 组
//     supervisor.SpawnWorker（守卫 → 写配置 → 派生 supervisor 子进程）。
//   - __supervisor / __parse-trace：runSupervisorFunc 与
//     runParseTraceFunc 指到 D 组 internal/channel/supervisor 包的
//     RunSupervisorFromConfigFile / RunParseTrace。
package cli

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/linrong1982/Sunshine/internal/channel/adapters"
	"github.com/linrong1982/Sunshine/internal/channel/supervisor"
)

// ---------------------------------------------------------------------------
// 常量与事件
// ---------------------------------------------------------------------------

// ChannelScope 是 channel 作用域：project（当前项目）或 global（全局）。
const (
	ScopeProject = "project"
	ScopeGlobal  = "global"
)

// ChannelType 是 channel 类型：chat（时间线式）或 forum（主题区）。
const (
	ChannelTypeChat  = "chat"
	ChannelTypeForum = "forum"
)

// ChannelEvent 是 channel 事件日志里的一行事件（JSON 对象）。
// 用 map 表示：事件字段随 kind 而变（text/thread/action/worker 等），
// map 能无损透传 B 组返回的全部字段，--raw 与 wait 输出直接 Marshal。
type ChannelEvent map[string]any

// Kind 返回事件 kind。
func (e ChannelEvent) Kind() string { return evString(e, "kind") }

// By 返回事件作者。
func (e ChannelEvent) By() string { return evString(e, "by") }

// Seq 返回事件 seq（JSON 数字解码后可能是 float64 / int64 / string）。
func (e ChannelEvent) Seq() int64 { return evInt(e, "seq") }

// Text 返回事件正文（message/thread/interrupt 等）。
func (e ChannelEvent) Text() string { return evString(e, "text") }

// evString 取事件的字符串字段。
func evString(e ChannelEvent, key string) string {
	if e == nil {
		return ""
	}
	s, _ := e[key].(string)
	return s
}

// evInt 取事件的整数字段。
func evInt(e ChannelEvent, key string) int64 {
	if e == nil {
		return 0
	}
	switch v := e[key].(type) {
	case int64:
		return v
	case int:
		return int64(v)
	case float64:
		return int64(v)
	case string:
		var n int64
		_, _ = fmt.Sscanf(v, "%d", &n)
		return n
	}
	return 0
}

// evToList 取事件的 to 字段（字符串或字符串数组）归一化为数组。
func evToList(e ChannelEvent) []string {
	if e == nil {
		return nil
	}
	switch v := e["to"].(type) {
	case string:
		if v == "" {
			return nil
		}
		return []string{v}
	case []string:
		return v
	case []any:
		out := make([]string, 0, len(v))
		for _, item := range v {
			if s, ok := item.(string); ok {
				out = append(out, s)
			}
		}
		return out
	}
	return nil
}

// ChannelEventKinds 是 21 种事件 kind（PLAN.md 阶段 6 存储一节）。
var ChannelEventKinds = []string{
	"create", "join", "leave", "message", "thread", "context", "channel",
	"spawned", "killed", "respawned", "progress", "done", "error",
	"waiting", "awake", "undeliverable", "interrupt_requested",
	"turn_started", "turn_finished", "interrupted", "supervisor_warning",
}

// meaningfulEventKinds 是默认过滤门控：无显式 kind 约束时，只有这些
// kind 能唤醒 wait（与上游 matchesEventFilter 的 MEANINGFUL_EVENT_KINDS 一致）。
var meaningfulEventKinds = map[string]bool{
	"create": true, "join": true, "leave": true, "message": true,
	"thread": true, "context": true, "channel": true, "spawned": true,
	"killed": true, "respawned": true, "done": true, "error": true,
}

// ThreadActions 是 forum thread 事件的合法 action。
var ThreadActions = []string{
	"opened", "comment", "status", "labels", "assignees", "summary", "processed",
}

// DeliveryModes 是 send 的合法投递模式。
var DeliveryModes = []string{"appendOnly", "requireKnownWorker", "requireRunningWorker"}

// InboxPolicies 是 worker 收件箱策略。
var InboxPolicies = []string{"explicitOnly", "broadcastAndExplicit"}

// CodexSandboxModes 是 codex worker 的合法沙箱模式。
var CodexSandboxModes = []string{"read-only", "workspace-write", "danger-full-access"}

// ---------------------------------------------------------------------------
// 结构
// ---------------------------------------------------------------------------

// ChannelRef 定位一个 channel。
type ChannelRef struct {
	Name    string `json:"name"`
	Scope   string `json:"scope"`
	Project string `json:"project"`
	Dir     string `json:"dir"`
}

// ChannelContextEntry 是一条上下文条目：file（绝对路径）或 raw（文本）。
type ChannelContextEntry struct {
	Type string `json:"type"` // file | raw
	Path string `json:"path,omitempty"`
	Text string `json:"text,omitempty"`
}

// ChannelSummary 是 list 一行 channel 的摘要（字段名与上游 JSON 一致）。
type ChannelSummary struct {
	Name          string `json:"name"`
	Project       string `json:"project"`
	CreatedAt     string `json:"createdAt,omitempty"`
	Task          string `json:"task,omitempty"`
	Type          string `json:"type"`
	Description   string `json:"description,omitempty"`
	WorkersAlive  int    `json:"workersAlive"`
	WorkersTotal  int    `json:"workersTotal"`
	LastEventTs   string `json:"lastEventTs,omitempty"`
	LastEventKind string `json:"lastEventKind,omitempty"`
	TotalEvents   int    `json:"totalEvents"`
	Ephemeral     bool   `json:"ephemeral"`
}

// ChannelWorkerState 是 worker 注册表投影的一行（B 组 ListWorkers 返回）。
type ChannelWorkerState struct {
	WorkerID    string `json:"workerId"`
	Channel     string `json:"channel"`
	Agent       string `json:"agent,omitempty"`
	Provider    string `json:"provider,omitempty"`
	Lifecycle   string `json:"lifecycle"` // starting|running|done|error|killed|crashed
	Activity    string `json:"activity"`  // idle|mid-turn
	SpawnedAt   string `json:"spawnedAt,omitempty"`
	UpdatedAt   string `json:"updatedAt,omitempty"`
	StartedBy   string `json:"startedBy,omitempty"`
	Reason      string `json:"reason,omitempty"`
	Error       string `json:"error,omitempty"`
	IdleSince   string `json:"idleSince,omitempty"`
	InboxPolicy string `json:"inboxPolicy,omitempty"`
}

// PruneCandidate 是一条待清理 channel 候选。
type PruneCandidate struct {
	Name    string `json:"name"`
	Project string `json:"project"`
	Reason  string `json:"reason"`
	LastTs  string `json:"lastTs,omitempty"`
}

// SpawnResult 是 spawn 成功后返回的信息（上游打印的 JSON）。
type SpawnResult struct {
	PID    int    `json:"pid"`
	Log    string `json:"log"`
	Worker string `json:"worker"`
}

// InterruptResult 是 interrupt 的结果。
type InterruptResult struct {
	Event       ChannelEvent `json:"event"`
	Interrupted bool         `json:"interrupted"`
	Delivery    string       `json:"delivery,omitempty"`
}

// ---------------------------------------------------------------------------
// 选项
// ---------------------------------------------------------------------------

// CreateChannelOptions 是 channel create 的参数（与 PLAN.md 命令表一致）。
type CreateChannelOptions struct {
	Name        string
	Scope       string // project | global，空为 project
	Type        string // chat | forum，空为 chat
	Task        string
	Project     string
	Labels      []string
	Description string
	Cwd         string
	By          string // 默认 main
	Force       bool
	Ephemeral   bool
	Context     []ChannelContextEntry
	Origin      string // 调用方标记：空或 "run"（一次性流程）
}

// SendMessageOptions 是 channel send 的参数。
type SendMessageOptions struct {
	Channel      string
	Scope        string
	By           string
	Text         string
	To           []string // 空为广播
	DeliveryMode string   // 空为 appendOnly
}

// WaitFilter 是 channel wait 的匹配条件（与上游 ChannelEventFilter 对应）。
type WaitFilter struct {
	Self            string // 排除自己发的事件
	From            []string
	Kinds           []string
	To              string // 空时默认等于 Self
	Thread          string
	Action          string
	IncludeProgress bool
}

// ReadEventsOptions 是 channel messages 的读过滤条件。
type ReadEventsOptions struct {
	AfterSeq          *int64
	BeforeSeq         *int64
	Limit             int // 0 为不截断；>0 时只取前 N 条过滤后事件
	Kind              string
	From              []string
	To                string
	Thread            string
	Action            string
	IncludeProgress   bool // 默认 true；--no-progress 时为 false
	IncludeNonMeaning bool // messages 固定 true（上游如此）
}

// ListChannelsOptions 是 channel list 的参数。
type ListChannelsOptions struct {
	Scope       string
	Project     string // 按 task 字段子串过滤
	All         bool   // 含临时 channel
	AllProjects bool   // 扫描全部项目桶
}

// PruneOptions 是 channel prune 的参数。
type PruneOptions struct {
	Scope     string
	All       bool
	Empty     bool
	Idle      time.Duration // 0 为未指定
	Ephemeral bool
	Keep      []string
}

// SpawnWorkerOptions 是 channel spawn 的参数（CLI 已解析 agent 与上下文，
// SystemPrompt 为组装好的完整 system prompt；后端只负责拉起与记账）。
type SpawnWorkerOptions struct {
	Channel          string
	Scope            string
	Provider         string // claude | codex
	As               string // worker 名
	Agent            string
	Cwd              string
	Model            string
	Resume           string
	Sandbox          string        // 仅 codex，默认 workspace-write
	Timeout          time.Duration // 0 为不设
	WarnBefore       time.Duration // 默认 5m，0 禁用
	Files            []string      // 展开后的上下文文件（绝对路径）
	JSONLManifests   []string
	By               string
	InboxPolicy      string        // 默认 explicitOnly
	IdleTimeout      time.Duration // 默认 5m，0 禁用
	MaxLiveWorkers   int           // 默认 6，0 禁用
	SystemPrompt     string
	ContextFiles     []string
	ContextManifests []string
}

// InterruptOptions 是 channel interrupt 的参数。
type InterruptOptions struct {
	Channel string
	Scope   string
	By      string
	Worker  string // --to 目标 worker
	Message string
	Reason  string // 固定 "user"
}

// ---------------------------------------------------------------------------
// WorkerRuntime（D 组提供；PLAN.md 关键决策 3：接口含 Start/Interrupt/Stop）
// ---------------------------------------------------------------------------

// WorkerRuntime 是拉起/打断/停止一个 worker 的运行时接口。
// D 组提供 claude/codex 真实实现与测试用 stub；CLI 层不直接 spawn 进程。
type WorkerRuntime interface {
	// Start 拉起 worker，返回 worker 身份与进程信息。
	Start(ctx context.Context, opts WorkerStartOptions) (WorkerStartResult, error)
	// Interrupt 打断 worker 当前回合。
	Interrupt(ctx context.Context, workerID string) (bool, error)
	// Stop 停止 worker。
	Stop(ctx context.Context, workerID string) error
}

// WorkerStartOptions 是 WorkerRuntime.Start 的参数。
type WorkerStartOptions struct {
	Channel      string
	Worker       string
	Provider     string
	Cwd          string
	SystemPrompt string
	Model        string
	Resume       string
	Sandbox      string
	Timeout      time.Duration
	WarnBefore   time.Duration
	IdleTimeout  time.Duration
}

// WorkerStartResult 是 WorkerRuntime.Start 的返回。
type WorkerStartResult struct {
	WorkerID  string `json:"workerId"`
	Provider  string `json:"provider,omitempty"`
	PID       int    `json:"pid,omitempty"`
	StartedAt string `json:"startedAt,omitempty"`
}

// ---------------------------------------------------------------------------
// channelBackend 接口（方法名以 PLAN.md 规格为准）
// ---------------------------------------------------------------------------

type channelBackend interface {
	CreateChannel(ctx context.Context, opts CreateChannelOptions) (ChannelRef, ChannelEvent, error)
	SendMessage(ctx context.Context, opts SendMessageOptions) (ChannelEvent, error)
	// ReadEvents 读已有事件（messages 用；不含跟随流）。
	ReadEvents(ctx context.Context, ref ChannelRef, opts ReadEventsOptions) ([]ChannelEvent, error)
	// WatchEvents 从“当前时刻”起流式推送匹配事件（wait 用；旧事件不重放，
	// 与上游 watchEvents 默认 from-now 语义一致）。ctx 取消时关闭通道。
	WatchEvents(ctx context.Context, ref ChannelRef, filter WaitFilter) (<-chan ChannelEvent, error)
	// RequestInterrupt 只追加 interrupt_requested 事件（持久化意图）。
	// InterruptWorker 在有 runtime 时尝试中断并返回结果。
	RequestInterrupt(ctx context.Context, opts InterruptOptions) (ChannelEvent, error)
	InterruptWorker(ctx context.Context, opts InterruptOptions, runtime WorkerRuntime) (InterruptResult, error)
	SpawnWorker(ctx context.Context, opts SpawnWorkerOptions, runtime WorkerRuntime) (SpawnResult, error)
	ListChannels(ctx context.Context, opts ListChannelsOptions) ([]ChannelSummary, error)
	ListWorkers(ctx context.Context, ref ChannelRef) ([]ChannelWorkerState, error)
	ListPruneCandidates(ctx context.Context, opts PruneOptions) ([]PruneCandidate, error)
	RemoveChannel(ctx context.Context, ref ChannelRef) error
	KillWorker(ctx context.Context, ref ChannelRef, worker string, force bool) error
	AddChannelContext(ctx context.Context, ref ChannelRef, by string, entries []ChannelContextEntry) (ChannelEvent, error)
	DeleteChannelContext(ctx context.Context, ref ChannelRef, by string, entries []ChannelContextEntry) (ChannelEvent, error)
	ListChannelContext(ctx context.Context, ref ChannelRef) ([]ChannelContextEntry, error)
	AddThreadContext(ctx context.Context, ref ChannelRef, thread string, by string, entries []ChannelContextEntry) (ChannelEvent, error)
	DeleteThreadContext(ctx context.Context, ref ChannelRef, thread string, by string, entries []ChannelContextEntry) (ChannelEvent, error)
	ListThreadContext(ctx context.Context, ref ChannelRef, thread string) ([]ChannelContextEntry, error)
	SetChannelTitle(ctx context.Context, ref ChannelRef, by string, title string) (ChannelEvent, error)
	ClearChannelTitle(ctx context.Context, ref ChannelRef, by string) (ChannelEvent, error)
}

// ---------------------------------------------------------------------------
// 接线点（可注入；测试与总协调人合并时替换）
// ---------------------------------------------------------------------------

// newChannelBackend 构造后端，默认是真实实现（channel_real_backend.go）。
// 测试用它注入 fake 后端。
var newChannelBackend = func() channelBackend { return realChannelBackend{} }

// channelWorkerRuntime 是 spawn/run 拉起 worker 用的运行时。
// 默认 nil：后端 SpawnWorker 在 nil 时走 D 组 supervisor.SpawnWorker
// （守卫 → 写配置 → 派生 supervisor 子进程）；
// 非 nil 时走 B 组 SpawnWorker（测试注入 stub 时用）。
var channelWorkerRuntime WorkerRuntime

// runSupervisorFunc 是 `channel __supervisor` 的实现：
// D 组 supervisor.RunSupervisorFromConfigFile。
var runSupervisorFunc = func(ctx context.Context, channelName, worker, configPath string) error {
	return supervisor.RunSupervisorFromConfigFile(ctx, channelName, worker, configPath)
}

// runParseTraceFunc 是 `channel __parse-trace` 的实现：
// D 组 supervisor.RunParseTrace。
var runParseTraceFunc = func(_ context.Context, adapter, file string) error {
	provider, err := adapters.ParseProvider(adapter)
	if err != nil {
		return err
	}
	return supervisor.RunParseTrace(provider, file, os.Stdout)
}

// ---------------------------------------------------------------------------
// wait 事件匹配（与上游 matchesEventFilter 同语义；
// 供 CLI 的 wait 循环与测试 fake 后端共用，B 组后端自行实现同款过滤）
// ---------------------------------------------------------------------------

// channelEventMatchesWait 判断事件是否唤醒 wait。
//   - 排除自己发的事件（self）；
//   - 无显式 kind 约束时只认有意义的 kind（progress/waiting/awake/
//     undeliverable/interrupt_requested/turn_started/turn_finished/
//     interrupted/supervisor_warning 默认不唤醒）；显式 --kind 可绕过该门控；
//   - progress 事件须 --include-progress 才唤醒；
//   - --thread/--action 只对 kind=thread 的事件生效；
//   - --to：无 to 的广播算匹配，有 to 的须包含目标（与上游一致）。
func channelEventMatchesWait(ev ChannelEvent, f WaitFilter) bool {
	if ev == nil {
		return false
	}
	if f.Self != "" && ev.By() == f.Self {
		return false
	}
	if ev.Kind() == "progress" && !f.IncludeProgress {
		return false
	}
	if len(f.Kinds) == 0 && !meaningfulEventKinds[ev.Kind()] {
		return false
	}
	if len(f.Kinds) > 0 {
		matched := false
		for _, k := range f.Kinds {
			if ev.Kind() == k {
				matched = true
				break
			}
		}
		if !matched {
			return false
		}
	}
	if f.Thread != "" {
		if ev.Kind() != "thread" || evString(ev, "thread") != f.Thread {
			return false
		}
	}
	if f.Action != "" {
		if ev.Kind() != "thread" || evString(ev, "action") != f.Action {
			return false
		}
	}
	if len(f.From) > 0 {
		matched := false
		for _, from := range f.From {
			if ev.By() == from {
				matched = true
				break
			}
		}
		if !matched {
			return false
		}
	}
	to := f.To
	if to == "" {
		to = f.Self
	}
	if to != "" {
		if to == "exclusive" {
			if len(evToList(ev)) == 0 {
				return false
			}
		} else {
			targets := evToList(ev)
			if len(targets) > 0 {
				matched := false
				for _, t := range targets {
					if t == to {
						matched = true
						break
					}
				}
				if !matched {
					return false
				}
			}
			// 无 to 的广播算匹配。
		}
	}
	return true
}
