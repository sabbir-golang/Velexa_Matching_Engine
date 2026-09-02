package main

// Runtime endpoints for the Matching Engine. Defaults keep the single-box
// behaviour (everything on 127.0.0.1); when the ME runs on its own server,
// set these env vars to point back at the OMS box:
//   ME_DB_HOST        Postgres host (velexa_sim + broker DBs)   e.g. 103.169.105.75
//   ME_API_URL        api internal order endpoint               e.g. http://103.169.105.75:8007/api/internal/order
//   ME_FIX_WS         FIX initiator auth WebSocket              e.g. ws://103.169.105.75:8003/ws/auth
//   ME_ADVERTISE_HOST host the FIX initiator dials to reach THIS ME's :5001 acceptor  e.g. 103.169.105.76
//   ME_AUDIT_URL      audit service base                        e.g. http://103.169.105.75:8092

import (
	"fmt"
	"os"
	"strings"
)

func envOr(k, d string) string {
	if v := strings.TrimSpace(os.Getenv(k)); v != "" {
		return v
	}
	return d
}

func meDBHost() string { return envOr("ME_DB_HOST", "127.0.0.1") }

// meDSN → the ME's OWN Postgres (local, independent). Holds velexa_sim (the
// synthetic benchmark data). Defaults to localhost so the ME server is
// self-contained.
func meDSN(db string) string {
	return fmt.Sprintf("host=%s port=%s user=%s password=%s dbname=%s sslmode=disable",
		meDBHost(), envOr("ME_DB_PORT", "5432"), envOr("ME_DB_USER", "postgres"),
		envOr("ME_DB_PASS", "Bdtask@#123"), db)
}

// omsDSN → the OMS box's Postgres (the broker tenant DBs + master + audit),
// used only by the cross-system Live/cleanup tooling. ME_OMS_HOST points at the
// OMS server; falls back to ME_DB_HOST for single-box mode.
func omsDBHost() string { return envOr("ME_OMS_HOST", meDBHost()) }
func omsDSN(db string) string {
	return fmt.Sprintf("host=%s port=%s user=%s password=%s dbname=%s sslmode=disable",
		omsDBHost(), envOr("ME_OMS_PORT", "5432"), envOr("ME_OMS_USER", "postgres"),
		envOr("ME_OMS_PASS", "Bdtask@#123"), db)
}

func meAPIURL() string       { return envOr("ME_API_URL", "http://127.0.0.1:8007/api/internal/order") }
func meAPILatencyURL() string {
	if v := envOr("ME_API_LATENCY_URL", ""); v != "" {
		return v
	}
	return strings.Replace(meAPIURL(), "/api/internal/order", "/api/internal/latency", 1)
}
func meFixAuthWS() string    { return envOr("ME_FIX_WS", "ws://127.0.0.1:8003/ws/auth") }
func meFixLatencyURL() string {
	if v := envOr("ME_FIX_LATENCY_URL", ""); v != "" {
		return v
	}
	u := strings.Replace(meFixAuthWS(), "wss://", "https://", 1)
	u = strings.Replace(u, "ws://", "http://", 1)
	return strings.Replace(u, "/ws/auth", "/api/latency", 1)
}
func meAdvertiseHost() string { return envOr("ME_ADVERTISE_HOST", "127.0.0.1") }
func meAuditURL() string     { return envOr("ME_AUDIT_URL", "http://127.0.0.1:8092") }
