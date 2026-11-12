package adapters

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/BondLam/Sunshine/internal/mem"
)

// Pi Agent 会话布局：~/.pi/agent/sessions/--<编码后的 cwd>--/
// <timestamp>_<id>.jsonl；或环境变量 / settings 配置的自定义
// 会话目录下的直接 .jsonl 文件。条目以 id / parentId 组成树，
// 提取时沿整条活动分支走（以最后一条条目为叶）。
//
// 压缩：Pi 写一条 compaction 条目，点名它保留的第一条条目；
// 这是 Pi 的上下文规则，不是对话记录——更早的条目仍在活动路径上，
// 提取时保留全路径，把压缩条目就地渲染成边界 marker。

func piSessions(ctx context.Context, home string) ([]mem.MemSessionInfo, []string, error) {
	files := piCandidateFiles(home)
	if len(files) == 0 {
		return nil, []string{fmt.Sprintf("未找到 Pi 会话存储（%s），已跳过", PiSessionsDir(home))}, nil
	}
	var out []mem.MemSessionInfo
	for _, filePath := range files {
		if ctx.Err() != nil {
			return out, nil, ctx.Err()
		}
		header, _ := ReadJSONLFirst(filePath)
		if header == nil || objStr(header, "type") != "session" {
			continue
		}
		id := objStr(header, "id")
		if id == "" {
			id = piIDFromFile(filePath)
		}
		cwd := objStr(header, "cwd")
		var title string
		var lastActivityMs *int64
		ReadJSONL(filePath, func(entry map[string]any) bool {
			switch objStr(entry, "type") {
			case "session_info":
				if name := strings.TrimSpace(objStr(entry, "name")); name != "" {
					title = name
				}
				return true
			case "message":
				msg := objMap(entry, "message")
				if msg == nil {
					return true
				}
				role := objStr(msg, "role")
				if role != "user" && role != "assistant" {
					return true
				}
				ms := piTimestampMs(msg["timestamp"])
				if ms == nil {
					ms = piTimestampMs(entry["timestamp"])
				}
				if ms != nil && (lastActivityMs == nil || *ms > *lastActivityMs) {
					lastActivityMs = ms
				}
				return true
			default:
				return true
			}
		})
		updated := ""
		if lastActivityMs != nil {
			updated = formatISOTime(time.UnixMilli(*lastActivityMs))
		} else if info, err := os.Stat(filePath); err == nil {
			updated = formatISOTime(info.ModTime())
		}
		created := objStr(header, "timestamp")
		out = append(out, mem.MemSessionInfo{
			Platform: mem.MemSourcePi,
			ID:       id,
			Title:    title,
			CWD:      cwd,
			Created:  created,
			Updated:  updated,
			FilePath: filePath,
		})
	}
	return out, nil, nil
}

// piCandidateFiles 列出各 Pi 会话根下的 .jsonl 文件（去重、排序）。
func piCandidateFiles(home string) []string {
	seen := make(map[string]bool)
	var out []string
	for _, root := range PiSessionRoots(home, "") {
		if !dirExists(root) {
			continue
		}
		for _, file := range WalkDir(root) {
			if !strings.HasSuffix(file, ".jsonl") {
				continue
			}
			normalized := filepath.Clean(file)
			if seen[normalized] {
				continue
			}
			seen[normalized] = true
			out = append(out, file)
		}
	}
	return out
}

// piIDFromFile 从文件名取会话 id：<timestamp>_<id>.jsonl 取首个下划线之后。
func piIDFromFile(filePath string) string {
	base := strings.TrimSuffix(filepath.Base(filePath), ".jsonl")
	if idx := strings.Index(base, "_"); idx != -1 {
		return base[idx+1:]
	}
	return base
}

// piTimestampMs 把数字或 ISO 字符串时间戳转成毫秒；解析不到返回 nil。
func piTimestampMs(value any) *int64 {
	switch v := value.(type) {
	case float64:
		ms := int64(v)
		return &ms
	case string:
		if parsed, err := time.Parse(time.RFC3339, v); err == nil {
			ms := parsed.UnixMilli()
			return &ms
		}
		if parsed, err := time.Parse("2006-01-02T15:04:05.999Z07:00", v); err == nil {
			ms := parsed.UnixMilli()
			return &ms
		}
	}
	return nil
}

func piDialogue(ctx context.Context, s mem.MemSessionInfo) ([]mem.DialogueTurn, []string, error) {
	turns, _, warnings, err := piCollect(ctx, s)
	return turns, warnings, err
}

// piCollect 沿活动分支一次遍历同时产出清理后的对话轮次与任务命令事件：
// 助手消息的 toolCall 块（name 为 bash/shell）与 bashExecution 角色
// 的消息里藏着 `sunshine task create|start` 调用，事件的 turnIndex 是
// 该条目自身轮次入池前的轮次数。分叉条目不在活动分支上，不参与。
func piCollect(ctx context.Context, s mem.MemSessionInfo) ([]mem.DialogueTurn, []mem.MemEvent, []string, error) {
	activePath, warnings := piActivePath(s.FilePath)
	var turns []mem.DialogueTurn
	var events []mem.MemEvent
	for _, entry := range activePath {
		if ctx.Err() != nil {
			return turns, events, warnings, ctx.Err()
		}
		events = append(events, piTaskEvents(entry, len(turns))...)
		if turn := piTurnFromEntry(entry); turn != nil {
			turns = append(turns, *turn)
		}
	}
	return turns, events, warnings, nil
}

