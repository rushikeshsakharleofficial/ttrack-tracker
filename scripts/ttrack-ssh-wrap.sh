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

cmd="$SSH_ORIGINAL_COMMAND"
shell="${SHELL:-/bin/bash}"

# Interactive login — hand off to the login shell (profile.d does recording).
if [ -z "$cmd" ]; then
    exec "$shell" -l
fi

# Pass file-transfer / subsystem commands through untouched.
case "$cmd" in
    scp\ *|*/scp\ *| \
    sftp-server*|*/sftp-server*|internal-sftp*| \
    rsync\ --server*|*/rsync\ --server*| \
    git-receive-pack*|git-upload-pack*|git-upload-archive*)
        exec "$shell" -c "$cmd"
        ;;
esac

# Record the command session if ttrack is available; else run it plainly.
if command -v ttrack >/dev/null 2>&1; then
    exec ttrack rec -q "$shell" -c "$cmd"
fi
exec "$shell" -c "$cmd"
