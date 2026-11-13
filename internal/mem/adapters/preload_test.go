package adapters

// search 预加载缓存测试（第 7 项）：
// 预加载命中与逐会话读结果一致、只读哈希不变、
// 库改写后指纹失效（不读脏数据）、平台过滤只预加载范围内。

import (
	"context"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/linrong007/Sunshine/internal/mem"
)

func TestPreloadMsgPartStore_OpenCode(t *testing.T) {
	home := newTempHome(t)
	xdg := filepath.Join(home, "xdg")
	t.Setenv("XDG_DATA_HOME", xdg)
	t.Setenv("OPENCODE_DB", "")
	t.Cleanup(ClearSearchPreload)
	dbPath := filepath.Join(xdg, "opencode", "opencode.db")
	opencodeFixtureDB(t, dbPath)
	ctx := context.Background()
	s := mem.MemSessionInfo{Platform: mem.MemSourceOpencode, ID: "oc-1", CWD: "/tmp/proj", FilePath: dbPath}

	before := hashFile(t, dbPath)
	direct, _, err := OpencodeAdapter{}.Dialogue(ctx, s)
	if err != nil {
		t.Fatalf("预加载前 Dialogue 失败：%v", err)
	}
	if len(direct) != 3 {
		t.Fatalf("预加载前轮次应为 3，实际 %d", len(direct))
	}
	if warnings := PreloadSearchStores(ctx, home, mem.SourceAll); len(warnings) != 0 {
		t.Fatalf("正常夹具预加载不应有警告，实际：%v", warnings)
	}
	if !msgPartStoreCached(dbPath) {
		t.Fatal("预加载后 OpenCode 库应已入缓存")
	}
	cached, _, err := OpencodeAdapter{}.Dialogue(ctx, s)
	if err != nil {
		t.Fatalf("预加载后 Dialogue 失败：%v", err)
	}
	if !reflect.DeepEqual(direct, cached) {
		t.Errorf("预加载命中与逐会话读结果应一致：\n逐会话=%+v\n缓存=%+v", direct, cached)
	}
	if before != hashFile(t, dbPath) {
		t.Error("只读验证：预加载后 OpenCode 会话库哈希变化")
	}
}

func TestPreloadMsgPartStore_Invalidation(t *testing.T) {
	home := newTempHome(t)
	xdg := filepath.Join(home, "xdg")
	t.Setenv("XDG_DATA_HOME", xdg)
	t.Setenv("OPENCODE_DB", "")
	t.Cleanup(ClearSearchPreload)
	dbPath := filepath.Join(xdg, "opencode", "opencode.db")
	opencodeFixtureDB(t, dbPath)
	ctx := context.Background()
	s := mem.MemSessionInfo{Platform: mem.MemSourceOpencode, ID: "oc-1", CWD: "/tmp/proj", FilePath: dbPath}
	if warnings := PreloadSearchStores(ctx, home, mem.SourceAll); len(warnings) != 0 {
		t.Fatalf("预加载不应有警告，实际：%v", warnings)
	}
	first, _, err := OpencodeAdapter{}.Dialogue(ctx, s)
	if err != nil {
		t.Fatal(err)
	}
	// 改写库：换一句对话文本（长度也变了，mtime 与 size 都变）。
	db := openFixtureDB(t, dbPath)
	execFixtureSQL(t, db,
		`UPDATE part SET data = '{"type":"text","text":"改写后的全新文本内容"}' WHERE message_id = 'm1'`,
	)
	db.Close()
	second, _, err := OpencodeAdapter{}.Dialogue(ctx, s)
	if err != nil {
		t.Fatal(err)
	}
	if reflect.DeepEqual(first, second) {
		t.Error("库改写后指纹应失效、读到新内容，实际仍返回旧轮次（读了脏数据）")
	}
	if second[0].Text != "改写后的全新文本内容" {
		t.Errorf("改写后第一轮应为新文本，实际：%q", second[0].Text)
	}
	// ClearSearchPreload 后缓存彻底清空。
	ClearSearchPreload()
	if msgPartStoreCached(dbPath) {
		t.Error("ClearSearchPreload 后缓存应为空")
	}
}

