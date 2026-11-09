package adapters

import (
	"context"
	"database/sql"
	"encoding/json"
	"sort"
	"strings"

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
	data        map[string]any
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
		data := parseDataJSON(row["data"])
		if data == nil {
			continue
		}
		tc, _ := rowInt(row, "time_created")
		list := store.partsByMsg[msgID]
		list = append(list, partRow{messageID: msgID, timeCreated: tc, seq: int64(i), data: data})
		store.partsByMsg[msgID] = list
	}
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
	return store, nil
}

// isCompactionSummaryPart 判断 part 是否为压缩摘要载体：
// type 为 compaction 且带 tail_start_id 或 compactBoundary。
func isCompactionSummaryPart(data map[string]any) bool {
	if objStr(data, "type") != "compaction" {
		return false
	}
	if tail, ok := data["tail_start_id"].(string); ok && tail != "" {
		return true
	}
	_, ok := data["compactBoundary"].(map[string]any)
	return ok
}

// compactionMarkerSummaryID 取 marker part 指向的摘要消息 id：
// type 为 compaction、replace 为 true 且 summaryMessageId 为字符串。
func compactionMarkerSummaryID(data map[string]any) string {
	if objStr(data, "type") != "compaction" || !objBool(data, "replace") {
		return ""
	}
	if id, ok := data["summaryMessageId"].(string); ok {
		return id
	}
	return ""
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
		if objStr(part.data, "type") != "text" {
			continue
		}
		text, ok := part.data["text"].(string)
		if !ok || text == "" {
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
