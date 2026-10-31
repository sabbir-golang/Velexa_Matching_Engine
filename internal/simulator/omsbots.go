package simulator

// omsbots.go — the market bots: simulated dealers that trade through a
// broker's OMS rather than logging on to the engine directly. They sign in to
// the OMS as a dealer and send orders for that dealer's clients on the OMS's
// own order channel (omsclient.go), so the orders pass the OMS's risk checks,
// reach the engine through the broker's FIX session, and show in the OMS
// blotter, executions, positions and purchase power like any dealer's orders.
// They quote a few ticks around a drifting fair price or take the best price
// (bots.go), so they trade with each other and with anyone else in the book.
// Buys stay under a value cap; sells only use shares the OMS says the client
// holds. Stop leaves their orders in the book and keeps the sign-in, so Clean
// Data (CleanOMS) can cancel them through the OMS later.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"math"
	"math/rand"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gorilla/websocket"

	"velexa-sim-me/internal/database/brokers"
	"velexa-sim-me/internal/engine"
)

// OMSConfig is what the page sends to start the OMS bots.
type OMSConfig struct {
	URL         string   `json:"url"`           // the OMS address traders use, e.g. https://ibl.bdtaskoms.com
	API         string   `json:"api,omitempty"` // the OMS API when it is not https://api.<domain of URL>
	User        string   `json:"user"`          // an OMS dealer login without a one-time code (MFA)
	Password    string   `json:"password"`      // blank: the saved one for this login (omslogin.go)
	Remember    bool     `json:"remember"`      // save the login once the OMS accepts it
	Clients     []string `json:"clients"`       // client codes (CODE or CODE/BO-ID); empty: ClientCount of the dealer's own clients
	ClientCount int      `json:"client_count"`  // clients to trade for when Clients is empty (1–20, default 8)
	Rate        int      `json:"rate"`          // orders per minute, all clients together (6–600)
	Symbols     []string `json:"symbols"`       // empty: Count of the best-known active shares
	Count       int      `json:"count"`         // instruments when Symbols is empty (1–100, default 10)
	MaxValue    float64  `json:"max_value"`     // largest buy in Tk (default 20,000)
}

const (
	maxOMSClients   = 20            // clients one run trades for
	maxOMSWorking   = 6             // resting orders per client before the oldest is cancelled
	omsSignInAgain  = 5 * time.Hour // the OMS access token lives 6 hours
	omsDefaultValue = 20000         // Tk, largest buy by default
)

var errOMSRunning = errors.New("the market bots are already running")

// omsAcct is one client the bots trade for.
type omsAcct struct {
	code, bo, name string

	mu    sync.Mutex
	hold  map[string]int64 // shares the OMS says the client holds, by symbol
	known bool             // the position stream answered, so sells are allowed
}

// omsOrder is one bot order, keyed by every ClOrdID it has had.
type omsOrder struct {
	id      string // current ClOrdID
	acct    *omsAcct
	sym     string
	side    string // BUY | SELL
	px      float64
	qty     int64 // total ordered
	leaves  int64
	orderID string // the exchange's OrderID, from the first report
	live    bool   // an execution report confirmed it
	pend    string // ClOrdID of a cancel or amend in flight
	newPx   float64
	at      time.Time
}

type omsRun struct {
	s       *Sim
	cfg     OMSConfig // without the password
	c       *omsClient
	accts   []*omsAcct
	syms    []*symState
	started time.Time
	run     string
	seq     atomic.Uint64
	rng     *rand.Rand

	wmu   sync.Mutex      // one writer on the order socket at a time
	ws    *websocket.Conn // the dealer's order socket
	wsUp  atomic.Bool
	socks []*websocket.Conn // position sockets

	mu    sync.Mutex
	ids   map[string]*omsOrder // every ClOrdID of a working bot order
	stats counters
	fills tradeIDs

	paused  atomic.Bool
	note    atomic.Value // string: why the bots are idle
	lastTry time.Time    // last reconnect attempt (loop goroutine)
	stop    chan struct{}
	done    chan struct{}
}

