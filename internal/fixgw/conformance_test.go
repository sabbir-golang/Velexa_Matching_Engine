package fixgw

// fix_conformance_test.go — drives the REAL FIX acceptor (FIXT.1.1 / FIX 5.0 SP2,
// strict data dictionary on) with the exact tag set the Velexa OMS sends to DSE
// (see docs/DSE_FIX_Order_Types.pdf) and records, per case, what the matching
// engine does against what DSE is expected to do.
//
// It documents gaps rather than failing on them: `go test` stays green and the
// verdicts land in docs/conformance_results.json (override with CONF_OUT), which
// tools/conformance_report.py turns into the comparison PDF. Set CONF_STRICT=1 to
// make FAIL/GAP verdicts fail the run.
//
//	go test -run TestConformance -v
//
// Verdicts: PASS (matches DSE), FAIL (accepts/rejects where DSE would do the
// opposite, or emits a non-conformant message), GAP (feature the OMS uses is not
// implemented or is silently ignored), PARTIAL, INFO (DSE behaviour not specified
// by the OMS reference — recorded, not judged).

import (
	"encoding/json"
	"fmt"
	"math"
	"net"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/quickfixgo/quickfix"

	"velexa-sim-me/internal/engine"
)

// ---- result model -----------------------------------------------------------

type confResult struct {
	ID       string   `json:"id"`
	Category string   `json:"category"`
	Title    string   `json:"title"`
	DSE      string   `json:"dse_expected"`
	Sent     string   `json:"sent"`
	Replies  []string `json:"replies"`
	Actual   string   `json:"actual"`
	Verdict  string   `json:"verdict"`
	Note     string   `json:"note"`
}

type reply struct {
	MT  string
	F   map[string]string
	Raw string
}

func (r reply) get(tag string) string { return r.F[tag] }

func parseFIX(raw string) reply {
	r := reply{F: map[string]string{}, Raw: strings.ReplaceAll(raw, "\x01", "|")}
	for _, kv := range strings.Split(raw, "\x01") {
		if i := strings.IndexByte(kv, '='); i > 0 {
			if _, dup := r.F[kv[:i]]; !dup {
				r.F[kv[:i]] = kv[i+1:]
			}
		}
	}
	r.MT = r.F["35"]
	return r
}

// ---- initiator ----------------------------------------------------------------

type confApp struct {
	in        chan reply
	logon     chan struct{}
	out       atomic.Value // last sent raw
	loggedOut int32
}

func (a *confApp) OnCreate(quickfix.SessionID) {}

func (a *confApp) OnLogon(quickfix.SessionID) {
	select {
	case a.logon <- struct{}{}:
	default:
	}
}

func (a *confApp) OnLogout(quickfix.SessionID) { atomic.StoreInt32(&a.loggedOut, 1) }

func (a *confApp) ToAdmin(*quickfix.Message, quickfix.SessionID) {}

func (a *confApp) ToApp(m *quickfix.Message, _ quickfix.SessionID) error {
	a.out.Store(m.String())
	return nil
}

func (a *confApp) push(m *quickfix.Message) {
	select {
	case a.in <- parseFIX(m.String()):
	default:
	}
}

func (a *confApp) FromAdmin(m *quickfix.Message, _ quickfix.SessionID) quickfix.MessageRejectError {
	a.push(m)
	return nil
}

func (a *confApp) FromApp(m *quickfix.Message, _ quickfix.SessionID) quickfix.MessageRejectError {
	a.push(m)
	return nil
}

type confSess struct {
	app *confApp
	ini *quickfix.Initiator
	sid quickfix.SessionID
}

type harness struct {
	t       *testing.T
	eng     *engine.Engine
	port    int
	target  string // the acceptor's SenderCompID ("" = DSEME)
	sess    *confSess
	n       int
	symN    int
	prev    map[string]float64
	mu      sync.Mutex
	results []confResult
}

func freePort(t *testing.T) int {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	return l.Addr().(*net.TCPAddr).Port
}

func (h *harness) newSession(sender string) *confSess {
	target := h.target
	if target == "" {
		target = "DSEME"
	}
	cfg := fmt.Sprintf(`[DEFAULT]
ConnectionType=initiator
SocketConnectHost=127.0.0.1
SocketConnectPort=%d
HeartBtInt=30
ReconnectInterval=1
StartTime=00:00:00
EndTime=00:00:00
UseDataDictionary=N
ResetOnLogon=Y
DefaultApplVerID=9

[SESSION]
BeginString=FIXT.1.1
SenderCompID=%s
TargetCompID=%s
`, h.port, sender, target)
	settings, err := quickfix.ParseSettings(strings.NewReader(cfg))
	if err != nil {
		h.t.Fatal(err)
	}
	app := &confApp{in: make(chan reply, 256), logon: make(chan struct{}, 1)}
	ini, err := quickfix.NewInitiator(app, quickfix.NewMemoryStoreFactory(), settings, quickfix.NewNullLogFactory())
	if err != nil {
		h.t.Fatal(err)
	}
	if err := ini.Start(); err != nil {
		h.t.Fatal(err)
	}
	select {
	case <-app.logon:
	case <-time.After(8 * time.Second):
		ini.Stop()
		h.t.Fatalf("FIX logon for %s did not complete", sender)
	}
	return &confSess{app: app, ini: ini,
		sid: quickfix.SessionID{BeginString: "FIXT.1.1", SenderCompID: sender, TargetCompID: target}}
}

// ---- sending --------------------------------------------------------------------

type ov map[string]string

// build makes a message of type mt from tag→value. A "" value drops the tag. Tag
// 50 lives in the header; "_noparty" suppresses the default party group.
func buildMsg(mt string, f ov) *quickfix.Message {
	m := quickfix.NewMessage()
	m.Header.SetString(35, mt)
	tags := make([]string, 0, len(f))
	for k := range f {
		tags = append(tags, k)
	}
	sort.Strings(tags)
	for _, k := range tags {
		v := f[k]
		if v == "" || strings.HasPrefix(k, "_") {
			continue
		}
		n, _ := strconv.Atoi(k)
		if n == 50 {
			m.Header.SetString(quickfix.Tag(n), v)
			continue
		}
		m.Body.SetString(quickfix.Tag(n), v)
	}
	if f["_party"] == "1" {
		g := quickfix.NewRepeatingGroup(453, quickfix.GroupTemplate{
			quickfix.GroupElement(448), quickfix.GroupElement(447), quickfix.GroupElement(452)})
		e := g.Add()
		e.SetString(448, "CLIENT01")
		e.SetString(447, "C")
		e.SetString(452, "5")
		m.Body.SetGroup(g)
	}
	return m
}

func drain(s *confSess) {
	for {
		select {
		case <-s.app.in:
		default:
			return
		}
	}
}

// collect gathers inbound messages: waits up to first for the first one, then
// until the line has been quiet for quiet. Heartbeat/Logon admin traffic is
// ignored unless wantAdmin is set.
func (s *confSess) collect(first, quiet time.Duration, wantAdmin bool) []reply {
	var out []reply
	timer := time.NewTimer(first)
	defer timer.Stop()
	for {
		select {
		case r := <-s.app.in:
			if !wantAdmin && (r.MT == "0" || r.MT == "A") {
				continue
			}
			out = append(out, r)
			if !timer.Stop() {
				select {
				case <-timer.C:
				default:
				}
			}
			timer.Reset(quiet)
		case <-timer.C:
			return out
		}
	}
}

func (h *harness) clid() string { h.n++; return fmt.Sprintf("CF%05d", h.n) }

func now60() string { return time.Now().UTC().Format("20060102-15:04:05.000") }

// send transmits one message on the main session and returns what came back.
func (h *harness) send(mt string, f ov) (string, []reply) {
	return h.sendOn(h.sess, mt, f, false)
}

func (h *harness) sendOn(s *confSess, mt string, f ov, admin bool) (string, []reply) {
	drain(s)
	m := buildMsg(mt, f)
	if err := quickfix.SendToTarget(m, s.sid); err != nil {
		h.t.Fatalf("send %s: %v", mt, err)
	}
	rs := s.collect(700*time.Millisecond, 120*time.Millisecond, admin)
	sent, _ := s.app.out.Load().(string)
	return strings.ReplaceAll(sent, "\x01", "|"), rs
}

