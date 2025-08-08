// Package handlers provides HTTP request handlers for the Scanorama API.
// This file implements scan profile management endpoints including CRUD operations
// and profile configurations for different scan types.
package handlers

import (
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/anstrom/scanorama/internal/db"
	"github.com/anstrom/scanorama/internal/metrics"
)

// Profile validation constants.
const (
	maxProfileNameLength  = 255
	maxProfileDescLength  = 1000
	maxProfileHostTimeout = 30 * time.Minute
	maxProfileScanDelay   = 60 * time.Second
	maxProfileRetries     = 10
	maxProfileRatePPS     = 10000
	maxProfileTagLength   = 50
)

// ProfileHandler handles profile-related API endpoints.
type ProfileHandler struct {
	database *db.DB
	logger   *slog.Logger
	metrics  *metrics.Registry
}

// NewProfileHandler creates a new profile handler.
func NewProfileHandler(database *db.DB, logger *slog.Logger, metricsManager *metrics.Registry) *ProfileHandler {
	return &ProfileHandler{
		database: database,
		logger:   logger.With("handler", "profile"),
		metrics:  metricsManager,
	}
}

// ProfileRequest represents a profile creation/update request.
type ProfileRequest struct {
	Name        string `json:"name" validate:"required,min=1,max=255"`
	Description string `json:"description,omitempty"`
	// ScanType supports: connect, syn, ack, aggressive, comprehensive
	ScanType         string            `json:"scan_type" validate:"required"`
	Ports            string            `json:"ports,omitempty"`
	Options          map[string]string `json:"options,omitempty"`
	Timing           TimingProfile     `json:"timing,omitempty"`
	ServiceDetection bool              `json:"service_detection"`
	OSDetection      bool              `json:"os_detection"`
	ScriptScan       bool              `json:"script_scan"`
	UDPScan          bool              `json:"udp_scan"`
	MaxRetries       int               `json:"max_retries,omitempty"`
	HostTimeout      time.Duration     `json:"host_timeout,omitempty"`
	ScanDelay        time.Duration     `json:"scan_delay,omitempty"`
	MaxRatePPS       int               `json:"max_rate_pps,omitempty"`
	MaxHostGroupSize int               `json:"max_host_group_size,omitempty"`
	MinHostGroupSize int               `json:"min_host_group_size,omitempty"`
	Tags             []string          `json:"tags,omitempty"`
	Default          bool              `json:"default"`
}

// TimingProfile represents timing configuration for scans.
type TimingProfile struct {
	Template          string        `json:"template,omitempty"` // paranoid, sneaky, polite, normal, aggressive, insane
	MinRTTTimeout     time.Duration `json:"min_rtt_timeout,omitempty"`
	MaxRTTTimeout     time.Duration `json:"max_rtt_timeout,omitempty"`
	InitialRTTTimeout time.Duration `json:"initial_rtt_timeout,omitempty"`
	MaxRetries        int           `json:"max_retries,omitempty"`
	HostTimeout       time.Duration `json:"host_timeout,omitempty"`
	ScanDelay         time.Duration `json:"scan_delay,omitempty"`
	MaxScanDelay      time.Duration `json:"max_scan_delay,omitempty"`
}

// ProfileResponse represents a profile response.
type ProfileResponse struct {
	ID               int64             `json:"id"`
	Name             string            `json:"name"`
	Description      string            `json:"description,omitempty"`
	ScanType         string            `json:"scan_type"`
	Ports            string            `json:"ports,omitempty"`
	Options          map[string]string `json:"options,omitempty"`
	Timing           TimingProfile     `json:"timing,omitempty"`
	ServiceDetection bool              `json:"service_detection"`
	OSDetection      bool              `json:"os_detection"`
	ScriptScan       bool              `json:"script_scan"`
	UDPScan          bool              `json:"udp_scan"`
	MaxRetries       int               `json:"max_retries,omitempty"`
	HostTimeout      time.Duration     `json:"host_timeout,omitempty"`
	ScanDelay        time.Duration     `json:"scan_delay,omitempty"`
	MaxRatePPS       int               `json:"max_rate_pps,omitempty"`
	MaxHostGroupSize int               `json:"max_host_group_size,omitempty"`
	MinHostGroupSize int               `json:"min_host_group_size,omitempty"`
	Tags             []string          `json:"tags,omitempty"`
	Default          bool              `json:"default"`
	UsageCount       int               `json:"usage_count"`
	LastUsed         *time.Time        `json:"last_used,omitempty"`
	CreatedAt        time.Time         `json:"created_at"`
	UpdatedAt        time.Time         `json:"updated_at"`
	CreatedBy        string            `json:"created_by,omitempty"`
}

// ListProfiles handles GET /api/v1/profiles - list all profiles with pagination.
func (h *ProfileHandler) ListProfiles(w http.ResponseWriter, r *http.Request) {
	listOp := &ListOperation[*db.Profile, db.ProfileFilters]{
		EntityType: "profiles",
		MetricName: "api_profiles_listed_total",
		Logger:     h.logger,
		Metrics:    h.metrics,
		GetFilters: h.getProfileFilters,
		ListFromDB: h.database.ListProfiles,
		ToResponse: func(profile *db.Profile) interface{} {
			return h.profileToResponse(profile)
		},
	}
	listOp.Execute(w, r)
}

