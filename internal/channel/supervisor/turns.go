package supervisor

import "strconv"

// ActiveTurn 是一回合的跟踪记录：输入消息的 seq 与回合 id。
type ActiveTurn struct {
	InputSeq int64
	TurnID   string
}

// TurnOutcome 是回合的结束方式。
type TurnOutcome string

const (
	// TurnOutcomeDone 回合正常完成。
	TurnOutcomeDone TurnOutcome = "done"
	// TurnOutcomeError 回合出错。
	TurnOutcomeError TurnOutcome = "error"
	// TurnOutcomeAborted 回合被中断。
	TurnOutcomeAborted TurnOutcome = "aborted"
)

// TurnTrackerHooks 是 idle ↔ mid-turn 切换时的回调，
// 供 supervisor 空闲定时器暂停/重置（与上游 turns.ts 一致）。
type TurnTrackerHooks struct {
	// OnIdleExit 在从空闲进入回合时调用。
	OnIdleExit func()
	// OnIdleEnter 在回到空闲时调用。
	OnIdleEnter func()
}

// TurnTracker 是单个 supervisor 进程内的回合跟踪器。
// 持久的事实来源是 events.jsonl；本对象只记住输入消息的 seq，
// 供收件箱监听与 stdout 泵配对发出 turn_started / turn_finished 事件。
type TurnTracker struct {
	turns []ActiveTurn
	hooks TurnTrackerHooks
}

// NewTurnTracker 建一个回合跟踪器。
func NewTurnTracker(hooks TurnTrackerHooks) *TurnTracker {
	return &TurnTracker{hooks: hooks}
}

// Begin 开始一回合，回合 id 为 msg:<输入 seq>。
func (t *TurnTracker) Begin(inputSeq int64) ActiveTurn {
	wasIdle := len(t.turns) == 0
	turn := ActiveTurn{InputSeq: inputSeq, TurnID: "msg:" + strconv.FormatInt(inputSeq, 10)}
	t.turns = append(t.turns, turn)
	if wasIdle && t.hooks.OnIdleExit != nil {
		t.hooks.OnIdleExit()
	}
	return turn
}

// Finish 结束最近一回合。
func (t *TurnTracker) Finish() (ActiveTurn, bool) {
	if len(t.turns) == 0 {
		return ActiveTurn{}, false
	}
	turn := t.turns[len(t.turns)-1]
	t.turns = t.turns[:len(t.turns)-1]
	if len(t.turns) == 0 && t.hooks.OnIdleEnter != nil {
		t.hooks.OnIdleEnter()
	}
	return turn, true
}

// AbortCurrent 中断当前回合（与 Finish 相同语义，供中断路径调用）。
func (t *TurnTracker) AbortCurrent() (ActiveTurn, bool) {
	return t.Finish()
}

// Current 返回当前回合（无回合时返回 false）。
func (t *TurnTracker) Current() (ActiveTurn, bool) {
	if len(t.turns) == 0 {
		return ActiveTurn{}, false
	}
	return t.turns[len(t.turns)-1], true
}
