package supervisor

import (
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/linrong007/Sunshine/internal/channel/store"
	"github.com/linrong007/Sunshine/internal/config"
)

// WorkerGuardConfig 是 worker 守卫的有效策略。
type WorkerGuardConfig struct {
	// IdleTimeoutMs 是空闲清理 TTL（毫秒）；0 表示禁用空闲清理。
	IdleTimeoutMs int64
	// MaxLiveWorkers 是存活 worker 预算；0 表示禁用 spawn 时预算检查。
	MaxLiveWorkers int
}

// DefaultIdleTimeoutMs 是内置默认空闲清理 TTL：5 分钟。
const DefaultIdleTimeoutMs = int64(5 * 60 * 1000)

// DefaultMaxLiveWorkers 是内置默认存活 worker 预算：6 个。
const DefaultMaxLiveWorkers = 6

// EnvIdleTimeout 是空闲 TTL 的环境变量覆盖。
const EnvIdleTimeout = "SUNSHINE_CHANNEL_WORKER_IDLE_TIMEOUT"

// EnvMaxLiveWorkers 是存活预算的环境变量覆盖。
const EnvMaxLiveWorkers = "SUNSHINE_CHANNEL_MAX_LIVE_WORKERS"

// legacyEnvIdleTimeout / legacyEnvMaxLiveWorkers 是旧环境变量名，
// 只作兼容回退读取，不产生新输出。
const (
	legacyEnvIdleTimeout    = "TRELLIS_CHANNEL_WORKER_IDLE_TIMEOUT"
	legacyEnvMaxLiveWorkers = "TRELLIS_CHANNEL_MAX_LIVE_WORKERS"
)

// ResolveGuardOptions 是 ResolveGuardConfig 的选项。
type ResolveGuardOptions struct {
	// FlagIdleTimeoutMs 是 CLI --idle-timeout 的值（毫秒，已解析）。
	FlagIdleTimeoutMs *int64
	// FlagMaxLiveWorkers 是 CLI --max-live-workers 的值。
	FlagMaxLiveWorkers *int
	// Cwd 是配置查找目录（默认当前工作目录）。
	Cwd string
	// Getenv 是环境变量读取函数；nil 时用 os.Getenv（测试可注入）。
	Getenv func(string) string
}

// ParseGuardDuration 解析守卫时长：裸整数按毫秒计（如 0 表示禁用），
// 其余按 Go duration 写法（5m、30s、1h）。负数报错。
func ParseGuardDuration(raw string) (int64, error) {
	s := strings.TrimSpace(raw)
	if s == "" {
		return 0, fmt.Errorf("空的时长")
	}
	if n, err := strconv.ParseInt(s, 10, 64); err == nil {
		if n < 0 {
			return 0, fmt.Errorf("时长须为非负数（实际 %q）", raw)
		}
		return n, nil
	}
	d, err := time.ParseDuration(s)
	if err != nil {
		return 0, fmt.Errorf("无效的时长 %q（用 5m / 30s / 1h 这类写法，裸整数按毫秒计）", raw)
	}
	if d < 0 {
		return 0, fmt.Errorf("时长须为非负数（实际 %q）", raw)
	}
	return d.Milliseconds(), nil
}

// ResolveGuardConfig 解析生效的守卫策略。优先级（与上游 guard.ts 一致）：
//  1. CLI flag；2. 环境变量；3. .sunshine/config.yaml 的 channel.worker_guard；
//  4. 内置默认值（idle_timeout 5m、max_live_workers 6）。
func ResolveGuardConfig(opts ResolveGuardOptions) (WorkerGuardConfig, error) {
	getenv := opts.Getenv
	if getenv == nil {
		getenv = os.Getenv
	}
	cwd := opts.Cwd
	if cwd == "" {
		var err error
		cwd, err = os.Getwd()
		if err != nil {
			cwd = "."
		}
	}
	var fromConfig *config.Config
	cfg, err := config.LoadProject(cwd)
	if err != nil {
		return WorkerGuardConfig{}, fmt.Errorf("读项目配置失败：%w", err)
	}
	fromConfig = cfg

	idleTimeoutMs := DefaultIdleTimeoutMs
	if v := pickGuardDuration(opts.FlagIdleTimeoutMs, parseEnvGuardDuration(firstNonEmpty(getenv(EnvIdleTimeout), getenv(legacyEnvIdleTimeout)))); v != nil {
		if v.err != nil {
			return WorkerGuardConfig{}, v.err
		}
		idleTimeoutMs = v.ms
	} else if fromConfig != nil {
		if ms, err := ParseGuardDuration(fromConfig.Channel.WorkerGuard.IdleTimeout); err != nil {
			return WorkerGuardConfig{}, fmt.Errorf("channel.worker_guard.idle_timeout：%w", err)
		} else {
			idleTimeoutMs = ms
		}
	}

	maxLiveWorkers := DefaultMaxLiveWorkers
	if v := pickGuardInt(opts.FlagMaxLiveWorkers, parseEnvGuardInt(firstNonEmpty(getenv(EnvMaxLiveWorkers), getenv(legacyEnvMaxLiveWorkers)))); v != nil {
		if v.err != nil {
			return WorkerGuardConfig{}, v.err
		}
		maxLiveWorkers = v.n
	} else if fromConfig != nil {
		maxLiveWorkers = fromConfig.Channel.WorkerGuard.MaxLiveWorkers
	}
	return WorkerGuardConfig{IdleTimeoutMs: idleTimeoutMs, MaxLiveWorkers: maxLiveWorkers}, nil
}

