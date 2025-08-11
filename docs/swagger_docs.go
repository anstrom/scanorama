// Package docs provides Swagger documentation for the Scanorama API.
//
// This file contains all API endpoint documentation using swaggo annotations.
// Run `swag init` to generate OpenAPI specification files.
//
// LINTING EXCLUSIONS:
// This file and generated swagger files (docs/swagger/docs.go) are excluded from
// many linters in .golangci.yml because:
// - Generated code may have long lines, high complexity, and duplication
// - Documentation functions are placeholders and may appear unused
// - Swagger annotations require specific formatting that linters may flag
// - Example data in annotations may contain "magic numbers" and repeated strings
//
//go:generate swag init -g swagger_docs.go -o ./swagger --parseDependency --parseInternal
package docs

import (
	"net/http"
	"time"
)

// @title Scanorama API
// @version 1.0.0
// @description Network scanning and discovery service with automated reconnaissance capabilities
// @description
// @description ## Features
// @description - Network host discovery and enumeration
// @description - Port scanning and service detection
// @description - Scan scheduling and automation
// @description - Real-time progress updates via WebSocket
// @description - Administrative monitoring and controls
// @description
// @description ## Authentication
// @description API key authentication can be enabled. Include your API key in the `X-API-Key` header when required.
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
	Version   string    `json:"version" example:"0.2.0"`
	Timestamp time.Time `json:"timestamp"`
	Uptime    string    `json:"uptime" example:"2h30m45s"`
}

// VersionResponse represents version information
type VersionResponse struct {
	Version   string    `json:"version" example:"0.2.0"`
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

// Health godoc
// @Summary Health check
// @Description Returns service health status
// @Tags System
// @Produce json
// @Success 200 {object} HealthResponse
// @Success 503 {object} HealthResponse
// @Router /health [get]
func Health(w http.ResponseWriter, r *http.Request) {}

// Status godoc
// @Summary System status
// @Description Returns detailed system status
// @Tags System
// @Produce json
// @Success 200 {object} StatusResponse
// @Router /status [get]
func Status(w http.ResponseWriter, r *http.Request) {}

// Version godoc
// @Summary Version information
// @Description Returns version and build info
// @Tags System
// @Produce json
// @Success 200 {object} VersionResponse
// @Router /version [get]
func Version(w http.ResponseWriter, r *http.Request) {}

// Metrics godoc
// @Summary Application metrics
// @Description Returns Prometheus metrics
// @Tags System
// @Produce text/plain
// @Success 200 {string} string
// @Failure 404 {object} ErrorResponse
// @Router /metrics [get]
func Metrics(w http.ResponseWriter, r *http.Request) {}

// AdminStatus godoc
// @Summary Admin status
// @Description Returns admin status info
// @Tags Admin
// @Produce json
// @Security ApiKeyAuth
// @Success 200 {object} AdminStatusResponse
// @Failure 401 {object} ErrorResponse
// @Failure 403 {object} ErrorResponse
// @Router /admin/status [get]
func AdminStatus(w http.ResponseWriter, r *http.Request) {}

// ListScans godoc
// @Summary List scans
// @Description Get paginated list of scans
// @Tags Scans
// @Produce json
// @Param page query int false "Page number" default(1)
// @Param page_size query int false "Items per page" default(20)
// @Param status query string false "Filter by status" Enums(pending,running,completed,failed,cancelled)
// @Param target query string false "Filter by target"
// @Success 200 {object} PaginatedScansResponse
// @Failure 400 {object} ErrorResponse
// @Failure 401 {object} ErrorResponse
// @Security ApiKeyAuth
// @Router /scans [get]
func ListScans(w http.ResponseWriter, r *http.Request) {}

// CreateScan godoc
// @Summary Create scan
// @Description Create new network scan
// @Tags Scans
// @Accept json
// @Produce json
// @Param scan body CreateScanRequest true "Scan config"
// @Success 201 {object} ScanResponse
// @Failure 400 {object} ErrorResponse
// @Failure 401 {object} ErrorResponse
// @Security ApiKeyAuth
// @Router /scans [post]
func CreateScan(w http.ResponseWriter, r *http.Request) {}

// GetScan godoc
// @Summary Get scan
// @Description Get scan details by ID
// @Tags Scans
// @Produce json
// @Param scanId path string true "Scan ID" format(uuid)
// @Success 200 {object} ScanResponse
// @Failure 404 {object} ErrorResponse
// @Failure 401 {object} ErrorResponse
// @Security ApiKeyAuth
// @Router /scans/{scanId} [get]
func GetScan(w http.ResponseWriter, r *http.Request) {}

// DeleteScan godoc
// @Summary Delete scan
// @Description Cancel or delete scan
// @Tags Scans
// @Param scanId path string true "Scan ID" format(uuid)
// @Success 204 "Deleted"
// @Failure 404 {object} ErrorResponse
// @Failure 401 {object} ErrorResponse
// @Failure 409 {object} ErrorResponse
// @Security ApiKeyAuth
// @Router /scans/{scanId} [delete]
func DeleteScan(w http.ResponseWriter, r *http.Request) {}

// ListHosts godoc
// @Summary List hosts
// @Description Get discovered hosts
// @Tags Hosts
// @Produce json
// @Param page query int false "Page number" default(1)
// @Param page_size query int false "Items per page" default(20)
// @Param ip_address query string false "Filter by IP"
// @Param hostname query string false "Filter by hostname"
// @Success 200 {object} PaginatedHostsResponse
// @Failure 400 {object} ErrorResponse
// @Failure 401 {object} ErrorResponse
// @Security ApiKeyAuth
// @Router /hosts [get]
func ListHosts(w http.ResponseWriter, r *http.Request) {}

// GetHost godoc
// @Summary Get host
// @Description Get host details by ID
// @Tags Hosts
// @Produce json
// @Param hostId path string true "Host ID" format(uuid)
// @Success 200 {object} HostResponse
// @Failure 404 {object} ErrorResponse
// @Failure 401 {object} ErrorResponse
// @Security ApiKeyAuth
// @Router /hosts/{hostId} [get]
func GetHost(w http.ResponseWriter, r *http.Request) {}

// NotImplemented godoc
// @Summary Not implemented
// @Description Endpoint not yet implemented
// @Tags System
// @Produce json
// @Success 501 {object} ErrorResponse
// @Router /discovery [get]
// @Router /discovery [post]
// @Router /profiles [get]
// @Router /profiles [post]
// @Router /schedules [get]
// @Router /schedules [post]
func NotImplemented(w http.ResponseWriter, r *http.Request) {}
