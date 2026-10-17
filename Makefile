# Common tasks for the Velexa matching engine.
# Run from the repository root: the engine looks for spec/ and data/ relative to it.

BIN := bin/velexa-sim-me
PKG := .

.PHONY: build run test race cover bench e2e check vet fmt conformance report clean

build:            ## compile the engine to bin/velexa-sim-me
	go build -o $(BIN) $(PKG)

run:              ## run the engine from source (PORT, FIX_PORT, ... as environment variables)
	go run $(PKG)

test:             ## unit tests plus the FIX conformance suite (about 40 s)
	go test ./...

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
