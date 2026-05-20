#!/bin/sh
# Percona Monitoring Plugins — terminal session recorder shim launcher
# Installed to /etc/profile.d/trackterm-rec.sh
# Sourced by bash, sh, dash, ksh on login and interactive sessions.

# Interactive shells only
case $- in *i*) ;; *) return 0 2>/dev/null || exit 0 ;; esac

# Real TTY required on stdin (not scp / sftp / cron)
# NOTE: do NOT check stdout — bash redirects it during profile.d sourcing
[ -t 0 ] || return 0 2>/dev/null || exit 0

# Re-entrancy guard: already inside a trackterm-rec session
[ -n "${TRACKTERM_REC_ACTIVE:-}" ] && return 0 2>/dev/null || exit 0

# Guard: we are the child shell of trackterm-rec (prevents infinite re-exec)
[ "${TRACKTERM_REC_SHIM_CHILD:-0}" = "1" ] && return 0 2>/dev/null || exit 0

# Launch shim if available
# NOTE: do NOT set TRACKTERM_REC_ACTIVE here — the shim sets it in the child shell's
# env after fork. Setting it before exec causes the shim to see the flag and
# skip recording entirely.
if [ -x /usr/libexec/trackterm-rec ]; then
    exec /usr/libexec/trackterm-rec
fi
# If shim not installed, silently continue without recording
