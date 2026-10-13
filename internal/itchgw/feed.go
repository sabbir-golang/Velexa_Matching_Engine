package itchgw

// itchfeed.go — native DSE-ITCH market-data feed emitted by the simulator
// matching engine. It is a SoupBinTCP *server*: the C++ `itch` service connects
// as a SoupBinTCP client, logs in, and receives a stream of SequencedData
// packets whose bodies are concatenated ITCH messages. The byte layouts here
// are the EXACT layouts the C++ decoders read (src/ITCH/market/decoders.cpp +
// utility/messages.h + decoder_helpers.h ITCH_LENGTH):
//
//   SoupBinTCP outer frame : [2-byte BE length][1-byte packet type][body(len-1)]
//     server→client : 'A' LoginAccept, 'H' ServerHeartbeat, 'S' SequencedData
//     client→server : 'L' LoginRequest, 'R' ClientHeartbeat, 'O' Logout
//
//   ITCH message           : [1-byte type][fixed payload], all ints big-endian
//     'S' system event(17) 'T' seconds(4) 'L' price-tick(16) 'P' company(68)
//     'R' orderbook dir(170) 'A' add order(33) 'E' order executed(28)
//     'D' order delete(12)
//
// Strategy (matches the deliverable's "simplest robust" approach): on connect
// send system-event + seconds + one 'R' directory (+ 'P'/'L') per sim
// instrument + a reference-price 'A' (order 0 / qty 0). Then every tick: emit
// an add+full-execute pair for each NEW engine trade (drives last price /
// volume / high-low), and re-publish each book's depth by DELETING the prior
// synthetic add-orders and re-ADDING the current depth levels — keeping the
// C++ order book sane with correct top-of-book + 10-level depth.

import (
	"encoding/binary"
	"io"
	"log"
	"net"
	"os"
	"strings"
	"sync"
	"time"

	"velexa-sim-me/internal/database/brokers"
	"velexa-sim-me/internal/engine"
)

type itchFeed struct {
	eng *engine.Engine
}

func NewFeed(eng *engine.Engine) *itchFeed {
	return &itchFeed{eng: eng}
}

func (f *itchFeed) Run() {
	port := os.Getenv("ITCH_PORT")
	if port == "" {
		port = "5501"
	}
	ln, err := net.Listen("tcp", ":"+port)
	if err != nil {
		log.Printf("[ITCH] listen :%s failed: %v", port, err)
		return
	}
	log.Printf("[ITCH] SoupBinTCP market feed listening on :%s (%d instruments)", port, len(f.eng.Instr()))
	for {
		c, err := ln.Accept()
		if err != nil {
			continue
		}
		go f.session(c)
	}
}

// ---- SoupBinTCP session ----

type itchSession struct {
	f         *itchFeed
	c         net.Conn
	wmu       sync.Mutex
	stop      chan struct{}
	closeOnce sync.Once

	nextRef   uint64
	matchSeq  uint64
	lastTrade uint64
	depthRefs map[int][]uint64 // orderbook id -> synthetic add-order refs currently resting
}

func (f *itchFeed) session(c net.Conn) {
	peer := c.RemoteAddr().String()
	if ok, probe := f.readLogin(c); !ok {
		// A connection that closes without sending a byte is a port health check
		// (the admin dashboard probes this port every few seconds) — not worth a log line.
		if !probe {
			log.Printf("[ITCH] %s: login read failed / not a LoginRequest", peer)
		}
		c.Close()
		return
	}
	if err := f.writeSoup(c, 'A', loginAcceptBody()); err != nil {
		c.Close()
		return
	}
	log.Printf("[ITCH] %s: LoginAccept sent (session=SIM0000001 seq=1)", peer)

	s := &itchSession{f: f, c: c, stop: make(chan struct{}), nextRef: 1, depthRefs: map[int][]uint64{}}
	if err := s.sendInitial(); err != nil {
		s.close()
		return
	}
	go s.heartbeatLoop()
	go s.readerLoop()

	tick := time.NewTicker(700 * time.Millisecond)
	defer tick.Stop()
	for {
		select {
		case <-s.stop:
			return
		case <-tick.C:
			if err := s.refresh(); err != nil {
				s.close()
				return
			}
		}
	}
}

