# Scanorama API Documentation

## Overview

The Scanorama API provides a RESTful interface for network scanning, host discovery, and scan management. This documentation is designed for frontend developers building user interfaces for the Scanorama platform.

## Base Information

- **Base URL**: `http://localhost:8080/api/v1`
- **Content-Type**: `application/json`
- **Authentication**: Currently not implemented (development mode)
- **API Version**: v1

## Health & Status Endpoints

### Health Check
```http
GET /api/v1/health
```

**Response (200 OK):**
```json
{
  "status": "healthy",
  "database": "connected",
  "timestamp": "2025-01-14T00:20:30Z"
}
```

### Liveness Check
```http
GET /api/v1/liveness
```

### Status Information
```http
GET /api/v1/status
```

### Version Information
```http
GET /api/v1/version
```

## Common Response Format

### Success Response
All list endpoints return paginated responses:
```json
{
  "data": [...],
  "pagination": {
    "page": 1,
    "page_size": 20,
    "total_items": 45,
    "total_pages": 3
  }
}
```

### Error Response
```json
{
  "error": "Error description",
  "timestamp": "2025-01-14T00:20:30Z",
  "request_id": "uuid-request-id"
}
```

## Pagination Parameters

All list endpoints support pagination:
- `page`: Page number (default: 1)
- `page_size`: Items per page (default: 20, max: 100)

Example: `GET /api/v1/hosts?page=2&page_size=50`

## Scan Management

### List Scans
```http
GET /api/v1/scans
```

**Query Parameters:**
- `status`: Filter by status (`pending`, `running`, `completed`, `failed`)
- `scan_type`: Filter by scan type (`connect`, `syn`, `version`, etc.)
- `created_after`: Filter by creation date (RFC3339 format)
- `created_before`: Filter by creation date (RFC3339 format)

**Response:**
```json
{
  "data": [
    {
      "id": "550e8400-e29b-41d4-a716-446655440000",
      "name": "Weekly Network Scan",
      "description": "Comprehensive weekly scan of internal network",
      "targets": ["192.168.1.0/24", "10.0.0.0/16"],
      "scan_type": "connect",
      "ports": "22,80,443,8080,8443",
      "profile_id": 1,
      "status": "completed",
      "progress": 100.0,
      "start_time": "2025-01-14T00:15:00Z",
      "end_time": "2025-01-14T00:18:30Z",
      "duration": "3m30s",
      "created_at": "2025-01-14T00:14:45Z",
      "updated_at": "2025-01-14T00:18:30Z",
      "created_by": "admin"
    }
  ],
  "pagination": {
    "page": 1,
    "page_size": 20,
    "total_items": 1,
    "total_pages": 1
  }
}
```

### Create Scan
```http
POST /api/v1/scans
```

**Request Body:**
```json
{
  "name": "My Network Scan",
  "description": "Scan of production network",
  "targets": ["192.168.1.0/24"],
  "scan_type": "connect",
  "ports": "22,80,443",
  "profile_id": 1,
  "options": {
    "timing": "normal",
    "max_retries": 3
  },
  "tags": ["production", "weekly"]
}
```

**Response (201 Created):**
```json
{
  "id": "550e8400-e29b-41d4-a716-446655440000",
  "name": "My Network Scan",
  "status": "pending",
  "created_at": "2025-01-14T00:20:30Z",
  ...
}
```

### Get Scan
```http
GET /api/v1/scans/{id}
```

### Update Scan
```http
PUT /api/v1/scans/{id}
```

### Delete Scan
```http
DELETE /api/v1/scans/{id}
```

### Start Scan
```http
POST /api/v1/scans/{id}/start
```

**Response (200 OK):**
```json
{
  "id": "550e8400-e29b-41d4-a716-446655440000",
  "status": "running",
  "started_at": "2025-01-14T00:20:30Z"
}
```

### Stop Scan
```http
POST /api/v1/scans/{id}/stop
```

