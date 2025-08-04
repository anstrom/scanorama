-- Drop existing tables if they exist
DROP TABLE IF EXISTS port_scans CASCADE;
DROP TABLE IF EXISTS scan_jobs CASCADE;
DROP TABLE IF EXISTS scan_targets CASCADE;
DROP TABLE IF EXISTS hosts CASCADE;

-- Create scan targets table
CREATE TABLE scan_targets (
    id UUID PRIMARY KEY,
    name VARCHAR(255) NOT NULL,
    network CIDR NOT NULL,
    description TEXT,
    scan_interval_seconds INT NOT NULL DEFAULT 3600,
    scan_ports VARCHAR(255) NOT NULL DEFAULT '22,80,443',
    scan_type VARCHAR(50) NOT NULL DEFAULT 'connect',
    enabled BOOLEAN NOT NULL DEFAULT true,
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW()
);

-- Create hosts table
CREATE TABLE hosts (
    id UUID PRIMARY KEY,
    ip_address INET NOT NULL UNIQUE,
    hostname VARCHAR(255),
    mac_address MACADDR,
    vendor VARCHAR(255),
    os_family VARCHAR(100),
    os_version VARCHAR(100),
    status VARCHAR(50) NOT NULL DEFAULT 'up',
    first_seen TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    last_seen TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW()
);

-- Create scan jobs table
CREATE TABLE scan_jobs (
    id UUID PRIMARY KEY,
    target_id UUID NOT NULL REFERENCES scan_targets(id),
    status VARCHAR(50) NOT NULL DEFAULT 'pending',
    error_message TEXT,
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    started_at TIMESTAMP WITH TIME ZONE,
    completed_at TIMESTAMP WITH TIME ZONE
);

-- Create port scans table
CREATE TABLE port_scans (
    id UUID PRIMARY KEY,
    job_id UUID NOT NULL REFERENCES scan_jobs(id),
    host_id UUID NOT NULL REFERENCES hosts(id),
    port INT NOT NULL,
    protocol VARCHAR(10) NOT NULL,
    state VARCHAR(50) NOT NULL,
    service_name VARCHAR(100),
    service_version VARCHAR(100),
    service_product VARCHAR(255),
    banner TEXT,
    scanned_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    UNIQUE(job_id, host_id, port, protocol)
);

-- Create active hosts view
CREATE OR REPLACE VIEW active_hosts AS
SELECT
    h.*,
    array_agg(DISTINCT ps.port ORDER BY ps.port) as open_ports,
    array_agg(DISTINCT ps.service_name) FILTER (WHERE ps.service_name IS NOT NULL) as services
FROM hosts h
LEFT JOIN port_scans ps ON h.id = ps.host_id
WHERE h.status = 'up'
GROUP BY h.id;

-- Create network summary view
CREATE OR REPLACE VIEW network_summary AS
SELECT
    st.id as target_id,
    st.name as target_name,
    st.network,
    COUNT(DISTINCT h.id) as total_hosts,
    COUNT(DISTINCT CASE WHEN h.status = 'up' THEN h.id END) as active_hosts,
    COUNT(DISTINCT ps.port) as unique_ports,
    array_agg(DISTINCT ps.service_name) FILTER (WHERE ps.service_name IS NOT NULL) as services,
    MAX(sj.completed_at) as last_scan
FROM scan_targets st
LEFT JOIN scan_jobs sj ON st.id = sj.target_id
LEFT JOIN port_scans ps ON sj.id = ps.job_id
LEFT JOIN hosts h ON ps.host_id = h.id
GROUP BY st.id, st.name, st.network;
