package instrument

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"velexa-sim-me/internal/database/audit"
	"velexa-sim-me/internal/rest/middleware"
)

// Export serves /admin/api/instruments/export.
func (h *Handler) Export(w http.ResponseWriter, r *http.Request) {
	eng := h.Eng
	q := r.URL.Query()
	format := strings.ToLower(strings.TrimSpace(q.Get("format")))
	if format == "" {
		format = "xml"
	}
	rows := filterInstrumentRows(instrumentRows(eng), q.Get("board"), q.Get("class"), q.Get("status"), q.Get("q"))
	stamp := time.Now().Format("20060102-150405")
	var body []byte
	var ctype string
	var err error
	switch format {
	case "xml":
		// An unfiltered export is the whole market, so it carries the index too.
		idx := 0.0
		if q.Get("board") == "" && q.Get("class") == "" && q.Get("status") == "" && q.Get("q") == "" {
			eng.Mu.RLock()
			idx = eng.Index
			eng.Mu.RUnlock()
		}
		body, err = instrumentsXML(rows, idx)
		ctype = "application/xml; charset=utf-8"
	case "csv":
		body, err = instrumentsCSV(rows)
		ctype = "text/csv; charset=utf-8"
	case "json":
		body, err = json.MarshalIndent(map[string]any{"count": len(rows), "instruments": masterRows(rows)}, "", " ")
		ctype = "application/json; charset=utf-8"
	default:
		http.Error(w, `{"error":"format must be xml, csv or json"}`, http.StatusBadRequest)
		return
	}
	if err != nil {
		http.Error(w, `{"error":"export failed"}`, http.StatusInternalServerError)
		return
	}
	audit.Record(middleware.ActorOf(r), "instruments.export", fmt.Sprintf("format=%s rows=%d", format, len(rows)))
	w.Header().Set("Content-Type", ctype)
	fname := fmt.Sprintf("instruments_dse_%s.%s", stamp, format)
	if format == "xml" {
		fname = stamp + "-eodtickers-ME.xml" // same naming as the DSE end-of-day file
	}
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, fname))
	w.Header().Set("X-Export-Count", strconv.Itoa(len(rows)))
	_, _ = w.Write(body)
}
