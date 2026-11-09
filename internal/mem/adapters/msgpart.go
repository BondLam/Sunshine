package adapters

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"sync"

	"github.com/linrong1982/Sunshine/internal/mem"
)

// 本文件是 OpenCode 与 ZCode 共用的 message / part 存储读取逻辑
//（两家表结构同源：session / message / part，message 的 data 是
// {role, ...} 的 JSON，part 的 data 是 {type: "text"|"tool"|..., ...}）。

// msgRow 是一条 message 行（已解析出 role）。
type msgRow struct {
	id          string
	timeCreated int64
	seq         int64
	role        string
}

// partRow 是一条 part 行（data 已解析）。
type partRow struct {
	messageID   string
	timeCreated int64
	seq         int64
	data        *partData
}

// partData 是 part 的 data JSON 的强类型解析：只取后续流程真正读的
// 字段（type / text / tool / state.input.command / replace /
// summaryMessageId / tail_start_id / compactBoundary），
// 比 map 解析快（bulk load 解析 30k 个 part 时约省一半解析时间）。
// 未知字段忽略——后续流程不读它们。
// Replace 与 CompactBoundary 用 RawMessage 存：任意 JSON 值都不报错，
// 再按旧的 objBool / map 断言语义判断（true 当且仅当字面 true；
// compactBoundary 当且仅当是个对象）。
type partData struct {
	Type             string          `json:"type"`
	Text             string          `json:"text"`
	Tool             string          `json:"tool"`
	State            json.RawMessage `json:"state"`
	Replace          json.RawMessage `json:"replace"`
	SummaryMessageID string          `json:"summaryMessageId"`
	TailStartID      string          `json:"tail_start_id"`
	CompactBoundary  json.RawMessage `json:"compactBoundary"`

	// stateCommand 是 state.input.command（解析时一次算好，
	// ZCode 事件抽取直接读它；state 不是对象或 command 不是字符串时为空，
	// 与旧的 objMap 链语义一致）。
	stateCommand string
}

// IsReplace 判断 replace 是否为字面 true（与 objBool 语义一致）。
func (d *partData) IsReplace() bool {
	return strings.TrimSpace(string(d.Replace)) == "true"
}

// HasCompactBoundary 判断 compactBoundary 是否是个对象
// （与旧的 map 断言语义一致）。
func (d *partData) HasCompactBoundary() bool {
	b := strings.TrimSpace(string(d.CompactBoundary))
	return len(b) > 0 && b[0] == '{'
}

// StateInputCommand 返回 state.input.command（见 stateCommand 注释）。
func (d *partData) StateInputCommand() string {
	return d.stateCommand
}

// parseStateCommand 从 state 的原始 JSON 里取 input.command，
// 不是对象或 command 不是字符串时返回 ""。
func parseStateCommand(state json.RawMessage) string {
	if len(strings.TrimSpace(string(state))) == 0 {
		return ""
	}
	var v struct {
		Input struct {
			Command string `json:"command"`
		} `json:"input"`
	}
	if err := json.Unmarshal(state, &v); err != nil {
		return ""
	}
	return v.Input.Command
}

// parsePartData 用 struct 解析 part 的 data JSON；
// 结构不合预期（如某个字符串字段给了数字）时回退到 map 解析再转换，
// 与通用路径的语义完全一致；JSON 语法错误或非对象时返回 nil。
func parsePartData(raw string) *partData {
	if strings.TrimSpace(raw) == "null" {
		return nil
	}
	var pd partData
	if err := json.Unmarshal([]byte(raw), &pd); err == nil {
		pd.stateCommand = parseStateCommand(pd.State)
		// 归一化到与 partDataFromMap 相同的表示（只保留行为有意义的值，
		// 这样两路产出的 partData 完全一致）：
		// State 只用来派生 stateCommand；Replace 只有字面 true 有意义；
		// CompactBoundary 只有“是个对象”有意义。
		pd.State = nil
		if strings.TrimSpace(string(pd.Replace)) != "true" {
			pd.Replace = nil
		}
		if pd.HasCompactBoundary() {
			pd.CompactBoundary = json.RawMessage("{}")
		} else {
			pd.CompactBoundary = nil
		}
		return &pd
	}
	return partDataFromMap(parseDataJSON(raw))
}

