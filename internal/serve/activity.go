package serve

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/BondLam/Sunshine/internal/hook"
	"github.com/BondLam/Sunshine/internal/task"
)

// activityEntry 是 /api/activity 的一条条目。
type activityEntry struct {
	TS     string `json:"ts"`
	Kind   string `json:"kind"`
	Text   string `json:"text"`
	Source string `json:"source"`
	// sortTime 只用于服务端排序，不输出。
	sortTime time.Time `json:"-"`
}

// watchedDesignFiles 是 mtime 活动流盯着的设计文档（相对项目根）。
// 原型按页分文件：doc/prototype/ 下各页 .html 在 watchedFiles() 里展开盯梢。
var watchedDesignFiles = []string{
	"doc/design/需求列表.md",
	"doc/design/概要设计.md",
}

// activityTracker 维护关键文件 mtime 快照：启动时记基线，每次
// /api/activity 调用时重新 Stat，比对出变化并记进环形缓冲（最近 100 条）。
// 只含 serve 启动后检测到的变化；不写盘。
type activityTracker struct {
	root string

	mu        sync.Mutex
	snapshot  map[string]time.Time // 相对路径 → mtime
	knownDirs map[string]bool      // 已见过的任务目录名
	entries   []activityEntry
}

func newActivityTracker(root string) *activityTracker {
	t := &activityTracker{
		root:      root,
		snapshot:  map[string]time.Time{},
		knownDirs: map[string]bool{},
	}
	t.scanLocked(true)
	return t
}

// watchedFiles 列出当前要盯的关键文件（相对路径 → 绝对路径）。
func (t *activityTracker) watchedFiles() map[string]string {
	out := map[string]string{}
	tasksDir := task.TasksDir(t.root)
	entries, _ := os.ReadDir(tasksDir)
	for _, e := range entries {
		if !e.IsDir() || e.Name() == task.DirArchive {
			continue
		}
		dir := filepath.Join(tasksDir, e.Name())
		for _, name := range []string{"task.json", "prd.md", "implement.jsonl", "check.jsonl"} {
			rel, _ := filepath.Rel(t.root, filepath.Join(dir, name))
			out[filepath.ToSlash(rel)] = filepath.Join(dir, name)
		}
	}
	for _, rel := range watchedDesignFiles {
		out[rel] = filepath.Join(t.root, filepath.FromSlash(rel))
	}
	// 原型按页分文件：展开 doc/prototype/ 下各页 .html（新增/删除页也被捕获；
	// 只盯目录 mtime 会漏掉页内编辑，故必须展开到文件）。
	if protoEntries, err := os.ReadDir(filepath.Join(t.root, "doc", "prototype")); err == nil {
		for _, e := range protoEntries {
			if e.IsDir() || !strings.HasSuffix(e.Name(), ".html") {
				continue
			}
			rel := "doc/prototype/" + e.Name()
			out[rel] = filepath.Join(t.root, "doc", "prototype", e.Name())
		}
	}
	return out
}

// scanLocked 比对一次 mtime。baseline 为真时只记基线、不产生条目。
func (t *activityTracker) scanLocked(baseline bool) {
	now := time.Now()
	files := t.watchedFiles()

	// 新任务目录出现记一条。
	tasksDir := task.TasksDir(t.root)
	if entries, err := os.ReadDir(tasksDir); err == nil {
		for _, e := range entries {
			if !e.IsDir() || e.Name() == task.DirArchive {
				continue
			}
			if !t.knownDirs[e.Name()] {
				t.knownDirs[e.Name()] = true
				if !baseline {
					t.appendLocked(activityEntry{
						TS:       now.Format(time.RFC3339),
						Kind:     "file_change",
						Text:     ".sunshine/tasks/" + e.Name() + " 已创建",
						Source:   "mtime",
						sortTime: now,
					})
				}
			}
		}
	}

	for rel, abs := range files {
		st, err := os.Stat(abs)
		if err != nil {
			// 文件被删：静默移出快照，不记条目。
			delete(t.snapshot, rel)
			continue
		}
		mtime := st.ModTime()
		prev, ok := t.snapshot[rel]
		t.snapshot[rel] = mtime
		if baseline {
			continue
		}
		if !ok {
			t.appendLocked(activityEntry{
				TS:       now.Format(time.RFC3339),
				Kind:     "file_change",
				Text:     rel + " 已创建",
				Source:   "mtime",
				sortTime: now,
			})
		} else if !mtime.Equal(prev) {
			t.appendLocked(activityEntry{
				TS:       now.Format(time.RFC3339),
				Kind:     "file_change",
				Text:     rel + " 已更新",
				Source:   "mtime",
				sortTime: now,
			})
		}
	}
}