### Get Scan Results
```http
GET /api/v1/scans/{id}/results
```

**Response:**
```json
{
  "scan_id": "550e8400-e29b-41d4-a716-446655440000",
  "total_hosts": 15,
  "total_ports": 150,
  "open_ports": 45,
  "closed_ports": 105,
  "results": [
    {
      "id": 1,
      "host_ip": "192.168.1.100",
      "hostname": "web-server-01",
      "port": 80,
      "protocol": "tcp",
      "state": "open",
      "service": "http",
      "version": "nginx/1.20.1",
      "banner": "nginx/1.20.1",
      "scan_time": "2025-01-14T00:16:15Z"
    }
  ],
  "summary": {
    "scan_duration": "3m30s",
    "hosts_up": 12,
    "hosts_down": 3,
    "services_found": 23
  },
  "generated_at": "2025-01-14T00:20:30Z"
}
```

## Host Management

### List Hosts
```http
GET /api/v1/hosts
```

**Query Parameters:**
- `os`: Filter by OS family (`linux`, `windows`, `macos`, etc.)
- `status`: Filter by status (`up`, `down`, `unknown`)
- `network`: Filter by network CIDR

**Response:**
```json
{
  "data": [
    {
      "id": 1001,
      "ip": "192.168.1.100",
      "hostname": "web-server-01",
      "description": "Main web server",
      "os": "Linux",
      "os_version": "Ubuntu 22.04.3 LTS",
      "tags": ["server", "web", "production"],
      "metadata": {
        "location": "datacenter-a",
        "owner": "web-team"
      },
      "active": true,
      "last_seen": "2025-01-14T00:18:30Z",
      "last_scan_id": 123,
      "scan_count": 45,
      "open_ports": 3,
      "total_ports": 65535,
      "created_at": "2025-01-10T14:30:00Z",
      "updated_at": "2025-01-14T00:18:30Z",
      "discovered_by": "network-scan"
    }
  ],
  "pagination": {...}
}
```

### Create Host
```http
POST /api/v1/hosts
```

**Request Body:**
```json
{
  "ip": "192.168.1.200",
  "hostname": "new-server",
  "description": "Newly discovered server",
  "os": "Linux",
  "os_version": "CentOS 8",
  "tags": ["server", "new"],
  "metadata": {
    "discovered_method": "manual"
  },
  "active": true
}
```

### Get Host
```http
GET /api/v1/hosts/{id}
```

### Update Host
```http
PUT /api/v1/hosts/{id}
```

### Delete Host
```http
DELETE /api/v1/hosts/{id}
```

### Get Host Scans
```http
GET /api/v1/hosts/{id}/scans
```

**Response:**
```json
{
  "data": [
    {
      "id": 123,
      "name": "Weekly Network Scan",
      "scan_type": "connect",
      "status": "completed",
      "progress": 100.0,
      "start_time": "2025-01-14T00:15:00Z",
      "end_time": "2025-01-14T00:18:30Z",
      "duration": "3m30s",
      "created_at": "2025-01-14T00:14:45Z"
    }
  ],
  "pagination": {...}
}
```

## Profile Management

### List Profiles
```http
GET /api/v1/profiles
```

**Query Parameters:**
- `scan_type`: Filter by scan type

**Response:**
```json
{
  "data": [
    {
      "id": 1,
      "name": "Quick Connect Scan",
      "description": "Fast TCP connect scan",
      "scan_type": "connect",
      "ports": "22,80,443,8080,8443",
      "options": {
        "timing": "normal",
        "max_retries": 3
      },
      "timing": {
        "template": "normal",
        "host_timeout": "30s",
        "scan_delay": "0s"
      },
      "service_detection": true,
      "os_detection": false,
      "script_scan": false,
      "udp_scan": false,
      "max_retries": 3,
      "host_timeout": "30s",
      "max_rate_pps": 1000,
      "max_host_group_size": 64,
      "min_host_group_size": 1,
      "tags": ["quick", "tcp"],
      "default": false,
      "usage_count": 15,
      "last_used": "2025-01-14T00:15:00Z",
      "created_at": "2025-01-10T14:30:00Z",
      "updated_at": "2025-01-14T00:15:00Z",
      "created_by": "admin"
    }
  ],
  "pagination": {...}
}
```

