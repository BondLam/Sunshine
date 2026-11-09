// Thread 状态投影：纯函数，只从事件日志折叠，不读 pid 文件、不读本地游标。
// 操作对象是 API 层的 ChannelEvent（见 types.go）；
// A 组 store 包里另有一份同形投影（store/thread.go，操作 store.ChannelEvent），
// 两份投影的语义一致，合并时由总协调人统一。
package channel

import (
	"sort"
)

// ThreadState 是一个 thread 折叠后的当前状态。
// ID 是当前规范键（json 键沿用上游的 "thread"，以便 --raw 输出与上游一致）；
// Processed 表示状态是否为 processed（processed 动作把状态置为 processed）。
type ThreadState struct {
	ID          string         `json:"thread"`
	Title       string         `json:"title,omitempty"`
	Status      string         `json:"status"`
	Labels      []string       `json:"labels"`
	Assignees   []string       `json:"assignees"`
	Description string         `json:"description,omitempty"`
	Context     []ContextEntry `json:"context,omitempty"`
	Summary     string         `json:"summary,omitempty"`
	Processed   bool           `json:"processed"`
	CreatedAt   string         `json:"openedAt,omitempty"`
	UpdatedAt   string         `json:"updatedAt,omitempty"`
	LastSeq     int64          `json:"lastSeq"`
	Comments    int            `json:"comments"`
	Aliases     []string       `json:"aliases"`
}

// ThreadAliasResolver 把任意（当前或历史）thread 键解析为当前规范键，
// 并给出某当前键的全部历史别名。
type ThreadAliasResolver struct {
	aliasToCurrent   map[string]string
	aliasesByCurrent map[string][]string
}

// Resolve 把键解析为当前规范键（无别名时返回原键）。
func (r *ThreadAliasResolver) Resolve(key string) string {
	cur := key
	if v, ok := r.aliasToCurrent[key]; ok {
		cur = v
	}
	seen := map[string]bool{}
	for {
		next, ok := r.aliasToCurrent[cur]
		if !ok || seen[cur] {
			return cur
		}
		seen[cur] = true
		cur = next
	}
}

// AliasesFor 返回某当前键的历史别名（不含自身）。
func (r *ThreadAliasResolver) AliasesFor(currentKey string) []string {
	if v, ok := r.aliasesByCurrent[currentKey]; ok {
		out := make([]string, len(v))
		copy(out, v)
		return out
	}
	return nil
}

// BuildThreadAliasResolver 从事件流构建别名解析器，
// 把 rename 链展平（同一键多次改名时链上全部旧键都指向最终键）。
func BuildThreadAliasResolver(events []ChannelEvent) *ThreadAliasResolver {
	aliasToCurrent := map[string]string{}
	aliasesByCurrent := map[string]map[string]bool{}

	currentFor := func(key string) string {
		cur, ok := aliasToCurrent[key]
		if !ok {
			return key
		}
		seen := map[string]bool{}
		for {
			next, ok := aliasToCurrent[cur]
			if !ok || seen[cur] {
				return cur
			}
			seen[cur] = true
			cur = next
		}
	}

	for _, ev := range events {
		if !IsThreadEvent(ev) || ev.Action != "rename" {
			continue
		}
		newKey := ev.NewThread
		oldKey := ev.Thread
		if newKey == "" || oldKey == "" || newKey == oldKey {
			continue
		}
		oldCurrent := currentFor(oldKey)
		targetCurrent := currentFor(newKey)
		if oldCurrent == targetCurrent {
			continue
		}
		// 把以 oldCurrent 为根的别名组整体迁移到 targetCurrent。
		moving := aliasesByCurrent[oldCurrent]
		if moving == nil {
			moving = map[string]bool{}
		}
		moving[oldCurrent] = true
		delete(aliasesByCurrent, oldCurrent)

		target := aliasesByCurrent[targetCurrent]
		if target == nil {
			target = map[string]bool{}
		}
		for alias := range moving {
			if alias == targetCurrent {
				continue
			}
			target[alias] = true
			aliasToCurrent[alias] = targetCurrent
		}
		aliasesByCurrent[targetCurrent] = target
	}

	aliases := make(map[string][]string, len(aliasesByCurrent))
	for current, set := range aliasesByCurrent {
		list := make([]string, 0, len(set))
		for alias := range set {
			list = append(list, alias)
		}
		sort.Strings(list)
		aliases[current] = list
	}
	return &ThreadAliasResolver{aliasToCurrent: aliasToCurrent, aliasesByCurrent: aliases}
}

// IsThreadEvent 判断事件是否为 thread 事件（kind 为 thread 且带 thread 键）。
func IsThreadEvent(ev ChannelEvent) bool {
	return ev.Kind == EventKindThread && ev.Thread != ""
}

// IsContextEvent 判断事件是否为 context 事件。
func IsContextEvent(ev ChannelEvent) bool {
	return ev.Kind == EventKindContext
}

// contextTarget 取 context 事件的 target（API 层事件的具名字段）。
func contextTarget(ev ChannelEvent) string {
	return ev.Target
}

type threadInternal struct {
	state      ThreadState
	contextMap map[string]ContextEntry
}

