-- Scanorama Database Schema
-- Squashed from migrations 001–011
--
-- Applying this file to a fresh database produces the complete final schema.
-- Existing databases that already have 001_initial_schema through 010_dns_cache
-- in schema_migrations will skip this file (migrator tracks by name).

-- ===========================================================================
-- Extensions
-- ===========================================================================

CREATE EXTENSION IF NOT EXISTS "uuid-ossp";
CREATE EXTENSION IF NOT EXISTS "btree_gist";

-- ===========================================================================
-- Functions
-- ===========================================================================

CREATE OR REPLACE FUNCTION update_updated_at_column()
RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at = NOW();
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE OR REPLACE FUNCTION update_host_last_seen()
RETURNS TRIGGER AS $$
BEGIN
    UPDATE hosts
    SET last_seen = NOW()
    WHERE id = NEW.host_id;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE OR REPLACE FUNCTION update_modified_by()
RETURNS TRIGGER AS $$
BEGIN
    NEW.modified_by = COALESCE(NEW.modified_by, 'system');
    NEW.updated_at = NOW();
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE OR REPLACE FUNCTION update_networks_updated_at()
RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at = NOW();
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE OR REPLACE FUNCTION update_network_exclusions_updated_at()
RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at = NOW();
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE OR REPLACE FUNCTION update_api_keys_updated_at()
RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at = NOW();
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

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

COMMENT ON FUNCTION update_network_host_counts() IS 'Maintain accurate host counts in networks table when hosts change';

CREATE OR REPLACE FUNCTION refresh_summary_views()
RETURNS void AS $$
BEGIN
    REFRESH MATERIALIZED VIEW CONCURRENTLY host_summary;
    REFRESH MATERIALIZED VIEW CONCURRENTLY network_summary_mv;
END;
$$ LANGUAGE plpgsql;

CREATE OR REPLACE FUNCTION cleanup_old_scan_data(days_to_keep INTEGER DEFAULT 90)
RETURNS INTEGER AS $$
DECLARE
    deleted_count INTEGER;
BEGIN
    WITH deleted_scans AS (
        DELETE FROM port_scans
        WHERE scanned_at < NOW() - (days_to_keep || ' days')::INTERVAL
        RETURNING job_id
    ),
    deleted_jobs AS (
        DELETE FROM scan_jobs sj
        WHERE sj.id IN (SELECT job_id FROM deleted_scans)
        AND sj.completed_at < NOW() - (days_to_keep || ' days')::INTERVAL
        RETURNING id
    )
    SELECT COUNT(*) INTO deleted_count FROM deleted_jobs;

    PERFORM refresh_summary_views();
    RETURN deleted_count;
END;
$$ LANGUAGE plpgsql;

CREATE OR REPLACE FUNCTION get_network_info(network_cidr CIDR)
RETURNS TABLE(
    network_addr    INET,
    broadcast_addr  INET,
    netmask         INET,
    host_min        INET,
    host_max        INET,
    total_hosts     BIGINT
) AS $$
BEGIN
    RETURN QUERY
    SELECT
        network(network_cidr)::INET                             AS network_addr,
        broadcast(network_cidr)::INET                           AS broadcast_addr,
        netmask(network_cidr)::INET                             AS netmask,
        (network(network_cidr) + 1)::INET                       AS host_min,
        (broadcast(network_cidr) - 1)::INET                     AS host_max,
        CASE
            WHEN masklen(network_cidr) >= 31 THEN
                (2^(32 - masklen(network_cidr)))::BIGINT
            ELSE
                (2^(32 - masklen(network_cidr)) - 2)::BIGINT
        END                                                     AS total_hosts;
END;
$$ LANGUAGE plpgsql STABLE;

COMMENT ON FUNCTION get_network_info(CIDR) IS 'Calculate network addressing information for a CIDR range';

CREATE OR REPLACE FUNCTION generate_host_ips(
    network_cidr CIDR,
    max_hosts    INTEGER DEFAULT 1024
)
RETURNS TABLE(ip_address INET) AS $$
DECLARE
    net_info   RECORD;
    current_ip INET;
    host_count INTEGER := 0;
BEGIN
    SELECT * INTO net_info FROM get_network_info(network_cidr);

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

    current_ip := net_info.host_min;
    WHILE current_ip <= net_info.host_max AND host_count < max_hosts LOOP
        ip_address := current_ip;
        RETURN NEXT;
        current_ip := current_ip + 1;
        host_count := host_count + 1;
    END LOOP;
END;
$$ LANGUAGE plpgsql STABLE;

COMMENT ON FUNCTION generate_host_ips(CIDR, INTEGER) IS 'Generate valid host IP addresses for a network, excluding network/broadcast addresses';

CREATE OR REPLACE FUNCTION is_ip_excluded(
    check_ip          INET,
    target_network_id UUID DEFAULT NULL
)
RETURNS BOOLEAN AS $$
BEGIN
    IF EXISTS (
        SELECT 1 FROM network_exclusions
        WHERE network_id IS NULL
          AND enabled = true
          AND check_ip << excluded_cidr
    ) THEN
        RETURN true;
    END IF;

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

COMMENT ON FUNCTION is_ip_excluded(INET, UUID) IS 'Check if an IP address is excluded for a given network';

CREATE OR REPLACE FUNCTION generate_host_ips_with_exclusions(
    network_cidr      CIDR,
    target_network_id UUID    DEFAULT NULL,
    max_hosts         INTEGER DEFAULT 1024
)
RETURNS TABLE(ip_address INET) AS $$
DECLARE
    net_info   RECORD;
    current_ip INET;
    host_count INTEGER := 0;