func TestPreloadMsgPartStore_ZCode(t *testing.T) {
	home := newTempHome(t)
	t.Cleanup(ClearSearchPreload)
	dbPath := filepath.Join(home, ".zcode", "cli", "db", "db.sqlite")
	zcodeCollectFixtureDB(t, dbPath)
	ctx := context.Background()
	s := mem.MemSessionInfo{Platform: mem.MemSourceZcode, ID: "z-phase", CWD: "/tmp/proj", FilePath: dbPath}

	dTurns, dEvents, _, err := ZcodeAdapter{}.Collect(ctx, s)
	if err != nil {
		t.Fatalf("预加载前 Collect 失败：%v", err)
	}
	if warnings := PreloadSearchStores(ctx, home, mem.SourceAll); len(warnings) != 0 {
		t.Fatalf("正常夹具预加载不应有警告，实际：%v", warnings)
	}
	if !msgPartStoreCached(dbPath) {
		t.Fatal("预加载后 ZCode 库应已入缓存")
	}
	cTurns, cEvents, _, err := ZcodeAdapter{}.Collect(ctx, s)
	if err != nil {
		t.Fatalf("预加载后 Collect 失败：%v", err)
	}
	if !reflect.DeepEqual(dTurns, cTurns) || !reflect.DeepEqual(dEvents, cEvents) {
		t.Errorf("预加载命中与逐会话读结果应一致：\n轮次 逐会话=%+v 缓存=%+v\n事件 逐会话=%+v 缓存=%+v", dTurns, cTurns, dEvents, cEvents)
	}
}

func TestPreloadDevinStore(t *testing.T) {
	home := newTempHome(t)
	xdg := filepath.Join(home, "xdg")
	t.Setenv("XDG_DATA_HOME", xdg)
	t.Setenv("DEVIN_DB_PATH", "")
	t.Cleanup(ClearSearchPreload)
	dbPath := filepath.Join(xdg, "devin", "cli", "sessions.db")
	devinFixtureDB(t, dbPath)
	ctx := context.Background()
	s := mem.MemSessionInfo{Platform: mem.MemSourceDevin, ID: "dev-1", CWD: "/tmp/proj", FilePath: dbPath}

	direct, _, err := DevinAdapter{}.Dialogue(ctx, s)
	if err != nil {
		t.Fatalf("预加载前 Dialogue 失败：%v", err)
	}
	if warnings := PreloadSearchStores(ctx, home, mem.SourceAll); len(warnings) != 0 {
		t.Fatalf("正常夹具预加载不应有警告，实际：%v", warnings)
	}
	if !devinStoreCached(dbPath) {
		t.Fatal("预加载后 Devin 库应已入缓存")
	}
	cached, _, err := DevinAdapter{}.Dialogue(ctx, s)
	if err != nil {
		t.Fatalf("预加载后 Dialogue 失败：%v", err)
	}
	if !reflect.DeepEqual(direct, cached) {
		t.Errorf("预加载命中与逐会话读结果应一致：\n逐会话=%+v\n缓存=%+v", direct, cached)
	}
}

func TestPreloadSearchStores_MissingAndPlatformFilter(t *testing.T) {
	home := newTempHome(t)
	xdg := filepath.Join(home, "xdg")
	t.Setenv("XDG_DATA_HOME", xdg)
	t.Setenv("OPENCODE_DB", "")
	t.Setenv("DEVIN_DB_PATH", "")
	t.Cleanup(ClearSearchPreload)
	ctx := context.Background()

	// 空 HOME：无库可预加载，不警告、不报错。
	if warnings := PreloadSearchStores(ctx, home, mem.SourceAll); len(warnings) != 0 {
		t.Errorf("空 HOME 预加载不应有警告，实际：%v", warnings)
	}

	// 只建 OpenCode 库；平台过滤为 claude 时不应预加载它。
	dbPath := filepath.Join(xdg, "opencode", "opencode.db")
	opencodeFixtureDB(t, dbPath)
	if warnings := PreloadSearchStores(ctx, home, mem.MemSourceFilter("claude")); len(warnings) != 0 {
		t.Errorf("--platform claude 时预加载不应有警告，实际：%v", warnings)
	}
	if msgPartStoreCached(dbPath) {
		t.Error("--platform claude 时不应预加载 OpenCode 库")
	}
	// 平台过滤为 opencode 时应预加载。
	if warnings := PreloadSearchStores(ctx, home, mem.MemSourceFilter("opencode")); len(warnings) != 0 {
		t.Errorf("--platform opencode 时预加载不应有警告，实际：%v", warnings)
	}
	if !msgPartStoreCached(dbPath) {
		t.Error("--platform opencode 时应预加载 OpenCode 库")
	}
}

