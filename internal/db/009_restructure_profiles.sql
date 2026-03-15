-- Migration 009: Restructure built-in scan profiles
--
-- Replaces all previous built-in profiles (from migrations 001 and 005) with a
-- new, consolidated set of six profiles that use nmap's mixed-protocol port
-- syntax (T:<tcp-ports>,U:<udp-ports>). This allows a single profile to drive
-- both TCP and UDP scanning without requiring separate profile entries.
--
-- Profiles removed:
--   TCP profiles (001): windows-server, windows-workstation, linux-server,
--                       linux-workstation, macos-system, generic-default
--   UDP profiles (005): udp-discovery, udp-comprehensive, udp-windows,
--                       udp-linux, udp-infrastructure, udp-fast, udp-voip,
--                       udp-gaming, udp-iot
--
-- Profiles inserted:
--   windows-server, windows-workstation, linux-server, linux-workstation,
--   macos, generic
--
-- scan_jobs.profile_id FK references are NULL-ed out before the deletes so
-- that the foreign-key constraint does not block removal of the old rows.

-- ── 1. Detach any scan_jobs that reference the profiles being removed ─────────

UPDATE scan_jobs
SET profile_id = NULL
WHERE profile_id IN (
    'windows-server',
    'windows-workstation',
    'linux-server',
    'linux-workstation',
    'macos-system',
    'generic-default',
    'udp-discovery',
    'udp-comprehensive',
    'udp-windows',
    'udp-linux',
    'udp-infrastructure',
    'udp-fast',
    'udp-voip',
    'udp-gaming',
    'udp-iot'
);

-- ── 2. Delete the old built-in profiles ──────────────────────────────────────

DELETE FROM scan_profiles
WHERE id IN (
    'windows-server',
    'windows-workstation',
    'linux-server',
    'linux-workstation',
    'macos-system',
    'generic-default',
    'udp-discovery',
    'udp-comprehensive',
    'udp-windows',
    'udp-linux',
    'udp-infrastructure',
    'udp-fast',
    'udp-voip',
    'udp-gaming',
    'udp-iot'
);

-- ── 3. Insert new built-in profiles ──────────────────────────────────────────
--
-- ports format: T:<tcp-ports>,U:<udp-ports>  (no spaces, TCP first)
-- scan_type:    syn  — default TCP technique for all profiles
-- timing:       normal
-- os_detection: stored as JSON boolean in the options JSONB column
-- os_pattern:   empty array
-- scripts:      empty array

INSERT INTO scan_profiles (id, name, description, os_family, os_pattern, ports, scan_type, timing, scripts, options, priority, built_in) VALUES

('windows-server',
 'Windows Server',
 'Comprehensive scan for Windows servers',
 ARRAY['windows'],
 ARRAY[]::TEXT[],
 'T:21,22,25,53,80,110,135,139,143,389,443,445,464,465,587,593,636,1433,3268,3269,3389,5985,5986,8080,8443,49152-49157,U:53,88,123,137,138,161,162,389,445,500,4500',
 'syn',
 'normal',
 ARRAY[]::TEXT[],
 '{"os_detection": false}',
 90,
 true),

('windows-workstation',
 'Windows Workstation',
 'Scan for Windows workstations',
 ARRAY['windows'],
 ARRAY[]::TEXT[],
 'T:80,135,139,443,445,1433,3389,5985,8080,49152-49155,U:123,137,138,500',
 'syn',
 'normal',
 ARRAY[]::TEXT[],
 '{"os_detection": false}',
 80,
 true),

('linux-server',
 'Linux Server',
 'Comprehensive scan for Linux servers',
 ARRAY['linux'],
 ARRAY[]::TEXT[],
 'T:21,22,25,53,80,111,143,443,465,587,2049,3306,5432,6379,8080,8443,9200,9300,27017,U:53,111,123,161,162,514,2049',
 'syn',
 'normal',
 ARRAY[]::TEXT[],
 '{"os_detection": false}',
 90,
 true),

('linux-workstation',
 'Linux Workstation',
 'Scan for Linux workstations',
 ARRAY['linux'],
 ARRAY[]::TEXT[],
 'T:22,80,443,631,5900,8080,U:631,5353',
 'syn',
 'normal',
 ARRAY[]::TEXT[],
 '{"os_detection": false}',
 80,
 true),

('macos',
 'macOS',
 'Scan for macOS systems',
 ARRAY['macos'],
 ARRAY[]::TEXT[],
 'T:22,80,443,445,548,631,3283,5900,7000,8080,U:123,631,5353',
 'syn',
 'normal',
 ARRAY[]::TEXT[],
 '{"os_detection": false}',
 85,
 true),

('generic',
 'Generic',
 'Default scan for unknown OS',
 ARRAY[]::TEXT[],
 ARRAY[]::TEXT[],
 'T:21,22,23,25,53,80,110,143,443,445,3389,8080,8443,U:53,123,161,500',
 'syn',
 'normal',
 ARRAY[]::TEXT[],
 '{"os_detection": false}',
 10,
 true);
