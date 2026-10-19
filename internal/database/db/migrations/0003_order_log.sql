-- Order report: one row per order and trading day, with its latest state.

CREATE TABLE order_log (
    key        text PRIMARY KEY,           -- unique per order (engine run + order number)
    day        date NOT NULL,              -- Dhaka trading day of the order
    entered_at timestamptz NOT NULL,
    updated_at timestamptz NOT NULL,
    seq        bigint NOT NULL DEFAULT 0,  -- engine order number (0: refused before it got one)
    order_id   text NOT NULL DEFAULT '',   -- OrderID (37) as sent to the broker
    actor      text NOT NULL,              -- FIX:<CompID> or USER:<name>
    cl_ord_id  text NOT NULL DEFAULT '',
    client     text NOT NULL DEFAULT '',
    account    text NOT NULL DEFAULT '',
    trader     text NOT NULL DEFAULT '',
    board      text NOT NULL DEFAULT '',
    symbol     text NOT NULL,
    side       text NOT NULL,
    ord_type   text NOT NULL,
    tif        text NOT NULL DEFAULT '',
    qty        bigint NOT NULL,
    price      numeric(20, 4) NOT NULL DEFAULT 0,
    filled     bigint NOT NULL DEFAULT 0,
    avg_px     numeric(20, 4) NOT NULL DEFAULT 0,
    status     text NOT NULL,
    reason     text NOT NULL DEFAULT ''
);

CREATE INDEX order_log_day ON order_log (day, entered_at DESC);
CREATE INDEX order_log_day_actor ON order_log (day, actor);
