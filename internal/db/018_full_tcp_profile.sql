-- Migration 018: add Full TCP built-in scan profile
-- Scans all 65535 TCP ports using a SYN scan with normal timing.
-- Intended for deep investigation of a single host or a small, targeted network
-- where thorough port coverage matters more than speed.
-- Requires raw socket access (root / CAP_NET_RAW).

INSERT INTO scan_profiles (id, name, description, ports, scan_type, timing, built_in)
VALUES (
    'template-full-tcp',
    'Full TCP',
    'Scans all 65535 TCP ports. Use for thorough host investigation where complete port coverage is needed. Requires root/CAP_NET_RAW. Slower than targeted profiles — prefer a targeted profile for routine scans.',
    '1-65535',
    'syn',
    'normal',
    TRUE
)
ON CONFLICT (id) DO NOTHING;
