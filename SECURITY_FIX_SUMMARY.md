# Security Fix Summary: Unsafe Deserialization Vulnerability

## Overview

This document summarizes the security vulnerability that was identified and fixed in the Scanorama network scanner project. The issue was related to unsafe deserialization practices in the configuration management API endpoints.

## Vulnerability Details

### Issue Type
**CWE-502: Deserialization of Untrusted Data**

### Location
- Primary: `internal/api/handlers/admin.go` - `UpdateConfig` endpoint
- Secondary: `internal/config/config.go` - Configuration file loading

### Risk Level
**High** - Potential for remote code execution

### Description
The vulnerability existed in the admin API endpoint `/api/v1/admin/config` where configuration updates were accepted. The endpoint used an `interface{}` type for configuration data, allowing arbitrary JSON structures to be deserialized without proper validation or type safety.

```go
// VULNERABLE CODE (before fix)
type ConfigUpdateRequest struct {
    Section string      `json:"section"`
    Config  interface{} `json:"config"` // ⚠️ Unsafe - allows any structure
}
```

### Attack Vector
An attacker with API access could potentially:
1. Send malicious JSON payloads with arbitrary structures
2. Exploit type confusion vulnerabilities
3. Cause memory exhaustion through oversized payloads
4. Inject malicious configuration values

## Security Fix Implementation

### 1. Typed Configuration Structures

Replaced the unsafe `interface{}` with properly typed structures for each configuration section:

```go
// SECURE CODE (after fix)
type ConfigUpdateRequest struct {
    Section string           `json:"section" validate:"required,oneof=api database scanning logging daemon"`
    Config  ConfigUpdateData `json:"config" validate:"required"`
}

type ConfigUpdateData struct {
    API      *APIConfigUpdate      `json:"api,omitempty"`
    Database *DatabaseConfigUpdate `json:"database,omitempty"`
    Scanning *ScanningConfigUpdate `json:"scanning,omitempty"`
    Logging  *LoggingConfigUpdate  `json:"logging,omitempty"`
    Daemon   *DaemonConfigUpdate   `json:"daemon,omitempty"`
}
```

### 2. Input Validation Framework

Implemented comprehensive input validation using:
- **Struct validation**: Using `github.com/go-playground/validator/v10`
- **Custom security validators**: For paths, hostnames, durations, and ports
- **Size limits**: Maximum request size (1MB) and field length limits
- **Content validation**: Checks for null bytes, control characters, and malicious patterns

### 3. Secure JSON Parsing

Enhanced JSON parsing with security constraints:

```go
func parseConfigJSON(r *http.Request, dest interface{}) error {
    // Enforce maximum request size (1MB)
    const maxConfigSize = 1024 * 1024
    r.Body = http.MaxBytesReader(nil, r.Body, maxConfigSize)

    decoder := json.NewDecoder(r.Body)
    decoder.DisallowUnknownFields() // Reject unknown fields
    decoder.UseNumber()             // Prevent precision issues

    if err := decoder.Decode(dest); err != nil {
        if err.Error() == "http: request body too large" {
            return fmt.Errorf("configuration data too large (max 1MB)")
        }
        return fmt.Errorf("invalid JSON: %w", err)
    }

    return nil
}
```

### 4. Configuration File Security

Enhanced configuration file loading with additional security measures:

- **File permission validation**: Ensures config files aren't world-readable or group-writable
- **Path traversal protection**: Prevents directory traversal attacks
- **Size limits**: Maximum file size of 10MB
- **Content validation**: Checks for binary data and malicious content
- **Extension validation**: Only allows `.yaml`, `.yml`, and `.json` files

### 5. Validation Constants

Added security-focused constants to prevent magic numbers and ensure consistent validation:

```go
const (
    maxDatabaseNameLength     = 63          // PostgreSQL limits
    maxUsernameLength         = 63          // PostgreSQL limits  
    maxAdminPortsStringLength = 1000        // Reasonable port string limit
    maxDurationStringLength   = 50          // Duration string limit
    maxPathLength             = 4096        // Maximum file path length
    maxConfigSize             = 1024 * 1024 // Maximum configuration size (1MB)
    maxAdminHostnameLength    = 255         // Maximum hostname length
)
```

## Security Measures Implemented

### 1. Defense in Depth
- **Input validation** at multiple layers
- **Type safety** through strongly typed structs
- **Size limits** to prevent DoS attacks
- **Content filtering** to block malicious data

### 2. Principle of Least Privilege
- **Field-specific validation** for each configuration type
- **Section isolation** - only allows updates to specified sections
- **Strict parsing** - rejects unknown fields and malformed data

### 3. Fail-Safe Defaults
- **Conservative size limits** 
- **Strict file permissions** requirements
- **Graceful error handling** with detailed security-focused error messages

## Testing

### Security Test Suite
Created comprehensive security tests in `admin_security_test.go`:

- **Typed configuration validation** tests
- **Malformed input rejection** tests  
- **Field validation constraint** tests
- **Size limit enforcement** tests
- **Path traversal prevention** tests
- **Input sanitization** tests

### Test Coverage
- ✅ Prevents unsafe deserialization
- ✅ Enforces size limits
- ✅ Validates field constraints
- ✅ Rejects malformed structures
- ✅ Blocks directory traversal
- ✅ Sanitizes string inputs

## Impact Assessment

### Before Fix
- ❌ Configuration API vulnerable to arbitrary JSON deserialization
- ❌ No input size limits
- ❌ Insufficient validation of configuration values
- ❌ Potential for remote code execution

### After Fix
- ✅ Strongly typed configuration structures prevent unsafe deserialization
- ✅ Comprehensive input validation with security constraints
- ✅ Size limits prevent DoS attacks
- ✅ Path validation prevents directory traversal
- ✅ Content filtering blocks malicious inputs
- ✅ Extensive test coverage for security scenarios

## Compatibility

### Backward Compatibility
- ✅ Existing configuration files continue to work
- ✅ Environment variable overrides still function
- ✅ CLI flag overrides maintained
- ✅ API response formats unchanged

### Breaking Changes
- Configuration update API now requires properly structured JSON
- Unknown fields in configuration updates are rejected
- Stricter validation may reject previously accepted invalid values

## Recommendations

### 1. Security Monitoring
- Monitor for rejected configuration update attempts
- Log validation failures for security analysis
- Set up alerts for repeated invalid requests

### 2. Additional Hardening
- Consider implementing API rate limiting for admin endpoints
- Add audit logging for all configuration changes
- Implement configuration change approval workflows for production

### 3. Regular Security Reviews
- Perform periodic security audits of API endpoints
- Review and update validation rules as needed
- Keep dependencies updated for latest security patches

## Verification

To verify the fix is working:

1. **Run security tests**: `go test ./internal/api/handlers -run TestAdminHandler_ConfigSecurity`
2. **Check linting**: `golangci-lint run`
3. **Run full test suite**: `go test ./...`
4. **Test with malicious payloads**: Use the provided test cases

## Files Modified

- `internal/api/handlers/admin.go` - Main security fix implementation
- `internal/api/handlers/common.go` - Enhanced JSON parsing security
- `internal/config/config.go` - Configuration file loading security
- `internal/api/handlers/admin_security_test.go` - Security test suite (new file)

## Security Contact

For security-related issues or questions about this fix, please follow the project's security reporting guidelines.

---

**Fix Status**: ✅ Complete  
**Date**: 2025-01-11  
**Severity**: High → Resolved  
**Impact**: Configuration API now secure against deserialization attacks