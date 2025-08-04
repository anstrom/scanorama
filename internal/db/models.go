package db

import (
	"database/sql/driver"
	"encoding/json"
	"fmt"
	"net"
	"time"

	"github.com/google/uuid"
)

// NetworkAddr wraps net.IPNet to implement PostgreSQL CIDR type
type NetworkAddr struct {
	net.IPNet
}

// Scan implements sql.Scanner for PostgreSQL CIDR type
func (n *NetworkAddr) Scan(value interface{}) error {
	if value == nil {
		return nil
	}

	switch v := value.(type) {
	case string:
		_, ipnet, err := net.ParseCIDR(v)
		if err != nil {
			return fmt.Errorf("failed to parse CIDR: %w", err)
		}
		n.IPNet = *ipnet
		return nil
	case []byte:
		_, ipnet, err := net.ParseCIDR(string(v))
		if err != nil {
			return fmt.Errorf("failed to parse CIDR: %w", err)
		}
		n.IPNet = *ipnet
		return nil
	default:
		return fmt.Errorf("cannot scan %T into NetworkAddr", value)
	}
}

// Value implements driver.Valuer for PostgreSQL CIDR type
func (n NetworkAddr) Value() (driver.Value, error) {
	if len(n.IP) == 0 {
		return nil, nil
	}
	return n.IPNet.String(), nil
}

// String returns the CIDR notation string
func (n NetworkAddr) String() string {
	return n.IPNet.String()
}

// IPAddr wraps net.IP to implement PostgreSQL INET type
type IPAddr struct {
	net.IP
}

// Scan implements sql.Scanner for PostgreSQL INET type
func (ip *IPAddr) Scan(value interface{}) error {
	if value == nil {
		return nil
	}

	switch v := value.(type) {
	case string:
		parsed := net.ParseIP(v)
		if parsed == nil {
			return fmt.Errorf("failed to parse IP address: %s", v)
		}
		ip.IP = parsed
		return nil
	case []byte:
		parsed := net.ParseIP(string(v))
		if parsed == nil {
			return fmt.Errorf("failed to parse IP address: %s", string(v))
		}
		ip.IP = parsed
		return nil
	default:
		return fmt.Errorf("cannot scan %T into IPAddr", value)
	}
}

// Value implements driver.Valuer for PostgreSQL INET type
func (ip IPAddr) Value() (driver.Value, error) {
	if ip.IP == nil {
		return nil, nil
	}
	return ip.IP.String(), nil
}

// String returns the IP address string
func (ip IPAddr) String() string {
	if ip.IP == nil {
		return ""
	}
	return ip.IP.String()
}

// MACAddr wraps net.HardwareAddr to implement PostgreSQL MACADDR type
type MACAddr struct {
	net.HardwareAddr
}

// Scan implements sql.Scanner for PostgreSQL MACADDR type
func (mac *MACAddr) Scan(value interface{}) error {
	if value == nil {
		return nil
	}

	switch v := value.(type) {
	case string:
		hw, err := net.ParseMAC(v)
		if err != nil {
			return fmt.Errorf("failed to parse MAC address: %w", err)
		}
		mac.HardwareAddr = hw
		return nil
	case []byte:
		hw, err := net.ParseMAC(string(v))
		if err != nil {
			return fmt.Errorf("failed to parse MAC address: %w", err)
		}
		mac.HardwareAddr = hw
		return nil
	default:
		return fmt.Errorf("cannot scan %T into MACAddr", value)
	}
}

// Value implements driver.Valuer for PostgreSQL MACADDR type
func (mac MACAddr) Value() (driver.Value, error) {
	if mac.HardwareAddr == nil {
		return nil, nil
	}
	return mac.HardwareAddr.String(), nil
}

// String returns the MAC address string
func (mac MACAddr) String() string {
	if mac.HardwareAddr == nil {
		return ""
	}
	return mac.HardwareAddr.String()
}

