package config

// envTemplate is me.env.example (kept identical; a test checks). It is written
// out as me.env, with the secrets filled in, the first time the engine starts
// without an env file.
const envTemplate = `# Matching engine settings. The engine reads ./me.env at start-up (or the file
# named by ME_ENV_FILE); variables already set in the environment win.
# On the first run without a me.env, the engine writes one from this template
# with a random admin password and session secret filled in.
# Lines starting with # are defaults or examples: remove the # to change one.

# ── Console sign-in ──────────────────────────────────────────────────────────
# The first admin account. Sign in on the welcome page with these.
ME_ADMIN_USER=admin
ME_ADMIN_PASS=
# Signs the console session cookies (at least 32 characters).
ME_ADMIN_SECRET=
# ME_REGISTRATION=on            # off closes self sign-up
# ME_RATE_LIMIT=off             # on: lock sign-in after 5 wrong passwords
# ME_COOKIE_SECURE=off          # on when users reach the console over HTTPS
# ME_TRUSTED_PROXIES=127.0.0.1,::1

# ── Listeners ────────────────────────────────────────────────────────────────
# PORT=8010                     # web console, JSON API, WebSockets
# FIX_PORT=5001                 # FIX acceptor
# FIX_SENDER=DSEME              # the engine's CompID (brokers' TargetCompID)
# ITCH_PORT=5501                # ITCH market feed
# ME_ITCH_INDEX_PORT=5502
# ME_ITCH_NEWS_PORT=5503

# ── FIX / ITCH logon checks ──────────────────────────────────────────────────
# ME_FIX_AUTH unset = auto: logons are checked once the first broker is
# registered in the console (Broker Credentials); on = always; off = never.
# ME_FIX_AUTH=on
# ME_ITCH_AUTH=off

# ── What the broker onboarding sheet shows ───────────────────────────────────
# Unset: this PC's LAN IP address (also as Host / SNI) and the real FIX/ITCH ports.
# ME_PUB_HOST=me.example.com
# ME_PUB_IP=203.0.113.10
# ME_INT_IP=10.0.0.5
# ME_ITCH_IP=203.0.113.10
# ME_FIXS_PORT=5443             # only if a TLS FIX gateway (nginx) runs in front

# ── Database (console accounts and the audit trail) ──────────────────────────
# Unset: kept in files in the data folder (users.json, admin_audit.jsonl).
# "make db" starts a local PostgreSQL container and adds this line for you.
# ME_DB_URL=postgres://velexa_me:PASSWORD@127.0.0.1:5435/velexa_me?sslmode=disable
# ME_DB_MAX_CONNS=10

# ── Data ─────────────────────────────────────────────────────────────────────
# ME_STATE_DIR=./data           # where users, brokers, logs are kept
# ME_INSTRUMENT_FILE=instruments.json
# EXCHANGE_INSTRUMENT_URL=http://exchange-host:8088/api/instrument-master
`
