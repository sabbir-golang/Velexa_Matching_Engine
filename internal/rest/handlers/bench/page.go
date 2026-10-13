package bench

import (
	"net/http"

	"velexa-sim-me/internal/web"
)

// Page serves /bench.
func (h *Handler) Page(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache")
	w.Write([]byte(web.BenchHTML))
}
