package adapters

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/linrong1982/Sunshine/internal/channel/store"
)

// 以下 JSONL 样例取自上游研究探针的真实输出格式
//（上游任务归档 2026-05/05-12-agent-runtime/
// research/probes/codex/{hello,list-files,mcp-call}.jsonl，
// 字段已按测试需要截短，但结构与真实行一致，未凭空编造格式）。

const codexThreadID = "019e1b39-2ec1-7fe0-979e-5cfc133b0805"

func codexCtxWithThread() *CodexCtx {
	ctx := NewCodexCtx()
	ctx.ThreadID = codexThreadID
	return ctx
}

func TestParseCodexInitializeResponse(t *testing.T) {
	ctx := NewCodexCtx()
	ctx.Pending[1] = "initialize"
	ctx.NextID = 2
	r := ParseCodexLine(`{"id":1,"result":{"userAgent":"probe/0.130.0","codexHome":"/Users/x/.codex"}}`, ctx)
	if len(r.Events) != 0 {
		t.Fatalf("initialize 响应不应发事件，实际 %+v", r.Events)
	}
	if len(r.Side.Resolved) != 1 || r.Side.Resolved[0].ID != 1 {
		t.Fatalf("应标记 id=1 已解决，实际 %+v", r.Side)
	}
	if _, ok := ctx.Pending[1]; ok {
		t.Fatal("已解决的 id 应从待处理表里删掉")
	}
}

func TestParseCodexThreadStartPersistsIDs(t *testing.T) {
	ctx := NewCodexCtx()
	ctx.Pending[2] = "thread/start"
	ctx.NextID = 3
	line := `{"id":2,"result":{"thread":{"id":"` + codexThreadID + `","sessionId":"` + codexThreadID + `","status":{"type":"idle"}},"model":"gpt-5.3-codex-spark"}}`
	r := ParseCodexLine(line, ctx)
	if len(r.Events) != 0 {
		t.Fatalf("thread/start 响应不应发事件，实际 %+v", r.Events)
	}
	if r.Side.PersistThreadID != codexThreadID || r.Side.PersistSessionID != codexThreadID {
		t.Fatalf("应持久化 threadId 与 sessionId（同一值），实际 %+v", r.Side)
	}
	if ctx.ThreadID != codexThreadID {
		t.Fatalf("ctx 应记下 threadId，实际 %q", ctx.ThreadID)
	}
}

func TestParseCodexSkipMethods(t *testing.T) {
	ctx := codexCtxWithThread()
	for _, line := range []string{
		`{"method":"remoteControl/status/changed","params":{"status":"disabled"}}`,
		`{"method":"mcpServer/startupStatus/updated","params":{"name":"exa","status":"starting"}}`,
		`{"method":"thread/tokenUsage/updated","params":{"threadId":"x","tokenUsage":{"total":{"totalTokens":1}}}}`,
		`{"method":"thread/started","params":{}}`,
		`{"method":"turn/started","params":{}}`,
	} {
		// 注意：thread/tokenUsage/updated 不在跳过之列（本包提取用量），
		// 其余四行应跳过。
		r := ParseCodexLine(line, ctx)
		if strings.Contains(line, "tokenUsage") {
			continue
		}
		if len(r.Events) != 0 {
			t.Fatalf("应跳过 %s，实际 %+v", line, r.Events)
		}
	}
}

func TestParseCodexTokenUsage(t *testing.T) {
	ctx := codexCtxWithThread()
	line := `{"method":"thread/tokenUsage/updated","params":{"threadId":"` + codexThreadID + `","tokenUsage":{"total":{"totalTokens":20029,"inputTokens":19570,"cachedInputTokens":6144,"outputTokens":459,"reasoningOutputTokens":448},"last":{"totalTokens":20029,"inputTokens":19570,"outputTokens":459}}}}`
	r := ParseCodexLine(line, ctx)
	if len(r.Events) != 1 || r.Events[0].Kind != store.KindProgress {
		t.Fatalf("tokenUsage 应产生 progress 事件，实际 %+v", r.Events)
	}
	detail, _ := r.Events[0].Payload["detail"].(map[string]any)
	if detail["kind"] != "token_usage" {
		t.Fatalf("detail.kind 应为 token_usage，实际 %v", detail)
	}
	usage, _ := detail["usage"].(map[string]any)
	total, _ := usage["total"].(map[string]any)
	if total["totalTokens"] != float64(20029) {
		t.Fatalf("应带 total 用量，实际 %v", usage)
	}
}

