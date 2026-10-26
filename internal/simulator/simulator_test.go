package simulator

import (
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"velexa-sim-me/internal/database/brokers"
	"velexa-sim-me/internal/database/orderlog"
	"velexa-sim-me/internal/engine"
	"velexa-sim-me/internal/fixgw"
)

func repoRoot(t *testing.T) string {
	t.Helper()
	dir, _ := os.Getwd()
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("go.mod not found")
		}
		dir = parent
	}
}

// One engine and FIX gateway serve every test: quickfix keeps its sessions in a
// registry shared by the whole process, so a second gateway cannot start.
var (
	envOnce sync.Once
	envEng  *engine.Engine
	envRec  *orderlog.Recorder
	envAddr string
	envErr  string
)

// newSim returns a simulator wired to the shared engine and its real FIX
// gateway (logon checks on, one real broker registered).
func newSim(t *testing.T) (*Sim, *engine.Engine, *orderlog.Recorder) {
	t.Helper()
	envOnce.Do(func() {
		wd := repoRoot(t)
		if _, err := os.Stat(filepath.Join(wd, "spec", "FIX50SP2.xml")); err != nil {
			envErr = "FIX dictionary not found: " + err.Error()
			return
		}
		dir, err := os.MkdirTemp("", "simtest")
		if err != nil {
			envErr = err.Error()
			return
		}
		for k, v := range map[string]string{"ME_FIX_AUTH": "on", "ME_FIX_AUTH_FILE": filepath.Join(dir, "fix_brokers.json"),
			"ME_FIX_SPEC_DIR": filepath.Join(wd, "spec"), "ME_FIX_MSGLOG_FILE": filepath.Join(dir, "fixmsg.log"),
			"ME_AUTO_LIST": "off", "ME_STATE_DIR": dir} {
			os.Setenv(k, v)
		}
		os.WriteFile(filepath.Join(dir, "fix_brokers.json"), []byte(`{"REALBRK":"Real-Pass1"}`), 0o600)
		envRec, _ = orderlog.NewRecorder(orderlog.NewFileStore(dir))
		envEng = engine.NewEngine([]engine.Instrument{{Symbol: "GP", Name: "Grameenphone", Prev: 331.2},
			{Symbol: "BATBC", Name: "BAT Bangladesh", Prev: 512}, {Symbol: "RENATA", Name: "Renata", Prev: 700}}, engine.NewBus())
		envEng.OnOrderEvent = envRec.Observe
		go envEng.Run()
		envEng.SetPhase(engine.PhaseOpen)
		l, _ := net.Listen("tcp", "127.0.0.1:0")
		port := l.Addr().(*net.TCPAddr).Port
		l.Close()
		if _, err := fixgw.Start(envEng, strconv.Itoa(port), "DSEME"); err != nil {
			envErr = "FIX gateway: " + err.Error()
			return
		}
		envAddr = "127.0.0.1:" + strconv.Itoa(port)
		time.Sleep(200 * time.Millisecond)
	})
	if envErr != "" {
		t.Skip(envErr)
	}
	return New(envEng, envAddr, "DSEME", "FIXT.1.1", envRec), envEng, envRec
}

func simOrders(eng *engine.Engine) int {
	s := eng.TakeSnapshot(true)
	n := 0
	for _, o := range s.Orders {
		if simActor(o.Actor) {
			n++
		}
	}
	return n
}

func stat(m map[string]any, k string) int64 { return m["stats"].(map[string]int64)[k] }

