package adapters

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/linrong007/Sunshine/internal/channel/store"
)

// 以下 JSONL 样例取自上游研究探针的真实输出格式
//（上游任务归档 2026-05/05-12-agent-runtime/
// research/probes/claude/{hello,list-files}.jsonl，
// 字段已按测试需要截短，但结构与真实行一致，未凭空编造格式）。

const claudeInitLine = `{"type":"system","subtype":"init","cwd":"/tmp/probe","session_id":"3f7c7a78-b3de-4bda-bf36-9ce272181b08","tools":["Task","Bash"]}`

const claudeHookLine = `{"type":"system","subtype":"hook_started","hook_id":"475eb367-1395-441a-997b-f8f1c8fde540","hook_name":"SessionStart:startup","hook_event":"SessionStart","session_id":"3f7c7a78-b3de-4bda-bf36-9ce272181b08"}`

const claudeToolUseLine = `{"type":"assistant","message":{"model":"claude-opus-4-7","id":"msg_01Japq5a6yeMW8uXaF8imnmv","type":"message","role":"assistant","content":[{"type":"tool_use","id":"toolu_01UWtYMZBCMqzPfDfrZLq8pL","name":"Bash","input":{"command":"ls -1 | wc -l","description":"Count files in current directory"}}],"stop_reason":null,"usage":{"input_tokens":6,"cache_creation_input_tokens":25182,"cache_read_input_tokens":18748,"output_tokens":40}}}`

const claudeTextLine = `{"type":"assistant","message":{"model":"claude-opus-4-7","id":"msg_01Japq5a6yeMW8uXaF8imnmv","type":"message","role":"assistant","content":[{"type":"text","text":"当前目录有 6 个文件。"}],"usage":{"input_tokens":7,"output_tokens":12}}}`

const claudeThinkingLine = `{"type":"assistant","message":{"model":"claude-opus-4-7","type":"message","role":"assistant","content":[{"type":"thinking","thinking":"先数一下文件。"}]}}`

const claudeToolResultLine = `{"type":"user","message":{"role":"user","content":[{"tool_use_id":"toolu_01UWtYMZBCMqzPfDfrZLq8pL","type":"tool_result","content":"       6","is_error":false}]},"session_id":"1b151330-0f19-4888-b92c-c06fb278e9a4"}`

const claudeResultLine = `{"type":"result","subtype":"success","is_error":false,"duration_ms":10366,"duration_api_ms":8331,"num_turns":2,"result":"6 files.","session_id":"1b151330-0f19-4888-b92c-c06fb278e9a4","total_cost_usd":0.19218025,"usage":{"input_tokens":7,"cache_creation_input_tokens":25301,"cache_read_input_tokens":62678,"output_tokens":120}}`

const claudeErrorResultLine = `{"type":"result","subtype":"error","is_error":true,"duration_ms":1200,"result":"调用被拒绝","session_id":"1b151330-0f19-4888-b92c-c06fb278e9a4"}`

func TestParseClaudeInitPersistsSessionID(t *testing.T) {
	r := ParseClaudeLine(claudeInitLine)
	if len(r.Events) != 0 {
		t.Fatalf("init 行不应发事件，实际 %d 个", len(r.Events))
	}
	if r.Side == nil || r.Side.PersistSessionID != "3f7c7a78-b3de-4bda-bf36-9ce272181b08" {
		t.Fatalf("init 行应持久化 session_id，实际 %+v", r.Side)
	}
}

func TestParseClaudeHookLinesSkipped(t *testing.T) {
	if r := ParseClaudeLine(claudeHookLine); len(r.Events) != 0 || r.Side != nil {
		t.Fatalf("hook 行应跳过，实际 %+v", r)
	}
	if r := ParseClaudeLine(`{"type":"rate_limit_event"}`); len(r.Events) != 0 {
		t.Fatalf("rate_limit_event 应跳过，实际 %+v", r)
	}
	if r := ParseClaudeLine(`{"type":"control_response"}`); len(r.Events) != 0 {
		t.Fatalf("control_response 应跳过，实际 %+v", r)
	}
}

