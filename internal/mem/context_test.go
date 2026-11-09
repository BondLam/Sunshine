package mem

import (
	"strings"
	"testing"
)

func sampleTurns() []DialogueTurn {
	return []DialogueTurn{
		{Role: RoleUser, Text: "第一轮：讨论缓存"},
		{Role: RoleAssistant, Text: "第二轮：缓存方案"},
		{Role: RoleUser, Text: "第三轮：换个话题"},
		{Role: RoleAssistant, Text: "第四轮：继续聊缓存"},
		{Role: RoleUser, Text: "第五轮：还是缓存"},
		{Role: RoleAssistant, Text: "第六轮：收尾"},
	}
}

func TestSelectContextTurns_GrepRanking(t *testing.T) {
	// grep=缓存：命中第 1、2、4、5 轮（用户轮次优先、再按命中数）；
	// turns=2 取第 1、5 轮（用户轮次），around=1 各扩一轮 → 展示 1、2、4、5、6 轮。
	sel := SelectContextTurns(sampleTurns(), MemFilter{Grep: "缓存", Turns: 2, Around: 1, MaxChars: 6000})
	if sel.TotalHitTurns != 4 {
		t.Errorf("命中轮次总数应为 4，实际 %d", sel.TotalHitTurns)
	}
	want := []int{0, 1, 3, 4, 5}
	if len(sel.Turns) != len(want) {
		t.Fatalf("展示轮次应为 %v，实际 %v", want, idxs(sel.Turns))
	}
	for i, idx := range want {
		if sel.Turns[i].Idx != idx {
			t.Errorf("第 %d 个展示轮次应为 %d，实际 %d", i, idx, sel.Turns[i].Idx)
		}
	}
	// 命中标记只打在目标轮次上，邻近轮次不打。
	for _, ct := range sel.Turns {
		shouldHit := ct.Idx == 0 || ct.Idx == 4
		if ct.IsHit != shouldHit {
			t.Errorf("第 %d 轮 IsHit 应为 %v，实际 %v", ct.Idx, shouldHit, ct.IsHit)
		}
	}
}

func idxs(turns []ContextTurn) []int {
	out := make([]int, len(turns))
	for i, t := range turns {
		out[i] = t.Idx
	}
	return out
}

func TestSelectContextTurns_NoGrepFirstTurns(t *testing.T) {
	// 无 grep：取开头的 turns 轮及其邻近轮次。
	sel := SelectContextTurns(sampleTurns(), MemFilter{Turns: 2, Around: 1, MaxChars: 6000})
	want := []int{0, 1, 2}
	if len(sel.Turns) != len(want) {
		t.Fatalf("无 grep 时应展示开头 2 轮加邻近共 %v，实际 %v", want, idxs(sel.Turns))
	}
	for i, idx := range want {
		if sel.Turns[i].Idx != idx {
			t.Errorf("第 %d 个展示轮次应为 %d，实际 %d", i, idx, sel.Turns[i].Idx)
		}
	}
	if sel.TotalHitTurns != 0 {
		t.Errorf("无 grep 时命中轮次总数应为 0，实际 %d", sel.TotalHitTurns)
	}
}

func TestSelectContextTurns_Defaults(t *testing.T) {
	// 零值 filter：turns=3、around=1、max-chars=6000。
	sel := SelectContextTurns(sampleTurns(), MemFilter{})
	if len(sel.Turns) != 4 {
		t.Fatalf("默认应展示开头 3 轮加邻近共 4 轮，实际 %d 轮：%v", len(sel.Turns), idxs(sel.Turns))
	}
	if sel.TotalTurns != 6 {
		t.Errorf("总轮次应为 6，实际 %d", sel.TotalTurns)
	}
}

