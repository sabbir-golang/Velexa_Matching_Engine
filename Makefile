# Common tasks for the Velexa matching engine.
# Run from the repository root: the engine looks for spec/ and data/ relative to it.

BIN := bin/velexa-sim-me
PKG := .

.PHONY: build run test vet fmt conformance report clean

build:            ## compile the engine to bin/velexa-sim-me
	go build -o $(BIN) $(PKG)

run:              ## run the engine from source (PORT, FIX_PORT, ... as environment variables)
	go run $(PKG)

test:             ## unit tests plus the FIX conformance suite (about 40 s)
	go test ./...

vet:
	go vet ./...

fmt:
	gofmt -l -w cmd internal

conformance:      ## re-run only the DSE FIX conformance suite (writes docs/conformance_results.json)
	go test ./internal/fixgw -run TestConformance -count=1 -v

report: conformance  ## rebuild docs/DSE_Conformance_Report.pdf from the latest results
	python3 tools/conformance_report.py

clean:
	rm -rf bin
