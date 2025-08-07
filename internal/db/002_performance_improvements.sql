-- Migration 002: Performance and Consistency Improvements
-- This migration adds performance indexes, data validation constraints,
-- and enhanced tracking fields to improve database efficiency and data integrity.

-- Add execution tracking fields to scan_jobs for better monitoring
ALTER TABLE scan_jobs ADD COLUMN IF NOT EXISTS progress_percent INTEGER DEFAULT 0;
ALTER TABLE scan_jobs ADD COLUMN IF NOT EXISTS timeout_at TIMESTAMPTZ;
ALTER TABLE scan_jobs ADD COLUMN IF NOT EXISTS execution_details JSONB;
ALTER TABLE scan_jobs ADD COLUMN IF NOT EXISTS worker_id VARCHAR(100);

-- Add audit fields to core tables
ALTER TABLE scan_targets ADD COLUMN IF NOT EXISTS created_by VARCHAR(100);
ALTER TABLE scan_targets ADD COLUMN IF NOT EXISTS modified_by VARCHAR(100);
ALTER TABLE scan_jobs ADD COLUMN IF NOT EXISTS created_by VARCHAR(100);

-- Add data validation constraints
DO $$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'check_job_timing') THEN
        ALTER TABLE scan_jobs ADD CONSTRAINT check_job_timing
            CHECK (completed_at IS NULL OR started_at IS NULL OR completed_at >= started_at);
    END IF;

    IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'check_confidence_range') THEN
        ALTER TABLE hosts ADD CONSTRAINT check_confidence_range
            CHECK (os_confidence IS NULL OR (os_confidence >= 0 AND os_confidence <= 100));
    END IF;

    IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'check_progress_range') THEN
        ALTER TABLE scan_jobs ADD CONSTRAINT check_progress_range
            CHECK (progress_percent >= 0 AND progress_percent <= 100);
    END IF;
END
$$;

-- Performance indexes for common query patterns
CREATE INDEX IF NOT EXISTS idx_port_scans_job_host
    ON port_scans (job_id, host_id);

CREATE INDEX IF NOT EXISTS idx_port_scans_host_port_state
    ON port_scans (host_id, port, state) WHERE state = 'open';

CREATE INDEX IF NOT EXISTS idx_scan_jobs_status_created
    ON scan_jobs (status, created_at);

CREATE INDEX IF NOT EXISTS idx_scan_jobs_target_status
    ON scan_jobs (target_id, status);

CREATE INDEX IF NOT EXISTS idx_hosts_status_last_seen
    ON hosts (status, last_seen) WHERE status = 'up';

CREATE INDEX IF NOT EXISTS idx_hosts_os_family_status
    ON hosts (os_family, status) WHERE os_family IS NOT NULL AND status = 'up';

CREATE INDEX IF NOT EXISTS idx_port_scans_scanned_at
    ON port_scans (scanned_at);

-- Composite index for dashboard queries
CREATE INDEX IF NOT EXISTS idx_hosts_discovery_status
    ON hosts (discovery_method, status, last_seen)
    WHERE discovery_method IS NOT NULL;

-- Index for scheduled job processing
CREATE INDEX IF NOT EXISTS idx_scheduled_jobs_next_run_enabled
    ON scheduled_jobs (next_run, enabled) WHERE enabled = true;

-- Enhance host_history for better audit trail
ALTER TABLE host_history ADD COLUMN IF NOT EXISTS changed_by VARCHAR(100);
ALTER TABLE host_history ADD COLUMN IF NOT EXISTS change_reason TEXT;
ALTER TABLE host_history ADD COLUMN IF NOT EXISTS client_ip INET;

-- Add index for audit queries
CREATE INDEX IF NOT EXISTS idx_host_history_host_created
    ON host_history (host_id, created_at);

-- Create materialized view for dashboard performance
CREATE MATERIALIZED VIEW IF NOT EXISTS host_summary AS
SELECT
    h.id,
    h.ip_address,
    h.hostname,
    h.mac_address,
    h.vendor,
    h.os_family,
    h.os_name,
    h.status,
    h.last_seen,
    h.first_seen,
    h.discovery_count,
    COUNT(ps.id) FILTER (WHERE ps.state = 'open') as open_ports,
    COUNT(ps.id) as total_ports_scanned,
    MAX(ps.scanned_at) as last_scanned,
    COUNT(DISTINCT ps.job_id) as scan_job_count
