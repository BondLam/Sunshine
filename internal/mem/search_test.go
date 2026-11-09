package mem

import (
	"errors"
	"strings"
	"testing"
)

func TestSearchInDialogue_MultiTokenAND(t *testing.T) {
	turns := []DialogueTurn{
		{Role: RoleUser, Text: "会话检索怎么做"},
		{Role: RoleAssistant, Text: "检索只谈关键词，不谈别的"},
		{Role: RoleUser, Text: "会话与检索都要提到才算命中"},
	}
	hit := SearchInDialogue(turns, "会话 检索", 3, 400)
	if hit.Count != 4 {
		t.Errorf("多 token AND：只有同时含“会话”“检索”的轮次算命中，期望 count=4，实际 %d", hit.Count)
	}
	if hit.UserCount != 4 || hit.AsstCount != 0 {
		t.Errorf("用户/助手命中拆分不对：u=%d a=%d", hit.UserCount, hit.AsstCount)
	}
	if hit.TotalTurns != 3 {
		t.Errorf("totalTurns 应为 3，实际 %d", hit.TotalTurns)
	}
	if len(hit.Excerpts) != 2 {
		t.Errorf("应有 2 条摘录（两轮命中），实际 %d 条", len(hit.Excerpts))
	}
}

func TestSearchInDialogue_SingleTokenMissing(t *testing.T) {
	turns := []DialogueTurn{
		{Role: RoleUser, Text: "只有会话，没有另一个词"},
	}
	hit := SearchInDialogue(turns, "会话 检索", 3, 400)
	if hit.Count != 0 || len(hit.Excerpts) != 0 {
		t.Errorf("缺一个 token 就不应命中，实际 count=%d，摘录 %d 条", hit.Count, len(hit.Excerpts))
	}
}

func TestSearchInDialogue_MarkerNotScored(t *testing.T) {
	turns := []DialogueTurn{
		{Role: RoleUser, Text: "压缩前讨论了缓存策略"},
		CompactionBoundaryTurn("已压缩 3 轮", "摘要：缓存策略很重要，缓存缓存缓存"),
		{Role: RoleAssistant, Text: "与此无关的回答"},
	}
	hit := SearchInDialogue(turns, "缓存", 3, 400)
	if hit.Count != 1 {
		t.Errorf("marker 轮次里的摘要不应计分，期望 count=1，实际 %d", hit.Count)
	}
	if hit.TotalTurns != 2 {
		t.Errorf("totalTurns 不应含 marker 轮次，期望 2，实际 %d", hit.TotalTurns)
	}
	for _, ex := range hit.Excerpts {
		if strings.Contains(ex.Snippet, "摘要") {
			t.Errorf("摘录不应来自 marker 轮次：%q", ex.Snippet)
		}
	}
}

func TestSearchInDialogue_ExcerptParagraphAligned(t *testing.T) {
	turns := []DialogueTurn{
		{Role: RoleUser, Text: "第一段没有命中词。\n\n第二段提到了关键词检索。\n\n第三段也没有。"},
	}
	hit := SearchInDialogue(turns, "关键词", 3, 400)
	if len(hit.Excerpts) != 1 {
		t.Fatalf("期望 1 条摘录，实际 %d 条", len(hit.Excerpts))
	}
	if hit.Excerpts[0].Snippet != "第二段提到了关键词检索。" {
		t.Errorf("摘录应对齐到命中所在的自然段，实际：%q", hit.Excerpts[0].Snippet)
	}
}

func TestSearchInDialogue_LongParagraphCenteredWindow(t *testing.T) {
	// 超长自然段（>400 字符）退化为居中窗口，前后打省略号。
	text := strings.Repeat("垫", 300) + "关键词" + strings.Repeat("垫", 300)
	turns := []DialogueTurn{{Role: RoleUser, Text: text}}
	hit := SearchInDialogue(turns, "关键词", 3, 400)
	if len(hit.Excerpts) != 1 {
		t.Fatalf("期望 1 条摘录，实际 %d 条", len(hit.Excerpts))
	}
	snippet := hit.Excerpts[0].Snippet
	if !strings.HasPrefix(snippet, "…") || !strings.HasSuffix(snippet, "…") {
		t.Errorf("超长段落的摘录前后都应有省略号，实际：%q", snippet)
	}
	if !strings.Contains(snippet, "关键词") {
		t.Errorf("摘录应包含命中词，实际：%q", snippet)
	}
	if len([]rune(snippet)) > 402 {
		t.Errorf("居中窗口不应超过 400 字符加省略号，实际 %d 字符", len([]rune(snippet)))
	}
}