// JSONB wraps json.RawMessage for PostgreSQL JSONB type
type JSONB json.RawMessage

// Scan implements sql.Scanner for PostgreSQL JSONB type
func (j *JSONB) Scan(value interface{}) error {
	if value == nil {
		*j = nil
		return nil
	}

	switch v := value.(type) {
	case []byte:
		*j = JSONB(v)
		return nil
	case string:
		*j = JSONB([]byte(v))
		return nil
	default:
		return fmt.Errorf("cannot scan %T into JSONB", value)
	}
}

// Value implements driver.Valuer for PostgreSQL JSONB type
func (j JSONB) Value() (driver.Value, error) {
	if j == nil {
		return nil, nil
	}
	return []byte(j), nil
}

// String returns the JSON string
func (j JSONB) String() string {
	return string(j)
}

// MarshalJSON implements json.Marshaler
func (j JSONB) MarshalJSON() ([]byte, error) {
	if j == nil {
		return []byte("null"), nil
	}
	return []byte(j), nil
}

// UnmarshalJSON implements json.Unmarshaler
func (j *JSONB) UnmarshalJSON(data []byte) error {
	*j = JSONB(data)
	return nil
}

// ScanTarget represents a network target to scan
type ScanTarget struct {
	ID                  uuid.UUID   `db:"id" json:"id"`
	Name                string      `db:"name" json:"name"`
	Network             NetworkAddr `db:"network" json:"network"`
	Description         *string     `db:"description" json:"description,omitempty"`
	ScanIntervalSeconds int         `db:"scan_interval_seconds" json:"scan_interval_seconds"`
	ScanPorts           string      `db:"scan_ports" json:"scan_ports"`
	ScanType            string      `db:"scan_type" json:"scan_type"`
	Enabled             bool        `db:"enabled" json:"enabled"`
	CreatedAt           time.Time   `db:"created_at" json:"created_at"`
	UpdatedAt           time.Time   `db:"updated_at" json:"updated_at"`
}

// ScanJob represents a scan job execution
type ScanJob struct {
	ID           uuid.UUID  `db:"id" json:"id"`
	TargetID     uuid.UUID  `db:"target_id" json:"target_id"`
	Status       string     `db:"status" json:"status"`
	StartedAt    *time.Time `db:"started_at" json:"started_at,omitempty"`
	CompletedAt  *time.Time `db:"completed_at" json:"completed_at,omitempty"`
	ErrorMessage *string    `db:"error_message" json:"error_message,omitempty"`
	ScanStats    JSONB      `db:"scan_stats" json:"scan_stats,omitempty"`
	CreatedAt    time.Time  `db:"created_at" json:"created_at"`
}

// Host represents a discovered host
type Host struct {
	ID         uuid.UUID `db:"id" json:"id"`
	IPAddress  IPAddr    `db:"ip_address" json:"ip_address"`
	Hostname   *string   `db:"hostname" json:"hostname,omitempty"`
	MACAddress *MACAddr  `db:"mac_address" json:"mac_address,omitempty"`
	Vendor     *string   `db:"vendor" json:"vendor,omitempty"`
	OSFamily   *string   `db:"os_family" json:"os_family,omitempty"`
	OSVersion  *string   `db:"os_version" json:"os_version,omitempty"`
	FirstSeen  time.Time `db:"first_seen" json:"first_seen"`
	LastSeen   time.Time `db:"last_seen" json:"last_seen"`
	Status     string    `db:"status" json:"status"`
}

// PortScan represents a port scan result
type PortScan struct {
	ID             uuid.UUID `db:"id" json:"id"`
	JobID          uuid.UUID `db:"job_id" json:"job_id"`
	HostID         uuid.UUID `db:"host_id" json:"host_id"`
	Port           int       `db:"port" json:"port"`
	Protocol       string    `db:"protocol" json:"protocol"`
	State          string    `db:"state" json:"state"`
	ServiceName    *string   `db:"service_name" json:"service_name,omitempty"`
	ServiceVersion *string   `db:"service_version" json:"service_version,omitempty"`
	ServiceProduct *string   `db:"service_product" json:"service_product,omitempty"`
	Banner         *string   `db:"banner" json:"banner,omitempty"`
	ScannedAt      time.Time `db:"scanned_at" json:"scanned_at"`
}

