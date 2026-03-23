// Package services provides business logic for Scanorama operations.
// This file implements scan management, including input validation, profile
// verification, and enriched state-transition methods.
package services

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"strconv"
	"strings"

	"github.com/google/uuid"

	"github.com/anstrom/scanorama/internal/db"
	"github.com/anstrom/scanorama/internal/errors"
)

// -- Validation constants --

// MaxScanNameLength is the maximum allowed length for a scan name.
const MaxScanNameLength = 100

// MaxTargetLength is the maximum allowed length for a single target string.
const MaxTargetLength = 200

// MaxTargetCount is the maximum number of targets allowed per scan.
const MaxTargetCount = 100

// validScanTypes lists the scan-type values accepted by the service layer.
var validScanTypes = map[string]bool{
	"connect":       true,
	"syn":           true,
	"ack":           true,
	"udp":           true,
	"aggressive":    true,
	"comprehensive": true,
}

// -- Repository interface --

// scanRepository is the DB-facing interface consumed by ScanService.
type scanRepository interface {
	ListScans(ctx context.Context, filters db.ScanFilters, offset, limit int) ([]*db.Scan, int64, error)
	CreateScan(ctx context.Context, input db.CreateScanInput) (*db.Scan, error)
	GetScan(ctx context.Context, id uuid.UUID) (*db.Scan, error)
	UpdateScan(ctx context.Context, id uuid.UUID, input db.UpdateScanInput) (*db.Scan, error)
	DeleteScan(ctx context.Context, id uuid.UUID) error
	StartScan(ctx context.Context, id uuid.UUID) error
	StopScan(ctx context.Context, id uuid.UUID, errMsg ...string) error
	CompleteScan(ctx context.Context, id uuid.UUID) error
	GetScanResults(ctx context.Context, scanID uuid.UUID, offset, limit int) ([]*db.ScanResult, int64, error)
	GetScanSummary(ctx context.Context, scanID uuid.UUID) (*db.ScanSummary, error)
	GetProfile(ctx context.Context, id string) (*db.ScanProfile, error)
}

// -- Service --

// ScanService provides business logic for scan lifecycle operations.
type ScanService struct {
	repo   scanRepository
	logger *slog.Logger
}

// NewScanService creates a new ScanService backed by the given repository.
func NewScanService(repo scanRepository, logger *slog.Logger) *ScanService {
	return &ScanService{
		repo:   repo,
		logger: logger,
	}
}

// DB returns the underlying raw *db.DB connection, or nil when the repository
// is not a concrete *db.ScanRepository (e.g. a mock in tests).
// This is used exclusively by the scan execution pipeline which needs direct
// database access to persist discovered hosts and port-scan results.
func (s *ScanService) DB() *db.DB {
	if repo, ok := s.repo.(*db.ScanRepository); ok {
		return repo.DB()
	}
	return nil
}

// ListScans retrieves scans with optional filtering and pagination.
func (s *ScanService) ListScans(
	ctx context.Context,
	filters db.ScanFilters,
	offset, limit int,
) ([]*db.Scan, int64, error) {
	return s.repo.ListScans(ctx, filters, offset, limit)
}

// CreateScan validates the input, optionally verifies the referenced profile,
// and delegates creation to the repository.
func (s *ScanService) CreateScan(ctx context.Context, input db.CreateScanInput) (*db.Scan, error) {
	if err := validateScanInput(input); err != nil {
		return nil, err
	}

	if input.ProfileID != nil && *input.ProfileID != "" {
		_, err := s.repo.GetProfile(ctx, *input.ProfileID)
		if err != nil {
			if errors.IsNotFound(err) {
				return nil, errors.NewScanError(
					errors.CodeValidation,
					fmt.Sprintf("profile %q not found", *input.ProfileID),
				)
			}
			return nil, fmt.Errorf("failed to verify profile: %w", err)
		}
	}

	return s.repo.CreateScan(ctx, input)
}

// GetScan retrieves a single scan by its ID.
func (s *ScanService) GetScan(ctx context.Context, id uuid.UUID) (*db.Scan, error) {
	return s.repo.GetScan(ctx, id)
}

// UpdateScan applies the given mutations to an existing scan.
func (s *ScanService) UpdateScan(
	ctx context.Context,
	id uuid.UUID,
	input db.UpdateScanInput,
) (*db.Scan, error) {
	return s.repo.UpdateScan(ctx, id, input)
}

// DeleteScan removes a scan record by ID.
func (s *ScanService) DeleteScan(ctx context.Context, id uuid.UUID) error {
	return s.repo.DeleteScan(ctx, id)
}

// StartScan transitions a scan to the running state.
// It fetches the current scan to guard against invalid state transitions,
// calls repo.StartScan, then returns the refreshed scan record.
func (s *ScanService) StartScan(ctx context.Context, id uuid.UUID) (*db.Scan, error) {
	scan, err := s.repo.GetScan(ctx, id)
	if err != nil {
		return nil, err
	}

	switch scan.Status {
	case db.ScanJobStatusRunning:
		return nil, errors.ErrConflictWithReason("scan", "scan is already running")
	case db.ScanJobStatusCompleted:
		return nil, errors.ErrConflictWithReason("scan", "scan is already completed")
	}

	if err := s.repo.StartScan(ctx, id); err != nil {
		return nil, err
	}

	return s.repo.GetScan(ctx, id)
}

// StopScan halts a running scan, optionally recording an error message.
func (s *ScanService) StopScan(ctx context.Context, id uuid.UUID, errMsg ...string) error {
	return s.repo.StopScan(ctx, id, errMsg...)
}

