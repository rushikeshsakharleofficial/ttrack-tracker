package main

import (
	"fmt"
	"os"

	"ttrack/internal/daemon"
)

const defaultSocket = "/run/ttrackd.sock"

func main() {
	sock := os.Getenv("TTRACKD_SOCK")
	if sock == "" {
		sock = defaultSocket
	}
	if err := daemon.Run(sock); err != nil {
		fmt.Fprintln(os.Stderr, "ttrackd:", err)
		os.Exit(1)
	}
}