// partDataFromMap 把 map 解析结果转成 partData（通用路径用，
// 字段语义与旧的 objStr / objBool / objMap 读取完全一致）。
func partDataFromMap(m map[string]any) *partData {
	if m == nil {
		return nil
	}
	pd := &partData{
		Type:             objStr(m, "type"),
		Text:             objStr(m, "text"),
		Tool:             objStr(m, "tool"),
		SummaryMessageID: objStr(m, "summaryMessageId"),
		TailStartID:      objStr(m, "tail_start_id"),
	}
	if b, ok := m["replace"].(bool); ok && b {
		pd.Replace = json.RawMessage("true")
	}
	if _, ok := m["compactBoundary"].(map[string]any); ok {
		pd.CompactBoundary = json.RawMessage("{}")
	}
	if state, ok := m["state"].(map[string]any); ok {
		if input, ok := state["input"].(map[string]any); ok {
			pd.stateCommand, _ = input["command"].(string)
		}
	}
	return pd
}

// parseMessageRole 取 message 的 data JSON 里的 role：
// struct 解析（快路径），结构不合预期时回退 map 解析；
// JSON 语法错误或非对象时返回 false。
func parseMessageRole(raw string) (string, bool) {
	if strings.TrimSpace(raw) == "null" {
		return "", false
	}
	var md struct {
		Role string `json:"role"`
	}
	if err := json.Unmarshal([]byte(raw), &md); err == nil {
		return md.Role, true
	}
	m := parseDataJSON(raw)
	if m == nil {
		return "", false
	}
	return objStr(m, "role"), true
}

// msgPartStore 是按会话与消息分组的 message / part。
type msgPartStore struct {
	messagesBySession map[string][]msgRow
	partsByMsg        map[string][]partRow
}

// parseDataJSON 解析 data 列里的 JSON；失败或非对象时返回 nil，
// 坏行直接丢弃，不抛错。
func parseDataJSON(raw any) map[string]any {
	text, ok := raw.(string)
	if !ok {
		return nil
	}
	var v map[string]any
	if err := json.Unmarshal([]byte(text), &v); err != nil || v == nil {
		return nil
	}
	return v
}

// loadMsgPartStore 读 message 与 part 表。sessionID 为空时读全库，
// 否则只读该会话的行（part 表有 session_id 列时直接按它过滤，
// 否则先取消息 id 再按 message_id 过滤）。
func loadMsgPartStore(ctx context.Context, db *sql.DB, sessionID string, partHasSessionID bool) (*msgPartStore, error) {
	store := &msgPartStore{
		messagesBySession: make(map[string][]msgRow),
		partsByMsg:        make(map[string][]partRow),
	}
	var messages []map[string]any
	var err error
	if sessionID == "" {
		messages, err = QueryRows(ctx, db, "SELECT id, session_id, time_created, data FROM message ORDER BY rowid")
	} else {
		messages, err = QueryRows(ctx, db, "SELECT id, session_id, time_created, data FROM message WHERE session_id = ? ORDER BY rowid", sessionID)
	}
	if err != nil {
		return nil, err
	}
	for i, row := range messages {
		session := rowStr(row, "session_id")
		id := rowStr(row, "id")
		if session == "" || id == "" {
			continue
		}
		data := parseDataJSON(row["data"])
		if data == nil {
			continue
		}
		role := objStr(data, "role")
		if role != "user" && role != "assistant" {
			continue
		}
		tc, _ := rowInt(row, "time_created")
		list := store.messagesBySession[session]
		list = append(list, msgRow{id: id, timeCreated: tc, seq: int64(i), role: role})
		store.messagesBySession[session] = list
	}
	var parts []map[string]any
	if sessionID == "" {
		parts, err = QueryRows(ctx, db, "SELECT message_id, time_created, data FROM part ORDER BY rowid")
	} else if partHasSessionID {
		parts, err = QueryRows(ctx, db, "SELECT message_id, session_id, time_created, data FROM part WHERE session_id = ? ORDER BY rowid", sessionID)
	} else {
		messageIDs := make(map[string]bool)
		for _, row := range messages {
			if id := rowStr(row, "id"); id != "" {
				messageIDs[id] = true
			}
		}
		allParts, qerr := QueryRows(ctx, db, "SELECT message_id, time_created, data FROM part ORDER BY rowid")
		if qerr != nil {
			return nil, qerr
		}
		for _, row := range allParts {
			if messageIDs[rowStr(row, "message_id")] {
				parts = append(parts, row)
			}
		}
	}
	if err != nil {
		return nil, err
	}
	for i, row := range parts {
		msgID := rowStr(row, "message_id")
		if msgID == "" {
			continue
		}
		data := partDataFromMap(parseDataJSON(row["data"]))
		if data == nil {
			continue
		}
		tc, _ := rowInt(row, "time_created")
		list := store.partsByMsg[msgID]
		list = append(list, partRow{messageID: msgID, timeCreated: tc, seq: int64(i), data: data})
		store.partsByMsg[msgID] = list
	}
	sortMsgPartStore(store)
	return store, nil
}

