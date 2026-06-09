// Package cmd 实现 CLI 命令层，使用 cobra 框架
package cmd

import (
	"github.com/spf13/cobra"
)

var rootCmd = &cobra.Command{
	Use:   "block",
	Short: "Block Area Bot - 基于地区/IP/ASN的服务器访问屏蔽工具",
	Long: `Block Area Bot 是一个 Linux 服务 + CLI 工具，
用于基于地区、IP段、ASN 来屏蔽特定来源对服务器的访问。
底层使用 iptables + ipset 实现高效的 IP 段屏蔽。`,
}

// Execute 执行根命令
func Execute() error {
	return rootCmd.Execute()
}

func init() {
	// 添加子命令组
	rootCmd.AddCommand(newStartCmd())
	rootCmd.AddCommand(newStopCmd())
	rootCmd.AddCommand(newEnableCmd())
	rootCmd.AddCommand(newDisableCmd())
	rootCmd.AddCommand(newStatusCmd())
	rootCmd.AddCommand(newListCmd())
	rootCmd.AddCommand(newRepoCmd())
	rootCmd.AddCommand(newRuleCmd())
	rootCmd.AddCommand(newCronCmd())
	rootCmd.AddCommand(newDaemonCmd())
}
