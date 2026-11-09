package migrate

import (
	"strconv"
	"strings"
)

// parsedVersion 是解析后的版本号：数字段 + 预发布标识。
type parsedVersion struct {
	nums []int
	pre  []string
}

// parseVersion 解析 "1.2.3"、"0.7.0-beta.3"、"0.2" 这类版本号；
// 数字段不足三段时补 0。无法解析返回 false。
func parseVersion(s string) (parsedVersion, bool) {
	s = strings.TrimSpace(s)
	if s == "" {
		return parsedVersion{}, false
	}
	s = strings.TrimPrefix(s, "v")
	main, pre, _ := strings.Cut(s, "-")
	parts := strings.Split(main, ".")
	if len(parts) > 3 {
		return parsedVersion{}, false
	}
	v := parsedVersion{nums: make([]int, 3)}
	for i, p := range parts {
		if p == "" {
			return parsedVersion{}, false
		}
		n, err := strconv.Atoi(p)
		if err != nil || n < 0 {
			return parsedVersion{}, false
		}
		v.nums[i] = n
	}
	if pre != "" {
		v.pre = strings.Split(pre, ".")
		for _, id := range v.pre {
			if id == "" {
				return parsedVersion{}, false
			}
		}
	}
	return v, true
}

// CompareVersions 比较版本号：<0 表示 a<b，0 相等，>0 表示 a>b。
// 数字段逐段比；同号时无预发布号的版本高于有预发布号的；预发布标识
// 按点分段比，纯数字段按数值比、数字段小于字母段、字母段按字典序。
// 任一无法解析时返回 0（视为相等，避免误判导致错误迁移）。
func CompareVersions(a, b string) int {
	pa, okA := parseVersion(a)
	pb, okB := parseVersion(b)
	if !okA || !okB {
		return 0
	}
	for i := 0; i < 3; i++ {
		if pa.nums[i] != pb.nums[i] {
			if pa.nums[i] < pb.nums[i] {
				return -1
			}
			return 1
		}
	}
	if len(pa.pre) == 0 && len(pb.pre) == 0 {
		return 0
	}
	// 无预发布号的版本高于有预发布号的同号版本。
	if len(pa.pre) == 0 {
		return 1
	}
	if len(pb.pre) == 0 {
		return -1
	}
	for i := 0; i < len(pa.pre) && i < len(pb.pre); i++ {
		x, y := pa.pre[i], pb.pre[i]
		if x == y {
			continue
		}
		xn, xErr := strconv.Atoi(x)
		yn, yErr := strconv.Atoi(y)
		switch {
		case xErr == nil && yErr == nil:
			if xn < yn {
				return -1
			}
			return 1
		case xErr == nil:
			return -1 // 数字标识小于字母标识
		case yErr == nil:
			return 1
		default:
			return strings.Compare(x, y)
		}
	}
	switch {
	case len(pa.pre) < len(pb.pre):
		return -1
	case len(pa.pre) > len(pb.pre):
		return 1
	default:
		return 0
	}
}
