# Running with Docker

This page is for whoever installs and runs the engine on a PC or server. With Docker, the engine and its PostgreSQL database run as two containers. Moving to another machine is a backup on the old one and a restore on the new one.

## Requirements

- Docker Engine 24+ with the Compose plugin (`docker compose version`).
- The repository (git clone or a copy of the folder). Everything else is downloaded or built by Docker.
- Free ports: 8010 (web), 5001 (FIX), 5501–5503 (ITCH). Each can be moved in `.env`.

## First start

```bash
make docker-up
```

The first time, this:

1. Writes `.env` (`tools/docker-setup.sh`) with:
   - this host's IP for broker sheets (`ME_PUB_IP`)
   - a random admin password, session secret and database password
   - your user ID as the owner of the `data/` folder

   If a `me.env` already exists (the engine ran without Docker before), its admin login and settings are reused.
2. Builds the image and starts PostgreSQL, then the engine, and waits until both report healthy.
3. Prints the console address. Sign in at `http://<host IP>:8010/` with **Admin login**, as `ME_ADMIN_USER` with `ME_ADMIN_PASS` from `.env`.

If accounts were kept in the stand-alone `velexa_me_pg` container (`make db`), they are copied into the compose database once, before the engine first starts, and that container is stopped. Broker credentials, order-book IDs and the instrument list in `data/` are used as they are.

## Everyday commands

| Command | Does |
|---|---|
| `make docker-up` | Build and start, or update after a `git pull` (rebuilds and restarts the engine). Also switches back from `make dev` to the production image |
| `make dev` | Live reload: the engine runs under Air on the project folder; a saved `.go` file is rebuilt and the engine restarts in seconds (see below) |
| `make docker-ps` | Status and health of both containers |
| `make docker-logs` | Follow the engine log |
| `make docker-down` | Stop both containers. Data and database are kept |
| `make docker-backup` | Back up database, `data/` and `.env` → `backups/velexa-me-<time>.tar.gz` |
| `make docker-restore FILE=…` | Restore such a backup |
| `docker compose exec db psql -U velexa_me -d velexa_me` | SQL shell on the database |

Both containers restart by themselves after a crash or a reboot (`restart: unless-stopped`).

## Live reload while developing

```bash
make dev        # engine under Air in Docker; follows its log (Ctrl+C stops following, the stack keeps running)
make docker-up  # back to the production image
```

`make dev` replaces the engine container with a development one (`Dockerfile.dev`, `docker-compose.dev.yml`):

