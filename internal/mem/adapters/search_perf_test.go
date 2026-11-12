package adapters

// search 的 SQLite 预加载缓存决策实测（第 7 项，2026-11-09）。
//
// 用法（默认跳过，不拖慢全量测试）：
//   SUNSHINE_PERF_GEN=1 go test ./internal/mem/adapters/ -run TestSearchPerfGenerate -v
//     在 $SUNSHINE_PERF_HOME（默认 /tmp/sunshine-perf-home）造一份
//     OpenCode 会话库：3000 个会话、30000 条消息、30000 个 part，
//     全部归属 $SUNSHINE_PERF_PROJ（默认 /tmp/sunshine-perf-proj）；
//     每 100 个会话有 1 个含关键词“缓存检索”（共 30 个命中会话）。
//   SUNSHINE_PERF_SEARCH=1 go test ./internal/mem/adapters/ -run TestSearchPerfMeasure -v
//     在进程内跑 ListSessions + SearchMatches（与 CLI 的 search 同路），
//     打每次耗时（p50/p95 见输出）；再用编译出的 sunshine 二进制
//     从项目目录跑 `sunshine mem search` 计端到端耗时，见 PLAN.md。
//
// 该文件只造夹具、不写任何产品代码；实测结论见 PLAN.md“第 7 项”。

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"testing"
	"time"

	"github.com/BondLam/Sunshine/internal/mem"
	_ "modernc.org/sqlite"
)

const (
	perfSessions     = 3000
	perfMsgsPerSess  = 10
	perfKeywordEvery = 100
	perfKeyword      = "缓存检索"
	perfNoHitKeyword = "不存在的关键词不存在"
	perfFiller       = "这是一段用于性能测试的会话文本，讨论日常开发工作、代码审查流程与线上问题排查，内容覆盖需求分析、接口设计与回归验证。"
	perfHitText      = "本轮讨论缓存检索方案：缓存检索的命中策略与失效处理需要重点验证，缓存检索的性能目标是单次查询足够快。"
)

func perfHome(t testing.TB) (home, proj, dbPath string) {
	t.Helper()
	home = os.Getenv("SUNSHINE_PERF_HOME")
	if home == "" {
		home = "/tmp/sunshine-perf-home"
	}
	proj = os.Getenv("SUNSHINE_PERF_PROJ")
	if proj == "" {
		proj = "/tmp/sunshine-perf-proj"
	}
	dbPath = filepath.Join(home, "xdg", "opencode", "opencode.db")
	return home, proj, dbPath
}

