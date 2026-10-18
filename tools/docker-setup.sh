#!/usr/bin/env bash
# Writes .env for docker compose (once): this host's IP for broker sheets,
# random admin password / session secret / database password, and the user
# that owns the data folder. Reuses the admin login and settings of an
# existing me.env, so moving from "make run" to Docker keeps everything.
# Running it again only fills in what is missing.
#   make docker-setup
set -euo pipefail
cd "$(dirname "$0")/.."
ENV=${DOCKER_ENV_FILE:-.env}

rand() { head -c 48 /dev/urandom | base64 | tr -dc 'A-Za-z0-9' | head -c "$1"; }
have() { grep -Eq "^$1=[^[:space:]#]" "$ENV" 2>/dev/null; }
# set_kv KEY VALUE [quote] — fill KEY if it is missing or empty. With quote,
# the value is written in single quotes, which compose takes literally (a
# password may contain $ or other special characters).
set_kv() {
  local k=$1 v=$2 line tmp
  have "$k" && return 0
  if [ "${3:-}" = quote ]; then
    case "$v" in *"'"*) echo "!! $k contains a single quote; set it in $ENV by hand"; return 1 ;; esac
    v="'$v'"
  fi
  if grep -q "^$k=" "$ENV"; then
    tmp=$(mktemp)
    while IFS= read -r line || [ -n "$line" ]; do
      if [[ $line == "$k="* ]]; then printf '%s=%s\n' "$k" "$v"; else printf '%s\n' "$line"; fi
    done <"$ENV" >"$tmp"
    cat "$tmp" >"$ENV"; rm -f "$tmp"
  else
    printf '%s=%s\n' "$k" "$v" >>"$ENV"
  fi
}
from_me_env() { [ -f me.env ] && grep "^$1=" me.env | tail -1 | cut -d= -f2- | sed -e 's/^"//' -e 's/"$//' -e "s/^'//" -e "s/'$//" || true; }

if [ ! -f "$ENV" ]; then
  cp .env.docker.example "$ENV"
  echo "→ created $ENV from .env.docker.example"
fi
chmod 600 "$ENV"

HOST_IP=$(ip route get 1.1.1.1 2>/dev/null | awk '{for (i = 1; i < NF; i++) if ($i == "src") { print $(i + 1); exit }}')
[ -n "$HOST_IP" ] || HOST_IP=$(hostname -I 2>/dev/null | awk '{print $1}')
[ -n "$HOST_IP" ] || HOST_IP=127.0.0.1
set_kv ME_PUB_IP "$HOST_IP"
set_kv ME_INT_IP "$HOST_IP"

# Admin login and secrets: keep the ones of an existing me.env.
set_kv ME_ADMIN_USER "$(from_me_env ME_ADMIN_USER | grep . || echo admin)"
P=$(from_me_env ME_ADMIN_PASS); set_kv ME_ADMIN_PASS "${P:-Me-$(rand 22)-7q}" quote
S=$(from_me_env ME_ADMIN_SECRET); [ ${#S} -ge 32 ] || S=$(rand 64); set_kv ME_ADMIN_SECRET "$S" quote
set_kv DB_PASSWORD "$(rand 32)" quote
for k in ME_FIX_AUTH ME_ITCH_AUTH FIX_SENDER ME_REGISTRATION ME_RATE_LIMIT ME_COOKIE_SECURE \
         ME_TRUSTED_PROXIES ME_SESSION_SCHEDULE ME_SESSION_AUTO EXCHANGE_INSTRUMENT_URL ME_FIXS_PORT ME_PUB_HOST; do
  v=$(from_me_env "$k"); [ -n "$v" ] && ! grep -q "^$k=" "$ENV" && printf "%s='%s'\n" "$k" "$v" >>"$ENV"
done

# The engine runs as the user that owns the data folder (never root).
mkdir -p data
uid=$(id -u); gid=$(id -g)
if [ "$uid" = 0 ]; then uid=10001; gid=10001; chown -R 10001:10001 data; fi
set_kv DOCKER_UID "$uid"
set_kv DOCKER_GID "$gid"
[ -f instruments.json ] && [ ! -f data/instruments.json ] && cp instruments.json data/ && echo "→ copied instruments.json into data/"

echo "✓ $ENV is ready: broker sheets will show $(grep '^ME_PUB_IP=' $ENV | cut -d= -f2 | cut -d' ' -f1); admin user $(grep '^ME_ADMIN_USER=' $ENV | cut -d= -f2 | cut -d' ' -f1)"
echo "  the admin password is ME_ADMIN_PASS in $ENV"
