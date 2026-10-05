-- Migration 031: Host notes
--
-- Free-form, user-written notes for a host, edited from the host detail
-- panel. Nullable: NULL means the host has no notes.

ALTER TABLE hosts
    ADD COLUMN IF NOT EXISTS notes TEXT;
