# CLAUDE.md — terminal-session-recorder

## Project

Audit-grade C terminal session recorder. Four binaries: `pmp-rec` (PTY shim), `pmp-recd` (root daemon), `pam_record.so` (PAM module), `pmp-rec-cli` (audit CLI).

Live deployment: Ubuntu 24.04, `89.167.44.42`, user `rushikesh.sakharle`, sudo available.

## Token Efficiency Rules

**Read before edit.** Never edit blind. Read the exact function, not the whole file — use `offset`+`limit`.

**Batch edits.** All changes to one file in one Edit call. All changes to related files in one response. Never: edit → ask → edit.

**No re-reads after edit.** Edit succeeded = file updated. Never `cat` a file you just wrote.

**One build per session.** Accumulate all code changes first, then `make` once. Never build after each file change.

**Grep before Read.** Unknown location → `grep -n` first, then Read only the relevant lines.

**Skip unchanged files.** If a file is not being modified, don't read it for context — only read what you need to change.

**Agents for big tasks.** Spawn `caveman:cavecrew-builder` for isolated 1-2 file edits. Spawn parallel agents for independent research. Never duplicate agent work in main thread.

## Deploy Pattern (always in this order)

```bash
# 1. All code changes done first
# 2. Single build
make 2>&1 | grep "error:" | head -10

# 3. Single scp + install
scp -i ~/.ssh/id_rsa -o IdentitiesOnly=yes build/<binary> rushikesh.sakharle@89.167.44.42:/tmp/
ssh -i ~/.ssh/id_rsa -o IdentitiesOnly=yes rushikesh.sakharle@89.167.44.42 "sudo install -m755 /tmp/<binary> /usr/<path>/<binary> && echo OK"

# 4. Verify
ssh -i ~/.ssh/id_rsa -o IdentitiesOnly=yes rushikesh.sakharle@89.167.44.42 "<check command>"
```

SSH key: `~/.ssh/id_rsa`, user: `rushikesh.sakharle`, host: `89.167.44.42`

## Install Paths (server)

| Binary | Path |
|--------|------|
| `pmp-rec` | `/usr/libexec/pmp-rec` |
| `pmp-recd` | `/usr/libexec/pmp-recd` |
| `pmp-rec-cli` | `/usr/bin/pmp-rec-cli` |
| `pam_record.so` | `/usr/lib/x86_64-linux-gnu/security/pam_record.so` |
| config | `/etc/pmp-rec/recd.conf` |
| profile.d | `/etc/profile.d/pmp-rec.sh` |
| storage | `/var/lib/pmp-rec/` |
| socket | `/run/pmp-recd.sock` |

## Build Targets

```bash
make all        # all 4 binaries
make rpm        # RPM package → release/
make deb        # DEB package → release/
make install    # install to system paths
```

Binaries in `build/`. Link flags: `-lutil` (shim), `-lsystemd -lz -lpthread` (daemon), `-lpam` (PAM), `-lncurses` (CLI).

## Key Files

```
src/shim/main.c          — PTY setup, /dev/tty fix, SID from env, fork
src/shim/loop.c          — poll loop: slave1↔master2↔daemon
src/shim/session.c       — daemon connect, HELLO frame, loginuid
src/shim/shell_resolve.c — shells.allow validation
src/shim/env_guard.c     — PMP_REC_ACTIVE / PMP_REC_SHIM_CHILD guards
src/daemon/server.c      — SO_PEERCRED auth, UUID SID validation, frame dispatch
src/daemon/session_store.c — per-session file open/write/close
src/daemon/meta.c        — meta.json write + finalize (json_escape applied)
src/daemon/rotate.c      — rotation stub (not yet implemented)
src/daemon/config.c      — recd.conf parser
src/pam/pam_record.c     — open_session / close_session
src/pam/pam_env_propagate.c — SID chain: old→parent, mint new
src/pam/pam_marker.c     — /run/pmp-rec/sessions/<sid>.json
src/cli/cmd_list.c       — list with STATUS/ACTIVE/EXITED columns
src/cli/cmd_play.c       — ttyrec playback, same-session loop guard
src/cli/cmd_tui.c        — ncurses interactive browser
```

## Known State (as of last session)

- Security fixes applied: JSON injection (json_escape in meta.c/session_store.c/pam_marker.c), path traversal (UUID validation in server.c)
- Play loop fixed: same-session and active-session guards in cmd_play.c
- NVM startup delay fixed: `~/.bashrc` on 89.167.44.42 skips NVM when `PMP_REC_SHIM_CHILD=1`
- `/dev/tty` fix in shim/main.c: bypasses bash fd1 redirect during profile.d sourcing
- Socket mode: still 0666 (P1 fix pending per PLAN.md)
- `service=unknown`: not fixed yet (Day 1 of PLAN.md)
- PAM `should_skip()` bug: not fixed yet (Day 1 of PLAN.md)
- Rotation: not implemented (Day 2 of PLAN.md)

## Commit Pattern

```bash
git add <only changed files>   # never git add -A
git commit -m "one-line summary

Body: what changed and why. No 'what' if code is self-evident."
git push
```

Then mirror to terminal-session-recorder repo if change is production-relevant:
```bash
cp -r <changed files> ~/terminal-session-recorder/<same path>
cd ~/terminal-session-recorder && git add <files> && git commit -m "..." && git push
```

## Debugging

**Check daemon logs:**
```bash
sudo journalctl -t pmp-recd --since='5 min ago' --no-pager
sudo journalctl -t pmp-rec --since='5 min ago' --no-pager
```

**Check running sessions:**
```bash
sudo pmp-rec-cli list
sudo ps aux | grep pmp-rec | grep -v grep
```

**Force a test session (no daemon, no profile.d):**
```bash
PMP_REC_NO_DAEMON=1 PMP_REC_FORCE_PTY=1 ./build/pmp-rec /bin/bash -c 'echo test; exit'
```

## Wire Protocol Quick Ref

Frames: `PMP_F_HELLO=1`, `PMP_F_OUT=2`, `PMP_F_RESIZE=3`, `PMP_F_CLOSE=4`, `PMP_F_HEARTBEAT=5`
Header: 28 bytes packed, all fields little-endian.
SID: UUID v4, exactly 36 chars `[0-9a-f-]`, validated in server.c before file path use.

## DO NOT — Commits

- Never add `Co-Authored-By:` lines to commits — not Claude, not anyone unless user explicitly asks

## DO NOT — Code

- Do not `isatty(STDOUT_FILENO)` in shim — bash redirects fd1 during profile.d sourcing
- Do not use nested C functions (GCC extension) — use static file-scope functions
- Do not hardcode `/usr/lib/security/` — path differs by distro (`x86_64-linux-gnu` on Ubuntu)
- Do not skip json_escape on any string field going into meta.json or events.jsonl
- Do not allow SID containing `/` or `..` to reach pmp_paths_build
- Do not `git add -A` — build artifacts go in `build/`, never tracked
