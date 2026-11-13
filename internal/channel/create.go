package channel

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/linrong007/Sunshine/internal/channel/store"
)

// CreateChannel 建一个新 channel。已存在且未传 Force 时报错；
// Force 时先清理已有 channel 目录再建。
//
// 与上游 create.ts 的差异：--force 清理时先尝试结束目录里 .pid
// 文件指向的活进程（与上游 forceCleanChannel 一致），再删目录。
func CreateChannel(opts CreateChannelOptions) (ChannelEvent, error) {
	if err := store.AssertSafeName(opts.Channel, "channel"); err != nil {
		return ChannelEvent{}, err
	}
	channelType, err := store.ParseChannelType(opts.Type)
	if err != nil {
		return ChannelEvent{}, err
	}
	if len(opts.Context) > 0 {
		if err := ValidateContextEntries(opts.Context); err != nil {
			return ChannelEvent{}, err
		}
	}
	ref, err := resolveRef(opts.ChannelAddress, true)
	if err != nil {
		return ChannelEvent{}, err
	}
	eventsPath, err := store.EventsPath(opts.Channel, ref.Project)
	if err != nil {
		return ChannelEvent{}, err
	}
	if _, err := os.Stat(eventsPath); err == nil && !opts.Force {
		return ChannelEvent{}, fmt.Errorf("channel %q 已存在（%s），用 --force 覆盖", opts.Channel, ref.Dir)
	}
	if opts.Force {
		if err := forceCleanChannel(ref); err != nil {
			return ChannelEvent{}, err
		}
	}
	extra := map[string]any{
		"cwd":   cwdOrDefault(opts.CWD),
		"scope": string(ref.Scope),
		"type":  string(channelType),
	}
	if opts.Task != "" {
		extra["task"] = opts.Task
	}
	if opts.Project != "" {
		extra["project"] = opts.Project
	}
	if len(opts.Labels) > 0 {
		extra["labels"] = opts.Labels
	}
	if opts.Description != "" {
		extra["description"] = opts.Description
	}
	if len(opts.Context) > 0 {
		extra["context"] = opts.Context
	}
	if opts.Ephemeral {
		extra["ephemeral"] = true
	}
	partial := withExtra(newAppendable(EventKindCreate, opts.By, opts.Origin, opts.Meta), extra)
	ev, err := store.AppendEvent(ref, partial)
	if err != nil {
		return ChannelEvent{}, err
	}
	return fromStoreEvent(ev)
}

// forceCleanChannel 清理已有 channel 目录：先结束 .pid 文件指向的
// 活进程（SIGTERM，等 1.5 秒仍活着则 SIGKILL），再删整个目录。
func forceCleanChannel(ref ChannelRef) error {
	entries, err := os.ReadDir(ref.Dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".pid") {
			continue
		}
		data, err := os.ReadFile(filepath.Join(ref.Dir, entry.Name()))
		if err != nil {
			continue
		}
		pid, err := strconv.Atoi(strings.TrimSpace(string(data)))
		if err != nil || pid <= 0 {
			continue
		}
		if !pidAlive(pid) {
			continue
		}
		_ = signalProcess(pid, syscall.SIGTERM)
		deadline := time.Now().Add(1500 * time.Millisecond)
		for time.Now().Before(deadline) {
			if !pidAlive(pid) {
				break
			}
			time.Sleep(50 * time.Millisecond)
		}
		if pidAlive(pid) {
			_ = signalProcess(pid, syscall.SIGKILL)
		}
	}
	if err := os.RemoveAll(ref.Dir); err != nil {
		return fmt.Errorf("清理已有 channel 目录失败：%w", err)
	}
	return nil
}
