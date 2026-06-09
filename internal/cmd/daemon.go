package cmd

import (
	"github.com/spf13/cobra"
)

func newDaemonCmd() *cobra.Command {
	return &cobra.Command{
		Use:    "daemon",
		Short:  "以守护进程模式运行（由 systemd 调用）",
		Hidden: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runDaemon()
		},
	}
}
