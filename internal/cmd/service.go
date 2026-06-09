package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
)

func newStartCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "start",
		Short: "启动屏蔽服务",
		RunE: func(cmd *cobra.Command, args []string) error {
			fmt.Println("正在启动 block-area-bot 服务...")
			return serviceAction("start")
		},
	}
}

func newStopCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "stop",
		Short: "停止屏蔽服务",
		RunE: func(cmd *cobra.Command, args []string) error {
			fmt.Println("正在停止 block-area-bot 服务...")
			return serviceAction("stop")
		},
	}
}

func newEnableCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "enable",
		Short: "设置开机自启动",
		RunE: func(cmd *cobra.Command, args []string) error {
			fmt.Println("正在设置开机自启动...")
			return serviceAction("enable")
		},
	}
}

func newDisableCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "disable",
		Short: "关闭开机自启动",
		RunE: func(cmd *cobra.Command, args []string) error {
			fmt.Println("正在关闭开机自启动...")
			return serviceAction("disable")
		},
	}
}

func newStatusCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "显示服务状态",
		RunE: func(cmd *cobra.Command, args []string) error {
			return showStatus()
		},
	}
}

func newListCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "显示服务状态、源、规则、定时任务总览",
		RunE: func(cmd *cobra.Command, args []string) error {
			return showList()
		},
	}
}
