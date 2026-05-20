# Operations

## Install

```bash
# Build
make all

# Install (as root)
sudo make install

# Or use the distro-aware installer
sudo bash scripts/install.sh
```

## Enable

```bash
sudo systemctl enable --now trackterm-recd.socket trackterm-recd.service
sudo systemctl enable --now trackterm-rec-purge.timer
```

## PAM hooks

See `scripts/pam.d/*.snippet` — add relevant lines to `/etc/pam.d/` files.

## List sessions

```bash
sudo trackterm-cli list
sudo trackterm-cli list --user alice
```

## Replay a session

```bash
sudo trackterm-cli play <sid>
sudo trackterm-cli play --speed 3 <sid>   # 3× speed
sudo trackterm-cli play --dump <sid>      # no timing delay (raw dump)
```

## View session tree (nested su/sudo)

```bash
sudo trackterm-cli tree <root-sid>
```

## Follow an active session

```bash
sudo trackterm-cli tail <sid>   # Ctrl-C to stop
```

## Purge old sessions

```bash
# Delete sessions older than 30 days
sudo trackterm-cli purge --older-than 30

# Dry run
sudo trackterm-cli purge --older-than 30 --dry-run

# Delete a specific session
sudo trackterm-cli purge --sid <sid>
```

## SELinux (RHEL 9)

trackterm-recd writes to `/var/lib/trackterm-rec`, which is outside the default
`var_t` label space. Create a custom policy:

```bash
# Generate policy from audit log denials
ausearch -c trackterm-recd --raw | audit2allow -M trackterm-rec
semodule -i trackterm-rec.pp
```

Or use the provided `.te` policy (when available in M6+):
```bash
make -f /usr/share/selinux/devel/Makefile trackterm-rec.pp
semodule -i trackterm-rec.pp
```

## Daemon config

`/etc/trackterm-rec/recd.conf`:
```ini
storage_dir      = /var/lib/trackterm-rec
socket_path      = /run/trackterm-recd.sock
max_session_mb   = 64
max_age_days     = 90
fail_closed      = 0        # 1 = deny session if daemon down
gzip_on_rotate   = 1
chattr_append_only = 0      # 1 = chattr +a on closed files
```

## Checking daemon status

```bash
systemctl status trackterm-recd
journalctl -u trackterm-recd -f
ls -la /var/lib/trackterm-rec/$(date +%Y-%m-%d)/
```
