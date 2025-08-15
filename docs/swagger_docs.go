// Package docs provides Swagger documentation for the Scanorama API.
//
// This file contains all API endpoint documentation using swaggo annotations.
// Run `swag init` to generate OpenAPI specification files.
//
// IMPROVEMENTS:
// - Added operationId to all endpoints for proper client generation
// - Applied security definitions consistently
// - Added comprehensive error responses
// - Improved response coverage and documentation
//
//go:generate swag init -g swagger_docs.go -o ./swagger --parseDependency --parseInternal
package docs

import (
	"net/http"
	"time"
)

// @title Scanorama API
// @version 0.7.0
// @description Enterprise-grade network scanning and discovery service with automated reconnaissance capabilities
// @description
// @description ## Features
// @description - **Advanced Scanning Engine**: Multiple scan types (connect, SYN, version, comprehensive, aggressive, stealth)
// @description - **Enterprise Reliability**: Race condition-free worker pools with graceful shutdown
// @description - **Database Integration**: PostgreSQL persistence with automatic migrations and transaction support
// @description - **Real-time Updates**: WebSocket support for live scan progress and results
// @description - **Comprehensive API**: RESTful endpoints with full CRUD operations
// @description - **Monitoring & Observability**: Built-in metrics, structured logging, and health checks
// @description - **Security**: Vulnerability scanning integration and secure error handling
// @description - **Scheduling**: Automated scan jobs with cron-like scheduling
// @description - **High Performance**: Concurrent processing with configurable rate limiting
// @description
// @description ## Quality Assurance
// @description - **Test Coverage**: >90% coverage on core packages with comprehensive integration tests
// @description - **Security**: Zero known vulnerabilities with automated security scanning
// @description - **Code Quality**: Zero linting issues with automated quality checks
// @description
// @description ## Authentication
// @description Most endpoints require API key authentication. Include your API key in the `X-API-Key` header.
// @description Public endpoints (health, status, version, metrics) do not require authentication.
//
// @security ApiKeyAuth
//
// @contact.name Scanorama Support
// @contact.url https://github.com/anstrom/scanorama
//
// @license.name MIT
// @license.url https://github.com/anstrom/scanorama/blob/main/LICENSE
//
// @host localhost:8080
// @BasePath /api/v1
//
// @securityDefinitions.apikey ApiKeyAuth
// @in header
// @name X-API-Key
// @description API key for authentication

// HealthResponse represents health check response
type HealthResponse struct {
	Status    string            `json:"status" example:"healthy"`
	Timestamp time.Time         `json:"timestamp"`
	Uptime    string            `json:"uptime" example:"2h30m45s"`
	Checks    map[string]string `json:"checks"`
}

// StatusResponse represents system status response
type StatusResponse struct {
	Service   string    `json:"service" example:"scanorama-api"`
	Version   string    `json:"version" example:"0.7.0"`
	Timestamp time.Time `json:"timestamp"`
	Uptime    string    `json:"uptime" example:"2h30m45s"`
}

// VersionResponse represents version information
type VersionResponse struct {
	Version   string    `json:"version" example:"0.7.0"`
	Service   string    `json:"service" example:"scanorama"`
	Timestamp time.Time `json:"timestamp"`
}

// ErrorResponse represents an error response
type ErrorResponse struct {
	Error     string    `json:"error" example:"Invalid request"`
	Timestamp time.Time `json:"timestamp"`
	RequestID string    `json:"request_id,omitempty" example:"req-123"`
}

// AdminStatusResponse represents administrative status
type AdminStatusResponse struct {
	AdminStatus string                 `json:"admin_status" example:"active"`
	Timestamp   time.Time              `json:"timestamp"`
	ServerInfo  map[string]interface{} `json:"server_info"`
}

// ScanResponse represents a scan object
type ScanResponse struct {
	ID              string     `json:"id" example:"550e8400-e29b-41d4-a716-446655440000"`
	ProfileID       string     `json:"profile_id" example:"550e8400-e29b-41d4-a716-446655440001"`
	Targets         []string   `json:"targets" example:"192.168.1.0/24"`
	Status          string     `json:"status" example:"running" enums:"pending,running,completed,failed,cancelled"`
	Progress        float64    `json:"progress" example:"65.5"`
	CreatedAt       time.Time  `json:"created_at"`
	StartedAt       *time.Time `json:"started_at,omitempty"`
	CompletedAt     *time.Time `json:"completed_at,omitempty"`
	Duration        *string    `json:"duration,omitempty" example:"14m30s"`
	HostsDiscovered int        `json:"hosts_discovered" example:"25"`
	PortsScanned    int        `json:"ports_scanned" example:"2500"`
	ErrorMessage    *string    `json:"error_message,omitempty"`
}