func TestParseClaudeToolUse(t *testing.T) {
	r := ParseClaudeLine(claudeToolUseLine)
	if len(r.Events) != 1 || r.Events[0].Kind != store.KindProgress {
		t.Fatalf("tool_use 应产生 1 个 progress 事件，实际 %+v", r.Events)
	}
	detail, _ := r.Events[0].Payload["detail"].(map[string]any)
	if detail["tool"] != "Bash" {
		t.Fatalf("tool 应为 Bash，实际 %v", detail["tool"])
	}
	if s, _ := detail["input_summary"].(string); !strings.Contains(s, "ls -1") {
		t.Fatalf("input_summary 应含命令，实际 %q", s)
	}
}

func TestParseClaudeMCPToolUse(t *testing.T) {
	line := `{"type":"assistant","message":{"role":"assistant","content":[{"type":"tool_use","id":"t1","name":"mcp__exa__search","input":{"query":"sunshine"}}]}}`
	r := ParseClaudeLine(line)
	if len(r.Events) != 1 {
		t.Fatalf("应产生 1 个事件，实际 %+v", r.Events)
	}
	detail, _ := r.Events[0].Payload["detail"].(map[string]any)
	if detail["kind"] != "mcp" || detail["server"] != "exa" || detail["tool_name"] != "search" {
		t.Fatalf("MCP 工具应拆出 kind/server/tool_name，实际 %v", detail)
	}
}

func TestParseClaudeTextAndUsage(t *testing.T) {
	r := ParseClaudeLine(claudeTextLine)
	if len(r.Events) != 1 || r.Events[0].Kind != store.KindMessage {
		t.Fatalf("文本块应产生 1 个 message 事件，实际 %+v", r.Events)
	}
	if r.Events[0].Payload["text"] != "当前目录有 6 个文件。" {
		t.Fatalf("文本不对：%v", r.Events[0].Payload["text"])
	}
	usage, _ := r.Events[0].Payload["usage"].(map[string]any)
	if usage["input_tokens"] != float64(7) || usage["output_tokens"] != float64(12) {
		t.Fatalf("应提取 token 用量，实际 %v", usage)
	}
}

func TestParseClaudeThinkingSkipped(t *testing.T) {
	if r := ParseClaudeLine(claudeThinkingLine); len(r.Events) != 0 {
		t.Fatalf("thinking 块应跳过，实际 %+v", r.Events)
	}
}

func TestParseClaudeToolResultExtracted(t *testing.T) {
	r := ParseClaudeLine(claudeToolResultLine)
	if len(r.Events) != 1 || r.Events[0].Kind != store.KindProgress {
		t.Fatalf("tool_result 应产生 1 个 progress 摘要事件，实际 %+v", r.Events)
	}
	detail, _ := r.Events[0].Payload["detail"].(map[string]any)
	if detail["tool_result"] != true || detail["tool_use_id"] != "toolu_01UWtYMZBCMqzPfDfrZLq8pL" {
		t.Fatalf("tool_result 摘要字段不对：%v", detail)
	}
	if s, _ := detail["output_summary"].(string); !strings.Contains(s, "6") {
		t.Fatalf("output_summary 应含输出，实际 %q", s)
	}
}

func TestParseClaudeResultDone(t *testing.T) {
	r := ParseClaudeLine(claudeResultLine)
	if len(r.Events) != 1 || r.Events[0].Kind != store.KindDone {
		t.Fatalf("成功 result 应产生 done，实际 %+v", r.Events)
	}
	p := r.Events[0].Payload
	if p["duration_ms"] != int64(10366) || p["num_turns"] != 2 {
		t.Fatalf("done 应带 duration_ms/num_turns，实际 %v", p)
	}
	if p["total_cost_usd"] != 0.19218025 {
		t.Fatalf("done 应带 total_cost_usd，实际 %v", p["total_cost_usd"])
	}
	if _, ok := p["text"]; ok {
		t.Fatalf("done 不应复制 result 文本（会让答案渲染两遍），实际 %v", p)
	}
	usage, _ := p["usage"].(map[string]any)
	if usage["output_tokens"] != float64(120) {
		t.Fatalf("done 应带 usage，实际 %v", usage)
	}
}

func TestParseClaudeResultError(t *testing.T) {
	r := ParseClaudeLine(claudeErrorResultLine)
	if len(r.Events) != 1 || r.Events[0].Kind != store.KindError {
		t.Fatalf("出错 result 应产生 error，实际 %+v", r.Events)
	}
	if r.Events[0].Payload["message"] != "调用被拒绝" {
		t.Fatalf("error 消息不对：%v", r.Events[0].Payload["message"])
	}
}