// orderFields is the OMS's exact new-order tag set for a symbol.
func (h *harness) orderFields(sym string, o ov) ov {
	f := ov{
		"1": "BO1001", "11": h.clid(), "55": sym, "54": "1", "38": "100", "40": "2",
		"44": fmt.Sprintf("%.2f", h.prev[sym]), "59": "0", "60": now60(), "762": "PUBLIC",
		"50": "TRADER1", "529": "N", "_party": "1",
	}
	for k, v := range o {
		if v == "" {
			delete(f, k)
		} else {
			f[k] = v
		}
	}
	return f
}

func (h *harness) order(sym string, o ov) (string, []reply, string) {
	f := h.orderFields(sym, o)
	sent, rs := h.send("D", f)
	return sent, rs, f["11"]
}

func (h *harness) cancelMsg(sym, orig string, side string, qty string) (string, []reply) {
	return h.send("F", ov{"1": "BO1001", "11": h.clid(), "41": orig, "38": qty, "55": sym, "54": side,
		"60": now60(), "50": "TRADER1"})
}

// replaceMsg is the OMS's 35=G: the new-order fields plus 41, WITHOUT Symbol(55)
// and SecuritySubType(762).
func (h *harness) replaceMsg(orig string, o ov) (string, []reply) {
	f := ov{"1": "BO1001", "11": h.clid(), "41": orig, "54": "1", "38": "100", "40": "2",
		"44": "100.00", "59": "0", "60": now60(), "50": "TRADER1", "529": "N", "_party": "1"}
	for k, v := range o {
		if v == "" {
			delete(f, k)
		} else {
			f[k] = v
		}
	}
	return h.send("G", f)
}

// ---- instruments & book helpers -------------------------------------------------

func (h *harness) sym(prev float64, board string) string {
	h.symN++
	s := fmt.Sprintf("T%03d%s", h.symN, board[:1])
	h.eng.ListInstruments(engine.UniverseSet{
		Instr: []engine.Instrument{{Symbol: s, Name: s + " Ltd", Prev: prev}},
		Meta:  map[string]engine.InstrMeta{s: {Board: board, Tick: 0.10, ISIN: "BD0001" + fmt.Sprintf("%06d", h.symN)}},
	})
	h.prev[s] = prev
	return s
}

func (h *harness) seed(sym string, side engine.Side, px float64, qty int64) {
	o := h.eng.NewOrder(sym, side, engine.Limit, px, qty, "SEED")
	h.eng.SubmitAndWait(o)
}

func (h *harness) depth(sym string, buy bool) []engine.Level {
	h.eng.Mu.RLock()
	defer h.eng.Mu.RUnlock()
	return h.eng.Book(sym).DepthSide(buy, 5)
}

func (h *harness) hasOpen(clid string) bool {
	h.eng.Mu.RLock()
	defer h.eng.Mu.RUnlock()
	return h.eng.FindOpen(clid, "") != nil
}

// ---- verdict helpers ---------------------------------------------------------------

func apps(rs []reply) []reply {
	var out []reply
	for _, r := range rs {
		if r.MT == "8" {
			out = append(out, r)
		}
	}
	return out
}

func hasMT(rs []reply, mt string) bool {
	for _, r := range rs {
		if r.MT == mt {
			return true
		}
	}
	return false
}

func accepted(rs []reply) bool {
	if hasMT(rs, "3") || hasMT(rs, "j") || hasMT(rs, "9") {
		return false
	}
	er := apps(rs)
	if len(er) == 0 {
		return false
	}
	for _, r := range er {
		if r.get("150") != "8" {
			return true
		}
	}
	return false
}

func rejected(rs []reply) bool {
	if hasMT(rs, "3") || hasMT(rs, "j") || hasMT(rs, "9") {
		return true
	}
	for _, r := range apps(rs) {
		if r.get("150") == "8" {
			return true
		}
	}
	return false
}

func cumQty(rs []reply) float64 {
	var m float64
	for _, r := range apps(rs) {
		if v, _ := strconv.ParseFloat(r.get("14"), 64); v > m {
			m = v
		}
	}
	return m
}

func summary(rs []reply) string {
	if len(rs) == 0 {
		return "no response"
	}
	var parts []string
	for _, r := range rs {
		switch r.MT {
		case "8":
			s := fmt.Sprintf("35=8 150=%s 39=%s", r.get("150"), r.get("39"))
			if v := r.get("14"); v != "" && v != "0" {
				s += " 14=" + v
			}
			if v := r.get("58"); v != "" {
				s += " 58=" + v
			}
			parts = append(parts, s)
		case "3", "j", "9":
			s := "35=" + r.MT
			for _, t := range []string{"371", "372", "373", "58", "102", "103"} {
				if v := r.get(t); v != "" {
					s += " " + t + "=" + v
				}
			}
			parts = append(parts, s)
		default:
			parts = append(parts, "35="+r.MT)
		}
	}
	return strings.Join(parts, " ; ")
}

type verdict struct{ v, note string }

func pass(note string) verdict { return verdict{"PASS", note} }

func fail(note string) verdict { return verdict{"FAIL", note} }

func gap(note string) verdict { return verdict{"GAP", note} }

func info(note string) verdict { return verdict{"INFO", note} }

func partial(note string) verdict { return verdict{"PARTIAL", note} }

func expectAccept(rs []reply) verdict {
	if accepted(rs) {
		return pass("accepted")
	}
	return fail("rejected or no response: " + summary(rs))
}

func expectReject(rs []reply) verdict {
	if rejected(rs) {
		return pass("rejected: " + summary(rs))
	}
	return fail("accepted although DSE rejects: " + summary(rs))
}

func (h *harness) record(cat, id, title, dse, sent string, rs []reply, v verdict) {
	r := confResult{ID: id, Category: cat, Title: title, DSE: dse, Sent: sent,
		Actual: summary(rs), Verdict: v.v, Note: v.note}
	for _, x := range rs {
		r.Replies = append(r.Replies, x.Raw)
	}
	h.results = append(h.results, r)
	h.t.Logf("%-8s %-5s %s — %s", id, v.v, title, r.Actual)
}

// c runs one dynamic case.
func (h *harness) c(cat, id, title, dse string, fn func() (string, []reply, verdict)) {
	sent, rs, v := fn()
	h.record(cat, id, title, dse, sent, rs, v)
}

// static records a case decided by code inspection rather than traffic.
func (h *harness) static(cat, id, title, dse, evidence string, v verdict) {
	r := confResult{ID: id, Category: cat, Title: title, DSE: dse, Sent: "(code inspection)",
		Actual: evidence, Verdict: v.v, Note: v.note}
	h.results = append(h.results, r)
	h.t.Logf("%-8s %-5s %s — %s", id, v.v, title, evidence)
}

// ---- the suite ----------------------------------------------------------------

func TestConformance(t *testing.T) {
	dir := t.TempDir()
	wd := repoRoot(t)
	t.Setenv("ME_FIX_AUTH", "off")
	t.Setenv("ME_FIX_DD", "on")
	t.Setenv("ME_FIX_SPEC_DIR", filepath.Join(wd, "spec"))
	t.Setenv("ME_AUTO_LIST", "off")
	t.Setenv("ME_INSTRUMENT_FILE", filepath.Join(dir, "instruments.json"))
	t.Setenv("ME_FIX_MSGLOG_FILE", filepath.Join(dir, "fixmsg.log"))
	t.Setenv("ME_FIX_RESEND_KEEP", "5") // so SS-3's resend reaches past what is kept: gap-fill, then the newest resent
	if _, err := os.Stat(filepath.Join(wd, "spec", "FIX50SP2.xml")); err != nil {
		t.Skipf("FIX dictionary not found: %v", err)
	}

	eng := engine.NewEngine([]engine.Instrument{{Symbol: "SEEDONLY", Name: "seed", Prev: 100}}, engine.NewBus())
	go eng.Run()

	port := freePort(t)
	fa, err := Start(eng, strconv.Itoa(port), "DSEME")
	if err != nil {
		t.Fatalf("start acceptor: %v", err)
	}
	defer fa.Stop()
	time.Sleep(300 * time.Millisecond)

	h := &harness{t: t, eng: eng, port: port, prev: map[string]float64{}}
	h.sess = h.newSession("TESTBRK")
	defer h.sess.ini.Stop()

	h.runOrderTypes()
	h.runTIF()
	h.runSideAndQty()
	h.runBoardGroups()
	h.runExecInst()
	h.runQtyOptions()
	h.runTagsAndReports()
	h.runPreTrade()
	h.runModifyCancel()
	h.runPhases()
	h.runMatching()
	h.runStatic()
	h.runSession()

	out := os.Getenv("CONF_OUT")
	if out == "" {
		out = filepath.Join(wd, "docs", "conformance_results.json")
	}
	counts := map[string]int{}
	for _, r := range h.results {
		counts[r.Verdict]++
	}
	b, _ := json.MarshalIndent(map[string]any{
		"generated": time.Now().UTC().Format(time.RFC3339),
		"counts":    counts,
		"total":     len(h.results),
		"results":   h.results,
	}, "", " ")
	if err := os.MkdirAll(filepath.Dir(out), 0o755); err == nil {
		_ = os.WriteFile(out, b, 0o644)
	}
	t.Logf("conformance: %d cases %v -> %s", len(h.results), counts, out)
	// A FAIL is a regression (behaviour that worked, or that the DSE requires and
	// the engine claims to do, is broken): it always fails the run. GAPs are known
	// unimplemented features; CONF_STRICT=1 makes them fail too.
	if counts["FAIL"] > 0 {
		t.Errorf("conformance failures: %v", counts)
	}
	if os.Getenv("CONF_STRICT") == "1" && counts["GAP"] > 0 {
		t.Fatalf("conformance gaps: %v", counts)
	}
}

