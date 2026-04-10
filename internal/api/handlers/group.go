// Package handlers — host group endpoints.
package handlers

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/google/uuid"

	"github.com/anstrom/scanorama/internal/db"
	"github.com/anstrom/scanorama/internal/metrics"
)

// GroupHandler handles group-related API endpoints.
type GroupHandler struct {
	service GroupServicer
	logger  *slog.Logger
	metrics *metrics.Registry
}

// NewGroupHandler creates a new GroupHandler.
func NewGroupHandler(service GroupServicer, logger *slog.Logger, m *metrics.Registry) *GroupHandler {
	return &GroupHandler{
		service: service,
		logger:  logger.With("handler", "group"),
		metrics: m,
	}
}

// GroupRequest is the body for create/update group endpoints.
type GroupRequest struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Color       string `json:"color"`
}

// GroupMembershipRequest is the body for add/remove hosts from group endpoints.
type GroupMembershipRequest struct {
	HostIDs []string `json:"host_ids"`
}

// groupMemberResponse is a lightweight host representation for group membership listings.
type groupMemberResponse struct {
	ID        string   `json:"id"`
	IPAddress string   `json:"ip_address"`
	Hostname  string   `json:"hostname,omitempty"`
	Status    string   `json:"status"`
	Tags      []string `json:"tags,omitempty"`
	LastSeen  string   `json:"last_seen"`
}

// ListGroups handles GET /api/v1/groups.
// Returns all host groups with a total count.
func (h *GroupHandler) ListGroups(w http.ResponseWriter, r *http.Request) {
	groups, err := h.service.ListGroups(r.Context())
	if err != nil {
		h.logger.Error("Failed to list groups", "error", err)
		writeError(w, r, http.StatusInternalServerError, fmt.Errorf("failed to list groups: %w", err))
		return
	}
	if groups == nil {
		groups = []*db.HostGroup{}
	}
	writeJSON(w, r, http.StatusOK, map[string]interface{}{
		"groups": groups,
		"total":  len(groups),
	})
}

// CreateGroup handles POST /api/v1/groups.
// Creates a new host group and returns 201 with the created group.
func (h *GroupHandler) CreateGroup(w http.ResponseWriter, r *http.Request) {
	var req GroupRequest
	if err := parseJSON(r, &req); err != nil {
		writeError(w, r, http.StatusBadRequest, fmt.Errorf("invalid request body: %w", err))
		return
	}

	if req.Name == "" {
		writeError(w, r, http.StatusBadRequest, fmt.Errorf("group name is required"))
		return
	}

	input := db.CreateGroupInput{
		Name:        req.Name,
		Description: req.Description,
		Color:       req.Color,
	}

	group, err := h.service.CreateGroup(r.Context(), input)
	if err != nil {
		handleDatabaseError(w, r, err, "create", "group", h.logger)
		return
	}

	writeJSON(w, r, http.StatusCreated, group)
}

// GetGroup handles GET /api/v1/groups/{id}.
// Returns a single group by its UUID.
func (h *GroupHandler) GetGroup(w http.ResponseWriter, r *http.Request) {
	id, err := extractUUIDFromPath(r)
	if err != nil {
		writeError(w, r, http.StatusBadRequest, fmt.Errorf("invalid group ID: %w", err))
		return
	}

	group, err := h.service.GetGroup(r.Context(), id)
	if err != nil {
		handleDatabaseError(w, r, err, "get", "group", h.logger)
		return
	}

	writeJSON(w, r, http.StatusOK, group)
}

// UpdateGroup handles PUT /api/v1/groups/{id}.
// Applies a partial update to an existing group.
func (h *GroupHandler) UpdateGroup(w http.ResponseWriter, r *http.Request) {
	id, err := extractUUIDFromPath(r)
	if err != nil {
		writeError(w, r, http.StatusBadRequest, fmt.Errorf("invalid group ID: %w", err))
		return
	}

	var req GroupRequest
	if err := parseJSON(r, &req); err != nil {
		writeError(w, r, http.StatusBadRequest, fmt.Errorf("invalid request body: %w", err))
		return
	}

	input := db.UpdateGroupInput{}
	if req.Name != "" {
		input.Name = &req.Name
	}
	if req.Description != "" {
		input.Description = &req.Description
	}
	if req.Color != "" {
		input.Color = &req.Color
	}

	group, err := h.service.UpdateGroup(r.Context(), id, input)
	if err != nil {
		handleDatabaseError(w, r, err, "update", "group", h.logger)
		return
	}

	writeJSON(w, r, http.StatusOK, group)
}

