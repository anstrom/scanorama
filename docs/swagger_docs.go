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
	ID           string            `json:"id" example:"550e8400-e29b-41d4-a716-446655440000"`
	Name         string            `json:"name" example:"Ad-hoc scan: 192.168.1.0/24"`
	Description  string            `json:"description,omitempty"`
	ProfileID    *string           `json:"profile_id,omitempty" example:"550e8400-e29b-41d4-a716-446655440001"`
	ScanType     string            `json:"scan_type" example:"connect" enums:"connect,syn,ack,udp,aggressive,comprehensive"`
	Ports        string            `json:"ports,omitempty" example:"22,80,443"`
	Targets      []string          `json:"targets" example:"192.168.1.0/24,myserver.local"`
	Options      map[string]string `json:"options,omitempty"`
	Tags         []string          `json:"tags,omitempty"`
	Status       string            `json:"status" example:"running" enums:"pending,running,completed,failed"`
	Progress     float64           `json:"progress" example:"65.5"`
	CreatedAt    time.Time         `json:"created_at"`
	UpdatedAt    time.Time         `json:"updated_at"`
	StartedAt    *time.Time        `json:"started_at,omitempty"`
	CompletedAt  *time.Time        `json:"completed_at,omitempty"`
	Duration     *string           `json:"duration,omitempty" example:"14m30s"`
	PortsScanned *string           `json:"ports_scanned,omitempty" example:"443 open / 1200 total"`
	ErrorMessage *string           `json:"error_message,omitempty"`
	CreatedBy    string            `json:"created_by,omitempty"`
}

// CreateScanRequest represents a request to create a new scan
type CreateScanRequest struct {
	Name        string            `json:"name" example:"Weekly security scan"`
	Targets     []string          `json:"targets" example:"192.168.1.0/24"`
	ScanType    string            `json:"scan_type" example:"connect" enums:"connect,syn,ack,udp,aggressive,comprehensive"`
	ProfileID   *string           `json:"profile_id,omitempty" example:"550e8400-e29b-41d4-a716-446655440001"`
	Description string            `json:"description,omitempty" example:"Regular security assessment"`
	Ports       string            `json:"ports,omitempty" example:"22,80,443"`
	Options     map[string]string `json:"options,omitempty"`
	OSDetection bool              `json:"os_detection,omitempty" example:"false"`
	Tags        []string          `json:"tags,omitempty"`
}

// DNSRecordResponse represents a single DNS record for a host.
type DNSRecordResponse struct {
	ID         string    `json:"id"`
	HostID     string    `json:"host_id"`
	RecordType string    `json:"record_type" example:"PTR" enums:"A,AAAA,PTR,CNAME,MX,TXT,SRV"`
	Value      string    `json:"value" example:"server01.local"`
	TTL        *int      `json:"ttl,omitempty" example:"300"`
	ResolvedAt time.Time `json:"resolved_at"`
}

// PortBannerResponse represents a service banner captured on a host port.
type PortBannerResponse struct {
	ID        string    `json:"id"`
	HostID    string    `json:"host_id"`
	Port      int       `json:"port" example:"80"`
	Protocol  string    `json:"protocol" example:"tcp" enums:"tcp,udp"`
	RawBanner *string   `json:"raw_banner,omitempty" example:"HTTP/1.1 200 OK"`
	Service   *string   `json:"service,omitempty" example:"nginx"`
	Version   *string   `json:"version,omitempty" example:"1.24.0"`
	ScannedAt time.Time `json:"scanned_at"`
}

// CertificateResponse represents a TLS certificate captured from a host.
type CertificateResponse struct {
	ID        string    `json:"id"`
	HostID    string    `json:"host_id"`
	Port      int       `json:"port" example:"443"`
	SubjectCN *string   `json:"subject_cn,omitempty" example:"server01.local"`
	SANs      []string  `json:"sans,omitempty" example:"server01.local,www.server01.local"`
	Issuer    *string   `json:"issuer,omitempty" example:"Let's Encrypt Authority X3"`
	NotBefore *string   `json:"not_before,omitempty"`
	NotAfter  *string   `json:"not_after,omitempty"`
	KeyType   *string   `json:"key_type,omitempty" example:"RSA-2048"`
	TLSVer    *string   `json:"tls_version,omitempty" example:"TLS 1.3"`
	ScannedAt time.Time `json:"scanned_at"`
}

// SNMPInterfaceResponse represents a single network interface reported by SNMP.
type SNMPInterfaceResponse struct {
	Index       int     `json:"index" example:"1"`
	Name        string  `json:"name" example:"eth0"`
	AdminStatus string  `json:"admin_status,omitempty" example:"up" enums:"up,down,testing,unknown,dormant,notPresent,lowerLayerDown"`
	Status      string  `json:"status" example:"up" enums:"up,down,testing,unknown,dormant,notPresent,lowerLayerDown"`
	SpeedMbps   float64 `json:"speed_mbps" example:"1000"`
	MAC         string  `json:"mac,omitempty" example:"00:1B:44:11:3A:B7"`
	RxBytes     uint64  `json:"rx_bytes,omitempty" example:"1048576"`
	TxBytes     uint64  `json:"tx_bytes,omitempty" example:"524288"`
}

