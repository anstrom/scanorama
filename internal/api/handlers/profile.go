// Package handlers provides HTTP request handlers for the Scanorama API.
// This file implements scan profile management endpoints including CRUD operations
// and profile configurations for different scan types.
package handlers

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/anstrom/scanorama/internal/db"
	"github.com/anstrom/scanorama/internal/errors"
	"github.com/anstrom/scanorama/internal/metrics"
)

// Duration is a custom type that can unmarshal duration strings from JSON
type Duration time.Duration

// UnmarshalJSON implements json.Unmarshaler for Duration
func (d *Duration) UnmarshalJSON(data []byte) error {
	var s string
	if err := json.Unmarshal(data, &s); err != nil {
		return err
	}
	if s == "" {
		*d = Duration(0)
		return nil
	}
	duration, err := time.ParseDuration(s)
	if err != nil {
		return fmt.Errorf("invalid duration format: %s", s)
	}
	*d = Duration(duration)
	return nil
}

// MarshalJSON implements json.Marshaler for Duration
func (d Duration) MarshalJSON() ([]byte, error) {
	return json.Marshal(time.Duration(d).String())
}

// ToDuration converts custom Duration to time.Duration
func (d Duration) ToDuration() time.Duration {
	return time.Duration(d)
}

// NewDuration creates a Duration from time.Duration
func NewDuration(d time.Duration) Duration {
	return Duration(d)
}

// Profile validation constants.
const (
	maxProfileNameLength  = 255
	maxProfileDescLength  = 1000
	maxProfileHostTimeout = 30 * time.Minute
	maxProfileScanDelay   = 60 * time.Second
	maxProfileRetries     = 10
	maxProfileRatePPS     = 10000
	maxProfileTagLength   = 50

	// Scan type constants
	scanTypeComprehensive = "comprehensive"
	scanTypeAggressive    = "aggressive"
)

// ProfileHandler handles profile-related API endpoints.
type ProfileHandler struct {
	service ProfileServicer
	logger  *slog.Logger
	metrics *metrics.Registry
}

