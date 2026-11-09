package store

// MeaningfulEventKinds 是默认监听时唤醒的事件 kind 集合；
// 非 meaningful 的 kind（如 progress、waiting、supervisor_warning）
// 只有显式指定 kind 或 includeNonMeaningful 时才匹配。
var MeaningfulEventKinds = map[ChannelEventKind]bool{
	KindCreate: true, KindJoin: true, KindLeave: true, KindMessage: true,
	KindThread: true, KindContext: true, KindChannel: true, KindSpawned: true,
	KindKilled: true, KindRespawned: true, KindDone: true, KindError: true,
}

// ChannelEventFilter 是事件过滤条件（watch 与 wait 共用）。
type ChannelEventFilter struct {
	// From 只匹配 by 在该列表里的事件。
	From []string
	// Kind 只匹配这些 kind（单个或多个，OR 语义）；空表示不限。
	// 显式指定 kind 会绕过默认的 meaningful-kind 过滤。
	Kind []ChannelEventKind
	// To 只匹配 to 含该目标的事件；"exclusive" 表示只匹配定向事件。
	To string
	// Self 排除 by 等于该值的事件（不看自己的事件）。
	Self string
	// IncludeProgress 为 false 时不匹配 progress 事件。
	IncludeProgress bool
	// IncludeNonMeaningful 为 true 时不过滤非 meaningful kind。
	IncludeNonMeaningful bool
	// Thread 只匹配该 thread 的 thread 事件。
	Thread string
	// Action 只匹配该动作的 thread 事件。
	Action ThreadAction
}

// MatchesEventFilter 判断事件是否满足过滤条件。
func MatchesEventFilter(ev ChannelEvent, filter ChannelEventFilter) bool {
	if filter.Self != "" && ev.By == filter.Self {
		return false
	}
	hasExplicitKind := len(filter.Kind) > 0
	if !filter.IncludeNonMeaningful && !hasExplicitKind && !MeaningfulEventKinds[ev.Kind] {
		return false
	}
	if !filter.IncludeProgress && ev.Kind == KindProgress {
		return false
	}
	if len(filter.Kind) > 0 {
		matched := false
		for _, k := range filter.Kind {
			if ev.Kind == k {
				matched = true
				break
			}
		}
		if !matched {
			return false
		}
	}
	if filter.Thread != "" {
		if !ev.IsThreadEvent() || ev.StrField("thread") != filter.Thread {
			return false
		}
	}
	if filter.Action != "" {
		if !ev.IsThreadEvent() || ThreadAction(ev.StrField("action")) != filter.Action {
			return false
		}
	}
	if len(filter.From) > 0 {
		matched := false
		for _, from := range filter.From {
			if ev.By == from {
				matched = true
				break
			}
		}
		if !matched {
			return false
		}
	}
	if filter.To != "" {
		if filter.To == "exclusive" {
			if len(ev.To) == 0 {
				return false
			}
		} else {
			if len(ev.To) == 0 {
				return true
			}
			matched := false
			for _, t := range ev.To {
				if t == filter.To {
					matched = true
					break
				}
			}
			if !matched {
				return false
			}
		}
	}
	return true
}
