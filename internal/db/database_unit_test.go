package db

import (
	"database/sql/driver"
	"net"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestDefaultConfig tests the default database configuration.
func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()

	assert.Equal(t, "localhost", cfg.Host)
	assert.Equal(t, 5432, cfg.Port)
	assert.Equal(t, "", cfg.Database)
	assert.Equal(t, "", cfg.Username)
	assert.Equal(t, "", cfg.Password)
	assert.Equal(t, "disable", cfg.SSLMode)
	assert.Equal(t, 25, cfg.MaxOpenConns)
	assert.Equal(t, 5, cfg.MaxIdleConns)
	assert.Equal(t, 5*time.Minute, cfg.ConnMaxLifetime)
	assert.Equal(t, 5*time.Minute, cfg.ConnMaxIdleTime)
}

// TestNetworkAddrExtended tests the NetworkAddr custom type with extended scenarios.
func TestNetworkAddrExtended(t *testing.T) {
	t.Run("scan_valid_cidr", func(t *testing.T) {
		var addr NetworkAddr
		err := addr.Scan("192.168.1.0/24")
		require.NoError(t, err)
		assert.Equal(t, "192.168.1.0/24", addr.String())
	})

	t.Run("scan_valid_cidr_bytes", func(t *testing.T) {
		var addr NetworkAddr
		err := addr.Scan([]byte("10.0.0.0/8"))
		require.NoError(t, err)
		assert.Equal(t, "10.0.0.0/8", addr.String())
	})

	t.Run("scan_invalid_cidr", func(t *testing.T) {
		var addr NetworkAddr
		err := addr.Scan("invalid-cidr")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to parse CIDR")
	})

	t.Run("scan_nil_value", func(t *testing.T) {
		var addr NetworkAddr
		err := addr.Scan(nil)
		assert.NoError(t, err)
	})

	t.Run("scan_unsupported_type", func(t *testing.T) {
		var addr NetworkAddr
		err := addr.Scan(123)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "cannot scan")
	})

	t.Run("value_empty", func(t *testing.T) {
		var addr NetworkAddr
		val, err := addr.Value()
		assert.NoError(t, err)
		assert.Nil(t, val)
	})

	t.Run("value_with_network", func(t *testing.T) {
		_, ipnet, err := net.ParseCIDR("172.16.0.0/12")
		require.NoError(t, err)

		addr := NetworkAddr{IPNet: *ipnet}
		val, err := addr.Value()
		assert.NoError(t, err)
		assert.Equal(t, "172.16.0.0/12", val)
	})
}

// TestIPAddrExtended tests the IPAddr custom type with extended scenarios.
func TestIPAddrExtended(t *testing.T) {
	t.Run("scan_valid_ipv4", func(t *testing.T) {
		var addr IPAddr
		err := addr.Scan("192.168.1.1")
		require.NoError(t, err)
		assert.Equal(t, "192.168.1.1", addr.String())
	})

	t.Run("scan_valid_ipv6", func(t *testing.T) {
		var addr IPAddr
		err := addr.Scan("2001:db8::1")
		require.NoError(t, err)
		assert.Equal(t, "2001:db8::1", addr.String())
	})

	t.Run("scan_bytes", func(t *testing.T) {
		var addr IPAddr
		err := addr.Scan([]byte("127.0.0.1"))
		require.NoError(t, err)
		assert.Equal(t, "127.0.0.1", addr.String())
	})

	t.Run("scan_invalid_ip", func(t *testing.T) {
		var addr IPAddr
		err := addr.Scan("invalid-ip")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to parse IP address")
	})

	t.Run("scan_nil", func(t *testing.T) {
		var addr IPAddr
		err := addr.Scan(nil)
		assert.NoError(t, err)
	})

	t.Run("scan_unsupported_type", func(t *testing.T) {
		var addr IPAddr
		err := addr.Scan(42)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "cannot scan")
	})

	t.Run("value_nil", func(t *testing.T) {
		var addr IPAddr
		val, err := addr.Value()
		assert.NoError(t, err)
		assert.Nil(t, val)
	})

	t.Run("value_with_ip", func(t *testing.T) {
		addr := IPAddr{IP: net.ParseIP("10.1.1.1")}
		val, err := addr.Value()
		assert.NoError(t, err)
		assert.Equal(t, "10.1.1.1", val)
	})

	t.Run("string_nil_ip", func(t *testing.T) {
		var addr IPAddr
		assert.Equal(t, "", addr.String())
	})
}