// ---- 1. order type (tag 40) ------------------------------------------------------

func (h *harness) runOrderTypes() {
	cat := "Order type (40)"
	s := h.sym(100, "MAIN")
	h.c(cat, "OT-01", "Limit (40=2) with price", "Accepted, rests on book", func() (string, []reply, verdict) {
		sent, rs, _ := h.order(s, ov{"44": "99.00"})
		return sent, rs, expectAccept(rs)
	})
	np := h.sym(100, "MAIN")
	h.seed(np, engine.Sell, 100, 500)
	h.c(cat, "OT-02", "Limit (40=2) without price (44), liquid book", "Rejected: price required for a limit order", func() (string, []reply, verdict) {
		sent, rs, _ := h.order(np, ov{"44": ""})
		if rejected(rs) {
			return sent, rs, pass("rejected")
		}
		return sent, rs, fail("accepted and executed — the engine silently converts a 40=2 order with no price into a MARKET order (fixacceptor.go:465-479)")
	})
	m := h.sym(100, "MAIN")
	h.seed(m, engine.Sell, 100.5, 500)
	h.c(cat, "OT-03", "Market (40=1) without price, liquid book", "Matches at best available price", func() (string, []reply, verdict) {
		sent, rs, _ := h.order(m, ov{"40": "1", "44": "", "38": "100"})
		if cumQty(rs) == 100 {
			return sent, rs, pass("filled 100")
		}
		return sent, rs, fail("not filled")
	})
	h.c(cat, "OT-04", "Market (40=1) carrying a price outside the post-close session", "Rejected: 'Price is not allowed for market order' (a price is only allowed, and must be the close, in post-close)", func() (string, []reply, verdict) {
		sent, rs, _ := h.order(m, ov{"40": "1", "44": "100.00", "38": "100"})
		for _, r := range apps(rs) {
			if r.get("150") == "8" && strings.Contains(r.get("58"), "Price is not allowed for market order") {
				return sent, rs, pass("rejected as the DSE does")
			}
		}
		return sent, rs, fail("not rejected with the DSE reason")
	})
	e := h.sym(100, "MAIN")
	h.c(cat, "OT-05", "Market (40=1) on an empty book", "Order not executed (no liquidity); treatment of the remainder not specified by the OMS reference", func() (string, []reply, verdict) {
		sent, rs, _ := h.order(e, ov{"40": "1", "44": ""})
		if rejected(rs) {
			return sent, rs, info("rejected with 'no liquidity' — market orders never rest")
		}
		return sent, rs, info("accepted: " + summary(rs))
	})
	mb := h.sym(100, "MAIN")
	h.seed(mb, engine.Sell, 100.5, 60)
	h.seed(mb, engine.Sell, 101.0, 500)
	h.c(cat, "OT-06", "Market-At-Best (40=Z)", "DSE-specific type: accepted and matched at best", func() (string, []reply, verdict) {
		sent, rs, _ := h.order(mb, ov{"40": "Z", "44": "", "38": "100"})
		if hasMT(rs, "3") {
			return sent, rs, gap("OrdType Z refused by the session layer (35=3) — Market-At-Best is not supported")
		}
		// At-best takes the best opposite price only (100.50 x 60); the rest rests there.
		if cumQty(rs) == 60 && accepted(rs) {
			return sent, rs, pass("filled 60 at the best ask 100.50, remainder rests at 100.50")
		}
		return sent, rs, fail(summary(rs))
	})
	st := h.sym(100, "MAIN")
	h.c(cat, "OT-07", "Stop (40=3) with StopPx (99)", "Not used by the OMS (informational)", func() (string, []reply, verdict) {
		sent, rs, _ := h.order(st, ov{"40": "3", "44": "", "99": "101.00"})
		return sent, rs, info("engine: " + summary(rs))
	})
	h.c(cat, "OT-08", "Stop-limit (40=4) with StopPx and price", "Not used by the OMS (informational)", func() (string, []reply, verdict) {
		sent, rs, _ := h.order(st, ov{"40": "4", "44": "101.00", "99": "101.00"})
		return sent, rs, info("engine: " + summary(rs))
	})
	for _, ot := range []string{"5", "9"} {
		ot := ot
		h.c(cat, "OT-09"+ot, "Unsupported OrdType 40="+ot, "Rejected as an unsupported order type", func() (string, []reply, verdict) {
			sent, rs, _ := h.order(e, ov{"40": ot})
			if rejected(rs) {
				return sent, rs, pass("rejected: " + summary(rs))
			}
			return sent, rs, fail("accepted and silently treated as a Limit order (mapping default in fixacceptor.go:465-479)")
		})
	}
}

// ---- 2. time in force (tag 59) ---------------------------------------------------

