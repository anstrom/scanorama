// Package handlers provides HTTP request handlers for the Scanorama API.
// This package implements REST endpoint handlers for scanning, discovery,
// host management, and administrative operations.
package handlers

import (
	"log/slog"
	"net/http"

	"github.com/anstrom/scanorama/internal/db"
	"github.com/anstrom/scanorama/internal/metrics"
)

// HandlerManager manages all API handlers and their dependencies.
type HandlerManager struct {
	database *db.DB
	logger   *slog.Logger
	metrics  *metrics.Registry

	// Individual handler groups
	health    *HealthHandler
	scan      *ScanHandler
	host      *HostHandler
	discovery *DiscoveryHandler
	profile   *ProfileHandler
	schedule  *ScheduleHandler
	admin     *AdminHandler
	websocket *WebSocketHandler
}

// New creates a new handler manager with all handler groups initialized.
func New(database *db.DB, logger *slog.Logger, metricsManager *metrics.Registry) *HandlerManager {
	hm := &HandlerManager{
		database: database,
		logger:   logger,
		metrics:  metricsManager,
	}

	// Initialize individual handler groups
	hm.health = NewHealthHandler(database, logger, metricsManager)
	hm.scan = NewScanHandler(database, logger, metricsManager)
	hm.host = NewHostHandler(database, logger, metricsManager)
	hm.discovery = NewDiscoveryHandler(database, logger, metricsManager)
	hm.profile = NewProfileHandler(database, logger, metricsManager)
	hm.schedule = NewScheduleHandler(database, logger, metricsManager)
	hm.admin = NewAdminHandler(database, logger, metricsManager)
	hm.websocket = NewWebSocketHandler(database, logger, metricsManager)

	return hm
}

// Health and status endpoints.
func (hm *HandlerManager) Health(w http.ResponseWriter, r *http.Request) {
	hm.health.Health(w, r)
}

// Status handles GET /status - get system status.
func (hm *HandlerManager) Status(w http.ResponseWriter, r *http.Request) {
	hm.health.Status(w, r)
}

// Version handles GET /version - get version information.
func (hm *HandlerManager) Version(w http.ResponseWriter, r *http.Request) {
	hm.health.Version(w, r)
}

// Metrics handles GET /metrics - get application metrics.
func (hm *HandlerManager) Metrics(w http.ResponseWriter, r *http.Request) {
	hm.health.Metrics(w, r)
}

// ListScans handles GET /api/v1/scans - list all scans.
func (hm *HandlerManager) ListScans(w http.ResponseWriter, r *http.Request) {
	hm.scan.ListScans(w, r)
}

// CreateScan handles POST /api/v1/scans - create a new scan.
func (hm *HandlerManager) CreateScan(w http.ResponseWriter, r *http.Request) {
	hm.scan.CreateScan(w, r)
}

// GetScan handles GET /api/v1/scans/{id} - get a specific scan.
func (hm *HandlerManager) GetScan(w http.ResponseWriter, r *http.Request) {
	hm.scan.GetScan(w, r)
}

// UpdateScan handles PUT /api/v1/scans/{id} - update an existing scan.
func (hm *HandlerManager) UpdateScan(w http.ResponseWriter, r *http.Request) {
	hm.scan.UpdateScan(w, r)
}

// DeleteScan handles DELETE /api/v1/scans/{id} - delete a scan.
func (hm *HandlerManager) DeleteScan(w http.ResponseWriter, r *http.Request) {
	hm.scan.DeleteScan(w, r)
}

// GetScanResults handles GET /api/v1/scans/{id}/results - get scan results.
func (hm *HandlerManager) GetScanResults(w http.ResponseWriter, r *http.Request) {
	hm.scan.GetScanResults(w, r)
}

// StartScan handles POST /api/v1/scans/{id}/start - start a scan.
func (hm *HandlerManager) StartScan(w http.ResponseWriter, r *http.Request) {
	hm.scan.StartScan(w, r)
}

