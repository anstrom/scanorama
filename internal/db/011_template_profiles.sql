-- Migration 011: seed built-in smart profile templates
-- Six read-only template profiles seeded with OS-appropriate port lists.
-- Each profile has built_in=TRUE so the repository blocks edits and deletes.

INSERT INTO scan_profiles (id, name, description, ports, scan_type, timing, built_in)
VALUES
    (
        'template-linux-standard',
        'Linux Standard',
        'Standard port scan for Linux/Unix hosts. Covers common services: SSH, web, NFS, databases.',
        '22,80,443,111,2049,3306,5432,8080,8443',
        'connect',
        'normal',
        TRUE
    ),
    (
        'template-windows-standard',
        'Windows Standard',
        'Standard port scan for Windows hosts. Covers SMB, RDP, WinRM, web.',
        '80,135,139,443,445,3389,5985,5986',
        'connect',
        'normal',
        TRUE
    ),
    (
        'template-network-device',
        'Network Device',
        'Port scan tuned for switches, routers, and managed appliances. Includes SNMP and Netconf.',
        '22,23,80,161,443,830',
        'connect',
        'polite',
        TRUE
    ),
    (
        'template-web-server',
        'Web Server',
        'Port scan for web-facing hosts. Covers HTTP, HTTPS, and common alternative ports.',
        '80,443,3000,8080,8443,8888',
        'connect',
        'normal',
        TRUE
    ),
    (
        'template-database-server',
        'Database Server',
        'Port scan for database hosts. Covers MSSQL, Oracle, MySQL, PostgreSQL, Redis, MongoDB, Elasticsearch.',
        '1433,1521,3306,5432,6379,9200,27017',
        'connect',
        'normal',
        TRUE
    ),
    (
        'template-iot-embedded',
        'IoT / Embedded',
        'Port scan for IoT devices, embedded systems, and industrial equipment. Covers Telnet, Modbus, MQTT.',
        '22,23,80,443,502,1883,8883',
        'connect',
        'polite',
        TRUE
    )
ON CONFLICT (id) DO NOTHING;