// TestLoadMsgPartStoreBulk_EqualsGeneric：lean bulk loader 与通用全库路径
// 产出的 store 应完全一致（消息、part、顺序、seq 都相同）。
func TestLoadMsgPartStoreBulk_EqualsGeneric(t *testing.T) {
	home := newTempHome(t)
	xdg := filepath.Join(home, "xdg")
	t.Setenv("XDG_DATA_HOME", xdg)
	t.Setenv("OPENCODE_DB", "")
	dbPath := filepath.Join(xdg, "opencode", "opencode.db")
	opencodeFixtureDB(t, dbPath)
	ctx := context.Background()
	db, warnings, opened := openMemDB(ctx, dbPath, "OpenCode")
	if !opened {
		t.Fatalf("打不开夹具库：%v", warnings)
	}
	defer db.Close()
	generic, err := loadMsgPartStore(ctx, db, "", true)
	if err != nil {
		t.Fatalf("通用全库路径失败：%v", err)
	}
	bulk, err := loadMsgPartStoreBulk(ctx, db)
	if err != nil {
		t.Fatalf("bulk 路径失败：%v", err)
	}
	if !reflect.DeepEqual(generic, bulk) {
		t.Errorf("bulk loader 与通用全库路径结果不一致：\n通用=%+v\nbulk=%+v", generic, bulk)
	}
}

// TestLoadMsgPartStoreBulk_MalformedEqualsGeneric：畸形 part 数据
// （text 是数字、replace 是字符串）下 bulk 路径与通用路径结果也应一致
// （struct 解析失败时回退 map 解析再转换）。
func TestLoadMsgPartStoreBulk_MalformedEqualsGeneric(t *testing.T) {
	home := newTempHome(t)
	xdg := filepath.Join(home, "xdg")
	t.Setenv("XDG_DATA_HOME", xdg)
	t.Setenv("OPENCODE_DB", "")
	dbPath := filepath.Join(xdg, "opencode", "opencode.db")
	db := openFixtureDB(t, dbPath)
	execFixtureSQL(t, db,
		`CREATE TABLE session (id TEXT, parent_id TEXT, title TEXT, directory TEXT, time_created INTEGER, time_updated INTEGER)`,
		`CREATE TABLE message (id TEXT, session_id TEXT, time_created INTEGER, data TEXT)`,
		`CREATE TABLE part (message_id TEXT, session_id TEXT, time_created INTEGER, data TEXT)`,
		`INSERT INTO session VALUES ('m-1', '', '畸形会话', '/tmp/proj', 1762600000000, 1762603600000)`,
		`INSERT INTO message VALUES ('m1', 'm-1', 1762600000000, '{"role":"user"}')`,
		`INSERT INTO message VALUES ('m2', 'm-1', 1762600100000, '{"role":"assistant"}')`,
		// text 是数字：旧语义下该 part 无文本贡献，但 compaction 标记仍生效。
		`INSERT INTO part VALUES ('m1', 'm-1', 1762600000000, '{"type":"text","text":123}')`,
		`INSERT INTO part VALUES ('m2', 'm-1', 1762600100000, '{"type":"compaction","replace":"yes","tail_start_id":"m1","text":"摘要"}')`,
		`INSERT INTO part VALUES ('m2', 'm-1', 1762600101000, '{"type":"text","text":"摘要文本"}')`,
		`INSERT INTO part VALUES ('m2', 'm-1', 1762600101000, 'not json at all')`,
	)
	db.Close()
	ctx := context.Background()
	db2, warnings, opened := openMemDB(ctx, dbPath, "OpenCode")
	if !opened {
		t.Fatalf("打不开夹具库：%v", warnings)
	}
	defer db2.Close()
	generic, err := loadMsgPartStore(ctx, db2, "", true)
	if err != nil {
		t.Fatalf("通用全库路径失败：%v", err)
	}
	bulk, err := loadMsgPartStoreBulk(ctx, db2)
	if err != nil {
		t.Fatalf("bulk 路径失败：%v", err)
	}
	if !reflect.DeepEqual(generic.messagesBySession, bulk.messagesBySession) {
		t.Errorf("畸形数据下两路消息不一致：\n通用=%+v\nbulk=%+v", generic.messagesBySession, bulk.messagesBySession)
	}
	normParts := func(s *msgPartStore) map[string][]partData {
		out := map[string][]partData{}
		for mid, parts := range s.partsByMsg {
			for _, p := range parts {
				var d partData
				if p.data != nil {
					d = *p.data
				}
				out[mid] = append(out[mid], d)
			}
		}
		return out
	}
	if !reflect.DeepEqual(normParts(generic), normParts(bulk)) {
		t.Errorf("畸形数据下两路 part 不一致：\n通用=%+v\nbulk=%+v", normParts(generic), normParts(bulk))
	}
	// 两路拼出的轮次也应一致：m2 是压缩 marker（tail_start_id 生效）。
	gTurns := opencodeTurnsFromStore(generic, "m-1")
	bTurns := opencodeTurnsFromStore(bulk, "m-1")
	if !reflect.DeepEqual(gTurns, bTurns) {
		t.Errorf("畸形数据下两路轮次不一致：通用=%+v bulk=%+v", gTurns, bTurns)
	}
	if len(gTurns) != 1 || gTurns[0].Kind != "marker" {
		t.Errorf("m2 应为压缩 marker 轮次，实际：%+v", gTurns)
	}
}
