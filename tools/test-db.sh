#!/usr/bin/env bash
# Runs the Go tests against a throwaway PostgreSQL container (removed afterwards),
# so the database tests run instead of being skipped.
#   make test-db            or   tools/test-db.sh [go test packages...]
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
IMAGE=${PG_IMAGE:-postgres:16-alpine}
NAME="velexa_me_testpg_$$"
PKGS=("$@"); [ ${#PKGS[@]} -eq 0 ] && PKGS=(./...)

docker run -d --rm --name "$NAME" -e POSTGRES_PASSWORD=test -p 127.0.0.1::5432 "$IMAGE" >/dev/null
trap 'docker rm -f "$NAME" >/dev/null 2>&1 || true' EXIT
PORT=$(docker port "$NAME" 5432/tcp | head -1 | sed 's/.*://')
for _ in $(seq 1 60); do
  docker exec "$NAME" pg_isready -h 127.0.0.1 -U postgres -q && break
  sleep 0.5
done
echo "test database: $IMAGE on 127.0.0.1:$PORT"
cd "$ROOT"
ME_TEST_DB_URL="postgres://postgres:test@127.0.0.1:$PORT/postgres?sslmode=disable" go test -count=1 "${PKGS[@]}"