// TestSearchPerfGenerate 造大规模 OpenCode 会话库（见文件头用法）。
func TestSearchPerfGenerate(t *testing.T) {
	if os.Getenv("SUNSHINE_PERF_GEN") == "" {
		t.Skip("默认跳过：置 SUNSHINE_PERF_GEN=1 才造大规模性能夹具库")
	}
	home, proj, dbPath := perfHome(t)
	if err := os.MkdirAll(filepath.Dir(dbPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(proj, 0o755); err != nil {
		t.Fatal(err)
	}
	_ = os.Remove(dbPath)
	db, err := sql.Open("sqlite", "file:"+dbPath+"?mode=rwc")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	for _, stmt := range []string{
		`CREATE TABLE session (id TEXT, parent_id TEXT, title TEXT, directory TEXT, time_created INTEGER, time_updated INTEGER)`,
		`CREATE TABLE message (id TEXT, session_id TEXT, time_created INTEGER, data TEXT)`,
		`CREATE TABLE part (message_id TEXT, session_id TEXT, time_created INTEGER, data TEXT)`,
	} {
		if _, err := db.Exec(stmt); err != nil {
			t.Fatalf("建表失败：%v", err)
		}
	}
	start := time.Now()
	tx, err := db.Begin()
	if err != nil {
		t.Fatal(err)
	}
	sessStmt, err := tx.Prepare(`INSERT INTO session VALUES (?, '', ?, ?, ?, ?)`)
	if err != nil {
		t.Fatal(err)
	}
	msgStmt, err := tx.Prepare(`INSERT INTO message VALUES (?, ?, ?, ?)`)
	if err != nil {
		t.Fatal(err)
	}
	partStmt, err := tx.Prepare(`INSERT INTO part VALUES (?, ?, ?, ?)`)
	if err != nil {
		t.Fatal(err)
	}
	base := int64(1762600000000)
	for i := 0; i < perfSessions; i++ {
		sid := fmt.Sprintf("perf-%06d", i)
		ts := base + int64(i)*60000
		if _, err := sessStmt.Exec(sid, fmt.Sprintf("性能会话 %d", i), proj, ts, ts+1000); err != nil {
			t.Fatal(err)
		}
		hit := i%perfKeywordEvery == 0
		for m := 0; m < perfMsgsPerSess; m++ {
			mid := fmt.Sprintf("%s-m%02d", sid, m)
			role := "user"
			if m%2 == 1 {
				role = "assistant"
			}
			mts := ts + int64(m)*1000
			if _, err := msgStmt.Exec(mid, sid, mts, fmt.Sprintf(`{"role":%q}`, role)); err != nil {
				t.Fatal(err)
			}
			text := perfFiller
			if hit && (m == 2 || m == 5) {
				text = perfHitText
			}
			data := fmt.Sprintf(`{"type":"text","text":%q}`, text)
			if _, err := partStmt.Exec(mid, sid, mts, data); err != nil {
				t.Fatal(err)
			}
		}
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	// 命中会话数：每 100 个会话 1 个。
	t.Logf("造库完成：会话=%d 消息=%d part=%d 命中会话=%d 用时=%s 库路径=%s",
		perfSessions, perfSessions*perfMsgsPerSess, perfSessions*perfMsgsPerSess,
		perfSessions/perfKeywordEvery, time.Since(start).Round(time.Millisecond), dbPath)
	if st, err := os.Stat(dbPath); err == nil {
		t.Logf("库文件大小=%.1f MB", float64(st.Size())/1024/1024)
	}
	_ = home
}

// TestSearchPerfMeasure 在进程内跑与 CLI search 同路的检索并计时。
func TestSearchPerfMeasure(t *testing.T) {
	if os.Getenv("SUNSHINE_PERF_SEARCH") == "" {
		t.Skip("默认跳过：置 SUNSHINE_PERF_SEARCH=1 才跑大规模检索计时")
	}
	home, proj, dbPath := perfHome(t)
	if _, err := os.Stat(dbPath); err != nil {
		t.Fatalf("性能库不存在（%s），先跑 SUNSHINE_PERF_GEN=1 的造库测试：%v", dbPath, err)
	}
	t.Setenv("XDG_DATA_HOME", filepath.Join(home, "xdg"))
	t.Setenv("OPENCODE_DB", "")
	ctx := context.Background()
	all := All()
	// 微基准：单次 Dialogue（每次重开 SQLite 库）的成本。
	mf := mem.MemFilter{Cwd: proj, Limit: mem.WideLimit}
	sessions, warnings, err := mem.ListSessions(ctx, home, mf, all)
	if err != nil {
		t.Fatalf("ListSessions 失败：%v", err)
	}
	if len(warnings) != 0 {
		t.Logf("警告：%v", warnings)
	}
	if len(sessions) != perfSessions {
		t.Fatalf("列出会话=%d，想要 %d", len(sessions), perfSessions)
	}
	fmt.Printf("【计时】ListSessions：%d 个会话\n", len(sessions))
	oneStart := time.Now()
	for i := 0; i < 100; i++ {
		if _, _, err := mem.LoadDialogue(ctx, sessions[i%len(sessions)], all); err != nil {
			t.Fatalf("LoadDialogue 失败：%v", err)
		}
	}
	oneAvg := time.Since(oneStart) / 100
	fmt.Printf("【计时】单次 Dialogue 均值=%s（100 次，含每次重开 SQLite 库）\n", oneAvg.Round(time.Microsecond))
	// 分段计时：列会话 vs 预加载 vs 全量加载+检索。
	// 与 CLI 的 search 同路：每轮 search 先 PreloadSearchStores
	//（进程内只做一次，第 1 轮为冷查含 bulk load，后续轮次为热查命中缓存）。
	preloadStart := time.Now()
	preloadWarnings := PreloadSearchStores(ctx, home, mem.SourceAll)
	fmt.Printf("【计时】PreloadSearchStores=%s 警告=%v\n", time.Since(preloadStart).Round(time.Millisecond), preloadWarnings)
	measure := func(keyword string, rounds int) []time.Duration {
		var durs []time.Duration
		for r := 0; r < rounds; r++ {
			listStart := time.Now()
			sessions, _, err := mem.ListSessions(ctx, home, mf, all)
			if err != nil {
				t.Fatalf("ListSessions 失败：%v", err)
			}
			listDur := time.Since(listStart)
			load := func(s mem.MemSessionInfo) ([]mem.DialogueTurn, error) {
				turns, _, err := mem.LoadDialogue(ctx, s, all)
				return turns, err
			}
			searchStart := time.Now()
			sf := mem.MemFilter{Grep: keyword, Limit: mem.WideLimit, Sessions: sessions}
			matches := mem.SearchMatches(sessions, load, sf)
			searchDur := time.Since(searchStart)
			total := listDur + searchDur
			durs = append(durs, total)
			fmt.Printf("【计时】关键词=%q 第 %d 轮：列会话=%s 加载+检索=%s 合计=%s 命中=%d\n",
				keyword, r+1, listDur.Round(time.Millisecond), searchDur.Round(time.Millisecond),
				total.Round(time.Millisecond), len(matches))
		}
		return durs
	}
	for _, kw := range []struct {
		word   string
		rounds int
	}{{perfKeyword, 3}, {perfNoHitKeyword, 1}} {
		durs := measure(kw.word, kw.rounds)
		sort.Slice(durs, func(i, j int) bool { return durs[i] < durs[j] })
		p50 := durs[len(durs)/2]
		p95 := durs[int(float64(len(durs))*0.95)]
		if p95 == 0 {
			p95 = durs[len(durs)-1]
		}
		t.Logf("关键词=%q 进程内检索：轮次=%d p50=%s p95=%s 最大=%s",
			kw.word, len(durs), p50.Round(time.Millisecond), p95.Round(time.Millisecond), durs[len(durs)-1].Round(time.Millisecond))
	}
}

// BenchmarkSearchWithPreload：预加载命中后的整条 search 路径
// （ListSessions + 逐会话 Dialogue（缓存命中）+ SearchMatches），
// 供 CPU profile 定位剩余成本。
func BenchmarkSearchWithPreload(b *testing.B) {
	home, proj, dbPath := perfHome(b)
	if _, err := os.Stat(dbPath); err != nil {
		b.Fatalf("性能库不存在（%s），先跑 SUNSHINE_PERF_GEN=1 的造库测试：%v", dbPath, err)
	}
	b.Setenv("XDG_DATA_HOME", filepath.Join(home, "xdg"))
	b.Setenv("OPENCODE_DB", "")
	ctx := context.Background()
	all := All()
	if warnings := PreloadSearchStores(ctx, home, mem.SourceAll); len(warnings) != 0 {
		b.Fatalf("预加载警告：%v", warnings)
	}
	mf := mem.MemFilter{Cwd: proj, Limit: mem.WideLimit}
	load := func(s mem.MemSessionInfo) ([]mem.DialogueTurn, error) {
		turns, _, err := mem.LoadDialogue(ctx, s, all)
		return turns, err
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		sessions, _, err := mem.ListSessions(ctx, home, mf, all)
		if err != nil {
			b.Fatal(err)
		}
		sf := mem.MemFilter{Grep: perfKeyword, Limit: mem.WideLimit, Sessions: sessions}
		_ = mem.SearchMatches(sessions, load, sf)
	}
}

// BenchmarkSearchMatchesMemory 给纯内存匹配做 CPU profile：
// go test ./internal/mem/adapters/ -bench BenchmarkSearchMatchesMemory -benchtime 3x -run XXX -cpuprofile /tmp/cpu.prof
func BenchmarkSearchMatchesMemory(b *testing.B) {
	sessions := make([]mem.MemSessionInfo, perfSessions)
	turnsBySession := make(map[string][]mem.DialogueTurn, perfSessions)
	for i := 0; i < perfSessions; i++ {
		sid := fmt.Sprintf("perf-%06d", i)
		sessions[i] = mem.MemSessionInfo{Platform: mem.MemSourceOpencode, ID: sid}
		turns := make([]mem.DialogueTurn, perfMsgsPerSess)
		for m := 0; m < perfMsgsPerSess; m++ {
			role := mem.RoleUser
			if m%2 == 1 {
				role = mem.RoleAssistant
			}
			text := perfFiller
			if i%perfKeywordEvery == 0 && (m == 2 || m == 5) {
				text = perfHitText
			}
			turns[m] = mem.DialogueTurn{Role: role, Text: text}
		}
		turnsBySession[sid] = turns
	}
	load := func(s mem.MemSessionInfo) ([]mem.DialogueTurn, error) {
		return turnsBySession[s.ID], nil
	}
	f := mem.MemFilter{Grep: perfKeyword, Limit: mem.WideLimit}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = mem.SearchMatches(sessions, load, f)
	}
}