type guardDurationValue struct {
	ms  int64
	err error
}

func parseEnvGuardDuration(raw string) *guardDurationValue {
	if strings.TrimSpace(raw) == "" {
		return nil
	}
	ms, err := ParseGuardDuration(raw)
	return &guardDurationValue{ms: ms, err: err}
}

func pickGuardDuration(flag *int64, env *guardDurationValue) *guardDurationValue {
	if flag != nil {
		if *flag < 0 {
			return &guardDurationValue{err: fmt.Errorf("空闲超时须为非负时长（实际 %dms）", *flag)}
		}
		return &guardDurationValue{ms: *flag}
	}
	return env
}

type guardIntValue struct {
	n   int
	err error
}

func parseEnvGuardInt(raw string) *guardIntValue {
	if strings.TrimSpace(raw) == "" {
		return nil
	}
	n, err := strconv.Atoi(strings.TrimSpace(raw))
	if err != nil || n < 0 {
		return &guardIntValue{err: fmt.Errorf("存活 worker 数须为非负整数（实际 %q）", raw)}
	}
	return &guardIntValue{n: n}
}

func pickGuardInt(flag *int, env *guardIntValue) *guardIntValue {
	if flag != nil {
		if *flag < 0 {
			return &guardIntValue{err: fmt.Errorf("存活 worker 数须为非负整数（实际 %d）", *flag)}
		}
		return &guardIntValue{n: *flag}
	}
	return env
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}

// LiveWorker 是守卫扫描中看到的一个存活 worker。
type LiveWorker struct {
	Channel  string
	WorkerID string
	State    store.WorkerState
	// SupervisorPID 是 supervisor 的 pid（读不到时为 0）。
	SupervisorPID int
	// SupervisorVerified 表示该 pid 的命令行确实像本 worker 的 supervisor。
	SupervisorVerified bool
	// WorkerPID 是 worker 子进程的 pid（读不到时为 0）。
	WorkerPID int
}

// ScanLiveWorkers 枚举某项目桶里全部存活（非终态且 supervisor pid 存活）的 worker。
// verify 为空时用默认的命令行校验（IsSupervisorProcess）；
// 测试可注入恒真/恒假的校验函数。
func ScanLiveWorkers(project string, verify func(pid int, channel, worker string) bool) []LiveWorker {
	if verify == nil {
		verify = IsSupervisorProcess
	}
	var out []LiveWorker
	for _, channel := range store.ListChannelNamesInProject(project) {
		ref := store.ChannelRef{Name: channel, Project: project}
		events, err := store.ReadChannelEvents(ref, nil)
		if err != nil {
			continue
		}
		registry := store.ReduceWorkerRegistry(events, &ref)
		for _, state := range registry.Workers {
			if state.Terminal || store.IsTerminalLifecycle(state.Lifecycle) {
				continue
			}
			supervisorPID := readPIDFile(ref, state.WorkerID, "pid")
			if supervisorPID == 0 || !pidAlive(supervisorPID) {
				// supervisor pid 文件缺失或已死：即使持久状态仍显示运行中，
				// 也不算存活的 OS 进程，守卫忽略它。
				continue
			}
			out = append(out, LiveWorker{
				Channel:            channel,
				WorkerID:           state.WorkerID,
				State:              state,
				SupervisorPID:      supervisorPID,
				SupervisorVerified: verify(supervisorPID, channel, state.WorkerID),
				WorkerPID:          readPIDFile(ref, state.WorkerID, "worker-pid"),
			})
		}
		// 预留位 worker（spawn 已写 reservation、supervisor 尚未写 spawned 事件）。
		for _, workerID := range listReservationWorkers(ref) {
			found := false
			for _, w := range out {
				if w.Channel == channel && w.WorkerID == workerID {
					found = true
					break
				}
			}
			if found {
				continue
			}
			supervisorPID := readPIDFile(ref, workerID, "pid")
			if supervisorPID == 0 || !pidAlive(supervisorPID) {
				continue
			}
			out = append(out, LiveWorker{
				Channel:            channel,
				WorkerID:           workerID,
				SupervisorPID:      supervisorPID,
				SupervisorVerified: verify(supervisorPID, channel, workerID),
				State: store.WorkerState{
					WorkerID:    workerID,
					Lifecycle:   store.WorkerLifecycleStarting,
					Activity:    store.WorkerActivityIdle,
					InboxPolicy: store.DefaultInboxPolicy,
				},
			})
		}
	}
	return out
}

