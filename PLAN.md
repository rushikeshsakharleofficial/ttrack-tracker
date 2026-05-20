# Production Readiness — 1-Week Daily Plan

> Goal: staging-ready by Day 5, wide-rollout-ready by Day 7.
> Token efficiency: batch related file edits per session, fix → test → commit in one block. No revisits.

---

## Timetable Overview

| Day | Theme | P-level | Gate |
|-----|-------|---------|------|
| Mon | Code fixes (PAM + service bug + socket) | P0 + P1 | — |
| Tue | Rotation + size cap + chattr | P0 | — |
| Wed | SELinux + RHEL 9 install test | P0 | — |
| Thu | Valgrind + load test + reconnect | P1 | **Staging gate** |
| Fri | Fail-closed + throttle + purge timer | P2 + P3 | — |
| Sat | Monitoring + TUI test + runbook | P2 + P3 | — |
| Sun | Multi-distro + nested chain + final gate | P4 | **Wide-rollout gate** |

---

## Day 1 — Monday: Code Fixes (3 bugs, 1 session)

**Batch all code changes together → 1 build → 1 deploy. No repeat builds.**

### AM — Code (2–3 h)

**Fix 1: PAM `should_skip()` logic**
- File: `src/pam/pam_record.c`
- Bug: loop body returns 0 (don't skip) for matching user → exclusion never fires
- Fix: `return 1;` when `strcmp(user, skip_users[i]) == 0`

**Fix 2: service=unknown**
- File: `src/pam/pam_record.c` open_session + `src/shim/session.c`
- Root cause: `PAM_SERVICE` not in env when shim runs
- Fix: pam_record.so writes `PMP_REC_SERVICE=<service>` via `pam_putenv`; shim reads `getenv("PMP_REC_SERVICE")` with fallback to `getenv("PAM_SERVICE")`
- Update `sudoers.d/pmp-rec`: add `PMP_REC_SERVICE` to `env_keep`

**Fix 3: Socket 0666 → 0660 + group check**
- File: `src/daemon/server.c` + `src/daemon/main.c`
- Change socket mode to `0660`, group `pmp-audit`
- In `pmp_server_accept`: after SO_PEERCRED, check `cred.gid == pmp_audit_gid || supplementary_groups_contain(cred.pid, pmp_audit_gid)`. Helper: read `/proc/<pid>/status` for `Groups:` line.
- Update `scripts/install.sh`: `groupadd -r pmp-audit`, `usermod -aG pmp-audit` for pmp-rec binary via newgrp or setgid

### PM — Build + deploy + smoke test (1 h)
```bash
make all
sudo make install    # on staging host
# verify: SSH login records, service=sshd in meta.json
sudo pmp-rec-cli list   # check service column populated
```

### Evening — Commit (15 min)
```bash
git add src/pam/pam_record.c src/shim/session.c src/daemon/server.c \
        src/daemon/main.c scripts/sudoers.d/pmp-rec scripts/install.sh
git commit -m "Fix PAM skip logic, service propagation, socket hardening"
git push
```

---

## Day 2 — Tuesday: Rotation + Size Cap + chattr

### AM — Implement rotate.c (3 h)
File: `src/daemon/rotate.c`

**Per-session size cap:**
```c
// In pmp_store_open: set RLIMIT_FSIZE on the process, or
// track bytes_written in session_store_t; when > max_session_bytes,
// close current ttyrec, rename to <sid>.ttyrec.1, open fresh <sid>.ttyrec
// write {"type":"rotated","part":1} to events.jsonl
```

**Age-based purge (called from purge timer):**
```c
int pmp_rotate_purge_old(const char *storage_dir, int max_age_days);
// Walk storage_dir/<date>/ dirs; stat mtime; unlink triplet if age > max_age_days
```

**Gzip on close (background thread):**
```c
// After pmp_store_close: pthread_create → gzip_thread(ttyrec_path)
// gzip_thread: open src → deflate → write .ttyrec.gz → unlink .ttyrec
// Only if config gzip_on_rotate=true
```

**chattr +a after close:**
```c
// ioctl(fd, FS_IOC_SETFLAGS, FS_APPEND_FL) on closed ttyrec
// Silently skip if ioctl returns EOPNOTSUPP (non-ext4/xfs)
```

### PM — Wire rotation into daemon + test (2 h)
- Call `pmp_rotate_purge_old` from daemon main loop (every epoll cycle > 1h interval)
- Test: generate 5 sessions, set `max_age_days=0`, verify purge clears them
- Test: generate 50 MiB session, set `max_session_bytes=10MiB`, verify rotation splits files

### Evening — Commit (15 min)
```bash
git add src/daemon/rotate.c src/daemon/session_store.c src/daemon/main.c Makefile
git commit -m "Implement session rotation, size cap, gzip, chattr+a"
git push
```

---

## Day 3 — Wednesday: SELinux + RHEL 9 Clean Install

**Needs: Rocky 9 minimal VM (fresh).**

### AM — SELinux policy (2 h)
1. Install pmp-rec RPM on Rocky 9 with SELinux in permissive mode
2. Run SSH login → record session → check `ausearch -m avc -ts recent`
3. Pipe denials through `audit2allow -m pmp_rec`
4. Hand-edit `.te` to tighten: allow only the specific types needed
5. `checkmodule -M -m -o pmp_rec.mod pmp_rec.te && semodule_package ...`
6. Save to `scripts/selinux/pmp-rec.te` + `pmp-rec.if`
7. Add `semodule -i pmp-rec.pp` to RPM `%post` in `packaging/rpm/pmp-rec.spec`

### PM — Clean install gate test (2 h)

**Rocky 9:**
```bash
# Fresh VM, zero manual steps allowed after package install
sudo dnf install -y pmp-rec-*.rpm
sudo systemctl status pmp-recd   # must be active
ssh localhost                     # must generate recording
sudo pmp-rec-cli list             # must show session with correct service
```

**Ubuntu 24.04:**
```bash
sudo apt install ./pmp-rec_*.deb
# same verification flow
```

Fix any install issues found → rebuild packages → retest.

### Evening — Commit + tag (20 min)
```bash
git add scripts/selinux/ packaging/rpm/pmp-rec.spec
git commit -m "Add SELinux policy, fix RPM/DEB clean install issues"
git tag v0.1.0-rc1
git push --tags
```

---

## Day 4 — Thursday: Valgrind + Load Test + Reconnect ← STAGING GATE

### AM — Valgrind + ASan (2 h)
```bash
# Build with sanitizers
make CFLAGS_EXTRA="-fsanitize=address,undefined" all

# Valgrind on shim
valgrind --leak-check=full --error-exitcode=1 \
  PMP_REC_NO_DAEMON=1 ./build/pmp-rec /bin/bash -c 'ls; echo done'

# Valgrind on daemon (run daemon, then connect shim)
valgrind --leak-check=full ./build/pmp-recd &
PMP_REC_FORCE_PTY=1 ./build/pmp-rec /bin/bash -c 'exit 0'
```
Fix all leaks. Zero errors = pass.

### AM/PM — 50-session load test (1 h)
```bash
# Write tests/load/load_test.sh
for i in $(seq 1 50); do
  PMP_REC_NO_DAEMON=1 ./build/pmp-rec /bin/bash -c \
    "echo session_$i; dd if=/dev/urandom bs=4k count=100 | base64; exit 0" &
done
wait
# Verify: 50 ttyrec files exist, no zombie processes, daemon alive
sudo pmp-rec-cli list | wc -l   # expect 50+
```

### PM — Daemon reconnect test (1 h)
```bash
# Session running → kill daemon → wait 5s → restart daemon → verify shim reconnects
sudo systemctl start pmp-recd
PMP_REC_FORCE_PTY=1 ./build/pmp-rec /bin/bash &
SHIM_PID=$!
sleep 2
sudo systemctl stop pmp-recd
sleep 3
sudo systemctl start pmp-recd
sleep 2
kill $SHIM_PID
# Check events.jsonl for {"type":"gap"} entry
```

### Evening — Staging gate decision
- All P0 + P1 items green → **promote to staging host**
- Document any failures → fix before Day 5

---

## Day 5 — Friday: Fail-Closed + Throttle + Purge Timer

### AM — Fail-closed end-to-end (1.5 h)
- Set `fail_closed=true` in recd.conf on staging host
- Stop pmp-recd
- Attempt SSH login → must be blocked (exit 1 from shim, no shell)
- Start pmp-recd → SSH login works again
- Verify error message is human-readable

### AM — Token bucket throttle (1.5 h)
File: `src/daemon/session_store.c`
```c
// Add to pmp_session_store_t:
uint64_t token_bucket;      // current tokens (bytes)
time_t   token_refill_ts;   // last refill time
#define BUCKET_SUSTAINED  (2 * 1024 * 1024)   // 2 MiB/s
#define BUCKET_BURST      (16 * 1024 * 1024)  // 16 MiB

// In pmp_store_write_output:
// Refill tokens since last write; if tokens < len, drop + write throttle event
```

### PM — Purge timer smoke test (1 h)
```bash
sudo systemctl start pmp-rec-purge.service
sudo journalctl -u pmp-rec-purge --no-pager | grep purged
# Verify: old sessions removed, active sessions untouched, count logged
```

### Evening — Commit (15 min)
```bash
git add src/ scripts/systemd/pmp-rec-purge.*
git commit -m "Fail-closed, throttle, purge timer validation"
git push
```

---

## Day 6 — Saturday: Monitoring + TUI + Runbook

### AM — Structured syslog events (1.5 h)
Add key daemon events in consistent format parseable by rsyslog/Vector/Loki:
```
pmp-recd[PID]: EVENT=session_open sid=UUID loginuid=N service=sshd rhost=IP
pmp-recd[PID]: EVENT=session_close sid=UUID exit=N bytes=N duration=N
pmp-recd[PID]: EVENT=session_gap sid=UUID bytes_dropped=N
pmp-recd[PID]: EVENT=disk_warning used_pct=91 path=/var/lib/pmp-rec
```

### AM — TUI manual test pass (1 h)
Test all key bindings systematically:
- 0 sessions → no crash
- 50 sessions → scroll, colors correct
- Play EXITED session → works
- Play ACTIVE session → blocked with message
- Delete → confirm prompt → files removed
- Tail ACTIVE → streams live
- Resize terminal mid-TUI → redraws correctly

Fix any crashes or display bugs found.

### PM — Runbook (2 h)
File: `docs/RUNBOOK.md`

Sections:
1. **Daily ops** — check active sessions, disk usage command
2. **Stuck shim** — `sudo pkill -x pmp-rec` + verify no zombies
3. **Daemon restart without session loss** — shim reconnects, gap marker expected
4. **Remove user from recording** — add to `skip_users[]` in pam_record.c config
5. **Emergency disable** — `sudo mv /etc/profile.d/pmp-rec.sh /etc/profile.d/pmp-rec.sh.disabled`
6. **Replay session** — `sudo pmp-rec-cli play <sid>` (from different terminal)
7. **Export session** — `sudo pmp-rec-cli play --dump <sid> > session.txt`
8. **Log locations** — `/var/log/auth.log`, `journalctl -t pmp-recd`, `/var/log/audit/audit.log`
9. **Disk full** — purge command + gzip existing sessions manually

### Evening — Commit
```bash
git add src/daemon/ docs/RUNBOOK.md
git commit -m "Structured syslog events, runbook"
git push
```

---

## Day 7 — Sunday: Multi-Distro + Nested Chain + Final Gate

### AM — zsh test (30 min)
```bash
# On staging host with zsh installed
chsh -s /usr/bin/zsh testuser
ssh testuser@host
# Verify: recording starts via /etc/zshenv snippet
sudo pmp-rec-cli list | grep testuser
```

### AM — Nested chain test (1 h)
```bash
# SSH as alice → sudo -i → su - bob
ssh alice@staging
sudo -i
su - bob
# In bob's shell:
exit; exit; exit
# Back to alice
sudo pmp-rec-cli tree <alice-root-sid>
```
Expected output:
```
└─ <alice-sid>  alice  sshd  loginuid=1001
   └─ <sudo-sid>  root  sudo  loginuid=1001
      └─ <bob-sid>  bob  su  loginuid=1001
```
Verify all `loginuid` values are alice's (1001), not root or bob. This is the key forensic invariant.

### AM — Debian 12 clean install (1 h)
Same as Day 3 Rocky flow but on Debian 12 minimal VM.
Fix any distro-specific install issues.

### PM — Final gate checklist (2 h)

Run full M1–M5 smoke suite:
```bash
bash tests/smoke/m1_shim_local.sh
bash tests/smoke/m2_daemon_wire.sh
```

Run security checks:
```bash
cppcheck --enable=all src/ include/ 2>&1 | grep -E "error|warning" | grep -v "_GNU_SOURCE"
scan-build make all 2>&1 | tail -20
```

Confirm all items green:
- [ ] PAM skip_users works
- [ ] service=sshd/su/sudo in meta.json (not "unknown")
- [ ] Socket 0660 + pmp-audit group
- [ ] Rotation splits large sessions
- [ ] gzip compresses on close (if enabled)
- [ ] chattr +a set on closed sessions
- [ ] SELinux enforcing on RHEL 9 — zero AVC denials
- [ ] Clean install: Rocky 9, Ubuntu 24.04 (zero manual steps)
- [ ] Valgrind clean
- [ ] 50-session load test passes
- [ ] Reconnect + gap marker works
- [ ] Fail-closed blocks login when daemon down
- [ ] Throttle drops over-limit output + events.jsonl entry
- [ ] Purge timer removes old sessions, spares active
- [ ] TUI — all keys work, resize handled
- [ ] zsh session recorded
- [ ] su/sudo nested chain: parent_sid correct, loginuid invariant holds
- [ ] cppcheck + scan-build clean

### Evening — Release commit + tag
```bash
git add -A
git commit -m "v0.1.0: production-ready release

All P0-P4 gates passed:
- Rocky 9 + Ubuntu 24.04 clean install
- SELinux enforcing
- Valgrind clean
- 50-session load test
- Nested chain loginuid invariant verified
"
git tag v0.1.0
git push --tags
```

---

## Token / Cost Efficiency Notes

- **Batch file edits per session** — never edit 1 file then stop. Group related changes (Day 1: 3 bugs in 1 session).
- **Fix → build → test → commit as one block** — no partial states across sessions.
- **Read files once** — keep context, avoid re-reading same files.
- **Smoke tests before deep testing** — catch regressions early, avoid wasting load-test time on broken builds.
- **One VM per distro** — snapshot after clean install, reuse for all tests on that distro.
- **Day 3 = SELinux only on RHEL** — don't attempt on Ubuntu (irrelevant), save time.
- **No docs until Day 6** — runbook written after code is stable, not before.
