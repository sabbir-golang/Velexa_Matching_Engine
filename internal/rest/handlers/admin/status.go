package admin

import (
	"net"
	"net/http"
	"os"
	"strings"
	"time"

	"velexa-sim-me/internal/config"
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
		"uptime":   time.Since(meStart).Round(time.Second).String(),
		"fix_port": fixPort(), "fix_sender": config.EnvOr("FIX_SENDER", "DSEME"), "fix_begin": config.EnvOr("ME_FIX_BEGIN", "FIXT.1.1"),
		"listeners": listeners(),
		"sessions":  sessions, "events": events, "scope": "all",
		"universe": engine.InstrumentStatus(eng),
	}
	// The live market is saved every second; the console shows how long ago.
	out["state_saved"], out["state_age"] = "off", -1.0
	if h.State != nil {
		if t := h.State.LastSaved(); !t.IsZero() {
			out["state_saved"] = t.In(engine.Dhaka).Format("2006-01-02 15:04:05")
			out["state_age"] = time.Since(t).Seconds()
		} else {
			out["state_saved"] = "not yet"
		}
		out["state_pending_trades"] = h.State.Pending()
	}
	if all {
		if fixgw.Active != nil {
			out["fix_queued"] = fixgw.Active.Queued()
		}
		out["audit"] = audit.Recent(60)
	} else {
		var actors []string
		for b := range own {
			actors = append(actors, "FIX:"+b)
		}
		st := eng.StatsFor(actors)
		mine := audit.RecentFor(middleware.Username(r)+" (", 60) // their own entries only
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
