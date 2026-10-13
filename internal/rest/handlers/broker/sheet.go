package broker

import (
	"net/http"
)

// Sheet serves /admin/api/broker/sheet.
// Onboarding sheet — renders a printable FIX/FIXS connectivity card filled
// with a broker's values (posted from the credential-issue step).
func (h *Handler) Sheet(w http.ResponseWriter, r *http.Request) {
	r.ParseForm()
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Write([]byte(brokerSheetHTML(r.FormValue("company"), r.FormValue("sender"),
		r.FormValue("fix_pass"), r.FormValue("itch_user"), r.FormValue("itch_pass"))))
}
