package cmd

import (
	"github.com/spf13/cobra"
)

func newRepoCmd() *cobra.Command {
	repoCmd := &cobra.Command{
		Use:   "repo",
		Short: "数据源管理",
	}

	repoCmd.AddCommand(newRepoAddCmd())
	repoCmd.AddCommand(newRepoDelCmd())
	repoCmd.AddCommand(newRepoListCmd())

	return repoCmd
}

func newRepoAddCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "add [url/path]",
		Short: "添加数据源",
		Long: `添加 IP 段数据源，支持以下类型：
  --type ipv4       每行一个 CIDR 的 IPv4 段文件
  --type ipv6       每行一个 CIDR 的 IPv6 段文件
  --type apnic:XX   APNIC delegated 格式，XX 为国家代码（如 cn）`,
		RunE: func(cmd *cobra.Command, args []string) error {
			repoType, _ := cmd.Flags().GetString("type")
			tag, _ := cmd.Flags().GetString("tag")
			var source string
			if len(args) > 0 {
				source = args[0]
			}
			return repoAdd(repoType, tag, source)
		},
	}

	cmd.Flags().StringP("type", "t", "", "数据源类型: ipv4, ipv6, apnic:XX")
	cmd.Flags().String("tag", "", "数据源标签（可选，不指定则自动命名）")
	_ = cmd.MarkFlagRequired("type")

	return cmd
}

func newRepoDelCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "del <tag/repo-id>",
		Short: "删除数据源",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return repoDel(args[0])
		},
	}
}

func newRepoListCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "查看所有数据源",
		RunE: func(cmd *cobra.Command, args []string) error {
			return repoList()
		},
	}
}
