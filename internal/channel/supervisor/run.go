package supervisor

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/linrong1982/Sunshine/internal/channel/adapters"
	"github.com/linrong1982/Sunshine/internal/channel/store"
)

// SupervisorConfig 是 supervisor 子进程的配置（JSON 文件，
// 由 spawn 方经 WriteSupervisorConfig 写出，与上游 supervisor.ts
// 的 SupervisorConfig 字段一一对应）。
type SupervisorConfig struct {
	// Provider 是供应商（claude / codex）。
	Provider string `json:"provider"`
	// Cwd 是 worker 启动目录。
	Cwd string `json:"cwd"`
	// SystemPrompt 是组合后的 worker 系统提示词：
	// channel 协议前缀 + agent 正文 + 注入的上下文文件。
	// 经 Claude --append-system-prompt(-file) 或 Codex
	// developerInstructions 注入。supervisor 也把它持久化到
	// <worker>.system-prompt.md。没有“初始用户提示词”——
	// worker 拉起后保持空闲，直到第一条 send --to 到达。
	SystemPrompt string `json:"systemPrompt"`
	// Env 是追加给 worker 的环境变量。
	Env map[string]string `json:"env,omitempty"`
	// Model 是模型覆盖。
	Model string `json:"model,omitempty"`
	// Resume 是恢复的会话/线程 id。
	Resume string `json:"resume,omitempty"`
	// Sandbox 是 codex 专用的沙箱模式（默认 workspace-write）。
	Sandbox string `json:"sandbox,omitempty"`
	// TimeoutMs 是 lifetime 上限（毫秒，防僵尸自动杀，0 禁用）。
	TimeoutMs int64 `json:"timeoutMs,omitempty"`
	// WarnBeforeMs 是超时前多少毫秒发 supervisor_warning 预警；
	// nil 表示用默认 5 分钟，0 表示禁用，>0 为显式提前量。
	WarnBeforeMs *int64 `json:"warnBeforeMs,omitempty"`
	// IdleTimeoutMs 是 OOM guard 空闲清理 TTL（毫秒，<=0 禁用）。
	IdleTimeoutMs int64 `json:"idleTimeoutMs,omitempty"`
	// SpawnedBy 是 spawned 事件的作者（默认 main）。
	SpawnedBy string `json:"spawnedBy,omitempty"`
	// Agent 是加载的 agent 定义名（记在 spawned 事件上）。
	Agent string `json:"agent,omitempty"`
	// ContextFiles 是经 --file / --jsonl 注入的相对路径（记在 spawned 上）。
	ContextFiles []string `json:"contextFiles,omitempty"`
	// ContextManifests 是处理过的 --jsonl 清单相对路径
	//（即使清单为空也记，记在 spawned 上）。
	ContextManifests []string `json:"contextManifests,omitempty"`
	// InboxPolicy 是 worker 收件箱策略（默认 explicitOnly）。
	InboxPolicy string `json:"inboxPolicy,omitempty"`
}

// RuntimeFactory 按供应商建 worker runtime；测试用它注入 StubWorkerRuntime。
type RuntimeFactory func(provider adapters.Provider, sink adapters.EventSink) (adapters.WorkerRuntime, error)

// DefaultRuntimeFactory 是默认工厂：建真实的 claude / codex 进程 runtime。
func DefaultRuntimeFactory(log io.Writer) RuntimeFactory {
	return func(provider adapters.Provider, sink adapters.EventSink) (adapters.WorkerRuntime, error) {
		return adapters.NewWorkerRuntime(provider, sink, log)
	}
}

// RunOption 是 RunSupervisor 的选项。
type RunOption func(*runOptions)

type runOptions struct {
	factory RuntimeFactory
	log     io.Writer
}

// WithRuntimeFactory 注入 runtime 工厂（测试用 stub）。
func WithRuntimeFactory(f RuntimeFactory) RunOption {
	return func(o *runOptions) { o.factory = f }
}

// WithLogWriter 指定 supervisor 日志输出（默认写 <worker>.log 文件）。
func WithLogWriter(w io.Writer) RunOption {
	return func(o *runOptions) { o.log = w }
}

