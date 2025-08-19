-- Migration 005: Enhanced Scan Types Support
-- This migration comprehensively updates scan type support to include all standard nmap
-- scan types and adds specialized UDP scan profiles for various network environments.

-- Drop the old restrictive constraint on scan_targets
ALTER TABLE scan_targets DROP CONSTRAINT IF EXISTS scan_targets_scan_type_check;

-- Add the updated constraint that includes all supported scan types
ALTER TABLE scan_targets ADD CONSTRAINT scan_targets_scan_type_check
    CHECK (scan_type IN ('connect', 'syn', 'udp', 'ack', 'window', 'null', 'fin', 'xmas', 'version', 'aggressive', 'comprehensive', 'stealth'));

-- Update scan_profiles table to support all scan types as well
-- First check if there's an existing constraint and drop it
DO $$
BEGIN
    IF EXISTS (
        SELECT 1 FROM pg_constraint
        WHERE conname LIKE '%scan_profiles%scan_type%'
        OR conname LIKE '%scan_type%scan_profiles%'
    ) THEN
        -- Find and drop the constraint dynamically
        EXECUTE (
            SELECT 'ALTER TABLE scan_profiles DROP CONSTRAINT ' || conname
            FROM pg_constraint
            WHERE conrelid = 'scan_profiles'::regclass
            AND contype = 'c'
            AND pg_get_constraintdef(oid) LIKE '%scan_type%'
            LIMIT 1
        );
    END IF;
EXCEPTION
    WHEN others THEN
        -- Continue if constraint doesn't exist or can't be found
        NULL;
END
$$;

-- Add comprehensive scan type constraint to scan_profiles
ALTER TABLE scan_profiles ADD CONSTRAINT scan_profiles_scan_type_check
    CHECK (scan_type IN ('connect', 'syn', 'udp', 'ack', 'window', 'null', 'fin', 'xmas', 'version', 'aggressive', 'comprehensive', 'stealth'));

-- Update any existing records that might have invalid scan types
-- This is mainly precautionary as the application should have been validating these
UPDATE scan_targets
SET scan_type = 'connect'
WHERE scan_type NOT IN ('connect', 'syn', 'udp', 'ack', 'window', 'null', 'fin', 'xmas', 'version', 'aggressive', 'comprehensive', 'stealth');

UPDATE scan_profiles
SET scan_type = 'connect'
WHERE scan_type NOT IN ('connect', 'syn', 'udp', 'ack', 'window', 'null', 'fin', 'xmas', 'version', 'aggressive', 'comprehensive', 'stealth');

-- Add index for scan type queries since they're commonly filtered
CREATE INDEX IF NOT EXISTS idx_scan_targets_scan_type ON scan_targets (scan_type);
CREATE INDEX IF NOT EXISTS idx_scan_profiles_scan_type ON scan_profiles (scan_type);

-- Add comments to document the supported scan types
COMMENT ON COLUMN scan_targets.scan_type IS 'Scan type: connect (basic TCP connect), syn (SYN stealth scan), udp (UDP scan), ack (ACK scan), window (Window scan), null (Null scan), fin (FIN scan), xmas (Xmas scan), version (service version detection), aggressive (OS detection + scripts), comprehensive (full scan with all options), stealth (slow/careful scan)';
COMMENT ON COLUMN scan_profiles.scan_type IS 'Scan type: connect (basic TCP connect), syn (SYN stealth scan), udp (UDP scan), ack (ACK scan), window (Window scan), null (Null scan), fin (FIN scan), xmas (Xmas scan), version (service version detection), aggressive (OS detection + scripts), comprehensive (full scan with all options), stealth (slow/careful scan)';

-- Add UDP-specific scan profiles for different use cases
INSERT INTO scan_profiles (id, name, description, os_family, ports, scan_type, timing, scripts, priority, built_in) VALUES