// CreateScanRequest represents a request to create a new scan
type CreateScanRequest struct {
	ProfileID   string                 `json:"profile_id" example:"550e8400-e29b-41d4-a716-446655440001"`
	Targets     []string               `json:"targets" example:"192.168.1.0/24"`
	Name        *string                `json:"name,omitempty" example:"Weekly security scan"`
	Description *string                `json:"description,omitempty" example:"Regular security assessment"`
	ScanOptions map[string]interface{} `json:"scan_options,omitempty"`
}

// HostResponse represents a discovered host
type HostResponse struct {
	ID         string    `json:"id" example:"550e8400-e29b-41d4-a716-446655440002"`
	IPAddress  string    `json:"ip_address" example:"192.168.1.100"`
	Hostname   *string   `json:"hostname,omitempty" example:"server01.local"`
	MACAddress *string   `json:"mac_address,omitempty" example:"00:1B:44:11:3A:B7"`
	OpenPorts  []int     `json:"open_ports" example:"22,80,443"`
	Status     string    `json:"status" example:"up" enums:"up,down,unknown"`
	LastSeen   time.Time `json:"last_seen"`
	FirstSeen  time.Time `json:"first_seen"`
	ScanCount  int       `json:"scan_count" example:"5"`
}

// ProfileResponse represents a scan profile
type ProfileResponse struct {
	ID          string                 `json:"id" example:"550e8400-e29b-41d4-a716-446655440003"`
	Name        string                 `json:"name" example:"Quick Connect Scan"`
	Description *string                `json:"description,omitempty" example:"Fast TCP connect scan"`
	ScanType    string                 `json:"scan_type" example:"connect"`
	Ports       *string                `json:"ports,omitempty" example:"22,80,443"`
	Options     map[string]interface{} `json:"options,omitempty"`
	CreatedAt   time.Time              `json:"created_at"`
	UpdatedAt   time.Time              `json:"updated_at"`
}

// CreateProfileRequest represents a request to create a scan profile
type CreateProfileRequest struct {
	Name        string                 `json:"name" example:"Custom Scan Profile"`
	Description *string                `json:"description,omitempty" example:"Custom scan configuration"`
	ScanType    string                 `json:"scan_type" example:"connect"`
	Ports       *string                `json:"ports,omitempty" example:"22,80,443,8080"`
	Options     map[string]interface{} `json:"options,omitempty"`
}

// DiscoveryJobResponse represents a discovery job
type DiscoveryJobResponse struct {
	ID        string     `json:"id" example:"550e8400-e29b-41d4-a716-446655440004"`
	Name      string     `json:"name" example:"Network Discovery"`
	Network   string     `json:"network" example:"192.168.1.0/24"`
	Method    string     `json:"method" example:"tcp" enums:"tcp,icmp,arp"`
	Status    string     `json:"status" example:"running" enums:"pending,running,completed,failed"`
	Progress  float64    `json:"progress" example:"45.5"`
	CreatedAt time.Time  `json:"created_at"`
	StartedAt *time.Time `json:"started_at,omitempty"`
}

// CreateDiscoveryJobRequest represents a request to create a discovery job
type CreateDiscoveryJobRequest struct {
	Name    string `json:"name" example:"Office Network Discovery"`
	Network string `json:"network" example:"192.168.1.0/24"`
	Method  string `json:"method" example:"tcp" enums:"tcp,icmp,arp"`
}

// ScheduleResponse represents a scheduled scan
type ScheduleResponse struct {
	ID        string     `json:"id" example:"550e8400-e29b-41d4-a716-446655440005"`
	Name      string     `json:"name" example:"Weekly Security Scan"`
	CronExpr  string     `json:"cron_expression" example:"0 2 * * 1"`
	ProfileID string     `json:"profile_id" example:"550e8400-e29b-41d4-a716-446655440001"`
	Targets   []string   `json:"targets" example:"192.168.1.0/24"`
	Enabled   bool       `json:"enabled" example:"true"`
	LastRun   *time.Time `json:"last_run,omitempty"`
	NextRun   *time.Time `json:"next_run,omitempty"`
	CreatedAt time.Time  `json:"created_at"`
	UpdatedAt time.Time  `json:"updated_at"`
}

