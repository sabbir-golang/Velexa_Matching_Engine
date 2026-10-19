-- Live market kept across restarts (internal/database/marketstate).

-- Saved states, one row each: 'market' is the engine snapshot (order books in
-- queue order, prices, the tape, counters, phase); 'fix_outbox' holds reports
-- waiting for brokers that are logged off. An unreadable state is renamed
-- '<name>.bad-<time>' and kept for inspection.
CREATE TABLE engine_state (
    name     text PRIMARY KEY,
    saved_at timestamptz NOT NULL,
    payload  jsonb NOT NULL
);

-- Every execution, with both sides. Never trimmed.
CREATE TABLE trade_log (
    trade_id      bigint PRIMARY KEY,          -- trade number (FIX 880 / ITCH match number)
    day           date NOT NULL,               -- Dhaka trading day
    at            timestamptz NOT NULL,
    symbol        text NOT NULL,
    price         numeric(20, 4) NOT NULL,
    qty           bigint NOT NULL,
    aggressor     text NOT NULL,               -- BUY or SELL: the side that took liquidity
    buy_actor     text NOT NULL DEFAULT '',    -- FIX:<CompID>, USER:<name>, …
    buy_order     bigint NOT NULL DEFAULT 0,   -- engine order number
    buy_cl_ord_id text NOT NULL DEFAULT '',
    sell_actor    text NOT NULL DEFAULT '',
    sell_order    bigint NOT NULL DEFAULT 0,
    sell_cl_ord_id text NOT NULL DEFAULT ''
);

CREATE INDEX trade_log_day ON trade_log (day, trade_id);
CREATE INDEX trade_log_day_symbol ON trade_log (day, symbol);
