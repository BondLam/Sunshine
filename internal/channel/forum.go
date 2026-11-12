// Forum API：PostThread、RenameThread、ListForumThreads、ShowThread。
// 行为与上游源码 post-thread.ts / read.ts 对照，
// 仅 forum 类型的 channel 可用（chat 类型调用时报错）。
// 持久化走 ForumStore（见 forum_store.go），类型复用 types.go 的
// ChannelEvent / ChannelMetadata / BuildContextEntries，
// thread 键规范复用 store.NormalizeThreadKey。
package channel

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/BondLam/Sunshine/internal/channel/store"
)

// ThreadAction 是 post 支持的六种动作（rename 另走 RenameThread）。
var validThreadActions = map[string]bool{
	"opened": true, "comment": true, "status": true,
	"labels": true, "assignees": true, "summary": true, "processed": true,
}

// ValidThreadActions 返回 post 支持的六种动作（固定顺序，供报错与帮助使用）。
func ValidThreadActions() []string {
	return []string{"opened", "comment", "status", "labels", "assignees", "summary", "processed"}
}

// ParseThreadAction 校验 thread 动作：六种 post 动作加 rename。
func ParseThreadAction(v string) (string, error) {
	if validThreadActions[v] || v == "rename" {
		return v, nil
	}
	return "", fmt.Errorf("无效的 thread 动作 %q，应为：opened、comment、status、labels、assignees、summary、processed、rename", v)
}

// ParseScope 校验 --scope：空（自动解析）、project、global。
func ParseScope(v string) (string, error) {
	switch v {
	case "", "project", "global":
		return v, nil
	default:
		return "", fmt.Errorf("无效的 --scope %q，应为 project 或 global", v)
	}
}

// NormalizeThreadKey 校验并规范 thread 键（复用 A 组 store 的实现）。
func NormalizeThreadKey(v string) (string, error) {
	return store.NormalizeThreadKey(v)
}

