package workflow

import (
	"os"
	"path/filepath"
	"regexp"
	"sort"
)

// 判定方式与 Trellis utils/agent-refs.ts 一致：纯词法扫描，宁可误报
// 也不漏报。两种表面形式：
//  1. `sunshine channel spawn ... --agent <名称>` 里的 --agent 参数
//     （上游是 trellis channel spawn --agent；Go 版命令名前缀已改名）；
//  2. `.sunshine/agents/<名称>.md` 的字面路径引用。
//
// 名称字符集与 agent-loader 的 SAFE_AGENT_NAME 一致：[A-Za-z0-9._-]。
var (
	agentFlagRe = regexp.MustCompile(`--agent[\s=]+([A-Za-z0-9._-]+)`)
	agentPathRe = regexp.MustCompile(`\.sunshine/agents/([A-Za-z0-9._-]+)\.md`)
)

// CollectReferencedAgents 提取工作流正文引用的 agent 名（排序、去重）。
func CollectReferencedAgents(workflowContent string) []string {
	found := map[string]bool{}
	for _, re := range []*regexp.Regexp{agentFlagRe, agentPathRe} {
		for _, m := range re.FindAllStringSubmatch(workflowContent, -1) {
			if m[1] != "" {
				found[m[1]] = true
			}
		}
	}
	out := make([]string, 0, len(found))
	for name := range found {
		out = append(out, name)
	}
	sort.Strings(out)
	return out
}

// CollectMissingAgents 返回被引用但磁盘上没有的 agent 名。
// 存在形式有两种：`.sunshine/agents/<名>.md` 或
// `.sunshine/agents/<名>/AGENT.md`（与上游一致）。
func CollectMissingAgents(cwd, workflowContent string) []string {
	referenced := CollectReferencedAgents(workflowContent)
	if len(referenced) == 0 {
		return nil
	}
	agentsRoot := filepath.Join(cwd, ".sunshine", "agents")
	var missing []string
	for _, name := range referenced {
		file := filepath.Join(agentsRoot, name+".md")
		nested := filepath.Join(agentsRoot, name, "AGENT.md")
		if _, err := os.Stat(file); err == nil {
			continue
		}
		if _, err := os.Stat(nested); err == nil {
			continue
		}
		missing = append(missing, name)
	}
	return missing
}
