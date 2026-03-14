package api

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// System handler constants.
const (
	healthCheckTimeout = 5 * time.Second
)

// livenessHandler provides a simple liveness check endpoint.
// livenessHandler godoc
// @Summary Liveness check
// @Description Returns simple liveness status without dependency checks
// @Tags System
// @Produce json
// @Success 200 {object} map[string]interface{}
// @Router /liveness [get]
func (s *Server) livenessHandler(w http.ResponseWriter, r *http.Request) {
	response := map[string]interface{}{
		"status":    "alive",
		"timestamp": time.Now().UTC(),
		"uptime":    time.Since(s.startTime).String(),
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(response)
}

// healthHandler provides basic health check endpoint.
// healthHandler godoc
// @Summary Health check
// @Description Returns service health status
// @Tags System
// @Produce json
// @Success 200 {object} handlers.HealthResponse
// @Success 503 {object} handlers.HealthResponse
// @Router /health [get]
func (s *Server) healthHandler(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), healthCheckTimeout)
	defer cancel()

	status := "healthy"
	checks := make(map[string]string)

	// Check database
	if s.database != nil {
		if err := s.database.Ping(ctx); err != nil {
			status = "unhealthy"
			checks["database"] = "failed: " + err.Error()
		} else {
			checks["database"] = "ok"
		}
	} else {
		checks["database"] = "not configured"
	}

	response := map[string]interface{}{
		"status":    status,
		"timestamp": time.Now().UTC(),
		"checks":    checks,
	}

	statusCode := http.StatusOK
	if status == "unhealthy" {
		statusCode = http.StatusServiceUnavailable
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	_ = json.NewEncoder(w).Encode(response)
}

// statusHandler provides detailed status information.
// statusHandler godoc
// @Summary System status
// @Description Returns detailed system status
// @Tags System
// @Produce json
// @Success 200 {object} handlers.StatusResponse
// @Router /status [get]
func (s *Server) statusHandler(w http.ResponseWriter, r *http.Request) {
	response := map[string]interface{}{
		"service":   "scanorama-api",
		"timestamp": time.Now().UTC(),
		"uptime":    time.Since(time.Now()).String(), // Placeholder
		"version":   "0.2.0",
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(response)
}

// versionHandler provides version information.
// versionHandler godoc
// @Summary Version information
// @Description Returns version and build info
// @Tags System
// @Produce json
// @Success 200 {object} handlers.VersionResponse
// @Router /version [get]
func (s *Server) versionHandler(w http.ResponseWriter, r *http.Request) {
	response := map[string]interface{}{
		"version":   "0.2.0",
		"timestamp": time.Now().UTC(),
		"service":   "scanorama",
	}

	s.WriteJSON(w, r, http.StatusOK, response)
}

// metricsHandler provides Prometheus-style metrics.
// metricsHandler godoc
// @Summary Application metrics
// @Description Returns Prometheus metrics
// @Tags System
// @Produce text/plain
// @Success 200 {string} string
// @Failure 404 {object} handlers.ErrorResponse
// @Router /metrics [get]
func (s *Server) metricsHandler(w http.ResponseWriter, r *http.Request) {
	if s.prom == nil {
		http.Error(w, "metrics unavailable", http.StatusNotFound)
		return
	}
	// Serve Prometheus exposition format
	promhttp.HandlerFor(s.prom.GetRegistry(), promhttp.HandlerOpts{}).ServeHTTP(w, r)
}

// notImplementedHandler returns a not implemented response.
// notImplementedHandler godoc
// @Summary Not implemented
// @Description Endpoint not yet implemented
// @Tags System
// @Produce json
// @Success 501 {object} handlers.ErrorResponse
// @Router /scans [get]
// @Router /scans [post]
// @Router /hosts [get]
// @Router /discovery [get]
// @Router /discovery [post]
// @Router /profiles [get]
// @Router /profiles [post]
// @Router /schedules [get]
// @Router /schedules [post]

// adminStatusHandler provides administrative status information.
// adminStatusHandler godoc
// @Summary Admin status
// @Description Returns admin status info
// @Tags Admin
// @Produce json
// @Security ApiKeyAuth
// @Success 200 {object} handlers.AdminStatusResponse
// @Failure 401 {object} handlers.ErrorResponse
// @Failure 403 {object} handlers.ErrorResponse
// @Router /admin/status [get]
func (s *Server) adminStatusHandler(w http.ResponseWriter, r *http.Request) {
	response := map[string]interface{}{
		"admin_status": "active",
		"timestamp":    time.Now().UTC(),
		"server_info": map[string]interface{}{
			"address":       s.httpServer.Addr,
			"read_timeout":  s.httpServer.ReadTimeout,
			"write_timeout": s.httpServer.WriteTimeout,
		},
	}

	s.WriteJSON(w, r, http.StatusOK, response)
}

// redirectToAPI returns API information for root requests.
func (s *Server) redirectToAPI(w http.ResponseWriter, r *http.Request) {
	response := map[string]interface{}{
		"service": "Scanorama API",
		"version": "v1",
		"endpoints": map[string]string{
			"liveness": "/api/v1/liveness",
			"health":   "/api/v1/health",
			"status":   "/api/v1/status",
			"docs":     "/swagger/",
		},
		"timestamp": time.Now().UTC(),
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(response); err != nil {
		s.logger.Error("Failed to encode API index response", "error", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}
}

// redirectToSwagger redirects to the Swagger UI.
func (s *Server) redirectToSwagger(w http.ResponseWriter, r *http.Request) {
	http.Redirect(w, r, "/swagger/index.html", http.StatusMovedPermanently)
}
