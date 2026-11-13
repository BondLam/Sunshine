package hook

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"unicode/utf8"
)

// 子代理名：Go 版用 sunshine- 前缀；trellis- 旧名作兼容别名接受，
// 统一归一化为 sunshine- 名。
const (
	AgentImplement = "sunshine-implement"
	AgentCheck     = "sunshine-check"
	AgentResearch  = "sunshine-research"
	AgentPrototype = "sunshine-prototype"
)

var agentsAll = []string{AgentImplement, AgentCheck, AgentResearch, AgentPrototype}

// shortAgentName 把 sunshine- 前缀去掉，得到 implement/check/research/prototype，
// 与 activity.jsonl 与 serve 前端的角色键一致（agentType 已归一化）。
func shortAgentName(agent string) string {
	return strings.TrimPrefix(agent, "sunshine-")
}

func normalizeAgentName(name string) string {
	name = strings.TrimSpace(name)
	if strings.HasPrefix(name, "trellis-") {
		name = "sunshine-" + strings.TrimPrefix(name, "trellis-")
	}
	for _, a := range agentsAll {
		if name == a {
			return a
		}
	}
	return ""
}

func agentRequiresTask(name string) bool {
	return name == AgentImplement || name == AgentCheck
}

// RunSubagentContext 实现 sunshine hook subagent-context，对应
// shared-hooks/inject-subagent-context.py（PreToolUse / SubagentStart）。
// 给 implement/check/research 子代理注入任务上下文。
func RunSubagentContext(r io.Reader, opts Options) error {
	if HooksDisabled() {
		return nil
	}
	input := ReadInput(r)
	if GetString(input, "hook_event_name", "hookEventName") == "SubagentStart" {
		handleCodexSubagentStart(input, opts)
		return nil
	}
	agentType, originalPrompt, toolInput := parseHookInput(input)
	if normalizeAgentName(agentType) == "" {
		return nil
	}
	agentType = normalizeAgentName(agentType)
	cwd := GetString(input, "cwd")
	if cwd == "" {
		if wd, err := os.Getwd(); err == nil {
			cwd = wd
		}
	}
	// 偏差：Python 用 .git 向上找仓库根；Go 版按任务要求统一用
	// .sunshine/ 定位（无 .sunshine/ 即非 Sunshine 项目，静默退出）。
	root := FindSunshineRoot(cwd)
	if root == "" {
		return nil
	}
	taskDir := resolveTaskForSubagent(root, input)
	if agentRequiresTask(agentType) {
		if taskDir == "" {
			return nil
		}
		// 收容检查：指针被投毒（指向仓外）时拒绝读取。
		if ResolveTaskRef(taskDir, root) == "" {
			return nil
		}
		full := ResolveTaskRef(taskDir, root)
		if full == "" || !isDir(full) {
			return nil
		}
	}
	if agentType == AgentPrototype {
		// prototype 只记 start 事件，不注入 implement/check 式的大上下文
		// （v0.3.11 token 规则：brief 由主会话在派发提示里给两份设计
		// 文档路径，hook 不再注入；也不读 spec/workflow.md）。
		RecordSubagentStart(root, "prototype", taskDir, GetString(input, "session_id", "sessionId", "sessionID"))
		return nil
	}
	var context, newPrompt string
	switch agentType {
	case AgentImplement:
		context = GetImplementContext(root, taskDir)
		newPrompt = buildImplementPrompt(originalPrompt, context)
	case AgentCheck:
		if strings.Contains(strings.ToLower(originalPrompt), "[finish]") {
			context = GetCheckContext(root, taskDir)
			newPrompt = buildFinishPrompt(originalPrompt, context)
		} else {
			context = GetCheckContext(root, taskDir)
			newPrompt = buildCheckPrompt(originalPrompt, context)
		}
	case AgentResearch:
		context = GetResearchContext(root, taskDir)
		newPrompt = buildResearchPrompt(originalPrompt, context)
	}
	if context == "" {
		return nil
	}
	// 成功注入上下文后记一条 start 事件（serve 推导运行状态用）；
	// 写盘失败静默丢弃，不影响注入输出。
	RecordSubagentStart(root, shortAgentName(agentType), taskDir, GetString(input, "session_id", "sessionId", "sessionID"))
	updated := map[string]any{}
	for k, v := range toolInput {
		updated[k] = v
	}
	updated["prompt"] = newPrompt
	hookOut := map[string]any{
		"hookEventName":      "PreToolUse",
		"permissionDecision": "allow",
		"updatedInput":       updated,
	}
	output := map[string]any{
		"hookSpecificOutput": hookOut,
	}
	if DetectPlatform(input) != "zcode" {
		// ZCode 只认嵌套的 Claude 兼容形状；其他平台多发几种
		// 形状，各家只读自己认的那份。
		output["permission"] = "allow"
		output["updated_input"] = updated
		output["updatedInput"] = updated
	}
	WriteJSON(opts.out(), output)
	return nil
}

