// Package cli 的 sunshine mem 命令：跨平台检索 AI 会话历史。
package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"strings"

	"github.com/linrong007/Sunshine/internal/mem"
	"github.com/linrong007/Sunshine/internal/mem/adapters"
	"github.com/spf13/cobra"
)

// memHelpText 是 sunshine mem help 打印的中文帮助，
// 与上游 mem.ts 的帮助文本逐项对应。
const memHelpText = `sunshine mem — 跨平台检索 Claude/Codex/Devin/Grok/OpenCode/Pi/ZCode 会话

子命令：
  list                          列出会话（不指定子命令时默认执行）
  search <关键词>               查找内容匹配关键词的会话
  context <会话编号>            深入查看：命中轮次及前后上下文
                                （配合 search 使用；用 --grep 关键词定位）
  extract <会话编号>            导出清理后的完整对话（用 --grep 关键词过滤轮次）
  projects                      列出有会话的项目（工作目录）及会话数，
                                先用它发现要传给 search 的 --cwd

选项：
  --platform claude|codex|devin|grok|opencode|pi|zcode|all   默认 all
  --since YYYY-MM-DD                     含该日期的下限
  --until YYYY-MM-DD                     含该日期的上限
  --global                               检索全部项目（默认只检索当前项目）
  --cwd <路径>                           指定项目目录
  --limit N                              最多输出条数（默认 50）
  --grep 关键词                          extract / context：按关键词过滤轮次（多 token 为 AND）
  --phase brainstorm|implement|all       extract：按工作阶段切片
                                         （默认 all；brainstorm = [sunshine task create, sunshine task start)；
                                         Claude/Codex/Devin/Grok/Pi/ZCode 支持；OpenCode 暂不支持，返回全部内容并警告）
  --turns N                              context：返回的命中轮次数（默认 3）
  --around N                             context：每命中轮次前后各取的轮次数（默认 1）
  --max-chars N                          context：总字符预算（默认 6000，约 1500 token）
  --include-children                     search / context：把 OpenCode 子会话并入父会话
  --json                                 输出 JSON
  --help, -h                             显示本帮助

示例：
  sunshine mem list
  sunshine mem list --global --platform claude --since 2026-04-01
  sunshine mem search "会话洞察" --global
  sunshine mem extract 5842592d --grep 记忆
  sunshine mem extract 5842592d --phase brainstorm
`

// memFlags 持有 sunshine mem 各子命令共享的 flags。
type memFlags struct {
	platform        string
	since           string
	until           string
	global          bool
	cwd             string
	limit           int
	grep            string
	phase           string
	turns           int
	around          int
	maxChars        int
	includeChildren bool
	json            bool
}

// buildFilter 把 flags 翻译成 mem.MemFilter。非法 --platform、
// 非法日期直接报错；phase 只在 extract 里校验（与上游一致：
// list/search 不解析 --phase）。
func (f *memFlags) buildFilter() (mem.MemFilter, error) {
	mf, err := mem.BuildFilter(mem.FilterOptions{
		Platform: f.platform,
		Since:    f.since,
		Until:    f.until,
		Global:   f.global,
		Cwd:      f.cwd,
		Limit:    f.limit,
	})
	if err != nil {
		return mf, err
	}
	mf.Grep = f.grep
	mf.Turns = f.turns
	mf.Around = f.around
	mf.MaxChars = f.maxChars
	mf.IncludeChildren = f.includeChildren
	mf.JSON = f.json
	return mf, nil
}

