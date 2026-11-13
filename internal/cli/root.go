// Package cli 定义 Sunshine 的命令行界面（全部中文）。
package cli

import (
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"
)

// version 是当前 CLI 版本；发布时由 goreleaser 经 ldflags 注入
// （-X github.com/linrong007/Sunshine/internal/cli.version=<tag>），
// 默认值 0.1.0（开发构建行为不变）。
var version = "0.1.0"

// repo 是 Sunshine 仓库地址；发布时由 goreleaser 经 ldflags 注入
// （-X github.com/linrong007/Sunshine/internal/cli.repo=<owner>/<name>）。
var repo = "github.com/linrong007/Sunshine"

// Version 返回当前 CLI 版本（供 upgrade 与 update 的最新版检查共用）。
func Version() string {
	return version
}

// Repo 返回 Sunshine 仓库地址（owner/name 形式，供最新版检查共用）。
func Repo() string {
	return repo
}

var rootCmd = &cobra.Command{
	Use:   "sunshine",
	Short: "Sunshine：面向 AI 编程的工程框架",
	Long: `Sunshine 把规范、任务与项目记忆持久化进仓库，
让 AI 编程助手按团队的工程标准工作，而不是每次从零猜测。

先用 sunshine init 初始化当前项目，再让 AI 助手按
.sunshine/workflow.md 推进任务。`,
	Version:       version,
	SilenceErrors: true,
	SilenceUsage:  true,
}

// init 接入各子命令（阶段 4：update 与生命周期命令由总协调人统一接线）。
func init() {
	RegisterUpdateCommand(rootCmd)
	RegisterUninstallCommand(rootCmd)
	RegisterAblateCommand(rootCmd)
	RegisterRestoreCommand(rootCmd)
	RegisterPlatformsCommand(rootCmd)
	RegisterMemCommand(rootCmd)
	RegisterChannelCommand(rootCmd)
	RegisterChannelForumCommand()
	RegisterUpgradeCommand(rootCmd)
	RegisterServeCommand(rootCmd)
}

// chineseHelpTemplate 是中文帮助模板（cobra 默认模板为英文）。
const chineseHelpTemplate = `{{with (or .Long .Short)}}{{. | trimTrailingWhitespaces}}

{{end}}用法：{{if .Runnable}}
  {{.UseLine}}{{end}}{{if .HasAvailableSubCommands}}
  {{.CommandPath}} [命令]{{end}}{{if gt (len .Aliases) 0}}

别名：
  {{.NameAndAliases}}{{end}}{{if .HasExample}}

示例：
{{.Example}}{{end}}{{if .HasAvailableSubCommands}}

可用命令：{{range .Commands}}{{if (or .IsAvailableCommand (eq .Name "help"))}}
  {{rpad .Name .NamePadding }} {{.Short}}{{end}}{{end}}{{end}}{{if .HasAvailableLocalFlags}}

选项：
{{.LocalFlags.FlagUsages | trimTrailingWhitespaces}}{{end}}{{if .HasAvailableInheritedFlags}}

全局选项：
{{.InheritedFlags.FlagUsages | trimTrailingWhitespaces}}{{end}}{{if .HasAvailableSubCommands}}

使用 "sunshine [命令] --help" 查看某个命令的详细帮助。{{end}}
`

// chineseUsageTemplate 是中文用法模板（出错时 cobra 打印此模板）。
const chineseUsageTemplate = `用法：{{if .Runnable}}
  {{.UseLine}}{{end}}{{if .HasAvailableSubCommands}}
  {{.CommandPath}} [命令]{{end}}{{if gt (len .Aliases) 0}}

别名：
  {{.NameAndAliases}}{{end}}{{if .HasExample}}

示例：
{{.Example}}{{end}}{{if .HasAvailableSubCommands}}

可用命令：{{range .Commands}}{{if (or .IsAvailableCommand (eq .Name "help"))}}
  {{rpad .Name .NamePadding }} {{.Short}}{{end}}{{end}}{{end}}{{if .HasAvailableLocalFlags}}

选项：
{{.LocalFlags.FlagUsages | trimTrailingWhitespaces}}{{end}}{{if .HasAvailableInheritedFlags}}

全局选项：
{{.InheritedFlags.FlagUsages | trimTrailingWhitespaces}}{{end}}{{if .HasAvailableSubCommands}}

使用 "sunshine [命令] --help" 查看某个命令的详细帮助。{{end}}
`

// translateError 把 cobra 的常见英文报错翻译成中文。
func translateError(err error) string {
	msg := err.Error()
	var name string
	if _, serr := fmt.Sscanf(msg, "unknown command %q", &name); serr == nil {
		return fmt.Sprintf("未知命令 %q，用 sunshine --help 查看可用命令", name)
	}
	if strings.HasPrefix(msg, "unknown flag: ") {
		return "未知选项：" + strings.TrimPrefix(msg, "unknown flag: ")
	}
	if strings.HasPrefix(msg, "unknown shorthand flag: ") {
		return "未知简写选项：" + strings.TrimPrefix(msg, "unknown shorthand flag: ")
	}
	return msg
}

// Execute 运行根命令，错误以中文打印到标准错误输出。
func Execute() {
	rootCmd.SetVersionTemplate("Sunshine 版本 {{.Version}}\n")
	// 统一帮助与版本选项的中文说明（cobra 默认是英文）。
	rootCmd.InitDefaultHelpFlag()
	rootCmd.InitDefaultVersionFlag()
	if f := rootCmd.Flags().Lookup("help"); f != nil {
		f.Usage = "显示帮助信息"
	}
	if f := rootCmd.Flags().Lookup("version"); f != nil {
		f.Usage = "显示 Sunshine 版本"
	}
	rootCmd.SetHelpTemplate(chineseHelpTemplate)
	rootCmd.SetUsageTemplate(chineseUsageTemplate)
	// 默认的 completion 命令是英文，禁用；help 命令改为中文。
	rootCmd.CompletionOptions.DisableDefaultCmd = true
	rootCmd.SetHelpCommand(&cobra.Command{
		Use:   "help [命令]",
		Short: "显示命令帮助信息",
		Long:  "显示指定命令的帮助信息。不指定命令时显示 sunshine 的帮助。",
		Run: func(cmd *cobra.Command, args []string) {
			target, _, err := rootCmd.Find(args)
			if err != nil || target == nil {
				target = rootCmd
			}
			_ = target.Help()
		},
	})
	for _, cmd := range rootCmd.Commands() {
		cmd.InitDefaultHelpFlag()
		if f := cmd.Flags().Lookup("help"); f != nil {
			f.Usage = "显示帮助信息"
		}
	}
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "错误：%s\n", translateError(err))
		os.Exit(1)
	}
}