// resolveTaskForSubagent 用统一 active task 解析器取当前任务，
// 允许唯一会话文件回退（拿不到父身份的子代理调用方需要它）。
func resolveTaskForSubagent(root string, input HookInput) string {
	platform := DetectPlatformForSubagent(input)
	active := ResolveActiveTask(root, input, platform, ResolveOptions{
		AllowSingleSessionFallback: true,
		AllowEnvironmentContext:    true,
	})
	return active.TaskPath
}

// handleCodexSubagentStart 处理 Codex 原生 SubagentStart 事件：
// 事件自带父会话 id；必须关掉唯一会话回退与环境上下文，
// 否则原生子代理会借到别的 Codex 窗口的任务；stale 指针也拒绝。
func handleCodexSubagentStart(input HookInput, opts Options) {
	agentType := normalizeAgentName(GetString(input, "agent_type", "agentType"))
	if agentType == "" {
		return
	}
	parentSessionID := GetString(input, "session_id", "sessionId", "sessionID")
	if parentSessionID == "" {
		return
	}
	var root string
	for _, cand := range []string{GetString(input, "cwd")} {
		if cand == "" {
			continue
		}
		if root = FindSunshineRoot(cand); root != "" {
			break
		}
	}
	if root == "" {
		if wd, err := os.Getwd(); err == nil {
			root = FindSunshineRoot(wd)
		}
	}
	if root == "" {
		return
	}
	parentInput := HookInput{"session_id": parentSessionID}
	active := ResolveActiveTask(root, parentInput, "codex", ResolveOptions{
		AllowSingleSessionFallback: false,
		AllowEnvironmentContext:    false,
	})
	if active.TaskPath == "" || active.Stale {
		return
	}
	taskDir := active.TaskPath
	if agentRequiresTask(agentType) {
		full := ResolveTaskRef(taskDir, root)
		if full == "" || !isDir(full) {
			return
		}
	}
	if agentType == AgentPrototype {
		// prototype 只记 start 事件，不注入上下文（同 Claude 路径）。
		RecordSubagentStart(root, "prototype", taskDir, parentSessionID)
		return
	}
	var context string
	switch agentType {
	case AgentImplement:
		context = GetImplementContext(root, taskDir)
	case AgentCheck:
		context = GetCheckContext(root, taskDir)
	default:
		context = GetResearchContext(root, taskDir)
	}
	if context == "" {
		return
	}
	// 成功注入上下文后记一条 start 事件；写盘失败静默丢弃。
	RecordSubagentStart(root, shortAgentName(agentType), taskDir, parentSessionID)
	WriteJSON(opts.out(), map[string]any{
		"hookSpecificOutput": map[string]any{
			"hookEventName":     "SubagentStart",
			"additionalContext": buildCodexSubagentContext(agentType, taskDir, context),
		},
	})
}

func buildCodexSubagentContext(subagentType, taskDir, context string) string {
	role := strings.TrimPrefix(subagentType, "sunshine-")
	role = strings.TrimPrefix(role, "trellis-")
	title := role
	if title != "" {
		title = strings.ToUpper(title[:1]) + title[1:]
	}
	return fmt.Sprintf(`<!-- sunshine-hook-injected -->
# Sunshine Native %s Subagent

You are the dispatched %s role for this task. Perform that role
directly; do not follow main-session dispatch or wait instructions, and do not
spawn another Sunshine subagent.

Active task: %s

## Curated Context

%s`, title, "`"+subagentType+"`", taskDir, context)
}

