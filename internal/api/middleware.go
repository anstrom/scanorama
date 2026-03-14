package api

import (
	"bufio"
	"fmt"
	"net"
	"net/http"
	"time"

	gorilla "github.com/gorilla/handlers"
	"github.com/gorilla/mux"

	"github.com/anstrom/scanorama/internal/api/middleware"
)

// Metrics constants.
const (
	statusServerErrorMin = 500
)

// setupMiddleware configures middleware for the API server.
func (s *Server) setupMiddleware(apiConfig *Config) {
	// Basic recovery middleware to prevent panics
	s.router.Use(s.recoveryMiddleware)

	// Basic logging middleware
	s.router.Use(s.loggingMiddleware)

	// CORS middleware
	if apiConfig.EnableCORS {
		corsOptions := gorilla.AllowedOrigins(apiConfig.CORSOrigins)
		corsHeaders := gorilla.AllowedHeaders([]string{"Content-Type", "Authorization", "X-API-Key"})
		corsMethods := gorilla.AllowedMethods([]string{"GET", "POST", "PUT", "DELETE", "OPTIONS"})
		s.router.Use(gorilla.CORS(corsOptions, corsHeaders, corsMethods))
	}

	// Authentication middleware (supports both config and database keys)
	if apiConfig.AuthEnabled {
		s.router.Use(middleware.Authentication(apiConfig.APIKeys, s.database, s.logger))
	}

	// Content type validation
	s.router.Use(s.contentTypeMiddleware)
}

// recoveryMiddleware recovers from panics and returns a 500 error.
func (s *Server) recoveryMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if err := recover(); err != nil {
				s.logger.Error("Panic in API handler",
					"error", err,
					"path", r.URL.Path,
					"method", r.Method)

				if !headersSent(w) {
					s.writeError(w, r, http.StatusInternalServerError, fmt.Errorf("internal server error"))
				}
			}
		}()
		next.ServeHTTP(w, r)
	})
}

// loggingMiddleware logs HTTP requests.
func (s *Server) loggingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()

		// Wrap response writer to capture status code
		wrapped := &responseWriter{ResponseWriter: w, statusCode: 200}

		next.ServeHTTP(wrapped, r)

		duration := time.Since(start)

		s.logger.Info("HTTP request",
			"method", r.Method,
			"path", r.URL.Path,
			"status", wrapped.statusCode,
			"duration", duration,
			"remote_addr", r.RemoteAddr,
			"user_agent", r.UserAgent())

		// Record metrics
		s.metrics.Counter("http_requests_total", map[string]string{
			"method": r.Method,
			"status": fmt.Sprintf("%d", wrapped.statusCode),
		})

		s.metrics.Histogram("http_request_duration_seconds", duration.Seconds(), map[string]string{
			"method": r.Method,
		})

		// Prometheus HTTP metrics (use route template to avoid high cardinality)
		if s.prom != nil {
			pathLabel := r.URL.Path
			if route := mux.CurrentRoute(r); route != nil {
				if tmpl, err := route.GetPathTemplate(); err == nil && tmpl != "" {
					pathLabel = tmpl
				}
			}
			s.prom.IncrementHTTPRequests(r.Method, pathLabel, fmt.Sprintf("%d", wrapped.statusCode))
			s.prom.RecordHTTPDuration(r.Method, pathLabel, duration)
			if wrapped.statusCode >= statusServerErrorMin {
				s.prom.IncrementHTTPErrors(r.Method, pathLabel, "server_error")
			}
		}
	})
}

// contentTypeMiddleware validates content type for POST/PUT requests.
func (s *Server) contentTypeMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "POST" || r.Method == "PUT" {
			contentType := r.Header.Get("Content-Type")
			if contentType != "" && contentType != "application/json" {
				s.writeError(w, r, http.StatusUnsupportedMediaType,
					fmt.Errorf("unsupported content type: %s", contentType))
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}

// responseWriter wraps http.ResponseWriter to capture status code.
type responseWriter struct {
	http.ResponseWriter
	statusCode int
}

func (rw *responseWriter) WriteHeader(code int) {
	rw.statusCode = code
	rw.ResponseWriter.WriteHeader(code)
}

// Hijack implements http.Hijacker interface to support WebSocket upgrades
func (rw *responseWriter) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	if hijacker, ok := rw.ResponseWriter.(http.Hijacker); ok {
		return hijacker.Hijack()
	}
	return nil, nil, fmt.Errorf("responseWriter does not implement http.Hijacker")
}

// headersSent checks if headers have been sent.
func headersSent(w http.ResponseWriter) bool {
	// This is a simplified check - in real implementations you might use
	// reflection or other techniques to determine if headers were sent
	return false
}
