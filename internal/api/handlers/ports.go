// Package handlers provides HTTP request handlers for the Scanorama API.
// This file implements port definition lookup and browsing endpoints.
package handlers

import (
	"fmt"
	"log/slog"
	"net/http"
	"strconv"

	"github.com/gorilla/mux"

	"github.com/anstrom/scanorama/internal/db"
	"github.com/anstrom/scanorama/internal/errors"
	"github.com/anstrom/scanorama/internal/metrics"
)

const (
	protoTCP = "tcp"
	protoUDP = "udp"
)

// PortHandler handles port definition endpoints.
type PortHandler struct {
	repo    *db.PortRepository
	logger  *slog.Logger
	metrics *metrics.Registry
}

// NewPortHandler creates a new PortHandler.
func NewPortHandler(repo *db.PortRepository, logger *slog.Logger, m *metrics.Registry) *PortHandler {
	return &PortHandler{repo: repo, logger: logger, metrics: m}
}

// ListPorts handles GET /api/v1/ports — list/search port definitions with pagination.
//
//	@Summary      List port definitions
//	@Description  Returns a paginated list of well-known port/service definitions.
//	@Description  Supports filtering by search query, category, and protocol.
//	@Tags         ports
//	@Produce      json
//	@Param        search    query  string  false  "Search by port number, service name, or description"
//	@Param        category  query  string  false  "Filter by category (web, database, windows, etc.)"
//	@Param        protocol  query  string  false  "Filter by protocol (tcp or udp)"
//	@Param        sort_by   query  string  false  "Sort field (port, service, category)"
//	@Param        sort_order query string  false  "Sort direction (asc or desc)"
//	@Param        page      query  int     false  "Page number"
//	@Param        page_size query  int     false  "Results per page"
//	@Success      200  {object}  docs.PortListResponse
//	@Router       /ports [get]
func (h *PortHandler) ListPorts(w http.ResponseWriter, r *http.Request) {
	params, err := getPaginationParams(r)
	if err != nil {
		writeError(w, r, http.StatusBadRequest, err)
		return
	}

	q := r.URL.Query()
	if proto := q.Get("protocol"); proto != "" && proto != protoTCP && proto != protoUDP {
		writeError(w, r, http.StatusBadRequest, fmt.Errorf("invalid protocol %q: must be tcp or udp", proto))
		return
	}
	filters := db.PortFilters{
		Search:    q.Get("search"),
		Category:  q.Get("category"),
		Protocol:  q.Get("protocol"),
		SortBy:    q.Get("sort_by"),
		SortOrder: q.Get("sort_order"),
	}
	if s := q.Get("is_standard"); s != "" {
		v := s == "true"
		filters.IsStandard = &v
	}

	ports, total, err := h.repo.ListPortDefinitions(r.Context(), filters, params.Offset, params.PageSize)
	if err != nil {
		writeError(w, r, http.StatusInternalServerError, fmt.Errorf("failed to list ports: %w", err))
		return
	}
	if ports == nil {
		ports = []*db.PortDefinition{}
	}

	type listResponse struct {
		Ports      []*db.PortDefinition `json:"ports"`
		Total      int64                `json:"total"`
		Page       int                  `json:"page"`
		PageSize   int                  `json:"page_size"`
		TotalPages int                  `json:"total_pages"`
	}

	totalPages := int((total + int64(params.PageSize) - 1) / int64(params.PageSize))
	if totalPages < 1 {
		totalPages = 1
	}

	writeJSON(w, r, http.StatusOK, listResponse{
		Ports:      ports,
		Total:      total,
		Page:       params.Page,
		PageSize:   params.PageSize,
		TotalPages: totalPages,
	})
}

// GetPort handles GET /api/v1/ports/{port} — look up a single port definition.
//
//	@Summary      Get port definition
//	@Description  Returns the service definition for a specific port number.
//	@Description  Use ?protocol=tcp (default) or ?protocol=udp.
//	@Tags         ports
//	@Produce      json
//	@Param        port      path   int     true   "Port number"
//	@Param        protocol  query  string  false  "Protocol (tcp or udp, default tcp)"
//	@Success      200  {object}  db.PortDefinition
//	@Failure      404  {object}  docs.ErrorResponse
//	@Router       /ports/{port} [get]
func (h *PortHandler) GetPort(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	portStr, ok := vars["port"]
	if !ok || portStr == "" {
		writeError(w, r, http.StatusBadRequest, fmt.Errorf("port not provided"))
		return
	}

	portNum, err := strconv.Atoi(portStr)
	if err != nil || portNum < 1 || portNum > 65535 {
		writeError(w, r, http.StatusBadRequest, fmt.Errorf("invalid port number: %s", portStr))
		return
	}

	protocol := r.URL.Query().Get("protocol")
	if protocol == "" {
		protocol = db.ProtocolTCP
	}
	if protocol != db.ProtocolTCP && protocol != db.ProtocolUDP {
		writeError(w, r, http.StatusBadRequest, fmt.Errorf("protocol must be tcp or udp"))
		return
	}

	def, err := h.repo.GetPortDefinition(r.Context(), portNum, protocol)
	if err != nil {
		if errors.IsNotFound(err) {
			writeError(w, r, http.StatusNotFound, fmt.Errorf("no definition for port %d/%s", portNum, protocol))
			return
		}
		writeError(w, r, http.StatusInternalServerError, fmt.Errorf("port lookup failed: %w", err))
		return
	}

	writeJSON(w, r, http.StatusOK, def)
}

// ListPortHostCounts handles GET /api/v1/ports/host-counts — returns per-port open host counts.
//
//	@Summary      List port host counts
//	@Description  Returns the number of distinct hosts with each port open, ordered by host count descending.
//	@Tags         ports
//	@Produce      json
//	@Success      200  {array}   docs.PortHostCountResponse
//	@Router       /ports/host-counts [get]
//	@Security     BearerAuth
func (h *PortHandler) ListPortHostCounts(w http.ResponseWriter, r *http.Request) {
	counts, err := h.repo.ListPortHostCounts(r.Context())
	if err != nil {
		writeError(w, r, http.StatusInternalServerError, fmt.Errorf("failed to list port host counts: %w", err))
		return
	}
	if counts == nil {
		counts = []db.PortHostCount{}
	}

	writeJSON(w, r, http.StatusOK, counts)
}

// ListPortCategories handles GET /api/v1/ports/categories — list distinct categories.
//
//	@Summary      List port categories
//	@Description  Returns the distinct category values used in the port definition database.
//	@Tags         ports
//	@Produce      json
//	@Success      200  {object}  docs.StringListResponse
//	@Router       /ports/categories [get]
func (h *PortHandler) ListPortCategories(w http.ResponseWriter, r *http.Request) {
	cats, err := h.repo.ListCategories(r.Context())
	if err != nil {
		writeError(w, r, http.StatusInternalServerError, fmt.Errorf("failed to list categories: %w", err))
		return
	}

	writeJSON(w, r, http.StatusOK, map[string]interface{}{"categories": cats})
}
