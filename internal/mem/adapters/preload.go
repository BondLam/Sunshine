package adapters

// search 的 SQLite 预加载缓存（第 7 项）。
//
// 问题：`sunshine mem search` 要给每个在 scope 内的会话各读一次对话；
// SQLite 共享库（OpenCode / ZCode 的 message/part、Devin 的
// message_nodes）按会话过滤的查询没有可用的索引，每次都要全表扫描
// 一遍。实测 3000 会话 / 30000 消息的库：单次 Dialogue 均值 10.2ms，
// 一次 scoped search 约 31.8s（达标线 500ms 的约 64 倍）。
//
// 做法：search 入口显式预加载——每个共享库只全量读一次放内存，
// 后续会话直接从内存取。一次全库 bulk load 约 275ms，
// 加上匹配约 76ms，一次 scoped search 回到 500ms 以内。
// JSONL 平台（Claude / Codex / Grok / Pi）是单会话单文件，
// 没有共享库可预加载，不走本缓存（匹配快路径仍使它们受益）。
//
// 失效策略（不得读脏数据）：
//   - 缓存只活在单个 CLI 进程内，不落盘；进程结束即失效，
//     下一次 search 重新从库里读，跨进程绝无过期数据。
//   - 每次取用前按（库文件 + -wal 文件）的 mtime 与 size 指纹校验，
//     不一致就丢弃、当次回退到逐会话读；预加载时加载前后各取一次
//     指纹，前后不一致（加载中有人写库）就不入缓存。
//   - 只在 search 入口显式预加载；extract / context 是单会话，
//     不预加载（全库扫描比单次逐会话读更贵）。
//   - 只读打开（mode=ro），预加载本身不写会话库、不建副作用文件。
//
// 并发：CLI 是单线程调用，加一把互斥锁防测试并行误用。

import (
	"context"
	"fmt"
	"os"
	"strings"
	"sync"

	"github.com/linrong1982/Sunshine/internal/mem"
)

// dbFingerprint 取库文件与 -wal 伴生文件的 mtime + size 指纹。
// -wal 不存在时只算主文件；读不到文件时返回 ""（视为不匹配）。
func dbFingerprint(dbPath string) string {
	var b strings.Builder
	seen := false
	for _, p := range []string{dbPath, dbPath + "-wal"} {
		st, err := os.Stat(p)
		if err != nil {
			continue
		}
		seen = true
		fmt.Fprintf(&b, "%s:%d:%d;", p, st.ModTime().UnixNano(), st.Size())
	}
	if !seen {
		return ""
	}
	return b.String()
}

// ---- message / part 共享库缓存（OpenCode / ZCode）----

type msgPartCacheEntry struct {
	fingerprint string
	store       *msgPartStore
}

var (
	msgPartCacheMu sync.Mutex
	msgPartCache   = map[string]*msgPartCacheEntry{}
)

// cachedMsgPartStore 取指纹仍有效的缓存；缺失或过期返回 nil。
// 过期条目当场删掉，不留脏数据。
func cachedMsgPartStore(dbPath string) *msgPartStore {
	msgPartCacheMu.Lock()
	defer msgPartCacheMu.Unlock()
	entry, ok := msgPartCache[dbPath]
	if !ok {
		return nil
	}
	if entry.fingerprint == "" || entry.fingerprint != dbFingerprint(dbPath) {
		delete(msgPartCache, dbPath)
		return nil
	}
	return entry.store
}

// putMsgPartStore 按加载后的指纹入缓存。
func putMsgPartStore(dbPath string, store *msgPartStore) {
	msgPartCacheMu.Lock()
	defer msgPartCacheMu.Unlock()
	msgPartCache[dbPath] = &msgPartCacheEntry{fingerprint: dbFingerprint(dbPath), store: store}
}

// msgPartStoreCached 供测试判断某库是否已预加载。
func msgPartStoreCached(dbPath string) bool {
	return cachedMsgPartStore(dbPath) != nil
}

// preloadMsgPartStore 全量读一个 message / part 库并入缓存；
// 加载前后指纹不一致（加载中有人写库）时不入缓存，调用方回退逐会话读。
// 库缺失返回 opened=false；结构不支持或读失败返回一条警告。
func preloadMsgPartStore(ctx context.Context, dbPath, platformLabel string) []string {
	before := dbFingerprint(dbPath)
	if before == "" {
		return nil
	}
	db, warnings, opened := openMemDB(ctx, dbPath, platformLabel)
	if !opened {
		return warnings
	}
	defer db.Close()
	if err := checkMsgPartSchema(ctx, db, dbPath, platformLabel); err != nil {
		return []string{err.Error()}
	}
	store, err := loadMsgPartStoreBulk(ctx, db)
	if err != nil {
		return []string{fmt.Sprintf("无法读取 %s 会话库（%s）：%v", platformLabel, dbPath, err)}
	}
	if dbFingerprint(dbPath) != before {
		return nil
	}
	putMsgPartStore(dbPath, store)
	return nil
}

// ---- Devin message_nodes 缓存 ----

type devinCacheEntry struct {
	fingerprint string
	nodes       map[string][]devinSlimNode
	mainChains  map[string]int64
}

var (
	devinCacheMu sync.Mutex
	devinCache   = map[string]*devinCacheEntry{}
)

