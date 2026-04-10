// Package handlers — tag management endpoints.
// These methods are added to HostHandler since tags are a property of hosts.
package handlers

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"github.com/google/uuid"

	"github.com/anstrom/scanorama/internal/errors"
)

// tagModifyRequest is the body for PUT/POST/DELETE /api/v1/hosts/{id}/tags.
type tagModifyRequest struct {
	Tags []string `json:"tags"`
}

// bulkTagRequest is the body for POST /api/v1/hosts/bulk/tags.
type bulkTagRequest struct {
	HostIDs []string `json:"host_ids"`
	Tags    []string `json:"tags"`
	Action  string   `json:"action"` // "add", "remove", "set"
}

// ListTags handles GET /api/v1/tags.
// Returns a deduplicated, sorted list of all tags in use across all hosts.
func (h *HostHandler) ListTags(w http.ResponseWriter, r *http.Request) {
	tags, err := h.service.ListTags(r.Context())
	if err != nil {
		writeError(w, r, http.StatusInternalServerError, fmt.Errorf("failed to list tags: %w", err))
		return
	}
	if tags == nil {
		tags = []string{}
	}
	writeJSON(w, r, http.StatusOK, map[string]interface{}{"tags": tags})
}

// applyTagsOp is a shared helper for tag-mutation endpoints that parse the
// request body, validate the tag list, call serviceOp, then return the
// refreshed host. opName is used only in error messages (e.g. "update").
func (h *HostHandler) applyTagsOp(
	w http.ResponseWriter,
	r *http.Request,
	opName string,
	serviceOp func(ctx context.Context, id uuid.UUID, tags []string) error,
) {
	id, err := extractUUIDFromPath(r)
	if err != nil {
		writeError(w, r, http.StatusBadRequest, fmt.Errorf("invalid host ID: %w", err))
		return
	}

	var req tagModifyRequest
	if err := parseJSON(r, &req); err != nil {
		writeError(w, r, http.StatusBadRequest, fmt.Errorf("invalid request body: %w", err))
		return
	}

	tags := normaliseTags(req.Tags)
	if err := validateTagList(tags); err != nil {
		writeError(w, r, http.StatusBadRequest, err)
		return
	}

	if err := serviceOp(r.Context(), id, tags); err != nil {
		handleDatabaseError(w, r, err, opName+" tags for", "host", h.logger)
		return
	}

	host, err := h.service.GetHost(r.Context(), id)
	if err != nil {
		handleDatabaseError(w, r, err, "get", "host", h.logger)
		return
	}
	writeJSON(w, r, http.StatusOK, h.hostToResponse(host))
}

// ReplaceHostTags handles PUT /api/v1/hosts/{id}/tags.
// Replaces the host's entire tag list.
func (h *HostHandler) ReplaceHostTags(w http.ResponseWriter, r *http.Request) {
	h.applyTagsOp(w, r, "update", h.service.UpdateHostTags)
}

// AddHostTags handles POST /api/v1/hosts/{id}/tags.
// Appends tags to the host's tag list (deduplicating the result).
func (h *HostHandler) AddHostTags(w http.ResponseWriter, r *http.Request) {
	h.applyTagsOp(w, r, "add", h.service.AddHostTags)
}

// DeleteHostTags handles DELETE /api/v1/hosts/{id}/tags.
// Removes the specified tags from the host's tag list.
func (h *HostHandler) DeleteHostTags(w http.ResponseWriter, r *http.Request) {
	id, err := extractUUIDFromPath(r)
	if err != nil {
		writeError(w, r, http.StatusBadRequest, fmt.Errorf("invalid host ID: %w", err))
		return
	}

	var req tagModifyRequest
	if err := parseJSON(r, &req); err != nil {
		writeError(w, r, http.StatusBadRequest, fmt.Errorf("invalid request body: %w", err))
		return
	}

	if err := h.service.RemoveHostTags(r.Context(), id, req.Tags); err != nil {
		handleDatabaseError(w, r, err, "remove tags for", "host", h.logger)
		return
	}

	host, err := h.service.GetHost(r.Context(), id)
	if err != nil {
		handleDatabaseError(w, r, err, "get", "host", h.logger)
		return
	}
	writeJSON(w, r, http.StatusOK, h.hostToResponse(host))
}

// BulkUpdateTags handles POST /api/v1/hosts/bulk/tags.
// Applies an add/remove/set tag operation to multiple hosts at once.
func (h *HostHandler) BulkUpdateTags(w http.ResponseWriter, r *http.Request) {
	var req bulkTagRequest
	if err := parseJSON(r, &req); err != nil {
		writeError(w, r, http.StatusBadRequest, fmt.Errorf("invalid request body: %w", err))
		return
	}

	if len(req.HostIDs) == 0 {
		writeError(w, r, http.StatusBadRequest, fmt.Errorf("host_ids must not be empty"))
		return
	}
	if req.Action != "add" && req.Action != "remove" && req.Action != "set" {
		writeError(w, r, http.StatusBadRequest, fmt.Errorf(`action must be "add", "remove", or "set"`))
		return
	}

	ids := make([]uuid.UUID, 0, len(req.HostIDs))
	for _, raw := range req.HostIDs {
		id, err := uuid.Parse(raw)
		if err != nil {
			writeError(w, r, http.StatusBadRequest, fmt.Errorf("invalid host ID %q: %w", raw, err))
			return
		}
		ids = append(ids, id)
	}

	tags := normaliseTags(req.Tags)
	if req.Action != "remove" {
		if err := validateTagList(tags); err != nil {
			writeError(w, r, http.StatusBadRequest, err)
			return
		}
	}

	if err := h.service.BulkUpdateTags(r.Context(), ids, tags, req.Action); err != nil {
		writeError(w, r, http.StatusInternalServerError, fmt.Errorf("bulk tag update failed: %w", err))
		return
	}

	writeJSON(w, r, http.StatusOK, map[string]interface{}{
		"updated": len(ids),
		"action":  req.Action,
	})
}

// normaliseTags trims whitespace and filters empty strings.
func normaliseTags(tags []string) []string {
	out := make([]string, 0, len(tags))
	for _, t := range tags {
		t = strings.TrimSpace(t)
		if t != "" {
			out = append(out, t)
		}
	}
	return out
}

// validateTagList validates a normalised tag list against the same rules as
// the host request validator (max count, max length, no duplicates).
func validateTagList(tags []string) error {
	if len(tags) > maxHostTagCount {
		return errors.NewScanError(errors.CodeValidation,
			"too many tags: maximum is 100")
	}
	seen := make(map[string]struct{}, len(tags))
	for i, tag := range tags {
		if len(tag) > maxHostTagLength {
			return errors.NewScanError(errors.CodeValidation,
				fmt.Sprintf("tag %d exceeds maximum length of %d characters", i+1, maxHostTagLength))
		}
		if _, dup := seen[tag]; dup {
			return errors.NewScanError(errors.CodeValidation,
				fmt.Sprintf("duplicate tag %q", tag))
		}
		seen[tag] = struct{}{}
	}
	return nil
}