func TestParseClaudeBadJSON(t *testing.T) {
	r := ParseClaudeLine(`{不是 json`)
	if len(r.Events) != 1 || r.Events[0].Kind != store.KindError {
		t.Fatalf("坏行应产生 error 事件，实际 %+v", r.Events)
	}
	if r := ParseClaudeLine("   "); len(r.Events) != 0 {
		t.Fatalf("空行应跳过，实际 %+v", r.Events)
	}
}

func TestBuildClaudeArgs(t *testing.T) {
	args := BuildClaudeArgs(ClaudeArgs{SystemPrompt: "agent body"})
	joined := strings.Join(args, " ")
	for _, want := range []string{"-p", "--input-format stream-json", "--output-format stream-json", "--permission-mode bypassPermissions", "--dangerously-skip-permissions", "--verbose"} {
		if !strings.Contains(joined, want) {
			t.Fatalf("参数缺 %q：%v", want, args)
		}
	}
	if !strings.Contains(joined, "--append-system-prompt agent body") {
		t.Fatalf("应内联系统提示词：%v", args)
	}
	// 空提示词不带 flag。
	args = BuildClaudeArgs(ClaudeArgs{SystemPrompt: "   "})
	for _, a := range args {
		if a == "--append-system-prompt" || a == "--append-system-prompt-file" {
			t.Fatalf("空提示词不应带系统提示词 flag：%v", args)
		}
	}
	// 文件优先于内联，且超长提示词不出现在 argv 里。
	big := strings.Repeat("x", 300000)
	args = BuildClaudeArgs(ClaudeArgs{SystemPrompt: big, SystemPromptFile: "/tmp/worker.system-prompt.md"})
	if strings.Contains(strings.Join(args, " "), "--append-system-prompt ") {
		t.Fatalf("用文件时不应再内联：%v", args[:8])
	}
	found := false
	for i, a := range args {
		if a == "--append-system-prompt-file" && args[i+1] == "/tmp/worker.system-prompt.md" {
			found = true
		}
	}
	if !found {
		t.Fatalf("应用文件路径：%v", args)
	}
	// resume / model。
	args = BuildClaudeArgs(ClaudeArgs{Resume: "sess-1", Model: "claude-opus-4-7"})
	joined = strings.Join(args, " ")
	if !strings.Contains(joined, "--resume sess-1") || !strings.Contains(joined, "--model claude-opus-4-7") {
		t.Fatalf("resume/model 参数不对：%v", args)
	}
}

func TestShouldUseSystemPromptFile(t *testing.T) {
	if ShouldUseSystemPromptFile("") || ShouldUseSystemPromptFile("   \n  ") || ShouldUseSystemPromptFile("agent body") {
		t.Fatal("空与小提示词应走内联")
	}
	if ShouldUseSystemPromptFile(strings.Repeat("x", InlineSystemPromptMaxChars)) {
		t.Fatal("正好等于上限时仍应内联")
	}
	if !ShouldUseSystemPromptFile(strings.Repeat("x", InlineSystemPromptMaxChars+1)) {
		t.Fatal("超上限 1 字就应走文件")
	}
	if !ShouldUseSystemPromptFile(strings.Repeat("y", 300000)) {
		t.Fatal("注入整篇设计文档时应走文件")
	}
}

func TestEncodeClaudeMessages(t *testing.T) {
	line := EncodeClaudeUserMessage("你好")
	var obj map[string]any
	if err := json.Unmarshal([]byte(line), &obj); err != nil {
		t.Fatalf("用户消息应为合法 JSON：%v", err)
	}
	if obj["type"] != "user" {
		t.Fatalf("type 应为 user：%v", obj)
	}
	interrupted := EncodeClaudeInterruptMessage("改做 B")
	if !strings.Contains(interrupted, "control_request") || !strings.Contains(interrupted, `"subtype":"interrupt"`) {
		t.Fatalf("中断应先发 control_request 中断帧：%s", interrupted)
	}
	if !strings.Contains(interrupted, "改做 B") {
		t.Fatalf("中断应带替换指令：%s", interrupted)
	}
}
