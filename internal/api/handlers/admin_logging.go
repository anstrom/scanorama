// Package handlers provides HTTP request handlers for the Scanorama API.
// This file implements the AdminHandler log retrieval endpoint.
package handlers

import (
	"fmt"
	"net/http"
)

// GetLogs handles GET /api/v1/admin/logs - retrieve system logs.
func (h *AdminHandler) GetLogs(w http.ResponseWriter, r *http.Request) {
	requestID := getRequestIDFromContext(r.Context())
	h.logger.Info("Getting logs", "request_id", requestID)

	// Parse pagination parameters — still returns 400 on bad input.
	if _, err := getPaginationParams(r); err != nil {
		writeError(w, r, http.StatusBadRequest, err)
		return
	}

	h.logger.Info("GetLogs called but not yet implemented", "request_id", requestID)

	// Log retrieval is not yet implemented.
	writeError(w, r, http.StatusNotImplemented,
		fmt.Errorf("get logs is not yet implemented"))
}
