package lifecycle

import (
	"strings"
	"testing"

	"github.com/linrong007/Sunshine/internal/platforms"
)

// 滚动轨道新增清理器与检测逻辑的测试。

func TestScrubZcodeConfigJSON(t *testing.T) {
	content := `{
  "hooks": {
    "enabled": true,
    "events": {
      "SessionStart": [
        {
          "hooks": [
            {
              "command": "SUNSHINE_HOOK_PLATFORM=zcode sunshine hook session-start",
              "timeout": 30,
              "type": "command"
            }
          ],
          "matcher": "startup|clear|compact"
        }
      ],
      "UserPromptSubmit": [
        {
          "hooks": [
            {
              "command": "my-custom-hook",
              "timeout": 5,
              "type": "command"
            }
          ]
        }
      ]
    }
  },
  "customKey": "keep-me"
}
`
	got := scrubZcodeConfigJSON(content)
	if got.FullyEmpty {
		t.Error("zcode config 清理后不应为空（有用户键）")
	}
	if strings.Contains(got.Content, "sunshine hook") {
		t.Error("zcode config 清理后仍含 Sunshine hook")
	}
	if !strings.Contains(got.Content, "my-custom-hook") || !strings.Contains(got.Content, "keep-me") {
		t.Error("zcode config 清理丢了用户内容")
	}
	if !strings.Contains(got.Content, "UserPromptSubmit") {
		t.Error("zcode config 清理丢了非 Sunshine 事件")
	}
	// 全部是 Sunshine hook 时整个 hooks 键删掉，文件为空。
	onlySunshine := `{"hooks": {"enabled": true, "events": {"SessionStart": [{"hooks": [{"command": "SUNSHINE_HOOK_PLATFORM=zcode sunshine hook session-start"}]}]}}}`
	got = scrubZcodeConfigJSON(onlySunshine)
	if !got.FullyEmpty {
		t.Errorf("纯 Sunshine 的 zcode config 清理后应为空，得到 %q", got.Content)
	}
}

func TestScrubSnowHookJSON(t *testing.T) {
	content := `{
  "onSessionStart": [
    {
      "description": "Sunshine 会话启动",
      "hooks": [
        {
          "command": "SUNSHINE_HOOK_PLATFORM=snow sunshine hook session-start",
          "enabled": true,
          "timeout": 20000,
          "type": "command"
        },
        {
          "command": "my-hook",
          "enabled": true,
          "timeout": 1000,
          "type": "command"
        }
      ]
    }
  ]
}
`
	got := scrubSnowHookJSON(content)
	if got.FullyEmpty {
		t.Error("snow hook 清理后不应为空（有用户 hook）")
	}
	if strings.Contains(got.Content, "sunshine hook") {
		t.Error("snow hook 清理后仍含 Sunshine hook")
	}
	if !strings.Contains(got.Content, "my-hook") {
		t.Error("snow hook 清理丢了用户 hook")
	}
}

func TestScrubOpencodePackageJSON(t *testing.T) {
	content := `{
  "dependencies": {
    "@opencode-ai/plugin": "^1.14.39",
    "other-lib": "^2.0.0"
  },
  "scripts": {
    "test": "echo hi"
  }
}
`
	got := scrubOpencodePackageJSON(content)
	if got.FullyEmpty {
		t.Error("opencode package.json 清理后不应为空")
	}
	if strings.Contains(got.Content, "@opencode-ai/plugin") {
		t.Error("opencode package.json 清理后仍含 Sunshine 依赖")
	}
	if !strings.Contains(got.Content, "other-lib") || !strings.Contains(got.Content, "echo hi") {
		t.Error("opencode package.json 清理丢了用户内容")
	}
	// 只有 Sunshine 依赖时文件为空。
	only := `{"dependencies": {"@opencode-ai/plugin": "^1.14.39"}}`
	if got := scrubOpencodePackageJSON(only); !got.FullyEmpty {
		t.Errorf("纯 Sunshine 的 package.json 清理后应为空，得到 %q", got.Content)
	}
}

