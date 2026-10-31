package simulator

import (
	"math"
	"math/rand"
	"sort"
	"strings"

	"velexa-sim-me/internal/database/brokers"
	"velexa-sim-me/internal/engine"
)

// bots.go — what the market bots (omsbots.go) and the benchmark share: which
// shares to trade, and how to price an order on one. A share has a fair price
// that drifts around its previous close; an order either quotes a few ticks
// from it (a limit order, which rests) or takes the best opposite price (an
// IOC order, which trades). Prices stay well inside the daily band and on the
// instrument's tick.

// blueChips are picked first when no symbols are given (those that are listed
// and active), then the busiest other shares.
var blueChips = []string{"GP", "BATBC", "SQURPHARMA", "BEXIMCO", "BRACBANK", "RENATA", "WALTONHIL", "ROBI",
	"LHBL", "BXPHARMA", "OLYMPIC", "ISLAMIBANK", "CITYBANK", "DUTCHBANGL", "MARICO", "UPGDCL", "SUMITPOWER",
	"BSRMLTD", "SINGERBD", "UNILEVERCL"}

// bandShare keeps bot prices this far inside the narrowest DSE band (±3.75%).
const bandShare = 0.03

type symState struct {
	sym  string
	tick float64
	lot  int64
	ref  float64 // drifting fair price
}

// botOrders counts the orders the old engine-only market bots (SIMBOT) left
// in the book; Clean Data cancels them.
func (s *Sim) botOrders() int {
	isBot := actorPrefix(brokers.SimBotPrefix)
	return s.eng.CountOpen(func(actor, _ string) bool { return isBot(actor) })
}

func clamp(v, lo, hi, def int) int {
	if v == 0 {
		return def
	}
	return max(lo, min(hi, v))
}

// pickSymbols chooses the shares to trade: the symbols given (those listed and
// active), else count of them, blue chips first, then the busiest others.
func (s *Sim) pickSymbols(symbols []string, count int) []*symState {
	rows := s.eng.MarketRows()
	type cand struct {
		sym    string
		volume int64
	}
	ok := map[string]cand{}
	for _, r := range rows {
		if r.Status == "active" && r.Prev > 0 && r.AssetClass == "EQ" {
			ok[r.Symbol] = cand{r.Symbol, r.Volume}
		}
	}
	var pick []string
	if len(symbols) > 0 {
		for _, sym := range symbols {
			sym = strings.ToUpper(strings.TrimSpace(sym))
			if _, good := ok[sym]; good {
				pick = append(pick, sym)
			}
		}
	} else {
		seen := map[string]bool{}
		for _, sym := range blueChips {
			if _, good := ok[sym]; good && len(pick) < count {
				pick, seen[sym] = append(pick, sym), true
			}
		}
		rest := make([]cand, 0, len(ok))
		for _, c := range ok {
			if !seen[c.sym] {
				rest = append(rest, c)
			}
		}
		sort.Slice(rest, func(i, j int) bool {
			if rest[i].volume != rest[j].volume {
				return rest[i].volume > rest[j].volume
			}
			return rest[i].sym < rest[j].sym
		})
		for _, c := range rest {
			if len(pick) >= count {
				break
			}
			pick = append(pick, c.sym)
		}
	}
	out := make([]*symState, 0, len(pick))
	for _, sym := range pick {
		m := engine.MetaFor(sym)
		st := &symState{sym: sym, tick: m.Tick, lot: int64(m.Lot)}
		if st.tick <= 0 {
			st.tick = 0.10
		}
		if st.lot < 1 {
			st.lot = 1
		}
		out = append(out, st)
	}
	return out
}

// market reads one instrument's previous close and best prices.
func (s *Sim) market(sym string) (prev, bid, ask float64) {
	s.eng.Mu.RLock()
	defer s.eng.Mu.RUnlock()
	if md := s.eng.MdOf(sym); md != nil {
		prev, bid, ask = md.Prev, md.Bid, md.Ask
		if prev <= 0 {
			prev = md.Last
		}
	}
	return
}

func onTick(px, tick float64) float64 { return engine.Round2(math.Round(px/tick) * tick) }

// band is the price range bot orders keep to: well inside the narrowest daily
// band around the previous close, on the tick.
func (st *symState) band(prev float64) (lo, hi float64) {
	return onTick(prev*(1-bandShare), st.tick) + st.tick, onTick(prev*(1+bandShare), st.tick) - st.tick
}

// drift moves the fair price one step: it wanders and is pulled back toward
// the previous close.
func (st *symState) drift(rng *rand.Rand, prev, bid, ask float64) {
	lo, hi := st.band(prev)
	if st.ref <= 0 {
		st.ref = prev
		if bid > 0 && ask > 0 {
			st.ref = (bid + ask) / 2
		}
	}
	st.ref += (prev-st.ref)*0.01 + rng.NormFloat64()*st.ref*0.0015
	st.ref = math.Max(lo, math.Min(hi, st.ref))
}

// price is a limit price and time in force for side ("1" buy, "2" sell): with
// take, the best opposite price as an IOC order, which trades; otherwise a few
// ticks behind the fair price as a day order, which rests.
func (st *symState) price(rng *rand.Rand, side string, take bool, prev, bid, ask float64) (float64, string) {
	lo, hi := st.band(prev)
	var px float64
	tif := "0"
	switch {
	case take && side == "1" && ask > 0:
		px, tif = ask, "3"
	case take && side == "2" && bid > 0:
		px, tif = bid, "3"
	case side == "1":
		px = st.ref - float64(1+rng.Intn(4))*st.tick
	default:
		px = st.ref + float64(1+rng.Intn(4))*st.tick
	}
	return math.Max(lo, math.Min(hi, onTick(px, st.tick))), tif
}
