package serve

import (
	"encoding/json"
	"net/http"
	"os"
	"sort"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/linrong007/Sunshine/internal/channel"
	"github.com/linrong007/Sunshine/internal/channel/store"
)

// ---------------------------------------------------------------------------
// Channel 列表
// ---------------------------------------------------------------------------

func (s *Server) handleChannels(c *gin.Context) {
	rows := []map[string]any{}
	for _, name := range store.ListChannelNamesInProject(s.projectKey) {
		ref := store.ChannelRef{Name: name, Project: s.projectKey}
		events, err := store.ReadChannelEvents(ref, nil)
		if err != nil || len(events) == 0 {
			continue
		}
		first, last := events[0], events[len(events)-1]
		metadata := store.ReduceChannelMetadata(events)
		registry := store.ReduceWorkerRegistry(events, &ref)
		alive, total := 0, 0
		for _, w := range registry.Workers {
			total++
			if !w.Terminal {
				alive++
			}
		}
		desc := metadata.Title
		if desc == "" {
			desc = metadata.Description
		}
		rows = append(rows, map[string]any{
			"name":          name,
			"project":       s.projectKey,
			"createdAt":     first.Ts,
			"task":          first.StrField("task"),
			"type":          string(metadata.Type),
			"description":   desc,
			"workersAlive":  alive,
			"workersTotal":  total,
			"lastEventTs":   last.Ts,
			"lastEventKind": string(last.Kind),
			"totalEvents":   len(events),
			"ephemeral":     first.BoolField("ephemeral"),
		})
	}
	sort.Slice(rows, func(i, j int) bool {
		return rows[i]["lastEventTs"].(string) > rows[j]["lastEventTs"].(string)
	})
	c.JSON(http.StatusOK, rows)
}

// ---------------------------------------------------------------------------
// Channel 事件流
// ---------------------------------------------------------------------------

// eventDTO 把 ChannelEvent 转成 API 事件对象：seq/ts/kind/by/to/thread
// + Extra 原样 + 原始 JSON 字符串 raw。时间一律取事件的 Ts 字段，
// 绝不调 StrField("ts")（ts 不在 Extra 里，恒为空）。
func eventDTO(ev store.ChannelEvent) map[string]any {
	raw, _ := json.Marshal(ev)
	extra := map[string]any{}
	for k, v := range ev.Extra {
		extra[k] = v
	}
	text := ""
	if v, ok := ev.Extra["text"].(string); ok {
		text = v
	} else if v, ok := ev.Extra["message"].(string); ok {
		text = v
	}
	dto := map[string]any{
		"seq":     ev.Seq,
		"ts":      ev.Ts,
		"kind":    string(ev.Kind),
		"by":      ev.By,
		"from":    ev.By,
		"author":  ev.By,
		"to":      strSlice(ev.To),
		"thread":  ev.StrField("thread"),
		"text":    text,
		"summary": text,
		"extra":   extra,
		"raw":     string(raw),
	}
	if len(ev.To) > 0 {
		dto["target"] = ev.To[0]
	}
	if v, ok := ev.Extra["detail"]; ok {
		dto["detail"] = v
	}
	return dto
}

