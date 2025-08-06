package db

import (
	"database/sql/driver"
	"encoding/json"
	"fmt"
	"net"
	"time"

	"github.com/google/uuid"
	"github.com/lib/pq"
)

// NetworkAddr wraps net.IPNet to implement PostgreSQL CIDR type.
type NetworkAddr struct {
	net.IPNet
}

// Scan implements sql.Scanner for PostgreSQL CIDR type.
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

// Value implements driver.Valuer for PostgreSQL CIDR type.
func (n NetworkAddr) Value() (driver.Value, error) {
	if len(n.IP) == 0 {
		return nil, nil
	}
	return n.IPNet.String(), nil
}

// String returns the CIDR notation string.
func (n NetworkAddr) String() string {
	return n.IPNet.String()
}

// IPAddr wraps net.IP to implement PostgreSQL INET type.
type IPAddr struct {
	net.IP
}

// Scan implements sql.Scanner for PostgreSQL INET type.
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

// Value implements driver.Valuer for PostgreSQL INET type.
func (ip IPAddr) Value() (driver.Value, error) {
	if ip.IP == nil {
		return nil, nil
	}
	return ip.IP.String(), nil
}

// String returns the IP address string.
func (ip IPAddr) String() string {
	if ip.IP == nil {
		return ""
	}
	return ip.IP.String()
}

// MACAddr wraps net.HardwareAddr to implement PostgreSQL MACADDR type.
type MACAddr struct {
	net.HardwareAddr
}

// Scan implements sql.Scanner for PostgreSQL MACADDR type.
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

// Value implements driver.Valuer for PostgreSQL MACADDR type.
func (mac MACAddr) Value() (driver.Value, error) {
	if mac.HardwareAddr == nil {
		return nil, nil
	}
	return mac.HardwareAddr.String(), nil
}

// String returns the MAC address string.
func (mac MACAddr) String() string {
	if mac.HardwareAddr == nil {
		return ""
	}
	return mac.HardwareAddr.String()
}

// JSONB wraps json.RawMessage for PostgreSQL JSONB type.
type JSONB json.RawMessage

// Scan implements sql.Scanner for PostgreSQL JSONB type.
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

// Value implements driver.Valuer for PostgreSQL JSONB type.
func (j JSONB) Value() (driver.Value, error) {
	if j == nil {
		return nil, nil
	}
	return []byte(j), nil
}

// String returns the JSON string.
func (j JSONB) String() string {
	return string(j)
}

// MarshalJSON implements json.Marshaler.
func (j JSONB) MarshalJSON() ([]byte, error) {
	if j == nil {
		return []byte("null"), nil
	}
	return []byte(j), nil
}

// UnmarshalJSON implements json.Unmarshaler.
func (j *JSONB) UnmarshalJSON(data []byte) error {
	*j = JSONB(data)
	return nil
}

// ScanTarget represents a network target to scan.
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

// DiscoveryJob represents a network discovery job.
type DiscoveryJob struct {
	ID              uuid.UUID   `db:"id" json:"id"`
	Network         NetworkAddr `db:"network" json:"network"`
	Method          string      `db:"method" json:"method"`
	StartedAt       *time.Time  `db:"started_at" json:"started_at,omitempty"`
	CompletedAt     *time.Time  `db:"completed_at" json:"completed_at,omitempty"`
	HostsDiscovered int         `db:"hosts_discovered" json:"hosts_discovered"`
	HostsResponsive int         `db:"hosts_responsive" json:"hosts_responsive"`
	Status          string      `db:"status" json:"status"`
	CreatedAt       time.Time   `db:"created_at" json:"created_at"`
}

// ScanProfile represents a scanning profile configuration.
type ScanProfile struct {
	ID          string         `db:"id" json:"id"`
	Name        string         `db:"name" json:"name"`
	Description string         `db:"description" json:"description"`
	OSFamily    pq.StringArray `db:"os_family" json:"os_family"`
	OSPattern   pq.StringArray `db:"os_pattern" json:"os_pattern"`
	Ports       string         `db:"ports" json:"ports"`
	ScanType    string         `db:"scan_type" json:"scan_type"`
	Timing      string         `db:"timing" json:"timing"`
	Scripts     pq.StringArray `db:"scripts" json:"scripts"`
	Options     JSONB          `db:"options" json:"options"`
	Priority    int            `db:"priority" json:"priority"`
	BuiltIn     bool           `db:"built_in" json:"built_in"`
	CreatedAt   time.Time      `db:"created_at" json:"created_at"`
	UpdatedAt   time.Time      `db:"updated_at" json:"updated_at"`
}

