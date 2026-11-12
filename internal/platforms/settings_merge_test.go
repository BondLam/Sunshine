package platforms

import (
	"encoding/json"
	"strings"
	"testing"
)

const mergeOldSettings = `{
  "env": {
    "CLAUDE_BASH_MAINTAIN_PROJECT_WORKING_DIR": "1"
  },
  "hooks": {
    "PreToolUse": [
      {
        "hooks": [
          {
            "command": "SUNSHINE_HOOK_PLATFORM=claude sunshine hook subagent-context",
            "timeout": 30,
            "type": "command"
          }
        ],
        "matcher": "Task"
      }
    ],
    "Stop": [
      {
        "hooks": [
          {
            "command": "my-notify",
            "timeout": 5,
            "type": "command"
          }
        ]
      }
    ]
  }
}
`

const mergeFreshSettings = `{
  "env": {
    "CLAUDE_BASH_MAINTAIN_PROJECT_WORKING_DIR": "1"
  },
  "hooks": {
    "PreToolUse": [
      {
        "hooks": [
          {
            "command": "SUNSHINE_HOOK_PLATFORM=claude sunshine hook subagent-context",
            "timeout": 30,
            "type": "command"
          }
        ],
        "matcher": "Task"
      }
    ],
    "SubagentStop": [
      {
        "hooks": [
          {
            "command": "SUNSHINE_HOOK_PLATFORM=claude sunshine hook subagent-stop",
            "timeout": 30,
            "type": "command"
          }
        ],
        "matcher": "sunshine-implement|sunshine-check|sunshine-research|sunshine-prototype"
      }
    ]
  }
}
`

func TestMergeMissingSettingsHooks(t *testing.T) {
	merged, err := MergeMissingSettingsHooks([]byte(mergeOldSettings), []byte(mergeFreshSettings))
	if err != nil {
		t.Fatal(err)
	}
	s := string(merged)
	// 缺失的 SubagentStop 应补上。
	if !strings.Contains(s, "SUNSHINE_HOOK_PLATFORM=claude sunshine hook subagent-stop") {
		t.Errorf("合并后应补上 SubagentStop：\n%s", s)
	}
	// 用户已有配置一行不动：自加的 Stop hook 与 env 都在。
	if !strings.Contains(s, `"my-notify"`) {
		t.Errorf("用户自加 hook 应保留：\n%s", s)
	}
	if !strings.Contains(s, "CLAUDE_BASH_MAINTAIN_PROJECT_WORKING_DIR") {
		t.Errorf("用户已有 env 应保留：\n%s", s)
	}
	// 已有的组不重复：subagent-context 只出现一次。
	if n := strings.Count(s, "sunshine hook subagent-context"); n != 1 {
		t.Errorf("已有 hook 组不应重复，subagent-context 出现 %d 次：\n%s", n, s)
	}
	// 合法 JSON，末尾换行。
	var v map[string]any
	if err := json.Unmarshal(merged, &v); err != nil {
		t.Errorf("合并结果应为合法 JSON：%v\n%s", err, s)
	}
	if !strings.HasSuffix(s, "\n") {
		t.Errorf("合并结果应以换行结尾")
	}
}

func TestMergeMissingSettingsHooksNoDup(t *testing.T) {
	// 已有 SubagentStop 时合并应是 no-op（只补缺失，不动已有）。
	merged, err := MergeMissingSettingsHooks([]byte(mergeFreshSettings), []byte(mergeFreshSettings))
	if err != nil {
		t.Fatal(err)
	}
	if n := strings.Count(string(merged), "subagent-stop"); n != 1 {
		t.Errorf("已有 SubagentStop 不应重复，出现 %d 次：\n%s", n, merged)
	}
}

func TestMergeMissingSettingsHooksBadJSON(t *testing.T) {
	if _, err := MergeMissingSettingsHooks([]byte("{ not json"), []byte(mergeFreshSettings)); err == nil {
		t.Error("已有文件非法时应返回错误（调用方回落到模板全文）")
	}
	if _, err := MergeMissingSettingsHooks([]byte(mergeOldSettings), []byte("[1,2]")); err == nil {
		t.Error("模板顶层非对象时应返回错误")
	}
}
