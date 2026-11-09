// Package mem 的会话汇总：跨适配器扇出、过滤、排序、按 id 查找。
//
// 适配器由调用方经 adapters.All() 传入（见 internal/mem/adapters），
// 本包不直接依赖 adapters 包，避免与 adapters → mem 的引用形成循环。
package mem

import (
	"context"
	"fmt"
	"os"
	"sort"
	"strings"
)

// WideLimit 是内部宽上限：limit 只截断展示，检索与按 id 查找必须扫全部。
const WideLimit = 1_000_000

// SessionNotFoundError 表示 readMemContext / extractMemDialogue 要找的
// 会话 id 无法解析。
type SessionNotFoundError struct {
	ID       string
	Warnings []string
}

// Error 实现 error 接口。
func (e *SessionNotFoundError) Error() string {
	return "未找到会话：" + e.ID
}

// effectiveTime 取会话的最新时间戳（updated 优先，其次 created）。
func effectiveTime(s MemSessionInfo) string {
	if s.Updated != "" {
		return s.Updated
	}
	return s.Created
}

// sortSessionsByRecency 按最新时间倒序排列；时间相同时按平台、id
// 排定，保证输出确定。
func sortSessionsByRecency(all []MemSessionInfo) {
	sort.SliceStable(all, func(i, j int) bool {
		ti, tj := effectiveTime(all[i]), effectiveTime(all[j])
		if ti != tj {
			return ti > tj
		}
		if all[i].Platform != all[j].Platform {
			return all[i].Platform < all[j].Platform
		}
		return all[i].ID < all[j].ID
	})
}

// ListSessions 汇总全部在 scope 内的平台会话，按最新时间倒序、
// 截断到 filter 的 limit（默认 50）。单个适配器出错时记一条警告并
// 跳过该平台，不中断其他平台。
func ListSessions(ctx context.Context, home string, f MemFilter, adapters []Adapter) ([]MemSessionInfo, []string, error) {
	f = ResolveFilter(f)
	all := make([]MemSessionInfo, 0)
	var warnings []string
	for _, a := range adapters {
		if !f.Platform.IsAll() && MemSourceKind(f.Platform) != a.Kind() {
			continue
		}
		sessions, warns, err := a.Sessions(ctx, home)
		warnings = append(warnings, warns...)
		if err != nil {
			warnings = append(warnings, fmt.Sprintf("读取%s会话失败：%v", a.Kind(), err))
			continue
		}
		for _, s := range sessions {
			if MatchesFilter(s, f) {
				all = append(all, s)
			}
		}
	}
	sortSessionsByRecency(all)
	if len(all) > f.Limit {
		all = all[:f.Limit]
	}
	return all, warnings, nil
}

// FindSessionById 按精确 id 或 id 前缀解析会话，全局扫描（忽略
// 项目作用域）。找不到时返回 *SessionNotFoundError。
func FindSessionByID(ctx context.Context, home string, id string, f MemFilter, adapters []Adapter) (MemSessionInfo, []string, error) {
	wide := ResolveFilter(f)
	wide.Cwd = ""
	wide.Global = true
	wide.Limit = WideLimit
	all, warnings, err := ListSessions(ctx, home, wide, adapters)
	if err != nil {
		return MemSessionInfo{}, warnings, err
	}
	for _, s := range all {
		if s.ID == id {
			return s, warnings, nil
		}
	}
	for _, s := range all {
		if strings.HasPrefix(s.ID, id) {
			return s, warnings, nil
		}
	}
	return MemSessionInfo{}, warnings, &SessionNotFoundError{ID: id, Warnings: warnings}
}

// LoadDialogue 读出一次会话清理后的对话轮次，按会话平台分发到
// 对应适配器。
func LoadDialogue(ctx context.Context, s MemSessionInfo, adapters []Adapter) ([]DialogueTurn, []string, error) {
	for _, a := range adapters {
		if a.Kind() == s.Platform {
			return a.Dialogue(ctx, s)
		}
	}
	return nil, nil, fmt.Errorf("平台 %s 暂无会话适配器", s.Platform)
}

// LoadCollect 一次读出会话清理后的对话轮次与任务命令事件，
// 按会话平台分发到对应适配器。返回的 turns 与 LoadDialogue 一致。
func LoadCollect(ctx context.Context, s MemSessionInfo, adapters []Adapter) ([]DialogueTurn, []MemEvent, []string, error) {
	for _, a := range adapters {
		if a.Kind() == s.Platform {
			return a.Collect(ctx, s)
		}
	}
	return nil, nil, nil, fmt.Errorf("平台 %s 暂无会话适配器", s.Platform)
}

// ShortDate 把 ISO 时间戳截成 "YYYY-MM-DD HH:MM"；无时间戳时
// 返回与该宽度对齐的空白。
func ShortDate(iso string) string {
	if iso == "" {
		return "         "
	}
	if len(iso) >= 16 {
		return strings.Replace(iso[:16], "T", " ", 1)
	}
	return iso
}

// ShortPath 把路径里的用户主目录缩写成 ~；无路径时给占位说明。
func ShortPath(p string) string {
	if p == "" {
		return "（无工作目录）"
	}
	if home, err := os.UserHomeDir(); err == nil && home != "" && p == home {
		return "~"
	} else if err == nil && home != "" && strings.HasPrefix(p, home+string(os.PathSeparator)) {
		return "~" + p[len(home):]
	}
	return p
}
