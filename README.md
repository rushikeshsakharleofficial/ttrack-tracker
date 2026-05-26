# ttrack

[![CI](https://github.com/rushikeshsakharleofficial/terminal-session-recorder/actions/workflows/ci.yml/badge.svg)](https://github.com/rushikeshsakharleofficial/terminal-session-recorder/actions/workflows/ci.yml)

A minimal Linux terminal session recorder and replayer. `ttrack rec` forks your shell under a PTY, captures all output to an [asciinema v2](https://docs.asciinema.org/manual/asciicast/v2/) cast file, and `ttrack play` replays it later. Think `script(1)` or `asciinema`, packaged as a single self-contained Go binary with no external runtime dependencies.

## Build

Requires Go 1.25.

```bash
CGO_ENABLED=0 go build -trimpath -o bin/ttrack ./cmd/ttrack
```

Or with make:

```bash
make build
```

Dependencies: [`github.com/creack/pty`](https://github.com/creack/pty), [`golang.org/x/term`](https://pkg.go.dev/golang.org/x/term).

## Quick start

```bash
# Record a session (auto-named file in the store dir)
ttrack rec

# Exit the shell when done, then list recordings
ttrack ls

# Replay a recording
ttrack play ~/.local/share/ttrack/20260526T120000-12345.cast
```

## Commands

### `ttrack rec [-o file] [cmd...]`

Record a session. Forks `cmd` (or `$SHELL`, falling back to `/bin/bash`) under a PTY and writes all output to a cast file.

| Flag | Default | Description |
|------|---------|-------------|
| `-o file` | auto-named in store dir | Output file path |
| `-q` | off | Quiet: suppress the recording banner and saved-path message (also via `TTRACK_QUIET=1`) |

If no `cmd` is given, the user's `$SHELL` is launched interactively.

### `ttrack play [--speed N] [--idle N] <file>`

Replay a recording.

| Flag | Default | Description |
|------|---------|-------------|
| `--speed N` | `1.0` | Playback speed multiplier |
| `--idle N` | `2.0` | Cap idle gaps to N seconds (`0` disables the cap) |

### `ttrack ls`

List recordings in the store dir. Columns: `FILE`, `STARTED`, `COMMAND`.

## Storage and file format

The store directory is `$TTRACK_DIR` if set, otherwise `~/.local/share/ttrack`.

Auto-named recordings use the pattern `<YYYYMMDDThhmmss>-<pid>.cast`.

Files are written in **asciinema v2 cast format** (JSON-lines):

- First line: header object — `version`, `width`, `height`, `timestamp`, `command`, `env`.
- Subsequent lines: event arrays — `[time_seconds, "o", "output-data"]`.

Because the format is standard asciinema v2, recordings are also playable with `asciinema play <file>` if asciinema is installed.

## Auto-record on login (optional)

To record every interactive login automatically, install the profile.d hook:

```bash
sudo install -m644 scripts/profile.d/ttrack-autorec.sh /etc/profile.d/ttrack-autorec.sh
```

Behavior:
- Records interactive login shells; logs out cleanly when the recorded shell exits.
- Skips non-interactive shells, non-TTY sessions (scp/sftp/cron), and `ttrack rec` is left untouched if absent.
- Nested shells (`sudo su -`, `su -`, subshells) are **not** re-recorded — a `ttrack` ancestor in the process tree is detected and skipped, so a session is captured once.
- **Fail-open**: if the recorder cannot start, the normal shell continues — login is never blocked.

Disable by removing the file: `sudo rm /etc/profile.d/ttrack-autorec.sh`.

## Audit mode (central root-only store)

Installed packages run **`ttrackd`**, a root daemon that collects sessions from
all users into a root-only central store at `/var/lib/ttrack` (`root:root 0700`,
files `0600`) — normal users cannot read recordings.

- `ttrack rec` streams to `ttrackd` over `/run/ttrackd.sock` when it is running;
  recordings land in the central store, owned by root.
- **Fail-open**: if the daemon is down, `ttrack rec` falls back to the user-local
  dir, and `ttrackd` ingests those files into the central store on next startup.
- Audit commands (run as **root**):

```bash
ttrack ls-user                    # users that have recordings
ttrack ls-user alice              # alice's sessions
ttrack play-user <sessionid>      # replay a session by id (any user)
ttrack tail <sessionid>           # live-stream an in-progress session
ttrack tree                       # users -> sessions tree
```

**Integrity note:** this gives root-only *access* to recordings and live watch.
It is not tamper-*proof* against a malicious user (who could avoid `ttrack`
entirely); true non-circumventable capture requires PAM/kernel-stage hooks.

## Limitations

- Records PTY output only — keystrokes are not captured.
- `ttrack rec` records the shell it launches, not pre-existing sessions.
- Audit/central commands require root (the central store is root-only).

## License

[LICENSE](LICENSE)
