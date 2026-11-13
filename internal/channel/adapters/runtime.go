// WorkerRuntime 的进程实现：拉起 claude / codex 子进程，
// 把 stdout 逐行交给适配器解析、把事件经 EventSink 发出，
// 并提供 Send / Interrupt / Stop 与 Wait。
package adapters

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/linrong007/Sunshine/internal/channel/store"
)

// ShutdownGrace 是关停宽限：先关 stdin，等 3 秒；
// 仍活着则 SIGTERM，再等 3 秒；仍活着则 SIGKILL（与上游一致）。
const ShutdownGrace = 3 * time.Second

// readyWaitTimeout 是 Send 等适配器就绪（如 codex 握手）的上限（与上游一致）。
const readyWaitTimeout = 60 * time.Second

// StartOptions 是 WorkerRuntime.Start 的选项。
type StartOptions struct {
	// Provider 是供应商（claude / codex）。
	Provider Provider
	// Binary 是 worker 二进制路径；为空时用供应商名（PATH 查找）。
	// 测试可用它指向模拟脚本。
	Binary string
	// Channel 是频道名（记入子进程环境变量与日志）。
	Channel string
	// Worker 是 worker 名（记入子进程环境变量与日志）。
	Worker string
	// Model 是模型覆盖。
	Model string
	// Resume 是恢复的会话 id（claude --resume）。
	Resume string
	// Sandbox 是 codex 沙箱模式（默认 workspace-write）。
	Sandbox string
	// SystemPrompt 是组合后的 worker 系统提示词。
	SystemPrompt string
	// SystemPromptFile 是系统提示词文件路径（超长时用）。
	SystemPromptFile string
	// Cwd 是 worker 启动目录。
	Cwd string
	// Env 是追加给子进程的环境变量。
	Env map[string]string
}

// StartResult 是 Start 成功后的结果。
type StartResult struct {
	Provider  Provider
	PID       int
	StartedAt time.Time
}

// EventSink 是适配器事件的接收端：supervisor 实现它，
// 把事件追加进 events.jsonl、把会话 id 持久化到 worker 文件。
type EventSink interface {
	// Emit 追加一个适配器事件（by 由调用方定，通常为 worker 名）。
	Emit(kind store.ChannelEventKind, by string, payload map[string]any) error
	// PersistSessionID 持久化 worker 会话 id（供 --resume 复用）。
	PersistSessionID(id string) error
	// PersistThreadID 持久化 codex 线程 id（与会话 id 相同值）。
	PersistThreadID(id string) error
}

// WorkerRuntime 是 supervisor 驱动 worker 的接口。
// 上游 core 接口只有 Start / Interrupt / Stop 三方法；
// Go 版把子进程封装在 runtime 内（上游 supervisor 直接写 child.stdin），
// 因此补充 Send（投递频道消息）与 Wait（等进程退出）、Ready（就绪判断）
// 三个方法，见 PLAN.md 阶段 6 实施记录 D 组差异。
type WorkerRuntime interface {
	// Start 拉起 worker 子进程并开始解析其 stdout；握手失败时返回错误。
	Start(ctx context.Context, opts StartOptions) (StartResult, error)
	// Send 把一条频道消息投递给 worker（等适配器就绪后写 stdin）。
	Send(ctx context.Context, text string) error
	// Interrupt 中断 worker 当前回合，并发送替换指令。
	Interrupt(ctx context.Context, text string) error
	// Stop 关停 worker：关 stdin → 宽限 → SIGTERM → 宽限 → SIGKILL。
	Stop(ctx context.Context) error
	// Wait 等 worker 进程退出，返回退出错误（正常退出为 nil）。
	Wait() error
	// Ready 判断适配器是否已能接受用户消息。
	Ready() bool
	// ExitState 返回进程退出码与信号；进程未退出时 exited 为 false，
	// 退出码为 -1。supervisor 用它合成兜底终态事件。
	ExitState() (code int, signal string, exited bool)
}

// processRuntime 是 WorkerRuntime 的通用进程实现，
// 由 claude / codex 两个适配器共用。
type processRuntime struct {
	provider Provider
	adapter  Adapter
	sink     EventSink
	log      io.Writer

	mu       sync.Mutex
	cmd      *exec.Cmd
	stdin    io.WriteCloser
	ctx      any
	worker   string
	started  bool
	stopped  bool
	done     chan struct{}
	waitErr  error
	waitOnce sync.Once
}

// NewClaudeWorkerRuntime 建一个 Claude worker runtime。
func NewClaudeWorkerRuntime(sink EventSink, log io.Writer) WorkerRuntime {
	a, _ := GetAdapter(ProviderClaude)
	return &processRuntime{provider: ProviderClaude, adapter: a, sink: sink, log: log}
}

// NewCodexWorkerRuntime 建一个 Codex worker runtime。
func NewCodexWorkerRuntime(sink EventSink, log io.Writer) WorkerRuntime {
	a, _ := GetAdapter(ProviderCodex)
	return &processRuntime{provider: ProviderCodex, adapter: a, sink: sink, log: log}
}