// CreateScheduleRequest represents a request to create a schedule
type CreateScheduleRequest struct {
	Name      string   `json:"name" example:"Daily Security Scan"`
	CronExpr  string   `json:"cron_expression" example:"0 2 * * *"`
	ProfileID string   `json:"profile_id" example:"550e8400-e29b-41d4-a716-446655440001"`
	Targets   []string `json:"targets" example:"192.168.1.0/24"`
	Enabled   bool     `json:"enabled" example:"true"`
}

// PaginationInfo represents pagination metadata
type PaginationInfo struct {
	Page       int `json:"page" example:"1"`
	PageSize   int `json:"page_size" example:"20"`
	TotalItems int `json:"total_items" example:"150"`
	TotalPages int `json:"total_pages" example:"8"`
}

// PaginatedScansResponse represents a paginated list of scans
type PaginatedScansResponse struct {
	Data       []ScanResponse `json:"data"`
	Pagination PaginationInfo `json:"pagination"`
}

// PaginatedHostsResponse represents a paginated list of hosts
type PaginatedHostsResponse struct {
	Data       []HostResponse `json:"data"`
	Pagination PaginationInfo `json:"pagination"`
}

// PaginatedProfilesResponse represents a paginated list of profiles
type PaginatedProfilesResponse struct {
	Data       []ProfileResponse `json:"data"`
	Pagination PaginationInfo    `json:"pagination"`
}

// PaginatedDiscoveryJobsResponse represents a paginated list of discovery jobs
type PaginatedDiscoveryJobsResponse struct {
	Data       []DiscoveryJobResponse `json:"data"`
	Pagination PaginationInfo         `json:"pagination"`
}

// PaginatedSchedulesResponse represents a paginated list of schedules
type PaginatedSchedulesResponse struct {
	Data       []ScheduleResponse `json:"data"`
	Pagination PaginationInfo     `json:"pagination"`
}

// Health godoc
// @Summary Health check
// @Description Returns service health status including database connectivity
// @Tags System
// @Produce json
// @Success 200 {object} HealthResponse
// @Success 503 {object} HealthResponse
// @Failure 429 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /health [get]
// @ID getHealth
func Health(_ http.ResponseWriter, _ *http.Request) {}

// Status godoc
// @Summary System status
// @Description Returns detailed system status information
// @Tags System
// @Produce json
// @Success 200 {object} StatusResponse
// @Failure 429 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /status [get]
// @ID getStatus
func Status(_ http.ResponseWriter, _ *http.Request) {}

// Version godoc
// @Summary Version information
// @Description Returns version and build information
// @Tags System
// @Produce json
// @Success 200 {object} VersionResponse
// @Failure 429 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /version [get]
// @ID getVersion
func Version(_ http.ResponseWriter, _ *http.Request) {}

// Metrics godoc
// @Summary Application metrics
// @Description Returns Prometheus metrics for monitoring
// @Tags System
// @Produce text/plain
// @Success 200 {string} string
// @Failure 404 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /metrics [get]
// @ID getMetrics
func Metrics(_ http.ResponseWriter, _ *http.Request) {}

// AdminStatus godoc
// @Summary Admin status
// @Description Returns administrative status information
// @Tags Admin
// @Produce json
// @Security ApiKeyAuth
// @Success 200 {object} AdminStatusResponse
// @Failure 401 {object} ErrorResponse
// @Failure 403 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /admin/status [get]
// @ID getAdminStatus
func AdminStatus(_ http.ResponseWriter, _ *http.Request) {}

// ListScans godoc
// @Summary List scans
// @Description Get paginated list of scans with optional filtering
// @Tags Scans
// @Produce json
// @Param page query int false "Page number" default(1)
// @Param page_size query int false "Items per page" default(20)
// @Param status query string false "Filter by status" Enums(pending,running,completed,failed,cancelled)
// @Param target query string false "Filter by target"
// @Success 200 {object} PaginatedScansResponse
// @Failure 400 {object} ErrorResponse
// @Failure 401 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Security ApiKeyAuth
// @Router /scans [get]
// @ID listScans
func ListScans(_ http.ResponseWriter, _ *http.Request) {}