// RegisterMemCommand 把 `sunshine mem` 接到根命令上。
func RegisterMemCommand(root *cobra.Command) {
	f := &memFlags{}

	memCmd := &cobra.Command{
		Use:   "mem",
		Short: "跨平台检索 AI 会话历史",
		Long: `跨平台检索 AI 会话历史（只读，不写会话存储）。

子命令：list（默认）、search <关键词>、context <会话编号>、
extract <会话编号>、projects、help。
用 sunshine mem help 查看完整用法。`,
		Example: `  sunshine mem list
  sunshine mem search "会话洞察" --global
  sunshine mem extract 5842592d --grep 记忆`,
		Args: cobra.ArbitraryArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) > 0 {
				return fmt.Errorf("未知命令：%s（用 sunshine mem help 查看可用命令）", args[0])
			}
			return runMemList(f)(cmd, args)
		},
	}
	pf := memCmd.PersistentFlags()
	pf.StringVar(&f.platform, "platform", "all", "只检索指定平台的会话：claude|codex|devin|grok|opencode|pi|zcode|all")
	pf.StringVar(&f.since, "since", "", "只列出该日期（含）之后的会话，格式 YYYY-MM-DD")
	pf.StringVar(&f.until, "until", "", "只列出该日期（含）之前的会话，格式 YYYY-MM-DD")
	pf.BoolVar(&f.global, "global", false, "检索全部项目（默认只检索当前项目）")
	pf.StringVar(&f.cwd, "cwd", "", "指定项目目录（默认当前工作目录，含子目录）")
	pf.IntVar(&f.limit, "limit", 50, "最多输出条数")
	pf.StringVar(&f.grep, "grep", "", "按关键词过滤轮次（多 token 为 AND）")
	pf.StringVar(&f.phase, "phase", "all", "extract：按工作阶段切片：brainstorm|implement|all")
	pf.IntVar(&f.turns, "turns", 3, "context：返回的命中轮次数")
	pf.IntVar(&f.around, "around", 1, "context：每命中轮次前后各取的轮次数")
	pf.IntVar(&f.maxChars, "max-chars", 6000, "context：总字符预算")
	pf.BoolVar(&f.includeChildren, "include-children", false, "search/context：把 OpenCode 子会话并入父会话")
	pf.BoolVar(&f.json, "json", false, "输出机器可读的 JSON")

	listCmd := &cobra.Command{
		Use:   "list",
		Short: "列出会话",
		Long: `列出会话（默认作用域为当前项目；--global 检索全部项目）。

示例：
  sunshine mem list
  sunshine mem list --global --platform claude --since 2026-04-01`,
		RunE: runMemList(f),
	}
	searchCmd := &cobra.Command{
		Use:   "search <关键词>",
		Short: "查找内容匹配关键词的会话",
		Long: `查找内容匹配关键词的会话，按相关度密度排序。

示例：
  sunshine mem search "会话洞察" --global`,
		Args: cobra.MaximumNArgs(1),
		RunE: runMemSearch(f),
	}
	contextCmd := &cobra.Command{
		Use:   "context <会话编号>",
		Short: "查看会话上下文",
		Long: `深入查看一次会话：命中轮次及前后上下文。
配合 search 使用；用 --grep 关键词定位命中轮次，
不传 --grep 时取前 --turns 轮。

示例：
  sunshine mem context 5842592d --grep 记忆 --turns 3 --around 1`,
		Args: cobra.MaximumNArgs(1),
		RunE: runMemContext(f),
	}
	extractCmd := &cobra.Command{
		Use:   "extract <会话编号>",
		Short: "导出会话完整对话",
		Long: `导出一次会话清理后的完整对话。
用 --grep 关键词过滤轮次，用 --phase 按工作阶段切片。

示例：
  sunshine mem extract 5842592d --grep 记忆
  sunshine mem extract 5842592d --phase brainstorm`,
		Args: cobra.MaximumNArgs(1),
		RunE: runMemExtract(f),
	}
	projectsCmd := &cobra.Command{
		Use:   "projects",
		Short: "列出有会话的项目",
		Long: `列出有会话的项目（工作目录）及会话数。
先用它发现要传给 search 的 --cwd。

示例：
  sunshine mem projects
  sunshine mem projects --json`,
		RunE: runMemProjects(f),
	}
	helpCmd := &cobra.Command{
		Use:   "help",
		Short: "显示 mem 的帮助信息",
		Long:  "显示 sunshine mem 的完整用法。",
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Print(memHelpText)
		},
	}

	memCmd.AddCommand(listCmd, searchCmd, contextCmd, extractCmd, projectsCmd, helpCmd)
	root.AddCommand(memCmd)
}

// memHome 返回各平台会话存储所在的家目录。
func memHome() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("无法获取用户主目录：%v", err)
	}
	return home, nil
}

// printMemWarnings 把非致命警告打印到标准错误输出。
func printMemWarnings(warnings []string) {
	for _, w := range warnings {
		fmt.Fprintf(os.Stderr, "警告：%s\n", w)
	}
}

