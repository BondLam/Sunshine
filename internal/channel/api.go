package channel

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/linrong1982/Sunshine/internal/channel/store"
)

// ChannelAddress 是定位一个 channel 的寻址参数。
type ChannelAddress struct {
	// Channel 是 channel 名。
	Channel string
	// Scope 是作用域：project、global，空为自动解析。
	Scope string
	// ProjectKey 是显式指定的存储分桶 key；非空时优先于 Scope/CWD。
	ProjectKey string
	// CWD 是用于推导项目桶的工作目录；空为当前工作目录。
	CWD string
}

// CreateChannelOptions 是建 channel 的参数。
type CreateChannelOptions struct {
	ChannelAddress
	By          string
	Origin      EventOrigin
	Meta        map[string]any
	Type        string
	Task        string
	Project     string
	Labels      []string
	Description string
	Context     []ContextEntry
	Ephemeral   bool
	Force       bool
}

// SendMessageOptions 是发消息的参数。
type SendMessageOptions struct {
	ChannelAddress
	By             string
	Origin         EventOrigin
	Meta           map[string]any
	IdempotencyKey string
	Text           string
	// To 为空表示广播；否则为定向目标 worker 列表。
	To []string
	// DeliveryMode 为空时默认 appendOnly。
	DeliveryMode string
}

// ReadEventsOptions 是读事件的参数。
type ReadEventsOptions struct {
	ChannelAddress
	Pagination
}

// WatchEventsOptions 是单 channel 监听的参数。
type WatchEventsOptions struct {
	Filter    store.ChannelEventFilter
	FromStart bool
	SinceSeq  *int64
	// PollInterval 是轮询间隔；为 0 时用 store 的默认值（200ms）。
	PollInterval time.Duration
}

// WatchChannelsOptions 是跨 channel 监听的参数。
type WatchChannelsOptions struct {
	// Global 为 true 时监听全局桶；否则按 ProjectKey/CWD 监听项目桶。
	Global bool
	// ProjectKey 为空时按 CWD（或当前工作目录）推导。
	ProjectKey string
	// CWD 用于推导项目桶；空为当前工作目录。
	CWD    string
	Filter store.ChannelEventFilter
	// Cursor 是每 channel 的续跑游标。
	Cursor ChannelCursor
	// FromStartNewChannels 为 true 时后发现的 channel 也从头读。
	FromStartNewChannels bool
	// DiscoveryIntervalMs 是新 channel 的发现间隔毫秒，默认 500。
	DiscoveryIntervalMs int64
}

// ListWorkersOptions 是列出 worker 的参数。
type ListWorkersOptions struct {
	ChannelAddress
	// IncludeTerminal 为 true 时包含终态 worker；默认只列非终态。
	IncludeTerminal bool
}

// WatchWorkersOptions 是监听 worker 注册表的参数。
type WatchWorkersOptions struct {
	ListWorkersOptions
	SinceSeq *int64
}

// WorkerInboxMessage 是某 worker 收件箱里的一条消息。
type WorkerInboxMessage struct {
	WorkerID string
	Event    ChannelEvent
	Seq      int64
	Cursor   int64
}

// ReadWorkerInboxOptions 是读 worker 收件箱的参数。
type ReadWorkerInboxOptions struct {
	ChannelAddress
	WorkerID string
	AfterSeq *int64
	Limit    *int
	// IncludeTerminal 为 true 时允许读终态 worker 的收件箱。
	IncludeTerminal bool
}

// WatchWorkerInboxOptions 是监听 worker 收件箱的参数。
type WatchWorkerInboxOptions struct {
	ChannelAddress
	WorkerID  string
	SinceSeq  *int64
	FromStart bool
}

// WorkerInboxError 是读/监听 worker 收件箱时的错误。
type WorkerInboxError struct {
	Code     string
	Channel  string
	WorkerID string
}

// WorkerInboxErrorCode 是收件箱错误的分类。
const (
	// WorkerInboxWorkerNotFound 表示 worker 不存在。
	WorkerInboxWorkerNotFound = "WORKER_INBOX_WORKER_NOT_FOUND"
	// WorkerInboxWorkerTerminal 表示 worker 已终态。
	WorkerInboxWorkerTerminal = "WORKER_INBOX_WORKER_TERMINAL"
)

func (e *WorkerInboxError) Error() string {
	switch e.Code {
	case WorkerInboxWorkerNotFound:
		return fmt.Sprintf("在频道 %q 中找不到 worker %q", e.Channel, e.WorkerID)
	case WorkerInboxWorkerTerminal:
		return fmt.Sprintf("频道 %q 中的 worker %q 已终态", e.Channel, e.WorkerID)
	default:
		return fmt.Sprintf("worker %q 收件箱错误", e.WorkerID)
	}
}

