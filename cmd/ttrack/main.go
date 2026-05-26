package main

import (
	"fmt"
	"os"

	"ttrack/internal/audit"
	"ttrack/internal/complete"
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
	case "ls-user":
		err = audit.LsUser(os.Args[2:])
	case "play-user":
		err = audit.PlayUser(os.Args[2:])
	case "tail":
		err = audit.Tail(os.Args[2:])
	case "tree":
		err = audit.Tree(os.Args[2:])
	case "completion":
		err = complete.Script(os.Args[2:])
	case "__complete":
		err = complete.Complete(os.Args[2:])
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
  ttrack rec [-q] [-o file] [cmd...]   record a shell session (default: $SHELL)
  ttrack play [--speed N] file         replay a local recording
  ttrack ls                            list your local recordings

audit commands (read the central root-only store; run as root):
  ttrack ls-user                       list users that have recordings
  ttrack ls-user <username>            list a user's sessions
  ttrack play-user [--speed N] <id>    replay a session by id (any user)
  ttrack tail <id>                     live-stream an in-progress session
  ttrack tree                          users -> sessions tree

  ttrack completion bash               print the bash completion script

local recordings: $TTRACK_DIR or ~/.local/share/ttrack
central store:    $TTRACK_CENTRAL_DIR or /var/lib/ttrack (root:root 0700)
format: asciinema v2 cast (.cast) — also playable with `+"`asciinema play`"+`
`)
}
