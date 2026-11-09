package store

import (
	"sort"
	"strings"
)

// ThreadState 是单个 thread 的投影状态。
type ThreadState struct {
	Thread      string
	Title       string
	Status      string
	Labels      []string
	Assignees   []string
	Description string
	Context     []ContextEntry
	Summary     string
	OpenedAt    string
	UpdatedAt   string
	LastSeq     int64
	Comments    int
	// Aliases 是该 thread 改名前的旧键。
	Aliases []string
}

// ThreadAliasResolver 把任意历史 thread 键解析为当前键。
type ThreadAliasResolver struct {
	aliasToCurrent   map[string]string
	aliasesByCurrent map[string]map[string]bool
}

func (r *ThreadAliasResolver) currentFor(key string) string {
	cur := key
	if v, ok := r.aliasToCurrent[key]; ok {
		cur = v
	}
	seen := map[string]bool{}
	for {
		next, ok := r.aliasToCurrent[cur]
		if !ok || seen[cur] {
			break
		}
		seen[cur] = true
		cur = next
	}
	return cur
}

// Resolve 把任意（当前或历史）thread 键解析为当前键。
func (r *ThreadAliasResolver) Resolve(key string) string {
	return r.currentFor(key)
}

// AliasesFor 返回某当前键的全部历史别名（不含自身）。
func (r *ThreadAliasResolver) AliasesFor(currentKey string) []string {
	set := r.aliasesByCurrent[currentKey]
	if len(set) == 0 {
		return nil
	}
	out := make([]string, 0, len(set))
	for alias := range set {
		out = append(out, alias)
	}
	sort.Strings(out)
	return out
}

// BuildThreadAliasResolver 从事件流构造别名解析器：只处理
// action:"rename" 的 thread 事件，链式改名会被压平。
func BuildThreadAliasResolver(events []ChannelEvent) *ThreadAliasResolver {
	r := &ThreadAliasResolver{
		aliasToCurrent:   map[string]string{},
		aliasesByCurrent: map[string]map[string]bool{},
	}
	for _, ev := range events {
		if !ev.IsThreadEvent() || ThreadAction(ev.StrField("action")) != ThreadActionRename {
			continue
		}
		newKey := strings.TrimSpace(ev.StrField("newThread"))
		oldKey := ev.StrField("thread")
		if newKey == "" || oldKey == "" || newKey == oldKey {
			continue
		}
		oldCurrent := r.currentFor(oldKey)
		targetCurrent := r.currentFor(newKey)
		if oldCurrent == targetCurrent {
			continue
		}
		moving := r.aliasesByCurrent[oldCurrent]
		if moving == nil {
			moving = map[string]bool{}
		}
		moving[oldCurrent] = true
		delete(r.aliasesByCurrent, oldCurrent)
		targetAliases := r.aliasesByCurrent[targetCurrent]
		if targetAliases == nil {
			targetAliases = map[string]bool{}
		}
		for alias := range moving {
			if alias != targetCurrent {
				targetAliases[alias] = true
			}
			r.aliasToCurrent[alias] = targetCurrent
		}
		r.aliasesByCurrent[targetCurrent] = targetAliases
	}
	return r
}

type threadInternalState struct {
	ThreadState
	contextMap   map[string]ContextEntry
	contextOrder []string
}

