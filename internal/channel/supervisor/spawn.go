package supervisor

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/BondLam/Sunshine/internal/channel/adapters"
	"github.com/BondLam/Sunshine/internal/channel/store"
)

// SpawnOptions 是 SpawnWorker 的选项（对应 `channel spawn` 的 flags）。
type SpawnOptions struct {
	// Provider 是供应商（claude / codex）；空时由 agent 定义补。
	Provider string
	// As 是 worker 名；空时由 agent 名补。
	As string
	// Agent 是加载的 agent 定义名（.sunshine/agents/<name>.md）。
	Agent string
	// Cwd 是 worker 启动目录与上下文解析目录（默认当前工作目录）。
	Cwd string
	// Model 是模型覆盖；空时由 agent 定义补。
	Model string
	// Resume 是恢复的会话 id（--resume）。
	Resume string
	// Sandbox 是 codex 沙箱模式（默认 workspace-write）。
	Sandbox string
	// TimeoutMs 是 lifetime 上限（毫秒，0 禁用）。
	TimeoutMs int64
	// WarnBeforeMs 是超时前预警提前量（毫秒）；
	// nil 表示默认 5 分钟，指向 0 表示禁用。
	WarnBeforeMs *int64
	// Files 是 --file 列表（可 glob、可重复）。
	Files []string
	// JSONLs 是 --jsonl 清单列表。
	JSONLs []string
	// Scope 是 channel 作用域（project / global，空时自动解析）。
	Scope string
	// By 是 spawned 事件的作者（默认 main，或 SUNSHINE_CHANNEL_AS 环境变量）。
	By string
	// InboxPolicy 是收件箱策略（默认 explicitOnly）。
	InboxPolicy string
	// IdleTimeoutMs 是本次派生的空闲清理 TTL（毫秒）；
	// nil 表示按守卫优先级解析（flag > 环境变量 > 配置 > 默认）。
	IdleTimeoutMs *int64
	// MaxLiveWorkers 是本次派生的存活预算；nil 表示按守卫优先级解析。
	MaxLiveWorkers *int
	// SupervisorBinary 是 supervisor 子进程的二进制路径；
	// 空时用当前进程的可执行文件（即 sunshine 自身）。
	// 测试可指向模拟脚本。
	SupervisorBinary string
}

// ResolvedSpawn 是 ResolveSpawn 的结果。
type ResolvedSpawn struct {
	Provider         string
	As               string
	SystemPrompt     string
	Model            string
	ContextFiles     []string
	ContextManifests []string
	Guard            WorkerGuardConfig
}

// ResolveSpawn 解析一次派生：加载 agent、组装上下文、
// 组合系统提示词、解析守卫策略。供 C 组的 spawn 命令复用。
func ResolveSpawn(channel string, opts SpawnOptions) (*ResolvedSpawn, error) {
	cwd := opts.Cwd
	if cwd == "" {
		var err error
		cwd, err = os.Getwd()
		if err != nil {
			return nil, fmt.Errorf("取当前工作目录失败：%w", err)
		}
	}
	trustedRoots := ResolveTrustedRoots(cwd)
	var agentBody string
	provider := opts.Provider
	model := opts.Model
	as := opts.As
	if opts.Agent != "" {
		agent, err := LoadAgent(opts.Agent, cwd, trustedRoots)
		if err != nil {
			return nil, err
		}
		agentBody = agent.SystemPrompt
		if provider == "" {
			provider = agent.Provider
		}
		if model == "" {
			model = agent.Model
		}
		if as == "" {
			as = agent.Name
		}
	}
	if provider == "" {
		return nil, fmt.Errorf("缺少 --provider（agent 定义也没有 provider: frontmatter）")
	}
	if _, err := adapters.ParseProvider(provider); err != nil {
		return nil, err
	}
	if as == "" {
		return nil, fmt.Errorf("缺少 --as（没有 agent 名可回落）")
	}
	if err := store.AssertSafeName(as, "worker"); err != nil {
		return nil, err
	}
	if opts.Sandbox != "" {
		if _, err := adapters.ParseCodexSandboxMode(opts.Sandbox); err != nil {
			return nil, err
		}
	}
	context := AssembleContext(cwd, opts.Files, opts.JSONLs, trustedRoots)
	systemPrompt := BuildSystemPrompt(channel, as, agentBody, context.Prompt)
	guard, err := ResolveGuardConfig(ResolveGuardOptions{
		FlagIdleTimeoutMs:  opts.IdleTimeoutMs,
		FlagMaxLiveWorkers: opts.MaxLiveWorkers,
		Cwd:                cwd,
	})
	if err != nil {
		return nil, err
	}
	return &ResolvedSpawn{
		Provider:         provider,
		As:               as,
		SystemPrompt:     systemPrompt,
		Model:            model,
		ContextFiles:     context.Paths,
		ContextManifests: context.Manifests,
		Guard:            guard,
	}, nil
}

