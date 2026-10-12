package me

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// ── credential audit log ───────────────────────────────────────────────────
// Every credential issue/revoke and session control action is recorded (in a
// ring + appended to admin_audit.jsonl) and shown in the Diagnostics view.
type AuditEntry struct {
	Time   time.Time `json:"time"`
	Actor  string    `json:"actor"`
	Action string    `json:"action"`
	Target string    `json:"target"`
}

var (
	auditMu   sync.Mutex
	auditRing []AuditEntry
	auditPath = envOr("ME_AUDIT_FILE", stateFile("admin_audit.jsonl"))
)

func recordAudit(actor, action, target string) {
	e := AuditEntry{Time: time.Now(), Actor: actor, Action: action, Target: target}
	auditMu.Lock()
	auditRing = append(auditRing, e)
	if len(auditRing) > 300 {
		auditRing = auditRing[len(auditRing)-300:]
	}
	auditMu.Unlock()
	if f, err := os.OpenFile(auditPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600); err == nil {
		b, _ := json.Marshal(e)
		f.Write(append(b, '\n'))
		f.Close()
	}
}

func recentAudit(n int) []AuditEntry {
	auditMu.Lock()
	defer auditMu.Unlock()
	out := []AuditEntry{}
	for i := len(auditRing) - 1; i >= 0 && len(out) < n; i-- {
		out = append(out, auditRing[i])
	}
	return out
}

func loadAudit() {
	b, err := os.ReadFile(auditPath)
	if err != nil {
		return
	}
	auditMu.Lock()
	defer auditMu.Unlock()
	for _, line := range strings.Split(strings.TrimSpace(string(b)), "\n") {
		if line == "" {
			continue
		}
		var e AuditEntry
		if json.Unmarshal([]byte(line), &e) == nil {
			auditRing = append(auditRing, e)
		}
	}
	if len(auditRing) > 300 {
		auditRing = auditRing[len(auditRing)-300:]
	}
}

// ── login brute-force throttle ─────────────────────────────────────────────
type failRec struct {
	count int
	until time.Time
}

var (
	loginMu    sync.Mutex
	loginFails = map[string]*failRec{}
)

const maxLoginFails = 5

func loginBlocked(ip string) bool {
	loginMu.Lock()
	defer loginMu.Unlock()
	r := loginFails[ip]
	return r != nil && r.count >= maxLoginFails && time.Now().Before(r.until)
}
func noteLoginFail(ip string) {
	loginMu.Lock()
	defer loginMu.Unlock()
	r := loginFails[ip]
	if r == nil {
		r = &failRec{}
		loginFails[ip] = r
	}
	r.count++
	r.until = time.Now().Add(5 * time.Minute)
}
func clearLoginFail(ip string) {
	loginMu.Lock()
	delete(loginFails, ip)
	loginMu.Unlock()
}
func clientIP(r *http.Request) string {
	if h := r.Header.Get("CF-Connecting-IP"); h != "" {
		return h
	}
	if h := r.Header.Get("X-Forwarded-For"); h != "" {
		return strings.TrimSpace(strings.Split(h, ",")[0])
	}
	host, _, _ := net.SplitHostPort(r.RemoteAddr)
	return host
}

// admin.go — the Matching Engine admin console: a login-protected panel on the
// gateway (:8010, /admin) for status, the simulator controls, broker FIX+ITCH
// credential management, and diagnostics. Auth is a signed cookie; credentials
// come from ME_ADMIN_USER / ME_ADMIN_PASS.

var meStart = time.Now()

func adminUser() string   { return envOr("ME_ADMIN_USER", "admin") }
func adminPass() string   { return envOr("ME_ADMIN_PASS", "Velexa@ME2026") }
func adminSecret() string { return envOr("ME_ADMIN_SECRET", "velexa-me/"+adminPass()) }

func adminToken() string {
	mac := hmac.New(sha256.New, []byte(adminSecret()))
	mac.Write([]byte(adminUser()))
	return hex.EncodeToString(mac.Sum(nil))
}

func adminAuthed(r *http.Request) bool {
	ck, err := r.Cookie("me_admin")
	if err != nil {
		return false
	}
	return hmac.Equal([]byte(ck.Value), []byte(adminToken()))
}

func requireAdmin(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !adminAuthed(r) {
			if strings.HasPrefix(r.URL.Path, "/admin/api/") {
				http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
			} else {
				http.Redirect(w, r, "/admin/login", http.StatusSeeOther)
			}
			return
		}
		next(w, r)
	}
}

