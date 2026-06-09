package cmd

import (
	"github.com/soulteary/block-area-bot/internal/daemon"
)

// runDaemon 以守护进程模式运行
func runDaemon() error {
	d, err := daemon.New()
	if err != nil {
		return err
	}
	return d.Run()
}
