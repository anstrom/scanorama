-- Migration 005: Response time min/max/avg tracking on hosts

ALTER TABLE hosts
    ADD COLUMN IF NOT EXISTS response_time_min_ms  INTEGER,
    ADD COLUMN IF NOT EXISTS response_time_max_ms  INTEGER,
    ADD COLUMN IF NOT EXISTS response_time_avg_ms  INTEGER;

COMMENT ON COLUMN hosts.response_time_min_ms IS 'Minimum response time observed across all discovery runs (ms)';
COMMENT ON COLUMN hosts.response_time_max_ms IS 'Maximum response time observed across all discovery runs (ms)';
COMMENT ON COLUMN hosts.response_time_avg_ms IS 'Rolling average response time across all discovery runs (ms)';
