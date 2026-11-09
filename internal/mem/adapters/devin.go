package adapters

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"

	"github.com/linrong1982/Sunshine/internal/mem"
)

// Devin CLI 会话库：$XDG_DATA_HOME/devin/cli/sessions.db
//（DEVIN_DB_PATH 可覆盖），WAL 模式 SQLite，只读打开。
// 读两张表（已对真实 Cognition 存储确认）：
//   - sessions：id / working_directory / created_at / last_activity_at
//     （Unix 秒）/ title / main_chain_id / hidden（/rm-session 软删除）
//   - message_nodes：session_id / node_id / parent_node_id / chat_message
//     （JSON）/ created_at。这是一片森林不是列表：沿 parent 指针从
//     main_chain_id 回走，丢弃分叉/回滚的旁支。
//
// chat_message JSON：{role, content: string, tool_calls?, thinking?,
// metadata}。role 有 user / assistant / tool / system；
// 真实用户轮次 metadata.is_user_input === true；
// 压缩是 metadata.extensions["devin-rs/summary"] 已设置的 system 节点。

func devinSessions(ctx context.Context, home string) ([]mem.MemSessionInfo, []string, error) {
	dbPath, ok := DevinDBPath(home)
	if !ok {
		return nil, []string{"DEVIN_DB_PATH 已设为 :memory:，无 Devin 会话文件可读，已跳过"}, nil
	}
	db, warnings, opened := openMemDB(ctx, dbPath, "Devin")
	if !opened {
		return nil, warnings, nil
	}
	defer db.Close()
	cols, err := TableColumns(ctx, db, "sessions")
	if err != nil {
		return nil, []string{fmt.Sprintf("不支持的 Devin 会话库结构（%s）：%v", dbPath, err)}, nil
	}
	if err := RequireColumns(cols, "sessions", "id", "working_directory", "created_at"); err != nil {
		return nil, []string{fmt.Sprintf("不支持的 Devin 会话库结构（%s）：%v", dbPath, err)}, nil
	}
	rows, err := QueryRows(ctx, db, "SELECT id, working_directory, created_at, last_activity_at, title, hidden FROM sessions")
	if err != nil {
		return nil, []string{fmt.Sprintf("无法读取 Devin 会话库（%s）：%v", dbPath, err)}, nil
	}
	var out []mem.MemSessionInfo
	for _, row := range rows {
		id := rowStr(row, "id")
		if id == "" {
			continue
		}
		if hidden, ok := rowInt(row, "hidden"); ok && hidden != 0 {
			continue
		}
		created := ""
		if epoch, ok := rowFloat(row, "created_at"); ok {
			created = toISOUnixEpoch(epoch)
		}
		updated := created
		if epoch, ok := rowFloat(row, "last_activity_at"); ok {
			if iso := toISOUnixEpoch(epoch); iso != "" {
				updated = iso
			}
		}
		out = append(out, mem.MemSessionInfo{
			Platform: mem.MemSourceDevin,
			ID:       id,
			Title:    rowStr(row, "title"),
			CWD:      rowStr(row, "working_directory"),
			Created:  created,
			Updated:  updated,
			FilePath: dbPath,
		})
	}
	return out, nil, nil
}

// devinSlimNode 是 message_nodes 的一行，已剥掉原始 chat_message JSON
// （thinking、工具载荷、telemetry 不保留）；链行走仍需全部
// node_id / parent 关系，user / assistant / 摘要节点的对话字段才填充。
// execCommands 只在 assistant 节点上：tool_calls 里 name 为 exec 的
// 调用恢复出的 shell 命令，供 Collect 抽任务命令事件。
type devinSlimNode struct {
	nodeID       int64
	parentNodeID *int64
	createdAt    float64
	isSummary    bool
	role         string
	text         string
	execCommands []string
}

func devinDialogue(ctx context.Context, s mem.MemSessionInfo) ([]mem.DialogueTurn, []string, error) {
	turns, _, warnings, err := devinCollect(ctx, s)
	return turns, warnings, err
}

