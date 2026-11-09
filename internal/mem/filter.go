// Package mem 的 flags 校验与过滤：对应上游 buildFilter / filter.ts。
package mem

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// dateLayout 是 --since / --until 接受的日期格式。
const dateLayout = "2006-01-02"

// FilterOptions 是 BuildFilter 的输入：CLI flags 的原始值。
type FilterOptions struct {
	Platform string
	Since    string
	Until    string
	Global   bool
	Cwd      string
	Limit    int
}

// ValidPlatformsText 是 --platform 的合法取值说明（用于报错与帮助）。
func ValidPlatformsText() string {
	kinds := make([]string, 0, len(MemSourceKinds)+1)
	for _, k := range MemSourceKinds {
		kinds = append(kinds, string(k))
	}
	kinds = append(kinds, "all")
	return strings.Join(kinds, "|")
}

// BuildFilter 把 CLI flags 翻译成 MemFilter。校验失败直接返回错误，
// 核心层永远见不到原始 flags。
//
//   - 非法 --platform、非法日期直接报错；
//   - --until 取当天 23:59:59.999Z（含当天全天）；
//   - 默认作用域为当前项目（--cwd 指定目录或当前工作目录，含子目录），
//     --global 时不限定项目。
func BuildFilter(opts FilterOptions) (MemFilter, error) {
	var f MemFilter

	platform := opts.Platform
	if platform == "" {
		platform = string(SourceAll)
	}
	if platform != string(SourceAll) && !IsValidMemSourceKind(platform) {
		return f, fmt.Errorf("未知平台：%s（可用：%s）", platform, ValidPlatformsText())
	}
	f.Platform = MemSourceFilter(platform)

	if opts.Since != "" {
		since, err := time.Parse(dateLayout, opts.Since)
		if err != nil {
			return f, fmt.Errorf("--since 日期格式错误：%q（应为 YYYY-MM-DD）", opts.Since)
		}
		f.Since = since.UTC()
		f.HasSince = true
	}
	if opts.Until != "" {
		until, err := time.Parse(dateLayout, opts.Until)
		if err != nil {
			return f, fmt.Errorf("--until 日期格式错误：%q（应为 YYYY-MM-DD）", opts.Until)
		}
		// 取当天 23:59:59.999Z（含当天全天）。
		f.Until = until.UTC().AddDate(0, 0, 1).Add(-time.Millisecond)
		f.HasUntil = true
	}

	if opts.Global {
		f.Global = true
		f.Cwd = ""
	} else {
		base := opts.Cwd
		if base == "" {
			wd, err := os.Getwd()
			if err != nil {
				return f, fmt.Errorf("无法获取当前工作目录：%v", err)
			}
			base = wd
		}
		abs, err := filepath.Abs(base)
		if err != nil {
			return f, fmt.Errorf("无法解析目录 %q：%v", base, err)
		}
		f.Cwd = abs
	}

	if opts.Limit <= 0 {
		f.Limit = 50
	} else {
		f.Limit = opts.Limit
	}
	f.Phase = PhaseAll
	return f, nil
}

// ParsePhase 解析 --phase。空值视为 all；非法值报错。
func ParsePhase(raw string) (MemPhase, error) {
	switch raw {
	case "", string(PhaseAll):
		return PhaseAll, nil
	case string(PhaseBrainstorm):
		return PhaseBrainstorm, nil
	case string(PhaseImplement):
		return PhaseImplement, nil
	default:
		return PhaseAll, fmt.Errorf("未知 --phase：%s（应为 brainstorm|implement|all）", raw)
	}
}

// ResolveFilter 补齐 platform / limit / phase 的默认值，
// 供内部汇总函数使用。
func ResolveFilter(f MemFilter) MemFilter {
	if f.Platform == "" {
		f.Platform = SourceAll
	}
	if f.Limit <= 0 {
		f.Limit = 50
	}
	if f.Phase == "" {
		f.Phase = PhaseAll
	}
	return f
}

// parseTime 宽松解析会话时间戳：RFC3339 优先，其次 YYYY-MM-DD。
// 解析失败返回零值与 false。
func parseTime(iso string) (time.Time, bool) {
	if iso == "" {
		return time.Time{}, false
	}
	if t, err := time.Parse(time.RFC3339, iso); err == nil {
		return t, true
	}
	if t, err := time.Parse(dateLayout, iso); err == nil {
		return t.UTC(), true
	}
	return time.Time{}, false
}

// InRange 是单点区间检查：since ≤ t ≤ until。
// 时间戳为空或无法解析时放行。
func InRange(iso string, f MemFilter) bool {
	t, ok := parseTime(iso)
	if !ok {
		return true
	}
	if f.HasSince && t.Before(f.Since) {
		return false
	}
	if f.HasUntil && t.After(f.Until) {
		return false
	}
	return true
}

// InRangeOverlap 是区间重叠检查：会话生命 [start, end] 与查询窗口
// [since, until] 有重叠即保留。跨天会话（开始早于 since 但在窗口内
// 仍活跃）不会被丢掉。两端都缺失或无法解析时放行。
func InRangeOverlap(start, end string, f MemFilter) bool {
	s := start
	if s == "" {
		s = end
	}
	e := end
	if e == "" {
		e = start
	}
	if s == "" && e == "" {
		return true
	}
	if f.HasSince && e != "" {
		if eT, ok := parseTime(e); ok && eT.Before(f.Since) {
			return false
		}
	}
	if f.HasUntil && s != "" {
		if sT, ok := parseTime(s); ok && sT.After(f.Until) {
			return false
		}
	}
	return true
}

// SameProject 判断会话目录是否在目标项目内（相等或子目录）。
// 目标为空表示不限定，全部匹配；目标非空而会话目录未知时不匹配。
func SameProject(sessionCWD, target string) bool {
	if target == "" {
		return true
	}
	if sessionCWD == "" {
		return false
	}
	a, err := filepath.Abs(sessionCWD)
	if err != nil {
		a = filepath.Clean(sessionCWD)
	}
	b, err := filepath.Abs(target)
	if err != nil {
		b = filepath.Clean(target)
	}
	return a == b || strings.HasPrefix(a, b+string(filepath.Separator))
}

// MatchesFilter 判断一条会话是否通过全部过滤条件：平台、项目作用域、
// 时间窗口。
func MatchesFilter(s MemSessionInfo, f MemFilter) bool {
	if !f.Platform.IsAll() && MemSourceKind(f.Platform) != s.Platform {
		return false
	}
	if !SameProject(s.CWD, f.Cwd) {
		return false
	}
	return InRangeOverlap(s.Created, s.Updated, f)
}