// parseHookInput 解析各平台的 hook 输入，返回（子代理类型，原始提示词，tool_input）。
func parseHookInput(input HookInput) (string, string, map[string]any) {
	toolInput := map[string]any{}
	if ti, ok := input["tool_input"].(map[string]any); ok {
		toolInput = ti
	} else if ti, ok := input["toolInput"].(map[string]any); ok {
		toolInput = ti
	}
	toolName := GetString(input, "tool_name", "toolName")
	switch strings.ToLower(toolName) {
	case "task", "agent", "subagent":
		return extractSubagentType(toolInput), stringField(toolInput, "prompt"), toolInput
	}
	if agentName := GetString(input, "agent_name"); agentName != "" {
		prompt := stringField(toolInput, "prompt")
		if prompt == "" {
			prompt = GetString(input, "prompt")
		}
		return agentName, prompt, toolInput
	}
	if normalizeAgentName(toolName) != "" {
		return toolName, stringField(toolInput, "prompt"), toolInput
	}
	if toolNameCamel := GetString(input, "toolName"); normalizeAgentName(toolNameCamel) != "" {
		return toolNameCamel, GetString(input, "toolArgs"), toolInput
	}
	return "", "", toolInput
}

func stringField(m map[string]any, key string) string {
	s, _ := m[key].(string)
	return s
}

func extractSubagentType(toolInput map[string]any) string {
	for _, key := range []string{
		"subagent_type", "subagentType", "subagent_type_name", "subagentTypeName",
		"subagent_name", "subagentName", "agent_type", "agentType", "name",
	} {
		if name := extractSubagentName(toolInput[key]); name != "" {
			return name
		}
	}
	return ""
}

// extractSubagentName 从各平台的子代理名编码里取值：Cursor 的 Task
// 参数把自定义子代理编成 protobuf oneof，可能出现 {"custom": {"name": …}}
// 或 {"type": {"case": "custom", "value": {"name": …}}} 两种形状。
func extractSubagentName(value any) string {
	if s, ok := value.(string); ok && strings.TrimSpace(s) != "" {
		return strings.TrimSpace(s)
	}
	m, ok := value.(map[string]any)
	if !ok {
		return ""
	}
	for _, key := range []string{"name", "subagent_type_name", "subagentTypeName"} {
		if s := stringField(m, key); strings.TrimSpace(s) != "" {
			return strings.TrimSpace(s)
		}
	}
	if custom, ok := m["custom"].(map[string]any); ok {
		if s := stringField(custom, "name"); s != "" {
			return s
		}
	}
	if oneof, ok := m["type"].(map[string]any); ok {
		if name := extractFromOneof(oneof); name != "" {
			return name
		}
	}
	if name := extractFromOneof(m); name != "" {
		return name
	}
	for _, agent := range agentsAll {
		if _, ok := m[agent]; ok {
			return agent
		}
	}
	// 兼容旧 trellis- 名作为键出现的情况。
	for _, agent := range []string{"trellis-implement", "trellis-check", "trellis-research"} {
		if _, ok := m[agent]; ok {
			return agent
		}
	}
	return ""
}

func extractFromOneof(m map[string]any) string {
	caseName := stringField(m, "case")
	if caseName == "custom" {
		if nested, ok := m["value"].(map[string]any); ok {
			if s := stringField(nested, "name"); s != "" {
				return s
			}
		}
	}
	return caseName
}

// ---------------------------------------------------------------------------
// 上下文注入预算与物化
// ---------------------------------------------------------------------------

// Limits 是 context_injection 的字节上限（0 表示不限）。
type Limits struct {
	MaxFileBytes     int
	MaxArtifactBytes int
	MaxTotalBytes    int
}

func limitsFromConfig(cfg Config) Limits {
	return Limits{
		MaxFileBytes:     cfg.MaxFileBytes,
		MaxArtifactBytes: cfg.MaxArtifactBytes,
		MaxTotalBytes:    cfg.MaxTotalBytes,
	}
}

// budget 追踪已注入的总字节数。
type budget struct {
	maxTotal int
	used     int
}

func (b *budget) hasRoom(size int) bool {
	if b.maxTotal <= 0 {
		return true
	}
	return b.used+size <= b.maxTotal
}

func (b *budget) add(size int) { b.used += size }

// truncateUTF8 把 data 截到至多 cap 字节，且不切断 UTF-8 多字节序列；
// cap <= 0 表示不限。
func truncateUTF8(data []byte, cap int) []byte {
	if cap <= 0 || len(data) <= cap {
		return data
	}
	truncated := data[:cap]
	i := len(truncated)
	for i > 0 && (truncated[i-1]&0xC0) == 0x80 {
		i--
	}
	if i == 0 {
		return []byte{}
	}
	lead := truncated[i-1]
	if lead&0x80 != 0 {
		var seqLen int
		switch {
		case (lead & 0xE0) == 0xC0:
			seqLen = 2
		case (lead & 0xF0) == 0xE0:
			seqLen = 3
		case (lead & 0xF8) == 0xF0:
			seqLen = 4
		default:
			seqLen = 1
		}
		if (i-1)+seqLen > len(truncated) {
			i--
		}
	}
	return truncated[:i]
}

