-- Migration 004: Host status model — "gone" status, status transition tracking
--
-- Adds three new columns to hosts, expands the status CHECK constraint to
-- include "gone", and introduces the host_status_events table with a trigger
-- that auto-populates previous_status / status_changed_at on every status change.

-- ── 1. New columns on hosts ───────────────────────────────────────────────────

ALTER TABLE hosts
    ADD COLUMN IF NOT EXISTS status_changed_at  TIMESTAMPTZ,
    ADD COLUMN IF NOT EXISTS previous_status    VARCHAR(20),
    ADD COLUMN IF NOT EXISTS timeout_count      INTEGER NOT NULL DEFAULT 0;

-- ── 2. Expand status CHECK constraint to include "gone" ───────────────────────
--
-- The existing inline CHECK has an auto-generated name we must discover at
-- runtime because it varies across PostgreSQL versions and restore paths.

DO $$
DECLARE
    v_constraint TEXT;
BEGIN
    SELECT conname
      INTO v_constraint
      FROM pg_constraint
     WHERE conrelid = 'hosts'::regclass
       AND contype  = 'c'
       AND pg_get_constraintdef(oid) LIKE '%status%';

    IF v_constraint IS NOT NULL THEN
        EXECUTE format('ALTER TABLE hosts DROP CONSTRAINT %I', v_constraint);
    END IF;
END $$;

ALTER TABLE hosts
    ADD CONSTRAINT hosts_status_check
        CHECK (status IN ('up', 'down', 'unknown', 'gone'));

-- ── 3. host_status_events table ───────────────────────────────────────────────

CREATE TABLE IF NOT EXISTS host_status_events (
    id          UUID        PRIMARY KEY DEFAULT uuid_generate_v4(),
    host_id     UUID        NOT NULL REFERENCES hosts(id) ON DELETE CASCADE,
    from_status VARCHAR(20) NOT NULL,
    to_status   VARCHAR(20) NOT NULL,
    changed_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    source      VARCHAR(50)           -- 'discovery', 'scan', 'api', …
);

CREATE INDEX IF NOT EXISTS idx_host_status_events_host_id
    ON host_status_events (host_id, changed_at DESC);

-- ── 4. Trigger: keep previous_status / status_changed_at in sync ─────────────

CREATE OR REPLACE FUNCTION track_host_status_change()
RETURNS TRIGGER
LANGUAGE plpgsql
AS $$
BEGIN
    -- Capture the transition.
    NEW.previous_status   := OLD.status;
    NEW.status_changed_at := NOW();

    INSERT INTO host_status_events (host_id, from_status, to_status)
    VALUES (NEW.id, OLD.status, NEW.status);

    RETURN NEW;
END;
$$;

-- Drop and recreate so the migration is idempotent.
DROP TRIGGER IF EXISTS host_status_change ON hosts;

CREATE TRIGGER host_status_change
    BEFORE UPDATE OF status ON hosts
    FOR EACH ROW
    WHEN (OLD.status IS DISTINCT FROM NEW.status)
    EXECUTE FUNCTION track_host_status_change();

-- ── 5. Indexes on new columns ─────────────────────────────────────────────────

CREATE INDEX IF NOT EXISTS idx_hosts_status_changed_at
    ON hosts (status_changed_at)
    WHERE status_changed_at IS NOT NULL;

CREATE INDEX IF NOT EXISTS idx_hosts_gone
    ON hosts (last_seen DESC)
    WHERE status = 'gone';

COMMENT ON COLUMN hosts.status_changed_at IS 'Timestamp of the most recent status transition';
COMMENT ON COLUMN hosts.previous_status   IS 'Status value before the most recent transition';
COMMENT ON COLUMN hosts.timeout_count     IS 'Number of consecutive timeouts; reset when host responds';
COMMENT ON TABLE  host_status_events      IS 'Audit log of every host status transition';
