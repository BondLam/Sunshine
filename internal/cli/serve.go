package cli

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/linrong007/Sunshine/internal/serve"
	"github.com/linrong007/Sunshine/internal/task"
)

// RegisterServeCommand 把 `sunshine serve` 接到根命令上。
func RegisterServeCommand(root *cobra.Command) {
	var port int
	var interval int
	var addr string

	serveCmd := &cobra.Command{
		Use:   "serve",
		Short: "启动本机只读观测后台（只看不操作）",
		Long: `启动本机只读观测后台，在浏览器里查看当前项目的任务、Channel、
会话、项目健康与设计文档。

只读：不提供任何写操作；默认只绑本机地址 127.0.0.1，不设账号密码；
前台运行，按 Ctrl+C 停止。必须在 Sunshine 项目目录（或其子目录）里运行。`,
		Example: `  sunshine serve
  sunshine serve --port 18080
  sunshine serve --interval 5`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if interval < 1 || interval > 30 {
				return fmt.Errorf("--interval 须在 1–30 秒之间，当前为 %d", interval)
			}
			if port < 1 || port > 65535 {
				return fmt.Errorf("--port 须在 1–65535 之间，当前为 %d", port)
			}
			if addr != "127.0.0.1" {
				return fmt.Errorf("只支持绑定本机地址 127.0.0.1（当前 --addr %s）；对外暴露需等 license 鉴权落地后再开启", addr)
			}
			root := task.FindRoot("")
			if root == "" {
				return fmt.Errorf("当前目录不在 Sunshine 项目内（未找到 .sunshine/），请先 cd 到项目目录，或运行 sunshine init 初始化")
			}
			srv := serve.NewServer(root, interval, Version(), Repo())
			fmt.Printf("Sunshine 只读观测后台\n")
			fmt.Printf("项目：%s\n", root)
			fmt.Printf("地址：http://%s:%d\n", addr, port)
			fmt.Printf("轮询间隔：%d 秒（按 Ctrl+C 停止）\n", interval)
			if err := srv.Run(addr, port); err != nil {
				return err
			}
			return nil
		},
	}
	serveCmd.Flags().IntVar(&port, "port", 17890, "监听端口（被占时报错，不自动跳端口）")
	serveCmd.Flags().IntVar(&interval, "interval", 3, "轮询间隔（秒，1–30）")
	serveCmd.Flags().StringVar(&addr, "addr", "127.0.0.1", "绑定地址（只支持 127.0.0.1）")
	root.AddCommand(serveCmd)
}
