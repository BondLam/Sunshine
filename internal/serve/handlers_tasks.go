package serve

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/BondLam/Sunshine/internal/task"
)

// ---------------------------------------------------------------------------
// /api/status /api/version
// ---------------------------------------------------------------------------

func (s *Server) handleStatus(c *gin.Context) {
	projectVersion, cliVersion, latestVersion := s.versions()
	current, _, _, _ := s.currentTaskObject()
	resp := map[string]any{
		"project_path":      s.root,
		"project_version":   projectVersion,
		"cli_version":       cliVersion,
		"latest_version":    latestVersion,
		"developer":         s.developer(),
		"platforms":         s.platforms(),
		"config_summary":    s.configSummary(),
		"current_task":      current,
		"poll_interval_sec": s.pollInterval,
	}
	if name := s.workflowTemplateName(); name != "" {
		resp["workflow_template"] = name
	}
	c.JSON(http.StatusOK, resp)
}

func (s *Server) handleVersion(c *gin.Context) {
	projectVersion, cliVersion, latestVersion := s.versions()
	c.JSON(http.StatusOK, map[string]any{
		"project_version": projectVersion,
		"cli_version":     cliVersion,
		"latest_version":  latestVersion,
	})
}

// ---------------------------------------------------------------------------
// 任务
// ---------------------------------------------------------------------------

func (s *Server) handleTasks(c *gin.Context) {
	tasks, allStatuses := s.activeTasks()
	rows := make([]map[string]any, 0, len(tasks))
	for _, t := range tasks {
		rows = append(rows, taskListDTO(t, allStatuses))
	}
	c.JSON(http.StatusOK, rows)
}

func (s *Server) handleCurrentTask(c *gin.Context) {
	current, stale, source, err := s.currentTaskObject()
	if err != nil {
		errorJSON(c, http.StatusInternalServerError, "读取当前任务失败：%v", err)
		return
	}
	c.JSON(http.StatusOK, map[string]any{
		"current_task": current,
		"source":       source,
		"stale":        stale,
	})
}

// jsonlEntry 是 implement/check.jsonl 里的一行（与 task.jsonlEntry 同形，
// 该类型未导出，这里自行解析）。
type jsonlEntry struct {
	File   string `json:"file"`
	Type   string `json:"type,omitempty"`
	Reason string `json:"reason,omitempty"`
}

// readJSONLEntries 逐行解析 jsonl 文件；文件不存在返回空数组。
func readJSONLEntries(path string) []jsonlEntry {
	data, err := os.ReadFile(path)
	if err != nil {
		return []jsonlEntry{}
	}
	entries := []jsonlEntry{}
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		var e jsonlEntry
		if err := json.Unmarshal([]byte(line), &e); err != nil || e.File == "" {
			continue
		}
		entries = append(entries, e)
	}
	return entries
}

// subtaskDTO 由子任务目录名解析出详情行；找不到时只给目录名。
func (s *Server) subtaskDTO(name string, allStatuses map[string]string) map[string]any {
	tasksDir := s.tasksDir()
	dir := filepath.Join(tasksDir, name)
	if st, err := os.Stat(dir); err != nil || !st.IsDir() {
		if found := task.FindArchivedTaskByDirName(tasksDir, name); found != "" {
			dir = found
		} else {
			return map[string]any{
				"dir": name, "id": name, "title": name, "status": "",
				"display_status": "", "priority": "", "assignee": "",
				"parent": nil, "children": []string{}, "package": nil,
				"progress_done": 0, "progress_total": 0,
			}
		}
	}
	rec, err := task.LoadRecord(dir)
	if err != nil {
		return map[string]any{
			"dir": name, "id": name, "title": name, "status": "",
			"display_status": "", "priority": "", "assignee": "",
			"parent": nil, "children": []string{}, "package": nil,
			"progress_done": 0, "progress_total": 0,
		}
	}
	info := &task.TaskInfo{
		DirName:  filepath.Base(dir),
		Dir:      dir,
		Title:    rec.Title,
		Name:     rec.Name,
		Status:   rec.Status,
		Assignee: rec.Assignee,
		Priority: rec.Priority,
		Children: rec.Children,
		Package:  strPtrValueString(rec.Package),
	}
	if rec.Parent != nil {
		info.Parent = *rec.Parent
	}
	if info.Name == "" {
		info.Name = info.DirName
	}
	return taskListDTO(info, allStatuses)
}

