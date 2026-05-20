# Architecture

## Overview

`trackterm-rec` records terminal sessions via a PTY-intercept shim and central daemon.

```
   sshd / login / su / sudo
            │
            ▼
       pam_record.so      ← PAM session_open: mint/chain SID
            │
            ▼
    /etc/profile.d/trackterm-rec.sh  (or /etc/zshenv for zsh)
            │ exec
            ▼
       trackterm-rec   (PTY shim, runs as user)
            │ AF_UNIX SOCK_STREAM
            ▼
       trackterm-recd  (daemon, epoll, root)
            │
            ▼
   /var/lib/trackterm-rec/<date>/<sid>.ttyrec
   /var/lib/trackterm-rec/<date>/<sid>.meta.json
   /var/lib/trackterm-rec/<date>/<sid>.events.jsonl
```

## Components

### trackterm-rec (shim)
PTY recorder running as the user. Allocated before the user's shell is exec'd.
- Opens PTY pair with `openpty()`
- Forks; child does `login_tty(slave)` then execs real shell
- Parent polls stdin→master (keyboard), master→stdout+daemon (output)
- Sends binary frames over AF_UNIX to `trackterm-recd`
- Guards against: non-tty stdin, re-entrancy, loop with itself

### trackterm-recd (daemon)
Root daemon. One epoll loop, per-client state machine.
- Authenticates via `SO_PEERCRED` and `/proc/<pid>/loginuid`
- Writes ttyrec, meta.json, events.jsonl per session
- Rotates and gzips completed files

### pam_record.so
PAM session module invoked before any session's shell is exec'd.
- Mints a new UUID session ID (SID) via `/proc/sys/kernel/random/uuid`
- Chains: promotes existing `TRACKTERM_REC_SID` to `TRACKTERM_REC_PARENT`, sets new SID
- Stamps env via `pam_putenv()` — picked up by the profile.d hook

### trackterm-cli
Audit CLI: `list`, `play`, `tail`, `purge`, `tree`.

## Session Nesting

```
SSH login → PAM: SID=A parent=""
  bash starts → profile.d execs trackterm-rec → records session A
  user runs: sudo -i
    sudo PAM: SID=B parent=A
      root bash → profile.d execs trackterm-rec → records session B
      root runs: su - alice
        su PAM: SID=C parent=B
          alice bash → records session C
```

`trackterm-cli tree <A>` reconstructs and displays this chain.

## Identity

`loginuid` from `/proc/self/loginuid` is set once by `pam_loginuid.so` and is
kernel-immutable across su/sudo within the same login session. Every shim
instance reads it and stamps the meta sidecar. This is the authoritative
"who actually logged in" field regardless of user switching.