- It runs [Air](https://github.com/air-verse/air) on the mounted project folder.
- It keeps the same ports, `.env`, data folder and database, so the console, FIX and ITCH work as usual.
- Saving a `.go` file (or a FIX dictionary in `spec/`) rebuilds the engine. The running engine is stopped cleanly (brokers get a FIX Logout, data is saved), and the new one is serving about 2 seconds later. A broker's OMS has to log on again after each restart.
- A change that doesn't compile is shown in the log, and the running engine keeps going.
- The build output and Go caches live in `tmp/` (git-ignored, owned by you).

The rules are in `.air.toml`. To run Air directly on this machine instead (with `me.env` and `./data`, no Docker), install it with `go install github.com/air-verse/air@v1.67.4` and run `make dev-local`. Stop the Docker engine first, because both use the same ports.

Use the production image (`make docker-up`) for anything that isn't development: it is smaller, read-only, and doesn't need the source.

## Moving to another PC or server

On the old machine:

```bash
make docker-backup                 # backups/velexa-me-20261017-120000.tar.gz
```

Copy that file and the repository to the new machine, then on the new machine:

```bash
make docker-restore FILE=backups/velexa-me-20261017-120000.tar.gz
make docker-up
```

The restore brings back:

- the database: accounts, password hashes, the audit trail
- the `data/` folder: broker FIX/ITCH credentials, the IP allowlist, order-book IDs, instruments
- `.env`

The new host's IP and user are filled in automatically. Admin and broker passwords stay the same, so brokers can reconnect as soon as their OMS points at the new address. The archive contains passwords: keep it private (it is written with mode 0600).

## What is where

| | Location | Kept across restarts and updates |
|---|---|---|
| Accounts, audit trail | PostgreSQL volume `velexa-me_db-data` | yes |
| Broker credentials, allowlist, order-book IDs, instruments, FIX message log | `data/` on the host (`DOCKER_DATA_DIR`) | yes |
| Settings and secrets | `.env` (mode 0600, git-ignored) | yes |
| The engine itself | image `velexa-sim-me:<version>`, rebuilt by `make docker-up` | — |

The FIX message log rotates at 256 MB and keeps 5 old files (`ME_FIX_MSGLOG_MAX_MB`, `ME_FIX_MSGLOG_KEEP`). Docker keeps 5 × 20 MB of container logs per service.

## Settings

`.env` holds two kinds of settings. `.env.docker.example` documents them all.

- **`DOCKER_*`**: the host side.
  - `DOCKER_BIND`: `127.0.0.1` keeps the ports local, for use behind a reverse proxy on the same host.
  - Published ports: `DOCKER_HTTP_PORT`, `DOCKER_FIX_PORT`, `DOCKER_ITCH_PORT`, …
  - `DOCKER_DATA_DIR`: the data folder.
  - `DOCKER_UID` / `DOCKER_GID`: the user that owns the data folder.
- **Engine settings**: any variable from [configuration](configuration.md) can be added (`ME_FIX_AUTH=on`, `ME_COOKIE_SECURE=on`, `ME_SESSION_SCHEDULE=…`, …).
  - Paths and in-container ports are fixed by the image.
  - The broker sheet shows `ME_PUB_IP` with the published FIX and ITCH ports.

After editing `.env`, run `make docker-up` again.

To use an external PostgreSQL instead of the `db` container, set `ME_DB_URL` in `.env`. The `db` container still runs, but the engine doesn't use it.

## Production checklist

- **Host IP.** `ME_PUB_IP` must be the address brokers can reach. Set it by hand behind NAT or a public IP.
- **HTTPS.** Put nginx (or another proxy) in front:
  - Set `DOCKER_BIND=127.0.0.1`, `ME_COOKIE_SECURE=on` and `ME_TRUSTED_PROXIES=127.0.0.1,::1,172.16.0.0/12` (the Docker networks), plus Cloudflare's ranges if it is in front.
  - See [operations](operations.md#nginx-front-ends).
- **Firewall.** Open only the ports brokers need (FIX, ITCH, HTTPS). PostgreSQL is never published; it is reachable only from the engine container.
- **FIX authentication.** Set `ME_FIX_AUTH=on` once brokers are registered. The default checks logons as soon as the first broker exists.
- **Backups.** Schedule `make docker-backup` (cron) and copy the archives off the machine.

## Security of the containers

- The engine runs as a non-root user, with no Linux capabilities and a read-only image; only `data/` and `/tmp` are writable.
- It refuses to start if the data folder isn't writable, instead of losing broker changes.
- `SIGTERM` (`docker compose stop`, a reboot) shuts it down cleanly: open streams are closed, brokers get a FIX Logout, and the database connection is closed.
- `no-new-privileges` is not set, because some kernels refuse to start any process with it.

## Testing the Docker setup

`make docker-test` builds the image and runs the whole deployment as a separate compose project on spare ports, with its own data folder and database volume. It runs:

- the system test (console, two FIX brokers, ITCH, throughput)
- a container restart
- a clean stop
- a backup → wipe everything → restore round trip
- the container hardening checks

Then it removes everything. Your `.env`, `data/` and running containers are not touched.

## Troubleshooting

| Symptom | Fix |
|---|---|
| `port … is in use` from `make docker-up` | An engine started with `make run` (or another program) holds the port. Stop it, or change `DOCKER_*_PORT` in `.env`. |
| Engine exits: `the data folder /data is not writable` | `DOCKER_UID`/`DOCKER_GID` must own `DOCKER_DATA_DIR`: run `make docker-setup`, or `sudo chown -R <uid>:<gid> data`. |
| Broker sheet shows a 172.x address | `ME_PUB_IP` is empty in `.env`. Set the host's IP and run `make docker-up`. |
| Docker installed as a snap can't use a folder | Snap Docker only sees folders under your home directory (not `/tmp` or hidden folders directly in `$HOME`). Keep the repository and `DOCKER_DATA_DIR` under your home. |
| `exec … operation not permitted` from any container | The kernel refuses `no-new-privileges`. Don't add it to `docker-compose.yml`. |
