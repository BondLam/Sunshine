// 对话窗口上下文抽取：按 grep 命中排序取目标轮次、
// 左右扩展邻近轮次、按字符预算输出；以及完整对话抽取
//（含压缩边界 marker 展示、按工作阶段切片）。
//
// 行为对照上游 context.ts 与 sessions.ts 的抽取/切片部分
// 重新实现，不复制源码。长度按字符（rune）数算。

package mem

import (
	"fmt"
	"sort"
	"strings"
)

// ContextSelection 是 SelectContextTurns 的返回：选中的轮次、
// 命中轮次总数、已用字符预算，以及合并子会话后的对话总轮次。
type ContextSelection struct {
	Turns         []ContextTurn
	TotalHitTurns int
	BudgetUsed    int
	TotalTurns    int
}

// resolveContextTurns 解析目标轮次数：未设置（0）时取 3；
// 负数按 0 计（不取目标轮次）。
func resolveContextTurns(f MemFilter) int {
	if f.Turns <= 0 {
		if f.Turns < 0 {
			return 0
		}
		return 3
	}
	return f.Turns
}

// resolveContextAround 解析邻近轮次数：未设置（0）时取 1；
// 负数按 0 计（不取邻近轮次）。
func resolveContextAround(f MemFilter) int {
	if f.Around <= 0 {
		if f.Around < 0 {
			return 0
		}
		return 1
	}
	return f.Around
}

// resolveContextMaxChars 解析字符预算：未设置（<= 0）时取 6000。
func resolveContextMaxChars(f MemFilter) int {
	if f.MaxChars <= 0 {
		return 6000
	}
	return f.MaxChars
}

// grepMatchCount 数一轮文本里 grep 的命中次数：多 token 为 AND，
// 有一个 token 不出现就返回 0；否则返回全部 token 的出现总数。
func grepMatchCount(text, grep string) int {
	tokens := strings.Fields(strings.ToLower(grep))
	if len(tokens) == 0 {
		return 0
	}
	hay := []rune(strings.ToLower(text))
	total := 0
	for _, tok := range tokens {
		n, _ := countToken(hay, []rune(tok))
		if n == 0 {
			return 0
		}
		total += n
	}
	return total
}

// BuildChildIndex 建 parent_id → 子孙会话表（传递闭包拍平）：
// 只有 parent_id 的会话才进索引；一个父会话的表里包含其全部
// 子孙（先子会话、再孙会话的栈式展开顺序与上游一致）。
func BuildChildIndex(sessions []MemSessionInfo) map[string][]MemSessionInfo {
	direct := map[string][]MemSessionInfo{}
	for _, s := range sessions {
		if s.ParentID == "" {
			continue
		}
		direct[s.ParentID] = append(direct[s.ParentID], s)
	}
	out := map[string][]MemSessionInfo{}
	for pid := range direct {
		stack := append([]MemSessionInfo{}, direct[pid]...)
		var flat []MemSessionInfo
		for len(stack) > 0 {
			cur := stack[len(stack)-1]
			stack = stack[:len(stack)-1]
			flat = append(flat, cur)
			stack = append(stack, direct[cur.ID]...)
		}
		out[pid] = flat
	}
	return out
}

// mergeChildrenTurns 在父会话轮次后并入子会话轮次
// （include-children 时）：子会话表与加载函数都取自 filter；
// 缺会话表、缺加载函数、缺目标会话 id 时不合并。
// 加载失败的子会话跳过，不中断。
func mergeChildrenTurns(turns []DialogueTurn, filter MemFilter) []DialogueTurn {
	if !filter.IncludeChildren || filter.SessionID == "" ||
		len(filter.Sessions) == 0 || filter.LoadTurns == nil {
		return turns
	}
	kids := BuildChildIndex(filter.Sessions)[filter.SessionID]
	if len(kids) == 0 {
		return turns
	}
	merged := append([]DialogueTurn{}, turns...)
	for _, kid := range kids {
		childTurns, err := filter.LoadTurns(kid)
		if err != nil {
			continue
		}
		merged = append(merged, childTurns...)
	}
	return merged
}

// SelectContextTurns 是纯选取逻辑：按 grep 给轮次排序
// （用户轮次优先，再按命中密度，最后按轮次顺序），取前 nTurns 个，
// 每个命中轮次左右各扩展 around 轮，再按 maxChars 预算输出；
// 单轮超过预算一半时从头部截断并标注剩余字符数。
// 无 grep 时返回开头的 nTurns 轮。传入轮次先过 CleanTurns，
// 返回的 Idx 是轮次在清理后对话里的下标。
func SelectContextTurns(turns []DialogueTurn, filter MemFilter) ContextSelection {
	cleaned := CleanTurns(turns)
	nTurns := resolveContextTurns(filter)
	around := resolveContextAround(filter)
	maxChars := resolveContextMaxChars(filter)
	grep := strings.TrimSpace(filter.Grep)

	var hitIndices []int
	totalHitTurns := 0
	if grep != "" {
		type ranked struct {
			idx  int
			role string
			hits int
		}
		var list []ranked
		for i, t := range cleaned {
			if h := grepMatchCount(t.Text, grep); h > 0 {
				list = append(list, ranked{idx: i, role: t.Role, hits: h})
			}
		}
		totalHitTurns = len(list)
		sort.Slice(list, func(a, b int) bool {
			if list[a].role != list[b].role {
				return list[a].role == RoleUser
			}
			if list[a].hits != list[b].hits {
				return list[a].hits > list[b].hits
			}
			return list[a].idx < list[b].idx
		})
		for _, r := range list {
			if len(hitIndices) >= nTurns {
				break
			}
			hitIndices = append(hitIndices, r.idx)
		}
	} else {
		for i := 0; i < nTurns && i < len(cleaned); i++ {
			hitIndices = append(hitIndices, i)
		}
	}

	display := map[int]bool{}
	for _, idx := range hitIndices {
		lo := idx - around
		if lo < 0 {
			lo = 0
		}
		hi := idx + around
		if hi > len(cleaned)-1 {
			hi = len(cleaned) - 1
		}
		for j := lo; j <= hi; j++ {
			display[j] = true
		}
	}
	ordered := make([]int, 0, len(display))
	for i := range display {
		ordered = append(ordered, i)
	}
	sort.Ints(ordered)
	hitSet := map[int]bool{}
	for _, idx := range hitIndices {
		hitSet[idx] = true
	}

	out := make([]ContextTurn, 0, len(ordered))
	used := 0
	cap := maxChars / 2
	for _, i := range ordered {
		t := cleaned[i]
		text := t.Text
		if runeLen(text) > cap {
			r := []rune(text)
			text = string(r[:cap]) + fmt.Sprintf("\n…[另有 %d 字符]", len(r)-cap)
		}
		if used+runeLen(text) > maxChars && len(out) > 0 {
			break
		}
		out = append(out, ContextTurn{Idx: i, Role: t.Role, Text: text, IsHit: hitSet[i]})
		used += runeLen(text)
	}
	return ContextSelection{Turns: out, TotalHitTurns: totalHitTurns, BudgetUsed: used, TotalTurns: len(cleaned)}
}

