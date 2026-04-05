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
