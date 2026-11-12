package channel

import (
	"context"
	"encoding/json"
	"errors"
	"os/exec"
	"strings"
	"testing"

	"github.com/BondLam/Sunshine/internal/channel/store"
)

// errTestSpawn 与 errTestInterrupt 是注入 runtime 失败用的测试错误。
var (
	errTestSpawn     = errors.New("测试用启动失败")
	errTestInterrupt = errors.New("测试用中断失败")
)

// testSetup 为每个测试隔离存储根。
func testSetup(t *testing.T) {
	t.Helper()
	root := t.TempDir()
	t.Setenv("SUNSHINE_CHANNEL_ROOT", root)
	t.Setenv("SUNSHINE_CHANNEL_PROJECT", "")
}

// testAddr 返回固定 testproj 分桶的寻址参数（不依赖 cwd）。
func testAddr(name string) ChannelAddress {
	return ChannelAddress{Channel: name, ProjectKey: "testproj"}
}

// testCreate 建一个默认 chat channel。
func testCreate(t *testing.T, name string) ChannelEvent {
	t.Helper()
	ev, err := CreateChannel(CreateChannelOptions{ChannelAddress: testAddr(name), By: "main"})
	if err != nil {
		t.Fatalf("建 channel 失败：%v", err)
	}
	return ev
}

// testRef 返回测试 channel 的 store 引用（供直接追加模拟事件用）。
func testRef(name string) store.ChannelRef {
	dir, _ := store.ChannelDir(name, "testproj")
	return store.ChannelRef{Name: name, Scope: store.ScopeProject, Project: "testproj", Dir: dir}
}

// appendStoreEvent 直接经 store 追加一条事件（模拟 supervisor/worker 写盘）。
func appendStoreEvent(t *testing.T, name string, partial store.AppendableEvent) store.ChannelEvent {
	t.Helper()
	ev, err := store.AppendEvent(testRef(name), partial)
	if err != nil {
		t.Fatalf("追加事件失败：%v", err)
	}
	return ev
}

// testSpawn 用 StubRuntime 注册一个 worker。
func testSpawn(t *testing.T, ctx context.Context, name, workerID string, runtime *StubRuntime) WorkerState {
	t.Helper()
	state, err := SpawnWorker(ctx, SpawnWorkerOptions{
		ChannelAddress: testAddr(name),
		By:             "main",
		WorkerID:       workerID,
		Provider:       "claude",
		SystemPrompt:   "你是测试 worker。",
	}, runtime)
	if err != nil {
		t.Fatalf("spawn worker 失败：%v", err)
	}
	return state
}

// deadPID 返回一个已退出进程的 pid（用于模拟死亡的 supervisor）。
func deadPID(t *testing.T) int {
	t.Helper()
	cmd := exec.Command("sleep", "0.01")
	if err := cmd.Start(); err != nil {
		t.Fatalf("启动 sleep 失败：%v", err)
	}
	pid := cmd.Process.Pid
	_ = cmd.Wait()
	return pid
}

func int64Ptr(v int64) *int64 { return &v }
func intPtr(v int) *int       { return &v }

// storeAppendableThread 造一个 kind thread、action opened 的开贴事件输入。
func storeAppendableThread(thread string) store.AppendableEvent {
	return store.AppendableEvent{
		Kind: store.KindThread, By: "main",
		Extra: map[string]any{
			"action": "opened",
			"thread": thread,
			"title":  "主题 " + thread,
		},
	}
}

func jsonUnmarshal(data []byte, v any) error { return json.Unmarshal(data, v) }
func jsonMarshal(v any) ([]byte, error)      { return json.Marshal(v) }
func contains(s, sub string) bool            { return strings.Contains(s, sub) }
