# API and CLI Functionality Parity

This document outlines the complete functionality parity between Scanorama's CLI commands and REST API endpoints, ensuring consistent capabilities across both interfaces.

## Overview

The Scanorama API provides full programmatic access to all network management features available through the CLI. This ensures that automated tools, web interfaces, and scripts can perform the same operations as interactive command-line users.

## Networks Management

### Basic Network Operations

| CLI Command | API Endpoint | Method | Description |
|-------------|--------------|---------|-------------|
| `scanorama networks list` | `/api/v1/networks` | GET | List all configured networks |
| `scanorama networks add` | `/api/v1/networks` | POST | Create a new network |
| `scanorama networks show <name>` | `/api/v1/networks/{id}` | GET | Get network details |
| `scanorama networks remove <name>` | `/api/v1/networks/{id}` | DELETE | Delete a network |
| `scanorama networks enable <name>` | `/api/v1/networks/{id}/enable` | POST | Enable network for discovery/scanning |
| `scanorama networks disable <name>` | `/api/v1/networks/{id}/disable` | POST | Disable network operations |
| `scanorama networks rename <old> <new>` | `/api/v1/networks/{id}/rename` | PUT | Rename a network |

### Network Exclusions Management

| CLI Command | API Endpoint | Method | Description |
|-------------|--------------|---------|-------------|
| `scanorama networks exclusions list` | `/api/v1/exclusions` | GET | List global exclusions |
| `scanorama networks exclusions list --network <name>` | `/api/v1/networks/{id}/exclusions` | GET | List network-specific exclusions |
| `scanorama networks exclusions add --cidr <cidr> --reason <reason>` | `/api/v1/exclusions` | POST | Add global exclusion |
| `scanorama networks exclusions add --network <name> --cidr <cidr>` | `/api/v1/networks/{id}/exclusions` | POST | Add network exclusion |
| `scanorama networks exclusions remove <id>` | `/api/v1/exclusions/{id}` | DELETE | Remove exclusion |

### Statistics and Monitoring

| CLI Command | API Endpoint | Method | Description |
|-------------|--------------|---------|-------------|
| `scanorama networks list` (shows stats) | `/api/v1/networks/stats` | GET | Get network statistics |

## Usage Examples

### CLI to API Translation Examples

#### List Networks
```bash
# CLI
scanorama networks list --show-inactive

# API Equivalent
curl -X GET "http://localhost:8080/api/v1/networks?show_inactive=true"
```

#### Create Network
```bash
# CLI
scanorama networks add --name "corp-lan" --cidr "192.168.1.0/24" --method ping --description "Corporate LAN"

# API Equivalent
curl -X POST http://localhost:8080/api/v1/networks \
  -H "Content-Type: application/json" \
  -d '{
    "name": "corp-lan",
    "cidr": "192.168.1.0/24",
    "discovery_method": "ping",
    "description": "Corporate LAN",
    "is_active": true,
    "scan_enabled": true
  }'
```

#### Enable/Disable Network
```bash
# CLI
scanorama networks enable corp-lan
scanorama networks disable corp-lan

# API Equivalent
curl -X POST http://localhost:8080/api/v1/networks/{network-id}/enable
curl -X POST http://localhost:8080/api/v1/networks/{network-id}/disable
```

#### Add Exclusions
```bash
# CLI - Global exclusion
scanorama networks exclusions add --cidr "192.168.1.1/32" --reason "Router"

# API Equivalent
curl -X POST http://localhost:8080/api/v1/exclusions \
  -H "Content-Type: application/json" \
  -d '{
    "excluded_cidr": "192.168.1.1/32",
    "reason": "Router"
  }'

# CLI - Network-specific exclusion
scanorama networks exclusions add --network corp-lan --cidr "192.168.1.0/29" --reason "Management subnet"

# API Equivalent
curl -X POST http://localhost:8080/api/v1/networks/{network-id}/exclusions \
  -H "Content-Type: application/json" \
  -d '{
    "excluded_cidr": "192.168.1.0/29",
    "reason": "Management subnet"
  }'
```

#### Update Network
```bash
# CLI - Update through individual commands (rename, enable/disable)
scanorama networks rename old-name new-name
scanorama networks enable new-name

# API Equivalent - Single update call
curl -X PUT http://localhost:8080/api/v1/networks/{network-id} \
  -H "Content-Type: application/json" \
  -d '{
    "name": "new-name",
    "is_active": true,
    "scan_enabled": true
  }'
```

