// Native 子代理活动事件落盘：.sunshine/.runtime/activity.jsonl。
//
// v0.5.0 新增：serve 的智能体团与活动热力格在 native 模式下全空，
// 因为两者只认 channel 事件/worker；native 子代理不走 channel。
// hook 在子代理启停时往这里写事件，serve 轮询读它推导运行状态。
//
// 铁律：写盘失败一律静默丢弃，绝不阻断 agent 会话；
// HooksDisabled() 时不写；无 .sunshine/ 时不写（调用方保证 root 有效）。
package hook

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// 活动事件 kind：本批只写启停两种；字段名与 kind 集合预留扩展
// （任务生命周期事件以后再加）。
const (
	ActivityKindStarted = "subagent_started"
	ActivityKindStopped = "subagent_stopped"
	ActivitySourceHook  = "hook"
)

// MaxActivityLines 是 activity.jsonl 的保留上限：写盘时顺手截断，
// 只留最近这么多行，防止只增不减膨胀。
const MaxActivityLines = 1000

// ActivityEvent 是 activity.jsonl 里的一行。Agent 用短名
// （implement/check/research/prototype/main，不带 sunshine- 前缀），
// 与 serve 前端 clusterRoles/heatEvents 的角色键一致。
type ActivityEvent struct {
	TS        string `json:"ts"`
	Kind      string `json:"kind"`
	Agent     string `json:"agent"`
	Task      string `json:"task"`
	SessionID string `json:"session_id,omitempty"`
	Source    string `json:"source"`
}

// ActivityFilePath 返回项目根目录下的活动事件文件路径。
func ActivityFilePath(root string) string {
	return filepath.Join(root, ".sunshine", ".runtime", "activity.jsonl")
}

// AppendActivityEvent 追加一行事件；失败静默丢弃。写盘后若行数
// 超过 MaxActivityLines，截断只留最近 MaxActivityLines 行。
func AppendActivityEvent(root string, ev ActivityEvent) {
	if root == "" {
		return
	}
	if ev.TS == "" {
		ev.TS = time.Now().Format(time.RFC3339)
	}
	if ev.Source == "" {
		ev.Source = ActivitySourceHook
	}
	path := ActivityFilePath(root)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return
	}
	line, err := json.Marshal(ev)
	if err != nil {
		return
	}
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return
	}
	_, err = f.Write(append(line, '\n'))
	_ = f.Close()
	if err != nil {
		return
	}
	truncateActivityFile(path)
}

// truncateActivityFile 在行数超限时重写文件只留最近 MaxActivityLines 行；
// 任何失败都静默（文件多长一点，下次写盘再试）。
func truncateActivityFile(path string) {
	data, err := os.ReadFile(path)
	if err != nil {
		return
	}
	lines := strings.Split(strings.TrimRight(string(data), "\n"), "\n")
	if len(lines) <= MaxActivityLines {
		return
	}
	keep := lines[len(lines)-MaxActivityLines:]
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, []byte(strings.Join(keep, "\n")+"\n"), 0o644); err != nil {
		return
	}
	_ = os.Rename(tmp, path)
}

// ReadActivityEvents 读活动事件文件（不存在返回空切片，不报错）。
// maxLines > 0 时只返回最近 maxLines 行；解析失败的行跳过。
func ReadActivityEvents(root string, maxLines int) ([]ActivityEvent, error) {
	if root == "" {
		return nil, nil
	}
	f, err := os.Open(ActivityFilePath(root))
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	defer f.Close()
	var lines []string
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for sc.Scan() {
		lines = append(lines, sc.Text())
	}
	if maxLines > 0 && len(lines) > maxLines {
		lines = lines[len(lines)-maxLines:]
	}
	out := make([]ActivityEvent, 0, len(lines))
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		var ev ActivityEvent
		if err := json.Unmarshal([]byte(line), &ev); err != nil {
			continue
		}
		if ev.Kind == "" || ev.Agent == "" {
			continue
		}
		out = append(out, ev)
	}
	return out, nil
}

// UnendedStart 返回 agent 最近一个未被 stop 配对的 start 事件；
// task 非空时只考虑该任务的 start。配对规则：同一 agent、文件顺序
// 在 start 之后、且 stop.task 为空或与 start.task 相同。
// 找不到返回 nil。事件按文件顺序（追加顺序）比较，不依赖 ts。
func UnendedStart(events []ActivityEvent, agent, task string) *ActivityEvent {
	for i := len(events) - 1; i >= 0; i-- {
		ev := events[i]
		if ev.Kind != ActivityKindStarted || ev.Agent != agent {
			continue
		}
		if task != "" && ev.Task != task {
			continue
		}
		ended := false
		for j := i + 1; j < len(events); j++ {
			st := events[j]
			if st.Kind == ActivityKindStopped && st.Agent == agent &&
				(st.Task == "" || st.Task == ev.Task) {
				ended = true
				break
			}
		}
		if !ended {
			cp := events[i]
			return &cp
		}
		// 最新一个 start 已结束，继续往前找更早的未结束 start
		// （同一角色可能先后跑过两个任务）。
	}
	return nil
}

// RecordSubagentStart 记一条子代理启动事件（agent 短名；task 归一化为目录名）。
func RecordSubagentStart(root, agent, task, sessionID string) {
	AppendActivityEvent(root, ActivityEvent{
		Kind:      ActivityKindStarted,
		Agent:     agent,
		Task:      taskDirName(task),
		SessionID: sessionID,
	})
}

// RecordSubagentStop 记一条子代理结束事件（agent 短名；task 归一化为目录名）。
func RecordSubagentStop(root, agent, task, sessionID string) {
	AppendActivityEvent(root, ActivityEvent{
		Kind:      ActivityKindStopped,
		Agent:     agent,
		Task:      taskDirName(task),
		SessionID: sessionID,
	})
}