// scopeLine 是人类可读输出的范围行。
func scopeLine(f mem.MemFilter) string {
	scope := "全部项目"
	if !f.Global {
		scope = "项目=" + mem.ShortPath(f.Cwd)
	}
	line := fmt.Sprintf("范围：%s  平台=%s", scope, f.Platform)
	if f.HasSince {
		line += "  自=" + f.Since.Format("2006-01-02")
	}
	if f.HasUntil {
		line += "  至=" + f.Until.Format("2006-01-02")
	}
	return line
}

// printMemSessions 打印人类可读的会话列表。
func printMemSessions(rows []mem.MemSessionInfo) {
	if len(rows) == 0 {
		fmt.Println("（暂无会话）")
		return
	}
	for _, s := range rows {
		fmt.Println(memSessionLine(s, ""))
	}
}

// memSessionLine 是一行会话：平台 / 时间 / id / 路径 / 标题 / 子会话标记。
func memSessionLine(s mem.MemSessionInfo, extra string) string {
	id := s.ID
	if len(id) > 12 {
		id = id[:12]
	} else {
		id = fmt.Sprintf("%-12s", id)
	}
	line := fmt.Sprintf("[%-8s] %s  %s  %s%s", s.Platform, mem.ShortDate(memEffectiveTime(s)), id, mem.ShortPath(s.CWD), extra)
	if s.Title != "" {
		line += "  — " + s.Title
	}
	if s.ParentID != "" {
		parent := s.ParentID
		if len(parent) > 12 {
			parent = parent[:12]
		}
		line += "  ↳ 子会话，父会话 " + parent
	}
	return line
}

// memEffectiveTime 取会话的最新时间戳。
func memEffectiveTime(s mem.MemSessionInfo) string {
	if s.Updated != "" {
		return s.Updated
	}
	return s.Created
}

// printMemJSON 输出机器可读的 JSON。
func printMemJSON(v any) error {
	out, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	fmt.Println(string(out))
	return nil
}

// runMemList 是 list 子命令与 mem 默认行为。
func runMemList(f *memFlags) func(cmd *cobra.Command, args []string) error {
	return func(cmd *cobra.Command, args []string) error {
		mf, err := f.buildFilter()
		if err != nil {
			return err
		}
		home, err := memHome()
		if err != nil {
			return err
		}
		rows, warnings, err := mem.ListSessions(context.Background(), home, mf, adapters.All())
		if err != nil {
			return err
		}
		printMemWarnings(warnings)
		if f.json {
			return printMemJSON(rows)
		}
		fmt.Println(scopeLine(mf))
		printMemSessions(rows)
		fmt.Printf("\n共 %d 个会话\n", len(rows))
		return nil
	}
}

// roundScore 把相关度保留 4 位小数（与上游 JSON 输出一致）。
func roundScore(score float64) float64 {
	return math.Round(score*10000) / 10000
}

// memSearchJSONEntry 是 search --json 的单条记录，字段名与上游一致。
type memSearchJSONEntry struct {
	Session           mem.MemSessionInfo  `json:"session"`
	Score             float64             `json:"score"`
	HitCount          int                 `json:"hit_count"`
	UserCount         int                 `json:"user_count"`
	AsstCount         int                 `json:"asst_count"`
	TotalTurns        int                 `json:"total_turns"`
	DescendantsMerged int                 `json:"descendants_merged"`
	Excerpts          []mem.SearchExcerpt `json:"excerpts"`
}