func TestParseCodexCommandExecution(t *testing.T) {
	ctx := codexCtxWithThread()
	started := `{"method":"item/started","params":{"item":{"type":"commandExecution","id":"call_H7Bi1lB0n22XvmNW2gDwfpuG","command":"/bin/zsh -lc 'ls -1 | wc -l'","status":"inProgress"},"threadId":"` + codexThreadID + `"}}`
	r := ParseCodexLine(started, ctx)
	if len(r.Events) != 1 || r.Events[0].Kind != store.KindProgress {
		t.Fatalf("commandExecution 开始应产生 progress，实际 %+v", r.Events)
	}
	detail, _ := r.Events[0].Payload["detail"].(map[string]any)
	if detail["tool"] != "shell" {
		t.Fatalf("tool 应为 shell，实际 %v", detail)
	}
	if s, _ := detail["cmd"].(string); !strings.Contains(s, "ls -1") {
		t.Fatalf("cmd 应含命令，实际 %q", s)
	}
	// 正常完成（exitCode 0）不发事件；失败才发 progress。
	completed := `{"method":"item/completed","params":{"item":{"type":"commandExecution","id":"call_H7Bi1lB0n22XvmNW2gDwfpuG","status":"completed","exitCode":0}}}`
	if r := ParseCodexLine(completed, ctx); len(r.Events) != 0 {
		t.Fatalf("成功完成不应发事件，实际 %+v", r.Events)
	}
	failed := `{"method":"item/completed","params":{"item":{"type":"commandExecution","id":"call_X","status":"failed","exitCode":2,"durationMs":120}}}`
	r = ParseCodexLine(failed, ctx)
	if len(r.Events) != 1 {
		t.Fatalf("失败完成应产生 progress，实际 %+v", r.Events)
	}
	detail, _ = r.Events[0].Payload["detail"].(map[string]any)
	if detail["exit_code"] != float64(2) {
		t.Fatalf("应带 exit_code，实际 %v", detail)
	}
}

func TestParseCodexMCPToolCall(t *testing.T) {
	ctx := codexCtxWithThread()
	line := `{"method":"item/started","params":{"item":{"type":"mcpToolCall","id":"m1","server":"exa","tool":"search","arguments":{"query":"sunshine"}}}}`
	r := ParseCodexLine(line, ctx)
	if len(r.Events) != 1 {
		t.Fatalf("mcpToolCall 应产生 progress，实际 %+v", r.Events)
	}
	detail, _ := r.Events[0].Payload["detail"].(map[string]any)
	if detail["kind"] != "mcp" || detail["server"] != "exa" || detail["tool_name"] != "search" {
		t.Fatalf("MCP 字段不对：%v", detail)
	}
}

func TestParseCodexCollabBlocked(t *testing.T) {
	ctx := codexCtxWithThread()
	line := `{"method":"item/started","params":{"item":{"type":"collabAgentToolCall","id":"c1","receiverThreadIds":["t1"]}}}`
	r := ParseCodexLine(line, ctx)
	if len(r.Events) != 1 || r.Events[0].Kind != store.KindError {
		t.Fatalf("子代理调用应产生 error 事件，实际 %+v", r.Events)
	}
	if s, _ := r.Events[0].Payload["recommendation"].(string); !strings.Contains(s, "multi_agent=false") {
		t.Fatalf("应提示关闭 multi_agent，实际 %q", s)
	}
}

