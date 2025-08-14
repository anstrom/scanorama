// Package handlers provides HTTP request handlers for the Scanorama API.
// This file contains common utilities shared across all handlers to reduce
// code duplication and provide consistent patterns.
package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/gorilla/mux"

	"github.com/anstrom/scanorama/internal/errors"
	"github.com/anstrom/scanorama/internal/metrics"
)

// ContextKey represents a context key type.
type ContextKey string

// PaginationParams holds pagination parameters.
type PaginationParams struct {
	Page     int `json:"page"`
	PageSize int `json:"page_size"`
	Offset   int `json:"offset"`
}

// PaginatedResponse represents a paginated API response.
type PaginatedResponse struct {
	Data       interface{} `json:"data"`
	Pagination struct {
		Page       int   `json:"page"`
		PageSize   int   `json:"page_size"`
		TotalItems int64 `json:"total_items"`
		TotalPages int   `json:"total_pages"`
	} `json:"pagination"`
}

// ErrorResponse represents an API error response.
type ErrorResponse struct {
	Error     string    `json:"error"`
	Message   string    `json:"message"`
	Timestamp time.Time `json:"timestamp"`
	RequestID string    `json:"request_id,omitempty"`
}

// BaseHandler provides common functionality for all handlers.
type BaseHandler struct {
	logger  *slog.Logger
	metrics metrics.MetricsRegistry
}

// NewBaseHandler creates a new base handler.
func NewBaseHandler(logger *slog.Logger, metricsRegistry metrics.MetricsRegistry) *BaseHandler {
	return &BaseHandler{
		logger:  logger,
		metrics: metricsRegistry,
	}
}

// Common utility functions

// getRequestIDFromContext extracts request ID from context.
func getRequestIDFromContext(ctx context.Context) string {
	if requestID, ok := ctx.Value(ContextKey("request_id")).(string); ok {
		return requestID
	}
	return "unknown"
}

// getQueryParamInt extracts integer query parameter with default value.
func getQueryParamInt(r *http.Request, key string, defaultValue int) (int, error) {
	if value := r.URL.Query().Get(key); value != "" {
		return strconv.Atoi(value)
	}
	return defaultValue, nil
}

// extractUUIDFromPath extracts UUID from URL path parameter.
func extractUUIDFromPath(r *http.Request) (uuid.UUID, error) {
	vars := mux.Vars(r)
	idStr, exists := vars["id"]
	if !exists {
		return uuid.Nil, fmt.Errorf("id not provided")
	}

	id, err := uuid.Parse(idStr)
	if err != nil {
		return uuid.Nil, fmt.Errorf("invalid id: %s", idStr)
	}

	return id, nil
}

// extractStringFromPath extracts string ID from URL path parameter.
func extractStringFromPath(r *http.Request) (string, error) {
	vars := mux.Vars(r)
	idStr, exists := vars["id"]
	if !exists {
		return "", fmt.Errorf("id not provided")
	}

	if strings.TrimSpace(idStr) == "" {
		return "", fmt.Errorf("id cannot be empty")
	}

	return idStr, nil
}

// Pagination utilities

// getPaginationParams extracts pagination parameters from request.
func getPaginationParams(r *http.Request) (PaginationParams, error) {
	const (
		defaultPage     = 1
		defaultPageSize = 50
		maxPageSize     = 1000
	)

	page, err := getQueryParamInt(r, "page", defaultPage)
	if err != nil {
		return PaginationParams{}, fmt.Errorf("invalid page parameter: %w", err)
	}

	pageSize, err := getQueryParamInt(r, "page_size", defaultPageSize)
	if err != nil {
		return PaginationParams{}, fmt.Errorf("invalid page_size parameter: %w", err)
	}

	if page < 1 {
		page = defaultPage
	}

	if pageSize < 1 {
		pageSize = defaultPageSize
	}

	if pageSize > maxPageSize {
		pageSize = maxPageSize
	}

	offset := (page - 1) * pageSize

	return PaginationParams{
		Page:     page,
		PageSize: pageSize,
		Offset:   offset,
	}, nil
}

// Response utilities

// writeJSON writes a JSON response with the given status code.
func writeJSON(w http.ResponseWriter, r *http.Request, statusCode int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)

	if err := json.NewEncoder(w).Encode(data); err != nil {
		// Log error but don't try to write another response
		requestID := getRequestIDFromContext(r.Context())
		slog.Error("Failed to encode JSON response",
			"request_id", requestID,
			"error", err)
	}
}