// StartOMS signs in to the OMS and starts trading through it. Orders a stopped
// run left in the book are cancelled through the OMS first, so the new bots,
// which do not know them, do not trade on top of them.
func (s *Sim) StartOMS(cfg OMSConfig) error {
	cfg.Rate = clamp(cfg.Rate, 6, 600, 60)
	cfg.Count = clamp(cfg.Count, 1, 100, 10)
	cfg.ClientCount = clamp(cfg.ClientCount, 1, maxOMSClients, 8)
	if cfg.MaxValue <= 0 {
		cfg.MaxValue = omsDefaultValue
	}
	s.op.Lock()
	defer s.op.Unlock()
	s.mu.Lock()
	old := s.oms
	s.mu.Unlock()
	if old != nil && !old.halted() {
		return errOMSRunning
	}
	site, err := parseOMSSite(cfg.URL, cfg.API)
	if err != nil {
		return err
	}
	typed := cfg.Password != ""
	if !typed {
		cfg.Password = savedPassword(site, cfg.User)
	}
	if strings.TrimSpace(cfg.User) == "" || cfg.Password == "" {
		return errors.New("enter the OMS dealer login and its password")
	}
	pw := cfg.Password
	c := newOMSClient(site, cfg.User, pw)
	cfg.Password = ""
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := c.login(ctx); err != nil {
		return err
	}
	if typed && cfg.Remember {
		if err := saveOMSLogin(savedLogin{URL: site.Origin, API: cfg.API, User: strings.TrimSpace(cfg.User), Password: pw}); err != nil {
			log.Printf("[sim] the OMS login could not be saved: %v", err)
		}
	}
	accts, err := omsAccounts(ctx, c, cfg.Clients, cfg.ClientCount)
	if err != nil {
		c.logout()
		return err
	}
	syms := s.pickSymbols(cfg.Symbols, cfg.Count)
	if len(syms) == 0 {
		c.logout()
		return errors.New("no active instrument to trade: check the symbols")
	}
	if old != nil {
		if left := s.releaseOMS(old, false); left > 0 {
			c.logout()
			return fmt.Errorf("%d orders of the last run are still working: the OMS did not confirm their cancels; press Clean Data again, or cancel them in the OMS", left)
		}
	}
	r := &omsRun{s: s, cfg: cfg, c: c, accts: accts, syms: syms, started: time.Now(),
		run: strconv.FormatInt(time.Now().Unix()%46656, 36), rng: rand.New(rand.NewSource(time.Now().UnixNano())),
		ids: map[string]*omsOrder{}, stop: make(chan struct{}), done: make(chan struct{})}
	r.note.Store("")
	if err := r.connect(ctx); err != nil {
		c.logout()
		return err
	}
	r.watchHoldings(ctx)
	s.mu.Lock()
	s.oms = r
	s.mu.Unlock()
	go r.loop()
	log.Printf("[sim] market bots started: %s as %s (dealer %s), %d clients, %d orders/min, %d instruments",
		site.Origin, cfg.User, c.dealerInfo().TraderCode, len(accts), cfg.Rate, len(syms))
	return nil
}

// omsAccounts resolves the clients to trade for: the ones asked for (their BO
// ids from the dealer's client list, or given as CODE/BO-ID), else the first
// count of the dealer's own clients.
func omsAccounts(ctx context.Context, c *omsClient, want []string, count int) ([]*omsAcct, error) {
	list, lerr := c.investors(ctx)
	byCode := map[string]omsInvestor{}
	for _, iv := range list {
		byCode[strings.ToUpper(iv.Code)] = iv
	}
	dealer := c.dealerInfo().TraderCode
	var out []*omsAcct
	add := func(iv omsInvestor) {
		out = append(out, &omsAcct{code: iv.Code, bo: iv.BO, name: iv.Name, hold: map[string]int64{}})
	}
	if len(want) == 0 {
		if lerr != nil {
			return nil, fmt.Errorf("the OMS did not list %s's clients (%v): type the client codes as CODE/BO-ID", dealer, lerr)
		}
		for _, iv := range list {
			if iv.BO != "" && len(out) < count {
				add(iv)
			}
		}
		if len(out) == 0 {
			return nil, fmt.Errorf("%s has no clients in the OMS: type the client codes as CODE/BO-ID", dealer)
		}
		return out, nil
	}
	for _, w := range want {
		code, bo, _ := strings.Cut(strings.TrimSpace(w), "/")
		code, bo = strings.TrimSpace(code), strings.TrimSpace(bo)
		if code == "" {
			continue
		}
		iv, ok := byCode[strings.ToUpper(code)]
		if !ok {
			iv = omsInvestor{Code: code}
		}
		if bo != "" {
			iv.BO = bo
		}
		if iv.BO == "" {
			return nil, fmt.Errorf("client %s is not in %s's client list: type it as %s/<BO ID>", code, dealer, code)
		}
		if len(out) == maxOMSClients {
			return nil, fmt.Errorf("at most %d clients at a time", maxOMSClients)
		}
		add(iv)
	}
	if len(out) == 0 {
		return nil, errors.New("no client codes given")
	}
	return out, nil
}