func TestParseCodexAgentMessageFlow(t *testing.T) {
	ctx := codexCtxWithThread()
	// commentary 先到：映射为 progress，不算最终答案。
	commentary := `{"method":"item/completed","params":{"item":{"type":"agentMessage","id":"msg_c","text":"按你的要求执行 ls。","phase":"commentary"}}}`
	r := ParseCodexLine(commentary, ctx)
	if len(r.Events) != 1 || r.Events[0].Kind != store.KindProgress {
		t.Fatalf("commentary 应映射为 progress，实际 %+v", r.Events)
	}
	detail, _ := r.Events[0].Payload["detail"].(map[string]any)
	if detail["kind"] != "commentary" {
		t.Fatalf("detail.kind 应为 commentary，实际 %v", detail)
	}
	if ctx.FinalMessageSeen {
		t.Fatal("commentary 不应标记最终答案已见")
	}
	// turn/completed 先于最终答案到达：先挂起，不发 done。
	r = ParseCodexLine(`{"method":"turn/completed","params":{"turn":{"id":"t1","status":"completed"}}}`, ctx)
	if len(r.Events) != 0 || !ctx.PendingDone {
		t.Fatalf("turn/completed 先到时应挂起 done，实际 %+v / pendingDone=%v", r.Events, ctx.PendingDone)
	}
	// 最终答案到达：message + 补发的 done。
	final := `{"method":"item/completed","params":{"item":{"type":"agentMessage","id":"msg_f","text":"ls 列出的当前目录中有 4 个可见条目。","phase":"final_answer"}}}`
	r = ParseCodexLine(final, ctx)
	if len(r.Events) != 2 || r.Events[0].Kind != store.KindMessage || r.Events[1].Kind != store.KindDone {
		t.Fatalf("最终答案应产生 message + 补发的 done，实际 %+v", r.Events)
	}
	if r.Events[0].Payload["text"] == "" {
		t.Fatal("message 应带文本")
	}
}

func TestParseCodexDoneImmediatelyAfterFinal(t *testing.T) {
	ctx := codexCtxWithThread()
	final := `{"method":"item/completed","params":{"item":{"type":"agentMessage","id":"msg_f","text":"完成。","phase":"final_answer"}}}`
	ParseCodexLine(final, ctx)
	r := ParseCodexLine(`{"method":"turn/completed","params":{"turn":{"id":"t1","status":"completed"}}}`, ctx)
	if len(r.Events) != 1 || r.Events[0].Kind != store.KindDone {
		t.Fatalf("最终答案已见时 turn/completed 应立即发 done，实际 %+v", r.Events)
	}
}

func TestParseCodexTurnFailedOnce(t *testing.T) {
	ctx := codexCtxWithThread()
	failed := `{"method":"turn/completed","params":{"turn":{"id":"t1","status":"failed","error":{"message":"模型调用失败"}}}}`
	r := ParseCodexLine(failed, ctx)
	if len(r.Events) != 1 || r.Events[0].Kind != store.KindError {
		t.Fatalf("失败回合应产生 error，实际 %+v", r.Events)
	}
	if r.Events[0].Payload["message"] != "模型调用失败" {
		t.Fatalf("error 消息不对：%v", r.Events[0].Payload["message"])
	}
	// 同一失败再报一次：不再重复发 error。
	r = ParseCodexLine(failed, ctx)
	if len(r.Events) != 0 {
		t.Fatalf("重复失败不应再发 error，实际 %+v", r.Events)
	}
	// 下一回合重置：先发一条成功 turn/completed 再失败应重新报错。
	ParseCodexLine(`{"method":"turn/completed","params":{"turn":{"id":"t2","status":"completed"}}}`, ctx)
	// 注意：上一回合的 terminalErrorSeen 仍为真时成功 turn 不重置；
	// 只有新 turn/start（EncodeCodexUserMessage）才重置。本测试验证
	// EncodeCodexUserMessage 的重置行为。
	if _, err := EncodeCodexUserMessage(ctx, "继续"); err != nil {
		t.Fatalf("编码用户消息失败：%v", err)
	}
	r = ParseCodexLine(failed, ctx)
	if len(r.Events) != 1 || r.Events[0].Kind != store.KindError {
		t.Fatalf("新回合失败应重新发 error，实际 %+v", r.Events)
	}
}

