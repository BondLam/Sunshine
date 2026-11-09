package supervisor

import (
	"context"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/linrong1982/Sunshine/internal/channel/adapters"
	"github.com/linrong1982/Sunshine/internal/channel/store"
)

// runInboxWatcher 是收件箱监听：轮询 events.jsonl，
// 把发给本 worker 的消息与中断请求转发进 worker stdin。
// 持久化的游标文件让重派生（同一 worker 名）不重放上次
// supervisor 已投递的事件。
//
// 步骤（与上游 supervisor/inbox.ts 一致）：
//   - 首次派生游标为 0，从头读积压消息；重派生从上次投递到的 seq 继续；
//   - message 事件按 inbox 策略过滤（explicitOnly 只消费 to 指向自己的，
//     broadcastAndExplicit 另消费无 to 的广播）；worker 永不消费自己发的消息；
//   - interrupt_requested 事件按 worker 字段过滤；
//   - 非中断消息先等当前回合结束再投递；中断直接打断当前回合；
//   - 投递前追加 turn_started，投递后把游标推进到该事件 seq。
func runInboxWatcher(ctx context.Context, ref store.ChannelRef, worker string, policy store.InboxPolicy, runtime adapters.WorkerRuntime, tracker *TurnTracker, shutdown *ShutdownController) {
	cursor := readInboxCursor(ref, worker)
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
		if shutdown.IsShuttingDown() {
			return
		}
		after := cursor
		limit := 200
		events, err := store.ReadChannelEvents(ref, &store.ReadChannelEventsPagination{
			AfterSeq: &after,
			Limit:    &limit,
		})
		if err != nil {
			continue
		}
		for _, ev := range events {
			select {
			case <-ctx.Done():
				return
			default:
			}
			if shutdown.IsShuttingDown() {
				return
			}
			delivered, done := handleInboxEvent(ctx, ref, worker, policy, runtime, tracker, ev)
			if done {
				return
			}
			if delivered {
				cursor = ev.Seq
				writeInboxCursor(ref, worker, cursor)
			}
		}
	}
}

// handleInboxEvent 处理一条收件箱事件：
// 返回（是否已投递并应推进游标，是否应退出监听）。
func handleInboxEvent(ctx context.Context, ref store.ChannelRef, worker string, policy store.InboxPolicy, runtime adapters.WorkerRuntime, tracker *TurnTracker, ev store.ChannelEvent) (bool, bool) {
	isInterrupt := ev.Kind == store.KindInterruptRequested
	if ev.Kind == store.KindMessage {
		if !store.MatchesInboxPolicy(ev, worker, policy) {
			return false, false
		}
	} else if isInterrupt {
		if ev.StrField("worker") != worker {
			return false, false
		}
	} else {
		return false, false
	}

	text := strings.TrimSpace(ev.StrField("text"))
	interruptText := strings.TrimSpace(ev.StrField("message"))
	if text == "" && (!isInterrupt || interruptText == "") {
		return false, false
	}

	if !isInterrupt {
		// 等当前回合结束再投递下一条（避免两条消息写进同一回合）。
		for {
			if _, ok := tracker.Current(); !ok {
				break
			}
			select {
			case <-ctx.Done():
				return false, true
			case <-time.After(25 * time.Millisecond):
			}
		}
	}

	if isInterrupt {
		aborted, hadTurn := tracker.AbortCurrent()
		if hadTurn {
			_, _ = appendEvent(ref, store.KindTurnFinished, worker, map[string]any{
				"worker":   worker,
				"inputSeq": aborted.InputSeq,
				"turnId":   aborted.TurnID,
				"outcome":  string(TurnOutcomeAborted),
			})
		}
		interruptedExtra := map[string]any{
			"worker":  worker,
			"reason":  "user",
			"method":  "stdin",
			"outcome": "no-active-turn",
		}
		if hadTurn {
			interruptedExtra["turnId"] = aborted.TurnID
			interruptedExtra["outcome"] = "interrupted"
		}
		_, _ = appendEvent(ref, store.KindInterrupted, worker, interruptedExtra)
	}
	turn := tracker.Begin(ev.Seq)
	_, _ = appendEvent(ref, store.KindTurnStarted, worker, map[string]any{
		"worker":   worker,
		"inputSeq": ev.Seq,
		"turnId":   turn.TurnID,
	})
	var err error
	if isInterrupt {
		err = runtime.Interrupt(ctx, interruptText)
	} else {
		err = runtime.Send(ctx, text)
	}
	if err != nil {
		// 投递失败：收尾该回合，避免下一条消息永远等它。
		tracker.Finish()
		_, _ = appendEvent(ref, store.KindTurnFinished, worker, map[string]any{
			"worker":   worker,
			"inputSeq": turn.InputSeq,
			"turnId":   turn.TurnID,
			"outcome":  string(TurnOutcomeAborted),
		})
		if _, _, exited := runtime.ExitState(); exited {
			// worker 已退出（stdin 已关）：退出监听。
			return false, true
		}
		// worker 还活着但本次投递失败（如 60 秒未就绪）：
		// 推进游标、不重放这条消息（与上游一致），继续监听。
		return true, false
	}
	return true, false
}

// readInboxCursor 读持久化的收件箱游标；不存在或非法时返回 0。
func readInboxCursor(ref store.ChannelRef, worker string) int64 {
	p, err := store.WorkerFile(ref.Name, worker, "inbox-cursor", ref.Project)
	if err != nil {
		return 0
	}
	data, err := os.ReadFile(p)
	if err != nil {
		return 0
	}
	n, err := strconv.ParseInt(strings.TrimSpace(string(data)), 10, 64)
	if err != nil || n <= 0 {
		return 0
	}
	return n
}

// writeInboxCursor 写持久化的收件箱游标（尽力而为；失败时最坏重放一条消息）。
func writeInboxCursor(ref store.ChannelRef, worker string, cursor int64) {
	p, err := store.WorkerFile(ref.Name, worker, "inbox-cursor", ref.Project)
	if err != nil {
		return
	}
	_ = os.WriteFile(p, []byte(strconv.FormatInt(cursor, 10)), 0o644)
}