// ScheduledJob represents a scheduled scanning or discovery job.
type ScheduledJob struct {
	ID             uuid.UUID  `db:"id" json:"id"`
	Name           string     `db:"name" json:"name"`
	Type           string     `db:"type" json:"type"` // 'discovery' or 'scan'
	CronExpression string     `db:"cron_expression" json:"cron_expression"`
	Config         JSONB      `db:"config" json:"config"`
	Enabled        bool       `db:"enabled" json:"enabled"`
	LastRun        *time.Time `db:"last_run" json:"last_run,omitempty"`
	NextRun        *time.Time `db:"next_run" json:"next_run,omitempty"`
	CreatedAt      time.Time  `db:"created_at" json:"created_at"`
}

// ScanJob represents a scan job execution.
type ScanJob struct {
	ID           uuid.UUID  `db:"id" json:"id"`
	TargetID     uuid.UUID  `db:"target_id" json:"target_id"`
	ProfileID    *string    `db:"profile_id" json:"profile_id,omitempty"`
	Status       string     `db:"status" json:"status"`
	StartedAt    *time.Time `db:"started_at" json:"started_at,omitempty"`
	CompletedAt  *time.Time `db:"completed_at" json:"completed_at,omitempty"`
	ErrorMessage *string    `db:"error_message" json:"error_message,omitempty"`
	ScanStats    JSONB      `db:"scan_stats" json:"scan_stats,omitempty"`
	CreatedAt    time.Time  `db:"created_at" json:"created_at"`
}

// OSFingerprint represents OS detection information.
type OSFingerprint struct {
	Family     string                 `json:"family"`     // "windows", "linux", "macos", "freebsd", etc.
	Name       string                 `json:"name"`       // "Windows Server 2019", "Ubuntu 22.04", etc.
	Version    string                 `json:"version"`    // "10.0.17763", "22.04.1", etc.
	Confidence int                    `json:"confidence"` // 0-100 confidence score
	Method     string                 `json:"method"`     // "tcp_fingerprint", "banner", "ttl_analysis"
	Details    map[string]interface{} `json:"details"`    // Additional OS-specific data
}

// Host represents a discovered host.
type Host struct {
	ID              uuid.UUID  `db:"id" json:"id"`
	IPAddress       IPAddr     `db:"ip_address" json:"ip_address"`
	Hostname        *string    `db:"hostname" json:"hostname,omitempty"`
	MACAddress      *MACAddr   `db:"mac_address" json:"mac_address,omitempty"`
	Vendor          *string    `db:"vendor" json:"vendor,omitempty"`
	OSFamily        *string    `db:"os_family" json:"os_family,omitempty"`
	OSName          *string    `db:"os_name" json:"os_name,omitempty"`
	OSVersion       *string    `db:"os_version" json:"os_version,omitempty"`
	OSConfidence    *int       `db:"os_confidence" json:"os_confidence,omitempty"`
	OSDetectedAt    *time.Time `db:"os_detected_at" json:"os_detected_at,omitempty"`
	OSMethod        *string    `db:"os_method" json:"os_method,omitempty"`
	OSDetails       JSONB      `db:"os_details" json:"os_details,omitempty"`
	DiscoveryMethod *string    `db:"discovery_method" json:"discovery_method,omitempty"`
	ResponseTimeMS  *int       `db:"response_time_ms" json:"response_time_ms,omitempty"`
	DiscoveryCount  int        `db:"discovery_count" json:"discovery_count"`
	IgnoreScanning  bool       `db:"ignore_scanning" json:"ignore_scanning"`
	FirstSeen       time.Time  `db:"first_seen" json:"first_seen"`
	LastSeen        time.Time  `db:"last_seen" json:"last_seen"`
	Status          string     `db:"status" json:"status"`
}

// GetOSFingerprint returns the OS fingerprint information.
func (h *Host) GetOSFingerprint() *OSFingerprint {
	if h.OSFamily == nil {
		return nil
	}

	fp := &OSFingerprint{
		Family:     *h.OSFamily,
		Confidence: 0,
		Method:     "unknown",
	}

	if h.OSName != nil {
		fp.Name = *h.OSName
	}
	if h.OSVersion != nil {
		fp.Version = *h.OSVersion
	}
	if h.OSConfidence != nil {
		fp.Confidence = *h.OSConfidence
	}
	if h.OSMethod != nil {
		fp.Method = *h.OSMethod
	}

	// Parse details from JSONB
	if len(h.OSDetails) > 0 {
		var details map[string]interface{}
		if err := json.Unmarshal([]byte(h.OSDetails), &details); err == nil {
			fp.Details = details
		}
	}

	return fp
}

