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
	closeLog, err := logger.TeeToFile(cfg.LogFile)
	if err != nil {
		fmt.Fprintln(os.Stderr, "ttrackd: open log file:", err)
		os.Exit(1)
	}
	defer closeLog()

	if err := daemon.Run(cfg.SocketPath); err != nil {
		fmt.Fprintln(os.Stderr, "ttrackd:", err)
		os.Exit(1)
	}
}
