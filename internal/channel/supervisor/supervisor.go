// Package supervisor 实现 Sunshine channel 的 supervisor 进程：
// 拥有一个 worker（claude 或 codex），桥接 worker 与频道事件日志。
//
// 运行方式：`sunshine channel __supervisor <channel> <worker> <config-path>`
// （C 组在 CLI 预留入口，本包提供 RunSupervisor / RunSupervisorFromConfigFile）。
//
// 三个并发循环（与上游 supervisor.ts 一致）：
//  1. stdout 泵——worker stdout 经适配器解析后追加事件（在 adapters.runtime 内）；
//  2. 收件箱监听——读 events.jsonl 里 to=<worker> 的消息，经适配器编码后写 worker stdin；
//  3. 信号处理——SIGTERM → 关 worker stdin → 3 秒 → SIGTERM → 3 秒 → SIGKILL，
//     再写 killed 事件后退出。
//
// 存储直接用 A 组的 internal/channel/store（AppendEvent / ReadChannelEvents /
// ReduceWorkerRegistry / MatchesInboxPolicy / WorkerFile），
// 不另起一份存储。
package supervisor

import (
	"github.com/linrong007/Sunshine/internal/channel/store"
)

// InboxPolicy 是 worker 收件箱策略（复用 store 的定义）。
type InboxPolicy = store.InboxPolicy

// DefaultInboxPolicy 是默认策略 explicitOnly。
const DefaultInboxPolicy = store.DefaultInboxPolicy

// appendEvent 是 supervisor 追加事件的薄封装：
// kind/by/extra，来源记为 worker。
func appendEvent(ref store.ChannelRef, kind store.ChannelEventKind, by string, extra map[string]any) (store.ChannelEvent, error) {
	return store.AppendEvent(ref, store.AppendableEvent{
		Kind:   kind,
		By:     by,
		Origin: store.OriginWorker,
		Extra:  extra,
	})
}

// appendSupervisorEvent 是 supervisor 自身名义追加的事件（by 为 supervisor:<worker>）。
func appendSupervisorEvent(ref store.ChannelRef, kind store.ChannelEventKind, worker string, extra map[string]any) (store.ChannelEvent, error) {
	return appendEvent(ref, kind, "supervisor:"+worker, extra)
}
