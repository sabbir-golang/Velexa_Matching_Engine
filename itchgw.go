package main

import (
	"fmt"
	"encoding/json"
	"net/http"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

// ItchGateway translates the sim engine's live state into the trader SPA's
// DSE-ITCH-compatible JSON protocol over three WebSocket streams:
//   /market       — compact-key instrument rows (ob,sn,s,lp,o,h,l,c,bd,ak,...)
//   /marketdepth  — per-orderbook 10-level depth ({ob,[ob]:{buy,sell}})
//   /index        — DSEX broad index, keyed by sn
// It reads engine state under the same RLock the HTTP snapshot endpoints use.

var itchUpgrader = websocket.Upgrader{
	CheckOrigin:     func(r *http.Request) bool { return true },
	ReadBufferSize:  1024,
	WriteBufferSize: 16384,
}

type wsHub struct {
	mu      sync.Mutex
	clients map[*websocket.Conn]chan []byte
}

func newWsHub() *wsHub { return &wsHub{clients: map[*websocket.Conn]chan []byte{}} }

func (h *wsHub) add(c *websocket.Conn) chan []byte {
	ch := make(chan []byte, 512)
	h.mu.Lock()
	h.clients[c] = ch
	h.mu.Unlock()
	return ch
}
func (h *wsHub) remove(c *websocket.Conn) {
	h.mu.Lock()
	if ch, ok := h.clients[c]; ok {
		close(ch)
		delete(h.clients, c)
	}
	h.mu.Unlock()
}
func (h *wsHub) broadcast(msg []byte) {
	h.mu.Lock()
	for _, ch := range h.clients {
		select {
		case ch <- msg:
		default:
		}
	}
	h.mu.Unlock()
}
func (h *wsHub) count() int { h.mu.Lock(); defer h.mu.Unlock(); return len(h.clients) }

type ItchGateway struct {
	eng    *Engine
	market      *wsHub
	depth       *wsHub
	index       *wsHub
	events      *wsHub
	lastTradeID uint64
}

func NewItchGateway(eng *Engine) *ItchGateway {
	return &ItchGateway{eng: eng, market: newWsHub(), depth: newWsHub(), index: newWsHub(), events: newWsHub()}
}

func statusCode(md *MarketData) string {
	switch {
	case md.Volume == 0:
		return "nt"
	case md.Last > md.Prev:
		return "u"
	case md.Last < md.Prev:
		return "d"
	default:
		return "uc"
	}
}

// buildFrames snapshots every stream under a single RLock and returns marshaled
// frames: one market row per instrument, one depth msg per instrument, one index.
// spaBoardFor maps the exchange board code (from the instrument master) to the
// board label the trader SPA's board filter expects. Without this every symbol
// was broadcast as "Main Board", so the SME/ATB filters matched nothing.
func spaBoardFor(symbol string) string {
	switch metaFor(symbol).Board {
	case "SME":
		return "Small Cap Board"
	case "ATB":
		return "Alternative Trading Board"
	default:
		return "Main Board"
	}
}

func (g *ItchGateway) buildFrames() (market [][]byte, depth [][]byte, index []byte) {
	e := g.eng
	e.mu.RLock()
	defer e.mu.RUnlock()
	var totVol, totTrades int64
	var totVal float64
	var up, dn, unch, nott int
	for _, in := range e.instr() {
		md := e.mdOf(in.Symbol)
		totVol += md.Volume
		totTrades += md.Trades
		totVal += float64(md.Volume) * md.Last
		switch {
		case md.Volume == 0:
			nott++
		case md.Last > md.Prev:
			up++
		case md.Last < md.Prev:
			dn++
		default:
			unch++
		}
		ob := e.orderBookID(in.Symbol)
		row, _ := json.Marshal(map[string]any{
			"ob": ob, "sn": in.Symbol, "fn": in.Name, "fln": in.Symbol, "ic": metaFor(in.Symbol).ISIN,
			"b": spaBoardFor(in.Symbol), "g": "PUBLIC", "sc": "General", "it": "EQ", "mt": "N", "d": md.Change,
			"s": statusCode(md), "r": md.Prev, "c": md.Last, "o": md.Open,
			"h": md.High, "l": md.Low, "lp": md.Last, "v": md.Volume,
			"bd": md.Bid, "ak": md.Ask, "bq": md.BidQty, "aq": md.AskQty,
			"cg": md.ChangePct, "tds": md.Trades, "ltt": time.Now().Unix(),
		})
		market = append(market, row)

		buy, sell := []map[string]any{}, []map[string]any{}
		if b := e.book(in.Symbol); b != nil {
			cum := int64(0)
			for _, lv := range b.depthSide(true, 10) {
				cum += lv.Qty
				// Trader SPA depth contract (marketDepthContext BuySellType):
				// p=price, q=level qty, o=order count, cq=cumulative qty.
				buy = append(buy, map[string]any{"o": lv.Orders, "p": lv.Price, "q": lv.Qty, "cq": cum})
			}
			cum = 0
			for _, lv := range b.depthSide(false, 10) {
				cum += lv.Qty
				sell = append(sell, map[string]any{"o": lv.Orders, "p": lv.Price, "q": lv.Qty, "cq": cum})
			}
		}
		// SPA reads response[ob].buy / .sell (see marketDepthContext.onmessage).
		dm, _ := json.Marshal(map[string]any{"ob": ob, fmt.Sprintf("%d", ob): map[string]any{"buy": buy, "sell": sell}})
		depth = append(depth, dm)
	}
	ip := 0.0
	if e.indexPrev > 0 {
		ip = round2((e.index - e.indexPrev) / e.indexPrev * 100)
	}
	// Exact key contract read by the SPA index ticker (BottomHeader.tsx /
	// IndexDataStruct): civ=value, ivd=change points, ip=change %, plus market
	// breadth. These are read via .toFixed() with no guard, so they MUST exist.
	index, _ = json.Marshal(map[string]any{
		"sn":  "DSEX",
		"civ": e.index,
		"ivd": round2(e.index - e.indexPrev),
		"ip":  ip,
		"v":   totVol,
		"vl":  round2(totVal),
		"tds": totTrades,
		"nt":  nott,
		"uc":  unch,
		"d":   dn,
		"u":   up,
	})
	return
}

func (g *ItchGateway) run() {
	tick := time.NewTicker(500 * time.Millisecond)
	defer tick.Stop()
	for range tick.C {
		if g.market.count() == 0 && g.depth.count() == 0 && g.index.count() == 0 {
			continue
		}
		market, depth, index := g.buildFrames()
		if g.market.count() > 0 {
			for _, m := range market {
				g.market.broadcast(m)
			}
		}
		if g.depth.count() > 0 {
			for _, m := range depth {
				g.depth.broadcast(m)
			}
		}
		if g.index.count() > 0 {
			g.index.broadcast(index)
		}
		if g.events.count() > 0 {
			for _, f := range g.newTsmFrames() {
				g.events.broadcast(f)
			}
		} else {
			g.advanceTradeCursor()
		}
	}
}

// serve upgrades a connection, registers it, streams broadcasts, and sends an
// immediate snapshot on connect (which stream via the selector).
func (g *ItchGateway) serve(h *wsHub, selector func(m, d [][]byte, i []byte) [][]byte) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		c, err := itchUpgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		ch := h.add(c)
		go func() {
			for msg := range ch {
				c.SetWriteDeadline(time.Now().Add(10 * time.Second))
				if err := c.WriteMessage(websocket.TextMessage, msg); err != nil {
					break
				}
			}
			c.Close()
		}()
		// initial snapshot so a fresh client is populated instantly
		m, d, i := g.buildFrames()
		for _, frame := range selector(m, d, i) {
			select {
			case ch <- frame:
			default:
			}
		}
		go func() {
			defer h.remove(c)
			c.SetReadLimit(4096)
			for {
				if _, _, err := c.ReadMessage(); err != nil {
					return
				}
			}
		}()
	}
}