FROM hosts h
LEFT JOIN port_scans ps ON h.id = ps.host_id
GROUP BY h.id, h.ip_address, h.hostname, h.mac_address, h.vendor,
         h.os_family, h.os_name, h.status, h.last_seen, h.first_seen, h.discovery_count;

-- Add unique index for materialized view
CREATE UNIQUE INDEX IF NOT EXISTS idx_host_summary_id ON host_summary (id);
CREATE INDEX IF NOT EXISTS idx_host_summary_ip ON host_summary (ip_address);
CREATE INDEX IF NOT EXISTS idx_host_summary_status_last_seen ON host_summary (status, last_seen);

-- Create network summary materialized view for reporting
CREATE MATERIALIZED VIEW IF NOT EXISTS network_summary_mv AS
SELECT
    st.id as target_id,
    st.name as target_name,
    st.network,
    st.enabled,
    COUNT(DISTINCT h.id) FILTER (WHERE h.status = 'up') as active_hosts,
    COUNT(DISTINCT h.id) as total_discovered_hosts,
    COUNT(DISTINCT ps.id) FILTER (WHERE ps.state = 'open') as total_open_ports,
    MAX(sj.completed_at) as last_scan_completed,
    COUNT(DISTINCT sj.id) FILTER (WHERE sj.status = 'completed') as completed_scans,
    COUNT(DISTINCT sj.id) FILTER (WHERE sj.status = 'failed') as failed_scans,
    AVG(EXTRACT(EPOCH FROM (sj.completed_at - sj.started_at))) as avg_scan_duration_seconds
FROM scan_targets st
LEFT JOIN scan_jobs sj ON st.id = sj.target_id
LEFT JOIN hosts h ON h.ip_address << st.network
LEFT JOIN port_scans ps ON h.id = ps.host_id AND ps.job_id = sj.id
GROUP BY st.id, st.name, st.network, st.enabled;

-- Add indexes for network summary
CREATE UNIQUE INDEX IF NOT EXISTS idx_network_summary_mv_target_id ON network_summary_mv (target_id);
CREATE INDEX IF NOT EXISTS idx_network_summary_mv_enabled ON network_summary_mv (enabled);

-- Add function to refresh materialized views
CREATE OR REPLACE FUNCTION refresh_summary_views()
RETURNS void AS $$
BEGIN
    REFRESH MATERIALIZED VIEW CONCURRENTLY host_summary;
    REFRESH MATERIALIZED VIEW CONCURRENTLY network_summary_mv;
END;
$$ LANGUAGE plpgsql;

-- Add scheduled job status tracking
ALTER TABLE scheduled_jobs ADD COLUMN IF NOT EXISTS last_run_duration_ms INTEGER;
ALTER TABLE scheduled_jobs ADD COLUMN IF NOT EXISTS last_run_status VARCHAR(20);
ALTER TABLE scheduled_jobs ADD COLUMN IF NOT EXISTS consecutive_failures INTEGER DEFAULT 0;
ALTER TABLE scheduled_jobs ADD COLUMN IF NOT EXISTS max_failures INTEGER DEFAULT 5;

-- Add constraints for new scheduled_jobs columns
DO $$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'check_last_run_status') THEN
        ALTER TABLE scheduled_jobs ADD CONSTRAINT check_last_run_status
            CHECK (last_run_status IN ('success', 'failed', 'timeout', 'cancelled'));
    END IF;
END
$$;

-- Add index for job monitoring
CREATE INDEX IF NOT EXISTS idx_scheduled_jobs_failures
    ON scheduled_jobs (consecutive_failures, enabled)
    WHERE consecutive_failures > 0 AND enabled = true;

-- Improve port scan table for better performance with large datasets
-- Add partial index for active scans only
CREATE INDEX IF NOT EXISTS idx_port_scans_recent
    ON port_scans (host_id, port, state)
    WHERE state = 'open';

-- Add index for service detection queries
CREATE INDEX IF NOT EXISTS idx_port_scans_service_detection
    ON port_scans (service_name, service_version)
    WHERE service_name IS NOT NULL;

