-- Daily report: one row per trading day (Dhaka date) and order source.
-- actor is "FIX:<CompID>", "USER:<name>", or "*" for the exchange totals.

CREATE TABLE daily_stats (
    day         date   NOT NULL,
    actor       text   NOT NULL,
    buy_orders  bigint NOT NULL DEFAULT 0,
    sell_orders bigint NOT NULL DEFAULT 0,
    accepted    bigint NOT NULL DEFAULT 0,
    rejected    bigint NOT NULL DEFAULT 0,
    cancelled   bigint NOT NULL DEFAULT 0,
    amended     bigint NOT NULL DEFAULT 0,
    expired     bigint NOT NULL DEFAULT 0,
    trades      bigint NOT NULL DEFAULT 0,
    volume      bigint NOT NULL DEFAULT 0,
    turnover    numeric(20, 2) NOT NULL DEFAULT 0,
    updated_at  timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (day, actor)
);
