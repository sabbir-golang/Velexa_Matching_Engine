#!/usr/bin/env bash
# Builds and starts the engine and its database with docker compose.
#   make docker-up
# First time: writes .env (tools/docker-setup.sh). If accounts were kept in
# the stand-alone velexa_me_pg container (make db), they are copied into the
# compose database once, before the engine first starts.
set -euo pipefail
cd "$(dirname "$0")/.."
DC=(docker compose)
ENV_FILE=${DOCKER_ENV_FILE:-.env}
[ -f "$ENV_FILE" ] || tools/docker-setup.sh
# kv KEY [default]: a value from the env file, taken literally (quotes and
# trailing " # comments" removed) — never sourced as shell code.
kv() {
  local v
  v=$(grep -E "^$1=" "$ENV_FILE" 2>/dev/null | tail -1 | cut -d= -f2- | sed -E 's/[[:space:]]+#.*$//; s/[[:space:]]+$//')
  case "$v" in \'*\') v=${v:1:${#v}-2} ;; \"*\") v=${v:1:${#v}-2} ;; esac
  printf '%s' "${v:-${2:-}}"
}

export ME_VERSION=${ME_VERSION_OVERRIDE:-$(git describe --tags --always --dirty 2>/dev/null || echo dev)}

# The ports must be free (stop an engine started with "make run" first).
if ! "${DC[@]}" ps --status running -q engine 2>/dev/null | grep -q .; then
  for p in $(kv DOCKER_HTTP_PORT 8010) $(kv DOCKER_FIX_PORT 5001) $(kv DOCKER_ITCH_PORT 5501); do
    if (exec 3<>"/dev/tcp/127.0.0.1/$p") 2>/dev/null; then
      echo "!! port $p is in use — stop the engine running outside Docker first (or change DOCKER_*_PORT in .env)"; exit 1
    fi
  done
fi

echo "→ building the image (version $ME_VERSION)"
"${DC[@]}" build engine
echo "→ starting the database"
"${DC[@]}" up -d --wait db

# One-time move of accounts from the stand-alone container of "make db".
if docker container inspect velexa_me_pg >/dev/null 2>&1 &&
   [ "$("${DC[@]}" exec -T db psql -U velexa_me -d velexa_me -tAc "select to_regclass('public.users') is not null")" = f ]; then
  echo "→ copying accounts and the audit trail from the velexa_me_pg container"
  docker start velexa_me_pg >/dev/null
  for _ in $(seq 1 60); do docker exec velexa_me_pg pg_isready -U velexa_me -q && break; sleep 0.5; done
  docker exec velexa_me_pg pg_dump -U velexa_me -d velexa_me --no-owner --no-privileges |
    "${DC[@]}" exec -T db psql -q -v ON_ERROR_STOP=1 -U velexa_me -d velexa_me >/dev/null
  echo "   $("${DC[@]}" exec -T db psql -U velexa_me -d velexa_me -tAc 'select count(*) from users') accounts copied"
  docker update --restart=no velexa_me_pg >/dev/null && docker stop velexa_me_pg >/dev/null
  echo "   the old container velexa_me_pg is stopped (kept as a backup; remove it with: docker rm velexa_me_pg)"
  sed -i 's|^ME_DB_URL=|# moved to docker compose: ME_DB_URL=|' me.env 2>/dev/null || true
fi

echo "→ starting the engine"
"${DC[@]}" up -d --wait engine
IP=$(kv ME_PUB_IP 127.0.0.1)
echo "✓ running: console http://$IP:$(kv DOCKER_HTTP_PORT 8010)/  ·  FIX $IP:$(kv DOCKER_FIX_PORT 5001)  ·  ITCH $IP:$(kv DOCKER_ITCH_PORT 5501)"
echo "  admin: $(kv ME_ADMIN_USER admin) / ME_ADMIN_PASS in $ENV_FILE   ·   logs: docker compose logs -f engine"
