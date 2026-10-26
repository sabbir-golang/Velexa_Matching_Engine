package admin

import (
	"fmt"
	"net/http"
	"runtime"
	"time"

	"velexa-sim-me/internal/config"
	"velexa-sim-me/internal/database/brokers"
	"velexa-sim-me/internal/database/db"
	"velexa-sim-me/internal/engine"
	"velexa-sim-me/internal/utils"
)

// Diagnostics serves /admin/api/diagnostics (admins): the engine's runtime
// figures and configuration; with ?test=1 also a self-test of read-only checks
// against the live engine (nothing is changed).
func (h *Handler) Diagnostics(w http.ResponseWriter, r *http.Request) {
	eng := h.Eng
	var ms runtime.MemStats
	runtime.ReadMemStats(&ms)
	resting, _, _ := eng.SnapshotOrders()
	eng.Mu.RLock()
	seq, tradeSeq := eng.Seq, eng.TradeSeq
	eng.Mu.RUnlock()

	dbName := "files in " + config.StateDir()
	if dsn := config.DBURL(); dsn != "" {
		dbName = "PostgreSQL " + db.Describe(dsn)
	}
	saving := "off (ME_MARKET_STATE=off)"
	if h.State != nil {
		saving = "every second, and at shutdown"
	}
	fixAuth := "off"
	if brokers.Fix != nil && brokers.Fix.Enforced() {
		fixAuth = "on (" + brokers.Fix.Mode + ")"
	} else if brokers.Fix != nil {
		fixAuth = "open (" + brokers.Fix.Mode + ")"
	}
	itchAuth := "off"
	if brokers.Itch != nil && brokers.Itch.Enabled {
		itchAuth = "on"
	}
	out := map[string]any{
		"ok": true,
		"runtime": map[string]any{
			"uptime": time.Since(meStart).Round(time.Second).String(), "version": config.Version, "go": runtime.Version(),
			"goroutines": runtime.NumGoroutine(), "heap_mb": float64(ms.HeapAlloc) / (1 << 20), "sys_mb": float64(ms.Sys) / (1 << 20),
			"gc_runs": ms.NumGC, "cpus": runtime.NumCPU(),
		},
		"engine": map[string]any{
			"phase": eng.CurrentPhase(), "auto_clock": eng.AutoClock(), "instruments": len(eng.Instr()),
			"resting_orders": len(resting), "orders": seq, "trades": tradeSeq,
		},
		"config": [][2]string{
			{"FIX acceptor", config.EnvOr("ME_FIX_BEGIN", "FIXT.1.1") + " on :" + fixPort() + " · SenderCompID " + config.EnvOr("FIX_SENDER", "DSEME")},
			{"FIX logon check", fixAuth},
			{"ITCH feeds", ":" + itchPort() + " market · :" + envPort("ME_ITCH_INDEX_PORT", "5502") + " index · :" + envPort("ME_ITCH_NEWS_PORT", "5503") + " news"},
			{"ITCH login check", itchAuth},
			{"Console", ":" + webPort()},
			{"Session schedule", eng.ScheduleText() + " (Asia/Dhaka, Sunday–Thursday)"},
			{"Market saved", saving},
			{"Data", dbName},
			{"Instruments from", engine.UniverseSource()},
			{"Version", config.Version + " · " + runtime.Version()},
		},
	}
	if r.URL.Query().Get("test") != "" {
		out["checks"] = h.selfTest()
		out["tested_at"] = time.Now().In(engine.Dhaka).Format("15:04:05")
	}
	utils.WriteJSON(w, out)
}

type check struct {
	Name   string `json:"name"`
	OK     bool   `json:"ok"`
	Result string `json:"result"`
}

// selfTest runs read-only checks against the running engine.
func (h *Handler) selfTest() []check {
	var out []check
	add := func(name string, ok bool, format string, a ...any) {
		out = append(out, check{Name: name, OK: ok, Result: fmt.Sprintf(format, a...)})
	}
	rows := h.Eng.MarketRows()
	crossed := 0
	for _, m := range rows {
		if m.Bid > 0 && m.Ask > 0 && m.Bid >= m.Ask {
			crossed++
		}
	}
	add("Order books not crossed (best bid below best ask)", crossed == 0, "%d books, %d crossed", len(rows), crossed)
	n := len(h.Eng.Instr())
	add("Instrument universe loaded", n > 0, "%d listed", n)
	timed := func(addr string) (bool, string) {
		t := time.Now()
		ok := portOpen(addr)
		return ok, fmt.Sprintf("%.1f ms", float64(time.Since(t).Microseconds())/1000)
	}
	ok, d := timed("127.0.0.1:" + fixPort())
	add("FIX acceptor accepts connections", ok, "connect %s", d)
	for _, f := range [][2]string{{"market", itchPort()}, {"index", envPort("ME_ITCH_INDEX_PORT", "5502")}, {"news", envPort("ME_ITCH_NEWS_PORT", "5503")}} {
		ok, d := timed("127.0.0.1:" + f[1])
		add("ITCH "+f[0]+" feed accepts connections", ok, ":%s %s", f[1], d)
	}
	if h.Reports != nil {
		t := time.Now()
		_, err := h.Reports.Days(1)
		add("Report store reads", err == nil, "%.1f ms", float64(time.Since(t).Microseconds())/1000)
	}
	if h.State != nil {
		if at := h.State.LastSaved(); at.IsZero() {
			add("Market state saved", false, "not yet")
		} else {
			age := time.Since(at)
			add("Market state saved", age < 10*time.Second, "%s ago", age.Round(100*time.Millisecond))
		}
		add("Trade log up to date", h.State.Pending() < 1000, "%d waiting", h.State.Pending())
	}
	return out
}