// ParseCSV 把逗号分隔串解析为字符串数组；空输入返回 nil。
func ParseCSV(v string) []string {
	if strings.TrimSpace(v) == "" {
		return nil
	}
	var out []string
	for _, part := range strings.Split(v, ",") {
		if p := strings.TrimSpace(part); p != "" {
			out = append(out, p)
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// ReduceChannelMetadata 从事件流折叠频道元数据。
// 旧日志里的 "thread"/"threads" 类型不归一化为 forum，一律投影为 chat，
// 因此旧频道上的 forum 操作会被拒绝（与上游一致）。
func ReduceChannelMetadata(events []ChannelEvent) ChannelMetadata {
	meta := ChannelMetadata{Type: ChannelTypeChat}
	for _, ev := range events {
		if ev.Kind != EventKindCreate {
			continue
		}
		if ev.Type == "forum" {
			meta.Type = ChannelTypeForum
		} else {
			meta.Type = ChannelTypeChat
		}
		if ev.Description != "" {
			meta.Description = ev.Description
		}
		if ev.Title != nil {
			meta.Title = *ev.Title
		}
		if ev.Labels != nil {
			meta.Labels = ev.Labels
		}
	}
	return meta
}

// ReadForumChannelEvents 读某频道的全部事件，并要求其为 forum 类型；
// 不是 forum（chat、或旧日志的 thread/threads、或尚无 create 事件）时报错。
func ReadForumChannelEvents(storeImpl ForumStore, channel, project, operation string) ([]ChannelEvent, error) {
	events, err := storeImpl.ReadChannelEvents(channel, project, Pagination{})
	if err != nil {
		return nil, err
	}
	meta := ReduceChannelMetadata(events)
	if meta.Type != ChannelTypeForum {
		return nil, fmt.Errorf("频道 %q 的类型是 %s，%s 操作只支持 forum 类型的频道", channel, meta.Type, operation)
	}
	return events, nil
}

// PostThreadOptions 是 PostThread 的参数；
// 空字符串/ nil 切片表示该字段不写（opened 时 Thread 为空则自动生成）。
type PostThreadOptions struct {
	Channel string
	Scope   string // project | global，空为自动解析
	By      string
	Action  string
	Thread  string

	Title       string
	Text        string
	Description string
	Status      string
	Labels      []string
	Assignees   []string
	Summary     string
	Context     []ContextEntry

	IdempotencyKey string
	Origin         string // cli | api | worker，空为不写
}

// PostThread 向 forum 频道追加一个结构化 thread 事件。
// 非 opened 动作缺 --thread 时报错；action 非法或频道不是 forum 时报错。
func PostThread(storeImpl ForumStore, opts PostThreadOptions) (ChannelEvent, error) {
	if !validThreadActions[opts.Action] {
		return ChannelEvent{}, fmt.Errorf("无效的 thread 动作 %q，应为：%s", opts.Action, strings.Join(ValidThreadActions(), "、"))
	}
	ref, err := ResolveChannel(ChannelAddress{Channel: opts.Channel, Scope: opts.Scope})
	if err != nil {
		return ChannelEvent{}, err
	}
	if _, err := ReadForumChannelEvents(storeImpl, opts.Channel, ref.Project, "post"); err != nil {
		return ChannelEvent{}, err
	}
	thread, err := resolveThreadKey(opts.Action, opts.Thread)
	if err != nil {
		return ChannelEvent{}, err
	}
	partial := map[string]any{
		"kind":   "thread",
		"by":     opts.By,
		"action": opts.Action,
		"thread": thread,
	}
	if opts.IdempotencyKey != "" {
		partial["idempotencyKey"] = opts.IdempotencyKey
	}
	if opts.Title != "" {
		partial["title"] = opts.Title
	}
	if opts.Text != "" {
		partial["text"] = opts.Text
	}
	if opts.Description != "" {
		partial["description"] = opts.Description
	}
	if opts.Status != "" {
		partial["status"] = opts.Status
	}
	if opts.Labels != nil {
		partial["labels"] = opts.Labels
	}
	if opts.Assignees != nil {
		partial["assignees"] = opts.Assignees
	}
	if opts.Summary != "" {
		partial["summary"] = opts.Summary
	}
	if len(opts.Context) > 0 {
		partial["context"] = opts.Context
	}
	if opts.Origin != "" {
		partial["origin"] = opts.Origin
	}
	return storeImpl.AppendEvent(opts.Channel, ref.Project, partial)
}

// resolveThreadKey：给了 thread 键就规范化；没给且动作为 opened 则自动生成
// thread-<毫秒时间戳36进制>；没给且为其他动作则报错。
func resolveThreadKey(action, value string) (string, error) {
	if value != "" {
		return NormalizeThreadKey(value)
	}
	if action == "opened" {
		return "thread-" + strconv.FormatInt(time.Now().UnixMilli(), 36), nil
	}
	return "", fmt.Errorf("除 opened 外，--thread 必填")
}

// RenameThreadOptions 是 RenameThread 的参数。
type RenameThreadOptions struct {
	Channel   string
	Scope     string // project | global，空为自动解析
	By        string
	Thread    string // 旧名（或任一别名）
	NewThread string // 新名
	Origin    string // cli | api | worker，空为不写
}

// RenameThread 重命名一个 thread：追加 kind 为 thread、action 为 rename 的事件，
// 后续投影通过别名解析把旧名展平到新名。
// 旧名不存在、旧名新名相同、或新名已是另一条 thread 时报错（拒绝静默合并两条时间线）。
func RenameThread(storeImpl ForumStore, opts RenameThreadOptions) (ChannelEvent, error) {
	ref, err := ResolveChannel(ChannelAddress{Channel: opts.Channel, Scope: opts.Scope})
	if err != nil {
		return ChannelEvent{}, err
	}
	events, err := ReadForumChannelEvents(storeImpl, opts.Channel, ref.Project, "thread rename")
	if err != nil {
		return ChannelEvent{}, err
	}
	oldKey, err := NormalizeThreadKey(opts.Thread)
	if err != nil {
		return ChannelEvent{}, err
	}
	newKey, err := NormalizeThreadKey(opts.NewThread)
	if err != nil {
		return ChannelEvent{}, err
	}
	if oldKey == newKey {
		return ChannelEvent{}, fmt.Errorf("旧 thread 名与新 thread 名相同")
	}
	resolver := BuildThreadAliasResolver(events)
	oldCurrent := resolver.Resolve(oldKey)
	currentTarget := resolver.Resolve(newKey)
	knownKeys := map[string]bool{}
	for _, ev := range events {
		if IsThreadEvent(ev) {
			knownKeys[resolver.Resolve(ev.Thread)] = true
		}
	}
	if !knownKeys[oldCurrent] {
		return ChannelEvent{}, fmt.Errorf("在频道 %q 中找不到 thread %q", opts.Channel, oldKey)
	}
	if knownKeys[currentTarget] && currentTarget != oldCurrent {
		return ChannelEvent{}, fmt.Errorf("thread %q 已存在于频道 %q，拒绝合并两条时间线", newKey, opts.Channel)
	}
	partial := map[string]any{
		"kind":      "thread",
		"by":        opts.By,
		"action":    "rename",
		"thread":    oldKey,
		"newThread": newKey,
	}
	if opts.Origin != "" {
		partial["origin"] = opts.Origin
	}
	return storeImpl.AppendEvent(opts.Channel, ref.Project, partial)
}

// ListForumThreadsOptions 是 ListForumThreads 的参数。
type ListForumThreadsOptions struct {
	Channel string
	Scope   string // project | global，空为自动解析
}

// ListForumThreads 列出 forum 频道的全部 thread 状态（按更新时间倒序）。
func ListForumThreads(storeImpl ForumStore, opts ListForumThreadsOptions) ([]ThreadState, error) {
	ref, err := ResolveChannel(ChannelAddress{Channel: opts.Channel, Scope: opts.Scope})
	if err != nil {
		return nil, err
	}
	events, err := ReadForumChannelEvents(storeImpl, opts.Channel, ref.Project, "forum")
	if err != nil {
		return nil, err
	}
	return ReduceThreads(events), nil
}

// ShowThreadOptions 是 ShowThread 的参数。
type ShowThreadOptions struct {
	Channel string
	Scope   string // project | global，空为自动解析
	Thread  string
}

// ShowThread 返回某 thread 的事件时间线（按 seq 升序，含改名前旧名下的事件）。
func ShowThread(storeImpl ForumStore, opts ShowThreadOptions) ([]ChannelEvent, error) {
	ref, err := ResolveChannel(ChannelAddress{Channel: opts.Channel, Scope: opts.Scope})
	if err != nil {
		return nil, err
	}
	events, err := ReadForumChannelEvents(storeImpl, opts.Channel, ref.Project, "thread")
	if err != nil {
		return nil, err
	}
	key, err := NormalizeThreadKey(opts.Thread)
	if err != nil {
		return nil, err
	}
	return CollectThreadTimeline(events, key), nil
}

// FormatThreadBoard 把 thread 状态列表格式化为看板行（forum 命令的默认输出）。
func FormatThreadBoard(states []ThreadState) []string {
	if len(states) == 0 {
		return []string{"（暂无主题）"}
	}
	lines := []string{"主题  状态  标题"}
	for _, st := range states {
		line := fmt.Sprintf("%s [%s] %s", st.ID, st.Status, st.Title)
		if len(st.Labels) > 0 {
			line += fmt.Sprintf(" labels=%s", strings.Join(st.Labels, ","))
		}
		if len(st.Assignees) > 0 {
			line += fmt.Sprintf(" assignees=%s", strings.Join(st.Assignees, ","))
		}
		lines = append(lines, line)
	}
	return lines
}