func (s *itchSession) close() {
	s.closeOnce.Do(func() {
		close(s.stop)
		s.c.Close()
	})
}

// readLogin consumes one SoupBinTCP LoginRequest ('L': user6+pass10+session10+
// seq20). Any credentials are accepted (this is a simulator feed).
func (f *itchFeed) readLogin(c net.Conn) (ok, probe bool) {
	hdr := make([]byte, 3)
	if n, err := io.ReadFull(c, hdr); err != nil {
		return false, n == 0 && err == io.EOF
	}
	length := int(binary.BigEndian.Uint16(hdr[0:2]))
	ptype := hdr[2]
	var body []byte
	if length > 1 {
		body = make([]byte, length-1)
		if _, err := io.ReadFull(c, body); err != nil {
			return false, false
		}
	}
	// When ITCH feed auth is enabled, validate the SoupBinTCP LoginRequest's
	// Username (bytes 0–6) + Password (bytes 6–16) against the broker allowlist.
	if brokers.Itch.Enabled && len(body) >= 16 {
		user := strings.TrimSpace(string(body[0:6]))
		pass := strings.TrimSpace(string(body[6:16]))
		if reason := brokers.Itch.Authorize(user, pass); reason != "" {
			log.Printf("[ITCH] %s: login REJECTED: %s", c.RemoteAddr(), reason)
			return false, false
		}
	}
	// Simulator feed: accept ANY well-framed first packet as the login (some
	// SoupBin clients send a slightly different login type). Log non-'L' types.
	if ptype != 'L' {
		log.Printf("[ITCH] %s: accepting non-'L' login packet type=%q len=%d", c.RemoteAddr(), ptype, length)
	}
	return true, false
}

// loginAcceptBody: session[10] + sequence[20] (right-aligned), space-padded —
// exactly what handle_login_accept() reads (10 + 20 bytes, trailing-trimmed).
func loginAcceptBody() []byte {
	body := make([]byte, 30)
	for i := range body {
		body[i] = ' '
	}
	copy(body[0:10], "SIM0000001")
	seq := "1"
	copy(body[10+20-len(seq):30], seq)
	return body
}

func (f *itchFeed) writeSoup(c net.Conn, ptype byte, body []byte) error {
	length := len(body) + 1
	buf := make([]byte, 3+len(body))
	binary.BigEndian.PutUint16(buf[0:2], uint16(length))
	buf[2] = ptype
	copy(buf[3:], body)
	_, err := c.Write(buf)
	return err
}

func (s *itchSession) writePkt(ptype byte, body []byte) error {
	s.wmu.Lock()
	defer s.wmu.Unlock()
	return s.f.writeSoup(s.c, ptype, body)
}

func (s *itchSession) sendSeq(payload []byte) error { return s.writePkt('S', payload) }

func (s *itchSession) heartbeatLoop() {
	t := time.NewTicker(5 * time.Second)
	defer t.Stop()
	for {
		select {
		case <-s.stop:
			return
		case <-t.C:
			if err := s.writePkt('H', nil); err != nil {
				s.close()
				return
			}
		}
	}
}

// readerLoop drains client ClientHeartbeat ('R') / Logout ('O') frames.
func (s *itchSession) readerLoop() {
	hdr := make([]byte, 3)
	for {
		if _, err := io.ReadFull(s.c, hdr); err != nil {
			s.close()
			return
		}
		length := int(hdr[0])<<8 | int(hdr[1])
		ptype := hdr[2]
		if length > 1 {
			body := make([]byte, length-1)
			if _, err := io.ReadFull(s.c, body); err != nil {
				s.close()
				return
			}
		}
		if ptype == 'O' {
			log.Printf("[ITCH] %s: client Logout", s.c.RemoteAddr())
			s.close()
			return
		}
	}
}

func (s *itchSession) nextMatch() uint64 { s.matchSeq++; return s.matchSeq }

