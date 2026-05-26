<div align="center">

# Terminal Session Recorder for Linux — ttrack

Record and replay Linux terminal sessions as asciinema-compatible casts, with an optional root daemon that collects every user's session into a root-only central store for audit.

[![CI](https://github.com/rushikeshsakharleofficial/ttrack-tracker/actions/workflows/ci.yml/badge.svg)](https://github.com/rushikeshsakharleofficial/ttrack-tracker/actions/workflows/ci.yml)
[![Release](https://img.shields.io/github/v/release/rushikeshsakharleofficial/ttrack-tracker)](https://github.com/rushikeshsakharleofficial/ttrack-tracker/releases)
[![License: GPL-2.0](https://img.shields.io/badge/license-GPL--2.0-blue.svg)](LICENSE)
[![PRs Welcome](https://img.shields.io/badge/PRs-welcome-brightgreen.svg)](CONTRIBUTING.md)
[![Issues](https://img.shields.io/github/issues/rushikeshsakharleofficial/ttrack-tracker)](https://github.com/rushikeshsakharleofficial/ttrack-tracker/issues)

**Free and 100% open source.** Contributions welcome — [open an issue](https://github.com/rushikeshsakharleofficial/ttrack-tracker/issues) or [send a PR](CONTRIBUTING.md).

</div>

---

`ttrack` is a command-line terminal session recorder for Linux. It runs a shell under a PTY, captures the output as an [asciinema v2](https://docs.asciinema.org/manual/asciicast/v2/) cast file, and replays it with original timing. A companion root daemon, `ttrackd`, collects sessions from all users into a root-only central store (`/var/lib/ttrack`) so a host's operator activity can be reviewed and live-tailed for audit. It is a single static Go binary with no runtime dependencies.

## Table of contents

- [Features](#features)
- [Demo](#demo)
- [Installation](#installation)
- [Quick start](#quick-start)
- [Commands](#commands)
- [Audit mode (central root-only store)](#audit-mode-central-root-only-store)
- [Auto-record on login](#auto-record-on-login-optional)
- [Shell completion](#shell-completion)
- [Configuration guide](#configuration-guide)
- [File format](#file-format)
- [Building and packaging](#building-and-packaging)
- [Contributing](#contributing)
- [License](#license)

## Features

- **Record & replay** any interactive shell session (`script(1)` / `asciinema`-style).
- **asciinema v2 cast** output (local recordings are inspectable JSON-lines and play with `asciinema play`; central recordings are encrypted — `export` them first).
- **Central audit store** via the `ttrackd` root daemon: all users' sessions in `/var/lib/ttrack`, `root:root 0700` — normal users cannot read recordings.
- **Encrypted at rest** — central recordings are AES-256-GCM encrypted; `cat`/`strings`/`grep` on a `.cast` reveal only ciphertext. Readable solely with the root-only key via `ttrack`.
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

Download `rpm`/`deb` from the [latest release](https://github.com/rushikeshsakharleofficial/ttrack-tracker/releases):

```bash
sudo dnf install ./ttrack-0.2.0-1.x86_64.rpm      # RHEL / Rocky / Fedora
sudo apt install ./ttrack_0.2.0_amd64.deb         # Debian / Ubuntu
```

Packages install `ttrack` to `/usr/bin`, the `ttrackd` daemon to `/usr/libexec`, a systemd unit, the bash completion, and the auto-record login hook. The post-install step creates `/var/lib/ttrack` (root-only) and enables `ttrackd`.

### From source

```bash
git clone https://github.com/rushikeshsakharleofficial/ttrack-tracker.git
cd ttrack-tracker
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
| `ttrack search [--from T] [--to T] [--user U] [-i] <pattern>` | Find a string across all recordings (command + output), optionally within a time range. |
| `ttrack export [-o file] <sessionid>` | Decrypt a recording to a plaintext asciinema cast (for offline use / `asciinema play`). |

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

Search recordings for a string (e.g. a command that was run), optionally within a
time window:

```text
$ sudo ttrack search nginx
user=alice  when=2026-05-26 14:59:18  session=20260526T145918-1420180
    cmd: /bin/bash -c echo starting deploy; systemctl restart nginx; echo deploy done
    > Failed to restart nginx.service: Unit nginx.service not found.

$ sudo ttrack search --from "2026-05-26 09:00" --to "2026-05-26 18:00" --user alice -i DEPLOY
user=alice  when=2026-05-26 14:59:18  session=20260526T145918-1420180
    cmd: /bin/bash -c echo starting deploy; systemctl restart nginx; echo deploy done
    > starting deploy
    > deploy done
```

Each match shows **which user** ran it (`user=`) and **when** the session started
(`when=`), the recorded command, and matching output lines.

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

### Encryption at rest

Central recordings are encrypted with AES-256-GCM. On disk a `.cast` is opaque —
`cat`, `strings`, and `grep` show only ciphertext. `ttrack` decrypts transparently
for `play-user`, `search`, `tail`, and `export` using the key at
`/var/lib/ttrack/.ttrack.key` (`root:root 0600`), created by the daemon on first run.

```text
$ sudo strings /var/lib/ttrack/alice/20260526T151022-1426734.cast | head -1
TTEC1                          # magic prefix; the rest is ciphertext

$ sudo ttrack export -o session.cast 20260526T151022-1426734
exported plaintext cast to session.cast      # now asciinema-compatible
```

The key is **unique per server** (random, generated by the daemon on first run) and
set **immutable** (`chattr +i`): it cannot be deleted, renamed, or modified by
`rm`/`vi`/`sed`/`>`/`tee` — even by root — until someone runs `chattr -i`. It is
`0600`, so non-root `cat`/`strings` are denied outright.

> **Honest scope:** this means *no plaintext session data sits on disk*, the files
> are unreadable without the key, and the key cannot be casually altered or deleted —
> not that "only ttrack can ever read them." Root can still `chattr -i` and read the
> key, and root can read any `.cast` via the key (root bypasses everything). **Back up
> `/var/lib/ttrack/.ttrack.key`** — if it is lost, every encrypted recording is
> permanently unreadable. The daemon refuses to start if the key is missing while
> encrypted recordings exist. To rotate or remove the key, run `chattr -i` first.

**Integrity note:** this provides root-only *access* to recordings plus live tail. It
is *not* tamper-proof against a malicious user, who could avoid `ttrack` entirely
(run another binary, `pkill ttrack`, bypass `profile.d`). Non-circumventable capture
requires PAM- or kernel-stage hooks, which this project does not implement. The
user-local fail-open fallback is plaintext until the daemon ingests and encrypts it.

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

## Configuration guide

`ttrack` and `ttrackd` need no config file — behavior is controlled by environment
variables, filesystem locations, and the systemd unit.

### Environment variables

| Variable | Default | Used by | Description |
|:---------|:--------|:--------|:------------|
| `TTRACK_DIR` | `~/.local/share/ttrack` | `ttrack` | User-local recordings dir (fail-open fallback + local `ls`/`play`). |
| `TTRACK_CENTRAL_DIR` | `/var/lib/ttrack` | `ttrack`, `ttrackd` | Central root-only store. |
| `TTRACKD_SOCK` | `/run/ttrackd.sock` | `ttrack`, `ttrackd` | Daemon unix socket. |
| `TTRACK_QUIET` | unset | `ttrack rec` | Any non-empty value suppresses the banner + saved-path message. |
| `SHELL` | `/bin/bash` | `ttrack rec` | Shell launched when no command is given. |

To point a whole host at a different store, set `TTRACK_CENTRAL_DIR` and `TTRACKD_SOCK`
in the systemd unit (see below) **and** in users' environment so `ttrack rec` reaches
the same daemon.

### Filesystem layout and permissions

| Path | Owner / mode | Purpose |
|:-----|:-------------|:--------|
| `/usr/bin/ttrack` | `root 0755` | CLI |
| `/usr/libexec/ttrackd` | `root 0755` | daemon |
| `/var/lib/ttrack/` | `root:root 0700` | central store (normal users cannot enter) |
| `/var/lib/ttrack/<user>/<id>.cast` | `root:root 0600` | encrypted recording |
| `/var/lib/ttrack/.ttrack.key` | `root:root 0600`, `chattr +i` | per-server AES key (immutable) |
| `/run/ttrackd.sock` | `root 0666` | recorder connect socket (file access is what enforces privacy, not the socket) |
| `/etc/profile.d/ttrack-autorec.sh` | `root 0644` | optional auto-record login hook |
| `~/.local/share/ttrack/` | the user | local fail-open recordings |

### Encryption key

The daemon creates a unique random key per host on first start. It is `0600` and set
immutable (`chattr +i`) so it cannot be removed or modified by `rm`/`vi`/`sed`/`>`/`tee`,
even by root, until `chattr -i`. **Back it up** — losing it makes every encrypted
recording unreadable. The daemon refuses to start if the key is missing while encrypted
recordings exist. To rotate: `chattr -i`, move recordings aside or `export` them, remove
the key, restart the daemon (it generates a fresh one).

### Daemon service

```bash
sudo systemctl status ttrackd
sudo systemctl restart ttrackd
sudo journalctl -u ttrackd --no-pager        # logs, incl. the key-backup warning
```

To override the store/socket, add a drop-in:

```bash
sudo systemctl edit ttrackd
# [Service]
# Environment=TTRACK_CENTRAL_DIR=/srv/ttrack
# Environment=TTRACKD_SOCK=/run/ttrackd.sock
```

### Shell completion

Installed to `/usr/share/bash-completion/completions/ttrack`. Enable manually with
`ttrack completion bash | sudo tee /usr/share/bash-completion/completions/ttrack`.

## File format

Recordings are asciinema v2 cast files (UTF-8, JSON-lines):

```text
{"version":2,"width":80,"height":24,"timestamp":1779776263,"command":"/bin/bash","env":{"SHELL":"/bin/bash","TERM":"xterm-256color"}}
[0.131000, "o", "hello\r\n"]
```

The first line is a header; each subsequent line is `[time_seconds, "o", data]`.
Local (plaintext) recordings are viewable with `asciinema cat` and playable with
`asciinema play`. Central recordings are encrypted — run `ttrack export` first to
get an asciinema-compatible plaintext cast.

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

**Releases are automated.** Every push to `main` runs the `Auto Release` workflow,
which bumps the patch version from the latest tag and publishes a GitHub Release with
`rpm`, `deb`, the static binary, and `SHA256SUMS`. Pushing an explicit `v*` tag also
publishes a release (for deliberate minor/major bumps). Grab the latest from the
[releases page](https://github.com/rushikeshsakharleofficial/ttrack-tracker/releases).

## Testing

```bash
make test                  # unit tests (go test ./...)
```

## Project structure

```text
cmd/ttrack         CLI (rec/play/ls/ls-user/play-user/tail/tree/search/export)
cmd/ttrackd        root collector daemon
internal/cast      asciinema v2 cast read/write
internal/crypto    at-rest AES-256-GCM encryption (+ tests)
internal/record    PTY capture for `ttrack rec`
internal/play      replay (snapshot-bounded)
internal/store     storage paths + transparent decrypt
internal/audit     root-only audit commands
internal/daemon    ttrackd socket server, live tail fan-out, ingest, key mgmt
internal/complete  shell completion
```

## Contributing

ttrack is **100% open source** and community-driven — contributions of all sizes are
welcome.

- **Found a bug or want a feature?** [Open an issue](https://github.com/rushikeshsakharleofficial/ttrack-tracker/issues).
- **Want to contribute code?** Fork, branch, and open a pull request. Run `make fmt`,
  `make vet`, `make test`, `make build` first — CI enforces all of them.

See [CONTRIBUTING.md](CONTRIBUTING.md) for the full guide (bug reports, PR workflow,
project layout, tests).

## License

Licensed under the GNU General Public License v2.0. See [LICENSE](LICENSE).

## Maintainer TODOs

- Set the GitHub repository **About** description (Settings) to ≤160 chars, e.g.:
  `Record and replay Linux terminal sessions; root daemon collects all users into a root-only audit store. Single static Go binary.`
- Set GitHub repository **Topics** (Settings → Topics), e.g. `linux`, `terminal`, `session-recording`, `asciinema`, `audit`, `cli`, `golang`, `pty`.
- Add a 1280×640 social preview image (Settings → Social preview): tool name, one-line description, a terminal screenshot.
- Add a recorded `.cast`/GIF demo asset and embed it in the Demo section.
- **Back up the encryption key** `/var/lib/ttrack/.ttrack.key` offsite — losing it makes all encrypted recordings permanently unreadable.
