package supervisor

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/linrong007/Sunshine/internal/channel/adapters"
	"github.com/linrong007/Sunshine/internal/channel/store"
)

// readSupervisorConfigFile 读回 WriteSupervisorConfig 写出的 JSON 配置。
func readSupervisorConfigFile(t *testing.T, path string) SupervisorConfig {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("读配置失败：%v", err)
	}
	var cfg SupervisorConfig
	if err := json.Unmarshal(data, &cfg); err != nil {
		t.Fatalf("解析配置失败：%v", err)
	}
	return cfg
}

func TestRunParseTraceClaude(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "claude.jsonl")
	lines := []string{
		`{"type":"system","subtype":"init","session_id":"sess-1"}`,
		`{"type":"assistant","message":{"model":"claude-opus-4-7","content":[{"type":"text","text":"你好"}]}}`,
		`{"type":"assistant","message":{"content":[{"type":"tool_use","name":"Bash","id":"toolu_1","input":{"command":"ls"}}]}}`,
		`{"type":"result","subtype":"success","session_id":"sess-1","duration_ms":100,"num_turns":2}`,
	}
	writeTestFile(t, file, strings.Join(lines, "\n")+"\n")
	var out strings.Builder
	if err := RunParseTrace(adapters.ProviderClaude, file, &out); err != nil {
		t.Fatalf("解析失败：%v", err)
	}
	text := out.String()
	for _, want := range []string{
		`"line":1`, `"kind":"message"`,
		`"line":2`, `"persistSessionId":"sess-1"`,
		`"line":3`, `"kind":"progress"`,
		`"line":4`, `"kind":"done"`,
		`"total_lines":4`, `"total_events":3`, // init 只产生 side-effect，不计事件。
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("trace 输出应含 %q，实际：\n%s", want, text)
		}
	}
}

func TestRunParseTraceCodex(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "codex.jsonl")
	lines := []string{
		`{"id":1,"result":{"userAgent":"Codex/0.164.0"}}`,
		`{"id":2,"result":{"thread":{"id":"thread-1"}}}`,
		`{"method":"item/started","params":{"item":{"id":"item_1","type":"agentMessage"}}}`,
		`{"method":"item/completed","params":{"item":{"id":"item_1","type":"agentMessage","phase":"final_answer","text":"完成"}}}`, // 含最终答案
		`{"method":"turn/completed","params":{"usage":{"inputTokens":10,"outputTokens":5}}}`,
	}
	writeTestFile(t, file, strings.Join(lines, "\n")+"\n")
	var out strings.Builder
	if err := RunParseTrace(adapters.ProviderCodex, file, &out); err != nil {
		t.Fatalf("解析失败：%v", err)
	}
	text := out.String()
	for _, want := range []string{
		"rpc-resolved", // initialize 与 thread/start 的响应解决行。
		`"kind":"message"`,
		`"kind":"done"`,
		`"total_lines":5`,
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("trace 输出应含 %q，实际：\n%s", want, text)
		}
	}
}

func TestRunParseTraceErrors(t *testing.T) {
	if err := RunParseTrace(adapters.Provider("gemini"), "x.jsonl", &strings.Builder{}); err == nil {
		t.Fatal("未知 provider 应报错")
	}
	if err := RunParseTrace(adapters.ProviderClaude, "/不存在/trace.jsonl", &strings.Builder{}); err == nil {
		t.Fatal("不存在的文件应报错")
	}
	dir := t.TempDir()
	file := filepath.Join(dir, "bad.jsonl")
	writeTestFile(t, file, "not json\n")
	var out strings.Builder
	if err := RunParseTrace(adapters.ProviderClaude, file, &out); err != nil {
		t.Fatalf("坏行不应致命：%v", err)
	}
	if !strings.Contains(out.String(), `"kind":"error"`) {
		t.Fatalf("坏行应输出 error 事件：\n%s", out.String())
	}
}

func TestResolveSpawn(t *testing.T) {
	dir := t.TempDir()
	// 写一个 agent 定义。
	agentMD := "---\nname: implement\ndescription: 实现专家。\nprovider: claude\nmodel: opus\n---\n\n你是实现专家。\n"
	writeTestFile(t, filepath.Join(dir, ".sunshine", "agents", "implement.md"), agentMD)
	writeTestFile(t, filepath.Join(dir, "spec.md"), "规范内容")
	resolved, err := ResolveSpawn("demo", SpawnOptions{
		As: "implement", Agent: "implement",
		Cwd: dir, Files: []string{"spec.md"}, TimeoutMs: 30 * 60 * 1000,
	})
	if err != nil {
		t.Fatalf("解析 spawn 失败：%v", err)
	}
	if resolved.Provider != "claude" || resolved.Model != "opus" || resolved.As != "implement" {
		t.Fatalf("agent 回落不对：%+v", resolved)
	}
	if !strings.Contains(resolved.SystemPrompt, "你是实现专家。") || !strings.Contains(resolved.SystemPrompt, "规范内容") {
		t.Fatalf("系统提示词应含 agent 角色与上下文：\n%s", resolved.SystemPrompt)
	}
	if len(resolved.ContextFiles) != 1 || resolved.ContextFiles[0] != "spec.md" {
		t.Fatalf("上下文文件应记录：%v", resolved.ContextFiles)
	}
	// 显式 --provider/--model/--as 优先于 agent。
	resolved, err = ResolveSpawn("demo", SpawnOptions{
		Agent:    "implement",
		Provider: "codex", Model: "gpt-5.3-codex", As: "checker",
		Cwd: dir,
	})
	if err != nil {
		t.Fatalf("解析失败：%v", err)
	}
	if resolved.Provider != "codex" || resolved.Model != "gpt-5.3-codex" || resolved.As != "checker" {
		t.Fatalf("显式参数应优先：%+v", resolved)
	}
	// 未知 provider 报错。
	if _, err := ResolveSpawn("demo", SpawnOptions{As: "w", Provider: "gemini", Cwd: dir}); err == nil {
		t.Fatal("未知 provider 应报错")
	}
	// codex 沙箱非法报错。
	if _, err := ResolveSpawn("demo", SpawnOptions{As: "w", Provider: "codex", Sandbox: "bad", Cwd: dir}); err == nil {
		t.Fatal("非法沙箱模式应报错")
	}
	// worker 名非法报错。
	if _, err := ResolveSpawn("demo", SpawnOptions{As: "a/b", Cwd: dir}); err == nil {
		t.Fatal("非法 worker 名应报错")
	}
	// agent 不存在报错。
	if _, err := ResolveSpawn("demo", SpawnOptions{As: "w", Agent: "nope", Cwd: dir}); err == nil {
		t.Fatal("不存在的 agent 应报错")
	}
}

