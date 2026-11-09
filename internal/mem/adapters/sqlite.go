package adapters

import (
	"context"
	"database/sql"
	"fmt"
	"net/url"
	"os"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

// OpenReadOnly 以只读方式打开 SQLite 会话库。
//
//   - DSN mode=ro + query_only：绝不写库，不建 -wal / -shm 副作用文件，
//     不锁用户正在用的会话库；
//   - busy_timeout 3000ms：读锁冲突时由驱动短暂等待；
//   - 仍打不开（锁冲突等）时短暂重试 5 次，仍失败则返回错误，调用方
//     转为警告并跳过该存储。
//
// -wal 伴生文件由驱动自动识别并读取，无需调用方单独处理。
func OpenReadOnly(dbPath string) (*sql.DB, error) {
	dsn := (&url.URL{
		Scheme:   "file",
		Path:     dbPath,
		RawQuery: "mode=ro&_pragma=query_only(1)&_pragma=busy_timeout(3000)",
	}).String()
	var lastErr error
	for attempt := 0; attempt < 5; attempt++ {
		if attempt > 0 {
			time.Sleep(150 * time.Millisecond)
		}
		db, err := sql.Open("sqlite", dsn)
		if err != nil {
			lastErr = err
			continue
		}
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		err = db.PingContext(ctx)
		cancel()
		if err == nil {
			return db, nil
		}
		db.Close()
		lastErr = err
		if !isLockedError(err) {
			break
		}
	}
	return nil, fmt.Errorf("只读打开会话库失败（%s）：%w", dbPath, lastErr)
}

// isLockedError 判断错误是否由读锁冲突引起。
func isLockedError(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "locked") || strings.Contains(msg, "busy")
}

// QueryRows 执行只读查询，返回行（列名 → 值）。TEXT 列的 []byte 会转成 string。
func QueryRows(ctx context.Context, db *sql.DB, query string, args ...any) ([]map[string]any, error) {
	rows, err := db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	cols, err := rows.Columns()
	if err != nil {
		return nil, err
	}
	var out []map[string]any
	for rows.Next() {
		vals := make([]any, len(cols))
		ptrs := make([]any, len(cols))
		for i := range vals {
			ptrs[i] = &vals[i]
		}
		if err := rows.Scan(ptrs...); err != nil {
			return nil, err
		}
		row := make(map[string]any, len(cols))
		for i, col := range cols {
			if b, ok := vals[i].([]byte); ok {
				row[col] = string(b)
			} else {
				row[col] = vals[i]
			}
		}
		out = append(out, row)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

// TableColumns 返回表的列名集合；表不存在时返回错误。
func TableColumns(ctx context.Context, db *sql.DB, table string) (map[string]bool, error) {
	rows, err := QueryRows(ctx, db, fmt.Sprintf("PRAGMA table_info(%q)", table))
	if err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		return nil, fmt.Errorf("缺少表：%s", table)
	}
	cols := make(map[string]bool, len(rows))
	for _, row := range rows {
		if name, ok := row["name"].(string); ok && name != "" {
			cols[name] = true
		}
	}
	return cols, nil
}

// RequireColumns 校验表包含指定列；缺失时返回中文错误。
func RequireColumns(cols map[string]bool, table string, names ...string) error {
	var missing []string
	for _, name := range names {
		if !cols[name] {
			missing = append(missing, name)
		}
	}
	if len(missing) > 0 {
		return fmt.Errorf("表 %s 缺少列：%s", table, strings.Join(missing, "、"))
	}
	return nil
}

// WALPath 返回数据库的 -wal 伴生文件路径。
func WALPath(dbPath string) string {
	return dbPath + "-wal"
}

// DatabaseFiles 返回数据库文件及存在的伴生文件（-wal、-shm），供只读
// 验证（实测前后哈希比对）使用。
func DatabaseFiles(dbPath string) []string {
	files := []string{dbPath}
	for _, suffix := range []string{"-wal", "-shm"} {
		if _, err := os.Stat(dbPath + suffix); err == nil {
			files = append(files, dbPath+suffix)
		}
	}
	return files
}

// rowStr 取行的字符串列值。
func rowStr(row map[string]any, key string) string {
	if v, ok := row[key]; ok {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return ""
}

// rowInt 取行的整数列值（int64 / float64 / []byte 数字均接受）。
func rowInt(row map[string]any, key string) (int64, bool) {
	v, ok := row[key]
	if !ok || v == nil {
		return 0, false
	}
	switch n := v.(type) {
	case int64:
		return n, true
	case float64:
		return int64(n), true
	case string:
		var parsed int64
		if _, err := fmt.Sscanf(n, "%d", &parsed); err == nil {
			return parsed, true
		}
	}
	return 0, false
}

// rowFloat 取行的数字列值。
func rowFloat(row map[string]any, key string) (float64, bool) {
	v, ok := row[key]
	if !ok || v == nil {
		return 0, false
	}
	switch n := v.(type) {
	case float64:
		return n, true
	case int64:
		return float64(n), true
	}
	return 0, false
}
