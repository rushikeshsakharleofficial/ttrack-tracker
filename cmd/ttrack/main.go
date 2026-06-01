package main

import (
	"fmt"
	"os"
	"strings"

	"ttrack/internal/ansible"
	"ttrack/internal/audit"
	"ttrack/internal/auth"
	"ttrack/internal/backup"
	"ttrack/internal/complete"
	"ttrack/internal/config"
	"ttrack/internal/initcmd"
	"ttrack/internal/play"
	"ttrack/internal/record"
	"ttrack/internal/store"
)

// Version is set at build time via -ldflags "-X main.Version=x.y.z".
var Version = "dev"

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}

	cmd := os.Args[1]
	rest := os.Args[2:]

	if cmd == "version" || cmd == "-V" || cmd == "--version" {
		fmt.Println("ttrack", Version)
		return
	}

	if cmd == "-c" || cmd == "--check" {
		os.Exit(runConfigCheck())
	}

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
	// Also handle `ttrack ansible list --help` etc.
	if len(rest) > 0 && isHelpToken(rest[0]) {
		if h, ok := commandHelp(cmd); ok {
			fmt.Print(h)
			return
		}
	}
	if cmd == "ansible" && len(rest) > 0 && len(rest) > 1 && isHelpToken(rest[1]) {
		if h, ok := commandHelp(cmd); ok {
			fmt.Print(h)
			return
		}
	}

	var err error
	switch cmd {
	case "init":
		err = initcmd.Run(rest)
	case "rec", "record":
		err = record.Run(rest)
	case "play":
		// Extract target first — needed for ansible pre-check before password prompt.
		target := lastNonFlagArg(rest)
		// Pre-check: if target is not a local file, see if it's an ansible run ID.
		// Give a helpful redirect before the password prompt blocks a TTY.
		if target != "" {
			if _, serr := os.Stat(target); serr != nil {
				if store.IsAnsibleRun(target) {
					fmt.Fprintf(os.Stderr, "ttrack: %q is an Ansible run — use: ttrack ansible show %s\n", target, target)
					os.Exit(1)
				}
			}
		}
		// Password gate — prompt if playback password is set.
		if perr := auth.PromptAndVerify(); perr != nil {
			fmt.Fprintln(os.Stderr, "ttrack:", perr)
			os.Exit(1)
		}
		// Auto-detect: existing local file → local play; otherwise → central store ID.
		if target != "" {
			if _, serr := os.Stat(target); serr == nil {
				err = play.Run(rest)
			} else {
				err = audit.PlayUser(rest)
			}
		} else {
			err = play.Run(rest)
		}
	case "ls", "list":
		// --all → all users in central store; --user <name> → that user; (none) → local.
		hasAll, userVal := parseLsScope(rest)
		if hasAll {
			err = audit.LsUser(nil)
		} else if userVal != "" {
			err = audit.LsUser([]string{userVal})
		} else {
			err = store.List(rest)
		}
	// Hidden backward-compat aliases — not shown in usage.
	case "ls-user":
		err = audit.LsUser(rest)
	case "play-user":
		err = audit.PlayUser(rest)
	case "tail":
		// Password gate — tail reveals recorded output (live or static).
		if perr := auth.PromptAndVerify(); perr != nil {
			fmt.Fprintln(os.Stderr, "ttrack:", perr)
			os.Exit(1)
		}
		if len(rest) > 0 && rest[0] == "-f" {
			err = audit.TailLive(rest[1:])
		} else {
			err = audit.TailStatic(rest)
		}
	case "tree":
		err = audit.Tree(rest)
	case "search":
		// Password gate — search reveals matching output snippets.
		if perr := auth.PromptAndVerify(); perr != nil {
			fmt.Fprintln(os.Stderr, "ttrack:", perr)
			os.Exit(1)
		}
		err = audit.Search(rest)
	case "export":
		// Password gate — export reveals the full decrypted session.
		if perr := auth.PromptAndVerify(); perr != nil {
			fmt.Fprintln(os.Stderr, "ttrack:", perr)
			os.Exit(1)
		}
		err = audit.Export(rest)
	case "prune":
		err = audit.Prune(rest)
	case "ansible":
		err = ansible.Dispatch(rest)
	case "ansible-ingest":
		// Hidden: called by the Ansible callback plugin subprocess.
		err = ansible.Ingest(rest)
	case "backup":
		err = backup.RunCLI(rest)
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

// lastNonFlagArg returns the last argument that does not start with "-",
// or "" if there is none. Used to pick the play target from a mixed arg list.
func lastNonFlagArg(args []string) string {
	target := ""
	for _, a := range args {
		if !strings.HasPrefix(a, "-") {
			target = a
		}
	}
	return target
}

// parseLsScope interprets `ls`/`list` flags: --all/-a selects all users in the
// central store, and --user <name> or --user=<name> selects a single user.
func parseLsScope(args []string) (all bool, user string) {
	for i, a := range args {
		switch {
		case a == "--all" || a == "-a":
			all = true
		case a == "--user" && i+1 < len(args):
			user = args[i+1]
		case strings.HasPrefix(a, "--user="):
			user = strings.TrimPrefix(a, "--user=")
		}
	}
	return all, user
}

func usage() {
	fmt.Fprint(os.Stderr, `ttrack — Linux terminal session tracker

usage:
  ttrack rec [-q] [-o file] [cmd...]      record a shell session (default: $SHELL)
  ttrack play [--speed N] <file|id>       replay local file or central session (auto-detect)
  ttrack ls                               list local recordings
  ttrack ls --all                         list all users in central store (root)
  ttrack ls --user <name>                 list one user's sessions in central store (root)

audit commands (central root-only store):
  ttrack tail [-n N] <id>                 show last N lines of a session (default 20)
  ttrack tail -f <id>                     live-stream an in-progress session (root)
  ttrack tree                             users -> sessions tree (root)
  ttrack search [opts] <string>           find a string across recordings (root)
  ttrack export [-o file] <id>            decrypt a session to a plaintext cast (root)
  ttrack prune                            interactively delete recordings (root)
  ttrack backup                           run configured backup immediately (root)
  ttrack ansible list [--user U]          list Ansible playbook runs (root)
  ttrack ansible show <runid>             show tasks and recap for a run (root)

  ttrack init                             first-time setup wizard
  ttrack init --reset-password            change playback password (requires current)
  ttrack init --clear-password            remove playback password (requires current)
  ttrack completion bash                  print the bash completion script
  ttrack version                          print version
  ttrack --check                          validate config and show resolved values

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
	case "init":
		return `ttrack init — first-time setup wizard and playback password management

usage: ttrack init                    run the setup wizard
       ttrack init --reset-password   change the playback password
       ttrack init --clear-password   remove playback password protection

The wizard checks:
  [1/4] Config file (/etc/ttrack/ttrack.conf)
  [2/4] Daemon (ttrackd) reachability
  [3/4] Encryption key existence
  [4/4] Playback password status (offers to set one if absent)

--reset-password and --clear-password both require the current password first.
A playback password must be at least 8 characters.
The hash is stored in /etc/ttrack/.playback_passwd (root:root 0600).
When set, ttrack play prompts for the password before replaying any session.
`, true
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
		return `ttrack play — replay a recording (local file or central session)

usage: ttrack play [--speed N] [--idle N] <file|id>

Auto-detects: if the argument is an existing local file path it plays that;
otherwise it looks up the session id in the central store (same as play-user).

options:
  --speed N   playback multiplier (default 1.0; >1 faster, <1 slower)
  --idle N    cap idle gaps to N seconds (default 0 = exact timing)

on a terminal this opens a full-screen player. controls:
  space pause/resume    left/right or h/l seek 5s    up/down or +/- speed
  g jump to a recorded command    0 restart    q or Ctrl-C quit
`, true
	case "ls", "list":
		return `ttrack ls — list recordings

usage: ttrack ls                   list local recordings (~/.local/share/ttrack)
       ttrack ls --all             list all users in central store (root)
       ttrack ls --user <name>     list one user's sessions in central store (root)

Columns (local): STATUS, FILE, STARTED, DURATION, COMMAND
Columns (central): STATUS, TYPE, SESSION, STARTED, DURATION, COMMAND
`, true
	case "tail":
		return `ttrack tail — show session output (tail or live-stream)

usage: ttrack tail [-n N] <sessionid>   print last N lines of a session (default 20)
       ttrack tail -f <sessionid>        live-stream an in-progress session (root)

-n N   number of output lines to display (static mode only)
-f     follow: stream live output from the daemon as it arrives
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
	case "ansible":
		return `ttrack ansible — Ansible playbook tracking (root)

usage: ttrack ansible list [--user U]
       ttrack ansible show [--user U] <runid>

Reads Ansible runs recorded by the ttrack callback plugin.
Runs are stored in the central store under each user's ansible/ directory.

subcommands:
  list       table of runs: RUN PLAYBOOK CONTROLLER OK CHG FAIL STARTED HOSTS
  show       full run detail: plays, tasks with status/output, PLAY RECAP

Enable tracking on the Ansible controller:
  export ANSIBLE_CALLBACK_PLUGINS=/usr/share/ttrack/ansible
  export ANSIBLE_CALLBACKS_ENABLED=ttrack

or in ansible.cfg:
  [defaults]
  callback_plugins = /usr/share/ttrack/ansible
  callbacks_enabled = ttrack
`, true
	case "completion":
		return `ttrack completion — print the shell completion script

usage: ttrack completion bash

Install:
  ttrack completion bash | sudo tee /usr/share/bash-completion/completions/ttrack
`, true
	case "backup":
		return `ttrack backup — run a configured backup immediately (root)

usage: ttrack backup

Triggers a one-shot backup of the central recording store to the configured
target. Respects the same backup_type and backup_target settings used by the
daemon's periodic backup.

Backup types:
  bucket_aws   shells out to: aws s3 sync <central_dir> <backup_target>
  bucket_gcp   shells out to: gsutil -m rsync -r <central_dir> <backup_target>
  rsync        shells out to: rsync -a --delete <central_dir>/ <backup_target>

Configure in /etc/ttrack/ttrack.conf:
  backup_type   = bucket_aws | bucket_gcp | rsync
  backup_target = s3://bucket/prefix | gs://bucket/prefix | user@host:/path

Access is enforced by filesystem permissions (central_dir is root:root 0700).
`, true
	}
	return "", false
}

// runConfigCheck validates the config file and prints all resolved values.
// Returns 0 on success, 1 on error (mirrors nginx -t behaviour).
func runConfigCheck() int {
	path := os.Getenv("TTRACK_CONFIG")
	if path == "" {
		path = config.DefaultPath
	}

	fmt.Fprintf(os.Stderr, "ttrack: reading config from %s\n", path)

	if _, err := os.Stat(path); os.IsNotExist(err) {
		fmt.Fprintf(os.Stderr, "ttrack: config file not found — using built-in defaults\n")
	}

	cfg := config.Load()

	// Resolved key file for display (may differ from raw KeyFile).
	resolvedKey := cfg.ResolvedKeyFile()

	fmt.Printf("%-22s = %s\n", "socket_path", cfg.SocketPath)
	fmt.Printf("%-22s = %s\n", "central_dir", cfg.CentralDir)
	if cfg.KeyFile == "" {
		fmt.Printf("%-22s = %s  (default: relative to central_dir)\n", "key_file", resolvedKey)
	} else {
		fmt.Printf("%-22s = %s  (resolved: %s)\n", "key_file", cfg.KeyFile, resolvedKey)
	}
	fmt.Printf("%-22s = %.3gs\n", "dial_timeout_sec", cfg.DialTimeout.Seconds())
	fmt.Printf("%-22s = %dms\n", "eof_grace_ms", cfg.EOFGrace.Milliseconds())
	fmt.Printf("%-22s = %d\n", "ansible_output_cap", cfg.AnsibleOutputCap)
	fmt.Printf("%-22s = %d\n", "scroll_buffer", cfg.ScrollBuffer)
	fmt.Printf("%-22s = %d  (0=off 1=error 2=warn 3=info 4=debug 5=trace)\n", "log_level", cfg.LogLevel)
	fmt.Printf("%-22s = %s\n", "log_file", cfg.LogFile)
	fmt.Printf("%-22s = %s\n", "backup_type", cfg.BackupType)
	fmt.Printf("%-22s = %s\n", "backup_target", cfg.BackupTarget)
	fmt.Printf("%-22s = %d\n", "backup_interval_sec", cfg.BackupIntervalSec)
	if auth.IsSet() {
		fmt.Printf("%-22s = SET\n", "playback_password")
	} else {
		fmt.Printf("%-22s = not set\n", "playback_password")
	}

	// Warn about active env overrides so user knows values may differ from file.
	overrides := [][2]string{
		{"TTRACKD_SOCK", os.Getenv("TTRACKD_SOCK")},
		{"TTRACK_CENTRAL_DIR", os.Getenv("TTRACK_CENTRAL_DIR")},
		{"TTRACK_KEY_FILE", os.Getenv("TTRACK_KEY_FILE")},
		{"TTRACK_DIAL_TIMEOUT_SEC", os.Getenv("TTRACK_DIAL_TIMEOUT_SEC")},
		{"TTRACK_EOF_GRACE_MS", os.Getenv("TTRACK_EOF_GRACE_MS")},
		{"TTRACK_ANSIBLE_OUTPUT_CAP", os.Getenv("TTRACK_ANSIBLE_OUTPUT_CAP")},
		{"TTRACK_SCROLL_BUFFER", os.Getenv("TTRACK_SCROLL_BUFFER")},
		{"TTRACK_LOG_LEVEL", os.Getenv("TTRACK_LOG_LEVEL")},
		{"TTRACK_LOG_FILE", os.Getenv("TTRACK_LOG_FILE")},
		{"TTRACK_BACKUP_TYPE", os.Getenv("TTRACK_BACKUP_TYPE")},
		{"TTRACK_BACKUP_TARGET", os.Getenv("TTRACK_BACKUP_TARGET")},
		{"TTRACK_BACKUP_INTERVAL_SEC", os.Getenv("TTRACK_BACKUP_INTERVAL_SEC")},
	}
	for _, ov := range overrides {
		if ov[1] != "" {
			fmt.Fprintf(os.Stderr, "ttrack: warning: %s=%s overrides config file\n", ov[0], ov[1])
		}
	}

	fmt.Fprintln(os.Stderr, "ttrack: config OK")
	return 0
}
