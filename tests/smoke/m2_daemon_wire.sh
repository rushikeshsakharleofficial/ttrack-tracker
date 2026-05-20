#!/bin/bash
# M2 smoke test: shim talks to daemon over AF_UNIX
set -euo pipefail

SHIM="${1:-./build/pmp-rec}"
DAEMON="${2:-./build/pmp-recd}"
OUTDIR="$(mktemp -d /tmp/pmp-rec-m2-XXXXXX)"
SOCKPATH="$OUTDIR/test.sock"
STORAGEDIR="$OUTDIR/recordings"
CONFFILE="$OUTDIR/recd.conf"
trap 'kill $DAEMON_PID 2>/dev/null || true; rm -rf "$OUTDIR"' EXIT

PASS=0; FAIL=0
check() {
    local name="$1" cond="$2"
    if [ "$cond" = "0" ]; then echo "OK  $name"; PASS=$((PASS+1));
    else echo "FAIL $name"; FAIL=$((FAIL+1)); fi
}

mkdir -p "$STORAGEDIR"

cat > "$CONFFILE" << EOF
storage_dir = $STORAGEDIR
socket_path = $SOCKPATH
max_session_mb = 64
log_level = 7
EOF

# Start daemon
"$DAEMON" -c "$CONFFILE" &
DAEMON_PID=$!
sleep 0.5

check "daemon running" "$(kill -0 $DAEMON_PID 2>/dev/null && echo 0 || echo 1)"
check "socket exists"  "$([ -S "$SOCKPATH" ] && echo 0 || echo 1)"

# Run shim against daemon
export PMP_REC_SOCK="$SOCKPATH"  # custom socket path — daemon should respect this
unset PMP_REC_NO_DAEMON PMP_REC_ACTIVE PMP_REC_SHIM_CHILD

"$SHIM" /bin/bash << 'EOF'
echo daemon_test
sleep 0.1
exit 0
EOF

sleep 0.5

# Check recordings exist
TTYREC_COUNT=$(find "$STORAGEDIR" -name "*.ttyrec" 2>/dev/null | wc -l)
META_COUNT=$(find "$STORAGEDIR" -name "*.meta.json" 2>/dev/null | wc -l)
check "ttyrec created by daemon" "$([ $TTYREC_COUNT -ge 1 ] && echo 0 || echo 1)"
check "meta.json created by daemon" "$([ $META_COUNT -ge 1 ] && echo 0 || echo 1)"

# Check meta content
META=$(find "$STORAGEDIR" -name "*.meta.json" | head -1)
if [ -f "$META" ]; then
    check "meta has sid"        "$(grep -q '"sid"' "$META" && echo 0 || echo 1)"
    check "meta has start_ts"   "$(grep -q '"start_ts"' "$META" && echo 0 || echo 1)"
fi

kill $DAEMON_PID 2>/dev/null || true
echo ""
echo "Results: $PASS passed, $FAIL failed"
[ $FAIL -eq 0 ]
