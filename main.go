package main

import (
	"fmt"
	"os"

	"github.com/soulteary/block-area-bot/internal/cmd"
)

// version 通过 ldflags 注入
var version = "dev"

func main() {
	cmd.SetVersion(version)
	if err := cmd.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "错误: %v\n", err)
		os.Exit(1)
	}
}