func (h *harness) runTIF() {
	cat := "Validity (59)"
	s := h.sym(100, "MAIN")
	restsOn := func(sym, cl string) bool { return h.hasOpen(cl) }
	for _, tc := range []struct{ v, name, dse string }{
		{"0", "Day", "Accepted, rests until close"},
		{"1", "GTC", "Accepted, rests until cancelled"},
	} {
		tc := tc
		h.c(cat, "TIF-"+tc.v, tc.name+" (59="+tc.v+") resting limit", tc.dse, func() (string, []reply, verdict) {
			sent, rs, cl := h.order(s, ov{"59": tc.v, "44": "95.00"})
			if accepted(rs) && restsOn(s, cl) {
				return sent, rs, pass("accepted and resting")
			}
			return sent, rs, fail("not resting: " + summary(rs))
		})
	}
	io := h.sym(100, "MAIN")
	h.seed(io, engine.Sell, 100, 50)
	h.c(cat, "TIF-3a", "IOC (59=3) partially fillable (50 of 100)", "Fills 50, cancels the remaining 50", func() (string, []reply, verdict) {
		sent, rs, _ := h.order(io, ov{"59": "3", "44": "100.00", "38": "100"})
		if cumQty(rs) == 50 && !h.hasOpen(rs[0].get("11")) {
			return sent, rs, pass("filled 50, remainder cancelled: " + summary(rs))
		}
		return sent, rs, fail("unexpected: " + summary(rs))
	})
	io2 := h.sym(100, "MAIN")
	h.c(cat, "TIF-3b", "IOC (59=3) with no liquidity", "Cancelled / expired with no fill", func() (string, []reply, verdict) {
		sent, rs, _ := h.order(io2, ov{"59": "3", "44": "100.00"})
		if cumQty(rs) == 0 && len(rs) > 0 && (rs[len(rs)-1].get("150") == "4" || rs[len(rs)-1].get("150") == "C") {
			return sent, rs, pass("cancelled")
		}
		return sent, rs, fail("unexpected: " + summary(rs))
	})
	fk := h.sym(100, "MAIN")
	h.seed(fk, engine.Sell, 100, 200)
	h.c(cat, "TIF-4a", "FOK (59=4) fully fillable", "Fully filled", func() (string, []reply, verdict) {
		sent, rs, _ := h.order(fk, ov{"59": "4", "44": "100.00", "38": "100"})
		if cumQty(rs) == 100 {
			return sent, rs, pass("filled 100")
		}
		return sent, rs, fail("not filled: " + summary(rs))
	})
	fk2 := h.sym(100, "MAIN")
	h.seed(fk2, engine.Sell, 100, 50)
	h.c(cat, "TIF-4b", "FOK (59=4) insufficient liquidity", "Whole order cancelled, no partial fill", func() (string, []reply, verdict) {
		sent, rs, _ := h.order(fk2, ov{"59": "4", "44": "100.00", "38": "100"})
		if cumQty(rs) == 0 && len(rs) > 0 {
			return sent, rs, pass("no partial fill: " + summary(rs))
		}
		return sent, rs, fail("partial fill or no response: " + summary(rs))
	})
	gd := h.sym(100, "MAIN")
	h.c(cat, "TIF-6a", "GTD (59=6) without expiry date (OMS sends none)", "Rejected without ExpireDate(432)/ExpireTime(126) — OMS reference says DSE 'will probably reject'", func() (string, []reply, verdict) {
		sent, rs, _ := h.order(gd, ov{"59": "6", "44": "95.00"})
		if rejected(rs) {
			return sent, rs, pass("rejected")
		}
		return sent, rs, gap("accepted; with no expiry the order never expires (parseExpire returns zero)")
	})
	exp := time.Now().UTC().Add(3 * time.Second).Format("20060102-15:04:05.000")
	h.c(cat, "TIF-6b", "GTD (59=6) with ExpireTime (126) 3 s ahead — expires", "Order removed at expiry AND an Execution Report (150=C Expired) is sent", func() (string, []reply, verdict) {
		sent, rs, cl := h.order(gd, ov{"59": "6", "44": "94.00", "126": exp})
		if !accepted(rs) {
			return sent, rs, fail("not accepted: " + summary(rs))
		}
		time.Sleep(4500 * time.Millisecond)
		late := (&confSess{app: h.sess.app}).collect(300*time.Millisecond, 100*time.Millisecond, false)
		if h.hasOpen(cl) {
			return sent, append(rs, late...), fail("still resting after expiry")
		}
		if len(late) == 0 {
			return sent, rs, gap("order removed from the book at expiry but NO Execution Report (150=C) was sent — the broker is never told")
		}
		return sent, append(rs, late...), pass("expired and reported")
	})
	h.c(cat, "TIF-6c", "GTD (59=6) with ExpireDate (432) only", "Accepted; expires at end of that date", func() (string, []reply, verdict) {
		sent, rs, cl := h.order(gd, ov{"59": "6", "44": "93.00", "432": time.Now().UTC().Add(48 * time.Hour).Format("20060102")})
		if accepted(rs) && h.hasOpen(cl) {
			return sent, rs, pass("accepted and resting")
		}
		return sent, rs, fail(summary(rs))
	})
	for _, tc := range []struct{ v, name string }{{"2", "At the Opening"}, {"5", "Good Till Crossing"}, {"7", "At the Close"}} {
		tc := tc
		h.c(cat, "TIF-"+tc.v, tc.name+" (59="+tc.v+")", "Honoured as its own validity (not offered by any OMS screen, but the builder can pass it)", func() (string, []reply, verdict) {
			sent, rs, _ := h.order(gd, ov{"59": tc.v, "44": "92.00"})
			if accepted(rs) {
				return sent, rs, gap("accepted but silently treated as Day (mapTIF default) — semantics not implemented")
			}
			return sent, rs, info("rejected: " + summary(rs))
		})
	}
	h.c(cat, "TIF-S", "Session (59=S) — offered by the OMS trader UI", "Accepted (the OMS offers it); not a FIX 5.0 SP1/SP2 enum value", func() (string, []reply, verdict) {
		sent, rs, _ := h.order(gd, ov{"59": "S", "44": "91.00"})
		if accepted(rs) {
			return sent, rs, pass("accepted")
		}
		return sent, rs, gap("rejected by the strict dictionary (35=3) — an order the OMS UI allows can never reach the engine")
	})
	h.c(cat, "TIF-X", "Missing TimeInForce", "Defaults to Day", func() (string, []reply, verdict) {
		sent, rs, _ := h.order(gd, ov{"59": "", "44": "90.50"})
		return sent, rs, expectAccept(rs)
	})
}

// ---- 3. side and quantity validation -----------------------------------------------

func (h *harness) runSideAndQty() {
	cat := "Side & quantity"
	s := h.sym(100, "MAIN")
	h.c(cat, "SD-1", "Buy (54=1)", "Accepted", func() (string, []reply, verdict) {
		sent, rs, _ := h.order(s, ov{"54": "1", "44": "95.00"})
		return sent, rs, expectAccept(rs)
	})
	h.c(cat, "SD-2", "Sell (54=2)", "Accepted", func() (string, []reply, verdict) {
		sent, rs, _ := h.order(s, ov{"54": "2", "44": "105.00"})
		return sent, rs, expectAccept(rs)
	})
	h.c(cat, "SD-3", "Missing Side (54)", "Rejected", func() (string, []reply, verdict) {
		sent, rs, _ := h.order(s, ov{"54": ""})
		return sent, rs, expectReject(rs)
	})
	h.c(cat, "SD-4", "Side 54=5 (sell short)", "Rejected (the OMS reference: no short-sell side)", func() (string, []reply, verdict) {
		sent, rs, _ := h.order(s, ov{"54": "5"})
		if rejected(rs) {
			return sent, rs, pass("rejected")
		}
		return sent, rs, fail("accepted — any Side other than '2' is treated as a BUY (fixacceptor.go:461-464), so a sell-short would execute as a purchase")
	})
	h.c(cat, "QT-1", "OrderQty = 0", "Rejected", func() (string, []reply, verdict) {
		sent, rs, _ := h.order(s, ov{"38": "0"})
		return sent, rs, expectReject(rs)
	})
	h.c(cat, "QT-2", "Fractional OrderQty (10.5)", "Rejected (whole shares only)", func() (string, []reply, verdict) {
		sent, rs, _ := h.order(s, ov{"38": "10.5"})
		return sent, rs, expectReject(rs)
	})
	h.c(cat, "QT-3", "Negative OrderQty", "Rejected", func() (string, []reply, verdict) {
		sent, rs, _ := h.order(s, ov{"38": "-5"})
		return sent, rs, expectReject(rs)
	})
	h.c(cat, "QT-4", "Quantity a multiple of 1000", "Accepted", func() (string, []reply, verdict) {
		sent, rs, _ := h.order(s, ov{"38": "2000", "44": "94.00"})
		return sent, rs, expectAccept(rs)
	})
	lotSym := "LOT1000"
	h.eng.ListInstruments(engine.UniverseSet{
		Instr: []engine.Instrument{{Symbol: lotSym, Name: "Lot 1000", Prev: 100}},
		Meta:  map[string]engine.InstrMeta{lotSym: {Board: "Main (A)", Tick: 0.10, Lot: 1000}},
	})
	h.prev[lotSym] = 100
	h.c(cat, "QT-5", "Quantity 1500 on a lot-1000 instrument", "Rejected: '(792) Minimum quantity step: 1000' (observed DSE reject, instrument-dependent)", func() (string, []reply, verdict) {
		sent, rs, _ := h.order(lotSym, ov{"38": "1500", "44": "94.00"})
		if rejected(rs) {
			return sent, rs, pass("rejected: " + summary(rs))
		}
		return sent, rs, gap("accepted — the market-lot rule was not applied")
	})
	h.c(cat, "QT-5b", "Quantity 2000 on a lot-1000 instrument", "Accepted (whole lots)", func() (string, []reply, verdict) {
		sent, rs, _ := h.order(lotSym, ov{"38": "2000", "44": "94.00"})
		return sent, rs, expectAccept(rs)
	})
	h.c(cat, "QT-6", "Duplicate ClOrdID (11) resubmitted", "Rejected as a duplicate order", func() (string, []reply, verdict) {
		f := h.orderFields(s, ov{"44": "95.00"})
		h.send("D", f)
		sent, rs := h.send("D", f)
		if rejected(rs) {
			return sent, rs, pass("rejected")
		}
		return sent, rs, gap("second order with the same ClOrdID accepted — no duplicate-ClOrdID check (also breaks cancel/replace by ClOrdID)")
	})
	h.c(cat, "QT-7", "Missing Symbol (55)", "Rejected", func() (string, []reply, verdict) {
		sent, rs, _ := h.order(s, ov{"55": ""})
		return sent, rs, expectReject(rs)
	})
	h.c(cat, "QT-8", "Unknown symbol", "Rejected (not in the security master)", func() (string, []reply, verdict) {
		sent, rs, _ := h.order(s, ov{"55": "NOSUCHSYM"})
		return sent, rs, expectReject(rs)
	})
}

