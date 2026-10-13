package instrument

import (
	"encoding/csv"
	"encoding/json"
	"encoding/xml"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"velexa-sim-me/internal/engine"
)

// ---- admin API ------------------------------------------------------------

// instrumentBoards are the board labels the Instruments page offers (the DSE
// board/category names the OMS uses). MAIN/SME/ATB are the older labels already
// in existing master files.
var instrumentBoards = []string{"Main (A)", "Main (B)", "Main (N)", "Main (Z)", "Bond", "Mutual Fund", "SME / ATB", "MAIN", "SME", "ATB"}

func canonBoard(b string) (string, bool) {
	b = strings.TrimSpace(b)
	for _, c := range instrumentBoards {
		if strings.EqualFold(b, c) {
			return c, true
		}
	}
	return b, false
}

// boardClass groups a board label into an asset class.
func boardClass(board string) string {
	b := strings.ToLower(board)
	switch {
	case strings.Contains(b, "bond"):
		return "bond"
	case strings.Contains(b, "mutual"):
		return "mf"
	case strings.Contains(b, "sme"), strings.Contains(b, "atb"):
		return "sme"
	}
	return "main"
}

// instrumentRows is the per-instrument table behind the admin Instruments page:
// reference data from the master plus live price/volume from the engine. Delisted
// scrips are left out.
func instrumentRows(eng *engine.Engine) []map[string]any {
	instr := eng.Instr()
	rows := make([]map[string]any, 0, len(instr))
	for _, in := range instr {
		m := engine.MetaFor(in.Symbol)
		if m.Delisted {
			continue
		}
		board := m.Board
		if board == "" {
			board = "MAIN"
		}
		tick := m.Tick
		if tick <= 0 {
			tick = engine.DefaultTick(in.Prev) // what the pre-trade check enforces when no tick is on file
		}
		lot := m.Lot
		if lot < 1 {
			lot = 1
		}
		row := map[string]any{"symbol": in.Symbol, "name": in.Name, "board": board, "sector": m.Sector,
			"isin": m.ISIN, "tick": tick, "lot": lot, "prev": in.Prev, "status": m.StatusOf(),
			"asset_class": engine.AssetClassOf(m),
			"halted":      m.Halted}
		if md := eng.MdOf(in.Symbol); md != nil {
			row["prev"], row["last"], row["volume"] = md.Prev, md.Last, md.Volume
			row["open"], row["high"], row["low"], row["trades"] = md.Open, md.High, md.Low, md.Trades
		}
		rows = append(rows, row)
	}
	sort.Slice(rows, func(i, j int) bool { return rows[i]["symbol"].(string) < rows[j]["symbol"].(string) })
	return rows
}

// instrumentSummary builds the KPI cards and the Asset Classes panel.
func instrumentSummary(rows []map[string]any) (map[string]any, []map[string]any) {
	cls := map[string]int{}
	var mainTradable, pending int
	for _, r := range rows {
		ac := r["asset_class"].(string)
		cls[ac]++
		st := r["status"].(string)
		if st == "pending" {
			pending++
		}
		if ac == "EQ" && boardClass(r["board"].(string)) == "main" && st == "active" {
			mainTradable++
		}
	}
	summary := map[string]any{"total": len(rows), "main_equity": mainTradable, "bonds": cls["CB"] + cls["DB"], "pending": pending}
	classes := make([]map[string]any, 0, len(engine.AssetClasses))
	for _, a := range engine.AssetClasses {
		classes = append(classes, map[string]any{"code": a.Code, "label": a.Label, "count": cls[a.Code], "color": a.Color})
	}
	return summary, classes
}

// validISIN checks the ISO 6166 shape: 2 letters, 9 alphanumerics, 1 check digit.
func validISIN(s string) bool {
	if len(s) != 12 {
		return false
	}
	for i, r := range s {
		switch {
		case r >= 'A' && r <= 'Z':
		case r >= '0' && r <= '9' && i >= 2:
		default:
			return false
		}
	}
	return true
}