func TestSearchInDialogue_UserExcerptsFirst(t *testing.T) {
	turns := []DialogueTurn{
		{Role: RoleAssistant, Text: "助手先提到了关键词"},
		{Role: RoleUser, Text: "用户也提到了关键词"},
	}
	hit := SearchInDialogue(turns, "关键词", 3, 400)
	if len(hit.Excerpts) != 2 {
		t.Fatalf("期望 2 条摘录，实际 %d 条", len(hit.Excerpts))
	}
	if hit.Excerpts[0].Role != RoleUser || hit.Excerpts[1].Role != RoleAssistant {
		t.Errorf("用户轮次的摘录应排在助手轮次之前，实际顺序：%v", hit.Excerpts)
	}
}

func TestSearchInDialogue_MaxExcerpts(t *testing.T) {
	turns := []DialogueTurn{
		{Role: RoleUser, Text: "关键词一"},
		{Role: RoleUser, Text: "关键词二"},
		{Role: RoleUser, Text: "关键词三"},
		{Role: RoleUser, Text: "关键词四"},
	}
	hit := SearchInDialogue(turns, "关键词", 2, 400)
	if len(hit.Excerpts) != 2 {
		t.Errorf("maxExcerpts=2 时应只返回 2 条摘录，实际 %d 条", len(hit.Excerpts))
	}
	if hit.Count != 4 {
		t.Errorf("摘录截断不应影响 count，期望 4，实际 %d", hit.Count)
	}
}

func TestSearchInDialogue_EmptyKeyword(t *testing.T) {
	turns := []DialogueTurn{{Role: RoleUser, Text: "有些内容"}}
	hit := SearchInDialogue(turns, "   ", 3, 400)
	if hit.Count != 0 || hit.TotalTurns != 1 || len(hit.Excerpts) != 0 {
		t.Errorf("空关键词应返回 count=0、totalTurns=对话规模、摘录为空，实际：%+v", hit)
	}
}

func TestRelevanceScore(t *testing.T) {
	h := SearchHit{UserCount: 2, AsstCount: 1, TotalTurns: 3}
	if got := RelevanceScore(h); got != 7.0/3.0 {
		t.Errorf("加权密度应为 (3*2+1)/3，实际 %v", got)
	}
	if got := RelevanceScore(SearchHit{}); got != 0 {
		t.Errorf("totalTurns=0 时相关度分应为 0，实际 %v", got)
	}
	if got := Density(SearchHit{Count: 2, TotalTurns: 4}); got != 0.5 {
		t.Errorf("普通密度应为 2/4，实际 %v", got)
	}
}

func TestChunkAround(t *testing.T) {
	text := "第一段。\n\n第二段。\n\n第三段。"
	chunk := ChunkAround(text, 6, 400)
	got := string([]rune(text)[chunk.Start:chunk.End])
	if got != "第二段。" {
		t.Errorf("段落对齐窗口应为“第二段。”，实际 %q", got)
	}
	if chunk.Truncated {
		t.Error("短段落不应标记截断")
	}
}

func loadTestTurns(store map[string][]DialogueTurn) func(MemSessionInfo) ([]DialogueTurn, error) {
	return func(s MemSessionInfo) ([]DialogueTurn, error) {
		turns, ok := store[s.ID]
		if !ok {
			return nil, errors.New("会话不存在")
		}
		return turns, nil
	}
}

func TestSearchSessions_DensityOrder(t *testing.T) {
	sessions := []MemSessionInfo{
		{ID: "long", Platform: MemSourceClaude},
		{ID: "short", Platform: MemSourceClaude},
		{ID: "nomatch", Platform: MemSourceClaude},
	}
	store := map[string][]DialogueTurn{
		"long": {
			{Role: RoleUser, Text: "关键词出现一次"},
			{Role: RoleAssistant, Text: "填充"}, {Role: RoleAssistant, Text: "填充"},
			{Role: RoleAssistant, Text: "填充"}, {Role: RoleAssistant, Text: "填充"},
			{Role: RoleAssistant, Text: "填充"}, {Role: RoleAssistant, Text: "填充"},
			{Role: RoleAssistant, Text: "填充"}, {Role: RoleAssistant, Text: "填充"},
			{Role: RoleAssistant, Text: "填充"},
		},
		"short": {
			{Role: RoleUser, Text: "关键词出现一次"},
			{Role: RoleAssistant, Text: "关键词又出现一次"},
		},
		"nomatch": {
			{Role: RoleUser, Text: "毫不相关的内容"},
		},
	}
	matches := SearchMatches(sessions, loadTestTurns(store), MemFilter{Grep: "关键词"})
	if len(matches) != 2 {
		t.Fatalf("应命中 2 个会话，实际 %d 个", len(matches))
	}
	if matches[0].Session.ID != "short" || matches[1].Session.ID != "long" {
		t.Errorf("应按密度（count/totalTurns）降序：短会话密度 1.0 应排在长会话 0.1 之前，实际顺序：%s, %s",
			matches[0].Session.ID, matches[1].Session.ID)
	}
	if matches[0].Score != 1.0 || matches[1].Score != 0.1 {
		t.Errorf("Score 应为密度值，实际：%v, %v", matches[0].Score, matches[1].Score)
	}
	hits := SearchSessions(sessions, loadTestTurns(store), MemFilter{Grep: "关键词"})
	if len(hits) != 2 || hits[0].Count != 2 || hits[1].Count != 1 {
		t.Errorf("SearchSessions 应返回同样顺序的命中，实际：%v", hits)
	}
}

