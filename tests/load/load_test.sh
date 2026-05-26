#!/usr/bin/env bash
# 50-session concurrent load test
set -euo pipefail

BINARY="./build/trackterm-rec"
N=${1:-50}

echo "Starting $N concurrent sessions..."
PIDS=()

for i in $(seq 1 "$N"); do
    TRACKTERM_REC_NO_DAEMON=1 TRACKTERM_REC_FORCE_PTY=1 \
        "$BINARY" /bin/bash -c \
        "echo session_$i; dd if=/dev/urandom bs=4k count=20 2>/dev/null | base64; exit 0" \
        >/dev/null 2>&1 &
    PIDS+=($!)
done

echo "Waiting for $N sessions to complete..."
FAILED=0
for pid in "${PIDS[@]}"; do
    if ! wait "$pid"; then
        ((FAILED++)) || true
    fi
done

# Check no zombie trackterm-rec processes remain
ZOMBIES=$(ps aux | grep "[t]rackterm-rec" | grep -c " Z " || true)

echo "Results:"
echo "  Sessions launched: $N"
echo "  Failed:            $FAILED"
echo "  Zombie processes:  $ZOMBIES"

if [[ $FAILED -eq 0 && $ZOMBIES -eq 0 ]]; then
    echo "PASS"
    exit 0
else
    echo "FAIL"
    exit 1
fi
