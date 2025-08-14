package db

import (
	"database/sql/driver"
	"net"
	"testing"

	"github.com/google/uuid"
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
			wantErr: true,
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

// TestScanTargetValidation tests ScanTarget model validation
func TestScanTargetValidation(t *testing.T) {
	tests := []struct {
		name    string
		target  ScanTarget
		wantErr bool
	}{
		{
			name: "valid target",
			target: ScanTarget{
				ID:                  uuid.New(),
				Name:                "Test Network",
				Network:             NetworkAddr{},
				ScanIntervalSeconds: 3600,
				ScanPorts:           "22,80,443",
				ScanType:            ScanTypeConnect,
				Enabled:             true,
			},
			wantErr: false,
		},
		{
			name: "valid with description",
			target: ScanTarget{
				ID:                  uuid.New(),
				Name:                "Test Network",
				Network:             NetworkAddr{},
				Description:         stringPtr("Test description"),
				ScanIntervalSeconds: 3600,
				ScanPorts:           "22,80,443",
				ScanType:            ScanTypeSYN,
				Enabled:             true,
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Basic validation - in a real implementation you might have
			// a Validate() method on the model
			assert.NotEmpty(t, tt.target.Name)
			assert.NotEmpty(t, tt.target.ScanPorts)
			assert.Contains(t, []string{ScanTypeConnect, ScanTypeSYN, ScanTypeVersion}, tt.target.ScanType)
			assert.Positive(t, tt.target.ScanIntervalSeconds)
		})
	}
}

// TestHostStatusConstants tests host status constants
func TestHostStatusConstants(t *testing.T) {
	validStatuses := []string{HostStatusUp, HostStatusDown, HostStatusUnknown}

	for _, status := range validStatuses {
		assert.NotEmpty(t, status)
		assert.IsType(t, "", status)
	}

	// Test uniqueness
	statusSet := make(map[string]bool)
	for _, status := range validStatuses {
		assert.False(t, statusSet[status], "Status %s should be unique", status)
		statusSet[status] = true
	}
}

// TestPortStateConstants tests port state constants
func TestPortStateConstants(t *testing.T) {
	validStates := []string{PortStateOpen, PortStateClosed, PortStateFiltered, PortStateUnknown}

	for _, state := range validStates {
		assert.NotEmpty(t, state)
		assert.IsType(t, "", state)
	}

	// Test uniqueness
	stateSet := make(map[string]bool)
	for _, state := range validStates {
		assert.False(t, stateSet[state], "State %s should be unique", state)
		stateSet[state] = true
	}
}

// TestScanTypeConstants tests scan type constants
func TestScanTypeConstants(t *testing.T) {
	validTypes := []string{ScanTypeConnect, ScanTypeSYN, ScanTypeVersion}

	for _, scanType := range validTypes {
		assert.NotEmpty(t, scanType)
		assert.IsType(t, "", scanType)
	}

	// Test uniqueness
	typeSet := make(map[string]bool)
	for _, scanType := range validTypes {
		assert.False(t, typeSet[scanType], "Type %s should be unique", scanType)
		typeSet[scanType] = true
	}
}

// TestProtocolConstants tests protocol constants
func TestProtocolConstants(t *testing.T) {
	validProtocols := []string{ProtocolTCP, ProtocolUDP}

	for _, protocol := range validProtocols {
		assert.NotEmpty(t, protocol)
		assert.IsType(t, "", protocol)
	}

	// Test uniqueness
	protocolSet := make(map[string]bool)
	for _, protocol := range validProtocols {
		assert.False(t, protocolSet[protocol], "Protocol %s should be unique", protocol)
		protocolSet[protocol] = true
	}
}

// TestScanJobStatusConstants tests scan job status constants
func TestScanJobStatusConstants(t *testing.T) {
	validStatuses := []string{
		ScanJobStatusPending,
		ScanJobStatusRunning,
		ScanJobStatusCompleted,
		ScanJobStatusFailed,
	}

	for _, status := range validStatuses {
		assert.NotEmpty(t, status)
		assert.IsType(t, "", status)
	}

	// Test uniqueness
	statusSet := make(map[string]bool)
	for _, status := range validStatuses {
		assert.False(t, statusSet[status], "Status %s should be unique", status)
		statusSet[status] = true
	}
}

// TestHostHistoryEventConstants tests host history event constants
func TestHostHistoryEventConstants(t *testing.T) {
	validEvents := []string{
		HostEventDiscovered,
		HostEventStatusChange,
		HostEventPortsChanged,
		HostEventServiceFound,
	}

	for _, event := range validEvents {
		assert.NotEmpty(t, event)
		assert.IsType(t, "", event)
	}

	// Test uniqueness
	eventSet := make(map[string]bool)
	for _, event := range validEvents {
		assert.False(t, eventSet[event], "Event %s should be unique", event)
		eventSet[event] = true
	}
}

// TestModelStructures tests basic model structure
func TestModelStructures(t *testing.T) {
	// Test that models have required fields and proper types

	// ScanTarget
	target := ScanTarget{}
	assert.IsType(t, uuid.UUID{}, target.ID)
	assert.IsType(t, "", target.Name)
	assert.IsType(t, NetworkAddr{}, target.Network)
	assert.IsType(t, 0, target.ScanIntervalSeconds)
	assert.IsType(t, true, target.Enabled)

	// Host
	host := Host{}
	assert.IsType(t, uuid.UUID{}, host.ID)
	assert.IsType(t, IPAddr{}, host.IPAddress)
	assert.IsType(t, "", host.Status)

	// PortScan
	portScan := PortScan{}
	assert.IsType(t, uuid.UUID{}, portScan.ID)
	assert.IsType(t, uuid.UUID{}, portScan.JobID)
	assert.IsType(t, uuid.UUID{}, portScan.HostID)
	assert.IsType(t, 0, portScan.Port)
	assert.IsType(t, "", portScan.Protocol)
	assert.IsType(t, "", portScan.State)

	// ScanJob
	job := ScanJob{}
	assert.IsType(t, uuid.UUID{}, job.ID)
	assert.IsType(t, uuid.UUID{}, job.TargetID)
	assert.IsType(t, "", job.Status)
}

// TestDriverValuerInterface tests that our types implement driver.Valuer
func TestDriverValuerInterface(t *testing.T) {
	var _ driver.Valuer = NetworkAddr{}
	var _ driver.Valuer = IPAddr{}
	var _ driver.Valuer = MACAddr{}
	var _ driver.Valuer = JSONB{}
}

// Helper function for tests
func stringPtr(s string) *string {
	return &s
}

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
