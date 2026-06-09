package cmd

import (
	"fmt"
	"os"
)

// checkRoot 检查是否以 root 权限运行
func checkRoot() error {
	if os.Geteuid() != 0 {
		return fmt.Errorf("此操作需要 root 权限，请使用 sudo 运行")
	}
	return nil
}