// WorkerRuntimeObservation 是对一个 worker 的本机运行时观测。
type WorkerRuntimeObservation struct {
	WorkerID        string `json:"workerId"`
	PID             *int   `json:"pid,omitempty"`
	WorkerPID       *int   `json:"workerPid,omitempty"`
	SupervisorAlive *bool  `json:"supervisorAlive,omitempty"`
	WorkerAlive     *bool  `json:"workerAlive,omitempty"`
	ObservedAt      string `json:"observedAt"`
	Source          string `json:"source"`
}

// ReconcileWorkerLivenessOptions 是 worker 存活对账的参数。
type ReconcileWorkerLivenessOptions struct {
	ChannelAddress
	// AppendTerminalEvents 为 true 时把提议的终态事件真正追加落盘；
	// 默认 false，只提议不写盘。
	AppendTerminalEvents bool
}

// ReconcileWorkerLivenessResult 是存活对账的结果。
type ReconcileWorkerLivenessResult struct {
	Observations   []WorkerRuntimeObservation `json:"observations"`
	ProposedEvents []ChannelEvent             `json:"proposedEvents"`
	Appended       []ChannelEvent             `json:"appended"`
}

// InterruptDelivery 是中断投递的结果分类。
type InterruptDelivery string

const (
	// InterruptDeliveryInterruptedCurrentTurn 表示中断了当前回合。
	InterruptDeliveryInterruptedCurrentTurn InterruptDelivery = "interrupted-current-turn"
	// InterruptDeliveryNoActiveTurn 表示当时无进行中的回合。
	InterruptDeliveryNoActiveTurn InterruptDelivery = "no-active-turn"
	// InterruptDeliveryWorkerTerminal 表示目标 worker 已终态。
	InterruptDeliveryWorkerTerminal InterruptDelivery = "worker-terminal"
	// InterruptDeliveryWorkerUnknown 表示目标 worker 未知。
	InterruptDeliveryWorkerUnknown InterruptDelivery = "worker-unknown"
)

// InterruptWorkerOptions 是中断 worker 的参数。
type InterruptWorkerOptions struct {
	ChannelAddress
	By       string
	Origin   EventOrigin
	Meta     map[string]any
	WorkerID string
	Message  string
	Reason   InterruptReason
}

// InterruptWorkerResult 是中断 worker 的结果。
type InterruptWorkerResult struct {
	// Event 是最后追加的事件（尝试中断时为 interrupted，否则为 interrupt_requested）。
	Event       ChannelEvent      `json:"event"`
	Interrupted bool              `json:"interrupted"`
	Delivery    InterruptDelivery `json:"delivery"`
}

// SpawnWorkerOptions 是 spawn worker 的参数。
type SpawnWorkerOptions struct {
	ChannelAddress
	By           string
	Origin       EventOrigin
	Meta         map[string]any
	WorkerID     string
	Provider     string
	Agent        string
	SystemPrompt string
	Model        string
	Resume       string
	Sandbox      string
	ContextFiles []string
	InboxPolicy  string
	Env          map[string]string
}

// ContextMutationOptions 是 channel 级上下文变更的参数。
type ContextMutationOptions struct {
	ChannelAddress
	By      string
	Origin  EventOrigin
	Meta    map[string]any
	Context []ContextEntry
}

// ThreadContextMutationOptions 是 thread 级上下文变更的参数。
type ThreadContextMutationOptions struct {
	ContextMutationOptions
	Thread string
}

// ThreadContextListOptions 是列出 thread 级上下文的参数。
type ThreadContextListOptions struct {
	ChannelAddress
	Thread string
}

// SetChannelTitleOptions 是设置 channel 标题的参数。
type SetChannelTitleOptions struct {
	ChannelAddress
	By     string
	Origin EventOrigin
	Meta   map[string]any
	Title  string
}

// ClearChannelTitleOptions 是清除 channel 标题的参数。
type ClearChannelTitleOptions struct {
	ChannelAddress
	By     string
	Origin EventOrigin
	Meta   map[string]any
}

