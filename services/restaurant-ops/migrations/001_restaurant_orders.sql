CREATE TABLE IF NOT EXISTS restaurant_orders (
    order_id TEXT PRIMARY KEY,
    restaurant_id TEXT NOT NULL,
    customer_id TEXT NOT NULL DEFAULT '',
    items JSONB NOT NULL DEFAULT '[]'::jsonb,
    status TEXT NOT NULL DEFAULT 'PENDING',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_restaurant_orders_restaurant_status ON restaurant_orders(restaurant_id,status,created_at);
CREATE TABLE IF NOT EXISTS restaurant_signal_outbox (
    outbox_id BIGSERIAL PRIMARY KEY,
    order_id TEXT NOT NULL,
    signal_name TEXT NOT NULL,
    payload JSONB NOT NULL,
    attempts INT NOT NULL DEFAULT 0,
    last_error TEXT NOT NULL DEFAULT '',
    locked_until TIMESTAMPTZ,
    dispatched_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_restaurant_signal_outbox_pending ON restaurant_signal_outbox(dispatched_at,locked_until,outbox_id);
