CREATE TABLE IF NOT EXISTS settings (
    key         VARCHAR(100) PRIMARY KEY,
    value       JSONB        NOT NULL,
    description TEXT,
    type        VARCHAR(20)  NOT NULL DEFAULT 'string',
    updated_at  TIMESTAMPTZ  NOT NULL DEFAULT NOW()
);

-- Seed mutable settings with defaults
INSERT INTO settings (key, value, type, description) VALUES
    ('scan.default_timing',         '3',            'int',      'Default nmap timing template (0-5)'),
    ('scan.max_concurrent',         '5',            'int',      'Maximum concurrent scans'),
    ('discovery.ping_timeout_ms',   '1000',         'int',      'Ping timeout in milliseconds'),
    ('discovery.methods',           '["icmp","arp"]','string[]', 'Discovery methods'),
    ('retention.auto_purge_days',   '0',            'int',      'Auto-purge scan data older than N days (0=disabled)'),
    ('retention.max_scan_history',  '0',            'int',      'Max scan history per host (0=unlimited)'),
    ('notifications.scan_complete', 'true',         'bool',     'Notify on scan completion'),
    ('notifications.host_down',     'true',         'bool',     'Notify when host goes down'),
    ('notifications.new_host',      'true',         'bool',     'Notify on new host discovery')
ON CONFLICT (key) DO NOTHING;
