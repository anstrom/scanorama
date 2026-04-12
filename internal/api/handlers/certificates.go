// Package handlers provides HTTP request handlers for the Scanorama API.
// This file implements TLS certificate endpoints.
package handlers

import (
	"fmt"
	"log/slog"
	"net/http"

	"github.com/anstrom/scanorama/internal/db"
	"github.com/anstrom/scanorama/internal/metrics"
)

const (
	maxExpiryDays     = 90
	defaultExpiryDays = 30
)

// CertificateHandler handles TLS certificate endpoints.
type CertificateHandler struct {
	repo    *db.BannerRepository
	logger  *slog.Logger
	metrics *metrics.Registry
}

// NewCertificateHandler creates a new CertificateHandler.
func NewCertificateHandler(repo *db.BannerRepository, logger *slog.Logger, m *metrics.Registry) *CertificateHandler {
	return &CertificateHandler{
		repo:    repo,
		logger:  logger.With("handler", "certificates"),
		metrics: m,
	}
}

// GetExpiringCertificates handles GET /api/v1/certificates/expiring
//
//	@Summary		List expiring TLS certificates
//	@Description	Returns TLS certificates expiring within the specified number of days (default 30, max 90).
//	@Description	Certificates are joined with their host to include the host IP and hostname.
//	@Tags			certificates
//	@Produce		json
//	@Param			days	query		int	false	"Lookahead window in days (1–90, default 30)"
//	@Success		200		{object}	db.ExpiringCertificatesResponse
//	@Failure		400		{object}	ErrorResponse
//	@Failure		500		{object}	ErrorResponse
//	@Router			/certificates/expiring [get]
func (h *CertificateHandler) GetExpiringCertificates(w http.ResponseWriter, r *http.Request) {
	days := defaultExpiryDays
	if raw := r.URL.Query().Get("days"); raw != "" {
		var err error
		days, err = parsePositiveInt(raw)
		if err != nil || days < 1 {
			writeError(w, r, http.StatusBadRequest,
				fmt.Errorf("days must be a positive integer between 1 and %d", maxExpiryDays))
			return
		}
		if days > maxExpiryDays {
			writeError(w, r, http.StatusBadRequest,
				fmt.Errorf("days must not exceed %d", maxExpiryDays))
			return
		}
	}

	certs, err := h.repo.ListExpiringCertificatesWithHosts(r.Context(), days)
	if err != nil {
		h.logger.Error("failed to list expiring certificates", "error", err)
		writeError(w, r, http.StatusInternalServerError,
			fmt.Errorf("failed to list expiring certificates: %w", err))
		return
	}

	writeJSON(w, r, http.StatusOK, db.ExpiringCertificatesResponse{
		Certificates: certs,
		Days:         days,
	})
}

// parsePositiveInt parses a decimal integer string. Returns an error for
// non-numeric input but does not enforce sign; callers check the value.
func parsePositiveInt(s string) (int, error) {
	var n int
	_, err := fmt.Sscanf(s, "%d", &n)
	return n, err
}
