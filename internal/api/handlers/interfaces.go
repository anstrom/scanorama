// Package handlers provides HTTP request handlers for the Scanorama API.
// This file defines the narrow interfaces that each handler depends on,
// allowing unit tests to inject mocks without a live database.
package handlers

import (
	"context"
	"time"

	"github.com/google/uuid"

	"github.com/anstrom/scanorama/internal/db"
	"github.com/anstrom/scanorama/internal/services"
)

// ScanServicer is the service-level interface consumed by ScanHandler.
// It extends the basic CRUD contract with a lifecycle-aware StartScan that
// enforces the state machine and returns the updated scan in one round-trip.
//
//go:generate go run go.uber.org/mock/mockgen -typed -destination mocks/mock_scan_servicer.go -package mocks github.com/anstrom/scanorama/internal/api/handlers ScanServicer
type ScanServicer interface {
	ListScans(ctx context.Context, filters db.ScanFilters, offset, limit int) ([]*db.Scan, int64, error)
	CreateScan(ctx context.Context, input db.CreateScanInput) (*db.Scan, error)
	GetScan(ctx context.Context, id uuid.UUID) (*db.Scan, error)
	UpdateScan(ctx context.Context, id uuid.UUID, input db.UpdateScanInput) (*db.Scan, error)
	DeleteScan(ctx context.Context, id uuid.UUID) error
	// StartScan validates the scan is in a startable state, marks it running,
	// and returns the refreshed scan record — saving the caller an extra GetScan call.
	StartScan(ctx context.Context, id uuid.UUID) (*db.Scan, error)
	StopScan(ctx context.Context, id uuid.UUID, errMsg ...string) error
	CompleteScan(ctx context.Context, id uuid.UUID) error
	GetScanResults(ctx context.Context, scanID uuid.UUID, offset, limit int) ([]*db.ScanResult, int64, error)
	GetScanSummary(ctx context.Context, scanID uuid.UUID) (*db.ScanSummary, error)
	GetProfile(ctx context.Context, id string) (*db.ScanProfile, error)
}

// ScheduleServicer is the service-level interface consumed by ScheduleHandler.
// It adds cron-expression validation (enforced inside CreateSchedule/UpdateSchedule)
// and a NextRun helper on top of the standard CRUD operations.
//
//go:generate go run go.uber.org/mock/mockgen -typed -destination mocks/mock_schedule_servicer.go -package mocks github.com/anstrom/scanorama/internal/api/handlers ScheduleServicer
type ScheduleServicer interface {
	ListSchedules(ctx context.Context, filters db.ScheduleFilters, offset, limit int) ([]*db.Schedule, int64, error)
	CreateSchedule(ctx context.Context, input db.CreateScheduleInput) (*db.Schedule, error)
	GetSchedule(ctx context.Context, id uuid.UUID) (*db.Schedule, error)
	UpdateSchedule(ctx context.Context, id uuid.UUID, input db.UpdateScheduleInput) (*db.Schedule, error)
	DeleteSchedule(ctx context.Context, id uuid.UUID) error
	EnableSchedule(ctx context.Context, id uuid.UUID) error
	DisableSchedule(ctx context.Context, id uuid.UUID) error
	// NextRun computes the next scheduled execution time for the given schedule.
	NextRun(ctx context.Context, id uuid.UUID) (time.Time, error)
}

// DiscoveryStore is the subset of *db.DB used by DiscoveryHandler.
//
//go:generate go run go.uber.org/mock/mockgen -typed -destination mocks/mock_discovery_store.go -package mocks github.com/anstrom/scanorama/internal/api/handlers DiscoveryStore
type DiscoveryStore interface {
	ListDiscoveryJobs(ctx context.Context, filters db.DiscoveryFilters, offset, limit int) (
		[]*db.DiscoveryJob, int64, error)
	ListDiscoveryJobsByNetwork(ctx context.Context, networkID uuid.UUID, offset, limit int) (
		[]*db.DiscoveryJob, int64, error)
	CreateDiscoveryJob(ctx context.Context, input db.CreateDiscoveryJobInput) (*db.DiscoveryJob, error)
	GetDiscoveryJob(ctx context.Context, id uuid.UUID) (*db.DiscoveryJob, error)
	UpdateDiscoveryJob(ctx context.Context, id uuid.UUID, input db.UpdateDiscoveryJobInput) (*db.DiscoveryJob, error)
	DeleteDiscoveryJob(ctx context.Context, id uuid.UUID) error
	StartDiscoveryJob(ctx context.Context, id uuid.UUID) error
	StopDiscoveryJob(ctx context.Context, id uuid.UUID) error
	GetDiscoveryDiff(ctx context.Context, jobID uuid.UUID) (*db.DiscoveryDiff, error)
	CompareDiscoveryRuns(ctx context.Context, jobA, jobB uuid.UUID) (*db.DiscoveryCompareDiff, error)
}

