#!/bin/sh
# ttrack pre-remove: disable ForceCommand BEFORE the wrapper binary is removed
# so SSH is never left broken mid-uninstall.
set -e

SSHD_CONF=/etc/ssh/sshd_config.d/zz-ttrack.conf
if [ -f "$SSHD_CONF" ]; then
    rm -f "$SSHD_CONF"
    # Reload sshd immediately so SSH works without the now-gone wrapper.
    if command -v systemctl >/dev/null 2>&1; then
        systemctl reload ssh 2>/dev/null || systemctl reload sshd 2>/dev/null || true
    fi
fi

exit 0