func genPass(prefix string) string {
	b := make([]byte, 6)
	rand.Read(b)
	return prefix + "-" + hex.EncodeToString(b)
}

// meEndpoints returns the reachable connection details a broker needs to fill
// in on their credential (bm_itch) page. IPs are the ORIGIN (not the
// Cloudflare-proxied host, which doesn't pass FIX/ITCH ports). Overridable via
// env so the admin can set the real public IP.
func meEndpoints() map[string]string {
	return map[string]string{
		"pub_host":  envOr("ME_PUB_HOST", "me.velexa.net"),
		"pub_ip":    envOr("ME_PUB_IP", "103.169.105.76"),
		"fixs_port": envOr("ME_FIXS_PORT", "5443"),
		"int_ip":    envOr("ME_INT_IP", "10.1.1.9"),
		"int_port":  envOr("ME_INT_PORT", "5001"),
		"itch_ip":   envOr("ME_ITCH_IP", envOr("ME_PUB_IP", "103.169.105.76")),
		"itch_port": envOr("ME_ITCH_PORT", "5501"),
		"target":    envOr("FIX_SENDER", "DSEME"),
	}
}

func portOpen(addr string) bool {
	c, err := net.DialTimeout("tcp", addr, 700*time.Millisecond)
	if err != nil {
		return false
	}
	c.Close()
	return true
}

// registerAdmin wires the /admin routes onto the gateway mux.
// ── External OMS IP allowlist (fix_allow.json, read live by the FIXS forwarder) ──
var allowPath = envOr("FIXS_ALLOW_FILE", stateFile("fix_allow.json"))

func readAllow() map[string]string {
	m := map[string]string{}
	if b, err := os.ReadFile(allowPath); err == nil {
		json.Unmarshal(b, &m)
	}
	return m
}

func writeAllow(m map[string]string) error {
	b, _ := json.MarshalIndent(m, "", " ")
	return os.WriteFile(allowPath, b, 0o644)
}

