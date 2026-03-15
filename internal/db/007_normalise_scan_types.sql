-- Migration 007: Normalise scan types to match supported nmap techniques
--
-- Background: migrations 001 and 005 allowed 'version' and 'stealth' as scan
-- types. The scanning engine never had proper nmap equivalents for these:
--   'version'  was just -sV with no scan technique (defaulted to connect)
--   'stealth'  was connect + polite timing, not a real -sS SYN scan
--
-- The valid set is now aligned with actual nmap scan techniques:
--   connect  -sT  TCP connect (no root required)
--   syn      -sS  SYN stealth (requires root)
--   ack      -sA  ACK scan (firewall mapping)
--   udp      -sU  UDP scan
--   aggressive    -sS -sV -A
--   comprehensive -sS -sV --script=default
--
-- Remap strategy:
--   version  → aggressive  (version detection is a subset of aggressive)
--   stealth  → syn         (syn is the real nmap stealth scan)
-- All other legacy types from 005 that the engine never handled
-- (window, null, fin, xmas) → connect as a safe fallback.

-- ── 1. Remap data before touching constraints ────────────────────────────────

UPDATE scan_targets
SET scan_type = CASE scan_type
    WHEN 'version' THEN 'aggressive'
    WHEN 'stealth' THEN 'syn'
    WHEN 'window'  THEN 'connect'
    WHEN 'null'    THEN 'connect'
    WHEN 'fin'     THEN 'connect'
    WHEN 'xmas'    THEN 'connect'
    ELSE scan_type
END
WHERE scan_type IN ('version', 'stealth', 'window', 'null', 'fin', 'xmas');

UPDATE scan_profiles
SET scan_type = CASE scan_type
    WHEN 'version' THEN 'aggressive'
    WHEN 'stealth' THEN 'syn'
    WHEN 'window'  THEN 'connect'
    WHEN 'null'    THEN 'connect'
    WHEN 'fin'     THEN 'connect'
    WHEN 'xmas'    THEN 'connect'
    ELSE scan_type
END
WHERE scan_type IN ('version', 'stealth', 'window', 'null', 'fin', 'xmas');

-- ── 2. Replace CHECK constraint on scan_targets ──────────────────────────────

ALTER TABLE scan_targets DROP CONSTRAINT IF EXISTS scan_targets_scan_type_check;

ALTER TABLE scan_targets
    ADD CONSTRAINT scan_targets_scan_type_check
    CHECK (scan_type IN ('connect', 'syn', 'ack', 'udp', 'aggressive', 'comprehensive'));

-- ── 3. Replace CHECK constraint on scan_profiles ─────────────────────────────

-- 005 created this constraint dynamically; drop by name if it exists.
ALTER TABLE scan_profiles DROP CONSTRAINT IF EXISTS scan_profiles_scan_type_check;

ALTER TABLE scan_profiles
    ADD CONSTRAINT scan_profiles_scan_type_check
    CHECK (scan_type IN ('connect', 'syn', 'ack', 'udp', 'aggressive', 'comprehensive'));

-- ── 4. Update the is_valid_scan_type helper function ─────────────────────────

CREATE OR REPLACE FUNCTION is_valid_scan_type(scan_type_input TEXT)
RETURNS BOOLEAN AS $$
BEGIN
    RETURN scan_type_input IN ('connect', 'syn', 'ack', 'udp', 'aggressive', 'comprehensive');
END;
$$ LANGUAGE plpgsql IMMUTABLE;

-- ── 5. Update column comments ─────────────────────────────────────────────────

COMMENT ON COLUMN scan_targets.scan_type IS
    'Nmap scan technique: connect (-sT), syn (-sS), ack (-sA), udp (-sU), aggressive (-sS -sV -A), comprehensive (-sS -sV --script=default)';

COMMENT ON COLUMN scan_profiles.scan_type IS
    'Nmap scan technique: connect (-sT), syn (-sS), ack (-sA), udp (-sU), aggressive (-sS -sV -A), comprehensive (-sS -sV --script=default)';
