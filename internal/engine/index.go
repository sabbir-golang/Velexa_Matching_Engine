package engine

import "sort"

// Index orderbook ids sit below the instrument range (obIDBase) so they never
// collide with a listed symbol.
const (
	IndexOBDSEX = 90001
	IndexOBDS30 = 90002
)

// IndexQuote is the current value of one index.
type IndexQuote struct {
	Name  string
	OB    int
	Value float64
	Prev  float64
}

// IndexBases are the starting values the indices are scaled to.
var indexBase = map[string]float64{"DSEX": 6000, "DS30": 2200}

// Indices returns DSEX (every tradable instrument, as before) and DS30 (the 30
// highest priced equities). Both are price-sum indices scaled to a base value:
// the simulator has no market capitalisation to weight them with.
func (e *Engine) Indices() []IndexQuote {
	e.Mu.RLock()
	defer e.Mu.RUnlock()
	type px struct {
		last, prev float64
	}
	var all, eq []px
	for _, in := range e.Instr() {
		md := e.MdOf(in.Symbol)
		if md == nil || !MetaFor(in.Symbol).Tradable() {
			continue
		}
		p := px{md.Last, md.Prev}
		all = append(all, p)
		if AssetClassOf(MetaFor(in.Symbol)) == "EQ" {
			eq = append(eq, p)
		}
	}
	sort.Slice(eq, func(i, j int) bool { return eq[i].prev > eq[j].prev })
	if len(eq) > 30 {
		eq = eq[:30]
	}
	calc := func(name string, ob int, set []px) IndexQuote {
		sum, prevSum := 0.0, 0.0
		for _, p := range set {
			sum += p.last
			prevSum += p.prev
		}
		base := indexBase[name]
		if prevSum <= 0 {
			return IndexQuote{Name: name, OB: ob, Value: base, Prev: base}
		}
		return IndexQuote{Name: name, OB: ob, Value: Round2(base * sum / prevSum), Prev: base}
	}
	return []IndexQuote{calc("DSEX", IndexOBDSEX, all), calc("DS30", IndexOBDS30, eq)}
}

// SymbolForOB returns the symbol of an orderbook id ("" when unknown).
func (e *Engine) SymbolForOB(ob int) string {
	for sym, id := range *e.obIDs.Load() {
		if id == ob {
			return sym
		}
	}
	return ""
}