func TestScrubPiSettingsJSON(t *testing.T) {
	content := `{
  "enableSkillCommands": true,
  "extensions": [
    "./extensions/sunshine/index.ts",
    "./extensions/mine/index.ts"
  ],
  "prompts": [
    "./prompts"
  ],
  "theme": "dark"
}
`
	got := scrubPiSettingsJSON(content)
	if got.FullyEmpty {
		t.Error("pi settings 清理后不应为空")
	}
	if strings.Contains(got.Content, "sunshine/index.ts") || strings.Contains(got.Content, "enableSkillCommands") || strings.Contains(got.Content, `"./prompts"`) {
		t.Error("pi settings 清理后仍含 Sunshine 注册")
	}
	if !strings.Contains(got.Content, "mine/index.ts") || !strings.Contains(got.Content, "dark") {
		t.Error("pi settings 清理丢了用户内容")
	}
	// 全部是 Sunshine 注册时文件为空。
	only := `{"enableSkillCommands": true, "extensions": ["./extensions/sunshine/index.ts"], "prompts": ["./prompts"]}`
	if got := scrubPiSettingsJSON(only); !got.FullyEmpty {
		t.Errorf("纯 Sunshine 的 pi settings 清理后应为空，得到 %q", got.Content)
	}
}

func TestConfiguredPlatformIDsNewPlatforms(t *testing.T) {
	dir := t.TempDir()
	writeSunshineSkeleton(t, dir, "tester")
	// 装 antigravity（configDir 为 .agent/workflows）与 snow
	//（configDir 为 .snow/skills），两者都应被认出。
	for _, id := range []string{"antigravity", "snow"} {
		files, err := platforms.Install(dir, id, platforms.InstallOptions{})
		if err != nil {
			t.Fatal(err)
		}
		recordHashes(t, dir, files)
	}
	hashes, _ := LoadHashes(dir)
	got := ConfiguredPlatformIDs(hashes)
	want := []string{"snow", "antigravity"}
	// 注册表顺序：snow 在 antigravity 之前（snow 先于 kilo/antigravity/devin 登记）。
	if len(got) != 2 || got[0] != want[0] || got[1] != want[1] {
		t.Errorf("已配置平台 = %v，想得到 %v", got, want)
	}
	// 原生 .snow 目录（清单里无记录）不应误判：snow 只认
	// .snow/skills 下有 Sunshine 模板哈希的情形。
	dir2 := t.TempDir()
	writeSunshineSkeleton(t, dir2, "tester")
	hashes2, _ := LoadHashes(dir2)
	if got := ConfiguredPlatformIDs(hashes2); len(got) != 0 {
		t.Errorf("只初始化骨架时已配置平台 = %v，想得到空", got)
	}
}

func TestConfiguredPlatformIDsDevinLegacy(t *testing.T) {
	hashes := map[string]string{
		".windsurf/workflows/sunshine-start.md": "abc",
	}
	got := ConfiguredPlatformIDs(hashes)
	found := false
	for _, id := range got {
		if id == "devin" {
			found = true
		}
	}
	if !found {
		t.Errorf(".windsurf/workflows/sunshine-* 在清单里时应判定 devin 已配置，得到 %v", got)
	}
	// 原生 Windsurf 目录（无 sunshine- 前缀）不算。
	hashes = map[string]string{
		".windsurf/workflows/my-workflow.md": "abc",
	}
	for _, id := range ConfiguredPlatformIDs(hashes) {
		if id == "devin" {
			t.Error("原生 Windsurf 工作流不应判定 devin 已配置")
		}
	}
}

func TestAllManagedDirsCoverNewPlatforms(t *testing.T) {
	covered := func(path string) bool {
		for _, d := range AllManagedDirs {
			if path == d || strings.HasPrefix(path, d+"/") {
				return true
			}
		}
		return false
	}
	for _, p := range platforms.Registry() {
		if !covered(p.ConfigDir) {
			t.Errorf("AllManagedDirs 未覆盖平台 %s 的 ConfigDir %s", p.ID, p.ConfigDir)
		}
		for _, extra := range p.ExtraManagedPaths {
			if !covered(extra) {
				t.Errorf("AllManagedDirs 未覆盖平台 %s 的 ExtraManagedPaths %s", p.ID, extra)
			}
		}
	}
}