// RunSupervisor 运行一个 supervisor：拥有一个 worker，桥接 worker 与频道事件。
// ctx 取消时关停 worker；进程入口 RunSupervisorFromConfigFile 会把
// SIGTERM/SIGINT/SIGHUP 转成 ctx 取消（直接调本函数时由调用方负责）。
// ref 定位频道；worker 是 worker 名；cfg 是 supervisor 配置。
func RunSupervisor(ctx context.Context, ref store.ChannelRef, worker string, cfg SupervisorConfig, opts ...RunOption) error {
	o := &runOptions{}
	for _, opt := range opts {
		opt(o)
	}
	if err := store.AssertSafeName(worker, "worker"); err != nil {
		return err
	}
	provider, err := adapters.ParseProvider(cfg.Provider)
	if err != nil {
		return err
	}
	inboxPolicy := store.DefaultInboxPolicy
	if cfg.InboxPolicy != "" {
		if p, err := store.ParseInboxPolicy(cfg.InboxPolicy); err != nil {
			return err
		} else {
			inboxPolicy = p
		}
	}
	// supervisor 自身的 pid 文件：`channel kill` 靠它找到我们。
	pidPath, err := store.WorkerFile(ref.Name, worker, "pid", ref.Project)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(pidPath), 0o700); err != nil {
		return fmt.Errorf("创建 worker 目录失败：%w", err)
	}
	if err := os.WriteFile(pidPath, []byte(fmt.Sprintf("%d", os.Getpid())), 0o644); err != nil {
		return fmt.Errorf("写 supervisor pid 文件失败：%w", err)
	}
	// 日志文件：worker 的 stderr 与 stdout 原行都进这里（取证用）。
	logPath, err := store.WorkerFile(ref.Name, worker, "log", ref.Project)
	if err != nil {
		return err
	}
	var logWriter io.Writer = o.log
	if logWriter == nil {
		f, err := os.OpenFile(logPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
		if err != nil {
			return fmt.Errorf("打开 supervisor 日志失败：%w", err)
		}
		logWriter = f
		defer f.Close()
	}
	logf := func(format string, args ...any) {
		_, _ = fmt.Fprintf(logWriter, format, args...)
	}

	// 超大系统提示词走文件，避免 argv 长度上限。
	// 8000 字以内仍走 argv 内联（照顾尚无 --append-system-prompt-file
	// 的旧版 Claude Code 安装）。
	systemPromptFile := ""
	if adapters.ShouldUseSystemPromptFile(cfg.SystemPrompt) {
		p, err := store.WorkerFile(ref.Name, worker, "system-prompt.md", ref.Project)
		if err != nil {
			return err
		}
		if err := os.WriteFile(p, []byte(cfg.SystemPrompt), 0o644); err != nil {
			return fmt.Errorf("写系统提示词文件失败：%w", err)
		}
		systemPromptFile = p
	}

	// 关停控制器：先建，再拉 worker（信号随时可能到）。
	var runtime adapters.WorkerRuntime
	shutdown := NewShutdownController(ref, worker, logWriter, func() error {
		if runtime == nil {
			return nil
		}
		return runtime.Stop(context.Background())
	}, cfg.TimeoutMs, cfg.IdleTimeoutMs)

	// 回合跟踪器：idle 定时器的暂停/重置挂在它上面。
	var idleHandle IdleTimerHandle
	tracker := NewTurnTracker(TurnTrackerHooks{
		OnIdleExit:  func() { idleHandle.Pause() },
		OnIdleEnter: func() { idleHandle.Reset() },
	})

	// sink：把适配器事件追加进 events.jsonl，处理终态收尾与会话持久化。
	sink := &supervisorSink{
		ref: ref, worker: worker, shutdown: shutdown, tracker: tracker,
		log: logWriter,
	}

	factory := o.factory
	if factory == nil {
		factory = DefaultRuntimeFactory(logWriter)
	}
	runtime, err = factory(provider, sink)
	if err != nil {
		return fmt.Errorf("建 worker runtime 失败：%w", err)
	}

	startOpts := adapters.StartOptions{
		Provider:         provider,
		Channel:          ref.Name,
		Worker:           worker,
		Model:            cfg.Model,
		Resume:           cfg.Resume,
		Sandbox:          cfg.Sandbox,
		SystemPrompt:     cfg.SystemPrompt,
		SystemPromptFile: systemPromptFile,
		Cwd:              cfg.Cwd,
		Env:              cfg.Env,
	}
	startedAt, startErr := runtime.Start(ctx, startOpts)
	if startErr != nil {
		// 拉起/握手失败：写一条 error 事件，不写误导性的 spawned，
		// 清理后退出（不在这里等一个不会来的 exit 事件）。
		_, _ = appendSupervisorEvent(ref, store.KindError, worker, map[string]any{
			"message":  fmt.Sprintf("worker 拉起失败：%v", startErr),
			"provider": string(provider),
		})
		_ = cleanupSupervisorFiles(ref, worker)
		return fmt.Errorf("拉起 worker 失败：%w", startErr)
	}
	_ = startedAt
	childExited := func() bool {
		_, _, exited := runtime.ExitState()
		return exited
	}

	// worker pid 文件（stub 的 pid 为 0 时不写）。
	if startedAt.PID > 0 {
		if p, err := store.WorkerFile(ref.Name, worker, "worker-pid", ref.Project); err == nil {
			_ = os.WriteFile(p, []byte(fmt.Sprintf("%d", startedAt.PID)), 0o644)
		}
	}

	// spawned 事件：worker 已切实拉起才写。
	spawnedBy := cfg.SpawnedBy
	if spawnedBy == "" {
		spawnedBy = "main"
	}
	spawnedExtra := map[string]any{
		"as":          worker,
		"provider":    string(provider),
		"inboxPolicy": string(inboxPolicy),
	}
	if startedAt.PID > 0 {
		spawnedExtra["pid"] = startedAt.PID
	}
	if cfg.Agent != "" {
		spawnedExtra["agent"] = cfg.Agent
	}
	if len(cfg.ContextFiles) > 0 {
		spawnedExtra["files"] = cfg.ContextFiles
	}
	if len(cfg.ContextManifests) > 0 {
		spawnedExtra["manifests"] = cfg.ContextManifests
	}
	if _, err := appendEvent(ref, store.KindSpawned, spawnedBy, spawnedExtra); err != nil {
		_ = runtime.Stop(context.Background())
		_ = cleanupSupervisorFiles(ref, worker)
		return fmt.Errorf("追加 spawned 事件失败：%w", err)
	}

	// OOM guard 空闲定时器：spawned 落盘后才启动。
	idleHandle = ScheduleIdleTimer(cfg.IdleTimeoutMs, shutdown, childExited, logWriter)
	defer idleHandle.Cancel()

	// lifetime 超时 guard（防僵尸）与超时前预警。
	var cancelWarning func()
	var timeoutTimer *time.Timer
	if cfg.TimeoutMs > 0 {
		timeoutMs := cfg.TimeoutMs
		timeoutTimer = time.AfterFunc(time.Duration(timeoutMs)*time.Millisecond, func() {
			logf("[supervisor] 超时 %dms 已到，正在杀掉 worker\n", timeoutMs)
			_ = shutdown.Request(ReasonTimeout)
		})
		warnBefore := DefaultWarnBeforeMs
		if cfg.WarnBeforeMs != nil {
			// 显式设置：0 表示禁用。
			warnBefore = *cfg.WarnBeforeMs
		}
		if warnBefore > 0 {
			cancelWarning = ScheduleTimeoutWarning(ref, worker, timeoutMs, warnBefore, shutdown, childExited, logWriter)
		}
	}
	if timeoutTimer != nil {
		defer timeoutTimer.Stop()
	}
	if cancelWarning != nil {
		defer cancelWarning()
	}

	// 收件箱监听：先于握手完成启动也无妨，runtime.Send 内部会等就绪。
	inboxCtx, cancelInbox := context.WithCancel(context.Background())
	defer cancelInbox()
	go runInboxWatcher(inboxCtx, ref, worker, inboxPolicy, runtime, tracker, shutdown)

	// 等 worker 退出或外部取消。
	waitDone := make(chan error, 1)
	go func() {
		waitDone <- runtime.Wait()
	}()
	select {
	case <-ctx.Done():
		// 外部信号：读 shutdown-reason sidecar（idle 清理写它），
		// 否则按显式杀掉处理；再走统一关停漏斗。
		reason := readExternalShutdownReason(ref, worker)
		_ = shutdown.Request(reason)
		<-waitDone
	case err := <-waitDone:
		_ = err
	}
	// 子进程已退出：合成兜底终态事件（如适配器没发），
	// 等进行中的 killed 追加落盘，再清理临时文件。
	code, signal, _ := runtime.ExitState()
	_ = shutdown.FinalizeOnExit(code, signal)
	_ = cleanupSupervisorFiles(ref, worker)
	return nil
}

