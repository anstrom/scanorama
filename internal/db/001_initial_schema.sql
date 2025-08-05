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

-- Scan profiles (must come before scan_jobs due to foreign key)
CREATE TABLE scan_profiles (
    id VARCHAR(100) PRIMARY KEY,
    name VARCHAR(255) NOT NULL,
    description TEXT,
    os_family TEXT[],
    os_pattern TEXT[],
    ports TEXT NOT NULL,
    scan_type VARCHAR(20) NOT NULL,
    timing VARCHAR(20) DEFAULT 'normal',
    scripts TEXT[],
    options JSONB,
    priority INTEGER DEFAULT 0,
    built_in BOOLEAN DEFAULT FALSE,
    created_at TIMESTAMPTZ DEFAULT NOW(),
    updated_at TIMESTAMPTZ DEFAULT NOW()
);

CREATE INDEX idx_scan_profiles_os_family ON scan_profiles USING GIN (os_family);
CREATE INDEX idx_scan_profiles_priority ON scan_profiles (priority DESC);

-- Scan job tracking with profile support
CREATE TABLE scan_jobs (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    target_id UUID NOT NULL REFERENCES scan_targets(id) ON DELETE CASCADE,
    profile_id VARCHAR(100) REFERENCES scan_profiles(id),
    status VARCHAR(20) DEFAULT 'pending' CHECK (status IN ('pending', 'running', 'completed', 'failed')),
    started_at TIMESTAMPTZ,
    completed_at TIMESTAMPTZ,
    error_message TEXT,
    scan_stats JSONB, -- Store scan statistics (hosts up/down, etc.)
    created_at TIMESTAMPTZ DEFAULT NOW()
);

CREATE INDEX idx_scan_jobs_target_id ON scan_jobs (target_id);
CREATE INDEX idx_scan_jobs_profile_id ON scan_jobs (profile_id);
CREATE INDEX idx_scan_jobs_status ON scan_jobs (status);
CREATE INDEX idx_scan_jobs_started_at ON scan_jobs (started_at);

-- Discovery jobs
CREATE TABLE discovery_jobs (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    network CIDR NOT NULL,
    method VARCHAR(20) NOT NULL,
    started_at TIMESTAMPTZ,
    completed_at TIMESTAMPTZ,
    hosts_discovered INTEGER DEFAULT 0,
    hosts_responsive INTEGER DEFAULT 0,
    status VARCHAR(20) DEFAULT 'pending' CHECK (status IN ('pending', 'running', 'completed', 'failed')),
    created_at TIMESTAMPTZ DEFAULT NOW()
);

CREATE INDEX idx_discovery_jobs_network ON discovery_jobs (network);
CREATE INDEX idx_discovery_jobs_status ON discovery_jobs (status);
CREATE INDEX idx_discovery_jobs_created_at ON discovery_jobs (created_at);

-- Scheduled jobs
CREATE TABLE scheduled_jobs (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    name VARCHAR(255) UNIQUE NOT NULL,
    type VARCHAR(20) NOT NULL CHECK (type IN ('discovery', 'scan')),
    cron_expression VARCHAR(100) NOT NULL,
    config JSONB NOT NULL,
    enabled BOOLEAN DEFAULT TRUE,
    last_run TIMESTAMPTZ,
    next_run TIMESTAMPTZ,
    created_at TIMESTAMPTZ DEFAULT NOW()
);

CREATE INDEX idx_scheduled_jobs_type ON scheduled_jobs (type);
CREATE INDEX idx_scheduled_jobs_enabled ON scheduled_jobs (enabled) WHERE enabled = true;
CREATE INDEX idx_scheduled_jobs_next_run ON scheduled_jobs (next_run) WHERE enabled = true;

-- Discovered hosts with enhanced OS detection
CREATE TABLE hosts (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    ip_address INET NOT NULL,
    hostname VARCHAR(255),
    mac_address MACADDR,
    vendor VARCHAR(255), -- MAC address vendor lookup
    os_family VARCHAR(50),
    os_name VARCHAR(255),
    os_version VARCHAR(100),
    os_confidence INTEGER,
    os_detected_at TIMESTAMPTZ,
    os_method VARCHAR(50),
    os_details JSONB,
    discovery_method VARCHAR(20),
    response_time_ms INTEGER,
    discovery_count INTEGER DEFAULT 0,
    ignore_scanning BOOLEAN DEFAULT FALSE,
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
CREATE INDEX idx_hosts_os_family ON hosts (os_family) WHERE os_family IS NOT NULL;
CREATE INDEX idx_hosts_discovery_method ON hosts (discovery_method) WHERE discovery_method IS NOT NULL;
CREATE INDEX idx_hosts_ignore_scanning ON hosts (ignore_scanning) WHERE ignore_scanning = false;

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

-- Built-in scan profiles
INSERT INTO scan_profiles (id, name, description, os_family, ports, scan_type, timing, scripts, priority, built_in) VALUES
('windows-server', 'Windows Server', 'Comprehensive scan for Windows servers',
 ARRAY['windows'], '21,22,23,25,53,80,110,135,139,143,443,445,993,995,1433,1521,3389,5985,5986',
 'version', 'normal', ARRAY['smb-os-discovery', 'smb-security-mode', 'ms-sql-info', 'rdp-enum-encryption'], 90, true),

('linux-server', 'Linux Server', 'Focused scan for Linux servers',
 ARRAY['linux'], '21,22,23,25,53,80,110,143,443,993,995,3306,5432,6379,9200,27017',
 'version', 'normal', ARRAY['ssh-hostkey', 'http-title', 'mysql-info', 'ssl-cert'], 90, true),

('windows-workstation', 'Windows Workstation', 'Light scan for Windows desktops',
 ARRAY['windows'], '135,139,445,3389,5985',
 'connect', 'polite', ARRAY['smb-os-discovery'], 80, true),

('linux-workstation', 'Linux Workstation', 'Light scan for Linux desktops',
 ARRAY['linux'], '22,80,443,631',
 'connect', 'polite', ARRAY['ssh-hostkey'], 80, true),

('macos-system', 'macOS System', 'Scan for macOS systems',
 ARRAY['macos'], '22,80,443,548,631,5900',
 'connect', 'polite', ARRAY['ssh-hostkey', 'vnc-info'], 85, true),

('generic-default', 'Generic Default', 'Default scan for unknown OS',
 ARRAY[], '21,22,23,25,53,80,110,143,443,993,995,3389',
 'connect', 'normal', ARRAY[], 10, true);

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
