package mem

import (
	"strings"
	"testing"
)

func TestParseTaskCommandsAll_AtStart(t *testing.T) {
	cmds := ParseTaskCommandsAll("sunshine task create --slug demo")
	if len(cmds) != 1 {
		t.Fatalf("命令开头的调用应命中，实际 %d 条", len(cmds))
	}
	if cmds[0].Action != "create" || cmds[0].Slug != "demo" {
		t.Errorf("应解析出 create/slug=demo，实际：%+v", cmds[0])
	}
}

func TestParseTaskCommandsAll_AfterWhitespaceAndPath(t *testing.T) {
	cmds := ParseTaskCommandsAll("echo ok && sunshine task start .sunshine/tasks/11-09-demo/")
	if len(cmds) != 1 || cmds[0].Action != "start" {
		t.Fatalf("空白后的调用应命中，实际：%v", cmds)
	}
	if cmds[0].TaskDir != ".sunshine/tasks/11-09-demo/" {
		t.Errorf("start 的任务目录应原样取出，实际：%q", cmds[0].TaskDir)
	}

	cmds = ParseTaskCommandsAll("/usr/local/bin/sunshine task create --slug from-path")
	if len(cmds) != 1 || cmds[0].Slug != "from-path" {
		t.Errorf("路径分隔符后的调用应命中，实际：%v", cmds)
	}

	cmds = ParseTaskCommandsAll("C:\\tools\\sunshine task create -s win-path")
	if len(cmds) != 1 || cmds[0].Slug != "win-path" {
		t.Errorf("反斜杠路径分隔符后的调用应命中，实际：%v", cmds)
	}
}

func TestParseTaskCommandsAll_FlagValueNotMatched(t *testing.T) {
	// 命令名嵌在 flag 值里（前面是 =，不是开头/空白/路径分隔符），不算调用。
	cmds := ParseTaskCommandsAll("echo --slug=sunshine task create")
	if len(cmds) != 0 {
		t.Errorf("flag 值里的命令名不应命中，实际：%v", cmds)
	}
	// 嵌在别的词里也不算。
	cmds = ParseTaskCommandsAll("xsunshine task create --slug demo")
	if len(cmds) != 0 {
		t.Errorf("嵌在词里的命令名不应命中，实际：%v", cmds)
	}
	// --slug= 形式的值正常解析。
	cmds = ParseTaskCommandsAll("sunshine task create --slug=eq-form")
	if len(cmds) != 1 || cmds[0].Slug != "eq-form" {
		t.Errorf("--slug= 形式应解析出 slug，实际：%v", cmds)
	}
}

func TestParseTaskCommandsAll_ProseExcluded(t *testing.T) {
	// 纯英文散文句式：动作后跟裸词加纯字母词，不是真正的调用。
	for _, cmd := range []string{
		"sunshine task create a new thing",
		"please sunshine task start the server now",
		"run sunshine task create my first task today",
	} {
		if cmds := ParseTaskCommandsAll(cmd); len(cmds) != 0 {
			t.Errorf("散文句式不应命中：%q 实际解析出 %v", cmd, cmds)
		}
	}
	// 但带 flag 或引用的真实调用不受散文规则影响。
	cmds := ParseTaskCommandsAll("sunshine task create --slug demo 我的任务")
	if len(cmds) != 1 || cmds[0].Slug != "demo" || cmds[0].TitleArg != "我的任务" {
		t.Errorf("带 flag 的真实调用应命中，实际：%v", cmds)
	}
	cmds = ParseTaskCommandsAll(`sunshine task create "我的任务标题"`)
	if len(cmds) != 1 || cmds[0].TitleArg != "我的任务标题" {
		t.Errorf("带引用的标题应解析出来，实际：%v", cmds)
	}
}

func TestParseTaskCommandsAll_MultipleInOrder(t *testing.T) {
	// 一句里多个调用，按源码顺序返回。
	cmds := ParseTaskCommandsAll("sunshine task create --slug first; sunshine task start .sunshine/tasks/11-09-first/; sunshine task create --slug second")
	if len(cmds) != 3 {
		t.Fatalf("一句里的 3 个调用都应按顺序返回，实际 %d 条：%v", len(cmds), cmds)
	}
	if cmds[0].Action != "create" || cmds[0].Slug != "first" {
		t.Errorf("第 1 个应为 create/first，实际：%+v", cmds[0])
	}
	if cmds[1].Action != "start" || cmds[1].TaskDir != ".sunshine/tasks/11-09-first/" {
		t.Errorf("第 2 个应为 start，实际：%+v", cmds[1])
	}
	if cmds[2].Action != "create" || cmds[2].Slug != "second" {
		t.Errorf("第 3 个应为 create/second，实际：%+v", cmds[2])
	}
}