// SpawnResult 是 SpawnWorker 的结果。
type SpawnResult struct {
	PID    int
	Log    string
	Worker string
}

// SpawnWorker 派生一个 worker：
// 先过 OOM 守卫（清理过期空闲 worker、检查存活预算），
// 再写 supervisor 配置并以 detached 方式派生 supervisor 子进程
// （`sunshine channel __supervisor <channel> <worker> <config>`）。
// 返回 supervisor 的 pid 与日志路径。
func SpawnWorker(channel string, opts SpawnOptions) (*SpawnResult, error) {
	cwd := opts.Cwd
	if cwd == "" {
		var err error
		cwd, err = os.Getwd()
		if err != nil {
			return nil, fmt.Errorf("取当前工作目录失败：%w", err)
		}
	}
	scope := store.ChannelScope("")
	if opts.Scope != "" {
		s, err := store.ParseChannelScope(opts.Scope)
		if err != nil {
			return nil, err
		}
		scope = s
	}
	ref, err := store.ResolveExistingChannelRef(channel, store.ResolveChannelOptions{Scope: scope, Cwd: cwd})
	if err != nil {
		return nil, err
	}
	resolved, err := ResolveSpawn(channel, opts)
	if err != nil {
		return nil, err
	}

	// 守卫：整个项目桶一把锁串行化预算检查，避免两个不同 worker 名
	// 同时看到空位、同时派生 supervisor。
	projectDir := store.ProjectDir(ref.Project)
	if err := os.MkdirAll(projectDir, 0o755); err != nil {
		return nil, fmt.Errorf("创建项目桶失败：%w", err)
	}
	guardLock := filepath.Join(projectDir, ".worker-guard.lock")
	spawned, err := store.WithLock(guardLock, func() (*SpawnResult, error) {
		guard := EnforceSpawnBudget(EnforceBudgetInput{
			Project: ref.Project,
			Policy:  resolved.Guard,
		})
		if len(guard.Cleaned) > 0 {
			names := make([]string, len(guard.Cleaned))
			for i, w := range guard.Cleaned {
				names[i] = w.Channel + "/" + w.WorkerID
			}
			fmt.Fprintf(os.Stderr, "[channel guard] 已清理 %d 个过期空闲 worker（TTL %dms）：%s\n",
				len(guard.Cleaned), resolved.Guard.IdleTimeoutMs, strings.Join(names, "、"))
		}
		if !guard.Allowed {
			return nil, fmt.Errorf("%s", FormatBudgetOverflowError(ref.Project, guard.Remaining, resolved.Guard.MaxLiveWorkers))
		}
		// worker 级锁：防并发 spawn / kill 竞态。
		workerLock, err := store.WorkerLockPath(channel, resolved.As, ref.Project)
		if err != nil {
			return nil, err
		}
		return store.WithLock(workerLock, func() (*SpawnResult, error) {
			return spawnLocked(ref, resolved, opts, cwd)
		}, store.AcquireOptions{})
	}, store.AcquireOptions{})
	if err != nil {
		return nil, err
	}
	return spawned, nil
}