// NewProfileHandler creates a new profile handler.
func NewProfileHandler(service ProfileServicer, logger *slog.Logger, metricsManager *metrics.Registry) *ProfileHandler {
	return &ProfileHandler{
		service: service,
		logger:  logger.With("handler", "profile"),
		metrics: metricsManager,
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
	HostTimeout      Duration          `json:"host_timeout,omitempty"`
	ScanDelay        Duration          `json:"scan_delay,omitempty"`
	MaxRatePPS       int               `json:"max_rate_pps,omitempty"`
	MaxHostGroupSize int               `json:"max_host_group_size,omitempty"`
	MinHostGroupSize int               `json:"min_host_group_size,omitempty"`
	Tags             []string          `json:"tags,omitempty"`
	Default          bool              `json:"default"`
}

// TimingProfile represents timing configuration for scans.
type TimingProfile struct {
	Template          string   `json:"template,omitempty"` // paranoid, sneaky, polite, normal, aggressive, insane
	MinRTTTimeout     Duration `json:"min_rtt_timeout,omitempty"`
	MaxRTTTimeout     Duration `json:"max_rtt_timeout,omitempty"`
	InitialRTTTimeout Duration `json:"initial_rtt_timeout,omitempty"`
	MaxRetries        int      `json:"max_retries,omitempty"`
	HostTimeout       Duration `json:"host_timeout,omitempty"`
	ScanDelay         Duration `json:"scan_delay,omitempty"`
	MaxScanDelay      Duration `json:"max_scan_delay,omitempty"`
}

// ProfileResponse represents a profile response.
type ProfileResponse struct {
	ID               string            `json:"id"`
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
	listOp := &ListOperation[*db.ScanProfile, db.ProfileFilters]{
		EntityType: "profiles",
		MetricName: "api_profiles_listed_total",
		Logger:     h.logger,
		Metrics:    h.metrics,
		GetFilters: h.getProfileFilters,
		ListFromDB: h.service.ListProfiles,
		ToResponse: func(profile *db.ScanProfile) interface{} {
			return h.profileToResponse(profile)
		},
	}
	listOp.Execute(w, r)
}

// CreateProfile handles POST /api/v1/profiles - create a new profile.
func (h *ProfileHandler) CreateProfile(w http.ResponseWriter, r *http.Request) {
	CreateEntity[db.ScanProfile, db.CreateProfileInput](
		w, r,
		"profile",
		h.logger,
		h.metrics,
		func(r *http.Request) (db.CreateProfileInput, error) {
			var req ProfileRequest
			if err := parseJSON(r, &req); err != nil {
				return db.CreateProfileInput{}, err
			}
			if err := h.validateProfileRequest(&req); err != nil {
				return db.CreateProfileInput{}, err
			}
			return h.requestToCreateProfile(&req), nil
		},
		h.service.CreateProfile,
		func(profile *db.ScanProfile) interface{} {
			return h.profileToResponse(profile)
		},
		"api_profiles_created_total")
}

// GetProfile handles GET /api/v1/profiles/{id} - get a specific profile.
func (h *ProfileHandler) GetProfile(w http.ResponseWriter, r *http.Request) {
	profileID, err := extractStringFromPath(r)
	if err != nil {
		writeError(w, r, http.StatusBadRequest, err)
		return
	}

	requestID := getRequestIDFromContext(r.Context())
	h.logger.Info("Getting profile", "request_id", requestID, "profile_id", profileID)

	// Get profile from database
	profile, err := h.service.GetProfile(r.Context(), profileID)
	if err != nil {
		if errors.IsNotFound(err) {
			writeError(w, r, http.StatusNotFound, fmt.Errorf("profile not found"))
			return
		}
		h.logger.Error("Failed to get profile", "request_id", requestID, "error", err)
		writeError(w, r, http.StatusInternalServerError, fmt.Errorf("failed to get profile: %w", err))
		return
	}

	response := h.profileToResponse(profile)
	writeJSON(w, r, http.StatusOK, response)

	// Record metrics
	if h.metrics != nil {
		h.metrics.Counter("api_profiles_retrieved_total", nil)
	}
}

// UpdateProfile handles PUT /api/v1/profiles/{id} - update a profile.
func (h *ProfileHandler) UpdateProfile(w http.ResponseWriter, r *http.Request) {
	profileID, err := extractStringFromPath(r)
	if err != nil {
		writeError(w, r, http.StatusBadRequest, err)
		return
	}

	requestID := getRequestIDFromContext(r.Context())
	h.logger.Info("Updating profile", "request_id", requestID, "profile_id", profileID)

	// Parse request body
	var req ProfileRequest
	if err := parseJSON(r, &req); err != nil {
		writeError(w, r, http.StatusBadRequest, err)
		return
	}

	// Update profile in database
	profile, err := h.service.UpdateProfile(r.Context(), profileID, h.requestToUpdateProfile(&req))
	if err != nil {
		if errors.IsNotFound(err) {
			writeError(w, r, http.StatusNotFound, fmt.Errorf("profile not found"))
			return
		}
		if errors.IsForbidden(err) {
			writeError(w, r, http.StatusForbidden, err)
			return
		}
		h.logger.Error("Failed to update profile", "request_id", requestID, "error", err)
		writeError(w, r, http.StatusInternalServerError, fmt.Errorf("failed to update profile: %w", err))
		return
	}

	response := h.profileToResponse(profile)
	writeJSON(w, r, http.StatusOK, response)

	// Record metrics
	if h.metrics != nil {
		h.metrics.Counter("api_profiles_updated_total", nil)
	}
}

// DeleteProfile handles DELETE /api/v1/profiles/{id} - delete a profile.
func (h *ProfileHandler) DeleteProfile(w http.ResponseWriter, r *http.Request) {
	profileID, err := extractStringFromPath(r)
	if err != nil {
		writeError(w, r, http.StatusBadRequest, err)
		return
	}

	requestID := getRequestIDFromContext(r.Context())
	h.logger.Info("Deleting profile", "request_id", requestID, "profile_id", profileID)

	// Delete profile from database
	err = h.service.DeleteProfile(r.Context(), profileID)
	if err != nil {
		if errors.IsNotFound(err) {
			writeError(w, r, http.StatusNotFound, fmt.Errorf("profile not found"))
			return
		}
		if errors.IsForbidden(err) {
			writeError(w, r, http.StatusForbidden, err)
			return
		}
		h.logger.Error("Failed to delete profile", "request_id", requestID, "error", err)
		writeError(w, r, http.StatusInternalServerError, fmt.Errorf("failed to delete profile: %w", err))
		return
	}

	w.WriteHeader(http.StatusNoContent)

	// Record metrics
	if h.metrics != nil {
		h.metrics.Counter("api_profiles_deleted_total", nil)
	}
}

// CloneProfile handles POST /api/v1/profiles/{id}/clone — create a copy of an existing profile.
// The JSON body must contain {"name": "<new name>"}.
func (h *ProfileHandler) CloneProfile(w http.ResponseWriter, r *http.Request) {
	profileID, err := extractStringFromPath(r)
	if err != nil {
		writeError(w, r, http.StatusBadRequest, err)
		return
	}

	requestID := getRequestIDFromContext(r.Context())
	h.logger.Info("Cloning profile", "request_id", requestID, "profile_id", profileID)

	var body struct {
		Name string `json:"name"`
	}
	if err := parseJSON(r, &body); err != nil {
		writeError(w, r, http.StatusBadRequest, err)
		return
	}
	if body.Name == "" {
		writeError(w, r, http.StatusBadRequest, fmt.Errorf("name is required"))
		return
	}

	profile, err := h.service.CloneProfile(r.Context(), profileID, body.Name)
	if err != nil {
		if errors.IsNotFound(err) {
			writeError(w, r, http.StatusNotFound, fmt.Errorf("profile not found"))
			return
		}
		if errors.IsConflict(err) {
			writeError(w, r, http.StatusConflict, err)
			return
		}
		h.logger.Error("Failed to clone profile", "request_id", requestID, "error", err)
		writeError(w, r, http.StatusInternalServerError, fmt.Errorf("failed to clone profile: %w", err))
		return
	}

	writeJSON(w, r, http.StatusCreated, h.profileToResponse(profile))

	if h.metrics != nil {
		h.metrics.Counter("api_profiles_cloned_total", nil)
	}
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
	hostTimeout := req.HostTimeout.ToDuration()
	if hostTimeout < 0 {
		return fmt.Errorf("host timeout cannot be negative")
	}
	if hostTimeout > maxProfileHostTimeout {
		return fmt.Errorf("host timeout too large (max %s)", maxProfileHostTimeout)
	}
	scanDelay := req.ScanDelay.ToDuration()
	if scanDelay < 0 {
		return fmt.Errorf("scan delay cannot be negative")
	}
	if scanDelay > maxProfileScanDelay {
		return fmt.Errorf("scan delay too large (max %s)", maxProfileScanDelay)
	}

	// Validate timing profile durations
	if req.Timing.MinRTTTimeout.ToDuration() < 0 {
		return fmt.Errorf("min RTT timeout cannot be negative")
	}
	if req.Timing.MaxRTTTimeout.ToDuration() < 0 {
		return fmt.Errorf("max RTT timeout cannot be negative")
	}
	minRTT := req.Timing.MinRTTTimeout.ToDuration()
	maxRTT := req.Timing.MaxRTTTimeout.ToDuration()
	if minRTT > maxRTT && maxRTT > 0 {
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

	if sortBy := r.URL.Query().Get("sort_by"); sortBy != "" {
		filters.SortBy = sortBy
	}
	if sortOrder := r.URL.Query().Get("sort_order"); sortOrder != "" {
		filters.SortOrder = sortOrder
	}

	return filters
}

// buildProfileOptions assembles the options map from a ProfileRequest, merging
// user-provided key/value pairs with the boolean scan flags.
func buildProfileOptions(req *ProfileRequest) map[string]interface{} {
	options := make(map[string]interface{})
	for k, v := range req.Options {
		options[k] = v
	}
	options["service_detection"] = req.ServiceDetection
	options["os_detection"] = req.OSDetection
	options["script_scan"] = req.ScriptScan
	options["udp_scan"] = req.UDPScan
	options["max_retries"] = req.MaxRetries
	options["host_timeout"] = req.HostTimeout.ToDuration()
	options["scan_delay"] = req.ScanDelay.ToDuration()
	options["max_rate_pps"] = req.MaxRatePPS
	options["max_host_group_size"] = req.MaxHostGroupSize
	options["min_host_group_size"] = req.MinHostGroupSize
	options["default"] = req.Default
	return options
}

// requestToCreateProfile converts a ProfileRequest to a typed CreateProfileInput for the DB layer.
func (h *ProfileHandler) requestToCreateProfile(req *ProfileRequest) db.CreateProfileInput {
	return db.CreateProfileInput{
		Name:        req.Name,
		Description: req.Description,
		ScanType:    req.ScanType,
		Ports:       req.Ports,
		Options:     buildProfileOptions(req),
		Timing:      req.Timing.Template,
	}
}

// requestToUpdateProfile converts a ProfileRequest to a typed UpdateProfileInput for the DB layer.
// Only non-empty / non-nil fields are set so that absent values don't overwrite existing data.
func (h *ProfileHandler) requestToUpdateProfile(req *ProfileRequest) db.UpdateProfileInput {
	input := db.UpdateProfileInput{
		Options: buildProfileOptions(req),
	}
	if req.Name != "" {
		input.Name = &req.Name
	}
	if req.Description != "" {
		input.Description = &req.Description
	}
	if req.ScanType != "" {
		input.ScanType = &req.ScanType
	}
	if req.Ports != "" {
		input.Ports = &req.Ports
	}
	if req.Timing.Template != "" {
		input.Timing = &req.Timing.Template
	}
	return input
}

// parseProfileOptions parses JSON options and converts to string map
func parseProfileOptions(optionsJSON string) (stringOptions map[string]string, parsedOptions map[string]interface{}) {
	stringOptions = make(map[string]string)

	if optionsJSON == "" {
		parsedOptions = make(map[string]interface{})
	} else if err := json.Unmarshal([]byte(optionsJSON), &parsedOptions); err != nil {
		parsedOptions = make(map[string]interface{})
	} else {
		// Convert to map[string]string for response
		for k, v := range parsedOptions {
			if str, ok := v.(string); ok {
				stringOptions[k] = str
			}
		}
	}

	return stringOptions, parsedOptions
}

// extractScanFlags extracts boolean scan flags from options with fallbacks
func extractScanFlags(parsedOptions map[string]interface{}, profile *db.ScanProfile) (service, os, script, udp bool) {
	if val, ok := parsedOptions["service_detection"].(bool); ok {
		service = val
	} else {
		service = profile.ScanType == scanTypeComprehensive
	}

	if val, ok := parsedOptions["os_detection"].(bool); ok {
		os = val
	} else {
		os = profile.ScanType == scanTypeAggressive || profile.ScanType == scanTypeComprehensive
	}

	if val, ok := parsedOptions["script_scan"].(bool); ok {
		script = val
	} else {
		script = len(profile.Scripts) > 0 || profile.ScanType == scanTypeComprehensive
	}

	if val, ok := parsedOptions["udp_scan"].(bool); ok {
		udp = val
	} else {
		udp = profile.ScanType == scanTypeComprehensive
	}

	return service, os, script, udp
}

// extractAdditionalOptions extracts numeric and duration options
func extractAdditionalOptions(parsedOptions map[string]interface{}) (
	maxRetries, maxRatePPS int, hostTimeout, scanDelay time.Duration) {
	if val, ok := parsedOptions["max_retries"].(float64); ok {
		maxRetries = int(val)
	}
	if val, ok := parsedOptions["max_rate_pps"].(float64); ok {
		maxRatePPS = int(val)
	}
	if val, ok := parsedOptions["host_timeout"].(string); ok {
		if duration, err := time.ParseDuration(val); err == nil {
			hostTimeout = duration
		}
	}
	if val, ok := parsedOptions["scan_delay"].(string); ok {
		if duration, err := time.ParseDuration(val); err == nil {
			scanDelay = duration
		}
	}
	return maxRetries, maxRatePPS, hostTimeout, scanDelay
}

// profileToResponse converts a database profile to response format.
func (h *ProfileHandler) profileToResponse(profile *db.ScanProfile) ProfileResponse {
	response := ProfileResponse{
		ID:          profile.ID,
		Name:        profile.Name,
		Description: profile.Description,
		ScanType:    profile.ScanType,
		Ports:       profile.Ports,
		Default:     profile.BuiltIn,
		CreatedAt:   profile.CreatedAt,
		UpdatedAt:   profile.UpdatedAt,
	}

	// Parse options using helper function
	stringOptions, parsedOptions := parseProfileOptions(string(profile.Options))
	response.Options = stringOptions

	// Parse timing information
	if profile.Timing != "" {
		response.Timing = TimingProfile{
			Template: profile.Timing,
		}
	}

	// Extract scan flags using helper function
	response.ServiceDetection, response.OSDetection, response.ScriptScan, response.UDPScan =
		extractScanFlags(parsedOptions, profile)

	// Extract additional options using helper function
	response.MaxRetries, response.MaxRatePPS, response.HostTimeout, response.ScanDelay =
		extractAdditionalOptions(parsedOptions)

	// Convert scripts array to tags (as placeholder)
	response.Tags = []string(profile.Scripts)

	// Set usage count (not stored in database yet)
	response.UsageCount = 0

	return response
}
