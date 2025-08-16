# Networks Command

The `networks` command provides comprehensive management of network discovery targets in Scanorama. Networks define CIDR ranges that can be automatically discovered and scanned, allowing you to organize and manage your network infrastructure effectively.

## Overview

Networks in Scanorama serve as discovery targets that define which IP ranges should be monitored. Each network has:

- **Name**: Human-readable identifier
- **CIDR**: Network range in CIDR notation (e.g., 192.168.1.0/24)
- **Discovery Method**: How hosts are discovered (tcp, ping, arp, icmp)
- **Status**: Active/inactive for discovery operations
- **Scan Configuration**: Whether detailed scanning is enabled
- **Statistics**: Host counts and last activity timestamps

## Available Commands

### `networks list`

List all configured networks with their status and statistics.

```bash
# List active networks
scanorama networks list

# Include inactive networks
scanorama networks list --show-inactive
```

**Output includes:**
- Network ID (UUID for unique identification)
- Network name and CIDR range
- Discovery method
- Active/scan status
- Host counts (total and active)
- Description

### `networks add`

Add a new network discovery target.

```bash
# Basic network addition
scanorama networks add --name "corp-lan" --cidr 192.168.1.0/24

# Advanced configuration
scanorama networks add \
  --name "dmz-servers" \
  --cidr 10.0.1.0/24 \
  --method ping \
  --description "DMZ server network" \
  --active=true \
  --scan=true
```

**Required Flags:**
- `--name`: Network name (must be unique)
- `--cidr`: Network CIDR range

**Optional Flags:**
- `--method`: Discovery method (tcp, ping, arp, icmp) - defaults to "ping"
- `--description`: Human-readable description
- `--active`: Enable for discovery operations (default: true)
- `--scan`: Enable for detailed scanning (default: true)

### `networks remove`

Remove a network discovery target by name.

```bash
# Remove by name
scanorama networks remove corp-lan

# Remove network with descriptive name
scanorama networks remove dmz-servers
```

**Note:** Removing a network deletes the network configuration and discovery history but preserves any hosts that were discovered from that network.

### `networks show`

Display detailed information about a specific network.

```bash
# Show network details
scanorama networks show corp-lan
```

**Output includes:**
- Network ID and complete configuration
- Discovery and scan statistics
- Network addressing information (network, broadcast, usable addresses)
- Activity timestamps
- Host counts and status

### `networks enable`

Enable a network for discovery and scanning operations.

```bash
scanorama networks enable corp-lan
```

Sets both `is_active` and `scan_enabled` to true.

### `networks disable`

Disable a network from discovery and scanning operations.

```bash
scanorama networks disable corp-lan
```

Sets both `is_active` and `scan_enabled` to false.

### `networks rename`

Rename an existing network discovery target.

```bash
# Rename a network
scanorama networks rename corp-lan corporate-network

# Rename network with spaces in name
scanorama networks rename "old name" "new name"
```

**Required Arguments:**
- `current-name`: Current name of the network
- `new-name`: New name for the network (must be unique)

**Note:** Renaming preserves all network configuration, exclusions, discovery history, and statistics. Only the name changes.

### `networks exclusions`

Manage IP addresses and CIDR ranges that should be excluded from discovery and scanning operations.

#### `networks exclusions list`

List all configured network exclusions.

```bash
# List all exclusions
scanorama networks exclusions list

# List exclusions for specific network
scanorama networks exclusions list --network corp-lan

# List only global exclusions
scanorama networks exclusions list --global
```

#### `networks exclusions add`

Add an IP address or CIDR range to exclude from discovery and scanning.

```bash
# Add global exclusion (applies to all networks)
scanorama networks exclusions add --cidr 192.168.1.1/32 --reason "Router" --global

# Add network-specific exclusion
scanorama networks exclusions add --network corp-lan --cidr 192.168.1.0/29 --reason "Management subnet"

# Add single IP exclusion (automatically converts to /32)
scanorama networks exclusions add --cidr 10.0.0.1 --reason "Critical server"
```

**Required Flags:**
- `--cidr`: IP address or CIDR range to exclude

**Optional Flags:**
- `--network`: Network name for network-specific exclusion
- `--global`: Create global exclusion (applies to all networks)
- `--reason`: Human-readable reason for exclusion

**Note:** You cannot specify both `--network` and `--global` flags simultaneously.