func TestSelectContextTurns_MaxCharsTruncation(t *testing.T) {
	longText := strings.Repeat("长", 4000)
	turns := []DialogueTurn{
		{Role: RoleUser, Text: longText},
		{Role: RoleAssistant, Text: "短回答"},
	}
	// maxChars=6000：单轮上限 3000 字符，长轮应从头部截断并标注剩余字符数。
	sel := SelectContextTurns(turns, MemFilter{Turns: 3, Around: 1, MaxChars: 6000})
	if len(sel.Turns) != 2 {
		t.Fatalf("应展示 2 轮，实际 %d 轮", len(sel.Turns))
	}
	first := sel.Turns[0].Text
	if !strings.Contains(first, "另有 1000 字符") {
		t.Errorf("超长单轮应标注剩余字符数，实际：%q", first[len(first)-30:])
	}
	if len([]rune(first)) > 3000+20 {
		t.Errorf("截断后的单轮不应远超预算一半，实际 %d 字符", len([]rune(first)))
	}

	// 预算收紧：两轮都超长，maxChars=100 时，第一轮截断后占满大半预算，
	// 第二轮再放就超预算，不再输出。
	twoLong := []DialogueTurn{
		{Role: RoleUser, Text: longText},
		{Role: RoleAssistant, Text: longText},
	}
	sel = SelectContextTurns(twoLong, MemFilter{Turns: 3, Around: 1, MaxChars: 100})
	if len(sel.Turns) != 1 {
		t.Errorf("预算 100 字符时应只展示 1 轮，实际 %d 轮", len(sel.Turns))
	}
	if sel.BudgetUsed > 100 {
		t.Errorf("已用预算不应超过 maxChars，实际 %d/100", sel.BudgetUsed)
	}
}

func TestSessionContext_Rendering(t *testing.T) {
	// turns=1 取命中最高的 1 轮（第 1 轮，用户优先），around=1 再扩一轮。
	out := SessionContext(sampleTurns(), MemFilter{Grep: "缓存", Turns: 1, Around: 1, MaxChars: 6000})
	if !strings.Contains(out, "## 第 0 轮（用户） ← 命中") {
		t.Errorf("渲染应含命中轮次标题，实际：\n%s", out)
	}
	if !strings.Contains(out, "第一轮：讨论缓存") {
		t.Errorf("渲染应含命中轮次文本，实际：\n%s", out)
	}
	if !strings.Contains(out, "第二轮：缓存方案") {
		t.Errorf("around=1 时应含邻近轮次，实际：\n%s", out)
	}
	if strings.Contains(out, "第三轮") {
		t.Errorf("只扩一轮时不应含第三轮，实际：\n%s", out)
	}
	if !strings.Contains(out, "---") {
		t.Errorf("轮次之间应有 --- 分隔，实际：\n%s", out)
	}
	if got := SessionContext(nil, MemFilter{}); got != "" {
		t.Errorf("空对话应渲染为空字符串，实际：%q", got)
	}
}

func TestSessionContext_IncludeChildren(t *testing.T) {
	parent := MemSessionInfo{ID: "p1", Platform: MemSourceOpencode}
	child := MemSessionInfo{ID: "c1", Platform: MemSourceOpencode, ParentID: "p1"}
	parentTurns := []DialogueTurn{{Role: RoleUser, Text: "父会话：讨论缓存"}}
	childTurns := []DialogueTurn{{Role: RoleAssistant, Text: "子会话：缓存细节"}}
	filter := MemFilter{
		Grep:            "缓存",
		Turns:           3,
		Around:          1,
		MaxChars:        6000,
		IncludeChildren: true,
		SessionID:       "p1",
		Sessions:        []MemSessionInfo{parent, child},
		LoadTurns: func(s MemSessionInfo) ([]DialogueTurn, error) {
			if s.ID == "c1" {
				return childTurns, nil
			}
			return nil, nil
		},
	}
	out := SessionContext(parentTurns, filter)
	if !strings.Contains(out, "父会话：讨论缓存") || !strings.Contains(out, "子会话：缓存细节") {
		t.Errorf("include-children 时子会话轮次应并入上下文，实际：\n%s", out)
	}

	// 不开 include-children：子会话不出现。
	filter.IncludeChildren = false
	out = SessionContext(parentTurns, filter)
	if strings.Contains(out, "子会话") {
		t.Errorf("不开 include-children 时子会话不应出现，实际：\n%s", out)
	}
}

func TestBuildChildIndex_Transitive(t *testing.T) {
	sessions := []MemSessionInfo{
		{ID: "p", Platform: MemSourceOpencode},
		{ID: "c1", Platform: MemSourceOpencode, ParentID: "p"},
		{ID: "c2", Platform: MemSourceOpencode, ParentID: "p"},
		{ID: "g1", Platform: MemSourceOpencode, ParentID: "c1"},
		{ID: "other", Platform: MemSourceOpencode},
	}
	index := BuildChildIndex(sessions)
	kids := index["p"]
	if len(kids) != 3 {
		t.Fatalf("父会话应有 3 个子孙（c1、c2、g1），实际 %d 个：%v", len(kids), kids)
	}
	ids := map[string]bool{}
	for _, k := range kids {
		ids[k.ID] = true
	}
	for _, want := range []string{"c1", "c2", "g1"} {
		if !ids[want] {
			t.Errorf("子孙表里缺 %s", want)
		}
	}
	if _, ok := index["other"]; ok {
		t.Error("无 parent_id 的会话不应出现在子会话索引里")
	}
}