func runMemSearch(f *memFlags) func(cmd *cobra.Command, args []string) error {
	return func(cmd *cobra.Command, args []string) error {
		if len(args) == 0 {
			return fmt.Errorf("用法：search <关键词>")
		}
		kw := args[0]
		mf, err := f.buildFilter()
		if err != nil {
			return err
		}
		mf.Grep = kw
		home, err := memHome()
		if err != nil {
			return err
		}
		ctx := context.Background()
		all := adapters.All()
		// 宽上限取候选会话；命中再按 limit 截断（与上游一致）。
		wide := mf
		wide.Limit = mem.WideLimit
		sessions, warnings, err := mem.ListSessions(ctx, home, wide, all)
		if err != nil {
			return err
		}
		// search 预加载：SQLite 共享库（OpenCode / ZCode / Devin）
		// 只全量读一次放内存，否则每个会话都要重开库、全表扫描一次；
		// 预加载失败只记警告，search 回退到逐会话读，不中断。
		// 只预加载平台过滤范围内的库。
		warnings = append(warnings, adapters.PreloadSearchStores(ctx, home, mf.Platform)...)
		load := func(s mem.MemSessionInfo) ([]mem.DialogueTurn, error) {
			turns, warns, err := mem.LoadDialogue(ctx, s, all)
			warnings = append(warnings, warns...)
			return turns, err
		}
		// SearchMatches 返回带会话身份的匹配（已按相关度密度排好序）；
		// 子会话表取 sessions 与 filter.Sessions 的并集。
		searchFilter := mf
		searchFilter.Limit = mem.WideLimit
		searchFilter.Sessions = sessions
		allMatches := mem.SearchMatches(sessions, load, searchFilter)
		totalMatches := len(allMatches)
		matches := allMatches
		if len(matches) > mf.Limit {
			matches = matches[:mf.Limit]
		}
		printMemWarnings(warnings)

		if f.json {
			entries := make([]memSearchJSONEntry, 0, len(matches))
			for _, m := range matches {
				excerpts := m.Hit.Excerpts
				if excerpts == nil {
					excerpts = []mem.SearchExcerpt{}
				}
				entries = append(entries, memSearchJSONEntry{
					Session:           m.Session,
					Score:             roundScore(m.Score),
					HitCount:          m.Hit.Count,
					UserCount:         m.Hit.UserCount,
					AsstCount:         m.Hit.AsstCount,
					TotalTurns:        m.Hit.TotalTurns,
					DescendantsMerged: m.DescendantsMerged,
					Excerpts:          excerpts,
				})
			}
			return printMemJSON(entries)
		}

		fmt.Printf("%s  关键词=%q", scopeLine(mf), kw)
		if f.includeChildren {
			fmt.Print("  含子会话=开")
		}
		fmt.Println()
		if len(matches) == 0 {
			fmt.Println("（无匹配结果）")
			return nil
		}
		for _, m := range matches {
			s := m.Session
			extra := fmt.Sprintf("  相关度=%.3f  命中=%d（用户=%d，助手=%d）  轮次=%d",
				m.Score, m.Hit.Count, m.Hit.UserCount, m.Hit.AsstCount, m.Hit.TotalTurns)
			if f.includeChildren && m.DescendantsMerged > 0 {
				extra += fmt.Sprintf("  + %d 个子会话", m.DescendantsMerged)
			}
			fmt.Println()
			fmt.Println(memSessionLine(s, extra))
			for _, ex := range m.Hit.Excerpts {
				fmt.Printf("    [%s] %s\n", memRoleLabel(ex.Role), ex.Snippet)
			}
		}
		if totalMatches > len(matches) {
			fmt.Printf("\n共 %d 个会话（共 %d 个）\n", len(matches), totalMatches)
		} else {
			fmt.Printf("\n共 %d 个会话\n", len(matches))
		}
		return nil
	}
}

// memRoleLabel 把角色值转成中文显示。
func memRoleLabel(role string) string {
	if role == mem.RoleUser {
		return "用户"
	}
	return "助手"
}

// runMemProjects 是 projects 子命令：始终全局扫描，
// limit 默认 30（未显式传 --limit 时；与上游一致）。
func runMemProjects(f *memFlags) func(cmd *cobra.Command, args []string) error {
	return func(cmd *cobra.Command, args []string) error {
		mf, err := f.buildFilter()
		if err != nil {
			return err
		}
		mf.Global = true
		mf.Cwd = ""
		limit := 30
		if cmd.Flags().Changed("limit") {
			limit = f.limit
		}
		home, err := memHome()
		if err != nil {
			return err
		}
		rows, warnings, err := mem.ListProjects(context.Background(), home, mf, adapters.All())
		if err != nil {
			return err
		}
		printMemWarnings(warnings)
		top := rows
		if len(top) > limit {
			top = top[:limit]
		}
		if f.json {
			return printMemJSON(top)
		}
		header := "活跃项目"
		if mf.HasSince {
			header += "  自=" + mf.Since.Format("2006-01-02")
		}
		if mf.HasUntil {
			header += "  至=" + mf.Until.Format("2006-01-02")
		}
		fmt.Println(header)
		if len(top) == 0 {
			fmt.Println("（暂无）")
			return nil
		}
		for _, r := range top {
			parts := make([]string, 0, len(mem.MemSourceKinds))
			for _, k := range mem.MemSourceKinds {
				if n := r.ByPlatform[k]; n > 0 {
					parts = append(parts, fmt.Sprintf("%s:%d", k, n))
				}
			}
			fmt.Printf("%s  会话数=%3d (%s)  %s\n",
				mem.ShortDate(r.LastActive), r.Sessions, strings.Join(parts, " "), mem.ShortPath(r.Cwd))
		}
		if len(rows) > len(top) {
			fmt.Printf("\n共 %d 个项目（共 %d 个）\n", len(top), len(rows))
		} else {
			fmt.Printf("\n共 %d 个项目\n", len(top))
		}
		return nil
	}
}