// RunSupervisorFromConfigFile 是 `sunshine channel __supervisor` 的入口：
// 读 JSON 配置文件后运行 supervisor。项目桶按环境变量解析
// （SUNSHINE_CHANNEL_PROJECT，spawn 方已设好），与 spawn 方落在同一桶。
func RunSupervisorFromConfigFile(ctx context.Context, channel, worker, configPath string, opts ...RunOption) error {
	data, err := os.ReadFile(configPath)
	if err != nil {
		return fmt.Errorf("读 supervisor 配置失败：%w", err)
	}
	var cfg SupervisorConfig
	if err := json.Unmarshal(data, &cfg); err != nil {
		return fmt.Errorf("解析 supervisor 配置失败：%w", err)
	}
	project := store.CurrentProjectKey()
	scope := store.ScopeProject
	if project == store.GlobalProjectKey {
		scope = store.ScopeGlobal
	}
	ref := store.ChannelRef{Name: channel, Scope: scope, Project: project}
	// 进程入口自己接管 SIGTERM/SIGINT/SIGHUP（与上游 supervisor.ts 一致），
	// 转成 ctx 取消，走优雅关停（killed 事件 + 临时文件清理）。
	// CLI 入口目前传 context.Background()，不在进程里接信号的话，
	// SIGTERM 会直接杀掉进程、来不及写 killed 事件（实测已验证）。
	sigCtx, stop := signal.NotifyContext(ctx, syscall.SIGTERM, syscall.SIGINT, syscall.SIGHUP)
	defer stop()
	return RunSupervisor(sigCtx, ref, worker, cfg, opts...)
}

