#!/bin/sh
# ttrack post-install: create the root-only central store and enable ttrackd.
set -e

# Central store: root-only so normal users cannot read recordings.
mkdir -p /var/lib/ttrack
chown root:root /var/lib/ttrack
chmod 0700 /var/lib/ttrack

# Enable and (re)start the collector daemon if systemd is present.
if command -v systemctl >/dev/null 2>&1; then
    systemctl daemon-reload || true
    systemctl enable ttrackd.service || true
    systemctl restart ttrackd.service || true
fi

exit 0
