CREATE TABLE IF NOT EXISTS drivers (
    driver_id TEXT PRIMARY KEY,
    status TEXT NOT NULL DEFAULT 'AVAILABLE',
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE TABLE IF NOT EXISTS driver_assignments (
    order_id TEXT PRIMARY KEY,
    driver_id TEXT NOT NULL REFERENCES drivers(driver_id),
    restaurant_id TEXT NOT NULL DEFAULT '',
    status TEXT NOT NULL DEFAULT 'ASSIGNED',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_drivers_available ON drivers(status,driver_id);
CREATE INDEX IF NOT EXISTS idx_assignments_driver_status ON driver_assignments(driver_id,status);
CREATE TABLE IF NOT EXISTS driver_signal_outbox (
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
CREATE INDEX IF NOT EXISTS idx_driver_signal_outbox_pending ON driver_signal_outbox(dispatched_at,locked_until,outbox_id);
