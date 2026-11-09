package store

// ReduceChannelMetadata 从事件流投影 channel 元数据（唯一事实来源）：
//
//   - create 事件定 type、description、labels 与初始 context；
//   - 旧值 type:"thread"/"threads" 不归一化为 forum，一律投影为 chat，
//     使 forum/thread API 拒绝改名前的旧 channel；
//   - kind:"context"、target:"channel" 的 add/delete 投影；
//   - kind:"channel"、action:"title" 的 set/clear 投影；
//   - 旧字段 linkedContext 与 context 同等对待。
func ReduceChannelMetadata(events []ChannelEvent) ChannelMetadata {
	typ := ChannelTypeChat
	var description string
	var labels []string
	var title string
	hasDescription, hasLabels, hasTitle := false, false, false
	contextMap := map[string]ContextEntry{}
	var contextOrder []string

	addEntries := func(entries []ContextEntry) {
		for _, entry := range entries {
			key := ContextEntryKey(entry)
			if _, ok := contextMap[key]; !ok {
				contextOrder = append(contextOrder, key)
			}
			contextMap[key] = entry
		}
	}
	deleteEntries := func(entries []ContextEntry) {
		for _, entry := range entries {
			key := ContextEntryKey(entry)
			if _, ok := contextMap[key]; ok {
				delete(contextMap, key)
				for i, k := range contextOrder {
					if k == key {
						contextOrder = append(contextOrder[:i], contextOrder[i+1:]...)
						break
					}
				}
			}
		}
	}

	for _, ev := range events {
		if ev.IsCreateEvent() {
			typ = normalizeChannelType(ev.StrField("type"))
			if v := ev.StrField("description"); v != "" {
				description = v
				hasDescription = true
			}
			if v := AsStringArray(ev.Extra["labels"]); v != nil {
				labels = v
				hasLabels = true
			}
			initial := AsContextEntries(ev.Extra["context"])
			if initial == nil {
				initial = AsContextEntries(ev.Extra["linkedContext"])
			}
			contextMap = map[string]ContextEntry{}
			contextOrder = nil
			addEntries(initial)
			continue
		}
		if ev.IsContextEvent() && ContextTarget(ev.StrField("target")) == ContextTargetChannel {
			entries := AsContextEntries(ev.Extra["context"])
			switch ContextMutationAction(ev.StrField("action")) {
			case ContextMutationAdd:
				addEntries(entries)
			case ContextMutationDelete:
				deleteEntries(entries)
			}
			continue
		}
		if ev.IsChannelMetadataEvent() && ev.StrField("action") == "title" {
			next, hasNext := ev.Extra["title"]
			if s, ok := next.(string); ok && s != "" {
				title = s
				hasTitle = true
			} else if hasNext && (next == nil || next == "") {
				title = ""
				hasTitle = false
			}
			continue
		}
	}

	meta := ChannelMetadata{Type: typ}
	if hasTitle {
		meta.Title = title
	}
	if hasDescription {
		meta.Description = description
	}
	if len(contextOrder) > 0 {
		for _, key := range contextOrder {
			meta.Context = append(meta.Context, contextMap[key])
		}
	}
	if hasLabels {
		meta.Labels = labels
	}
	return meta
}

// MetadataFromCreateEvent 由单个 create 事件投影元数据（兼容旧调用方）；
// 需要 context add/delete 与 title set/clear 时请用 ReduceChannelMetadata。
func MetadataFromCreateEvent(create *ChannelEvent) ChannelMetadata {
	if create == nil || !create.IsCreateEvent() {
		return ChannelMetadata{Type: ChannelTypeChat}
	}
	return ReduceChannelMetadata([]ChannelEvent{*create})
}

// normalizeChannelType 只认 "forum"，其余一律为 chat：
// 旧值 "thread"/"threads" 故意不升级为 forum。
func normalizeChannelType(value string) ChannelType {
	if value == string(ChannelTypeForum) {
		return ChannelTypeForum
	}
	return ChannelTypeChat
}
