package admin

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"velexa-sim-me/internal/database/audit"
	"velexa-sim-me/internal/rest/middleware"
	"velexa-sim-me/internal/simulator"
	"velexa-sim-me/internal/utils"
)

// Sim serves GET /admin/api/sim (admins): the market bots, the benchmark, the
// live order tape and the commonest reject reasons.
func (h *Handler) Sim(w http.ResponseWriter, r *http.Request) {
	if h.Simulator == nil {
		utils.WriteJSON(w, map[string]any{"ok": false, "error": "the simulator is off (ME_SIMULATOR=off, or the FIX gateway did not start)"})
		return
	}
	utils.WriteJSON(w, h.Simulator.Status())
}

// SimBots serves POST /admin/api/sim/bots: {action: start|pause|resume|stop}
// plus, for start, the bot settings (rate, bots, symbols, count).
func (h *Handler) SimBots(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Action string `json:"action"`
		simulator.BotConfig
	}
	if !h.simReq(w, r, &req) {
		return
	}
	var err error
	detail := ""
	switch req.Action {
	case "start":
		err = h.Simulator.Start(req.BotConfig)
		detail = fmt.Sprintf("%d orders/s · %d bots · %s", req.Rate, req.Bots, symList(req.Symbols, req.Count))
	case "pause":
		err = h.Simulator.Pause(true)
	case "resume":
		err = h.Simulator.Pause(false)
	case "stop":
		h.Simulator.StopBots()
	default:
		err = fmt.Errorf("unknown action %q", req.Action)
	}
	h.simDone(w, r, "SIM BOTS "+strings.ToUpper(req.Action), detail, err)
}

// SimBench serves POST /admin/api/sim/bench: {action: start|stop} plus, for
// start, the benchmark settings (mode, rate, seconds, symbol).
func (h *Handler) SimBench(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Action string `json:"action"`
		simulator.BenchConfig
	}
	if !h.simReq(w, r, &req) {
		return
	}
	var err error
	detail := ""
	switch req.Action {
	case "start":
		err = h.Simulator.StartBench(req.BenchConfig)
		rate := "max"
		if req.Rate > 0 {
			rate = fmt.Sprintf("%d/s", req.Rate)
		}
		detail = fmt.Sprintf("%s · %s · %d s · %s", req.Mode, rate, req.Seconds, req.Symbol)
	case "stop":
		h.Simulator.StopBench()
	default:
		err = fmt.Errorf("unknown action %q", req.Action)
	}
	h.simDone(w, r, "SIM BENCH "+strings.ToUpper(req.Action), detail, err)
}

// SimClean serves POST /admin/api/sim/clean: stops everything, cancels the
// simulator's orders and removes its rows from the order report.
func (h *Handler) SimClean(w http.ResponseWriter, r *http.Request) {
	if h.Simulator == nil {
		utils.WriteJSON(w, map[string]any{"ok": false, "error": "the simulator is off"})
		return
	}
	res, err := h.Simulator.Clean()
	audit.Record(middleware.ActorOf(r), "SIM CLEAN", fmt.Sprintf("%d orders cancelled · %d report rows removed", res.Cancelled, res.ReportRows))
	if err != nil {
		utils.WriteJSON(w, map[string]any{"ok": false, "error": err.Error(), "result": res})
		return
	}
	utils.WriteJSON(w, map[string]any{"ok": true, "result": res})
}

func (h *Handler) simReq(w http.ResponseWriter, r *http.Request, v any) bool {
	if h.Simulator == nil {
		utils.WriteJSON(w, map[string]any{"ok": false, "error": "the simulator is off"})
		return false
	}
	middleware.LimitBody(w, r)
	if err := json.NewDecoder(r.Body).Decode(v); err != nil {
		utils.WriteJSON(w, map[string]any{"ok": false, "error": "malformed request"})
		return false
	}
	return true
}

func (h *Handler) simDone(w http.ResponseWriter, r *http.Request, action, detail string, err error) {
	if err != nil {
		utils.WriteJSON(w, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	audit.Record(middleware.ActorOf(r), action, detail)
	utils.WriteJSON(w, h.Simulator.Status())
}

func symList(syms []string, n int) string {
	if len(syms) > 0 {
		return strings.Join(syms, ",")
	}
	if n <= 0 {
		n = 20
	}
	return fmt.Sprintf("%d shares", n)
}
