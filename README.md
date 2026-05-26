# trackterm-rec

Audit-grade terminal session recorder for Linux. Captures every interactive shell — SSH, `su`, `sudo -i`, local console — and stores per-session, replayable recordings in **ttyrec** format. Designed for shared admin hosts where forensic audit and compliance review of operator activity is required.

## Architecture

```
   sshd / login / su / sudo
            │
            ▼
    pam_record.so          ← PAM session module: mints session ID, chains nested sessions
            │
            ▼
  /etc/profile.d/trackterm-rec.sh  (auto-triggered for interactive shells)
            │ exec
            ▼
    trackterm-rec          ← PTY shim: runs as user, relays I/O, records output
            │ AF_UNIX socket
            ▼
    trackterm-recd         ← Root daemon: writes per-session files
            │
            ▼
  /var/lib/trackterm-rec/<date>/<sid>.ttyrec
  /var/lib/trackterm-rec/<date>/<sid>.meta.json
  /var/lib/trackterm-rec/<date>/<sid>.events.jsonl

    trackterm-cli          ← Audit CLI: list / play / tail / purge / tree / tui
```

**Identity anchor:** `/proc/self/loginuid` (Linux audit subsystem, immutable across `su`/`sudo`) is recorded on every session. Nested sessions (e.g. `sudo -i` inside SSH) carry a `parent_sid` link back to the originating session.

**Fail-open:** if the daemon is unreachable, the shim falls back to recording locally under `/tmp/trackterm-rec/` and continues without interrupting the user's session. Reconnect is attempted automatically every 5 seconds; a `gap` event is written to the session log on reconnect.

## Components

| Binary | Runs as | Purpose |
|--------|---------|---------|
| `trackterm-rec` | user | PTY shim; intercepted via `profile.d` |
| `trackterm-recd` | root | Central daemon; epoll; writes recordings |
| `pam_record.so` | — | PAM session module; SID minting + env propagation |
| `trackterm-cli` | root (audit) | CLI for listing, replaying, purging sessions |

## Requirements

**RHEL 9 / Rocky 9:**
```bash
sudo dnf install -y pam-devel systemd-devel zlib-devel ncurses-devel gcc make
```

**Debian 12 / Ubuntu 24.04:**
```bash
sudo apt install -y libpam0g-dev libsystemd-dev zlib1g-dev libncurses-dev gcc make
```

## Build

```bash
make all          # build all four binaries into build/
make rpm          # build RPM package → release/
make deb          # build DEB package → release/
make packages     # build both RPM and DEB
make tests        # run unit tests
make clean        # remove build artifacts
```

For sanitizer builds (leak and undefined-behaviour checking):

```bash
make CFLAGS_EXTRA="-fsanitize=address,undefined" \
     LDFLAGS_EXTRA="-fsanitize=address,undefined" all
```

## Install

Run as root after `make all`:

```bash
sudo bash scripts/install.sh
```

The installer:
- Detects distro and installs `pam_record.so` to the correct PAM security directory
- Installs binaries: `trackterm-rec` → `/usr/libexec/`, `trackterm-cli` → `/usr/bin/`
- Enables and starts `trackterm-recd.socket` and `trackterm-recd.service`
- Drops `profile.d` and `zshenv` hooks
- Configures `sudoers.d` to preserve session env vars

Or install manually:

```bash
sudo make install
```

## PAM Integration

Add to `/etc/pam.d/sshd` (after `pam_loginuid.so`):
```
session    optional     pam_record.so service=sshd
```

See `scripts/pam.d/` for ready-made snippets for `sshd`, `su`, `sudo`, and `login`. See [`docs/PAM-INTEGRATION.md`](docs/PAM-INTEGRATION.md) for full details.

## Configuration

`/etc/trackterm-rec/recd.conf` — see `config/recd.conf.sample`:

```ini
storage_dir = /var/lib/trackterm-rec
socket_path = /run/trackterm-recd.sock
max_session_mb = 64          # rotate session file above this size
max_age_days = 90
fail_closed = 0              # 1 = deny shell if daemon unreachable
gzip_on_rotate = 1
chattr_append_only = 0       # ext4/xfs only; adds forensic friction
log_level = 6
```

`/etc/trackterm-rec/shells.allow` — whitelist of shells the shim will exec (one path per line). Prevents shim from launching arbitrary binaries as the child shell.

