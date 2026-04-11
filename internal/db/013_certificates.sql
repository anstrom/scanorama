-- Migration 013: TLS certificate records per host/port
-- Populated by banner enrichment after scans with open HTTPS ports.

CREATE TABLE IF NOT EXISTS certificates (
    id          UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    host_id     UUID        NOT NULL REFERENCES hosts(id) ON DELETE CASCADE,
    port        INT         NOT NULL,
    subject_cn  TEXT,
    sans        TEXT[]      DEFAULT '{}',
    issuer      TEXT,
    not_before  TIMESTAMPTZ,
    not_after   TIMESTAMPTZ,
    key_type    VARCHAR(20),
    tls_version VARCHAR(10),
    raw_banner  TEXT,
    scanned_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (host_id, port)
);

CREATE INDEX IF NOT EXISTS idx_certificates_host   ON certificates(host_id);
CREATE INDEX IF NOT EXISTS idx_certificates_expiry ON certificates(not_after);
