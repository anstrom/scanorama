// Package handlers provides HTTP request handlers for the Scanorama API.
// This file defines the narrow interfaces that each handler depends on,
// allowing unit tests to inject mocks without a live database.
package handlers

import (
	"context"

	"github.com/google/uuid"

	"github.com/anstrom/scanorama/internal/db"
	"github.com/anstrom/scanorama/internal/services"
)

// ScanStore is the subset of *db.DB used by ScanHandler.
//
//go:generate go run go.uber.org/mock/mockgen -typed -destination mocks/mock_scan_store.go -package mocks github.com/anstrom/scanorama/internal/api/handlers ScanStore
type ScanStore interface {
	ListScans(ctx context.Context, filters db.ScanFilters, offset, limit int) ([]*db.Scan, int64, error)
	CreateScan(ctx context.Context, input interface{}) (*db.Scan, error)
	GetScan(ctx context.Context, id uuid.UUID) (*db.Scan, error)
	UpdateScan(ctx context.Context, id uuid.UUID, scanData interface{}) (*db.Scan, error)
	DeleteScan(ctx context.Context, id uuid.UUID) error
	StartScan(ctx context.Context, id uuid.UUID) error
	CompleteScan(ctx context.Context, id uuid.UUID) error
	StopScan(ctx context.Context, id uuid.UUID, errMsg ...string) error
	GetScanResults(ctx context.Context, scanID uuid.UUID, offset, limit int) ([]*db.ScanResult, int64, error)
	GetScanSummary(ctx context.Context, scanID uuid.UUID) (*db.ScanSummary, error)
}

// ScheduleStore is the subset of *db.DB used by ScheduleHandler.
//
//go:generate go run go.uber.org/mock/mockgen -typed -destination mocks/mock_schedule_store.go -package mocks github.com/anstrom/scanorama/internal/api/handlers ScheduleStore
//nolint:dupl
type ScheduleStore interface {
	ListSchedules(ctx context.Context, filters db.ScheduleFilters, offset, limit int) ([]*db.Schedule, int64, error)
	CreateSchedule(ctx context.Context, scheduleData interface{}) (*db.Schedule, error)
	GetSchedule(ctx context.Context, id uuid.UUID) (*db.Schedule, error)
	UpdateSchedule(ctx context.Context, id uuid.UUID, scheduleData interface{}) (*db.Schedule, error)
	DeleteSchedule(ctx context.Context, id uuid.UUID) error
	EnableSchedule(ctx context.Context, id uuid.UUID) error
	DisableSchedule(ctx context.Context, id uuid.UUID) error
}

// DiscoveryStore is the subset of *db.DB used by DiscoveryHandler.
//
//go:generate go run go.uber.org/mock/mockgen -typed -destination mocks/mock_discovery_store.go -package mocks github.com/anstrom/scanorama/internal/api/handlers DiscoveryStore
//nolint:dupl
type DiscoveryStore interface {
	ListDiscoveryJobs(ctx context.Context, filters db.DiscoveryFilters, offset, limit int) (
		[]*db.DiscoveryJob, int64, error)
	CreateDiscoveryJob(ctx context.Context, jobData interface{}) (*db.DiscoveryJob, error)
	GetDiscoveryJob(ctx context.Context, id uuid.UUID) (*db.DiscoveryJob, error)
	UpdateDiscoveryJob(ctx context.Context, id uuid.UUID, jobData interface{}) (*db.DiscoveryJob, error)
	DeleteDiscoveryJob(ctx context.Context, id uuid.UUID) error
	StartDiscoveryJob(ctx context.Context, id uuid.UUID) error
	StopDiscoveryJob(ctx context.Context, id uuid.UUID) error
}

// HostStore is the subset of *db.DB used by HostHandler.
//
//go:generate go run go.uber.org/mock/mockgen -typed -destination mocks/mock_host_store.go -package mocks github.com/anstrom/scanorama/internal/api/handlers HostStore
type HostStore interface {
	ListHosts(ctx context.Context, filters db.HostFilters, offset, limit int) ([]*db.Host, int64, error)
	CreateHost(ctx context.Context, hostData interface{}) (*db.Host, error)
	GetHost(ctx context.Context, id uuid.UUID) (*db.Host, error)
	UpdateHost(ctx context.Context, id uuid.UUID, hostData interface{}) (*db.Host, error)
	DeleteHost(ctx context.Context, id uuid.UUID) error
	GetHostScans(ctx context.Context, hostID uuid.UUID, offset, limit int) ([]*db.Scan, int64, error)
}

// ProfileStore is the subset of *db.DB used by ProfileHandler.
//
//go:generate go run go.uber.org/mock/mockgen -typed -destination mocks/mock_profile_store.go -package mocks github.com/anstrom/scanorama/internal/api/handlers ProfileStore
type ProfileStore interface {
	ListProfiles(ctx context.Context, filters db.ProfileFilters, offset, limit int) ([]*db.ScanProfile, int64, error)
	CreateProfile(ctx context.Context, profileData interface{}) (*db.ScanProfile, error)
	GetProfile(ctx context.Context, id string) (*db.ScanProfile, error)
	UpdateProfile(ctx context.Context, id string, profileData interface{}) (*db.ScanProfile, error)
	DeleteProfile(ctx context.Context, id string) error
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