func TestExtractDialogue_FullAndMarkers(t *testing.T) {
	turns := []DialogueTurn{
		{Role: RoleUser, Text: "第一轮"},
		CompactionBoundaryTurn("已压缩 2 轮", "摘要"),
		{Role: RoleAssistant, Text: "第三轮"},
	}
	got := ExtractDialogue(turns, MemFilter{})
	if len(got) != 3 {
		t.Fatalf("抽取应返回全部 3 轮（含 marker），实际 %d 轮", len(got))
	}
	if got[1].Kind != "marker" {
		t.Errorf("marker 轮次应保留在抽取结果里，实际 kind=%q", got[1].Kind)
	}
}

func TestExtractDialogue_GrepFilter(t *testing.T) {
	turns := []DialogueTurn{
		{Role: RoleUser, Text: "讨论缓存策略"},
		{Role: RoleAssistant, Text: "讨论别的"},
		{Role: RoleUser, Text: "缓存与检索都要提"},
	}
	got := ExtractDialogue(turns, MemFilter{Grep: "缓存 检索"})
	if len(got) != 1 || got[0].Text != "缓存与检索都要提" {
		t.Errorf("grep 多 token AND 应只留同时含两词的轮次，实际：%v", got)
	}
}

func taskPhaseTurns() []DialogueTurn {
	return []DialogueTurn{
		{Role: RoleUser, Text: "先想想做什么"},
		{Role: RoleAssistant, Text: "sunshine task create --slug demo"},
		{Role: RoleUser, Text: "方案定了，开始动手"},
		{Role: RoleAssistant, Text: "sunshine task start .sunshine/tasks/11-09-demo/"},
		{Role: RoleUser, Text: "写代码中"},
		{Role: RoleAssistant, Text: "继续实现功能"},
	}
}

// taskPhaseEvents 是 taskPhaseTurns 对应的事件通道：
// create 在第 2 轮（下标 1）、start 在第 4 轮（下标 3），
// 与 CLI 路径适配器 Collect 从工具调用里抽出的事件一致。
// 轮次文本里的命令只是普通文本，不再作为事件来源。
func taskPhaseEvents() []MemEvent {
	return []MemEvent{
		{Action: "create", TurnIndex: 1, Slug: "demo"},
		{Action: "start", TurnIndex: 3, TaskDir: ".sunshine/tasks/11-09-demo/"},
	}
}

func TestExtractDialogue_PhaseBrainstorm(t *testing.T) {
	// 事件通道有边界时：头脑风暴窗口 = [create 所在轮次 1, start 所在轮次 3)，
	// 即第 2、3 轮。
	got := ExtractDialogue(taskPhaseTurns(), MemFilter{Phase: PhaseBrainstorm, Events: taskPhaseEvents()})
	if len(got) != 2 {
		t.Fatalf("brainstorm 应返回窗口内 2 轮，实际 %d 轮：%v", len(got), got)
	}
	if got[0].Text != "sunshine task create --slug demo" {
		t.Errorf("窗口第一轮不对：%q", got[0].Text)
	}
}

func TestExtractDialogue_PhaseImplement(t *testing.T) {
	// 事件通道有边界时：implement = 窗口外的轮次：第 1、4、5、6 轮。
	got := ExtractDialogue(taskPhaseTurns(), MemFilter{Phase: PhaseImplement, Events: taskPhaseEvents()})
	if len(got) != 4 {
		t.Fatalf("implement 应返回窗口外 4 轮，实际 %d 轮：%v", len(got), got)
	}
	if !strings.Contains(got[0].Text, "先想想做什么") {
		t.Errorf("implement 第一轮应为窗口前的内容，实际：%q", got[0].Text)
	}
}