// ReduceThreads 把事件流折叠为 thread 状态列表，按更新时间倒序。
// 状态、标签、负责人、摘要、上下文均以事件投影；rename 只走别名解析。
func ReduceThreads(events []ChannelEvent) []ThreadState {
	resolver := BuildThreadAliasResolver(events)
	states := map[string]*threadInternal{}

	ensure := func(key string, seq int64) *threadInternal {
		if s, ok := states[key]; ok {
			return s
		}
		s := &threadInternal{
			state: ThreadState{
				ID:        key,
				Status:    "open",
				Labels:    []string{},
				Assignees: []string{},
				LastSeq:   seq,
			},
			contextMap: map[string]ContextEntry{},
		}
		states[key] = s
		return s
	}

	for _, ev := range events {
		if IsThreadEvent(ev) {
			current := resolver.Resolve(ev.Thread)
			st := ensure(current, ev.Seq)
			if ev.TS != "" {
				st.state.UpdatedAt = ev.TS
			}
			if st.state.CreatedAt == "" && ev.TS != "" {
				st.state.CreatedAt = ev.TS
			}
			st.state.LastSeq = ev.Seq
			applyThreadAction(st, ev)
			continue
		}
		if IsContextEvent(ev) && contextTarget(ev) == "thread" && ev.Thread != "" {
			current := resolver.Resolve(ev.Thread)
			st, ok := states[current]
			if !ok {
				continue
			}
			entries := asContextEntries(ev.Context)
			if entries == nil {
				continue
			}
			switch ev.Action {
			case "add":
				for _, e := range entries {
					st.contextMap[ContextEntryKey(e)] = e
				}
			case "delete":
				for _, e := range entries {
					delete(st.contextMap, ContextEntryKey(e))
				}
			}
			if ev.TS != "" {
				st.state.UpdatedAt = ev.TS
			}
			st.state.LastSeq = ev.Seq
			continue
		}
	}

	out := make([]ThreadState, 0, len(states))
	for currentKey, st := range states {
		st.state.Aliases = resolver.AliasesFor(currentKey)
		if st.state.Aliases == nil {
			st.state.Aliases = []string{}
		}
		if len(st.contextMap) > 0 {
			ctx := make([]ContextEntry, 0, len(st.contextMap))
			for _, e := range st.contextMap {
				ctx = append(ctx, e)
			}
			st.state.Context = ctx
		}
		st.state.Processed = st.state.Status == "processed"
		out = append(out, st.state)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].UpdatedAt != out[j].UpdatedAt {
			return out[i].UpdatedAt > out[j].UpdatedAt
		}
		return out[i].ID < out[j].ID
	})
	return out
}

func applyThreadAction(st *threadInternal, ev ChannelEvent) {
	switch ev.Action {
	case "opened":
		if ev.Status != "" {
			st.state.Status = ev.Status
		} else {
			st.state.Status = "open"
		}
		if ev.Title != nil {
			st.state.Title = *ev.Title
		}
		if ev.Description != "" {
			st.state.Description = ev.Description
		}
		initial := asContextEntries(ev.Context)
		if initial == nil {
			initial = asContextEntries(ev.LinkedContext)
		}
		if initial != nil {
			st.contextMap = map[string]ContextEntry{}
			for _, e := range initial {
				st.contextMap[ContextEntryKey(e)] = e
			}
		}
		if ev.Labels != nil {
			st.state.Labels = ev.Labels
		}
		if ev.Assignees != nil {
			st.state.Assignees = ev.Assignees
		}
	case "comment":
		st.state.Comments++
	case "status":
		if ev.Status != "" {
			st.state.Status = ev.Status
		}
	case "labels":
		if ev.Labels != nil {
			st.state.Labels = ev.Labels
		}
	case "assignees":
		if ev.Assignees != nil {
			st.state.Assignees = ev.Assignees
		}
	case "summary":
		if ev.Summary != "" {
			st.state.Summary = ev.Summary
		}
	case "processed":
		if ev.Status != "" {
			st.state.Status = ev.Status
		} else {
			st.state.Status = "processed"
		}
	case "rename":
		// 改名只走别名解析，此处无事可做。
	}
}

// asContextEntries 只保留合法的上下文条目；无合法条目时返回 nil。
func asContextEntries(entries []ContextEntry) []ContextEntry {
	var out []ContextEntry
	for _, e := range entries {
		switch e.Type {
		case "file":
			if e.Path != "" {
				out = append(out, e)
			}
		case "raw":
			if e.Text != "" {
				out = append(out, e)
			}
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// CollectThreadTimeline 返回属于某 thread（或其任一改名别名）的事件，
// 按 seq 升序：thread 事件与 target 为 thread 的 context 事件。
func CollectThreadTimeline(events []ChannelEvent, threadKey string) []ChannelEvent {
	resolver := BuildThreadAliasResolver(events)
	current := resolver.Resolve(threadKey)
	aliases := map[string]bool{current: true}
	for _, a := range resolver.AliasesFor(current) {
		aliases[a] = true
	}
	var out []ChannelEvent
	for _, ev := range events {
		if IsThreadEvent(ev) {
			if aliases[ev.Thread] {
				out = append(out, ev)
			}
			continue
		}
		if IsContextEvent(ev) && contextTarget(ev) == "thread" && ev.Thread != "" {
			if aliases[ev.Thread] {
				out = append(out, ev)
			}
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Seq < out[j].Seq })
	return out
}
