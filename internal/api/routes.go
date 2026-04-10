// Package api - HTTP route configuration for the Scanorama API server.
package api

import (
	"github.com/gorilla/mux"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	httpSwagger "github.com/swaggo/http-swagger/v2"

	_ "github.com/anstrom/scanorama/docs/swagger" // Import generated swagger docs
	apihandlers "github.com/anstrom/scanorama/internal/api/handlers"
	"github.com/anstrom/scanorama/internal/db"
	"github.com/anstrom/scanorama/internal/services"
)

// setupRoutes registers all HTTP routes on the server's router.
func (s *Server) setupRoutes() {
	api := s.router.PathPrefix("/api/v1").Subrouter()

	s.setupSystemRoutes(api)

	scanHandler := apihandlers.NewScanHandler(
		services.NewScanService(db.NewScanRepository(s.database), s.logger), s.logger, s.metrics).
		WithScanMode(s.config.Scanning.ScanMode)
	if s.scanQueue != nil {
		scanHandler.SetScanQueue(s.scanQueue)
	}
	hostHandler := apihandlers.NewHostHandler(
		services.NewHostService(db.NewHostRepository(s.database), s.logger), s.logger, s.metrics)
	discoveryHandler := apihandlers.NewDiscoveryHandler(db.NewDiscoveryRepository(s.database), s.logger, s.metrics).
		WithEngine(s.discoveryEngine)
	profileHandler := apihandlers.NewProfileHandler(
		services.NewProfileService(db.NewProfileRepository(s.database), s.logger), s.logger, s.metrics)
	scheduleHandler := apihandlers.NewScheduleHandler(
		services.NewScheduleService(db.NewScheduleRepository(s.database), s.logger), s.logger, s.metrics)
	networkHandler := apihandlers.NewNetworkHandler(services.NewNetworkService(s.database), s.logger, s.metrics).
		WithDiscovery(db.NewDiscoveryRepository(s.database), s.discoveryEngine).
		WithHostService(services.NewHostService(db.NewHostRepository(s.database), s.logger)).
		WithScanService(services.NewScanService(db.NewScanRepository(s.database), s.logger))
	groupHandler := apihandlers.NewGroupHandler(
		services.NewGroupService(db.NewGroupRepository(s.database), s.logger), s.logger, s.metrics)
	handlerManager := apihandlers.New(s.database, s.logger, s.metrics).
		WithRingBuffer(s.ringBuffer)
	if s.scanQueue != nil {
		handlerManager.SetScanQueue(s.scanQueue)
		discoveryHandler = discoveryHandler.WithScanQueue(s.scanQueue)
		networkHandler = networkHandler.WithScanQueue(s.scanQueue)
	}

	s.setupScanRoutes(api, scanHandler)
	s.setupHostRoutes(api, hostHandler)
	s.setupTagRoutes(api, hostHandler)
	s.setupGroupRoutes(api, groupHandler)
	s.setupDiscoveryRoutes(api, discoveryHandler)
	s.setupProfileRoutes(api, profileHandler)
	s.setupScheduleRoutes(api, scheduleHandler)
	s.setupNetworkRoutes(api, networkHandler)

	api.HandleFunc("/ws", handlerManager.GeneralWebSocket).Methods("GET")
	api.HandleFunc("/ws/scans", handlerManager.ScanWebSocket).Methods("GET")
	api.HandleFunc("/ws/logs", handlerManager.LogsWebSocket).Methods("GET")
	api.HandleFunc("/admin/logs", handlerManager.GetLogs).Methods("GET")
	api.HandleFunc("/admin/status", s.adminStatusHandler).Methods("GET")
	api.HandleFunc("/admin/workers", handlerManager.GetWorkerStatus).Methods("GET")

	statsHandler := apihandlers.NewStatsHandler(s.database, s.logger)
	settingsHandler := apihandlers.NewSettingsHandler(
		db.NewSettingsRepository(s.database), s.logger)

	api.HandleFunc("/stats/summary", statsHandler.GetStatsSummary).Methods("GET")
	api.HandleFunc("/admin/settings", settingsHandler.GetSettings).Methods("GET")
	api.HandleFunc("/admin/settings", settingsHandler.UpdateSettings).Methods("PUT")

	s.setupDocRoutes()
	s.router.HandleFunc("/", s.redirectToAPI).Methods("GET")
}

