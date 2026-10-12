package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync/atomic"
	"time"
)

func main() {
	port := os.Getenv("PORT")
	if port == "" {
		port = "8010"
	}
	bus := NewBus()
	eng := NewEngine(loadUniverse(), bus)
	go eng.run()
	eng.startBots(time.Now().UnixNano())

	// FIX acceptor — brokers' fix_initiator_go connect here (real FIX round-trip)
	fixPort := os.Getenv("FIX_PORT")
	if fixPort == "" {
		fixPort = "5001"
	}
	fixSender := os.Getenv("FIX_SENDER")
	if fixSender == "" {
		fixSender = "DSEME"
	}
	if _, err := StartFixAcceptor(eng, fixPort, fixSender); err != nil {
		log.Printf("[FIX] acceptor start failed: %v (HTTP still serving)", err)
	}
	meItchAuth = loadItchAuth()

	mux := http.NewServeMux()
	registerAdmin(mux, eng)           // /admin console (login-protected)
	registerInstrumentAdmin(mux, eng) // security-master inspect / reload / import
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Write([]byte(indexHTML))
	})
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		eng.mu.RLock()
		phase, running := eng.phase, eng.running
		eng.mu.RUnlock()
		// Public gateway health for the exchange /marketfeed panel. No secrets:
		// if this responds, the gateway is up and serving all feeds. `phase` and
		// `running` reflect the live matching-engine trading state.
		writeJSON(w, map[string]any{
			"ok": true, "phase": phase, "running": running,
			"feeds": map[string]bool{"market": true, "index": true, "news": true, "depth": true},
		})
	})
	itch := NewItchGateway(eng)
	go itch.run()
	go newItchFeed(eng).run() // native DSE-ITCH SoupBinTCP feed for the C++ itch service
	mux.HandleFunc("/market", itch.serve(itch.market, selMarket))
	mux.HandleFunc("/marketdepth", itch.serve(itch.depth, selDepth))
	mux.HandleFunc("/index", itch.serve(itch.index, selIndex))
	pool, poolErr := mustPool()
	if poolErr != nil {
		log.Printf("[bench] pool init: %v", poolErr)
	}
	bench := NewBench(pool, eng)
	mux.HandleFunc("/bench", serveBenchUI)
	mux.HandleFunc("/api/bench/start", func(w http.ResponseWriter, r *http.Request) {
		cfg := BenchConfig{RMS: true, Persist: true}
		if r.Body != nil {
			json.NewDecoder(r.Body).Decode(&cfg)
		}
		if err := bench.Start(cfg); err != nil {
			writeJSON(w, map[string]any{"ok": false, "error": err.Error()})
			return
		}
		writeJSON(w, map[string]any{"ok": true, "cfg": cfg})
	})
	mux.HandleFunc("/api/bench/stop", func(w http.ResponseWriter, r *http.Request) {
		bench.Stop()
		writeJSON(w, map[string]any{"ok": true})
	})
	mux.HandleFunc("/api/bench/stats", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, bench.Stats())
	})
	live := NewLiveBench(eng)
	mux.HandleFunc("/api/bots", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, map[string]any{"paused": atomic.LoadInt32(&botsPaused) == 1})
	})
	mux.HandleFunc("/api/bots/pause", func(w http.ResponseWriter, r *http.Request) {
		atomic.StoreInt32(&botsPaused, 1)
		writeJSON(w, map[string]any{"paused": true})
	})
	mux.HandleFunc("/api/bots/resume", func(w http.ResponseWriter, r *http.Request) {
		atomic.StoreInt32(&botsPaused, 0)
		writeJSON(w, map[string]any{"paused": false})
	})
	// Session control: pre-open call auction → continuous trading.
	mux.HandleFunc("/api/session", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, map[string]any{"phase": eng.phase})
	})
	mux.HandleFunc("/api/session/preopen", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, map[string]any{"phase": eng.SetPhase("Pre-Open")})
	})
	mux.HandleFunc("/api/session/open", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, map[string]any{"phase": eng.SetPhase("Continuous Trading")})
	})
	mux.HandleFunc("/api/live/start", func(w http.ResponseWriter, r *http.Request) {
		var cfg LiveConfig
		if r.Body != nil {
			json.NewDecoder(r.Body).Decode(&cfg)
		}
		if err := live.Start(cfg); err != nil {
			writeJSON(w, map[string]any{"ok": false, "error": err.Error()})
			return
		}
		writeJSON(w, map[string]any{"ok": true, "cfg": cfg})
	})
	mux.HandleFunc("/api/live/stop", func(w http.ResponseWriter, r *http.Request) {
		live.Stop()
		writeJSON(w, map[string]any{"ok": true})
	})
	mux.HandleFunc("/api/live/stats", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, live.Stats())
	})
	mux.HandleFunc("/api/live/latency", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, pipelineLatency())
	})
	mux.HandleFunc("/api/live/cleanup", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, live.Cleanup())
	})
	mux.HandleFunc("/api/bench/summary.csv", func(w http.ResponseWriter, r *http.Request) {
		bench.writeSummaryCSV(w)
	})
	mux.HandleFunc("/api/bench/orders.csv", ordersCSVHandler(pool))
	mux.HandleFunc("/api/loadtest", func(w http.ResponseWriter, r *http.Request) {
		n := atoiDefault(r.URL.Query().Get("n"), 100000)
		if n > 2000000 {
			n = 2000000
		}
		writeJSON(w, eng.LoadTest(n, r.URL.Query().Get("symbol")))
	})
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) { fmt.Fprint(w, "ok") })

	mux.HandleFunc("/api/market", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, map[string]any{"index": eng.index, "phase": eng.phase, "rows": eng.marketRows()})
	})
	mux.HandleFunc("/api/book", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, eng.snapshotBook(r.URL.Query().Get("symbol")))
	})
	mux.HandleFunc("/api/status", func(w http.ResponseWriter, r *http.Request) {
		eng.mu.RLock()
		st := map[string]any{"phase": eng.phase, "index": eng.index, "instruments": len(eng.instr()), "orders": eng.seq, "trades": eng.tradeSeq, "running": eng.running, "universe": instrumentStatus(eng)}
		eng.mu.RUnlock()
		writeJSON(w, st)
	})
	// external order entry (this is where the OMS / fix acceptor will feed orders)
	mux.HandleFunc("/api/order", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "POST only", 405)
			return
		}
		var req struct {
			Symbol  string  `json:"symbol"`
			Side    string  `json:"side"`
			Type    string  `json:"type"`
			Price   float64 `json:"price"`
			Qty     int64   `json:"qty"`
			Actor   string  `json:"actor"`
			ClOrdID string  `json:"cl_ord_id"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "malformed order JSON", 400)
			return
		}
		req.Symbol = strings.ToUpper(strings.TrimSpace(req.Symbol))
		if req.Qty <= 0 {
			http.Error(w, fmt.Sprintf("invalid quantity %d: must be greater than zero", req.Qty), 400)
			return
		}
		if !eng.listed(req.Symbol) && !eng.autoList(req.Symbol, req.Price) {
			http.Error(w, fmt.Sprintf("unknown symbol %s: not in this engine's instrument universe (%s)",
				req.Symbol, universeSource()), 400)
			return
		}
		side := Buy
		if req.Side == "SELL" || req.Side == "sell" {
			side = Sell
		}
		typ := Limit
		if req.Type == "MARKET" || req.Type == "market" || req.Price <= 0 {
			typ = Market
		}
		actor := req.Actor
		if actor == "" {
			actor = "API"
		}
		o := eng.newOrder(req.Symbol, side, typ, req.Price, req.Qty, actor)
		o.ClOrdID = req.ClOrdID
		rep := eng.SubmitAndWait(o)
		writeJSON(w, rep)
	})

	// /events is dual-mode: a WebSocket upgrade (the trader SPA news socket)
	// routes to the itch news/time-&-sales feed; a plain GET is the sim UI's
	// SSE market stream.
	mux.HandleFunc("/events", func(w http.ResponseWriter, r *http.Request) {
		if strings.EqualFold(r.Header.Get("Upgrade"), "websocket") {
			itch.serveEvents(w, r)
			return
		}
		fl, ok := w.(http.Flusher)
		if !ok {
			http.Error(w, "stream unsupported", 500)
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Connection", "keep-alive")
		w.Header().Set("X-Accel-Buffering", "no")
		ch := bus.subscribe()
		defer bus.unsubscribe(ch)
		fmt.Fprintf(w, "retry: 3000\n\n")
		fl.Flush()
		ka := time.NewTicker(15 * time.Second)
		defer ka.Stop()
		for {
			select {
			case <-r.Context().Done():
				return
			case msg := <-ch:
				fmt.Fprintf(w, "data: %s\n\n", msg)
				fl.Flush()
			case <-ka.C:
				fmt.Fprintf(w, ": keepalive\n\n")
				fl.Flush()
			}
		}
	})

	log.Printf("Velexa Simulator Matching Engine on :%s  (%d instruments from %s, auto-list=%v, phase=%s)",
		port, len(eng.instr()), universeSource(), autoListEnabled(), eng.phase)
	srv := &http.Server{Addr: ":" + port, Handler: mux, ReadHeaderTimeout: 5 * time.Second}
	log.Fatal(srv.ListenAndServe())
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(v)
}

func atoiDefault(s string, d int) int {
	if n, err := strconv.Atoi(s); err == nil {
		return n
	}
	return d
}
