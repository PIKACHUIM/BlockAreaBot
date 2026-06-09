// Package cmd 实现 CLI 命令层，使用 cobra 框架
package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
)

// Version 版本号，通过 ldflags 注入
var Version = "dev"

var rootCmd = &cobra.Command{
	Use:   "block",
	Short: "Block Area Bot - 基于地区/IP/ASN的服务器访问屏蔽工具",
	Long: `Block Area Bot 是一个 Linux 服务 + CLI 工具，
用于基于地区、IP段、ASN 来屏蔽特定来源对服务器的访问。
底层使用 iptables + ipset 实现高效的 IP 段屏蔽。`,
	Version: Version,
}

// Execute 执行根命令
func Execute() error {
	return rootCmd.Execute()
}

// SetVersion 设置版本号（由 main 包调用）
func SetVersion(v string) {
	Version = v
	rootCmd.Version = v
}

func init() {
	rootCmd.SetVersionTemplate(fmt.Sprintf("Block Area Bot %s\n", Version))
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
	rootCmd.AddCommand(newUpdateCmd())
}
