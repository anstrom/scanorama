// Package middleware provides HTTP middleware functions for the Scanorama API server.
// This package implements logging, metrics, authentication, rate limiting, and other
// cross-cutting concerns for API requests.
package middleware

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"runtime/debug"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/anstrom/scanorama/internal/metrics"
)

// HTTP method constants
const (
	methodGET     = "GET"
	methodPOST    = "POST"
	methodPUT     = "PUT"
	methodDELETE  = "DELETE"
	methodOPTIONS = "OPTIONS"
)

// ContextKey represents a context key type.
type ContextKey string

const (
	// RequestIDKey is the context key for request IDs.
	RequestIDKey ContextKey = "request_id"
	// StartTimeKey is the context key for request start time.
	StartTimeKey ContextKey = "start_time"
	// httpErrorThreshold is the status code threshold for HTTP errors.
	httpErrorThreshold = 400
)

// RateLimiter implements a simple in-memory rate limiter.
type RateLimiter struct {
	requests map[string][]time.Time
	mutex    sync.RWMutex
	limit    int
	window   time.Duration
}

// NewRateLimiter creates a new rate limiter.
func NewRateLimiter(limit int, window time.Duration) *RateLimiter {
	return &RateLimiter{
		requests: make(map[string][]time.Time),
		limit:    limit,
		window:   window,
	}
}

// Allow checks if a request from the given IP is allowed.
func (rl *RateLimiter) Allow(ip string) bool {
	rl.mutex.Lock()
	defer rl.mutex.Unlock()

	now := time.Now()
	cutoff := now.Add(-rl.window)

	// Get existing requests for this IP
	requests, exists := rl.requests[ip]
	if !exists {
		requests = make([]time.Time, 0)
	}

	// Filter out old requests
	filtered := make([]time.Time, 0, len(requests))
	for _, reqTime := range requests {
		if reqTime.After(cutoff) {
			filtered = append(filtered, reqTime)
		}
	}

	// Check if we're under the limit
	if len(filtered) >= rl.limit {
		rl.requests[ip] = filtered
		return false
	}

	// Add current request
	filtered = append(filtered, now)
	rl.requests[ip] = filtered

	return true
}

// Cleanup removes old entries from the rate limiter.
func (rl *RateLimiter) Cleanup() {
	rl.mutex.Lock()
	defer rl.mutex.Unlock()

	now := time.Now()
	cutoff := now.Add(-rl.window * 2) // Keep some buffer

	for ip, requests := range rl.requests {
		filtered := make([]time.Time, 0, len(requests))
		for _, reqTime := range requests {
			if reqTime.After(cutoff) {
				filtered = append(filtered, reqTime)
			}
		}

		if len(filtered) == 0 {
			delete(rl.requests, ip)
		} else {
			rl.requests[ip] = filtered
		}
	}
}

// Logging creates a logging middleware that logs HTTP requests and responses.
func Logging(logger *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()

			// Generate request ID
			requestID := generateRequestID()
			ctx := context.WithValue(r.Context(), RequestIDKey, requestID)
			ctx = context.WithValue(ctx, StartTimeKey, start)
			r = r.WithContext(ctx)

			// Create response writer wrapper to capture status code
			wrapped := &responseWriter{
				ResponseWriter: w,
				statusCode:     http.StatusOK,
				size:           0,
			}

			// Set request ID header
			w.Header().Set("X-Request-ID", requestID)

			// Log request
			if logger != nil {
				logger.Info("HTTP request started",
					"request_id", requestID,
					"method", r.Method,
					"path", r.URL.Path,
					"query", r.URL.RawQuery,
					"remote_addr", getClientIP(r),
					"user_agent", r.UserAgent(),
					"content_length", r.ContentLength)
			}

			// Process request
			next.ServeHTTP(wrapped, r)

			// Log response
			duration := time.Since(start)
			if logger != nil {
				logger.Info("HTTP request completed",
					"request_id", requestID,
					"method", r.Method,
					"path", r.URL.Path,
					"status_code", wrapped.statusCode,
					"response_size", wrapped.size,
					"duration_ms", duration.Milliseconds(),
					"remote_addr", getClientIP(r))
			}
		})
	}
}

// Metrics creates a metrics middleware that collects HTTP request metrics.
func Metrics(metricsRegistry metrics.MetricsRegistry) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()

			// Create response writer wrapper to capture status code
			wrapped := &responseWriter{
				ResponseWriter: w,
				statusCode:     http.StatusOK,
				size:           0,
			}

			// Process request
			next.ServeHTTP(wrapped, r)

			// Record metrics
			if metricsRegistry != nil {
				duration := time.Since(start)
				labels := map[string]string{
					"method": r.Method,
					"path":   r.URL.Path,
					"status": strconv.Itoa(wrapped.statusCode),
				}

				metricsRegistry.Counter("http_requests_total", labels)
				metricsRegistry.Histogram("http_request_duration_seconds", duration.Seconds(), labels)
				metricsRegistry.Histogram("http_response_size_bytes", float64(wrapped.size), labels)

				// Record error rate
				if wrapped.statusCode >= httpErrorThreshold {
					errorLabels := map[string]string{
						"method": r.Method,
						"path":   r.URL.Path,
						"status": strconv.Itoa(wrapped.statusCode),
					}
					metricsRegistry.Counter("http_errors_total", errorLabels)
				}
			}
		})
	}
}

