-- Migration 003: link discovery_jobs to the registered networks table.
--
-- Adds a nullable network_id FK so that discovery jobs created from the
-- network detail view are associated with their parent network.  A trigger
-- stamps networks.last_discovery whenever a linked job reaches a terminal
-- state (completed or failed).

ALTER TABLE discovery_jobs
    ADD COLUMN network_id UUID REFERENCES networks(id) ON DELETE SET NULL;

CREATE INDEX IF NOT EXISTS idx_discovery_jobs_network_id ON discovery_jobs (network_id);

-- Trigger function: update networks.last_discovery when a linked discovery
-- job transitions to 'completed' or 'failed'.
CREATE OR REPLACE FUNCTION stamp_network_last_discovery()
RETURNS TRIGGER AS $$
BEGIN
    IF NEW.network_id IS NOT NULL
       AND NEW.status IN ('completed', 'failed')
       AND (OLD.status IS DISTINCT FROM NEW.status)
    THEN
        UPDATE networks
        SET last_discovery = NOW()
        WHERE id = NEW.network_id;
    END IF;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER trg_stamp_network_last_discovery
AFTER UPDATE ON discovery_jobs
FOR EACH ROW EXECUTE FUNCTION stamp_network_last_discovery();

COMMENT ON COLUMN discovery_jobs.network_id IS
    'FK to the registered network this job was triggered from; NULL for ad-hoc jobs';
