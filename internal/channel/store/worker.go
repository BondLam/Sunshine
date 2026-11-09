package store

import (
	"sort"
	"strings"
)

// WorkerLifecycle 是 worker 的进程生命周期，纯由持久事件投影。
type WorkerLifecycle string

const (
	// WorkerLifecycleStarting 启动中。
	WorkerLifecycleStarting WorkerLifecycle = "starting"
	// WorkerLifecycleRunning 运行中。
	WorkerLifecycleRunning WorkerLifecycle = "running"
	// WorkerLifecycleDone 正常完成。
	WorkerLifecycleDone WorkerLifecycle = "done"
	// WorkerLifecycleError 出错终止。
	WorkerLifecycleError WorkerLifecycle = "error"
	// WorkerLifecycleKilled 被杀掉。
	WorkerLifecycleKilled WorkerLifecycle = "killed"
	// WorkerLifecycleCrashed 崩溃。
	WorkerLifecycleCrashed WorkerLifecycle = "crashed"
)

// TerminalLifecycles 是终态生命周期集合。
var TerminalLifecycles = map[WorkerLifecycle]bool{
	WorkerLifecycleDone:    true,
	WorkerLifecycleError:   true,
	WorkerLifecycleKilled:  true,
	WorkerLifecycleCrashed: true,
}

// IsTerminalLifecycle 判断生命周期是否为终态。
func IsTerminalLifecycle(lifecycle WorkerLifecycle) bool {
	return TerminalLifecycles[lifecycle]
}

// WorkerActivity 是 worker 的回合活动：是否正在跑一回合。
type WorkerActivity string

const (
	// WorkerActivityIdle 空闲。
	WorkerActivityIdle WorkerActivity = "idle"
	// WorkerActivityMidTurn 回合进行中。
	WorkerActivityMidTurn WorkerActivity = "mid-turn"
)

// WorkerState 是单个 worker 的投影状态。
type WorkerState struct {
	WorkerID            string
	Channel             *ChannelRef
	Agent               string
	Provider            string
	Lifecycle           WorkerLifecycle
	Terminal            bool
	Activity            WorkerActivity
	ActiveTurnID        string
	ActiveTurnStartedAt string
	// PendingMessageCount 是可投递 message 事件数：seq 大于最新已消费
	// turn_started.inputSeq 且符合该 worker inbox 策略的条数。
	// 终态 worker 恒为 0。只从持久事件派生，不读本地游标。
	PendingMessageCount int
	InboxPolicy         InboxPolicy
	SpawnedAt           string
	UpdatedAt           string
	StartedBy           string
	ExitCode            *int
	Signal              string
	Reason              string
	Error               string
	IdleSince           string
	LastSeq             int64
}

// WorkerRegistry 是 worker 注册表投影。
type WorkerRegistry struct {
	Workers []WorkerState
}

// FindWorker 按 workerId 查找；不存在返回 nil。
func (r WorkerRegistry) FindWorker(workerID string) *WorkerState {
	for i := range r.Workers {
		if r.Workers[i].WorkerID == workerID {
			return &r.Workers[i]
		}
	}
	return nil
}

// workerAcc 是投影过程中的累加器：比 WorkerState 多一个已消费 inputSeq。
type workerAcc struct {
	WorkerState
	consumedInputSeq int64
}

// identifyWorker 解析事件指向的 worker id，以及该 kind 是否允许
// 新建 worker 条目（只有 spawned 与明确带 worker 身份的终态事件可以，
// 避免普通 by 别名造出幽灵 worker）。
func identifyWorker(ev ChannelEvent) (id string, canCreate bool, ok bool) {
	switch ev.Kind {
	case KindSpawned:
		id = ev.StrField("as")
		if id == "" {
			return "", false, false
		}
		return id, true, true
	case KindTurnStarted, KindTurnFinished, KindInterruptRequested, KindInterrupted:
		id = ev.StrField("worker")
		if id == "" {
			return "", false, false
		}
		return id, false, true
	case KindKilled:
		if id = ev.StrField("worker"); id == "" {
			id = ev.StrField("as")
		}
		if id != "" {
			return id, true, true
		}
		if strings.HasPrefix(ev.By, "supervisor:") {
			return strings.TrimPrefix(ev.By, "supervisor:"), true, true
		}
		return ev.By, false, true
	case KindDone, KindError:
		if id = ev.StrField("worker"); id == "" {
			id = ev.StrField("as")
		}
		if id != "" {
			return id, true, true
		}
		if strings.HasPrefix(ev.By, "supervisor:") {
			return strings.TrimPrefix(ev.By, "supervisor:"), true, true
		}
		return ev.By, false, true
	default:
		return "", false, false
	}
}

func blankWorker(id string, ev ChannelEvent) *workerAcc {
	return &workerAcc{
		WorkerState: WorkerState{
			WorkerID:    id,
			Lifecycle:   WorkerLifecycleRunning,
			Terminal:    false,
			Activity:    WorkerActivityIdle,
			InboxPolicy: DefaultInboxPolicy,
			UpdatedAt:   ev.Ts,
			LastSeq:     ev.Seq,
		},
	}
}

