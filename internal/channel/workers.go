package channel

import (
	"os"
	"strconv"
	"strings"

	"github.com/BondLam/Sunshine/internal/channel/store"
)

// ListWorkers 投影某 channel 的持久 worker 注册表。
// 默认只列非终态 worker；IncludeTerminal 为 true 时包含终态。
// 本函数是 CLI list / status、daemon 运行时卡片与测试的事实来源，
// 不要绕过它直接解析事件日志。
func ListWorkers(opts ListWorkersOptions) ([]WorkerState, error) {
	ref, err := resolveRef(opts.ChannelAddress, false)
	if err != nil {
		return nil, err
	}
	registry, _, err := readRegistry(ref)
	if err != nil {
		return nil, err
	}
	return filterWorkers(registry, opts.IncludeTerminal), nil
}

// ProbeWorkerRuntime 做本机运行时观测：读 <worker>.pid /
// <worker>.worker-pid 文件并检查 OS 存活。
//
// 这不是持久 channel 真相——ReduceWorkerRegistry 绝不读 pid 文件；
// 本函数只在拥有 supervisor 文件的本机上有效。
func ProbeWorkerRuntime(addr ChannelAddress) ([]WorkerRuntimeObservation, error) {
	ref, err := resolveRef(addr, false)
	if err != nil {
		return nil, err
	}
	registry, _, err := readRegistry(ref)
	if err != nil {
		return nil, err
	}
	observedAt := NowRFC3339()
	observations := make([]WorkerRuntimeObservation, 0, len(registry.Workers))
	for _, w := range registry.Workers {
		obs := WorkerRuntimeObservation{
			WorkerID:   w.WorkerID,
			ObservedAt: observedAt,
			Source:     "local-pid-files",
		}
		if pid, ok := readPIDFile(ref, w.WorkerID, "pid"); ok {
			p := pid
			obs.PID = &p
			alive := pidAlive(pid)
			obs.SupervisorAlive = &alive
		}
		if pid, ok := readPIDFile(ref, w.WorkerID, "worker-pid"); ok {
			p := pid
			obs.WorkerPID = &p
			alive := pidAlive(pid)
			obs.WorkerAlive = &alive
		}
		observations = append(observations, obs)
	}
	return observations, nil
}

// ReconcileWorkerLiveness 把持久 worker 状态与本机 pid 文件对账：
// 报告观测结果与提议追加的终态事件。默认不写盘；
// AppendTerminalEvents 为 true 时才把提议的事件真正追加落盘。
// 只在拥有 supervisor 文件的本机上有效。
func ReconcileWorkerLiveness(opts ReconcileWorkerLivenessOptions) (ReconcileWorkerLivenessResult, error) {
	ref, err := resolveRef(opts.ChannelAddress, false)
	if err != nil {
		return ReconcileWorkerLivenessResult{}, err
	}
	registry, _, err := readRegistry(ref)
	if err != nil {
		return ReconcileWorkerLivenessResult{}, err
	}
	observations, err := ProbeWorkerRuntime(opts.ChannelAddress)
	if err != nil {
		return ReconcileWorkerLivenessResult{}, err
	}
	obsByID := map[string]WorkerRuntimeObservation{}
	for _, obs := range observations {
		obsByID[obs.WorkerID] = obs
	}
	now := NowRFC3339()
	proposed := []ChannelEvent{}
	for _, w := range registry.Workers {
		if w.Terminal {
			continue
		}
		obs, ok := obsByID[w.WorkerID]
		if !ok {
			continue
		}
		// 非终态 worker 的 supervisor pid 已不存在，说明它崩溃了，
		// 但从未写下终态事件。
		if obs.SupervisorAlive != nil && !*obs.SupervisorAlive {
			proposed = append(proposed, ChannelEvent{
				Seq:         0,
				TS:          now,
				Kind:        EventKindError,
				By:          "supervisor:" + w.WorkerID,
				Worker:      w.WorkerID,
				Message:     "supervisor 进程已不存在（对账发现）",
				Synthesized: true,
			})
		}
	}
	result := ReconcileWorkerLivenessResult{
		Observations:   observations,
		ProposedEvents: proposed,
		Appended:       []ChannelEvent{},
	}
	if opts.AppendTerminalEvents {
		for _, ev := range proposed {
			partial := newAppendable(EventKindError, ev.By, "", nil)
			partial = withExtra(partial, map[string]any{
				"worker":      ev.Worker,
				"message":     ev.Message,
				"synthesized": true,
			})
			appended, err := store.AppendEvent(ref, partial)
			if err != nil {
				return result, err
			}
			converted, err := fromStoreEvent(appended)
			if err != nil {
				return result, err
			}
			result.Appended = append(result.Appended, converted)
		}
	}
	return result, nil
}

// readPIDFile 读 <worker>.<suffix> 文件里的 pid；文件不存在或
// 内容非法时返回 false。
func readPIDFile(ref ChannelRef, workerID, suffix string) (int, bool) {
	path, err := store.WorkerFile(ref.Name, workerID, suffix, ref.Project)
	if err != nil {
		return 0, false
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return 0, false
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(data)))
	if err != nil || pid <= 0 {
		return 0, false
	}
	return pid, true
}
