// Package mem 的项目汇总：有会话的项目（对应上游 projects.ts）。
package mem

import (
	"context"
	"sort"
)

// ListProjects 汇总有会话的项目：不同工作目录、最近活跃时间、
// 各平台会话数。始终全局扫描（丢掉 cwd 作用域），since / until /
// platform 仍然生效。结果按 last_active 倒序；调用方决定展示上限。
func ListProjects(ctx context.Context, home string, f MemFilter, adapters []Adapter) ([]MemProjectSummary, []string, error) {
	f = ResolveFilter(f)
	f.Cwd = ""
	f.Global = true
	f.Limit = WideLimit
	all, warnings, err := ListSessions(ctx, home, f, adapters)
	if err != nil {
		return nil, warnings, err
	}

	byCwd := make(map[string]*MemProjectSummary)
	for _, s := range all {
		if s.CWD == "" {
			continue
		}
		ts := effectiveTime(s)
		agg, ok := byCwd[s.CWD]
		if !ok {
			byPlatform := make(map[MemSourceKind]int, len(MemSourceKinds))
			for _, k := range MemSourceKinds {
				byPlatform[k] = 0
			}
			agg = &MemProjectSummary{
				Cwd:        s.CWD,
				LastActive: ts,
				ByPlatform: byPlatform,
			}
			byCwd[s.CWD] = agg
		}
		agg.Sessions++
		agg.ByPlatform[s.Platform]++
		if ts > agg.LastActive {
			agg.LastActive = ts
		}
	}

	rows := make([]MemProjectSummary, 0, len(byCwd))
	for _, agg := range byCwd {
		rows = append(rows, *agg)
	}
	sort.SliceStable(rows, func(i, j int) bool {
		if rows[i].LastActive != rows[j].LastActive {
			return rows[i].LastActive > rows[j].LastActive
		}
		return rows[i].Cwd < rows[j].Cwd
	})
	return rows, warnings, nil
}
