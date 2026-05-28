# Configuration

`ttrack` and `ttrackd` need no config file. Behavior is controlled by environment variables, filesystem locations, and the systemd unit.

## Environment variables

| Variable | Default | Used by | Description |
|:---------|:--------|:--------|:------------|
| `TTRACK_DIR` | `~/.local/share/ttrack` | `ttrack` | User-local recordings directory (fail-open fallback + local `ls`/`play`). |
| `TTRACK_CENTRAL_DIR` | `/var/lib/ttrack` | `ttrack`, `ttrackd` | Central root-only store. |
| `TTRACKD_SOCK` | `/run/ttrackd.sock` | `ttrack`, `ttrackd` | Daemon unix socket path. |
| `TTRACK_QUIET` | unset | `ttrack rec` | Any non-empty value suppresses the recording banner and saved-path message. |
| `SHELL` | `/bin/bash` | `ttrack rec` | Shell launched when no command is given. |

## Filesystem layout

| Path | Owner / mode | Purpose |
|:-----|:-------------|:--------|
| `/usr/bin/ttrack` | `root 0755` | CLI binary |
| `/usr/libexec/ttrackd` | `root 0755` | daemon |
| `/var/lib/ttrack/` | `root:root 0700` | central store (no non-root access) |
| `/var/lib/ttrack/<user>/<id>.cast` | `root:root 0600` | encrypted recording |
| `/var/lib/ttrack/.ttrack.key` | `root:root 0600`, `chattr +i` | per-server AES-256-GCM key (immutable) |
| `/run/ttrackd.sock` | `root 0666` | recorder connect socket |
| `/etc/profile.d/ttrack-autorec.sh` | `root 0644` | optional auto-record login hook |
| `~/.local/share/ttrack/` | the user | local fail-open recordings |

## Daemon systemd unit

```bash
sudo systemctl status ttrackd
sudo systemctl restart ttrackd
sudo journalctl -u ttrackd --no-pager --since '10 min ago'
```

Override store or socket path with a systemd drop-in:

```bash
sudo systemctl edit ttrackd
```

Add in the editor:

```ini
[Service]
Environment=TTRACK_CENTRAL_DIR=/srv/ttrack
Environment=TTRACKD_SOCK=/run/ttrackd.sock
```

## Encryption key

The daemon creates a unique random key on first start: `/var/lib/ttrack/.ttrack.key` (`root:root 0600`, `chattr +i`).

**Back it up offsite.** Losing it makes every encrypted recording permanently unreadable. The daemon refuses to start if the key is missing while encrypted recordings exist.

To rotate the key:

```bash
# 1. Export all existing recordings to plaintext first
for id in $(sudo ttrack ls-user --all --ids); do
  sudo ttrack export -o "${id}.cast" "$id"
done

# 2. Remove the immutable flag, then the key
sudo chattr -i /var/lib/ttrack/.ttrack.key
sudo rm /var/lib/ttrack/.ttrack.key

# 3. Restart the daemon — generates a fresh key
sudo systemctl restart ttrackd
```

New recordings use the new key. Exported `.cast` files are plaintext asciinema.

## Bash completion

Installed by the package to `/usr/share/bash-completion/completions/ttrack`. Enable manually:

```bash
ttrack completion bash | sudo tee /usr/share/bash-completion/completions/ttrack
```

Completes subcommands, flags, local sessions (for `play`), and — as root — users and central session ids.

## Auto-record on login

The package installs `/etc/profile.d/ttrack-autorec.sh`. It:
- Triggers only for interactive shells with a real TTY.
- Skips nested shells (`sudo su -`, subshells) by detecting a `ttrack` process in the process ancestry — a session is recorded exactly once.
- Is fail-open: if the recorder cannot start, a normal shell continues.

Remove the file to disable:

```bash
sudo rm /etc/profile.d/ttrack-autorec.sh
```

## Non-interactive SSH recording

```bash
sudo cp /usr/share/doc/ttrack/sshd-forcecommand.conf.example \
        /etc/ssh/sshd_config.d/zz-ttrack.conf
sudo sshd -t && sudo systemctl reload ssh
```

- `scp`/`sftp`/`rsync`/git transfers pass through untouched.
- Interactive logins keep recording via the profile.d hook (no double-wrap).
- Fail-open: if anything is wrong, the command runs normally — SSH is never blocked.

Exclude a specific account (e.g. automation bot):

```
Match User *,!deploy-bot
    ForceCommand /usr/libexec/ttrack-ssh-wrap
```

Disable by removing `/etc/ssh/sshd_config.d/zz-ttrack.conf` and reloading sshd.
