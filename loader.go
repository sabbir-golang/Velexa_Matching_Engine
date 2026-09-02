package main

import (
	"encoding/json"
	"log"
	"net/http"
	"os"
	"strings"
	"time"
)

// instrMeta carries per-symbol board metadata for the engine's board-aware
// pre-trade rules (tick size, price band). Populated once at boot by
// loadUniverse from the exchange master; read-only thereafter.
type instrMetaT struct {
	Board string
	ISIN  string
	Tick  float64
}

var instrMeta = map[string]instrMetaT{}

// loadUniverse pulls the exchange's published security master — Model 1: the
// exchange is the single source of truth, so the ME trades EXACTLY the symbols
// the exchange publishes and every broker syncs. Indices (is_index) are computed
// by the engine, not traded, so they're skipped here. Falls back to the built-in
// hardcoded universe if the endpoint is unreachable, so the ME always boots.
func loadUniverse() []Instrument {
	url := strings.TrimSpace(os.Getenv("EXCHANGE_INSTRUMENT_URL"))
	if url == "" {
		url = "http://10.1.1.45:8088/api/instrument-master"
	}
	client := &http.Client{Timeout: 12 * time.Second}
	resp, err := client.Get(url)
	if err != nil {
		log.Printf("[universe] fetch %s failed: %v — using built-in universe (%d)", url, err, len(universe))
		return universe
	}
	defer resp.Body.Close()
	var payload struct {
		Count       int `json:"count"`
		Instruments []struct {
			Symbol    string  `json:"symbol"`
			ISIN      string  `json:"isin"`
			Name      string  `json:"name"`
			Board     string  `json:"board"`
			PrevClose float64 `json:"prev_close"`
			Tick      float64 `json:"tick"`
			IsIndex   bool    `json:"is_index"`
			Status    string  `json:"status"`
		} `json:"instruments"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		log.Printf("[universe] decode failed: %v — using built-in universe", err)
		return universe
	}
	out := make([]Instrument, 0, len(payload.Instruments))
	byBoard := map[string]int{}
	for _, i := range payload.Instruments {
		if i.IsIndex || strings.EqualFold(i.Status, "halted") || strings.EqualFold(i.Status, "delisted") {
			continue
		}
		prev := i.PrevClose
		if prev <= 0 {
			prev = 100
		}
		name := i.Name
		if name == "" {
			name = i.Symbol
		}
		tick := i.Tick
		if tick <= 0 {
			tick = 0.10
		}
		instrMeta[i.Symbol] = instrMetaT{Board: i.Board, ISIN: i.ISIN, Tick: tick}
		out = append(out, Instrument{Symbol: i.Symbol, Name: name, Prev: prev})
		byBoard[i.Board]++
	}
	// Safety: until the exchange master is actually populated (real DSE has ~350),
	// a near-empty response means the admin has not imported the EOD file yet — keep
	// the built-in universe so the ME never boots with a crippled book set.
	if len(out) < 50 {
		log.Printf("[universe] exchange master has only %d tradeable (<50) — using built-in universe until EOD import", len(out))
		return universe
	}
	log.Printf("[universe] loaded %d instruments from exchange master (MAIN=%d SME=%d ATB=%d) via %s",
		len(out), byBoard["MAIN"], byBoard["SME"], byBoard["ATB"], url)
	return out
}