// resolveMemTurns 解析 context 的目标轮次数：0 取 3，负数取 0。
// 与 C 组 resolveContextTurns 的口径一致。
func resolveMemTurns(n int) int {
	if n < 0 {
		return 0
	}
	if n == 0 {
		return 3
	}
	return n
}

// resolveMemMaxChars 解析 context 的字符预算：<= 0 取 6000。
func resolveMemMaxChars(n int) int {
	if n <= 0 {
		return 6000
	}
	return n
}

// findMemSession 按 id 或 id 前缀找会话；找不到时先打警告再报错。
func findMemSession(ctx context.Context, home, id string, mf mem.MemFilter) (mem.MemSessionInfo, error) {
	s, warnings, err := mem.FindSessionByID(ctx, home, id, mf, adapters.All())
	printMemWarnings(warnings)
	if err != nil {
		return s, err
	}
	return s, nil
}

// loadMemTurns 读出一次会话的对话轮次，警告追加到 warnings。
func loadMemTurns(ctx context.Context, s mem.MemSessionInfo, all []mem.Adapter, warnings *[]string) ([]mem.DialogueTurn, error) {
	turns, warns, err := mem.LoadDialogue(ctx, s, all)
	*warnings = append(*warnings, warns...)
	return turns, err
}

// loadMemCollect 一次读出会话的对话轮次与任务命令事件，
// 警告追加到 warnings。extract 用它代替 loadMemTurns，
// 轮次与事件来自同一次遍历，TurnIndex 才对得上。
func loadMemCollect(ctx context.Context, s mem.MemSessionInfo, all []mem.Adapter, warnings *[]string) ([]mem.DialogueTurn, []mem.MemEvent, error) {
	turns, events, warns, err := mem.LoadCollect(ctx, s, all)
	*warnings = append(*warnings, warns...)
	return turns, events, err
}