func TestParseTaskCommandsAll_SubshellNotMatched(t *testing.T) {
	// 与上游正则语义一致：$( 紧跟的调用不命中，因为 ( 不是
	// 开头/空白/路径分隔符。只记录该行为，不擅自放宽。
	cmds := ParseTaskCommandsAll("SMOKE=$(sunshine task create --slug a)")
	if len(cmds) != 0 {
		t.Errorf("$( 紧跟的调用按上游语义不应命中，实际：%v", cmds)
	}
}

func TestParseTaskCommandsAll_BareCommandSkipped(t *testing.T) {
	// 光秃的 create（无 slug 也无标题）与光秃的 start（无任务目录）跳过。
	if cmds := ParseTaskCommandsAll("sunshine task create"); len(cmds) != 0 {
		t.Errorf("无参数的 create 应跳过，实际：%v", cmds)
	}
	if cmds := ParseTaskCommandsAll("sunshine task start"); len(cmds) != 0 {
		t.Errorf("无参数的 start 应跳过，实际：%v", cmds)
	}
	if cmds := ParseTaskCommandsAll("sunshine task create --verbose"); len(cmds) != 0 {
		t.Errorf("只有 flag、没有 slug/标题的 create 应跳过，实际：%v", cmds)
	}
}

func TestParseTaskCommand_FirstOnly(t *testing.T) {
	cmd := ParseTaskCommand("sunshine task create --slug first; sunshine task start .sunshine/tasks/11-09-first/")
	if cmd == nil || cmd.Action != "create" || cmd.Slug != "first" {
		t.Errorf("应返回第一条调用，实际：%v", cmd)
	}
	if cmd := ParseTaskCommand("没有命令"); cmd != nil {
		t.Errorf("没有调用时应返回 nil，实际：%v", cmd)
	}
}

func TestSplitShellArgs(t *testing.T) {
	got := SplitShellArgs(`--slug "my task" --flag 'single quoted' plain; next`)
	want := []string{"--slug", "my task", "--flag", "single quoted", "plain", "next"}
	if len(got) != len(want) {
		t.Fatalf("切分结果应为 %v，实际 %v", want, got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("第 %d 个参数应为 %q，实际 %q", i, want[i], got[i])
		}
	}
	got = SplitShellArgs("a) b} c; d|e&f")
	for _, v := range got {
		if v != "a" && v != "b" && v != "c" && v != "d" && v != "e" && v != "f" {
			t.Errorf("shell 尾巴应被去掉，出现意外参数 %q（全部：%v）", v, got)
		}
	}
	if len(got) != 6 {
		t.Errorf("应切出 6 个参数，实际 %v", got)
	}
}

func TestSlugFromTaskDir(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{".sunshine/tasks/05-08-mem-phase-slice/", "mem-phase-slice"},
		{".sunshine/tasks/11-09-demo", "demo"},
		{"tasks/no-date-slug", "no-date-slug"},
		{`C:\proj\.sunshine\tasks\01-02-win`, "win"},
		{"", ""},
		{"/", ""},
	}
	for _, c := range cases {
		if got := SlugFromTaskDir(c.in); got != c.want {
			t.Errorf("SlugFromTaskDir(%q) 应为 %q，实际 %q", c.in, c.want, got)
		}
	}
}

func TestBuildBrainstormWindows_SlugPairing(t *testing.T) {
	events := []MemEvent{
		{Action: "create", TurnIndex: 1, Slug: "alpha"},
		{Action: "create", TurnIndex: 3, Slug: "beta"},
		{Action: "start", TurnIndex: 5, TaskDir: ".sunshine/tasks/11-09-beta/"},
		{Action: "start", TurnIndex: 7, TaskDir: ".sunshine/tasks/11-09-alpha/"},
	}
	windows := BuildBrainstormWindows(events, 10)
	if len(windows) != 2 {
		t.Fatalf("slug 配对应产出 2 个窗口，实际 %d 个：%v", len(windows), windows)
	}
	// 按 startTurn 升序：alpha 窗口 [1,7)，beta 窗口 [3,5)。
	if windows[0].Label != "alpha" || windows[0].StartTurn != 1 || windows[0].EndTurn != 7 {
		t.Errorf("alpha 窗口应为 [1,7)，实际：%+v", windows[0])
	}
	if windows[1].Label != "beta" || windows[1].StartTurn != 3 || windows[1].EndTurn != 5 {
		t.Errorf("beta 窗口应为 [3,5)，实际：%+v", windows[1])
	}
}