// ReduceWorkerRegistry 是纯函数：只从事件日志折叠出 WorkerRegistry，
// 不读 pid 文件、不读本地游标。channel 为空时不给 WorkerState 贴 channel。
func ReduceWorkerRegistry(events []ChannelEvent, channel *ChannelRef) WorkerRegistry {
	acc := map[string]*workerAcc{}
	var order []string

	for _, ev := range events {
		id, canCreate, ok := identifyWorker(ev)
		if !ok {
			continue
		}
		w, exists := acc[id]
		if !exists {
			if !canCreate {
				continue
			}
			w = blankWorker(id, ev)
			acc[id] = w
			order = append(order, id)
		}
		w.UpdatedAt = ev.Ts
		w.LastSeq = ev.Seq

		switch ev.Kind {
		case KindSpawned:
			w.Lifecycle = WorkerLifecycleRunning
			w.Terminal = false
			w.Activity = WorkerActivityIdle
			w.ActiveTurnID = ""
			w.ActiveTurnStartedAt = ""
			w.ExitCode = nil
			w.Signal = ""
			w.Reason = ""
			w.Error = ""
			w.SpawnedAt = ev.Ts
			w.IdleSince = ev.Ts
			w.StartedBy = ev.By
			if v := ev.StrField("provider"); v != "" {
				w.Provider = v
			}
			if v := ev.StrField("agent"); v != "" {
				w.Agent = v
			}
			if v := ev.StrField("inboxPolicy"); v != "" {
				if policy, err := ParseInboxPolicy(v); err == nil {
					w.InboxPolicy = policy
				}
			}
		case KindTurnStarted:
			w.Activity = WorkerActivityMidTurn
			w.ActiveTurnID = ev.StrField("turnId")
			w.ActiveTurnStartedAt = ev.Ts
			w.IdleSince = ""
			if inputSeq, ok := ev.NumField("inputSeq"); ok && int64(inputSeq) > w.consumedInputSeq {
				w.consumedInputSeq = int64(inputSeq)
			}
		case KindTurnFinished:
			w.Activity = WorkerActivityIdle
			w.ActiveTurnID = ""
			w.ActiveTurnStartedAt = ""
			w.IdleSince = ev.Ts
		case KindInterrupted:
			// 中断会 abort 当前回合。
			w.Activity = WorkerActivityIdle
			w.ActiveTurnID = ""
			w.ActiveTurnStartedAt = ""
			w.IdleSince = ev.Ts
		case KindInterruptRequested:
			// 持久化意图，不改生命周期与活动。
		case KindDone:
			w.Activity = WorkerActivityIdle
			w.ActiveTurnID = ""
			w.ActiveTurnStartedAt = ""
			if ev.BoolField("synthesized") {
				w.Lifecycle = WorkerLifecycleDone
				w.Terminal = true
				if code, ok := ev.NumField("exit_code"); ok {
					c := int(code)
					w.ExitCode = &c
				}
				w.IdleSince = ""
			} else {
				w.IdleSince = ev.Ts
			}
		case KindError:
			w.Activity = WorkerActivityIdle
			w.ActiveTurnID = ""
			w.ActiveTurnStartedAt = ""
			if v := ev.StrField("message"); v != "" {
				w.Error = v
			}
			if ev.BoolField("synthesized") || strings.HasPrefix(ev.By, "supervisor:") {
				w.Lifecycle = WorkerLifecycleError
				w.Terminal = true
				if code, ok := ev.NumField("exit_code"); ok {
					c := int(code)
					w.ExitCode = &c
				}
				if v := ev.StrField("exit_signal"); v != "" {
					w.Signal = v
				}
				w.IdleSince = ""
			} else {
				w.IdleSince = ev.Ts
			}
		case KindKilled:
			reason := ev.StrField("reason")
			if reason == "crash" {
				w.Lifecycle = WorkerLifecycleCrashed
			} else {
				w.Lifecycle = WorkerLifecycleKilled
			}
			w.Terminal = true
			w.Activity = WorkerActivityIdle
			w.ActiveTurnID = ""
			w.ActiveTurnStartedAt = ""
			w.IdleSince = ""
			if reason != "" {
				w.Reason = reason
			}
			if v := ev.StrField("signal"); v != "" {
				w.Signal = v
			}
		}
	}

	// 第二遍：只从持久事件算 pendingMessageCount。
	for _, w := range acc {
		if w.Terminal {
			w.PendingMessageCount = 0
			continue
		}
		pending := 0
		for _, ev := range events {
			if ev.Seq <= w.consumedInputSeq {
				continue
			}
			if MatchesInboxPolicy(ev, w.WorkerID, w.InboxPolicy) {
				pending++
			}
		}
		w.PendingMessageCount = pending
	}

	workers := make([]WorkerState, 0, len(acc))
	for _, w := range acc {
		state := w.WorkerState
		if channel != nil {
			ref := *channel
			state.Channel = &ref
		}
		workers = append(workers, state)
	}
	sort.Slice(workers, func(i, j int) bool { return workers[i].WorkerID < workers[j].WorkerID })
	return WorkerRegistry{Workers: workers}
}