// sortMsgPartStore 按（time_created, seq）稳定排序 store 里的
// 全部消息与 part（通用路径与 bulk 路径共用）。
func sortMsgPartStore(store *msgPartStore) {
	for _, list := range store.messagesBySession {
		sort.SliceStable(list, func(i, j int) bool {
			if list[i].timeCreated != list[j].timeCreated {
				return list[i].timeCreated < list[j].timeCreated
			}
			return list[i].seq < list[j].seq
		})
	}
	for _, list := range store.partsByMsg {
		sort.SliceStable(list, func(i, j int) bool {
			if list[i].timeCreated != list[j].timeCreated {
				return list[i].timeCreated < list[j].timeCreated
			}
			return list[i].seq < list[j].seq
		})
	}
}

// toInt64 把直接扫描出的列值转成整数（int64 / float64 / 数字字符串，
// 与 rowInt 接受的类型一致）；转不了返回 false。
func toInt64(v any) (int64, bool) {
	switch n := v.(type) {
	case nil:
		return 0, false
	case int64:
		return n, true
	case float64:
		return int64(n), true
	case string:
		var parsed int64
		if _, err := fmt.Sscanf(n, "%d", &parsed); err == nil {
			return parsed, true
		}
	}
	return 0, false
}

// loadMsgPartStoreBulk 全量读 message 与 part 表（预加载专用）：
// 直接按列扫描，不经过 QueryRows 的逐行 map，60k 行规模下比通用
// 路径省掉大量小分配；两张表互不依赖，并行扫描。
// 语义与 loadMsgPartStore(sessionID="") 完全一致
// （同样的 ORDER BY rowid、同样的 role 过滤、同样的 seq 与排序），
// 等价性见 TestLoadMsgPartStoreBulk_EqualsGeneric。
func loadMsgPartStoreBulk(ctx context.Context, db *sql.DB) (*msgPartStore, error) {
	store := &msgPartStore{
		messagesBySession: make(map[string][]msgRow),
		partsByMsg:        make(map[string][]partRow),
	}
	var wg sync.WaitGroup
	var msgErr, partErr error
	wg.Add(2)
	go func() {
		defer wg.Done()
		msgErr = scanBulkMessages(ctx, db, store)
	}()
	go func() {
		defer wg.Done()
		partErr = scanBulkParts(ctx, db, store)
	}()
	wg.Wait()
	if msgErr != nil {
		return nil, msgErr
	}
	if partErr != nil {
		return nil, partErr
	}
	sortMsgPartStore(store)
	return store, nil
}

// scanBulkMessages 全量读 message 表进 store（bulk 并行扫描的一路）。
func scanBulkMessages(ctx context.Context, db *sql.DB, store *msgPartStore) error {
	msgRows, err := db.QueryContext(ctx, "SELECT id, session_id, time_created, data FROM message ORDER BY rowid")
	if err != nil {
		return err
	}
	defer msgRows.Close()
	seq := int64(0)
	for msgRows.Next() {
		var id, session, dataStr string
		var tcRaw any
		if err := msgRows.Scan(&id, &session, &tcRaw, &dataStr); err != nil {
			return err
		}
		tc, _ := toInt64(tcRaw)
		if session == "" || id == "" {
			seq++
			continue
		}
		role, ok := parseMessageRole(dataStr)
		if !ok {
			seq++
			continue
		}
		if role != "user" && role != "assistant" {
			seq++
			continue
		}
		list := store.messagesBySession[session]
		list = append(list, msgRow{id: id, timeCreated: tc, seq: seq, role: role})
		store.messagesBySession[session] = list
		seq++
	}
	return msgRows.Err()
}