// SNMPDataResponse represents SNMP device information for a host.
type SNMPDataResponse struct {
	SysName     *string                 `json:"sys_name,omitempty" example:"router01"`
	SysDescr    *string                 `json:"sys_descr,omitempty" example:"Cisco IOS XE"`
	SysLocation *string                 `json:"sys_location,omitempty" example:"Server Room A"`
	SysContact  *string                 `json:"sys_contact,omitempty" example:"admin@example.com"`
	SysUptime   *int64                  `json:"sys_uptime_cs,omitempty" example:"123456789"`
	IfCount     *int                    `json:"if_count,omitempty" example:"4"`
	Interfaces  []SNMPInterfaceResponse `json:"interfaces,omitempty"`
	CollectedAt time.Time               `json:"collected_at"`
}

// HostResponse represents a discovered host
type HostResponse struct {
	ID          string  `json:"id" example:"550e8400-e29b-41d4-a716-446655440002"`
	IPAddress   string  `json:"ip_address" example:"192.168.1.100"`
	Hostname    *string `json:"hostname,omitempty" example:"server01.local"`
	Description *string `json:"description,omitempty" example:"Primary web server"`
	MACAddress  *string `json:"mac_address,omitempty" example:"00:1B:44:11:3A:B7"`
	Vendor      *string `json:"vendor,omitempty" example:"Dell Inc."`
	// OSFamily is the broad OS family detected by nmap (e.g. "Linux", "Windows").
	OSFamily *string `json:"os_family,omitempty" example:"Linux"`
	// OSName is the full OS name returned by nmap (e.g. "Linux 5.15").
	OSName    *string  `json:"os_name,omitempty" example:"Ubuntu 22.04"`
	OSVersion *string  `json:"os_version,omitempty" example:"22.04"`
	OpenPorts []int    `json:"open_ports" example:"22,80,443"`
	Tags      []string `json:"tags,omitempty" example:"web,production"`
	// NetworkID is the network this host belongs to, if any.
	NetworkID         *string   `json:"network_id,omitempty"`
	Status            string    `json:"status" example:"up" enums:"up,down,unknown"`
	ResponseTimeMs    *float64  `json:"response_time_ms,omitempty" example:"1.23"`
	ResponseTimeAvgMs *float64  `json:"response_time_avg_ms,omitempty" example:"1.5"`
	LastSeen          time.Time `json:"last_seen"`
	FirstSeen         time.Time `json:"first_seen"`
	ScanCount         int       `json:"scan_count" example:"5"`
	// DNSRecords are populated when DNS enrichment has run for this host.
	DNSRecords []DNSRecordResponse `json:"dns_records,omitempty"`
	// Banners are populated when banner grabbing has run for this host.
	Banners []PortBannerResponse `json:"banners,omitempty"`
	// Certificates are populated when TLS banner grabbing has run for this host.
	Certificates []CertificateResponse `json:"certificates,omitempty"`
	// SNMPData is populated when SNMP enrichment has run for this host.
	SNMPData *SNMPDataResponse `json:"snmp_data,omitempty"`
	// KnowledgeScore is a 0-100 integer indicating how much is known about this host.
	KnowledgeScore int `json:"knowledge_score" example:"60"`
}

// ProfileResponse represents a scan profile
type ProfileResponse struct {
	ID          string            `json:"id" example:"550e8400-e29b-41d4-a716-446655440003"`
	Name        string            `json:"name" example:"Quick Connect Scan"`
	Description string            `json:"description,omitempty" example:"Fast TCP connect scan"`
	ScanType    string            `json:"scan_type" example:"connect" enums:"connect,syn,ack,udp,aggressive,comprehensive"`
	Ports       string            `json:"ports,omitempty" example:"22,80,443"`
	Options     map[string]string `json:"options,omitempty"`
	CreatedAt   time.Time         `json:"created_at"`
	UpdatedAt   time.Time         `json:"updated_at"`
}

// CreateProfileRequest represents a request to create a scan profile
type CreateProfileRequest struct {
	Name        string            `json:"name" example:"Custom Scan Profile"`
	Description string            `json:"description,omitempty" example:"Custom scan configuration"`
	ScanType    string            `json:"scan_type" example:"connect" enums:"connect,syn,ack,udp,aggressive,comprehensive"`
	Ports       string            `json:"ports,omitempty" example:"22,80,443,8080"`
	Options     map[string]string `json:"options,omitempty"`
}

// DiscoveryJobResponse represents a discovery job
type DiscoveryJobResponse struct {
	ID          string     `json:"id" example:"550e8400-e29b-41d4-a716-446655440004"`
	Name        string     `json:"name" example:"Network Discovery"`
	Description string     `json:"description,omitempty"`
	Networks    []string   `json:"networks" example:"192.168.1.0/24"`
	Method      string     `json:"method" example:"ping" enums:"ping,arp,icmp,tcp_connect,dns"`
	Status      string     `json:"status" example:"running" enums:"pending,running,completed,failed"`
	Progress    float64    `json:"progress" example:"45.5"`
	HostsFound  int        `json:"hosts_found" example:"12"`
	Enabled     bool       `json:"enabled" example:"true"`
	CreatedAt   time.Time  `json:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at"`
	StartedAt   *time.Time `json:"started_at,omitempty"`
	CompletedAt *time.Time `json:"completed_at,omitempty"`
	LastRun     *time.Time `json:"last_run,omitempty"`
	NextRun     *time.Time `json:"next_run,omitempty"`
	LastError   string     `json:"last_error,omitempty"`
	CreatedBy   string     `json:"created_by,omitempty"`
}

