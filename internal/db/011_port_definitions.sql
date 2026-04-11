-- Migration 010: curated port definitions table
-- Stores well-known port/protocol pairs with service names and metadata.
-- Used for port browser UI and service fingerprinting enrichment.

CREATE TABLE IF NOT EXISTS port_definitions (
    port        INT          NOT NULL,
    protocol    VARCHAR(10)  NOT NULL,
    service     VARCHAR(100) NOT NULL,
    description TEXT,
    category    VARCHAR(50),
    os_families TEXT[]       DEFAULT '{}',
    is_standard BOOLEAN      NOT NULL DEFAULT TRUE,
    PRIMARY KEY (port, protocol)
);

-- Seed well-known TCP ports.
INSERT INTO port_definitions (port, protocol, service, description, category, os_families, is_standard) VALUES
-- Web / HTTP
(80,   'tcp', 'http',        'Hypertext Transfer Protocol',                          'web',        '{}',                        true),
(443,  'tcp', 'https',       'HTTP over TLS/SSL',                                    'web',        '{}',                        true),
(8080, 'tcp', 'http-alt',    'HTTP alternate (proxy, dev servers)',                  'web',        '{}',                        true),
(8443, 'tcp', 'https-alt',   'HTTPS alternate',                                      'web',        '{}',                        true),
(8888, 'tcp', 'http-dev',    'HTTP dev server (Jupyter, various)',                   'web',        '{}',                        false),
(3000, 'tcp', 'http-dev',    'HTTP dev server (Node.js, Rails, etc.)',               'web',        '{}',                        false),
(4000, 'tcp', 'http-dev',    'HTTP dev server (Phoenix, etc.)',                      'web',        '{}',                        false),
(5000, 'tcp', 'http-dev',    'HTTP dev server (Flask, Docker Registry)',             'web',        '{}',                        false),
(9090, 'tcp', 'http-alt',    'HTTP alternate (Prometheus, Cockpit)',                 'web',        '{}',                        false),

-- Remote access / shell
(22,   'tcp', 'ssh',         'Secure Shell',                                         'remote',     '{linux,macos,network}',     true),
(23,   'tcp', 'telnet',      'Telnet — unencrypted remote shell',                    'remote',     '{network,iot}',             true),
(3389, 'tcp', 'rdp',         'Remote Desktop Protocol',                             'remote',     '{windows}',                 true),
(5900, 'tcp', 'vnc',         'Virtual Network Computing',                            'remote',     '{}',                        true),
(5901, 'tcp', 'vnc-1',       'VNC display :1',                                       'remote',     '{}',                        true),

-- Windows / SMB / AD
(135,  'tcp', 'msrpc',       'Microsoft RPC Endpoint Mapper',                        'windows',    '{windows}',                 true),
(137,  'tcp', 'netbios-ns',  'NetBIOS Name Service',                                 'windows',    '{windows}',                 true),
(138,  'tcp', 'netbios-dgm', 'NetBIOS Datagram Service',                             'windows',    '{windows}',                 true),
(139,  'tcp', 'netbios-ssn', 'NetBIOS Session Service',                              'windows',    '{windows}',                 true),
(445,  'tcp', 'smb',         'Server Message Block (SMB/CIFS)',                      'windows',    '{windows}',                 true),
(5985, 'tcp', 'winrm-http',  'Windows Remote Management (HTTP)',                     'windows',    '{windows}',                 true),
(5986, 'tcp', 'winrm-https', 'Windows Remote Management (HTTPS)',                    'windows',    '{windows}',                 true),
(88,   'tcp', 'kerberos',    'Kerberos authentication',                              'windows',    '{windows,linux}',           true),
(389,  'tcp', 'ldap',        'Lightweight Directory Access Protocol',                'windows',    '{windows,linux}',           true),
(636,  'tcp', 'ldaps',       'LDAP over TLS/SSL',                                    'windows',    '{windows,linux}',           true),
(3268, 'tcp', 'ldap-gc',     'Active Directory Global Catalog',                      'windows',    '{windows}',                 true),
(3269, 'tcp', 'ldaps-gc',    'Active Directory Global Catalog over TLS',             'windows',    '{windows}',                 true),