// ---- 4. board group (762) -------------------------------------------------------------

func (h *harness) runBoardGroups() {
	cat := "Board group (762)"
	main := h.sym(100, "MAIN")
	sme := h.sym(100, "SME")
	atb := h.sym(100, "ATB")
	groups := []string{"PUBLIC", "ATBPUB", "SPUBLIC", "BLOCK", "SBLOCK", "BUYIN", "ATBBUYIN", "SBUYIN",
		"DEBT", "ATBDEBT", "SDEBT", "ALTDBT", "BUYDBT", "YIELDDBT", "INDEX", "ATBX", "SCPX"}
	for i, g := range groups {
		g := g
		valid := g == "PUBLIC"
		dse := "Group must match the instrument's board/segment — rejected on a MAIN-board equity"
		if valid {
			dse = "Accepted (public market, MAIN board)"
		}
		h.c(cat, fmt.Sprintf("BG-%02d", i+1), "762="+g+" on a MAIN-board equity", dse, func() (string, []reply, verdict) {
			sent, rs, _ := h.order(main, ov{"762": g, "44": "95.00"})
			switch {
			case valid && accepted(rs):
				return sent, rs, pass("accepted")
			case valid:
				return sent, rs, fail(summary(rs))
			case rejected(rs):
				return sent, rs, pass("rejected")
			}
			return sent, rs, gap("accepted — tag 762 is never read, so board groups are not validated against the instrument")
		})
	}
	h.c(cat, "BG-18", "762=SPUBLIC on an SME-board equity", "Accepted", func() (string, []reply, verdict) {
		sent, rs, _ := h.order(sme, ov{"762": "SPUBLIC", "44": "95.00"})
		return sent, rs, expectAccept(rs)
	})
	h.c(cat, "BG-19", "762=ATBPUB on an ATB-board equity", "Accepted", func() (string, []reply, verdict) {
		sent, rs, _ := h.order(atb, ov{"762": "ATBPUB", "44": "95.00"})
		return sent, rs, expectAccept(rs)
	})
	h.c(cat, "BG-20", "Missing 762", "Accepted (defaults to the instrument's group)", func() (string, []reply, verdict) {
		sent, rs, _ := h.order(main, ov{"762": "", "44": "95.00"})
		return sent, rs, expectAccept(rs)
	})
}

// ---- 5. execution instruction (tag 18) -----------------------------------------------------

func (h *harness) runExecInst() {
	cat := "ExecInst (18)"
	for i, tc := range []struct{ v, name, dse string }{
		{"n", "Placed", "Normal placed order — rests on the book"},
		{"S", "Park", "Order parked: held off the book, no matching"},
		{"w", "Block", "Block-board order"},
		{"w S", "Block Park", "Block order parked"},
	} {
		tc := tc
		s := h.sym(100, "MAIN")
		h.c(cat, fmt.Sprintf("EI-%d", i+1), "18="+tc.name+" ("+tc.v+") on a new order", tc.dse, func() (string, []reply, verdict) {
			sent, rs, cl := h.order(s, ov{"18": tc.v, "44": "95.00"})
			if !accepted(rs) {
				return sent, rs, gap("rejected — '" + tc.v + "' is not in the engine's tag-18 dictionary enum, so the OMS's DSE-specific instruction never reaches the engine")
			}
			switch tc.v {
			case "S", "w S":
				if h.hasOpen(cl) {
					return sent, rs, gap("accepted but the order rests on the book — Park is not implemented (tag 18 never read)")
				}
				return sent, rs, pass("parked")
			}
			return sent, rs, partial("accepted; instruction not interpreted (tag 18 never read)")
		})
	}
	s := h.sym(100, "MAIN")
	for i, v := range []string{"q", "w q"} {
		v := v
		h.c(cat, fmt.Sprintf("EI-%d", i+5), "18="+v+" (Unpark) on a modify (35=G)", "Un-parks the order and places it on the book", func() (string, []reply, verdict) {
			_, rs0, cl := h.order(s, ov{"44": "95.00"})
			if !accepted(rs0) {
				return "", rs0, fail("setup order rejected")
			}
			sent, rs := h.replaceMsg(cl, ov{"18": v, "44": "95.00"})
			if rejected(rs) {
				return sent, rs, gap("rejected — Unpark ('" + v + "') cannot be sent through the dictionary-validated session")
			}
			return sent, rs, partial("accepted as an ordinary amend; instruction ignored")
		})
	}
}

// ---- 6. quantity options (1138, 111, 110) -----------------------------------------------------

func (h *harness) runQtyOptions() {
	cat := "Quantity options"
	s := h.sym(100, "MAIN")
	h.c(cat, "QO-1", "DisplayQty (1138=30) iceberg on a 100-lot (the OMS's tag)", "Only 30 shown on the book", func() (string, []reply, verdict) {
		sent, rs, _ := h.order(s, ov{"1138": "30", "44": "90.00"})
		d := h.depth(s, true)
		if len(d) > 0 && d[0].Qty == 30 {
			return sent, rs, pass("book shows 30")
		}
		q := int64(0)
		if len(d) > 0 {
			q = d[0].Qty
		}
		return sent, rs, gap(fmt.Sprintf("book shows %d — tag 1138 is ignored (the engine reads tag 111 MaxFloor instead), so the iceberg is not applied to OMS orders", q))
	})
	s2 := h.sym(100, "MAIN")
	h.c(cat, "QO-2", "MaxFloor (111=30) iceberg (engine's own tag)", "(not sent by the OMS) 30 shown", func() (string, []reply, verdict) {
		sent, rs, _ := h.order(s2, ov{"111": "30", "44": "90.00"})
		d := h.depth(s2, true)
		if len(d) > 0 && d[0].Qty == 30 {
			return sent, rs, info("111 honoured — book shows 30")
		}
		return sent, rs, info("111 not honoured")
	})
	s3 := h.sym(100, "MAIN")
	h.c(cat, "QO-3", "MinQty (110) greater than OrderQty (500 > 100)", "Rejected", func() (string, []reply, verdict) {
		sent, rs, _ := h.order(s3, ov{"110": "500", "44": "90.00"})
		if rejected(rs) {
			return sent, rs, pass("rejected")
		}
		return sent, rs, gap("accepted — MinQty (110) is ignored")
	})
	s4 := h.sym(100, "MAIN")
	h.seed(s4, engine.Sell, 100, 30)
	h.c(cat, "QO-4", "MinQty (110=50) with only 30 available", "No fill (minimum quantity cannot be met)", func() (string, []reply, verdict) {
		sent, rs, _ := h.order(s4, ov{"110": "50", "44": "100.00", "38": "100"})
		if cumQty(rs) == 0 {
			return sent, rs, pass("not filled")
		}
		return sent, rs, gap("filled below the minimum quantity — MinQty (110) is ignored")
	})
}

// ---- 7. other tags & execution-report conformance ---------------------------------------------

