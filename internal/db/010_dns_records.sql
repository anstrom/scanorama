-- Migration 010: host DNS records
-- Stores per-host DNS records collected during enrichment (PTR, A, AAAA, MX, TXT, SRV).
-- Each row is one resolved record value. Multiple rows per (host_id, record_type) are normal.

CREATE TABLE IF NOT EXISTS host_dns_records (
    id           UUID         PRIMARY KEY DEFAULT gen_random_uuid(),
    host_id      UUID         NOT NULL REFERENCES hosts(id) ON DELETE CASCADE,
    record_type  VARCHAR(10)  NOT NULL,  -- PTR, A, AAAA, MX, TXT, SRV, CNAME
    value        TEXT         NOT NULL,
    ttl          INT,
    resolved_at  TIMESTAMPTZ  NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_host_dns_records_host ON host_dns_records(host_id);