// SetOSFingerprint updates the host with OS fingerprint information.
func (h *Host) SetOSFingerprint(fp *OSFingerprint) error {
	if fp == nil {
		return nil
	}

	h.OSFamily = &fp.Family
	h.OSName = &fp.Name
	h.OSVersion = &fp.Version
	h.OSConfidence = &fp.Confidence
	h.OSMethod = &fp.Method
	now := time.Now()
	h.OSDetectedAt = &now

	if fp.Details != nil {
		detailsJSON, err := json.Marshal(fp.Details)
		if err != nil {
			return fmt.Errorf("failed to marshal OS details: %w", err)
		}
		h.OSDetails = JSONB(detailsJSON)
	}

	return nil
}

// PortScan represents a port scan result.
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

// Service represents detailed service detection.
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

// HostHistory represents changes to hosts over time.
type HostHistory struct {
	ID        uuid.UUID `db:"id" json:"id"`
	HostID    uuid.UUID `db:"host_id" json:"host_id"`
	JobID     uuid.UUID `db:"job_id" json:"job_id"`
	EventType string    `db:"event_type" json:"event_type"`
	OldValue  JSONB     `db:"old_value" json:"old_value,omitempty"`
	NewValue  JSONB     `db:"new_value" json:"new_value,omitempty"`
	CreatedAt time.Time `db:"created_at" json:"created_at"`
}

// ActiveHost represents the active_hosts view.
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

// NetworkSummary represents the network_summary view.
type NetworkSummary struct {
	TargetName  string      `db:"target_name" json:"target_name"`
	Network     NetworkAddr `db:"network" json:"network"`
	ActiveHosts int         `db:"active_hosts" json:"active_hosts"`
	TotalHosts  int         `db:"total_hosts" json:"total_hosts"`
	OpenPorts   int         `db:"open_ports" json:"open_ports"`
	LastScan    *time.Time  `db:"last_scan" json:"last_scan,omitempty"`
}

// ScanJobStatus constants.
const (
	ScanJobStatusPending   = "pending"
	ScanJobStatusRunning   = "running"
	ScanJobStatusCompleted = "completed"
	ScanJobStatusFailed    = "failed"
)

// DiscoveryJobStatus constants.
const (
	DiscoveryJobStatusPending   = "pending"
	DiscoveryJobStatusRunning   = "running"
	DiscoveryJobStatusCompleted = "completed"
	DiscoveryJobStatusFailed    = "failed"
)

// ScheduledJobType constants.
const (
	ScheduledJobTypeDiscovery = "discovery"
	ScheduledJobTypeScan      = "scan"
)

// DiscoveryMethod constants.
const (
	DiscoveryMethodPing = "ping"
	DiscoveryMethodARP  = "arp"
	DiscoveryMethodTCP  = "tcp"
)

// OSFamily constants.
const (
	OSFamilyWindows = "windows"
	OSFamilyLinux   = "linux"
	OSFamilyMacOS   = "macos"
	OSFamilyFreeBSD = "freebsd"
	OSFamilyUnix    = "unix"
	OSFamilyUnknown = "unknown"
)

// ScanTiming constants.
const (
	ScanTimingParanoid   = "paranoid"
	ScanTimingPolite     = "polite"
	ScanTimingNormal     = "normal"
	ScanTimingAggressive = "aggressive"
	ScanTimingInsane     = "insane"
)

// HostStatus constants.
const (
	HostStatusUp      = "up"
	HostStatusDown    = "down"
	HostStatusUnknown = "unknown"
)

// PortState constants.
const (
	PortStateOpen     = "open"
	PortStateClosed   = "closed"
	PortStateFiltered = "filtered"
	PortStateUnknown  = "unknown"
)

// ScanType constants.
const (
	ScanTypeConnect = "connect"
	ScanTypeSYN     = "syn"
	ScanTypeVersion = "version"
)

// Protocol constants.
const (
	ProtocolTCP = "tcp"
	ProtocolUDP = "udp"
)

// HostHistoryEvent constants.
const (
	HostEventDiscovered   = "discovered"
	HostEventStatusChange = "status_change"
	HostEventPortsChanged = "ports_changed"
	HostEventServiceFound = "service_found"
)