func TestBotsTradeThroughFIXAndCleanUp(t *testing.T) {
	sim, eng, rec := newSim(t)
	if err := sim.Start(BotConfig{Rate: 300, Bots: 3, Symbols: []string{"GP", "BATBC"}}); err != nil {
		t.Fatalf("start: %v", err)
	}
	if brokers.Fix.Enforced() == false || len(brokers.Fix.List()) != 1 {
		t.Fatalf("bot logons must not be saved or listed as brokers: %v", brokers.Fix.List())
	}
	if err := sim.Start(BotConfig{}); err == nil {
		t.Fatal("a second start must be refused")
	}
	deadline := time.Now().Add(8 * time.Second)
	var st map[string]any
	for time.Now().Before(deadline) {
		st = sim.Status()["bots"].(map[string]any)
		if stat(st, "trades") >= 5 {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	if stat(st, "trades") < 5 || stat(st, "accepted") == 0 {
		t.Fatalf("the bots should trade: %v", st["stats"])
	}
	if stat(st, "rejected") != 0 {
		t.Fatalf("bot orders were rejected: %v", sim.Status()["rejects"])
	}
	if eng.MdOf("GP").Volume+eng.MdOf("BATBC").Volume == 0 {
		t.Fatal("bot trades must reach the market data")
	}
	sim.StopBots()
	if n := simOrders(eng); n != 0 {
		t.Fatalf("%d bot orders left in the book after Stop", n)
	}
	if s := sim.Status()["bots"].(map[string]any); s["state"] != "stopped" {
		t.Fatalf("state after stop: %v", s["state"])
	}

	_, rows := rec.Today()
	bots := 0
	for _, r := range rows {
		if strings.HasPrefix(r.Actor, "FIX:SIMBOT") {
			bots++
		}
	}
	if bots == 0 {
		t.Fatal("bot orders must be in the order report")
	}
	res, err := sim.Clean()
	if err != nil || res.ReportRows != bots {
		t.Fatalf("clean: %+v %v (want %d rows)", res, err, bots)
	}
	if _, rows := rec.Today(); len(rows) != 0 {
		t.Fatalf("the order report still has %d rows", len(rows))
	}
}

func TestBenchmarkMeasuresTradesAndLatency(t *testing.T) {
	sim, eng, _ := newSim(t)
	if err := sim.StartBench(BenchConfig{Mode: "trade", Rate: 0, Seconds: 1, Symbol: "GP"}); err != nil {
		t.Fatalf("start: %v", err)
	}
	wait := func() map[string]any {
		end := time.Now().Add(15 * time.Second)
		for time.Now().Before(end) {
			b := sim.Status()["bench"].(map[string]any)
			if b["state"] != "running" {
				return b
			}
			time.Sleep(100 * time.Millisecond)
		}
		t.Fatal("the benchmark did not finish")
		return nil
	}
	b := wait()
	if stat(b, "trades")*2 != stat(b, "sent") {
		t.Fatalf("every buy+sell pair should trade: %v", b["stats"])
	}
	ack := b["latency"].(map[string]any)["ack"].(map[string]any)
	fill := b["latency"].(map[string]any)["fill"].(map[string]any)
	if b["state"] != "done" || stat(b, "trades") == 0 || ack["n"].(int) == 0 || fill["n"].(int) == 0 {
		t.Fatalf("trade benchmark: %v ack %v fill %v", b["stats"], ack, fill)
	}
	if stat(b, "rejected") != 0 {
		t.Fatalf("benchmark orders were rejected: %v", sim.Status()["rejects"])
	}
	if n := simOrders(eng); n != 0 {
		t.Fatalf("%d benchmark orders left in the book", n)
	}

	if err := sim.StartBench(BenchConfig{Mode: "orders", Rate: 400, Seconds: 1, Symbol: "BATBC"}); err != nil {
		t.Fatalf("start: %v", err)
	}
	b = wait()
	if sent := stat(b, "sent"); sent < 300 || sent > 500 || stat(b, "trades") != 0 || stat(b, "accepted") != sent {
		t.Fatalf("orders benchmark at 400/s for 1 s: %v", b["stats"])
	}
	if err := sim.StartBench(BenchConfig{Symbol: "NOSUCH"}); err == nil {
		t.Fatal("an unknown symbol must be refused")
	}
}

func TestSweepCancelsLeftoverBotOrders(t *testing.T) {
	sim, eng, _ := newSim(t)
	o := eng.NewOrder("RENATA", engine.Buy, engine.Limit, 690, 10, "FIX:SIMBOT1")
	o.ClOrdID = "LEFT-1"
	eng.SubmitAndWait(o)
	keep := eng.NewOrder("RENATA", engine.Buy, engine.Limit, 689, 10, "FIX:REALBRK")
	keep.ClOrdID = "REAL-1"
	eng.SubmitAndWait(keep)
	if n := sim.Sweep(); n != 1 {
		t.Fatalf("sweep cancelled %d, want 1", n)
	}
	if eng.FindOpen("REAL-1", "FIX:REALBRK") == nil {
		t.Fatal("a real broker's order was cancelled")
	}
}