// dealerInfo is the signed-in dealer.
func (c *omsClient) dealerInfo() omsDealer {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.dealer
}

// connect opens the dealer's order socket and starts reading it.
func (r *omsRun) connect(ctx context.Context) error {
	ws, err := r.c.dial(ctx, "order/"+r.c.dealerInfo().TraderCode)
	if err != nil {
		return fmt.Errorf("the OMS order channel: %v", err)
	}
	r.wmu.Lock()
	r.ws = ws
	r.wmu.Unlock()
	r.wsUp.Store(true)
	go r.readOrders(ws)
	return nil
}

// watchHoldings opens each client's position stream. A client whose stream
// does not open only buys.
func (r *omsRun) watchHoldings(ctx context.Context) {
	for _, a := range r.accts {
		ws, err := r.c.dial(ctx, "position/"+a.bo)
		if err != nil {
			log.Printf("[sim] OMS position stream for %s did not open (%v): it only buys", a.code, err)
			continue
		}
		r.socks = append(r.socks, ws)
		go r.readHoldings(a, ws)
	}
}

func (r *omsRun) readHoldings(a *omsAcct, ws *websocket.Conn) {
	for {
		_, b, err := ws.ReadMessage()
		if err != nil {
			return
		}
		var f struct {
			Type string           `json:"message_type"`
			Data []map[string]any `json:"data"`
		}
		if json.Unmarshal(b, &f) != nil {
			continue
		}
		a.mu.Lock()
		if f.Type == "connection" {
			a.known = true
		}
		for _, row := range f.Data {
			if cc := str(row["client_code"]); cc != "" && !strings.EqualFold(cc, a.code) {
				continue
			}
			if sym := strings.ToUpper(str(row["security_code"])); sym != "" {
				a.hold[sym] = int64(num(row["quantity"]))
			}
		}
		a.mu.Unlock()
	}
}

func (r *omsRun) readOrders(ws *websocket.Conn) {
	defer r.wsUp.Store(false)
	for {
		_, b, err := ws.ReadMessage()
		if err != nil {
			return
		}
		r.onFrame(b)
	}
}

// onFrame applies one message from the OMS order channel. Messages about other
// orders of the dealer (a person trading at the same time) are ignored.
func (r *omsRun) onFrame(b []byte) {
	var f struct {
		Type    string         `json:"message_type"`
		Message string         `json:"message"`
		Data    map[string]any `json:"data"`
	}
	if json.Unmarshal(b, &f) != nil || f.Data == nil {
		return
	}
	switch f.Type {
	case "order_submitted":
		cl := firstStr(f.Data, "client_order_id", "clOrdID")
		r.mu.Lock()
		o := r.ids[cl]
		isNew := o != nil && o.id == cl && !o.live
		r.mu.Unlock()
		if isNew {
			r.stats.Accepted.Add(1)
		}
	case "order_error":
		r.onError(str(f.Data["client_order_id"]), f.Message)
	case "order_report":
		r.onReport(f.Data, f.Message)
	}
}

// onError is the OMS refusing a request before it reached the exchange.
func (r *omsRun) onError(cl, msg string) {
	r.mu.Lock()
	o := r.ids[cl]
	if o == nil {
		r.mu.Unlock()
		return
	}
	newOrder := cl == o.id
	if newOrder {
		r.forget(o)
	} else if o.pend == cl {
		delete(r.ids, cl)
		o.pend = ""
	}
	r.mu.Unlock()
	r.stats.Rejected.Add(1)
	r.s.noteReject("OMS: " + msg)
	if newOrder {
		r.tape(o, "Rejected", o.qty, o.px, msg)
	}
}

