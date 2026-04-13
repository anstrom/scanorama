// Package handlers - SNMP credential management endpoints.
package handlers

import (
	"fmt"
	"log/slog"
	"net/http"

	"github.com/google/uuid"

	"github.com/anstrom/scanorama/internal/db"
	"github.com/anstrom/scanorama/internal/errors"
)

// SNMPCredentialsHandler handles the /api/v1/admin/snmp/credentials endpoints.
type SNMPCredentialsHandler struct {
	repo   *db.SNMPCredentialsRepository
	logger *slog.Logger
}

// NewSNMPCredentialsHandler creates a new SNMPCredentialsHandler.
func NewSNMPCredentialsHandler(repo *db.SNMPCredentialsRepository, logger *slog.Logger) *SNMPCredentialsHandler {
	return &SNMPCredentialsHandler{
		repo:   repo,
		logger: logger.With("handler", "snmp_credentials"),
	}
}

// snmpCredUpsertRequest is the body for PUT /admin/snmp/credentials.
type snmpCredUpsertRequest struct {
	NetworkID *uuid.UUID     `json:"network_id,omitempty"`
	Version   db.SNMPVersion `json:"version"`
	// v2c
	Community string `json:"community,omitempty"`
	// v3
	Username  string `json:"username,omitempty"`
	AuthProto string `json:"auth_proto,omitempty"`
	AuthPass  string `json:"auth_pass,omitempty"`
	PrivProto string `json:"priv_proto,omitempty"`
	PrivPass  string `json:"priv_pass,omitempty"`
}

func (req *snmpCredUpsertRequest) validate() error {
	if req.Version == "" {
		req.Version = db.SNMPVersionV2c
	}
	if req.Version != db.SNMPVersionV2c && req.Version != db.SNMPVersionV3 {
		return fmt.Errorf("version must be 'v2c' or 'v3'")
	}
	if req.Version == db.SNMPVersionV2c && req.Community == "" {
		return fmt.Errorf("community is required for v2c")
	}
	if req.Version == db.SNMPVersionV3 && req.Username == "" {
		return fmt.Errorf("username is required for v3")
	}
	return nil
}

// ListSNMPCredentials handles GET /api/v1/admin/snmp/credentials.
// Secrets are redacted in the response.
func (h *SNMPCredentialsHandler) ListSNMPCredentials(w http.ResponseWriter, r *http.Request) {
	creds, err := h.repo.ListSNMPCredentials(r.Context())
	if err != nil {
		h.logger.Error("list snmp credentials", "err", err)
		writeError(w, r, http.StatusInternalServerError, err)
		return
	}

	writeJSON(w, r, http.StatusOK, map[string]any{"credentials": creds})
}

// UpsertSNMPCredential handles PUT /api/v1/admin/snmp/credentials.
// Creates or replaces the credential set for the given network/version scope.
func (h *SNMPCredentialsHandler) UpsertSNMPCredential(w http.ResponseWriter, r *http.Request) {
	var req snmpCredUpsertRequest
	if err := parseJSON(r, &req); err != nil {
		writeError(w, r, http.StatusBadRequest, err)
		return
	}
	if err := req.validate(); err != nil {
		writeError(w, r, http.StatusBadRequest, err)
		return
	}

	cred := &db.SNMPCredential{
		NetworkID: req.NetworkID,
		Version:   req.Version,
		Community: req.Community,
		Username:  req.Username,
		AuthProto: req.AuthProto,
		AuthPass:  req.AuthPass,
		PrivProto: req.PrivProto,
		PrivPass:  req.PrivPass,
	}
	saved, err := h.repo.UpsertSNMPCredential(r.Context(), cred)
	if err != nil {
		h.logger.Error("upsert snmp credential", "err", err)
		writeError(w, r, http.StatusInternalServerError, err)
		return
	}

	writeJSON(w, r, http.StatusOK, saved.Redact())
}

// DeleteSNMPCredential handles DELETE /api/v1/admin/snmp/credentials/{id}.
func (h *SNMPCredentialsHandler) DeleteSNMPCredential(w http.ResponseWriter, r *http.Request) {
	id, err := extractUUIDFromPath(r)
	if err != nil {
		writeError(w, r, http.StatusBadRequest, err)
		return
	}

	if err := h.repo.DeleteSNMPCredential(r.Context(), id); err != nil {
		if errors.IsNotFound(err) {
			writeError(w, r, http.StatusNotFound, err)
			return
		}
		h.logger.Error("delete snmp credential", "id", id, "err", err)
		writeError(w, r, http.StatusInternalServerError, err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}
