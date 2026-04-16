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

// MarshalJSON implements json.Marshaler for NetworkAddr, encoding as a CIDR string.
func (n NetworkAddr) MarshalJSON() ([]byte, error) {
	if len(n.IP) == 0 {
		return nil, nil
	}
	return json.Marshal(n.IPNet.String())
}

// UnmarshalJSON implements json.Unmarshaler for NetworkAddr, decoding from a CIDR string.
func (n *NetworkAddr) UnmarshalJSON(data []byte) error {
	var s string
	if err := json.Unmarshal(data, &s); err != nil {
		return fmt.Errorf("NetworkAddr.UnmarshalJSON: expected a string, got: %w", err)
	}
	_, ipnet, err := net.ParseCIDR(s)
	if err != nil {
		return fmt.Errorf("NetworkAddr.UnmarshalJSON: failed to parse CIDR %q: %w", s, err)
	}
	n.IPNet = *ipnet
	return nil
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
// When the pq driver uses binary wire protocol for a JSONB column it prepends a
// one-byte version marker (0x01) before the JSON text.  We strip that prefix
// so callers always receive well-formed JSON bytes.  If the resulting bytes are
// not valid JSON (e.g. the column contains unexpected binary data), Scan stores
// nil rather than propagating garbage that would later cause MarshalJSON to fail.
func (j *JSONB) Scan(value interface{}) error {
	if value == nil {
		*j = nil
		return nil
	}

	var raw []byte
	switch v := value.(type) {
	case []byte:
		// Make a copy so we don't hold a reference to the driver-owned buffer.
		raw = make([]byte, len(v))
		copy(raw, v)
	case string:
		raw = []byte(v)
	default:
		return fmt.Errorf("cannot scan %T into JSONB", value)
	}

	// PostgreSQL JSONB binary format: first byte is version 0x01 followed by
	// JSON text.  Strip the version byte when present.
	if len(raw) > 1 && raw[0] == 0x01 {
		raw = raw[1:]
	}

	if !json.Valid(raw) {
		*j = nil
		return nil
	}

	*j = JSONB(raw)
	return nil
}

// Value implements driver.Valuer for PostgreSQL JSONB type.
// Returning a string (not []byte) ensures pq uses the text wire protocol when
// binding this value as a query parameter.  This prevents pq from encoding the
// value as a PostgreSQL bytea in binary mode, which would corrupt the stored
// JSONB data.
func (j JSONB) Value() (driver.Value, error) {
	if j == nil {
		return nil, nil
	}
	return string(j), nil
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

// DiscoveryJob represents a network discovery job.
type DiscoveryJob struct {
	ID              uuid.UUID   `db:"id" json:"id"`
	NetworkID       *uuid.UUID  `db:"network_id" json:"network_id,omitempty"`
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
	ID                  uuid.UUID  `db:"id" json:"id"`
	Name                string     `db:"name" json:"name"`
	Type                string     `db:"type" json:"type"` // 'discovery' or 'scan'
	CronExpression      string     `db:"cron_expression" json:"cron_expression"`
	Config              JSONB      `db:"config" json:"config"`
	Enabled             bool       `db:"enabled" json:"enabled"`
	LastRun             *time.Time `db:"last_run" json:"last_run,omitempty"`
	NextRun             *time.Time `db:"next_run" json:"next_run,omitempty"`
	CreatedAt           time.Time  `db:"created_at" json:"created_at"`
	LastRunDurationMs   *int       `db:"last_run_duration_ms" json:"last_run_duration_ms,omitempty"`
	LastRunStatus       *string    `db:"last_run_status" json:"last_run_status,omitempty"`
	ConsecutiveFailures *int       `db:"consecutive_failures" json:"consecutive_failures,omitempty"`
	MaxFailures         *int       `db:"max_failures" json:"max_failures,omitempty"`
}

// ScanJob represents a scan job execution.
type ScanJob struct {
	ID               uuid.UUID  `db:"id" json:"id"`
	NetworkID        *uuid.UUID `db:"network_id" json:"network_id,omitempty"`
	ProfileID        *string    `db:"profile_id" json:"profile_id,omitempty"`
	Status           string     `db:"status" json:"status"`
	StartedAt        *time.Time `db:"started_at" json:"started_at,omitempty"`
	CompletedAt      *time.Time `db:"completed_at" json:"completed_at,omitempty"`
	ErrorMessage     *string    `db:"error_message" json:"error_message,omitempty"`
	ScanStats        JSONB      `db:"scan_stats" json:"scan_stats,omitempty"`
	CreatedAt        time.Time  `db:"created_at" json:"created_at"`
	ProgressPercent  *int       `db:"progress_percent" json:"progress_percent,omitempty"`
	TimeoutAt        *time.Time `db:"timeout_at" json:"timeout_at,omitempty"`
	ExecutionDetails JSONB      `db:"execution_details" json:"execution_details,omitempty"`
	WorkerID         *string    `db:"worker_id" json:"worker_id,omitempty"`
	CreatedBy        *string    `db:"created_by" json:"created_by,omitempty"`
	Source           *string    `db:"source" json:"source,omitempty"`
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

// PortInfo holds the latest known state for a single (port, protocol) pair on a host.
// It is computed from port_scans using DISTINCT ON so it always reflects the most
// recent scan observation, not a historical snapshot.
type PortInfo struct {
	Port     int       `json:"port"`
	Protocol string    `json:"protocol"`
	State    string    `json:"state"`
	Service  string    `json:"service,omitempty"`
	LastSeen time.Time `json:"last_seen"`
}

// Host represents a discovered host.
type Host struct {
	ID                uuid.UUID  `db:"id" json:"id"`
	IPAddress         IPAddr     `db:"ip_address" json:"ip_address"`
	Hostname          *string    `db:"hostname" json:"hostname,omitempty"`
	MACAddress        *MACAddr   `db:"mac_address" json:"mac_address,omitempty"`
	Vendor            *string    `db:"vendor" json:"vendor,omitempty"`
	OSFamily          *string    `db:"os_family" json:"os_family,omitempty"`
	OSName            *string    `db:"os_name" json:"os_name,omitempty"`
	OSVersion         *string    `db:"os_version" json:"os_version,omitempty"`
	OSConfidence      *int       `db:"os_confidence" json:"os_confidence,omitempty"`
	OSDetectedAt      *time.Time `db:"os_detected_at" json:"os_detected_at,omitempty"`
	OSMethod          *string    `db:"os_method" json:"os_method,omitempty"`
	OSDetails         JSONB      `db:"os_details" json:"os_details,omitempty"`
	DiscoveryMethod   *string    `db:"discovery_method" json:"discovery_method,omitempty"`
	ResponseTimeMS    *int       `db:"response_time_ms" json:"response_time_ms,omitempty"`
	DiscoveryCount    int        `db:"discovery_count" json:"discovery_count"`
	IgnoreScanning    bool       `db:"ignore_scanning" json:"ignore_scanning"`
	FirstSeen         time.Time  `db:"first_seen" json:"first_seen"`
	LastSeen          time.Time  `db:"last_seen" json:"last_seen"`
	Status            string     `db:"status" json:"status"`
	StatusChangedAt   *time.Time `db:"status_changed_at" json:"status_changed_at,omitempty"`
	PreviousStatus    *string    `db:"previous_status" json:"previous_status,omitempty"`
	TimeoutCount      int        `db:"timeout_count" json:"timeout_count"`
	ResponseTimeMinMS *int       `db:"response_time_min_ms" json:"response_time_min_ms,omitempty"`
	ResponseTimeMaxMS *int       `db:"response_time_max_ms" json:"response_time_max_ms,omitempty"`
	ResponseTimeAvgMS *int       `db:"response_time_avg_ms" json:"response_time_avg_ms,omitempty"`
	// Tags is the list of user-defined labels assigned to this host.
	// Populated from the hosts.tags column (TEXT[] array).
	// pq.StringArray (not []string) is required so sqlx SELECT * auto-mapping
	// can scan a PostgreSQL TEXT[] value via its sql.Scanner implementation.
	Tags pq.StringArray `db:"tags" json:"tags,omitempty"`
	// KnowledgeScore is a 0-100 integer indicating how much is known about this
	// host. Recomputed by the knowledge service after each enrichment pass.
	KnowledgeScore int `db:"knowledge_score" json:"knowledge_score"`
	// Computed from port_scans — latest known state per (port, protocol).
	// Ports holds the full PortInfo for every distinct (port, protocol) seen.
	// TotalPorts is the count of distinct (port, protocol) pairs ever seen.
	Ports      []PortInfo `db:"-" json:"-"`
	TotalPorts int        `db:"-" json:"-"`
	ScanCount  int        `db:"-" json:"-"`
	// Groups the host belongs to. Populated by GetHost (detail view only, not list).
	Groups []HostGroupSummary `db:"-" json:"groups,omitempty"`
	// DeviceID is the stable device this host is currently attached to, if any.
	DeviceID *uuid.UUID `db:"device_id" json:"device_id,omitempty"`
	// MDNSName is the most recently resolved mDNS .local name for this host/IP.
	MDNSName *string `db:"mdns_name" json:"mdns_name,omitempty"`
	// DeviceName is the name of the attached device, populated by JOIN in GetHost/ListHosts.
	DeviceName *string `db:"-" json:"device_name,omitempty"`
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
	ScanDurationMs *int      `db:"scan_duration_ms" json:"scan_duration_ms,omitempty"`
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
	ID           uuid.UUID `db:"id" json:"id"`
	HostID       uuid.UUID `db:"host_id" json:"host_id"`
	JobID        uuid.UUID `db:"job_id" json:"job_id"`
	EventType    string    `db:"event_type" json:"event_type"`
	OldValue     JSONB     `db:"old_value" json:"old_value,omitempty"`
	NewValue     JSONB     `db:"new_value" json:"new_value,omitempty"`
	CreatedAt    time.Time `db:"created_at" json:"created_at"`
	ChangedBy    *string   `db:"changed_by" json:"changed_by,omitempty"`
	ChangeReason *string   `db:"change_reason" json:"change_reason,omitempty"`
	ClientIP     *IPAddr   `db:"client_ip" json:"client_ip,omitempty"`
}

// HostStatusEvent records a single host status transition.
type HostStatusEvent struct {
	ID         uuid.UUID `db:"id"          json:"id"`
	HostID     uuid.UUID `db:"host_id"     json:"host_id"`
	FromStatus string    `db:"from_status" json:"from_status"`
	ToStatus   string    `db:"to_status"   json:"to_status"`
	ChangedAt  time.Time `db:"changed_at"  json:"changed_at"`
	Source     *string   `db:"source"      json:"source,omitempty"`
}

// HostTimeoutEvent records a single instance of a host failing to respond
// during a discovery run. Unlike HostStatusEvent, a row is inserted on every
// missed run — not just on status transitions — so the frontend can show an
// accurate consecutive-miss count and a "last timed out" timestamp.
type HostTimeoutEvent struct {
	ID             uuid.UUID  `db:"id"               json:"id"`
	HostID         uuid.UUID  `db:"host_id"          json:"host_id"`
	Source         string     `db:"source"           json:"source"`
	DiscoveryRunID *uuid.UUID `db:"discovery_run_id" json:"discovery_run_id,omitempty"`
	RecordedAt     time.Time  `db:"recorded_at"      json:"recorded_at"`
}

// DiffHost is a lightweight host snapshot used in discovery diff results.
type DiffHost struct {
	ID             uuid.UUID `db:"id"              json:"id"`
	IPAddress      string    `db:"ip_address"      json:"ip_address"`
	Hostname       *string   `db:"hostname"        json:"hostname,omitempty"`
	Status         string    `db:"status"          json:"status"`
	PreviousStatus *string   `db:"previous_status" json:"previous_status,omitempty"`
	Vendor         *string   `db:"vendor"          json:"vendor,omitempty"`
	MACAddress     *string   `db:"mac_address"     json:"mac_address,omitempty"`
	LastSeen       time.Time `db:"last_seen"       json:"last_seen"`
	FirstSeen      time.Time `db:"first_seen"      json:"first_seen"`
}

// DiscoveryDiff summarizes what changed during a single discovery run.
type DiscoveryDiff struct {
	JobID          uuid.UUID  `json:"job_id"`
	NewHosts       []DiffHost `json:"new_hosts"`
	GoneHosts      []DiffHost `json:"gone_hosts"`
	ChangedHosts   []DiffHost `json:"changed_hosts"`
	UnchangedCount int        `json:"unchanged_count"`
	// Suggestions are non-dismissed device match candidates for new and
	// changed hosts in this diff, ordered by confidence_score DESC.
	Suggestions []DeviceSuggestion `json:"suggestions"`
}

// DiscoveryCompareDiff summarizes what changed between two discovery runs.
type DiscoveryCompareDiff struct {
	RunAID         uuid.UUID  `json:"run_a_id"`
	RunBID         uuid.UUID  `json:"run_b_id"`
	NewHosts       []DiffHost `json:"new_hosts"`
	GoneHosts      []DiffHost `json:"gone_hosts"`
	ChangedHosts   []DiffHost `json:"changed_hosts"`
	UnchangedCount int        `json:"unchanged_count"`
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

// Network represents a network configured for discovery and scanning.
type Network struct {
	ID                  uuid.UUID   `db:"id" json:"id"`
	Name                string      `db:"name" json:"name"`
	CIDR                NetworkAddr `db:"cidr" json:"cidr"`
	Description         *string     `db:"description" json:"description,omitempty"`
	DiscoveryMethod     string      `db:"discovery_method" json:"discovery_method"`
	IsActive            bool        `db:"is_active" json:"is_active"`
	ScanEnabled         bool        `db:"scan_enabled" json:"scan_enabled"`
	ScanIntervalSeconds int         `db:"scan_interval_seconds" json:"scan_interval_seconds"`
	ScanPorts           string      `db:"scan_ports" json:"scan_ports"`
	ScanType            string      `db:"scan_type" json:"scan_type"`
	LastDiscovery       *time.Time  `db:"last_discovery" json:"last_discovery,omitempty"`
	LastScan            *time.Time  `db:"last_scan" json:"last_scan,omitempty"`
	HostCount           int         `db:"host_count" json:"host_count"`
	ActiveHostCount     int         `db:"active_host_count" json:"active_host_count"`
	CreatedAt           time.Time   `db:"created_at" json:"created_at"`
	UpdatedAt           time.Time   `db:"updated_at" json:"updated_at"`
	CreatedBy           *string     `db:"created_by" json:"created_by,omitempty"`
	ModifiedBy          *string     `db:"modified_by" json:"modified_by,omitempty"`
}

// NetworkExclusion represents an IP exclusion rule for networks.
type NetworkExclusion struct {
	ID           uuid.UUID  `db:"id" json:"id"`
	NetworkID    *uuid.UUID `db:"network_id" json:"network_id,omitempty"` // NULL for global exclusions
	ExcludedCIDR string     `db:"excluded_cidr" json:"excluded_cidr"`
	Reason       *string    `db:"reason" json:"reason,omitempty"`
	Enabled      bool       `db:"enabled" json:"enabled"`
	CreatedAt    time.Time  `db:"created_at" json:"created_at"`
	UpdatedAt    time.Time  `db:"updated_at" json:"updated_at"`
	CreatedBy    *string    `db:"created_by" json:"created_by,omitempty"`
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

// HostGroupSummary is a lightweight representation of a group embedded in host responses.
type HostGroupSummary struct {
	ID    uuid.UUID `json:"id"`
	Name  string    `json:"name"`
	Color string    `json:"color,omitempty"`
}

// HostGroup is the full host group record returned by group endpoints.
type HostGroup struct {
	ID          uuid.UUID `json:"id"                    db:"id"`
	Name        string    `json:"name"                  db:"name"`
	Description string    `json:"description"           db:"description"`
	FilterRule  JSONB     `json:"filter_rule,omitempty" db:"filter_rule"`
	Color       string    `json:"color,omitempty"       db:"color"`
	MemberCount int       `json:"member_count"          db:"-"` // computed via COUNT JOIN
	CreatedAt   time.Time `json:"created_at"            db:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"            db:"updated_at"`
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
	ScheduledJobTypeSmartScan = "smart_scan"
)

// ScanSource constants describe what triggered a scan job.
const (
	ScanSourceAPI       = "api"       // triggered by a user via the API
	ScanSourceAuto      = "auto"      // triggered by post-scan auto-progression
	ScanSourceScheduled = "scheduled" // triggered by the cron scheduler
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
	HostStatusGone    = "gone"
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
	ScanTypeConnect       = "connect"
	ScanTypeSYN           = "syn"
	ScanTypeACK           = "ack"
	ScanTypeUDP           = "udp"
	ScanTypeAggressive    = "aggressive"
	ScanTypeComprehensive = "comprehensive"
)

// Protocol constants.
const (
	ProtocolTCP = "tcp"
	ProtocolUDP = "udp"
)

// DNSRecord represents one resolved DNS record for a host.
type DNSRecord struct {
	ID         uuid.UUID `db:"id"          json:"id"`
	HostID     uuid.UUID `db:"host_id"     json:"host_id"`
	RecordType string    `db:"record_type" json:"record_type"`
	Value      string    `db:"value"       json:"value"`
	TTL        *int      `db:"ttl"         json:"ttl,omitempty"`
	ResolvedAt time.Time `db:"resolved_at" json:"resolved_at"`
}

// HostHistoryEvent constants.
const (
	HostEventDiscovered   = "discovered"
	HostEventStatusChange = "status_change"
	HostEventPortsChanged = "ports_changed"
	HostEventServiceFound = "service_found"
)

// PortDefinition is a curated port/protocol entry with service metadata.
type PortDefinition struct {
	Port        int      `db:"port"        json:"port"`
	Protocol    string   `db:"protocol"    json:"protocol"`
	Service     string   `db:"service"     json:"service"`
	Description string   `db:"description" json:"description,omitempty"`
	Category    string   `db:"category"    json:"category,omitempty"`
	OSFamilies  []string `db:"os_families" json:"os_families"`
	IsStandard  bool     `db:"is_standard" json:"is_standard"`
}

// PortFilters holds query parameters for listing port definitions.
type PortFilters struct {
	Search     string
	Category   string
	Protocol   string
	IsStandard *bool
	SortBy     string
	SortOrder  string
}

// Certificate is a TLS certificate record captured from a host/port.
type Certificate struct {
	ID          uuid.UUID  `db:"id"           json:"id"`
	HostID      uuid.UUID  `db:"host_id"      json:"host_id"`
	Port        int        `db:"port"         json:"port"`
	SubjectCN   *string    `db:"subject_cn"   json:"subject_cn,omitempty"`
	SANs        []string   `db:"sans"         json:"sans,omitempty"`
	Issuer      *string    `db:"issuer"       json:"issuer,omitempty"`
	NotBefore   *time.Time `db:"not_before"   json:"not_before,omitempty"`
	NotAfter    *time.Time `db:"not_after"    json:"not_after,omitempty"`
	KeyType     *string    `db:"key_type"     json:"key_type,omitempty"`
	TLSVersion  *string    `db:"tls_version"  json:"tls_version,omitempty"`
	CipherSuite *string    `db:"cipher_suite" json:"cipher_suite,omitempty"`
	RawBanner   *string    `db:"raw_banner"   json:"raw_banner,omitempty"`
	ScannedAt   time.Time  `db:"scanned_at"   json:"scanned_at"`
}

// PortBanner is a raw service banner captured from a host/port.
type PortBanner struct {
	ID                  uuid.UUID `db:"id"                     json:"id"`
	HostID              uuid.UUID `db:"host_id"                json:"host_id"`
	Port                int       `db:"port"                   json:"port"`
	Protocol            string    `db:"protocol"               json:"protocol"`
	RawBanner           *string   `db:"raw_banner"             json:"raw_banner,omitempty"`
	Service             *string   `db:"service"                json:"service,omitempty"`
	Version             *string   `db:"version"                json:"version,omitempty"`
	HTTPTitle           *string   `db:"http_title"             json:"http_title,omitempty"`
	SSHKeyFingerprint   *string   `db:"ssh_key_fingerprint"    json:"ssh_key_fingerprint,omitempty"`
	HTTPStatusCode      *int16    `db:"http_status_code"       json:"http_status_code,omitempty"`
	HTTPRedirect        *string   `db:"http_redirect"          json:"http_redirect,omitempty"`
	HTTPResponseHeaders JSONB     `db:"http_response_headers"  json:"http_response_headers,omitempty"`
	ScannedAt           time.Time `db:"scanned_at"             json:"scanned_at"`
	ExtendedProbeDone   bool      `db:"extended_probe_done"    json:"-"`
}

// SNMPInterface describes a single network interface collected via SNMP.
type SNMPInterface struct {
	Name        string `json:"name,omitempty"`
	AdminStatus string `json:"admin_status,omitempty"`
	Status      string `json:"status,omitempty"`
	Speed       uint   `json:"speed_mbps,omitempty"`
	MAC         string `json:"mac,omitempty"`
	RxBytes     uint64 `json:"rx_bytes,omitempty"`
	TxBytes     uint64 `json:"tx_bytes,omitempty"`
}

// HostSNMPData holds SNMP data collected from a network device.
type HostSNMPData struct {
	HostID      uuid.UUID `db:"host_id"      json:"host_id"`
	SysName     *string   `db:"sys_name"     json:"sys_name,omitempty"`
	SysDescr    *string   `db:"sys_descr"    json:"sys_descr,omitempty"`
	SysLocation *string   `db:"sys_location" json:"sys_location,omitempty"`
	SysContact  *string   `db:"sys_contact"  json:"sys_contact,omitempty"`
	SysUptime   *int64    `db:"sys_uptime"   json:"sys_uptime_cs,omitempty"`
	IfCount     *int      `db:"if_count"     json:"if_count,omitempty"`
	Interfaces  JSONB     `db:"interfaces"   json:"interfaces,omitempty"`
	Community   *string   `db:"community"    json:"-"`
	CollectedAt time.Time `db:"collected_at" json:"collected_at"`
}
