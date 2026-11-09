// Package channel 是多 Agent 协同的 core API 层。
//
// 本包只负责 API 语义：建 channel、发消息、读事件、监听、worker 注册与
// 投影、中断、上下文、标题，以及 forum 的 thread 操作。持久化全部走
// internal/channel/store 子包（A 组实现，唯一的事件溯源存储）；
// 本包的 FileStore 适配器（forum_store.go）与本 API 层共享同一份
// store，只做 channel 事件与 store 事件之间的 JSON 转换。
//
// 事件模型与上游 core 的 channel API 对齐：21 种事件 kind、
// ChannelRef / ChannelMetadata / WorkerState / DeliveryMode /
// InboxPolicy 的字段与语义均按源码实现；与源码不一致处见 PLAN.md
// 的阶段 6 实施记录（B 组小节）。
//
// 测试用 StubRuntime 见 teststub.go；测试的存储根用环境变量
// SUNSHINE_CHANNEL_ROOT 指向临时目录（与 store 包自身的测试做法一致）。
package channel