-- Add constraint to ensure scan jobs have proper status progression
DO $$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'check_status_timing') THEN
        ALTER TABLE scan_jobs ADD CONSTRAINT check_status_timing
            CHECK (
                (status = 'pending' AND started_at IS NULL) OR
                (status = 'running' AND started_at IS NOT NULL) OR
                (status IN ('completed', 'failed') AND started_at IS NOT NULL)
            );
    END IF;
END
$$;

-- Add function for cleaning old scan data (for maintenance)
CREATE OR REPLACE FUNCTION cleanup_old_scan_data(days_to_keep INTEGER DEFAULT 90)
RETURNS INTEGER AS $$
DECLARE
    deleted_count INTEGER;
BEGIN
    -- Delete old port scans and their related data
    WITH deleted_scans AS (
        DELETE FROM port_scans
        WHERE scanned_at < NOW() - (days_to_keep || ' days')::INTERVAL
        RETURNING job_id
    ),
    deleted_jobs AS (
        DELETE FROM scan_jobs sj
        WHERE sj.id IN (SELECT job_id FROM deleted_scans)
        AND sj.completed_at < NOW() - (days_to_keep || ' days')::INTERVAL
        RETURNING id
    )
    SELECT COUNT(*) INTO deleted_count FROM deleted_jobs;

    -- Refresh materialized views after cleanup
    PERFORM refresh_summary_views();

    RETURN deleted_count;
END;
$$ LANGUAGE plpgsql;

-- Add comments for new fields
COMMENT ON COLUMN scan_jobs.progress_percent IS 'Scan completion percentage (0-100)';
COMMENT ON COLUMN scan_jobs.timeout_at IS 'When the scan job should timeout';
COMMENT ON COLUMN scan_jobs.execution_details IS 'Additional execution context and metadata';
COMMENT ON COLUMN scan_jobs.worker_id IS 'Identifier of worker/process executing the scan';
COMMENT ON COLUMN scheduled_jobs.consecutive_failures IS 'Number of consecutive failures for alerting';
COMMENT ON COLUMN scheduled_jobs.max_failures IS 'Maximum failures before disabling job';

-- Add triggers for automatic audit field updates
CREATE OR REPLACE FUNCTION update_modified_by()
RETURNS TRIGGER AS $$
BEGIN
    NEW.modified_by = COALESCE(NEW.modified_by, 'system');
    NEW.updated_at = NOW();
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

-- Apply audit triggers
DROP TRIGGER IF EXISTS update_scan_targets_audit ON scan_targets;
CREATE TRIGGER update_scan_targets_audit
    BEFORE UPDATE ON scan_targets
    FOR EACH ROW
    EXECUTE FUNCTION update_modified_by();

-- Set default values for existing records
UPDATE scan_targets SET created_by = 'migration' WHERE created_by IS NULL;
UPDATE scan_targets SET modified_by = 'migration' WHERE modified_by IS NULL;
UPDATE scan_jobs SET created_by = 'migration' WHERE created_by IS NULL;

-- Performance statistics view for monitoring
CREATE OR REPLACE VIEW scan_performance_stats AS
SELECT
    DATE_TRUNC('day', sj.created_at) as scan_date,
    COUNT(*) as total_jobs,
    COUNT(*) FILTER (WHERE sj.status = 'completed') as completed_jobs,
    COUNT(*) FILTER (WHERE sj.status = 'failed') as failed_jobs,
    AVG(EXTRACT(EPOCH FROM (sj.completed_at - sj.started_at))) as avg_duration_seconds,
    SUM(COALESCE((sj.scan_stats->>'total_hosts')::INTEGER, 0)) as total_hosts_scanned,
    SUM(COALESCE((sj.scan_stats->>'hosts_up')::INTEGER, 0)) as total_hosts_up
FROM scan_jobs sj
WHERE sj.created_at > NOW() - INTERVAL '30 days'
GROUP BY DATE_TRUNC('day', sj.created_at)
ORDER BY scan_date DESC;

-- Note: Index with DATE_TRUNC removed due to immutable function requirement

COMMENT ON VIEW scan_performance_stats IS 'Daily scan performance metrics for monitoring and reporting';