var persistMu sync.Mutex

// filterInstrumentRows narrows the directory rows by board, asset class, status
// and a case-insensitive symbol/name search; blank filters match everything.
func filterInstrumentRows(rows []map[string]any, board, class, status, q string) []map[string]any {
	q = strings.ToLower(strings.TrimSpace(q))
	out := make([]map[string]any, 0, len(rows))
	for _, r := range rows {
		if board != "" && r["board"].(string) != board {
			continue
		}
		if class != "" && r["asset_class"].(string) != class {
			continue
		}
		if status != "" && r["status"].(string) != status {
			continue
		}
		if q != "" && !strings.Contains(strings.ToLower(r["symbol"].(string)+" "+r["name"].(string)), q) {
			continue
		}
		out = append(out, r)
	}
	return out
}

// masterRows converts directory rows to the master-file shape (which the import
// accepts back, so an exported JSON can be re-imported).
func masterRows(rows []map[string]any) []engine.MasterInstr {
	out := make([]engine.MasterInstr, 0, len(rows))
	for _, in := range rows {
		out = append(out, engine.MasterInstr{
			Symbol: in["symbol"].(string), Name: in["name"].(string), ISIN: in["isin"].(string),
			Board: in["board"].(string), PrevClose: in["prev"].(float64), Tick: in["tick"].(float64),
			Sector: in["sector"].(string), AssetClass: in["asset_class"].(string), Lot: in["lot"].(int),
			Status: in["status"].(string),
		})
	}
	return out
}

// eodTicker is one <Ticker .../> of the DSE end-of-day ticker file
// (YYYYMMDD-HHMMSS-eodtickers-DSE.xml). Every field is an attribute; Category,
// Sector and the trading figures are omitted when the instrument has none, exactly
// as the DSE file does for a scrip that did not trade that day.
type eodTicker struct {
	XMLName        xml.Name `xml:"Ticker"`
	SecurityCode   string   `xml:"SecurityCode,attr"`
	ISIN           string   `xml:"ISIN,attr"`
	Category       string   `xml:"Category,attr,omitempty"`
	AssetClass     string   `xml:"AssetClass,attr"`
	Sector         string   `xml:"Sector,attr,omitempty"`
	CompulsorySpot string   `xml:"CompulsorySpot,attr"`
	TradeDate      string   `xml:"TradeDate,attr"`
	Close          string   `xml:"Close,attr"`
	Open           string   `xml:"Open,attr,omitempty"`
	High           string   `xml:"High,attr,omitempty"`
	Low            string   `xml:"Low,attr,omitempty"`
	Var            string   `xml:"Var,attr,omitempty"`
	VarPercent     string   `xml:"VarPercent,attr,omitempty"`
}

type eodTickers struct {
	XMLName xml.Name    `xml:"EODTickers"`
	Tickers []eodTicker `xml:"Ticker"`
}

// price prints a price the way the DSE file does: no trailing zeros ("6.1", "77").
func price(v any) string {
	f, _ := v.(float64)
	return strconv.FormatFloat(engine.Round2(f), 'f', -1, 64)
}

func num(v any) float64 { f, _ := v.(float64); return f }

// rowToTicker maps a directory row to a DSE EOD ticker. Close is the last traded
// price; Var and VarPercent are measured against the previous close. A scrip that
// has not traded carries only its previous close, like the DSE file's untraded rows.
func rowToTicker(r map[string]any, date string) eodTicker {
	t := eodTicker{
		SecurityCode: r["symbol"].(string), ISIN: r["isin"].(string),
		Category: engine.CategoryOf(r["board"].(string)), AssetClass: r["asset_class"].(string),
		Sector: r["sector"].(string), CompulsorySpot: "N", TradeDate: date,
	}
	if t.AssetClass == "DB" {
		t.CompulsorySpot = "Y" // debentures trade on the compulsory spot market
	}
	if t.Category == "" && t.AssetClass != "EQ" {
		t.Category = "A" // funds and bonds are category A unless the master says otherwise
	}
	prev := num(r["prev"])
	trades, _ := r["trades"].(int64)
	if trades == 0 {
		t.Close = price(prev)
		return t
	}
	last := num(r["last"])
	t.Close, t.Open, t.High, t.Low = price(last), price(r["open"]), price(r["high"]), price(r["low"])
	v := last - prev
	t.Var = strconv.FormatFloat(v, 'f', 5, 64)
	pct := 0.0
	if prev != 0 {
		pct = v / prev * 100
	}
	t.VarPercent = strconv.FormatFloat(pct, 'f', 5, 64)
	return t
}

