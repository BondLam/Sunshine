package supervisor

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os"

	"github.com/linrong007/Sunshine/internal/channel/adapters"
)

// RunParseTrace 是内部开发用解析命令 `sunshine channel __parse-trace` 的实现
// （供 C 组挂接）：把一段录制的 stream-json / 线路 trace 喂给对应适配器，
// 逐行打印解析出的频道事件（JSON 行），最后打印结构化摘要。
//
// 注意：codex trace 只含入站行（服务端 → 我们）；探针夹具录制的出站请求 id
// 不在 trace 里；这里按上游做法预置 ctx 的待处理 id（1 为 initialize、
// 2 为 thread/start、3 为 turn/start），id 匹配才能工作。
func RunParseTrace(provider adapters.Provider, file string, w io.Writer) error {
	f, err := os.Open(file)
	if err != nil {
		return fmt.Errorf("读 trace 文件失败：%w", err)
	}
	defer f.Close()

	counts := map[string]int{}
	totalLines := 0
	totalEvents := 0
	printEvent := func(lineNo int, ev adapters.AdapterEvent) {
		out := map[string]any{"line": lineNo, "kind": string(ev.Kind)}
		for k, v := range ev.Payload {
			out[k] = v
		}
		writeTraceJSON(w, out)
		counts[string(ev.Kind)]++
		totalEvents++
	}

	if provider == adapters.ProviderClaude {
		scanner := bufio.NewScanner(f)
		scanner.Buffer(make([]byte, 64*1024), 4*1024*1024)
		for scanner.Scan() {
			totalLines++
			line := scanner.Text()
			if len(line) == 0 {
				continue
			}
			result := adapters.ParseClaudeLine(line)
			printTraceResult(w, totalLines, result, printEvent)
		}
		if err := scanner.Err(); err != nil {
			return fmt.Errorf("读 trace 文件失败：%w", err)
		}
	} else if provider == adapters.ProviderCodex {
		ctx := adapters.NewCodexCtx()
		ctx.Pending[1] = "initialize"
		ctx.Pending[2] = "thread/start"
		ctx.Pending[3] = "turn/start"
		ctx.NextID = 4
		scanner := bufio.NewScanner(f)
		scanner.Buffer(make([]byte, 64*1024), 4*1024*1024)
		for scanner.Scan() {
			totalLines++
			line := scanner.Text()
			if len(line) == 0 {
				continue
			}
			result := adapters.ParseCodexLine(line, ctx)
			printTraceResult(w, totalLines, result, printEvent)
		}
		if err := scanner.Err(); err != nil {
			return fmt.Errorf("读 trace 文件失败：%w", err)
		}
	} else {
		return fmt.Errorf("无效的适配器 %q，应为 claude 或 codex", string(provider))
	}

	// 结构化摘要行。
	summary := map[string]any{
		"line":           0,
		"kind":           "<摘要>",
		"adapter":        string(provider),
		"total_lines":    totalLines,
		"total_events":   totalEvents,
		"events_by_kind": counts,
	}
	writeTraceJSON(w, summary)
	return nil
}

func printTraceResult(w io.Writer, lineNo int, result adapters.ParseResult, printEvent func(int, adapters.AdapterEvent)) {
	for _, ev := range result.Events {
		printEvent(lineNo, ev)
	}
	if result.Side == nil {
		return
	}
	if result.Side.PersistSessionID != "" || result.Side.PersistThreadID != "" {
		persist := map[string]any{"line": lineNo, "kind": "<side-effect>"}
		if result.Side.PersistSessionID != "" {
			persist["persistSessionId"] = result.Side.PersistSessionID
		}
		if result.Side.PersistThreadID != "" {
			persist["persistThreadId"] = result.Side.PersistThreadID
		}
		writeTraceJSON(w, persist)
	}
	for _, reply := range result.Side.Reply {
		writeTraceJSON(w, map[string]any{
			"line": lineNo, "kind": "<outbound>", "text": reply,
		})
	}
	for _, r := range result.Side.Resolved {
		entry := map[string]any{"line": lineNo, "kind": "<rpc-resolved>", "id": r.ID}
		if r.Result != nil {
			entry["result"] = r.Result
		}
		if r.Error != nil {
			entry["error"] = r.Error
		}
		writeTraceJSON(w, entry)
	}
}

func writeTraceJSON(w io.Writer, obj map[string]any) {
	// encoding/json 会按字典序输出 map 键，输出稳定可读。
	data, err := json.Marshal(obj)
	if err != nil {
		return
	}
	_, _ = w.Write(append(data, '\n'))
}
