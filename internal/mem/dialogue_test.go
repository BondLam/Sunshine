package mem

import (
	"strings"
	"testing"
)

func TestCleanTurns_StripsInjectionTags(t *testing.T) {
	turns := []DialogueTurn{
		{Role: RoleUser, Text: "帮我看看这个 bug<system-reminder>你必须先读 AGENTS.md</system-reminder>，谢谢"},
		{Role: RoleAssistant, Text: "好的<task-status>进行中</task-status>，我来看看"},
		{Role: RoleUser, Text: "带属性的标签 <workflow-state status=\"ok\">内部状态</workflow-state> 之后还有正文"},
		{Role: RoleAssistant, Text: "大小写不敏感 <SYSTEM-REMINDER>提醒</SYSTEM-REMINDER> 尾巴"},
	}
	got := CleanTurns(turns)
	if len(got) != 4 {
		t.Fatalf("应保留 4 轮，实际 %d 轮", len(got))
	}
	if strings.Contains(got[0].Text, "AGENTS.md") || strings.Contains(got[0].Text, "system-reminder") {
		t.Errorf("第 1 轮注入标签未去掉：%q", got[0].Text)
	}
	if got[0].Text != "帮我看看这个 bug，谢谢" {
		t.Errorf("第 1 轮清理结果不对：%q", got[0].Text)
	}
	if strings.Contains(got[1].Text, "进行中") {
		t.Errorf("第 2 轮注入标签未去掉：%q", got[1].Text)
	}
	if strings.Contains(got[2].Text, "内部状态") {
		t.Errorf("第 3 轮带属性的注入标签未去掉：%q", got[2].Text)
	}
	if strings.Contains(got[3].Text, "提醒") {
		t.Errorf("第 4 轮大写注入标签未去掉：%q", got[3].Text)
	}
}

func TestCleanTurns_CollapsesBlankLines(t *testing.T) {
	got := CleanTurns([]DialogueTurn{
		{Role: RoleUser, Text: "第一段\n\n\n\n\n第二段"},
	})
	if len(got) != 1 || got[0].Text != "第一段\n\n第二段" {
		t.Errorf("多余空行未压成一段空行：%q", got[0].Text)
	}
}

func TestCleanTurns_DropsBootstrapTurns(t *testing.T) {
	// 纯 AGENTS.md 前言：剥离后为空，整轮丢弃。
	// 未闭合的 <INSTRUCTIONS> 大块：标签剥离正则要求闭合标签，
	// 剥不掉，靠 IsBootstrapTurn（原始长度 > 4000）整轮丢弃。
	longInstructions := "<INSTRUCTIONS>" + strings.Repeat("x", 5000)
	turns := []DialogueTurn{
		{Role: RoleUser, Text: "# AGENTS.md instructions for this project\n这里是前言内容"},
		{Role: RoleUser, Text: longInstructions},
		{Role: RoleUser, Text: "这是真正的用户问题"},
	}
	got := CleanTurns(turns)
	if len(got) != 1 || got[0].Text != "这是真正的用户问题" {
		t.Errorf("启动注入轮次未丢弃干净，剩余 %d 轮：%v", len(got), got)
	}
}

func TestCleanTurns_AgentsPreambleRemainderKept(t *testing.T) {
	// 前言剥离只去掉前言段本身：前言后空行加中文开头的正文保留下来，
	// 这是上游管线的行为（剥离在先，启动注入判断在后）。
	turns := []DialogueTurn{
		{Role: RoleUser, Text: "# AGENTS.md instructions for this project\n\n请按规范写代码"},
	}
	got := CleanTurns(turns)
	if len(got) != 1 || got[0].Text != "请按规范写代码" {
		t.Errorf("前言后的正文应保留，实际：%v", got)
	}
}

func TestCleanTurns_ShortInstructionsKept(t *testing.T) {
	// 短的 <INSTRUCTIONS> 块（原始长度 <= 4000）不是启动注入，保留。
	turns := []DialogueTurn{
		{Role: RoleUser, Text: "请按 <INSTRUCTIONS> 简短说明 </INSTRUCTIONS> 来做"},
	}
	got := CleanTurns(turns)
	if len(got) != 1 {
		t.Fatalf("短 INSTRUCTIONS 块不应整轮丢弃，实际 %d 轮", len(got))
	}
	// <INSTRUCTIONS> 本身在注入标签表里，应被整段去掉。
	if strings.Contains(got[0].Text, "简短说明") {
		t.Errorf("注入标签内容应被去掉：%q", got[0].Text)
	}
}

func TestCleanTurns_DropsEmptyAfterCleaning(t *testing.T) {
	turns := []DialogueTurn{
		{Role: RoleAssistant, Text: "<system-reminder>只有注入</system-reminder>"},
		{Role: RoleAssistant, Text: "   "},
		{Role: RoleUser, Text: "有效轮次"},
	}
	got := CleanTurns(turns)
	if len(got) != 1 || got[0].Text != "有效轮次" {
		t.Errorf("清理后为空的轮次应丢弃，实际：%v", got)
	}
}