func spawnLocked(ref store.ChannelRef, resolved *ResolvedSpawn, opts SpawnOptions, cwd string) (*SpawnResult, error) {
	// 锁内复查 worker 是否已在跑。
	pidPath, err := store.WorkerFile(ref.Name, resolved.As, "pid", ref.Project)
	if err != nil {
		return nil, err
	}
	if data, err := os.ReadFile(pidPath); err == nil {
		if pid, perr := strconv.Atoi(strings.TrimSpace(string(data))); perr == nil && pidAlive(pid) {
			return nil, fmt.Errorf("worker %q 已在 channel %q 里运行（pid %d）", resolved.As, ref.Name, pid)
		}
	}

	spawnedBy := opts.By
	if spawnedBy == "" {
		if v := strings.TrimSpace(os.Getenv("SUNSHINE_CHANNEL_AS")); v != "" {
			spawnedBy = v
		} else if v := strings.TrimSpace(os.Getenv("TRELLIS_CHANNEL_AS")); v != "" {
			// 兼容读旧环境变量（只读旧值，不产生新输出）。
			spawnedBy = v
		} else {
			spawnedBy = "main"
		}
	}

	cfg := SupervisorConfig{
		Provider:         resolved.Provider,
		Cwd:              cwd,
		SystemPrompt:     resolved.SystemPrompt,
		Model:            resolved.Model,
		Resume:           opts.Resume,
		Sandbox:          opts.Sandbox,
		TimeoutMs:        opts.TimeoutMs,
		WarnBeforeMs:     opts.WarnBeforeMs,
		IdleTimeoutMs:    resolved.Guard.IdleTimeoutMs,
		SpawnedBy:        spawnedBy,
		Agent:            opts.Agent,
		ContextFiles:     resolved.ContextFiles,
		ContextManifests: resolved.ContextManifests,
		InboxPolicy:      opts.InboxPolicy,
	}
	configPath, err := WriteSupervisorConfig(ref.Name, resolved.As, cfg, ref.Project)
	if err != nil {
		return nil, err
	}
	reservationPath, err := store.WorkerFile(ref.Name, resolved.As, "reservation", ref.Project)
	if err != nil {
		return nil, err
	}
	if err := os.WriteFile(reservationPath, []byte(fmt.Sprintf(`{"channel":%q,"worker":%q}`, ref.Name, resolved.As)), 0o644); err != nil {
		return nil, fmt.Errorf("写 worker 预留文件失败：%w", err)
	}

	binary := opts.SupervisorBinary
	if binary == "" {
		exe, err := os.Executable()
		if err != nil {
			return nil, fmt.Errorf("取当前可执行文件失败：%w", err)
		}
		binary = exe
	}
	cmd := exec.Command(binary, "channel", "__supervisor", ref.Name, resolved.As, configPath)
	setDetached(cmd)
	cmd.Env = append(os.Environ(), "SUNSHINE_CHANNEL_PROJECT="+ref.Project)
	// detached：父进程不接管子进程的 stdio。
	cmd.Stdout = nil
	cmd.Stderr = nil
	cmd.Stdin = nil
	if err := cmd.Start(); err != nil {
		_ = os.Remove(configPath)
		_ = os.Remove(reservationPath)
		return nil, fmt.Errorf("派生 supervisor 失败（worker %q）：%w", resolved.As, err)
	}
	pid := -1
	if cmd.Process != nil {
		pid = cmd.Process.Pid
		_ = os.WriteFile(pidPath, []byte(fmt.Sprintf("%d", pid)), 0o644)
	}
	logPath, _ := store.WorkerFile(ref.Name, resolved.As, "log", ref.Project)
	return &SpawnResult{PID: pid, Log: logPath, Worker: resolved.As}, nil
}
