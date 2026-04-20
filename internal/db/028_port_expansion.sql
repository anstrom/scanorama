-- Migration 028: expanded port definitions and SmartScan port settings.
-- Adds curated port/service entries covering Prometheus exporters,
-- modern data infrastructure, virtualization, service mesh, and DevOps tooling.
-- Also seeds per-stage base-port settings keys for operator configurability.

-- ── Prometheus exporters ──────────────────────────────────────────────────────
INSERT INTO port_definitions (port, protocol, service, description, category, os_families, is_standard) VALUES
(9091,  'tcp', 'prometheus-pushgateway', 'Prometheus Pushgateway',              'monitoring', '{linux}', false),
(9101,  'tcp', 'haproxy-exporter',       'HAProxy Prometheus exporter',          'monitoring', '{linux}', false),
(9102,  'tcp', 'statsd-exporter',        'StatsD Prometheus exporter',           'monitoring', '{linux}', false),
(9113,  'tcp', 'nginx-exporter',         'nginx Prometheus exporter',            'monitoring', '{linux}', false),
(9115,  'tcp', 'blackbox-exporter',      'Prometheus Blackbox exporter',         'monitoring', '{linux}', false),
(9116,  'tcp', 'snmp-exporter',          'Prometheus SNMP exporter',             'monitoring', '{linux}', false),
(9121,  'tcp', 'redis-exporter',         'Redis Prometheus exporter',            'monitoring', '{linux}', false),
(9216,  'tcp', 'mongodb-exporter',       'MongoDB Prometheus exporter',          'monitoring', '{linux}', false),
(9308,  'tcp', 'kafka-exporter',         'Kafka Prometheus exporter',            'monitoring', '{linux}', false),
(9419,  'tcp', 'rabbitmq-exporter',      'RabbitMQ Prometheus exporter',         'monitoring', '{linux}', false),
(8085,  'tcp', 'cadvisor',               'cAdvisor container metrics',           'monitoring', '{linux}', false),
(9999,  'tcp', 'jmx-exporter',           'Prometheus JMX exporter (default)',    'monitoring', '{linux}', false)
ON CONFLICT (port, protocol) DO NOTHING;

-- ── Identity / auth ───────────────────────────────────────────────────────────
INSERT INTO port_definitions (port, protocol, service, description, category, os_families, is_standard) VALUES
(5556,  'tcp', 'dex',                    'Dex OIDC identity provider',           'security',   '{linux}', false)
ON CONFLICT (port, protocol) DO NOTHING;

-- ── Virtualization ────────────────────────────────────────────────────────────
INSERT INTO port_definitions (port, protocol, service, description, category, os_families, is_standard) VALUES
(8006,  'tcp', 'proxmox-web',            'Proxmox VE web UI (HTTPS)',             'virtualization', '{}', true),
(9443,  'tcp', 'vsphere-https',          'VMware vSphere / ESXi HTTPS',          'virtualization', '{}', true),
(5405,  'udp', 'corosync',               'Proxmox cluster / Corosync heartbeat', 'virtualization', '{}', false)
ON CONFLICT (port, protocol) DO NOTHING;

-- ── Modern data infrastructure ────────────────────────────────────────────────
INSERT INTO port_definitions (port, protocol, service, description, category, os_families, is_standard) VALUES
(8123,  'tcp', 'clickhouse-http',        'ClickHouse HTTP interface',             'database',   '{linux}', true),
(9440,  'tcp', 'clickhouse-https',       'ClickHouse native TLS interface',       'database',   '{linux}', false),
(26257, 'tcp', 'cockroachdb',            'CockroachDB SQL wire + admin HTTP',    'database',   '{linux}', true),
(4222,  'tcp', 'nats',                   'NATS messaging client port',            'messaging',  '{linux}', true),
(8222,  'tcp', 'nats-monitor',           'NATS HTTP monitoring interface',        'messaging',  '{linux}', false),
(6222,  'tcp', 'nats-cluster',           'NATS cluster routing port',             'messaging',  '{linux}', false),
(6650,  'tcp', 'pulsar',                 'Apache Pulsar broker service',          'messaging',  '{linux}', true),
(9001,  'tcp', 'minio-console',          'MinIO web console',                     'storage',    '{linux}', false),
(6333,  'tcp', 'qdrant',                 'Qdrant vector database HTTP API',       'database',   '{linux}', false),
(19530, 'tcp', 'milvus',                 'Milvus vector database gRPC',           'database',   '{linux}', false),
(8428,  'tcp', 'victoriametrics',        'VictoriaMetrics HTTP API',              'monitoring', '{linux}', false)
ON CONFLICT (port, protocol) DO NOTHING;

