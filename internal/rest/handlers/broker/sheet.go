package broker

import (
	"net/http"

	"velexa-sim-me/internal/rest/middleware"
)

// Sheet serves /admin/api/broker/sheet.
// Onboarding sheet — renders a printable FIX/FIXS connectivity card filled
// with a broker's values (posted from the credential-issue step).
func (h *Handler) Sheet(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 1<<16)
	r.ParseForm()
	if !middleware.CanAccessBroker(r, r.FormValue("sender")) {
		middleware.Forbid(w)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Write([]byte(brokerSheetHTML(r.FormValue("company"), r.FormValue("sender"),
		r.FormValue("fix_pass"), r.FormValue("itch_user"), r.FormValue("itch_pass"))))
}