// onReport applies one execution report the OMS passed on from the exchange.
func (r *omsRun) onReport(d map[string]any, msg string) {
	cl, orig := str(d["clientOrderId"]), str(d["origClOrdID"])
	r.mu.Lock()
	o := r.ids[cl]
	if o == nil {
		o = r.ids[orig]
	}
	if o == nil {
		r.mu.Unlock()
		return
	}
	if id := str(d["orderId"]); id != "" {
		o.orderID = id
	}
	leaves := int64(num(d["leavesQty"]))
	lastQty, lastPx := int64(num(d["lastQty"])), num(d["lastPx"])
	status, note := "", ""
	// A completed amend comes as execType REPLACED, with orderStatus the
	// order's state now (NEW or PARTIALLY_FILLED), as FIX reports it.
	et := strings.ToUpper(str(d["execType"]))
	switch st := strings.ToUpper(str(d["orderStatus"])); {
	case et == "REPLACED" || st == "REPLACED":
		if o.pend == cl {
			delete(r.ids, o.id)
			o.id, o.pend, o.px, o.leaves = cl, "", o.newPx, leaves
			status = "Amended"
		}
	case lastQty > 0 && (st == "PARTIALLY_FILLED" || st == "FILLED"):
		o.live, o.leaves = true, leaves
		status = "Partly filled"
		if st == "FILLED" || leaves == 0 {
			status = "Filled"
			r.forget(o)
		}
	case st == "NEW" || st == "PENDING" || st == "UNPLACED" || st == "PENDING_NEW":
		if !o.live && cl == o.id {
			o.live, o.leaves = true, leaves
			status = "New"
		}
	case st == "CANCELED" || st == "EXPIRED":
		r.forget(o)
		status = map[string]string{"CANCELED": "Cancelled", "EXPIRED": "Expired"}[st]
	case st == "REJECTED":
		note = firstStr(d, "rejectionReason")
		if note == "" {
			note = msg
		}
		if o.pend == cl { // the exchange refused a cancel or amend; the order stands
			delete(r.ids, cl)
			o.pend = ""
		} else {
			r.forget(o)
			status = "Rejected"
		}
	}
	r.mu.Unlock()
	switch status {
	case "Filled", "Partly filled":
		if id := firstStr(d, "execID", "trdMatchID"); id == "" || r.fills.first(id) {
			r.stats.Trades.Add(1)
		}
		r.tape(o, status, lastQty, lastPx, "")
	case "Cancelled":
		r.stats.Cancelled.Add(1)
		r.tape(o, status, o.leaves, o.px, "")
	case "Rejected", "":
		if note != "" {
			r.stats.Rejected.Add(1)
			r.s.noteReject(note)
		}
		if status != "" {
			r.tape(o, status, o.qty, o.px, note)
		}
	default:
		r.tape(o, status, o.leaves, o.px, "")
	}
}

// forget drops a finished order and all its ClOrdIDs. The caller holds r.mu.
func (r *omsRun) forget(o *omsOrder) {
	for id, x := range r.ids {
		if x == o {
			delete(r.ids, id)
		}
	}
}

func (r *omsRun) tape(o *omsOrder, status string, qty int64, px float64, note string) {
	row := TapeRow{Time: time.Now().In(engine.Dhaka).Format("15:04:05.000"), Bot: "OMS " + o.acct.code, Symbol: o.sym,
		Side: map[string]string{"BUY": "Buy", "SELL": "Sell"}[o.side], Qty: qty, Price: px, Status: status, Note: note}
	r.s.mu.Lock()
	r.s.tape = append(r.s.tape, row)
	if len(r.s.tape) > 120 {
		r.s.tape = append([]TapeRow(nil), r.s.tape[len(r.s.tape)-80:]...)
	}
	r.s.mu.Unlock()
}

