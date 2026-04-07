-- Migration 006: per-host scan duration in port_scans
--
-- Records how long nmap spent scanning a particular host (start → end of the
-- per-host XML block).  The value is derived from nmap's per-host timestamps
-- and stored alongside the port results so the API can surface it without a
-- separate round-trip.

ALTER TABLE port_scans
    ADD COLUMN IF NOT EXISTS scan_duration_ms INTEGER;

COMMENT ON COLUMN port_scans.scan_duration_ms IS
    'Wall-clock milliseconds nmap spent scanning this host in this job, '
    'derived from the per-host start/end timestamps in the nmap XML output. '
    'NULL when nmap did not report per-host timing (e.g. host was down).';
