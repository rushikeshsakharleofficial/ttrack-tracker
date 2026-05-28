#!/bin/sh
# ttrack post-install: create the root-only central store and enable ttrackd.
set -e

# Central store: root-only so normal users cannot read recordings.
mkdir -p /var/lib/ttrack
chown root:root /var/lib/ttrack
chmod 0700 /var/lib/ttrack

# Log directory: root-owned normal logfile for ttrackd alongside journald.
mkdir -p /var/log/ttrack
chown root:root /var/log/ttrack
chmod 0750 /var/log/ttrack

# Install default config on first deploy (never overwrite existing config).
TTRACK_CONF=/etc/ttrack/ttrack.conf
if [ ! -f "$TTRACK_CONF" ]; then
    mkdir -p /etc/ttrack
    if [ -f /usr/share/ttrack/ttrack.conf.example ]; then
        cp /usr/share/ttrack/ttrack.conf.example "$TTRACK_CONF"
        chmod 644 "$TTRACK_CONF"
    fi
fi

# Install ForceCommand sshd config to capture non-interactive SSH sessions.
# Idempotent: only write if not already present (preserves manual edits).
SSHD_MAIN=/etc/ssh/sshd_config
SSHD_CONF=/etc/ssh/sshd_config.d/zz-ttrack.conf
if [ -d /etc/ssh/sshd_config.d ]; then
    # Ensure main sshd_config includes the drop-in directory; without this
    # line the drop-in files are silently ignored by sshd.
    if [ -f "$SSHD_MAIN" ] && ! grep -qE '^Include\s+/etc/ssh/sshd_config\.d/\*' "$SSHD_MAIN"; then
        sed -i '1s|^|Include /etc/ssh/sshd_config.d/*.conf\n|' "$SSHD_MAIN"
    fi
    if [ ! -f "$SSHD_CONF" ]; then
        cat > "$SSHD_CONF" << 'SSHD_EOF'
# Installed by ttrack package. Remove this file to disable SSH session recording.
# The wrapper is fail-open: scp/sftp/rsync pass through untouched.
ForceCommand /usr/libexec/ttrack-ssh-wrap
SSHD_EOF
    fi
fi
# Validate and reload sshd if config is valid.
if command -v sshd >/dev/null 2>&1 && sshd -t 2>/dev/null; then
    if command -v systemctl >/dev/null 2>&1; then
        systemctl reload ssh 2>/dev/null || systemctl reload sshd 2>/dev/null || true
    fi
fi

# Enable and (re)start the collector daemon if systemd is present.
# The daemon generates the per-server encryption key on first start.
if command -v systemctl >/dev/null 2>&1; then
    systemctl daemon-reload || true
    systemctl enable ttrackd.service || true
    systemctl restart ttrackd.service || true
    # Wait briefly for the daemon to create the key.
    i=0
    while [ ! -f /var/lib/ttrack/.ttrack.key ] && [ "$i" -lt 5 ]; do
        sleep 1
        i=$((i + 1))
    done
fi

# Lock the key immutable (chattr +i): cannot be removed/modified by
# rm/vi/sed/>/tee, even by root, until `chattr -i`. Idempotent with the daemon.
if command -v chattr >/dev/null 2>&1 && [ -f /var/lib/ttrack/.ttrack.key ]; then
    chattr +i /var/lib/ttrack/.ttrack.key 2>/dev/null || true
fi

exit 0
