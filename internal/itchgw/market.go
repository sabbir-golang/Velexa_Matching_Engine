package itchgw

// market.go — the DSE-ITCH market feed (SoupBinTCP server, default :5501).
//
// The feed is driven by the engine's market-event journal: every order added to
// the book is an 'A', every execution of a resting order an 'E' (or 'C' in a call
// auction), every removal a 'D', and trading-state and session changes are 'H' and
// 'S'. Order numbers are the engine's own, so the receiver's book is the engine's
// book. A new connection first gets the start-of-day set the OMS itch service
// needs, in the order its decoders require, then the book as it stands, then the
// live stream. A client that logs in again with the session and next sequence it
// last saw resumes where it left off.

import (
	"fmt"
	"log"
	"os"
	"strings"
	"sync"
	"time"

	"velexa-sim-me/internal/engine"
)

// Runner is a feed that can be started with Run (it blocks).
type Runner interface{ Run() }

// NewFeed returns the market feed: the journal driven one, or with
// ME_ITCH_MODE=legacy the original one that re-publishes the depth as synthetic orders.
func NewFeed(eng *engine.Engine) Runner {
	if strings.EqualFold(os.Getenv("ME_ITCH_MODE"), "legacy") {
		log.Printf("[ITCH] ME_ITCH_MODE=legacy: using the synthetic depth feed")
		return newLegacyFeed(eng)
	}
	return &marketFeed{eng: eng, session: fmt.Sprintf("SIM%07d", time.Now().Unix()%10000000)}
}

type marketFeed struct {
	eng     *engine.Engine
	session string

	mu     sync.Mutex
	resume struct{ soup, journal uint64 } // where the last client stopped
}

func (f *marketFeed) Run() {
	port := os.Getenv("ITCH_PORT")
	if port == "" {
		port = "5501"
	}
	if _, err := serveSoup("ITCH", ":"+port, f.serve); err != nil {
		log.Printf("[ITCH] %v", err)
		return
	}
	log.Printf("[ITCH] SoupBinTCP market feed listening on :%s (%d instruments, session %s)", port, len(f.eng.Instr()), f.session)
	select {}
}

// serve runs one logged-in client.
func (f *marketFeed) serve(s *soupConn, req loginReq) {
	j := f.eng.Journal()
	peer := s.c.RemoteAddr()

	cursor, resumed := uint64(0), false
	f.mu.Lock()
	rp := f.resume
	f.mu.Unlock()
	if req.session == f.session && req.seq > 1 && req.seq == rp.soup {
		if _, _, ok := j.Read(rp.journal, 1); ok {
			cursor, resumed = rp.journal, true
		}
	}
	if resumed {
		s.seq = req.seq
		if s.writePkt('A', loginAccept(f.session, req.seq)) != nil {
			return
		}
		log.Printf("[ITCH] %s: resumed session %s at seq %d", peer, f.session, req.seq)
	} else {
		if s.writePkt('A', loginAccept(f.session, 1)) != nil {
			return
		}
		var err error
		if cursor, err = f.startOfDay(s); err != nil {
			return
		}
		log.Printf("[ITCH] %s: LoginAccept sent (session=%s seq=1), start-of-day sent", peer, f.session)
	}
	defer func() {
		f.mu.Lock()
		f.resume.soup, f.resume.journal = s.seq, cursor
		f.mu.Unlock()
	}()

	p := &packer{s: s}
	var lastSec uint32
	idle := time.NewTicker(time.Second)
	defer idle.Stop()
	for !s.closed() {
		wake := j.Wait() // taken before reading so an append in between is not missed
		evs, next, ok := j.Read(cursor, 1000)
		if !ok {
			log.Printf("[ITCH] %s: client fell behind the journal, closing", peer)
			return
		}
		if len(evs) > 0 {
			for _, ev := range evs {
				if p.add(f.encode(ev, &lastSec)...) != nil {
					return
				}
			}
			if p.flush() != nil {
				return
			}
			cursor = next
			continue
		}
		select {
		case <-wake:
		case <-idle.C: // keep the receiver's clock moving
			lastSec = secOf(time.Now())
			if p.add(mSeconds(lastSec)) != nil || p.flush() != nil {
				return
			}
		case <-s.stop:
			return
		}
	}
}

// stamp returns the timestamp fields for t, with a 'T' first when the second changed.
func stamp(t time.Time, lastSec *uint32) (ns uint32, secMsg []byte) {
	if sec := secOf(t); sec != *lastSec {
		*lastSec = sec
		secMsg = mSeconds(sec)
	}
	return nsOf(t), secMsg
}

