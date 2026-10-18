#!/usr/bin/env bash
# Restores a backup made by tools/docker-backup.sh: the .env (only if there is
# none yet), the data folder, and the database (replacing what is in it).
#   make docker-restore FILE=backups/velexa-me-20261017-120000.tar.gz
set -euo pipefail
cd "$(dirname "$0")/.."
ENV_FILE=${DOCKER_ENV_FILE:-.env}   # COMPOSE_PROJECT_NAME / COMPOSE_ENV_FILES select another deployment
FILE=${1:?usage: tools/docker-restore.sh backups/velexa-me-….tar.gz}
TMP=$(mktemp -d); trap 'rm -rf "$TMP"' EXIT
tar -C "$TMP" -xzf "$FILE"
B="$TMP/velexa-me"
[ -f "$B/db.sql" ] && [ -f "$B/data.tar" ] || { echo "!! $FILE is not a velexa-me backup"; exit 1; }

if [ -f "$ENV_FILE" ]; then
  echo "→ keeping the existing $ENV_FILE (the backup's is in the archive as velexa-me/env)"
else
  install -m 600 "$B/env" "$ENV_FILE" && echo "→ restored $ENV_FILE"
  # A new machine has its own IP and user: clear them so setup fills them in.
  sed -i -E 's/^(ME_PUB_IP|ME_INT_IP|DOCKER_UID|DOCKER_GID)=.*/\1=/' "$ENV_FILE"
  tools/docker-setup.sh >/dev/null && echo "→ set this host's IP ($(grep '^ME_PUB_IP=' "$ENV_FILE" | cut -d= -f2)) and user in $ENV_FILE"
fi
# kv KEY [default]: a value from the env file, taken literally (quotes and
# trailing " # comments" removed) — never sourced as shell code.
kv() {
  local v
  v=$(grep -E "^$1=" "$ENV_FILE" 2>/dev/null | tail -1 | cut -d= -f2- | sed -E 's/[[:space:]]+#.*$//; s/[[:space:]]+$//')
  case "$v" in \'*\') v=${v:1:${#v}-2} ;; \"*\") v=${v:1:${#v}-2} ;; esac
  printf '%s' "${v:-${2:-}}"
}
DATA=$(kv DOCKER_DATA_DIR ./data)

docker compose stop engine >/dev/null 2>&1 || true
mkdir -p "$DATA" && tar -C "$DATA" -xf "$B/data.tar" && echo "→ restored $DATA"
if [ "$(id -u)" = 0 ]; then chown -R "$(kv DOCKER_UID 10001):$(kv DOCKER_GID 10001)" "$DATA"; fi
docker compose up -d --wait db
docker compose exec -T db psql -q -U velexa_me -d velexa_me -c 'DROP SCHEMA public CASCADE; CREATE SCHEMA public;'
docker compose exec -T db psql -q -v ON_ERROR_STOP=1 -U velexa_me -d velexa_me <"$B/db.sql" >/dev/null
echo "→ restored the database ($(docker compose exec -T db psql -U velexa_me -d velexa_me -tAc 'select count(*) from users') accounts)"
echo "✓ restore done — start with: make docker-up"