// HostServicer is the service-level interface consumed by HostHandler.
//
//go:generate go run go.uber.org/mock/mockgen -typed -destination mocks/mock_host_servicer.go -package mocks github.com/anstrom/scanorama/internal/api/handlers HostServicer
type HostServicer interface {
	ListHosts(ctx context.Context, filters *db.HostFilters, offset, limit int) ([]*db.Host, int64, error)
	CreateHost(ctx context.Context, input db.CreateHostInput) (*db.Host, error)
	GetHost(ctx context.Context, id uuid.UUID) (*db.Host, error)
	UpdateHost(ctx context.Context, id uuid.UUID, input db.UpdateHostInput) (*db.Host, error)
	DeleteHost(ctx context.Context, id uuid.UUID) error
	BulkDeleteHosts(ctx context.Context, ids []uuid.UUID) (int64, error)
	GetHostScans(ctx context.Context, hostID uuid.UUID, offset, limit int) ([]*db.Scan, int64, error)
	ListTags(ctx context.Context) ([]string, error)
	UpdateHostTags(ctx context.Context, id uuid.UUID, tags []string) error
	AddHostTags(ctx context.Context, id uuid.UUID, tags []string) error
	RemoveHostTags(ctx context.Context, id uuid.UUID, tags []string) error
	BulkUpdateTags(ctx context.Context, ids []uuid.UUID, tags []string, action string) error
	GetHostGroups(ctx context.Context, hostID uuid.UUID) ([]db.HostGroupSummary, error)
	GetHostNetworks(ctx context.Context, hostID uuid.UUID) ([]*db.Network, error)
}

// ProfileServicer is the service-level interface consumed by ProfileHandler.
// It extends the basic CRUD contract with a CloneProfile operation.
//
//go:generate go run go.uber.org/mock/mockgen -typed -destination mocks/mock_profile_servicer.go -package mocks github.com/anstrom/scanorama/internal/api/handlers ProfileServicer
type ProfileServicer interface {
	ListProfiles(ctx context.Context, filters db.ProfileFilters, offset, limit int) ([]*db.ScanProfile, int64, error)
	CreateProfile(ctx context.Context, input db.CreateProfileInput) (*db.ScanProfile, error)
	GetProfile(ctx context.Context, id string) (*db.ScanProfile, error)
	UpdateProfile(ctx context.Context, id string, input db.UpdateProfileInput) (*db.ScanProfile, error)
	DeleteProfile(ctx context.Context, id string) error
	// CloneProfile creates a new user-owned profile that is a copy of the source
	// profile identified by fromID, assigned the given newName.
	CloneProfile(ctx context.Context, fromID string, newName string) (*db.ScanProfile, error)
	// GetProfileStats returns scan effectiveness statistics for a profile.
	GetProfileStats(ctx context.Context, id string) (*db.ProfileStats, error)
}

// NetworkServicer is the subset of *services.NetworkService used by NetworkHandler.
//
//go:generate go run go.uber.org/mock/mockgen -typed -destination mocks/mock_network_servicer.go -package mocks github.com/anstrom/scanorama/internal/api/handlers NetworkServicer
type NetworkServicer interface {
	ListNetworks(ctx context.Context, activeOnly bool) ([]*db.Network, error)
	GetNetworkByID(ctx context.Context, id uuid.UUID) (*services.NetworkWithExclusions, error)
	CreateNetwork(
		ctx context.Context, name, cidr, description, method string, active, scanEnabled bool,
	) (*db.Network, error)
	UpdateNetwork(
		ctx context.Context, id uuid.UUID, name, cidr, description, method string, active bool,
	) (*db.Network, error)
	DeleteNetwork(ctx context.Context, id uuid.UUID) error
	AddExclusion(ctx context.Context, networkID *uuid.UUID, cidr, reason string) (*db.NetworkExclusion, error)
	RemoveExclusion(ctx context.Context, exclusionID uuid.UUID) error
	GetNetworkExclusions(ctx context.Context, networkID uuid.UUID) ([]*db.NetworkExclusion, error)
	GetGlobalExclusions(ctx context.Context) ([]*db.NetworkExclusion, error)
	GetNetworkStats(ctx context.Context) (map[string]interface{}, error)
}

// DeviceServicer is the service-level interface consumed by DeviceHandler.
//
//go:generate go run go.uber.org/mock/mockgen -typed -destination mocks/mock_device_servicer.go -package mocks github.com/anstrom/scanorama/internal/api/handlers DeviceServicer
type DeviceServicer interface {
	ListDevices(ctx context.Context) ([]db.DeviceSummary, error)
	CreateDevice(ctx context.Context, input db.CreateDeviceInput) (*db.Device, error)
	GetDeviceDetail(ctx context.Context, id uuid.UUID) (*db.DeviceDetail, error)
	UpdateDevice(ctx context.Context, id uuid.UUID, input db.UpdateDeviceInput) (*db.Device, error)
	DeleteDevice(ctx context.Context, id uuid.UUID) error
	AttachHost(ctx context.Context, deviceID, hostID uuid.UUID) error
	DetachHost(ctx context.Context, deviceID, hostID uuid.UUID) error
	AcceptSuggestion(ctx context.Context, suggestionID uuid.UUID) error
	DismissSuggestion(ctx context.Context, suggestionID uuid.UUID) error
}

// GroupServicer is the service-level interface consumed by GroupHandler.
//
//go:generate go run go.uber.org/mock/mockgen -typed -destination mocks/mock_group_servicer.go -package mocks github.com/anstrom/scanorama/internal/api/handlers GroupServicer
type GroupServicer interface {
	ListGroups(ctx context.Context) ([]*db.HostGroup, error)
	CreateGroup(ctx context.Context, input db.CreateGroupInput) (*db.HostGroup, error)
	GetGroup(ctx context.Context, id uuid.UUID) (*db.HostGroup, error)
	UpdateGroup(ctx context.Context, id uuid.UUID, input db.UpdateGroupInput) (*db.HostGroup, error)
	DeleteGroup(ctx context.Context, id uuid.UUID) error
	AddHostsToGroup(ctx context.Context, groupID uuid.UUID, hostIDs []uuid.UUID) error
	RemoveHostsFromGroup(ctx context.Context, groupID uuid.UUID, hostIDs []uuid.UUID) error
	GetGroupMembers(ctx context.Context, groupID uuid.UUID, offset, limit int) ([]*db.Host, int64, error)
}
