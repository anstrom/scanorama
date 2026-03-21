-- Migration 011: Unify scan_targets into networks
--
-- Resolves: https://github.com/anstrom/scanorama/issues/499
--
-- The scan_targets table served as an ephemeral per-scan CIDR store while the
-- networks table provides persistent network management with discovery and
-- host-count tracking.  This migration merges all scan-specific fields into
-- networks, re-points scan_jobs.target_id to scan_jobs.network_id, and drops
-- the now-redundant scan_targets table.
--
-- Also resolves as part of this migration:
--   #500 - add ON DELETE SET NULL to scan_jobs.profile_id FK
--   #501 - remove duplicate index idx_port_scans_recent
--   #502 - drop stale udp_profile_recommendations view
--   #503 - fix cleanup_expired_api_keys() broken audit_log INSERT
--   #504 - remove sample data seeded in migration 001

-- ============================================================
-- Step 1: Add scan-specific columns to networks
-- ============================================================

ALTER TABLE networks
    ADD COLUMN IF NOT EXISTS scan_interval_seconds INTEGER     NOT NULL DEFAULT 3600,
    ADD COLUMN IF NOT EXISTS scan_ports            TEXT        NOT NULL DEFAULT '22,80,443,8080',
    ADD COLUMN IF NOT EXISTS scan_type             VARCHAR(20) NOT NULL DEFAULT 'connect',
    ADD COLUMN IF NOT EXISTS modified_by           VARCHAR(100);

ALTER TABLE networks
    ADD CONSTRAINT networks_scan_type_check
        CHECK (scan_type IN ('connect', 'syn', 'ack', 'udp', 'aggressive', 'comprehensive'));

-- ============================================================
-- Step 2: Migrate scan_targets rows into networks
--
-- • ON CONFLICT (cidr)  — if a network with the same CIDR already exists,
--   update the scan settings from the scan_target row.
-- • Name collisions     — if another network already claims this name for a
--   different CIDR, fall back to using the CIDR string as the name.
-- ============================================================

INSERT INTO networks (
    id,
    name,
    cidr,
    description,
    discovery_method,
    is_active,
    scan_enabled,
    scan_interval_seconds,
    scan_ports,
    scan_type,
    created_by,
    modified_by,
    created_at,
    updated_at
)
SELECT
    st.id,
    CASE
        WHEN EXISTS (
            SELECT 1 FROM networks n
            WHERE n.name = st.name AND n.cidr != st.network
        )
        THEN host(st.network) || '/' || masklen(st.network)
        ELSE st.name
    END,
    st.network,
    st.description,
    'tcp',
    st.enabled,
    st.enabled,
    st.scan_interval_seconds,
    COALESCE(NULLIF(st.scan_ports, ''), '22,80,443,8080'),
    st.scan_type,
    st.created_by,
    st.modified_by,
    st.created_at,
    st.updated_at
FROM scan_targets st
ON CONFLICT (cidr) DO UPDATE
    SET scan_interval_seconds = EXCLUDED.scan_interval_seconds,
        scan_ports            = EXCLUDED.scan_ports,
        scan_type             = EXCLUDED.scan_type,
        is_active             = EXCLUDED.is_active,
        scan_enabled          = EXCLUDED.scan_enabled,
        modified_by           = EXCLUDED.modified_by;

-- ============================================================
-- Step 3: Add network_id FK to scan_jobs (nullable during backfill)
-- ============================================================

ALTER TABLE scan_jobs
    ADD COLUMN IF NOT EXISTS network_id UUID
        REFERENCES networks(id) ON DELETE CASCADE;

-- ============================================================
-- Step 4: Backfill network_id
--
-- For each scan_job, look up its scan_target by target_id, then find the
-- network whose CIDR matches the scan_target's network column.  This handles
-- both cases:
--   a) The scan_target's UUID was inserted as the networks row (same id).
--   b) The CIDR already existed in networks under a different UUID.
-- ============================================================

UPDATE scan_jobs sj
SET    network_id = n.id
FROM   scan_targets st
JOIN   networks     n  ON n.cidr = st.network
WHERE  sj.target_id = st.id;

-- ============================================================
-- Step 5: Enforce NOT NULL on network_id
-- ============================================================

ALTER TABLE scan_jobs
    ALTER COLUMN network_id SET NOT NULL;

