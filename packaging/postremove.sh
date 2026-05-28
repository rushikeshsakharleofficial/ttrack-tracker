#!/bin/sh
# ttrack post-remove: stop and disable ttrackd. Recordings in /var/lib/ttrack
# are intentionally left in place (audit data is not deleted on uninstall).

if command -v systemctl >/dev/null 2>&1; then
    systemctl disable ttrackd.service || true
    systemctl stop ttrackd.service || true
    systemctl daemon-reload || true
fi

exit 0