// WriteSupervisorConfig 在派生 supervisor 子进程前写出配置 JSON 文件，
// 返回配置文件路径。
func WriteSupervisorConfig(channel, worker string, cfg SupervisorConfig, project string) (string, error) {
	p, err := store.WorkerFile(channel, worker, "config", project)
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(filepath.Dir(p), 0o700); err != nil {
		return "", fmt.Errorf("创建 worker 目录失败：%w", err)
	}
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return "", fmt.Errorf("编码 supervisor 配置失败：%w", err)
	}
	if err := os.WriteFile(p, append(data, '\n'), 0o644); err != nil {
		return "", fmt.Errorf("写 supervisor 配置失败：%w", err)
	}
	return p, nil
}

// supervisorSink 是 supervisor 侧的 adapters.EventSink 实现：
// 事件追加进 events.jsonl；done/error 触发回合收尾与终态标记；
// 会话/线程 id 写进 worker 文件。
type supervisorSink struct {
	ref      store.ChannelRef
	worker   string
	shutdown *ShutdownController
	tracker  *TurnTracker
	log      io.Writer
}

func (s *supervisorSink) Emit(kind store.ChannelEventKind, by string, payload map[string]any) error {
	// 先同步抢占终态槽，再追加事件，避免子进程退出处理
	// 在终态事件落盘前合成重复的兜底事件。
	if kind == store.KindDone || kind == store.KindError {
		s.shutdown.MarkTerminalEmitted()
	}
	if _, err := appendEvent(s.ref, kind, by, payload); err != nil {
		return err
	}
	if kind == store.KindDone || kind == store.KindError {
		if turn, ok := s.tracker.Finish(); ok {
			outcome := TurnOutcomeDone
			if kind == store.KindError {
				outcome = TurnOutcomeError
			}
			_, _ = appendEvent(s.ref, store.KindTurnFinished, s.worker, map[string]any{
				"worker":   s.worker,
				"inputSeq": turn.InputSeq,
				"turnId":   turn.TurnID,
				"outcome":  string(outcome),
			})
		}
	}
	return nil
}

func (s *supervisorSink) PersistSessionID(id string) error {
	return WriteSessionID(s.ref, s.worker, id)
}

func (s *supervisorSink) PersistThreadID(id string) error {
	return WriteThreadID(s.ref, s.worker, id)
}

// cleanupSupervisorFiles 删除临时运行时文件，保留 log（取证）、
// session-id / thread-id（供将来 resume）与 inbox-cursor
// （重派生同一 worker 时不重放已投递消息）。
// system-prompt.md 即使本次内联提示词也删（重派生时按需重写，
// 避免残留过期的大上下文）。
func cleanupSupervisorFiles(ref store.ChannelRef, worker string) error {
	for _, suffix := range []string{"pid", "worker-pid", "config", "system-prompt.md", "spawnlock", "shutdown-reason", "reservation"} {
		p, err := store.WorkerFile(ref.Name, worker, suffix, ref.Project)
		if err != nil {
			continue
		}
		_ = os.Remove(p)
	}
	return nil
}

// readExternalShutdownReason 读 kill 方写的一次性关停原因 sidecar：
// guard 的 idle 清理会先写 "idle-timeout" 再发 SIGTERM；
// 没有 sidecar 时普通外部 SIGTERM 就是显式杀掉。
func readExternalShutdownReason(ref store.ChannelRef, worker string) ShutdownReason {
	p, err := store.WorkerFile(ref.Name, worker, "shutdown-reason", ref.Project)
	if err != nil {
		return ReasonExplicitKill
	}
	data, err := os.ReadFile(p)
	if err != nil {
		return ReasonExplicitKill
	}
	_ = os.Remove(p)
	if strings.TrimSpace(string(data)) == "idle-timeout" {
		return ReasonIdleTimeout
	}
	return ReasonExplicitKill
}
