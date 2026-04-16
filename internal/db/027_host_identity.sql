-- Migration 027: Host identity resolution
--
-- Adds the plumbing for picking a single "display name" per host from several
-- candidate sources, plus an explicit user-defined override.
--
-- Two nullable columns on hosts:
--   custom_name      — user-defined override. Written only by the UI's
--                      Identity tab. Always wins when non-null; auto-enrichers
--                      never touch it.
--   hostname_source  — provenance tag for the existing hosts.hostname value.
--                      One of manual|ptr|mdns|snmp|cert. Existing non-null
--                      hostnames are backfilled as 'ptr' — historically the
--                      DNS enricher was the only writer.
--
-- Configurable ranking lives on the existing settings table under key
-- identity.rank_order as a JSONB array.

ALTER TABLE hosts
    ADD COLUMN IF NOT EXISTS custom_name     VARCHAR(255),
    ADD COLUMN IF NOT EXISTS hostname_source VARCHAR(32)
        CHECK (hostname_source IS NULL OR hostname_source IN (
            'manual', 'ptr', 'mdns', 'snmp', 'cert'
        ));

-- Backfill provenance for every existing non-null hostname.
UPDATE hosts
   SET hostname_source = 'ptr'
 WHERE hostname IS NOT NULL
   AND hostname <> ''
   AND hostname_source IS NULL;

-- Seed default identity rank order. Operators can edit via the settings API.
INSERT INTO settings (key, value, type, description) VALUES
    ('identity.rank_order',
     '["mdns","snmp","ptr","cert"]',
     'string[]',
     'Ordered list of automatic name sources for display_name resolution')
ON CONFLICT (key) DO NOTHING;

COMMENT ON COLUMN hosts.custom_name IS
    'User-defined display-name override. Wins over any auto-discovered name '
    'when non-null. Never written by enrichers.';

COMMENT ON COLUMN hosts.hostname_source IS
    'Provenance for hosts.hostname: one of manual|ptr|mdns|snmp|cert. '
    'NULL means unknown/legacy.';
