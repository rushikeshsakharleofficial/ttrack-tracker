<div align="center">

# Terminal Session Recorder for Linux — ttrack

Record and replay Linux terminal sessions as asciinema-compatible casts, with an optional root daemon that collects every user's session into a root-only central store for audit.

[![CI](https://github.com/rushikeshsakharleofficial/terminal-session-recorder/actions/workflows/ci.yml/badge.svg)](https://github.com/rushikeshsakharleofficial/terminal-session-recorder/actions/workflows/ci.yml)
[![Release](https://img.shields.io/github/v/release/rushikeshsakharleofficial/terminal-session-recorder)](https://github.com/rushikeshsakharleofficial/terminal-session-recorder/releases)
[![License: GPL-2.0](https://img.shields.io/badge/license-GPL--2.0-blue.svg)](LICENSE)

</div>

---

`ttrack` is a command-line terminal session recorder for Linux. It runs a shell under a PTY, captures the output as an [asciinema v2](https://docs.asciinema.org/manual/asciicast/v2/) cast file, and replays it with original timing. A companion root daemon, `ttrackd`, collects sessions from all users into a root-only central store (`/var/lib/ttrack`) so a host's operator activity can be reviewed and live-tailed for audit. It is a single static Go binary with no runtime dependencies.

## Features

- **Record & replay** any interactive shell session (`script(1)` / `asciinema`-style).
- **asciinema v2 cast** output — inspectable JSON-lines; also playable with `asciinema play`.
- **Central audit store** via the `ttrackd` root daemon: all users' sessions in `/var/lib/ttrack`, `root:root 0700` — normal users cannot read recordings.
- **Live tail** an in-progress session (`ttrack tail <id>`, root).
- **Audit CLI**: list users, list a user's sessions, replay by id, tree view.
- **Auto-record on login** via an optional `profile.d` hook (skips nested `sudo su -`).
- **Fail-open**: if the daemon is down, recording falls back to a user-local file and is ingested into the central store when the daemon restarts.
- **Bash tab-completion** for subcommands, flags, sessions, and users.
- Ships as `rpm` and `deb` packages with a systemd unit.

## Demo

```text
$ ttrack --help
ttrack — Linux terminal session tracker

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
```

## Requirements

- Linux (uses `/proc` and `SO_PEERCRED`).
- To build from source: Go 1.25.

## Installation

### From a released package

Download `rpm`/`deb` from the [latest release](https://github.com/rushikeshsakharleofficial/terminal-session-recorder/releases):

```bash
sudo dnf install ./ttrack-0.2.0-1.x86_64.rpm      # RHEL / Rocky / Fedora
sudo apt install ./ttrack_0.2.0_amd64.deb         # Debian / Ubuntu
```

Packages install `ttrack` to `/usr/bin`, the `ttrackd` daemon to `/usr/libexec`, a systemd unit, the bash completion, and the auto-record login hook. The post-install step creates `/var/lib/ttrack` (root-only) and enables `ttrackd`.

### From source

```bash
git clone https://github.com/rushikeshsakharleofficial/terminal-session-recorder.git
cd terminal-session-recorder
make build          # builds bin/ttrack and bin/ttrackd
sudo make install   # installs binaries, man page, systemd unit, completion
```

## Quick start

Record a session, list it, and replay it:

```text
$ ttrack rec /bin/bash -c 'echo "hello from ttrack"; uname -sr'
ttrack: recording to /home/alice/.local/share/ttrack/20260526T145029-1413696.cast — type 'exit' or Ctrl-D to stop
hello from ttrack
Linux 5.14.0-611.55.1.el9_7.x86_64

ttrack: session saved to /home/alice/.local/share/ttrack/20260526T145029-1413696.cast

$ ttrack ls
STATUS   FILE                          STARTED              COMMAND
SAVED    20260526T145029-1413696.cast  2026-05-26 14:50:29  /bin/bash -c echo "hello from ttrack"; uname -sr

$ ttrack play --speed 100 20260526T145029-1413696.cast
--- ttrack replay start ---
hello from ttrack
Linux 5.14.0-611.55.1.el9_7.x86_64
--- ttrack replay end ---
```

With no command, `ttrack rec` records your `$SHELL` interactively until you `exit`.

## Commands

### Personal commands

| Command | Description |
|:--------|:------------|
| `ttrack rec [-q] [-o file] [cmd...]` | Record a session. Runs `$SHELL` (fallback `/bin/bash`) with no command. |
| `ttrack play [--speed N] [--idle N] <file>` | Replay a local recording with original timing. |
| `ttrack ls` | List your local recordings (`STATUS`, `FILE`, `STARTED`, `COMMAND`). |
| `ttrack completion bash` | Print the bash completion script. |

`rec` flags: `-o <file>` writes a local file at that path; `-q` (or `TTRACK_QUIET=1`) suppresses the recording banner and saved-path message.

`play` flags: `--speed N` playback multiplier (default `1.0`); `--idle N` caps idle gaps to N seconds (default `2.0`, `0` disables).

### Audit commands (root)

These read the central root-only store and require root:

| Command | Description |
|:--------|:------------|
| `ttrack ls-user` | List users that have recordings and their session counts. |
| `ttrack ls-user <username>` | List a user's sessions. |
| `ttrack play-user [--speed N] <sessionid>` | Replay a session by id, searched across all users. |
| `ttrack tail <sessionid>` | Live-stream an in-progress session from the daemon. |
| `ttrack tree` | Print a users → sessions tree. |

## Audit mode (central root-only store)

When `ttrackd` runs (it does after package install), `ttrack rec` streams the cast
to it over `/run/ttrackd.sock` and the recording is written by root to
`/var/lib/ttrack/<user>/<sessionid>.cast` (`root:root 0600`, dirs `0700`). Normal
users cannot read other users' — or their own — recordings.

```text
$ sudo ttrack ls-user
USER                  SESSIONS
root                  1
alice                 7

$ sudo ttrack ls-user alice
STATUS   SESSION                       STARTED              COMMAND
SAVED    20260526T145020-1413240.cast  2026-05-26 14:50:19  /bin/bash -c echo deploy-step-1; whoami

$ sudo ttrack tree
/var/lib/ttrack
├─ root
│  └─ 20260526T124229-1909275.cast  [SAVED]  2026-05-26 12:42:29  /bin/bash
└─ alice
   └─ 20260526T145020-1413240.cast  [SAVED]  2026-05-26 14:50:19  /bin/bash -c echo deploy-step-1; whoami
```

Other audit commands:

```bash
sudo systemctl status ttrackd        # daemon state
sudo ttrack play-user <sessionid>    # replay any session
sudo ttrack tail <sessionid>         # watch a live session
```

**Fail-open:** if the daemon is unreachable, `ttrack rec` records to the user-local
directory; on its next startup `ttrackd` ingests those files into the central store
(source files are opened `O_NOFOLLOW` and verified regular, so a user cannot symlink
a root-readable target into the store).

**Integrity note:** this provides root-only *access* to recordings plus live tail. It
is *not* tamper-proof against a malicious user, who could avoid `ttrack` entirely
(run another binary, `pkill ttrack`, bypass `profile.d`). Non-circumventable capture
requires PAM- or kernel-stage hooks, which this project does not implement.

## Auto-record on login (optional)

The package installs a `profile.d` hook that records every interactive login and logs
out when the recorded shell exits. To enable manually:

```bash
sudo install -m644 scripts/profile.d/ttrack-autorec.sh /etc/profile.d/ttrack-autorec.sh
```

The hook only triggers for interactive shells with a real TTY, skips when `ttrack` is
absent, and skips nested shells (`sudo su -`, `su -`, subshells) by detecting a
`ttrack` process in the ancestry — a session is recorded once. It is fail-open: if the
recorder cannot start, a normal shell continues. Remove the file to disable.

## Shell completion

Bash completion is installed by the package to
`/usr/share/bash-completion/completions/ttrack`. To enable manually:

```bash
ttrack completion bash | sudo tee /usr/share/bash-completion/completions/ttrack
```

It completes subcommands, flags, local sessions (for `play`), and — when run as root —
users and central session ids (for `ls-user`, `play-user`, `tail`).

## Configuration

`ttrack` is configured entirely through environment variables:

| Variable | Default | Description |
|:---------|:--------|:------------|
| `TTRACK_DIR` | `~/.local/share/ttrack` | User-local recordings directory. |
| `TTRACK_CENTRAL_DIR` | `/var/lib/ttrack` | Central root-only store (daemon + audit commands). |
| `TTRACKD_SOCK` | `/run/ttrackd.sock` | Daemon socket used by `ttrack rec` and audit commands. |
| `TTRACK_QUIET` | unset | If set, `ttrack rec` suppresses banner and saved-path message. |
| `SHELL` | `/bin/bash` | Shell launched by `ttrack rec` when no command is given. |

## File format

Recordings are asciinema v2 cast files (UTF-8, JSON-lines):

```text
{"version":2,"width":80,"height":24,"timestamp":1779776263,"command":"/bin/bash","env":{"SHELL":"/bin/bash","TERM":"xterm-256color"}}
[0.131000, "o", "hello\r\n"]
```

The first line is a header; each subsequent line is `[time_seconds, "o", data]`. Files
are also viewable with `asciinema cat` and playable with `asciinema play`.

## Building and packaging

```bash
make build          # bin/ttrack and bin/ttrackd (static, CGO disabled)
make test           # run unit tests
make rpm            # build an rpm into release/
make deb            # build a deb into release/
make packages       # both
make VERSION=1.2.3 packages
```

Packaging uses [`nfpm`](https://github.com/goreleaser/nfpm) (`go install` it first).
CI builds packages on every push (downloadable as the `ttrack-packages` workflow
artifact); pushing a `v*` tag publishes `rpm`/`deb`/binary/checksums to a GitHub Release.

## Testing

```bash
make test                  # unit tests (go test ./...)
```

## Project structure

```text
cmd/ttrack      CLI entry point (rec/play/ls/ls-user/play-user/tail/tree)
cmd/ttrackd     root collector daemon
internal/cast   asciinema v2 cast read/write
internal/record PTY capture for `ttrack rec`
internal/play   replay
internal/store  user-local and central storage paths
internal/audit  root-only audit commands
internal/daemon ttrackd socket server, fan-out, ingest
internal/complete  shell completion
```

## Contributing

Issues and pull requests are welcome via the
[issue tracker](https://github.com/rushikeshsakharleofficial/terminal-session-recorder/issues).
Before opening a PR, run `make fmt`, `make vet`, and `make test`; CI enforces `gofmt`,
`go vet`, and the test suite. No `CONTRIBUTING.md` exists yet.

## License

Licensed under the GNU General Public License v2.0. See [LICENSE](LICENSE).

## Maintainer TODOs

- Set the GitHub repository **About** description (Settings) to ≤160 chars, e.g.:
  `Record and replay Linux terminal sessions; root daemon collects all users into a root-only audit store. Single static Go binary.`
- Set GitHub repository **Topics** (Settings → Topics), e.g. `linux`, `terminal`, `session-recording`, `asciinema`, `audit`, `cli`, `golang`, `pty`.
- Add a 1280×640 social preview image (Settings → Social preview): tool name, one-line description, a terminal screenshot.
- Add a recorded `.cast`/GIF demo asset and embed it in the Demo section.