// appendLocked 追加一条 mtime 条目，环形保留最近 100 条。
func (t *activityTracker) appendLocked(e activityEntry) {
	t.entries = append(t.entries, e)
	if len(t.entries) > 100 {
		t.entries = t.entries[len(t.entries)-100:]
	}
}

// scan 比对一次并返回当前全部 mtime 条目（副本）。
func (t *activityTracker) scan() []activityEntry {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.scanLocked(false)
	out := make([]activityEntry, len(t.entries))
	copy(out, t.entries)
	return out
}

// mergeActivityEntries 把 mtime 条目、channel 事件条目与 native hook
// 事件条目合并，按 ts 倒序取最近 30 条。
func mergeActivityEntries(mtimeEntries []activityEntry, channelEntries []activityEntry, nativeEntries []activityEntry) []activityEntry {
	all := make([]activityEntry, 0, len(mtimeEntries)+len(channelEntries)+len(nativeEntries))
	all = append(all, mtimeEntries...)
	all = append(all, channelEntries...)
	all = append(all, nativeEntries...)
	sort.SliceStable(all, func(i, j int) bool {
		return all[i].sortTime.After(all[j].sortTime)
	})
	if len(all) > 30 {
		all = all[:30]
	}
	return all
}

// ---------------------------------------------------------------------------
// Native hook 活动（v0.5.0）
// ---------------------------------------------------------------------------

// NativeRunningStaleAfter 是运行中推导的防 stale 阈值：start 超过
// 这么久仍无对应 stop，视为未知（stop hook 未触发，如会话被杀、
// 旧版二进制），回落待命，不无上限显示运行中。
// 取值说明：实测 demo 里 implement 一轮约 2 分钟；research 也可能跑
// 几十分钟；2 小时是留足余量的初值，待真实使用后按实测再调。
const NativeRunningStaleAfter = 2 * time.Hour

// NativeActivityMaxRead 是 serve 一次读 activity.jsonl 的最多行数
// （hook 写盘侧上限 1000 行，读 500 行足够覆盖推导与 feed）。
const NativeActivityMaxRead = 500

// nativeAgentRoles 是活动事件里的角色顺序（与前端 clusterRoles 一致）。
// 主会话不在事件里，本批不推导主会话。
var nativeAgentRoles = []string{"implement", "check", "research", "prototype"}

// loadNativeEvents 读 .sunshine/.runtime/activity.jsonl；
// 文件不存在或读失败返回空（不是错误：旧项目与未派发时本就没有）。
func loadNativeEvents(root string) []hook.ActivityEvent {
	events, err := hook.ReadActivityEvents(root, NativeActivityMaxRead)
	if err != nil {
		return nil
	}
	return events
}

// nativeEntries 把 native 事件转成活动流条目（source="native"），
// 文本写明角色与启停，如 `implement 已启动`。
func nativeEntries(events []hook.ActivityEvent) []activityEntry {
	out := make([]activityEntry, 0, len(events))
	for _, ev := range events {
		text := ev.Agent + " 已启动"
		if ev.Kind == hook.ActivityKindStopped {
			text = ev.Agent + " 已结束"
		}
		out = append(out, activityEntry{
			TS:       ev.TS,
			Kind:     ev.Kind,
			Text:     text,
			Source:   "native",
			sortTime: parseEventTime(ev.TS),
		})
	}
	return out
}

// deriveNativeRunning 从 native 事件推导当前运行中的角色：
// 最近一个 start 未被 stop 配对、且 start 未超过 stale 阈值。
// 无 start 事件绝不显示运行中；stale start 回落待命。
func deriveNativeRunning(events []hook.ActivityEvent, now time.Time) []string {
	running := []string{}
	for _, role := range nativeAgentRoles {
		st := hook.UnendedStart(events, role, "")
		if st == nil {
			continue
		}
		ts := parseEventTime(st.TS)
		if ts.IsZero() || now.Sub(ts) > NativeRunningStaleAfter {
			continue
		}
		running = append(running, role)
	}
	return running
}

// parseEventTime 解析事件 ts（RFC3339）；解析失败返回零值（排最后）。
func parseEventTime(ts string) time.Time {
	if ts == "" {
		return time.Time{}
	}
	if parsed, err := time.Parse(time.RFC3339Nano, ts); err == nil {
		return parsed
	}
	if parsed, err := time.Parse(time.RFC3339, ts); err == nil {
		return parsed
	}
	// 兼容 "2006-01-02 15:04:05" 等本地写法。
	if parsed, err := time.ParseInLocation("2006-01-02 15:04:05", ts, time.Local); err == nil {
		return parsed
	}
	return time.Time{}
}

// relPath 返回相对项目根的斜杠路径（超出时返回原路径）。
func (s *Server) relPath(abs string) string {
	if rel, err := filepath.Rel(s.root, abs); err == nil && !strings.HasPrefix(rel, "..") {
		return filepath.ToSlash(rel)
	}
	return abs
}
