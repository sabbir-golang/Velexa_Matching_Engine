package marketstate

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"velexa-sim-me/internal/engine"
)

// PGStore keeps the states in engine_state and the trade log in trade_log.
type PGStore struct{ pool *pgxpool.Pool }

// NewPGStore uses pool's engine_state and trade_log tables.
func NewPGStore(pool *pgxpool.Pool) *PGStore { return &PGStore{pool: pool} }

func timeout() (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.Background(), 10*time.Second)
}

func (p *PGStore) SaveState(name string, payload []byte) error {
	ctx, cancel := timeout()
	defer cancel()
	_, err := p.pool.Exec(ctx, `INSERT INTO engine_state (name, saved_at, payload) VALUES ($1, now(), $2::jsonb)
		ON CONFLICT (name) DO UPDATE SET saved_at = EXCLUDED.saved_at, payload = EXCLUDED.payload`, name, string(payload))
	return err
}

func (p *PGStore) LoadState(name string) ([]byte, error) {
	ctx, cancel := timeout()
	defer cancel()
	var s string
	err := p.pool.QueryRow(ctx, `SELECT payload::text FROM engine_state WHERE name = $1`, name).Scan(&s)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return []byte(s), nil
}

func (p *PGStore) SetAside(name string) (string, error) {
	ctx, cancel := timeout()
	defer cancel()
	to := name + ".bad-" + time.Now().Format("20060102-150405")
	_, err := p.pool.Exec(ctx, `UPDATE engine_state SET name = $2 WHERE name = $1`, name, to)
	return "engine_state row " + to, err
}

func (p *PGStore) AppendTrades(trades []engine.TradeRecord) error {
	ctx, cancel := timeout()
	defer cancel()
	b := &pgx.Batch{}
	for _, t := range trades {
		b.Queue(`INSERT INTO trade_log (trade_id, day, at, symbol, price, qty, aggressor,
				buy_actor, buy_order, buy_cl_ord_id, sell_actor, sell_order, sell_cl_ord_id)
			VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13) ON CONFLICT (trade_id) DO NOTHING`,
			int64(t.ID), engine.ReportDay(t.Time), t.Time, t.Symbol, t.Price, t.Qty, string(t.Aggressor),
			t.Buy.Actor, int64(t.Buy.OrderID), t.Buy.ClOrdID, t.Sell.Actor, int64(t.Sell.OrderID), t.Sell.ClOrdID)
	}
	return p.pool.SendBatch(ctx, b).Close()
}

func (p *PGStore) Trades(day string) ([]engine.TradeRecord, error) {
	ctx, cancel := timeout()
	defer cancel()
	rows, err := p.pool.Query(ctx, `SELECT trade_id, at, symbol, price::float8, qty, aggressor,
		buy_actor, buy_order, buy_cl_ord_id, sell_actor, sell_order, sell_cl_ord_id
		FROM trade_log WHERE day = $1 ORDER BY trade_id`, day)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []engine.TradeRecord
	for rows.Next() {
		var t engine.TradeRecord
		var id, bo, so int64
		var aggr string
		if err := rows.Scan(&id, &t.Time, &t.Symbol, &t.Price, &t.Qty, &aggr,
			&t.Buy.Actor, &bo, &t.Buy.ClOrdID, &t.Sell.Actor, &so, &t.Sell.ClOrdID); err != nil {
			return nil, err
		}
		t.ID, t.Buy.OrderID, t.Sell.OrderID, t.Aggressor = uint64(id), uint64(bo), uint64(so), engine.Side(aggr)
		out = append(out, t)
	}
	return out, rows.Err()
}

func (p *PGStore) MaxTradeID() (uint64, error) {
	ctx, cancel := timeout()
	defer cancel()
	var n int64
	err := p.pool.QueryRow(ctx, `SELECT COALESCE(max(trade_id), 0) FROM trade_log`).Scan(&n)
	return uint64(n), err
}
