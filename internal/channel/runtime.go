package channel

import (
	"context"
	"time"
)

// WorkerRuntime 是 provider 注入的 worker 运行时契约。
//
// Core（本包）拥有事件写盘、注册表投影与生命周期契约，绝不直接
// import CLI 的 provider 适配器；适配器实现本接口并注入
// SpawnWorker / InterruptWorker。
//
// 注意与上游的差异（存疑点 11，已解决）：上游 core 的
// WorkerRuntime 接口没有 start 方法（只有 interrupt/stop），但
// spawnWorker 调用的 runtime.start 在类型层面未声明；Go 版接口
// 明确包含 Start / Interrupt / Stop 三个方法。
type WorkerRuntime interface {
	// Start 按 WorkerSpec 拉起一个 worker，返回启动结果。
	Start(ctx context.Context, spec WorkerSpec) (WorkerStart, error)
	// Interrupt 尝试中断指定 worker 的当前回合。
	Interrupt(ctx context.Context, workerID string) error
	// Stop 停止指定 worker。
	Stop(ctx context.Context, workerID string) error
}

// WorkerSpec 是拉起一个 worker 的输入。
type WorkerSpec struct {
	// Channel 是 worker 归属的 channel。
	Channel ChannelRef
	// WorkerID 是 worker 名。
	WorkerID string
	// Provider 是 provider：claude 或 codex。
	Provider string
	// Agent 是 agent 名（如 implement、check、research），可为空。
	Agent string
	// Model 是模型名，可为空（用 provider 默认）。
	Model string
	// CWD 是 worker 的工作目录。
	CWD string
	// Resume 是续跑的会话 id，可为空。
	Resume string
	// Sandbox 是沙箱模式（仅 codex），可为空。
	Sandbox string
	// SystemPrompt 是注入 worker 的系统提示词。
	SystemPrompt string
	// ContextFiles 是注入系统提示词的上下文文件（绝对路径）。
	ContextFiles []string
	// InboxPolicy 是收件箱策略，空为默认 explicitOnly。
	InboxPolicy InboxPolicy
	// Env 是透传给 worker 进程的环境变量。
	Env map[string]string
}

// WorkerStart 是 runtime 拉起 worker 后的返回。
type WorkerStart struct {
	// WorkerID 是 worker 名。
	WorkerID string
	// Provider 是实际使用的 provider。
	Provider string
	// PID 是 worker 进程 pid（未知时为 0）。
	PID int
	// StartedAt 是启动时间（RFC3339）。
	StartedAt string
}

// NowRFC3339 返回当前 UTC 时间的 RFC3339 字符串。
func NowRFC3339() string {
	return time.Now().UTC().Format(time.RFC3339Nano)
}

// ValidProvider 判断 provider 是否合法（claude 或 codex）。
func ValidProvider(provider string) bool {
	return provider == "claude" || provider == "codex"
}
