-- Migration 007: host_timeout_events — per-host timeout event log
--
-- Records each discovery run in which a host failed to respond.  Unlike
-- host_status_events (which only fires on status transitions), a row is
-- inserted here on every run where a host is absent from the discovered set,
-- regardless of whether the status changed.  This lets the frontend show a
-- precise timeout count and a "last timed out at" timestamp without having to
-- reconstruct it from the status-change log.

CREATE TABLE IF NOT EXISTS host_timeout_events (
    id               UUID        PRIMARY KEY DEFAULT uuid_generate_v4(),
    host_id          UUID        NOT NULL REFERENCES hosts(id) ON DELETE CASCADE,
    source           VARCHAR(50) NOT NULL DEFAULT 'discovery'
                                 CHECK (source IN ('discovery', 'scan')),
    discovery_run_id UUID        REFERENCES discovery_jobs(id) ON DELETE SET NULL,
    recorded_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

COMMENT ON TABLE  host_timeout_events              IS 'One row per host per discovery run in which the host did not respond';
COMMENT ON COLUMN host_timeout_events.source       IS 'Context that produced the timeout: discovery or scan';
COMMENT ON COLUMN host_timeout_events.discovery_run_id IS 'Discovery job that triggered this event; NULL for scan-sourced timeouts';

CREATE INDEX IF NOT EXISTS idx_host_timeout_events_host_id
    ON host_timeout_events (host_id, recorded_at DESC);

CREATE INDEX IF NOT EXISTS idx_host_timeout_events_run_id
    ON host_timeout_events (discovery_run_id)
    WHERE discovery_run_id IS NOT NULL;
