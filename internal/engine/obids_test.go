package engine

import (
	"path/filepath"
	"testing"
)

func instr(syms ...string) []Instrument {
	out := make([]Instrument, len(syms))
	for i, s := range syms {
		out[i] = Instrument{Symbol: s, Name: s, Prev: 10}
	}
	return out
}

// A restart with a new symbol sorted into the middle of the list must not move
// anyone else's orderbook id.
func TestOrderBookIDsSurviveARestart(t *testing.T) {
	path := filepath.Join(t.TempDir(), "orderbook_ids.json")

	first := NewEngine(instr("AAA", "BBB", "CCC"), NewBus())
	first.LoadOrderBookIDs(path)
	if first.OrderBookID("CCC") != obIDBase+2 {
		t.Fatalf("first run should use list positions, CCC = %d", first.OrderBookID("CCC"))
	}
	first.listInstruments(instr("BBC")) // listed while running: next free id
	if got := first.OrderBookID("BBC"); got != obIDBase+3 {
		t.Fatalf("new symbol while running = %d, want %d", got, obIDBase+3)
	}

	// Restart: the saved master is sorted, so BBC now sits between BBB and CCC.
	second := NewEngine(instr("AAA", "BBB", "BBC", "CCC"), NewBus())
	second.LoadOrderBookIDs(path)
	want := map[string]int{"AAA": obIDBase, "BBB": obIDBase + 1, "CCC": obIDBase + 2, "BBC": obIDBase + 3}
	for sym, id := range want {
		if got := second.OrderBookID(sym); got != id {
			t.Errorf("%s = %d after restart, want %d", sym, got, id)
		}
	}

	// A symbol first seen at a restart gets a new id, never a used one.
	third := NewEngine(instr("AAA", "ABB", "BBB", "BBC", "CCC"), NewBus())
	third.LoadOrderBookIDs(path)
	if got := third.OrderBookID("ABB"); got != obIDBase+4 {
		t.Errorf("ABB = %d, want %d", got, obIDBase+4)
	}
	if got := third.OrderBookID("BBB"); got != obIDBase+1 {
		t.Errorf("BBB moved to %d", got)
	}
}