func registerAdmin(mux *http.ServeMux, eng *Engine) {
	loadAudit()
	loadBrokerMeta()
	mux.HandleFunc("/admin/login", func(w http.ResponseWriter, r *http.Request) {
		ip := clientIP(r)
		if r.Method == http.MethodPost {
			if loginBlocked(ip) {
				http.Redirect(w, r, "/admin/login?e=lock", http.StatusSeeOther)
				return
			}
			r.ParseForm()
			if r.FormValue("user") == adminUser() && r.FormValue("pass") == adminPass() {
				clearLoginFail(ip)
				http.SetCookie(w, &http.Cookie{Name: "me_admin", Value: adminToken(), Path: "/admin",
					HttpOnly: true, MaxAge: 8 * 3600, SameSite: http.SameSiteLaxMode,
					Secure: r.Header.Get("X-Forwarded-Proto") == "https"})
				http.Redirect(w, r, "/admin", http.StatusSeeOther)
				return
			}
			noteLoginFail(ip)
			http.Redirect(w, r, "/admin/login?e=1", http.StatusSeeOther)
			return
		}
		if adminAuthed(r) {
			http.Redirect(w, r, "/admin", http.StatusSeeOther)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		page := adminLoginHTML
		msg := ""
		switch r.URL.Query().Get("e") {
		case "1":
			msg = "Invalid username or password."
		case "lock":
			msg = "Too many attempts. Try again in a few minutes."
		}
		w.Write([]byte(strings.Replace(page, "__ERR__", msg, 1)))
	})

	mux.HandleFunc("/admin/logout", func(w http.ResponseWriter, r *http.Request) {
		http.SetCookie(w, &http.Cookie{Name: "me_admin", Value: "", Path: "/admin", MaxAge: -1})
		http.Redirect(w, r, "/admin/login", http.StatusSeeOther)
	})

	mux.HandleFunc("/admin", requireAdmin(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/admin" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Write([]byte(adminDashboardHTML))
	}))

	mux.HandleFunc("/admin/api/status", requireAdmin(func(w http.ResponseWriter, r *http.Request) {
		sessions := []map[string]any{}
		events := []SessEvent{}
		if meFixAcceptor != nil {
			for b, t := range meFixAcceptor.ActiveSessions() {
				s, tg := meFixAcceptor.SeqNums(b)
				sessions = append(sessions, map[string]any{"broker": b, "since": t.Format("2006-01-02 15:04:05"),
					"seq_sender": s, "seq_target": tg})
			}
			events = meFixAcceptor.RecentEvents(50)
		}
		eng.mu.RLock()
		phase, instr, orders, trades, running := eng.phase, len(eng.instr()), eng.seq, eng.tradeSeq, eng.running
		eng.mu.RUnlock()
		writeJSON(w, map[string]any{
			"phase": phase, "instruments": instr, "orders": orders, "trades": trades, "running": running,
			"bots_paused": atomic.LoadInt32(&botsPaused) == 1,
			"fix_auth":    meFixAuth.enabled, "itch_auth": meItchAuth.enabled,
			"uptime": time.Since(meStart).Round(time.Second).String(),
			"listeners": map[string]bool{
				"FIX :5001":     portOpen("127.0.0.1:5001"),
				"FIXS :5443":    portOpen("127.0.0.1:5443"),
				"ITCH :5501":    portOpen("127.0.0.1:5501"),
				"Gateway :8010": portOpen("127.0.0.1:8010"),
			},
			"sessions": sessions, "events": events, "audit": recentAudit(60),
			"universe": instrumentStatus(eng),
		})
	}))

	mux.HandleFunc("/admin/api/brokers", requireAdmin(func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, map[string]any{
			"brokers":    meBrokers.list(),
			"fix_total":  len(meFixAuth.list()),
			"itch_total": len(meItchAuth.list()),
		})
	}))

	// broker/detail — full dynamic credential + linkage for one broker.
	mux.HandleFunc("/admin/api/broker/detail", requireAdmin(func(w http.ResponseWriter, r *http.Request) {
		sender := strings.ToUpper(strings.TrimSpace(r.URL.Query().Get("sender")))
		rec := meBrokers.get(sender)
		fixPass := meFixAuth.password(sender)
		itchUser, company, created, contact := "", "", "", ""
		if rec != nil {
			itchUser, company, created, contact = rec.ItchUser, rec.Company, rec.Created, rec.Contact
		}
		itchPass := meItchAuth.password(itchUser)
		writeJSON(w, map[string]any{
			"sender": sender, "company": company, "contact": contact, "created": created,
			"fix_password": fixPass, "itch_user": itchUser, "itch_password": itchPass,
			"endpoints":  meEndpoints(),
			"fix_active": fixPass != "", "itch_active": itchPass != "",
		})
	}))

	mux.HandleFunc("/admin/api/broker/add", requireAdmin(func(w http.ResponseWriter, r *http.Request) {
		var req struct{ Sender, Company, Contact, FixPass, ItchUser, ItchPass string }
		json.NewDecoder(r.Body).Decode(&req)
		sender := strings.ToUpper(strings.TrimSpace(req.Sender))
		if sender == "" {
			writeJSON(w, map[string]any{"ok": false, "error": "SenderCompID is required"})
			return
		}
		fixPass := strings.TrimSpace(req.FixPass)
		if fixPass == "" {
			fixPass = genPass("FIX")
		}
		itchUser := strings.TrimSpace(req.ItchUser)
		if itchUser == "" {
			itchUser = sender
			if len(itchUser) > 6 {
				itchUser = itchUser[:6]
			}
		}
		itchPass := strings.TrimSpace(req.ItchPass)
		if itchPass == "" {
			itchPass = genPass("ITCH")
		}
		company := strings.TrimSpace(req.Company)
		meFixAuth.add(sender, fixPass)
		meItchAuth.add(itchUser, itchPass)
		meBrokers.add(&BrokerRec{Company: company, Sender: sender, ItchUser: itchUser,
			Contact: strings.TrimSpace(req.Contact), Created: time.Now().Format("2006-01-02 15:04")})
		label := sender
		if company != "" {
			label = company + " (" + sender + ")"
		}
		recordAudit(clientIP(r), "ISSUE", label)
		writeJSON(w, map[string]any{"ok": true, "sender": sender, "company": company,
			"fix_password": fixPass, "itch_user": itchUser, "itch_password": itchPass})
	}))

	// broker/del — revoke a whole broker (FIX + ITCH + metadata) by SenderCompID.
	mux.HandleFunc("/admin/api/broker/del", requireAdmin(func(w http.ResponseWriter, r *http.Request) {
		var req struct{ Sender string }
		json.NewDecoder(r.Body).Decode(&req)
		sender := strings.ToUpper(strings.TrimSpace(req.Sender))
		if rec := meBrokers.get(sender); rec != nil && rec.ItchUser != "" {
			meItchAuth.del(rec.ItchUser)
		}
		meFixAuth.del(sender)
		meBrokers.del(sender)
		recordAudit(clientIP(r), "REVOKE", sender)
		writeJSON(w, map[string]any{"ok": true})
	}))

	mux.HandleFunc("/admin/api/allowlist", requireAdmin(func(w http.ResponseWriter, r *http.Request) {
		m := readAllow()
		list := make([]map[string]string, 0, len(m))
		for ip, label := range m {
			list = append(list, map[string]string{"ip": ip, "label": label})
		}
		writeJSON(w, map[string]any{"list": list, "count": len(list)})
	}))
	mux.HandleFunc("/admin/api/allowlist/add", requireAdmin(func(w http.ResponseWriter, r *http.Request) {
		var req struct{ IP, Label string }
		json.NewDecoder(r.Body).Decode(&req)
		ip := strings.TrimSpace(req.IP)
		if ip == "" || net.ParseIP(ip) == nil {
			writeJSON(w, map[string]any{"ok": false, "error": "a valid IP address is required"})
			return
		}
		m := readAllow()
		m[ip] = strings.TrimSpace(req.Label)
		if err := writeAllow(m); err != nil {
			writeJSON(w, map[string]any{"ok": false, "error": err.Error()})
			return
		}
		recordAudit(clientIP(r), "ALLOW+", ip)
		writeJSON(w, map[string]any{"ok": true})
	}))
	mux.HandleFunc("/admin/api/allowlist/del", requireAdmin(func(w http.ResponseWriter, r *http.Request) {
		var req struct{ IP string }
		json.NewDecoder(r.Body).Decode(&req)
		ip := strings.TrimSpace(req.IP)
		m := readAllow()
		delete(m, ip)
		writeAllow(m)
		recordAudit(clientIP(r), "ALLOW-", ip)
		writeJSON(w, map[string]any{"ok": true})
	}))
	mux.HandleFunc("/admin/api/sim", requireAdmin(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Query().Get("action") {
		case "bots_pause":
			atomic.StoreInt32(&botsPaused, 1)
		case "bots_resume":
			atomic.StoreInt32(&botsPaused, 0)
		case "preopen":
			eng.SetPhase("Pre-Open")
		case "open":
			eng.SetPhase("Continuous Trading")
		}
		writeJSON(w, map[string]any{"ok": true, "phase": eng.phase,
			"bots_paused": atomic.LoadInt32(&botsPaused) == 1})
	}))

	// Per-broker session controls: force-disconnect or reset sequence numbers.
	mux.HandleFunc("/admin/api/session", requireAdmin(func(w http.ResponseWriter, r *http.Request) {
		broker := strings.TrimSpace(r.URL.Query().Get("broker"))
		if meFixAcceptor == nil || broker == "" {
			writeJSON(w, map[string]any{"ok": false, "error": "broker required"})
			return
		}
		var err error
		switch r.URL.Query().Get("action") {
		case "disconnect":
			err = meFixAcceptor.Disconnect(broker)
		case "reset":
			err = meFixAcceptor.ResetSeq(broker)
		default:
			err = fmt.Errorf("unknown action")
		}
		if err != nil {
			writeJSON(w, map[string]any{"ok": false, "error": err.Error()})
			return
		}
		recordAudit(clientIP(r), "SESSION "+strings.ToUpper(r.URL.Query().Get("action")), broker)
		writeJSON(w, map[string]any{"ok": true})
	}))

	// FIX message log — every inbound/outbound message, filterable by broker + ClOrdID.
	mux.HandleFunc("/admin/api/fixlog", requireAdmin(func(w http.ResponseWriter, r *http.Request) {
		limit := 250
		if n, err := strconv.Atoi(r.URL.Query().Get("limit")); err == nil && n > 0 && n <= 1000 {
			limit = n
		}
		writeJSON(w, map[string]any{"messages": meMsgLog.recent(
			strings.TrimSpace(r.URL.Query().Get("broker")),
			strings.TrimSpace(r.URL.Query().Get("clordid")), limit)})
	}))

	// Onboarding sheet — renders a printable FIX/FIXS connectivity card filled
	// with a broker's values (posted from the credential-issue step).
	mux.HandleFunc("/admin/api/broker/sheet", requireAdmin(func(w http.ResponseWriter, r *http.Request) {
		r.ParseForm()
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Write([]byte(brokerSheetHTML(r.FormValue("company"), r.FormValue("sender"),
			r.FormValue("fix_pass"), r.FormValue("itch_user"), r.FormValue("itch_pass"))))
	}))
}
