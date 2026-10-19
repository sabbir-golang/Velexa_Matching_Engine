# Common tasks for the Velexa matching engine.
# Run from the repository root: the engine looks for spec/ and data/ relative to it.

BIN := bin/velexa-sim-me
PKG := .

.PHONY: build run test test-db race cover bench e2e check vet fmt conformance report clean db db-psql db-backup \
	docker-setup docker-up docker-down docker-logs docker-ps docker-backup docker-restore docker-test dev dev-local

build:            ## compile the engine to bin/velexa-sim-me
	go build -o $(BIN) $(PKG)

run:              ## run the engine from source (PORT, FIX_PORT, ... as environment variables)
	go run $(PKG)

test:             ## unit tests plus the FIX conformance suite (about 40 s)
	go test ./...

test-db:          ## all tests, with the PostgreSQL ones, against a throwaway container
	tools/test-db.sh

db:               ## create/start the engine's PostgreSQL container and add ME_DB_URL to me.env
	tools/db-setup.sh

db-psql:          ## SQL shell on the engine's database
	docker exec -it velexa_me_pg psql -U velexa_me -d velexa_me

db-backup:        ## dump the engine's database to backups/
	mkdir -p backups && docker exec velexa_me_pg pg_dump -U velexa_me -d velexa_me | gzip > backups/velexa_me-$$(date +%Y%m%d-%H%M).sql.gz && ls -la backups/velexa_me-*.sql.gz | tail -1

docker-setup:     ## write .env for docker compose (host IP, random secrets, data-folder owner)
	tools/docker-setup.sh

dev:              ## live reload in Docker: Air rebuilds and restarts the engine on every saved .go file
	tools/docker-dev.sh

dev-local:        ## live reload on this machine with Air (go install github.com/air-verse/air@v1.67.4)
	air -c .air.toml

docker-up:        ## build and start engine + PostgreSQL in Docker (runs docker-setup the first time)
	tools/docker-up.sh

docker-down:      ## stop the containers (data and database are kept)
	docker compose down

docker-logs:      ## follow the engine's log
	docker compose logs -f --tail=200 engine

docker-ps:        ## container status and health
	docker compose ps

docker-backup:    ## database + data folder + .env -> backups/velexa-me-<time>.tar.gz
	tools/docker-backup.sh

docker-restore:   ## restore a backup: make docker-restore FILE=backups/velexa-me-....tar.gz
	tools/docker-restore.sh $(FILE)

docker-test:      ## build the image and run the system test against containers on spare ports
	tools/docker-e2e.sh

race:             ## every test under the race detector
	go test -race -count=1 ./...

cover:            ## coverage across packages (handlers are exercised from internal/rest)
	go test -count=1 -coverpkg=./... -coverprofile=bin/cover.out ./... >/dev/null
	go tool cover -func=bin/cover.out | tail -1

bench:            ## matching-engine throughput benchmark
	go test ./internal/engine -run '^$$' -bench . -benchtime 100000x

e2e:              ## system test: fresh isolated engine, console + FIX + ITCH + restart (spare ports)
	tools/e2e.sh

check: vet race e2e   ## everything in docs/testing.md that runs unattended

vet:
	go vet ./...

fmt:
	gofmt -l -w Cmd internal

conformance:      ## re-run only the DSE FIX conformance suite (writes docs/conformance_results.json)
	go test ./internal/fixgw -run TestConformance -count=1 -v

report: conformance  ## rebuild docs/DSE_Conformance_Report.pdf from the latest results
	python3 tools/conformance_report.py

clean:
	rm -rf bin