// writeError writes an error response.
func writeError(w http.ResponseWriter, r *http.Request, statusCode int, err error) {
	requestID := getRequestIDFromContext(r.Context())

	response := ErrorResponse{
		Error:     http.StatusText(statusCode),
		Message:   err.Error(),
		Timestamp: time.Now().UTC(),
		RequestID: requestID,
	}

	writeJSON(w, r, statusCode, response)
}

// writePaginatedResponse writes a paginated response.
func writePaginatedResponse(
	w http.ResponseWriter,
	r *http.Request,
	data interface{},
	params PaginationParams,
	totalItems int64,
) {
	totalPages := int((totalItems + int64(params.PageSize) - 1) / int64(params.PageSize))

	response := PaginatedResponse{
		Data: data,
	}
	response.Pagination.Page = params.Page
	response.Pagination.PageSize = params.PageSize
	response.Pagination.TotalItems = totalItems
	response.Pagination.TotalPages = totalPages

	writeJSON(w, r, http.StatusOK, response)
}

// Request parsing utilities

// parseJSON parses JSON request body into the provided destination with security constraints.
func parseJSON(r *http.Request, dest interface{}) error {
	if r.Body == nil {
		return fmt.Errorf("request body is empty")
	}

	// Enforce maximum request size (10MB) to prevent DoS attacks
	const maxRequestSize = 10 * 1024 * 1024
	r.Body = http.MaxBytesReader(nil, r.Body, maxRequestSize)

	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()

	// Use strict number handling to prevent precision issues
	decoder.UseNumber()

	if err := decoder.Decode(dest); err != nil {
		if err.Error() == "http: request body too large" {
			return fmt.Errorf("request body too large (max 10MB)")
		}
		return fmt.Errorf("invalid JSON: %w", err)
	}

	return nil
}

// CRUD operation patterns

// CRUDMetrics holds metric names for CRUD operations.
type CRUDMetrics struct {
	Listed    string
	Created   string
	Retrieved string
	Updated   string
	Deleted   string
	Started   string
	Stopped   string
}

// recordCRUDMetric records a CRUD operation metric.
func recordCRUDMetric(metricsRegistry metrics.MetricsRegistry, metricName string, labels map[string]string) {
	if metricsRegistry != nil {
		metricsRegistry.Counter(metricName, labels)
	}
}

// Operation result helpers

// handleDatabaseError handles common database errors and writes appropriate HTTP responses.
func handleDatabaseError(
	w http.ResponseWriter,
	r *http.Request,
	err error,
	operation, entityType string,
	logger *slog.Logger,
) {
	requestID := getRequestIDFromContext(r.Context())

	if errors.IsNotFound(err) {
		writeError(w, r, http.StatusNotFound, fmt.Errorf("%s not found", entityType))
		return
	}

	if errors.IsConflict(err) {
		writeError(w, r, http.StatusConflict, err)
		return
	}

	logger.Error(fmt.Sprintf("Failed to %s %s", operation, entityType),
		"request_id", requestID,
		"error", err)
	writeError(w, r, http.StatusInternalServerError,
		fmt.Errorf("failed to %s %s: %w", operation, entityType, err))
}

// ListOperation is a generic list operation pattern.
type ListOperation[T any, F any] struct {
	EntityType string
	MetricName string
	Logger     *slog.Logger
	Metrics    metrics.MetricsRegistry
	GetFilters func(*http.Request) F
	ListFromDB func(context.Context, F, int, int) ([]T, int64, error)
	ToResponse func(T) interface{}
}

// Execute performs a generic list operation.
func (op *ListOperation[T, F]) Execute(w http.ResponseWriter, r *http.Request) {
	requestID := getRequestIDFromContext(r.Context())
	op.Logger.Info(fmt.Sprintf("Listing %s", op.EntityType), "request_id", requestID)

	// Parse pagination parameters
	params, err := getPaginationParams(r)
	if err != nil {
		writeError(w, r, http.StatusBadRequest, err)
		return
	}

	// Parse filter parameters
	filters := op.GetFilters(r)

	// Get items from database
	items, total, err := op.ListFromDB(r.Context(), filters, params.Offset, params.PageSize)
	if err != nil {
		handleDatabaseError(w, r, err, "list", op.EntityType, op.Logger)
		return
	}

	// Convert to response format
	responses := make([]interface{}, len(items))
	for i, item := range items {
		responses[i] = op.ToResponse(item)
	}

	// Write paginated response
	writePaginatedResponse(w, r, responses, params, total)

	// Record metrics
	recordCRUDMetric(op.Metrics, op.MetricName, map[string]string{
		"status": "success",
	})
}