#### `networks exclusions remove`

Remove a network exclusion by its ID.

```bash
# Remove exclusion by ID (get ID from 'exclusions list')
scanorama networks exclusions remove f47ac10b-58cc-4372-a567-0e02b2c3d479
```

## Discovery Methods

Each network can use different discovery methods:

- **ping**: ICMP echo ping (default method)
- **tcp**: TCP SYN ping to common ports (22, 80, 443, 8080, 8022, 8379)
- **arp**: ARP ping (effective for local networks)
- **icmp**: ICMP ping (alias for ping)

## Example Workflows

### Setting Up Network Monitoring

```bash
# 1. Add your primary networks
scanorama networks add --name "corp-lan" --cidr 192.168.0.0/16
scanorama networks add --name "guest-wifi" --cidr 10.0.100.0/24
scanorama networks add --name "server-dmz" --cidr 172.16.1.0/24 --method tcp

# 2. Rename if needed (preserves all configuration and history)
scanorama networks rename guest-wifi guest-network

# 3. Add exclusions for critical infrastructure
scanorama networks exclusions add --cidr 192.168.1.1/32 --reason "Primary router" --global
scanorama networks exclusions add --network corp-lan --cidr 192.168.1.0/29 --reason "Management subnet"

# 4. View configured networks and exclusions (includes network IDs)
scanorama networks list
scanorama networks exclusions list

# 5. Verify network configuration (shows network ID)
scanorama networks show corp-lan

# 6. Discover hosts on configured networks (automatically applies exclusions)
scanorama discover --configured-networks

# 7. Or discover specific network
scanorama discover --network corp-lan

# 8. Alternative: Discover and add new networks in one step
scanorama discover 172.16.0.0/24 --add --name "lab-network"

# 9. View discovered hosts
scanorama hosts
```

### Managing Network Status

```bash
# Temporarily disable a network
scanorama networks disable guest-network

# View status changes (shows network IDs)
scanorama networks list --show-inactive

# Re-enable when needed
scanorama networks enable guest-network
```

### Network Maintenance

```bash
# Rename networks (preserves configuration)
scanorama networks rename old-lab-net lab-network-v2

# Update network method (requires remove/add)
scanorama networks remove lab-network-old
scanorama networks add --name "lab-net-v2" --cidr 192.168.1.0/24 --method arp

# Clean up unused networks
scanorama networks remove decommissioned-net
```

## Shell Completion

The networks command supports intelligent shell completion:

```bash
# Enable completion for your shell
source <(scanorama completion bash)  # bash
source <(scanorama completion zsh)   # zsh

# Completion features:
scanorama networks <TAB>                    # Shows: add, disable, enable, exclusions, list, remove, rename, show
scanorama networks remove <TAB>             # Shows: configured network names
scanorama networks rename <TAB>             # Shows: configured network names
scanorama networks add --method <TAB>       # Shows: tcp, ping, arp, icmp
scanorama networks exclusions <TAB>         # Shows: add, list, remove
scanorama networks exclusions add --network <TAB>  # Shows: configured network names
```

## Integration with Other Commands

Networks integrate seamlessly with other Scanorama commands:

### Discovery Integration
```bash
# Discover all active configured networks (recommended)
scanorama discover --configured-networks

# Discover specific configured network by name
scanorama discover --network corp-lan

# Discover and add network to database in one step
scanorama discover 192.168.1.0/24 --add --name "corp-lan"
scanorama discover 10.0.0.0/16 --add  # Uses CIDR as name

# Discover all local network interfaces
scanorama discover --all-networks

# Discover specific CIDR (traditional method)
scanorama discover 192.168.1.0/24
```

### Host Management
```bash
# View hosts from all networks
scanorama hosts

# View hosts from specific network (using network CIDR as filter)
scanorama hosts | grep "192.168.1"
```

### Scanning Integration
```bash
# Scan discovered hosts from configured networks
scanorama scan --live-hosts

# Scan specific network range
scanorama scan --targets 192.168.1.0/24
```

### Discovery Workflow Integration
```bash
# Complete workflow using configured networks
scanorama networks add --name "production" --cidr 10.0.0.0/16
scanorama networks exclusions add --network production --cidr 10.0.0.1/32 --reason "Gateway"
scanorama discover --network production
scanorama hosts
scanorama scan --live-hosts

# Streamlined workflow with --add (discover and configure in one step)
scanorama discover 10.0.0.0/16 --add --name "production"
scanorama networks exclusions add --network production --cidr 10.0.0.1/32 --reason "Gateway"
scanorama discover --network production  # Uses configured exclusions
scanorama hosts
scanorama scan --live-hosts
```

