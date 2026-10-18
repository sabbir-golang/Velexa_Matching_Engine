#!/usr/bin/env bash
# Sets up the engine's own PostgreSQL on this PC and points me.env at it.
#   make db
# Creates (or starts) the container velexa_me_pg — postgres:16-alpine on
# 127.0.0.1:5435, data in the volume velexa_me_pg_data — with a random password
# for the user velexa_me, and adds ME_DB_URL to me.env (mode 0600). Safe to run
# again: an existing container is just started, and the password never changes.
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
ENV_FILE="${ME_ENV_FILE:-$ROOT/me.env}"
NAME=${ME_DB_CONTAINER:-velexa_me_pg}
PORT=${ME_DB_PORT:-5435}
IMAGE=${PG_IMAGE:-postgres:16-alpine}
DBUSER=velexa_me
DBNAME=velexa_me

if grep -q '^ME_DB_URL=' "$ENV_FILE" 2>/dev/null; then
  HAVE_URL=1
else
  HAVE_URL=0
fi

if docker container inspect "$NAME" >/dev/null 2>&1; then
  echo "→ starting existing container $NAME"
  docker start "$NAME" >/dev/null
  if [ "$HAVE_URL" = 0 ]; then
    echo "!! $NAME exists but $ENV_FILE has no ME_DB_URL, and its password is not stored anywhere else."
    echo "   Add ME_DB_URL by hand, or remove the container and its volume to start over:"
    echo "   docker rm -f $NAME && docker volume rm ${NAME}_data"
    exit 1
  fi
else
  if [ "$HAVE_URL" = 1 ]; then
    echo "!! $ENV_FILE already has ME_DB_URL but the container $NAME does not exist — not overwriting it."
    exit 1
  fi
  PASS=$(head -c 24 /dev/urandom | base64 | tr -dc 'A-Za-z0-9' | head -c 28)
  echo "→ creating container $NAME ($IMAGE, 127.0.0.1:$PORT, volume ${NAME}_data)"
  docker run -d --name "$NAME" --restart unless-stopped \
    -e POSTGRES_USER="$DBUSER" -e POSTGRES_PASSWORD="$PASS" -e POSTGRES_DB="$DBNAME" \
    -p "127.0.0.1:$PORT:5432" -v "${NAME}_data:/var/lib/postgresql/data" "$IMAGE" >/dev/null
  touch "$ENV_FILE"; chmod 600 "$ENV_FILE"
  {
    echo ""
    echo "# PostgreSQL for console accounts and the audit trail (added by tools/db-setup.sh)"
    echo "ME_DB_URL=postgres://$DBUSER:$PASS@127.0.0.1:$PORT/$DBNAME?sslmode=disable"
  } >>"$ENV_FILE"
  echo "→ added ME_DB_URL to $ENV_FILE"
fi

for _ in $(seq 1 60); do
  docker exec "$NAME" pg_isready -h 127.0.0.1 -U "$DBUSER" -d "$DBNAME" -q && { echo "✓ $NAME is ready on 127.0.0.1:$PORT — restart the engine to use it"; exit 0; }
  sleep 0.5
done
echo "!! $NAME did not become ready; see: docker logs $NAME"; exit 1
