#!/bin/sh
# ttrack post-install: create the root-only central store and enable ttrackd.
set -e

# Central store: root-only so normal users cannot read recordings.
mkdir -p /var/lib/ttrack
chown root:root /var/lib/ttrack
chmod 0700 /var/lib/ttrack

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
