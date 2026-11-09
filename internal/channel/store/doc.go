// Package store 是 Sunshine Channel 的事件溯源存储层（唯一一份）。
//
// 每个 channel 对应一个目录：
//
//	~/.sunshine/channels/<projectKey>/<channel>/events.jsonl
//	~/.sunshine/channels/<projectKey>/<channel>/.seq
//
// 每行一个 JSON 事件（见 ChannelEvent），.seq sidecar 存最后一条
// 事件的 seq。所有写操作都在 channel 文件锁内执行（见 lock.go），
// 保证多进程并发追加时 seq 连续、无撕裂行。
//
// 本包只做存储与纯函数投影（worker 注册表、channel 元数据、
// thread 状态、inbox 策略、投递分类），不碰 CLI、supervisor、
// forum 命令与 worker 适配器；那些由上层组在 store 之上实现。
//
// 存储根默认 ~/.sunshine/channels/，环境变量
// SUNSHINE_CHANNEL_ROOT 可覆盖。projectKey 由 cwd 按 Claude Code
// 约定生成（/、\、_ → -，其余非法字符 → -）；global 桶固定 _global。
// Sunshine 是全新产品，无旧版扁平布局，不实现任何旧布局迁移。
package store
