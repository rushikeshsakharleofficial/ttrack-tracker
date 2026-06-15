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

	// reload carries freshly-parsed configs to the daemon on SIGHUP so it can
	// apply the hot-reloadable fields (log level, session cap) without a restart.
	reload := make(chan *config.Config, 1)
	sighup := make(chan os.Signal, 1)
	signal.Notify(sighup, syscall.SIGHUP)
	go func() {
		for range sighup {
			// Parse fresh config first so Reopen uses the new log_file path.
			nc := config.Parse()
			if err := logger.Reopen(nc.LogFile); err != nil {
				logger.Warnf("ttrackd: reopen log: %v", err)
			} else {
				logger.Infof("ttrackd: log file reopened (SIGHUP)")
			}
			// Hand the fresh config to the daemon. Drop if a prior reload is pending.
			select {
			case reload <- nc:
			default:
			}
		}
	}()

	if err := daemon.Run(ctx, cfg, reload); err != nil {
		fmt.Fprintln(os.Stderr, "ttrackd:", err)
		os.Exit(1)
	}
}