// roleLabel 把轮次角色译成展示用的中文名；未知角色原样输出。
func roleLabel(role string) string {
	switch role {
	case RoleUser:
		return "用户"
	case RoleAssistant:
		return "助手"
	default:
		return role
	}
}

// SessionContext 取一次会话的上下文并渲染成可读文本：
// 先按 filter 合并子会话（include-children），再按
// SelectContextTurns 选取轮次。每轮的格式是：
//
//	## 第 <下标> 轮（用户/助手） ← 命中
//	<文本>
//	---
//
// 无 grep 时取开头的 turns 轮（默认 3）；有 grep 时取命中最高的
// turns 轮及其前后 around 轮（默认 1），总字符不超过 maxChars
// （默认 6000）。结构化结果（下标、是否命中、已用预算）
// 见 SelectContextTurns。
func SessionContext(turns []DialogueTurn, filter MemFilter) string {
	merged := mergeChildrenTurns(turns, filter)
	sel := SelectContextTurns(merged, filter)
	var b strings.Builder
	for _, t := range sel.Turns {
		marker := ""
		if t.IsHit {
			marker = " ← 命中"
		}
		fmt.Fprintf(&b, "## 第 %d 轮（%s）%s\n\n%s\n\n---\n", t.Idx, roleLabel(t.Role), marker, t.Text)
	}
	return b.String()
}

// sliceByPhase 按工作阶段切分清理后的对话：brainstorm 取全部头脑风暴
// 窗口内的轮次（按窗口顺序拼接）；implement 取窗口外的全部轮次。
// 没有任何任务边界时，brainstorm 返回全文、implement 返回空，
// 与上游切片语义一致（上游同时记一条警告，本函数只返回轮次）。
func sliceByPhase(turns []DialogueTurn, phase MemPhase) []DialogueTurn {
	events := FindTaskCommands(turns)
	windows := BuildBrainstormWindows(events, len(turns))
	if phase == PhaseBrainstorm {
		if len(windows) == 0 {
			return turns
		}
		out := make([]DialogueTurn, 0)
		for _, w := range windows {
			out = append(out, turns[w.StartTurn:w.EndTurn]...)
		}
		return out
	}
	if len(windows) == 0 {
		return []DialogueTurn{}
	}
	covered := map[int]bool{}
	for _, w := range windows {
		for i := w.StartTurn; i < w.EndTurn; i++ {
			covered[i] = true
		}
	}
	out := make([]DialogueTurn, 0)
	for i, t := range turns {
		if !covered[i] {
			out = append(out, t)
		}
	}
	return out
}

// ExtractDialogue 抽取一次会话的完整内容：先按 filter.Phase 切片
// （brainstorm / implement / all，默认 all；非法值按 all 处理，
// 非法值的报错由 CLI 层负责），再按 filter.Grep 过滤轮次
// （多 token 为 AND，不区分大小写；与 context 的 grep 语义一致）。
// marker 轮次保留在结果里，展示模型上下文被截断的位置。
// 本函数不合并子会话（只对 context / search 生效）。
// 注意：这里的阶段切片扫轮次文本（历史行为，见 sliceByPhase）；
// 真实会话的命令藏在工具调用块里，轮次文本中没有，
// CLI 的 extract 走适配器 Collect 的事件 + SliceMemPhase，
// 不要用本函数的 Phase 切片代替它。
func ExtractDialogue(turns []DialogueTurn, filter MemFilter) []DialogueTurn {
	cleaned := CleanTurns(turns)
	phase := filter.Phase
	if phase == "" {
		phase = PhaseAll
	}
	var sliced []DialogueTurn
	switch phase {
	case PhaseAll:
		sliced = cleaned
	case PhaseBrainstorm, PhaseImplement:
		sliced = sliceByPhase(cleaned, phase)
	default:
		sliced = cleaned
	}
	grep := strings.TrimSpace(filter.Grep)
	if grep == "" {
		return sliced
	}
	out := make([]DialogueTurn, 0, len(sliced))
	for _, t := range sliced {
		if grepMatchCount(t.Text, grep) > 0 {
			out = append(out, t)
		}
	}
	return out
}