func TestSearchSessions_LimitAndLoadError(t *testing.T) {
	sessions := []MemSessionInfo{
		{ID: "ok1", Platform: MemSourceClaude},
		{ID: "broken", Platform: MemSourceClaude},
		{ID: "ok2", Platform: MemSourceClaude},
	}
	store := map[string][]DialogueTurn{
		"ok1": {{Role: RoleUser, Text: "关键词"}},
		"ok2": {{Role: RoleUser, Text: "关键词"}},
	}
	matches := SearchMatches(sessions, loadTestTurns(store), MemFilter{Grep: "关键词", Limit: 1})
	if len(matches) != 1 {
		t.Errorf("Limit=1 时应只返回 1 个匹配，实际 %d 个", len(matches))
	}
	matches = SearchMatches(sessions, loadTestTurns(store), MemFilter{Grep: "关键词"})
	if len(matches) != 2 {
		t.Errorf("加载失败的会话应跳过、不中断整批，期望 2 个匹配，实际 %d 个", len(matches))
	}
	if got := SearchMatches(sessions, loadTestTurns(store), MemFilter{Grep: ""}); len(got) != 0 {
		t.Errorf("空关键词应返回空匹配，实际 %d 个", len(got))
	}
	if got := SearchMatches(sessions, nil, MemFilter{Grep: "关键词"}); len(got) != 0 {
		t.Errorf("load 为 nil 时应返回空匹配，实际 %d 个", len(got))
	}
}

func TestSearchSessions_IncludeChildren(t *testing.T) {
	parent := MemSessionInfo{ID: "parent", Platform: MemSourceOpencode}
	child := MemSessionInfo{ID: "child", Platform: MemSourceOpencode, ParentID: "parent"}
	grandchild := MemSessionInfo{ID: "grandchild", Platform: MemSourceOpencode, ParentID: "child"}
	sessions := []MemSessionInfo{parent, child, grandchild}
	store := map[string][]DialogueTurn{
		"parent":     {{Role: RoleUser, Text: "父会话内容"}},
		"child":      {{Role: RoleUser, Text: "子会话提到关键词"}},
		"grandchild": {{Role: RoleAssistant, Text: "孙会话也提到关键词"}},
	}
	filter := MemFilter{Grep: "关键词", IncludeChildren: true}
	matches := SearchMatches(sessions, loadTestTurns(store), filter)
	if len(matches) != 1 {
		t.Fatalf("include-children 时子会话应并入父会话，只返回 1 个匹配，实际 %d 个", len(matches))
	}
	if matches[0].Session.ID != "parent" {
		t.Errorf("合并后的匹配应归属父会话，实际：%s", matches[0].Session.ID)
	}
	if matches[0].Hit.Count != 2 {
		t.Errorf("子会话与孙会话的命中都应计入父会话，期望 count=2，实际 %d", matches[0].Hit.Count)
	}
	if matches[0].Hit.TotalTurns != 3 {
		t.Errorf("合并后总轮次应为父 1 + 子 1 + 孙 1 = 3，实际 %d", matches[0].Hit.TotalTurns)
	}
	if matches[0].DescendantsMerged != 2 {
		t.Errorf("并入的子孙会话数应为 2，实际 %d", matches[0].DescendantsMerged)
	}

	// 不开 include-children：三个会话各自检索。
	matches = SearchMatches(sessions, loadTestTurns(store), MemFilter{Grep: "关键词"})
	if len(matches) != 2 {
		t.Fatalf("不开 include-children 时应命中子会话与孙会话共 2 个，实际 %d 个", len(matches))
	}
}
