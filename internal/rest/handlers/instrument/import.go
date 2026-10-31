package instrument

import (
	"fmt"
	"io"
	"log"
	"net/http"
	"strconv"
	"strings"

	"velexa-sim-me/internal/database/audit"
	"velexa-sim-me/internal/engine"
	"velexa-sim-me/internal/rest/middleware"
	"velexa-sim-me/internal/utils"
)

func (h *Handler) Reload(w http.ResponseWriter, r *http.Request) {
	eng := h.Eng
	if r.Method != http.MethodPost {
		http.Error(w, `{"error":"POST only"}`, http.StatusMethodNotAllowed)
		return
	}
	added, updated := eng.ListInstruments(engine.UniverseSet{Instr: engine.LoadUniverse()})
	audit.Record(middleware.ActorOf(r), "instruments.reload", fmt.Sprintf("added=%d updated=%d", added, updated))
	log.Printf("[universe] reload: +%d new, %d refreshed (source: %s)", added, updated, engine.UniverseSource())
	utils.WriteJSON(w, map[string]any{"ok": true, "added": added, "updated": updated,
		"count": len(eng.Instr()), "source": engine.UniverseSource()})
}

const importLimit = 32 << 20

type importNew struct {
	Symbol     string  `json:"symbol"`
	Name       string  `json:"name"`
	Board      string  `json:"board"`
	Sector     string  `json:"sector"`
	AssetClass string  `json:"asset_class"`
	Prev       float64 `json:"prev"`
	Relisted   bool    `json:"relisted,omitempty"`
}

type importChange struct {
	Field string `json:"field"`
	From  string `json:"from"`
	To    string `json:"to"`
}

type importUpdate struct {
	Symbol  string         `json:"symbol"`
	Changes []importChange `json:"changes"`
}

type importSkip struct {
	Symbol string `json:"symbol"`
	Reason string `json:"reason"`
}
type importPlan struct {
	apply     []engine.MasterInstr
	New       []importNew    `json:"new"`
	Updated   []importUpdate `json:"updated"`
	Skipped   []importSkip   `json:"skipped"`
	Unchanged int            `json:"unchanged"`
	Indices   int            `json:"indices"`
}

