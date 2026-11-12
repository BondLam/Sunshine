package cli

import (
	"fmt"
	"os"

	"golang.org/x/term"
)

// sunshineBanner 是 init 启动时打印的大字：FIGlet "Rebel" 字体渲染的
// "Sunshine"（与 Trellis init.ts 的 figlet "Rebel" 做法一致，字样固定、
// 直接以内嵌常量存放，运行时不再渲染）。生成方式：node figlet@1.9.4，
// font "Rebel"，每行去尾空格、去尾空行。
const sunshineBanner = `  █████████                                █████       ███
 ███▒▒▒▒▒███                              ▒▒███       ▒▒▒
▒███    ▒▒▒  █████ ████ ████████    █████  ▒███████   ████  ████████    ██████
▒▒█████████ ▒▒███ ▒███ ▒▒███▒▒███  ███▒▒   ▒███▒▒███ ▒▒███ ▒▒███▒▒███  ███▒▒███
 ▒▒▒▒▒▒▒▒███ ▒███ ▒███  ▒███ ▒███ ▒▒█████  ▒███ ▒███  ▒███  ▒███ ▒███ ▒███████
 ███    ▒███ ▒███ ▒███  ▒███ ▒███  ▒▒▒▒███ ▒███ ▒███  ▒███  ▒███ ▒███ ▒███▒▒▒
▒▒█████████  ▒▒████████ ████ █████ ██████  ████ █████ █████ ████ █████▒▒██████
 ▒▒▒▒▒▒▒▒▒    ▒▒▒▒▒▒▒▒ ▒▒▒▒ ▒▒▒▒▒ ▒▒▒▒▒▒  ▒▒▒▒ ▒▒▒▒▒ ▒▒▒▒▒ ▒▒▒▒ ▒▒▒▒▒  ▒▒▒▒▒▒`

// sunshineBannerTagline 是大字下方的标语（CLI 全中文，标语也用中文）。
const sunshineBannerTagline = "开箱即用的 AI 编程工程框架"

// renderInitBanner 拼出 init 启动时打印的整段文本：空行 + 青色大字，
// 再空行 + 灰色标语 + 空行。非终端、NO_COLOR、TERM=dumb 时不着色。
func renderInitBanner(color bool) string {
	if color {
		return "\n\x1b[36m" + sunshineBanner + "\x1b[0m\n" +
			"\x1b[90m\n   " + sunshineBannerTagline + "\n\x1b[0m\n"
	}
	return "\n" + sunshineBanner + "\n\n   " + sunshineBannerTagline + "\n\n"
}

// bannerColorEnabled 按终端能力决定是否给大字着色：只有 stdout 是终端、
// 且未设置 NO_COLOR、TERM 不是 dumb 时才着色（与 chalk 的默认行为一致，
// 管道/重定向输出里不出现转义码）。
func bannerColorEnabled() bool {
	if os.Getenv("NO_COLOR") != "" || os.Getenv("TERM") == "dumb" {
		return false
	}
	return term.IsTerminal(int(os.Stdout.Fd()))
}

// printInitBanner 在 init 命令最开始打印大字与标语。
func printInitBanner() {
	fmt.Fprint(os.Stdout, renderInitBanner(bannerColorEnabled()))
}