// resolveRef 解析 channel 引用。forCreate 为 true 时不要求 channel
// 已存在（建 channel 用）；否则要求已存在。
// ProjectKey 非空时直接定桶；否则走 store 的解析
// （显式 scope 只查该桶；未指定时按当前项目桶 → global → 其他项目桶搜索，
// 多处同名时报错要求明确指定 scope）。
func resolveRef(addr ChannelAddress, forCreate bool) (ChannelRef, error) {
	if err := store.AssertSafeName(addr.Channel, "channel"); err != nil {
		return ChannelRef{}, err
	}
	scope := ChannelScope(addr.Scope)
	if scope != "" && scope != ScopeProject && scope != ScopeGlobal {
		return ChannelRef{}, fmt.Errorf("无效的 --scope %q，必须是 project 或 global", addr.Scope)
	}
	if addr.ProjectKey != "" {
		if scope == "" {
			scope = ScopeProject
		}
		dir, err := store.ChannelDir(addr.Channel, addr.ProjectKey)
		if err != nil {
			return ChannelRef{}, err
		}
		ref := ChannelRef{Name: addr.Channel, Scope: scope, Project: addr.ProjectKey, Dir: dir}
		if !forCreate {
			p, err := store.EventsPath(addr.Channel, addr.ProjectKey)
			if err != nil {
				return ChannelRef{}, err
			}
			if _, err := os.Stat(p); err != nil {
				return ChannelRef{}, fmt.Errorf("channel %q 在 %s 作用域（%s）下不存在", addr.Channel, string(scope), addr.ProjectKey)
			}
		}
		return ref, nil
	}
	opts := store.ResolveChannelOptions{Scope: scope, Cwd: addr.CWD}
	if forCreate {
		return store.ResolveChannel(addr.Channel, opts)
	}
	return store.ResolveExistingChannelRef(addr.Channel, opts)
}

// ResolveChannel 解析一个已存在的 channel 引用（不存在时报错）。
func ResolveChannel(addr ChannelAddress) (ChannelRef, error) {
	return resolveRef(addr, false)
}

// ResolveChannelForCreate 解析新建 channel 的引用（不要求已存在）。
func ResolveChannelForCreate(addr ChannelAddress) (ChannelRef, error) {
	return resolveRef(addr, true)
}

// defaultBy 在 By 为空时补默认作者 main（与 CLI 的 --by 默认值一致）。
func defaultBy(by string) string {
	if strings.TrimSpace(by) == "" {
		return "main"
	}
	return by
}

// cwdOrDefault 返回调用方的工作目录；取不到时返回空字符串。
func cwdOrDefault(cwd string) string {
	if strings.TrimSpace(cwd) != "" {
		return cwd
	}
	dir, err := os.Getwd()
	if err != nil {
		return ""
	}
	return dir
}

// newAppendable 由通用字段拼出 store 的追加输入。
func newAppendable(kind EventKind, by string, origin EventOrigin, meta map[string]any) store.AppendableEvent {
	partial := store.AppendableEvent{
		Kind:   store.ChannelEventKind(kind),
		By:     defaultBy(by),
		Origin: store.EventOrigin(origin),
	}
	if len(meta) > 0 {
		partial.Meta = meta
	}
	return partial
}

// withExtra 给追加输入补专属字段。
func withExtra(partial store.AppendableEvent, extra map[string]any) store.AppendableEvent {
	if len(extra) > 0 {
		if partial.Extra == nil {
			partial.Extra = map[string]any{}
		}
		for k, v := range extra {
			partial.Extra[k] = v
		}
	}
	return partial
}

// readAllEvents 读某 channel 的全部事件（无分页）。
func readAllEvents(ref ChannelRef) ([]store.ChannelEvent, error) {
	return store.ReadChannelEvents(ref, nil)
}

// readRegistry 读事件并折叠出 worker 注册表。
func readRegistry(ref ChannelRef) (WorkerRegistry, []store.ChannelEvent, error) {
	events, err := readAllEvents(ref)
	if err != nil {
		return WorkerRegistry{}, nil, err
	}
	return store.ReduceWorkerRegistry(events, &ref), events, nil
}

// toAPIEvents 把 store 事件列表转为 API 层事件列表。
func toAPIEvents(events []store.ChannelEvent) ([]ChannelEvent, error) {
	out := make([]ChannelEvent, 0, len(events))
	for _, ev := range events {
		converted, err := fromStoreEvent(ev)
		if err != nil {
			return nil, err
		}
		out = append(out, converted)
	}
	return out, nil
}

// parseDeliveryModeOpt 校验投递模式；空字符串返回默认 appendOnly。
func parseDeliveryModeOpt(v string) (DeliveryMode, error) {
	if strings.TrimSpace(v) == "" {
		return DeliveryModeAppendOnly, nil
	}
	return store.ParseDeliveryMode(v)
}

// parseInboxPolicyOpt 校验收件箱策略；空字符串返回默认 explicitOnly。
func parseInboxPolicyOpt(v string) (InboxPolicy, error) {
	if strings.TrimSpace(v) == "" {
		return DefaultInboxPolicy, nil
	}
	return store.ParseInboxPolicy(v)
}

// parseInterruptReasonOpt 校验中断原因；空字符串表示不写。
func parseInterruptReasonOpt(v InterruptReason) (InterruptReason, error) {
	if v == "" {
		return "", nil
	}
	switch v {
	case InterruptReasonUser, InterruptReasonSystem, InterruptReasonTimeout, InterruptReasonSuperseded:
		return v, nil
	default:
		return "", fmt.Errorf("无效的中断原因 %q，应为 user、system、timeout 或 superseded", string(v))
	}
}
