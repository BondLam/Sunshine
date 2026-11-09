package store

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"
	"time"
)

// ChannelEventKind 是事件类型。共 21 种。
type ChannelEventKind string

const (
	KindCreate             ChannelEventKind = "create"
	KindJoin               ChannelEventKind = "join"
	KindLeave              ChannelEventKind = "leave"
	KindMessage            ChannelEventKind = "message"
	KindThread             ChannelEventKind = "thread"
	KindContext            ChannelEventKind = "context"
	KindChannel            ChannelEventKind = "channel"
	KindSpawned            ChannelEventKind = "spawned"
	KindKilled             ChannelEventKind = "killed"
	KindRespawned          ChannelEventKind = "respawned"
	KindProgress           ChannelEventKind = "progress"
	KindDone               ChannelEventKind = "done"
	KindError              ChannelEventKind = "error"
	KindWaiting            ChannelEventKind = "waiting"
	KindAwake              ChannelEventKind = "awake"
	KindUndeliverable      ChannelEventKind = "undeliverable"
	KindInterruptRequested ChannelEventKind = "interrupt_requested"
	KindTurnStarted        ChannelEventKind = "turn_started"
	KindTurnFinished       ChannelEventKind = "turn_finished"
	KindInterrupted        ChannelEventKind = "interrupted"
	KindSupervisorWarning  ChannelEventKind = "supervisor_warning"
)

// ChannelEventKinds 是全部 21 种事件 kind。
var ChannelEventKinds = map[ChannelEventKind]bool{
	KindCreate: true, KindJoin: true, KindLeave: true, KindMessage: true,
	KindThread: true, KindContext: true, KindChannel: true, KindSpawned: true,
	KindKilled: true, KindRespawned: true, KindProgress: true, KindDone: true,
	KindError: true, KindWaiting: true, KindAwake: true, KindUndeliverable: true,
	KindInterruptRequested: true, KindTurnStarted: true, KindTurnFinished: true,
	KindInterrupted: true, KindSupervisorWarning: true,
}

// ParseChannelKind 校验事件 kind。
func ParseChannelKind(v string) (ChannelEventKind, error) {
	if !ChannelEventKinds[ChannelEventKind(v)] {
		return "", fmt.Errorf("无效的事件 kind %q", v)
	}
	return ChannelEventKind(v), nil
}

// ParseChannelKinds 解析逗号分隔的 kind 列表，去重保序；空输入返回 nil。
func ParseChannelKinds(v string) ([]ChannelEventKind, error) {
	if strings.TrimSpace(v) == "" {
		return nil, nil
	}
	var out []ChannelEventKind
	seen := map[ChannelEventKind]bool{}
	for _, part := range strings.Split(v, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		kind, err := ParseChannelKind(part)
		if err != nil {
			return nil, err
		}
		if !seen[kind] {
			seen[kind] = true
			out = append(out, kind)
		}
	}
	if len(out) == 0 {
		return nil, nil
	}
	return out, nil
}

// UndeliverableReason 是消息无法投递的原因。
type UndeliverableReason string

const (
	// UndeliverableWorkerUnknown 目标 worker 从未出现过。
	UndeliverableWorkerUnknown UndeliverableReason = "worker-unknown"
	// UndeliverableWorkerTerminal 目标 worker 已处于终态。
	UndeliverableWorkerTerminal UndeliverableReason = "worker-terminal"
)

// InterruptReason 是中断请求的原因。
type InterruptReason string

const (
	InterruptReasonUser       InterruptReason = "user"
	InterruptReasonSystem     InterruptReason = "system"
	InterruptReasonTimeout    InterruptReason = "timeout"
	InterruptReasonSuperseded InterruptReason = "superseded"
)

// InterruptMethod 是 worker runtime 尝试中断的方式。
type InterruptMethod string

const (
	InterruptMethodProvider InterruptMethod = "provider"
	InterruptMethodStdin    InterruptMethod = "stdin"
	InterruptMethodSignal   InterruptMethod = "signal"
	InterruptMethodNone     InterruptMethod = "none"
)

// InterruptOutcome 是中断尝试的结果。
type InterruptOutcome string

const (
	InterruptOutcomeInterrupted  InterruptOutcome = "interrupted"
	InterruptOutcomeQueued       InterruptOutcome = "queued"
	InterruptOutcomeUnsupported  InterruptOutcome = "unsupported"
	InterruptOutcomeNoActiveTurn InterruptOutcome = "no-active-turn"
	InterruptOutcomeFailed       InterruptOutcome = "failed"
)

// SupervisorWarningReason 是 supervisor 预警的原因。
type SupervisorWarningReason string

const (
	// SupervisorWarningApproachingTimeout 接近超时。
	SupervisorWarningApproachingTimeout SupervisorWarningReason = "approaching_timeout"
)

