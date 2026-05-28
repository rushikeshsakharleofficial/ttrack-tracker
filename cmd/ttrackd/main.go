package main

import (
	"fmt"
	"os"

	"ttrack/internal/config"
	"ttrack/internal/daemon"
	"ttrack/internal/logger"
)

func main() {
	cfg := config.Load()
	logger.Set(logger.Level(cfg.LogLevel))

	if err := daemon.Run(cfg.SocketPath); err != nil {
		fmt.Fprintln(os.Stderr, "ttrackd:", err)
		os.Exit(1)
	}
}