// encode turns one journal event into ITCH messages.
func (f *marketFeed) encode(ev engine.Event, lastSec *uint32) [][]byte {
	ns, sec := stamp(time.Unix(0, ev.At), lastSec)
	var out [][]byte
	if sec != nil {
		out = append(out, sec)
	}
	ob := uint32(ev.OB)
	switch ev.Kind {
	case engine.EvAdd:
		out = append(out, mAdd(ns, ev.OrderID, ev.Side, uint64(ev.Qty), ob, ev.PriceRaw))
	case engine.EvExec:
		out = append(out, mExec(ns, ev.OrderID, uint64(ev.Qty), ev.Match))
	case engine.EvExecAuction:
		out = append(out, mExecPrice(ns, ev.OrderID, uint64(ev.Qty), ev.Match, ob, ev.PriceRaw))
	case engine.EvDelete:
		out = append(out, mDelete(ns, ev.OrderID))
	case engine.EvState:
		out = append(out, mTradingState(ns, ob, ev.Code))
	case engine.EvSession:
		out = append(out, mSystemEvent(ns, ev.Code))
	case engine.EvClose:
		out = append(out, mClose(ns, uint64(ev.Qty), ob, ev.PriceRaw, ev.Match))
	case engine.EvRef:
		out = append(out, mReference(ns, ob, ev.PriceRaw))
	case engine.EvList:
		if sym := f.eng.SymbolForOB(int(ev.OB)); sym != "" {
			out = append(out, f.directory(ns, sym, true)...)
		}
	}
	return out
}

// groupFor is the DSE trading group (ITCH 'R' group) of an instrument.
func groupFor(m engine.InstrMeta) string {
	switch engine.AssetClassOf(m) {
	case "CB", "DB":
		return "DEBT"
	}
	b := strings.ToUpper(m.Board)
	switch {
	case strings.Contains(b, "ATB") && !strings.Contains(b, "SME"):
		return "ATBPUB"
	case strings.Contains(b, "SME"):
		return "SPUBLIC"
	}
	return "PUBLIC"
}

// directoryEntry builds the 'R' content for a listed symbol.
func (f *marketFeed) directoryEntry(sym string) rdir {
	m := engine.MetaFor(sym)
	name := sym
	for _, in := range f.eng.Instr() {
		if in.Symbol == sym {
			name = in.Name
			break
		}
	}
	isin := m.ISIN
	if isin == "" {
		isin = sym
	}
	listing := byte('N')
	if c := engine.CategoryOf(m.Board); c != "" {
		listing = c[0]
	}
	lot := uint64(1)
	if m.Lot > 1 {
		lot = uint64(m.Lot)
	}
	return rdir{ob: uint32(f.eng.OrderBookID(sym)), isin: isin, sec: sym, group: groupFor(m), market: 'P',
		listing: listing, sector: m.Sector, instr: engine.AssetClassOf(m), name: name, minQty: lot}
}

// directory is P + R + H, and with ref the reference-price 'A', for one symbol.
func (f *marketFeed) directory(ns uint32, sym string, ref bool) [][]byte {
	d := f.directoryEntry(sym)
	state := byte('T')
	if !engine.MetaFor(sym).Tradable() {
		state = 'V'
	}
	out := [][]byte{mCompany(ns, d.ob, d.name, sym), mOrderbook(ns, d), mTradingState(ns, d.ob, state)}
	if ref {
		if md := f.eng.MdOf(sym); md != nil {
			out = append(out, mReference(ns, d.ob, uint32(md.Prev*100+0.5)))
		}
	}
	return out
}

// startOfDay sends what a fresh receiver needs and returns the journal sequence
// to continue from. Order matters: 'P' before 'R', 'R' before 'H' and the
// reference price, and the reference price before any order.
func (f *marketFeed) startOfDay(s *soupConn) (uint64, error) {
	now := time.Now()
	var lastSec uint32
	ns, sec := stamp(now, &lastSec)
	p := &packer{s: s}
	p.add(sec, mSystemEvent(ns, 'O'), mPriceTick(ns), mQuantityTick(ns))

	var syms []string
	f.eng.Mu.RLock()
	for _, in := range f.eng.Instr() {
		if !engine.MetaFor(in.Symbol).Delisted {
			syms = append(syms, in.Symbol)
		}
	}
	f.eng.Mu.RUnlock()

	for _, sym := range syms {
		d := f.directoryEntry(sym)
		state := byte('T')
		if !engine.MetaFor(sym).Tradable() {
			state = 'V'
		}
		p.add(mCompany(ns, d.ob, d.name, sym), mOrderbook(ns, d), mTradingState(ns, d.ob, state))
	}
	idx := f.eng.Indices()
	for _, q := range idx {
		p.add(mCompany(ns, uint32(q.OB), q.Name, q.Name), mOrderbook(ns, rdir{ob: uint32(q.OB), isin: q.Name, sec: q.Name,
			group: "INDEX", market: 'I', listing: 'N', sector: "Index", instr: "IN", name: q.Name}),
			mTradingState(ns, uint32(q.OB), 'T'))
	}
	for _, sym := range syms {
		if md := f.eng.MdOf(sym); md != nil {
			p.add(mReference(ns, uint32(f.eng.OrderBookID(sym)), uint32(md.Prev*100+0.5)))
		}
	}
	for _, q := range idx {
		p.add(mReference(ns, uint32(q.OB), uint32(q.Prev*100+0.5)))
	}

	orders, next, phase := f.eng.SnapshotOrders()
	for _, o := range orders {
		p.add(mAdd(ns, o.ID, o.Side, uint64(o.Qty), uint32(o.OB), o.PriceRaw))
	}
	// the session the market is in now (pre-open, continuous, close, post-close or ended)
	if code := engine.SessionCode(phase); code != 0 {
		p.add(mSystemEvent(ns, code))
	}
	return next, p.flush()
}
