-- Migration 002: allow host-targeted scans to omit a networks row.
--
-- A "network" is defined as a CIDR range with a prefix length strictly less
-- than 32 (IPv4) or 128 (IPv6).  Single-host targets (/32 or /128) must not
-- create rows in the networks table, so scan_jobs.network_id is made nullable.

ALTER TABLE scan_jobs ALTER COLUMN network_id DROP NOT NULL;
