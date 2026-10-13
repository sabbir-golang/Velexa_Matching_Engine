package admin

import (
	"net"
	"net/http"
	"sync/atomic"
	"time"

	"velexa-sim-me/internal/database/audit"
	"velexa-sim-me/internal/database/brokers"
	"velexa-sim-me/internal/engine"
	"velexa-sim-me/internal/fixgw"
	"velexa-sim-me/internal/utils"
)

var meStart = time.Now()

func portOpen(addr string) bool {
	c, err := net.DialTimeout("tcp", addr, 700*time.Millisecond)
	if err != nil {
		return false
	}
	c.Close()
	return true
}

// Status serves /admin/api/status.
func (h *Handler) Status(w http.ResponseWriter, r *http.Request) {
	eng := h.Eng
	sessions := []map[string]any{}
	events := []fixgw.SessEvent{}
	if fixgw.Active != nil {
		for b, t := range fixgw.Active.ActiveSessions() {
			s, tg := fixgw.Active.SeqNums(b)
			sessions = append(sessions, map[string]any{"broker": b, "since": t.Format("2006-01-02 15:04:05"),
				"seq_sender": s, "seq_target": tg})
		}
		events = fixgw.Active.RecentEvents(50)
	}
	eng.Mu.RLock()
	phase, instr, orders, trades, running := eng.Phase, len(eng.Instr()), eng.Seq, eng.TradeSeq, eng.Running
	eng.Mu.RUnlock()
	utils.WriteJSON(w, map[string]any{
		"phase": phase, "instruments": instr, "orders": orders, "trades": trades, "running": running,
		"bots_paused": atomic.LoadInt32(&engine.BotsPaused) == 1,
		"fix_auth":    brokers.Fix.Enabled, "itch_auth": brokers.Itch.Enabled,
		"uptime": time.Since(meStart).Round(time.Second).String(),
		"listeners": map[string]bool{
			"FIX :5001":     portOpen("127.0.0.1:5001"),
			"FIXS :5443":    portOpen("127.0.0.1:5443"),
			"ITCH :5501":    portOpen("127.0.0.1:5501"),
			"Gateway :8010": portOpen("127.0.0.1:8010"),
		},
		"sessions": sessions, "events": events, "audit": audit.Recent(60),
		"universe": engine.InstrumentStatus(eng),
	})
}
