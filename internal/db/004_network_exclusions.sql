-- Migration: Add network exclusions table and enhanced target generation
-- This migration adds support for excluding specific IP addresses or ranges
-- from discovery and scanning operations.

-- Create network exclusions table
CREATE TABLE network_exclusions (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),

    -- Reference to network (NULL for global exclusions)
    network_id UUID REFERENCES networks(id) ON DELETE CASCADE,

    -- CIDR range to exclude (can be single IP as /32 or range)
    excluded_cidr CIDR NOT NULL,

    -- Human-readable reason for exclusion
    reason TEXT,

    -- Whether this exclusion is active
    enabled BOOLEAN NOT NULL DEFAULT true,

    -- Timestamps
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    created_by VARCHAR(100)
);

-- Indexes for performance
CREATE INDEX idx_network_exclusions_network_id ON network_exclusions(network_id);
CREATE INDEX idx_network_exclusions_enabled ON network_exclusions(enabled) WHERE enabled = true;
CREATE INDEX idx_network_exclusions_cidr ON network_exclusions USING gist(excluded_cidr);
CREATE INDEX idx_network_exclusions_created_at ON network_exclusions(created_at);

-- Prevent duplicate exclusions for the same network
CREATE UNIQUE INDEX idx_network_exclusions_unique
ON network_exclusions(network_id, excluded_cidr)
WHERE enabled = true;

-- Create trigger for updated_at
CREATE OR REPLACE FUNCTION update_network_exclusions_updated_at()
RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at = NOW();
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER trigger_network_exclusions_updated_at
    BEFORE UPDATE ON network_exclusions
    FOR EACH ROW
    EXECUTE FUNCTION update_network_exclusions_updated_at();

-- Function to check if an IP is excluded for a specific network
CREATE OR REPLACE FUNCTION is_ip_excluded(
    check_ip INET,
    target_network_id UUID DEFAULT NULL
)
RETURNS BOOLEAN AS $$
BEGIN
    -- Check global exclusions (network_id IS NULL)
    IF EXISTS (
        SELECT 1 FROM network_exclusions
        WHERE network_id IS NULL
        AND enabled = true
        AND check_ip << excluded_cidr
    ) THEN
        RETURN true;
    END IF;

    -- Check network-specific exclusions if network_id provided
    IF target_network_id IS NOT NULL THEN
        IF EXISTS (
            SELECT 1 FROM network_exclusions
            WHERE network_id = target_network_id
            AND enabled = true
            AND check_ip << excluded_cidr
        ) THEN
            RETURN true;
        END IF;
    END IF;

    RETURN false;
END;
$$ LANGUAGE plpgsql STABLE;

-- Enhanced function to generate host IPs with exclusions
CREATE OR REPLACE FUNCTION generate_host_ips_with_exclusions(
    network_cidr CIDR,
    target_network_id UUID DEFAULT NULL,
    max_hosts INTEGER DEFAULT 1024
)
RETURNS TABLE(ip_address INET) AS $$
DECLARE
    net_info RECORD;
    current_ip INET;
    host_count INTEGER := 0;
BEGIN
    -- Get network information
    SELECT * INTO net_info FROM get_network_info(network_cidr);

    -- For /31 and /32 networks, include all addresses
    IF masklen(network_cidr) >= 31 THEN
        current_ip := network(network_cidr);
        WHILE current_ip <= broadcast(network_cidr) AND host_count < max_hosts LOOP
            -- Check exclusions
            IF NOT is_ip_excluded(current_ip, target_network_id) THEN
                ip_address := current_ip;
                RETURN NEXT;
                host_count := host_count + 1;
            END IF;
            current_ip := current_ip + 1;
        END LOOP;
        RETURN;
    END IF;

    -- For other networks, exclude network and broadcast addresses
    current_ip := net_info.host_min;
    WHILE current_ip <= net_info.host_max AND host_count < max_hosts LOOP
        -- Check exclusions
        IF NOT is_ip_excluded(current_ip, target_network_id) THEN
            ip_address := current_ip;
            RETURN NEXT;
            host_count := host_count + 1;
        END IF;
        current_ip := current_ip + 1;
    END LOOP;
END;
$$ LANGUAGE plpgsql STABLE;

-- Add helpful comments
COMMENT ON TABLE network_exclusions IS 'IP addresses and ranges to exclude from discovery and scanning';
COMMENT ON COLUMN network_exclusions.network_id IS 'Network to apply exclusion to, NULL for global exclusions';
COMMENT ON COLUMN network_exclusions.excluded_cidr IS 'CIDR range to exclude, use /32 for single IPs';
COMMENT ON COLUMN network_exclusions.reason IS 'Human-readable reason for exclusion (e.g., "Router", "Critical server")';
COMMENT ON FUNCTION is_ip_excluded IS 'Check if an IP address is excluded for a given network';
COMMENT ON FUNCTION generate_host_ips_with_exclusions IS 'Generate host IPs for a network, excluding network/broadcast and explicit exclusions';
