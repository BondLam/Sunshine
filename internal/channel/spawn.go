package channel

import (
	"context"
	"fmt"
	"strings"

	"github.com/BondLam/Sunshine/internal/channel/store"
)

// SpawnWorker 通过注入的 runtime 注册一个 worker。
//
// 流程：解析 channel → 调 runtime.Start 拉起 worker 进程 →
// 追加持久 spawned 事件（含 runtime 元数据与收件箱策略）→
// 重读事件投影返回 WorkerState。runtime 拥有进程拉起细节；
// core 拥有事件写盘与状态投影。
//
// 空闲语义：worker 注册后空闲等待第一条 send --to 才工作，
// spawn 本身不派活、不发消息；新 worker 的 pendingMessageCount
// 为 0，activity 为 idle。
func SpawnWorker(ctx context.Context, input SpawnWorkerOptions, runtime WorkerRuntime) (WorkerState, error) {
	ref, err := resolveRef(input.ChannelAddress, false)
	if err != nil {
		return WorkerState{}, err
	}
	if err := store.AssertSafeName(input.WorkerID, "worker"); err != nil {
		return WorkerState{}, err
	}
	if strings.TrimSpace(input.SystemPrompt) == "" {
		return WorkerState{}, fmt.Errorf("spawn worker %q 缺少 systemPrompt", input.WorkerID)
	}
	if input.Provider != "" && !ValidProvider(input.Provider) {
		return WorkerState{}, fmt.Errorf("provider %q 无效，应为 claude 或 codex", input.Provider)
	}
	policy, err := parseInboxPolicyOpt(input.InboxPolicy)
	if err != nil {
		return WorkerState{}, err
	}
	if runtime == nil {
		return WorkerState{}, fmt.Errorf("spawn worker %q 需要 WorkerRuntime，未提供", input.WorkerID)
	}
	spec := WorkerSpec{
		Channel:      ref,
		WorkerID:     input.WorkerID,
		Provider:     input.Provider,
		Agent:        input.Agent,
		Model:        input.Model,
		CWD:          cwdOrDefault(input.CWD),
		Resume:       input.Resume,
		Sandbox:      input.Sandbox,
		SystemPrompt: input.SystemPrompt,
		ContextFiles: input.ContextFiles,
		InboxPolicy:  policy,
		Env:          input.Env,
	}
	handle, err := runtime.Start(ctx, spec)
	if err != nil {
		return WorkerState{}, fmt.Errorf("拉起 worker %q 失败：%w", input.WorkerID, err)
	}
	provider := input.Provider
	if provider == "" {
		provider = handle.Provider
	}
	extra := map[string]any{
		"as":          input.WorkerID,
		"inboxPolicy": string(policy),
	}
	if provider != "" {
		extra["provider"] = provider
	}
	if handle.PID > 0 {
		extra["pid"] = handle.PID
	}
	if input.Agent != "" {
		extra["agent"] = input.Agent
	}
	partial := withExtra(newAppendable(EventKindSpawned, input.By, input.Origin, input.Meta), extra)
	if _, err := store.AppendEvent(ref, partial); err != nil {
		return WorkerState{}, err
	}
	registry, _, err := readRegistry(ref)
	if err != nil {
		return WorkerState{}, err
	}
	state := registry.FindWorker(input.WorkerID)
	if state == nil {
		// 不应发生：spawned 事件刚落盘。
		return WorkerState{}, fmt.Errorf("spawn worker %q 后在注册表中找不到该 worker", input.WorkerID)
	}
	return *state, nil
}
