package cmd

import (
	"github.com/spf13/cobra"
)

func newUpdateCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "update",
		Short: "从 GitHub Release 更新 Block Area Bot",
		Long: `从 GitHub Release 下载最新版本并替换当前二进制文件。
更新前自动停止服务并禁用防火墙规则，更新后自动恢复。

默认从 beta release 更新，也可以指定版本号。`,
		RunE: func(cmd *cobra.Command, args []string) error {
			version, _ := cmd.Flags().GetString("version")
			force, _ := cmd.Flags().GetBool("force")
			return runUpdate(version, force)
		},
	}

	cmd.Flags().StringP("version", "v", "", "指定更新版本（默认更新到最新 beta）")
	cmd.Flags().BoolP("force", "f", false, "强制更新（即使版本相同）")

	return cmd
}