func isBinaryContent(data []byte) bool {
	for _, b := range data {
		if b == 0 {
			return true
		}
	}
	return !utf8.Valid(data)
}

// readFileBytes 读 basePath 下 filePath 的原始字节；文件不存在或
// 解析后的真实路径落在仓库与 .sunshine 实址之外时返回 nil。
func readFileBytes(basePath, filePath string) []byte {
	fullPath := filepath.Join(basePath, filepath.FromSlash(filePath))
	rootReal, err := filepath.EvalSymlinks(basePath)
	if err != nil {
		return nil
	}
	workflowReal, _ := filepath.EvalSymlinks(filepath.Join(basePath, ".sunshine"))
	fullReal, err := filepath.EvalSymlinks(fullPath)
	if err != nil {
		// 不存在的文件 EvalSymlinks 失败：用词法路径检查。
		fullReal = filepath.Clean(fullPath)
	}
	if !contained(rootReal, fullReal) && (workflowReal == "" || !contained(workflowReal, fullReal)) {
		return nil
	}
	data, err := os.ReadFile(fullPath)
	if err != nil {
		return nil
	}
	return data
}

func truncateNotice(path string, cap int) string {
	return fmt.Sprintf("\n[Sunshine: truncated at %d bytes — read %s for the full content]", cap, path)
}

func binaryNotice(path string, size int, reason string) string {
	return fmt.Sprintf("[Sunshine: not inlined (binary file) — %s (%d bytes): %s]", path, size, reason)
}

func indexNotice(path string, size int, reason string) string {
	return fmt.Sprintf("[Sunshine: not inlined (total context limit reached) — %s (%d bytes): %s]", path, size, reason)
}

// budgetedBlock 返回 "=== header ===\n内容" 块；总预算耗尽时退化为索引提示。
func budgetedBlock(b *budget, header, plainPath, content, reason string, sizeForIndex int) string {
	block := "=== " + header + " ===\n" + content
	blockBytes := len(block)
	if !b.hasRoom(blockBytes) {
		notice := indexNotice(plainPath, sizeForIndex, reason)
		b.add(len(notice))
		return notice
	}
	b.add(blockBytes)
	return block
}

// parseLineRange 解析 jsonl 条目的 lines 字段（"120-180"，1-based 闭区间）。
// 空串表示不限（整文件）；格式非法或 start<1 或 start>end 返回 ok=false。
func parseLineRange(s string) (start, end int, ok bool) {
	if s == "" {
		return 0, 0, false
	}
	parts := strings.Split(s, "-")
	if len(parts) != 2 {
		return 0, 0, false
	}
	start, err1 := strconv.Atoi(strings.TrimSpace(parts[0]))
	end, err2 := strconv.Atoi(strings.TrimSpace(parts[1]))
	if err1 != nil || err2 != nil || start < 1 || start > end {
		return 0, 0, false
	}
	return start, end, true
}

// sliceLines 按 1-based 闭区间 [start, end] 取 data 的行子集；end 超过
// 总行数时截到文件末尾。返回（切片内容，总行数，实际注入的末行）。
func sliceLines(data []byte, start, end int) (string, int, int) {
	lines := strings.Split(string(data), "\n")
	if n := len(lines); n > 0 && lines[n-1] == "" {
		lines = lines[:n-1]
	}
	total := len(lines)
	if start > total {
		return "", total, 0
	}
	if end > total {
		end = total
	}
	return strings.Join(lines[start-1:end], "\n"), total, end
}