// ChannelEvent 是 events.jsonl 里的一行事件。Base 字段（seq、ts、kind、
// by、idempotencyKey、to、origin、meta）是结构化字段；各 kind 的专有
// 字段（如 text、thread、worker、targetWorker 等）放在 Extra 里，
// 读写时原样保留。To 在 JSON 里可写成字符串或字符串数组，
// 读入后统一为 []string；写出时单个目标写成字符串，多个写成数组。
type ChannelEvent struct {
	Seq            int64            `json:"seq"`
	Ts             string           `json:"ts"`
	Kind           ChannelEventKind `json:"kind"`
	By             string           `json:"by"`
	IdempotencyKey string           `json:"idempotencyKey,omitempty"`
	To             []string         `json:"-"`
	Origin         EventOrigin      `json:"origin,omitempty"`
	Meta           map[string]any   `json:"meta,omitempty"`
	// Extra 是各 kind 的专有字段与未知字段，读写时原样保留。
	Extra map[string]any `json:"-"`
}

var baseEventKeys = map[string]bool{
	"seq": true, "ts": true, "kind": true, "by": true,
	"idempotencyKey": true, "to": true, "origin": true, "meta": true,
}

// MarshalJSON 把 Base 字段与 Extra 合并成一行 JSON 对象。
func (e ChannelEvent) MarshalJSON() ([]byte, error) {
	m := make(map[string]any, len(e.Extra)+8)
	for k, v := range e.Extra {
		m[k] = v
	}
	m["seq"] = e.Seq
	m["ts"] = e.Ts
	m["kind"] = string(e.Kind)
	m["by"] = e.By
	if e.IdempotencyKey != "" {
		m["idempotencyKey"] = e.IdempotencyKey
	}
	switch len(e.To) {
	case 0:
		// 不写 to。
	case 1:
		m["to"] = e.To[0]
	default:
		m["to"] = e.To
	}
	if e.Origin != "" {
		m["origin"] = string(e.Origin)
	}
	if len(e.Meta) > 0 {
		m["meta"] = e.Meta
	}
	return json.Marshal(m)
}

// UnmarshalJSON 解析一行 JSON 事件；未知字段进 Extra。
func (e *ChannelEvent) UnmarshalJSON(data []byte) error {
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	e.Extra = nil
	for k, v := range raw {
		if baseEventKeys[k] {
			continue
		}
		var val any
		if err := json.Unmarshal(v, &val); err != nil {
			return err
		}
		if e.Extra == nil {
			e.Extra = make(map[string]any)
		}
		e.Extra[k] = val
	}
	if v, ok := raw["seq"]; ok {
		if err := json.Unmarshal(v, &e.Seq); err != nil {
			return err
		}
	}
	if v, ok := raw["ts"]; ok {
		if err := json.Unmarshal(v, &e.Ts); err != nil {
			return err
		}
	}
	if v, ok := raw["kind"]; ok {
		if err := json.Unmarshal(v, &e.Kind); err != nil {
			return err
		}
	}
	if v, ok := raw["by"]; ok {
		if err := json.Unmarshal(v, &e.By); err != nil {
			return err
		}
	}
	if v, ok := raw["idempotencyKey"]; ok {
		if err := json.Unmarshal(v, &e.IdempotencyKey); err != nil {
			return err
		}
	}
	if v, ok := raw["to"]; ok {
		var single string
		if err := json.Unmarshal(v, &single); err == nil {
			e.To = []string{single}
		} else {
			var list []string
			if err := json.Unmarshal(v, &list); err != nil {
				return err
			}
			e.To = list
		}
	}
	if v, ok := raw["origin"]; ok {
		if err := json.Unmarshal(v, &e.Origin); err != nil {
			return err
		}
	}
	if v, ok := raw["meta"]; ok {
		if err := json.Unmarshal(v, &e.Meta); err != nil {
			return err
		}
	}
	return nil
}

// StrField 取 Extra 里的字符串字段；不存在或非字符串返回 ""。
func (e ChannelEvent) StrField(key string) string {
	if e.Extra == nil {
		return ""
	}
	s, _ := e.Extra[key].(string)
	return s
}

// NumField 取 Extra 里的数字字段；不存在或非数字返回 0 与 false。
func (e ChannelEvent) NumField(key string) (float64, bool) {
	if e.Extra == nil {
		return 0, false
	}
	n, ok := e.Extra[key].(float64)
	return n, ok
}

// BoolField 取 Extra 里的布尔字段。
func (e ChannelEvent) BoolField(key string) bool {
	if e.Extra == nil {
		return false
	}
	b, _ := e.Extra[key].(bool)
	return b
}

// IsCreateEvent 判断是否为 create 事件。
func (e ChannelEvent) IsCreateEvent() bool { return e.Kind == KindCreate }

