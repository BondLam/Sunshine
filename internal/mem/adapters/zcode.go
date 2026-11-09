package adapters

import (
	"context"
	"fmt"

	"github.com/linrong1982/Sunshine/internal/mem"
)

// ZCode 会话库：~/.zcode/cli/db/db.sqlite（WAL 模式），只读打开。
// 伴生 -wal 文件由驱动自动识别并读取。
// 读三张表：
//   - session：id / title / directory（工作区 cwd）/ time_created /
//     time_updated / task_type
//   - message：id / session_id / time_created / data（JSON：{role, ...}）
//   - part：message_id / time_created / data
//     （JSON：{type, text|tool, ...}），另有压缩 marker，
//     以摘要消息替换更早的对话。
//
// 旧的 ~/.zcode/v2/sessions/*.json 布局已被当前 ZCode 放弃，
// ~/.zcode/cli/rollout/*.jsonl 是定期清理的非持久模型 IO 流，
// 两者都不读；SQLite 是唯一完整的事实来源。

func zcodeSessions(ctx context.Context, home string) ([]mem.MemSessionInfo, []string, error) {
	dbPath := ZcodeDBPath(home)
	db, warnings, opened := openMemDB(ctx, dbPath, "ZCode")
	if !opened {
		return nil, warnings, nil
	}
	defer db.Close()
	cols, err := TableColumns(ctx, db, "session")
	if err != nil {
		return nil, []string{fmt.Sprintf("不支持的 ZCode 会话库结构（%s）：%v", dbPath, err)}, nil
	}
	if err := RequireColumns(cols, "session", "id", "directory", "time_created", "time_updated"); err != nil {
		return nil, []string{fmt.Sprintf("不支持的 ZCode 会话库结构（%s）：%v", dbPath, err)}, nil
	}
	rows, err := QueryRows(ctx, db, "SELECT id, title, directory, time_created, time_updated, task_type FROM session")
	if err != nil {
		return nil, []string{fmt.Sprintf("无法读取 ZCode 会话库（%s）：%v", dbPath, err)}, nil
	}
	var out []mem.MemSessionInfo
	for _, row := range rows {
		// subagent_child 会话是子代理对话（Explore / research 分发），
		// 默认列表排除——日常回顾关心的是用户的交互会话。
		if rowStr(row, "task_type") == "subagent_child" {
			continue
		}
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
			Platform: mem.MemSourceZcode,
			ID:       id,
			Title:    rowStr(row, "title"),
			CWD:      rowStr(row, "directory"),
			Created:  created,
			Updated:  updated,
			FilePath: dbPath,
		})
	}
	return out, nil, nil
}

func zcodeDialogue(ctx context.Context, s mem.MemSessionInfo) ([]mem.DialogueTurn, []string, error) {
	turns, _, warnings, err := zcodeCollect(ctx, s)
	return turns, warnings, err
}

// zcodeCollect 一次遍历同时产出清理后的对话轮次与任务命令事件：
// tool 类型 part（tool 为 Bash/bash）的 state.input.command 里藏着
// `sunshine task create|start` 调用。注意顺序与上游一致：先把本消息
// 的文本轮次入池，再记工具事件——事件的 turnIndex 是入池后的轮次数
// （助手先写回复、工具随后执行）。
func zcodeCollect(ctx context.Context, s mem.MemSessionInfo) ([]mem.DialogueTurn, []mem.MemEvent, []string, error) {
	db, warnings, opened := openMemDB(ctx, s.FilePath, "ZCode")
	if !opened {
		return nil, nil, warnings, nil
	}
	defer db.Close()
	if err := checkMsgPartSchema(ctx, db, s.FilePath, "ZCode"); err != nil {
		return nil, nil, append(warnings, err.Error()), nil
	}
	store, err := loadMsgPartStore(ctx, db, s.ID, false)
	if err != nil {
		return nil, nil, append(warnings, fmt.Sprintf("无法读取 ZCode 会话库（%s）：%v", s.FilePath, err)), nil
	}
	messages := store.messagesBySession[s.ID]
	summaryIDs := compactSummaryMessageIDs(messages, store.partsByMsg)
	var turns []mem.DialogueTurn
	var events []mem.MemEvent
	for _, msg := range messages {
		if turn := buildMsgPartTurn(msg, store.partsByMsg[msg.id], summaryIDs); turn != nil {
			turns = append(turns, *turn)
		}
		for _, part := range store.partsByMsg[msg.id] {
			if objStr(part.data, "type") != "tool" {
				continue
			}
			tool := objStr(part.data, "tool")
			if tool != "Bash" && tool != "bash" {
				continue
			}
			state := objMap(part.data, "state")
			input := objMap(state, "input")
			command, ok := input["command"].(string)
			if !ok || command == "" {
				continue
			}
			events = append(events, mem.EventsFromParsed(
				mem.ParseTaskCommandsAll(command), toISOEpochMs(float64(part.timeCreated)), len(turns))...)
		}
	}
	return turns, events, warnings, nil
}