// Recovery creates a recovery middleware that catches panics.
func Recovery(logger *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			defer func() {
				if err := recover(); err != nil {
					requestID := GetRequestID(r)
					stack := debug.Stack()

					logger.Error("HTTP request panic recovered",
						"request_id", requestID,
						"method", r.Method,
						"path", r.URL.Path,
						"panic", err,
						"stack", string(stack),
						"remote_addr", getClientIP(r))

					// Return 500 error
					w.Header().Set("Content-Type", "application/json")
					w.WriteHeader(http.StatusInternalServerError)

					response := map[string]interface{}{
						"error":      "Internal server error",
						"request_id": requestID,
						"timestamp":  time.Now().UTC(),
					}

					if encodeErr := json.NewEncoder(w).Encode(response); encodeErr != nil {
						logger.Error("Failed to encode panic response", "error", encodeErr)
					}
				}
			}()

			next.ServeHTTP(w, r)
		})
	}
}

// Authentication creates an authentication middleware using API keys.
func Authentication(apiKeys []string, logger *slog.Logger) func(http.Handler) http.Handler {
	keySet := make(map[string]bool)
	for _, key := range apiKeys {
		keySet[key] = true
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Skip authentication for health checks
			if r.URL.Path == "/api/v1/health" || r.URL.Path == "/api/v1/version" || r.URL.Path == "/api/v1/liveness" {
				next.ServeHTTP(w, r)
				return
			}

			// Get API key from header
			apiKey := r.Header.Get("X-API-Key")
			if apiKey == "" {
				// Try Authorization header as backup
				auth := r.Header.Get("Authorization")
				if strings.HasPrefix(auth, "Bearer ") {
					apiKey = strings.TrimPrefix(auth, "Bearer ")
				}
			}

			if apiKey == "" {
				logger.Warn("API request without authentication",
					"request_id", GetRequestID(r),
					"path", r.URL.Path,
					"remote_addr", getClientIP(r))

				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusUnauthorized)
				response := map[string]interface{}{
					"error":      "Authentication required",
					"message":    "Provide API key in X-API-Key header or Authorization: Bearer <key>",
					"request_id": GetRequestID(r),
					"timestamp":  time.Now().UTC(),
				}
				_ = json.NewEncoder(w).Encode(response)
				return
			}

			// Validate API key
			if !keySet[apiKey] {
				logger.Warn("API request with invalid key",
					"request_id", GetRequestID(r),
					"path", r.URL.Path,
					"remote_addr", getClientIP(r))

				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusUnauthorized)
				response := map[string]interface{}{
					"error":      "Authentication failed: Invalid API key",
					"request_id": GetRequestID(r),
					"timestamp":  time.Now().UTC(),
				}
				_ = json.NewEncoder(w).Encode(response)
				return
			}

			// Authentication successful
			logger.Debug("API request authenticated",
				"request_id", GetRequestID(r),
				"path", r.URL.Path,
				"remote_addr", getClientIP(r))

			next.ServeHTTP(w, r)
		})
	}
}

// RateLimit creates a rate limiting middleware.
func RateLimit(requests int, window time.Duration, logger *slog.Logger) func(http.Handler) http.Handler {
	limiter := NewRateLimiter(requests, window)

	// Start cleanup goroutine
	go func() {
		ticker := time.NewTicker(window)
		defer ticker.Stop()
		for range ticker.C {
			limiter.Cleanup()
		}
	}()

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			clientIP := getClientIP(r)

			if !limiter.Allow(clientIP) {
				logger.Warn("Rate limit exceeded",
					"request_id", GetRequestID(r),
					"client_ip", clientIP,
					"path", r.URL.Path,
					"limit", requests,
					"window", window)

				w.Header().Set("Content-Type", "application/json")
				w.Header().Set("X-RateLimit-Limit", strconv.Itoa(requests))
				w.Header().Set("X-RateLimit-Window", window.String())
				w.WriteHeader(http.StatusTooManyRequests)

				response := map[string]interface{}{
					"error":       "Rate limit exceeded",
					"message":     fmt.Sprintf("Maximum %d requests per %s", requests, window),
					"request_id":  GetRequestID(r),
					"timestamp":   time.Now().UTC(),
					"retry_after": window.Seconds(),
				}
				_ = json.NewEncoder(w).Encode(response)
				return
			}

			// Add rate limit headers
			w.Header().Set("X-RateLimit-Limit", strconv.Itoa(requests))
			w.Header().Set("X-RateLimit-Window", window.String())

			next.ServeHTTP(w, r)
		})
	}
}