// IsThreadEvent 判断是否为带 thread 键的 thread 事件。
func (e ChannelEvent) IsThreadEvent() bool {
	return e.Kind == KindThread && e.StrField("thread") != ""
}

// IsContextEvent 判断是否为 context 事件。
func (e ChannelEvent) IsContextEvent() bool { return e.Kind == KindContext }

// IsChannelMetadataEvent 判断是否为 channel 元数据事件。
func (e ChannelEvent) IsChannelMetadataEvent() bool { return e.Kind == KindChannel }

// AppendableEvent 是 AppendEvent 的输入：除 seq 外的事件内容。
// Ts 为空时自动填当前时间（RFC3339Nano，UTC）。
type AppendableEvent struct {
	Kind           ChannelEventKind
	By             string
	Ts             string
	IdempotencyKey string
	To             []string
	Origin         EventOrigin
	Meta           map[string]any
	// Extra 是各 kind 的专有字段（如 text、thread、worker 等）。
	Extra map[string]any
}

// AppendEvent 在 channel 锁内追加一个事件：先截断撕裂的 JSONL 尾部，
// 再按 idempotencyKey 去重（同 key 不同 kind 报错；同 kind 返回已有
// 事件），seq = lastSeq + 1，追加一行 JSON，更新 .seq sidecar。
func AppendEvent(ref ChannelRef, partial AppendableEvent) (ChannelEvent, error) {
	if err := validateAppendable(partial); err != nil {
		return ChannelEvent{}, err
	}
	if _, err := EnsureChannelDir(ref.Name, ref.Project); err != nil {
		return ChannelEvent{}, err
	}
	jsonl, err := EventsPath(ref.Name, ref.Project)
	if err != nil {
		return ChannelEvent{}, err
	}
	sidecar, err := SeqSidecarPath(ref.Name, ref.Project)
	if err != nil {
		return ChannelEvent{}, err
	}
	lockFile, err := LockPath(ref.Name, ref.Project)
	if err != nil {
		return ChannelEvent{}, err
	}
	var result ChannelEvent
	_, err = WithLock(lockFile, func() (struct{}, error) {
		if err := TruncateIncompleteTail(jsonl); err != nil {
			return struct{}{}, err
		}
		if partial.IdempotencyKey != "" {
			existing, found, err := findIdempotentEvent(jsonl, partial)
			if err != nil {
				return struct{}{}, err
			}
			if found {
				result = existing
				return struct{}{}, nil
			}
		}
		lastSeq, err := ReconcileSeqFile(jsonl, sidecar)
		if err != nil {
			return struct{}{}, err
		}
		ts := partial.Ts
		if ts == "" {
			ts = time.Now().UTC().Format(time.RFC3339Nano)
		}
		ev := ChannelEvent{
			Seq:            lastSeq + 1,
			Ts:             ts,
			Kind:           partial.Kind,
			By:             partial.By,
			IdempotencyKey: partial.IdempotencyKey,
			To:             partial.To,
			Origin:         partial.Origin,
			Meta:           partial.Meta,
			Extra:          partial.Extra,
		}
		line, err := json.Marshal(ev)
		if err != nil {
			return struct{}{}, fmt.Errorf("序列化事件失败：%w", err)
		}
		f, err := os.OpenFile(jsonl, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
		if err != nil {
			return struct{}{}, fmt.Errorf("打开事件日志失败：%w", err)
		}
		if _, err := f.Write(append(line, '\n')); err != nil {
			f.Close()
			return struct{}{}, fmt.Errorf("追加事件失败：%w", err)
		}
		if err := f.Close(); err != nil {
			return struct{}{}, fmt.Errorf("关闭事件日志失败：%w", err)
		}
		if err := WriteSidecar(sidecar, ev.Seq); err != nil {
			return struct{}{}, err
		}
		result = ev
		return struct{}{}, nil
	}, AcquireOptions{})
	if err != nil {
		return ChannelEvent{}, err
	}
	return result, nil
}

// findIdempotentEvent 在 JSONL 里找同 idempotencyKey 的已有事件：
// 同 key 不同 kind 时报错；同 key 同 kind 返回已有事件。
func findIdempotentEvent(jsonl string, partial AppendableEvent) (ChannelEvent, bool, error) {
	events, err := readAllEventsFile(jsonl)
	if err != nil {
		return ChannelEvent{}, false, err
	}
	for _, ev := range events {
		if ev.IdempotencyKey != partial.IdempotencyKey {
			continue
		}
		if ev.Kind != partial.Kind {
			return ChannelEvent{}, false, fmt.Errorf("幂等键 %q 已用于 %s，不能再用于 %s", partial.IdempotencyKey, ev.Kind, partial.Kind)
		}
		return ev, true, nil
	}
	return ChannelEvent{}, false, nil
}

func validateAppendable(partial AppendableEvent) error {
	if !ChannelEventKinds[partial.Kind] {
		return fmt.Errorf("无效的事件 kind %q", string(partial.Kind))
	}
	if strings.TrimSpace(partial.IdempotencyKey) == "" && partial.IdempotencyKey != "" {
		return fmt.Errorf("idempotencyKey 必须是非空字符串")
	}
	if partial.Origin != "" && !EventOrigins[partial.Origin] {
		return fmt.Errorf("无效的 origin %q，必须是 cli、api 或 worker", string(partial.Origin))
	}
	return nil
}

// ReadLastSeq 读 channel 最后一条事件的 seq（不存在返回 0）。
// 与 appendEvent 用同一对账路径，不会读到陈旧的 sidecar。
func ReadLastSeq(ref ChannelRef) (int64, error) {
	jsonl, err := EventsPath(ref.Name, ref.Project)
	if err != nil {
		return 0, err
	}
	if _, err := os.Stat(jsonl); os.IsNotExist(err) {
		return 0, nil
	}
	sidecar, err := SeqSidecarPath(ref.Name, ref.Project)
	if err != nil {
		return 0, err
	}
	return ReconcileSeqFile(jsonl, sidecar)
}

// ReadChannelEventsPagination 是 ReadChannelEvents 的分页参数。
type ReadChannelEventsPagination struct {
	// AfterSeq 返回 seq > AfterSeq 的事件（升序）。
	AfterSeq *int64
	// BeforeSeq 返回 seq < BeforeSeq 的事件（升序）。
	BeforeSeq *int64
	// Limit 页大小上限；未设时默认 DefaultPageSize。
	Limit *int
}

// DefaultPageSize 是带游标分页时的默认页大小。
const DefaultPageSize = 200

// ReadChannelEvents 读 channel 事件。无分页参数时返回全部事件；
// 有游标时按游标分页，Limit 未设时默认 200。afterSeq 与 beforeSeq
// 不可同传；损坏行解析失败时跳过。
func ReadChannelEvents(ref ChannelRef, pagination *ReadChannelEventsPagination) ([]ChannelEvent, error) {
	jsonl, err := EventsPath(ref.Name, ref.Project)
	if err != nil {
		return nil, err
	}
	all, err := readAllEventsFile(jsonl)
	if err != nil {
		return nil, err
	}
	if pagination == nil || (pagination.AfterSeq == nil && pagination.BeforeSeq == nil && pagination.Limit == nil) {
		return all, nil
	}
	if pagination.AfterSeq != nil && pagination.BeforeSeq != nil {
		return nil, fmt.Errorf("readChannelEvents：afterSeq 与 beforeSeq 不可同传")
	}
	limit := 0
	if pagination.Limit != nil {
		if *pagination.Limit < 0 {
			return nil, fmt.Errorf("readChannelEvents：limit 必须是非负整数")
		}
		limit = *pagination.Limit
	}
	if pagination.AfterSeq != nil {
		var page []ChannelEvent
		for _, ev := range all {
			if ev.Seq > *pagination.AfterSeq {
				page = append(page, ev)
			}
		}
		cap := limit
		if pagination.Limit == nil {
			cap = DefaultPageSize
		}
		if len(page) > cap {
			page = page[:cap]
		}
		return page, nil
	}
	if pagination.BeforeSeq != nil {
		var page []ChannelEvent
		for _, ev := range all {
			if ev.Seq < *pagination.BeforeSeq {
				page = append(page, ev)
			}
		}
		cap := limit
		if pagination.Limit == nil {
			cap = DefaultPageSize
		}
		// 取最新一页，仍按升序返回。
		if len(page) > cap {
			page = page[len(page)-cap:]
		}
		return page, nil
	}
	// 只有 limit：返回最新 N 条，升序。
	if len(all) > limit {
		return all[len(all)-limit:], nil
	}
	return all, nil
}

// readAllEventsFile 读整个 JSONL 文件，损坏行跳过。
func readAllEventsFile(file string) ([]ChannelEvent, error) {
	data, err := os.ReadFile(file)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("读取事件日志失败：%w", err)
	}
	var events []ChannelEvent
	for _, line := range bytes.Split(data, []byte("\n")) {
		if len(bytes.TrimSpace(line)) == 0 {
			continue
		}
		var ev ChannelEvent
		if err := json.Unmarshal(line, &ev); err != nil {
			continue
		}
		events = append(events, ev)
	}
	return events, nil
}

// sortEventsBySeq 按 seq 升序排序（ReduceWorkerRegistry 等投影的输入
// 一般已按 seq 升序；本函数供调用方在事件来源无序时使用）。
func sortEventsBySeq(events []ChannelEvent) {
	sort.SliceStable(events, func(i, j int) bool { return events[i].Seq < events[j].Seq })
}