func TestBuildBrainstormWindows_FIFOFallbackAndUnmatched(t *testing.T) {
	// 无 slug 的 create 按 FIFO 与其后的 start 配对；
	// 未配对的 create 延到对话末尾；未配对的 start 从 0 开始。
	events := []MemEvent{
		{Action: "start", TurnIndex: 1, TaskDir: ".sunshine/tasks/11-09-early/"},
		{Action: "create", TurnIndex: 2},
		{Action: "start", TurnIndex: 4, TaskDir: ".sunshine/tasks/11-09-foo/"},
		{Action: "create", TurnIndex: 6, Slug: "lonely"},
	}
	windows := BuildBrainstormWindows(events, 10)
	if len(windows) != 3 {
		t.Fatalf("应产出 3 个窗口，实际 %d 个：%v", len(windows), windows)
	}
	// 排序后：[0,1) early、[2,4) foo、[6,10) lonely。
	if windows[0].Label != "early" || windows[0].StartTurn != 0 || windows[0].EndTurn != 1 {
		t.Errorf("未配对 start 的窗口应为 [0,1)/early，实际：%+v", windows[0])
	}
	if windows[1].Label != "foo" || windows[1].StartTurn != 2 || windows[1].EndTurn != 4 {
		t.Errorf("FIFO 配对窗口应为 [2,4)/foo，实际：%+v", windows[1])
	}
	if windows[2].Label != "lonely" || windows[2].StartTurn != 6 || windows[2].EndTurn != 10 {
		t.Errorf("未配对 create 的窗口应为 [6,10)/lonely，实际：%+v", windows[2])
	}
}

func TestFindTaskCommands_TurnIndex(t *testing.T) {
	turns := []DialogueTurn{
		{Role: RoleUser, Text: "开场"},
		{Role: RoleAssistant, Text: "sunshine task create --slug a; sunshine task start .sunshine/tasks/11-09-a/"},
		{Role: RoleUser, Text: "收尾"},
	}
	events := FindTaskCommands(turns)
	if len(events) != 2 {
		t.Fatalf("应找到 2 个事件，实际 %d 个", len(events))
	}
	if events[0].TurnIndex != 1 || events[1].TurnIndex != 1 {
		t.Errorf("两个事件都应在第 1 轮，实际：%v", events)
	}
	if events[0].Action != "create" || events[1].Action != "start" {
		t.Errorf("事件顺序应与源码顺序一致，实际：%v", events)
	}
}

func TestSliceMemPhase(t *testing.T) {
	turns := []DialogueTurn{
		{Role: RoleUser, Text: "先想想做什么"},
		{Role: RoleAssistant, Text: "方案草稿"},
		{Role: RoleUser, Text: "方案定了，开始动手"},
		{Role: RoleAssistant, Text: "写代码中"},
		{Role: RoleAssistant, Text: "继续实现功能"},
	}
	events := []MemEvent{
		{Action: "create", Timestamp: "2026-11-09T10:01:00.000Z", TurnIndex: 1, Slug: "demo"},
		{Action: "start", Timestamp: "2026-11-09T10:02:00.000Z", TurnIndex: 3, TaskDir: ".sunshine/tasks/11-09-demo/"},
	}

	// brainstorm 取窗口 [1,3) 内的轮次。
	got, windows, warnings := SliceMemPhase(turns, events, PhaseBrainstorm, MemSourceClaude)
	if len(warnings) != 0 {
		t.Errorf("有边界时 brainstorm 不应有警告，实际：%v", warnings)
	}
	if len(windows) != 1 || windows[0].Label != "demo" || windows[0].StartTurn != 1 || windows[0].EndTurn != 3 {
		t.Errorf("窗口应为 [1,3)/demo，实际：%v", windows)
	}
	if len(got) != 2 || got[0].Text != "方案草稿" || got[1].Text != "方案定了，开始动手" {
		t.Errorf("brainstorm 应只返回窗口内 2 轮，实际：%v", got)
	}

	// implement 取窗口外的轮次。
	got, windows, warnings = SliceMemPhase(turns, events, PhaseImplement, MemSourceClaude)
	if len(warnings) != 0 {
		t.Errorf("有边界时 implement 不应有警告，实际：%v", warnings)
	}
	if len(got) != 3 || got[0].Text != "先想想做什么" || got[2].Text != "继续实现功能" {
		t.Errorf("implement 应只返回窗口外 3 轮，实际：%v", got)
	}

	// all 返回全文、空窗口、无警告。
	got, windows, warnings = SliceMemPhase(turns, events, PhaseAll, MemSourceClaude)
	if len(got) != 5 || len(windows) != 0 || len(warnings) != 0 {
		t.Errorf("all 应返回全文且无窗口无警告，实际 %d 轮、%v、%v", len(got), windows, warnings)
	}
}