// devinCollect 一次遍历主链同时产出清理后的对话轮次与任务命令事件：
// 助手节点的 exec 工具调用里藏着 `sunshine task create|start` 调用，
// 事件的 turnIndex 是该节点自身轮次入池前的轮次数
// （工具调用属于正在生成的这轮助手回复）；缺主链时返回空对话与空事件。
func devinCollect(ctx context.Context, s mem.MemSessionInfo) ([]mem.DialogueTurn, []mem.MemEvent, []string, error) {
	var nodes []devinSlimNode
	var mainChainID *int64
	var warnings []string
	// search 预加载命中时直接从内存取节点与主链，不重开库、不再查表。
	if cachedNodes, cachedChains, ok := cachedDevinStore(s.FilePath); ok {
		nodes = cachedNodes[s.ID]
		if tip, ok := cachedChains[s.ID]; ok {
			mainChainID = &tip
		}
	} else {
		var stop bool
		nodes, mainChainID, warnings, stop = devinLoadNodes(ctx, s)
		if stop {
			return nil, nil, warnings, nil
		}
	}
	chain, missingTip := devinWalkMainChain(nodes, mainChainID)
	if missingTip {
		return nil, nil, append(warnings, fmt.Sprintf("Devin 会话 %s 没有可用的 main_chain_id，已拒绝沿分叉行走（%s）", s.ID, s.FilePath)), nil
	}
	var turns []mem.DialogueTurn
	var events []mem.MemEvent
	for _, node := range chain {
		if len(node.execCommands) > 0 {
			ts := toISOUnixEpoch(node.createdAt)
			for _, cmd := range node.execCommands {
				events = append(events, mem.EventsFromParsed(
					mem.ParseTaskCommandsAll(cmd), ts, len(turns))...)
			}
		}
		if turn := devinBuildTurn(node); turn != nil {
			turns = append(turns, *turn)
		}
	}
	return turns, events, warnings, nil
}

// devinLoadNodes 逐会话读一个 Devin 会话的节点与主链
// （预加载未命中时的老路径；stop 为真时调用方直接返回 warnings）。
func devinLoadNodes(ctx context.Context, s mem.MemSessionInfo) (nodes []devinSlimNode, mainChainID *int64, warnings []string, stop bool) {
	db, warnings, opened := openMemDB(ctx, s.FilePath, "Devin")
	if !opened {
		return nil, nil, warnings, true
	}
	defer db.Close()
	cols, err := TableColumns(ctx, db, "message_nodes")
	if err != nil {
		return nil, nil, append(warnings, fmt.Sprintf("不支持的 Devin 会话库结构（%s）：%v", s.FilePath, err)), true
	}
	if err := RequireColumns(cols, "message_nodes", "session_id", "node_id", "parent_node_id", "chat_message"); err != nil {
		return nil, nil, append(warnings, fmt.Sprintf("不支持的 Devin 会话库结构（%s）：%v", s.FilePath, err)), true
	}
	rows, err := QueryRows(ctx, db, "SELECT node_id, parent_node_id, chat_message, created_at FROM message_nodes WHERE session_id = ? ORDER BY rowid", s.ID)
	if err != nil {
		return nil, nil, append(warnings, fmt.Sprintf("无法读取 Devin 会话库（%s）：%v", s.FilePath, err)), true
	}
	nodes = make([]devinSlimNode, 0, len(rows))
	for _, row := range rows {
		nodeID, ok := rowInt(row, "node_id")
		if !ok {
			continue
		}
		var parent *int64
		if p, ok := rowInt(row, "parent_node_id"); ok {
			parent = &p
		}
		createdAt, _ := rowFloat(row, "created_at")
		nodes = append(nodes, devinSlimFromChatMessage(nodeID, parent, createdAt, row["chat_message"]))
	}
	// 取 main_chain_id。
	sessionCols, err := TableColumns(ctx, db, "sessions")
	if err == nil && sessionCols["main_chain_id"] {
		if sessionRows, err := QueryRows(ctx, db, "SELECT main_chain_id FROM sessions WHERE id = ?", s.ID); err == nil && len(sessionRows) > 0 {
			if tip, ok := rowInt(sessionRows[0], "main_chain_id"); ok {
				mainChainID = &tip
			}
		}
	}
	return nodes, mainChainID, warnings, false
}

func devinSlimFromChatMessage(nodeID int64, parent *int64, createdAt float64, raw any) devinSlimNode {
	base := devinSlimNode{nodeID: nodeID, parentNodeID: parent, createdAt: createdAt}
	text, ok := raw.(string)
	if !ok || text == "" {
		return base
	}
	var msg map[string]any
	if err := json.Unmarshal([]byte(text), &msg); err != nil || msg == nil {
		return base
	}
	if devinIsSummaryMessage(msg) {
		content, _ := msg["content"].(string)
		base.isSummary = true
		base.text = content
		return base
	}
	role := objStr(msg, "role")
	if role == "user" {
		if !devinIsUserInput(msg) {
			return base
		}
		content, _ := msg["content"].(string)
		base.role, base.text = "user", content
		return base
	}
	if role == "assistant" {
		content, _ := msg["content"].(string)
		base.role, base.text = "assistant", content
		base.execCommands = devinExecCommandsFrom(msg)
		return base
	}
	return base
}

