CREATE TABLE IF NOT EXISTS food_orders (
    order_id TEXT PRIMARY KEY,
    idempotency_key TEXT NOT NULL UNIQUE,
    request_hash TEXT NOT NULL,
    customer_id TEXT NOT NULL,
    restaurant_id TEXT NOT NULL,
    items JSONB NOT NULL,
    delivery_address JSONB NOT NULL DEFAULT '{}'::jsonb,
    total_amount_minor BIGINT NOT NULL CHECK (total_amount_minor >= 0),
    currency CHAR(3) NOT NULL DEFAULT 'INR',
    status TEXT NOT NULL DEFAULT 'PENDING',
    failure_reason TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_food_orders_customer_created ON food_orders(customer_id, created_at DESC);
CREATE TABLE IF NOT EXISTS food_order_events (
    event_id BIGSERIAL PRIMARY KEY,
    order_id TEXT NOT NULL REFERENCES food_orders(order_id),
    from_status TEXT NOT NULL,
    to_status TEXT NOT NULL,
    reason TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE TABLE IF NOT EXISTS food_order_outbox (
    outbox_id BIGSERIAL PRIMARY KEY,
    event_type TEXT NOT NULL,
    aggregate_id TEXT NOT NULL,
    payload JSONB NOT NULL,
    attempts INT NOT NULL DEFAULT 0,
    last_error TEXT NOT NULL DEFAULT '',
    locked_until TIMESTAMPTZ,
    dispatched_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_food_order_outbox_pending ON food_order_outbox(dispatched_at, locked_until, outbox_id);

ALTER TABLE food_order_outbox ADD COLUMN IF NOT EXISTS aggregatetype TEXT NOT NULL DEFAULT 'food.order.events';

-- Backfill legacy rows so the initial Debezium snapshot uses the consumer envelope.
UPDATE food_order_outbox
SET payload = CASE
 WHEN event_type = 'START_ORDER_WORKFLOW' THEN jsonb_build_object('event_type', event_type, 'workflow_input', payload)
 WHEN event_type = 'SIGNAL_CANCEL' THEN jsonb_build_object('event_type', event_type, 'signal', payload)
 ELSE jsonb_build_object('event_type', event_type, 'legacy_payload', payload)
END
WHERE NOT (payload ? 'event_type');