BEGIN
    SELECT * INTO net_info FROM get_network_info(network_cidr);

    IF masklen(network_cidr) >= 31 THEN
        current_ip := network(network_cidr);
        WHILE current_ip <= broadcast(network_cidr) AND host_count < max_hosts LOOP
            IF NOT is_ip_excluded(current_ip, target_network_id) THEN
                ip_address := current_ip;
                RETURN NEXT;
                host_count := host_count + 1;
            END IF;
            current_ip := current_ip + 1;
        END LOOP;
        RETURN;
    END IF;

    current_ip := net_info.host_min;
    WHILE current_ip <= net_info.host_max AND host_count < max_hosts LOOP
        IF NOT is_ip_excluded(current_ip, target_network_id) THEN
            ip_address := current_ip;
            RETURN NEXT;
            host_count := host_count + 1;
        END IF;
        current_ip := current_ip + 1;
    END LOOP;
END;
$$ LANGUAGE plpgsql STABLE;

COMMENT ON FUNCTION generate_host_ips_with_exclusions(CIDR, UUID, INTEGER) IS 'Generate host IPs for a network, excluding network/broadcast and explicit exclusions';

CREATE OR REPLACE FUNCTION is_valid_scan_type(scan_type_input TEXT)
RETURNS BOOLEAN AS $$
BEGIN
    RETURN scan_type_input IN ('connect', 'syn', 'ack', 'udp', 'aggressive', 'comprehensive');
END;
$$ LANGUAGE plpgsql IMMUTABLE;

COMMENT ON FUNCTION is_valid_scan_type(TEXT) IS 'Validates if a scan type is supported by the system';

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

-- Fixed version: no broken INSERT into nonexistent audit_log table
CREATE OR REPLACE FUNCTION cleanup_expired_api_keys()
RETURNS INTEGER AS $$
DECLARE
    deactivated_count INTEGER;
BEGIN
    UPDATE api_keys
    SET    is_active  = false,
           updated_at = NOW()
    WHERE  is_active    = true
      AND  expires_at IS NOT NULL
      AND  expires_at  < NOW();

    GET DIAGNOSTICS deactivated_count = ROW_COUNT;
    RETURN deactivated_count;
END;
$$ LANGUAGE plpgsql;

-- ===========================================================================
-- Tables (in FK-dependency order)
-- ===========================================================================

-- Scan profiles (no FK dependencies)
CREATE TABLE IF NOT EXISTS scan_profiles (
    id          VARCHAR(100) PRIMARY KEY,
    name        VARCHAR(255) NOT NULL,
    description TEXT,
    os_family   TEXT[],
    os_pattern  TEXT[],
    ports       TEXT         NOT NULL,
    scan_type   VARCHAR(20)  NOT NULL
                    CONSTRAINT scan_profiles_scan_type_check
                    CHECK (scan_type IN ('connect', 'syn', 'ack', 'udp', 'aggressive', 'comprehensive')),
    timing      VARCHAR(20)  DEFAULT 'normal',
    scripts     TEXT[],
    options     JSONB,
    priority    INTEGER      DEFAULT 0,
    built_in    BOOLEAN      DEFAULT FALSE,
    created_at  TIMESTAMPTZ  DEFAULT NOW(),
    updated_at  TIMESTAMPTZ  DEFAULT NOW()
);

COMMENT ON TABLE scan_profiles IS 'Scan profiles including comprehensive scan type configurations for TCP, UDP, and specialized scanning scenarios';

CREATE INDEX IF NOT EXISTS idx_scan_profiles_os_family ON scan_profiles USING GIN (os_family);
CREATE INDEX IF NOT EXISTS idx_scan_profiles_priority  ON scan_profiles (priority DESC);
CREATE INDEX IF NOT EXISTS idx_scan_profiles_scan_type ON scan_profiles (scan_type);
CREATE INDEX IF NOT EXISTS idx_scan_profiles_udp_type  ON scan_profiles (scan_type) WHERE scan_type = 'udp';

-- Networks table (no FK dependencies)
CREATE TABLE IF NOT EXISTS networks (
    id                    UUID         PRIMARY KEY DEFAULT uuid_generate_v4(),
    name                  VARCHAR(255) NOT NULL,
    cidr                  CIDR         NOT NULL,
    description           TEXT,
    discovery_method      VARCHAR(20)  NOT NULL DEFAULT 'tcp',
    is_active             BOOLEAN      NOT NULL DEFAULT true,
    scan_enabled          BOOLEAN      NOT NULL DEFAULT true,
    last_discovery        TIMESTAMPTZ,
    last_scan             TIMESTAMPTZ,
    host_count            INTEGER      DEFAULT 0,
    active_host_count     INTEGER      DEFAULT 0,
    created_at            TIMESTAMPTZ  DEFAULT NOW(),
    updated_at            TIMESTAMPTZ  DEFAULT NOW(),
    created_by            VARCHAR(100),
    scan_interval_seconds INTEGER      NOT NULL DEFAULT 3600,
    scan_ports            TEXT         NOT NULL DEFAULT '22,80,443,8080',
    scan_type             VARCHAR(20)  NOT NULL DEFAULT 'connect',
    modified_by           VARCHAR(100),
    CONSTRAINT networks_discovery_method_check
        CHECK (discovery_method IN ('ping', 'tcp', 'arp', 'icmp')),
    CONSTRAINT networks_host_counts_check
        CHECK (host_count >= 0 AND active_host_count >= 0 AND active_host_count <= host_count),
    CONSTRAINT networks_name_key UNIQUE (name),
    CONSTRAINT networks_cidr_key UNIQUE (cidr),
    CONSTRAINT networks_scan_type_check
        CHECK (scan_type IN ('connect', 'syn', 'ack', 'udp', 'aggressive', 'comprehensive'))
);

COMMENT ON TABLE  networks                     IS 'Network discovery targets with CIDR ranges and configuration';
COMMENT ON COLUMN networks.cidr               IS 'Network CIDR range (e.g., 192.168.1.0/24)';
COMMENT ON COLUMN networks.discovery_method   IS 'Method for discovering hosts (ping, tcp, arp, icmp)';
COMMENT ON COLUMN networks.is_active          IS 'Whether this network is enabled for discovery';
COMMENT ON COLUMN networks.scan_enabled       IS 'Whether this network is enabled for detailed scanning';
COMMENT ON COLUMN networks.host_count         IS 'Total number of discovered hosts in this network';
COMMENT ON COLUMN networks.active_host_count  IS 'Number of hosts with status=up in this network';