func (r *omsRun) loop() {
	defer close(r.done)
	t := time.NewTicker(time.Minute / time.Duration(r.cfg.Rate))
	defer t.Stop()
	for {
		select {
		case <-r.stop:
			return
		case <-t.C:
		}
		if r.paused.Load() {
			r.note.Store("paused: orders stay in the book")
			continue
		}
		if ph := r.s.eng.CurrentPhase(); ph != engine.PhaseOpen {
			r.note.Store("waiting: the market is " + ph + " (the bots trade in Continuous Trading)")
			continue
		}
		if !r.wsUp.Load() {
			r.reconnect()
			continue
		}
		r.note.Store(r.silence())
		r.step()
	}
}

// silence explains a run whose orders the OMS never answers.
func (r *omsRun) silence() string {
	sent := r.stats.Sent.Load()
	if sent >= 10 && r.stats.Accepted.Load()+r.stats.Rejected.Load() == 0 && time.Since(r.started) > 20*time.Second {
		return fmt.Sprintf("the OMS has not answered any of %d bot orders: check that %s may trade and that the OMS is connected to the exchange", sent, r.c.dealerInfo().TraderCode)
	}
	return ""
}

// reconnect opens the order socket again, at most every 5 s, signing in again
// first when the access token is old.
func (r *omsRun) reconnect() {
	if time.Since(r.lastTry) < 5*time.Second {
		return
	}
	r.lastTry = time.Now()
	r.note.Store("reconnecting to the OMS order channel…")
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	if r.c.signedInFor() > omsSignInAgain {
		if err := r.c.login(ctx); err != nil {
			r.note.Store("reconnecting: " + err.Error())
			return
		}
	}
	if err := r.connect(ctx); err != nil {
		r.note.Store("reconnecting: " + err.Error())
	}
}

func (r *omsRun) clOrdID(kind string) string {
	return "SO" + kind + r.run + strconv.FormatUint(r.seq.Add(1), 10) // at most 20 characters, as DSE allows
}

// step sends one request: a cancel when a client has too many orders resting,
// now and then an amend, otherwise a new order.
func (r *omsRun) step() {
	rng := r.rng
	if r.c.signedInFor() > omsSignInAgain { // keep a fresh token for reconnects
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		r.c.login(ctx)
		cancel()
	}
	a := r.accts[rng.Intn(len(r.accts))]
	if mine := r.resting(a); len(mine) >= maxOMSWorking {
		r.cancel(mine[0])
		return
	} else if len(mine) > 0 && rng.Float64() < 0.12 {
		r.amend(mine[rng.Intn(len(mine))])
		return
	}
	st := r.syms[rng.Intn(len(r.syms))]
	prev, bid, ask := r.s.market(st.sym)
	if prev <= 0 {
		return
	}
	st.drift(rng, prev, bid, ask)
	side, fixSide := "BUY", "1"
	free := r.sellable(a, st.sym)
	if free >= st.lot && rng.Intn(2) == 0 {
		side, fixSide = "SELL", "2"
	}
	px, tif := st.price(rng, fixSide, rng.Float64() < 0.45, prev, bid, ask)
	qty := r.size(st, px)
	if side == "SELL" {
		qty = min(qty, free/st.lot*st.lot)
	}
	if qty <= 0 {
		return
	}
	o := &omsOrder{id: r.clOrdID("N"), acct: a, sym: st.sym, side: side, px: px, qty: qty, leaves: qty, at: time.Now()}
	r.mu.Lock()
	r.ids[o.id] = o
	r.mu.Unlock()
	if r.send(r.frame("NEW_ORDER_SINGLE_REQUEST", o, o.id, qty, px, tif)) != nil {
		r.mu.Lock()
		r.forget(o)
		r.mu.Unlock()
	}
}

// size is a buy quantity worth at most the configured value: whole lots, in
// steps of 10 shares where the lot is 1.
func (r *omsRun) size(st *symState, px float64) int64 {
	if px <= 0 {
		return 0
	}
	unit := st.lot
	if unit == 1 && px*10 <= r.cfg.MaxValue {
		unit = 10
	}
	most := int64(r.cfg.MaxValue / (px * float64(unit)))
	if most < 1 {
		return 0
	}
	return unit * (1 + r.rng.Int63n(min(most, 20)))
}

