// Package handlers provides HTTP request handlers for the Scanorama API.
// This file implements the unified search endpoint.
package handlers

import (
	"context"
	"log/slog"
	"net/http"
	"strconv"

	"github.com/anstrom/scanorama/internal/db"
	apierrors "github.com/anstrom/scanorama/internal/errors"
	"github.com/anstrom/scanorama/internal/metrics"
)

const (
	searchDefaultLimit = 10
	searchMaxLimit     = 50
	searchMinQueryLen  = 2
	searchMaxQueryLen  = 100
)

// SearchServicer is the service-level interface consumed by SearchHandler.
type SearchServicer interface {
	Search(ctx context.Context, q string, limit int) (*db.SearchResults, error)
}

// SearchHandler handles the unified search endpoint.
type SearchHandler struct {
	service SearchServicer
	logger  *slog.Logger
	metrics *metrics.Registry
}

// NewSearchHandler creates a new SearchHandler.
func NewSearchHandler(service SearchServicer, logger *slog.Logger, m *metrics.Registry) *SearchHandler {
	return &SearchHandler{
		service: service,
		logger:  logger.With("handler", "search"),
		metrics: m,
	}
}

// Search handles GET /api/v1/search.
//
//	@Summary		Unified cross-entity search
//	@Description	Search across hosts, networks, scans, and profiles in a single request.
//	@Tags			search
//	@Produce		json
//	@Param			q		query		string	true	"Search query (2–100 characters)"
//	@Param			limit	query		int		false	"Maximum results per entity type (default 10, max 50)"
//	@Success		200		{object}	db.SearchResults
//	@Failure		400		{object}	ErrorResponse
//	@Failure		500		{object}	ErrorResponse
//	@Router			/search [get]
func (h *SearchHandler) Search(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query().Get("q")
	if len(q) < searchMinQueryLen {
		writeError(w, r, http.StatusBadRequest,
			apierrors.NewScanError(apierrors.CodeValidation, "q must be at least 2 characters"),
		)
		return
	}
	if len(q) > searchMaxQueryLen {
		writeError(w, r, http.StatusBadRequest,
			apierrors.NewScanError(apierrors.CodeValidation, "q must be at most 100 characters"),
		)
		return
	}

	limit := searchDefaultLimit
	if raw := r.URL.Query().Get("limit"); raw != "" {
		n, err := strconv.Atoi(raw)
		if err != nil || n < 1 {
			writeError(w, r, http.StatusBadRequest,
				apierrors.NewScanError(apierrors.CodeValidation, "limit must be a positive integer"),
			)
			return
		}
		if n > searchMaxLimit {
			n = searchMaxLimit
		}
		limit = n
	}

	results, err := h.service.Search(r.Context(), q, limit)
	if err != nil {
		h.logger.Error("search failed", "query", q, "error", err)
		writeError(w, r, http.StatusInternalServerError, err)
		return
	}

	writeJSON(w, r, http.StatusOK, results)
}
