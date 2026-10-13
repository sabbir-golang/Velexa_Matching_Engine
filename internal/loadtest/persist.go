package loadtest

import (
	"context"
	"sync/atomic"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"velexa-sim-me/internal/latency"
)

type simOrderRow struct {
	id             int64
	clOrdID        string
	account        string
	symbol         string
	side           string
	qty            int64
	price          float64
	status         string
	reason         string
	rmsNs, matchNs int32
}

type simExecRow struct {
	execID, orderID int64
	symbol, side    string
	lastPx          float64
	lastQty, cumQty int64
	status          string
}

type persistItem struct {
	ord   simOrderRow
	execs []simExecRow
}

var ordCols = []string{"id", "cl_ord_id", "account", "symbol", "side", "qty", "price", "status", "reject_reason", "rms_ns", "match_ns"}

var exCols = []string{"exec_id", "order_id", "symbol", "side", "last_px", "last_qty", "cum_qty", "status"}

// Persister drains persist items and writes them to Postgres in batches using
// the COPY protocol (UNLOGGED tables) — the realistic durable-store stage.
type Persister struct {
	pool *pgxpool.Pool
	in   chan persistItem
	stat *latency.Stat
	cnt  *int64
	done chan struct{}
}

func newPersister(pool *pgxpool.Pool, stat *latency.Stat, cnt *int64) *Persister {
	return &Persister{pool: pool, in: make(chan persistItem, 8192), stat: stat, cnt: cnt, done: make(chan struct{})}
}

func (p *Persister) run() {
	ordBuf := make([][]any, 0, 4096)
	exBuf := make([][]any, 0, 4096)
	flush := func() {
		if len(ordBuf) == 0 && len(exBuf) == 0 {
			return
		}
		t0 := time.Now()
		ctx := context.Background()
		if len(ordBuf) > 0 {
			p.pool.CopyFrom(ctx, pgx.Identifier{"sim_orders"}, ordCols, pgx.CopyFromRows(ordBuf))
		}
		if len(exBuf) > 0 {
			p.pool.CopyFrom(ctx, pgx.Identifier{"sim_execs"}, exCols, pgx.CopyFromRows(exBuf))
		}
		dur := time.Since(t0).Nanoseconds()
		n := len(ordBuf) + len(exBuf)
		if n > 0 {
			p.stat.RecBatch(int64(n), dur) // amortized per-row store time
		}
		atomic.AddInt64(p.cnt, int64(len(ordBuf)))
		ordBuf = ordBuf[:0]
		exBuf = exBuf[:0]
	}
	ticker := time.NewTicker(40 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case it, ok := <-p.in:
			if !ok {
				flush()
				close(p.done)
				return
			}
			o := it.ord
			ordBuf = append(ordBuf, []any{o.id, o.clOrdID, o.account, o.symbol, o.side, o.qty, o.price, o.status, o.reason, o.rmsNs, o.matchNs})
			for _, e := range it.execs {
				exBuf = append(exBuf, []any{e.execID, e.orderID, e.symbol, e.side, e.lastPx, e.lastQty, e.cumQty, e.status})
			}
			if len(ordBuf) >= 3000 || len(exBuf) >= 3000 {
				flush()
			}
		case <-ticker.C:
			flush()
		}
	}
}