// resting is a client's confirmed resting orders with no request in flight,
// oldest first.
func (r *omsRun) resting(a *omsAcct) []*omsOrder {
	r.mu.Lock()
	defer r.mu.Unlock()
	seen := map[*omsOrder]bool{}
	var out []*omsOrder
	for _, o := range r.ids {
		if o.acct == a && o.live && o.pend == "" && o.orderID != "" && !seen[o] {
			seen[o] = true
			out = append(out, o)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].at.Before(out[j].at) })
	return out
}

// sellable is how many shares of sym the client holds that no bot sell order
// already uses.
func (r *omsRun) sellable(a *omsAcct, sym string) int64 {
	a.mu.Lock()
	held, known := a.hold[sym], a.known
	a.mu.Unlock()
	if !known || held <= 0 {
		return 0
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	seen := map[*omsOrder]bool{}
	for _, o := range r.ids {
		if o.acct == a && o.sym == sym && o.side == "SELL" && !seen[o] {
			seen[o] = true
			held -= o.leaves
		}
	}
	return max(held, 0)
}

func (r *omsRun) cancel(o *omsOrder) {
	cl := r.clOrdID("X")
	d := r.c.dealerInfo()
	r.mu.Lock()
	o.pend = cl
	r.ids[cl] = o
	r.mu.Unlock()
	r.send(map[string]any{"action": "ORDER_CANCEL_REQUEST", "session": "continuous", "trader_id": d.TraderCode,
		"bo_id": o.acct.bo, "client_code": o.acct.code, "user_id": d.UserID, "client_order_id": cl,
		"orig_client_order_id": o.id, "order_id": o.orderID, "symbol": o.sym, "side": o.side})
}

// amend moves a resting order one or two ticks, staying inside the band.
func (r *omsRun) amend(o *omsOrder) {
	var st *symState
	for _, x := range r.syms {
		if x.sym == o.sym {
			st = x
		}
	}
	prev, _, _ := r.s.market(o.sym)
	if st == nil || prev <= 0 {
		return
	}
	lo, hi := st.band(prev)
	px := o.px + float64(1+r.rng.Intn(2))*st.tick*float64(1-2*r.rng.Intn(2))
	px = math.Max(lo, math.Min(hi, onTick(px, st.tick)))
	if px == o.px {
		return
	}
	cl := r.clOrdID("A")
	r.mu.Lock()
	o.pend, o.newPx = cl, px
	r.ids[cl] = o
	r.mu.Unlock()
	f := r.frame("ORDER_CANCEL_REPLACE_REQUEST", o, cl, o.qty, px, "0")
	f["orig_client_order_id"], f["order_id"] = o.id, o.orderID
	r.send(f)
}

// frame is an order request exactly as the OMS front end builds it.
func (r *omsRun) frame(action string, o *omsOrder, cl string, qty int64, px float64, tif string) map[string]any {
	m := engine.MetaFor(o.sym)
	board, group := "MAIN", "PUBLIC"
	if strings.Contains(strings.ToUpper(m.Board), "SME") {
		board, group = "SME", "SPUBLIC"
	}
	d := r.c.dealerInfo()
	return map[string]any{"action": action, "trader_pk": d.TraderPK, "trader_id": d.TraderCode, "bo_id": o.acct.bo,
		"client_code": o.acct.code, "client_order_id": cl, "branch_id": d.BranchID, "user_id": d.UserID,
		"order_type": "LIMIT", "qty": qty, "price": px, "side": o.side, "symbol": o.sym, "isin": m.ISIN,
		"asset_class": engine.AssetClassOf(m), "security_code": o.sym, "order_book_id": r.s.eng.OrderBookID(o.sym),
		"board": board, "session": "continuous", "category": engine.CategoryOf(m.Board), "market_type": "Public",
		"group": group, "timeInForce": tif, "displayQty": nil, "minQty": nil, "execInst": nil}
}

// send writes one request on the order socket.
func (r *omsRun) send(f map[string]any) error {
	b, err := json.Marshal(f)
	if err != nil {
		return err
	}
	r.wmu.Lock()
	defer r.wmu.Unlock()
	if r.ws == nil {
		return errors.New("not connected")
	}
	r.ws.SetWriteDeadline(time.Now().Add(5 * time.Second))
	if err := r.ws.WriteMessage(websocket.TextMessage, b); err != nil {
		r.ws.Close()
		r.wsUp.Store(false)
		return err
	}
	r.stats.Sent.Add(1)
	return nil
}

// PauseOMS stops sending; resting orders stay. ResumeOMS continues.
func (s *Sim) PauseOMS(on bool) error {
	s.mu.Lock()
	r := s.oms
	s.mu.Unlock()
	if r == nil || r.halted() {
		return errors.New("the market bots are not running")
	}
	r.paused.Store(on)
	return nil
}

// StopOMS stops the bots sending. Their orders stay in the book and the bots
// stay signed in to the OMS, following its reports, until CleanOMS, the next
// StartOMS or shutdown.
func (s *Sim) StopOMS() {
	s.op.Lock()
	defer s.op.Unlock()
	s.mu.Lock()
	r := s.oms
	s.mu.Unlock()
	if r == nil || r.halted() {
		return
	}
	r.halt()
	log.Printf("[sim] market bots stopped: %d orders sent, %d fills; %d orders stay in the book", r.stats.Sent.Load(),
		r.stats.Trades.Load(), r.working())
}

// CleanOMS cancels, through the OMS, the orders the stopped bots left in the
// book and signs them out, then cancels in the engine any bot order that no run
// follows any more: orphans (see omsOrphan) and orders the old engine-only
// bots (SIMBOT) left. It returns how many orders it cancelled. When the OMS
// does not confirm every cancel, the bots stay signed in so it can be tried
// again.
func (s *Sim) CleanOMS() (int, error) {
	s.op.Lock()
	defer s.op.Unlock()
	s.mu.Lock()
	r := s.oms
	s.mu.Unlock()
	if r != nil && !r.halted() {
		return 0, errors.New("stop the market bots first")
	}
	n := s.sweep(actorPrefix(brokers.SimBotPrefix))
	if r != nil {
		before := r.working()
		left := s.releaseOMS(r, false)
		n += before - left
		if left > 0 {
			return n, fmt.Errorf("%d orders are still working: the OMS did not confirm their cancels; try Clean Data again, or cancel them in the OMS", left)
		}
	}
	return n + s.cancelOMSOrphans(), nil
}

// omsOrphan reports whether a resting order is an OMS-bot order (its ClOrdID
// is one the bots make: "SO" then N for new or A for amended, see clOrdID)
// that no signed-in run follows: one left by a run that could not cancel it,
// such as an amended order before the bots read amends right.
func (s *Sim) omsOrphan(clOrdID string) bool {
	if !strings.HasPrefix(clOrdID, "SON") && !strings.HasPrefix(clOrdID, "SOA") {
		return false
	}
	s.mu.Lock()
	r := s.oms
	s.mu.Unlock()
	if r == nil {
		return true
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.ids[clOrdID] == nil
}

// omsOrphans counts the orphaned OMS-bot orders in the book.
func (s *Sim) omsOrphans() int {
	return s.eng.CountOpen(func(_, cl string) bool { return s.omsOrphan(cl) })
}

// cancelOMSOrphans cancels the orphaned OMS-bot orders in the engine. Each
// owner gets the cancel report under the order's own ClOrdID, as for an
// exchange cancel, so the OMS marks it cancelled and frees its cash.
func (s *Sim) cancelOMSOrphans() int {
	snap := s.eng.TakeSnapshot(true)
	s.eng.MarkDirty() // the copy above must not keep the market saver from saving
	n := 0
	for _, o := range append(snap.Orders, snap.Stops...) {
		if s.omsOrphan(o.ClOrdID) {
			if rep := s.eng.CancelAs(o.ClOrdID, o.ClOrdID, o.Actor); rep != nil && rep.Status == "CANCELED" {
				n++
			}
		}
	}
	if n > 0 {
		log.Printf("[sim] cancelled %d orphaned OMS-bot order(s) in the engine", n)
	}
	return n
}

// EndOMS signs the bots out at shutdown. Their sign-in cannot outlive the
// engine, so their orders are cancelled through the OMS first (while the FIX
// gateway still runs) rather than left with no one to manage them.
func (s *Sim) EndOMS() {
	s.op.Lock()
	defer s.op.Unlock()
	s.mu.Lock()
	r := s.oms
	s.mu.Unlock()
	if r != nil {
		s.releaseOMS(r, true)
	}
}

// releaseOMS stops the run, cancels its orders through the OMS (signing in
// and reconnecting first if need be) and signs out. Unless force, it keeps the
// run when orders are still working at the deadline. It returns how many are.
// The caller holds s.op.
func (s *Sim) releaseOMS(r *omsRun, force bool) int {
	r.halt()
	if r.working() > 0 && !r.wsUp.Load() {
		r.lastTry = time.Time{}
		r.reconnect()
	}
	left := r.cancelAll(5 * time.Second)
	if left > 0 && !force {
		return left
	}
	r.wmu.Lock()
	if r.ws != nil {
		r.ws.Close()
	}
	r.wmu.Unlock()
	for _, ws := range r.socks {
		ws.Close()
	}
	r.c.logout()
	s.mu.Lock()
	if s.oms == r {
		s.oms = nil
	}
	s.mu.Unlock()
	log.Printf("[sim] market bots signed out: %d orders sent, %d fills, %d left working", r.stats.Sent.Load(), r.stats.Trades.Load(), left)
	return left
}

// halt stops the trading loop and waits for it; calling it again does nothing.
func (r *omsRun) halt() {
	select {
	case <-r.stop:
	default:
		close(r.stop)
	}
	<-r.done
}

// halted reports whether the trading loop has stopped.
func (r *omsRun) halted() bool {
	select {
	case <-r.done:
		return true
	default:
		return false
	}
}

// working counts the bots' orders still working.
func (r *omsRun) working() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	seen := map[*omsOrder]bool{}
	for _, o := range r.ids {
		seen[o] = true
	}
	return len(seen)
}

// cancelAll cancels every resting bot order through the OMS and waits for the
// confirmations; it returns how many orders are still working at the deadline.
func (r *omsRun) cancelAll(wait time.Duration) int {
	deadline := time.Now().Add(wait)
	for {
		r.mu.Lock()
		seen := map[*omsOrder]bool{}
		var todo []*omsOrder
		for _, o := range r.ids {
			if !seen[o] {
				seen[o] = true
				if o.live && o.pend == "" && o.orderID != "" {
					todo = append(todo, o)
				}
			}
		}
		left := len(seen)
		r.mu.Unlock()
		if left == 0 || time.Now().After(deadline) || !r.wsUp.Load() {
			return left
		}
		for _, o := range todo {
			r.cancel(o)
		}
		time.Sleep(200 * time.Millisecond)
	}
}

func (r *omsRun) status() map[string]any {
	if r == nil {
		return map[string]any{"state": "stopped"}
	}
	state := "running"
	switch {
	case r.halted():
		state = "stopped" // signed in, its orders still in the book
	case r.paused.Load():
		state = "paused"
	}
	r.mu.Lock()
	work := map[*omsAcct]int{}
	seen := map[*omsOrder]bool{}
	for _, o := range r.ids {
		if !seen[o] {
			seen[o] = true
			work[o.acct]++
		}
	}
	r.mu.Unlock()
	var accts []map[string]any
	for _, a := range r.accts {
		a.mu.Lock()
		held := 0
		for _, q := range a.hold {
			if q > 0 {
				held++
			}
		}
		accts = append(accts, map[string]any{"code": a.code, "bo": a.bo, "name": a.name, "working": work[a],
			"holdings": held, "sells": a.known})
		a.mu.Unlock()
	}
	syms := make([]string, 0, len(r.syms))
	for _, st := range r.syms {
		syms = append(syms, st.sym)
	}
	d := r.c.dealerInfo()
	note, _ := r.note.Load().(string)
	el := time.Since(r.started).Seconds()
	return map[string]any{"state": state, "note": note, "site": r.c.site.Origin, "tenant": r.c.site.Tenant, "env": r.c.site.Env,
		"user": r.c.user, "dealer": d.TraderCode, "dealer_name": d.Name, "connected": r.wsUp.Load(), "clients": accts,
		"symbols": syms, "working": len(seen), "stats": statsMap(&r.stats), "running_for": math.Round(el),
		"orders_per_min": math.Round(float64(r.stats.Sent.Load())/math.Max(el, 1)*600) / 10,
		"config":         map[string]any{"rate": r.cfg.Rate, "count": r.cfg.Count, "max_value": r.cfg.MaxValue, "symbols": r.cfg.Symbols}}
}