// CRUDOperation is a generic CRUD operation pattern.
type CRUDOperation[T any] struct {
	EntityType string
	Logger     *slog.Logger
	Metrics    metrics.MetricsRegistry
}

// ExecuteGet performs a generic get operation.
func (op *CRUDOperation[T]) ExecuteGet(
	w http.ResponseWriter,
	r *http.Request,
	id uuid.UUID,
	getFromDB func(context.Context, uuid.UUID) (*T, error),
	toResponse func(*T) interface{},
	metricName string,
) {
	requestID := getRequestIDFromContext(r.Context())
	op.Logger.Info(fmt.Sprintf("Getting %s", op.EntityType), "request_id", requestID, "id", id)

	// Get item from database
	item, err := getFromDB(r.Context(), id)
	if err != nil {
		handleDatabaseError(w, r, err, "get", op.EntityType, op.Logger)
		return
	}

	response := toResponse(item)
	writeJSON(w, r, http.StatusOK, response)

	// Record metrics
	recordCRUDMetric(op.Metrics, metricName, nil)
}

// ExecuteDelete performs a generic delete operation.
func (op *CRUDOperation[T]) ExecuteDelete(
	w http.ResponseWriter,
	r *http.Request,
	id uuid.UUID,
	deleteFromDB func(context.Context, uuid.UUID) error,
	metricName string,
) {
	requestID := getRequestIDFromContext(r.Context())
	op.Logger.Info(fmt.Sprintf("Deleting %s", op.EntityType), "request_id", requestID, "id", id)

	// Delete item from database
	err := deleteFromDB(r.Context(), id)
	if err != nil {
		handleDatabaseError(w, r, err, "delete", op.EntityType, op.Logger)
		return
	}

	op.Logger.Info(fmt.Sprintf("%s deleted successfully", op.EntityType),
		"request_id", requestID,
		"id", id)

	w.WriteHeader(http.StatusNoContent)

	// Record metrics
	recordCRUDMetric(op.Metrics, metricName, map[string]string{
		"status": "success",
	})
}

// ExecuteUpdate performs a generic update operation.
func (op *CRUDOperation[T]) ExecuteUpdate(
	w http.ResponseWriter,
	r *http.Request,
	id uuid.UUID,
	parseRequest func(*http.Request) (interface{}, error),
	validateRequest func(interface{}) error,
	updateInDB func(context.Context, uuid.UUID, interface{}) (*T, error),
	toResponse func(*T) interface{},
	metricName string,
) {
	requestID := getRequestIDFromContext(r.Context())
	op.Logger.Info(fmt.Sprintf("Updating %s", op.EntityType), "request_id", requestID, "id", id)

	// Parse request body
	req, err := parseRequest(r)
	if err != nil {
		writeError(w, r, http.StatusBadRequest, err)
		return
	}

	// Validate request
	if validateRequest != nil {
		if err := validateRequest(req); err != nil {
			writeError(w, r, http.StatusBadRequest, err)
			return
		}
	}

	// Update item in database
	item, err := updateInDB(r.Context(), id, req)
	if err != nil {
		handleDatabaseError(w, r, err, "update", op.EntityType, op.Logger)
		return
	}

	response := toResponse(item)

	op.Logger.Info(fmt.Sprintf("%s updated successfully", op.EntityType),
		"request_id", requestID,
		"id", id)

	writeJSON(w, r, http.StatusOK, response)

	// Record metrics
	recordCRUDMetric(op.Metrics, metricName, nil)
}

// UpdateEntity is a generic helper to eliminate duplication in update operations.
func UpdateEntity[T any, R any](
	w http.ResponseWriter,
	r *http.Request,
	entityType string,
	logger *slog.Logger,
	metricsRegistry metrics.MetricsRegistry,
	parseAndConvert func(*http.Request) (interface{}, error),
	updateInDB func(context.Context, uuid.UUID, interface{}) (*T, error),
	toResponse func(*T) interface{},
	metricName string,
) {
	id, err := extractUUIDFromPath(r)
	if err != nil {
		writeError(w, r, http.StatusBadRequest, err)
		return
	}

	crudOp := &CRUDOperation[T]{
		EntityType: entityType,
		Logger:     logger,
		Metrics:    metricsRegistry,
	}

	crudOp.ExecuteUpdate(w, r, id,
		parseAndConvert,
		func(req interface{}) error {
			// Skip validation for now as we converted to DB format
			return nil
		},
		updateInDB,
		toResponse,
		metricName)
}