CREATE INDEX IF NOT EXISTS idx_networks_active           ON networks (is_active)         WHERE is_active = true;
CREATE INDEX IF NOT EXISTS idx_networks_cidr             ON networks USING gist (cidr);
CREATE INDEX IF NOT EXISTS idx_networks_discovery_method ON networks (discovery_method);
CREATE INDEX IF NOT EXISTS idx_networks_last_discovery   ON networks (last_discovery);
CREATE INDEX IF NOT EXISTS idx_networks_last_scan        ON networks (last_scan);
CREATE INDEX IF NOT EXISTS idx_networks_scan_enabled     ON networks (scan_enabled)      WHERE scan_enabled = true;

-- Scan jobs (depends on networks and scan_profiles)
CREATE TABLE IF NOT EXISTS scan_jobs (
    id                UUID         PRIMARY KEY DEFAULT uuid_generate_v4(),
    network_id        UUID         NOT NULL REFERENCES networks(id) ON DELETE CASCADE,
    profile_id        VARCHAR(100) REFERENCES scan_profiles(id) ON DELETE SET NULL,
    status            VARCHAR(20)  DEFAULT 'pending'
                          CHECK (status IN ('pending', 'running', 'completed', 'failed')),
    started_at        TIMESTAMPTZ,
    completed_at      TIMESTAMPTZ,
    error_message     TEXT,
    scan_stats        JSONB,
    created_at        TIMESTAMPTZ  DEFAULT NOW(),
    progress_percent  INTEGER      DEFAULT 0,
    timeout_at        TIMESTAMPTZ,
    execution_details JSONB,
    worker_id         VARCHAR(100),
    created_by        VARCHAR(100),
    CONSTRAINT check_job_timing
        CHECK (completed_at IS NULL OR started_at IS NULL OR completed_at >= started_at),
    CONSTRAINT check_progress_range
        CHECK (progress_percent >= 0 AND progress_percent <= 100),
    CONSTRAINT check_status_timing
        CHECK (
            (status = 'pending'  AND started_at IS NULL) OR
            (status = 'running'  AND started_at IS NOT NULL) OR
            (status IN ('completed', 'failed') AND started_at IS NOT NULL)
        )
);

COMMENT ON TABLE  scan_jobs                   IS 'Individual scan job tracking and status';
COMMENT ON COLUMN scan_jobs.progress_percent  IS 'Scan completion percentage (0-100)';
COMMENT ON COLUMN scan_jobs.timeout_at        IS 'When the scan job should timeout';
COMMENT ON COLUMN scan_jobs.execution_details IS 'Additional execution context and metadata';
COMMENT ON COLUMN scan_jobs.worker_id         IS 'Identifier of worker/process executing the scan';

CREATE INDEX IF NOT EXISTS idx_scan_jobs_profile_id     ON scan_jobs (profile_id);
CREATE INDEX IF NOT EXISTS idx_scan_jobs_status         ON scan_jobs (status);
CREATE INDEX IF NOT EXISTS idx_scan_jobs_started_at     ON scan_jobs (started_at);
CREATE INDEX IF NOT EXISTS idx_scan_jobs_status_created ON scan_jobs (status, created_at);

-- Discovery jobs (no FK dependencies)
CREATE TABLE IF NOT EXISTS discovery_jobs (
    id               UUID        PRIMARY KEY DEFAULT uuid_generate_v4(),
    network          CIDR        NOT NULL,
    method           VARCHAR(20) NOT NULL,
    started_at       TIMESTAMPTZ,
    completed_at     TIMESTAMPTZ,
    hosts_discovered INTEGER     DEFAULT 0,
    hosts_responsive INTEGER     DEFAULT 0,
    status           VARCHAR(20) DEFAULT 'pending'
                         CHECK (status IN ('pending', 'running', 'completed', 'failed')),
    created_at       TIMESTAMPTZ DEFAULT NOW()
);

COMMENT ON TABLE discovery_jobs IS 'Network discovery job tracking';

CREATE INDEX IF NOT EXISTS idx_discovery_jobs_network    ON discovery_jobs (network);
CREATE INDEX IF NOT EXISTS idx_discovery_jobs_status     ON discovery_jobs (status);
CREATE INDEX IF NOT EXISTS idx_discovery_jobs_created_at ON discovery_jobs (created_at);

-- Scheduled jobs (no FK dependencies)
CREATE TABLE IF NOT EXISTS scheduled_jobs (
    id                   UUID         PRIMARY KEY DEFAULT uuid_generate_v4(),
    name                 VARCHAR(255) UNIQUE NOT NULL,
    type                 VARCHAR(20)  NOT NULL CHECK (type IN ('discovery', 'scan')),
    cron_expression      VARCHAR(100) NOT NULL,
    config               JSONB        NOT NULL,
    enabled              BOOLEAN      DEFAULT TRUE,
    last_run             TIMESTAMPTZ,
    next_run             TIMESTAMPTZ,
    created_at           TIMESTAMPTZ  DEFAULT NOW(),
    last_run_duration_ms INTEGER,
    last_run_status      VARCHAR(20)
                             CONSTRAINT check_last_run_status
                             CHECK (last_run_status IN ('success', 'failed', 'timeout', 'cancelled')),
    consecutive_failures INTEGER      DEFAULT 0,
    max_failures         INTEGER      DEFAULT 5
);

COMMENT ON TABLE  scheduled_jobs                        IS 'Scheduled recurring scan and discovery jobs';
COMMENT ON COLUMN scheduled_jobs.consecutive_failures   IS 'Number of consecutive failures for alerting';
COMMENT ON COLUMN scheduled_jobs.max_failures           IS 'Maximum failures before disabling job';