func TestSliceMemPhase_NoBoundary(t *testing.T) {
	turns := []DialogueTurn{
		{Role: RoleUser, Text: "没有任务命令的会话"},
		{Role: RoleAssistant, Text: "纯聊天"},
	}
	// 无边界时 brainstorm 返回全文 + 警告。
	got, _, warnings := SliceMemPhase(turns, nil, PhaseBrainstorm, MemSourceClaude)
	if len(got) != 2 {
		t.Errorf("无边界时 brainstorm 应返回全文，实际 %d 轮", len(got))
	}
	if len(warnings) != 1 || !strings.Contains(warnings[0], "未找到") {
		t.Errorf("无边界时 brainstorm 应附一条无边界警告，实际：%v", warnings)
	}
	// 无边界时 implement 返回空 + 警告。
	got, _, warnings = SliceMemPhase(turns, nil, PhaseImplement, MemSourceClaude)
	if len(got) != 0 {
		t.Errorf("无边界时 implement 应为空，实际 %d 轮", len(got))
	}
	if len(warnings) != 1 || !strings.Contains(warnings[0], "implement") {
		t.Errorf("无边界时 implement 应附一条无边界警告，实际：%v", warnings)
	}
}

func TestSliceMemPhase_OpencodeDowngrade(t *testing.T) {
	turns := []DialogueTurn{
		{Role: RoleUser, Text: "第一轮"},
		{Role: RoleAssistant, Text: "第二轮"},
	}
	// OpenCode 不支持阶段切片：即使给了事件也返回全文 + 降级警告。
	events := []MemEvent{
		{Action: "create", TurnIndex: 0, Slug: "demo"},
		{Action: "start", TurnIndex: 1, TaskDir: ".sunshine/tasks/11-09-demo/"},
	}
	for _, phase := range []MemPhase{PhaseBrainstorm, PhaseImplement} {
		got, windows, warnings := SliceMemPhase(turns, events, phase, MemSourceOpencode)
		if len(got) != 2 {
			t.Errorf("OpenCode 的 --phase %s 应降级返回全文，实际 %d 轮", phase, len(got))
		}
		if len(windows) != 0 {
			t.Errorf("OpenCode 降级时窗口应为空，实际：%v", windows)
		}
		if len(warnings) != 1 || !strings.Contains(warnings[0], "暂不支持") {
			t.Errorf("OpenCode 的 --phase %s 应附一条暂不支持警告，实际：%v", phase, warnings)
		}
	}
	if got := PhaseSupported(MemSourceOpencode); got {
		t.Error("OpenCode 的 PhaseSupported 应为 false")
	}
	for _, kind := range []MemSourceKind{MemSourceClaude, MemSourceCodex, MemSourceDevin, MemSourceGrok, MemSourcePi, MemSourceZcode} {
		if !PhaseSupported(kind) {
			t.Errorf("平台 %s 的 PhaseSupported 应为 true", kind)
		}
	}
}

func TestEventsFromParsed(t *testing.T) {
	cmds := ParseTaskCommandsAll("sunshine task create --slug a; sunshine task start .sunshine/tasks/11-09-a/")
	events := EventsFromParsed(cmds, "2026-11-09T10:00:00.000Z", 2)
	if len(events) != 2 {
		t.Fatalf("应转出 2 个事件，实际 %d 个", len(events))
	}
	if events[0].Action != "create" || events[0].Slug != "a" || events[0].Timestamp != "2026-11-09T10:00:00.000Z" || events[0].TurnIndex != 2 {
		t.Errorf("第 1 个事件不对：%+v", events[0])
	}
	if events[1].Action != "start" || events[1].TaskDir != ".sunshine/tasks/11-09-a/" || events[1].TurnIndex != 2 {
		t.Errorf("第 2 个事件不对：%+v", events[1])
	}
	if got := EventsFromParsed(nil, "", 0); len(got) != 0 {
		t.Errorf("无命令时应返回空，实际：%v", got)
	}
}