func selMarket(m, d [][]byte, i []byte) [][]byte { return m }
func selDepth(m, d [][]byte, i []byte) [][]byte  { return d }
func selIndex(m, d [][]byte, i []byte) [][]byte  { return [][]byte{i} }

// ── /events (news socket): time-&-sales tape (tp:tsm) + news (tp:news) ──

func tsmItem(t Trade) map[string]any {
	side := "B"
	if t.Aggressor == Sell {
		side = "S"
	}
	return map[string]any{
		"p": t.Price, "v": t.Qty, "tst": t.Time.Format("15:04:05"),
		"s": side, "tsm_id": t.ID,
	}
}

// advanceTradeCursor moves the cursor to the newest trade without emitting —
// keeps a later first-client connect from replaying the whole ring as singles.
func (g *ItchGateway) advanceTradeCursor() {
	e := g.eng
	e.mu.RLock()
	if n := len(e.trades); n > 0 {
		g.lastTradeID = e.trades[n-1].ID
	}
	e.mu.RUnlock()
}

// newTsmFrames emits one tsm frame per trade newer than the cursor.
func (g *ItchGateway) newTsmFrames() [][]byte {
	e := g.eng
	e.mu.RLock()
	defer e.mu.RUnlock()
	out := [][]byte{}
	for _, t := range e.trades {
		if t.ID <= g.lastTradeID {
			continue
		}
		f, _ := json.Marshal(map[string]any{"tp": "tsm", "ob": g.eng.orderBookID(t.Symbol), "d": tsmItem(t)})
		out = append(out, f)
		g.lastTradeID = t.ID
	}
	return out
}