func TestParseCodexDeltaClassification(t *testing.T) {
	ctx := codexCtxWithThread()
	// 先让 final_answer 的 item 进入表，再发 delta。
	ParseCodexLine(`{"method":"item/started","params":{"item":{"type":"agentMessage","id":"msg_f","text":"","phase":"final_answer"}}}`, ctx)
	r := ParseCodexLine(`{"method":"item/agentMessage/delta","params":{"itemId":"msg_f","delta":"你好"}}`, ctx)
	if len(r.Events) != 1 || r.Events[0].Kind != store.KindProgress {
		t.Fatalf("delta 应产生 progress，实际 %+v", r.Events)
	}
	detail, _ := r.Events[0].Payload["detail"].(map[string]any)
	if detail["kind"] != "output" || detail["text_delta"] != "你好" || detail["stream_id"] != "msg_f" {
		t.Fatalf("delta 字段不对：%v", detail)
	}
	// reasoning item 的 delta 归类为 reasoning。
	ParseCodexLine(`{"method":"item/started","params":{"item":{"type":"reasoning","id":"rs_1"}}}`, ctx)
	r = ParseCodexLine(`{"method":"item/agentMessage/delta","params":{"itemId":"rs_1","delta":"思考中"}}`, ctx)
	detail, _ = r.Events[0].Payload["detail"].(map[string]any)
	if detail["kind"] != "reasoning" {
		t.Fatalf("reasoning delta 应归类为 reasoning，实际 %v", detail)
	}
}

func TestParseCodexElicitationAutoAccept(t *testing.T) {
	ctx := codexCtxWithThread()
	line := `{"method":"mcpServer/elicitation/request","id":7,"params":{"serverName":"exa","_meta":{"tool_description":"搜索"}}}`
	r := ParseCodexLine(line, ctx)
	if len(r.Events) != 1 || r.Events[0].Kind != store.KindProgress {
		t.Fatalf("MCP 询问应产生 progress，实际 %+v", r.Events)
	}
	if len(r.Side.Reply) != 1 {
		t.Fatalf("MCP 询问应自动回复，实际 %+v", r.Side)
	}
	var reply map[string]any
	if err := json.Unmarshal([]byte(r.Side.Reply[0]), &reply); err != nil {
		t.Fatalf("回复应为合法 JSON：%v", err)
	}
	result, _ := reply["result"].(map[string]any)
	if result["action"] != "accept" {
		t.Fatalf("应自动放行（accept），实际 %v", result)
	}
	// 未知服务端请求：拒绝并记 error。
	r = ParseCodexLine(`{"method":"something/unknown","id":8,"params":{}}`, ctx)
	if len(r.Events) != 1 || r.Events[0].Kind != store.KindError {
		t.Fatalf("未知服务端请求应记 error，实际 %+v", r.Events)
	}
	var reply2 map[string]any
	_ = json.Unmarshal([]byte(r.Side.Reply[0]), &reply2)
	result2, _ := reply2["result"].(map[string]any)
	if result2["action"] != "decline" {
		t.Fatalf("未知请求应拒绝（decline），实际 %v", result2)
	}
}

func TestParseCodexWarningAndErrorNotification(t *testing.T) {
	ctx := codexCtxWithThread()
	r := ParseCodexLine(`{"method":"warning","params":{"message":"技能上下文预算超了"}}`, ctx)
	if len(r.Events) != 1 || r.Events[0].Kind != store.KindProgress {
		t.Fatalf("warning 应产生 progress，实际 %+v", r.Events)
	}
	// willRetry 的 error 降级为 warning。
	r = ParseCodexLine(`{"method":"error","params":{"error":{"message":"闪断"},"willRetry":true}}`, ctx)
	if len(r.Events) != 1 || r.Events[0].Kind != store.KindProgress {
		t.Fatalf("可重试 error 应降级为 progress，实际 %+v", r.Events)
	}
	// 不可重试的 error 发 error 事件；turn/aborted 同理。
	r = ParseCodexLine(`{"method":"error","params":{"error":{"message":"彻底失败"}}}`, ctx)
	if len(r.Events) != 1 || r.Events[0].Kind != store.KindError {
		t.Fatalf("不可重试 error 应发 error，实际 %+v", r.Events)
	}
	r = ParseCodexLine(`{"method":"turn/aborted","params":{}}`, ctx)
	if len(r.Events) != 1 || r.Events[0].Kind != store.KindError {
		t.Fatalf("turn/aborted 应发 error，实际 %+v", r.Events)
	}
	// 坏行发 error 事件。
	r = ParseCodexLine(`{不是 json`, ctx)
	if len(r.Events) != 1 || r.Events[0].Kind != store.KindError {
		t.Fatalf("坏行应产生 error 事件，实际 %+v", r.Events)
	}
}