// devinExecCommandsFrom 从助手消息的 tool_calls 里取出 name 为 exec 的
// 调用恢复出的 shell 命令：arguments 可以是 JSON 字符串或原始对象，
// 命令取其中的 command 字段；其余工具调用一律忽略。
func devinExecCommandsFrom(msg map[string]any) []string {
	calls, ok := msg["tool_calls"].([]any)
	if !ok {
		return nil
	}
	var out []string
	for _, call := range calls {
		callMap, ok := call.(map[string]any)
		if !ok || objStr(callMap, "name") != "exec" {
			continue
		}
		var obj map[string]any
		switch args := callMap["arguments"].(type) {
		case string:
			obj = parseDataJSON(args)
		case map[string]any:
			obj = args
		}
		if obj == nil {
			continue
		}
		if cmd, ok := obj["command"].(string); ok && cmd != "" {
			out = append(out, cmd)
		}
	}
	return out
}

func devinIsSummaryMessage(msg map[string]any) bool {
	md := objMap(msg, "metadata")
	if md == nil {
		return false
	}
	ext := objMap(md, "extensions")
	if ext == nil {
		return false
	}
	_, ok := ext["devin-rs/summary"]
	return ok
}

func devinIsUserInput(msg map[string]any) bool {
	md := objMap(msg, "metadata")
	return md != nil && objBool(md, "is_user_input")
}

// devinWalkMainChain 沿 parent_node_id 从 main_chain_id 回走到根，
// 再反转成时间顺序。tip 缺失或未知时不猜 max(node_id)——
// 那常是已放弃的分叉，返回 missingTip。
func devinWalkMainChain(nodes []devinSlimNode, tip *int64) ([]devinSlimNode, bool) {
	if len(nodes) == 0 {
		return nil, false
	}
	byID := make(map[int64]devinSlimNode, len(nodes))
	for _, node := range nodes {
		byID[node.nodeID] = node
	}
	if tip == nil {
		return nil, true
	}
	cur, ok := byID[*tip]
	if !ok {
		return nil, true
	}
	var chain []devinSlimNode
	seen := make(map[int64]bool)
	for {
		if seen[cur.nodeID] {
			break
		}
		seen[cur.nodeID] = true
		chain = append(chain, cur)
		if cur.parentNodeID == nil {
			break
		}
		next, ok := byID[*cur.parentNodeID]
		if !ok {
			break
		}
		cur = next
	}
	for i, j := 0, len(chain)-1; i < j; i, j = i+1, j-1 {
		chain[i], chain[j] = chain[j], chain[i]
	}
	return chain, false
}

func devinBuildTurn(node devinSlimNode) *mem.DialogueTurn {
	if node.isSummary {
		turn := compactionBoundaryTurn(
			"此处上下文已压缩；上方的轮次仍在 Devin 会话库里", node.text)
		return &turn
	}
	if node.role == "" || node.text == "" {
		return nil
	}
	cleaned := stripInjectionTags(node.text)
	if cleaned == "" {
		return nil
	}
	if node.role == "user" && isBootstrapTurn(cleaned, len(node.text)) {
		return nil
	}
	return &mem.DialogueTurn{Role: node.role, Text: cleaned}
}

// openMemDB 是 SQLite 适配器的公共开库入口：文件缺失返回 opened=false
// 与一条警告；打开失败（读锁冲突重试后仍失败等）同样警告并跳过。
func openMemDB(ctx context.Context, dbPath, platformLabel string) (*sql.DB, []string, bool) {
	_ = ctx
	if !fileExists(dbPath) {
		return nil, []string{fmt.Sprintf("未找到 %s 会话库（%s），已跳过", platformLabel, dbPath)}, false
	}
	db, err := OpenReadOnly(dbPath)
	if err != nil {
		return nil, []string{fmt.Sprintf("无法读取 %s 会话库（%s）：%v", platformLabel, dbPath, err)}, false
	}
	return db, nil, true
}
