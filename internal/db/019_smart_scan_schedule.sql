-- Migration 019: add smart_scan type to scheduled_jobs and source tracking on scan_jobs.
--
-- 1. Extend the scheduled_jobs.type constraint to accept 'smart_scan'.
-- 2. Add a source column to scan_jobs to distinguish API-, auto-, and scheduler-triggered scans.

-- Drop and recreate the type check constraint to include 'smart_scan'.
ALTER TABLE scheduled_jobs
    DROP CONSTRAINT IF EXISTS scheduled_jobs_type_check;

ALTER TABLE scheduled_jobs
    ADD CONSTRAINT scheduled_jobs_type_check
        CHECK (type IN ('discovery', 'scan', 'smart_scan'));

-- Add source to scan_jobs (uses the existing created_by column for backwards
-- compat; source is a new separate concept).
ALTER TABLE scan_jobs
    ADD COLUMN IF NOT EXISTS source VARCHAR(50);

COMMENT ON COLUMN scan_jobs.source IS
    'Origin of the scan: api (user-triggered), auto (post-scan auto-progression), scheduled (cron job)';

CREATE INDEX IF NOT EXISTS idx_scan_jobs_source ON scan_jobs (source) WHERE source IS NOT NULL;