func TestParseCodexSandbox(t *testing.T) {
	if m, err := ParseCodexSandboxMode(""); err != nil || m != "" {
		t.Fatalf("空值应返回空（调用方回落默认），实际 %q / %v", m, err)
	}
	for _, m := range []string{"read-only", "workspace-write", "danger-full-access"} {
		if _, err := ParseCodexSandboxMode(m); err != nil {
			t.Fatalf("应接受 %q：%v", m, err)
		}
	}
	if _, err := ParseCodexSandboxMode("nope"); err == nil {
		t.Fatal("未知沙箱模式应报错")
	}
	params := BuildCodexThreadStartParams("/tmp/proj", "你是助手", "")
	if params["sandbox"] != "workspace-write" {
		t.Fatalf("默认沙箱应为 workspace-write，实际 %v", params["sandbox"])
	}
	if params["approvalPolicy"] != "never" {
		t.Fatalf("审批策略应为 never，实际 %v", params["approvalPolicy"])
	}
	cfg, _ := params["config"].(map[string]any)
	features, _ := cfg["features"].(map[string]any)
	if features["multi_agent"] != false {
		t.Fatalf("应关闭原生多 agent，实际 %v", features)
	}
	if params["developerInstructions"] != "你是助手" {
		t.Fatalf("系统提示词应进 developerInstructions，实际 %v", params["developerInstructions"])
	}
	params = BuildCodexThreadStartParams("/tmp/proj", "", "read-only")
	if params["sandbox"] != "read-only" {
		t.Fatalf("应按传入值覆盖沙箱，实际 %v", params["sandbox"])
	}
	if _, ok := params["developerInstructions"]; ok {
		t.Fatal("空系统提示词不应写 developerInstructions")
	}
}

func TestBuildCodexArgs(t *testing.T) {
	args := BuildCodexArgs("")
	if len(args) != 1 || args[0] != "app-server" {
		t.Fatalf("无模型时应只有 app-server，实际 %v", args)
	}
	args = BuildCodexArgs("gpt-5.3-codex")
	if len(args) != 3 || args[1] != "-c" || !strings.Contains(args[2], "gpt-5.3-codex") {
		t.Fatalf("模型参数不对：%v", args)
	}
}

func TestEncodeCodexUserMessageNeedsThread(t *testing.T) {
	ctx := NewCodexCtx()
	if _, err := EncodeCodexUserMessage(ctx, "你好"); err == nil {
		t.Fatal("无 threadId 时发送消息应报错")
	}
	ctx.ThreadID = codexThreadID
	line, err := EncodeCodexUserMessage(ctx, "你好")
	if err != nil {
		t.Fatalf("有 threadId 时应能编码：%v", err)
	}
	var obj map[string]any
	if err := json.Unmarshal([]byte(line), &obj); err != nil {
		t.Fatalf("应为合法 JSON：%v", err)
	}
	if obj["method"] != "turn/start" {
		t.Fatalf("应为 turn/start 请求，实际 %v", obj["method"])
	}
	params, _ := obj["params"].(map[string]any)
	if params["threadId"] != codexThreadID {
		t.Fatalf("应带 threadId，实际 %v", params)
	}
	// 中断带标记前缀。
	interrupt, err := EncodeCodexInterruptMessage(ctx, "改做 B")
	if err != nil {
		t.Fatalf("编码中断失败：%v", err)
	}
	if !strings.Contains(interrupt, "GRID INTERRUPT") || !strings.Contains(interrupt, "改做 B") {
		t.Fatalf("中断应带标记与替换指令：%s", interrupt)
	}
}

func TestCodexAdapterReady(t *testing.T) {
	a := codexAdapter{}
	ctx := a.CreateCtx()
	if a.IsReady(ctx) {
		t.Fatal("握手前不应就绪")
	}
	cc := ctx.(*CodexCtx)
	cc.ThreadID = codexThreadID
	if !a.IsReady(ctx) {
		t.Fatal("拿到 threadId 后应就绪")
	}
	if a.Provider() != ProviderCodex {
		t.Fatalf("供应商应为 codex，实际 %v", a.Provider())
	}
}