// CreateDiscoveryJobRequest represents a request to create a discovery job
type CreateDiscoveryJobRequest struct {
	Name        string   `json:"name" example:"Office Network Discovery"`
	Networks    []string `json:"networks" example:"192.168.1.0/24"`
	Method      string   `json:"method" example:"ping" enums:"ping,arp,icmp,tcp_connect,dns"`
	Description string   `json:"description,omitempty"`
	Enabled     bool     `json:"enabled" example:"true"`
}

// ScheduleResponse represents a scheduled scan
type ScheduleResponse struct {
	ID          string     `json:"id" example:"550e8400-e29b-41d4-a716-446655440005"`
	Name        string     `json:"name" example:"Weekly Security Scan"`
	Description string     `json:"description,omitempty"`
	CronExpr    string     `json:"cron_expr" example:"0 2 * * 1"`
	Type        string     `json:"type" example:"scan" enums:"scan,discovery"`
	NetworkID   string     `json:"network_id,omitempty" example:"550e8400-e29b-41d4-a716-446655440010"`
	NetworkName string     `json:"network_name,omitempty" example:"Office Network"`
	Enabled     bool       `json:"enabled" example:"true"`
	Status      string     `json:"status" example:"active"`
	LastRun     *time.Time `json:"last_run,omitempty"`
	NextRun     *time.Time `json:"next_run,omitempty"`
	RunCount    int        `json:"run_count" example:"5"`
	ErrorCount  int        `json:"error_count" example:"0"`
	LastError   string     `json:"last_error,omitempty"`
	CreatedAt   time.Time  `json:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at"`
	CreatedBy   string     `json:"created_by,omitempty"`
}

