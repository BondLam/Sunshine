package cli

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/linrong007/Sunshine/internal/lifecycle"
	"github.com/spf13/cobra"
)

// RegisterPlatformsCommand 把 `sunshine platforms` 接到根命令上。
// 由总协调人统一接线（不要改 root.go，也不要用 init() 自注册）。
func RegisterPlatformsCommand(root *cobra.Command) {
	var asJSON bool
	cmd := &cobra.Command{
		Use:   "platforms",
		Short: "显示当前项目已配置的 AI 平台",
		Long: `显示当前项目已配置的 AI 平台。

按 .sunshine/.template-hashes.json 判断：清单里有该平台
配置目录下条目的即为已配置（避免把原生平台目录误判为已配置）。
每平台输出 id、displayName、configDir。

示例：
  sunshine platforms
  sunshine platforms --json`,
		RunE: func(cmd *cobra.Command, args []string) error {
			cwd, err := os.Getwd()
			if err != nil {
				return err
			}
			infos, err := lifecycle.ConfiguredPlatforms(cwd)
			if err != nil {
				return err
			}
			if asJSON {
				out, err := json.MarshalIndent(map[string]any{"platforms": infos}, "", "  ")
				if err != nil {
					return err
				}
				fmt.Println(string(out))
				return nil
			}
			if len(infos) == 0 {
				fmt.Println("当前项目未配置任何平台。")
				return nil
			}
			fmt.Println("已配置的平台：")
			for _, p := range infos {
				fmt.Printf("  %s (%s) — %s\n", p.DisplayName, p.ID, p.ConfigDir)
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&asJSON, "json", false, "输出机器可读的 JSON")
	root.AddCommand(cmd)
}
