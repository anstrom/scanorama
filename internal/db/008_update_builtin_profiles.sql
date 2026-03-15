-- Migration 008: Update built-in profile scan types
--
-- The linux-server and linux-workstation profiles were seeded with scan_type
-- 'version' which migration 007 remapped to 'aggressive'. Aggressive (-sS -sV -A)
-- is too noisy and slow for a default profile. Use 'syn' instead — it's fast,
-- stealthy, and only requires the raw socket privilege we already have via setuid.

UPDATE scan_profiles
SET scan_type = 'syn'
WHERE id IN ('linux-server', 'linux-workstation')
  AND built_in = true;
