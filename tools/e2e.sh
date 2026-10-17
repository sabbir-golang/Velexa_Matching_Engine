#!/usr/bin/env bash
# End-to-end system test: builds the engine, starts it as a fresh install in a
# temporary folder on spare ports (your data/ and me.env are not touched), runs
# tools/e2e_test.py and tools/ui_test.mjs against it, restarts it and checks
# that state survived.
#   make e2e           or   tools/e2e.sh [orders-per-broker]
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
LOAD="${1:-2000}"
WORK="$(mktemp -d)"
HTTP=${E2E_HTTP_PORT:-18910} FIX=${E2E_FIX_PORT:-15901} ITCH=${E2E_ITCH_PORT:-15951}
PID=""
port_busy() { (exec 3<>"/dev/tcp/127.0.0.1/$1") 2>/dev/null; }
stop() { # stop the engine and wait until it has really exited
  [ -n "$PID" ] || return 0
  kill "$PID" 2>/dev/null || true
  for _ in $(seq 1 100); do kill -0 "$PID" 2>/dev/null || break; sleep 0.1; done
  kill -9 "$PID" 2>/dev/null || true
  PID=""
}
cleanup() { stop; rm -rf "$WORK"; }
trap cleanup EXIT

(cd "$ROOT" && go build -o "$WORK/me" .)
cp -r "$ROOT/spec" "$WORK/spec"

start() {
  for p in $HTTP $FIX $ITCH; do
    if port_busy "$p"; then echo "port $p is already in use — stop whatever runs there (or set E2E_*_PORT)"; exit 1; fi
  done
  # Background the engine process itself (not a subshell), so $! is its PID.
  cd "$WORK"
  env -i PATH="$PATH" HOME="$HOME" PORT=$HTTP FIX_PORT=$FIX ITCH_PORT=$ITCH \
    ME_ITCH_INDEX_PORT=$((ITCH + 1)) ME_ITCH_NEWS_PORT=$((ITCH + 2)) ME_SESSION_AUTO=off \
    ./me >>"$WORK/engine.log" 2>&1 &
  PID=$!
  cd - >/dev/null
  for _ in $(seq 1 80); do
    kill -0 "$PID" 2>/dev/null || break
    curl -fs "http://127.0.0.1:$HTTP/healthz" >/dev/null 2>&1 && return 0
    sleep 0.25
  done
  echo "engine did not start; log:"; tail -20 "$WORK/engine.log"; exit 1
}

start
PASS=$(grep '^ME_ADMIN_PASS=' "$WORK/me.env" | cut -d= -f2-)
ARGS=(--http "$HTTP" --fix "$FIX" --itch "$ITCH" --admin-pass "$PASS")
echo "== system test (fresh install, $LOAD orders per broker)"
rc=0
python3 -I "$ROOT/tools/e2e_test.py" "${ARGS[@]}" --load "$LOAD" --save "$WORK/creds.json" || rc=$?

echo; echo "== browser UI"
CHROME_BIN=$(command -v google-chrome || command -v chromium || command -v chromium-browser || true)
if command -v node >/dev/null && [ -n "$CHROME_BIN" ]; then
  ME_URL="http://127.0.0.1:$HTTP" ADMIN_PASS="$PASS" CHROME="$CHROME_BIN" \
    timeout 180 node "$ROOT/tools/ui_test.mjs" "$WORK/shots" || rc=$?
else
  echo "UI       SKIP  node and google-chrome/chromium are needed for tools/ui_test.mjs"
fi

echo; echo "== restart and persistence"
OB_BEFORE=$(md5sum "$WORK/data/orderbook_ids.json" | cut -d' ' -f1)
stop
start
python3 -I "$ROOT/tools/e2e_test.py" "${ARGS[@]}" --after-restart "$WORK/creds.json" || rc=$?
if [ "$OB_BEFORE" = "$(md5sum "$WORK/data/orderbook_ids.json" | cut -d' ' -f1)" ]; then
  echo "PER-05   PASS  Order book IDs are unchanged across the restart"
else
  echo "PER-05   FAIL  Order book IDs changed across the restart"; rc=1
fi
if grep -qE "panic|DATA RACE|fatal error" "$WORK/engine.log"; then
  echo "LOG-01   FAIL  engine log has a panic/race:"; grep -nE "panic|DATA RACE|fatal error" "$WORK/engine.log" | head; rc=1
else
  echo "LOG-01   PASS  No panic or fatal error in the engine log"
fi
exit $rc
