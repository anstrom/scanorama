-- Migration 010: SNMP data captured from network devices.
-- Populated by SNMP enrichment when port 161 is open post-scan.

CREATE TABLE IF NOT EXISTS host_snmp_data (
    host_id      UUID        PRIMARY KEY REFERENCES hosts(id) ON DELETE CASCADE,
    sys_name     TEXT,
    sys_descr    TEXT,
    sys_location TEXT,
    sys_contact  TEXT,
    sys_uptime   BIGINT,
    if_count     INT,
    interfaces   JSONB       DEFAULT '[]'::jsonb,
    community    VARCHAR(100),
    collected_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