// StopScan handles POST /api/v1/scans/{id}/stop - stop a scan.
func (hm *HandlerManager) StopScan(w http.ResponseWriter, r *http.Request) {
	hm.scan.StopScan(w, r)
}

// ListHosts handles GET /api/v1/hosts - list all hosts.
func (hm *HandlerManager) ListHosts(w http.ResponseWriter, r *http.Request) {
	hm.host.ListHosts(w, r)
}

// CreateHost handles POST /api/v1/hosts - create a new host.
func (hm *HandlerManager) CreateHost(w http.ResponseWriter, r *http.Request) {
	hm.host.CreateHost(w, r)
}

// GetHost handles GET /api/v1/hosts/{id} - get a specific host.
func (hm *HandlerManager) GetHost(w http.ResponseWriter, r *http.Request) {
	hm.host.GetHost(w, r)
}

// UpdateHost handles PUT /api/v1/hosts/{id} - update an existing host.
func (hm *HandlerManager) UpdateHost(w http.ResponseWriter, r *http.Request) {
	hm.host.UpdateHost(w, r)
}

// DeleteHost handles DELETE /api/v1/hosts/{id} - delete a host.
func (hm *HandlerManager) DeleteHost(w http.ResponseWriter, r *http.Request) {
	hm.host.DeleteHost(w, r)
}

// GetHostScans handles GET /api/v1/hosts/{id}/scans - get scans for a host.
func (hm *HandlerManager) GetHostScans(w http.ResponseWriter, r *http.Request) {
	hm.host.GetHostScans(w, r)
}

// ListDiscoveryJobs handles GET /api/v1/discovery - list all discovery jobs.
func (hm *HandlerManager) ListDiscoveryJobs(w http.ResponseWriter, r *http.Request) {
	hm.discovery.ListDiscoveryJobs(w, r)
}

// CreateDiscoveryJob handles POST /api/v1/discovery - create a new discovery job.
func (hm *HandlerManager) CreateDiscoveryJob(w http.ResponseWriter, r *http.Request) {
	hm.discovery.CreateDiscoveryJob(w, r)
}

// GetDiscoveryJob handles GET /api/v1/discovery/{id} - get a specific discovery job.
func (hm *HandlerManager) GetDiscoveryJob(w http.ResponseWriter, r *http.Request) {
	hm.discovery.GetDiscoveryJob(w, r)
}

// UpdateDiscoveryJob handles PUT /api/v1/discovery/{id} - update an existing discovery job.
func (hm *HandlerManager) UpdateDiscoveryJob(w http.ResponseWriter, r *http.Request) {
	hm.discovery.UpdateDiscoveryJob(w, r)
}

// DeleteDiscoveryJob handles DELETE /api/v1/discovery/{id} - delete a discovery job.
func (hm *HandlerManager) DeleteDiscoveryJob(w http.ResponseWriter, r *http.Request) {
	hm.discovery.DeleteDiscoveryJob(w, r)
}

// StartDiscovery handles POST /api/v1/discovery/{id}/start - start discovery.
func (hm *HandlerManager) StartDiscovery(w http.ResponseWriter, r *http.Request) {
	hm.discovery.StartDiscovery(w, r)
}

// StopDiscovery handles POST /api/v1/discovery/{id}/stop - stop discovery.
func (hm *HandlerManager) StopDiscovery(w http.ResponseWriter, r *http.Request) {
	hm.discovery.StopDiscovery(w, r)
}

// ListProfiles handles GET /api/v1/profiles - list all profiles.
func (hm *HandlerManager) ListProfiles(w http.ResponseWriter, r *http.Request) {
	hm.profile.ListProfiles(w, r)
}

// CreateProfile handles POST /api/v1/profiles - create a new profile.
func (hm *HandlerManager) CreateProfile(w http.ResponseWriter, r *http.Request) {
	hm.profile.CreateProfile(w, r)
}

// GetProfile handles GET /api/v1/profiles/{id} - get a specific profile.
func (hm *HandlerManager) GetProfile(w http.ResponseWriter, r *http.Request) {
	hm.profile.GetProfile(w, r)
}

