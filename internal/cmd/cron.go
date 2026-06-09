package cmd

import (
	"github.com/spf13/cobra"
)

func newCronCmd() *cobra.Command {
	cronCmd := &cobra.Command{
		Use:   "cron",
		Short: "定时任务管理",
	}

	cronCmd.AddCommand(newCronAddCmd())
	cronCmd.AddCommand(newCronDelCmd())
	cronCmd.AddCommand(newCronListCmd())

	return cronCmd
}

func newCronAddCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "add <tag> <interval>",
		Short: "添加定时更新任务",
		Long: `为数据源设置定时更新周期：
  interval 格式: 30m(30分钟), 1h(1小时), 3d(3天)`,
		Args: cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			return cronAdd(args[0], args[1])
		},
	}
}

func newCronDelCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "del <tag>",
		Short: "删除定时更新任务",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return cronDel(args[0])
		},
	}
}

func newCronListCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "显示所有定时任务",
		RunE: func(cmd *cobra.Command, args []string) error {
			return cronList()
		},
	}
}
