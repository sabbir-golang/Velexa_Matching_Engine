#!/usr/bin/env bash
# System test against the Docker deployment: builds the image, starts engine +
# PostgreSQL as a separate compose project on spare ports with its own data
# folder and database volume, runs tools/e2e_test.py, restarts the engine
# container, checks persistence, a clean stop, and a backup -> wipe -> restore round trip,
# then removes everything. Your .env, data/ and running containers are not touched.
#   make docker-test
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
P=velexa-me-e2e
# Inside the project: a snap-installed Docker cannot see /tmp.
WORK=$(mktemp -d "$ROOT/.docker-e2e.XXXXXX")
HTTP=${E2E_HTTP_PORT:-18810} FIX=${E2E_FIX_PORT:-15801} ITCH=${E2E_ITCH_PORT:-15851}
DC=(docker compose -p "$P" -f "$ROOT/docker-compose.yml" --project-directory "$ROOT" --env-file "$WORK/.env")
cleanup() { "${DC[@]}" down -v --remove-orphans >/dev/null 2>&1 || true; rm -rf "$WORK"; }
trap cleanup EXIT
rc=0

mkdir -p "$WORK/data"
PASS="Me-$(head -c 32 /dev/urandom | base64 | tr -dc A-Za-z0-9 | head -c 20)-7q"
cat >"$WORK/.env" <<ENV
DOCKER_ENV_FILE=$WORK/.env
DOCKER_DATA_DIR=$WORK/data
DOCKER_BIND=127.0.0.1
DOCKER_HTTP_PORT=$HTTP
DOCKER_FIX_PORT=$FIX
DOCKER_ITCH_PORT=$ITCH
DOCKER_ITCH_INDEX_PORT=$((ITCH + 1))
DOCKER_ITCH_NEWS_PORT=$((ITCH + 2))
DOCKER_UID=$(id -u)
DOCKER_GID=$(id -g)
ME_VERSION=e2e
DB_PASSWORD='$(head -c 32 /dev/urandom | base64 | tr -dc A-Za-z0-9 | head -c 24)'
ME_PUB_IP=127.0.0.1
ME_INT_IP=127.0.0.1
ME_ADMIN_USER=admin
ME_ADMIN_PASS='$PASS'
ME_ADMIN_SECRET='$(head -c 64 /dev/urandom | base64 | tr -dc A-Za-z0-9 | head -c 64)'
ME_SESSION_AUTO=off
ENV

echo "== build and start ($P)"
"${DC[@]}" up -d --build --wait 2>&1 | grep -vE "^\s*$" | tail -3
"${DC[@]}" ps --format '{{.Service}} {{.Status}}'

ARGS=(--http "$HTTP" --fix "$FIX" --itch "$ITCH" --admin-pass "$PASS")
echo; echo "== system test against the containers"
python3 -I "$ROOT/tools/e2e_test.py" "${ARGS[@]}" --load 1000 --save "$WORK/creds.json" || rc=$?

echo; echo "== restart the engine container"
t0=$(date +%s)
"${DC[@]}" stop engine >/dev/null
dt=$(( $(date +%s) - t0 ))
if "${DC[@]}" logs engine 2>&1 | grep -q "stopped$" && [ "$dt" -lt 15 ]; then
  echo "DOCK-01  PASS  Graceful stop on SIGTERM in ${dt}s (brokers logged out, database closed)"
else
  echo "DOCK-01  FAIL  no clean stop (took ${dt}s)"; rc=1
fi
"${DC[@]}" start engine >/dev/null
"${DC[@]}" up -d --wait engine >/dev/null 2>&1
python3 -I "$ROOT/tools/e2e_test.py" "${ARGS[@]}" --after-restart "$WORK/creds.json" || rc=$?

echo; echo "== backup, wipe everything, restore (moving to a new server)"
export COMPOSE_PROJECT_NAME=$P COMPOSE_FILE="$ROOT/docker-compose.yml" COMPOSE_ENV_FILES="$WORK/.env" \
       DOCKER_ENV_FILE="$WORK/.env" BACKUP_DIR="$WORK/backups"
"$ROOT/tools/docker-backup.sh" | sed "s|$WORK/||"
ARCH=$(ls "$WORK"/backups/velexa-me-*.tar.gz | head -1)
"${DC[@]}" down -v >/dev/null 2>&1          # containers and the database volume are gone
rm -rf "$WORK/data" && mkdir -p "$WORK/data"  # and so is the data folder
"$ROOT/tools/docker-restore.sh" "$ARCH" | sed "s|$WORK/||"
"${DC[@]}" up -d --wait >/dev/null 2>&1
python3 -I "$ROOT/tools/e2e_test.py" "${ARGS[@]}" --after-restore "$WORK/creds.json" || rc=$?

echo; echo "== container checks"
U=$("${DC[@]}" exec -T engine id -u)
RO=$("${DC[@]}" exec -T engine sh -c 'touch /app/x 2>&1 || true')
H=$(docker inspect -f '{{.State.Health.Status}}' "$("${DC[@]}" ps -q engine)")
V=$("${DC[@]}" exec -T engine /app/velexa-sim-me version)
[ "$U" != 0 ] && echo "DOCK-02  PASS  Engine runs as uid $U (not root)" || { echo "DOCK-02  FAIL  runs as root"; rc=1; }
case "$RO" in *"Read-only"*) echo "DOCK-03  PASS  Root filesystem is read-only";; *) echo "DOCK-03  FAIL  root filesystem writable"; rc=1;; esac
[ "$H" = healthy ] && echo "DOCK-04  PASS  Docker health check: $H (version $V)" || { echo "DOCK-04  FAIL  health $H"; rc=1; }
if "${DC[@]}" exec -T db psql -U velexa_me -d velexa_me -tAc "select count(*) from users" | grep -qE '^[1-9]'; then
  echo "DOCK-05  PASS  Accounts are in the compose PostgreSQL; data files in $WORK/data: $(ls "$WORK/data" | tr '\n' ' ')"
else
  echo "DOCK-05  FAIL  no accounts in the database"; rc=1
fi
if "${DC[@]}" logs engine 2>&1 | grep -qE "panic|fatal error|DATA RACE"; then
  echo "LOG-01   FAIL  panic in the engine log"; rc=1
else
  echo "LOG-01   PASS  No panic or fatal error in the engine log"
fi
exit $rc
