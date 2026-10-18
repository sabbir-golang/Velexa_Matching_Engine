#!/usr/bin/env bash
# Backs up everything needed to run the engine elsewhere: the database (pg_dump),
# the data folder and .env, into backups/velexa-me-YYYYMMDD-HHMMSS.tar.gz (mode 0600 —
# it holds passwords). Restore it on the new machine with tools/docker-restore.sh.
#   make docker-backup
set -euo pipefail
cd "$(dirname "$0")/.."
ENV_FILE=${DOCKER_ENV_FILE:-.env}   # COMPOSE_PROJECT_NAME / COMPOSE_ENV_FILES select another deployment
# kv KEY [default]: a value from the env file, taken literally (quotes and
# trailing " # comments" removed) — never sourced as shell code.
kv() {
  local v
  v=$(grep -E "^$1=" "$ENV_FILE" 2>/dev/null | tail -1 | cut -d= -f2- | sed -E 's/[[:space:]]+#.*$//; s/[[:space:]]+$//')
  case "$v" in \'*\') v=${v:1:${#v}-2} ;; \"*\") v=${v:1:${#v}-2} ;; esac
  printf '%s' "${v:-${2:-}}"
}
DATA=$(kv DOCKER_DATA_DIR ./data)
STAMP=$(date +%Y%m%d-%H%M%S)
TMP=$(mktemp -d); trap 'rm -rf "$TMP"' EXIT
OUTDIR=${BACKUP_DIR:-backups}
mkdir -p "$OUTDIR" "$TMP/velexa-me"
docker compose exec -T db pg_dump -U velexa_me -d velexa_me --no-owner --no-privileges >"$TMP/velexa-me/db.sql"
# The data folder without the rotated FIX logs (the current one is kept).
tar -C "$DATA" --exclude='fix_messages.log.[0-9]*' -cf "$TMP/velexa-me/data.tar" .
cp "$ENV_FILE" "$TMP/velexa-me/env"
OUT="$OUTDIR/velexa-me-$STAMP.tar.gz"
umask 077
tar -C "$TMP" -czf "$OUT" velexa-me
echo "✓ $OUT ($(du -h "$OUT" | cut -f1)) — contains passwords: keep it private"
