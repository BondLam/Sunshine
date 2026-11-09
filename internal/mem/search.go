// 关键词检索：多 token AND 匹配、相关度密度排序、段落对齐摘录。
//
// 行为对照上游 search.ts 重新实现，不复制源码。长度一律按字符
//（rune）数算：中文文本按字节算会把预算与摘录窗口缩掉约三分之二，
// 上游按 UTF-16 码元计长，与 rune 计数基本一致。

package mem

import (
	"sort"
	"strings"
)

// RelevanceScore 是上游的加权密度相关度分：
// (3 * userCount + asstCount) / totalTurns。
// 用户自己的命中权重 ×3：用户原话锚定“他真正在意什么”，
// 助手展开是下游噪音；除以 totalTurns 后，短而集中的会话
// 可以排过冗长松散的会话。totalTurns 为 0 时返回 0。
func RelevanceScore(h SearchHit) float64 {
	if h.TotalTurns == 0 {
		return 0
	}
	return float64(3*h.UserCount+h.AsstCount) / float64(h.TotalTurns)
}

// Density 是普通命中密度：count / totalTurns。
// 本包 SearchSessions / SearchMatches 按它排序（任务要求）；
// totalTurns 为 0 时返回 0。
func Density(h SearchHit) float64 {
	if h.TotalTurns == 0 {
		return 0
	}
	return float64(h.Count) / float64(h.TotalTurns)
}

// TextChunk 是摘录窗口：原文本里的字符下标区间，
// Truncated 表示自然段落超过预算、已退化为居中字符窗口。
type TextChunk struct {
	Start     int
	End       int
	Truncated bool
}

// ChunkAround 取命中位置周围的段落对齐片段：一段是离命中最近的
// 两侧空行（\n\n）之间的连续文本。自然段落超过 maxChars 时，
// 退化为以命中为中心的字符窗口，并报告截断，调用方可据此打省略号。
// hitIdx 与返回值都是字符（rune）下标。
func ChunkAround(text string, hitIdx, maxChars int) TextChunk {
	r := []rune(text)
	if maxChars <= 0 {
		maxChars = 400
	}
	if hitIdx < 0 {
		hitIdx = 0
	}
	if hitIdx > len(r) {
		hitIdx = len(r)
	}
	start := 0
	for i := 0; i+1 < len(r) && i <= hitIdx; i++ {
		if r[i] == '\n' && r[i+1] == '\n' {
			start = i + 2
		}
	}
	end := len(r)
	for i := hitIdx; i+1 < len(r); i++ {
		if r[i] == '\n' && r[i+1] == '\n' {
			end = i
			break
		}
	}
	truncated := false
	if end-start > maxChars {
		start = hitIdx - maxChars/2
		if start < 0 {
			start = 0
		}
		end = hitIdx + (maxChars+1)/2
		if end > len(r) {
			end = len(r)
		}
		truncated = true
	}
	return TextChunk{Start: start, End: end, Truncated: truncated}
}

// countToken 在小写后的字符序列里数一个 token 的非重叠出现次数，
// 并返回每次出现的位置（字符下标）。
func countToken(hay, tok []rune) (int, []int) {
	if len(tok) == 0 {
		return 0, nil
	}
	var positions []int
	for i := 0; i+len(tok) <= len(hay); {
		if runesEqual(hay[i:i+len(tok)], tok) {
			positions = append(positions, i)
			i += len(tok)
			continue
		}
		i++
	}
	return len(positions), positions
}

func runesEqual(a, b []rune) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func runeLen(s string) int {
	return len([]rune(s))
}