// materializeFile 读一个 jsonl 引用的文件，施加单文件上限再计入总预算。
// lines 非空时只注入该行范围（1-based 闭区间），块头注明实际注入的行号。
func materializeFile(basePath, filePath, reason, lines string, limits Limits, b *budget) string {
	data := readFileBytes(basePath, filePath)
	if data == nil {
		return ""
	}
	size := len(data)
	if isBinaryContent(data) {
		notice := binaryNotice(filePath, size, reason)
		b.add(len(notice))
		return notice
	}
	header := filePath
	if lines != "" {
		if start, end, ok := parseLineRange(lines); ok {
			content, total, clampedEnd := sliceLines(data, start, end)
			if clampedEnd == 0 {
				notice := fmt.Sprintf("[Sunshine: %s 的 lines %q 超出文件总行数（%d 行），未注入内容]", filePath, lines, total)
				b.add(len(notice))
				return notice
			}
			header = fmt.Sprintf("%s (lines %d-%d of %d)", filePath, start, clampedEnd, total)
			data = []byte(content)
			size = len(data)
		}
	}
	truncated := truncateUTF8(data, limits.MaxFileBytes)
	content := string(truncated)
	if len(truncated) < size {
		content += truncateNotice(filePath, limits.MaxFileBytes)
	}
	return budgetedBlock(b, header, filePath, content, reason, size)
}

// materializeDirectory 读目录下全部 .md 文件（最多 20 个，按名排序），
// 与单文件走同样的单文件上限与总预算。
func materializeDirectory(basePath, dirPath, reason string, limits Limits, b *budget) []string {
	fullPath := filepath.Join(basePath, filepath.FromSlash(dirPath))
	entries, err := os.ReadDir(fullPath)
	if err != nil {
		return nil
	}
	var names []string
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".md") {
			names = append(names, e.Name())
		}
	}
	sort.Strings(names)
	if len(names) > 20 {
		names = names[:20]
	}
	var blocks []string
	for _, name := range names {
		rel := dirPath + "/" + name
		if block := materializeFile(basePath, rel, reason, "", limits, b); block != "" {
			blocks = append(blocks, block)
		}
	}
	return blocks
}

// jsonlEntry 是 jsonl 上下文文件里的一行：{"file": 路径, "type": "file"|
// "directory", "reason": …, "lines": "120-180"（可选，1-based 闭区间，
// 缺省整文件）}；无 file 字段的行（旧版占位行）跳过。
type jsonlEntry struct {
	File   string
	Type   string
	Reason string
	Lines  string
}

func readJSONLEntries(basePath, jsonlPath string, stderr io.Writer) []jsonlEntry {
	fullPath := filepath.Join(basePath, filepath.FromSlash(jsonlPath))
	if _, err := os.Stat(fullPath); err != nil {
		fmt.Fprintf(stderr, "[inject-subagent-context] WARN: %s not found — sub-agent will receive only task artifacts\n", jsonlPath)
		return nil
	}
	data, err := os.ReadFile(fullPath)
	if err != nil {
		return nil
	}
	var entries []jsonlEntry
	sawReal := false
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		var item map[string]any
		if err := json.Unmarshal([]byte(line), &item); err != nil {
			continue
		}
		file, _ := item["file"].(string)
		if file == "" {
			file, _ = item["path"].(string)
		}
		if file == "" {
			continue
		}
		sawReal = true
		typ, _ := item["type"].(string)
		if typ == "" {
			typ = "file"
		}
		reason, _ := item["reason"].(string)
		if reason == "" {
			reason = "-"
		}
		lines, _ := item["lines"].(string)
		if lines != "" {
			if _, _, ok := parseLineRange(lines); !ok {
				fmt.Fprintf(stderr, "[inject-subagent-context] WARN: %s 的 lines %q 格式非法（应为 \"起始行-结束行\"，如 \"120-180\"），已按整文件注入\n", file, lines)
				lines = ""
			}
		}
		entries = append(entries, jsonlEntry{File: file, Type: typ, Reason: reason, Lines: lines})
	}
	if !sawReal {
		fmt.Fprintf(stderr, "[inject-subagent-context] WARN: %s has no curated entries (only seed / empty) — sub-agent will receive only task artifacts. See workflow.md planning artifact guidance.\n", jsonlPath)
	}
	return entries
}

func materializeJSONLEntries(basePath, jsonlPath string, limits Limits, b *budget, stderr io.Writer) []string {
	var blocks []string
	for _, entry := range readJSONLEntries(basePath, jsonlPath, stderr) {
		if entry.Type == "directory" {
			blocks = append(blocks, materializeDirectory(basePath, entry.File, entry.Reason, limits, b)...)
		} else if block := materializeFile(basePath, entry.File, entry.Reason, entry.Lines, limits, b); block != "" {
			blocks = append(blocks, block)
		}
	}
	return blocks
}