// listReservationWorkers 列出某 channel 下写了 .reservation 文件的 worker 名。
func listReservationWorkers(ref store.ChannelRef) []string {
	dir := ref.Dir
	if dir == "" {
		var err error
		dir, err = store.ChannelDir(ref.Name, ref.Project)
		if err != nil {
			return nil
		}
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	var out []string
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".reservation") {
			continue
		}
		out = append(out, strings.TrimSuffix(e.Name(), ".reservation"))
	}
	return out
}

func readPIDFile(ref store.ChannelRef, worker, suffix string) int {
	p, err := store.WorkerFile(ref.Name, worker, suffix, ref.Project)
	if err != nil {
		return 0
	}
	data, err := os.ReadFile(p)
	if err != nil {
		return 0
	}
	n, err := strconv.Atoi(strings.TrimSpace(string(data)))
	if err != nil || n <= 0 {
		return 0
	}
	return n
}

// supervisorCmdRe 匹配 supervisor 子进程的命令行：
// `sunshine channel __supervisor <channel> <worker>`。
func supervisorCmdPattern(channel, worker string) *regexp.Regexp {
	return regexp.MustCompile(`(?:^|\s)channel\s+__supervisor\s+` + regexp.QuoteMeta(channel) + `\s+` + regexp.QuoteMeta(worker) + `(?:\s|$)`)
}

// IsSupervisorProcess 判断 pid 的命令行是否像指定 worker 的 supervisor。
// 非 Linux 平台返回 false（与上游 win32 分支一致）。
func IsSupervisorProcess(pid int, channel, worker string) bool {
	if pid <= 0 {
		return false
	}
	out, err := exec.Command("ps", "-p", strconv.Itoa(pid), "-o", "args=").Output()
	if err != nil {
		return false
	}
	return supervisorCmdPattern(channel, worker).MatchString(strings.TrimSpace(string(out)))
}

// IsIdleCleanupEligible 是纯判断：该存活 worker 现在是否可被空闲清理？
// 回合中的 worker 与没有 idleSince 的 worker（从未干净拉起）永不清理。
func IsIdleCleanupEligible(live LiveWorker, idleTimeoutMs int64, now time.Time) bool {
	if idleTimeoutMs <= 0 {
		return false
	}
	if live.State.Activity != store.WorkerActivityIdle {
		return false
	}
	if live.State.IdleSince == "" {
		return false
	}
	if live.State.Terminal {
		return false
	}
	idleSince, err := time.Parse(time.RFC3339Nano, live.State.IdleSince)
	if err != nil {
		return false
	}
	return now.Sub(idleSince) >= time.Duration(idleTimeoutMs)*time.Millisecond
}

// CleanupResult 是空闲清理的结果。
type CleanupResult struct {
	Killed []LiveWorker
	Failed []struct {
		Worker LiveWorker
		Err    string
	}
}