func (s *Server) handleChannelEvents(c *gin.Context) {
	name := c.Param("name")
	if _, err := s.readChannelEvents(name); err != nil {
		if err == errChannelNotFound {
			errorJSON(c, http.StatusNotFound, "channel %q 不存在", name)
			return
		}
		errorJSON(c, http.StatusBadRequest, "读取 channel %q 失败：%v", name, err)
		return
	}

	kindParam := strings.TrimSpace(c.Query("kind"))
	fromParam := strings.TrimSpace(c.Query("from"))
	sinceParam := strings.TrimSpace(c.Query("since"))
	lastParam := strings.TrimSpace(c.Query("last"))

	kinds, err := store.ParseChannelKinds(kindParam)
	if err != nil {
		errorJSON(c, http.StatusBadRequest, "kind 参数错误：%v", err)
		return
	}
	var from []string
	if fromParam != "" {
		for _, f := range strings.Split(fromParam, ",") {
			if f = strings.TrimSpace(f); f != "" {
				from = append(from, f)
			}
		}
	}
	var since *int64
	if sinceParam != "" {
		n, err := parseInt64(sinceParam)
		if err != nil {
			errorJSON(c, http.StatusBadRequest, "since 参数错误：应为整数 seq")
			return
		}
		since = &n
	}
	last := 0
	if lastParam != "" {
		n, err := parseInt64(lastParam)
		if err != nil || n < 0 {
			errorJSON(c, http.StatusBadRequest, "last 参数错误：应为非负整数")
			return
		}
		last = int(n)
	}

	// 无 kind/from 过滤时用 AfterSeq+Limit 分页读（默认页 200）；
	// 有过滤时全量读入再按 MatchesEventFilter 过滤（progress 事件
	// 必须保留：显式置 IncludeProgress 与 IncludeNonMeaningful）。
	ref, _ := s.channelRef(name)
	var events []store.ChannelEvent
	if len(kinds) == 0 && len(from) == 0 && since != nil {
		limit := 200
		if last > 0 {
			limit = last
		}
		events, err = store.ReadChannelEvents(ref, &store.ReadChannelEventsPagination{
			AfterSeq: since, Limit: &limit,
		})
		if err != nil {
			errorJSON(c, http.StatusInternalServerError, "读取事件失败：%v", err)
			return
		}
	} else {
		all, err := store.ReadChannelEvents(ref, nil)
		if err != nil {
			errorJSON(c, http.StatusInternalServerError, "读取事件失败：%v", err)
			return
		}
		filter := store.ChannelEventFilter{
			Kind:                 kinds,
			From:                 from,
			IncludeProgress:      true,
			IncludeNonMeaningful: true,
		}
		for _, ev := range all {
			if !store.MatchesEventFilter(ev, filter) {
				continue
			}
			if since != nil && ev.Seq <= *since {
				continue
			}
			events = append(events, ev)
		}
		if last > 0 && len(events) > last {
			events = events[len(events)-last:]
		}
	}

	rows := make([]map[string]any, 0, len(events))
	for _, ev := range events {
		rows = append(rows, eventDTO(ev))
	}
	c.JSON(http.StatusOK, rows)
}

func parseInt64(raw string) (int64, error) {
	return strconv.ParseInt(strings.TrimSpace(raw), 10, 64)
}

// ---------------------------------------------------------------------------
// Worker 状态与日志
// ---------------------------------------------------------------------------

func (s *Server) handleChannelWorkers(c *gin.Context) {
	name := c.Param("name")
	events, err := s.readChannelEvents(name)
	if err != nil {
		if err == errChannelNotFound {
			errorJSON(c, http.StatusNotFound, "channel %q 不存在", name)
			return
		}
		errorJSON(c, http.StatusBadRequest, "读取 channel %q 失败：%v", name, err)
		return
	}
	ref, _ := s.channelRef(name)
	registry := store.ReduceWorkerRegistry(events, &ref)

	// pid 探活（本机）：读 <worker>.pid / <worker>.worker-pid。
	obsByID := map[string]channel.WorkerRuntimeObservation{}
	if obs, err := channel.ProbeWorkerRuntime(channel.ChannelAddress{
		Channel: name, ProjectKey: s.projectKey,
	}); err == nil {
		for _, o := range obs {
			obsByID[o.WorkerID] = o
		}
	}

	rows := []map[string]any{}
	for _, w := range registry.Workers {
		var supervisorAlive, workerAlive any
		if o, ok := obsByID[w.WorkerID]; ok {
			supervisorAlive = o.SupervisorAlive
			workerAlive = o.WorkerAlive
		}
		row := map[string]any{
			"workerId":            w.WorkerID,
			"name":                w.WorkerID,
			"id":                  w.WorkerID,
			"agent":               w.Agent,
			"provider":            w.Provider,
			"lifecycle":           string(w.Lifecycle),
			"Lifecycle":           string(w.Lifecycle),
			"terminal":            w.Terminal,
			"activity":            string(w.Activity),
			"Activity":            string(w.Activity),
			"activeTurnId":        w.ActiveTurnID,
			"activeTurnStartedAt": w.ActiveTurnStartedAt,
			"pendingMessageCount": w.PendingMessageCount,
			"inboxPolicy":         string(w.InboxPolicy),
			"spawnedAt":           w.SpawnedAt,
			"updatedAt":           w.UpdatedAt,
			"startedBy":           w.StartedBy,
			"exitCode":            w.ExitCode,
			"ExitCode":            w.ExitCode,
			"signal":              w.Signal,
			"reason":              w.Reason,
			"error":               w.Error,
			"idleSince":           w.IdleSince,
			"lastSeq":             w.LastSeq,
			"supervisorAlive":     supervisorAlive,
			"workerAlive":         workerAlive,
		}
		rows = append(rows, row)
	}
	c.JSON(http.StatusOK, rows)
}