// ContentType creates a content type middleware that validates request content types.
func ContentType() func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Skip content type validation for GET, DELETE, and OPTIONS requests
			if r.Method == methodGET || r.Method == methodDELETE || r.Method == methodOPTIONS {
				next.ServeHTTP(w, r)
				return
			}

			// For POST and PUT requests, expect JSON content type
			if r.Method == methodPOST || r.Method == methodPUT {
				contentType := r.Header.Get("Content-Type")
				if contentType != "" && !strings.HasPrefix(contentType, "application/json") {
					w.Header().Set("Content-Type", "application/json")
					w.WriteHeader(http.StatusUnsupportedMediaType)

					response := map[string]interface{}{
						"error":      "Unsupported media type",
						"message":    "Content-Type must be application/json",
						"expected":   "application/json",
						"received":   contentType,
						"request_id": GetRequestID(r),
						"timestamp":  time.Now().UTC(),
					}
					_ = json.NewEncoder(w).Encode(response)
					return
				}
			}

			next.ServeHTTP(w, r)
		})
	}
}

// responseWriter wraps http.ResponseWriter to capture response information.
type responseWriter struct {
	http.ResponseWriter
	statusCode int
	size       int
}

// WriteHeader captures the status code.
func (rw *responseWriter) WriteHeader(code int) {
	rw.statusCode = code
	rw.ResponseWriter.WriteHeader(code)
}

// Write captures the response size.
func (rw *responseWriter) Write(b []byte) (int, error) {
	size, err := rw.ResponseWriter.Write(b)
	rw.size += size
	return size, err
}

// generateRequestID generates a unique request ID.
func generateRequestID() string {
	bytes := make([]byte, 8)
	if _, err := rand.Read(bytes); err != nil {
		// Fallback to timestamp-based ID if random generation fails
		return fmt.Sprintf("req_%d", time.Now().UnixNano())
	}
	return "req_" + hex.EncodeToString(bytes)
}

// GetRequestID extracts the request ID from context.
func GetRequestID(r *http.Request) string {
	if requestID, ok := r.Context().Value(RequestIDKey).(string); ok {
		return requestID
	}
	return "unknown"
}

// getClientIP extracts the real client IP address from the request.
func getClientIP(r *http.Request) string {
	// Check X-Forwarded-For header first (for load balancers/proxies)
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		// Take the first IP if multiple are present
		if ips := strings.Split(xff, ","); len(ips) > 0 {
			return strings.TrimSpace(ips[0])
		}
	}

	// Check X-Real-IP header
	if xri := r.Header.Get("X-Real-IP"); xri != "" {
		return xri
	}

	// Fall back to RemoteAddr
	if strings.Contains(r.RemoteAddr, ":") {
		if ip := strings.Split(r.RemoteAddr, ":")[0]; ip != "" {
			return ip
		}
	}

	return "unknown"
}

// CORS creates a simple CORS middleware.
func CORS(origins, headers, methods []string) func(http.Handler) http.Handler {
	originsMap := make(map[string]bool)
	for _, origin := range origins {
		originsMap[origin] = true
	}

	allowAllOrigins := originsMap["*"]

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			origin := r.Header.Get("Origin")

			// Set CORS headers
			if allowAllOrigins || originsMap[origin] {
				w.Header().Set("Access-Control-Allow-Origin", origin)
			}

			if len(headers) > 0 {
				w.Header().Set("Access-Control-Allow-Headers", strings.Join(headers, ", "))
			}

			if len(methods) > 0 {
				w.Header().Set("Access-Control-Allow-Methods", strings.Join(methods, ", "))
			}

			w.Header().Set("Access-Control-Allow-Credentials", "true")
			w.Header().Set("Access-Control-Max-Age", "3600")

			// Handle preflight requests
			if r.Method == methodOPTIONS {
				w.WriteHeader(http.StatusNoContent)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

// RequestTimeout creates a request timeout middleware.
func RequestTimeout(timeout time.Duration) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx, cancel := context.WithTimeout(r.Context(), timeout)
			defer cancel()

			r = r.WithContext(ctx)
			next.ServeHTTP(w, r)
		})
	}
}

// SecurityHeaders adds common security headers.
func SecurityHeaders() func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Security headers
			w.Header().Set("X-Content-Type-Options", "nosniff")
			w.Header().Set("X-Frame-Options", "DENY")
			w.Header().Set("X-XSS-Protection", "1; mode=block")
			w.Header().Set("Referrer-Policy", "strict-origin-when-cross-origin")
			w.Header().Set("Content-Security-Policy", "default-src 'self'")

			next.ServeHTTP(w, r)
		})
	}
}

// Compression creates a compression middleware (placeholder for now).
func Compression() func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Pass through without compression
			next.ServeHTTP(w, r)
		})
	}
}
