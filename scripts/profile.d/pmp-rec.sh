#!/bin/sh
# Percona Monitoring Plugins — terminal session recorder shim launcher
# Installed to /etc/profile.d/pmp-rec.sh
# Sourced by bash, sh, dash, ksh on login and interactive sessions.

# Interactive shells only
case $- in *i*) ;; *) return 0 2>/dev/null || exit 0 ;; esac

# Real TTY required on stdin (not scp / sftp / cron)
# NOTE: do NOT check stdout — bash redirects it during profile.d sourcing
[ -t 0 ] || return 0 2>/dev/null || exit 0

# Re-entrancy guard: already inside a pmp-rec session
[ -n "${PMP_REC_ACTIVE:-}" ] && return 0 2>/dev/null || exit 0

# Guard: we are the child shell of pmp-rec (prevents infinite re-exec)
[ "${PMP_REC_SHIM_CHILD:-0}" = "1" ] && return 0 2>/dev/null || exit 0

# Launch shim if available
# NOTE: do NOT set PMP_REC_ACTIVE here — the shim sets it in the child shell's
# env after fork. Setting it before exec causes the shim to see the flag and
# skip recording entirely.
if [ -x /usr/libexec/pmp-rec ]; then
    exec /usr/libexec/pmp-rec
fi
# If shim not installed, silently continue without recording
