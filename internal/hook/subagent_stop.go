// RunSubagentStop 实现 sunshine hook subagent-stop，对应 Claude Code
// 的 SubagentStop 事件（Codex / Cursor 的同名事件亦然）：子代理结束
// 时记一条 subagent_stopped 事件，供 serve 推导运行状态。
//
// 关联办法（2026-11-12 按 Claude Code 官方 hooks 文档核实）：
//  1. 首选 payload 的 agent_type 字段：现版 SubagentStop 输入含
//     agent_id、agent_type、agent_transcript_path（Codex 与 Cursor
//     的 subagentStop 同样带 agent_type/subagent_type）。归一化后
//     不是 sunshine 四类子代理的（如 Claude 内部 agent），直接忽略。
//  2. 旧版宿主没有 agent_type 时，读 agent_transcript_path（无则
//     transcript_path）前若干行：从注入提示词的角色头
//     （# Implement Agent Task 等）取 agent，从
//     `.sunshine/tasks/<目录>/` 或 `Active task:` 取任务目录。
//  3. 任务仍取不到时，按当前任务 + 最近一个未结束的 start 事件匹配。
//  4. 任务始终取不到时，仍按 agent 记 stop（task 留空）；serve 按
//     agent 配对最近的未结束 start，feed 里显示“<agent> 已结束”。
//     只有 agent 也取不到时才不写盘。
//
// 退路的成立条件与失效行为：
//   - 顺序派发（workflow 的既定做法：一个子任务完整走完才启动下一个）
//     时，同一时刻同一角色最多一个在跑，最近未结束 start 就是刚结束
//     的那个，匹配正确。
//   - 并行派发同一角色时，退路可能把 stop 配给错的 start；start/stop
//     文本仍在 feed 里，运行中推导按 agent 取最近 start，至多把一个
//     仍在跑的角色误判为已结束——偏保守，不谎称运行中。
//   - 本命令不输出任何 decision（SubagentStop 的 block 语义与本批
//     无关），只写盘；绝不阻止子代理结束。
//
// 铁律：任何情况下返回 nil（退出码 0）；写盘失败静默丢弃。
package hook

import (
	"bufio"
	"io"
	"os"
	"regexp"
	"strings"
)

// transcriptHeadMaxBytes 是 transcript 扫描上限：注入提示词在子代理
// transcript 的前几行，读这么多足够；避免大文件拖慢 hook。
const transcriptHeadMaxBytes = 256 * 1024

var (
	// 注入提示词的角色头（buildImplementPrompt 等写进提示词首行）。
	transcriptRoleHeads = []struct {
		head  string
		agent string
	}{
		{"# Implement Agent Task", AgentImplement},
		{"# Check Agent Task", AgentCheck},
		{"# Finish Agent Task", AgentCheck},
		{"# Research Agent Task", AgentResearch},
	}
	transcriptTaskRe     = regexp.MustCompile(`\.sunshine/tasks/([^/\s"\\]+)`)
	transcriptTaskReBare = regexp.MustCompile(`(?:^|[\s"'])tasks/([^/\s"\\]+)`)
	transcriptActiveRe   = regexp.MustCompile(`Active task:\s*([^\s"\\]+)`)
)

// RunSubagentStop 处理 SubagentStop 事件，只写盘，不输出。
func RunSubagentStop(r io.Reader, opts Options) error {
	if HooksDisabled() {
		return nil
	}
	input := ReadInput(r)
	root := ResolveProjectRoot(input)
	if root == "" {
		root = rootFromWorkspaceRoots(input)
	}
	if root == "" {
		return nil
	}
	sessionID := GetString(input, "session_id", "sessionId", "sessionID")
	agent := normalizeAgentName(GetString(input, "agent_type", "agentType", "subagent_type", "subagentType"))
	transcript := GetString(input, "agent_transcript_path", "agentTranscriptPath")
	if transcript == "" {
		transcript = GetString(input, "transcript_path", "transcriptPath")
	}
	task := ""
	if transcript != "" {
		headAgent, headTask := scanTranscriptHead(transcript)
		if agent == "" {
			agent = headAgent
		}
		task = headTask
	}
	if agent == "" {
		// 不是 sunshine 子代理（Claude 内部 agent 等），忽略。
		return nil
	}
	if task == "" {
		task = resolveStopTask(root, input, agent)
	}
	RecordSubagentStop(root, shortAgentName(agent), task, sessionID)
	return nil
}

// scanTranscriptHead 读 transcript 前若干行，从注入提示词取 agent
// 与任务目录。文件不存在或读失败时返回空。
func scanTranscriptHead(path string) (agent, task string) {
	f, err := os.Open(path)
	if err != nil {
		return "", ""
	}
	defer f.Close()
	// 只读前 transcriptHeadMaxBytes 字节：角色头与任务引用都在开头。
	buf := make([]byte, transcriptHeadMaxBytes)
	n, _ := io.ReadFull(bufio.NewReader(f), buf)
	text := string(buf[:n])
	for _, h := range transcriptRoleHeads {
		if strings.Contains(text, h.head) {
			agent = h.agent
			break
		}
	}
	if agent == "" && strings.Contains(text, "sunshine-prototype") {
		agent = AgentPrototype
	}
	if m := transcriptTaskRe.FindStringSubmatch(text); m != nil {
		task = m[1]
	} else if m := transcriptTaskReBare.FindStringSubmatch(text); m != nil {
		task = m[1]
	} else if m := transcriptActiveRe.FindStringSubmatch(text); m != nil {
		task = taskDirName(m[1])
	}
	return agent, task
}

// resolveStopTask 按当前任务 + 最近未结束 start 取任务目录名；
// 取不到返回 ""（stop 仍按 agent 记录，task 留空）。
func resolveStopTask(root string, input HookInput, agent string) string {
	role := shortAgentName(agent)
	if events, err := ReadActivityEvents(root, 500); err == nil {
		if st := UnendedStart(events, role, ""); st != nil && st.Task != "" {
			return st.Task
		}
	}
	platform := DetectPlatform(input)
	active := ResolveActiveTask(root, input, platform, ResolveOptions{
		AllowSingleSessionFallback: true,
		AllowEnvironmentContext:    true,
	})
	return taskDirName(active.TaskPath)
}

// taskDirName 把任务引用（.sunshine/tasks/<名>、tasks/<名> 或 <名>）
// 归一化为任务目录名。
func taskDirName(ref string) string {
	ref = strings.TrimSpace(ref)
	ref = strings.TrimSuffix(ref, "/")
	if i := strings.LastIndex(ref, "/"); i >= 0 {
		ref = ref[i+1:]
	}
	return ref
}

// rootFromWorkspaceRoots 从 Cursor 式 payload 的 workspace_roots
// 数组取项目根（cwd 字段缺失时的退路）。
func rootFromWorkspaceRoots(input HookInput) string {
	var roots []any
	if v, ok := input["workspace_roots"].([]any); ok {
		roots = v
	} else if v, ok := input["workspaceRoots"].([]any); ok {
		roots = v
	}
	for _, r := range roots {
		if s, ok := r.(string); ok && s != "" {
			if root := FindSunshineRoot(s); root != "" {
				return root
			}
		}
	}
	return ""
}
