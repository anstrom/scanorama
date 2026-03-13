// Package handlers - admin API types and request/response structures.
package handlers

import "time"

// WorkerStatusResponse represents worker pool status information.
type WorkerStatusResponse struct {
	TotalWorkers   int                    `json:"total_workers"`
	ActiveWorkers  int                    `json:"active_workers"`
	IdleWorkers    int                    `json:"idle_workers"`
	QueueSize      int                    `json:"queue_size"`
	ProcessedJobs  int64                  `json:"processed_jobs"`
	FailedJobs     int64                  `json:"failed_jobs"`
	AvgJobDuration time.Duration          `json:"avg_job_duration"`
	Workers        []WorkerInfo           `json:"workers"`
	Summary        map[string]interface{} `json:"summary"`
	Timestamp      time.Time              `json:"timestamp"`
}

// WorkerInfo represents individual worker information.
type WorkerInfo struct {
	ID            string         `json:"id"`
	Status        string         `json:"status"`
	CurrentJob    *JobInfo       `json:"current_job,omitempty"`
	JobsProcessed int64          `json:"jobs_processed"`
	JobsFailed    int64          `json:"jobs_failed"`
	LastJobTime   *time.Time     `json:"last_job_time,omitempty"`
	StartTime     time.Time      `json:"start_time"`
	Uptime        time.Duration  `json:"uptime"`
	MemoryUsage   int64          `json:"memory_usage_bytes"`
	CPUUsage      float64        `json:"cpu_usage_percent"`
	ErrorRate     float64        `json:"error_rate"`
	Metrics       map[string]int `json:"metrics"`
}

// JobInfo represents current job information.
type JobInfo struct {
	ID        string        `json:"id"`
	Type      string        `json:"type"`
	Target    string        `json:"target,omitempty"`
	StartTime time.Time     `json:"start_time"`
	Duration  time.Duration `json:"duration"`
	Progress  float64       `json:"progress"`
}

// ConfigResponse represents configuration information.
type ConfigResponse struct {
	API      interface{} `json:"api"`
	Database interface{} `json:"database"`
	Scanning interface{} `json:"scanning"`
	Logging  interface{} `json:"logging"`
	Daemon   interface{} `json:"daemon"`
}

// ConfigUpdateRequest represents a request to update configuration.
type ConfigUpdateRequest struct {
	Section string           `json:"section" validate:"required,oneof=api database scanning logging daemon"`
	Config  ConfigUpdateData `json:"config" validate:"required"`
}

// ConfigUpdateData represents the configuration data for updates.
type ConfigUpdateData struct {
	API      *APIConfigUpdate      `json:"api,omitempty"`
	Database *DatabaseConfigUpdate `json:"database,omitempty"`
	Scanning *ScanningConfigUpdate `json:"scanning,omitempty"`
	Logging  *LoggingConfigUpdate  `json:"logging,omitempty"`
	Daemon   *DaemonConfigUpdate   `json:"daemon,omitempty"`
}

// APIConfigUpdate represents updatable API configuration fields.
type APIConfigUpdate struct {
	Enabled           *bool    `json:"enabled,omitempty"`
	Host              *string  `json:"host,omitempty"`
	Port              *int     `json:"port,omitempty" validate:"omitempty,min=1,max=65535"`
	ReadTimeout       *string  `json:"read_timeout,omitempty"`
	WriteTimeout      *string  `json:"write_timeout,omitempty"`
	IdleTimeout       *string  `json:"idle_timeout,omitempty"`
	MaxHeaderBytes    *int     `json:"max_header_bytes,omitempty" validate:"omitempty,min=1024,max=1048576"`
	EnableCORS        *bool    `json:"enable_cors,omitempty"`
	CORSOrigins       []string `json:"cors_origins,omitempty"`
	AuthEnabled       *bool    `json:"auth_enabled,omitempty"`
	RateLimitEnabled  *bool    `json:"rate_limit_enabled,omitempty"`
	RateLimitRequests *int     `json:"rate_limit_requests,omitempty" validate:"omitempty,min=1,max=10000"`
	RateLimitWindow   *string  `json:"rate_limit_window,omitempty"`
	RequestTimeout    *string  `json:"request_timeout,omitempty"`
	MaxRequestSize    *int     `json:"max_request_size,omitempty" validate:"omitempty,min=1,max=104857600"`
}

