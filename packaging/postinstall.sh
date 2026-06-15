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

# Older/manual installs may have placed a full ttrackd unit in /etc/systemd,
# which shadows the packaged unit in /lib/systemd and prevents upgrades from
# applying service fixes. Retire only units that look like ttrack's own legacy
# unit; administrators should use drop-ins for local overrides.
ETC_UNIT=/etc/systemd/system/ttrackd.service
PKG_UNIT=/lib/systemd/system/ttrackd.service
if [ -f "$ETC_UNIT" ] && [ -f "$PKG_UNIT" ] &&
    grep -q 'ttrack session recording collector' "$ETC_UNIT" &&
    grep -q '^ExecStart=/usr/libexec/ttrackd$' "$ETC_UNIT"; then
    if ! cmp -s "$ETC_UNIT" "$PKG_UNIT"; then
        cp -a "$ETC_UNIT" "${ETC_UNIT}.bak.$(date +%Y%m%d%H%M%S)" || true
        rm -f "$ETC_UNIT"
    fi
fi

# NOTE: SSH ForceCommand and interactive auto-record are NOT enabled here.
# Both hooks require explicit administrator opt-in to avoid altering SSH
# behavior on install. Enable them with:
#
#   sudo ttrack init --enable-ssh-forcecommand   (non-interactive SSH recording)
#   sudo ttrack init --enable-autorec             (interactive login recording)
#
# Example configs are installed at:
#   /usr/share/doc/ttrack/ttrack-ssh-wrap.conf.example  (sshd ForceCommand)
#   /usr/share/doc/ttrack/ttrack-autorec.sh.example     (profile.d hook)

# Enable and (re)start the collector daemon if systemd is present.
# The daemon generates the per-server encryption key on first start.
if command -v systemctl >/dev/null 2>&1; then
    systemctl daemon-reload || true
    systemctl enable ttrackd.service || true
    systemctl restart ttrackd.service || true
    # Wait briefly for the daemon to create the key.
    i=0
    while [ ! -f /var/lib/ttrack/.ttrack.key ] && [ "$i" -lt 30 ]; do
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
