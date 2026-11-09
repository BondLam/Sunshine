// 任务命令解析与头脑风暴窗口切片、会话阶段判定。
//
// 行为对照上游 phase.ts 重新实现，不复制源码。命令名改为 Go 版：
// 旧版找 `task.py create|start`，Go 版找 `sunshine task create|start`，
// 防误判规则原样保留。

package mem

import (
	"fmt"
	"regexp"
	"sort"
	"strings"
	"unicode"
)

// taskCommandRe 找一条 shell 调用字符串里的全部任务命令。
// 命令名必须出现在命令开头、空白后或路径分隔符后，
// 绝不能嵌在 flag 值里（如 --slug=... 的值）。
var taskCommandRe = regexp.MustCompile(`(^|[\s/\\])sunshine task\s+(create|start)(?:\s+|$)`)

// proseRe 是散文排除规则：动作后面跟一个裸词再加一个纯字母词，
// 是英文散文句式（如 create a new thing），不是真正的调用。
var proseRe = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9_-]*\s+[A-Za-z]{2,}\b`)

// datePrefixRe 去任务目录名开头的 MM-DD- 日期前缀。
var datePrefixRe = regexp.MustCompile(`^\d{2}-\d{2}-`)

// ParseTaskCommandsAll 找一条 shell 调用字符串里的全部
// `sunshine task create|start` 调用，按源码顺序返回。
// 一条真实的 shell 调用可以含多个调用；每条调用的参数只取到
// 下一条调用开始或行末为止。
func ParseTaskCommandsAll(cmd string) []ParsedTaskCommand {
	if cmd == "" {
		return nil
	}
	matches := taskCommandRe.FindAllStringSubmatchIndex(cmd, -1)
	type found struct {
		action    string
		bodyStart int
	}
	var list []found
	for _, m := range matches {
		list = append(list, found{action: cmd[m[4]:m[5]], bodyStart: m[1]})
	}
	var out []ParsedTaskCommand
	for i, cur := range list {
		end := len(cmd)
		if i+1 < len(list) {
			end = list[i+1].bodyStart
		}
		slice := cmd[cur.bodyStart:end]
		restRaw := strings.TrimSpace(strings.SplitN(slice, "\n", 2)[0])
		if proseRe.MatchString(restRaw) {
			continue
		}
		parsed := parseRestOfTaskCommand(cur.action, restRaw)
		if cur.action == "create" && parsed.Action == "create" &&
			parsed.Slug == "" && parsed.TitleArg == "" {
			continue
		}
		if cur.action == "start" && parsed.Action == "start" && parsed.TaskDir == "" {
			continue
		}
		out = append(out, parsed)
	}
	return out
}

// ParseTaskCommand 返回第一条任务命令，没有时返回 nil。
func ParseTaskCommand(cmd string) *ParsedTaskCommand {
	all := ParseTaskCommandsAll(cmd)
	if len(all) == 0 {
		return nil
	}
	return &all[0]
}

// parseRestOfTaskCommand 解析一条调用里动作后面的参数。
// create 取 --slug / -s（或 --slug= 值）与第一个非 flag 参数（标题）；
// start 取第一个非 flag 参数（任务目录）。
func parseRestOfTaskCommand(action, restRaw string) ParsedTaskCommand {
	args := SplitShellArgs(restRaw)
	if action == "create" {
		var slug, titleArg string
		for i := 0; i < len(args); i++ {
			a := args[i]
			if a == "--slug" || a == "-s" {
				if i+1 < len(args) {
					slug = args[i+1]
				}
				i++
				continue
			}
			if strings.HasPrefix(a, "--slug=") {
				slug = strings.TrimPrefix(a, "--slug=")
				continue
			}
			if strings.HasPrefix(a, "-") {
				continue
			}
			if titleArg == "" {
				titleArg = a
			}
		}
		return ParsedTaskCommand{Action: "create", Slug: slug, TitleArg: titleArg}
	}
	var taskDir string
	for _, a := range args {
		if strings.HasPrefix(a, "-") {
			continue
		}
		taskDir = a
		break
	}
	return ParsedTaskCommand{Action: "start", TaskDir: taskDir}
}

// SplitShellArgs 是尽力而为的 shell 参数切分：认 `"…"` / `'…'` 引用，
// 按空白切分，`;`、`|`、`&`、`(`、`)` 是分隔边界，
// 并去掉每个 token 末尾的 shell 尾巴（`)};&|>`）。
// 不是完整的 POSIX 解析器，够从任务命令调用里抠 slug 与路径。
func SplitShellArgs(s string) []string {
	var out []string
	var cur strings.Builder
	var quote rune
	flush := func() {
		if cur.Len() == 0 {
			return
		}
		cleaned := strings.TrimRight(cur.String(), ")};&|>")
		if cleaned != "" {
			out = append(out, cleaned)
		}
		cur.Reset()
	}
	for _, ch := range s {
		if quote != 0 {
			if ch == quote {
				quote = 0
				continue
			}
			cur.WriteRune(ch)
			continue
		}
		switch {
		case ch == '"' || ch == '\'':
			quote = ch
		case unicode.IsSpace(ch):
			flush()
		case ch == ';' || ch == '|' || ch == '&' || ch == '(' || ch == ')':
			flush()
		default:
			cur.WriteRune(ch)
		}
	}
	flush()
	return out
}

// SlugFromTaskDir 从 start 的任务目录路径里取 slug，
// 如 `.sunshine/tasks/05-08-foo/` → `foo`
// （去掉 MM-DD- 日期前缀，以便与配对的 create 的 --slug 对上）。
// 路径为空或取不到段时返回 ""。
func SlugFromTaskDir(p string) string {
	if p == "" {
		return ""
	}
	norm := strings.ReplaceAll(p, "\\", "/")
	norm = strings.TrimRight(norm, "/")
	parts := strings.Split(norm, "/")
	var last string
	for i := len(parts) - 1; i >= 0; i-- {
		if parts[i] != "" {
			last = parts[i]
			break
		}
	}
	if last == "" {
		return ""
	}
	return datePrefixRe.ReplaceAllString(last, "")
}

// EventsFromParsed 把解析出的任务命令转成事件：同一条 shell 调用里的
// 多个调用共用一个 timestamp 与 turnIndex，按源码顺序排列。
func EventsFromParsed(cmds []ParsedTaskCommand, timestamp string, turnIndex int) []MemEvent {
	if len(cmds) == 0 {
		return nil
	}
	events := make([]MemEvent, 0, len(cmds))
	for _, cmd := range cmds {
		events = append(events, MemEvent{
			Action:    cmd.Action,
			Timestamp: timestamp,
			TurnIndex: turnIndex,
			Slug:      cmd.Slug,
			TaskDir:   cmd.TaskDir,
		})
	}
	return events
}

// FindTaskCommands 扫描清理后的对话轮次，找出其中全部任务命令，
// 按轮次顺序、轮次内按源码顺序返回；事件带所在轮次的下标。
// marker 轮次不扫描（它是平台摘要，不是发言）。
// 注意：本函数直接扫轮次原文本，不先过 CleanTurns——
// 命令可能藏在注入标签块里，清理会先把它去掉。
// 轮次文本里没有时间戳，事件的 Timestamp 为空；
// 真实会话的事件应走适配器的 Collect（从工具调用里抽取）。
func FindTaskCommands(turns []DialogueTurn) []MemEvent {
	var events []MemEvent
	for i, t := range turns {
		if t.IsMarker() {
			continue
		}
		events = append(events, EventsFromParsed(ParseTaskCommandsAll(t.Text), "", i)...)
	}
	return events
}

// BuildBrainstormWindows 把 create → start 事件配对成头脑风暴窗口。
//
// 配对策略：
//  1. slug 匹配优先，不论位置；
//  2. 剩下的 create 按 FIFO 与其后出现的下一个未配对 start 配对；
//  3. 未配对的 create → [create, totalTurns)；
//  4. 未配对的 start → [0, start)。
//
// 窗口按 startTurn 升序排列，便于稳定输出。
func BuildBrainstormWindows(events []MemEvent, totalTurns int) []BrainstormWindow {
	var creates, starts []int
	for i, e := range events {
		if e.Action == "create" {
			creates = append(creates, i)
		} else if e.Action == "start" {
			starts = append(starts, i)
		}
	}

	usedStart := map[int]bool{}
	usedCreate := map[int]bool{}
	var windows []BrainstormWindow
	counter := 0
	pushWindow := func(startTurn, endTurn int, slug string, n int) {
		// 挡住畸形窗口（事件交错导致 end 在 start 之前），
		// 不输出负区间。
		if endTurn < startTurn {
			return
		}
		label := slug
		if label == "" {
			label = fmt.Sprintf("window-%d", n)
		}
		windows = append(windows, BrainstormWindow{Label: label, StartTurn: startTurn, EndTurn: endTurn})
	}

	// 第一轮：按 slug 配对。
	for _, ci := range creates {
		createEv := events[ci]
		if createEv.Slug == "" {
			continue
		}
		match := -1
		for _, si := range starts {
			if usedStart[si] {
				continue
			}
			if SlugFromTaskDir(events[si].TaskDir) == createEv.Slug {
				match = si
				break
			}
		}
		if match == -1 {
			continue
		}
		usedStart[match] = true
		usedCreate[ci] = true
		counter++
		pushWindow(createEv.TurnIndex, events[match].TurnIndex, createEv.Slug, counter)
	}

	// 第二轮：剩下的 create 按 FIFO 与其后的 start 配对。
	for _, ci := range creates {
		if usedCreate[ci] {
			continue
		}
		createEv := events[ci]
		paired := -1
		for _, si := range starts {
			if !usedStart[si] && si > ci {
				paired = si
				break
			}
		}
		counter++
		if paired != -1 {
			usedStart[paired] = true
			usedCreate[ci] = true
			slug := createEv.Slug
			if slug == "" {
				slug = SlugFromTaskDir(events[paired].TaskDir)
			}
			pushWindow(createEv.TurnIndex, events[paired].TurnIndex, slug, counter)
		} else {
			usedCreate[ci] = true
			pushWindow(createEv.TurnIndex, totalTurns, createEv.Slug, counter)
		}
	}

	// 第三轮：未配对的 start → [0, start)。
	for _, si := range starts {
		if usedStart[si] {
			continue
		}
		counter++
		pushWindow(0, events[si].TurnIndex, SlugFromTaskDir(events[si].TaskDir), counter)
	}

	sort.Slice(windows, func(i, j int) bool { return windows[i].StartTurn < windows[j].StartTurn })
	return windows
}

// PhaseSupported 判断平台是否支持按阶段切片：只有 OpenCode 不支持
// （其余 6 个平台都支持），与上游 MEM_PLATFORMS 的 phaseSupported 一致。
func PhaseSupported(platform MemSourceKind) bool {
	return platform != MemSourceOpencode
}

// SliceMemPhase 按上游 sliceMemPhase 语义切分清理后的对话：
//
//   - phase=all，或平台不支持（OpenCode）：返回全文 + 空窗口；
//     平台不支持且 phase != all 时另附一条降级警告。
//   - phase=brainstorm：返回全部头脑风暴窗口内的轮次（按窗口顺序拼接）；
//     没有任何任务边界时返回全文 + 一条无边界警告。
//   - phase=implement：返回窗口外的全部轮次；
//     没有任何任务边界时返回空 + 一条无边界警告。
//
// events 是适配器 Collect 从工具调用里抽出的事件（TurnIndex 指向
// turns 里的下标）；turns 应为 Collect 返回的同一份清理后对话。
// 返回切分后的轮次、窗口列表与警告。
func SliceMemPhase(turns []DialogueTurn, events []MemEvent, phase MemPhase, platform MemSourceKind) ([]DialogueTurn, []BrainstormWindow, []string) {
	if phase == PhaseAll || !PhaseSupported(platform) {
		if phase != PhaseAll && !PhaseSupported(platform) {
			return turns, nil, []string{
				fmt.Sprintf("--phase %s 在平台 %s 上暂不支持，返回全部内容。", phase, platform),
			}
		}
		return turns, nil, nil
	}
	windows := BuildBrainstormWindows(events, len(turns))
	if phase == PhaseBrainstorm {
		if len(windows) == 0 {
			return turns, nil, []string{
				"未找到 sunshine task create/start 边界，返回全部内容。",
			}
		}
		out := make([]DialogueTurn, 0)
		for _, w := range windows {
			out = append(out, turns[w.StartTurn:w.EndTurn]...)
		}
		return out, windows, nil
	}
	// phase == implement：窗口外的全部轮次。
	if len(windows) == 0 {
		return []DialogueTurn{}, nil, []string{
			"未找到 sunshine task create/start 边界，implement 阶段为空。",
		}
	}
	covered := make(map[int]bool)
	for _, w := range windows {
		for i := w.StartTurn; i < w.EndTurn; i++ {
			covered[i] = true
		}
	}
	out := make([]DialogueTurn, 0)
	for i, t := range turns {
		if !covered[i] {
			out = append(out, t)
		}
	}
	return out, windows, nil
}
