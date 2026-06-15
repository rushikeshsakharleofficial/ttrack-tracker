# ttrack Fix Log

All fixes verified: `go vet ./...` + `go test ./... -count=1` pass clean.

---

## P0 — Critical Security / Correctness

### 1. Exit-code preservation (`internal/record`, `cmd/ttrack`)
`Run()` previously discarded the child process exit code. Commands like
`ttrack rec /bin/sh -c 'exit 7'` always exited 0. Added `ExitCodeError`
and `exitCodeError()` to extract the real exit code (including 128+signal
for signal-terminated children) and `os.Exit(ec.Code)` in `main()`.

### 2. Recording file permissions (`internal/record`)
`openSink` used `os.Create` (mode 0o666 before umask). Changed to
`os.OpenFile(..., 0o600)` so recording files are owner-readable only.

### 3. Live-tail race (subscribe-before-replay) (`internal/daemon`)
`addTailer` replayed the snapshot before subscribing; bytes written between
replay-end and subscribe were silently dropped. Fixed by subscribing first,
then replaying, so new bytes queue in the subscriber channel during replay.

### 4. Logger `Reopen` MultiWriter stacking (`internal/logger`)
Each `Reopen` call stacked a new `MultiWriter` on top of the previous one,
eventually logging to both old and new files and leaking fds. Refactored to
capture `baseWriter` lazily on the first `TeeToFile` call (so tests can
redirect `log.Writer()` before calling it), then always tee `baseWriter +
newFile`.

### 5. SIGHUP reload order (`cmd/ttrackd`)
SIGHUP handler called `logger.Reopen(old path)` before parsing the new
config, so a rotated log file with a new path was never opened. Fixed to
parse the fresh config first, then reopen at the new path.

### 6. `postinstall.sh` ForceCommand auto-injection (`packaging`)
The post-install script auto-modified `/etc/ssh/sshd_config` and reloaded
sshd without operator consent. Removed the auto-injection block. Operators
now run `sudo ttrack init --enable-ssh-forcecommand` explicitly.

### 7. SSH/auto-rec as explicit opt-in (`internal/initcmd`)
Added `ttrack init --enable-ssh-forcecommand` / `--disable-ssh-forcecommand`
and `--enable-auto-rec` / `--disable-auto-rec` subcommands so SSH recording
and shell profile hooks are installed only on explicit admin request.

---

## P1 — High-Impact Bugs

### 8. Session reference parsing (`internal/store`)
`FindCentral` only matched exact filenames. Added `ParseSessionRef` to
accept `user/stem`, `user/stem.cast`, `stem`, and `stem.cast` forms, with
ambiguity detection when multiple users share the same bare stem.

### 9. Rune-safe truncation (`internal/store`)
`trunc` used byte slicing; multibyte UTF-8 (emoji, CJK, Devanagari) could
be split mid-codepoint. Replaced with `[]rune` slicing. Ellipsis is counted
as one rune so `trunc("hello", 3)` → `"he…"` (3 runes total).

### 10. Crypto authentication error vs. EOF (`internal/crypto`)
GCM `Open` failure silently returned `io.EOF`, making tampered or corrupt
recordings appear as normal end-of-stream. Now returns a typed
`ErrAuthentication` error with the frame index so callers and humans can
distinguish truncation from tampering.

### 11. Backup credential exposure (`internal/backup`)
`buildBackupArgs` accepted arbitrary target strings, and errors included
full arg lists. Added `validateTarget` to reject targets with shell
metacharacters, and `redactOutput` to scrub credential-looking lines from
error output. Error messages omit args.

---

## P1 — CI/CD Hardening

### 12. `StrictHostKeyChecking=no` removed (`pipeline.yml`)
All SSH/SCP invocations in the deploy stage now use
`StrictHostKeyChecking=yes -o BatchMode=yes`. A new `JUMP_SERVER_KNOWN_HOST`
secret can hold a pre-pinned known_hosts line; the fallback is `ssh-keyscan`
(records the key at first deploy). Either way the key is verified on
subsequent deploys.

---

## P2 — Daemon Protocol Hardening

### 13. Handshake deadline + line length bound (`internal/daemon`)
The initial protocol line read had no deadline (idle clients held goroutines
forever) and no length bound (a client sending a line with no `\n` could
exhaust memory). Added a 10-second read deadline cleared after the handshake,
and a 256-byte reader so oversized lines are rejected immediately.

### 14. `REC` ACK (`internal/daemon`)
After accepting a `REC` connection the daemon now sends `OK <id>\n` before
the recorder begins streaming, giving the client an unambiguous signal that
the session is registered and any earlier `ERR` response was not missed.

---

## P2 — Shell Wrapper

### 15. Case-pattern trailing-space fix (`scripts/ttrack-ssh-wrap.sh`)
The `| \` line-continuation style in the `case` statement left a literal
space pattern (matching `" "`) between each real pattern. Replaced with
`|\` continuations (no space before backslash) to avoid the phantom match.
Added documentation clarifying that `TTRACK_REC=0` injected via
`SSH_ORIGINAL_COMMAND` cannot bypass the wrapper's `ttrack rec` invocation.