// CreateEntity is a generic helper to eliminate duplication in create operations.
func CreateEntity[T any, R any](
	w http.ResponseWriter,
	r *http.Request,
	entityType string,
	logger *slog.Logger,
	metricsRegistry metrics.MetricsRegistry,
	parseAndConvert func(*http.Request) (interface{}, error),
	createInDB func(context.Context, interface{}) (*T, error),
	toResponse func(*T) interface{},
	metricName string,
) {
	requestID := getRequestIDFromContext(r.Context())
	logger.Info(fmt.Sprintf("Creating %s", entityType), "request_id", requestID)

	// Parse request body
	req, err := parseAndConvert(r)
	if err != nil {
		writeError(w, r, http.StatusBadRequest, err)
		return
	}

	// Create item in database
	item, err := createInDB(r.Context(), req)
	if err != nil {
		handleDatabaseError(w, r, err, "create", entityType, logger)
		return
	}

	response := toResponse(item)

	logger.Info(fmt.Sprintf("%s created successfully", entityType),
		"request_id", requestID)

	writeJSON(w, r, http.StatusCreated, response)

	// Record metrics
	recordCRUDMetric(metricsRegistry, metricName, nil)
}

// JobControlOperation is a job control operation pattern for start/stop operations.
type JobControlOperation struct {
	EntityType string
	Logger     *slog.Logger
	Metrics    metrics.MetricsRegistry
}

// ExecuteStart performs a generic start operation.
func (op *JobControlOperation) ExecuteStart(
	w http.ResponseWriter,
	r *http.Request,
	id uuid.UUID,
	startInDB func(context.Context, uuid.UUID) error,
	getFromDB func(context.Context, uuid.UUID) (interface{}, error),
	toResponse func(interface{}) interface{},
	metricName string,
) {
	requestID := getRequestIDFromContext(r.Context())
	op.Logger.Info(fmt.Sprintf("Starting %s", op.EntityType), "request_id", requestID, "id", id)

	// Start execution
	err := startInDB(r.Context(), id)
	if err != nil {
		handleDatabaseError(w, r, err, "start", op.EntityType, op.Logger)
		return
	}

	// Get updated status
	item, err := getFromDB(r.Context(), id)
	if err != nil {
		op.Logger.Error(fmt.Sprintf("Failed to get %s after start", op.EntityType),
			"request_id", requestID, "id", id, "error", err)
		// Still return success since the job was started
	}

	response := map[string]interface{}{
		"id":         id,
		"status":     "started",
		"message":    fmt.Sprintf("%s has been queued for execution", op.EntityType),
		"timestamp":  time.Now().UTC(),
		"request_id": requestID,
	}

	if item != nil {
		response[op.EntityType] = toResponse(item)
	}

	op.Logger.Info(fmt.Sprintf("%s started successfully", op.EntityType),
		"request_id", requestID,
		"id", id)

	writeJSON(w, r, http.StatusOK, response)

	// Record metrics
	recordCRUDMetric(op.Metrics, metricName, nil)
}

// ExecuteStop performs a generic stop operation.
func (op *JobControlOperation) ExecuteStop(
	w http.ResponseWriter,
	r *http.Request,
	id uuid.UUID,
	stopInDB func(context.Context, uuid.UUID) error,
	metricName string,
) {
	requestID := getRequestIDFromContext(r.Context())
	op.Logger.Info(fmt.Sprintf("Stopping %s", op.EntityType), "request_id", requestID, "id", id)

	// Stop execution
	err := stopInDB(r.Context(), id)
	if err != nil {
		handleDatabaseError(w, r, err, "stop", op.EntityType, op.Logger)
		return
	}

	response := map[string]interface{}{
		"id":         id,
		"status":     "stopped",
		"message":    fmt.Sprintf("%s has been stopped", op.EntityType),
		"timestamp":  time.Now().UTC(),
		"request_id": requestID,
	}

	op.Logger.Info(fmt.Sprintf("%s stopped successfully", op.EntityType),
		"request_id", requestID,
		"id", id)

	writeJSON(w, r, http.StatusOK, response)

	// Record metrics
	recordCRUDMetric(op.Metrics, metricName, nil)
}