// DeleteGroup handles DELETE /api/v1/groups/{id}.
// Removes a host group and returns 204 No Content.
func (h *GroupHandler) DeleteGroup(w http.ResponseWriter, r *http.Request) {
	id, err := extractUUIDFromPath(r)
	if err != nil {
		writeError(w, r, http.StatusBadRequest, fmt.Errorf("invalid group ID: %w", err))
		return
	}

	if err := h.service.DeleteGroup(r.Context(), id); err != nil {
		handleDatabaseError(w, r, err, "delete", "group", h.logger)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// ListGroupMembers handles GET /api/v1/groups/{id}/hosts.
// Returns a paginated list of hosts belonging to the group.
func (h *GroupHandler) ListGroupMembers(w http.ResponseWriter, r *http.Request) {
	id, err := extractUUIDFromPath(r)
	if err != nil {
		writeError(w, r, http.StatusBadRequest, fmt.Errorf("invalid group ID: %w", err))
		return
	}

	params, err := getPaginationParams(r)
	if err != nil {
		writeError(w, r, http.StatusBadRequest, err)
		return
	}

	hosts, total, err := h.service.GetGroupMembers(r.Context(), id, params.Offset, params.PageSize)
	if err != nil {
		handleDatabaseError(w, r, err, "list members of", "group", h.logger)
		return
	}

	members := make([]groupMemberResponse, 0, len(hosts))
	for _, host := range hosts {
		m := groupMemberResponse{
			ID:        host.ID.String(),
			IPAddress: host.IPAddress.String(),
			Status:    host.Status,
			LastSeen:  host.LastSeen.Format(time.RFC3339),
			Tags:      host.Tags,
		}
		if host.Hostname != nil {
			m.Hostname = *host.Hostname
		}
		members = append(members, m)
	}

	writePaginatedResponse(w, r, members, params, total)
}

// modifyGroupMembers is a shared helper for the add/remove membership endpoints.
// It parses and validates the request, calls serviceOp, then writes a JSON
// response with resultKey set to the number of hosts affected.
func (h *GroupHandler) modifyGroupMembers(
	w http.ResponseWriter,
	r *http.Request,
	opName string,
	resultKey string,
	serviceOp func(ctx context.Context, groupID uuid.UUID, hostIDs []uuid.UUID) error,
) {
	id, err := extractUUIDFromPath(r)
	if err != nil {
		writeError(w, r, http.StatusBadRequest, fmt.Errorf("invalid group ID: %w", err))
		return
	}

	var req GroupMembershipRequest
	if err := parseJSON(r, &req); err != nil {
		writeError(w, r, http.StatusBadRequest, fmt.Errorf("invalid request body: %w", err))
		return
	}

	if len(req.HostIDs) == 0 {
		writeError(w, r, http.StatusBadRequest, fmt.Errorf("host_ids must not be empty"))
		return
	}

	hostIDs, err := parseHostIDs(req.HostIDs)
	if err != nil {
		writeError(w, r, http.StatusBadRequest, err)
		return
	}

	if err := serviceOp(r.Context(), id, hostIDs); err != nil {
		handleDatabaseError(w, r, err, opName+" members", "group", h.logger)
		return
	}

	writeJSON(w, r, http.StatusOK, map[string]interface{}{
		resultKey: len(hostIDs),
	})
}

// AddGroupMembers handles POST /api/v1/groups/{id}/hosts.
// Adds one or more hosts to the group.
func (h *GroupHandler) AddGroupMembers(w http.ResponseWriter, r *http.Request) {
	h.modifyGroupMembers(w, r, "add", "added", h.service.AddHostsToGroup)
}

// RemoveGroupMembers handles DELETE /api/v1/groups/{id}/hosts.
// Removes one or more hosts from the group.
func (h *GroupHandler) RemoveGroupMembers(w http.ResponseWriter, r *http.Request) {
	h.modifyGroupMembers(w, r, "remove", "removed", h.service.RemoveHostsFromGroup)
}

// parseHostIDs converts a slice of raw UUID strings into []uuid.UUID.
// Returns the first parse error encountered, if any.
func parseHostIDs(raw []string) ([]uuid.UUID, error) {
	ids := make([]uuid.UUID, 0, len(raw))
	for _, s := range raw {
		id, err := uuid.Parse(s)
		if err != nil {
			return nil, fmt.Errorf("invalid host ID %q: %w", s, err)
		}
		ids = append(ids, id)
	}
	return ids, nil
}
