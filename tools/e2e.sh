#!/usr/bin/env bash
# End-to-end system test: builds the engine, starts it as a fresh install in a
# temporary folder on spare ports (your data/ and me.env are not touched), runs
# tools/e2e_test.py and tools/ui_test.mjs against it, restarts it and checks
# that state survived.
#   make e2e           or   tools/e2e.sh [orders-per-broker]
#   E2E_DB=1 make e2e  — the same with accounts and audit in a throwaway PostgreSQL
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
DBC=""
cleanup() { stop; [ -n "$DBC" ] && docker rm -f "$DBC" >/dev/null 2>&1; rm -rf "$WORK"; }
trap cleanup EXIT

(cd "$ROOT" && go build -o "$WORK/me" .)

DB_ENV=()
if [ "${E2E_DB:-0}" = 1 ]; then
  DBC="velexa_me_e2epg_$$"
  docker run -d --rm --name "$DBC" -e POSTGRES_PASSWORD=e2e -p 127.0.0.1::5432 postgres:16-alpine >/dev/null
  DBPORT=$(docker port "$DBC" 5432/tcp | head -1 | sed 's/.*://')
  for _ in $(seq 1 60); do docker exec "$DBC" pg_isready -h 127.0.0.1 -U postgres -q && break; sleep 0.5; done
  DB_ENV=(ME_DB_URL="postgres://postgres:e2e@127.0.0.1:$DBPORT/velexa_me?sslmode=disable")
  echo "== using PostgreSQL on 127.0.0.1:$DBPORT for accounts and audit"
fi
cp -r "$ROOT/spec" "$WORK/spec"

start() {
  for p in $HTTP $FIX $ITCH; do
    if port_busy "$p"; then echo "port $p is already in use — stop whatever runs there (or set E2E_*_PORT)"; exit 1; fi
  done
  # Background the engine process itself (not a subshell), so $! is its PID.
  cd "$WORK"
  env -i PATH="$PATH" HOME="$HOME" PORT=$HTTP FIX_PORT=$FIX ITCH_PORT=$ITCH \
    ME_ITCH_INDEX_PORT=$((ITCH + 1)) ME_ITCH_NEWS_PORT=$((ITCH + 2)) ME_SESSION_AUTO=off "${DB_ENV[@]}" \
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
python3 -I "$ROOT/tools/e2e_test.py" "${ARGS[@]}" --dump-market "$WORK/market.json" || rc=$?
stop
start
python3 -I "$ROOT/tools/e2e_test.py" "${ARGS[@]}" --after-restart "$WORK/creds.json" --market "$WORK/market.json" || rc=$?
if [ "$OB_BEFORE" = "$(md5sum "$WORK/data/orderbook_ids.json" | cut -d' ' -f1)" ]; then
  echo "PER-05   PASS  Order book IDs are unchanged across the restart"
else
  echo "PER-05   FAIL  Order book IDs changed across the restart"; rc=1
fi
if grep -q "\[state\] restored the market" "$WORK/engine.log"; then
  echo "PER-09   PASS  $(grep -o '\[state\] restored the market.*' "$WORK/engine.log" | tail -1 | cut -c1-150)"
else
  echo "PER-09   FAIL  the engine log does not show the market being restored"; rc=1
fi
if [ -n "$DBC" ]; then
  q() { docker exec "$DBC" psql -U postgres -d velexa_me -tAc "$1"; }
  if [ ! -e "$WORK/data/users.json" ] && [ "$(q 'select count(*) from users')" -ge 2 ] && [ "$(q 'select count(*) from audit_log')" -ge 5 ]; then
    echo "DB-01    PASS  Accounts ($(q 'select count(*) from users')) and audit entries ($(q 'select count(*) from audit_log')) are in PostgreSQL, no users.json"
  else
    echo "DB-01    FAIL  accounts/audit not in PostgreSQL (users.json present: $([ -e "$WORK/data/users.json" ] && echo yes || echo no))"; rc=1
  fi
fi
if grep -qE "panic|DATA RACE|fatal error" "$WORK/engine.log"; then
  echo "LOG-01   FAIL  engine log has a panic/race:"; grep -nE "panic|DATA RACE|fatal error" "$WORK/engine.log" | head; rc=1
else
  echo "LOG-01   PASS  No panic or fatal error in the engine log"
fi
exit $rc