func strPtrValueString(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}

// resolveTaskDirWithArchive 先按活动任务解析，找不到再到归档里找。
// ResolveTaskDir 只认活动任务，归档任务的详情与提交都须经这道回退。
func (s *Server) resolveTaskDirWithArchive(id string) (string, error) {
	dir, err := task.ResolveTaskDir(id, s.root)
	if err == nil {
		return dir, nil
	}
	if found := task.FindArchivedTaskByDirName(s.tasksDir(), filepath.Base(id)); found != "" {
		return found, nil
	}
	return "", err
}

func (s *Server) handleTaskDetail(c *gin.Context) {
	id := c.Param("id")
	dir, err := s.resolveTaskDirWithArchive(id)
	if err != nil {
		errorJSON(c, http.StatusNotFound, "找不到任务 %q：%v", id, err)
		return
	}
	rec, err := task.LoadRecord(dir)
	if err != nil {
		errorJSON(c, http.StatusNotFound, "读取任务 %q 失败：%v", id, err)
		return
	}
	// 24 字段按 FieldOrder 顺序的键：MarshalRecord 已保序，
	// 在此基础上追加 prd_md / implement / check / subtasks。
	ordered, err := task.MarshalRecord(rec)
	if err != nil {
		errorJSON(c, http.StatusInternalServerError, "编码任务失败：%v", err)
		return
	}
	prd, _ := os.ReadFile(filepath.Join(dir, "prd.md"))
	implement := readJSONLEntries(filepath.Join(dir, "implement.jsonl"))
	check := readJSONLEntries(filepath.Join(dir, "check.jsonl"))

	_, allStatuses := s.activeTasks()
	subtasks := []map[string]any{}
	for _, name := range rec.Subtasks {
		subtasks = append(subtasks, s.subtaskDTO(name, allStatuses))
	}
	extra := map[string]any{
		"prd_md":    string(prd),
		"implement": implement,
		"check":     check,
		"subtasks":  subtasks,
	}
	extraJSON, err := json.Marshal(extra)
	if err != nil {
		errorJSON(c, http.StatusInternalServerError, "编码任务详情失败：%v", err)
		return
	}
	// ordered 形如 "{\n ... \n}\n"：去掉末尾 "}\n"，追加额外字段后闭合。
	trimmed := bytes.TrimSpace(ordered)
	if len(trimmed) == 0 || trimmed[len(trimmed)-1] != '}' {
		errorJSON(c, http.StatusInternalServerError, "任务记录编码异常")
		return
	}
	body := bytes.TrimSuffix(trimmed, []byte("}"))
	extraTrimmed := bytes.TrimSpace(extraJSON)
	extraTrimmed = bytes.TrimPrefix(extraTrimmed, []byte("{"))
	extraTrimmed = bytes.TrimSuffix(extraTrimmed, []byte("}"))
	var buf bytes.Buffer
	buf.Write(body)
	if len(bytes.TrimSpace(extraTrimmed)) > 0 {
		buf.WriteString(",")
		buf.Write(extraTrimmed)
	}
	buf.WriteString("}\n")
	c.Data(http.StatusOK, "application/json; charset=utf-8", buf.Bytes())
}