// CompleteScan marks a scan as successfully completed.
func (s *ScanService) CompleteScan(ctx context.Context, id uuid.UUID) error {
	return s.repo.CompleteScan(ctx, id)
}

// GetScanResults retrieves paginated port-scan results for a given scan.
func (s *ScanService) GetScanResults(
	ctx context.Context,
	scanID uuid.UUID,
	offset, limit int,
) ([]*db.ScanResult, int64, error) {
	return s.repo.GetScanResults(ctx, scanID, offset, limit)
}

// GetScanSummary retrieves aggregated statistics for a given scan.
func (s *ScanService) GetScanSummary(ctx context.Context, scanID uuid.UUID) (*db.ScanSummary, error) {
	return s.repo.GetScanSummary(ctx, scanID)
}

// GetProfile retrieves a scan profile by its string ID.
func (s *ScanService) GetProfile(ctx context.Context, id string) (*db.ScanProfile, error) {
	return s.repo.GetProfile(ctx, id)
}

// -- Input validation --

// validateScanInput checks all fields of a CreateScanInput before the record
// is persisted. Returns a *errors.ScanError with CodeValidation on failure.
func validateScanInput(input db.CreateScanInput) error {
	if input.Name == "" {
		return errors.NewScanError(errors.CodeValidation, "scan name is required")
	}
	if len(input.Name) > MaxScanNameLength {
		return errors.NewScanError(errors.CodeValidation,
			fmt.Sprintf("scan name too long (max %d characters)", MaxScanNameLength))
	}

	if len(input.Targets) == 0 {
		return errors.NewScanError(errors.CodeValidation, "at least one target is required")
	}
	if len(input.Targets) > MaxTargetCount {
		return errors.NewScanError(errors.CodeValidation,
			fmt.Sprintf("too many targets (max %d)", MaxTargetCount))
	}

	for i, target := range input.Targets {
		if target == "" {
			return errors.NewScanError(errors.CodeValidation,
				fmt.Sprintf("target %d is empty", i+1))
		}
		if len(target) > MaxTargetLength {
			return errors.NewScanError(errors.CodeValidation,
				fmt.Sprintf("target %d too long (max %d characters)", i+1, MaxTargetLength))
		}
		if _, _, err := net.ParseCIDR(target); err != nil {
			if net.ParseIP(target) == nil {
				return errors.NewScanError(errors.CodeValidation,
					fmt.Sprintf("target %d: %q is not a valid IP address or CIDR range", i+1, target))
			}
		}
	}

	if !validScanTypes[input.ScanType] {
		return errors.NewScanError(errors.CodeValidation,
			fmt.Sprintf("invalid scan type: %s", input.ScanType))
	}

	if input.Ports == "" {
		return errors.NewScanError(errors.CodeValidation, "ports is required")
	}
	if err := ParsePortSpec(input.Ports); err != nil {
		return errors.NewScanError(errors.CodeValidation, err.Error())
	}

	return nil
}

// -- Port-spec parsing (exported so handlers and tests can reuse) --

// ParsePortSpec validates a port specification string.
// The spec is comma-separated with optional T:/U: protocol prefixes and
// optional hyphenated ranges (e.g. "T:80,U:53,1024-9999").
// Every individual port value must be in the range 1-65535.
func ParsePortSpec(ports string) error {
	for _, token := range strings.Split(ports, ",") {
		token = strings.TrimSpace(token)
		if token == "" {
			continue
		}
		if err := parsePortToken(token); err != nil {
			return err
		}
	}
	return nil
}

// parsePortToken validates a single port token (after comma-splitting).
// It strips an optional T:/U: prefix, rejects whitespace, then validates the
// port number or range.
func parsePortToken(token string) error {
	// Strip optional protocol prefix (T: or U:).
	if len(token) >= 2 && (token[0] == 'T' || token[0] == 'U') && token[1] == ':' {
		token = token[2:]
	}
	// Reject tokens containing whitespace (e.g. "80 - 443").
	if strings.ContainsAny(token, " 	") {
		return fmt.Errorf("invalid port spec %q: whitespace not allowed", token)
	}
	parts := strings.SplitN(token, "-", 2)
	if len(parts) == 2 {
		return parsePortRange(parts[0], parts[1])
	}
	return validatePortNumber(parts[0])
}

// parsePortRange validates that start and end are valid ports and start <= end.
func parsePortRange(startStr, endStr string) error {
	startNum, err := strconv.Atoi(startStr)
	if err != nil {
		return fmt.Errorf("invalid port %q: must be a number", startStr)
	}
	if startNum < 1 || startNum > 65535 {
		return fmt.Errorf("invalid port %d: must be between 1 and 65535", startNum)
	}
	endNum, err := strconv.Atoi(endStr)
	if err != nil {
		return fmt.Errorf("invalid port %q: must be a number", endStr)
	}
	if endNum < 1 || endNum > 65535 {
		return fmt.Errorf("invalid port %d: must be between 1 and 65535", endNum)
	}
	if startNum > endNum {
		return fmt.Errorf("invalid port range %d-%d: start must be <= end", startNum, endNum)
	}
	return nil
}

// validatePortNumber checks that s is a valid port-number string (1-65535).
func validatePortNumber(s string) error {
	portNum, err := strconv.Atoi(s)
	if err != nil {
		return fmt.Errorf("invalid port %q: must be a number", s)
	}
	if portNum < 1 || portNum > 65535 {
		return fmt.Errorf("invalid port %d: must be between 1 and 65535", portNum)
	}
	return nil
}