// TestMACAddrExtended tests the MACAddr custom type with extended scenarios.
func TestMACAddrExtended(t *testing.T) {
	t.Run("scan_valid_mac_colons", func(t *testing.T) {
		var addr MACAddr
		err := addr.Scan("00:11:22:33:44:55")
		require.NoError(t, err)
		assert.Equal(t, "00:11:22:33:44:55", addr.String())
	})

	t.Run("scan_valid_mac_dashes", func(t *testing.T) {
		var addr MACAddr
		err := addr.Scan("00-11-22-33-44-55")
		require.NoError(t, err)
		assert.Equal(t, "00:11:22:33:44:55", addr.String())
	})

	t.Run("scan_valid_mac_no_separators", func(t *testing.T) {
		var addr MACAddr
		err := addr.Scan("001122334455")
		require.NoError(t, err)
		assert.Equal(t, "00:11:22:33:44:55", addr.String())
	})

	t.Run("scan_bytes", func(t *testing.T) {
		var addr MACAddr
		err := addr.Scan([]byte("aa:bb:cc:dd:ee:ff"))
		require.NoError(t, err)
		assert.Equal(t, "aa:bb:cc:dd:ee:ff", addr.String())
	})

	t.Run("scan_invalid_mac", func(t *testing.T) {
		var addr MACAddr
		err := addr.Scan("invalid-mac")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to parse MAC address")
	})

	t.Run("scan_nil", func(t *testing.T) {
		var addr MACAddr
		err := addr.Scan(nil)
		assert.NoError(t, err)
	})

	t.Run("scan_unsupported_type", func(t *testing.T) {
		var addr MACAddr
		err := addr.Scan(123)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "cannot scan")
	})

	t.Run("value_nil", func(t *testing.T) {
		var addr MACAddr
		val, err := addr.Value()
		assert.NoError(t, err)
		assert.Nil(t, val)
	})

	t.Run("value_with_mac", func(t *testing.T) {
		hw, err := net.ParseMAC("12:34:56:78:9a:bc")
		require.NoError(t, err)

		addr := MACAddr{HardwareAddr: hw}
		val, err := addr.Value()
		assert.NoError(t, err)
		assert.Equal(t, "12:34:56:78:9a:bc", val)
	})

	t.Run("string_nil_mac", func(t *testing.T) {
		var addr MACAddr
		assert.Equal(t, "", addr.String())
	})
}

// TestJSONBExtended tests the JSONB custom type with extended scenarios.
func TestJSONBExtended(t *testing.T) {
	t.Run("scan_valid_json_string", func(t *testing.T) {
		var j JSONB
		err := j.Scan(`{"key": "value", "number": 42}`)
		require.NoError(t, err)
		assert.JSONEq(t, `{"key": "value", "number": 42}`, string(j))
	})

	t.Run("scan_valid_json_bytes", func(t *testing.T) {
		var j JSONB
		err := j.Scan([]byte(`[1, 2, 3]`))
		require.NoError(t, err)
		assert.JSONEq(t, `[1, 2, 3]`, string(j))
	})

	t.Run("scan_invalid_json", func(t *testing.T) {
		var j JSONB
		err := j.Scan(`{invalid json`)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "invalid JSON")
	})

	t.Run("scan_nil", func(t *testing.T) {
		var j JSONB
		err := j.Scan(nil)
		assert.NoError(t, err)
		assert.Nil(t, []byte(j))
	})

	t.Run("scan_unsupported_type", func(t *testing.T) {
		var j JSONB
		err := j.Scan(123)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "cannot scan")
	})

	t.Run("value_empty", func(t *testing.T) {
		var j JSONB
		val, err := j.Value()
		assert.NoError(t, err)
		assert.Nil(t, val)
	})

	t.Run("value_with_data", func(t *testing.T) {
		j := JSONB(`{"test": true}`)
		val, err := j.Value()
		assert.NoError(t, err)
		assert.Equal(t, `{"test": true}`, val)
	})

	t.Run("marshal_json", func(t *testing.T) {
		j := JSONB(`{"nested": {"value": 123}}`)
		data, err := j.MarshalJSON()
		assert.NoError(t, err)
		assert.JSONEq(t, `{"nested": {"value": 123}}`, string(data))
	})

	t.Run("unmarshal_json", func(t *testing.T) {
		var j JSONB
		err := j.UnmarshalJSON([]byte(`{"unmarshal": "test"}`))
		assert.NoError(t, err)
		assert.JSONEq(t, `{"unmarshal": "test"}`, string(j))
	})
}

