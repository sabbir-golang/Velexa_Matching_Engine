package public

import (
	"fmt"
	"net/http"
	"strings"
	"time"
)

// Events serves /events.
// events is dual-mode: a WebSocket upgrade (the trader SPA news socket)
// routes to the itch news/time-&-sales feed; a plain GET is the sim UI's
// SSE market stream.
func (h *Handler) Events(w http.ResponseWriter, r *http.Request) {
	bus := h.Bus
	itch := h.Itch
	if strings.EqualFold(r.Header.Get("Upgrade"), "websocket") {
		itch.ServeEvents(w, r)
		return
	}
	fl, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "stream unsupported", 500)
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")
	ch := bus.Subscribe()
	defer bus.Unsubscribe(ch)
	fmt.Fprintf(w, "retry: 3000\n\n")
	fl.Flush()
	ka := time.NewTicker(15 * time.Second)
	defer ka.Stop()
	for {
		select {
		case <-r.Context().Done():
			return
		case msg := <-ch:
			fmt.Fprintf(w, "data: %s\n\n", msg)
			fl.Flush()
		case <-ka.C:
			fmt.Fprintf(w, ": keepalive\n\n")
			fl.Flush()
		}
	}
}
