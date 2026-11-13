package serve

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/linrong007/Sunshine/internal/channel/store"
	"github.com/linrong007/Sunshine/internal/mem"
	"github.com/linrong007/Sunshine/internal/mem/adapters"
	"github.com/linrong007/Sunshine/internal/migrate"
	"github.com/linrong007/Sunshine/internal/task"
	"github.com/linrong007/Sunshine/internal/update"
)

// ---------------------------------------------------------------------------
// 会话 / 记忆
// ---------------------------------------------------------------------------

// memSessionDTO 返回 MemSessionInfo 的 8 个字段；同时补一份首字母大写
// 的键（前端表格用 s.Platform / s.Title / s.CWD 等大写键取值）。
func memSessionDTO(s mem.MemSessionInfo) map[string]any {
	return map[string]any{
		"platform":  string(s.Platform),
		"id":        s.ID,
		"title":     s.Title,
		"cwd":       s.CWD,
		"created":   s.Created,
		"updated":   s.Updated,
		"filePath":  s.FilePath,
		"parent_id": s.ParentID,
		"Platform":  string(s.Platform),
		"ID":        s.ID,
		"Title":     s.Title,
		"CWD":       s.CWD,
		"Created":   s.Created,
		"Updated":   s.Updated,
		"FilePath":  s.FilePath,
		"ParentID":  s.ParentID,
	}
}

func (s *Server) handleMemSessions(c *gin.Context) {
	home, err := os.UserHomeDir()
	if err != nil {
		errorJSON(c, http.StatusInternalServerError, "无法获取用户主目录：%v", err)
		return
	}
	mf, err := mem.BuildFilter(mem.FilterOptions{
		Platform: strings.TrimSpace(c.Query("platform")),
		Cwd:      s.root,
		Limit:    mem.WideLimit,
	})
	if err != nil {
		errorJSON(c, http.StatusBadRequest, "%v", err)
		return
	}
	rows, _, err := mem.ListSessions(context.Background(), home, mf, adapters.All())
	if err != nil {
		errorJSON(c, http.StatusInternalServerError, "读取会话失败：%v", err)
		return
	}
	out := make([]map[string]any, 0, len(rows))
	for _, row := range rows {
		out = append(out, memSessionDTO(row))
	}
	c.JSON(http.StatusOK, out)
}

func (s *Server) handleMemProjects(c *gin.Context) {
	home, err := os.UserHomeDir()
	if err != nil {
		errorJSON(c, http.StatusInternalServerError, "无法获取用户主目录：%v", err)
		return
	}
	mf, err := mem.BuildFilter(mem.FilterOptions{
		Platform: strings.TrimSpace(c.Query("platform")),
		Global:   true,
		Limit:    mem.WideLimit,
	})
	if err != nil {
		errorJSON(c, http.StatusBadRequest, "%v", err)
		return
	}
	// ListProjects 恒全局扫描，须按 Cwd 过滤出当前项目。
	rows, _, err := mem.ListProjects(context.Background(), home, mf, adapters.All())
	if err != nil {
		errorJSON(c, http.StatusInternalServerError, "读取项目汇总失败：%v", err)
		return
	}
	out := []mem.MemProjectSummary{}
	for _, row := range rows {
		if mem.SameProject(row.Cwd, s.root) {
			out = append(out, row)
		}
	}
	c.JSON(http.StatusOK, out)
}

// ---------------------------------------------------------------------------
// 项目健康
// ---------------------------------------------------------------------------

func fileChangePaths(in []update.FileChange) []string {
	out := make([]string, 0, len(in))
	for _, f := range in {
		out = append(out, f.RelativePath)
	}
	return out
}

