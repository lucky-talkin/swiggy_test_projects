-- Explicit local-development fixture. Never run automatically in service startup.
INSERT INTO drivers (driver_id, status)
VALUES
    ('driver-demo-001', 'AVAILABLE'),
    ('driver-demo-002', 'AVAILABLE'),
    ('driver-demo-003', 'AVAILABLE')
ON CONFLICT (driver_id) DO NOTHING;