// getAgentContext 读 {agent}.jsonl 的整理上下文；无整理条目时在提示词
// 里明说（只写 stderr 的警告进不了会话，子代理会误以为上下文完整）。
func getAgentContext(repoRoot, taskDir, agent string, limits Limits, b *budget, stderr io.Writer) string {
	jsonlPath := taskDir + "/" + agent + ".jsonl"
	blocks := materializeJSONLEntries(repoRoot, jsonlPath, limits, b, stderr)
	if len(blocks) == 0 {
		return fmt.Sprintf("[Sunshine] %s has no curated entries, so no spec/research context was injected. Before working, read the guidelines relevant to the code you will touch under .sunshine/spec/, and treat the task artifacts below as the only prepared context.", jsonlPath)
	}
	return strings.Join(blocks, "\n\n")
}

// materializeArtifact 读任务产物（prd/design/implement.md），施加
// 单产物上限再计入总预算。
func materializeArtifact(basePath, filePath, headerLabel, reason string, limits Limits, b *budget) string {
	data := readFileBytes(basePath, filePath)
	if data == nil {
		return ""
	}
	size := len(data)
	truncated := truncateUTF8(data, limits.MaxArtifactBytes)
	content := string(truncated)
	if len(truncated) < size {
		content += truncateNotice(filePath, limits.MaxArtifactBytes)
	}
	return budgetedBlock(b, headerLabel, filePath, content, reason, size)
}

// GetImplementContext 生成 Implement 子代理的完整上下文：
// implement.jsonl → prd.md → design.md（若有）→ implement.md（若有）。
func GetImplementContext(repoRoot, taskDir string) string {
	return getTaskContext(repoRoot, taskDir, "implement")
}

// GetCheckContext 生成 Check 子代理的上下文：check.jsonl + 任务产物。
// Finish 阶段复用同一上下文（最终检查即同一套材料）。
func GetCheckContext(repoRoot, taskDir string) string {
	return getTaskContext(repoRoot, taskDir, "check")
}

func getTaskContext(repoRoot, taskDir, agent string) string {
	cfg := LoadConfig(repoRoot)
	limits := limitsFromConfig(cfg)
	b := &budget{maxTotal: limits.MaxTotalBytes}
	var parts []string
	base := getAgentContext(repoRoot, taskDir, agent, limits, b, os.Stderr)
	if base != "" {
		parts = append(parts, base)
	}
	for _, art := range []struct{ file, label, reason string }{
		{taskDir + "/prd.md", taskDir + "/prd.md (Requirements)", "Requirements document"},
		{taskDir + "/design.md", taskDir + "/design.md (Technical Design)", "Technical design document"},
		{taskDir + "/implement.md", taskDir + "/implement.md (Execution Plan)", "Execution plan document"},
	} {
		if block := materializeArtifact(repoRoot, art.file, art.label, art.reason, limits, b); block != "" {
			parts = append(parts, block)
		}
	}
	return strings.Join(parts, "\n\n")
}

// GetResearchContext 生成 Research 子代理的上下文：项目规范目录结构
// 概览（不依赖当前任务）。
func GetResearchContext(repoRoot, taskDir string) string {
	specPath := ".sunshine/spec"
	specRoot := filepath.Join(repoRoot, ".sunshine", "spec")
	var treeLines []string
	treeLines = append(treeLines, specPath+"/")
	if entries, err := os.ReadDir(specRoot); err == nil {
		var pkgs []os.DirEntry
		for _, e := range entries {
			if e.IsDir() {
				pkgs = append(pkgs, e)
			}
		}
		sort.Slice(pkgs, func(i, j int) bool { return pkgs[i].Name() < pkgs[j].Name() })
		for i, pkg := range pkgs {
			isLast := i == len(pkgs)-1
			prefix := "├── "
			if isLast {
				prefix = "└── "
			}
			var layers []string
			if sub, err := os.ReadDir(filepath.Join(specRoot, pkg.Name())); err == nil {
				for _, d := range sub {
					if d.IsDir() {
						layers = append(layers, d.Name())
					}
				}
			}
			sort.Strings(layers)
			layerInfo := ""
			if len(layers) > 0 {
				layerInfo = " (" + strings.Join(layers, ", ") + ")"
			}
			treeLines = append(treeLines, prefix+pkg.Name()+"/"+layerInfo)
		}
	}
	specTree := strings.Join(treeLines, "\n")
	return fmt.Sprintf(`## Project Spec Directory Structure

`+"```"+`
%s
`+"```"+`

## Search Tips

- Spec files: `+"`"+`%s/**/*.md`+"`"+`
- Code search: Use Glob and Grep tools
- Tech solutions: Use web search tools`, specTree, specPath)
}