// ReduceThreads 从事件流投影全部 thread 状态，按 updatedAt 倒序返回。
// thread 状态/标签/负责人/摘要均以事件投影；rename 由别名解析器处理。
func ReduceThreads(events []ChannelEvent) []ThreadState {
	resolver := BuildThreadAliasResolver(events)
	states := map[string]*threadInternalState{}

	ensure := func(key string, seq int64) *threadInternalState {
		if s, ok := states[key]; ok {
			return s
		}
		s := &threadInternalState{
			ThreadState: ThreadState{
				Thread:    key,
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
		if ev.IsThreadEvent() {
			current := resolver.Resolve(ev.StrField("thread"))
			state := ensure(current, ev.Seq)
			if ev.Ts != "" {
				state.UpdatedAt = ev.Ts
			}
			if state.OpenedAt == "" && ev.Ts != "" {
				state.OpenedAt = ev.Ts
			}
			state.LastSeq = ev.Seq
			applyThreadAction(state, ev)
			continue
		}
		if ev.IsContextEvent() && ContextTarget(ev.StrField("target")) == ContextTargetThread && ev.StrField("thread") != "" {
			current := resolver.Resolve(ev.StrField("thread"))
			state, ok := states[current]
			if !ok {
				continue
			}
			entries := AsContextEntries(ev.Extra["context"])
			if entries == nil {
				continue
			}
			switch ContextMutationAction(ev.StrField("action")) {
			case ContextMutationAdd:
				for _, entry := range entries {
					key := ContextEntryKey(entry)
					if _, ok := state.contextMap[key]; !ok {
						state.contextOrder = append(state.contextOrder, key)
					}
					state.contextMap[key] = entry
				}
			case ContextMutationDelete:
				for _, entry := range entries {
					key := ContextEntryKey(entry)
					delete(state.contextMap, key)
					for i, k := range state.contextOrder {
						if k == key {
							state.contextOrder = append(state.contextOrder[:i], state.contextOrder[i+1:]...)
							break
						}
					}
				}
			}
			if ev.Ts != "" {
				state.UpdatedAt = ev.Ts
			}
			state.LastSeq = ev.Seq
			continue
		}
	}

	out := make([]ThreadState, 0, len(states))
	for currentKey, state := range states {
		result := state.ThreadState
		result.Aliases = resolver.AliasesFor(currentKey)
		if len(state.contextOrder) > 0 {
			for _, key := range state.contextOrder {
				result.Context = append(result.Context, state.contextMap[key])
			}
		}
		out = append(out, result)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].UpdatedAt > out[j].UpdatedAt })
	return out
}

func applyThreadAction(state *threadInternalState, ev ChannelEvent) {
	switch ThreadAction(ev.StrField("action")) {
	case ThreadActionOpened:
		if v := ev.StrField("status"); v != "" {
			state.Status = v
		} else {
			state.Status = "open"
		}
		if v := ev.StrField("title"); v != "" {
			state.Title = v
		}
		if v := ev.StrField("description"); v != "" {
			state.Description = v
		}
		initial := AsContextEntries(ev.Extra["context"])
		if initial == nil {
			initial = AsContextEntries(ev.Extra["linkedContext"])
		}
		if initial != nil {
			state.contextMap = map[string]ContextEntry{}
			state.contextOrder = nil
			for _, entry := range initial {
				key := ContextEntryKey(entry)
				state.contextMap[key] = entry
				state.contextOrder = append(state.contextOrder, key)
			}
		}
		if v := AsStringArray(ev.Extra["labels"]); v != nil {
			state.Labels = v
		}
		if v := AsStringArray(ev.Extra["assignees"]); v != nil {
			state.Assignees = v
		}
	case ThreadActionComment:
		state.Comments++
	case ThreadActionStatus:
		if v := ev.StrField("status"); v != "" {
			state.Status = v
		}
	case ThreadActionLabels:
		if v := AsStringArray(ev.Extra["labels"]); v != nil {
			state.Labels = v
		}
	case ThreadActionAssignees:
		if v := AsStringArray(ev.Extra["assignees"]); v != nil {
			state.Assignees = v
		}
	case ThreadActionSummary:
		if v := ev.StrField("summary"); v != "" {
			state.Summary = v
		}
	case ThreadActionProcessed:
		if v := ev.StrField("status"); v != "" {
			state.Status = v
		} else {
			state.Status = "processed"
		}
	case ThreadActionRename:
		// 改名由别名解析器处理，此处无事可做。
	}
}

// CollectThreadTimeline 返回属于某 thread（或其任一改名别名）的
// 时间线事件，按 seq 升序：thread 事件与 thread 级 context 事件。
func CollectThreadTimeline(events []ChannelEvent, threadKey string) []ChannelEvent {
	resolver := BuildThreadAliasResolver(events)
	current := resolver.Resolve(threadKey)
	aliases := map[string]bool{current: true}
	for _, alias := range resolver.AliasesFor(current) {
		aliases[alias] = true
	}
	var out []ChannelEvent
	for _, ev := range events {
		if ev.IsThreadEvent() {
			if aliases[ev.StrField("thread")] {
				out = append(out, ev)
			}
			continue
		}
		if ev.IsContextEvent() && ContextTarget(ev.StrField("target")) == ContextTargetThread && ev.StrField("thread") != "" {
			if aliases[ev.StrField("thread")] {
				out = append(out, ev)
			}
		}
	}
	return out
}
