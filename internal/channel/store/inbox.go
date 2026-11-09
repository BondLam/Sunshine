package store

// MatchesInboxPolicy 判断一条 channel message 事件是否应投递到
// 指定 worker 的收件箱（worker 注册表投影、投递与 supervisor
// 收件箱监听共用的唯一判断）：
//
//   - explicitOnly：仅当 to 指向该 worker；
//   - broadcastAndExplicit：另消费无 to 的广播消息。
//
// 只对 kind:"message" 生效；worker 永不消费自己发的消息。
func MatchesInboxPolicy(ev ChannelEvent, workerID string, policy InboxPolicy) bool {
	if ev.Kind != KindMessage {
		return false
	}
	if ev.By == workerID {
		return false
	}
	if len(ev.To) > 0 {
		for _, target := range ev.To {
			if target == workerID {
				return true
			}
		}
		return false
	}
	// 广播（无 to）。
	return policy == InboxPolicyBroadcastAndExplicit
}
