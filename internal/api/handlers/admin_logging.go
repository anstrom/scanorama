// Package handlers provides HTTP request handlers for the Scanorama API.
// This file implements the AdminHandler log retrieval endpoint backed by the
// in-memory ring buffer.
package handlers

import (
	"net/http"
	"time"

	"github.com/anstrom/scanorama/internal/logging"
)

// LogsResponse is the JSON envelope returned by GET /api/v1/admin/logs.
type LogsResponse struct {
	Data       []logging.LogEntry `json:"data"`
	Pagination PaginationMeta     `json:"pagination"`
}

// PaginationMeta carries the pagination summary in a LogsResponse.
type PaginationMeta struct {
	Page       int `json:"page"`
	PageSize   int `json:"page_size"`
	TotalItems int `json:"total_items"`
	TotalPages int `json:"total_pages"`
}

// GetLogs handles GET /api/v1/admin/logs – retrieve system logs.
//
// Supported query parameters:
//
//	level      – minimum log level: debug | info | warn | error
//	component  – exact component name filter
//	search     – case-insensitive substring match on the log message
//	since      – RFC 3339 timestamp; omit entries before this time
//	until      – RFC 3339 timestamp; omit entries after this time
//	page       – 1-based page number (default 1)
//	page_size  – entries per page (default 50, max 100)
func (h *AdminHandler) GetLogs(w http.ResponseWriter, r *http.Request) {
	requestID := getRequestIDFromContext(r.Context())
	h.logger.Info("Getting logs", "request_id", requestID)

	// Validate pagination params first so we can return 400 on bad input.
	pagination, err := getPaginationParams(r)
	if err != nil {
		writeError(w, r, http.StatusBadRequest, err)
		return
	}

	// Short-circuit when no ring buffer is wired.
	if h.ringBuffer == nil {
		writeJSON(w, r, http.StatusOK, LogsResponse{
			Data: []logging.LogEntry{},
			Pagination: PaginationMeta{
				Page:       pagination.Page,
				PageSize:   pagination.PageSize,
				TotalItems: 0,
				TotalPages: 0,
			},
		})
		return
	}

	q := r.URL.Query()

	// Parse optional time filters; silently ignore malformed values.
	var since, until time.Time
	if s := q.Get("since"); s != "" {
		if t, parseErr := time.Parse(time.RFC3339, s); parseErr == nil {
			since = t
		}
	}
	if u := q.Get("until"); u != "" {
		if t, parseErr := time.Parse(time.RFC3339, u); parseErr == nil {
			until = t
		}
	}

	filter := logging.LogFilter{
		Level:     q.Get("level"),
		Component: q.Get("component"),
		Search:    q.Get("search"),
		Since:     since,
		Until:     until,
		Page:      pagination.Page,
		PageSize:  pagination.PageSize,
	}

	entries, total := h.ringBuffer.Query(filter)

	// Ensure we never serialize a nil slice as JSON null.
	if entries == nil {
		entries = []logging.LogEntry{}
	}

	totalPages := 0
	if total > 0 {
		totalPages = (total + pagination.PageSize - 1) / pagination.PageSize
	}

	writeJSON(w, r, http.StatusOK, LogsResponse{
		Data: entries,
		Pagination: PaginationMeta{
			Page:       pagination.Page,
			PageSize:   pagination.PageSize,
			TotalItems: total,
			TotalPages: totalPages,
		},
	})
}