// TestScanTargetValidationExtended tests scan target model validation with extended scenarios.
func TestScanTargetValidationExtended(t *testing.T) {
	t.Run("valid_scan_target", func(t *testing.T) {
		_, ipnet, err := net.ParseCIDR("192.168.1.0/24")
		require.NoError(t, err)

		description := "Test network"
		target := &ScanTarget{
			ID:                  uuid.New(),
			Name:                "Test Target",
			Network:             NetworkAddr{IPNet: *ipnet},
			Description:         &description,
			ScanIntervalSeconds: 3600,
			ScanPorts:           "22,80,443",
			ScanType:            "connect",
			Enabled:             true,
		}

		assert.NotEqual(t, uuid.Nil, target.ID)
		assert.NotEmpty(t, target.Name)
		assert.NotNil(t, target.Description)
		assert.True(t, target.ScanIntervalSeconds > 0)
		assert.NotEmpty(t, target.ScanPorts)
		assert.NotEmpty(t, target.ScanType)
	})

	t.Run("scan_target_with_nil_description", func(t *testing.T) {
		_, ipnet, err := net.ParseCIDR("10.0.0.0/8")
		require.NoError(t, err)

		target := &ScanTarget{
			ID:                  uuid.New(),
			Name:                "Minimal Target",
			Network:             NetworkAddr{IPNet: *ipnet},
			Description:         nil,
			ScanIntervalSeconds: 1800,
			ScanPorts:           "80",
			ScanType:            "syn",
			Enabled:             false,
		}

		assert.NotEqual(t, uuid.Nil, target.ID)
		assert.Nil(t, target.Description)
		assert.False(t, target.Enabled)
	})
}

// TestScanJobValidation tests scan job model validation.
func TestScanJobValidation(t *testing.T) {
	t.Run("valid_scan_job", func(t *testing.T) {
		targetID := uuid.New()
		startTime := time.Now()

		job := &ScanJob{
			ID:              uuid.New(),
			TargetID:        targetID,
			Status:          ScanJobStatusPending,
			StartedAt:       &startTime,
			CompletedAt:     nil,
			ErrorMessage:    nil,
			ScanStats:       JSONB(`{"ports_scanned": 1000, "hosts_found": 5}`),
			ProgressPercent: nil,
		}

		assert.NotEqual(t, uuid.Nil, job.ID)
		assert.Equal(t, targetID, job.TargetID)
		assert.Equal(t, ScanJobStatusPending, job.Status)
		assert.NotNil(t, job.StartedAt)
		assert.Nil(t, job.CompletedAt)
	})

	t.Run("completed_scan_job", func(t *testing.T) {
		startTime := time.Now()
		endTime := startTime.Add(30 * time.Minute)
		progress := 100
		errorMsg := "Scan completed with warnings"

		job := &ScanJob{
			ID:              uuid.New(),
			TargetID:        uuid.New(),
			Status:          ScanJobStatusCompleted,
			StartedAt:       &startTime,
			CompletedAt:     &endTime,
			ErrorMessage:    &errorMsg,
			ScanStats:       JSONB(`{"duration": 1800, "success": true}`),
			ProgressPercent: &progress,
		}

		assert.Equal(t, ScanJobStatusCompleted, job.Status)
		assert.NotNil(t, job.CompletedAt)
		assert.NotNil(t, job.ErrorMessage)
		assert.Equal(t, 100, *job.ProgressPercent)
		assert.True(t, job.CompletedAt.After(*job.StartedAt))
	})
}

// TestHostValidation tests host model validation.
func TestHostValidation(t *testing.T) {
	t.Run("valid_host", func(t *testing.T) {
		ip := IPAddr{IP: net.ParseIP("192.168.1.100")}
		mac, err := net.ParseMAC("00:11:22:33:44:55")
		require.NoError(t, err)

		hostname := "server.example.com"
		vendor := "Dell Inc."
		osFamily := "linux"
		osName := "Ubuntu"
		osVersion := "20.04"
		discoveryMethod := "ping"

		host := &Host{
			ID:              uuid.New(),
			IPAddress:       ip,
			Hostname:        &hostname,
			MACAddress:      &MACAddr{HardwareAddr: mac},
			Vendor:          &vendor,
			OSFamily:        &osFamily,
			OSName:          &osName,
			OSVersion:       &osVersion,
			Status:          HostStatusUp,
			DiscoveryMethod: &discoveryMethod,
			ResponseTimeMS:  func() *int { v := 15; return &v }(),
			DiscoveryCount:  3,
			FirstSeen:       time.Now().Add(-24 * time.Hour),
			LastSeen:        time.Now(),
		}

		assert.NotEqual(t, uuid.Nil, host.ID)
		assert.Equal(t, "192.168.1.100", host.IPAddress.String())
		assert.Equal(t, "server.example.com", *host.Hostname)
		assert.Equal(t, "00:11:22:33:44:55", host.MACAddress.String())
		assert.Equal(t, HostStatusUp, host.Status)
		assert.True(t, host.LastSeen.After(host.FirstSeen))
	})

	t.Run("minimal_host", func(t *testing.T) {
		ip := IPAddr{IP: net.ParseIP("10.1.1.1")}

		discoveryMethod := "tcp"

		host := &Host{
			ID:              uuid.New(),
			IPAddress:       ip,
			Hostname:        nil,
			MACAddress:      nil,
			Vendor:          nil,
			Status:          HostStatusUnknown,
			DiscoveryMethod: &discoveryMethod,
			DiscoveryCount:  1,
			FirstSeen:       time.Now(),
			LastSeen:        time.Now(),
		}

		assert.NotEqual(t, uuid.Nil, host.ID)
		assert.Nil(t, host.Hostname)
		assert.Nil(t, host.MACAddress)
		assert.Nil(t, host.Vendor)
		assert.Equal(t, HostStatusUnknown, host.Status)
	})
}

