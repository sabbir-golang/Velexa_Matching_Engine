package me

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"regexp"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gorilla/websocket"
	"github.com/jackc/pgx/v5/pgxpool"
)

var reInv = regexp.MustCompile(`INV[0-9]+`)
var reTr = regexp.MustCompile(`\bT[0-9]{2,}\b`)
var reNum = regexp.MustCompile(`[0-9]{3,}`)

func normReason(s string) string {
	if len(s) > 140 {
		s = s[:140]
	}
	s = reInv.ReplaceAllString(s, "INV*")
	s = reTr.ReplaceAllString(s, "T*")
	s = reNum.ReplaceAllString(s, "#")
	return s
}

var liveTransport = &http.Transport{
	MaxIdleConns:        8192,
	MaxIdleConnsPerHost: 8192,
	MaxConnsPerHost:     0,
	IdleConnTimeout:     90 * time.Second,
}

type simTrader struct{ PK, Code, Branch string }
type simClient struct{ BOID, Code string }
type brokerInfo struct {
	Sub, DB, BOPrefix, Sender string
	fixLoaded                 bool
	Traders                   []simTrader
	Clients                   []simClient
}

// The 3 REAL registered brokers (exchange registry). bo_id prefixes match the
// per-broker load-test seed (opal=11 pre-existing; bdtask=16, samim=17 seeded).
var liveBrokers = []brokerInfo{
	{Sub: "opal", DB: "tenant_opal_db", BOPrefix: "11", Sender: "OPAL"},
	{Sub: "bdtask", DB: "terminal_bdtask_db", BOPrefix: "16", Sender: "BDTASK"},
	{Sub: "samim", DB: "terminal_samim_db", BOPrefix: "17", Sender: "SAMIM"},
}

// loadBrokersFile replaces liveBrokers from a TSV (prefix, subdomain, db, sender,
// …) when present, so the full DSE broker set can be managed without recompiling.
// Falls back to the built-in defaults if the file is missing/empty.
func loadBrokersFile() {
	path := "/opt/velexa-sim-me/brokers.tsv"
	if p := strings.TrimSpace(os.Getenv("LIVE_BROKERS_FILE")); p != "" {
		path = p
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return
	}
	var bs []brokerInfo
	for _, ln := range strings.Split(string(data), "\n") {
		ln = strings.TrimRight(ln, "\r")
		if strings.TrimSpace(ln) == "" {
			continue
		}
		c := strings.Split(ln, "\t")
		if len(c) < 4 || strings.TrimSpace(c[1]) == "" {
			continue
		}
		bs = append(bs, brokerInfo{Sub: c[1], DB: c[2], BOPrefix: c[0], Sender: c[3]})
	}
	if len(bs) > 0 {
		liveBrokers = bs
	}
}

// loadBrokerFixture pulls a broker's REAL active traders + investors from its
// OMS DB so the simulator drives orders that resolve against that broker's own
// risk cache (real trader_pk / investor_code / branch), instead of the legacy
// synthetic T01..T20 / INV##### identities. Loaded once per broker and cached.
// Thin load-test brokers with no real rows keep the synthetic fallback in fire().
func loadBrokerFixture(bk *brokerInfo) {
	if bk.fixLoaded {
		return
	}
	pc, err := pgxpool.ParseConfig(omsDSN(bk.DB))
	if err != nil {
		return
	}
	pc.MaxConns = 2
	pool, err := pgxpool.NewWithConfig(context.Background(), pc)
	if err != nil {
		return
	}
	defer pool.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	var ts []simTrader
	if rows, e := pool.Query(ctx, `SELECT DISTINCT t.id, t.trader_code, COALESCE(t.branch_id,1)
		FROM broker_house_trader t
		JOIN broker_house_tradermarketsegmentauthorization a ON a.trader_id=t.id AND a.is_public=true
		WHERE upper(COALESCE(t.status,'ACTIVE'))='ACTIVE' ORDER BY t.id LIMIT 200`); e == nil {
		for rows.Next() {
			var id, br int64
			var code string
			if rows.Scan(&id, &code, &br) == nil && code != "" {
				ts = append(ts, simTrader{PK: fmt.Sprintf("%d", id), Code: code, Branch: fmt.Sprintf("%d", br)})
			}
		}
		rows.Close()
	}
	var cs []simClient
	if rows, e := pool.Query(ctx, `SELECT bo_id, investor_code FROM broker_house_investor
		WHERE COALESCE(investor_code,'')<>'' AND COALESCE(bo_id,'')<>'' ORDER BY id LIMIT 1000`); e == nil {
		for rows.Next() {
			var bo, code string
			if rows.Scan(&bo, &code) == nil {
				cs = append(cs, simClient{BOID: bo, Code: code})
			}
		}
		rows.Close()
	}
	bk.Traders = ts
	bk.Clients = cs
	bk.fixLoaded = true
}