CREATE INDEX IF NOT EXISTS idx_scheduled_jobs_type             ON scheduled_jobs (type);
CREATE INDEX IF NOT EXISTS idx_scheduled_jobs_enabled          ON scheduled_jobs (enabled)       WHERE enabled = true;
CREATE INDEX IF NOT EXISTS idx_scheduled_jobs_next_run         ON scheduled_jobs (next_run)      WHERE enabled = true;
CREATE INDEX IF NOT EXISTS idx_scheduled_jobs_next_run_enabled ON scheduled_jobs (next_run, enabled) WHERE enabled = true;
CREATE INDEX IF NOT EXISTS idx_scheduled_jobs_failures         ON scheduled_jobs (consecutive_failures, enabled)
    WHERE consecutive_failures > 0 AND enabled = true;

-- Hosts (no FK dependencies)
CREATE TABLE IF NOT EXISTS hosts (
    id               UUID        PRIMARY KEY DEFAULT uuid_generate_v4(),
    ip_address       INET        NOT NULL,
    hostname         VARCHAR(255),
    mac_address      MACADDR,
    vendor           VARCHAR(255),
    os_family        VARCHAR(50),
    os_name          VARCHAR(255),
    os_version       VARCHAR(100),
    os_confidence    INTEGER,
    os_detected_at   TIMESTAMPTZ,
    os_method        VARCHAR(50),
    os_details       JSONB,
    discovery_method VARCHAR(20),
    response_time_ms INTEGER,
    discovery_count  INTEGER     DEFAULT 0,
    ignore_scanning  BOOLEAN     DEFAULT FALSE,
    first_seen       TIMESTAMPTZ DEFAULT NOW(),
    last_seen        TIMESTAMPTZ DEFAULT NOW(),
    status           VARCHAR(20) DEFAULT 'up'
                         CHECK (status IN ('up', 'down', 'unknown')),
    CONSTRAINT unique_ip_address   UNIQUE (ip_address),
    CONSTRAINT check_confidence_range
        CHECK (os_confidence IS NULL OR (os_confidence >= 0 AND os_confidence <= 100))
);

COMMENT ON TABLE  hosts            IS 'Discovered hosts with their properties';
COMMENT ON COLUMN hosts.ip_address IS 'IPv4 or IPv6 address using PostgreSQL inet type';
COMMENT ON COLUMN hosts.mac_address IS 'MAC address using PostgreSQL macaddr type';

CREATE INDEX IF NOT EXISTS idx_hosts_ip_address       ON hosts USING GIST (ip_address);
CREATE INDEX IF NOT EXISTS idx_hosts_last_seen        ON hosts (last_seen);
CREATE INDEX IF NOT EXISTS idx_hosts_status           ON hosts (status);
CREATE INDEX IF NOT EXISTS idx_hosts_mac_address      ON hosts (mac_address)         WHERE mac_address IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_hosts_os_family        ON hosts (os_family)           WHERE os_family IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_hosts_discovery_method ON hosts (discovery_method)    WHERE discovery_method IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_hosts_ignore_scanning  ON hosts (ignore_scanning)     WHERE ignore_scanning = false;
CREATE INDEX IF NOT EXISTS idx_hosts_status_last_seen ON hosts (status, last_seen)   WHERE status = 'up';
CREATE INDEX IF NOT EXISTS idx_hosts_os_family_status ON hosts (os_family, status)   WHERE os_family IS NOT NULL AND status = 'up';
CREATE INDEX IF NOT EXISTS idx_hosts_discovery_status ON hosts (discovery_method, status, last_seen)
    WHERE discovery_method IS NOT NULL;

-- Network exclusions (depends on networks)
CREATE TABLE IF NOT EXISTS network_exclusions (
    id            UUID        PRIMARY KEY DEFAULT uuid_generate_v4(),
    network_id    UUID        REFERENCES networks(id) ON DELETE CASCADE,
    excluded_cidr CIDR        NOT NULL,
    reason        TEXT,
    enabled       BOOLEAN     NOT NULL DEFAULT true,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    created_by    VARCHAR(100)
);

COMMENT ON TABLE  network_exclusions           IS 'IP addresses and ranges to exclude from discovery and scanning';
COMMENT ON COLUMN network_exclusions.network_id    IS 'Network to apply exclusion to, NULL for global exclusions';
COMMENT ON COLUMN network_exclusions.excluded_cidr IS 'CIDR range to exclude, use /32 for single IPs';
COMMENT ON COLUMN network_exclusions.reason        IS 'Human-readable reason for exclusion (e.g., "Router", "Critical server")';

CREATE INDEX IF NOT EXISTS idx_network_exclusions_network_id ON network_exclusions (network_id);
CREATE INDEX IF NOT EXISTS idx_network_exclusions_enabled    ON network_exclusions (enabled)      WHERE enabled = true;
CREATE INDEX IF NOT EXISTS idx_network_exclusions_cidr       ON network_exclusions USING gist (excluded_cidr);
CREATE INDEX IF NOT EXISTS idx_network_exclusions_created_at ON network_exclusions (created_at);
CREATE UNIQUE INDEX IF NOT EXISTS idx_network_exclusions_unique
    ON network_exclusions (network_id, excluded_cidr)
    WHERE enabled = true;

-- Port scans (depends on scan_jobs and hosts)
CREATE TABLE IF NOT EXISTS port_scans (
    id              UUID        PRIMARY KEY DEFAULT uuid_generate_v4(),
    job_id          UUID        NOT NULL REFERENCES scan_jobs(id)  ON DELETE CASCADE,
    host_id         UUID        NOT NULL REFERENCES hosts(id)      ON DELETE CASCADE,
    port            INTEGER     NOT NULL CHECK (port BETWEEN 1 AND 65535),
    protocol        VARCHAR(10) DEFAULT 'tcp' CHECK (protocol IN ('tcp', 'udp')),
    state           VARCHAR(20) NOT NULL CHECK (state IN ('open', 'closed', 'filtered', 'unknown')),
    service_name    VARCHAR(100),
    service_version VARCHAR(255),
    service_product VARCHAR(255),
    banner          TEXT,
    scanned_at      TIMESTAMPTZ DEFAULT NOW(),
    CONSTRAINT unique_host_port_protocol_scan UNIQUE (job_id, host_id, port, protocol)
);