// setupSystemRoutes registers health, status, and metrics endpoints.
func (s *Server) setupSystemRoutes(api *mux.Router) {
	api.HandleFunc("/liveness", s.livenessHandler).Methods("GET")
	api.HandleFunc("/health", s.healthHandler).Methods("GET")
	api.HandleFunc("/status", s.statusHandler).Methods("GET")
	api.HandleFunc("/version", s.versionHandler).Methods("GET")
	api.HandleFunc("/metrics", s.metricsHandler).Methods("GET")
	s.router.Handle("/metrics", promhttp.HandlerFor(s.prom.GetRegistry(), promhttp.HandlerOpts{})).Methods("GET")
}

// setupScanRoutes registers scan CRUD and action endpoints.
func (s *Server) setupScanRoutes(api *mux.Router, h *apihandlers.ScanHandler) {
	api.HandleFunc("/scans", h.ListScans).Methods("GET")
	api.HandleFunc("/scans", h.CreateScan).Methods("POST")
	api.HandleFunc("/scans/{id}", h.GetScan).Methods("GET")
	api.HandleFunc("/scans/{id}", h.UpdateScan).Methods("PUT")
	api.HandleFunc("/scans/{id}", h.DeleteScan).Methods("DELETE")
	api.HandleFunc("/scans/{id}/results", h.GetScanResults).Methods("GET")
	api.HandleFunc("/scans/{id}/start", h.StartScan).Methods("POST")
	api.HandleFunc("/scans/{id}/stop", h.StopScan).Methods("POST")
}

// setupHostRoutes registers host CRUD endpoints.
func (s *Server) setupHostRoutes(api *mux.Router, h *apihandlers.HostHandler) {
	api.HandleFunc("/hosts", h.ListHosts).Methods("GET")
	api.HandleFunc("/hosts", h.CreateHost).Methods("POST")
	api.HandleFunc("/hosts", h.BulkDeleteHosts).Methods("DELETE")
	api.HandleFunc("/hosts/{id}", h.GetHost).Methods("GET")
	api.HandleFunc("/hosts/{id}", h.UpdateHost).Methods("PUT")
	api.HandleFunc("/hosts/{id}", h.DeleteHost).Methods("DELETE")
	api.HandleFunc("/hosts/{id}/scans", h.GetHostScans).Methods("GET")
}

// setupDiscoveryRoutes registers discovery job endpoints.
func (s *Server) setupDiscoveryRoutes(api *mux.Router, h *apihandlers.DiscoveryHandler) {
	api.HandleFunc("/discovery", h.ListDiscoveryJobs).Methods("GET")
	api.HandleFunc("/discovery", h.CreateDiscoveryJob).Methods("POST")
	api.HandleFunc("/discovery/compare", h.GetDiscoveryCompare).Methods("GET")
	api.HandleFunc("/discovery/{id}", h.GetDiscoveryJob).Methods("GET")
	api.HandleFunc("/discovery/{id}/diff", h.GetDiscoveryDiff).Methods("GET")
	api.HandleFunc("/discovery/{id}/start", h.StartDiscovery).Methods("POST")
	api.HandleFunc("/discovery/{id}/stop", h.StopDiscovery).Methods("POST")
}

// setupProfileRoutes registers scan profile CRUD endpoints.
func (s *Server) setupProfileRoutes(api *mux.Router, h *apihandlers.ProfileHandler) {
	api.HandleFunc("/profiles", h.ListProfiles).Methods("GET")
	api.HandleFunc("/profiles", h.CreateProfile).Methods("POST")
	api.HandleFunc("/profiles/{id}", h.GetProfile).Methods("GET")
	api.HandleFunc("/profiles/{id}", h.UpdateProfile).Methods("PUT")
	api.HandleFunc("/profiles/{id}", h.DeleteProfile).Methods("DELETE")
	api.HandleFunc("/profiles/{id}/clone", h.CloneProfile).Methods("POST")
}

// setupScheduleRoutes registers scheduled job CRUD and control endpoints.
func (s *Server) setupScheduleRoutes(api *mux.Router, h *apihandlers.ScheduleHandler) {
	api.HandleFunc("/schedules", h.ListSchedules).Methods("GET")
	api.HandleFunc("/schedules", h.CreateSchedule).Methods("POST")
	api.HandleFunc("/schedules/{id}", h.GetSchedule).Methods("GET")
	api.HandleFunc("/schedules/{id}", h.UpdateSchedule).Methods("PUT")
	api.HandleFunc("/schedules/{id}", h.DeleteSchedule).Methods("DELETE")
	api.HandleFunc("/schedules/{id}/enable", h.EnableSchedule).Methods("POST")
	api.HandleFunc("/schedules/{id}/disable", h.DisableSchedule).Methods("POST")
	api.HandleFunc("/schedules/{id}/next-run", h.GetScheduleNextRun).Methods("GET")
}

