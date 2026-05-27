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

	cmd := os.Args[1]
	rest := os.Args[2:]

	// `ttrack help [command]` — overall usage, or one command's help.
	if cmd == "help" || cmd == "-h" || cmd == "--help" {
		if len(rest) > 0 {
			if h, ok := commandHelp(rest[0]); ok {
				fmt.Print(h)
				return
			}
			fmt.Fprintf(os.Stderr, "ttrack: no help for %q\n\n", rest[0])
			usage()
			os.Exit(2)
		}
		usage()
		return
	}
	// `ttrack <command> help|-h|--help` — that command's help.
	if len(rest) > 0 && isHelpToken(rest[0]) {
		if h, ok := commandHelp(cmd); ok {
			fmt.Print(h)
			return
		}
	}

	var err error
	switch cmd {
	case "rec", "record":
		err = record.Run(rest)
	case "play":
		err = play.Run(rest)
	case "ls", "list":
		err = store.List(rest)
	case "ls-user":
		err = audit.LsUser(rest)
	case "play-user":
		err = audit.PlayUser(rest)
	case "tail":
		err = audit.Tail(rest)
	case "tree":
		err = audit.Tree(rest)
	case "search":
		err = audit.Search(rest)
	case "export":
		err = audit.Export(rest)
	case "prune":
		err = audit.Prune(rest)
	case "completion":
		err = complete.Script(rest)
	case "__complete":
		err = complete.Complete(rest)
	default:
		fmt.Fprintf(os.Stderr, "ttrack: unknown command %q\n\n", cmd)
		usage()
		os.Exit(2)
	}

	if err != nil {
		fmt.Fprintln(os.Stderr, "ttrack:", err)
		os.Exit(1)
	}
}

func isHelpToken(s string) bool {
	return s == "help" || s == "-h" || s == "--help"
}

func usage() {
	fmt.Fprint(os.Stderr, `ttrack — Linux terminal session tracker

usage:
  ttrack rec [-q] [-o file] [cmd...]   record a shell session (default: $SHELL)
  ttrack play [--speed N] file         replay a local recording
  ttrack ls                            list your local recordings

audit commands (read the central root-only store; run as root):
  ttrack ls-user [username]            list users, or one user's sessions
  ttrack play-user [--speed N] <id>    replay a session by id (any user)
  ttrack tail <id>                     live-stream an in-progress session
  ttrack tree                          users -> sessions tree
  ttrack search [opts] <string>        find a string across recordings
  ttrack export [-o file] <id>         decrypt a session to a plaintext cast
  ttrack prune                         interactively delete recordings (by user/time)

  ttrack completion bash               print the bash completion script

search opts: --from / --to <YYYY-MM-DD[ HH:MM]>, --user <name>, -i
recordings in the central store are encrypted at rest (opaque to cat/strings)

local recordings: $TTRACK_DIR or ~/.local/share/ttrack
central store:    $TTRACK_CENTRAL_DIR or /var/lib/ttrack (root:root 0700)
format: asciinema v2 cast (.cast) — also playable with `+"`asciinema play`"+`

run 'ttrack help <command>' (or 'ttrack <command> --help') for command details
`)
}

// commandHelp returns detailed help text for one command. The second result is
// false for an unknown command.
func commandHelp(name string) (string, bool) {
	switch name {
	case "rec", "record":
		return `ttrack rec — record a terminal session

usage: ttrack rec [-q] [-o file] [cmd...]

Runs cmd (or $SHELL, default /bin/bash, when none is given) under a PTY and
records its output as an asciinema v2 cast. Streams to the ttrackd daemon when
reachable, otherwise writes a user-local file (fail-open).

options:
  -o file   write the recording to file (implies local, bypasses the daemon)
  -q        quiet: suppress the banner and saved-path message (also TTRACK_QUIET=1)
`, true
	case "play":
		return `ttrack play — replay a recording

usage: ttrack play [--speed N] [--idle N] <file|id>

Resolves a path, a local 'ttrack ls' id, or — as root — a central-store
session id (like play-user). Replays with the original timing.

options:
  --speed N   playback multiplier (default 1.0; >1 faster, <1 slower)
  --idle N    cap idle gaps to N seconds (default 0 = exact timing);
              ignored in interactive mode

on a terminal this opens a full-screen player (thin transport bar; holds the
final frame until you quit). controls:
  space pause/resume    left/right or h/l seek 5s    up/down or +/- speed
  g jump to a recorded command (list; t = type a time)  0 restart
  b hide/show the bar (full-height playback)            q or Ctrl-C quit
  click the bar to seek (Shift+click to select text)
`, true
	case "ls", "list":
		return `ttrack ls — list your local recordings

usage: ttrack ls

Lists recordings in $TTRACK_DIR (default ~/.local/share/ttrack).
Columns: STATUS, FILE, STARTED, DURATION, COMMAND.
`, true
	case "ls-user":
		return `ttrack ls-user — list central-store users or sessions (root)

usage: ttrack ls-user [username]

With no argument, lists users that have recordings and their session counts.
With a username, lists that user's sessions. Columns: STATUS, TYPE, SESSION,
STARTED, DURATION, COMMAND. TYPE is interactive or non-interactive.
`, true
	case "play-user":
		return `ttrack play-user — replay any user's session by id (root)

usage: ttrack play-user [--speed N] [--idle N] <sessionid>

Searches all users in the central store for the id. Same options and
interactive controls as 'ttrack play'.
`, true
	case "tail":
		return `ttrack tail — live-stream an in-progress session (root)

usage: ttrack tail <sessionid>

Streams a running session's output from the daemon as it happens.
`, true
	case "tree":
		return `ttrack tree — central store as a users -> sessions tree (root)

usage: ttrack tree

Each session shows [STATUS TYPE], start time, duration, and command.
`, true
	case "search":
		return `ttrack search — find a string across recordings (root)

usage: ttrack search [--from T] [--to T] [--user U] [-i] [--all] <pattern>

Searches recorded commands and output. Prints the owning user, start time,
command, and matching output lines.

options:
  --from T   only sessions started at/after T (YYYY-MM-DD[ HH:MM])
  --to T     only sessions started at/before T
  --user U   restrict to one user
  -i         case-insensitive match
  --all      list every session (no pattern needed)
`, true
	case "export":
		return `ttrack export — decrypt a session to a plaintext cast (root)

usage: ttrack export [-o file] <sessionid>

Writes a plaintext asciinema v2 cast, playable with 'asciinema play'.

options:
  -o file   output file (default: stdout)
`, true
	case "prune":
		return `ttrack prune — interactively delete recordings (root)

usage: ttrack prune [--yes]

Shows a storage overview, asks which user(s) and what to delete
(all / days N / range FROM TO), previews the targets, and confirms.
Requires the prune password (set on first use). Never deletes active sessions.

options:
  --yes   skip the final confirmation prompt
`, true
	case "completion":
		return `ttrack completion — print the shell completion script

usage: ttrack completion bash

Install:
  ttrack completion bash | sudo tee /usr/share/bash-completion/completions/ttrack
`, true
	}
	return "", false
}
