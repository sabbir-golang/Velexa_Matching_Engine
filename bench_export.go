package main

import (
	"context"
	"encoding/csv"
	"fmt"
	"net/http"
	"strconv"
	"sync/atomic"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// writeSummaryCSV emits a one-file benchmark report: throughput, per-stage
// latency percentiles, and RMS reject breakdown — evidence for DSE.
func (b *Bench) writeSummaryCSV(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "text/csv")
	w.Header().Set("Content-Disposition", `attachment; filename="velexa_bench_summary.csv"`)
	end := atomic.LoadInt64(&b.endNs)
	if end == 0 {
		end = time.Now().UnixNano()
	}
	elapsed := float64(end-b.startNs) / 1e9
	if elapsed <= 0 {
		elapsed = 1e-6
	}
	rc, ra, r50, r95, r99 := b.rmsStat.snap()
	mc, ma, m50, m95, m99 := b.matchStat.snap()
	pc, pa, p50, p95, p99 := b.persistStat.snap()
	ec, ea, e50, e95, e99 := b.e2eStat.snap()
	us := func(ns int64) string { return fmt.Sprintf("%.3f", float64(ns)/1000.0) }
	matched := atomic.LoadInt64(&b.matchCnt)
	trades := atomic.LoadInt64(&b.tradeCnt)

	cw := csv.NewWriter(w)
	cw.Write([]string{"Velexa Matching Engine - Benchmark Report"})
	cw.Write([]string{"Generated", time.Now().Format("2006-01-02 15:04:05")})
	cw.Write([]string{"DSE Requirement (trades/sec)", "8000"})
	cw.Write([]string{"Config: RMS", fmt.Sprint(b.cfg.RMS), "Persist", fmt.Sprint(b.cfg.Persist), "TargetRate", fmt.Sprint(b.cfg.Rate)})
	cw.Write(nil)
	cw.Write([]string{"THROUGHPUT"})
	cw.Write([]string{"metric", "value"})
	cw.Write([]string{"Elapsed (s)", fmt.Sprintf("%.2f", elapsed)})
	cw.Write([]string{"Orders/sec", fmt.Sprint(int64(float64(matched) / elapsed))})
	cw.Write([]string{"Trades/sec", fmt.Sprint(int64(float64(trades) / elapsed))})
	cw.Write([]string{"Generated", fmt.Sprint(atomic.LoadInt64(&b.genCnt))})
	cw.Write([]string{"RMS Passed", fmt.Sprint(atomic.LoadInt64(&b.rmsPass))})
	cw.Write([]string{"RMS Rejected", fmt.Sprint(atomic.LoadInt64(&b.rmsRej))})
	cw.Write([]string{"Matched", fmt.Sprint(matched)})
	cw.Write([]string{"Trades (fills)", fmt.Sprint(trades)})
	cw.Write([]string{"Persisted", fmt.Sprint(atomic.LoadInt64(&b.persistCnt))})
	cw.Write(nil)
	cw.Write([]string{"PER-STAGE LATENCY (microseconds)"})
	cw.Write([]string{"stage", "count", "avg", "p50", "p95", "p99"})
	cw.Write([]string{"RMS Check", fmt.Sprint(rc), us(ra), us(r50), us(r95), us(r99)})
	cw.Write([]string{"Matching", fmt.Sprint(mc), us(ma), us(m50), us(m95), us(m99)})
	cw.Write([]string{"DB Store", fmt.Sprint(pc), us(pa), us(p50), us(p95), us(p99)})
	cw.Write([]string{"End-to-end", fmt.Sprint(ec), us(ea), us(e50), us(e95), us(e99)})
	cw.Write(nil)
	cw.Write([]string{"RMS REJECT REASONS"})
	cw.Write([]string{"reason", "count"})
	b.rejReasons.Range(func(k, v any) bool {
		cw.Write([]string{k.(string), fmt.Sprint(atomic.LoadInt64(v.(*int64)))})
		return true
	})
	cw.Flush()
}

// ordersCSVHandler streams the persisted order rows (most recent first) with
// their per-order RMS + match times — the raw record set behind the summary.
func ordersCSVHandler(pool *pgxpool.Pool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if pool == nil {
			http.Error(w, "persistence not available", 500)
			return
		}
		limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
		if limit <= 0 || limit > 2000000 {
			limit = 100000
		}
		rows, err := pool.Query(context.Background(),
			`SELECT id, cl_ord_id, account, symbol, side, qty, price, status, reject_reason, rms_ns, match_ns, ts
			 FROM sim_orders ORDER BY id DESC LIMIT $1`, limit)
		if err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
		defer rows.Close()
		w.Header().Set("Content-Type", "text/csv")
		w.Header().Set("Content-Disposition", `attachment; filename="velexa_bench_orders.csv"`)
		cw := csv.NewWriter(w)
		cw.Write([]string{"id", "cl_ord_id", "account", "symbol", "side", "qty", "price", "status", "reject_reason", "rms_us", "match_us", "ts"})
		var id, qty int64
		var clord, acct, sym, side, status, reason string
		var price float64
		var rmsNs, matchNs int32
		var ts time.Time
		for rows.Next() {
			if err := rows.Scan(&id, &clord, &acct, &sym, &side, &qty, &price, &status, &reason, &rmsNs, &matchNs, &ts); err != nil {
				continue
			}
			cw.Write([]string{
				strconv.FormatInt(id, 10), clord, acct, sym, side,
				strconv.FormatInt(qty, 10), strconv.FormatFloat(price, 'f', 2, 64),
				status, reason,
				fmt.Sprintf("%.3f", float64(rmsNs)/1000.0),
				fmt.Sprintf("%.3f", float64(matchNs)/1000.0),
				ts.Format("2006-01-02 15:04:05.000"),
			})
		}
		cw.Flush()
	}
}
