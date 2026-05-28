package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

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

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGTERM, os.Interrupt)
	defer stop()

	sighup := make(chan os.Signal, 1)
	signal.Notify(sighup, syscall.SIGHUP)
	go func() {
		for range sighup {
			if err := logger.Reopen(cfg.LogFile); err != nil {
				logger.Warnf("ttrackd: reopen log: %v", err)
			} else {
				logger.Infof("ttrackd: log file reopened (SIGHUP)")
			}
		}
	}()

	if err := daemon.Run(ctx, cfg.SocketPath); err != nil {
		fmt.Fprintln(os.Stderr, "ttrackd:", err)
		os.Exit(1)
	}
}
