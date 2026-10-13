package instrument

import (
	"fmt"
	"net/http"
	"sort"

	"velexa-sim-me/internal/engine"
	"velexa-sim-me/internal/utils"
)

// List serves /admin/api/instruments.
func (h *Handler) List(w http.ResponseWriter, r *http.Request) {
	eng := h.Eng
	instr := eng.Instr()
	rows := instrumentRows(eng)
	summary, classes := instrumentSummary(rows)
	syms := make([]string, 0, len(instr))
	for _, in := range instr {
		syms = append(syms, in.Symbol)
	}
	sort.Strings(syms)
	auto := engine.AutoListedSymbols()
	note := "the loaded security master covers every symbol traded so far"
	if len(auto) > 0 {
		note = fmt.Sprintf("%d symbol(s) had to be auto-listed because the loaded master does not carry them — import the real master to give them their true previous close and tick", len(auto))
	}
	utils.WriteJSON(w, map[string]any{
		"count":        len(instr),
		"source":       engine.UniverseSource(),
		"file":         engine.InstrumentFile(),
		"exchange_url": engine.InstrumentURL(),
		"auto_list":    engine.AutoListEnabled(),
		"auto_listed":  auto,
		"symbols":      syms,
		"rows":         rows,
		"summary":      summary,
		"classes":      classes,
		"note":         note,
	})
}