// CreateScan godoc
// @Summary Create scan
// @Description Create a new network scan job
// @Tags Scans
// @Accept json
// @Produce json
// @Param scan body CreateScanRequest true "Scan configuration"
// @Success 201 {object} ScanResponse
// @Failure 400 {object} ErrorResponse
// @Failure 401 {object} ErrorResponse
// @Failure 422 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Security ApiKeyAuth
// @Router /scans [post]
// @ID createScan
func CreateScan(_ http.ResponseWriter, _ *http.Request) {}

// GetScan godoc
// @Summary Get scan
// @Description Get scan details by ID
// @Tags Scans
// @Produce json
// @Param scanId path string true "Scan ID" format(uuid)
// @Success 200 {object} ScanResponse
// @Failure 400 {object} ErrorResponse
// @Failure 401 {object} ErrorResponse
// @Failure 404 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Security ApiKeyAuth
// @Router /scans/{scanId} [get]
// @ID getScan
func GetScan(_ http.ResponseWriter, _ *http.Request) {}

// DeleteScan godoc
// @Summary Delete scan
// @Description Cancel running scan or delete completed scan
// @Tags Scans
// @Param scanId path string true "Scan ID" format(uuid)
// @Success 204 "Successfully deleted"
// @Failure 400 {object} ErrorResponse
// @Failure 401 {object} ErrorResponse
// @Failure 404 {object} ErrorResponse
// @Failure 409 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Security ApiKeyAuth
// @Router /scans/{scanId} [delete]
// @ID deleteScan
func DeleteScan(_ http.ResponseWriter, _ *http.Request) {}

// ListHosts godoc
// @Summary List hosts
// @Description Get paginated list of discovered hosts with optional filtering
// @Tags Hosts
// @Produce json
// @Param page query int false "Page number" default(1)
// @Param page_size query int false "Items per page" default(20)
// @Param ip_address query string false "Filter by IP address"
// @Param hostname query string false "Filter by hostname"
// @Param status query string false "Filter by status" Enums(up,down,unknown)
// @Success 200 {object} PaginatedHostsResponse
// @Failure 400 {object} ErrorResponse
// @Failure 401 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Security ApiKeyAuth
// @Router /hosts [get]
// @ID listHosts
func ListHosts(_ http.ResponseWriter, _ *http.Request) {}

// CreateHost godoc
// @Summary Create host
// @Description Manually add a host to the inventory
// @Tags Hosts
// @Accept json
// @Produce json
// @Param host body HostResponse true "Host information"
// @Success 201 {object} HostResponse
// @Failure 400 {object} ErrorResponse
// @Failure 401 {object} ErrorResponse
// @Failure 409 {object} ErrorResponse
// @Failure 422 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Security ApiKeyAuth
// @Router /hosts [post]
// @ID createHost
func CreateHost(_ http.ResponseWriter, _ *http.Request) {}

// GetHost godoc
// @Summary Get host
// @Description Get host details by ID
// @Tags Hosts
// @Produce json
// @Param hostId path string true "Host ID" format(uuid)
// @Success 200 {object} HostResponse
// @Failure 400 {object} ErrorResponse
// @Failure 401 {object} ErrorResponse
// @Failure 404 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Security ApiKeyAuth
// @Router /hosts/{hostId} [get]
// @ID getHost
func GetHost(_ http.ResponseWriter, _ *http.Request) {}

// UpdateHost godoc
// @Summary Update host
// @Description Update host information
// @Tags Hosts
// @Accept json
// @Produce json
// @Param hostId path string true "Host ID" format(uuid)
// @Param host body HostResponse true "Updated host information"
// @Success 200 {object} HostResponse
// @Failure 400 {object} ErrorResponse
// @Failure 401 {object} ErrorResponse
// @Failure 404 {object} ErrorResponse
// @Failure 422 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Security ApiKeyAuth
// @Router /hosts/{hostId} [put]
// @ID updateHost
func UpdateHost(_ http.ResponseWriter, _ *http.Request) {}