// CreateProfile handles POST /api/v1/profiles - create a new profile.
func (h *ProfileHandler) CreateProfile(w http.ResponseWriter, r *http.Request) {
	CreateEntity[db.Profile, ProfileRequest](
		w, r,
		"profile",
		h.logger,
		h.metrics,
		func(r *http.Request) (interface{}, error) {
			var req ProfileRequest
			if err := parseJSON(r, &req); err != nil {
				return nil, err
			}
			if err := h.validateProfileRequest(&req); err != nil {
				return nil, err
			}
			return h.requestToDBProfile(&req), nil
		},
		h.database.CreateProfile,
		func(profile *db.Profile) interface{} {
			return h.profileToResponse(profile)
		},
		"api_profiles_created_total")
}

// GetProfile handles GET /api/v1/profiles/{id} - get a specific profile.
func (h *ProfileHandler) GetProfile(w http.ResponseWriter, r *http.Request) {
	profileID, err := extractUUIDFromPath(r)
	if err != nil {
		writeError(w, r, http.StatusBadRequest, err)
		return
	}

	crudOp := &CRUDOperation[db.Profile]{
		EntityType: "profile",
		Logger:     h.logger,
		Metrics:    h.metrics,
	}

	crudOp.ExecuteGet(w, r, profileID,
		h.database.GetProfile,
		func(profile *db.Profile) interface{} {
			return h.profileToResponse(profile)
		},
		"api_profiles_retrieved_total")
}

// UpdateProfile handles PUT /api/v1/profiles/{id} - update a profile.
func (h *ProfileHandler) UpdateProfile(w http.ResponseWriter, r *http.Request) {
	UpdateEntity[db.Profile, ProfileRequest](
		w, r,
		"profile",
		h.logger,
		h.metrics,
		func(r *http.Request) (interface{}, error) {
			var req ProfileRequest
			if err := parseJSON(r, &req); err != nil {
				return nil, err
			}
			if err := h.validateProfileRequest(&req); err != nil {
				return nil, err
			}
			return h.requestToDBProfile(&req), nil
		},
		h.database.UpdateProfile,
		func(profile *db.Profile) interface{} {
			return h.profileToResponse(profile)
		},
		"api_profiles_updated_total")
}

// DeleteProfile handles DELETE /api/v1/profiles/{id} - delete a profile.
func (h *ProfileHandler) DeleteProfile(w http.ResponseWriter, r *http.Request) {
	profileID, err := extractUUIDFromPath(r)
	if err != nil {
		writeError(w, r, http.StatusBadRequest, err)
		return
	}

	crudOp := &CRUDOperation[db.Profile]{
		EntityType: "profile",
		Logger:     h.logger,
		Metrics:    h.metrics,
	}

	crudOp.ExecuteDelete(w, r, profileID, h.database.DeleteProfile, "api_profiles_deleted_total")
}

// Helper methods

// validateProfileRequest validates a profile request.
func (h *ProfileHandler) validateProfileRequest(req *ProfileRequest) error {
	if err := h.validateBasicProfileFields(req); err != nil {
		return err
	}
	if err := h.validateProfileScanType(req.ScanType); err != nil {
		return err
	}
	if err := h.validateTimingTemplate(req.Timing.Template); err != nil {
		return err
	}
	if err := h.validateProfileTimeouts(req); err != nil {
		return err
	}
	if err := h.validateProfileRateLimiting(req); err != nil {
		return err
	}
	if err := h.validateHostGroupSizes(req); err != nil {
		return err
	}
	return h.validateProfileTags(req.Tags)
}

func (h *ProfileHandler) validateBasicProfileFields(req *ProfileRequest) error {
	if req.Name == "" {
		return fmt.Errorf("profile name is required")
	}
	if len(req.Name) > maxProfileNameLength {
		return fmt.Errorf("profile name too long (max %d characters)", maxProfileNameLength)
	}
	if len(req.Description) > maxProfileDescLength {
		return fmt.Errorf("description too long (max %d characters)", maxProfileDescLength)
	}
	return nil
}

func (h *ProfileHandler) validateProfileScanType(scanType string) error {
	validScanTypes := map[string]bool{
		"connect":       true,
		"syn":           true,
		"ack":           true,
		"aggressive":    true,
		"comprehensive": true,
	}
	if !validScanTypes[scanType] {
		return fmt.Errorf("invalid scan type: %s", scanType)
	}
	return nil
}

func (h *ProfileHandler) validateTimingTemplate(template string) error {
	if template == "" {
		return nil
	}
	validTimingTemplates := map[string]bool{
		"paranoid":   true,
		"sneaky":     true,
		"polite":     true,
		"normal":     true,
		"aggressive": true,
		"insane":     true,
	}
	if !validTimingTemplates[template] {
		return fmt.Errorf("invalid timing template: %s", template)
	}
	return nil
}

