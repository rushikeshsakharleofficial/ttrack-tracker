#!/bin/sh
# ttrack sshd ForceCommand wrapper.
#
# Wire it in sshd_config:  ForceCommand /usr/libexec/ttrack-ssh-wrap
#
# Behavior:
#   - Interactive login (no SSH command): exec the user's login shell. The
#     profile.d hook records interactive sessions, so we do not double-wrap.
#   - File-transfer / subsystem requests (scp, sftp, rsync, git pack): exec
#     them UNTOUCHED so transfers keep working.
#   - Any other remote command (`ssh host "cmd"`): record it with ttrack.
#
# Fail-open: on any doubt, exec the original command / login shell so SSH is
# never broken.
#
# TTRACK_REC bypass: this wrapper does NOT check the TTRACK_REC env var.
# The recording decision is made here unconditionally. Any TTRACK_REC=0 the
# user injects via their SSH command (e.g. `ssh host "TTRACK_REC=0 bash"`)
# only affects that inner shell's environment, NOT this wrapper, so it cannot
# suppress the outer `ttrack rec` invocation.

cmd="$SSH_ORIGINAL_COMMAND"
shell="${SHELL:-/bin/bash}"

# Interactive login — hand off to the login shell (profile.d does recording).
if [ -z "$cmd" ]; then
    exec "$shell" -l
fi

# Pass file-transfer / subsystem commands through untouched.
# Each pattern is on its own line with explicit | between them so there are no
# accidental "space" patterns from trailing whitespace before line continuations.
case "$cmd" in
    ttrack\ rec*|\
    */ttrack\ rec*|\
    scp\ *|\
    */scp\ *|\
    sftp-server*|\
    */sftp-server*|\
    internal-sftp*|\
    rsync\ --server*|\
    */rsync\ --server*|\
    git-receive-pack*|\
    git-upload-pack*|\
    git-upload-archive*)
        exec "$shell" -c "$cmd"
        ;;
esac

# Record the command session if ttrack is available; else run it plainly.
if command -v ttrack >/dev/null 2>&1; then
    exec ttrack rec -q "$shell" -c "$cmd"
fi
exec "$shell" -c "$cmd"
