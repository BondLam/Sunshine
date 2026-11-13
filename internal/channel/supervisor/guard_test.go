package supervisor

import (
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/linrong007/Sunshine/internal/channel/store"
)

func splitLinesForTest(s string) []string {
	return strings.Split(strings.TrimSuffix(s, "\n"), "\n")
}

func joinLinesForTest(lines []string) string {
	return strings.Join(lines, "\n") + "\n"
}

var tsFieldRe = regexp.MustCompile(`"ts":"[^"]*"`)

func replaceTsForTest(line, ts string) string {
	return tsFieldRe.ReplaceAllString(line, `"ts":"`+ts+`"`)
}

func containsForTest(s, sub string) bool { return strings.Contains(s, sub) }

// startSleeper 启动一个 sleep 30 子进程并返回 *exec.Cmd（测试结束时杀掉）。
func startSleeper(t *testing.T) *exec.Cmd {
	t.Helper()
	cmd := exec.Command("sleep", "30")
	if err := cmd.Start(); err != nil {
		t.Fatalf("启动 sleep 失败：%v", err)
	}
	t.Cleanup(func() {
		if cmd.Process != nil {
			_ = cmd.Process.Kill()
			_ = cmd.Wait()
		}
	})
	return cmd
}

func TestParseGuardDuration(t *testing.T) {
	cases := []struct {
		in   string
		want int64
	}{
		{"0", 0},
		{"300000", 300000}, // 裸整数按毫秒计。
		{"5m", 5 * 60 * 1000},
		{"30s", 30 * 1000},
		{"1h", 60 * 60 * 1000},
		{"1500ms", 1500},
	}
	for _, c := range cases {
		got, err := ParseGuardDuration(c.in)
		if err != nil {
			t.Fatalf("ParseGuardDuration(%q) 报错：%v", c.in, err)
		}
		if got != c.want {
			t.Fatalf("ParseGuardDuration(%q) = %d，应为 %d", c.in, got, c.want)
		}
	}
	for _, bad := range []string{"", "nonsense", "-5m", "-1", "5x"} {
		if _, err := ParseGuardDuration(bad); err == nil {
			t.Fatalf("ParseGuardDuration(%q) 应报错", bad)
		}
	}
}

func getenvMap(m map[string]string) func(string) string {
	return func(k string) string { return m[k] }
}

