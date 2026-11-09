package store

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"time"
)

// WatchOptions 是 WatchChannelEvents 的选项。
type WatchOptions struct {
	// FromStart 为 true 时从头读已有事件；否则默认只推新事件。
	FromStart bool
	// SinceSeq 只推 seq > SinceSeq 的事件（须与 HasSinceSeq 联用）。
	SinceSeq int64
	// HasSinceSeq 表示 SinceSeq 有效。
	HasSinceSeq bool
	// Filter 事件过滤条件。
	Filter ChannelEventFilter
	// PollInterval 轮询间隔，默认 200ms。
	PollInterval time.Duration
}

func (o WatchOptions) pollInterval() time.Duration {
	if o.PollInterval <= 0 {
		return 200 * time.Millisecond
	}
	return o.PollInterval
}

// WatchChannelEvents 以 goroutine + channel 实现流式推送：监听某
// channel 的 events.jsonl，有新事件且通过过滤时推送。默认只推
// 监听开始后的新事件；FromStart 或 SinceSeq 可回读历史。
// 上下文取消时关闭返回的两个 channel。
func WatchChannelEvents(ctx context.Context, ref ChannelRef, opts WatchOptions) (<-chan ChannelEvent, <-chan error) {
	out := make(chan ChannelEvent)
	errCh := make(chan error, 1)
	go func() {
		defer close(out)
		defer close(errCh)
		jsonl, err := EventsPath(ref.Name, ref.Project)
		if err != nil {
			select {
			case errCh <- err:
			case <-ctx.Done():
			}
			return
		}
		if _, err := EnsureChannelDir(ref.Name, ref.Project); err != nil {
			select {
			case errCh <- err:
			case <-ctx.Done():
			}
			return
		}
		var offset int64
		if !opts.FromStart && !opts.HasSinceSeq {
			if stat, err := os.Stat(jsonl); err == nil {
				offset = stat.Size()
			}
		}
		carry := []byte{}
		ticker := time.NewTicker(opts.pollInterval())
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
			}
			stat, err := os.Stat(jsonl)
			if err != nil {
				if os.IsNotExist(err) {
					continue
				}
				select {
				case errCh <- err:
					return
				case <-ctx.Done():
					return
				}
			}
			if stat.Size() < offset {
				offset = 0
				carry = carry[:0]
			}
			if stat.Size() <= offset {
				continue
			}
			f, err := os.Open(jsonl)
			if err != nil {
				continue
			}
			length := stat.Size() - offset
			buf := make([]byte, length)
			_, err = f.ReadAt(buf, offset)
			f.Close()
			if err != nil {
				continue
			}
			offset = stat.Size()
			data := append(carry, buf...)
			lines := bytes.Split(data, []byte("\n"))
			carry = lines[len(lines)-1]
			for _, line := range lines[:len(lines)-1] {
				if len(bytes.TrimSpace(line)) == 0 {
					continue
				}
				var ev ChannelEvent
				if err := json.Unmarshal(line, &ev); err != nil {
					continue
				}
				if opts.HasSinceSeq && ev.Seq <= opts.SinceSeq {
					continue
				}
				if !MatchesEventFilter(ev, opts.Filter) {
					continue
				}
				select {
				case out <- ev:
				case <-ctx.Done():
					return
				}
			}
		}
	}()
	return out, errCh
}

// ChannelCursor 是跨 channel 监听的游标：每 channel 一个 seq。
// 键格式为 "<scope>/<project>/<name>"（见 ChannelCursorKey）。
type ChannelCursor map[string]int64

// ChannelCursorKey 返回某 channel 在游标里的键。
func ChannelCursorKey(ref ChannelRef) string {
	return string(ref.Scope) + "/" + ref.Project + "/" + ref.Name
}

// CrossChannelEvent 是跨 channel 监听推送的一条事件，附带推送后
// 的全量游标快照，供调用方做 (channel, seq) 检查点。
type CrossChannelEvent struct {
	Channel ChannelRef
	Event   ChannelEvent
	Cursor  ChannelCursor
}

// WatchChannelsInput 是 WatchChannels 的输入。
type WatchChannelsInput struct {
	// ProjectKey 项目桶；为空时用当前项目桶。
	ProjectKey string
	// Global 为 true 时监听 global 桶。
	Global bool
	// Filter 事件过滤条件。
	Filter ChannelEventFilter
	// Cursor 每 channel 的续读游标。
	Cursor ChannelCursor
	// FromStartNewChannels 为 true 时，监听开始后新发现的 channel
	// 从 seq 0 读起；默认从末尾开始（只推新事件）。初始扫描到的
	// channel 一律读积压（除非给了游标，按游标续读）。
	FromStartNewChannels bool
	// DiscoveryInterval channel 发现轮询间隔，默认 500ms。
	DiscoveryInterval time.Duration
}

