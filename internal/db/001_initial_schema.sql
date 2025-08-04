-- Scanorama Database Schema
-- PostgreSQL with native network types for efficient network scanning storage

-- Enable required extensions
CREATE EXTENSION IF NOT EXISTS "uuid-ossp";
CREATE EXTENSION IF NOT EXISTS "btree_gist";

-- Networks/targets to scan
CREATE TABLE scan_targets (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    name VARCHAR(255) NOT NULL,
    network CIDR NOT NULL,
    description TEXT,
    scan_interval_seconds INTEGER DEFAULT 3600, -- Default 1 hour
    scan_ports TEXT DEFAULT '22,80,443,8080', -- Comma-separated port list
    scan_type VARCHAR(20) DEFAULT 'connect' CHECK (scan_type IN ('connect', 'syn', 'version')),
    enabled BOOLEAN DEFAULT true,
    created_at TIMESTAMPTZ DEFAULT NOW(),
    updated_at TIMESTAMPTZ DEFAULT NOW()
);

-- Index for network containment queries
CREATE INDEX idx_scan_targets_network ON scan_targets USING GIST (network);
CREATE INDEX idx_scan_targets_enabled ON scan_targets (enabled) WHERE enabled = true;

-- Scan job tracking
CREATE TABLE scan_jobs (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    target_id UUID NOT NULL REFERENCES scan_targets(id) ON DELETE CASCADE,
    status VARCHAR(20) DEFAULT 'pending' CHECK (status IN ('pending', 'running', 'completed', 'failed')),
    started_at TIMESTAMPTZ,
    completed_at TIMESTAMPTZ,
    error_message TEXT,
    scan_stats JSONB, -- Store scan statistics (hosts up/down, etc.)
    created_at TIMESTAMPTZ DEFAULT NOW()
);

CREATE INDEX idx_scan_jobs_target_id ON scan_jobs (target_id);
CREATE INDEX idx_scan_jobs_status ON scan_jobs (status);
CREATE INDEX idx_scan_jobs_started_at ON scan_jobs (started_at);

-- Discovered hosts
CREATE TABLE hosts (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    ip_address INET NOT NULL,
    hostname VARCHAR(255),
    mac_address MACADDR,
    vendor VARCHAR(255), -- MAC address vendor lookup
    os_family VARCHAR(100),
    os_version VARCHAR(100),
    first_seen TIMESTAMPTZ DEFAULT NOW(),
    last_seen TIMESTAMPTZ DEFAULT NOW(),
    status VARCHAR(20) DEFAULT 'up' CHECK (status IN ('up', 'down', 'unknown')),

    -- Ensure unique IP addresses
    CONSTRAINT unique_ip_address UNIQUE (ip_address)
);

-- Indexes for efficient host queries
CREATE INDEX idx_hosts_ip_address ON hosts USING GIST (ip_address);
CREATE INDEX idx_hosts_last_seen ON hosts (last_seen);
CREATE INDEX idx_hosts_status ON hosts (status);
CREATE INDEX idx_hosts_mac_address ON hosts (mac_address) WHERE mac_address IS NOT NULL;

-- Port scan results
CREATE TABLE port_scans (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    job_id UUID NOT NULL REFERENCES scan_jobs(id) ON DELETE CASCADE,
    host_id UUID NOT NULL REFERENCES hosts(id) ON DELETE CASCADE,
    port INTEGER NOT NULL CHECK (port BETWEEN 1 AND 65535),
    protocol VARCHAR(10) DEFAULT 'tcp' CHECK (protocol IN ('tcp', 'udp')),
    state VARCHAR(20) NOT NULL CHECK (state IN ('open', 'closed', 'filtered', 'unknown')),
    service_name VARCHAR(100),
    service_version VARCHAR(255),
    service_product VARCHAR(255),
    banner TEXT,
    scanned_at TIMESTAMPTZ DEFAULT NOW(),

    -- Composite index for efficient port queries
    CONSTRAINT unique_host_port_protocol_scan UNIQUE (job_id, host_id, port, protocol)
);

CREATE INDEX idx_port_scans_host_id ON port_scans (host_id);
CREATE INDEX idx_port_scans_job_id ON port_scans (job_id);
CREATE INDEX idx_port_scans_port ON port_scans (port);
CREATE INDEX idx_port_scans_state ON port_scans (state) WHERE state = 'open';
CREATE INDEX idx_port_scans_service_name ON port_scans (service_name) WHERE service_name IS NOT NULL;

