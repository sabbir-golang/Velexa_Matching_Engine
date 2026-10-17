package admin

import (
	"net"
	"net/http"
	"os"
	"strings"
	"time"

	"velexa-sim-me/internal/database/audit"
	"velexa-sim-me/internal/database/brokers"
	"velexa-sim-me/internal/engine"
	"velexa-sim-me/internal/fixgw"
	"velexa-sim-me/internal/rest/middleware"
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

// Status serves /admin/api/status. An admin gets the whole exchange; a user
// gets the same page scoped to their own brokers: their sessions, logon events,
// order and trade counts and their own audit entries.
func (h *Handler) Status(w http.ResponseWriter, r *http.Request) {
	eng := h.Eng
	all, own := middleware.BrokerScope(r)
	sessions := []map[string]any{}
	events := []fixgw.SessEvent{}
	if fixgw.Active != nil {
		for b, t := range fixgw.Active.ActiveSessions() {
			if !all && !own[strings.ToUpper(b)] {
				continue
			}
			s, tg := fixgw.Active.SeqNums(b)
			sessions = append(sessions, map[string]any{"broker": b, "since": t.Format("2006-01-02 15:04:05"),
				"seq_sender": s, "seq_target": tg})
		}
		for _, ev := range fixgw.Active.RecentEvents(200) {
			if (all || own[strings.ToUpper(ev.Broker)]) && len(events) < 50 {
				events = append(events, ev)
			}
		}
	}
	eng.Mu.RLock()
	phase, instr, orders, trades, running := eng.Phase, len(eng.Instr()), eng.Seq, eng.TradeSeq, eng.Running
	eng.Mu.RUnlock()
	out := map[string]any{
		"phase": phase, "instruments": instr, "orders": orders, "trades": trades, "running": running,
		"auto_clock": eng.AutoClock(), "schedule": eng.ScheduleText(),
		"fix_auth": brokers.Fix.Enforced(), "fix_auth_mode": brokers.Fix.Mode, "itch_auth": brokers.Itch.Enabled,
		"uptime":    time.Since(meStart).Round(time.Second).String(),
		"listeners": listeners(),
		"sessions":  sessions, "events": events, "scope": "all",
		"universe": engine.InstrumentStatus(eng),
	}
	if all {
		out["audit"] = audit.Recent(60)
	} else {
		var actors []string
		for b := range own {
			actors = append(actors, "FIX:"+b)
		}
		st := eng.StatsFor(actors)
		me := strings.ToLower(middleware.Username(r)) + " ("
		mine := []audit.Entry{}
		for _, e := range audit.Recent(300) {
			if strings.HasPrefix(strings.ToLower(e.Actor), me) && len(mine) < 60 {
				mine = append(mine, e)
			}
		}
		out["scope"], out["my_brokers"], out["orders"], out["trades"] = "own", len(own), st.Orders, st.Trades
		out["volume"], out["value"], out["audit"] = st.Volume, st.Value, mine
	}
	utils.WriteJSON(w, out)
}

// listeners checks the engine's own ports, and the TLS FIX gateway in front of
// it when one is configured (ME_FIXS_PORT).
func listeners() map[string]bool {
	l := map[string]bool{
		"FIX :" + fixPort():     portOpen("127.0.0.1:" + fixPort()),
		"ITCH :" + itchPort():   portOpen("127.0.0.1:" + itchPort()),
		"Gateway :" + webPort(): portOpen("127.0.0.1:" + webPort()),
	}
	if p := envPort("ME_FIXS_PORT", ""); p != "" {
		l["FIXS :"+p] = portOpen("127.0.0.1:" + p)
	}
	return l
}

func fixPort() string  { return envPort("FIX_PORT", "5001") }
func itchPort() string { return envPort("ITCH_PORT", "5501") }
func webPort() string  { return envPort("PORT", "8010") }

func envPort(k, d string) string {
	if v := strings.TrimSpace(os.Getenv(k)); v != "" {
		return v
	}
	return d
}