// CreateScheduleRequest represents a request to create a schedule
type CreateScheduleRequest struct {
	Name      string `json:"name" example:"Daily Security Scan"`
	CronExpr  string `json:"cron_expr" example:"0 2 * * *"`
	Type      string `json:"type" example:"scan" enums:"scan,discovery"`
	NetworkID string `json:"network_id" example:"550e8400-e29b-41d4-a716-446655440010"`
	Enabled   bool   `json:"enabled" example:"true"`
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

// NetworkResponse represents a network object
type NetworkResponse struct {
	ID                  string     `json:"id" example:"550e8400-e29b-41d4-a716-446655440010"`
	Name                string     `json:"name" example:"Office Network"`
	CIDR                string     `json:"cidr" example:"192.168.1.0/24"`
	Description         *string    `json:"description,omitempty" example:"Main office network"`
	DiscoveryMethod     string     `json:"discovery_method" example:"ping" enums:"ping,tcp,arp,icmp"`
	IsActive            bool       `json:"is_active" example:"true"`
	ScanEnabled         bool       `json:"scan_enabled" example:"true"`
	ScanIntervalSeconds int        `json:"scan_interval_seconds" example:"3600"`
	ScanPorts           string     `json:"scan_ports" example:"22,80,443,8080"`
	ScanType            string     `json:"scan_type" example:"connect" enums:"connect,syn,ack,udp,aggressive,comprehensive"`
	LastDiscovery       *time.Time `json:"last_discovery,omitempty"`
	LastScan            *time.Time `json:"last_scan,omitempty"`
	HostCount           int        `json:"host_count" example:"25"`
	ActiveHostCount     int        `json:"active_host_count" example:"20"`
	CreatedAt           time.Time  `json:"created_at"`
	UpdatedAt           time.Time  `json:"updated_at"`
	CreatedBy           *string    `json:"created_by,omitempty" example:"admin"`
	ModifiedBy          *string    `json:"modified_by,omitempty" example:"admin"`
}

// CreateNetworkRequest represents a request to create a network
type CreateNetworkRequest struct {
	Name            string  `json:"name" example:"Office Network"`
	CIDR            string  `json:"cidr" example:"192.168.1.0/24"`
	Description     *string `json:"description,omitempty" example:"Main office network"`
	DiscoveryMethod string  `json:"discovery_method" example:"ping" enums:"ping,tcp,arp,icmp"`
	IsActive        *bool   `json:"is_active,omitempty" example:"true"`
	ScanEnabled     *bool   `json:"scan_enabled,omitempty" example:"true"`
}

// UpdateNetworkRequest represents a request to update a network
type UpdateNetworkRequest struct {
	Name            *string `json:"name,omitempty" example:"Office Network"`
	CIDR            *string `json:"cidr,omitempty" example:"192.168.1.0/24"`
	Description     *string `json:"description,omitempty" example:"Main office network"`
	DiscoveryMethod *string `json:"discovery_method,omitempty" example:"ping" enums:"ping,tcp,arp,icmp"`
	IsActive        *bool   `json:"is_active,omitempty" example:"true"`
	ScanEnabled     *bool   `json:"scan_enabled,omitempty" example:"true"`
}

// RenameNetworkRequest represents a request to rename a network
type RenameNetworkRequest struct {
	NewName string `json:"new_name" example:"New Office Network"`
}

// NetworkStatsResponse represents network statistics
type NetworkStatsResponse struct {
	Networks   map[string]interface{} `json:"networks"`
	Hosts      map[string]interface{} `json:"hosts"`
	Exclusions map[string]interface{} `json:"exclusions"`
}

// NetworkExclusionResponse represents a network exclusion rule
type NetworkExclusionResponse struct {
	ID           string    `json:"id" example:"550e8400-e29b-41d4-a716-446655440011"`
	NetworkID    *string   `json:"network_id,omitempty" example:"550e8400-e29b-41d4-a716-446655440010"`
	ExcludedCIDR string    `json:"excluded_cidr" example:"192.168.1.128/25"`
	Reason       *string   `json:"reason,omitempty" example:"Reserved for printers"`
	Enabled      bool      `json:"enabled" example:"true"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
	CreatedBy    *string   `json:"created_by,omitempty" example:"admin"`
}

// CreateExclusionRequest represents a request to create a network exclusion
type CreateExclusionRequest struct {
	ExcludedCIDR string  `json:"excluded_cidr" example:"192.168.1.128/25"`
	Reason       *string `json:"reason,omitempty" example:"Reserved for printers"`
}

// PaginatedNetworksResponse represents a paginated list of networks
type PaginatedNetworksResponse struct {
	Data       []NetworkResponse `json:"data"`
	Pagination PaginationInfo    `json:"pagination"`
}

// LivenessResponse represents a liveness check response
type LivenessResponse struct {
	Status    string    `json:"status" example:"alive"`
	Timestamp time.Time `json:"timestamp"`
	Uptime    string    `json:"uptime" example:"2h30m45s"`
}

// UpdateScanRequest represents a request to update an existing scan
type UpdateScanRequest struct {
	Name        string            `json:"name" example:"Updated scan name"`
	Description string            `json:"description,omitempty" example:"Updated description"`
	Targets     []string          `json:"targets" example:"192.168.1.0/24"`
	ScanType    string            `json:"scan_type" example:"connect" enums:"connect,syn,ack,udp,aggressive,comprehensive"`
	Ports       string            `json:"ports,omitempty" example:"22,80,443"`
	ProfileID   *string           `json:"profile_id,omitempty"`
	Options     map[string]string `json:"options,omitempty"`
	Tags        []string          `json:"tags,omitempty"`
	OSDetection bool              `json:"os_detection,omitempty" example:"false"`
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

// ProfileStatsResponse represents effectiveness statistics for a scan profile.
type ProfileStatsResponse struct {
	ProfileID     string     `json:"profile_id" example:"web-full"`
	TotalScans    int        `json:"total_scans" example:"42"`
	UniqueHosts   int        `json:"unique_hosts" example:"7"`
	LastUsed      *time.Time `json:"last_used,omitempty"`
	AvgHostsFound *float64   `json:"avg_hosts_found,omitempty" example:"12.5"`
}

// GetProfileStats godoc
// @Summary Get profile stats
// @Description Get scan effectiveness statistics for a profile
// @Tags Profiles
// @Produce json
// @Param profileId path string true "Profile ID"
// @Success 200 {object} ProfileStatsResponse
// @Failure 400 {object} ErrorResponse
// @Failure 401 {object} ErrorResponse
// @Failure 404 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Security ApiKeyAuth
// @Router /profiles/{profileId}/stats [get]
// @ID getProfileStats
func GetProfileStats(_ http.ResponseWriter, _ *http.Request) {}

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

// UpdateScan godoc
// @Summary Update scan
// @Description Update an existing scan configuration
// @Tags Scans
// @Accept json
// @Produce json
// @Param scanId path string true "Scan ID" format(uuid)
// @Param scan body UpdateScanRequest true "Updated scan configuration"
// @Success 200 {object} ScanResponse
// @Failure 400 {object} ErrorResponse
// @Failure 401 {object} ErrorResponse
// @Failure 404 {object} ErrorResponse
// @Failure 422 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Security ApiKeyAuth
// @Router /scans/{scanId} [put]
// @ID updateScan
func UpdateScan(_ http.ResponseWriter, _ *http.Request) {}

// Liveness godoc
// @Summary Liveness check
// @Description Returns simple liveness status without dependency checks
// @Tags System
// @Produce json
// @Success 200 {object} LivenessResponse
// @Failure 500 {object} ErrorResponse
// @Router /liveness [get]
// @ID getLiveness
func Liveness(_ http.ResponseWriter, _ *http.Request) {}

// ListNetworks godoc
// @Summary List networks
// @Description Get paginated list of networks with optional filtering
// @Tags Networks
// @Produce json
// @Param page query int false "Page number" default(1)
// @Param page_size query int false "Items per page" default(20)
// @Param show_inactive query boolean false "Include inactive networks"
// @Param name query string false "Filter by network name"
// @Success 200 {object} PaginatedNetworksResponse
// @Failure 400 {object} ErrorResponse
// @Failure 401 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Security ApiKeyAuth
// @Router /networks [get]
// @ID listNetworks
func ListNetworks(_ http.ResponseWriter, _ *http.Request) {}

// CreateNetwork godoc
// @Summary Create network
// @Description Create a new network for scanning and discovery
// @Tags Networks
// @Accept json
// @Produce json
// @Param network body CreateNetworkRequest true "Network configuration"
// @Success 201 {object} NetworkResponse
// @Failure 400 {object} ErrorResponse
// @Failure 401 {object} ErrorResponse
// @Failure 409 {object} ErrorResponse
// @Failure 422 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Security ApiKeyAuth
// @Router /networks [post]
// @ID createNetwork
func CreateNetwork(_ http.ResponseWriter, _ *http.Request) {}

// GetNetworkStats godoc
// @Summary Get network statistics
// @Description Returns aggregate statistics about networks, hosts, and exclusions
// @Tags Networks
// @Produce json
// @Success 200 {object} NetworkStatsResponse
// @Failure 401 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Security ApiKeyAuth
// @Router /networks/stats [get]
// @ID getNetworkStats
func GetNetworkStats(_ http.ResponseWriter, _ *http.Request) {}

// GetNetwork godoc
// @Summary Get network
// @Description Get network details by ID
// @Tags Networks
// @Produce json
// @Param networkId path string true "Network ID" format(uuid)
// @Success 200 {object} NetworkResponse
// @Failure 400 {object} ErrorResponse
// @Failure 401 {object} ErrorResponse
// @Failure 404 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Security ApiKeyAuth
// @Router /networks/{networkId} [get]
// @ID getNetwork
func GetNetwork(_ http.ResponseWriter, _ *http.Request) {}

// UpdateNetwork godoc
// @Summary Update network
// @Description Update network configuration
// @Tags Networks
// @Accept json
// @Produce json
// @Param networkId path string true "Network ID" format(uuid)
// @Param network body UpdateNetworkRequest true "Updated network configuration"
// @Success 200 {object} NetworkResponse
// @Failure 400 {object} ErrorResponse
// @Failure 401 {object} ErrorResponse
// @Failure 404 {object} ErrorResponse
// @Failure 422 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Security ApiKeyAuth
// @Router /networks/{networkId} [put]
// @ID updateNetwork
func UpdateNetwork(_ http.ResponseWriter, _ *http.Request) {}

// DeleteNetwork godoc
// @Summary Delete network
// @Description Delete a network and its associated exclusions
// @Tags Networks
// @Param networkId path string true "Network ID" format(uuid)
// @Success 204 "Successfully deleted"
// @Failure 400 {object} ErrorResponse
// @Failure 401 {object} ErrorResponse
// @Failure 404 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Security ApiKeyAuth
// @Router /networks/{networkId} [delete]
// @ID deleteNetwork
func DeleteNetwork(_ http.ResponseWriter, _ *http.Request) {}

// EnableNetwork godoc
// @Summary Enable network
// @Description Enable a network for scanning and discovery
// @Tags Networks
// @Produce json
// @Param networkId path string true "Network ID" format(uuid)
// @Success 200 {object} NetworkResponse
// @Failure 400 {object} ErrorResponse
// @Failure 401 {object} ErrorResponse
// @Failure 404 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Security ApiKeyAuth
// @Router /networks/{networkId}/enable [post]
// @ID enableNetwork
func EnableNetwork(_ http.ResponseWriter, _ *http.Request) {}

// DisableNetwork godoc
// @Summary Disable network
// @Description Disable a network from scanning and discovery
// @Tags Networks
// @Produce json
// @Param networkId path string true "Network ID" format(uuid)
// @Success 200 {object} NetworkResponse
// @Failure 400 {object} ErrorResponse
// @Failure 401 {object} ErrorResponse
// @Failure 404 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Security ApiKeyAuth
// @Router /networks/{networkId}/disable [post]
// @ID disableNetwork
func DisableNetwork(_ http.ResponseWriter, _ *http.Request) {}

// RenameNetwork godoc
// @Summary Rename network
// @Description Rename an existing network
// @Tags Networks
// @Accept json
// @Produce json
// @Param networkId path string true "Network ID" format(uuid)
// @Param rename body RenameNetworkRequest true "New network name"
// @Success 200 {object} NetworkResponse
// @Failure 400 {object} ErrorResponse
// @Failure 401 {object} ErrorResponse
// @Failure 404 {object} ErrorResponse
// @Failure 409 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Security ApiKeyAuth
// @Router /networks/{networkId}/rename [put]
// @ID renameNetwork
func RenameNetwork(_ http.ResponseWriter, _ *http.Request) {}

// ListNetworkExclusions godoc
// @Summary List network exclusions
// @Description Get exclusion rules for a specific network
// @Tags Networks
// @Produce json
// @Param networkId path string true "Network ID" format(uuid)
// @Success 200 {array} NetworkExclusionResponse
// @Failure 400 {object} ErrorResponse
// @Failure 401 {object} ErrorResponse
// @Failure 404 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Security ApiKeyAuth
// @Router /networks/{networkId}/exclusions [get]
// @ID listNetworkExclusions
func ListNetworkExclusions(_ http.ResponseWriter, _ *http.Request) {}

// CreateNetworkExclusion godoc
// @Summary Create network exclusion
// @Description Add an exclusion rule to a specific network
// @Tags Networks
// @Accept json
// @Produce json
// @Param networkId path string true "Network ID" format(uuid)
// @Param exclusion body CreateExclusionRequest true "Exclusion configuration"
// @Success 201 {object} NetworkExclusionResponse
// @Failure 400 {object} ErrorResponse
// @Failure 401 {object} ErrorResponse
// @Failure 404 {object} ErrorResponse
// @Failure 422 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Security ApiKeyAuth
// @Router /networks/{networkId}/exclusions [post]
// @ID createNetworkExclusion
func CreateNetworkExclusion(_ http.ResponseWriter, _ *http.Request) {}

// ListGlobalExclusions godoc
// @Summary List global exclusions
// @Description Get all global exclusion rules not tied to a specific network
// @Tags Exclusions
// @Produce json
// @Success 200 {array} NetworkExclusionResponse
// @Failure 401 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Security ApiKeyAuth
// @Router /exclusions [get]
// @ID listGlobalExclusions
func ListGlobalExclusions(_ http.ResponseWriter, _ *http.Request) {}

// CreateGlobalExclusion godoc
// @Summary Create global exclusion
// @Description Create a global exclusion rule that applies to all networks
// @Tags Exclusions
// @Accept json
// @Produce json
// @Param exclusion body CreateExclusionRequest true "Exclusion configuration"
// @Success 201 {object} NetworkExclusionResponse
// @Failure 400 {object} ErrorResponse
// @Failure 401 {object} ErrorResponse
// @Failure 422 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Security ApiKeyAuth
// @Router /exclusions [post]
// @ID createGlobalExclusion
func CreateGlobalExclusion(_ http.ResponseWriter, _ *http.Request) {}

// DeleteExclusion godoc
// @Summary Delete exclusion
// @Description Delete an exclusion rule by ID
// @Tags Exclusions
// @Param exclusionId path string true "Exclusion ID" format(uuid)
// @Success 204 "Successfully deleted"
// @Failure 400 {object} ErrorResponse
// @Failure 401 {object} ErrorResponse
// @Failure 404 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Security ApiKeyAuth
// @Router /exclusions/{exclusionId} [delete]
// @ID deleteExclusion
func DeleteExclusion(_ http.ResponseWriter, _ *http.Request) {}

// StartNetworkDiscovery godoc
// @Summary Start a discovery run for a network
// @Description Creates a discovery job linked to the network, immediately transitions it to running, and returns the job. If a discovery engine is configured the actual nmap scan executes asynchronously.
// @Tags Networks
// @Produce json
// @Param networkId path string true "Network ID" format(uuid)
// @Success 202 {object} DiscoveryJobResponse
// @Failure 400 {object} ErrorResponse
// @Failure 401 {object} ErrorResponse
// @Failure 404 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Failure 503 {object} ErrorResponse
// @Security ApiKeyAuth
// @Router /networks/{networkId}/discover [post]
// @ID startNetworkDiscovery
func StartNetworkDiscovery(_ http.ResponseWriter, _ *http.Request) {}

// ListNetworkDiscoveryJobs godoc
// @Summary List discovery jobs for a network
// @Description Returns paginated history of discovery runs linked to the network, ordered most-recent first.
// @Tags Networks
// @Produce json
// @Param networkId path string true "Network ID" format(uuid)
// @Param page query int false "Page number (default 1)"
// @Param page_size query int false "Page size (default 50, max 100)"
// @Success 200 {object} PaginatedDiscoveryJobsResponse
// @Failure 400 {object} ErrorResponse
// @Failure 401 {object} ErrorResponse
// @Failure 503 {object} ErrorResponse
// @Security ApiKeyAuth
// @Router /networks/{networkId}/discovery [get]
// @ID listNetworkDiscoveryJobs
func ListNetworkDiscoveryJobs(_ http.ResponseWriter, _ *http.Request) {}

// GetSmartScanSuggestions godoc
// @Summary Get Smart Scan suggestions
// @Description Returns fleet-wide counts of hosts in each knowledge-gap category.
// @Tags Smart Scan
// @Produce json
// @Success 200 {object} SuggestionSummaryResponse
// @Failure 401 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Security ApiKeyAuth
// @Router /smart-scan/suggestions [get]
// @ID getSmartScanSuggestions
func GetSmartScanSuggestions(_ http.ResponseWriter, _ *http.Request) {}

// EvaluateHostStage godoc
// @Summary Evaluate next scan stage for a host
// @Description Returns the recommended next scan stage for the specified host.
// @Tags Smart Scan
// @Produce json
// @Param id path string true "Host UUID" format(uuid)
// @Success 200 {object} ScanStageResponse
// @Failure 400 {object} ErrorResponse
// @Failure 401 {object} ErrorResponse
// @Failure 404 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Security ApiKeyAuth
// @Router /smart-scan/hosts/{id}/stage [get]
// @ID evaluateHostStage
func EvaluateHostStage(_ http.ResponseWriter, _ *http.Request) {}

// TriggerSmartScan godoc
// @Summary Trigger Smart Scan for a host
// @Description Evaluates the host's knowledge gaps and queues the appropriate scan.
// @Tags Smart Scan
// @Produce json
// @Param id path string true "Host UUID" format(uuid)
// @Success 202 {object} TriggerHostResponse
// @Success 200 {object} TriggerHostResponse "No action needed"
// @Failure 400 {object} ErrorResponse
// @Failure 401 {object} ErrorResponse
// @Failure 404 {object} ErrorResponse
// @Failure 429 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Security ApiKeyAuth
// @Router /smart-scan/hosts/{id}/trigger [post]
// @ID triggerSmartScan
func TriggerSmartScan(_ http.ResponseWriter, _ *http.Request) {}

// TriggerSmartScanBatch godoc
// @Summary Batch trigger Smart Scan
// @Description Queues smart scans for all eligible hosts matching the filter.
// @Tags Smart Scan
// @Accept json
// @Produce json
// @Param body body TriggerBatchRequest false "Batch filter"
// @Success 202 {object} BatchResultResponse
// @Failure 400 {object} ErrorResponse
// @Failure 401 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Security ApiKeyAuth
// @Router /smart-scan/trigger-batch [post]
// @ID triggerSmartScanBatch
func TriggerSmartScanBatch(_ http.ResponseWriter, _ *http.Request) {}

// GetProfileRecommendations godoc
// @Summary Get profile recommendations
// @Description Returns profile suggestions grouped by detected OS family.
// @Tags smart-scan
// @Produce json
// @Success 200 {array} ProfileRecommendationResponse
// @Failure 401 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Security ApiKeyAuth
// @Router /smart-scan/profile-recommendations [get]
// @ID getProfileRecommendations
func GetProfileRecommendations(_ http.ResponseWriter, _ *http.Request) {}

// ScanStageResponse is the response body for EvaluateHostStage.
type ScanStageResponse struct {
	Stage       string  `json:"stage" example:"os_detection" enums:"os_detection,port_expansion,service_scan,refresh,skip"`
	ScanType    string  `json:"scan_type" example:"syn"`
	Ports       string  `json:"ports" example:"1-1024"`
	OSDetection bool    `json:"os_detection" example:"true"`
	ProfileID   *string `json:"profile_id,omitempty" example:"linux-common"`
	Reason      string  `json:"reason" example:"no OS information recorded"`
}

// SuggestionGroupResponse represents a count of hosts sharing a knowledge gap.
type SuggestionGroupResponse struct {
	Count       int    `json:"count" example:"12"`
	Description string `json:"description" example:"Hosts with no OS information"`
	Action      string `json:"action" example:"os_detection"`
}

// SuggestionSummaryResponse is the response body for GetSmartScanSuggestions.
type SuggestionSummaryResponse struct {
	NoOSInfo    SuggestionGroupResponse `json:"no_os_info"`
	NoPorts     SuggestionGroupResponse `json:"no_ports"`
	NoServices  SuggestionGroupResponse `json:"no_services"`
	Stale       SuggestionGroupResponse `json:"stale"`
	WellKnown   SuggestionGroupResponse `json:"well_known"`
	TotalHosts  int                     `json:"total_hosts" example:"1082"`
	GeneratedAt time.Time               `json:"generated_at"`
}

// TriggerHostResponse is the response body for TriggerSmartScan.
type TriggerHostResponse struct {
	HostID  string `json:"host_id" example:"550e8400-e29b-41d4-a716-446655440000"`
	Queued  bool   `json:"queued" example:"true"`
	ScanID  string `json:"scan_id,omitempty" example:"550e8400-e29b-41d4-a716-446655440001"`
	Message string `json:"message,omitempty" example:"no scan needed — host knowledge is sufficient"`
}

// TriggerBatchRequest is the request body for TriggerSmartScanBatch.
type TriggerBatchRequest struct {
	Stage       string   `json:"stage,omitempty" example:"os_detection" enums:"os_detection,port_expansion,service_scan,refresh,skip"`
	HostIDs     []string `json:"host_ids,omitempty" example:"550e8400-e29b-41d4-a716-446655440000"`
	NetworkCIDR string   `json:"network_cidr,omitempty" example:"192.168.1.0/24"`
	OSFamily    string   `json:"os_family,omitempty" example:"Linux"`
	Limit       int      `json:"limit,omitempty" example:"50"`
}

// BatchDetailEntryResponse records the outcome for one host in a batch.
type BatchDetailEntryResponse struct {
	HostID string `json:"host_id" example:"550e8400-e29b-41d4-a716-446655440000"`
	Stage  string `json:"stage" example:"os_detection"`
	ScanID string `json:"scan_id,omitempty" example:"550e8400-e29b-41d4-a716-446655440001"`
	Reason string `json:"reason,omitempty" example:"skip: host knowledge is sufficient"`
}

// PortDefinitionResponse mirrors db.PortDefinition for Swagger documentation.
type PortDefinitionResponse struct {
	Port        int      `json:"port" example:"443"`
	Protocol    string   `json:"protocol" example:"tcp"`
	Service     string   `json:"service" example:"https"`
	Description string   `json:"description,omitempty" example:"HTTP over TLS/SSL"`
	Category    string   `json:"category,omitempty" example:"web"`
	OSFamilies  []string `json:"os_families" example:"linux,windows"`
	IsStandard  bool     `json:"is_standard" example:"true"`
}

// PortListResponse is the paginated response body for ListPorts.
type PortListResponse struct {
	Ports      []PortDefinitionResponse `json:"ports"`
	Total      int64                    `json:"total" example:"103"`
	Page       int                      `json:"page" example:"1"`
	PageSize   int                      `json:"page_size" example:"50"`
	TotalPages int                      `json:"total_pages" example:"3"`
}

// StringListResponse is a generic response containing a list of strings.
type StringListResponse struct {
	Categories []string `json:"categories" example:"web,database,network"`
}

// PortHostCountResponse represents the open-host count for a single port+protocol pair.
type PortHostCountResponse struct {
	Port     int    `json:"port" example:"443"`
	Protocol string `json:"protocol" example:"tcp"`
	Count    int    `json:"count" example:"12"`
}

// PortHostCountListResponse is the response body for ListPortHostCounts.
type PortHostCountListResponse = []PortHostCountResponse

// ProfileRecommendationResponse is a single entry in the profile recommendations list.
type ProfileRecommendationResponse struct {
	OSFamily    string `json:"os_family" example:"Linux"`
	HostCount   int    `json:"host_count" example:"42"`
	ProfileID   string `json:"profile_id" example:"550e8400-e29b-41d4-a716-446655440000"`
	ProfileName string `json:"profile_name" example:"Linux Standard"`
	Action      string `json:"action" example:"port_expansion"`
}

// BatchResultResponse is the response body for TriggerSmartScanBatch.
type BatchResultResponse struct {
	Queued  int                        `json:"queued" example:"15"`
	Skipped int                        `json:"skipped" example:"51"`
	Details []BatchDetailEntryResponse `json:"details"`
}

// ExpiringCertificateResponse represents a single certificate that is approaching expiry.
type ExpiringCertificateResponse struct {
	HostID    string    `json:"host_id" example:"550e8400-e29b-41d4-a716-446655440002"`
	HostIP    string    `json:"host_ip" example:"192.168.1.100"`
	Hostname  string    `json:"hostname,omitempty" example:"server01.local"`
	Port      int       `json:"port" example:"443"`
	Protocol  string    `json:"protocol" example:"tcp"`
	SubjectCN string    `json:"subject_cn,omitempty" example:"server01.local"`
	NotAfter  time.Time `json:"not_after"`
	DaysLeft  int       `json:"days_left" example:"14"`
}

// ExpiringCertificatesResponse is the response body for GET /certificates/expiring.
type ExpiringCertificatesResponse struct {
	Certificates []ExpiringCertificateResponse `json:"certificates"`
	Days         int                           `json:"days" example:"30"`
}

// GetExpiringCertificates godoc
// @Summary      List expiring TLS certificates
// @Description  Returns certificates expiring within the specified lookahead window (default 30 days, max 90).
// @Description  Each entry is enriched with the host IP address and hostname.
// @Tags         certificates
// @Produce      json
// @Param        days  query  int  false  "Lookahead window in days (1–90, default 30)"
// @Success      200  {object}  ExpiringCertificatesResponse
// @Failure      400  {object}  ErrorResponse
// @Failure      500  {object}  ErrorResponse
// @Security     ApiKeyAuth
// @Router       /certificates/expiring [get]
// @ID           getExpiringCertificates
func GetExpiringCertificates(_ http.ResponseWriter, _ *http.Request) {}

// ListPorts godoc
// @Summary      List port definitions
// @Description  Returns a paginated list of well-known port/service definitions.
// @Description  Supports filtering by search query, category, and protocol.
// @Tags         ports
// @Produce      json
// @Param        search     query  string  false  "Search by port number, service name, or description"
// @Param        category   query  string  false  "Filter by category (web, database, windows, etc.)"
// @Param        protocol   query  string  false  "Filter by protocol (tcp or udp)"
// @Param        sort_by    query  string  false  "Sort field (port, service, category)"
// @Param        sort_order query  string  false  "Sort direction (asc or desc)"
// @Param        page       query  int     false  "Page number"
// @Param        page_size  query  int     false  "Results per page"
// @Success      200  {object}  PortListResponse
// @Failure      400  {object}  ErrorResponse
// @Failure      500  {object}  ErrorResponse
// @Security     ApiKeyAuth
// @Router       /ports [get]
// @ID           listPorts
func ListPorts(_ http.ResponseWriter, _ *http.Request) {}

// GetPort godoc
// @Summary      Get port definition
// @Description  Returns the service definition for a specific port number.
// @Description  Use ?protocol=tcp (default) or ?protocol=udp.
// @Tags         ports
// @Produce      json
// @Param        port      path   int     true   "Port number"
// @Param        protocol  query  string  false  "Protocol (tcp or udp, default tcp)"
// @Success      200  {object}  PortDefinitionResponse
// @Failure      400  {object}  ErrorResponse
// @Failure      404  {object}  ErrorResponse
// @Security     ApiKeyAuth
// @Router       /ports/{port} [get]
// @ID           getPort
func GetPort(_ http.ResponseWriter, _ *http.Request) {}

// ListPortCategories godoc
// @Summary      List port categories
// @Description  Returns the distinct category values used in the port definition database.
// @Tags         ports
// @Produce      json
// @Success      200  {object}  StringListResponse
// @Failure      500  {object}  ErrorResponse
// @Security     ApiKeyAuth
// @Router       /ports/categories [get]
// @ID           listPortCategories
func ListPortCategories(_ http.ResponseWriter, _ *http.Request) {}

// ListPortHostCounts godoc
// @Summary      List port host counts
// @Description  Returns the number of distinct hosts with at least one open scan result
// @Description  for each port+protocol pair observed in the network.
// @Tags         ports
// @Produce      json
// @Success      200  {array}   PortHostCountResponse
// @Failure      500  {object}  ErrorResponse
// @Security     ApiKeyAuth
// @Router       /ports/host-counts [get]
// @ID           listPortHostCounts
func ListPortHostCounts(_ http.ResponseWriter, _ *http.Request) {}
