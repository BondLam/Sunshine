package adapters

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/linrong007/Sunshine/internal/mem"
)

// OpenCode 会话库：$XDG_DATA_HOME/opencode/opencode.db
//（OPENCODE_DB 可覆盖，opencode-<channel>.db 取最近写入的一份），
//XDG 规则全平台一致，无 Windows/macOS 分支。只读打开。
// 读三张表（已对真实 1.18 存储确认）：
//   - session：id / parent_id（子代理链）/ title / directory
//     （工作区 cwd）/ time_created / time_updated
//   - message：id / session_id / time_created / data（JSON：{role, ...}）
//   - part：message_id / session_id / time_created /
//     data（JSON：{type: "text"|"tool"|"reasoning"|..., ...}）
//
// 列会话只读 session 表，从不碰 message / part。

func opencodeSessions(ctx context.Context, home string) ([]mem.MemSessionInfo, []string, error) {
	dbPath, ok := OpencodeDBPath(home)
	if !ok {
		return nil, []string{"未找到 OpenCode 会话库，已跳过"}, nil
	}
	db, warnings, opened := openMemDB(ctx, dbPath, "OpenCode")
	if !opened {
		return nil, warnings, nil
	}
	defer db.Close()
	cols, err := TableColumns(ctx, db, "session")
	if err != nil {
		return nil, []string{fmt.Sprintf("不支持的 OpenCode 会话库结构（%s）：%v", dbPath, err)}, nil
	}
	if err := RequireColumns(cols, "session", "id", "time_created", "time_updated"); err != nil {
		return nil, []string{fmt.Sprintf("不支持的 OpenCode 会话库结构（%s）：%v", dbPath, err)}, nil
	}
	cwdColumn := ""
	for _, candidate := range []string{"directory", "cwd"} {
		if cols[candidate] {
			cwdColumn = candidate
			break
		}
	}
	if cwdColumn == "" {
		return nil, []string{fmt.Sprintf("不支持的 OpenCode 会话库结构（%s）：session 表缺少 directory / cwd 列", dbPath)}, nil
	}
	rows, err := QueryRows(ctx, db, fmt.Sprintf("SELECT id, parent_id, title, %q, time_created, time_updated FROM session", cwdColumn))
	if err != nil {
		return nil, []string{fmt.Sprintf("无法读取 OpenCode 会话库（%s）：%v", dbPath, err)}, nil
	}
	var out []mem.MemSessionInfo
	for _, row := range rows {
		id := rowStr(row, "id")
		if id == "" {
			continue
		}
		created := ""
		if epoch, ok := rowFloat(row, "time_created"); ok {
			created = toISOEpochMs(epoch)
		}
		updated := created
		if epoch, ok := rowFloat(row, "time_updated"); ok {
			if iso := toISOEpochMs(epoch); iso != "" {
				updated = iso
			}
		}
		out = append(out, mem.MemSessionInfo{
			Platform: mem.MemSourceOpencode,
			ID:       id,
			Title:    rowStr(row, "title"),
			CWD:      rowStr(row, cwdColumn),
			Created:  created,
			Updated:  updated,
			FilePath: dbPath,
			// 子代理会话指向分发者；--include-children 会把它并入父会话。
			ParentID: rowStr(row, "parent_id"),
		})
	}
	return out, nil, nil
}

func opencodeDialogue(ctx context.Context, s mem.MemSessionInfo) ([]mem.DialogueTurn, []string, error) {
	turns, _, warnings, err := opencodeCollect(ctx, s)
	return turns, warnings, err
}

// opencodeCollect 只产出对话轮次，事件恒为空：OpenCode 不支持按阶段
// 切片（phaseSupported=false），extract --phase 时降级为全文 + 警告，
// 见 mem.SliceMemPhase。
// search 预加载命中时直接从内存 store 取轮次，不重开库、不再查表。
func opencodeCollect(ctx context.Context, s mem.MemSessionInfo) ([]mem.DialogueTurn, []mem.MemEvent, []string, error) {
	if store := cachedMsgPartStore(s.FilePath); store != nil {
		return opencodeTurnsFromStore(store, s.ID), nil, nil, nil
	}
	db, warnings, opened := openMemDB(ctx, s.FilePath, "OpenCode")
	if !opened {
		return nil, nil, warnings, nil
	}
	defer db.Close()
	if err := checkMsgPartSchema(ctx, db, s.FilePath, "OpenCode"); err != nil {
		return nil, nil, append(warnings, err.Error()), nil
	}
	partCols, err := TableColumns(ctx, db, "part")
	if err != nil {
		return nil, nil, append(warnings, fmt.Sprintf("不支持的 OpenCode 会话库结构（%s）：%v", s.FilePath, err)), nil
	}
	store, err := loadMsgPartStore(ctx, db, s.ID, partCols["session_id"])
	if err != nil {
		return nil, nil, append(warnings, fmt.Sprintf("无法读取 OpenCode 会话库（%s）：%v", s.FilePath, err)), nil
	}
	return opencodeTurnsFromStore(store, s.ID), nil, warnings, nil
}

// opencodeTurnsFromStore 从 message / part store 里拼出一次会话的轮次
// （预加载命中与逐会话读共用，保证两路结果一致）。
func opencodeTurnsFromStore(store *msgPartStore, sessionID string) []mem.DialogueTurn {
	messages := store.messagesBySession[sessionID]
	summaryIDs := compactSummaryMessageIDs(messages, store.partsByMsg)
	var turns []mem.DialogueTurn
	for _, msg := range messages {
		if turn := buildMsgPartTurn(msg, store.partsByMsg[msg.id], summaryIDs); turn != nil {
			turns = append(turns, *turn)
		}
	}
	return turns
}

// checkMsgPartSchema 校验 message / part 表与必需列。
func checkMsgPartSchema(ctx context.Context, db *sql.DB, dbPath, platformLabel string) error {
	msgCols, err := TableColumns(ctx, db, "message")
	if err != nil {
		return fmt.Errorf("不支持的 %s 会话库结构（%s）：%v", platformLabel, dbPath, err)
	}
	if err := RequireColumns(msgCols, "message", "id", "session_id", "data"); err != nil {
		return fmt.Errorf("不支持的 %s 会话库结构（%s）：%v", platformLabel, dbPath, err)
	}
	partCols, err := TableColumns(ctx, db, "part")
	if err != nil {
		return fmt.Errorf("不支持的 %s 会话库结构（%s）：%v", platformLabel, dbPath, err)
	}
	if err := RequireColumns(partCols, "part", "message_id", "data"); err != nil {
		return fmt.Errorf("不支持的 %s 会话库结构（%s）：%v", platformLabel, dbPath, err)
	}
	return nil
}