// scanBulkParts 全量读 part 表进 store（bulk 并行扫描的一路）。
func scanBulkParts(ctx context.Context, db *sql.DB, store *msgPartStore) error {
	partRows, err := db.QueryContext(ctx, "SELECT message_id, time_created, data FROM part ORDER BY rowid")
	if err != nil {
		return err
	}
	defer partRows.Close()
	seq := int64(0)
	for partRows.Next() {
		var msgID, dataStr string
		var tcRaw any
		if err := partRows.Scan(&msgID, &tcRaw, &dataStr); err != nil {
			return err
		}
		tc, _ := toInt64(tcRaw)
		if msgID == "" {
			seq++
			continue
		}
		data := parsePartData(dataStr)
		if data == nil {
			seq++
			continue
		}
		list := store.partsByMsg[msgID]
		list = append(list, partRow{messageID: msgID, timeCreated: tc, seq: seq, data: data})
		store.partsByMsg[msgID] = list
		seq++
	}
	return partRows.Err()
}

// isCompactionSummaryPart 判断 part 是否为压缩摘要载体：
// type 为 compaction 且带 tail_start_id 或 compactBoundary。
func isCompactionSummaryPart(data *partData) bool {
	if data == nil || data.Type != "compaction" {
		return false
	}
	if data.TailStartID != "" {
		return true
	}
	return data.HasCompactBoundary()
}

// compactionMarkerSummaryID 取 marker part 指向的摘要消息 id：
// type 为 compaction、replace 为 true 且 summaryMessageId 为字符串。
func compactionMarkerSummaryID(data *partData) string {
	if data == nil || data.Type != "compaction" || !data.IsReplace() {
		return ""
	}
	return data.SummaryMessageID
}

// compactSummaryMessageIDs 找出承载压缩摘要的消息：压缩后的轮次
// 仍以行留在同一库里，所以保留在对话中，摘要消息就地渲染成边界 marker。
func compactSummaryMessageIDs(messages []msgRow, partsByMsg map[string][]partRow) map[string]bool {
	summaryIDs := make(map[string]bool)
	markerSummaryIDs := make(map[string]bool)
	for _, msg := range messages {
		if markerSummaryIDs[msg.id] {
			summaryIDs[msg.id] = true
		}
		for _, part := range partsByMsg[msg.id] {
			if markerID := compactionMarkerSummaryID(part.data); markerID != "" {
				markerSummaryIDs[markerID] = true
			}
			if isCompactionSummaryPart(part.data) {
				summaryIDs[msg.id] = true
				break
			}
		}
	}
	return summaryIDs
}

// buildMsgPartTurn 把一条消息变成一轮：text part 拼接后清理；
// reasoning / tool / step-start / step-finish 等 part 不是对话，跳过；
// 无 surviving 文本的消息丢弃；摘要消息变成边界 marker。
func buildMsgPartTurn(msg msgRow, parts []partRow, compactSummaryIDs map[string]bool) *mem.DialogueTurn {
	var collected []string
	totalRaw := 0
	for _, part := range parts {
		if part.data == nil || part.data.Type != "text" {
			continue
		}
		text := part.data.Text
		if text == "" {
			continue
		}
		totalRaw += len(text)
		collected = append(collected, stripInjectionTags(text))
	}
	if len(collected) == 0 {
		return nil
	}
	merged := strings.Join(collected, "\n\n")
	if compactSummaryIDs[msg.id] {
		turn := compactionBoundaryTurn(
			"此处上下文已压缩；上方的轮次仍在会话库里", merged)
		return &turn
	}
	if isBootstrapTurn(merged, totalRaw) {
		return nil
	}
	if strings.TrimSpace(merged) == "" {
		return nil
	}
	return &mem.DialogueTurn{Role: msg.role, Text: merged}
}