// piTaskEvents 从一条 Pi 条目里抽任务命令事件：只看 message 类型条目；
// bashExecution 角色直接取 message.command，助手消息取 content 里
// type 为 toolCall、name 为 bash/shell 的块的 arguments.command。
// 时间戳取条目的 timestamp（字符串原样取，数字按毫秒转 ISO）。
func piTaskEvents(entry map[string]any, turnIndex int) []mem.MemEvent {
	if objStr(entry, "type") != "message" {
		return nil
	}
	msg := objMap(entry, "message")
	if msg == nil {
		return nil
	}
	ts := piEventTimestamp(entry)
	if objStr(msg, "role") == "bashExecution" {
		if command, ok := msg["command"].(string); ok && command != "" {
			return mem.EventsFromParsed(mem.ParseTaskCommandsAll(command), ts, turnIndex)
		}
		return nil
	}
	if objStr(msg, "role") != "assistant" {
		return nil
	}
	blocks, ok := msg["content"].([]any)
	if !ok {
		return nil
	}
	var events []mem.MemEvent
	for _, b := range blocks {
		block, ok := b.(map[string]any)
		if !ok || objStr(block, "type") != "toolCall" {
			continue
		}
		name := strings.ToLower(objStr(block, "name"))
		if name != "bash" && name != "shell" {
			continue
		}
		args := objMap(block, "arguments")
		if args == nil {
			continue
		}
		command, ok := args["command"].(string)
		if !ok || command == "" {
			continue
		}
		events = append(events, mem.EventsFromParsed(
			mem.ParseTaskCommandsAll(command), ts, turnIndex)...)
	}
	return events
}

// piEventTimestamp 取 Pi 条目的时间戳：字符串原样取；
// 数字按毫秒转 ISO；取不到时返回 ""。
func piEventTimestamp(entry map[string]any) string {
	if ts, ok := entry["timestamp"].(string); ok {
		return ts
	}
	if ms := piTimestampMs(entry["timestamp"]); ms != nil {
		return formatISOTime(time.UnixMilli(*ms))
	}
	return ""
}

// piActivePath 从最后一条条目沿 parent 链回走，得到会话结束时
// 所在的分支，按时间顺序返回。压缩不截断：compaction 条目是 Pi 的
// 上下文规则，它排除的条目仍在这条路径上。
func piActivePath(filePath string) ([]map[string]any, []string) {
	var entries []map[string]any
	warnings := ReadJSONL(filePath, func(entry map[string]any) bool {
		if objStr(entry, "type") == "session" {
			return true
		}
		if objStr(entry, "id") == "" {
			return true
		}
		entries = append(entries, entry)
		return true
	})
	if len(entries) == 0 {
		return nil, warnings
	}
	byID := make(map[string]map[string]any, len(entries))
	for _, entry := range entries {
		byID[objStr(entry, "id")] = entry
	}
	leaf := entries[len(entries)-1]
	var path []map[string]any
	seen := make(map[string]bool)
	current := leaf
	for current != nil {
		id := objStr(current, "id")
		if id == "" || seen[id] {
			break
		}
		seen[id] = true
		path = append([]map[string]any{current}, path...)
		parentID := objStr(current, "parentId")
		if parentID == "" {
			break
		}
		current = byID[parentID]
	}
	return path, warnings
}

func piTurnFromEntry(entry map[string]any) *mem.DialogueTurn {
	switch objStr(entry, "type") {
	case "compaction":
		turn := piCompactionBoundary(entry["summary"])
		return &turn
	case "branch_summary":
		return piSyntheticTurn("[branch summary]", entry["summary"])
	case "custom_message":
		return piBuildTurn("user", entry["content"])
	case "message":
		msg := objMap(entry, "message")
		if msg == nil {
			return nil
		}
		switch objStr(msg, "role") {
		case "user":
			return piBuildTurn("user", msg["content"])
		case "assistant":
			return piBuildTurn("assistant", msg["content"])
		case "custom":
			return piBuildTurn("user", msg["content"])
		case "branchSummary":
			return piSyntheticTurn("[branch summary]", msg["summary"])
		case "compactionSummary":
			turn := piCompactionBoundary(msg["summary"])
			return &turn
		}
	}
	return nil
}

func piCompactionBoundary(raw any) mem.DialogueTurn {
	summary, _ := raw.(string)
	return compactionBoundaryTurn(
		"此处上下文已压缩；上方的轮次仍留在活动分支上",
		stripInjectionTags(summary))
}

func piSyntheticTurn(prefix string, raw any) *mem.DialogueTurn {
	text, ok := raw.(string)
	if !ok {
		return nil
	}
	cleaned := stripInjectionTags(text)
	if cleaned == "" {
		return nil
	}
	return &mem.DialogueTurn{Role: "user", Text: prefix + "\n" + cleaned}
}

// piBuildTurn 从字符串或内容块数组里取文本：只取 type 为 text 的块，
// 注入标签剥离，启动注入轮次丢弃。
func piBuildTurn(role string, content any) *mem.DialogueTurn {
	var parts []string
	totalRaw := 0
	if text, ok := content.(string); ok {
		totalRaw = len(text)
		if cleaned := stripInjectionTags(text); cleaned != "" {
			parts = append(parts, cleaned)
		}
	} else if blocks, ok := content.([]any); ok {
		for _, b := range blocks {
			block, ok := b.(map[string]any)
			if !ok || objStr(block, "type") != "text" {
				continue
			}
			text, ok := block["text"].(string)
			if !ok {
				continue
			}
			totalRaw += len(text)
			if cleaned := stripInjectionTags(text); cleaned != "" {
				parts = append(parts, cleaned)
			}
		}
	}
	if len(parts) == 0 {
		return nil
	}
	merged := strings.Join(parts, "\n\n")
	if isBootstrapTurn(merged, totalRaw) {
		return nil
	}
	return &mem.DialogueTurn{Role: role, Text: merged}
}
