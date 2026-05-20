#!/bin/bash
# M1 smoke test: PTY shim in local-file mode (no daemon, no PAM)
# PMP_REC_FORCE_PTY=1 bypasses the isatty gate for automated testing.
set -uo pipefail

SHIM="${1:-./build/pmp-rec}"
OUTDIR="$(mktemp -d /tmp/pmp-rec-smoke-XXXXXX)"
trap 'rm -rf "$OUTDIR"' EXIT

export PMP_REC_OUTDIR="$OUTDIR"
export PMP_REC_NO_DAEMON=1
export PMP_REC_FORCE_PTY=1
unset PMP_REC_ACTIVE PMP_REC_SHIM_CHILD PMP_REC_SID PMP_REC_PARENT 2>/dev/null || true

PASS=0; FAIL=0

check() {
    local name="$1" result="$2"
    if [ "$result" = "pass" ]; then
        echo "OK  $name"; PASS=$((PASS+1))
    else
        echo "FAIL $name"; FAIL=$((FAIL+1))
    fi
}

# ── Test 1: Basic bash -c session, clean exit ─────────────────────────────────
rm -f "$OUTDIR"/*.ttyrec "$OUTDIR"/*.events.jsonl 2>/dev/null || true

"$SHIM" /bin/bash -c 'echo hello_test_1; exit 0' </dev/null > /dev/null 2>/dev/null
EXIT=$?

TTYREC_COUNT=$(ls "$OUTDIR"/*.ttyrec 2>/dev/null | wc -l)
EVENTS_COUNT=$(ls "$OUTDIR"/*.events.jsonl 2>/dev/null | wc -l)

[ $EXIT -eq 0 ]         && check "t1: exit code 0"         pass || check "t1: exit code 0"         fail
[ "$TTYREC_COUNT" -ge 1 ] && check "t1: ttyrec file created" pass || check "t1: ttyrec file created" fail
[ "$EVENTS_COUNT" -ge 1 ] && check "t1: events file created" pass || check "t1: events file created" fail

TTYREC=$(ls "$OUTDIR"/*.ttyrec 2>/dev/null | head -1)
if [ -f "${TTYREC:-/nonexistent}" ]; then
    SZ=$(stat -c%s "$TTYREC")
    [ "$SZ" -gt 0 ] && check "t1: ttyrec size > 0" pass || check "t1: ttyrec size > 0" fail

    # Validate first frame header: tv_sec (LE u32) should be a recent epoch second
    HDR_BYTES=$(dd if="$TTYREC" bs=1 count=4 2>/dev/null | od -An -tu4 | tr -d ' \n')
    NOW=$(date +%s)
    if [ -n "$HDR_BYTES" ] && [ "$HDR_BYTES" -gt $((NOW - 300)) ] && \
       [ "$HDR_BYTES" -le $((NOW + 60)) ] 2>/dev/null; then
        check "t1: tv_sec is recent" pass
    else
        check "t1: tv_sec is recent" fail
    fi
fi

EVENTS=$(ls "$OUTDIR"/*.events.jsonl 2>/dev/null | head -1)
if [ -f "${EVENTS:-/nonexistent}" ]; then
    grep -q '"type":"start"' "$EVENTS" && check "t1: events contain start" pass || \
        check "t1: events contain start" fail
fi

# ── Test 2: Non-tty stdin (no FORCE_PTY) → no ttyrec ─────────────────────────
rm -f "$OUTDIR"/*.ttyrec 2>/dev/null || true
unset PMP_REC_FORCE_PTY
echo "exit 0" | "$SHIM" /bin/bash 2>/dev/null || true
export PMP_REC_FORCE_PTY=1
TTYREC_COUNT2=$(ls "$OUTDIR"/*.ttyrec 2>/dev/null | wc -l)
[ "$TTYREC_COUNT2" -eq 0 ] && check "t2: non-tty stdin → no ttyrec" pass || \
    check "t2: non-tty stdin → no ttyrec" fail

# ── Test 3: Re-entrancy guard ──────────────────────────────────────────────────
rm -f "$OUTDIR"/*.ttyrec 2>/dev/null || true
PMP_REC_ACTIVE=1 "$SHIM" /bin/bash -c 'echo guard_test; exit 0' </dev/null >/dev/null 2>/dev/null || true
TTYREC_COUNT3=$(ls "$OUTDIR"/*.ttyrec 2>/dev/null | wc -l)
[ "$TTYREC_COUNT3" -eq 0 ] && check "t3: re-entrancy guard → no ttyrec" pass || \
    check "t3: re-entrancy guard → no ttyrec" fail

# ── Test 4: Content created for simple command ────────────────────────────────
rm -f "$OUTDIR"/*.ttyrec 2>/dev/null || true
"$SHIM" /bin/bash -c 'printf "%0.sX" {1..1000}; echo' </dev/null >/dev/null 2>/dev/null || true
TTYREC4=$(ls "$OUTDIR"/*.ttyrec 2>/dev/null | head -1)
if [ -f "${TTYREC4:-/nonexistent}" ]; then
    SZ4=$(stat -c%s "$TTYREC4")
    [ "$SZ4" -gt 100 ] && check "t4: ttyrec has content" pass || check "t4: ttyrec has content" fail
else
    check "t4: ttyrec has content" fail
fi

# ── Test 5: Exit code propagation ─────────────────────────────────────────────
"$SHIM" /bin/bash -c 'exit 42' </dev/null >/dev/null 2>/dev/null || EXIT5=$?
[ "${EXIT5:-0}" -eq 42 ] && check "t5: exit code 42 propagated" pass || \
    check "t5: exit code 42 propagated" fail

# ── Test 6: Events file structure ─────────────────────────────────────────────
rm -f "$OUTDIR"/*.ttyrec "$OUTDIR"/*.events.jsonl 2>/dev/null || true
"$SHIM" /bin/bash -c 'echo structure_test; exit 0' </dev/null >/dev/null 2>/dev/null || true
EVT6=$(ls "$OUTDIR"/*.events.jsonl 2>/dev/null | head -1)
if [ -f "${EVT6:-/nonexistent}" ]; then
    grep -q '"type":"start"' "$EVT6" && check "t6: events.jsonl has start" pass || \
        check "t6: events.jsonl has start" fail
    grep -q '"type":"end"' "$EVT6" && check "t6: events.jsonl has end" pass || \
        check "t6: events.jsonl has end" fail
else
    check "t6: events.jsonl has start" fail
    check "t6: events.jsonl has end" fail
fi

echo ""
echo "Results: $PASS passed, $FAIL failed (out of $((PASS+FAIL)) total)"
[ $FAIL -eq 0 ]