### Create Profile
```http
POST /api/v1/profiles
```

**Request Body:**
```json
{
  "name": "Custom Web Scan",
  "description": "Specialized scan for web services",
  "scan_type": "connect",
  "ports": "80,443,8080,8443,9000",
  "options": {
    "timing": "polite",
    "max_retries": 2
  },
  "timing": {
    "template": "polite",
    "host_timeout": "45s",
    "scan_delay": "100ms"
  },
  "service_detection": true,
  "os_detection": false,
  "script_scan": false,
  "udp_scan": false,
  "max_retries": 2,
  "host_timeout": "45s",
  "max_rate_pps": 500,
  "tags": ["web", "custom"],
  "default": false
}
```

### Get Profile
```http
GET /api/v1/profiles/{id}
```

### Update Profile
```http
PUT /api/v1/profiles/{id}
```

### Delete Profile
```http
DELETE /api/v1/profiles/{id}
```

## Discovery Management

### List Discovery Jobs
```http
GET /api/v1/discovery
```

### Create Discovery Job
```http
POST /api/v1/discovery
```

**Request Body:**
```json
{
  "network": "192.168.1.0/24",
  "method": "tcp",
  "name": "Office Network Discovery"
}
```

### Start Discovery
```http
POST /api/v1/discovery/{id}/start
```

### Stop Discovery
```http
POST /api/v1/discovery/{id}/stop
```

## Schedule Management

### List Schedules
```http
GET /api/v1/schedules
```

### Create Schedule
```http
POST /api/v1/schedules
```

**Request Body:**
```json
{
  "name": "Daily Security Scan",
  "cron_expression": "0 2 * * *",
  "scan_config": {
    "targets": ["192.168.1.0/24"],
    "scan_type": "connect",
    "profile_id": 1
  },
  "enabled": true
}
```

## API Key Management

### Overview

The Scanorama API uses API key authentication for secure access. API keys support full Unicode characters in names, allowing for international and multilingual key identification.

**Authentication:**
- Header: `Authorization: Bearer {api_key}`
- Format: `sk_xxxxxxxxx` (secret key prefix + base32 encoded random data)
- Unicode Support: ✅ Full Unicode support in API key names

### List API Keys
```http
GET /api/v1/auth/keys
```

**Query Parameters:**
- `active`: Filter by active status (`true`, `false`)
- `created_after`: Filter by creation date (RFC3339 format)
- `expires_before`: Filter by expiration date (RFC3339 format)

**Response:**
```json
{
  "data": [
    {
      "id": "550e8400-e29b-41d4-a716-446655440000",
      "name": "Production API Key 🔑",
      "key_prefix": "sk_abc123...",
      "created_at": "2025-01-14T00:00:00Z",
      "updated_at": "2025-01-14T00:00:00Z",
      "last_used_at": "2025-01-14T08:30:00Z",
      "expires_at": "2025-12-31T23:59:59Z",
      "is_active": true,
      "usage_count": 1250,
      "notes": "Main production API access"
    }
  ],
  "pagination": {...}
}
```

### Create API Key
```http
POST /api/v1/auth/keys
```

**Request Body:**
```json
{
  "name": "测试密钥 🔑",
  "expires_at": "2025-12-31T23:59:59Z",
  "notes": "Unicode test key with emoji and Chinese characters",
  "permissions": ["read", "write"]
}
```

