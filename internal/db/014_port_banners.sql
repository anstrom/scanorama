-- Migration 011: port banner and service version records per host/port
-- Populated by banner enrichment after scans with open ports.

CREATE TABLE IF NOT EXISTS port_banners (
    id          UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    host_id     UUID        NOT NULL REFERENCES hosts(id) ON DELETE CASCADE,
    port        INT         NOT NULL,
    protocol    VARCHAR(10) NOT NULL DEFAULT 'tcp',
    raw_banner  TEXT,
    service     VARCHAR(100),
    version     VARCHAR(100),
    scanned_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (host_id, port, protocol)
);

CREATE INDEX IF NOT EXISTS idx_port_banners_host ON port_banners(host_id);
