#!/usr/bin/env bash
# Starts the live-reload stack: the database as usual and the engine under Air
# (docker-compose.dev.yml). Edit any .go file and the engine rebuilds and
# restarts. Ctrl+C stops following the log; the stack keeps running.
#   make dev                 make docker-up   (back to the production image)
set -euo pipefail
cd "$(dirname "$0")/.."
[ -f .env ] || tools/docker-setup.sh
mkdir -p tmp/home tmp/gocache tmp/gomod
# A go.mod of its own keeps the module cache in tmp/ out of `go test ./...`.
[ -f tmp/go.mod ] || printf 'module devcache\n' > tmp/go.mod
DC=(docker compose -f docker-compose.yml -f docker-compose.dev.yml)
"${DC[@]}" build engine
"${DC[@]}" up -d db
"${DC[@]}" up -d --force-recreate engine
echo "✓ live reload is running — save a .go file to rebuild. Following the engine log (Ctrl+C to stop following):"
exec "${DC[@]}" logs -f --tail=50 engine