-- General UDP discovery profile
('udp-discovery', 'UDP Service Discovery', 'Common UDP services discovery scan',
 ARRAY[]::TEXT[], '53,67,68,69,123,161,162,514,1194,1900,5353', 'udp', 'normal',
 ARRAY['dns-service-discovery', 'snmp-info', 'ntp-info'], 75, true),

-- Comprehensive UDP scan for security assessment
('udp-comprehensive', 'UDP Comprehensive Security', 'Extensive UDP port scan for security assessment',
 ARRAY[]::TEXT[], '7,9,13,17,19,37,42,53,67,68,69,111,123,135,137,138,139,161,162,177,427,443,445,500,514,520,623,631,1194,1434,1604,1645,1646,1701,1812,1813,1900,2049,2302,4500,5060,5353,17185,31337',
 'udp', 'normal', ARRAY['dns-service-discovery', 'snmp-info', 'ntp-info', 'dhcp-discover'], 85, true),

-- Windows-specific UDP services
('udp-windows', 'UDP Windows Services', 'UDP scan focused on Windows services',
 ARRAY['windows'], '53,67,68,135,137,138,139,161,445,500,1434,1604,4500',
 'udp', 'normal', ARRAY['smb-os-discovery', 'snmp-win32-services', 'ms-sql-info'], 88, true),

-- Linux/Unix UDP services
('udp-linux', 'UDP Linux/Unix Services', 'UDP scan for common Linux/Unix services',
 ARRAY['linux'], '53,69,111,123,161,162,514,520,623,2049,5353',
 'udp', 'normal', ARRAY['dns-service-discovery', 'nfs-ls', 'snmp-info', 'rpcinfo'], 88, true),

-- Network infrastructure UDP scan
('udp-infrastructure', 'UDP Network Infrastructure', 'UDP scan for network infrastructure devices',
 ARRAY[]::TEXT[], '53,67,68,69,123,161,162,514,520,1812,1813,1900,5060',
 'udp', 'polite', ARRAY['snmp-info', 'dns-service-discovery', 'dhcp-discover', 'ntp-info'], 92, true),

-- Fast UDP scan for basic service detection
('udp-fast', 'UDP Fast Scan', 'Quick UDP scan of most common services',
 ARRAY[]::TEXT[], '53,161,123,1900', 'udp', 'aggressive',
 ARRAY['dns-service-discovery', 'snmp-info'], 70, true),

-- VoIP/SIP specific UDP scanning
('udp-voip', 'UDP VoIP/SIP Services', 'UDP scan targeting VoIP and SIP services',
 ARRAY[]::TEXT[], '53,69,5060,5061,10000-20000', 'udp', 'normal',
 ARRAY['sip-methods', 'sip-enum-users'], 80, true),

-- Gaming and P2P UDP services
('udp-gaming', 'UDP Gaming/P2P', 'UDP scan for gaming and P2P services',
 ARRAY[]::TEXT[], '1194,3478,3479,4380,4500,6112,27015,28910', 'udp', 'normal',
 ARRAY[]::TEXT[], 60, true),

-- IoT and embedded device UDP services
('udp-iot', 'UDP IoT/Embedded', 'UDP scan for IoT and embedded devices',
 ARRAY[]::TEXT[], '53,67,68,69,123,161,162,443,1900,5353,5683,47808', 'udp', 'polite',
 ARRAY['dns-service-discovery', 'snmp-info', 'upnp-info'], 85, true);

-- Create a view to show scan type usage statistics
CREATE OR REPLACE VIEW scan_type_usage_stats AS
SELECT
    st.scan_type,
    COUNT(*) as target_count,
    COUNT(*) FILTER (WHERE st.enabled = true) as enabled_targets,
    COUNT(DISTINCT sj.id) as total_jobs,
    COUNT(DISTINCT sj.id) FILTER (WHERE sj.status = 'completed') as completed_jobs,
    COUNT(DISTINCT sj.id) FILTER (WHERE sj.status = 'failed') as failed_jobs,
    AVG(EXTRACT(EPOCH FROM (sj.completed_at - sj.started_at))) as avg_duration_seconds,
    -- Add profile usage statistics
    (SELECT COUNT(*) FROM scan_profiles sp WHERE sp.scan_type = st.scan_type) as available_profiles