// NewWorkerRuntime 按供应商建一个 worker runtime。
func NewWorkerRuntime(provider Provider, sink EventSink, log io.Writer) (WorkerRuntime, error) {
	a, err := GetAdapter(provider)
	if err != nil {
		return nil, err
	}
	return &processRuntime{provider: provider, adapter: a, sink: sink, log: log}, nil
}

// Start 拉起 worker 子进程：建管道 → 起进程 → 起 stdout 泵 → 做握手。
func (r *processRuntime) Start(ctx context.Context, opts StartOptions) (StartResult, error) {
	r.mu.Lock()
	if r.started {
		r.mu.Unlock()
		return StartResult{}, fmt.Errorf("worker runtime 已启动")
	}
	r.started = true
	r.worker = opts.Worker
	r.mu.Unlock()

	binary := opts.Binary
	if binary == "" {
		binary = string(opts.Provider)
	}
	view := SupervisorView{
		Resume:           opts.Resume,
		Model:            opts.Model,
		SystemPrompt:     opts.SystemPrompt,
		SystemPromptFile: opts.SystemPromptFile,
		Cwd:              opts.Cwd,
		Sandbox:          opts.Sandbox,
	}
	args := r.adapter.BuildArgs(view)
	cmd := exec.Command(binary, args...)
	if opts.Cwd != "" {
		cmd.Dir = opts.Cwd
	}
	env := os.Environ()
	for k, v := range opts.Env {
		env = append(env, k+"="+v)
	}
	// 关掉 worker 内的 hooks，避免 worker 自己的会话再触发注入；
	// 同时把频道身份桥接给子进程（hook 的会话身份解析用）。
	env = append(env,
		"SUNSHINE_HOOKS=0",
		"SUNSHINE_CHANNEL="+opts.Channel,
		"SUNSHINE_CHANNEL_AS="+opts.Worker,
	)
	cmd.Env = env

	stdin, err := cmd.StdinPipe()
	if err != nil {
		return StartResult{}, fmt.Errorf("建 worker stdin 管道失败：%w", err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return StartResult{}, fmt.Errorf("建 worker stdout 管道失败：%w", err)
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return StartResult{}, fmt.Errorf("建 worker stderr 管道失败：%w", err)
	}
	actx := r.adapter.CreateCtx()
	r.mu.Lock()
	r.cmd = cmd
	r.stdin = stdin
	r.ctx = actx
	r.done = make(chan struct{})
	r.mu.Unlock()

	r.logf("[supervisor] 启动 %s：%s %s\n", opts.Provider, binary, strings.Join(args, " "))
	if err := cmd.Start(); err != nil {
		return StartResult{}, fmt.Errorf("拉起 %s worker 失败：%w", opts.Provider, err)
	}
	// stderr 直接进日志文件，不解析。
	go func() {
		_, _ = io.Copy(r.logWriter(), stderr)
	}()
	// stdout 泵：逐行解析，行处理串行进行。
	go r.pumpStdout(stdout)
	// 退出哨兵：进程退出后关闭 done。
	go func() {
		err := cmd.Wait()
		r.waitOnce.Do(func() {
			r.waitErr = err
			close(r.done)
		})
	}()

	// 适配器握手（如 codex 的 initialize + thread/start）。
	// 握手失败时关停进程并报错，supervisor 会据此写 error 事件。
	if err := r.adapter.Handshake(ctx, r, actx, view); err != nil {
		_ = r.Stop(context.Background())
		return StartResult{}, fmt.Errorf("worker 握手失败：%w", err)
	}
	return StartResult{Provider: opts.Provider, PID: cmd.Process.Pid, StartedAt: time.Now()}, nil
}

// WriteLine 实现 StdinWriter：供适配器握手写行。
func (r *processRuntime) WriteLine(line string) error {
	return r.writeStdin(line)
}

// Ready 判断适配器是否已能接受用户消息。
// 注意：claude 的解析上下文就是 nil（无握手状态），不能拿 ctx 是否为 nil
// 当就绪条件；就绪只由适配器自己判断。
func (r *processRuntime) Ready() bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	if !r.started {
		return false
	}
	return r.adapter.IsReady(r.ctx)
}

// Send 等适配器就绪（上限 60 秒），再把消息编码后写进 worker stdin。
func (r *processRuntime) Send(ctx context.Context, text string) error {
	if err := r.waitReady(ctx); err != nil {
		return err
	}
	r.mu.Lock()
	line, err := r.adapter.EncodeUserMessage(text, r.ctx)
	r.mu.Unlock()
	if err != nil {
		return fmt.Errorf("编码 worker 消息失败：%w", err)
	}
	return r.writeStdin(line)
}

// Interrupt 编码中断重定向并写进 worker stdin。
func (r *processRuntime) Interrupt(ctx context.Context, text string) error {
	r.mu.Lock()
	line, err := r.adapter.EncodeInterruptMessage(text, r.ctx)
	r.mu.Unlock()
	if err != nil {
		return fmt.Errorf("编码中断消息失败：%w", err)
	}
	return r.writeStdin(line)
}

