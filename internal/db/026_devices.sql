-- Migration 026: Device identity
--
-- Introduces a stable device concept above raw host records.
-- A device survives MAC randomization and IP churn by accumulating known MACs
-- and names across multiple host sightings.
--
-- Four new tables:
--   devices              — stable identity record
--   device_known_macs    — every MAC address ever seen for a device
--   device_known_names   — every hostname ever seen, by source
--   device_suggestions   — low-confidence match candidates for user review
--
-- hosts gains two nullable columns:
--   device_id  — FK to devices(id), NULL when unidentified
--   mdns_name  — most recently resolved mDNS .local name for this specific host/IP

CREATE TABLE devices (
    id         UUID         PRIMARY KEY DEFAULT uuid_generate_v4(),
    name       VARCHAR(255) NOT NULL,
    notes      TEXT,
    created_at TIMESTAMPTZ  DEFAULT NOW(),
    updated_at TIMESTAMPTZ  DEFAULT NOW()
);

-- One row per MAC address ever seen for a device.
-- UNIQUE on mac_address: a MAC can only belong to one device at a time.
CREATE TABLE device_known_macs (
    id          UUID        PRIMARY KEY DEFAULT uuid_generate_v4(),
    device_id   UUID        NOT NULL REFERENCES devices(id) ON DELETE CASCADE,
    mac_address MACADDR     NOT NULL,
    first_seen  TIMESTAMPTZ DEFAULT NOW(),
    last_seen   TIMESTAMPTZ DEFAULT NOW(),
    CONSTRAINT uq_device_mac UNIQUE (mac_address)
);

-- Stable names ever associated with a device.
-- source must be one of: mdns, dns, snmp, netbios, user
-- UNIQUE on (name, source): same name from same source is one row.
CREATE TABLE device_known_names (
    id         UUID        PRIMARY KEY DEFAULT uuid_generate_v4(),
    device_id  UUID        NOT NULL REFERENCES devices(id) ON DELETE CASCADE,
    name       TEXT        NOT NULL,
    source     VARCHAR(20) NOT NULL
                   CHECK (source IN ('mdns', 'dns', 'snmp', 'netbios', 'user')),
    first_seen TIMESTAMPTZ DEFAULT NOW(),
    last_seen  TIMESTAMPTZ DEFAULT NOW(),
    CONSTRAINT uq_device_name UNIQUE (name, source)
);

-- Low-confidence match candidates surfaced for user review.
-- dismissed = TRUE means the user explicitly declined the match.
CREATE TABLE device_suggestions (
    id                UUID    PRIMARY KEY DEFAULT uuid_generate_v4(),
    host_id           UUID    NOT NULL REFERENCES hosts(id) ON DELETE CASCADE,
    device_id         UUID    NOT NULL REFERENCES devices(id) ON DELETE CASCADE,
    confidence_score  INTEGER NOT NULL,
    confidence_reason TEXT,
    dismissed         BOOLEAN DEFAULT FALSE,
    created_at        TIMESTAMPTZ DEFAULT NOW(),
    CONSTRAINT uq_suggestion UNIQUE (host_id, device_id)
);

-- hosts gains a device FK and an mDNS name cache.
-- device_id = NULL means unidentified (the common case on first sight).
-- mdns_name caches the last resolved .local name for this specific host/IP;
-- durable names go into device_known_names.
ALTER TABLE hosts
    ADD COLUMN device_id UUID REFERENCES devices(id) ON DELETE SET NULL,
    ADD COLUMN mdns_name TEXT;

-- Index for fast "which hosts belong to device D?" lookups.
CREATE INDEX idx_hosts_device_id ON hosts(device_id) WHERE device_id IS NOT NULL;

COMMENT ON TABLE devices IS
    'Stable device identity that survives MAC randomization and IP churn. '
    'Hosts are attached via hosts.device_id FK.';

COMMENT ON TABLE device_known_macs IS
    'Every MAC address ever observed for a device, with first/last seen timestamps. '
    'A MAC can only belong to one device (UNIQUE constraint).';

COMMENT ON TABLE device_known_names IS
    'Every hostname observed for a device, keyed by (name, source). '
    'source is one of: mdns, dns, snmp, netbios, user.';

COMMENT ON TABLE device_suggestions IS
    'Low-confidence host↔device match candidates produced by DeviceMatcher. '
    'Users accept or dismiss these via the API; accepted suggestions attach the host.';
