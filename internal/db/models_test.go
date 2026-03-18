package db

import (
	"database/sql/driver"
	"net"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestNetworkAddr tests the NetworkAddr type for PostgreSQL CIDR
func TestNetworkAddr(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantErr bool
	}{
		{
			name:    "valid IPv4 CIDR",
			input:   "192.168.1.0/24",
			wantErr: false,
		},
		{
			name:    "valid IPv6 CIDR",
			input:   "2001:db8::/32",
			wantErr: false,
		},
		{
			name:    "invalid CIDR",
			input:   "not-a-cidr",
			wantErr: true,
		},
		{
			name:    "IP without mask",
			input:   "192.168.1.1",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var addr NetworkAddr

			// Test Scan method
			err := addr.Scan(tt.input)
			if tt.wantErr {
				assert.Error(t, err)
				return
			}

			require.NoError(t, err)
			assert.Equal(t, tt.input, addr.String())

			// Test Value method
			value, err := addr.Value()
			require.NoError(t, err)
			assert.Equal(t, tt.input, value)

			// Test round-trip with bytes
			var addr2 NetworkAddr
			err = addr2.Scan([]byte(tt.input))
			require.NoError(t, err)
			assert.Equal(t, addr.String(), addr2.String())
		})
	}
}

// TestNetworkAddrEdgeCases tests edge cases for NetworkAddr
func TestNetworkAddrEdgeCases(t *testing.T) {
	var addr NetworkAddr

	// Test nil scan
	err := addr.Scan(nil)
	assert.NoError(t, err)

	// Test empty NetworkAddr value
	value, err := addr.Value()
	assert.NoError(t, err)
	assert.Nil(t, value)

	// Test invalid type scan
	err = addr.Scan(123)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "cannot scan")
}

// TestIPAddr tests the IPAddr type for PostgreSQL INET
func TestIPAddr(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantErr bool
	}{
		{
			name:    "valid IPv4",
			input:   "192.168.1.100",
			wantErr: false,
		},
		{
			name:    "valid IPv6",
			input:   "2001:db8::1",
			wantErr: false,
		},
		{
			name:    "invalid IP",
			input:   "not-an-ip",
			wantErr: true,
		},
		{
			name:    "empty string",
			input:   "",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var addr IPAddr

			// Test Scan method
			err := addr.Scan(tt.input)
			if tt.wantErr {
				assert.Error(t, err)
				return
			}

			require.NoError(t, err)
			assert.Equal(t, tt.input, addr.String())

			// Test Value method
			value, err := addr.Value()
			require.NoError(t, err)
			assert.Equal(t, tt.input, value)

			// Test round-trip with bytes
			var addr2 IPAddr
			err = addr2.Scan([]byte(tt.input))
			require.NoError(t, err)
			assert.Equal(t, addr.String(), addr2.String())
		})
	}
}

// TestIPAddrEdgeCases tests edge cases for IPAddr
func TestIPAddrEdgeCases(t *testing.T) {
	var addr IPAddr

	// Test nil scan
	err := addr.Scan(nil)
	assert.NoError(t, err)

	// Test empty IPAddr value
	value, err := addr.Value()
	assert.NoError(t, err)
	assert.Nil(t, value)

	// Test string representation of nil IP
	assert.Equal(t, "", addr.String())

	// Test invalid type scan
	err = addr.Scan(123)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "cannot scan")
}

// TestMACAddr tests the MACAddr type for PostgreSQL MACADDR
func TestMACAddr(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantErr bool
	}{
		{
			name:    "valid MAC with colons",
			input:   "aa:bb:cc:dd:ee:ff",
			wantErr: false,
		},
		{
			name:    "valid MAC with dashes",
			input:   "aa-bb-cc-dd-ee-ff",
			wantErr: false,
		},
		{
			name:    "valid MAC without separators",
			input:   "aabbccddeeff",
			wantErr: false,
		},
		{
			name:    "invalid MAC",
			input:   "not-a-mac",
			wantErr: true,
		},
		{
			name:    "too short",
			input:   "aa:bb:cc",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var addr MACAddr

			// Test Scan method
			err := addr.Scan(tt.input)
			if tt.wantErr {
				assert.Error(t, err)
				return
			}

			require.NoError(t, err)

			// Normalize expected output (Go always uses colons)
			expectedMAC, err := net.ParseMAC(tt.input)
			require.NoError(t, err)
			assert.Equal(t, expectedMAC.String(), addr.String())

			// Test Value method
			value, err := addr.Value()
			require.NoError(t, err)
			assert.Equal(t, expectedMAC.String(), value)

			// Test round-trip with bytes
			var addr2 MACAddr
			err = addr2.Scan([]byte(tt.input))
			require.NoError(t, err)
			assert.Equal(t, addr.String(), addr2.String())
		})
	}
}

// TestMACAddrEdgeCases tests edge cases for MACAddr
func TestMACAddrEdgeCases(t *testing.T) {
	var addr MACAddr

	// Test nil scan
	err := addr.Scan(nil)
	assert.NoError(t, err)

	// Test empty MACAddr value
	value, err := addr.Value()
	assert.NoError(t, err)
	assert.Nil(t, value)

	// Test string representation of nil MAC
	assert.Equal(t, "", addr.String())

	// Test invalid type scan
	err = addr.Scan(123)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "cannot scan")
}

