package engine

import (
	"fmt"
	"testing"
)

// BenchmarkSubmitAndMatch measures end-to-end order handling on the matching
// goroutine: half the orders rest, half cross and trade against them.
//
//	go test ./internal/engine -run '^$' -bench . -benchtime 100000x
func BenchmarkSubmitAndMatch(b *testing.B) {
	e := NewEngine([]Instrument{{Symbol: "GP", Name: "Grameenphone", Prev: 10}}, NewBus())
	go e.Run()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		side, px := Buy, 9.9
		if i%2 == 1 {
			side, px = Sell, 9.9 // crosses the bid just placed
		}
		o := e.NewOrder("GP", side, Limit, px, 100, "FIX:BENCH")
		o.ClOrdID = fmt.Sprint(i)
		e.SubmitAndWait(o)
	}
}