-- Databases
(1433, 'tcp', 'mssql',       'Microsoft SQL Server',                                 'database',   '{windows}',                 true),
(1434, 'tcp', 'mssql-mon',   'Microsoft SQL Server Monitor (UDP discovery)',         'database',   '{windows}',                 false),
(1521, 'tcp', 'oracle',      'Oracle Database listener',                             'database',   '{}',                        true),
(3306, 'tcp', 'mysql',       'MySQL / MariaDB',                                      'database',   '{linux,windows}',           true),
(5432, 'tcp', 'postgresql',  'PostgreSQL',                                           'database',   '{linux,windows}',           true),
(6379, 'tcp', 'redis',       'Redis in-memory data store',                           'database',   '{linux}',                   true),
(27017,'tcp', 'mongodb',     'MongoDB document database',                            'database',   '{linux,windows}',           true),
(27018,'tcp', 'mongodb-shard','MongoDB shard server',                               'database',   '{linux}',                   false),
(9200, 'tcp', 'elasticsearch','Elasticsearch HTTP API',                              'database',   '{linux}',                   true),
(9300, 'tcp', 'elasticsearch-transport','Elasticsearch transport (cluster)',         'database',   '{linux}',                   false),
(5672, 'tcp', 'amqp',        'RabbitMQ AMQP',                                        'messaging',  '{linux}',                   true),
(15672,'tcp', 'rabbitmq-mgmt','RabbitMQ management console',                        'messaging',  '{linux}',                   false),
(6380, 'tcp', 'redis-tls',   'Redis over TLS',                                       'database',   '{linux}',                   false),
(8529, 'tcp', 'arangodb',    'ArangoDB HTTP API',                                    'database',   '{linux}',                   false),
(7474, 'tcp', 'neo4j-http',  'Neo4j HTTP connector',                                 'database',   '{linux}',                   false),
(7687, 'tcp', 'bolt',        'Neo4j Bolt protocol',                                  'database',   '{linux}',                   false),
(9042, 'tcp', 'cassandra',   'Apache Cassandra native transport',                    'database',   '{linux}',                   true),
(2181, 'tcp', 'zookeeper',   'Apache ZooKeeper client port',                         'messaging',  '{linux}',                   true),
(9092, 'tcp', 'kafka',       'Apache Kafka broker',                                  'messaging',  '{linux}',                   true),

-- Email / messaging
(25,   'tcp', 'smtp',        'Simple Mail Transfer Protocol',                        'email',      '{}',                        true),
(465,  'tcp', 'smtps',       'SMTP over TLS',                                        'email',      '{}',                        true),
(587,  'tcp', 'submission',  'Email message submission (STARTTLS)',                  'email',      '{}',                        true),
(110,  'tcp', 'pop3',        'Post Office Protocol v3',                              'email',      '{}',                        true),
(995,  'tcp', 'pop3s',       'POP3 over TLS',                                        'email',      '{}',                        true),
(143,  'tcp', 'imap',        'Internet Message Access Protocol',                     'email',      '{}',                        true),
(993,  'tcp', 'imaps',       'IMAP over TLS',                                        'email',      '{}',                        true),

-- File transfer / storage
(20,   'tcp', 'ftp-data',    'FTP data transfer',                                    'transfer',   '{}',                        true),
(21,   'tcp', 'ftp',         'File Transfer Protocol control',                       'transfer',   '{}',                        true),
(69,   'tcp', 'tftp',        'Trivial File Transfer Protocol',                       'transfer',   '{network}',                 true),
(111,  'tcp', 'rpcbind',     'ONC RPC portmapper',                                   'linux',      '{linux}',                   true),
(2049, 'tcp', 'nfs',         'Network File System',                                  'linux',      '{linux}',                   true),
(139,  'tcp', 'smb',         'SMB/CIFS file sharing',                                'windows',    '{windows}',                 true),
(548,  'tcp', 'afp',         'Apple Filing Protocol',                                'transfer',   '{macos}',                   true),

-- DNS / infrastructure
(53,   'tcp', 'dns',         'Domain Name System',                                   'network',    '{}',                        true),
(67,   'tcp', 'dhcp-server', 'DHCP server',                                          'network',    '{network}',                 true),
(123,  'tcp', 'ntp',         'Network Time Protocol',                                'network',    '{}',                        true),
(161,  'tcp', 'snmp',        'Simple Network Management Protocol',                   'network',    '{network,linux}',           true),
(162,  'tcp', 'snmp-trap',   'SNMP trap receiver',                                   'network',    '{network,linux}',           true),
(514,  'tcp', 'syslog',      'Unix syslog',                                           'network',    '{linux,network}',           true),
(830,  'tcp', 'netconf-ssh', 'NETCONF over SSH (network device management)',         'network',    '{network}',                 true),