// UpdateProfile handles PUT /api/v1/profiles/{id} - update an existing profile.
func (hm *HandlerManager) UpdateProfile(w http.ResponseWriter, r *http.Request) {
	hm.profile.UpdateProfile(w, r)
}

// DeleteProfile handles DELETE /api/v1/profiles/{id} - delete a profile.
func (hm *HandlerManager) DeleteProfile(w http.ResponseWriter, r *http.Request) {
	hm.profile.DeleteProfile(w, r)
}

// ListSchedules handles GET /api/v1/schedules - list all schedules.
func (hm *HandlerManager) ListSchedules(w http.ResponseWriter, r *http.Request) {
	hm.schedule.ListSchedules(w, r)
}

// CreateSchedule handles POST /api/v1/schedules - create a new schedule.
func (hm *HandlerManager) CreateSchedule(w http.ResponseWriter, r *http.Request) {
	hm.schedule.CreateSchedule(w, r)
}

// GetSchedule handles GET /api/v1/schedules/{id} - get a specific schedule.
func (hm *HandlerManager) GetSchedule(w http.ResponseWriter, r *http.Request) {
	hm.schedule.GetSchedule(w, r)
}

// UpdateSchedule handles PUT /api/v1/schedules/{id} - update an existing schedule.
func (hm *HandlerManager) UpdateSchedule(w http.ResponseWriter, r *http.Request) {
	hm.schedule.UpdateSchedule(w, r)
}

// DeleteSchedule handles DELETE /api/v1/schedules/{id} - delete a schedule.
func (hm *HandlerManager) DeleteSchedule(w http.ResponseWriter, r *http.Request) {
	hm.schedule.DeleteSchedule(w, r)
}

// EnableSchedule handles POST /api/v1/schedules/{id}/enable - enable a schedule.
func (hm *HandlerManager) EnableSchedule(w http.ResponseWriter, r *http.Request) {
	hm.schedule.EnableSchedule(w, r)
}

// DisableSchedule handles POST /api/v1/schedules/{id}/disable - disable a schedule.
func (hm *HandlerManager) DisableSchedule(w http.ResponseWriter, r *http.Request) {
	hm.schedule.DisableSchedule(w, r)
}

// GetWorkerStatus retrieves the status of workers.
func (hm *HandlerManager) GetWorkerStatus(w http.ResponseWriter, r *http.Request) {
	hm.admin.GetWorkerStatus(w, r)
}

// StopWorker stops a specific worker.
func (hm *HandlerManager) StopWorker(w http.ResponseWriter, r *http.Request) {
	hm.admin.StopWorker(w, r)
}

// GetConfig retrieves the current configuration.
func (hm *HandlerManager) GetConfig(w http.ResponseWriter, r *http.Request) {
	hm.admin.GetConfig(w, r)
}

// UpdateConfig updates the configuration.
func (hm *HandlerManager) UpdateConfig(w http.ResponseWriter, r *http.Request) {
	hm.admin.UpdateConfig(w, r)
}

// GetLogs retrieves system logs.
func (hm *HandlerManager) GetLogs(w http.ResponseWriter, r *http.Request) {
	hm.admin.GetLogs(w, r)
}

// ScanWebSocket handles WebSocket connections for scan updates.
func (hm *HandlerManager) ScanWebSocket(w http.ResponseWriter, r *http.Request) {
	hm.websocket.ScanWebSocket(w, r)
}

// DiscoveryWebSocket handles WebSocket connections for discovery updates.
func (hm *HandlerManager) DiscoveryWebSocket(w http.ResponseWriter, r *http.Request) {
	hm.websocket.DiscoveryWebSocket(w, r)
}

// GetDatabase returns the database instance.
func (hm *HandlerManager) GetDatabase() *db.DB {
	return hm.database
}

// GetLogger returns the logger instance.
func (hm *HandlerManager) GetLogger() *slog.Logger {
	return hm.logger
}

// GetMetrics returns the metrics manager.
func (hm *HandlerManager) GetMetrics() *metrics.Registry {
	return hm.metrics
}
