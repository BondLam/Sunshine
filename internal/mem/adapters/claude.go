package adapters

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/linrong1982/Sunshine/internal/mem"
)

// Claude 会话布局：~/.claude/projects/<清理后的 cwd>/<sessionId>.jsonl，
// 可选的 <项目目录>/sessions-index.json 提供 cwd / created / title。

type claudeIndexEntry struct {
	ID      string `json:"id"`
	CWD     string `json:"cwd"`
	Created string `json:"created"`
	Title   string `json:"title"`
}

func claudeSessions(ctx context.Context, home string) ([]mem.MemSessionInfo, []string, error) {
	root := ClaudeProjectsDir(home)
	if !dirExists(root) {
		return nil, []string{fmt.Sprintf("未找到 Claude 会话存储（%s），已跳过", root)}, nil
	}
	dirs, err := os.ReadDir(root)
	if err != nil {
		return nil, []string{fmt.Sprintf("无法读取 Claude 会话存储（%s）：%v，已跳过", root, err)}, nil
	}
	var out []mem.MemSessionInfo
	for _, dir := range dirs {
		if ctx.Err() != nil {
			return out, nil, ctx.Err()
		}
		if !dir.IsDir() {
			continue
		}
		projectDir := filepath.Join(root, dir.Name())
		out = append(out, claudeSessionsInDir(projectDir)...)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Updated != out[j].Updated {
			return out[i].Updated > out[j].Updated
		}
		return out[i].ID < out[j].ID
	})
	return out, nil, nil
}

func claudeSessionsInDir(dir string) []mem.MemSessionInfo {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	indexByID := make(map[string]claudeIndexEntry)
	if data, err := os.ReadFile(filepath.Join(dir, "sessions-index.json")); err == nil {
		var index struct {
			Entries []claudeIndexEntry `json:"entries"`
		}
		if json.Unmarshal(data, &index) == nil {
			for _, e := range index.Entries {
				if e.ID != "" {
					indexByID[e.ID] = e
				}
			}
		}
	}
	var out []mem.MemSessionInfo
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".jsonl") {
			continue
		}
		filePath := filepath.Join(dir, e.Name())
		id := strings.TrimSuffix(e.Name(), ".jsonl")
		idx := indexByID[id]
		cwd, created, title := idx.CWD, idx.Created, idx.Title
		if cwd == "" || created == "" {
			evt, _ := FindInJSONL(filePath, 100, func(obj map[string]any) bool {
				return objStr(obj, "cwd") != ""
			})
			if cwd == "" && evt != nil {
				cwd = objStr(evt, "cwd")
			}
			if created == "" {
				if evt != nil {
					created = objStr(evt, "timestamp")
				}
				if created == "" {
					if first, _ := ReadJSONLFirst(filePath); first != nil {
						created = objStr(first, "timestamp")
					}
				}
			}
		}
		updated := ""
		if info, err := os.Stat(filePath); err == nil {
			updated = formatISOTime(info.ModTime())
		}
		out = append(out, mem.MemSessionInfo{
			Platform: mem.MemSourceClaude,
			ID:       id,
			Title:    title,
			CWD:      cwd,
			Created:  created,
			Updated:  updated,
			FilePath: filePath,
		})
	}
	return out
}

func claudeDialogue(ctx context.Context, s mem.MemSessionInfo) ([]mem.DialogueTurn, []string, error) {
	turns, _, warnings, err := claudeCollect(ctx, s)
	return turns, warnings, err
}

// claudeCollect 一次遍历同时产出清理后的对话轮次与任务命令事件：
// 助手消息里的 Bash tool_use 块藏着 `sunshine task create|start` 调用，
// 事件的 turnIndex 是该调用发生时已产出的轮次数
// （工具调用属于正在生成的这轮助手回复，轮次尚未入池）。
func claudeCollect(ctx context.Context, s mem.MemSessionInfo) ([]mem.DialogueTurn, []mem.MemEvent, []string, error) {
	var turns []mem.DialogueTurn
	var events []mem.MemEvent
	warnings := ReadJSONL(s.FilePath, func(obj map[string]any) bool {
		if ctx.Err() != nil {
			return false
		}
		msg := objMap(obj, "message")
		if msg == nil {
			return true
		}
		typ := objStr(obj, "type")
		content := msg["content"]
		if typ == "user" && objBool(obj, "isCompactSummary") {
			turns = append(turns, compactionBoundaryTurn(
				"此处上下文已压缩；上方的轮次是 Claude 摘要前的对话",
				claudeText(content),
			))
			return true
		}
		if typ == "user" && objStr(msg, "role") == "user" {
			if text, ok := content.(string); ok {
				cleaned := stripInjectionTags(text)
				if cleaned != "" && !isBootstrapTurn(cleaned, len(text)) {
					turns = append(turns, mem.DialogueTurn{Role: "user", Text: cleaned})
				}
			}
			return true
		}
		if typ == "assistant" && objStr(msg, "role") == "assistant" {
			if blocks, ok := content.([]any); ok {
				var parts []string
				for _, b := range blocks {
					block, ok := b.(map[string]any)
					if !ok {
						continue
					}
					if objStr(block, "type") == "tool_use" && objStr(block, "name") == "Bash" {
						if input := objMap(block, "input"); input != nil {
							if command, ok := input["command"].(string); ok {
								events = append(events, mem.EventsFromParsed(
									mem.ParseTaskCommandsAll(command), objStr(obj, "timestamp"), len(turns))...)
							}
						}
						continue
					}
					if objStr(block, "type") != "text" {
						continue
					}
					if text, ok := block["text"].(string); ok {
						if cleaned := stripInjectionTags(text); cleaned != "" {
							parts = append(parts, cleaned)
						}
					}
				}
				if len(parts) > 0 {
					turns = append(turns, mem.DialogueTurn{Role: "assistant", Text: strings.Join(parts, "\n\n")})
				}
			}
			return true
		}
		// 其余事件类型（tool_result、errors、telemetry 等）一律容忍、跳过。
		return true
	})
	return turns, events, warnings, nil
}

// claudeText 取出 Claude 消息体里的文本：字符串原样取；数组只取
// type 为 text 的块，thinking / tool_use 块整段丢弃，注入标签剥离。
func claudeText(content any) string {
	if text, ok := content.(string); ok {
		return stripInjectionTags(text)
	}
	blocks, ok := content.([]any)
	if !ok {
		return ""
	}
	var parts []string
	for _, b := range blocks {
		block, ok := b.(map[string]any)
		if !ok {
			continue
		}
		if objStr(block, "type") != "text" {
			continue
		}
		if text, ok := block["text"].(string); ok {
			if cleaned := stripInjectionTags(text); cleaned != "" {
				parts = append(parts, cleaned)
			}
		}
	}
	return strings.Join(parts, "\n\n")
}
