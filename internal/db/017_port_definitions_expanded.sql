-- Migration 017: expand port definitions with UDP ports and additional TCP ports.
-- Adds well-known UDP ports and fills notable TCP gaps missing from migration 012.

INSERT INTO port_definitions (port, protocol, service, description, category, os_families, is_standard) VALUES

-- UDP: DNS / network infrastructure
(53,    'udp', 'dns',           'Domain Name System',                                  'network',    '{}',              true),
(67,    'udp', 'dhcp-server',   'DHCP server',                                         'network',    '{network}',       true),
(68,    'udp', 'dhcp-client',   'DHCP client',                                         'network',    '{network}',       true),
(69,    'udp', 'tftp',          'Trivial File Transfer Protocol',                      'network',    '{network}',       true),
(123,   'udp', 'ntp',           'Network Time Protocol',                               'network',    '{}',              true),
(161,   'udp', 'snmp',          'Simple Network Management Protocol',                  'network',    '{network,linux}', true),
(162,   'udp', 'snmp-trap',     'SNMP trap receiver',                                  'network',    '{network,linux}', true),
(514,   'udp', 'syslog',        'Unix syslog',                                         'network',    '{linux,network}', true),

-- UDP: security / VPN
(500,   'udp', 'ike',           'IKE / ISAKMP (IPsec key exchange)',                   'security',   '{}',              true),
(1194,  'udp', 'openvpn',       'OpenVPN',                                             'security',   '{}',              true),
(4500,  'udp', 'ipsec-nat-t',   'IPsec NAT traversal',                                 'security',   '{}',              true),
(51820, 'udp', 'wireguard',     'WireGuard VPN',                                       'security',   '{}',              false),

-- UDP: discovery / mDNS
(1900,  'udp', 'ssdp',          'SSDP / UPnP discovery',                              'network',    '{iot,windows}',   false),
(5353,  'udp', 'mdns',          'Multicast DNS',                                       'network',    '{}',              false),
(5355,  'udp', 'llmnr',         'Link-Local Multicast Name Resolution',                'network',    '{windows}',       false),

-- TCP gaps: legacy / niche protocols
(79,    'tcp', 'finger',        'Finger user information protocol',                    'network',    '{linux}',         true),
(104,   'tcp', 'dicom',         'DICOM medical imaging',                               'network',    '{}',              true),
(119,   'tcp', 'nntp',          'Network News Transfer Protocol',                      'network',    '{}',              true),
(220,   'tcp', 'imap3',         'IMAP version 3',                                      'email',      '{}',              true),
(512,   'tcp', 'rexec',         'Remote process execution',                            'remote',     '{linux}',         true),
(513,   'tcp', 'rlogin',        'Remote login',                                        'remote',     '{linux}',         true),
(515,   'tcp', 'lpd',           'Line Printer Daemon',                                 'linux',      '{linux}',         true),
(554,   'tcp', 'rtsp',          'Real Time Streaming Protocol',                        'network',    '{iot}',           true),
(853,   'tcp', 'dns-over-tls',  'DNS over TLS',                                        'network',    '{}',              true),
(873,   'tcp', 'rsync',         'rsync file synchronisation',                          'linux',      '{linux}',         true),

-- TCP gaps: alternate service ports
(2222,  'tcp', 'ssh-alt',       'SSH alternate port',                                  'remote',     '{linux,macos}',   false),
(2525,  'tcp', 'smtp-alt',      'SMTP alternate port',                                 'email',      '{}',              false),

-- TCP gaps: databases / storage
(3050,  'tcp', 'firebird',      'Firebird database',                                   'database',   '{}',              false),
(3260,  'tcp', 'iscsi',         'iSCSI target',                                        'linux',      '{linux}',         true),

-- TCP gaps: real-time / comms
(3478,  'tcp', 'stun',          'STUN / TURN (WebRTC NAT traversal)',                  'network',    '{}',              true),
(4369,  'tcp', 'epmd',          'Erlang Port Mapper Daemon',                           'messaging',  '{linux}',         true),
(5060,  'tcp', 'sip',           'Session Initiation Protocol',                         'network',    '{}',              true),
(5061,  'tcp', 'sip-tls',       'SIP over TLS',                                        'network',    '{}',              true),

-- TCP gaps: XMPP
(5222,  'tcp', 'xmpp-client',   'XMPP client connections',                            'messaging',  '{}',              true),
(5269,  'tcp', 'xmpp-server',   'XMPP server federation',                             'messaging',  '{}',              true),
(5280,  'tcp', 'xmpp-bosh',     'XMPP BOSH (HTTP tunnelling)',                         'messaging',  '{}',              false),

-- TCP gaps: observability / monitoring
(5044,  'tcp', 'logstash-beats','Logstash Beats input',                               'monitoring', '{linux}',         false),
(5601,  'tcp', 'kibana',        'Kibana web UI',                                       'monitoring', '{linux}',         false),
(8086,  'tcp', 'influxdb',      'InfluxDB HTTP API',                                   'monitoring', '{linux}',         false),
(9093,  'tcp', 'alertmanager',  'Prometheus Alertmanager',                             'monitoring', '{linux}',         false),
(9411,  'tcp', 'zipkin',        'Zipkin distributed tracing',                          'monitoring', '{linux}',         false),

-- TCP gaps: messaging / queue
(5671,  'tcp', 'amqps',         'AMQP over TLS (RabbitMQ)',                            'messaging',  '{linux}',         true),
(6514,  'tcp', 'syslog-tls',    'Syslog over TLS',                                     'network',    '{linux,network}', true),
(61616, 'tcp', 'activemq',      'Apache ActiveMQ broker',                              'messaging',  '{linux}',         false),

-- TCP gaps: infrastructure / ops
(8000,  'tcp', 'http-dev2',     'HTTP dev server (Django, etc.)',                      'web',        '{}',              false),
(8081,  'tcp', 'http-alt2',     'HTTP alternate',                                      'web',        '{}',              false),
(8200,  'tcp', 'vault',         'HashiCorp Vault HTTP API',                            'security',   '{linux}',         false),
(8301,  'tcp', 'consul-lan',    'Consul Serf LAN',                                     'network',    '{linux}',         false),
(8302,  'tcp', 'consul-wan',    'Consul Serf WAN',                                     'network',    '{linux}',         false),
(8500,  'tcp', 'consul-http',   'Consul HTTP API',                                     'network',    '{linux}',         false),
(10000, 'tcp', 'webmin',        'Webmin administration panel',                         'remote',     '{linux}',         false),
(11434, 'tcp', 'ollama',        'Ollama LLM inference server',                         'web',        '{linux,macos}',   false)

ON CONFLICT DO NOTHING;
