# API Validation Rules

This document outlines comprehensive validation rules for all Scanorama API endpoints. Understanding these rules helps ensure successful API requests and proper error handling.

## Table of Contents

- [General Validation Principles](#general-validation-principles)
- [API Key Validation](#api-key-validation)
- [Scan Validation](#scan-validation)
- [Host Validation](#host-validation)
- [Profile Validation](#profile-validation)
- [Discovery Validation](#discovery-validation)
- [Schedule Validation](#schedule-validation)
- [Common Patterns](#common-patterns)
- [Error Response Format](#error-response-format)
- [Validation Examples](#validation-examples)

## General Validation Principles

### Request Headers
```
Content-Type: application/json (required for POST/PUT)
Authorization: Bearer {api_key} (required when auth is enabled)
```

### Character Encoding
- **Default**: UTF-8 encoding for all text fields
- **Unicode Support**: Full Unicode character support where specified
- **Normalization**: No automatic Unicode normalization applied

### Field Requirements
- **Required fields**: Must be present and non-null
- **Optional fields**: Can be omitted or null
- **Immutable fields**: Cannot be changed after creation (e.g., `id`, `created_at`)

## API Key Validation

### Name Field (`name`)
```yaml
Type: string
Required: true
Length: 1-255 characters (Unicode-aware)
Encoding: UTF-8

Allowed Characters:
  - All Unicode printable characters
  - Emoji (🔑, ✨, 🌟, etc.)
  - Mathematical symbols (∑, ∆, ∇, ∞)
  - Accented characters (café, naïve, résumé)
  - CJK characters (中文, 日本語, 한글)
  - Arabic script (العربية)
  - Cyrillic (Русский)
  - Zero-width characters (U+200B)
  - Line/paragraph separators (U+2028, U+2029)

Forbidden Characters:
  - ASCII control characters (0-31)
  - ASCII DEL character (127)
  - Unicode control characters in ranges:
    - U+0080-U+009F (C1 Controls)
    - U+202A-U+202E (Bidirectional formatting)
    - U+2066-U+2069 (Directional isolates)
```

#### Valid Name Examples
```json
{
  "valid_examples": [
    "Production API Key",
    "测试密钥 (Test Key)",
    "مفتاح API الإنتاج",
    "本番API키",
    "Ключ для тестирования",
    "API Key ∑∆∇∞ Math",
    "Café API naïve résumé",
    "Team Alpha 🔑 Key",
    "Multi-line\u2028API\u2029Key",
    "Zero\u200Bwidth\u200Bspaces"
  ]
}
```

#### Invalid Name Examples
```json
{
  "invalid_examples": [
    "",                    // Empty string
    "Test\u0001Key",      // Contains control character
    "API\u007FKey",       // Contains DEL character
    "Test\tKey\n",        // Contains tab/newline (ASCII control)
    "API\u202EKey",       // Contains RTL override
    "A".repeat(256)       // Exceeds 255 character limit
  ]
}
```

### Expiration (`expires_at`)
```yaml
Type: string (RFC3339 datetime) | null
Required: false
Format: "2025-12-31T23:59:59Z"
Validation:
  - Must be in the future (if provided)
  - Must be valid RFC3339 format
  - Maximum: 10 years from creation
  - Timezone: UTC recommended
```

### Notes (`notes`)
```yaml
Type: string
Required: false
Length: 0-1000 characters
Encoding: UTF-8 (full Unicode support)
```

### Permissions (`permissions`)
```yaml
Type: array of strings
Required: false
Valid Values: ["read", "write", "admin", "scan", "host_manage"]
Default: ["read"]
Maximum: 10 permissions per key
```

## Scan Validation

### Name (`name`)
```yaml
Type: string
Required: true
Length: 1-255 characters
Pattern: ^[a-zA-Z0-9\s\-_.,()]+$
Description: Alphanumeric with common punctuation
```

### Targets (`targets`)
```yaml
Type: array of strings
Required: true
Minimum Items: 1
Maximum Items: 100
Valid Formats:
  - IPv4: "192.168.1.1"
  - IPv4 CIDR: "192.168.1.0/24"
  - IPv6: "2001:db8::1"
  - IPv6 CIDR: "2001:db8::/32"
  - Hostname: "example.com"
  - IP Range: "192.168.1.1-192.168.1.254"

Validation Rules:
  - No duplicate targets
  - Private IP ranges allowed
  - Hostnames must be valid FQDN
  - CIDR ranges: /8 to /32 for IPv4, /64 to /128 for IPv6
```

### Scan Type (`scan_type`)
```yaml
Type: string (enum)
Required: true
Valid Values:
  - "connect"      # TCP connect scan
  - "syn"          # SYN stealth scan
  - "version"      # Version detection
  - "aggressive"   # Aggressive scan with OS detection
  - "comprehensive" # Full comprehensive scan
```

### Ports (`ports`)
```yaml
Type: string
Required: false
Default: "22,80,443,8080,8443"
Format Examples:
  - "80,443"              # Specific ports
  - "1-1000"              # Port range
  - "22,80,443,8000-9000" # Mixed format
  - "T:80,443,U:53,161"   # TCP/UDP specific

Validation Rules:
  - Port numbers: 1-65535
  - Maximum 1000 ports per scan
  - Valid separators: comma, dash
  - TCP/UDP prefixes: "T:" and "U:"
```

### Description (`description`)
```yaml
Type: string
Required: false
Length: 0-1000 characters
Encoding: UTF-8
```

### Options (`options`)
```yaml
Type: object
Required: false
Valid Keys:
  timing: "paranoid" | "sneaky" | "polite" | "normal" | "aggressive" | "insane"
  max_retries: 0-10
  source_port: 1-65535
  fragment: boolean
  decoys: array of IP addresses (max 10)
  
Example:
{
  "timing": "normal",
  "max_retries": 3,
  "fragment": false,
  "decoys": ["192.168.1.100", "192.168.1.101"]
}
```

### Tags (`tags`)
```yaml
Type: array of strings
Required: false
Maximum Items: 50
Each Tag:
  Length: 1-50 characters
  Pattern: ^[a-zA-Z0-9\-_]+$
  No duplicates allowed
```

## Host Validation

### IP Address (`ip`)
```yaml
Type: string
Required: true
Valid Formats:
  - IPv4: "192.168.1.100"
  - IPv6: "2001:db8::1"
  - IPv6 with zone: "fe80::1%eth0"

Validation:
  - Must be valid IP address
  - IPv4 and IPv6 both supported
  - No hostname resolution performed
  - Uniqueness enforced (cannot duplicate existing host)
```

### Hostname (`hostname`)
```yaml
Type: string
Required: false
Length: 1-255 characters
Pattern: ^[a-zA-Z0-9]([a-zA-Z0-9\-]{0,61}[a-zA-Z0-9])?(\.[a-zA-Z0-9]([a-zA-Z0-9\-]{0,61}[a-zA-Z0-9])?)*$
Examples:
  - "web-server-01"
  - "db.example.com"
  - "host-123.internal"
```

### Operating System (`os`)
```yaml
Type: string
Required: false
Length: 1-100 characters
Common Values: "Linux", "Windows", "macOS", "FreeBSD", "Unknown"
Case-Insensitive: Stored as provided
```

### OS Version (`os_version`)
```yaml
Type: string
Required: false
Length: 1-255 characters
Examples:
  - "Ubuntu 22.04.3 LTS"
  - "Windows Server 2019"
  - "macOS 14.2"
  - "FreeBSD 13.2"
```

### Metadata (`metadata`)
```yaml
Type: object (key-value pairs)
Required: false
Maximum Keys: 50
Key Rules:
  Length: 1-100 characters
  Pattern: ^[a-zA-Z0-9_-]+$
Value Rules:
  Length: 0-500 characters
  Type: string only

Example:
{
  "location": "datacenter-a",
  "owner": "web-team",
  "environment": "production",
  "cost_center": "engineering"
}
```

## Profile Validation

### Timing Configuration (`timing`)
```yaml
Type: object
Required: false

template: "paranoid" | "sneaky" | "polite" | "normal" | "aggressive" | "insane"
min_rtt_timeout: duration string (e.g., "100ms", "1s")
max_rtt_timeout: duration string (max "30s")
initial_rtt_timeout: duration string
max_retries: 0-10
host_timeout: duration string (max "30m")
scan_delay: duration string (max "10s")
max_scan_delay: duration string (max "60s")

Duration Format:
  - Units: ns, us, ms, s, m, h
  - Examples: "100ms", "5s", "2m", "1h"
  - No spaces allowed
```

### Detection Options
```yaml
service_detection: boolean (default: false)
os_detection: boolean (default: false)
script_scan: boolean (default: false)
udp_scan: boolean (default: false)
```

### Performance Limits
```yaml
max_rate_pps: 1-10000 (packets per second)
max_host_group_size: 1-1024 (default: 64)
min_host_group_size: 1-512 (default: 1)
```

## Discovery Validation

### Network (`network`)
```yaml
Type: string
Required: true
Valid Formats:
  - IPv4 CIDR: "192.168.1.0/24"
  - IPv6 CIDR: "2001:db8::/64"
  
Validation:
  - Must be valid CIDR notation
  - Network ranges: /16 to /30 for IPv4
  - Maximum hosts: 65536 per discovery
```

### Method (`method`)
```yaml
Type: string (enum)
Required: true
Valid Values:
  - "tcp"    # TCP ping/connect
  - "icmp"   # ICMP ping
  - "arp"    # ARP discovery (local networks)
  - "syn"    # SYN discovery
```

## Schedule Validation

### Cron Expression (`cron_expression`)
```yaml
Type: string
Required: true
Format: Standard 5-field cron format
Fields: "minute hour day month weekday"
Range:
  - minute: 0-59
  - hour: 0-23
  - day: 1-31
  - month: 1-12
  - weekday: 0-7 (0 and 7 = Sunday)

Special Characters: * , - /
Examples:
  - "0 2 * * *"     # Daily at 2 AM
  - "0 */4 * * *"   # Every 4 hours
  - "0 9 * * 1-5"   # Weekdays at 9 AM
  - "30 23 * * 0"   # Sundays at 11:30 PM
```

## Common Patterns

### Pagination Parameters
```yaml
page: integer (min: 1, default: 1)
page_size: integer (min: 1, max: 100, default: 20)
```

### Date/Time Fields
```yaml
Format: RFC3339 (ISO 8601)
Examples:
  - "2025-01-14T15:30:00Z"           # UTC
  - "2025-01-14T15:30:00-08:00"      # With timezone
  - "2025-01-14T15:30:00.123Z"       # With milliseconds

Validation:
  - Must be valid RFC3339 format
  - Future dates: checked for expiration fields
  - Past dates: allowed for historical data
```

### UUID Fields
```yaml
Format: UUID v4
Pattern: ^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$
Example: "550e8400-e29b-41d4-a716-446655440000"
Case: Lowercase preferred, case-insensitive accepted
```

### Boolean Fields
```yaml
Accepted Values:
  True: true, "true", "1", 1
  False: false, "false", "0", 0
  
JSON: Use native boolean types (true/false)
Query Params: Use strings ("true"/"false")
```

## Error Response Format

### Validation Error Response
```json
{
  "error": "Validation failed",
  "details": {
    "field": "name",
    "message": "Name must be between 1 and 255 characters",
    "code": "INVALID_LENGTH",
    "value": ""
  },
  "timestamp": "2025-01-14T15:30:00Z",
  "request_id": "550e8400-e29b-41d4-a716-446655440000"
}
```

### Multiple Validation Errors
```json
{
  "error": "Multiple validation errors",
  "details": [
    {
      "field": "name",
      "message": "Name cannot be empty",
      "code": "REQUIRED_FIELD"
    },
    {
      "field": "targets",
      "message": "Invalid IP address format",
      "code": "INVALID_FORMAT",
      "value": "300.300.300.300"
    }
  ],
  "timestamp": "2025-01-14T15:30:00Z",
  "request_id": "550e8400-e29b-41d4-a716-446655440000"
}
```

### Error Codes
```yaml
REQUIRED_FIELD: Field is required but missing
INVALID_FORMAT: Field format is incorrect
INVALID_LENGTH: Field length exceeds limits
INVALID_TYPE: Field type is incorrect
INVALID_VALUE: Field value not in allowed set
DUPLICATE_VALUE: Value already exists (uniqueness constraint)
INVALID_RANGE: Numeric value outside allowed range
INVALID_PATTERN: String doesn't match required pattern
EXPIRED_VALUE: Date/time value is in the past when future required
INVALID_UNICODE: Unicode character validation failed
INVALID_ENCODING: Character encoding validation failed
```

## Validation Examples

### Creating an API Key with Unicode
```bash
# Valid Request
curl -X POST http://localhost:8080/api/v1/auth/keys \
  -H "Content-Type: application/json" \
  -d '{
    "name": "生产环境密钥 🔑",
    "expires_at": "2025-12-31T23:59:59Z",
    "notes": "Production key with Chinese characters and emoji"
  }'

# Invalid Request - Control Character
curl -X POST http://localhost:8080/api/v1/auth/keys \
  -H "Content-Type: application/json" \
  -d '{
    "name": "Test\u0001Key"
  }'

# Error Response
{
  "error": "Validation failed",
  "details": {
    "field": "name",
    "message": "key name contains invalid characters",
    "code": "INVALID_UNICODE",
    "value": "Test\u0001Key"
  }
}
```

### Creating a Scan with Validation Errors
```bash
# Invalid Request
curl -X POST http://localhost:8080/api/v1/scans \
  -H "Content-Type: application/json" \
  -d '{
    "name": "",
    "targets": ["300.300.300.300", "192.168.1.0/8"],
    "scan_type": "invalid_type",
    "ports": "99999"
  }'

# Error Response
{
  "error": "Multiple validation errors",
  "details": [
    {
      "field": "name",
      "message": "Name cannot be empty",
      "code": "REQUIRED_FIELD"
    },
    {
      "field": "targets[0]",
      "message": "Invalid IP address format",
      "code": "INVALID_FORMAT",
      "value": "300.300.300.300"
    },
    {
      "field": "targets[1]",
      "message": "CIDR range too broad, maximum /16 allowed",
      "code": "INVALID_RANGE",
      "value": "192.168.1.0/8"
    },
    {
      "field": "scan_type",
      "message": "Must be one of: connect, syn, version, aggressive, comprehensive",
      "code": "INVALID_VALUE",
      "value": "invalid_type"
    },
    {
      "field": "ports",
      "message": "Port number must be between 1 and 65535",
      "code": "INVALID_RANGE",
      "value": "99999"
    }
  ]
}
```

### Valid Host Creation
```bash
curl -X POST http://localhost:8080/api/v1/hosts \
  -H "Content-Type: application/json" \
  -d '{
    "ip": "192.168.1.100",
    "hostname": "web-server-01.example.com",
    "description": "Primary web server",
    "os": "Linux",
    "os_version": "Ubuntu 22.04.3 LTS",
    "tags": ["production", "web", "ubuntu"],
    "metadata": {
      "location": "datacenter-a",
      "owner": "web-team",
      "environment": "production"
    }
  }'
```

## Best Practices

### For API Consumers

1. **Always validate on client-side** before sending requests
2. **Handle validation errors gracefully** with user-friendly messages
3. **Use proper character encoding** (UTF-8) for Unicode content
4. **Test with Unicode characters** to ensure proper handling
5. **Implement proper error handling** for all validation scenarios

### For Frontend Developers

1. **Form Validation**: Implement client-side validation matching server rules
2. **Unicode Support**: Ensure forms properly handle Unicode input
3. **Error Display**: Show field-specific validation errors clearly
4. **Real-time Validation**: Provide immediate feedback for format errors
5. **Testing**: Test with various Unicode characters and edge cases

### Security Considerations

1. **Input Sanitization**: Always validate and sanitize user input
2. **Unicode Security**: Be aware of Unicode-based security issues
3. **Length Limits**: Enforce proper length limits to prevent DoS
4. **Character Validation**: Block dangerous control characters
5. **Encoding**: Always use UTF-8 encoding consistently