# terminal-session-recorder

Audit-grade terminal session recorder for Linux. Captures every interactive shell — SSH, `su`, `sudo -i`, local console — and stores per-session, replayable recordings in **ttyrec** format. Designed for shared admin hosts where forensic audit and compliance review of operator activity is required.

## Architecture

```
   sshd / login / su / sudo
            │
            ▼
    pam_record.so          ← PAM session module: mints session ID, chains nested sessions
            │
            ▼
  /etc/profile.d/pmp-rec.sh  (auto-triggered for interactive shells)
            │ exec
            ▼
      pmp-rec               ← PTY shim: runs as user, relays I/O, records output
            │ AF_UNIX socket
            ▼
      pmp-recd              ← Root daemon: writes per-session files
            │
            ▼
  /var/lib/pmp-rec/<date>/<sid>.ttyrec
  /var/lib/pmp-rec/<date>/<sid>.meta.json
  /var/lib/pmp-rec/<date>/<sid>.events.jsonl

      pmp-rec-cli           ← Audit CLI: list / play / tail / purge / tree / tui
```

**Identity anchor:** `/proc/self/loginuid` (Linux audit subsystem, immutable across `su`/`sudo`) is recorded on every session. Nested sessions (e.g. `sudo -i` inside SSH) carry a `parent_sid` link back to the originating session.

**Fail-open:** if the daemon is unreachable, the shim falls back to recording locally under `/tmp/pmp-rec/` and continues without interrupting the user's session.

## Components

| Binary | Runs as | Purpose |
|--------|---------|---------|
| `pmp-rec` | user | PTY shim; intercepted via `profile.d` |
| `pmp-recd` | root | Central daemon; epoll; writes recordings |
| `pam_record.so` | — | PAM session module; SID minting + env propagation |
| `pmp-rec-cli` | root (audit) | CLI for listing, replaying, purging sessions |

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
make all          # build all four binaries
make rpm          # build RPM package (requires rpmbuild)
make deb          # build DEB package
make packages     # build both RPM and DEB
```

Output binaries are placed in `build/`.

## Install

```bash
sudo make install
```

Or use the distro-aware installer:

```bash
sudo bash scripts/install.sh
```

The installer:
- Installs binaries to `/usr/libexec/` and `/usr/bin/`
- Installs PAM module to the correct platform path
- Enables and starts `pmp-recd.socket` and `pmp-recd.service`
- Drops `profile.d` and `zshenv` hooks
- Configures `sudoers.d` to preserve session env vars

## PAM Integration

Add to `/etc/pam.d/sshd` (after `pam_loginuid.so`):
```
session    optional     pam_record.so service=sshd
```

See `scripts/pam.d/` for ready-made snippets for `sshd`, `su`, `sudo`, and `login`. See `docs/PAM-INTEGRATION.md` for full details.

## Configuration

`/etc/pmp-rec/recd.conf` — see `config/recd.conf.sample`:

```ini
storage_dir = /var/lib/pmp-rec
socket_path = /run/pmp-recd.sock
max_session_bytes = 1073741824   # 1 GiB per session
max_age_days = 90
fail_closed = false
gzip_on_rotate = true
```

`/etc/pmp-rec/shells.allow` — whitelist of shells the shim will exec (one path per line). Prevents shim from launching arbitrary binaries as the child shell.

## Usage

### List sessions
```bash
sudo pmp-rec-cli list
sudo pmp-rec-cli list --user alice
```

```
STATUS   SID                                   USER                 SERVICE  RHOST            START                EXIT
--------------------------------------------------------------------------------------------------
ACTIVE   31aea091-8224-4a7e-acf9-9a172f86f4d7  alice                sshd     10.0.0.5         2026-05-20T11:01:10  null
EXITED   336dcf62-895f-495c-88f0-c10eec70d033  alice                sshd     10.0.0.5         2026-05-20T10:43:33  0
```

### Replay a session
```bash
sudo pmp-rec-cli play <sid>
sudo pmp-rec-cli play --speed 2.0 <sid>     # 2× speed
sudo pmp-rec-cli play --dump <sid>           # dump raw bytes, no timing
```

> Play refuses to replay the **currently recording session** to prevent a feedback loop where replay output is captured and grows the ttyrec file. Use a separate terminal or `--force` to override.

### Tail a live session
```bash
sudo pmp-rec-cli tail <sid>      # stream raw output from active session
```

### Interactive TUI browser
```bash
sudo pmp-rec-cli tui
```

Keys: `j`/`k` or arrows — navigate · `p`/Enter — play · `t` — tail · `d` — delete (confirm) · `r` — refresh · `?` — help · `q` — quit

Active sessions shown in green.

### Session tree (nested su/sudo chains)
```bash
sudo pmp-rec-cli tree <root-sid>
```

### Purge old sessions
```bash
sudo pmp-rec-cli purge --days 30          # delete sessions older than 30 days
sudo pmp-rec-cli purge --sid <sid>        # delete one specific session
```

## Session Files

Each session produces three files in `/var/lib/pmp-rec/<YYYY-MM-DD>/`:

| File | Content |
|------|---------|
| `<sid>.ttyrec` | Raw terminal output in ttyrec binary format |
| `<sid>.meta.json` | Session metadata: user, loginuid, rhost, timestamps, exit status |
| `<sid>.events.jsonl` | Out-of-band events: start, resize, gap, end |

ttyrec files are compatible with `ttyplay`, `ipbt`, and `termrec`.

## Security Model

- **Authentication:** `SO_PEERCRED` on the AF_UNIX socket provides kernel-attested peer UID/PID. The shim's claimed `loginuid` is cross-checked against `/proc/<pid>/loginuid`.
- **SID validation:** Session IDs are validated as UUID format before use in file paths, preventing path traversal attacks.
- **JSON safety:** All user-controlled fields (rhost, tty, TERM, etc.) are JSON-escaped before writing to audit files, preventing log injection.
- **Storage permissions:** `/var/lib/pmp-rec/` is `root:pmp-audit 0750`. Files are `0640`. Users cannot read their own recordings.
- **Daemon hardening:** `ProtectSystem=strict`, `NoNewPrivileges=yes`, `PrivateTmp=yes`, `RestrictAddressFamilies=AF_UNIX`, `MemoryDenyWriteExecute=yes`.

See `docs/SECURITY.md` for the full threat model.

## Re-entrancy

The shim sets `PMP_REC_ACTIVE=1` and `PMP_REC_SHIM_CHILD=1` in the child shell environment. `profile.d` and `zshenv` bail early if either is set, preventing infinite self-wrapping. Nested `sudo -i` re-enters PAM → fresh SID with `parent_sid` pointing to the outer session.

## Building Packages

```bash
make rpm     # produces release/pmp-rec-<version>.rpm
make deb     # produces release/pmp-rec_<version>_amd64.deb
```

## License

GPL-2.0. See [LICENSE](LICENSE).