// Service represents detailed service detection
type Service struct {
	ID          uuid.UUID `db:"id" json:"id"`
	PortScanID  uuid.UUID `db:"port_scan_id" json:"port_scan_id"`
	ServiceType *string   `db:"service_type" json:"service_type,omitempty"`
	Version     *string   `db:"version" json:"version,omitempty"`
	CPE         *string   `db:"cpe" json:"cpe,omitempty"`
	Confidence  *int      `db:"confidence" json:"confidence,omitempty"`
	Details     JSONB     `db:"details" json:"details,omitempty"`
	DetectedAt  time.Time `db:"detected_at" json:"detected_at"`
}

// HostHistory represents changes to hosts over time
type HostHistory struct {
	ID        uuid.UUID `db:"id" json:"id"`
	HostID    uuid.UUID `db:"host_id" json:"host_id"`
	JobID     uuid.UUID `db:"job_id" json:"job_id"`
	EventType string    `db:"event_type" json:"event_type"`
	OldValue  JSONB     `db:"old_value" json:"old_value,omitempty"`
	NewValue  JSONB     `db:"new_value" json:"new_value,omitempty"`
	CreatedAt time.Time `db:"created_at" json:"created_at"`
}

// ActiveHost represents the active_hosts view
type ActiveHost struct {
	IPAddress         IPAddr    `db:"ip_address" json:"ip_address"`
	Hostname          *string   `db:"hostname" json:"hostname,omitempty"`
	MACAddress        *MACAddr  `db:"mac_address" json:"mac_address,omitempty"`
	Vendor            *string   `db:"vendor" json:"vendor,omitempty"`
	Status            string    `db:"status" json:"status"`
	LastSeen          time.Time `db:"last_seen" json:"last_seen"`
	OpenPorts         int       `db:"open_ports" json:"open_ports"`
	TotalPortsScanned int       `db:"total_ports_scanned" json:"total_ports_scanned"`
}

// NetworkSummary represents the network_summary view
type NetworkSummary struct {
	TargetName  string      `db:"target_name" json:"target_name"`
	Network     NetworkAddr `db:"network" json:"network"`
	ActiveHosts int         `db:"active_hosts" json:"active_hosts"`
	TotalHosts  int         `db:"total_hosts" json:"total_hosts"`
	OpenPorts   int         `db:"open_ports" json:"open_ports"`
	LastScan    *time.Time  `db:"last_scan" json:"last_scan,omitempty"`
}

// ScanJobStatus constants
const (
	ScanJobStatusPending   = "pending"
	ScanJobStatusRunning   = "running"
	ScanJobStatusCompleted = "completed"
	ScanJobStatusFailed    = "failed"
)

// HostStatus constants
const (
	HostStatusUp      = "up"
	HostStatusDown    = "down"
	HostStatusUnknown = "unknown"
)

// PortState constants
const (
	PortStateOpen     = "open"
	PortStateClosed   = "closed"
	PortStateFiltered = "filtered"
	PortStateUnknown  = "unknown"
)

// ScanType constants
const (
	ScanTypeConnect = "connect"
	ScanTypeSYN     = "syn"
	ScanTypeVersion = "version"
)

// Protocol constants
const (
	ProtocolTCP = "tcp"
	ProtocolUDP = "udp"
)

// HostHistoryEvent constants
const (
	HostEventDiscovered   = "discovered"
	HostEventStatusChange = "status_change"
	HostEventPortsChanged = "ports_changed"
	HostEventServiceFound = "service_found"
)