-- ── DevOps / GitOps ───────────────────────────────────────────────────────────
INSERT INTO port_definitions (port, protocol, service, description, category, os_families, is_standard) VALUES
(4646,  'tcp', 'nomad-http',             'HashiCorp Nomad HTTP API',              'devops',     '{linux}', false),
(4647,  'tcp', 'nomad-rpc',              'HashiCorp Nomad RPC',                   'devops',     '{linux}', false),
(4648,  'tcp', 'nomad-serf',             'HashiCorp Nomad Serf',                  'devops',     '{linux}', false),
(2746,  'tcp', 'argo-workflows',         'Argo Workflows server',                 'devops',     '{linux}', false),
(8075,  'tcp', 'gitaly',                 'GitLab Gitaly Git service',             'devops',     '{linux}', false)
ON CONFLICT (port, protocol) DO NOTHING;

-- ── Service mesh / proxy ──────────────────────────────────────────────────────
INSERT INTO port_definitions (port, protocol, service, description, category, os_families, is_standard) VALUES
(15012, 'tcp', 'istio-pilot',            'Istio pilot discovery gRPC',            'network',    '{linux}', false),
(15021, 'tcp', 'istio-health',           'Istio health check endpoint',           'network',    '{linux}', false),
(9901,  'tcp', 'envoy-admin',            'Envoy proxy admin interface',           'network',    '{linux}', false),
(4140,  'tcp', 'linkerd-proxy',          'Linkerd proxy inbound port',            'network',    '{linux}', false),
(4191,  'tcp', 'linkerd-admin',          'Linkerd admin interface',               'network',    '{linux}', false),
(1936,  'tcp', 'haproxy-stats',          'HAProxy statistics page',               'network',    '{linux}', false),
(10902, 'tcp', 'thanos-sidecar',         'Thanos sidecar gRPC',                   'monitoring', '{linux}', false)
ON CONFLICT (port, protocol) DO NOTHING;

-- ── Storage ───────────────────────────────────────────────────────────────────
INSERT INTO port_definitions (port, protocol, service, description, category, os_families, is_standard) VALUES
(6789,  'tcp', 'ceph-mon',               'Ceph Monitor port',                     'storage',    '{linux}', true),
(24007, 'tcp', 'glusterfs',              'GlusterFS daemon port',                 'storage',    '{linux}', true)
ON CONFLICT (port, protocol) DO NOTHING;

-- ── SmartScan per-stage base port settings ────────────────────────────────────
INSERT INTO settings (key, value, type, description) VALUES
    ('smartscan.os_detection.ports',
     '"22,80,135,443,445,3389"',
     'string',
     'Base port list for the os_detection SmartScan stage'),
    ('smartscan.identity_enrichment.ports',
     '"22,80,161,443"',
     'string',
     'Base port list for the identity_enrichment SmartScan stage'),
    ('smartscan.refresh.ports',
     '"1-1024"',
     'string',
     'Base port list for the refresh SmartScan stage'),
    ('smartscan.top_ports_limit',
     '256',
     'int',
     'Maximum merged port count across all sources (0 = use default 256)')
ON CONFLICT (key) DO NOTHING;