// DatabaseConfigUpdate represents updatable database configuration fields.
type DatabaseConfigUpdate struct {
	Host            *string `json:"host,omitempty"`
	Port            *int    `json:"port,omitempty" validate:"omitempty,min=1,max=65535"`
	Database        *string `json:"database,omitempty" validate:"omitempty,min=1,max=63"`
	Username        *string `json:"username,omitempty" validate:"omitempty,min=1,max=63"`
	SSLMode         *string `json:"ssl_mode,omitempty" validate:"omitempty,oneof=disable require verify-ca verify-full"`
	MaxOpenConns    *int    `json:"max_open_conns,omitempty" validate:"omitempty,min=1,max=100"`
	MaxIdleConns    *int    `json:"max_idle_conns,omitempty" validate:"omitempty,min=1,max=100"`
	ConnMaxLifetime *string `json:"conn_max_lifetime,omitempty"`
	ConnMaxIdleTime *string `json:"conn_max_idle_time,omitempty"`
}

// ScanningConfigUpdate represents updatable scanning configuration fields.
type ScanningConfigUpdate struct {
	WorkerPoolSize         *int    `json:"worker_pool_size,omitempty" validate:"omitempty,min=1,max=1000"`
	DefaultInterval        *string `json:"default_interval,omitempty"`
	MaxScanTimeout         *string `json:"max_scan_timeout,omitempty"`
	DefaultPorts           *string `json:"default_ports,omitempty" validate:"omitempty,max=1000"`
	DefaultScanType        *string `json:"default_scan_type,omitempty" validate:"omitempty,oneof=connect syn ack window fin null xmas maimon"` //nolint:lll
	MaxConcurrentTargets   *int    `json:"max_concurrent_targets,omitempty" validate:"omitempty,min=1,max=10000"`
	EnableServiceDetection *bool   `json:"enable_service_detection,omitempty"`
	EnableOSDetection      *bool   `json:"enable_os_detection,omitempty"`
}

// LoggingConfigUpdate represents updatable logging configuration fields.
type LoggingConfigUpdate struct {
	Level          *string `json:"level,omitempty" validate:"omitempty,oneof=debug info warn error"`
	Format         *string `json:"format,omitempty" validate:"omitempty,oneof=text json"`
	Output         *string `json:"output,omitempty" validate:"omitempty,min=1,max=255"`
	Structured     *bool   `json:"structured,omitempty"`
	RequestLogging *bool   `json:"request_logging,omitempty"`
}

// DaemonConfigUpdate represents updatable daemon configuration fields.
type DaemonConfigUpdate struct {
	PIDFile         *string `json:"pid_file,omitempty" validate:"omitempty,min=1,max=255"`
	WorkDir         *string `json:"work_dir,omitempty" validate:"omitempty,min=1,max=255"`
	User            *string `json:"user,omitempty" validate:"omitempty,min=1,max=32"`
	Group           *string `json:"group,omitempty" validate:"omitempty,min=1,max=32"`
	Daemonize       *bool   `json:"daemonize,omitempty"`
	ShutdownTimeout *string `json:"shutdown_timeout,omitempty"`
}

// LogsResponse represents log retrieval response.
type LogsResponse struct {
	Lines       []LogEntry `json:"lines"`
	TotalLines  int        `json:"total_lines"`
	StartLine   int        `json:"start_line"`
	EndLine     int        `json:"end_line"`
	HasMore     bool       `json:"has_more"`
	GeneratedAt time.Time  `json:"generated_at"`
}

// LogEntry represents a single log entry.
type LogEntry struct {
	Timestamp time.Time              `json:"timestamp"`
	Level     string                 `json:"level"`
	Message   string                 `json:"message"`
	Component string                 `json:"component,omitempty"`
	Fields    map[string]interface{} `json:"fields,omitempty"`
	Error     string                 `json:"error,omitempty"`
}
