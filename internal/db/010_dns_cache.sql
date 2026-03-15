-- DNS cache table for storing both forward (name→IP) and reverse (IP→name) lookups
-- server-side, so repeated resolutions hit the database instead of the resolver.
--
-- Design notes:
--   • "reverse" rows  – PTR lookups:  lookup_key = IP address text  (e.g. "192.168.1.10")
--                                     resolved_value = hostname      (e.g. "host.example.com")
--   • "forward" rows  – A/AAAA lookups: lookup_key = hostname        (e.g. "host.example.com")
--                                       resolved_value = IP address   (e.g. "192.168.1.10")
--
--   A single row represents the *primary* result of one lookup attempt.
--   Multiple A records for a single name are stored as separate rows (same
--   lookup_key, different resolved_value).  Callers should use SELECT … WHERE
--   direction = 'forward' AND lookup_key = $1 to get all IPs for a name.
--
--   The (direction, lookup_key, resolved_value) triple is the natural key so
--   that we can independently refresh each mapping without clobbering others.

CREATE TABLE IF NOT EXISTS dns_cache (
    id             UUID        PRIMARY KEY DEFAULT uuid_generate_v4(),

    -- 'forward' = hostname → IP  |  'reverse' = IP → hostname
    direction      VARCHAR(8)  NOT NULL CHECK (direction IN ('forward', 'reverse')),

    -- What was looked up:
    --   forward  → the hostname / FQDN that was queried
    --   reverse  → the IP address that was queried (stored as text so IPv6 works too)
    lookup_key     TEXT        NOT NULL,

    -- The result of the lookup:
    --   forward  → one of the IP addresses returned
    --   reverse  → the first PTR record returned; empty string means the lookup
    --              succeeded but returned no names (NXDOMAIN / empty answer)
    resolved_value TEXT        NOT NULL DEFAULT '',

    resolved_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    -- How long (seconds) before this entry should be considered stale.
    -- Default 1 hour; callers may override (e.g. shorter TTL for negative results).
    ttl_seconds    INTEGER     NOT NULL DEFAULT 3600 CHECK (ttl_seconds > 0),

    -- NULL on success; non-NULL records the last resolver error so callers can
    -- distinguish "resolved to nothing" from "lookup never attempted / errored".
    last_error     TEXT,

    -- Prevent duplicate (direction, lookup_key, resolved_value) triples so
    -- upserts are idempotent.
    CONSTRAINT uq_dns_cache_entry UNIQUE (direction, lookup_key, resolved_value)
);

-- Efficiently find stale or missing entries for a given key + direction.
CREATE INDEX IF NOT EXISTS idx_dns_cache_lookup
    ON dns_cache (direction, lookup_key);

-- Efficiently find all entries whose TTL has expired (background refresh worker).
CREATE INDEX IF NOT EXISTS idx_dns_cache_resolved_at
    ON dns_cache (resolved_at);

COMMENT ON TABLE  dns_cache                  IS 'Server-side DNS lookup cache for both forward (name→IP) and reverse (IP→name) resolutions';
COMMENT ON COLUMN dns_cache.direction        IS '"forward" for A/AAAA queries, "reverse" for PTR queries';
COMMENT ON COLUMN dns_cache.lookup_key       IS 'The name or IP that was looked up';
COMMENT ON COLUMN dns_cache.resolved_value   IS 'The result of the lookup; empty string for successful but empty responses';
COMMENT ON COLUMN dns_cache.resolved_at      IS 'Wall-clock time of the most recent lookup attempt';
COMMENT ON COLUMN dns_cache.ttl_seconds      IS 'Seconds until this entry is considered stale and should be re-resolved';
COMMENT ON COLUMN dns_cache.last_error       IS 'Resolver error from the last attempt, NULL when the last attempt succeeded';