COMMENT ON TABLE port_scans IS 'Port scan results for each host';

CREATE INDEX IF NOT EXISTS idx_port_scans_host_id           ON port_scans (host_id);
CREATE INDEX IF NOT EXISTS idx_port_scans_job_id            ON port_scans (job_id);
CREATE INDEX IF NOT EXISTS idx_port_scans_port              ON port_scans (port);
CREATE INDEX IF NOT EXISTS idx_port_scans_state             ON port_scans (state)           WHERE state = 'open';
CREATE INDEX IF NOT EXISTS idx_port_scans_service_name      ON port_scans (service_name)    WHERE service_name IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_port_scans_job_host          ON port_scans (job_id, host_id);
CREATE INDEX IF NOT EXISTS idx_port_scans_host_port_state   ON port_scans (host_id, port, state) WHERE state = 'open';
CREATE INDEX IF NOT EXISTS idx_port_scans_scanned_at        ON port_scans (scanned_at);
CREATE INDEX IF NOT EXISTS idx_port_scans_service_detection ON port_scans (service_name, service_version)
    WHERE service_name IS NOT NULL;

-- Services (depends on port_scans)
CREATE TABLE IF NOT EXISTS services (
    id           UUID        PRIMARY KEY DEFAULT uuid_generate_v4(),
    port_scan_id UUID        NOT NULL REFERENCES port_scans(id) ON DELETE CASCADE,
    service_type VARCHAR(100),
    version      VARCHAR(255),
    cpe          VARCHAR(255),
    confidence   INTEGER     CHECK (confidence BETWEEN 0 AND 100),
    details      JSONB,
    detected_at  TIMESTAMPTZ DEFAULT NOW()
);

COMMENT ON TABLE services IS 'Detailed service detection results';

CREATE INDEX IF NOT EXISTS idx_services_port_scan_id ON services (port_scan_id);
CREATE INDEX IF NOT EXISTS idx_services_service_type ON services (service_type);
CREATE INDEX IF NOT EXISTS idx_services_details      ON services USING GIN (details);

-- Host history (depends on hosts and scan_jobs)
CREATE TABLE IF NOT EXISTS host_history (
    id            UUID        PRIMARY KEY DEFAULT uuid_generate_v4(),
    host_id       UUID        NOT NULL REFERENCES hosts(id)     ON DELETE CASCADE,
    job_id        UUID        NOT NULL REFERENCES scan_jobs(id) ON DELETE CASCADE,
    event_type    VARCHAR(50) NOT NULL,
    old_value     JSONB,
    new_value     JSONB,
    created_at    TIMESTAMPTZ DEFAULT NOW(),
    changed_by    VARCHAR(100),
    change_reason TEXT,
    client_ip     INET
);

COMMENT ON TABLE host_history IS 'Audit trail of host changes over time';

CREATE INDEX IF NOT EXISTS idx_host_history_host_id     ON host_history (host_id);
CREATE INDEX IF NOT EXISTS idx_host_history_created_at  ON host_history (created_at);
CREATE INDEX IF NOT EXISTS idx_host_history_event_type  ON host_history (event_type);
CREATE INDEX IF NOT EXISTS idx_host_history_host_created ON host_history (host_id, created_at);

-- API keys (no FK dependencies)
CREATE TABLE IF NOT EXISTS api_keys (
    id           UUID                     PRIMARY KEY DEFAULT uuid_generate_v4(),
    name         VARCHAR(255)             NOT NULL,
    key_hash     VARCHAR(255)             UNIQUE NOT NULL,
    key_prefix   VARCHAR(20)              NOT NULL,
    created_at   TIMESTAMP WITH TIME ZONE DEFAULT NOW() NOT NULL,
    updated_at   TIMESTAMP WITH TIME ZONE DEFAULT NOW() NOT NULL,
    last_used_at TIMESTAMP WITH TIME ZONE,
    expires_at   TIMESTAMP WITH TIME ZONE,
    is_active    BOOLEAN                  DEFAULT true NOT NULL,
    usage_count  INTEGER                  DEFAULT 0 NOT NULL,
    permissions  JSONB,
    created_by   UUID,
    notes        TEXT,
    CONSTRAINT api_keys_name_length
        CHECK (char_length(name) >= 1 AND char_length(name) <= 255),
    CONSTRAINT api_keys_key_prefix_length
        CHECK (char_length(key_prefix) >= 8 AND char_length(key_prefix) <= 20),
    CONSTRAINT api_keys_usage_count_positive
        CHECK (usage_count >= 0),
    CONSTRAINT api_keys_expires_after_created
        CHECK (expires_at IS NULL OR expires_at > created_at)
);

COMMENT ON TABLE  api_keys            IS 'Runtime-managed API keys for authentication, replacing static configuration-based keys';
COMMENT ON COLUMN api_keys.key_hash   IS 'bcrypt hash of the actual API key (never store plaintext keys)';
COMMENT ON COLUMN api_keys.key_prefix IS 'Display-safe prefix of the key for identification in UI (e.g., sk_abc123...)';
COMMENT ON COLUMN api_keys.permissions IS 'JSONB object containing granular permissions (deprecated - use roles instead)';
COMMENT ON COLUMN api_keys.usage_count IS 'Number of times this key has been used for authentication';
COMMENT ON COLUMN api_keys.expires_at  IS 'Optional expiration timestamp - key becomes invalid after this time';