func runMemContext(f *memFlags) func(cmd *cobra.Command, args []string) error {
	return func(cmd *cobra.Command, args []string) error {
		if len(args) == 0 {
			return fmt.Errorf("用法：context <会话编号> [--grep 关键词] [--turns N] [--around M]")
		}
		id := args[0]
		mf, err := f.buildFilter()
		if err != nil {
			return err
		}
		if cmd.Flags().Changed("grep") && strings.TrimSpace(f.grep) == "" {
			return fmt.Errorf("--grep 需要非空值")
		}
		home, err := memHome()
		if err != nil {
			return err
		}
		ctx := context.Background()
		all := adapters.All()
		s, err := findMemSession(ctx, home, id, mf)
		if err != nil {
			return err
		}
		var warnings []string
		turns, err := loadMemTurns(ctx, s, all, &warnings)
		if err != nil {
			return err
		}
		// include-children 时先把子会话轮次并入父会话（沿 parent_id 链），
		// 再统一选取与渲染；SessionContext 不再二次合并。
		merged := turns
		mergedChildren := 0
		if mf.IncludeChildren {
			wide := mf
			wide.Cwd = ""
			wide.Global = true
			wide.Limit = mem.WideLimit
			allSessions, warns, err := mem.ListSessions(ctx, home, wide, all)
			if err != nil {
				return err
			}
			warnings = append(warnings, warns...)
			kids := mem.BuildChildIndex(allSessions)[s.ID]
			mergedChildren = len(kids)
			for _, kid := range kids {
				childTurns, err := loadMemTurns(ctx, kid, all, &warnings)
				if err != nil {
					continue
				}
				merged = append(merged, childTurns...)
			}
		}
		printMemWarnings(warnings)
		// 选取统计与渲染文本用同一份合并后轮次。
		sel := mem.SelectContextTurns(merged, mf)
		ctxFilter := mf
		ctxFilter.IncludeChildren = false
		text := mem.SessionContext(merged, ctxFilter)
		selTurns := sel.Turns
		if selTurns == nil {
			selTurns = []mem.ContextTurn{}
		}

		if f.json {
			out := map[string]any{
				"session":         s,
				"total_turns":     sel.TotalTurns,
				"total_hit_turns": sel.TotalHitTurns,
				"merged_children": mergedChildren,
				"budget_used":     sel.BudgetUsed,
				"max_chars":       resolveMemMaxChars(mf.MaxChars),
				"turns":           selTurns,
			}
			if mf.Grep != "" {
				out["query"] = mf.Grep
			}
			return printMemJSON(out)
		}

		fmt.Printf("# 上下文：[%s] %s\n", s.Platform, s.ID)
		if s.Title != "" {
			fmt.Printf("# 标题：%s\n", s.Title)
		}
		if s.CWD != "" {
			fmt.Printf("# 工作目录：%s\n", mem.ShortPath(s.CWD))
		}
		nTurns := resolveMemTurns(mf.Turns)
		if mf.Grep != "" {
			shown := sel.TotalHitTurns
			if shown > nTurns {
				shown = nTurns
			}
			fmt.Printf("# 查询：%q  命中轮次=%d  显示前 %d\n", mf.Grep, sel.TotalHitTurns, shown)
		} else {
			shown := nTurns
			if shown > sel.TotalTurns {
				shown = sel.TotalTurns
			}
			fmt.Printf("# 未指定关键词 — 显示前 %d 轮，共 %d 轮\n", shown, sel.TotalTurns)
		}
		if mergedChildren > 0 {
			fmt.Printf("# 合并子会话：%d 个\n", mergedChildren)
		}
		fmt.Printf("# 显示轮次：%d  已用预算：%d/%d 字符\n", len(sel.Turns), sel.BudgetUsed, resolveMemMaxChars(mf.MaxChars))
		fmt.Println()
		fmt.Println(text)
		return nil
	}
}

// memDialogueGroup 是 extract 的一个分组：label 为 nil 时无标题。
type memDialogueGroup struct {
	Label *string            `json:"label"`
	Turns []mem.DialogueTurn `json:"turns"`
}

// memGrepMatch 判断一轮文本是否命中 grep（多 token 为 AND，不区分大小写）。
// 与 C 组 grepMatchCount 的口径一致。
func memGrepMatch(text, grep string) bool {
	tokens := strings.Fields(strings.ToLower(grep))
	if len(tokens) == 0 {
		return true
	}
	hay := strings.ToLower(text)
	for _, tok := range tokens {
		if !strings.Contains(hay, tok) {
			return false
		}
	}
	return true
}

