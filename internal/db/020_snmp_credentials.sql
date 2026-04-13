CREATE TABLE IF NOT EXISTS snmp_credentials (
    id         UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    network_id UUID        REFERENCES networks(id) ON DELETE CASCADE,
    version    VARCHAR(4)  NOT NULL DEFAULT 'v2c' CHECK (version IN ('v2c', 'v3')),
    -- SNMPv2c
    community  TEXT,
    -- SNMPv3
    username   TEXT,
    auth_proto VARCHAR(8)  CHECK (auth_proto IS NULL OR auth_proto IN ('MD5', 'SHA')),
    auth_pass  TEXT,
    priv_proto VARCHAR(8)  CHECK (priv_proto IS NULL OR priv_proto IN ('DES', 'AES')),
    priv_pass  TEXT,
    -- Metadata
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    -- One credential set per scope (global=NULL network_id) per version
    UNIQUE NULLS NOT DISTINCT (network_id, version)
);