// DeleteHost godoc
// @Summary Delete host
// @Description Remove host from inventory
// @Tags Hosts
// @Param hostId path string true "Host ID" format(uuid)
// @Success 204 "Successfully deleted"
// @Failure 400 {object} ErrorResponse
// @Failure 401 {object} ErrorResponse
// @Failure 404 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Security ApiKeyAuth
// @Router /hosts/{hostId} [delete]
// @ID deleteHost
func DeleteHost(_ http.ResponseWriter, _ *http.Request) {}

// GetHostScans godoc
// @Summary Get host scans
// @Description Get scans associated with a specific host
// @Tags Hosts
// @Produce json
// @Param hostId path string true "Host ID" format(uuid)
// @Param page query int false "Page number" default(1)
// @Param page_size query int false "Items per page" default(20)
// @Success 200 {object} PaginatedScansResponse
// @Failure 400 {object} ErrorResponse
// @Failure 401 {object} ErrorResponse
// @Failure 404 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Security ApiKeyAuth
// @Router /hosts/{hostId}/scans [get]
// @ID getHostScans
func GetHostScans(_ http.ResponseWriter, _ *http.Request) {}

// ListProfiles godoc
// @Summary List profiles
// @Description Get paginated list of scan profiles
// @Tags Profiles
// @Produce json
// @Param page query int false "Page number" default(1)
// @Param page_size query int false "Items per page" default(20)
// @Param scan_type query string false "Filter by scan type"
// @Success 200 {object} PaginatedProfilesResponse
// @Failure 400 {object} ErrorResponse
// @Failure 401 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Security ApiKeyAuth
// @Router /profiles [get]
// @ID listProfiles
func ListProfiles(_ http.ResponseWriter, _ *http.Request) {}

// CreateProfile godoc
// @Summary Create profile
// @Description Create a new scan profile
// @Tags Profiles
// @Accept json
// @Produce json
// @Param profile body CreateProfileRequest true "Profile configuration"
// @Success 201 {object} ProfileResponse
// @Failure 400 {object} ErrorResponse
// @Failure 401 {object} ErrorResponse
// @Failure 409 {object} ErrorResponse
// @Failure 422 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Security ApiKeyAuth
// @Router /profiles [post]
// @ID createProfile
func CreateProfile(_ http.ResponseWriter, _ *http.Request) {}

// GetProfile godoc
// @Summary Get profile
// @Description Get scan profile details by ID
// @Tags Profiles
// @Produce json
// @Param profileId path string true "Profile ID" format(uuid)
// @Success 200 {object} ProfileResponse
// @Failure 400 {object} ErrorResponse
// @Failure 401 {object} ErrorResponse
// @Failure 404 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Security ApiKeyAuth
// @Router /profiles/{profileId} [get]
// @ID getProfile
func GetProfile(_ http.ResponseWriter, _ *http.Request) {}

// UpdateProfile godoc
// @Summary Update profile
// @Description Update scan profile configuration
// @Tags Profiles
// @Accept json
// @Produce json
// @Param profileId path string true "Profile ID" format(uuid)
// @Param profile body CreateProfileRequest true "Updated profile configuration"
// @Success 200 {object} ProfileResponse
// @Failure 400 {object} ErrorResponse
// @Failure 401 {object} ErrorResponse
// @Failure 404 {object} ErrorResponse
// @Failure 422 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Security ApiKeyAuth
// @Router /profiles/{profileId} [put]
// @ID updateProfile
func UpdateProfile(_ http.ResponseWriter, _ *http.Request) {}

// DeleteProfile godoc
// @Summary Delete profile
// @Description Delete scan profile
// @Tags Profiles
// @Param profileId path string true "Profile ID" format(uuid)
// @Success 204 "Successfully deleted"
// @Failure 400 {object} ErrorResponse
// @Failure 401 {object} ErrorResponse
// @Failure 404 {object} ErrorResponse
// @Failure 409 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Security ApiKeyAuth
// @Router /profiles/{profileId} [delete]
// @ID deleteProfile
func DeleteProfile(_ http.ResponseWriter, _ *http.Request) {}

