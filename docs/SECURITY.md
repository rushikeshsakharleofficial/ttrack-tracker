# Security

## Threat Model

### In scope
- Audit trail of all interactive shell sessions by legitimately-authenticated users
- Tamper-resistance against unprivileged users
- Detection of session nesting / user switching

### Out of scope
- Privileged (root) users deliberately circumventing the recorder
- Kernel-level attackers
- Sessions that never go through the TTY layer (raw socket, kernel threads)

## Defense layers

### PTY layer (kernel-mediated)
Recording is at the PTY master, which the kernel mediates. All data passing
through the slave (what the shell reads/writes) passes through the master.
- User `exec /bin/sh` inside session: still inside the PTY. Still recorded.
- User unsets env vars: shim already started; irrelevant.
- LD_PRELOAD bypass: recording is in the parent process at the PTY layer.

### Identity anchoring
`loginuid` from `/proc/self/loginuid` is set by `pam_loginuid.so` at login
and is kernel-immutable across su/sudo boundaries (within the same mount
namespace). The daemon records both `loginuid` (human who logged in) and
`euid` (current effective user).

### File security
- Recordings: `root:trackterm-audit`, mode `0640`
- Storage dir: mode `0750`
- `chattr +a` (append-only) on closed files — adds friction even if root is
  compromised (requires `chattr -a` first, which leaves a trail in audit.log)

### Socket security
- Socket: `/run/trackterm-recd.sock`, mode `0666`
- All connections authenticated via `SO_PEERCRED` (kernel-attested uid/gid/pid)
- Daemon cross-references `hello.loginuid` against `/proc/<peer_pid>/loginuid`
- Mismatch from non-root clients is rejected

## Evasion scenarios

| Technique | Result |
|-----------|--------|
| `exec /bin/sh` | Still inside PTY → recorded |
| `unset TRACKTERM_REC_ACTIVE` | Shim already running → no effect |
| Kill shim process | PTY closes → child shell dies → session ends |
| `LD_PRELOAD` on child | Recording is in parent → no effect |
| Write to raw `/dev/pts/N` | Bypasses shim (accepted: requires CAP_SYS_TTY) |
| Root: `rm` recording file | `chattr +a` prevents; unlink still possible with `chattr -a` |
| Root: kill trackterm-recd | Session continues unrecorded; PAM close_session logs end marker |