func (h *harness) runTagsAndReports() {
	cat := "Tags & reports"
	s := h.sym(100, "MAIN")
	h.c(cat, "TG-1", "Party group 453 (448/447/452) present", "Accepted", func() (string, []reply, verdict) {
		sent, rs, _ := h.order(s, ov{"44": "90.00"})
		return sent, rs, expectAccept(rs)
	})
	h.c(cat, "TG-2", "No party group", "Accepted", func() (string, []reply, verdict) {
		sent, rs, _ := h.order(s, ov{"_party": "", "44": "90.00"})
		return sent, rs, expectAccept(rs)
	})
	h.c(cat, "TG-3", "OrderRestrictions 529=N (non-standard)", "Accepted (DSE accepted it in logged traffic)", func() (string, []reply, verdict) {
		sent, rs, _ := h.order(s, ov{"529": "N", "44": "90.00"})
		if accepted(rs) {
			return sent, rs, pass("accepted")
		}
		return sent, rs, gap("rejected: " + summary(rs))
	})
	h.c(cat, "TG-4", "Missing Account (1)", "Accepted or rejected per DSE (BO ID required by the OMS)", func() (string, []reply, verdict) {
		sent, rs, _ := h.order(s, ov{"1": "", "44": "90.00"})
		return sent, rs, info("engine: " + summary(rs))
	})
	h.c(cat, "TG-5", "Missing TransactTime (60)", "Rejected (required field)", func() (string, []reply, verdict) {
		sent, rs, _ := h.order(s, ov{"60": "", "44": "90.00"})
		return sent, rs, expectReject(rs)
	})
	h.c(cat, "TG-6", "Missing ClOrdID (11)", "Rejected", func() (string, []reply, verdict) {
		f := h.orderFields(s, ov{"44": "90.00"})
		delete(f, "11")
		sent, rs := h.send("D", f)
		return sent, rs, expectReject(rs)
	})

	// Execution-report conformance on a real fill.
	f := h.sym(100, "MAIN")
	h.seed(f, engine.Sell, 100, 100)
	sent, rs, _ := h.order(f, ov{"44": "100.00", "38": "100"})
	er := apps(rs)
	last := reply{F: map[string]string{}}
	if len(er) > 0 {
		last = er[len(er)-1]
	}
	chk := func(id, title, dse string, ok bool, good, bad string, kind func(string) verdict) {
		v := kind(bad)
		if ok {
			v = pass(good)
		}
		h.record(cat, id, title, dse, sent, rs, v)
	}
	missing := ""
	for _, t := range []string{"37", "17", "150", "39", "55", "54", "151", "14", "11"} {
		if last.get(t) == "" {
			missing += " " + t
		}
	}
	chk("ER-1", "Fill ER carries FIX-required tags (37,17,150,39,55,54,151,14,11)", "All present", missing == "",
		"all present", "missing:"+missing, fail)
	chk("ER-2", "Fill ER ExecType (150)", "150=F (Trade)", last.get("150") == "F",
		"150=F", "150="+last.get("150")+" — not a valid FIX 5.0 value (must be 'F')", fail)
	chk("ER-3", "ER echoes Account (1)", "Echoed (OMS correlates by BO ID)", last.get("1") != "",
		"echoed", "tag 1 not echoed on the execution report", gap)
	chk("ER-4", "ER echoes OrdType (40) / TimeInForce (59)", "Echoed", last.get("40") != "" && last.get("59") != "",
		"echoed", "tags 40/59 not echoed", gap)
	chk("ER-5", "ER carries last fill price/qty (31/32)", "31 and 32 on a fill", last.get("31") != "" && last.get("32") != "",
		"present", "missing 31/32 on a fill", fail)
	chk("ER-6", "ER carries average price (6) and cumulative qty (14)", "Both present", last.get("6") != "" && last.get("14") != "",
		"present", "missing 6/14", fail)
	chk("ER-7", "ER identifies the instrument by Symbol (55) only, as the DSE does", "No 48/22/207/1300 (the DSE sends none)",
		last.get("55") != "" && last.get("48") == "" && last.get("1300") == "", "55 only", "extra instrument tags", gap)

	// Reject reason code.
	_, rrs, _ := h.order(f, ov{"38": "0"})
	rj := reply{F: map[string]string{}}
	for _, r := range rrs {
		if r.MT == "8" {
			rj = r
		}
	}
	rsent, rrs2, _ := h.order(f, ov{"44": "50.00"}) // far outside the band
	for _, r := range rrs2 {
		if r.MT == "8" {
			rj = r
		}
	}
	h.c(cat, "ER-8", "Reject ER carries OrdRejReason (103) and Text (58)", "103 + 58 on every rejected order", func() (string, []reply, verdict) {
		if rj.get("58") != "" && rj.get("103") != "" {
			return rsent, rrs2, pass("103 and 58 present")
		}
		if rj.get("58") != "" {
			return rsent, rrs2, partial("Text (58) present but OrdRejReason (103) is never set")
		}
		return rsent, rrs2, fail("no reject text")
	})
	_ = rrs
}

// ---- 8. DSE pre-trade rules -------------------------------------------------------------------

type tier struct{ prev, pct float64 }

func (h *harness) runPreTrade() {
	cat := "Price band / tick"
	s := h.sym(100, "MAIN")
	h.c(cat, "PT-1", "Price on tick (100.10)", "Accepted", func() (string, []reply, verdict) {
		sent, rs, _ := h.order(s, ov{"44": "100.10"})
		return sent, rs, expectAccept(rs)
	})
	h.c(cat, "PT-2", "Price off tick (100.05)", "Rejected: off-tick", func() (string, []reply, verdict) {
		sent, rs, _ := h.order(s, ov{"44": "100.05"})
		return sent, rs, expectReject(rs)
	})
	h.c(cat, "PT-3", "Price 100.02 (sub-paisa tick)", "Rejected: off-tick", func() (string, []reply, verdict) {
		sent, rs, _ := h.order(s, ov{"44": "100.02"})
		return sent, rs, expectReject(rs)
	})

	// DSE circuit-breaker tiers (observed ±7.50% / ±8.75% rejects; 10% to Tk200 ... 3.75% above Tk5000).
	tiers := []tier{{100, .10}, {200, .10}, {200.10, .0875}, {300, .0875}, {500, .0875}, {500.10, .075},
		{800, .075}, {1000, .075}, {1000.10, .0625}, {1500, .0625}, {2000, .0625},
		{2000.10, .05}, {3000, .05}, {5000, .05}, {5000.10, .0375}, {8000, .0375}}
	round2 := func(x float64) float64 { return math.Round(x*100) / 100 }
	for i, tr := range tiers {
		tr := tr
		sym := h.sym(tr.prev, "MAIN")
		hi := math.Floor(round2(tr.prev*(1+tr.pct))*10+1e-9) / 10
		lo := math.Ceil(round2(tr.prev*(1-tr.pct))*10-1e-9) / 10
		label := fmt.Sprintf("prev %.2f ±%.2f%%", tr.prev, tr.pct*100)
		for j, c := range []struct {
			name string
			px   float64
			ok   bool
		}{
			{"upper limit", hi, true}, {"above upper", hi + 0.1, false},
			{"lower limit", lo, true}, {"below lower", lo - 0.1, false},
		} {
			c := c
			dse := "Accepted at the limit"
			if !c.ok {
				dse = "Rejected: price outside the band"
			}
			h.c(cat, fmt.Sprintf("PB-%02d%c", i+1, 'a'+j), fmt.Sprintf("%s: %s %.2f", label, c.name, c.px), dse, func() (string, []reply, verdict) {
				sent, rs, _ := h.order(sym, ov{"44": fmt.Sprintf("%.2f", c.px), "38": "100"})
				if c.ok {
					return sent, rs, expectAccept(rs)
				}
				return sent, rs, expectReject(rs)
			})
		}
	}
	mk := h.sym(100, "MAIN")
	h.seed(mk, engine.Sell, 109, 100)
	h.c(cat, "PT-4", "Market order ignores the price band", "DSE applies circuit limits to execution prices", func() (string, []reply, verdict) {
		sent, rs, _ := h.order(mk, ov{"40": "1", "44": ""})
		return sent, rs, info("market orders skip band/tick checks (engine.go:297 applies them to limits only): " + summary(rs))
	})
	// Halted / unlisted
	hl := h.sym(100, "MAIN")
	engine.SetHalted(hl, true)
	h.c(cat, "PT-5", "Order on a halted instrument", "Rejected", func() (string, []reply, verdict) {
		sent, rs, _ := h.order(hl, ov{"44": "95.00"})
		return sent, rs, expectReject(rs)
	})
	engine.SetHalted(hl, false)
	h.c(cat, "PT-6", "Order after the halt is lifted", "Accepted", func() (string, []reply, verdict) {
		sent, rs, _ := h.order(hl, ov{"44": "95.00"})
		return sent, rs, expectAccept(rs)
	})
}

// ---- 9. cancel / replace / status ------------------------------------------------------------