// eventsSnapshot sends each instrument's recent tape (newest first) + empty news.
func (g *ItchGateway) eventsSnapshot() [][]byte {
	e := g.eng
	e.mu.RLock()
	defer e.mu.RUnlock()
	bySym := map[string][]map[string]any{}
	for _, t := range e.trades {
		bySym[t.Symbol] = append([]map[string]any{tsmItem(t)}, bySym[t.Symbol]...) // prepend => newest first
	}
	out := [][]byte{}
	nf, _ := json.Marshal(map[string]any{"tp": "news", "d": simNews()})
	out = append(out, nf)
	for sym, items := range bySym {
		f, _ := json.Marshal(map[string]any{"tp": "tsm", "ob": g.eng.orderBookID(sym), "d": items})
		out = append(out, f)
	}
	return out
}

// simNews returns DSE-style news items in the trader panel's news shape
// (compact keys: dt,t,ob,nid,fid,tl,rf,nt via newsContext keyMap).
func simNews() []map[string]any {
	items := []struct {
		ob    int
		title string
	}{
		{100001, "Grameenphone Ltd. — Board meeting on 28 Aug to consider Q2 dividend"},
		{100002, "Square Pharmaceuticals declares 55% cash dividend for FY2026"},
		{100003, "Beximco Ltd. resumes trading after brief circuit-breaker halt"},
		{100005, "British American Tobacco BD reports 12% YoY revenue growth"},
		{100009, "BRAC Bank Ltd. — credit rating reaffirmed AAA by CRAB"},
		{100016, "United Power Generation commissions new 150MW plant"},
		{100007, "Renata Ltd. announces record date for rights issue"},
		{0, "DSEX crosses 6,100 as turnover tops BDT 5.5 billion"},
	}
	dt := time.Now().Format("2006-01-02")
	tm := time.Now().Format("15:04:05")
	out := make([]map[string]any, 0, len(items))
	for i, it := range items {
		out = append(out, map[string]any{
			"dt": dt, "t": tm, "ob": it.ob, "nid": 90000 + i,
			"fid": "DSE", "tl": it.title, "rf": "", "nt": it.title + ".",
		})
	}
	return out
}

func (g *ItchGateway) serveEvents(w http.ResponseWriter, r *http.Request) {
	c, err := itchUpgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}
	ch := g.events.add(c)
	go func() {
		for msg := range ch {
			c.SetWriteDeadline(time.Now().Add(10 * time.Second))
			if err := c.WriteMessage(websocket.TextMessage, msg); err != nil {
				break
			}
		}
		c.Close()
	}()
	for _, frame := range g.eventsSnapshot() {
		select {
		case ch <- frame:
		default:
		}
	}
	go func() {
		defer g.events.remove(c)
		c.SetReadLimit(4096)
		for {
			if _, _, err := c.ReadMessage(); err != nil {
				return
			}
		}
	}()
}