// TestConstants tests that all constants are properly defined.
func TestConstants(t *testing.T) {
	// Host status constants
	assert.Equal(t, "up", HostStatusUp)
	assert.Equal(t, "down", HostStatusDown)
	assert.Equal(t, "unknown", HostStatusUnknown)

	// Port state constants
	assert.Equal(t, "open", PortStateOpen)
	assert.Equal(t, "closed", PortStateClosed)
	assert.Equal(t, "filtered", PortStateFiltered)

	// Scan type constants
	assert.Equal(t, "connect", ScanTypeConnect)
	assert.Equal(t, "syn", ScanTypeSYN)
	assert.Equal(t, "version", ScanTypeVersion)

	// Protocol constants
	assert.Equal(t, "tcp", ProtocolTCP)
	assert.Equal(t, "udp", ProtocolUDP)

	// Scan job status constants
	assert.Equal(t, "pending", ScanJobStatusPending)
	assert.Equal(t, "running", ScanJobStatusRunning)
	assert.Equal(t, "completed", ScanJobStatusCompleted)
	assert.Equal(t, "failed", ScanJobStatusFailed)
}

// TestConfigValidation tests configuration validation scenarios.
func TestConfigValidation(t *testing.T) {
	t.Run("valid_config", func(t *testing.T) {
		cfg := &Config{
			Host:            "localhost",
			Port:            5432,
			Database:        "testdb",
			Username:        "testuser",
			Password:        "testpass",
			SSLMode:         "require",
			MaxOpenConns:    10,
			MaxIdleConns:    2,
			ConnMaxLifetime: 30 * time.Minute,
			ConnMaxIdleTime: 5 * time.Minute,
		}

		assert.True(t, cfg.Port > 0 && cfg.Port <= 65535)
		assert.NotEmpty(t, cfg.Host)
		assert.NotEmpty(t, cfg.Database)
		assert.NotEmpty(t, cfg.Username)
		assert.True(t, cfg.MaxOpenConns > 0)
		assert.True(t, cfg.MaxIdleConns >= 0)
		assert.True(t, cfg.ConnMaxLifetime > 0)
	})

	t.Run("ssl_mode_validation", func(t *testing.T) {
		validSSLModes := []string{"disable", "require", "verify-ca", "verify-full"}

		for _, mode := range validSSLModes {
			cfg := &Config{SSLMode: mode}
			assert.Contains(t, validSSLModes, cfg.SSLMode)
		}
	})
}

// TestDriverInterfaces tests that custom types implement driver interfaces.
func TestDriverInterfaces(t *testing.T) {
	t.Run("network_addr_implements_interfaces", func(t *testing.T) {
		var addr NetworkAddr

		// Test that it implements driver.Valuer
		_, ok := interface{}(&addr).(driver.Valuer)
		assert.True(t, ok, "NetworkAddr should implement driver.Valuer")

		// Test that it implements sql.Scanner
		var scanner interface{} = &addr
		_, ok = scanner.(interface{ Scan(interface{}) error })
		assert.True(t, ok, "NetworkAddr should implement sql.Scanner")
	})

	t.Run("ip_addr_implements_interfaces", func(t *testing.T) {
		var addr IPAddr

		_, ok := interface{}(&addr).(driver.Valuer)
		assert.True(t, ok, "IPAddr should implement driver.Valuer")

		var scanner interface{} = &addr
		_, ok = scanner.(interface{ Scan(interface{}) error })
		assert.True(t, ok, "IPAddr should implement sql.Scanner")
	})

	t.Run("mac_addr_implements_interfaces", func(t *testing.T) {
		var addr MACAddr

		_, ok := interface{}(&addr).(driver.Valuer)
		assert.True(t, ok, "MACAddr should implement driver.Valuer")

		var scanner interface{} = &addr
		_, ok = scanner.(interface{ Scan(interface{}) error })
		assert.True(t, ok, "MACAddr should implement sql.Scanner")
	})

	t.Run("jsonb_implements_interfaces", func(t *testing.T) {
		var j JSONB

		_, ok := interface{}(&j).(driver.Valuer)
		assert.True(t, ok, "JSONB should implement driver.Valuer")

		var scanner interface{} = &j
		_, ok = scanner.(interface{ Scan(interface{}) error })
		assert.True(t, ok, "JSONB should implement sql.Scanner")
	})
}