CREATE INDEX IF NOT EXISTS idx_api_keys_key_hash    ON api_keys (key_hash)                  WHERE is_active = true;
CREATE INDEX IF NOT EXISTS idx_api_keys_active      ON api_keys (is_active)                 WHERE is_active = true;
CREATE INDEX IF NOT EXISTS idx_api_keys_expires_at  ON api_keys (expires_at)                WHERE expires_at IS NOT NULL AND is_active = true;
CREATE INDEX IF NOT EXISTS idx_api_keys_last_used   ON api_keys (last_used_at DESC)          WHERE is_active = true;
CREATE INDEX IF NOT EXISTS idx_api_keys_created_at  ON api_keys (created_at DESC);
CREATE INDEX IF NOT EXISTS idx_api_keys_active_valid ON api_keys (key_hash, last_used_at)   WHERE is_active = true;

-- Roles (no FK dependencies)
CREATE TABLE IF NOT EXISTS roles (
    id          UUID                     PRIMARY KEY DEFAULT uuid_generate_v4(),
    name        VARCHAR(100)             UNIQUE NOT NULL,
    description TEXT,
    permissions JSONB                    DEFAULT '{}' NOT NULL,
    is_active   BOOLEAN                  DEFAULT true NOT NULL,
    is_system   BOOLEAN                  DEFAULT false NOT NULL,
    created_at  TIMESTAMP WITH TIME ZONE DEFAULT NOW() NOT NULL,
    updated_at  TIMESTAMP WITH TIME ZONE DEFAULT NOW() NOT NULL,
    created_by  UUID,
    CONSTRAINT roles_name_length
        CHECK (char_length(name) >= 1 AND char_length(name) <= 100),
    CONSTRAINT roles_name_format
        CHECK (name ~ '^[a-zA-Z][a-zA-Z0-9_-]*$')
);

COMMENT ON TABLE  roles            IS 'Roles for role-based access control (RBAC) system';
COMMENT ON COLUMN roles.name        IS 'Unique role name (e.g., admin, readonly, operator)';
COMMENT ON COLUMN roles.description IS 'Human-readable description of the role';
COMMENT ON COLUMN roles.permissions IS 'JSONB object defining what actions this role can perform';
COMMENT ON COLUMN roles.is_system   IS 'System roles cannot be deleted and are managed by code';

CREATE INDEX IF NOT EXISTS idx_roles_active ON roles (is_active) WHERE is_active = true;
CREATE INDEX IF NOT EXISTS idx_roles_name   ON roles (name)      WHERE is_active = true;
CREATE INDEX IF NOT EXISTS idx_roles_system ON roles (is_system);

-- API key roles — junction table (depends on api_keys and roles)
CREATE TABLE IF NOT EXISTS api_key_roles (
    api_key_id UUID                     NOT NULL REFERENCES api_keys(id) ON DELETE CASCADE,
    role_id    UUID                     NOT NULL REFERENCES roles(id)    ON DELETE CASCADE,
    granted_at TIMESTAMP WITH TIME ZONE DEFAULT NOW() NOT NULL,
    granted_by UUID,
    PRIMARY KEY (api_key_id, role_id)
);

COMMENT ON TABLE  api_key_roles            IS 'Many-to-many relationship between API keys and roles';
COMMENT ON COLUMN api_key_roles.granted_at IS 'When this role was granted to the API key';
COMMENT ON COLUMN api_key_roles.granted_by IS 'Admin user who granted this role (future feature)';

CREATE INDEX IF NOT EXISTS idx_api_key_roles_api_key    ON api_key_roles (api_key_id);
CREATE INDEX IF NOT EXISTS idx_api_key_roles_role       ON api_key_roles (role_id);
CREATE INDEX IF NOT EXISTS idx_api_key_roles_granted_at ON api_key_roles (granted_at DESC);

-- DNS cache (no FK dependencies)
CREATE TABLE IF NOT EXISTS dns_cache (
    id             UUID        PRIMARY KEY DEFAULT uuid_generate_v4(),
    direction      VARCHAR(8)  NOT NULL CHECK (direction IN ('forward', 'reverse')),
    lookup_key     TEXT        NOT NULL,
    resolved_value TEXT        NOT NULL DEFAULT '',
    resolved_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    ttl_seconds    INTEGER     NOT NULL DEFAULT 3600 CHECK (ttl_seconds > 0),
    last_error     TEXT,
    CONSTRAINT uq_dns_cache_entry UNIQUE (direction, lookup_key, resolved_value)
);

COMMENT ON TABLE  dns_cache                IS 'Server-side DNS lookup cache for both forward (name->IP) and reverse (IP->name) resolutions';
COMMENT ON COLUMN dns_cache.direction      IS '"forward" for A/AAAA queries, "reverse" for PTR queries';
COMMENT ON COLUMN dns_cache.lookup_key     IS 'The name or IP that was looked up';
COMMENT ON COLUMN dns_cache.resolved_value IS 'The result of the lookup; empty string for successful but empty responses';
COMMENT ON COLUMN dns_cache.resolved_at    IS 'Wall-clock time of the most recent lookup attempt';
COMMENT ON COLUMN dns_cache.ttl_seconds    IS 'Seconds until this entry is considered stale and should be re-resolved';
COMMENT ON COLUMN dns_cache.last_error     IS 'Resolver error from the last attempt, NULL when the last attempt succeeded';

CREATE INDEX IF NOT EXISTS idx_dns_cache_lookup      ON dns_cache (direction, lookup_key);
CREATE INDEX IF NOT EXISTS idx_dns_cache_resolved_at ON dns_cache (resolved_at);

-- ===========================================================================
-- Triggers
-- ===========================================================================

DROP TRIGGER IF EXISTS trigger_networks_updated_at ON networks;
CREATE TRIGGER trigger_networks_updated_at
    BEFORE UPDATE ON networks
    FOR EACH ROW EXECUTE FUNCTION update_networks_updated_at();

DROP TRIGGER IF EXISTS trigger_update_network_counts_insert ON hosts;
CREATE TRIGGER trigger_update_network_counts_insert
    AFTER INSERT ON hosts
    FOR EACH ROW EXECUTE FUNCTION update_network_host_counts();