-- Monitoring / observability
(9100, 'tcp', 'node-exporter','Prometheus Node Exporter',                            'monitoring', '{linux}',                   false),
(9104, 'tcp', 'mysql-exporter','Prometheus MySQL Exporter',                          'monitoring', '{linux}',                   false),
(9187, 'tcp', 'postgres-exporter','Prometheus PostgreSQL Exporter',                  'monitoring', '{linux}',                   false),
(3001, 'tcp', 'grafana-alt', 'Grafana alternate port',                               'monitoring', '{linux}',                   false),
(3100, 'tcp', 'loki',        'Grafana Loki log aggregation',                         'monitoring', '{linux}',                   false),
(4317, 'tcp', 'otlp-grpc',   'OpenTelemetry gRPC collector',                         'monitoring', '{linux}',                   false),
(4318, 'tcp', 'otlp-http',   'OpenTelemetry HTTP collector',                         'monitoring', '{linux}',                   false),
(14268,'tcp', 'jaeger-http', 'Jaeger HTTP collector',                                'monitoring', '{linux}',                   false),
(16686,'tcp', 'jaeger-ui',   'Jaeger UI',                                             'monitoring', '{linux}',                   false),

-- Containers / orchestration
(2375, 'tcp', 'docker',      'Docker daemon (unauthenticated — dangerous)',          'container',  '{linux}',                   false),
(2376, 'tcp', 'docker-tls',  'Docker daemon over TLS',                               'container',  '{linux}',                   false),
(6443, 'tcp', 'k8s-api',     'Kubernetes API server',                                'container',  '{linux}',                   false),
(10250,'tcp', 'kubelet',     'Kubernetes Kubelet API',                               'container',  '{linux}',                   false),
(2379, 'tcp', 'etcd-client', 'etcd client port',                                     'container',  '{linux}',                   false),
(2380, 'tcp', 'etcd-peer',   'etcd peer port',                                       'container',  '{linux}',                   false),

-- IoT / industrial
(502,  'tcp', 'modbus',      'Modbus TCP (industrial control systems)',               'iot',        '{iot}',                     true),
(1883, 'tcp', 'mqtt',        'MQTT (IoT messaging)',                                  'iot',        '{iot,linux}',               true),
(8883, 'tcp', 'mqtts',       'MQTT over TLS',                                         'iot',        '{iot,linux}',               true),
(47808,'tcp', 'bacnet',      'BACnet building automation',                            'iot',        '{iot}',                     false),
(4840, 'tcp', 'opc-ua',      'OPC-UA (industrial automation)',                        'iot',        '{iot}',                     false),

-- Security / VPN
(500,  'tcp', 'isakmp',      'ISAKMP / IKE (IPsec key exchange)',                    'security',   '{}',                        true),
(1194, 'tcp', 'openvpn',     'OpenVPN',                                              'security',   '{}',                        true),
(1701, 'tcp', 'l2tp',        'Layer 2 Tunneling Protocol',                           'security',   '{}',                        true),
(1723, 'tcp', 'pptp',        'Point-to-Point Tunneling Protocol',                    'security',   '{}',                        true),
(51820,'tcp', 'wireguard',   'WireGuard VPN',                                        'security',   '{}',                        false),
(4433, 'tcp', 'alt-https',   'HTTPS alternate (strongSwan, etc.)',                   'security',   '{}',                        false),
(8444, 'tcp', 'https-dev',   'HTTPS dev alternate',                                  'security',   '{}',                        false),

-- Miscellaneous / common
(179,  'tcp', 'bgp',         'Border Gateway Protocol',                              'network',    '{network}',                 true),
(443,  'tcp', 'https',       'HTTP Secure',                                          'web',        '{}',                        true),
(1080, 'tcp', 'socks',       'SOCKS proxy',                                          'proxy',      '{}',                        true),
(3128, 'tcp', 'squid',       'Squid HTTP proxy',                                     'proxy',      '{linux}',                   true),
(8118, 'tcp', 'privoxy',     'Privoxy web proxy',                                    'proxy',      '{linux}',                   false),
(6000, 'tcp', 'x11',         'X Window System',                                      'linux',      '{linux}',                   true),
(631,  'tcp', 'ipp',         'Internet Printing Protocol (CUPS)',                    'linux',      '{linux,macos}',             true),
(9000, 'tcp', 'php-fpm',     'PHP-FPM FastCGI process manager',                      'web',        '{linux}',                   false),
(8009, 'tcp', 'ajp',         'Apache JServ Protocol (Tomcat)',                       'web',        '{linux}',                   false),
(11211,'tcp', 'memcached',   'Memcached in-memory cache',                            'database',   '{linux}',                   true)
ON CONFLICT (port, protocol) DO NOTHING;
