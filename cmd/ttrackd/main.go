package main

import (
	"fmt"
	"os"

	"ttrack/internal/config"
	"ttrack/internal/daemon"
)

func main() {
	sock := os.Getenv("TTRACKD_SOCK")
	if sock == "" {
		sock = config.Load().SocketPath
	}
	if err := daemon.Run(sock); err != nil {
		fmt.Fprintln(os.Stderr, "ttrackd:", err)
		os.Exit(1)
	}
}