func (s *Server) handleHealth(c *gin.Context) {
	projectVersion, cliVersion, latestVersion := s.versions()
	hashes := update.LoadHashes(s.root)
	templates, err := s.cachedTemplates(hashes)
	if err != nil {
		errorJSON(c, http.StatusInternalServerError, "收集模板失败：%v", err)
		return
	}
	changes := update.AnalyzeChanges(s.root, hashes, templates, templates[update.CopilotInstructionsPath])

	// 孤儿清单键（persist=false，只计算不写盘）。
	orphanKeys := []string{}
	configured := update.ConfiguredPlatforms(s.root, hashes)
	if prune, err := update.PruneOrphanManifestKeys(s.root, configured, hashes, update.MigrationPaths(), false); err == nil {
		orphanKeys = prune.Pruned
		if orphanKeys == nil {
			orphanKeys = []string{}
		}
	}

	// 遗漏迁移：区间内待迁移项 + 孤儿迁移检测（与 update 主流程同款）。
	pendingMigrations := []string{}
	if projectVersion != "unknown" {
		if registry, _ := migrate.LoadEmbeddedRegistry(); registry != nil {
			pending := registry.MigrationsForVersion(projectVersion, cliVersion)
			if orphaned := migrate.FindOrphanMigrations(s.root, registry.AllMigrations(), pending); len(orphaned) > 0 {
				pending = append(pending, orphaned...)
			}
			for _, item := range pending {
				if item.To != "" {
					pendingMigrations = append(pendingMigrations, item.From+" → "+item.To)
				} else {
					pendingMigrations = append(pendingMigrations, item.From)
				}
			}
		}
	}

	newFiles := fileChangePaths(changes.NewFiles)
	unchangedFiles := fileChangePaths(changes.UnchangedFiles)
	autoUpdateFiles := fileChangePaths(changes.AutoUpdateFiles)
	changedFiles := fileChangePaths(changes.ChangedFiles)
	userDeletedFiles := fileChangePaths(changes.UserDeletedFiles)
	c.JSON(http.StatusOK, map[string]any{
		"new_files":          newFiles,
		"unchanged_files":    unchangedFiles,
		"auto_update_files":  autoUpdateFiles,
		"changed_files":      changedFiles,
		"user_deleted_files": userDeletedFiles,
		"orphan_keys":        orphanKeys,
		"pending_migrations": pendingMigrations,
		"version": map[string]any{
			"project_version": projectVersion,
			"cli_version":     cliVersion,
			"latest_version":  latestVersion,
		},
		// 前端健康桶按计数展示，另给一份计数。
		"drift": map[string]any{
			"added":     len(newFiles),
			"updatable": len(autoUpdateFiles),
			"modified":  len(changedFiles),
			"unchanged": len(unchangedFiles),
			"deleted":   len(userDeletedFiles),
		},
		"added":     len(newFiles),
		"updatable": len(autoUpdateFiles),
		"modified":  len(changedFiles),
		"unchanged": len(unchangedFiles),
		"deleted":   len(userDeletedFiles),
	})
}

// ---------------------------------------------------------------------------
// 设计文档
// ---------------------------------------------------------------------------

// countDesignLines 启发式行数：非空行数。
func countDesignLines(data []byte) int {
	n := 0
	for _, line := range strings.Split(string(data), "\n") {
		if strings.TrimSpace(line) != "" {
			n++
		}
	}
	return n
}

func designDocInfo(root, rel string) map[string]any {
	data, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(rel)))
	if err != nil {
		return map[string]any{"exists": false, "lines": 0}
	}
	return map[string]any{"exists": true, "lines": countDesignLines(data)}
}

func (s *Server) handleDesign(c *gin.Context) {
	requirements := designDocInfo(s.root, "doc/design/需求列表.md")
	outline := designDocInfo(s.root, "doc/design/概要设计.md")
	prototype := prototypeDocInfo(s.root)
	c.JSON(http.StatusOK, map[string]any{
		"requirements": requirements,
		"outline":      outline,
		"prototype":    prototype,
		"note":         "启发式计数，仅供参考",
		// 前端 designDocs() 读 docs 数组，另给一份。
		"docs": []map[string]any{
			{"name": "需求列表.md", "exists": requirements["exists"], "lines": requirements["lines"]},
			{"name": "概要设计.md", "exists": outline["exists"], "lines": outline["lines"]},
			{"name": "原型/", "exists": prototype["exists"], "lines": prototype["lines"], "countText": prototype["countText"]},
		},
	})
}

