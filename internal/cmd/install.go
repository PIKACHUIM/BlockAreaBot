package cmd

import (
	"github.com/spf13/cobra"
)

func newInstallCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "install",
		Short: "安装 Block Area Bot 系统服务",
		Long:  "安装 systemd 服务文件、创建配置目录和数据目录，使 block-area-bot 可作为系统服务运行。",
		RunE: func(cmd *cobra.Command, args []string) error {
			return installService()
		},
	}
}

func newUninstallCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "uninstall",
		Short: "卸载 Block Area Bot 服务",
		Long: `卸载 block-area-bot 系统服务。
默认只卸载服务（停止服务、移除 systemd 文件、清除防火墙规则）。
使用 --all 同时卸载 CLI 二进制文件、配置和数据。`,
		RunE: func(cmd *cobra.Command, args []string) error {
			all, _ := cmd.Flags().GetBool("all")
			return uninstallService(all)
		},
	}
	cmd.Flags().Bool("all", false, "卸载全部（包括 CLI 二进制、配置和数据目录）")
	return cmd
}