// ?mode=merge   add new instruments and update listed ones (default)
// ?mode=new     add new instruments only
// ?mode=update  update listed instruments only
// ?dry_run=1    report what would change and change nothing
func (h *Handler) Import(w http.ResponseWriter, r *http.Request) {
	eng := h.Eng
	if r.Method != http.MethodPost {
		http.Error(w, `{"error":"POST only"}`, http.StatusMethodNotAllowed)
		return
	}
	fail := func(msg string) { utils.WriteJSON(w, map[string]any{"ok": false, "error": msg}) }
	mode := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("mode")))
	switch mode {
	case "":
		mode = "merge"
	case "merge", "new", "update":
	default:
		fail("mode must be merge, new or update")
		return
	}
	dry := r.URL.Query().Get("dry_run") != ""
	body, err := io.ReadAll(io.LimitReader(r.Body, importLimit+1))
	if err != nil {
		fail("the file could not be read")
		return
	}
	if len(body) > importLimit {
		fail("the file is larger than 32 MB")
		return
	}
	rows, err := engine.ParseMaster(body)
	if err != nil {
		fail("this file could not be read as a security master: " + err.Error())
		return
	}
	plan := planImport(eng, rows, mode)
	out := map[string]any{"ok": true, "dry_run": dry, "mode": mode, "format": engine.MasterFormat(body),
		"rows": len(rows), "plan": plan, "count": len(eng.Instr())}
	if dry || len(plan.apply) == 0 {
		utils.WriteJSON(w, out)
		return
	}

	before := map[string]bool{}
	for _, u := range plan.Updated {
		before[u.Symbol] = engine.MetaFor(u.Symbol).Tradable()
	}
	added, updated := eng.ListInstruments(engine.BuildUniverse(plan.apply))
	for sym, was := range before {
		if engine.MetaFor(sym).Tradable() != was {
			eng.PublishTradingState(sym) // halted or resumed by the import
		}
	}
	out["added"], out["updated"], out["count"] = added, updated, len(instrumentRows(eng))
	if err := persistUniverse(eng); err != nil {
		out["warning"] = fmt.Sprintf("applied, but could not be saved to %s: %v — it is lost on restart", engine.InstrumentFile(), err)
	} else {
		out["persisted_to"] = engine.InstrumentFile()
	}
	audit.Record(middleware.ActorOf(r), "instruments.import", fmt.Sprintf("%s · mode=%s · %d new, %d updated, %d unchanged, %d skipped",
		engine.MasterFormat(body), mode, len(plan.New), len(plan.Updated), plan.Unchanged, len(plan.Skipped)))
	log.Printf("[universe] admin import (%s, mode %s): %d new, %d updated, %d unchanged, %d skipped",
		engine.MasterFormat(body), mode, len(plan.New), len(plan.Updated), plan.Unchanged, len(plan.Skipped))
	utils.WriteJSON(w, out)
}
func planImport(eng *engine.Engine, rows []engine.MasterInstr, mode string) importPlan {
	cur := map[string]map[string]any{}
	for _, row := range instrumentRows(eng) {
		cur[row["symbol"].(string)] = row
	}
	p := importPlan{New: []importNew{}, Updated: []importUpdate{}, Skipped: []importSkip{}}
	seen := map[string]bool{}
	for _, in := range rows {
		sym := strings.ToUpper(strings.TrimSpace(in.Symbol))
		in.Symbol = sym
		skip := func(reason string) { p.Skipped = append(p.Skipped, importSkip{sym, reason}) }
		switch {
		case in.IsIndex:
			p.Indices++
			continue
		case sym == "":
			skip("no symbol")
			continue
		case !engine.ValidSymbol(sym):
			skip("not a valid ticker (up to 20 characters from A-Z, 0-9, . - _)")
			continue
		case seen[sym]:
			skip("appears more than once in the file; the first row is used")
			continue
		case strings.EqualFold(strings.TrimSpace(in.Status), "delisted"):
			skip("marked delisted")
			continue
		}
		seen[sym] = true
		if in.ISIN = strings.ToUpper(strings.TrimSpace(in.ISIN)); in.ISIN != "" && !validISIN(in.ISIN) {
			skip("ISIN " + in.ISIN + " is not 12 characters (2 letters, then 10 letters or digits)")
			continue
		}
		if st := strings.ToLower(strings.TrimSpace(in.Status)); st != "" && st != "active" && st != "halted" && st != "pending" {
			skip("status " + in.Status + " is not active, halted or pending")
			continue
		}
		if in.AssetClass != "" && engine.NormAssetClass(in.AssetClass) == "" {
			skip("asset class " + in.AssetClass + " is not EQ, MF, CB or DB")
			continue
		}
		if c, ok := canonBoard(in.Board); ok {
			in.Board = c
		}

		c, listed := cur[sym]
		if !listed {
			if mode == "update" {
				skip("not listed (this import only updates listed instruments)")
				continue
			}
			if in.PrevClose <= 0 {
				skip("no previous close (reference price)")
				continue
			}
			if strings.TrimSpace(in.Name) == "" {
				in.Name = engine.BuiltinName(sym)
			}
			if in.Board == "" && in.AssetClass == "" {
				in.Board = "Main (A)"
			}
			p.apply = append(p.apply, in)
			name := in.Name
			if name == "" {
				name = sym
			}
			p.New = append(p.New, importNew{Symbol: sym, Name: name, Board: in.Board, Sector: in.Sector,
				AssetClass: engine.NormAssetClass(in.AssetClass), Prev: engine.Round2(in.PrevClose), Relisted: engine.MetaFor(sym).Delisted})
			continue
		}
		if mode == "new" {
			skip("already listed (this import only adds new instruments)")
			continue
		}
		str := func(k string) string { s, _ := c[k].(string); return s }
		fill := func(v *string, k string) {
			if strings.TrimSpace(*v) == "" {
				*v = str(k)
			}
		}
		fill(&in.Name, "name")
		fill(&in.ISIN, "isin")
		fill(&in.Board, "board")
		fill(&in.Sector, "sector")
		fill(&in.AssetClass, "asset_class")
		fill(&in.Status, "status")
		in.AssetClass = engine.NormAssetClass(in.AssetClass)
		in.Status = strings.ToLower(strings.TrimSpace(in.Status))
		if in.PrevClose <= 0 {
			in.PrevClose = num(c["prev"])
		}
		if in.Tick <= 0 {
			in.Tick = num(c["tick"])
		}
		if in.Lot <= 0 {
			in.Lot, _ = c["lot"].(int)
		}
		in.PrevClose = engine.Round2(in.PrevClose)

		var ch []importChange
		diff := func(field, from, to string) {
			if !strings.EqualFold(strings.TrimSpace(from), strings.TrimSpace(to)) {
				ch = append(ch, importChange{field, from, to})
			}
		}
		money := func(v float64) string { return strconv.FormatFloat(v, 'f', 2, 64) }
		diff("Ref price", money(num(c["prev"])), money(in.PrevClose))
		diff("Name", str("name"), in.Name)
		diff("ISIN", str("isin"), in.ISIN)
		diff("Board", str("board"), in.Board)
		diff("Sector", str("sector"), in.Sector)
		diff("Asset class", str("asset_class"), in.AssetClass)
		lot, _ := c["lot"].(int)
		diff("Lot", strconv.Itoa(lot), strconv.Itoa(in.Lot))
		diff("Tick", money(num(c["tick"])), money(in.Tick))
		diff("Status", str("status"), in.Status)
		if len(ch) == 0 {
			p.Unchanged++
			continue
		}
		p.apply = append(p.apply, in)
		p.Updated = append(p.Updated, importUpdate{sym, ch})
	}
	return p
}