// extractGroups 按 phase 切片 + grep 过滤，返回分组、窗口、
// 清理后对话总轮次、拍平后的轮次与切片警告。phase 为 all 时窗口为空
// （与上游一致）；groups 的轮次与拍平结果与
// mem.ExtractDialogue(allTurns, mf) 在 phase=all 时一致。
// 切片只认适配器 Collect 从工具调用里抽出的 events，
// 不扫轮次文本——真实会话的命令藏在工具调用块里，轮次文本中没有。
// 切片本身走 mem.ExtractDialogue 的事件通道（与直接调同一套
// SliceMemPhase 语义）；窗口与警告仍由 SliceMemPhase 给出
// （同一批输入），供分组与展示。
func extractGroups(allTurns []mem.DialogueTurn, events []mem.MemEvent, platform mem.MemSourceKind, mf mem.MemFilter) (groups []memDialogueGroup, windows []mem.BrainstormWindow, totalTurns int, flat []mem.DialogueTurn, sliceWarnings []string) {
	// 清理后全文：总轮次以它为准。
	cleaned := mem.ExtractDialogue(allTurns, mem.MemFilter{Phase: mem.PhaseAll})
	totalTurns = len(cleaned)
	if mf.Phase == "" {
		mf.Phase = mem.PhaseAll
	}
	// 切片走 ExtractDialogue（事件通道）；grep 按分组另行过滤，
	// 不在这里提前过滤。
	sliceFilter := mf
	sliceFilter.Events = events
	sliceFilter.PlatformKind = platform
	sliceFilter.Grep = ""
	sliced := mem.ExtractDialogue(allTurns, sliceFilter)
	_, windows, sliceWarnings = mem.SliceMemPhase(cleaned, events, mf.Phase, platform)
	if windows == nil {
		windows = []mem.BrainstormWindow{}
	}

	grepFilter := func(turns []mem.DialogueTurn) []mem.DialogueTurn {
		out := make([]mem.DialogueTurn, 0, len(turns))
		for _, t := range turns {
			if memGrepMatch(t.Text, mf.Grep) {
				out = append(out, t)
			}
		}
		return out
	}

	groups = []memDialogueGroup{}
	switch mf.Phase {
	case mem.PhaseBrainstorm:
		if len(windows) == 0 {
			groups = append(groups, memDialogueGroup{Label: nil, Turns: grepFilter(sliced)})
			break
		}
		for _, w := range windows {
			label := w.Label
			groups = append(groups, memDialogueGroup{
				Label: &label,
				Turns: grepFilter(cleaned[w.StartTurn:w.EndTurn]),
			})
		}
	case mem.PhaseImplement:
		groups = append(groups, memDialogueGroup{Label: nil, Turns: grepFilter(sliced)})
	default:
		groups = append(groups, memDialogueGroup{Label: nil, Turns: grepFilter(sliced)})
	}
	flat = make([]mem.DialogueTurn, 0)
	for _, g := range groups {
		flat = append(flat, g.Turns...)
	}
	return groups, windows, totalTurns, flat, sliceWarnings
}

func runMemExtract(f *memFlags) func(cmd *cobra.Command, args []string) error {
	return func(cmd *cobra.Command, args []string) error {
		if len(args) == 0 {
			return fmt.Errorf("用法：extract <会话编号>")
		}
		id := args[0]
		mf, err := f.buildFilter()
		if err != nil {
			return err
		}
		phase, err := mem.ParsePhase(f.phase)
		if err != nil {
			return err
		}
		mf.Phase = phase
		home, err := memHome()
		if err != nil {
			return err
		}
		ctx := context.Background()
		s, err := findMemSession(ctx, home, id, mf)
		if err != nil {
			return err
		}
		var warnings []string
		allTurns, events, err := loadMemCollect(ctx, s, adapters.All(), &warnings)
		if err != nil {
			return err
		}
		printMemWarnings(warnings)
		groups, windows, totalTurns, flat, sliceWarnings := extractGroups(allTurns, events, s.Platform, mf)
		printMemWarnings(sliceWarnings)

		if f.json {
			return printMemJSON(map[string]any{
				"session":     s,
				"phase":       string(phase),
				"windows":     windows,
				"total_turns": totalTurns,
				"groups":      groups,
				"turns":       flat,
			})
		}

		fmt.Printf("# 会话：[%s] %s\n", s.Platform, s.ID)
		if s.Title != "" {
			fmt.Printf("# 标题：%s\n", s.Title)
		}
		if s.CWD != "" {
			fmt.Printf("# 工作目录：%s\n", mem.ShortPath(s.CWD))
		}
		if s.Created != "" {
			fmt.Printf("# 日期：%s\n", mem.ShortDate(s.Created))
		}
		header := fmt.Sprintf("# 阶段：%s  轮次：%d/%d", phase, len(flat), totalTurns)
		if mf.Grep != "" {
			header += fmt.Sprintf("（关键词 /%s/ 过滤）", mf.Grep)
		}
		fmt.Println(header)
		fmt.Println()
		for _, g := range groups {
			if g.Label != nil {
				fmt.Printf("--- 任务：%s ---\n\n", *g.Label)
			}
			for _, t := range g.Turns {
				if t.IsMarker() {
					fmt.Println("## 压缩边界")
				} else if t.Role == mem.RoleUser {
					fmt.Println("## 用户")
				} else {
					fmt.Println("## 助手")
				}
				fmt.Println()
				fmt.Println(t.Text)
				fmt.Print("\n---\n")
			}
		}
		return nil
	}
}
