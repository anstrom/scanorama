package db

import (
	"time"

	"github.com/google/uuid"
)

// Device is a stable identity record that survives MAC randomization and IP churn.
// Hosts are attached via the hosts.device_id FK.
type Device struct {
	ID        uuid.UUID `db:"id"         json:"id"`
	Name      string    `db:"name"       json:"name"`
	Notes     *string   `db:"notes"      json:"notes,omitempty"`
	CreatedAt time.Time `db:"created_at" json:"created_at"`
	UpdatedAt time.Time `db:"updated_at" json:"updated_at"`
}

// DeviceKnownMAC records one MAC address ever seen for a device.
type DeviceKnownMAC struct {
	ID         uuid.UUID `db:"id"          json:"id"`
	DeviceID   uuid.UUID `db:"device_id"   json:"device_id"`
	MACAddress string    `db:"mac_address" json:"mac_address"`
	FirstSeen  time.Time `db:"first_seen"  json:"first_seen"`
	LastSeen   time.Time `db:"last_seen"   json:"last_seen"`
}

// DeviceKnownName records one (name, source) pair ever seen for a device.
// Source is one of: mdns, dns, snmp, netbios, user.
type DeviceKnownName struct {
	ID        uuid.UUID `db:"id"         json:"id"`
	DeviceID  uuid.UUID `db:"device_id"  json:"device_id"`
	Name      string    `db:"name"       json:"name"`
	Source    string    `db:"source"     json:"source"`
	FirstSeen time.Time `db:"first_seen" json:"first_seen"`
	LastSeen  time.Time `db:"last_seen"  json:"last_seen"`
}

// DeviceSuggestion is a low-confidence host↔device match candidate produced by
// DeviceMatcher and surfaced to the user for accept/dismiss.
type DeviceSuggestion struct {
	ID               uuid.UUID `db:"id"                json:"id"`
	HostID           uuid.UUID `db:"host_id"           json:"host_id"`
	DeviceID         uuid.UUID `db:"device_id"         json:"device_id"`
	ConfidenceScore  int       `db:"confidence_score"  json:"confidence_score"`
	ConfidenceReason *string   `db:"confidence_reason" json:"confidence_reason,omitempty"`
	Dismissed        bool      `db:"dismissed"         json:"dismissed"`
	CreatedAt        time.Time `db:"created_at"        json:"created_at"`
}

// AttachedHostSummary is the lightweight host view embedded in DeviceDetail.
// It contains only the columns populated by listAttachedHosts — using the full
// Host type would serialize misleading zero values for unscanned fields.
type AttachedHostSummary struct {
	ID         uuid.UUID `json:"id"`
	IPAddress  IPAddr    `json:"ip_address"`
	MACAddress *MACAddr  `json:"mac_address,omitempty"`
	Hostname   *string   `json:"hostname,omitempty"`
	Status     string    `json:"status"`
	OSFamily   *string   `json:"os_family,omitempty"`
	Vendor     *string   `json:"vendor,omitempty"`
	FirstSeen  time.Time `json:"first_seen"`
	LastSeen   time.Time `json:"last_seen"`
}

// DeviceDetail is the full device view returned by GET /api/v1/devices/{id}.
type DeviceDetail struct {
	Device
	KnownMACs  []DeviceKnownMAC      `json:"known_macs"`
	KnownNames []DeviceKnownName     `json:"known_names"`
	Hosts      []AttachedHostSummary `json:"hosts"`
}

// DeviceSummary is the lightweight row returned by GET /api/v1/devices.
type DeviceSummary struct {
	ID        uuid.UUID `json:"id"`
	Name      string    `json:"name"`
	MACCount  int       `json:"mac_count"`
	HostCount int       `json:"host_count"`
}

// CreateDeviceInput holds the fields accepted by POST /api/v1/devices.
type CreateDeviceInput struct {
	Name  string  `json:"name"`
	Notes *string `json:"notes,omitempty"`
}

// UpdateDeviceInput holds the fields accepted by PUT /api/v1/devices/{id}.
// Nil fields are left unchanged (COALESCE semantics in the repository).
type UpdateDeviceInput struct {
	Name  *string `json:"name,omitempty"`
	Notes *string `json:"notes,omitempty"`
}

// DeviceSignals bundles a device with its known MACs and names for bulk matching.
// Used by DeviceMatcher to score candidates without N+1 queries.
type DeviceSignals struct {
	Device     Device
	KnownMACs  []string
	KnownNames []DeviceKnownNameSignal
}

// DeviceKnownNameSignal is the minimal name+source pair used during matching.
type DeviceKnownNameSignal struct {
	Name   string
	Source string
}