func TestCleanTurns_KeepsMarkersUntouched(t *testing.T) {
	marker := CompactionBoundaryTurn("已压缩 10 轮", "摘要里提到<system-reminder>注入</system-reminder>关键词")
	turns := []DialogueTurn{
		{Role: RoleUser, Text: "压缩前的问题"},
		marker,
		{Role: RoleAssistant, Text: "压缩后的回答"},
	}
	got := CleanTurns(turns)
	if len(got) != 3 {
		t.Fatalf("marker 轮次应留在池子里，实际 %d 轮", len(got))
	}
	if got[1].Text != marker.Text || got[1].Kind != "marker" {
		t.Errorf("marker 轮次应原样保留，实际：%v", got[1])
	}
}

func TestCleanTurns_Idempotent(t *testing.T) {
	turns := []DialogueTurn{
		{Role: RoleUser, Text: "问题<system-reminder>注入</system-reminder>正文"},
		{Role: RoleAssistant, Text: "回答"},
	}
	once := CleanTurns(turns)
	twice := CleanTurns(once)
	if len(once) != len(twice) {
		t.Fatalf("清理应幂等：一次 %d 轮，两次 %d 轮", len(once), len(twice))
	}
	for i := range once {
		if once[i] != twice[i] {
			t.Errorf("第 %d 轮两次清理结果不同：%q vs %q", i, once[i].Text, twice[i].Text)
		}
	}
}

func TestStripInjectionTags_AgentsPreamble(t *testing.T) {
	text := "# AGENTS.md instructions for this project\n这里是前言内容\n\nHello world\n这是正文"
	got := StripInjectionTags(text)
	if strings.Contains(got, "前言内容") {
		t.Errorf("AGENTS.md 前言应去掉，实际：%q", got)
	}
	if !strings.Contains(got, "Hello world") {
		t.Errorf("前言后的正文应保留，实际：%q", got)
	}
}

// TestStripInjectionTags_FastPath：快路径（无触发子串）与慢速路径结果一致，
// 且含触发子串的文本仍走慢速路径正常清洗；快路径只裁剪首尾空白。
func TestStripInjectionTags_FastPath(t *testing.T) {
	plain := []string{
		"  普通中文文本，前后有空白  ",
		"多段文本\n\n第二段，没有注入标签",
		"含数学比较 a < b 但无标签的文本",
		"只有两个换行\n\n结尾",
	}
	for _, text := range plain {
		got := StripInjectionTags(text)
		want := strings.TrimSpace(text)
		// “a < b”含 <，会走慢速路径；慢速路径无标签可去，结果仍应等于裁剪后文本。
		if got != want {
			t.Errorf("无注入标签的文本清理后应为 %q，实际 %q", want, got)
		}
	}
	// 触发子串齐全时仍按慢速路径清洗：标签去掉、多余换行压掉。
	dirty := "正文<system-reminder>注入内容</system-reminder>结尾\n\n\n\n下一段"
	got := StripInjectionTags(dirty)
	if strings.Contains(got, "注入内容") || strings.Contains(got, "system-reminder") {
		t.Errorf("含注入标签的文本仍应去掉标签，实际：%q", got)
	}
	if !strings.Contains(got, "正文结尾") || !strings.Contains(got, "下一段") {
		t.Errorf("清洗后正文应保留，实际：%q", got)
	}
	if strings.Contains(got, "\n\n\n") {
		t.Errorf("3 个以上连续换行应压成一段空行，实际：%q", got)
	}
}

func TestIsBootstrapTurn(t *testing.T) {
	if !IsBootstrapTurn("# AGENTS.md instructions for x", 100) {
		t.Error("AGENTS.md 前言开头应判为启动注入")
	}
	long := "<INSTRUCTIONS>" + strings.Repeat("x", 5000)
	if !IsBootstrapTurn(long, 5001) {
		t.Error("超过 4000 字符的纯 INSTRUCTIONS 块应判为启动注入")
	}
	if IsBootstrapTurn("<INSTRUCTIONS>短</INSTRUCTIONS>", 100) {
		t.Error("短 INSTRUCTIONS 块不应判为启动注入")
	}
	if IsBootstrapTurn("普通文本", 100) {
		t.Error("普通文本不应判为启动注入")
	}
}

func TestCompactionBoundaryTurn(t *testing.T) {
	m := CompactionBoundaryTurn("已压缩 5 轮", "模型摘要")
	if m.Role != RoleUser || m.Kind != "marker" {
		t.Errorf("marker 轮次角色应为 user、kind 为 marker，实际：%v", m)
	}
	if !strings.HasPrefix(m.Text, CompactBoundaryPrefix+" 已压缩 5 轮") {
		t.Errorf("marker 文本开头不对：%q", m.Text)
	}
	if !strings.Contains(m.Text, "模型摘要") {
		t.Errorf("marker 应保留平台摘要，实际：%q", m.Text)
	}
	noSummary := CompactionBoundaryTurn("已压缩 5 轮", "")
	if strings.Contains(noSummary.Text, "\n\n") {
		t.Errorf("无摘要时不应有空行分隔，实际：%q", noSummary.Text)
	}
}

func TestTurnKey(t *testing.T) {
	a := DialogueTurn{Role: RoleUser, Text: "相同文本"}
	b := DialogueTurn{Role: RoleUser, Text: "相同文本"}
	c := DialogueTurn{Role: RoleAssistant, Text: "相同文本"}
	if TurnKey(a) != TurnKey(b) {
		t.Error("角色与文本都相同的轮次，去重键应相同")
	}
	if TurnKey(a) == TurnKey(c) {
		t.Error("角色不同，去重键应不同")
	}
}