DROP TRIGGER IF EXISTS trigger_update_network_counts_update ON hosts;
CREATE TRIGGER trigger_update_network_counts_update
    AFTER UPDATE OF ip_address, status ON hosts
    FOR EACH ROW EXECUTE FUNCTION update_network_host_counts();

DROP TRIGGER IF EXISTS trigger_update_network_counts_delete ON hosts;
CREATE TRIGGER trigger_update_network_counts_delete
    AFTER DELETE ON hosts
    FOR EACH ROW EXECUTE FUNCTION update_network_host_counts();

DROP TRIGGER IF EXISTS trigger_network_exclusions_updated_at ON network_exclusions;
CREATE TRIGGER trigger_network_exclusions_updated_at
    BEFORE UPDATE ON network_exclusions
    FOR EACH ROW EXECUTE FUNCTION update_network_exclusions_updated_at();

DROP TRIGGER IF EXISTS update_host_last_seen_trigger ON port_scans;
CREATE TRIGGER update_host_last_seen_trigger
    AFTER INSERT ON port_scans
    FOR EACH ROW EXECUTE FUNCTION update_host_last_seen();

DROP TRIGGER IF EXISTS trigger_api_keys_updated_at ON api_keys;
CREATE TRIGGER trigger_api_keys_updated_at
    BEFORE UPDATE ON api_keys
    FOR EACH ROW EXECUTE FUNCTION update_api_keys_updated_at();

-- ===========================================================================
-- Views
-- ===========================================================================

-- Active hosts with port counts
CREATE OR REPLACE VIEW active_hosts AS
SELECT
    h.ip_address,
    h.hostname,
    h.mac_address,
    h.vendor,
    h.status,
    h.last_seen,
    COUNT(ps.id) FILTER (WHERE ps.state = 'open') AS open_ports,
    COUNT(ps.id)                                   AS total_ports_scanned
FROM hosts h
LEFT JOIN port_scans ps ON h.id = ps.host_id
WHERE h.status = 'up'
GROUP BY h.id, h.ip_address, h.hostname, h.mac_address, h.vendor, h.status, h.last_seen;

-- Network summary (uses final networks-based schema from migration 011)
CREATE OR REPLACE VIEW network_summary AS
SELECT
    n.name                                                         AS target_name,
    n.cidr::text                                                   AS network,
    COUNT(DISTINCT h.id) FILTER (WHERE h.status = 'up')           AS active_hosts,
    COUNT(DISTINCT h.id)                                           AS total_hosts,
    COUNT(DISTINCT ps.id) FILTER (WHERE ps.state = 'open')        AS open_ports,
    MAX(sj.completed_at)                                           AS last_scan
FROM   networks  n
LEFT JOIN scan_jobs   sj ON n.id   = sj.network_id AND sj.status = 'completed'
LEFT JOIN hosts       h  ON h.ip_address << n.cidr
LEFT JOIN port_scans  ps ON h.id   = ps.host_id
WHERE  n.is_active    = true
  AND  n.scan_enabled = true
GROUP  BY n.id, n.name, n.cidr;

-- Daily scan performance metrics
CREATE OR REPLACE VIEW scan_performance_stats AS
SELECT
    DATE_TRUNC('day', sj.created_at)                                         AS scan_date,
    COUNT(*)                                                                  AS total_jobs,
    COUNT(*) FILTER (WHERE sj.status = 'completed')                          AS completed_jobs,
    COUNT(*) FILTER (WHERE sj.status = 'failed')                             AS failed_jobs,
    AVG(EXTRACT(EPOCH FROM (sj.completed_at - sj.started_at)))               AS avg_duration_seconds,
    SUM(COALESCE((sj.scan_stats->>'total_hosts')::INTEGER, 0))               AS total_hosts_scanned,
    SUM(COALESCE((sj.scan_stats->>'hosts_up')::INTEGER, 0))                  AS total_hosts_up
FROM scan_jobs sj
WHERE sj.created_at > NOW() - INTERVAL '30 days'
GROUP BY DATE_TRUNC('day', sj.created_at)
ORDER BY scan_date DESC;

COMMENT ON VIEW scan_performance_stats IS 'Daily scan performance metrics for monitoring and reporting';

-- Scan type usage statistics (uses final networks-based schema from migration 011)
CREATE OR REPLACE VIEW scan_type_usage_stats AS
SELECT
    n.scan_type,
    COUNT(DISTINCT n.id)                                      AS target_count,
    COUNT(sj.id)                                              AS total_jobs,
    COUNT(sj.id) FILTER (WHERE sj.status = 'completed')      AS completed_jobs,
    COUNT(sj.id) FILTER (WHERE sj.status = 'failed')         AS failed_jobs,
    MAX(sj.completed_at)                                      AS last_used
FROM   networks  n
LEFT JOIN scan_jobs sj ON n.id = sj.network_id
GROUP  BY n.scan_type
ORDER  BY total_jobs DESC;

COMMENT ON VIEW scan_type_usage_stats IS 'Statistics showing usage patterns of different scan types';

-- ===========================================================================
-- Materialized views
-- ===========================================================================

-- Denormalized host summary for dashboard performance
CREATE MATERIALIZED VIEW IF NOT EXISTS host_summary AS
SELECT
    h.id,
    h.ip_address,
    h.hostname,
    h.mac_address,
    h.vendor,
    h.os_family,
    h.os_name,
    h.status,
    h.last_seen,
    h.first_seen,
    h.discovery_count,
    COUNT(ps.id) FILTER (WHERE ps.state = 'open') AS open_ports,
    COUNT(ps.id)                                   AS total_ports_scanned,
    MAX(ps.scanned_at)                             AS last_scanned,
    COUNT(DISTINCT ps.job_id)                      AS scan_job_count
FROM hosts h
LEFT JOIN port_scans ps ON h.id = ps.host_id
GROUP BY h.id, h.ip_address, h.hostname, h.mac_address, h.vendor,
         h.os_family, h.os_name, h.status, h.last_seen, h.first_seen, h.discovery_count;