// Stop 关停 worker：关 stdin → 宽限 → SIGTERM → 宽限 → SIGKILL。
func (r *processRuntime) Stop(ctx context.Context) error {
	r.mu.Lock()
	if r.stopped {
		r.mu.Unlock()
		return nil
	}
	r.stopped = true
	cmd := r.cmd
	stdin := r.stdin
	r.mu.Unlock()
	if cmd == nil || cmd.Process == nil {
		return nil
	}
	// 1. 关 stdin，让 worker 自然收尾。
	if stdin != nil {
		_ = stdin.Close()
	}
	// 2. 宽限后 SIGTERM。
	if exited := r.waitExit(ShutdownGrace); exited {
		return nil
	}
	_ = signalTerm(cmd.Process)
	// 3. 再宽限后 SIGKILL。
	if exited := r.waitExit(ShutdownGrace); exited {
		return nil
	}
	_ = signalKill(cmd.Process)
	<-r.done
	return nil
}

// Wait 等 worker 进程退出。
func (r *processRuntime) Wait() error {
	r.mu.Lock()
	done := r.done
	r.mu.Unlock()
	if done == nil {
		return fmt.Errorf("worker 尚未启动")
	}
	<-done
	return r.waitErr
}

// ExitState 返回进程退出码与信号。
func (r *processRuntime) ExitState() (code int, signal string, exited bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.cmd == nil || r.cmd.ProcessState == nil {
		return -1, "", false
	}
	st := r.cmd.ProcessState
	code = st.ExitCode()
	if ws, ok := st.Sys().(syscall.WaitStatus); ok && ws.Signaled() {
		signal = ws.Signal().String()
	}
	return code, signal, true
}

// waitReady 等适配器就绪，上限 60 秒（与上游收件箱监听一致）。
func (r *processRuntime) waitReady(ctx context.Context) error {
	deadline := time.Now().Add(readyWaitTimeout)
	for {
		if r.Ready() {
			return nil
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("worker 在 60 秒内未就绪，消息未投递")
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(25 * time.Millisecond):
		}
	}
}

// waitExit 等进程退出，最多 d；返回是否已退出。
func (r *processRuntime) waitExit(d time.Duration) bool {
	r.mu.Lock()
	done := r.done
	r.mu.Unlock()
	if done == nil {
		return true
	}
	select {
	case <-done:
		return true
	case <-time.After(d):
		return false
	}
}

// writeStdin 把一行写进 worker stdin（加锁串行，避免交错）。
func (r *processRuntime) writeStdin(line string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.stdin == nil {
		return fmt.Errorf("worker stdin 未就绪")
	}
	if _, err := io.WriteString(r.stdin, line); err != nil {
		return fmt.Errorf("写 worker stdin 失败：%w", err)
	}
	return nil
}

// pumpStdout 逐行读 stdout，交给适配器解析，再把事件与副作用落实。
func (r *processRuntime) pumpStdout(stdout io.Reader) {
	reader := bufio.NewReader(stdout)
	for {
		line, err := reader.ReadString('\n')
		trimmed := strings.TrimRight(line, "\r\n")
		if strings.TrimSpace(trimmed) != "" {
			r.handleLine(trimmed)
		}
		if err != nil {
			// EOF 或读错：若还有未以换行结尾的残行已在上面处理，退出。
			return
		}
	}
}

// handleLine 处理一行 stdout：解析 → 发事件 → 落副作用。
func (r *processRuntime) handleLine(line string) {
	r.logf("%s\n", line)
	r.mu.Lock()
	actx := r.ctx
	r.mu.Unlock()
	result := r.adapter.ParseLine(line, actx)
	r.applyParseResult(result)
}

// applyParseResult 把解析结果落实：事件经 sink 发出；
// done/error 由 supervisor 的 sink 侧接管回合收尾；
// 会话/线程 id 持久化；reply 写回 stdin。
func (r *processRuntime) applyParseResult(result ParseResult) {
	for _, ev := range result.Events {
		if r.sink == nil {
			continue
		}
		if err := r.sink.Emit(ev.Kind, r.worker, ev.Payload); err != nil {
			r.logf("[supervisor] 事件追加失败：%v\n", err)
		}
	}
	if result.Side == nil {
		return
	}
	if r.sink != nil {
		if result.Side.PersistSessionID != "" {
			if err := r.sink.PersistSessionID(result.Side.PersistSessionID); err != nil {
				r.logf("[supervisor] 会话 id 持久化失败：%v\n", err)
			}
		}
		if result.Side.PersistThreadID != "" {
			if err := r.sink.PersistThreadID(result.Side.PersistThreadID); err != nil {
				r.logf("[supervisor] 线程 id 持久化失败：%v\n", err)
			}
		}
	}
	for _, reply := range result.Side.Reply {
		if err := r.writeStdin(reply); err != nil {
			r.logf("[supervisor] 写回 worker stdin 失败：%v\n", err)
		}
	}
}

func (r *processRuntime) logf(format string, args ...any) {
	w := r.logWriter()
	if w == nil {
		return
	}
	_, _ = fmt.Fprintf(w, format, args...)
}

func (r *processRuntime) logWriter() io.Writer {
	if r.log == nil {
		return io.Discard
	}
	return r.log
}