func TestResolveGuardConfigPrecedence(t *testing.T) {
	// 1. 无任何覆盖：内置默认。
	cfg, err := ResolveGuardConfig(ResolveGuardOptions{Cwd: t.TempDir(), Getenv: getenvMap(nil)})
	if err != nil {
		t.Fatalf("解析失败：%v", err)
	}
	if cfg.IdleTimeoutMs != DefaultIdleTimeoutMs || cfg.MaxLiveWorkers != DefaultMaxLiveWorkers {
		t.Fatalf("默认策略不对：%+v", cfg)
	}
	// 2. 配置文件层。
	dir := t.TempDir()
	configYAML := "channel:\n  worker_guard:\n    idle_timeout: 10m\n    max_live_workers: 3\n"
	if err := os.MkdirAll(filepath.Join(dir, ".sunshine"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, ".sunshine", "config.yaml"), []byte(configYAML), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, err = ResolveGuardConfig(ResolveGuardOptions{Cwd: dir, Getenv: getenvMap(nil)})
	if err != nil {
		t.Fatalf("解析失败：%v", err)
	}
	if cfg.IdleTimeoutMs != 10*60*1000 || cfg.MaxLiveWorkers != 3 {
		t.Fatalf("配置层应生效：%+v", cfg)
	}
	// 3. 环境变量覆盖配置。
	cfg, err = ResolveGuardConfig(ResolveGuardOptions{Cwd: dir, Getenv: getenvMap(map[string]string{
		EnvIdleTimeout: "1m",
	})})
	if err != nil {
		t.Fatalf("解析失败：%v", err)
	}
	if cfg.IdleTimeoutMs != 60*1000 || cfg.MaxLiveWorkers != 3 {
		t.Fatalf("环境变量应覆盖 idle、保留配置的 max：%+v", cfg)
	}
	// 4. flag 覆盖环境变量；0 表示禁用。
	flagIdle := int64(0)
	flagMax := 0
	cfg, err = ResolveGuardConfig(ResolveGuardOptions{
		Cwd: dir, FlagIdleTimeoutMs: &flagIdle, FlagMaxLiveWorkers: &flagMax,
		Getenv: getenvMap(map[string]string{EnvIdleTimeout: "1m", EnvMaxLiveWorkers: "2"}),
	})
	if err != nil {
		t.Fatalf("解析失败：%v", err)
	}
	if cfg.IdleTimeoutMs != 0 || cfg.MaxLiveWorkers != 0 {
		t.Fatalf("flag 0 应禁用两项：%+v", cfg)
	}
	// 5. 非法值报错。
	if _, err := ResolveGuardConfig(ResolveGuardOptions{Cwd: dir, Getenv: getenvMap(map[string]string{EnvIdleTimeout: "nonsense"})}); err == nil {
		t.Fatal("非法环境变量时长应报错")
	}
	if _, err := ResolveGuardConfig(ResolveGuardOptions{Cwd: dir, Getenv: getenvMap(map[string]string{EnvMaxLiveWorkers: "-1"})}); err == nil {
		t.Fatal("非法环境变量整数应报错")
	}
	neg := int64(-5)
	if _, err := ResolveGuardConfig(ResolveGuardOptions{Cwd: dir, FlagIdleTimeoutMs: &neg}); err == nil {
		t.Fatal("负数 flag 应报错")
	}
}

func TestIsIdleCleanupEligible(t *testing.T) {
	now := time.Now()
	idleSince := now.Add(-10 * time.Minute).Format(time.RFC3339Nano)
	base := LiveWorker{State: store.WorkerState{
		Activity:  store.WorkerActivityIdle,
		IdleSince: idleSince,
	}}
	if !IsIdleCleanupEligible(base, 5*60*1000, now) {
		t.Fatal("空闲 10 分钟、TTL 5 分钟应可清理")
	}
	if IsIdleCleanupEligible(base, 15*60*1000, now) {
		t.Fatal("TTL 15 分钟时不应清理")
	}
	if IsIdleCleanupEligible(base, 0, now) {
		t.Fatal("TTL 0（禁用）时不应清理")
	}
	midTurn := base
	midTurn.State.Activity = store.WorkerActivityMidTurn
	if IsIdleCleanupEligible(midTurn, 5*60*1000, now) {
		t.Fatal("回合中的 worker 不应清理")
	}
	noIdleSince := base
	noIdleSince.State.IdleSince = ""
	if IsIdleCleanupEligible(noIdleSince, 5*60*1000, now) {
		t.Fatal("无 idleSince 的 worker 不应清理")
	}
	terminal := base
	terminal.State.Terminal = true
	if IsIdleCleanupEligible(terminal, 5*60*1000, now) {
		t.Fatal("终态 worker 不应清理")
	}
}

// makeLiveWorker 在测试存储里造一个存活 worker：
// spawned 事件 + supervisor pid 文件（指向当前测试进程，存活）。
func makeLiveWorker(t *testing.T, ref store.ChannelRef, worker string) {
	t.Helper()
	if _, err := store.AppendEvent(ref, store.AppendableEvent{
		Kind: store.KindSpawned, By: "main",
		Extra: map[string]any{"as": worker, "provider": "claude", "inboxPolicy": "explicitOnly"},
	}); err != nil {
		t.Fatalf("写 spawned 事件失败：%v", err)
	}
	p, err := store.WorkerFile(ref.Name, worker, "pid", ref.Project)
	if err != nil {
		t.Fatalf("取 pid 路径失败：%v", err)
	}
	if err := os.WriteFile(p, []byte(strconv.Itoa(os.Getpid())), 0o644); err != nil {
		t.Fatalf("写 pid 文件失败：%v", err)
	}
}

func TestScanLiveWorkersAndBudget(t *testing.T) {
	ref := testSetup(t, "guard")
	makeLiveWorker(t, ref, "w1")
	live := ScanLiveWorkers("test-proj", func(pid int, channel, worker string) bool { return true })
	if len(live) != 1 || live[0].WorkerID != "w1" {
		t.Fatalf("应扫到 1 个存活 worker，实际 %+v", live)
	}
	if live[0].SupervisorPID != os.Getpid() {
		t.Fatalf("supervisor pid 应为测试进程 pid，实际 %d", live[0].SupervisorPID)
	}
	// 预算 1：已满，不可再派生。
	res := EnforceSpawnBudget(EnforceBudgetInput{
		Project: "test-proj",
		Policy:  WorkerGuardConfig{IdleTimeoutMs: 0, MaxLiveWorkers: 1},
		Verify:  func(pid int, channel, worker string) bool { return true },
	})
	if res.Allowed || len(res.Remaining) != 1 {
		t.Fatalf("预算 1 且已有 1 个时应拒绝：%+v", res)
	}
	// 预算 2：可派生。
	res = EnforceSpawnBudget(EnforceBudgetInput{
		Project: "test-proj",
		Policy:  WorkerGuardConfig{IdleTimeoutMs: 0, MaxLiveWorkers: 2},
		Verify:  func(pid int, channel, worker string) bool { return true },
	})
	if !res.Allowed {
		t.Fatalf("预算 2 时应允许：%+v", res)
	}
	// 预算 0：禁用检查，恒允许。
	res = EnforceSpawnBudget(EnforceBudgetInput{
		Project: "test-proj",
		Policy:  WorkerGuardConfig{IdleTimeoutMs: 0, MaxLiveWorkers: 0},
		Verify:  func(pid int, channel, worker string) bool { return true },
	})
	if !res.Allowed {
		t.Fatalf("预算 0（禁用）时应允许：%+v", res)
	}
}

func TestEnforceSpawnBudgetCleansExpiredIdle(t *testing.T) {
	ref := testSetup(t, "guard2")
	makeLiveWorker(t, ref, "w1")
	// 让 spawned 事件的 idleSince 变旧：直接改写事件行的 ts
	//（测试存储是 JSONL 文件，改 ts 后投影的 idleSince 随之变旧）。
	eventsPath, err := store.EventsPath(ref.Name, ref.Project)
	if err != nil {
		t.Fatalf("取事件路径失败：%v", err)
	}
	data, err := os.ReadFile(eventsPath)
	if err != nil {
		t.Fatalf("读事件日志失败：%v", err)
	}
	old := time.Now().Add(-30 * time.Minute).UTC().Format(time.RFC3339Nano)
	// 把 spawned 事件（不是 create 事件）的 ts 换成 30 分钟前。
	lines := splitLinesForTest(string(data))
	if len(lines) == 0 {
		t.Fatal("事件日志为空")
	}
	rewrote := false
	for i, l := range lines {
		if strings.Contains(l, `"kind":"spawned"`) {
			lines[i] = replaceTsForTest(l, old)
			rewrote = true
		}
	}
	if !rewrote {
		t.Fatal("事件日志里没有 spawned 事件")
	}
	if err := os.WriteFile(eventsPath, []byte(joinLinesForTest(lines)), 0o644); err != nil {
		t.Fatalf("改写事件日志失败：%v", err)
	}
	// TTL 5 分钟：应清理 w1（发 SIGTERM 给测试进程自身？
	// 注意：pid 文件指向测试进程！清理会给自己发 SIGTERM。
	// 因此这里把 pid 文件改成一个已死的 pid，避免真杀自己，
	// 同时验证“pid 已死不算存活、直接不计入预算”的分支。
	// 改用另一个思路：先验证扫描在 pid 已死时不计入。
	p, _ := store.WorkerFile(ref.Name, "w1", "pid", ref.Project)
	if err := os.WriteFile(p, []byte("999999"), 0o644); err != nil {
		t.Fatalf("改 pid 文件失败：%v", err)
	}
	live := ScanLiveWorkers("test-proj", func(pid int, channel, worker string) bool { return true })
	if len(live) != 0 {
		t.Fatalf("supervisor pid 已死时不应计入存活，实际 %+v", live)
	}
	res := EnforceSpawnBudget(EnforceBudgetInput{
		Project: "test-proj",
		Policy:  WorkerGuardConfig{IdleTimeoutMs: 5 * 60 * 1000, MaxLiveWorkers: 1},
		Verify:  func(pid int, channel, worker string) bool { return true },
	})
	if !res.Allowed {
		t.Fatalf("死 worker 不占预算，应允许派生：%+v", res)
	}
}

func TestCleanupExpiredIdleWorkersSignalsSupervisor(t *testing.T) {
	// 用一个真实子进程（sleep）当 supervisor：给它发 SIGTERM 后应退出，
	// 且 shutdown-reason sidecar 应先写好。
	ref := testSetup(t, "guard3")
	makeLiveWorker(t, ref, "w1")
	// 把 pid 文件指向一个 sleep 子进程。
	p, _ := store.WorkerFile(ref.Name, "w1", "pid", ref.Project)
	sleeper := startSleeper(t)
	if err := os.WriteFile(p, []byte(strconv.Itoa(sleeper.Process.Pid)), 0o644); err != nil {
		t.Fatalf("改 pid 文件失败：%v", err)
	}
	// 把 idleSince 推到 30 分钟前（改事件 ts），再扫描。
	eventsPath, _ := store.EventsPath(ref.Name, ref.Project)
	data, _ := os.ReadFile(eventsPath)
	old := time.Now().Add(-30 * time.Minute).UTC().Format(time.RFC3339Nano)
	lines := splitLinesForTest(string(data))
	rewrote := false
	for i, l := range lines {
		if strings.Contains(l, `"kind":"spawned"`) {
			lines[i] = replaceTsForTest(l, old)
			rewrote = true
		}
	}
	if !rewrote {
		t.Fatal("事件日志里没有 spawned 事件")
	}
	_ = os.WriteFile(eventsPath, []byte(joinLinesForTest(lines)), 0o644)

	live := ScanLiveWorkers("test-proj", func(pid int, channel, worker string) bool { return true })
	if len(live) != 1 {
		t.Fatalf("应扫到 1 个存活 worker，实际 %+v", live)
	}

	res := CleanupExpiredIdleWorkers(live, 5*60*1000, "test-proj", time.Now())
	if len(res.Killed) != 1 {
		t.Fatalf("应清理 1 个过期 worker，实际 %+v", res)
	}
	reasonPath, _ := store.WorkerFile(ref.Name, "w1", "shutdown-reason", ref.Project)
	reason, err := os.ReadFile(reasonPath)
	if err != nil || string(reason) != "idle-timeout\n" {
		t.Fatalf("清理前应写 shutdown-reason sidecar，实际 %q / %v", string(reason), err)
	}
	// sleep 进程应已收到 SIGTERM 而退出（用 Wait 回收：子进程死后
	// 未 Wait 前是僵尸，kill(pid, 0) 仍报存活，不能只用 pidAlive 判断）。
	exited := make(chan error, 1)
	go func() { exited <- sleeper.Wait() }()
	select {
	case <-exited:
	case <-time.After(3 * time.Second):
		t.Fatalf("sleep 进程收到 SIGTERM 后应退出")
	}
}

func TestFormatBudgetOverflowError(t *testing.T) {
	msg := FormatBudgetOverflowError("proj-x", []LiveWorker{{
		Channel: "c1", WorkerID: "w1", SupervisorPID: 123,
		State: store.WorkerState{Provider: "claude", Lifecycle: store.WorkerLifecycleRunning, Activity: store.WorkerActivityIdle},
	}}, 1)
	for _, want := range []string{"proj-x", "1/1", "c1", "w1", "sunshine channel kill", "--max-live-workers 2", "max_live_workers"} {
		if !containsForTest(msg, want) {
			t.Fatalf("预算报错应含 %q，实际：\n%s", want, msg)
		}
	}
}