func (s *Server) handleWorkerLog(c *gin.Context) {
	name := c.Param("name")
	worker := c.Param("worker")
	lines := parseIntQuery(c, "lines", 100)
	if lines < 1 {
		lines = 1
	}
	if lines > 1000 {
		lines = 1000
	}

	events, err := s.readChannelEvents(name)
	if err != nil {
		if err == errChannelNotFound {
			errorJSON(c, http.StatusNotFound, "channel %q 不存在", name)
			return
		}
		errorJSON(c, http.StatusBadRequest, "读取 channel %q 失败：%v", name, err)
		return
	}
	ref, _ := s.channelRef(name)
	registry := store.ReduceWorkerRegistry(events, &ref)
	if registry.FindWorker(worker) == nil {
		errorJSON(c, http.StatusNotFound, "channel %q 下没有 worker %q", name, worker)
		return
	}
	path, err := store.WorkerFile(name, worker, "log", s.projectKey)
	if err != nil {
		errorJSON(c, http.StatusBadRequest, "读取日志失败：%v", err)
		return
	}
	data, err := os.ReadFile(path)
	if err != nil {
		// 日志文件尚未生成：返回空数组。
		c.JSON(http.StatusOK, map[string]any{"lines": []string{}})
		return
	}
	all := strings.Split(string(data), "\n")
	if len(all) > 0 && all[len(all)-1] == "" {
		all = all[:len(all)-1]
	}
	if len(all) > lines {
		all = all[len(all)-lines:]
	}
	if all == nil {
		all = []string{}
	}
	c.JSON(http.StatusOK, map[string]any{"lines": all})
}

// ---------------------------------------------------------------------------
// Forum 看板
// ---------------------------------------------------------------------------

func threadDTO(t store.ThreadState) map[string]any {
	return map[string]any{
		"id":          t.Thread,
		"thread":      t.Thread,
		"title":       t.Title,
		"status":      t.Status,
		"labels":      strSlice(t.Labels),
		"assignees":   strSlice(t.Assignees),
		"description": t.Description,
		"summary":     t.Summary,
		"openedAt":    t.OpenedAt,
		"updatedAt":   t.UpdatedAt,
		"lastSeq":     t.LastSeq,
		"comments":    t.Comments,
		"aliases":     strSlice(t.Aliases),
	}
}

func (s *Server) handleChannelThreads(c *gin.Context) {
	name := c.Param("name")
	events, err := s.readChannelEvents(name)
	if err != nil {
		if err == errChannelNotFound {
			errorJSON(c, http.StatusNotFound, "channel %q 不存在", name)
			return
		}
		errorJSON(c, http.StatusBadRequest, "读取 channel %q 失败：%v", name, err)
		return
	}
	threads := store.ReduceThreads(events)
	rows := []map[string]any{}
	for _, t := range threads {
		rows = append(rows, threadDTO(t))
	}
	c.JSON(http.StatusOK, rows)
}

func (s *Server) handleChannelThread(c *gin.Context) {
	name := c.Param("name")
	thread := c.Param("thread")
	events, err := s.readChannelEvents(name)
	if err != nil {
		if err == errChannelNotFound {
			errorJSON(c, http.StatusNotFound, "channel %q 不存在", name)
			return
		}
		errorJSON(c, http.StatusBadRequest, "读取 channel %q 失败：%v", name, err)
		return
	}
	timeline := store.CollectThreadTimeline(events, thread)
	if len(timeline) == 0 {
		errorJSON(c, http.StatusNotFound, "channel %q 下没有 thread %q", name, thread)
		return
	}
	rows := make([]map[string]any, 0, len(timeline))
	for _, ev := range timeline {
		rows = append(rows, eventDTO(ev))
	}
	c.JSON(http.StatusOK, map[string]any{
		"thread": thread,
		"events": rows,
	})
}