// cachedDevinStore 取指纹仍有效的 Devin 缓存；缺失或过期返回 ok=false。
func cachedDevinStore(dbPath string) (nodes map[string][]devinSlimNode, mainChains map[string]int64, ok bool) {
	devinCacheMu.Lock()
	defer devinCacheMu.Unlock()
	entry, found := devinCache[dbPath]
	if !found {
		return nil, nil, false
	}
	if entry.fingerprint == "" || entry.fingerprint != dbFingerprint(dbPath) {
		delete(devinCache, dbPath)
		return nil, nil, false
	}
	return entry.nodes, entry.mainChains, true
}

// devinStoreCached 供测试判断某 Devin 库是否已预加载。
func devinStoreCached(dbPath string) bool {
	_, _, ok := cachedDevinStore(dbPath)
	return ok
}

// preloadDevinStore 全量读一个 Devin 库（全部 message_nodes + 各会话
// 的 main_chain_id）并入缓存；加载前后指纹不一致时不入缓存。
func preloadDevinStore(ctx context.Context, dbPath string) []string {
	before := dbFingerprint(dbPath)
	if before == "" {
		return nil
	}
	db, warnings, opened := openMemDB(ctx, dbPath, "Devin")
	if !opened {
		return warnings
	}
	defer db.Close()
	cols, err := TableColumns(ctx, db, "message_nodes")
	if err != nil {
		return []string{fmt.Sprintf("不支持的 Devin 会话库结构（%s）：%v", dbPath, err)}
	}
	if err := RequireColumns(cols, "message_nodes", "session_id", "node_id", "parent_node_id", "chat_message"); err != nil {
		return []string{fmt.Sprintf("不支持的 Devin 会话库结构（%s）：%v", dbPath, err)}
	}
	rows, err := QueryRows(ctx, db, "SELECT session_id, node_id, parent_node_id, chat_message, created_at FROM message_nodes ORDER BY rowid")
	if err != nil {
		return []string{fmt.Sprintf("无法读取 Devin 会话库（%s）：%v", dbPath, err)}
	}
	nodes := map[string][]devinSlimNode{}
	for _, row := range rows {
		sessionID := rowStr(row, "session_id")
		nodeID, ok := rowInt(row, "node_id")
		if sessionID == "" || !ok {
			continue
		}
		var parent *int64
		if p, ok := rowInt(row, "parent_node_id"); ok {
			parent = &p
		}
		createdAt, _ := rowFloat(row, "created_at")
		nodes[sessionID] = append(nodes[sessionID], devinSlimFromChatMessage(nodeID, parent, createdAt, row["chat_message"]))
	}
	mainChains := map[string]int64{}
	if sessionCols, err := TableColumns(ctx, db, "sessions"); err == nil && sessionCols["main_chain_id"] {
		if sessionRows, err := QueryRows(ctx, db, "SELECT id, main_chain_id FROM sessions"); err == nil {
			for _, row := range sessionRows {
				if id := rowStr(row, "id"); id != "" {
					if tip, ok := rowInt(row, "main_chain_id"); ok {
						mainChains[id] = tip
					}
				}
			}
		}
	}
	if dbFingerprint(dbPath) != before {
		return nil
	}
	devinCacheMu.Lock()
	devinCache[dbPath] = &devinCacheEntry{fingerprint: dbFingerprint(dbPath), nodes: nodes, mainChains: mainChains}
	devinCacheMu.Unlock()
	return nil
}

// ClearSearchPreload 清空全部 search 预加载缓存（测试用；
// 正常 CLI 进程结束即失效，不需要调它）。
func ClearSearchPreload() {
	msgPartCacheMu.Lock()
	msgPartCache = map[string]*msgPartCacheEntry{}
	msgPartCacheMu.Unlock()
	devinCacheMu.Lock()
	devinCache = map[string]*devinCacheEntry{}
	devinCacheMu.Unlock()
}

// PreloadSearchStores 预加载 search 要用的 SQLite 共享库：
// OpenCode / ZCode 的 message / part、Devin 的 message_nodes。
// 只预加载平台过滤范围内的库（--platform 只指定某一家时，
// 其余家的库不读，避免白白全量扫描）。
// 尽力而为：库缺失是正常空结果（不警告）；结构不支持或读失败
// 返回一条警告，search 回退到逐会话读，不中断。
func PreloadSearchStores(ctx context.Context, home string, platform mem.MemSourceFilter) []string {
	var warnings []string
	inScope := func(kind mem.MemSourceKind) bool {
		return platform.IsAll() || mem.MemSourceKind(platform) == kind
	}
	if inScope(mem.MemSourceOpencode) {
		if dbPath, ok := OpencodeDBPath(home); ok {
			warnings = append(warnings, preloadMsgPartStore(ctx, dbPath, "OpenCode")...)
		}
	}
	if inScope(mem.MemSourceZcode) {
		if dbPath := ZcodeDBPath(home); fileExists(dbPath) {
			warnings = append(warnings, preloadMsgPartStore(ctx, dbPath, "ZCode")...)
		}
	}
	if inScope(mem.MemSourceDevin) {
		if dbPath, ok := DevinDBPath(home); ok && fileExists(dbPath) {
			warnings = append(warnings, preloadDevinStore(ctx, dbPath)...)
		}
	}
	return warnings
}
