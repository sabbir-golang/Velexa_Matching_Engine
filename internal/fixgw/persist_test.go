package fixgw

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"velexa-sim-me/internal/engine"
)

// A restart and a broker that is away must not cost the broker a report.

func persistEnv(t *testing.T) {
	t.Helper()
	wd := repoRoot(t)
	if _, err := os.Stat(filepath.Join(wd, "spec", "FIX50SP2.xml")); err != nil {
		t.Skipf("FIX dictionary not found: %v", err)
	}
	dir := t.TempDir()
	t.Setenv("ME_FIX_AUTH", "off")
	t.Setenv("ME_FIX_DD", "on")
	t.Setenv("ME_FIX_SPEC_DIR", filepath.Join(wd, "spec"))
	t.Setenv("ME_AUTO_LIST", "off")
	t.Setenv("ME_FIX_MSGLOG_FILE", filepath.Join(dir, "fixmsg.log"))
}

var persistInstr = []engine.Instrument{{Symbol: "GP", Name: "Grameenphone", Prev: 370}}

var acceptors int

// startEngine starts an engine (restoring snap, if any) and an acceptor in
// front of it. Each acceptor has its own CompID: quickfix keeps one session
// registry per process, and the old acceptors are left running (its
// Acceptor.Stop races on the static session).
func startEngine(t *testing.T, snap *engine.Snapshot) (*engine.Engine, *Acceptor, *harness) {
	t.Helper()
	eng := engine.NewEngine(persistInstr, engine.NewBus())
	if snap != nil {
		b, _ := json.Marshal(snap) // as saved and read back
		var back engine.Snapshot
		if err := json.Unmarshal(b, &back); err != nil {
			t.Fatal(err)
		}
		if _, err := eng.Restore(&back); err != nil {
			t.Fatal(err)
		}
	}
	go eng.Run()
	port := freePort(t)
	acceptors++
	target := "DSEME" + strconv.Itoa(acceptors)
	fa, err := Start(eng, strconv.Itoa(port), target)
	if err != nil {
		t.Fatal(err)
	}
	time.Sleep(200 * time.Millisecond)
	return eng, fa, &harness{t: t, eng: eng, port: port, target: target, prev: map[string]float64{"GP": 370}}
}

func sell(eng *engine.Engine, qty int64) {
	eng.SubmitAndWait(eng.NewOrder("GP", engine.Sell, engine.Limit, 370, qty, "USER:alice"))
}

func fills(rs []reply) []reply {
	var out []reply
	for _, r := range rs {
		if r.MT == "8" && r.get("150") == "F" {
			out = append(out, r)
		}
	}
	return out
}

func waitFor(t *testing.T, what string, ok func() bool) {
	t.Helper()
	for i := 0; i < 100; i++ {
		if ok() {
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatalf("timed out waiting: %s", what)
}

// An order resting when the engine restarts still reports its fills to its
// broker, with the same OrderID and the order's own fields.
func TestRestoredOrderReportsToItsBroker(t *testing.T) {
	persistEnv(t)
	eng1, fa1, h1 := startEngine(t, nil)
	h1.sess = h1.newSession("PERSA")
	_, rs, cl := h1.order("GP", ov{"38": "100"})
	if !accepted(rs) {
		t.Fatalf("order not accepted: %s", summary(rs))
	}
	oid := apps(rs)[0].get("37")
	snap := eng1.TakeSnapshot(true)
	h1.sess.ini.Stop()
	waitFor(t, "PERSA logged off", func() bool { _, on := fa1.ActiveSessions()["PERSA"]; return !on })

	_, fa2, h2 := startEngine(t, snap)
	if n := fa2.AttachRestored(); n != 1 {
		t.Fatalf("re-attached %d orders, want 1", n)
	}
	h2.sess = h2.newSession("PERSA")
	defer h2.sess.ini.Stop()
	sell(h2.eng, 40)
	got := fills(h2.sess.collect(3*time.Second, 300*time.Millisecond, false))
	if len(got) != 1 {
		t.Fatalf("fill reports after the restart: %d, want 1", len(got))
	}
	f := got[0]
	if f.get("37") != oid || f.get("11") != cl || f.get("32") != "40" || f.get("151") != "60" || f.get("1") != "BO1001" ||
		f.get("448") != "CLIENT01" || f.get("762") != "PUBLIC" || f.get("57") != "TRADER1" {
		t.Fatalf("fill after the restart: %v (OrderID before %s)", f.F, oid)
	}
}

// Fills while the broker is logged off wait for it — also over a restart —
// and arrive in order when it logs on again.
func TestReportsWaitForALoggedOffBroker(t *testing.T) {
	persistEnv(t)
	eng1, fa1, h1 := startEngine(t, nil)
	h1.sess = h1.newSession("PERSB")
	_, rs, cl := h1.order("GP", ov{"38": "100"})
	if !accepted(rs) {
		t.Fatalf("order not accepted: %s", summary(rs))
	}
	oid := apps(rs)[0].get("37")
	h1.sess.ini.Stop()
	waitFor(t, "PERSB logged off", func() bool { _, on := fa1.ActiveSessions()["PERSB"]; return !on })

	sell(eng1, 30) // the broker is away
	waitFor(t, "the fill queued", func() bool { return fa1.Queued()["PERSB"] == 1 })
	box, changed := fa1.OutboxState()
	if !changed {
		t.Fatal("the outbox did not report a change")
	}
	snap := eng1.TakeSnapshot(true)

	_, fa2, h2 := startEngine(t, snap) // the restart, still without the broker
	fa2.AttachRestored()
	if n, err := fa2.RestoreOutbox(box); n != 1 || err != nil {
		t.Fatalf("restored %d queued reports (%v), want 1", n, err)
	}
	sell(h2.eng, 20)
	waitFor(t, "the second fill queued", func() bool { return fa2.Queued()["PERSB"] == 2 })

	h2.sess = h2.newSession("PERSB")
	defer h2.sess.ini.Stop()
	got := fills(h2.sess.collect(3*time.Second, 300*time.Millisecond, false))
	if len(got) != 2 {
		t.Fatalf("reports at logon: %d fills, want 2", len(got))
	}
	if got[0].get("32") != "30" || got[1].get("32") != "20" || got[0].get("14") != "30" || got[1].get("14") != "50" {
		t.Fatalf("fills out of order: %v / %v", got[0].F, got[1].F)
	}
	for _, f := range got {
		if f.get("37") != oid || f.get("11") != cl {
			t.Fatalf("queued fill for the wrong order: %v (want 37=%s 11=%s)", f.F, oid, cl)
		}
	}
	if a, b := got[0].get("880"), got[1].get("880"); a == "" || a == b {
		t.Fatalf("trade numbers %q, %q", a, b)
	}
	waitFor(t, "the outbox emptied", func() bool { return fa2.Queued()["PERSB"] == 0 })

	// Live again: the next fill goes straight out.
	drain(h2.sess)
	sell(h2.eng, 10)
	if got := fills(h2.sess.collect(3*time.Second, 300*time.Millisecond, false)); len(got) != 1 || got[0].get("14") != "60" {
		t.Fatalf("fill after the queue was sent: %d", len(got))
	}
}