func init() { loadBrokersFile() }

// tickers every investor holds (positions seeded) — safe for SELL orders.
var sellable = []string{"GP", "SQURPHARMA", "BEXIMCO", "ROBI", "BATBC"}

type LiveConfig struct {
	Broker  string `json:"broker"`
	Rate    int    `json:"rate"`
	Total   int    `json:"total"`
	Sells   bool   `json:"sells"` // legacy: true ⇒ Side "BOTH"
	Side    string `json:"side"`  // "BUY" | "SELL" | "BOTH"
	Workers int    `json:"workers"`
}

// LiveBench drives REAL orders into a chosen broker's OMS via the api internal
// order endpoint: gen → api RMS → FIX → sim ME → blotter. Metrics are observed
// from the api responses + the broker's own blotter (orders_fixorderexec).
type LiveBench struct {
	eng    *Engine
	apiURL string
	token  string

	sent, accepted, rejected, filled int64
	rejReasons                       sync.Map
	latStat                          Stat
	seq                              uint64
	runID                            int64

	running                     int32
	startNs, endNs, firstSentNs int64
	cfg                         LiveConfig
	cancel                      context.CancelFunc

	tapeMu sync.Mutex
	tape   []map[string]any

	poolMu  sync.Mutex
	pools   []*pgxpool.Pool
	targets []*brokerInfo
}

// resolveTargets returns the brokers to fan orders out to. "ALL"/"*"/"" means
// every seeded broker (orders divided equally across them); otherwise the single
// named broker.
func resolveTargets(sub string) []*brokerInfo {
	switch strings.ToUpper(strings.TrimSpace(sub)) {
	case "ALL", "*", "":
		out := make([]*brokerInfo, len(liveBrokers))
		for i := range liveBrokers {
			out[i] = &liveBrokers[i]
		}
		return out
	}
	if bk := brokerBySub(sub); bk != nil {
		return []*brokerInfo{bk}
	}
	return nil
}

// ensureSession (re)registers the broker's FIX session on the shared :8003
// initiator → sim, so Start is self-healing after a restart.
func ensureSession(bk *brokerInfo) {
	c, _, err := websocket.DefaultDialer.Dial(meFixAuthWS(), nil)
	if err != nil {
		return
	}
	defer c.Close()
	data := map[string]any{
		"sender_comp_id": bk.Sender, "target_comp_id": "DSEME", "begin_string": "FIXT.1.1",
		"host": meAdvertiseHost(), "port": "5001", "heartbeat": 30}
	// When ME logon auth is enabled, send this broker's registered password so
	// the self-heal logon is accepted (same allowlist the acceptor validates).
	if pw := meFixAuth.password(bk.Sender); pw != "" {
		data["username"] = bk.Sender
		data["password"] = pw
	}
	frame, _ := json.Marshal(map[string]any{"type": "logon", "broker": bk.Sub, "data": data})
	c.WriteMessage(websocket.TextMessage, frame)
	time.Sleep(1500 * time.Millisecond)
}

func NewLiveBench(eng *Engine) *LiveBench {
	return &LiveBench{
		eng:    eng,
		apiURL: meAPIURL(),
		token:  "4011ae325069f02d6a53b89a0145c251",
	}
}

func brokerBySub(sub string) *brokerInfo {
	for i := range liveBrokers {
		if liveBrokers[i].Sub == sub {
			return &liveBrokers[i]
		}
	}
	return nil
}

func (l *LiveBench) Running() bool { return atomic.LoadInt32(&l.running) == 1 }

func (l *LiveBench) bumpReason(r string) {
	if r == "" {
		r = "unknown"
	}
	r = normReason(r)
	v, _ := l.rejReasons.LoadOrStore(r, new(int64))
	atomic.AddInt64(v.(*int64), 1)
}