func (in WatchChannelsInput) discoveryInterval() time.Duration {
	if in.DiscoveryInterval <= 0 {
		return 500 * time.Millisecond
	}
	return in.DiscoveryInterval
}

type watchChannelState struct {
	ref    ChannelRef
	offset int64
	carry  []byte
}

// WatchChannels 跨 channel 监听：监听某项目桶（或 global 桶）内全部
// channel 的事件，合并成一条流；监听开始后新建的 channel 会被
// 动态发现。每个事件附带全量游标快照；投递为至少一次。
// 上下文取消时关闭返回的两个 channel。
func WatchChannels(ctx context.Context, input WatchChannelsInput) (<-chan CrossChannelEvent, <-chan error) {
	out := make(chan CrossChannelEvent)
	errCh := make(chan error, 1)
	go func() {
		defer close(out)
		defer close(errCh)
		project := input.ProjectKey
		scope := ScopeProject
		if input.Global {
			project = GlobalProjectKey
			scope = ScopeGlobal
		} else if project == "" {
			project = CurrentProjectKey()
		}
		cursor := ChannelCursor{}
		for k, v := range input.Cursor {
			cursor[k] = v
		}
		states := map[string]*watchChannelState{}
		ticker := time.NewTicker(input.discoveryInterval())
		defer ticker.Stop()
		// 初始扫描的 channel 读积压（从头开始）；之后才发现的 channel
		// 按 FromStartNewChannels 决定是否读积压，默认只从末尾推新事件。
		// 有游标的 channel 一律从头读起、按 seq 过滤。
		initialScan := true
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
			}
			// 发现新 channel。
			for _, name := range ListChannelNamesInProject(project) {
				dir, err := ChannelDir(name, project)
				if err != nil {
					continue
				}
				ref := ChannelRef{Name: name, Scope: scope, Project: project, Dir: dir}
				key := ChannelCursorKey(ref)
				if _, ok := states[key]; ok {
					continue
				}
				state := &watchChannelState{ref: ref}
				if _, ok := cursor[key]; ok {
					// 有游标：从头读起，按 seq 过滤。
					state.offset = 0
				} else if initialScan || input.FromStartNewChannels {
					state.offset = 0
				} else {
					jsonl, err := EventsPath(name, project)
					if err == nil {
						if stat, err := os.Stat(jsonl); err == nil {
							state.offset = stat.Size()
						}
					}
				}
				states[key] = state
			}
			initialScan = false
			// 轮询各 channel 的新事件。
			for key, state := range states {
				jsonl, err := EventsPath(state.ref.Name, project)
				if err != nil {
					continue
				}
				stat, err := os.Stat(jsonl)
				if err != nil {
					continue
				}
				if stat.Size() < state.offset {
					state.offset = 0
					state.carry = state.carry[:0]
				}
				if stat.Size() <= state.offset {
					continue
				}
				f, err := os.Open(jsonl)
				if err != nil {
					continue
				}
				length := stat.Size() - state.offset
				buf := make([]byte, length)
				_, err = f.ReadAt(buf, state.offset)
				f.Close()
				if err != nil {
					continue
				}
				state.offset = stat.Size()
				data := append(state.carry, buf...)
				lines := bytes.Split(data, []byte("\n"))
				state.carry = lines[len(lines)-1]
				for _, line := range lines[:len(lines)-1] {
					if len(bytes.TrimSpace(line)) == 0 {
						continue
					}
					var ev ChannelEvent
					if err := json.Unmarshal(line, &ev); err != nil {
						continue
					}
					if sinceSeq, ok := cursor[key]; ok && ev.Seq <= sinceSeq {
						continue
					}
					if !MatchesEventFilter(ev, input.Filter) {
						continue
					}
					cursor[key] = ev.Seq
					snapshot := ChannelCursor{}
					for k, v := range cursor {
						snapshot[k] = v
					}
					select {
					case out <- CrossChannelEvent{Channel: state.ref, Event: ev, Cursor: snapshot}:
					case <-ctx.Done():
						return
					}
				}
			}
		}
	}()
	return out, errCh
}
