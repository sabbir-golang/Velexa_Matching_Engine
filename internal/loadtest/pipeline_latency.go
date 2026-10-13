package loadtest

import (
	"encoding/json"
	"net/http"
	"time"

	"velexa-sim-me/internal/config"
	"velexa-sim-me/internal/latency"
)

// fetchStages pulls a {"stages":{name:{count,avg_us,p50_us,p95_us,p99_us}}}
// document from a service's latency endpoint. Returns nil on any error.
func fetchStages(url string) map[string]map[string]float64 {
	c := http.Client{Timeout: 3 * time.Second}
	resp, err := c.Get(url)
	if err != nil {
		return nil
	}
	defer resp.Body.Close()
	var out struct {
		Stages map[string]map[string]float64 `json:"stages"`
	}
	if json.NewDecoder(resp.Body).Decode(&out) != nil {
		return nil
	}
	return out.Stages
}

type latRow struct {
	Key   string   `json:"key"`
	Label string   `json:"label"`
	Src   string   `json:"src"`
	Count float64  `json:"count"`
	Avg   *float64 `json:"avg_us"`
	P50   *float64 `json:"p50_us"`
	P95   *float64 `json:"p95_us"`
	P99   *float64 `json:"p99_us"`
}

func meRow() latRow {
	row := latRow{Key: "me", Label: "Matching Engine", Src: "me"}
	if c, avg, p50, p95, p99 := latency.Fix.Snap(); c > 0 {
		us := func(ns int64) *float64 { v := float64(ns) / 1000.0; return &v }
		row.Count = float64(c)
		row.Avg, row.P50, row.P95, row.P99 = us(avg), us(p50), us(p95), us(p99)
	}
	return row
}

// PipelineLatency assembles the full ordered order-pipeline latency for /bench.
// Phase 1 fills the api-owned stages (API/RMS/Kafka/DB/Total) from velexa-api;
// FIX Initiator / Matching Engine (Phase 2) and Blotter (Phase 3) are present
// but pending until their services are instrumented.
func PipelineLatency() map[string]any {
	api := fetchStages(config.MeAPILatencyURL())
	fixS := fetchStages(config.MeFixLatencyURL())
	pick := func(key, label, src string, from map[string]map[string]float64) latRow {
		row := latRow{Key: key, Label: label, Src: src}
		if from != nil {
			if m, ok := from[key]; ok {
				row.Count = m["count"]
				a, p50, p95, p99 := m["avg_us"], m["p50_us"], m["p95_us"], m["p99_us"]
				row.Avg, row.P50, row.P95, row.P99 = &a, &p50, &p95, &p99
			}
		}
		return row
	}
	rows := []latRow{
		pick("api", "API", "api", api),
		pick("rms", "RMS", "api", api),
		pick("kafka", "Kafka", "fix", fixS),
		pick("fix", "FIX Initiator", "fix", fixS),
		meRow(),
		pick("db", "DB Store", "api", api),
	}
	return map[string]any{"stages": rows, "target_ms": 10}
}