// SearchInDialogue 在清理后的对话上做多 token AND 检索。
// 空白切分的每个 token（不区分大小写）都必须出现在一轮里，
// 该轮才算命中；count 是全部匹配轮次里全部 token 的出现总数。
// 摘录是每个命中周围的段落对齐片段，按段首去重，用户轮次的摘录
// 排在助手轮次之前。
//
// marker 轮次不计分、也不计入 totalTurns：池子里已经是压缩前的轮次
// 本身，marker 里的平台摘要复述了它们，两边都算会把一个话题数两遍。
// 传入的轮次会先过 CleanTurns（幂等，已清理过再跑结果不变）。
// maxExcerpts <= 0 时取 3，chunkChars <= 0 时取 400。
func SearchInDialogue(turns []DialogueTurn, keyword string, maxExcerpts, chunkChars int) SearchHit {
	if maxExcerpts <= 0 {
		maxExcerpts = 3
	}
	if chunkChars <= 0 {
		chunkChars = 400
	}
	dialogue := make([]DialogueTurn, 0, len(turns))
	for _, t := range CleanTurns(turns) {
		if t.IsMarker() {
			continue
		}
		dialogue = append(dialogue, t)
	}

	tokens := strings.Fields(strings.ToLower(keyword))
	if len(tokens) == 0 {
		return SearchHit{TotalTurns: len(dialogue), Excerpts: []SearchExcerpt{}}
	}
	tokenRunes := make([][]rune, len(tokens))
	for i, tok := range tokens {
		tokenRunes[i] = []rune(tok)
	}

	var userCount, asstCount int
	var userExcerpts, asstExcerpts []SearchExcerpt

	for _, t := range dialogue {
		hay := []rune(strings.ToLower(t.Text))
		type hitPos struct {
			idx    int
			tokIdx int
		}
		var positions []hitPos
		tokenFreq := make([]int, len(tokens))
		matched := true
		turnHits := 0
		for ti, tr := range tokenRunes {
			n, pos := countToken(hay, tr)
			if n == 0 {
				matched = false
				break
			}
			tokenFreq[ti] = n
			turnHits += n
			for _, p := range pos {
				positions = append(positions, hitPos{idx: p, tokIdx: ti})
			}
		}
		if !matched {
			continue
		}
		if t.Role == RoleUser {
			userCount += turnHits
		} else {
			asstCount += turnHits
		}
		sort.Slice(positions, func(i, j int) bool { return positions[i].idx < positions[j].idx })

		textRunes := []rune(t.Text)
		type candidate struct {
			start     int
			end       int
			truncated bool
			coverage  int
			rarity    float64
		}
		var candidates []candidate
		seenStarts := map[int]bool{}
		for _, hp := range positions {
			chunk := ChunkAround(t.Text, hp.idx, chunkChars)
			if seenStarts[chunk.Start] {
				continue
			}
			seenStarts[chunk.Start] = true
			slice := strings.ToLower(string(textRunes[chunk.Start:chunk.End]))
			coverage := 0
			for _, tok := range tokens {
				if strings.Contains(slice, tok) {
					coverage++
				}
			}
			rarity := 1.0 / float64(tokenFreq[hp.tokIdx])
			candidates = append(candidates, candidate{
				start: chunk.Start, end: chunk.End, truncated: chunk.Truncated,
				coverage: coverage, rarity: rarity,
			})
		}
		sort.Slice(candidates, func(i, j int) bool {
			if candidates[i].coverage != candidates[j].coverage {
				return candidates[i].coverage > candidates[j].coverage
			}
			if candidates[i].rarity != candidates[j].rarity {
				return candidates[i].rarity > candidates[j].rarity
			}
			return candidates[i].start < candidates[j].start
		})
		for _, c := range candidates {
			snippet := strings.TrimSpace(string(textRunes[c.start:c.end]))
			if c.truncated {
				if c.start > 0 {
					snippet = "…" + snippet
				}
				if c.end < len(textRunes) {
					snippet += "…"
				}
			}
			ex := SearchExcerpt{Role: t.Role, Snippet: snippet}
			if t.Role == RoleUser {
				userExcerpts = append(userExcerpts, ex)
			} else {
				asstExcerpts = append(asstExcerpts, ex)
			}
		}
	}

	excerpts := append(append([]SearchExcerpt{}, userExcerpts...), asstExcerpts...)
	if len(excerpts) > maxExcerpts {
		excerpts = excerpts[:maxExcerpts]
	}
	return SearchHit{
		Count:      userCount + asstCount,
		UserCount:  userCount,
		AsstCount:  asstCount,
		TotalTurns: len(dialogue),
		Excerpts:   excerpts,
	}
}

// SearchMatches 在一批会话里检索关键词，返回带会话身份的匹配，
// 按相关度密度（count / totalTurns）降序排列，密度相同按命中数降序，
// 再相同保持传入顺序；结果按 filter.Limit 截断（<= 0 时取 50）。
//
// 关键词取 filter.Grep（多 token 为 AND）；为空时返回空。
// load 负责读出一个会话的对话轮次，读失败的会话跳过（不中断整批）。
// 轮次在计分前过 CleanTurns，marker 轮次不计分。
// filter.IncludeChildren 为真时，沿 parent_id 链把子会话并入父会话
// 再检索：子会话本身不再单独列出，DescendantsMerged 记并入个数；
// 子会话表取 sessions 参数与 filter.Sessions 的并集。
func SearchMatches(sessions []MemSessionInfo, load func(MemSessionInfo) ([]DialogueTurn, error), filter MemFilter) []SearchMatch {
	keyword := strings.TrimSpace(filter.Grep)
	if keyword == "" || load == nil {
		return nil
	}
	limit := filter.Limit
	if limit <= 0 {
		limit = 50
	}

	table := append(append([]MemSessionInfo{}, sessions...), filter.Sessions...)
	childIndex := BuildChildIndex(table)
	candidateIDs := map[string]bool{}
	for _, s := range sessions {
		candidateIDs[s.ID] = true
	}

	var matches []SearchMatch
	for _, s := range sessions {
		if filter.IncludeChildren && s.ParentID != "" && candidateIDs[s.ParentID] {
			continue
		}
		turns, err := load(s)
		if err != nil {
			continue
		}
		merged := 0
		if filter.IncludeChildren {
			for _, child := range childIndex[s.ID] {
				childTurns, err := load(child)
				if err != nil {
					continue
				}
				turns = append(turns, childTurns...)
			}
			merged = len(childIndex[s.ID])
		}
		hit := SearchInDialogue(turns, keyword, 3, 400)
		if hit.Count == 0 {
			continue
		}
		matches = append(matches, SearchMatch{
			Session:           s,
			Score:             Density(hit),
			Hit:               hit,
			DescendantsMerged: merged,
		})
	}
	sort.SliceStable(matches, func(i, j int) bool {
		if matches[i].Score != matches[j].Score {
			return matches[i].Score > matches[j].Score
		}
		return matches[i].Hit.Count > matches[j].Hit.Count
	})
	if len(matches) > limit {
		matches = matches[:limit]
	}
	return matches
}

// SearchSessions 与 SearchMatches 相同，但只返回命中本身（不含会话身份
// 与分值），供只需要命中统计的调用方使用。顺序与截断规则与
// SearchMatches 一致。
func SearchSessions(sessions []MemSessionInfo, load func(MemSessionInfo) ([]DialogueTurn, error), filter MemFilter) []SearchHit {
	matches := SearchMatches(sessions, load, filter)
	hits := make([]SearchHit, 0, len(matches))
	for _, m := range matches {
		hits = append(hits, m.Hit)
	}
	return hits
}