// instrumentsXML renders the directory as a DSE end-of-day ticker file. When
// index is non-zero, the engine's own DSEX index is listed first, as the DSE file
// lists its indices.
func instrumentsXML(rows []map[string]any, index float64) ([]byte, error) {
	date := time.Now().Format("20060102")
	doc := eodTickers{Tickers: make([]eodTicker, 0, len(rows)+1)}
	if index > 0 {
		doc.Tickers = append(doc.Tickers, eodTicker{SecurityCode: "DSEX", ISIN: "DSEX", AssetClass: "IN",
			CompulsorySpot: "N", TradeDate: date, Close: strconv.FormatFloat(index, 'f', 5, 64)})
	}
	for _, r := range rows {
		doc.Tickers = append(doc.Tickers, rowToTicker(r, date))
	}
	b, err := xml.MarshalIndent(doc, "", "  ")
	if err != nil {
		return nil, err
	}
	s := strings.ReplaceAll(string(b), "></Ticker>", " />")
	return []byte(`<?xml version="1.0" encoding="utf-8"?>` + "\n" + s), nil
}

func instrumentsCSV(rows []map[string]any) ([]byte, error) {
	var sb strings.Builder
	cw := csv.NewWriter(&sb)
	_ = cw.Write([]string{"symbol", "isin", "name", "asset_class", "board", "sector", "lot", "tick", "prev_close", "last", "volume", "status"})
	for _, r := range rows {
		vol, _ := r["volume"].(int64)
		_ = cw.Write([]string{r["symbol"].(string), r["isin"].(string), r["name"].(string), r["asset_class"].(string),
			r["board"].(string), r["sector"].(string), strconv.Itoa(r["lot"].(int)),
			strconv.FormatFloat(num(r["tick"]), 'f', 2, 64), strconv.FormatFloat(num(r["prev"]), 'f', 2, 64),
			strconv.FormatFloat(num(r["last"]), 'f', 2, 64), strconv.FormatInt(vol, 10), r["status"].(string)})
	}
	cw.Flush()
	return []byte(sb.String()), cw.Error()
}

// persistUniverse writes the engine's whole current universe to the master file
// (normalised JSON). It dumps everything rather than upserting one row because
// the file is the sole source at next boot: a one-row file would shrink the
// universe to a single instrument on restart when none existed before. Delisted
// scrips are omitted, so they are gone after a restart.
func persistUniverse(eng *engine.Engine) error {
	rows := instrumentRows(eng)
	out := make([]engine.MasterInstr, 0, len(rows))
	for _, in := range rows {
		out = append(out, engine.MasterInstr{
			Symbol: in["symbol"].(string), Name: in["name"].(string), ISIN: in["isin"].(string),
			Board: in["board"].(string), PrevClose: in["prev"].(float64), Tick: in["tick"].(float64),
			Sector: in["sector"].(string), AssetClass: in["asset_class"].(string), Lot: in["lot"].(int), Status: in["status"].(string),
		})
	}
	body, err := json.MarshalIndent(map[string]any{"count": len(out), "instruments": out}, "", " ")
	if err != nil {
		return err
	}
	persistMu.Lock()
	defer persistMu.Unlock()
	return engine.SaveInstrumentFile(body)
}
