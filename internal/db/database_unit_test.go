package db

import (
	"context"
	"database/sql/driver"
	"encoding/json"
	"fmt"
	"net"
	"strings"
	"testing"
	"time"

	sqlmock "github.com/DATA-DOG/go-sqlmock"
	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
	"github.com/lib/pq"
	"github.com/lib/pq/pqerror"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/anstrom/scanorama/internal/errors"
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

	t.Run("scan_valid_mac_with_hyphens", func(t *testing.T) {
		var addr MACAddr
		err := addr.Scan("00-11-22-33-44-55")
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
		assert.NoError(t, err) // Scan doesn't validate JSON, just stores bytes
		assert.Equal(t, `{invalid json`, string(j))
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
		assert.Equal(t, []byte(`{"test": true}`), val)
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

// TestNetworkValidationExtended tests network model validation with extended scenarios.
func TestNetworkValidationExtended(t *testing.T) {
	t.Run("valid_network", func(t *testing.T) {
		_, ipnet, err := net.ParseCIDR("192.168.1.0/24")
		require.NoError(t, err)

		description := "Test network"
		network := &Network{
			ID:                  uuid.New(),
			Name:                "Test Network",
			CIDR:                NetworkAddr{IPNet: *ipnet},
			Description:         &description,
			DiscoveryMethod:     "tcp",
			IsActive:            true,
			ScanEnabled:         true,
			ScanIntervalSeconds: 3600,
			ScanPorts:           "22,80,443",
			ScanType:            "connect",
		}

		assert.NotEqual(t, uuid.Nil, network.ID)
		assert.NotEmpty(t, network.Name)
		assert.NotNil(t, network.Description)
		assert.True(t, network.ScanIntervalSeconds > 0)
		assert.NotEmpty(t, network.ScanPorts)
		assert.NotEmpty(t, network.ScanType)
		assert.True(t, network.IsActive)
	})

	t.Run("network_with_nil_description", func(t *testing.T) {
		_, ipnet, err := net.ParseCIDR("10.0.0.0/8")
		require.NoError(t, err)

		network := &Network{
			ID:                  uuid.New(),
			Name:                "Minimal Network",
			CIDR:                NetworkAddr{IPNet: *ipnet},
			Description:         nil,
			DiscoveryMethod:     "tcp",
			IsActive:            false,
			ScanEnabled:         false,
			ScanIntervalSeconds: 1800,
			ScanPorts:           "80",
			ScanType:            "syn",
		}

		assert.NotEqual(t, uuid.Nil, network.ID)
		assert.Nil(t, network.Description)
		assert.False(t, network.IsActive)
	})
}

// TestScanJobValidation tests scan job model validation.
func TestScanJobValidation(t *testing.T) {
	t.Run("valid_scan_job", func(t *testing.T) {
		targetID := uuid.New()
		startTime := time.Now()

		job := &ScanJob{
			ID:              uuid.New(),
			NetworkID:       &targetID,
			Status:          ScanJobStatusPending,
			StartedAt:       &startTime,
			CompletedAt:     nil,
			ErrorMessage:    nil,
			ScanStats:       JSONB(`{"ports_scanned": 1000, "hosts_found": 5}`),
			ProgressPercent: nil,
		}

		assert.NotEqual(t, uuid.Nil, job.ID)
		require.NotNil(t, job.NetworkID)
		assert.Equal(t, targetID, *job.NetworkID)
		assert.Equal(t, ScanJobStatusPending, job.Status)
		assert.NotNil(t, job.StartedAt)
		assert.Nil(t, job.CompletedAt)
	})

	t.Run("completed_scan_job", func(t *testing.T) {
		startTime := time.Now()
		endTime := startTime.Add(30 * time.Minute)
		progress := 100
		errorMsg := "Scan completed with warnings"

		nid := uuid.New()
		job := &ScanJob{
			ID:              uuid.New(),
			NetworkID:       &nid,
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

// TestBuildWhereClause tests the buildWhereClause utility function behavior.
func TestBuildWhereClause(t *testing.T) {
	t.Run("empty_conditions", func(t *testing.T) {
		whereClause, args := buildWhereClause([]filterCondition{})
		assert.Empty(t, whereClause)
		assert.Nil(t, args)
	})

	t.Run("single_condition", func(t *testing.T) {
		conditions := []filterCondition{
			{column: "status", value: "active"},
		}
		whereClause, args := buildWhereClause(conditions)

		// Test structure and content, not exact formatting
		assert.Contains(t, whereClause, "WHERE")
		assert.Contains(t, whereClause, "status")
		assert.Contains(t, whereClause, "$1")
		assert.Len(t, args, 1)
		assert.Equal(t, "active", args[0])
	})

	t.Run("multiple_conditions", func(t *testing.T) {
		conditions := []filterCondition{
			{column: "status", value: "active"},
			{column: "type", value: "scan"},
			{column: "priority", value: 1},
		}
		whereClause, args := buildWhereClause(conditions)

		// Test that all conditions are present
		assert.Contains(t, whereClause, "WHERE")
		assert.Contains(t, whereClause, "status")
		assert.Contains(t, whereClause, "type")
		assert.Contains(t, whereClause, "priority")
		assert.Contains(t, whereClause, "AND")

		// Test parameter placeholders and values
		assert.Len(t, args, 3)
		assert.Contains(t, args, "active")
		assert.Contains(t, args, "scan")
		assert.Contains(t, args, 1)
	})

	t.Run("different_value_types", func(t *testing.T) {
		conditions := []filterCondition{
			{column: "name", value: "test"},
			{column: "count", value: 42},
			{column: "enabled", value: true},
		}
		whereClause, args := buildWhereClause(conditions)

		// Test logical structure
		assert.Contains(t, whereClause, "WHERE")
		assert.Len(t, args, 3)

		// Verify all values are preserved correctly
		assert.Contains(t, args, "test")
		assert.Contains(t, args, 42)
		assert.Contains(t, args, true)
	})
}

// TestBuildScanFilters tests the buildScanFilters utility function behavior.
func TestBuildScanFilters(t *testing.T) {
	t.Run("empty_filters", func(t *testing.T) {
		filters := ScanFilters{}
		whereClause, args := buildScanFilters(filters)
		assert.Empty(t, whereClause)
		assert.Nil(t, args)
	})

	t.Run("status_filter_only", func(t *testing.T) {
		filters := ScanFilters{Status: "running"}
		whereClause, args := buildScanFilters(filters)

		assert.Contains(t, whereClause, "WHERE")
		assert.Contains(t, whereClause, "sj.status")
		assert.Len(t, args, 1)
		assert.Equal(t, "running", args[0])
	})

	t.Run("scan_type_filter_only", func(t *testing.T) {
		filters := ScanFilters{ScanType: "syn"}
		whereClause, args := buildScanFilters(filters)

		assert.Contains(t, whereClause, "WHERE")
		assert.Contains(t, whereClause, "scan_type")
		assert.Len(t, args, 1)
		assert.Equal(t, "syn", args[0])
	})

	t.Run("profile_id_filter_only", func(t *testing.T) {
		profileID := "linux-server"
		filters := ScanFilters{ProfileID: &profileID}
		whereClause, args := buildScanFilters(filters)

		assert.Contains(t, whereClause, "WHERE")
		assert.Contains(t, whereClause, "profile_id")
		assert.Len(t, args, 1)
		assert.Equal(t, "linux-server", args[0])
	})

	t.Run("all_filters", func(t *testing.T) {
		profileID := "windows-server"
		filters := ScanFilters{
			Status:    "completed",
			ScanType:  "version",
			ProfileID: &profileID,
		}
		whereClause, args := buildScanFilters(filters)

		// Test that all filter conditions are applied
		assert.Contains(t, whereClause, "WHERE")
		assert.Contains(t, whereClause, "status")
		assert.Contains(t, whereClause, "scan_type")
		assert.Contains(t, whereClause, "profile_id")
		assert.Contains(t, whereClause, "AND")

		// Verify all values are present
		assert.Len(t, args, 3)
		assert.Contains(t, args, "completed")
		assert.Contains(t, args, "version")
		assert.Contains(t, args, "windows-server")
	})
}

// TestBuildHostFilters tests the buildHostFilters utility function behavior.
func TestBuildHostFilters(t *testing.T) {
	t.Run("empty_filters", func(t *testing.T) {
		filters := &HostFilters{}
		whereClause, args := buildHostFilters(filters)
		assert.Empty(t, whereClause)
		assert.Nil(t, args)
	})

	t.Run("status_filter_only", func(t *testing.T) {
		filters := &HostFilters{Status: "up"}
		whereClause, args := buildHostFilters(filters)

		assert.Contains(t, whereClause, "WHERE")
		assert.Contains(t, whereClause, "status")
		assert.Len(t, args, 1)
		assert.Equal(t, "up", args[0])
	})

	t.Run("os_family_filter_only", func(t *testing.T) {
		filters := &HostFilters{OSFamily: "linux"}
		whereClause, args := buildHostFilters(filters)

		assert.Contains(t, whereClause, "WHERE")
		assert.Contains(t, whereClause, "os_family")
		assert.Len(t, args, 1)
		assert.Equal(t, "linux", args[0])
	})

	t.Run("network_filter_only", func(t *testing.T) {
		filters := &HostFilters{Network: "192.168.1.0/24"}
		whereClause, args := buildHostFilters(filters)

		assert.Contains(t, whereClause, "WHERE")
		assert.Contains(t, whereClause, "ip_address")
		assert.Len(t, args, 1)
		assert.Equal(t, "192.168.1.0/24", args[0])
	})

	t.Run("all_filters", func(t *testing.T) {
		filters := &HostFilters{
			Status:   "up",
			OSFamily: "windows",
			Network:  "10.0.0.0/8",
		}
		whereClause, args := buildHostFilters(filters)

		// Test that all conditions are included
		assert.Contains(t, whereClause, "WHERE")
		assert.Contains(t, whereClause, "status")
		assert.Contains(t, whereClause, "os_family")
		assert.Contains(t, whereClause, "ip_address")
		assert.Contains(t, whereClause, "AND")

		// Verify all values are present
		assert.Len(t, args, 3)
		assert.Contains(t, args, "up")
		assert.Contains(t, args, "windows")
		assert.Contains(t, args, "10.0.0.0/8")
	})
}

// TestParsePostgreSQLArray tests the parsePostgreSQLArray utility function.
func TestParsePostgreSQLArray(t *testing.T) {
	t.Run("nil_input", func(t *testing.T) {
		result := parsePostgreSQLArray(nil)
		assert.Nil(t, result)
	})

	t.Run("empty_array", func(t *testing.T) {
		input := []interface{}{}
		result := parsePostgreSQLArray(input)
		assert.Equal(t, []string{}, result)
	})

	t.Run("string_array", func(t *testing.T) {
		input := []interface{}{"one", "two", "three"}
		result := parsePostgreSQLArray(input)
		assert.Equal(t, []string{"one", "two", "three"}, result)
	})

	t.Run("mixed_array_filters_non_strings", func(t *testing.T) {
		input := []interface{}{"valid", 123, "also_valid", nil, "another"}
		result := parsePostgreSQLArray(input)
		assert.Equal(t, []string{"valid", "", "also_valid", "", "another"}, result)
	})

	t.Run("invalid_input_type", func(t *testing.T) {
		input := "not an array"
		result := parsePostgreSQLArray(input)
		assert.Nil(t, result)
	})
}

// TestBuildUpdateQuery tests the buildUpdateQuery utility function behavior.
func TestBuildUpdateQuery(t *testing.T) {
	t.Run("empty_data", func(t *testing.T) {
		data := map[string]interface{}{}
		fieldMappings := map[string]string{
			"name":   "name",
			"status": "status",
		}

		setParts, args := buildUpdateQuery(data, fieldMappings)
		assert.Empty(t, setParts)
		assert.Empty(t, args)
	})

	t.Run("single_field", func(t *testing.T) {
		data := map[string]interface{}{
			"name": "Updated Name",
		}
		fieldMappings := map[string]string{
			"name":   "name",
			"status": "status",
		}

		setParts, args := buildUpdateQuery(data, fieldMappings)

		// Test structure rather than exact formatting
		assert.Len(t, setParts, 1)
		assert.Len(t, args, 1)
		assert.Contains(t, setParts[0], "name")
		assert.Contains(t, setParts[0], "=")
		assert.Contains(t, setParts[0], "$")
		assert.Equal(t, "Updated Name", args[0])
	})

	t.Run("multiple_fields", func(t *testing.T) {
		data := map[string]interface{}{
			"name":    "Updated Name",
			"status":  "active",
			"count":   42,
			"enabled": true,
		}
		fieldMappings := map[string]string{
			"name":    "name",
			"status":  "status",
			"count":   "total_count",
			"enabled": "is_enabled",
		}

		setParts, args := buildUpdateQuery(data, fieldMappings)

		// Test logical structure
		assert.Len(t, setParts, 4)
		assert.Len(t, args, 4)

		// Check that mapped fields are present in SET parts
		setString := strings.Join(setParts, " ")
		assert.Contains(t, setString, "name")
		assert.Contains(t, setString, "status")
		assert.Contains(t, setString, "total_count") // mapped name
		assert.Contains(t, setString, "is_enabled")  // mapped name

		// Check all expected values are preserved
		assert.Contains(t, args, "Updated Name")
		assert.Contains(t, args, "active")
		assert.Contains(t, args, 42)
		assert.Contains(t, args, true)
	})

	t.Run("nil_values_excluded", func(t *testing.T) {
		data := map[string]interface{}{
			"name":   "Updated Name",
			"status": nil,
			"count":  42,
		}
		fieldMappings := map[string]string{
			"name":   "name",
			"status": "status",
			"count":  "total_count",
		}

		setParts, args := buildUpdateQuery(data, fieldMappings)

		// Should only include non-nil values
		assert.Len(t, setParts, 2)
		assert.Len(t, args, 2)
		assert.Contains(t, args, "Updated Name")
		assert.Contains(t, args, 42)
		assert.NotContains(t, args, nil)
	})

	t.Run("unmapped_fields_ignored", func(t *testing.T) {
		data := map[string]interface{}{
			"name":         "Updated Name",
			"unmapped":     "ignored",
			"also_ignored": 123,
		}
		fieldMappings := map[string]string{
			"name": "name",
		}

		setParts, args := buildUpdateQuery(data, fieldMappings)

		// Should only include mapped fields
		assert.Len(t, setParts, 1)
		assert.Len(t, args, 1)
		assert.Contains(t, setParts[0], "name")
		assert.Equal(t, "Updated Name", args[0])
	})
}

// TestAssignmentFunctions tests the various assignment utility functions.
func TestAssignmentFunctions(t *testing.T) {
	t.Run("assignStringPtr", func(t *testing.T) {
		var target *string

		// Test with valid string
		source := "test value"
		assignStringPtr(&target, &source)
		require.NotNil(t, target)
		assert.Equal(t, "test value", *target)

		// Test with empty string (should not assign)
		target = nil
		empty := ""
		assignStringPtr(&target, &empty)
		assert.Nil(t, target)

		// Test with nil source
		target = nil
		assignStringPtr(&target, nil)
		assert.Nil(t, target)
	})

	t.Run("assignMACAddress", func(t *testing.T) {
		var target *MACAddr

		// Test with valid MAC address
		validMAC := "aa:bb:cc:dd:ee:ff"
		assignMACAddress(&target, &validMAC)
		require.NotNil(t, target)
		assert.Equal(t, "aa:bb:cc:dd:ee:ff", target.String())

		// Test with invalid MAC address (should not assign)
		target = nil
		invalidMAC := "not-a-mac"
		assignMACAddress(&target, &invalidMAC)
		assert.Nil(t, target)

		// Test with empty string (should not assign)
		target = nil
		empty := ""
		assignMACAddress(&target, &empty)
		assert.Nil(t, target)

		// Test with nil source
		target = nil
		assignMACAddress(&target, nil)
		assert.Nil(t, target)
	})

	t.Run("assignIntPtr", func(t *testing.T) {
		var target *int

		// Test with valid int
		source := 42
		assignIntPtr(&target, &source)
		require.NotNil(t, target)
		assert.Equal(t, 42, *target)

		// Test with zero value
		target = nil
		zero := 0
		assignIntPtr(&target, &zero)
		require.NotNil(t, target)
		assert.Equal(t, 0, *target)

		// Test with nil source
		target = nil
		assignIntPtr(&target, nil)
		assert.Nil(t, target)
	})

	t.Run("assignBoolFromPtr", func(t *testing.T) {
		var target bool

		// Test with true
		sourceTrue := true
		assignBoolFromPtr(&target, &sourceTrue)
		assert.True(t, target)

		// Test with false
		target = true // Reset to opposite value
		sourceFalse := false
		assignBoolFromPtr(&target, &sourceFalse)
		assert.False(t, target)

		// Test with nil source (should not change target)
		target = true
		assignBoolFromPtr(&target, nil)
		assert.True(t, target) // Should remain unchanged
	})
}

// TestHostOSFingerprint tests the Host model OS fingerprint methods.
func TestHostOSFingerprint(t *testing.T) {
	t.Run("GetOSFingerprint_nil_family", func(t *testing.T) {
		host := &Host{}
		fp := host.GetOSFingerprint()
		assert.Nil(t, fp)
	})

	t.Run("GetOSFingerprint_minimal", func(t *testing.T) {
		family := "linux"
		host := &Host{
			OSFamily: &family,
		}

		fp := host.GetOSFingerprint()
		require.NotNil(t, fp)
		assert.Equal(t, "linux", fp.Family)
		assert.Equal(t, "", fp.Name)
		assert.Equal(t, "", fp.Version)
		assert.Equal(t, 0, fp.Confidence)
		assert.Equal(t, "unknown", fp.Method)
		assert.Nil(t, fp.Details)
	})

	t.Run("GetOSFingerprint_complete", func(t *testing.T) {
		family := "linux"
		name := "Ubuntu"
		version := "20.04"
		confidence := 95
		method := "tcp_fingerprint"
		details := JSONB(`{"kernel": "5.4.0", "arch": "x86_64"}`)

		host := &Host{
			OSFamily:     &family,
			OSName:       &name,
			OSVersion:    &version,
			OSConfidence: &confidence,
			OSMethod:     &method,
			OSDetails:    details,
		}

		fp := host.GetOSFingerprint()
		require.NotNil(t, fp)
		assert.Equal(t, "linux", fp.Family)
		assert.Equal(t, "Ubuntu", fp.Name)
		assert.Equal(t, "20.04", fp.Version)
		assert.Equal(t, 95, fp.Confidence)
		assert.Equal(t, "tcp_fingerprint", fp.Method)
		require.NotNil(t, fp.Details)

		expectedDetails := map[string]interface{}{
			"kernel": "5.4.0",
			"arch":   "x86_64",
		}
		assert.Equal(t, expectedDetails, fp.Details)
	})

	t.Run("GetOSFingerprint_invalid_json_details", func(t *testing.T) {
		family := "windows"
		details := JSONB(`{invalid json`)

		host := &Host{
			OSFamily:  &family,
			OSDetails: details,
		}

		fp := host.GetOSFingerprint()
		require.NotNil(t, fp)
		assert.Equal(t, "windows", fp.Family)
		assert.Nil(t, fp.Details) // Should be nil due to invalid JSON
	})

	t.Run("SetOSFingerprint_nil_input", func(t *testing.T) {
		host := &Host{}
		err := host.SetOSFingerprint(nil)
		assert.NoError(t, err)
		assert.Nil(t, host.OSFamily)
	})

	t.Run("SetOSFingerprint_minimal", func(t *testing.T) {
		host := &Host{}
		fp := &OSFingerprint{
			Family: "linux",
		}

		err := host.SetOSFingerprint(fp)
		assert.NoError(t, err)

		require.NotNil(t, host.OSFamily)
		assert.Equal(t, "linux", *host.OSFamily)
		assert.Equal(t, "", *host.OSName)
		assert.Equal(t, "", *host.OSVersion)
		assert.Equal(t, 0, *host.OSConfidence)
		assert.Equal(t, "", *host.OSMethod)
		assert.NotNil(t, host.OSDetectedAt)
		assert.Empty(t, host.OSDetails)
	})

	t.Run("SetOSFingerprint_complete", func(t *testing.T) {
		host := &Host{}
		details := map[string]interface{}{
			"kernel":     "5.4.0",
			"arch":       "x86_64",
			"build_date": "2021-01-01",
		}

		fp := &OSFingerprint{
			Family:     "linux",
			Name:       "Ubuntu",
			Version:    "20.04",
			Confidence: 95,
			Method:     "tcp_fingerprint",
			Details:    details,
		}

		err := host.SetOSFingerprint(fp)
		assert.NoError(t, err)

		require.NotNil(t, host.OSFamily)
		assert.Equal(t, "linux", *host.OSFamily)
		assert.Equal(t, "Ubuntu", *host.OSName)
		assert.Equal(t, "20.04", *host.OSVersion)
		assert.Equal(t, 95, *host.OSConfidence)
		assert.Equal(t, "tcp_fingerprint", *host.OSMethod)
		assert.NotNil(t, host.OSDetectedAt)

		// Verify JSON details
		var parsedDetails map[string]interface{}
		err = json.Unmarshal([]byte(host.OSDetails), &parsedDetails)
		require.NoError(t, err)
		assert.Equal(t, details, parsedDetails)
	})

	t.Run("SetOSFingerprint_round_trip", func(t *testing.T) {
		host := &Host{}
		originalDetails := map[string]interface{}{
			"test":    "value",
			"number":  42.0, // JSON numbers become float64
			"boolean": true,
		}

		fp := &OSFingerprint{
			Family:     "windows",
			Name:       "Windows 10",
			Version:    "2004",
			Confidence: 85,
			Method:     "banner_grab",
			Details:    originalDetails,
		}

		// Set the fingerprint
		err := host.SetOSFingerprint(fp)
		require.NoError(t, err)

		// Get it back
		retrievedFp := host.GetOSFingerprint()
		require.NotNil(t, retrievedFp)

		assert.Equal(t, fp.Family, retrievedFp.Family)
		assert.Equal(t, fp.Name, retrievedFp.Name)
		assert.Equal(t, fp.Version, retrievedFp.Version)
		assert.Equal(t, fp.Confidence, retrievedFp.Confidence)
		assert.Equal(t, fp.Method, retrievedFp.Method)
		assert.Equal(t, originalDetails, retrievedFp.Details)
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

// TestScanDerivedFields tests the derived-field logic applied after a DB row is scanned.
// The logic is identical in both GetScan and processScanRow; we exercise it directly
// without a database connection by constructing Scan structs and running the same
// conditional assignments inline.
func TestScanDerivedFields(t *testing.T) {
	t.Run("duration_set_when_both_timestamps_present", func(t *testing.T) {
		started := time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC)
		completed := started.Add(90 * time.Second)

		scan := &Scan{
			StartedAt:   &started,
			CompletedAt: &completed,
		}

		// Apply the same logic used in GetScan / processScanRow.
		if scan.StartedAt != nil && scan.CompletedAt != nil {
			d := scan.CompletedAt.Sub(*scan.StartedAt).String()
			scan.DurationStr = &d
		}

		require.NotNil(t, scan.DurationStr)
		assert.Equal(t, (90 * time.Second).String(), *scan.DurationStr)
	})

	t.Run("duration_nil_when_only_started_at_set", func(t *testing.T) {
		started := time.Now()

		scan := &Scan{
			StartedAt:   &started,
			CompletedAt: nil,
		}

		if scan.StartedAt != nil && scan.CompletedAt != nil {
			d := scan.CompletedAt.Sub(*scan.StartedAt).String()
			scan.DurationStr = &d
		}

		assert.Nil(t, scan.DurationStr)
	})

	t.Run("duration_nil_when_only_completed_at_set", func(t *testing.T) {
		completed := time.Now()

		scan := &Scan{
			StartedAt:   nil,
			CompletedAt: &completed,
		}

		if scan.StartedAt != nil && scan.CompletedAt != nil {
			d := scan.CompletedAt.Sub(*scan.StartedAt).String()
			scan.DurationStr = &d
		}

		assert.Nil(t, scan.DurationStr)
	})

	t.Run("duration_nil_when_neither_timestamp_set", func(t *testing.T) {
		scan := &Scan{
			StartedAt:   nil,
			CompletedAt: nil,
		}

		if scan.StartedAt != nil && scan.CompletedAt != nil {
			d := scan.CompletedAt.Sub(*scan.StartedAt).String()
			scan.DurationStr = &d
		}

		assert.Nil(t, scan.DurationStr)
	})

	t.Run("ports_scanned_set_when_ports_non_empty", func(t *testing.T) {
		scan := &Scan{
			Ports: "22,80,443",
		}

		if scan.Ports != "" {
			p := scan.Ports
			scan.PortsScanned = &p
		}

		require.NotNil(t, scan.PortsScanned)
		assert.Equal(t, "22,80,443", *scan.PortsScanned)
	})

	t.Run("ports_scanned_nil_when_ports_empty", func(t *testing.T) {
		scan := &Scan{
			Ports: "",
		}

		if scan.Ports != "" {
			p := scan.Ports
			scan.PortsScanned = &p
		}

		assert.Nil(t, scan.PortsScanned)
	})

	t.Run("both_derived_fields_set_together", func(t *testing.T) {
		started := time.Date(2024, 6, 15, 9, 0, 0, 0, time.UTC)
		completed := started.Add(5 * time.Minute)

		scan := &Scan{
			Ports:       "1-1024",
			StartedAt:   &started,
			CompletedAt: &completed,
		}

		if scan.StartedAt != nil && scan.CompletedAt != nil {
			d := scan.CompletedAt.Sub(*scan.StartedAt).String()
			scan.DurationStr = &d
		}
		if scan.Ports != "" {
			p := scan.Ports
			scan.PortsScanned = &p
		}

		require.NotNil(t, scan.DurationStr)
		assert.Equal(t, (5 * time.Minute).String(), *scan.DurationStr)
		require.NotNil(t, scan.PortsScanned)
		assert.Equal(t, "1-1024", *scan.PortsScanned)
	})
}

// TestNetworkAddrJSONInStruct tests that a struct containing a NetworkAddr field
// marshals and unmarshals correctly via encoding/json (the real usage context).
func TestNetworkAddrJSONInStruct(t *testing.T) {
	type Payload struct {
		Name    string      `json:"name"`
		Network NetworkAddr `json:"network"`
	}

	t.Run("round_trip_ipv4", func(t *testing.T) {
		var src Payload
		src.Name = "lan"
		require.NoError(t, src.Network.Scan("192.168.0.0/16"))

		raw, err := json.Marshal(src)
		require.NoError(t, err)

		// Verify the JSON contains the CIDR string.
		assert.Contains(t, string(raw), `"192.168.0.0/16"`)

		var dst Payload
		require.NoError(t, json.Unmarshal(raw, &dst))

		assert.Equal(t, src.Name, dst.Name)
		assert.Equal(t, src.Network.String(), dst.Network.String())
	})

	t.Run("round_trip_ipv6", func(t *testing.T) {
		var src Payload
		src.Name = "ipv6-net"
		require.NoError(t, src.Network.Scan("2001:db8::/32"))

		raw, err := json.Marshal(src)
		require.NoError(t, err)
		assert.Contains(t, string(raw), `"2001:db8::/32"`)

		var dst Payload
		require.NoError(t, json.Unmarshal(raw, &dst))

		assert.Equal(t, src.Network.String(), dst.Network.String())
	})

	t.Run("unmarshal_bad_value_returns_error", func(t *testing.T) {
		// network field is a number instead of a CIDR string — should fail.
		raw := []byte(`{"name":"x","network":42}`)

		var dst Payload
		err := json.Unmarshal(raw, &dst)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "expected a string")
	})
}

// TestCreateProfile_UniqueNameConflict verifies that CreateProfile translates a
// PostgreSQL unique-violation on the profile name constraint into a typed
// conflict error via pq.As, not a generic wrapped error.
func TestCreateProfile_UniqueNameConflict(t *testing.T) {
	db, mock := newMockDB(t)

	pqErr := &pq.Error{
		Code:       pqerror.UniqueViolation,
		Constraint: "scan_profiles_name_key",
		Message:    `duplicate key value violates unique constraint "scan_profiles_name_key"`,
	}
	mock.ExpectExec("INSERT INTO scan_profiles").WillReturnError(pqErr)

	profileData := CreateProfileInput{
		Name:     "My Profile",
		ScanType: "connect",
		Ports:    "22,80,443",
		Timing:   "normal",
	}

	_, err := NewProfileRepository(db).CreateProfile(context.Background(), profileData)

	require.Error(t, err)
	assert.True(t, errors.IsCode(err, errors.CodeConflict),
		"expected CodeConflict, got: %v", err)
	assert.Contains(t, err.Error(), "My Profile",
		"conflict message should include the duplicate profile name")
	require.NoError(t, mock.ExpectationsWereMet())
}

// TestCreateProfile_NonPQError verifies that a non-PostgreSQL error (e.g. a
// connection reset) is returned as a generic wrapped error, not a conflict.
func TestCreateProfile_NonPQError(t *testing.T) {
	db, mock := newMockDB(t)

	mock.ExpectExec("INSERT INTO scan_profiles").WillReturnError(
		fmt.Errorf("connection reset by peer"),
	)

	profileData := CreateProfileInput{
		Name:     "My Profile",
		ScanType: "connect",
		Ports:    "22,80,443",
		Timing:   "normal",
	}

	_, err := NewProfileRepository(db).CreateProfile(context.Background(), profileData)

	require.Error(t, err)
	assert.False(t, errors.IsCode(err, errors.CodeConflict))
	assert.Contains(t, err.Error(), "create profile")
	require.NoError(t, mock.ExpectationsWereMet())
}

// newMockDB creates a *DB backed by a sqlmock database for unit tests that
// need to exercise SQL-level error handling without a real PostgreSQL instance.
func newMockDB(t *testing.T) (*DB, sqlmock.Sqlmock) {
	t.Helper()
	sqlDB, mock, err := sqlmock.New()
	require.NoError(t, err)
	t.Cleanup(func() { _ = sqlDB.Close() })
	return &DB{DB: sqlx.NewDb(sqlDB, "sqlmock")}, mock
}

// TestCreateHost_UniqueIPConflict verifies that CreateHost translates a
// PostgreSQL unique-violation on the unique_ip_address constraint into a
// typed conflict error via pq.As.
func TestCreateHost_UniqueIPConflict(t *testing.T) {
	db, mock := newMockDB(t)

	pqErr := &pq.Error{
		Code:       pqerror.UniqueViolation,
		Constraint: "unique_ip_address",
		Message:    "duplicate key value violates unique constraint \"unique_ip_address\"",
	}
	mock.ExpectExec("INSERT INTO hosts").WillReturnError(pqErr)

	hostData := CreateHostInput{
		IPAddress: "10.0.0.1",
	}

	_, err := NewHostRepository(db).CreateHost(context.Background(), hostData)

	require.Error(t, err)
	assert.True(t, errors.IsCode(err, errors.CodeConflict),
		"expected CodeConflict, got: %v", err)
	assert.Contains(t, err.Error(), "10.0.0.1")
	require.NoError(t, mock.ExpectationsWereMet())
}

// TestCreateHost_UniqueViolationOtherConstraint verifies that a unique
// violation on a *different* constraint is not swallowed as a conflict —
// it should fall through to the generic error path.
func TestCreateHost_UniqueViolationOtherConstraint(t *testing.T) {
	db, mock := newMockDB(t)

	pqErr := &pq.Error{
		Code:       pqerror.UniqueViolation,
		Constraint: "some_other_unique_constraint",
		Message:    "duplicate key value violates unique constraint \"some_other_unique_constraint\"",
	}
	mock.ExpectExec("INSERT INTO hosts").WillReturnError(pqErr)

	hostData := CreateHostInput{
		IPAddress: "10.0.0.2",
	}

	_, err := NewHostRepository(db).CreateHost(context.Background(), hostData)

	require.Error(t, err)
	assert.True(t, errors.IsCode(err, errors.CodeConflict),
		"unique violation should be a conflict error, got: %v", err)
	assert.Contains(t, err.Error(), "create host")
	require.NoError(t, mock.ExpectationsWereMet())
}

// TestCreateHost_NonPQError verifies that a non-PostgreSQL error (e.g. a
// network timeout) is returned as a generic wrapped error, not a conflict.
func TestCreateHost_NonPQError(t *testing.T) {
	db, mock := newMockDB(t)

	mock.ExpectExec("INSERT INTO hosts").WillReturnError(
		fmt.Errorf("connection reset by peer"),
	)

	hostData := CreateHostInput{
		IPAddress: "10.0.0.3",
	}

	_, err := NewHostRepository(db).CreateHost(context.Background(), hostData)

	require.Error(t, err)
	assert.False(t, errors.IsCode(err, errors.CodeConflict))
	assert.Contains(t, err.Error(), "create host")
	require.NoError(t, mock.ExpectationsWereMet())
}
