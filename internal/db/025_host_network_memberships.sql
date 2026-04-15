-- Migration 025: Host ↔ Network membership view
--
-- Derived many-to-many relationship between hosts and registered networks.
-- A host is a member of every network whose CIDR contains its IP address.
-- Uses the existing GIST index on hosts.ip_address and networks.cidr for
-- fast lookups.  No materialisation; always reflects current data.

CREATE OR REPLACE VIEW host_network_memberships AS
SELECT
    h.id              AS host_id,
    n.id              AS network_id,
    h.ip_address      AS ip_address,
    n.cidr            AS cidr,
    masklen(n.cidr)   AS mask_len
FROM hosts h
JOIN networks n ON h.ip_address <<= n.cidr;

COMMENT ON VIEW host_network_memberships IS
    'Derived host↔network membership via CIDR containment. '
    'A host is a member of every registered network whose CIDR contains its IP. '
    'Use mask_len DESC for longest-prefix ordering.';