// sendInitial streams the start-of-day directory: system-event(O) + seconds +
// price-tick + per-instrument company('P') / orderbook('R') / reference-price('A').
func (s *itchSession) sendInitial() error {
	batch := []byte{}
	batch = append(batch, msgSystemEvent('O')...) // start_message
	batch = append(batch, msgSeconds(secOfDay())...)
	batch = append(batch, msgPriceTick()...)

	e := s.f.eng
	type inst struct {
		ob        uint32
		sym, name string
		prev      float64
	}
	var insts []inst
	e.Mu.RLock()
	for _, in := range e.Instr() {
		insts = append(insts, inst{uint32(e.OrderBookID(in.Symbol)), in.Symbol, in.Name, in.Prev})
	}
	e.Mu.RUnlock()

	for _, it := range insts {
		batch = append(batch, msgCompanyDir(it.ob, it.name, it.sym)...)
		batch = append(batch, msgOrderbookDir(it.ob, it.sym, it.name)...)
		batch = append(batch, msgAddOrder(0, 'B', 0, it.ob, rawPx(it.prev))...) // reference price
		if len(batch) > 8000 {
			if err := s.sendSeq(batch); err != nil {
				return err
			}
			batch = batch[:0]
		}
	}
	if len(batch) > 0 {
		return s.sendSeq(batch)
	}
	return nil
}

// refresh emits one tick's worth of updates: new trades (add+execute) then a
// full depth re-publish (delete prior synthetic orders, add current levels).
func (s *itchSession) refresh() error {
	e := s.f.eng

	type lvl struct {
		price float64
		qty   int64
	}
	type obSnap struct {
		ob         uint32
		bids, asks []lvl
	}
	type trd struct {
		ob    uint32
		verb  byte
		qty   uint64
		price float64
	}
	var snaps []obSnap
	var trades []trd

	e.Mu.RLock()
	for _, in := range e.Instr() {
		ob := uint32(e.OrderBookID(in.Symbol))
		os := obSnap{ob: ob}
		if b := e.Book(in.Symbol); b != nil {
			for _, l := range b.DepthSide(true, 10) {
				os.bids = append(os.bids, lvl{l.Price, l.Qty})
			}
			for _, l := range b.DepthSide(false, 10) {
				os.asks = append(os.asks, lvl{l.Price, l.Qty})
			}
		}
		snaps = append(snaps, os)
	}
	for _, t := range e.Trades {
		if t.ID <= s.lastTrade {
			continue
		}
		verb := byte('B')
		if t.Aggressor == engine.Sell {
			verb = 'S'
		}
		trades = append(trades, trd{uint32(s.f.eng.OrderBookID(t.Symbol)), verb, uint64(t.Qty), t.Price})
		s.lastTrade = t.ID
	}
	e.Mu.RUnlock()

	batch := []byte{}
	batch = append(batch, msgSeconds(secOfDay())...)

	flush := func() error {
		if len(batch) > 8000 {
			if err := s.sendSeq(batch); err != nil {
				return err
			}
			batch = batch[:0]
		}
		return nil
	}

	// New trades: add a marketable order then fully execute it → registers last
	// price / volume / trade count / high-low without leaving resting depth.
	for _, t := range trades {
		ref := s.nextRef
		s.nextRef++
		batch = append(batch, msgAddOrder(ref, t.verb, t.qty, t.ob, rawPx(t.price))...)
		batch = append(batch, msgOrderExec(ref, t.qty, s.nextMatch())...)
		if err := flush(); err != nil {
			return err
		}
	}

	// Depth re-publish: delete last cycle's synthetic orders, add current levels.
	for _, os := range snaps {
		for _, ref := range s.depthRefs[int(os.ob)] {
			batch = append(batch, msgOrderDelete(ref)...)
		}
		var refs []uint64
		for _, l := range os.bids {
			if l.qty <= 0 {
				continue
			}
			ref := s.nextRef
			s.nextRef++
			batch = append(batch, msgAddOrder(ref, 'B', uint64(l.qty), os.ob, rawPx(l.price))...)
			refs = append(refs, ref)
		}
		for _, l := range os.asks {
			if l.qty <= 0 {
				continue
			}
			ref := s.nextRef
			s.nextRef++
			batch = append(batch, msgAddOrder(ref, 'S', uint64(l.qty), os.ob, rawPx(l.price))...)
			refs = append(refs, ref)
		}
		s.depthRefs[int(os.ob)] = refs
		if err := flush(); err != nil {
			return err
		}
	}
	if len(batch) > 0 {
		return s.sendSeq(batch)
	}
	return nil
}

// ---- ITCH message builders (big-endian; each returns type byte + payload) ----

type ibuf struct{ b []byte }

