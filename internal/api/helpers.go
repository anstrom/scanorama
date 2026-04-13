// Package api — HTTP response helpers, pagination, and query parameter utilities.
package api

import (
	"encoding/json"
	"fmt"
	"net/http"
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