// TestExtractDialogue_TextCommandsIgnored 是统一后的关键回归：
// 轮次文本里就算写着任务命令，没有事件通道就不算边界——
// 真实会话的命令藏在工具调用块里，轮次文本中没有，
// 旧的文本扫描分支会把普通讨论误判为边界。
func TestExtractDialogue_TextCommandsIgnored(t *testing.T) {
	turns := taskPhaseTurns()
	if got := ExtractDialogue(turns, MemFilter{Phase: PhaseBrainstorm}); len(got) != len(turns) {
		t.Errorf("无事件时 brainstorm 应返回全文 %d 轮（文本命令不算边界），实际 %d 轮", len(turns), len(got))
	}
	if got := ExtractDialogue(turns, MemFilter{Phase: PhaseImplement}); len(got) != 0 {
		t.Errorf("无事件时 implement 应为空，实际 %d 轮", len(got))
	}
}

// TestExtractDialogue_MatchesSliceMemPhase 是两种入口一致性的直接断言：
// 直接调 ExtractDialogue（事件通道）与 CLI 路径的 SliceMemPhase
// 用同一批事件、同一平台，应切出完全相同的轮次。
func TestExtractDialogue_MatchesSliceMemPhase(t *testing.T) {
	turns := taskPhaseTurns()
	events := taskPhaseEvents()
	for _, phase := range []MemPhase{PhaseBrainstorm, PhaseImplement, PhaseAll} {
		direct := ExtractDialogue(turns, MemFilter{Phase: phase, Events: events})
		cleaned := ExtractDialogue(turns, MemFilter{Phase: PhaseAll})
		sliced, _, _ := SliceMemPhase(cleaned, events, phase, MemSourceClaude)
		if len(direct) != len(sliced) {
			t.Fatalf("phase=%s 时直接调应与 SliceMemPhase 一致（%d 轮），实际 %d 轮", phase, len(sliced), len(direct))
		}
		for i := range direct {
			if direct[i] != sliced[i] {
				t.Fatalf("phase=%s 时第 %d 轮不一致：直接调=%+v，SliceMemPhase=%+v", phase, i, direct[i], sliced[i])
			}
		}
	}
}

// TestExtractDialogue_OpencodeDowngrade：OpenCode 平台不支持阶段切片，
// 直接调与 SliceMemPhase 一致：brainstorm 降级返回全文。
func TestExtractDialogue_OpencodeDowngrade(t *testing.T) {
	turns := taskPhaseTurns()
	got := ExtractDialogue(turns, MemFilter{Phase: PhaseBrainstorm, Events: taskPhaseEvents(), PlatformKind: MemSourceOpencode})
	if len(got) != len(turns) {
		t.Errorf("OpenCode 上 brainstorm 应降级返回全文 %d 轮，实际 %d 轮", len(turns), len(got))
	}
}

func TestExtractDialogue_PhaseNoBoundary(t *testing.T) {
	turns := []DialogueTurn{
		{Role: RoleUser, Text: "没有任务命令的会话"},
		{Role: RoleAssistant, Text: "纯聊天"},
	}
	if got := ExtractDialogue(turns, MemFilter{Phase: PhaseBrainstorm}); len(got) != 2 {
		t.Errorf("无边界时 brainstorm 应返回全文，实际 %d 轮", len(got))
	}
	if got := ExtractDialogue(turns, MemFilter{Phase: PhaseImplement}); len(got) != 0 {
		t.Errorf("无边界时 implement 应为空，实际 %d 轮", len(got))
	}
}

func TestExtractDialogue_PhaseAndGrep(t *testing.T) {
	// 先按阶段切片（事件通道），再按 grep 过滤。
	got := ExtractDialogue(taskPhaseTurns(), MemFilter{Phase: PhaseImplement, Events: taskPhaseEvents(), Grep: "写代码"})
	if len(got) != 1 || !strings.Contains(got[0].Text, "写代码中") {
		t.Errorf("implement + grep 应只留窗口外含“写代码”的轮次，实际：%v", got)
	}
}

// TestExtractDialogue_InvalidPhase：非法 phase 按 all 处理（返回全文），
// 与统一前的行为一致；非法值的报错由 CLI 层负责。
func TestExtractDialogue_InvalidPhase(t *testing.T) {
	turns := taskPhaseTurns()
	got := ExtractDialogue(turns, MemFilter{Phase: MemPhase("foo"), Events: taskPhaseEvents()})
	if len(got) != len(turns) {
		t.Errorf("非法 phase 应按 all 返回全文 %d 轮，实际 %d 轮", len(turns), len(got))
	}
}