func (h *ProfileHandler) validateProfileTimeouts(req *ProfileRequest) error {
	if req.HostTimeout < 0 {
		return fmt.Errorf("host timeout cannot be negative")
	}
	if req.HostTimeout > maxProfileHostTimeout {
		return fmt.Errorf("host timeout too long (max %v)", maxProfileHostTimeout)
	}
	if req.ScanDelay < 0 {
		return fmt.Errorf("scan delay cannot be negative")
	}
	if req.ScanDelay > maxProfileScanDelay {
		return fmt.Errorf("scan delay too long (max %v)", maxProfileScanDelay)
	}
	if req.Timing.MinRTTTimeout < 0 {
		return fmt.Errorf("min RTT timeout cannot be negative")
	}
	if req.Timing.MaxRTTTimeout < 0 {
		return fmt.Errorf("max RTT timeout cannot be negative")
	}
	if req.Timing.MinRTTTimeout > req.Timing.MaxRTTTimeout && req.Timing.MaxRTTTimeout > 0 {
		return fmt.Errorf("min RTT timeout cannot be greater than max RTT timeout")
	}
	return nil
}

func (h *ProfileHandler) validateProfileRateLimiting(req *ProfileRequest) error {
	if req.MaxRetries < 0 {
		return fmt.Errorf("max retries cannot be negative")
	}
	if req.MaxRetries > maxProfileRetries {
		return fmt.Errorf("max retries too high (max %d)", maxProfileRetries)
	}
	if req.MaxRatePPS < 0 {
		return fmt.Errorf("max rate PPS cannot be negative")
	}
	if req.MaxRatePPS > maxProfileRatePPS {
		return fmt.Errorf("max rate PPS too high (max %d)", maxProfileRatePPS)
	}
	return nil
}

func (h *ProfileHandler) validateHostGroupSizes(req *ProfileRequest) error {
	if req.MaxHostGroupSize < 0 {
		return fmt.Errorf("max host group size cannot be negative")
	}
	if req.MinHostGroupSize < 0 {
		return fmt.Errorf("min host group size cannot be negative")
	}
	if req.MinHostGroupSize > req.MaxHostGroupSize && req.MaxHostGroupSize > 0 {
		return fmt.Errorf("min host group size cannot be greater than max host group size")
	}
	return nil
}

func (h *ProfileHandler) validateProfileTags(tags []string) error {
	for i, tag := range tags {
		if tag == "" {
			return fmt.Errorf("tag %d is empty", i+1)
		}
		if len(tag) > maxProfileTagLength {
			return fmt.Errorf("tag %d too long (max %d characters)", i+1, maxProfileTagLength)
		}
	}
	return nil
}

// getProfileFilters extracts filter parameters from request.
func (h *ProfileHandler) getProfileFilters(r *http.Request) db.ProfileFilters {
	filters := db.ProfileFilters{}

	if scanType := r.URL.Query().Get("scan_type"); scanType != "" {
		filters.ScanType = scanType
	}

	return filters
}

// requestToDBProfile converts a profile request to database profile object.
func (h *ProfileHandler) requestToDBProfile(req *ProfileRequest) interface{} {
	// This should return the appropriate database profile type
	// The exact structure would depend on the database package implementation
	return map[string]interface{}{
		"name":                req.Name,
		"description":         req.Description,
		"scan_type":           req.ScanType,
		"ports":               req.Ports,
		"options":             req.Options,
		"timing":              req.Timing,
		"service_detection":   req.ServiceDetection,
		"os_detection":        req.OSDetection,
		"script_scan":         req.ScriptScan,
		"udp_scan":            req.UDPScan,
		"max_retries":         req.MaxRetries,
		"host_timeout":        req.HostTimeout,
		"scan_delay":          req.ScanDelay,
		"max_rate_pps":        req.MaxRatePPS,
		"max_host_group_size": req.MaxHostGroupSize,
		"min_host_group_size": req.MinHostGroupSize,
		"tags":                req.Tags,
		"default":             req.Default,
		"usage_count":         0,
		"created_at":          time.Now().UTC(),
		"updated_at":          time.Now().UTC(),
	}
}

// profileToResponse converts a database profile to response format.
func (h *ProfileHandler) profileToResponse(_ interface{}) ProfileResponse {
	// This would convert from the actual database profile type
	// For now, return a placeholder structure
	return ProfileResponse{
		ID:               1,                // profile.ID
		Name:             "",               // profile.Name
		Description:      "",               // profile.Description
		ScanType:         "connect",        // profile.ScanType
		ServiceDetection: false,            // profile.ServiceDetection
		OSDetection:      false,            // profile.OSDetection
		ScriptScan:       false,            // profile.ScriptScan
		UDPScan:          false,            // profile.UDPScan
		Tags:             []string{},       // profile.Tags
		Default:          false,            // profile.Default
		UsageCount:       0,                // profile.UsageCount
		CreatedAt:        time.Now().UTC(), // profile.CreatedAt
		UpdatedAt:        time.Now().UTC(), // profile.UpdatedAt
	}
}