-- Service detection details
CREATE TABLE services (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    port_scan_id UUID NOT NULL REFERENCES port_scans(id) ON DELETE CASCADE,
    service_type VARCHAR(100), -- http, ssh, ftp, etc.
    version VARCHAR(255),
    cpe VARCHAR(255), -- Common Platform Enumeration
    confidence INTEGER CHECK (confidence BETWEEN 0 AND 100),
    details JSONB, -- Additional service-specific data
    detected_at TIMESTAMPTZ DEFAULT NOW()
);

CREATE INDEX idx_services_port_scan_id ON services (port_scan_id);
CREATE INDEX idx_services_service_type ON services (service_type);
CREATE INDEX idx_services_details ON services USING GIN (details);

-- Scan history for tracking changes over time
CREATE TABLE host_history (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    host_id UUID NOT NULL REFERENCES hosts(id) ON DELETE CASCADE,
    job_id UUID NOT NULL REFERENCES scan_jobs(id) ON DELETE CASCADE,
    event_type VARCHAR(50) NOT NULL, -- 'discovered', 'status_change', 'ports_changed', etc.
    old_value JSONB,
    new_value JSONB,
    created_at TIMESTAMPTZ DEFAULT NOW()
);

CREATE INDEX idx_host_history_host_id ON host_history (host_id);
CREATE INDEX idx_host_history_created_at ON host_history (created_at);
CREATE INDEX idx_host_history_event_type ON host_history (event_type);

-- Views for common queries

-- Active hosts with port counts
CREATE VIEW active_hosts AS
SELECT
    h.ip_address,
    h.hostname,
    h.mac_address,
    h.vendor,
    h.status,
    h.last_seen,
    COUNT(ps.id) FILTER (WHERE ps.state = 'open') as open_ports,
    COUNT(ps.id) as total_ports_scanned
FROM hosts h
LEFT JOIN port_scans ps ON h.id = ps.host_id
WHERE h.status = 'up'
GROUP BY h.id, h.ip_address, h.hostname, h.mac_address, h.vendor, h.status, h.last_seen;

-- Network summary
CREATE VIEW network_summary AS
SELECT
    st.name as target_name,
    st.network,
    COUNT(DISTINCT h.id) FILTER (WHERE h.status = 'up') as active_hosts,
    COUNT(DISTINCT h.id) as total_hosts,
    COUNT(DISTINCT ps.id) FILTER (WHERE ps.state = 'open') as open_ports,
    MAX(sj.completed_at) as last_scan
FROM scan_targets st
LEFT JOIN scan_jobs sj ON st.id = sj.target_id AND sj.status = 'completed'
LEFT JOIN hosts h ON h.ip_address << st.network
LEFT JOIN port_scans ps ON h.id = ps.host_id
WHERE st.enabled = true
GROUP BY st.id, st.name, st.network;

-- Functions for automatic timestamp updates
CREATE OR REPLACE FUNCTION update_updated_at_column()
RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at = NOW();
    RETURN NEW;
END;
$$ language 'plpgsql';

-- Triggers for automatic timestamp updates
CREATE TRIGGER update_scan_targets_updated_at BEFORE UPDATE ON scan_targets
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

-- Function to update host last_seen timestamp
CREATE OR REPLACE FUNCTION update_host_last_seen()
RETURNS TRIGGER AS $$
BEGIN
    UPDATE hosts
    SET last_seen = NOW()
    WHERE id = NEW.host_id;
    RETURN NEW;
END;
$$ language 'plpgsql';

-- Trigger to update host last_seen when new port scans are added
CREATE TRIGGER update_host_last_seen_trigger AFTER INSERT ON port_scans
    FOR EACH ROW EXECUTE FUNCTION update_host_last_seen();

-- Sample data for development
INSERT INTO scan_targets (name, network, description, scan_interval_seconds) VALUES
('Local Network', '192.168.1.0/24', 'Local development network', 1800),
('DMZ Network', '10.0.1.0/24', 'DMZ servers', 3600),
('Test Network', '172.16.0.0/24', 'Test environment', 7200);

-- Comments for documentation
COMMENT ON TABLE scan_targets IS 'Networks and IP ranges to scan continuously';
COMMENT ON TABLE scan_jobs IS 'Individual scan job tracking and status';
COMMENT ON TABLE hosts IS 'Discovered hosts with their properties';
COMMENT ON TABLE port_scans IS 'Port scan results for each host';
COMMENT ON TABLE services IS 'Detailed service detection results';
COMMENT ON TABLE host_history IS 'Audit trail of host changes over time';

COMMENT ON COLUMN hosts.ip_address IS 'IPv4 or IPv6 address using PostgreSQL inet type';
COMMENT ON COLUMN hosts.mac_address IS 'MAC address using PostgreSQL macaddr type';
COMMENT ON COLUMN scan_targets.network IS 'Network range using PostgreSQL cidr type';