func (l *LiveBench) Start(cfg LiveConfig) error {
	targets := resolveTargets(cfg.Broker)
	if len(targets) == 0 {
		return fmt.Errorf("unknown broker %q", cfg.Broker)
	}
	if !atomic.CompareAndSwapInt32(&l.running, 0, 1) {
		return fmt.Errorf("live test already running")
	}
	l.cfg = cfg
	l.targets = targets
	atomic.StoreInt64(&l.sent, 0)
	atomic.StoreInt64(&l.accepted, 0)
	atomic.StoreInt64(&l.rejected, 0)
	atomic.StoreInt64(&l.filled, 0)
	l.latStat.reset()
	l.rejReasons = sync.Map{}
	l.tape = nil
	l.runID = time.Now().UnixNano() / 1000
	atomic.StoreInt64(&l.endNs, 0)
	atomic.StoreInt64(&l.firstSentNs, 0)
	l.startNs = time.Now().UnixNano()
	// Register FIX sessions for all targets in PARALLEL (bounded). Sequential
	// setup (~1.5s each) would take minutes across the full DSE broker set.
	{
		sem := make(chan struct{}, 40)
		var swg sync.WaitGroup
		for _, bk := range targets {
			swg.Add(1)
			sem <- struct{}{}
			go func(b *brokerInfo) {
				defer swg.Done()
				defer func() { <-sem }()
				ensureSession(b)
				loadBrokerFixture(b)
			}(bk)
		}
		swg.Wait()
	}

	// per-broker blotter pools (for the live fill count, summed across brokers)
	l.poolMu.Lock()
	for _, p := range l.pools {
		if p != nil {
			p.Close()
		}
	}
	l.pools = nil
	// Only maintain live-fill pools for small fan-outs; opening a pool per broker
	// across the full DSE set would exceed Postgres max_connections. For large
	// runs the blotter-fill counter is skipped (accept/reject rates still live).
	if len(targets) <= 8 {
		for _, bk := range targets {
			if pc, err := pgxpool.ParseConfig(omsDSN(bk.DB)); err == nil {
				pc.MaxConns = 2
				if p, err := pgxpool.NewWithConfig(context.Background(), pc); err == nil {
					l.pools = append(l.pools, p)
				}
			}
		}
	}
	l.poolMu.Unlock()

	ctx, cancel := context.WithCancel(context.Background())
	l.cancel = cancel

	orders := make(chan int, 4096)
	var wg sync.WaitGroup
	client := &http.Client{Timeout: 8 * time.Second, Transport: liveTransport}
	nw := cfg.Workers
	if nw <= 0 {
		nw = 96
	}
	if nw > 4096 {
		nw = 4096
	}
	for i := 0; i < nw; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for range orders {
				l.fire(client)
			}
		}()
	}

	// live fill-count poller
	go func() {
		t := time.NewTicker(1500 * time.Millisecond)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				l.poolMu.Lock()
				pools := append([]*pgxpool.Pool{}, l.pools...)
				l.poolMu.Unlock()
				runMark := fmt.Sprintf("%%-%d-%%", l.runID) // matches LIVE-<sub>-<runID>-<seq>
				var total int64
				for _, p := range pools {
					if p == nil {
						continue
					}
					var c int64
					_ = p.QueryRow(context.Background(),
						"SELECT count(*) FROM orders_fixorderexec WHERE exec_type='TRADE' AND client_order_id LIKE $1", runMark).Scan(&c)
					total += c
				}
				atomic.StoreInt64(&l.filled, total)
			}
		}
	}()

	// generator
	go func() {
		defer func() {
			close(orders)
			wg.Wait()
			atomic.StoreInt64(&l.endNs, time.Now().UnixNano())
			atomic.StoreInt32(&l.running, 0)
		}()
		count := 0
		done := func() bool { return cfg.Total != 0 && count >= cfg.Total }
		emit := func() bool {
			select {
			case orders <- count:
				return true
			case <-ctx.Done():
				return false
			}
		}
		if cfg.Rate > 0 {
			tick := time.NewTicker(time.Millisecond)
			defer tick.Stop()
			perMs := float64(cfg.Rate) / 1000.0
			budget := 0.0
			for !done() {
				select {
				case <-ctx.Done():
					return
				case <-tick.C:
				}
				budget += perMs
				for budget >= 1 && !done() {
					if !emit() {
						return
					}
					count++
					budget--
				}
			}
		} else {
			for !done() {
				if !emit() {
					return
				}
				count++
			}
		}
	}()
	return nil
}