**Response (201 Created):**
```json
{
  "id": "550e8400-e29b-41d4-a716-446655440000",
  "name": "测试密钥 🔑",
  "key": "sk_abcdef1234567890abcdef1234567890",
  "key_prefix": "sk_abcdef...",
  "created_at": "2025-01-14T00:00:00Z",
  "expires_at": "2025-12-31T23:59:59Z",
  "is_active": true,
  "notes": "Unicode test key with emoji and Chinese characters"
}
```

**⚠️ Important**: The full `key` value is only returned once during creation. Store it securely!

### Get API Key
```http
GET /api/v1/auth/keys/{id}
```

**Response:**
```json
{
  "id": "550e8400-e29b-41d4-a716-446655440000",
  "name": "مفتاح الاختبار",
  "key_prefix": "sk_abcdef...",
  "created_at": "2025-01-14T00:00:00Z",
  "updated_at": "2025-01-14T00:00:00Z",
  "last_used_at": "2025-01-14T08:30:00Z",
  "expires_at": "2025-12-31T23:59:59Z",
  "is_active": true,
  "usage_count": 1250,
  "notes": "Arabic API key name example"
}
```

### Update API Key
```http
PUT /api/v1/auth/keys/{id}
```

**Request Body:**
```json
{
  "name": "Updated Key Name ✨",
  "expires_at": "2026-01-31T23:59:59Z",
  "notes": "Updated notes with emoji",
  "is_active": false
}
```

### Revoke API Key
```http
DELETE /api/v1/auth/keys/{id}
```

**Response (204 No Content)**

### Unicode Support Details

#### Supported Characters
- ✅ **Full Unicode Support**: All Unicode characters are supported in API key names
- ✅ **Multilingual**: Chinese (中文), Arabic (العربية), Japanese (日本語), Russian (Русский)
- ✅ **Emojis**: All emoji characters (🔑, ✨, 🌟, etc.)
- ✅ **Mathematical Symbols**: ∑, ∆, ∇, ∞, α, β, γ
- ✅ **Accented Characters**: café, naïve, piñata, résumé

#### Validation Rules
- **Length**: 1-255 characters (Unicode-aware length counting)
- **Forbidden**: ASCII control characters (0-31, 127)
- **Allowed**: All printable Unicode characters, including:
  - Zero-width spaces (U+200B)
  - Unicode line/paragraph separators (U+2028, U+2029)
  - Combining characters and diacritics

#### Examples of Valid Names
```json
{
  "examples": [
    "Production API 🔑",
    "测试密钥",
    "مفتاح API",
    "テストキー",
    "Ключ API",
    "Clé d'API café",
    "API Key ∑∆∇",
    "Mixed: Test مفتاح 테스트 🔑"
  ]
}
```

#### Security Considerations
- API key names are logged and may appear in audit trails
- Consider data privacy laws when using personal information in key names
- Unicode normalization is not applied - names are stored exactly as provided

### Authentication Examples

#### JavaScript/Fetch
```javascript
const response = await fetch('/api/v1/scans', {
  headers: {
    'Authorization': 'Bearer sk_abcdef1234567890abcdef1234567890',
    'Content-Type': 'application/json'
  }
});
```

#### cURL
```bash
curl -H "Authorization: Bearer sk_abcdef1234567890abcdef1234567890" \
     http://localhost:8080/api/v1/scans
```

#### Python Requests
```python
import requests

headers = {
    'Authorization': 'Bearer sk_abcdef1234567890abcdef1234567890',
    'Content-Type': 'application/json'
}

response = requests.get('http://localhost:8080/api/v1/scans', headers=headers)
```

### API Key Management Best Practices

1. **Rotation**: Regularly rotate API keys (recommended: every 90 days)
2. **Least Privilege**: Grant minimal required permissions
3. **Monitoring**: Monitor `usage_count` and `last_used_at` for anomalies  
4. **Expiration**: Always set appropriate expiration dates
5. **Storage**: Store keys securely (environment variables, key vaults)
6. **Naming**: Use descriptive Unicode names for easy identification

### API Key Data Model