// TestJSONB tests the JSONB type for PostgreSQL JSONB
func TestJSONB(t *testing.T) {
	tests := []struct {
		name     string
		input    interface{}
		expected string
	}{
		{
			name:     "simple object",
			input:    `{"key": "value"}`,
			expected: `{"key": "value"}`,
		},
		{
			name:     "array",
			input:    `[1, 2, 3]`,
			expected: `[1, 2, 3]`,
		},
		{
			name:     "complex object",
			input:    `{"users": [{"name": "John", "age": 30}], "count": 1}`,
			expected: `{"users": [{"name": "John", "age": 30}], "count": 1}`,
		},
		{
			name:     "null",
			input:    `null`,
			expected: `null`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var j JSONB

			// Test Scan with string
			err := j.Scan(tt.input)
			require.NoError(t, err)
			assert.Equal(t, tt.expected, j.String())

			// Test Value method
			value, err := j.Value()
			require.NoError(t, err)
			assert.Equal(t, []byte(tt.expected), value)

			// Test Scan with bytes
			var j2 JSONB
			err = j2.Scan([]byte(tt.input.(string)))
			require.NoError(t, err)
			assert.Equal(t, j.String(), j2.String())

			// Test JSON marshaling
			marshaled, err := j.MarshalJSON()
			require.NoError(t, err)
			assert.Equal(t, []byte(tt.expected), marshaled)

			// Test JSON unmarshaling
			var j3 JSONB
			err = j3.UnmarshalJSON(marshaled)
			require.NoError(t, err)
			assert.Equal(t, j.String(), j3.String())
		})
	}
}

// TestJSONBEdgeCases tests edge cases for JSONB
func TestJSONBEdgeCases(t *testing.T) {
	var j JSONB

	// Test nil scan
	err := j.Scan(nil)
	assert.NoError(t, err)
	assert.Nil(t, j)

	// Test nil value
	value, err := j.Value()
	assert.NoError(t, err)
	assert.Nil(t, value)

	// Test nil marshal
	marshaled, err := j.MarshalJSON()
	assert.NoError(t, err)
	assert.Equal(t, []byte("null"), marshaled)

	// Test invalid type scan
	err = j.Scan(123)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "cannot scan")
}

// TestConstantValues checks the actual string values of every constant.
// This acts as a change-detector: if a constant value is accidentally renamed
// or changed, this test will catch it.
func TestConstantValues(t *testing.T) {
	assert.Equal(t, "up", HostStatusUp)
	assert.Equal(t, "down", HostStatusDown)
	assert.Equal(t, "unknown", HostStatusUnknown)
	assert.Equal(t, "open", PortStateOpen)
	assert.Equal(t, "closed", PortStateClosed)
	assert.Equal(t, "filtered", PortStateFiltered)
	assert.Equal(t, "unknown", PortStateUnknown)
	assert.Equal(t, "connect", ScanTypeConnect)
	assert.Equal(t, "syn", ScanTypeSYN)
	assert.Equal(t, "version", ScanTypeVersion)
	assert.Equal(t, "tcp", ProtocolTCP)
	assert.Equal(t, "udp", ProtocolUDP)
	assert.Equal(t, "pending", ScanJobStatusPending)
	assert.Equal(t, "running", ScanJobStatusRunning)
	assert.Equal(t, "completed", ScanJobStatusCompleted)
	assert.Equal(t, "failed", ScanJobStatusFailed)
	assert.Equal(t, "discovered", HostEventDiscovered)
	assert.Equal(t, "status_change", HostEventStatusChange)
	assert.Equal(t, "ports_changed", HostEventPortsChanged)
	assert.Equal(t, "service_found", HostEventServiceFound)
}

// TestConstantUniqueness verifies that constants within each group are distinct.
func TestConstantUniqueness(t *testing.T) {
	t.Run("host statuses are unique", func(t *testing.T) {
		assertUnique(t, []string{HostStatusUp, HostStatusDown, HostStatusUnknown})
	})
	t.Run("port states are unique", func(t *testing.T) {
		assertUnique(t, []string{PortStateOpen, PortStateClosed, PortStateFiltered, PortStateUnknown})
	})
	t.Run("scan types are unique", func(t *testing.T) {
		assertUnique(t, []string{ScanTypeConnect, ScanTypeSYN, ScanTypeVersion})
	})
	t.Run("protocols are unique", func(t *testing.T) {
		assertUnique(t, []string{ProtocolTCP, ProtocolUDP})
	})
	t.Run("scan job statuses are unique", func(t *testing.T) {
		assertUnique(t, []string{ScanJobStatusPending, ScanJobStatusRunning, ScanJobStatusCompleted, ScanJobStatusFailed})
	})
	t.Run("host history events are unique", func(t *testing.T) {
		assertUnique(t, []string{HostEventDiscovered, HostEventStatusChange, HostEventPortsChanged, HostEventServiceFound})
	})
}

func assertUnique(t *testing.T, values []string) {
	t.Helper()
	seen := make(map[string]bool)
	for _, v := range values {
		assert.False(t, seen[v], "duplicate value: %q", v)
		seen[v] = true
	}
}

// Helper function for tests
func stringPtr(s string) *string {
	return &s
}

// Compile-time interface satisfaction checks.
var (
	_ driver.Valuer = (*NetworkAddr)(nil)
	_ driver.Valuer = (*IPAddr)(nil)
	_ driver.Valuer = (*MACAddr)(nil)
	_ driver.Valuer = (*JSONB)(nil)
)

// Benchmark tests for performance-critical operations
func BenchmarkNetworkAddrScan(b *testing.B) {
	cidr := "192.168.1.0/24"
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		var addr NetworkAddr
		_ = addr.Scan(cidr)
	}
}

func BenchmarkIPAddrScan(b *testing.B) {
	ip := "192.168.1.100"
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		var addr IPAddr
		_ = addr.Scan(ip)
	}
}

func BenchmarkJSONBScan(b *testing.B) {
	jsonData := `{"key": "value", "number": 42, "array": [1,2,3]}`
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		var j JSONB
		_ = j.Scan(jsonData)
	}
}