// prototypeDocInfo 汇总 doc/prototype/ 文件夹：页数与各页总行数。
// 原型按页分文件后不再有单文件原型；旧的单文件原型.html 如存在也计为一页。
func prototypeDocInfo(root string) map[string]any {
	dir := filepath.Join(root, "doc", "prototype")
	entries, err := os.ReadDir(dir)
	if err != nil {
		return map[string]any{"exists": false, "lines": 0, "pages": 0, "countText": "0 页"}
	}
	pages, total := 0, 0
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".html") {
			continue
		}
		data, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			continue
		}
		pages++
		total += countDesignLines(data)
	}
	return map[string]any{"exists": pages > 0, "lines": total, "pages": pages, "countText": fmt.Sprintf("%d 页", pages)}
}

// ---------------------------------------------------------------------------
// Journal
// ---------------------------------------------------------------------------

func (s *Server) journalDir() string {
	dev := s.developer()
	if dev == "" {
		return ""
	}
	return filepath.Join(s.root, task.DirSunshine, "workspace", dev)
}

func (s *Server) handleJournal(c *gin.Context) {
	dir := s.journalDir()
	files := []map[string]any{}
	if dir != "" {
		entries, err := os.ReadDir(dir)
		if err == nil {
			type journalFile struct {
				name    string
				modTime time.Time
			}
			var found []journalFile
			for _, e := range entries {
				if e.IsDir() {
					continue
				}
				if strings.HasPrefix(e.Name(), "journal-") && strings.HasSuffix(e.Name(), ".md") {
					info, err := e.Info()
					if err != nil {
						continue
					}
					found = append(found, journalFile{name: e.Name(), modTime: info.ModTime()})
				}
			}
			// 最新修改的文件在前。
			sort.Slice(found, func(i, j int) bool { return found[i].modTime.After(found[j].modTime) })
			for _, f := range found {
				files = append(files, map[string]any{
					"name": f.name,
					"path": filepath.Join(dir, f.name),
				})
			}
		}
	}
	want := strings.TrimSpace(c.Query("file"))
	content := ""
	if want != "" {
		// 按文件名或完整路径取指定文件；拒绝逃出 journal 目录的路径。
		for _, f := range files {
			if want == f["name"] || want == f["path"] {
				if data, err := os.ReadFile(f["path"].(string)); err == nil {
					content = string(data)
				}
				break
			}
		}
	} else if len(files) > 0 {
		// 默认返回最新一个文件文本。
		if data, err := os.ReadFile(files[0]["path"].(string)); err == nil {
			content = string(data)
		}
	}
	c.JSON(http.StatusOK, map[string]any{
		"files":   files,
		"content": content,
	})
}

// ---------------------------------------------------------------------------
// 活动流
// ---------------------------------------------------------------------------

func (s *Server) handleActivity(c *gin.Context) {
	mtimeEntries := s.activity.scan()

	// channel 事件条目：当前项目各 channel 取最新 20 条事件转成条目。
	channelEntries := []activityEntry{}
	for _, name := range store.ListChannelNamesInProject(s.projectKey) {
		ref := store.ChannelRef{Name: name, Project: s.projectKey}
		events, err := store.ReadChannelEvents(ref, nil)
		if err != nil || len(events) == 0 {
			continue
		}
		if len(events) > 20 {
			events = events[len(events)-20:]
		}
		for _, ev := range events {
			channelEntries = append(channelEntries, activityEntry{
				TS:       ev.Ts,
				Kind:     string(ev.Kind),
				Text:     name + " " + string(ev.Kind) + " " + ev.By,
				Source:   "channel",
				sortTime: parseEventTime(ev.Ts),
			})
		}
	}

	nativeEvents := loadNativeEvents(s.root)
	merged := mergeActivityEntries(mtimeEntries, channelEntries, nativeEntries(nativeEvents))
	entries := make([]map[string]any, 0, len(merged))
	for _, e := range merged {
		ts := e.TS
		if ts == "" {
			ts = e.sortTime.Format(time.RFC3339)
		}
		entries = append(entries, map[string]any{
			"ts":     ts,
			"kind":   e.Kind,
			"text":   e.Text,
			"source": e.Source,
		})
	}
	c.JSON(http.StatusOK, map[string]any{
		"entries":        entries,
		"native_running": deriveNativeRunning(nativeEvents, time.Now()),
	})
}
