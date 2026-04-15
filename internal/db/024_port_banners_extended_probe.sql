-- Migration 024: track whether extended protocol probing has been attempted
-- for a port. Set once per (host_id, port) combination; never reset.

ALTER TABLE port_banners
    ADD COLUMN IF NOT EXISTS extended_probe_done BOOLEAN NOT NULL DEFAULT FALSE;