func TestResolveSpawnGuardFromEnv(t *testing.T) {
	dir := t.TempDir()
	t.Setenv(EnvIdleTimeout, "2m")
	t.Setenv(EnvMaxLiveWorkers, "3")
	resolved, err := ResolveSpawn("demo", SpawnOptions{As: "w", Provider: "claude", Cwd: dir})
	if err != nil {
		t.Fatalf("解析失败：%v", err)
	}
	if resolved.Guard.IdleTimeoutMs != 2*60*1000 || resolved.Guard.MaxLiveWorkers != 3 {
		t.Fatalf("环境变量守卫策略应解析：%+v", resolved.Guard)
	}
}

func TestWriteSupervisorConfigRoundTrip(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("SUNSHINE_CHANNEL_ROOT", t.TempDir())
	t.Setenv("SUNSHINE_CHANNEL_PROJECT", "test-proj")
	cfg := SupervisorConfig{
		Provider: "claude", Cwd: dir, Model: "opus",
		SystemPrompt: "提示词", TimeoutMs: 60000,
	}
	path, err := WriteSupervisorConfig("cfgtest", "w1", cfg, "test-proj")
	if err != nil {
		t.Fatalf("写配置失败：%v", err)
	}
	loaded := readSupervisorConfigFile(t, path)
	if loaded.Provider != cfg.Provider || loaded.Model != cfg.Model || loaded.SystemPrompt != cfg.SystemPrompt {
		t.Fatalf("配置往返不一致：%+v", loaded)
	}
	if loaded.WarnBeforeMs != nil {
		t.Fatalf("未设置 WarnBeforeMs 时应为 nil，实际 %v", *loaded.WarnBeforeMs)
	}
}

func TestSpawnWorkerWritesReservationAndPID(t *testing.T) {
	ref := testSetup(t, "spawn")
	// supervisorBinary 用一个“睡 30 秒”的小脚本：SpawnWorker 只负责派生
	// （不等待子进程），因此可验证配置写盘、.reservation 与 pid 文件的写盘逻辑；
	// 真正的 supervisor 闭环由 TestSupervisorClosedLoop（stub runtime）覆盖。
	script := filepath.Join(t.TempDir(), "fake-supervisor.sh")
	writeTestFile(t, script, "#!/bin/sh\nsleep 30\n")
	if err := os.Chmod(script, 0o755); err != nil {
		t.Fatalf("改脚本权限失败：%v", err)
	}
	result, err := SpawnWorker("spawn", SpawnOptions{
		As: "implement", Provider: "claude",
		Cwd: t.TempDir(), TimeoutMs: 60000, SupervisorBinary: script,
	})
	if err != nil {
		t.Fatalf("SpawnWorker 失败：%v", err)
	}
	t.Cleanup(func() {
		if p, err := os.FindProcess(result.PID); err == nil {
			_ = p.Kill()
		}
	})
	if result.PID <= 0 {
		t.Fatalf("应返回子进程 pid，实际 %d", result.PID)
	}
	if result.Log == "" {
		t.Fatal("应返回 log 路径")
	}
	// .reservation 文件已写，含 channel 与 worker。
	resPath, _ := store.WorkerFile("spawn", "implement", "reservation", "test-proj")
	resData, err := os.ReadFile(resPath)
	if err != nil {
		t.Fatalf("应写 .reservation 文件：%v", err)
	}
	if !strings.Contains(string(resData), `"channel":"spawn"`) || !strings.Contains(string(resData), `"worker":"implement"`) {
		t.Fatalf(".reservation 应含 channel 与 worker：%s", string(resData))
	}
	// pid 文件已写且与返回一致。
	pidPath, _ := store.WorkerFile("spawn", "implement", "pid", "test-proj")
	pidData, err := os.ReadFile(pidPath)
	if err != nil {
		t.Fatalf("应写 pid 文件：%v", err)
	}
	if strings.TrimSpace(string(pidData)) != strconv.Itoa(result.PID) {
		t.Fatalf("pid 文件应为 %d，实际 %s", result.PID, string(pidData))
	}
	// supervisor 配置文件已写且能读回。
	cfgPath, _ := store.WorkerFile("spawn", "implement", "config", "test-proj")
	cfg := readSupervisorConfigFile(t, cfgPath)
	if cfg.Provider != "claude" {
		t.Fatalf("配置 provider 应为 claude，实际 %s", cfg.Provider)
	}
	_ = ref
	// 重复 spawn 同一 worker 应报错（pid 仍存活）。
	if _, err := SpawnWorker("spawn", SpawnOptions{
		As: "implement", Provider: "claude",
		Cwd: t.TempDir(), TimeoutMs: 60000, SupervisorBinary: script,
	}); err == nil {
		t.Fatal("重复 spawn 存活 worker 应报错")
	}
}