```typescript
interface APIKey {
  id: string;
  name: string;           // Full Unicode support
  key?: string;           // Only returned during creation
  key_prefix: string;     // Display prefix (e.g., "sk_abc123...")
  created_at: string;
  updated_at: string;
  last_used_at?: string;
  expires_at?: string;
  is_active: boolean;
  usage_count: number;
  notes?: string;
  created_by?: string;
  permissions?: string[]; // Future: RBAC permissions
}
```

## Error Codes

| Code | Description |
|------|-------------|
| 200  | Success |
| 201  | Created |
| 204  | No Content (for DELETE operations) |
| 400  | Bad Request (validation errors) |
| 404  | Not Found |
| 409  | Conflict (duplicate resources) |
| 500  | Internal Server Error |

## Frontend Integration Examples

### React Hook for Scans
```javascript
import { useState, useEffect } from 'react';

const useScans = () => {
  const [scans, setScans] = useState([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState(null);

  useEffect(() => {
    fetchScans();
  }, []);

  const fetchScans = async () => {
    try {
      setLoading(true);
      const response = await fetch('/api/v1/scans');
      const data = await response.json();
      setScans(data.data);
    } catch (err) {
      setError(err.message);
    } finally {
      setLoading(false);
    }
  };

  const createScan = async (scanData) => {
    const response = await fetch('/api/v1/scans', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(scanData)
    });
    
    if (!response.ok) {
      throw new Error('Failed to create scan');
    }
    
    await fetchScans(); // Refresh list
    return response.json();
  };

  const startScan = async (scanId) => {
    const response = await fetch(`/api/v1/scans/${scanId}/start`, {
      method: 'POST'
    });
    
    if (!response.ok) {
      throw new Error('Failed to start scan');
    }
    
    await fetchScans(); // Refresh list
    return response.json();
  };

  return {
    scans,
    loading,
    error,
    createScan,
    startScan,
    refetch: fetchScans
  };
};
```

### Vue.js Composable for Hosts
```javascript
import { ref, onMounted } from 'vue';

export function useHosts() {
  const hosts = ref([]);
  const loading = ref(true);
  const error = ref(null);

  const fetchHosts = async (filters = {}) => {
    try {
      loading.value = true;
      const params = new URLSearchParams(filters);
      const response = await fetch(`/api/v1/hosts?${params}`);
      const data = await response.json();
      hosts.value = data.data;
    } catch (err) {
      error.value = err.message;
    } finally {
      loading.value = false;
    }
  };

  const getHostScans = async (hostId) => {
    const response = await fetch(`/api/v1/hosts/${hostId}/scans`);
    return response.json();
  };

  onMounted(() => fetchHosts());

  return {
    hosts,
    loading,
    error,
    fetchHosts,
    getHostScans
  };
}
```

### WebSocket Integration (if implemented)
```javascript
const useRealtimeUpdates = () => {
  const [updates, setUpdates] = useState([]);
  
  useEffect(() => {
    const ws = new WebSocket('ws://localhost:8080/api/v1/ws');
    
    ws.onmessage = (event) => {
      const update = JSON.parse(event.data);
      setUpdates(prev => [...prev, update]);
    };
    
    return () => ws.close();
  }, []);
  
  return updates;
};
```

## Data Models

### Scan Object
```typescript
interface Scan {
  id: string;
  name: string;
  description?: string;
  targets: string[];
  scan_type: 'connect' | 'syn' | 'version' | 'aggressive' | 'comprehensive';
  ports: string;
  profile_id?: number;
  options: Record<string, any>;
  schedule_id?: number;
  tags: string[];
  status: 'pending' | 'running' | 'completed' | 'failed';
  progress: number;
  start_time?: string;
  end_time?: string;
  duration?: string;
  created_at: string;
  updated_at: string;
  created_by: string;
}
```

