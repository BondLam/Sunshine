package cli

import (
	"strings"
	"testing"
	"unicode/utf8"
)

func TestRenderInitBanner(t *testing.T) {
	plain := renderInitBanner(false)
	if !strings.Contains(plain, "█████████") {
		t.Fatalf("无色大字里没有块字符：\n%s", plain)
	}
	if !strings.Contains(plain, sunshineBannerTagline) {
		t.Fatalf("无色大字里没有标语：\n%s", plain)
	}
	if strings.Contains(plain, "\x1b[") {
		t.Fatalf("无色输出里不应有转义码：\n%q", plain)
	}
	// 每行不超过 80 列（80 列终端刚好放下），且没有行尾空格。
	// 注意：█/▒ 是 3 字节 UTF-8，按列数算要用 rune 数，不能用 len（字节数）。
	for _, line := range strings.Split(plain, "\n") {
		if utf8.RuneCountInString(line) > 80 {
			t.Fatalf("大字行超宽（%d 列）：%q", utf8.RuneCountInString(line), line)
		}
		if line != strings.TrimRight(line, " ") {
			t.Fatalf("大字行有行尾空格：%q", line)
		}
	}

	colored := renderInitBanner(true)
	if !strings.Contains(colored, "\x1b[36m") {
		t.Fatalf("着色输出里没有青色：\n%q", colored)
	}
	if !strings.Contains(colored, "\x1b[90m") {
		t.Fatalf("着色输出里没有灰色标语：\n%q", colored)
	}
	if !strings.Contains(colored, "\x1b[0m") {
		t.Fatalf("着色输出里没有复位码：\n%q", colored)
	}
	// 除转义码外，着色与无色输出的可见内容应一致。
	strip := func(s string) string {
		for _, code := range []string{"\x1b[36m", "\x1b[90m", "\x1b[0m"} {
			s = strings.ReplaceAll(s, code, "")
		}
		return s
	}
	if strip(colored) != plain {
		t.Fatalf("着色与无色输出的可见内容不一致：\n着色去码后：\n%s\n无色：\n%s", strip(colored), plain)
	}
}