// handleArchive：不传 month 列全部月份与每组数量；传 month 返回该月归档任务。
func (s *Server) handleArchive(c *gin.Context) {
	month := strings.TrimSpace(c.Query("month"))
	archiveDir := filepath.Join(s.tasksDir(), task.DirArchive)
	if month == "" {
		months, err := os.ReadDir(archiveDir)
		if err != nil {
			c.JSON(http.StatusOK, map[string]any{"months": []any{}})
			return
		}
		rows := []map[string]any{}
		for _, m := range months {
			if !m.IsDir() {
				continue
			}
			count := 0
			if entries, err := os.ReadDir(filepath.Join(archiveDir, m.Name())); err == nil {
				for _, e := range entries {
					if !e.IsDir() {
						continue
					}
					if _, err := os.Stat(filepath.Join(archiveDir, m.Name(), e.Name(), task.FileTaskJSON)); err == nil {
						count++
					}
				}
			}
			rows = append(rows, map[string]any{"month": m.Name(), "count": count})
		}
		sort.Slice(rows, func(i, j int) bool { return rows[i]["month"].(string) < rows[j]["month"].(string) })
		c.JSON(http.StatusOK, map[string]any{"months": rows})
		return
	}
	if len(month) != 7 || month[4] != '-' {
		errorJSON(c, http.StatusBadRequest, "month 参数格式错误，应为 YYYY-MM")
		return
	}
	monthDir := filepath.Join(archiveDir, month)
	entries, err := os.ReadDir(monthDir)
	if err != nil {
		c.JSON(http.StatusOK, []any{})
		return
	}
	_, allStatuses := s.activeTasks()
	rows := []map[string]any{}
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		rec, err := task.LoadRecord(filepath.Join(monthDir, e.Name()))
		if err != nil {
			continue
		}
		info := &task.TaskInfo{
			DirName:  e.Name(),
			Dir:      filepath.Join(monthDir, e.Name()),
			Title:    rec.Title,
			Name:     rec.Name,
			Status:   rec.Status,
			Assignee: rec.Assignee,
			Priority: rec.Priority,
			Children: rec.Children,
			Package:  strPtrValueString(rec.Package),
		}
		if rec.Parent != nil {
			info.Parent = *rec.Parent
		}
		if info.Name == "" {
			info.Name = info.DirName
		}
		rows = append(rows, taskListDTO(info, allStatuses))
	}
	c.JSON(http.StatusOK, rows)
}

// handleTaskCommits 用 git log --grep 定位归档提交；查不到返回空数组。
func (s *Server) handleTaskCommits(c *gin.Context) {
	id := c.Param("id")
	dir, err := s.resolveTaskDirWithArchive(id)
	if err != nil {
		errorJSON(c, http.StatusNotFound, "找不到任务 %q：%v", id, err)
		return
	}
	dirName := filepath.Base(dir)
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "git",
		"-c", "i18n.logOutputEncoding=UTF-8",
		"log", "--fixed-strings",
		"--grep=chore(task): archive "+dirName,
		"--format=%H%x1f%ad%x1f%s", "--date=iso-strict")
	cmd.Dir = s.root
	out, err := cmd.Output()
	if err != nil {
		// 非 git 仓库或无提交：返回空数组，不编造。
		c.JSON(http.StatusOK, []any{})
		return
	}
	rows := []map[string]any{}
	for _, line := range strings.Split(string(out), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		parts := strings.SplitN(line, "\x1f", 3)
		if len(parts) != 3 {
			continue
		}
		rows = append(rows, map[string]any{
			"hash":    parts[0],
			"date":    parts[1],
			"message": parts[2],
		})
	}
	c.JSON(http.StatusOK, rows)
}

// parseIntQuery 解析整数查询参数，非法时返回默认值。
func parseIntQuery(c *gin.Context, key string, def int) int {
	raw := strings.TrimSpace(c.Query(key))
	if raw == "" {
		return def
	}
	n, err := strconv.Atoi(raw)
	if err != nil {
		return def
	}
	return n
}