FROM scan_targets st
LEFT JOIN scan_jobs sj ON st.id = sj.target_id
GROUP BY st.scan_type
ORDER BY target_count DESC;

COMMENT ON VIEW scan_type_usage_stats IS 'Statistics showing usage patterns of different scan types including profile availability';

-- Create a view specifically for UDP profile recommendations
CREATE OR REPLACE VIEW udp_profile_recommendations AS
SELECT
    sp.id,
    sp.name,
    sp.description,
    sp.os_family,
    sp.ports,
    sp.priority,
    CASE
        WHEN sp.os_family = ARRAY[]::TEXT[] THEN 'General'
        WHEN 'windows' = ANY(sp.os_family) THEN 'Windows'
        WHEN 'linux' = ANY(sp.os_family) THEN 'Linux/Unix'
        ELSE 'Specialized'
    END as category
FROM scan_profiles sp
WHERE sp.scan_type = 'udp' AND sp.built_in = true
ORDER BY sp.priority DESC, sp.name;

COMMENT ON VIEW udp_profile_recommendations IS 'Recommended UDP scan profiles categorized by use case and OS family';

-- Add a function to validate scan type compatibility
CREATE OR REPLACE FUNCTION is_valid_scan_type(scan_type_input TEXT)
RETURNS BOOLEAN AS $$
BEGIN
    RETURN scan_type_input IN ('connect', 'syn', 'udp', 'ack', 'window', 'null', 'fin', 'xmas', 'version', 'aggressive', 'comprehensive', 'stealth');
END;
$$ LANGUAGE plpgsql IMMUTABLE;

COMMENT ON FUNCTION is_valid_scan_type(TEXT) IS 'Validates if a scan type is supported by the system';

-- Add a function to get recommended UDP ports for a given OS family
CREATE OR REPLACE FUNCTION get_recommended_udp_ports(target_os_family TEXT DEFAULT NULL)
RETURNS TEXT AS $$
DECLARE
    recommended_ports TEXT;
BEGIN
    SELECT sp.ports INTO recommended_ports
    FROM scan_profiles sp
    WHERE sp.scan_type = 'udp'
      AND sp.built_in = true
      AND (target_os_family IS NULL OR target_os_family = ANY(sp.os_family) OR sp.os_family = ARRAY[]::TEXT[])
    ORDER BY sp.priority DESC
    LIMIT 1;

    RETURN COALESCE(recommended_ports, '53,161,123,1900');
END;
$$ LANGUAGE plpgsql;

COMMENT ON FUNCTION get_recommended_udp_ports(TEXT) IS 'Returns recommended UDP ports for scanning based on target OS family';

-- Add indexes for efficient querying of UDP profiles
CREATE INDEX IF NOT EXISTS idx_scan_profiles_udp_type ON scan_profiles (scan_type) WHERE scan_type = 'udp';

-- Update table comments to reflect enhanced functionality
COMMENT ON TABLE scan_profiles IS 'Scan profiles including comprehensive scan type configurations for TCP, UDP, and specialized scanning scenarios';

-- Migration verification and sample queries (commented out, but useful for testing)
-- SELECT scan_type, COUNT(*) FROM scan_targets GROUP BY scan_type;
-- SELECT scan_type, COUNT(*) FROM scan_profiles GROUP BY scan_type;
-- SELECT * FROM scan_type_usage_stats;
-- SELECT * FROM udp_profile_recommendations;
-- SELECT get_recommended_udp_ports('windows');
-- SELECT get_recommended_udp_ports('linux');
-- SELECT get_recommended_udp_ports();