// setupNetworkRoutes registers network CRUD, control, and exclusion endpoints.
func (s *Server) setupNetworkRoutes(api *mux.Router, h *apihandlers.NetworkHandler) {
	api.HandleFunc("/networks", h.ListNetworks).Methods("GET")
	api.HandleFunc("/networks", h.CreateNetwork).Methods("POST")
	api.HandleFunc("/networks/stats", h.GetNetworkStats).Methods("GET")
	api.HandleFunc("/networks/{id}", h.GetNetwork).Methods("GET")
	api.HandleFunc("/networks/{id}", h.UpdateNetwork).Methods("PUT")
	api.HandleFunc("/networks/{id}", h.DeleteNetwork).Methods("DELETE")
	api.HandleFunc("/networks/{id}/enable", h.EnableNetwork).Methods("POST")
	api.HandleFunc("/networks/{id}/disable", h.DisableNetwork).Methods("POST")
	api.HandleFunc("/networks/{id}/rename", h.RenameNetwork).Methods("PUT")
	api.HandleFunc("/networks/{id}/exclusions", h.ListNetworkExclusions).Methods("GET")
	api.HandleFunc("/networks/{id}/exclusions", h.CreateNetworkExclusion).Methods("POST")
	api.HandleFunc("/networks/{id}/discover", h.StartNetworkDiscovery).Methods("POST")
	api.HandleFunc("/networks/{id}/discovery", h.ListNetworkDiscoveryJobs).Methods("GET")
	api.HandleFunc("/networks/{id}/scan", h.StartNetworkScan).Methods("POST")
	api.HandleFunc("/exclusions", h.ListGlobalExclusions).Methods("GET")
	api.HandleFunc("/exclusions", h.CreateGlobalExclusion).Methods("POST")
	api.HandleFunc("/exclusions/{id}", h.DeleteExclusion).Methods("DELETE")
}

// setupTagRoutes registers tag management endpoints.
func (s *Server) setupTagRoutes(api *mux.Router, h *apihandlers.HostHandler) {
	api.HandleFunc("/tags", h.ListTags).Methods("GET")
	// Bulk tag endpoint must be registered BEFORE the {id} pattern to avoid mux ambiguity.
	api.HandleFunc("/hosts/bulk/tags", h.BulkUpdateTags).Methods("POST")
	api.HandleFunc("/hosts/{id}/tags", h.ReplaceHostTags).Methods("PUT")
	api.HandleFunc("/hosts/{id}/tags", h.AddHostTags).Methods("POST")
	api.HandleFunc("/hosts/{id}/tags", h.DeleteHostTags).Methods("DELETE")
}

// setupGroupRoutes registers host group CRUD and membership endpoints.
func (s *Server) setupGroupRoutes(api *mux.Router, h *apihandlers.GroupHandler) {
	api.HandleFunc("/groups", h.ListGroups).Methods("GET")
	api.HandleFunc("/groups", h.CreateGroup).Methods("POST")
	api.HandleFunc("/groups/{id}", h.GetGroup).Methods("GET")
	api.HandleFunc("/groups/{id}", h.UpdateGroup).Methods("PUT")
	api.HandleFunc("/groups/{id}", h.DeleteGroup).Methods("DELETE")
	api.HandleFunc("/groups/{id}/hosts", h.ListGroupMembers).Methods("GET")
	api.HandleFunc("/groups/{id}/hosts", h.AddGroupMembers).Methods("POST")
	api.HandleFunc("/groups/{id}/hosts", h.RemoveGroupMembers).Methods("DELETE")
}

// setupDocRoutes registers Swagger documentation and alias endpoints.
func (s *Server) setupDocRoutes() {
	s.router.PathPrefix("/swagger/").Handler(httpSwagger.Handler(
		httpSwagger.URL("/swagger/doc.json"),
		httpSwagger.DeepLinking(true),
		httpSwagger.DocExpansion("none"),
	))
	s.router.HandleFunc("/docs", s.redirectToSwagger).Methods("GET")
	s.router.HandleFunc("/docs/", s.redirectToSwagger).Methods("GET")
	s.router.HandleFunc("/api-docs", s.redirectToSwagger).Methods("GET")
}