func (h *harness) runModifyCancel() {
	cat := "Cancel / modify / status"
	s := h.sym(100, "MAIN")
	h.c(cat, "CM-1", "Cancel a resting order (35=F)", "Execution Report 150=4 Cancelled", func() (string, []reply, verdict) {
		_, _, cl := h.order(s, ov{"44": "95.00"})
		sent, rs := h.cancelMsg(s, cl, "1", "100")
		if len(apps(rs)) > 0 && apps(rs)[0].get("150") == "4" {
			return sent, rs, pass("cancelled")
		}
		return sent, rs, fail(summary(rs))
	})
	h.c(cat, "CM-2", "Cancel an unknown order", "Order Cancel Reject (35=9)", func() (string, []reply, verdict) {
		sent, rs := h.cancelMsg(s, "NOSUCH", "1", "100")
		if hasMT(rs, "9") {
			return sent, rs, pass("35=9")
		}
		return sent, rs, fail(summary(rs))
	})
	h.c(cat, "CM-3", "35=9 carries OrderID (37) — required by FIX", "37 present on the reject", func() (string, []reply, verdict) {
		sent, rs := h.cancelMsg(s, "NOSUCH2", "1", "100")
		for _, r := range rs {
			if r.MT == "9" {
				if r.get("37") != "" {
					return sent, rs, pass("present")
				}
				return sent, rs, fail("35=9 has no OrderID (37) — fixacceptor.go:531-543")
			}
		}
		return sent, rs, fail("no 35=9")
	})
	h.c(cat, "CM-4", "Modify price WITHOUT Symbol/762 (as the OMS sends 35=G)", "Execution Report 150=5 Replaced", func() (string, []reply, verdict) {
		_, _, cl := h.order(s, ov{"44": "95.00"})
		sent, rs := h.replaceMsg(cl, ov{"44": "96.00"})
		if len(apps(rs)) > 0 && apps(rs)[0].get("150") == "5" {
			return sent, rs, pass("replaced")
		}
		return sent, rs, fail(summary(rs))
	})
	h.c(cat, "CM-5", "Modify quantity 100 -> 60", "150=5 Replaced with the new quantity", func() (string, []reply, verdict) {
		_, _, cl := h.order(s, ov{"44": "95.00"})
		sent, rs := h.replaceMsg(cl, ov{"44": "95.00", "38": "60"})
		if len(apps(rs)) > 0 && apps(rs)[0].get("150") == "5" {
			return sent, rs, pass("replaced")
		}
		return sent, rs, fail(summary(rs))
	})
	h.c(cat, "CM-6", "Modify an unknown order", "35=9", func() (string, []reply, verdict) {
		sent, rs := h.replaceMsg("NOSUCH3", ov{})
		if hasMT(rs, "9") {
			return sent, rs, pass("35=9")
		}
		return sent, rs, fail(summary(rs))
	})
	h.c(cat, "CM-7", "Modify to a price off tick (96.05)", "Rejected: off-tick", func() (string, []reply, verdict) {
		_, _, cl := h.order(s, ov{"44": "95.00"})
		sent, rs := h.replaceMsg(cl, ov{"44": "96.05"})
		if rejected(rs) {
			return sent, rs, pass("rejected")
		}
		return sent, rs, gap("accepted — replace does not re-run the tick check")
	})
	h.c(cat, "CM-8", "Modify to a price outside the band (150.00)", "Rejected: outside price band", func() (string, []reply, verdict) {
		_, _, cl := h.order(s, ov{"44": "95.00"})
		sent, rs := h.replaceMsg(cl, ov{"44": "150.00"})
		if rejected(rs) {
			return sent, rs, pass("rejected")
		}
		return sent, rs, gap("accepted — replace does not re-run the price-band check, so an order can be amended to any price")
	})
	h.c(cat, "CM-9", "Modify using the latest ClOrdID chain (replace a replaced order)", "Accepted", func() (string, []reply, verdict) {
		_, _, cl := h.order(s, ov{"44": "95.00"})
		_, r1 := h.replaceMsg(cl, ov{"44": "95.50"})
		if len(r1) == 0 {
			return "", r1, fail("first replace failed")
		}
		newCl := r1[0].get("11")
		sent, rs := h.replaceMsg(newCl, ov{"44": "96.00"})
		if len(apps(rs)) > 0 && apps(rs)[0].get("150") == "5" {
			return sent, rs, pass("replaced")
		}
		return sent, rs, fail(summary(rs))
	})
	h.c(cat, "CM-10", "Replace ER echoes OrigClOrdID (41)", "41 echoed on the replace/cancel ER", func() (string, []reply, verdict) {
		_, _, cl := h.order(s, ov{"44": "95.00"})
		sent, rs := h.replaceMsg(cl, ov{"44": "95.50"})
		if len(apps(rs)) > 0 && apps(rs)[0].get("41") == cl {
			return sent, rs, pass("echoed")
		}
		return sent, rs, gap("OrigClOrdID (41) not echoed on the execution report")
	})
	h.c(cat, "CM-11", "Order Status Request (35=H) on a live order", "Execution Report with the current status (150=I)", func() (string, []reply, verdict) {
		_, _, cl := h.order(s, ov{"44": "95.00"})
		sent, rs := h.send("H", ov{"11": cl, "54": "1", "55": s, "790": "ST1", "50": "TRADER1"})
		if len(apps(rs)) > 0 {
			return sent, rs, pass("status returned")
		}
		return sent, rs, gap("no reply — 35=H is not handled (fixacceptor.go:403-410); the OMS cannot reconcile order state")
	})
	h.c(cat, "CM-12", "Order Status Request (35=H) on an unknown order", "Execution report 150=I, 39=8 (or a reject)", func() (string, []reply, verdict) {
		sent, rs := h.send("H", ov{"11": "NOSUCH4", "54": "1", "55": s, "790": "ST2", "50": "TRADER1"})
		if len(rs) > 0 {
			return sent, rs, pass(summary(rs))
		}
		return sent, rs, gap("no reply — 35=H is not handled")
	})
	st := h.sym(100, "MAIN")
	h.c(cat, "CM-13", "Modify a parked stop order", "(not used by the OMS) Replaced", func() (string, []reply, verdict) {
		_, _, cl := h.order(st, ov{"40": "3", "44": "", "99": "104.00"})
		sent, rs := h.replaceMsg(cl, ov{"40": "3", "44": "", "99": "105.00"})
		return sent, rs, info("engine: " + summary(rs))
	})
}

// ---- 10. sessions / phases -------------------------------------------------------------------------

func (h *harness) runPhases() {
	cat := "Phases"
	s := h.sym(100, "MAIN")
	defer h.eng.SetPhase("Continuous Trading")
	h.eng.SetPhase("Pre-Open")
	var auctionCl string
	h.c(cat, "PH-1", "Limit order during Pre-Open", "Accepted into the call auction, no matching", func() (string, []reply, verdict) {
		h.seed(s, engine.Sell, 100, 100)
		sent, rs, cl := h.order(s, ov{"44": "100.00"})
		auctionCl = cl
		if accepted(rs) && cumQty(rs) == 0 {
			return sent, rs, pass("accepted, not matched")
		}
		return sent, rs, fail(summary(rs))
	})
	h.c(cat, "PH-2", "Market order during Pre-Open", "Rejected (no market orders in the auction)", func() (string, []reply, verdict) {
		sent, rs, _ := h.order(s, ov{"40": "1", "44": ""})
		return sent, rs, expectReject(rs)
	})
	h.c(cat, "PH-3", "Limit far outside the band during Pre-Open", "Rejected — circuit limits apply in every session", func() (string, []reply, verdict) {
		sent, rs, _ := h.order(h.sym(100, "MAIN"), ov{"44": "150.00"})
		if rejected(rs) {
			return sent, rs, pass("rejected")
		}
		return sent, rs, gap("accepted — Pre-Open skips the tick and price-band checks (engine.go:278-289 returns before them)")
	})
	h.c(cat, "PH-4", "Uncross at the Pre-Open -> Continuous transition", "Matching orders trade at the equilibrium price", func() (string, []reply, verdict) {
		h.eng.SetPhase("Continuous Trading")
		time.Sleep(300 * time.Millisecond)
		rs := h.sess.collect(600*time.Millisecond, 100*time.Millisecond, false)
		if cumQty(rs) > 0 {
			return "(phase change)", rs, pass("auction uncross produced a fill and reported it: " + summary(rs))
		}
		if !h.hasOpen(auctionCl) {
			return "(phase change)", rs, gap("the crossed order was executed in the uncross but the broker received NO Execution Report — auction fills are not reported over FIX")
		}
		return "(phase change)", rs, fail("crossed orders did not trade at the uncross")
	})
	g := h.sym(100, "MAIN")
	_, _, cl := h.order(g, ov{"59": "1", "44": "95.00"})
	h.eng.SetPhase("Pre-Open")
	h.eng.SetPhase("Continuous Trading")
	h.c(cat, "PH-5", "GTC order survives a phase change", "Still on the book", func() (string, []reply, verdict) {
		if h.hasOpen(cl) {
			return "(phase change)", nil, pass("still resting")
		}
		return "(phase change)", nil, fail("order lost")
	})
}