## API Response Formats

### Network Object
```json
{
  "id": "550e8400-e29b-41d4-a716-446655440000",
  "name": "corp-lan",
  "cidr": "192.168.1.0/24",
  "description": "Corporate LAN",
  "discovery_method": "ping",
  "is_active": true,
  "scan_enabled": true,
  "last_discovery": "2024-01-15T10:30:00Z",
  "last_scan": "2024-01-15T09:00:00Z",
  "host_count": 45,
  "active_host_count": 32,
  "created_at": "2024-01-01T00:00:00Z",
  "updated_at": "2024-01-15T10:30:00Z"
}
```

### Paginated List Response
```json
{
  "data": [
    {/* network objects */}
  ],
  "pagination": {
    "page": 1,
    "page_size": 10,
    "total_items": 25,
    "total_pages": 3
  }
}
```

### Network Statistics
```json
{
  "networks": {
    "total": 5,
    "active": 4,
    "scan_enabled": 3
  },
  "hosts": {
    "total": 150,
    "active": 120
  },
  "exclusions": {
    "total": 8,
    "global": 3,
    "network": 5
  }
}
```

### Exclusion Object
```json
{
  "id": "550e8400-e29b-41d4-a716-446655440001",
  "network_id": "550e8400-e29b-41d4-a716-446655440000",
  "excluded_cidr": "192.168.1.1/32",
  "reason": "Router",
  "enabled": true,
  "created_at": "2024-01-01T00:00:00Z",
  "updated_at": "2024-01-01T00:00:00Z"
}
```

## Query Parameters

### List Networks (`GET /api/v1/networks`)
- `page` (int): Page number for pagination (default: 1)
- `page_size` (int): Items per page (default: 10, max: 100)
- `show_inactive` (bool): Include inactive networks (default: false)
- `name` (string): Filter by network name (partial match)

## Error Handling

Both CLI and API use consistent error codes and messages:

- **400 Bad Request**: Invalid input parameters
- **404 Not Found**: Network or exclusion not found
- **409 Conflict**: Network name already exists
- **500 Internal Server Error**: Database or service errors

## Differences and Considerations

### ID vs Name References
- **CLI**: Uses human-readable network names for most operations
- **API**: Uses UUID-based IDs for network references
- **Solution**: API provides name-based lookup through list operations with filtering

### Batch Operations
- **CLI**: Individual commands for each operation
- **API**: Supports bulk operations and more efficient single calls for complex updates

### Output Formatting
- **CLI**: Human-readable tables and formatted output
- **API**: Structured JSON responses suitable for programmatic consumption

### Authentication
- **CLI**: Uses local database access (when running on same machine)
- **API**: Supports API key authentication and proper access controls

## Migration Guide

### From CLI Scripts to API Calls

1. **Replace network names with IDs**: Use `GET /api/v1/networks?name=<network-name>` to find network ID
2. **Convert command flags to JSON**: Map CLI flags to JSON request body fields
3. **Handle pagination**: Use page parameters for large result sets
4. **Error handling**: Parse JSON error responses instead of CLI exit codes

### Example Migration

```bash
# Old CLI script
#!/bin/bash
scanorama networks add --name "test-net" --cidr "10.0.0.0/24" --method ping
scanorama networks enable "test-net"
scanorama networks exclusions add --network "test-net" --cidr "10.0.0.1/32" --reason "Gateway"

# New API script
#!/bin/bash
# Create network
NETWORK_ID=$(curl -s -X POST http://localhost:8080/api/v1/networks \
  -H "Content-Type: application/json" \
  -d '{"name":"test-net","cidr":"10.0.0.0/24","discovery_method":"ping"}' | \
  jq -r '.id')

# Enable network
curl -X POST "http://localhost:8080/api/v1/networks/$NETWORK_ID/enable"

# Add exclusion
curl -X POST "http://localhost:8080/api/v1/networks/$NETWORK_ID/exclusions" \
  -H "Content-Type: application/json" \
  -d '{"excluded_cidr":"10.0.0.1/32","reason":"Gateway"}'
```

## Conclusion

The API provides complete functionality parity with the CLI, enabling:
- **Automated network management** through scripts and tools
- **Web-based interfaces** for network configuration
- **Integration with external systems** for enterprise environments
- **Programmatic discovery workflows** and scheduling

All network management operations available through the CLI can be performed via the REST API, maintaining consistent behavior and validation rules across both interfaces.