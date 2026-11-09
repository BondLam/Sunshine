package cli

import (
	"fmt"

	"github.com/spf13/cobra"
)

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "显示 Sunshine 版本",
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Printf("Sunshine 版本 %s\n", version)
	},
}

func init() {
	rootCmd.AddCommand(versionCmd)
}