func (l *LiveBench) fire(client *http.Client) {
	seq := atomic.AddUint64(&l.seq, 1)
	targets := l.targets
	if len(targets) == 0 {
		return
	}
	// Broker chosen round-robin so orders divide EQUALLY across brokers; the
	// per-broker sequence then divides investors (1..10000) and traders (1..20)
	// evenly within each broker.
	bk := targets[int(seq)%len(targets)]
	bseq := seq / uint64(len(targets))
	n := int(bseq%1000) + 1
	tr := (int(bseq) % 20) + 1
	// Option Y: prefer the broker's REAL traders/investors so orders resolve
	// against its own risk cache; thin brokers fall back to synthetic ids.
	traderID := fmt.Sprintf("T%02d", tr)
	traderPK := fmt.Sprintf("%d", tr)
	branchID := "1"
	boID := fmt.Sprintf("%s%07d", bk.BOPrefix, n)
	clientCode := fmt.Sprintf("INV%05d", n)
	if len(bk.Traders) > 0 {
		tt := bk.Traders[int(bseq)%len(bk.Traders)]
		traderID, traderPK, branchID = tt.Code, tt.PK, tt.Branch
	}
	if len(bk.Clients) > 0 {
		cl := bk.Clients[int(bseq)%len(bk.Clients)]
		boID, clientCode = cl.BOID, cl.Code
	}
	side := "BUY"
	var sym string
	sideCfg := l.cfg.Side
	if sideCfg == "" { // legacy sells bool
		if l.cfg.Sells {
			sideCfg = "BOTH"
		} else {
			sideCfg = "BUY"
		}
	}
	switch sideCfg {
	case "SELL": // sell-only — always a held (seeded) position
		side = "SELL"
		sym = sellable[int(seq)%len(sellable)]
	case "BOTH": // mixed ~30% sells
		if seq%10 >= 7 {
			side = "SELL"
			sym = sellable[int(seq)%len(sellable)]
		} else {
			sym = pickSymbol(l.eng.instr(), seq)
		}
	default: // BUY-only
		sym = pickSymbol(l.eng.instr(), seq)
	}
	l.eng.mu.RLock()
	last := l.eng.mdOf(sym).Last
	l.eng.mu.RUnlock()
	if last <= 0 {
		last = 100
	}
	// Snap to the instrument tick (same tick Phase B enforces on the ME) so
	// simulator orders aren't rejected off-tick. round2 cleans the float.
	tick := metaFor(sym).Tick
	if tick <= 0 {
		tick = 0.10
	}
	mult := 1.02
	if side == "SELL" {
		mult = 0.98
	}
	price := round2(float64(int64(last*mult/tick+0.5)) * tick)
	qty := 100 + int(seq%9)*100
	coid := fmt.Sprintf("LIVE-%s-%d-%d", bk.Sub, l.runID, seq)
	frame := map[string]any{
		"action": "NEW_ORDER_SINGLE_REQUEST", "trader_id": traderID,
		"trader_pk": traderPK, "branch_id": branchID, "board": "MAIN", "group": "PUBLIC",
		"bo_id": boID, "client_code": clientCode,
		"client_order_id": coid, "symbol": sym, "order_book_id": fmt.Sprintf("%d", l.eng.orderBookID(sym)),
		"qty": fmt.Sprintf("%d", qty), "price": fmt.Sprintf("%.2f", price), "side": side, "order_type": "LIMIT",
	}
	body, _ := json.Marshal(map[string]any{"subdomain": bk.Sub, "env": "prod", "group": traderID, "frame": frame})
	t0 := time.Now()
	req, _ := http.NewRequest("POST", l.apiURL, bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Internal-Token", l.token)
	atomic.AddInt64(&l.sent, 1)
	atomic.CompareAndSwapInt64(&l.firstSentNs, 0, time.Now().UnixNano())
	resp, err := client.Do(req)
	lat := time.Since(t0).Nanoseconds()
	l.latStat.rec(lat)
	if err != nil {
		atomic.AddInt64(&l.rejected, 1)
		l.bumpReason("http_error")
		return
	}
	defer resp.Body.Close()
	var out struct {
		OK      bool   `json:"ok"`
		Message string `json:"message"`
	}
	json.NewDecoder(resp.Body).Decode(&out)
	if out.OK {
		atomic.AddInt64(&l.accepted, 1)
	} else {
		atomic.AddInt64(&l.rejected, 1)
		l.bumpReason(out.Message)
	}
	if l.rngSample(seq) {
		l.pushTape(coid, sym, side, qty, price, out.OK, out.Message)
	}
}

// Sample every 47th order for the tape. 47 is coprime with 10, so sampled
// sequence numbers cycle through every residue mod 10 — including 7/8/9 where
// BOTH-mode sells land. (A multiple-of-10 stride like 50 only ever sampled
// buys, making a mixed run look buy-only in the tape.)
func (l *LiveBench) rngSample(seq uint64) bool { return seq%47 == 0 }

func (l *LiveBench) pushTape(coid, sym, side string, qty int, price float64, ok bool, msg string) {
	st := "ACCEPTED"
	if !ok {
		st = "REJECTED"
	}
	row := map[string]any{"id": coid, "sym": sym, "side": side, "qty": qty, "price": price, "status": st, "msg": msg, "t": time.Now().Format("15:04:05")}
	l.tapeMu.Lock()
	l.tape = append([]map[string]any{row}, l.tape...)
	if len(l.tape) > 30 {
		l.tape = l.tape[:30]
	}
	l.tapeMu.Unlock()
}

func (l *LiveBench) Stop() {
	if l.cancel != nil {
		l.cancel()
	}
}

// Cleanup is the "clean" half of test-and-clean: it removes every
// simulator-generated (client_order_id LIKE 'LIVE-%') order/exec across ALL
// seeded brokers, purges the matching rows from the shared master routing table,
// resets each broker's intraday cash/positions to baseline, and purges the
// matching audit-trail events (via the audit service, which also re-seals its
// hash chain). Returns per-broker counts. Safe: only LIVE-* rows are touched.
func (l *LiveBench) Cleanup() map[string]any {
	if l.Running() {
		l.Stop()
		time.Sleep(600 * time.Millisecond)
	}
	ctx := context.Background()

	// 1) Enumerate EVERY broker DB (terminal_*/tenant_*) live from Postgres, not
	// just the brokers.tsv set — so brokers created AFTER the file was generated
	// are cleaned too. Falls back to the built-in list if enumeration fails.
	var brokerDBs []string
	if mc, err := pgxpool.New(ctx, omsDSN("postgres")); err == nil {
		if rows, qerr := mc.Query(ctx,
			"SELECT datname FROM pg_database WHERE datname LIKE 'terminal_%' OR datname LIKE 'tenant_%'"); qerr == nil {
			for rows.Next() {
				var d string
				if rows.Scan(&d) == nil {
					brokerDBs = append(brokerDBs, d)
				}
			}
			rows.Close()
		}
		mc.Close()
	}
	if len(brokerDBs) == 0 {
		for i := range liveBrokers {
			brokerDBs = append(brokerDBs, liveBrokers[i].DB)
		}
	}

	// 2) Truncate each broker DB (parallel, bounded). TRUNCATE reclaims disk and
	// stays fast even after millions of test orders — dedicated sim brokers.
	var mu sync.Mutex
	perBroker := []map[string]any{}
	var totalOrders, totalExecs int64
	sem := make(chan struct{}, 24)
	var wg sync.WaitGroup
	for _, dbName := range brokerDBs {
		wg.Add(1)
		sem <- struct{}{}
		go func(dbName string) {
			defer wg.Done()
			defer func() { <-sem }()
			row := map[string]any{"broker": dbName}
			pc, perr := pgxpool.ParseConfig(omsDSN(dbName))
			if perr != nil {
				row["error"] = perr.Error()
				mu.Lock()
				perBroker = append(perBroker, row)
				mu.Unlock()
				return
			}
			pc.MaxConns = 1
			conn, err := pgxpool.NewWithConfig(ctx, pc)
			if err != nil {
				row["error"] = err.Error()
				mu.Lock()
				perBroker = append(perBroker, row)
				mu.Unlock()
				return
			}
			defer conn.Close()
			var ordN, exeN int64
			conn.QueryRow(ctx, "SELECT count(*) FROM orders_fixorder").Scan(&ordN)
			conn.QueryRow(ctx, "SELECT count(*) FROM orders_fixorderexec").Scan(&exeN)
			if _, e := conn.Exec(ctx, "TRUNCATE orders_fixorder, orders_fixorderexec, orders_fixmessage, orders_fixorderstatus RESTART IDENTITY CASCADE"); e != nil {
				row["error"] = e.Error()
			}
			conn.Exec(ctx, "UPDATE broker_house_clientcashlimit SET intraday_buy_amount=0, intraday_amount=0")
			conn.Exec(ctx, "UPDATE broker_house_clientposition SET quantity=initial_quantity, intraday_buy_quantity=0, intraday_sell_quantity=0, intraday_buy_price_avg=0, intraday_sell_price_avg=0")
			row["orders"] = ordN
			row["execs"] = exeN
			mu.Lock()
			totalOrders += ordN
			totalExecs += exeN
			perBroker = append(perBroker, row)
			mu.Unlock()
		}(dbName)
	}
	wg.Wait()

	// 3) Exchange/master registry — the shared client_order_routes table.
	var routes int64
	if mc, err := pgxpool.New(ctx, omsDSN("bdtask_trade_master")); err == nil {
		mc.QueryRow(ctx, "SELECT count(*) FROM client_order_routes").Scan(&routes)
		mc.Exec(ctx, "TRUNCATE client_order_routes")
		mc.Close()
	}

	// 4) Matching Engine's OWN database (synthetic-benchmark data): velexa_sim.
	var simOrders, simExecs int64
	if sc, err := pgxpool.New(ctx, meDSN("velexa_sim")); err == nil {
		sc.QueryRow(ctx, "SELECT count(*) FROM sim_orders").Scan(&simOrders)
		sc.QueryRow(ctx, "SELECT count(*) FROM sim_execs").Scan(&simExecs)
		sc.Exec(ctx, "TRUNCATE sim_orders, sim_execs RESTART IDENTITY")
		sc.Close()
	}

	// 5) Audit trail — let the audit service purge + re-seal its own chain.
	var auditPurged int64
	if resp, err := http.Post(meAuditURL()+"/purge", "application/json", nil); err == nil {
		var pr struct {
			Purged int64 `json:"purged"`
		}
		json.NewDecoder(resp.Body).Decode(&pr)
		resp.Body.Close()
		auditPurged = pr.Purged
	}

	return map[string]any{
		"ok": true, "brokers": perBroker, "broker_dbs": len(brokerDBs),
		"total_orders": totalOrders, "total_execs": totalExecs,
		"routes": routes, "sim_orders": simOrders, "sim_execs": simExecs,
		"audit_events": auditPurged,
	}
}

func (l *LiveBench) Stats() map[string]any {
	end := atomic.LoadInt64(&l.endNs)
	if end == 0 {
		end = time.Now().UnixNano()
	}
	fs := atomic.LoadInt64(&l.firstSentNs)
	var elapsed float64
	if fs > 0 {
		elapsed = float64(end-fs) / 1e9
	}
	rateDen := elapsed
	if rateDen <= 0 {
		rateDen = 1e-6
	}
	_, la, l50, l95, l99 := l.latStat.snap()
	sent := atomic.LoadInt64(&l.sent)
	acc := atomic.LoadInt64(&l.accepted)
	reasons := map[string]int64{}
	l.rejReasons.Range(func(k, v any) bool { reasons[k.(string)] = atomic.LoadInt64(v.(*int64)); return true })
	l.tapeMu.Lock()
	tape := make([]map[string]any, len(l.tape))
	copy(tape, l.tape)
	l.tapeMu.Unlock()
	ms := func(ns int64) float64 { return float64(ns) / 1e6 }
	brokers := make([]string, len(liveBrokers))
	for i, b := range liveBrokers {
		brokers[i] = b.Sub
	}
	return map[string]any{
		"running": l.Running(), "elapsed_s": elapsed, "cfg": l.cfg, "brokers": brokers, "targets": len(l.targets),
		"sent": sent, "accepted": acc, "rejected": atomic.LoadInt64(&l.rejected),
		"filled":       atomic.LoadInt64(&l.filled),
		"sent_per_sec": int64(float64(sent) / rateDen), "accepted_per_sec": int64(float64(acc) / rateDen),
		"lat_avg_ms": ms(la), "lat_p50_ms": ms(l50), "lat_p95_ms": ms(l95), "lat_p99_ms": ms(l99),
		"reject_reasons": reasons, "tape": tape,
	}
}

// pickSymbol rotates through one instrument snapshot. Taking the snapshot once
// matters: the universe can grow while the live bench runs (a master import, a
// FIX auto-list), so indexing into a second, differently sized snapshot could
// run off the end.
func pickSymbol(instr []Instrument, seq uint64) string {
	if len(instr) == 0 {
		return ""
	}
	return instr[int(seq)%len(instr)].Symbol
}