### Host Object
```typescript
interface Host {
  id: number;
  ip: string;
  hostname?: string;
  description?: string;
  os?: string;
  os_version?: string;
  tags: string[];
  metadata: Record<string, string>;
  active: boolean;
  last_seen?: string;
  last_scan_id?: number;
  scan_count: number;
  open_ports: number;
  total_ports: number;
  created_at: string;
  updated_at: string;
  discovered_by?: string;
}
```

### Profile Object
```typescript
interface Profile {
  id: number;
  name: string;
  description?: string;
  scan_type: string;
  ports?: string;
  options: Record<string, string>;
  timing: {
    template?: string;
    min_rtt_timeout?: string;
    max_rtt_timeout?: string;
    initial_rtt_timeout?: string;
    max_retries?: number;
    host_timeout?: string;
    scan_delay?: string;
    max_scan_delay?: string;
  };
  service_detection: boolean;
  os_detection: boolean;
  script_scan: boolean;
  udp_scan: boolean;
  max_retries?: number;
  host_timeout?: string;
  scan_delay?: string;
  max_rate_pps?: number;
  max_host_group_size?: number;
  min_host_group_size?: number;
  tags: string[];
  default: boolean;
  usage_count: number;
  last_used?: string;
  created_at: string;
  updated_at: string;
  created_by?: string;
}
```

## Validation Rules

### Scan Validation
- `name`: Required, max 255 characters
- `targets`: Required, valid CIDR notation or IP addresses
- `scan_type`: Required, one of: `connect`, `syn`, `version`, `aggressive`, `comprehensive`
- `ports`: Valid port ranges (e.g., "22,80,443,8000-9000")

### Host Validation
- `ip`: Required, valid IPv4 or IPv6 address
- `hostname`: Optional, max 255 characters
- `description`: Optional, max 1000 characters
- `tags`: Max 100 tags, each max 50 characters

### Profile Validation
- `name`: Required, unique, max 255 characters
- `scan_type`: Required, valid scan type
- `timing.template`: One of: `paranoid`, `sneaky`, `polite`, `normal`, `aggressive`, `insane`
- `max_retries`: 0-10
- `host_timeout`: Max 30 minutes
- `max_rate_pps`: Max 10000

## Rate Limiting

- Default: 100 requests per minute per IP
- Burst: 20 requests
- Headers included in response:
  - `X-RateLimit-Limit`
  - `X-RateLimit-Remaining`
  - `X-RateLimit-Reset`

## Development Notes

### Current Implementation Status

✅ **Working Endpoints:**
- All health/status endpoints
- Scan CRUD operations
- Host listing and retrieval
- Profile listing and retrieval
- Pagination and filtering
- Error handling for 404/400 cases

⚠️ **Known Issues:**
- Host creation endpoint validation needs refinement
- Profile creation validation requires adjustment
- Host scans relationship endpoint needs testing
- Some edge case validations

🔧 **Recommended Frontend Approach:**
1. Start with list endpoints (hosts, scans, profiles)
2. Implement scan creation and monitoring
3. Add host management features
4. Implement profile management
5. Add real-time updates for scan progress

### Testing the API

You can test the API using curl:

```bash
# Check health
curl http://localhost:8080/api/v1/health

# List all scans
curl http://localhost:8080/api/v1/scans

# List hosts with OS filter
curl "http://localhost:8080/api/v1/hosts?os=linux"

# Get scan results
curl http://localhost:8080/api/v1/scans/{scan-id}/results
```

### Database Connection

The API automatically handles:
- Database migrations on startup
- Connection pooling
- Transaction management
- Error recovery

### Next Steps for Frontend Development

1. **Set up API client**: Create a base API client with error handling
2. **Implement list views**: Start with hosts, scans, and profiles tables
3. **Add creation forms**: Begin with scan creation (most stable)
4. **Implement real-time updates**: Use polling or WebSocket for scan progress
5. **Add dashboard**: Overview of network status and recent scans
6. **Implement filtering/search**: Use the provided query parameters

The API is ready for frontend integration with the majority of endpoints working correctly. Focus on the core workflows first, then expand to advanced features.