// ListDiscoveryJobs godoc
// @Summary List discovery jobs
// @Description Get paginated list of discovery jobs
// @Tags Discovery
// @Produce json
// @Param page query int false "Page number" default(1)
// @Param page_size query int false "Items per page" default(20)
// @Param status query string false "Filter by status" Enums(pending,running,completed,failed)
// @Success 200 {object} PaginatedDiscoveryJobsResponse
// @Failure 400 {object} ErrorResponse
// @Failure 401 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Security ApiKeyAuth
// @Router /discovery [get]
// @ID listDiscoveryJobs
func ListDiscoveryJobs(_ http.ResponseWriter, _ *http.Request) {}

// CreateDiscoveryJob godoc
// @Summary Create discovery job
// @Description Create a new network discovery job
// @Tags Discovery
// @Accept json
// @Produce json
// @Param discovery body CreateDiscoveryJobRequest true "Discovery job configuration"
// @Success 201 {object} DiscoveryJobResponse
// @Failure 400 {object} ErrorResponse
// @Failure 401 {object} ErrorResponse
// @Failure 422 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Security ApiKeyAuth
// @Router /discovery [post]
// @ID createDiscoveryJob
func CreateDiscoveryJob(_ http.ResponseWriter, _ *http.Request) {}

// GetDiscoveryJob godoc
// @Summary Get discovery job
// @Description Get discovery job details by ID
// @Tags Discovery
// @Produce json
// @Param discoveryId path string true "Discovery Job ID" format(uuid)
// @Success 200 {object} DiscoveryJobResponse
// @Failure 400 {object} ErrorResponse
// @Failure 401 {object} ErrorResponse
// @Failure 404 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Security ApiKeyAuth
// @Router /discovery/{discoveryId} [get]
// @ID getDiscoveryJob
func GetDiscoveryJob(_ http.ResponseWriter, _ *http.Request) {}

// StartDiscovery godoc
// @Summary Start discovery
// @Description Start a discovery job
// @Tags Discovery
// @Produce json
// @Param discoveryId path string true "Discovery Job ID" format(uuid)
// @Success 200 {object} DiscoveryJobResponse
// @Failure 400 {object} ErrorResponse
// @Failure 401 {object} ErrorResponse
// @Failure 404 {object} ErrorResponse
// @Failure 409 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Security ApiKeyAuth
// @Router /discovery/{discoveryId}/start [post]
// @ID startDiscovery
func StartDiscovery(_ http.ResponseWriter, _ *http.Request) {}

// StopDiscovery godoc
// @Summary Stop discovery
// @Description Stop a running discovery job
// @Tags Discovery
// @Produce json
// @Param discoveryId path string true "Discovery Job ID" format(uuid)
// @Success 200 {object} DiscoveryJobResponse
// @Failure 400 {object} ErrorResponse
// @Failure 401 {object} ErrorResponse
// @Failure 404 {object} ErrorResponse
// @Failure 409 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Security ApiKeyAuth
// @Router /discovery/{discoveryId}/stop [post]
// @ID stopDiscovery
func StopDiscovery(_ http.ResponseWriter, _ *http.Request) {}

// ListSchedules godoc
// @Summary List schedules
// @Description Get paginated list of scheduled scans
// @Tags Schedules
// @Produce json
// @Param page query int false "Page number" default(1)
// @Param page_size query int false "Items per page" default(20)
// @Param enabled query boolean false "Filter by enabled status"
// @Success 200 {object} PaginatedSchedulesResponse
// @Failure 400 {object} ErrorResponse
// @Failure 401 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Security ApiKeyAuth
// @Router /schedules [get]
// @ID listSchedules
func ListSchedules(_ http.ResponseWriter, _ *http.Request) {}

// CreateSchedule godoc
// @Summary Create schedule
// @Description Create a new scheduled scan
// @Tags Schedules
// @Accept json
// @Produce json
// @Param schedule body CreateScheduleRequest true "Schedule configuration"
// @Success 201 {object} ScheduleResponse
// @Failure 400 {object} ErrorResponse
// @Failure 401 {object} ErrorResponse
// @Failure 422 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Security ApiKeyAuth
// @Router /schedules [post]
// @ID createSchedule
func CreateSchedule(_ http.ResponseWriter, _ *http.Request) {}

// GetSchedule godoc
// @Summary Get schedule
// @Description Get schedule details by ID
// @Tags Schedules
// @Produce json
// @Param scheduleId path string true "Schedule ID" format(uuid)
// @Success 200 {object} ScheduleResponse
// @Failure 400 {object} ErrorResponse
// @Failure 401 {object} ErrorResponse
// @Failure 404 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Security ApiKeyAuth
// @Router /schedules/{scheduleId} [get]
// @ID getSchedule
func GetSchedule(_ http.ResponseWriter, _ *http.Request) {}

