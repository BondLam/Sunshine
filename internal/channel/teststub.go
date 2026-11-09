package channel

import (
	"context"
	"fmt"
	"sync"
	"time"
)

// StubRuntime 是 WorkerRuntime 的测试替身：记录全部调用，
// 默认成功拉起（返回自增 pid 与当前时间），可注入失败。
// 供本包测试与 C/D/E 组联调用；不要在生产路径使用。
type StubRuntime struct {
	mu sync.Mutex

	// Starts 是收到的全部 Start 调用（按调用顺序）。
	Starts []WorkerSpec
	// Interrupts 是收到的全部 Interrupt 调用（workerID 列表）。
	Interrupts []string
	// Stops 是收到的全部 Stop 调用（workerID 列表）。
	Stops []string

	// NextPID 是下一次 Start 返回的 pid 起始值；为 0 时默认 1000。
	NextPID int
	// FailStart 非空时 Start 返回该错误。
	FailStart error
	// FailInterrupt 非空时 Interrupt 返回该错误。
	FailInterrupt error
	// FailStop 非空时 Stop 返回该错误。
	FailStop error

	// StartFunc 非空时 Start 改调它（完全自定义行为）。
	StartFunc func(ctx context.Context, spec WorkerSpec) (WorkerStart, error)
	// InterruptFunc 非空时 Interrupt 改调它。
	InterruptFunc func(ctx context.Context, workerID string) error
	// StopFunc 非空时 Stop 改调它。
	StopFunc func(ctx context.Context, workerID string) error
}

// NewStubRuntime 建一个默认成功的 StubRuntime。
func NewStubRuntime() *StubRuntime {
	return &StubRuntime{NextPID: 1000}
}

// Start 记录调用并返回默认成功的启动结果。
func (s *StubRuntime) Start(ctx context.Context, spec WorkerSpec) (WorkerStart, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.Starts = append(s.Starts, spec)
	if s.StartFunc != nil {
		return s.StartFunc(ctx, spec)
	}
	if s.FailStart != nil {
		return WorkerStart{}, s.FailStart
	}
	pid := s.NextPID
	if pid == 0 {
		pid = 1000
	}
	s.NextPID = pid + 1
	provider := spec.Provider
	if provider == "" {
		provider = "claude"
	}
	return WorkerStart{
		WorkerID:  spec.WorkerID,
		Provider:  provider,
		PID:       pid,
		StartedAt: time.Now().UTC().Format(time.RFC3339Nano),
	}, nil
}

// Interrupt 记录调用并返回注入的错误（默认成功）。
func (s *StubRuntime) Interrupt(ctx context.Context, workerID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.Interrupts = append(s.Interrupts, workerID)
	if s.InterruptFunc != nil {
		return s.InterruptFunc(ctx, workerID)
	}
	return s.FailInterrupt
}

// Stop 记录调用并返回注入的错误（默认成功）。
func (s *StubRuntime) Stop(ctx context.Context, workerID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.Stops = append(s.Stops, workerID)
	if s.StopFunc != nil {
		return s.StopFunc(ctx, workerID)
	}
	return s.FailStop
}

// StartCount 返回 Start 被调的次数。
func (s *StubRuntime) StartCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.Starts)
}

// InterruptCount 返回 Interrupt 被调的次数。
func (s *StubRuntime) InterruptCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.Interrupts)
}

// LastStart 返回最近一次 Start 的 spec；从未调用时返回错误。
func (s *StubRuntime) LastStart() (WorkerSpec, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.Starts) == 0 {
		return WorkerSpec{}, fmt.Errorf("StubRuntime：尚未调用 Start")
	}
	return s.Starts[len(s.Starts)-1], nil
}
