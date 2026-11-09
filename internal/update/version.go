package update

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// CompareVersions 比较两个 semver 版本（上游 compare-versions.ts 的移植）：
// -1 表示 a < b，0 表示相等，1 表示 a > b。
// 规则：0.3.0-beta.1 < 0.3.0（预发布版小于正式版）；
// 0.3.0-alpha < 0.3.0-beta（字典序）；0.3.0-beta.1 < 0.3.0-beta.2（数值）；
// 非法段按 0 处理（"unknown" 视为 0.0.0）。
func CompareVersions(a, b string) int {
	splitHyphen := func(v string) (string, string, bool) {
		if i := strings.Index(v, "-"); i >= 0 {
			return v[:i], v[i+1:], true
		}
		return v, "", false
	}
	aBase, aPre, aHasPre := splitHyphen(a)
	bBase, bPre, bHasPre := splitHyphen(b)

	parseBase := func(v string) []int {
		parts := strings.Split(v, ".")
		out := make([]int, len(parts))
		for i, p := range parts {
			n, err := strconv.Atoi(p)
			if err != nil {
				n = 0
			}
			out[i] = n
		}
		return out
	}
	aParts, bParts := parseBase(aBase), parseBase(bBase)
	for i := 0; i < len(aParts) || i < len(bParts); i++ {
		var av, bv int
		if i < len(aParts) {
			av = aParts[i]
		}
		if i < len(bParts) {
			bv = bParts[i]
		}
		if av < bv {
			return -1
		}
		if av > bv {
			return 1
		}
	}

	// 基础版本相等时比较预发布段：无预发布版 > 有预发布版。
	if !aHasPre && bHasPre {
		return 1
	}
	if aHasPre && !bHasPre {
		return -1
	}
	if !aHasPre && !bHasPre {
		return 0
	}
	aPreParts, bPreParts := strings.Split(aPre, "."), strings.Split(bPre, ".")
	for i := 0; i < len(aPreParts) || i < len(bPreParts); i++ {
		if i >= len(aPreParts) {
			return -1
		}
		if i >= len(bPreParts) {
			return 1
		}
		ap, bp := aPreParts[i], bPreParts[i]
		aNum, aErr := strconv.Atoi(ap)
		bNum, bErr := strconv.Atoi(bp)
		aIsNum := aErr == nil && strconv.Itoa(aNum) == ap
		bIsNum := bErr == nil && strconv.Itoa(bNum) == bp
		// 数值标识符的优先级低于字符串标识符。
		if aIsNum && !bIsNum {
			return -1
		}
		if !aIsNum && bIsNum {
			return 1
		}
		if aIsNum && bIsNum {
			if aNum < bNum {
				return -1
			}
			if aNum > bNum {
				return 1
			}
			continue
		}
		if ap < bp {
			return -1
		}
		if ap > bp {
			return 1
		}
	}
	return 0
}

// GetInstalledVersion 读项目版（.sunshine/.version）；文件缺失时返回
// "unknown"（上游同名函数的行为）。
func GetInstalledVersion(cwd string) string {
	data, err := os.ReadFile(filepath.Join(cwd, ".sunshine", ".version"))
	if err != nil {
		return "unknown"
	}
	return strings.TrimSpace(string(data))
}

// WriteVersionFile 把 CLI 版写入 .sunshine/.version（末尾换行，
// 与 init 模板的 .version 文件格式一致）。
func WriteVersionFile(cwd, version string) error {
	return os.WriteFile(
		filepath.Join(cwd, ".sunshine", ".version"),
		[]byte(version+"\n"), 0o644)
}
