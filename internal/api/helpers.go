// Package api — HTTP response helpers, pagination, and query parameter utilities.
package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"time"
)

// ErrorResponse represents a standard API error response.
type ErrorResponse struct {
	Error     string    `json:"error"`
	Timestamp time.Time `json:"timestamp"`
	RequestID string    `json:"request_id,omitempty"`
}

// writeError writes a standardized error response.
func (s *Server) writeError(w http.ResponseWriter, r *http.Request, statusCode int, err error) {
	s.logger.Error("API error",
		"method", r.Method,
		"path", r.URL.Path,
		"status", statusCode,
		"error", err,
		"remote_addr", r.RemoteAddr)

	response := ErrorResponse{
		Error:     err.Error(),
		Timestamp: time.Now().UTC(),
		RequestID: getRequestID(r),
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)

	if encodeErr := json.NewEncoder(w).Encode(response); encodeErr != nil {
		s.logger.Error("Failed to encode error response", "error", encodeErr)
	}
}

// WriteJSON writes a JSON response.
func (s *Server) WriteJSON(w http.ResponseWriter, r *http.Request, statusCode int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)

	if err := json.NewEncoder(w).Encode(data); err != nil {
		s.logger.Error("Failed to encode JSON response",
			"error", err,
			"path", r.URL.Path,
			"method", r.Method)
		// Don't try to write another error response here as headers are already sent
	}
}

// getRequestID extracts or generates a request ID.
func getRequestID(r *http.Request) string {
	// Try to get request ID from header first
	if reqID := r.Header.Get("X-Request-ID"); reqID != "" {
		return reqID
	}

	// Generate a simple request ID
	return fmt.Sprintf("%d", time.Now().UnixNano())
}

// ParseJSON parses JSON request body into the provided struct.
func (s *Server) ParseJSON(r *http.Request, v interface{}) error {
	if r.Body == nil {
		return fmt.Errorf("empty request body")
	}

	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields() // Strict parsing

	if err := decoder.Decode(v); err != nil {
		return fmt.Errorf("invalid JSON: %w", err)
	}

	return nil
}

// GetQueryParam gets a query parameter with optional default value.
func (s *Server) GetQueryParam(r *http.Request, key, defaultValue string) string {
	if value := r.URL.Query().Get(key); value != "" {
		return value
	}
	return defaultValue
}

// GetQueryParamInt gets an integer query parameter with optional default value.
func (s *Server) GetQueryParamInt(r *http.Request, key string, defaultValue int) (int, error) {
	if value := r.URL.Query().Get(key); value != "" {
		return strconv.Atoi(value)
	}
	return defaultValue, nil
}

// GetQueryParamBool gets a boolean query parameter with optional default value.
func (s *Server) GetQueryParamBool(r *http.Request, key string, defaultValue bool) bool {
	if value := r.URL.Query().Get(key); value != "" {
		if parsed, err := strconv.ParseBool(value); err == nil {
			return parsed
		}
	}
	return defaultValue
}

// ValidateRequest performs basic request validation.
func (s *Server) ValidateRequest(r *http.Request) error {
	// Check content type for POST/PUT requests
	if r.Method == "POST" || r.Method == "PUT" {
		contentType := r.Header.Get("Content-Type")
		if contentType != "application/json" {
			return fmt.Errorf("content-type must be application/json, got: %s", contentType)
		}
	}

	return nil
}

// PaginationParams represents pagination parameters.
type PaginationParams struct {
	Page     int `json:"page"`
	PageSize int `json:"page_size"`
	Offset   int `json:"offset"`
}

// GetPaginationParams extracts pagination parameters from request.
func (s *Server) GetPaginationParams(r *http.Request) PaginationParams {
	const (
		defaultPage     = 1
		defaultPageSize = 20
		maxPageSize     = 100
	)

	page, err := s.GetQueryParamInt(r, "page", defaultPage)
	if err != nil || page < 1 {
		page = defaultPage
	}

	pageSize, err := s.GetQueryParamInt(r, "page_size", defaultPageSize)
	if err != nil || pageSize < 1 {
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
	}
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

// WritePaginatedResponse writes a paginated response.
func (s *Server) WritePaginatedResponse(
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

	s.WriteJSON(w, r, http.StatusOK, response)
}