## Best Practices

1. **Naming Convention**: Use descriptive names that indicate location or purpose
   ```bash
   scanorama networks add --name "hq-floor1-workstations" --cidr 192.168.10.0/24
   scanorama networks add --name "branch-office-servers" --cidr 10.0.20.0/24
   
   # Rename if naming convention changes
   scanorama networks rename hq-floor1-workstations hq-f1-workstations
   ```

2. **Method Selection**: Choose appropriate discovery methods
   - Use `ping` for general networks (default - works well for most scenarios)
   - Use `tcp` for networks that block ICMP but allow TCP connections
   - Use `arp` for local network segments where you have broadcast access

3. **Network Segmentation**: Match your network configuration to actual infrastructure
   ```bash
   # Separate by function
   scanorama networks add --name "prod-web-tier" --cidr 10.0.1.0/24
   scanorama networks add --name "prod-db-tier" --cidr 10.0.2.0/24
   scanorama networks add --name "dev-environment" --cidr 10.0.100.0/24
   ```

4. **Status Management**: Use enable/disable for temporary changes
   ```bash
   # Disable scanning during maintenance windows
   scanorama networks disable prod-web-tier
   # ... perform maintenance ...
   scanorama networks enable prod-web-tier
   ```

6. **Network Management**: Use IDs for scripting and rename for organization
   ```bash
   # View network IDs for scripting/automation
   scanorama networks list
   
   # Rename networks for better organization
   scanorama networks rename temp-network production-web-tier
   ```

7. **Exclusions Management**: Use exclusions to protect critical infrastructure
   ```bash
   # Global exclusions for infrastructure that should never be scanned
   scanorama networks exclusions add --cidr 192.168.1.1/32 --reason "Primary router" --global
   scanorama networks exclusions add --cidr 10.0.0.0/24 --reason "Management network" --global
   
   # Network-specific exclusions
   scanorama networks exclusions add --network prod-web-tier --cidr 10.0.1.100/32 --reason "Load balancer"
   ```

## Troubleshooting

### Common Issues

**Network not found errors:**
```bash
# Check exact network name
scanorama networks list

# Check exact network name (use kebab-case for easier CLI usage)
scanorama networks show my-network-name
```

**CIDR validation errors:**
```bash
# Ensure proper CIDR format
scanorama networks add --name "Test" --cidr 192.168.1.0/24  # ✓ Correct
scanorama networks add --name "Test" --cidr 192.168.1.1/24  # ✗ Host IP, not network
```

**Discovery method issues:**
```bash
# Valid methods only
scanorama networks add --name "Test" --cidr 192.168.1.0/24 --method tcp   # ✓
scanorama networks add --name "Test" --cidr 192.168.1.0/24 --method udp   # ✗ Invalid
```

### Verbose Output

Use the global `--verbose` flag for detailed operation information:

```bash
scanorama --verbose networks add --name "debug-net" --cidr 192.168.1.0/24
```

## Database Schema

Networks are stored in the `networks` table with automatic host count maintenance:

- Host counts are automatically updated when hosts are discovered
- Network relationships are maintained through IP address containment
- Discovery history is tracked with timestamps
- Network exclusions are stored in the `network_exclusions` table
- Exclusions are automatically applied during host generation

For direct database access, query the `networks` and `network_exclusions` tables:

```sql
-- View all networks
SELECT name, cidr, discovery_method, is_active, host_count FROM networks;

-- View networks with recent activity (includes IDs)
SELECT id, name, cidr, last_discovery, host_count 
FROM networks 
WHERE last_discovery > NOW() - INTERVAL '24 hours';

-- View all exclusions with network context
SELECT ne.excluded_cidr, ne.reason, n.name as network_name,
       CASE WHEN ne.network_id IS NULL THEN 'Global' ELSE 'Network-specific' END as scope
FROM network_exclusions ne
LEFT JOIN networks n ON ne.network_id = n.id
WHERE ne.enabled = true
ORDER BY ne.created_at DESC;

-- Check if an IP would be excluded
SELECT is_ip_excluded('192.168.1.1'::inet) as is_excluded;
```