## Usage

### List sessions
```bash
sudo trackterm-cli list
sudo trackterm-cli list --user alice
```

```
STATUS   SID                                   USER   SERVICE  RHOST     START
ACTIVE   31aea091-8224-4a7e-acf9-9a172f86f4d7  alice  sshd     10.0.0.5  2026-05-20T11:01:10
EXITED   336dcf62-895f-495c-88f0-c10eec70d033  alice  sshd     10.0.0.5  2026-05-20T10:43:33
```

### Replay a session
```bash
sudo trackterm-cli play <sid>
sudo trackterm-cli play --speed 2.0 <sid>     # 2× speed
sudo trackterm-cli play --dump <sid>           # dump raw bytes, no timing
```

> Play refuses to replay the currently recording session to prevent a feedback loop where replay output is captured and grows the ttyrec file. Use a separate terminal to replay active sessions.

### Tail a live session
```bash
sudo trackterm-cli tail <sid>      # stream raw output from active session
```

### Interactive TUI browser
```bash
sudo trackterm-cli tui
```

Keys: `j`/`k` or arrows — navigate · `p`/Enter — play · `t` — tail · `d` — delete (confirm) · `r` — refresh · `?` — help · `q` — quit

Active sessions are highlighted in green.

### Session tree (nested su/sudo chains)
```bash
sudo trackterm-cli tree <root-sid>
```

Output shows the chain of PIDs with `loginuid` preserved at every level — the key forensic invariant proving which operator initiated the original session.

### Purge old sessions
```bash
sudo trackterm-cli purge --days 30          # delete sessions older than 30 days
sudo trackterm-cli purge --sid <sid>        # delete one specific session
```

## Session Files

Each session produces three files in `/var/lib/trackterm-rec/<YYYY-MM-DD>/`:

| File | Content |
|------|---------|
| `<sid>.ttyrec` | Raw terminal output in ttyrec binary format |
| `<sid>.meta.json` | Session metadata: user, loginuid, rhost, timestamps, exit status |
| `<sid>.events.jsonl` | Out-of-band events: `start`, `resize`, `gap`, `end` |

ttyrec files are compatible with `ttyplay`, `ipbt`, and `termrec`.

## Testing

Unit tests:

```bash
make tests
```

Smoke tests (requires daemon running):

```bash
bash tests/smoke/m1_shim_local.sh    # shim recording without daemon
bash tests/smoke/m2_daemon_wire.sh   # full shim → daemon wire path
```

Load test (50 concurrent sessions):

```bash
sudo bash tests/load/load_test.sh 50
```

## Security Model

- **Authentication:** `SO_PEERCRED` on the AF_UNIX socket provides kernel-attested peer UID/PID. The shim's claimed `loginuid` is cross-checked against `/proc/<pid>/loginuid`.
- **SID validation:** Session IDs are validated as UUID format before use in file paths, preventing path traversal attacks.
- **JSON safety:** All user-controlled fields (rhost, tty, TERM, etc.) are JSON-escaped before writing to audit files, preventing log injection.
- **Storage permissions:** `/var/lib/trackterm-rec/` is `root:trackterm-audit 0750`. Files are `0640`. Users cannot read their own recordings.
- **Socket permissions:** Socket is `0660`, group `trackterm-audit`. Shim binary has setgid `trackterm-audit` to pass the SO_PEERCRED group check.
- **Daemon hardening:** `ProtectSystem=strict`, `NoNewPrivileges=yes`, `PrivateTmp=yes`, `RestrictAddressFamilies=AF_UNIX`, `MemoryDenyWriteExecute=yes`.

See [`docs/SECURITY.md`](docs/SECURITY.md) for the full threat model.

## Re-entrancy

The shim sets `TRACKTERM_REC_ACTIVE=1` and `TRACKTERM_REC_SHIM_CHILD=1` in the child shell environment. `profile.d` and `zshenv` bail early if either is set, preventing infinite self-wrapping. Nested `sudo -i` re-enters PAM → fresh SID with `parent_sid` pointing to the outer session.

## Operations

See [`docs/OPERATIONS.md`](docs/OPERATIONS.md) for runbook procedures: restarting the daemon without session loss, removing a user from recording, emergency disable, disk-full handling, and log locations.

## License

GPL-2.0. See [LICENSE](LICENSE).