func buildImplementPrompt(originalPrompt, context string) string {
	return fmt.Sprintf(`<!-- sunshine-hook-injected -->
# Implement Agent Task

You are the Implement Agent in the Multi-Agent Pipeline.

## Your Context

All the information you need has been prepared for you:

%s

---

## Your Task

%s

---

## Workflow

1. **Understand specs** - All dev specs are injected above, understand them
    2. **Understand task artifacts** - Read requirements, technical design if present, and execution plan if present
    3. **Implement feature** - Implement following specs and task artifacts
4. **Self-check** - Ensure code quality against check specs

## Important Constraints

- Do NOT execute git commit, only code modifications
- Follow all dev specs injected above
- Report list of modified/created files when done`, context, originalPrompt)
}

func buildCheckPrompt(originalPrompt, context string) string {
	return fmt.Sprintf(`<!-- sunshine-hook-injected -->
# Check Agent Task

You are the Check Agent in the Multi-Agent Pipeline (code and cross-layer checker).

## Your Context

All check specs and dev specs you need:

%s

---

## Your Task

%s

---

## Workflow

1. **Get changes** - Run `+"`"+`git diff --name-only`+"`"+` and `+"`"+`git diff`+"`"+` to get code changes
2. **Check against specs** - Check item by item against specs above
3. **Self-fix** - Fix issues directly, don't just report
4. **Run verification** - Run project's lint and typecheck commands

## Important Constraints

- Fix issues yourself, don't just report
- Must execute complete checklist in check specs
- Pay special attention to impact radius analysis (L1-L5)`, context, originalPrompt)
}

func buildFinishPrompt(originalPrompt, context string) string {
	return fmt.Sprintf(`<!-- sunshine-hook-injected -->
# Finish Agent Task

You are performing the final check before creating a PR.

## Your Context

Finish checklist and requirements:

%s

---

## Your Task

%s

---

## Workflow

1. **Review changes** - Run `+"`"+`git diff --name-only`+"`"+` to see all changed files
	2. **Verify task artifacts** - Check requirements in prd.md and, when present, design.md / implement.md
3. **Spec sync** - Analyze whether changes introduce new patterns, contracts, or conventions
   - If new pattern/convention found: read target spec file → update it → update index.md if needed
   - If infra/cross-layer change: follow the 7-section mandatory template from update-spec.md
   - If pure code fix with no new patterns: skip this step
4. **Run final checks** - Execute lint and typecheck
5. **Confirm ready** - Ensure code is ready for PR

## Important Constraints

- You MAY update spec files when gaps are detected (use update-spec.md as guide)
- MUST read the target spec file BEFORE editing (avoid duplicating existing content)
- Do NOT update specs for trivial changes (typos, formatting, obvious fixes)
- If critical CODE issues found, report them clearly (fix specs, not code)
- Verify all acceptance criteria in prd.md are met
- Verify design.md and implement.md constraints when those files are present`, context, originalPrompt)
}

func buildResearchPrompt(originalPrompt, context string) string {
	return fmt.Sprintf(`# Research Agent Task

You are the Research Agent in the Multi-Agent Pipeline (search researcher).

## Core Principle

**You do one thing: find and explain information.**

You are a documenter, not a reviewer.

## Project Info

%s

---

## Your Task

%s

---

## Workflow

1. **Understand query** - Determine search type (internal/external) and scope
2. **Plan search** - List search steps for complex queries
3. **Execute search** - Execute multiple independent searches in parallel
4. **Organize results** - Output structured report

## Search Tools

| Tool | Purpose |
|------|---------|
| Glob | Search by filename pattern |
| Grep | Search by content |
| Read | Read file content |
| Web search | External web search |
| Code/doc search | External code/doc search |

## Strict Boundaries

**Only allowed**: Describe what exists, where it is, how it works

**Forbidden** (unless explicitly asked):
- Suggest improvements
- Criticize implementation
- Recommend refactoring
- Modify any files

## Report Format

Provide structured search results including:
- List of files found (with paths)
- Code pattern analysis (if applicable)
- Related spec documents
- External references (if any)`, context, originalPrompt)
}
