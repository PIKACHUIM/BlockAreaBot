package main

import (
	"fmt"
	"os"

	"github.com/soulteary/block-area-bot/internal/cmd"
)

func main() {
	if err := cmd.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "错误: %v\n", err)
		os.Exit(1)
	}
}