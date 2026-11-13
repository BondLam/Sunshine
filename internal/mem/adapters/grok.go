package adapters

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/linrong007/Sunshine/internal/mem"
)

// Grok 会话布局：~/.grok/sessions/<cwd url-encoded>/<session-id>/
// chat_history.jsonl，同目录的 summary.json 提供元数据
//（info.id、info.cwd、session_summary、created_at、updated_at）。
// 同目录的 session_search.sqlite 只是搜索索引，不读。
//
// 事件形态（顶层 type）：
//   user：对话；带 synthetic_reason 的除外（注入提醒、项目指令、
//     后台任务通知）；synthetic_reason 为 compaction_meta 时是压缩边界
//   assistant：对话，content 是普通字符串
//   reasoning：丢弃（加密思维链）
//   tool_result / system / backend_tool_call：丢弃
//
// 压缩：Grok 注入 synthetic_reason 为 compaction_meta 的 user 事件，
// 文件从压缩后的状态开始写，边界前的轮次不在 chat_history.jsonl 里；
// 标注边界并警告，不假装会话完整。

func grokSessions(ctx context.Context, home string) ([]mem.MemSessionInfo, []string, error) {
	root := GrokSessionsDir(home)
	if !dirExists(root) {
		return nil, []string{fmt.Sprintf("未找到 Grok 会话存储（%s），已跳过", root)}, nil
	}
	projects, err := os.ReadDir(root)
	if err != nil {
		return nil, []string{fmt.Sprintf("无法读取 Grok 会话存储（%s）：%v，已跳过", root, err)}, nil
	}
	var out []mem.MemSessionInfo
	for _, project := range projects {
		if ctx.Err() != nil {
			return out, nil, ctx.Err()
		}
		if !project.IsDir() {
			continue
		}
		projectDir := filepath.Join(root, project.Name())
		sessions, err := os.ReadDir(projectDir)
		if err != nil {
			continue
		}
		for _, session := range sessions {
			if !session.IsDir() {
				continue
			}
			sessionDir := filepath.Join(projectDir, session.Name())
			filePath := filepath.Join(sessionDir, "chat_history.jsonl")
			if !fileExists(filePath) {
				continue
			}
			summary := ReadJSONFile(filepath.Join(sessionDir, "summary.json"))
			info := objMap(summary, "info")
			id := objStr(info, "id")
			if id == "" {
				id = session.Name()
			}
			cwd := objStr(info, "cwd")
			if cwd == "" {
				cwd = GrokCwdFromProjectDir(project.Name())
			}
			created := objStr(summary, "created_at")
			updated := objStr(summary, "updated_at")
			if updated == "" {
				if stat, err := os.Stat(filePath); err == nil {
					updated = formatISOTime(stat.ModTime())
				}
			}
			title := strings.TrimSpace(objStr(summary, "session_summary"))
			out = append(out, mem.MemSessionInfo{
				Platform: mem.MemSourceGrok,
				ID:       id,
				Title:    title,
				CWD:      cwd,
				Created:  created,
				Updated:  updated,
				FilePath: filePath,
			})
		}
	}
	return out, nil, nil
}

func grokDialogue(ctx context.Context, s mem.MemSessionInfo) ([]mem.DialogueTurn, []string, error) {
	turns, _, warnings, err := grokCollect(ctx, s)
	return turns, warnings, err
}

// grokCollect 一次遍历同时产出清理后的对话轮次与任务命令事件：
// 助手事件的 `run_terminal_command` 工具调用里藏着
// `sunshine task create|start` 调用，事件的 turnIndex 是调用时
// 已产出的轮次数（工具调用属于正在生成的这轮助手回复，轮次尚未入池）。
// Grok 事件不带调用时间戳，事件的 Timestamp 为空。
func grokCollect(ctx context.Context, s mem.MemSessionInfo) ([]mem.DialogueTurn, []mem.MemEvent, []string, error) {
	var turns []mem.DialogueTurn
	var events []mem.MemEvent
	compactions := 0
	warnings := ReadJSONL(s.FilePath, func(obj map[string]any) bool {
		if ctx.Err() != nil {
			return false
		}
		switch objStr(obj, "type") {
		case "user":
			reason := objStr(obj, "synthetic_reason")
			if reason == "compaction_meta" {
				compactions++
				text, _ := grokText(obj["content"])
				turns = append(turns, compactionBoundaryTurn(
					"此处上下文已压缩；Grok 从压缩后的状态开始写本文件，边界前的轮次不在其中", text))
				return true
			}
			// 其余 synthetic_reason 都是注入提醒或后台任务通知，不是用户说的话。
			if reason != "" {
				return true
			}
			text, rawLength := grokText(obj["content"])
			if text != "" && !isBootstrapTurn(text, rawLength) {
				turns = append(turns, mem.DialogueTurn{Role: "user", Text: text})
			}
			return true
		case "assistant":
			for _, call := range objList(obj, "tool_calls") {
				callMap, ok := call.(map[string]any)
				if !ok || objStr(callMap, "name") != "run_terminal_command" {
					continue
				}
				cmd := commandFromCodexArguments(callMap["arguments"])
				if cmd == "" {
					continue
				}
				events = append(events, mem.EventsFromParsed(
					mem.ParseTaskCommandsAll(cmd), "", len(turns))...)
			}
			text, _ := grokText(obj["content"])
			if text != "" {
				turns = append(turns, mem.DialogueTurn{Role: "assistant", Text: text})
			}
			return true
		default:
			// reasoning / tool_result / system / backend_tool_call
			// 与未知事件类型一律容忍、跳过。
			return true
		}
	})
	if compactions > 0 {
		archive := filepath.Join(filepath.Dir(s.FilePath), "compaction")
		warnings = append(warnings, fmt.Sprintf("会话 %s：压缩了 %d 次；每次边界前的轮次都不在 chat_history.jsonl 里，无法作为对话恢复；Grok 在 %s/ 下另存了一份渲染版记录，可去那里读", s.ID, compactions, archive))
	}
	return turns, events, warnings, nil
}

// grokText 取出 Grok 事件的文本：字符串原样取；数组只取 type 为 text 的块。
func grokText(content any) (string, int) {
	if text, ok := content.(string); ok {
		return stripInjectionTags(text), len(text)
	}
	blocks, ok := content.([]any)
	if !ok {
		return "", 0
	}
	var parts []string
	rawLength := 0
	for _, b := range blocks {
		block, ok := b.(map[string]any)
		if !ok || objStr(block, "type") != "text" {
			continue
		}
		text, ok := block["text"].(string)
		if !ok {
			continue
		}
		rawLength += len(text)
		if cleaned := stripInjectionTags(text); cleaned != "" {
			parts = append(parts, cleaned)
		}
	}
	return strings.Join(parts, "\n\n"), rawLength
}
