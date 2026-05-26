package main

import (
	"fmt"
	"os"

	"ttrack/internal/play"
	"ttrack/internal/record"
	"ttrack/internal/store"
)

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}

	var err error
	switch os.Args[1] {
	case "rec", "record":
		err = record.Run(os.Args[2:])
	case "play":
		err = play.Run(os.Args[2:])
	case "ls", "list":
		err = store.List(os.Args[2:])
	case "-h", "--help", "help":
		usage()
		return
	default:
		fmt.Fprintf(os.Stderr, "ttrack: unknown command %q\n\n", os.Args[1])
		usage()
		os.Exit(2)
	}

	if err != nil {
		fmt.Fprintln(os.Stderr, "ttrack:", err)
		os.Exit(1)
	}
}

func usage() {
	fmt.Fprint(os.Stderr, `ttrack — Linux terminal session tracker

usage:
  ttrack rec [-o file] [cmd...]    record a shell session (default: $SHELL)
  ttrack play [--speed N] file     replay a recorded session
  ttrack ls                        list recorded sessions

recordings stored in $TTRACK_DIR or ~/.local/share/ttrack
format: asciinema v2 cast (.cast) — also playable with `+"`asciinema play`"+`
`)
}