// UpdateSchedule godoc
// @Summary Update schedule
// @Description Update schedule configuration
// @Tags Schedules
// @Accept json
// @Produce json
// @Param scheduleId path string true "Schedule ID" format(uuid)
// @Param schedule body CreateScheduleRequest true "Updated schedule configuration"
// @Success 200 {object} ScheduleResponse
// @Failure 400 {object} ErrorResponse
// @Failure 401 {object} ErrorResponse
// @Failure 404 {object} ErrorResponse
// @Failure 422 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Security ApiKeyAuth
// @Router /schedules/{scheduleId} [put]
// @ID updateSchedule
func UpdateSchedule(_ http.ResponseWriter, _ *http.Request) {}

// DeleteSchedule godoc
// @Summary Delete schedule
// @Description Delete scheduled scan
// @Tags Schedules
// @Param scheduleId path string true "Schedule ID" format(uuid)
// @Success 204 "Successfully deleted"
// @Failure 400 {object} ErrorResponse
// @Failure 401 {object} ErrorResponse
// @Failure 404 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Security ApiKeyAuth
// @Router /schedules/{scheduleId} [delete]
// @ID deleteSchedule
func DeleteSchedule(_ http.ResponseWriter, _ *http.Request) {}

// EnableSchedule godoc
// @Summary Enable schedule
// @Description Enable a scheduled scan
// @Tags Schedules
// @Produce json
// @Param scheduleId path string true "Schedule ID" format(uuid)
// @Success 200 {object} ScheduleResponse
// @Failure 400 {object} ErrorResponse
// @Failure 401 {object} ErrorResponse
// @Failure 404 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Security ApiKeyAuth
// @Router /schedules/{scheduleId}/enable [post]
// @ID enableSchedule
func EnableSchedule(_ http.ResponseWriter, _ *http.Request) {}

// DisableSchedule godoc
// @Summary Disable schedule
// @Description Disable a scheduled scan
// @Tags Schedules
// @Produce json
// @Param scheduleId path string true "Schedule ID" format(uuid)
// @Success 200 {object} ScheduleResponse
// @Failure 400 {object} ErrorResponse
// @Failure 401 {object} ErrorResponse
// @Failure 404 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Security ApiKeyAuth
// @Router /schedules/{scheduleId}/disable [post]
// @ID disableSchedule
func DisableSchedule(_ http.ResponseWriter, _ *http.Request) {}

// GetScanResults godoc
// @Summary Get scan results
// @Description Get detailed results from a completed scan
// @Tags Scans
// @Produce json
// @Param scanId path string true "Scan ID" format(uuid)
// @Success 200 {object} map[string]interface{}
// @Failure 400 {object} ErrorResponse
// @Failure 401 {object} ErrorResponse
// @Failure 404 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Security ApiKeyAuth
// @Router /scans/{scanId}/results [get]
// @ID getScanResults
func GetScanResults(_ http.ResponseWriter, _ *http.Request) {}

// StartScan godoc
// @Summary Start scan
// @Description Start a pending scan
// @Tags Scans
// @Produce json
// @Param scanId path string true "Scan ID" format(uuid)
// @Success 200 {object} ScanResponse
// @Failure 400 {object} ErrorResponse
// @Failure 401 {object} ErrorResponse
// @Failure 404 {object} ErrorResponse
// @Failure 409 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Security ApiKeyAuth
// @Router /scans/{scanId}/start [post]
// @ID startScan
func StartScan(_ http.ResponseWriter, _ *http.Request) {}

// StopScan godoc
// @Summary Stop scan
// @Description Stop a running scan
// @Tags Scans
// @Produce json
// @Param scanId path string true "Scan ID" format(uuid)
// @Success 200 {object} ScanResponse
// @Failure 400 {object} ErrorResponse
// @Failure 401 {object} ErrorResponse
// @Failure 404 {object} ErrorResponse
// @Failure 409 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Security ApiKeyAuth
// @Router /scans/{scanId}/stop [post]
// @ID stopScan
func StopScan(_ http.ResponseWriter, _ *http.Request) {}
