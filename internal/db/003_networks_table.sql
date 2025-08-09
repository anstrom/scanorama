-- Migration 003: Networks Table
-- This migration creates the networks table for managing network discovery targets
-- and provides database functions for network IP generation and management.

-- Create networks table for storing network discovery targets
CREATE TABLE networks (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    name VARCHAR(255) NOT NULL,
    cidr CIDR NOT NULL,
    description TEXT,
    discovery_method VARCHAR(20) NOT NULL DEFAULT 'tcp',
    is_active BOOLEAN NOT NULL DEFAULT true,
    scan_enabled BOOLEAN NOT NULL DEFAULT true,
    last_discovery TIMESTAMPTZ,
    last_scan TIMESTAMPTZ,
    host_count INTEGER DEFAULT 0,
    active_host_count INTEGER DEFAULT 0,
    created_at TIMESTAMPTZ DEFAULT NOW(),
    updated_at TIMESTAMPTZ DEFAULT NOW(),
    created_by VARCHAR(100)
);

-- Add constraints
ALTER TABLE networks ADD CONSTRAINT networks_discovery_method_check
    CHECK (discovery_method IN ('ping', 'tcp', 'arp', 'icmp'));

ALTER TABLE networks ADD CONSTRAINT networks_host_counts_check
    CHECK (host_count >= 0 AND active_host_count >= 0 AND active_host_count <= host_count);

-- Add unique constraints
ALTER TABLE networks ADD CONSTRAINT networks_name_key UNIQUE (name);
ALTER TABLE networks ADD CONSTRAINT networks_cidr_key UNIQUE (cidr);

-- Create indexes for performance
CREATE INDEX idx_networks_active ON networks(is_active) WHERE is_active = true;
CREATE INDEX idx_networks_cidr ON networks USING gist(cidr);
CREATE INDEX idx_networks_discovery_method ON networks(discovery_method);
CREATE INDEX idx_networks_last_discovery ON networks(last_discovery);
CREATE INDEX idx_networks_last_scan ON networks(last_scan);
CREATE INDEX idx_networks_scan_enabled ON networks(scan_enabled) WHERE scan_enabled = true;

-- Create trigger function for updated_at
CREATE OR REPLACE FUNCTION update_networks_updated_at()
RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at = NOW();
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

-- Create trigger
CREATE TRIGGER trigger_networks_updated_at
    BEFORE UPDATE ON networks
    FOR EACH ROW
    EXECUTE FUNCTION update_networks_updated_at();

-- Function to get network information (used by IP generation)
CREATE OR REPLACE FUNCTION get_network_info(network_cidr CIDR)
RETURNS TABLE(
    network_addr INET,
    broadcast_addr INET,
    netmask INET,
    host_min INET,
    host_max INET,
    total_hosts BIGINT
) AS $$
BEGIN
    RETURN QUERY
    SELECT
        network(network_cidr) as network_addr,
        broadcast(network_cidr) as broadcast_addr,
        netmask(network_cidr) as netmask,
        -- First usable host (network + 1)
        (network(network_cidr) + 1) as host_min,
        -- Last usable host (broadcast - 1)
        (broadcast(network_cidr) - 1) as host_max,
        -- Total usable hosts (excluding network and broadcast)
        CASE
            WHEN masklen(network_cidr) >= 31 THEN
                -- /31 and /32 networks don't have network/broadcast addresses
                (2^(32 - masklen(network_cidr)))::BIGINT
            ELSE
                (2^(32 - masklen(network_cidr)) - 2)::BIGINT
        END as total_hosts;
END;
$$ LANGUAGE plpgsql STABLE;

-- Function to generate host IPs for a network (excluding network/broadcast)
CREATE OR REPLACE FUNCTION generate_host_ips(
    network_cidr CIDR,
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
            ip_address := current_ip;
            RETURN NEXT;
            current_ip := current_ip + 1;
            host_count := host_count + 1;
        END LOOP;
        RETURN;
    END IF;

    -- For other networks, exclude network and broadcast addresses
    current_ip := net_info.host_min;
    WHILE current_ip <= net_info.host_max AND host_count < max_hosts LOOP
        ip_address := current_ip;
        RETURN NEXT;
        current_ip := current_ip + 1;
        host_count := host_count + 1;
    END LOOP;
END;
$$ LANGUAGE plpgsql STABLE;

-- Function to update network host counts (called by triggers)
CREATE OR REPLACE FUNCTION update_network_host_counts()
RETURNS TRIGGER AS $$
DECLARE
    network_record RECORD;
BEGIN
    -- Find networks that contain this host's IP
    FOR network_record IN
        SELECT id, cidr FROM networks WHERE NEW.ip_address << cidr
    LOOP
        -- Update host counts for this network
        UPDATE networks SET
            host_count = (
                SELECT COUNT(*) FROM hosts
                WHERE ip_address << network_record.cidr
            ),
            active_host_count = (
                SELECT COUNT(*) FROM hosts
                WHERE ip_address << network_record.cidr AND status = 'up'
            ),
            updated_at = NOW()
        WHERE id = network_record.id;
    END LOOP;

    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

-- Create triggers to maintain network host counts
CREATE TRIGGER trigger_update_network_counts_insert
    AFTER INSERT ON hosts
    FOR EACH ROW
    EXECUTE FUNCTION update_network_host_counts();

CREATE TRIGGER trigger_update_network_counts_update
    AFTER UPDATE OF ip_address, status ON hosts
    FOR EACH ROW
    EXECUTE FUNCTION update_network_host_counts();

CREATE TRIGGER trigger_update_network_counts_delete
    AFTER DELETE ON hosts
    FOR EACH ROW
    EXECUTE FUNCTION update_network_host_counts();

-- Add helpful comments
COMMENT ON TABLE networks IS 'Network discovery targets with CIDR ranges and configuration';
COMMENT ON COLUMN networks.cidr IS 'Network CIDR range (e.g., 192.168.1.0/24)';
COMMENT ON COLUMN networks.discovery_method IS 'Method for discovering hosts (ping, tcp, arp, icmp)';
COMMENT ON COLUMN networks.is_active IS 'Whether this network is enabled for discovery';
COMMENT ON COLUMN networks.scan_enabled IS 'Whether this network is enabled for detailed scanning';
COMMENT ON COLUMN networks.host_count IS 'Total number of discovered hosts in this network';
COMMENT ON COLUMN networks.active_host_count IS 'Number of hosts with status=up in this network';

COMMENT ON FUNCTION get_network_info IS 'Calculate network addressing information for a CIDR range';
COMMENT ON FUNCTION generate_host_ips IS 'Generate valid host IP addresses for a network, excluding network/broadcast addresses';
COMMENT ON FUNCTION update_network_host_counts IS 'Maintain accurate host counts in networks table when hosts change';