CREATE UNIQUE INDEX IF NOT EXISTS idx_host_summary_id             ON host_summary (id);
CREATE INDEX        IF NOT EXISTS idx_host_summary_ip             ON host_summary (ip_address);
CREATE INDEX        IF NOT EXISTS idx_host_summary_status_last_seen ON host_summary (status, last_seen);

-- Network statistics materialized view (uses final networks-based schema from migration 011)
CREATE MATERIALIZED VIEW IF NOT EXISTS network_summary_mv AS
SELECT
    n.id                                                                        AS target_id,
    n.name                                                                      AS target_name,
    n.cidr::text                                                                AS network,
    n.is_active                                                                 AS enabled,
    COUNT(DISTINCT sj.id)                                                       AS total_scans,
    COUNT(DISTINCT sj.id) FILTER (WHERE sj.status = 'completed')               AS completed_scans,
    COUNT(DISTINCT sj.id) FILTER (WHERE sj.status = 'failed')                  AS failed_scans,
    MAX(sj.completed_at)                                                        AS last_scan_at,
    MIN(sj.created_at)                                                          AS first_scan_at,
    AVG(EXTRACT(EPOCH FROM (sj.completed_at - sj.started_at)))                 AS avg_duration_seconds,
    COUNT(DISTINCT ps.host_id)                                                  AS unique_hosts_scanned
FROM   networks  n
LEFT JOIN scan_jobs   sj ON n.id = sj.network_id
LEFT JOIN port_scans  ps ON sj.id = ps.job_id
GROUP  BY n.id, n.name, n.cidr, n.is_active
WITH DATA;

CREATE UNIQUE INDEX IF NOT EXISTS idx_network_summary_mv_target  ON network_summary_mv (target_id);
CREATE INDEX        IF NOT EXISTS idx_network_summary_mv_network ON network_summary_mv (network);

-- ===========================================================================
-- Data: system defaults (no sample/environment-specific data)
-- ===========================================================================

-- Default RBAC roles
INSERT INTO roles (name, description, permissions, is_system) VALUES
    ('admin',    'Full administrative access to all resources',
     '{"*": ["*"]}', true),
    ('readonly', 'Read-only access to all resources',
     '{"*": ["read"]}', true),
    ('operator', 'Operational access for scans and discovery',
     '{"scans": ["*"], "discovery": ["*"], "hosts": ["read"], "networks": ["read"]}', true)
ON CONFLICT (name) DO NOTHING;

-- Initial admin API key (change immediately in production)
DO $$
DECLARE
    admin_key_exists BOOLEAN;
    new_key_id       UUID;
    admin_role_id    UUID;
BEGIN
    SELECT EXISTS(SELECT 1 FROM api_keys WHERE is_active = true) INTO admin_key_exists;

    IF NOT admin_key_exists THEN
        INSERT INTO api_keys (name, key_hash, key_prefix, notes, created_at)
        VALUES (
            'Initial Admin Key (CHANGE IMMEDIATELY)',
            '$2a$12$LQv3c1yqBWVHxkd0LHAkCOYz6TtxMQJqhN8/lewQ9L93Y0X9F8/oa',
            'sk_initial...',
            'Default admin key created during migration. Change this immediately for security.',
            NOW()
        ) RETURNING id INTO new_key_id;

        SELECT id INTO admin_role_id FROM roles WHERE name = 'admin';
        INSERT INTO api_key_roles (api_key_id, role_id) VALUES (new_key_id, admin_role_id);
    END IF;
END $$;

-- Built-in scan profiles (final versions from migration 009)
INSERT INTO scan_profiles (id, name, description, os_family, os_pattern, ports, scan_type, timing, scripts, options, priority, built_in) VALUES

('windows-server',
 'Windows Server',
 'Comprehensive scan for Windows servers',
 ARRAY['windows'], ARRAY[]::TEXT[],
 'T:21,22,25,53,80,110,135,139,143,389,443,445,464,465,587,593,636,1433,3268,3269,3389,5985,5986,8080,8443,49152-49157,U:53,88,123,137,138,161,162,389,445,500,4500',
 'syn', 'normal', ARRAY[]::TEXT[], '{"os_detection": false}', 90, true),

('windows-workstation',
 'Windows Workstation',
 'Scan for Windows workstations',
 ARRAY['windows'], ARRAY[]::TEXT[],
 'T:80,135,139,443,445,1433,3389,5985,8080,49152-49155,U:123,137,138,500',
 'syn', 'normal', ARRAY[]::TEXT[], '{"os_detection": false}', 80, true),

('linux-server',
 'Linux Server',
 'Comprehensive scan for Linux servers',
 ARRAY['linux'], ARRAY[]::TEXT[],
 'T:21,22,25,53,80,111,143,443,465,587,2049,3306,5432,6379,8080,8443,9200,9300,27017,U:53,111,123,161,162,514,2049',
 'syn', 'normal', ARRAY[]::TEXT[], '{"os_detection": false}', 90, true),

('linux-workstation',
 'Linux Workstation',
 'Scan for Linux workstations',
 ARRAY['linux'], ARRAY[]::TEXT[],
 'T:22,80,443,631,5900,8080,U:631,5353',
 'syn', 'normal', ARRAY[]::TEXT[], '{"os_detection": false}', 80, true),

('macos',
 'macOS',
 'Scan for macOS systems',
 ARRAY['macos'], ARRAY[]::TEXT[],
 'T:22,80,443,445,548,631,3283,5900,7000,8080,U:123,631,5353',
 'syn', 'normal', ARRAY[]::TEXT[], '{"os_detection": false}', 85, true),

('generic',
 'Generic',
 'Default scan for unknown OS',
 ARRAY[]::TEXT[], ARRAY[]::TEXT[],
 'T:21,22,23,25,53,80,110,143,443,445,3389,8080,8443,U:53,123,161,500',
 'syn', 'normal', ARRAY[]::TEXT[], '{"os_detection": false}', 10, true)

ON CONFLICT (id) DO NOTHING;