-- ============================================================
-- Step 6: Fix scan_jobs.profile_id FK — add ON DELETE SET NULL (#500)
-- ============================================================

ALTER TABLE scan_jobs
    DROP CONSTRAINT IF EXISTS scan_jobs_profile_id_fkey;

ALTER TABLE scan_jobs
    ADD CONSTRAINT scan_jobs_profile_id_fkey
        FOREIGN KEY (profile_id) REFERENCES scan_profiles(id)
        ON DELETE SET NULL;

-- ============================================================
-- Step 7: Drop the old target_id column and its FK
-- ============================================================

ALTER TABLE scan_jobs
    DROP CONSTRAINT IF EXISTS scan_jobs_target_id_fkey;

ALTER TABLE scan_jobs
    DROP COLUMN IF EXISTS target_id CASCADE;

-- ============================================================
-- Step 8: Drop scan_targets
--   CASCADE removes any remaining dependent views/indexes/triggers.
-- ============================================================

DROP TABLE IF EXISTS scan_targets CASCADE;

-- ============================================================
-- Step 9: Rebuild views that depended on scan_targets
-- ============================================================

-- network_summary view
CREATE OR REPLACE VIEW network_summary AS
SELECT
    n.name                                                              AS target_name,
    n.cidr::text                                                        AS network,
    COUNT(DISTINCT h.id) FILTER (WHERE h.status = 'up')                AS active_hosts,
    COUNT(DISTINCT h.id)                                                AS total_hosts,
    COUNT(DISTINCT ps.id) FILTER (WHERE ps.state = 'open')             AS open_ports,
    MAX(sj.completed_at)                                                AS last_scan
FROM   networks  n
LEFT JOIN scan_jobs   sj ON n.id   = sj.network_id AND sj.status = 'completed'
LEFT JOIN hosts       h  ON h.ip_address << n.cidr
LEFT JOIN port_scans  ps ON h.id   = ps.host_id
WHERE  n.is_active    = true
  AND  n.scan_enabled = true
GROUP  BY n.id, n.name, n.cidr;

-- network_summary_mv materialized view
DROP MATERIALIZED VIEW IF EXISTS network_summary_mv;

CREATE MATERIALIZED VIEW network_summary_mv AS
SELECT
    n.id                                                                               AS target_id,
    n.name                                                                             AS target_name,
    n.cidr::text                                                                       AS network,
    n.is_active                                                                        AS enabled,
    COUNT(DISTINCT sj.id)                                                              AS total_scans,
    COUNT(DISTINCT sj.id) FILTER (WHERE sj.status = 'completed')                      AS completed_scans,
    COUNT(DISTINCT sj.id) FILTER (WHERE sj.status = 'failed')                         AS failed_scans,
    MAX(sj.completed_at)                                                               AS last_scan_at,
    MIN(sj.created_at)                                                                 AS first_scan_at,
    AVG(EXTRACT(EPOCH FROM (sj.completed_at - sj.started_at)))                        AS avg_duration_seconds,
    COUNT(DISTINCT ps.host_id)                                                         AS unique_hosts_scanned
FROM   networks  n
LEFT JOIN scan_jobs   sj ON n.id = sj.network_id
LEFT JOIN port_scans  ps ON sj.id = ps.job_id
GROUP  BY n.id, n.name, n.cidr, n.is_active
WITH DATA;

CREATE UNIQUE INDEX idx_network_summary_mv_target  ON network_summary_mv(target_id);
CREATE INDEX        idx_network_summary_mv_network ON network_summary_mv(network);

-- scan_type_usage_stats view
DROP VIEW IF EXISTS scan_type_usage_stats;

CREATE VIEW scan_type_usage_stats AS
SELECT
    n.scan_type,
    COUNT(DISTINCT n.id)                                           AS target_count,
    COUNT(sj.id)                                                   AS total_jobs,
    COUNT(sj.id) FILTER (WHERE sj.status = 'completed')           AS completed_jobs,
    COUNT(sj.id) FILTER (WHERE sj.status = 'failed')              AS failed_jobs,
    MAX(sj.completed_at)                                           AS last_used
FROM   networks  n
LEFT JOIN scan_jobs sj ON n.id = sj.network_id
GROUP  BY n.scan_type
ORDER  BY total_jobs DESC;

-- ============================================================
-- Step 10: Remove duplicate index on port_scans (#501)
-- ============================================================

DROP INDEX IF EXISTS idx_port_scans_recent;

-- ============================================================
-- Step 11: Drop stale udp_profile_recommendations view (#502)
-- ============================================================

DROP VIEW IF EXISTS udp_profile_recommendations;

-- ============================================================
-- Step 12: Fix cleanup_expired_api_keys() — remove broken INSERT
--   into the nonexistent audit_log table (#503)
-- ============================================================

CREATE OR REPLACE FUNCTION cleanup_expired_api_keys()
RETURNS INTEGER AS $$
DECLARE
    deactivated_count INTEGER;
BEGIN
    UPDATE api_keys
    SET    is_active  = false,
           updated_at = NOW()
    WHERE  is_active    = true
      AND  expires_at IS NOT NULL
      AND  expires_at  < NOW();

    GET DIAGNOSTICS deactivated_count = ROW_COUNT;
    RETURN deactivated_count;
END;
$$ LANGUAGE plpgsql;

-- ============================================================
-- Step 13: Remove sample data seeded in migration 001 (#504)
-- ============================================================

DELETE FROM networks
WHERE (name, cidr::text) IN (
    ('Local Network', '192.168.1.0/24'),
    ('DMZ Network',   '10.0.0.0/8'),
    ('Test Network',  '172.16.0.0/12')
);

-- ============================================================
-- Step 14: Update scheduled_jobs JSONB config
--   The old code stored target_id as an int64 (a known bug; it was
--   never a valid UUID FK).  Rename the key to network_id and clear
--   the value so application code can re-assign it as a proper UUID.
-- ============================================================

UPDATE scheduled_jobs
SET    config = (config - 'target_id') || '{"network_id": null}'::jsonb
WHERE  config ? 'target_id';