func (x *ibuf) u8(v byte) { x.b = append(x.b, v) }

func (x *ibuf) u32(v uint32) {
	x.b = append(x.b, byte(v>>24), byte(v>>16), byte(v>>8), byte(v))
}

func (x *ibuf) u64(v uint64) {
	var t [8]byte
	binary.BigEndian.PutUint64(t[:], v)
	x.b = append(x.b, t[:]...)
}

// fix appends a fixed-width, space-padded (truncated) text field.
func (x *ibuf) fix(s string, n int) {
	p := make([]byte, n)
	for i := range p {
		p[i] = ' '
	}
	copy(p, s)
	x.b = append(x.b, p...)
}

func nowNS() uint32 { return uint32(time.Now().Nanosecond()) }

func secOfDay() uint32 {
	t := time.Now()
	return uint32(t.Hour()*3600 + t.Minute()*60 + t.Second())
}

func rawPx(p float64) uint32 { return uint32(int64(p*100 + 0.5)) }

// price_decimals = 2

func msgSeconds(sec uint32) []byte { // 'T' payload 4
	x := &ibuf{}
	x.u8('T')
	x.u32(sec)
	return x.b
}

func msgSystemEvent(code byte) []byte { // 'S' payload 17
	x := &ibuf{}
	x.u8('S')
	x.u32(nowNS()) // timestamp
	x.fix("", 8)   // group
	x.u8(code)     // event_code
	x.u32(0)       // orderbook
	return x.b
}

func msgPriceTick() []byte { // 'L' payload 16
	x := &ibuf{}
	x.u8('L')
	x.u32(nowNS()) // timestamp
	x.u32(1)       // tick_size_table_id
	x.u32(1)       // tick_size (raw; /100 = 0.01 display)
	x.u32(0)       // price_start
	return x.b
}

func msgCompanyDir(company uint32, name, id string) []byte { // 'P' payload 68
	x := &ibuf{}
	x.u8('P')
	x.u32(nowNS()) // timestamp
	x.u32(company) // company
	x.fix(name, 30)
	x.fix(id, 30)
	return x.b
}

func msgOrderbookDir(ob uint32, sym, name string) []byte { // 'R' payload 170
	x := &ibuf{}
	x.u8('R')
	x.u32(nowNS()) // timestamp
	x.u32(ob)      // orderbook
	x.u8('P')      // price_type
	x.fix(sym, 12) // isin (placeholder)
	x.fix(sym, 12) // sec_code (ticker)
	x.fix("BDT", 3)
	x.fix("PUBLIC", 8) // group -> board "Main Board"
	x.u64(1)           // minimum_quantity
	x.u32(1)           // quantity_tick_size_table_id
	x.u32(1)           // price_tick_size_table_id
	x.u32(2)           // price_decimals
	x.u32(0)           // delisting_date
	x.u32(0)           // delisting_time
	x.u8('P')          // market_type = Public (NOT 'I' index)
	x.u32(ob)          // company (matches 'P')
	x.u8('N')          // listing_type
	x.fix("General", 12)
	x.fix("EQ", 12)
	x.fix(name, 60) // security_name
	x.u32(0)        // maturity_date
	x.u32(0)        // yield_decimals
	return x.b
}

func msgAddOrder(ref uint64, verb byte, qty uint64, ob uint32, priceRaw uint32) []byte { // 'A' payload 33
	x := &ibuf{}
	x.u8('A')
	x.u32(nowNS()) // timestamp
	x.u64(ref)     // order_number
	x.u8(verb)     // order_verb 'B'/'S'
	x.u64(qty)     // quantity
	x.u32(ob)      // orderbook
	x.u32(priceRaw)
	x.u32(0) // yield
	return x.b
}

func msgOrderExec(ref, qty, match uint64) []byte { // 'E' payload 28
	x := &ibuf{}
	x.u8('E')
	x.u32(nowNS()) // timestamp
	x.u64(ref)     // order_number
	x.u64(qty)     // executed_quantity
	x.u64(match)   // match_number
	return x.b
}

func msgOrderDelete(ref uint64) []byte { // 'D' payload 12
	x := &ibuf{}
	x.u8('D')
	x.u32(nowNS()) // timestamp
	x.u64(ref)     // order_number
	return x.b
}