// CleanupExpiredIdleWorkers 杀掉空闲 TTL 已到的 worker：
// 先写一次性 shutdown-reason sidecar，再给 supervisor 发 SIGTERM，
// supervisor 已有的关停漏斗会追加唯一一条 killed 事件。
// 失败只收集，不致命；下次扫描重新评估。
func CleanupExpiredIdleWorkers(candidates []LiveWorker, idleTimeoutMs int64, project string, now time.Time) CleanupResult {
	var result CleanupResult
	if idleTimeoutMs <= 0 {
		return result
	}
	for _, live := range candidates {
		if !IsIdleCleanupEligible(live, idleTimeoutMs, now) {
			continue
		}
		if live.SupervisorPID == 0 || !live.SupervisorVerified || !pidAlive(live.SupervisorPID) {
			continue
		}
		reasonPath, err := store.WorkerFile(live.Channel, live.WorkerID, "shutdown-reason", project)
		if err != nil {
			result.Failed = append(result.Failed, struct {
				Worker LiveWorker
				Err    string
			}{live, err.Error()})
			continue
		}
		if err := os.WriteFile(reasonPath, []byte("idle-timeout\n"), 0o644); err != nil {
			result.Failed = append(result.Failed, struct {
				Worker LiveWorker
				Err    string
			}{live, err.Error()})
			continue
		}
		if err := signalTerm(live.SupervisorPID); err != nil {
			_ = os.Remove(reasonPath)
			result.Failed = append(result.Failed, struct {
				Worker LiveWorker
				Err    string
			}{live, err.Error()})
			continue
		}
		result.Killed = append(result.Killed, live)
	}
	return result
}

// EnforceBudgetInput 是 EnforceSpawnBudget 的输入。
type EnforceBudgetInput struct {
	// Project 是项目桶（空时按当前项目解析）。
	Project string
	// Policy 是生效的守卫策略。
	Policy WorkerGuardConfig
	// Now 覆盖当前时间（测试用确定性时间）。
	Now time.Time
	// Verify 覆盖 supervisor 进程校验（测试用）。
	Verify func(pid int, channel, worker string) bool
}

// EnforceBudgetResult 是 EnforceSpawnBudget 的结果。
type EnforceBudgetResult struct {
	// Cleaned 是本次清理掉的过期空闲 worker。
	Cleaned []LiveWorker
	// Remaining 是清理后剩余的存活 worker。
	Remaining []LiveWorker
	// Allowed 表示是否还有预算再派生一个。
	Allowed bool
}

// EnforceSpawnBudget 执行 spawn 时的守卫：
// 先清理过期空闲 worker，再重扫，最后判断存活预算是否还有空位。
func EnforceSpawnBudget(input EnforceBudgetInput) EnforceBudgetResult {
	project := input.Project
	if project == "" {
		project = store.CurrentProjectKey()
	}
	now := input.Now
	if now.IsZero() {
		now = time.Now()
	}
	initial := ScanLiveWorkers(project, input.Verify)
	cleanup := CleanupExpiredIdleWorkers(initial, input.Policy.IdleTimeoutMs, project, now)
	// 重扫时排除刚被信号通知、但尚未拆掉 pid 文件的 worker，
	// 避免把它们重复计入预算。
	killed := map[string]bool{}
	for _, w := range cleanup.Killed {
		killed[w.Channel+"::"+w.WorkerID] = true
	}
	var remaining []LiveWorker
	for _, w := range ScanLiveWorkers(project, input.Verify) {
		if !killed[w.Channel+"::"+w.WorkerID] {
			remaining = append(remaining, w)
		}
	}
	allowed := input.Policy.MaxLiveWorkers <= 0 || len(remaining) < input.Policy.MaxLiveWorkers
	return EnforceBudgetResult{Cleaned: cleanup.Killed, Remaining: remaining, Allowed: allowed}
}

// FormatBudgetOverflowError 生成预算超限的多行报错文案。
func FormatBudgetOverflowError(project string, live []LiveWorker, limit int) string {
	var b strings.Builder
	fmt.Fprintf(&b, "项目 %q 的存活 worker 预算已用完：%d/%d 个存活 worker。\n", project, len(live), limit)
	for _, w := range live {
		fmt.Fprintf(&b, "  • channel=%q worker=%q provider=%s lifecycle=%s activity=%s pid=%d",
			w.Channel, w.WorkerID, w.State.Provider, w.State.Lifecycle, w.State.Activity, w.SupervisorPID)
		if !w.SupervisorVerified {
			b.WriteString(" supervisor=未验证")
		}
		b.WriteString("\n")
	}
	b.WriteString("派生前请先释放一个名额，例如：\n")
	b.WriteString("  sunshine channel kill <频道> --as <worker>\n")
	b.WriteString("或本次派生时放宽预算：\n")
	fmt.Fprintf(&b, "  sunshine channel spawn ... --max-live-workers %d\n", len(live)+1)
	b.WriteString("或在 .sunshine/config.yaml 的 channel.worker_guard.max_live_workers 里调高默认值。")
	return b.String()
}
