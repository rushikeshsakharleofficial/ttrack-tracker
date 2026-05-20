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
sudo systemctl enable --now pmp-recd.socket pmp-recd.service
sudo systemctl enable --now pmp-rec-purge.timer
```

## PAM hooks

See `scripts/pam.d/*.snippet` — add relevant lines to `/etc/pam.d/` files.

## List sessions

```bash
sudo pmp-rec-cli list
sudo pmp-rec-cli list --user alice
```

## Replay a session

```bash
sudo pmp-rec-cli play <sid>
sudo pmp-rec-cli play --speed 3 <sid>   # 3× speed
sudo pmp-rec-cli play --dump <sid>      # no timing delay (raw dump)
```

## View session tree (nested su/sudo)

```bash
sudo pmp-rec-cli tree <root-sid>
```

## Follow an active session

```bash
sudo pmp-rec-cli tail <sid>   # Ctrl-C to stop
```

## Purge old sessions

```bash
# Delete sessions older than 30 days
sudo pmp-rec-cli purge --older-than 30

# Dry run
sudo pmp-rec-cli purge --older-than 30 --dry-run

# Delete a specific session
sudo pmp-rec-cli purge --sid <sid>
```

## SELinux (RHEL 9)

pmp-recd writes to `/var/lib/pmp-rec`, which is outside the default
`var_t` label space. Create a custom policy:

```bash
# Generate policy from audit log denials
ausearch -c pmp-recd --raw | audit2allow -M pmp-rec
semodule -i pmp-rec.pp
```

Or use the provided `.te` policy (when available in M6+):
```bash
make -f /usr/share/selinux/devel/Makefile pmp-rec.pp
semodule -i pmp-rec.pp
```

## Daemon config

`/etc/pmp-rec/recd.conf`:
```ini
storage_dir      = /var/lib/pmp-rec
socket_path      = /run/pmp-recd.sock
max_session_mb   = 64
max_age_days     = 90
fail_closed      = 0        # 1 = deny session if daemon down
gzip_on_rotate   = 1
chattr_append_only = 0      # 1 = chattr +a on closed files
```

## Checking daemon status

```bash
systemctl status pmp-recd
journalctl -u pmp-recd -f
ls -la /var/lib/pmp-rec/$(date +%Y-%m-%d)/
```