// ---- 11. matching behaviour ------------------------------------------------------------------------

func (h *harness) runMatching() {
	cat := "Matching"
	s := h.sym(100, "MAIN")
	h.seed(s, engine.Sell, 100.2, 100)
	h.seed(s, engine.Sell, 100.0, 100)
	h.c(cat, "MT-1", "Buy limit crosses the best ask — price improvement", "Executes at the resting (better) price 100.00", func() (string, []reply, verdict) {
		sent, rs, _ := h.order(s, ov{"44": "100.20", "38": "100"})
		if er := apps(rs); len(er) > 0 && cumQty(rs) == 100 && strings.HasPrefix(er[len(er)-1].get("31"), "100") && !strings.HasPrefix(er[len(er)-1].get("31"), "100.2") {
			return sent, rs, pass("executed at 100.00")
		}
		return sent, rs, fail(summary(rs))
	})
	p := h.sym(100, "MAIN")
	_, _, c1 := h.order(p, ov{"54": "2", "44": "101.00", "38": "100", "1": "A1"})
	_, _, c2 := h.order(p, ov{"54": "2", "44": "101.00", "38": "100", "1": "A2"})
	h.c(cat, "MT-2", "Price-time priority (two asks at the same price)", "Earlier order fills first", func() (string, []reply, verdict) {
		sent, rs, _ := h.order(p, ov{"44": "101.00", "38": "100"})
		if !h.hasOpen(c1) && h.hasOpen(c2) {
			return sent, rs, pass("first order filled, second still resting")
		}
		return sent, rs, fail("priority not respected")
	})
	pf := h.sym(100, "MAIN")
	h.seed(pf, engine.Sell, 100, 40)
	h.c(cat, "MT-3", "Partial fill then rest (day limit)", "Fills 40, rests the remaining 60", func() (string, []reply, verdict) {
		sent, rs, cl := h.order(pf, ov{"44": "100.00", "38": "100"})
		if cumQty(rs) == 40 && h.hasOpen(cl) {
			return sent, rs, pass("filled 40, 60 resting")
		}
		return sent, rs, fail(summary(rs))
	})
}

// ---- static findings ---------------------------------------------------------------------------------

func (h *harness) runStatic() {
	cat := "Rules not modelled"
	h.static(cat, "ST-1", "Day orders expire at market close", "Day orders are removed at close and reported",
		"at the end of the day (Closed) expireDayOrders removes Day orders and reports 150=C; GTC/GTD stay", pass("covered by engine TestTradingDayHousekeeping"))
	h.static(cat, "ST-2", "Post-close session: Market order priced at the close price", "DSE requires the close price on market orders after close",
		"Post-Close accepts only orders at the closing price; a market order is priced at the close", pass("covered by engine TestTradingDayHousekeeping and the FIX market-price rule"))
	h.static(cat, "ST-3", "Automatic circuit breaker / volatility halt", "Instrument halts on breaching limits; DSE halts per board rules",
		"only a static price-band check plus the manual per-symbol halt added in the admin console; no automatic trigger", gap("not modelled"))
	h.static(cat, "ST-4", "Category-based rules (A/B/G/N/Z) and board-specific rules", "Different lot sizes / bands / sessions by category and board",
		"Instrument has Board/ISIN/Tick only; no category, market lot, face value; band tiers are board-independent", gap("not modelled"))
	h.static(cat, "ST-5", "Pre-trade RMS (buying power, holdings, position limit) on the FIX path", "OMS-side check; the exchange simulator should also enforce",
		"rms.go RiskEngine.Check is called only from bench.go:352, never from the FIX acceptor", info("OMS performs these checks; the ME does not duplicate them"))
	h.static(cat, "ST-6", "Order-status / execution-report replay on reconnect", "Missed ERs recoverable (ResendRequest honoured)",
		"in-memory message store only (quickfix.NewMemoryStoreFactory) with ResetOnLogon=Y — nothing survives a reconnect", info("by design for a simulator"))
}

// ---- session-level messages ----------------------------------------------------------------------------

func (h *harness) runSession() {
	cat := "Session messages"
	h.static(cat, "SS-1", "Logon (35=A) with ResetSeqNumFlag 141=Y", "Session established", "initiator sent 141=Y (ResetOnLogon=Y); session reached logged-on state", pass("session established"))

	s2 := h.newSession("TESTBRK2")
	defer s2.ini.Stop()
	{
		sent, rs := h.sendOn(s2, "1", ov{"112": "PING"}, true)
		ok := false
		for _, r := range rs {
			if r.MT == "0" && r.get("112") == "PING" {
				ok = true
			}
		}
		v := fail("no Heartbeat answering the TestRequest")
		if ok {
			v = pass("Heartbeat with TestReqID PING")
		}
		h.record(cat, "SS-2", "TestRequest (35=1, 112=PING)", "Heartbeat (35=0) echoing 112", sent, rs, v)
	}
	{
		for i := 0; i < 3; i++ {
			s := h.sym(100, "MAIN")
			h.sendOn(s2, "D", h.orderFields(s, ov{"44": "95.00"}), false)
		}
		sent, rs := h.sendOn(s2, "2", ov{"7": "1", "16": "0"}, true)
		time.Sleep(400 * time.Millisecond)
		// A resend arrives at the initiator as a PossDup duplicate that QuickFIX drops
		// before the application sees it, so verify on the acceptor's outbound log.
		ok := false
		MsgLog.mu.Lock()
		for _, m := range MsgLog.ring {
			if m.Dir == "OUT" && m.Session == "TESTBRK2" && strings.Contains(m.Raw, "|43=Y|") {
				ok = true
				rs = append(rs, reply{MT: "(log)", F: map[string]string{}, Raw: m.Raw})
			}
		}
		MsgLog.mu.Unlock()
		gapFill := false
		MsgLog.mu.Lock()
		for _, m := range MsgLog.ring {
			if m.Dir == "OUT" && m.Session == "TESTBRK2" && strings.Contains(m.Raw, "|35=4|") && strings.Contains(m.Raw, "|123=Y|") {
				gapFill = true
			}
		}
		MsgLog.mu.Unlock()
		if !gapFill {
			h.t.Errorf("SS-3: messages older than ME_FIX_RESEND_KEEP must be gap-filled (35=4, 123=Y)")
		}
		v := fail("no resend / gap-fill after a ResendRequest")
		if ok {
			v = pass("resend or gap-fill received")
		}
		h.record(cat, "SS-3", "ResendRequest (35=2, 7=1, 16=0)", "Missed messages resent or gap-filled", sent, rs, v)
	}
	{
		sent, rs := h.sendOn(s2, "4", ov{"123": "N", "36": "5000"}, true)
		v := pass("accepted with no reject")
		if hasMT(rs, "3") {
			v = fail("rejected: " + summary(rs))
		}
		h.record(cat, "SS-4", "SequenceReset (35=4, 123=N, 36=5000)", "Accepted; peer's expected sequence moves", sent, rs, v)
	}
	s3 := h.newSession("TESTBRK3")
	defer s3.ini.Stop()
	{
		sent, rs := h.sendOn(s3, "5", ov{"58": "bye"}, true)
		v := fail("no Logout reply")
		if hasMT(rs, "5") {
			v = pass("Logout acknowledged")
		}
		h.record(cat, "SS-5", "Logout (35=5)", "Logout acknowledged, session closed", sent, rs, v)
	}
	h.static(cat, "SS-6", "Heartbeat (35=0) interval 30 s", "Sent automatically", "handled by the QuickFIX session layer (HeartBtInt negotiated at logon)", pass("session layer"))
}
