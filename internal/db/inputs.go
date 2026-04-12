// Package db provides typed input structs for all database mutating methods.
// These replace the previous interface{}/map[string]interface{} parameters,
// giving callers compile-time safety and making the available fields explicit.
package db

import (
	"time"

	"github.com/google/uuid"
)

// ── Scan inputs ───────────────────────────────────────────────────────────────

// CreateScanInput holds the data required to create a new scan record.
type CreateScanInput struct {
	Name        string
	Description string
	Targets     []string
	ScanType    string
	Ports       string
	ProfileID   *string
	OSDetection bool
	NetworkID   *uuid.UUID // optional: caller-supplied FK to an existing networks row
	Source      *string    // optional: ScanSourceAPI / ScanSourceAuto / ScanSourceScheduled
}

// UpdateScanInput holds the optional fields that may be changed on an existing
// scan.  A nil pointer means "leave this field unchanged".
type UpdateScanInput struct {
	Name        *string
	Description *string
	ScanType    *string
	Ports       *string
	ProfileID   *string
	Status      *string
}

// ── Host inputs ───────────────────────────────────────────────────────────────

// CreateHostInput holds the data required to create a new host record.
type CreateHostInput struct {
	IPAddress      string
	Hostname       string
	Vendor         string
	OSFamily       string
	OSName         string
	OSVersion      string
	IgnoreScanning bool
	Status         string
	// Tags is the initial list of labels to assign to this host.
	Tags []string
}

// UpdateHostInput holds the optional fields that may be changed on an existing
// host.  A nil pointer means "leave this field unchanged".
type UpdateHostInput struct {
	Hostname       *string
	Vendor         *string
	OSFamily       *string
	OSName         *string
	OSVersion      *string
	IgnoreScanning *bool
	Status         *string
	// Tags replaces the host's entire tag list when non-nil.
	// A pointer to an empty slice clears all tags; nil means "leave unchanged".
	Tags *[]string
}

// ── Profile inputs ────────────────────────────────────────────────────────────

// CreateProfileInput holds the data required to create a new scan profile.
type CreateProfileInput struct {
	Name        string
	Description string
	ScanType    string
	Ports       string
	Options     map[string]interface{}
	Timing      string
}

// UpdateProfileInput holds the optional fields that may be changed on an
// existing profile.  A nil pointer / nil map means "leave this field
// unchanged".
type UpdateProfileInput struct {
	Name        *string
	Description *string
	ScanType    *string
	Ports       *string
	Timing      *string
	Scripts     []string
	Options     map[string]interface{}
	Priority    *int
}

// ── Schedule inputs ───────────────────────────────────────────────────────────

// CreateScheduleInput holds the data required to create a new scheduled job.
type CreateScheduleInput struct {
	Name           string
	JobType        string
	CronExpression string
	JobConfig      map[string]interface{}
	Enabled        bool
}

// UpdateScheduleInput holds the optional fields that may be changed on an
// existing schedule.  A nil pointer / nil map means "leave this field
// unchanged".
type UpdateScheduleInput struct {
	Name           *string
	JobType        *string
	CronExpression *string
	JobConfig      map[string]interface{}
	Enabled        *bool
	NextRun        *time.Time
}

// ── Discovery job inputs ──────────────────────────────────────────────────────

// CreateDiscoveryJobInput holds the data required to create a new discovery
// job.  Networks must contain at least one CIDR; only the first is used when
// creating the underlying DB row.  NetworkID, when set, links the job to a
// registered network in the networks table so discovery history and stats can
// be surfaced from the network detail view.
type CreateDiscoveryJobInput struct {
	Networks  []string
	Method    string
	NetworkID *uuid.UUID // optional FK to registered network
}

// UpdateDiscoveryJobInput holds the optional fields that may be changed on an
// existing discovery job.  A nil pointer means "leave this field unchanged".
type UpdateDiscoveryJobInput struct {
	Method          *string
	Status          *string
	HostsDiscovered *int
	HostsResponsive *int
	StartedAt       *time.Time
	CompletedAt     *time.Time
}

// ── Group inputs ──────────────────────────────────────────────────────────────

// CreateGroupInput holds the data required to create a new host group.
type CreateGroupInput struct {
	Name        string
	Description string
	FilterRule  *JSONB // optional stored FilterExpr (not enforced in v0.24)
	Color       string // hex color, e.g. "#3b82f6"
}

// UpdateGroupInput holds the optional fields that may be changed on an existing
// host group. A nil pointer means "leave this field unchanged".
type UpdateGroupInput struct {
	Name        *string
	Description *string
	FilterRule  *JSONB // set to non-nil to update; ignored unless ClearFilter is true or non-nil
	ClearFilter bool   // when true, sets filter_rule to NULL regardless of FilterRule value
	Color       *string
}
