package simulator

import (
	"net"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"testing"
	"time"

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

// Clean test data cancels every simulator order and removes the simulator's
// rows from the order report, and nothing else; the page counts the old
// engine-only bots' orders so Clean Data can offer to cancel them.
func TestCleanRemovesOnlySimulatorData(t *testing.T) {
	sim, eng, rec := newSim(t)
	for i, actor := range []string{"FIX:SIMBOT1", "FIX:SIMBENCH1", "FIX:REALBRK"} {
		o := eng.NewOrder("RENATA", engine.Buy, engine.Limit, 680+float64(i), 10, actor)
		o.ClOrdID = "CLN-" + strconv.Itoa(i)
		eng.SubmitAndWait(o)
	}
	t.Cleanup(func() {
		eng.CancelAs("CLN-2", "CLN-2-X", "FIX:REALBRK")
		rec.Forget("FIX:REALBRK")
	})
	if left := sim.Status()["bots"].(map[string]any)["leftover"]; left != 1 {
		t.Fatalf("old engine-bot orders counted: %v, want 1", left)
	}
	_, rows := rec.Today()
	sims := 0
	for _, r := range rows {
		if simActor(r.Actor) {
			sims++
		}
	}
	res, err := sim.Clean()
	if err != nil || res.Cancelled < 2 || res.ReportRows != sims || simOrders(eng) != 0 {
		t.Fatalf("clean: %+v %v (want %d rows), %d simulator orders left", res, err, sims, simOrders(eng))
	}
	_, after := rec.Today()
	for _, r := range after {
		if simActor(r.Actor) {
			t.Fatalf("the order report still has a simulator row: %+v", r)
		}
	}
	if len(after) != len(rows)-sims {
		t.Fatalf("Clean removed %d rows that were not the simulator's", len(rows)-sims-len(after))
	}
	eng.Mu.RLock()
	real := eng.FindOpen("CLN-2", "FIX:REALBRK")
	eng.Mu.RUnlock()
	if real == nil {
		t.Fatal("a real broker's order was cancelled")
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
	bench := eng.NewOrder("RENATA", engine.Sell, engine.Limit, 710, 10, "FIX:SIMBENCH1")
	bench.ClOrdID = "LEFT-2"
	eng.SubmitAndWait(bench)
	// At start-up only the benchmark's leftovers go: Stop keeps the market bots' orders.
	if n := sim.SweepBench(); n != 1 || eng.FindOpen("LEFT-1", "FIX:SIMBOT1") == nil {
		t.Fatalf("SweepBench cancelled %d, want only the benchmark order", n)
	}
	if n := sim.Sweep(); n != 1 {
		t.Fatalf("sweep cancelled %d, want 1", n)
	}
	if eng.FindOpen("REAL-1", "FIX:REALBRK") == nil {
		t.Fatal("a real broker's order was cancelled")
	}
}
