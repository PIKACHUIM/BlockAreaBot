package cmd

import (
	"github.com/spf13/cobra"
)

func newRuleCmd() *cobra.Command {
	ruleCmd := &cobra.Command{
		Use:   "rule",
		Short: "屏蔽规则管理",
	}

	ruleCmd.AddCommand(newRuleBanCmd())
	ruleCmd.AddCommand(newRuleDelCmd())

	return ruleCmd
}

func newRuleBanCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "ban <tag>",
		Short: "添加屏蔽规则",
		Long: `基于数据源创建屏蔽规则：
  --mode black  黑名单模式（默认），阻止匹配IP访问
  --mode white  白名单模式，仅允许匹配IP访问`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			mode, _ := cmd.Flags().GetString("mode")
			port, _ := cmd.Flags().GetString("port")
			tcp, _ := cmd.Flags().GetBool("tcp")
			udp, _ := cmd.Flags().GetBool("udp")
			icmp, _ := cmd.Flags().GetBool("icmp")
			return ruleBan(args[0], mode, port, tcp, udp, icmp)
		},
	}

	cmd.Flags().String("mode", "black", "规则模式: black(黑名单) 或 white(白名单)")
	cmd.Flags().String("port", "", "端口或端口范围（如 443 或 10000-19999）")
	cmd.Flags().Bool("tcp", false, "仅匹配 TCP 协议")
	cmd.Flags().Bool("udp", false, "仅匹配 UDP 协议")
	cmd.Flags().Bool("icmp", false, "仅匹配 ICMP 协议")

	return cmd
}

func newRuleDelCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "del <tag/rule-id>",
		Short: "删除屏蔽规则",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			port, _ := cmd.Flags().GetString("port")
			tcp, _ := cmd.Flags().GetBool("tcp")
			udp, _ := cmd.Flags().GetBool("udp")
			icmp, _ := cmd.Flags().GetBool("icmp")
			return ruleDel(args[0], port, tcp, udp, icmp)
		},
	}

	cmd.Flags().String("port", "", "匹配端口或端口范围")
	cmd.Flags().Bool("tcp", false, "匹配 TCP 协议")
	cmd.Flags().Bool("udp", false, "匹配 UDP 协议")
	cmd.Flags().Bool("icmp", false, "匹配 ICMP 协议")

	return cmd
}
